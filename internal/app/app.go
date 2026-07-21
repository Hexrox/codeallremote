// Package app composes the CAR core services into a single application
// ready to be served over HTTP. It wires storage, sessions, adapters,
// approvals, audit and the lifecycle manager together.
package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/code-all-remote/car/internal/adapter"
	"github.com/code-all-remote/car/internal/adapter/claude"
	"github.com/code-all-remote/car/internal/approval"
	"github.com/code-all-remote/car/internal/audit"
	"github.com/code-all-remote/car/internal/config"
	"github.com/code-all-remote/car/internal/domain"
	"github.com/code-all-remote/car/internal/identity"
	"github.com/code-all-remote/car/internal/lifecycle"
	"github.com/code-all-remote/car/internal/projection"
	"github.com/code-all-remote/car/internal/session"
	"github.com/code-all-remote/car/internal/storage"
	"github.com/code-all-remote/car/internal/workspace"
)

// App holds the composed CAR core services.
type App struct {
	config        *config.Config
	logger        *slog.Logger
	db            *storage.DB
	workspaces    *workspace.Registry
	sessions      *storage.SessionRepository
	events        *storage.EventRepository
	cursor        *storage.CursorRepository
	approvals     *approval.ApprovalBridge
	approvalStore approval.ApprovalStore
	snapshots     *projection.SnapshotProjection
	audit         *audit.Writer
	lifecycle     *lifecycle.Manager
	idempotent    *session.IdempotencyStore
	stateMachine  *session.StateMachine
	adapters      map[string]adapter.Adapter
	adapterEnv    map[string]map[string]string // per-adapter non-secret env overrides
	identity      *identity.Service
	publisher     EventPublisher
	nextRunID     uint64

	// runCtx is the lifetime context for agent processes and their observer
	// goroutines. It is deliberately detached from any HTTP request context: a
	// run must outlive the request that started it (a request-scoped context is
	// cancelled when the handler returns, which would kill the child process
	// via exec.CommandContext ~immediately after StartRun). runCancel is called
	// on Shutdown as a backstop to terminate any still-running processes.
	runCtx    context.Context
	runCancel context.CancelFunc
	// observers tracks in-flight adapter observer goroutines started by
	// StartRun. Each goroutine calls Done only after its signal loop exits,
	// i.e. after the run's terminal event has been persisted. Tests use
	// waitForObservers to obtain a deterministic "journal is quiescent"
	// barrier without sleeping.
	observers sync.WaitGroup
}

// EventPublisher fans a durable domain event out to live subscribers (e.g. the
// WebSocket hub). It is set by the server after wiring so the app package does
// not import the ws package (avoiding a cycle).
type EventPublisher interface {
	Publish(ev domain.Event)
}

// New composes a new App from configuration.
func New(cfg *config.Config, logger *slog.Logger) (*App, error) {
	// Open storage
	db, err := storage.Open(cfg.Storage.Type, cfg.Storage.DataSource)
	if err != nil {
		return nil, fmt.Errorf("opening storage: %w", err)
	}

	// Default retention for the event journal: 90 days.
	const eventRetention = 90 * 24 * time.Hour

	app := &App{
		config:       cfg,
		logger:       logger,
		db:           db,
		workspaces:   workspace.NewRegistry(db, ""),
		sessions:     storage.NewSessionRepository(db),
		events:       storage.NewEventRepository(db),
		cursor:       storage.NewCursorRepository(db, eventRetention),
		adapterEnv:   make(map[string]map[string]string),
		snapshots:    projection.NewSnapshotProjection(db),
		audit:        audit.NewWriter(db).WithRedactPatterns(cfg.Security.APIToken).WithRedactPatterns(cfg.Security.RedactionPatterns...),
		lifecycle:    lifecycle.NewManager(db, nil, logger),
		idempotent:   session.NewIdempotencyStore(24 * time.Hour),
		stateMachine: session.NewStateMachine(),
		adapters:     make(map[string]adapter.Adapter),
	}

	// Detached lifetime context for runs (see the runCtx field comment).
	app.runCtx, app.runCancel = context.WithCancel(context.Background())

	// Memory store for approvals keeps recent approvals in-process; the DB
	// store provides durability for restart recovery.
	app.approvalStore = approval.NewDBStore(db)
	app.approvals = approval.NewApprovalBridge(app.approvalStore, approval.RealClock{})

	// Identity service backs pairing, access tokens and revocation.
	app.identity = identity.NewService(db)

	// Register the fake adapter for testing/deterministic runs.
	fake := adapter.NewFakeAdapter()
	app.RegisterAdapter(fake)

	// Register configured real adapters (e.g. claude-code). An adapter with an
	// empty ExecPath is not registered; its absence is visible in diagnostics.
	for _, ac := range cfg.Adapters {
		if ac.ID == "" || ac.ExecPath == "" {
			logger.Warn("adapter skipped (missing id or exec_path)", "id", ac.ID)
			continue
		}
		switch ac.ID {
		case "claude-code":
			app.RegisterAdapter(claude.New(ac.ExecPath, logger))
		default:
			logger.Warn("unknown adapter id in config; ignored", "id", ac.ID)
		}
		// Record non-secret env overrides for this adapter (e.g.
		// ANTHROPIC_BASE_URL pointing claude at a local CCR). Passed to the
		// child process on StartRun on top of the server's environment.
		if len(ac.Env) > 0 {
			app.adapterEnv[ac.ID] = ac.Env
		}
	}

	// Register any workspaces declared in config.
	if registered, errs := app.workspaces.LoadFromConfig(toDomainWorkspaces(cfg.Workspaces)); len(errs) > 0 {
		for _, e := range errs {
			logger.Warn("workspace load error", "error", e)
		}
		_ = registered
	}

	return app, nil
}

