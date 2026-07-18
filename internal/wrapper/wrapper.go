// Package wrapper provides process wrapper functionality for agent execution.
package wrapper

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ProcessState represents the current state of a wrapped process.
type ProcessState string

const (
	StatePending ProcessState = "pending"
	StateRunning ProcessState = "running"
	StateExited  ProcessState = "exited"
	StateKilled  ProcessState = "killed"
	StateError   ProcessState = "error"
)

// ProcessInfo contains information about a running process.
type ProcessInfo struct {
	PID       int          `json:"pid"`
	PGroup    int          `json:"pgroup"`
	Command   string       `json:"command"`
	Args      []string     `json:"args"`
	Dir       string       `json:"dir"`
	StartedAt time.Time    `json:"started_at"`
	ExitedAt  *time.Time   `json:"exited_at,omitempty"`
	ExitCode  *int         `json:"exit_code,omitempty"`
	ExitError *string      `json:"exit_error,omitempty"`
	State     ProcessState `json:"state"`
}

// ProcessWrapper wraps a child process for agent execution.
type ProcessWrapper struct {
	mu        sync.RWMutex
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    io.ReadCloser
	stderr    io.ReadCloser
	startedAt time.Time
	exitedAt  *time.Time
	exitCode  *int
	exitError *string
	state     ProcessState
	done      chan struct{}
	doneOnce  sync.Once
	outputCh  chan []byte
	errCh     chan []byte
	readersWG sync.WaitGroup
}

// WrapperOptions contains options for process execution.
type WrapperOptions struct {
	// Command is the executable to run
	Command string `json:"command"`

	// Args are command-line arguments (not including command)
	Args []string `json:"args"`

	// Dir is the working directory
	Dir string `json:"dir"`

	// Env is environment variables (key=value format)
	Env []string `json:"env"`

	// Secrets are environment variables that should not be logged
	Secrets []string `json:"-"`

	// UsePTY indicates whether to use a pseudo-terminal
	UsePTY bool `json:"use_pty"`

	// StdoutBufSize is the buffer size for stdout
	StdoutBufSize int `json:"stdout_buf_size"`

	// StderrBufSize is the buffer size for stderr
	StderrBufSize int `json:"stderr_buf_size"`
}

// NewProcessWrapper creates a new process wrapper.
func NewProcessWrapper() *ProcessWrapper {
	return &ProcessWrapper{
		state:    StatePending,
		done:     make(chan struct{}),
		outputCh: make(chan []byte, 100),
		errCh:    make(chan []byte, 100),
	}
}

// Start starts the wrapped process.
func (w *ProcessWrapper) Start(ctx context.Context, opts WrapperOptions) (*ProcessInfo, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.state == StateRunning {
		return nil, fmt.Errorf("process already running")
	}

	// Create command
	cmd := exec.CommandContext(ctx, opts.Command, opts.Args...)

	// Set working directory
	if opts.Dir != "" {
		cmd.Dir = opts.Dir
	}

	// Set environment
	if len(opts.Env) > 0 {
		cmd.Env = opts.Env
	} else {
		cmd.Env = os.Environ()
	}

	// Set process group (for killing entire family on Unix)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	// Set up pipes for stdout/stderr
	var err error
	w.stdout, err = cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("creating stdout pipe: %w", err)
	}

	w.stderr, err = cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("creating stderr pipe: %w", err)
	}

	w.stdin, err = cmd.StdinPipe()
	if err != nil {
		// Stdin pipe is optional - some commands don't need it
		w.stdin = nil
	}

	w.cmd = cmd

	// Start the process
	if err := cmd.Start(); err != nil {
		w.state = StateError
		errStr := err.Error()
		w.exitError = &errStr
		return nil, fmt.Errorf("starting process: %w", err)
	}

	// Record start time
	w.startedAt = time.Now()
	w.state = StateRunning

	// Get process group
	pgroup := cmd.Process.Pid
	if cmd.SysProcAttr != nil && cmd.SysProcAttr.Setpgid {
		// On success, Pid should be the process group leader
		pgroup = cmd.Process.Pid
	}

	// Start goroutines to read output
	w.readersWG.Add(2)
	go w.readOutput(w.stdout, w.outputCh)
	go w.readOutput(w.stderr, w.errCh)

	// Start goroutine to wait for exit
	go w.waitForExit()

	info := &ProcessInfo{
		PID:       cmd.Process.Pid,
		PGroup:    pgroup,
		Command:   opts.Command,
		Args:      opts.Args,
		Dir:       opts.Dir,
		StartedAt: w.startedAt,
		State:     StateRunning,
	}

	return info, nil
}

// readOutput reads from a pipe and sends to channel.
func (w *ProcessWrapper) readOutput(r io.ReadCloser, ch chan<- []byte) {
	defer r.Close()
	defer w.readersWG.Done()

	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])
			select {
			case ch <- data:
			default:
				// Channel full, drop data
			}
		}
		if err != nil {
			if err != io.EOF {
				// Log error but don't fail
			}
			return
		}
	}
}

