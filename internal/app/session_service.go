package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/code-all-remote/car/internal/adapter"
	"github.com/code-all-remote/car/internal/approval"
	"github.com/code-all-remote/car/internal/audit"
	"github.com/code-all-remote/car/internal/domain"
	"github.com/code-all-remote/car/internal/session"
)

// Sentinel errors returned by the service layer.
var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
	ErrInvalid  = errors.New("invalid")
)

// CreateSessionRequest is the input for creating a session.
type CreateSessionRequest struct {
	WorkspaceID string `json:"workspace_id"`
	AdapterID   string `json:"adapter_id"`
	Title       string `json:"title,omitempty"`
}

// CreateSession creates a new session without starting a run.
func (a *App) CreateSession(ctx context.Context, actorID string, req CreateSessionRequest) (*domain.Session, error) {
	if req.WorkspaceID == "" {
		return nil, fmt.Errorf("%w: workspace_id is required", ErrInvalid)
	}
	if req.AdapterID == "" {
		return nil, fmt.Errorf("%w: adapter_id is required", ErrInvalid)
	}

	ws, err := a.workspaces.GetByID(req.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("%w: workspace %s", ErrNotFound, req.WorkspaceID)
	}

	ad, ok := a.adapters[req.AdapterID]
	if !ok {
		return nil, fmt.Errorf("%w: adapter %s not available", ErrInvalid, req.AdapterID)
	}
	if v := ad.ValidateWorkspace(ws); !v.Valid {
		return nil, fmt.Errorf("%w: adapter rejected workspace: %v", ErrInvalid, v.Errors)
	}

	s := &domain.Session{
		ID:          newID("ses"),
		WorkspaceID: req.WorkspaceID,
		AdapterID:   req.AdapterID,
		Title:       req.Title,
		State:       domain.SessionStateCreated,
	}
	if err := a.sessions.CreateSession(s); err != nil {
		return nil, fmt.Errorf("creating session: %w", err)
	}

	if err := a.emitEvent(s.ID, "session.created", map[string]any{
		"workspace_id": s.WorkspaceID,
		"adapter_id":   s.AdapterID,
		"title":        s.Title,
	}); err != nil {
		return nil, fmt.Errorf("emitting session.created: %w", err)
	}

	a.audit.Record(audit.Entry{
		ActorID: actorID, ActorType: audit.ActorTypeUser,
		Action: audit.ActionSessionCreate, TargetType: "session", TargetID: s.ID,
		Outcome: audit.OutcomeSuccess,
		Context: map[string]any{"workspace_id": s.WorkspaceID, "adapter_id": s.AdapterID},
	})

	refreshed, _ := a.sessions.GetByID(s.ID)
	if refreshed != nil {
		return refreshed, nil
	}
	return s, nil
}

