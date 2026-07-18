package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/code-all-remote/car/internal/domain"
)

// FakeAdapter is a deterministic fake adapter for testing.
// It simulates agent behavior without requiring a real agent process.
type FakeAdapter struct {
	*BaseAdapter
	mu                    sync.RWMutex
	runs                  map[string]*fakeRun
	signals               map[string]chan AdapterSignal
	scenario              FakeScenario
	compatibilityDegraded bool
}

// FakeScenario defines a deterministic sequence of fake agent behavior.
type FakeScenario struct {
	// StartupDelay is the delay before run.started signal
	StartupDelay time.Duration `json:"startup_delay"`

	// OutputLines are lines of output to emit
	OutputLines []string `json:"output_lines"`

	// OutputDelay is delay between output lines
	OutputDelay time.Duration `json:"output_delay"`

	// RequestApproval at some point during execution
	RequestApproval *ApprovalRequest `json:"request_approval,omitempty"`

	// ExitCode for completion
	ExitCode int `json:"exit_code"`

	// ExitAfterApproval delays before completing after approval
	ExitAfterApproval time.Duration `json:"exit_after_approval"`

	// FailOnStart causes start to fail
	FailOnStart bool `json:"fail_on_start"`

	// FailWithError is emitted if non-empty
	FailWithError string `json:"fail_with_error,omitempty"`
}

// ApprovalRequest defines an approval to request during execution.
type ApprovalRequest struct {
	Category             string         `json:"category"`
	ActionKind           string         `json:"action_kind"`
	HumanReadableContext string         `json:"human_readable_context"`
	StructuredPayload    map[string]any `json:"structured_payload"`
	AfterOutput          int            `json:"after_output"` // request after this many output lines
}

// fakeRun tracks a fake agent run.
type fakeRun struct {
	handle    *RunHandle
	state     string
	startedAt time.Time
	outputIdx int
	approval  *ApprovalRequest
	approved  *bool
	done      chan struct{}
}

// NewFakeAdapter creates a new fake adapter.
func NewFakeAdapter() *FakeAdapter {
	return &FakeAdapter{
		BaseAdapter: NewBaseAdapter("fake-adapter", CapabilitySet{
			SupportsResume:    true,
			SupportsApproval:  true,
			SupportsInterrupt: true,
			SupportsStreaming: true,
			Version:           "1.0.0",
			AgentType:         "fake",
			AgentVersion:      "0.0.0",
		}),
		runs:    make(map[string]*fakeRun),
		signals: make(map[string]chan AdapterSignal),
		scenario: FakeScenario{
			StartupDelay:      100 * time.Millisecond,
			OutputDelay:       50 * time.Millisecond,
			OutputLines:       []string{"Hello from fake agent", "Processing...", "Done."},
			ExitCode:          0,
			ExitAfterApproval: 50 * time.Millisecond,
		},
	}
}

// WithScenario sets the scenario for the fake adapter.
func (a *FakeAdapter) WithScenario(scenario FakeScenario) *FakeAdapter {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.scenario = scenario
	return a
}

// SetCompatibilityDegraded sets the compatibility degraded flag.
func (a *FakeAdapter) SetCompatibilityDegraded(degraded bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.compatibilityDegraded = degraded
}

// IsCompatibilityDegraded returns true if compatibility is degraded.
func (a *FakeAdapter) IsCompatibilityDegraded() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.compatibilityDegraded
}

// ValidateWorkspace always validates for the fake adapter.
func (a *FakeAdapter) ValidateWorkspace(ws *domain.Workspace) ValidationResult {
	return ValidationResult{Valid: true}
}

// Start begins a fake agent run.
func (a *FakeAdapter) Start(ctx context.Context, session *domain.Session, input Input) (*RunHandle, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.scenario.FailOnStart {
		return nil, fmt.Errorf("simulated startup failure: %s", a.scenario.FailWithError)
	}

	now := time.Now()
	handle := &RunHandle{
		ID:        "run-" + session.ID,
		SessionID: session.ID,
		PID:       12345, // Fake PID
		StartedAt: now,
	}

	run := &fakeRun{
		handle:    handle,
		state:     domain.RunStateStarting,
		startedAt: now,
		done:      make(chan struct{}),
	}

	if a.scenario.RequestApproval != nil {
		run.approval = a.scenario.RequestApproval
	}

	a.runs[handle.ID] = run
	a.signals[handle.ID] = make(chan AdapterSignal, 100)

	// Start goroutine to simulate agent execution
	go a.simulateExecution(ctx, run)

	return handle, nil
}

// SubmitInput accepts input for a running agent.
func (a *FakeAdapter) SubmitInput(ctx context.Context, run *RunHandle, prompt string) Accepted {
	return Accepted{Accepted: true, Message: "prompt accepted"}
}

// Interrupt stops a running fake agent.
func (a *FakeAdapter) Interrupt(ctx context.Context, run *RunHandle) Accepted {
	a.mu.Lock()
	defer a.mu.Unlock()

	fakeRun, ok := a.runs[run.ID]
	if !ok {
		return Accepted{Accepted: false, Message: "run not found"}
	}

	select {
	case <-fakeRun.done:
		return Accepted{Accepted: false, Message: "run already completed"}
	default:
		// Signal completion with interrupted state
		close(fakeRun.done)
		fakeRun.state = domain.RunStateInterrupted
		return Accepted{Accepted: true, Message: "interrupt accepted"}
	}
}

