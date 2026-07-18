// Package sdk defines the public CAR adapter plugin SDK contract.
//
// Plugins (server-side agent adapters) implement [Adapter] and register a
// [Manifest]. The core loads adapters from an explicit allowlist, validates
// the manifest, gates activation on SDK compatibility, and exposes
// capabilities through diagnostics. Incompatible plugins are rejected before
// startup (docs/21-plugin-sdk.md §Versioning).
//
// Security (docs/21 §Security): MVP plugins are trusted local binaries
// installed by the operator. This SDK MUST NOT imply untrusted marketplace
// code is safe; a sandboxed model is a separate design.
package sdk

// SDKVersion is the adapter SDK version this build speaks.
// Additive within a major; breaking changes bump the major and require a
// migration note (docs/21 §Versioning).
const SDKVersion = "1.0.0"

// SDKProtocolRange is the CAR protocol version range the SDK supports.
type SDKProtocolRange struct {
	Min int
	Max int
}

// SupportedProtocolRange is the current SDK's protocol compatibility envelope.
var SupportedProtocolRange = SDKProtocolRange{Min: 1, Max: 1}

// Manifest declares an adapter plugin and its compatibility envelope.
type Manifest struct {
	PluginID       string // unique, stable plugin id
	Name           string // human-readable
	Version        string // semantic version of the plugin
	SDKRange       SDKProtocolRange
	AgentType      string   // e.g. "claude-code", "codex"
	Capabilities   []string // e.g. "approvals", "streaming", "resume"
	RequiresSecret bool     // declares if the adapter needs a provider credential
}

// StartConfig carries the inputs an adapter needs to start a run.
type StartConfig struct {
	SessionID     string
	WorkspacePath string
	InitialPrompt string
	Args          []string
	Env           map[string]string
	// Secrets are sensitive environment values (provider tokens). They MUST
	// NOT be logged or persisted by the adapter beyond the process lifetime.
	Secrets map[string]string
}

// RunHandle identifies a running adapter process.
type RunHandle struct {
	ID        string
	SessionID string
	PID       int
}

// Accepted indicates a command was accepted for asynchronous processing.
type Accepted struct {
	Accepted bool
	Message  string
}

// SignalType enumerates normalized signals an adapter emits to core.
type SignalType string

const (
	SignalOutput          SignalType = "output"
	SignalStatusChange    SignalType = "status_change"
	SignalApprovalRequest SignalType = "approval_request"
	SignalChangedFile     SignalType = "changed_file"
	SignalDiagnostic      SignalType = "diagnostic"
	SignalCompletion      SignalType = "completion"
	SignalError           SignalType = "error"
)

// Signal is a normalized adapter event. Raw terminal data MAY be retained as
// an artifact but MUST NOT be the only representation of a significant event.
type Signal struct {
	Type      SignalType
	Payload   any
	Timestamp int64 // unix ms
}

// Adapter is the contract every agent plugin implements.
//
// Methods mirror docs/11-claude-code-adapter.md but are agent-neutral:
// validate workspace, start run, submit input, interrupt, observe output,
// surface approvals, report lifecycle, restore or explicitly decline.
type Adapter interface {
	// ID returns the plugin id from the manifest.
	ID() string

	// Manifest returns the plugin's manifest.
	Manifest() Manifest

	// ValidateWorkspace checks the workspace is suitable for this adapter.
	ValidateWorkspace(workspacePath string) ValidationResult

	// Start begins a new agent run.
	Start(cfg StartConfig) (*RunHandle, error)

	// SubmitInput sends a prompt to a running agent.
	SubmitInput(run *RunHandle, prompt string) Accepted

	// Interrupt requests the agent stop.
	Interrupt(run *RunHandle) Accepted

	// Observe returns a channel of normalized signals. Core consumes until
	// the channel closes (run exited).
	Observe(run *RunHandle) <-chan Signal

	// DecideApproval sends an approval decision to the agent.
	DecideApproval(run *RunHandle, approvalID string, approved bool, reason string) Accepted

	// Recover attempts to recover a session after a server restart. Adapters
	// that cannot safely restore MUST return CanRecover=false rather than
	// synthesize state.
	Recover(sessionID string) RecoveryResult

	// Capabilities returns the declared capability set.
	Capabilities() []string

	// SelfCheck reports whether the adapter is healthy enough to create
	// sessions (e.g. its executable is discoverable). A failing self-check
	// makes the plugin visible in diagnostics but not ready.
	SelfCheck() error

	// Drain gives the plugin a bounded shutdown window to finish work.
	Drain() error
}

// ValidationResult is returned by ValidateWorkspace.
type ValidationResult struct {
	Valid    bool
	Errors   []string
	Warnings []string
}

// RecoveryResult is returned by Recover.
type RecoveryResult struct {
	CanRecover bool
	State      string
	Error      string
}

// IsCompatible reports whether a manifest's SDK range overlaps the supported
// protocol range. The core rejects incompatible plugins before startup.
func IsCompatible(m Manifest, supported SDKProtocolRange) bool {
	if m.SDKRange.Min > supported.Max || m.SDKRange.Max < supported.Min {
		return false
	}
	return true
}

// ValidateManifest checks the manifest fields without activation. Returns
// the reason for rejection, or "" if sound.
func ValidateManifest(m Manifest) string {
	if m.PluginID == "" {
		return "plugin_id is required"
	}
	if m.Name == "" {
		return "name is required"
	}
	if m.Version == "" {
		return "version is required"
	}
	if m.SDKRange.Min <= 0 {
		return "sdk min must be positive"
	}
	if m.SDKRange.Max < m.SDKRange.Min {
		return "sdk max < min"
	}
	if m.AgentType == "" {
		return "agent_type is required"
	}
	return ""
}
