package identity

import (
	"context"
	"testing"
	"time"

	"github.com/code-all-remote/car/internal/domain"
	"github.com/code-all-remote/car/internal/storage"
)

func setupService(t *testing.T) (*Service, func()) {
	db, err := storage.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	s := NewService(db)
	return s, func() { db.Close() }
}

func TestService_CreateChallenge(t *testing.T) {
	s, cleanup := setupService(t)
	defer cleanup()

	ch, err := s.CreateChallenge(5 * time.Minute)
	if err != nil {
		t.Fatalf("CreateChallenge failed: %v", err)
	}
	if ch.Code == "" {
		t.Error("expected non-empty code")
	}
	if ch.Used {
		t.Error("expected challenge to be unused")
	}
	if ch.ExpiresAt.Before(time.Now()) {
		t.Error("expected challenge to expire in the future")
	}
}

func TestService_PairDevice_Success(t *testing.T) {
	s, cleanup := setupService(t)
	defer cleanup()

	ch, _ := s.CreateChallenge(5 * time.Minute)

	token, err := s.PairDevice(context.Background(), ch.Code, "Pixel 8", "pubkey-abc")
	if err != nil {
		t.Fatalf("PairDevice failed: %v", err)
	}
	if token.Value == "" {
		t.Error("expected non-empty token")
	}
	if token.DeviceID == "" {
		t.Error("expected non-empty device ID")
	}
}

func TestService_PairDevice_ChallengeSingleUse(t *testing.T) {
	s, cleanup := setupService(t)
	defer cleanup()

	ch, _ := s.CreateChallenge(5 * time.Minute)

	// First use succeeds.
	_, err := s.PairDevice(context.Background(), ch.Code, "Pixel 8", "pubkey-abc")
	if err != nil {
		t.Fatalf("first pair failed: %v", err)
	}

	// Second use of the SAME challenge fails.
	_, err = s.PairDevice(context.Background(), ch.Code, "Pixel 8b", "pubkey-def")
	if err != ErrChallengeExpired {
		t.Errorf("expected ErrChallengeExpired, got %v", err)
	}
}

func TestService_PairDevice_ChallengeExpired(t *testing.T) {
	s, cleanup := setupService(t)
	defer cleanup()

	// 1ms ttl so it expires immediately.
	ch, _ := s.CreateChallenge(1 * time.Millisecond)
	time.Sleep(5 * time.Millisecond)

	_, err := s.PairDevice(context.Background(), ch.Code, "Pixel 8", "pubkey-abc")
	if err != ErrChallengeExpired {
		t.Errorf("expected ErrChallengeExpired, got %v", err)
	}
}

func TestService_PairDevice_UnknownChallenge(t *testing.T) {
	s, cleanup := setupService(t)
	defer cleanup()

	_, err := s.PairDevice(context.Background(), "nonexistent", "Pixel 8", "pubkey-abc")
	if err != ErrChallengeNotFound {
		t.Errorf("expected ErrChallengeNotFound, got %v", err)
	}
}

func TestService_PairDevice_MissingFields(t *testing.T) {
	s, cleanup := setupService(t)
	defer cleanup()

	ch, _ := s.CreateChallenge(5 * time.Minute)

	_, err := s.PairDevice(context.Background(), ch.Code, "", "pubkey-abc")
	if err == nil {
		t.Error("expected error for empty device name")
	}

	// Challenge consumed on attempt; create a fresh one.
	ch2, _ := s.CreateChallenge(5 * time.Minute)
	_, err = s.PairDevice(context.Background(), ch2.Code, "Pixel 8", "")
	if err == nil {
		t.Error("expected error for empty device key")
	}
}

func TestService_RefreshToken_Success(t *testing.T) {
	s, cleanup := setupService(t)
	defer cleanup()

	ch, _ := s.CreateChallenge(5 * time.Minute)
	s.PairDevice(context.Background(), ch.Code, "Pixel 8", "pubkey-abc")

	tok, err := s.RefreshToken(context.Background(), "pubkey-abc")
	if err != nil {
		t.Fatalf("RefreshToken failed: %v", err)
	}
	if tok.Value == "" {
		t.Error("expected non-empty token")
	}
}

func TestService_RefreshToken_UnknownKey(t *testing.T) {
	s, cleanup := setupService(t)
	defer cleanup()

	_, err := s.RefreshToken(context.Background(), "nonexistent-key")
	if err == nil {
		t.Error("expected error for unknown device key")
	}
}

func TestService_RefreshToken_Revoked(t *testing.T) {
	s, cleanup := setupService(t)
	defer cleanup()

	ch, _ := s.CreateChallenge(5 * time.Minute)
	token, _ := s.PairDevice(context.Background(), ch.Code, "Pixel 8", "pubkey-abc")

	s.RevokeDevice(context.Background(), token.DeviceID)

	_, err := s.RefreshToken(context.Background(), "pubkey-abc")
	if err != ErrDeviceRevoked {
		t.Errorf("expected ErrDeviceRevoked, got %v", err)
	}
}

