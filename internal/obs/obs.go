// Package obs provides structured observability for CAR: request-scoped
// correlation IDs (no secrets), counters for sessions/approvals/replay/API
// errors/storage, and a diagnostic view distinguishing liveness from
// readiness.
package obs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// CorrelationKey is the context key for a request's correlation ID.
type CorrelationKey struct{}

// WithCorrelation returns a context carrying the correlation ID.
func WithCorrelation(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, CorrelationKey{}, id)
}

// CorrelationFrom returns the correlation ID from a context, or "" if absent.
func CorrelationFrom(ctx context.Context) string {
	if v, ok := ctx.Value(CorrelationKey{}).(string); ok {
		return v
	}
	return ""
}

// NewCorrelationID returns a fresh correlation ID.
func NewCorrelationID() string {
	// Monotonic-ish; good enough for correlation (not a security token).
	return fmt.Sprintf("corr_%d_%d", time.Now().UnixNano(), atomic.AddUint64(&corrSeq, 1))
}

var corrSeq uint64

// Metrics holds atomic counters for the documented metric surfaces.
// All fields are atomic so reads/writes are race-free without external locking.
type Metrics struct {
	SessionsCreated    uint64
	SessionsActive     uint64
	SessionsCompleted  uint64
	SessionsFailed     uint64
	ApprovalsRequested uint64
	ApprovalsApproved  uint64
	ApprovalsDenied    uint64
	ApprovalsExpired   uint64
	EventsReplayed     uint64
	ReplayResyncs      uint64
	APIErrors          uint64
	DBErrors           uint64
}

// Recorder is the singleton metrics recorder.
type Recorder struct {
	m Metrics
}

// NewRecorder creates a new recorder.
func NewRecorder() *Recorder { return &Recorder{} }

// IncSessionsCreated increments the sessions.created counter.
func (r *Recorder) IncSessionsCreated()        { atomic.AddUint64(&r.m.SessionsCreated, 1) }
func (r *Recorder) IncSessionsActive()         { atomic.AddUint64(&r.m.SessionsActive, 1) }
func (r *Recorder) DecSessionsActive()         { atomic.AddUint64(&r.m.SessionsActive, ^uint64(0)) }
func (r *Recorder) IncSessionsCompleted()      { atomic.AddUint64(&r.m.SessionsCompleted, 1) }
func (r *Recorder) IncSessionsFailed()         { atomic.AddUint64(&r.m.SessionsFailed, 1) }
func (r *Recorder) IncApprovalRequested()      { atomic.AddUint64(&r.m.ApprovalsRequested, 1) }
func (r *Recorder) IncApprovalApproved()       { atomic.AddUint64(&r.m.ApprovalsApproved, 1) }
func (r *Recorder) IncApprovalDenied()         { atomic.AddUint64(&r.m.ApprovalsDenied, 1) }
func (r *Recorder) IncApprovalExpired()        { atomic.AddUint64(&r.m.ApprovalsExpired, 1) }
func (r *Recorder) AddEventsReplayed(n uint64) { atomic.AddUint64(&r.m.EventsReplayed, n) }
func (r *Recorder) IncReplayResync()           { atomic.AddUint64(&r.m.ReplayResyncs, 1) }
func (r *Recorder) IncAPIError()               { atomic.AddUint64(&r.m.APIErrors, 1) }
func (r *Recorder) IncDBError()                { atomic.AddUint64(&r.m.DBErrors, 1) }

