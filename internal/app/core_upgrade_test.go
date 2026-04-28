package app

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"minimalist/internal/config"
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

func TestSelectLatestAlphaAssetChoosesNewestAlphaWithMatchingAsset(t *testing.T) {
	releases := []githubRelease{
		{
			TagName:     "v1.19.25-alpha-1",
			Name:        "v1.19.25 alpha 1",
			Prerelease:  true,
			PublishedAt: mustParseRFC3339(t, "2026-04-25T10:00:00Z"),
			Assets: []githubReleaseAsset{
				{Name: "mihomo-darwin-arm64-v1.19.25.gz", BrowserDownloadURL: "https://example.com/darwin.gz"},
			},
		},
		{
			TagName:     "v1.19.24-alpha-1",
			Name:        "v1.19.24 alpha 1",
			Prerelease:  true,
			PublishedAt: mustParseRFC3339(t, "2026-04-24T10:00:00Z"),
			Assets: []githubReleaseAsset{
				{Name: "mihomo-linux-arm64-v1.19.24.gz", BrowserDownloadURL: "https://example.com/linux-arm64.gz"},
			},
		},
	}

	release, asset, err := selectLatestAlphaAsset(releases, "linux", "arm64")
	if err != nil {
		t.Fatalf("select latest matching alpha asset: %v", err)
	}
	if release.TagName != "v1.19.24-alpha-1" {
		t.Fatalf("expected newest alpha with matching asset, got %+v", release)
	}
	if asset.Name != "mihomo-linux-arm64-v1.19.24.gz" {
		t.Fatalf("expected matching arm64 asset, got %+v", asset)
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
				{Name: "mihomo-linux-amd64-v10-v1.19.23.gz", BrowserDownloadURL: "https://example.com/v10.gz"},
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
	if !strings.Contains(err.Error(), "mihomo-linux-amd64-v10-v1.19.23.gz") {
		t.Fatalf("expected v10 asset name in error, got %v", err)
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

func TestDownloadReleaseAssetWritesExecutableCandidate(t *testing.T) {
	app, _ := newTestApp(t)
	var requested string
	payload := []byte("#!/bin/sh\nexit 0\n")
	coreBin := filepath.Join(t.TempDir(), "bin", "mihomo-core")
	app.Client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requested = req.URL.String()
			var body bytes.Buffer
			zw := gzip.NewWriter(&body)
			if _, err := zw.Write(payload); err != nil {
				t.Fatalf("gzip write: %v", err)
			}
			if err := zw.Close(); err != nil {
				t.Fatalf("gzip close: %v", err)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(body.Bytes())),
				Header:     make(http.Header),
			}, nil
		}),
	}

	candidate, err := app.downloadReleaseAsset(githubReleaseAsset{
		Name:               "mihomo-linux-amd64-v1.19.23.gz",
		BrowserDownloadURL: "https://example.com/mihomo.gz",
	}, coreBin)
	if err != nil {
		t.Fatalf("download release asset: %v", err)
	}
	if filepath.Dir(filepath.Dir(candidate)) != filepath.Dir(coreBin) {
		t.Fatalf("expected candidate under core bin dir, got %s for core %s", candidate, coreBin)
	}
	if requested != "https://example.com/mihomo.gz" {
		t.Fatalf("unexpected asset url: %s", requested)
	}
	body, err := os.ReadFile(candidate)
	if err != nil {
		t.Fatalf("read candidate: %v", err)
	}
	if !bytes.Equal(body, payload) {
		t.Fatalf("expected ungzipped payload %q, got %q", payload, body)
	}
	info, err := os.Stat(candidate)
	if err != nil {
		t.Fatalf("stat candidate: %v", err)
	}
	if info.Size() == 0 {
		t.Fatalf("expected non-empty candidate")
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("expected candidate to be executable, mode=%v", info.Mode())
	}
}

