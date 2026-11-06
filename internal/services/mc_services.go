package services

import (
	"archive/zip"
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"HRPMLW/internal/models"
	"HRPMLW/internal/repository"
)

// ==================== Authlib Injector Service ====================

// AuthService handles Minecraft authentication
type AuthService struct {
	client *http.Client
}

// NewAuthService creates a new AuthService
func NewAuthService() *AuthService {
	return &AuthService{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// AuthlibServerMeta represents authlib-injector server metadata.
//
// Per the authlib-injector wiki (API Metadata Retrieval, GET /), the response
// nests serverName / implementationName / implementationVersion / links inside
// a `meta` object, while signaturePublickey and skinDomains live at the top
// level. The `meta` object itself is free-form; only the keys below are
// recognised.
type AuthlibServerMeta struct {
	Meta struct {
		ServerName            string `json:"serverName"`
		ImplementationName    string `json:"implementationName"`
		ImplementationVersion string `json:"implementationVersion"`
		Links                 struct {
			Homepage string `json:"homepage"`
			Register string `json:"register"`
		} `json:"links"`
		// Feature flags live in `meta` and use dotted keys, e.g.
		// "feature.non_email_login". We expose the most relevant one and
		// leave the rest as a passthrough map.
		Features map[string]bool `json:"-"`
	} `json:"meta"`
	SignaturePublickey string   `json:"signaturePublickey"`
	SkinDomains        []string `json:"skinDomains"`
}

// AuthlibAuthRequest represents authentication request to authlib-injector
//
// Per the wiki, the request body MUST include `agent: {name, version}` and
// `requestUser` (set to true so the launcher can update the stored user ID /
// properties per the launcher technical spec).
type AuthlibAuthRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	ClientToken string `json:"clientToken,omitempty"`
	RequestUser bool   `json:"requestUser"`
	Agent       struct {
		Name    string `json:"name"`
		Version int    `json:"version"`
	} `json:"agent"`
}

// AuthlibAuthResponse represents authentication response from authlib-injector
type AuthlibAuthResponse struct {
	AccessToken       string           `json:"accessToken"`
	ClientToken       string           `json:"clientToken"`
	AvailableProfiles []AuthlibProfile `json:"availableProfiles"`
	SelectedProfile   AuthlibProfile   `json:"selectedProfile"`
	User              *AuthlibUser     `json:"user,omitempty"`
}

// AuthlibProfile represents a Minecraft profile (player character).
//
// Per the wiki, `properties` (and per-property `signature`) is included only in
// specific cases (e.g. refresh responses, profile lookups), so it is optional.
type AuthlibProfile struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Properties []AuthlibProperty `json:"properties,omitempty"`
}

// AuthlibProperty is a single name/value (and optional signature) entry of a
// profile or user `properties` array.
type AuthlibProperty struct {
	Name      string `json:"name"`
	Value     string `json:"value"`
	Signature string `json:"signature,omitempty"`
}

type AuthlibUser struct {
	ID         string            `json:"id"`
	Properties []AuthlibProperty `json:"properties"`
}

// getMetaWithRedirects performs a GET to metaURL, following the authlib
// API Location Indication (ALI) protocol:
//
//  1. If the response carries an X-Authlib-Injector-API-Location header, the
//     value (absolute or relative) becomes the new API URL.
//  2. Otherwise, the response body itself is the metadata and metaURL is the
//     resolved API URL.
//
// The returned raw JSON is the metadata body the caller should pass to
// -Dauthlibinjector.yggdrasil.prefetched (Base64-encoded) at launch time.
func (a *AuthService) getMetaWithRedirects(metaURL string) (*AuthlibServerMeta, []byte, string, error) {
	resp, err := a.client.Get(metaURL)
	if err != nil {
		return nil, nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil, "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	if ali := resp.Header.Get("X-Authlib-Injector-API-Location"); ali != "" {
		newURL, err := resolveALI(metaURL, ali)
		if err != nil {
			return nil, nil, "", err
		}
		// Re-fetch from the resolved URL so the caller gets the real
		// metadata body, not the homepage.
		if newURL != metaURL {
			return a.getMetaWithRedirects(newURL)
		}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, "", err
	}

	var meta AuthlibServerMeta
	if err := json.Unmarshal(body, &meta); err != nil {
		return nil, nil, "", err
	}
	return &meta, body, metaURL, nil
}

// resolveALI converts an X-Authlib-Injector-API-Location header value into an
// absolute URL, relative to the request URL.
func resolveALI(requestURL, ali string) (string, error) {
	ali = strings.TrimSpace(ali)
	if ali == "" {
		return requestURL, nil
	}
	if strings.HasPrefix(ali, "http://") || strings.HasPrefix(ali, "https://") {
		return ali, nil
	}
	base, err := url.Parse(requestURL)
	if err != nil {
		return "", err
	}
	ref, err := url.Parse(ali)
	if err != nil {
		return "", err
	}
	return base.ResolveReference(ref).String(), nil
}

// GetAuthlibMeta fetches authlib-injector server metadata.
//
// Returns the decoded struct, the raw JSON body (so the launcher can pass it
// to -Dauthlibinjector.yggdrasil.prefetched) and the resolved API URL.
func (a *AuthService) GetAuthlibMeta(authServerURL string) *models.ApiResponse {
	metaURL := strings.TrimSuffix(authServerURL, "/") + "/"

	meta, rawBody, resolvedURL, err := a.getMetaWithRedirects(metaURL)
	if err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}

	return &models.ApiResponse{
		Success: true,
		Data: map[string]interface{}{
			"meta":         meta,
			"raw":          string(rawBody),
			"resolved_url": resolvedURL,
		},
	}
}

// AuthenticateWithAuthlib authenticates with an authlib-injector server
//
// Per the wiki, the request MUST include `agent: {name:"Minecraft", version:1}`
// and `requestUser: true` so the launcher receives the user object.
func (a *AuthService) AuthenticateWithAuthlib(authServerURL, username, password, clientToken string) *models.ApiResponse {
	url := strings.TrimSuffix(authServerURL, "/") + "/authserver/authenticate"

	reqBody := AuthlibAuthRequest{
		Username:    username,
		Password:    password,
		ClientToken: clientToken,
		RequestUser: true,
	}
	reqBody.Agent.Name = "Minecraft"
	reqBody.Agent.Version = 1

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}

	req, err := http.NewRequest("POST", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := a.client.Do(req)
	if err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&errResp)
		if errMsg, ok := errResp["errorMessage"].(string); ok {
			return &models.ApiResponse{Success: false, Error: errMsg}
		}
		return &models.ApiResponse{Success: false, Error: fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}

	var authResp AuthlibAuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}

	return &models.ApiResponse{Success: true, Data: authResp}
}

// RefreshAuthlibTokenWithProfile refreshes an authlib-injector access token.
//
// Per the wiki, the refresh request:
//   - sets `requestUser: true` so the launcher receives the user object
//   - may include `selectedProfile` to perform a profile-selection refresh
//     (required when the user has multiple profiles and login returned no
//     selectedProfile)
//
// If selectedProfileID is non-empty it is sent as `selectedProfile.id` so
// the server binds the new token to that profile; pass "" to omit it.
func (a *AuthService) RefreshAuthlibTokenWithProfile(authServerURL, accessToken, clientToken, selectedProfileID string) *models.ApiResponse {
	url := strings.TrimSuffix(authServerURL, "/") + "/authserver/refresh"

	reqBody := map[string]interface{}{
		"accessToken": accessToken,
		"clientToken": clientToken,
		"requestUser": true,
	}
	if selectedProfileID != "" {
		reqBody["selectedProfile"] = map[string]string{"id": selectedProfileID}
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}

	req, err := http.NewRequest("POST", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := a.client.Do(req)
	if err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&errResp)
		if errMsg, ok := errResp["errorMessage"].(string); ok {
			return &models.ApiResponse{Success: false, Error: errMsg}
		}
		return &models.ApiResponse{Success: false, Error: fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}

	var authResp AuthlibAuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}

	return &models.ApiResponse{Success: true, Data: authResp}
}

// ValidateAuthlibToken calls POST /authserver/validate to check whether a
// stored accessToken (and optional clientToken) is still valid. The wiki
// requires a 204 No Content response on success; any other status means the
// token is invalid. This is a prerequisite of the launcher's token
// validity-check flow described in the launcher technical spec.
func (a *AuthService) ValidateAuthlibToken(authServerURL, accessToken, clientToken string) *models.ApiResponse {
	url := strings.TrimSuffix(authServerURL, "/") + "/authserver/validate"

	reqBody := map[string]string{
		"accessToken": accessToken,
	}
	if clientToken != "" {
		reqBody["clientToken"] = clientToken
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}

	req, err := http.NewRequest("POST", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := a.client.Do(req)
	if err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNoContent:
		return &models.ApiResponse{Success: true}
	case http.StatusForbidden:
		return &models.ApiResponse{Success: false, Error: "Invalid token"}
	default:
		var errResp map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&errResp)
		if errMsg, ok := errResp["errorMessage"].(string); ok && errMsg != "" {
			return &models.ApiResponse{Success: false, Error: errMsg}
		}
		return &models.ApiResponse{Success: false, Error: fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}
}