// Observe returns the signal channel for a run.
func (a *FakeAdapter) Observe(ctx context.Context, run *RunHandle) <-chan AdapterSignal {
	a.mu.RLock()
	defer a.mu.RUnlock()

	ch, ok := a.signals[run.ID]
	if !ok {
		ch = make(chan AdapterSignal)
		close(ch)
	}
	return ch
}

// DecideApproval records an approval decision.
func (a *FakeAdapter) DecideApproval(ctx context.Context, run *RunHandle, approvalID string, approved bool, reason string) Accepted {
	a.mu.Lock()
	defer a.mu.Unlock()

	fakeRun, ok := a.runs[run.ID]
	if !ok {
		return Accepted{Accepted: false, Message: "run not found"}
	}

	if fakeRun.approval == nil {
		return Accepted{Accepted: false, Message: "no approval pending"}
	}

	fakeRun.approved = &approved

	return Accepted{Accepted: true, Message: "decision recorded"}
}

// Recover returns recovery state for a session.
func (a *FakeAdapter) Recover(ctx context.Context, session *domain.Session) RecoveryResult {
	// Fake adapter can always recover
	return RecoveryResult{
		CanRecover: true,
		State:      domain.SessionStateResumable,
		Metadata: map[string]string{
			"recovered_at": time.Now().Format(time.RFC3339),
			"adapter":      a.ID(),
		},
	}
}

// simulateExecution runs the fake scenario.
func (a *FakeAdapter) simulateExecution(ctx context.Context, run *fakeRun) {
	defer func() {
		a.mu.Lock()
		if ch, ok := a.signals[run.handle.ID]; ok {
			close(ch)
		}
		a.mu.Unlock()
	}()

	// Emit run.started signal
	select {
	case <-ctx.Done():
		return
	case <-time.After(a.scenario.StartupDelay):
		a.emitSignal(run.handle.ID, SignalStatusChange, StatusChangePayload{
			OldState: domain.RunStatePending,
			NewState: domain.RunStateActive,
		})
	}

	// Emit output lines
	for i, line := range a.scenario.OutputLines {
		select {
		case <-ctx.Done():
			return
		case <-time.After(a.scenario.OutputDelay):
			a.emitSignal(run.handle.ID, SignalOutput, OutputPayload{
				Content: line,
				Stream:  "stdout",
			})
			run.outputIdx = i + 1

			// Check if we should request approval after this line
			if a.scenario.RequestApproval != nil && i+1 == a.scenario.RequestApproval.AfterOutput {
				a.requestApproval(run)

				// Wait for an approval decision, polling with a timeout.
				deadline := time.Now().Add(30 * time.Second)
				decided := false
				for time.Now().Before(deadline) {
					select {
					case <-ctx.Done():
						return
					case <-time.After(10 * time.Millisecond):
					}
					a.mu.RLock()
					approved := run.approved
					a.mu.RUnlock()
					if approved != nil {
						decided = true
						break
					}
				}
				if !decided {
					return
				}

				// Emit approval resolved
				a.emitSignal(run.handle.ID, SignalStatusChange, StatusChangePayload{
					OldState: "waiting_approval",
					NewState: domain.RunStateActive,
					Reason:   "approval decision received",
				})

				// Wait before exiting
				<-time.After(a.scenario.ExitAfterApproval)
			}
		}
	}

	// Emit completion
	a.emitSignal(run.handle.ID, SignalCompletion, CompletionPayload{
		ExitCode:   a.scenario.ExitCode,
		DurationMs: time.Since(run.startedAt).Milliseconds(),
	})

	run.state = domain.RunStateCompleted
}

// emitSignal sends a signal to the run's channel.
func (a *FakeAdapter) emitSignal(runID string, signalType AdapterSignalType, payload any) {
	a.mu.RLock()
	ch, ok := a.signals[runID]
	a.mu.RUnlock()

	if !ok {
		return
	}

	payloadJSON, _ := json.Marshal(payload)

	select {
	case ch <- AdapterSignal{
		Type:      signalType,
		Timestamp: time.Now(),
		Payload:   payloadJSON,
	}:
	default:
		// Channel full, drop signal
	}
}

// requestApproval emits an approval request signal.
func (a *FakeAdapter) requestApproval(run *fakeRun) {
	req := a.scenario.RequestApproval
	a.emitSignal(run.handle.ID, SignalApprovalRequest, ApprovalRequestPayload{
		ApprovalID:           "approval-" + run.handle.ID,
		Category:             req.Category,
		ActionKind:           req.ActionKind,
		HumanReadableContext: req.HumanReadableContext,
		StructuredPayload:    req.StructuredPayload,
		ExpiresIn:            5 * time.Minute,
	})

	// Emit status change to waiting_approval
	a.emitSignal(run.handle.ID, SignalStatusChange, StatusChangePayload{
		OldState: domain.RunStateActive,
		NewState: domain.SessionStateWaitingApprov,
		Reason:   "approval requested",
	})
}

// GetRunState returns the current state of a run (for testing).
func (a *FakeAdapter) GetRunState(runID string) string {
	a.mu.RLock()
	defer a.mu.RUnlock()

	run, ok := a.runs[runID]
	if !ok {
		return ""
	}
	return run.state
}

// GetRun returns a run handle (for testing).
func (a *FakeAdapter) GetRun(runID string) *RunHandle {
	a.mu.RLock()
	defer a.mu.RUnlock()

	run, ok := a.runs[runID]
	if !ok {
		return nil
	}
	return run.handle
}
