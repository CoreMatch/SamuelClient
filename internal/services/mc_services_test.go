package services

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"HRPMLW/internal/database"
	"HRPMLW/internal/models"
	"HRPMLW/internal/repository"
)

// TestMain initializes one temporary SQLite database for the whole test
// binary (database.Initialize is sync.Once-guarded). The launcher-config
// tests rely on it; tests that need a clean settings state delete the
// launcher.* keys themselves.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "samuelclient-svctest")
	if err != nil {
		os.Exit(1)
	}
	if err := database.Initialize(&database.Config{Path: filepath.Join(dir, "svc.db")}); err != nil {
		os.Exit(1)
	}
	code := m.Run()
	_ = database.Close()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// ==================== AuthService ====================

const testMetaJSON = `{
	"meta": {
		"serverName": "TestAuth",
		"implementationName": "TestImpl",
		"implementationVersion": "1.0"
	},
	"signaturePublickey": "test-public-key",
	"skinDomains": ["skin.example.com"]
}`

// pointHRPAuthAt aims the built-in HRPAuth endpoint at a test server for the
// duration of one test and restores the production value afterwards.
func pointHRPAuthAt(t *testing.T, url string) {
	t.Helper()
	old := HRPAuthBaseURL
	HRPAuthBaseURL = url
	t.Cleanup(func() { HRPAuthBaseURL = old })
}

func TestIsHRPAuthAccount(t *testing.T) {
	cases := []struct {
		authType string
		want     bool
	}{
		{"hrpauth", true},
		{"HRPAuth", true},
		{"authlib-injector", true}, // legacy pre-migration value
		{"offline", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := IsHRPAuthAccount(tc.authType); got != tc.want {
			t.Errorf("IsHRPAuthAccount(%q) = %v, want %v", tc.authType, got, tc.want)
		}
	}
}

func TestGetHRPAuthMeta(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, testMetaJSON)
	}))
	defer srv.Close()
	pointHRPAuthAt(t, srv.URL)

	auth := NewAuthService()
	result := auth.GetHRPAuthMeta()
	if !result.Success {
		t.Fatalf("GetHRPAuthMeta failed: %s", result.Error)
	}

	data, ok := result.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("Data is %T, want map[string]interface{}", result.Data)
	}
	meta, ok := data["meta"].(*AuthlibServerMeta)
	if !ok {
		t.Fatalf("data[meta] is %T, want *AuthlibServerMeta", data["meta"])
	}
	if meta.Meta.ServerName != "TestAuth" {
		t.Errorf("ServerName = %q, want TestAuth", meta.Meta.ServerName)
	}
	if meta.SignaturePublickey != "test-public-key" {
		t.Errorf("SignaturePublickey = %q, want test-public-key", meta.SignaturePublickey)
	}
	raw, _ := data["raw"].(string)
	if !strings.Contains(raw, "TestAuth") {
		t.Errorf("raw body does not contain serverName: %q", raw)
	}
	if data["resolved_url"] != srv.URL {
		t.Errorf("resolved_url = %v, want %s", data["resolved_url"], srv.URL)
	}
}

// The HA core root and the Yggdrasil-API meta endpoint nest the metadata
// under a top-level "yggdrasil" key; the client must unwrap it so the
// prefetched document is the flat authlib-injector shape.
func TestGetHRPAuthMetaUnwrapsNestedYggdrasil(t *testing.T) {
	const nested = `{
		"success": true,
		"message": "HRPAuth core service is running.",
		"site": {"name": "HRPAuth"},
		"yggdrasil": {
			"meta": {"serverName": "NestedAuth", "implementationName": "Impl"},
			"signaturePublickey": "nested-public-key",
			"skinDomains": ["skin.example.com", ".example.com"]
		}
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, nested)
	}))
	defer srv.Close()
	pointHRPAuthAt(t, srv.URL)

	result := NewAuthService().GetHRPAuthMeta()
	if !result.Success {
		t.Fatalf("GetHRPAuthMeta failed: %s", result.Error)
	}
	data := result.Data.(map[string]interface{})
	meta := data["meta"].(*AuthlibServerMeta)
	if meta.Meta.ServerName != "NestedAuth" {
		t.Errorf("ServerName = %q, want NestedAuth", meta.Meta.ServerName)
	}
	if meta.SignaturePublickey != "nested-public-key" {
		t.Errorf("SignaturePublickey = %q, want nested-public-key", meta.SignaturePublickey)
	}
	if len(meta.SkinDomains) != 2 || meta.SkinDomains[0] != "skin.example.com" {
		t.Errorf("SkinDomains = %v, want [skin.example.com .example.com]", meta.SkinDomains)
	}
	// The raw document handed to authlib-injector must be the flat metadata,
	// not the nested envelope.
	raw, _ := data["raw"].(string)
	if strings.Contains(raw, `"yggdrasil"`) {
		t.Errorf("raw still nested under yggdrasil: %q", raw)
	}
	if !strings.Contains(raw, "nested-public-key") {
		t.Errorf("raw does not carry the unwrapped metadata: %q", raw)
	}
}

func TestGetHRPAuthMetaFollowsALIRedirect(t *testing.T) {
	var srvURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Point the client at /api per the authlib-injector ALI header.
		w.Header().Set("X-Authlib-Injector-API-Location", srvURL+"/api")
		io.WriteString(w, "homepage, not meta")
	})
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, testMetaJSON)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	srvURL = srv.URL
	pointHRPAuthAt(t, srv.URL)

	auth := NewAuthService()
	result := auth.GetHRPAuthMeta()
	if !result.Success {
		t.Fatalf("GetHRPAuthMeta with ALI redirect failed: %s", result.Error)
	}
	data := result.Data.(map[string]interface{})
	if data["resolved_url"] != srv.URL+"/api" {
		t.Errorf("resolved_url = %v, want %s/api", data["resolved_url"], srv.URL)
	}
	meta := data["meta"].(*AuthlibServerMeta)
	if meta.Meta.ServerName != "TestAuth" {
		t.Errorf("ServerName after redirect = %q, want TestAuth", meta.Meta.ServerName)
	}
}

func TestGetHRPAuthMetaHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	pointHRPAuthAt(t, srv.URL)

	result := NewAuthService().GetHRPAuthMeta()
	if result.Success {
		t.Fatal("expected failure for HTTP 500")
	}
	if !strings.Contains(result.Error, "500") {
		t.Errorf("error = %q, want mention of 500", result.Error)
	}
}

func TestAuthenticateWithHRPAuth(t *testing.T) {
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/authserver/authenticate" {
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{
			"accessToken": "access-123",
			"clientToken": "client-456",
			"availableProfiles": [{"id": "p1", "name": "PlayerOne"}],
			"selectedProfile": {"id": "p1", "name": "PlayerOne"},
			"user": {"id": "u1"}
		}`)
	}))
	defer srv.Close()
	pointHRPAuthAt(t, srv.URL)

	auth := NewAuthService()
	result := auth.AuthenticateWithHRPAuth("alice", "secret", "client-456")
	if !result.Success {
		t.Fatalf("AuthenticateWithHRPAuth failed: %s", result.Error)
	}

	// The request must follow the Yggdrasil shape.
	if gotBody["username"] != "alice" || gotBody["password"] != "secret" {
		t.Errorf("credentials not transmitted: %v", gotBody)
	}
	if gotBody["clientToken"] != "client-456" {
		t.Errorf("clientToken = %v, want client-456", gotBody["clientToken"])
	}
	if gotBody["requestUser"] != true {
		t.Errorf("requestUser = %v, want true", gotBody["requestUser"])
	}
	agent, _ := gotBody["agent"].(map[string]interface{})
	if agent == nil || agent["name"] != "Minecraft" {
		t.Errorf("agent = %v, want Minecraft agent object", gotBody["agent"])
	}

	resp, ok := result.Data.(AuthlibAuthResponse)
	if !ok {
		t.Fatalf("Data is %T, want AuthlibAuthResponse", result.Data)
	}
	if resp.AccessToken != "access-123" || resp.SelectedProfile.Name != "PlayerOne" {
		t.Errorf("unexpected auth response: %+v", resp)
	}
}

func TestAuthenticateWithHRPAuthErrorMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		io.WriteString(w, `{"errorMessage": "Invalid credentials. Invalid username or password."}`)
	}))
	defer srv.Close()
	pointHRPAuthAt(t, srv.URL)

	result := NewAuthService().AuthenticateWithHRPAuth("alice", "wrong", "")
	if result.Success {
		t.Fatal("expected failure for 403")
	}
	if !strings.Contains(result.Error, "Invalid credentials") {
		t.Errorf("error = %q, want the server errorMessage", result.Error)
	}
}

func TestRefreshHRPAuthTokenWithProfile(t *testing.T) {
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/authserver/refresh" {
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		io.WriteString(w, `{"accessToken": "new-token", "clientToken": "client-456", "selectedProfile": {"id": "p1", "name": "PlayerOne"}}`)
	}))
	defer srv.Close()
	pointHRPAuthAt(t, srv.URL)

	auth := NewAuthService()
	result := auth.RefreshHRPAuthTokenWithProfile("old-token", "client-456", "p1")
	if !result.Success {
		t.Fatalf("refresh failed: %s", result.Error)
	}
	if gotBody["accessToken"] != "old-token" || gotBody["clientToken"] != "client-456" {
		t.Errorf("tokens not transmitted: %v", gotBody)
	}
	profile, _ := gotBody["selectedProfile"].(map[string]interface{})
	if profile == nil || profile["id"] != "p1" {
		t.Errorf("selectedProfile = %v, want id p1", gotBody["selectedProfile"])
	}
	resp := result.Data.(AuthlibAuthResponse)
	if resp.AccessToken != "new-token" {
		t.Errorf("AccessToken = %q, want new-token", resp.AccessToken)
	}
}

