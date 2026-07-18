// Package approval provides approval request handling and decision management.
package approval

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/code-all-remote/car/internal/domain"
)

// Decision represents an approval decision.
type Decision struct {
	Approved bool   `json:"approved"`
	Reason   string `json:"reason,omitempty"`
	ActorID  string `json:"actor_id"`
}

// ApprovalBridge handles approval requests and decisions.
type ApprovalBridge struct {
	mu            sync.RWMutex
	approvals     map[string]*domain.Approval
	decisionCh    chan *DecisionResult
	subscribers   map[string]chan *domain.Approval
	store         ApprovalStore
	clock         Clock
	defaultExpiry time.Duration
}

// DecisionResult is the result of an approval decision.
type DecisionResult struct {
	ApprovalID   string   `json:"approval_id"`
	SessionID    string   `json:"session_id"`
	Decision     Decision `json:"decision"`
	Error        error    `json:"error,omitempty"`
	AlreadyFinal bool     `json:"already_final,omitempty"` // late/duplicate decision
	FinalState   string   `json:"final_state,omitempty"`   // current state when AlreadyFinal
}

// ApprovalStore is the interface for persisting approvals.
type ApprovalStore interface {
	Create(*domain.Approval) error
	GetByID(id string) (*domain.Approval, error)
	Update(*domain.Approval) error
	GetPendingBySession(sessionID string) ([]*domain.Approval, error)
	Delete(id string) error
}

// Clock provides time functions for testing.
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

// RealClock implements Clock with real time.
type RealClock struct{}

func (RealClock) Now() time.Time                         { return time.Now() }
func (RealClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// MockClock implements Clock for testing.
type MockClock struct {
	mu      sync.Mutex
	current time.Time
}

func NewMockClock() *MockClock {
	return &MockClock{current: time.Now()}
}

func (c *MockClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.current
}

func (c *MockClock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.current = t
}

func (c *MockClock) After(d time.Duration) <-chan time.Time {
	// For testing, return a channel that never fires automatically
	// Tests should manually advance time
	return make(chan time.Time)
}

// NewApprovalBridge creates a new approval bridge.
func NewApprovalBridge(store ApprovalStore, clock Clock) *ApprovalBridge {
	if clock == nil {
		clock = RealClock{}
	}

	b := &ApprovalBridge{
		approvals:     make(map[string]*domain.Approval),
		decisionCh:    make(chan *DecisionResult, 100),
		subscribers:   make(map[string]chan *domain.Approval),
		store:         store,
		clock:         clock,
		defaultExpiry: 5 * time.Minute,
	}

	// Start expiry checker
	go b.checkExpiryLoop()

	return b
}

// Request creates a new approval request.
func (b *ApprovalBridge) Request(ctx context.Context, req Request) (*domain.Approval, error) {
	now := b.clock.Now()

	approval := &domain.Approval{
		ID:                   req.ID,
		SessionID:            req.SessionID,
		Category:             req.Category,
		State:                domain.ApprovalStatePending,
		ActionKind:           req.ActionKind,
		HumanReadableContext: req.HumanReadableContext,
		CreatedAt:            now,
		ExpiresAt:            now.Add(b.defaultExpiry),
	}

	if req.StructuredPayload != nil {
		payloadJSON, _ := json.Marshal(req.StructuredPayload)
		approval.StructuredPayload = string(payloadJSON)
	}

	// Store in memory
	b.mu.Lock()
	b.approvals[approval.ID] = approval
	b.mu.Unlock()

	// Persist
	if b.store != nil {
		if err := b.store.Create(approval); err != nil {
			return nil, fmt.Errorf("persisting approval: %w", err)
		}
	}

	// Notify subscribers
	b.notifySubscribers(req.SessionID, approval)

	return approval, nil
}

// Request contains parameters for creating an approval.
type Request struct {
	ID                   string         `json:"id"`
	SessionID            string         `json:"session_id"`
	Category             string         `json:"category"`
	ActionKind           string         `json:"action_kind"`
	HumanReadableContext string         `json:"human_readable_context"`
	StructuredPayload    map[string]any `json:"structured_payload"`
}

// Decide submits a decision for an approval.
//
// The full check-and-mutate runs under the write lock so two concurrent
// Decides cannot both observe Pending and both persist (double-decision),
// and the expiry loop cannot resurrect an approval between the pending check
// and the state write.
//
// A late or duplicate decision (already approved/denied/expired) returns the
// approval's current state and a nil error so callers can surface the final
// state to the client (docs/13 §Resolve approval, docs/12 §Failure behavior).
func (b *ApprovalBridge) Decide(ctx context.Context, approvalID string, decision Decision) (*DecisionResult, error) {
	result := &DecisionResult{
		ApprovalID: approvalID,
		Decision:   decision,
	}

	b.mu.Lock()
	approval, ok := b.approvals[approvalID]
	if !ok {
		// Try to load from store (outside the lock to avoid holding it across DB IO).
		b.mu.Unlock()
		if b.store != nil {
			loaded, err := b.store.GetByID(approvalID)
			if err != nil {
				result.Error = fmt.Errorf("approval not found")
				return result, result.Error
			}
			approval = loaded
		} else {
			result.Error = fmt.Errorf("approval not found")
			return result, result.Error
		}
		b.mu.Lock()
	}

	// Terminal state: return final state WITHOUT altering the adapter twice.
	if approval.State != domain.ApprovalStatePending {
		final := approval
		b.mu.Unlock()
		result.SessionID = final.SessionID
		result.Error = nil
		result.AlreadyFinal = true
		result.FinalState = final.State
		return result, nil
	}

	// Expiry: also terminal; treat as late/impossible to decide.
	now := b.clock.Now()
	if now.After(approval.ExpiresAt) {
		approval.State = domain.ApprovalStateExpired
		expiredAt := now
		approval.DecidedAt = &expiredAt
		b.mu.Unlock()
		if b.store != nil {
			b.store.Update(approval)
		}
		result.SessionID = approval.SessionID
		result.Error = fmt.Errorf("approval expired")
		return result, result.Error
	}

	// Apply decision under the lock (atomic with the pending check above).
	approvedAt := now
	approval.State = domain.ApprovalStateApproved
	if !decision.Approved {
		approval.State = domain.ApprovalStateDenied
	}
	approval.DecidedAt = &approvedAt
	reason := decision.Reason
	approval.DecisionReason = &reason
	b.approvals[approvalID] = approval
	b.mu.Unlock()

	result.SessionID = approval.SessionID

	// Persist (outside the lock).
	if b.store != nil {
		if err := b.store.Update(approval); err != nil {
			result.Error = fmt.Errorf("updating approval: %w", err)
			return result, result.Error
		}
	}

	// Send decision to channel
	select {
	case b.decisionCh <- result:
	default:
		// Channel full, decision still applied
	}

	return result, nil
}

// GetByID returns an approval by ID.
func (b *ApprovalBridge) GetByID(approvalID string) (*domain.Approval, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	approval, ok := b.approvals[approvalID]
	if !ok {
		if b.store != nil {
			return b.store.GetByID(approvalID)
		}
		return nil, fmt.Errorf("approval not found")
	}

	return approval, nil
}

// GetPendingBySession returns pending approvals for a session.
func (b *ApprovalBridge) GetPendingBySession(sessionID string) []*domain.Approval {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var approvals []*domain.Approval
	for _, a := range b.approvals {
		if a.SessionID == sessionID && a.State == domain.ApprovalStatePending {
			approvals = append(approvals, a)
		}
	}

	return approvals
}

// Subscribe creates a channel for approval notifications for a session.
func (b *ApprovalBridge) Subscribe(sessionID string) <-chan *domain.Approval {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan *domain.Approval, 10)
	b.subscribers[sessionID] = ch
	return ch
}

