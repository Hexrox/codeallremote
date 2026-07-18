// Package projection provides read-model projections from the event journal.
package projection

import (
	"database/sql"
	"fmt"

	"github.com/code-all-remote/car/internal/domain"
	"github.com/code-all-remote/car/internal/storage"
)

// SnapshotProjection builds session snapshots from the persistence layer.
// It returns lifecycle state, active run, pending approval and last sequence
// in a single transaction so the snapshot is internally consistent.
type SnapshotProjection struct {
	db *storage.DB
}

// NewSnapshotProjection creates a new snapshot projection.
func NewSnapshotProjection(db *storage.DB) *SnapshotProjection {
	return &SnapshotProjection{db: db}
}

// Snapshot is an internal read model that combines session, run and approval.
type Snapshot struct {
	Session         *domain.Session  `json:"session"`
	ActiveRun       *domain.Run      `json:"active_run,omitempty"`
	PendingApproval *domain.Approval `json:"pending_approval,omitempty"`
	LastSequence    int64            `json:"last_sequence"`
}

// GetSnapshot returns a session snapshot with run and approval info.
// It uses a single transaction for consistency.
func (p *SnapshotProjection) GetSnapshot(sessionID string) (*Snapshot, error) {
	var snap Snapshot

	err := p.db.WithTransaction(func(tx *sql.Tx) error {
		// Load session
		session, err := loadSession(tx, sessionID)
		if err != nil {
			return err
		}
		snap.Session = session

		// Load last sequence (consistent with session)
		snap.LastSequence = session.LastSequence

		// Load active run if any
		run, err := loadActiveRun(tx, sessionID)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		snap.ActiveRun = run

		// Load pending approval if any
		if session.PendingApproval != nil && *session.PendingApproval != "" {
			approval, err := loadApproval(tx, *session.PendingApproval)
			if err != nil && err != sql.ErrNoRows {
				return err
			}
			snap.PendingApproval = approval
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return &snap, nil
}

// GetSnapshotMultiple returns snapshots for multiple sessions.
func (p *SnapshotProjection) GetSnapshotMultiple(sessionIDs []string) ([]*Snapshot, error) {
	snapshots := make([]*Snapshot, 0, len(sessionIDs))

	for _, id := range sessionIDs {
		snap, err := p.GetSnapshot(id)
		if err != nil {
			// Skip missing sessions
			continue
		}
		snapshots = append(snapshots, snap)
	}

	return snapshots, nil
}

// Reconstruct rebuilds session state from the event journal.
// This is used after a server restart to verify the snapshot is correct.
func (p *SnapshotProjection) Reconstruct(sessionID string) (*Snapshot, error) {
	var snap Snapshot

	err := p.db.WithTransaction(func(tx *sql.Tx) error {
		// Load session
		session, err := loadSession(tx, sessionID)
		if err != nil {
			return err
		}
		snap.Session = session

		// Recompute last sequence from events
		lastSeq, err := loadLastEventSequence(tx, sessionID)
		if err != nil {
			return err
		}

		// Verify stored sequence matches events
		if lastSeq != session.LastSequence {
			// Stored sequence is out of sync; use event-derived value
			snap.LastSequence = lastSeq
		} else {
			snap.LastSequence = session.LastSequence
		}

		// Load active run
		run, err := loadActiveRun(tx, sessionID)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		snap.ActiveRun = run

		// Load pending approval
		if session.PendingApproval != nil && *session.PendingApproval != "" {
			approval, err := loadApproval(tx, *session.PendingApproval)
			if err != nil && err != sql.ErrNoRows {
				return err
			}
			snap.PendingApproval = approval
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return &snap, nil
}

// loadSession loads a session within a transaction.
func loadSession(tx *sql.Tx, id string) (*domain.Session, error) {
	var s domain.Session
	var lastActivityAt sql.NullTime
	var pendingApprovalID sql.NullString
	var ownerDeviceID sql.NullString
	var recoveryState sql.NullString

	err := tx.QueryRow(`
		SELECT id, workspace_id, adapter_id, state, title, created_at, updated_at, last_activity_at, last_sequence, pending_approval_id, owner_device_id, recovery_state
		FROM sessions
		WHERE id = ?
	`, id).Scan(
		&s.ID, &s.WorkspaceID, &s.AdapterID, &s.State, &s.Title,
		&s.CreatedAt, &s.UpdatedAt, &lastActivityAt, &s.LastSequence,
		&pendingApprovalID, &ownerDeviceID, &recoveryState,
	)
	if err != nil {
		return nil, err
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

// loadActiveRun loads the active run for a session.
func loadActiveRun(tx *sql.Tx, sessionID string) (*domain.Run, error) {
	var r domain.Run
	var processPID sql.NullInt64
	var processArgs sql.NullString
	var startedAt sql.NullTime
	var endedAt sql.NullTime
	var exitCode sql.NullInt64
	var exitError sql.NullString

	err := tx.QueryRow(`
		SELECT id, session_id, state, process_pid, process_args, started_at, ended_at, exit_code, exit_error, created_at
		FROM runs
		WHERE session_id = ? AND state IN ('active', 'starting')
		ORDER BY created_at DESC
		LIMIT 1
	`, sessionID).Scan(
		&r.ID, &r.SessionID, &r.State, &processPID, &processArgs,
		&startedAt, &endedAt, &exitCode, &exitError, &r.CreatedAt,
	)
	if err != nil {
		return nil, err
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

// loadApproval loads an approval by ID.
func loadApproval(tx *sql.Tx, id string) (*domain.Approval, error) {
	var a domain.Approval
	var decidedAt sql.NullTime
	var decisionReason sql.NullString

	err := tx.QueryRow(`
		SELECT id, session_id, category, state, action_kind, human_readable_context, structured_payload, created_at, expires_at, decided_at, decision_reason
		FROM approvals
		WHERE id = ?
	`, id).Scan(
		&a.ID, &a.SessionID, &a.Category, &a.State, &a.ActionKind,
		&a.HumanReadableContext, &a.StructuredPayload, &a.CreatedAt,
		&a.ExpiresAt, &decidedAt, &decisionReason,
	)
	if err != nil {
		return nil, err
	}

	if decidedAt.Valid {
		a.DecidedAt = &decidedAt.Time
	}
	if decisionReason.Valid {
		a.DecisionReason = &decisionReason.String
	}

	return &a, nil
}

// loadLastEventSequence returns the last event sequence from the journal.
func loadLastEventSequence(tx *sql.Tx, sessionID string) (int64, error) {
	var seq sql.NullInt64
	err := tx.QueryRow(`
		SELECT MAX(sequence) FROM events WHERE session_id = ?
	`, sessionID).Scan(&seq)
	if err != nil {
		return 0, err
	}
	if !seq.Valid {
		return 0, nil
	}
	return seq.Int64, nil
}

// ReconstructAll rebuilds all session snapshots for restart recovery.
func (p *SnapshotProjection) ReconstructAll() ([]*Snapshot, error) {
	// Get all session IDs
	rows, err := p.db.Query(`SELECT id FROM sessions ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("querying sessions: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	snapshots := make([]*Snapshot, 0, len(ids))
	for _, id := range ids {
		snap, err := p.Reconstruct(id)
		if err != nil {
			continue
		}
		snapshots = append(snapshots, snap)
	}

	return snapshots, nil
}

// IsConsistent checks if a snapshot is internally consistent.
// This verifies that the stored last_sequence matches the event journal.
func (p *SnapshotProjection) IsConsistent(sessionID string) (bool, error) {
	var storedSeq int64
	err := p.db.QueryRow(`SELECT last_sequence FROM sessions WHERE id = ?`, sessionID).Scan(&storedSeq)
	if err != nil {
		return false, err
	}

	journalSeq, err := p.getLastSequenceFromDB(sessionID)
	if err != nil {
		return false, err
	}

	return storedSeq == journalSeq, nil
}

// FixConsistency updates the stored last_sequence to match the journal.
func (p *SnapshotProjection) FixConsistency(sessionID string) error {
	journalSeq, err := p.getLastSequenceFromDB(sessionID)
	if err != nil {
		return err
	}

	_, err = p.db.Exec(`UPDATE sessions SET last_sequence = ? WHERE id = ?`, journalSeq, sessionID)
	return err
}

// getLastSequenceFromDB queries the last sequence directly without a transaction.
func (p *SnapshotProjection) getLastSequenceFromDB(sessionID string) (int64, error) {
	var seq sql.NullInt64
	err := p.db.QueryRow(`SELECT MAX(sequence) FROM events WHERE session_id = ?`, sessionID).Scan(&seq)
	if err != nil {
		return 0, err
	}
	if !seq.Valid {
		return 0, nil
	}
	return seq.Int64, nil
}
