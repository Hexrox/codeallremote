package mcpperm

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestServer_Handshake_ListAndDecide(t *testing.T) {
	var lastToolName, lastToolUseID string
	decide := func(toolName string, input json.RawMessage, toolUseID string) Decision {
		lastToolName = toolName
		lastToolUseID = toolUseID
		if toolName == "Read" {
			return Decision{Allow: true}
		}
		return Decision{Allow: false, Message: "nope"}
	}

	s := NewServer("car", "approve", decide)

	input := `{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2025-11-25"}}
{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}
{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"approve","arguments":{"tool_name":"Bash","input":{"command":"rm x"},"tool_use_id":"tu1"}}}`

	var buf bytes.Buffer
	if err := s.Serve(context.Background(), strings.NewReader(input), &buf); err != nil {
		t.Fatalf("Serve error: %v", err)
	}

	var responses []map[string]any
	for _, line := range strings.Split(buf.String(), "\n") {
		if line == "" {
			continue
		}
		var resp map[string]any
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}
		responses = append(responses, resp)
	}

	// No response for the notification -> exactly 3.
	if len(responses) != 3 {
		t.Fatalf("expected 3 responses, got %d", len(responses))
	}

	initResult := responses[0]["result"].(map[string]any)
	if initResult["serverInfo"].(map[string]any)["name"] != "car" {
		t.Fatal("server name mismatch")
	}
	if initResult["capabilities"].(map[string]any)["tools"] == nil {
		t.Fatal("capabilities.tools missing")
	}

	tools := responses[1]["result"].(map[string]any)["tools"].([]any)
	if len(tools) == 0 || tools[0].(map[string]any)["name"] != "approve" {
		t.Fatalf("expected tool 'approve', got %v", tools)
	}

	content := responses[2]["result"].(map[string]any)["content"].([]any)
	text := content[0].(map[string]any)["text"].(string)
	var decMap map[string]any
	if err := json.Unmarshal([]byte(text), &decMap); err != nil {
		t.Fatalf("failed to unmarshal decision text: %v", err)
	}
	if decMap["behavior"] != "deny" || decMap["message"] != "nope" {
		t.Fatalf("expected deny/nope, got %v", decMap)
	}
	if lastToolName != "Bash" || lastToolUseID != "tu1" {
		t.Fatalf("decide got wrong args: %q %q", lastToolName, lastToolUseID)
	}

	// Read tool -> allow.
	var buf2 bytes.Buffer
	in2 := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"approve","arguments":{"tool_name":"Read","input":{"file":"test.txt"},"tool_use_id":"tu2"}}}`
	if err := s.Serve(context.Background(), strings.NewReader(in2), &buf2); err != nil {
		t.Fatalf("Serve error (sub-check): %v", err)
	}
	var resp2 map[string]any
	_ = json.Unmarshal([]byte(strings.TrimSpace(buf2.String())), &resp2)
	text2 := resp2["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
	var decMap2 map[string]any
	_ = json.Unmarshal([]byte(text2), &decMap2)
	if decMap2["behavior"] != "allow" {
		t.Fatalf("expected allow for Read, got %v", decMap2)
	}
}
