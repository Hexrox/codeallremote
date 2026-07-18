package adapter

import (
	"strings"
	"testing"
)

func TestOutputParser_ParseLine_Approval(t *testing.T) {
	p := NewOutputParser()

	tests := []struct {
		line        string
		wantType    AdapterSignalType
		wantContent string
	}{
		{
			line:        "Approval required: execute command 'rm -rf /tmp/test'",
			wantType:    SignalApprovalRequest,
			wantContent: "execute command 'rm -rf /tmp/test'",
		},
		{
			line:     "APPROVAL NEEDED: Write to config.json",
			wantType: SignalApprovalRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			event := p.ParseLine(tt.line)
			if event == nil {
				t.Fatal("expected event, got nil")
			}
			if event.Type != tt.wantType {
				t.Errorf("expected type %s, got %s", tt.wantType, event.Type)
			}
		})
	}
}

func TestOutputParser_ParseLine_PlainOutput(t *testing.T) {
	p := NewOutputParser()

	line := "Hello, this is normal output"
	event := p.ParseLine(line)

	if event == nil {
		t.Fatal("expected event, got nil")
	}
	if event.Type != SignalOutput {
		t.Errorf("expected SignalOutput, got %s", event.Type)
	}

	payload, ok := event.Payload.(OutputPayload)
	if !ok {
		t.Fatal("expected OutputPayload")
	}
	if payload.Content != line {
		t.Errorf("expected content '%s', got '%s'", line, payload.Content)
	}
}

func TestOutputParser_ParseLine_Error(t *testing.T) {
	p := NewOutputParser()

	line := "Error: file not found"
	event := p.ParseLine(line)

	if event == nil {
		t.Fatal("expected event, got nil")
	}
	if event.Type != SignalError {
		t.Errorf("expected SignalError, got %s", event.Type)
	}

	payload, ok := event.Payload.(ErrorPayload)
	if !ok {
		t.Fatal("expected ErrorPayload")
	}
	if !strings.Contains(payload.Message, "file not found") {
		t.Errorf("expected message to contain 'file not found', got '%s'", payload.Message)
	}
}

func TestOutputParser_ParseLine_Empty(t *testing.T) {
	p := NewOutputParser()

	tests := []string{"", "   ", "\t", "\n"}
	for _, line := range tests {
		event := p.ParseLine(line)
		if event != nil {
			t.Errorf("expected nil for empty line '%s', got %+v", line, event)
		}
	}
}

func TestOutputParser_ParseStream(t *testing.T) {
	p := NewOutputParser()

	data := []byte(`Line 1
Approval required: do something
Line 2
Error: something went wrong
Line 3`)

	events := p.ParseStream(data)

	if len(events) < 3 {
		t.Fatalf("expected at least 3 events, got %d", len(events))
	}

	// Verify we got different types
	hasApproval := false
	hasError := false
	hasOutput := false

	for _, e := range events {
		switch e.Type {
		case SignalApprovalRequest:
			hasApproval = true
		case SignalError:
			hasError = true
		case SignalOutput:
			hasOutput = true
		}
	}

	if !hasApproval {
		t.Error("expected to find approval event")
	}
	if !hasError {
		t.Error("expected to find error event")
	}
	if !hasOutput {
		t.Error("expected to find output events")
	}
}

func TestOutputParser_ParseBuffer_PartialLine(t *testing.T) {
	p := NewOutputParser()

	// Buffer with partial line at end
	buffer := []byte("Complete line 1\nComplete line 2\nPartial")

	events, remaining := p.ParseBuffer(buffer, false)

	if len(events) != 2 {
		t.Errorf("expected 2 events, got %d", len(events))
	}

	if string(remaining) != "Partial" {
		t.Errorf("expected remaining 'Partial', got '%s'", string(remaining))
	}

	// Now parse with complete buffer
	events, remaining = p.ParseBuffer(remaining, true)
	if len(events) != 1 {
		t.Errorf("expected 1 event from complete partial, got %d", len(events))
	}
	if len(remaining) != 0 {
		t.Errorf("expected no remaining, got '%s'", string(remaining))
	}
}

func TestOutputParser_RecognizeApproval(t *testing.T) {
	p := NewOutputParser()

	tests := []struct {
		text     string
		expected bool
	}{
		{"Do you want to execute this command?", true},
		{"Would you like to write to this file?", true},
		{"Should I run npm install?", true},
		{"Permission to access /etc/passwd?", true},
		{"This is just normal text", false},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			result := p.RecognizeApproval(tt.text)
			if tt.expected && result == nil {
				t.Error("expected approval payload, got nil")
			}
			if !tt.expected && result != nil {
				t.Errorf("expected nil, got %+v", result)
			}
		})
	}
}

