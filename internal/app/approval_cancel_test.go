package app

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/code-all-remote/car/internal/adapter"
	"github.com/code-all-remote/car/internal/approval"
	"github.com/code-all-remote/car/internal/domain"
)

func TestHandleCompletion_CancelsPendingApprovals(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()

	a.RegisterAdapter(adapter.NewFakeAdapter())

	s, err := a.CreateSession(ctx, "owner", CreateSessionRequest{
		WorkspaceID: "ws-1",
		AdapterID:   "fake-adapter",
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	ap, err := a.approvals.Request(ctx, approval.Request{
		SessionID:            s.ID,
		Category:             "file_write",
		ActionKind:           "write",
		HumanReadableContext: "write config.txt",
		StructuredPayload:    map[string]any{"path": "config.txt"},
	})
	if err != nil {
		t.Fatalf("approvals.Request: %v", err)
	}
	if ap.State != domain.ApprovalStatePending {
		t.Fatalf("want pending approval, got %s", ap.State)
	}

	payload, _ := json.Marshal(adapter.CompletionPayload{ExitCode: 0})
	run := &domain.Run{ID: "run-test", SessionID: s.ID}
	a.handleCompletion(s, run, adapter.AdapterSignal{
		Type:    adapter.SignalCompletion,
		Payload: payload,
	})

	got, err := a.approvals.GetByID(ap.ID)
	if err != nil {
		t.Fatalf("approvals.GetByID: %v", err)
	}
	if got.State != domain.ApprovalStateCancelled {
		t.Fatalf("want cancelled, got %s", got.State)
	}
	if len(a.approvals.GetPendingBySession(s.ID)) != 0 {
		t.Fatalf("want no pending approvals, got %d", len(a.approvals.GetPendingBySession(s.ID)))
	}
}