// StartRun starts (or resumes) a run for a session.
func (a *App) StartRun(ctx context.Context, actorID, sessionID string) (*domain.Run, error) {
	s, err := a.sessions.GetByID(sessionID)
	if err != nil {
		return nil, fmt.Errorf("%w: session %s", ErrNotFound, sessionID)
	}

	if !a.lifecycle.IsAcceptingRuns() {
		return nil, fmt.Errorf("%w: server is shutting down", ErrConflict)
	}

	if !a.stateMachine.CanTransition(s.State, domain.SessionStateStarting) {
		return nil, fmt.Errorf("%w: cannot start run from state %s", ErrConflict, s.State)
	}

	ad, ok := a.adapters[s.AdapterID]
	if !ok {
		return nil, fmt.Errorf("%w: adapter %s not available", ErrInvalid, s.AdapterID)
	}

	ws, _ := a.workspaces.GetByID(s.WorkspaceID)

	if _, err := a.sessions.UpdateState(sessionID, domain.SessionStateStarting, &s.State); err != nil {
		return nil, fmt.Errorf("updating state: %w", err)
	}

	handle, err := ad.Start(ctx, s, adapter.Input{
		WorkspacePath: wsPath(ws),
		Env:           a.adapterEnv[s.AdapterID],
	})
	if err != nil {
		a.sessions.UpdateState(sessionID, domain.SessionStateFailed, strPtr(domain.SessionStateStarting))
		a.emitEvent(sessionID, "run.start_failed", map[string]any{"error": err.Error()})
		return nil, fmt.Errorf("starting adapter: %w", err)
	}

	run := &domain.Run{
		ID:        handle.ID,
		SessionID: sessionID,
		State:     domain.RunStateActive,
	}
	if err := a.sessions.CreateRun(run); err != nil {
		return nil, fmt.Errorf("creating run: %w", err)
	}

	a.lifecycle.RegisterRun(sessionID, run.ID)
	a.sessions.UpdateState(sessionID, domain.SessionStateActive, strPtr(domain.SessionStateStarting))

	if err := a.emitEvent(sessionID, "run.started", map[string]any{
		"run_id": run.ID,
		"pid":    handle.PID,
	}); err != nil {
		return nil, err
	}

	a.audit.Record(audit.Entry{
		ActorID: actorID, ActorType: audit.ActorTypeUser,
		Action: audit.ActionSessionStart, TargetType: "session", TargetID: sessionID,
		Outcome: audit.OutcomeSuccess,
		Context: map[string]any{"run_id": run.ID},
	})

	// Track the observer goroutine so tests can await journal quiescence
	// deterministically. Add before launch so a subsequent Wait cannot race
	// ahead of the increment.
	a.observers.Add(1)
	go a.observeAdapter(ctx, ad, s, run, handle)

	return run, nil
}

// observeAdapter consumes adapter signals and persists domain events, driving
// approvals and lifecycle transitions.
func (a *App) observeAdapter(ctx context.Context, ad adapter.Adapter, s *domain.Session, run *domain.Run, handle *adapter.RunHandle) {
	// Done runs after the signal loop below drains, i.e. after the terminal
	// event has been persisted, giving waitForObservers a precise barrier.
	defer a.observers.Done()
	defer a.lifecycle.CompleteRun(run.ID)

	signals := ad.Observe(ctx, handle)
	for sig := range signals {
		switch sig.Type {
		case adapter.SignalOutput:
			a.emitEvent(s.ID, "run.output", map[string]any{"content": parseContent(sig), "run_id": run.ID})
		case adapter.SignalApprovalRequest:
			a.handleApprovalRequest(s, run, sig)
		case adapter.SignalStatusChange:
			a.handleStatusChange(s, run, sig)
		case adapter.SignalCompletion:
			a.handleCompletion(s, run, sig)
		case adapter.SignalError:
			a.emitEvent(s.ID, "run.error", map[string]any{"message": parseErrMessage(sig), "run_id": run.ID})
		}
	}
}

// SubmitPrompt submits operator input to a session's active run.
func (a *App) SubmitPrompt(ctx context.Context, actorID, sessionID, text string) error {
	if text == "" {
		return fmt.Errorf("%w: text is required", ErrInvalid)
	}

	s, err := a.sessions.GetByID(sessionID)
	if err != nil {
		return fmt.Errorf("%w: session %s", ErrNotFound, sessionID)
	}

	if s.State != domain.SessionStateActive && s.State != domain.SessionStateWaitingApprov {
		return fmt.Errorf("%w: cannot submit prompt in state %s", ErrConflict, s.State)
	}

	ad, ok := a.adapters[s.AdapterID]
	if !ok {
		return fmt.Errorf("%w: adapter not available", ErrInvalid)
	}

	run, err := a.sessions.GetActiveRun(sessionID)
	if err != nil || run == nil {
		return fmt.Errorf("%w: no active run", ErrConflict)
	}

	handle := &adapter.RunHandle{ID: run.ID, SessionID: sessionID}
	accepted := ad.SubmitInput(ctx, handle, text)
	if !accepted.Accepted {
		return fmt.Errorf("%w: adapter rejected prompt: %s", ErrConflict, accepted.Message)
	}

	if err := a.emitEvent(sessionID, "run.prompt", map[string]any{
		"run_id":      run.ID,
		"text_length": len(text),
	}); err != nil {
		return err
	}

	a.audit.Record(audit.Entry{
		ActorID: actorID, ActorType: audit.ActorTypeUser,
		Action: audit.ActionPromptSubmit, TargetType: "session", TargetID: sessionID,
		Outcome: audit.OutcomeSuccess,
		Context: map[string]any{"run_id": run.ID, "text_length": len(text)},
	})

	return nil
}