// Unsubscribe removes a subscription.
func (b *ApprovalBridge) Unsubscribe(sessionID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if ch, ok := b.subscribers[sessionID]; ok {
		close(ch)
		delete(b.subscribers, sessionID)
	}
}

// notifySubscribers notifies the subscriber for a session. The write lock is
// held across the send so Unsubscribe cannot close(ch) between the lookup and
// the send (which would panic with send-on-closed-channel).
func (b *ApprovalBridge) notifySubscribers(sessionID string, approval *domain.Approval) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if ch, ok := b.subscribers[sessionID]; ok {
		select {
		case ch <- approval:
		default:
			// Subscriber not reading, skip
		}
	}
}

// checkExpiryLoop periodically checks for expired approvals.
func (b *ApprovalBridge) checkExpiryLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		b.checkExpiry()
	}
}

// checkExpiry marks expired approvals as expired.
func (b *ApprovalBridge) checkExpiry() {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := b.clock.Now()

	for _, a := range b.approvals {
		if a.State == domain.ApprovalStatePending && now.After(a.ExpiresAt) {
			a.State = domain.ApprovalStateExpired
			expiredAt := now
			a.DecidedAt = &expiredAt

			// Persist if store available
			if b.store != nil {
				b.store.Update(a)
			}
		}
	}
}

// DecisionChannel returns the channel for decision results.
func (b *ApprovalBridge) DecisionChannel() <-chan *DecisionResult {
	return b.decisionCh
}

// Cancel cancels a pending approval.
func (b *ApprovalBridge) Cancel(approvalID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	approval, ok := b.approvals[approvalID]
	if !ok {
		return fmt.Errorf("approval not found")
	}

	if approval.State != domain.ApprovalStatePending {
		return fmt.Errorf("approval not pending")
	}

	now := b.clock.Now()
	approval.State = domain.ApprovalStateCancelled
	approval.DecidedAt = &now

	if b.store != nil {
		return b.store.Update(approval)
	}

	return nil
}

// ExtendExpiry extends the expiry time of a pending approval.
func (b *ApprovalBridge) ExtendExpiry(approvalID string, d time.Duration) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	approval, ok := b.approvals[approvalID]
	if !ok {
		return fmt.Errorf("approval not found")
	}

	if approval.State != domain.ApprovalStatePending {
		return fmt.Errorf("approval not pending")
	}

	approval.ExpiresAt = approval.ExpiresAt.Add(d)

	if b.store != nil {
		return b.store.Update(approval)
	}

	return nil
}

// Stats returns statistics about approvals.
func (b *ApprovalBridge) Stats() ApprovalStats {
	b.mu.RLock()
	defer b.mu.RUnlock()

	stats := ApprovalStats{}
	for _, a := range b.approvals {
		switch a.State {
		case domain.ApprovalStatePending:
			stats.Pending++
		case domain.ApprovalStateApproved:
			stats.Approved++
		case domain.ApprovalStateDenied:
			stats.Denied++
		case domain.ApprovalStateExpired:
			stats.Expired++
		case domain.ApprovalStateCancelled:
			stats.Cancelled++
		}
	}
	return stats
}

// ApprovalStats contains approval statistics.
type ApprovalStats struct {
	Pending   int `json:"pending"`
	Approved  int `json:"approved"`
	Denied    int `json:"denied"`
	Expired   int `json:"expired"`
	Cancelled int `json:"cancelled"`
	Total     int `json:"total"`
}
