package services

import (
	"context"
	"crypto/sha512"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type modrinthTransport func(*http.Request) (*http.Response, error)

func (f modrinthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func testModrinthService(t *testing.T) (*ModrinthService, *bool) {
	t.Helper()
	main := testModrinthVersion("projectA", "versionA", "main.jar", "main")
	dep := testModrinthVersion("projectB", "versionB", "dep.jar", "dependency")
	dep.Dependencies = []modrinthDependency{{ProjectID: main.ProjectID, Type: "required"}}
	main.Dependencies = []modrinthDependency{
		{ProjectID: dep.ProjectID, Type: "required"},
		{VersionID: "unused", Type: "optional"},
	}
	corrupt := false
	s := NewModrinthService()
	s.client.Transport = modrinthTransport(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("User-Agent") == "" {
			t.Error("missing Modrinth User-Agent")
		}
		var body any
		switch req.URL.Host + req.URL.Path {
		case "api.modrinth.com/v2/search":
			if got := req.URL.Query().Get("facets"); got != `[["project_type:mod"],["categories:fabric"],["versions:1.20.1"]]` {
				t.Errorf("facets = %q", got)
			}
			if req.URL.Query().Get("offset") != "20" || req.URL.Query().Get("query") != "hello world" {
				t.Errorf("search query = %v", req.URL.Query())
			}
			body = ModrinthSearchResult{Limit: 20, Total: 21, Hits: []ModrinthProject{{ID: main.ProjectID}}}
		case "api.modrinth.com/v2/version/versionA":
			body = main
		case "api.modrinth.com/v2/project/projectB/version":
			unmatched := dep
			unmatched.Loaders = []string{"forge"}
			unmatched.ID = "wrongLoader"
			body = []ModrinthVersion{unmatched, dep}
		case "cdn.modrinth.com/main.jar":
			return modrinthResponse("main"), nil
		case "cdn.modrinth.com/dep.jar":
			if corrupt {
				return modrinthResponse("corruption"), nil
			}
			return modrinthResponse("dependency"), nil
		default:
			return nil, fmt.Errorf("unexpected request: %s", req.URL)
		}
		raw, err := json.Marshal(body)
		return modrinthResponse(string(raw)), err
	})
	return s, &corrupt
}

func testModrinthVersion(projectID, versionID, filename, content string) ModrinthVersion {
	return ModrinthVersion{
		ID: versionID, ProjectID: projectID, Number: "1.0", Channel: "release",
		Loaders: []string{"fabric"}, Versions: []string{"1.20.1"},
		Files: []modrinthFile{{
			Name: filename, URL: "https://cdn.modrinth.com/" + filename, Primary: true,
			Size: int64(len(content)), Hashes: map[string]string{"sha512": fmt.Sprintf("%x", sha512.Sum512([]byte(content)))},
		}},
	}
}

func modrinthResponse(body string) *http.Response {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}
}

func TestModrinthSearchAndPlan(t *testing.T) {
	s, _ := testModrinthService(t)
	ctx := context.Background()
	target := ModrinthTarget{ProfileID: "fabric-loader-test", GameVersion: "1.20.1", Loader: "fabric"}
	result, err := s.Search(ctx, target, " hello world ", 20)
	if err != nil || len(result.Hits) != 1 || result.Total != 21 {
		t.Fatalf("search: %+v, %v", result, err)
	}
	plan, err := s.Plan(ctx, target, "projectA", "versionA")
	if err != nil || len(plan.Files) != 2 || plan.Files[1].VersionID != "versionB" || plan.Fingerprint == "" {
		t.Fatalf("plan: %+v, %v", plan, err)
	}
	target.Loader = "forge"
	if _, err := s.Plan(ctx, target, "projectA", "versionA"); err == nil {
		t.Fatal("expected incompatible loader to be rejected")
	}
}

func TestModrinthVerifiedInstall(t *testing.T) {
	s, corrupt := testModrinthService(t)
	ctx := context.Background()
	target := ModrinthTarget{GameVersion: "1.20.1", Loader: "fabric"}
	plan, err := s.Plan(ctx, target, "projectA", "versionA")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	install := func(fingerprint string) error {
		return s.Install(ctx, dir, target, "projectA", "versionA", fingerprint)
	}
	if err := install("old-plan"); err == nil {
		t.Fatal("expected an obsolete plan to be rejected")
	}
	*corrupt = true
	if err := install(plan.Fingerprint); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected a checksum error, got %v", err)
	}
	mods := filepath.Join(dir, "mods")
	if entries, err := os.ReadDir(mods); err != nil || len(entries) != 0 {
		t.Fatalf("failed install left files behind: %v, %v", entries, err)
	}
	*corrupt = false
	backup := filepath.Join(mods, "dep.jar.bak")
	writeModFixture(t, backup, "disabled")
	if err := install(plan.Fingerprint); err == nil {
		t.Fatal("expected a disabled-file conflict")
	}
	if err := os.Remove(backup); err != nil {
		t.Fatal(err)
	}
	if err := install(plan.Fingerprint); err != nil {
		t.Fatal(err)
	}
	if err := install(plan.Fingerprint); err != nil {
		t.Fatalf("identical files should be reused: %v", err)
	}
	got, err := ListMods(dir)
	want := []ModFile{{Name: "dep.jar", Enabled: true}, {Name: "main.jar", Enabled: true}}
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("installed mods = %v, %v; want %v", got, err, want)
	}
}
