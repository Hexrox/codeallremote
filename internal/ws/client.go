package ws

import (
	"encoding/json"
	"sync"
	"time"
)

// client wraps a single WebSocket connection. It owns a bounded outbound
// channel; a full channel triggers backpressure: the client is marked slow
// and the connection is closed so the client reconnects with its cursor.
type client struct {
	id        string
	deviceID  string
	sendCh    chan Envelope
	closeOnce sync.Once
	closed    chan struct{}
	slow      bool

	// closeCode is the WebSocket close code to send on disconnect.
	closeCode int

	// writeTimeout caps how long a write may block before the client is
	// considered slow.
	writeTimeout time.Duration

	// onClose is invoked when the client disconnects (to remove subs).
	onClose func(*client)

	// writeFunc writes an envelope to the underlying connection. Set by the
	// handler when it upgrades the socket.
	writeFunc func(Envelope) error

	// bufMu guards the replay buffer. While buffering, live events (via send)
	// accumulate in buffer instead of going to sendCh, so replayAndSubscribe
	// can deliver retained events first and then flush live events captured
	// during the replay window — eliminating the drop window while keeping
	// delivery contiguous and in order.
	bufMu     sync.Mutex
	buffering bool
	buffer    []Envelope
}

// newClient creates a client with a bounded send channel.
func newClient(id, deviceID string, sendBuffer int, writeTimeout time.Duration, onClose func(*client)) *client {
	return &client{
		id:           id,
		deviceID:     deviceID,
		sendCh:       make(chan Envelope, sendBuffer),
		closed:       make(chan struct{}),
		writeTimeout: writeTimeout,
		onClose:      onClose,
		closeCode:    4000, // default normal close
	}
}

// send delivers an envelope to the client, applying backpressure. If the
// outbound buffer is full, the client is marked slow and closed; the caller
// (Hub.Publish) drops the event, and the client reconnects with its cursor.
//
// send is safe to call after close: it checks the closed signal first to
// avoid sending on a closed channel.
func (c *client) send(env Envelope) {
	// Fast path: already closed, drop.
	select {
	case <-c.closed:
		return
	default:
	}
	// While replaying, capture live events in the buffer instead of delivering
	// them out of order ahead of the retained events.
	c.bufMu.Lock()
	if c.buffering {
		c.buffer = append(c.buffer, env)
		c.bufMu.Unlock()
		return
	}
	c.bufMu.Unlock()
	select {
	case c.sendCh <- env:
	case <-c.closed:
	default:
		// Buffer full: trigger backpressure disconnect.
		c.markSlow()
	}
}

// sendDirect delivers an envelope to sendCh, bypassing the replay buffer. Used
// for retained (replay) events, which must land before any buffered live event.
func (c *client) sendDirect(env Envelope) {
	select {
	case <-c.closed:
		return
	default:
	}
	select {
	case c.sendCh <- env:
	case <-c.closed:
	default:
		c.markSlow()
	}
}

// startBuffering makes subsequent send() calls accumulate live events instead
// of delivering them, so replayAndSubscribe controls ordering.
func (c *client) startBuffering() {
	c.bufMu.Lock()
	c.buffering = true
	c.buffer = nil
	c.bufMu.Unlock()
}

// flushAndStopBuffering delivers the buffered live events (in arrival = sequence
// order) to sendCh, then leaves buffering mode so later sends go direct. The
// per-envelope delivery is non-blocking, so holding bufMu during the flush
// cannot deadlock.
func (c *client) flushAndStopBuffering() {
	c.bufMu.Lock()
	defer c.bufMu.Unlock()
	for _, env := range c.buffer {
		select {
		case c.sendCh <- env:
		case <-c.closed:
			c.buffer = nil
			c.buffering = false
			return
		default:
			c.markSlow()
		}
	}
	c.buffer = nil
	c.buffering = false
}

// markSlow flags the client as slow and initiates a clean close with the
// backpressure close code so the client knows to reconnect with its cursor.
func (c *client) markSlow() {
	c.slow = true
	c.closeCode = 4001 // CloseBackpressure
	c.close()
}

// close shuts down the client. Safe to call multiple times.
// It signals closed and unsubscribes (via onClose). sendCh is NOT closed —
// instead senders select on c.closed, which avoids the send-on-closed-channel
// panic that closing sendCh would introduce (the TOCTOU between a sender's
// fast-path check and its send).
func (c *client) close() {
	c.closeOnce.Do(func() {
		close(c.closed)
		if c.onClose != nil {
			c.onClose(c)
		}
		// sendCh is drained by runWriter until it observes c.closed; any
		// buffered-but-unsent events are dropped. We do NOT close sendCh.
	})
}

// runWriter pumps envelopes from the send channel to the underlying writer.
// Blocks until the client is closed or the writer fails.
func (c *client) runWriter() {
	for {
		select {
		case env, ok := <-c.sendCh:
			if !ok {
				return
			}
			if c.writeFunc == nil {
				return
			}
			if err := c.writeFunc(env); err != nil {
				c.close()
				return
			}
		case <-c.closed:
			// Drain remaining buffered events, then exit. sendCh is never
			// closed, so range would block; we exit on closed here.
			return
		}
	}
}

// writeJSON writes a raw envelope using the writeFunc (used for control
// messages like welcome/resync_required).
func (c *client) writeJSON(env Envelope) error {
	if c.writeFunc == nil {
		return nil
	}
	return c.writeFunc(env)
}

// encodeEnvelope marshals an envelope to JSON.
func encodeEnvelope(env Envelope) ([]byte, error) {
	return json.Marshal(env)
}

// IsSlow reports whether this client hit backpressure.
func (c *client) IsSlow() bool {
	return c.slow
}
