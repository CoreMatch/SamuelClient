package updater

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	DefaultManifestURL = "https://mlup.mcnb.dev/v1/channels/stable.json"
	maxManifestSize    = 128 << 10
	maxPayloadSize     = 64 << 10
	maxAssetSize       = 500 << 20
)

var trustedPublicKeys = map[string][]byte{
	"733e4c77f6f216c2": mustDecodeKey("dVkjU9S6eqLgyvmi7c9Rr030uSk35AGJv+SaWzVOW24="),
}

type signedManifest struct {
	SchemaVersion int    `json:"schemaVersion"`
	Payload       string `json:"payload"`
	Signature     string `json:"signature"`
}

type manifestPayload struct {
	SchemaVersion  int            `json:"schemaVersion"`
	Channel        string         `json:"channel"`
	Version        string         `json:"version"`
	ReleasedAt     string         `json:"releasedAt"`
	Mandatory      bool           `json:"mandatory"`
	AllowDowngrade bool           `json:"allowDowngrade"`
	Notes          string         `json:"notes"`
	KeyID          string         `json:"keyId"`
	Assets         []ReleaseAsset `json:"assets"`
}

type ReleaseAsset struct {
	OS     string `json:"os"`
	Arch   string `json:"arch"`
	Name   string `json:"name"`
	URL    string `json:"url"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type Update struct {
	Version        string       `json:"version"`
	Notes          string       `json:"notes"`
	Mandatory      bool         `json:"mandatory"`
	AllowDowngrade bool         `json:"allowDowngrade"`
	Asset          ReleaseAsset `json:"asset"`
	manifestJSON   []byte
}

func (u *Update) ManifestJSON() []byte {
	return append([]byte(nil), u.manifestJSON...)
}

func Check(ctx context.Context, currentVersion, manifestURL string, client *http.Client) (*Update, error) {
	if manifestURL == "" {
		manifestURL = DefaultManifestURL
	}
	base, err := url.Parse(manifestURL)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return nil, fmt.Errorf("invalid manifest URL")
	}
	client = securedClient(client, base)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, manifestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "SamuelClient-Updater")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update check returned HTTP %d", resp.StatusCode)
	}
	raw, err := readLimited(resp.Body, maxManifestSize)
	if err != nil {
		return nil, err
	}
	return updateFromManifest(raw, currentVersion, base, trustedPublicKeys)
}

func LoadCachedUpdate(cachePath, currentVersion string) (*Update, error) {
	raw, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, err
	}
	if len(raw) > maxManifestSize {
		return nil, fmt.Errorf("cached manifest is too large")
	}
	base, _ := url.Parse(DefaultManifestURL)
	return updateFromManifest(raw, currentVersion, base, trustedPublicKeys)
}

func SaveManifestCache(cachePath string, raw []byte) error {
	if len(raw) > maxManifestSize {
		return fmt.Errorf("manifest is too large")
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o700); err != nil {
		return err
	}
	return os.WriteFile(cachePath, raw, 0o600)
}

func updateFromManifest(raw []byte, currentVersion string, base *url.URL, keys map[string][]byte) (*Update, error) {
	payload, err := verifyManifest(raw, keys)
	if err != nil {
		return nil, err
	}
	current, err := parseVersion(currentVersion)
	if err != nil {
		return nil, nil
	}
	available, err := parseVersion(payload.Version)
	if err != nil {
		return nil, err
	}
	comparison := compareVersions(available, current)
	if comparison == 0 || (!payload.AllowDowngrade && comparison < 0) {
		return nil, nil
	}
	asset, err := findAsset(payload.Assets, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return nil, err
	}
	absolute, err := resolveAssetURL(base, payload.Version, asset)
	if err != nil {
		return nil, err
	}
	asset.URL = absolute.String()
	return &Update{
		Version:        payload.Version,
		Notes:          payload.Notes,
		Mandatory:      payload.Mandatory,
		AllowDowngrade: payload.AllowDowngrade,
		Asset:          asset,
		manifestJSON:   append([]byte(nil), raw...),
	}, nil
}

func verifyManifest(raw []byte, keys map[string][]byte) (*manifestPayload, error) {
	var envelope signedManifest
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}
	if envelope.SchemaVersion != 1 {
		return nil, fmt.Errorf("unsupported manifest schema")
	}
	payloadBytes, err := decodeBase64URL(envelope.Payload)
	if err != nil || len(payloadBytes) > maxPayloadSize {
		return nil, fmt.Errorf("invalid manifest payload")
	}
	signature, err := decodeBase64URL(envelope.Signature)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return nil, fmt.Errorf("invalid manifest signature")
	}
	var payload manifestPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return nil, fmt.Errorf("decode manifest payload: %w", err)
	}
	key, ok := keys[payload.KeyID]
	if !ok {
		return nil, fmt.Errorf("manifest uses an untrusted key")
	}
	if !ed25519.Verify(key, payloadBytes, signature) {
		return nil, fmt.Errorf("manifest signature is invalid")
	}
	if err := validateManifest(&payload); err != nil {
		return nil, err
	}
	return &payload, nil
}

func validateManifest(payload *manifestPayload) error {
	if payload.SchemaVersion != 1 || payload.Channel != "stable" {
		return fmt.Errorf("unsupported manifest schema or channel")
	}
	if _, err := parseVersion(payload.Version); err != nil {
		return fmt.Errorf("invalid manifest version: %w", err)
	}
	if _, err := time.Parse(time.RFC3339, payload.ReleasedAt); err != nil {
		return fmt.Errorf("invalid manifest release time")
	}
	if payload.AllowDowngrade && !payload.Mandatory {
		return fmt.Errorf("downgrade manifests must be mandatory")
	}
	if len(payload.Notes) > 4000 || len(payload.KeyID) != 16 {
		return fmt.Errorf("invalid manifest metadata")
	}
	if len(payload.Assets) == 0 || len(payload.Assets) > 16 {
		return fmt.Errorf("invalid manifest assets")
	}
	seen := map[string]bool{}
	for _, asset := range payload.Assets {
		target := asset.OS + "/" + asset.Arch
		if seen[target] {
			return fmt.Errorf("duplicate asset for %s", target)
		}
		seen[target] = true
		if err := validateAsset(payload.Version, asset); err != nil {
			return err
		}
	}
	return nil
}

func validateAsset(version string, asset ReleaseAsset) error {
	if asset.OS != "linux" && asset.OS != "windows" {
		return fmt.Errorf("unsupported asset OS %q", asset.OS)
	}
	if asset.Arch != "amd64" && asset.Arch != "arm64" {
		return fmt.Errorf("unsupported asset architecture %q", asset.Arch)
	}
	expectedName := fmt.Sprintf("SamuelClient-%s-%s-%s", version, asset.OS, asset.Arch)
	if asset.OS == "windows" {
		expectedName += ".exe"
	}
	if asset.Name != expectedName {
		return fmt.Errorf("invalid asset name for %s/%s", asset.OS, asset.Arch)
	}
	expectedPath := fmt.Sprintf("/v1/releases/%s/%s-%s/%s", version, asset.OS, asset.Arch, expectedName)
	parsed, err := url.Parse(asset.URL)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path != expectedPath {
		return fmt.Errorf("invalid asset URL for %s/%s", asset.OS, asset.Arch)
	}
	if asset.Size <= 0 || asset.Size > maxAssetSize {
		return fmt.Errorf("invalid asset size for %s/%s", asset.OS, asset.Arch)
	}
	digest, err := hex.DecodeString(asset.SHA256)
	if err != nil || len(digest) != sha256.Size {
		return fmt.Errorf("invalid asset digest for %s/%s", asset.OS, asset.Arch)
	}
	return nil
}

func resolveAssetURL(base *url.URL, version string, asset ReleaseAsset) (*url.URL, error) {
	if err := validateAsset(version, asset); err != nil {
		return nil, err
	}
	reference, _ := url.Parse(asset.URL)
	resolved := base.ResolveReference(reference)
	if resolved.Scheme != base.Scheme || resolved.Host != base.Host {
		return nil, fmt.Errorf("asset URL does not use the trusted update origin")
	}
	return resolved, nil
}

func findAsset(assets []ReleaseAsset, goos, goarch string) (ReleaseAsset, error) {
	for _, asset := range assets {
		if asset.OS == goos && asset.Arch == goarch {
			return asset, nil
		}
	}
	return ReleaseAsset{}, fmt.Errorf("no update is available for %s/%s", goos, goarch)
}

func securedClient(client *http.Client, base *url.URL) *http.Client {
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Minute}
	}
	clone := *client
	clone.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("too many redirects")
		}
		if req.URL.Scheme != base.Scheme || req.URL.Host != base.Host {
			return fmt.Errorf("update request redirected outside the trusted origin")
		}
		return nil
	}
	return &clone
}

func decodeBase64URL(value string) ([]byte, error) {
	if value == "" || strings.ContainsAny(value, "=+/\\") {
		return nil, fmt.Errorf("invalid base64url")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || base64.RawURLEncoding.EncodeToString(decoded) != value {
		return nil, fmt.Errorf("invalid base64url")
	}
	return decoded, nil
}

func mustDecodeKey(value string) []byte {
	key, err := base64.StdEncoding.DecodeString(value)
	if err != nil || len(key) != ed25519.PublicKeySize {
		panic("invalid embedded OTA public key")
	}
	return key
}

func readLimited(reader io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("response exceeds the size limit")
	}
	return data, nil
}
