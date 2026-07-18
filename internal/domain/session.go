// Package domain contains the core domain models for CAR.
package domain

import (
	"time"
)

// Session states (lifecycle)
const (
	SessionStateCreated       = "created"
	SessionStateStarting      = "starting"
	SessionStateActive        = "active"
	SessionStateWaitingApprov = "waiting_approval"
	SessionStateInterrupted   = "interrupted"
	SessionStateCompleted     = "completed"
	SessionStateFailed        = "failed"
	SessionStateResumable     = "resumable"
	SessionStateRecovering    = "recovering"
)

// Session represents a coding agent session.
type Session struct {
	ID              string     `json:"id"`
	WorkspaceID     string     `json:"workspace_id"`
	AdapterID       string     `json:"adapter_id"`
	State           string     `json:"state"`
	Title           string     `json:"title,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	LastActivityAt  *time.Time `json:"last_activity_at,omitempty"`
	LastSequence    int64      `json:"last_sequence"`
	PendingApproval *string    `json:"pending_approval_id,omitempty"`
	OwnerDeviceID   *string    `json:"owner_device_id,omitempty"`
	RecoveryState   *string    `json:"recovery_state,omitempty"`
}

// Snapshot returns a lightweight snapshot of the session for API responses.
func (s *Session) Snapshot() SessionSnapshot {
	return SessionSnapshot{
		ID:                s.ID,
		WorkspaceID:       s.WorkspaceID,
		AdapterID:         s.AdapterID,
		State:             s.State,
		LastSequence:      s.LastSequence,
		PendingApprovalID: s.PendingApproval,
	}
}

// SessionSnapshot is a read-only view of a session for API responses.
type SessionSnapshot struct {
	ID                string  `json:"id"`
	WorkspaceID       string  `json:"workspace_id"`
	AdapterID         string  `json:"adapter_id"`
	State             string  `json:"state"`
	LastSequence      int64   `json:"last_sequence"`
	PendingApprovalID *string `json:"pending_approval_id,omitempty"`
}

// Run states
const (
	RunStatePending     = "pending"
	RunStateStarting    = "starting"
	RunStateActive      = "active"
	RunStateCompleted   = "completed"
	RunStateFailed      = "failed"
	RunStateInterrupted = "interrupted"
)

// Run represents an execution attempt within a session.
type Run struct {
	ID          string     `json:"id"`
	SessionID   string     `json:"session_id"`
	State       string     `json:"state"`
	ProcessPID  *int       `json:"process_pid,omitempty"`
	ProcessArgs *string    `json:"process_args,omitempty"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	EndedAt     *time.Time `json:"ended_at,omitempty"`
	ExitCode    *int       `json:"exit_code,omitempty"`
	ExitError   *string    `json:"exit_error,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// Workspace represents a registered workspace.
type Workspace struct {
	ID              string          `json:"id"`
	DisplayName     string          `json:"display_name"`
	Path            string          `json:"path"`
	AllowedAdapters []string        `json:"allowed_adapters,omitempty"`
	ExecutionPolicy ExecutionPolicy `json:"execution_policy"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// ExecutionPolicy defines workspace execution restrictions.
type ExecutionPolicy struct {
	AllowNetworkAccess bool     `json:"allow_network_access"`
	AllowWrites        bool     `json:"allow_writes"`
	AllowedCommands    []string `json:"allowed_commands,omitempty"`
}

// Approval states
const (
	ApprovalStatePending   = "pending"
	ApprovalStateApproved  = "approved"
	ApprovalStateDenied    = "denied"
	ApprovalStateExpired   = "expired"
	ApprovalStateCancelled = "cancelled"
)

// Approval represents a request for user decision.
type Approval struct {
	ID                   string     `json:"id"`
	SessionID            string     `json:"session_id"`
	Category             string     `json:"category"`
	State                string     `json:"state"`
	ActionKind           string     `json:"action_kind"`
	HumanReadableContext string     `json:"human_readable_context"`
	StructuredPayload    string     `json:"structured_payload"`
	CreatedAt            time.Time  `json:"created_at"`
	ExpiresAt            time.Time  `json:"expires_at"`
	DecidedAt            *time.Time `json:"decided_at,omitempty"`
	DecisionReason       *string    `json:"decision_reason,omitempty"`
}

// Event represents an immutable domain event.
type Event struct {
	ID            int64          `json:"id"`
	SessionID     string         `json:"session_id"`
	Sequence      int64          `json:"sequence"`
	Type          string         `json:"type"`
	MessageID     string         `json:"message_id"`
	SchemaVersion int            `json:"schema_version"`
	Payload       map[string]any `json:"payload"`
	OccurredAt    time.Time      `json:"occurred_at"`
}

// AuditEntry represents an auditable action.
type AuditEntry struct {
	ID         int64     `json:"id"`
	ActorID    string    `json:"actor_id"`
	ActorType  string    `json:"actor_type"` // "user" | "system" | "device"
	Action     string    `json:"action"`
	TargetType string    `json:"target_type"`
	TargetID   string    `json:"target_id"`
	Outcome    string    `json:"outcome"` // "success" | "failure" | "denied"
	Context    *string   `json:"context,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// Device states
const (
	DeviceStatePending = "pending"
	DeviceStatePaired  = "paired"
	DeviceStateRevoked = "revoked"
)

// Device represents a paired client device.
type Device struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	PublicKey  string     `json:"public_key"`
	State      string     `json:"state"` // "pending" | "paired" | "revoked"
	PairedAt   *time.Time `json:"paired_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	LastSeenAt *time.Time `json:"last_seen_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}
