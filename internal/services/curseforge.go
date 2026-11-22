package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

const curseForgeHost = "api.curseforge.com"

type CurseForgeService struct {
	key    string
	client *http.Client
}

func NewCurseForgeService(key string) *CurseForgeService {
	return &CurseForgeService{key: strings.TrimSpace(key), client: &http.Client{
		Timeout: 90 * time.Second,
		CheckRedirect: func(next *http.Request, via []*http.Request) error {
			// API requests never redirect. CDN requests carry no key and may
			// only redirect to another official CDN host.
			if len(via) >= 5 || !curseForgeCDN(via[0].URL.String()) || !curseForgeCDN(next.URL.String()) {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}}
}

func (s *CurseForgeService) request(ctx context.Context, address string, api bool) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	if api {
		if s.key == "" {
			return nil, fmt.Errorf("CurseForge API key is not configured")
		}
		req.Header.Set("x-api-key", s.key)
		req.Header.Set("Accept", "application/json")
	}
	req.Header.Set("User-Agent", "CoreMatch/SamuelClient")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return nil, fmt.Errorf("CurseForge denied access; check the API key or use the official project page")
		case http.StatusTooManyRequests:
			return nil, fmt.Errorf("CurseForge rate limit reached; try again shortly")
		default:
			return nil, fmt.Errorf("CurseForge returned HTTP %d", resp.StatusCode)
		}
	}
	return resp, nil
}

func (s *CurseForgeService) read(ctx context.Context, path string, query url.Values, out any) error {
	address := "https://" + curseForgeHost + "/v1" + path
	if len(query) > 0 {
		address += "?" + query.Encode()
	}
	resp, err := s.request(ctx, address, true)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxModrinthJSON+1))
	if err != nil {
		return err
	}
	if len(raw) > maxModrinthJSON {
		return fmt.Errorf("CurseForge response is too large")
	}
	return json.Unmarshal(raw, out)
}

func curseForgeTarget(target ModrinthTarget) url.Values {
	loader := "1" // Forge
	if target.Loader == "fabric" {
		loader = "4"
	}
	return url.Values{"gameVersion": {target.GameVersion}, "modLoaderType": {loader}}
}

func curseForgeID(id string) bool {
	value, err := strconv.ParseInt(id, 10, 32)
	return err == nil && value > 0 && strconv.FormatInt(value, 10) == id
}

func (s *CurseForgeService) Search(ctx context.Context, target ModrinthTarget, query string, offset int) (ModrinthSearchResult, error) {
	result := ModrinthSearchResult{Hits: []ModrinthProject{}, Limit: ModrinthPageSize}
	query = strings.TrimSpace(query)
	if query == "" || len(query) > 256 || offset < 0 || offset+ModrinthPageSize > 10000 {
		return result, fmt.Errorf("invalid CurseForge search")
	}
	var response struct {
		Data []struct {
			ID      int    `json:"id"`
			Name    string `json:"name"`
			Summary string `json:"summary"`
			Authors []struct {
				Name string `json:"name"`
			} `json:"authors"`
		} `json:"data"`
		Pagination struct {
			Total int `json:"totalCount"`
		} `json:"pagination"`
	}
	params := curseForgeTarget(target)
	params.Set("gameId", "432")
	params.Set("classId", "6")
	params.Set("searchFilter", query)
	params.Set("index", strconv.Itoa(offset))
	params.Set("pageSize", strconv.Itoa(ModrinthPageSize))
	if err := s.read(ctx, "/mods/search", params, &response); err != nil {
		return result, err
	}
	result.Total = response.Pagination.Total
	for _, p := range response.Data {
		var authors []string
		for _, author := range p.Authors {
			authors = append(authors, author.Name)
		}
		result.Hits = append(result.Hits, ModrinthProject{
			ID: strconv.Itoa(p.ID), Title: p.Name, Author: strings.Join(authors, ", "), Description: p.Summary,
		})
	}
	return result, nil
}

type curseForgeFile struct {
	ID           int      `json:"id"`
	ModID        int      `json:"modId"`
	Name         string   `json:"fileName"`
	DisplayName  string   `json:"displayName"`
	Size         int64    `json:"fileLength"`
	URL          string   `json:"downloadUrl"`
	GameVersions []string `json:"gameVersions"`
	Release      int      `json:"releaseType"`
	Hashes       []struct {
		Value string `json:"value"`
		Algo  int    `json:"algo"`
	} `json:"hashes"`
	Dependencies []struct {
		ModID int `json:"modId"`
		Type  int `json:"relationType"`
	} `json:"dependencies"`
}

type CurseForgeVersion struct {
	ID           string `json:"id"`
	Number       string `json:"version_number"`
	Channel      string `json:"version_type"`
	Downloadable bool   `json:"downloadable"`
}

func (f curseForgeFile) channel() string {
	switch f.Release {
	case 1:
		return "release"
	case 2:
		return "beta"
	case 3:
		return "alpha"
	default:
		return "unknown"
	}
}

func (f curseForgeFile) compatible(target ModrinthTarget) bool {
	return slices.Contains(f.GameVersions, target.GameVersion) &&
		slices.ContainsFunc(f.GameVersions, func(v string) bool { return strings.EqualFold(v, target.Loader) })
}

func curseForgeCDN(address string) bool {
	u, err := url.Parse(address)
	return err == nil && u.Scheme == "https" && u.User == nil && u.Port() == "" &&
		strings.HasSuffix(u.Hostname(), ".forgecdn.net")
}

