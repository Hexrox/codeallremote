// Package server provides the HTTP server for the CAR API.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/code-all-remote/car/internal/api"
	"github.com/code-all-remote/car/internal/app"
	"github.com/code-all-remote/car/internal/auth"
	"github.com/code-all-remote/car/internal/config"
	"github.com/code-all-remote/car/internal/obs"
	"github.com/code-all-remote/car/internal/ws"
)

// Server represents the CAR HTTP server.
type Server struct {
	config      *config.Config
	httpServer  *http.Server
	logger      *slog.Logger
	mux         *http.ServeMux
	application *app.App

	addrMu     sync.RWMutex
	actualAddr string
}

// HealthResponse represents the /health endpoint response.
type HealthResponse struct {
	Status string `json:"status"`
}

// ErrorResponse represents an API error response.
type ErrorResponse struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Details   map[string]any `json:"details,omitempty"`
	RequestID string         `json:"request_id"`
}

// New creates a new server instance backed by the given application.
// If app is nil, API endpoints return 501 (used in tests).
func New(cfg *config.Config, logger *slog.Logger) *Server {
	return NewWithApp(cfg, nil, logger)
}

// NewWithApp creates a new server instance wired to an application.
func NewWithApp(cfg *config.Config, application *app.App, logger *slog.Logger) *Server {
	mux := http.NewServeMux()
	s := &Server{
		config:      cfg,
		logger:      logger,
		mux:         mux,
		application: application,
	}

	s.registerRoutes()

	return s
}

// registerRoutes registers all HTTP handlers.
func (s *Server) registerRoutes() {
	// Public endpoints (no auth required). /health is liveness; /ready is
	// readiness (storage reachable).
	s.mux.HandleFunc("/health", s.withCORS(s.healthHandler))

	if s.application != nil {
		// Register the full API contract directly on the mux.
		handlers := api.NewHandlers(s.application, s.logger)
		handlers.Register(s.mux)

		// Pairing and device-management endpoints. The auth wrapper reuses
		// the Handlers' withAuth so paired-device tokens are honoured.
		pairing := api.NewPairingHandlers(s.application.Identity())
		pairing.RegisterPairing(s.mux, handlers.WithAuth())

		// WebSocket gateway for live session events.
		hub := ws.NewHub(s.application.CursorRepository(), s.logger)
		s.application.SetEventPublisher(ws.NewPublishAdapter(hub))
		wsHandler := ws.NewHandler(hub, s.application.Identity(), s.logger)
		s.mux.HandleFunc("/api/v1/ws", wsHandler.ServeHTTP)

		// Observability: readiness + diagnostics (no secrets).
		diag := obs.NewDiagnosticView(obs.NewRecorder(), s.application.ReadinessProbe())
		s.mux.HandleFunc("/ready", s.withCORS(diag.HTTPHandler()))
	} else {
		// No application wired: return 501 for API endpoints (used in unit tests).
		s.mux.HandleFunc("/api/v1/", s.withCORS(s.withAuth(s.notImplementedHandler)))
	}
}

// withCORS adds CORS headers for the Android client.
func (s *Server) withCORS(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Idempotency-Key")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		handler(w, r)
	}
}

// withAuth wraps a handler with authentication middleware.
func (s *Server) withAuth(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for OPTIONS requests
		if r.Method == http.MethodOptions {
			handler(w, r)
			return
		}

		// Extract bearer token with a strict prefix check (no length-only strip).
		token := r.Header.Get("Authorization")
		token = auth.BearerToken(token)
		if token == "" {
			s.writeError(w, http.StatusUnauthorized, "unauthorized", "Missing or invalid Authorization header")
			return
		}

		// Constant-time comparison prevents timing-based token recovery.
		if !auth.ConstantTimeEqual(token, s.config.Security.APIToken) {
			s.writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid authentication token")
			return
		}

		handler(w, r)
	}
}

// healthHandler handles GET /health requests.
func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET is supported")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	resp := HealthResponse{Status: "ok"}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		s.logger.Error("failed to encode health response", "error", err)
	}
}

