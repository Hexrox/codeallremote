package protocol

import (
	"encoding/json"
	"fmt"
)

// --- Schema validators ---
//
// Each validator enforces the documented contract for its command type.
// Validators are intentionally strict: fields with secrets, oversized payloads
// or missing required values are rejected here, before the adapter.

// MaxPromptLength bounds the size of a prompt payload (matches the REST API).
const MaxPromptLength = 100000

// MaxReasonLength bounds an approval decision reason.
const MaxReasonLength = 2000

// MinIdempotencyLen / MaxIdempotencyLen bound the idempotency key.
const (
	MinIdempotencyLen = 8
	MaxIdempotencyLen = 200
)

// PromptPayload is the payload shape for session.prompt.
type PromptPayload struct {
	Text string `json:"text"`
}

// promptSchema validates session.prompt.
type promptSchema struct{}

func (promptSchema) Validate(payload json.RawMessage) error {
	var p PromptPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("invalid prompt payload: %w", err)
	}
	if p.Text == "" {
		return fmt.Errorf("text is required")
	}
	if len(p.Text) > MaxPromptLength {
		return fmt.Errorf("text exceeds %d characters", MaxPromptLength)
	}
	return nil
}

// StartPayload is the payload shape for session.start.
type StartPayload struct {
	WorkspaceID string `json:"workspace_id"`
	AdapterID   string `json:"adapter_id"`
	Title       string `json:"title,omitempty"`
}

// startSchema validates session.start.
type startSchema struct{}

func (startSchema) Validate(payload json.RawMessage) error {
	var p StartPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("invalid start payload: %w", err)
	}
	if p.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}
	if p.AdapterID == "" {
		return fmt.Errorf("adapter_id is required")
	}
	return nil
}

// InterruptPayload is the payload shape for session.interrupt (empty).
type InterruptPayload struct{}

// interruptSchema validates session.interrupt (no payload fields).
type interruptSchema struct{}

func (interruptSchema) Validate(payload json.RawMessage) error {
	// Payload is intentionally empty; tolerate an absent or empty object.
	if len(payload) == 0 || string(payload) == "null" {
		return nil
	}
	var p InterruptPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("invalid interrupt payload: %w", err)
	}
	return nil
}

// DecisionPayload is the payload shape for approval.decide.
type DecisionPayload struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason,omitempty"`
}

// decideSchema validates approval.decide.
type decideSchema struct{}

func (decideSchema) Validate(payload json.RawMessage) error {
	var p DecisionPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("invalid decision payload: %w", err)
	}
	if p.Decision != "approve" && p.Decision != "deny" {
		return fmt.Errorf("decision must be approve or deny")
	}
	if len(p.Reason) > MaxReasonLength {
		return fmt.Errorf("reason exceeds %d characters", MaxReasonLength)
	}
	return nil
}

// Schemas returns the schema for each documented command type.
// Callers register these with the dispatcher.
func Schemas() map[CommandType]CommandSchema {
	return map[CommandType]CommandSchema{
		CmdSessionStart:     startSchema{},
		CmdSessionPrompt:    promptSchema{},
		CmdSessionInterrupt: interruptSchema{},
		CmdApprovalDecide:   decideSchema{},
	}
}

// ValidateIdempotencyKey checks the key length requirement (shared by all
// write commands).
func ValidateIdempotencyKey(key string) error {
	if len(key) < MinIdempotencyLen || len(key) > MaxIdempotencyLen {
		return fmt.Errorf("idempotency key must be %d-%d characters", MinIdempotencyLen, MaxIdempotencyLen)
	}
	return nil
}
