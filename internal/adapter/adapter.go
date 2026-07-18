// Package adapter provides the adapter interface and implementations for coding agents.
package adapter

import (
	"context"
	"encoding/json"
	"time"

	"github.com/code-all-remote/car/internal/domain"
)

// AdapterSignalType defines types of signals from adapter to core.
type AdapterSignalType string

const (
	SignalOutput          AdapterSignalType = "output"
	SignalStatusChange    AdapterSignalType = "status_change"
	SignalApprovalRequest AdapterSignalType = "approval_request"
	SignalChangedFile     AdapterSignalType = "changed_file"
	SignalDiagnostic      AdapterSignalType = "diagnostic"
	SignalCompletion      AdapterSignalType = "completion"
	SignalError           AdapterSignalType = "error"
)

// AdapterSignal represents a normalized signal from an adapter.
type AdapterSignal struct {
	Type      AdapterSignalType `json:"type"`
	SessionID string            `json:"session_id"`
	Timestamp time.Time         `json:"timestamp"`
	Payload   json.RawMessage   `json:"payload,omitempty"`
}

// OutputPayload contains text output from the agent.
type OutputPayload struct {
	Content string `json:"content"`
	Stream  string `json:"stream,omitempty"` // "stdout" | "stderr"
}

// StatusChangePayload contains a lifecycle state change.
type StatusChangePayload struct {
	OldState string `json:"old_state"`
	NewState string `json:"new_state"`
	Reason   string `json:"reason,omitempty"`
}

// ApprovalRequestPayload contains an approval request.
type ApprovalRequestPayload struct {
	ApprovalID           string         `json:"approval_id"`
	Category             string         `json:"category"`
	ActionKind           string         `json:"action_kind"`
	HumanReadableContext string         `json:"human_readable_context"`
	StructuredPayload    map[string]any `json:"structured_payload"`
	ExpiresIn            time.Duration  `json:"expires_in"`
}

// ChangedFilePayload contains information about a modified file.
type ChangedFilePayload struct {
	Path        string `json:"path"`
	Operation   string `json:"operation"` // "create" | "modify" | "delete"
	Diff        string `json:"diff,omitempty"`
	ContentSize int64  `json:"content_size,omitempty"`
}

// DiagnosticPayload contains a diagnostic message.
type DiagnosticPayload struct {
	Level   string `json:"level"` // "info" | "warn" | "error"
	Message string `json:"message"`
}

// CompletionPayload contains session completion info.
type CompletionPayload struct {
	ExitCode   int    `json:"exit_code"`
	ExitError  string `json:"exit_error,omitempty"`
	DurationMs int64  `json:"duration_ms"`
}

// ErrorPayload contains error information.
type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// ValidationResult is returned by ValidateWorkspace.
type ValidationResult struct {
	Valid    bool     `json:"valid"`
	Errors   []string `json:"errors,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// RunHandle identifies a running agent process.
type RunHandle struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"started_at"`
}

// Accepted indicates a command was accepted for async processing.
type Accepted struct {
	Accepted bool   `json:"accepted"`
	Message  string `json:"message,omitempty"`
}

// RecoveryResult is returned by Recover.
type RecoveryResult struct {
	CanRecover bool              `json:"can_recover"`
	State      string            `json:"state,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	Error      string            `json:"error,omitempty"`
}

// CapabilitySet declares adapter capabilities.
type CapabilitySet struct {
	// SupportsResume indicates if the adapter can resume interrupted sessions
	SupportsResume bool `json:"supports_resume"`

	// SupportsApproval indicates if the adapter can request approvals
	SupportsApproval bool `json:"supports_approval"`

	// SupportsInterrupt indicates if the adapter can be interrupted
	SupportsInterrupt bool `json:"supports_interrupt"`

	// SupportsStreaming indicates if the adapter streams output
	SupportsStreaming bool `json:"supports_streaming"`

	// Version is the adapter version
	Version string `json:"version"`

	// AgentType is the type of agent this adapter supports
	AgentType string `json:"agent_type"`

	// AgentVersion is the version of the agent (if known)
	AgentVersion string `json:"agent_version,omitempty"`
}

// Adapter is the interface that all agent adapters must implement.
type Adapter interface {
	// ID returns the unique identifier for this adapter.
	ID() string

	// Capabilities returns the adapter's capability set.
	Capabilities() CapabilitySet

	// ValidateWorkspace validates that a workspace is suitable for this adapter.
	ValidateWorkspace(ws *domain.Workspace) ValidationResult

	// Start begins a new agent run in the given session.
	Start(ctx context.Context, session *domain.Session, input Input) (*RunHandle, error)

	// SubmitInput sends input (prompt) to a running agent.
	SubmitInput(ctx context.Context, run *RunHandle, prompt string) Accepted

	// Interrupt requests the agent to stop execution.
	Interrupt(ctx context.Context, run *RunHandle) Accepted

	// Observe returns a channel of normalized signals from the agent.
	Observe(ctx context.Context, run *RunHandle) <-chan AdapterSignal

	// DecideApproval sends an approval decision to the agent.
	DecideApproval(ctx context.Context, run *RunHandle, approvalID string, approved bool, reason string) Accepted

	// Recover attempts to recover session state after restart.
	Recover(ctx context.Context, session *domain.Session) RecoveryResult
}

// Input contains parameters for starting an agent run.
type Input struct {
	// WorkspacePath is the directory to run the agent in
	WorkspacePath string `json:"workspace_path"`

	// InitialPrompt is an optional initial prompt to send
	InitialPrompt string `json:"initial_prompt,omitempty"`

	// Args are additional command-line arguments
	Args []string `json:"args,omitempty"`

	// Env is environment variables for the agent process
	Env map[string]string `json:"env,omitempty"`

	// Secrets are sensitive environment variables (not logged)
	Secrets map[string]string `json:"-"`
}

// BaseAdapter provides common functionality for adapters.
type BaseAdapter struct {
	id           string
	capabilities CapabilitySet
}

// NewBaseAdapter creates a base adapter with the given ID and capabilities.
func NewBaseAdapter(id string, caps CapabilitySet) *BaseAdapter {
	return &BaseAdapter{
		id:           id,
		capabilities: caps,
	}
}

// ID returns the adapter ID.
func (a *BaseAdapter) ID() string {
	return a.id
}

// Capabilities returns the capability set.
func (a *BaseAdapter) Capabilities() CapabilitySet {
	return a.capabilities
}

// ValidateWorkspace provides a default workspace validation.
func (a *BaseAdapter) ValidateWorkspace(ws *domain.Workspace) ValidationResult {
	result := ValidationResult{Valid: true}

	if ws.Path == "" {
		result.Valid = false
		result.Errors = append(result.Errors, "workspace path is empty")
	}

	// Check if adapter is allowed
	if len(ws.AllowedAdapters) > 0 {
		allowed := false
		for _, id := range ws.AllowedAdapters {
			if id == a.id {
				allowed = true
				break
			}
		}
		if !allowed {
			result.Valid = false
			result.Errors = append(result.Errors, "adapter not allowed in workspace")
		}
	}

	return result
}
