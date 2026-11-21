package services

import (
	"os"
	"path/filepath"
	"testing"

	"HRPMLW/internal/database"
)

// swapDataDir points the launcher data directory at dir for one test.
func swapDataDir(t *testing.T, dir string) {
	t.Helper()
	old := database.DataDirFn
	database.DataDirFn = func() string { return dir }
	t.Cleanup(func() { database.DataDirFn = old })
}

// makeGameDir creates root/name with a versions/ profile per given id (or an
// empty directory when no ids are given).
func makeGameDir(t *testing.T, root, name string, versionIDs ...string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	for _, id := range versionIDs {
		vdir := filepath.Join(dir, "versions", id)
		if err := os.MkdirAll(vdir, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", vdir, err)
		}
		if err := os.WriteFile(filepath.Join(vdir, id+".json"), []byte("{}"), 0644); err != nil {
			t.Fatalf("write version json: %v", err)
		}
	}
	return dir
}

func TestGetMinecraftDirIsPortableDefault(t *testing.T) {
	dataDir := t.TempDir()
	swapDataDir(t, dataDir)

	want := filepath.Join(dataDir, ".minecraft")
	if got := NewLauncherService().GetMinecraftDir(); got != want {
		t.Errorf("GetMinecraftDir = %q, want %q", got, want)
	}
}

func TestResolveGameDirPrefersPersistedOverride(t *testing.T) {
	clearLauncherSettings(t)
	t.Cleanup(func() { clearLauncherSettings(t) })
	root := t.TempDir()
	swapDataDir(t, root)
	override := makeGameDir(t, root, "override", "1.0")

	svc := NewLauncherService()
	if got, want := svc.ResolveGameDir(), filepath.Join(root, ".minecraft"); got != want {
		t.Fatalf("ResolveGameDir without override = %q, want %q", got, want)
	}
	if res := svc.SetGameDir(override); !res.Success {
		t.Fatalf("SetGameDir failed: %s", res.Error)
	}
	if got := svc.ResolveGameDir(); got != override {
		t.Errorf("ResolveGameDir with override = %q, want %q", got, override)
	}
}

func TestGameDirStatus(t *testing.T) {
	clearLauncherSettings(t)
	t.Cleanup(func() { clearLauncherSettings(t) })
	root := t.TempDir()
	swapDataDir(t, root)
	detected := makeGameDir(t, root, ".minecraft", "1.0")
	svc := NewLauncherService()

	status := svc.GameDirStatus()
	if status.Source != "auto" || status.Path != detected || !status.Exists {
		t.Errorf("auto status = %+v, want path %q source auto exists true", status, detected)
	}

	missing := filepath.Join(root, "not-created-yet")
	if res := svc.SetGameDir(missing); !res.Success {
		t.Fatalf("SetGameDir failed: %s", res.Error)
	}
	status = svc.GameDirStatus()
	if status.Source != "custom" || status.Path != missing || status.Exists {
		t.Errorf("custom status = %+v, want path %q source custom exists false", status, missing)
	}
}

func TestGameDirsPreserveLegacyDefaultAndPersistNewFolders(t *testing.T) {
	clearLauncherSettings(t)
	t.Cleanup(func() { clearLauncherSettings(t) })
	root := t.TempDir()
	swapDataDir(t, root)
	legacy := makeGameDir(t, root, "legacy")
	added := filepath.Join(root, "new")
	if err := saveLauncherSetting(settingKeyGameDir, legacy, "string", "Legacy game directory"); err != nil {
		t.Fatal(err)
	}
	svc := NewLauncherService()
	for _, path := range []string{added, added + string(filepath.Separator)} {
		if res := svc.AddGameDir(path); !res.Success {
			t.Fatal(res.Error)
		}
	}
	status := NewLauncherService().GameDirStatus()
	if status.Path != legacy || len(status.Dirs) != 3 {
		t.Fatalf("expected portable, legacy, and added folders with legacy default: %+v", status)
	}
	if res := svc.RemoveGameDir(legacy); res.Success {
		t.Fatal("default folder should not be removable before switching")
	}
	if res := svc.SetGameDir(added); !res.Success {
		t.Fatal(res.Error)
	}
	if res := svc.RemoveGameDir(legacy); !res.Success {
		t.Fatal(res.Error)
	}
	status = NewLauncherService().GameDirStatus()
	if status.Path != added || len(status.Dirs) != 2 {
		t.Fatalf("expected the new default and built-in folder after removal: %+v", status)
	}
}

func TestSetGameDirValidationAndClear(t *testing.T) {
	clearLauncherSettings(t)
	t.Cleanup(func() { clearLauncherSettings(t) })
	home := t.TempDir()
	t.Setenv("HOME", home)
	swapDataDir(t, home)
	svc := NewLauncherService()

	if res := svc.SetGameDir("relative/path"); res.Success {
		t.Error("relative path must be rejected")
	}

	if res := svc.SetGameDir("~/games/mc"); !res.Success {
		t.Fatalf("SetGameDir(~/games/mc) failed: %s", res.Error)
	}
	want := filepath.Join(home, "games", "mc")
	if status := svc.GameDirStatus(); status.Path != want || status.Source != "custom" {
		t.Errorf("tilde expansion = %+v, want path %q source custom", status, want)
	}

	// Empty clears the override: back to the portable default.
	if res := svc.SetGameDir(""); !res.Success {
		t.Fatalf("SetGameDir(\"\") failed: %s", res.Error)
	}
	if status := svc.GameDirStatus(); status.Source != "auto" {
		t.Errorf("after clear source = %q, want auto", status.Source)
	}
}

