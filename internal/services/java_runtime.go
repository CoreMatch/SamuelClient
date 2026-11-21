package services

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"HRPMLW/internal/database"
)

const adoptiumAPIBaseURL = "https://api.adoptium.net/v3/binary/latest"
const maxJavaArchiveBytes = 1024 * 1024 * 1024
const maxJavaExtractedBytes = 4 * 1024 * 1024 * 1024

var supportedJavaRuntimeMajors = map[int]bool{
	8: true, 11: true, 17: true, 21: true, 25: true,
}

// JavaRuntimeInstaller downloads supported Temurin JRE archives and installs
// them into a user-selected directory.
type JavaRuntimeInstaller struct {
	APIBaseURL string
	HTTPClient *http.Client
	OS         string
	Arch       string
}

type JavaRuntimeProgress func(stage string, downloaded, total int64)

type JavaRuntimeInstall struct {
	JavaPath string `json:"java_path"`
	Version  int    `json:"version"`
}

func NewJavaRuntimeInstaller() *JavaRuntimeInstaller {
	return &JavaRuntimeInstaller{
		APIBaseURL: adoptiumAPIBaseURL,
		HTTPClient: &http.Client{Timeout: 30 * time.Minute},
		OS:         runtime.GOOS,
		Arch:       runtime.GOARCH,
	}
}

// SupportedJavaRuntimeMajors returns the maintained Temurin LTS majors
// offered by the Java runtime installer.
func SupportedJavaRuntimeMajors() []int {
	return []int{8, 11, 17, 21, 25}
}

// DefaultJavaRuntimeDir follows the launcher's portable data directory.
func DefaultJavaRuntimeDir(major int) (string, error) {
	if !supportedJavaRuntimeMajors[major] {
		return "", fmt.Errorf("unsupported Java version %d", major)
	}
	return filepath.Join(database.DataDir(), "jre", fmt.Sprintf("%d", major)), nil
}

