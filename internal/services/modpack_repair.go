package services

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

const (
	packRepairDir   = ".samuel-modpack"
	packReceiptPath = packRepairDir + "/receipt.json"
	packArchivePath = packRepairDir + "/source.mrpack"
	maxPackReceipt  = 32 << 20
)

type packReceipt struct {
	Preview   ModpackPreview `json:"pack"`
	Source    ModpackSource  `json:"source"`
	Options   []string       `json:"options"`
	JavaMajor int            `json:"java_major"`
	GameFiles []string       `json:"game_files"`
	PackFiles []string       `json:"pack_files"`
}

func makePackReceipt(dir string, pack *modpackArchive, source ModpackSource, options []string, selected map[string]bool, major int) (packReceipt, error) {
	source.LocalPath = ""
	preview := pack.preview
	preview.Options = nil
	receipt := packReceipt{Preview: preview, Source: source, Options: options, JavaMajor: major}
	for _, top := range []string{"versions", "libraries", "assets"} {
		root := filepath.Join(dir, top)
		if _, err := os.Stat(root); os.IsNotExist(err) {
			continue
		}
		if err := filepath.WalkDir(root, func(file string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return err
			}
			if !entry.Type().IsRegular() {
				return nil
			}
			rel, err := filepath.Rel(dir, file)
			if err != nil {
				return err
			}
			receipt.GameFiles = append(receipt.GameFiles, filepath.ToSlash(rel))
			return nil
		}); err != nil {
			return receipt, err
		}
	}
	files := map[string]bool{}
	for _, file := range pack.files {
		if file.Env["client"] != "optional" || selected[file.Path] {
			files[file.Path] = true
		}
	}
	for _, layer := range pack.overrides {
		for _, entry := range layer {
			_, name, _ := strings.Cut(entry.Name, "/")
			files[name] = true
		}
	}
	for name := range files {
		receipt.PackFiles = append(receipt.PackFiles, name)
	}
	slices.Sort(receipt.PackFiles)
	return receipt, nil
}

func writePackReceipt(ctx context.Context, dir string, archive *os.File, receipt packReceipt) error {
	data, err := json.Marshal(receipt)
	if err != nil || len(data) > maxPackReceipt {
		return fmt.Errorf("could not create a repair inventory for this modpack")
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	info, err := archive.Stat()
	if err != nil {
		return err
	}
	if _, err := archive.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if err := writePackFile(ctx, root, packArchivePath, archive, info.Size(), false, nil); err != nil {
		return err
	}
	return writePackFile(ctx, root, packReceiptPath, strings.NewReader(string(data)), int64(len(data)), false, nil)
}

func gamePackPath(name string) bool {
	top, _, _ := strings.Cut(name, "/")
	return safePackPath(name) && (top == "versions" || top == "libraries" || top == "assets")
}

func findMissingPackFiles(ctx context.Context, root *os.Root, names []string, valid func(string) bool, allowDisabled bool) (map[string]bool, error) {
	missing := map[string]bool{}
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !valid(name) {
			return nil, fmt.Errorf("invalid repair path %q", name)
		}
		local, err := filepath.Localize(name)
		if err != nil {
			return nil, err
		}
		info, err := root.Lstat(local)
		if err == nil && info.Mode().IsRegular() {
			continue
		}
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("cannot repair non-regular or inaccessible file %q: %v", name, err)
		}
		if allowDisabled && path.Dir(name) == "mods" && strings.EqualFold(path.Ext(name), ".jar") {
			if backup, err := root.Lstat(local + ".bak"); err == nil && backup.Mode().IsRegular() {
				continue
			}
		}
		missing[name] = true
	}
	return missing, nil
}

