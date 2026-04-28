package app

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
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

var currentGOOS = func() string { return runtime.GOOS }
var currentGOARCH = func() string { return runtime.GOARCH }

func (a *App) CoreUpgradeAlpha() error {
	if err := a.requireRoot(); err != nil {
		return err
	}
	cfg, _, err := a.ensureAll()
	if err != nil {
		return err
	}

	releases, err := a.fetchMihomoReleases()
	if err != nil {
		return err
	}
	release, asset, err := selectLatestAlphaAsset(releases, currentGOOS(), currentGOARCH())
	if err != nil {
		return err
	}
	candidate, err := a.downloadReleaseAsset(asset, cfg.Install.CoreBin)
	if err != nil {
		return err
	}
	defer os.RemoveAll(filepath.Dir(candidate))

	oldVersion, err := a.readBinaryVersion(cfg.Install.CoreBin)
	if err != nil {
		return fmt.Errorf("read current core version: %w", err)
	}
	newVersion, err := a.readBinaryVersion(candidate)
	if err != nil {
		return fmt.Errorf("read candidate core version: %w", err)
	}
	if err := replaceCoreBinaryAtomically(cfg.Install.CoreBin, candidate); err != nil {
		return err
	}
	if err := a.restartMinimalistServiceAfterCoreUpgrade(); err != nil {
		return err
	}

	fmt.Fprintf(a.Stdout, "core path: %s\n", cfg.Install.CoreBin)
	fmt.Fprintf(a.Stdout, "release: %s\n", release.TagName)
	fmt.Fprintf(a.Stdout, "asset: %s\n", asset.Name)
	fmt.Fprintf(a.Stdout, "old version: %s\n", oldVersion)
	fmt.Fprintf(a.Stdout, "new version: %s\n", newVersion)
	fmt.Fprintln(a.Stdout, "service restarted: minimalist.service")
	return nil
}

func (a *App) fetchMihomoReleases() ([]githubRelease, error) {
	req, err := http.NewRequest(http.MethodGet, mihomoReleasesAPI, nil)
	if err != nil {
		return nil, err
	}
	resp, err := a.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	var releases []githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, err
	}
	return releases, nil
}

func replaceCoreBinaryAtomically(coreBin, candidate string) error {
	if err := os.Rename(candidate, coreBin); err != nil {
		return fmt.Errorf("replace core binary: %w", err)
	}
	return nil
}

func (a *App) restartMinimalistServiceAfterCoreUpgrade() error {
	if err := a.Runner.Run("systemctl", "restart", "minimalist.service"); err != nil {
		a.writeMinimalistJournalSummary()
		return fmt.Errorf("restart minimalist.service: %w", err)
	}
	if _, _, err := a.Runner.Output("systemctl", "is-active", "minimalist.service"); err != nil {
		a.writeMinimalistJournalSummary()
		return fmt.Errorf("minimalist.service is not active after restart: %w", err)
	}
	return nil
}

func (a *App) writeMinimalistJournalSummary() {
	stdout, stderr, err := a.Runner.Output("journalctl", "-u", "minimalist.service", "-n", "20", "--no-pager")
	logs := strings.TrimSpace(stdout)
	if logs == "" {
		logs = strings.TrimSpace(stderr)
	}
	if logs != "" {
		fmt.Fprintln(a.Stderr, logs)
		return
	}
	if err != nil {
		fmt.Fprintf(a.Stderr, "journalctl failed: %v\n", err)
	}
}

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
	alphaReleases := alphaReleasesByNewest(releases)
	if len(alphaReleases) == 0 {
		return githubRelease{}, githubReleaseAsset{}, fmt.Errorf("no alpha prerelease found")
	}
	for _, release := range alphaReleases {
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

func alphaReleasesByNewest(releases []githubRelease) []githubRelease {
	alphaReleases := make([]githubRelease, 0, len(releases))
	for _, release := range releases {
		if !isAlphaPrerelease(release) {
			continue
		}
		alphaReleases = append(alphaReleases, release)
	}
	slices.SortFunc(alphaReleases, func(left, right githubRelease) int {
		if releaseIsNewer(left, right) {
			return -1
		}
		if releaseIsNewer(right, left) {
			return 1
		}
		return 0
	})
	return alphaReleases
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

func (a *App) downloadReleaseAsset(asset githubReleaseAsset, coreBin string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, asset.BrowserDownloadURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := a.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("http %d", resp.StatusCode)
	}
	if !strings.HasSuffix(strings.ToLower(asset.Name), ".gz") {
		return "", fmt.Errorf("unsupported asset format: %s", asset.Name)
	}

	tmpParent := filepath.Dir(coreBin)
	if err := os.MkdirAll(tmpParent, 0o755); err != nil {
		return "", err
	}
	tmpDir, err := os.MkdirTemp(tmpParent, ".mihomo-core-*")
	if err != nil {
		return "", err
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.RemoveAll(tmpDir)
		}
	}()

	zr, err := gzip.NewReader(resp.Body)
	if err != nil {
		return "", err
	}
	defer zr.Close()
	body, err := io.ReadAll(zr)
	if err != nil {
		return "", err
	}
	candidate := filepath.Join(tmpDir, "mihomo-core")
	if err := os.WriteFile(candidate, body, 0o755); err != nil {
		return "", err
	}
	complete = true
	return candidate, nil
}

func (a *App) readBinaryVersion(path string) (string, error) {
	stdout, _, err := a.Runner.Output(path, "-v")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(stdout), nil
}