func TestReadBinaryVersionUsesCandidatePath(t *testing.T) {
	app, _ := newTestApp(t)
	var calls []commandCall
	app.Runner = fakeRunner{
		outputFn: func(name string, args ...string) (string, string, error) {
			calls = append(calls, commandCall{name: name, args: append([]string{}, args...)})
			return "Mihomo Meta alpha-c59c99a\n", "", nil
		},
	}

	version, err := app.readBinaryVersion("/tmp/mihomo-core")
	if err != nil {
		t.Fatalf("read binary version: %v", err)
	}
	if version != "Mihomo Meta alpha-c59c99a" {
		t.Fatalf("unexpected version: %q", version)
	}
	if !hasRecordedCall(calls, "/tmp/mihomo-core", "-v") {
		t.Fatalf("expected version command call, got %#v", calls)
	}
}

func TestReadBinaryVersionFallsBackToStderr(t *testing.T) {
	app, _ := newTestApp(t)
	app.Runner = fakeRunner{
		outputFn: func(name string, args ...string) (string, string, error) {
			return "", "Mihomo Meta alpha-c59c99a\n", nil
		},
	}

	version, err := app.readBinaryVersion("/tmp/mihomo-core")
	if err != nil {
		t.Fatalf("read binary version: %v", err)
	}
	if version != "Mihomo Meta alpha-c59c99a" {
		t.Fatalf("expected stderr version fallback, got %q", version)
	}
}

func TestCoreUpgradeAlphaRequiresRoot(t *testing.T) {
	app, _ := newTestApp(t)
	oldGeteuid := geteuid
	geteuid = func() int { return 1000 }
	defer func() { geteuid = oldGeteuid }()

	err := app.CoreUpgradeAlpha()
	if err == nil || !strings.Contains(err.Error(), "请用 root 运行") {
		t.Fatalf("expected root error, got %v", err)
	}
}

func TestCoreUpgradeAlphaReplacesBinaryAndRestartsService(t *testing.T) {
	app, root := newTestApp(t)
	oldGeteuid := geteuid
	geteuid = func() int { return 0 }
	defer func() { geteuid = oldGeteuid }()
	oldCurrentGOOS := currentGOOS
	oldCurrentGOARCH := currentGOARCH
	currentGOOS = func() string { return "linux" }
	currentGOARCH = func() string { return "arm64" }
	defer func() {
		currentGOOS = oldCurrentGOOS
		currentGOARCH = oldCurrentGOARCH
	}()

	cfg, err := config.Ensure(app.Paths.ConfigPath())
	if err != nil {
		t.Fatalf("ensure config: %v", err)
	}
	corePath := filepath.Join(root, "bin", "mihomo-core")
	cfg.Install.CoreBin = corePath
	if err := config.Save(app.Paths.ConfigPath(), cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(corePath), 0o755); err != nil {
		t.Fatalf("mkdir core dir: %v", err)
	}
	if err := os.WriteFile(corePath, []byte("old-core"), 0o755); err != nil {
		t.Fatalf("write old core: %v", err)
	}

	var calls []commandCall
	app.Runner = fakeRunner{
		runFn: func(name string, args ...string) error {
			calls = append(calls, commandCall{name: name, args: append([]string{}, args...)})
			return nil
		},
		outputFn: func(name string, args ...string) (string, string, error) {
			calls = append(calls, commandCall{name: name, args: append([]string{}, args...)})
			switch {
			case name == "systemctl" && len(args) == 2 && args[0] == "is-active" && args[1] == "minimalist.service":
				return "active\n", "", nil
			case strings.HasSuffix(name, "mihomo-core") && len(args) == 1 && args[0] == "-v":
				body, err := os.ReadFile(name)
				if err != nil {
					t.Fatalf("read version target: %v", err)
				}
				if bytes.Equal(body, []byte("old-core")) {
					return "Mihomo Meta alpha-old\n", "", nil
				}
				return "Mihomo Meta alpha-new\n", "", nil
			default:
				t.Fatalf("unexpected output call: %s %v", name, args)
				return "", "", nil
			}
		},
	}
	app.Client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.String() {
			case mihomoReleasesAPI:
				return textResponse(http.StatusOK, `[{"tag_name":"Prerelease-Alpha","name":"Prerelease-Alpha","prerelease":true,"published_at":"2026-04-28T00:00:00Z","assets":[{"name":"mihomo-linux-arm64-v1.19.23.gz","browser_download_url":"https://example.com/mihomo.gz"}]}]`), nil
			case "https://example.com/mihomo.gz":
				return gzippedResponse(t, []byte("new-core")), nil
			default:
				t.Fatalf("unexpected url: %s", req.URL.String())
				return nil, nil
			}
		}),
	}

	if err := app.CoreUpgradeAlpha(); err != nil {
		t.Fatalf("core upgrade alpha: %v", err)
	}
	if !hasRecordedCall(calls, "systemctl", "restart", "minimalist.service") {
		t.Fatalf("expected restart call, got %#v", calls)
	}
	if !hasRecordedCall(calls, "systemctl", "is-active", "minimalist.service") {
		t.Fatalf("expected is-active call, got %#v", calls)
	}
	body, err := os.ReadFile(corePath)
	if err != nil {
		t.Fatalf("read core path: %v", err)
	}
	if string(body) != "new-core" {
		t.Fatalf("expected replaced core, got %q", string(body))
	}
	if _, err := os.Stat(corePath + ".bak"); !os.IsNotExist(err) {
		t.Fatalf("expected successful restart to remove backup, err=%v", err)
	}
	stdout := app.Stdout.(*bytes.Buffer).String()
	for _, want := range []string{
		"core path: " + corePath,
		"release: Prerelease-Alpha",
		"asset: mihomo-linux-arm64-v1.19.23.gz",
		"old version: Mihomo Meta alpha-old",
		"new version: Mihomo Meta alpha-new",
		"service restarted: minimalist.service",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected %q in stdout:\n%s", want, stdout)
		}
	}
}

