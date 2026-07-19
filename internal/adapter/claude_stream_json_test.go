package adapter

import (
	"strings"
	"testing"
)

// TestParseStreamJSONLine covers A-3 (ADR-009): the parser maps the documented
// Claude Code stream-json events, and non-stream-json lines fall back to the
// existing behavior.
func TestParseStreamJSONLine(t *testing.T) {
	p := NewOutputParser()

	// assistant text -> SignalOutput with joined text
	ev := p.ParseLine(`{"type":"assistant","message":{"content":[{"type":"text","text":"hello "},{"type":"text","text":"world"}]}}`)
	if ev == nil || ev.Type != SignalOutput {
		t.Fatalf("assistant text: expected SignalOutput, got %+v", ev)
	}
	op, ok := ev.Payload.(OutputPayload)
	if !ok || op.Content != "hello world" {
		t.Fatalf("assistant text: expected joined 'hello world', got %+v", ev.Payload)
	}

	// assistant with only tool_use -> SignalDiagnostic mentioning the tool name
	ev = p.ParseLine(`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash","input":{}}]}}`)
	if ev == nil || ev.Type != SignalDiagnostic {
		t.Fatalf("assistant tool_use: expected SignalDiagnostic, got %+v", ev)
	}
	dp, ok := ev.Payload.(DiagnosticPayload)
	if !ok || !strings.Contains(dp.Message, "Bash") {
		t.Fatalf("assistant tool_use: expected message mentioning 'Bash', got %+v", ev.Payload)
	}

	// result -> SignalOutput with the result text
	ev = p.ParseLine(`{"type":"result","subtype":"success","result":"final text","is_error":false}`)
	if ev == nil || ev.Type != SignalOutput {
		t.Fatalf("result: expected SignalOutput, got %+v", ev)
	}
	if op, _ := ev.Payload.(OutputPayload); op.Content != "final text" {
		t.Fatalf("result: expected 'final text', got %q", op.Content)
	}

	// system -> recognized but suppressed (ParseLine returns nil)
	if ev := p.ParseLine(`{"type":"system","subtype":"init","session_id":"abc"}`); ev != nil {
		t.Fatalf("system: expected nil event, got %+v", ev)
	}

	// stream_event text_delta -> SignalOutput with the delta text
	ev = p.ParseLine(`{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"delta chunk"}}}`)
	if ev == nil || ev.Type != SignalOutput {
		t.Fatalf("stream_event: expected SignalOutput, got %+v", ev)
	}
	if op, _ := ev.Payload.(OutputPayload); op.Content != "delta chunk" {
		t.Fatalf("stream_event: expected 'delta chunk', got %q", op.Content)
	}

	// non-JSON line still returns SignalOutput via fallback
	ev = p.ParseLine("plain terminal output")
	if ev == nil || ev.Type != SignalOutput {
		t.Fatalf("plain line: expected SignalOutput, got %+v", ev)
	}
	if op, _ := ev.Payload.(OutputPayload); op.Content != "plain terminal output" {
		t.Fatalf("plain line: expected 'plain terminal output', got %q", op.Content)
	}
}
