package main

// bindings.go is the Wails binding surface of SamuelClient (ADR-0004).
//
// Every method on App that the frontend may call lives here. The frontend
// reaches these methods exclusively through the generated Wails bindings
// (frontend/wailsjs); there is no local HTTP listener involved. All methods
// return *models.ApiResponse so the frontend keeps one uniform response
// shape: { success, data, error }.
//
// Streaming (live launch logs, download progress) is pushed from Go to the
// frontend via Wails runtime events instead of HTTP polling. The event
// contract (ADR-0004):
//
//   - "launcher:log" — one event per output line of the launched game
//     process, or of the Forge installer while InstallForgeLoader runs.
//     Payload: {"source": "stdout"|"stderr", "line": string}.
//     For the game process, emission starts after LaunchGame returns success
//     and ends when the process exits.
//   - "download:progress" — progress while DownloadVersion installs a
//     version. Payload: {"version_id": string, "stage": "jar"|"library"|
//     "native"|"asset"|"loader", "name": string, "downloaded": number,
//     "total": number}. The "loader" stage is emitted by
//     InstallFabricLoader/InstallForgeLoader.
//   - "interop:handoff" — match handoff lifecycle (INTEROP-SPEC.md).
//     Payload: {"status": "pending"|"dismissed"|"gone"|"invalid"|
//     "version_mismatch"|"pull_timeout", "handoff": <payload when pending>,
//     "message": <readable text for degraded statuses>, ...}. Handoff
//     payloads never carry credentials; the receiver uses its own account.
//
// The match handoff arrives over a separate loopback-only, token-protected
// inter-process listener (INTEROP-SPEC.md) — not through the frontend. The
// frontend only sees the events and bindings below.

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"HRPMLW/internal/interop"
	"HRPMLW/internal/models"
	"HRPMLW/internal/services"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// emit sends a Wails runtime event to the frontend. Safe to call from any
// goroutine; a no-op before startup has provided the context.
func (a *App) emit(event string, payload interface{}) {
	if a.ctx == nil {
		return
	}
	wailsruntime.EventsEmit(a.ctx, event, payload)
}

// ==================== Auth (read) ====================

// GetAuthlibMeta fetches the authlib-injector metadata document of the given
// auth server. Used by the login view to validate/preview a custom server.
func (a *App) GetAuthlibMeta(server string) *models.ApiResponse {
	if server == "" {
		return &models.ApiResponse{Success: false, Error: "server is required"}
	}
	return a.authService.GetAuthlibMeta(server)
}

// ==================== Versions & loaders (read) ====================

// GetVersions returns the BMCLAPI version manifest list.
func (a *App) GetVersions() *models.ApiResponse {
	return a.bmclService.GetVersionList()
}

// GetFabricVersions returns available Fabric loader versions for a Minecraft
// version.
func (a *App) GetFabricVersions(mcVersion string) *models.ApiResponse {
	if mcVersion == "" {
		return &models.ApiResponse{Success: false, Error: "mc version is required"}
	}
	return a.bmclService.GetFabricVersions(mcVersion)
}

// GetForgeVersions returns available Forge loader versions for a Minecraft
// version.
func (a *App) GetForgeVersions(mcVersion string) *models.ApiResponse {
	if mcVersion == "" {
		return &models.ApiResponse{Success: false, Error: "mc version is required"}
	}
	return a.bmclService.GetForgeVersions(mcVersion)
}

// ==================== Java installations (read) ====================

// GetJavaInstallations lists all known Java installations. On first use (no
// rows yet) it auto-detects the system Java and stores it as the default,
// mirroring the previous GET /api/v1/db/java behaviour.
func (a *App) GetJavaInstallations() *models.ApiResponse {
	javas, err := a.javaRepo.GetAll()
	if err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}

	if len(javas) == 0 {
		if systemJavaPath := services.DetectSystemJava(); systemJavaPath != "" {
			version, _ := services.DetectJavaVersion(systemJavaPath)
			defaultJava := &models.JavaInstallation{
				Path:         systemJavaPath,
				FriendlyName: "System Default Java",
				Version:      version,
				IsDefault:    true,
			}
			if err := a.javaRepo.Create(defaultJava); err == nil {
				javas = []models.JavaInstallation{*defaultJava}
			}
		}
	}

	return &models.ApiResponse{Success: true, Data: javas}
}

