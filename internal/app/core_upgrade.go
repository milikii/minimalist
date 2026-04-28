package app

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

const mihomoReleasesAPI = "https://api.github.com/repos/MetaCubeX/mihomo/releases"

type githubRelease struct {
	TagName     string               `json:"tag_name"`
	Name        string               `json:"name"`
	Prerelease  bool                 `json:"prerelease"`
	PublishedAt time.Time            `json:"published_at"`
	Assets      []githubReleaseAsset `json:"assets"`
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
	release, err := latestAlphaRelease(releases)
	if err != nil {
		return githubRelease{}, githubReleaseAsset{}, err
	}

	asset, err := selectLinuxReleaseAsset(release.Assets, goarch, assetPrefix)
	if err != nil {
		if errors.Is(err, errNoMatchingLinuxAsset) {
			return githubRelease{}, githubReleaseAsset{}, fmt.Errorf("latest alpha release %s has no matching asset for %s/%s", release.TagName, goos, goarch)
		}
		return githubRelease{}, githubReleaseAsset{}, fmt.Errorf("select asset for release %s: %w", release.TagName, err)
	}
	return release, asset, nil
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
	if strings.HasPrefix(rest, "compatible-") {
		return true
	}
	if !strings.HasPrefix(rest, "v") {
		return false
	}
	level, _, found := strings.Cut(rest[1:], "-")
	if !found || level == "" {
		return false
	}
	_, err := strconv.Atoi(level)
	return err == nil
}

func joinAssetNames(assets []githubReleaseAsset) string {
	names := make([]string, 0, len(assets))
	for _, asset := range assets {
		names = append(names, asset.Name)
	}
	slices.Sort(names)
	return strings.Join(names, ", ")
}

func latestAlphaRelease(releases []githubRelease) (githubRelease, error) {
	var latest githubRelease
	found := false
	for _, release := range releases {
		if !isAlphaPrerelease(release) {
			continue
		}
		if !found || releaseIsNewer(release, latest) {
			latest = release
			found = true
		}
	}
	if !found {
		return githubRelease{}, fmt.Errorf("no alpha prerelease found")
	}
	return latest, nil
}

func isAlphaPrerelease(release githubRelease) bool {
	if !release.Prerelease {
		return false
	}
	label := strings.ToLower(release.TagName + " " + release.Name)
	return strings.Contains(label, "alpha")
}

func releaseIsNewer(left, right githubRelease) bool {
	if left.PublishedAt.After(right.PublishedAt) {
		return true
	}
	if left.PublishedAt.Before(right.PublishedAt) {
		return false
	}
	if left.TagName != right.TagName {
		return left.TagName > right.TagName
	}
	return left.Name > right.Name
}
