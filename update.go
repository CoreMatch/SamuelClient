package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"HRPMLW/internal/database"
	"HRPMLW/internal/models"
	"HRPMLW/internal/updater"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const updateEvent = "launcher:update"

type UpdateStatus struct {
	State          string `json:"state"`
	Version        string `json:"version"`
	Notes          string `json:"notes"`
	Mandatory      bool   `json:"mandatory"`
	AllowDowngrade bool   `json:"allowDowngrade"`
	Downloaded     int64  `json:"downloaded"`
	Total          int64  `json:"total"`
	Error          string `json:"error"`
}

func (a *App) GetUpdateStatus() *models.ApiResponse {
	a.updateMu.Lock()
	status := a.updateStatus
	a.updateMu.Unlock()
	return &models.ApiResponse{Success: true, Data: status}
}

func (a *App) CheckForUpdates() *models.ApiResponse {
	a.startUpdateCheck()
	return &models.ApiResponse{Success: true}
}

func (a *App) InstallUpdate() *models.ApiResponse {
	a.updateMu.Lock()
	if a.updateInstalling {
		a.updateMu.Unlock()
		return &models.ApiResponse{Error: "an update is already being installed"}
	}
	candidate := a.updateCandidate
	if candidate == nil {
		a.updateMu.Unlock()
		return &models.ApiResponse{Error: "no update is available"}
	}
	a.updateInstalling = true
	a.updateCheckID++
	if a.updateCheckCancel != nil {
		a.updateCheckCancel()
		a.updateCheckCancel = nil
	}
	a.updateStatus = statusForUpdate("downloading", candidate, "")
	status := a.updateStatus
	a.updateMu.Unlock()
	a.emit(updateEvent, status)

	go a.installUpdate(candidate)
	return &models.ApiResponse{Success: true}
}

func (a *App) startUpdateCheck() {
	executablePath, err := os.Executable()
	if err != nil {
		fmt.Printf("[updater] locate executable: %v\n", err)
		return
	}
	currentVersion := updater.DetectVersion(version, executablePath)
	cachePath := filepath.Join(database.DataDir(), "update-cache.json")

	a.updateMu.Lock()
	if a.updateCheckCancel != nil || a.updateInstalling {
		a.updateMu.Unlock()
		return
	}
	if cached, err := updater.LoadCachedUpdate(cachePath, currentVersion); err == nil && cached != nil && cached.Mandatory {
		a.updateCandidate = cached
		a.updateStatus = statusForUpdate("available", cached, "")
	}
	a.updateCheckID++
	checkID := a.updateCheckID
	parent := a.ctx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 45*time.Second)
	a.updateCheckCancel = cancel
	a.updateStatus.State = "checking"
	a.updateStatus.Error = ""
	status := a.updateStatus
	a.updateMu.Unlock()
	a.emit(updateEvent, status)

	go func() {
		defer func() {
			a.updateMu.Lock()
			if a.updateCheckID == checkID {
				a.updateCheckCancel = nil
			}
			a.updateMu.Unlock()
		}()

		candidate, err := updater.Check(ctx, currentVersion, "", nil)
		a.updateMu.Lock()
		if a.updateCheckID != checkID || a.updateInstalling {
			a.updateMu.Unlock()
			return
		}
		if err != nil {
			if a.updateCandidate != nil {
				a.updateStatus = statusForUpdate("error", a.updateCandidate, err.Error())
			} else {
				a.updateStatus = UpdateStatus{State: "error", Error: err.Error()}
			}
			status := a.updateStatus
			a.updateMu.Unlock()
			a.emit(updateEvent, status)
			fmt.Printf("[updater] check failed: %v\n", err)
			return
		}
		if candidate == nil {
			a.updateCandidate = nil
			a.updateStatus = UpdateStatus{State: "current"}
			_ = os.Remove(cachePath)
			status := a.updateStatus
			a.updateMu.Unlock()
			a.emit(updateEvent, status)
			return
		}
		if err := updater.SaveManifestCache(cachePath, candidate.ManifestJSON()); err != nil {
			fmt.Printf("[updater] save manifest cache: %v\n", err)
		}
		a.updateCandidate = candidate
		a.updateStatus = statusForUpdate("available", candidate, "")
		status := a.updateStatus
		a.updateMu.Unlock()
		a.emit(updateEvent, status)
	}()
}

func (a *App) installUpdate(candidate *updater.Update) {
	parent := a.ctx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 15*time.Minute)
	a.updateMu.Lock()
	a.updateInstallCancel = cancel
	a.updateMu.Unlock()
	defer cancel()

	executablePath, err := os.Executable()
	if err != nil {
		a.failUpdate(err)
		return
	}
	progress := func(downloaded, total int64) {
		a.updateMu.Lock()
		if a.updateInstalling {
			a.updateStatus.Downloaded = downloaded
			a.updateStatus.Total = total
		}
		status := a.updateStatus
		a.updateMu.Unlock()
		a.emit(updateEvent, status)
	}
	stagedPath, err := updater.DownloadAndStage(ctx, candidate, executablePath, nil, progress)
	if err != nil {
		a.failUpdate(err)
		return
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		_ = os.Remove(stagedPath)
		a.failUpdate(err)
		return
	}
	if err := updater.StartApply(stagedPath, executablePath, workingDirectory, os.Args[1:]); err != nil {
		_ = os.Remove(stagedPath)
		a.failUpdate(err)
		return
	}
	a.updateMu.Lock()
	a.updateStatus = statusForUpdate("installing", candidate, "")
	status := a.updateStatus
	a.updateMu.Unlock()
	a.emit(updateEvent, status)
	wailsruntime.Quit(a.ctx)
}

func (a *App) failUpdate(err error) {
	a.updateMu.Lock()
	a.updateInstalling = false
	a.updateInstallCancel = nil
	if a.updateCandidate != nil {
		a.updateStatus = statusForUpdate("error", a.updateCandidate, err.Error())
	} else {
		a.updateStatus = UpdateStatus{State: "error", Error: err.Error()}
	}
	status := a.updateStatus
	a.updateMu.Unlock()
	a.emit(updateEvent, status)
	fmt.Printf("[updater] installation failed: %v\n", err)
}

func statusForUpdate(state string, update *updater.Update, message string) UpdateStatus {
	return UpdateStatus{
		State:          state,
		Version:        update.Version,
		Notes:          update.Notes,
		Mandatory:      update.Mandatory,
		AllowDowngrade: update.AllowDowngrade,
		Total:          update.Asset.Size,
		Error:          message,
	}
}
