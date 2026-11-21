package services

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"HRPMLW/internal/models"
)

const (
	downloadSettingsKey = "launcher.download"
	sourceBMCL          = "bmclapi"
	sourceOfficial      = "official"
	sourceCustom        = "custom"
	defaultBMCLURL      = "https://bmclapi2.bangbang93.com"
	officialMetaURL     = "https://piston-meta.mojang.com"
	officialAssetsURL   = "https://resources.download.minecraft.net"
	fabricMetaURL       = "https://meta.fabricmc.net/v2"
	forgeMavenURL       = "https://maven.minecraftforge.net"
)

type DownloadSettings struct {
	Source      string `json:"source"`
	CustomShape string `json:"custom_shape"`
	CustomURL   string `json:"custom_url"`
	Concurrency int    `json:"concurrency"`
}

func normalizeDownloadSettings(cfg DownloadSettings) (DownloadSettings, error) {
	if cfg.Concurrency < 1 || cfg.Concurrency > 32 {
		return cfg, fmt.Errorf("download concurrency must be between 1 and 32")
	}
	switch cfg.Source {
	case sourceBMCL, sourceOfficial, sourceCustom:
	default:
		return cfg, fmt.Errorf("unknown download source")
	}
	if cfg.CustomShape != sourceOfficial && cfg.CustomShape != sourceBMCL {
		return cfg, fmt.Errorf("unknown custom download API shape")
	}
	cfg.CustomURL = strings.TrimRight(strings.TrimSpace(cfg.CustomURL), "/")
	if cfg.Source == sourceCustom || cfg.CustomURL != "" {
		u, err := url.Parse(cfg.CustomURL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" ||
			u.User != nil || u.ForceQuery || u.RawQuery != "" || strings.Contains(cfg.CustomURL, "#") {
			return cfg, fmt.Errorf("custom source must be an HTTP(S) base URL without credentials, query or fragment")
		}
	}
	return cfg, nil
}

func GetDownloadSettings() DownloadSettings {
	defaults := DownloadSettings{Source: sourceBMCL, CustomShape: sourceBMCL, Concurrency: assetDownloadWorkers}
	settings, _ := loadLauncherSettings()
	cfg := defaults
	if json.Unmarshal([]byte(settings[downloadSettingsKey]), &cfg) == nil {
		if normalized, err := normalizeDownloadSettings(cfg); err == nil {
			return normalized
		}
	}
	return defaults
}

func SaveDownloadSettings(cfg DownloadSettings) (DownloadSettings, error) {
	cfg, err := normalizeDownloadSettings(cfg)
	if err != nil {
		return cfg, err
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return cfg, err
	}
	return cfg, saveLauncherSetting(downloadSettingsKey, string(data), "json", "Game download settings")
}

// WithDownloadSettings takes a per-operation snapshot without mutating a
// service that may already be downloading on behalf of another install.
func (b *BMCLAPIService) WithDownloadSettings(cfg DownloadSettings) *BMCLAPIService {
	svc := *b
	svc.assetWorkers = cfg.Concurrency
	switch cfg.Source {
	case sourceOfficial:
		svc.baseURL, svc.officialShape = "", true
	case sourceCustom:
		svc.baseURL, svc.officialShape = cfg.CustomURL, cfg.CustomShape == sourceOfficial
	}
	return &svc
}

func (b *BMCLAPIService) versionListURL() string {
	if !b.officialShape {
		return b.baseURL + "/mc/game/version_manifest.json"
	}
	base := b.baseURL
	if base == "" {
		base = officialMetaURL
	}
	return base + "/mc/game/version_manifest_v2.json"
}

func (b *BMCLAPIService) assetURL(relativePath string) string {
	if !b.officialShape {
		return b.baseURL + "/assets" + relativePath
	}
	if b.baseURL == "" {
		return officialAssetsURL + relativePath
	}
	return b.baseURL + relativePath
}

func (b *BMCLAPIService) rewriteGameURL(raw string) string {
	if b.baseURL == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return raw
	}
	var prefix string
	switch strings.ToLower(u.Hostname()) {
	case "piston-meta.mojang.com", "launchermeta.mojang.com", "piston-data.mojang.com", "launcher.mojang.com":
	case "libraries.minecraft.net":
		if !b.officialShape {
			prefix = "/maven"
		}
	case "resources.download.minecraft.net":
		if !b.officialShape {
			prefix = "/assets"
		}
	default:
		return raw
	}
	return b.baseURL + prefix + u.RequestURI()
}

func (b *BMCLAPIService) fabricMetaBase() string {
	if b.officialShape {
		return fabricMetaURL
	}
	return b.baseURL + "/fabric-meta/v2"
}

func (b *BMCLAPIService) forgeMavenBase() string {
	if b.officialShape {
		return forgeMavenURL
	}
	return b.baseURL + "/maven"
}

func (b *BMCLAPIService) officialForgeVersions(mcVersion string) *models.ApiResponse {
	resp, err := b.client.Get(b.forgeMavenBase() + "/net/minecraftforge/forge/maven-metadata.xml")
	if err != nil {
		return &models.ApiResponse{Error: err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &models.ApiResponse{Error: fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}
	var metadata struct {
		Versions []string `xml:"versioning>versions>version"`
	}
	if err := xml.NewDecoder(resp.Body).Decode(&metadata); err != nil {
		return &models.ApiResponse{Error: err.Error()}
	}
	versions := make([]map[string]interface{}, 0)
	for _, coordinate := range metadata.Versions {
		if version, ok := strings.CutPrefix(coordinate, mcVersion+"-"); ok && version != "" {
			versions = append(versions, map[string]interface{}{"version": version})
		}
	}
	return &models.ApiResponse{Success: true, Data: versions}
}
