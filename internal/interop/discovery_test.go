package interop

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoveryRoundTrip(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	path, err := WriteDiscovery(AppReceiver, 39371, "tok-abc")
	if err != nil {
		t.Fatalf("WriteDiscovery: %v", err)
	}
	if filepath.Base(path) != AppReceiver+".json" {
		t.Errorf("discovery path = %q", path)
	}

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat discovery file: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0600 {
		t.Errorf("discovery file perms = %o, want 0600", perm)
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat discovery dir: %v", err)
	}
	if perm := dirInfo.Mode().Perm(); perm != 0700 {
		t.Errorf("discovery dir perms = %o, want 0700", perm)
	}

	doc, err := ReadDiscovery(AppReceiver)
	if err != nil {
		t.Fatalf("ReadDiscovery: %v", err)
	}
	if doc.App != AppReceiver || doc.Port != 39371 || doc.Token != "tok-abc" ||
		doc.ProtocolVersion != ProtocolVersion || doc.Pid != os.Getpid() {
		t.Errorf("discovery doc = %+v", doc)
	}

	RemoveDiscovery(AppReceiver)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("discovery file still present after removal (err=%v)", err)
	}
}

func TestDiscoveryFallsBackToHomeConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv("HOME", home)

	dir, err := DiscoveryDir()
	if err != nil {
		t.Fatalf("DiscoveryDir: %v", err)
	}
	want := filepath.Join(home, ".config", "matchcore-interop")
	if dir != want {
		t.Errorf("DiscoveryDir = %q, want %q", dir, want)
	}
}

func TestPidAlive(t *testing.T) {
	if !PidAlive(os.Getpid()) {
		t.Error("own pid reported dead")
	}
	if PidAlive(0) || PidAlive(-5) {
		t.Error("non-positive pid reported alive")
	}
	// A pid that almost certainly does not exist on any sane system.
	if PidAlive(1<<30 + 7) {
		t.Error("implausible pid reported alive")
	}
}

func TestNewTokenShape(t *testing.T) {
	tok, err := NewToken()
	if err != nil {
		t.Fatalf("NewToken: %v", err)
	}
	if len(tok) != 64 {
		t.Errorf("token length = %d, want 64 hex chars", len(tok))
	}
	other, _ := NewToken()
	if tok == other {
		t.Error("two tokens are identical")
	}
}
