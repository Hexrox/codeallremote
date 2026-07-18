package auth

import (
	"testing"
)

func TestBearerToken(t *testing.T) {
	tests := []struct {
		header string
		want   string
	}{
		{"Bearer abc123", "abc123"},
		{"bearer abc123", "abc123"}, // case-insensitive scheme
		{"BEARER abc", "abc"},
		{"Token abc", ""}, // wrong scheme
		{"Bearer", ""},    // too short
		{"Bearer", ""},    // no space
		{"Bear abc", ""},  // prefix not exact
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.header, func(t *testing.T) {
			if got := BearerToken(tt.header); got != tt.want {
				t.Errorf("BearerToken(%q) = %q, want %q", tt.header, got, tt.want)
			}
		})
	}
}

func TestConstantTimeEqual(t *testing.T) {
	if !ConstantTimeEqual("abc", "abc") {
		t.Error("equal strings should match")
	}
	if ConstantTimeEqual("abc", "abd") {
		t.Error("different strings should not match")
	}
	if ConstantTimeEqual("abc", "abcd") {
		t.Error("different lengths should not match")
	}
}

// TestBearerToken_StrictPrefix ensures "BearerXYZ" (no space) is rejected,
// not length-stripped. Regression for the S13 finding.
func TestBearerToken_StrictPrefix(t *testing.T) {
	if got := BearerToken("BearerXYZabc"); got != "" {
		t.Errorf("expected empty for malformed prefix, got %q", got)
	}
}