func TestValidateHRPAuthToken(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		body      string
		wantOK    bool
		wantError string
	}{
		{"204 valid", http.StatusNoContent, "", true, ""},
		{"403 invalid", http.StatusForbidden, `{"errorMessage": "ForbiddenOperationException"}`, false, "Invalid token"},
		{"500 with message", http.StatusInternalServerError, `{"errorMessage": "server broke"}`, false, "server broke"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.body)
			}))
			defer srv.Close()
			pointHRPAuthAt(t, srv.URL)

			result := NewAuthService().ValidateHRPAuthToken("token", "client")
			if result.Success != tc.wantOK {
				t.Fatalf("Success = %v, want %v (error: %s)", result.Success, tc.wantOK, result.Error)
			}
			if tc.wantError != "" && result.Error != tc.wantError {
				t.Errorf("Error = %q, want %q", result.Error, tc.wantError)
			}
		})
	}
}

// ==================== resolveALI / metadata edge cases ====================

func TestResolveALI(t *testing.T) {
	cases := []struct {
		name       string
		requestURL string
		ali        string
		want       string
		wantErr    bool
	}{
		{"empty header keeps request url", "https://auth.example.com/api", "  ", "https://auth.example.com/api", false},
		{"absolute location", "https://auth.example.com/api", "https://other.example.com/api", "https://other.example.com/api", false},
		{"relative location", "https://auth.example.com/api", "/v2/api", "https://auth.example.com/v2/api", false},
		{"invalid request url", "://bad", "/api", "", true},
		{"invalid location", "https://auth.example.com/api", "/a\x7f", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveALI(tc.requestURL, tc.ali)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("resolveALI(%q, %q) = %q, want error", tc.requestURL, tc.ali, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveALI(%q, %q) error: %v", tc.requestURL, tc.ali, err)
			}
			if got != tc.want {
				t.Errorf("resolveALI(%q, %q) = %q, want %q", tc.requestURL, tc.ali, got, tc.want)
			}
		})
	}
}

func TestGetHRPAuthMetaInvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "this is not json")
	}))
	defer srv.Close()
	pointHRPAuthAt(t, srv.URL)

	result := NewAuthService().GetHRPAuthMeta()
	if result.Success {
		t.Fatal("expected failure for a non-JSON metadata body")
	}
	if result.Error == "" {
		t.Error("error must not be empty")
	}
}

// A refresh without profile selection must send no selectedProfile.
func TestRefreshHRPAuthTokenWithoutProfileOmitsSelectedProfile(t *testing.T) {
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		io.WriteString(w, `{"accessToken": "new-token", "clientToken": "client-456"}`)
	}))
	defer srv.Close()
	pointHRPAuthAt(t, srv.URL)

	result := NewAuthService().RefreshHRPAuthTokenWithProfile("old-token", "client-456", "")
	if !result.Success {
		t.Fatalf("refresh failed: %s", result.Error)
	}
	if _, present := gotBody["selectedProfile"]; present {
		t.Errorf("selectedProfile must be absent without profile selection: %v", gotBody)
	}
	if resp := result.Data.(AuthlibAuthResponse); resp.AccessToken != "new-token" {
		t.Errorf("AccessToken = %q, want new-token", resp.AccessToken)
	}
}

func TestRefreshHRPAuthTokenErrors(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		body      string
		wantError string // substring
	}{
		{"403 with errorMessage", http.StatusForbidden, `{"errorMessage": "ForbiddenOperationException"}`, "ForbiddenOperationException"},
		{"500 without body", http.StatusInternalServerError, "", "HTTP 500"},
		{"200 with invalid json", http.StatusOK, "not-json", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.body)
			}))
			defer srv.Close()
			pointHRPAuthAt(t, srv.URL)

			result := NewAuthService().RefreshHRPAuthTokenWithProfile("token", "client", "")
			if result.Success {
				t.Fatal("expected failure")
			}
			if tc.wantError != "" && !strings.Contains(result.Error, tc.wantError) {
				t.Errorf("Error = %q, want substring %q", result.Error, tc.wantError)
			}
		})
	}
}

// ==================== BMCLAPIService ====================

// recordingMux wraps an http.ServeMux and records every request path in
// order so orchestration tests can assert the stage sequence.
type recordingMux struct {
	mu    sync.Mutex
	paths []string
	mux   *http.ServeMux
}

func newRecordingMux() *recordingMux {
	return &recordingMux{mux: http.NewServeMux()}
}

func (r *recordingMux) Handle(pattern string, handler http.HandlerFunc) {
	r.mux.HandleFunc(pattern, func(w http.ResponseWriter, req *http.Request) {
		r.mu.Lock()
		r.paths = append(r.paths, req.URL.Path)
		r.mu.Unlock()
		handler(w, req)
	})
}

func (r *recordingMux) recorded() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string{}, r.paths...)
}

func (r *recordingMux) server() *httptest.Server {
	return httptest.NewServer(r.mux)
}

const testVersionID = "1.99"

// testAssetHash is the 32-hex-char object hash served by installFixtures;
// assets are stored under objects/<first two chars>/<hash>.
const testAssetHash = "abababababababababababababababab"

