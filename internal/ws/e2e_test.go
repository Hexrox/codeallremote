package ws

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/code-all-remote/car/internal/domain"
	"github.com/code-all-remote/car/internal/identity"
	"github.com/code-all-remote/car/internal/storage"
	"github.com/gorilla/websocket"
)

// newAuthedHandler builds an identity-backed WS handler with the heartbeat
// pinger disabled (tests use short, deterministic connections).
func newAuthedHandler(t *testing.T, db *storage.DB) (*Handler, *identity.Service) {
	t.Helper()
	cursor := storage.NewCursorRepository(db, 0)
	hub := NewHub(cursor, slog.New(slog.NewTextHandler(io.Discard, nil)))
	auth := identity.NewService(db)
	h := NewHandler(hub, auth, slog.New(slog.NewTextHandler(io.Discard, nil)))
	h.heartbeat = -1 // no pinger; tests control connection lifetime
	return h, auth
}

// pairForTest issues a challenge and pairs a device, returning the token.
func pairForTest(t *testing.T, auth *identity.Service) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ch, err := auth.CreateChallenge(5 * time.Minute)
	if err != nil {
		t.Fatalf("CreateChallenge: %v", err)
	}
	tok, err := auth.PairDevice(ctx, ch.Code, "test-device", "pubkey-test")
	if err != nil {
		t.Fatalf("PairDevice: %v", err)
	}
	return tok.Value
}

// dialWS dials the test server with a token.
func dialWS(t *testing.T, server *httptest.Server, tok string) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/ws?token=" + tok
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return conn
}

// TestWebSocket_PairingTokenAuthorizesWS verifies a paired-device token works
// over the WebSocket handshake and a revoked device is rejected.
func TestWebSocket_PairingTokenAuthorizesWS(t *testing.T) {
	db, err := storage.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	h, auth := newAuthedHandler(t, db)
	server := httptest.NewServer(h)
	defer func() {
		server.CloseClientConnections()
		server.Close()
	}()

	tok := pairForTest(t, auth)

	// Token authorizes WS.
	id, err := auth.AuthorizeWS(tok)
	if err != nil {
		t.Fatalf("AuthorizeWS: %v", err)
	}
	if id == "" {
		t.Error("expected device ID")
	}

	// Revoking the device blocks WS auth.
	dev, _ := auth.RefreshToken(context.Background(), "pubkey-test")
	if err := auth.RevokeDevice(context.Background(), dev.DeviceID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := auth.AuthorizeWS(tok); err == nil {
		t.Error("expected revoked token to fail WS auth")
	}
}

// TestWebSocket_Unauthorized verifies a missing token fails the HTTP upgrade
// (401) rather than upgrading to a WebSocket.
func TestWebSocket_Unauthorized(t *testing.T) {
	db, err := storage.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	h, _ := newAuthedHandler(t, db)
	server := httptest.NewServer(h)
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/v1/ws")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

// TestWebSocket_HelloReplayAndLive verifies hello→welcome→replay→live delivery
// over a real socket, proving no gaps/duplicates in the simple case.
func TestWebSocket_HelloReplayAndLive(t *testing.T) {
	db, err := storage.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	// Seed a session with two retained events.
	wsRepo := storage.NewWorkspaceRepository(db)
	wsRepo.Create(&domain.Workspace{ID: "ws-1", DisplayName: "T", Path: "/tmp/ws1"})
	sessRepo := storage.NewSessionRepository(db)
	sessRepo.CreateSession(&domain.Session{
		ID: "ses-1", WorkspaceID: "ws-1", AdapterID: "fake", State: domain.SessionStateActive,
	})
	cursor := storage.NewCursorRepository(db, 0)
	for i := int64(1); i <= 2; i++ {
		cursor.AppendWithoutTx(&domain.Event{
			SessionID: "ses-1", Type: "run.output", MessageID: "m" + string(rune('a'+int(i))),
			SchemaVersion: 1, Payload: map[string]any{"seq": i}, OccurredAt: time.Now(),
		})
	}

	h, auth := newAuthedHandler(t, db)
	// Replace the hub's journal to point at our seeded cursor.
	h.hub.journal = cursor

	server := httptest.NewServer(h)
	defer func() {
		server.CloseClientConnections()
		server.Close()
	}()

	conn := dialWS(t, server, pairForTest(t, auth))
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))

	// hello with cursor at 0 → expect welcome + events 1,2.
	hello := Hello{
		Type: "hello", ProtocolVersion: 1, DeviceID: "dev-1",
		Cursors: []Cursor{{SessionID: "ses-1", After: 0}},
	}
	data, _ := json.Marshal(hello)
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		t.Fatalf("write hello: %v", err)
	}

	// Read messages until we have welcome + 2 events.
	got := []string{}
	for len(got) < 3 {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read: %v (got %d msgs)", err, len(got))
		}
		var env Envelope
		json.Unmarshal(msg, &env)
		got = append(got, env.Type)
	}

	if got[0] != "welcome" {
		t.Errorf("expected welcome first, got %s", got[0])
	}
	// got[1] and got[2] are the two replayed run.output events.
	if got[1] != "run.output" || got[2] != "run.output" {
		t.Errorf("expected two run.output events, got %s, %s", got[1], got[2])
	}
	// Publish a live event; it should arrive on the open socket.
	h.hub.Publish(domain.Event{
		Type: "run.output", MessageID: "live", SessionID: "ses-1",
		Sequence: 3, Payload: map[string]any{"live": true}, OccurredAt: time.Now(),
	})
	_, live, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read live: %v", err)
	}
	var liveEnv Envelope
	json.Unmarshal(live, &liveEnv)
	if liveEnv.Sequence != 3 {
		t.Errorf("expected live seq 3, got %d", liveEnv.Sequence)
	}
}
