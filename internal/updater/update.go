package updater

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var versionPattern = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z.-]+))?(?:\+[0-9A-Za-z.-]+)?$`)
var releaseExecutablePattern = regexp.MustCompile(`^SamuelClient-(v?\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?)$`)

type version struct {
	major, minor, patch uint64
	prerelease          []string
}

type ProgressFunc func(downloaded, total int64)

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

func DownloadAndStage(ctx context.Context, update *Update, executablePath string, client *http.Client, progress ProgressFunc) (string, error) {
	if update == nil {
		return "", fmt.Errorf("no update selected")
	}
	assetURL, err := url.Parse(update.Asset.URL)
	if err != nil || assetURL.Scheme == "" || assetURL.Host == "" {
		return "", fmt.Errorf("invalid update asset URL")
	}
	expectedName := fmt.Sprintf("SamuelClient-%s-%s-%s", update.Version, update.Asset.OS, update.Asset.Arch)
	if update.Asset.OS == "windows" {
		expectedName += ".exe"
	}
	expectedPath := fmt.Sprintf("/v1/releases/%s/%s-%s/%s", update.Version, update.Asset.OS, update.Asset.Arch, expectedName)
	if update.Asset.Name != expectedName || assetURL.Path != expectedPath || assetURL.RawQuery != "" || assetURL.Fragment != "" {
		return "", fmt.Errorf("update asset does not match the signed manifest")
	}
	if update.Asset.Size <= 0 || update.Asset.Size > maxAssetSize {
		return "", fmt.Errorf("update asset has an invalid size")
	}
	expectedDigest, err := hex.DecodeString(update.Asset.SHA256)
	if err != nil || len(expectedDigest) != sha256.Size {
		return "", fmt.Errorf("update asset has an invalid digest")
	}

	executablePath, err = filepath.EvalSymlinks(executablePath)
	if err != nil {
		return "", fmt.Errorf("resolve executable path: %w", err)
	}
	info, err := os.Stat(executablePath)
	if err != nil {
		return "", fmt.Errorf("stat executable: %w", err)
	}
	staged, err := os.CreateTemp(filepath.Dir(executablePath), ".SamuelClient-update-*")
	if err != nil {
		return "", fmt.Errorf("create staged update: %w", err)
	}
	stagedPath := staged.Name()
	removeStaged := true
	defer func() {
		if removeStaged {
			_ = os.Remove(stagedPath)
		}
	}()

	client = securedClient(client, assetURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL.String(), nil)
	if err != nil {
		_ = staged.Close()
		return "", err
	}
	req.Header.Set("User-Agent", "SamuelClient-Updater")
	resp, err := client.Do(req)
	if err != nil {
		_ = staged.Close()
		return "", fmt.Errorf("download update: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_ = staged.Close()
		return "", fmt.Errorf("update download returned HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength >= 0 && resp.ContentLength != update.Asset.Size {
		_ = staged.Close()
		return "", fmt.Errorf("update download size does not match the signed manifest")
	}

	hash := sha256.New()
	counter := &progressWriter{total: update.Asset.Size, progress: progress}
	written, err := io.CopyBuffer(io.MultiWriter(staged, hash, counter), io.LimitReader(resp.Body, maxAssetSize+1), make([]byte, 32<<10))
	if err != nil {
		_ = staged.Close()
		return "", fmt.Errorf("download update: %w", err)
	}
	if written != update.Asset.Size {
		_ = staged.Close()
		return "", fmt.Errorf("update download size does not match the signed manifest")
	}
	if subtle.ConstantTimeCompare(hash.Sum(nil), expectedDigest) != 1 {
		_ = staged.Close()
		return "", fmt.Errorf("update SHA-256 does not match the signed manifest")
	}
	if err := staged.Chmod(info.Mode().Perm() | 0o111); err != nil {
		_ = staged.Close()
		return "", fmt.Errorf("set staged executable permissions: %w", err)
	}
	if err := staged.Sync(); err != nil {
		_ = staged.Close()
		return "", fmt.Errorf("flush staged update: %w", err)
	}
	if err := staged.Close(); err != nil {
		return "", fmt.Errorf("close staged update: %w", err)
	}
	counter.emit(true)
	removeStaged = false
	return stagedPath, nil
}

type progressWriter struct {
	total    int64
	written  int64
	lastEmit time.Time
	progress ProgressFunc
}

func (w *progressWriter) Write(data []byte) (int, error) {
	count := len(data)
	w.written += int64(count)
	w.emit(w.written == w.total)
	return count, nil
}

func (w *progressWriter) emit(force bool) {
	if w.progress == nil {
		return
	}
	now := time.Now()
	if !force && now.Sub(w.lastEmit) < 100*time.Millisecond {
		return
	}
	w.lastEmit = now
	w.progress(w.written, w.total)
}

func parseVersion(raw string) (version, error) {
	matches := versionPattern.FindStringSubmatch(strings.TrimSpace(raw))
	if matches == nil {
		return version{}, fmt.Errorf("unsupported version %q", raw)
	}
	values := make([]uint64, 3)
	for index := range values {
		value, err := strconv.ParseUint(matches[index+1], 10, 64)
		if err != nil {
			return version{}, fmt.Errorf("invalid version %q", raw)
		}
		values[index] = value
	}
	parsed := version{major: values[0], minor: values[1], patch: values[2]}
	if matches[4] != "" {
		parsed.prerelease = strings.Split(matches[4], ".")
		for _, part := range parsed.prerelease {
			if part == "" {
				return version{}, fmt.Errorf("invalid prerelease version %q", raw)
			}
		}
	}
	return parsed, nil
}

func compareVersions(left, right version) int {
	for _, pair := range [][2]uint64{{left.major, right.major}, {left.minor, right.minor}, {left.patch, right.patch}} {
		if pair[0] > pair[1] {
			return 1
		}
		if pair[0] < pair[1] {
			return -1
		}
	}
	return comparePrerelease(left.prerelease, right.prerelease)
}

func comparePrerelease(left, right []string) int {
	if len(left) == 0 && len(right) == 0 {
		return 0
	}
	if len(left) == 0 {
		return 1
	}
	if len(right) == 0 {
		return -1
	}
	for index := 0; index < len(left) && index < len(right); index += 1 {
		a, b := left[index], right[index]
		if a == b {
			continue
		}
		aNumber, aErr := strconv.ParseUint(a, 10, 64)
		bNumber, bErr := strconv.ParseUint(b, 10, 64)
		if aErr == nil && bErr == nil {
			if aNumber > bNumber {
				return 1
			}
			return -1
		}
		if aErr == nil {
			return -1
		}
		if bErr == nil {
			return 1
		}
		if a > b {
			return 1
		}
		return -1
	}
	if len(left) > len(right) {
		return 1
	}
	if len(left) < len(right) {
		return -1
	}
	return 0
}
