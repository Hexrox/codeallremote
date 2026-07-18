// Package config provides configuration loading and validation for the CAR server.
package config

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Duration is a time.Duration that can be unmarshaled from either a JSON
// number (nanoseconds) or a JSON string (e.g. "30s", "5m"). This keeps
// human-readable config files working with encoding/json.
type Duration time.Duration

// UnmarshalJSON implements json.Unmarshaler for Duration.
func (d *Duration) UnmarshalJSON(b []byte) error {
	// Try as a number first (nanoseconds).
	var n int64
	if err := json.Unmarshal(b, &n); err == nil {
		*d = Duration(n)
		return nil
	}

	// Otherwise treat as a string like "30s".
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("duration must be a number or a string like \"30s\": %w", err)
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	*d = Duration(parsed)
	return nil
}

// Duration returns the underlying time.Duration.
func (d Duration) Duration() time.Duration {
	return time.Duration(d)
}

// Config holds the complete server configuration.
type Config struct {
	// Server settings
	Server ServerConfig `json:"server"`

	// Storage settings
	Storage StorageConfig `json:"storage"`

	// Workspace settings
	Workspaces []WorkspaceConfig `json:"workspaces,omitempty"`

	// Security settings
	Security SecurityConfig `json:"security"`

	// Logging settings
	Logging LoggingConfig `json:"logging"`

	// Adapters defines executable paths and supported versions for agent
	// adapters (docs/40 §adapters). An adapter with an empty path is not
	// registered (its self-check fails closed).
	Adapters []AdapterConfig `json:"adapters,omitempty"`
}

