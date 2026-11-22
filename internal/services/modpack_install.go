package services

import (
	"context"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"HRPMLW/internal/models"
)

type packGameInstaller interface {
	InstallVersion(context.Context, string, string, func(string, string, int64, int64)) *models.ApiResponse
	InstallFabricLoader(context.Context, string, string, string, func(string, string, int64, int64)) *models.ApiResponse
	InstallForgeLoader(context.Context, string, string, string, string, func(string, string), func(string, string, int64, int64)) *models.ApiResponse
}

type ModpackInstallResult struct {
	ModpackInstance
	GameDir   string `json:"game_dir"`
	JavaMajor int    `json:"java_major,omitempty"`
}

func ReadModpackInstance(gameDir string) (ModpackInstance, bool) {
	var pack ModpackInstance
	f, err := os.Open(filepath.Join(gameDir, packMarker))
	if err != nil {
		return pack, false
	}
	defer f.Close()
	err = json.NewDecoder(io.LimitReader(f, 8192)).Decode(&pack)
	return pack, err == nil && pack.Name != "" && packVersion.MatchString(pack.VersionID)
}

func (s *ModrinthService) InstallModpack(ctx context.Context, source ModpackSource, fingerprint string, options []string,
	parent string, installer packGameInstaller, javaPaths []string, register func(string) error,
	progress PackProgress, log func(string, string)) (ModpackInstallResult, error) {
	var result ModpackInstallResult
	if !isPackDigest(fingerprint, 32) {
		return result, fmt.Errorf("preview the modpack before installing it")
	}
	if progress == nil {
		progress = func(string, string, int64, int64) {}
	}
	progress("pack-archive", "Modrinth", 0, 0)
	f, pack, err := s.openModpack(ctx, source, progress)
	if err != nil {
		return result, err
	}
	defer closePack(f)
	if pack.preview.Fingerprint != fingerprint {
		return result, fmt.Errorf("the .mrpack has changed; preview it again")
	}
	available := make(map[string]bool, len(pack.preview.Options))
	for _, option := range pack.preview.Options {
		available[option.Path] = true
	}
	selected := make(map[string]bool, len(options))
	for _, name := range options {
		if !available[name] || selected[name] {
			return result, fmt.Errorf("invalid optional file selection")
		}
		selected[name] = true
	}
	if err := os.MkdirAll(parent, 0755); err != nil {
		return result, err
	}
	staging, err := os.MkdirTemp(parent, ".import-")
	if err != nil {
		return result, err
	}
	defer os.RemoveAll(staging)

	progress("jar", pack.preview.Name, 0, 0)
	game := pack.preview.GameVersion
	if res := installer.InstallVersion(ctx, game, staging, progress); !res.Success {
		return result, fmt.Errorf("install Minecraft %s: %s", game, res.Error)
	}
	profile := game
	meta, err := NewLauncherService().loadVersionJSON(game, staging)
	if err != nil {
		return result, err
	}
	var loader *models.ApiResponse
	switch pack.preview.Loader {
	case "fabric":
		loader = installer.InstallFabricLoader(ctx, game, pack.preview.LoaderVersion, staging, progress)
	case "forge":
		java, err := packInstallerJava(javaPaths, meta.JavaVersion.MajorVersion)
		if err != nil {
			return result, err
		}
		loader = installer.InstallForgeLoader(ctx, game, pack.preview.LoaderVersion, staging, java, log, progress)
	}
	if loader != nil {
		if !loader.Success {
			return result, fmt.Errorf("install %s %s: %s", pack.preview.Loader, pack.preview.LoaderVersion, loader.Error)
		}
		data, _ := loader.Data.(map[string]interface{})
		profile, _ = data["version_id"].(string)
		if !packVersion.MatchString(profile) {
			return result, fmt.Errorf("loader did not produce a valid launch profile")
		}
		child, err := NewLauncherService().loadVersionJSON(profile, staging)
		if err != nil || child.InheritsFrom != game {
			return result, fmt.Errorf("loader profile does not inherit Minecraft %s", game)
		}
	}
	if info, err := os.Stat(filepath.Join(staging, "versions", game, game+".jar")); err != nil || !info.Mode().IsRegular() {
		return result, fmt.Errorf("Minecraft client JAR is missing")
	}
	if err := s.installModpackContent(ctx, staging, pack, selected, progress); err != nil {
		return result, err
	}
	instance := ModpackInstance{pack.preview.Name, profile}
	marker, err := json.Marshal(instance)
	if err != nil {
		return result, err
	}
	if err := os.WriteFile(filepath.Join(staging, packMarker), marker, 0644); err != nil {
		return result, err
	}
	dest := filepath.Join(parent, "pack-"+strings.TrimPrefix(filepath.Base(staging), ".import-"))
	if _, err := os.Lstat(dest); !os.IsNotExist(err) {
		return result, fmt.Errorf("instance destination is not available")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if err := os.Rename(staging, dest); err != nil {
		return result, err
	}
	if err := register(dest); err != nil {
		if cleanupErr := os.RemoveAll(dest); cleanupErr != nil {
			return result, fmt.Errorf("register instance: %v; could not remove %s: %w", err, dest, cleanupErr)
		}
		return result, fmt.Errorf("register instance: %w", err)
	}
	return ModpackInstallResult{instance, dest, meta.JavaVersion.MajorVersion}, nil
}

func packInstallerJava(paths []string, major int) (string, error) {
	for _, path := range paths {
		version, err := DetectJavaVersion(path)
		if err != nil {
			continue
		}
		number := strings.TrimPrefix(version, "1.")
		number, _, _ = strings.Cut(number, ".")
		number, _, _ = strings.Cut(number, "-")
		actual, _ := strconv.Atoi(number)
		if actual > 0 && (major == 0 || actual == major || (major == 16 && actual == 17)) {
			return path, nil
		}
	}
	if major > 0 {
		return "", fmt.Errorf("Forge requires Java %d; add a compatible Java runtime in Settings and retry", major)
	}
	return "", fmt.Errorf("Forge needs Java; add a Java runtime in Settings and retry")
}

func (s *ModrinthService) installModpackContent(ctx context.Context, dir string, pack *modpackArchive, selected map[string]bool, progress PackProgress) error {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	client := *s.client
	client.Timeout = 5 * time.Minute
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 6 || !packDownloadURL(req.URL, true) {
			return fmt.Errorf("unsupported modpack download redirect")
		}
		return nil
	}
	service := &ModrinthService{client: &client}
	var done, total int64
	include := func(file modpackFile) bool { return file.Env["client"] != "optional" || selected[file.Path] }
	for _, file := range pack.files {
		if include(file) {
			total += file.Size
		}
	}
	if len(pack.files) > 0 {
		progress("pack-files", pack.preview.Name, 0, total)
	}
	for _, file := range pack.files {
		if !include(file) {
			continue
		}
		if err := service.downloadModpackFile(ctx, root, file); err != nil {
			return err
		}
		done += file.Size
		progress("pack-files", file.Path, done, total)
	}
	written := map[string]bool{}
	for _, file := range pack.files {
		if include(file) {
			written[file.Path] = true
		}
	}
	var count, extracted int64
	for _, layer := range pack.overrides {
		count += int64(len(layer))
	}
	if count > 0 {
		progress("pack-overrides", pack.preview.Name, 0, count)
	}
	for level, layer := range pack.overrides {
		prefix := []string{"overrides/", "client-overrides/"}[level]
		for _, entry := range layer {
			name := strings.TrimPrefix(entry.Name, prefix)
			r, err := entry.Open()
			if err != nil {
				return err
			}
			err = writePackFile(ctx, root, name, r, int64(entry.UncompressedSize64), written[name], nil)
			_ = r.Close()
			if err != nil {
				return fmt.Errorf("extract %q: %w", name, err)
			}
			written[name] = true
			extracted++
			progress("pack-overrides", name, extracted, count)
		}
	}
	return ctx.Err()
}