// waitForExit waits for the process to exit and records exit info.
func (w *ProcessWrapper) waitForExit() {
	if w.cmd == nil {
		return
	}

	err := w.cmd.Wait()

	// Wait for the output readers to drain the pipes (the OS pipe is closed
	// when the child exits, so Read returns EOF and the readers return).
	w.readersWG.Wait()

	// Now that no more output will arrive, close the output channels so
	// consumers ranging over them can terminate.
	close(w.outputCh)
	close(w.errCh)

	w.mu.Lock()
	defer w.mu.Unlock()

	now := time.Now()
	w.exitedAt = &now
	w.doneOnce.Do(func() { close(w.done) })

	// Preserve the killed state if Kill() was invoked.
	if w.state == StateKilled {
		if err != nil {
			errStr := err.Error()
			w.exitError = &errStr
		}
		return
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code := exitErr.ExitCode()
			w.exitCode = &code
			errStr := exitErr.Error()
			w.exitError = &errStr
		} else {
			errStr := err.Error()
			w.exitError = &errStr
		}
		w.state = StateError
	} else {
		code := 0
		w.exitCode = &code
		w.state = StateExited
	}
}

// WriteInput writes input to the process stdin.
func (w *ProcessWrapper) WriteInput(data []byte) (int, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.stdin == nil {
		return 0, fmt.Errorf("stdin not available")
	}

	if w.state != StateRunning {
		return 0, fmt.Errorf("process not running")
	}

	return w.stdin.Write(data)
}

// WriteInputString writes a string to stdin with newline.
func (w *ProcessWrapper) WriteInputString(s string) (int, error) {
	return w.WriteInput([]byte(s + "\n"))
}

// OutputChannel returns the channel for stdout output.
func (w *ProcessWrapper) OutputChannel() <-chan []byte {
	return w.outputCh
}

// ErrorChannel returns the channel for stderr output.
func (w *ProcessWrapper) ErrorChannel() <-chan []byte {
	return w.errCh
}

// Kill terminates the process group.
func (w *ProcessWrapper) Kill() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.cmd == nil || w.cmd.Process == nil {
		return nil
	}

	// Kill the entire process group
	if err := syscall.Kill(-w.cmd.Process.Pid, syscall.SIGKILL); err != nil {
		// If process group kill fails, try individual process
		if err := w.cmd.Process.Kill(); err != nil {
			return fmt.Errorf("killing process: %w", err)
		}
	}

	w.state = StateKilled
	return nil
}

// Signal sends a signal to the process.
func (w *ProcessWrapper) Signal(sig syscall.Signal) error {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.cmd == nil || w.cmd.Process == nil {
		return fmt.Errorf("process not running")
	}

	return w.cmd.Process.Signal(sig)
}

// Wait waits for the process to exit.
func (w *ProcessWrapper) Wait() error {
	w.mu.RLock()
	done := w.done
	w.mu.RUnlock()

	<-done
	return nil
}

// Done returns a channel that closes when the process exits.
func (w *ProcessWrapper) Done() <-chan struct{} {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.done
}

// Info returns the current process info.
func (w *ProcessWrapper) Info() *ProcessInfo {
	w.mu.RLock()
	defer w.mu.RUnlock()

	info := &ProcessInfo{
		Command:   w.cmd.Path,
		Args:      w.cmd.Args[1:],
		Dir:       w.cmd.Dir,
		State:     w.state,
		StartedAt: w.startedAt,
		ExitedAt:  w.exitedAt,
		ExitCode:  w.exitCode,
		ExitError: w.exitError,
	}

	if w.cmd != nil && w.cmd.Process != nil {
		info.PID = w.cmd.Process.Pid
		info.PGroup = w.cmd.Process.Pid
	}

	return info
}

// State returns the current process state.
func (w *ProcessWrapper) State() ProcessState {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.state
}

// IsRunning returns true if the process is still running.
func (w *ProcessWrapper) IsRunning() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.state == StateRunning
}

// ExitCode returns the exit code if exited.
func (w *ProcessWrapper) ExitCode() (*int, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.exitCode, w.state == StateExited || w.state == StateError
}

// ExitError returns the exit error if any.
func (w *ProcessWrapper) ExitError() (*string, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.exitError, w.state == StateError
}

// RedactSecrets removes secrets from command args for logging.
func (w *ProcessWrapper) RedactSecrets(secrets []string, args []string) []string {
	result := make([]string, len(args))
	copy(result, args)

	for i, arg := range result {
		for _, secret := range secrets {
			if secret != "" && strings.Contains(arg, secret) {
				result[i] = "[REDACTED]"
			}
		}
	}
	return result
}
