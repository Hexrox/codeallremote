// Package lifecycle handles graceful shutdown and startup reconciliation.
package lifecycle

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/code-all-remote/car/internal/domain"
	"github.com/code-all-remote/car/internal/storage"
)

// Phase represents the current lifecycle phase of the server.
type Phase string

const (
	PhaseStarting Phase = "starting"
	PhaseRunning  Phase = "running"
	PhaseDraining Phase = "draining"
	PhaseStopped  Phase = "stopped"
)

// RunReconciler is the interface for adapters that can reconcile runs.
type RunReconciler interface {
	// Recover queries the adapter about a session's recoverable state.
	Recover(ctx context.Context, session *domain.Session) (state string, canRecover bool, err error)
}

// Manager coordinates graceful shutdown and startup reconciliation.
type Manager struct {
	mu           sync.RWMutex
	phase        Phase
	db           *storage.DB
	reconciler   RunReconciler
	logger       *slog.Logger
	activeRuns   map[string]*drainingRun
	drainTimeout time.Duration
	shutdownCh   chan struct{}
	done         chan struct{}
}

// drainingRun tracks a run that is being drained during shutdown.
type drainingRun struct {
	SessionID string
	RunID     string
	StartedAt time.Time
	Done      chan struct{}
}

// NewManager creates a new lifecycle manager.
func NewManager(db *storage.DB, reconciler RunReconciler, logger *slog.Logger) *Manager {
	return &Manager{
		phase:        PhaseStarting,
		db:           db,
		reconciler:   reconciler,
		logger:       logger,
		activeRuns:   make(map[string]*drainingRun),
		drainTimeout: 30 * time.Second,
		shutdownCh:   make(chan struct{}),
		done:         make(chan struct{}),
	}
}

// Start transitions to running phase and performs startup reconciliation.
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	if m.phase != PhaseStarting {
		m.mu.Unlock()
		return fmt.Errorf("cannot start from phase %s", m.phase)
	}
	m.mu.Unlock()

	// Reconcile nonterminal runs from the previous session.
	if err := m.reconcileOnStartup(ctx); err != nil {
		m.logger.Error("startup reconciliation failed", "error", err)
		// Continue anyway - reconciliation failures are not fatal.
	}

	m.mu.Lock()
	m.phase = PhaseRunning
	m.mu.Unlock()

	m.logger.Info("lifecycle manager started")
	return nil
}

// reconcileOnStartup marks unverified active runs as recovering and resolves them.
func (m *Manager) reconcileOnStartup(ctx context.Context) error {
	sessRepo := storage.NewSessionRepository(m.db)

	// Find sessions in nonterminal states.
	sessions, err := sessRepo.GetAll()
	if err != nil {
		return fmt.Errorf("querying sessions: %w", err)
	}

	var reconciled, failed int
	for _, session := range sessions {
		if !isNonterminalState(session.State) {
			continue
		}

		// Mark as recovering.
		m.logger.Info("reconciling session",
			"session_id", session.ID,
			"previous_state", session.State,
		)

		_, err := sessRepo.UpdateState(session.ID, domain.SessionStateRecovering, nil)
		if err != nil {
			m.logger.Error("failed to mark session recovering",
				"session_id", session.ID, "error", err)
			failed++
			continue
		}

		// Query the adapter for actual state.
		resolved, err := m.resolveRecovery(ctx, &session)
		if err != nil {
			m.logger.Error("recovery resolution failed",
				"session_id", session.ID, "error", err)
			// Mark as failed with diagnostic context.
			m.markFailed(sessRepo, session.ID, err.Error())
			failed++
			continue
		}

		_, err = sessRepo.UpdateState(session.ID, resolved, strPtr(domain.SessionStateRecovering))
		if err != nil {
			m.logger.Error("failed to update resolved state",
				"session_id", session.ID, "error", err)
			failed++
			continue
		}

		reconciled++
	}

	if reconciled > 0 || failed > 0 {
		m.logger.Info("startup reconciliation complete",
			"reconciled", reconciled,
			"failed", failed,
		)
	}

	return nil
}

// resolveRecovery queries the adapter for the actual state of a session.
func (m *Manager) resolveRecovery(ctx context.Context, session *domain.Session) (string, error) {
	if m.reconciler == nil {
		// Without an adapter, mark as failed with diagnostic context.
		return domain.SessionStateFailed, nil
	}

	state, canRecover, err := m.reconciler.Recover(ctx, session)
	if err != nil {
		return domain.SessionStateFailed, err
	}

	if !canRecover {
		return domain.SessionStateFailed, nil
	}

	// Map adapter-reported state to a CAR session state.
	// Run states share string values with session states, so we check each once.
	switch state {
	case domain.SessionStateActive:
		return domain.SessionStateActive, nil
	case domain.SessionStateCompleted:
		return domain.SessionStateCompleted, nil
	case domain.SessionStateFailed:
		return domain.SessionStateFailed, nil
	case domain.SessionStateResumable:
		return domain.SessionStateResumable, nil
	case domain.RunStateStarting, domain.RunStatePending:
		return domain.SessionStateStarting, nil
	case domain.RunStateInterrupted:
		return domain.SessionStateInterrupted, nil
	default:
		return domain.SessionStateFailed, nil
	}
}

// markFailed marks a session as failed with diagnostic context.
func (m *Manager) markFailed(repo *storage.SessionRepository, sessionID, reason string) {
	_, err := repo.UpdateState(sessionID, domain.SessionStateFailed, nil)
	if err != nil {
		m.logger.Error("failed to mark session failed",
			"session_id", sessionID, "error", err)
	}
}

