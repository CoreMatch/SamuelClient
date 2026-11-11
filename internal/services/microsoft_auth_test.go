package services

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// pointMicrosoftEverywhere aims every Microsoft/Mojang base and the HRPAuth
// exchange endpoint at one test server for the duration of a test.
func pointMicrosoftEverywhere(t *testing.T, base string) {
	t.Helper()
	oldTenant, oldClient := MicrosoftTenant, MicrosoftClientID
	oldMSA, oldXBL, oldXSTS, oldMC := MSABaseURL, XBLBaseURL, XSTSBaseURL, MCBaseURL
	oldHRP := HRPAuthBaseURL
	MicrosoftTenant = "consumers"
	MicrosoftClientID = "test-client"
	MSABaseURL, XBLBaseURL, XSTSBaseURL, MCBaseURL = base, base, base, base
	HRPAuthBaseURL = base
	t.Cleanup(func() {
		MicrosoftTenant, MicrosoftClientID = oldTenant, oldClient
		MSABaseURL, XBLBaseURL, XSTSBaseURL, MCBaseURL = oldMSA, oldXBL, oldXSTS, oldMC
		HRPAuthBaseURL = oldHRP
	})
}

// newMicrosoftMockServer mimics the mcmock chain plus the HA exchange.
func newMicrosoftMockServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/consumers/oauth2/v2.0/devicecode", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"device_code": "dev-1", "user_code": "ABCD1234",
			"verification_uri": "https://mock/device", "expires_in": 900, "interval": 0,
		})
	})
	mux.HandleFunc("/consumers/oauth2/v2.0/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		grant := r.FormValue("grant_type")
		switch grant {
		case "urn:ietf:params:oauth:grant-type:device_code", "refresh_token":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"token_type": "Bearer", "expires_in": 3600,
				"access_token": "msa-access", "refresh_token": "msa-refresh",
			})
		default:
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "unsupported_grant_type"})
		}
	})
	mux.HandleFunc("/user/authenticate", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "d=msa-access") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"Token":         "xbl-token",
			"DisplayClaims": map[string]interface{}{"xui": []map[string]string{{"uhs": "uhsX"}}},
		})
	})
	mux.HandleFunc("/xsts/authorize", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "xbl-token") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"Token":         "xsts-token",
			"DisplayClaims": map[string]interface{}{"xui": []map[string]string{{"uhs": "uhsX"}}},
		})
	})
	mux.HandleFunc("/authentication/login_with_xbox", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "XBL3.0 x=uhsX;xsts-token") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"access_token": "mc-token", "expires_in": 86400})
	})
	mux.HandleFunc("/authserver/microsoft", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "mc-token") {
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]string{"error": "ForbiddenOperationException", "errorMessage": "Invalid credentials."})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"accessToken": "ygg-access", "clientToken": "ygg-client",
			"availableProfiles": []map[string]string{{"id": "puuid", "name": "GenuinePlayer"}},
			"selectedProfile":   map[string]string{"id": "puuid", "name": "GenuinePlayer"},
			"claimRequired":     true,
		})
	})
	return httptest.NewServer(mux)
}

func TestMicrosoftFullChainAndExchange(t *testing.T) {
	srv := newMicrosoftMockServer(t)
	defer srv.Close()
	pointMicrosoftEverywhere(t, srv.URL)

	ms := NewMicrosoftService()

	dc, err := ms.StartDeviceCode()
	if err != nil {
		t.Fatalf("StartDeviceCode: %v", err)
	}
	if dc.UserCode != "ABCD1234" {
		t.Errorf("UserCode = %q, want ABCD1234", dc.UserCode)
	}

	tokens, err := ms.PollDeviceCode(dc)
	if err != nil {
		t.Fatalf("PollDeviceCode: %v", err)
	}
	if tokens.AccessToken != "msa-access" || tokens.RefreshToken != "msa-refresh" {
		t.Fatalf("MSA tokens = %+v", tokens)
	}

	mcToken, err := ms.FullChain(tokens.AccessToken)
	if err != nil {
		t.Fatalf("FullChain: %v", err)
	}
	if mcToken != "mc-token" {
		t.Fatalf("mcToken = %q, want mc-token", mcToken)
	}

	exch, err := ms.Exchange(mcToken, "")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if exch.AccessToken != "ygg-access" {
		t.Errorf("accessToken = %q, want ygg-access", exch.AccessToken)
	}
	if !exch.ClaimRequired {
		t.Error("claimRequired = false, want true for absorbed account")
	}
	if exch.SelectedProfile == nil || exch.SelectedProfile.Name != "GenuinePlayer" {
		t.Errorf("selectedProfile = %+v", exch.SelectedProfile)
	}
}

func TestMicrosoftRefresh(t *testing.T) {
	srv := newMicrosoftMockServer(t)
	defer srv.Close()
	pointMicrosoftEverywhere(t, srv.URL)

	tokens, err := NewMicrosoftService().RefreshMSA("old-refresh")
	if err != nil {
		t.Fatalf("RefreshMSA: %v", err)
	}
	if tokens.AccessToken != "msa-access" {
		t.Errorf("refreshed access = %q, want msa-access", tokens.AccessToken)
	}
}

func TestMicrosoftExchangeRejected(t *testing.T) {
	srv := newMicrosoftMockServer(t)
	defer srv.Close()
	pointMicrosoftEverywhere(t, srv.URL)

	_, err := NewMicrosoftService().Exchange("wrong-token", "")
	if err == nil {
		t.Fatal("expected exchange to fail for a rejected token")
	}
	if !strings.Contains(err.Error(), "Invalid credentials") {
		t.Errorf("error = %q, want the upstream errorMessage surfaced", err)
	}
}
