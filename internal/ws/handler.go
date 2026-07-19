package ws

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/code-all-remote/car/internal/domain"
	"github.com/gorilla/websocket"
)

// Authorizer authenticates an incoming WebSocket connection.
// It returns the device ID if the token is valid, or an error otherwise.
type Authorizer interface {
	AuthorizeWS(deviceToken string) (deviceID string, err error)
}

// Handler upgrades HTTP to WebSocket and manages a client connection.
type Handler struct {
	hub          *Hub
	auth         Authorizer
	logger       *slog.Logger
	upgrader     websocket.Upgrader
	nextClient   uint64
	writeTimeout time.Duration
	heartbeat    time.Duration // 0 uses the default; -1 disables the pinger
}

// NewHandler creates a new WebSocket handler.
func NewHandler(hub *Hub, auth Authorizer, logger *slog.Logger) *Handler {
	return &Handler{
		hub:       hub,
		auth:      auth,
		logger:    logger,
		heartbeat: heartbeatInterval,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			// Origin is checked by the gateway/proxy; CAR itself runs in the
			// homelab behind a TLS-terminating reverse proxy.
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		writeTimeout: 2 * time.Second,
	}
}

// ServeHTTP upgrades the connection and runs the client loop.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Authenticate before upgrading. The token may be in the Authorization
	// header or a query param (browsers cannot set headers on WS upgrade).
	token := r.Header.Get("Authorization")
	if len(token) > 7 && token[:7] == "Bearer " {
		token = token[7:]
	}
	if token == "" {
		token = r.URL.Query().Get("token")
	}

	deviceID, err := h.auth.AuthorizeWS(token)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Debug("ws upgrade failed", "error", err)
		return
	}

	clientID := "cli_" + hex.EncodeToString(randBytes(6))
	c := newClient(clientID, deviceID, h.hub.sendBuffer, h.writeTimeout, func(cl *client) {
		h.hub.unsubscribe(cl)
	})
	c.writeFunc = func(env Envelope) error {
		_ = conn.SetWriteDeadline(time.Now().Add(h.writeTimeout))
		data, err := json.Marshal(env)
		if err != nil {
			return err
		}
		return conn.WriteMessage(websocket.TextMessage, data)
	}

	// Always close the underlying connection when the client closes.
	go func() {
		<-c.closed
		conn.Close()
	}()

	atomic.AddUint64(&h.nextClient, 1)

	// Writer pump goroutine.
	go c.runWriter()

	// Reader loop on the main goroutine.
	h.readLoop(r.Context(), conn, c, deviceID, token)
}

// readLoop reads messages from the client. The first message must be hello;
// subsequent messages are acknowledgements (read and discarded). A heartbeat
// (ping/pong) keeps the connection live and detects dead peers.
func (h *Handler) readLoop(ctx context.Context, conn *websocket.Conn, c *client, deviceID string, token string) {
	defer func() {
		// Close the underlying connection first so any blocked read/write on
		// the peer side fails immediately; then attempt a best-effort close
		// frame (ignored if the socket is already gone).
		code := c.closeCode
		if code == 0 {
			code = CloseNormal
		}
		_ = conn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(code, closeReason(code)), time.Now().Add(closeWriteTimeout))
		conn.Close()
		c.close()
	}()

	// Set up pong handler: a pong resets the read deadline (peer is alive).
	hb := h.heartbeat
	conn.SetPongHandler(func(string) error {
		if hb > 0 {
			_ = conn.SetReadDeadline(time.Now().Add(hb + heartbeatGrace))
		}
		return nil
	})

	// Start the heartbeat pinger unless disabled.
	pingCtx, pingCancel := context.WithCancel(ctx)
	defer pingCancel()
	if hb > 0 {
		go h.pinger(pingCtx, conn, c, hb, token)
	}

	// Deadline for the initial hello.
	_ = conn.SetReadDeadline(time.Now().Add(helloTimeout))
	conn.SetReadLimit(1 << 20) // 1 MiB max message

	// First message: hello.
	_, data, err := conn.ReadMessage()
	if err != nil {
		return
	}

	hello, err := decodeHello(data)
	if err != nil {
		// Report a protocol error and close with a documented code.
		h.sendErrorAndClose(conn, CloseProtocolError, err.Error())
		return
	}

	// Send welcome.
	if err := c.writeJSON(Envelope{
		Type:    "welcome",
		Payload: map[string]any{"protocol_version": 1, "server_time": time.Now().UTC().Format(time.RFC3339)},
	}); err != nil {
		return
	}

	// Replay retained events and subscribe for live delivery.
	resyncs := h.hub.replayAndSubscribe(ctx, c, hello.Cursors)
	for _, r := range resyncs {
		c.send(Envelope{
			Type:      r.Type,
			SessionID: r.SessionID,
			Sequence:  r.After,
			Payload:   map[string]any{"session_id": r.SessionID, "after": r.After},
		})
	}

	// Heartbeat deadline: keep rolling while the peer is alive. The reader
	// loop drains acks; a read error (peer gone) closes the socket.
	if hb > 0 {
		_ = conn.SetReadDeadline(time.Now().Add(hb + heartbeatGrace))
	} else {
		_ = conn.SetReadDeadline(time.Time{}) // no heartbeat: no idle deadline
	}
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
		if hb > 0 {
			_ = conn.SetReadDeadline(time.Now().Add(hb + heartbeatGrace))
		}
	}
}

