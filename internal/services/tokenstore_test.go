package services

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestKeychainTokenStoreRoundTrip(t *testing.T) {
	keyring.MockInit()
	store := NewKeychainTokenStore()

	if err := store.Available(); err != nil {
		t.Fatalf("Available on mock backend: %v", err)
	}

	want := TokenPair{AccessToken: "access-1", ClientToken: "client-1"}
	if err := store.Set("acc-1", want); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, err := store.Get("acc-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != want {
		t.Fatalf("Get = %+v, want %+v", got, want)
	}

	// Overwrite replaces the pair (login refresh path).
	want2 := TokenPair{AccessToken: "access-2", ClientToken: "client-2"}
	if err := store.Set("acc-1", want2); err != nil {
		t.Fatalf("Set overwrite: %v", err)
	}
	got, err = store.Get("acc-1")
	if err != nil {
		t.Fatalf("Get after overwrite: %v", err)
	}
	if got != want2 {
		t.Fatalf("Get = %+v, want %+v", got, want2)
	}

	if err := store.Delete("acc-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Get("acc-1"); !errors.Is(err, ErrTokenNotFound) {
		t.Fatalf("Get after Delete = %v, want ErrTokenNotFound", err)
	}

	// Deleting again is idempotent (account removal must not fail).
	if err := store.Delete("acc-1"); err != nil {
		t.Fatalf("second Delete: %v", err)
	}
}

func TestKeychainTokenStoreRejectsEmptyID(t *testing.T) {
	keyring.MockInit()
	store := NewKeychainTokenStore()

	if err := store.Set("", TokenPair{}); err == nil {
		t.Fatal("Set with empty id accepted")
	}
	if _, err := store.Get(""); err == nil {
		t.Fatal("Get with empty id accepted")
	}
	if err := store.Delete(""); err == nil {
		t.Fatal("Delete with empty id accepted")
	}
}

func TestFileTokenStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tokens.json")
	store := NewFileTokenStore(path)

	if err := store.Available(); err != nil {
		t.Fatalf("Available: %v", err)
	}

	want := TokenPair{AccessToken: "access-1", ClientToken: "client-1", MSARefreshToken: "msa-1"}
	if err := store.Set("acc-1", want); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := store.Get("acc-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != want {
		t.Fatalf("Get = %+v, want %+v", got, want)
	}

	// The file must be owner-only: it holds session secrets.
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat token file: %v", err)
		}
		if perm := fi.Mode().Perm(); perm != 0600 {
			t.Errorf("token file mode = %o, want 0600", perm)
		}
	}

	// Overwrite replaces the pair (login refresh path).
	want2 := TokenPair{AccessToken: "access-2", ClientToken: "client-2"}
	if err := store.Set("acc-1", want2); err != nil {
		t.Fatalf("Set overwrite: %v", err)
	}
	if got, err = store.Get("acc-1"); err != nil || got != want2 {
		t.Fatalf("Get after overwrite = %+v err=%v, want %+v", got, err, want2)
	}

	if err := store.Delete("acc-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Get("acc-1"); !errors.Is(err, ErrTokenNotFound) {
		t.Fatalf("Get after Delete = %v, want ErrTokenNotFound", err)
	}
	if err := store.Delete("acc-1"); err != nil {
		t.Fatalf("second Delete: %v", err)
	}
}

func TestFileTokenStoreRejectsEmptyID(t *testing.T) {
	store := NewFileTokenStore(filepath.Join(t.TempDir(), "tokens.json"))

	if err := store.Set("", TokenPair{}); err == nil {
		t.Fatal("Set with empty id accepted")
	}
	if _, err := store.Get(""); err == nil {
		t.Fatal("Get with empty id accepted")
	}
	if err := store.Delete(""); err == nil {
		t.Fatal("Delete with empty id accepted")
	}
}

func TestFileTokenStoreCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tokens.json")
	if err := os.WriteFile(path, []byte("not json"), 0600); err != nil {
		t.Fatal(err)
	}
	store := NewFileTokenStore(path)
	if _, err := store.Get("acc-1"); err == nil || errors.Is(err, ErrTokenNotFound) {
		t.Fatalf("Get on corrupt file = %v, want a parse error", err)
	}
}

// brokenTokenStore fails every operation, standing in for a machine with no
// usable OS credential store.
type brokenTokenStore struct{ err error }

func (b *brokenTokenStore) Set(string, TokenPair) error       { return b.err }
func (b *brokenTokenStore) Get(string) (TokenPair, error)     { return TokenPair{}, b.err }
func (b *brokenTokenStore) Delete(string) error               { return b.err }
func (b *brokenTokenStore) Available() error                  { return b.err }