// notImplementedHandler returns 501 for unimplemented endpoints.
func (s *Server) notImplementedHandler(w http.ResponseWriter, r *http.Request) {
	s.writeError(w, http.StatusNotImplemented, "not_implemented", "This endpoint is not yet implemented")
}

// writeError writes a standardized error response.
func (s *Server) writeError(w http.ResponseWriter, statusCode int, code string, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	resp := ErrorResponse{
		Code:      code,
		Message:   message,
		RequestID: generateRequestID(),
	}

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		s.logger.Error("failed to encode error response", "error", err, "status", statusCode)
	}
}

// generateRequestID creates a unique request identifier.
func generateRequestID() string {
	// Simple implementation - could use uuid package in production
	return fmt.Sprintf("req-%d", time.Now().UnixNano())
}

// Run starts the server and blocks until shutdown.
// Returns an error if the server fails to start or encounters a fatal error.
func (s *Server) Run() error {
	// Ensure storage directory exists
	if err := s.config.Storage.EnsureDir(); err != nil {
		return fmt.Errorf("failed to create storage directory: %w", err)
	}

	// Create HTTP server
	s.httpServer = &http.Server{
		Addr:         s.config.Server.Address(),
		Handler:      s.mux,
		ReadTimeout:  s.config.Server.ReadTimeout.Duration(),
		WriteTimeout: s.config.Server.WriteTimeout.Duration(),
	}

	// Resolve the actual listen address (needed when port 0 is configured
	// and the OS assigns an ephemeral port).
	ln, err := net.Listen("tcp", s.config.Server.Address())
	if err != nil {
		return fmt.Errorf("listening on %s: %w", s.config.Server.Address(), err)
	}

	s.addrMu.Lock()
	s.actualAddr = ln.Addr().String()
	s.addrMu.Unlock()

	// Channel to receive shutdown signals
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Channel to receive server errors
	serverErr := make(chan error, 1)

	// Start serving in a goroutine.
	go func() {
		s.logger.Info("starting CAR server",
			"address", s.ActualAddress(),
			"storage_type", s.config.Storage.Type,
			"storage_path", s.config.Storage.DataSource,
		)

		if err := s.httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
		close(serverErr)
	}()

	// Wait for shutdown signal or server error
	select {
	case sig := <-quit:
		s.logger.Info("received shutdown signal", "signal", sig)
	case err, ok := <-serverErr:
		// ok == false means the channel was closed because Serve returned
		// http.ErrServerClosed (a normal shutdown), not a real error.
		if ok && err != nil {
			return fmt.Errorf("server error: %w", err)
		}
		// Otherwise the server stopped cleanly; proceed to graceful shutdown.
	}

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), s.config.Server.ShutdownTimeout.Duration())
	defer cancel()

	s.logger.Info("shutting down server", "timeout", s.config.Server.ShutdownTimeout)

	if err := s.httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("server shutdown error: %w", err)
	}

	// Shut down the application (drains runs, closes storage).
	if s.application != nil {
		if err := s.application.Shutdown(ctx); err != nil {
			s.logger.Error("application shutdown error", "error", err)
		}
	}

	s.logger.Info("server shutdown complete")
	return nil
}

// Shutdown initiates a graceful shutdown of the server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpServer == nil {
		return nil
	}
	return s.httpServer.Shutdown(ctx)
}

// Address returns the server's bind address.
func (s *Server) Address() string {
	s.addrMu.RLock()
	defer s.addrMu.RUnlock()
	if s.actualAddr != "" {
		return s.actualAddr
	}
	return s.config.Server.Address()
}

// ActualAddress returns the real address the server is listening on,
// including an OS-assigned ephemeral port when port 0 was configured.
func (s *Server) ActualAddress() string {
	s.addrMu.RLock()
	defer s.addrMu.RUnlock()
	if s.actualAddr != "" {
		return s.actualAddr
	}
	return s.config.Server.Address()
}
