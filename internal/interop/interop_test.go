package interop

import (
	"net"
	"strings"
	"testing"
	"time"
)

// validPayload returns a spec-conforming v1 handoff for reuse.
func validPayload() *HandoffPayload {
	return &HandoffPayload{
		Version:   ProtocolVersion,
		HandoffID: "9b2f0c1e-3a7d-4e5f-8b21-6c9d0e1f2a3b",
		Nonce:     "11111111-2222-4333-8444-555555555555",
		CreatedAt: "2026-11-06T10:00:00+08:00",
		ExpiresAt: "2099-01-01T00:00:00Z",
		Match: MatchInfo{
			MatchID: "m-1",
			Mode:    "bedwars",
			MapName: "Aquarium",
			JoinTarget: JoinTarget{
				Host:        "bw.match.local",
				Port:        25565,
				GameVersion: "1.20.4",
			},
			Players: []PlayerEntry{{Username: "alice", UUID: "9b2f0c1e3a7d4e5f8b216c9d0e1f2a3b"}},
		},
	}
}

func TestPayloadValidation(t *testing.T) {
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)

	t.Run("valid payload passes", func(t *testing.T) {
		if err := validPayload().Validate(now); err != nil {
			t.Fatalf("valid payload rejected: %v", err)
		}
	})

	cases := []struct {
		name    string
		mutate  func(p *HandoffPayload)
		wantSub string
	}{
		{"wrong version", func(p *HandoffPayload) { p.Version = 2 }, "unsupported protocol version"},
		{"missing handoff id", func(p *HandoffPayload) { p.HandoffID = "" }, "handoff_id"},
		{"non uuid handoff id", func(p *HandoffPayload) { p.HandoffID = "not-a-uuid" }, "handoff_id"},
		{"long nonce", func(p *HandoffPayload) { p.Nonce = strings.Repeat("n", MaxNonceLen+1) }, "nonce"},
		{"missing expires", func(p *HandoffPayload) { p.ExpiresAt = "" }, "expires_at"},
		{"bad expires format", func(p *HandoffPayload) { p.ExpiresAt = "tomorrow" }, "expires_at"},
		{"expired", func(p *HandoffPayload) { p.ExpiresAt = "2020-01-01T00:00:00Z" }, "expired"},
		{"bad created format", func(p *HandoffPayload) { p.CreatedAt = "yesterday" }, "created_at"},
		{"missing match id", func(p *HandoffPayload) { p.Match.MatchID = "" }, "match_id"},
		{"long match id", func(p *HandoffPayload) { p.Match.MatchID = strings.Repeat("m", MaxMatchIDLen+1) }, "match_id"},
		{"non ascii match id", func(p *HandoffPayload) { p.Match.MatchID = "m-é" }, "printable ASCII"},
		{"bad mode charset", func(p *HandoffPayload) { p.Match.Mode = "Bed Wars!" }, "mode"},
		{"missing mode", func(p *HandoffPayload) { p.Match.Mode = "" }, "mode"},
		{"long map name", func(p *HandoffPayload) { p.Match.MapName = strings.Repeat("x", MaxMapNameLen+1) }, "map_name"},
		{"missing host", func(p *HandoffPayload) { p.Match.JoinTarget.Host = "" }, "host"},
		{"host with scheme", func(p *HandoffPayload) { p.Match.JoinTarget.Host = "https://evil" }, "host"},
		{"host with space", func(p *HandoffPayload) { p.Match.JoinTarget.Host = "bw match" }, "host"},
		{"host arg injection", func(p *HandoffPayload) { p.Match.JoinTarget.Host = "a --server evil" }, "host"},
		{"long host", func(p *HandoffPayload) { p.Match.JoinTarget.Host = strings.Repeat("a", MaxHostLen+1) }, "host"},
		{"port zero", func(p *HandoffPayload) { p.Match.JoinTarget.Port = 0 }, "port"},
		{"port too big", func(p *HandoffPayload) { p.Match.JoinTarget.Port = 65536 }, "port"},
		{"bad game version", func(p *HandoffPayload) { p.Match.JoinTarget.GameVersion = "1.20; rm" }, "game_version"},
		{"too many players", func(p *HandoffPayload) {
			p.Match.Players = make([]PlayerEntry, MaxPlayers+1)
		}, "players"},
		{"long player name", func(p *HandoffPayload) {
			p.Match.Players = []PlayerEntry{{Username: strings.Repeat("u", MaxUsernameLen+1)}}
		}, "username"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := validPayload()
			tc.mutate(p)
			err := p.Validate(now)
			if err == nil {
				t.Fatalf("expected rejection, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("error %q does not mention %q", err, tc.wantSub)
			}
		})
	}
}

func TestJoinTargetAcceptsIPLiterals(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "192.168.1.10", "::1", "2001:db8::42"} {
		tgt := JoinTarget{Host: host, Port: 25565}
		if err := tgt.Validate(); err != nil {
			t.Errorf("ip literal %q rejected: %v", host, err)
		}
	}
}

func TestContainsCredentialKeys(t *testing.T) {
	clean := map[string]interface{}{
		"version": 1,
		"match":   map[string]interface{}{"match_id": "m", "players": []interface{}{map[string]interface{}{"username": "a"}}},
	}
	if ContainsCredentialKeys(clean) {
		t.Error("clean payload flagged as carrying credentials")
	}

	dirty := map[string]interface{}{
		"match": map[string]interface{}{"nested": []interface{}{map[string]interface{}{"Access_Token": "x"}}},
	}
	if !ContainsCredentialKeys(dirty) {
		t.Error("nested credential key not detected")
	}
}

func TestCheckReachable(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()

	port := ln.Addr().(*net.TCPAddr).Port
	if err := CheckReachable("127.0.0.1", port, time.Second); err != nil {
		t.Errorf("reachable target reported unreachable: %v", err)
	}
	if err := CheckReachable("127.0.0.1", 1, 500*time.Millisecond); err == nil {
		t.Error("closed port reported reachable")
	}
}
