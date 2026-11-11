package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Cancellation tests for the install/download path (PENDING-DECISIONS #10,
// option B). A cancelled context must abort the transfer and surface the
// ErrInstallCancelled sentinel rather than a generic failure.

func TestDownloadFileCancelledContext(t *testing.T) {
	// A server that would otherwise serve the file; the context is already
	// cancelled so the request must never succeed.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("data"))
	}))
	defer srv.Close()

	svc := NewBMCLAPIServiceWithBaseURL(srv.URL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before the call

	result := svc.DownloadFile(ctx, srv.URL+"/file", t.TempDir()+"/out", 0, nil)
	if result.Success {
		t.Fatalf("DownloadFile succeeded on a cancelled context")
	}
	if !strings.Contains(result.Error, ErrInstallCancelled.Error()) {
		t.Fatalf("error = %q, want it to contain %q", result.Error, ErrInstallCancelled.Error())
	}
}

func TestInstallVersionCancelledContext(t *testing.T) {
	svc := NewBMCLAPIServiceWithBaseURL("http://127.0.0.1:0") // unreachable; ctx cancels first
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := svc.InstallVersion(ctx, "1.99", t.TempDir(), nil)
	if result.Success {
		t.Fatalf("InstallVersion succeeded on a cancelled context")
	}
	if !strings.Contains(result.Error, ErrInstallCancelled.Error()) {
		t.Fatalf("error = %q, want it to contain %q", result.Error, ErrInstallCancelled.Error())
	}
}

func TestDownloadLibrariesCancelledContext(t *testing.T) {
	svc := NewBMCLAPIServiceWithBaseURL("http://127.0.0.1:0")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// One library so the loop body runs and hits the ctx check.
	libs := []BMCLLibrary{{
		Name: "com.example:lib:1.0",
		Downloads: BMCLLibraryDownloads{
			Artifact: BMCLArtifact{URL: "https://libraries.minecraft.net/com/example/lib/1.0/lib-1.0.jar", Size: 10},
		},
	}}
	result := svc.DownloadLibraries(ctx, libs, t.TempDir(), nil)
	if result.Success {
		t.Fatalf("DownloadLibraries succeeded on a cancelled context")
	}
	if !strings.Contains(result.Error, ErrInstallCancelled.Error()) {
		t.Fatalf("error = %q, want it to contain %q", result.Error, ErrInstallCancelled.Error())
	}
}
