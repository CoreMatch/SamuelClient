package services

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"HRPMLW/internal/database"
	"HRPMLW/internal/models"
)

const settingKeyGameDirs = "launcher.game_dirs"

var gameDirsMu sync.Mutex

type gameDirConfig struct {
	Default string   `json:"default"`
	Paths   []string `json:"paths"`
}

// GetMinecraftDir returns the default game directory: .minecraft inside the
// launcher data directory (next to the executable), so downloads stay with
// the portable launcher folder (ADR-0010). Existing installs elsewhere are
// not auto-adopted; the settings UI (ScanGameDirs + SetGameDir) lets the user
// point the launcher at one, and ResolveGameDir honors that override.
func (l *LauncherService) GetMinecraftDir() string {
	return filepath.Join(database.DataDir(), ".minecraft")
}

// ResolveGameDir returns the default destination for new installations.
func (l *LauncherService) ResolveGameDir() string {
	settings, _ := loadLauncherSettings()
	return l.gameDirsFromSettings(settings).Default
}

func (l *LauncherService) gameDirsFromSettings(settings map[string]string) gameDirConfig {
	portable := l.GetMinecraftDir()
	cfg := gameDirConfig{}
	if raw := settings[settingKeyGameDirs]; raw == "" || json.Unmarshal([]byte(raw), &cfg) != nil {
		// Preserve the old single-folder selection on first use.
		cfg = gameDirConfig{Default: settings[settingKeyGameDir]}
	}
	if cfg.Default == "" || sameGameDir(cfg.Default, portable) {
		cfg.Default = portable
	}
	paths := []string{portable}
	for _, path := range append(cfg.Paths, cfg.Default) {
		if path != "" && !containsGameDir(paths, path) {
			paths = append(paths, path)
		}
	}
	cfg.Paths = paths
	return cfg
}

func sameGameDir(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func containsGameDir(paths []string, dir string) bool {
	for _, path := range paths {
		if sameGameDir(path, dir) {
			return true
		}
	}
	return false
}

// GameDirs returns the registered roots in scan order, prioritizing the
// default when a caller only has a version ID (such as a match handoff).
func (l *LauncherService) GameDirs() []string {
	settings, _ := loadLauncherSettings()
	cfg := l.gameDirsFromSettings(settings)
	paths := []string{cfg.Default}
	for _, path := range cfg.Paths {
		if !sameGameDir(path, cfg.Default) {
			paths = append(paths, path)
		}
	}
	return paths
}

type GameDirEntry struct {
	Path   string `json:"path"`
	Source string `json:"source"` // "custom" or "auto" (portable default)
	Exists bool   `json:"exists"`
}

// GameDirStatus includes the install destination and every registered folder.
type GameDirStatus struct {
	Path   string `json:"path"`
	Source string `json:"source"`
	Exists bool   `json:"exists"`
	Dirs   []GameDirEntry `json:"dirs"`
}

func (l *LauncherService) GameDirStatus() GameDirStatus {
	settings, _ := loadLauncherSettings()
	return l.gameDirStatus(l.gameDirsFromSettings(settings))
}

func (l *LauncherService) gameDirStatus(cfg gameDirConfig) GameDirStatus {
	status := GameDirStatus{Path: cfg.Default}
	for _, path := range cfg.Paths {
		source := "custom"
		if sameGameDir(path, l.GetMinecraftDir()) {
			source = "auto"
		}
		fi, err := os.Stat(path)
		entry := GameDirEntry{Path: path, Source: source, Exists: err == nil && fi.IsDir()}
		status.Dirs = append(status.Dirs, entry)
		if sameGameDir(path, cfg.Default) {
			status.Source, status.Exists = source, entry.Exists
			status.Path = path
		}
	}
	return status
}

func normalizeGameDir(dir string) (string, error) {
	dir = strings.TrimSpace(dir)
	if dir == "~" || strings.HasPrefix(dir, "~/") || (runtime.GOOS == "windows" && strings.HasPrefix(dir, `~\`)) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, strings.TrimPrefix(dir, "~"))
	}
	if !filepath.IsAbs(dir) {
		return "", errors.New("game directory must be an absolute path")
	}
	dir = filepath.Clean(dir)
	if fi, err := os.Stat(dir); err == nil && !fi.IsDir() {
		return "", errors.New("game directory path points to a file")
	}
	return dir, nil
}

func (l *LauncherService) updateGameDirs(update func(*gameDirConfig) error) *models.ApiResponse {
	gameDirsMu.Lock()
	defer gameDirsMu.Unlock()
	settings, err := loadLauncherSettings()
	if err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}
	cfg := l.gameDirsFromSettings(settings)
	if err := update(&cfg); err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}
	// Do not pin the built-in path to this machine's launcher location.
	stored := gameDirConfig{Default: cfg.Default, Paths: cfg.Paths[1:]}
	if sameGameDir(stored.Default, l.GetMinecraftDir()) {
		stored.Default = ""
	}
	raw, err := json.Marshal(stored)
	if err == nil {
		err = saveLauncherSetting(settingKeyGameDirs, string(raw), "string", "Registered game directories and default install destination")
	}
	if err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}
	return &models.ApiResponse{Success: true, Data: l.gameDirStatus(cfg)}
}

// SetGameDir selects the default install destination, registering it if new.
// An empty dir selects the portable folder without removing saved folders.
func (l *LauncherService) SetGameDir(dir string) *models.ApiResponse {
	return l.updateGameDirs(func(cfg *gameDirConfig) error {
		if strings.TrimSpace(dir) == "" {
			cfg.Default = l.GetMinecraftDir()
			return nil
		}
		path, err := normalizeGameDir(dir)
		if err != nil {
			return err
		}
		for _, registered := range cfg.Paths {
			if sameGameDir(registered, path) {
				cfg.Default = registered
				return nil
			}
		}
		cfg.Default = path
		cfg.Paths = append(cfg.Paths, path)
		return nil
	})
}

func (l *LauncherService) AddGameDir(dir string) *models.ApiResponse {
	return l.updateGameDirs(func(cfg *gameDirConfig) error {
		path, err := normalizeGameDir(dir)
		if err != nil {
			return err
		}
		if !containsGameDir(cfg.Paths, path) {
			cfg.Paths = append(cfg.Paths, path)
		}
		return nil
	})
}

// RemoveGameDir forgets a folder without touching its contents.
func (l *LauncherService) RemoveGameDir(dir string) *models.ApiResponse {
	return l.updateGameDirs(func(cfg *gameDirConfig) error {
		if sameGameDir(dir, cfg.Default) || sameGameDir(dir, l.GetMinecraftDir()) {
			return errors.New("cannot remove the default install folder or the built-in folder")
		}
		for i, path := range cfg.Paths {
			if sameGameDir(path, dir) {
				cfg.Paths = append(cfg.Paths[:i], cfg.Paths[i+1:]...)
				break
			}
		}
		return nil
	})
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