// pinger sends periodic ping control frames. If a write fails the peer is
// gone; the reader will also error and close.
func (h *Handler) pinger(ctx context.Context, conn *websocket.Conn, c *client, interval time.Duration, token string) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Re-check authorization on every heartbeat so a device revoked
			// mid-connection is dropped promptly rather than keeping its live
			// socket until it disconnects on its own.
			if _, err := h.auth.AuthorizeWS(token); err != nil {
				h.sendErrorAndClose(conn, CloseUnauthorized, "unauthorized")
				c.close()
				return
			}
			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(h.writeTimeout)); err != nil {
				c.close()
				return
			}
		case <-c.closed:
			return
		}
	}
}

// sendErrorAndClose writes a protocol error envelope then closes the socket
// with a documented close code.
func (h *Handler) sendErrorAndClose(conn *websocket.Conn, closeCode int, message string) {
	_ = conn.SetWriteDeadline(time.Now().Add(h.writeTimeout))
	errEnv, _ := json.Marshal(Envelope{
		Type:    "error",
		Payload: map[string]any{"message": message},
	})
	_ = conn.WriteMessage(websocket.TextMessage, errEnv)
	_ = conn.WriteControl(websocket.CloseMessage,
		websocket.FormatCloseMessage(closeCode, message), time.Now().Add(h.writeTimeout))
}

// Documented WebSocket close codes used by the gateway. They extend the
// RFC 6455 codes with CAR-specific reasons so a client can branch cleanly.
const (
	CloseNormal         = 4000 // normal close (server shutdown)
	CloseBackpressure   = 4001 // client too slow; reconnect with cursor
	CloseUnauthorized   = 4003 // token invalid/revoked
	CloseProtocolError  = 4002 // bad hello envelope
	CloseResyncRequired = 4004 // cursor past retention; resync via REST
)

// Heartbeat tuning.
const (
	helloTimeout      = 30 * time.Second
	heartbeatInterval = 25 * time.Second
	heartbeatGrace    = 10 * time.Second
	closeWriteTimeout = 1 * time.Second
)

// closeReason returns a stable, secret-free reason string for a close code.
func closeReason(code int) string {
	switch code {
	case CloseBackpressure:
		return "client too slow; reconnect with cursor"
	case CloseUnauthorized:
		return "token invalid or revoked"
	case CloseProtocolError:
		return "protocol error"
	case CloseResyncRequired:
		return "cursor past retention; resync required"
	default:
		return "bye"
	}
}

// PublishAdapter wraps a hub.Publish call for the app to invoke without a
// direct dependency on the ws package's internal client types.
type PublishAdapter struct {
	hub *Hub
}

// NewPublishAdapter returns a PublishAdapter bound to a hub.
func NewPublishAdapter(hub *Hub) *PublishAdapter {
	return &PublishAdapter{hub: hub}
}

// Publish fans a domain event out to subscribers.
func (p *PublishAdapter) Publish(ev domain.Event) {
	p.hub.Publish(ev)
}

// randBytes returns n random bytes.
func randBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// Fallback; non-cryptographic but avoids a nil/error path.
		for i := range b {
			b[i] = byte(time.Now().UnixNano() >> uint(i))
		}
	}
	return b
}
