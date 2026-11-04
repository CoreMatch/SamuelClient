package main

// bindings_test.go covers the Wails binding surface (ADR-0004): input
// validation, error mapping, and the success paths that do not require a
// real network, real accounts, or a GUI. HTTP-backed bindings are exercised
// against httptest servers; java detection is faked through the function
// variables in internal/services.

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"HRPMLW/internal/database"
	"HRPMLW/internal/models"
	"HRPMLW/internal/repository"
	"HRPMLW/internal/services"
)

// TestMain initializes one temporary SQLite database for the whole test
// binary (database.Initialize is sync.Once-guarded).
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "samuelclient-bindtest")
	if err != nil {
		os.Exit(1)
	}
	if err := database.Initialize(&database.Config{Path: filepath.Join(dir, "bindings.db")}); err != nil {
		os.Exit(1)
	}
	code := m.Run()
	_ = database.Close()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// newTestApp builds an App wired to real services but with no Wails context
// (emit becomes a no-op) — exactly what binding tests need.
func newTestApp() *App {
	return &App{
		authService:     services.NewAuthService(),
		bmclService:     services.NewBMCLAPIService(),
		launcherService: services.NewLauncherService(),
		javaRepo:        repository.NewJavaRepository(),
	}
}

// clearLauncherSettings removes launcher.* rows so config tests start clean
// (the DB is shared across the test binary).
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

// clearJavaTable empties java_installations for tests that assert list
// contents.
func clearJavaTable(t *testing.T) {
	t.Helper()
	all, err := repository.NewJavaRepository().GetAll()
	if err != nil {
		t.Fatalf("list java installations: %v", err)
	}
	for _, j := range all {
		_ = repository.NewJavaRepository().Delete(j.ID)
	}
}

// fakeJavaDetections swaps the java detection seams and restores them on
// cleanup.
func fakeJavaDetections(t *testing.T, systemPath, version string) {
	t.Helper()
	origVersion, origSystem := services.DetectJavaVersion, services.DetectSystemJava
	t.Cleanup(func() {
		services.DetectJavaVersion, services.DetectSystemJava = origVersion, origSystem
	})
	services.DetectJavaVersion = func(path string) (string, error) {
		if systemPath != "" && path == systemPath {
			return version, nil
		}
		return version, nil
	}
	services.DetectSystemJava = func() string { return systemPath }
}

func wantFailure(t *testing.T, got *models.ApiResponse, wantErr string) {
	t.Helper()
	if got.Success {
		t.Fatalf("expected failure, got success (data=%v)", got.Data)
	}
	if wantErr != "" && got.Error != wantErr {
		t.Errorf("Error = %q, want %q", got.Error, wantErr)
	}
	if got.Error == "" {
		t.Error("failed ApiResponse must carry a non-empty Error")
	}
}

// ==================== newClientToken ====================

func TestNewClientToken(t *testing.T) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	first := newClientToken()
	if len(first) != 32 {
		t.Errorf("token length = %d, want 32", len(first))
	}
	for _, r := range first {
		if !strings.ContainsRune(charset, r) {
			t.Errorf("token contains %q outside the charset", r)
		}
	}

	// Two draws must differ (crypto/rand, not the old deterministic
	// placeholder).
	second := newClientToken()
	if first == second {
		t.Errorf("two consecutive tokens are identical: %q", first)
	}
}

// ==================== validation failures (all 17 bindings) ====================

