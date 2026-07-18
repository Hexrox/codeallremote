package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("expected host 127.0.0.1, got %s", cfg.Server.Host)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Storage.Type != "sqlite" {
		t.Errorf("expected storage type sqlite, got %s", cfg.Storage.Type)
	}
	if cfg.Logging.Level != "info" {
		t.Errorf("expected logging level info, got %s", cfg.Logging.Level)
	}
}

func TestLoadConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	validConfig := `{
		"server": {"host": "0.0.0.0", "port": 9000},
		"storage": {"type": "sqlite", "data_source": "/tmp/car.db"},
		"security": {"api_token": "this-is-a-valid-token-12345"}
	}`

	err := os.WriteFile(configPath, []byte(validConfig), 0o600)
	if err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("expected host 0.0.0.0, got %s", cfg.Server.Host)
	}
	if cfg.Server.Port != 9000 {
		t.Errorf("expected port 9000, got %d", cfg.Server.Port)
	}
	if cfg.Storage.DataSource != "/tmp/car.db" {
		t.Errorf("expected data_source /tmp/car.db, got %s", cfg.Storage.DataSource)
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path/config.json")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoadConfig_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	err := os.WriteFile(configPath, []byte(`{invalid json}`), 0o600)
	if err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	_, err = LoadConfig(configPath)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestLoadConfig_EmptyPath(t *testing.T) {
	_, err := LoadConfig("")
	if err == nil {
		t.Fatal("expected error for empty path, got nil")
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name      string
		config    *Config
		wantError bool
	}{
		{
			name:      "valid minimal config",
			config:    validMinimalConfig(),
			wantError: false,
		},
		{
			name:      "valid full config with workspace",
			config:    validFullConfig(),
			wantError: false,
		},
		{
			name:      "empty host",
			config:    withHost(validMinimalConfig(), ""),
			wantError: true,
		},
		{
			name:      "invalid port zero",
			config:    withPort(validMinimalConfig(), 0),
			wantError: true,
		},
		{
			name:      "invalid port too high",
			config:    withPort(validMinimalConfig(), 70000),
			wantError: true,
		},
		{
			name:      "negative read timeout",
			config:    withReadTimeout(validMinimalConfig(), -1*time.Second),
			wantError: true,
		},
		{
			name:      "empty storage type",
			config:    withStorageType(validMinimalConfig(), ""),
			wantError: true,
		},
		{
			name:      "invalid storage type",
			config:    withStorageType(validMinimalConfig(), "mongodb"),
			wantError: true,
		},
		{
			name:      "empty data source",
			config:    withDataSource(validMinimalConfig(), ""),
			wantError: true,
		},
		{
			name:      "empty API token",
			config:    withAPIToken(validMinimalConfig(), ""),
			wantError: true,
		},
		{
			name:      "short API token",
			config:    withAPIToken(validMinimalConfig(), "short"),
			wantError: true,
		},
		{
			name:      "invalid log level",
			config:    withLogLevel(validMinimalConfig(), "verbose"),
			wantError: true,
		},
		{
			name:      "invalid log format",
			config:    withLogFormat(validMinimalConfig(), "xml"),
			wantError: true,
		},
		{
			name:      "duplicate workspace IDs",
			config:    withDuplicateWorkspaces(validMinimalConfig()),
			wantError: true,
		},
		{
			name:      "workspace with relative path",
			config:    withWorkspaceRelativePath(validMinimalConfig()),
			wantError: true,
		},
		{
			name:      "negative max_open_conns",
			config:    withMaxOpenConns(validMinimalConfig(), -1),
			wantError: true,
		},
		{
			name:      "max_idle_conns exceeds max_open_conns",
			config:    withIdleConns(validMinimalConfig(), 5, 2),
			wantError: true,
		},
		{
			name:      "token expiry too short",
			config:    withTokenExpiry(validMinimalConfig(), 30*time.Second),
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantError && err == nil {
				t.Error("expected validation error, got nil")
			}
			if !tt.wantError && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
		})
	}
}

