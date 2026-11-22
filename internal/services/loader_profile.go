package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"HRPMLW/internal/models"
)

// RemoveLoaderProfile removes only a Fabric or Forge version profile. Shared
// libraries and the inherited Minecraft version remain available to siblings.
func RemoveLoaderProfile(gameDir, versionID string) *models.ApiResponse {
	if gameDir == "" || versionID == "" || filepath.Base(versionID) != versionID ||
		versionID == "." || versionID == ".." || strings.ContainsAny(versionID, `/\`) {
		return &models.ApiResponse{Success: false, Error: "invalid loader profile"}
	}

	versionDir := filepath.Join(gameDir, "versions", versionID)
	raw, err := os.ReadFile(filepath.Join(versionDir, versionID+".json"))
	if err != nil {
		return &models.ApiResponse{Success: false, Error: "read loader profile: " + err.Error()}
	}
	var profile struct {
		InheritsFrom string `json:"inheritsFrom"`
	}
	if err := json.Unmarshal(raw, &profile); err != nil || profile.InheritsFrom == "" {
		return &models.ApiResponse{Success: false, Error: "version is not a removable loader profile"}
	}
	isFabric := strings.HasPrefix(versionID, "fabric-loader-")
	isForge := strings.HasPrefix(versionID, profile.InheritsFrom+"-forge")
	if !isFabric && !isForge {
		return &models.ApiResponse{Success: false, Error: "version is not a removable loader profile"}
	}

	if err := os.RemoveAll(versionDir); err != nil {
		return &models.ApiResponse{Success: false, Error: "remove loader profile: " + err.Error()}
	}
	removeLauncherProfileEntry(gameDir, versionID)
	return &models.ApiResponse{Success: true, Data: map[string]interface{}{"version_id": versionID}}
}

func removeLauncherProfileEntry(gameDir, versionID string) {
	path := filepath.Join(gameDir, "launcher_profiles.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var root map[string]json.RawMessage
	if json.Unmarshal(raw, &root) != nil {
		return
	}
	var profiles map[string]json.RawMessage
	if json.Unmarshal(root["profiles"], &profiles) != nil || profiles == nil {
		return
	}
	changed := false
	for name, rawProfile := range profiles {
		var profile struct {
			LastVersionID string `json:"lastVersionId"`
		}
		if json.Unmarshal(rawProfile, &profile) == nil && profile.LastVersionID == versionID {
			delete(profiles, name)
			changed = true
		}
	}
	if !changed {
		return
	}
	root["profiles"], err = json.Marshal(profiles)
	if err != nil {
		return
	}
	updated, err := json.Marshal(root)
	if err != nil {
		return
	}
	if err := os.WriteFile(path, updated, 0644); err != nil {
		return
	}
}