func (i *JavaRuntimeInstaller) Install(ctx context.Context, major int, destination string, progress JavaRuntimeProgress) (*JavaRuntimeInstall, error) {
	if !supportedJavaRuntimeMajors[major] {
		return nil, fmt.Errorf("unsupported Java version %d", major)
	}
	destination = strings.TrimSpace(destination)
	if destination == "" || !filepath.IsAbs(destination) {
		return nil, errors.New("Java install path must be absolute")
	}
	destination = filepath.Clean(destination)
	if _, err := os.Lstat(destination); err == nil {
		return nil, fmt.Errorf("Java install path already exists: %s", destination)
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("check Java install path: %w", err)
	}

	osName, archiveKind, err := javaArchiveOS(i.OS)
	if err != nil {
		return nil, err
	}
	arch, err := javaArchiveArch(i.Arch)
	if err != nil {
		return nil, err
	}
	parent := filepath.Dir(destination)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return nil, fmt.Errorf("create Java install parent: %w", err)
	}
	archiveFile, err := os.CreateTemp(parent, ".temurin-*.archive")
	if err != nil {
		return nil, fmt.Errorf("create Java archive temp file: %w", err)
	}
	archivePath := archiveFile.Name()
	defer os.Remove(archivePath)

	client := i.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	apiBaseURL := i.APIBaseURL
	if apiBaseURL == "" {
		apiBaseURL = adoptiumAPIBaseURL
	}
	url := fmt.Sprintf("%s/%d/ga/%s/%s/jre/hotspot/normal/eclipse", strings.TrimRight(apiBaseURL, "/"), major, osName, arch)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		archiveFile.Close()
		return nil, fmt.Errorf("create Temurin request: %w", err)
	}
	req.Header.Set("User-Agent", "SamuelClient/1.0")
	resp, err := client.Do(req)
	if err != nil {
		archiveFile.Close()
		return nil, fmt.Errorf("download Temurin Java %d: %w", major, err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		archiveFile.Close()
		return nil, fmt.Errorf("Temurin API returned HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > maxJavaArchiveBytes {
		resp.Body.Close()
		archiveFile.Close()
		return nil, fmt.Errorf("Temurin archive exceeds the %d-byte download limit", maxJavaArchiveBytes)
	}
	if progress != nil {
		progress("java-download", 0, resp.ContentLength)
	}
	checksumURL, checksumErr := javaChecksumURL(resp)
	sha256Writer := sha256.New()
	downloaded, copyErr := io.Copy(io.MultiWriter(archiveFile, sha256Writer), &progressReader{
		reader: io.LimitReader(resp.Body, maxJavaArchiveBytes+1),
		total:  resp.ContentLength,
		progress: func(n, total int64) {
			if progress != nil {
				progress("java-download", n, total)
			}
		},
	})
	bodyCloseErr := resp.Body.Close()
	fileCloseErr := archiveFile.Close()
	if copyErr != nil {
		return nil, fmt.Errorf("download Temurin archive: %w", copyErr)
	}
	if downloaded > maxJavaArchiveBytes {
		return nil, fmt.Errorf("Temurin archive exceeds the %d-byte download limit", maxJavaArchiveBytes)
	}
	if checksumErr != nil {
		return nil, checksumErr
	}
	if bodyCloseErr != nil {
		return nil, fmt.Errorf("close Temurin response: %w", bodyCloseErr)
	}
	if fileCloseErr != nil {
		return nil, fmt.Errorf("close Temurin archive: %w", fileCloseErr)
	}
	if downloaded == 0 {
		return nil, errors.New("Temurin returned an empty Java archive")
	}
	expectedHash, err := fetchJavaChecksum(ctx, client, checksumURL)
	if err != nil {
		return nil, err
	}
	if actualHash := hex.EncodeToString(sha256Writer.Sum(nil)); !strings.EqualFold(expectedHash, actualHash) {
		return nil, errors.New("Temurin Java archive SHA-256 checksum mismatch")
	}

	stage, err := os.MkdirTemp(parent, ".jre-install-*")
	if err != nil {
		return nil, fmt.Errorf("create Java staging directory: %w", err)
	}
	defer os.RemoveAll(stage)
	if progress != nil {
		progress("java-extract", downloaded, 0)
	}
	if err := extractJavaArchive(ctx, archivePath, archiveKind, stage); err != nil {
		return nil, fmt.Errorf("extract Temurin Java %d: %w", major, err)
	}

	javaName := "java"
	javaRelativePath := filepath.Join("bin", javaName)
	if i.OS == "windows" {
		javaName = "java.exe"
		javaRelativePath = filepath.Join("bin", javaName)
	} else if i.OS == "darwin" {
		javaRelativePath = filepath.Join("Contents", "Home", "bin", javaName)
	}
	javaPath := filepath.Join(stage, javaRelativePath)
	if i.OS != "windows" {
		if err := os.Chmod(javaPath, 0755); err != nil {
			return nil, fmt.Errorf("make Java executable: %w", err)
		}
	}
	info, err := os.Stat(javaPath)
	if err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("Temurin archive is missing %s", filepath.ToSlash(javaRelativePath))
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := os.Rename(stage, destination); err != nil {
		return nil, fmt.Errorf("move Java runtime into place: %w", err)
	}
	if progress != nil {
		progress("java-extract", downloaded, downloaded)
	}
	return &JavaRuntimeInstall{JavaPath: filepath.Join(destination, javaRelativePath), Version: major}, nil
}

func javaChecksumURL(resp *http.Response) (*url.URL, error) {
	if resp.Request == nil || resp.Request.URL == nil {
		return nil, errors.New("Temurin response is missing its final download URL")
	}
	u := *resp.Request.URL
	u.Path += ".sha256.txt"
	u.RawQuery = ""
	u.Fragment = ""
	return &u, nil
}

func fetchJavaChecksum(ctx context.Context, client *http.Client, checksumURL *url.URL) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, checksumURL.String(), nil)
	if err != nil {
		return "", fmt.Errorf("create Temurin checksum request: %w", err)
	}
	req.Header.Set("User-Agent", "SamuelClient/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download Temurin checksum: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Temurin checksum endpoint returned HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return "", fmt.Errorf("read Temurin checksum: %w", err)
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 || len(fields[0]) != sha256.Size*2 {
		return "", errors.New("Temurin returned an invalid SHA-256 checksum")
	}
	if _, err := hex.DecodeString(fields[0]); err != nil {
		return "", errors.New("Temurin returned an invalid SHA-256 checksum")
	}
	return fields[0], nil
}