func TestServerConfigAddress(t *testing.T) {
	cfg := ServerConfig{Host: "127.0.0.1", Port: 8080}
	addr := cfg.Address()
	if addr != "127.0.0.1:8080" {
		t.Errorf("expected 127.0.0.1:8080, got %s", addr)
	}
}

func TestStorageConfigEnsureDir(t *testing.T) {
	tmpDir := t.TempDir()

	// SQLite with subdirectory
	cfg := StorageConfig{
		Type:         "sqlite",
		DataSource:   filepath.Join(tmpDir, "data", "car.db"),
		MaxOpenConns: 1,
		MaxIdleConns: 1,
	}

	err := cfg.EnsureDir()
	if err != nil {
		t.Errorf("EnsureDir failed: %v", err)
	}

	// Check directory was created
	dir := filepath.Dir(cfg.DataSource)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Errorf("expected directory %s to exist", dir)
	}

	// Postgres should not create directories
	cfg.Type = "postgres"
	cfg.DataSource = "postgres://user:pass@localhost/db"
	err = cfg.EnsureDir()
	if err != nil {
		t.Errorf("EnsureDir should be no-op for postgres: %v", err)
	}
}

// Helper functions for building test configs

func validMinimalConfig() *Config {
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
			DataSource:   "/tmp/car.db",
			MaxOpenConns: 1,
			MaxIdleConns: 1,
		},
		Security: SecurityConfig{
			APIToken:    "this-is-a-valid-token-12345",
			TokenExpiry: Duration(24 * time.Hour),
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "text",
			Output: "stdout",
		},
	}
}

func validFullConfig() *Config {
	cfg := validMinimalConfig()
	cfg.Workspaces = []WorkspaceConfig{
		{
			ID:              "ws-1",
			DisplayName:     "Main Project",
			Path:            "/home/user/projects/main",
			AllowedAdapters: []string{"claude-code"},
			ExecutionPolicy: ExecutionPolicy{
				AllowNetworkAccess: false,
				AllowWrites:        true,
			},
		},
	}
	return cfg
}

func withHost(cfg *Config, host string) *Config {
	c := *cfg
	c.Server.Host = host
	return &c
}

func withPort(cfg *Config, port int) *Config {
	c := *cfg
	c.Server.Port = port
	return &c
}

func withReadTimeout(cfg *Config, d time.Duration) *Config {
	c := *cfg
	c.Server.ReadTimeout = Duration(d)
	return &c
}

func withStorageType(cfg *Config, typ string) *Config {
	c := *cfg
	c.Storage.Type = typ
	return &c
}

func withDataSource(cfg *Config, ds string) *Config {
	c := *cfg
	c.Storage.DataSource = ds
	return &c
}

func withAPIToken(cfg *Config, token string) *Config {
	c := *cfg
	c.Security.APIToken = token
	return &c
}

func withLogLevel(cfg *Config, level string) *Config {
	c := *cfg
	c.Logging.Level = level
	return &c
}

func withLogFormat(cfg *Config, format string) *Config {
	c := *cfg
	c.Logging.Format = format
	return &c
}

func withDuplicateWorkspaces(cfg *Config) *Config {
	c := *cfg
	c.Workspaces = []WorkspaceConfig{
		{ID: "ws-1", DisplayName: "First", Path: "/tmp/ws1"},
		{ID: "ws-1", DisplayName: "Duplicate", Path: "/tmp/ws2"},
	}
	return &c
}

func withWorkspaceRelativePath(cfg *Config) *Config {
	c := *cfg
	c.Workspaces = []WorkspaceConfig{
		{ID: "ws-1", DisplayName: "Relative", Path: "relative/path"},
	}
	return &c
}

func withMaxOpenConns(cfg *Config, n int) *Config {
	c := *cfg
	c.Storage.MaxOpenConns = n
	return &c
}

func withIdleConns(cfg *Config, idle, max int) *Config {
	c := *cfg
	c.Storage.MaxIdleConns = idle
	c.Storage.MaxOpenConns = max
	return &c
}

func withTokenExpiry(cfg *Config, d time.Duration) *Config {
	c := *cfg
	c.Security.TokenExpiry = Duration(d)
	return &c
}
