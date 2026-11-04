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
// frontend via Wails runtime events; see the LaunchGame and DownloadVersion
// methods added in the next step for the event contract.

import (
	"encoding/json"
	"os"
	"path/filepath"

	"HRPMLW/internal/models"
	"HRPMLW/internal/services"
)

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
		if hasJSON {
			if data, err := os.ReadFile(jsonPath); err == nil {
				var meta struct {
					Type string `json:"type"`
				}
				if json.Unmarshal(data, &meta) == nil && meta.Type != "" {
					vType = meta.Type
				}
			}
		}
		out = append(out, InstalledVersion{ID: id, Type: vType, HasJSON: hasJSON, HasJAR: hasJAR})
	}

	return &models.ApiResponse{
		Success: true,
		Data:    map[string]interface{}{"versions": out},
	}
}

// ==================== Logs (read) ====================

// GetLogs returns the most recent entries of the local log table.
func (a *App) GetLogs(limit int) *models.ApiResponse {
	if limit <= 0 {
		limit = 100
	}
	logs, err := a.logRepo.GetAll("", limit, 0)
	if err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}
	return &models.ApiResponse{Success: true, Data: logs}
}