// ==================== Launcher config & local state (read) ====================

// GetLauncherConfig returns the merged launcher configuration (persisted
// user preferences plus auto-detected Java path and game directory).
func (a *App) GetLauncherConfig() *models.ApiResponse {
	return &models.ApiResponse{Success: true, Data: a.launcherService.GetLauncherConfig()}
}

// ==================== Match handoff (INTEROP-SPEC) ====================

// GetPendingHandoff returns the currently pending match handoff, if any.
// Data is {"status": "idle"|"pending", "handoff": <payload|nil>}. The
// payload never contains credentials (protocol-enforced).
func (a *App) GetPendingHandoff() *models.ApiResponse {
	a.handoffMu.Lock()
	pending := a.pendingHandoff
	a.handoffMu.Unlock()

	if pending == nil {
		return &models.ApiResponse{Success: true, Data: map[string]interface{}{
			"status":  "idle",
			"handoff": nil,
		}}
	}
	return &models.ApiResponse{Success: true, Data: map[string]interface{}{
		"status":  "pending",
		"handoff": pending,
	}}
}

// DismissHandoff drops the pending handoff (user declined or acknowledged a
// failure). The frontend is notified via an interop:handoff event with
// status "dismissed".
func (a *App) DismissHandoff() *models.ApiResponse {
	a.handoffMu.Lock()
	a.pendingHandoff = nil
	a.handoffMu.Unlock()

	a.emit("interop:handoff", map[string]interface{}{"status": "dismissed"})
	return &models.ApiResponse{Success: true}
}

// GetInteropStatus reports the handoff listener state for UI display.
func (a *App) GetInteropStatus() *models.ApiResponse {
	running := a.interopServer != nil
	port := 0
	if running {
		port = a.interopServer.Port()
	}
	return &models.ApiResponse{Success: true, Data: map[string]interface{}{
		"running":          running,
		"port":             port,
		"protocol_version": interop.ProtocolVersion,
	}}
}

// InstalledVersion describes a version present on disk under
// <gameDir>/versions.
type InstalledVersion struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	HasJSON bool   `json:"has_json"`
	HasJAR  bool   `json:"has_jar"`
}

// GetInstalledVersions scans <gameDir>/versions and reports every directory
// that contains a version JSON, so the UI can populate its version dropdown
// from disk.
func (a *App) GetInstalledVersions() *models.ApiResponse {
	gameDir := a.launcherService.GetMinecraftDir()
	versionsDir := filepath.Join(gameDir, "versions")

	entries, err := os.ReadDir(versionsDir)
	if err != nil {
		return &models.ApiResponse{
			Success: true,
			Data:    map[string]interface{}{"versions": []InstalledVersion{}},
		}
	}

	out := []InstalledVersion{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		id := e.Name()
		dir := filepath.Join(versionsDir, id)
		jsonPath := filepath.Join(dir, id+".json")
		jarPath := filepath.Join(dir, id+".jar")
		hasJSON := false
		hasJAR := false
		if fi, err := os.Stat(jsonPath); err == nil && !fi.IsDir() {
			hasJSON = true
		}
		if fi, err := os.Stat(jarPath); err == nil && !fi.IsDir() {
			hasJAR = true
		}
		vType := "release"
		inheritsFrom := ""
		if hasJSON {
			if data, err := os.ReadFile(jsonPath); err == nil {
				var meta struct {
					Type         string `json:"type"`
					InheritsFrom string `json:"inheritsFrom"`
				}
				if json.Unmarshal(data, &meta) == nil {
					if meta.Type != "" {
						vType = meta.Type
					}
					inheritsFrom = meta.InheritsFrom
				}
			}
		}
		// Loader versions (Fabric/Forge) inherit the parent's client jar, so
		// they count as installed when the parent jar exists.
		if !hasJAR && inheritsFrom != "" {
			parentJar := filepath.Join(versionsDir, inheritsFrom, inheritsFrom+".jar")
			if fi, err := os.Stat(parentJar); err == nil && !fi.IsDir() {
				hasJAR = true
			}
		}
		out = append(out, InstalledVersion{ID: id, Type: vType, HasJSON: hasJSON, HasJAR: hasJAR})
	}

	return &models.ApiResponse{
		Success: true,
		Data:    map[string]interface{}{"versions": out},
	}
}

