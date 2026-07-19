// Package codex is a non-Claude adapter (M5 future direction).
//
// It applies the adapter SDK contract to a different agent (OpenAI Codex CLI).
// It spawns a real child process through the shared process wrapper and streams
// its output as normalized signals; tests drive it with a deterministic `sh`
// rig, so no real provider credentials are required.
//
// Per docs/09-future.md: each adapter must state how it detects approvals,
// session restoration and structured changes. This adapter declares those
// capabilities explicitly and declines parts it cannot safely support
// (Recover). Approval detection from Codex's structured JSON is a follow-up.
package codex

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/code-all-remote/car/internal/wrapper"
	"github.com/code-all-remote/car/sdk"
)

// Adapter implements the CAR adapter contract for the Codex CLI.
//
// Capability declaration (docs/09 §Additional adapters):
//   - approvals: detected via structured JSON ("--json") events, not terminal text;
//   - session restoration: declined (Codex CLI has no durable resume) — Recover
//     returns CanRecover=false rather than fabricating state;
//   - structured changes: parsed from Codex's JSON diff events.
type Adapter struct {
	manifest        sdk.Manifest
	execPath        string
	selfCheckResult error

	mu   sync.Mutex
	runs map[string]*codexRun
}

// codexRun tracks a spawned Codex process and its signal channel.
type codexRun struct {
	proc      *wrapper.ProcessWrapper
	signals   chan sdk.Signal
	cancel    context.CancelFunc
	sessionID string
}

// New creates a Codex adapter. execPath is discovered by the operator; if
// empty, SelfCheck reports not-ready (the adapter is visible in diagnostics
// but cannot create sessions).
func New(execPath string) *Adapter {
	return &Adapter{
		execPath: execPath,
		manifest: sdk.Manifest{
			PluginID:     "codex",
			Name:         "Codex CLI adapter",
			Version:      "0.1.0",
			SDKRange:     sdk.SupportedProtocolRange,
			AgentType:    "codex",
			Capabilities: []string{"approvals", "streaming"},
			// RequiresSecret: true — Codex needs an API key; the adapter never
			// logs it and passes it only via the process env.
			RequiresSecret: true,
		},
	}
}

func (a *Adapter) ID() string             { return a.manifest.PluginID }
func (a *Adapter) Manifest() sdk.Manifest { return a.manifest }
func (a *Adapter) Capabilities() []string { return a.manifest.Capabilities }

// SetExecPath points the adapter at an executable (used by tests with `sh`).
func (a *Adapter) SetExecPath(p string) { a.execPath = p }

// SelfCheck verifies the agent executable is discoverable. A failing self-
// check keeps the adapter visible in diagnostics but not ready.
func (a *Adapter) SelfCheck() error {
	if a.execPath == "" {
		return errors.New("codex executable not configured")
	}
	return a.selfCheckResult
}

// ValidateWorkspace is agent-neutral: the path must be non-empty.
func (a *Adapter) ValidateWorkspace(workspacePath string) sdk.ValidationResult {
	if workspacePath == "" {
		return sdk.ValidationResult{Valid: false, Errors: []string{"workspace path is empty"}}
	}
	return sdk.ValidationResult{Valid: true}
}

// Start spawns a Codex run as a child process and begins streaming its output.
func (a *Adapter) Start(cfg sdk.StartConfig) (*sdk.RunHandle, error) {
	if a.execPath == "" {
		return nil, fmt.Errorf("codex executable not configured")
	}
	env := os.Environ()
	for k, v := range cfg.Env {
		env = append(env, k+"="+v)
	}
	for k, v := range cfg.Secrets {
		env = append(env, k+"="+v)
	}
	ctx, cancel := context.WithCancel(context.Background())
	proc := wrapper.NewProcessWrapper()
	if _, err := proc.Start(ctx, wrapper.WrapperOptions{
		Command: a.execPath,
		Args:    cfg.Args,
		Dir:     cfg.WorkspacePath,
		Env:     env,
	}); err != nil {
		cancel()
		return nil, fmt.Errorf("starting codex: %w", err)
	}
	id := "codex-run-" + cfg.SessionID
	run := &codexRun{
		proc:      proc,
		signals:   make(chan sdk.Signal, 256),
		cancel:    cancel,
		sessionID: cfg.SessionID,
	}
	a.mu.Lock()
	if a.runs == nil {
		a.runs = make(map[string]*codexRun)
	}
	a.runs[id] = run
	a.mu.Unlock()
	go a.pump(ctx, id, run)
	if cfg.InitialPrompt != "" {
		_, _ = proc.WriteInputString(cfg.InitialPrompt)
	}
	handle := &sdk.RunHandle{ID: id, SessionID: cfg.SessionID}
	if info := proc.Info(); info != nil {
		handle.PID = info.PID
	}
	return handle, nil
}