func (f curseForgeFile) installFile() (ModrinthPlanFile, error) {
	if f.URL == "" {
		return ModrinthPlanFile{}, fmt.Errorf("CurseForge does not provide a download link for %q; use the official website", f.Name)
	}
	mod, ok := modFile(f.Name)
	if !ok || !mod.Enabled || f.Size <= 0 || f.Size > maxModrinthBytes || !curseForgeCDN(f.URL) {
		return ModrinthPlanFile{}, fmt.Errorf("unsupported CurseForge download: %q", f.Name)
	}
	for _, h := range f.Hashes {
		raw, err := hex.DecodeString(h.Value)
		if h.Algo == 1 && err == nil && len(raw) == 20 {
			return ModrinthPlanFile{
				ProjectID: strconv.Itoa(f.ModID), VersionID: strconv.Itoa(f.ID), Version: f.DisplayName,
				Channel: f.channel(), Name: f.Name, Size: f.Size, SHA1: strings.ToLower(h.Value), url: f.URL,
			}, nil
		}
	}
	return ModrinthPlanFile{}, fmt.Errorf("no SHA-1 checksum for CurseForge file %q", f.Name)
}

func (s *CurseForgeService) files(ctx context.Context, target ModrinthTarget, projectID string) ([]curseForgeFile, error) {
	if !curseForgeID(projectID) {
		return nil, fmt.Errorf("invalid CurseForge project ID")
	}
	var response struct {
		Data []curseForgeFile `json:"data"`
	}
	params := curseForgeTarget(target)
	params.Set("pageSize", "50")
	if err := s.read(ctx, "/mods/"+projectID+"/files", params, &response); err != nil {
		return nil, err
	}
	out := []curseForgeFile{}
	for _, file := range response.Data {
		mod, ok := modFile(file.Name)
		if strconv.Itoa(file.ModID) == projectID && file.ID > 0 && ok && mod.Enabled && file.compatible(target) {
			out = append(out, file)
		}
	}
	slices.SortStableFunc(out, func(a, b curseForgeFile) int {
		if (a.Release == 1) != (b.Release == 1) {
			if a.Release == 1 {
				return -1
			}
			return 1
		}
		return 0
	})
	return out, nil
}

func (s *CurseForgeService) Versions(ctx context.Context, target ModrinthTarget, projectID string) ([]CurseForgeVersion, error) {
	files, err := s.files(ctx, target, projectID)
	if err != nil {
		return nil, err
	}
	out := []CurseForgeVersion{}
	for _, file := range files {
		_, err := file.installFile()
		out = append(out, CurseForgeVersion{
			ID: strconv.Itoa(file.ID), Number: file.DisplayName, Channel: file.channel(), Downloadable: err == nil,
		})
	}
	return out, nil
}

func (s *CurseForgeService) Plan(ctx context.Context, target ModrinthTarget, projectID, versionID string) (ModrinthPlan, error) {
	var plan ModrinthPlan
	if !curseForgeID(projectID) || !curseForgeID(versionID) {
		return plan, fmt.Errorf("invalid CurseForge project or file ID")
	}
	var response struct {
		Data curseForgeFile `json:"data"`
	}
	if err := s.read(ctx, "/mods/"+projectID+"/files/"+versionID, nil, &response); err != nil {
		return plan, err
	}
	main := response.Data
	if strconv.Itoa(main.ModID) != projectID || strconv.Itoa(main.ID) != versionID || !main.compatible(target) {
		return plan, fmt.Errorf("CurseForge file does not match the selected project or profile")
	}
	seen := map[int]bool{}
	names := map[string]bool{}
	var total int64
	var visit func(curseForgeFile) error
	visit = func(f curseForgeFile) error {
		file, err := f.installFile()
		if err != nil {
			return err
		}
		total += file.Size
		name := strings.ToLower(file.Name)
		if names[name] || len(plan.Files) >= maxModrinthFiles || total > maxModrinthBytes {
			return fmt.Errorf("CurseForge installation has a filename conflict or exceeds its limits")
		}
		seen[f.ModID], names[name] = true, true
		plan.Files = append(plan.Files, file)
		for _, dep := range f.Dependencies {
			if dep.Type != 3 || seen[dep.ModID] { // RequiredDependency
				continue
			}
			dependencyID := strconv.Itoa(dep.ModID)
			files, err := s.files(ctx, target, dependencyID)
			if err != nil {
				return err
			}
			choice := slices.IndexFunc(files, func(f curseForgeFile) bool {
				_, err := f.installFile()
				return err == nil
			})
			if choice < 0 {
				return fmt.Errorf("no compatible downloadable JAR for required CurseForge project %s; check the official website", dependencyID)
			}
			if err := visit(files[choice]); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit(main); err != nil {
		return ModrinthPlan{}, err
	}
	h := sha256.New()
	fmt.Fprintf(h, "curseforge %q %q %q\n", target.ProfileID, target.GameVersion, target.Loader)
	for _, f := range plan.Files {
		fmt.Fprintf(h, "%q %q %q %d %s\n", f.ProjectID, f.VersionID, f.Name, f.Size, f.SHA1)
	}
	plan.Fingerprint = hex.EncodeToString(h.Sum(nil))
	return plan, nil
}

func (s *CurseForgeService) Install(ctx context.Context, gameDir string, target ModrinthTarget, projectID, versionID, fingerprint string) error {
	plan, err := s.Plan(ctx, target, projectID, versionID)
	if err != nil {
		return err
	}
	return installOnlineMods(ctx, gameDir, plan, fingerprint, func(ctx context.Context, address string) (*http.Response, error) {
		if !curseForgeCDN(address) {
			return nil, fmt.Errorf("invalid CurseForge download host")
		}
		return s.request(ctx, address, false)
	})
}
