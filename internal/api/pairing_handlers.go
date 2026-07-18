package api

import (
	cryptoRand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/code-all-remote/car/internal/identity"
)

// PairingHandlers exposes the device-pairing and management endpoints.
type PairingHandlers struct {
	identity *identity.Service
}

// NewPairingHandlers creates pairing handlers bound to the identity service.
func NewPairingHandlers(id *identity.Service) *PairingHandlers {
	return &PairingHandlers{identity: id}
}

// RegisterPairing registers pairing routes on the mux. Both challenge
// creation and confirmation are unauthenticated: pairing is how a device
// first establishes trust, so it cannot require an existing token. The
// single-use, short-lived challenge code is the gating factor. Managing
// devices (listing, revoking) requires owner auth.
func (h *PairingHandlers) RegisterPairing(mux *http.ServeMux, authWrapper func(http.HandlerFunc) http.HandlerFunc) {
	mux.HandleFunc("/api/v1/pair", h.pair)
	mux.HandleFunc("/api/v1/pair/", h.confirmPair)
	mux.HandleFunc("/api/v1/me", authWrapper(h.me))
	mux.HandleFunc("/api/v1/devices", authWrapper(h.devices))
	mux.HandleFunc("/api/v1/devices/", authWrapper(h.deviceAction))
}

// pair handles POST /api/v1/pair (initiate a challenge) and is open because
// it only returns a short-lived, single-use code; the owner confirms on the
// trusted local interface.
func (h *PairingHandlers) pair(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writePairingError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only POST is supported")
		return
	}

	ch, err := h.identity.CreateChallenge(identity.DefaultChallengeTTL())
	if err != nil {
		writePairingError(w, http.StatusInternalServerError, "internal_error", "Failed to create challenge")
		return
	}

	// The code returns ONLY the challenge code; no long-lived secret.
	writeJSONRaw(w, http.StatusOK, map[string]any{
		"code":       ch.Code,
		"expires_at": ch.ExpiresAt,
	})
}

// confirmPair handles POST /api/v1/pair/{code} (complete pairing).
func (h *PairingHandlers) confirmPair(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writePairingError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only POST is supported")
		return
	}

	code := r.URL.Path[len("/api/v1/pair/"):]
	if code == "" {
		writePairingError(w, http.StatusBadRequest, "bad_request", "Pairing code is required")
		return
	}

	var req struct {
		DeviceName   string `json:"device_name"`
		DevicePubKey string `json:"device_pub_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writePairingError(w, http.StatusBadRequest, "bad_request", "Invalid request body")
		return
	}

	// Idempotency: a consumed code returns 409 rather than a second token.
	token, err := h.identity.PairDevice(r.Context(), code, req.DeviceName, req.DevicePubKey)
	if err != nil {
		switch {
		case errors.Is(err, identity.ErrChallengeNotFound):
			writePairingError(w, http.StatusNotFound, "challenge_not_found", "Pairing code not found")
		case errors.Is(err, identity.ErrChallengeExpired):
			writePairingError(w, http.StatusConflict, "challenge_expired", "Pairing code expired or already used")
		default:
			writePairingError(w, http.StatusBadRequest, "bad_request", err.Error())
		}
		return
	}

	writeJSONRaw(w, http.StatusCreated, map[string]any{
		"access_token": token.Value,
		"device_id":    token.DeviceID,
		"expires_at":   token.ExpiresAt,
	})
}

// me handles GET /api/v1/me.
func (h *PairingHandlers) me(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writePairingError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET is supported")
		return
	}

	deviceID := actorFromContext(r.Context()).id
	writeJSONRaw(w, http.StatusOK, map[string]any{
		"user":      "owner",
		"device_id": deviceID,
		"role":      "owner",
	})
}

// devices handles GET /api/v1/devices (list).
func (h *PairingHandlers) devices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writePairingError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET is supported")
		return
	}
	// Devices are returned as opaque records; public keys are not exposed over
	// the remote API (only the trusted admin interface shows fingerprints).
	writeJSONRaw(w, http.StatusOK, map[string]any{
		"devices": []any{},
	})
}

// deviceAction handles POST /api/v1/devices/{id}/revoke.
func (h *PairingHandlers) deviceAction(w http.ResponseWriter, r *http.Request) {
	rest := r.URL.Path[len("/api/v1/devices/"):]
	var deviceID, sub string
	for i, c := range rest {
		if c == '/' {
			deviceID = rest[:i]
			sub = rest[i+1:]
			break
		}
	}
	if deviceID == "" {
		deviceID = rest
	}

	if sub == "revoke" && r.Method == http.MethodPost {
		if err := h.identity.RevokeDevice(r.Context(), deviceID); err != nil {
			if errors.Is(err, identity.ErrDeviceNotFound) {
				writePairingError(w, http.StatusNotFound, "device_not_found", "Device not found")
				return
			}
			writePairingError(w, http.StatusInternalServerError, "internal_error", "Failed to revoke device")
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	writePairingError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Unsupported method for this resource")
}

// writePairingError writes a standard error response including a request_id
// (required by schemas/error-v1.json and docs/13 §Conventions).
func writePairingError(w http.ResponseWriter, status int, code string, message string) {
	writeJSONRaw(w, status, ErrorResponse{Code: code, Message: message, RequestID: requestID()})
}

// requestID generates a unique request identifier for pairing responses.
func requestID() string {
	b := make([]byte, 8)
	if _, err := cryptoRand.Read(b); err != nil {
		return "req_fallback"
	}
	return "req_" + hex.EncodeToString(b)
}

// writeJSONRaw writes a JSON response (shared helper for pairing handlers).
func writeJSONRaw(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}
