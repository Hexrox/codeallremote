package storage

import (
	"sync"
	"testing"
	"time"

	"github.com/code-all-remote/car/internal/domain"
)

func setupCursorTest(t *testing.T, retention time.Duration) (*DB, *CursorRepository, func()) {
	db, err := Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	// Create prerequisites
	wsRepo := NewWorkspaceRepository(db)
	wsRepo.Create(&domain.Workspace{ID: "ws-1", DisplayName: "Test", Path: "/tmp/ws1"})

	sessRepo := NewSessionRepository(db)
	sessRepo.CreateSession(&domain.Session{
		ID:          "session-1",
		WorkspaceID: "ws-1",
		AdapterID:   "claude-code",
		State:       domain.SessionStateActive,
	})

	repo := NewCursorRepository(db, retention)
	cleanup := func() { db.Close() }
	return db, repo, cleanup
}

func TestCursorRepository_Replay_Basic(t *testing.T) {
	_, repo, cleanup := setupCursorTest(t, 0)
	defer cleanup()

	// Add events
	for i := int64(1); i <= 5; i++ {
		repo.AppendWithoutTx(&domain.Event{
			SessionID: "session-1", Type: "test", MessageID: "msg" + string(rune('a'+int(i))),
			SchemaVersion: 1, Payload: map[string]any{"seq": i}, OccurredAt: time.Now(),
		})
	}

	// Replay from start
	result, err := repo.Replay("session-1", 0, 100)
	if err != nil {
		t.Fatalf("Replay failed: %v", err)
	}

	if len(result.Events) != 5 {
		t.Errorf("expected 5 events, got %d", len(result.Events))
	}
	if result.NextAfter != 5 {
		t.Errorf("expected next_after 5, got %d", result.NextAfter)
	}
	if result.ResyncRequired {
		t.Error("expected resync_required to be false")
	}
}

func TestCursorRepository_Replay_After(t *testing.T) {
	_, repo, cleanup := setupCursorTest(t, 0)
	defer cleanup()

	// Add events
	for i := int64(1); i <= 10; i++ {
		repo.AppendWithoutTx(&domain.Event{
			SessionID: "session-1", Type: "test", MessageID: "msg" + string(rune('a'+int(i))),
			SchemaVersion: 1, Payload: map[string]any{}, OccurredAt: time.Now(),
		})
	}

	// Replay after sequence 7
	result, err := repo.Replay("session-1", 7, 100)
	if err != nil {
		t.Fatalf("Replay failed: %v", err)
	}

	if len(result.Events) != 3 {
		t.Errorf("expected 3 events, got %d", len(result.Events))
	}
	if result.NextAfter != 10 {
		t.Errorf("expected next_after 10, got %d", result.NextAfter)
	}
}

func TestCursorRepository_Replay_Limit(t *testing.T) {
	_, repo, cleanup := setupCursorTest(t, 0)
	defer cleanup()

	// Add events
	for i := int64(1); i <= 20; i++ {
		repo.AppendWithoutTx(&domain.Event{
			SessionID: "session-1", Type: "test", MessageID: "msg" + string(rune('a'+int(i))),
			SchemaVersion: 1, Payload: map[string]any{}, OccurredAt: time.Now(),
		})
	}

	// Replay with limit
	result, err := repo.Replay("session-1", 0, 5)
	if err != nil {
		t.Fatalf("Replay failed: %v", err)
	}

	if len(result.Events) != 5 {
		t.Errorf("expected 5 events, got %d", len(result.Events))
	}
	if !result.HasMore {
		t.Error("expected has_more to be true")
	}
}

