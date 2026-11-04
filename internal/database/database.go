package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"
)

var (
	db   *sql.DB
	once sync.Once
)

// Config holds database configuration
type Config struct {
	Path string
}

// DefaultConfig returns default database configuration
func DefaultConfig() *Config {
	homeDir, _ := os.UserHomeDir()
	return &Config{
		Path: filepath.Join(homeDir, ".hrpml", "data.db"),
	}
}

// Initialize creates database connection and runs migrations
func Initialize(cfg *Config) error {
	var initErr error
	once.Do(func() {
		// Ensure directory exists
		dir := filepath.Dir(cfg.Path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			initErr = fmt.Errorf("failed to create database directory: %w", err)
			return
		}

		// Open database connection
		connStr := cfg.Path + "?_journal_mode=WAL&_busy_timeout=5000"
		db, initErr = sql.Open("sqlite", connStr)
		if initErr != nil {
			return
		}

		// Set connection pool settings
		db.SetMaxOpenConns(1) // SQLite recommends single connection
		db.SetMaxIdleConns(1)

		// Test connection
		if initErr = db.Ping(); initErr != nil {
			return
		}

		// Run migrations
		initErr = runMigrations()
	})

	return initErr
}

// GetDB returns the database connection
func GetDB() *sql.DB {
	return db
}

// Close closes the database connection
func Close() error {
	if db != nil {
		return db.Close()
	}
	return nil
}

// runMigrations runs database migrations
func runMigrations() error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS settings (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			key TEXT UNIQUE NOT NULL,
			value TEXT,
			type TEXT DEFAULT 'string',
			description TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS java_installations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			path TEXT NOT NULL,
			friendly_name TEXT,
			version TEXT,
			is_default BOOLEAN DEFAULT FALSE,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_settings_key ON settings(key)`,
		// The logs table and its indexes are no longer created: nothing writes
		// or reads it anymore (live launch logs stream over Wails events,
		// ADR-0004 follow-up). No DROP is issued on purpose — databases that
		// already contain a logs table keep it inert on disk, which avoids any
		// destructive migration.
		//
		// The users, tasks and files tables (and their indexes) are retired the
		// same way: the repositories that used them were deleted because no
		// launcher feature reads or writes them. Fresh databases no longer
		// create them; existing deployments keep their copies inert on disk.
		`CREATE INDEX IF NOT EXISTS idx_java_path ON java_installations(path)`,
		`CREATE INDEX IF NOT EXISTS idx_java_default ON java_installations(is_default)`,
	}

	for _, migration := range migrations {
		if _, err := db.Exec(migration); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}

	return nil
}

// Transaction executes a function within a transaction
func Transaction(fn func(*sql.Tx) error) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}

	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit()
}
