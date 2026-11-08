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

	"github.com/zalando/go-keyring"
)

// keychainService namespaces all entries this launcher writes. Kept as a var
// (not const) so tests can point live verification at a scratch namespace.
var keychainService = "SamuelClient"

// TokenPair is the secret payload stored per account.
type TokenPair struct {
	AccessToken string `json:"accessToken"`
	ClientToken string `json:"clientToken"`
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
