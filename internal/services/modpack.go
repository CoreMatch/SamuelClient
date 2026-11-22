package services

import (
	"archive/zip"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const (
	maxPackArchive = 1 << 30
	maxPackContent = 8 << 30
	maxPackEntries = 50000
	packMarker     = ".samuel-modpack.json"
)

type ModpackSource struct {
	ProjectID string `json:"project_id"`
	VersionID string `json:"version_id"`
	LocalPath string `json:"local_path"`
}

type ModpackVersion struct {
	ID       string   `json:"id"`
	Number   string   `json:"version_number"`
	Channel  string   `json:"version_type"`
	Versions []string `json:"game_versions"`
	Loaders  []string `json:"loaders"`
}

type ModpackOption struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

type ModpackPreview struct {
	Name          string          `json:"name"`
	Version       string          `json:"version"`
	GameVersion   string          `json:"game_version"`
	Loader        string          `json:"loader"`
	LoaderVersion string          `json:"loader_version"`
	RequiredBytes int64           `json:"required_bytes"`
	Options       []ModpackOption `json:"options"`
	Fingerprint   string          `json:"fingerprint"`
}

type ModpackInstance struct {
	Name      string `json:"name"`
	VersionID string `json:"version_id"`
}

type modpackIndex struct {
	Format       int               `json:"formatVersion"`
	Game         string            `json:"game"`
	Name         string            `json:"name"`
	Version      string            `json:"versionId"`
	Dependencies map[string]string `json:"dependencies"`
	Files        []modpackFile     `json:"files"`
}

type modpackFile struct {
	Path      string            `json:"path"`
	Size      int64             `json:"fileSize"`
	Env       map[string]string `json:"env"`
	Downloads []string          `json:"downloads"`
	Hashes    map[string]string `json:"hashes"`
}

type modpackArchive struct {
	preview   ModpackPreview
	files     []modpackFile
	overrides [2][]*zip.File
}

type PackProgress func(stage, name string, downloaded, total int64)

func (s *ModrinthService) SearchModpacks(ctx context.Context, query string, offset int) (ModrinthSearchResult, error) {
	result := ModrinthSearchResult{Hits: []ModrinthProject{}, Limit: ModrinthPageSize}
	query = strings.TrimSpace(query)
	if len(query) > 256 || offset < 0 || offset >= 10000 {
		return result, fmt.Errorf("invalid Modrinth search")
	}
	index := "relevance"
	if query == "" {
		index = "downloads"
	}
	err := s.readJSON(ctx, "/search", url.Values{
		"query": {query}, "facets": {`[["project_type:modpack"]]`}, "index": {index},
		"offset": {strconv.Itoa(offset)}, "limit": {strconv.Itoa(ModrinthPageSize)},
	}, &result)
	return result, err
}

func (s *ModrinthService) ModpackVersions(ctx context.Context, projectID string) ([]ModpackVersion, error) {
	if !modrinthID(projectID) {
		return nil, fmt.Errorf("invalid Modrinth project ID")
	}
	var versions []ModrinthVersion
	if err := s.readJSON(ctx, "/project/"+projectID+"/version", nil, &versions); err != nil {
		return nil, err
	}
	out := []ModpackVersion{}
	for _, v := range versions {
		if v.ProjectID == projectID && modrinthID(v.ID) && v.Environment != "server_only" {
			if _, err := packAsset(v); err == nil {
				out = append(out, ModpackVersion{v.ID, v.Number, v.Channel, v.Versions, v.Loaders})
			}
		}
	}
	return out, nil
}

func isPackDigest(value string, size int) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == size
}

func packAsset(v ModrinthVersion) (modrinthFile, error) {
	var asset modrinthFile
	for _, file := range v.Files {
		u, err := url.Parse(file.URL)
		if strings.EqualFold(path.Ext(file.Name), ".mrpack") && file.Size > 0 && file.Size <= maxPackArchive &&
			isPackDigest(file.Hashes["sha512"], sha512.Size) && err == nil &&
			u.Scheme == "https" && u.Host == "cdn.modrinth.com" && u.User == nil {
			if asset.URL == "" || file.Primary {
				asset = file
			}
		}
	}
	if asset.URL == "" {
		return asset, fmt.Errorf("no downloadable .mrpack for Modrinth version %s", v.ID)
	}
	return asset, nil
}

type packReader struct {
	ctx context.Context
	io.Reader
	onRead func(int64)
}

func (r packReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.Reader.Read(p)
	if n > 0 && r.onRead != nil {
		r.onRead(int64(n))
	}
	return n, err
}

