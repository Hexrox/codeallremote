package ws

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/code-all-remote/car/internal/domain"
	"github.com/code-all-remote/car/internal/storage"
)

// raceJournal simulates the replay/subscribe race: while Replay is in progress
// (returning retained events 1 and 2), a live event (sequence 3) is published
// to the hub. With the pre-E-3 replay-then-subscribe order, event 3 would be
// dropped (client not yet subscribed). E-3 subscribes first and buffers, so it
// must be delivered contiguously after the retained events.
type raceJournal struct {
	hub *Hub
}

func (j *raceJournal) Replay(sessionID string, after int64, limit int) (*storage.CursorResult, error) {
	j.hub.Publish(domain.Event{
		Type: "test_event", MessageID: "live-3", OccurredAt: time.Now(),
		SessionID: "s1", Sequence: 3, Payload: map[string]any{"seq": 3},
	})
	return &storage.CursorResult{
		ResyncRequired: false,
		Events: []domain.Event{
			{Type: "test_event", MessageID: "ret-1", OccurredAt: time.Now(), SessionID: "s1", Sequence: 1, Payload: map[string]any{"seq": 1}},
			{Type: "test_event", MessageID: "ret-2", OccurredAt: time.Now(), SessionID: "s1", Sequence: 2, Payload: map[string]any{"seq": 2}},
		},
	}, nil
}

func TestHub_ReplayThenLive_NoDropInWindow(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	stub := &raceJournal{}
	hub := NewHub(stub, logger)
	stub.hub = hub

	c := newClient("c1", "dev-1", 16, time.Second, nil)
	defer c.close()

	hub.replayAndSubscribe(context.Background(), c, []Cursor{{SessionID: "s1", After: 0}})

	// Everything was delivered synchronously into sendCh before return; drain it.
	var seqs []int64
drain:
	for {
		select {
		case env := <-c.sendCh:
			seqs = append(seqs, env.Sequence)
		case <-time.After(100 * time.Millisecond):
			break drain
		}
	}

	if len(seqs) != 3 {
		t.Fatalf("expected 3 events (retained 1,2 + live 3, none dropped), got %d: %v", len(seqs), seqs)
	}
	if seqs[0] != 1 || seqs[1] != 2 || seqs[2] != 3 {
		t.Fatalf("expected contiguous order 1,2,3, got %v", seqs)
	}
}
