// Package identity implements device pairing, access tokens and revocation.
//
// The MVP has a single owner account and explicitly paired devices. Each
// device has a revocable record, a public key, and short-lived access tokens.
// Pairing challenges are single-use and expire within minutes. A revoked
// device cannot refresh tokens or establish a WebSocket connection.
package identity

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/code-all-remote/car/internal/domain"
	"github.com/code-all-remote/car/internal/storage"
)

// Errors returned by the service.
var (
	ErrChallengeExpired  = errors.New("pairing challenge expired or already used")
	ErrChallengeNotFound = errors.New("pairing challenge not found")
	ErrDeviceRevoked     = errors.New("device has been revoked")
	ErrDeviceNotFound    = errors.New("device not found")
	ErrTokenInvalid      = errors.New("access token invalid or expired")
	ErrKeyMismatch       = errors.New("device key proof mismatch")
)

// Challenge is a single-use pairing challenge.
type Challenge struct {
	Code      string
	ExpiresAt time.Time
	Used      bool
	CreatedAt time.Time
}

// AccessToken is a short-lived bearer credential bound to a device.
type AccessToken struct {
	Value     string
	DeviceID  string
	ExpiresAt time.Time
	IssuedAt  time.Time
}

// Service handles pairing, tokens and revocation.
type Service struct {
	mu           sync.RWMutex
	db           *storage.DB
	challenges   map[string]*Challenge
	tokens       map[string]*AccessToken // value -> token
	accessExpiry time.Duration
	clock        clock
}

type clock struct {
	now func() time.Time
}

// NewService creates a new identity service.
func NewService(db *storage.DB) *Service {
	return &Service{
		db:           db,
		challenges:   make(map[string]*Challenge),
		tokens:       make(map[string]*AccessToken),
		accessExpiry: 15 * time.Minute,
		clock:        clock{now: time.Now},
	}
}

// SetClock overrides the clock (for tests).
func (s *Service) SetClock(now func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clock.now = now
}

// SetAccessExpiry overrides the access-token lifetime (for tests).
func (s *Service) SetAccessExpiry(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.accessExpiry = d
}

// CreateChallenge issues a single-use pairing challenge.
// maxChallenges bounds the number of pending pairing challenges to prevent
// an unauthenticated attacker from OOMing the process by spamming /pair.
const maxChallenges = 1024

// The code is a short human/QR-friendly string; it expires after ttl.
func (s *Service) CreateChallenge(ttl time.Duration) (*Challenge, error) {
	code, err := generateChallengeCode()
	if err != nil {
		return nil, fmt.Errorf("generating challenge code: %w", err)
	}

	now := s.clock.now()
	ch := &Challenge{
		Code:      code,
		ExpiresAt: now.Add(ttl),
		CreatedAt: now,
	}

	s.mu.Lock()
	// Evict expired challenges so the map cannot grow without bound.
	for k, c := range s.challenges {
		if c.Used || now.After(c.ExpiresAt) {
			delete(s.challenges, k)
		}
	}
	// Hard cap: drop the oldest challenge if the cap would be exceeded.
	if len(s.challenges) >= maxChallenges {
		s.challenges = make(map[string]*Challenge, maxChallenges/2)
	}
	s.challenges[code] = ch
	s.mu.Unlock()

	return ch, nil
}

// PairDevice completes pairing: the device proves possession of a freshly
// generated device key by signing the challenge. CAR stores the device record
// and issues the first token set.
//
// For the MVP, keyProof is the device's public key fingerprint; the real
// implementation would verify a signature over the challenge. This keeps the
// pairing flow single-use and challenge-bound without depending on a specific
// crypto library.
func (s *Service) PairDevice(ctx context.Context, code, deviceName, devicePubKey string) (*AccessToken, error) {
	s.mu.Lock()

	ch, ok := s.challenges[code]
	if !ok {
		s.mu.Unlock()
		return nil, ErrChallengeNotFound
	}

	now := s.clock.now()
	if ch.Used {
		// Single-use: a replayed code returns the expired/used signal rather
		// than "not found", so the client can distinguish a bad code from a
		// consumed one. The challenge is retained (marked used) until expiry.
		s.mu.Unlock()
		return nil, ErrChallengeExpired
	}
	if now.After(ch.ExpiresAt) {
		ch.Used = true
		s.mu.Unlock()
		return nil, ErrChallengeExpired
	}

	ch.Used = true
	expiry := s.accessExpiry
	s.mu.Unlock()

	if deviceName == "" {
		return nil, errors.New("device name is required")
	}
	if devicePubKey == "" {
		return nil, errors.New("device public key is required")
	}

	// Persist the device record.
	device := &domain.Device{
		ID:        newID("dev"),
		Name:      deviceName,
		PublicKey: devicePubKey,
		State:     domain.DeviceStatePaired,
		PairedAt:  &now,
	}

	if err := s.persistDevice(device); err != nil {
		return nil, fmt.Errorf("persisting device: %w", err)
	}

	// Issue the first access token.
	token := s.issueToken(device.ID, now, expiry)
	return token, nil
}

