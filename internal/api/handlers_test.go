package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/code-all-remote/car/internal/app"
	"github.com/code-all-remote/car/internal/config"
	"github.com/code-all-remote/car/internal/domain"
)

const testToken = "test-token-min-16-chars"

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func setupTestApp(t *testing.T) *app.App {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "car.db")

	cfg := &config.Config{
		Server: config.ServerConfig{
			Host: "127.0.0.1", Port: 0,
			ReadTimeout:     config.Duration(5 * time.Second),
			WriteTimeout:    config.Duration(5 * time.Second),
			ShutdownTimeout: config.Duration(5 * time.Second),
		},
		Storage: config.StorageConfig{Type: "sqlite", DataSource: dbPath},
		Security: config.SecurityConfig{
			APIToken: testToken, TokenExpiry: config.Duration(24 * time.Hour),
		},
		Logging: config.LoggingConfig{Level: "error", Format: "text", Output: "stderr"},
	}

	a, err := app.New(cfg, testLogger())
	if err != nil {
		t.Fatalf("failed to create app: %v", err)
	}

	wsDir := filepath.Join(tmpDir, "workspace")
	os.MkdirAll(wsDir, 0o755)
	a.RegisterWorkspaceForTest(&domain.Workspace{
		ID: "ws-1", DisplayName: "Test", Path: wsDir,
		AllowedAdapters: []string{"fake-adapter"},
	})

	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("app start failed: %v", err)
	}
	t.Cleanup(func() { a.Shutdown(context.Background()) })
	return a
}

func setupTestServer(t *testing.T) (*httptest.Server, *app.App) {
	t.Helper()
	a := setupTestApp(t)
	h := NewHandlers(a, testLogger())
	mux := http.NewServeMux()
	h.Register(mux)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server, a
}

func doRequest(t *testing.T, server *httptest.Server, method, path, body string, withAuth bool, idemKey string) (*http.Response, []byte) {
	t.Helper()
	var req *http.Request
	var err error
	if body != "" {
		req, err = http.NewRequest(method, server.URL+path, strings.NewReader(body))
	} else {
		req, err = http.NewRequest(method, server.URL+path, nil)
	}
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if withAuth {
		req.Header.Set("Authorization", "Bearer "+testToken)
	}
	if idemKey != "" {
		req.Header.Set("Idempotency-Key", idemKey)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, respBody
}

func TestHandlers_CreateSession_Unauthorized(t *testing.T) {
	server, _ := setupTestServer(t)

	resp, _ := doRequest(t, server, "POST", "/api/v1/sessions", `{"workspace_id":"ws-1","adapter_id":"fake-adapter"}`, false, "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 without auth, got %d", resp.StatusCode)
	}
}

func TestHandlers_CreateSession_MissingIdempotencyKey(t *testing.T) {
	server, _ := setupTestServer(t)

	resp, _ := doRequest(t, server, "POST", "/api/v1/sessions", `{"workspace_id":"ws-1","adapter_id":"fake-adapter"}`, true, "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 without idempotency key, got %d", resp.StatusCode)
	}
}

func TestHandlers_CreateSession_Success(t *testing.T) {
	server, _ := setupTestServer(t)

	resp, body := doRequest(t, server, "POST", "/api/v1/sessions",
		`{"workspace_id":"ws-1","adapter_id":"fake-adapter","title":"Test"}`, true, "idem-12345678")

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, body)
	}

	var snap struct {
		ID           string `json:"id"`
		WorkspaceID  string `json:"workspace_id"`
		AdapterID    string `json:"adapter_id"`
		State        string `json:"state"`
		LastSequence int64  `json:"last_sequence"`
	}
	json.Unmarshal(body, &snap)

	if snap.ID == "" {
		t.Error("expected non-empty session ID")
	}
	if snap.State != "created" {
		t.Errorf("expected state created, got %s", snap.State)
	}
	if snap.WorkspaceID != "ws-1" {
		t.Errorf("expected workspace ws-1, got %s", snap.WorkspaceID)
	}
}

func TestHandlers_CreateSession_InvalidBody(t *testing.T) {
	server, _ := setupTestServer(t)

	resp, _ := doRequest(t, server, "POST", "/api/v1/sessions",
		`{invalid json}`, true, "idem-12345678")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d", resp.StatusCode)
	}
}

func TestHandlers_CreateSession_InvalidWorkspace(t *testing.T) {
	server, _ := setupTestServer(t)

	resp, _ := doRequest(t, server, "POST", "/api/v1/sessions",
		`{"workspace_id":"nonexistent","adapter_id":"fake-adapter"}`, true, "idem-12345678")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for nonexistent workspace, got %d", resp.StatusCode)
	}
}

