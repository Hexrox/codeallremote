package storage

import (
	"testing"

	"github.com/code-all-remote/car/internal/domain"
)

func TestSessionRepository_CreateAndGet(t *testing.T) {
	db, err := Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Create workspace first
	wsRepo := NewWorkspaceRepository(db)
	ws := &domain.Workspace{
		ID:          "ws-1",
		DisplayName: "Test Workspace",
		Path:        "/tmp/test-ws",
	}
	if err := wsRepo.Create(ws); err != nil {
		t.Fatalf("failed to create workspace: %v", err)
	}

	repo := NewSessionRepository(db)
	session := &domain.Session{
		ID:          "session-1",
		WorkspaceID: "ws-1",
		AdapterID:   "claude-code",
		State:       domain.SessionStateCreated,
		Title:       "Test Session",
	}

	// Create session
	if err := repo.CreateSession(session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Get session
	retrieved, err := repo.GetByID("session-1")
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}

	if retrieved.ID != session.ID {
		t.Errorf("expected ID %s, got %s", session.ID, retrieved.ID)
	}
	if retrieved.WorkspaceID != session.WorkspaceID {
		t.Errorf("expected workspace_id %s, got %s", session.WorkspaceID, retrieved.WorkspaceID)
	}
	if retrieved.State != session.State {
		t.Errorf("expected state %s, got %s", session.State, retrieved.State)
	}
}

func TestSessionRepository_GetNotFound(t *testing.T) {
	db, err := Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	repo := NewSessionRepository(db)

	_, err = repo.GetByID("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent session, got nil")
	}
}

func TestSessionRepository_UpdateState(t *testing.T) {
	db, err := Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	wsRepo := NewWorkspaceRepository(db)
	wsRepo.Create(&domain.Workspace{ID: "ws-1", DisplayName: "Test", Path: "/tmp/ws1"})

	repo := NewSessionRepository(db)
	session := &domain.Session{
		ID: "session-1", WorkspaceID: "ws-1", AdapterID: "claude-code",
		State: domain.SessionStateCreated,
	}
	repo.CreateSession(session)

	// Update state without expected state
	ok, err := repo.UpdateState("session-1", domain.SessionStateActive, nil)
	if err != nil {
		t.Fatalf("UpdateState failed: %v", err)
	}
	if !ok {
		t.Error("expected update to succeed")
	}

	// Verify state changed
	retrieved, _ := repo.GetByID("session-1")
	if retrieved.State != domain.SessionStateActive {
		t.Errorf("expected state %s, got %s", domain.SessionStateActive, retrieved.State)
	}
}

func TestSessionRepository_UpdateState_WithExpected(t *testing.T) {
	db, err := Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	wsRepo := NewWorkspaceRepository(db)
	wsRepo.Create(&domain.Workspace{ID: "ws-1", DisplayName: "Test", Path: "/tmp/ws1"})

	repo := NewSessionRepository(db)
	session := &domain.Session{
		ID: "session-1", WorkspaceID: "ws-1", AdapterID: "claude-code",
		State: domain.SessionStateCreated,
	}
	repo.CreateSession(session)

	// Update with wrong expected state should fail
	expected := domain.SessionStateStarting
	ok, err := repo.UpdateState("session-1", domain.SessionStateActive, &expected)
	if err != nil {
		t.Fatalf("UpdateState failed: %v", err)
	}
	if ok {
		t.Error("expected update to fail with wrong expected state")
	}

	// Update with correct expected state should succeed
	expected = domain.SessionStateCreated
	ok, err = repo.UpdateState("session-1", domain.SessionStateActive, &expected)
	if err != nil {
		t.Fatalf("UpdateState failed: %v", err)
	}
	if !ok {
		t.Error("expected update to succeed with correct expected state")
	}
}

func TestSessionRepository_UpdateLastSequence(t *testing.T) {
	db, err := Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	wsRepo := NewWorkspaceRepository(db)
	wsRepo.Create(&domain.Workspace{ID: "ws-1", DisplayName: "Test", Path: "/tmp/ws1"})

	repo := NewSessionRepository(db)
	session := &domain.Session{
		ID: "session-1", WorkspaceID: "ws-1", AdapterID: "claude-code",
		State: domain.SessionStateCreated, LastSequence: 0,
	}
	repo.CreateSession(session)

	// Update sequence
	if err := repo.UpdateLastSequence("session-1", 5); err != nil {
		t.Fatalf("UpdateLastSequence failed: %v", err)
	}

	retrieved, _ := repo.GetByID("session-1")
	if retrieved.LastSequence != 5 {
		t.Errorf("expected last_sequence 5, got %d", retrieved.LastSequence)
	}
}

