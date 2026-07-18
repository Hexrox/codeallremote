package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/code-all-remote/car/internal/domain"
)

// SessionRepository provides access to sessions and runs.
type SessionRepository struct {
	db *DB
}

// NewSessionRepository creates a new session repository.
func NewSessionRepository(db *DB) *SessionRepository {
	return &SessionRepository{db: db}
}

// CreateSession creates a new session.
func (r *SessionRepository) CreateSession(s *domain.Session) error {
	now := time.Now()
	s.CreatedAt = now
	s.UpdatedAt = now
	s.LastActivityAt = &now

	_, err := r.db.Exec(`
		INSERT INTO sessions (id, workspace_id, adapter_id, state, title, created_at, updated_at, last_activity_at, last_sequence)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, s.ID, s.WorkspaceID, s.AdapterID, s.State, s.Title, s.CreatedAt, s.UpdatedAt, s.LastActivityAt, s.LastSequence)

	if err != nil {
		return fmt.Errorf("creating session: %w", err)
	}

	return nil
}

// GetByID retrieves a session by ID.
func (r *SessionRepository) GetByID(id string) (*domain.Session, error) {
	row := r.db.QueryRow(`
		SELECT id, workspace_id, adapter_id, state, title, created_at, updated_at, last_activity_at, last_sequence, pending_approval_id, owner_device_id, recovery_state
		FROM sessions
		WHERE id = ?
	`, id)

	return scanSession(row)
}

// GetByWorkspaceID retrieves all sessions for a workspace.
func (r *SessionRepository) GetByWorkspaceID(workspaceID string) ([]domain.Session, error) {
	rows, err := r.db.Query(`
		SELECT id, workspace_id, adapter_id, state, title, created_at, updated_at, last_activity_at, last_sequence, pending_approval_id, owner_device_id, recovery_state
		FROM sessions
		WHERE workspace_id = ?
		ORDER BY created_at DESC
	`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("querying sessions: %w", err)
	}
	defer rows.Close()

	return scanSessions(rows)
}

// GetAll retrieves all sessions.
func (r *SessionRepository) GetAll() ([]domain.Session, error) {
	rows, err := r.db.Query(`
		SELECT id, workspace_id, adapter_id, state, title, created_at, updated_at, last_activity_at, last_sequence, pending_approval_id, owner_device_id, recovery_state
		FROM sessions
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("querying sessions: %w", err)
	}
	defer rows.Close()

	return scanSessions(rows)
}

