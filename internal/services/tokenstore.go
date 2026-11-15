package services

// tokenstore.go stores account session tokens (Yggdrasil access/client token
// pairs, and later MSA refresh tokens) in the operating system's credential
// store instead of the frontend's localStorage (ADR-0008). The frontend keeps
// only non-sensitive account metadata; every secret crosses into the Go side
// through the Wails binding surface and lands here.
//
// The store is deliberately narrow: one entry per account id holding a small
// JSON payload. Keychain entries are namespaced by service so multiple
// installs (or the dev/test binary) do not read each other's credentials.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/zalando/go-keyring"
)

// keychainService namespaces all entries this launcher writes. Kept as a var
// (not const) so tests can point live verification at a scratch namespace.
var keychainService = "SamuelClient"

// TokenPair is the secret payload stored per account. Microsoft-login
// accounts additionally carry the MSA refresh token (90-day rolling, equal to
// account control — ADR-0007 §5); HRPAuth-only accounts leave it empty.
type TokenPair struct {
	AccessToken     string `json:"accessToken"`
	ClientToken     string `json:"clientToken"`
	MSARefreshToken string `json:"msaRefreshToken,omitempty"`
}

// TokenStore persists per-account token pairs. The keychain implementation is
// the production path; tests substitute fakes through the same interface.
type TokenStore interface {
	Set(accountID string, pair TokenPair) error
	Get(accountID string) (TokenPair, error)
	Delete(accountID string) error
	// Available reports whether the backing credential store is usable on
	// this machine. Callers treat false as "degrade with a warning", never
	// as "silently drop secrets".
	Available() error
}

// ErrTokenNotFound distinguishes a missing entry from a broken backend so the
// frontend can ask for re-login instead of showing a storage error.
var ErrTokenNotFound = errors.New("token entry not found")

// KeychainTokenStore implements TokenStore on the OS credential store
// (Windows Credential Manager / macOS Keychain / freedesktop Secret Service).
type KeychainTokenStore struct{}

func NewKeychainTokenStore() *KeychainTokenStore {
	return &KeychainTokenStore{}
}

func keychainKey(accountID string) string {
	return "account/" + accountID
}

func (k *KeychainTokenStore) Set(accountID string, pair TokenPair) error {
	if accountID == "" {
		return errors.New("account id is required")
	}
	payload, err := json.Marshal(pair)
	if err != nil {
		return fmt.Errorf("marshal token pair: %w", err)
	}
	if err := keyring.Set(keychainService, keychainKey(accountID), string(payload)); err != nil {
		return fmt.Errorf("keychain set: %w", err)
	}
	return nil
}

func (k *KeychainTokenStore) Get(accountID string) (TokenPair, error) {
	var pair TokenPair
	if accountID == "" {
		return pair, errors.New("account id is required")
	}
	raw, err := keyring.Get(keychainService, keychainKey(accountID))
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return pair, ErrTokenNotFound
		}
		return pair, fmt.Errorf("keychain get: %w", err)
	}
	if err := json.Unmarshal([]byte(raw), &pair); err != nil {
		return pair, fmt.Errorf("unmarshal token pair: %w", err)
	}
	return pair, nil
}

func (k *KeychainTokenStore) Delete(accountID string) error {
	if accountID == "" {
		return errors.New("account id is required")
	}
	err := keyring.Delete(keychainService, keychainKey(accountID))
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf("keychain delete: %w", err)
	}
	// Deleting a missing entry is success: account removal must not fail on
	// an already-clean keychain.
	return nil
}

// Available probes the backend with a scratch entry. A write/read/delete
// round-trip is the only honest check: some systems have a D-Bus session but
// no secret service behind it.
func (k *KeychainTokenStore) Available() error {
	const probeKey = "__availability_probe__"
	if err := keyring.Set(keychainService, probeKey, "probe"); err != nil {
		return fmt.Errorf("keychain unavailable: %w", err)
	}
	// Best-effort cleanup; a leftover probe value is harmless (opaque,
	// namespaced, overwritten on the next probe).
	_ = keyring.Delete(keychainService, probeKey)
	return nil
}

// FileTokenStore implements TokenStore on a single JSON file with 0600
// permissions. It is the automatic fallback for machines without a usable OS
// credential store (ADR-0010): secrets stay out of the webview and out of
// logs, protected by filesystem permissions instead of the keychain. Writes
// are atomic (temp file + rename) so a crash never leaves a partial file.
type FileTokenStore struct {
	path string
	mu   sync.Mutex
}

func NewFileTokenStore(path string) *FileTokenStore {
	return &FileTokenStore{path: path}
}

func (f *FileTokenStore) load() (map[string]TokenPair, error) {
	data, err := os.ReadFile(f.path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]TokenPair{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read token file: %w", err)
	}
	var entries map[string]TokenPair
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parse token file: %w", err)
	}
	return entries, nil
}

