package services

import (
	"errors"
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
