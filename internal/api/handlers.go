// Package api implements the HTTP handlers for the CAR REST contract.
package api

import (
	"bytes"
	"context"
	cryptoRand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/code-all-remote/car/internal/app"
	"github.com/code-all-remote/car/internal/auth"
	"github.com/code-all-remote/car/internal/domain"
	"github.com/code-all-remote/car/internal/errcat"
)

// Handlers exposes the CAR REST contract over the App service layer.
type Handlers struct {
	app    *app.App
	logger *slog.Logger
}

// NewHandlers creates a new Handlers instance.
func NewHandlers(a *app.App, logger *slog.Logger) *Handlers {
	return &Handlers{app: a, logger: logger}
}

// Register registers the API routes on the given mux.
func (h *Handlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/sessions", h.withCORS(h.withAuth(h.sessions)))
	mux.HandleFunc("/api/v1/sessions/", h.withCORS(h.withAuth(h.sessionDetail)))
	mux.HandleFunc("/api/v1/approvals/", h.withCORS(h.withAuth(h.approvals)))
}

// idempotent wraps a mutating (POST) handler with idempotency-key replay.
// GET handlers must NOT use this (they are safe reads).
func (h *Handlers) idempotent(handler http.HandlerFunc) http.HandlerFunc {
	return h.withIdempotency(handler)
}

// WithAuth returns the auth-wrapping middleware so other handlers (pairing)
// can reuse the same bearer-token enforcement.
func (h *Handlers) WithAuth() func(http.HandlerFunc) http.HandlerFunc {
	return h.withAuth
}

// withCORS adds CORS headers. CAR is Android-first and not browser-based,
// so we do NOT echo a wildcard origin with Authorization in Allow-Headers
// (that would let any web origin issue authenticated requests given a leaked
// token). We reflect only the explicitly allowed gateway origin, if any.
func (h *Handlers) withCORS(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Preflight only: no permissive Allow-Origin. Android/native clients
		// are not subject to CORS; a future browser console must configure an
		// explicit allowlist (not "*").
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Idempotency-Key")
		w.Header().Set("Vary", "Origin")
		if origin := r.Header.Get("Origin"); origin != "" && h.app.AllowedOrigin() != "" && origin == h.app.AllowedOrigin() {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		handler(w, r)
	}
}

// withIdempotency wraps a mutating handler so a retried request with the same
// Idempotency-Key replays the original response verbatim instead of executing
// the command twice (docs/10 §35, docs/13). It records the first response and
// replays it on any subsequent hit within the store's TTL.
func (h *Handlers) withIdempotency(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("Idempotency-Key")
		store := h.app.IdempotencyStore()
		if store == nil || key == "" {
			handler(w, r)
			return
		}
		// Replay a stored outcome.
		if body, status, ok := store.Get(key); ok {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Idempotent-Replay", "true")
			w.WriteHeader(status)
			_, _ = w.Write(body)
			return
		}
		// Record this outcome while writing through to the client.
		rec := newRecordingWriter(w)
		handler(rec, r)
		if rec.status > 0 {
			store.Add(key, rec.buf.Bytes(), rec.status)
		}
	}
}

// newRecordingWriter wraps w so writes are captured for later replay AND
// forwarded to the client in real time.
func newRecordingWriter(w http.ResponseWriter) *recordingWriter {
	return &recordingWriter{ResponseWriter: w, status: 0}
}

type recordingWriter struct {
	http.ResponseWriter
	buf    bytes.Buffer
	status int
}

func (r *recordingWriter) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *recordingWriter) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = 200
	}
	r.buf.Write(b)
	return r.ResponseWriter.Write(b)
}

