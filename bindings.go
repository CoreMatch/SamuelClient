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
//     "native"|"asset"|"loader"|"java-download"|"java-extract"|
//     "completed"|"failed"|"cancelled", "name": string,
//     "downloaded": number, "total": number, "error"?: string}. The "loader" stage is emitted
//     by InstallFabricLoader/InstallForgeLoader; total>0 means a real byte
//     percentage is available, total<=0 means an indeterminate stage.
//     downloaded/total are cumulative within a stage: downloaded grows toward
//     the stage total as each artifact completes and only equals it when the
//     stage is done (so the bar advances 0->100 instead of pinning at 100%
//     after the first file). Terminal stages let the app-wide task queue
//     finalize each task across page changes.
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
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

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

const installRetryAttempts = 3

func waitForInstallRetry(ctx context.Context, attempt int) bool {
	timer := time.NewTimer(250 * time.Millisecond * time.Duration(1<<attempt))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func retryInstall(ctx context.Context, run func() *models.ApiResponse) *models.ApiResponse {
	var result *models.ApiResponse
	for attempt := 0; attempt < installRetryAttempts; attempt++ {
		result = run()
		if result != nil && result.Success {
			return result
		}
		if ctx.Err() != nil || attempt+1 == installRetryAttempts || !waitForInstallRetry(ctx, attempt) {
			return result
		}
	}
	return result
}

// beginInstall registers a cancellable context for the install identified by
// versionID and returns both. Starting an install for an id that is already in
// flight cancels the previous run (only one tracked install per version id).
// The caller must defer finishInstall(versionID, entry).
func (a *App) beginInstall(versionID string) (context.Context, *installCancel) {
	ctx, cancel := context.WithCancel(context.Background())
	entry := &installCancel{cancel: cancel}
	a.installMu.Lock()
	if a.installCancels == nil {
		a.installCancels = map[string]*installCancel{}
	}
	if prev, ok := a.installCancels[versionID]; ok {
		prev.cancel()
	}
	a.installCancels[versionID] = entry
	a.installMu.Unlock()
	return ctx, entry
}

// finishInstall cancels and unregisters the install entry. It only deletes the
// map entry if it still refers to this install, so a finished (e.g. superseded)
// run never removes a newer install registered under the same version id.
func (a *App) finishInstall(versionID string, entry *installCancel) {
	entry.cancel()
	a.installMu.Lock()
	if cur, ok := a.installCancels[versionID]; ok && cur == entry {
		delete(a.installCancels, versionID)
	}
	a.installMu.Unlock()
}

// CancelInstall aborts the in-flight download/install registered under
// versionID (PENDING-DECISIONS #10). Idempotent: cancelling an id with no
// running install succeeds without side effects. The cancelled install returns
// from its own binding with an "install cancelled" error and runs its normal
// failure cleanup, so a retry stays safe.
func (a *App) CancelInstall(versionID string) *models.ApiResponse {
	if versionID == "" {
		return &models.ApiResponse{Success: false, Error: "version_id is required"}
	}
	a.installMu.Lock()
	entry, ok := a.installCancels[versionID]
	a.installMu.Unlock()
	if !ok {
		return &models.ApiResponse{Success: true, Data: map[string]interface{}{"cancelled": false}}
	}
	entry.cancel()
	return &models.ApiResponse{Success: true, Data: map[string]interface{}{"cancelled": true}}
}

// installResult normalizes a service result after a cancellable install and
// emits a terminal progress event for the app-wide task queue.
func (a *App) installResult(ctx context.Context, versionID string, result *models.ApiResponse) *models.ApiResponse {
	payload := map[string]interface{}{
		"version_id": versionID,
		"stage":      "completed",
		"name":       "",
		"downloaded": int64(0),
		"total":      int64(0),
	}
	if ctx.Err() != nil {
		payload["stage"] = "cancelled"
		a.emit("download:progress", payload)
		return &models.ApiResponse{Success: false, Error: services.ErrInstallCancelled.Error()}
	}
	if result == nil {
		result = &models.ApiResponse{Success: false, Error: "install returned no result"}
	}
	if !result.Success {
		payload["stage"] = "failed"
		payload["error"] = result.Error
	}
	a.emit("download:progress", payload)
	return result
}

// ==================== Versions & loaders (read) ====================

func (a *App) downloadService() *services.BMCLAPIService {
	return a.bmclService.WithDownloadSettings(services.GetDownloadSettings())
}

func (a *App) GetDownloadSettings() *models.ApiResponse {
	return &models.ApiResponse{Success: true, Data: services.GetDownloadSettings()}
}

func (a *App) SetDownloadSettings(source, customShape, customURL string, concurrency int) *models.ApiResponse {
	cfg, err := services.SaveDownloadSettings(services.DownloadSettings{
		Source: source, CustomShape: customShape, CustomURL: customURL, Concurrency: concurrency,
	})
	if err != nil {
		return &models.ApiResponse{Error: err.Error()}
	}
	return &models.ApiResponse{Success: true, Data: cfg}
}

// GetVersions returns the version manifest list from the selected source.
func (a *App) GetVersions() *models.ApiResponse {
	return a.downloadService().GetVersionList(context.Background())
}

// GetFabricVersions returns available Fabric loader versions for a Minecraft
// version.
func (a *App) GetFabricVersions(mcVersion string) *models.ApiResponse {
	if mcVersion == "" {
		return &models.ApiResponse{Success: false, Error: "mc version is required"}
	}
	return a.downloadService().GetFabricVersions(mcVersion)
}

// GetForgeVersions returns available Forge loader versions for a Minecraft
// version.
func (a *App) GetForgeVersions(mcVersion string) *models.ApiResponse {
	if mcVersion == "" {
		return &models.ApiResponse{Success: false, Error: "mc version is required"}
	}
	return a.downloadService().GetForgeVersions(mcVersion)
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

// ==================== Game directory (read / write) ====================

// GetGameDirStatus reports the default install destination and saved folders.
func (a *App) GetGameDirStatus() *models.ApiResponse {
	return &models.ApiResponse{Success: true, Data: a.launcherService.GameDirStatus()}
}

// SetGameDir changes the install destination. An empty dir selects the built-in folder.
func (a *App) SetGameDir(dir string) *models.ApiResponse {
	return a.launcherService.SetGameDir(dir)
}

func (a *App) AddGameDir(dir string) *models.ApiResponse {
	return a.launcherService.AddGameDir(dir)
}

func (a *App) RemoveGameDir(dir string) *models.ApiResponse {
	return a.launcherService.RemoveGameDir(dir)
}

// ScanGameDirs walks common disk locations (home directory, plus drive roots
// on Windows) looking for Minecraft game directories. Data is
// {"dirs": []services.GameDirCandidate} with path and version_count per entry.
func (a *App) ScanGameDirs() *models.ApiResponse {
	return &models.ApiResponse{Success: true, Data: map[string]interface{}{
		"dirs": services.ScanGameDirs(),
	}}
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
	ID           string `json:"id"`
	Type         string `json:"type"`
	HasJSON      bool   `json:"has_json"`
	HasJAR       bool   `json:"has_jar"`
	GameDir      string `json:"game_dir"`
	InheritsFrom string `json:"inherits_from,omitempty"`
	ModpackName  string `json:"modpack_name,omitempty"`
}

// GetInstalledVersions scans each registered game directory. Versions with the
// same ID in different roots remain separate entries.
func (a *App) GetInstalledVersions() *models.ApiResponse {
	out := []InstalledVersion{}
	for _, gameDir := range a.launcherService.GameDirs() {
		out = append(out, scanInstalledVersions(gameDir)...)
	}
	return &models.ApiResponse{
		Success: true,
		Data:    map[string]interface{}{"versions": out},
	}
}

func scanInstalledVersions(gameDir string) []InstalledVersion {
	versionsDir := filepath.Join(gameDir, "versions")

	entries, err := os.ReadDir(versionsDir)
	if err != nil {
		return nil
	}

	out := []InstalledVersion{}
	pack, isPack := services.ReadModpackInstance(gameDir)
	for _, e := range entries {
		if !e.IsDir() || (isPack && e.Name() != pack.VersionID) {
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
		out = append(out, InstalledVersion{
			ID: id, Type: vType, HasJSON: hasJSON, HasJAR: hasJAR,
			GameDir: gameDir, InheritsFrom: inheritsFrom, ModpackName: pack.Name,
		})
	}

	return out
}

// ==================== Auth (write / sensitive) ====================
//
// These carry credentials or session tokens. They run in-process via the
// Wails binding and never traverse a local HTTP socket (ADR-0004). None of
// the values are written to logs.

// HRPAuthLogin authenticates against the built-in HRPAuth endpoint. There is
// no generic/configurable Yggdrasil entry point. The plaintext password is
// passed in-process to the auth service only.
func (a *App) HRPAuthLogin(username, password, clientToken string) *models.ApiResponse {
	if username == "" || password == "" {
		return &models.ApiResponse{Success: false, Error: "username and password are required"}
	}
	if clientToken == "" {
		clientToken = newClientToken()
	}
	return a.authService.AuthenticateWithHRPAuth(username, password, clientToken)
}

// HRPAuthRefresh refreshes an access token, optionally binding it to a
// specific profile.
func (a *App) HRPAuthRefresh(accessToken, clientToken, selectedProfileID string) *models.ApiResponse {
	if accessToken == "" || clientToken == "" {
		return &models.ApiResponse{Success: false, Error: "access token and client token are required"}
	}
	return a.authService.RefreshHRPAuthTokenWithProfile(accessToken, clientToken, selectedProfileID)
}

// HRPAuthValidate checks whether a stored access token is still valid.
func (a *App) HRPAuthValidate(accessToken, clientToken string) *models.ApiResponse {
	if accessToken == "" {
		return &models.ApiResponse{Success: false, Error: "access token is required"}
	}
	return a.authService.ValidateHRPAuthToken(accessToken, clientToken)
}

// GetProfileTextures returns the signed-in player's current skin and cape
// bindings. The argument is the Yggdrasil session token, not an MSA OAuth
// access/refresh token; both login paths provide this credential.
func (a *App) GetProfileTextures(yggdrasilAccessToken string) *models.ApiResponse {
	return a.authService.GetHRPAuthTextures(yggdrasilAccessToken)
}

// UploadProfileTexture forwards a PNG to the Yggdrasil API without exposing
// the session token to a frontend HTTP request.
func (a *App) UploadProfileTexture(yggdrasilAccessToken, profileID, textureType, model, fileName, imageBase64 string) *models.ApiResponse {
	if len(imageBase64) > 3*1024*1024 {
		return &models.ApiResponse{Success: false, Error: "PNG file must be no larger than 2 MiB"}
	}
	imageData, err := base64.StdEncoding.DecodeString(imageBase64)
	if err != nil {
		return &models.ApiResponse{Success: false, Error: "invalid image data"}
	}
	return a.authService.UploadHRPAuthTexture(yggdrasilAccessToken, profileID, textureType, model, fileName, imageData)
}

// DeleteProfileTexture removes the signed-in player's active skin or cape.
func (a *App) DeleteProfileTexture(yggdrasilAccessToken, profileID, textureType string) *models.ApiResponse {
	return a.authService.DeleteHRPAuthTexture(yggdrasilAccessToken, profileID, textureType)
}

// ==================== Microsoft login (ADR-0007) ====================
//
// The client walks the public Microsoft OAuth chain (endpoints swappable,
// defaulting to the local mcmock) and exchanges the resulting Minecraft
// Services token at the HA side. The MSA refresh token is persisted in the OS
// keychain alongside the Yggdrasil session tokens.

// MicrosoftBeginDeviceCode starts the MSA device-code flow. The frontend shows
// the returned user_code + verification_uri, then calls
// MicrosoftCompleteDeviceCode with the device_code.
func (a *App) MicrosoftBeginDeviceCode() *models.ApiResponse {
	dc, err := a.microsoftService.StartDeviceCode()
	if err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}
	return &models.ApiResponse{Success: true, Data: map[string]interface{}{
		"device_code":      dc.DeviceCode,
		"user_code":        dc.UserCode,
		"verification_uri": dc.VerificationURI,
		"expires_in":       dc.ExpiresIn,
		"interval":         dc.Interval,
	}}
}

// MicrosoftCompleteDeviceCode polls the device-code grant, walks the
// XBL/XSTS/Minecraft chain, and exchanges the MC token for an HA Yggdrasil
// session. It returns the session tokens, the MSA refresh token (for keychain
// storage) and the resolved identity; the frontend persists metadata and calls
// StoreMicrosoftAccountTokens.
func (a *App) MicrosoftCompleteDeviceCode(deviceCode string, expiresIn, interval int) *models.ApiResponse {
	if deviceCode == "" {
		return &models.ApiResponse{Success: false, Error: "device code is required"}
	}
	dc := &services.DeviceCodeResponse{DeviceCode: deviceCode, ExpiresIn: expiresIn, Interval: interval}

	msa, err := a.microsoftService.PollDeviceCode(dc)
	if err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}
	mcToken, err := a.microsoftService.FullChain(msa.AccessToken)
	if err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}
	exch, err := a.microsoftService.Exchange(mcToken, "")
	if err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}

	uuid, name := "", ""
	if exch.SelectedProfile != nil {
		uuid = exch.SelectedProfile.ID
		name = exch.SelectedProfile.Name
	}

	return &models.ApiResponse{Success: true, Data: map[string]interface{}{
		"accessToken":     exch.AccessToken,
		"clientToken":     exch.ClientToken,
		"msaRefreshToken": msa.RefreshToken,
		"uuid":            uuid,
		"username":        name,
		"claimRequired":   exch.ClaimRequired,
	}}
}

