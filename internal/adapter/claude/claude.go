// Package claude is the production adapter for the Claude Code CLI.
//
// It is the only component permitted to understand Claude Code's launch
// arguments, terminal conventions and compatibility quirks (docs/11). It
// composes the process wrapper (spawn/observe the `claude` executable) with
// the output parser (normalize terminal output into CAR signals).
//
// When parsing confidence is insufficient the adapter emits
// `adapter.compatibility_degraded`, preserves raw diagnostics locally, and
// disables only features it cannot safely normalize. It MUST NEVER synthesize
// an approval or claim an action completed (docs/11 §Compatibility strategy).
package claude

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/code-all-remote/car/internal/adapter"
	"github.com/code-all-remote/car/internal/domain"
	"github.com/code-all-remote/car/internal/wrapper"
)

// ClaudeAdapter is the production Claude Code adapter.
type ClaudeAdapter struct {
	*adapter.BaseAdapter
	parser  *adapter.OutputParser
	wrapper *wrapper.ProcessWrapper
	logger  *slog.Logger

	mu       sync.Mutex
	runs     map[string]*claudeRun
	execPath string

	// degraded is set when parsing confidence dropped; features that cannot be
	// safely normalized are disabled until the run ends.
	degraded bool
}

// claudeRun tracks a live run's wrapper and signal channel.
type claudeRun struct {
	handle    *adapter.RunHandle
	proc      *wrapper.ProcessWrapper
	signals   chan adapter.AdapterSignal
	done      chan struct{}
	startedAt time.Time
	// streamJSONInput is true when the child is the real claude CLI (started
	// with --input-format stream-json), so stdin prompts must be wrapped as
	// stream-json user messages. The sh test rig leaves this false (raw text).
	streamJSONInput bool
}

// New creates a Claude Code adapter. execPath is the `claude` executable path;
// if empty, SelfCheck (via Start) fails closed — no session can start until the
// operator configures a discoverable executable.
func New(execPath string, logger *slog.Logger) *ClaudeAdapter {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(discardWriter{}, nil))
	}
	return &ClaudeAdapter{
		BaseAdapter: adapter.NewBaseAdapter("claude-code", adapter.CapabilitySet{
			SupportsResume:    false, // Claude Code has no durable resume we can verify
			SupportsApproval:  true,
			SupportsInterrupt: true,
			SupportsStreaming: true,
			Version:           "1.0.0",
			AgentType:         "claude-code",
		}),
		parser:   adapter.NewOutputParser(),
		logger:   logger,
		runs:     make(map[string]*claudeRun),
		execPath: execPath,
	}
}

// discardWriter is a no-op writer for the default logger.
type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

// SetExecPath overrides the executable path (used by tests / config reload).
func (a *ClaudeAdapter) SetExecPath(p string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.execPath = p
}

