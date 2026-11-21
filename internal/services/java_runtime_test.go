package services

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJavaRuntimeInstallerDownloadsExtractsAndReportsProgress(t *testing.T) {
	archive := testJavaTarGz(t, map[string]string{"jdk-25/bin/java": "#!/bin/sh\n"})
	var requestPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".sha256.txt") {
			sum := sha256.Sum256(archive)
			_, _ = fmt.Fprintf(w, "%x  temurin.tar.gz\n", sum)
			return
		}
		requestPath = r.URL.Path
		_, _ = w.Write(archive)
	}))
	defer server.Close()

	destination := filepath.Join(t.TempDir(), "jre", "25")
	installer := &JavaRuntimeInstaller{
		APIBaseURL: server.URL,
		HTTPClient: server.Client(),
		OS:         "linux",
		Arch:       "amd64",
	}
	var stages []string
	got, err := installer.Install(context.Background(), 25, destination, func(stage string, _, _ int64) {
		stages = append(stages, stage)
	})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if requestPath != "/25/ga/linux/x64/jre/hotspot/normal/eclipse" {
		t.Fatalf("request path = %q", requestPath)
	}
	wantJava := filepath.Join(destination, "bin", "java")
	if got.JavaPath != wantJava || got.Version != 25 {
		t.Fatalf("Install() = %+v, want java path %q and version 25", got, wantJava)
	}
	if data, err := os.ReadFile(wantJava); err != nil || string(data) != "#!/bin/sh\n" {
		t.Fatalf("installed java contents = %q, error = %v", data, err)
	}
	if info, err := os.Stat(wantJava); err != nil || info.Mode().Perm()&0111 == 0 {
		t.Fatalf("installed java mode = %v, error = %v", info, err)
	}
	if len(stages) < 3 || stages[0] != "java-download" || stages[len(stages)-1] != "java-extract" {
		t.Fatalf("progress stages = %v", stages)
	}
}

func TestJavaRuntimeInstallerUsesMacOSBundleJavaPath(t *testing.T) {
	archive := testJavaTarGz(t, map[string]string{
		"jdk-25.jdk/Contents/Home/bin/java": "#!/bin/sh\n",
	})
	server := javaArchiveTestServer(t, archive)
	defer server.Close()

	destination := filepath.Join(t.TempDir(), "jre", "25")
	installer := &JavaRuntimeInstaller{
		APIBaseURL: server.URL,
		HTTPClient: server.Client(),
		OS:         "darwin",
		Arch:       "arm64",
	}
	got, err := installer.Install(context.Background(), 25, destination, nil)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	want := filepath.Join(destination, "Contents", "Home", "bin", "java")
	if got.JavaPath != want {
		t.Fatalf("JavaPath = %q, want %q", got.JavaPath, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("installed macOS Java executable: %v", err)
	}
}

func TestJavaRuntimeInstallerRejectsUnsupportedAndExistingTargets(t *testing.T) {
	installer := &JavaRuntimeInstaller{OS: "linux", Arch: "amd64"}
	target := filepath.Join(t.TempDir(), "existing")
	if err := os.Mkdir(target, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := installer.Install(context.Background(), 22, target, nil); err == nil || !strings.Contains(err.Error(), "unsupported Java version") {
		t.Fatalf("unsupported version error = %v", err)
	}
	if _, err := installer.Install(context.Background(), 21, target, nil); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("existing target error = %v", err)
	}
}

func TestJavaRuntimeArchiveRejectsPathTraversal(t *testing.T) {
	var archive bytes.Buffer
	zw := zip.NewWriter(&archive)
	entry, err := zw.Create("jdk-21/../../escaped")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = entry.Write([]byte("no"))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	destination := t.TempDir()
	if err := extractJavaZip(context.Background(), writeTempArchive(t, archive.Bytes()), destination); err == nil {
		t.Fatal("extractJavaZip accepted a traversal entry")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(destination), "escaped")); !os.IsNotExist(err) {
		t.Fatalf("escaped file stat error = %v", err)
	}
}

func javaArchiveTestServer(t *testing.T, archive []byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".sha256.txt") {
			sum := sha256.Sum256(archive)
			_, _ = fmt.Fprintf(w, "%x  temurin.tar.gz\n", sum)
			return
		}
		_, _ = w.Write(archive)
	}))
}

func testJavaTarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{
			Name:     name,
			Mode:     0644,
			Size:     int64(len(body)),
			Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return archive.Bytes()
}

func writeTempArchive(t *testing.T, content []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "archive.zip")
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