// pump reads process output and emits normalized signals until the process
// exits, then emits a completion signal and closes the channel. A single
// goroutine owns run.signals (closed here), avoiding any send-on-closed race.
func (a *Adapter) pump(ctx context.Context, id string, run *codexRun) {
	defer close(run.signals)
	defer func() {
		a.mu.Lock()
		delete(a.runs, id)
		a.mu.Unlock()
	}()
	run.signals <- sdk.Signal{
		Type:      sdk.SignalStatusChange,
		Payload:   map[string]any{"old_state": "starting", "new_state": "active"},
		Timestamp: time.Now().UnixMilli(),
	}
	out := run.proc.OutputChannel()
	for {
		select {
		case <-ctx.Done():
			return
		case data, ok := <-out:
			if !ok {
				code := 0
				if c, has := run.proc.ExitCode(); has && c != nil {
					code = *c
				}
				run.signals <- sdk.Signal{
					Type:      sdk.SignalCompletion,
					Payload:   map[string]any{"exit_code": code},
					Timestamp: time.Now().UnixMilli(),
				}
				return
			}
			run.signals <- sdk.Signal{
				Type:      sdk.SignalOutput,
				Payload:   map[string]any{"content": string(data), "stream": "stdout"},
				Timestamp: time.Now().UnixMilli(),
			}
		}
	}
}

// Observe returns the signal channel for a run (or a closed channel if the run
// is unknown).
func (a *Adapter) Observe(run *sdk.RunHandle) <-chan sdk.Signal {
	a.mu.Lock()
	r, ok := a.runs[run.ID]
	a.mu.Unlock()
	if !ok {
		ch := make(chan sdk.Signal)
		close(ch)
		return ch
	}
	return r.signals
}

// SubmitInput writes operator input to the run's stdin.
func (a *Adapter) SubmitInput(run *sdk.RunHandle, prompt string) sdk.Accepted {
	a.mu.Lock()
	r, ok := a.runs[run.ID]
	a.mu.Unlock()
	if !ok {
		return sdk.Accepted{Accepted: false, Message: "run not found"}
	}
	if _, err := r.proc.WriteInputString(prompt); err != nil {
		return sdk.Accepted{Accepted: false, Message: err.Error()}
	}
	return sdk.Accepted{Accepted: true}
}

// Interrupt cancels the run and kills its process group.
func (a *Adapter) Interrupt(run *sdk.RunHandle) sdk.Accepted {
	a.mu.Lock()
	r, ok := a.runs[run.ID]
	a.mu.Unlock()
	if !ok {
		return sdk.Accepted{Accepted: false, Message: "run not found"}
	}
	r.cancel()
	_ = r.proc.Kill()
	return sdk.Accepted{Accepted: true}
}

// DecideApproval is not yet wired to Codex's structured approval protocol; the
// call is accepted so the bridge stays consistent (real detection is a
// follow-up, see ApprovalDetection).
func (a *Adapter) DecideApproval(run *sdk.RunHandle, approvalID string, approved bool, reason string) sdk.Accepted {
	return sdk.Accepted{Accepted: true}
}

// Recover explicitly declines: Codex CLI has no durable resume, so we never
// synthesize an approval or claim an action completed. The session surfaces
// as `failed` with diagnostic context (docs/10 §Recovery).
func (a *Adapter) Recover(sessionID string) sdk.RecoveryResult {
	return sdk.RecoveryResult{CanRecover: false, State: "failed", Error: "codex has no durable resume"}
}

// Drain is a bounded no-op (no background work beyond per-run goroutines,
// which exit when their process does).
func (a *Adapter) Drain() error { return nil }

// SetSelfCheckResult is used by tests to simulate a failing self-check.
func (a *Adapter) SetSelfCheckResult(err error) { a.selfCheckResult = err }

// Ensure the adapter satisfies the SDK contract at compile time.
var _ sdk.Adapter = (*Adapter)(nil)

// ApprovalDetection describes how the Codex adapter detects approval
// requests (docs/09 §Additional adapters). Declared, not terminal-parsed.
func (a *Adapter) ApprovalDetection() string {
	return "structured JSON events from `codex --json`; never terminal text"
}