// swapScanSeams points the disk walk at a temporary root and restores the
// production seams afterwards.
func swapScanSeams(t *testing.T, roots []scanRoot, budget int) {
	t.Helper()
	oldRoots, oldBudget := scanGameDirRoots, scanMaxVisited
	scanGameDirRoots = func() []scanRoot { return roots }
	scanMaxVisited = budget
	t.Cleanup(func() { scanGameDirRoots, scanMaxVisited = oldRoots, oldBudget })
}

func TestScanGameDirsFindsAndCounts(t *testing.T) {
	root := t.TempDir()
	makeGameDir(t, root, ".minecraft", "1.0", "2.0")
	makeGameDir(t, filepath.Join(root, "Games", "mc"), ".minecraft", "3.0")
	// launcher_profiles.json qualifies a directory regardless of its name.
	custom := filepath.Join(root, "MyMC")
	if err := os.MkdirAll(custom, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(custom, "launcher_profiles.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	// A minecraft-named dir with only options.txt (no versions) still counts.
	opts := filepath.Join(root, "minecraft")
	if err := os.MkdirAll(opts, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(opts, "options.txt"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	swapScanSeams(t, []scanRoot{{path: root, depth: 5}}, 10000)

	got := scanGameDirs()
	byPath := map[string]GameDirCandidate{}
	for _, c := range got {
		byPath[c.Path] = c
	}
	if len(got) != 4 {
		t.Fatalf("scan found %d dirs, want 4: %+v", len(got), got)
	}
	if c := byPath[filepath.Join(root, ".minecraft")]; c.VersionCount != 2 {
		t.Errorf(".minecraft version_count = %d, want 2", c.VersionCount)
	}
	if c := byPath[filepath.Join(root, "Games", "mc", ".minecraft")]; c.VersionCount != 1 {
		t.Errorf("nested version_count = %d, want 1", c.VersionCount)
	}
	if _, ok := byPath[custom]; !ok {
		t.Errorf("launcher_profiles.json dir %q not found", custom)
	}
	if c := byPath[opts]; c.VersionCount != 0 {
		t.Errorf("options.txt-only dir version_count = %d, want 0", c.VersionCount)
	}
	// Sorted by path.
	for i := 1; i < len(got); i++ {
		if got[i-1].Path >= got[i].Path {
			t.Errorf("results not sorted by path: %+v", got)
			break
		}
	}
}

func TestScanGameDirsSkipsPrunedAndDecoyDirs(t *testing.T) {
	root := t.TempDir()
	// Inside a pruned dependency tree: must not be reported.
	makeGameDir(t, filepath.Join(root, "node_modules", "pkg"), ".minecraft", "1.0")
	// A random versions/ folder without game-dir markers: must not qualify.
	if err := os.MkdirAll(filepath.Join(root, "project", "versions", "v1"), 0755); err != nil {
		t.Fatal(err)
	}
	// An empty .minecraft leftover: must not qualify.
	makeGameDir(t, root, ".minecraft")
	swapScanSeams(t, []scanRoot{{path: root, depth: 5}}, 10000)

	if got := scanGameDirs(); len(got) != 0 {
		t.Errorf("scan found %d dirs, want 0: %+v", len(got), got)
	}
}

func TestScanGameDirsRespectsDepthAndBudget(t *testing.T) {
	root := t.TempDir()
	makeGameDir(t, root, ".minecraft", "1.0")
	makeGameDir(t, filepath.Join(root, "a", "b"), ".minecraft", "2.0")

	// Depth 1 reaches root/.minecraft but not root/a/b/.minecraft (depth 3).
	swapScanSeams(t, []scanRoot{{path: root, depth: 1}}, 10000)
	got := scanGameDirs()
	if len(got) != 1 || got[0].Path != filepath.Join(root, ".minecraft") {
		t.Errorf("depth-limited scan = %+v, want only root/.minecraft", got)
	}

	// A budget of 1 visits only the root itself, which is not a game dir.
	swapScanSeams(t, []scanRoot{{path: root, depth: 5}}, 1)
	if got := scanGameDirs(); len(got) != 0 {
		t.Errorf("budget-limited scan = %+v, want none", got)
	}
}

func TestScanGameDirsDeduplicatesOverlappingRoots(t *testing.T) {
	root := t.TempDir()
	makeGameDir(t, root, ".minecraft", "1.0")
	swapScanSeams(t, []scanRoot{{path: root, depth: 5}, {path: root, depth: 5}}, 10000)

	if got := scanGameDirs(); len(got) != 1 {
		t.Errorf("overlapping roots produced %d entries, want 1: %+v", len(got), got)
	}
}