// RegisterAdapter registers an adapter under its ID.
func (a *App) RegisterAdapter(ad adapter.Adapter) {
	a.adapters[ad.ID()] = ad
}

// Start runs startup reconciliation.
func (a *App) Start(ctx context.Context) error {
	return a.lifecycle.Start(ctx)
}

// APIToken returns the configured bearer token.
func (a *App) APIToken() string {
	return a.config.Security.APIToken
}

// ListSessions returns all sessions as domain objects.
func (a *App) ListSessions() ([]domain.Session, error) {
	return a.sessions.GetAll()
}

// GetSession returns a single session by ID.
func (a *App) GetSession(id string) (*domain.Session, error) {
	s, err := a.sessions.GetByID(id)
	if err != nil {
		return nil, fmt.Errorf("%w: session %s", ErrNotFound, id)
	}
	return s, nil
}

// GetApproval returns an approval by ID.
func (a *App) GetApproval(id string) (*domain.Approval, error) {
	ap, err := a.approvals.GetByID(id)
	if err != nil || ap == nil {
		return nil, fmt.Errorf("%w: approval %s", ErrNotFound, id)
	}
	return ap, nil
}

// GetEvents returns events for a session after a cursor.
func (a *App) GetEvents(sessionID string, after int64, limit int) (*storage.CursorResult, error) {
	if _, err := a.sessions.GetByID(sessionID); err != nil {
		return nil, fmt.Errorf("%w: session %s", ErrNotFound, sessionID)
	}
	result, err := a.cursor.Replay(sessionID, after, limit)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// Shutdown gracefully stops all services.
func (a *App) Shutdown(ctx context.Context) error {
	if err := a.lifecycle.Shutdown(ctx); err != nil {
		a.logger.Error("lifecycle shutdown error", "error", err)
	}
	// Backstop: terminate any agent processes still bound to the run context
	// after the graceful drain above.
	if a.runCancel != nil {
		a.runCancel()
	}
	a.idempotent.Close()
	if err := a.db.Close(); err != nil {
		return fmt.Errorf("closing database: %w", err)
	}
	return nil
}

// RegisterWorkspaceForTest registers a workspace (used by tests).
func (a *App) RegisterWorkspaceForTest(ws *domain.Workspace) {
	a.workspaces.Register(ws)
}

// NewForTest builds an app with an in-memory SQLite DB, a registered workspace,
// and started lifecycle, suitable for cross-package integration tests.
// It returns (app, "") on success or (nil, "error message") on failure so
// callers can t.Fatalf without importing errors.
func NewForTest(t testingT) (*App, string) {
	return newForTestWithRetention(t, 0)
}

// NewForTestWithRetention is like NewForTest but with a non-zero event
// retention window (used to exercise cursor-expiry paths).
func NewForTestWithRetention(t testingT, retention time.Duration) (*App, string) {
	return newForTestWithRetention(t, retention)
}

// testingT is the subset of *testing.T used, so app does not import testing.
type testingT interface {
	Helper()
	TempDir() string
	Fatalf(format string, args ...any)
	Cleanup(func())
}

// newForTestWithRetention is the shared constructor.
func newForTestWithRetention(t testingT, retention time.Duration) (*App, string) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "car.db")
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host: "127.0.0.1", Port: 0,
			ReadTimeout:     config.Duration(5 * time.Second),
			WriteTimeout:    config.Duration(5 * time.Second),
			ShutdownTimeout: config.Duration(5 * time.Second),
		},
		Storage: config.StorageConfig{Type: "sqlite", DataSource: dbPath},
		Security: config.SecurityConfig{
			APIToken: "test-token-min-16-chars", TokenExpiry: config.Duration(24 * time.Hour),
		},
		Logging: config.LoggingConfig{Level: "error", Format: "text", Output: "stderr"},
	}
	a, err := New(cfg, testLoggerInner())
	if err != nil {
		return nil, err.Error()
	}
	wsDir := filepath.Join(tmpDir, "workspace")
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		return nil, err.Error()
	}
	a.workspaces.Register(&domain.Workspace{
		ID: "ws-1", DisplayName: "Test Workspace", Path: wsDir,
		AllowedAdapters: []string{"fake-adapter"},
	})
	if retention > 0 {
		a.cursor = storage.NewCursorRepository(a.db, retention)
	}
	// Keep the drain timeout short for tests so a fake adapter's pending
	// approval does not stall shutdown.
	a.lifecycle.SetDrainTimeout(200 * time.Millisecond)
	if err := a.Start(context.Background()); err != nil {
		return nil, err.Error()
	}
	t.Cleanup(func() { a.Shutdown(context.Background()) })
	return a, ""
}