// UpdateState updates a session's state atomically.
// Returns true if the update was successful (session existed and state matched expected if provided).
func (r *SessionRepository) UpdateState(id, newState string, expectedState *string) (bool, error) {
	now := time.Now()

	var result sql.Result
	var err error

	if expectedState != nil {
		result, err = r.db.Exec(`
			UPDATE sessions SET state = ?, updated_at = ?, last_activity_at = ?
			WHERE id = ? AND state = ?
		`, newState, now, now, id, *expectedState)
	} else {
		result, err = r.db.Exec(`
			UPDATE sessions SET state = ?, updated_at = ?, last_activity_at = ?
			WHERE id = ?
		`, newState, now, now, id)
	}

	if err != nil {
		return false, fmt.Errorf("updating session state: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("checking rows affected: %w", err)
	}

	if rows == 0 {
		return false, nil
	}

	return true, nil
}

// SetPendingApproval sets or clears the pending approval ID for a session.
func (r *SessionRepository) SetPendingApproval(id string, approvalID *string) error {
	_, err := r.db.Exec(`
		UPDATE sessions SET pending_approval_id = ?, updated_at = datetime('now')
		WHERE id = ?
	`, approvalID, id)

	return err
}

// UpdateLastSequence updates the last sequence number for a session.
func (r *SessionRepository) UpdateLastSequence(id string, sequence int64) error {
	_, err := r.db.Exec(`
		UPDATE sessions SET last_sequence = ?, updated_at = datetime('now')
		WHERE id = ?
	`, sequence, id)

	return err
}

// Delete removes a session by ID.
func (r *SessionRepository) Delete(id string) error {
	_, err := r.db.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	return err
}

// CreateRun creates a new run for a session.
func (r *SessionRepository) CreateRun(run *domain.Run) error {
	now := time.Now()
	run.CreatedAt = now

	_, err := r.db.Exec(`
		INSERT INTO runs (id, session_id, state, process_pid, process_args, started_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, run.ID, run.SessionID, run.State, run.ProcessPID, run.ProcessArgs, run.StartedAt, run.CreatedAt)

	if err != nil {
		return fmt.Errorf("creating run: %w", err)
	}

	return nil
}

// GetActiveRun retrieves the active run for a session.
func (r *SessionRepository) GetActiveRun(sessionID string) (*domain.Run, error) {
	row := r.db.QueryRow(`
		SELECT id, session_id, state, process_pid, process_args, started_at, ended_at, exit_code, exit_error, created_at
		FROM runs
		WHERE session_id = ? AND state IN ('active', 'starting')
		ORDER BY created_at DESC
		LIMIT 1
	`, sessionID)

	return scanRun(row)
}

// UpdateRunState updates a run's state.
func (r *SessionRepository) UpdateRunState(id, newState string, exitCode *int, exitError *string) error {
	now := time.Now()

	_, err := r.db.Exec(`
		UPDATE runs SET state = ?, ended_at = ?, exit_code = ?, exit_error = ?, updated_at = datetime('now')
		WHERE id = ?
	`, newState, now, exitCode, exitError, id)

	return err
}

// scanSession scans a single session from a row.
func scanSession(row *sql.Row) (*domain.Session, error) {
	var s domain.Session
	var lastActivityAt sql.NullTime
	var pendingApprovalID sql.NullString
	var ownerDeviceID sql.NullString
	var recoveryState sql.NullString

	err := row.Scan(
		&s.ID,
		&s.WorkspaceID,
		&s.AdapterID,
		&s.State,
		&s.Title,
		&s.CreatedAt,
		&s.UpdatedAt,
		&lastActivityAt,
		&s.LastSequence,
		&pendingApprovalID,
		&ownerDeviceID,
		&recoveryState,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("session not found")
		}
		return nil, fmt.Errorf("scanning session: %w", err)
	}

	if lastActivityAt.Valid {
		s.LastActivityAt = &lastActivityAt.Time
	}
	if pendingApprovalID.Valid {
		s.PendingApproval = &pendingApprovalID.String
	}
	if ownerDeviceID.Valid {
		s.OwnerDeviceID = &ownerDeviceID.String
	}
	if recoveryState.Valid {
		s.RecoveryState = &recoveryState.String
	}

	return &s, nil
}

// scanSessions scans multiple sessions from rows.
func scanSessions(rows *sql.Rows) ([]domain.Session, error) {
	var sessions []domain.Session

	for rows.Next() {
		s, err := scanSingleSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, *s)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating sessions: %w", err)
	}

	return sessions, nil
}

func scanSingleSession(rows *sql.Rows) (*domain.Session, error) {
	var s domain.Session
	var lastActivityAt sql.NullTime
	var pendingApprovalID sql.NullString
	var ownerDeviceID sql.NullString
	var recoveryState sql.NullString

	err := rows.Scan(
		&s.ID,
		&s.WorkspaceID,
		&s.AdapterID,
		&s.State,
		&s.Title,
		&s.CreatedAt,
		&s.UpdatedAt,
		&lastActivityAt,
		&s.LastSequence,
		&pendingApprovalID,
		&ownerDeviceID,
		&recoveryState,
	)
	if err != nil {
		return nil, fmt.Errorf("scanning session: %w", err)
	}

	if lastActivityAt.Valid {
		s.LastActivityAt = &lastActivityAt.Time
	}
	if pendingApprovalID.Valid {
		s.PendingApproval = &pendingApprovalID.String
	}
	if ownerDeviceID.Valid {
		s.OwnerDeviceID = &ownerDeviceID.String
	}
	if recoveryState.Valid {
		s.RecoveryState = &recoveryState.String
	}

	return &s, nil
}

func scanRun(row *sql.Row) (*domain.Run, error) {
	var r domain.Run
	var processPID sql.NullInt64
	var processArgs sql.NullString
	var startedAt sql.NullTime
	var endedAt sql.NullTime
	var exitCode sql.NullInt64
	var exitError sql.NullString

	err := row.Scan(
		&r.ID,
		&r.SessionID,
		&r.State,
		&processPID,
		&processArgs,
		&startedAt,
		&endedAt,
		&exitCode,
		&exitError,
		&r.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("run not found")
		}
		return nil, fmt.Errorf("scanning run: %w", err)
	}

	if processPID.Valid {
		pid := int(processPID.Int64)
		r.ProcessPID = &pid
	}
	if processArgs.Valid {
		r.ProcessArgs = &processArgs.String
	}
	if startedAt.Valid {
		r.StartedAt = &startedAt.Time
	}
	if endedAt.Valid {
		r.EndedAt = &endedAt.Time
	}
	if exitCode.Valid {
		code := int(exitCode.Int64)
		r.ExitCode = &code
	}
	if exitError.Valid {
		r.ExitError = &exitError.String
	}

	return &r, nil
}

// WorkspaceRepository provides access to workspaces.
type WorkspaceRepository struct {
	db *DB
}

// NewWorkspaceRepository creates a new workspace repository.
func NewWorkspaceRepository(db *DB) *WorkspaceRepository {
	return &WorkspaceRepository{db: db}
}

// Create creates a new workspace.
func (r *WorkspaceRepository) Create(ws *domain.Workspace) error {
	now := time.Now()
	ws.CreatedAt = now
	ws.UpdatedAt = now

	allowedAdaptersJSON, _ := json.Marshal(ws.AllowedAdapters)
	execPolicyJSON, _ := json.Marshal(ws.ExecutionPolicy)

	_, err := r.db.Exec(`
		INSERT INTO workspaces (id, display_name, path, allowed_adapters, execution_policy, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, ws.ID, ws.DisplayName, ws.Path, string(allowedAdaptersJSON), string(execPolicyJSON), ws.CreatedAt, ws.UpdatedAt)

	if err != nil {
		return fmt.Errorf("creating workspace: %w", err)
	}

	return nil
}

// GetByID retrieves a workspace by ID.
func (r *WorkspaceRepository) GetByID(id string) (*domain.Workspace, error) {
	row := r.db.QueryRow(`
		SELECT id, display_name, path, allowed_adapters, execution_policy, created_at, updated_at
		FROM workspaces
		WHERE id = ?
	`, id)

	return scanWorkspace(row)
}

// GetByPath retrieves a workspace by path.
func (r *WorkspaceRepository) GetByPath(path string) (*domain.Workspace, error) {
	row := r.db.QueryRow(`
		SELECT id, display_name, path, allowed_adapters, execution_policy, created_at, updated_at
		FROM workspaces
		WHERE path = ?
	`, path)

	return scanWorkspace(row)
}

// GetAll retrieves all workspaces.
func (r *WorkspaceRepository) GetAll() ([]domain.Workspace, error) {
	rows, err := r.db.Query(`
		SELECT id, display_name, path, allowed_adapters, execution_policy, created_at, updated_at
		FROM workspaces
		ORDER BY display_name
	`)
	if err != nil {
		return nil, fmt.Errorf("querying workspaces: %w", err)
	}
	defer rows.Close()

	return scanWorkspaces(rows)
}

// Delete removes a workspace by ID.
func (r *WorkspaceRepository) Delete(id string) error {
	_, err := r.db.Exec(`DELETE FROM workspaces WHERE id = ?`, id)
	return err
}

func scanWorkspace(row *sql.Row) (*domain.Workspace, error) {
	var ws domain.Workspace
	var allowedAdaptersStr sql.NullString
	var execPolicyStr sql.NullString

	err := row.Scan(
		&ws.ID,
		&ws.DisplayName,
		&ws.Path,
		&allowedAdaptersStr,
		&execPolicyStr,
		&ws.CreatedAt,
		&ws.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("workspace not found")
		}
		return nil, fmt.Errorf("scanning workspace: %w", err)
	}

	if allowedAdaptersStr.Valid {
		json.Unmarshal([]byte(allowedAdaptersStr.String), &ws.AllowedAdapters)
	}
	if execPolicyStr.Valid {
		json.Unmarshal([]byte(execPolicyStr.String), &ws.ExecutionPolicy)
	}

	return &ws, nil
}

func scanWorkspaces(rows *sql.Rows) ([]domain.Workspace, error) {
	var workspaces []domain.Workspace

	for rows.Next() {
		ws, err := scanWorkspaceRow(rows)
		if err != nil {
			return nil, err
		}
		workspaces = append(workspaces, *ws)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating workspaces: %w", err)
	}

	return workspaces, nil
}

func scanWorkspaceRow(rows *sql.Rows) (*domain.Workspace, error) {
	var ws domain.Workspace
	var allowedAdaptersStr sql.NullString
	var execPolicyStr sql.NullString

	err := rows.Scan(
		&ws.ID,
		&ws.DisplayName,
		&ws.Path,
		&allowedAdaptersStr,
		&execPolicyStr,
		&ws.CreatedAt,
		&ws.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scanning workspace: %w", err)
	}

	if allowedAdaptersStr.Valid {
		json.Unmarshal([]byte(allowedAdaptersStr.String), &ws.AllowedAdapters)
	}
	if execPolicyStr.Valid {
		json.Unmarshal([]byte(execPolicyStr.String), &ws.ExecutionPolicy)
	}

	return &ws, nil
}