// StoreMicrosoftAccountTokens persists a Microsoft account's token triple
// (Yggdrasil access/client + MSA refresh token) in the OS keychain.
func (a *App) StoreMicrosoftAccountTokens(id, accessToken, clientToken, msaRefreshToken string) *models.ApiResponse {
	if id == "" {
		return &models.ApiResponse{Success: false, Error: "account id is required"}
	}
	if accessToken == "" {
		return &models.ApiResponse{Success: false, Error: "access token is required"}
	}
	if err := a.tokenStore.Set(id, services.TokenPair{
		AccessToken:     accessToken,
		ClientToken:     clientToken,
		MSARefreshToken: msaRefreshToken,
	}); err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}
	return &models.ApiResponse{Success: true}
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

// ==================== Token storage (write / sensitive) ====================
//
// Session tokens live in the OS credential store, not in the frontend's
// localStorage (ADR-0008). The frontend keeps only non-sensitive account
// metadata and calls these bindings to persist, load, or drop the token pair
// of one account. Values are never written to logs.

// StoreAccountTokens persists the token pair of one account in the OS
// keychain, keyed by the stable account id. Callers treat a failure as
// "keep the previous state and warn" — the frontend migration relies on that
// to stay loss-free.
func (a *App) StoreAccountTokens(id, accessToken, clientToken string) *models.ApiResponse {
	if id == "" {
		return &models.ApiResponse{Success: false, Error: "account id is required"}
	}
	if accessToken == "" {
		return &models.ApiResponse{Success: false, Error: "access token is required"}
	}
	if err := a.tokenStore.Set(id, services.TokenPair{AccessToken: accessToken, ClientToken: clientToken}); err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}
	return &models.ApiResponse{Success: true}
}