// testLoggerInner returns a discard logger for tests (avoids importing testing
// in non-_test files by keeping this in app.go).
func testLoggerInner() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// Identity returns the identity service (pairing, tokens, revocation).
func (a *App) Identity() *identity.Service {
	return a.identity
}

// CursorRepository returns the event cursor repository (used by the ws hub).
func (a *App) CursorRepository() *storage.CursorRepository {
	return a.cursor
}

// ReadinessProbe returns a probe that reports readiness by pinging the
// database. It MUST NOT leak secrets in its error (caller wraps as needed).
func (a *App) ReadinessProbe() func(context.Context) error {
	return func(ctx context.Context) error {
		return a.db.PingContext(ctx)
	}
}

// SetEventPublisher wires a live-event fan-out (the WebSocket hub).
// Called by the server after the hub is constructed.
func (a *App) SetEventPublisher(p EventPublisher) {
	a.publisher = p
}

// IdempotencyStore returns the store backing idempotency-aware command
// dedup (docs/13, docs/33). Used by handlers to replay a stored outcome for
// a retried write instead of executing the command twice.
func (a *App) IdempotencyStore() *session.IdempotencyStore {
	return a.idempotent
}

// AllowedOrigin returns the configured CORS allowlist origin ("" = none).
// CAR is Android-first; a browser console would configure this explicitly.
func (a *App) AllowedOrigin() string {
	return a.config.Security.AllowedOrigin
}

// newID returns a fresh opaque ID with the given prefix.
func newID(prefix string) string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		// Fall back to a timestamp-derived value if entropy fails.
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(b)
}

// nextRunID returns a monotonic run counter (for synthetic run IDs).
func (a *App) nextRunSeq() uint64 {
	return atomic.AddUint64(&a.nextRunID, 1)
}

// emitEvent appends a domain event to the journal in a single transaction
// with the session's last_sequence update, then fans it out to live
// subscribers. Persisting before publishing guarantees that a client which
// receives the event live can always recover it through REST replay.
func (a *App) emitEvent(sessionID, eventType string, payload map[string]any) error {
	ev := &domain.Event{
		SessionID:     sessionID,
		Type:          eventType,
		MessageID:     newID("msg"),
		SchemaVersion: 1,
		Payload:       payload,
		OccurredAt:    time.Now().UTC(),
	}
	seq, err := a.cursor.AppendWithoutTx(ev)
	if err != nil {
		return err
	}
	// Publish the persisted event (with assigned sequence) to live subscribers.
	if a.publisher != nil {
		published := *ev
		published.Sequence = seq
		a.publisher.Publish(published)
	}
	return nil
}

// waitForObservers blocks until every adapter observer goroutine started by
// StartRun has finished. Because a goroutine marks itself Done only after its
// signal loop drains (which happens strictly after the terminal event has been
// appended to the journal), a return from this method guarantees the event
// journal for those runs is quiescent. It is used by tests to make cursor
// replay deterministic without sleeping; it is not on any request path.
func (a *App) waitForObservers() {
	a.observers.Wait()
}

// toDomainWorkspaces converts config workspaces to domain workspaces.
func toDomainWorkspaces(cfgs []config.WorkspaceConfig) []domain.Workspace {
	result := make([]domain.Workspace, 0, len(cfgs))
	for _, c := range cfgs {
		result = append(result, domain.Workspace{
			ID:              c.ID,
			DisplayName:     c.DisplayName,
			Path:            c.Path,
			AllowedAdapters: c.AllowedAdapters,
			ExecutionPolicy: domain.ExecutionPolicy{
				AllowNetworkAccess: c.ExecutionPolicy.AllowNetworkAccess,
				AllowWrites:        c.ExecutionPolicy.AllowWrites,
				AllowedCommands:    c.ExecutionPolicy.AllowedCommands,
			},
		})
	}
	return result
}