// ==================== BMCLAPI Service ====================

// BMCLAPIService handles BMCLAPI downloads
type BMCLAPIService struct {
	client  *http.Client
	mirrors []string
	baseURL string
}

// userAgentTransport gives mirror requests a descriptive User-Agent. Some
// CDNs in the BMCLAPI redirect chain answer Go's default client UA with 403
// (observed live on mirrors.ustc.edu.cn), which broke library downloads.
type userAgentTransport struct {
	base http.RoundTripper
}

func (t userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.UserAgent() == "" {
		req = req.Clone(req.Context())
		req.Header.Set("User-Agent", "SamuelClient/1.0 (Minecraft launcher)")
	}
	return t.base.RoundTrip(req)
}

// NewBMCLAPIService creates a new BMCLAPIService
func NewBMCLAPIService() *BMCLAPIService {
	return &BMCLAPIService{
		client: &http.Client{
			Timeout:   60 * time.Second,
			Transport: userAgentTransport{base: http.DefaultTransport},
		},
		mirrors: []string{"https://bmclapi2.bangbang93.com", "https://bmclapi.ggsdream.com"},
		baseURL: "https://bmclapi2.bangbang93.com",
	}
}

// NewBMCLAPIServiceWithBaseURL creates a BMCLAPIService against a custom
// mirror base URL. It exists so tests can point the client at an httptest
// server (no network access), and so a future mirror configuration
// (LauncherConfig.BMCLAPIMirrors) can reuse it.
func NewBMCLAPIServiceWithBaseURL(baseURL string) *BMCLAPIService {
	svc := NewBMCLAPIService()
	svc.baseURL = baseURL
	return svc
}

// BMCLVersionManifestResponse represents the version manifest response from BMCLAPI
type BMCLVersionManifestResponse struct {
	Latest   map[string]string `json:"latest"`
	Versions []BMCLVersion     `json:"versions"`
}

type BMCLVersion struct {
	ID              string `json:"id"`
	Type            string `json:"type"`
	URL             string `json:"url"`
	Time            string `json:"time"`
	ReleaseTime     string `json:"releaseTime"`
	Sha1            string `json:"sha1"`
	ComplianceLevel int    `json:"complianceLevel"`
}

// BMCLVersionJSON represents version.json
type BMCLVersionJSON struct {
	ID                     string                  `json:"id"`
	InheritsFrom           string                  `json:"inheritsFrom,omitempty"`
	AssetIndex             BMCLAssetIndex          `json:"assetIndex"`
	Assets                 string                  `json:"assets"`
	ComplianceLevel        int                     `json:"complianceLevel"`
	Downloads              map[string]BMCLDownload `json:"downloads"`
	JavaVersion            BMCLJavaVersion         `json:"javaVersion"`
	Libraries              []BMCLLibrary           `json:"libraries"`
	MainClass              string                  `json:"mainClass"`
	MinecraftArguments     string                  `json:"minecraftArguments,omitempty"`
	Arguments              BMCLArguments           `json:"arguments"`
	MinimumLauncherVersion int                     `json:"minimumLauncherVersion"`
	ReleaseTime            string                  `json:"releaseTime"`
	Time                   string                  `json:"time"`
	Type                   string                  `json:"type"`
}

type BMCLAssetIndex struct {
	ID   string `json:"id"`
	URL  string `json:"url"`
	SHA1 string `json:"sha1"`
	Size int64  `json:"size"`
}

type BMCLDownload struct {
	SHA1 string `json:"sha1"`
	URL  string `json:"url"`
	Size int64  `json:"size"`
}

type BMCLJavaVersion struct {
	Component string `json:"component"`
	SHA1      string `json:"sha1"`
	Size      int64  `json:"size"`
	URL       string `json:"url"`
}

type BMCLLibrary struct {
	Name      string               `json:"name"`
	Downloads BMCLLibraryDownloads `json:"downloads"`
	Rules     []BMCLRule           `json:"rules,omitempty"`
	// URL is set by Fabric meta profiles: a maven repository base URL at the
	// library level (no downloads object). fillMavenURLs resolves it.
	URL string `json:"url,omitempty"`
}

type BMCLLibraryDownloads struct {
	Artifact    BMCLArtifact            `json:"artifact"`
	Classifiers map[string]BMCLArtifact `json:"classifiers,omitempty"`
}

type BMCLArtifact struct {
	Path string `json:"path"`
	URL  string `json:"url"`
	SHA1 string `json:"sha1"`
	Size int64  `json:"size"`
}

type BMCLArguments struct {
	Game []interface{} `json:"game"`
	JVM  []interface{} `json:"jvm"`
}

type BMCLRule struct {
	Action string  `json:"action"`
	OS     *BMCLOS `json:"os,omitempty"`
}

type BMCLOS struct {
	Name string `json:"name"`
}

// GetVersionList returns all available Minecraft versions
// BMCLAPI endpoint: /mc/game/version_manifest.json
func (b *BMCLAPIService) GetVersionList() *models.ApiResponse {
	url := b.baseURL + "/mc/game/version_manifest.json"

	resp, err := b.client.Get(url)
	if err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &models.ApiResponse{Success: false, Error: fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}

	var manifest BMCLVersionManifestResponse
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}

	return &models.ApiResponse{Success: true, Data: manifest}
}

// toBMCLAPI rewrites an official Mojang resource URL to the equivalent
// BMCLAPI mirror path. It covers both the legacy (launchermeta / launcher)
// and the current (piston-meta / piston-data) hosts plus the assets and
// maven hosts. See https://bmclapi.bangbang93.com/
func (b *BMCLAPIService) toBMCLAPI(u string) string {
	u = strings.ReplaceAll(u, "https://piston-meta.mojang.com/", b.baseURL+"/")
	u = strings.ReplaceAll(u, "https://launchermeta.mojang.com/", b.baseURL+"/")
	u = strings.ReplaceAll(u, "https://piston-data.mojang.com/", b.baseURL+"/")
	u = strings.ReplaceAll(u, "https://launcher.mojang.com/", b.baseURL+"/")
	u = strings.ReplaceAll(u, "https://resources.download.minecraft.net/", b.baseURL+"/assets/")
	u = strings.ReplaceAll(u, "https://libraries.minecraft.net/", b.baseURL+"/maven/")
	return u
}

// toBMCLAPIMaven rewrites a Mojang library URL to the BMCLAPI maven mirror.
// It is a thin wrapper kept for clarity at call sites.
func (b *BMCLAPIService) toBMCLAPIMaven(u string) string {
	return strings.ReplaceAll(u, "https://libraries.minecraft.net/", b.baseURL+"/maven/")
}

// GetVersionManifest returns version JSON for a specific version
// The version URL from manifest should be replaced with BMCLAPI domain
func (b *BMCLAPIService) GetVersionManifest(versionURL string) *models.ApiResponse {
	bmclURL := b.toBMCLAPI(versionURL)

	resp, err := b.client.Get(bmclURL)
	if err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &models.ApiResponse{Success: false, Error: fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}

	var versionJSON BMCLVersionJSON
	if err := json.NewDecoder(resp.Body).Decode(&versionJSON); err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}

	return &models.ApiResponse{Success: true, Data: versionJSON}
}

// GetVersionJAR returns download URL for version JAR from version JSON downloads
func (b *BMCLAPIService) GetVersionJAR(versionJSON *BMCLVersionJSON) *models.ApiResponse {
	if versionJSON.Downloads == nil {
		return &models.ApiResponse{Success: false, Error: "No downloads found in version JSON"}
	}

	clientDownload, ok := versionJSON.Downloads["client"]
	if !ok {
		return &models.ApiResponse{Success: false, Error: "No client download found"}
	}

	// Rewrite to BMCLAPI mirror (handles piston-data and legacy launcher)
	url := b.toBMCLAPI(clientDownload.URL)

	return &models.ApiResponse{Success: true, Data: map[string]interface{}{
		"url":  url,
		"sha1": clientDownload.SHA1,
		"size": clientDownload.Size,
	}}
}

