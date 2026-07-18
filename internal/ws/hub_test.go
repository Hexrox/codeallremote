package ws

import (
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/code-all-remote/car/internal/domain"
	"github.com/code-all-remote/car/internal/storage"
)

type fakeJournal struct {
	mu        sync.Mutex
	events    map[string][]domain.Event
	retention map[string]bool // sessionID -> expired cursor
}

func newFakeJournal() *fakeJournal {
	return &fakeJournal{
		events:    make(map[string][]domain.Event),
		retention: make(map[string]bool),
	}
}

func (f *fakeJournal) add(sessionID string, ev domain.Event) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ev.SessionID = sessionID
	f.events[sessionID] = append(f.events[sessionID], ev)
}

func (f *fakeJournal) markExpired(sessionID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.retention[sessionID] = true
}

func (f *fakeJournal) Replay(sessionID string, after int64, limit int) (*storage.CursorResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.retention[sessionID] {
		return &storage.CursorResult{
			Events:         []domain.Event{},
			NextAfter:      after,
			ResyncRequired: true,
		}, nil
	}

	all := f.events[sessionID]
	var matched []domain.Event
	for _, e := range all {
		if e.Sequence > after {
			matched = append(matched, e)
		}
	}

	nextAfter := after
	if len(matched) > 0 {
		nextAfter = matched[len(matched)-1].Sequence
	}

	hasMore := false
	if len(matched) > limit {
		matched = matched[:limit]
		hasMore = true
	}

	return &storage.CursorResult{
		Events:         matched,
		NextAfter:      nextAfter,
		ResyncRequired: false,
		HasMore:        hasMore,
	}, nil
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestHub_Publish_DeliversToSubscriber(t *testing.T) {
	journal := newFakeJournal()
	hub := NewHub(journal, testLogger())

	c := newClient("cli-1", "dev-1", 64, time.Second, nil)
	received := make(chan Envelope, 10)
	c.writeFunc = func(env Envelope) error {
		received <- env
		return nil
	}
	go c.runWriter()
	defer c.close()

	hub.subscribe(c, "ses-1")

	hub.Publish(domain.Event{
		Type: "run.output", MessageID: "m1", SessionID: "ses-1",
		Sequence: 1, Payload: map[string]any{"content": "hello"},
	})

	select {
	case env := <-received:
		if env.Type != "run.output" {
			t.Errorf("expected run.output, got %s", env.Type)
		}
		if env.Sequence != 1 {
			t.Errorf("expected sequence 1, got %d", env.Sequence)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestHub_Publish_NoSubscribers(t *testing.T) {
	journal := newFakeJournal()
	hub := NewHub(journal, testLogger())

	// Should not panic with no subscribers.
	hub.Publish(domain.Event{Type: "run.output", SessionID: "ses-1", Sequence: 1})
}

func TestHub_ReplayAndSubscribe(t *testing.T) {
	journal := newFakeJournal()
	journal.add("ses-1", domain.Event{Type: "a", MessageID: "m1", Sequence: 1})
	journal.add("ses-1", domain.Event{Type: "b", MessageID: "m2", Sequence: 2})
	journal.add("ses-1", domain.Event{Type: "c", MessageID: "m3", Sequence: 3})

	hub := NewHub(journal, testLogger())

	c := newClient("cli-1", "dev-1", 64, time.Second, nil)
	received := make(chan Envelope, 10)
	c.writeFunc = func(env Envelope) error {
		received <- env
		return nil
	}
	go c.runWriter()
	defer c.close()

	resyncs := hub.replayAndSubscribe(nil, c, []Cursor{{SessionID: "ses-1", After: 1}})

	if len(resyncs) != 0 {
		t.Errorf("expected 0 resyncs, got %d", len(resyncs))
	}

	// Should receive events 2 and 3 (after cursor 1).
	count := 0
	for {
		select {
		case env := <-received:
			if env.Sequence != 2 && env.Sequence != 3 {
				t.Errorf("unexpected sequence %d", env.Sequence)
			}
			count++
			if count == 2 {
				return
			}
		case <-time.After(time.Second):
			t.Fatalf("timeout, received %d events", count)
		}
	}
}

func TestHub_Replay_ExpiredCursor(t *testing.T) {
	journal := newFakeJournal()
	journal.markExpired("ses-1")

	hub := NewHub(journal, testLogger())

	c := newClient("cli-1", "dev-1", 64, time.Second, nil)
	go c.runWriter()
	defer c.close()

	resyncs := hub.replayAndSubscribe(nil, c, []Cursor{{SessionID: "ses-1", After: 5}})

	if len(resyncs) != 1 {
		t.Fatalf("expected 1 resync, got %d", len(resyncs))
	}
	if resyncs[0].SessionID != "ses-1" {
		t.Errorf("expected session ses-1, got %s", resyncs[0].SessionID)
	}

	// Should NOT be subscribed (client must resync via REST first).
	if hub.SubscriberCount("ses-1") != 0 {
		t.Error("expected client not to be subscribed after resync")
	}
}

func TestHub_MultipleSubscribers(t *testing.T) {
	journal := newFakeJournal()
	hub := NewHub(journal, testLogger())

	c1 := newClient("cli-1", "dev-1", 64, time.Second, nil)
	c2 := newClient("cli-2", "dev-2", 64, time.Second, nil)
	r1 := make(chan Envelope, 10)
	r2 := make(chan Envelope, 10)
	c1.writeFunc = func(e Envelope) error { r1 <- e; return nil }
	c2.writeFunc = func(e Envelope) error { r2 <- e; return nil }
	go c1.runWriter()
	go c2.runWriter()
	defer c1.close()
	defer c2.close()

	hub.subscribe(c1, "ses-1")
	hub.subscribe(c2, "ses-1")

	if hub.SubscriberCount("ses-1") != 2 {
		t.Errorf("expected 2 subscribers, got %d", hub.SubscriberCount("ses-1"))
	}

	hub.Publish(domain.Event{Type: "run.output", SessionID: "ses-1", Sequence: 1})

	if env := <-r1; env.Sequence != 1 {
		t.Errorf("c1 got wrong sequence: %d", env.Sequence)
	}
	if env := <-r2; env.Sequence != 1 {
		t.Errorf("c2 got wrong sequence: %d", env.Sequence)
	}
}

func TestHub_Unsubscribe(t *testing.T) {
	journal := newFakeJournal()
	hub := NewHub(journal, testLogger())

	c := newClient("cli-1", "dev-1", 64, time.Second, nil)
	go c.runWriter()

	hub.subscribe(c, "ses-1")
	if hub.SubscriberCount("ses-1") != 1 {
		t.Error("expected 1 subscriber")
	}

	hub.unsubscribe(c)
	if hub.SubscriberCount("ses-1") != 0 {
		t.Error("expected 0 subscribers after unsubscribe")
	}
}

func TestHub_Publish_AfterUnsubscribe(t *testing.T) {
	journal := newFakeJournal()
	hub := NewHub(journal, testLogger())

	c := newClient("cli-1", "dev-1", 64, time.Second, nil)
	received := make(chan Envelope, 10)
	c.writeFunc = func(e Envelope) error { received <- e; return nil }
	go c.runWriter()

	hub.subscribe(c, "ses-1")
	hub.unsubscribe(c)

	hub.Publish(domain.Event{Type: "run.output", SessionID: "ses-1", Sequence: 1})

	select {
	case <-received:
		t.Error("should not receive events after unsubscribe")
	case <-time.After(100 * time.Millisecond):
		// expected
	}
}

func TestClient_BackpressureDisconnect(t *testing.T) {
	journal := newFakeJournal()
	hub := NewHub(journal, testLogger())
	// Small buffer to trigger backpressure quickly.
	hub.sendBuffer = 2

	c := newClient("cli-1", "dev-1", hub.sendBuffer, 100*time.Millisecond, nil)
	// writeFunc blocks forever so the buffer fills.
	c.writeFunc = func(e Envelope) error {
		<-c.closed
		return nil
	}
	go c.runWriter()
	defer c.close()

	hub.subscribe(c, "ses-1")

	// Publish more than the buffer can hold.
	for i := int64(0); i < 20; i++ {
		hub.Publish(domain.Event{
			Type: "run.output", SessionID: "ses-1", Sequence: i,
		})
	}

	// Give the backpressure path a moment to fire.
	deadline := time.After(time.Second)
	for {
		select {
		case <-c.closed:
			if !c.IsSlow() {
				t.Error("expected client to be marked slow")
			}
			return
		case <-deadline:
			t.Fatal("expected client to be closed by backpressure")
		}
	}
}

func TestDecodeHello_Valid(t *testing.T) {
	data := []byte(`{"type":"hello","protocol_version":1,"device_id":"dev_01","cursors":[{"session_id":"ses_01","after":39}]}`)
	h, err := decodeHello(data)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if h.DeviceID != "dev_01" {
		t.Errorf("expected device dev_01, got %s", h.DeviceID)
	}
	if len(h.Cursors) != 1 {
		t.Fatalf("expected 1 cursor, got %d", len(h.Cursors))
	}
	if h.Cursors[0].SessionID != "ses_01" || h.Cursors[0].After != 39 {
		t.Errorf("bad cursor: %+v", h.Cursors[0])
	}
}

func TestDecodeHello_WrongType(t *testing.T) {
	data := []byte(`{"type":"goodbye","protocol_version":1,"device_id":"dev_01"}`)
	_, err := decodeHello(data)
	if err == nil {
		t.Error("expected error for wrong type")
	}
}

func TestDecodeHello_BadVersion(t *testing.T) {
	data := []byte(`{"type":"hello","protocol_version":2,"device_id":"dev_01"}`)
	_, err := decodeHello(data)
	if err == nil {
		t.Error("expected error for bad version")
	}
}

func TestDecodeHello_MissingDeviceID(t *testing.T) {
	data := []byte(`{"type":"hello","protocol_version":1}`)
	_, err := decodeHello(data)
	if err == nil {
		t.Error("expected error for missing device_id")
	}
}

func TestDecodeHello_InvalidJSON(t *testing.T) {
	_, err := decodeHello([]byte(`{bad json`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestClient_CloseIdempotent(t *testing.T) {
	c := newClient("cli-1", "dev-1", 4, time.Second, nil)
	c.close()
	c.close() // should not panic
}