// LoadAccountTokens returns the stored token pair of one account. A missing
// entry is reported as success=false with error "token entry not found" so
// the frontend can distinguish re-login-needed from a broken backend.
func (a *App) LoadAccountTokens(id string) *models.ApiResponse {
	if id == "" {
		return &models.ApiResponse{Success: false, Error: "account id is required"}
	}
	pair, err := a.tokenStore.Get(id)
	if err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}
	return &models.ApiResponse{Success: true, Data: map[string]interface{}{
		"accessToken": pair.AccessToken,
		"clientToken": pair.ClientToken,
	}}
}

// DeleteAccountTokens removes the stored token pair of one account. Deleting
// a missing entry succeeds so account removal cannot fail on a keychain that
// is already clean (or unavailable).
func (a *App) DeleteAccountTokens(id string) *models.ApiResponse {
	if id == "" {
		return &models.ApiResponse{Success: false, Error: "account id is required"}
	}
	if err := a.tokenStore.Delete(id); err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}
	return &models.ApiResponse{Success: true}
}

// KeychainStatus reports whether a credential store is usable on this
// machine and which one is active. Data is {"available": bool, "error":
// string, "backend": "keychain"|"file"|""}; the frontend shows a persistent
// warning when nothing is usable, and an informational notice when the
// automatic file fallback (ADR-0010) is active instead of the OS keychain.
func (a *App) KeychainStatus() *models.ApiResponse {
	backend := ""
	if br, ok := a.tokenStore.(interface{ Backend() string }); ok {
		backend = br.Backend()
	}
	if err := a.tokenStore.Available(); err != nil {
		return &models.ApiResponse{Success: true, Data: map[string]interface{}{
			"available": false,
			"error":     err.Error(),
			"backend":   backend,
		}}
	}
	return &models.ApiResponse{Success: true, Data: map[string]interface{}{
		"available": true,
		"error":     "",
		"backend":   backend,
	}}
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
		gameDir = a.launcherService.ResolveGameDir()
	}

	ctx, entry := a.beginInstall(versionID)
	defer a.finishInstall(versionID, entry)

	progress := func(stage, name string, downloaded, total int64) {
		a.emit("download:progress", map[string]interface{}{
			"version_id": versionID,
			"stage":      stage,
			"name":       name,
			"downloaded": downloaded,
			"total":      total,
		})
	}

	service := a.downloadService()
	return a.installResult(ctx, versionID, retryInstall(ctx, func() *models.ApiResponse {
		return service.InstallVersion(ctx, versionID, gameDir, progress)
	}))
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
		gameDir = a.launcherService.ResolveGameDir()
	}

	versionID := fmt.Sprintf("fabric-loader-%s-%s", loaderVersion, mcVersion)
	ctx, entry := a.beginInstall(versionID)
	defer a.finishInstall(versionID, entry)

	progress := func(stage, name string, downloaded, total int64) {
		a.emit("download:progress", map[string]interface{}{
			"version_id": versionID,
			"stage":      stage,
			"name":       name,
			"downloaded": downloaded,
			"total":      total,
		})
	}

	service := a.downloadService()
	return a.installResult(ctx, versionID, retryInstall(ctx, func() *models.ApiResponse {
		return service.InstallFabricLoader(ctx, mcVersion, loaderVersion, gameDir, progress)
	}))
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
		gameDir = a.launcherService.ResolveGameDir()
	}

	versionID := mcVersion + "-forge-" + forgeVersion
	ctx, entry := a.beginInstall(versionID)
	defer a.finishInstall(versionID, entry)

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
	service := a.downloadService()
	return a.installResult(ctx, versionID, retryInstall(ctx, func() *models.ApiResponse {
		return service.InstallForgeLoader(ctx, mcVersion, forgeVersion, gameDir, javaPath, logCallback, progress)
	}))
}

