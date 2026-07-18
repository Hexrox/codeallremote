package storage

import (
	"database/sql"
	"testing"
)

func TestMigrations_Apply(t *testing.T) {
	db, err := Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Verify tables exist after migration
	tables := []string{"workspaces", "sessions", "runs", "events", "approvals", "audit_log", "schema_migrations"}

	for _, table := range tables {
		var exists string
		err := db.QueryRow(`
			SELECT name FROM sqlite_master WHERE type='table' AND name=?
		`, table).Scan(&exists)

		if err != nil {
			t.Errorf("table %s does not exist: %v", table, err)
		}
	}
}

func TestMigrations_Indexes(t *testing.T) {
	db, err := Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	indexes := []string{
		"idx_sessions_workspace",
		"idx_sessions_state",
		"idx_runs_session",
		"idx_events_session_sequence",
		"idx_approvals_state",
	}

	for _, idx := range indexes {
		var exists string
		err := db.QueryRow(`
			SELECT name FROM sqlite_master WHERE type='index' AND name=?
		`, idx).Scan(&exists)

		if err != nil {
			t.Errorf("index %s does not exist: %v", idx, err)
		}
	}
}

func TestMigrations_Rollback(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Apply migrations manually for testing
	if err := Migrate(db); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	// Rollback
	if err := Rollback(db); err != nil {
		t.Fatalf("failed to rollback: %v", err)
	}

	// Verify version decreased
	var version int
	err = db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&version)
	if err != nil {
		t.Fatalf("failed to read version: %v", err)
	}

	if version != len(Migrations)-1 {
		t.Errorf("expected version %d after rollback, got %d", len(Migrations)-1, version)
	}
}

func TestMigrations_Idempotency(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Apply migrations twice - should not error
	if err := Migrate(db); err != nil {
		t.Fatalf("first migration failed: %v", err)
	}

	if err := Migrate(db); err != nil {
		t.Fatalf("second migration failed: %v", err)
	}

	// Verify version is correct
	var version int
	err = db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&version)
	if err != nil {
		t.Fatalf("failed to read version: %v", err)
	}

	if version != len(Migrations) {
		t.Errorf("expected version %d, got %d", len(Migrations), version)
	}
}