func TestCursorRepository_Replay_StableOrdering(t *testing.T) {
	_, repo, cleanup := setupCursorTest(t, 0)
	defer cleanup()

	// Add events
	for i := int64(1); i <= 20; i++ {
		repo.AppendWithoutTx(&domain.Event{
			SessionID: "session-1", Type: "test", MessageID: "msg" + string(rune('a'+int(i))),
			SchemaVersion: 1, Payload: map[string]any{"seq": i}, OccurredAt: time.Now(),
		})
	}

	// Replay in pages and verify ordering
	cursor := int64(0)
	var seenSeqs []int64

	for {
		result, err := repo.Replay("session-1", cursor, 5)
		if err != nil {
			t.Fatalf("Replay failed: %v", err)
		}

		for _, e := range result.Events {
			seenSeqs = append(seenSeqs, e.Sequence)
		}

		cursor = result.NextAfter
		if !result.HasMore {
			break
		}
	}

	// Verify we got all 20 events in order
	if len(seenSeqs) != 20 {
		t.Fatalf("expected 20 sequences, got %d", len(seenSeqs))
	}

	for i, seq := range seenSeqs {
		if seq != int64(i+1) {
			t.Errorf("expected sequence %d at position %d, got %d", i+1, i, seq)
		}
	}
}

func TestCursorRepository_Replay_EmptySession(t *testing.T) {
	_, repo, cleanup := setupCursorTest(t, 0)
	defer cleanup()

	result, err := repo.Replay("session-1", 0, 100)
	if err != nil {
		t.Fatalf("Replay failed: %v", err)
	}

	if len(result.Events) != 0 {
		t.Errorf("expected 0 events, got %d", len(result.Events))
	}
}

func TestCursorRepository_Replay_LimitCapped(t *testing.T) {
	_, repo, cleanup := setupCursorTest(t, 0)
	defer cleanup()

	for i := int64(1); i <= 10; i++ {
		repo.AppendWithoutTx(&domain.Event{
			SessionID: "session-1", Type: "test", MessageID: "msg" + string(rune('a'+int(i))),
			SchemaVersion: 1, Payload: map[string]any{}, OccurredAt: time.Now(),
		})
	}

	// Request huge limit; should be capped to 500
	result, err := repo.Replay("session-1", 0, 10000)
	if err != nil {
		t.Fatalf("Replay failed: %v", err)
	}

	if len(result.Events) > 500 {
		t.Errorf("expected at most 500 events, got %d", len(result.Events))
	}
}

func TestCursorRepository_AppendWithoutTx_AssignsSequence(t *testing.T) {
	_, repo, cleanup := setupCursorTest(t, 0)
	defer cleanup()

	seq1, _ := repo.AppendWithoutTx(&domain.Event{
		SessionID: "session-1", Type: "test", MessageID: "msg1",
		SchemaVersion: 1, Payload: map[string]any{}, OccurredAt: time.Now(),
	})
	seq2, _ := repo.AppendWithoutTx(&domain.Event{
		SessionID: "session-1", Type: "test", MessageID: "msg2",
		SchemaVersion: 1, Payload: map[string]any{}, OccurredAt: time.Now(),
	})

	if seq1 != 1 {
		t.Errorf("expected first sequence 1, got %d", seq1)
	}
	if seq2 != 2 {
		t.Errorf("expected second sequence 2, got %d", seq2)
	}
}

func TestCursorRepository_AppendWithoutTx_UpdatesSessionLastSequence(t *testing.T) {
	db, repo, cleanup := setupCursorTest(t, 0)
	defer cleanup()

	sessRepo := NewSessionRepository(db)

	repo.AppendWithoutTx(&domain.Event{
		SessionID: "session-1", Type: "test", MessageID: "msg1",
		SchemaVersion: 1, Payload: map[string]any{}, OccurredAt: time.Now(),
	})

	session, _ := sessRepo.GetByID("session-1")
	if session.LastSequence != 1 {
		t.Errorf("expected session last_sequence 1, got %d", session.LastSequence)
	}
}

func TestCursorRepository_ValidateCursor(t *testing.T) {
	_, repo, cleanup := setupCursorTest(t, 0)
	defer cleanup()

	// Add 5 events
	for i := int64(1); i <= 5; i++ {
		repo.AppendWithoutTx(&domain.Event{
			SessionID: "session-1", Type: "test", MessageID: "msg" + string(rune('a'+int(i))),
			SchemaVersion: 1, Payload: map[string]any{}, OccurredAt: time.Now(),
		})
	}

	// Valid cursor
	err := repo.ValidateCursor("session-1", 3)
	if err != nil {
		t.Errorf("expected cursor 3 to be valid: %v", err)
	}

	// Cursor at last sequence is valid
	err = repo.ValidateCursor("session-1", 5)
	if err != nil {
		t.Errorf("expected cursor 5 to be valid: %v", err)
	}

	// Cursor beyond last is invalid
	err = repo.ValidateCursor("session-1", 10)
	if err == nil {
		t.Error("expected cursor 10 to be invalid")
	}

	// Negative cursor is invalid
	err = repo.ValidateCursor("session-1", -1)
	if err == nil {
		t.Error("expected negative cursor to be invalid")
	}
}

