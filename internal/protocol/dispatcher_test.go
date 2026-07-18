package protocol

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// memoryIdempotent is a minimal IdempotencyStore for tests.
type memoryIdempotent struct {
	store map[string]Result
}

func newMemIdempotent() *memoryIdempotent {
	return &memoryIdempotent{store: make(map[string]Result)}
}
func (m *memoryIdempotent) Get(key string) (Result, bool) {
	r, ok := m.store[key]
	return r, ok
}
func (m *memoryIdempotent) Add(key string, r Result) { m.store[key] = r }

// staticAuth allows/denies based on a flag.
type staticAuth struct{ allow bool }

func (s staticAuth) Authenticate(ctx context.Context, cmd Command) (string, error) {
	if !s.allow {
		return "", errors.New("denied")
	}
	return "owner", nil
}

func TestDispatcher_UnsupportedCommandRejected(t *testing.T) {
	d := NewDispatcher(staticAuth{allow: true}, newMemIdempotent())
	cmd := Command{Type: "session.nuke", IdempotencyKey: "key-1234", ProtocolVersion: 1}
	r := d.Dispatch(context.Background(), cmd)
	if r.Error == nil || r.Error.Code != "unsupported_command" {
		t.Errorf("expected unsupported_command, got %+v", r.Error)
	}
}

func TestDispatcher_AuthRejected(t *testing.T) {
	d := NewDispatcher(staticAuth{allow: false}, newMemIdempotent())
	cmd := Command{Type: CmdSessionPrompt, IdempotencyKey: "key-1234", ProtocolVersion: 1}
	r := d.Dispatch(context.Background(), cmd)
	if r.Error == nil || r.Error.Code != "unauthorized" {
		t.Errorf("expected unauthorized, got %+v", r.Error)
	}
}

func TestDispatcher_ProtocolVersionIncompatible(t *testing.T) {
	d := NewDispatcher(staticAuth{allow: true}, newMemIdempotent())
	cmd := Command{Type: CmdSessionPrompt, IdempotencyKey: "key-1234", ProtocolVersion: 99}
	r := d.Dispatch(context.Background(), cmd)
	if r.Error == nil || r.Error.Code != "unsupported_command" {
		t.Errorf("expected unsupported_command for incompatible version, got %+v", r.Error)
	}
}

func TestDispatcher_AdditiveVersionAccepted(t *testing.T) {
	// A client declaring a lower (still supported) version MUST be accepted
	// for additive fields (docs/34-protocol-versioning.md).
	d := NewDispatcher(staticAuth{allow: true}, newMemIdempotent())
	schemas := Schemas()
	d.Register(CmdSessionPrompt, schemas[CmdSessionPrompt], HandlerFunc(func(ctx context.Context, cmd Command) Result {
		return Result{Accepted: true}
	}))
	payload, _ := json.Marshal(PromptPayload{Text: "hello"})
	cmd := Command{Type: CmdSessionPrompt, IdempotencyKey: "key-1234", ProtocolVersion: 1, Payload: payload}
	r := d.Dispatch(context.Background(), cmd)
	if r.Error != nil {
		t.Errorf("expected accept, got %+v", r.Error)
	}
}

func TestDispatcher_IdempotencyReplay(t *testing.T) {
	idem := newMemIdempotent()
	d := NewDispatcher(staticAuth{allow: true}, idem)
	called := 0
	schemas := Schemas()
	d.Register(CmdSessionPrompt, schemas[CmdSessionPrompt], HandlerFunc(func(ctx context.Context, cmd Command) Result {
		called++
		return Result{Accepted: true, Message: "ok"}
	}))
	payload, _ := json.Marshal(PromptPayload{Text: "hello"})

	cmd := Command{Type: CmdSessionPrompt, IdempotencyKey: "key-1234", ProtocolVersion: 1, Payload: payload}
	r1 := d.Dispatch(context.Background(), cmd)
	if !r1.Accepted {
		t.Fatal("expected accepted")
	}
	// Replay with the same key returns the stored result WITHOUT calling handler again.
	r2 := d.Dispatch(context.Background(), cmd)
	if r2.Message != r1.Message {
		t.Errorf("replay mismatch: %s vs %s", r2.Message, r1.Message)
	}
	if called != 1 {
		t.Errorf("expected handler called once, got %d", called)
	}
}

