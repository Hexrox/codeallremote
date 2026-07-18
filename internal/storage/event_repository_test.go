package storage

import (
	"testing"
	"time"

	"github.com/code-all-remote/car/internal/domain"
)

// createTestSessionForEvents creates a session+workspace so that events can
// satisfy the foreign key on events.session_id.
func createTestSessionForEvents(t *testing.T, db *DB, sessionID, workspaceID string) {
	t.Helper()
	wsRepo := NewWorkspaceRepository(db)
	if err := wsRepo.Create(&domain.Workspace{
		ID: workspaceID, DisplayName: "Test", Path: "/tmp/" + workspaceID,
	}); err != nil {
		t.Fatalf("failed to create workspace: %v", err)
	}
	sessRepo := NewSessionRepository(db)
	if err := sessRepo.CreateSession(&domain.Session{
		ID: sessionID, WorkspaceID: workspaceID, AdapterID: "claude-code",
		State: domain.SessionStateActive,
	}); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
}

func TestEventRepository_AppendAndGet(t *testing.T) {
	db, err := Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	repo := NewEventRepository(db)
	createTestSessionForEvents(t, db, "s1", "ws-1")

	event := &domain.Event{
		SessionID:     "s1",
		Sequence:      1,
		Type:          "session.started",
		MessageID:     "msg-001",
		SchemaVersion: 1,
		Payload:       map[string]any{"adapter_id": "claude-code"},
		OccurredAt:    time.Now(),
	}

	// Append event
	seq, err := repo.Append(event)
	if err != nil {
		t.Fatalf("failed to append event: %v", err)
	}
	if seq != 1 {
		t.Errorf("expected sequence 1, got %d", seq)
	}

	// Get events after sequence 0
	events, err := repo.GetBySequence("s1", 0, 100)
	if err != nil {
		t.Fatalf("failed to get events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	if events[0].Type != event.Type {
		t.Errorf("expected type %s, got %s", event.Type, events[0].Type)
	}
	if events[0].MessageID != event.MessageID {
		t.Errorf("expected message_id %s, got %s", event.MessageID, events[0].MessageID)
	}
}

func TestEventRepository_GetLastSequence(t *testing.T) {
	db, err := Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	repo := NewEventRepository(db)
	createTestSessionForEvents(t, db, "s1", "ws-1")

	// Empty session should return 0
	seq, err := repo.GetLastSequence("nonexistent")
	if err != nil {
		t.Fatalf("GetLastSequence failed: %v", err)
	}
	if seq != 0 {
		t.Errorf("expected 0 for empty session, got %d", seq)
	}

	// Add events
	events := []domain.Event{
		{SessionID: "s1", Sequence: 1, Type: "e1", MessageID: "m1", SchemaVersion: 1, Payload: map[string]any{}, OccurredAt: time.Now()},
		{SessionID: "s1", Sequence: 2, Type: "e2", MessageID: "m2", SchemaVersion: 1, Payload: map[string]any{}, OccurredAt: time.Now()},
		{SessionID: "s1", Sequence: 5, Type: "e5", MessageID: "m5", SchemaVersion: 1, Payload: map[string]any{}, OccurredAt: time.Now()},
	}

	for _, e := range events {
		_, err := repo.Append(&e)
		if err != nil {
			t.Fatalf("failed to append event: %v", err)
		}
	}

	seq, err = repo.GetLastSequence("s1")
	if err != nil {
		t.Fatalf("GetLastSequence failed: %v", err)
	}
	if seq != 5 {
		t.Errorf("expected last sequence 5, got %d", seq)
	}
}

func TestEventRepository_GetBySequence_Ordering(t *testing.T) {
	db, err := Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	repo := NewEventRepository(db)
	createTestSessionForEvents(t, db, "s1", "ws-1")

	// Add events out of order
	events := []domain.Event{
		{SessionID: "s1", Sequence: 3, Type: "e3", MessageID: "m3", SchemaVersion: 1, Payload: map[string]any{}, OccurredAt: time.Now()},
		{SessionID: "s1", Sequence: 1, Type: "e1", MessageID: "m1", SchemaVersion: 1, Payload: map[string]any{}, OccurredAt: time.Now()},
		{SessionID: "s1", Sequence: 2, Type: "e2", MessageID: "m2", SchemaVersion: 1, Payload: map[string]any{}, OccurredAt: time.Now()},
	}

	for _, e := range events {
		_, err := repo.Append(&e)
		if err != nil {
			t.Fatalf("failed to append event: %v", err)
		}
	}

	// Should return in order
	result, err := repo.GetBySequence("s1", 0, 100)
	if err != nil {
		t.Fatalf("failed to get events: %v", err)
	}

	if len(result) != 3 {
		t.Fatalf("expected 3 events, got %d", len(result))
	}

	if result[0].Sequence != 1 || result[1].Sequence != 2 || result[2].Sequence != 3 {
		t.Errorf("events not returned in sequence order: %v", []int64{result[0].Sequence, result[1].Sequence, result[2].Sequence})
	}
}

func TestEventRepository_GetBySequence_Limit(t *testing.T) {
	db, err := Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	repo := NewEventRepository(db)
	createTestSessionForEvents(t, db, "s1", "ws-1")

	// Add 10 events
	for i := int64(1); i <= 10; i++ {
		e := domain.Event{
			SessionID: "s1", Sequence: i, Type: "e", MessageID: string(rune('a' + i)), SchemaVersion: 1,
			Payload: map[string]any{}, OccurredAt: time.Now(),
		}
		_, err := repo.Append(&e)
		if err != nil {
			t.Fatalf("failed to append event: %v", err)
		}
	}

	// Get with limit
	result, err := repo.GetBySequence("s1", 0, 5)
	if err != nil {
		t.Fatalf("failed to get events: %v", err)
	}
	if len(result) != 5 {
		t.Errorf("expected 5 events with limit, got %d", len(result))
	}
}

func TestEventRepository_GetBySequence_After(t *testing.T) {
	db, err := Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	repo := NewEventRepository(db)
	createTestSessionForEvents(t, db, "s1", "ws-1")

	// Add events
	for i := int64(1); i <= 5; i++ {
		e := domain.Event{
			SessionID: "s1", Sequence: i, Type: "e", MessageID: string(rune('a' + i)), SchemaVersion: 1,
			Payload: map[string]any{}, OccurredAt: time.Now(),
		}
		_, err := repo.Append(&e)
		if err != nil {
			t.Fatalf("failed to append event: %v", err)
		}
	}

	// Get after sequence 3
	result, err := repo.GetBySequence("s1", 3, 100)
	if err != nil {
		t.Fatalf("failed to get events: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 events after sequence 3, got %d", len(result))
	}
	if result[0].Sequence != 4 || result[1].Sequence != 5 {
		t.Errorf("wrong sequences: expected 4,5 got %d,%d", result[0].Sequence, result[1].Sequence)
	}
}

func TestEventRepository_UniqueMessageID(t *testing.T) {
	db, err := Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	repo := NewEventRepository(db)
	createTestSessionForEvents(t, db, "s1", "ws-1")
	createTestSessionForEvents(t, db, "s2", "ws-2")

	e1 := domain.Event{
		SessionID: "s1", Sequence: 1, Type: "e1", MessageID: "msg-dup", SchemaVersion: 1,
		Payload: map[string]any{}, OccurredAt: time.Now(),
	}
	e2 := domain.Event{
		SessionID: "s2", Sequence: 1, Type: "e2", MessageID: "msg-dup", SchemaVersion: 1,
		Payload: map[string]any{}, OccurredAt: time.Now(),
	}

	_, err = repo.Append(&e1)
	if err != nil {
		t.Fatalf("failed to append first event: %v", err)
	}

	// Same message_id should fail (unique constraint)
	_, err = repo.Append(&e2)
	if err == nil {
		t.Error("expected error for duplicate message_id, got nil")
	}
}

func TestEventRepository_Count(t *testing.T) {
	db, err := Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	repo := NewEventRepository(db)
	createTestSessionForEvents(t, db, "s1", "ws-1")

	// Empty session
	count, err := repo.Count("s1")
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 events, got %d", count)
	}

	// Add events
	for i := int64(1); i <= 5; i++ {
		e := domain.Event{
			SessionID: "s1", Sequence: i, Type: "e", MessageID: string(rune('a' + i)), SchemaVersion: 1,
			Payload: map[string]any{}, OccurredAt: time.Now(),
		}
		_, err := repo.Append(&e)
		if err != nil {
			t.Fatalf("failed to append event: %v", err)
		}
	}

	count, err = repo.Count("s1")
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 5 {
		t.Errorf("expected 5 events, got %d", count)
	}
}

