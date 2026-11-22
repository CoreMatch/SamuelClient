package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
		{"v1.2.3", "v1.2.4", -1},
		{"v1.2.3", "v2.0.0", -1},
		{"v1.2.3-rc.1", "v1.2.3", -1},
		{"v1.2.3", "v1.2.3-rc.1", 1},
	} {
		current, err := parseVersion(tc.current)
		if err != nil {
			t.Fatalf("parse current %q: %v", tc.current, err)
		}
		latest, err := parseVersion(tc.latest)
		if err != nil {
			t.Fatalf("parse latest %q: %v", tc.latest, err)
		}
		if got := compareVersions(current, latest); got != tc.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", tc.current, tc.latest, got, tc.want)
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

func TestFindAssetUsesPlatformSuffix(t *testing.T) {
	assets := []releaseAsset{
		{Name: "SamuelClient-v1.2.0-linux-amd64", URL: "linux"},
		{Name: "SamuelClient-v1.2.0-windows-amd64.exe", URL: "windows"},
	}
	for _, tc := range []struct{ goos, goarch, want string }{
		{"linux", "amd64", "linux"},
		{"windows", "amd64", "windows"},
	} {
		got, err := findAsset(assets, tc.goos, tc.goarch)
		if err != nil || got.URL != tc.want {
			t.Errorf("findAsset(%s/%s) = %+v, %v; want %q", tc.goos, tc.goarch, got, err, tc.want)
		}
	}
	if _, err := findAsset(assets, "darwin", "arm64"); err == nil {
		t.Fatal("findAsset unexpectedly found an unavailable platform")
	}
}

func TestCheckAndStageDownloadsAndVerifiesRelease(t *testing.T) {
	binary := []byte("new executable contents")
	digest := sha256.Sum256(binary)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"tag_name":"v1.2.0","assets":[{"name":"SamuelClient-v1.2.0-%s","browser_download_url":"https://github.com/CoreMatch/SamuelClient/releases/download/v1.2.0/app","digest":"sha256:%s"}]}`,
			testAssetSuffix(), hex.EncodeToString(digest[:]))
	}))
	defer api.Close()
	client := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "github.com" {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(stringsReader(string(binary))),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		}
		return http.DefaultTransport.RoundTrip(req)
	})}
	executable := filepath.Join(t.TempDir(), "SamuelClient")
	if err := os.WriteFile(executable, []byte("old executable"), 0755); err != nil {
		t.Fatal(err)
	}

	staged, tag, err := CheckAndStage(context.Background(), "v1.0.0", executable, api.URL, client)
	if err != nil {
		t.Fatalf("CheckAndStage failed: %v", err)
	}
	if tag != "v1.2.0" {
		t.Fatalf("release tag = %q, want v1.2.0", tag)
	}
	got, err := os.ReadFile(staged)
	if err != nil {
		t.Fatalf("read staged executable: %v", err)
	}
	if string(got) != string(binary) {
		t.Fatalf("staged executable = %q, want %q", got, binary)
	}
	if info, err := os.Stat(staged); err != nil || info.Mode().Perm()&0111 == 0 {
		t.Fatalf("staged executable is not executable (info=%v, err=%v)", info, err)
	}
}

func TestCheckAndStageSkipsSameVersion(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"tag_name":"v1.2.0","assets":[]}`)
	}))
	defer api.Close()
	executable := filepath.Join(t.TempDir(), "SamuelClient")
	if err := os.WriteFile(executable, []byte("current"), 0755); err != nil {
		t.Fatal(err)
	}

	staged, tag, err := CheckAndStage(context.Background(), "1.2.0", executable, api.URL, api.Client())
	if err != nil {
		t.Fatalf("CheckAndStage failed: %v", err)
	}
	if staged != "" || tag != "v1.2.0" {
		t.Fatalf("result = (%q, %q), want no staged file and v1.2.0", staged, tag)
	}
}

func TestCheckAndStageRejectsBadDigest(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"tag_name":"v1.2.0","assets":[{"name":"SamuelClient-v1.2.0-%s","browser_download_url":"https://github.com/CoreMatch/SamuelClient/releases/download/v1.2.0/app","digest":"sha256:%064d"}]}`,
			testAssetSuffix(), 0)
	}))
	defer api.Close()
	client := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "github.com" {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(stringsReader("tampered")), Header: make(http.Header), Request: req}, nil
		}
		return http.DefaultTransport.RoundTrip(req)
	})}
	executable := filepath.Join(t.TempDir(), "SamuelClient")
	if err := os.WriteFile(executable, []byte("current"), 0755); err != nil {
		t.Fatal(err)
	}
	if staged, _, err := CheckAndStage(context.Background(), "v1.0.0", executable, api.URL, client); err == nil || staged != "" {
		t.Fatalf("CheckAndStage = (%q, %v), want digest error", staged, err)
	}
	entries, err := os.ReadDir(filepath.Dir(executable))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("temporary files remain after rejected update: %v", entries)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func stringsReader(value string) io.Reader {
	return strings.NewReader(value)
}

func testAssetSuffix() string {
	suffix := runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		suffix += ".exe"
	}
	return suffix
}
