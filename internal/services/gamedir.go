package services

// Game directory resolution: the portable default, the persisted user
// override, and an on-demand disk scan. Every code path that touches the game
// directory (config display, version scan, install, launch) goes through
// ResolveGameDir so a user-specified directory is honored consistently.

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"HRPMLW/internal/database"
	"HRPMLW/internal/models"
)

// GetMinecraftDir returns the default game directory: .minecraft inside the
// launcher data directory (next to the executable), so downloads stay with
// the portable launcher folder (ADR-0010). Existing installs elsewhere are
// not auto-adopted; the settings UI (ScanGameDirs + SetGameDir) lets the user
// point the launcher at one, and ResolveGameDir honors that override.
func (l *LauncherService) GetMinecraftDir() string {
	return filepath.Join(database.DataDir(), ".minecraft")
}

// ResolveGameDir returns the effective game directory: the persisted user
// override when set, otherwise the auto-detected default.
func (l *LauncherService) ResolveGameDir() string {
	settings, _ := loadLauncherSettings()
	if dir := settings[settingKeyGameDir]; dir != "" {
		return dir
	}
	return l.GetMinecraftDir()
}

// GameDirStatus describes the effective game directory for the settings UI.
type GameDirStatus struct {
	Path   string `json:"path"`
	Source string `json:"source"` // "custom" (user override) or "auto" (detected)
	Exists bool   `json:"exists"`
}

// GameDirStatus reports the effective directory, whether it came from the
// user override or auto-detection, and whether it exists on disk.
func (l *LauncherService) GameDirStatus() GameDirStatus {
	settings, _ := loadLauncherSettings()
	source := "auto"
	path := settings[settingKeyGameDir]
	if path != "" {
		source = "custom"
	} else {
		path = l.GetMinecraftDir()
	}
	fi, err := os.Stat(path)
	return GameDirStatus{Path: path, Source: source, Exists: err == nil && fi.IsDir()}
}

// SetGameDir persists a user-specified game directory override. An empty dir
// clears the override so auto-detection applies again. A leading ~ is
// expanded and relative paths are rejected. The directory does not need to
// exist yet — installing a version creates it.
func (l *LauncherService) SetGameDir(dir string) *models.ApiResponse {
	dir = strings.TrimSpace(dir)
	if dir != "" {
		if dir == "~" || strings.HasPrefix(dir, "~/") || strings.HasPrefix(dir, `~\`) {
			home, _ := os.UserHomeDir()
			dir = filepath.Join(home, strings.TrimPrefix(dir, "~"))
		}
		if !filepath.IsAbs(dir) {
			return &models.ApiResponse{Success: false, Error: "game directory must be an absolute path"}
		}
		dir = filepath.Clean(dir)
	}
	if err := saveLauncherSetting(settingKeyGameDir, dir, "string", "Game directory (.minecraft)"); err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}
	return &models.ApiResponse{Success: true, Data: l.GameDirStatus()}
}

// GameDirCandidate is one game directory found by ScanGameDirs.
type GameDirCandidate struct {
	Path         string `json:"path"`
	VersionCount int    `json:"version_count"`
}

// ScanGameDirs is a function variable (like DetectSystemJava) so binding
// tests can replace the real disk walk with a fake.
var ScanGameDirs = scanGameDirs

// Scan seams: the roots to walk and the total directory budget. Tests shrink
// both to keep the walk inside a temporary tree.
var (
	scanGameDirRoots = defaultScanRoots
	scanMaxVisited   = 50000
)

type scanRoot struct {
	path  string
	depth int
}

// defaultScanRoots walks the home directory (depth 5 covers layouts like
// ~/Games/<launcher>/.minecraft) and, on Windows, every drive root at a
// shallower depth for games installed outside the user profile.
func defaultScanRoots() []scanRoot {
	home, _ := os.UserHomeDir()
	roots := []scanRoot{{path: home, depth: 5}}
	if runtime.GOOS == "windows" {
		for letter := 'C'; letter <= 'Z'; letter++ {
			drive := string(letter) + ":\\"
			if fi, err := os.Stat(drive); err == nil && fi.IsDir() {
				roots = append(roots, scanRoot{path: drive, depth: 3})
			}
		}
	}
	return roots
}

// scanSkipDirs are directory names never descended into: dependency trees,
// caches and OS internals that cannot contain a game directory but can hold
// tens of thousands of entries.
var scanSkipDirs = map[string]bool{
	"node_modules": true, ".git": true, ".cache": true, ".npm": true,
	".cargo": true, ".rustup": true, ".gradle": true, ".m2": true, "go": true,
	"Caches": true, "Cache": true, "Code Cache": true, "GPUCache": true,
	"Crashpad": true, "IndexedDB": true, "Local Storage": true,
	"Session Storage": true, "Service Worker": true, "blob_storage": true,
	"Containers": true, "Group Containers": true, "Developer": true,
	"CoreSimulator": true, "Packages": true, "Trash": true, ".Trash": true,
	"Windows": true, "$Recycle.Bin": true, "System Volume Information": true,
	"Program Files": true, "Program Files (x86)": true, "ProgramData": true,
	"steamapps": true,
}

// scanGameDirs walks the scan roots looking for game directories: directories
// named .minecraft/minecraft that contain versions/ or options.txt, or any
// directory holding launcher_profiles.json. The walk is depth-limited, prunes
// scanSkipDirs and stops after scanMaxVisited directories, so it stays
// bounded on large disks. Results are deduplicated and sorted by path.
func scanGameDirs() []GameDirCandidate {
	found := map[string]GameDirCandidate{}
	budget := scanMaxVisited
	for _, root := range scanGameDirRoots() {
		if root.path == "" {
			continue
		}
		walkGameDirs(root.path, root.depth, &budget, found)
	}
	out := make([]GameDirCandidate, 0, len(found))
	for _, c := range found {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func walkGameDirs(dir string, depth int, budget *int, found map[string]GameDirCandidate) {
	if *budget <= 0 {
		return
	}
	*budget--
	if gameDirContent(dir) {
		found[dir] = GameDirCandidate{Path: dir, VersionCount: countVersions(dir)}
	}
	if depth <= 0 {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		// e.IsDir() is false for symlinks, so the walk never follows them
		// (no cycles) and only inspects real directories.
		if !e.IsDir() || scanSkipDirs[e.Name()] {
			continue
		}
		walkGameDirs(filepath.Join(dir, e.Name()), depth-1, budget, found)
	}
}

// gameDirContent recognizes a game directory by its layout, not just its
// name: launcher_profiles.json is a strong signal on its own, while the
// generic .minecraft/minecraft names also need versions/ or options.txt so
// an empty leftover folder does not qualify.
func gameDirContent(dir string) bool {
	if fi, err := os.Stat(filepath.Join(dir, "launcher_profiles.json")); err == nil && !fi.IsDir() {
		return true
	}
	base := filepath.Base(dir)
	if !strings.EqualFold(base, ".minecraft") && !strings.EqualFold(base, "minecraft") {
		return false
	}
	if fi, err := os.Stat(filepath.Join(dir, "versions")); err == nil && fi.IsDir() {
		return true
	}
	_, err := os.Stat(filepath.Join(dir, "options.txt"))
	return err == nil
}

// countVersions counts the version profiles under dir/versions (subdirectories
// that contain their own <id>.json).
func countVersions(dir string) int {
	entries, err := os.ReadDir(filepath.Join(dir, "versions"))
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, "versions", e.Name(), e.Name()+".json")); err == nil {
			count++
		}
	}
	return count
}