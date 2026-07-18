package wrapper

import (
	"bytes"
	"context"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

func skipWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping test on Windows")
	}
}

func TestProcessWrapper_Start(t *testing.T) {
	skipWindows(t)

	w := NewProcessWrapper()
	ctx := context.Background()

	opts := WrapperOptions{
		Command: "echo",
		Args:    []string{"hello"},
		Dir:     "/tmp",
	}

	info, err := w.Start(ctx, opts)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if info.PID <= 0 {
		t.Errorf("expected positive PID, got %d", info.PID)
	}
	if info.Command != "echo" {
		t.Errorf("expected command 'echo', got '%s'", info.Command)
	}
	if info.Dir != "/tmp" {
		t.Errorf("expected dir '/tmp', got '%s'", info.Dir)
	}

	// Wait for exit
	w.Wait()

	state := w.State()
	if state != StateExited {
		t.Errorf("expected state exited, got %s", state)
	}

	code, ok := w.ExitCode()
	if !ok {
		t.Error("expected exit code to be available")
	}
	if code == nil || *code != 0 {
		t.Errorf("expected exit code 0, got %v", code)
	}
}

func TestProcessWrapper_Start_NonExistentCommand(t *testing.T) {
	w := NewProcessWrapper()
	ctx := context.Background()

	opts := WrapperOptions{
		Command: "nonexistent_command_xyz",
		Args:    []string{},
	}

	_, err := w.Start(ctx, opts)
	if err == nil {
		t.Error("expected error for nonexistent command, got nil")
	}

	state := w.State()
	if state != StateError {
		t.Errorf("expected state error, got %s", state)
	}
}

func TestProcessWrapper_Output(t *testing.T) {
	skipWindows(t)

	w := NewProcessWrapper()
	ctx := context.Background()

	opts := WrapperOptions{
		Command: "sh",
		Args:    []string{"-c", "echo stdout_line; echo stderr_line >&2"},
	}

	_, err := w.Start(ctx, opts)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	var stdout, stderr bytes.Buffer
	stdoutCh := w.OutputChannel()
	stderrCh := w.ErrorChannel()

	doneOut := make(chan struct{})
	go func() {
		for data := range stdoutCh {
			stdout.Write(data)
		}
		close(doneOut)
	}()

	doneErr := make(chan struct{})
	go func() {
		for data := range stderrCh {
			stderr.Write(data)
		}
		close(doneErr)
	}()

	w.Wait()
	// Both output channels are closed once the process exits and the
	// readers drain them; wait for the writers to finish before reading
	// the buffers to avoid a data race on the bytes.Buffer.
	<-doneOut
	<-doneErr

	stdoutStr := strings.TrimSpace(stdout.String())
	stderrStr := strings.TrimSpace(stderr.String())

	if !strings.Contains(stdoutStr, "stdout_line") {
		t.Errorf("expected stdout to contain 'stdout_line', got '%s'", stdoutStr)
	}
	if !strings.Contains(stderrStr, "stderr_line") {
		t.Errorf("expected stderr to contain 'stderr_line', got '%s'", stderrStr)
	}
}

func TestProcessWrapper_Kill(t *testing.T) {
	skipWindows(t)

	w := NewProcessWrapper()
	ctx := context.Background()

	// Start a long-running process
	opts := WrapperOptions{
		Command: "sleep",
		Args:    []string{"60"},
	}

	_, err := w.Start(ctx, opts)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Give it time to start
	time.Sleep(10 * time.Millisecond)

	if !w.IsRunning() {
		t.Error("expected process to be running")
	}

	// Kill it
	if err := w.Kill(); err != nil {
		t.Errorf("Kill failed: %v", err)
	}

	w.Wait()

	state := w.State()
	if state != StateKilled {
		t.Errorf("expected state killed, got %s", state)
	}
}

