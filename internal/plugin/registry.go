// Package plugin implements the CAR plugin manifest and registry.
//
// A plugin declares a manifest with its id, version, the CAR protocol range
// it supports, and its capabilities. The registry validates manifests, gates
// activation on compatibility, exposes capabilities through diagnostics, and
// drains active plugin work within a bounded timeout on shutdown.
//
// Invalid or incompatible manifests prevent activation: a misbehaving plugin
// must never endanger the core.
package plugin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Manifest describes a plugin and its compatibility envelope.
type Manifest struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Version        string   `json:"version"`
	MinProtocol    int      `json:"min_protocol"` // inclusive
	MaxProtocol    int      `json:"max_protocol"` // inclusive
	Capabilities   []string `json:"capabilities"`
	RequiresSecret bool     `json:"requires_secret,omitempty"`
}

// Plugin is an activated plugin instance.
type Plugin struct {
	Manifest  Manifest
	Activated time.Time
	State     State
	// Drain is invoked on shutdown; it must complete within the registry's
	// drain timeout. A nil Drain is a no-op.
	Drain func(ctx context.Context) error
}

// State is the lifecycle state of a plugin.
type State string

const (
	StateActive   State = "active"
	StateDraining State = "draining"
	StateStopped  State = "stopped"
	StateRejected State = "rejected"
)

// Registry holds activated plugins keyed by ID.
type Registry struct {
	mu           sync.RWMutex
	plugins      map[string]*Plugin
	logger       *slog.Logger
	supported    int // CAR protocol version this server speaks
	drainTimeout time.Duration
}

// NewRegistry creates a new plugin registry.
// supported is the CAR protocol version; manifests whose [min,max] range does
// not include it are rejected as incompatible.
func NewRegistry(logger *slog.Logger, supported int) *Registry {
	return &Registry{
		plugins:      make(map[string]*Plugin),
		logger:       logger,
		supported:    supported,
		drainTimeout: 5 * time.Second,
	}
}

// SetDrainTimeout overrides the shutdown drain timeout (for tests).
func (r *Registry) SetDrainTimeout(d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.drainTimeout = d
}

// Validate checks a manifest without activating it. Returns the reason for
// rejection, or nil if the manifest is sound and compatible.
func (r *Registry) Validate(m Manifest) error {
	if m.ID == "" {
		return errors.New("manifest id is required")
	}
	if m.Name == "" {
		return errors.New("manifest name is required")
	}
	if m.Version == "" {
		return errors.New("manifest version is required")
	}
	if m.MinProtocol <= 0 {
		return errors.New("manifest min_protocol must be positive")
	}
	if m.MaxProtocol < m.MinProtocol {
		return errors.New("manifest max_protocol < min_protocol")
	}
	// Compatibility: the server's protocol version must fall within the
	// plugin's supported range. An incompatible plugin is rejected without
	// activation — it cannot endanger the core.
	if r.supported < m.MinProtocol || r.supported > m.MaxProtocol {
		return fmt.Errorf("plugin %s supports protocol [%d,%d], server speaks %d",
			m.ID, m.MinProtocol, m.MaxProtocol, r.supported)
	}
	return nil
}

// Register validates and activates a plugin. On validation failure the plugin
// is recorded as rejected (so diagnostics can show why) and an error returned.
func (r *Registry) Register(p *Plugin) error {
	if p == nil {
		return errors.New("plugin is nil")
	}
	if err := r.Validate(p.Manifest); err != nil {
		r.mu.Lock()
		r.plugins[p.Manifest.ID] = &Plugin{
			Manifest: p.Manifest, Activated: time.Now(), State: StateRejected,
		}
		r.mu.Unlock()
		r.logger.Warn("plugin rejected", "id", p.Manifest.ID, "reason", err)
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.plugins[p.Manifest.ID]; exists {
		return fmt.Errorf("plugin %s already registered", p.Manifest.ID)
	}
	p.Activated = time.Now()
	p.State = StateActive
	r.plugins[p.Manifest.ID] = p
	r.logger.Info("plugin activated",
		"id", p.Manifest.ID, "version", p.Manifest.Version,
		"capabilities", p.Manifest.Capabilities)
	return nil
}

// Get returns an activated plugin by ID.
func (r *Registry) Get(id string) (*Plugin, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.plugins[id]
	return p, ok
}

// List returns all plugins (active and rejected).
func (r *Registry) List() []*Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Plugin, 0, len(r.plugins))
	for _, p := range r.plugins {
		out = append(out, p)
	}
	return out
}

// ActivePlugins returns only active plugins (for capability dispatch).
func (r *Registry) ActivePlugins() []*Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Plugin, 0)
	for _, p := range r.plugins {
		if p.State == StateActive {
			out = append(out, p)
		}
	}
	return out
}

// Diagnostics returns a diagnostic summary of all plugins (capabilities
// visible; no secrets).
func (r *Registry) Diagnostics() []Diagnostic {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Diagnostic, 0, len(r.plugins))
	for _, p := range r.plugins {
		out = append(out, Diagnostic{
			ID:           p.Manifest.ID,
			Name:         p.Manifest.Name,
			Version:      p.Manifest.Version,
			State:        string(p.State),
			Capabilities: p.Manifest.Capabilities,
			MinProtocol:  p.Manifest.MinProtocol,
			MaxProtocol:  p.Manifest.MaxProtocol,
			ActivatedAt:  p.Activated,
		})
	}
	return out
}

// Diagnostic is a secret-free plugin summary exposed to operators.
type Diagnostic struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Version      string    `json:"version"`
	State        string    `json:"state"`
	Capabilities []string  `json:"capabilities"`
	MinProtocol  int       `json:"min_protocol"`
	MaxProtocol  int       `json:"max_protocol"`
	ActivatedAt  time.Time `json:"activated_at"`
}

// Shutdown drains active plugin work within the bounded timeout.
// Plugins that do not finish in time are marked stopped (best-effort) and
// logged; the core continues shutdown regardless.
func (r *Registry) Shutdown(ctx context.Context) error {
	r.mu.Lock()
	active := make([]*Plugin, 0)
	for _, p := range r.plugins {
		if p.State == StateActive {
			p.State = StateDraining
			active = append(active, p)
		}
	}
	timeout := r.drainTimeout
	r.mu.Unlock()

	if len(active) == 0 {
		return nil
	}

	drainCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var wg sync.WaitGroup
	for _, p := range active {
		wg.Add(1)
		go func(p *Plugin) {
			defer wg.Done()
			if p.Drain == nil {
				return
			}
			if err := p.Drain(drainCtx); err != nil {
				r.logger.Warn("plugin drain error", "id", p.Manifest.ID, "error", err)
			}
		}(p)
	}

	// Wait up to the drain timeout; do not block shutdown forever.
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-drainCtx.Done():
		r.logger.Warn("plugin drain timed out", "timeout", timeout)
	}

	r.mu.Lock()
	for _, p := range active {
		p.State = StateStopped
	}
	r.mu.Unlock()
	return nil
}