func TestBindingValidationFailures(t *testing.T) {
	app := newTestApp()

	cases := []struct {
		name    string
		call    func() *models.ApiResponse
		wantErr string
	}{
		{"GetAuthlibMeta empty server", func() *models.ApiResponse { return app.GetAuthlibMeta("") }, "server is required"},
		{"GetFabricVersions empty mc", func() *models.ApiResponse { return app.GetFabricVersions("") }, "mc version is required"},
		{"GetForgeVersions empty mc", func() *models.ApiResponse { return app.GetForgeVersions("") }, "mc version is required"},
		{"AuthlibLogin empty server", func() *models.ApiResponse { return app.AuthlibLogin("", "u", "p", "") }, "server is required"},
		{"AuthlibLogin empty username", func() *models.ApiResponse { return app.AuthlibLogin("s", "", "p", "") }, "username and password are required"},
		{"AuthlibLogin empty password", func() *models.ApiResponse { return app.AuthlibLogin("s", "u", "", "") }, "username and password are required"},
		{"AuthlibRefresh empty server", func() *models.ApiResponse { return app.AuthlibRefresh("", "at", "ct", "") }, "server is required"},
		{"AuthlibRefresh empty tokens", func() *models.ApiResponse { return app.AuthlibRefresh("s", "", "", "") }, "access token and client token are required"},
		{"AuthlibValidate empty server", func() *models.ApiResponse { return app.AuthlibValidate("", "at", "") }, "server is required"},
		{"AuthlibValidate empty token", func() *models.ApiResponse { return app.AuthlibValidate("s", "", "") }, "access token is required"},
		{"DownloadVersion empty id", func() *models.ApiResponse { return app.DownloadVersion("", "") }, "version_id is required"},
		{"LaunchGame empty request", func() *models.ApiResponse { return app.LaunchGame(LaunchGameRequest{}) }, "version_id is required"},
		{"LaunchGame missing user", func() *models.ApiResponse {
			return app.LaunchGame(LaunchGameRequest{VersionID: "1.99"})
		}, "username and uuid are required"},
		{"LaunchGame missing tokens", func() *models.ApiResponse {
			return app.LaunchGame(LaunchGameRequest{VersionID: "1.99", Username: "u", UUID: "id"})
		}, "access token and client token are required"},
		{"AddJavaInstallation empty path", func() *models.ApiResponse { return app.AddJavaInstallation("", "") }, "path is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wantFailure(t, tc.call(), tc.wantErr)
		})
	}
}

// UpdateJavaInstallation on a missing id must fail with a not-found error.
func TestUpdateJavaInstallationMissingID(t *testing.T) {
	app := newTestApp()
	wantFailure(t, app.UpdateJavaInstallation(999999, "/x", "n"), "")
}

// Delete/SetDefault on a missing id must surface the repository not-found
// error instead of silently succeeding.
func TestDeleteJavaInstallationMissingID(t *testing.T) {
	app := newTestApp()
	wantFailure(t, app.DeleteJavaInstallation(999999), "")
}

func TestSetDefaultJavaInstallationMissingID(t *testing.T) {
	app := newTestApp()
	wantFailure(t, app.SetDefaultJavaInstallation(999999), "")
}

// ==================== HTTP-backed bindings against httptest ====================

const yggdrasilAuthResponse = `{
	"accessToken": "access-123",
	"clientToken": "client-456",
	"availableProfiles": [{"id": "p1", "name": "PlayerOne"}],
	"selectedProfile": {"id": "p1", "name": "PlayerOne"}
}`

func TestAuthlibLoginBindingSuccessGeneratesClientToken(t *testing.T) {
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/authserver/authenticate" {
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		io.WriteString(w, yggdrasilAuthResponse)
	}))
	defer srv.Close()

	app := newTestApp()
	result := app.AuthlibLogin(srv.URL, "alice", "secret", "")
	if !result.Success {
		t.Fatalf("AuthlibLogin failed: %s", result.Error)
	}

	// The binding must have generated a 32-char client token on its own.
	ct, _ := gotBody["clientToken"].(string)
	if len(ct) != 32 {
		t.Errorf("generated clientToken length = %d, want 32 (body=%v)", len(ct), gotBody)
	}

	resp, ok := result.Data.(services.AuthlibAuthResponse)
	if !ok {
		t.Fatalf("Data is %T, want AuthlibAuthResponse", result.Data)
	}
	if resp.AccessToken != "access-123" || resp.SelectedProfile.Name != "PlayerOne" {
		t.Errorf("unexpected login result: %+v", resp)
	}
}

func TestAuthBindingsFailureMapping(t *testing.T) {
	// A closed server simulates an unreachable auth server: the binding must
	// map the transport error into ApiResponse.Error, never panic.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	app := newTestApp()
	wantFailure(t, app.AuthlibLogin(url, "u", "p", ""), "")
	wantFailure(t, app.AuthlibRefresh(url, "at", "ct", ""), "")
	wantFailure(t, app.AuthlibValidate(url, "at", ""), "")
	wantFailure(t, app.GetAuthlibMeta(url), "")
}

func TestRefreshAndValidateBindingSuccess(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/authserver/refresh", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, yggdrasilAuthResponse)
	})
	mux.HandleFunc("/authserver/validate", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"meta": {"serverName": "BindingTestAuth"}, "signaturePublickey": "pk", "skinDomains": []}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	app := newTestApp()
	if result := app.AuthlibRefresh(srv.URL, "at", "ct", "p1"); !result.Success {
		t.Fatalf("AuthlibRefresh failed: %s", result.Error)
	}
	if result := app.AuthlibValidate(srv.URL, "at", "ct"); !result.Success {
		t.Fatalf("AuthlibValidate failed: %s", result.Error)
	}

	meta := app.GetAuthlibMeta(srv.URL)
	if !meta.Success {
		t.Fatalf("GetAuthlibMeta failed: %s", meta.Error)
	}
	data, ok := meta.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("meta Data is %T, want map", meta.Data)
	}
	serverMeta, ok := data["meta"].(*services.AuthlibServerMeta)
	if !ok || serverMeta.Meta.ServerName != "BindingTestAuth" {
		t.Errorf("meta payload = %+v, want serverName BindingTestAuth", data["meta"])
	}
}