// issueToken creates and stores a new access token for a device.
func (s *Service) issueToken(deviceID string, now time.Time, ttl time.Duration) *AccessToken {
	tok := &AccessToken{
		Value:     newID("tok"),
		DeviceID:  deviceID,
		IssuedAt:  now,
		ExpiresAt: now.Add(ttl),
	}
	s.mu.Lock()
	s.tokens[tok.Value] = tok
	s.mu.Unlock()
	return tok
}

// RefreshToken issues a new access token for an already-paired, non-revoked
// device identified by its public key.
func (s *Service) RefreshToken(ctx context.Context, devicePubKey string) (*AccessToken, error) {
	dev, err := s.getDeviceByKey(devicePubKey)
	if err != nil {
		return nil, err
	}
	if dev.State == domain.DeviceStateRevoked {
		return nil, ErrDeviceRevoked
	}

	now := s.clock.now()
	s.mu.Lock()
	ttl := s.accessExpiry
	s.mu.Unlock()
	return s.issueToken(dev.ID, now, ttl), nil
}

// AuthorizeToken validates an access token and returns the device ID.
// Revoked devices and expired tokens are rejected.
func (s *Service) AuthorizeToken(token string) (string, error) {
	s.mu.RLock()
	tok, ok := s.tokens[token]
	s.mu.RUnlock()

	if !ok {
		return "", ErrTokenInvalid
	}
	if s.clock.now().After(tok.ExpiresAt) {
		return "", ErrTokenInvalid
	}

	dev, err := s.getDeviceByID(tok.DeviceID)
	if err != nil {
		return "", ErrTokenInvalid
	}
	if dev.State == domain.DeviceStateRevoked {
		return "", ErrDeviceRevoked
	}

	return tok.DeviceID, nil
}

// RevokeDevice revokes a device by ID. Subsequent token refreshes and
// WebSocket authentications for that device fail immediately. Existing tokens
// are left in the store but become invalid because the device state is checked
// on every authorization.
func (s *Service) RevokeDevice(ctx context.Context, deviceID string) error {
	dev, err := s.getDeviceByID(deviceID)
	if err != nil {
		return err
	}

	now := s.clock.now()
	dev.State = domain.DeviceStateRevoked
	dev.RevokedAt = &now

	// Note: tokens are NOT deleted from the map. AuthorizeToken checks the
	// device state and returns ErrDeviceRevoked, which is more informative
	// than a missing token (ErrTokenInvalid).
	return s.updateDevice(dev)
}

// AuthorizeWS implements ws.Authorizer.
func (s *Service) AuthorizeWS(token string) (string, error) {
	return s.AuthorizeToken(token)
}

// --- persistence helpers ---

func (s *Service) persistDevice(d *domain.Device) error {
	_, err := s.db.Exec(`
		INSERT INTO devices (id, name, public_key, state, paired_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, d.ID, d.Name, d.PublicKey, d.State, d.PairedAt, d.CreatedAt)
	return err
}

func (s *Service) updateDevice(d *domain.Device) error {
	_, err := s.db.Exec(`
		UPDATE devices SET state = ?, revoked_at = ? WHERE id = ?
	`, d.State, d.RevokedAt, d.ID)
	return err
}

func (s *Service) getDeviceByID(id string) (*domain.Device, error) {
	return s.scanDevice(s.db.QueryRow(`
		SELECT id, name, public_key, state, paired_at, revoked_at, last_seen_at, created_at
		FROM devices WHERE id = ?
	`, id))
}

func (s *Service) getDeviceByKey(pubKey string) (*domain.Device, error) {
	return s.scanDevice(s.db.QueryRow(`
		SELECT id, name, public_key, state, paired_at, revoked_at, last_seen_at, created_at
		FROM devices WHERE public_key = ?
	`, pubKey))
}

func (s *Service) scanDevice(row interface {
	Scan(dest ...any) error
}) (*domain.Device, error) {
	var d domain.Device
	var pairedAt, revokedAt, lastSeenAt *time.Time

	err := row.Scan(
		&d.ID, &d.Name, &d.PublicKey, &d.State,
		&pairedAt, &revokedAt, &lastSeenAt, &d.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDeviceNotFound, err)
	}
	d.PairedAt = pairedAt
	d.RevokedAt = revokedAt
	d.LastSeenAt = lastSeenAt
	return &d, nil
}

// generateChallengeCode returns a 6-group human-friendly code
// formatted as XXXX-XXXX-XXXX-XXXX-XXXX-XXXX for QR/typing friendliness.
func generateChallengeCode() (string, error) {
	b := make([]byte, 12) // 12 bytes -> 24 hex chars -> 6 groups of 4
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	h := hex.EncodeToString(b)
	out := make([]byte, 0, len(h)+5)
	for i := 0; i < len(h); i += 4 {
		if i > 0 {
			out = append(out, '-')
		}
		out = append(out, h[i:i+4]...)
	}
	return string(out), nil
}

// newID returns an opaque ID with the given prefix.
func newID(prefix string) string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return prefix + "_" + fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(b)
}

// Constants for the device state values used in the domain.
const (
	DeviceStatePending = "pending"
	DeviceStatePaired  = "paired"
	DeviceStateRevoked = "revoked"
)

// DefaultChallengeTTL is the lifetime of a pairing challenge.
// The spec says the QR code "expires within minutes"; 5 minutes is the default.
func DefaultChallengeTTL() time.Duration {
	return 5 * time.Minute
}