func TestEventRepository_MultipleSessions(t *testing.T) {
	db, err := Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	repo := NewEventRepository(db)
	createTestSessionForEvents(t, db, "s1", "ws-1")
	createTestSessionForEvents(t, db, "s2", "ws-2")

	// Add events to different sessions
	for i := int64(1); i <= 3; i++ {
		e := domain.Event{
			SessionID: "s1", Sequence: i, Type: "e", MessageID: "s1m" + string(rune('a'+i)), SchemaVersion: 1,
			Payload: map[string]any{}, OccurredAt: time.Now(),
		}
		_, err := repo.Append(&e)
		if err != nil {
			t.Fatalf("failed to append event: %v", err)
		}
	}

	for i := int64(1); i <= 5; i++ {
		e := domain.Event{
			SessionID: "s2", Sequence: i, Type: "e", MessageID: "s2m" + string(rune('a'+i)), SchemaVersion: 1,
			Payload: map[string]any{}, OccurredAt: time.Now(),
		}
		_, err := repo.Append(&e)
		if err != nil {
			t.Fatalf("failed to append event: %v", err)
		}
	}

	// Verify isolation
	s1Events, _ := repo.GetBySequence("s1", 0, 100)
	s2Events, _ := repo.GetBySequence("s2", 0, 100)

	if len(s1Events) != 3 {
		t.Errorf("expected 3 events for s1, got %d", len(s1Events))
	}
	if len(s2Events) != 5 {
		t.Errorf("expected 5 events for s2, got %d", len(s2Events))
	}

	// Verify sequences are independent
	s1Seq, _ := repo.GetLastSequence("s1")
	s2Seq, _ := repo.GetLastSequence("s2")

	if s1Seq != 3 {
		t.Errorf("expected s1 sequence 3, got %d", s1Seq)
	}
	if s2Seq != 5 {
		t.Errorf("expected s2 sequence 5, got %d", s2Seq)
	}
}
