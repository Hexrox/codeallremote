// Package workspace provides workspace registration and management.
package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/code-all-remote/car/internal/domain"
	"github.com/code-all-remote/car/internal/storage"
)

// Registry manages workspace registration.
type Registry struct {
	mu           sync.RWMutex
	store        *storage.WorkspaceRepository
	workspaces   map[string]*domain.Workspace
	paths        map[string]string // path -> ID mapping
	workspaceDir string            // base directory for validation
}

// NewRegistry creates a new workspace registry.
func NewRegistry(db *storage.DB, workspaceDir string) *Registry {
	return &Registry{
		store:        storage.NewWorkspaceRepository(db),
		workspaces:   make(map[string]*domain.Workspace),
		paths:        make(map[string]string),
		workspaceDir: workspaceDir,
	}
}

// RegisterResult contains the result of a registration attempt.
type RegisterResult struct {
	Workspace *domain.Workspace
	Warning   string
}

// Register registers a new workspace.
func (r *Registry) Register(ws *domain.Workspace) (*RegisterResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Validate ID
	if ws.ID == "" {
		return nil, fmt.Errorf("workspace ID cannot be empty")
	}

	// Check for duplicate ID
	if _, exists := r.workspaces[ws.ID]; exists {
		return nil, fmt.Errorf("workspace ID '%s' is already registered", ws.ID)
	}

	// Check for duplicate ID in store
	existing, _ := r.store.GetByID(ws.ID)
	if existing != nil {
		return nil, fmt.Errorf("workspace ID '%s' is already registered", ws.ID)
	}

	// Validate path
	if ws.Path == "" {
		return nil, fmt.Errorf("workspace path cannot be empty")
	}

	// Resolve to canonical absolute path
	canonicalPath, err := r.resolveCanonicalPath(ws.Path)
	if err != nil {
		return nil, fmt.Errorf("resolving path: %w", err)
	}

	// Check for path escape (symlink attack prevention)
	if err := r.validatePathEscape(canonicalPath); err != nil {
		return nil, fmt.Errorf("path validation: %w", err)
	}

	// Check for duplicate path
	if existingID, exists := r.paths[canonicalPath]; exists {
		return nil, fmt.Errorf("path '%s' is already registered as workspace '%s'", canonicalPath, existingID)
	}

	// Check if path exists
	if _, err := os.Stat(canonicalPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("workspace path does not exist: %s", canonicalPath)
	}

	// Update workspace with canonical path
	ws.Path = canonicalPath

	// Validate allowed adapters
	if err := r.validateAllowedAdapters(ws.AllowedAdapters); err != nil {
		return nil, fmt.Errorf("invalid adapters: %w", err)
	}

	// Persist to store
	if err := r.store.Create(ws); err != nil {
		return nil, fmt.Errorf("persisting workspace: %w", err)
	}

	// Add to memory
	r.workspaces[ws.ID] = ws
	r.paths[canonicalPath] = ws.ID

	result := &RegisterResult{
		Workspace: ws,
	}

	// Add warning if path is outside recommended directory
	if r.workspaceDir != "" && !strings.HasPrefix(canonicalPath, r.workspaceDir) {
		result.Warning = fmt.Sprintf("workspace path is outside recommended directory '%s'", r.workspaceDir)
	}

	return result, nil
}

// resolveCanonicalPath resolves a path to its canonical absolute form.
func (r *Registry) resolveCanonicalPath(path string) (string, error) {
	// Clean the path first
	cleaned := filepath.Clean(path)

	// Make absolute if relative
	if !filepath.IsAbs(cleaned) {
		var err error
		cleaned, err = filepath.Abs(cleaned)
		if err != nil {
			return "", fmt.Errorf("making path absolute: %w", err)
		}
	}

	// Evaluate symlinks to get true canonical path
	canonical, err := filepath.EvalSymlinks(cleaned)
	if err != nil {
		if os.IsNotExist(err) {
			// Path doesn't exist, return cleaned absolute path
			// Final validation will catch this
			return cleaned, nil
		}
		return "", fmt.Errorf("evaluating symlinks: %w", err)
	}

	return canonical, nil
}

// validatePathEscape ensures the path doesn't escape the allowed directory.
//
// Even when no confinement root (workspaceDir) is configured, absolute-path
// sanity is enforced: the path must be absolute and canonical (symlinks
// resolved by the caller), and must not contain traversal components in its
// cleaned form. When a confinement root IS configured, the canonical path
// must fall under it.
func (r *Registry) validatePathEscape(path string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("path must be absolute, got %q", path)
	}
	// Reject any lingering traversal after cleaning (defense-in-depth; the
	// caller already runs filepath.Clean + EvalSymlinks, but a path like
	// /a/../etc would survive IsAbs).
	cleaned := filepath.Clean(path)
	if strings.Contains(cleaned, ".."+string(filepath.Separator)) || strings.HasSuffix(cleaned, "..") {
		return fmt.Errorf("path contains traversal: %q", path)
	}

	if r.workspaceDir == "" {
		// No confinement root configured: only absolute + traversal-cleared
		// paths are accepted. (Operators wanting stricter confinement set a
		// workspaceDir.)
		return nil
	}

	// Resolve base directory
	baseDir, err := filepath.Abs(r.workspaceDir)
	if err != nil {
		return fmt.Errorf("resolving base directory: %w", err)
	}

	// Check if path is under base directory
	if !strings.HasPrefix(path, baseDir) {
		return fmt.Errorf("path '%s' escapes workspace directory '%s'", path, baseDir)
	}

	// Additional check: ensure it's not just a prefix match issue
	// e.g., /tmp/ws vs /tmp/ws-other
	remaining := strings.TrimPrefix(path, baseDir)
	if remaining != "" && !strings.HasPrefix(remaining, string(filepath.Separator)) {
		return fmt.Errorf("path '%s' escapes workspace directory '%s'", path, baseDir)
	}

	return nil
}

