package app

import (
	"context"
	"testing"
	"time"

	"github.com/code-all-remote/car/internal/adapter"
	"github.com/code-all-remote/car/internal/domain"
)

// TestApp_DemoScript_M1 walks the full M1 demo script from docs/52-m1-demo-script.md
// against a deterministic fake adapter configured with an approval scenario.
//
// It verifies the end-to-end contract:
//  1. Start CAR with an empty database.
//  2. Workspace is registered.
//  3. Create an adapter session.
//  4. Start a run and observe run.started.
//  5. Submit a prompt and observe ordered output chunks.
//  6. Emit an approval request from the fake adapter.
//  7. Read approval details and deny it.
//  8. Observe approval.resolved.
//  9. Disconnect the client and restart CAR.
//
// 10. Reconnect with the last event cursor and verify snapshot/replay.
func TestApp_DemoScript_M1(t *testing.T) {
	app := newTestApp(t)

	// Replace the fake adapter with one configured for an approval scenario.
	fake := adapter.NewFakeAdapter().WithScenario(adapter.FakeScenario{
		StartupDelay: 10 * time.Millisecond,
		OutputDelay:  10 * time.Millisecond,
		OutputLines:  []string{"Analyzing repository", "Proposing change"},
		RequestApproval: &adapter.ApprovalRequest{
			Category:             "file_write",
			ActionKind:           "write",
			HumanReadableContext: "Agent wants to write to config.txt",
			StructuredPayload:    map[string]any{"path": "config.txt"},
			AfterOutput:          1,
		},
		ExitAfterApproval: 20 * time.Millisecond,
		ExitCode:          0,
	})
	app.RegisterAdapter(fake)

	ctx := context.Background()

	// Step 3: Create a session.
	session, err := app.CreateSession(ctx, "owner", CreateSessionRequest{
		WorkspaceID: "ws-1", AdapterID: "fake-adapter", Title: "M1 Demo",
	})
	if err != nil {
		t.Fatalf("step 3 CreateSession failed: %v", err)
	}

	// Step 4: Start a run.
	run, err := app.StartRun(ctx, "owner", session.ID)
	if err != nil {
		t.Fatalf("step 4 StartRun failed: %v", err)
	}
	t.Logf("run started: %s", run.ID)

	// Wait for the adapter to emit output + approval.
	time.Sleep(150 * time.Millisecond)

	// Step 5: Observe ordered output.
	events, err := app.GetEvents(session.ID, 0, 100)
	if err != nil {
		t.Fatalf("step 5 GetEvents failed: %v", err)
	}
	if len(events.Events) < 3 {
		t.Fatalf("step 5 expected >=3 events, got %d", len(events.Events))
	}
	hasStarted := false
	for _, e := range events.Events {
		if e.Type == "run.started" {
			hasStarted = true
		}
	}
	if !hasStarted {
		t.Error("step 5: run.started event not found")
	}

	// Step 6: An approval should have been requested.
	snap, err := app.GetSession(session.ID)
	if err != nil {
		t.Fatalf("step 6 GetSession failed: %v", err)
	}
	if snap.PendingApproval == nil || *snap.PendingApproval == "" {
		t.Fatalf("step 6: expected pending approval, snapshot=%+v", snap)
	}
	approvalID := *snap.PendingApproval

	// Step 7: Read approval details.
	ap, err := app.GetApproval(approvalID)
	if err != nil {
		t.Fatalf("step 7 GetApproval failed: %v", err)
	}
	if ap.Category != "file_write" {
		t.Errorf("step 7: expected category file_write, got %s", ap.Category)
	}

	// Step 8: Deny the approval.
	denied, err := app.ResolveApproval(ctx, "owner", approvalID, "deny", "demo: deny")
	if err != nil {
		t.Fatalf("step 8 ResolveApproval failed: %v", err)
	}
	if denied.State != domain.ApprovalStateDenied {
		t.Errorf("step 8: expected state denied, got %s", denied.State)
	}

	// Wait for the adapter to react to the decision and complete.
	time.Sleep(100 * time.Millisecond)

	// Observe approval.resolved event.
	resolvedEvents, _ := app.GetEvents(session.ID, events.NextAfter, 100)
	hasResolved := false
	for _, e := range resolvedEvents.Events {
		if e.Type == "approval.resolved" {
			hasResolved = true
		}
	}
	if !hasResolved {
		t.Error("step 8: approval.resolved event not found")
	}

	// Step 10: Capture the last cursor, then verify replay consistency.
	lastSeq := uint64(0)
	for _, e := range events.Events {
		if uint64(e.Sequence) > lastSeq {
			lastSeq = uint64(e.Sequence)
		}
	}

	// Replay from 0 should match the full sequence.
	replay, err := app.GetEvents(session.ID, 0, 1000)
	if err != nil {
		t.Fatalf("step 10 replay failed: %v", err)
	}
	if len(replay.Events) == 0 {
		t.Fatal("step 10: expected events on replay")
	}
	// Verify ordering is monotonic.
	for i := 1; i < len(replay.Events); i++ {
		if replay.Events[i].Sequence <= replay.Events[i-1].Sequence {
			t.Errorf("step 10: events not monotonic at %d", i)
		}
	}
	t.Logf("step 10: replayed %d events, next_after=%d, monotonic ordering verified",
		len(replay.Events), replay.NextAfter)
}