// installFixtures serves a complete fake BMCLAPI: version manifest, version
// JSON, client jar, one plain library and one natives library (artifact +
// classifier zip). Returns the server and the recorded request paths.
func installFixtures(t *testing.T, jarStatus int) (*httptest.Server, *recordingMux) {
	t.Helper()

	versionJSON := fmt.Sprintf(`{
		"id": %q,
		"mainClass": "net.minecraft.client.main.Main",
		"assetIndex": {"id": "5", "url": "https://piston-meta.mojang.com/v1/assets/5.json", "sha1": "x", "size": 1},
		"assets": "5",
		"downloads": {
			"client": {"sha1": "c1", "size": 9, "url": "https://piston-data.mojang.com/v1/objects/c1/client.jar"}
		},
		"libraries": [
			{
				"name": "com.example:lib:1.0",
				"downloads": {"artifact": {"path": "com/example/lib/1.0/lib-1.0.jar", "url": "https://libraries.minecraft.net/com/example/lib/1.0/lib-1.0.jar", "sha1": "l1", "size": 3}}
			},
			{
				"name": "com.example:natives:1.0",
				"downloads": {
					"artifact": {"path": "com/example/natives/1.0/natives-1.0.jar", "url": "https://libraries.minecraft.net/com/example/natives/1.0/natives-1.0.jar", "sha1": "n1", "size": 3},
					"classifiers": {
						"natives-linux": {"path": "com/example/natives/1.0/natives-1.0-linux.jar", "url": "https://libraries.minecraft.net/com/example/natives/1.0/natives-1.0-linux.jar", "sha1": "n2", "size": 100}
					}
				}
			}
		],
		"type": "release"
	}`, testVersionID)

	manifest := fmt.Sprintf(`{
		"latest": {"release": %q},
		"versions": [{"id": %q, "type": "release", "url": "https://piston-meta.mojang.com/v1/packages/abc/%s.json"}]
	}`, testVersionID, testVersionID, testVersionID)

	rec := newRecordingMux()
	rec.Handle("/mc/game/version_manifest.json", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, manifest)
	})
	rec.Handle("/v1/packages/abc/"+testVersionID+".json", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, versionJSON)
	})
	rec.Handle("/v1/objects/c1/client.jar", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(jarStatus)
		if jarStatus == http.StatusOK {
			io.WriteString(w, "jar-bytes")
		}
	})
	rec.Handle("/maven/com/example/lib/1.0/lib-1.0.jar", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "lib")
	})
	rec.Handle("/maven/com/example/natives/1.0/natives-1.0.jar", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "nat")
	})
	rec.Handle("/maven/com/example/natives/1.0/natives-1.0-linux.jar", func(w http.ResponseWriter, r *http.Request) {
		w.Write(nativeZipFixture(t))
	})
	rec.Handle("/v1/assets/5.json", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"objects": {"minecraft/sounds/one.ogg": {"hash": %q, "size": 5}}}`, testAssetHash)
	})
	rec.Handle("/assets/ab/"+testAssetHash, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "asset")
	})

	srv := rec.server()
	t.Cleanup(srv.Close)
	return srv, rec
}

// nativeZipFixture builds a real zip in memory containing one .so (which must
// be extracted), one META-INF entry and one .txt file (both must be skipped).
func nativeZipFixture(t *testing.T) []byte {
	t.Helper()
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	entries := []struct{ name, body string }{
		{"libfoo.so", "native-bytes"},
		{"META-INF/MANIFEST.MF", "Manifest-Version: 1.0"},
		{"readme.txt", "not a native"},
	}
	for _, e := range entries {
		w, err := zw.Create(e.name)
		if err != nil {
			t.Fatalf("zip create %q: %v", e.name, err)
		}
		if _, err := w.Write([]byte(e.body)); err != nil {
			t.Fatalf("zip write %q: %v", e.name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

func TestGetVersionList(t *testing.T) {
	srv, _ := installFixtures(t, http.StatusOK)
	svc := NewBMCLAPIServiceWithBaseURL(srv.URL)

	result := svc.GetVersionList()
	if !result.Success {
		t.Fatalf("GetVersionList failed: %s", result.Error)
	}
	manifest, ok := result.Data.(BMCLVersionManifestResponse)
	if !ok {
		t.Fatalf("Data is %T, want BMCLVersionManifestResponse", result.Data)
	}
	if len(manifest.Versions) != 1 || manifest.Versions[0].ID != testVersionID {
		t.Errorf("unexpected manifest: %+v", manifest)
	}
	if manifest.Latest["release"] != testVersionID {
		t.Errorf("latest.release = %q, want %s", manifest.Latest["release"], testVersionID)
	}
}

func TestGetVersionListHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()

	result := NewBMCLAPIServiceWithBaseURL(srv.URL).GetVersionList()
	if result.Success {
		t.Fatal("expected failure for HTTP 404")
	}
	if result.Error != "HTTP 404" {
		t.Errorf("Error = %q, want HTTP 404", result.Error)
	}
}

func TestGetVersionManifestRewritesMojangURL(t *testing.T) {
	srv, rec := installFixtures(t, http.StatusOK)
	svc := NewBMCLAPIServiceWithBaseURL(srv.URL)

	// The caller passes the official Mojang URL; the service must rewrite it
	// onto the mirror (here: the test server).
	result := svc.GetVersionManifest("https://piston-meta.mojang.com/v1/packages/abc/" + testVersionID + ".json")
	if !result.Success {
		t.Fatalf("GetVersionManifest failed: %s", result.Error)
	}
	vj, ok := result.Data.(BMCLVersionJSON)
	if !ok {
		t.Fatalf("Data is %T, want BMCLVersionJSON", result.Data)
	}
	if vj.MainClass != "net.minecraft.client.main.Main" {
		t.Errorf("MainClass = %q", vj.MainClass)
	}
	paths := rec.recorded()
	if len(paths) != 1 || paths[0] != "/v1/packages/abc/"+testVersionID+".json" {
		t.Errorf("recorded paths = %v, want the rewritten manifest path", paths)
	}
}

func TestGetFabricAndForgeVersions(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/fabric-meta/v2/versions/loader/1.99", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `[{"loader": {"version": "0.15.0"}}]`)
	})
	mux.HandleFunc("/forge/minecraft/1.99", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `[{"version": "47.2.0"}]`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	svc := NewBMCLAPIServiceWithBaseURL(srv.URL)

	fabric := svc.GetFabricVersions("1.99")
	if !fabric.Success {
		t.Fatalf("GetFabricVersions failed: %s", fabric.Error)
	}
	fabricList, ok := fabric.Data.([]map[string]interface{})
	if !ok || len(fabricList) != 1 {
		t.Fatalf("fabric Data = %T %v, want one entry", fabric.Data, fabric.Data)
	}

	forge := svc.GetForgeVersions("1.99")
	if !forge.Success {
		t.Fatalf("GetForgeVersions failed: %s", forge.Error)
	}
	forgeList, ok := forge.Data.([]map[string]interface{})
	if !ok || len(forgeList) != 1 {
		t.Fatalf("forge Data = %T %v, want one entry", forge.Data, forge.Data)
	}
}

func TestGetVersionManifestErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bad-json.json":
			io.WriteString(w, "not-json")
		default:
			http.Error(w, "down", http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	svc := NewBMCLAPIServiceWithBaseURL(srv.URL)

	result := svc.GetVersionManifest(srv.URL + "/missing.json")
	if result.Success || !strings.Contains(result.Error, "500") {
		t.Errorf("HTTP error case: Success=%v Error=%q, want failure mentioning 500", result.Success, result.Error)
	}

	result = svc.GetVersionManifest(srv.URL + "/bad-json.json")
	if result.Success || result.Error == "" {
		t.Errorf("invalid JSON case: Success=%v Error=%q, want decode failure", result.Success, result.Error)
	}
}

func TestGetFabricAndForgeVersionsErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/bad") {
			io.WriteString(w, "not-json")
			return
		}
		http.Error(w, "down", http.StatusInternalServerError)
	}))
	defer srv.Close()

	svc := NewBMCLAPIServiceWithBaseURL(srv.URL)

	if result := svc.GetFabricVersions("1.99"); result.Success || !strings.Contains(result.Error, "500") {
		t.Errorf("fabric HTTP error: Success=%v Error=%q", result.Success, result.Error)
	}
	if result := svc.GetFabricVersions("bad"); result.Success {
		t.Error("fabric invalid JSON must fail")
	}
	if result := svc.GetForgeVersions("1.99"); result.Success || !strings.Contains(result.Error, "500") {
		t.Errorf("forge HTTP error: Success=%v Error=%q", result.Success, result.Error)
	}
	if result := svc.GetForgeVersions("bad"); result.Success {
		t.Error("forge invalid JSON must fail")
	}
}

func TestGetFabricInstallerHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()

	result := NewBMCLAPIServiceWithBaseURL(srv.URL).GetFabricInstaller("1.99", "0.15.0", t.TempDir())
	if result.Success || !strings.Contains(result.Error, "404") {
		t.Errorf("Success=%v Error=%q, want failure mentioning 404", result.Success, result.Error)
	}
}

func TestDownloadForgeInstallerHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()

	result := NewBMCLAPIServiceWithBaseURL(srv.URL).DownloadForgeInstaller("1.99", "47.2.0", t.TempDir())
	if result.Success || !strings.Contains(result.Error, "404") {
		t.Errorf("Success=%v Error=%q, want failure mentioning 404", result.Success, result.Error)
	}
}

func TestDownloadFileWritesContentAndReportsProgress(t *testing.T) {
	body := "download-me"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, body)
	}))
	defer srv.Close()

	svc := NewBMCLAPIServiceWithBaseURL(srv.URL)
	dest := filepath.Join(t.TempDir(), "sub", "out.bin")

	var calls []int64
	result := svc.DownloadFile(srv.URL+"/file", dest, int64(len(body)), func(downloaded, total int64) {
		calls = append(calls, downloaded)
		if total != int64(len(body)) {
			t.Errorf("progress total = %d, want %d", total, len(body))
		}
	})
	if !result.Success {
		t.Fatalf("DownloadFile failed: %s", result.Error)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if string(got) != body {
		t.Errorf("file content = %q, want %q", got, body)
	}
	if len(calls) == 0 || calls[len(calls)-1] != int64(len(body)) {
		t.Errorf("progress calls = %v, want final downloaded == %d", calls, len(body))
	}
}

// A truncated response (fewer bytes than Content-Length) must fail and remove
// the partial file: DownloadLibraries/DownloadAssets skip existing files, so
// a leftover truncated file would block every retry.
func TestDownloadFileFailureRemovesPartial(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		io.WriteString(w, "partial")
		// Handler returns: connection closes with bytes still promised, so the
		// client read fails with an unexpected EOF.
	}))
	defer srv.Close()

	svc := NewBMCLAPIServiceWithBaseURL(srv.URL)
	destPath := filepath.Join(t.TempDir(), "sub", "partial.jar")

	result := svc.DownloadFile(srv.URL+"/x.jar", destPath, 100, nil)
	if result.Success {
		t.Fatal("expected failure for a truncated response")
	}
	if _, err := os.Stat(destPath); !os.IsNotExist(err) {
		t.Errorf("partial file must be removed after a failed download, stat err = %v", err)
	}
}

func TestDownloadFileHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "gone", http.StatusNotFound)
	}))
	defer srv.Close()

	result := NewBMCLAPIServiceWithBaseURL(srv.URL).DownloadFile(srv.URL+"/missing", filepath.Join(t.TempDir(), "x"), 0, nil)
	if result.Success {
		t.Fatal("expected failure for HTTP 404")
	}
}

func TestDownloadLibrariesRewritesAndSkipsExisting(t *testing.T) {
	var mu sync.Mutex
	libRequests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/maven/com/example/lib/1.0/lib-1.0.jar" {
			mu.Lock()
			libRequests++
			mu.Unlock()
			io.WriteString(w, "lib")
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc := NewBMCLAPIServiceWithBaseURL(srv.URL)
	libsDir := t.TempDir()
	libs := []BMCLLibrary{{
		Name: "com.example:lib:1.0",
		Downloads: BMCLLibraryDownloads{
			Artifact: BMCLArtifact{
				Path: "com/example/lib/1.0/lib-1.0.jar",
				URL:  "https://libraries.minecraft.net/com/example/lib/1.0/lib-1.0.jar",
				Size: 3,
			},
		},
	}}

	var progressNames []string
	result := svc.DownloadLibraries(libs, libsDir, func(name string, downloaded, total int64) {
		progressNames = append(progressNames, name)
	})
	if !result.Success {
		t.Fatalf("DownloadLibraries failed: %s", result.Error)
	}

	wantPath := filepath.Join(libsDir, "com", "example", "lib", "1.0", "lib-1.0.jar")
	if _, err := os.Stat(wantPath); err != nil {
		t.Fatalf("library not written to maven layout %q: %v", wantPath, err)
	}
	if len(progressNames) != 1 || progressNames[0] != "com.example:lib:1.0" {
		t.Errorf("progress = %v, want one call for the library name", progressNames)
	}

	// Second run: the file exists, so no request must be made.
	result = svc.DownloadLibraries(libs, libsDir, nil)
	if !result.Success {
		t.Fatalf("second DownloadLibraries failed: %s", result.Error)
	}
	mu.Lock()
	if libRequests != 1 {
		t.Errorf("library requested %d times, want 1 (existing file must be skipped)", libRequests)
	}
	mu.Unlock()
}

func TestDownloadLibrariesReportsFailureButContinues(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/maven/good.jar" {
			io.WriteString(w, "ok")
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc := NewBMCLAPIServiceWithBaseURL(srv.URL)
	libsDir := t.TempDir()
	libs := []BMCLLibrary{
		{Name: "com.example:bad:1.0", Downloads: BMCLLibraryDownloads{Artifact: BMCLArtifact{Path: "bad.jar", URL: "https://libraries.minecraft.net/bad.jar", Size: 1}}},
		{Name: "com.example:good:1.0", Downloads: BMCLLibraryDownloads{Artifact: BMCLArtifact{Path: "good.jar", URL: "https://libraries.minecraft.net/good.jar", Size: 2}}},
	}

	result := svc.DownloadLibraries(libs, libsDir, nil)
	if result.Success {
		t.Fatal("expected failure when one library 404s")
	}
	if !strings.Contains(result.Error, "com.example:bad:1.0") {
		t.Errorf("error = %q, want the failed library name", result.Error)
	}
	// The good library must still have been downloaded (continue semantics).
	// DownloadLibraries derives the path from the maven name.
	goodPath := filepath.Join(libsDir, "com", "example", "good", "1.0", "good-1.0.jar")
	if _, err := os.Stat(goodPath); err != nil {
		t.Errorf("good library missing after partial failure: %v", err)
	}
}

func TestDownloadNativesExtractsOnlyNativeFiles(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skipf("fixture provides natives-linux only; GOOS=%s", runtime.GOOS)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/maven/natives-linux.jar" {
			w.Write(nativeZipFixture(t))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc := NewBMCLAPIServiceWithBaseURL(srv.URL)
	nativesDir := filepath.Join(t.TempDir(), "natives")
	libs := []BMCLLibrary{{
		Name: "com.example:natives:1.0",
		Downloads: BMCLLibraryDownloads{
			Classifiers: map[string]BMCLArtifact{
				"natives-linux": {Path: "natives-linux.jar", URL: "https://libraries.minecraft.net/natives-linux.jar", Size: 100},
			},
		},
	}}

	var progressNames []string
	result := svc.DownloadNatives(libs, nativesDir, func(name string, downloaded, total int64) {
		progressNames = append(progressNames, name)
	})
	if !result.Success {
		t.Fatalf("DownloadNatives failed: %s", result.Error)
	}

	if got, err := os.ReadFile(filepath.Join(nativesDir, "libfoo.so")); err != nil || string(got) != "native-bytes" {
		t.Errorf("libfoo.so missing or wrong (err=%v, got=%q)", err, got)
	}
	if _, err := os.Stat(filepath.Join(nativesDir, "readme.txt")); !os.IsNotExist(err) {
		t.Error("non-native readme.txt must not be extracted")
	}
	if _, err := os.Stat(filepath.Join(nativesDir, "MANIFEST.MF")); !os.IsNotExist(err) {
		t.Error("META-INF entries must not be extracted")
	}
	if len(progressNames) != 1 {
		t.Errorf("progress calls = %v, want exactly one", progressNames)
	}
}

func TestShouldDownloadLibraryRules(t *testing.T) {
	current := runtime.GOOS
	other := "windows"
	if current == "windows" {
		other = "linux"
	}

	rule := func(action, osName string) BMCLRule {
		r := BMCLRule{Action: action}
		if osName != "" {
			r.OS = &BMCLOS{Name: osName}
		}
		return r
	}

	cases := []struct {
		name  string
		rules []BMCLRule
		want  bool
	}{
		{"no rules downloads", nil, true},
		{"allow current os", []BMCLRule{rule("allow", current)}, true},
		{"allow other os only", []BMCLRule{rule("allow", other)}, false},
		{"allow without os matches all", []BMCLRule{rule("allow", "")}, true},
		{"disallow current os wins", []BMCLRule{rule("allow", ""), rule("disallow", current)}, false},
		{"disallow other os keeps allow", []BMCLRule{rule("allow", ""), rule("disallow", other)}, true},
		{"disallow without os is ignored", []BMCLRule{rule("allow", ""), rule("disallow", "")}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldDownloadLibrary(BMCLLibrary{Name: "g:a:1", Rules: tc.rules}); got != tc.want {
				t.Errorf("shouldDownloadLibrary(rules=%+v) = %v, want %v", tc.rules, got, tc.want)
			}
		})
	}
}

// DownloadNatives has continue-semantics: libraries without a matching
// classifier, failed downloads or corrupt zips are skipped without failing
// the whole stage.
func TestDownloadNativesSkipsUnusableEntries(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skipf("classifier selection is OS specific; GOOS=%s", runtime.GOOS)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/maven/corrupt.jar":
			io.WriteString(w, "this is not a zip")
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	libs := []BMCLLibrary{
		{Name: "no-classifiers"},
		{Name: "wrong-classifier", Downloads: BMCLLibraryDownloads{
			Classifiers: map[string]BMCLArtifact{"natives-windows": {URL: srv.URL + "/maven/win.jar", Size: 1}},
		}},
		{Name: "corrupt-zip", Downloads: BMCLLibraryDownloads{
			Classifiers: map[string]BMCLArtifact{"natives-linux": {URL: srv.URL + "/maven/corrupt.jar", Size: 17}},
		}},
		{Name: "missing-zip", Downloads: BMCLLibraryDownloads{
			Classifiers: map[string]BMCLArtifact{"natives-linux": {URL: srv.URL + "/maven/missing.jar", Size: 1}},
		}},
	}

	nativesDir := filepath.Join(t.TempDir(), "natives")
	var progressCalls int
	result := NewBMCLAPIServiceWithBaseURL(srv.URL).DownloadNatives(libs, nativesDir, func(name string, downloaded, total int64) {
		progressCalls++
	})
	if !result.Success {
		t.Fatalf("DownloadNatives must succeed despite unusable entries: %s", result.Error)
	}
	if progressCalls != 0 {
		t.Errorf("progress calls = %d, want none for skipped entries", progressCalls)
	}
	if entries, _ := os.ReadDir(nativesDir); len(entries) != 0 {
		t.Errorf("natives dir must stay empty, found %v", entries)
	}
}

func TestInstallVersionHappyPath(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skipf("fixture provides natives-linux only; GOOS=%s", runtime.GOOS)
	}

	srv, rec := installFixtures(t, http.StatusOK)
	svc := NewBMCLAPIServiceWithBaseURL(srv.URL)
	gameDir := t.TempDir()

	type progressEvent struct {
		stage, name string
	}
	var events []progressEvent
	result := svc.InstallVersion(testVersionID, gameDir, func(stage, name string, downloaded, total int64) {
		events = append(events, progressEvent{stage, name})
	})
	if !result.Success {
		t.Fatalf("InstallVersion failed: %s", result.Error)
	}

	// Stage order: manifest -> version json -> jar -> libraries -> natives ->
	// asset index -> asset objects.
	wantPaths := []string{
		"/mc/game/version_manifest.json",
		"/v1/packages/abc/" + testVersionID + ".json",
		"/v1/objects/c1/client.jar",
		"/maven/com/example/lib/1.0/lib-1.0.jar",
		"/maven/com/example/natives/1.0/natives-1.0.jar",
		"/maven/com/example/natives/1.0/natives-1.0-linux.jar",
		"/v1/assets/5.json",
		"/assets/ab/" + testAssetHash,
	}
	gotPaths := rec.recorded()
	if len(gotPaths) != len(wantPaths) {
		t.Fatalf("request sequence = %v, want %v", gotPaths, wantPaths)
	}
	for i := range wantPaths {
		if gotPaths[i] != wantPaths[i] {
			t.Errorf("request %d = %q, want %q", i, gotPaths[i], wantPaths[i])
		}
	}

	// Files on disk.
	versionDir := filepath.Join(gameDir, "versions", testVersionID)
	jsonData, err := os.ReadFile(filepath.Join(versionDir, testVersionID+".json"))
	if err != nil {
		t.Fatalf("version JSON not written: %v", err)
	}
	var vj BMCLVersionJSON
	if err := json.Unmarshal(jsonData, &vj); err != nil || vj.MainClass != "net.minecraft.client.main.Main" {
		t.Errorf("persisted version JSON invalid: %v / %+v", err, vj)
	}
	if got, err := os.ReadFile(filepath.Join(versionDir, testVersionID+".jar")); err != nil || string(got) != "jar-bytes" {
		t.Errorf("version jar missing or wrong (err=%v, got=%q)", err, got)
	}
	if _, err := os.Stat(filepath.Join(gameDir, "libraries", "com", "example", "lib", "1.0", "lib-1.0.jar")); err != nil {
		t.Errorf("library missing: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(versionDir, "natives", "libfoo.so")); err != nil || string(got) != "native-bytes" {
		t.Errorf("native missing or wrong (err=%v, got=%q)", err, got)
	}
	if got, err := os.ReadFile(filepath.Join(gameDir, "assets", "objects", "ab", testAssetHash)); err != nil || string(got) != "asset" {
		t.Errorf("asset object missing or wrong (err=%v, got=%q)", err, got)
	}

	// Result payload paths.
	data, ok := result.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("Data is %T, want map", result.Data)
	}
	if data["version_json"] != filepath.Join(versionDir, testVersionID+".json") {
		t.Errorf("version_json = %v", data["version_json"])
	}
	if data["assets_dir"] != filepath.Join(gameDir, "assets") {
		t.Errorf("assets_dir = %v", data["assets_dir"])
	}

	// Progress stage order: all jar events before library events, library
	// before native, native before asset.
	stageOrder := map[string]int{"jar": 0, "library": 1, "native": 2, "asset": 3}
	last := -1
	for _, e := range events {
		if stageOrder[e.stage] < last {
			t.Errorf("progress stage %q arrived after a later stage; events=%v", e.stage, events)
			break
		}
		last = stageOrder[e.stage]
	}
	for _, want := range []string{"jar", "library", "native", "asset"} {
		seen := false
		for _, e := range events {
			if e.stage == want {
				seen = true
			}
		}
		if !seen {
			t.Errorf("no progress events for stage %q; events=%v", want, events)
		}
	}
}

func TestInstallVersionUnknownVersion(t *testing.T) {
	srv, _ := installFixtures(t, http.StatusOK)
	svc := NewBMCLAPIServiceWithBaseURL(srv.URL)
	gameDir := t.TempDir()

	result := svc.InstallVersion("does-not-exist", gameDir, nil)
	if result.Success {
		t.Fatal("expected failure for unknown version")
	}
	if result.Error != "version not found" {
		t.Errorf("Error = %q, want 'version not found'", result.Error)
	}
	if _, err := os.Stat(filepath.Join(gameDir, "versions")); !os.IsNotExist(err) {
		t.Error("no version directory may be created for an unknown version")
	}
}

func TestInstallVersionListFailureWritesNothing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusInternalServerError)
	}))
	defer srv.Close()

	svc := NewBMCLAPIServiceWithBaseURL(srv.URL)
	gameDir := t.TempDir()

	result := svc.InstallVersion(testVersionID, gameDir, nil)
	if result.Success {
		t.Fatal("expected failure when the version list is unavailable")
	}
	entries, _ := os.ReadDir(gameDir)
	if len(entries) != 0 {
		t.Errorf("gameDir must stay untouched on list failure, found %d entries", len(entries))
	}
}

func TestInstallVersionJarFailureStopsBeforeLibraries(t *testing.T) {
	srv, rec := installFixtures(t, http.StatusInternalServerError)
	svc := NewBMCLAPIServiceWithBaseURL(srv.URL)
	gameDir := t.TempDir()

	result := svc.InstallVersion(testVersionID, gameDir, nil)
	if result.Success {
		t.Fatal("expected failure when the jar download fails")
	}

	// No residue: the stage-1 version JSON is written before the jar download,
	// but the failed install must clean the whole version directory up again.
	versionDir := filepath.Join(gameDir, "versions", testVersionID)
	if _, err := os.Stat(versionDir); !os.IsNotExist(err) {
		t.Errorf("version directory must be removed after a failed install, stat err = %v", err)
	}
	for _, p := range rec.recorded() {
		if strings.HasPrefix(p, "/maven/") {
			t.Errorf("library/native requested despite jar failure: %s", p)
		}
	}
}

func TestInstallVersionLibraryFailureSkipsNatives(t *testing.T) {
	// Custom server: manifest, version JSON and jar succeed, the library 404s.
	failSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/mc/game/version_manifest.json":
			fmt.Fprintf(w, `{"versions": [{"id": %q, "type": "release", "url": "https://piston-meta.mojang.com/v1/packages/abc/%s.json"}]}`, testVersionID, testVersionID)
		case "/v1/packages/abc/" + testVersionID + ".json":
			fmt.Fprintf(w, `{
				"id": %q,
				"mainClass": "net.minecraft.client.main.Main",
				"downloads": {"client": {"sha1": "c1", "size": 3, "url": "https://piston-data.mojang.com/v1/objects/c1/client.jar"}},
				"libraries": [
					{"name": "com.example:lib:1.0", "downloads": {"artifact": {"path": "com/example/lib/1.0/lib-1.0.jar", "url": "https://libraries.minecraft.net/com/example/lib/1.0/lib-1.0.jar", "size": 3}}}
				]
			}`, testVersionID)
		case "/v1/objects/c1/client.jar":
			io.WriteString(w, "jar")
		default:
			http.NotFound(w, r) // the library 404s
		}
	}))
	defer failSrv.Close()

	failSvc := NewBMCLAPIServiceWithBaseURL(failSrv.URL)
	gameDir := t.TempDir()

	result := failSvc.InstallVersion(testVersionID, gameDir, nil)
	if result.Success {
		t.Fatal("expected failure when a library download fails")
	}
	if !strings.Contains(result.Error, "com.example:lib:1.0") {
		t.Errorf("Error = %q, want the failed library name", result.Error)
	}
	// No residue: even though the jar (stage 2) completed, the failed install
	// must remove the version directory again; the natives stage never runs.
	versionDir := filepath.Join(gameDir, "versions", testVersionID)
	if _, err := os.Stat(versionDir); !os.IsNotExist(err) {
		t.Errorf("version directory must be removed after a failed install, stat err = %v", err)
	}
}

// A failure in the asset stage (stage 5) must also leave no residue.
func TestInstallVersionAssetFailureCleansUp(t *testing.T) {
	failSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/mc/game/version_manifest.json":
			fmt.Fprintf(w, `{"versions": [{"id": %q, "type": "release", "url": "https://piston-meta.mojang.com/v1/packages/abc/%s.json"}]}`, testVersionID, testVersionID)
		case "/v1/packages/abc/" + testVersionID + ".json":
			fmt.Fprintf(w, `{
				"id": %q,
				"mainClass": "net.minecraft.client.main.Main",
				"assetIndex": {"id": "5", "url": "https://piston-meta.mojang.com/v1/assets/5.json", "sha1": "x", "size": 1},
				"downloads": {"client": {"sha1": "c1", "size": 3, "url": "https://piston-data.mojang.com/v1/objects/c1/client.jar"}}
			}`, testVersionID)
		case "/v1/objects/c1/client.jar":
			io.WriteString(w, "jar")
		default:
			http.Error(w, "asset index down", http.StatusInternalServerError)
		}
	}))
	defer failSrv.Close()

	gameDir := t.TempDir()
	result := NewBMCLAPIServiceWithBaseURL(failSrv.URL).InstallVersion(testVersionID, gameDir, nil)
	if result.Success {
		t.Fatal("expected failure when the asset index download fails")
	}
	if !strings.Contains(result.Error, "500") {
		t.Errorf("Error = %q, want mention of 500", result.Error)
	}

	versionDir := filepath.Join(gameDir, "versions", testVersionID)
	if _, err := os.Stat(versionDir); !os.IsNotExist(err) {
		t.Errorf("version directory must be removed after a failed install, stat err = %v", err)
	}
	if entries, _ := os.ReadDir(filepath.Join(gameDir, "assets")); len(entries) != 0 {
		t.Errorf("assets dir must stay empty after a failed asset stage, found %v", entries)
	}
}

// A re-install over an existing, working version directory must not destroy
// it when the new attempt fails mid-download.
func TestInstallVersionFailureKeepsPreExistingInstall(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skipf("fixture provides natives-linux only; GOOS=%s", runtime.GOOS)
	}

	okSrv, _ := installFixtures(t, http.StatusOK)
	defer okSrv.Close()
	gameDir := t.TempDir()
	if result := NewBMCLAPIServiceWithBaseURL(okSrv.URL).InstallVersion(testVersionID, gameDir, nil); !result.Success {
		t.Fatalf("initial install failed: %s", result.Error)
	}
	versionDir := filepath.Join(gameDir, "versions", testVersionID)
	wantJar, err := os.ReadFile(filepath.Join(versionDir, testVersionID+".jar"))
	if err != nil {
		t.Fatalf("jar missing after initial install: %v", err)
	}

	// Second attempt: same version, but the jar download now fails.
	failSrv, _ := installFixtures(t, http.StatusInternalServerError)
	defer failSrv.Close()
	result := NewBMCLAPIServiceWithBaseURL(failSrv.URL).InstallVersion(testVersionID, gameDir, nil)
	if result.Success {
		t.Fatal("expected failure when the jar download fails")
	}

	// The pre-existing install must survive the failed re-install.
	if _, err := os.Stat(filepath.Join(versionDir, testVersionID+".json")); err != nil {
		t.Errorf("pre-existing version JSON destroyed by failed re-install: %v", err)
	}
	gotJar, err := os.ReadFile(filepath.Join(versionDir, testVersionID+".jar"))
	if err != nil {
		t.Errorf("pre-existing jar destroyed by failed re-install: %v", err)
	} else if string(gotJar) != string(wantJar) {
		t.Errorf("jar content changed after failed re-install: %q -> %q", wantJar, gotJar)
	}
}

// ==================== LauncherService: config persistence ====================

// clearLauncherSettings removes all launcher.* rows so config tests start
// from a known state (the DB is shared across the test binary).
func clearLauncherSettings(t *testing.T) {
	t.Helper()
	repo := repository.NewSettingRepository()
	for _, key := range []string{
		"launcher.max_memory", "launcher.min_memory", "launcher.java_args",
		"launcher.game_width", "launcher.game_height", "launcher.game_dir",
		"launcher.authlib_jar",
	} {
		_ = repo.Delete(key)
	}
}

// fakeJavaHome points JAVA_HOME at a temp dir containing a fake bin/java so
// GetJavaPath is deterministic.
func fakeJavaHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	bin := filepath.Join(home, "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	javaPath := filepath.Join(bin, "java")
	if err := os.WriteFile(javaPath, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatalf("write fake java: %v", err)
	}
	t.Setenv("JAVA_HOME", home)
	return javaPath
}

func TestGetLauncherConfigDefaultsOnEmptySettings(t *testing.T) {
	clearLauncherSettings(t)
	javaPath := fakeJavaHome(t)
	t.Setenv("HOME", t.TempDir())

	cfg := NewLauncherService().GetLauncherConfig()
	if cfg.MaxMemory != 4096 || cfg.MinMemory != 1024 {
		t.Errorf("memory defaults = %d/%d, want 4096/1024", cfg.MaxMemory, cfg.MinMemory)
	}
	if cfg.GameWidth != 854 || cfg.GameHeight != 480 {
		t.Errorf("window defaults = %dx%d, want 854x480", cfg.GameWidth, cfg.GameHeight)
	}
	if cfg.JavaPath != javaPath {
		t.Errorf("JavaPath = %q, want fake JAVA_HOME java %q", cfg.JavaPath, javaPath)
	}
	if cfg.GameDir == "" {
		t.Error("GameDir must fall back to a platform default, got empty")
	}
	if cfg.JavaArgs != "" || cfg.AuthlibJar != "" {
		t.Errorf("optional fields must default to empty, got %q / %q", cfg.JavaArgs, cfg.AuthlibJar)
	}
}

func TestSaveLauncherConfigRoundTrip(t *testing.T) {
	clearLauncherSettings(t)
	fakeJavaHome(t)
	gameDir := t.TempDir()

	svc := NewLauncherService()
	result := svc.SaveLauncherConfig(models.LauncherConfig{
		MaxMemory:  2048,
		MinMemory:  512,
		JavaArgs:   "-XX:+UseG1GC",
		GameWidth:  1024,
		GameHeight: 768,
		GameDir:    gameDir,
		AuthlibJar: "/opt/authlib-injector.jar",
	})
	if !result.Success {
		t.Fatalf("SaveLauncherConfig failed: %s", result.Error)
	}

	// The response carries the merged config.
	merged, ok := result.Data.(models.LauncherConfig)
	if !ok {
		t.Fatalf("Data is %T, want models.LauncherConfig", result.Data)
	}
	if merged.MaxMemory != 2048 || merged.MinMemory != 512 || merged.GameWidth != 1024 || merged.GameHeight != 768 {
		t.Errorf("merged config wrong: %+v", merged)
	}

	// A fresh read sees the persisted values.
	cfg := svc.GetLauncherConfig()
	if cfg.MaxMemory != 2048 || cfg.MinMemory != 512 {
		t.Errorf("persisted memory = %d/%d, want 2048/512", cfg.MaxMemory, cfg.MinMemory)
	}
	if cfg.JavaArgs != "-XX:+UseG1GC" {
		t.Errorf("persisted java args = %q", cfg.JavaArgs)
	}
	if cfg.GameDir != gameDir {
		t.Errorf("persisted game dir = %q, want %q", cfg.GameDir, gameDir)
	}
	if cfg.AuthlibJar != "/opt/authlib-injector.jar" {
		t.Errorf("persisted authlib jar = %q", cfg.AuthlibJar)
	}
}

func TestSaveLauncherConfigValidation(t *testing.T) {
	clearLauncherSettings(t)
	svc := NewLauncherService()

	cases := []struct {
		name      string
		cfg       models.LauncherConfig
		wantError string
	}{
		{"negative max memory", models.LauncherConfig{MaxMemory: -1}, "memory values must be non-negative"},
		{"negative min memory", models.LauncherConfig{MinMemory: -5}, "memory values must be non-negative"},
		{"min exceeds max", models.LauncherConfig{MinMemory: 2048, MaxMemory: 1024}, "min memory cannot exceed max memory"},
		{"negative width", models.LauncherConfig{GameWidth: -1}, "game window dimensions must be non-negative"},
		{"negative height", models.LauncherConfig{GameHeight: -1}, "game window dimensions must be non-negative"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := svc.SaveLauncherConfig(tc.cfg)
			if result.Success {
				t.Fatalf("expected validation failure for %+v", tc.cfg)
			}
			if result.Error != tc.wantError {
				t.Errorf("Error = %q, want %q", result.Error, tc.wantError)
			}
		})
	}
}

func TestSaveLauncherConfigSkipsEmptyOptionalFields(t *testing.T) {
	clearLauncherSettings(t)
	svc := NewLauncherService()

	// Empty GameDir/AuthlibJar must not be persisted (mirrors the old HTTP
	// PUT semantics where absent fields were left untouched).
	result := svc.SaveLauncherConfig(models.LauncherConfig{MaxMemory: 1024, MinMemory: 512})
	if !result.Success {
		t.Fatalf("SaveLauncherConfig failed: %s", result.Error)
	}

	repo := repository.NewSettingRepository()
	for _, key := range []string{"launcher.game_dir", "launcher.authlib_jar"} {
		if _, err := repo.Get(key); err == nil {
			t.Errorf("setting %q must not be persisted when empty", key)
		}
	}
}

func TestParseIntSetting(t *testing.T) {
	if got := parseIntSetting("4096", 1); got != 4096 {
		t.Errorf("parseIntSetting(4096) = %d", got)
	}
	if got := parseIntSetting("", 7); got != 7 {
		t.Errorf("parseIntSetting(empty) = %d, want default 7", got)
	}
	if got := parseIntSetting("not-a-number", 9); got != 9 {
		t.Errorf("parseIntSetting(garbage) = %d, want default 9", got)
	}
}

// ==================== LauncherService: command building ====================

// writeVersionJSON creates <gameDir>/versions/<id>/<id>.json with the given
// content for BuildLaunchCommand tests.
func writeVersionJSON(t *testing.T, gameDir, id, content string) {
	t.Helper()
	dir := filepath.Join(gameDir, "versions", id)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, id+".json"), []byte(content), 0644); err != nil {
		t.Fatalf("write version json: %v", err)
	}
}

const testVersionJSON = `{
	"id": "1.99",
	"mainClass": "net.minecraft.client.main.Main",
	"assetIndex": {"id": "5"},
	"libraries": [
		{"name": "com.example:lib:1.0", "downloads": {"artifact": {"path": "com/example/lib/1.0/lib-1.0.jar"}}},
		{"name": "com.example:nat:1.0", "downloads": {"artifact": {"path": "com/example/nat/1.0/nat-1.0.jar"}, "classifiers": {"natives-linux": {"path": "natives-linux"}}}},
		{"name": "com.example:win:1.0", "downloads": {"artifact": {"path": "com/example/win/1.0/win-1.0.jar"}}, "rules": [{"action": "allow", "os": {"name": "windows"}}]}
	]
}`

func TestBuildLaunchCommandOffline(t *testing.T) {
	gameDir := t.TempDir()
	writeVersionJSON(t, gameDir, "1.99", testVersionJSON)

	svc := NewLauncherService()
	args, err := svc.BuildLaunchCommand(
		models.LauncherConfig{
			JavaPath:   "/opt/java/bin/java",
			MaxMemory:  2048,
			MinMemory:  512,
			JavaArgs:   "-XX:+UseG1GC -Dfoo=bar",
			GameWidth:  1024,
			GameHeight: 768,
			GameDir:    gameDir,
		},
		&models.MinecraftVersion{VersionID: "1.99"},
		&models.MinecraftAccount{
			Username:    "Steve",
			UUID:        "11111111-2222-3333-4444-555555555555",
			AccessToken: "at",
			ClientToken: "ct",
			AuthType:    "hrpauth",
		},
	)
	if err != nil {
		t.Fatalf("BuildLaunchCommand: %v", err)
	}

	joined := strings.Join(args, "\x00")
	contains := func(want string) {
		t.Helper()
		if !strings.Contains(joined, want) {
			t.Errorf("args missing %q\nfull args: %v", want, args)
		}
	}

	if args[0] != "/opt/java/bin/java" {
		t.Errorf("args[0] = %q, want the java path", args[0])
	}
	contains("-Xmx2048M")
	contains("-Xms512M")
	contains("-XX:+UseG1GC")
	contains("-Dfoo=bar")
	contains("-Djava.library.path=" + filepath.Join(gameDir, "versions", "1.99", "natives"))

	// Classpath: version jar + plain library; natives classifier libs and
	// windows-only libs are excluded on linux.
	cpIdx := -1
	for i, a := range args {
		if a == "-cp" {
			cpIdx = i
		}
	}
	if cpIdx == -1 || cpIdx+1 >= len(args) {
		t.Fatalf("-cp not found in args: %v", args)
	}
	cp := args[cpIdx+1]
	if !strings.Contains(cp, filepath.Join(gameDir, "versions", "1.99", "1.99.jar")) {
		t.Errorf("classpath missing version jar: %q", cp)
	}
	if !strings.Contains(cp, filepath.Join(gameDir, "libraries", "com", "example", "lib", "1.0", "lib-1.0.jar")) {
		t.Errorf("classpath missing plain library: %q", cp)
	}
	if strings.Contains(cp, "nat-1.0.jar") {
		t.Errorf("classpath must exclude natives-classifier library: %q", cp)
	}
	if strings.Contains(cp, "win-1.0.jar") {
		t.Errorf("classpath must exclude windows-only library on linux: %q", cp)
	}
	if args[cpIdx+2] != "net.minecraft.client.main.Main" {
		t.Errorf("main class = %q, want net.minecraft.client.main.Main", args[cpIdx+2])
	}

	contains("--username")
	contains("Steve")
	contains("--uuid")
	contains("11111111222233334444555555555555") // dashes stripped
	contains("--accessToken")
	contains("--userType")
	contains("mojang") // hrpauth accounts launch as the Yggdrasil user type
	contains("--versionType")
	contains("release") // default for empty version type
	contains("--assetsDir")
	contains(filepath.Join(gameDir, "assets"))
	contains("--assetIndex")
	contains("--width")
	contains("1024")
	contains("--height")
	contains("768")
}

// A loader version (Fabric/Forge style) declares inheritsFrom: the launch
// command must use the child's mainClass and libraries, but the parent's
// client jar, natives dir and asset index.
func TestBuildLaunchCommandInheritsFromParent(t *testing.T) {
	gameDir := t.TempDir()
	writeVersionJSON(t, gameDir, "1.99", testVersionJSON)
	// Parent jar must exist for the inherited classpath entry.
	parentDir := filepath.Join(gameDir, "versions", "1.99")
	if err := os.WriteFile(filepath.Join(parentDir, "1.99.jar"), []byte("jar"), 0644); err != nil {
		t.Fatalf("write parent jar: %v", err)
	}

	childJSON := `{
		"id": "fabric-loader-0.15.0-1.99",
		"inheritsFrom": "1.99",
		"mainClass": "net.fabricmc.loader.impl.launch.knot.KnotClient",
		"libraries": [
			{"name": "net.fabricmc:fabric-loader:0.15.0", "downloads": {"artifact": {"path": "net/fabricmc/fabric-loader/0.15.0/fabric-loader-0.15.0.jar"}}}
		]
	}`
	writeVersionJSON(t, gameDir, "fabric-loader-0.15.0-1.99", childJSON)

	svc := NewLauncherService()
	args, err := svc.BuildLaunchCommand(
		models.LauncherConfig{JavaPath: "/opt/java/bin/java", GameDir: gameDir},
		&models.MinecraftVersion{VersionID: "fabric-loader-0.15.0-1.99"},
		&models.MinecraftAccount{Username: "Steve", UUID: "u", AccessToken: "at", ClientToken: "ct", AuthType: "hrpauth"},
	)
	if err != nil {
		t.Fatalf("BuildLaunchCommand: %v", err)
	}

	joined := strings.Join(args, "\x00")
	contains := func(want string) {
		t.Helper()
		if !strings.Contains(joined, want) {
			t.Errorf("args missing %q\nfull args: %v", want, args)
		}
	}

	// Child's mainClass wins.
	contains("net.fabricmc.loader.impl.launch.knot.KnotClient")
	// Parent's jar and natives are used.
	contains(filepath.Join(parentDir, "1.99.jar"))
	contains("-Djava.library.path=" + filepath.Join(parentDir, "natives"))
	// Parent's asset index is inherited.
	contains("--assetIndex")

	cpIdx := -1
	for i, a := range args {
		if a == "-cp" {
			cpIdx = i
		}
	}
	if cpIdx == -1 || cpIdx+1 >= len(args) {
		t.Fatalf("-cp not found in args: %v", args)
	}
	cp := args[cpIdx+1]
	// Child library leads, parent library follows.
	childLib := filepath.Join(gameDir, "libraries", "net", "fabricmc", "fabric-loader", "0.15.0", "fabric-loader-0.15.0.jar")
	parentLib := filepath.Join(gameDir, "libraries", "com", "example", "lib", "1.0", "lib-1.0.jar")
	childIdx, parentIdx := strings.Index(cp, childLib), strings.Index(cp, parentLib)
	if childIdx == -1 || parentIdx == -1 {
		t.Fatalf("classpath missing loader or parent library: %q", cp)
	}
	if childIdx > parentIdx {
		t.Errorf("loader library must lead the parent's on the classpath: %q", cp)
	}
}

// A loader version whose parent is missing must fail with a clear error.
func TestBuildLaunchCommandInheritsFromMissingParent(t *testing.T) {
	gameDir := t.TempDir()
	writeVersionJSON(t, gameDir, "fabric-loader-x", `{"id": "fabric-loader-x", "inheritsFrom": "no-such-version", "mainClass": "Main"}`)

	svc := NewLauncherService()
	_, err := svc.BuildLaunchCommand(
		models.LauncherConfig{JavaPath: "/opt/java/bin/java", GameDir: gameDir},
		&models.MinecraftVersion{VersionID: "fabric-loader-x"},
		&models.MinecraftAccount{Username: "Steve", UUID: "u", AccessToken: "at", ClientToken: "ct", AuthType: "hrpauth"},
	)
	if err == nil || !strings.Contains(err.Error(), "no-such-version") {
		t.Errorf("err = %v, want a missing-parent error naming no-such-version", err)
	}
}

// Loader versions whose profiles carry Mojang-style `arguments` must get
// them expanded on top of the standard args (verified against the live
// forge 1.20.4/fabric 0.20.1 metadata shapes): string entries, {rules,
// value} entries with OS filtering, and ${variable} substitution.
func TestBuildLaunchCommandLoaderArguments(t *testing.T) {
	gameDir := t.TempDir()
	writeVersionJSON(t, gameDir, "1.99", testVersionJSON)
	parentDir := filepath.Join(gameDir, "versions", "1.99")
	if err := os.WriteFile(filepath.Join(parentDir, "1.99.jar"), []byte("jar"), 0644); err != nil {
		t.Fatalf("write parent jar: %v", err)
	}

	childJSON := fmt.Sprintf(`{
		"id": "1.99-forge47.2.0",
		"inheritsFrom": "1.99",
		"mainClass": "net.minecraftforge.bootstrap.ForgeBootstrap",
		"arguments": {
			"game": [
				"--launchTarget", "forge_client",
				{"rules": [{"action": "allow", "os": {"name": "windows"}}], "value": "--win-only"},
				{"rules": [{"action": "allow"}, {"action": "disallow", "os": {"name": %q}}], "value": "--never"}
			],
			"jvm": [
				"-Djava.net.preferIPv6Addresses=system",
				"-Dlibrary_directory=${library_directory}"
			]
		}
	}`, runtime.GOOS)
	writeVersionJSON(t, gameDir, "1.99-forge47.2.0", childJSON)

	svc := NewLauncherService()
	args, err := svc.BuildLaunchCommand(
		models.LauncherConfig{JavaPath: "/opt/java/bin/java", GameDir: gameDir},
		&models.MinecraftVersion{VersionID: "1.99-forge47.2.0"},
		&models.MinecraftAccount{Username: "Steve", UUID: "u", AccessToken: "at", ClientToken: "ct", AuthType: "hrpauth"},
	)
	if err != nil {
		t.Fatalf("BuildLaunchCommand: %v", err)
	}
	joined := strings.Join(args, "\x00")

	contains := func(want string) {
		t.Helper()
		if !strings.Contains(joined, want) {
			t.Errorf("args missing %q\nfull args: %v", want, args)
		}
	}
	absent := func(unwanted string) {
		t.Helper()
		if strings.Contains(joined, unwanted) {
			t.Errorf("args must not contain %q\nfull args: %v", unwanted, args)
		}
	}

	// Forge-style game arguments are appended after the standard set.
	contains("--launchTarget")
	contains("forge_client")
	contains("--username")
	// JVM arguments are expanded with variable substitution.
	contains("-Djava.net.preferIPv6Addresses=system")
	contains("-Dlibrary_directory=" + filepath.Join(gameDir, "libraries"))
	// OS rules filter entries.
	if runtime.GOOS != "windows" {
		absent("--win-only")
	}
	absent("--never")
	// The child mainClass wins over the parent's.
	contains("net.minecraftforge.bootstrap.ForgeBootstrap")
}

// Fabric profiles add a single jvm flag (-DFabricMcEmu); it must reach the
// launch command.
func TestBuildLaunchCommandFabricArguments(t *testing.T) {
	gameDir := t.TempDir()
	writeVersionJSON(t, gameDir, "1.99", testVersionJSON)
	parentDir := filepath.Join(gameDir, "versions", "1.99")
	if err := os.WriteFile(filepath.Join(parentDir, "1.99.jar"), []byte("jar"), 0644); err != nil {
		t.Fatalf("write parent jar: %v", err)
	}

	childJSON := `{
		"id": "fabric-loader-0.20.1-1.99",
		"inheritsFrom": "1.99",
		"mainClass": "net.fabricmc.loader.impl.launch.knot.KnotClient",
		"arguments": {"game": [], "jvm": ["-DFabricMcEmu= net.minecraft.client.main.Main "]}
	}`
	writeVersionJSON(t, gameDir, "fabric-loader-0.20.1-1.99", childJSON)

	svc := NewLauncherService()
	args, err := svc.BuildLaunchCommand(
		models.LauncherConfig{JavaPath: "/opt/java/bin/java", GameDir: gameDir},
		&models.MinecraftVersion{VersionID: "fabric-loader-0.20.1-1.99"},
		&models.MinecraftAccount{Username: "Steve", UUID: "u", AccessToken: "at", ClientToken: "ct", AuthType: "hrpauth"},
	)
	if err != nil {
		t.Fatalf("BuildLaunchCommand: %v", err)
	}
	found := false
	for _, a := range args {
		if a == "-DFabricMcEmu= net.minecraft.client.main.Main " {
			found = true
		}
	}
	if !found {
		t.Errorf("args missing the FabricMcEmu flag: %v", args)
	}
}

func TestBuildLaunchCommandHRPAuthInjector(t *testing.T) {
	gameDir := t.TempDir()
	writeVersionJSON(t, gameDir, "1.99", testVersionJSON)
	pointHRPAuthAt(t, "https://auth.example.com/")

	metaJSON := `{"meta":{"serverName":"TestAuth"}}`
	svc := NewLauncherService()
	args, err := svc.BuildLaunchCommand(
		models.LauncherConfig{
			JavaPath:        "/opt/java/bin/java",
			GameDir:         gameDir,
			AuthlibJar:      "/opt/authlib-injector.jar",
			AuthlibMetaJSON: metaJSON,
		},
		&models.MinecraftVersion{VersionID: "1.99"},
		&models.MinecraftAccount{
			Username:    "Steve",
			UUID:        "abcd",
			AccessToken: "at",
			ClientToken: "ct",
			AuthType:    "hrpauth",
		},
	)
	if err != nil {
		t.Fatalf("BuildLaunchCommand: %v", err)
	}

	joined := strings.Join(args, "\x00")
	// The injected auth server is always the built-in HRPAuth endpoint.
	wantAgent := "-javaagent:/opt/authlib-injector.jar=https://auth.example.com"
	if !strings.Contains(joined, wantAgent) {
		t.Errorf("args missing %q\nfull args: %v", wantAgent, args)
	}
	wantPrefetched := "-Dauthlibinjector.yggdrasil.prefetched=" + base64.StdEncoding.EncodeToString([]byte(metaJSON))
	if !strings.Contains(joined, wantPrefetched) {
		t.Errorf("args missing prefetched meta %q", wantPrefetched)
	}
	if !strings.Contains(joined, "mojang") {
		t.Errorf("hrpauth userType must be mojang, args: %v", args)
	}
}

func TestBuildLaunchCommandForgeTweakClass(t *testing.T) {
	gameDir := t.TempDir()
	writeVersionJSON(t, gameDir, "1.12.2", strings.Replace(testVersionJSON, "net.minecraft.client.main.Main", "net.minecraft.launchwrapper.Launch", 1))

	svc := NewLauncherService()
	args, err := svc.BuildLaunchCommand(
		models.LauncherConfig{JavaPath: "java", GameDir: gameDir},
		&models.MinecraftVersion{VersionID: "1.12.2"},
		&models.MinecraftAccount{Username: "Steve", UUID: "u", AccessToken: "at", ClientToken: "ct", AuthType: "hrpauth"},
	)
	if err != nil {
		t.Fatalf("BuildLaunchCommand: %v", err)
	}

	found := false
	for i, a := range args {
		if a == "--tweakClass" && i+1 < len(args) && args[i+1] == "net.minecraftforge.fml.common.launcher.FMLTweaker" {
			found = true
		}
	}
	if !found {
		t.Errorf("LaunchWrapper main class must add --tweakClass FMLTweaker, args: %v", args)
	}
}

func TestBuildLaunchCommandErrors(t *testing.T) {
	svc := NewLauncherService()
	account := &models.MinecraftAccount{Username: "u", UUID: "u", AccessToken: "a", ClientToken: "c"}
	version := &models.MinecraftVersion{VersionID: "missing"}

	if _, err := svc.BuildLaunchCommand(models.LauncherConfig{}, nil, account); err == nil {
		t.Error("nil version must fail")
	}
	if _, err := svc.BuildLaunchCommand(models.LauncherConfig{}, version, nil); err == nil {
		t.Error("nil account must fail")
	}
	// No version JSON on disk for id "missing".
	if _, err := svc.BuildLaunchCommand(models.LauncherConfig{GameDir: t.TempDir()}, version, account); err == nil {
		t.Error("missing version JSON must fail")
	}
}

// ==================== small helpers ====================

func TestResolveUserType(t *testing.T) {
	cases := map[string]string{
		"microsoft":        "msa",
		"msa":              "msa",
		"authlib-injector": "mojang",
		"mojang":           "mojang",
		"":                 "mojang",
		// Offline login was removed (ADR-0008); a residual value must not
		// produce the legacy user type and falls back to the Yggdrasil value.
		"offline": "mojang",
	}
	for in, want := range cases {
		if got := resolveUserType(in); got != want {
			t.Errorf("resolveUserType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolveVersionType(t *testing.T) {
	if got := resolveVersionType(""); got != "release" {
		t.Errorf("resolveVersionType(empty) = %q, want release", got)
	}
	if got := resolveVersionType("snapshot"); got != "snapshot" {
		t.Errorf("resolveVersionType(snapshot) = %q", got)
	}
}

func TestStripDashes(t *testing.T) {
	if got := stripDashes("aa-bb-cc"); got != "aabbcc" {
		t.Errorf("stripDashes = %q, want aabbcc", got)
	}
}

func TestShellQuote(t *testing.T) {
	if got := shellQuote("plain"); got != "plain" {
		t.Errorf("shellQuote(plain) = %q", got)
	}
	if got := shellQuote(""); got != "''" {
		t.Errorf("shellQuote(empty) = %q, want ''", got)
	}
	if got := shellQuote("has space"); !strings.Contains(got, "'") {
		t.Errorf("shellQuote(has space) = %q, want quoted", got)
	}
	if got := shellQuote("it's"); got != `'it'\''s'` {
		t.Errorf("shellQuote(it's) = %q, want 'it'\\''s'", got)
	}
}