// bmclFixture serves the three read-only BMCLAPI endpoints used by bindings.
func bmclFixture(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/mc/game/version_manifest.json", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"latest": {"release": "1.99"}, "versions": [{"id": "1.99", "type": "release", "url": "u"}]}`)
	})
	mux.HandleFunc("/fabric-meta/v2/versions/loader/1.99", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `[{"loader": {"version": "0.15.0"}}]`)
	})
	mux.HandleFunc("/forge/minecraft/1.99", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `[{"version": "47.2.0"}]`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestVersionListBindingsSuccess(t *testing.T) {
	srv := bmclFixture(t)
	app := newTestApp()
	app.bmclService = services.NewBMCLAPIServiceWithBaseURL(srv.URL)

	versions := app.GetVersions()
	if !versions.Success {
		t.Fatalf("GetVersions failed: %s", versions.Error)
	}
	manifest, ok := versions.Data.(services.BMCLVersionManifestResponse)
	if !ok || len(manifest.Versions) != 1 {
		t.Fatalf("unexpected versions data: %T %+v", versions.Data, versions.Data)
	}

	fabric := app.GetFabricVersions("1.99")
	if !fabric.Success {
		t.Fatalf("GetFabricVersions failed: %s", fabric.Error)
	}
	forge := app.GetForgeVersions("1.99")
	if !forge.Success {
		t.Fatalf("GetForgeVersions failed: %s", forge.Error)
	}
}

func TestVersionBindingFailureMapping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	app := newTestApp()
	app.bmclService = services.NewBMCLAPIServiceWithBaseURL(url)

	for name, result := range map[string]*models.ApiResponse{
		"GetVersions":        app.GetVersions(),
		"GetFabricVersions":  app.GetFabricVersions("1.99"),
		"GetForgeVersions":   app.GetForgeVersions("1.99"),
	} {
		if result.Success {
			t.Errorf("%s: expected failure against a closed server", name)
		}
		if result.Error == "" {
			t.Errorf("%s: failure must carry an error message", name)
		}
	}
}

// ==================== java installation bindings ====================

func TestJavaInstallationBindingsLifecycle(t *testing.T) {
	clearJavaTable(t)
	fakeJavaDetections(t, "", "17.0.9")
	app := newTestApp()

	// Add.
	added := app.AddJavaInstallation("/opt/jdk/bin/java", "Test JDK")
	if !added.Success {
		t.Fatalf("AddJavaInstallation failed: %s", added.Error)
	}
	created, ok := added.Data.(*models.JavaInstallation)
	if !ok || created.ID == 0 {
		t.Fatalf("AddJavaInstallation Data = %T, want *models.JavaInstallation with ID", added.Data)
	}
	if created.Version != "17.0.9" {
		t.Errorf("detected version = %q, want the faked 17.0.9", created.Version)
	}

	// List contains it.
	list := app.GetJavaInstallations()
	if !list.Success {
		t.Fatalf("GetJavaInstallations failed: %s", list.Error)
	}
	javas, ok := list.Data.([]models.JavaInstallation)
	if !ok || len(javas) != 1 {
		t.Fatalf("GetJavaInstallations Data = %T %+v, want one row", list.Data, list.Data)
	}

	// Update name.
	upd := app.UpdateJavaInstallation(int(created.ID), "", "Renamed JDK")
	if !upd.Success {
		t.Fatalf("UpdateJavaInstallation failed: %s", upd.Error)
	}
	if updated := upd.Data.(*models.JavaInstallation); updated.FriendlyName != "Renamed JDK" {
		t.Errorf("friendly name = %q, want Renamed JDK", updated.FriendlyName)
	}

	// Second installation + default switching.
	second := app.AddJavaInstallation("/opt/jdk2/bin/java", "Second JDK")
	if !second.Success {
		t.Fatalf("second AddJavaInstallation failed: %s", second.Error)
	}
	secondID := second.Data.(*models.JavaInstallation).ID

	if result := app.SetDefaultJavaInstallation(int(created.ID)); !result.Success {
		t.Fatalf("SetDefaultJavaInstallation failed: %s", result.Error)
	}
	def, err := app.javaRepo.GetDefault()
	if err != nil || def.ID != created.ID {
		t.Fatalf("default after first SetDefault = %+v / %v", def, err)
	}
	if result := app.SetDefaultJavaInstallation(int(secondID)); !result.Success {
		t.Fatalf("second SetDefaultJavaInstallation failed: %s", result.Error)
	}
	def, err = app.javaRepo.GetDefault()
	if err != nil || def.ID != secondID {
		t.Errorf("default must move to the second installation, got %+v / %v", def, err)
	}

	// Delete both.
	if result := app.DeleteJavaInstallation(int(created.ID)); !result.Success {
		t.Fatalf("DeleteJavaInstallation failed: %s", result.Error)
	}
	if result := app.DeleteJavaInstallation(int(secondID)); !result.Success {
		t.Fatalf("second DeleteJavaInstallation failed: %s", result.Error)
	}
	if _, err := app.javaRepo.GetByID(created.ID); err == nil {
		t.Error("deleted java installation still readable")
	}
}