// Snapshot local inputs as well as downloads, so the bytes previewed and
// parsed are the same bytes later used to extract overrides.
func (s *ModrinthService) openModpack(ctx context.Context, source ModpackSource, progress PackProgress) (*os.File, *modpackArchive, error) {
	var body io.ReadCloser
	var expected int64
	var checksum string
	name := "Modrinth"
	if source.LocalPath != "" {
		if source.ProjectID != "" || source.VersionID != "" || !filepath.IsAbs(source.LocalPath) ||
			!strings.EqualFold(filepath.Ext(source.LocalPath), ".mrpack") {
			return nil, nil, fmt.Errorf("select a local .mrpack file")
		}
		info, err := os.Stat(source.LocalPath)
		if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxPackArchive {
			return nil, nil, fmt.Errorf("local .mrpack is missing or exceeds the archive size limit")
		}
		body, err = os.Open(source.LocalPath)
		if err != nil {
			return nil, nil, err
		}
		expected, name = info.Size(), filepath.Base(source.LocalPath)
	} else {
		if !modrinthID(source.ProjectID) || !modrinthID(source.VersionID) {
			return nil, nil, fmt.Errorf("invalid Modrinth project or version ID")
		}
		var v ModrinthVersion
		if err := s.readJSON(ctx, "/version/"+source.VersionID, nil, &v); err != nil {
			return nil, nil, err
		}
		if v.ID != source.VersionID || v.ProjectID != source.ProjectID || v.Environment == "server_only" {
			return nil, nil, fmt.Errorf("the selected version is not a client version of this project")
		}
		file, err := packAsset(v)
		if err != nil {
			return nil, nil, err
		}
		resp, err := s.get(ctx, file.URL)
		if err != nil {
			return nil, nil, err
		}
		body, expected, checksum, name = resp.Body, file.Size, file.Hashes["sha512"], file.Name
	}
	defer body.Close()
	f, err := os.CreateTemp("", "samuel-modpack-*.mrpack")
	if err != nil {
		return nil, nil, err
	}
	ok := false
	defer func() {
		if !ok {
			closePack(f)
		}
	}()
	var done int64
	var last time.Time
	digest, archiveHash := sha256.New(), sha512.New()
	n, err := io.Copy(io.MultiWriter(f, digest, archiveHash), packReader{ctx, io.LimitReader(body, expected+1), func(n int64) {
		done += n
		if progress != nil && (done == expected || time.Since(last) >= 150*time.Millisecond) {
			last = time.Now()
			progress("pack-archive", name, min(done, expected), expected)
		}
	}})
	if err != nil {
		return nil, nil, err
	}
	if n != expected || (checksum != "" && !strings.EqualFold(hex.EncodeToString(archiveHash.Sum(nil)), checksum)) {
		return nil, nil, fmt.Errorf("modpack archive size or checksum mismatch")
	}
	z, err := zip.NewReader(f, n)
	if err != nil {
		return nil, nil, fmt.Errorf("read .mrpack archive: %w", err)
	}
	pack, err := parseModpack(z)
	if err != nil {
		return nil, nil, err
	}
	pack.preview.Fingerprint = hex.EncodeToString(digest.Sum(nil))
	ok = true
	return f, pack, nil
}

func closePack(f *os.File) {
	_ = f.Close()
	_ = os.Remove(f.Name())
}

func (s *ModrinthService) PreviewModpack(ctx context.Context, source ModpackSource) (ModpackPreview, error) {
	f, pack, err := s.openModpack(ctx, source, nil)
	if err != nil {
		return ModpackPreview{}, err
	}
	defer closePack(f)
	return pack.preview, nil
}