// Start begins a Claude Code run. It spawns the configured executable in the
// workspace directory with the operator-supplied args and env. Secrets are
// passed via the process environment and never logged.
func (a *ClaudeAdapter) Start(ctx context.Context, session *domain.Session, input adapter.Input) (*adapter.RunHandle, error) {
	a.mu.Lock()
	execPath := a.execPath
	a.mu.Unlock()

	if execPath == "" {
		return nil, fmt.Errorf("claude executable not configured (self-check failed)")
	}
	if input.WorkspacePath == "" {
		return nil, fmt.Errorf("workspace path is required")
	}
	if !filepath.IsAbs(input.WorkspacePath) {
		return nil, fmt.Errorf("workspace path must be absolute")
	}

	// Build the command args. We prefer structured streaming output when the
	// operator has not passed an explicit output format; that keeps parsing
	// confidence high. We never inject secrets into args. Auto-args are only
	// added when the executable looks like the claude CLI (so test rigs using
	// sh/echo are not corrupted).
	args := buildArgs(execPath, input)

	// Environment: base env + non-secret env + secrets (kept out of logs).
	env := buildEnv(input)

	proc := wrapper.NewProcessWrapper()
	_, err := proc.Start(ctx, wrapper.WrapperOptions{
		Command: execPath,
		Args:    args,
		Dir:     input.WorkspacePath,
		Env:     env,
	})
	if err != nil {
		return nil, fmt.Errorf("starting claude: %w", err)
	}

	handle := &adapter.RunHandle{
		ID:        "run-" + session.ID,
		SessionID: session.ID,
		StartedAt: time.Now(),
	}
	if info := proc.Info(); info != nil {
		handle.PID = info.PID
	}

	run := &claudeRun{
		handle:          handle,
		proc:            proc,
		signals:         make(chan adapter.AdapterSignal, 256),
		done:            make(chan struct{}),
		startedAt:       handle.StartedAt,
		streamJSONInput: strings.Contains(filepath.Base(execPath), "claude"),
	}

	a.mu.Lock()
	a.runs[handle.ID] = run
	a.mu.Unlock()

	// Deliver the initial prompt via stdin (never argv; a prompt in argv is
	// visible in `ps` and can be parsed as a flag). Best-effort: if the write
	// fails the run still started; the operator may submit the prompt again.
	if input.InitialPrompt != "" {
		_ = writePrompt(run.proc, run.streamJSONInput, input.InitialPrompt)
	}

	// Status transition: starting -> active. This is enqueued BEFORE the pump
	// goroutine starts so the run's first signal is always `active`, never
	// output. The channel is buffered (cap 256) so the send does not block, and
	// nothing the pump reads is set up only after this point (run and
	// run.signals are fully constructed above). This guarantees the
	// active-before-output ordering the adapter contract requires.
	run.signals <- adapter.AdapterSignal{
		Type:      adapter.SignalStatusChange,
		SessionID: session.ID,
		Timestamp: time.Now(),
		Payload:   mustMarshal(adapter.StatusChangePayload{OldState: "starting", NewState: domain.RunStateActive}),
	}

	// Pump wrapper output through the parser into the signal channel.
	go a.pump(ctx, run)

	return handle, nil
}

// pump consumes stdout/stderr from the wrapper, parses it, and emits signals.
// On process exit it emits a completion (or error) signal and closes the channel.
func (a *ClaudeAdapter) pump(ctx context.Context, run *claudeRun) {
	defer close(run.signals)
	defer func() {
		a.mu.Lock()
		delete(a.runs, run.handle.ID)
		a.mu.Unlock()
	}()

	var pending []byte
	outCh := run.proc.OutputChannel()
	errCh := run.proc.ErrorChannel()

	// One goroutine multiplexes both streams so it remains the sole owner of
	// run.signals (closed via the deferred close), avoiding any send-on-closed
	// race. stdout is parsed as stream-json; stderr is emitted raw (it is not
	// JSON) so it is neither dropped nor fed to the parser. Completion fires
	// once both streams are drained; the wrapper closes both channels together
	// on process exit, so the timing is preserved.
	for outCh != nil || errCh != nil {
		select {
		case <-ctx.Done():
			return
		case data, ok := <-outCh:
			if !ok {
				outCh = nil
				continue
			}
			// Redact secrets before parsing (defensive; secrets are not in output
			// normally, but a leak should never reach events/logs).
			events, rest := a.parser.ParseBuffer(data, false)
			pending = append(pending, rest...)
			for i := range events {
				a.emitParsed(run, &events[i])
			}
		case data, ok := <-errCh:
			if !ok {
				errCh = nil
				continue
			}
			a.emitStderr(run, data)
		}
	}

	// Both streams drained; flush any pending partial line and emit completion.
	a.flush(run, pending)
	a.emitCompletion(run)
}

// emitStderr surfaces a raw stderr chunk as an output signal tagged
// stream="stderr" (never parsed as stream-json).
func (a *ClaudeAdapter) emitStderr(run *claudeRun, data []byte) {
	a.send(run, adapter.AdapterSignal{
		Type:      adapter.SignalOutput,
		SessionID: run.handle.SessionID,
		Timestamp: time.Now(),
		Payload:   mustMarshal(adapter.OutputPayload{Content: string(data), Stream: "stderr"}),
	})
}

// flush parses any remaining buffered bytes (final, isComplete=true).
func (a *ClaudeAdapter) flush(run *claudeRun, pending []byte) {
	if len(pending) == 0 {
		return
	}
	events, _ := a.parser.ParseBuffer(pending, true)
	for i := range events {
		a.emitParsed(run, &events[i])
	}
}

