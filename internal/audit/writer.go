// Package audit implements the audit writer for authority-changing actions.
package audit

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/code-all-remote/car/internal/domain"
	"github.com/code-all-remote/car/internal/storage"
)

// Actor types
const (
	ActorTypeUser   = "user"
	ActorTypeDevice = "device"
	ActorTypeSystem = "system"
)

// Outcomes
const (
	OutcomeSuccess = "success"
	OutcomeFailure = "failure"
	OutcomeDenied  = "denied"
)

// Actions
const (
	ActionPromptSubmit      = "prompt_submit"
	ActionInterrupt         = "interrupt"
	ActionApprovalDecision  = "approval_decision"
	ActionPairing           = "pairing"
	ActionRevocation        = "revocation"
	ActionSessionCreate     = "session_create"
	ActionSessionStart      = "session_start"
	ActionSessionResume     = "session_resume"
	ActionWorkspaceRegister = "workspace_register"
	ActionWorkspaceRemove   = "workspace_remove"
)

// Writer writes audit entries with redaction of sensitive context.
type Writer struct {
	mu             sync.Mutex
	db             *storage.DB
	redactPatterns []string
	redactKeys     []string
}

// Entry is an audit entry to be recorded.
type Entry struct {
	ActorID    string         `json:"actor_id"`
	ActorType  string         `json:"actor_type"`
	Action     string         `json:"action"`
	TargetType string         `json:"target_type"`
	TargetID   string         `json:"target_id"`
	Outcome    string         `json:"outcome"`
	Context    map[string]any `json:"context,omitempty"`
}

// NewWriter creates a new audit writer.
func NewWriter(db *storage.DB) *Writer {
	return &Writer{
		db: db,
		// Default redaction keys: any context field with these keys is redacted.
		redactKeys: []string{
			"token", "api_key", "apikey", "secret", "password", "passwd",
			"authorization", "cookie", "credential", "private_key",
			"access_token", "refresh_token", "bearer", "prompt",
		},
	}
}

// WithRedactPatterns adds regex/string patterns to redact from context values.
func (w *Writer) WithRedactPatterns(patterns ...string) *Writer {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.redactPatterns = append(w.redactPatterns, patterns...)
	return w
}

// WithRedactKeys adds context keys to redact.
func (w *Writer) WithRedactKeys(keys ...string) *Writer {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.redactKeys = append(w.redactKeys, keys...)
	return w
}

// Record writes an audit entry to the database.
// Sensitive context fields are redacted before persistence.
func (w *Writer) Record(entry Entry) error {
	if entry.ActorID == "" {
		return fmt.Errorf("actor_id is required")
	}
	if entry.Action == "" {
		return fmt.Errorf("action is required")
	}
	if entry.Outcome == "" {
		entry.Outcome = OutcomeSuccess
	}

	// Redact context
	redactedContext := w.redactContext(entry.Context)

	contextJSON, err := json.Marshal(redactedContext)
	if err != nil {
		return fmt.Errorf("marshaling context: %w", err)
	}

	_, err = w.db.Exec(`
		INSERT INTO audit_log (actor_id, actor_type, action, target_type, target_id, outcome, context)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, entry.ActorID, entry.ActorType, entry.Action, entry.TargetType,
		entry.TargetID, entry.Outcome, string(contextJSON))

	if err != nil {
		return fmt.Errorf("writing audit entry: %w", err)
	}

	return nil
}

// RecordWithTx writes an audit entry within a transaction.
// Use this when the audit record must commit atomically with the state change.
func (w *Writer) RecordWithTx(tx *sql.Tx, entry Entry) error {
	if entry.ActorID == "" {
		return fmt.Errorf("actor_id is required")
	}
	if entry.Action == "" {
		return fmt.Errorf("action is required")
	}
	if entry.Outcome == "" {
		entry.Outcome = OutcomeSuccess
	}

	redactedContext := w.redactContext(entry.Context)
	contextJSON, err := json.Marshal(redactedContext)
	if err != nil {
		return fmt.Errorf("marshaling context: %w", err)
	}

	_, err = tx.Exec(`
		INSERT INTO audit_log (actor_id, actor_type, action, target_type, target_id, outcome, context)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, entry.ActorID, entry.ActorType, entry.Action, entry.TargetType,
		entry.TargetID, entry.Outcome, string(contextJSON))

	if err != nil {
		return fmt.Errorf("writing audit entry: %w", err)
	}

	return nil
}

// redactContext returns a copy of the context with sensitive values replaced.
// It recurses into nested maps and slices so a value like
// {"headers": {"authorization": "Bearer x"}} is redacted, and treats
// []byte / json.RawMessage string-like values (redacting patterns within them).
func (w *Writer) redactContext(context map[string]any) map[string]any {
	if context == nil {
		return nil
	}
	w.mu.Lock()
	keys := make([]string, len(w.redactKeys))
	copy(keys, w.redactKeys)
	patterns := make([]string, len(w.redactPatterns))
	copy(patterns, w.redactPatterns)
	w.mu.Unlock()

	out := make(map[string]any, len(context))
	for k, v := range context {
		out[k] = redactValue(k, v, keys, patterns)
	}
	return out
}

// redactValue redacts a single value by key name, type, and nested structures.
func redactValue(key string, v any, keys, patterns []string) any {
	if isSensitiveKey(key, keys) {
		return "[REDACTED]"
	}
	switch val := v.(type) {
	case string:
		return redactPatterns(val, patterns)
	case []byte:
		return redactPatterns(string(val), patterns)
	case json.RawMessage:
		return redactPatterns(string(val), patterns)
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, vv := range val {
			out[k] = redactValue(k, vv, keys, patterns)
		}
		return out
	case []any:
		out := make([]any, len(val))
		for i, vv := range val {
			out[i] = redactValue("", vv, keys, patterns)
		}
		return out
	default:
		return v
	}
}