// AdapterConfig configures one agent adapter.
type AdapterConfig struct {
	// ID is the plugin id, e.g. "claude-code".
	ID string `json:"id"`

	// ExecPath is the absolute path to the agent executable.
	ExecPath string `json:"exec_path"`

	// SupportedVersions is a human-readable version range (informational).
	SupportedVersions string `json:"supported_versions,omitempty"`

	// Env is non-secret environment variables passed to the adapter's child
	// process on top of the server's environment (e.g.
	// ANTHROPIC_BASE_URL=http://127.0.0.1:3456 to point claude at a local
	// Claude Code Router instead of the Anthropic API). NEVER put secrets here:
	// config is backed up alongside the database; place credentials in the
	// server's environment (systemd) which the adapter inherits.
	Env map[string]string `json:"env,omitempty"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	// Host to bind to (default: "127.0.0.1")
	Host string `json:"host"`

	// Port to listen on (default: 8080)
	Port int `json:"port"`

	// ReadTimeout for HTTP requests
	ReadTimeout Duration `json:"read_timeout"`

	// WriteTimeout for HTTP responses
	WriteTimeout Duration `json:"write_timeout"`

	// ShutdownTimeout for graceful shutdown
	ShutdownTimeout Duration `json:"shutdown_timeout"`
}

// StorageConfig holds database/file storage settings.
type StorageConfig struct {
	// Type of storage backend ("sqlite" or "postgres")
	Type string `json:"type"`

	// Connection string or file path
	DataSource string `json:"data_source"`

	// MaxOpenConns for connection pooling
	MaxOpenConns int `json:"max_open_conns"`

	// MaxIdleConns for connection pooling
	MaxIdleConns int `json:"max_idle_conns"`

	// ConnLifetime is the maximum time a connection can be reused
	ConnLifetime Duration `json:"conn_lifetime"`
}

// WorkspaceConfig defines a registered workspace.
type WorkspaceConfig struct {
	// ID is a unique identifier for the workspace
	ID string `json:"id"`

	// DisplayName is a human-readable name
	DisplayName string `json:"display_name"`

	// Path is the absolute filesystem path to the workspace
	Path string `json:"path"`

	// AllowedAdapters lists which adapters can operate in this workspace
	AllowedAdapters []string `json:"allowed_adapters,omitempty"`

	// ExecutionPolicy restricts what operations are allowed
	ExecutionPolicy ExecutionPolicy `json:"execution_policy"`
}

// ExecutionPolicy defines workspace execution restrictions.
type ExecutionPolicy struct {
	// AllowNetworkAccess permits outbound network calls from agent
	AllowNetworkAccess bool `json:"allow_network_access"`

	// AllowWrites permits filesystem writes
	AllowWrites bool `json:"allow_writes"`

	// AllowedCommands restricts which commands can be executed (empty = all)
	AllowedCommands []string `json:"allowed_commands,omitempty"`
}

// SecurityConfig holds security-related settings.
type SecurityConfig struct {
	// APIToken is the bearer token for API authentication
	APIToken string `json:"api_token"`

	// TokenExpiry is how long pairing tokens remain valid
	TokenExpiry Duration `json:"token_expiry"`

	// RequireApprovalForWrites requires approval for write operations
	RequireApprovalForWrites bool `json:"require_approval_for_writes"`

	// RedactionPatterns are extra substring patterns to redact from audit
	// context (e.g. "ANTHROPIC_API_KEY-xxxx"). The APIToken value itself is
	// automatically redacted and need not be listed.
	RedactionPatterns []string `json:"redaction_patterns,omitempty"`

	// AllowedOrigin is the single web origin allowed by CORS (e.g.
	// "https://console.car.example"). Empty (default) refuses all cross-origin
	// browser requests, which is correct for the Android-first MVP.
	AllowedOrigin string `json:"allowed_origin,omitempty"`
}

// LoggingConfig holds logging settings.
type LoggingConfig struct {
	// Level is the minimum log level ("debug", "info", "warn", "error")
	Level string `json:"level"`

	// Format is the log output format ("json" or "text")
	Format string `json:"format"`

	// Output is where to write logs ("stdout", "stderr", or file path)
	Output string `json:"output"`
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Host:            "127.0.0.1",
			Port:            8080,
			ReadTimeout:     Duration(30 * time.Second),
			WriteTimeout:    Duration(30 * time.Second),
			ShutdownTimeout: Duration(30 * time.Second),
		},
		Storage: StorageConfig{
			Type:         "sqlite",
			DataSource:   "data/car.db",
			MaxOpenConns: 1, // SQLite default
			MaxIdleConns: 1,
			ConnLifetime: 0,
		},
		Security: SecurityConfig{
			TokenExpiry:              Duration(24 * time.Hour),
			RequireApprovalForWrites: true,
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "text",
			Output: "stdout",
		},
		Workspaces: []WorkspaceConfig{},
	}
}

// LoadConfig loads configuration from a JSON file, applying defaults.
func LoadConfig(path string) (*Config, error) {
	if path == "" {
		return nil, fmt.Errorf("config path cannot be empty")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	return cfg, nil
}

// Validate checks that the configuration is valid.
// Returns a multi-error with all validation failures.
func (c *Config) Validate() error {
	var errs []string

	// Validate server config
	if c.Server.Host == "" {
		errs = append(errs, "server.host cannot be empty")
	}
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		errs = append(errs, fmt.Sprintf("server.port must be between 1 and 65535, got %d", c.Server.Port))
	}
	if c.Server.ReadTimeout < 0 {
		errs = append(errs, "server.read_timeout cannot be negative")
	}
	if c.Server.WriteTimeout < 0 {
		errs = append(errs, "server.write_timeout cannot be negative")
	}
	if c.Server.ShutdownTimeout < 0 {
		errs = append(errs, "server.shutdown_timeout cannot be negative")
	}

	// Validate storage config
	if c.Storage.Type == "" {
		errs = append(errs, "storage.type cannot be empty")
	}
	if c.Storage.Type != "sqlite" && c.Storage.Type != "postgres" {
		errs = append(errs, fmt.Sprintf("storage.type must be 'sqlite' or 'postgres', got '%s'", c.Storage.Type))
	}
	if c.Storage.DataSource == "" {
		errs = append(errs, "storage.data_source cannot be empty")
	}
	if c.Storage.MaxOpenConns < 0 {
		errs = append(errs, "storage.max_open_conns cannot be negative")
	}
	if c.Storage.MaxIdleConns < 0 {
		errs = append(errs, "storage.max_idle_conns cannot be negative")
	}
	if c.Storage.MaxIdleConns > c.Storage.MaxOpenConns && c.Storage.MaxOpenConns > 0 {
		errs = append(errs, "storage.max_idle_conns cannot exceed storage.max_open_conns")
	}

	// Validate security config
	if c.Security.APIToken == "" {
		errs = append(errs, "security.api_token cannot be empty")
	}
	if len(c.Security.APIToken) < 16 {
		errs = append(errs, "security.api_token must be at least 16 characters")
	}
	if c.Security.TokenExpiry.Duration() < time.Minute {
		errs = append(errs, "security.token_expiry must be at least 1 minute")
	}

	// Validate logging config
	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLevels[strings.ToLower(c.Logging.Level)] {
		errs = append(errs, fmt.Sprintf("logging.level must be 'debug', 'info', 'warn', or 'error', got '%s'", c.Logging.Level))
	}
	validFormats := map[string]bool{"json": true, "text": true}
	if !validFormats[strings.ToLower(c.Logging.Format)] {
		errs = append(errs, fmt.Sprintf("logging.format must be 'json' or 'text', got '%s'", c.Logging.Format))
	}

	// Validate workspaces
	workspaceIDs := make(map[string]bool)
	for i, ws := range c.Workspaces {
		if ws.ID == "" {
			errs = append(errs, fmt.Sprintf("workspaces[%d].id cannot be empty", i))
		}
		if workspaceIDs[ws.ID] {
			errs = append(errs, fmt.Sprintf("workspaces[%d].id '%s' is duplicated", i, ws.ID))
		}
		workspaceIDs[ws.ID] = true

		if ws.DisplayName == "" {
			errs = append(errs, fmt.Sprintf("workspaces[%d].display_name cannot be empty", i))
		}
		if ws.Path == "" {
			errs = append(errs, fmt.Sprintf("workspaces[%d].path cannot be empty", i))
		}
		if !filepath.IsAbs(ws.Path) {
			errs = append(errs, fmt.Sprintf("workspaces[%d].path must be absolute, got '%s'", i, ws.Path))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("configuration validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}

	return nil
}

// Address returns the full bind address for the server.
func (c *ServerConfig) Address() string {
	return net.JoinHostPort(c.Host, fmt.Sprintf("%d", c.Port))
}

// EnsureDir creates the directory for a SQLite data source if needed.
func (c *StorageConfig) EnsureDir() error {
	if c.Type != "sqlite" {
		return nil
	}
	dir := filepath.Dir(c.DataSource)
	if dir == "." || dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0o750)
}
