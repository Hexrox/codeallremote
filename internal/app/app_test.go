package app

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/code-all-remote/car/internal/config"
	"github.com/code-all-remote/car/internal/domain"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTestApp(t *testing.T) *App {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "car.db")

	cfg := &config.Config{
		Server: config.ServerConfig{
			Host: "127.0.0.1", Port: 0,
			ReadTimeout:     config.Duration(5 * time.Second),
			WriteTimeout:    config.Duration(5 * time.Second),
			ShutdownTimeout: config.Duration(5 * time.Second),
		},
		Storage: config.StorageConfig{
			Type:       "sqlite",
			DataSource: dbPath,
		},
		Security: config.SecurityConfig{
			APIToken:    "test-token-min-16-chars",
			TokenExpiry: config.Duration(24 * time.Hour),
		},
		Logging: config.LoggingConfig{
			Level: "error", Format: "text", Output: "stderr",
		},
	}

	app, err := New(cfg, testLogger())
	if err != nil {
		t.Fatalf("failed to create app: %v", err)
	}

	// Register a workspace.
	wsDir := filepath.Join(tmpDir, "workspace")
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		t.Fatalf("failed to create workspace dir: %v", err)
	}
	app.workspaces.Register(&domain.Workspace{
		ID: "ws-1", DisplayName: "Test Workspace", Path: wsDir,
		AllowedAdapters: []string{"fake-adapter"},
	})

	if err := app.Start(context.Background()); err != nil {
		t.Fatalf("app start failed: %v", err)
	}

	t.Cleanup(func() {
		app.Shutdown(context.Background())
	})

	return app
}

