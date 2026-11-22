package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"HRPMLW/internal/database"
	"HRPMLW/internal/models"
	"HRPMLW/internal/services"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func modpackResponse(data any, err error) *models.ApiResponse {
	if err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}
	return &models.ApiResponse{Success: true, Data: data}
}

func (a *App) SearchModrinthModpacks(query string, offset int) *models.ApiResponse {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return modpackResponse(services.NewModrinthService().SearchModpacks(ctx, query, offset))
}

func (a *App) GetModrinthModpackVersions(projectID string) *models.ApiResponse {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return modpackResponse(services.NewModrinthService().ModpackVersions(ctx, projectID))
}

func (a *App) PickModpack() *models.ApiResponse {
	if a.ctx == nil {
		return modpackResponse(nil, fmt.Errorf("file picker is unavailable"))
	}
	return modpackResponse(wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title:   "Select a Modrinth modpack",
		Filters: []wailsruntime.FileFilter{{DisplayName: "Modrinth modpacks (*.mrpack)", Pattern: "*.mrpack;*.MRPACK"}},
	}))
}

func (a *App) PreviewModpack(projectID, versionID, localPath string) *models.ApiResponse {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	service := services.NewModrinthService()
	return modpackResponse(service.PreviewModpack(ctx, services.ModpackSource{ProjectID: projectID, VersionID: versionID, LocalPath: localPath}))
}

func (a *App) InstallModpack(taskID, projectID, versionID, localPath, fingerprint string, selected []string) *models.ApiResponse {
	suffix, hasPrefix := strings.CutPrefix(taskID, "modpack-")
	id, err := hex.DecodeString(suffix)
	if !hasPrefix || err != nil || len(id) != 16 || len(selected) > 50000 {
		return modpackResponse(nil, fmt.Errorf("invalid modpack install request"))
	}
	ctx, entry := a.beginInstall(taskID)
	defer a.finishInstall(taskID, entry)
	progress := func(stage, name string, downloaded, total int64) {
		a.emit("download:progress", map[string]any{
			"version_id": taskID, "stage": stage, "name": name, "downloaded": downloaded, "total": total,
		})
	}
	log := func(source, line string) {
		a.emit("launcher:log", map[string]any{"source": source, "line": line})
	}
	var javaPaths []string
	if a.javaRepo != nil {
		if installations, err := a.javaRepo.GetAll(); err == nil {
			for _, java := range installations {
				javaPaths = append(javaPaths, java.Path)
			}
		}
	}
	javaPaths = append(javaPaths, a.launcherService.GetJavaPath())
	parent, err := filepath.Abs(filepath.Join(database.DataDir(), "instances"))
	if err != nil {
		return a.installResult(ctx, taskID, modpackResponse(nil, err))
	}
	register := func(dir string) error {
		if res := a.launcherService.AddGameDir(dir); !res.Success {
			return fmt.Errorf("%s", res.Error)
		}
		return nil
	}
	result, err := services.NewModrinthService().InstallModpack(ctx,
		services.ModpackSource{ProjectID: projectID, VersionID: versionID, LocalPath: localPath}, fingerprint, selected,
		parent, a.downloadService(), javaPaths, register, progress, log)
	if err == nil {
		// Once registered, a late cancel must not report a successful instance as cancelled.
		ctx = context.Background()
	}
	return a.installResult(ctx, taskID, modpackResponse(result, err))
}
