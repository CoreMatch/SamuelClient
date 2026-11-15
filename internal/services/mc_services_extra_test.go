package services

// Second test batch (unattended follow-up): covers the launch-log line
// splitter and the remaining BMCLAPI download helpers that were left
// uncovered by the first batch.

import (
	"bytes"
	"context"
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
	result := svc.DownloadAssets(context.Background(), &BMCLAssetIndex{
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
	result = svc.DownloadAssets(context.Background(), &BMCLAssetIndex{ID: "5", URL: "https://piston-meta.mojang.com/v1/assets/index.json"}, assetsDir, func(name string, downloaded, total int64) {
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
	result := svc.GetFabricInstaller(context.Background(), "1.99", "0.15.0", dir)
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
		if r.URL.Path == "/maven/net/minecraftforge/forge/1.12.2-14.23.5.2859/forge-1.12.2-14.23.5.2859-installer.jar" {
			io.WriteString(w, "forge-bytes")
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc := NewBMCLAPIServiceWithBaseURL(srv.URL)
	dir := t.TempDir()
	result := svc.DownloadForgeInstaller(context.Background(), "1.12.2", "14.23.5.2859", dir)
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

// Regression for the "progress bar stuck at 100%" bug: each artifact used to
// report (size, size), i.e. 100%, so the bar pinned at 100% after the first
// library and never moved for the rest of the install. Progress must instead
// accumulate toward a shared stage total, reaching it only on the last file.
func TestDownloadLibrariesProgressIsCumulative(t *testing.T) {
	sizes := map[string]int{
		"/maven/com/example/a/1.0/a-1.0.jar": 2,
		"/maven/com/example/b/1.0/b-1.0.jar": 3,
		"/maven/com/example/c/1.0/c-1.0.jar": 5,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n, ok := sizes[r.URL.Path]; ok {
			io.WriteString(w, strings.Repeat("x", n))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc := NewBMCLAPIServiceWithBaseURL(srv.URL)
	libs := []BMCLLibrary{
		{Name: "com.example:a:1.0", Downloads: BMCLLibraryDownloads{Artifact: BMCLArtifact{Path: "com/example/a/1.0/a-1.0.jar", URL: "https://libraries.minecraft.net/com/example/a/1.0/a-1.0.jar", Size: 2}}},
		{Name: "com.example:b:1.0", Downloads: BMCLLibraryDownloads{Artifact: BMCLArtifact{Path: "com/example/b/1.0/b-1.0.jar", URL: "https://libraries.minecraft.net/com/example/b/1.0/b-1.0.jar", Size: 3}}},
		{Name: "com.example:c:1.0", Downloads: BMCLLibraryDownloads{Artifact: BMCLArtifact{Path: "com/example/c/1.0/c-1.0.jar", URL: "https://libraries.minecraft.net/com/example/c/1.0/c-1.0.jar", Size: 5}}},
	}

	type ev struct{ downloaded, total int64 }
	var events []ev
	result := svc.DownloadLibraries(context.Background(), libs, t.TempDir(), func(name string, downloaded, total int64) {
		events = append(events, ev{downloaded, total})
	})
	if !result.Success {
		t.Fatalf("DownloadLibraries failed: %s", result.Error)
	}
	if len(events) != 3 {
		t.Fatalf("progress events = %d, want 3", len(events))
	}
	want := []ev{{2, 10}, {5, 10}, {10, 10}}
	for i, e := range events {
		if e != want[i] {
			t.Errorf("event %d = %+v, want %+v (cumulative toward stage total)", i, e, want[i])
		}
	}
}

// Same regression for the asset stage (the longest part of an install). Map
// iteration order is random, so assert the cumulative invariants rather than
// an exact sequence: a constant stage total, strictly increasing downloaded,
// and 100% only on the final object.
func TestDownloadAssetsProgressIsCumulative(t *testing.T) {
	objs := []struct {
		hash string
		size int
	}{
		{strings.Repeat("a", 40), 2},
		{strings.Repeat("b", 40), 3},
		{strings.Repeat("c", 40), 5},
	}
	mux := http.NewServeMux()
	var index strings.Builder
	index.WriteString(`{"objects":{`)
	for i, o := range objs {
		if i > 0 {
			index.WriteString(",")
		}
		fmt.Fprintf(&index, `"obj/%d":{"hash":%q,"size":%d}`, i, o.hash, o.size)
	}
	index.WriteString("}}")
	mux.HandleFunc("/v1/assets/index.json", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, index.String())
	})
	for _, o := range objs {
		o := o
		mux.HandleFunc("/assets/"+o.hash[:2]+"/"+o.hash, func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, strings.Repeat("x", o.size))
		})
	}
	srv := httptest.NewServer(mux)
	defer srv.Close()

	svc := NewBMCLAPIServiceWithBaseURL(srv.URL)
	type ev struct{ downloaded, total int64 }
	var events []ev
	result := svc.DownloadAssets(context.Background(), &BMCLAssetIndex{ID: "5", URL: "https://piston-meta.mojang.com/v1/assets/index.json"}, t.TempDir(), func(name string, downloaded, total int64) {
		events = append(events, ev{downloaded, total})
	})
	if !result.Success {
		t.Fatalf("DownloadAssets failed: %s", result.Error)
	}
	if len(events) != 3 {
		t.Fatalf("progress events = %d, want 3", len(events))
	}
	const wantTotal = 10
	var prev int64
	for i, e := range events {
		if e.total != wantTotal {
			t.Errorf("event %d total = %d, want the constant stage total %d", i, e.total, wantTotal)
		}
		if e.downloaded <= prev {
			t.Errorf("event %d downloaded = %d, want > %d (cumulative)", i, e.downloaded, prev)
		}
		if i < len(events)-1 && e.downloaded == wantTotal {
			t.Errorf("event %d hit 100%% before the final object (the stuck-at-100 bug)", i)
		}
		prev = e.downloaded
	}
	if prev != wantTotal {
		t.Errorf("final downloaded = %d, want %d", prev, wantTotal)
	}
}