func (f *FileTokenStore) store(entries map[string]TokenPair) error {
	payload, err := json.Marshal(entries)
	if err != nil {
		return fmt.Errorf("marshal token file: %w", err)
	}
	dir := filepath.Dir(f.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create token dir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".tokens-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp token file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename succeeded
	if _, err := tmp.Write(payload); err != nil {
		tmp.Close()
		return fmt.Errorf("write token file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close token file: %w", err)
	}
	if err := os.Chmod(tmpName, 0600); err != nil {
		return fmt.Errorf("chmod token file: %w", err)
	}
	if err := os.Rename(tmpName, f.path); err != nil {
		return fmt.Errorf("install token file: %w", err)
	}
	return nil
}

func (f *FileTokenStore) Set(accountID string, pair TokenPair) error {
	if accountID == "" {
		return errors.New("account id is required")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	entries, err := f.load()
	if err != nil {
		return err
	}
	entries[accountID] = pair
	return f.store(entries)
}

func (f *FileTokenStore) Get(accountID string) (TokenPair, error) {
	if accountID == "" {
		return TokenPair{}, errors.New("account id is required")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	entries, err := f.load()
	if err != nil {
		return TokenPair{}, err
	}
	pair, ok := entries[accountID]
	if !ok {
		return TokenPair{}, ErrTokenNotFound
	}
	return pair, nil
}

func (f *FileTokenStore) Delete(accountID string) error {
	if accountID == "" {
		return errors.New("account id is required")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	entries, err := f.load()
	if err != nil {
		return err
	}
	if _, ok := entries[accountID]; !ok {
		return nil
	}
	delete(entries, accountID)
	return f.store(entries)
}

// Available probes with a scratch write, mirroring the keychain store: the
// directory must be creatable and the file writable.
func (f *FileTokenStore) Available() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	const probeKey = "__availability_probe__"
	entries, err := f.load()
	if err != nil {
		return fmt.Errorf("token file unavailable: %w", err)
	}
	entries[probeKey] = TokenPair{AccessToken: "probe"}
	if err := f.store(entries); err != nil {
		return fmt.Errorf("token file unavailable: %w", err)
	}
	delete(entries, probeKey)
	return f.store(entries)
}

// FallbackTokenStore keeps the OS keychain as the primary store and
// automatically falls back to a local file when the keychain is unusable
// (ADR-0010). Reads prefer the keychain; an entry written during a fallback
// period is still found after the keychain comes back.
type FallbackTokenStore struct {
	primary  TokenStore
	fallback TokenStore
}

func NewFallbackTokenStore(primary, fallback TokenStore) *FallbackTokenStore {
	return &FallbackTokenStore{primary: primary, fallback: fallback}
}

func (s *FallbackTokenStore) Set(accountID string, pair TokenPair) error {
	if err := s.primary.Set(accountID, pair); err != nil {
		return s.fallback.Set(accountID, pair)
	}
	// The keychain accepted the pair; drop any stale fallback copy so
	// secrets do not linger in the file after the keychain recovers.
	_ = s.fallback.Delete(accountID)
	return nil
}

func (s *FallbackTokenStore) Get(accountID string) (TokenPair, error) {
	pair, err := s.primary.Get(accountID)
	if err == nil {
		return pair, nil
	}
	fbPair, fbErr := s.fallback.Get(accountID)
	if fbErr == nil {
		return fbPair, nil
	}
	// The fallback is readable and holds no entry: there is no token to
	// hand out, whatever the keychain's state — report not-found so the
	// frontend asks for re-login instead of surfacing a storage error.
	if errors.Is(fbErr, ErrTokenNotFound) {
		return TokenPair{}, ErrTokenNotFound
	}
	return TokenPair{}, err
}

func (s *FallbackTokenStore) Delete(accountID string) error {
	primaryErr := s.primary.Delete(accountID)
	fallbackErr := s.fallback.Delete(accountID)
	// Succeed when at least one store is clean afterwards: an unavailable
	// keychain must not block account removal.
	if primaryErr != nil && fallbackErr != nil {
		return primaryErr
	}
	return nil
}

func (s *FallbackTokenStore) Available() error {
	if err := s.primary.Available(); err == nil {
		return nil
	}
	return s.fallback.Available()
}

// Backend names the store that would accept a write right now: "keychain",
// "file", or "" when neither is usable. Surfaced by KeychainStatus so the UI
// can tell the user where their credentials live.
func (s *FallbackTokenStore) Backend() string {
	if err := s.primary.Available(); err == nil {
		return "keychain"
	}
	if err := s.fallback.Available(); err == nil {
		return "file"
	}
	return ""
}
