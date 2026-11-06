package interop

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

// ErrSlotBusy is returned by the accept hook when the receiver already holds
// a pending/launching handoff; the server maps it to 503 busy.
var ErrSlotBusy = errors.New("handoff slot busy")

// AcceptFunc decides whether the receiver takes a validated handoff. It
// returns ErrSlotBusy when the single handoff slot is occupied.
type AcceptFunc func(payload *HandoffPayload) error

// Server is the receiver's loopback-only handoff listener (INTEROP-SPEC §4.1).
type Server struct {
	app     string
	token   string
	accept  AcceptFunc
	onEvent func(format string, args ...interface{})

	listener   net.Listener
	httpServer *http.Server

	mu         sync.Mutex
	seenNonces map[string]time.Time
}

// NewServer creates a receiver listener with a fresh per-process token.
// accept is invoked for every fully validated handoff; onEvent receives
// sanitized log lines (never tokens or bearer headers).
func NewServer(app string, accept AcceptFunc, onEvent func(format string, args ...interface{})) (*Server, error) {
	token, err := NewToken()
	if err != nil {
		return nil, err
	}
	if onEvent == nil {
		onEvent = func(string, ...interface{}) {}
	}
	return &Server{
		app:        app,
		token:      token,
		accept:     accept,
		onEvent:    onEvent,
		seenNonces: make(map[string]time.Time),
	}, nil
}

// Token returns the bearer token published in the discovery file.
func (s *Server) Token() string { return s.token }

// Port returns the bound loopback port (valid after Start).
func (s *Server) Port() int {
	if s.listener == nil {
		return 0
	}
	return s.listener.Addr().(*net.TCPAddr).Port
}

// Start binds 127.0.0.1:0 and serves until Stop.
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("bind loopback: %w", err)
	}
	s.listener = ln

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/health", s.handleHealth)
	mux.HandleFunc("/v1/join", s.handleJoin)
	s.httpServer = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}

	go func() {
		if err := s.httpServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.onEvent("interop listener error: %v", err)
		}
	}()
	return nil
}

// Stop shuts the listener down and removes the discovery file.
func (s *Server) Stop() {
	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.httpServer.Shutdown(ctx)
	}
	RemoveDiscovery(s.app)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, ErrBadRequest, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"app":              s.app,
		"protocol_version": ProtocolVersion,
		"ready":            true,
	})
}

func (s *Server) handleJoin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, ErrBadRequest, "method not allowed")
		return
	}
	if !s.authorized(r) {
		s.onEvent("join rejected: bad or missing bearer token")
		writeError(w, http.StatusUnauthorized, ErrUnauthorized, "missing or invalid bearer token")
		return
	}

	// Size cap (INTEROP-SPEC §3).
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, ErrPayloadTooLarge,
			fmt.Sprintf("body exceeds %d bytes", MaxBodyBytes))
		return
	}

	var tree map[string]interface{}
	if err := json.Unmarshal(raw, &tree); err != nil {
		writeError(w, http.StatusBadRequest, ErrBadRequest, "body is not a JSON object: "+err.Error())
		return
	}

	// Credential firewall (INTEROP-SPEC §5): the handoff must never carry
	// authentication material.
	if ContainsCredentialKeys(tree) {
		s.onEvent("join rejected: payload contains credential-looking keys")
		writeError(w, http.StatusUnprocessableEntity, ErrCredentialInPayload,
			"handoff payloads must not carry credentials; the receiver uses its own account")
		return
	}

	var payload HandoffPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		writeError(w, http.StatusBadRequest, ErrBadRequest, "cannot decode handoff payload: "+err.Error())
		return
	}

	// Version negotiation (INTEROP-SPEC §8) before anything else.
	if payload.Version != ProtocolVersion {
		s.onEvent("join rejected: protocol version %d", payload.Version)
		writeVersionMismatch(w, fmt.Sprintf("protocol version %d is not supported; please update the out-of-date app", payload.Version))
		return
	}

	now := time.Now()
	// Expiry is reported with its own code; malformed timestamps stay
	// bad_request.
	if payload.ExpiresAt == "" {
		writeError(w, http.StatusBadRequest, ErrBadRequest, "expires_at is required")
		return
	}
	expires, err := time.Parse(time.RFC3339, payload.ExpiresAt)
	if err != nil {
		writeError(w, http.StatusBadRequest, ErrBadRequest, "expires_at must be RFC 3339")
		return
	}
	if !expires.After(now) {
		writeError(w, http.StatusUnprocessableEntity, ErrHandoffExpired, "handoff expired before delivery")
		return
	}

	if err := payload.Validate(now); err != nil {
		s.onEvent("join rejected: %v", err)
		writeError(w, http.StatusBadRequest, ErrBadRequest, err.Error())
		return
	}

	if payload.Nonce != "" {
		if !s.recordNonce(payload.Nonce, now) {
			s.onEvent("join rejected: replayed nonce for handoff %s", payload.HandoffID)
			writeError(w, http.StatusConflict, ErrReplayedNonce, "nonce was already used")
			return
		}
	}

	if s.accept != nil {
		if err := s.accept(&payload); err != nil {
			if errors.Is(err, ErrSlotBusy) {
				writeError(w, http.StatusServiceUnavailable, ErrBusy,
					"another match handoff is already pending in SamuelClient")
				return
			}
			writeError(w, http.StatusInternalServerError, ErrInternal, err.Error())
			return
		}
	}

	s.onEvent("handoff accepted: %s match=%s target=%s:%d",
		payload.HandoffID, payload.Match.MatchID, payload.Match.JoinTarget.Host, payload.Match.JoinTarget.Port)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":     "accepted",
		"handoff_id": payload.HandoffID,
	})
}

func (s *Server) authorized(r *http.Request) bool {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if len(h) <= len(prefix) || h[:len(prefix)] != prefix {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(h[len(prefix):]), []byte(s.token)) == 1
}

// recordNonce stores a nonce and reports false when it was already seen
// within the replay window.
func (s *Server) recordNonce(nonce string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Prune expired entries first so the window is honest.
	for k, seen := range s.seenNonces {
		if now.Sub(seen) > NonceWindow {
			delete(s.seenNonces, k)
		}
	}
	if _, dup := s.seenNonces[nonce]; dup {
		return false
	}
	// Bound the cache; evict the oldest entry when full.
	if len(s.seenNonces) >= NonceCacheCap {
		var oldestK string
		var oldestT time.Time
		first := true
		for k, t := range s.seenNonces {
			if first || t.Before(oldestT) {
				oldestK, oldestT, first = k, t, false
			}
		}
		delete(s.seenNonces, oldestK)
	}
	s.seenNonces[nonce] = now
	return true
}

func writeVersionMismatch(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnprocessableEntity)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"code":      ErrVersionMismatch,
			"message":   message,
			"supported": []int{ProtocolVersion},
		},
	})
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{"code": code, "message": message},
	})
}

func writeJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
