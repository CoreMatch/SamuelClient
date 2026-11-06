package interop

import "testing"

func TestParseDeepLinkValid(t *testing.T) {
	link, err := ParseDeepLink("matchcore://join?handoff_id=9b2f0c1e-3a7d-4e5f-8b21-6c9d0e1f2a3b&v=1")
	if err != nil {
		t.Fatalf("ParseDeepLink: %v", err)
	}
	if link.HandoffID != "9b2f0c1e-3a7d-4e5f-8b21-6c9d0e1f2a3b" || link.Version != 1 {
		t.Errorf("parsed = %+v", link)
	}
}

func TestParseDeepLinkDefaultsVersion(t *testing.T) {
	link, err := ParseDeepLink("matchcore://join?handoff_id=9b2f0c1e-3a7d-4e5f-8b21-6c9d0e1f2a3b")
	if err != nil {
		t.Fatalf("ParseDeepLink: %v", err)
	}
	if link.Version != ProtocolVersion {
		t.Errorf("default version = %d, want %d", link.Version, ProtocolVersion)
	}
}

func TestParseDeepLinkRejects(t *testing.T) {
	cases := []string{
		"",
		"https://example.com/join",
		"matchcore://steal?handoff_id=9b2f0c1e-3a7d-4e5f-8b21-6c9d0e1f2a3b",
		"matchcore://join",
		"matchcore://join?handoff_id=not-a-uuid",
		"matchcore://join?handoff_id=9b2f0c1e-3a7d-4e5f-8b21-6c9d0e1f2a3b&v=0",
		"matchcore://join?handoff_id=9b2f0c1e-3a7d-4e5f-8b21-6c9d0e1f2a3b&v=x",
		// Credentials must never ride along — extra params are ignored, but
		// the link itself stays valid only with a proper id; this one smuggles
		// nothing and is fine, so it is asserted separately below.
	}
	for _, raw := range cases {
		if _, err := ParseDeepLink(raw); err == nil {
			t.Errorf("ParseDeepLink(%q) accepted, want error", raw)
		}
	}
}

func TestParseDeepLinkIgnoresExtraParams(t *testing.T) {
	// The deep link is an invocation hint: unknown query params are
	// ignored, they never extend the contract.
	link, err := ParseDeepLink("matchcore://join?handoff_id=9b2f0c1e-3a7d-4e5f-8b21-6c9d0e1f2a3b&utm_source=x")
	if err != nil {
		t.Fatalf("ParseDeepLink: %v", err)
	}
	if link.HandoffID != "9b2f0c1e-3a7d-4e5f-8b21-6c9d0e1f2a3b" {
		t.Errorf("parsed = %+v", link)
	}
}

func TestFindDeepLinkArg(t *testing.T) {
	args := []string{"--something", "matchcore://join?handoff_id=abc", "other"}
	if got := FindDeepLinkArg(args); got != "matchcore://join?handoff_id=abc" {
		t.Errorf("FindDeepLinkArg = %q", got)
	}
	if got := FindDeepLinkArg([]string{"plain"}); got != "" {
		t.Errorf("FindDeepLinkArg = %q, want empty", got)
	}
}

func TestBuildDeepLinkRoundTrip(t *testing.T) {
	raw := BuildDeepLink("9b2f0c1e-3a7d-4e5f-8b21-6c9d0e1f2a3b", 1)
	link, err := ParseDeepLink(raw)
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	if link.HandoffID != "9b2f0c1e-3a7d-4e5f-8b21-6c9d0e1f2a3b" || link.Version != 1 {
		t.Errorf("round trip = %+v", link)
	}
}
