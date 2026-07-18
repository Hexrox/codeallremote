package adapter

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/code-all-remote/car/internal/domain"
)

func TestFakeAdapter_Start(t *testing.T) {
	adapter := NewFakeAdapter()

	session := &domain.Session{
		ID:          "session-1",
		WorkspaceID: "ws-1",
		AdapterID:   "fake-adapter",
		State:       domain.SessionStateCreated,
	}

	input := Input{
		WorkspacePath: "/tmp/test",
	}

	handle, err := adapter.Start(context.Background(), session, input)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if handle.SessionID != session.ID {
		t.Errorf("expected session_id %s, got %s", session.ID, handle.SessionID)
	}
	if handle.PID != 12345 {
		t.Errorf("expected PID 12345, got %d", handle.PID)
	}
}

func TestFakeAdapter_Start_FailOnStart(t *testing.T) {
	adapter := NewFakeAdapter().WithScenario(FakeScenario{
		FailOnStart:   true,
		FailWithError: "simulated error",
	})

	session := &domain.Session{
		ID: "session-1", WorkspaceID: "ws-1", AdapterID: "fake-adapter",
	}

	_, err := adapter.Start(context.Background(), session, Input{WorkspacePath: "/tmp"})
	if err == nil {
		t.Error("expected error, got nil")
	}
	if err.Error() != "simulated startup failure: simulated error" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestFakeAdapter_Observe(t *testing.T) {
	adapter := NewFakeAdapter().WithScenario(FakeScenario{
		StartupDelay: 10 * time.Millisecond,
		OutputDelay:  10 * time.Millisecond,
		OutputLines:  []string{"line1", "line2"},
		ExitCode:     0,
	})

	session := &domain.Session{ID: "s1", WorkspaceID: "ws-1", AdapterID: "fake-adapter"}
	handle, _ := adapter.Start(context.Background(), session, Input{WorkspacePath: "/tmp"})

	ctx := context.Background()
	signals := adapter.Observe(ctx, handle)

	var received []AdapterSignal
	timeout := time.After(500 * time.Millisecond)

	for {
		select {
		case signal, ok := <-signals:
			if !ok {
				// Channel closed
				goto done
			}
			received = append(received, signal)
		case <-timeout:
			t.Fatal("timeout waiting for signals")
		}
	}

done:
	// Verify we got signals
	if len(received) < 3 {
		t.Fatalf("expected at least 3 signals (started + 2 outputs + completion), got %d", len(received))
	}

	// First signal should be status_change to active
	if received[0].Type != SignalStatusChange {
		t.Errorf("expected first signal to be status_change, got %s", received[0].Type)
	}

	// Last signal should be completion
	last := received[len(received)-1]
	if last.Type != SignalCompletion {
		t.Errorf("expected last signal to be completion, got %s", last.Type)
	}
}

func TestFakeAdapter_WithApproval(t *testing.T) {
	scenario := FakeScenario{
		StartupDelay: 10 * time.Millisecond,
		OutputDelay:  10 * time.Millisecond,
		OutputLines:  []string{"line1", "line2", "line3"},
		RequestApproval: &ApprovalRequest{
			Category:             "file_write",
			ActionKind:           "write",
			HumanReadableContext: "Agent wants to write to config.txt",
			StructuredPayload:    map[string]any{"path": "/config.txt"},
			AfterOutput:          1,
		},
		ExitAfterApproval: 10 * time.Millisecond,
		ExitCode:          0,
	}

	adapter := NewFakeAdapter().WithScenario(scenario)

	session := &domain.Session{ID: "s1", WorkspaceID: "ws-1", AdapterID: "fake-adapter"}
	handle, _ := adapter.Start(context.Background(), session, Input{WorkspacePath: "/tmp"})

	ctx := context.Background()
	signals := adapter.Observe(ctx, handle)

	var received []AdapterSignal
	timeout := time.After(500 * time.Millisecond)
	approvalReceived := false

	for {
		select {
		case signal, ok := <-signals:
			if !ok {
				goto done
			}
			received = append(received, signal)

			// When we receive approval request, submit decision
			if signal.Type == SignalApprovalRequest {
				approvalReceived = true
				var payload ApprovalRequestPayload
				json.Unmarshal(signal.Payload, &payload)

				// Approve
				adapter.DecideApproval(ctx, handle, payload.ApprovalID, true, "approved for testing")
			}
		case <-timeout:
			t.Fatal("timeout waiting for signals")
		}
	}

done:
	if !approvalReceived {
		t.Error("expected to receive approval request")
	}

	// Verify we got completion
	last := received[len(received)-1]
	if last.Type != SignalCompletion {
		t.Errorf("expected completion signal, got %s", last.Type)
	}
}

func TestFakeAdapter_Interrupt(t *testing.T) {
	adapter := NewFakeAdapter().WithScenario(FakeScenario{
		StartupDelay: 10 * time.Millisecond,
		OutputDelay:  100 * time.Millisecond,
		OutputLines:  []string{"line1", "line2", "line3", "line4", "line5"},
	})

	session := &domain.Session{ID: "s1", WorkspaceID: "ws-1", AdapterID: "fake-adapter"}
	handle, _ := adapter.Start(context.Background(), session, Input{WorkspacePath: "/tmp"})

	ctx := context.Background()
	signals := adapter.Observe(ctx, handle)

	// Read at least one signal
	<-signals

	// Interrupt
	result := adapter.Interrupt(ctx, handle)
	if !result.Accepted {
		t.Errorf("interrupt should be accepted: %s", result.Message)
	}

	// Verify run state
	state := adapter.GetRunState(handle.ID)
	if state != domain.RunStateInterrupted {
		t.Errorf("expected state interrupted, got %s", state)
	}
}

func TestFakeAdapter_Recover(t *testing.T) {
	adapter := NewFakeAdapter()

	session := &domain.Session{
		ID: "s1", WorkspaceID: "ws-1", AdapterID: "fake-adapter",
		State: domain.SessionStateInterrupted,
	}

	result := adapter.Recover(context.Background(), session)
	if !result.CanRecover {
		t.Error("expected fake adapter to support recovery")
	}
	if result.State != domain.SessionStateResumable {
		t.Errorf("expected resumable state, got %s", result.State)
	}
}

func TestFakeAdapter_Capabilities(t *testing.T) {
	adapter := NewFakeAdapter()
	caps := adapter.Capabilities()

	if !caps.SupportsResume {
		t.Error("expected fake adapter to support resume")
	}
	if !caps.SupportsApproval {
		t.Error("expected fake adapter to support approval")
	}
	if !caps.SupportsInterrupt {
		t.Error("expected fake adapter to support interrupt")
	}
	if caps.Version == "" {
		t.Error("expected version to be set")
	}
}

func TestFakeAdapter_ValidateWorkspace(t *testing.T) {
	adapter := NewFakeAdapter()

	ws := &domain.Workspace{
		ID:          "ws-1",
		DisplayName: "Test",
		Path:        "/tmp/test",
	}

	result := adapter.ValidateWorkspace(ws)
	if !result.Valid {
		t.Errorf("expected workspace to be valid: %v", result.Errors)
	}
}

func TestFakeAdapter_SubmitInput(t *testing.T) {
	adapter := NewFakeAdapter()

	session := &domain.Session{ID: "s1", WorkspaceID: "ws-1", AdapterID: "fake-adapter"}
	handle, _ := adapter.Start(context.Background(), session, Input{WorkspacePath: "/tmp"})

	result := adapter.SubmitInput(context.Background(), handle, "test prompt")
	if !result.Accepted {
		t.Error("expected input to be accepted")
	}
}

func TestFakeAdapter_DecideApproval_NoApproval(t *testing.T) {
	adapter := NewFakeAdapter()

	session := &domain.Session{ID: "s1", WorkspaceID: "ws-1", AdapterID: "fake-adapter"}
	handle, _ := adapter.Start(context.Background(), session, Input{WorkspacePath: "/tmp"})

	result := adapter.DecideApproval(context.Background(), handle, "nonexistent", true, "")
	if result.Accepted {
		t.Error("expected decision to not be accepted when no approval pending")
	}
}

func TestFakeAdapter_SignalPayload(t *testing.T) {
	adapter := NewFakeAdapter().WithScenario(FakeScenario{
		StartupDelay: 5 * time.Millisecond,
		OutputDelay:  5 * time.Millisecond,
		OutputLines:  []string{"test output"},
		ExitCode:     0,
	})

	session := &domain.Session{ID: "s1", WorkspaceID: "ws-1", AdapterID: "fake-adapter"}
	handle, _ := adapter.Start(context.Background(), session, Input{WorkspacePath: "/tmp"})

	signals := adapter.Observe(context.Background(), handle)

	for signal := range signals {
		if signal.Type == SignalOutput {
			var payload OutputPayload
			if err := json.Unmarshal(signal.Payload, &payload); err != nil {
				t.Fatalf("failed to unmarshal output payload: %v", err)
			}
			if payload.Content != "test output" {
				t.Errorf("expected content 'test output', got %s", payload.Content)
			}
			if payload.Stream != "stdout" {
				t.Errorf("expected stream stdout, got %s", payload.Stream)
			}
		}
	}
}