func TestApp_CreateSession(t *testing.T) {
	app := newTestApp(t)

	s, err := app.CreateSession(context.Background(), "owner", CreateSessionRequest{
		WorkspaceID: "ws-1",
		AdapterID:   "fake-adapter",
		Title:       "Test session",
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	if s.ID == "" {
		t.Error("expected non-empty session ID")
	}
	if s.State != domain.SessionStateCreated {
		t.Errorf("expected state created, got %s", s.State)
	}
	if s.LastSequence < 1 {
		t.Errorf("expected last_sequence >= 1 (session.created event), got %d", s.LastSequence)
	}
}

func TestApp_CreateSession_InvalidWorkspace(t *testing.T) {
	app := newTestApp(t)

	_, err := app.CreateSession(context.Background(), "owner", CreateSessionRequest{
		WorkspaceID: "nonexistent",
		AdapterID:   "fake-adapter",
	})
	if err == nil {
		t.Error("expected error for nonexistent workspace")
	}
}

func TestApp_CreateSession_InvalidAdapter(t *testing.T) {
	app := newTestApp(t)

	_, err := app.CreateSession(context.Background(), "owner", CreateSessionRequest{
		WorkspaceID: "ws-1",
		AdapterID:   "nonexistent-adapter",
	})
	if err == nil {
		t.Error("expected error for nonexistent adapter")
	}
}

func TestApp_CreateSession_MissingFields(t *testing.T) {
	app := newTestApp(t)

	_, err := app.CreateSession(context.Background(), "owner", CreateSessionRequest{})
	if err == nil {
		t.Error("expected error for empty request")
	}
}

func TestApp_StartRunAndGetEvents(t *testing.T) {
	app := newTestApp(t)

	s, _ := app.CreateSession(context.Background(), "owner", CreateSessionRequest{
		WorkspaceID: "ws-1", AdapterID: "fake-adapter",
	})

	run, err := app.StartRun(context.Background(), "owner", s.ID)
	if err != nil {
		t.Fatalf("StartRun failed: %v", err)
	}
	if run.ID == "" {
		t.Error("expected non-empty run ID")
	}

	// Give the fake adapter time to emit signals.
	time.Sleep(200 * time.Millisecond)

	// Fetch events.
	result, err := app.GetEvents(s.ID, 0, 100)
	if err != nil {
		t.Fatalf("GetEvents failed: %v", err)
	}
	if len(result.Events) < 2 {
		t.Errorf("expected at least 2 events, got %d", len(result.Events))
	}

	// First event should be session.created.
	if result.Events[0].Type != "session.created" {
		t.Errorf("expected first event session.created, got %s", result.Events[0].Type)
	}
}

func TestApp_StartRun_NotFound(t *testing.T) {
	app := newTestApp(t)

	_, err := app.StartRun(context.Background(), "owner", "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent session")
	}
}

func TestApp_SubmitPrompt(t *testing.T) {
	app := newTestApp(t)

	s, _ := app.CreateSession(context.Background(), "owner", CreateSessionRequest{
		WorkspaceID: "ws-1", AdapterID: "fake-adapter",
	})
	app.StartRun(context.Background(), "owner", s.ID)

	time.Sleep(50 * time.Millisecond)

	err := app.SubmitPrompt(context.Background(), "owner", s.ID, "do something")
	if err != nil {
		t.Fatalf("SubmitPrompt failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	result, _ := app.GetEvents(s.ID, 0, 100)
	found := false
	for _, e := range result.Events {
		if e.Type == "run.prompt" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected run.prompt event")
	}
}

func TestApp_SubmitPrompt_EmptyText(t *testing.T) {
	app := newTestApp(t)

	s, _ := app.CreateSession(context.Background(), "owner", CreateSessionRequest{
		WorkspaceID: "ws-1", AdapterID: "fake-adapter",
	})
	err := app.SubmitPrompt(context.Background(), "owner", s.ID, "")
	if err == nil {
		t.Error("expected error for empty prompt")
	}
}

func TestApp_Interrupt(t *testing.T) {
	app := newTestApp(t)

	s, _ := app.CreateSession(context.Background(), "owner", CreateSessionRequest{
		WorkspaceID: "ws-1", AdapterID: "fake-adapter",
	})
	app.StartRun(context.Background(), "owner", s.ID)
	time.Sleep(50 * time.Millisecond)

	err := app.Interrupt(context.Background(), "owner", s.ID)
	if err != nil {
		t.Fatalf("Interrupt failed: %v", err)
	}

	// Verify session is interrupted.
	refreshed, _ := app.GetSession(s.ID)
	if refreshed.State != domain.SessionStateInterrupted {
		t.Errorf("expected interrupted state, got %s", refreshed.State)
	}
}

func TestApp_GetSession_NotFound(t *testing.T) {
	app := newTestApp(t)

	_, err := app.GetSession("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent session")
	}
}

func TestApp_GetEvents_SessionNotFound(t *testing.T) {
	app := newTestApp(t)

	_, err := app.GetEvents("nonexistent", 0, 100)
	if err == nil {
		t.Error("expected error for nonexistent session")
	}
}

func TestApp_ReplayWithCursor(t *testing.T) {
	app := newTestApp(t)

	s, _ := app.CreateSession(context.Background(), "owner", CreateSessionRequest{
		WorkspaceID: "ws-1", AdapterID: "fake-adapter",
	})
	app.StartRun(context.Background(), "owner", s.ID)
	time.Sleep(200 * time.Millisecond)

	// Replay from beginning.
	all, _ := app.GetEvents(s.ID, 0, 100)
	if len(all.Events) == 0 {
		t.Fatal("expected at least one event")
	}

	// Replay from after the first event.
	midCursor := all.Events[0].Sequence
	rest, _ := app.GetEvents(s.ID, midCursor, 100)
	if len(rest.Events) != len(all.Events)-1 {
		t.Errorf("expected %d events after cursor %d, got %d",
			len(all.Events)-1, midCursor, len(rest.Events))
	}
}

func TestApp_APIToken(t *testing.T) {
	app := newTestApp(t)
	if app.APIToken() != "test-token-min-16-chars" {
		t.Errorf("unexpected API token: %s", app.APIToken())
	}
}

func TestApp_ListSessions(t *testing.T) {
	app := newTestApp(t)

	app.CreateSession(context.Background(), "owner", CreateSessionRequest{
		WorkspaceID: "ws-1", AdapterID: "fake-adapter",
	})
	app.CreateSession(context.Background(), "owner", CreateSessionRequest{
		WorkspaceID: "ws-1", AdapterID: "fake-adapter",
	})

	sessions, err := app.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(sessions))
	}
}
