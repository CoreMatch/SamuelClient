package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemoveLoaderProfileKeepsParentAndSharedFiles(t *testing.T) {
	gameDir := t.TempDir()
	versionID := "fabric-loader-0.15.0-1.99"
	loaderDir := filepath.Join(gameDir, "versions", versionID)
	parentDir := filepath.Join(gameDir, "versions", "1.99")
	library := filepath.Join(gameDir, "libraries", "net", "fabricmc", "fabric-loader.jar")

	for _, dir := range []string{loaderDir, parentDir, filepath.Dir(library)} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(loaderDir, versionID+".json"), []byte(`{"id":"fabric-loader-0.15.0-1.99","inheritsFrom":"1.99"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parentDir, "1.99.jar"), []byte("minecraft"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(library, []byte("shared loader library"), 0644); err != nil {
		t.Fatal(err)
	}
	profiles := `{"profiles":{"fabric":{"lastVersionId":"fabric-loader-0.15.0-1.99"},"other":{"lastVersionId":"1.99"}},"settings":{"foo":"bar"}}`
	if err := os.WriteFile(filepath.Join(gameDir, "launcher_profiles.json"), []byte(profiles), 0644); err != nil {
		t.Fatal(err)
	}

	result := RemoveLoaderProfile(gameDir, versionID)
	if !result.Success {
		t.Fatalf("RemoveLoaderProfile failed: %s", result.Error)
	}
	if _, err := os.Stat(loaderDir); !os.IsNotExist(err) {
		t.Errorf("loader profile still exists (err=%v)", err)
	}
	for _, path := range []string{filepath.Join(parentDir, "1.99.jar"), library} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("shared or parent file was removed: %s (%v)", path, err)
		}
	}
	raw, err := os.ReadFile(filepath.Join(gameDir, "launcher_profiles.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(raw); !containsAll(got, `"other"`, `"settings"`, `"foo"`) || containsAll(got, `"fabric"`) {
		t.Errorf("launcher_profiles.json = %s, want only matching profile removed and settings preserved", raw)
	}
}

func TestRemoveLoaderProfileRejectsVanillaAndTraversal(t *testing.T) {
	gameDir := t.TempDir()
	vanillaDir := filepath.Join(gameDir, "versions", "1.99")
	if err := os.MkdirAll(vanillaDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vanillaDir, "1.99.json"), []byte(`{"id":"1.99"}`), 0644); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"1.99", "../1.99"} {
		if result := RemoveLoaderProfile(gameDir, id); result.Success {
			t.Errorf("RemoveLoaderProfile(%q) unexpectedly succeeded", id)
		}
	}
	if _, err := os.Stat(vanillaDir); err != nil {
		t.Fatalf("vanilla profile was removed: %v", err)
	}
}

func containsAll(value string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}
