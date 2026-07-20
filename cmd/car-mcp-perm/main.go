// Command car-mcp-perm is a stdio MCP permission-prompt server for Claude Code
// (ADR-010). Claude is launched with --permission-prompt-tool
// mcp__car__approve --mcp-config <cfg> where the config runs this binary.
//
// It is a thin, fail-closed transport: each permission request is forwarded
// over a per-run unix socket to the CAR adapter (--socket), which turns it into
// a real ApprovalBridge request and returns the phone's decision. With no
// --socket (or any transport error) it denies fail-closed.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"time"

	"github.com/code-all-remote/car/internal/mcpperm"
)

func main() {
	socket := flag.String("socket", "", "unix socket the CAR adapter listens on for permission decisions")
	session := flag.String("session", "", "CAR session id this run belongs to")
	timeout := flag.Duration("timeout", 5*time.Minute, "max time to wait for an approval decision")
	flag.Parse()

	decide := func(toolName string, input json.RawMessage, toolUseID string) mcpperm.Decision {
		if *socket == "" {
			return mcpperm.Decision{
				Allow:   false,
				Message: "CAR: no approval socket configured; denying fail-closed",
			}
		}
		return mcpperm.DecideOverSocket(*socket, mcpperm.PermissionRequest{
			Session:   *session,
			ToolName:  toolName,
			ToolUseID: toolUseID,
			Input:     input,
		}, *timeout)
	}

	srv := mcpperm.NewServer("car", "approve", decide)
	_ = srv.Serve(context.Background(), os.Stdin, os.Stdout)
}
