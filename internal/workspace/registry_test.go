package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/code-all-remote/car/internal/domain"
	"github.com/code-all-remote/car/internal/storage"
)

func setupTestRegistry(t *testing.T) (*Registry, string, func()) {
	db, err := storage.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	tmpDir := t.TempDir()

	reg := NewRegistry(db, tmpDir)

	cleanup := func() {
		db.Close()
	}

	return reg, tmpDir, cleanup
}

func TestRegistry_Register_Basic(t *testing.T) {
	reg, tmpDir, cleanup := setupTestRegistry(t)
	defer cleanup()

	// Create test directory
	wsPath := filepath.Join(tmpDir, "ws1")
	if err := os.Mkdir(wsPath, 0o755); err != nil {
		t.Fatalf("failed to create test dir: %v", err)
	}

	ws := &domain.Workspace{
		ID:          "ws-1",
		DisplayName: "Test Workspace 1",
		Path:        wsPath,
	}

	result, err := reg.Register(ws)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	if result.Workspace.ID != "ws-1" {
		t.Errorf("expected ID ws-1, got %s", result.Workspace.ID)
	}
	if result.Workspace.DisplayName != "Test Workspace 1" {
		t.Errorf("expected display name 'Test Workspace 1', got %s", result.Workspace.DisplayName)
	}
}

func TestRegistry_Register_DuplicateID(t *testing.T) {
	reg, tmpDir, cleanup := setupTestRegistry(t)
	defer cleanup()

	wsPath := filepath.Join(tmpDir, "ws1")
	os.Mkdir(wsPath, 0o755)

	ws := &domain.Workspace{
		ID: "ws-1", DisplayName: "Test", Path: wsPath,
	}

	// First registration should succeed
	_, err := reg.Register(ws)
	if err != nil {
		t.Fatalf("first Register failed: %v", err)
	}

	// Second registration with same ID should fail
	_, err = reg.Register(ws)
	if err == nil {
		t.Error("expected error for duplicate ID, got nil")
	}
}

func TestRegistry_Register_DuplicatePath(t *testing.T) {
	reg, tmpDir, cleanup := setupTestRegistry(t)
	defer cleanup()

	wsPath := filepath.Join(tmpDir, "ws1")
	os.Mkdir(wsPath, 0o755)

	ws1 := &domain.Workspace{ID: "ws-1", DisplayName: "First", Path: wsPath}
	ws2 := &domain.Workspace{ID: "ws-2", DisplayName: "Second", Path: wsPath}

	_, err := reg.Register(ws1)
	if err != nil {
		t.Fatalf("first Register failed: %v", err)
	}

	_, err = reg.Register(ws2)
	if err == nil {
		t.Error("expected error for duplicate path, got nil")
	}
}

func TestRegistry_Register_EmptyID(t *testing.T) {
	reg, tmpDir, cleanup := setupTestRegistry(t)
	defer cleanup()

	wsPath := filepath.Join(tmpDir, "ws1")
	os.Mkdir(wsPath, 0o755)

	ws := &domain.Workspace{
		ID:          "",
		DisplayName: "Test",
		Path:        wsPath,
	}

	_, err := reg.Register(ws)
	if err == nil {
		t.Error("expected error for empty ID, got nil")
	}
}

func TestRegistry_Register_EmptyPath(t *testing.T) {
	reg, _, cleanup := setupTestRegistry(t)
	defer cleanup()

	ws := &domain.Workspace{
		ID:          "ws-1",
		DisplayName: "Test",
		Path:        "",
	}

	_, err := reg.Register(ws)
	if err == nil {
		t.Error("expected error for empty path, got nil")
	}
}

func TestRegistry_Register_NonExistentPath(t *testing.T) {
	reg, tmpDir, cleanup := setupTestRegistry(t)
	defer cleanup()

	ws := &domain.Workspace{
		ID:          "ws-1",
		DisplayName: "Test",
		Path:        filepath.Join(tmpDir, "nonexistent"),
	}

	_, err := reg.Register(ws)
	if err == nil {
		t.Error("expected error for nonexistent path, got nil")
	}
}

func TestRegistry_Register_PathEscape(t *testing.T) {
	reg, tmpDir, cleanup := setupTestRegistry(t)
	defer cleanup()

	// Create directory outside workspace dir
	outsidePath := filepath.Join(tmpDir, "..", "outside")

	ws := &domain.Workspace{
		ID:          "ws-1",
		DisplayName: "Test",
		Path:        outsidePath,
	}

	_, err := reg.Register(ws)
	if err == nil {
		t.Error("expected error for path escape, got nil")
	}
}

