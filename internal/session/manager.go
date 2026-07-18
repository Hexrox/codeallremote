// Package session provides session management and state machine logic.
package session

import (
	"fmt"
	"sync"
	"time"

	"github.com/code-all-remote/car/internal/domain"
)

// Valid state transitions for sessions
var validTransitions = map[string][]string{
	domain.SessionStateCreated:       {domain.SessionStateStarting, domain.SessionStateFailed},
	domain.SessionStateStarting:      {domain.SessionStateActive, domain.SessionStateFailed},
	domain.SessionStateActive:        {domain.SessionStateWaitingApprov, domain.SessionStateInterrupted, domain.SessionStateCompleted, domain.SessionStateFailed},
	domain.SessionStateWaitingApprov: {domain.SessionStateActive, domain.SessionStateInterrupted, domain.SessionStateFailed},
	domain.SessionStateInterrupted:   {domain.SessionStateResumable, domain.SessionStateFailed},
	domain.SessionStateCompleted:     {domain.SessionStateResumable},
	domain.SessionStateFailed:        {},
	domain.SessionStateResumable:     {domain.SessionStateStarting},
	domain.SessionStateRecovering:    {domain.SessionStateActive, domain.SessionStateCompleted, domain.SessionStateFailed},
}

// StateMachine handles session state transitions.
type StateMachine struct {
	mu sync.RWMutex
}

// NewStateMachine creates a new state machine.
func NewStateMachine() *StateMachine {
	return &StateMachine{}
}

// CanTransition checks if a transition from one state to another is valid.
func (sm *StateMachine) CanTransition(from, to string) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	validTargets, ok := validTransitions[from]
	if !ok {
		return false
	}

	for _, target := range validTargets {
		if target == to {
			return true
		}
	}

	return false
}

// Transition validates and returns a new state if the transition is valid.
func (sm *StateMachine) Transition(from, to string) (string, error) {
	if !sm.CanTransition(from, to) {
		return "", fmt.Errorf("invalid state transition from %s to %s", from, to)
	}
	return to, nil
}

// GetAllTransitions returns all valid transitions for documentation/testing.
func (sm *StateMachine) GetAllTransitions() map[string][]string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	result := make(map[string][]string)
	for k, v := range validTransitions {
		result[k] = append([]string{}, v...)
	}
	return result
}

// IdempotencyStore provides in-memory storage for idempotency keys.
// In production, this should be backed by persistent storage.
type IdempotencyStore struct {
	mu        sync.RWMutex
	keys      map[string]*idempotencyEntry
	expiry    time.Duration
	cleanupCh chan struct{}
}

type idempotencyEntry struct {
	response  []byte
	status    int
	createdAt time.Time
	expiresAt time.Time
}

// NewIdempotencyStore creates a new idempotency store.
func NewIdempotencyStore(expiry time.Duration) *IdempotencyStore {
	s := &IdempotencyStore{
		keys:      make(map[string]*idempotencyEntry),
		expiry:    expiry,
		cleanupCh: make(chan struct{}),
	}

	// Start cleanup goroutine
	go s.cleanupLoop()

	return s
}

// Add stores a response for an idempotency key.
func (s *IdempotencyStore) Add(key string, response []byte, status int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	s.keys[key] = &idempotencyEntry{
		response:  response,
		status:    status,
		createdAt: now,
		expiresAt: now.Add(s.expiry),
	}

	return nil
}

// Get retrieves a stored response for an idempotency key.
// Returns nil if the key doesn't exist or has expired.
func (s *IdempotencyStore) Get(key string) ([]byte, int, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, ok := s.keys[key]
	if !ok {
		return nil, 0, false
	}

	if time.Now().After(entry.expiresAt) {
		return nil, 0, false
	}

	return entry.response, entry.status, true
}

// Exists checks if a key exists (for idempotency validation).
func (s *IdempotencyStore) Exists(key string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, ok := s.keys[key]
	if !ok {
		return false
	}

	return time.Now().Before(entry.expiresAt)
}

// Delete removes a key from the store.
func (s *IdempotencyStore) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.keys, key)
}

// cleanupLoop periodically removes expired entries.
func (s *IdempotencyStore) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.cleanup()
		case <-s.cleanupCh:
			return
		}
	}
}

// cleanup removes expired entries.
func (s *IdempotencyStore) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for key, entry := range s.keys {
		if now.After(entry.expiresAt) {
			delete(s.keys, key)
		}
	}
}

// Close stops the cleanup goroutine.
func (s *IdempotencyStore) Close() {
	close(s.cleanupCh)
}

// RequestValidator validates and processes API requests.
type RequestValidator struct {
	store *IdempotencyStore
}

// NewRequestValidator creates a new request validator.
func NewRequestValidator(store *IdempotencyStore) *RequestValidator {
	return &RequestValidator{store: store}
}

// CheckIdempotency checks if a request with this idempotency key was already processed.
// Returns (response, status, wasProcessed).
func (v *RequestValidator) CheckIdempotency(key string) ([]byte, int, bool) {
	return v.store.Get(key)
}

// ValidateKey validates an idempotency key format.
func (v *RequestValidator) ValidateKey(key string) error {
	if key == "" {
		return fmt.Errorf("idempotency key cannot be empty")
	}
	if len(key) < 8 {
		return fmt.Errorf("idempotency key must be at least 8 characters")
	}
	if len(key) > 200 {
		return fmt.Errorf("idempotency key must be at most 200 characters")
	}
	return nil
}