func TestCoreUpgradeAlphaSurfacesRestartFailureWithLogs(t *testing.T) {
	app, root := newTestApp(t)
	oldGeteuid := geteuid
	geteuid = func() int { return 0 }
	defer func() { geteuid = oldGeteuid }()
	oldCurrentGOOS := currentGOOS
	oldCurrentGOARCH := currentGOARCH
	currentGOOS = func() string { return "linux" }
	currentGOARCH = func() string { return "arm64" }
	defer func() {
		currentGOOS = oldCurrentGOOS
		currentGOARCH = oldCurrentGOARCH
	}()

	cfg, err := config.Ensure(app.Paths.ConfigPath())
	if err != nil {
		t.Fatalf("ensure config: %v", err)
	}
	cfg.Install.CoreBin = filepath.Join(root, "bin", "mihomo-core")
	if err := config.Save(app.Paths.ConfigPath(), cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(cfg.Install.CoreBin), 0o755); err != nil {
		t.Fatalf("mkdir core dir: %v", err)
	}
	if err := os.WriteFile(cfg.Install.CoreBin, []byte("old-core"), 0o755); err != nil {
		t.Fatalf("write old core: %v", err)
	}

	app.Runner = fakeRunner{
		runFn: func(name string, args ...string) error {
			if name == "systemctl" && len(args) == 2 && args[0] == "restart" && args[1] == "minimalist.service" {
				return errors.New("restart failed")
			}
			return nil
		},
		outputFn: func(name string, args ...string) (string, string, error) {
			switch {
			case name == "journalctl":
				return "line1\nline2\n", "", nil
			case strings.HasSuffix(name, "mihomo-core") && len(args) == 1 && args[0] == "-v":
				return "Mihomo Meta alpha-version\n", "", nil
			default:
				return "", "", nil
			}
		},
	}
	app.Client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() == mihomoReleasesAPI {
				return textResponse(http.StatusOK, `[{"tag_name":"Prerelease-Alpha","name":"Prerelease-Alpha","prerelease":true,"published_at":"2026-04-28T00:00:00Z","assets":[{"name":"mihomo-linux-arm64-v1.19.23.gz","browser_download_url":"https://example.com/mihomo.gz"}]}]`), nil
			}
			return gzippedResponse(t, []byte("new-core")), nil
		}),
	}

	err = app.CoreUpgradeAlpha()
	if err == nil || !strings.Contains(err.Error(), "restart failed") {
		t.Fatalf("expected restart failure, got %v", err)
	}
	stderr := app.Stderr.(*bytes.Buffer).String()
	if !strings.Contains(stderr, "line1") || !strings.Contains(stderr, "line2") {
		t.Fatalf("expected journal output in stderr:\n%s", stderr)
	}
	backupBody, readErr := os.ReadFile(cfg.Install.CoreBin + ".bak")
	if readErr != nil {
		t.Fatalf("expected backup to remain after restart failure: %v", readErr)
	}
	if string(backupBody) != "old-core" {
		t.Fatalf("expected backup to contain old core, got %q", string(backupBody))
	}
}

