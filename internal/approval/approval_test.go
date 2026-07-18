package approval

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/code-all-remote/car/internal/domain"
)

func TestApprovalBridge_Request(t *testing.T) {
	clock := NewMockClock()
	bridge := NewApprovalBridge(nil, clock)
	defer close(bridge.decisionCh)

	ctx := context.Background()
	req := Request{
		ID:                   "approval-1",
		SessionID:            "session-1",
		Category:             "file_write",
		ActionKind:           "write",
		HumanReadableContext: "Agent wants to write to config.txt",
		StructuredPayload:    map[string]any{"path": "/config.txt"},
	}

	approval, err := bridge.Request(ctx, req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if approval.ID != "approval-1" {
		t.Errorf("expected ID approval-1, got %s", approval.ID)
	}
	if approval.SessionID != "session-1" {
		t.Errorf("expected session_id session-1, got %s", approval.SessionID)
	}
	if approval.State != domain.ApprovalStatePending {
		t.Errorf("expected state pending, got %s", approval.State)
	}
	if approval.Category != "file_write" {
		t.Errorf("expected category file_write, got %s", approval.Category)
	}
}

func TestApprovalBridge_Decide_Approve(t *testing.T) {
	clock := NewMockClock()
	bridge := NewApprovalBridge(nil, clock)
	defer close(bridge.decisionCh)

	ctx := context.Background()
	bridge.Request(ctx, Request{
		ID: "approval-1", SessionID: "session-1", Category: "test",
		ActionKind: "exec", HumanReadableContext: "Test approval",
	})

	decision := Decision{
		Approved: true,
		Reason:   "approved for testing",
		ActorID:  "user-1",
	}

	result, err := bridge.Decide(ctx, "approval-1", decision)
	if err != nil {
		t.Fatalf("Decide failed: %v", err)
	}
	if result.ApprovalID != "approval-1" {
		t.Errorf("expected approval_id approval-1, got %s", result.ApprovalID)
	}
	if !result.Decision.Approved {
		t.Error("expected decision to be approved")
	}

	// Verify approval state changed
	approval, _ := bridge.GetByID("approval-1")
	if approval.State != domain.ApprovalStateApproved {
		t.Errorf("expected state approved, got %s", approval.State)
	}
	if approval.DecidedAt == nil {
		t.Error("expected decided_at to be set")
	}
}

func TestApprovalBridge_Decide_Deny(t *testing.T) {
	clock := NewMockClock()
	bridge := NewApprovalBridge(nil, clock)
	defer close(bridge.decisionCh)

	ctx := context.Background()
	bridge.Request(ctx, Request{
		ID: "approval-1", SessionID: "session-1", Category: "test",
		ActionKind: "exec", HumanReadableContext: "Test approval",
	})

	decision := Decision{Approved: false, Reason: "denied for testing", ActorID: "user-1"}

	result, err := bridge.Decide(ctx, "approval-1", decision)
	if err != nil {
		t.Fatalf("Decide failed: %v", err)
	}

	if result.Decision.Approved {
		t.Error("expected decision to be denied")
	}

	approval, _ := bridge.GetByID("approval-1")
	if approval.State != domain.ApprovalStateDenied {
		t.Errorf("expected state denied, got %s", approval.State)
	}
}

func TestApprovalBridge_Decide_NotFound(t *testing.T) {
	clock := NewMockClock()
	bridge := NewApprovalBridge(nil, clock)
	defer close(bridge.decisionCh)

	ctx := context.Background()
	decision := Decision{Approved: true}

	_, err := bridge.Decide(ctx, "nonexistent", decision)
	if err == nil {
		t.Error("expected error for nonexistent approval, got nil")
	}
}

func TestApprovalBridge_Decide_AlreadyDecided(t *testing.T) {
	clock := NewMockClock()
	bridge := NewApprovalBridge(nil, clock)
	defer close(bridge.decisionCh)

	ctx := context.Background()
	bridge.Request(ctx, Request{
		ID: "approval-1", SessionID: "session-1", Category: "test",
		ActionKind: "exec", HumanReadableContext: "Test",
	})

	// First decision
	bridge.Decide(ctx, "approval-1", Decision{Approved: true})

	// Second decision returns the final state, no error, no double-mutation.
	result, err := bridge.Decide(ctx, "approval-1", Decision{Approved: false})
	if err != nil {
		t.Errorf("expected no error on late decision, got %v", err)
	}
	if !result.AlreadyFinal {
		t.Error("expected AlreadyFinal=true on late decision")
	}
	if result.FinalState != domain.ApprovalStateApproved {
		t.Errorf("expected final state approved (first decision wins), got %s", result.FinalState)
	}
	// The first decision must be authoritative (approved stays approved).
	ap, _ := bridge.GetByID("approval-1")
	if ap.State != domain.ApprovalStateApproved {
		t.Errorf("late decision must not alter state, got %s", ap.State)
	}
}

func TestApprovalBridge_Expiry(t *testing.T) {
	clock := NewMockClock()
	bridge := NewApprovalBridge(nil, clock)
	defer close(bridge.decisionCh)

	ctx := context.Background()
	approval, _ := bridge.Request(ctx, Request{
		ID: "approval-1", SessionID: "session-1", Category: "test",
		ActionKind: "exec", HumanReadableContext: "Test",
	})

	// Advance time past expiry (default 5 min)
	clock.Set(clock.Now().Add(6 * time.Minute))

	// Manually trigger expiry check
	bridge.checkExpiry()

	// Check state
	approval, _ = bridge.GetByID("approval-1")
	if approval.State != domain.ApprovalStateExpired {
		t.Errorf("expected state expired, got %s", approval.State)
	}

	// Decision on expired returns the final state (expired), no error.
	result, err := bridge.Decide(ctx, "approval-1", Decision{Approved: true})
	if err != nil {
		t.Errorf("expected no error on expired decision (final state), got %v", err)
	}
	if !result.AlreadyFinal || result.FinalState != domain.ApprovalStateExpired {
		t.Errorf("expected AlreadyFinal expired, got %+v", result)
	}
}

func TestApprovalBridge_Cancel(t *testing.T) {
	clock := NewMockClock()
	bridge := NewApprovalBridge(nil, clock)
	defer close(bridge.decisionCh)

	ctx := context.Background()
	bridge.Request(ctx, Request{
		ID: "approval-1", SessionID: "session-1", Category: "test",
		ActionKind: "exec", HumanReadableContext: "Test",
	})

	err := bridge.Cancel("approval-1")
	if err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}

	approval, _ := bridge.GetByID("approval-1")
	if approval.State != domain.ApprovalStateCancelled {
		t.Errorf("expected state cancelled, got %s", approval.State)
	}
}

