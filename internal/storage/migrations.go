// Package storage provides database access and repositories for CAR.
package storage

import (
	"database/sql"
	"fmt"
)

// Migration represents a database migration.
type Migration struct {
	ID   int
	Name string
	Up   string
	Down string
}

// .Migrations returns all migrations in order.
var Migrations = []Migration{
	{
		ID:   1,
		Name: "create_schema",
		Up: `
			-- Workspaces table
			CREATE TABLE IF NOT EXISTS workspaces (
				id TEXT PRIMARY KEY,
				display_name TEXT NOT NULL,
				path TEXT NOT NULL UNIQUE,
				allowed_adapters TEXT DEFAULT '[]',
				execution_policy TEXT DEFAULT '{}',
				created_at DATETIME NOT NULL DEFAULT (datetime('now')),
				updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
			);

			-- Sessions table
			CREATE TABLE IF NOT EXISTS sessions (
				id TEXT PRIMARY KEY,
				workspace_id TEXT NOT NULL REFERENCES workspaces(id),
				adapter_id TEXT NOT NULL,
				state TEXT NOT NULL DEFAULT 'created',
				title TEXT,
				created_at DATETIME NOT NULL DEFAULT (datetime('now')),
				updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
				last_activity_at DATETIME,
				last_sequence INTEGER NOT NULL DEFAULT 0,
				pending_approval_id TEXT,
				recovery_state TEXT
			);

			-- Runs table
			CREATE TABLE IF NOT EXISTS runs (
				id TEXT PRIMARY KEY,
				session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
				state TEXT NOT NULL DEFAULT 'pending',
				process_pid INTEGER,
				process_args TEXT,
				started_at DATETIME,
				ended_at DATETIME,
				exit_code INTEGER,
				exit_error TEXT,
				created_at DATETIME NOT NULL DEFAULT (datetime('now'))
			);

			-- Events table (journal)
			CREATE TABLE IF NOT EXISTS events (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
				sequence INTEGER NOT NULL,
				type TEXT NOT NULL,
				message_id TEXT NOT NULL UNIQUE,
				schema_version INTEGER NOT NULL DEFAULT 1,
				payload TEXT NOT NULL,
				occurred_at DATETIME NOT NULL DEFAULT (datetime('now')),
				UNIQUE(session_id, sequence)
			);

			-- Approvals table
			CREATE TABLE IF NOT EXISTS approvals (
				id TEXT PRIMARY KEY,
				session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
				category TEXT NOT NULL,
				state TEXT NOT NULL DEFAULT 'pending',
				action_kind TEXT NOT NULL,
				human_readable_context TEXT NOT NULL,
				structured_payload TEXT NOT NULL,
				created_at DATETIME NOT NULL DEFAULT (datetime('now')),
				expires_at DATETIME NOT NULL,
				decided_at DATETIME,
				decision_reason TEXT
			);

			-- Audit log table
			CREATE TABLE IF NOT EXISTS audit_log (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				actor_id TEXT NOT NULL,
				actor_type TEXT NOT NULL,
				action TEXT NOT NULL,
				target_type TEXT NOT NULL,
				target_id TEXT NOT NULL,
				outcome TEXT NOT NULL,
				context TEXT,
				created_at DATETIME NOT NULL DEFAULT (datetime('now'))
			);

			-- Indexes for performance
			CREATE INDEX IF NOT EXISTS idx_sessions_workspace ON sessions(workspace_id);
			CREATE INDEX IF NOT EXISTS idx_sessions_state ON sessions(state);
			CREATE INDEX IF NOT EXISTS idx_runs_session ON runs(session_id);
			CREATE INDEX IF NOT EXISTS idx_runs_state ON runs(state);
			CREATE INDEX IF NOT EXISTS idx_events_session_sequence ON events(session_id, sequence);
			CREATE INDEX IF NOT EXISTS idx_events_session ON events(session_id);
			CREATE INDEX IF NOT EXISTS idx_approvals_session ON approvals(session_id);
			CREATE INDEX IF NOT EXISTS idx_approvals_state ON approvals(state);
			CREATE INDEX IF NOT EXISTS idx_audit_actor ON audit_log(actor_id, actor_type);
			CREATE INDEX IF NOT EXISTS idx_audit_created ON audit_log(created_at);
		`,
		Down: `
			DROP INDEX IF EXISTS idx_audit_created;
			DROP INDEX IF EXISTS idx_audit_actor;
			DROP INDEX IF EXISTS idx_approvals_state;
			DROP INDEX IF EXISTS idx_approvals_session;
			DROP INDEX IF EXISTS idx_events_session;
			DROP INDEX IF EXISTS idx_events_session_sequence;
			DROP INDEX IF EXISTS idx_runs_state;
			DROP INDEX IF EXISTS idx_runs_session;
			DROP INDEX IF EXISTS idx_sessions_state;
			DROP INDEX IF EXISTS idx_sessions_workspace;
			DROP TABLE IF EXISTS audit_log;
			DROP TABLE IF EXISTS approvals;
			DROP TABLE IF EXISTS events;
			DROP TABLE IF EXISTS runs;
			DROP TABLE IF EXISTS sessions;
			DROP TABLE IF EXISTS workspaces;
		`,
	},
	{
		ID:   2,
		Name: "add_device_auth",
		Up: `
			-- Devices table for authenticated clients
			CREATE TABLE IF NOT EXISTS devices (
				id TEXT PRIMARY KEY,
				name TEXT NOT NULL,
				public_key TEXT NOT NULL UNIQUE,
				state TEXT NOT NULL DEFAULT 'pending',
				paired_at DATETIME,
				revoked_at DATETIME,
				last_seen_at DATETIME,
				created_at DATETIME NOT NULL DEFAULT (datetime('now'))
			);

			-- Sessions table: add owner device
			ALTER TABLE sessions ADD COLUMN owner_device_id TEXT REFERENCES devices(id);

			-- Index for device lookups
			CREATE INDEX IF NOT EXISTS idx_devices_state ON devices(state);
		`,
		Down: `
			DROP INDEX IF EXISTS idx_devices_state;
			ALTER TABLE sessions DROP COLUMN owner_device_id;
			DROP TABLE IF EXISTS devices;
		`,
	},
}

