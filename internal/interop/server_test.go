package interop

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// startTestServer builds a receiver Server bound to a random loopback port.
// slotBusy controls the accept hook.
func startTestServer(t *testing.T, slotBusy bool) *Server {
	t.Helper()
	accept := func(*HandoffPayload) error {
		if slotBusy {
			return ErrSlotBusy
		}
		return nil
	}
	srv, err := NewServer(AppReceiver, accept, nil)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(srv.Stop)
	return srv
}

func (s *Server) url(path string) string {
	return fmt.Sprintf("http://127.0.0.1:%d%s", s.Port(), path)
}

func postJoin(t *testing.T, srv *Server, token string, body []byte) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, srv.url("/v1/join"), bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post join: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func errorCode(t *testing.T, resp *http.Response) string {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	return body.Error.Code
}

func marshalPayload(t *testing.T, p *HandoffPayload) []byte {
	t.Helper()
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return data
}

func TestHealthEndpoint(t *testing.T) {
	srv := startTestServer(t, false)
	resp, err := http.Get(srv.url("/v1/health"))
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health status = %d", resp.StatusCode)
	}
	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode health: %v", err)
	}
	if body["app"] != AppReceiver || body["ready"] != true ||
		int(body["protocol_version"].(float64)) != ProtocolVersion {
		t.Errorf("health body = %v", body)
	}
}

func TestJoinHappyPath(t *testing.T) {
	srv := startTestServer(t, false)
	resp := postJoin(t, srv, srv.Token(), marshalPayload(t, validPayload()))
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d body=%s", resp.StatusCode, body)
	}
	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "accepted" || body["handoff_id"] != validPayload().HandoffID {
		t.Errorf("accept body = %v", body)
	}
}

func TestJoinAuthFailures(t *testing.T) {
	srv := startTestServer(t, false)
	body := marshalPayload(t, validPayload())

	if resp := postJoin(t, srv, "", body); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("no token: status = %d, want 401", resp.StatusCode)
	}
	if resp := postJoin(t, srv, "wrong-token", body); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("wrong token: status = %d, want 401", resp.StatusCode)
	}
}

func TestJoinRejectsCredentials(t *testing.T) {
	srv := startTestServer(t, false)
	raw := `{"version":1,"handoff_id":"9b2f0c1e-3a7d-4e5f-8b21-6c9d0e1f2a3b",
		"expires_at":"2099-01-01T00:00:00Z",
		"match":{"match_id":"m","mode":"bedwars","join_target":{"host":"h.local","port":25565}},
		"access_token":"stolen"}`
	resp := postJoin(t, srv, srv.Token(), []byte(raw))
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", resp.StatusCode)
	}
	if code := errorCode(t, resp); code != ErrCredentialInPayload {
		t.Errorf("code = %q, want %q", code, ErrCredentialInPayload)
	}
}

func TestJoinVersionMismatch(t *testing.T) {
	srv := startTestServer(t, false)
	p := validPayload()
	p.Version = 2
	resp := postJoin(t, srv, srv.Token(), marshalPayload(t, p))
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", resp.StatusCode)
	}
	var body struct {
		Error struct {
			Code      string `json:"code"`
			Supported []int  `json:"supported"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error.Code != ErrVersionMismatch || len(body.Error.Supported) != 1 || body.Error.Supported[0] != ProtocolVersion {
		t.Errorf("version mismatch body = %+v", body.Error)
	}
}

func TestJoinExpired(t *testing.T) {
	srv := startTestServer(t, false)
	p := validPayload()
	p.ExpiresAt = "2020-01-01T00:00:00Z"
	resp := postJoin(t, srv, srv.Token(), marshalPayload(t, p))
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", resp.StatusCode)
	}
	if code := errorCode(t, resp); code != ErrHandoffExpired {
		t.Errorf("code = %q, want %q", code, ErrHandoffExpired)
	}
}

func TestJoinBadRequest(t *testing.T) {
	srv := startTestServer(t, false)

	// Invalid JSON.
	if resp := postJoin(t, srv, srv.Token(), []byte("{nope")); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("broken json: status = %d, want 400", resp.StatusCode)
	}

	// Schema violation (bad port).
	p := validPayload()
	p.Match.JoinTarget.Port = 0
	resp := postJoin(t, srv, srv.Token(), marshalPayload(t, p))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad port: status = %d, want 400", resp.StatusCode)
	}
	if code := errorCode(t, resp); code != ErrBadRequest {
		t.Errorf("code = %q, want %q", code, ErrBadRequest)
	}
}

func TestJoinReplayedNonce(t *testing.T) {
	srv := startTestServer(t, false)
	body := marshalPayload(t, validPayload())

	if resp := postJoin(t, srv, srv.Token(), body); resp.StatusCode != http.StatusOK {
		t.Fatalf("first push status = %d", resp.StatusCode)
	}
	resp := postJoin(t, srv, srv.Token(), body)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("replay status = %d, want 409", resp.StatusCode)
	}
	if code := errorCode(t, resp); code != ErrReplayedNonce {
		t.Errorf("code = %q, want %q", code, ErrReplayedNonce)
	}
}

func TestJoinWithoutNonceIsAccepted(t *testing.T) {
	srv := startTestServer(t, false)
	p := validPayload()
	p.Nonce = ""
	resp := postJoin(t, srv, srv.Token(), marshalPayload(t, p))
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d body=%s", resp.StatusCode, body)
	}
}

func TestJoinBusySlot(t *testing.T) {
	srv := startTestServer(t, true)
	resp := postJoin(t, srv, srv.Token(), marshalPayload(t, validPayload()))
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}
	if code := errorCode(t, resp); code != ErrBusy {
		t.Errorf("code = %q, want %q", code, ErrBusy)
	}
}

func TestJoinOversizedBody(t *testing.T) {
	srv := startTestServer(t, false)
	p := validPayload()
	p.Match.MapName = strings.Repeat("x", MaxBodyBytes) // way over the cap
	resp := postJoin(t, srv, srv.Token(), marshalPayload(t, p))
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413", resp.StatusCode)
	}
}

func TestNonceWindowPrunes(t *testing.T) {
	srv := startTestServer(t, false)
	now := time.Now()

	if !srv.recordNonce("n1", now.Add(-NonceWindow-time.Minute)) {
		t.Fatal("first recording of aged nonce rejected")
	}
	// The aged entry must have been pruned by the next recording, so the
	// same nonce is accepted again after the window.
	if !srv.recordNonce("n1", now) {
		t.Error("nonce not pruned after the replay window")
	}
	// Immediate duplicate within the window is rejected.
	if srv.recordNonce("n1", now.Add(time.Second)) {
		t.Error("duplicate nonce within window accepted")
	}
}