// RemoveLoaderProfile drops one installed Fabric or Forge profile without
// touching its inherited Minecraft version or shared libraries.
func (a *App) RemoveLoaderProfile(versionID, gameDir string) *models.ApiResponse {
	if gameDir == "" {
		gameDir = a.launcherService.ResolveGameDir()
	}
	return services.RemoveLoaderProfile(gameDir, versionID)
}

// ==================== Mods ====================

func (a *App) withModGameDir(gameDir string, run func() (interface{}, error)) *models.ApiResponse {
	if gameDir == "" || !slices.Contains(a.launcherService.GameDirs(), gameDir) {
		return &models.ApiResponse{Error: "select a registered game folder"}
	}
	data, err := run()
	if err != nil {
		return &models.ApiResponse{Error: err.Error()}
	}
	return &models.ApiResponse{Success: true, Data: data}
}

func (a *App) ListMods(gameDir string) *models.ApiResponse {
	return a.withModGameDir(gameDir, func() (interface{}, error) {
		return services.ListMods(gameDir)
	})
}

func (a *App) AddMod(gameDir string) *models.ApiResponse {
	return a.withModGameDir(gameDir, func() (interface{}, error) {
		if a.ctx == nil {
			return nil, fmt.Errorf("file picker is unavailable")
		}
		source, err := wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
			Title:   "Select a mod",
			Filters: []wailsruntime.FileFilter{{DisplayName: "Minecraft mods (*.jar)", Pattern: "*.jar;*.JAR"}},
		})
		if err != nil || source == "" {
			return false, err
		}
		return true, services.AddMod(gameDir, source)
	})
}

