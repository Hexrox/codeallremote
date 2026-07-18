package lifecycle

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/code-all-remote/car/internal/domain"
	"github.com/code-all-remote/car/internal/storage"
)

type fakeReconciler struct {
	state      string
	canRecover bool
	err        error
	mu         sync.Mutex
	called     int
}

func (f *fakeReconciler) Recover(ctx context.Context, session *domain.Session) (string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.called++
	return f.state, f.canRecover, f.err
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func setupManager(t *testing.T) (*Manager, *storage.DB, func()) {
	db, err := storage.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	// Create prerequisites
	wsRepo := storage.NewWorkspaceRepository(db)
	wsRepo.Create(&domain.Workspace{ID: "ws-1", DisplayName: "Test", Path: "/tmp/ws1"})

	sessRepo := storage.NewSessionRepository(db)
	sessRepo.CreateSession(&domain.Session{
		ID: "session-1", WorkspaceID: "ws-1", AdapterID: "claude-code",
		State: domain.SessionStateActive,
	})

	m := NewManager(db, nil, testLogger())
	cleanup := func() { db.Close() }
	return m, db, cleanup
}

func TestManager_Start(t *testing.T) {
	m, _, cleanup := setupManager(t)
	defer cleanup()

	err := m.Start(context.Background())
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if m.Phase() != PhaseRunning {
		t.Errorf("expected phase running, got %s", m.Phase())
	}

	if !m.IsAcceptingRuns() {
		t.Error("expected to accept runs after start")
	}
}

func TestManager_Shutdown_NoActiveRuns(t *testing.T) {
	m, _, cleanup := setupManager(t)
	defer cleanup()

	m.Start(context.Background())

	err := m.Shutdown(context.Background())
	if err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	if m.Phase() != PhaseStopped {
		t.Errorf("expected phase stopped, got %s", m.Phase())
	}

	if m.IsAcceptingRuns() {
		t.Error("expected to not accept runs after shutdown")
	}
}

func TestManager_Shutdown_DrainsActiveRuns(t *testing.T) {
	m, _, cleanup := setupManager(t)
	defer cleanup()

	m.SetDrainTimeout(100 * time.Millisecond)
	m.Start(context.Background())

	// Register a run
	run, err := m.RegisterRun("session-1", "run-1")
	if err != nil {
		t.Fatalf("RegisterRun failed: %v", err)
	}

	// Simulate work completing
	go func() {
		time.Sleep(10 * time.Millisecond)
		m.CompleteRun("run-1")
	}()

	err = m.Shutdown(context.Background())
	if err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	// Run channel should be closed
	select {
	case <-run.Done:
		// Good
	default:
		t.Error("expected run Done channel to be closed")
	}
}

func TestManager_Shutdown_TimesOut(t *testing.T) {
	m, db, cleanup := setupManager(t)
	defer cleanup()

	m.SetDrainTimeout(50 * time.Millisecond)
	m.Start(context.Background())

	// Register a run that never completes
	m.RegisterRun("session-1", "run-1")

	err := m.Shutdown(context.Background())
	if err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	// Session should be marked for recovery
	sessRepo := storage.NewSessionRepository(db)
	session, _ := sessRepo.GetByID("session-1")
	if session.State != domain.SessionStateRecovering {
		t.Errorf("expected session to be recovering, got %s", session.State)
	}
}

func TestManager_ReconcileOnStartup(t *testing.T) {
	db, err := storage.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	wsRepo := storage.NewWorkspaceRepository(db)
	wsRepo.Create(&domain.Workspace{ID: "ws-1", DisplayName: "Test", Path: "/tmp/ws1"})

	sessRepo := storage.NewSessionRepository(db)
	// Create sessions in various states
	sessRepo.CreateSession(&domain.Session{
		ID: "s-active", WorkspaceID: "ws-1", AdapterID: "claude-code",
		State: domain.SessionStateActive,
	})
	sessRepo.CreateSession(&domain.Session{
		ID: "s-completed", WorkspaceID: "ws-1", AdapterID: "claude-code",
		State: domain.SessionStateCompleted,
	})
	sessRepo.CreateSession(&domain.Session{
		ID: "s-failed", WorkspaceID: "ws-1", AdapterID: "claude-code",
		State: domain.SessionStateFailed,
	})

	// Use a reconciler that says sessions can recover to completed state
	reconciler := &fakeReconciler{
		state:      domain.SessionStateCompleted,
		canRecover: true,
	}

	m := NewManager(db, reconciler, testLogger())
	err = m.Start(context.Background())
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Active session should have been reconciled
	session, _ := sessRepo.GetByID("s-active")
	if session.State != domain.SessionStateCompleted {
		t.Errorf("expected s-active to be completed, got %s", session.State)
	}

	// Completed session should be unchanged
	session, _ = sessRepo.GetByID("s-completed")
	if session.State != domain.SessionStateCompleted {
		t.Errorf("expected s-completed to be completed, got %s", session.State)
	}

	// Failed session should be unchanged
	session, _ = sessRepo.GetByID("s-failed")
	if session.State != domain.SessionStateFailed {
		t.Errorf("expected s-failed to be failed, got %s", session.State)
	}
}

func TestManager_ReconcileOnStartup_NoAdapter(t *testing.T) {
	db, err := storage.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	wsRepo := storage.NewWorkspaceRepository(db)
	wsRepo.Create(&domain.Workspace{ID: "ws-1", DisplayName: "Test", Path: "/tmp/ws1"})

	sessRepo := storage.NewSessionRepository(db)
	sessRepo.CreateSession(&domain.Session{
		ID: "s-active", WorkspaceID: "ws-1", AdapterID: "claude-code",
		State: domain.SessionStateActive,
	})

	// No reconciler
	m := NewManager(db, nil, testLogger())
	m.Start(context.Background())

	// Without an adapter, nonterminal sessions should be marked failed
	session, _ := sessRepo.GetByID("s-active")
	if session.State != domain.SessionStateFailed {
		t.Errorf("expected s-active to be failed without adapter, got %s", session.State)
	}
}

func TestManager_RegisterRun_AfterShutdown(t *testing.T) {
	m, _, cleanup := setupManager(t)
	defer cleanup()

	m.Start(context.Background())

	// Shutdown first
	m.Shutdown(context.Background())

	// Then try to register a run
	_, err := m.RegisterRun("session-1", "run-1")
	if err == nil {
		t.Error("expected error when registering run after shutdown")
	}
}

func TestManager_RegisterRun_DuringShutdown(t *testing.T) {
	m, _, cleanup := setupManager(t)
	defer cleanup()

	m.Start(context.Background())

	// Start shutdown in goroutine
	go m.Shutdown(context.Background())

	// Give it a moment to begin draining
	time.Sleep(10 * time.Millisecond)

	// Should not be able to register runs during shutdown
	_, err := m.RegisterRun("session-1", "run-1")
	if err == nil {
		t.Error("expected error when registering run during shutdown")
	}
}

func TestManager_Wait(t *testing.T) {
	m, _, cleanup := setupManager(t)
	defer cleanup()

	m.Start(context.Background())

	// Shutdown in goroutine
	go func() {
		time.Sleep(10 * time.Millisecond)
		m.Shutdown(context.Background())
	}()

	// Wait should block until shutdown completes
	done := make(chan struct{})
	go func() {
		m.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Good
	case <-time.After(1 * time.Second):
		t.Error("Wait did not return after shutdown")
	}
}

func TestManager_CompleteRun(t *testing.T) {
	m, _, cleanup := setupManager(t)
	defer cleanup()

	m.Start(context.Background())

	run, _ := m.RegisterRun("session-1", "run-1")

	if m.ActiveRunCount() != 1 {
		t.Errorf("expected 1 active run, got %d", m.ActiveRunCount())
	}

	m.CompleteRun("run-1")

	if m.ActiveRunCount() != 0 {
		t.Errorf("expected 0 active runs, got %d", m.ActiveRunCount())
	}

	// Run Done channel should be closed
	select {
	case <-run.Done:
		// Good
	default:
		t.Error("expected run Done to be closed")
	}
}

func TestManager_CompleteRun_Nonexistent(t *testing.T) {
	m, _, cleanup := setupManager(t)
	defer cleanup()

	m.Start(context.Background())

	// Should not panic on unknown run
	m.CompleteRun("nonexistent")
}

func TestManager_ShutdownChannel(t *testing.T) {
	m, _, cleanup := setupManager(t)
	defer cleanup()

	m.Start(context.Background())

	ch := m.ShutdownChannel()

	// Channel should not be closed yet
	select {
	case <-ch:
		t.Error("expected shutdown channel to not be closed")
	default:
		// Good
	}

	m.Shutdown(context.Background())

	// Now it should be closed
	select {
	case <-ch:
		// Good
	default:
		t.Error("expected shutdown channel to be closed")
	}
}

func TestManager_DoubleShutdown(t *testing.T) {
	m, _, cleanup := setupManager(t)
	defer cleanup()

	m.Start(context.Background())

	err := m.Shutdown(context.Background())
	if err != nil {
		t.Fatalf("first Shutdown failed: %v", err)
	}

	// Second shutdown should be a no-op
	err = m.Shutdown(context.Background())
	if err != nil {
		t.Fatalf("second Shutdown failed: %v", err)
	}
}

func TestManager_IsShuttingDown(t *testing.T) {
	m, _, cleanup := setupManager(t)
	defer cleanup()

	m.Start(context.Background())

	if m.IsShuttingDown() {
		t.Error("expected not to be shutting down")
	}

	m.Shutdown(context.Background())

	if !m.IsShuttingDown() {
		t.Error("expected to be shutting down")
	}
}

func TestManager_Shutdown_MultipleActiveRuns(t *testing.T) {
	_, _, cleanup := setupManager(t)
	defer cleanup()

	// Use the manager created in setupManager via db
	db, err := storage.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	wsRepo := storage.NewWorkspaceRepository(db)
	wsRepo.Create(&domain.Workspace{ID: "ws-1", DisplayName: "Test", Path: "/tmp/ws1"})

	sessRepo := storage.NewSessionRepository(db)
	for i := 0; i < 3; i++ {
		sessRepo.CreateSession(&domain.Session{
			ID: "s" + string(rune('a'+i)), WorkspaceID: "ws-1", AdapterID: "claude-code",
			State: domain.SessionStateActive,
		})
	}

	m := NewManager(db, nil, testLogger())
	m.SetDrainTimeout(50 * time.Millisecond)
	m.Start(context.Background())

	// Register multiple runs
	for i := 0; i < 3; i++ {
		m.RegisterRun("s"+string(rune('a'+i)), "r"+string(rune('a'+i)))
	}

	m.Shutdown(context.Background())

	// All sessions should be marked for recovery
	for i := 0; i < 3; i++ {
		session, _ := sessRepo.GetByID("s" + string(rune('a'+i)))
		if session.State != domain.SessionStateRecovering {
			t.Errorf("expected session %s to be recovering, got %s",
				"s"+string(rune('a'+i)), session.State)
		}
	}
}