func TestRegistry_Register_SymlinkEscape(t *testing.T) {
	reg, tmpDir, cleanup := setupTestRegistry(t)
	defer cleanup()

	// Create a directory inside the workspace root.
	insidePath := filepath.Join(tmpDir, "inside")
	if err := os.Mkdir(insidePath, 0o755); err != nil {
		t.Fatalf("failed to create inside dir: %v", err)
	}

	// Create a directory genuinely OUTSIDE the workspace root.
	outsideParent := t.TempDir()
	outsidePath := filepath.Join(outsideParent, "outside")
	if err := os.Mkdir(outsidePath, 0o755); err != nil {
		t.Fatalf("failed to create outside dir: %v", err)
	}

	// Create symlink inside pointing outside the workspace root.
	symlinkPath := filepath.Join(insidePath, "link")
	if err := os.Symlink(outsidePath, symlinkPath); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	ws := &domain.Workspace{
		ID:          "ws-1",
		DisplayName: "Test",
		Path:        symlinkPath,
	}

	_, err := reg.Register(ws)
	if err == nil {
		t.Error("expected error for symlink escape, got nil")
	}
}

func TestRegistry_Register_RelativePath(t *testing.T) {
	reg, tmpDir, cleanup := setupTestRegistry(t)
	defer cleanup()

	// Create directory
	wsPath := filepath.Join(tmpDir, "ws1")
	os.Mkdir(wsPath, 0o755)

	// Change to that directory
	originalDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(originalDir)

	ws := &domain.Workspace{
		ID:          "ws-1",
		DisplayName: "Test",
		Path:        "ws1", // Relative path
	}

	result, err := reg.Register(ws)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Should be resolved to absolute
	if !filepath.IsAbs(result.Workspace.Path) {
		t.Errorf("expected absolute path, got %s", result.Workspace.Path)
	}
}

func TestRegistry_Unregister(t *testing.T) {
	reg, tmpDir, cleanup := setupTestRegistry(t)
	defer cleanup()

	wsPath := filepath.Join(tmpDir, "ws1")
	os.Mkdir(wsPath, 0o755)

	ws := &domain.Workspace{ID: "ws-1", DisplayName: "Test", Path: wsPath}
	reg.Register(ws)

	err := reg.Unregister("ws-1")
	if err != nil {
		t.Fatalf("Unregister failed: %v", err)
	}

	// Should not be found
	_, err = reg.GetByID("ws-1")
	if err == nil {
		t.Error("expected error after unregister, got nil")
	}
}

func TestRegistry_GetByID(t *testing.T) {
	reg, tmpDir, cleanup := setupTestRegistry(t)
	defer cleanup()

	wsPath := filepath.Join(tmpDir, "ws1")
	os.Mkdir(wsPath, 0o755)

	ws := &domain.Workspace{ID: "ws-1", DisplayName: "Test Workspace", Path: wsPath}
	reg.Register(ws)

	retrieved, err := reg.GetByID("ws-1")
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if retrieved.DisplayName != "Test Workspace" {
		t.Errorf("expected display name 'Test Workspace', got %s", retrieved.DisplayName)
	}
}

func TestRegistry_GetByID_NotFound(t *testing.T) {
	reg, _, cleanup := setupTestRegistry(t)
	defer cleanup()

	_, err := reg.GetByID("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent ID, got nil")
	}
}

func TestRegistry_GetByPath(t *testing.T) {
	reg, tmpDir, cleanup := setupTestRegistry(t)
	defer cleanup()

	wsPath := filepath.Join(tmpDir, "ws1")
	os.Mkdir(wsPath, 0o755)

	ws := &domain.Workspace{ID: "ws-1", DisplayName: "Test", Path: wsPath}
	reg.Register(ws)

	retrieved, err := reg.GetByPath(wsPath)
	if err != nil {
		t.Fatalf("GetByPath failed: %v", err)
	}

	if retrieved.ID != "ws-1" {
		t.Errorf("expected ID ws-1, got %s", retrieved.ID)
	}
}