// DownloadFile downloads a file with progress callback
func (b *BMCLAPIService) DownloadFile(url, destPath string, expectedSize int64, progressCallback func(downloaded, total int64)) *models.ApiResponse {
	// Create directory if not exists
	dir := filepath.Dir(destPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}

	resp, err := b.client.Get(url)
	if err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &models.ApiResponse{Success: false, Error: fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}

	totalSize := expectedSize
	if totalSize == 0 {
		totalSize = resp.ContentLength
	}

	file, err := os.Create(destPath)
	if err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}
	// A failed download must not leave a truncated file behind: callers such
	// as DownloadLibraries/DownloadAssets treat an existing file as already
	// downloaded and would skip it forever.
	failed := true
	defer func() {
		file.Close()
		if failed {
			os.Remove(destPath)
		}
	}()

	downloaded := int64(0)
	buf := make([]byte, 32*1024)
	reader := bufio.NewReader(resp.Body)

	for {
		n, err := reader.Read(buf)
		if n > 0 {
			file.Write(buf[:n])
			downloaded += int64(n)
			if progressCallback != nil {
				progressCallback(downloaded, totalSize)
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return &models.ApiResponse{Success: false, Error: err.Error()}
		}
	}

	failed = false
	return &models.ApiResponse{Success: true, Data: map[string]interface{}{
		"path": destPath,
		"size": downloaded,
	}}
}

// DownloadLibraries downloads all required libraries
// BMCLAPI: https://libraries.minecraft.net/ -> https://bmclapi2.bangbang93.com/maven
func (b *BMCLAPIService) DownloadLibraries(libraries []BMCLLibrary, libsDir string, progressCallback func(name string, downloaded, total int64)) *models.ApiResponse {
	var lastErr error
	for _, lib := range libraries {
		if !shouldDownloadLibrary(lib) {
			continue
		}

		artifact := lib.Downloads.Artifact
		// Rewrite official library URL to BMCLAPI maven mirror
		url := b.toBMCLAPIMaven(artifact.URL)

		parts := strings.Split(lib.Name, ":")
		if len(parts) != 3 {
			continue
		}
		group := strings.ReplaceAll(parts[0], ".", "/")
		artifactPath := filepath.Join(libsDir, group, parts[1], parts[2], parts[1]+"-"+parts[2]+".jar")

		if _, err := os.Stat(artifactPath); err == nil {
			continue // Already exists
		}

		result := b.DownloadFile(url, artifactPath, artifact.Size, nil)
		if !result.Success {
			lastErr = fmt.Errorf("failed to download %s: %s", lib.Name, result.Error)
			continue // Try next library instead of failing completely
		}

		if progressCallback != nil {
			progressCallback(lib.Name, artifact.Size, artifact.Size)
		}
	}

	if lastErr != nil {
		return &models.ApiResponse{Success: false, Error: lastErr.Error()}
	}
	return &models.ApiResponse{Success: true}
}

func shouldDownloadLibrary(lib BMCLLibrary) bool {
	if len(lib.Rules) == 0 {
		return true
	}

	allowed := false
	for _, rule := range lib.Rules {
		if rule.Action == "allow" {
			if rule.OS == nil || rule.OS.Name == runtime.GOOS {
				allowed = true
			}
		} else if rule.Action == "disallow" {
			if rule.OS != nil && rule.OS.Name == runtime.GOOS {
				return false
			}
		}
	}

	return allowed
}

// GetFabricVersions returns available Fabric versions
// BMCLAPI: https://meta.fabricmc.net -> https://bmclapi2.bangbang93.com/fabric-meta
func (b *BMCLAPIService) GetFabricVersions(mcVersion string) *models.ApiResponse {
	url := b.baseURL + "/fabric-meta/v2/versions/loader/" + mcVersion

	resp, err := b.client.Get(url)
	if err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &models.ApiResponse{Success: false, Error: fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}

	var versions []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&versions); err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}

	return &models.ApiResponse{Success: true, Data: versions}
}

// GetFabricInstaller downloads Fabric installer
// BMCLAPI: https://meta.fabricmc.net -> https://bmclapi2.bangbang93.com/fabric-meta
func (b *BMCLAPIService) GetFabricInstaller(mcVersion, loaderVersion, targetDir string) *models.ApiResponse {
	url := fmt.Sprintf("%s/fabric-meta/v2/versions/loader/%s/%s/profile/jar", b.baseURL, mcVersion, loaderVersion)

	resp, err := b.client.Get(url)
	if err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &models.ApiResponse{Success: false, Error: fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}

	destPath := filepath.Join(targetDir, "fabric-loader-"+loaderVersion+".jar")
	file, err := os.Create(destPath)
	if err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}
	failed := true
	defer func() {
		file.Close()
		if failed {
			os.Remove(destPath)
		}
	}()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}

	failed = false
	return &models.ApiResponse{Success: true, Data: map[string]string{"path": destPath}}
}

// GetForgeVersions returns available Forge versions for a Minecraft version
func (b *BMCLAPIService) GetForgeVersions(mcVersion string) *models.ApiResponse {
	url := b.baseURL + "/forge/minecraft/" + mcVersion

	resp, err := b.client.Get(url)
	if err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &models.ApiResponse{Success: false, Error: fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}

	var versions []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&versions); err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}

	return &models.ApiResponse{Success: true, Data: versions}
}

// DownloadForgeInstaller downloads Forge installer
// BMCLAPI serves the installer through its maven mirror (the historical
// /forge/minecraft/<mc>/<ver>/installer endpoint 404s; verified live).
func (b *BMCLAPIService) DownloadForgeInstaller(mcVersion, forgeVersion, targetDir string) *models.ApiResponse {
	url := fmt.Sprintf("%s/maven/net/minecraftforge/forge/%s-%s/forge-%s-%s-installer.jar", b.baseURL, mcVersion, forgeVersion, mcVersion, forgeVersion)

	resp, err := b.client.Get(url)
	if err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &models.ApiResponse{Success: false, Error: fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}

	destPath := filepath.Join(targetDir, "forge-installer-"+forgeVersion+".jar")
	file, err := os.Create(destPath)
	if err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}
	failed := true
	defer func() {
		file.Close()
		if failed {
			os.Remove(destPath)
		}
	}()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}

	failed = false
	return &models.ApiResponse{Success: true, Data: map[string]string{"path": destPath}}
}

// InstallFabricLoader installs the Fabric loader on top of an already
// installed vanilla version:
//  1. requires <gameDir>/versions/<mcVersion>/<mcVersion>.json (the parent)
//  2. fetches the loader profile JSON from the (mirrored) Fabric meta API
//  3. writes it to <gameDir>/versions/<profile id>/<profile id>.json — the
//     profile keeps inheritsFrom, resolved by BuildLaunchCommand at launch
//  4. downloads the loader libraries into <gameDir>/libraries
//
// progress, when non-nil, is invoked with stage "loader" (the
// download:progress contract). Failure removes a freshly created version
// directory again (same semantics as InstallVersion); a pre-existing one is
// left untouched so a retry cannot destroy a working loader install.
func (b *BMCLAPIService) InstallFabricLoader(mcVersion, loaderVersion, gameDir string, progress func(stage, name string, downloaded, total int64)) *models.ApiResponse {
	parentJSONPath := filepath.Join(gameDir, "versions", mcVersion, mcVersion+".json")
	if _, err := os.Stat(parentJSONPath); err != nil {
		return &models.ApiResponse{Success: false, Error: fmt.Sprintf("version %s is not installed; install the vanilla version first", mcVersion)}
	}

	profileURL := fmt.Sprintf("%s/fabric-meta/v2/versions/loader/%s/%s/profile/json", b.baseURL, mcVersion, loaderVersion)
	resp, err := b.client.Get(profileURL)
	if err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &models.ApiResponse{Success: false, Error: fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}

	var profile BMCLVersionJSON
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return &models.ApiResponse{Success: false, Error: "parse fabric profile: " + err.Error()}
	}
	if profile.ID == "" {
		profile.ID = fmt.Sprintf("fabric-loader-%s-%s", loaderVersion, mcVersion)
	}
	if profile.InheritsFrom == "" {
		profile.InheritsFrom = mcVersion
	}

	versionDir := filepath.Join(gameDir, "versions", profile.ID)
	_, statErr := os.Stat(versionDir)
	preExisting := statErr == nil
	fail := func(resp *models.ApiResponse) *models.ApiResponse {
		if !preExisting {
			_ = os.RemoveAll(versionDir)
		}
		return resp
	}
	if err := os.MkdirAll(versionDir, 0755); err != nil {
		return fail(&models.ApiResponse{Success: false, Error: fmt.Sprintf("create %s: %v", versionDir, err)})
	}

	jsonData, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return fail(&models.ApiResponse{Success: false, Error: "marshal fabric profile: " + err.Error()})
	}
	versionJSONPath := filepath.Join(versionDir, profile.ID+".json")
	if err := os.WriteFile(versionJSONPath, jsonData, 0644); err != nil {
		return fail(&models.ApiResponse{Success: false, Error: "write fabric profile: " + err.Error()})
	}

	var libProgress func(name string, downloaded, total int64)
	if progress != nil {
		libProgress = func(name string, downloaded, total int64) { progress("loader", name, downloaded, total) }
	}
	// Fabric meta omits artifact URLs for its own maven artifacts; derive
	// them from the maven coordinates (verified against the live API).
	b.fillMavenURLs(profile.Libraries)
	libDownload := b.DownloadLibraries(profile.Libraries, filepath.Join(gameDir, "libraries"), libProgress)
	if !libDownload.Success {
		return fail(libDownload)
	}

	// Register the loader version in launcher_profiles.json so the install is
	// visible exactly like the official launcher's entries.
	if err := upsertLauncherProfile(gameDir, profile.ID); err != nil {
		return fail(&models.ApiResponse{Success: false, Error: "write launcher_profiles.json: " + err.Error()})
	}

	return &models.ApiResponse{Success: true, Data: map[string]interface{}{
		"version_id":    profile.ID,
		"version_dir":   versionDir,
		"version_json":  versionJSONPath,
		"inherits_from": profile.InheritsFrom,
	}}
}

