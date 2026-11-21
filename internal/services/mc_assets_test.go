package services

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDownloadAssetsRetriesTransientFailures(t *testing.T) {
	hash := strings.Repeat("ab", 20)
	var indexAttempts, assetAttempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/index.json" {
			if indexAttempts.Add(1) == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			fmt.Fprintf(w, `{"objects":{"one":{"hash":%q,"size":5},"duplicate":{"hash":%q,"size":5}}}`, hash, hash)
			return
		}
		switch assetAttempts.Add(1) {
		case 1:
			w.WriteHeader(http.StatusForbidden)
		case 2:
			io.WriteString(w, "part")
		default:
			io.WriteString(w, "asset")
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "objects", "ab", hash)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	var events int
	result := NewBMCLAPIServiceWithBaseURL(srv.URL).DownloadAssets(t.Context(), &BMCLAssetIndex{URL: srv.URL + "/index.json"}, dir, func(_ string, downloaded, total int64) {
		events++
		if downloaded != 5 || total != 5 {
			t.Errorf("progress = %d/%d, want 5/5", downloaded, total)
		}
	})
	if !result.Success || indexAttempts.Load() != 2 || assetAttempts.Load() != 3 || events != 1 {
		t.Fatalf("result = %+v, index attempts = %d, asset attempts = %d, progress events = %d", result, indexAttempts.Load(), assetAttempts.Load(), events)
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "asset" {
		t.Fatalf("asset = %q, err = %v", content, err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Fatalf("unexpected entries after retries: %v, err = %v", entries, err)
	}
}

func TestDownloadAssetsConcurrentRequests(t *testing.T) {
	hashA, hashB := strings.Repeat("a", 40), strings.Repeat("b", 40)
	var started atomic.Int32
	overlap := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/index.json" {
			fmt.Fprintf(w, `{"objects":{"a":{"hash":%q,"size":1},"b":{"hash":%q,"size":1}}}`, hashA, hashB)
			return
		}
		if started.Add(1) == 2 {
			close(overlap)
		}
		select {
		case <-overlap:
			io.WriteString(w, "x")
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	result := NewBMCLAPIServiceWithBaseURL(srv.URL).DownloadAssets(ctx, &BMCLAssetIndex{URL: srv.URL + "/index.json"}, t.TempDir(), nil)
	if !result.Success || started.Load() != 2 {
		t.Fatalf("result = %+v, started %d requests, expected two concurrent requests", result, started.Load())
	}
}

func TestDownloadAssetsCancellationCleansUp(t *testing.T) {
	hash := strings.Repeat("ab", 20)
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/index.json" {
			fmt.Fprintf(w, `{"objects":{"a":{"hash":%q,"size":5}}}`, hash)
			return
		}
		io.WriteString(w, "x")
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	go func() {
		select {
		case <-started:
			cancel()
		case <-ctx.Done():
		}
	}()
	dir := t.TempDir()
	result := NewBMCLAPIServiceWithBaseURL(srv.URL).DownloadAssets(ctx, &BMCLAssetIndex{URL: srv.URL + "/index.json"}, dir, nil)
	if result.Success || !strings.Contains(result.Error, ErrInstallCancelled.Error()) {
		t.Fatalf("expected cancellation, got %+v", result)
	}
	entries, err := os.ReadDir(filepath.Join(dir, "objects", "ab"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("partial files remain: %v, err = %v", entries, err)
	}
}