func TestDispatcher_SchemaValidationRejects(t *testing.T) {
	d := NewDispatcher(staticAuth{allow: true}, newMemIdempotent())
	schemas := Schemas()
	d.Register(CmdSessionPrompt, schemas[CmdSessionPrompt], HandlerFunc(func(ctx context.Context, cmd Command) Result {
		t.Error("handler should not be reached on schema failure")
		return Result{}
	}))
	// Empty text fails validation.
	payload, _ := json.Marshal(PromptPayload{Text: ""})
	cmd := Command{Type: CmdSessionPrompt, IdempotencyKey: "key-1234", ProtocolVersion: 1, Payload: payload}
	r := d.Dispatch(context.Background(), cmd)
	if r.Error == nil || r.Error.Code != "invalid_input" {
		t.Errorf("expected invalid_input, got %+v", r.Error)
	}
}

func TestDispatcher_DocumentedCommandWithNoHandlerFailsClosed(t *testing.T) {
	d := NewDispatcher(staticAuth{allow: true}, newMemIdempotent())
	// CmdSessionPrompt is documented but not registered.
	payload, _ := json.Marshal(PromptPayload{Text: "hi"})
	cmd := Command{Type: CmdSessionPrompt, IdempotencyKey: "key-1234", ProtocolVersion: 1, Payload: payload}
	r := d.Dispatch(context.Background(), cmd)
	if r.Error == nil || r.Error.Code != "unsupported_command" {
		t.Errorf("expected unsupported_command (not configured), got %+v", r.Error)
	}
}

func TestSchemas_PromptValidates(t *testing.T) {
	s := Schemas()[CmdSessionPrompt]
	good, _ := json.Marshal(PromptPayload{Text: "hello"})
	if err := s.Validate(good); err != nil {
		t.Errorf("expected valid: %v", err)
	}
	bad, _ := json.Marshal(PromptPayload{Text: ""})
	if err := s.Validate(bad); err == nil {
		t.Error("expected empty text to fail")
	}
}

func TestSchemas_StartValidates(t *testing.T) {
	s := Schemas()[CmdSessionStart]
	good, _ := json.Marshal(StartPayload{WorkspaceID: "ws", AdapterID: "claude-code"})
	if err := s.Validate(good); err != nil {
		t.Errorf("expected valid: %v", err)
	}
	bad, _ := json.Marshal(StartPayload{WorkspaceID: "", AdapterID: "claude-code"})
	if err := s.Validate(bad); err == nil {
		t.Error("expected empty workspace to fail")
	}
}

func TestSchemas_DecideValidates(t *testing.T) {
	s := Schemas()[CmdApprovalDecide]
	good, _ := json.Marshal(DecisionPayload{Decision: "approve"})
	if err := s.Validate(good); err != nil {
		t.Errorf("expected valid: %v", err)
	}
	bad, _ := json.Marshal(DecisionPayload{Decision: "maybe"})
	if err := s.Validate(bad); err == nil {
		t.Error("expected invalid decision to fail")
	}
}

func TestValidateIdempotencyKey(t *testing.T) {
	if err := ValidateIdempotencyKey("short"); err == nil {
		t.Error("expected short key to fail")
	}
	if err := ValidateIdempotencyKey("validkey-12345"); err != nil {
		t.Errorf("expected valid key: %v", err)
	}
}

func TestIsDocumented(t *testing.T) {
	if !IsDocumented(CmdSessionPrompt) {
		t.Error("CmdSessionPrompt should be documented")
	}
	if IsDocumented(CommandType("session.bogus")) {
		t.Error("session.bogus should NOT be documented")
	}
}

func TestDispatcher_FullPromptFlowEmitsEvents(t *testing.T) {
	// Verify an accepted session.prompt returns the documented event type list.
	d := NewDispatcher(staticAuth{allow: true}, newMemIdempotent())
	schemas := Schemas()
	d.Register(CmdSessionPrompt, schemas[CmdSessionPrompt], HandlerFunc(func(ctx context.Context, cmd Command) Result {
		return Result{Accepted: true, EventTypes: []string{"run.prompt"}}
	}))
	payload, _ := json.Marshal(PromptPayload{Text: "do the thing"})
	cmd := Command{Type: CmdSessionPrompt, IdempotencyKey: "key-1234", ProtocolVersion: 1, Payload: payload}
	r := d.Dispatch(context.Background(), cmd)
	if !r.Accepted {
		t.Fatal("expected accepted")
	}
	if len(r.EventTypes) != 1 || r.EventTypes[0] != "run.prompt" {
		t.Errorf("expected run.prompt event, got %+v", r.EventTypes)
	}
}