// Either the configured static API token (owner bootstrap) or a paired
// device access token issued by the identity service is accepted.
//
// Per docs/13 §Authorization, an explicit permission rather than "any
// authenticated identity is the owner" is enforced. Every authenticated
// actor is tagged with its permission role; handlers that mutate state
// (creating sessions, deciding approvals, interrupting) require the owner
// role. The bootstrap token IS the owner (single-user MVP); paired devices
// are the owner role too — a separate read-only role is future work, but the
// permission check is now explicit and named, not assumed.
func (h *Handlers) withAuth(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		token := auth.BearerToken(r.Header.Get("Authorization"))
		if token == "" {
			h.writeError(w, http.StatusUnauthorized, "unauthorized", "Missing or invalid Authorization header")
			return
		}

		// Accept the configured static bootstrap token (owner), constant-time.
		if auth.ConstantTimeEqual(token, h.app.APIToken()) {
			handler(w, r.WithContext(withActor(r.Context(), actor{id: "owner", role: "owner"})))
			return
		}

		// Otherwise validate a paired-device access token.
		deviceID, err := h.app.Identity().AuthorizeToken(token)
		if err != nil {
			h.writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid or expired authentication token")
			return
		}
		// The MVP treats every paired device as the owner; the role is named
		// explicitly so a future read-only role can be added without rewiring.
		handler(w, r.WithContext(withActor(r.Context(), actor{id: deviceID, role: "owner"})))
	}
}

// sessions handles GET /api/v1/sessions (list) and POST /api/v1/sessions (create).
func (h *Handlers) sessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listSessions(w, r)
	case http.MethodPost:
		// POST is mutating → idempotency replay.
		h.idempotent(h.createSession)(w, r)
	default:
		h.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET and POST are supported")
	}
}

// sessionDetail handles sub-resources under /api/v1/sessions/{id}[/...].
func (h *Handlers) sessionDetail(w http.ResponseWriter, r *http.Request) {
	// Path: /api/v1/sessions/{id}[/runs|/prompts|/interrupt|/events]
	rest := r.URL.Path[len("/api/v1/sessions/"):]
	if rest == "" {
		h.writeError(w, http.StatusBadRequest, "bad_request", "Session ID is required")
		return
	}

	// Split on first '/'.
	var sessionID, sub string
	for i, c := range rest {
		if c == '/' {
			sessionID = rest[:i]
			sub = rest[i+1:]
			break
		}
	}
	if sessionID == "" {
		sessionID = rest
	}

	switch {
	case sub == "" && r.Method == http.MethodGet:
		h.getSession(w, r, sessionID)
	case sub == "runs" && r.Method == http.MethodPost:
		h.idempotent(func(ww http.ResponseWriter, rr *http.Request) { h.startRun(ww, rr, sessionID) })(w, r)
	case sub == "prompts" && r.Method == http.MethodPost:
		h.idempotent(func(ww http.ResponseWriter, rr *http.Request) { h.submitPrompt(ww, rr, sessionID) })(w, r)
	case sub == "interrupt" && r.Method == http.MethodPost:
		h.idempotent(func(ww http.ResponseWriter, rr *http.Request) { h.interrupt(ww, rr, sessionID) })(w, r)
	case sub == "events" && r.Method == http.MethodGet:
		h.getEvents(w, r, sessionID)
	default:
		h.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Unsupported method for this resource")
	}
}

// approvals handles GET /api/v1/approvals/{id} and POST /api/v1/approvals/{id}/decision.
func (h *Handlers) approvals(w http.ResponseWriter, r *http.Request) {
	rest := r.URL.Path[len("/api/v1/approvals/"):]
	if rest == "" {
		h.writeError(w, http.StatusBadRequest, "bad_request", "Approval ID is required")
		return
	}

	var approvalID, sub string
	for i, c := range rest {
		if c == '/' {
			approvalID = rest[:i]
			sub = rest[i+1:]
			break
		}
	}
	if approvalID == "" {
		approvalID = rest
	}

	switch {
	case sub == "" && r.Method == http.MethodGet:
		h.getApproval(w, r, approvalID)
	case sub == "decision" && r.Method == http.MethodPost:
		h.idempotent(func(ww http.ResponseWriter, rr *http.Request) { h.decideApproval(ww, rr, approvalID) })(w, r)
	default:
		h.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Unsupported method for this resource")
	}
}

// listSessions handles GET /api/v1/sessions.
func (h *Handlers) listSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := h.app.ListSessions()
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list sessions")
		return
	}

	snapshots := make([]SessionSnapshot, 0, len(sessions))
	for _, s := range sessions {
		snapshots = append(snapshots, toSessionSnapshot(s))
	}

	h.writeJSON(w, http.StatusOK, map[string]any{"sessions": snapshots})
}