// isSensitiveKey checks if a key name indicates sensitive data.
func isSensitiveKey(key string, sensitiveKeys []string) bool {
	lower := strings.ToLower(key)
	for _, sk := range sensitiveKeys {
		if strings.Contains(lower, strings.ToLower(sk)) {
			return true
		}
	}
	return false
}

// redactPatterns replaces pattern matches with [REDACTED].
func redactPatterns(value string, patterns []string) string {
	for _, p := range patterns {
		if p != "" && len(p) >= 4 {
			value = strings.ReplaceAll(value, p, "[REDACTED]")
		}
	}
	return value
}

// GetByID returns an audit entry by ID.
func (w *Writer) GetByID(id int64) (*domain.AuditEntry, error) {
	row := w.db.QueryRow(`
		SELECT id, actor_id, actor_type, action, target_type, target_id, outcome, context, created_at
		FROM audit_log
		WHERE id = ?
	`, id)
	return scanAuditEntry(row)
}

// GetByActor returns audit entries for an actor.
func (w *Writer) GetByActor(actorID string, limit int) ([]domain.AuditEntry, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}

	rows, err := w.db.Query(`
		SELECT id, actor_id, actor_type, action, target_type, target_id, outcome, context, created_at
		FROM audit_log
		WHERE actor_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`, actorID, limit)
	if err != nil {
		return nil, fmt.Errorf("querying audit entries: %w", err)
	}
	defer rows.Close()

	return scanAuditEntries(rows)
}

// GetByTarget returns audit entries for a target.
func (w *Writer) GetByTarget(targetType, targetID string, limit int) ([]domain.AuditEntry, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}

	rows, err := w.db.Query(`
		SELECT id, actor_id, actor_type, action, target_type, target_id, outcome, context, created_at
		FROM audit_log
		WHERE target_type = ? AND target_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`, targetType, targetID, limit)
	if err != nil {
		return nil, fmt.Errorf("querying audit entries: %w", err)
	}
	defer rows.Close()

	return scanAuditEntries(rows)
}

// GetByAction returns audit entries for an action.
func (w *Writer) GetByAction(action string, limit int) ([]domain.AuditEntry, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}

	rows, err := w.db.Query(`
		SELECT id, actor_id, actor_type, action, target_type, target_id, outcome, context, created_at
		FROM audit_log
		WHERE action = ?
		ORDER BY created_at DESC
		LIMIT ?
	`, action, limit)
	if err != nil {
		return nil, fmt.Errorf("querying audit entries: %w", err)
	}
	defer rows.Close()

	return scanAuditEntries(rows)
}

// GetRecent returns recent audit entries.
func (w *Writer) GetRecent(limit int) ([]domain.AuditEntry, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}

	rows, err := w.db.Query(`
		SELECT id, actor_id, actor_type, action, target_type, target_id, outcome, context, created_at
		FROM audit_log
		ORDER BY created_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("querying audit entries: %w", err)
	}
	defer rows.Close()

	return scanAuditEntries(rows)
}

// GetByTimeRange returns audit entries within a time range.
func (w *Writer) GetByTimeRange(from, to time.Time, limit int) ([]domain.AuditEntry, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}

	rows, err := w.db.Query(`
		SELECT id, actor_id, actor_type, action, target_type, target_id, outcome, context, created_at
		FROM audit_log
		WHERE created_at >= ? AND created_at <= ?
		ORDER BY created_at DESC
		LIMIT ?
	`, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("querying audit entries: %w", err)
	}
	defer rows.Close()

	return scanAuditEntries(rows)
}

// Count returns the total number of audit entries.
func (w *Writer) Count() (int64, error) {
	var count int64
	err := w.db.QueryRow(`SELECT COUNT(*) FROM audit_log`).Scan(&count)
	return count, err
}

// PurgeOlderThan removes audit entries older than the given time.
// Returns the number of entries removed.
func (w *Writer) PurgeOlderThan(before time.Time) (int64, error) {
	result, err := w.db.Exec(`DELETE FROM audit_log WHERE created_at < ?`, before)
	if err != nil {
		return 0, fmt.Errorf("purging audit entries: %w", err)
	}
	return result.RowsAffected()
}

func scanAuditEntry(row *sql.Row) (*domain.AuditEntry, error) {
	var e domain.AuditEntry
	var contextStr sql.NullString

	err := row.Scan(
		&e.ID, &e.ActorID, &e.ActorType, &e.Action,
		&e.TargetType, &e.TargetID, &e.Outcome,
		&contextStr, &e.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("audit entry not found")
		}
		return nil, fmt.Errorf("scanning audit entry: %w", err)
	}

	if contextStr.Valid {
		s := contextStr.String
		e.Context = &s
	}

	return &e, nil
}

func scanAuditEntries(rows *sql.Rows) ([]domain.AuditEntry, error) {
	var entries []domain.AuditEntry

	for rows.Next() {
		var e domain.AuditEntry
		var contextStr sql.NullString

		err := rows.Scan(
			&e.ID, &e.ActorID, &e.ActorType, &e.Action,
			&e.TargetType, &e.TargetID, &e.Outcome,
			&contextStr, &e.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning audit entry: %w", err)
		}

		if contextStr.Valid {
			s := contextStr.String
			e.Context = &s
		}

		entries = append(entries, e)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating audit entries: %w", err)
	}

	return entries, nil
}