func TestGetJavaPathHonorsJavaHome(t *testing.T) {
	javaPath := fakeJavaHome(t)
	if got := NewLauncherService().GetJavaPath(); got != javaPath {
		t.Errorf("GetJavaPath = %q, want %q", got, javaPath)
	}
}

// swapCommonJavaPaths points the common-path scan at the supplied candidates
// for the duration of a test and restores the production list afterwards. It
// exists so the fallback branches of GetJavaPath are deterministic
// (PENDING-DECISIONS #9, option B).
func swapCommonJavaPaths(t *testing.T, paths []string) {
	t.Helper()
	prev := commonJavaPaths
	commonJavaPaths = paths
	t.Cleanup(func() { commonJavaPaths = prev })
}

func TestGetJavaPathScansCommonPaths(t *testing.T) {
	// No JAVA_HOME so the scan falls through to the common-path list.
	t.Setenv("JAVA_HOME", "")

	// A usable java placed second: the scan must skip the missing first entry
	// and return the first candidate that exists.
	dir := t.TempDir()
	realJava := filepath.Join(dir, "java")
	if err := os.WriteFile(realJava, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatalf("write fake java: %v", err)
	}
	swapCommonJavaPaths(t, []string{filepath.Join(dir, "does-not-exist"), realJava})

	if got := NewLauncherService().GetJavaPath(); got != realJava {
		t.Errorf("GetJavaPath = %q, want %q", got, realJava)
	}
}

