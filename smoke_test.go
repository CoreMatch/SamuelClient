package main

// smoke_test.go is the end-to-end smoke test requested in TESTING-TODO #1:
// it drives the three user-visible steps — login → version install → launch —
// through the real Wails binding methods against in-process fake servers.
// No real network, real accounts, or GUI are involved: the Yggdrasil server
// and the BMCLAPI mirror are httptest servers, and the "game" is a shell
// script standing in for java.

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"HRPMLW/internal/services"
)

const smokeVersionID = "1.99-smoke"

// smokeAssetHash is the 32-hex-char object hash served by fakeBMCLAPI;
// assets land under assets/objects/<first two chars>/<hash>.
const smokeAssetHash = "cdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcd"

// fakeYggdrasil serves the two endpoints the login flow touches:
// GET / (metadata, also prefetched before launch) and POST
// /authserver/authenticate.
func fakeYggdrasil(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/":
			io.WriteString(w, `{"meta": {"serverName": "SmokeAuth"}, "signaturePublickey": "k", "skinDomains": []}`)
		case "/authserver/authenticate":
			io.WriteString(w, `{
				"accessToken": "smoke-access",
				"clientToken": "smoke-client",
				"availableProfiles": [{"id": "p-smoke", "name": "SmokePlayer"}],
				"selectedProfile": {"id": "p-smoke", "name": "SmokePlayer"}
			}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// fakeBMCLAPI serves everything InstallVersion needs for smokeVersionID:
// version manifest, version JSON, client jar, one library, the asset index
// and one asset object. No natives library, so the smoke test stays
// OS-independent.
func fakeBMCLAPI(t *testing.T) *httptest.Server {
	t.Helper()

	versionJSON := fmt.Sprintf(`{
		"id": %q,
		"mainClass": "net.minecraft.client.main.Main",
		"assetIndex": {"id": "smoke", "url": "https://piston-meta.mojang.com/v1/assets/smoke.json", "sha1": "x", "size": 1},
		"assets": "smoke",
		"downloads": {
			"client": {"sha1": "c1", "size": 9, "url": "https://piston-data.mojang.com/v1/objects/c1/client.jar"}
		},
		"libraries": [
			{
				"name": "com.example:lib:1.0",
				"downloads": {"artifact": {"path": "com/example/lib/1.0/lib-1.0.jar", "url": "https://libraries.minecraft.net/com/example/lib/1.0/lib-1.0.jar", "sha1": "l1", "size": 3}}
			}
		],
		"type": "release"
	}`, smokeVersionID)

	manifest := fmt.Sprintf(`{
		"latest": {"release": %q},
		"versions": [{"id": %q, "type": "release", "url": "https://piston-meta.mojang.com/v1/packages/smoke/%s.json"}]
	}`, smokeVersionID, smokeVersionID, smokeVersionID)

	mux := http.NewServeMux()
	mux.HandleFunc("/mc/game/version_manifest.json", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, manifest)
	})
	mux.HandleFunc("/v1/packages/smoke/"+smokeVersionID+".json", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, versionJSON)
	})
	mux.HandleFunc("/v1/objects/c1/client.jar", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "jar-bytes")
	})
	mux.HandleFunc("/maven/com/example/lib/1.0/lib-1.0.jar", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "lib")
	})
	mux.HandleFunc("/v1/assets/smoke.json", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"objects": {"minecraft/sounds/smoke.ogg": {"hash": %q, "size": 5}}}`, smokeAssetHash)
	})
	mux.HandleFunc("/assets/cd/"+smokeAssetHash, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "asset")
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// fakeJava writes a shell script that stands in for the java binary: it
// prints one stdout and one stderr line, then exits.
func fakeJava(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skipf("fake java is a shell script; GOOS=%s", runtime.GOOS)
	}
	script := "#!/bin/sh\necho 'smoke-boot-line'\necho 'smoke-stderr-line' >&2\nexit 0\n"
	path := filepath.Join(t.TempDir(), "java")
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatalf("write fake java: %v", err)
	}
	return path
}

