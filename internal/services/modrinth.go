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

const (
	modrinthAPI      = "https://api.modrinth.com/v2"
	ModrinthPageSize = 20
	maxModrinthFiles = 32
	maxModrinthBytes = 512 << 20
	maxModrinthJSON  = 8 << 20
)

type ModrinthTarget struct {
	ProfileID   string `json:"profile_id"`
	GameVersion string `json:"game_version"`
	Loader      string `json:"loader"`
}

type ModrinthProject struct {
	ID          string `json:"project_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Author      string `json:"author"`
}

type ModrinthSearchResult struct {
	Hits  []ModrinthProject `json:"hits"`
	Total int               `json:"total_hits"`
	Limit int               `json:"limit"`
}

type ModrinthVersion struct {
	ID           string               `json:"id"`
	Number       string               `json:"version_number"`
	Channel      string               `json:"version_type"`
	ProjectID    string               `json:"project_id"`
	Published    string               `json:"date_published"`
	Environment  string               `json:"environment"`
	Versions     []string             `json:"game_versions"`
	Loaders      []string             `json:"loaders"`
	Files        []modrinthFile       `json:"files"`
	Dependencies []modrinthDependency `json:"dependencies"`
}

type modrinthFile struct {
	Name    string            `json:"filename"`
	URL     string            `json:"url"`
	Size    int64             `json:"size"`
	Primary bool              `json:"primary"`
	Type    string            `json:"file_type"`
	Hashes  map[string]string `json:"hashes"`
}

type modrinthDependency struct {
	ProjectID string `json:"project_id"`
	VersionID string `json:"version_id"`
	FileName  string `json:"file_name"`
	Type      string `json:"dependency_type"`
}

type ModrinthPlanFile struct {
	ProjectID string `json:"project_id"`
	VersionID string `json:"version_id"`
	Version   string `json:"version"`
	Channel   string `json:"channel"`
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	SHA512    string `json:"-"`
	SHA1      string `json:"-"`
	url       string
}

type ModrinthPlan struct {
	Fingerprint string             `json:"fingerprint"`
	Files       []ModrinthPlanFile `json:"files"`
}

type ModrinthService struct {
	client *http.Client
}

func NewModrinthService() *ModrinthService {
	return &ModrinthService{client: &http.Client{
		Timeout: 90 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 || req.URL.Scheme != "https" || req.URL.Host != via[0].URL.Host || req.URL.User != nil {
				return fmt.Errorf("unexpected Modrinth redirect")
			}
			return nil
		},
	}}
}

func (s *ModrinthService) get(ctx context.Context, address string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "CoreMatch/SamuelClient (https://github.com/CoreMatch/SamuelClient)")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests {
			return nil, fmt.Errorf("Modrinth rate limit reached; try again shortly")
		}
		return nil, fmt.Errorf("Modrinth returned HTTP %d", resp.StatusCode)
	}
	return resp, nil
}

func (s *ModrinthService) readJSON(ctx context.Context, path string, query url.Values, out any) error {
	address := modrinthAPI + path
	if len(query) > 0 {
		address += "?" + query.Encode()
	}
	resp, err := s.get(ctx, address)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxModrinthJSON+1))
	if err != nil {
		return err
	}
	if len(raw) > maxModrinthJSON {
		return fmt.Errorf("Modrinth response is too large")
	}
	return json.Unmarshal(raw, out)
}

func (s *ModrinthService) Search(ctx context.Context, target ModrinthTarget, query string, offset int) (ModrinthSearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" || len(query) > 256 || offset < 0 || offset >= 10000 {
		return ModrinthSearchResult{}, fmt.Errorf("invalid Modrinth search")
	}
	facets, _ := json.Marshal([][]string{
		{"project_type:mod"}, {"categories:" + target.Loader}, {"versions:" + target.GameVersion},
	})
	result := ModrinthSearchResult{Hits: []ModrinthProject{}}
	err := s.readJSON(ctx, "/search", url.Values{
		"query": {query}, "facets": {string(facets)},
		"offset": {strconv.Itoa(offset)}, "limit": {strconv.Itoa(ModrinthPageSize)},
	}, &result)
	if result.Hits == nil {
		result.Hits = []ModrinthProject{}
	}
	result.Limit = ModrinthPageSize
	return result, err
}

func modrinthID(id string) bool {
	return len(id) > 0 && len(id) <= 64 && strings.IndexFunc(id, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9')
	}) == -1
}

func (v ModrinthVersion) compatible(target ModrinthTarget) bool {
	return v.Environment != "server_only" &&
		slices.Contains(v.Versions, target.GameVersion) && slices.Contains(v.Loaders, target.Loader)
}

func jsonStringList(value string) string {
	raw, _ := json.Marshal([]string{value})
	return string(raw)
}

func (s *ModrinthService) Versions(ctx context.Context, target ModrinthTarget, projectID string) ([]ModrinthVersion, error) {
	if !modrinthID(projectID) {
		return nil, fmt.Errorf("invalid Modrinth project ID")
	}
	var versions []ModrinthVersion
	err := s.readJSON(ctx, "/project/"+projectID+"/version", url.Values{
		"loaders": {jsonStringList(target.Loader)}, "game_versions": {jsonStringList(target.GameVersion)},
	}, &versions)
	if err != nil {
		return nil, err
	}
	out := []ModrinthVersion{}
	for _, v := range versions {
		if v.ProjectID == projectID && modrinthID(v.ID) && v.compatible(target) {
			if _, err := v.installFile(); err == nil {
				out = append(out, v)
			}
		}
	}
	slices.SortStableFunc(out, func(a, b ModrinthVersion) int {
		if (a.Channel == "release") != (b.Channel == "release") {
			if a.Channel == "release" {
				return -1
			}
			return 1
		}
		return strings.Compare(b.Published, a.Published)
	})
	return out, nil
}

func (v ModrinthVersion) installFile() (ModrinthPlanFile, error) {
	for i, file := range v.Files {
		if !file.Primary && (i != 0 || slices.ContainsFunc(v.Files, func(f modrinthFile) bool { return f.Primary })) {
			continue
		}
		name, ok := modFile(file.Name)
		address, urlErr := url.Parse(file.URL)
		hash, hashErr := hex.DecodeString(file.Hashes["sha512"])
		if !ok || !name.Enabled || file.Type != "" || file.Size <= 0 || file.Size > maxModrinthBytes ||
			hashErr != nil || len(hash) != 64 || urlErr != nil || address.Scheme != "https" ||
			address.Host != "cdn.modrinth.com" || address.User != nil {
			break
		}
		return ModrinthPlanFile{
			ProjectID: v.ProjectID, VersionID: v.ID, Version: v.Number, Channel: v.Channel,
			Name: file.Name, Size: file.Size, SHA512: strings.ToLower(file.Hashes["sha512"]), url: file.URL,
		}, nil
	}
	return ModrinthPlanFile{}, fmt.Errorf("no supported primary JAR for Modrinth version %s", v.ID)
}

func (s *ModrinthService) version(ctx context.Context, target ModrinthTarget, id string) (ModrinthVersion, error) {
	var v ModrinthVersion
	if !modrinthID(id) {
		return v, fmt.Errorf("invalid Modrinth version ID")
	}
	if err := s.readJSON(ctx, "/version/"+id, nil, &v); err != nil {
		return v, err
	}
	if v.ID != id || !modrinthID(v.ProjectID) || !v.compatible(target) {
		return v, fmt.Errorf("Modrinth version %s is not compatible with %s %s", id, target.Loader, target.GameVersion)
	}
	return v, nil
}

func (s *ModrinthService) Plan(ctx context.Context, target ModrinthTarget, projectID, versionID string) (ModrinthPlan, error) {
	var plan ModrinthPlan
	main, err := s.version(ctx, target, versionID)
	if err != nil {
		return plan, err
	}
	if main.ProjectID != projectID {
		return plan, fmt.Errorf("Modrinth version does not belong to this project")
	}
	selected := map[string]string{}
	names := map[string]bool{}
	var total int64
	var visit func(ModrinthVersion) error
	visit = func(v ModrinthVersion) error {
		if id, exists := selected[v.ProjectID]; exists {
			if id != v.ID {
				return fmt.Errorf("conflicting required versions for Modrinth project %s", v.ProjectID)
			}
			return nil
		}
		file, err := v.installFile()
		if err != nil {
			return err
		}
		total += file.Size
		if len(selected) >= maxModrinthFiles || total > maxModrinthBytes {
			return fmt.Errorf("Modrinth installation exceeds the file count or size limit")
		}
		nameKey := strings.ToLower(file.Name)
		if names[nameKey] {
			return fmt.Errorf("duplicate filename in Modrinth installation: %s", file.Name)
		}
		names[nameKey] = true
		selected[v.ProjectID] = v.ID
		plan.Files = append(plan.Files, file)
		for _, dep := range v.Dependencies {
			if dep.Type != "required" {
				continue
			}
			// A pinned dependency must still be checked if an unpinned choice was made.
			if id := selected[dep.ProjectID]; id != "" && (dep.VersionID == "" || dep.VersionID == id) {
				continue
			}
			if dep.VersionID != "" {
				next, err := s.version(ctx, target, dep.VersionID)
				if err != nil {
					return err
				}
				if dep.ProjectID != "" && dep.ProjectID != next.ProjectID {
					return fmt.Errorf("Modrinth dependency version does not match its project")
				}
				if err := visit(next); err != nil {
					return err
				}
			} else if dep.ProjectID != "" {
				versions, err := s.Versions(ctx, target, dep.ProjectID)
				if err != nil {
					return err
				}
				if len(versions) == 0 {
					return fmt.Errorf("no compatible version for required Modrinth project %s", dep.ProjectID)
				}
				if err := visit(versions[0]); err != nil {
					return err
				}
			} else {
				return fmt.Errorf("required dependency %q cannot be resolved through Modrinth", dep.FileName)
			}
		}
		return nil
	}
	if err := visit(main); err != nil {
		return ModrinthPlan{}, err
	}
	h := sha256.New()
	fmt.Fprintf(h, "%q %q %q\n", target.ProfileID, target.GameVersion, target.Loader)
	for _, f := range plan.Files {
		fmt.Fprintf(h, "%q %q %q %d %s\n", f.ProjectID, f.VersionID, f.Name, f.Size, f.SHA512)
	}
	plan.Fingerprint = hex.EncodeToString(h.Sum(nil))
	return plan, nil
}