// Interrupt requests interruption of the active run.
func (a *App) Interrupt(ctx context.Context, actorID, sessionID string) error {
	s, err := a.sessions.GetByID(sessionID)
	if err != nil {
		return fmt.Errorf("%w: session %s", ErrNotFound, sessionID)
	}

	if s.State != domain.SessionStateActive && s.State != domain.SessionStateWaitingApprov {
		return fmt.Errorf("%w: cannot interrupt in state %s", ErrConflict, s.State)
	}

	ad, ok := a.adapters[s.AdapterID]
	if !ok {
		return fmt.Errorf("%w: adapter not available", ErrInvalid)
	}

	run, err := a.sessions.GetActiveRun(sessionID)
	if err != nil || run == nil {
		return fmt.Errorf("%w: no active run", ErrConflict)
	}

	handle := &adapter.RunHandle{ID: run.ID, SessionID: sessionID}
	accepted := ad.Interrupt(ctx, handle)
	if !accepted.Accepted {
		return fmt.Errorf("%w: adapter rejected interrupt: %s", ErrConflict, accepted.Message)
	}

	a.sessions.UpdateState(sessionID, domain.SessionStateInterrupted, &s.State)
	if err := a.emitEvent(sessionID, "run.interrupted", map[string]any{"run_id": run.ID}); err != nil {
		return err
	}

	a.audit.Record(audit.Entry{
		ActorID: actorID, ActorType: audit.ActorTypeUser,
		Action: audit.ActionInterrupt, TargetType: "session", TargetID: sessionID,
		Outcome: audit.OutcomeSuccess,
		Context: map[string]any{"run_id": run.ID},
	})

	return nil
}

// ResolveApproval records an approval decision and notifies the adapter.
func (a *App) ResolveApproval(ctx context.Context, actorID, approvalID, decision, reason string) (*domain.Approval, error) {
	if decision != "approve" && decision != "deny" {
		return nil, fmt.Errorf("%w: decision must be approve or deny", ErrInvalid)
	}
	approved := decision == "approve"

	decisionObj := approval.Decision{
		Approved: approved,
		Reason:   reason,
		ActorID:  actorID,
	}
	result, err := a.approvals.Decide(ctx, approvalID, decisionObj)
	if err != nil {
		if result != nil && result.Error != nil && result.Error.Error() == "approval not found" {
			return nil, fmt.Errorf("%w: approval %s", ErrNotFound, approvalID)
		}
		return nil, fmt.Errorf("%w: %s", ErrConflict, err)
	}

	// Late/duplicate decision (docs/13 §Resolve approval, docs/12 §Failure
	// behavior): return the final state WITHOUT emitting a new resolved event
	// or notifying the adapter a second time.
	if result.AlreadyFinal {
		ap, _ := a.approvals.GetByID(approvalID)
		return ap, nil
	}

	a.emitEvent(result.SessionID, "approval.resolved", map[string]any{
		"approval_id": approvalID,
		"decision":    decision,
	})

	a.audit.Record(audit.Entry{
		ActorID: actorID, ActorType: audit.ActorTypeUser,
		Action: audit.ActionApprovalDecision, TargetType: "approval", TargetID: approvalID,
		Outcome: audit.OutcomeSuccess,
		Context: map[string]any{"decision": decision, "reason": reason},
	})

	a.sessions.SetPendingApproval(result.SessionID, nil)

	// Notify the adapter (once) so its pending run can resume or stop.
	if session, err := a.sessions.GetByID(result.SessionID); err == nil {
		if ad, ok := a.adapters[session.AdapterID]; ok {
			if run, _ := a.sessions.GetActiveRun(result.SessionID); run != nil {
				handle := &adapter.RunHandle{ID: run.ID, SessionID: result.SessionID}
				ad.DecideApproval(ctx, handle, approvalID, approved, reason)
			}
		}
	}

	ap, _ := a.approvals.GetByID(approvalID)
	return ap, nil
}