func TestFallbackTokenStorePrefersKeychain(t *testing.T) {
	keyring.MockInit()
	file := NewFileTokenStore(filepath.Join(t.TempDir(), "tokens.json"))
	store := NewFallbackTokenStore(NewKeychainTokenStore(), file)

	want := TokenPair{AccessToken: "access", ClientToken: "client"}
	if err := store.Set("acc-1", want); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := store.Get("acc-1")
	if err != nil || got != want {
		t.Fatalf("Get = %+v err=%v, want %+v", got, err, want)
	}
	// Nothing lands in the file while the keychain works.
	if _, err := file.Get("acc-1"); !errors.Is(err, ErrTokenNotFound) {
		t.Fatalf("file entry exists while keychain is usable: %v", err)
	}
	if store.Backend() != "keychain" {
		t.Errorf("Backend = %q, want keychain", store.Backend())
	}
}

func TestFallbackTokenStoreFallsBackToFile(t *testing.T) {
	file := NewFileTokenStore(filepath.Join(t.TempDir(), "tokens.json"))
	store := NewFallbackTokenStore(&brokenTokenStore{err: errors.New("no secret service")}, file)

	if err := store.Available(); err != nil {
		t.Fatalf("Available with working file fallback: %v", err)
	}
	if store.Backend() != "file" {
		t.Errorf("Backend = %q, want file", store.Backend())
	}

	want := TokenPair{AccessToken: "access", ClientToken: "client", MSARefreshToken: "msa"}
	if err := store.Set("acc-1", want); err != nil {
		t.Fatalf("Set via fallback: %v", err)
	}
	got, err := store.Get("acc-1")
	if err != nil || got != want {
		t.Fatalf("Get via fallback = %+v err=%v, want %+v", got, err, want)
	}
	// Account removal must succeed even though the keychain is broken.
	if err := store.Delete("acc-1"); err != nil {
		t.Fatalf("Delete via fallback: %v", err)
	}
	if _, err := store.Get("acc-1"); !errors.Is(err, ErrTokenNotFound) {
		t.Fatalf("Get after Delete = %v, want ErrTokenNotFound", err)
	}
}

func TestFallbackTokenStoreFindsFallbackEntryWhenKeychainMissing(t *testing.T) {
	keyring.MockInit()
	file := NewFileTokenStore(filepath.Join(t.TempDir(), "tokens.json"))
	// A pair written during a fallback period must survive the keychain
	// coming back empty.
	want := TokenPair{AccessToken: "old-access", ClientToken: "old-client"}
	if err := file.Set("acc-1", want); err != nil {
		t.Fatal(err)
	}
	store := NewFallbackTokenStore(NewKeychainTokenStore(), file)

	got, err := store.Get("acc-1")
	if err != nil || got != want {
		t.Fatalf("Get = %+v err=%v, want the fallback entry %+v", got, err, want)
	}
}

func TestFallbackTokenStoreSetClearsStaleFallback(t *testing.T) {
	keyring.MockInit()
	file := NewFileTokenStore(filepath.Join(t.TempDir(), "tokens.json"))
	if err := file.Set("acc-1", TokenPair{AccessToken: "stale"}); err != nil {
		t.Fatal(err)
	}
	store := NewFallbackTokenStore(NewKeychainTokenStore(), file)

	if err := store.Set("acc-1", TokenPair{AccessToken: "fresh", ClientToken: "c"}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if _, err := file.Get("acc-1"); !errors.Is(err, ErrTokenNotFound) {
		t.Fatalf("stale fallback entry survived a keychain write: %v", err)
	}
}

func TestFallbackTokenStoreBothMissing(t *testing.T) {
	keyring.MockInit()
	file := NewFileTokenStore(filepath.Join(t.TempDir(), "tokens.json"))
	store := NewFallbackTokenStore(NewKeychainTokenStore(), file)

	if _, err := store.Get("acc-1"); !errors.Is(err, ErrTokenNotFound) {
		t.Fatalf("Get = %v, want ErrTokenNotFound", err)
	}
}

func TestFallbackTokenStoreBothBroken(t *testing.T) {
	// A file whose parent is a regular file cannot be created.
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	file := NewFileTokenStore(filepath.Join(blocker, "tokens.json"))
	store := NewFallbackTokenStore(&brokenTokenStore{err: errors.New("no secret service")}, file)

	if err := store.Available(); err == nil {
		t.Fatal("Available must fail when neither store works")
	}
	if store.Backend() != "" {
		t.Errorf("Backend = %q, want empty", store.Backend())
	}
	if err := store.Set("acc-1", TokenPair{AccessToken: "a"}); err == nil {
		t.Fatal("Set must fail when neither store works")
	}
}