// fillMavenURLs makes sure every library carries a downloadable artifact
// URL. The live Fabric meta API returns libraries without a downloads
// object, or with `url` set to a maven repository base (e.g.
// https://maven.fabricmc.net/) instead of a file URL; both forms are
// resolved to the maven layout derived from the coordinates, served from
// the BMCLAPI mirror (verified live).
func (b *BMCLAPIService) fillMavenURLs(libraries []BMCLLibrary) {
	for i := range libraries {
		artifact := &libraries[i].Downloads.Artifact
		parts := strings.Split(libraries[i].Name, ":")
		if len(parts) != 3 {
			continue
		}
		if artifact.Path == "" {
			artifact.Path = strings.ReplaceAll(parts[0], ".", "/") + "/" + parts[1] + "/" + parts[2] + "/" + parts[1] + "-" + parts[2] + ".jar"
		}
		if artifact.URL == "" && libraries[i].URL != "" {
			// Fabric meta: repository base at the library level.
			artifact.URL = libraries[i].URL
		}
		if artifact.URL == "" {
			artifact.URL = b.baseURL + "/maven/" + artifact.Path
		} else if !strings.HasSuffix(artifact.URL, ".jar") {
			base := strings.TrimRight(artifact.URL, "/")
			if base == "https://maven.fabricmc.net" {
				base = b.baseURL + "/maven"
			}
			artifact.URL = base + "/" + artifact.Path
		}
	}
}

// upsertLauncherProfile writes a profile entry for versionID into
// <gameDir>/launcher_profiles.json (creating a minimal file when missing),
// mirroring what the official launcher and HMCL keep there. The Forge
// installer injects its own entry; Fabric profiles have no installer step,
// so this launcher writes the entry itself. An existing file that cannot be
// parsed is left untouched rather than clobbered.
func upsertLauncherProfile(gameDir, versionID string) error {
	profilesPath := filepath.Join(gameDir, "launcher_profiles.json")

	root := map[string]interface{}{}
	if raw, err := os.ReadFile(profilesPath); err == nil {
		if json.Unmarshal(raw, &root) != nil {
			return nil // keep an unparseable file as-is
		}
	}
	profiles, ok := root["profiles"].(map[string]interface{})
	if !ok {
		profiles = map[string]interface{}{}
		root["profiles"] = profiles
	}
	if _, exists := profiles[versionID]; exists {
		return nil
	}
	profiles[versionID] = map[string]interface{}{
		"name":          versionID,
		"type":          "custom",
		"lastVersionId": versionID,
	}

	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(profilesPath, data, 0644)
}

// launcherProfileExists reports whether launcher_profiles.json already
// contains an entry whose lastVersionId is versionID (the Forge installer
// keys its entry "forge" rather than by the version id).
func launcherProfileExists(gameDir, versionID string) bool {
	raw, err := os.ReadFile(filepath.Join(gameDir, "launcher_profiles.json"))
	if err != nil {
		return false
	}
	var root struct {
		Profiles map[string]struct {
			LastVersionID string `json:"lastVersionId"`
		} `json:"profiles"`
	}
	if json.Unmarshal(raw, &root) != nil {
		return false
	}
	for _, p := range root.Profiles {
		if p.LastVersionID == versionID {
			return true
		}
	}
	return false
}

// forgeInstallTimeout bounds the Forge installer run. It is a variable (like
// the java detection seams) so tests can shrink it; 10 minutes matches the
// slowest real-world installer runs on cold caches.
var forgeInstallTimeout = 10 * time.Minute

// InstallForgeLoader installs the Forge loader for a Minecraft version:
//  1. downloads the installer jar into gameDir (transient, always removed)
//  2. ensures <gameDir>/launcher_profiles.json exists (the installer refuses
//     to run without a launcher profile file; a minimal one is created)
//  3. runs `java -jar <installer> --installClient` headless with a timeout,
//     streaming the installer's output line-by-line to logCallback; the
//     installer targets its working directory, which is gameDir
//  4. locates the generated version directory under
//     <gameDir>/versions/<mcVersion>-forge* (the installer chooses the exact
//     id, e.g. 1.20.4-forge49.0.3) and verifies its version JSON
//
// On any failure a freshly created version directory is removed again (same
// semantics as InstallVersion). logCallback may be invoked concurrently
// (stdout and stderr are streamed in parallel). Verified against the real
// 1.20.4 / 49.0.3 installer from the BMCLAPI mirror on JDK 25.
func (b *BMCLAPIService) InstallForgeLoader(mcVersion, forgeVersion, gameDir, javaPath string, logCallback func(source, line string), progress func(stage, name string, downloaded, total int64)) *models.ApiResponse {
	installerName := "forge-installer-" + forgeVersion + ".jar"
	if progress != nil {
		progress("loader", installerName, 0, 0)
	}
	download := b.DownloadForgeInstaller(mcVersion, forgeVersion, gameDir)
	if !download.Success {
		return download
	}
	installerPath := filepath.Join(gameDir, installerName)
	// The installer jar is transient: remove it on every exit path.
	defer os.Remove(installerPath)

	// The installer requires a launcher profile file in the target directory
	// ("There is no minecraft launcher profile in ..."). Third-party
	// launchers seed a minimal one; do the same.
	profilesPath := filepath.Join(gameDir, "launcher_profiles.json")
	if _, err := os.Stat(profilesPath); err != nil {
		if err := os.WriteFile(profilesPath, []byte(`{"profiles": {}}`), 0644); err != nil {
			return &models.ApiResponse{Success: false, Error: fmt.Sprintf("create launcher_profiles.json: %v", err)}
		}
	}

	if javaPath == "" {
		javaPath = "java"
	}
	cmd := exec.Command(javaPath, "-jar", installerPath, "--installClient")
	cmd.Dir = gameDir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return &models.ApiResponse{Success: false, Error: fmt.Sprintf("stdout pipe: %v", err)}
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return &models.ApiResponse{Success: false, Error: fmt.Sprintf("stderr pipe: %v", err)}
	}
	if err := cmd.Start(); err != nil {
		return &models.ApiResponse{Success: false, Error: fmt.Sprintf("start forge installer: %v", err)}
	}

	// Stream both pipes into the log callback; both must finish before the
	// result is evaluated (mirrors the LaunchGame streaming discipline).
	streamDone := make(chan struct{})
	forward := func(r io.Reader, source string) {
		if logCallback != nil {
			streamLines(io.Discard, r, source, logCallback)
		} else {
			io.Copy(io.Discard, r)
		}
	}
	go func() {
		go func() { forward(stdout, "stdout"); close(streamDone) }()
		forward(stderr, "stderr")
	}()

	waitErr := make(chan error, 1)
	go func() { waitErr <- cmd.Wait() }()

	// The installer chooses the exact version id itself (e.g.
	// 1.20.4-forge49.0.3), so the result is discovered under
	// versions/<mcVersion>-forge* instead of guessed. Directories that
	// existed before the run are never touched by the failure cleanup.
	versionsRoot := filepath.Join(gameDir, "versions")
	preExistingDirs := map[string]bool{}
	if entries, err := os.ReadDir(versionsRoot); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				preExistingDirs[e.Name()] = true
			}
		}
	}
	fail := func(resp *models.ApiResponse) *models.ApiResponse {
		if entries, err := os.ReadDir(versionsRoot); err == nil {
			for _, e := range entries {
				if e.IsDir() && strings.HasPrefix(e.Name(), mcVersion+"-forge") && !preExistingDirs[e.Name()] {
					_ = os.RemoveAll(filepath.Join(versionsRoot, e.Name()))
				}
			}
		}
		return resp
	}

	select {
	case err := <-waitErr:
		<-streamDone
		if err != nil {
			return fail(&models.ApiResponse{Success: false, Error: fmt.Sprintf("forge installer failed: %v (see launcher:log output)", err)})
		}
	case <-time.After(forgeInstallTimeout):
		_ = cmd.Process.Kill()
		<-waitErr
		<-streamDone
		return fail(&models.ApiResponse{Success: false, Error: fmt.Sprintf("forge installer timed out after %v", forgeInstallTimeout)})
	}

	// Verify the generated version directory.
	versionID, versionDir, found := findForgeVersionDir(versionsRoot, mcVersion)
	if !found {
		return fail(&models.ApiResponse{Success: false, Error: fmt.Sprintf("forge installer did not produce a %s-forge* version directory", mcVersion)})
	}

	// The installer normally injects its own launcher_profiles.json entry;
	// if it did not, write one so the install is consistently registered.
	if !launcherProfileExists(gameDir, versionID) {
		if err := upsertLauncherProfile(gameDir, versionID); err != nil {
			return fail(&models.ApiResponse{Success: false, Error: "write launcher_profiles.json: " + err.Error()})
		}
	}

	return &models.ApiResponse{Success: true, Data: map[string]interface{}{
		"version_id":    versionID,
		"version_dir":   versionDir,
		"version_json":  filepath.Join(versionDir, versionID+".json"),
		"inherits_from": mcVersion,
	}}
}

