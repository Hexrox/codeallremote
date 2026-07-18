package approval

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/code-all-remote/car/internal/domain"
	"github.com/code-all-remote/car/internal/storage"
)

// createSessionForApprovalTests creates a workspace+session so that approvals
// can satisfy the foreign key on approvals.session_id.
func createSessionForApprovalTests(t *testing.T, db *storage.DB) {
	t.Helper()
	wsRepo := storage.NewWorkspaceRepository(db)
	if err := wsRepo.Create(&domain.Workspace{
		ID: "ws-1", DisplayName: "Test", Path: "/tmp/ws-1",
	}); err != nil {
		t.Fatalf("failed to create workspace: %v", err)
	}
	sessRepo := storage.NewSessionRepository(db)
	if err := sessRepo.CreateSession(&domain.Session{
		ID: "session-1", WorkspaceID: "ws-1", AdapterID: "claude-code",
		State: domain.SessionStateActive,
	}); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
}

func TestMemoryStore_CreateAndGet(t *testing.T) {
	store := NewMemoryStore()

	approval := &domain.Approval{
		ID:                   "approval-1",
		SessionID:            "session-1",
		Category:             "file_write",
		State:                domain.ApprovalStatePending,
		ActionKind:           "write",
		HumanReadableContext: "Test approval",
		CreatedAt:            time.Now(),
		ExpiresAt:            time.Now().Add(5 * time.Minute),
	}

	err := store.Create(approval)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	retrieved, ok := store.GetApproval("approval-1")
	if !ok {
		t.Fatal("expected approval to exist")
	}
	if retrieved.ID != approval.ID {
		t.Errorf("expected ID %s, got %s", approval.ID, retrieved.ID)
	}
}

func TestMemoryStore_Update(t *testing.T) {
	store := NewMemoryStore()

	approval := &domain.Approval{
		ID: "approval-1", SessionID: "session-1", Category: "test",
		State: domain.ApprovalStatePending, ActionKind: "exec",
		HumanReadableContext: "Test",
		CreatedAt:            time.Now(), ExpiresAt: time.Now().Add(5 * time.Minute),
	}
	store.Create(approval)

	// Update state
	approval.State = domain.ApprovalStateApproved
	err := store.Update(approval)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	retrieved, _ := store.GetApproval("approval-1")
	if retrieved.State != domain.ApprovalStateApproved {
		t.Errorf("expected state approved, got %s", retrieved.State)
	}
}

func TestMemoryStore_Update_NotFound(t *testing.T) {
	store := NewMemoryStore()

	approval := &domain.Approval{ID: "nonexistent"}
	err := store.Update(approval)
	if err == nil {
		t.Error("expected error for nonexistent approval, got nil")
	}
}

func TestMemoryStore_Delete(t *testing.T) {
	store := NewMemoryStore()

	approval := &domain.Approval{ID: "approval-1"}
	store.Create(approval)

	err := store.Delete("approval-1")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, ok := store.GetApproval("approval-1")
	if ok {
		t.Error("expected approval to be deleted")
	}
}

func TestMemoryStore_GetPendingBySession(t *testing.T) {
	store := NewMemoryStore()

	// Create approvals for different sessions and states
	approvals := []*domain.Approval{
		{ID: "p1", SessionID: "s1", State: domain.ApprovalStatePending},
		{ID: "p2", SessionID: "s1", State: domain.ApprovalStatePending},
		{ID: "d1", SessionID: "s1", State: domain.ApprovalStateApproved},
		{ID: "p3", SessionID: "s2", State: domain.ApprovalStatePending},
	}

	for _, a := range approvals {
		a.Category = "test"
		a.ActionKind = "exec"
		a.HumanReadableContext = "Test"
		a.CreatedAt = time.Now()
		a.ExpiresAt = time.Now().Add(5 * time.Minute)
		store.Create(a)
	}

	pending, _ := store.GetPendingBySession("s1")
	if len(pending) != 2 {
		t.Errorf("expected 2 pending for s1, got %d", len(pending))
	}
}

func TestMemoryStore_GetAll(t *testing.T) {
	store := NewMemoryStore()

	for i := 0; i < 5; i++ {
		store.Create(&domain.Approval{
			ID: string(rune('a' + i)), SessionID: "s1",
			Category: "test", ActionKind: "exec", HumanReadableContext: "Test",
			CreatedAt: time.Now(), ExpiresAt: time.Now().Add(5 * time.Minute),
		})
	}

	all := store.GetAll()
	if len(all) != 5 {
		t.Errorf("expected 5 approvals, got %d", len(all))
	}
}