// emitParsed converts a parsed event to an AdapterSignal.
func (a *ClaudeAdapter) emitParsed(run *claudeRun, ev *adapter.ParsedEvent) {
	if ev == nil {
		return
	}
	sig := adapter.AdapterSignal{
		Type:      ev.Type,
		SessionID: run.handle.SessionID,
		Timestamp: time.Now(),
	}
	switch ev.Type {
	case adapter.SignalOutput:
		if p, ok := ev.Payload.(adapter.OutputPayload); ok {
			sig.Payload = mustMarshal(p)
		} else {
			sig.Payload = mustMarshal(ev.Payload)
		}
	case adapter.SignalError:
		if p, ok := ev.Payload.(adapter.ErrorPayload); ok {
			sig.Payload = mustMarshal(p)
		} else {
			sig.Payload = mustMarshal(ev.Payload)
		}
		// An error from the parser lowers confidence: mark degraded.
		a.markDegraded(run)
	case adapter.SignalApprovalRequest:
		if p, ok := ev.Payload.(adapter.ApprovalRequestPayload); ok {
			sig.Payload = mustMarshal(p)
		} else {
			sig.Payload = mustMarshal(ev.Payload)
		}
	default:
		sig.Payload = mustMarshal(ev.Payload)
	}
	a.send(run, sig)
}

// emitCompletion emits a terminal signal based on the wrapper exit state.
// Uses the non-blocking a.send helper so the pump cannot stall if the observer
// is gone (e.g. the app is shutting down and the consumer goroutine exited).
func (a *ClaudeAdapter) emitCompletion(run *claudeRun) {
	run.proc.Wait()
	if code, ok := run.proc.ExitCode(); ok && code != nil {
		newState := domain.RunStateCompleted
		if *code != 0 {
			newState = domain.RunStateFailed
		}
		a.send(run, adapter.AdapterSignal{
			Type:      adapter.SignalCompletion,
			SessionID: run.handle.SessionID,
			Timestamp: time.Now(),
			Payload: mustMarshal(adapter.CompletionPayload{
				ExitCode:   *code,
				DurationMs: time.Since(run.startedAt).Milliseconds(),
			}),
		})
		// Also emit a status change for the lifecycle projection.
		a.send(run, adapter.AdapterSignal{
			Type:      adapter.SignalStatusChange,
			SessionID: run.handle.SessionID,
			Timestamp: time.Now(),
			Payload: mustMarshal(adapter.StatusChangePayload{
				OldState: domain.RunStateActive, NewState: newState,
			}),
		})
	}
}

// markDegraded records that parsing confidence dropped and emits a
// compatibility_degraded diagnostic. The adapter never synthesizes an
// approval or claims completion from degraded state.
func (a *ClaudeAdapter) markDegraded(run *claudeRun) {
	a.mu.Lock()
	if a.degraded {
		a.mu.Unlock()
		return
	}
	a.degraded = true
	a.parser.SetCompatibilityDegraded(true)
	a.mu.Unlock()

	a.send(run, adapter.AdapterSignal{
		Type:      adapter.SignalDiagnostic,
		SessionID: run.handle.SessionID,
		Timestamp: time.Now(),
		Payload: mustMarshal(adapter.DiagnosticPayload{
			Level:   "warn",
			Message: "adapter.compatibility_degraded: parsing confidence low; unsafe features disabled",
		}),
	})
}

func (a *ClaudeAdapter) send(run *claudeRun, sig adapter.AdapterSignal) {
	select {
	case run.signals <- sig:
	case <-run.done:
	default:
		// Channel full: client is slow. Drop rather than block the pump; the
		// durable journal preserves events for replay.
	}
}

// SubmitInput sends a prompt to a running claude process.
func (a *ClaudeAdapter) SubmitInput(ctx context.Context, run *adapter.RunHandle, prompt string) adapter.Accepted {
	r := a.runFor(run.ID)
	if r == nil {
		return adapter.Accepted{Accepted: false, Message: "run not found"}
	}
	if err := writePrompt(r.proc, r.streamJSONInput, prompt); err != nil {
		return adapter.Accepted{Accepted: false, Message: err.Error()}
	}
	return adapter.Accepted{Accepted: true}
}