var packVersion = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.+-]{0,79}$`)
var packLoaderVersion = regexp.MustCompile(`^[0-9][0-9A-Za-z.+_-]{0,79}$`)
var packWindowsDevice = regexp.MustCompile(`(?i)^(CON|PRN|AUX|NUL|COM[0-9¹²³]|LPT[0-9¹²³])$`)

func safePackPath(name string) bool {
	if name == "." || len(name) > 1024 || !fs.ValidPath(name) || strings.ContainsAny(name, "\\:*?\"<>|") ||
		strings.ContainsFunc(name, unicode.IsControl) {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		base, _, _ := strings.Cut(part, ".")
		if len(part) > 255 || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") ||
			packWindowsDevice.MatchString(strings.TrimRight(base, " ")) {
			return false
		}
	}
	return true
}

func packDestination(name string) bool {
	if !safePackPath(name) {
		return false
	}
	top, _, _ := strings.Cut(strings.ToLower(name), "/")
	switch top {
	case "versions", "libraries", "assets", "runtime", "launcher_profiles.json", "launcher_accounts.json", packMarker:
		return false
	}
	return true
}

func packDownloadURL(u *url.URL, redirects bool) bool {
	if u == nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || u.Fragment != "" {
		return false
	}
	switch strings.ToLower(u.Host) {
	case "cdn.modrinth.com", "github.com", "raw.githubusercontent.com", "gitlab.com":
		return true
	case "release-assets.githubusercontent.com", "objects.githubusercontent.com", "media.githubusercontent.com", "storage.googleapis.com":
		return redirects
	}
	return false
}

func parseModpack(z *zip.Reader) (*modpackArchive, error) {
	if len(z.File) > maxPackEntries {
		return nil, fmt.Errorf("too many archive entries")
	}
	var index *zip.File
	for _, f := range z.File {
		if !safePackPath(strings.TrimSuffix(f.Name, "/")) || (!f.Mode().IsRegular() && !f.Mode().IsDir()) {
			return nil, fmt.Errorf("unsafe archive entry %q", f.Name)
		}
		if f.Name == "modrinth.index.json" {
			if index != nil {
				return nil, fmt.Errorf("duplicate modrinth.index.json")
			}
			index = f
		}
	}
	if index == nil || index.UncompressedSize64 > maxModrinthJSON {
		return nil, fmt.Errorf("missing or oversized modrinth.index.json")
	}
	r, err := index.Open()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	raw, err := io.ReadAll(io.LimitReader(r, maxModrinthJSON+1))
	if err != nil {
		return nil, err
	}
	var manifest modpackIndex
	if len(raw) > maxModrinthJSON || json.Unmarshal(raw, &manifest) != nil || manifest.Format != 1 ||
		manifest.Game != "minecraft" || strings.TrimSpace(manifest.Name) == "" || len(manifest.Name) > 512 ||
		strings.TrimSpace(manifest.Version) == "" || len(manifest.Version) > 256 || len(manifest.Files) > maxPackEntries {
		return nil, fmt.Errorf("invalid or unsupported Modrinth modpack manifest")
	}
	pack := &modpackArchive{preview: ModpackPreview{
		Name: manifest.Name, Version: manifest.Version, GameVersion: manifest.Dependencies["minecraft"],
		Loader: "vanilla", Options: []ModpackOption{},
	}}
	if !packVersion.MatchString(pack.preview.GameVersion) {
		return nil, fmt.Errorf("invalid or missing Minecraft version")
	}
	for key, version := range manifest.Dependencies {
		switch key {
		case "minecraft":
			continue
		case "fabric-loader", "forge":
			if pack.preview.Loader != "vanilla" || !packLoaderVersion.MatchString(version) {
				return nil, fmt.Errorf("invalid or conflicting mod loader versions")
			}
			pack.preview.Loader, pack.preview.LoaderVersion = strings.TrimSuffix(key, "-loader"), version
		default:
			return nil, fmt.Errorf("unsupported modpack dependency %q", key)
		}
	}

	// Compare destination spellings across layers as well as within a layer:
	// overrides may replace the same file, but never alias a different path.
	type pathUse struct {
		name string
		file bool
	}
	paths := map[string]pathUse{}
	addPath := func(name string) error {
		for p, file := name, true; p != "."; p, file = path.Dir(p), false {
			key := strings.ToLower(p)
			if previous, ok := paths[key]; ok && (previous.name != p || previous.file != file) {
				return fmt.Errorf("conflicting modpack destination %q", name)
			}
			paths[key] = pathUse{p, file}
		}
		return nil
	}
	var total int64
	seen := map[string]bool{}
	for _, f := range manifest.Files {
		client := f.Env["client"]
		if client == "unsupported" {
			continue
		}
		if !packDestination(f.Path) || seen[f.Path] || (client != "" && client != "required" && client != "optional") ||
			f.Size < 0 || f.Size > maxPackArchive || len(f.Downloads) == 0 || len(f.Downloads) > 16 ||
			!isPackDigest(f.Hashes["sha512"], sha512.Size) || !isPackDigest(f.Hashes["sha1"], sha1.Size) {
			return nil, fmt.Errorf("invalid client file %q", f.Path)
		}
		if err := addPath(f.Path); err != nil {
			return nil, err
		}
		if !slices.ContainsFunc(f.Downloads, func(address string) bool {
			u, err := url.Parse(address)
			return err == nil && packDownloadURL(u, false)
		}) {
			return nil, fmt.Errorf("no supported HTTPS download for %q", f.Path)
		}
		seen[f.Path] = true
		total += f.Size
		if client == "optional" {
			pack.preview.Options = append(pack.preview.Options, ModpackOption{f.Path, f.Size})
		} else {
			pack.preview.RequiredBytes += f.Size
		}
		pack.files = append(pack.files, f)
	}
	for level, prefix := range []string{"overrides/", "client-overrides/"} {
		seen := map[string]bool{}
		for _, f := range z.File {
			name, ok := strings.CutPrefix(f.Name, prefix)
			if !ok || f.Mode().IsDir() {
				continue
			}
			if !packDestination(name) || seen[name] || f.UncompressedSize64 > maxPackArchive {
				return nil, fmt.Errorf("invalid override %q", f.Name)
			}
			if err := addPath(name); err != nil {
				return nil, err
			}
			seen[name] = true
			total += int64(f.UncompressedSize64)
			pack.overrides[level] = append(pack.overrides[level], f)
		}
	}
	if total > maxPackContent {
		return nil, fmt.Errorf("modpack content exceeds the total size limit")
	}
	return pack, nil
}
