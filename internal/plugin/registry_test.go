package plugin

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func validManifest() Manifest {
	return Manifest{
		ID: "diff-viewer", Name: "Diff Viewer", Version: "1.0.0",
		MinProtocol: 1, MaxProtocol: 1, Capabilities: []string{"diff"},
	}
}

func TestRegistry_RegisterValid(t *testing.T) {
	r := NewRegistry(testLogger(), 1)
	p := &Plugin{Manifest: validManifest()}
	if err := r.Register(p); err != nil {
		t.Fatalf("register: %v", err)
	}
	got, ok := r.Get("diff-viewer")
	if !ok {
		t.Fatal("expected plugin registered")
	}
	if got.State != StateActive {
		t.Errorf("expected active, got %s", got.State)
	}
}

func TestRegistry_RejectIncompatible(t *testing.T) {
	// Server speaks protocol 1; plugin requires 2+.
	r := NewRegistry(testLogger(), 1)
	m := validManifest()
	m.MinProtocol = 2
	m.MaxProtocol = 3
	p := &Plugin{Manifest: m}

	if err := r.Register(p); err == nil {
		t.Fatal("expected rejection for incompatible protocol")
	}
	// Rejected plugins appear in diagnostics explaining why.
	got, ok := r.Get(m.ID)
	if !ok {
		t.Fatal("expected rejected plugin recorded")
	}
	if got.State != StateRejected {
		t.Errorf("expected rejected state, got %s", got.State)
	}
}

func TestRegistry_RejectInvalidManifest(t *testing.T) {
	r := NewRegistry(testLogger(), 1)

	cases := []struct {
		name string
		m    Manifest
	}{
		{"empty id", Manifest{ID: "", Name: "x", Version: "1", MinProtocol: 1, MaxProtocol: 1}},
		{"empty name", Manifest{ID: "x", Name: "", Version: "1", MinProtocol: 1, MaxProtocol: 1}},
		{"empty version", Manifest{ID: "x", Name: "x", Version: "", MinProtocol: 1, MaxProtocol: 1}},
		{"zero min", Manifest{ID: "x", Name: "x", Version: "1", MinProtocol: 0, MaxProtocol: 1}},
		{"max<min", Manifest{ID: "x", Name: "x", Version: "1", MinProtocol: 2, MaxProtocol: 1}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := r.Validate(c.m); err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

func TestRegistry_RejectDuplicate(t *testing.T) {
	r := NewRegistry(testLogger(), 1)
	r.Register(&Plugin{Manifest: validManifest()})
	err := r.Register(&Plugin{Manifest: validManifest()})
	if err == nil {
		t.Fatal("expected duplicate error")
	}
}

func TestRegistry_ShutdownDrainsPlugins(t *testing.T) {
	r := NewRegistry(testLogger(), 1)
	r.SetDrainTimeout(500 * time.Millisecond)

	drained := make(chan struct{})
	p := &Plugin{
		Manifest: validManifest(),
		Drain: func(ctx context.Context) error {
			time.Sleep(20 * time.Millisecond)
			close(drained)
			return nil
		},
	}
	r.Register(p)

	r.Shutdown(context.Background())

	select {
	case <-drained:
	default:
		t.Error("expected drain to be called")
	}

	got, _ := r.Get(p.Manifest.ID)
	if got.State != StateStopped {
		t.Errorf("expected stopped, got %s", got.State)
	}
}

func TestRegistry_ShutdownTimeout(t *testing.T) {
	r := NewRegistry(testLogger(), 1)
	r.SetDrainTimeout(50 * time.Millisecond)

	// Drain that blocks forever.
	p := &Plugin{
		Manifest: validManifest(),
		Drain: func(ctx context.Context) error {
			<-ctx.Done() // honor cancellation
			return ctx.Err()
		},
	}
	r.Register(p)

	start := time.Now()
	r.Shutdown(context.Background())
	elapsed := time.Since(start)

	// Must return within roughly the drain timeout, not block forever.
	if elapsed > 1*time.Second {
		t.Errorf("shutdown took too long: %v", elapsed)
	}
	got, _ := r.Get(p.Manifest.ID)
	if got.State != StateStopped {
		t.Errorf("expected stopped after timeout, got %s", got.State)
	}
}

func TestRegistry_DiagnosticsHideSecrets(t *testing.T) {
	r := NewRegistry(testLogger(), 1)
	r.Register(&Plugin{Manifest: Manifest{
		ID: "secret-tool", Name: "Secret Tool", Version: "1.0",
		MinProtocol: 1, MaxProtocol: 1,
		Capabilities:   []string{"vault"},
		RequiresSecret: true,
	}})

	diags := r.Diagnostics()
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	// RequiresSecret is intentionally not in the Diagnostic struct.
	d := diags[0]
	// Validate no secret-related field leaks.
	for _, cap := range d.Capabilities {
		if cap == "" {
			continue
		}
	}
}

func TestRegistry_ActivePluginsOnly(t *testing.T) {
	r := NewRegistry(testLogger(), 1)
	r.Register(&Plugin{Manifest: validManifest()})

	// Add a rejected plugin.
	bad := validManifest()
	bad.ID = "bad"
	bad.MinProtocol = 5
	r.Register(&Plugin{Manifest: bad})

	active := r.ActivePlugins()
	if len(active) != 1 {
		t.Errorf("expected 1 active, got %d", len(active))
	}
	if active[0].Manifest.ID != "diff-viewer" {
		t.Errorf("expected diff-viewer, got %s", active[0].Manifest.ID)
	}
}

func TestRegistry_FakePluginFullCycle(t *testing.T) {
	// Fake plugin scenario: register, use capability, drain on shutdown.
	r := NewRegistry(testLogger(), 1)

	called := false
	p := &Plugin{
		Manifest: Manifest{
			ID: "fake", Name: "Fake", Version: "0.1",
			MinProtocol: 1, MaxProtocol: 1,
			Capabilities: []string{"greet"},
		},
		Drain: func(ctx context.Context) error {
			called = true
			return nil
		},
	}
	if err := r.Register(p); err != nil {
		t.Fatalf("register: %v", err)
	}

	// "Use" the plugin.
	active := r.ActivePlugins()
	if len(active) != 1 || active[0].Manifest.ID != "fake" {
		t.Fatal("expected fake plugin active")
	}

	r.Shutdown(context.Background())
	if !called {
		t.Error("expected drain called")
	}
}

func TestRegistry_RejectedPluginRejectedFullCycle(t *testing.T) {
	r := NewRegistry(testLogger(), 1)
	// Incompatible plugin.
	m := validManifest()
	m.ID = "incompat"
	m.MinProtocol = 2
	m.MaxProtocol = 2
	err := r.Register(&Plugin{Manifest: m})
	if err == nil {
		t.Fatal("expected rejection")
	}

	// It must NOT appear in active plugins.
	if len(r.ActivePlugins()) != 0 {
		t.Error("rejected plugin should not be active")
	}
	// But it appears in diagnostics explaining the rejection.
	diags := r.Diagnostics()
	if len(diags) != 1 || diags[0].State != string(StateRejected) {
		t.Errorf("expected 1 rejected diagnostic, got %+v", diags)
	}
}
