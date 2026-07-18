package codex

import (
	"testing"

	"github.com/code-all-remote/car/sdk"
)

func TestCodex_Manifest(t *testing.T) {
	a := New("/usr/local/bin/codex")
	m := a.Manifest()

	if m.PluginID != "codex" {
		t.Errorf("expected plugin_id codex, got %s", m.PluginID)
	}
	if m.AgentType != "codex" {
		t.Errorf("expected agent_type codex, got %s", m.AgentType)
	}
	if !sdk.IsCompatible(m, sdk.SupportedProtocolRange) {
		t.Error("codex adapter must be SDK-compatible")
	}
}

func TestCodex_SelfCheck(t *testing.T) {
	// No executable configured -> not ready.
	a := New("")
	if err := a.SelfCheck(); err == nil {
		t.Error("expected self-check to fail without executable")
	}

	// Executable present -> ready.
	a2 := New("/usr/local/bin/codex")
	if err := a2.SelfCheck(); err != nil {
		t.Errorf("expected self-check to pass, got %v", err)
	}
}

func TestCodex_ValidateWorkspace(t *testing.T) {
	a := New("/usr/local/bin/codex")
	if v := a.ValidateWorkspace(""); v.Valid {
		t.Error("expected empty path to be invalid")
	}
	if v := a.ValidateWorkspace("/tmp/ws"); !v.Valid {
		t.Error("expected non-empty path to be valid")
	}
}

func TestCodex_RecoverDeclines(t *testing.T) {
	// Codex has no durable resume: Recover MUST return CanRecover=false
	// rather than fabricate state (docs/10 §Recovery).
	a := New("/usr/local/bin/codex")
	r := a.Recover("ses-1")
	if r.CanRecover {
		t.Error("codex must decline recovery rather than synthesize state")
	}
}

func TestCodex_StartFailsWithoutExec(t *testing.T) {
	a := New("")
	if _, err := a.Start(sdk.StartConfig{SessionID: "s", WorkspacePath: "/tmp"}); err == nil {
		t.Error("expected Start to fail without configured executable")
	}
}

func TestCodex_ApprovalDetection(t *testing.T) {
	a := New("/usr/local/bin/codex")
	if a.ApprovalDetection() == "" {
		t.Error("adapter must declare its approval detection mechanism")
	}
}

func TestCodex_Drain(t *testing.T) {
	a := New("/usr/local/bin/codex")
	if err := a.Drain(); err != nil {
		t.Errorf("Drain failed: %v", err)
	}
}

// Compile-time contract check.
func TestCodex_SatisfiesAdapter(t *testing.T) {
	var _ sdk.Adapter = New("/usr/local/bin/codex")
}
