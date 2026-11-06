// Package interop implements the MatchCore ↔ SamuelClient match handoff
// protocol (INTEROP-SPEC.md, protocol major 1): a loopback-only HTTP
// listener authenticated by a per-process random token published in a 0600
// discovery file, plus the deep-link invocation hint parsing.
//
// Security posture (see INTEROP-SPEC §11): loopback bind only, same-user
// file permissions, no credentials ever cross the process boundary, strict
// host validation doubling as argument-injection protection.
package interop

import (
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"
)

// ProtocolVersion is the protocol major implemented here.
const ProtocolVersion = 1

// Scheme is the deep-link URL scheme registered with the OS.
const Scheme = "matchcore"

// Limits enforced on incoming handoff payloads (INTEROP-SPEC §5).
const (
	MaxBodyBytes     = 64 << 10 // 64 KiB request body cap
	MaxHandoffIDLen  = 64
	MaxNonceLen      = 64
	MaxMatchIDLen    = 128
	MaxModeLen       = 64
	MaxMapNameLen    = 64
	MaxHostLen       = 253
	MaxGameVerLen    = 32
	MaxPlayers       = 24
	MaxUsernameLen   = 32
	MaxPlayerUUIDLen = 36

	// NonceWindow is how long seen nonces are remembered for replay
	// protection.
	NonceWindow = 10 * time.Minute
	// NonceCacheCap bounds the replay cache.
	NonceCacheCap = 1024
)

// ReachabilityTimeout bounds the pre-launch TCP dial of the join target.
const ReachabilityTimeout = 3 * time.Second

// JoinTarget is the match server the game must connect to.
type JoinTarget struct {
	Host        string `json:"host"`
	Port        int    `json:"port"`
	GameVersion string `json:"game_version,omitempty"`
}

// PlayerEntry is one participant listed by the sender (informational).
type PlayerEntry struct {
	Username string `json:"username"`
	UUID     string `json:"uuid"`
}

// MatchInfo describes the finished matchmaking result.
type MatchInfo struct {
	MatchID    string        `json:"match_id"`
	Mode       string        `json:"mode"`
	MapName    string        `json:"map_name,omitempty"`
	JoinTarget JoinTarget    `json:"join_target"`
	Players    []PlayerEntry `json:"players,omitempty"`
}

// HandoffPayload is the v1 handoff body pushed by the sender (or pulled by
// the receiver after a deep-link wake-up). It must never carry credentials;
// the receiver rejects payloads containing credential-looking keys.
type HandoffPayload struct {
	Version   int       `json:"version"`
	HandoffID string    `json:"handoff_id"`
	Nonce     string    `json:"nonce,omitempty"`
	CreatedAt string    `json:"created_at"`
	ExpiresAt string    `json:"expires_at"`
	Match     MatchInfo `json:"match"`
}

// DiscoveryFile is the 0600 rendezvous document each process publishes
// (INTEROP-SPEC §2).
type DiscoveryFile struct {
	App             string `json:"app"`
	ProtocolVersion int    `json:"protocol_version"`
	Pid             int    `json:"pid"`
	Port            int    `json:"port"`
	Token           string `json:"token"`
	CreatedAt       string `json:"created_at"`
}

// App names used in discovery files.
const (
	AppReceiver = "samuel-client"
	AppSender   = "mca-client"
)

// Error codes returned in {"error":{"code,message}} bodies
// (INTEROP-SPEC §9).
const (
	ErrBadRequest          = "bad_request"
	ErrUnauthorized        = "unauthorized"
	ErrGone                = "gone"
	ErrReplayedNonce       = "replayed_nonce"
	ErrPayloadTooLarge     = "payload_too_large"
	ErrVersionMismatch     = "version_mismatch"
	ErrHandoffExpired      = "handoff_expired"
	ErrCredentialInPayload = "credential_forbidden"
	ErrBusy                = "busy"
	ErrInternal            = "internal"
)

var (
	uuidRe     = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	hostnameRe = regexp.MustCompile(`(?i)^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)*$`)
	modeRe     = regexp.MustCompile(`^[a-z0-9_-]+$`)
	gameVerRe  = regexp.MustCompile(`^[0-9a-z.\-+]+$`)

	// credentialKeys are rejected anywhere in the payload
	// (INTEROP-SPEC §5 credential firewall).
	credentialKeys = []string{"access_token", "client_token", "password", "token", "secret"}
)