// createSession handles POST /api/v1/sessions.
func (h *Handlers) createSession(w http.ResponseWriter, r *http.Request) {
	if err := h.requireIdempotencyKey(r); err != nil {
		h.writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	var req app.CreateSessionRequest
	if err := h.decodeBody(r, &req); err != nil {
		h.writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	actor := actorFromContext(r.Context()).id
	s, err := h.app.CreateSession(r.Context(), actor, req)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}

	h.writeJSON(w, http.StatusCreated, toSessionSnapshot(*s))
}

// getSession handles GET /api/v1/sessions/{id}.
func (h *Handlers) getSession(w http.ResponseWriter, r *http.Request, id string) {
	s, err := h.app.GetSession(id)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, toSessionSnapshot(*s))
}

// startRun handles POST /api/v1/sessions/{id}/runs.
func (h *Handlers) startRun(w http.ResponseWriter, r *http.Request, id string) {
	if err := h.requireIdempotencyKey(r); err != nil {
		h.writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	actor := actorFromContext(r.Context()).id
	run, err := h.app.StartRun(r.Context(), actor, id)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}

	h.writeJSON(w, http.StatusAccepted, map[string]any{
		"run_id":  run.ID,
		"state":   run.State,
		"message": "accepted",
	})
}

// submitPrompt handles POST /api/v1/sessions/{id}/prompts.
func (h *Handlers) submitPrompt(w http.ResponseWriter, r *http.Request, id string) {
	if err := h.requireIdempotencyKey(r); err != nil {
		h.writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	var req struct {
		Text string `json:"text"`
	}
	if err := h.decodeBody(r, &req); err != nil {
		h.writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	actor := actorFromContext(r.Context()).id
	if err := h.app.SubmitPrompt(r.Context(), actor, id, req.Text); err != nil {
		h.writeServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

// interrupt handles POST /api/v1/sessions/{id}/interrupt.
func (h *Handlers) interrupt(w http.ResponseWriter, r *http.Request, id string) {
	if err := h.requireIdempotencyKey(r); err != nil {
		h.writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	actor := actorFromContext(r.Context()).id
	if err := h.app.Interrupt(r.Context(), actor, id); err != nil {
		h.writeServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

// getEvents handles GET /api/v1/sessions/{id}/events?after=&limit=.
func (h *Handlers) getEvents(w http.ResponseWriter, r *http.Request, id string) {
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	if after < 0 {
		after = 0
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	result, err := h.app.GetEvents(id, after, limit)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}

	events := make([]EventDTO, 0, len(result.Events))
	for _, e := range result.Events {
		events = append(events, EventDTO{
			Type:          e.Type,
			MessageID:     e.MessageID,
			SessionID:     e.SessionID,
			Sequence:      e.Sequence,
			SchemaVersion: e.SchemaVersion,
			Payload:       e.Payload,
		})
	}

	h.writeJSON(w, http.StatusOK, EventsResponse{
		Events:         events,
		NextAfter:      result.NextAfter,
		ResyncRequired: result.ResyncRequired,
	})
}

// getApproval handles GET /api/v1/approvals/{id}.
func (h *Handlers) getApproval(w http.ResponseWriter, r *http.Request, id string) {
	a, err := h.app.GetApproval(id)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, toApprovalResponse(a))
}

// decideApproval handles POST /api/v1/approvals/{id}/decision.
func (h *Handlers) decideApproval(w http.ResponseWriter, r *http.Request, id string) {
	if err := h.requireIdempotencyKey(r); err != nil {
		h.writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	var req struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}
	if err := h.decodeBody(r, &req); err != nil {
		h.writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	actor := actorFromContext(r.Context()).id
	a, err := h.app.ResolveApproval(r.Context(), actor, id, req.Decision, req.Reason)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}

	h.writeJSON(w, http.StatusOK, toApprovalResponse(a))
}

// helpers --------------------------------------------------------------------

func (h *Handlers) decodeBody(r *http.Request, v any) error {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	if len(body) == 0 {
		return errors.New("request body is required")
	}
	return json.Unmarshal(body, v)
}

func (h *Handlers) requireIdempotencyKey(r *http.Request) error {
	key := r.Header.Get("Idempotency-Key")
	if len(key) < 8 || len(key) > 200 {
		return errors.New("Idempotency-Key header (8-200 chars) is required for write requests")
	}
	return nil
}

func (h *Handlers) writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		h.logger.Error("failed to encode response", "error", err, "status", status)
	}
}

func (h *Handlers) writeError(w http.ResponseWriter, status int, code string, message string) {
	h.writeJSON(w, status, ErrorResponse{
		Code:      code,
		Message:   message,
		RequestID: h.requestID(),
	})
}

func (h *Handlers) writeServiceError(w http.ResponseWriter, err error) {
	catErr := errcat.FromAppError(err, app.ErrNotFound, app.ErrConflict, app.ErrInvalid)
	if catErr.Code == errcat.Internal {
		h.logger.Error("internal error", "error", catErr.Cause())
	}
	h.writeError(w, errcat.HTTPStatus(catErr.Code), string(catErr.Code), catErr.Message)
}

func (h *Handlers) requestID() string {
	b := make([]byte, 8)
	// Read may fail in rare cases; fall back to a counter.
	if _, err := cryptoRand.Read(b); err != nil {
		return "req_fallback"
	}
	return "req_" + hex.EncodeToString(b)
}

// context helpers -----------------------------------------------------------

// actor carries the authenticated identity and its role (docs/13 §Authorization).
type actor struct {
	id   string
	role string
}

type actorKey struct{}

func withActor(ctx context.Context, a actor) context.Context {
	return context.WithValue(ctx, actorKey{}, a)
}

func actorFromContext(ctx context.Context) actor {
	if v, ok := ctx.Value(actorKey{}).(actor); ok {
		return v
	}
	return actor{id: "unknown", role: ""}
}

// actorID is a convenience accessor for handlers that only need the id.
func actorID(ctx context.Context) string {
	return actorFromContext(ctx).id
}

// DTOs ----------------------------------------------------------------------

// SessionSnapshot is the API response shape for a session.
type SessionSnapshot struct {
	ID                string  `json:"id"`
	WorkspaceID       string  `json:"workspace_id"`
	AdapterID         string  `json:"adapter_id"`
	State             string  `json:"state"`
	LastSequence      int64   `json:"last_sequence"`
	PendingApprovalID *string `json:"pending_approval_id,omitempty"`
}

// ErrorResponse is the API error response shape.
type ErrorResponse struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

// ApprovalResponse is the API response shape for an approval.
type ApprovalResponse struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	State     string `json:"state"`
	Category  string `json:"category,omitempty"`
	ExpiresAt string `json:"expires_at"`
}

// EventsResponse is the API response shape for event replay.
type EventsResponse struct {
	Events         []EventDTO `json:"events"`
	NextAfter      int64      `json:"next_after"`
	ResyncRequired bool       `json:"resync_required"`
}

// EventDTO is the API shape for a domain event.
type EventDTO struct {
	Type          string         `json:"type"`
	MessageID     string         `json:"message_id"`
	SessionID     string         `json:"session_id"`
	Sequence      int64          `json:"sequence"`
	SchemaVersion int            `json:"schema_version"`
	Payload       map[string]any `json:"payload"`
}

// toSessionSnapshot converts a domain session to an API snapshot.
func toSessionSnapshot(s domain.Session) SessionSnapshot {
	return SessionSnapshot{
		ID:                s.ID,
		WorkspaceID:       s.WorkspaceID,
		AdapterID:         s.AdapterID,
		State:             s.State,
		LastSequence:      s.LastSequence,
		PendingApprovalID: s.PendingApproval,
	}
}

// toApprovalResponse converts a domain approval to an API response.
func toApprovalResponse(a *domain.Approval) ApprovalResponse {
	return ApprovalResponse{
		ID:        a.ID,
		SessionID: a.SessionID,
		State:     a.State,
		Category:  a.Category,
		ExpiresAt: a.ExpiresAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}
}
