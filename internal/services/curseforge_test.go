package services

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCurseForgeWithoutKey(t *testing.T) {
	service := NewCurseForgeService("")
	service.client.Transport = modrinthTransport(func(*http.Request) (*http.Response, error) {
		t.Fatal("no request should be sent without a key")
		return nil, nil
	})
	_, err := service.Search(context.Background(), ModrinthTarget{GameVersion: "1.20.1", Loader: "fabric"}, "example", 0)
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("expected missing-key error, got %v", err)
	}
}

func TestCurseForgeSearchPlanAndInstall(t *testing.T) {
	service := NewCurseForgeService("test-key")
	main := `{"id":11,"modId":1,"displayName":"Example","fileName":"example.jar","fileLength":3,"releaseType":1,"downloadUrl":"https://edge.forgecdn.net/example.jar","gameVersions":["1.20.1","Fabric"],"hashes":[{"algo":1,"value":"a9993e364706816aba3e25717850c26c9cd0d89d"}],"dependencies":[{"modId":2,"relationType":3},{"modId":3,"relationType":2}]}`
	dep := `{"id":22,"modId":2,"displayName":"Dependency","fileName":"dep.jar","fileLength":3,"releaseType":1,"downloadUrl":"https://edge.forgecdn.net/dep.jar","gameVersions":["1.20.1","Fabric"],"hashes":[{"algo":1,"value":"a9993e364706816aba3e25717850c26c9cd0d89d"}]}`
	restricted := `{"id":12,"modId":1,"displayName":"Restricted","fileName":"restricted.jar","releaseType":1,"downloadUrl":null,"gameVersions":["1.20.1","Fabric"]}`
	service.client.Transport = modrinthTransport(func(req *http.Request) (*http.Response, error) {
		key := req.Header.Get("x-api-key")
		if req.URL.Host == curseForgeHost && key != "test-key" {
			t.Errorf("missing CurseForge API key")
		}
		switch req.URL.Host + req.URL.Path {
		case curseForgeHost + "/v1/mods/search":
			q := req.URL.Query()
			if q.Get("gameId") != "432" || q.Get("classId") != "6" || q.Get("modLoaderType") != "4" || q.Get("gameVersion") != "1.20.1" || q.Get("index") != "20" {
				t.Errorf("unexpected search filters: %v", q)
			}
			return modrinthResponse(`{"data":[{"id":1,"name":"Example","authors":[{"name":"Author"}]}],"pagination":{"totalCount":21}}`), nil
		case curseForgeHost + "/v1/mods/1/files":
			return modrinthResponse(`{"data":[` + main + `,` + restricted + `]}`), nil
		case curseForgeHost + "/v1/mods/1/files/11":
			return modrinthResponse(`{"data":` + main + `}`), nil
		case curseForgeHost + "/v1/mods/1/files/12":
			return modrinthResponse(`{"data":` + restricted + `}`), nil
		case curseForgeHost + "/v1/mods/2/files":
			return modrinthResponse(`{"data":[` + dep + `]}`), nil
		case "edge.forgecdn.net/example.jar", "edge.forgecdn.net/dep.jar":
			if key != "" {
				t.Error("API key leaked to CDN")
			}
			return modrinthResponse("abc"), nil
		default:
			return nil, fmt.Errorf("unexpected request %s", req.URL)
		}
	})
	ctx := context.Background()
	target := ModrinthTarget{GameVersion: "1.20.1", Loader: "fabric"}
	results, err := service.Search(ctx, target, "example", 20)
	if err != nil || len(results.Hits) != 1 || results.Total != 21 {
		t.Fatalf("search = %+v, %v", results, err)
	}
	versions, err := service.Versions(ctx, target, "1")
	if err != nil || len(versions) != 2 || !versions[0].Downloadable || versions[1].Downloadable {
		t.Fatalf("versions = %+v, %v", versions, err)
	}
	if _, err := service.Plan(ctx, target, "1", "12"); err == nil {
		t.Fatal("restricted download should not be resolved")
	}
	plan, err := service.Plan(ctx, target, "1", "11")
	if err != nil || len(plan.Files) != 2 || plan.Files[1].Name != "dep.jar" {
		t.Fatalf("plan = %+v, %v", plan, err)
	}
	dir := t.TempDir()
	if err := service.Install(ctx, dir, target, "1", "11", plan.Fingerprint); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"dep.jar", "example.jar"} {
		got, err := os.ReadFile(filepath.Join(dir, "mods", name))
		if err != nil || string(got) != "abc" {
			t.Fatalf("installed %s = %q, %v", name, got, err)
		}
	}
}