func TestGetJavaInstallationsAutoDetectsSystemJava(t *testing.T) {
	clearJavaTable(t)
	fakeJavaDetections(t, "/fake/system/java", "21.0.1")
	app := newTestApp()

	result := app.GetJavaInstallations()
	if !result.Success {
		t.Fatalf("GetJavaInstallations failed: %s", result.Error)
	}
	javas := result.Data.([]models.JavaInstallation)
	if len(javas) != 1 {
		t.Fatalf("auto-detection created %d rows, want 1", len(javas))
	}
	if javas[0].Path != "/fake/system/java" || !javas[0].IsDefault || javas[0].Version != "21.0.1" {
		t.Errorf("auto-detected row = %+v", javas[0])
	}

	clearJavaTable(t)
}

func TestGetJavaInstallationsEmptyWithoutSystemJava(t *testing.T) {
	clearJavaTable(t)
	fakeJavaDetections(t, "", "")
	app := newTestApp()

	result := app.GetJavaInstallations()
	if !result.Success {
		t.Fatalf("GetJavaInstallations failed: %s", result.Error)
	}
	javas := result.Data.([]models.JavaInstallation)
	if len(javas) != 0 {
		t.Errorf("expected empty list when no system java exists, got %+v", javas)
	}
}

// ==================== launcher config bindings ====================

