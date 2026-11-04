package services

// Loader installation tests: Fabric (fake meta API) and Forge (fake
// installer script standing in for `java -jar installer --installClient`).

import (
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
	"time"
)

// ==================== Fabric ====================

// fabricMetaFixture serves the Fabric meta profile endpoint plus the loader
// library. libStatus controls the library response (http.StatusOK or 404).
func fabricMetaFixture(t *testing.T, profileStatus, libStatus int) *httptest.Server {
	t.Helper()
	var srvURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/fabric-meta/v2/versions/loader/1.99/0.15.0/profile/json", func(w http.ResponseWriter, r *http.Request) {
		if profileStatus != http.StatusOK {
			http.Error(w, "no such loader", profileStatus)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{
			"id": "fabric-loader-0.15.0-1.99",
			"inheritsFrom": "1.99",
			"mainClass": "net.fabricmc.loader.impl.launch.knot.KnotClient",
			"libraries": [
				{
					"name": "net.fabricmc:fabric-loader:0.15.0",
					"downloads": {"artifact": {"path": "net/fabricmc/fabric-loader/0.15.0/fabric-loader-0.15.0.jar", "url": "%s/maven/fabric-loader.jar", "size": 11}}
				}
			]
		}`, srvURL)
	})
	mux.HandleFunc("/maven/fabric-loader.jar", func(w http.ResponseWriter, r *http.Request) {
		if libStatus != http.StatusOK {
			http.NotFound(w, r)
			return
		}
		io.WriteString(w, "fabric-lib")
	})
	srv := httptest.NewServer(mux)
	srvURL = srv.URL
	t.Cleanup(srv.Close)
	return srv
}

func TestInstallFabricLoaderHappyPath(t *testing.T) {
	srv := fabricMetaFixture(t, http.StatusOK, http.StatusOK)
	gameDir := t.TempDir()
	writeVersionJSON(t, gameDir, "1.99", testVersionJSON)

	var stages []string
	result := NewBMCLAPIServiceWithBaseURL(srv.URL).InstallFabricLoader("1.99", "0.15.0", gameDir, func(stage, name string, downloaded, total int64) {
		stages = append(stages, stage)
	})
	if !result.Success {
		t.Fatalf("InstallFabricLoader failed: %s", result.Error)
	}

	data := result.Data.(map[string]interface{})
	if data["version_id"] != "fabric-loader-0.15.0-1.99" {
		t.Errorf("version_id = %v", data["version_id"])
	}
	if data["inherits_from"] != "1.99" {
		t.Errorf("inherits_from = %v", data["inherits_from"])
	}

	// The profile JSON is persisted with inheritsFrom intact; the launch
	// command resolves it (TestBuildLaunchCommandInheritsFromParent).
	versionDir := filepath.Join(gameDir, "versions", "fabric-loader-0.15.0-1.99")
	raw, err := os.ReadFile(filepath.Join(versionDir, "fabric-loader-0.15.0-1.99.json"))
	if err != nil {
		t.Fatalf("profile json not written: %v", err)
	}
	var profile BMCLVersionJSON
	if err := json.Unmarshal(raw, &profile); err != nil {
		t.Fatalf("persisted profile invalid: %v", err)
	}
	if profile.InheritsFrom != "1.99" || profile.MainClass != "net.fabricmc.loader.impl.launch.knot.KnotClient" {
		t.Errorf("profile = %+v, want inheritsFrom 1.99 and fabric mainClass", profile)
	}

	// Loader library downloaded into the shared libraries dir.
	libPath := filepath.Join(gameDir, "libraries", "net", "fabricmc", "fabric-loader", "0.15.0", "fabric-loader-0.15.0.jar")
	if got, err := os.ReadFile(libPath); err != nil || string(got) != "fabric-lib" {
		t.Errorf("loader library missing or wrong (err=%v got=%q)", err, got)
	}

	// All progress events carry the "loader" stage.
	if len(stages) == 0 {
		t.Error("no progress events emitted")
	}
	for _, s := range stages {
		if s != "loader" {
			t.Errorf("progress stage = %q, want loader", s)
		}
	}
}

func TestInstallFabricLoaderMissingParent(t *testing.T) {
	srv := fabricMetaFixture(t, http.StatusOK, http.StatusOK)
	result := NewBMCLAPIServiceWithBaseURL(srv.URL).InstallFabricLoader("1.99", "0.15.0", t.TempDir(), nil)
	if result.Success || !strings.Contains(result.Error, "not installed") {
		t.Errorf("Success=%v Error=%q, want a 'not installed' failure", result.Success, result.Error)
	}
}

func TestInstallFabricLoaderProfileHTTPError(t *testing.T) {
	srv := fabricMetaFixture(t, http.StatusNotFound, http.StatusOK)
	gameDir := t.TempDir()
	writeVersionJSON(t, gameDir, "1.99", testVersionJSON)

	result := NewBMCLAPIServiceWithBaseURL(srv.URL).InstallFabricLoader("1.99", "0.15.0", gameDir, nil)
	if result.Success || !strings.Contains(result.Error, "404") {
		t.Errorf("Success=%v Error=%q, want failure mentioning 404", result.Success, result.Error)
	}
	if _, err := os.Stat(filepath.Join(gameDir, "versions", "fabric-loader-0.15.0-1.99")); !os.IsNotExist(err) {
		t.Errorf("no version directory may remain after a failed profile fetch, stat err = %v", err)
	}
}

func TestInstallFabricLoaderLibraryFailureCleansUp(t *testing.T) {
	srv := fabricMetaFixture(t, http.StatusOK, http.StatusNotFound)
	gameDir := t.TempDir()
	writeVersionJSON(t, gameDir, "1.99", testVersionJSON)

	result := NewBMCLAPIServiceWithBaseURL(srv.URL).InstallFabricLoader("1.99", "0.15.0", gameDir, nil)
	if result.Success {
		t.Fatal("expected failure when a loader library 404s")
	}
	if _, err := os.Stat(filepath.Join(gameDir, "versions", "fabric-loader-0.15.0-1.99")); !os.IsNotExist(err) {
		t.Errorf("version directory must be removed after a failed install, stat err = %v", err)
	}
}

// ==================== Forge ====================

// fakeForgeInstaller writes a shell script standing in for
// `java -jar installer --installClient` (arguments are ignored). On exit
// code 0 with createVersion it produces the version directory the real
// installer would generate.
func fakeForgeInstaller(t *testing.T, exitCode int, createVersion bool) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skipf("fake installer is a shell script; GOOS=%s", runtime.GOOS)
	}
	script := "#!/bin/sh\necho 'installer: fake work'\necho 'installer: fake stderr' >&2\n"
	if createVersion {
		script += `mkdir -p "versions/1.99-forge-47.2.0"
printf '{"id": "1.99-forge-47.2.0", "inheritsFrom": "1.99"}' > "versions/1.99-forge-47.2.0/1.99-forge-47.2.0.json"
`
	}
	script += fmt.Sprintf("exit %d\n", exitCode)
	path := filepath.Join(t.TempDir(), "fake-java")
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatalf("write fake installer: %v", err)
	}
	return path
}

// forgeInstallerFixture serves the installer jar download.
func forgeInstallerFixture(t *testing.T, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/forge/minecraft/1.99/47.2.0/installer" {
			http.NotFound(w, r)
			return
		}
		if status != http.StatusOK {
			http.Error(w, "gone", status)
			return
		}
		io.WriteString(w, "installer-bytes")
	}))
	t.Cleanup(srv.Close)
	return srv
}

type logLine struct{ source, line string }

// logCollector gathers installer output lines; the stdout/stderr streamers
// invoke the callback concurrently, so access is mutex-guarded.
type logCollector struct {
	mu    sync.Mutex
	lines []logLine
}

func (c *logCollector) add(source, line string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lines = append(c.lines, logLine{source, line})
}

func (c *logCollector) snapshot() []logLine {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]logLine(nil), c.lines...)
}

func TestInstallForgeLoaderHappyPath(t *testing.T) {
	srv := forgeInstallerFixture(t, http.StatusOK)
	gameDir := t.TempDir()

	var logs logCollector
	result := NewBMCLAPIServiceWithBaseURL(srv.URL).InstallForgeLoader("1.99", "47.2.0", gameDir, fakeForgeInstaller(t, 0, true), logs.add, nil)
	if !result.Success {
		t.Fatalf("InstallForgeLoader failed: %s", result.Error)
	}

	data := result.Data.(map[string]interface{})
	if data["version_id"] != "1.99-forge-47.2.0" {
		t.Errorf("version_id = %v", data["version_id"])
	}
	versionJSON := filepath.Join(gameDir, "versions", "1.99-forge-47.2.0", "1.99-forge-47.2.0.json")
	if _, err := os.Stat(versionJSON); err != nil {
		t.Errorf("generated version json missing: %v", err)
	}
	// The installer jar is transient and must be gone afterwards.
	if _, err := os.Stat(filepath.Join(gameDir, "forge-installer-47.2.0.jar")); !os.IsNotExist(err) {
		t.Errorf("installer jar must be removed after the install, stat err = %v", err)
	}
	// Installer output was streamed to the log callback, sources intact.
	var sawStdout, sawStderr bool
	for _, l := range logs.snapshot() {
		if l.source == "stdout" && strings.Contains(l.line, "installer: fake work") {
			sawStdout = true
		}
		if l.source == "stderr" && strings.Contains(l.line, "installer: fake stderr") {
			sawStderr = true
		}
	}
	if !sawStdout || !sawStderr {
		t.Errorf("installer output not fully streamed: %v", logs.snapshot())
	}
}

func TestInstallForgeLoaderInstallerFailure(t *testing.T) {
	srv := forgeInstallerFixture(t, http.StatusOK)
	gameDir := t.TempDir()

	var logs logCollector
	result := NewBMCLAPIServiceWithBaseURL(srv.URL).InstallForgeLoader("1.99", "47.2.0", gameDir, fakeForgeInstaller(t, 1, false), logs.add, nil)
	if result.Success || !strings.Contains(result.Error, "forge installer failed") {
		t.Fatalf("Success=%v Error=%q, want an installer failure", result.Success, result.Error)
	}
	if _, err := os.Stat(filepath.Join(gameDir, "versions", "1.99-forge-47.2.0")); !os.IsNotExist(err) {
		t.Errorf("no version directory may remain after a failed install, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(gameDir, "forge-installer-47.2.0.jar")); !os.IsNotExist(err) {
		t.Errorf("installer jar must be removed after a failed install, stat err = %v", err)
	}
}

// Installer exits 0 but produces no version directory: verification must fail.
func TestInstallForgeLoaderMissingOutput(t *testing.T) {
	srv := forgeInstallerFixture(t, http.StatusOK)
	gameDir := t.TempDir()

	result := NewBMCLAPIServiceWithBaseURL(srv.URL).InstallForgeLoader("1.99", "47.2.0", gameDir, fakeForgeInstaller(t, 0, false), nil, nil)
	if result.Success || !strings.Contains(result.Error, "did not produce") {
		t.Errorf("Success=%v Error=%q, want a verification failure", result.Success, result.Error)
	}
}

func TestInstallForgeLoaderDownloadFailure(t *testing.T) {
	srv := forgeInstallerFixture(t, http.StatusNotFound)
	gameDir := t.TempDir()

	result := NewBMCLAPIServiceWithBaseURL(srv.URL).InstallForgeLoader("1.99", "47.2.0", gameDir, fakeForgeInstaller(t, 0, true), nil, nil)
	if result.Success || !strings.Contains(result.Error, "404") {
		t.Errorf("Success=%v Error=%q, want failure mentioning 404", result.Success, result.Error)
	}
	if _, err := os.Stat(filepath.Join(gameDir, "forge-installer-47.2.0.jar")); !os.IsNotExist(err) {
		t.Errorf("no installer jar may remain after a download failure, stat err = %v", err)
	}
}

func TestInstallForgeLoaderTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("fake installer is a shell script; GOOS=%s", runtime.GOOS)
	}
	srv := forgeInstallerFixture(t, http.StatusOK)
	gameDir := t.TempDir()

	orig := forgeInstallTimeout
	forgeInstallTimeout = 300 * time.Millisecond
	t.Cleanup(func() { forgeInstallTimeout = orig })

	script := "#!/bin/sh\nsleep 5\nexit 0\n"
	javaPath := filepath.Join(t.TempDir(), "slow-java")
	if err := os.WriteFile(javaPath, []byte(script), 0755); err != nil {
		t.Fatalf("write slow fake java: %v", err)
	}

	start := time.Now()
	result := NewBMCLAPIServiceWithBaseURL(srv.URL).InstallForgeLoader("1.99", "47.2.0", gameDir, javaPath, nil, nil)
	if result.Success || !strings.Contains(result.Error, "timed out") {
		t.Errorf("Success=%v Error=%q, want a timeout failure", result.Success, result.Error)
	}
	if time.Since(start) > 10*time.Second {
		t.Error("timeout did not kill the installer promptly")
	}
	if _, err := os.Stat(filepath.Join(gameDir, "forge-installer-47.2.0.jar")); !os.IsNotExist(err) {
		t.Errorf("installer jar must be removed after a timeout, stat err = %v", err)
	}
}