// ==================== Auth (write / sensitive) ====================
//
// These carry credentials or session tokens. They run in-process via the
// Wails binding and never traverse a local HTTP socket (ADR-0004). None of
// the values are written to logs.

// AuthlibLogin authenticates against an authlib-injector (Yggdrasil) server.
// The plaintext password is passed in-process to the auth service only.
func (a *App) AuthlibLogin(server, username, password, clientToken string) *models.ApiResponse {
	if server == "" {
		return &models.ApiResponse{Success: false, Error: "server is required"}
	}
	if username == "" || password == "" {
		return &models.ApiResponse{Success: false, Error: "username and password are required"}
	}
	if clientToken == "" {
		clientToken = newClientToken()
	}
	return a.authService.AuthenticateWithAuthlib(server, username, password, clientToken)
}

// AuthlibRefresh refreshes an access token, optionally binding it to a
// specific profile.
func (a *App) AuthlibRefresh(server, accessToken, clientToken, selectedProfileID string) *models.ApiResponse {
	if server == "" {
		return &models.ApiResponse{Success: false, Error: "server is required"}
	}
	if accessToken == "" || clientToken == "" {
		return &models.ApiResponse{Success: false, Error: "access token and client token are required"}
	}
	return a.authService.RefreshAuthlibTokenWithProfile(server, accessToken, clientToken, selectedProfileID)
}

// AuthlibValidate checks whether a stored access token is still valid.
func (a *App) AuthlibValidate(server, accessToken, clientToken string) *models.ApiResponse {
	if server == "" {
		return &models.ApiResponse{Success: false, Error: "server is required"}
	}
	if accessToken == "" {
		return &models.ApiResponse{Success: false, Error: "access token is required"}
	}
	return a.authService.ValidateAuthlibToken(server, accessToken, clientToken)
}

// newClientToken generates a random 32-character client token.
func newClientToken() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		// rand.Read on the system CSPRNG effectively never fails; fall back to
		// a deterministic value rather than aborting the login.
		return strings.Repeat("a", 32)
	}
	for i := range buf {
		buf[i] = charset[int(buf[i])%len(charset)]
	}
	return string(buf)
}

// ==================== Download (write, streaming) ====================

// DownloadVersion installs a Minecraft version (JSON + JAR + libraries +
// natives). Progress is pushed to the frontend as "download:progress" events
// (see the event contract at the top of this file); the method itself blocks
// until the install finishes and returns the final result.
func (a *App) DownloadVersion(versionID, gameDir string) *models.ApiResponse {
	if versionID == "" {
		return &models.ApiResponse{Success: false, Error: "version_id is required"}
	}
	if gameDir == "" {
		gameDir = a.launcherService.GetMinecraftDir()
	}

	progress := func(stage, name string, downloaded, total int64) {
		a.emit("download:progress", map[string]interface{}{
			"version_id": versionID,
			"stage":      stage,
			"name":       name,
			"downloaded": downloaded,
			"total":      total,
		})
	}

	return a.bmclService.InstallVersion(versionID, gameDir, progress)
}

// InstallFabricLoader installs the Fabric loader profile on top of an
// already installed vanilla version. The resulting version inherits the
// parent's client jar (inheritsFrom) and becomes selectable through
// GetInstalledVersions/LaunchGame. Progress is emitted as
// "download:progress" events with stage "loader".
func (a *App) InstallFabricLoader(mcVersion, loaderVersion, gameDir string) *models.ApiResponse {
	if mcVersion == "" {
		return &models.ApiResponse{Success: false, Error: "mc version is required"}
	}
	if loaderVersion == "" {
		return &models.ApiResponse{Success: false, Error: "loader version is required"}
	}
	if gameDir == "" {
		gameDir = a.launcherService.GetMinecraftDir()
	}

	versionID := fmt.Sprintf("fabric-loader-%s-%s", loaderVersion, mcVersion)
	progress := func(stage, name string, downloaded, total int64) {
		a.emit("download:progress", map[string]interface{}{
			"version_id": versionID,
			"stage":      stage,
			"name":       name,
			"downloaded": downloaded,
			"total":      total,
		})
	}

	return a.bmclService.InstallFabricLoader(mcVersion, loaderVersion, gameDir, progress)
}

