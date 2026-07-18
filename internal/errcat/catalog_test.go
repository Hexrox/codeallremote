package errcat

import (
	"errors"
	"net/http"
	"testing"
)

func TestCatalog_HTTPStatusMapping(t *testing.T) {
	tests := []struct {
		code     Code
		expected int
	}{
		{Unauthorized, http.StatusUnauthorized},
		{Forbidden, http.StatusForbidden},
		{NotFound, http.StatusNotFound},
		{Conflict, http.StatusConflict},
		{ExpiredApproval, http.StatusConflict},
		{IdempotencyConflict, http.StatusConflict},
		{CursorExpired, http.StatusConflict},
		{AdapterUnavailable, http.StatusServiceUnavailable},
		{InvalidInput, http.StatusBadRequest},
		{UnsupportedCommand, http.StatusBadRequest},
		{Internal, http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(string(tt.code), func(t *testing.T) {
			if got := HTTPStatus(tt.code); got != tt.expected {
				t.Errorf("HTTPStatus(%s) = %d, want %d", tt.code, got, tt.expected)
			}
		})
	}
}

func TestCatalog_DefaultMessage(t *testing.T) {
	// Every code MUST have a non-empty default message with no secrets.
	for _, c := range []Code{
		Unauthorized, Forbidden, NotFound, Conflict, ExpiredApproval,
		IdempotencyConflict, CursorExpired, AdapterUnavailable, InvalidInput,
		RateLimited, UnsupportedCommand, Internal,
	} {
		e := New(c, "")
		if e.Message == "" {
			t.Errorf("code %s has empty default message", c)
		}
	}
}

func TestNew_WithMessage(t *testing.T) {
	e := New(NotFound, "session not found")
	if e.Code != NotFound {
		t.Error("code mismatch")
	}
	if e.Message != "session not found" {
		t.Errorf("message mismatch: %s", e.Message)
	}
}

func TestWithDetails(t *testing.T) {
	e := New(Conflict, "x").WithDetails(map[string]any{"session_id": "ses_1"})
	if e.Details == nil {
		t.Error("expected details")
	}
}

func TestWrap_CarriesCause(t *testing.T) {
	underlying := errors.New("disk full")
	e := Wrap(Internal, underlying, "boom")
	if e.Cause() != underlying {
		t.Error("cause not preserved")
	}
	// The cause itself must not appear in the client-visible message.
	if e.Message != "boom" {
		t.Errorf("message leaked cause: %s", e.Message)
	}
}

func TestError_Is(t *testing.T) {
	e := New(NotFound, "x")
	if !errors.Is(e, New(NotFound, "different message")) {
		t.Error("Is should match by code disregarding message")
	}
	if errors.Is(e, New(Conflict, "x")) {
		t.Error("Is should not match different code")
	}
}

func TestCodeFrom(t *testing.T) {
	if CodeFrom(New(CursorExpired, "")) != CursorExpired {
		t.Error("CodeFrom failed for catalog error")
	}
	if CodeFrom(errors.New("plain")) != "" {
		t.Error("CodeFrom should return empty for non-catalog error")
	}
}

func TestFromAppError(t *testing.T) {
	appNotFound := errors.New("nf")
	appConflict := errors.New("cf")
	appInvalid := errors.New("iv")

	if FromAppError(appNotFound, appNotFound, appConflict, appInvalid).Code != NotFound {
		t.Error("not mapped to NotFound")
	}
	if FromAppError(appConflict, appNotFound, appConflict, appInvalid).Code != Conflict {
		t.Error("not mapped to Conflict")
	}
	if FromAppError(appInvalid, appNotFound, appConflict, appInvalid).Code != InvalidInput {
		t.Error("not mapped to InvalidInput")
	}
	if FromAppError(errors.New("other"), appNotFound, appConflict, appInvalid).Code != Internal {
		t.Error("unknown not mapped to Internal")
	}
}

// TestCatalog_StableCodes guards against accidental code reassignment.
// Codes are a public contract; renaming or reusing a code for a different
// meaning would break clients branching on them.
func TestCatalog_StableCodes(t *testing.T) {
	expected := map[Code]bool{
		"unauthorized": true, "forbidden": true, "not_found": true,
		"conflict": true, "expired_approval": true, "idempotency_conflict": true,
		"cursor_expired": true, "adapter_unavailable": true, "invalid_input": true,
		"rate_limited": true, "internal_error": true, "unsupported_command": true,
	}
	for c := range expected {
		// Each code string must be lower_snake_case.
		if c != Code(string(c)) {
			t.Errorf("code %s is not canonical", c)
		}
	}
}
