package interop

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

// DiscoveryDir returns the interop rendezvous directory (INTEROP-SPEC §2):
// $XDG_RUNTIME_DIR/matchcore-interop, falling back to
// $HOME/.config/matchcore-interop.
func DiscoveryDir() (string, error) {
	if runtime := os.Getenv("XDG_RUNTIME_DIR"); runtime != "" {
		return filepath.Join(runtime, "matchcore-interop"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".config", "matchcore-interop"), nil
}

// DiscoveryPath returns the discovery file path for the given app name.
func DiscoveryPath(app string) (string, error) {
	dir, err := DiscoveryDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, app+".json"), nil
}

// NewToken generates a 32-byte CSPRNG token, hex encoded (64 chars).
func NewToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// WriteDiscovery atomically publishes the discovery file (0700 dir, 0600
// file) for a listener on port with the given token.
func WriteDiscovery(app string, port int, token string) (string, error) {
	dir, err := DiscoveryDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("create discovery dir: %w", err)
	}

	doc := DiscoveryFile{
		App:             app,
		ProtocolVersion: ProtocolVersion,
		Pid:             os.Getpid(),
		Port:            port,
		Token:           token,
		CreatedAt:       time.Now().Format(time.RFC3339),
	}
	data, err := json.MarshalIndent(&doc, "", "  ")
	if err != nil {
		return "", err
	}

	path := filepath.Join(dir, app+".json")
	tmp, err := os.CreateTemp(dir, app+".*.tmp")
	if err != nil {
		return "", fmt.Errorf("create temp discovery file: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return "", err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return "", err
	}
	if err := os.Chmod(tmpName, 0600); err != nil {
		os.Remove(tmpName)
		return "", err
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return "", fmt.Errorf("publish discovery file: %w", err)
	}
	return path, nil
}

// RemoveDiscovery deletes the discovery file on clean shutdown. Best-effort:
// a stale file left by a crash is neutralized by the pid check on the reader
// side.
func RemoveDiscovery(app string) {
	if path, err := DiscoveryPath(app); err == nil {
		_ = os.Remove(path)
	}
}

// ReadDiscovery parses the discovery file of the given app. It does NOT
// verify liveness; callers use PidAlive for the staleness check.
func ReadDiscovery(app string) (*DiscoveryFile, error) {
	path, err := DiscoveryPath(app)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc DiscoveryFile
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &doc, nil
}

// PidAlive reports whether pid refers to a running process (Linux /proc
// check; on other platforms it falls back to signal 0 probing via os).
func PidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	if _, err := os.Stat(filepath.Join("/proc", strconv.Itoa(pid))); err == nil {
		return true
	}
	// Non-Linux fallback: process existence probe.
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Signal 0 probes without delivering; EPERM means "exists but not ours",
	// which still counts as alive for staleness purposes.
	err = p.Signal(syscall.Signal(0))
	return err == nil || os.IsPermission(err)
}
