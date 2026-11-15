package database

import (
	"database/sql"
	"fmt"
	"io"
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

// DataDirFn is the seam for the launcher data directory (same pattern as
// services.ScanGameDirs): tests point it at a temporary directory.
var DataDirFn = defaultDataDir

// DataDir returns the directory holding the launcher's persisted data
// (database, token fallback file, bundled-agent cache). The default is the
// directory of the running executable so the whole launcher folder stays
// portable (ADR-0010); a non-writable executable directory (e.g. a system
// install location) falls back to ~/.hrpml.
func DataDir() string { return DataDirFn() }

var defaultDataDir = sync.OnceValue(func() string {
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		if dirWritable(dir) {
			return dir
		}
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".hrpml")
})

func dirWritable(dir string) bool {
	f, err := os.CreateTemp(dir, ".samuel-write-probe-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}

// DefaultConfig returns default database configuration
func DefaultConfig() *Config {
	return &Config{
		Path: filepath.Join(DataDir(), "data.db"),
	}
}

// AdoptLegacyData moves persisted data from the pre-portability location
// (~/.hrpml) into dir, once per file: a file already present in dir is never
// overwritten. Best-effort — a failure leaves the legacy copy in place. The
// SQLite sidecar files move with the database so a WAL left by a crash stays
// recoverable.
func AdoptLegacyData(dir string) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	legacy := filepath.Join(home, ".hrpml")
	if legacy == dir {
		return
	}
	for _, name := range []string{"data.db", "data.db-wal", "data.db-shm", "authlib-injector.jar"} {
		src := filepath.Join(legacy, name)
		dst := filepath.Join(dir, name)
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		if _, err := os.Stat(src); err != nil {
			continue
		}
		if err := os.Rename(src, dst); err != nil {
			// Cross-device move: copy, then drop the source.
			if err := copyFile(src, dst); err != nil {
				continue
			}
			_ = os.Remove(src)
		}
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
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
