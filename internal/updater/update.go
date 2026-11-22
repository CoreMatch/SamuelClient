package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	defaultLatestReleaseURL = "https://api.github.com/repos/CoreMatch/SamuelClient/releases/latest"
	maxReleaseAssetSize     = 500 << 20
)

var versionPattern = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z.-]+))?(?:\+[0-9A-Za-z.-]+)?$`)
var releaseExecutablePattern = regexp.MustCompile(`^SamuelClient-(v?\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?)$`)

type release struct {
	TagName string         `json:"tag_name"`
	Assets  []releaseAsset `json:"assets"`
}

type releaseAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Hash string `json:"digest"`
}

type version struct {
	major, minor, patch uint64
	prerelease          string
}

// DetectVersion prefers the version embedded in a release build and falls
// back to the version in the official published executable filename.
func DetectVersion(buildVersion, executablePath string) string {
	if _, err := parseVersion(buildVersion); err == nil {
		return buildVersion
	}
	name := filepath.Base(executablePath)
	for _, suffix := range []string{
		"-linux-amd64", "-linux-arm64", "-windows-amd64.exe", "-windows-arm64.exe",
	} {
		name = strings.TrimSuffix(name, suffix)
	}
	match := releaseExecutablePattern.FindStringSubmatch(name)
	if len(match) == 2 {
		if _, err := parseVersion(match[1]); err == nil {
			return match[1]
		}
	}
	return buildVersion
}

// CheckAndStage checks the latest stable release and writes a verified update
// beside the running executable so it can be atomically installed later.
func CheckAndStage(ctx context.Context, currentVersion, executablePath, latestReleaseURL string, client *http.Client) (string, string, error) {
	current, err := parseVersion(currentVersion)
	if err != nil {
		return "", "", nil
	}
	if latestReleaseURL == "" {
		latestReleaseURL = defaultLatestReleaseURL
	}
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Minute}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, latestReleaseURL, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "SamuelClient-Updater")
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("release check returned HTTP %d", resp.StatusCode)
	}
	var latest release
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&latest); err != nil {
		return "", "", fmt.Errorf("decode latest release: %w", err)
	}
	available, err := parseVersion(latest.TagName)
	if err != nil {
		return "", "", fmt.Errorf("invalid release tag %q", latest.TagName)
	}
	if compareVersions(available, current) <= 0 {
		return "", latest.TagName, nil
	}

	asset, err := findAsset(latest.Assets, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return "", latest.TagName, err
	}
	downloadURL, err := url.Parse(asset.URL)
	if err != nil || downloadURL.Scheme != "https" || (downloadURL.Host != "github.com" && downloadURL.Host != "objects.githubusercontent.com") {
		return "", latest.TagName, errors.New("release asset has an unsupported download URL")
	}
	digestParts := strings.SplitN(asset.Hash, ":", 2)
	if len(digestParts) != 2 || digestParts[0] != "sha256" || len(digestParts[1]) != sha256.Size*2 {
		return "", latest.TagName, errors.New("release asset is missing a valid SHA-256 digest")
	}
	expectedDigest, err := hex.DecodeString(digestParts[1])
	if err != nil {
		return "", latest.TagName, errors.New("release asset has an invalid SHA-256 digest")
	}

	req, err = http.NewRequestWithContext(ctx, http.MethodGet, asset.URL, nil)
	if err != nil {
		return "", latest.TagName, err
	}
	req.Header.Set("User-Agent", "SamuelClient-Updater")
	resp, err = client.Do(req)
	if err != nil {
		return "", latest.TagName, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", latest.TagName, fmt.Errorf("release download returned HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > maxReleaseAssetSize {
		return "", latest.TagName, errors.New("release asset exceeds the maximum supported size")
	}

	executablePath, err = filepath.EvalSymlinks(executablePath)
	if err != nil {
		return "", latest.TagName, fmt.Errorf("resolve executable path: %w", err)
	}
	info, err := os.Stat(executablePath)
	if err != nil {
		return "", latest.TagName, fmt.Errorf("stat executable: %w", err)
	}
	staged, err := os.CreateTemp(filepath.Dir(executablePath), ".SamuelClient-update-*")
	if err != nil {
		return "", latest.TagName, fmt.Errorf("create staged update: %w", err)
	}
	stagedPath := staged.Name()
	removeStaged := true
	defer func() {
		if removeStaged {
			_ = os.Remove(stagedPath)
		}
	}()

	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(staged, hash), io.LimitReader(resp.Body, maxReleaseAssetSize+1))
	if err != nil {
		_ = staged.Close()
		return "", latest.TagName, fmt.Errorf("download release asset: %w", err)
	}
	if written == 0 || written > maxReleaseAssetSize {
		_ = staged.Close()
		return "", latest.TagName, errors.New("release asset has an invalid size")
	}
	if err := staged.Chmod(info.Mode().Perm() | 0111); err != nil {
		_ = staged.Close()
		return "", latest.TagName, fmt.Errorf("set staged executable permissions: %w", err)
	}
	if err := staged.Sync(); err != nil {
		_ = staged.Close()
		return "", latest.TagName, fmt.Errorf("flush staged update: %w", err)
	}
	if err := staged.Close(); err != nil {
		return "", latest.TagName, fmt.Errorf("close staged update: %w", err)
	}
	if !equalBytes(hash.Sum(nil), expectedDigest) {
		return "", latest.TagName, errors.New("release asset SHA-256 digest does not match")
	}

	removeStaged = false
	return stagedPath, latest.TagName, nil
}

func findAsset(assets []releaseAsset, goos, goarch string) (releaseAsset, error) {
	suffix := "-" + goos + "-" + goarch
	if goos == "windows" {
		suffix += ".exe"
	}
	for _, asset := range assets {
		if strings.HasSuffix(asset.Name, suffix) {
			return asset, nil
		}
	}
	return releaseAsset{}, fmt.Errorf("no release executable is available for %s/%s", goos, goarch)
}

func parseVersion(raw string) (version, error) {
	matches := versionPattern.FindStringSubmatch(strings.TrimSpace(raw))
	if matches == nil {
		return version{}, fmt.Errorf("unsupported version %q", raw)
	}
	major, err := strconv.ParseUint(matches[1], 10, 64)
	if err != nil {
		return version{}, fmt.Errorf("invalid version %q", raw)
	}
	minor, err := strconv.ParseUint(matches[2], 10, 64)
	if err != nil {
		return version{}, fmt.Errorf("invalid version %q", raw)
	}
	patch, err := strconv.ParseUint(matches[3], 10, 64)
	if err != nil {
		return version{}, fmt.Errorf("invalid version %q", raw)
	}
	return version{major: major, minor: minor, patch: patch, prerelease: matches[4]}, nil
}

func compareVersions(a, b version) int {
	for _, pair := range [][2]uint64{{a.major, b.major}, {a.minor, b.minor}, {a.patch, b.patch}} {
		if pair[0] > pair[1] {
			return 1
		}
		if pair[0] < pair[1] {
			return -1
		}
	}
	if a.prerelease == b.prerelease {
		return 0
	}
	if a.prerelease == "" {
		return 1
	}
	if b.prerelease == "" {
		return -1
	}
	if a.prerelease > b.prerelease {
		return 1
	}
	return -1
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := range a {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}