// validateAllowedAdapters checks adapter IDs.
func (r *Registry) validateAllowedAdapters(adapters []string) error {
	validAdapters := map[string]bool{
		"claude-code":  true,
		"fake-adapter": true,
	}

	for _, adapter := range adapters {
		if !validAdapters[adapter] {
			return fmt.Errorf("unknown adapter '%s'", adapter)
		}
	}

	return nil
}

// Unregister removes a workspace by ID.
func (r *Registry) Unregister(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	ws, exists := r.workspaces[id]
	if !exists {
		// Try to load from store
		var err error
		ws, err = r.store.GetByID(id)
		if err != nil {
			return fmt.Errorf("workspace not found")
		}
	}

	// Delete from store
	if err := r.store.Delete(id); err != nil {
		return fmt.Errorf("deleting workspace: %w", err)
	}

	// Remove from memory
	delete(r.workspaces, id)
	delete(r.paths, ws.Path)

	return nil
}

// GetByID returns a workspace by ID.
func (r *Registry) GetByID(id string) (*domain.Workspace, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ws, exists := r.workspaces[id]
	if exists {
		return ws, nil
	}

	return r.store.GetByID(id)
}

// GetByPath returns a workspace by path.
func (r *Registry) GetByPath(path string) (*domain.Workspace, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Resolve path first
	canonical, err := r.resolveCanonicalPath(path)
	if err != nil {
		return nil, err
	}

	// Check memory
	id, exists := r.paths[canonical]
	if exists {
		return r.workspaces[id], nil
	}

	return r.store.GetByPath(canonical)
}

// GetAll returns all registered workspaces.
func (r *Registry) GetAll() ([]*domain.Workspace, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Start with in-memory workspaces
	result := make([]*domain.Workspace, 0, len(r.workspaces))
	for _, ws := range r.workspaces {
		result = append(result, ws)
	}

	// Add any from store not in memory
	storeWorkspaces, err := r.store.GetAll()
	if err != nil {
		return nil, err
	}

	for i := range storeWorkspaces {
		ws := &storeWorkspaces[i]
		if _, exists := r.workspaces[ws.ID]; !exists {
			result = append(result, ws)
		}
	}

	return result, nil
}

// IsRegistered checks if a workspace ID is registered.
func (r *Registry) IsRegistered(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.workspaces[id]
	if exists {
		return true
	}

	_, err := r.store.GetByID(id)
	return err == nil
}

// IsPathRegistered checks if a path is registered.
func (r *Registry) IsPathRegistered(path string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	canonical, err := r.resolveCanonicalPath(path)
	if err != nil {
		return false
	}

	_, exists := r.paths[canonical]
	if exists {
		return true
	}

	_, err = r.store.GetByPath(canonical)
	return err == nil
}

// ValidatePath returns an error if a path would be invalid for registration.
func (r *Registry) ValidatePath(path string) error {
	// Check empty
	if path == "" {
		return fmt.Errorf("path cannot be empty")
	}

	// Resolve canonical
	canonical, err := r.resolveCanonicalPath(path)
	if err != nil {
		return err
	}

	// Check escape
	if err := r.validatePathEscape(canonical); err != nil {
		return err
	}

	// Check exists
	if _, err := os.Stat(canonical); os.IsNotExist(err) {
		return fmt.Errorf("path does not exist: %s", canonical)
	}

	return nil
}

// LoadFromConfig loads workspaces from configuration.
// Already-registered workspaces (matching ID and path) are treated as
// no-ops so that restarting the server does not error on persisted rows.
func (r *Registry) LoadFromConfig(configs []domain.Workspace) ([]*domain.Workspace, []error) {
	var registered []*domain.Workspace
	var errors []error

	for _, cfg := range configs {
		// Skip if already registered in memory.
		if _, err := r.GetByID(cfg.ID); err == nil {
			continue
		}
		// Skip if already persisted with a matching path.
		if existing, err := r.store.GetByID(cfg.ID); err == nil && existing.Path == cfg.Path {
			r.mu.Lock()
			r.workspaces[existing.ID] = existing
			r.paths[existing.Path] = existing.ID
			r.mu.Unlock()
			continue
		}
		result, err := r.Register(&cfg)
		if err != nil {
			errors = append(errors, fmt.Errorf("workspace %s: %w", cfg.ID, err))
			continue
		}
		registered = append(registered, result.Workspace)
	}

	return registered, errors
}

// Stats returns workspace statistics.
func (r *Registry) Stats() WorkspaceStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return WorkspaceStats{
		Total: len(r.workspaces),
	}
}

// WorkspaceStats contains workspace statistics.
type WorkspaceStats struct {
	Total int `json:"total"`
}
