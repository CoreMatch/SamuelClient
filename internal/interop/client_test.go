package interop

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"
)

// senderFixture serves the sender-side pull endpoint (INTEROP-SPEC §4.2).
func senderFixture(t *testing.T, status int, payload *HandoffPayload) (*DiscoveryFile, *httptest.Server) {
	t.Helper()
	token := "sender-token"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if status != http.StatusOK {
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]interface{}{"code": ErrGone, "message": "gone"},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(payload)
	}))
	t.Cleanup(srv.Close)

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse fixture url: %v", err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatalf("parse fixture port: %v", err)
	}
	doc := &DiscoveryFile{
		App:             AppSender,
		ProtocolVersion: ProtocolVersion,
		Pid:             1, // not used by FetchPendingHandoff itself
		Port:            port,
		Token:           token,
	}
	return doc, srv
}

func TestFetchPendingHandoffSuccess(t *testing.T) {
	want := validPayload()
	doc, _ := senderFixture(t, http.StatusOK, want)

	got, err := FetchPendingHandoff(doc, want.HandoffID, 2*time.Second)
	if err != nil {
		t.Fatalf("FetchPendingHandoff: %v", err)
	}
	if got.HandoffID != want.HandoffID || got.Match.JoinTarget.Host != want.Match.JoinTarget.Host {
		t.Errorf("fetched = %+v", got)
	}
}

func TestFetchPendingHandoffGone(t *testing.T) {
	doc, _ := senderFixture(t, http.StatusNotFound, nil)
	_, err := FetchPendingHandoff(doc, "9b2f0c1e-3a7d-4e5f-8b21-6c9d0e1f2a3b", 2*time.Second)
	if !errors.Is(err, ErrHandoffGone) {
		t.Errorf("err = %v, want ErrHandoffGone", err)
	}
}

func TestFetchPendingHandoffServerError(t *testing.T) {
	doc, _ := senderFixture(t, http.StatusInternalServerError, nil)
	_, err := FetchPendingHandoff(doc, "9b2f0c1e-3a7d-4e5f-8b21-6c9d0e1f2a3b", 2*time.Second)
	if err == nil || errors.Is(err, ErrHandoffGone) {
		t.Errorf("err = %v, want a non-gone failure", err)
	}
}

func TestFetchPendingHandoffRejectsBadDoc(t *testing.T) {
	if _, err := FetchPendingHandoff(nil, "x", time.Second); err == nil {
		t.Error("nil discovery doc accepted")
	}
	if _, err := FetchPendingHandoff(&DiscoveryFile{}, "x", time.Second); err == nil {
		t.Error("empty discovery doc accepted")
	}
}

func TestLoadLiveSenderDiscoveryStaleAndMissing(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	// Missing file: (nil, nil).
	doc, err := LoadLiveSenderDiscovery()
	if err != nil || doc != nil {
		t.Fatalf("missing: doc=%v err=%v, want nil/nil", doc, err)
	}

	// Stale pid: (nil, nil).
	if _, err := WriteDiscovery(AppSender, 12345, "tok"); err != nil {
		t.Fatalf("WriteDiscovery: %v", err)
	}
	stale, err := ReadDiscovery(AppSender)
	if err != nil {
		t.Fatalf("ReadDiscovery: %v", err)
	}
	stale.Pid = 1<<30 + 7 // implausible pid
	data, _ := json.Marshal(stale)
	path, _ := DiscoveryPath(AppSender)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("rewrite stale discovery: %v", err)
	}
	doc, err = LoadLiveSenderDiscovery()
	if err != nil || doc != nil {
		t.Errorf("stale: doc=%v err=%v, want nil/nil", doc, err)
	}
}