func (a *App) SetModEnabled(gameDir, name string, enabled bool) *models.ApiResponse {
	return a.withModGameDir(gameDir, func() (interface{}, error) {
		return nil, services.SetModEnabled(gameDir, name, enabled)
	})
}

func (a *App) DeleteMod(gameDir, name string) *models.ApiResponse {
	return a.withModGameDir(gameDir, func() (interface{}, error) {
		return nil, services.DeleteMod(gameDir, name)
	})
}

func modrinthTargets(gameDir string) []services.ModrinthTarget {
	targets := []services.ModrinthTarget{}
	for _, v := range scanInstalledVersions(gameDir) {
		if !v.HasJSON || !v.HasJAR || v.InheritsFrom == "" {
			continue
		}
		var loader string
		switch {
		case strings.HasPrefix(v.ID, "fabric-loader-"):
			loader = "fabric"
		case strings.HasPrefix(v.ID, v.InheritsFrom+"-forge"):
			loader = "forge"
		default:
			continue
		}
		targets = append(targets, services.ModrinthTarget{ProfileID: v.ID, GameVersion: v.InheritsFrom, Loader: loader})
	}
	return targets
}

func (a *App) GetModrinthTargets(gameDir string) *models.ApiResponse {
	return a.withModGameDir(gameDir, func() (any, error) {
		return modrinthTargets(gameDir), nil
	})
}

