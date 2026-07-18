// Package auth provides authentication helpers shared by the server and API
// middleware: strict bearer-token extraction and constant-time comparison.
package auth

import (
	"crypto/subtle"
	"strings"
)

// BearerToken extracts the token from an "Authorization: Bearer <token>"
// header value. It requires the exact "Bearer " prefix (no length-only
// stripping) and returns "" if the header is malformed.
func BearerToken(header string) string {
	const prefix = "Bearer "
	if len(header) < len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return ""
	}
	return header[len(prefix):]
}

// ConstantTimeEqual reports whether a and b are equal in constant time. It
// returns false (not false-with-timing-leak) when lengths differ.
func ConstantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
