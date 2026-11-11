package services

// Microsoft login mapped onto HA accounts (SamuelClient knowledge/0007).
//
// The client performs the public Microsoft OAuth chain itself and then
// exchanges the resulting Minecraft Services access token at the HA side
// (POST {HRPAuth}/authserver/microsoft), which re-verifies the token and
// answers a standard Yggdrasil session plus claimRequired. All tokens stay
// opaque to the client — it never parses or validates them.
//
// Every endpoint base is a swappable var. The defaults point at the local
// mcmock service (HRPAuth-Yggdrasil-API tools/mcmock); once the Azure
// application passes Mojang's allowlist review only these values change,
// never the code path.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Configurable Microsoft / Mojang endpoint bases. Production values are the
// official hosts; the shipped defaults target the local mock.
var (
	MicrosoftTenant   = "consumers"
	MicrosoftClientID = "mock-client-id"
	MSABaseURL        = "http://127.0.0.1:8088" // https://login.microsoftonline.com
	XBLBaseURL        = "http://127.0.0.1:8088" // https://user.auth.xboxlive.com
	XSTSBaseURL       = "http://127.0.0.1:8088" // https://xsts.auth.xboxlive.com
	MCBaseURL         = "http://127.0.0.1:8088" // https://api.minecraftservices.com
)

// MicrosoftScope is the MSA scope requested for Xbox Live sign-in with a
// refresh token (offline_access) so the session can be renewed for 90 days.
const MicrosoftScope = "XboxLive.signin offline_access"

// Typed failures surfaced to the UI (knowledge/0007 §7 failure-mode list).
var (
	ErrMSAAuthorizationPending = errors.New("authorization_pending")
	ErrMSASlowDown             = errors.New("slow_down")
	ErrMSAExpired              = errors.New("expired_token")
	ErrMSADeclined             = errors.New("authorization_declined")
	ErrMSAGrantFailed          = errors.New("microsoft grant failed")
	ErrXboxChainFailed         = errors.New("xbox chain failed")
	ErrMCLoginFailed           = errors.New("minecraft login failed")
)

type MicrosoftService struct {
	client *http.Client
}

func NewMicrosoftService() *MicrosoftService {
	return &MicrosoftService{client: &http.Client{Timeout: 30 * time.Second}}
}

