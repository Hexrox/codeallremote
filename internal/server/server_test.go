package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"log/slog"

	"github.com/code-all-remote/car/internal/config"
)

func testConfig() *config.Config {
	return &config.Config{
		Server: config.ServerConfig{
			Host:            "127.0.0.1",
			Port:            0, // Use any available port
			ReadTimeout:     config.Duration(5 * time.Second),
			WriteTimeout:    config.Duration(5 * time.Second),
			ShutdownTimeout: config.Duration(5 * time.Second),
		},
		Storage: config.StorageConfig{
			Type:       "sqlite",
			DataSource: ":memory:",
		},
		Security: config.SecurityConfig{
			APIToken: "test-token-min-16-chars",
		},
		Logging: config.LoggingConfig{
			Level:  "error",
			Format: "text",
			Output: "stderr",
		},
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestHealthHandler(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	server := New(cfg, logger)

	tests := []struct {
		name           string
		method         string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "GET returns ok",
			method:         http.MethodGet,
			expectedStatus: http.StatusOK,
			expectedBody:   `{"status":"ok"}`,
		},
		{
			name:           "POST returns method not allowed",
			method:         http.MethodPost,
			expectedStatus: http.StatusMethodNotAllowed,
			expectedBody:   `"method_not_allowed"`,
		},
		{
			name:           "PUT returns method not allowed",
			method:         http.MethodPut,
			expectedStatus: http.StatusMethodNotAllowed,
			expectedBody:   `"method_not_allowed"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/health", nil)
			w := httptest.NewRecorder()

			server.healthHandler(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			body := w.Body.String()
			if tt.expectedStatus == http.StatusOK {
				var resp HealthResponse
				if err := json.Unmarshal([]byte(body), &resp); err != nil {
					t.Fatalf("failed to parse response: %v", err)
				}
				if resp.Status != "ok" {
					t.Errorf("expected status 'ok', got '%s'", resp.Status)
				}
			}
			if !strings.Contains(body, tt.expectedBody) {
				t.Errorf("expected body to contain '%s', got '%s'", tt.expectedBody, body)
			}
		})
	}
}

func TestAuthMiddleware(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	server := New(cfg, logger)

	tests := []struct {
		name           string
		authHeader     string
		expectedStatus int
	}{
		{
			name:           "valid token",
			authHeader:     "Bearer test-token-min-16-chars",
			expectedStatus: http.StatusNotImplemented, // Auth passes, but endpoint not implemented
		},
		{
			name:           "missing token",
			authHeader:     "",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "invalid token",
			authHeader:     "Bearer wrong-token",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "malformed bearer",
			authHeader:     "Bearer",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "wrong scheme",
			authHeader:     "Token test-token-min-16-chars",
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			w := httptest.NewRecorder()

			server.mux.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestCORSHeaders(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	server := New(cfg, logger)

	req := httptest.NewRequest(http.MethodOptions, "/health", nil)
	w := httptest.NewRecorder()

	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status 204 for OPTIONS, got %d", w.Code)
	}

	headers := w.Header()
	if headers.Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("expected CORS Allow-Origin: *, got %s", headers.Get("Access-Control-Allow-Origin"))
	}
	if headers.Get("Access-Control-Allow-Methods") == "" {
		t.Error("expected CORS Allow-Methods to be set")
	}
	if headers.Get("Access-Control-Allow-Headers") == "" {
		t.Error("expected CORS Allow-Headers to be set")
	}
}

func TestErrorResponse(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	server := New(cfg, logger)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/unknown", nil)
	req.Header.Set("Authorization", "Bearer test-token-min-16-chars")
	w := httptest.NewRecorder()

	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Errorf("expected status 501, got %d", w.Code)
	}

	var resp ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}

	if resp.Code != "not_implemented" {
		t.Errorf("expected code 'not_implemented', got '%s'", resp.Code)
	}
	if resp.RequestID == "" {
		t.Error("expected request_id to be set")
	}
}

func TestServerRunAndShutdown(t *testing.T) {
	cfg := testConfig()
	cfg.Server.Port = 0 // Let OS assign port
	logger := testLogger()
	server := New(cfg, logger)

	// Start server in goroutine
	done := make(chan error, 1)
	go func() {
		done <- server.Run()
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Test health endpoint
	addr := server.Address()
	if addr == "" {
		t.Fatal("server address should not be empty")
	}

	resp, err := http.Get("http://" + addr + "/health")
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("health check returned %d", resp.StatusCode)
	}

	// Shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		t.Errorf("shutdown error: %v", err)
	}

	// Wait for Run() to return
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run() returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("server did not shutdown within timeout")
	}
}

func TestHealthEndpointWithInvalidMethod(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	server := New(cfg, logger)

	req := httptest.NewRequest(http.MethodDelete, "/health", nil)
	w := httptest.NewRecorder()

	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestAuthMiddlewareWithIdempotencyKey(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	server := New(cfg, logger)

	// Request with valid token and idempotency key should pass auth
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", nil)
	req.Header.Set("Authorization", "Bearer test-token-min-16-chars")
	req.Header.Set("Idempotency-Key", "test-key-12345")
	w := httptest.NewRecorder()

	server.mux.ServeHTTP(w, req)

	// Should pass auth (405 or 501, not 401)
	if w.Code == http.StatusUnauthorized {
		t.Error("auth should pass with valid token")
	}
}