func TestRestartMinimalistServiceAfterCoreUpgradeRejectsInactiveStatusOutput(t *testing.T) {
	app, _ := newTestApp(t)
	app.Runner = fakeRunner{
		runFn: func(name string, args ...string) error {
			return nil
		},
		outputFn: func(name string, args ...string) (string, string, error) {
			if name == "systemctl" && len(args) == 2 && args[0] == "is-active" && args[1] == "minimalist.service" {
				return "inactive\n", "", nil
			}
			return "", "", nil
		},
	}

	err := app.restartMinimalistServiceAfterCoreUpgrade()
	if err == nil || !strings.Contains(err.Error(), "not active") {
		t.Fatalf("expected inactive status error, got %v", err)
	}
}

func TestDownloadReleaseAssetRejectsHTTPFailures(t *testing.T) {
	tests := []struct {
		name   string
		status int
		want   string
	}{
		{name: "client error", status: http.StatusNotFound, want: "http 404"},
		{name: "server error", status: http.StatusBadGateway, want: "http 502"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app, _ := newTestApp(t)
			app.Client = &http.Client{
				Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					return textResponse(tc.status, http.StatusText(tc.status)), nil
				}),
			}

			_, err := app.downloadReleaseAsset(githubReleaseAsset{
				Name:               "mihomo-linux-amd64-v1.19.23.gz",
				BrowserDownloadURL: "https://example.com/mihomo.gz",
			}, filepath.Join(t.TempDir(), "bin", "mihomo-core"))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %s failure, got %v", tc.want, err)
			}
		})
	}
}

func TestDownloadReleaseAssetRejectsEmptyPayload(t *testing.T) {
	app, _ := newTestApp(t)
	app.Client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return gzippedResponse(t, nil), nil
		}),
	}

	_, err := app.downloadReleaseAsset(githubReleaseAsset{
		Name:               "mihomo-linux-amd64-v1.19.23.gz",
		BrowserDownloadURL: "https://example.com/mihomo.gz",
	}, filepath.Join(t.TempDir(), "bin", "mihomo-core"))
	if err == nil || !strings.Contains(err.Error(), "empty asset payload") {
		t.Fatalf("expected empty payload error, got %v", err)
	}
}

func TestReleaseIsNewerUsesNaturalOrderingForEqualPublishedAt(t *testing.T) {
	publishedAt := mustParseRFC3339(t, "2026-04-28T00:00:00Z")
	newer := githubRelease{
		TagName:     "v1.19.10-alpha-1",
		Name:        "v1.19.10 alpha 1",
		PublishedAt: publishedAt,
	}
	older := githubRelease{
		TagName:     "v1.19.9-alpha-9",
		Name:        "v1.19.9 alpha 9",
		PublishedAt: publishedAt,
	}

	if !releaseIsNewer(newer, older) {
		t.Fatalf("expected %s to be newer than %s", newer.TagName, older.TagName)
	}
	if releaseIsNewer(older, newer) {
		t.Fatalf("did not expect %s to be newer than %s", older.TagName, newer.TagName)
	}
}

func gzippedResponse(t *testing.T, payload []byte) *http.Response {
	t.Helper()
	var body bytes.Buffer
	zw := gzip.NewWriter(&body)
	if _, err := zw.Write(payload); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(body.Bytes())),
		Header:     make(http.Header),
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
