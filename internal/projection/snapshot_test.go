package projection

import (
	"testing"
	"time"

	"github.com/code-all-remote/car/internal/domain"
	"github.com/code-all-remote/car/internal/storage"
)

func setupTestDB(t *testing.T) (*storage.DB, func()) {
	db, err := storage.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	cleanup := func() { db.Close() }
	return db, cleanup
}

func createTestSession(t *testing.T, db *storage.DB, id string) *domain.Session {
	wsRepo := storage.NewWorkspaceRepository(db)
	wsRepo.Create(&domain.Workspace{ID: "ws-1", DisplayName: "Test", Path: "/tmp/ws1"})

	sessRepo := storage.NewSessionRepository(db)
	session := &domain.Session{
		ID:          id,
		WorkspaceID: "ws-1",
		AdapterID:   "claude-code",
		State:       domain.SessionStateActive,
	}
	if err := sessRepo.CreateSession(session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	return session
}

func TestSnapshotProjection_GetSnapshot(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	createTestSession(t, db, "session-1")

	sessRepo := storage.NewSessionRepository(db)
	sessRepo.UpdateLastSequence("session-1", 5)

	proj := NewSnapshotProjection(db)

	snap, err := proj.GetSnapshot("session-1")
	if err != nil {
		t.Fatalf("GetSnapshot failed: %v", err)
	}

	if snap.Session.ID != "session-1" {
		t.Errorf("expected session ID session-1, got %s", snap.Session.ID)
	}
	if snap.LastSequence != 5 {
		t.Errorf("expected last_sequence 5, got %d", snap.LastSequence)
	}
}

func TestSnapshotProjection_GetSnapshot_NotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	proj := NewSnapshotProjection(db)

	_, err := proj.GetSnapshot("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent session, got nil")
	}
}

func TestSnapshotProjection_GetSnapshot_WithRun(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	session := createTestSession(t, db, "session-1")

	sessRepo := storage.NewSessionRepository(db)
	run := &domain.Run{
		ID:        "run-1",
		SessionID: session.ID,
		State:     domain.RunStateActive,
	}
	sessRepo.CreateRun(run)

	proj := NewSnapshotProjection(db)

	snap, err := proj.GetSnapshot("session-1")
	if err != nil {
		t.Fatalf("GetSnapshot failed: %v", err)
	}

	if snap.ActiveRun == nil {
		t.Fatal("expected active run to be set")
	}
	if snap.ActiveRun.ID != "run-1" {
		t.Errorf("expected run ID run-1, got %s", snap.ActiveRun.ID)
	}
}

func TestSnapshotProjection_GetSnapshot_WithPendingApproval(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	session := createTestSession(t, db, "session-1")

	sessRepo := storage.NewSessionRepository(db)
	approvalID := "approval-1"
	sessRepo.SetPendingApproval("session-1", &approvalID)

	// Insert approval directly
	now := time.Now()
	approval := &domain.Approval{
		ID:                   "approval-1",
		SessionID:            session.ID,
		Category:             "test",
		State:                domain.ApprovalStatePending,
		ActionKind:           "exec",
		HumanReadableContext: "Test approval",
		CreatedAt:            now,
		ExpiresAt:            now.Add(5 * time.Minute),
	}
	insertApproval(t, db, approval)

	proj := NewSnapshotProjection(db)
	snap, err := proj.GetSnapshot("session-1")
	if err != nil {
		t.Fatalf("GetSnapshot failed: %v", err)
	}

	if snap.PendingApproval == nil {
		t.Fatal("expected pending approval to be set")
	}
	if snap.PendingApproval.ID != "approval-1" {
		t.Errorf("expected approval ID approval-1, got %s", snap.PendingApproval.ID)
	}
}

func TestSnapshotProjection_Reconstruct(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	createTestSession(t, db, "session-1")

	// Add events
	eventRepo := storage.NewEventRepository(db)
	for i := int64(1); i <= 3; i++ {
		event := &domain.Event{
			SessionID:     "session-1",
			Sequence:      i,
			Type:          "test",
			MessageID:     "msg" + string(rune('a'+int(i))),
			SchemaVersion: 1,
			Payload:       map[string]any{"seq": i},
			OccurredAt:    time.Now(),
		}
		eventRepo.Append(event)
	}

	// Update session with correct sequence
	sessRepo := storage.NewSessionRepository(db)
	sessRepo.UpdateLastSequence("session-1", 3)

	proj := NewSnapshotProjection(db)

	snap, err := proj.Reconstruct("session-1")
	if err != nil {
		t.Fatalf("Reconstruct failed: %v", err)
	}

	if snap.LastSequence != 3 {
		t.Errorf("expected last_sequence 3, got %d", snap.LastSequence)
	}
}

