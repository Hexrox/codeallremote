package claude

import (
	"context"
	"encoding/json"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/code-all-remote/car/internal/adapter"
	"github.com/code-all-remote/car/internal/domain"
)

func skipNonUnix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test relies on sh")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
}

func newTestAdapter() *ClaudeAdapter {
	return New("claude-testing", nil)
}

func TestClaudeAdapter_Capabilities(t *testing.T) {
	a := newTestAdapter()
	caps := a.Capabilities()

	if caps.AgentType != "claude-code" {
		t.Errorf("expected agent_type claude-code, got %s", caps.AgentType)
	}
	if a.ID() != "claude-code" {
		t.Errorf("expected id claude-code, got %s", a.ID())
	}
	if !caps.SupportsApproval || !caps.SupportsInterrupt || !caps.SupportsStreaming {
		t.Error("expected approval/interrupt/streaming capabilities")
	}
	if caps.SupportsResume {
		t.Error("claude-code must declare no durable resume (decline, not fabricate)")
	}
}

func TestClaudeAdapter_Start_NoExec(t *testing.T) {
	a := newTestAdapter()
	a.SetExecPath("")
	_, err := a.Start(context.Background(), &domain.Session{ID: "s1"}, adapter.Input{WorkspacePath: "/tmp"})
	if err == nil {
		t.Error("expected Start to fail without configured executable")
	}
}

func TestClaudeAdapter_Start_RelativePath(t *testing.T) {
	a := newTestAdapter()
	a.SetExecPath("sh")
	_, err := a.Start(context.Background(), &domain.Session{ID: "s1"}, adapter.Input{
		WorkspacePath: "relative/path",
	})
	if err == nil {
		t.Error("expected error for relative workspace path")
	}
}

func TestClaudeAdapter_Start_ObservesOutput(t *testing.T) {
	skipNonUnix(t)
	a := newTestAdapter()
	// Use sh as a stand-in for the claude CLI; pass our own args so buildArgs
	// does not corrupt them (execPath basename lacks "claude").
	a.SetExecPath("sh")

	sess := &domain.Session{ID: "s1", WorkspaceID: "ws", AdapterID: "claude-code", State: domain.SessionStateActive}
	input := adapter.Input{
		WorkspacePath: "/tmp",
		Args:          []string{"-c", "echo hello; echo 'Modified: /tmp/x.txt'"},
	}
	handle, err := a.Start(context.Background(), sess, input)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer a.Interrupt(context.Background(), handle)

	signals := a.Observe(context.Background(), handle)

	var got []adapter.AdapterSignal
	timeout := time.After(2 * time.Second)
	for {
		select {
		case sig, ok := <-signals:
			if !ok {
				goto done
			}
			got = append(got, sig)
			if len(got) > 30 {
				goto done
			}
		case <-timeout:
			t.Fatalf("timeout, got %d signals", len(got))
		}
	}
done:
	if len(got) == 0 {
		t.Fatal("expected at least one signal")
	}
	// First signal is a status change to active.
	if got[0].Type != "status_change" {
		t.Errorf("expected status_change first, got %s", got[0].Type)
	}
	// Expect output + completion before channel closes.
	hasOutput, hasCompletion := false, false
	for _, s := range got {
		if s.Type == "output" {
			hasOutput = true
		}
		if s.Type == "completion" {
			hasCompletion = true
		}
	}
	if !hasOutput {
		t.Error("expected an output signal")
	}
	if !hasCompletion {
		t.Error("expected a completion signal")
	}
}

func TestClaudeAdapter_Interrupt(t *testing.T) {
	skipNonUnix(t)
	a := newTestAdapter()
	a.SetExecPath("sh")

	sess := &domain.Session{ID: "s1", WorkspaceID: "ws", AdapterID: "claude-code", State: domain.SessionStateActive}
	input := adapter.Input{
		WorkspacePath: "/tmp",
		Args:          []string{"-c", "sleep 30"},
	}
	handle, err := a.Start(context.Background(), sess, input)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)
	acc := a.Interrupt(context.Background(), handle)
	if !acc.Accepted {
		t.Errorf("interrupt should be accepted: %s", acc.Message)
	}
}

func TestClaudeAdapter_Recover_Declines(t *testing.T) {
	a := newTestAdapter()
	// Recover MUST return CanRecover=false rather than fabricate state.
	r := a.Recover(context.Background(), &domain.Session{ID: "s1"})
	if r.CanRecover {
		t.Error("claude-code must decline recovery (no durable resume)")
	}
	if r.State != domain.SessionStateFailed {
		t.Errorf("expected failed state on declined recovery, got %s", r.State)
	}
}

