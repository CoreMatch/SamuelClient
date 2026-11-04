package services

// Second test batch (unattended follow-up): covers the launch-log line
// splitter and the remaining BMCLAPI download helpers that were left
// uncovered by the first batch.

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStreamLines(t *testing.T) {
	var w bytes.Buffer
	var got []string
	streamLines(&w, strings.NewReader("one\ntwo\nthree"), "stdout", func(source, line string) {
		if source != "stdout" {
			t.Errorf("source = %q, want stdout", source)
		}
		got = append(got, line)
	})

	if len(got) != 3 || got[0] != "one" || got[2] != "three" {
		t.Errorf("lines = %v, want [one two three]", got)
	}
	if w.String() != "one\ntwo\nthree\n" {
		t.Errorf("written to file = %q, want the same lines", w.String())
	}
}

func TestDownloadAssets(t *testing.T) {
	hash := strings.Repeat("ab", 16) // 32 hex chars
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/assets/index.json", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"objects": {"sound/one": {"hash": %q, "size": 5}}}`, hash)
	})
	mux.HandleFunc("/assets/"+hash[:2]+"/"+hash, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "asset")
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	svc := NewBMCLAPIServiceWithBaseURL(srv.URL)
	assetsDir := filepath.Join(t.TempDir(), "assets")

	var progressNames []string
	result := svc.DownloadAssets(&BMCLAssetIndex{
		ID:   "5",
		URL:  "https://piston-meta.mojang.com/v1/assets/index.json",
		Size: 1,
	}, assetsDir, func(name string, downloaded, total int64) {
		progressNames = append(progressNames, name)
	})
	if !result.Success {
		t.Fatalf("DownloadAssets failed: %s", result.Error)
	}

	got, err := os.ReadFile(filepath.Join(assetsDir, "objects", hash[:2], hash))
	if err != nil || string(got) != "asset" {
		t.Errorf("asset object missing or wrong (err=%v got=%q)", err, got)
	}
	if len(progressNames) != 1 || progressNames[0] != "sound/one" {
		t.Errorf("progress = %v, want one call for sound/one", progressNames)
	}

	// Second run: the object exists, so it is skipped (no progress call).
	progressNames = nil
	result = svc.DownloadAssets(&BMCLAssetIndex{ID: "5", URL: "https://piston-meta.mojang.com/v1/assets/index.json"}, assetsDir, func(name string, downloaded, total int64) {
		progressNames = append(progressNames, name)
	})
	if !result.Success {
		t.Fatalf("second DownloadAssets failed: %s", result.Error)
	}
	if len(progressNames) != 0 {
		t.Errorf("existing asset must be skipped, progress = %v", progressNames)
	}
}

func TestGetFabricInstaller(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/fabric-meta/v2/versions/loader/1.99/0.15.0/profile/jar" {
			io.WriteString(w, "installer-bytes")
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc := NewBMCLAPIServiceWithBaseURL(srv.URL)
	dir := t.TempDir()
	result := svc.GetFabricInstaller("1.99", "0.15.0", dir)
	if !result.Success {
		t.Fatalf("GetFabricInstaller failed: %s", result.Error)
	}
	got, err := os.ReadFile(filepath.Join(dir, "fabric-loader-0.15.0.jar"))
	if err != nil || string(got) != "installer-bytes" {
		t.Errorf("installer missing or wrong (err=%v got=%q)", err, got)
	}
}

func TestDownloadForgeInstaller(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/forge/minecraft/1.12.2/14.23.5.2859/installer" {
			io.WriteString(w, "forge-bytes")
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc := NewBMCLAPIServiceWithBaseURL(srv.URL)
	dir := t.TempDir()
	result := svc.DownloadForgeInstaller("1.12.2", "14.23.5.2859", dir)
	if !result.Success {
		t.Fatalf("DownloadForgeInstaller failed: %s", result.Error)
	}
	got, err := os.ReadFile(filepath.Join(dir, "forge-installer-14.23.5.2859.jar"))
	if err != nil || string(got) != "forge-bytes" {
		t.Errorf("installer missing or wrong (err=%v got=%q)", err, got)
	}
}

func TestGetVersionJARErrors(t *testing.T) {
	svc := NewBMCLAPIService()

	if result := svc.GetVersionJAR(&BMCLVersionJSON{}); result.Success {
		t.Error("missing downloads must fail")
	}
	vj := &BMCLVersionJSON{Downloads: map[string]BMCLDownload{
		"server": {URL: "https://piston-data.mojang.com/x.jar", Size: 1},
	}}
	if result := svc.GetVersionJAR(vj); result.Success {
		t.Error("missing client download must fail")
	}
}
