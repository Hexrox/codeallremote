package audit

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/code-all-remote/car/internal/storage"
)

func setupWriter(t *testing.T) (*Writer, func()) {
	db, err := storage.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	w := NewWriter(db)
	cleanup := func() { db.Close() }
	return w, cleanup
}

// insertAuditAt inserts an audit row with an explicit created_at timestamp,
// bypassing datetime('now') so time-range queries are deterministic.
func insertAuditAt(t *testing.T, w *Writer, actorID, targetID string, at time.Time) {
	t.Helper()
	_, err := w.db.Exec(`
		INSERT INTO audit_log (actor_id, actor_type, action, target_type, target_id, outcome, context, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, actorID, ActorTypeUser, "test", "t", targetID, OutcomeSuccess, "{}", at)
	if err != nil {
		t.Fatalf("failed to insert audit at: %v", err)
	}
}

func TestWriter_Record_Basic(t *testing.T) {
	w, cleanup := setupWriter(t)
	defer cleanup()

	err := w.Record(Entry{
		ActorID:    "user-1",
		ActorType:  ActorTypeUser,
		Action:     ActionPromptSubmit,
		TargetType: "session",
		TargetID:   "session-1",
		Outcome:    OutcomeSuccess,
		Context:    map[string]any{"prompt_length": 42},
	})
	if err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	// Verify it was written
	count, _ := w.Count()
	if count != 1 {
		t.Errorf("expected 1 entry, got %d", count)
	}
}

func TestWriter_Record_DefaultOutcome(t *testing.T) {
	w, cleanup := setupWriter(t)
	defer cleanup()

	err := w.Record(Entry{
		ActorID:    "user-1",
		ActorType:  ActorTypeUser,
		Action:     ActionPromptSubmit,
		TargetType: "session",
		TargetID:   "session-1",
		// No outcome
	})
	if err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	recent, _ := w.GetRecent(1)
	if len(recent) != 1 {
		t.Fatal("expected 1 entry")
	}
	if recent[0].Outcome != OutcomeSuccess {
		t.Errorf("expected default outcome success, got %s", recent[0].Outcome)
	}
}

func TestWriter_Record_MissingActorID(t *testing.T) {
	w, cleanup := setupWriter(t)
	defer cleanup()

	err := w.Record(Entry{
		ActorID: "",
		Action:  ActionPromptSubmit,
	})
	if err == nil {
		t.Error("expected error for missing actor_id")
	}
}

func TestWriter_Record_MissingAction(t *testing.T) {
	w, cleanup := setupWriter(t)
	defer cleanup()

	err := w.Record(Entry{
		ActorID: "user-1",
		Action:  "",
	})
	if err == nil {
		t.Error("expected error for missing action")
	}
}

func TestWriter_Record_RedactsSensitiveKeys(t *testing.T) {
	w, cleanup := setupWriter(t)
	defer cleanup()

	err := w.Record(Entry{
		ActorID:    "user-1",
		ActorType:  ActorTypeUser,
		Action:     ActionPromptSubmit,
		TargetType: "session",
		TargetID:   "session-1",
		Context: map[string]any{
			"prompt":        "do the thing",
			"api_key":       "sk-secret-123",
			"token":         "bearer-token-xyz",
			"password":      "hunter2",
			"safe_field":    "visible",
			"prompt_length": 11,
		},
	})
	if err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	recent, _ := w.GetRecent(1)
	if len(recent) != 1 {
		t.Fatal("expected 1 entry")
	}

	var ctx map[string]any
	if recent[0].Context != nil {
		json.Unmarshal([]byte(*recent[0].Context), &ctx)
	}

	if ctx["prompt"] != "[REDACTED]" {
		t.Errorf("expected prompt to be redacted, got %v", ctx["prompt"])
	}
	if ctx["api_key"] != "[REDACTED]" {
		t.Errorf("expected api_key to be redacted, got %v", ctx["api_key"])
	}
	if ctx["token"] != "[REDACTED]" {
		t.Errorf("expected token to be redacted, got %v", ctx["token"])
	}
	if ctx["password"] != "[REDACTED]" {
		t.Errorf("expected password to be redacted, got %v", ctx["password"])
	}
	if ctx["safe_field"] != "visible" {
		t.Errorf("expected safe_field to be visible, got %v", ctx["safe_field"])
	}
}

func TestWriter_Record_RedactsPatterns(t *testing.T) {
	w, cleanup := setupWriter(t)
	defer cleanup()

	w.WithRedactPatterns("sk-secret-123", "secret-key-456")

	err := w.Record(Entry{
		ActorID:    "user-1",
		Action:     ActionPromptSubmit,
		TargetType: "session",
		TargetID:   "session-1",
		Context: map[string]any{
			"command":  "run script with sk-secret-123 and secret-key-456",
			"safe_cmd": "run safe script",
		},
	})
	if err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	recent, _ := w.GetRecent(1)
	var ctx map[string]any
	json.Unmarshal([]byte(*recent[0].Context), &ctx)

	cmd := ctx["command"].(string)
	if strings.Contains(cmd, "sk-secret-123") {
		t.Error("expected pattern sk-secret-123 to be redacted")
	}
	if strings.Contains(cmd, "secret-key-456") {
		t.Error("expected pattern secret-key-456 to be redacted")
	}
}

func TestWriter_GetByID(t *testing.T) {
	w, cleanup := setupWriter(t)
	defer cleanup()

	w.Record(Entry{
		ActorID: "user-1", ActorType: ActorTypeUser,
		Action: ActionPromptSubmit, TargetType: "session", TargetID: "s1",
	})

	recent, _ := w.GetRecent(1)
	id := recent[0].ID

	entry, err := w.GetByID(id)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if entry.ID != id {
		t.Errorf("expected ID %d, got %d", id, entry.ID)
	}
}

func TestWriter_GetByID_NotFound(t *testing.T) {
	w, cleanup := setupWriter(t)
	defer cleanup()

	_, err := w.GetByID(99999)
	if err == nil {
		t.Error("expected error for nonexistent entry")
	}
}

func TestWriter_GetByActor(t *testing.T) {
	w, cleanup := setupWriter(t)
	defer cleanup()

	// Create entries for different actors
	w.Record(Entry{ActorID: "user-1", Action: "test", TargetType: "t", TargetID: "1"})
	w.Record(Entry{ActorID: "user-1", Action: "test", TargetType: "t", TargetID: "2"})
	w.Record(Entry{ActorID: "user-2", Action: "test", TargetType: "t", TargetID: "3"})

	entries, err := w.GetByActor("user-1", 10)
	if err != nil {
		t.Fatalf("GetByActor failed: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries for user-1, got %d", len(entries))
	}
}

func TestWriter_GetByTarget(t *testing.T) {
	w, cleanup := setupWriter(t)
	defer cleanup()

	w.Record(Entry{ActorID: "u1", Action: "test", TargetType: "session", TargetID: "s1"})
	w.Record(Entry{ActorID: "u2", Action: "test", TargetType: "session", TargetID: "s1"})
	w.Record(Entry{ActorID: "u3", Action: "test", TargetType: "session", TargetID: "s2"})

	entries, err := w.GetByTarget("session", "s1", 10)
	if err != nil {
		t.Fatalf("GetByTarget failed: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}
}

func TestWriter_GetByAction(t *testing.T) {
	w, cleanup := setupWriter(t)
	defer cleanup()

	w.Record(Entry{ActorID: "u1", Action: ActionPromptSubmit, TargetType: "t", TargetID: "1"})
	w.Record(Entry{ActorID: "u2", Action: ActionInterrupt, TargetType: "t", TargetID: "2"})
	w.Record(Entry{ActorID: "u3", Action: ActionPromptSubmit, TargetType: "t", TargetID: "3"})

	entries, err := w.GetByAction(ActionPromptSubmit, 10)
	if err != nil {
		t.Fatalf("GetByAction failed: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}
}

func TestWriter_GetRecent(t *testing.T) {
	w, cleanup := setupWriter(t)
	defer cleanup()

	for i := 0; i < 5; i++ {
		w.Record(Entry{
			ActorID: "user-1", Action: "test",
			TargetType: "t", TargetID: string(rune('a' + i)),
		})
	}

	entries, err := w.GetRecent(3)
	if err != nil {
		t.Fatalf("GetRecent failed: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}
}

func TestWriter_GetByTimeRange(t *testing.T) {
	w, cleanup := setupWriter(t)
	defer cleanup()

	// Insert an entry far in the past directly (datetime('now') has second
	// resolution, so we control timestamps explicitly for deterministic range).
	past := time.Now().Add(-2 * time.Hour)
	recent := time.Now()
	insertAuditAt(t, w, "u-old", "1", past)
	insertAuditAt(t, w, "u-recent", "2", recent)

	// Range from 1 hour ago should only include the recent entry.
	oneHourAgo := time.Now().Add(-1 * time.Hour)
	entries, err := w.GetByTimeRange(oneHourAgo, time.Now().Add(time.Hour), 10)
	if err != nil {
		t.Fatalf("GetByTimeRange failed: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(entries))
	}
}

func TestWriter_Count(t *testing.T) {
	w, cleanup := setupWriter(t)
	defer cleanup()

	for i := 0; i < 10; i++ {
		w.Record(Entry{ActorID: "u1", Action: "test", TargetType: "t", TargetID: string(rune('a' + i))})
	}

	count, err := w.Count()
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 10 {
		t.Errorf("expected 10 entries, got %d", count)
	}
}

func TestWriter_PurgeOlderThan(t *testing.T) {
	w, cleanup := setupWriter(t)
	defer cleanup()

	// We cannot control created_at precisely (set to datetime('now')),
	// so this test verifies the purge API does not error and returns 0
	// for a future boundary.
	removed, err := w.PurgeOlderThan(time.Now().Add(1 * time.Hour))
	if err != nil {
		t.Fatalf("PurgeOlderThan failed: %v", err)
	}

	w.Record(Entry{ActorID: "u1", Action: "test", TargetType: "t", TargetID: "1"})

	// Purging with future boundary removes recent entry
	removed, err = w.PurgeOlderThan(time.Now().Add(1 * time.Hour))
	if err != nil {
		t.Fatalf("PurgeOlderThan failed: %v", err)
	}
	if removed != 1 {
		t.Errorf("expected 1 removed, got %d", removed)
	}
}

func TestWriter_AllActions(t *testing.T) {
	w, cleanup := setupWriter(t)
	defer cleanup()

	actions := []string{
		ActionPromptSubmit, ActionInterrupt, ActionApprovalDecision,
		ActionPairing, ActionRevocation, ActionSessionCreate,
		ActionSessionStart, ActionSessionResume,
		ActionWorkspaceRegister, ActionWorkspaceRemove,
	}

	for _, action := range actions {
		err := w.Record(Entry{
			ActorID: "u1", Action: action,
			TargetType: "t", TargetID: "1",
		})
		if err != nil {
			t.Errorf("Record failed for action %s: %v", action, err)
		}
	}

	count, _ := w.Count()
	if count != int64(len(actions)) {
		t.Errorf("expected %d entries, got %d", len(actions), count)
	}
}

func TestWriter_RecordWithTx(t *testing.T) {
	db, err := storage.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	w := NewWriter(db)

	// Audit record should commit with the transaction.
	err = db.WithTransaction(func(tx *sql.Tx) error {
		return w.RecordWithTx(tx, Entry{
			ActorID: "user-1", ActorType: ActorTypeUser,
			Action: ActionPromptSubmit, TargetType: "session", TargetID: "s1",
		})
	})
	if err != nil {
		t.Fatalf("RecordWithTx failed: %v", err)
	}

	count, _ := w.Count()
	if count != 1 {
		t.Errorf("expected 1 entry after tx commit, got %d", count)
	}
}

func TestWriter_RecordWithTx_Rollback(t *testing.T) {
	db, err := storage.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	w := NewWriter(db)

	// Audit record should roll back when the transaction fails.
	err = db.WithTransaction(func(tx *sql.Tx) error {
		if err := w.RecordWithTx(tx, Entry{
			ActorID: "user-1", Action: ActionPromptSubmit,
			TargetType: "session", TargetID: "s1",
		}); err != nil {
			return err
		}
		// Force a rollback.
		return fmt.Errorf("simulated failure")
	})
	if err == nil {
		t.Fatal("expected transaction error, got nil")
	}

	count, _ := w.Count()
	if count != 0 {
		t.Errorf("expected 0 entries after rollback, got %d", count)
	}
}