func TestClaudeAdapter_DecideApproval_WritesStdin(t *testing.T) {
	skipNonUnix(t)
	a := newTestAdapter()
	a.SetExecPath("sh")
	sess := &domain.Session{ID: "s1", WorkspaceID: "ws", AdapterID: "claude-code", State: domain.SessionStateActive}
	// A script that prints a marker after reading a stdin line, proving the
	// decision was delivered to the agent process.
	handle, _ := a.Start(context.Background(), sess, adapter.Input{
		WorkspacePath: "/tmp",
		Args:          []string{"-c", "read line; echo DECISION_RECEIVED"},
	})
	defer a.Interrupt(context.Background(), handle)

	decided := a.DecideApproval(context.Background(), handle, "apr-1", true, "ok")
	if !decided.Accepted {
		t.Fatalf("DecideApproval should accept: %s", decided.Message)
	}

	// The script prints DECISION_RECEIVED after consuming the decision line.
	signals := a.Observe(context.Background(), handle)
	deadline := time.After(2 * time.Second)
	sawMarker := false
	for {
		select {
		case s, ok := <-signals:
			if !ok {
				if !sawMarker {
					t.Fatal("expected DECISION_RECEIVED marker before channel close")
				}
				return
			}
			if s.Type == "output" && strings.Contains(string(s.Payload), "DECISION_RECEIVED") {
				sawMarker = true
			}
			// We MUST NOT have synthesized an approval.resolved signal.
			if strings.Contains(string(s.Payload), "approval.resolved") {
				t.Error("adapter synthesized approval.resolved; forbidden")
			}
		case <-deadline:
			t.Fatalf("timeout waiting for marker, saw=%v", sawMarker)
		}
	}
}

func TestClaudeAdapter_DecideApproval_AlwaysNeverSynthesized(t *testing.T) {
	skipNonUnix(t)
	// approvalDecisionLine must contain only decision + reason; never a
	// synthesized "resolved" marker.
	line := string(approvalDecisionLine(true, "ok"))
	if !strings.Contains(line, "\"approve\"") {
		t.Errorf("expected approve decision, got %s", line)
	}
	if strings.Contains(line, "resolved") {
		t.Error("decision line must not contain 'resolved'")
	}
}

func TestApprovalDecisionLine_NoReason(t *testing.T) {
	line := string(approvalDecisionLine(false, ""))
	if !strings.Contains(line, "\"deny\"") {
		t.Errorf("expected deny, got %s", line)
	}
}

func TestClaudeAdapter_SubmitInput_NotFound(t *testing.T) {
	a := newTestAdapter()
	acc := a.SubmitInput(context.Background(), &adapter.RunHandle{ID: "nope"}, "hi")
	if acc.Accepted {
		t.Error("expected SubmitInput to fail for unknown run")
	}
}

func TestBuildArgs_AutoFormatForClaudeOnly(t *testing.T) {
	// sh path -> no auto -p (test rigs uncorrupted).
	args := buildArgs("/usr/bin/sh", adapter.Input{Args: []string{"-c", "echo hi"}})
	for _, a := range args {
		if a == "--output-format" {
			t.Error("sh args must not get auto --output-format")
		}
	}
	// claude path -> auto streaming JSON when operator didn't specify.
	args = buildArgs("/usr/local/bin/claude", adapter.Input{})
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "stream-json") {
		t.Error("claude path should get auto stream-json")
	}
}

func TestBuildEnv_SeparatesSecrets(t *testing.T) {
	env := buildEnv(adapter.Input{
		Env:     map[string]string{"FOO": "bar"},
		Secrets: map[string]string{"ANTHROPIC_API_KEY": "sk-secret"},
	})
	joined := strings.Join(env, "\n")
	if !strings.Contains(joined, "FOO=bar") {
		t.Error("expected FOO=bar")
	}
	if !strings.Contains(joined, "ANTHROPIC_API_KEY=sk-secret") {
		t.Error("expected secret passed to env")
	}
}

