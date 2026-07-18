package session

import (
	"testing"
	"time"

	"github.com/code-all-remote/car/internal/domain"
)

func TestStateMachine_ValidTransitions(t *testing.T) {
	sm := NewStateMachine()

	tests := []struct {
		from     string
		to       string
		expected bool
	}{
		// Normal flow
		{domain.SessionStateCreated, domain.SessionStateStarting, true},
		{domain.SessionStateStarting, domain.SessionStateActive, true},
		{domain.SessionStateActive, domain.SessionStateCompleted, true},

		// Approval flow
		{domain.SessionStateActive, domain.SessionStateWaitingApprov, true},
		{domain.SessionStateWaitingApprov, domain.SessionStateActive, true},

		// Interrupt flow
		{domain.SessionStateActive, domain.SessionStateInterrupted, true},
		{domain.SessionStateInterrupted, domain.SessionStateResumable, true},
		{domain.SessionStateResumable, domain.SessionStateStarting, true},

		// Failure paths
		{domain.SessionStateCreated, domain.SessionStateFailed, true},
		{domain.SessionStateStarting, domain.SessionStateFailed, true},
		{domain.SessionStateActive, domain.SessionStateFailed, true},
		{domain.SessionStateWaitingApprov, domain.SessionStateFailed, true},
		{domain.SessionStateWaitingApprov, domain.SessionStateInterrupted, true},

		// Invalid transitions
		{domain.SessionStateCompleted, domain.SessionStateActive, false},
		{domain.SessionStateFailed, domain.SessionStateActive, false},
		{domain.SessionStateCreated, domain.SessionStateActive, false},
		{domain.SessionStateCreated, domain.SessionStateCompleted, false},
		{domain.SessionStateResumable, domain.SessionStateCompleted, false},
	}

	for _, tt := range tests {
		t.Run(tt.from+"_to_"+tt.to, func(t *testing.T) {
			result := sm.CanTransition(tt.from, tt.to)
			if result != tt.expected {
				t.Errorf("CanTransition(%s, %s) = %v, expected %v", tt.from, tt.to, result, tt.expected)
			}
		})
	}
}

func TestStateMachine_Transition(t *testing.T) {
	sm := NewStateMachine()

	// Valid transition
	to, err := sm.Transition(domain.SessionStateCreated, domain.SessionStateStarting)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if to != domain.SessionStateStarting {
		t.Errorf("expected %s, got %s", domain.SessionStateStarting, to)
	}

	// Invalid transition
	_, err = sm.Transition(domain.SessionStateFailed, domain.SessionStateActive)
	if err == nil {
		t.Error("expected error for invalid transition, got nil")
	}
}

func TestIdempotencyStore_AddAndGet(t *testing.T) {
	store := NewIdempotencyStore(1 * time.Hour)
	defer store.Close()

	key := "test-key-12345"
	response := []byte(`{"status":"ok"}`)
	status := 200

	err := store.Add(key, response, status)
	if err != nil {
		t.Fatalf("failed to add: %v", err)
	}

	// Get should return stored values
	resp, st, ok := store.Get(key)
	if !ok {
		t.Fatal("expected key to exist")
	}
	if string(resp) != string(response) {
		t.Errorf("expected response %s, got %s", response, resp)
	}
	if st != status {
		t.Errorf("expected status %d, got %d", status, st)
	}
}

func TestIdempotencyStore_Exists(t *testing.T) {
	store := NewIdempotencyStore(1 * time.Hour)
	defer store.Close()

	key := "test-key-exists"
	store.Add(key, []byte(`{}`), 200)

	if !store.Exists(key) {
		t.Error("expected key to exist")
	}

	if store.Exists("nonexistent") {
		t.Error("expected nonexistent key to not exist")
	}
}

func TestIdempotencyStore_Delete(t *testing.T) {
	store := NewIdempotencyStore(1 * time.Hour)
	defer store.Close()

	key := "test-key-delete"
	store.Add(key, []byte(`{}`), 200)

	store.Delete(key)

	if store.Exists(key) {
		t.Error("expected key to be deleted")
	}
}

func TestIdempotencyStore_Expiry(t *testing.T) {
	// Use very short expiry for testing
	store := NewIdempotencyStore(100 * time.Millisecond)
	defer store.Close()

	key := "test-key-expiry"
	store.Add(key, []byte(`{}`), 200)

	// Should exist initially
	if !store.Exists(key) {
		t.Error("expected key to exist initially")
	}

	// Wait for expiry
	time.Sleep(150 * time.Millisecond)

	// Should be expired
	if store.Exists(key) {
		t.Error("expected key to be expired")
	}

	_, _, ok := store.Get(key)
	if ok {
		t.Error("expected Get to return false for expired key")
	}
}