// TestSmokeLoginInstallLaunch drives the full user flow through the binding
// surface: AuthlibLogin → DownloadVersion → LaunchGame.
func TestSmokeLoginInstallLaunch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("smoke test uses a shell-script java; GOOS=%s", runtime.GOOS)
	}

	authSrv := fakeYggdrasil(t)
	bmclSrv := fakeBMCLAPI(t)

	app := newTestApp()
	app.bmclService = services.NewBMCLAPIServiceWithBaseURL(bmclSrv.URL)

	// 1. Login against the fake Yggdrasil server.
	login := app.AuthlibLogin(authSrv.URL, "smoke-user", "smoke-password", "")
	if !login.Success {
		t.Fatalf("AuthlibLogin failed: %s", login.Error)
	}

	// 2. Install the version into a scratch game directory.
	gameDir := t.TempDir()
	install := app.DownloadVersion(smokeVersionID, gameDir)
	if !install.Success {
		t.Fatalf("DownloadVersion failed: %s", install.Error)
	}
	versionDir := filepath.Join(gameDir, "versions", smokeVersionID)
	for _, p := range []string{
		filepath.Join(versionDir, smokeVersionID+".json"),
		filepath.Join(versionDir, smokeVersionID+".jar"),
		filepath.Join(gameDir, "libraries", "com", "example", "lib", "1.0", "lib-1.0.jar"),
		filepath.Join(gameDir, "assets", "objects", "cd", smokeAssetHash),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("installed artifact missing %q: %v", p, err)
		}
	}

	// 3. Launch with the fake java. The authlib-injector auth type also
	//    exercises the pre-launch metadata prefetch against fakeYggdrasil.
	launch := app.LaunchGame(LaunchGameRequest{
		VersionID:   smokeVersionID,
		VersionType: "release",
		Username:    "SmokePlayer",
		UUID:        "p-smoke",
		AccessToken: "smoke-access",
		ClientToken: "smoke-client",
		AuthType:    "authlib-injector",
		AuthServer:  authSrv.URL,
		GameDir:     gameDir,
		JavaPath:    fakeJava(t),
	})
	if !launch.Success {
		t.Fatalf("LaunchGame failed: %s", launch.Error)
	}
	data, ok := launch.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("LaunchGame Data is %T, want map", launch.Data)
	}
	pid, _ := data["pid"].(int)
	if pid <= 0 {
		t.Errorf("pid = %v, want a positive process id", data["pid"])
	}
	logPath, _ := data["log_path"].(string)
	if logPath == "" {
		t.Fatal("log_path missing from LaunchGame payload")
	}

	// 4. The detached fake-java process must run to completion; its output is
	//    streamed into the log file (the same path the launcher:log events
	//    mirror). Poll because the process runs asynchronously.
	deadline := time.Now().Add(10 * time.Second)
	var content string
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(logPath); err == nil {
			content = string(b)
			if strings.Contains(content, "smoke-boot-line") &&
				strings.Contains(content, "smoke-stderr-line") &&
				strings.Contains(content, "# Process exited") {
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !strings.Contains(content, "smoke-boot-line") || !strings.Contains(content, "smoke-stderr-line") {
		t.Errorf("game output missing from launch log:\n%s", content)
	}
	if !strings.Contains(content, "# Process exited") {
		t.Errorf("fake game process did not exit cleanly:\n%s", content)
	}
	// The recorded launch command must carry the launch arguments (values may
	// be single-quoted by shellQuote).
	if !strings.Contains(content, "--username") || !strings.Contains(content, "SmokePlayer") ||
		!strings.Contains(content, "--accessToken") || !strings.Contains(content, "smoke-access") {
		t.Errorf("launch command in log lacks game args:\n%s", content)
	}
}
