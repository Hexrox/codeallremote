// Package protocol implements the CAR protocol contract: command dispatch,
// schema validation, protocol-version negotiation and idempotency.
//
// Commands are authenticated, schema-validated and idempotency-aware client
// intents. Accepted commands emit documented lifecycle events. Unsupported
// commands return a stable error without reaching an adapter.
package protocol

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/code-all-remote/car/internal/errcat"
)

// SupportedProtocolVersion is the protocol version this server speaks.
// Additive changes bump the minor; breaking changes require a new major and
// migration fixtures (docs/34-protocol-versioning.md).
const SupportedProtocolVersion = 1

// CommandType identifies a client command. Unknown types are rejected by the
// dispatcher without reaching an adapter.
type CommandType string

// Documented command types.
const (
	CmdSessionStart     CommandType = "session.start"
	CmdSessionPrompt    CommandType = "session.prompt"
	CmdSessionInterrupt CommandType = "session.interrupt"
	CmdApprovalDecide   CommandType = "approval.decide"
)

// documentedCommands is the set of command types the dispatcher accepts.
// Adding a command is additive; removing or repurposing one is breaking.
var documentedCommands = map[CommandType]bool{
	CmdSessionStart:     true,
	CmdSessionPrompt:    true,
	CmdSessionInterrupt: true,
	CmdApprovalDecide:   true,
}

// Command is the on-the-wire command envelope.
type Command struct {
	CommandID      string          `json:"command_id"`
	Type           CommandType     `json:"type"`
	SessionID      string          `json:"session_id,omitempty"`
	IdempotencyKey string          `json:"idempotency_key"`
	Payload        json.RawMessage `json:"payload"`
	// ProtocolVersion is the client-declared version. Defaults to 1 if absent
	// (additive field readable by prior clients per docs/34-protocol-versioning.md).
	ProtocolVersion int `json:"protocol_version,omitempty"`
}

// Handler handles a validated command. Implementations carry the command to
// the app layer and return a stable result for the dispatcher.
type Handler interface {
	Handle(ctx context.Context, cmd Command) Result
}

// HandlerFunc adapts a function to Handler.
type HandlerFunc func(ctx context.Context, cmd Command) Result

// Handle implements Handler.
func (f HandlerFunc) Handle(ctx context.Context, cmd Command) Result { return f(ctx, cmd) }

// Result is the outcome of dispatching a command.
type Result struct {
	Accepted bool          `json:"accepted"`
	Message  string        `json:"message,omitempty"`
	Error    *errcat.Error `json:"error,omitempty"`
	// EventTypes lists the lifecycle event types the accepted command emits,
	// so callers (and tests) can assert the documented contract.
	EventTypes []string `json:"event_types,omitempty"`
}

// CommandSchema validates a command payload against its documented shape.
// Validators are registered per command type; an unvalidated command type is
// rejected as UnsupportedCommand (defensive: better to reject than misroute).
type CommandSchema interface {
	Validate(payload json.RawMessage) error
}

// Dispatcher routes validated commands to handlers and enforces the contract:
// authentication (caller-provided), schema validation, idempotency, and
// rejection of unsupported commands before they reach an adapter.
type Dispatcher struct {
	mu         sync.RWMutex
	handlers   map[CommandType]Handler
	schemas    map[CommandType]CommandSchema
	idempotent IdempotencyStore
	auth       Authenticator
}

// IdempotencyStore stores request results by idempotency key.
type IdempotencyStore interface {
	Get(key string) (Result, bool)
	Add(key string, r Result)
}

// Authenticator authenticates the actor submitting a command. Returning an
// error rejects the command with Unauthorized before any further processing.
type Authenticator interface {
	Authenticate(ctx context.Context, cmd Command) (actorID string, err error)
}

// NewDispatcher creates a new dispatcher with the given authenticator and
// idempotency store (either may be nil to skip that check).
func NewDispatcher(auth Authenticator, idempotent IdempotencyStore) *Dispatcher {
	return &Dispatcher{
		handlers:   make(map[CommandType]Handler),
		schemas:    make(map[CommandType]CommandSchema),
		auth:       auth,
		idempotent: idempotent,
	}
}

// Register associates a command type with its handler and schema.
// Registering an undocumented command type is a programming error (panic-free;
// the dispatcher returns UnsupportedCommand for unregistered types).
func (d *Dispatcher) Register(t CommandType, schema CommandSchema, h Handler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.handlers[t] = h
	d.schemas[t] = schema
}

// Dispatch validates and routes a command. The steps, in order:
//  1. Authenticate (caller-provided Authenticator).
//  2. Negotiate protocol version (reject incompatible clients with a safe
//     diagnostic and no session data).
//  3. Idempotency: replay a stored result for a known key.
//  4. Reject unsupported command types before reaching an adapter.
//  5. Schema-validate the payload.
//  6. Delegate to the handler.
//  7. Store the result under the idempotency key.
func (d *Dispatcher) Dispatch(ctx context.Context, cmd Command) Result {
	// 1. Authentication.
	if d.auth != nil {
		if _, err := d.auth.Authenticate(ctx, cmd); err != nil {
			return Result{Error: errcat.New(errcat.Unauthorized, "")}
		}
	}

	// 2. Protocol negotiation: reject clients declaring an incompatible major.
	if cmd.ProtocolVersion != 0 && cmd.ProtocolVersion > SupportedProtocolVersion {
		return Result{
			Error: errcat.New(errcat.UnsupportedCommand,
				fmt.Sprintf("protocol version %d is not supported (max %d)", cmd.ProtocolVersion, SupportedProtocolVersion)),
		}
	}

	// 3. Idempotency: return the stored result if this key was already seen.
	if d.idempotent != nil && cmd.IdempotencyKey != "" {
		if r, ok := d.idempotent.Get(cmd.IdempotencyKey); ok {
			return r
		}
	}

	// 4. Reject unsupported command types before reaching an adapter.
	if !documentedCommands[cmd.Type] {
		return Result{Error: errcat.New(errcat.UnsupportedCommand, "unknown command type "+string(cmd.Type))}
	}

	d.mu.RLock()
	handler, ok := d.handlers[cmd.Type]
	schema := d.schemas[cmd.Type]
	d.mu.RUnlock()

	if !ok || schema == nil {
		// Documented but no handler registered: treat as UnsupportedCommand
		// so a misconfigured server fails closed.
		return Result{Error: errcat.New(errcat.UnsupportedCommand, "command not configured")}
	}

	// 5. Schema validation.
	if err := schema.Validate(cmd.Payload); err != nil {
		return Result{Error: errcat.New(errcat.InvalidInput, err.Error())}
	}

	// 6. Delegate.
	r := handler.Handle(ctx, cmd)
	if r.Error == nil && !r.Accepted {
		r.Accepted = true
	}

	// 7. Store under the idempotency key.
	if d.idempotent != nil && cmd.IdempotencyKey != "" {
		d.idempotent.Add(cmd.IdempotencyKey, r)
	}

	return r
}

// IsDocumented reports whether a command type is part of the documented
// protocol contract.
func IsDocumented(t CommandType) bool { return documentedCommands[t] }
