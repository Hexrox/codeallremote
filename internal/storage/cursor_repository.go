package storage

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/code-all-remote/car/internal/domain"
)

// CursorRepository provides replay-by-cursor access to the event journal.
// Replay is by (session_id, after_sequence) with stable ordering and a
// defined retention boundary: events older than the retention window are
// considered expired and the cursor past them is invalid.
type CursorRepository struct {
	db        *DB
	retention time.Duration
	eventRepo *EventRepository
}

// CursorResult is the outcome of a replay request.
type CursorResult struct {
	Events         []domain.Event `json:"events"`
	NextAfter      int64          `json:"next_after"`
	ResyncRequired bool           `json:"resync_required"`
	HasMore        bool           `json:"has_more"`
}

// NewCursorRepository creates a new cursor repository.
// retention is how long events remain valid for replay; zero means no expiry.
func NewCursorRepository(db *DB, retention time.Duration) *CursorRepository {
	return &CursorRepository{
		db:        db,
		retention: retention,
		eventRepo: NewEventRepository(db),
	}
}

// Replay returns events after the given sequence for a session.
// If after_sequence points past expired events, resync_required is set true.
func (r *CursorRepository) Replay(sessionID string, afterSequence int64, limit int) (*CursorResult, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	// Validate cursor against retention boundary.
	expired, err := r.isCursorExpired(sessionID, afterSequence)
	if err != nil {
		return nil, fmt.Errorf("checking cursor: %w", err)
	}
	if expired {
		return &CursorResult{
			Events:         []domain.Event{},
			NextAfter:      afterSequence,
			ResyncRequired: true,
		}, nil
	}

	events, err := r.eventRepo.GetBySequence(sessionID, afterSequence, limit)
	if err != nil {
		return nil, fmt.Errorf("querying events: %w", err)
	}

	// Determine next_after and has_more
	nextAfter := afterSequence
	if len(events) > 0 {
		nextAfter = events[len(events)-1].Sequence
	}

	// Check if there are more events beyond the limit
	hasMore := false
	if len(events) == limit {
		more, err := r.eventRepo.GetBySequence(sessionID, nextAfter, 1)
		if err == nil && len(more) > 0 {
			hasMore = true
		}
	}

	if events == nil {
		events = []domain.Event{}
	}

	return &CursorResult{
		Events:         events,
		NextAfter:      nextAfter,
		ResyncRequired: false,
		HasMore:        hasMore,
	}, nil
}

// isCursorExpired checks if the cursor points to events that have been
// purged by the retention policy.
func (r *CursorRepository) isCursorExpired(sessionID string, afterSequence int64) (bool, error) {
	if r.retention == 0 {
		return false, nil
	}

	// If afterSequence is 0 (start), nothing can be expired.
	if afterSequence == 0 {
		return false, nil
	}

	// Check if the event at afterSequence still exists.
	var count int64
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM events
		WHERE session_id = ? AND sequence = ?
	`, sessionID, afterSequence).Scan(&count)
	if err != nil {
		return false, err
	}

	if count == 0 {
		// Event is gone; check whether the session is older than retention.
		var createdAt sql.NullTime
		err := r.db.QueryRow(`
			SELECT created_at FROM sessions WHERE id = ?
		`, sessionID).Scan(&createdAt)
		if err != nil {
			return false, err
		}
		if !createdAt.Valid {
			return false, nil
		}

		if time.Since(createdAt.Time) > r.retention {
			return true, nil
		}
	}

	return false, nil
}

// Append appends an event within a transaction, assigning the next sequence.
// This is safe for concurrent appends: the UNIQUE(session_id, sequence)
// constraint and the next-sequence query inside the transaction serialise
// concurrent writers.
func (r *CursorRepository) Append(tx *sql.Tx, event *domain.Event) error {
	// Compute next sequence inside the transaction.
	var seq sql.NullInt64
	err := tx.QueryRow(`
		SELECT MAX(sequence) FROM events WHERE session_id = ?
	`, event.SessionID).Scan(&seq)
	if err != nil {
		return fmt.Errorf("querying max sequence: %w", err)
	}

	nextSeq := int64(1)
	if seq.Valid {
		nextSeq = seq.Int64 + 1
	}

	event.Sequence = nextSeq

	if err := r.eventRepo.AppendWithTx(tx, event); err != nil {
		return fmt.Errorf("appending event: %w", err)
	}

	// Update session last_sequence in the same transaction.
	if _, err := tx.Exec(`
		UPDATE sessions SET last_sequence = ?, updated_at = datetime('now'), last_activity_at = datetime('now')
		WHERE id = ?
	`, nextSeq, event.SessionID); err != nil {
		return fmt.Errorf("updating session last_sequence: %w", err)
	}

	return nil
}

// AppendWithoutTx appends an event without an external transaction.
// It internally wraps the work in a transaction to preserve the
// sequence/last_sequence invariant.
func (r *CursorRepository) AppendWithoutTx(event *domain.Event) (int64, error) {
	err := r.db.WithTransaction(func(tx *sql.Tx) error {
		return r.Append(tx, event)
	})
	if err != nil {
		return 0, err
	}
	return event.Sequence, nil
}

// PurgeExpired removes events older than the retention boundary.
// Returns the number of events removed.
func (r *CursorRepository) PurgeExpired(before time.Time) (int64, error) {
	result, err := r.db.Exec(`
		DELETE FROM events WHERE occurred_at < ?
	`, before)
	if err != nil {
		return 0, fmt.Errorf("purging expired events: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}

	return rows, nil
}

// GetRetentionBoundary returns the time before which events are expired.
// Returns zero time if there is no retention configured.
func (r *CursorRepository) GetRetentionBoundary() time.Time {
	if r.retention == 0 {
		return time.Time{}
	}
	return time.Now().Add(-r.retention)
}

// ValidateCursor checks whether a cursor is valid for this session.
// Returns nil if valid, otherwise an error explaining why.
func (r *CursorRepository) ValidateCursor(sessionID string, afterSequence int64) error {
	if afterSequence < 0 {
		return fmt.Errorf("cursor after_sequence cannot be negative: %d", afterSequence)
	}

	lastSeq, err := r.eventRepo.GetLastSequence(sessionID)
	if err != nil {
		return err
	}

	if afterSequence > lastSeq {
		return fmt.Errorf("cursor after_sequence %d is ahead of last sequence %d", afterSequence, lastSeq)
	}

	return nil
}

// GetCursorMetadata returns metadata about the cursor position.
func (r *CursorRepository) GetCursorMetadata(sessionID string, afterSequence int64) (*CursorMetadata, error) {
	lastSeq, err := r.eventRepo.GetLastSequence(sessionID)
	if err != nil {
		return nil, err
	}

	count, err := r.eventRepo.Count(sessionID)
	if err != nil {
		return nil, err
	}

	remaining := int64(0)
	if afterSequence < lastSeq {
		remaining = lastSeq - afterSequence
	}

	return &CursorMetadata{
		SessionID:       sessionID,
		AfterSequence:   afterSequence,
		LastSequence:    lastSeq,
		TotalEvents:     count,
		RemainingEvents: remaining,
	}, nil
}

// CursorMetadata contains metadata about a cursor position.
type CursorMetadata struct {
	SessionID       string `json:"session_id"`
	AfterSequence   int64  `json:"after_sequence"`
	LastSequence    int64  `json:"last_sequence"`
	TotalEvents     int64  `json:"total_events"`
	RemainingEvents int64  `json:"remaining_events"`
}