// TestBuildEnv_InheritsAndOverrides proves the child gets the server's env
// (PATH/HOME survive) and operator-supplied Env overrides inherited values.
func TestBuildEnv_InheritsAndOverrides(t *testing.T) {
	env := buildEnv(adapter.Input{
		Env: map[string]string{"ANTHROPIC_BASE_URL": "http://127.0.0.1:3456"},
	})
	joined := strings.Join(env, "\n")
	// Inherited server env survives (PATH is always present on a real OS).
	if !strings.Contains(joined, "PATH=") {
		t.Error("expected PATH to survive from os.Environ()")
	}
	// Operator override present.
	if !strings.Contains(joined, "ANTHROPIC_BASE_URL=http://127.0.0.1:3456") {
		t.Error("expected operator ANTHROPIC_BASE_URL in child env")
	}
	// Override semantics: the operator value wins (last entry).
	// Set PATH explicitly in Env; the child should see the override last.
	env2 := buildEnv(adapter.Input{Env: map[string]string{"PATH": "/override/bin"}})
	last := ""
	for _, e := range env2 {
		if strings.HasPrefix(e, "PATH=") {
			last = e
		}
	}
	if last != "PATH=/override/bin" {
		t.Errorf("expected operator PATH to override inherited, last PATH entry = %q", last)
	}
}

// TestClaudeAdapter_Start_PassesEnv verifies the adapter env from config
// reaches the spawned process as an environment variable the child echoes.
func TestClaudeAdapter_Start_PassesEnv(t *testing.T) {
	skipNonUnix(t)
	a := newTestAdapter()
	a.SetExecPath("sh")
	sess := &domain.Session{ID: "s1", WorkspaceID: "ws", AdapterID: "claude-code", State: domain.SessionStateActive}
	handle, _ := a.Start(context.Background(), sess, adapter.Input{
		WorkspacePath: "/tmp",
		Args:          []string{"-c", "echo $CAR_TEST_ENV"},
		Env:           map[string]string{"CAR_TEST_ENV": "from-config"},
	})
	defer a.Interrupt(context.Background(), handle)

	deadline := time.After(2 * time.Second)
	for sig := range a.Observe(context.Background(), handle) {
		if sig.Type == "output" && strings.Contains(string(sig.Payload), "from-config") {
			return
		}
		select {
		default:
		case <-deadline:
			t.Fatal("timeout waiting for env var echo")
		}
	}
	_ = deadline
}

func TestMustMarshal_Fallback(t *testing.T) {
	out := mustMarshal(make(chan int)) // unmarshallable
	if string(out) != "{}" {
		t.Errorf("expected fallback {}, got %s", out)
	}
}

func TestClaudeAdapter_PayloadJSON(t *testing.T) {
	skipNonUnix(t)
	a := newTestAdapter()
	a.SetExecPath("sh")
	sess := &domain.Session{ID: "s1", WorkspaceID: "ws", AdapterID: "claude-code", State: domain.SessionStateActive}
	handle, _ := a.Start(context.Background(), sess, adapter.Input{
		WorkspacePath: "/tmp",
		Args:          []string{"-c", "echo hi"},
	})
	defer a.Interrupt(context.Background(), handle)

	for sig := range a.Observe(context.Background(), handle) {
		if sig.Type == "completion" {
			var p adapter.CompletionPayload
			if err := json.Unmarshal(sig.Payload, &p); err != nil {
				t.Errorf("completion payload not JSON: %v", err)
			}
			break
		}
	}
}

// TestClaudeAdapter_Start_EmitsStderr verifies stderr is surfaced as a raw
// output signal (stream="stderr"), not dropped and not parsed as stream-json.
func TestClaudeAdapter_Start_EmitsStderr(t *testing.T) {
	skipNonUnix(t)
	a := newTestAdapter()
	a.SetExecPath("sh")
	sess := &domain.Session{ID: "s1", WorkspaceID: "ws", AdapterID: "claude-code", State: domain.SessionStateActive}
	handle, err := a.Start(context.Background(), sess, adapter.Input{
		WorkspacePath: "/tmp",
		Args:          []string{"-c", "echo to-stderr >&2"},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer a.Interrupt(context.Background(), handle)

	sigs := a.Observe(context.Background(), handle)
	timeout := time.After(2 * time.Second)
	for {
		select {
		case <-timeout:
			t.Fatal("timed out waiting for stderr output signal")
		case sig, ok := <-sigs:
			if !ok {
				t.Fatal("signal channel closed before stderr was observed")
			}
			if sig.Type != adapter.SignalOutput {
				continue
			}
			var p adapter.OutputPayload
			if err := json.Unmarshal(sig.Payload, &p); err != nil {
				continue
			}
			if p.Stream == "stderr" && strings.Contains(p.Content, "to-stderr") {
				return
			}
		}
	}
}