func TestSessionRepository_SetPendingApproval(t *testing.T) {
	db, err := Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	wsRepo := NewWorkspaceRepository(db)
	wsRepo.Create(&domain.Workspace{ID: "ws-1", DisplayName: "Test", Path: "/tmp/ws1"})

	repo := NewSessionRepository(db)
	session := &domain.Session{
		ID: "session-1", WorkspaceID: "ws-1", AdapterID: "claude-code",
		State: domain.SessionStateActive,
	}
	repo.CreateSession(session)

	// Set pending approval
	approvalID := "approval-1"
	if err := repo.SetPendingApproval("session-1", &approvalID); err != nil {
		t.Fatalf("SetPendingApproval failed: %v", err)
	}

	retrieved, _ := repo.GetByID("session-1")
	if retrieved.PendingApproval == nil {
		t.Fatal("expected pending_approval_id to be set")
	}
	if *retrieved.PendingApproval != "approval-1" {
		t.Errorf("expected pending_approval_id approval-1, got %s", *retrieved.PendingApproval)
	}

	// Clear pending approval
	if err := repo.SetPendingApproval("session-1", nil); err != nil {
		t.Fatalf("SetPendingApproval clear failed: %v", err)
	}

	retrieved, _ = repo.GetByID("session-1")
	if retrieved.PendingApproval != nil {
		t.Errorf("expected pending_approval_id to be nil, got %v", *retrieved.PendingApproval)
	}
}

func TestSessionRepository_CreateAndGetRun(t *testing.T) {
	db, err := Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	wsRepo := NewWorkspaceRepository(db)
	wsRepo.Create(&domain.Workspace{ID: "ws-1", DisplayName: "Test", Path: "/tmp/ws1"})

	sessRepo := NewSessionRepository(db)
	session := &domain.Session{
		ID: "session-1", WorkspaceID: "ws-1", AdapterID: "claude-code",
		State: domain.SessionStateActive,
	}
	sessRepo.CreateSession(session)

	run := &domain.Run{
		ID:        "run-1",
		SessionID: "session-1",
		State:     domain.RunStateActive,
	}

	if err := sessRepo.CreateRun(run); err != nil {
		t.Fatalf("failed to create run: %v", err)
	}

	retrieved, err := sessRepo.GetActiveRun("session-1")
	if err != nil {
		t.Fatalf("failed to get run: %v", err)
	}

	if retrieved.ID != run.ID {
		t.Errorf("expected run ID %s, got %s", run.ID, retrieved.ID)
	}
}

func TestSessionRepository_GetByWorkspaceID(t *testing.T) {
	db, err := Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	wsRepo := NewWorkspaceRepository(db)
	wsRepo.Create(&domain.Workspace{ID: "ws-1", DisplayName: "Test", Path: "/tmp/ws1"})

	repo := NewSessionRepository(db)

	// Create multiple sessions
	for i := 1; i <= 3; i++ {
		s := &domain.Session{
			ID:          string(rune('a' + i)),
			WorkspaceID: "ws-1",
			AdapterID:   "claude-code",
			State:       domain.SessionStateCreated,
		}
		repo.CreateSession(s)
	}

	sessions, err := repo.GetByWorkspaceID("ws-1")
	if err != nil {
		t.Fatalf("GetByWorkspaceID failed: %v", err)
	}

	if len(sessions) != 3 {
		t.Errorf("expected 3 sessions, got %d", len(sessions))
	}
}

func TestWorkspaceRepository_CreateAndGet(t *testing.T) {
	db, err := Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	repo := NewWorkspaceRepository(db)
	ws := &domain.Workspace{
		ID:              "ws-1",
		DisplayName:     "Test Workspace",
		Path:            "/tmp/test-ws",
		AllowedAdapters: []string{"claude-code"},
		ExecutionPolicy: domain.ExecutionPolicy{
			AllowNetworkAccess: false,
			AllowWrites:        true,
		},
	}

	if err := repo.Create(ws); err != nil {
		t.Fatalf("failed to create workspace: %v", err)
	}

	// Get by ID
	retrieved, err := repo.GetByID("ws-1")
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if retrieved.DisplayName != ws.DisplayName {
		t.Errorf("expected display_name %s, got %s", ws.DisplayName, retrieved.DisplayName)
	}
	if retrieved.Path != ws.Path {
		t.Errorf("expected path %s, got %s", ws.Path, retrieved.Path)
	}

	// Get by path
	retrieved, err = repo.GetByPath("/tmp/test-ws")
	if err != nil {
		t.Fatalf("GetByPath failed: %v", err)
	}

	if retrieved.ID != ws.ID {
		t.Errorf("expected ID %s, got %s", ws.ID, retrieved.ID)
	}
}

func TestWorkspaceRepository_DuplicatePath(t *testing.T) {
	db, err := Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	repo := NewWorkspaceRepository(db)
	ws1 := &domain.Workspace{ID: "ws-1", DisplayName: "First", Path: "/tmp/ws1"}
	ws2 := &domain.Workspace{ID: "ws-2", DisplayName: "Second", Path: "/tmp/ws1"}

	if err := repo.Create(ws1); err != nil {
		t.Fatalf("failed to create first workspace: %v", err)
	}

	// Duplicate path should fail
	if err := repo.Create(ws2); err == nil {
		t.Error("expected error for duplicate path, got nil")
	}
}

func TestWorkspaceRepository_GetAll(t *testing.T) {
	db, err := Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	repo := NewWorkspaceRepository(db)

	// Create multiple workspaces
	workspaces := []domain.Workspace{
		{ID: "ws-a", DisplayName: "Alpha", Path: "/tmp/alpha"},
		{ID: "ws-b", DisplayName: "Beta", Path: "/tmp/beta"},
	}

	for _, ws := range workspaces {
		if err := repo.Create(&ws); err != nil {
			t.Fatalf("failed to create workspace: %v", err)
		}
	}

	all, err := repo.GetAll()
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}

	if len(all) != 2 {
		t.Errorf("expected 2 workspaces, got %d", len(all))
	}
}
