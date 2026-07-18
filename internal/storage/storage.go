package storage

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

// DB wraps the sql.DB connection and provides access to repositories.
type DB struct {
	*sql.DB
}

// Open opens a database connection based on the storage config.
// For SQLite, dataSource is a file path or ":memory:".
// For Postgres, dataSource is a connection string.
// The driver name "sqlite" is mapped to the go-sqlite3 driver name.
func Open(driver, dataSource string) (*DB, error) {
	if driver == "sqlite" {
		driver = "sqlite3"
	}
	db, err := sql.Open(driver, dataSource)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Apply SQLite pragmas for durability and concurrency.
	// These are safe no-ops for other drivers when passed through.
	if driver == "sqlite3" {
		// For in-memory databases, force a single shared connection so that
		// concurrent goroutines see the same tables (each connection to
		// ":memory:" otherwise gets its own private database).
		if dataSource == ":memory:" {
			db.SetMaxOpenConns(1)
		}
		if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
			db.Close()
			return nil, fmt.Errorf("setting journal_mode: %w", err)
		}
		if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
			db.Close()
			return nil, fmt.Errorf("enabling foreign keys: %w", err)
		}
		if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
			db.Close()
			return nil, fmt.Errorf("setting busy timeout: %w", err)
		}
	}

	// Test connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("connecting to database: %w", err)
	}

	wrapped := &DB{db}

	// Run migrations
	if err := Migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	return wrapped, nil
}

// Close closes the database connection.
func (db *DB) Close() error {
	return db.DB.Close()
}

// BeginTx starts a new transaction.
func (db *DB) BeginTx() (*sql.Tx, error) {
	return db.DB.Begin()
}

// WithTransaction executes a function within a database transaction.
// The function is called with the transaction, and the transaction is
// automatically committed or rolled back based on the function's error return.
func (db *DB) WithTransaction(fn func(*sql.Tx) error) error {
	tx, err := db.BeginTx()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("rollback error: %v; original error: %w", rbErr, err)
		}
		return err
	}

	return tx.Commit()
}