func TestProcessWrapper_WriteInput(t *testing.T) {
	skipWindows(t)

	w := NewProcessWrapper()
	ctx := context.Background()

	// Use cat which echoes stdin
	opts := WrapperOptions{
		Command: "cat",
	}

	_, err := w.Start(ctx, opts)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Write input
	n, err := w.WriteInputString("hello")
	if err != nil {
		t.Errorf("WriteInputString failed: %v", err)
	}
	if n <= 0 {
		t.Error("expected bytes written")
	}

	// Give cat time to output
	time.Sleep(50 * time.Millisecond)

	// Read output
	select {
	case data := <-w.OutputChannel():
		if !strings.Contains(string(data), "hello") {
			t.Errorf("expected output to contain 'hello', got '%s'", string(data))
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("timeout waiting for output")
	}

	// Kill cat
	w.Kill()
}

func TestProcessWrapper_Done(t *testing.T) {
	skipWindows(t)

	w := NewProcessWrapper()
	ctx := context.Background()

	opts := WrapperOptions{
		Command: "sh",
		Args:    []string{"-c", "exit 42"},
	}

	_, err := w.Start(ctx, opts)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Wait using done channel
	select {
	case <-w.Done():
		// Process exited
	case <-time.After(5 * time.Second):
		t.Error("timeout waiting for done")
	}

	code, ok := w.ExitCode()
	if !ok {
		t.Error("expected exit code")
	}
	if code == nil || *code != 42 {
		t.Errorf("expected exit code 42, got %v", code)
	}
}

func TestProcessWrapper_Signal(t *testing.T) {
	skipWindows(t)

	w := NewProcessWrapper()
	ctx := context.Background()

	// Start sleep
	opts := WrapperOptions{
		Command: "sleep",
		Args:    []string{"60"},
	}

	_, err := w.Start(ctx, opts)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Send SIGTERM
	if err := w.Signal(syscall.SIGTERM); err != nil {
		t.Errorf("Signal failed: %v", err)
	}

	w.Wait()

	// Should have exited
	if w.IsRunning() {
		t.Error("expected process to have stopped")
	}
}

func TestProcessWrapper_Info(t *testing.T) {
	skipWindows(t)

	w := NewProcessWrapper()
	ctx := context.Background()

	opts := WrapperOptions{
		Command: "sleep",
		Args:    []string{"1"},
		Dir:     "/tmp",
		Env:     []string{"TEST=value"},
	}

	info, err := w.Start(ctx, opts)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if info.Command != "sleep" {
		t.Errorf("expected command 'sleep', got '%s'", info.Command)
	}
	if info.Dir != "/tmp" {
		t.Errorf("expected dir '/tmp', got '%s'", info.Dir)
	}
	if info.State != StateRunning {
		t.Errorf("expected state running, got %s", info.State)
	}
	if info.StartedAt.IsZero() {
		t.Error("expected started_at to be set")
	}

	w.Wait()

	// Check updated info
	info = w.Info()
	if info.State != StateExited {
		t.Errorf("expected state exited, got %s", info.State)
	}
	if info.ExitedAt == nil {
		t.Error("expected exited_at to be set")
	}
}

func TestProcessWrapper_RedactSecrets(t *testing.T) {
	w := NewProcessWrapper()

	args := []string{"--token", "secret123", "--verbose"}
	secrets := []string{"secret123"}

	redacted := w.RedactSecrets(secrets, args)

	for i, arg := range redacted {
		if args[i] == "secret123" && arg != "[REDACTED]" {
			t.Errorf("expected secret to be redacted, got '%s'", arg)
		}
	}
}

func TestProcessWrapper_DoubleStart(t *testing.T) {
	skipWindows(t)

	w := NewProcessWrapper()
	ctx := context.Background()

	opts := WrapperOptions{
		Command: "true",
	}

	// First start
	_, err := w.Start(ctx, opts)
	if err != nil {
		t.Fatalf("first Start failed: %v", err)
	}

	// Second start should fail
	_, err = w.Start(ctx, opts)
	if err == nil {
		t.Error("expected error for double start, got nil")
	}
}

func TestProcessWrapper_ExitError(t *testing.T) {
	skipWindows(t)

	w := NewProcessWrapper()
	ctx := context.Background()

	opts := WrapperOptions{
		Command: "sh",
		Args:    []string{"-c", "exit 1"},
	}

	_, err := w.Start(ctx, opts)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	w.Wait()

	state := w.State()
	if state != StateError {
		t.Errorf("expected state error, got %s", state)
	}

	errStr, ok := w.ExitError()
	if !ok {
		t.Error("expected exit error")
	}
	if errStr == nil {
		t.Error("expected non-nil error string")
	}
}

func TestProcessWrapper_ContextCancellation(t *testing.T) {
	skipWindows(t)

	w := NewProcessWrapper()
	ctx, cancel := context.WithCancel(context.Background())

	opts := WrapperOptions{
		Command: "sleep",
		Args:    []string{"60"},
	}

	_, err := w.Start(ctx, opts)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Cancel context
	cancel()

	// Wait for process to be killed
	w.Wait()

	// Process should have exited
	if w.IsRunning() {
		t.Error("expected process to have stopped after context cancellation")
	}
}

func TestNewProcessWrapper(t *testing.T) {
	w := NewProcessWrapper()

	if w == nil {
		t.Fatal("expected non-nil wrapper")
	}
	if w.State() != StatePending {
		t.Errorf("expected state pending, got %s", w.State())
	}
	if w.IsRunning() {
		t.Error("expected not running initially")
	}
}

func TestFindCommand(t *testing.T) {
	skipWindows(t)

	// Test that we can find common commands
	cmds := []string{"echo", "sh", "true", "false"}
	for _, cmd := range cmds {
		path, err := exec.LookPath(cmd)
		if err != nil {
			t.Errorf("could not find command '%s': %v", cmd, err)
		}
		if path == "" {
			t.Errorf("LookPath returned empty for '%s'", cmd)
		}
	}
}
