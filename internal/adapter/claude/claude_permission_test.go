package claude

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/code-all-remote/car/internal/adapter"
	"github.com/code-all-remote/car/internal/mcpperm"
)

// TestDecideApproval_ResolvesParkedPermission checks the A-2 increment-3 wiring:
// a permission request parks and emits a SignalApprovalRequest, and
// DecideApproval resolves the parked request (rather than writing stdin).
func TestDecideApproval_ResolvesParkedPermission(t *testing.T) {
	a := New("claude", slog.New(slog.NewTextHandler(io.Discard, nil)))

	r := &claudeRun{
		handle:  &adapter.RunHandle{ID: "run-x", SessionID: "s"},
		signals: make(chan adapter.AdapterSignal, 8),
		done:    make(chan struct{}),
		parked:  map[string]chan mcpperm.Decision{},
	}
	a.mu.Lock()
	a.runs["run-x"] = r
	a.mu.Unlock()

	decCh := make(chan mcpperm.Decision, 1)
	go func() {
		decCh <- a.handlePermission("run-x", mcpperm.PermissionRequest{
			Session:   "s",
			ToolName:  "Bash",
			ToolUseID: "tu1",
			Input:     json.RawMessage(`{"command":"rm x"}`),
		})
	}()

	select {
	case sig := <-r.signals:
		if sig.Type != adapter.SignalApprovalRequest {
			t.Fatalf("expected SignalApprovalRequest, got %v", sig.Type)
		}
		var p adapter.ApprovalRequestPayload
		if err := json.Unmarshal(sig.Payload, &p); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
		if p.ApprovalID != "tu1" || p.ActionKind != "Bash" {
			t.Fatalf("unexpected approval payload: %+v", p)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for approval signal")
	}

	acc := a.DecideApproval(context.Background(), r.handle, "tu1", true, "ok")
	if !acc.Accepted {
		t.Fatalf("expected DecideApproval accepted, got %+v", acc)
	}

	select {
	case d := <-decCh:
		if !d.Allow {
			t.Fatalf("expected Allow=true, got %+v", d)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for handlePermission decision")
	}
}