// PrepareModpackLaunch checks the inventory even if the selected profile is
// missing. Repair stages new files first and never overwrites existing ones.
func (s *ModrinthService) PrepareModpackLaunch(ctx context.Context, dir string, pack ModpackInstance, installer packGameInstaller,
	resolveJava PackJavaResolver, progress PackProgress, log func(string, string)) (string, error) {
	if !pack.Managed {
		launcher := NewLauncherService()
		meta, err := launcher.loadVersionJSON(pack.VersionID, dir)
		if err != nil {
			return "", err
		}
		if packVersion.MatchString(meta.InheritsFrom) {
			meta, err = launcher.loadVersionJSON(meta.InheritsFrom, dir)
			if err != nil {
				return "", err
			}
		}
		return resolveJava(ctx, meta.JavaVersion.MajorVersion, progress)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return "", err
	}
	defer root.Close()
	f, err := root.Open(packReceiptPath)
	if err != nil {
		return "", err
	}
	var receipt packReceipt
	err = json.NewDecoder(io.LimitReader(f, maxPackReceipt)).Decode(&receipt)
	_ = f.Close()
	p := receipt.Preview
	if err != nil || !packVersion.MatchString(p.GameVersion) ||
		(p.Loader != "vanilla" && ((p.Loader != "forge" && p.Loader != "fabric") || !packLoaderVersion.MatchString(p.LoaderVersion))) {
		return "", fmt.Errorf("invalid modpack repair inventory")
	}
	game, err := findMissingPackFiles(ctx, root, receipt.GameFiles, gamePackPath, false)
	if err != nil {
		return "", err
	}
	content, err := findMissingPackFiles(ctx, root, receipt.PackFiles, packDestination, true)
	if err != nil {
		return "", err
	}
	java, err := resolveJava(ctx, receipt.JavaMajor, progress)
	if err != nil || (len(game) == 0 && len(content) == 0) {
		return java, err
	}

	progress("pack-repair", pack.Name, 0, 0)
	stage, err := os.MkdirTemp(filepath.Dir(dir), ".repair-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	if len(game) > 0 {
		profile, _, _, err := installPackGame(ctx, stage, p, installer, func(context.Context, int, PackProgress) (string, error) {
			return java, nil
		}, progress, log)
		if err != nil {
			return "", err
		}
		if profile != pack.VersionID {
			return "", fmt.Errorf("repaired launch profile %q does not match %q", profile, pack.VersionID)
		}
	}
	if len(content) > 0 {
		if err := s.stagePackRepair(ctx, root, stage, receipt, content, progress); err != nil {
			return "", err
		}
	}
	var done int64
	total := int64(len(game) + len(content))
	for _, files := range []map[string]bool{game, content} {
		for name := range files {
			if err := copyMissingPackFile(ctx, root, stage, name); err != nil {
				return "", fmt.Errorf("restore %q: %w", name, err)
			}
			done++
			progress("pack-repair", pack.Name, done, total)
		}
	}
	return java, nil
}

func (s *ModrinthService) stagePackRepair(ctx context.Context, root *os.Root, stage string, receipt packReceipt, missing map[string]bool, progress PackProgress) error {
	var archive *modpackArchive
	f, err := root.Open(packArchivePath)
	if os.IsNotExist(err) && receipt.Source.ProjectID != "" {
		f, archive, err = s.openModpack(ctx, receipt.Source, progress)
		if err != nil {
			return err
		}
		defer closePack(f)
	} else if err != nil {
		return err
	} else {
		defer f.Close()
		info, err := f.Stat()
		if err != nil || !info.Mode().IsRegular() || info.Size() > maxPackArchive {
			return fmt.Errorf("invalid cached modpack archive")
		}
		digest := sha256.New()
		if _, err := io.Copy(digest, packReader{ctx: ctx, Reader: io.LimitReader(f, maxPackArchive+1)}); err != nil {
			return err
		}
		z, err := zip.NewReader(f, info.Size())
		if err != nil {
			return err
		}
		archive, err = parseModpack(z)
		if err != nil {
			return err
		}
		archive.preview.Fingerprint = fmt.Sprintf("%x", digest.Sum(nil))
	}
	if archive.preview.Fingerprint != receipt.Preview.Fingerprint {
		return fmt.Errorf("modpack repair archive checksum mismatch")
	}
	// An override is the final content for its path; repairing that file
	// does not require downloading the superseded base file.
	overrides := map[string]bool{}
	for i, layer := range archive.overrides {
		archive.overrides[i] = slices.DeleteFunc(layer, func(entry *zip.File) bool {
			_, name, _ := strings.Cut(entry.Name, "/")
			if missing[name] {
				overrides[name] = true
			}
			return !missing[name]
		})
	}
	archive.files = slices.DeleteFunc(archive.files, func(file modpackFile) bool {
		return !missing[file.Path] || overrides[file.Path]
	})
	selected := map[string]bool{}
	for _, name := range receipt.Options {
		selected[name] = true
	}
	return s.installModpackContent(ctx, stage, archive, selected, progress)
}

func copyMissingPackFile(ctx context.Context, root *os.Root, stage, name string) error {
	file, err := os.Open(filepath.Join(stage, filepath.FromSlash(name)))
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("repair source is not a regular file")
	}
	if err := writePackFile(ctx, root, name, file, info.Size(), false, nil); os.IsExist(err) {
		local, localErr := filepath.Localize(name)
		if localErr == nil {
			if existing, statErr := root.Lstat(local); statErr == nil && existing.Mode().IsRegular() {
				return nil
			}
		}
		return err
	} else {
		return err
	}
}