func (a *App) withModrinthTarget(gameDir, profileID string, run func(context.Context, services.ModrinthTarget) (any, error)) *models.ApiResponse {
	return a.withModGameDir(gameDir, func() (any, error) {
		for _, target := range modrinthTargets(gameDir) {
			if target.ProfileID == profileID {
				parent := a.ctx
				if parent == nil {
					parent = context.Background()
				}
				ctx, cancel := context.WithTimeout(parent, 5*time.Minute)
				defer cancel()
				return run(ctx, target)
			}
		}
		return nil, fmt.Errorf("select an installed Fabric or Forge profile from this game folder")
	})
}

func (a *App) SearchModrinth(gameDir, profileID, query string, offset int) *models.ApiResponse {
	return a.withModrinthTarget(gameDir, profileID, func(ctx context.Context, target services.ModrinthTarget) (any, error) {
		return services.NewModrinthService().Search(ctx, target, query, offset)
	})
}

func (a *App) GetModrinthVersions(gameDir, profileID, projectID string) *models.ApiResponse {
	return a.withModrinthTarget(gameDir, profileID, func(ctx context.Context, target services.ModrinthTarget) (any, error) {
		versions, err := services.NewModrinthService().Versions(ctx, target, projectID)
		out := make([]map[string]string, 0, len(versions))
		for _, v := range versions {
			out = append(out, map[string]string{"id": v.ID, "version_number": v.Number, "version_type": v.Channel})
		}
		return out, err
	})
}

func (a *App) PreviewModrinthInstall(gameDir, profileID, projectID, versionID string) *models.ApiResponse {
	return a.withModrinthTarget(gameDir, profileID, func(ctx context.Context, target services.ModrinthTarget) (any, error) {
		return services.NewModrinthService().Plan(ctx, target, projectID, versionID)
	})
}

func (a *App) InstallModrinthMod(gameDir, profileID, projectID, versionID, fingerprint string) *models.ApiResponse {
	return a.withModrinthTarget(gameDir, profileID, func(ctx context.Context, target services.ModrinthTarget) (any, error) {
		return nil, services.NewModrinthService().Install(ctx, gameDir, target, projectID, versionID, fingerprint)
	})
}

func curseForgeService() *services.CurseForgeService {
	return services.NewCurseForgeService(os.Getenv("SAMUEL_CURSEFORGE_API_KEY"))
}

func (a *App) GetCurseForgeTargets(gameDir string) *models.ApiResponse {
	if strings.TrimSpace(os.Getenv("SAMUEL_CURSEFORGE_API_KEY")) == "" {
		return &models.ApiResponse{Error: "CurseForge is not configured. Set SAMUEL_CURSEFORGE_API_KEY before launching SamuelClient, or download from the official website."}
	}
	return a.GetModrinthTargets(gameDir)
}

func (a *App) SearchCurseForge(gameDir, profileID, query string, offset int) *models.ApiResponse {
	return a.withModrinthTarget(gameDir, profileID, func(ctx context.Context, target services.ModrinthTarget) (any, error) {
		return curseForgeService().Search(ctx, target, query, offset)
	})
}