// Snapshot returns a point-in-time copy of the metrics (for exposition).
func (r *Recorder) Snapshot() Metrics {
	return Metrics{
		SessionsCreated:    atomic.LoadUint64(&r.m.SessionsCreated),
		SessionsActive:     atomic.LoadUint64(&r.m.SessionsActive),
		SessionsCompleted:  atomic.LoadUint64(&r.m.SessionsCompleted),
		SessionsFailed:     atomic.LoadUint64(&r.m.SessionsFailed),
		ApprovalsRequested: atomic.LoadUint64(&r.m.ApprovalsRequested),
		ApprovalsApproved:  atomic.LoadUint64(&r.m.ApprovalsApproved),
		ApprovalsDenied:    atomic.LoadUint64(&r.m.ApprovalsDenied),
		ApprovalsExpired:   atomic.LoadUint64(&r.m.ApprovalsExpired),
		EventsReplayed:     atomic.LoadUint64(&r.m.EventsReplayed),
		ReplayResyncs:      atomic.LoadUint64(&r.m.ReplayResyncs),
		APIErrors:          atomic.LoadUint64(&r.m.APIErrors),
		DBErrors:           atomic.LoadUint64(&r.m.DBErrors),
	}
}

// HealthStatus distinguishes liveness from readiness.
// Liveness: the process is up and can serve requests.
// Readiness: the process can actually do useful work (storage healthy, etc).
type HealthStatus struct {
	Live    bool      `json:"live"`
	Ready   bool      `json:"ready"`
	Time    time.Time `json:"time"`
	Reason  string    `json:"reason,omitempty"`
	Metrics Metrics   `json:"metrics"`
}

// ReadinessProbe is a function that reports whether the server is ready
// (storage reachable, etc.). It MUST NOT leak secrets.
type ReadinessProbe func(ctx context.Context) error

// DiagnosticView composes liveness (always true while the process runs) with
// readiness (storage reachable) and the metrics snapshot.
type DiagnosticView struct {
	recorder *Recorder
	probe    ReadinessProbe
}

// NewDiagnosticView creates a diagnostic view with a readiness probe.
func NewDiagnosticView(r *Recorder, probe ReadinessProbe) *DiagnosticView {
	return &DiagnosticView{recorder: r, probe: probe}
}

// Status returns the current health status.
func (d *DiagnosticView) Status(ctx context.Context) HealthStatus {
	status := HealthStatus{
		Live:    true,
		Ready:   true,
		Time:    time.Now().UTC(),
		Metrics: d.recorder.Snapshot(),
	}
	if d.probe != nil {
		if err := d.probe(ctx); err != nil {
			status.Ready = false
			status.Reason = "storage unavailable" // do NOT leak the error text
		}
	}
	return status
}

// HTTPHandler returns an http.Handler for /health and /ready with the
// liveness/readiness distinction. `/health` returns 200 if live; `/ready`
// returns 200 only if ready (503 otherwise).
func (d *DiagnosticView) HTTPHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := d.Status(r.Context())
		w.Header().Set("Content-Type", "application/json")

		path := r.URL.Path
		switch path {
		case "/health":
			// Liveness: process is up. Report ready flag but always 200 live.
			w.WriteHeader(http.StatusOK)
		case "/ready":
			if !status.Ready {
				w.WriteHeader(http.StatusServiceUnavailable)
			} else {
				w.WriteHeader(http.StatusOK)
			}
		default:
			w.WriteHeader(http.StatusOK)
		}
		json.NewEncoder(w).Encode(status)
	}
}

// CorrelationMiddleware injects a correlation ID into each request's context
// and response header, so logs can be correlated across services. It does NOT
// log request bodies or headers (which may contain secrets).
func CorrelationMiddleware(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cid := r.Header.Get("X-Correlation-ID")
		if cid == "" {
			cid = NewCorrelationID()
		}
		w.Header().Set("X-Correlation-ID", cid)
		next.ServeHTTP(w, r.WithContext(WithCorrelation(r.Context(), cid)))
	}
}

// LogFields is a tiny helper that returns the correlation ID as slog-style
// fields for log lines; returns nil if no correlation is set.
func LogFields(ctx context.Context) []any {
	if cid := CorrelationFrom(ctx); cid != "" {
		return []any{"correlation_id", cid}
	}
	return nil
}

// snapshotMutex is retained for potential future histogram use.
var snapshotMutex sync.RWMutex
