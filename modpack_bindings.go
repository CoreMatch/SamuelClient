package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
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
		parent, a.downloadService(), a.modpackJavaResolver(""), register, progress, log)
	if err == nil {
		// Once registered, a late cancel must not report a successful instance as cancelled.
		ctx = context.Background()
	}
	return a.installResult(ctx, taskID, modpackResponse(result, err))
}

func (a *App) modpackJavaResolver(preferred string) services.PackJavaResolver {
	paths := []string{preferred, a.launcherService.GetJavaPath()}
	if a.javaRepo != nil && database.GetDB() != nil {
		if installations, err := a.javaRepo.GetAll(); err == nil {
			for _, java := range installations {
				paths = append(paths, java.Path)
			}
		}
	}
	return func(ctx context.Context, major int, progress services.PackProgress) (string, error) {
		cache, err := filepath.Abs(filepath.Join(database.DataDir(), "jre", "modpacks"))
		if err != nil {
			return "", err
		}
		return services.EnsureModpackJava(ctx, cache, paths, major, progress)
	}
}

var modpackLaunchGate = make(chan struct{}, 1)

func (a *App) prepareModpackLaunch(gameDir, versionID, preferredJava string) (string, error) {
	if !slices.Contains(a.launcherService.GameDirs(), gameDir) {
		return preferredJava, nil
	}
	parent, err := filepath.Abs(filepath.Join(database.DataDir(), "instances"))
	if err != nil {
		return "", err
	}
	if filepath.Dir(gameDir) != parent || !strings.HasPrefix(filepath.Base(gameDir), "pack-") {
		return preferredJava, nil
	}
	pack, ok := services.ReadModpackInstance(gameDir)
	if !ok || pack.VersionID != versionID {
		return preferredJava, nil
	}

	id := "modpack-repair-" + newClientToken()
	ctx, entry := a.beginInstall(id)
	defer a.finishInstall(id, entry)
	select {
	case modpackLaunchGate <- struct{}{}:
		defer func() { <-modpackLaunchGate }()
	case <-ctx.Done():
		return "", ctx.Err()
	}
	var emitted atomic.Bool
	progress := func(stage, name string, downloaded, total int64) {
		emitted.Store(true)
		a.emit("download:progress", map[string]any{
			"version_id": id, "stage": stage, "name": name, "downloaded": downloaded, "total": total,
		})
	}
	log := func(source, line string) {
		a.emit("launcher:log", map[string]any{"source": source, "line": line})
	}
	java, err := services.NewModrinthService().PrepareModpackLaunch(ctx, gameDir, pack, a.downloadService(),
		a.modpackJavaResolver(preferredJava), progress, log)
	if err == nil {
		err = ctx.Err()
	}
	if emitted.Load() {
		result := a.installResult(ctx, id, modpackResponse(nil, err))
		if !result.Success {
			return "", fmt.Errorf("%s", result.Error)
		}
	}
	return java, err
}
