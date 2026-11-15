package services

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"HRPMLW/internal/database"
)

// authlibInjectorJar is the official authlib-injector agent bundled into the
// binary (ADR-0009). -javaagent requires a real file on disk, so the embedded
// bytes are materialized into the launcher data directory before launch.
//
//go:embed assets/authlib-injector.jar
var authlibInjectorJar []byte

// AuthlibInjectorVersion is the version of the bundled agent; update it
// together with assets/authlib-injector.jar.
const AuthlibInjectorVersion = "1.2.8"

// EnsureAuthlibInjectorJar materializes the bundled agent and returns its
// path. An identical on-disk copy is left untouched; a missing or outdated
// copy is replaced atomically (temp file + rename) so a concurrent game
// launch never observes a partial jar.
func EnsureAuthlibInjectorJar() (string, error) {
	dir := filepath.Dir(database.DefaultConfig().Path)
	dest := filepath.Join(dir, "authlib-injector.jar")

	if existing, err := os.ReadFile(dest); err == nil && bytes.Equal(existing, authlibInjectorJar) {
		return dest, nil
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".authlib-injector-*.tmp")
	if err != nil {
		return "", fmt.Errorf("create temp jar: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename succeeded
	if _, err := tmp.Write(authlibInjectorJar); err != nil {
		tmp.Close()
		return "", fmt.Errorf("write temp jar: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("close temp jar: %w", err)
	}
	if err := os.Chmod(tmpName, 0644); err != nil {
		return "", fmt.Errorf("chmod temp jar: %w", err)
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return "", fmt.Errorf("install jar: %w", err)
	}
	return dest, nil
}
