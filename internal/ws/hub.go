// Package ws implements the WebSocket gateway for CAR live session events.
//
// The gateway is an optimization for live use; the durable source of truth
// remains the event journal, recoverable through REST replay. A client
// connects to /api/v1/ws, sends a hello envelope with its last-known cursors,
// and the server replays available events then switches to live delivery.
// Slow clients are disconnected with a resumable cursor rather than causing
// unbounded memory growth.
package ws

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/code-all-remote/car/internal/domain"
	"github.com/code-all-remote/car/internal/storage"
)

// Envelope is the on-the-wire shape of every WebSocket message.
type Envelope struct {
	Type       string         `json:"type"`
	MessageID  string         `json:"message_id,omitempty"`
	OccurredAt time.Time      `json:"occurred_at,omitempty"`
	SessionID  string         `json:"session_id,omitempty"`
	Sequence   int64          `json:"sequence,omitempty"`
	Payload    map[string]any `json:"payload,omitempty"`
}

// Hello is the client's opening message after the socket is established.
type Hello struct {
	Type            string   `json:"type"` // must be "hello"
	ProtocolVersion int      `json:"protocol_version"`
	DeviceID        string   `json:"device_id"`
	Cursors         []Cursor `json:"cursors"`
}

// Cursor is a per-session resume point.
type Cursor struct {
	SessionID string `json:"session_id"`
	After     int64  `json:"after"`
}

// Welcome is the server's reply to a hello.
type Welcome struct {
	Type            string `json:"type"` // "welcome"
	ProtocolVersion int    `json:"protocol_version"`
	ServerTime      string `json:"server_time"`
}

// ResyncRequired is emitted when a cursor is older than retention.
type ResyncRequired struct {
	Type      string `json:"type"` // "resync_required"
	SessionID string `json:"session_id"`
	After     int64  `json:"after"`
}

// EventJournal is the interface the hub uses to replay and subscribe.
type EventJournal interface {
	Replay(sessionID string, after int64, limit int) (*storage.CursorResult, error)
}

// Hub fans out live events to subscribed clients and serves replay requests.
type Hub struct {
	mu            sync.RWMutex
	subsBySession map[string]map[*client]struct{}
	journal       EventJournal
	logger        *slog.Logger
	// replayLimit bounds replay page sizes.
	replayLimit int
	// sendBuffer is the per-client outbound buffer; a client exceeding it is
	// disconnected with a resumable cursor (backpressure).
	sendBuffer int
}

// NewHub creates a new hub.
func NewHub(journal EventJournal, logger *slog.Logger) *Hub {
	return &Hub{
		subsBySession: make(map[string]map[*client]struct{}),
		journal:       journal,
		logger:        logger,
		replayLimit:   100,
		sendBuffer:    256,
	}
}

// Publish fans a live domain event out to all subscribers of its session.
// Coalescing of adjacent run.output chunks is intentionally NOT performed
// here; the spec requires byte-order preservation, so each event is delivered
// as-is. Callers MAY batch at the source if they preserve order.
func (h *Hub) Publish(ev domain.Event) {
	env := Envelope{
		Type:       ev.Type,
		MessageID:  ev.MessageID,
		OccurredAt: ev.OccurredAt,
		SessionID:  ev.SessionID,
		Sequence:   ev.Sequence,
		Payload:    ev.Payload,
	}

	h.mu.RLock()
	subs := h.subsBySession[ev.SessionID]
	// Snapshot the subscriber set so we can deliver without holding the lock.
	clients := make([]*client, 0, len(subs))
	for c := range subs {
		clients = append(clients, c)
	}
	h.mu.RUnlock()

	for _, c := range clients {
		c.send(env)
	}
}

// subscribe registers a client for live events on a session.
func (h *Hub) subscribe(c *client, sessionID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subsBySession[sessionID] == nil {
		h.subsBySession[sessionID] = make(map[*client]struct{})
	}
	h.subsBySession[sessionID][c] = struct{}{}
}

// unsubscribe removes a client from all session subscriptions.
func (h *Hub) unsubscribe(c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for sid, subs := range h.subsBySession {
		delete(subs, c)
		if len(subs) == 0 {
			delete(h.subsBySession, sid)
		}
	}
}

// unsubscribeSession removes a client from a single session's subscribers.
func (h *Hub) unsubscribeSession(c *client, sessionID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if clients, ok := h.subsBySession[sessionID]; ok {
		delete(clients, c)
		if len(clients) == 0 {
			delete(h.subsBySession, sessionID)
		}
	}
}

// replayAndSubscribe delivers retained events after each cursor and switches the
// client to live delivery without an event-loss window. It subscribes to all
// requested sessions FIRST (so live events published during replay are captured
// in the client's buffer), replays retained events directly, then flushes the
// buffered live events after them. The Android client applies events only when
// contiguous (advanceIfContiguous), so this ordering — retained [after+1..N]
// then buffered [N+1..] — is required; duplicates are harmless. Returns a
// resync signal per expired cursor (which is not subscribed for live delivery).
func (h *Hub) replayAndSubscribe(ctx context.Context, c *client, cursors []Cursor) []ResyncRequired {
	var resyncs []ResyncRequired

	c.startBuffering()

	// Subscribe to ALL requested sessions first so live events published during
	// replay are captured (Publish routes only to subscribers).
	for _, cur := range cursors {
		h.subscribe(c, cur.SessionID)
	}

	for _, cur := range cursors {
		result, err := h.journal.Replay(cur.SessionID, cur.After, h.replayLimit)
		if err != nil {
			h.logger.Warn("ws replay error", "session_id", cur.SessionID, "error", err)
			h.unsubscribeSession(c, cur.SessionID)
			continue
		}

		if result.ResyncRequired {
			resyncs = append(resyncs, ResyncRequired{
				Type: "resync_required", SessionID: cur.SessionID, After: cur.After,
			})
			// The client must resync via REST first; do not serve live events.
			h.unsubscribeSession(c, cur.SessionID)
			continue
		}

		// Retained events go directly to sendCh, bypassing the buffer, so they
		// land before any buffered live event.
		for _, ev := range result.Events {
			c.sendDirect(Envelope{
				Type:       ev.Type,
				MessageID:  ev.MessageID,
				OccurredAt: ev.OccurredAt,
				SessionID:  ev.SessionID,
				Sequence:   ev.Sequence,
				Payload:    ev.Payload,
			})
		}
	}

	// Deliver the live events captured during the replay window, in order, then
	// go live.
	c.flushAndStopBuffering()

	return resyncs
}

// SubscribeSession subscribes a client to a session (used by the handler after
// replay). Exposed for tests.
func (h *Hub) SubscribeSession(c *client, sessionID string) {
	h.subscribe(c, sessionID)
}

// SubscriberCount returns the number of subscribers on a session (for tests).
func (h *Hub) SubscriberCount(sessionID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subsBySession[sessionID])
}

// decodeHello decodes and validates a hello envelope.
func decodeHello(data []byte) (Hello, error) {
	var h Hello
	if err := json.Unmarshal(data, &h); err != nil {
		return h, err
	}
	if h.Type != "hello" {
		return h, &ProtocolError{Message: "expected hello envelope, got " + h.Type}
	}
	if h.ProtocolVersion != 1 {
		return h, &ProtocolError{Message: "unsupported protocol version"}
	}
	if h.DeviceID == "" {
		return h, &ProtocolError{Message: "device_id is required"}
	}
	return h, nil
}

// ProtocolError is a recoverable protocol violation reported to the client.
type ProtocolError struct {
	Message string
}

func (e *ProtocolError) Error() string { return e.Message }