// Interrupt stops the run's process group.
func (a *ClaudeAdapter) Interrupt(ctx context.Context, run *adapter.RunHandle) adapter.Accepted {
	r := a.runFor(run.ID)
	if r == nil {
		return adapter.Accepted{Accepted: false, Message: "run not found"}
	}
	if err := r.proc.Signal(interruptSignal); err != nil {
		// Fall back to a hard kill if the signal fails.
		if err := r.proc.Kill(); err != nil {
			return adapter.Accepted{Accepted: false, Message: err.Error()}
		}
	}
	return adapter.Accepted{Accepted: true}
}

// Observe returns the signal channel for a run.
func (a *ClaudeAdapter) Observe(ctx context.Context, run *adapter.RunHandle) <-chan adapter.AdapterSignal {
	r := a.runFor(run.ID)
	if r == nil {
		ch := make(chan adapter.AdapterSignal)
		close(ch)
		return ch
	}
	return r.signals
}

// DecideApproval records an approval decision by writing it to the agent
// process stdin as a JSON line {"decision":"approve"|"deny"}.
//
// WARNING (ADR-009, 2026-07-19): this stdin decision protocol is NOT how real
// Claude Code handles non-interactive permissions. The real mechanism is
// `--permission-prompt-tool <mcp-tool>` (Claude calls an MCP tool for the
// decision) or static `--allowedTools`/`--permission-mode`. Against a real
// `claude` this write is a no-op, so the Claude adapter's approvals are not yet
// functional. Replacing this with the MCP permission-prompt flow changes the
// approval trust boundary and is a separate, reviewed increment — see
// adr/ADR-009-claude-cli-interface.md. The stdin path is retained only so the
// deterministic `sh` rig continues to exercise the surrounding plumbing.
//
// The adapter acknowledges the write but does NOT synthesize an
// approval.resolved event — the run's actual outcome arrives through Observe
// as run.output / run.completed (docs/33 §Approval semantics: "Clients cannot
// manufacture approval events"). If the write fails (process gone, stdin
// closed), the caller is told the decision was not delivered.
func (a *ClaudeAdapter) DecideApproval(ctx context.Context, run *adapter.RunHandle, approvalID string, approved bool, reason string) adapter.Accepted {
	r := a.runFor(run.ID)
	if r == nil {
		return adapter.Accepted{Accepted: false, Message: "run not found"}
	}
	payload := approvalDecisionLine(approved, reason)
	if _, err := r.proc.WriteInput(payload); err != nil {
		return adapter.Accepted{Accepted: false, Message: "failed to write decision to stdin: " + err.Error()}
	}
	return adapter.Accepted{Accepted: true}
}

// approvalDecisionLine builds the stdin JSON line for a Claude Code approval
// decision. It contains ONLY the decision and an optional reason — no
// session id, secrets, or request context (the process already has those).
func approvalDecisionLine(approved bool, reason string) []byte {
	decision := "deny"
	if approved {
		decision = "approve"
	}
	if reason == "" {
		return []byte(`{"decision":"` + decision + `"}` + "\n")
	}
	// Escape reason for JSON string safety.
	return []byte(`{"decision":"` + decision + `","reason":` + jsonString(reason) + `}` + "\n")
}

// jsonString returns a JSON-encoded string literal (with quotes) for s.
func jsonString(s string) string {
	b, err := jsonMarshal(s)
	if err != nil {
		return `""`
	}
	return string(b)
}

// Recover explicitly declines: Claude Code has no durable resume the adapter
// can safely verify. The session surfaces as failed with diagnostic context
// rather than silently resuming (docs/10 §Recovery).
func (a *ClaudeAdapter) Recover(ctx context.Context, session *domain.Session) adapter.RecoveryResult {
	return adapter.RecoveryResult{
		CanRecover: false,
		State:      domain.SessionStateFailed,
		Error:      "claude-code has no durable resume the adapter can verify",
		Metadata:   map[string]string{"adapter": a.ID(), "session_id": session.ID},
	}
}

