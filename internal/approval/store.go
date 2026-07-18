package approval

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/code-all-remote/car/internal/domain"
	"github.com/code-all-remote/car/internal/storage"
)

// DBStore implements ApprovalStore using SQLite/Postgres.
type DBStore struct {
	db *storage.DB
}

// NewDBStore creates a new database-backed approval store.
func NewDBStore(db *storage.DB) *DBStore {
	return &DBStore{db: db}
}

// Create persists an approval.
func (s *DBStore) Create(approval *domain.Approval) error {
	payloadJSON, err := json.Marshal(approval.StructuredPayload)
	if err != nil {
		return fmt.Errorf("marshaling payload: %w", err)
	}

	_, err = s.db.Exec(`
		INSERT INTO approvals (id, session_id, category, state, action_kind, human_readable_context, structured_payload, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, approval.ID, approval.SessionID, approval.Category, approval.State, approval.ActionKind, approval.HumanReadableContext, string(payloadJSON), approval.CreatedAt, approval.ExpiresAt)

	return err
}

// GetByID retrieves an approval by ID.
func (s *DBStore) GetByID(id string) (*domain.Approval, error) {
	row := s.db.QueryRow(`
		SELECT id, session_id, category, state, action_kind, human_readable_context, structured_payload, created_at, expires_at, decided_at, decision_reason
		FROM approvals
		WHERE id = ?
	`, id)

	return scanApproval(row)
}

// Update updates an approval.
func (s *DBStore) Update(approval *domain.Approval) error {
	_, err := s.db.Exec(`
		UPDATE approvals
		SET state = ?, decided_at = ?, decision_reason = ?
		WHERE id = ?
	`, approval.State, approval.DecidedAt, approval.DecisionReason, approval.ID)

	return err
}

// GetPendingBySession returns pending approvals for a session.
func (s *DBStore) GetPendingBySession(sessionID string) ([]*domain.Approval, error) {
	rows, err := s.db.Query(`
		SELECT id, session_id, category, state, action_kind, human_readable_context, structured_payload, created_at, expires_at, decided_at, decision_reason
		FROM approvals
		WHERE session_id = ? AND state = 'pending'
		ORDER BY created_at ASC
	`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("querying approvals: %w", err)
	}
	defer rows.Close()

	return scanApprovals(rows)
}

// Delete removes an approval.
func (s *DBStore) Delete(id string) error {
	_, err := s.db.Exec(`DELETE FROM approvals WHERE id = ?`, id)
	return err
}

func scanApproval(row *sql.Row) (*domain.Approval, error) {
	var a domain.Approval
	var structuredPayloadStr string
	var decidedAt sql.NullTime
	var decisionReason sql.NullString

	err := row.Scan(
		&a.ID,
		&a.SessionID,
		&a.Category,
		&a.State,
		&a.ActionKind,
		&a.HumanReadableContext,
		&structuredPayloadStr,
		&a.CreatedAt,
		&a.ExpiresAt,
		&decidedAt,
		&decisionReason,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("approval not found")
		}
		return nil, fmt.Errorf("scanning approval: %w", err)
	}

	if err := json.Unmarshal([]byte(structuredPayloadStr), &a.StructuredPayload); err != nil {
		return nil, fmt.Errorf("unmarshaling payload: %w", err)
	}

	if decidedAt.Valid {
		a.DecidedAt = &decidedAt.Time
	}
	if decisionReason.Valid {
		a.DecisionReason = &decisionReason.String
	}

	return &a, nil
}

func scanApprovals(rows *sql.Rows) ([]*domain.Approval, error) {
	var approvals []*domain.Approval

	for rows.Next() {
		a, err := scanApprovalRow(rows)
		if err != nil {
			return nil, err
		}
		approvals = append(approvals, a)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating approvals: %w", err)
	}

	return approvals, nil
}

func scanApprovalRow(rows *sql.Rows) (*domain.Approval, error) {
	var a domain.Approval
	var structuredPayloadStr string
	var decidedAt sql.NullTime
	var decisionReason sql.NullString

	err := rows.Scan(
		&a.ID,
		&a.SessionID,
		&a.Category,
		&a.State,
		&a.ActionKind,
		&a.HumanReadableContext,
		&structuredPayloadStr,
		&a.CreatedAt,
		&a.ExpiresAt,
		&decidedAt,
		&decisionReason,
	)
	if err != nil {
		return nil, fmt.Errorf("scanning approval: %w", err)
	}

	if err := json.Unmarshal([]byte(structuredPayloadStr), &a.StructuredPayload); err != nil {
		return nil, fmt.Errorf("unmarshaling payload: %w", err)
	}

	if decidedAt.Valid {
		a.DecidedAt = &decidedAt.Time
	}
	if decisionReason.Valid {
		a.DecisionReason = &decisionReason.String
	}

	return &a, nil
}

// MemoryStore implements ApprovalStore in-memory (for testing).
type MemoryStore struct {
	mu        interface{} // dummy for sync.Mutex
	approvals map[string]*domain.Approval
}

// NewMemoryStore creates a new in-memory approval store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		approvals: make(map[string]*domain.Approval),
	}
}

// Create persists an approval.
func (s *MemoryStore) Create(approval *domain.Approval) error {
	s.approvals[approval.ID] = approval
	return nil
}

// GetByID retrieves an approval by ID.
func (s *MemoryStore) GetByID(id string) (*domain.Approval, error) {
	a, ok := s.approvals[id]
	if !ok {
		return nil, fmt.Errorf("approval not found")
	}
	return a, nil
}

// Update updates an approval.
func (s *MemoryStore) Update(approval *domain.Approval) error {
	if _, ok := s.approvals[approval.ID]; !ok {
		return fmt.Errorf("approval not found")
	}
	s.approvals[approval.ID] = approval
	return nil
}

// GetPendingBySession returns pending approvals for a session.
func (s *MemoryStore) GetPendingBySession(sessionID string) ([]*domain.Approval, error) {
	var result []*domain.Approval
	for _, a := range s.approvals {
		if a.SessionID == sessionID && a.State == domain.ApprovalStatePending {
			result = append(result, a)
		}
	}
	return result, nil
}

// Delete removes an approval.
func (s *MemoryStore) Delete(id string) error {
	delete(s.approvals, id)
	return nil
}

// SetApproval sets an approval directly (for testing).
func (s *MemoryStore) SetApproval(id string, approval *domain.Approval) {
	s.approvals[id] = approval
}

// GetApproval gets an approval directly (for testing).
func (s *MemoryStore) GetApproval(id string) (*domain.Approval, bool) {
	a, ok := s.approvals[id]
	return a, ok
}

// GetAll returns all approvals (for testing).
func (s *MemoryStore) GetAll() []*domain.Approval {
	result := make([]*domain.Approval, 0, len(s.approvals))
	for _, a := range s.approvals {
		result = append(result, a)
	}
	return result
}