// InstallForgeLoader downloads the Forge installer and runs it headless
// (`java -jar <installer> --installClient`) with a timeout. The installer's
// output is streamed as "launcher:log" events; progress as
// "download:progress" with stage "loader".
func (a *App) InstallForgeLoader(mcVersion, forgeVersion, gameDir string) *models.ApiResponse {
	if mcVersion == "" {
		return &models.ApiResponse{Success: false, Error: "mc version is required"}
	}
	if forgeVersion == "" {
		return &models.ApiResponse{Success: false, Error: "forge version is required"}
	}
	if gameDir == "" {
		gameDir = a.launcherService.GetMinecraftDir()
	}

	versionID := mcVersion + "-forge-" + forgeVersion
	progress := func(stage, name string, downloaded, total int64) {
		a.emit("download:progress", map[string]interface{}{
			"version_id": versionID,
			"stage":      stage,
			"name":       name,
			"downloaded": downloaded,
			"total":      total,
		})
	}
	logCallback := func(source, line string) {
		a.emit("launcher:log", map[string]interface{}{
			"source": source,
			"line":   line,
		})
	}

	javaPath := a.launcherService.GetJavaPath()
	return a.bmclService.InstallForgeLoader(mcVersion, forgeVersion, gameDir, javaPath, logCallback, progress)
}

// ==================== Launch (write, streaming) ====================

// LaunchGameRequest mirrors the previous POST /api/v1/launcher/launch body so
// the frontend can keep the same field names.
type LaunchGameRequest struct {
	VersionID     string `json:"version_id"`
	VersionType   string `json:"version_type"`
	Username      string `json:"username"`
	UUID          string `json:"uuid"`
	AccessToken   string `json:"access_token"`
	ClientToken   string `json:"client_token"`
	AuthType      string `json:"auth_type"`
	AuthServer    string `json:"auth_server"`
	AuthlibJar    string `json:"authlib_jar"`
	AuthlibMeta   string `json:"authlib_meta"`
	GameDir       string `json:"game_dir"`
	JavaPath      string `json:"java_path"`
	MaxMemory     int    `json:"max_memory"`
	MinMemory     int    `json:"min_memory"`
	JavaArgs      string `json:"java_args"`
	GameWidth     int    `json:"game_width"`
	GameHeight    int    `json:"game_height"`
	LoaderType    string `json:"loader_type"`
	LoaderVersion string `json:"loader_version"`

	// JoinServer/JoinPort implement the match handoff (INTEROP-SPEC §6):
	// when set, the game launches directly into the match server. Validated
	// and reachability-checked before launch; never persisted.
	JoinServer string `json:"join_server"`
	JoinPort   int    `json:"join_port"`
}

