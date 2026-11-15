package database

import (
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
	dir := t.TempDir()
	old := DataDirFn
	DataDirFn = func() string { return dir }
	t.Cleanup(func() { DataDirFn = old })

	cfg := DefaultConfig()
	want := filepath.Join(dir, "data.db")
	if cfg.Path != want {
		t.Errorf("DefaultConfig().Path = %q, want %q", cfg.Path, want)
	}
}

func TestDirWritable(t *testing.T) {
	if !dirWritable(t.TempDir()) {
		t.Error("a fresh temp dir must be reported writable")
	}
	if dirWritable(filepath.Join(t.TempDir(), "does-not-exist")) {
		t.Error("a missing dir must not be reported writable")
	}
}

func TestAdoptLegacyDataMovesFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // os.UserHomeDir reads this on Windows
	legacy := filepath.Join(home, ".hrpml")
	if err := os.MkdirAll(legacy, 0755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"data.db": "legacy-db", "authlib-injector.jar": "legacy-jar"} {
		if err := os.WriteFile(filepath.Join(legacy, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	dir := t.TempDir()
	AdoptLegacyData(dir)

	for name, want := range map[string]string{"data.db": "legacy-db", "authlib-injector.jar": "legacy-jar"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(data) != want {
			t.Errorf("%s not adopted: %q err=%v", name, data, err)
		}
		if _, err := os.Stat(filepath.Join(legacy, name)); !os.IsNotExist(err) {
			t.Errorf("%s still present in legacy dir: %v", name, err)
		}
	}
}

func TestAdoptLegacyDataKeepsExistingTarget(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	legacy := filepath.Join(home, ".hrpml")
	if err := os.MkdirAll(legacy, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "data.db"), []byte("legacy-db"), 0644); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "data.db"), []byte("current-db"), 0644); err != nil {
		t.Fatal(err)
	}

	AdoptLegacyData(dir)

	if data, err := os.ReadFile(filepath.Join(dir, "data.db")); err != nil || string(data) != "current-db" {
		t.Errorf("existing target was overwritten: %q err=%v", data, err)
	}
}

func TestAdoptLegacyDataNoLegacy(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	dir := t.TempDir()

	AdoptLegacyData(dir)

	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Errorf("no-legacy adoption must be a no-op: err=%v entries=%d", err, len(entries))
	}
}
