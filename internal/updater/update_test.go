package updater

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	for _, tc := range []struct {
		current, latest string
		want            int
	}{
		{"v1.2.3", "v1.2.3", 0},
		{"v1.2.3", "v1.2.4", 1},
		{"v1.2.3", "v2.0.0", 1},
		{"v1.2.3-rc.2", "v1.2.3-rc.10", 1},
		{"v1.2.3-rc.10", "v1.2.3", 1},
		{"v1.2.3-rc.1", "v1.2.3", 1},
		{"v1.2.3", "v1.2.3-rc.1", -1},
	} {
		current, err := parseVersion(tc.current)
		if err != nil {
			t.Fatalf("parse current %q: %v", tc.current, err)
		}
		latest, err := parseVersion(tc.latest)
		if err != nil {
			t.Fatalf("parse latest %q: %v", tc.latest, err)
		}
		if got := compareVersions(latest, current); got != tc.want {
			t.Errorf("compare %q to %q = %d, want %d", tc.latest, tc.current, got, tc.want)
		}
	}
}

func TestDetectVersion(t *testing.T) {
	for _, tc := range []struct {
		build, path, want string
	}{
		{"v1.2.0", "/opt/SamuelClient", "v1.2.0"},
		{"dev", "/opt/SamuelClient-v0.1.0-linux-amd64", "v0.1.0"},
		{"dev", "/Apps/SamuelClient-v0.1.0-windows-amd64.exe", "v0.1.0"},
		{"dev", "/opt/SamuelClient", "dev"},
	} {
		if got := DetectVersion(tc.build, tc.path); got != tc.want {
			t.Errorf("DetectVersion(%q, %q) = %q, want %q", tc.build, tc.path, got, tc.want)
		}
	}
}

func TestSignedUpdateDownloadsAndVerifies(t *testing.T) {
	binary := []byte("new executable contents")
	server, client := manifestServer(t, binary, "v1.2.0", false, false)
	defer server.Close()
	executable := writeExecutable(t)

	update, err := Check(context.Background(), "v1.0.0", server.URL+"/v1/channels/stable.json", client)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if update == nil || update.Version != "v1.2.0" {
		t.Fatalf("unexpected update: %+v", update)
	}
	var finalDownloaded int64
	staged, err := DownloadAndStage(context.Background(), update, executable, client, func(downloaded, total int64) {
		if total != int64(len(binary)) {
			t.Errorf("progress total = %d, want %d", total, len(binary))
		}
		finalDownloaded = downloaded
	})
	if err != nil {
		t.Fatalf("DownloadAndStage failed: %v", err)
	}
	defer os.Remove(staged)
	got, err := os.ReadFile(staged)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(binary) {
		t.Fatalf("staged content = %q", got)
	}
	if finalDownloaded != int64(len(binary)) {
		t.Fatalf("final progress = %d", finalDownloaded)
	}
	info, err := os.Stat(staged)
	if err != nil || info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("staged update is not executable: info=%v err=%v", info, err)
	}
}

func TestCheckRejectsInvalidSignature(t *testing.T) {
	server, client := manifestServer(t, []byte("binary"), "v1.2.0", false, false)
	defer server.Close()
	raw := fetch(t, client, server.URL+"/v1/channels/stable.json")
	var envelope signedManifest
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	envelope.Signature = mutateBase64(t, envelope.Signature)
	tampered, _ := json.Marshal(envelope)
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(tampered)
	}))
	defer redirect.Close()
	if _, err := Check(context.Background(), "v1.0.0", redirect.URL, client); err == nil {
		t.Fatal("Check accepted an invalid signature")
	}
}

func TestDownloadRejectsTamperedAssetAndCleansStaging(t *testing.T) {
	expected := []byte("expected binary")
	server, client := manifestServer(t, expected, "v1.2.0", false, false)
	defer server.Close()
	executable := writeExecutable(t)
	update, err := Check(context.Background(), "v1.0.0", server.URL, client)
	if err != nil {
		t.Fatal(err)
	}

	tampered := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".json") {
			_, _ = w.Write(update.ManifestJSON())
			return
		}
		_, _ = w.Write([]byte("tampered binary"))
	}))
	defer tampered.Close()
	update.Asset.URL = tampered.URL + strings.TrimPrefix(update.Asset.URL, server.URL)
	if staged, err := DownloadAndStage(context.Background(), update, executable, tampered.Client(), nil); err == nil || staged != "" {
		t.Fatalf("DownloadAndStage = (%q, %v), want digest error", staged, err)
	}
	entries, err := os.ReadDir(filepath.Dir(executable))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("temporary files remain after rejected update: %v", entries)
	}
}