func (a *App) GetCurseForgeVersions(gameDir, profileID, projectID string) *models.ApiResponse {
	return a.withModrinthTarget(gameDir, profileID, func(ctx context.Context, target services.ModrinthTarget) (any, error) {
		return curseForgeService().Versions(ctx, target, projectID)
	})
}

func (a *App) PreviewCurseForgeInstall(gameDir, profileID, projectID, versionID string) *models.ApiResponse {
	return a.withModrinthTarget(gameDir, profileID, func(ctx context.Context, target services.ModrinthTarget) (any, error) {
		return curseForgeService().Plan(ctx, target, projectID, versionID)
	})
}

func (a *App) InstallCurseForgeMod(gameDir, profileID, projectID, versionID, fingerprint string) *models.ApiResponse {
	return a.withModrinthTarget(gameDir, profileID, func(ctx context.Context, target services.ModrinthTarget) (any, error) {
		return nil, curseForgeService().Install(ctx, gameDir, target, projectID, versionID, fingerprint)
	})
}

// ==================== Launch (write, streaming) ====================

// LaunchGameRequest mirrors the previous POST /api/v1/launcher/launch body so
// the frontend can keep the same field names.
type LaunchGameRequest struct {
	VersionID   string `json:"version_id"`
	VersionType string `json:"version_type"`
	Username    string `json:"username"`
	UUID        string `json:"uuid"`
	AccessToken string `json:"access_token"`
	ClientToken string `json:"client_token"`
	AuthType    string `json:"auth_type"`
	// Optional override; empty launches with the agent bundled in the
	// binary (ADR-0009).
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
		gameDir = a.launcherService.ResolveGameDir()
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
	// HRPAuth (required pre-launch step per the authlib-injector wiki). On
	// failure we still launch; authlib-injector can fall back to a runtime
	// fetch.
	authlibMeta := req.AuthlibMeta
	if authlibMeta == "" && services.IsHRPAuthAccount(req.AuthType) {
		metaResult := a.authService.GetHRPAuthMeta()
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

// GetSupportedJavaRuntimeVersions lists the maintained Temurin LTS versions.
func (a *App) GetSupportedJavaRuntimeVersions() *models.ApiResponse {
	return &models.ApiResponse{Success: true, Data: services.SupportedJavaRuntimeMajors()}
}

// GetDefaultJavaRuntimePath returns the portable default destination for a
// Java runtime major.
func (a *App) GetDefaultJavaRuntimePath(major int) *models.ApiResponse {
	path, err := services.DefaultJavaRuntimeDir(major)
	if err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}
	return &models.ApiResponse{Success: true, Data: path}
}

// InstallJavaRuntime downloads, extracts, and registers an Eclipse Temurin JRE.
func (a *App) InstallJavaRuntime(major int, destination string) *models.ApiResponse {
	versionID := fmt.Sprintf("java-%d", major)
	ctx, entry := a.beginInstall(versionID)
	defer a.finishInstall(versionID, entry)

	installer := services.NewJavaRuntimeInstaller()
	progress := func(stage string, downloaded, total int64) {
		a.emit("download:progress", map[string]interface{}{
			"version_id": versionID,
			"stage":      stage,
			"name":       fmt.Sprintf("Temurin Java %d", major),
			"downloaded": downloaded,
			"total":      total,
		})
	}
	var installed *services.JavaRuntimeInstall
	var err error
	for attempt := 0; attempt < installRetryAttempts; attempt++ {
		installed, err = installer.Install(ctx, major, destination, progress)
		if err == nil || ctx.Err() != nil || attempt+1 == installRetryAttempts || !waitForInstallRetry(ctx, attempt) {
			break
		}
	}
	if err != nil {
		return a.installResult(ctx, versionID, &models.ApiResponse{Success: false, Error: err.Error()})
	}
	if ctx.Err() != nil {
		_ = os.RemoveAll(destination)
		return a.installResult(ctx, versionID, &models.ApiResponse{Success: false, Error: ctx.Err().Error()})
	}
	result := a.AddJavaInstallation(installed.JavaPath, fmt.Sprintf("Temurin Java %d", major))
	if !result.Success {
		_ = os.RemoveAll(destination)
	}
	return a.installResult(ctx, versionID, result)
}

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