// Shutdown initiates graceful shutdown.
// It stops accepting new runs, drains active runs, persists outcomes,
// and marks unresolved processes for recovery on next startup.
func (m *Manager) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	if m.phase == PhaseStopped || m.phase == PhaseDraining {
		m.mu.Unlock()
		return nil
	}
	m.phase = PhaseDraining
	close(m.shutdownCh)
	activeRuns := make(map[string]*drainingRun, len(m.activeRuns))
	for k, v := range m.activeRuns {
		activeRuns[k] = v
	}
	m.mu.Unlock()

	m.logger.Info("shutdown initiated",
		"active_runs", len(activeRuns),
		"drain_timeout", m.drainTimeout,
	)

	// Drain active runs.
	for _, run := range activeRuns {
		m.drainRun(ctx, run)
	}

	// Mark any still-unresolved runs as recovering for next startup.
	if err := m.markUnresolvedForRecovery(); err != nil {
		m.logger.Error("failed to mark unresolved runs", "error", err)
	}

	m.mu.Lock()
	m.phase = PhaseStopped
	close(m.done)
	m.mu.Unlock()

	m.logger.Info("shutdown complete")
	return nil
}

// drainRun waits for a single run to drain or times out.
func (m *Manager) drainRun(ctx context.Context, run *drainingRun) {
	m.logger.Info("draining run",
		"session_id", run.SessionID,
		"run_id", run.RunID,
	)

	select {
	case <-run.Done:
		m.logger.Info("run drained", "run_id", run.RunID)
	case <-time.After(m.drainTimeout):
		m.logger.Warn("run drain timed out, marking for recovery",
			"session_id", run.SessionID,
			"run_id", run.RunID,
		)
		// Mark for recovery on next startup.
		m.markSessionForRecovery(run.SessionID)
	case <-ctx.Done():
		m.logger.Warn("shutdown context cancelled",
			"session_id", run.SessionID,
		)
		m.markSessionForRecovery(run.SessionID)
	}
}

// markSessionForRecovery marks a session as recovering.
func (m *Manager) markSessionForRecovery(sessionID string) {
	sessRepo := storage.NewSessionRepository(m.db)
	_, err := sessRepo.UpdateState(sessionID, domain.SessionStateRecovering, nil)
	if err != nil {
		m.logger.Error("failed to mark session for recovery",
			"session_id", sessionID, "error", err)
	}
}

// markUnresolvedForRecovery marks any nonterminal sessions as recovering.
func (m *Manager) markUnresolvedForRecovery() error {
	sessRepo := storage.NewSessionRepository(m.db)

	sessions, err := sessRepo.GetAll()
	if err != nil {
		return fmt.Errorf("querying sessions: %w", err)
	}

	var marked int
	for _, session := range sessions {
		if !isNonterminalState(session.State) {
			continue
		}

		_, err := sessRepo.UpdateState(session.ID, domain.SessionStateRecovering, nil)
		if err != nil {
			m.logger.Error("failed to mark session recovering",
				"session_id", session.ID, "error", err)
			continue
		}
		marked++
	}

	if marked > 0 {
		m.logger.Info("marked sessions for recovery on next startup", "count", marked)
	}

	return nil
}

// RegisterRun registers an active run for shutdown tracking.
func (m *Manager) RegisterRun(sessionID, runID string) (*drainingRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.phase != PhaseRunning {
		return nil, fmt.Errorf("cannot register run in phase %s (server is shutting down)", m.phase)
	}

	run := &drainingRun{
		SessionID: sessionID,
		RunID:     runID,
		StartedAt: time.Now(),
		Done:      make(chan struct{}),
	}
	m.activeRuns[runID] = run
	return run, nil
}

// CompleteRun marks a run as complete and removes it from active tracking.
func (m *Manager) CompleteRun(runID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if run, ok := m.activeRuns[runID]; ok {
		select {
		case <-run.Done:
			// Already closed
		default:
			close(run.Done)
		}
		delete(m.activeRuns, runID)
	}
}

// Phase returns the current lifecycle phase.
func (m *Manager) Phase() Phase {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.phase
}

// IsAcceptingRuns returns true if new runs can be started.
func (m *Manager) IsAcceptingRuns() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.phase == PhaseRunning
}

// IsShuttingDown returns true if shutdown has been initiated.
func (m *Manager) IsShuttingDown() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.phase == PhaseDraining || m.phase == PhaseStopped
}

// Wait blocks until shutdown is complete.
func (m *Manager) Wait() {
	<-m.done
}

// SetDrainTimeout sets the timeout for draining runs.
func (m *Manager) SetDrainTimeout(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.drainTimeout = d
}

// ActiveRunCount returns the number of active runs.
func (m *Manager) ActiveRunCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.activeRuns)
}

// ShutdownChannel returns a channel that closes when shutdown is initiated.
func (m *Manager) ShutdownChannel() <-chan struct{} {
	return m.shutdownCh
}

// isNonterminalState returns true if the state requires reconciliation.
func isNonterminalState(state string) bool {
	switch state {
	case domain.SessionStateCreated,
		domain.SessionStateStarting,
		domain.SessionStateActive,
		domain.SessionStateWaitingApprov,
		domain.SessionStateRecovering:
		return true
	default:
		return false
	}
}

// strPtr returns a pointer to a string.
func strPtr(s string) *string {
	return &s
}
