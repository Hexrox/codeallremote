// Command car-mcp-perm is a stdio MCP permission-prompt server for Claude Code
// (ADR-010). Claude is launched with --permission-prompt-tool
// mcp__car__approve --mcp-config <cfg> where the config runs this binary.
//
// This is the Increment-1 skeleton: it is fail-closed (denies every tool use)
// until Increment 2 wires the decision to the running CAR server's
// ApprovalBridge over a local socket so a real phone approval drives it.
package main

import (
	"context"
	"encoding/json"
	"os"

	"github.com/code-all-remote/car/internal/mcpperm"
)

func main() {
	deny := func(toolName string, input json.RawMessage, toolUseID string) mcpperm.Decision {
		return mcpperm.Decision{
			Allow:   false,
			Message: "CAR: approvals not yet wired to the operator device (ADR-010 increment 2); denying fail-closed",
		}
	}
	srv := mcpperm.NewServer("car", "approve", deny)
	_ = srv.Serve(context.Background(), os.Stdin, os.Stdout)
}