func TestCursorRepository_GetCursorMetadata(t *testing.T) {
	_, repo, cleanup := setupCursorTest(t, 0)
	defer cleanup()

	for i := int64(1); i <= 10; i++ {
		repo.AppendWithoutTx(&domain.Event{
			SessionID: "session-1", Type: "test", MessageID: "msg" + string(rune('a'+int(i))),
			SchemaVersion: 1, Payload: map[string]any{}, OccurredAt: time.Now(),
		})
	}

	meta, err := repo.GetCursorMetadata("session-1", 3)
	if err != nil {
		t.Fatalf("GetCursorMetadata failed: %v", err)
	}

	if meta.LastSequence != 10 {
		t.Errorf("expected last_sequence 10, got %d", meta.LastSequence)
	}
	if meta.TotalEvents != 10 {
		t.Errorf("expected total_events 10, got %d", meta.TotalEvents)
	}
	if meta.RemainingEvents != 7 {
		t.Errorf("expected remaining_events 7, got %d", meta.RemainingEvents)
	}
}

func TestCursorRepository_ConcurrentAppends(t *testing.T) {
	db, repo, cleanup := setupCursorTest(t, 0)
	defer cleanup()

	eventRepo := NewEventRepository(db)
	_ = eventRepo

	var wg sync.WaitGroup
	errCh := make(chan error, 20)

	// 20 concurrent appends
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_, err := repo.AppendWithoutTx(&domain.Event{
				SessionID:     "session-1",
				Type:          "test",
				MessageID:     "msg" + string(rune('A'+id)) + string(rune('A'+id)),
				SchemaVersion: 1,
				Payload:       map[string]any{},
				OccurredAt:    time.Now(),
			})
			if err != nil {
				errCh <- err
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent append failed: %v", err)
	}

	// Verify all 20 events were appended with unique sequences
	result, err := repo.Replay("session-1", 0, 500)
	if err != nil {
		t.Fatalf("Replay failed: %v", err)
	}

	if len(result.Events) != 20 {
		t.Errorf("expected 20 events, got %d", len(result.Events))
	}

	// Verify sequences are unique and ordered 1..20
	seen := make(map[int64]bool)
	for _, e := range result.Events {
		if seen[e.Sequence] {
			t.Errorf("duplicate sequence: %d", e.Sequence)
		}
		seen[e.Sequence] = true
	}

	expectedSeq := int64(1)
	for i := int64(1); i <= 20; i++ {
		if !seen[i] {
			t.Errorf("missing sequence %d", i)
		}
	}
	_ = expectedSeq
}

func TestCursorRepository_PurgeExpired(t *testing.T) {
	_, repo, cleanup := setupCursorTest(t, 0)
	defer cleanup()

	// Add an old event and a recent one
	repo.AppendWithoutTx(&domain.Event{
		SessionID: "session-1", Type: "test", MessageID: "msg-old",
		SchemaVersion: 1, Payload: map[string]any{}, OccurredAt: time.Now().Add(-2 * time.Hour),
	})
	repo.AppendWithoutTx(&domain.Event{
		SessionID: "session-1", Type: "test", MessageID: "msg-new",
		SchemaVersion: 1, Payload: map[string]any{}, OccurredAt: time.Now(),
	})

	// Purge events older than 1 hour ago
	removed, err := repo.PurgeExpired(time.Now().Add(-1 * time.Hour))
	if err != nil {
		t.Fatalf("PurgeExpired failed: %v", err)
	}

	if removed != 1 {
		t.Errorf("expected 1 removed, got %d", removed)
	}

	// Verify only recent event remains
	result, _ := repo.Replay("session-1", 0, 100)
	if len(result.Events) != 1 {
		t.Errorf("expected 1 remaining event, got %d", len(result.Events))
	}
}
