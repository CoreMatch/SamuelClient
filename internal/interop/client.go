package interop

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ErrHandoffGone means the sender no longer holds the handoff (unknown,
// consumed, or expired) — INTEROP-SPEC §4.2 `404 gone`.
var ErrHandoffGone = errors.New("handoff gone on the sender side")

// FetchPendingHandoff pulls one handoff from the sender's loopback endpoint
// (INTEROP-SPEC §7 flow B step 4). The sender discovery document must have
// been pid-checked by the caller.
func FetchPendingHandoff(sender *DiscoveryFile, handoffID string, timeout time.Duration) (*HandoffPayload, error) {
	if sender == nil || sender.Port < 1 || sender.Token == "" {
		return nil, fmt.Errorf("invalid sender discovery document")
	}
	url := fmt.Sprintf("http://127.0.0.1:%d/v1/handoff/%s", sender.Port, handoffID)

	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+sender.Token)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("reach sender loopback: %w", err)
	}
	defer resp.Body.Close()

	body := io.LimitReader(resp.Body, MaxBodyBytes)
	switch resp.StatusCode {
	case http.StatusOK:
		var payload HandoffPayload
		if err := json.NewDecoder(body).Decode(&payload); err != nil {
			return nil, fmt.Errorf("decode handoff from sender: %w", err)
		}
		return &payload, nil
	case http.StatusNotFound:
		return nil, ErrHandoffGone
	default:
		return nil, fmt.Errorf("sender returned http %d", resp.StatusCode)
	}
}

// LoadLiveSenderDiscovery reads the sender's discovery file and verifies the
// recorded pid is alive. A missing, unreadable, or stale document returns
// (nil, nil) so callers can retry without treating it as an error.
func LoadLiveSenderDiscovery() (*DiscoveryFile, error) {
	doc, err := ReadDiscovery(AppSender)
	if err != nil {
		return nil, nil
	}
	if !PidAlive(doc.Pid) {
		return nil, nil
	}
	return doc, nil
}
