package mcpperm

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSocket_RoundTrip(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "perm.sock")

	handler := func(req PermissionRequest) Decision {
		switch req.ToolName {
		case "Bash":
			return Decision{Allow: false, Message: "denied"}
		case "Read":
			return Decision{Allow: true}
		default:
			return Decision{Allow: false, Message: "unknown tool"}
		}
	}

	srv, err := NewSocketServer(sockPath, handler)
	if err != nil {
		t.Fatalf("NewSocketServer: %v", err)
	}
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.Serve(ctx) }()

	// Wait for the socket file to appear.
	pollDeadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(pollDeadline) {
		if _, statErr := os.Stat(sockPath); statErr == nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Deny case.
	d := DecideOverSocket(sockPath, PermissionRequest{
		Session:   "s1",
		ToolName:  "Bash",
		ToolUseID: "tu1",
		Input:     json.RawMessage(`{"command":"rm x"}`),
	}, time.Second)
	if d.Allow != false || d.Message != "denied" {
		t.Fatalf("expected deny with message 'denied', got Allow=%v Message=%q", d.Allow, d.Message)
	}

	// Allow case.
	d2 := DecideOverSocket(sockPath, PermissionRequest{
		Session:   "s1",
		ToolName:  "Read",
		ToolUseID: "tu2",
		Input:     json.RawMessage(`{}`),
	}, time.Second)
	if d2.Allow != true {
		t.Fatalf("expected allow, got Allow=%v Message=%q", d2.Allow, d2.Message)
	}

	// Fail-closed: no server listening.
	d3 := DecideOverSocket(filepath.Join(t.TempDir(), "nope.sock"), PermissionRequest{
		ToolName: "Read",
	}, 200*time.Millisecond)
	if d3.Allow != false {
		t.Fatalf("expected fail-closed deny, got Allow=%v Message=%q", d3.Allow, d3.Message)
	}
}
