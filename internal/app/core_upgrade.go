package app

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

const mihomoReleasesAPI = "https://api.github.com/repos/MetaCubeX/mihomo/releases"

type githubRelease struct {
	TagName    string               `json:"tag_name"`
	Name       string               `json:"name"`
	Prerelease bool                 `json:"prerelease"`
	Assets     []githubReleaseAsset `json:"assets"`
}

type githubReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

var errNoMatchingLinuxAsset = errors.New("no matching linux asset")

func linuxAssetArch(goarch string) (string, error) {
	switch goarch {
	case "amd64":
		return "amd64", nil
	case "arm64":
		return "arm64", nil
	default:
		return "", fmt.Errorf("unsupported linux arch: %s", goarch)
	}
}

func selectLatestAlphaAsset(releases []githubRelease, goos, goarch string) (githubRelease, githubReleaseAsset, error) {
	if goos != "linux" {
		return githubRelease{}, githubReleaseAsset{}, fmt.Errorf("unsupported os: %s", goos)
	}
	arch, err := linuxAssetArch(goarch)
	if err != nil {
		return githubRelease{}, githubReleaseAsset{}, err
	}

	assetPrefix := "mihomo-linux-" + arch + "-"
	for _, release := range releases {
		if !release.Prerelease {
			continue
		}
		label := strings.ToLower(release.TagName + " " + release.Name)
		if !strings.Contains(label, "alpha") {
			continue
		}
		asset, err := selectLinuxReleaseAsset(release.Assets, goarch, assetPrefix)
		if err == nil {
			return release, asset, nil
		}
		if errors.Is(err, errNoMatchingLinuxAsset) {
			continue
		}
		return githubRelease{}, githubReleaseAsset{}, fmt.Errorf("select asset for release %s: %w", release.TagName, err)
	}

	return githubRelease{}, githubReleaseAsset{}, fmt.Errorf("no matching alpha asset for %s/%s", goos, goarch)
}

func selectLinuxReleaseAsset(assets []githubReleaseAsset, goarch, assetPrefix string) (githubReleaseAsset, error) {
	candidates := make([]githubReleaseAsset, 0)
	for _, asset := range assets {
		name := strings.ToLower(asset.Name)
		if strings.HasPrefix(name, assetPrefix) && strings.HasSuffix(name, ".gz") {
			candidates = append(candidates, asset)
		}
	}

	if len(candidates) == 0 {
		return githubReleaseAsset{}, errNoMatchingLinuxAsset
	}
	if goarch != "amd64" {
		if len(candidates) == 1 {
			return candidates[0], nil
		}
		return githubReleaseAsset{}, fmt.Errorf("ambiguous linux/%s assets: %s", goarch, joinAssetNames(candidates))
	}

	legacy := make([]githubReleaseAsset, 0, len(candidates))
	cpuLevel := make([]githubReleaseAsset, 0, len(candidates))
	for _, asset := range candidates {
		if isAMD64CPULevelAsset(asset.Name, assetPrefix) {
			cpuLevel = append(cpuLevel, asset)
			continue
		}
		legacy = append(legacy, asset)
	}

	if len(cpuLevel) > 0 {
		return githubReleaseAsset{}, fmt.Errorf("explicit amd64 cpu level required; candidates: %s", joinAssetNames(cpuLevel))
	}
	if len(legacy) == 1 {
		return legacy[0], nil
	}
	if len(legacy) > 1 {
		return githubReleaseAsset{}, fmt.Errorf("ambiguous linux/amd64 assets: %s", joinAssetNames(legacy))
	}

	return githubReleaseAsset{}, errNoMatchingLinuxAsset
}

func isAMD64CPULevelAsset(name, assetPrefix string) bool {
	rest := strings.ToLower(strings.TrimPrefix(name, assetPrefix))
	return strings.HasPrefix(rest, "compatible-") ||
		strings.HasPrefix(rest, "v1-") ||
		strings.HasPrefix(rest, "v2-") ||
		strings.HasPrefix(rest, "v3-")
}

func joinAssetNames(assets []githubReleaseAsset) string {
	names := make([]string, 0, len(assets))
	for _, asset := range assets {
		names = append(names, asset.Name)
	}
	slices.Sort(names)
	return strings.Join(names, ", ")
}
