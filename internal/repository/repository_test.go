package repository

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"HRPMLW/internal/database"
	"HRPMLW/internal/models"
)

// The database package guards Initialize with sync.Once, so the whole test
// binary shares one temporary database created here. Tests use unique field
// values and clean up after themselves instead of assuming empty tables.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "samuelclient-repotest")
	if err != nil {
		os.Exit(1)
	}
	if err := database.Initialize(&database.Config{Path: filepath.Join(dir, "repo.db")}); err != nil {
		os.Exit(1)
	}
	code := m.Run()
	_ = database.Close()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func TestSettingRepositoryCRUD(t *testing.T) {
	repo := NewSettingRepository()

	if err := repo.Set("repo_test_key", "v1", "string", "test setting"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	t.Cleanup(func() { _ = repo.Delete("repo_test_key") })

	got, err := repo.Get("repo_test_key")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Value != "v1" || got.Type != "string" {
		t.Errorf("Get = %+v, want value v1 / type string", got)
	}

	// Overwrite (Set on an existing key must update, not duplicate).
	if err := repo.Set("repo_test_key", "v2", "string", "test setting"); err != nil {
		t.Fatalf("Set overwrite: %v", err)
	}
	got, err = repo.Get("repo_test_key")
	if err != nil {
		t.Fatalf("Get after overwrite: %v", err)
	}
	if got.Value != "v2" {
		t.Errorf("value after overwrite = %q, want v2", got.Value)
	}

	all, err := repo.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	found := false
	for _, s := range all {
		if s.Key == "repo_test_key" {
			found = true
		}
	}
	if !found {
		t.Error("setting missing from GetAll")
	}

	if err := repo.Delete("repo_test_key"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.Get("repo_test_key"); err == nil {
		t.Error("Get after Delete should fail")
	}
}

func TestJavaRepositoryCRUDAndDefault(t *testing.T) {
	repo := NewJavaRepository()

	first := &models.JavaInstallation{Path: "/opt/jdk-a/bin/java", FriendlyName: "JDK A", Version: "17.0.1"}
	if err := repo.Create(first); err != nil {
		t.Fatalf("Create first: %v", err)
	}
	t.Cleanup(func() { _ = repo.Delete(first.ID) })
	second := &models.JavaInstallation{Path: "/opt/jdk-b/bin/java", FriendlyName: "JDK B", Version: "21.0.2"}
	if err := repo.Create(second); err != nil {
		t.Fatalf("Create second: %v", err)
	}
	t.Cleanup(func() { _ = repo.Delete(second.ID) })

	if _, err := repo.GetByID(first.ID); err != nil {
		t.Fatalf("GetByID: %v", err)
	}

	all, err := repo.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(all) < 2 {
		t.Fatalf("GetAll returned %d rows, want at least 2", len(all))
	}

	// No default yet among these two; setting one makes it the default and
	// clears any previous default.
	if err := repo.SetDefault(first.ID); err != nil {
		t.Fatalf("SetDefault(first): %v", err)
	}
	def, err := repo.GetDefault()
	if err != nil {
		t.Fatalf("GetDefault after SetDefault(first): %v", err)
	}
	if def.ID != first.ID {
		t.Errorf("default ID = %d, want %d", def.ID, first.ID)
	}

	if err := repo.SetDefault(second.ID); err != nil {
		t.Fatalf("SetDefault(second): %v", err)
	}
	def, err = repo.GetDefault()
	if err != nil {
		t.Fatalf("GetDefault after SetDefault(second): %v", err)
	}
	if def.ID != second.ID {
		t.Errorf("default ID = %d, want %d", def.ID, second.ID)
	}

	second.FriendlyName = "JDK B renamed"
	if err := repo.Update(second); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, err := repo.GetByID(second.ID)
	if err != nil {
		t.Fatalf("GetByID after Update: %v", err)
	}
	if got.FriendlyName != "JDK B renamed" {
		t.Errorf("friendly name after Update = %q, want JDK B renamed", got.FriendlyName)
	}

	if err := repo.Delete(second.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.GetByID(second.ID); err == nil {
		t.Error("GetByID after Delete should fail")
	}
}

// Delete on a missing id must fail instead of silently succeeding.
func TestJavaRepositoryDeleteMissingID(t *testing.T) {
	repo := NewJavaRepository()

	err := repo.Delete(999999)
	if err == nil {
		t.Fatal("Delete of a missing id must fail")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("Delete error = %v, want a wrapped sql.ErrNoRows", err)
	}
}

// SetDefault on a missing id must fail and must not clear the existing
// default flag (the transaction has to roll back).
func TestJavaRepositorySetDefaultMissingID(t *testing.T) {
	repo := NewJavaRepository()

	seed := &models.JavaInstallation{Path: "/opt/jdk-c/bin/java", FriendlyName: "JDK C", Version: "21.0.3"}
	if err := repo.Create(seed); err != nil {
		t.Fatalf("Create seed: %v", err)
	}
	t.Cleanup(func() { _ = repo.Delete(seed.ID) })
	if err := repo.SetDefault(seed.ID); err != nil {
		t.Fatalf("SetDefault(seed): %v", err)
	}

	err := repo.SetDefault(999999)
	if err == nil {
		t.Fatal("SetDefault of a missing id must fail")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("SetDefault error = %v, want a wrapped sql.ErrNoRows", err)
	}

	def, err := repo.GetDefault()
	if err != nil {
		t.Fatalf("GetDefault after failed SetDefault: %v", err)
	}
	if def.ID != seed.ID {
		t.Errorf("default ID = %d, want %d (failed SetDefault must not clear the existing default)", def.ID, seed.ID)
	}
}