func TestHandlers_ListSessions(t *testing.T) {
	server, _ := setupTestServer(t)

	// Create two sessions.
	doRequest(t, server, "POST", "/api/v1/sessions",
		`{"workspace_id":"ws-1","adapter_id":"fake-adapter"}`, true, "idem-aaa11111")
	doRequest(t, server, "POST", "/api/v1/sessions",
		`{"workspace_id":"ws-1","adapter_id":"fake-adapter"}`, true, "idem-bbb22222")

	resp, body := doRequest(t, server, "GET", "/api/v1/sessions", "", true, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var list struct {
		Sessions []map[string]any `json:"sessions"`
	}
	json.Unmarshal(body, &list)
	if len(list.Sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(list.Sessions))
	}
}

func TestHandlers_GetSession_NotFound(t *testing.T) {
	server, _ := setupTestServer(t)

	resp, _ := doRequest(t, server, "GET", "/api/v1/sessions/nonexistent", "", true, "")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestHandlers_GetSession_Success(t *testing.T) {
	server, _ := setupTestServer(t)

	_, body := doRequest(t, server, "POST", "/api/v1/sessions",
		`{"workspace_id":"ws-1","adapter_id":"fake-adapter"}`, true, "idem-12345678")
	var snap struct {
		ID string `json:"id"`
	}
	json.Unmarshal(body, &snap)

	resp, body2 := doRequest(t, server, "GET", "/api/v1/sessions/"+snap.ID, "", true, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if !bytes.Contains(body2, []byte(`"id":"`+snap.ID)) {
		t.Errorf("response does not contain session ID: %s", body2)
	}
}

func TestHandlers_StartRun_Success(t *testing.T) {
	server, _ := setupTestServer(t)

	_, body := doRequest(t, server, "POST", "/api/v1/sessions",
		`{"workspace_id":"ws-1","adapter_id":"fake-adapter"}`, true, "idem-12345678")
	var snap struct {
		ID string `json:"id"`
	}
	json.Unmarshal(body, &snap)

	resp, _ := doRequest(t, server, "POST", "/api/v1/sessions/"+snap.ID+"/runs", "", true, "idem-run-1234567")
	if resp.StatusCode != http.StatusAccepted {
		t.Errorf("expected 202, got %d", resp.StatusCode)
	}
}

func TestHandlers_StartRun_MissingIdempotencyKey(t *testing.T) {
	server, _ := setupTestServer(t)

	_, body := doRequest(t, server, "POST", "/api/v1/sessions",
		`{"workspace_id":"ws-1","adapter_id":"fake-adapter"}`, true, "idem-12345678")
	var snap struct {
		ID string `json:"id"`
	}
	json.Unmarshal(body, &snap)

	resp, _ := doRequest(t, server, "POST", "/api/v1/sessions/"+snap.ID+"/runs", "", true, "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestHandlers_SubmitPrompt_Success(t *testing.T) {
	server, _ := setupTestServer(t)

	_, body := doRequest(t, server, "POST", "/api/v1/sessions",
		`{"workspace_id":"ws-1","adapter_id":"fake-adapter"}`, true, "idem-12345678")
	var snap struct {
		ID string `json:"id"`
	}
	json.Unmarshal(body, &snap)

	doRequest(t, server, "POST", "/api/v1/sessions/"+snap.ID+"/runs", "", true, "idem-run-1234567")
	time.Sleep(50 * time.Millisecond)

	resp, _ := doRequest(t, server, "POST", "/api/v1/sessions/"+snap.ID+"/prompts",
		`{"text":"do something"}`, true, "idem-prompt-12345")
	if resp.StatusCode != http.StatusAccepted {
		t.Errorf("expected 202, got %d", resp.StatusCode)
	}
}

func TestHandlers_SubmitPrompt_EmptyText(t *testing.T) {
	server, _ := setupTestServer(t)

	_, body := doRequest(t, server, "POST", "/api/v1/sessions",
		`{"workspace_id":"ws-1","adapter_id":"fake-adapter"}`, true, "idem-12345678")
	var snap struct {
		ID string `json:"id"`
	}
	json.Unmarshal(body, &snap)

	resp, _ := doRequest(t, server, "POST", "/api/v1/sessions/"+snap.ID+"/prompts",
		`{"text":""}`, true, "idem-prompt-12345")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for empty text, got %d", resp.StatusCode)
	}
}

func TestHandlers_GetEvents_Success(t *testing.T) {
	server, _ := setupTestServer(t)

	_, body := doRequest(t, server, "POST", "/api/v1/sessions",
		`{"workspace_id":"ws-1","adapter_id":"fake-adapter"}`, true, "idem-12345678")
	var snap struct {
		ID string `json:"id"`
	}
	json.Unmarshal(body, &snap)

	doRequest(t, server, "POST", "/api/v1/sessions/"+snap.ID+"/runs", "", true, "idem-run-1234567")
	time.Sleep(200 * time.Millisecond)

	resp, body2 := doRequest(t, server, "GET", "/api/v1/sessions/"+snap.ID+"/events", "", true, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var events struct {
		Events         []map[string]any `json:"events"`
		NextAfter      int64            `json:"next_after"`
		ResyncRequired bool             `json:"resync_required"`
	}
	json.Unmarshal(body2, &events)
	if len(events.Events) == 0 {
		t.Error("expected at least one event")
	}
}

func TestHandlers_GetEvents_WithAfter(t *testing.T) {
	server, _ := setupTestServer(t)

	_, body := doRequest(t, server, "POST", "/api/v1/sessions",
		`{"workspace_id":"ws-1","adapter_id":"fake-adapter"}`, true, "idem-12345678")
	var snap struct {
		ID string `json:"id"`
	}
	json.Unmarshal(body, &snap)

	doRequest(t, server, "POST", "/api/v1/sessions/"+snap.ID+"/runs", "", true, "idem-run-1234567")
	time.Sleep(200 * time.Millisecond)

	// Get all events first.
	_, body2 := doRequest(t, server, "GET", "/api/v1/sessions/"+snap.ID+"/events", "", true, "")
	var all struct {
		Events []struct {
			Sequence int64 `json:"sequence"`
		} `json:"events"`
	}
	json.Unmarshal(body2, &all)

	if len(all.Events) < 2 {
		t.Fatalf("expected at least 2 events, got %d", len(all.Events))
	}

	// Get events after the first.
	after := all.Events[0].Sequence
	resp, _ := doRequest(t, server, "GET",
		fmt.Sprintf("/api/v1/sessions/%s/events?after=%d", snap.ID, after), "", true, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestHandlers_GetEvents_SessionNotFound(t *testing.T) {
	server, _ := setupTestServer(t)

	resp, _ := doRequest(t, server, "GET", "/api/v1/sessions/nonexistent/events", "", true, "")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestHandlers_Interrupt_Success(t *testing.T) {
	server, _ := setupTestServer(t)

	_, body := doRequest(t, server, "POST", "/api/v1/sessions",
		`{"workspace_id":"ws-1","adapter_id":"fake-adapter"}`, true, "idem-12345678")
	var snap struct {
		ID string `json:"id"`
	}
	json.Unmarshal(body, &snap)

	doRequest(t, server, "POST", "/api/v1/sessions/"+snap.ID+"/runs", "", true, "idem-run-1234567")
	time.Sleep(50 * time.Millisecond)

	resp, _ := doRequest(t, server, "POST", "/api/v1/sessions/"+snap.ID+"/interrupt", "", true, "idem-int-1234567")
	if resp.StatusCode != http.StatusAccepted {
		t.Errorf("expected 202, got %d", resp.StatusCode)
	}
}

func TestHandlers_DecideApproval_InvalidDecision(t *testing.T) {
	server, _ := setupTestServer(t)

	resp, _ := doRequest(t, server, "POST", "/api/v1/approvals/apr-1/decision",
		`{"decision":"maybe"}`, true, "idem-apr-1234567")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid decision, got %d", resp.StatusCode)
	}
}

func TestHandlers_DecideApproval_NotFound(t *testing.T) {
	server, _ := setupTestServer(t)

	resp, _ := doRequest(t, server, "POST", "/api/v1/approvals/nonexistent/decision",
		`{"decision":"approve"}`, true, "idem-apr-1234567")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for nonexistent approval, got %d", resp.StatusCode)
	}
}

func TestHandlers_MethodNotAllowed(t *testing.T) {
	server, _ := setupTestServer(t)

	resp, _ := doRequest(t, server, "DELETE", "/api/v1/sessions", "", true, "")
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", resp.StatusCode)
	}
}

func TestHandlers_GetApproval_NotFound(t *testing.T) {
	server, _ := setupTestServer(t)

	resp, _ := doRequest(t, server, "GET", "/api/v1/approvals/nonexistent", "", true, "")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}