func TestGetJavaPathFallsBackToBareJava(t *testing.T) {
	// No JAVA_HOME and no common candidate exists -> bare "java" (PATH lookup).
	t.Setenv("JAVA_HOME", "")
	dir := t.TempDir()
	swapCommonJavaPaths(t, []string{filepath.Join(dir, "no-1"), filepath.Join(dir, "no-2")})

	if got := NewLauncherService().GetJavaPath(); got != "java" {
		t.Errorf("GetJavaPath = %q, want %q", got, "java")
	}
}

func TestGetMinecraftDirUsesHome(t *testing.T) {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		t.Skipf("linux-specific expectation; GOOS=%s", runtime.GOOS)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	if got := NewLauncherService().GetMinecraftDir(); got != filepath.Join(home, ".minecraft") {
		t.Errorf("GetMinecraftDir = %q, want %q", got, filepath.Join(home, ".minecraft"))
	}
}

// Match handoff (INTEROP-SPEC §6): a configured join target becomes
// --server/--port game arguments so the client connects straight into the
// match server.
func TestBuildLaunchCommandJoinTarget(t *testing.T) {
	gameDir := t.TempDir()
	writeVersionJSON(t, gameDir, "1.99", testVersionJSON)

	svc := NewLauncherService()
	args, err := svc.BuildLaunchCommand(
		models.LauncherConfig{
			JavaPath:   "/opt/java/bin/java",
			MaxMemory:  2048,
			MinMemory:  512,
			GameDir:    gameDir,
			JoinServer: "bw.match.local",
			JoinPort:   25565,
		},
		&models.MinecraftVersion{VersionID: "1.99"},
		&models.MinecraftAccount{
			Username: "Steve", UUID: "11111111-2222-3333-4444-555555555555",
			AccessToken: "at", ClientToken: "ct", AuthType: "hrpauth",
		},
	)
	if err != nil {
		t.Fatalf("BuildLaunchCommand: %v", err)
	}

	var serverIdx, portIdx = -1, -1
	for i, a := range args {
		if a == "--server" {
			serverIdx = i
		}
		if a == "--port" {
			portIdx = i
		}
	}
	if serverIdx < 0 || serverIdx+1 >= len(args) || args[serverIdx+1] != "bw.match.local" {
		t.Errorf("--server missing or wrong value: %v", args)
	}
	if portIdx < 0 || portIdx+1 >= len(args) || args[portIdx+1] != "25565" {
		t.Errorf("--port missing or wrong value: %v", args)
	}
}

// Without a join target the extra arguments must not appear.
func TestBuildLaunchCommandWithoutJoinTarget(t *testing.T) {
	gameDir := t.TempDir()
	writeVersionJSON(t, gameDir, "1.99", testVersionJSON)

	svc := NewLauncherService()
	args, err := svc.BuildLaunchCommand(
		models.LauncherConfig{JavaPath: "/opt/java/bin/java", MaxMemory: 2048, MinMemory: 512, GameDir: gameDir},
		&models.MinecraftVersion{VersionID: "1.99"},
		&models.MinecraftAccount{
			Username: "Steve", UUID: "11111111-2222-3333-4444-555555555555",
			AccessToken: "at", ClientToken: "ct", AuthType: "hrpauth",
		},
	)
	if err != nil {
		t.Fatalf("BuildLaunchCommand: %v", err)
	}
	for _, a := range args {
		if a == "--server" {
			t.Errorf("--server present without a join target: %v", args)
		}
	}
}
