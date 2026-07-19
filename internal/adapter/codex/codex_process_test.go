package codex

import (
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/code-all-remote/car/sdk"
)

// TestCodexAdapter_Start_StreamsOutputAndCompletes drives the real
// process-spawning path with a deterministic `sh` rig (no codex binary).
func TestCodexAdapter_Start_StreamsOutputAndCompletes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix only")
	}
	a := New("")
	a.SetExecPath("sh")
	handle, err := a.Start(sdk.StartConfig{
		SessionID:     "s1",
		WorkspacePath: "/tmp",
		Args:          []string{"-c", "echo hello-codex"},
	})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	ch := a.Observe(handle)
	timeout := time.After(2 * time.Second)
	sawOutput := false
	sawCompletion := false
	for {
		if sawOutput && sawCompletion {
			return
		}
		select {
		case <-timeout:
			t.Fatalf("timeout: sawOutput=%v sawCompletion=%v", sawOutput, sawCompletion)
		case sig, ok := <-ch:
			if !ok {
				if !sawCompletion {
					t.Fatalf("channel closed before completion")
				}
				return
			}
			m, _ := sig.Payload.(map[string]any)
			switch sig.Type {
			case sdk.SignalOutput:
				s, _ := m["content"].(string)
				if strings.Contains(s, "hello-codex") {
					sawOutput = true
				}
			case sdk.SignalCompletion:
				sawCompletion = true
			}
		}
	}
}
