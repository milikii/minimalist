package app

import (
	"strings"
	"testing"
	"time"
)

func TestSelectLatestAlphaAssetChoosesLatestMatchingAlphaRelease(t *testing.T) {
	releases := []githubRelease{
		{
			TagName:     "v1.19.23",
			Name:        "v1.19.23",
			Prerelease:  false,
			PublishedAt: mustParseRFC3339(t, "2026-04-25T10:00:00Z"),
			Assets: []githubReleaseAsset{
				{Name: "mihomo-linux-arm64-v1.19.23.gz", BrowserDownloadURL: "https://example.com/stable.gz"},
			},
		},
		{
			TagName:     "v1.19.22-alpha-1",
			Name:        "v1.19.22 alpha 1",
			Prerelease:  true,
			PublishedAt: mustParseRFC3339(t, "2026-04-22T10:00:00Z"),
			Assets: []githubReleaseAsset{
				{Name: "mihomo-linux-arm64-v1.19.22.gz", BrowserDownloadURL: "https://example.com/older.gz"},
			},
		},
		{
			TagName:     "v1.19.24-alpha-2",
			Name:        "v1.19.24 alpha 2",
			Prerelease:  true,
			PublishedAt: mustParseRFC3339(t, "2026-04-24T10:00:00Z"),
			Assets: []githubReleaseAsset{
				{Name: "mihomo-linux-arm64-v1.19.24.gz", BrowserDownloadURL: "https://example.com/newest.gz"},
			},
		},
	}

	release, asset, err := selectLatestAlphaAsset(releases, "linux", "arm64")
	if err != nil {
		t.Fatalf("select latest alpha asset: %v", err)
	}
	if release.TagName != "v1.19.24-alpha-2" {
		t.Fatalf("expected latest alpha release, got %+v", release)
	}
	if asset.Name != "mihomo-linux-arm64-v1.19.24.gz" {
		t.Fatalf("expected latest linux arm64 asset, got %+v", asset)
	}
}

func TestSelectLatestAlphaAssetChoosesArm64Asset(t *testing.T) {
	releases := []githubRelease{
		{
			TagName:    "v1.19.23-alpha-1",
			Name:       "v1.19.23 alpha 1",
			Prerelease: true,
			Assets: []githubReleaseAsset{
				{Name: "mihomo-linux-arm64-v1.19.23.gz", BrowserDownloadURL: "https://example.com/linux-arm64.gz"},
			},
		},
	}

	_, asset, err := selectLatestAlphaAsset(releases, "linux", "arm64")
	if err != nil {
		t.Fatalf("select latest alpha arm64 asset: %v", err)
	}
	if asset.Name != "mihomo-linux-arm64-v1.19.23.gz" {
		t.Fatalf("expected linux arm64 asset, got %+v", asset)
	}
}

func TestSelectLatestAlphaAssetRejectsAMD64CPUVariants(t *testing.T) {
	releases := []githubRelease{
		{
			TagName:    "v1.19.23-alpha-1",
			Name:       "v1.19.23 alpha 1",
			Prerelease: true,
			Assets: []githubReleaseAsset{
				{Name: "mihomo-linux-amd64-v3-v1.19.23.gz", BrowserDownloadURL: "https://example.com/v3.gz"},
				{Name: "mihomo-darwin-amd64-v1-v1.19.23.gz", BrowserDownloadURL: "https://example.com/darwin.gz"},
				{Name: "mihomo-linux-amd64-v1-v1.19.23.gz", BrowserDownloadURL: "https://example.com/v1.gz"},
				{Name: "mihomo-linux-amd64-v2-v1.19.23.gz", BrowserDownloadURL: "https://example.com/v2.gz"},
			},
		},
	}

	_, _, err := selectLatestAlphaAsset(releases, "linux", "amd64")
	if err == nil || !strings.Contains(err.Error(), "explicit amd64 cpu level") {
		t.Fatalf("expected explicit cpu level error, got %v", err)
	}
	for _, needle := range []string{
		"mihomo-linux-amd64-v1-v1.19.23.gz",
		"mihomo-linux-amd64-v2-v1.19.23.gz",
		"mihomo-linux-amd64-v3-v1.19.23.gz",
	} {
		if !strings.Contains(err.Error(), needle) {
			t.Fatalf("expected %q in error, got %v", needle, err)
		}
	}
}

func TestSelectLatestAlphaAssetRejectsSingleAMD64CPUVariant(t *testing.T) {
	releases := []githubRelease{
		{
			TagName:    "v1.19.23-alpha-1",
			Name:       "v1.19.23 alpha 1",
			Prerelease: true,
			Assets: []githubReleaseAsset{
				{Name: "mihomo-linux-amd64-v1-v1.19.23.gz", BrowserDownloadURL: "https://example.com/v1.gz"},
			},
		},
	}

	_, _, err := selectLatestAlphaAsset(releases, "linux", "amd64")
	if err == nil || !strings.Contains(err.Error(), "explicit amd64 cpu level") {
		t.Fatalf("expected explicit cpu level error, got %v", err)
	}
}

func TestSelectLatestAlphaAssetRejectsAMD64HigherCPUVariant(t *testing.T) {
	releases := []githubRelease{
		{
			TagName:    "v1.19.23-alpha-1",
			Name:       "v1.19.23 alpha 1",
			Prerelease: true,
			Assets: []githubReleaseAsset{
				{Name: "mihomo-linux-amd64-v4-v1.19.23.gz", BrowserDownloadURL: "https://example.com/v4.gz"},
			},
		},
	}

	_, _, err := selectLatestAlphaAsset(releases, "linux", "amd64")
	if err == nil || !strings.Contains(err.Error(), "explicit amd64 cpu level") {
		t.Fatalf("expected explicit cpu level error for v4 asset, got %v", err)
	}
	if !strings.Contains(err.Error(), "mihomo-linux-amd64-v4-v1.19.23.gz") {
		t.Fatalf("expected v4 asset name in error, got %v", err)
	}
}

func TestSelectLatestAlphaAssetRejectsUnsupportedArch(t *testing.T) {
	releases := []githubRelease{
		{
			TagName:    "Prerelease-Alpha",
			Name:       "Prerelease-Alpha",
			Prerelease: true,
			Assets: []githubReleaseAsset{
				{Name: "mihomo-linux-amd64-v1.19.23.gz", BrowserDownloadURL: "https://example.com/linux.gz"},
			},
		},
	}

	_, _, err := selectLatestAlphaAsset(releases, "linux", "mips64")
	if err == nil || !strings.Contains(err.Error(), "unsupported linux arch") {
		t.Fatalf("expected unsupported arch error, got %v", err)
	}
}

func mustParseRFC3339(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("parse time %q: %v", value, err)
	}
	return parsed
}