// Validate checks the payload against INTEROP-SPEC §5. It returns a readable
// error describing the first violation; now is the reference time for the
// expiry check.
func (p *HandoffPayload) Validate(now time.Time) error {
	if p.Version != ProtocolVersion {
		return fmt.Errorf("unsupported protocol version %d (supported: %d)", p.Version, ProtocolVersion)
	}
	if p.HandoffID == "" || len(p.HandoffID) > MaxHandoffIDLen || !uuidRe.MatchString(p.HandoffID) {
		return fmt.Errorf("handoff_id must be a uuid (max %d chars)", MaxHandoffIDLen)
	}
	if len(p.Nonce) > MaxNonceLen {
		return fmt.Errorf("nonce longer than %d chars", MaxNonceLen)
	}
	if p.ExpiresAt == "" {
		return fmt.Errorf("expires_at is required")
	}
	expires, err := time.Parse(time.RFC3339, p.ExpiresAt)
	if err != nil {
		return fmt.Errorf("expires_at must be RFC 3339: %v", err)
	}
	if !expires.After(now) {
		return fmt.Errorf("handoff expired at %s", p.ExpiresAt)
	}
	if p.CreatedAt != "" {
		if _, err := time.Parse(time.RFC3339, p.CreatedAt); err != nil {
			return fmt.Errorf("created_at must be RFC 3339: %v", err)
		}
	}

	m := p.Match
	if m.MatchID == "" || len(m.MatchID) > MaxMatchIDLen {
		return fmt.Errorf("match.match_id required (1-%d chars)", MaxMatchIDLen)
	}
	for _, r := range m.MatchID {
		if r < 0x20 || r > 0x7e {
			return fmt.Errorf("match.match_id must be printable ASCII")
		}
	}
	if m.Mode == "" || len(m.Mode) > MaxModeLen || !modeRe.MatchString(m.Mode) {
		return fmt.Errorf("match.mode required, 1-%d chars of [a-z0-9_-]", MaxModeLen)
	}
	if len(m.MapName) > MaxMapNameLen {
		return fmt.Errorf("match.map_name longer than %d chars", MaxMapNameLen)
	}
	if err := m.JoinTarget.Validate(); err != nil {
		return err
	}
	if len(m.Players) > MaxPlayers {
		return fmt.Errorf("match.players has %d entries (max %d)", len(m.Players), MaxPlayers)
	}
	for i, pl := range m.Players {
		if len(pl.Username) > MaxUsernameLen {
			return fmt.Errorf("match.players[%d].username longer than %d chars", i, MaxUsernameLen)
		}
		if len(pl.UUID) > MaxPlayerUUIDLen {
			return fmt.Errorf("match.players[%d].uuid longer than %d chars", i, MaxPlayerUUIDLen)
		}
	}
	return nil
}

// Validate checks the join target host/port (INTEROP-SPEC §5). The host
// grammar is deliberately strict because the value ends up on the game
// command line (--server), where it must not be able to inject arguments.
func (t *JoinTarget) Validate() error {
	if t.Host == "" || len(t.Host) > MaxHostLen {
		return fmt.Errorf("match.join_target.host required (max %d chars)", MaxHostLen)
	}
	host := t.Host
	// net.ParseIP covers IPv4/IPv6 literals; the hostname grammar (no
	// colons) covers DNS names. Anything else is rejected.
	if net.ParseIP(host) == nil && !hostnameRe.MatchString(host) {
		return fmt.Errorf("match.join_target.host is not a valid hostname or ip")
	}
	if t.Port < 1 || t.Port > 65535 {
		return fmt.Errorf("match.join_target.port must be 1-65535")
	}
	if t.GameVersion != "" {
		if len(t.GameVersion) > MaxGameVerLen || !gameVerRe.MatchString(t.GameVersion) {
			return fmt.Errorf("match.join_target.game_version must be 1-%d chars of [0-9a-z.-+]", MaxGameVerLen)
		}
	}
	return nil
}

// ContainsCredentialKeys reports whether any key in the raw JSON tree
// matches the credential firewall list (INTEROP-SPEC §5). The raw body is
// scanned as decoded generic JSON so nested/unknown locations are covered.
func ContainsCredentialKeys(raw map[string]interface{}) bool {
	var walk func(v interface{}) bool
	walk = func(v interface{}) bool {
		switch node := v.(type) {
		case map[string]interface{}:
			for k, child := range node {
				lk := strings.ToLower(k)
				for _, banned := range credentialKeys {
					if lk == banned {
						return true
					}
				}
				if walk(child) {
					return true
				}
			}
		case []interface{}:
			for _, child := range node {
				if walk(child) {
					return true
				}
			}
		}
		return false
	}
	return walk(raw)
}

// CheckReachable dials host:port with a bounded timeout (INTEROP-SPEC §6
// preflight). It is used before launch so an unreachable match server
// degrades readably instead of spawning a doomed game process.
func CheckReachable(host string, port int, timeout time.Duration) error {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), timeout)
	if err != nil {
		return err
	}
	_ = conn.Close()
	return nil
}