// LaunchGame launches the game for the selected version and account. The
// access/client tokens stay in-process. Live game output is pushed to the
// frontend as "launcher:log" events (see the event contract at the top of
// this file); the method returns as soon as the process has started.
func (a *App) LaunchGame(req LaunchGameRequest) *models.ApiResponse {
	if req.VersionID == "" {
		return &models.ApiResponse{Success: false, Error: "version_id is required"}
	}
	if req.Username == "" || req.UUID == "" {
		return &models.ApiResponse{Success: false, Error: "username and uuid are required"}
	}
	if req.AccessToken == "" || req.ClientToken == "" {
		return &models.ApiResponse{Success: false, Error: "access token and client token are required"}
	}

	gameDir := req.GameDir
	if gameDir == "" {
		gameDir = a.launcherService.GetMinecraftDir()
	}

	// Match handoff preflight (INTEROP-SPEC §6): validate the join target
	// grammar and dial it before spawning a doomed game process, so an
	// unreachable match server degrades readably instead of wasting the
	// matchmaking result.
	if req.JoinServer != "" {
		jt := interop.JoinTarget{Host: req.JoinServer, Port: req.JoinPort}
		if err := jt.Validate(); err != nil {
			return &models.ApiResponse{Success: false, Error: "invalid join target: " + err.Error()}
		}
		if err := interop.CheckReachable(jt.Host, jt.Port, interop.ReachabilityTimeout); err != nil {
			return &models.ApiResponse{
				Success: false,
				Error:   fmt.Sprintf("match server %s:%d is not reachable right now: %v", jt.Host, jt.Port, err),
			}
		}
	}

	// 配置预获取: if the caller didn't supply the metadata JSON, fetch it from
	// the authlib server (required pre-launch step per the authlib-injector
	// wiki). On failure we still launch; authlib-injector can fall back to a
	// runtime fetch.
	authlibMeta := req.AuthlibMeta
	if authlibMeta == "" && strings.EqualFold(req.AuthType, "authlib-injector") && req.AuthServer != "" {
		metaResult := a.authService.GetAuthlibMeta(req.AuthServer)
		if metaResult.Success {
			if data, ok := metaResult.Data.(map[string]interface{}); ok {
				if raw, ok := data["raw"].(string); ok {
					authlibMeta = raw
				}
			}
		}
	}

	config := models.LauncherConfig{
		JavaPath:        req.JavaPath,
		MaxMemory:       req.MaxMemory,
		MinMemory:       req.MinMemory,
		JavaArgs:        req.JavaArgs,
		GameWidth:       req.GameWidth,
		GameHeight:      req.GameHeight,
		GameDir:         gameDir,
		AuthlibJar:      req.AuthlibJar,
		AuthlibMetaJSON: authlibMeta,
		JoinServer:      req.JoinServer,
		JoinPort:        req.JoinPort,
	}

	if config.MaxMemory == 0 {
		config.MaxMemory = 4096
	}
	if config.MinMemory == 0 {
		config.MinMemory = 1024
	}
	if config.GameWidth == 0 {
		config.GameWidth = 854
	}
	if config.GameHeight == 0 {
		config.GameHeight = 480
	}

	version := &models.MinecraftVersion{
		VersionID:     req.VersionID,
		VersionType:   req.VersionType,
		LoaderType:    req.LoaderType,
		LoaderVersion: req.LoaderVersion,
	}

	account := &models.MinecraftAccount{
		Username:    req.Username,
		UUID:        req.UUID,
		AccessToken: req.AccessToken,
		ClientToken: req.ClientToken,
		AuthType:    req.AuthType,
		AuthServer:  req.AuthServer,
	}

	logCallback := func(source, line string) {
		a.emit("launcher:log", map[string]interface{}{
			"source": source,
			"line":   line,
		})
	}

	return a.launcherService.LaunchGame(config, version, account, logCallback)
}

// ==================== Launcher config (write) ====================

// UpdateLauncherConfig persists the user-editable subset of the launcher
// configuration and returns the full merged config.
func (a *App) UpdateLauncherConfig(cfg models.LauncherConfig) *models.ApiResponse {
	return a.launcherService.SaveLauncherConfig(cfg)
}

// ==================== Java installations (write) ====================

// AddJavaInstallation registers a Java executable after detecting its version.
func (a *App) AddJavaInstallation(path, friendlyName string) *models.ApiResponse {
	if path == "" {
		return &models.ApiResponse{Success: false, Error: "path is required"}
	}

	version, err := services.DetectJavaVersion(path)
	if err != nil {
		return &models.ApiResponse{Success: false, Error: "Failed to detect Java version: " + err.Error()}
	}

	java := &models.JavaInstallation{
		Path:         path,
		FriendlyName: friendlyName,
		Version:      version,
		IsDefault:    false,
	}

	if err := a.javaRepo.Create(java); err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}

	return &models.ApiResponse{Success: true, Data: java}
}

// UpdateJavaInstallation updates the path and/or friendly name of an existing
// Java installation, re-detecting the version when the path changes.
func (a *App) UpdateJavaInstallation(id int, path, friendlyName string) *models.ApiResponse {
	java, err := a.javaRepo.GetByID(int64(id))
	if err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}

	if path != "" {
		java.Path = path
		version, err := services.DetectJavaVersion(path)
		if err != nil {
			return &models.ApiResponse{Success: false, Error: "Failed to detect Java version: " + err.Error()}
		}
		java.Version = version
	}

	if friendlyName != "" {
		java.FriendlyName = friendlyName
	}

	if err := a.javaRepo.Update(java); err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}

	return &models.ApiResponse{Success: true, Data: java}
}

// DeleteJavaInstallation removes a Java installation record.
func (a *App) DeleteJavaInstallation(id int) *models.ApiResponse {
	if err := a.javaRepo.Delete(int64(id)); err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}
	return &models.ApiResponse{Success: true}
}

// SetDefaultJavaInstallation marks a Java installation as the default.
func (a *App) SetDefaultJavaInstallation(id int) *models.ApiResponse {
	if err := a.javaRepo.SetDefault(int64(id)); err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}
	return &models.ApiResponse{Success: true}
}
