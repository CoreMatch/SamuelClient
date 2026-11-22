package services

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestModLifecycle(t *testing.T) {
	gameDir := filepath.Join(t.TempDir(), ".minecraft")
	modsDir := filepath.Join(gameDir, "mods")
	source := filepath.Join(t.TempDir(), "Example.jar")
	writeModFixture(t, source, "mod")

	assertMods := func(want ...ModFile) {
		t.Helper()
		got, err := ListMods(gameDir)
		if err != nil || !slices.Equal(got, want) {
			t.Fatalf("ListMods = %v, %v; want %v", got, err, want)
		}
	}
	assertMods()
	if _, err := os.Stat(modsDir); !os.IsNotExist(err) {
		t.Fatalf("listing should not create the mods folder: %v", err)
	}
	if err := AddMod(gameDir, source); err != nil {
		t.Fatal(err)
	}
	writeModFixture(t, filepath.Join(modsDir, "readme.txt"), "ignored")
	assertMods(ModFile{Name: "Example.jar", Enabled: true})
	if err := SetModEnabled(gameDir, "Example.jar", false); err != nil {
		t.Fatal(err)
	}
	assertMods(ModFile{Name: "Example.jar.bak", Enabled: false})
	if err := AddMod(gameDir, source); err == nil {
		t.Fatal("import must not create an enabled duplicate of a disabled mod")
	}
	if err := SetModEnabled(gameDir, "Example.jar.bak", true); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(modsDir, "Example.jar")); err != nil || string(got) != "mod" {
		t.Fatalf("restored mod = %q, %v", got, err)
	}
	if err := DeleteMod(gameDir, "Example.jar"); err != nil {
		t.Fatal(err)
	}
	assertMods()
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("source file should be untouched: %v", err)
	}
}

func TestModConflictAndInvalidTargets(t *testing.T) {
	gameDir := t.TempDir()
	modsDir := filepath.Join(gameDir, "mods")
	if err := os.Mkdir(modsDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeModFixture(t, filepath.Join(modsDir, "example.jar"), "enabled")
	writeModFixture(t, filepath.Join(modsDir, "example.jar.bak"), "disabled")
	writeModFixture(t, filepath.Join(gameDir, "outside.jar"), "outside")
	if err := os.Mkdir(filepath.Join(modsDir, "directory.jar"), 0755); err != nil {
		t.Fatal(err)
	}

	if err := SetModEnabled(gameDir, "example.jar", false); err == nil {
		t.Fatal("disabling must not replace an existing backup")
	}
	if err := SetModEnabled(gameDir, "example.jar.bak", true); err == nil {
		t.Fatal("enabling must not replace an existing mod")
	}
	for _, name := range []string{"../outside.jar", `..\outside.jar`, "directory.jar", "notes.txt"} {
		if err := DeleteMod(gameDir, name); err == nil {
			t.Fatalf("DeleteMod should reject %q", name)
		}
	}
	for name, want := range map[string]string{"example.jar": "enabled", "example.jar.bak": "disabled", "../outside.jar": "outside"} {
		if got, err := os.ReadFile(filepath.Join(modsDir, name)); err != nil || string(got) != want {
			t.Errorf("%s = %q, %v; want %q", name, got, err, want)
		}
	}
}

func writeModFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