func TestOutputParser_ExtractChangedFiles(t *testing.T) {
	p := NewOutputParser()

	output := `
Modified: src/main.go
Created: src/new_file.txt
Deleted: old_file.md
Changed: config.yaml
Added: test.txt
Removed: deprecated.js
`

	files := p.ExtractChangedFiles(output)

	if len(files) != 6 {
		t.Fatalf("expected 6 files, got %d", len(files))
	}

	ops := make(map[string]string)
	for _, f := range files {
		ops[f.Path] = f.Operation
	}

	if ops["src/main.go"] != "modify" {
		t.Errorf("expected src/main.go to be modified")
	}
	if ops["src/new_file.txt"] != "create" {
		t.Errorf("expected src/new_file.txt to be created")
	}
	if ops["old_file.md"] != "delete" {
		t.Errorf("expected old_file.md to be deleted")
	}
}

func TestOutputParser_RecognizeStatusChange(t *testing.T) {
	p := NewOutputParser()

	tests := []struct {
		text     string
		newState string
	}{
		{"Starting agent...", "starting"},
		{"Ready to accept commands", "active"},
		{"Running process", "active"},
		{"Task completed successfully", "completed"},
		{"Finished all operations", "completed"},
		{"Failed to execute command", "failed"},
		{"Error: critical failure", "failed"},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			result := p.RecognizeStatusChange(tt.text)
			if result == nil {
				t.Fatal("expected status change, got nil")
			}
			if result.NewState != tt.newState {
				t.Errorf("expected state %s, got %s", tt.newState, result.NewState)
			}
		})
	}
}

func TestOutputParser_RedactSecrets(t *testing.T) {
	p := NewOutputParser()

	output := "Using token abc123secret and key secret_key_456 for auth"
	secrets := []string{"abc123secret", "secret_key_456"}

	result := p.RedactSecrets(output, secrets)

	if strings.Contains(result, "abc123secret") {
		t.Error("expected secret to be redacted")
	}
	if strings.Contains(result, "secret_key_456") {
		t.Error("expected secret_key to be redacted")
	}
	if !strings.Contains(result, "[REDACTED]") {
		t.Error("expected [REDACTED] in output")
	}
}

func TestOutputParser_NormalizePayload(t *testing.T) {
	p := NewOutputParser()

	// Output payload
	payload, err := p.NormalizePayload("test content", SignalOutput)
	if err != nil {
		t.Fatalf("NormalizePayload failed: %v", err)
	}
	op, ok := payload.(OutputPayload)
	if !ok {
		t.Fatal("expected OutputPayload")
	}
	if op.Content != "test content" {
		t.Errorf("expected content 'test content', got '%s'", op.Content)
	}

	// Error payload
	payload, err = p.NormalizePayload("error message", SignalError)
	if err != nil {
		t.Fatalf("NormalizePayload failed: %v", err)
	}
	ep, ok := payload.(ErrorPayload)
	if !ok {
		t.Fatal("expected ErrorPayload")
	}
	if ep.Message != "error message" {
		t.Errorf("expected message 'error message', got '%s'", ep.Message)
	}
}

func TestOutputParser_IsSignalLine(t *testing.T) {
	p := NewOutputParser()

	tests := []struct {
		line     string
		expected bool
	}{
		{"Approval required: test", true},
		{"Error: something wrong", true},
		{"Normal output line", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			result := p.IsSignalLine(tt.line)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestOutputParser_CompatibilityDegraded(t *testing.T) {
	p := NewOutputParser()

	if p.IsCompatibilityDegraded() {
		t.Error("expected not degraded initially")
	}

	p.SetCompatibilityDegraded(true)
	if !p.IsCompatibilityDegraded() {
		t.Error("expected degraded after setting")
	}

	p.SetCompatibilityDegraded(false)
	if p.IsCompatibilityDegraded() {
		t.Error("expected not degraded after clearing")
	}
}

func TestOutputParser_ShouldEmitRaw(t *testing.T) {
	p := NewOutputParser()

	if !p.ShouldEmitRaw() {
		t.Error("expected to emit raw when not degraded")
	}

	p.SetCompatibilityDegraded(true)
	if p.ShouldEmitRaw() {
		t.Error("expected not to emit raw when degraded")
	}
}

func TestOutputParser_JSONEvent(t *testing.T) {
	p := NewOutputParser()

	jsonLine := `{"type": "approval", "payload": {"action": "write", "path": "/tmp/test"}}`
	event := p.ParseLine(jsonLine)

	if event == nil {
		t.Fatal("expected event, got nil")
	}
	if event.Type != SignalApprovalRequest {
		t.Errorf("expected SignalApprovalRequest, got %s", event.Type)
	}
}

func TestOutputParser_ToolExecution(t *testing.T) {
	p := NewOutputParser()

	line := "Executing: npm install"
	event := p.ParseLine(line)

	if event == nil {
		t.Fatal("expected event, got nil")
	}
	if event.Type != SignalOutput {
		t.Errorf("expected SignalOutput, got %s", event.Type)
	}

	payload, ok := event.Payload.(OutputPayload)
	if !ok {
		t.Fatal("expected OutputPayload")
	}
	if !strings.Contains(payload.Content, "npm install") {
		t.Errorf("expected content to contain 'npm install', got '%s'", payload.Content)
	}
}