func TestCheckSkipsSameVersionAndSupportsSignedDowngrade(t *testing.T) {
	for _, tc := range []struct {
		name           string
		current        string
		version        string
		allowDowngrade bool
		wantUpdate     bool
	}{
		{name: "same", current: "v1.2.0", version: "v1.2.0", wantUpdate: false},
		{name: "older", current: "v1.2.0", version: "v1.1.0", wantUpdate: false},
		{name: "rollback", current: "v1.2.0", version: "v1.1.0", allowDowngrade: true, wantUpdate: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, client := manifestServer(t, []byte("binary"), tc.version, tc.allowDowngrade, tc.allowDowngrade)
			defer server.Close()
			update, err := Check(context.Background(), tc.current, server.URL, client)
			if err != nil {
				t.Fatal(err)
			}
			if (update != nil) != tc.wantUpdate {
				t.Fatalf("update = %+v, wantUpdate %v", update, tc.wantUpdate)
			}
			if update != nil && update.AllowDowngrade != tc.allowDowngrade {
				t.Fatalf("allowDowngrade = %v", update.AllowDowngrade)
			}
		})
	}
}

func TestCheckRejectsCrossOriginRedirect(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"schemaVersion":1}`))
	}))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer redirect.Close()
	if _, err := Check(context.Background(), "v1.0.0", redirect.URL, redirect.Client()); err == nil {
		t.Fatal("Check accepted a cross-origin redirect")
	}
}

func TestManifestCacheRoundTrip(t *testing.T) {
	server, client := manifestServer(t, []byte("binary"), "v1.2.0", true, false)
	defer server.Close()
	raw := fetch(t, client, server.URL)
	cachePath := filepath.Join(t.TempDir(), "data", "update-cache.json")
	if err := SaveManifestCache(cachePath, raw); err != nil {
		t.Fatal(err)
	}
	update, err := LoadCachedUpdate(cachePath, "v1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if update == nil || !update.Mandatory || update.Version != "v1.2.0" {
		t.Fatalf("unexpected cached update: %+v", update)
	}
}

func manifestServer(t *testing.T, binary []byte, version string, mandatory, allowDowngrade bool) (*httptest.Server, *http.Client) {
	t.Helper()
	public, private := generateKeys(t)
	installTrustedKey(t, public)
	asset := ReleaseAsset{
		OS:     runtime.GOOS,
		Arch:   runtime.GOARCH,
		Name:   testAssetName(version),
		URL:    testAssetPath(version),
		Size:   int64(len(binary)),
		SHA256: fmt.Sprintf("%x", sha256.Sum256(binary)),
	}
	payload := manifestPayload{
		SchemaVersion:  1,
		Channel:        "stable",
		Version:        version,
		ReleasedAt:     "2026-11-24T12:00:00Z",
		Mandatory:      mandatory,
		AllowDowngrade: allowDowngrade,
		Notes:          "Test release",
		KeyID:          keyID(public),
		Assets:         []ReleaseAsset{asset},
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	envelope := signedManifest{
		SchemaVersion: 1,
		Payload:       base64.RawURLEncoding.EncodeToString(payloadBytes),
		Signature:     base64.RawURLEncoding.EncodeToString(ed25519.Sign(private, payloadBytes)),
	}
	raw, _ := json.Marshal(envelope)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/channels/stable.json", "/":
			_, _ = w.Write(raw)
		case testAssetPath(version):
			_, _ = w.Write(binary)
		default:
			http.NotFound(w, r)
		}
	}))
	return server, server.Client()
}

func generateKeys(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return public, private
}

func installTrustedKey(t *testing.T, public ed25519.PublicKey) {
	t.Helper()
	previous := trustedPublicKeys
	trustedPublicKeys = map[string][]byte{keyID(public): public}
	t.Cleanup(func() { trustedPublicKeys = previous })
}

func keyID(public ed25519.PublicKey) string {
	digest := sha256.Sum256(public)
	return hex.EncodeToString(digest[:8])
}

func testAssetSuffix() string {
	suffix := "-" + runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		suffix += ".exe"
	}
	return suffix
}

func testAssetName(version string) string {
	return "SamuelClient-" + version + testAssetSuffix()
}

func testAssetPath(version string) string {
	return "/v1/releases/" + version + "/" + runtime.GOOS + "-" + runtime.GOARCH + "/" + testAssetName(version)
}

func writeExecutable(t *testing.T) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "SamuelClient")
	if err := os.WriteFile(file, []byte("old executable"), 0o755); err != nil {
		t.Fatal(err)
	}
	return file
}

func fetch(t *testing.T, client *http.Client, target string) []byte {
	t.Helper()
	response, err := client.Get(target)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func mutateBase64(t *testing.T, value string) string {
	t.Helper()
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		t.Fatal(err)
	}
	decoded[0] ^= 0xff
	return base64.RawURLEncoding.EncodeToString(decoded)
}