// findForgeVersionDir locates the version directory the Forge installer
// generated: the first <mcVersion>-forge* directory containing its version
// JSON.
func findForgeVersionDir(versionsRoot, mcVersion string) (id, dir string, found bool) {
	entries, err := os.ReadDir(versionsRoot)
	if err != nil {
		return "", "", false
	}
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), mcVersion+"-forge") {
			continue
		}
		if _, err := os.Stat(filepath.Join(versionsRoot, e.Name(), e.Name()+".json")); err == nil {
			return e.Name(), filepath.Join(versionsRoot, e.Name()), true
		}
	}
	return "", "", false
}

// DownloadAssets downloads game assets
// BMCLAPI: http://resources.download.minecraft.net -> https://bmclapi2.bangbang93.com/assets
func (b *BMCLAPIService) DownloadAssets(assetIndex *BMCLAssetIndex, assetsDir string, progressCallback func(name string, downloaded, total int64)) *models.ApiResponse {
	// Download asset index JSON (rewritten to BMCLAPI mirror)
	assetIndexURL := b.toBMCLAPI(assetIndex.URL)

	resp, err := b.client.Get(assetIndexURL)
	if err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &models.ApiResponse{Success: false, Error: fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}

	var assetIndexJSON struct {
		Objects map[string]struct {
			Hash string `json:"hash"`
			Size int64  `json:"size"`
		} `json:"objects"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&assetIndexJSON); err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}

	// Download each asset
	for name, obj := range assetIndexJSON.Objects {
		hash := obj.Hash
		subDir := hash[:2]
		url := fmt.Sprintf("%s/assets/%s/%s", b.baseURL, subDir, hash)

		destPath := filepath.Join(assetsDir, "objects", subDir, hash)
		if _, err := os.Stat(destPath); err == nil {
			continue // Already exists
		}

		result := b.DownloadFile(url, destPath, obj.Size, nil)
		if !result.Success {
			return &models.ApiResponse{Success: false, Error: fmt.Sprintf("failed to download asset %s: %s", name, result.Error)}
		}

		if progressCallback != nil {
			progressCallback(name, obj.Size, obj.Size)
		}
	}

	return &models.ApiResponse{Success: true}
}

// DownloadNatives downloads native libraries for the current platform
func (b *BMCLAPIService) DownloadNatives(libraries []BMCLLibrary, nativesDir string, progressCallback func(name string, downloaded, total int64)) *models.ApiResponse {
	for _, lib := range libraries {
		if len(lib.Downloads.Classifiers) == 0 {
			continue
		}

		// Determine native classifier based on OS
		classifier := ""
		switch runtime.GOOS {
		case "windows":
			classifier = "natives-windows"
		case "darwin":
			classifier = "natives-macos"
		case "linux":
			classifier = "natives-linux"
		}

		if classifier == "" {
			continue
		}

		artifact, ok := lib.Downloads.Classifiers[classifier]
		if !ok {
			continue
		}

		url := b.toBMCLAPIMaven(artifact.URL)

		// Extract the native jar
		resp, err := b.client.Get(url)
		if err != nil {
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			continue
		}

		// Create temp file
		tmpFile, err := os.CreateTemp("", "native-*.jar")
		if err != nil {
			resp.Body.Close()
			continue
		}
		tmpPath := tmpFile.Name()

		_, err = io.Copy(tmpFile, resp.Body)
		tmpFile.Close()
		resp.Body.Close()

		if err != nil {
			os.Remove(tmpPath)
			continue
		}

		// Extract native files
		err = b.extractNatives(tmpPath, nativesDir)
		os.Remove(tmpPath)

		if err != nil {
			continue
		}

		if progressCallback != nil {
			progressCallback(lib.Name, artifact.Size, artifact.Size)
		}
	}

	return &models.ApiResponse{Success: true}
}

// extractNatives extracts native libraries from a jar file
func (b *BMCLAPIService) extractNatives(jarPath, destDir string) error {
	r, err := zip.OpenReader(jarPath)
	if err != nil {
		return err
	}
	defer r.Close()

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	for _, f := range r.File {
		// Skip directories and META-INF
		if f.FileInfo().IsDir() || strings.HasPrefix(f.Name, "META-INF/") {
			continue
		}

		// Only extract native files (.dll, .so, .dylib, .jnilib)
		ext := strings.ToLower(filepath.Ext(f.Name))
		if ext != ".dll" && ext != ".so" && ext != ".dylib" && ext != ".jnilib" {
			continue
		}

		destPath := filepath.Join(destDir, filepath.Base(f.Name))

		rc, err := f.Open()
		if err != nil {
			continue
		}

		destFile, err := os.Create(destPath)
		if err != nil {
			rc.Close()
			continue
		}

		io.Copy(destFile, rc)
		destFile.Close()
		rc.Close()
	}

	return nil
}

// InstallVersion performs a full install of a Minecraft version per the
// launcher standard. It is the service-layer orchestration that previously
// lived inside the HTTP handler (ADR-0004):
//  1. writes <gameDir>/versions/<id>/<id>.json (required by BuildLaunchCommand)
//  2. downloads the version JAR
//  3. downloads every library listed in the version JSON
//  4. extracts natives to <gameDir>/versions/<id>/natives
//  5. downloads the asset objects into <gameDir>/assets (the --assetsDir the
//     launch command passes; without them the game has no textures/sounds)
//
// progress, when non-nil, is invoked as each stage advances so callers can
// surface live progress. stage is one of "jar", "library", "native" or
// "asset"; name identifies the artifact; downloaded/total are byte counts.
// The Wails binding forwards these as "download:progress" events.
func (b *BMCLAPIService) InstallVersion(versionID, gameDir string, progress func(stage, name string, downloaded, total int64)) *models.ApiResponse {
	// First get the version list to find the version URL
	listResult := b.GetVersionList()
	if !listResult.Success {
		return listResult
	}

	manifest, ok := listResult.Data.(BMCLVersionManifestResponse)
	if !ok {
		return &models.ApiResponse{Success: false, Error: "failed to parse version list"}
	}

	// Find the version URL
	var versionURL string
	for _, v := range manifest.Versions {
		if v.ID == versionID {
			versionURL = v.URL
			break
		}
	}

	if versionURL == "" {
		return &models.ApiResponse{Success: false, Error: "version not found"}
	}

	// Get version manifest
	manifestResult := b.GetVersionManifest(versionURL)
	if !manifestResult.Success {
		return manifestResult
	}

	versionJSON, ok := manifestResult.Data.(BMCLVersionJSON)
	if !ok {
		return &models.ApiResponse{Success: false, Error: "failed to parse manifest"}
	}

	versionDir := gameDir + "/versions/" + versionID
	jarPath := versionDir + "/" + versionID + ".jar"
	versionJSONPath := versionDir + "/" + versionID + ".json"
	nativesDir := versionDir + "/natives"
	librariesDir := gameDir + "/libraries"

	// A failed install must not leave an orphan <id>.json (or partial jar)
	// behind, so the version directory is removed again on any later failure.
	// A pre-existing directory (re-install over a working copy) is kept, so a
	// transient download error cannot destroy a playable install. Shared
	// libraries under librariesDir are intentionally untouched: they may be
	// used by other installed versions.
	_, statErr := os.Stat(versionDir)
	preExisting := statErr == nil
	fail := func(resp *models.ApiResponse) *models.ApiResponse {
		if !preExisting {
			_ = os.RemoveAll(versionDir)
		}
		return resp
	}

	for _, dir := range []string{versionDir, librariesDir, nativesDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fail(&models.ApiResponse{Success: false, Error: fmt.Sprintf("create %s: %v", dir, err)})
		}
	}

	// 1. Persist the version JSON. This is the file BuildLaunchCommand reads to
	//    obtain mainClass, assetIndex, libraries, etc. Without it the game
	//    cannot be launched.
	jsonData, err := json.MarshalIndent(versionJSON, "", "  ")
	if err != nil {
		return fail(&models.ApiResponse{Success: false, Error: "marshal version json: " + err.Error()})
	}
	if err := os.WriteFile(versionJSONPath, jsonData, 0644); err != nil {
		return fail(&models.ApiResponse{Success: false, Error: "write version json: " + err.Error()})
	}

	// 2. Download the version JAR
	jarResult := b.GetVersionJAR(&versionJSON)
	if !jarResult.Success {
		return fail(jarResult)
	}

	jarData, ok := jarResult.Data.(map[string]interface{})
	if !ok {
		return fail(&models.ApiResponse{Success: false, Error: "failed to parse JAR data"})
	}

	jarURL, ok := jarData["url"].(string)
	if !ok {
		return fail(&models.ApiResponse{Success: false, Error: "JAR data missing url"})
	}
	jarSize, ok := jarData["size"].(int64)
	if !ok {
		return fail(&models.ApiResponse{Success: false, Error: "JAR data missing size"})
	}

	var jarProgress func(downloaded, total int64)
	if progress != nil {
		jarProgress = func(downloaded, total int64) { progress("jar", versionID+".jar", downloaded, total) }
	}
	jarDownload := b.DownloadFile(jarURL, jarPath, jarSize, jarProgress)
	if !jarDownload.Success {
		return fail(jarDownload)
	}

	// 3. Download every library. Required for the -cp classpath the launcher
	//    builds in BuildLaunchCommand.
	var libProgress func(name string, downloaded, total int64)
	if progress != nil {
		libProgress = func(name string, downloaded, total int64) { progress("library", name, downloaded, total) }
	}
	libDownload := b.DownloadLibraries(versionJSON.Libraries, librariesDir, libProgress)
	if !libDownload.Success {
		return fail(libDownload)
	}

	// 4. Extract natives (used by -Djava.library.path).
	var nativeProgress func(name string, downloaded, total int64)
	if progress != nil {
		nativeProgress = func(name string, downloaded, total int64) { progress("native", name, downloaded, total) }
	}
	nativesDownload := b.DownloadNatives(versionJSON.Libraries, nativesDir, nativeProgress)
	if !nativesDownload.Success {
		return fail(nativesDownload)
	}

	// 5. Download assets into <gameDir>/assets. BuildLaunchCommand points
	//    --assetsDir there; without the objects the game starts but has no
	//    textures or sounds. Versions whose manifest carries no asset index
	//    URL skip this stage.
	assetsDir := gameDir + "/assets"
	if versionJSON.AssetIndex.URL != "" {
		var assetProgress func(name string, downloaded, total int64)
		if progress != nil {
			assetProgress = func(name string, downloaded, total int64) { progress("asset", name, downloaded, total) }
		}
		assetsDownload := b.DownloadAssets(&versionJSON.AssetIndex, assetsDir, assetProgress)
		if !assetsDownload.Success {
			return fail(assetsDownload)
		}
	}

	return &models.ApiResponse{
		Success: true,
		Data: map[string]interface{}{
			"version_id":    versionID,
			"version_dir":   versionDir,
			"version_json":  versionJSONPath,
			"version_jar":   jarPath,
			"libraries_dir": librariesDir,
			"natives_dir":   nativesDir,
			"assets_dir":    assetsDir,
		},
	}
}

// ==================== Minecraft Launcher Service ====================

// LauncherService handles Minecraft game launching
type LauncherService struct {
	authService *AuthService
	bmclService *BMCLAPIService
}

// NewLauncherService creates a new LauncherService
func NewLauncherService() *LauncherService {
	return &LauncherService{
		authService: NewAuthService(),
		bmclService: NewBMCLAPIService(),
	}
}

// GetMinecraftDir returns the Minecraft directory path
func (l *LauncherService) GetMinecraftDir() string {
	home, _ := os.UserHomeDir()
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("APPDATA"), ".minecraft")
	} else if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Application Support", "minecraft")
	}
	return filepath.Join(home, ".minecraft")
}

// GetJavaPath finds Java installation
func (l *LauncherService) GetJavaPath() string {
	javaHome := os.Getenv("JAVA_HOME")
	if javaHome != "" {
		javapath := filepath.Join(javaHome, "bin", "java")
		if _, err := os.Stat(javapath); err == nil {
			return javapath
		}
	}

	// Check common locations
	paths := []string{
		"/usr/bin/java",
		"/usr/local/bin/java",
		"/opt/java/bin/java",
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	return "java" // Fallback to PATH
}

// Setting keys used to persist the launcher config in the `settings` table.
// Memory values are stored as text and parsed back to int on load.
const (
	settingKeyMaxMemory  = "launcher.max_memory"
	settingKeyMinMemory  = "launcher.min_memory"
	settingKeyJavaArgs   = "launcher.java_args"
	settingKeyGameWidth  = "launcher.game_width"
	settingKeyGameHeight = "launcher.game_height"
	settingKeyGameDir    = "launcher.game_dir"
	settingKeyAuthlibJar = "launcher.authlib_jar"
)

// GetLauncherConfig returns the current launcher configuration, merging
// persisted user preferences (memory, JVM args, window size) with the
// auto-detected Java path and game directory. Unset values fall back to
// sane defaults matching the launcher standard:
//   - MinMemory:  1024 MB  (-Xms1024m)
//   - MaxMemory:  4096 MB  (-Xmx4096m)
//   - GameWidth:  854
//   - GameHeight: 480
func (l *LauncherService) GetLauncherConfig() models.LauncherConfig {
	settings, _ := loadLauncherSettings()
	maxMem := parseIntSetting(settings[settingKeyMaxMemory], 4096)
	minMem := parseIntSetting(settings[settingKeyMinMemory], 1024)
	width := parseIntSetting(settings[settingKeyGameWidth], 854)
	height := parseIntSetting(settings[settingKeyGameHeight], 480)

	gameDir := settings[settingKeyGameDir]
	if gameDir == "" {
		gameDir = l.GetMinecraftDir()
	}

	javaArgs := settings[settingKeyJavaArgs]

	return models.LauncherConfig{
		JavaPath:   l.GetJavaPath(),
		MaxMemory:  maxMem,
		MinMemory:  minMem,
		JavaArgs:   javaArgs,
		GameWidth:  width,
		GameHeight: height,
		GameDir:    gameDir,
		AuthlibJar: settings[settingKeyAuthlibJar],
	}
}

// SaveLauncherConfig persists the user-editable subset of the launcher
// configuration. JavaPath and GameDir are auto-detected on every launch and
// intentionally not overwritten here.
//
// The values are written to the `settings` table under the keys defined
// above. Invalid (non-numeric) memory/size values are rejected before
// persistence and returned as a non-success ApiResponse so the frontend
// can surface a clear error to the user.
func (l *LauncherService) SaveLauncherConfig(cfg models.LauncherConfig) *models.ApiResponse {
	if cfg.MinMemory < 0 || cfg.MaxMemory < 0 {
		return &models.ApiResponse{Success: false, Error: "memory values must be non-negative"}
	}
	if cfg.MaxMemory > 0 && cfg.MinMemory > 0 && cfg.MinMemory > cfg.MaxMemory {
		return &models.ApiResponse{Success: false, Error: "min memory cannot exceed max memory"}
	}
	if cfg.GameWidth < 0 || cfg.GameHeight < 0 {
		return &models.ApiResponse{Success: false, Error: "game window dimensions must be non-negative"}
	}

	if err := saveLauncherSetting(settingKeyMaxMemory, cfg.MaxMemory, "integer", "Maximum heap size in MB (-Xmx)"); err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}
	if err := saveLauncherSetting(settingKeyMinMemory, cfg.MinMemory, "integer", "Minimum heap size in MB (-Xms)"); err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}
	if err := saveLauncherSetting(settingKeyJavaArgs, cfg.JavaArgs, "string", "Additional JVM arguments"); err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}
	if err := saveLauncherSetting(settingKeyGameWidth, cfg.GameWidth, "integer", "Game window width"); err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}
	if err := saveLauncherSetting(settingKeyGameHeight, cfg.GameHeight, "integer", "Game window height"); err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}
	if cfg.GameDir != "" {
		if err := saveLauncherSetting(settingKeyGameDir, cfg.GameDir, "string", "Game directory (.minecraft)"); err != nil {
			return &models.ApiResponse{Success: false, Error: err.Error()}
		}
	}
	if cfg.AuthlibJar != "" {
		if err := saveLauncherSetting(settingKeyAuthlibJar, cfg.AuthlibJar, "string", "Path to authlib-injector.jar"); err != nil {
			return &models.ApiResponse{Success: false, Error: err.Error()}
		}
	}

	return &models.ApiResponse{
		Success: true,
		Data:    l.GetLauncherConfig(),
	}
}

// loadVersionJSON reads the version JSON file from <gameDir>/versions/<id>/<id>.json
func (l *LauncherService) loadVersionJSON(versionID, gameDir string) (*BMCLVersionJSON, error) {
	versionJSONPath := filepath.Join(gameDir, "versions", versionID, versionID+".json")
	data, err := os.ReadFile(versionJSONPath)
	if err != nil {
		return nil, fmt.Errorf("read version json %q: %w", versionJSONPath, err)
	}
	var versionJSON BMCLVersionJSON
	if err := json.Unmarshal(data, &versionJSON); err != nil {
		return nil, fmt.Errorf("parse version json: %w", err)
	}
	return &versionJSON, nil
}

// mergeInheritedVersion folds the parent (vanilla) version JSON into a child
// loader version JSON per the inheritsFrom semantics: the child keeps its own
// id, mainClass and libraries, and inherits everything it does not define.
// The parent's libraries are appended after the child's so the loader's own
// jars lead the classpath.
func mergeInheritedVersion(child, parent *BMCLVersionJSON) {
	if child.MainClass == "" {
		child.MainClass = parent.MainClass
	}
	if child.AssetIndex.ID == "" {
		child.AssetIndex = parent.AssetIndex
	}
	if child.Assets == "" {
		child.Assets = parent.Assets
	}
	if len(child.Downloads) == 0 {
		child.Downloads = parent.Downloads
	}
	child.Libraries = append(child.Libraries, parent.Libraries...)
}

// buildLibraryClasspath assembles the classpath from the version's libraries,
// honouring OS rules and skipping natives (which are loaded via -Djava.library.path).
func buildLibraryClasspath(versionJSON *BMCLVersionJSON, librariesDir string) string {
	sep := string(os.PathListSeparator)
	parts := make([]string, 0, len(versionJSON.Libraries)+1)
	seen := make(map[string]bool)

	for _, lib := range versionJSON.Libraries {
		if !shouldDownloadLibrary(lib) {
			continue
		}
		// Skip natives: they are not jars on the classpath, they're extracted to the natives dir
		if len(lib.Downloads.Classifiers) > 0 {
			continue
		}
		artifact := lib.Downloads.Artifact
		if artifact.Path == "" {
			continue
		}
		if seen[artifact.Path] {
			continue
		}
		seen[artifact.Path] = true
		parts = append(parts, filepath.Join(librariesDir, artifact.Path))
	}

	return strings.Join(parts, sep)
}

// resolveUserType maps the stored account auth type to the value Mojang-style
// game arguments expect (--userType). Per the launcher standard:
//   - offline accounts use "legacy"
//   - Microsoft accounts use "msa"
//   - authlib-injector / Mojang Yggdrasil accounts use "mojang"
func resolveUserType(authType string) string {
	switch strings.ToLower(authType) {
	case "offline":
		return "legacy"
	case "microsoft", "msa":
		return "msa"
	default:
		return "mojang"
	}
}

// resolveVersionType maps the stored version type to the value game arguments
// expect (--versionType). Defaults to "release" for unknown types.
func resolveVersionType(versionType string) string {
	if versionType == "" {
		return "release"
	}
	return versionType
}

// BuildLaunchCommand builds the Minecraft launch command per the standard:
//   - reads <gameDir>/versions/<id>/<id>.json to obtain mainClass, assetIndex and libraries
//   - assembles -cp with the version jar + all matching libraries
//   - sets -Djava.library.path to the natives dir (must be populated beforehand)
//   - prepends -javaagent:/path/authlib-injector.jar=<auth-server> when an
//     authlib-injector account is in use and the jar path is provided
//   - emits the standard game args (--username, --uuid, --accessToken, --userType,
//     --version, --gameDir, --assetsDir, --assetIndex, --width, --height)
//   - injects --tweakClass for Forge LaunchWrapper main classes
func (l *LauncherService) BuildLaunchCommand(config models.LauncherConfig, version *models.MinecraftVersion, account *models.MinecraftAccount) ([]string, error) {
	if version == nil {
		return nil, fmt.Errorf("version is required")
	}
	if account == nil {
		return nil, fmt.Errorf("account is required")
	}

	javaPath := config.JavaPath
	if javaPath == "" {
		javaPath = l.GetJavaPath()
	}

	gameDir := config.GameDir
	if gameDir == "" {
		gameDir = l.GetMinecraftDir()
	}

	librariesDir := filepath.Join(gameDir, "libraries")

	// Load the version JSON; fall back to safe defaults if it cannot be read.
	versionJSON, err := l.loadVersionJSON(version.VersionID, gameDir)
	if err != nil {
		return nil, err
	}

	// Loader versions (Fabric/Forge) declare inheritsFrom: merge the parent
	// version into the child and use the parent's client jar and natives
	// (loader profiles carry neither of their own).
	jarVersionID := version.VersionID
	if versionJSON.InheritsFrom != "" {
		parent, err := l.loadVersionJSON(versionJSON.InheritsFrom, gameDir)
		if err != nil {
			return nil, fmt.Errorf("load parent version %q: %w", versionJSON.InheritsFrom, err)
		}
		mergeInheritedVersion(versionJSON, parent)
		jarVersionID = versionJSON.InheritsFrom
	}

	jarPath := filepath.Join(gameDir, "versions", jarVersionID, jarVersionID+".jar")
	nativesDir := filepath.Join(gameDir, "versions", jarVersionID, "natives")

	if err := os.MkdirAll(nativesDir, 0755); err != nil {
		return nil, fmt.Errorf("create natives dir: %w", err)
	}

	mainClass := versionJSON.MainClass
	if mainClass == "" {
		mainClass = "net.minecraft.client.main.Main"
	}

	assetIndex := versionJSON.AssetIndex.ID
	if assetIndex == "" {
		assetIndex = version.VersionID
	}

	// Build the classpath: <version jar> + every library (with rules applied).
	classpathParts := []string{jarPath}
	if libCP := buildLibraryClasspath(versionJSON, librariesDir); libCP != "" {
		classpathParts = append(classpathParts, libCP)
	}
	classpath := strings.Join(classpathParts, string(os.PathListSeparator))

	userType := resolveUserType(account.AuthType)
	versionType := resolveVersionType(version.VersionType)

	// Loader profiles (Fabric/Forge) carry their own launch arguments on top
	// of the standard set — fabric's -DFabricMcEmu jvm flag and forge's
	// --launchTarget game args (shapes verified against live metadata). They
	// are expanded only for inheriting versions; vanilla keeps the legacy
	// argument construction.
	var extraJVM, extraGame []string
	if versionJSON.InheritsFrom != "" {
		vars := map[string]string{
			"auth_player_name":    account.Username,
			"version_name":        version.VersionID,
			"game_directory":      gameDir,
			"assets_root":         filepath.Join(gameDir, "assets"),
			"assets_index_name":   assetIndex,
			"auth_uuid":           stripDashes(account.UUID),
			"auth_access_token":   account.AccessToken,
			"user_type":           userType,
			"version_type":        versionType,
			"natives_directory":   nativesDir,
			"launcher_name":       "SamuelClient",
			"launcher_version":    "1.0",
			"classpath":           classpath,
			"classpath_separator": string(os.PathListSeparator),
			"library_directory":   librariesDir,
			"user_properties":     "{}",
			"resolution_width":    strconv.Itoa(config.GameWidth),
			"resolution_height":   strconv.Itoa(config.GameHeight),
		}
		extraJVM = expandArgumentEntries(versionJSON.Arguments.JVM, vars)
		extraGame = expandArgumentEntries(versionJSON.Arguments.Game, vars)
	}

	// JVM args
	args := []string{javaPath}
	if config.MaxMemory > 0 {
		args = append(args, fmt.Sprintf("-Xmx%dM", config.MaxMemory))
	}
	if config.MinMemory > 0 {
		args = append(args, fmt.Sprintf("-Xms%dM", config.MinMemory))
	}
	if config.JavaArgs != "" {
		args = append(args, strings.Fields(config.JavaArgs)...)
	}
	args = append(args, "-Djava.library.path="+nativesDir)
	args = append(args, extraJVM...)

	// Authlib-Injector: -javaagent:/path/authlib-injector.jar=<auth-server>
	if strings.EqualFold(account.AuthType, "authlib-injector") && config.AuthlibJar != "" && account.AuthServer != "" {
		args = append(args, fmt.Sprintf("-javaagent:%s=%s", config.AuthlibJar, strings.TrimRight(account.AuthServer, "/")))
		// 配置预获取: -Dauthlibinjector.yggdrasil.prefetched=<Base64(meta JSON)>
		// The metadata is fetched once via GetAuthlibMeta and cached; passing
		// it here lets authlib-injector avoid a network round-trip at game
		// startup and prevents a crash if the network drops mid-launch.
		if config.AuthlibMetaJSON != "" {
			args = append(args, "-Dauthlibinjector.yggdrasil.prefetched="+base64.StdEncoding.EncodeToString([]byte(config.AuthlibMetaJSON)))
		}
	}

	args = append(args, "-cp", classpath, mainClass)

	// Game args per the standard
	gameArgs := []string{
		"--username", account.Username,
		"--version", version.VersionID,
		"--gameDir", gameDir,
		"--assetsDir", filepath.Join(gameDir, "assets"),
		"--assetIndex", assetIndex,
		"--uuid", stripDashes(account.UUID),
		"--accessToken", account.AccessToken,
		"--userType", userType,
		"--versionType", versionType,
	}
	if config.GameWidth > 0 {
		gameArgs = append(gameArgs, "--width", fmt.Sprintf("%d", config.GameWidth))
	}
	if config.GameHeight > 0 {
		gameArgs = append(gameArgs, "--height", fmt.Sprintf("%d", config.GameHeight))
	}
	// Match handoff (INTEROP-SPEC §6): go straight into the match server.
	// JoinServer is strictly validated before reaching this point because it
	// is placed on the command line verbatim.
	if config.JoinServer != "" {
		gameArgs = append(gameArgs, "--server", config.JoinServer, "--port", strconv.Itoa(config.JoinPort))
	}
	gameArgs = append(gameArgs, extraGame...)

	// Forge 1.12.x and older use LaunchWrapper and need --tweakClass
	if strings.HasPrefix(mainClass, "net.minecraft.launchwrapper.Launch") {
		gameArgs = append(gameArgs, "--tweakClass", "net.minecraftforge.fml.common.launcher.FMLTweaker")
	}

	args = append(args, gameArgs...)
	return args, nil
}

// expandArgumentEntries resolves a version JSON arguments list (strings or
// {rules, value} objects) into concrete command-line tokens, substituting
// ${variable} placeholders. Entries whose rules exclude the current OS are
// dropped, mirroring the official launcher / HMCL behaviour.
func expandArgumentEntries(entries []interface{}, vars map[string]string) []string {
	var out []string
	for _, entry := range entries {
		switch v := entry.(type) {
		case string:
			out = append(out, substituteLaunchVars(v, vars))
		case map[string]interface{}:
			if !argumentRulesAllow(v["rules"]) {
				continue
			}
			switch value := v["value"].(type) {
			case string:
				out = append(out, substituteLaunchVars(value, vars))
			case []interface{}:
				for _, item := range value {
					if s, ok := item.(string); ok {
						out = append(out, substituteLaunchVars(s, vars))
					}
				}
			}
		}
	}
	return out
}

// argumentRulesAllow evaluates Mojang-style argument rules. Absent or empty
// rules mean the argument applies.
func argumentRulesAllow(rulesRaw interface{}) bool {
	rules, ok := rulesRaw.([]interface{})
	if !ok || len(rules) == 0 {
		return true
	}
	allowed := false
	for _, ruleRaw := range rules {
		rule, ok := ruleRaw.(map[string]interface{})
		if !ok {
			continue
		}
		action, _ := rule["action"].(string)
		matches := true
		if osObj, ok := rule["os"].(map[string]interface{}); ok {
			if name, ok := osObj["name"].(string); ok && name != runtime.GOOS {
				matches = false
			}
		}
		switch action {
		case "allow":
			if matches {
				allowed = true
			}
		case "disallow":
			if matches {
				return false
			}
		}
	}
	return allowed
}

// substituteLaunchVars replaces ${name} placeholders with launch context
// values; unknown placeholders are left as-is so they stay visible.
func substituteLaunchVars(s string, vars map[string]string) string {
	for name, value := range vars {
		s = strings.ReplaceAll(s, "${"+name+"}", value)
	}
	return s
}

// stripDashes removes dashes from a UUID (game expects compact form).
func stripDashes(s string) string {
	return strings.ReplaceAll(s, "-", "")
}

// LaunchGame launches Minecraft with the given configuration.
//
// The process is detached into its own process group so it survives the
// launcher (Wails) process exiting. stdout/stderr are streamed to a log file
// under <gameDir>/logs and the live file path is returned alongside the PID
// so the frontend can tail it.
//
// ADR-0004: when logCallback is non-nil, every line the game process writes
// to stdout/stderr is also forwarded to it (source is "stdout" or "stderr").
// The Wails binding supplies a callback that re-emits each line as a
// "launcher:log" event so the UI can render live launch logs without any
// HTTP polling.
func (l *LauncherService) LaunchGame(config models.LauncherConfig, version *models.MinecraftVersion, account *models.MinecraftAccount, logCallback func(source, line string)) *models.ApiResponse {
	args, err := l.BuildLaunchCommand(config, version, account)
	if err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}

	gameDir := config.GameDir
	if gameDir == "" {
		gameDir = l.GetMinecraftDir()
	}

	logsDir := filepath.Join(gameDir, "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return &models.ApiResponse{Success: false, Error: fmt.Sprintf("create logs dir: %v", err)}
	}
	logPath := filepath.Join(logsDir, fmt.Sprintf("launcher-%s.log", time.Now().Format("20060102-150405")))
	logFile, err := os.Create(logPath)
	if err != nil {
		return &models.ApiResponse{Success: false, Error: fmt.Sprintf("create log file: %v", err)}
	}

	// Write the exact command we are about to run, for debugging.
	fmt.Fprintf(logFile, "# Minecraft launch command\n# %s\n\n", time.Now().Format(time.RFC3339))
	for _, a := range args {
		fmt.Fprintf(logFile, "%s ", shellQuote(a))
	}
	fmt.Fprintln(logFile)

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = gameDir

	// Detach the process so it keeps running when the launcher exits.
	// configureDetachedProcess is implemented per-platform in:
	//   - mc_services_unix.go    (Linux / macOS)
	//   - mc_services_windows.go (Windows)
	configureDetachedProcess(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		logFile.Close()
		return &models.ApiResponse{Success: false, Error: fmt.Sprintf("stdout pipe: %v", err)}
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		logFile.Close()
		return &models.ApiResponse{Success: false, Error: fmt.Sprintf("stderr pipe: %v", err)}
	}

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return &models.ApiResponse{Success: false, Error: fmt.Sprintf("start game: %v", err)}
	}

	// Stream game output into the log file in the background. When a
	// logCallback is supplied (ADR-0004 Wails binding path) each complete
	// line is additionally forwarded so the UI can render live launch logs.
	// Both streamers must finish before the trailing marker is written and
	// the file is closed, otherwise the stdout goroutine could race the
	// close.
	go func() {
		defer logFile.Close()
		stdoutDone := make(chan struct{})
		if logCallback != nil {
			go func() {
				streamLines(logFile, stdout, "stdout", logCallback)
				close(stdoutDone)
			}()
			streamLines(logFile, stderr, "stderr", logCallback)
		} else {
			go func() {
				io.Copy(logFile, stdout)
				close(stdoutDone)
			}()
			io.Copy(logFile, stderr)
		}
		<-stdoutDone
		cmd.Wait()
		fmt.Fprintf(logFile, "\n# Process exited\n")
	}()

	return &models.ApiResponse{
		Success: true,
		Data: map[string]interface{}{
			"pid":      cmd.Process.Pid,
			"args":     args,
			"log_path": logPath,
		},
	}
}

// streamLines copies r into w and forwards each complete line to logCallback
// labelled with source ("stdout" or "stderr"). Used by LaunchGame to feed the
// live launch-log event stream (ADR-0004).
func streamLines(w io.Writer, r io.Reader, source string, logCallback func(source, line string)) {
	scanner := bufio.NewScanner(r)
	// Give the scanner headroom beyond the default 64 KiB token limit; game
	// output occasionally contains very long lines.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		fmt.Fprintln(w, line)
		logCallback(source, line)
	}
}

// shellQuote quotes an argument for the log file so the recorded command can
// be re-run safely. Single-quote and escape embedded single quotes.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t\n\"'\\$`&;|*?<>()[]{}#~!") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// ==================== Launcher Config Persistence ====================

// loadLauncherSettings reads all `settings` rows into a key→value map. Used by
// GetLauncherConfig to layer persisted user preferences over the defaults.
// Errors are intentionally swallowed (treated as empty) so a missing or
// corrupt settings table still returns a usable config.
func loadLauncherSettings() (map[string]string, error) {
	settingsRepo := repository.NewSettingRepository()
	all, err := settingsRepo.GetAll()
	if err != nil {
		return map[string]string{}, err
	}
	out := make(map[string]string, len(all))
	for _, s := range all {
		out[s.Key] = s.Value
	}
	return out, nil
}

// saveLauncherSetting writes a single key/value to the settings table. The
// value is formatted using fmt.Sprintf("%v", value) so callers can pass int or
// string interchangeably.
func saveLauncherSetting(key string, value interface{}, settingType, description string) error {
	settingsRepo := repository.NewSettingRepository()
	return settingsRepo.Set(key, fmt.Sprintf("%v", value), settingType, description)
}

// parseIntSetting converts a persisted setting value to int. Falls back to the
// provided default when the value is empty, missing, or not a valid integer.
func parseIntSetting(value string, defaultValue int) int {
	if value == "" {
		return defaultValue
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}
	return n
}
