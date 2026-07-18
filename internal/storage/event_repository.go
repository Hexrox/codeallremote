package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/code-all-remote/car/internal/domain"
)

// EventRepository provides access to domain events.
type EventRepository struct {
	db *DB
}

// NewEventRepository creates a new event repository.
func NewEventRepository(db *DB) *EventRepository {
	return &EventRepository{db: db}
}

// Append adds a new event to the journal.
// Returns the assigned sequence number.
func (r *EventRepository) Append(event *domain.Event) (int64, error) {
	payloadJSON, err := json.Marshal(event.Payload)
	if err != nil {
		return 0, fmt.Errorf("marshaling payload: %w", err)
	}

	var seq int64
	err = r.db.QueryRow(`
		INSERT INTO events (session_id, sequence, type, message_id, schema_version, payload, occurred_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		RETURNING sequence
	`, event.SessionID, event.Sequence, event.Type, event.MessageID, event.SchemaVersion, string(payloadJSON), event.OccurredAt).Scan(&seq)

	if err != nil {
		return 0, fmt.Errorf("inserting event: %w", err)
	}

	return seq, nil
}

// AppendWithTx adds an event within a transaction.
func (r *EventRepository) AppendWithTx(tx *sql.Tx, event *domain.Event) error {
	payloadJSON, err := json.Marshal(event.Payload)
	if err != nil {
		return fmt.Errorf("marshaling payload: %w", err)
	}

	_, err = tx.Exec(`
		INSERT INTO events (session_id, sequence, type, message_id, schema_version, payload, occurred_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, event.SessionID, event.Sequence, event.Type, event.MessageID, event.SchemaVersion, string(payloadJSON), event.OccurredAt)

	if err != nil {
		return fmt.Errorf("inserting event: %w", err)
	}

	return nil
}

// GetBySequence retrieves events for a session after a given sequence number.
func (r *EventRepository) GetBySequence(sessionID string, afterSeq int64, limit int) ([]domain.Event, error) {
	rows, err := r.db.Query(`
		SELECT id, session_id, sequence, type, message_id, schema_version, payload, occurred_at
		FROM events
		WHERE session_id = ? AND sequence > ?
		ORDER BY sequence ASC
		LIMIT ?
	`, sessionID, afterSeq, limit)
	if err != nil {
		return nil, fmt.Errorf("querying events: %w", err)
	}
	defer rows.Close()

	events, err := scanEvents(rows)
	if err != nil {
		return nil, err
	}

	return events, nil
}

// GetLastSequence returns the last sequence number for a session.
func (r *EventRepository) GetLastSequence(sessionID string) (int64, error) {
	var seq sql.NullInt64
	err := r.db.QueryRow(`
		SELECT MAX(sequence) FROM events WHERE session_id = ?
	`, sessionID).Scan(&seq)
	if err != nil {
		return 0, fmt.Errorf("querying last sequence: %w", err)
	}

	if !seq.Valid {
		return 0, nil
	}

	return seq.Int64, nil
}

// GetSessionEvents returns all events for a session.
func (r *EventRepository) GetSessionEvents(sessionID string) ([]domain.Event, error) {
	rows, err := r.db.Query(`
		SELECT id, session_id, sequence, type, message_id, schema_version, payload, occurred_at
		FROM events
		WHERE session_id = ?
		ORDER BY sequence ASC
	`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("querying events: %w", err)
	}
	defer rows.Close()

	return scanEvents(rows)
}

// Count returns the total number of events for a session.
func (r *EventRepository) Count(sessionID string) (int64, error) {
	var count int64
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM events WHERE session_id = ?
	`, sessionID).Scan(&count)

	return count, err
}

// DeleteBySessionID removes all events for a session.
func (r *EventRepository) DeleteBySessionID(tx *sql.Tx, sessionID string) error {
	_, err := tx.Exec(`DELETE FROM events WHERE session_id = ?`, sessionID)
	return err
}

func scanEvents(rows *sql.Rows) ([]domain.Event, error) {
	var events []domain.Event

	for rows.Next() {
		var e domain.Event
		var payloadStr string

		err := rows.Scan(
			&e.ID,
			&e.SessionID,
			&e.Sequence,
			&e.Type,
			&e.MessageID,
			&e.SchemaVersion,
			&payloadStr,
			&e.OccurredAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning event: %w", err)
		}

		if err := json.Unmarshal([]byte(payloadStr), &e.Payload); err != nil {
			return nil, fmt.Errorf("unmarshaling payload for event %d: %w", e.ID, err)
		}

		events = append(events, e)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating events: %w", err)
	}

	return events, nil
}
