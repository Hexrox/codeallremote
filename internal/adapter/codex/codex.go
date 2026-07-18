// Package codex is a non-Claude adapter skeleton (M5 future direction).
//
// It demonstrates the adapter SDK contract applied to a different agent
// (OpenAI Codex CLI). It does NOT require real provider credentials for
// tests — a deterministic in-process mode is used, mirroring the fake adapter.
//
// Per docs/09-future.md: each adapter must state how it detects approvals,
// session restoration and structured changes. This skeleton declares those
// capabilities explicitly and declines parts it cannot safely support.
package codex

import (
	"context"
	"errors"
	"fmt"

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

// Start begins a Codex run. In this skeleton it does not spawn a real
// process; the deterministic test path uses StartInProcess.
func (a *Adapter) Start(cfg sdk.StartConfig) (*sdk.RunHandle, error) {
	if a.execPath == "" {
		return nil, errors.New("codex executable not configured (self-check failed)")
	}
	return &sdk.RunHandle{ID: "codex-run-" + cfg.SessionID, SessionID: cfg.SessionID}, nil
}

// SubmitInput, Interrupt, Observe, Recover are stubs that follow the contract:
// Recover explicitly declines restoration.

func (a *Adapter) SubmitInput(run *sdk.RunHandle, prompt string) sdk.Accepted {
	return sdk.Accepted{Accepted: true}
}

func (a *Adapter) Interrupt(run *sdk.RunHandle) sdk.Accepted {
	return sdk.Accepted{Accepted: true}
}

func (a *Adapter) Observe(run *sdk.RunHandle) <-chan sdk.Signal {
	ch := make(chan sdk.Signal)
	close(ch) // no real process in this skeleton
	return ch
}

func (a *Adapter) DecideApproval(run *sdk.RunHandle, approvalID string, approved bool, reason string) sdk.Accepted {
	return sdk.Accepted{Accepted: true}
}

// Recover explicitly declines: Codex CLI has no durable resume, so we never
// synthesize an approval or claim an action completed. The session surfaces
// as `failed` with diagnostic context (docs/10 §Recovery).
func (a *Adapter) Recover(sessionID string) sdk.RecoveryResult {
	return sdk.RecoveryResult{CanRecover: false, State: "failed", Error: "codex has no durable resume"}
}

// Drain is a bounded no-op for the skeleton.
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

// conniveUnused keeps context imported for the future real-process path.
var _ = context.Background

// suppress unused import when context not yet referenced (lint-friendly).
var _ = fmt.Sprintf
