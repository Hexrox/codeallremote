package obs

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecorder_Counters(t *testing.T) {
	r := NewRecorder()
	r.IncSessionsCreated()
	r.IncSessionsActive()
	r.IncSessionsActive()
	r.DecSessionsActive()
	r.IncApprovalRequested()
	r.IncApprovalApproved()
	r.IncApprovalDenied()
	r.IncApprovalExpired()
	r.IncReplayResync()
	r.IncAPIError()
	r.IncDBError()
	r.AddEventsReplayed(42)

	s := r.Snapshot()
	if s.SessionsCreated != 1 {
		t.Errorf("created: %d", s.SessionsCreated)
	}
	if s.SessionsActive != 1 {
		t.Errorf("active: %d (2 inc, 1 dec)", s.SessionsActive)
	}
	if s.ApprovalsApproved != 1 || s.ApprovalsDenied != 1 || s.ApprovalsExpired != 1 {
		t.Errorf("approval counts: %+v", s)
	}
	if s.EventsReplayed != 42 {
		t.Errorf("replayed: %d", s.EventsReplayed)
	}
	if s.APIErrors != 1 || s.DBErrors != 1 || s.ReplayResyncs != 1 {
		t.Errorf("error/resync: %+v", s)
	}
}

func TestNewCorrelationID_Unique(t *testing.T) {
	a := NewCorrelationID()
	b := NewCorrelationID()
	if a == b {
		t.Error("expected unique correlation IDs")
	}
	if !strings.HasPrefix(a, "corr_") {
		t.Errorf("expected corr_ prefix, got %s", a)
	}
}

func TestWithCorrelation(t *testing.T) {
	ctx := WithCorrelation(context.Background(), "cid-123")
	if CorrelationFrom(ctx) != "cid-123" {
		t.Error("correlation not retrieved")
	}
	if CorrelationFrom(context.Background()) != "" {
		t.Error("expected empty correlation from bare context")
	}
}

func TestDiagnosticView_LiveAndReady(t *testing.T) {
	r := NewRecorder()
	dv := NewDiagnosticView(r, func(ctx context.Context) error { return nil })

	status := dv.Status(context.Background())
	if !status.Live || !status.Ready {
		t.Error("expected live and ready")
	}
}

func TestDiagnosticView_NotReady(t *testing.T) {
	r := NewRecorder()
	dv := NewDiagnosticView(r, func(ctx context.Context) error { return errors.New("db down") })

	status := dv.Status(context.Background())
	if !status.Live {
		t.Error("process should be live")
	}
	if status.Ready {
		t.Error("expected not ready")
	}
	// The reason must not leak the underlying error text.
	if strings.Contains(status.Reason, "db down") {
		t.Errorf("reason leaked error text: %s", status.Reason)
	}
}

func TestHTTPHandler_HealthAlwaysOK(t *testing.T) {
	r := NewRecorder()
	dv := NewDiagnosticView(r, func(ctx context.Context) error { return errors.New("db down") })
	srv := httptest.NewServer(dv.HTTPHandler())
	defer srv.Close()

	resp, _ := http.Get(srv.URL + "/health")
	if resp.StatusCode != http.StatusOK {
		t.Errorf("health should be 200 even when not ready, got %d", resp.StatusCode)
	}
	var status HealthStatus
	json.NewDecoder(resp.Body).Decode(&status)
	resp.Body.Close()
	if status.Ready {
		t.Error("expected ready=false in body")
	}
}

func TestHTTPHandler_Ready503WhenNotReady(t *testing.T) {
	r := NewRecorder()
	dv := NewDiagnosticView(r, func(ctx context.Context) error { return errors.New("db down") })
	srv := httptest.NewServer(dv.HTTPHandler())
	defer srv.Close()

	resp, _ := http.Get(srv.URL + "/ready")
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("ready should be 503 when storage down, got %d", resp.StatusCode)
	}
}

func TestHTTPHandler_Ready200WhenHealthy(t *testing.T) {
	r := NewRecorder()
	dv := NewDiagnosticView(r, func(ctx context.Context) error { return nil })
	srv := httptest.NewServer(dv.HTTPHandler())
	defer srv.Close()

	resp, _ := http.Get(srv.URL + "/ready")
	if resp.StatusCode != http.StatusOK {
		t.Errorf("ready should be 200, got %d", resp.StatusCode)
	}
}

func TestCorrelationMiddleware_InjectsID(t *testing.T) {
	var seen string
	h := CorrelationMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = CorrelationFrom(r.Context())
	}))
	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, _ := http.Get(srv.URL)
	resp.Body.Close()
	if seen == "" {
		t.Error("expected correlation ID injected")
	}
	if resp.Header.Get("X-Correlation-ID") == "" {
		t.Error("expected X-Correlation-ID response header")
	}
}

func TestCorrelationMiddleware_PreservesClientID(t *testing.T) {
	h := CorrelationMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv := httptest.NewServer(h)
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL, nil)
	req.Header.Set("X-Correlation-ID", "client-cid-123")
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.Header.Get("X-Correlation-ID") != "client-cid-123" {
		t.Errorf("expected client cid preserved, got %s", resp.Header.Get("X-Correlation-ID"))
	}
}

func TestLogFields_NoCorrelation(t *testing.T) {
	if fields := LogFields(context.Background()); fields != nil {
		t.Error("expected nil without correlation")
	}
}

func TestMetricsSnapshot_IsCopy(t *testing.T) {
	r := NewRecorder()
	r.IncSessionsCreated()
	s1 := r.Snapshot()
	r.IncSessionsCreated()
	s2 := r.Snapshot()
	if s1.SessionsCreated != 1 {
		t.Error("snapshot mutated")
	}
	if s2.SessionsCreated != 2 {
		t.Error("second snapshot wrong")
	}
}