func TestSnapshotProjection_Reconstruct_Inconsistency(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	createTestSession(t, db, "session-1")

	// Add 5 events
	eventRepo := storage.NewEventRepository(db)
	for i := int64(1); i <= 5; i++ {
		event := &domain.Event{
			SessionID:     "session-1",
			Sequence:      i,
			Type:          "test",
			MessageID:     "msg" + string(rune('a'+int(i))),
			SchemaVersion: 1,
			Payload:       map[string]any{"seq": i},
			OccurredAt:    time.Now(),
		}
		eventRepo.Append(event)
	}

	// Update session with WRONG sequence (out of sync)
	sessRepo := storage.NewSessionRepository(db)
	sessRepo.UpdateLastSequence("session-1", 2) // Should be 5

	proj := NewSnapshotProjection(db)

	// Reconstruct should detect the right sequence from events
	snap, err := proj.Reconstruct("session-1")
	if err != nil {
		t.Fatalf("Reconstruct failed: %v", err)
	}

	if snap.LastSequence != 5 {
		t.Errorf("expected reconstructed sequence 5, got %d", snap.LastSequence)
	}
}

func TestSnapshotProjection_IsConsistent(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	createTestSession(t, db, "session-1")

	eventRepo := storage.NewEventRepository(db)
	for i := int64(1); i <= 3; i++ {
		event := &domain.Event{
			SessionID:     "session-1",
			Sequence:      i,
			Type:          "test",
			MessageID:     "msg" + string(rune('a'+int(i))),
			SchemaVersion: 1,
			Payload:       map[string]any{},
			OccurredAt:    time.Now(),
		}
		eventRepo.Append(event)
	}

	// Set correct sequence
	sessRepo := storage.NewSessionRepository(db)
	sessRepo.UpdateLastSequence("session-1", 3)

	proj := NewSnapshotProjection(db)

	// Should be consistent
	consistent, err := proj.IsConsistent("session-1")
	if err != nil {
		t.Fatalf("IsConsistent failed: %v", err)
	}
	if !consistent {
		t.Error("expected to be consistent")
	}

	// Make it inconsistent
	sessRepo.UpdateLastSequence("session-1", 5)

	consistent, _ = proj.IsConsistent("session-1")
	if consistent {
		t.Error("expected to be inconsistent")
	}
}

func TestSnapshotProjection_FixConsistency(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	createTestSession(t, db, "session-1")

	eventRepo := storage.NewEventRepository(db)
	for i := int64(1); i <= 3; i++ {
		event := &domain.Event{
			SessionID:     "session-1",
			Sequence:      i,
			Type:          "test",
			MessageID:     "msg" + string(rune('a'+int(i))),
			SchemaVersion: 1,
			Payload:       map[string]any{},
			OccurredAt:    time.Now(),
		}
		eventRepo.Append(event)
	}

	// Make inconsistent
	sessRepo := storage.NewSessionRepository(db)
	sessRepo.UpdateLastSequence("session-1", 1)

	proj := NewSnapshotProjection(db)

	// Fix it
	err := proj.FixConsistency("session-1")
	if err != nil {
		t.Fatalf("FixConsistency failed: %v", err)
	}

	// Should now be consistent
	consistent, _ := proj.IsConsistent("session-1")
	if !consistent {
		t.Error("expected to be consistent after fix")
	}
}

func TestSnapshotProjection_ReconstructAll(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create multiple sessions
	for i := 0; i < 3; i++ {
		createTestSession(t, db, "session-"+string(rune('a'+i)))
	}

	proj := NewSnapshotProjection(db)

	snapshots, err := proj.ReconstructAll()
	if err != nil {
		t.Fatalf("ReconstructAll failed: %v", err)
	}

	if len(snapshots) != 3 {
		t.Errorf("expected 3 snapshots, got %d", len(snapshots))
	}
}

func TestSnapshotProjection_GetSnapshotMultiple(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	createTestSession(t, db, "session-a")
	createTestSession(t, db, "session-b")

	proj := NewSnapshotProjection(db)

	snapshots, err := proj.GetSnapshotMultiple([]string{"session-a", "session-b", "nonexistent"})
	if err != nil {
		t.Fatalf("GetSnapshotMultiple failed: %v", err)
	}

	if len(snapshots) != 2 {
		t.Errorf("expected 2 snapshots, got %d", len(snapshots))
	}
}

// insertApproval inserts an approval directly for testing.
func insertApproval(t *testing.T, db *storage.DB, approval *domain.Approval) {
	_, err := db.Exec(`
		INSERT INTO approvals (id, session_id, category, state, action_kind, human_readable_context, structured_payload, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, approval.ID, approval.SessionID, approval.Category, approval.State,
		approval.ActionKind, approval.HumanReadableContext,
		approval.StructuredPayload, approval.CreatedAt, approval.ExpiresAt)
	if err != nil {
		t.Fatalf("failed to insert approval: %v", err)
	}
}