func (s *ModrinthService) downloadModpackFile(ctx context.Context, root *os.Root, file modpackFile) error {
	var last error
	for _, address := range file.Downloads {
		if err := ctx.Err(); err != nil {
			return err
		}
		u, err := url.Parse(address)
		if err != nil || !packDownloadURL(u, false) {
			continue
		}
		resp, err := s.get(ctx, address)
		if err != nil {
			last = err
			continue
		}
		h1, h512 := sha1.New(), sha512.New()
		last = writePackFile(ctx, root, file.Path, io.TeeReader(resp.Body, io.MultiWriter(h1, h512)), file.Size, false, func() error {
			if !strings.EqualFold(hex.EncodeToString(h1.Sum(nil)), file.Hashes["sha1"]) ||
				!strings.EqualFold(hex.EncodeToString(h512.Sum(nil)), file.Hashes["sha512"]) {
				return fmt.Errorf("download checksum mismatch")
			}
			return nil
		})
		_ = resp.Body.Close()
		if last == nil {
			return nil
		}
	}
	if last == nil {
		last = fmt.Errorf("no supported download source")
	}
	return fmt.Errorf("download %q: %w", file.Path, last)
}

func writePackFile(ctx context.Context, root *os.Root, name string, r io.Reader, size int64, replace bool, verify func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	name, err := filepath.Localize(name)
	if err != nil {
		return err
	}
	if err := root.MkdirAll(filepath.Dir(name), 0755); err != nil {
		return err
	}
	if replace {
		if err := root.Remove(name); err != nil {
			return err
		}
	}
	f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, packReader{ctx: ctx, Reader: io.LimitReader(r, size+1)})
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil && n != size {
		err = fmt.Errorf("expected %d bytes, read %d", size, n)
	}
	if err == nil && verify != nil {
		err = verify()
	}
	if err != nil {
		_ = root.Remove(name)
	}
	return err
}