func TestApprovalBridge_Cancel_AlreadyDecided(t *testing.T) {
	clock := NewMockClock()
	bridge := NewApprovalBridge(nil, clock)
	defer close(bridge.decisionCh)

	ctx := context.Background()
	bridge.Request(ctx, Request{
		ID: "approval-1", SessionID: "session-1", Category: "test",
		ActionKind: "exec", HumanReadableContext: "Test",
	})
	bridge.Decide(ctx, "approval-1", Decision{Approved: true})

	err := bridge.Cancel("approval-1")
	if err == nil {
		t.Error("expected error for cancelling decided approval, got nil")
	}
}

func TestApprovalBridge_ExtendExpiry(t *testing.T) {
	clock := NewMockClock()
	bridge := NewApprovalBridge(nil, clock)
	defer close(bridge.decisionCh)

	ctx := context.Background()
	approval, _ := bridge.Request(ctx, Request{
		ID: "approval-1", SessionID: "session-1", Category: "test",
		ActionKind: "exec", HumanReadableContext: "Test",
	})

	initialExpiry := approval.ExpiresAt

	// Extend by 10 minutes
	err := bridge.ExtendExpiry("approval-1", 10*time.Minute)
	if err != nil {
		t.Fatalf("ExtendExpiry failed: %v", err)
	}

	approval, _ = bridge.GetByID("approval-1")
	if approval.ExpiresAt.Before(initialExpiry) {
		t.Error("expected expiry to be extended")
	}
}

