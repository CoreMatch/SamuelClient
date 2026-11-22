package services

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"HRPMLW/internal/models"
)

func testPackFile(name, content, client string) modpackFile {
	return modpackFile{
		Path: name, Size: int64(len(content)), Env: map[string]string{"client": client},
		Downloads: []string{"https://cdn.modrinth.com/" + filepath.Base(name)},
		Hashes: map[string]string{
			"sha1": fmt.Sprintf("%x", sha1.Sum([]byte(content))), "sha512": fmt.Sprintf("%x", sha512.Sum512([]byte(content))),
		},
	}
}

func testPackZIP(t *testing.T, index modpackIndex, layers map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	w, err := z.Create("modrinth.index.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(w).Encode(index); err != nil {
		t.Fatal(err)
	}
	for name, content := range layers {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func testPackManifest() modpackIndex {
	return modpackIndex{Format: 1, Game: "minecraft", Name: "Example pack", Version: "v1",
		Dependencies: map[string]string{"minecraft": "1.20.1", "fabric-loader": "0.16.0"}}
}

type testPackInstaller struct{}

func testPackJava(_ context.Context, major int, _ PackProgress) (string, error) {
	if major != 17 {
		return "", fmt.Errorf("unexpected Java version %d", major)
	}
	return "test-java", nil
}

func (testPackInstaller) InstallVersion(_ context.Context, game, dir string, _ func(string, string, int64, int64)) *models.ApiResponse {
	return testWritePackProfile(dir, game, "")
}

func (testPackInstaller) InstallFabricLoader(_ context.Context, game, loader, dir string, _ func(string, string, int64, int64)) *models.ApiResponse {
	return testWritePackProfile(dir, "fabric-loader-"+loader+"-"+game, game)
}

func (testPackInstaller) InstallForgeLoader(context.Context, string, string, string, string, func(string, string), func(string, string, int64, int64)) *models.ApiResponse {
	return &models.ApiResponse{Error: "unexpected Forge install"}
}

func testWritePackProfile(dir, id, parent string) *models.ApiResponse {
	dest := filepath.Join(dir, "versions", id)
	if err := os.MkdirAll(dest, 0755); err != nil {
		return &models.ApiResponse{Error: err.Error()}
	}
	data, _ := json.Marshal(BMCLVersionJSON{ID: id, InheritsFrom: parent, JavaVersion: BMCLJavaVersion{MajorVersion: 17}})
	if err := os.WriteFile(filepath.Join(dest, id+".json"), data, 0644); err != nil {
		return &models.ApiResponse{Error: err.Error()}
	}
	if parent == "" {
		if err := os.WriteFile(filepath.Join(dest, id+".jar"), []byte("client"), 0644); err != nil {
			return &models.ApiResponse{Error: err.Error()}
		}
	}
	return &models.ApiResponse{Success: true, Data: map[string]any{"version_id": id}}
}

func TestModpackInstallPublishesOnlyCompleteInstance(t *testing.T) {
	index := testPackManifest()
	index.Files = []modpackFile{
		testPackFile("mods/client.jar", "good", "required"),
		testPackFile("mods/option.jar", "optional", "optional"),
		testPackFile("mods/unused.jar", "unused", "optional"),
		testPackFile("mods/server.jar", "server", "unsupported"),
	}
	path := filepath.Join(t.TempDir(), "test.mrpack")
	if err := os.WriteFile(path, testPackZIP(t, index, map[string]string{
		"overrides/config/example.txt":        "default",
		"client-overrides/config/example.txt": "client",
		"server-overrides/server.properties":  "server",
	}), 0644); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"success", "checksum", "cancel", "register", "changed"} {
		t.Run(scenario, func(t *testing.T) {
			service := NewModrinthService()
			service.client.Transport = modrinthTransport(func(req *http.Request) (*http.Response, error) {
				switch req.URL.Path {
				case "/client.jar":
					if scenario == "checksum" {
						return modrinthResponse("oops"), nil
					}
					return modrinthResponse("good"), nil
				case "/option.jar":
					return modrinthResponse("optional"), nil
				default:
					t.Fatalf("unexpected download: %s", req.URL)
					return nil, nil
				}
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			source := ModpackSource{LocalPath: path}
			preview, err := service.PreviewModpack(ctx, source)
			if err != nil || preview.RequiredBytes != 4 || len(preview.Options) != 2 {
				t.Fatalf("preview: %+v, %v", preview, err)
			}
			if scenario == "changed" {
				preview.Fingerprint = strings.Repeat("0", 64)
			}
			parent := t.TempDir()
			registered := ""
			result, err := service.InstallModpack(ctx, source, preview.Fingerprint, []string{"mods/option.jar"}, parent,
				testPackInstaller{}, testPackJava, func(dir string) error {
					registered = dir
					if scenario == "register" {
						return fmt.Errorf("registration failed")
					}
					return nil
				}, func(stage, _ string, _, _ int64) {
					if scenario == "cancel" && stage == "pack-files" {
						cancel()
					}
				}, nil)
			entries, readErr := os.ReadDir(parent)
			if scenario != "success" {
				if err == nil || readErr != nil || len(entries) != 0 || (scenario != "register" && registered != "") {
					t.Fatalf("expected rollback, got entries=%v registered=%q error=%v", entries, registered, err)
				}
				return
			}
			if err != nil || readErr != nil || len(entries) != 1 || registered == "" || result.GameDir != registered ||
				result.VersionID != "fabric-loader-0.16.0-1.20.1" || result.JavaMajor != 17 {
				t.Fatalf("install: %+v, %v", result, err)
			}
			if pack, ok := ReadModpackInstance(registered); !ok || pack.Name != index.Name {
				t.Fatalf("missing instance metadata: %+v", pack)
			}
			for name, expected := range map[string]string{"config/example.txt": "client", "mods/client.jar": "good", "mods/option.jar": "optional"} {
				got, err := os.ReadFile(filepath.Join(registered, name))
				if err != nil || string(got) != expected {
					t.Fatalf("%s = %q, %v", name, got, err)
				}
			}
			for _, name := range []string{"mods/server.jar", "mods/unused.jar", "server.properties"} {
				if _, err := os.Stat(filepath.Join(registered, name)); !os.IsNotExist(err) {
					t.Fatalf("unexpected file %s: %v", name, err)
				}
			}
			pack, _ := ReadModpackInstance(registered)
			if err := os.Remove(filepath.Join(registered, "versions/1.20.1/1.20.1.jar")); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(filepath.Join(registered, "mods/client.jar")); err != nil {
				t.Fatal(err)
			}
			optional := filepath.Join(registered, "mods/option.jar")
			if err := os.Rename(optional, optional+".bak"); err != nil {
				t.Fatal(err)
			}
			config := filepath.Join(registered, "config/example.txt")
			if err := os.WriteFile(config, []byte("changed by player"), 0644); err != nil {
				t.Fatal(err)
			}
			quiet := func(string, string, int64, int64) {}
			java, err := service.PrepareModpackLaunch(ctx, registered, pack, testPackInstaller{}, testPackJava, quiet, nil)
			if err != nil || java != "test-java" {
				t.Fatalf("prepare: %q, %v", java, err)
			}
			for name, expected := range map[string]string{
				"versions/1.20.1/1.20.1.jar": "client", "mods/client.jar": "good",
				"mods/option.jar.bak": "optional", "config/example.txt": "changed by player",
			} {
				got, err := os.ReadFile(filepath.Join(registered, name))
				if err != nil || string(got) != expected {
					t.Fatalf("%s after repair = %q, %v", name, got, err)
				}
			}
			if _, err := os.Stat(optional); !os.IsNotExist(err) {
				t.Fatalf("disabled mod restored: %v", err)
			}
			_, err = service.PrepareModpackLaunch(ctx, registered, pack, testPackInstaller{}, testPackJava,
				func(stage, _ string, _, _ int64) { t.Errorf("complete instance unexpectedly starts %s", stage) }, nil)
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestModpackRejectsInvalidManifest(t *testing.T) {
	for _, scenario := range []string{"reserved", "conflict", "unsupported loader", "unapproved host"} {
		t.Run(scenario, func(t *testing.T) {
			index := testPackManifest()
			index.Files = []modpackFile{testPackFile("mods/file.jar", "content", "required")}
			layers := map[string]string{}
			switch scenario {
			case "reserved":
				layers["overrides/versions/metadata.json"] = "{}"
			case "conflict":
				layers["client-overrides/Mods/File.jar"] = "conflict"
			case "unsupported loader":
				index.Dependencies = map[string]string{"minecraft": "1.20.1", "quilt-loader": "0.1"}
			case "unapproved host":
				index.Files[0].Downloads[0] = "https://example.invalid/file.jar"
			}
			raw := testPackZIP(t, index, layers)
			z, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := parseModpack(z); err == nil {
				t.Fatal("invalid pack was accepted")
			}
		})
	}
}

func TestModrinthModpackCatalogAndVerifiedArchive(t *testing.T) {
	raw := testPackZIP(t, testPackManifest(), nil)
	version := ModrinthVersion{ID: "versionA", ProjectID: "projectA", Number: "1", Files: []modrinthFile{
		{Name: "example.mrpack", URL: "https://cdn.modrinth.com/example.mrpack", Size: int64(len(raw)),
			Primary: true, Hashes: map[string]string{"sha512": fmt.Sprintf("%x", sha512.Sum512(raw))}},
	}}
	service := NewModrinthService()
	badArchive := false
	service.client.Transport = modrinthTransport(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/v2/search":
			if got := req.URL.Query().Get("facets"); got != `[["project_type:modpack"]]` {
				t.Fatalf("unexpected facets: %s", got)
			}
			return modrinthResponse(`{"hits":[{"project_id":"projectA"}],"total_hits":1}`), nil
		case "/v2/project/projectA/version":
			body, _ := json.Marshal([]ModrinthVersion{version})
			return modrinthResponse(string(body)), nil
		case "/v2/version/versionA":
			body, _ := json.Marshal(version)
			return modrinthResponse(string(body)), nil
		case "/example.mrpack":
			if badArchive {
				return modrinthResponse(strings.Repeat("x", len(raw))), nil
			}
			return modrinthResponse(string(raw)), nil
		default:
			return nil, fmt.Errorf("unexpected Modrinth request %s", req.URL)
		}
	})
	ctx := context.Background()
	if page, err := service.SearchModpacks(ctx, "Demo", 0); err != nil || len(page.Hits) != 1 {
		t.Fatalf("search: %+v, %v", page, err)
	}
	if versions, err := service.ModpackVersions(ctx, "projectA"); err != nil || len(versions) != 1 {
		t.Fatalf("versions: %+v, %v", versions, err)
	}
	source := ModpackSource{ProjectID: "projectA", VersionID: "versionA"}
	if preview, err := service.PreviewModpack(ctx, source); err != nil || preview.Name != "Example pack" {
		t.Fatalf("preview: %+v, %v", preview, err)
	}
	badArchive = true
	if _, err := service.PreviewModpack(ctx, source); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected checksum error, got %v", err)
	}
}