// DeviceCodeResponse is the MSA device-code grant start answer.
type DeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// MSATokens is the MSA token endpoint answer (subset).
type MSATokens struct {
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	ExpiresIn    int    `json:"expires_in"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// MCProfile is the Minecraft Services profile (subset) used for display only;
// the identity that matters is re-fetched server-side during the exchange.
type MCProfile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ExchangeResult is the HA /authserver/microsoft answer.
type ExchangeResult struct {
	AccessToken       string           `json:"accessToken"`
	ClientToken       string           `json:"clientToken"`
	AvailableProfiles []AuthlibProfile `json:"availableProfiles"`
	SelectedProfile   *AuthlibProfile  `json:"selectedProfile"`
	ClaimRequired     bool             `json:"claimRequired"`
}

// StartDeviceCode begins the MSA device-code flow. The caller shows
// UserCode + VerificationURI to the player, then polls PollDeviceCode.
func (m *MicrosoftService) StartDeviceCode() (*DeviceCodeResponse, error) {
	form := url.Values{}
	form.Set("client_id", MicrosoftClientID)
	form.Set("scope", MicrosoftScope)

	endpoint := fmt.Sprintf("%s/%s/oauth2/v2.0/devicecode", strings.TrimRight(MSABaseURL, "/"), MicrosoftTenant)
	body, status, err := m.postForm(endpoint, form)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("%w: devicecode status %d", ErrMSAGrantFailed, status)
	}
	var resp DeviceCodeResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	if resp.DeviceCode == "" {
		return nil, fmt.Errorf("%w: empty device_code", ErrMSAGrantFailed)
	}
	return &resp, nil
}

// RequestDeviceCodeToken performs a single device-code token poll. It returns
// the tokens on success or a typed error (pending / slow_down / expired /
// declined / other).
func (m *MicrosoftService) RequestDeviceCodeToken(deviceCode string) (*MSATokens, error) {
	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	form.Set("client_id", MicrosoftClientID)
	form.Set("device_code", deviceCode)

	endpoint := fmt.Sprintf("%s/%s/oauth2/v2.0/token", strings.TrimRight(MSABaseURL, "/"), MicrosoftTenant)
	body, status, err := m.postForm(endpoint, form)
	if err != nil {
		return nil, err
	}
	if status == http.StatusOK {
		var tokens MSATokens
		if err := json.Unmarshal(body, &tokens); err != nil {
			return nil, err
		}
		if tokens.AccessToken == "" {
			return nil, fmt.Errorf("%w: empty access_token", ErrMSAGrantFailed)
		}
		return &tokens, nil
	}

	// Non-200: MSA answers {"error": "..."} for pending/slow_down/expired.
	var errResp struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(body, &errResp)
	switch errResp.Error {
	case "authorization_pending":
		return nil, ErrMSAAuthorizationPending
	case "slow_down":
		return nil, ErrMSASlowDown
	case "expired_token":
		return nil, ErrMSAExpired
	case "access_denied":
		return nil, ErrMSADeclined
	default:
		return nil, fmt.Errorf("%w: status %d (%s)", ErrMSAGrantFailed, status, errResp.Error)
	}
}

// PollDeviceCode polls the device-code token endpoint until it yields tokens,
// a terminal failure, or the device code expires. interval is honored and
// slow_down adds 5s backoff per the MSA spec.
func (m *MicrosoftService) PollDeviceCode(dc *DeviceCodeResponse) (*MSATokens, error) {
	interval := dc.Interval
	if interval <= 0 {
		interval = 5
	}
	deadline := time.Now().Add(time.Duration(dc.ExpiresIn) * time.Second)
	if dc.ExpiresIn <= 0 {
		deadline = time.Now().Add(15 * time.Minute)
	}

	for {
		if time.Now().After(deadline) {
			return nil, ErrMSAExpired
		}
		tokens, err := m.RequestDeviceCodeToken(dc.DeviceCode)
		if err == nil {
			return tokens, nil
		}
		switch {
		case errors.Is(err, ErrMSAAuthorizationPending):
			// keep waiting
		case errors.Is(err, ErrMSASlowDown):
			interval += 5
		default:
			return nil, err
		}
		time.Sleep(time.Duration(interval) * time.Second)
	}
}

// RefreshMSA exchanges an MSA refresh token for fresh tokens (90-day rolling).
func (m *MicrosoftService) RefreshMSA(refreshToken string) (*MSATokens, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", MicrosoftClientID)
	form.Set("refresh_token", refreshToken)
	form.Set("scope", MicrosoftScope)

	endpoint := fmt.Sprintf("%s/%s/oauth2/v2.0/token", strings.TrimRight(MSABaseURL, "/"), MicrosoftTenant)
	body, status, err := m.postForm(endpoint, form)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("%w: refresh status %d", ErrMSAGrantFailed, status)
	}
	var tokens MSATokens
	if err := json.Unmarshal(body, &tokens); err != nil {
		return nil, err
	}
	if tokens.AccessToken == "" {
		return nil, fmt.Errorf("%w: empty access_token on refresh", ErrMSAGrantFailed)
	}
	return &tokens, nil
}

// XboxLive exchanges the MSA access token for an Xbox Live token + uhs.
func (m *MicrosoftService) XboxLive(msaAccessToken string) (xblToken, uhs string, err error) {
	payload := map[string]interface{}{
		"Properties": map[string]interface{}{
			"AuthMethod": "RPS",
			"SiteName":   "user.auth.xboxlive.com",
			"RpsTicket":  "d=" + msaAccessToken,
		},
		"RelyingParty": "http://auth.xboxlive.com",
		"TokenType":    "JWT",
	}
	body, status, err := m.postJSON(strings.TrimRight(XBLBaseURL, "/")+"/user/authenticate", payload)
	if err != nil {
		return "", "", err
	}
	if status != http.StatusOK {
		return "", "", fmt.Errorf("%w: xbl status %d", ErrXboxChainFailed, status)
	}
	token, uhsVal, err := parseXboxResponse(body)
	if err != nil {
		return "", "", fmt.Errorf("%w: %v", ErrXboxChainFailed, err)
	}
	return token, uhsVal, nil
}

// XSTS exchanges the Xbox Live token for an XSTS token scoped to Minecraft
// Services, returning the token and the uhs (which must match the XBL uhs).
func (m *MicrosoftService) XSTS(xblToken string) (xstsToken, uhs string, err error) {
	payload := map[string]interface{}{
		"Properties": map[string]interface{}{
			"SandboxId":  "RETAIL",
			"UserTokens": []string{xblToken},
		},
		"RelyingParty": "rp://api.minecraftservices.com/",
		"TokenType":    "JWT",
	}
	body, status, err := m.postJSON(strings.TrimRight(XSTSBaseURL, "/")+"/xsts/authorize", payload)
	if err != nil {
		return "", "", err
	}
	if status != http.StatusOK {
		return "", "", fmt.Errorf("%w: xsts status %d", ErrXboxChainFailed, status)
	}
	token, uhsVal, err := parseXboxResponse(body)
	if err != nil {
		return "", "", fmt.Errorf("%w: %v", ErrXboxChainFailed, err)
	}
	return token, uhsVal, nil
}

// MCLogin trades the XSTS token + uhs for a Minecraft Services access token.
func (m *MicrosoftService) MCLogin(uhs, xstsToken string) (string, error) {
	payload := map[string]interface{}{
		"identityToken": "XBL3.0 x=" + uhs + ";" + xstsToken,
	}
	body, status, err := m.postJSON(strings.TrimRight(MCBaseURL, "/")+"/authentication/login_with_xbox", payload)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("%w: status %d", ErrMCLoginFailed, status)
	}
	var resp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", err
	}
	if resp.AccessToken == "" {
		return "", fmt.Errorf("%w: empty access_token", ErrMCLoginFailed)
	}
	return resp.AccessToken, nil
}

// FullChain runs XBL -> XSTS -> MC login from an MSA access token, returning
// the Minecraft Services access token. uhs consistency between XBL and XSTS
// is enforced.
func (m *MicrosoftService) FullChain(msaAccessToken string) (mcToken string, err error) {
	xblToken, xblUHS, err := m.XboxLive(msaAccessToken)
	if err != nil {
		return "", err
	}
	xstsToken, xstsUHS, err := m.XSTS(xblToken)
	if err != nil {
		return "", err
	}
	if xblUHS != xstsUHS {
		return "", fmt.Errorf("%w: uhs mismatch between XBL and XSTS", ErrXboxChainFailed)
	}
	return m.MCLogin(xstsUHS, xstsToken)
}

// Exchange trades the MC access token for an HA Yggdrasil session at
// POST {HRPAuth}/authserver/microsoft.
func (m *MicrosoftService) Exchange(mcAccessToken, clientToken string) (*ExchangeResult, error) {
	endpoint := strings.TrimSuffix(HRPAuthBaseURL, "/") + "/authserver/microsoft"
	payload := map[string]interface{}{"mcAccessToken": mcAccessToken}
	if clientToken != "" {
		payload["clientToken"] = clientToken
	}
	body, status, err := m.postJSON(endpoint, payload)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("exchange failed: status %d: %s", status, yggdrasilErrorMessage(body))
	}
	var resp ExchangeResult
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	if resp.AccessToken == "" {
		return nil, fmt.Errorf("exchange failed: empty accessToken")
	}
	return &resp, nil
}

// --- helpers ---

func (m *MicrosoftService) postForm(endpoint string, form url.Values) ([]byte, int, error) {
	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	return m.do(req)
}

func (m *MicrosoftService) postJSON(endpoint string, payload interface{}) ([]byte, int, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(string(data)))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	return m.do(req)
}

func (m *MicrosoftService) do(req *http.Request) ([]byte, int, error) {
	resp, err := m.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

// parseXboxResponse extracts Token and DisplayClaims.xui[0].uhs from an
// XBL/XSTS answer.
func parseXboxResponse(body []byte) (token, uhs string, err error) {
	var resp struct {
		Token         string `json:"Token"`
		DisplayClaims struct {
			Xui []struct {
				Uhs string `json:"uhs"`
			} `json:"xui"`
		} `json:"DisplayClaims"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", "", err
	}
	if resp.Token == "" {
		return "", "", errors.New("empty xbox token")
	}
	if len(resp.DisplayClaims.Xui) == 0 || resp.DisplayClaims.Xui[0].Uhs == "" {
		return "", "", errors.New("missing uhs")
	}
	return resp.Token, resp.DisplayClaims.Xui[0].Uhs, nil
}

// yggdrasilErrorMessage pulls errorMessage out of a Yggdrasil error body.
func yggdrasilErrorMessage(body []byte) string {
	var e struct {
		ErrorMessage string `json:"errorMessage"`
		Error        string `json:"error"`
	}
	if err := json.Unmarshal(body, &e); err != nil {
		return strings.TrimSpace(string(body))
	}
	if e.ErrorMessage != "" {
		return e.ErrorMessage
	}
	return e.Error
}
