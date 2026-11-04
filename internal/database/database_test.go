package database

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// The package uses sync.Once, so exactly one Initialize can happen per test
// binary. TestMain creates a single temporary database that every test in
// this package shares; tests must not DROP tables or rely on empty state
// beyond what they set up themselves.
var testDBPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "samuelclient-dbtest")
	if err != nil {
		os.Exit(1)
	}
	testDBPath = filepath.Join(dir, "test.db")
	if err := Initialize(&Config{Path: testDBPath}); err != nil {
		os.Exit(1)
	}
	code := m.Run()
	_ = Close()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func TestInitializeCreatedDatabaseFile(t *testing.T) {
	if _, err := os.Stat(testDBPath); err != nil {
		t.Fatalf("database file was not created at %q: %v", testDBPath, err)
	}
	if GetDB() == nil {
		t.Fatal("GetDB() returned nil after successful Initialize")
	}
}

func TestInitializeIsIdempotent(t *testing.T) {
	// A second Initialize call within the same process is a no-op (sync.Once)
	// and must not fail.
	if err := Initialize(&Config{Path: testDBPath}); err != nil {
		t.Fatalf("second Initialize call failed: %v", err)
	}
	if GetDB() == nil {
		t.Fatal("GetDB() nil after idempotent Initialize")
	}
}

func TestMigrationsCreateExpectedTables(t *testing.T) {
	rows, err := GetDB().Query(`SELECT name FROM sqlite_master WHERE type = 'table'`)
	if err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	defer rows.Close()

	got := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got[name] = true
	}

	for _, table := range []string{"settings", "java_installations"} {
		if !got[table] {
			t.Errorf("expected table %q to be created by migrations", table)
		}
	}
}

func TestMigrationsDoNotCreateLogsTable(t *testing.T) {
	// Regression for the 2026-11-05 retirement: fresh databases must no
	// longer contain a logs table (existing deployments keep theirs inert).
	var count int
	err := GetDB().QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'logs'`).Scan(&count)
	if err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	if count != 0 {
		t.Errorf("logs table was created on a fresh database; expected it to be retired")
	}
}

func TestMigrationsDoNotCreateRetiredTables(t *testing.T) {
	// Regression for the 2026-11-05 retirement of the user/task/file
	// repositories: fresh databases must no longer create these tables.
	// Existing deployments keep theirs inert on disk (no DROP migration).
	for _, table := range []string{"users", "tasks", "files"} {
		var count int
		err := GetDB().QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&count)
		if err != nil {
			t.Fatalf("query sqlite_master for %q: %v", table, err)
		}
		if count != 0 {
			t.Errorf("retired table %q was created on a fresh database", table)
		}
	}
}

func TestMigrationsCreateExpectedIndexes(t *testing.T) {
	rows, err := GetDB().Query(`SELECT name FROM sqlite_master WHERE type = 'index' AND name LIKE 'idx_%'`)
	if err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	defer rows.Close()

	got := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got[name] = true
	}

	for _, idx := range []string{
		"idx_settings_key", "idx_java_path", "idx_java_default",
	} {
		if !got[idx] {
			t.Errorf("expected index %q to be created by migrations", idx)
		}
	}
}

func TestDefaultConfigPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("cannot resolve home directory: %v", err)
	}

	cfg := DefaultConfig()
	want := filepath.Join(home, ".hrpml", "data.db")
	if cfg.Path != want {
		t.Errorf("DefaultConfig().Path = %q, want %q", cfg.Path, want)
	}
}

func TestTransactionCommitAndRollback(t *testing.T) {
	// Commit path: the insert survives.
	err := Transaction(func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO settings (key, value, type) VALUES ('tx_commit', '1', 'string')`)
		return err
	})
	if err != nil {
		t.Fatalf("commit transaction failed: %v", err)
	}
	var count int
	if err := GetDB().QueryRow(`SELECT COUNT(*) FROM settings WHERE key = 'tx_commit'`).Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != 1 {
		t.Errorf("committed row missing: count = %d, want 1", count)
	}

	// Rollback path: returning an error discards the insert.
	wantErr := errors.New("boom")
	err = Transaction(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO settings (key, value, type) VALUES ('tx_rollback', '1', 'string')`); err != nil {
			return err
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Transaction returned %v, want the propagated error %v", err, wantErr)
	}
	if err := GetDB().QueryRow(`SELECT COUNT(*) FROM settings WHERE key = 'tx_rollback'`).Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != 0 {
		t.Errorf("rolled-back row survived: count = %d, want 0", count)
	}
}