// Migrate runs all pending migrations on the database.
func Migrate(db *sql.DB) error {
	// Create migrations tracking table if not exists
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)
	`)
	if err != nil {
		return fmt.Errorf("creating migrations table: %w", err)
	}

	// Get current version
	var currentVersion int
	err = db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&currentVersion)
	if err != nil {
		return fmt.Errorf("reading current version: %w", err)
	}

	// Apply pending migrations
	for _, m := range Migrations {
		if m.ID <= currentVersion {
			continue
		}

		// Run migration in transaction
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("starting transaction for migration %d: %w", m.ID, err)
		}

		// Execute UP migration
		if _, err := tx.Exec(m.Up); err != nil {
			tx.Rollback()
			return fmt.Errorf("applying migration %d (%s): %w", m.ID, m.Name, err)
		}

		// Record migration
		if _, err := tx.Exec("INSERT INTO schema_migrations (version, name) VALUES (?, ?)", m.ID, m.Name); err != nil {
			tx.Rollback()
			return fmt.Errorf("recording migration %d: %w", m.ID, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing migration %d: %w", m.ID, err)
		}
	}

	return nil
}

// Rollback rolls back the last migration.
func Rollback(db *sql.DB) error {
	var currentVersion int
	err := db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&currentVersion)
	if err != nil {
		return fmt.Errorf("reading current version: %w", err)
	}

	if currentVersion == 0 {
		return fmt.Errorf("no migrations to roll back")
	}

	// Find the migration to roll back
	var target Migration
	for _, m := range Migrations {
		if m.ID == currentVersion {
			target = m
			break
		}
	}

	if target.ID == 0 {
		return fmt.Errorf("migration %d not found", currentVersion)
	}

	// Run rollback in transaction
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("starting transaction for rollback: %w", err)
	}

	if _, err := tx.Exec(target.Down); err != nil {
		tx.Rollback()
		return fmt.Errorf("rolling back migration %d: %w", currentVersion, err)
	}

	if _, err := tx.Exec("DELETE FROM schema_migrations WHERE version = ?", currentVersion); err != nil {
		tx.Rollback()
		return fmt.Errorf("removing migration record: %w", err)
	}

	return tx.Commit()
}