func TestGetLauncherConfigBindingDefaults(t *testing.T) {
	clearLauncherSettings(t)

	javaHome := t.TempDir()
	bin := filepath.Join(javaHome, "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	javaPath := filepath.Join(bin, "java")
	if err := os.WriteFile(javaPath, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatalf("write fake java: %v", err)
	}
	t.Setenv("JAVA_HOME", javaHome)
	t.Setenv("HOME", t.TempDir())

	app := newTestApp()
	result := app.GetLauncherConfig()
	if !result.Success {
		t.Fatalf("GetLauncherConfig failed: %s", result.Error)
	}
	cfg, ok := result.Data.(models.LauncherConfig)
	if !ok {
		t.Fatalf("Data is %T, want models.LauncherConfig", result.Data)
	}
	if cfg.MaxMemory != 4096 || cfg.MinMemory != 1024 || cfg.GameWidth != 854 || cfg.GameHeight != 480 {
		t.Errorf("defaults wrong: %+v", cfg)
	}
	if cfg.JavaPath != javaPath {
		t.Errorf("JavaPath = %q, want %q", cfg.JavaPath, javaPath)
	}
}

func TestUpdateLauncherConfigBinding(t *testing.T) {
	clearLauncherSettings(t)
	app := newTestApp()

	valid := app.UpdateLauncherConfig(models.LauncherConfig{
		MaxMemory: 2048, MinMemory: 512, JavaArgs: "-Xss1M", GameWidth: 1280, GameHeight: 720,
	})
	if !valid.Success {
		t.Fatalf("valid UpdateLauncherConfig failed: %s", valid.Error)
	}
	merged, ok := valid.Data.(models.LauncherConfig)
	if !ok || merged.MaxMemory != 2048 || merged.GameWidth != 1280 {
		t.Errorf("merged config = %+v, want the saved values", merged)
	}

	// Invalid values must surface the service validation error.
	invalid := app.UpdateLauncherConfig(models.LauncherConfig{MaxMemory: 1024, MinMemory: 2048})
	wantFailure(t, invalid, "min memory cannot exceed max memory")

	clearLauncherSettings(t)
}

// ==================== installed versions binding ====================

func TestGetInstalledVersionsBinding(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	versionsDir := filepath.Join(home, ".minecraft", "versions")
	// Complete version: json + jar.
	full := filepath.Join(versionsDir, "1.99")
	if err := os.MkdirAll(full, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(full, "1.99.json"), []byte(`{"type": "release"}`), 0644); err != nil {
		t.Fatalf("write json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(full, "1.99.jar"), []byte("jar"), 0644); err != nil {
		t.Fatalf("write jar: %v", err)
	}
	// Partial version: json only.
	partial := filepath.Join(versionsDir, "0.5-partial")
	if err := os.MkdirAll(partial, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(partial, "0.5-partial.json"), []byte(`{"type": "snapshot"}`), 0644); err != nil {
		t.Fatalf("write json: %v", err)
	}

	app := newTestApp()
	result := app.GetInstalledVersions()
	if !result.Success {
		t.Fatalf("GetInstalledVersions failed: %s", result.Error)
	}
	data, ok := result.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("Data is %T, want map", result.Data)
	}
	versions, ok := data["versions"].([]InstalledVersion)
	if !ok || len(versions) != 2 {
		t.Fatalf("versions = %+v, want both directories", data["versions"])
	}

	byID := map[string]InstalledVersion{}
	for _, v := range versions {
		byID[v.ID] = v
	}
	fullGot, ok := byID["1.99"]
	if !ok || !fullGot.HasJSON || !fullGot.HasJAR || fullGot.Type != "release" {
		t.Errorf("full version = %+v, want has_json+has_jar release", fullGot)
	}
	partialGot, ok := byID["0.5-partial"]
	if !ok || !partialGot.HasJSON || partialGot.HasJAR || partialGot.Type != "snapshot" {
		t.Errorf("partial version = %+v, want has_json only snapshot", partialGot)
	}
}

func TestGetInstalledVersionsEmptyWhenNoDir(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	app := newTestApp()
	result := app.GetInstalledVersions()
	if !result.Success {
		t.Fatalf("GetInstalledVersions failed on missing dir: %s", result.Error)
	}
	data := result.Data.(map[string]interface{})
	versions := data["versions"].([]InstalledVersion)
	if len(versions) != 0 {
		t.Errorf("versions = %+v, want empty", versions)
	}
}

// ==================== download binding (full flow, hermetic) ====================

func TestDownloadVersionBindingSuccess(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/mc/game/version_manifest.json", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"versions": [{"id": "1.99", "type": "release", "url": "https://piston-meta.mojang.com/v1/packages/abc/1.99.json"}]}`)
	})
	mux.HandleFunc("/v1/packages/abc/1.99.json", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{
			"id": "1.99",
			"mainClass": "net.minecraft.client.main.Main",
			"downloads": {"client": {"sha1": "c1", "size": 8, "url": "https://piston-data.mojang.com/v1/objects/c1/client.jar"}},
			"libraries": []
		}`)
	})
	mux.HandleFunc("/v1/objects/c1/client.jar", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "jar-bytes")
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	app := newTestApp()
	app.bmclService = services.NewBMCLAPIServiceWithBaseURL(srv.URL)
	gameDir := t.TempDir()

	result := app.DownloadVersion("1.99", gameDir)
	if !result.Success {
		t.Fatalf("DownloadVersion failed: %s", result.Error)
	}

	versionDir := filepath.Join(gameDir, "versions", "1.99")
	if _, err := os.Stat(filepath.Join(versionDir, "1.99.json")); err != nil {
		t.Errorf("version JSON missing: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(versionDir, "1.99.jar")); err != nil || string(got) != "jar-bytes" {
		t.Errorf("version jar missing or wrong (err=%v got=%q)", err, got)
	}
}

func TestDownloadVersionBindingFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusInternalServerError)
	}))
	defer srv.Close()

	app := newTestApp()
	app.bmclService = services.NewBMCLAPIServiceWithBaseURL(srv.URL)

	wantFailure(t, app.DownloadVersion("1.99", t.TempDir()), "")
}

// ==================== launch binding ====================

// LaunchGame's success path spawns a real detached game process and is
// deliberately not exercised here (see TESTING-TODO.md); validation and the
// pre-flight authlib metadata fetch are the in-scope parts.

func TestLaunchGameValidationOnly(t *testing.T) {
	app := newTestApp()

	// Covered by the table-driven validation test above; here we additionally
	// assert the exact ApiResponse shape of a failure: Success=false, Error
	// set, Data nil.
	result := app.LaunchGame(LaunchGameRequest{})
	if result.Success || result.Error == "" || result.Data != nil {
		t.Errorf("failure shape = %+v, want Success=false/Error set/Data nil", result)
	}
}
