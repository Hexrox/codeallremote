// Package mcpperm implements a minimal MCP (Model Context Protocol)
// permission-prompt server that Claude Code calls (via
// --permission-prompt-tool + --mcp-config) to approve or deny each tool use.
//
// The wire protocol (newline-delimited JSON-RPC 2.0) was captured live from
// claude 2.1.214 and is documented in adr/ADR-010-mcp-permission-approvals.md.
// This package is transport-agnostic (it reads/writes an io.Reader/io.Writer)
// and dependency-free, so it is unit-testable without a real claude.
package mcpperm

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
)

// Decision is the outcome of a permission request. When Allow is true,
// UpdatedInput (if non-nil) replaces the tool input claude will run; when nil,
// the original input is echoed. When Allow is false, Message is the deny reason.
type Decision struct {
	Allow        bool
	Message      string
	UpdatedInput any
}

// DecideFunc decides a single tool-use permission request.
type DecideFunc func(toolName string, input json.RawMessage, toolUseID string) Decision

// Server answers Claude Code's permission-prompt tool over MCP.
type Server struct {
	serverName string
	toolName   string
	decide     DecideFunc
}

// NewServer creates a permission server exposing a single tool named toolName
// (claude is launched with --permission-prompt-tool mcp__<serverName>__<toolName>).
func NewServer(serverName, toolName string, decide DecideFunc) *Server {
	return &Server{serverName: serverName, toolName: toolName, decide: decide}
}

type request struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id"`
	Method  string           `json:"method"`
	Params  json.RawMessage  `json:"params"`
}

// Serve reads JSON-RPC messages from r and writes responses to w until r is
// exhausted (EOF) or ctx is cancelled. Notifications (no id) get no response.
func (s *Server) Serve(ctx context.Context, r io.Reader, w io.Writer) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Text()
		if line == "" {
			continue
		}

		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			continue
		}

		var resp map[string]any
		switch req.Method {
		case "initialize":
			var params struct {
				ProtocolVersion string `json:"protocolVersion"`
			}
			_ = json.Unmarshal(req.Params, &params)
			protoVer := params.ProtocolVersion
			if protoVer == "" {
				protoVer = "2025-11-25"
			}
			resp = map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result": map[string]any{
					"protocolVersion": protoVer,
					"capabilities":    map[string]any{"tools": struct{}{}},
					"serverInfo":      map[string]any{"name": s.serverName, "version": "0.1.0"},
				},
			}
		case "notifications/initialized":
			continue
		case "tools/list":
			resp = map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result": map[string]any{
					"tools": []map[string]any{
						{
							"name":        s.toolName,
							"description": "Approve or deny a tool use",
							"inputSchema": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"tool_name":   map[string]any{"type": "string"},
									"input":       map[string]any{"type": "object"},
									"tool_use_id": map[string]any{"type": "string"},
								},
							},
						},
					},
				},
			}
		case "tools/call":
			var callParams struct {
				Name      string `json:"name"`
				Arguments struct {
					ToolName  string          `json:"tool_name"`
					Input     json.RawMessage `json:"input"`
					ToolUseID string          `json:"tool_use_id"`
				} `json:"arguments"`
			}
			_ = json.Unmarshal(req.Params, &callParams)
			dec := s.decide(callParams.Arguments.ToolName, callParams.Arguments.Input, callParams.Arguments.ToolUseID)

			type decision struct {
				Behavior     string `json:"behavior"`
				UpdatedInput any    `json:"updatedInput,omitempty"`
				Message      string `json:"message,omitempty"`
			}
			var d decision
			if dec.Allow {
				ui := dec.UpdatedInput
				if ui == nil {
					// Echo the original input so claude runs the tool unchanged.
					ui = callParams.Arguments.Input
				}
				d = decision{Behavior: "allow", UpdatedInput: ui}
			} else {
				d = decision{Behavior: "deny", Message: dec.Message}
			}
			decJSON, _ := json.Marshal(d)
			resp = map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result": map[string]any{
					"content": []map[string]any{
						{"type": "text", "text": string(decJSON)},
					},
				},
			}
		default:
			resp = map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{}}
		}

		if resp != nil {
			out, _ := json.Marshal(resp)
			_, _ = w.Write(out)
			_, _ = w.Write([]byte("\n"))
		}
	}
	return scanner.Err()
}
