package services

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const disabledModSuffix = ".bak"

// Serialize launcher file operations so list reads and name checks cannot
// race with a copy, rename, or deletion by another launcher binding.
var modsMu sync.Mutex

type ModFile struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

func modFile(name string) (ModFile, bool) {
	if !filepath.IsLocal(name) || strings.ContainsAny(name, `/\:`) {
		return ModFile{}, false
	}
	base, disabled := strings.CutSuffix(name, disabledModSuffix)
	if len(base) <= len(".jar") || !strings.EqualFold(filepath.Ext(base), ".jar") {
		return ModFile{}, false
	}
	return ModFile{Name: name, Enabled: !disabled}, true
}

func openMods(gameDir string, create bool) (*os.Root, error) {
	path := filepath.Join(gameDir, "mods")
	if create {
		if err := os.MkdirAll(gameDir, 0755); err != nil {
			return nil, err
		}
		if err := os.Mkdir(path, 0755); err != nil && !errors.Is(err, os.ErrExist) {
			return nil, err
		}
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("mods is not a regular directory: %s", path)
	}
	return os.OpenRoot(path)
}

func ListMods(gameDir string) ([]ModFile, error) {
	modsMu.Lock()
	defer modsMu.Unlock()

	mods := []ModFile{}
	root, err := openMods(gameDir, false)
	if errors.Is(err, os.ErrNotExist) {
		return mods, nil
	}
	if err != nil {
		return nil, err
	}
	defer root.Close()

	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if mod, ok := modFile(entry.Name()); ok && entry.Type().IsRegular() {
			mods = append(mods, mod)
		}
	}
	return mods, nil
}

func AddMod(gameDir, source string) error {
	name := filepath.Base(source)
	mod, ok := modFile(name)
	if !ok || !mod.Enabled {
		return fmt.Errorf("select a .jar file: %q", name)
	}
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("not a regular .jar file: %q", name)
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()

	modsMu.Lock()
	defer modsMu.Unlock()
	root, err := openMods(gameDir, true)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := checkModDestination(root, name+disabledModSuffix); err != nil {
		return err
	}
	output, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	if err := errors.Join(copyErr, output.Close()); err != nil {
		return errors.Join(err, root.Remove(name))
	}
	return nil
}

func checkModDestination(root *os.Root, name string) error {
	_, err := root.Lstat(name)
	if err == nil {
		return fmt.Errorf("a file named %q already exists", name)
	}
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func withMod(gameDir, name string, action func(*os.Root, ModFile) error) error {
	mod, ok := modFile(name)
	if !ok {
		return fmt.Errorf("invalid mod filename: %q", name)
	}

	modsMu.Lock()
	defer modsMu.Unlock()
	root, err := openMods(gameDir, false)
	if err != nil {
		return err
	}
	defer root.Close()
	info, err := root.Lstat(name)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("not a regular mod file: %q", name)
	}
	return action(root, mod)
}

func SetModEnabled(gameDir, name string, enabled bool) error {
	return withMod(gameDir, name, func(root *os.Root, mod ModFile) error {
		if mod.Enabled == enabled {
			return nil
		}
		target := name + disabledModSuffix
		if enabled {
			target = strings.TrimSuffix(name, disabledModSuffix)
		}
		if err := checkModDestination(root, target); err != nil {
			return err
		}
		return root.Rename(name, target)
	})
}

func DeleteMod(gameDir, name string) error {
	return withMod(gameDir, name, func(root *os.Root, mod ModFile) error {
		return root.Remove(mod.Name)
	})
}
