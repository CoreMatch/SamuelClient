// Package tokenstorelive carries the opt-in live verification of the OS
// keychain token store. It lives in its own package on purpose: the unit
// tests in internal/services install go-keyring's process-global mock
// backend, and a live run must never see that mock (a mock hit would be a
// false green). Run on a desktop session with:
//
//	TOKENSTORE_LIVE=1 go test ./internal/tokenstorelive/
package tokenstorelive

import (
	"os"
	"testing"

	"HRPMLW/internal/services"
)

func TestKeychainTokenStoreLive(t *testing.T) {
	if os.Getenv("TOKENSTORE_LIVE") != "1" {
		t.Skip("set TOKENSTORE_LIVE=1 to run against the real OS keychain")
	}
	store := services.NewKeychainTokenStore()

	if err := store.Available(); err != nil {
		t.Fatalf("live Available: %v", err)
	}

	const id = "live-roundtrip"
	want := services.TokenPair{AccessToken: "live-access", ClientToken: "live-client"}
	if err := store.Set(id, want); err != nil {
		t.Fatalf("live Set: %v", err)
	}
	defer func() { _ = store.Delete(id) }()

	got, err := store.Get(id)
	if err != nil {
		t.Fatalf("live Get: %v", err)
	}
	if got != want {
		t.Fatalf("live Get = %+v, want %+v", got, want)
	}
	if err := store.Delete(id); err != nil {
		t.Fatalf("live Delete: %v", err)
	}
	if _, err := store.Get(id); err == nil {
		t.Fatal("live Get after Delete succeeded, want failure")
	}
}