func TestService_AuthorizeToken_Success(t *testing.T) {
	s, cleanup := setupService(t)
	defer cleanup()

	ch, _ := s.CreateChallenge(5 * time.Minute)
	token, _ := s.PairDevice(context.Background(), ch.Code, "Pixel 8", "pubkey-abc")

	deviceID, err := s.AuthorizeToken(token.Value)
	if err != nil {
		t.Fatalf("AuthorizeToken failed: %v", err)
	}
	if deviceID != token.DeviceID {
		t.Errorf("expected device %s, got %s", token.DeviceID, deviceID)
	}
}

func TestService_AuthorizeToken_Invalid(t *testing.T) {
	s, cleanup := setupService(t)
	defer cleanup()

	_, err := s.AuthorizeToken("bogus-token")
	if err != ErrTokenInvalid {
		t.Errorf("expected ErrTokenInvalid, got %v", err)
	}
}

func TestService_AuthorizeToken_Expired(t *testing.T) {
	s, cleanup := setupService(t)
	defer cleanup()
	s.SetAccessExpiry(1 * time.Millisecond)

	ch, _ := s.CreateChallenge(5 * time.Minute)
	token, _ := s.PairDevice(context.Background(), ch.Code, "Pixel 8", "pubkey-abc")

	time.Sleep(5 * time.Millisecond)

	_, err := s.AuthorizeToken(token.Value)
	if err != ErrTokenInvalid {
		t.Errorf("expected ErrTokenInvalid, got %v", err)
	}
}

func TestService_AuthorizeToken_Revoked(t *testing.T) {
	s, cleanup := setupService(t)
	defer cleanup()

	ch, _ := s.CreateChallenge(5 * time.Minute)
	token, _ := s.PairDevice(context.Background(), ch.Code, "Pixel 8", "pubkey-abc")

	s.RevokeDevice(context.Background(), token.DeviceID)

	_, err := s.AuthorizeToken(token.Value)
	if err != ErrDeviceRevoked {
		t.Errorf("expected ErrDeviceRevoked, got %v", err)
	}
}

func TestService_RevokeDevice_Unknown(t *testing.T) {
	s, cleanup := setupService(t)
	defer cleanup()

	err := s.RevokeDevice(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error for unknown device")
	}
}

func TestService_AuthorizeWS(t *testing.T) {
	s, cleanup := setupService(t)
	defer cleanup()

	ch, _ := s.CreateChallenge(5 * time.Minute)
	token, _ := s.PairDevice(context.Background(), ch.Code, "Pixel 8", "pubkey-abc")

	// AuthorizerWS satisfies the ws.Authorizer interface.
	deviceID, err := s.AuthorizeWS(token.Value)
	if err != nil {
		t.Fatalf("AuthorizeWS failed: %v", err)
	}
	if deviceID != token.DeviceID {
		t.Errorf("expected device %s, got %s", token.DeviceID, deviceID)
	}

	// Revoked device cannot establish WS.
	s.RevokeDevice(context.Background(), token.DeviceID)
	_, err = s.AuthorizeWS(token.Value)
	if err != ErrDeviceRevoked {
		t.Errorf("expected ErrDeviceRevoked on WS, got %v", err)
	}
}

func TestService_ChallengeCodeFormat(t *testing.T) {
	s, cleanup := setupService(t)
	defer cleanup()

	ch, _ := s.CreateChallenge(5 * time.Minute)
	// Format is XXXX-XXXX-... (6 groups of 4 hex chars, separated by -).
	if len(ch.Code) != 29 { // 6*4 + 5 separators
		t.Errorf("expected 29-char code, got %d: %s", len(ch.Code), ch.Code)
	}
}

func TestService_TokensAreUnique(t *testing.T) {
	s, cleanup := setupService(t)
	defer cleanup()

	ch1, _ := s.CreateChallenge(5 * time.Minute)
	t1, _ := s.PairDevice(context.Background(), ch1.Code, "Device A", "key-a")

	ch2, _ := s.CreateChallenge(5 * time.Minute)
	t2, _ := s.PairDevice(context.Background(), ch2.Code, "Device B", "key-b")

	if t1.Value == t2.Value {
		t.Error("expected distinct token values")
	}
	if t1.DeviceID == t2.DeviceID {
		t.Error("expected distinct device IDs")
	}
}

func TestService_DeviceStateConstants(t *testing.T) {
	if domain.DeviceStatePaired != "paired" {
		t.Error("DeviceStatePaired mismatch")
	}
	if domain.DeviceStateRevoked != "revoked" {
		t.Error("DeviceStateRevoked mismatch")
	}
	if domain.DeviceStatePending != "pending" {
		t.Error("DeviceStatePending mismatch")
	}
}
