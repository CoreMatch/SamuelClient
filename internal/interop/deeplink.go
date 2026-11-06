package interop

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// DeepLink is the parsed invocation hint (INTEROP-SPEC §7 flow B). It
// carries only the handoff id and protocol version — never credentials or
// match parameters.
type DeepLink struct {
	HandoffID string
	Version   int
}

// ParseDeepLink validates a matchcore://join?... URL. Any deviation from the
// spec grammar is an error; callers log and ignore invalid invocations.
func ParseDeepLink(raw string) (*DeepLink, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("unparseable deep link: %w", err)
	}
	if !strings.EqualFold(u.Scheme, Scheme) {
		return nil, fmt.Errorf("not a %s:// deep link", Scheme)
	}
	if !strings.EqualFold(u.Host, "join") && !strings.EqualFold(u.Path, "join") &&
		!strings.EqualFold(u.Host+u.Path, "join") {
		return nil, fmt.Errorf("unsupported deep link action %q (only join is defined)", u.Host+u.Path)
	}

	q := u.Query()
	handoffID := q.Get("handoff_id")
	if handoffID == "" || len(handoffID) > MaxHandoffIDLen || !uuidRe.MatchString(handoffID) {
		return nil, fmt.Errorf("deep link handoff_id must be a uuid")
	}

	version := ProtocolVersion
	if v := q.Get("v"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed < 1 {
			return nil, fmt.Errorf("deep link v must be a positive integer")
		}
		version = parsed
	}

	return &DeepLink{HandoffID: handoffID, Version: version}, nil
}

// FindDeepLinkArg scans process argv for the first matchcore:// URL (the OS
// passes the deep link as an argument through the .desktop Exec line).
func FindDeepLinkArg(args []string) string {
	for _, a := range args {
		if strings.HasPrefix(a, Scheme+"://") {
			return a
		}
	}
	return ""
}

// BuildDeepLink renders a canonical invocation hint for tests and for the
// sender side documentation.
func BuildDeepLink(handoffID string, version int) string {
	return fmt.Sprintf("%s://join?handoff_id=%s&v=%d", Scheme, handoffID, version)
}