type progressReader struct {
	reader   io.Reader
	total    int64
	read     int64
	progress func(int64, int64)
}

func (r *progressReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.read += int64(n)
	if n > 0 && r.progress != nil {
		r.progress(r.read, r.total)
	}
	return n, err
}

func javaArchiveOS(goos string) (string, string, error) {
	switch goos {
	case "linux":
		return "linux", "tar.gz", nil
	case "darwin":
		return "mac", "tar.gz", nil
	case "windows":
		return "windows", "zip", nil
	default:
		return "", "", fmt.Errorf("Temurin Java installation is unsupported on %s", goos)
	}
}

func javaArchiveArch(goarch string) (string, error) {
	switch goarch {
	case "amd64":
		return "x64", nil
	case "arm64":
		return "aarch64", nil
	case "arm":
		return "arm", nil
	case "ppc64le":
		return "ppc64le", nil
	case "s390x":
		return "s390x", nil
	case "riscv64":
		return "riscv64", nil
	default:
		return "", fmt.Errorf("Temurin Java installation is unsupported on architecture %s", goarch)
	}
}

func extractJavaArchive(ctx context.Context, archive, kind, destination string) error {
	if kind == "zip" {
		return extractJavaZip(ctx, archive, destination)
	}
	return extractJavaTarGz(ctx, archive, destination)
}

func stripArchiveRoot(name, root string) (string, string, error) {
	if strings.Contains(name, "\\") || path.IsAbs(name) {
		return "", root, fmt.Errorf("unsafe archive entry %q", name)
	}
	clean := path.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", root, fmt.Errorf("unsafe archive entry %q", name)
	}
	parts := strings.Split(clean, "/")
	if len(parts) < 2 {
		return "", root, nil
	}
	if root == "" {
		root = parts[0]
	} else if root != parts[0] {
		return "", root, fmt.Errorf("archive contains multiple root directories")
	}
	return filepath.Join(parts[1:]...), root, nil
}

func extractJavaZip(ctx context.Context, archive, destination string) error {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer zr.Close()
	var root string
	var extractedBytes int64
	for _, file := range zr.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, nextRoot, err := stripArchiveRoot(file.Name, root)
		if err != nil {
			return err
		}
		root = nextRoot
		if rel == "" {
			continue
		}
		target := filepath.Join(destination, rel)
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
			continue
		}
		if err := addExtractedJavaSize(&extractedBytes, int64(file.UncompressedSize64)); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		src, err := file.Open()
		if err != nil {
			return err
		}
		mode := file.Mode().Perm()
		if mode == 0 {
			mode = 0644
		}
		dst, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
		if err != nil {
			src.Close()
			return err
		}
		_, copyErr := io.Copy(dst, contextReader{ctx: ctx, reader: src})
		src.Close()
		closeErr := dst.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func extractJavaTarGz(ctx context.Context, archive, destination string) error {
	file, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	var root string
	var extractedBytes int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		header, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		rel, nextRoot, err := stripArchiveRoot(header.Name, root)
		if err != nil {
			return err
		}
		root = nextRoot
		if rel == "" {
			continue
		}
		target := filepath.Join(destination, rel)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := addExtractedJavaSize(&extractedBytes, header.Size); err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			mode := os.FileMode(header.Mode).Perm()
			if mode == 0 {
				mode = 0644
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(out, contextReader{ctx: ctx, reader: tr})
			closeErr := out.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		case tar.TypeSymlink:
			link := filepath.FromSlash(header.Linkname)
			resolved := path.Clean(path.Join(path.Dir(filepath.ToSlash(rel)), header.Linkname))
			if path.IsAbs(header.Linkname) || resolved == ".." || strings.HasPrefix(resolved, "../") {
				return fmt.Errorf("unsafe archive symlink %q", header.Name)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			if err := os.Symlink(link, target); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported archive entry type for %q", header.Name)
		}
	}
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func addExtractedJavaSize(total *int64, size int64) error {
	if size < 0 || size > maxJavaExtractedBytes-*total {
		return fmt.Errorf("Java archive exceeds the %d-byte extraction limit", maxJavaExtractedBytes)
	}
	*total += size
	return nil
}