func TestIdempotencyStore_Cleanup(t *testing.T) {
	// Use short expiry for testing
	store := NewIdempotencyStore(50 * time.Millisecond)
	defer store.Close()

	// Add multiple keys
	for i := 0; i < 10; i++ {
		key := string(rune('a' + i))
		store.Add(key, []byte(`{}`), 200)
	}

	// Wait for expiry plus cleanup interval
	time.Sleep(100 * time.Millisecond)

	// Manually trigger cleanup for testing
	store.cleanup()

	// Manually check map is empty (using reflection or by checking all keys)
	store.mu.RLock()
	count := len(store.keys)
	store.mu.RUnlock()

	if count != 0 {
		t.Errorf("expected 0 keys after cleanup, got %d", count)
	}
}

func TestRequestValidator_ValidateKey(t *testing.T) {
	store := NewIdempotencyStore(1 * time.Hour)
	defer store.Close()
	validator := NewRequestValidator(store)

	tests := []struct {
		key       string
		shouldErr bool
	}{
		{"valid-key-123", false},
		{"min-length", false}, // exactly 8 chars
		{"", true},            // empty
		{"short", true},       // less than 8
		{"this-is-a-very-long-key-that-exceeds-the-maximum-allowed-length-of-two-hundred-characters-this-is-a-very-long-key-that-exceeds-the-maximum-allowed-length-of-two-hundred-characters-this-is-a-very-long-key-that-exceeds", true}, // > 200 chars
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			err := validator.ValidateKey(tt.key)
			if tt.shouldErr && err == nil {
				t.Errorf("expected error for key '%s', got nil", tt.key)
			}
			if !tt.shouldErr && err != nil {
				t.Errorf("unexpected error for key '%s': %v", tt.key, err)
			}
		})
	}
}

func TestRequestValidator_CheckIdempotency(t *testing.T) {
	store := NewIdempotencyStore(1 * time.Hour)
	defer store.Close()
	validator := NewRequestValidator(store)

	key := "test-idempotency-key"
	response := []byte(`{"id":"123"}`)
	status := 201

	// First request - not processed yet
	_, _, ok := validator.CheckIdempotency(key)
	if ok {
		t.Error("expected first request to not be processed")
	}

	// Store response
	store.Add(key, response, status)

	// Retry - should return stored response
	resp, st, ok := validator.CheckIdempotency(key)
	if !ok {
		t.Error("expected retry to be processed")
	}
	if string(resp) != string(response) {
		t.Errorf("expected response %s, got %s", response, resp)
	}
	if st != status {
		t.Errorf("expected status %d, got %d", status, st)
	}
}

func TestStateMachine_Documentation(t *testing.T) {
	sm := NewStateMachine()
	transitions := sm.GetAllTransitions()

	// Verify all expected states have transitions defined
	expectedStates := []string{
		domain.SessionStateCreated,
		domain.SessionStateStarting,
		domain.SessionStateActive,
		domain.SessionStateWaitingApprov,
		domain.SessionStateInterrupted,
		domain.SessionStateCompleted,
		domain.SessionStateFailed,
		domain.SessionStateResumable,
		domain.SessionStateRecovering,
	}

	for _, state := range expectedStates {
		if _, ok := transitions[state]; !ok {
			t.Errorf("missing transitions for state %s", state)
		}
	}
}

// Concurrent access test for StateMachine
func TestStateMachine_ConcurrentAccess(t *testing.T) {
	sm := NewStateMachine()
	done := make(chan bool, 100)

	// Run multiple goroutines checking transitions concurrently
	for i := 0; i < 100; i++ {
		go func() {
			sm.CanTransition(domain.SessionStateCreated, domain.SessionStateStarting)
			sm.CanTransition(domain.SessionStateActive, domain.SessionStateCompleted)
			done <- true
		}()
	}

	for i := 0; i < 100; i++ {
		<-done
	}
	// If there's a race condition, go test -race will catch it
}

// Concurrent access test for IdempotencyStore
func TestIdempotencyStore_ConcurrentAccess(t *testing.T) {
	store := NewIdempotencyStore(1 * time.Hour)
	defer store.Close()
	done := make(chan bool, 100)

	for i := 0; i < 100; i++ {
		go func(id int) {
			key := string(rune('a' + (id % 26)))
			store.Add(key, []byte(`{}`), 200)
			store.Exists(key)
			store.Get(key)
			done <- true
		}(i)
	}

	for i := 0; i < 100; i++ {
		<-done
	}
}
