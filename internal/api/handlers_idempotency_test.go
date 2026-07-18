package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

// TestHandlers_IdempotencyReplay proves a retried POST with the same
// Idempotency-Key replays the original response instead of creating a second
// resource (docs/10 §35, docs/13). Regression for K1.
func TestHandlers_IdempotencyReplay(t *testing.T) {
	server, _ := setupTestServer(t)
	const key = "idem-replay-1234"
	body := `{"workspace_id":"ws-1","adapter_id":"fake-adapter","title":"R1"}`

	// First request creates a session.
	resp1, b1 := doRequest(t, server, "POST", "/api/v1/sessions", body, true, key)
	if resp1.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp1.StatusCode)
	}
	var snap1 struct {
		ID string `json:"id"`
	}
	json.Unmarshal(b1, &snap1)
	if snap1.ID == "" {
		t.Fatal("expected a session id")
	}

	// Retry with the SAME key + body: MUST replay the original response (same
	// id) and signal the replay. No second session is created.
	resp2, b2 := doRequest(t, server, "POST", "/api/v1/sessions", body, true, key)
	if resp2.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 replay, got %d", resp2.StatusCode)
	}
	if resp2.Header.Get("X-Idempotent-Replay") != "true" {
		t.Error("expected X-Idempotent-Replay header on retry")
	}
	var snap2 struct {
		ID string `json:"id"`
	}
	json.Unmarshal(b2, &snap2)
	if snap2.ID != snap1.ID {
		t.Errorf("idempotency violated: retry created a different session (%s vs %s)", snap1.ID, snap2.ID)
	}

	// List: only ONE session exists (the retry did not create a second).
	resp3, b3 := doRequest(t, server, "GET", "/api/v1/sessions", "", true, "")
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 list, got %d", resp3.StatusCode)
	}
	var list struct {
		Sessions []map[string]any `json:"sessions"`
	}
	json.Unmarshal(b3, &list)
	if len(list.Sessions) != 1 {
		t.Errorf("expected exactly 1 session after retry, got %d", len(list.Sessions))
	}
}

// TestHandlers_IdempotencyDifferentKeyCreatesNew proves a different key is
// treated as a new command (not deduped).
func TestHandlers_IdempotencyDifferentKeyCreatesNew(t *testing.T) {
	server, _ := setupTestServer(t)
	body := `{"workspace_id":"ws-1","adapter_id":"fake-adapter"}`

	doRequest(t, server, "POST", "/api/v1/sessions", body, true, "key-aaa11111")
	doRequest(t, server, "POST", "/api/v1/sessions", body, true, "key-bbb22222")

	resp, b := doRequest(t, server, "GET", "/api/v1/sessions", "", true, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var list struct {
		Sessions []map[string]any `json:"sessions"`
	}
	json.Unmarshal(b, &list)
	if len(list.Sessions) != 2 {
		t.Errorf("expected 2 sessions for distinct keys, got %d", len(list.Sessions))
	}
}