// handler helpers ------------------------------------------------------------

func (a *App) handleApprovalRequest(s *domain.Session, run *domain.Run, sig adapter.AdapterSignal) {
	var p adapter.ApprovalRequestPayload
	if err := json.Unmarshal(sig.Payload, &p); err != nil {
		return
	}

	approvalReq := approval.Request{
		ID:                   p.ApprovalID,
		SessionID:            s.ID,
		Category:             p.Category,
		ActionKind:           p.ActionKind,
		HumanReadableContext: p.HumanReadableContext,
		StructuredPayload:    p.StructuredPayload,
	}
	if approvalReq.ID == "" {
		approvalReq.ID = newID("apr")
	}

	ap, err := a.approvals.Request(context.Background(), approvalReq)
	if err != nil {
		return
	}

	a.sessions.SetPendingApproval(s.ID, &ap.ID)
	if a.stateMachine.CanTransition(s.State, domain.SessionStateWaitingApprov) {
		a.sessions.UpdateState(s.ID, domain.SessionStateWaitingApprov, &s.State)
	}

	// Reload session to pick up the new state for subsequent transitions.
	refreshed, _ := a.sessions.GetByID(s.ID)
	if refreshed != nil {
		s.State = refreshed.State
	}

	a.emitEvent(s.ID, "approval.requested", map[string]any{
		"approval_id": ap.ID,
		"category":    ap.Category,
		"run_id":      run.ID,
	})
}

func (a *App) handleStatusChange(s *domain.Session, run *domain.Run, sig adapter.AdapterSignal) {
	var p adapter.StatusChangePayload
	if err := json.Unmarshal(sig.Payload, &p); err != nil {
		return
	}
	a.emitEvent(s.ID, "run.status_change", map[string]any{
		"old_state": p.OldState,
		"new_state": p.NewState,
		"reason":    p.Reason,
		"run_id":    run.ID,
	})
}

func (a *App) handleCompletion(s *domain.Session, run *domain.Run, sig adapter.AdapterSignal) {
	var p adapter.CompletionPayload
	if err := json.Unmarshal(sig.Payload, &p); err != nil {
		return
	}

	exitCode := p.ExitCode
	newState := domain.SessionStateCompleted
	if exitCode != 0 {
		newState = domain.SessionStateFailed
	}

	a.sessions.UpdateRunState(run.ID, newState, &exitCode, strPtrOrNil(p.ExitError))
	if a.stateMachine.CanTransition(s.State, newState) {
		a.sessions.UpdateState(s.ID, newState, &s.State)
	}

	a.emitEvent(s.ID, "run.completed", map[string]any{
		"run_id":     run.ID,
		"exit_code":  exitCode,
		"exit_error": p.ExitError,
	})
}

func wsPath(ws *domain.Workspace) string {
	if ws == nil {
		return ""
	}
	return ws.Path
}

func strPtr(s string) *string { return &s }

func strPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func parseContent(sig adapter.AdapterSignal) string {
	var p adapter.OutputPayload
	if err := json.Unmarshal(sig.Payload, &p); err != nil {
		return ""
	}
	return p.Content
}

func parseErrMessage(sig adapter.AdapterSignal) string {
	var p adapter.ErrorPayload
	if err := json.Unmarshal(sig.Payload, &p); err != nil {
		return ""
	}
	return p.Message
}

// keep import for state machine usage in package.
var _ = session.NewStateMachine