func TestDBStore_Integration(t *testing.T) {
	db, err := storage.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	store := NewDBStore(db)
	createSessionForApprovalTests(t, db)

	payloadJSON, _ := json.Marshal(map[string]any{"path": "/test.txt"})
	approval := &domain.Approval{
		ID:                   "approval-1",
		SessionID:            "session-1",
		Category:             "file_write",
		State:                domain.ApprovalStatePending,
		ActionKind:           "write",
		HumanReadableContext: "Test approval",
		StructuredPayload:    string(payloadJSON),
		CreatedAt:            time.Now(),
		ExpiresAt:            time.Now().Add(5 * time.Minute),
	}

	err = store.Create(approval)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	retrieved, err := store.GetByID("approval-1")
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if retrieved.ID != approval.ID {
		t.Errorf("expected ID %s, got %s", approval.ID, retrieved.ID)
	}
	if retrieved.Category != approval.Category {
		t.Errorf("expected category %s, got %s", approval.Category, retrieved.Category)
	}
}

func TestDBStore_Update(t *testing.T) {
	db, err := storage.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	store := NewDBStore(db)
	createSessionForApprovalTests(t, db)

	approval := &domain.Approval{
		ID: "approval-1", SessionID: "session-1", Category: "test",
		State: domain.ApprovalStatePending, ActionKind: "exec",
		HumanReadableContext: "Test",
		CreatedAt:            time.Now(), ExpiresAt: time.Now().Add(5 * time.Minute),
	}
	store.Create(approval)

	// Update
	approval.State = domain.ApprovalStateApproved
	now := time.Now()
	approval.DecidedAt = &now
	reason := "approved"
	approval.DecisionReason = &reason

	err = store.Update(approval)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	retrieved, _ := store.GetByID("approval-1")
	if retrieved.State != domain.ApprovalStateApproved {
		t.Errorf("expected state approved, got %s", retrieved.State)
	}
}

func TestDBStore_GetPendingBySession(t *testing.T) {
	db, err := storage.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	store := NewDBStore(db)
	createSessionForApprovalTests(t, db)

	// Create approvals
	approvals := []*domain.Approval{
		{ID: "p1", SessionID: "session-1", State: domain.ApprovalStatePending},
		{ID: "p2", SessionID: "session-1", State: domain.ApprovalStatePending},
		{ID: "d1", SessionID: "session-1", State: domain.ApprovalStateApproved},
	}

	for _, a := range approvals {
		a.Category = "test"
		a.ActionKind = "exec"
		a.HumanReadableContext = "Test"
		a.CreatedAt = time.Now()
		a.ExpiresAt = time.Now().Add(5 * time.Minute)
		store.Create(a)
	}

	pending, err := store.GetPendingBySession("session-1")
	if err != nil {
		t.Fatalf("GetPendingBySession failed: %v", err)
	}
	if len(pending) != 2 {
		t.Errorf("expected 2 pending, got %d", len(pending))
	}
}

func TestDBStore_GetNotFound(t *testing.T) {
	db, err := storage.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	store := NewDBStore(db)
	createSessionForApprovalTests(t, db)

	_, err = store.GetByID("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent approval, got nil")
	}
}

func TestDBStore_Delete(t *testing.T) {
	db, err := storage.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	store := NewDBStore(db)
	createSessionForApprovalTests(t, db)

	approval := &domain.Approval{
		ID: "approval-1", SessionID: "s1", Category: "test",
		State: domain.ApprovalStatePending, ActionKind: "exec",
		HumanReadableContext: "Test",
		CreatedAt:            time.Now(), ExpiresAt: time.Now().Add(5 * time.Minute),
	}
	store.Create(approval)

	err = store.Delete("approval-1")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = store.GetByID("approval-1")
	if err == nil {
		t.Error("expected error for deleted approval, got nil")
	}
}

func TestApprovalBridge_WithDBStore(t *testing.T) {
	db, err := storage.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	store := NewDBStore(db)
	createSessionForApprovalTests(t, db)
	clock := NewMockClock()
	bridge := NewApprovalBridge(store, clock)
	defer close(bridge.decisionCh)

	ctx := testContext{}
	_, err = bridge.Request(ctx, Request{
		ID: "approval-1", SessionID: "session-1", Category: "test",
		ActionKind: "exec", HumanReadableContext: "Test approval",
	})
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Verify persisted
	_, err = store.GetByID("approval-1")
	if err != nil {
		t.Errorf("approval should be persisted: %v", err)
	}
}

// testContext implements context.Context for testing
type testContext struct{}

func (testContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (testContext) Done() <-chan struct{}       { return nil }
func (testContext) Err() error                  { return nil }
func (testContext) Value(key any) any           { return nil }