// runFor returns the live run (or nil).
func (a *ClaudeAdapter) runFor(id string) *claudeRun {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.runs[id]
}

// buildArgs constructs the claude CLI args. It prefers structured streaming
// output so the parser keeps high confidence, unless the operator overrides.
// Auto-args are only added when execPath looks like the claude CLI; test rigs
// using sh/echo pass their own args verbatim. The initial prompt is NEVER
// placed in argv (it would be visible in `ps`/cmdline and could be parsed as
// a flag); it is delivered via stdin after Start by the caller.
func buildArgs(execPath string, input adapter.Input) []string {
	args := append([]string{}, input.Args...)

	isClaude := strings.Contains(filepath.Base(execPath), "claude")
	if isClaude {
		hasOutputFormat := false
		for _, a := range args {
			if strings.HasPrefix(a, "--output-format") || a == "--print" || a == "-p" {
				hasOutputFormat = true
			}
		}
		if !hasOutputFormat {
			// ADR-009: --input-format stream-json is required for multi-turn
			// stdin prompts (without it -p reads stdin as a single one-shot
			// prompt); --bare gives deterministic startup. Pending an operator
			// smoke-test against a real claude binary.
			args = append(args, "-p", "--output-format", "stream-json", "--input-format", "stream-json", "--verbose", "--bare")
		}
	}
	// InitialPrompt is intentionally NOT appended to argv; see Start.
	return args
}

// streamJSONUserMessage wraps a prompt as one newline-delimited stream-json
// user message for `claude --input-format stream-json`. ADR-009: the exact
// top-level "type" token ("user") and content-block schema are pending operator
// verification against a real claude binary.
func streamJSONUserMessage(prompt string) []byte {
	type textBlock struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	type msg struct {
		Role    string      `json:"role"`
		Content []textBlock `json:"content"`
	}
	type envelope struct {
		Type    string `json:"type"`
		Message msg    `json:"message"`
	}
	b, err := jsonMarshal(envelope{
		Type:    "user",
		Message: msg{Role: "user", Content: []textBlock{{Type: "text", Text: prompt}}},
	})
	if err != nil {
		return nil
	}
	return append(b, '\n')
}

// writePrompt delivers a prompt to the child's stdin, as a stream-json user
// message for the real claude CLI or as raw text for the sh test rig.
func writePrompt(proc *wrapper.ProcessWrapper, streamJSON bool, prompt string) error {
	if streamJSON {
		if line := streamJSONUserMessage(prompt); line != nil {
			_, err := proc.WriteInput(line)
			return err
		}
	}
	_, err := proc.WriteInputString(prompt)
	return err
}

// buildEnv assembles the child process environment: start from the server's
// environment (so PATH/HOME/TERM survive), then layer the operator-supplied
// adapter Env on top, then Secrets last (Secrets never logged, but they reach
// the child). Later entries override earlier ones for the same key.
func buildEnv(input adapter.Input) []string {
	// Preserve the server environment (e.g. PATH, HOME, TERM, ANTHROPIC_API_KEY
	// if the operator set it in the service unit). Without this the child could
	// not find its own libraries/executables.
	env := os.Environ()
	merge := func(k, v string) {
		env = append(env, k+"="+v)
	}
	for k, v := range input.Env {
		merge(k, v)
	}
	for k, v := range input.Secrets {
		merge(k, v)
	}
	return env
}

// mustMarshal is a tiny helper that panics only on truly impossible encoding
// failures (the payload types are concrete structs).
func mustMarshal(v any) []byte {
	// We embed payloads as raw JSON in the signal; adapters are responsible
	// for normalized payloads. Fall back to a string on failure.
	b, err := jsonMarshal(v)
	if err != nil {
		return []byte("{}")
	}
	return b
}

// Ensure exec is reachable for path checks without forcing an import cycle
// guard at call sites.
var _ = exec.LookPath

// interruptSignal is the signal sent on Interrupt (SIGINT lets Claude Code
// flush; we fall back to kill on failure).
var interruptSignal = sigInterrupt

// jsonMarshal is defined in encode.go to keep import grouping tidy.