func TestApprovalBridge_GetPendingBySession(t *testing.T) {
	clock := NewMockClock()
	bridge := NewApprovalBridge(nil, clock)
	defer close(bridge.decisionCh)

	ctx := context.Background()

	// Create multiple approvals for same session
	bridge.Request(ctx, Request{ID: "approval-1", SessionID: "session-1", Category: "test", ActionKind: "exec", HumanReadableContext: "Test 1"})
	bridge.Request(ctx, Request{ID: "approval-2", SessionID: "session-1", Category: "test", ActionKind: "exec", HumanReadableContext: "Test 2"})
	bridge.Request(ctx, Request{ID: "approval-3", SessionID: "session-2", Category: "test", ActionKind: "exec", HumanReadableContext: "Test 3"})

	// Decide one for session-1
	bridge.Decide(ctx, "approval-1", Decision{Approved: true})

	pending := bridge.GetPendingBySession("session-1")
	if len(pending) != 1 {
		t.Errorf("expected 1 pending for session-1, got %d", len(pending))
	}
	if pending[0].ID != "approval-2" {
		t.Errorf("expected approval-2, got %s", pending[0].ID)
	}
}

func TestApprovalBridge_Subscribe(t *testing.T) {
	clock := NewMockClock()
	bridge := NewApprovalBridge(nil, clock)
	defer close(bridge.decisionCh)

	ctx := context.Background()

	ch := bridge.Subscribe("session-1")
	defer bridge.Unsubscribe("session-1")

	// Request should notify subscribers
	go bridge.Request(ctx, Request{
		ID: "approval-1", SessionID: "session-1", Category: "test",
		ActionKind: "exec", HumanReadableContext: "Test",
	})

	// Wait for notification
	select {
	case approval := <-ch:
		if approval.ID != "approval-1" {
			t.Errorf("expected approval-1, got %s", approval.ID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for subscription notification")
	}
}

func TestApprovalBridge_DecisionChannel(t *testing.T) {
	clock := NewMockClock()
	bridge := NewApprovalBridge(nil, clock)
	defer close(bridge.decisionCh)

	ctx := context.Background()
	bridge.Request(ctx, Request{
		ID: "approval-1", SessionID: "session-1", Category: "test",
		ActionKind: "exec", HumanReadableContext: "Test",
	})

	decisionCh := bridge.DecisionChannel()

	go bridge.Decide(ctx, "approval-1", Decision{Approved: true, ActorID: "user-1"})

	select {
	case result := <-decisionCh:
		if result.ApprovalID != "approval-1" {
			t.Errorf("expected approval_id approval-1, got %s", result.ApprovalID)
		}
		if !result.Decision.Approved {
			t.Error("expected approved decision")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for decision result")
	}
}

func TestApprovalBridge_Stats(t *testing.T) {
	clock := NewMockClock()
	bridge := NewApprovalBridge(nil, clock)
	defer close(bridge.decisionCh)

	ctx := context.Background()

	// Create approvals in different states
	bridge.Request(ctx, Request{ID: "p1", SessionID: "s1", Category: "test", ActionKind: "exec", HumanReadableContext: "Test"})
	bridge.Request(ctx, Request{ID: "p2", SessionID: "s1", Category: "test", ActionKind: "exec", HumanReadableContext: "Test"})
	bridge.Decide(ctx, "p1", Decision{Approved: true})
	bridge.Decide(ctx, "p2", Decision{Approved: false})

	stats := bridge.Stats()

	if stats.Pending != 0 {
		t.Errorf("expected 0 pending, got %d", stats.Pending)
	}
	if stats.Approved != 1 {
		t.Errorf("expected 1 approved, got %d", stats.Approved)
	}
	if stats.Denied != 1 {
		t.Errorf("expected 1 denied, got %d", stats.Denied)
	}
}

// Concurrent access test
func TestApprovalBridge_ConcurrentDecisions(t *testing.T) {
	clock := NewMockClock()
	bridge := NewApprovalBridge(nil, clock)
	defer close(bridge.decisionCh)

	ctx := context.Background()

	// Create multiple approvals
	for i := 0; i < 10; i++ {
		id := string(rune('a' + i))
		bridge.Request(ctx, Request{
			ID: "approval-" + id, SessionID: "session-1", Category: "test",
			ActionKind: "exec", HumanReadableContext: "Test",
		})
	}

	var wg sync.WaitGroup
	errors := make(chan error, 10)

	// Make concurrent decisions
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			id := string(rune('a' + idx))
			approved := idx%2 == 0
			_, err := bridge.Decide(ctx, "approval-"+id, Decision{Approved: approved})
			if err != nil && idx < 10 {
				// Only first 10 should succeed (no duplicates)
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Should have no errors for unique approvals
	errCount := 0
	for range errors {
		errCount++
	}
	if errCount > 0 {
		t.Errorf("expected no errors, got %d", errCount)
	}
}

// TestApprovalBridge_ConcurrentDecideSameApproval proves only ONE concurrent
// Decide mutates a single approval; the rest get AlreadyFinal. Regression for
// the Decide TOCTOU that let two decides both persist.
func TestApprovalBridge_ConcurrentDecideSameApproval(t *testing.T) {
	clock := NewMockClock()
	bridge := NewApprovalBridge(nil, clock)
	defer close(bridge.decisionCh)

	ctx := context.Background()
	bridge.Request(ctx, Request{
		ID: "apr-1", SessionID: "session-1", Category: "test",
		ActionKind: "exec", HumanReadableContext: "Test",
	})

	var wg sync.WaitGroup
	results := make(chan *DecisionResult, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, _ := bridge.Decide(ctx, "apr-1", Decision{Approved: true})
			results <- r
		}()
	}
	wg.Wait()
	close(results)

	mutations := 0
	for r := range results {
		if !r.AlreadyFinal {
			mutations++ // the actual decision write
		}
	}
	if mutations != 1 {
		t.Errorf("expected exactly one Decide to mutate, got %d", mutations)
	}
	ap, _ := bridge.GetByID("apr-1")
	if ap.State != domain.ApprovalStateApproved {
		t.Errorf("expected approved state, got %s", ap.State)
	}
}

func TestApprovalBridge_ConcurrentAccess(t *testing.T) {
	clock := NewMockClock()
	bridge := NewApprovalBridge(nil, clock)
	defer close(bridge.decisionCh)

	ctx := context.Background()
	done := make(chan bool, 100)

	// Concurrent reads and writes
	for i := 0; i < 50; i++ {
		go func(id int) {
			bridge.Request(ctx, Request{
				ID: string(rune('a' + (id % 26))), SessionID: "s1",
				Category: "test", ActionKind: "exec", HumanReadableContext: "Test",
			})
			done <- true
		}(i)
	}

	for i := 0; i < 50; i++ {
		go func() {
			bridge.Stats()
			done <- true
		}()
	}

	for i := 0; i < 50; i++ {
		go func() {
			bridge.GetPendingBySession("s1")
			done <- true
		}()
	}

	for i := 0; i < 100; i++ {
		<-done
	}
}

func TestNewMockClock(t *testing.T) {
	clock := NewMockClock()
	if clock.Now().IsZero() {
		t.Error("expected non-zero initial time")
	}

	initial := clock.Now()
	clock.Set(initial.Add(1 * time.Hour))

	if clock.Now().Sub(initial) != 1*time.Hour {
		t.Error("clock.Set did not advance time correctly")
	}
}
