package ws

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/code-all-remote/car/internal/storage"
)

func TestWebSocket_RevokedDeviceClosedOnHeartbeat(t *testing.T) {
	db, err := storage.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	defer db.Close()
	h, auth := newAuthedHandler(t, db)
	h.heartbeat = 20 * time.Millisecond
	server := httptest.NewServer(h)
	defer server.CloseClientConnections()
	defer server.Close()
	tok := pairForTest(t, auth)
	conn := dialWS(t, server, tok)
	defer conn.Close()
	dev, err := auth.RefreshToken(context.Background(), "pubkey-test")
	if err != nil {
		t.Fatalf("RefreshToken: %v", err)
	}
	if err := auth.RevokeDevice(context.Background(), dev.DeviceID); err != nil {
		t.Fatalf("RevokeDevice: %v", err)
	}

	// Safety net only: guarantees the test cannot hang indefinitely. A correct
	// implementation delivers the error envelope + close frame within the
	// 20 ms heartbeat window, far inside this budget. A no-op implementation
	// that never sends anything trips this deadline and yields a net.Error
	// timeout — which is NOT a websocket close error, so the assertion below
	// fails. This deadline must NOT be treated as a passing condition.
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}

	// Drain frames until ReadMessage returns a non-nil error. The server is
	// expected to emit a TEXT error-envelope frame and then a CLOSE frame
	// carrying code CloseUnauthorized; the close frame is what surfaces as
	// the terminal error from ReadMessage.
	var readErr error
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			readErr = err
			break
		}
	}

	// The terminal error MUST be a websocket close with code CloseUnauthorized.
	// A read timeout (or any other non-close error) fails the test here.
	if !websocket.IsCloseError(readErr, CloseUnauthorized) {
		t.Fatalf("expected websocket close error with code CloseUnauthorized (%d), got %T: %v",
			CloseUnauthorized, readErr, readErr)
	}
}
