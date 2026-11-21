package services

import "testing"

func TestDownloadSourceRouting(t *testing.T) {
	const library = "https://libraries.minecraft.net/org/example/a.jar"
	const object = "/ab/abcdef"
	const custom = "https://mirror.example/base"
	base := NewBMCLAPIService()
	for _, tc := range []struct {
		name, source, shape, customURL string
		wantManifest, wantLib, wantObj string
	}{
		{"default", sourceBMCL, sourceBMCL, "", defaultBMCLURL + "/mc/game/version_manifest.json", defaultBMCLURL + "/maven/org/example/a.jar", defaultBMCLURL + "/assets" + object},
		{"official", sourceOfficial, sourceBMCL, "", officialMetaURL + "/mc/game/version_manifest_v2.json", library, officialAssetsURL + object},
		{"custom official", sourceCustom, sourceOfficial, custom, custom + "/mc/game/version_manifest_v2.json", custom + "/org/example/a.jar", custom + object},
		{"custom BMCLAPI", sourceCustom, sourceBMCL, custom, custom + "/mc/game/version_manifest.json", custom + "/maven/org/example/a.jar", custom + "/assets" + object},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := base.WithDownloadSettings(DownloadSettings{
				Source: tc.source, CustomShape: tc.shape, CustomURL: tc.customURL, Concurrency: 2,
			})
			if got := svc.versionListURL(); got != tc.wantManifest {
				t.Errorf("manifest URL = %q, want %q", got, tc.wantManifest)
			}
			if got := svc.rewriteGameURL(library); got != tc.wantLib {
				t.Errorf("library URL = %q, want %q", got, tc.wantLib)
			}
			if got := svc.assetURL(object); got != tc.wantObj {
				t.Errorf("asset URL = %q, want %q", got, tc.wantObj)
			}
			const thirdParty = "https://example.org/https://libraries.minecraft.net/test"
			if got := svc.rewriteGameURL(thirdParty); got != thirdParty {
				t.Errorf("unexpected rewrite of third-party URL: %q", got)
			}
			if svc.assetWorkers != 2 || base.assetWorkers != assetDownloadWorkers {
				t.Errorf("worker counts: configured=%d, base=%d", svc.assetWorkers, base.assetWorkers)
			}
		})
	}
}