func TestRegistry_GetAll(t *testing.T) {
	reg, tmpDir, cleanup := setupTestRegistry(t)
	defer cleanup()

	// Create multiple workspaces
	for i := 0; i < 3; i++ {
		wsPath := filepath.Join(tmpDir, string(rune('a'+i)))
		os.Mkdir(wsPath, 0o755)

		ws := &domain.Workspace{
			ID:          string(rune('a' + i)),
			DisplayName: "Workspace " + string(rune('a'+i)),
			Path:        wsPath,
		}
		reg.Register(ws)
	}

	all, err := reg.GetAll()
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}

	if len(all) != 3 {
		t.Errorf("expected 3 workspaces, got %d", len(all))
	}
}

func TestRegistry_IsRegistered(t *testing.T) {
	reg, tmpDir, cleanup := setupTestRegistry(t)
	defer cleanup()

	wsPath := filepath.Join(tmpDir, "ws1")
	os.Mkdir(wsPath, 0o755)

	ws := &domain.Workspace{ID: "ws-1", DisplayName: "Test", Path: wsPath}
	reg.Register(ws)

	if !reg.IsRegistered("ws-1") {
		t.Error("expected ws-1 to be registered")
	}

	if reg.IsRegistered("nonexistent") {
		t.Error("expected nonexistent to not be registered")
	}
}

func TestRegistry_ValidatePath(t *testing.T) {
	reg, tmpDir, cleanup := setupTestRegistry(t)
	defer cleanup()

	wsPath := filepath.Join(tmpDir, "ws1")
	os.Mkdir(wsPath, 0o755)

	// Valid path
	err := reg.ValidatePath(wsPath)
	if err != nil {
		t.Errorf("expected valid path: %v", err)
	}

	// Empty path
	err = reg.ValidatePath("")
	if err == nil {
		t.Error("expected error for empty path")
	}

	// Nonexistent path
	err = reg.ValidatePath(filepath.Join(tmpDir, "nonexistent"))
	if err == nil {
		t.Error("expected error for nonexistent path")
	}
}

func TestRegistry_ValidateAllowedAdapters(t *testing.T) {
	reg, tmpDir, cleanup := setupTestRegistry(t)
	defer cleanup()

	wsPath := filepath.Join(tmpDir, "ws1")
	os.Mkdir(wsPath, 0o755)

	// Valid adapters
	ws := &domain.Workspace{
		ID:              "ws-1",
		DisplayName:     "Test",
		Path:            wsPath,
		AllowedAdapters: []string{"claude-code", "fake-adapter"},
	}
	_, err := reg.Register(ws)
	if err != nil {
		t.Fatalf("Register with valid adapters failed: %v", err)
	}

	// Unregister for next test
	reg.Unregister("ws-1")

	// Invalid adapter
	ws2 := &domain.Workspace{
		ID:              "ws-2",
		DisplayName:     "Test",
		Path:            wsPath,
		AllowedAdapters: []string{"unknown-adapter"},
	}
	_, err = reg.Register(ws2)
	if err == nil {
		t.Error("expected error for unknown adapter, got nil")
	}
}

func TestRegistry_LoadFromConfig(t *testing.T) {
	reg, tmpDir, cleanup := setupTestRegistry(t)
	defer cleanup()

	// Create directories
	for i := 0; i < 3; i++ {
		wsPath := filepath.Join(tmpDir, string(rune('a'+i)))
		os.Mkdir(wsPath, 0o755)
	}

	configs := []domain.Workspace{
		{ID: "a", DisplayName: "Workspace A", Path: filepath.Join(tmpDir, "a")},
		{ID: "b", DisplayName: "Workspace B", Path: filepath.Join(tmpDir, "b")},
		{ID: "c", DisplayName: "Workspace C", Path: filepath.Join(tmpDir, "c")},
	}

	registered, errors := reg.LoadFromConfig(configs)

	if len(registered) != 3 {
		t.Errorf("expected 3 registered, got %d", len(registered))
	}
	if len(errors) != 0 {
		t.Errorf("expected 0 errors, got %d", len(errors))
	}
}

func TestRegistry_Stats(t *testing.T) {
	reg, tmpDir, cleanup := setupTestRegistry(t)
	defer cleanup()

	// Create workspaces
	for i := 0; i < 5; i++ {
		wsPath := filepath.Join(tmpDir, string(rune('a'+i)))
		os.Mkdir(wsPath, 0o755)
		ws := &domain.Workspace{ID: string(rune('a' + i)), DisplayName: "Test", Path: wsPath}
		reg.Register(ws)
	}

	stats := reg.Stats()
	if stats.Total != 5 {
		t.Errorf("expected 5 workspaces, got %d", stats.Total)
	}
}
