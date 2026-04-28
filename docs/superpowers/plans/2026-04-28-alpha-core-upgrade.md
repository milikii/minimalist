# Alpha Core Upgrade Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 `minimalist` 增加一个显式命令，从官方 `MetaCubeX/mihomo` GitHub releases 列表中选择最新 alpha prerelease，下载匹配架构的 `mihomo-core` Linux 资产，原子替换本机 `install.core_bin`，并自动重启 `minimalist.service`。

**Architecture:** 保持现有命令编排边界，把升级逻辑放到 `internal/app`，单独拆成聚焦文件 `internal/app/core_upgrade.go`，避免继续膨胀 [internal/app/app.go](/home/projects/minimalist/internal/app/app.go:1)。CLI 只做分发。测试继续沿用 `fakeRunner` 和自定义 `http.Client` 的模式，在 app 层覆盖发布选择、下载解包、版本校验、替换和重启失败传播。

**Tech Stack:** Go 标准库 `net/http`、`encoding/json`、`compress/gzip`、`os`、`filepath`、`runtime`，现有 `internal/app`、`internal/cli`、`internal/system`。

---

## File Map

- Create: `internal/app/core_upgrade.go`
  - 负责 release 查询、alpha 资产选择、下载、解包、版本校验、原子替换、服务重启、结果输出
- Create: `internal/app/core_upgrade_test.go`
  - 负责 focused app tests，避免继续把升级链路塞进超长的 [internal/app/app_test.go](/home/projects/minimalist/internal/app/app_test.go:1)
- Modify: `internal/cli/cli.go`
  - 新增 `core-upgrade-alpha` 分发与 usage
- Modify: `internal/cli/cli_test.go`
  - 新增 CLI 分发与 usage 覆盖
- Modify: `docs/DECISIONS.md`
  - 明确“显式 alpha 升级”与“不恢复通道/回滚”并存
- Modify: `docs/STATUS.md`
  - 记录能力、验证范围和限制
- Modify: `docs/NEXT_STEP.md`
  - 从“完全不引入 core 升级”改为“仅引入显式 alpha 升级”
- Modify: `README.md`
  - 增加命令说明、root 要求和自动重启行为

## Task 1: 发布发现与 alpha 资产选择

**Files:**
- Create: `internal/app/core_upgrade.go`
- Test: `internal/app/core_upgrade_test.go`

- [ ] **Step 1: 写发布发现与架构选择的失败测试**

```go
func TestSelectLatestAlphaAssetChoosesFirstMatchingLinuxAsset(t *testing.T) {
	releases := []githubRelease{
		{
			TagName:    "v1.19.23",
			Name:       "v1.19.23",
			Prerelease: false,
			Assets: []githubReleaseAsset{
				{Name: "mihomo-linux-amd64-v1.19.23.gz", BrowserDownloadURL: "https://example.com/stable.gz"},
			},
		},
		{
			TagName:    "Prerelease-Alpha",
			Name:       "Prerelease-Alpha",
			Prerelease: true,
			Assets: []githubReleaseAsset{
				{Name: "mihomo-darwin-amd64-v1.19.23.gz", BrowserDownloadURL: "https://example.com/darwin.gz"},
				{Name: "mihomo-linux-amd64-v1.19.23.gz", BrowserDownloadURL: "https://example.com/linux.gz"},
			},
		},
	}

	release, asset, err := selectLatestAlphaAsset(releases, "linux", "amd64")
	if err != nil {
		t.Fatalf("select latest alpha asset: %v", err)
	}
	if release.TagName != "Prerelease-Alpha" {
		t.Fatalf("expected alpha release, got %+v", release)
	}
	if asset.Name != "mihomo-linux-amd64-v1.19.23.gz" {
		t.Fatalf("expected linux amd64 asset, got %+v", asset)
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
```

- [ ] **Step 2: 运行 focused test，确认当前缺少实现**

Run:

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/app -run 'TestSelectLatestAlphaAsset' -count=1
```

Expected:

```text
FAIL	minimalist/internal/app [build failed]
```

并且报错包含 `undefined: githubRelease` 或 `undefined: selectLatestAlphaAsset`。

- [ ] **Step 3: 在 `internal/app/core_upgrade.go` 写最小实现**

```go
package app

import (
	"fmt"
	"runtime"
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
	for _, release := range releases {
		if !release.Prerelease {
			continue
		}
		alphaLabel := strings.ToLower(release.TagName + " " + release.Name)
		if !strings.Contains(alphaLabel, "alpha") {
			continue
		}
		for _, asset := range release.Assets {
			if strings.Contains(asset.Name, "mihomo-linux-"+arch) && strings.HasSuffix(asset.Name, ".gz") {
				return release, asset, nil
			}
		}
	}
	return githubRelease{}, githubReleaseAsset{}, fmt.Errorf("no matching alpha asset for %s/%s", goos, goarch)
}

func currentGOOS() string   { return runtime.GOOS }
func currentGOARCH() string { return runtime.GOARCH }
```

- [ ] **Step 4: 重新运行 focused test，确认选择逻辑通过**

Run:

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/app -run 'TestSelectLatestAlphaAsset' -count=1
```

Expected:

```text
ok  	minimalist/internal/app	0.xxxs
```

- [ ] **Step 5: 提交这个最小闭环**

```bash
git add internal/app/core_upgrade.go internal/app/core_upgrade_test.go
git commit -m "test: cover alpha release asset selection"
```

## Task 2: 下载、解包与候选二进制版本校验

**Files:**
- Modify: `internal/app/core_upgrade.go`
- Test: `internal/app/core_upgrade_test.go`

- [ ] **Step 1: 先写下载、解包与版本校验的失败测试**

```go
func TestDownloadReleaseAssetWritesExecutableCandidate(t *testing.T) {
	app, _ := newTestApp(t)
	var requested string
	app.Client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requested = req.URL.String()
			var body bytes.Buffer
			zw := gzip.NewWriter(&body)
			if _, err := zw.Write([]byte("#!/bin/sh\nexit 0\n")); err != nil {
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
	})
	if err != nil {
		t.Fatalf("download release asset: %v", err)
	}
	if requested != "https://example.com/mihomo.gz" {
		t.Fatalf("unexpected asset url: %s", requested)
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
		runFn: func(name string, args ...string) error { return nil },
		outputFn: func(name string, args ...string) (string, string, error) {
			calls = append(calls, commandCall{name: name, args: append([]string{}, args...)})
			return "Mihomo Meta alpha-c59c99a", "", nil
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

func TestDownloadReleaseAssetRejectsHTTPFailure(t *testing.T) {
	app, _ := newTestApp(t)
	app.Client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return textResponse(http.StatusBadGateway, "bad gateway"), nil
		}),
	}

	_, err := app.downloadReleaseAsset(githubReleaseAsset{
		Name:               "mihomo-linux-amd64-v1.19.23.gz",
		BrowserDownloadURL: "https://example.com/mihomo.gz",
	})
	if err == nil || !strings.Contains(err.Error(), "http 502") {
		t.Fatalf("expected http failure, got %v", err)
	}
}
```

- [ ] **Step 2: 运行 focused test，确认 helper 尚未实现**

Run:

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/app -run 'TestDownloadReleaseAsset|TestReadBinaryVersion' -count=1
```

Expected:

```text
FAIL	minimalist/internal/app [build failed]
```

并且报错包含 `app.downloadReleaseAsset undefined` 或 `app.readBinaryVersion undefined`。

- [ ] **Step 3: 写最小 helper 实现**

```go
func (a *App) downloadReleaseAsset(asset githubReleaseAsset) (string, error) {
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

	tmpDir, err := os.MkdirTemp(filepath.Dir(a.Paths.BinPath), ".mihomo-core-*")
	if err != nil {
		return "", err
	}
	candidate := filepath.Join(tmpDir, "mihomo-core")
	if strings.HasSuffix(asset.Name, ".gz") {
		zr, err := gzip.NewReader(resp.Body)
		if err != nil {
			return "", err
		}
		defer zr.Close()
		body, err := io.ReadAll(zr)
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(candidate, body, 0o755); err != nil {
			return "", err
		}
		return candidate, nil
	}
	return "", fmt.Errorf("unsupported asset format: %s", asset.Name)
}

func (a *App) readBinaryVersion(path string) (string, error) {
	stdout, _, err := a.Runner.Output(path, "-v")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(stdout), nil
}
```

- [ ] **Step 4: 运行 focused test，确认下载和版本 helper 通过**

Run:

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/app -run 'TestDownloadReleaseAsset|TestReadBinaryVersion' -count=1
```

Expected:

```text
ok  	minimalist/internal/app	0.xxxs
```

- [ ] **Step 5: 提交 helper 闭环**

```bash
git add internal/app/core_upgrade.go internal/app/core_upgrade_test.go
git commit -m "test: cover alpha core download helpers"
```

## Task 3: 编排升级、替换与自动重启

**Files:**
- Modify: `internal/app/core_upgrade.go`
- Test: `internal/app/core_upgrade_test.go`

- [ ] **Step 1: 先写端到端编排失败测试**

```go
func TestCoreUpgradeAlphaRequiresRoot(t *testing.T) {
	app, _ := newTestApp(t)
	oldGeteuid := geteuid
	geteuid = func() int { return 1000 }
	defer func() { geteuid = oldGeteuid }()

	if err := app.CoreUpgradeAlpha(); err == nil || !strings.Contains(err.Error(), "请用 root 运行") {
		t.Fatalf("expected root error, got %v", err)
	}
}

func TestCoreUpgradeAlphaReplacesBinaryAndRestartsService(t *testing.T) {
	app, root := newTestApp(t)
	oldGeteuid := geteuid
	geteuid = func() int { return 0 }
	defer func() { geteuid = oldGeteuid }()

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
			case name == "systemctl" && len(args) >= 2 && args[0] == "is-active":
				return "", "", nil
			case name == "journalctl":
				return "", "", nil
			case strings.HasSuffix(name, "mihomo-core") && len(args) == 1 && args[0] == "-v":
				if bytes.Equal(mustReadFile(t, name), []byte("old-core")) {
					return "Mihomo Meta alpha-old", "", nil
				}
				return "Mihomo Meta alpha-new", "", nil
			default:
				return "", "", fmt.Errorf("unexpected output call: %s %v", name, args)
			}
		},
	}
	app.Client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() == mihomoReleasesAPI {
				return textResponse(http.StatusOK, `[{"tag_name":"Prerelease-Alpha","name":"Prerelease-Alpha","prerelease":true,"assets":[{"name":"mihomo-linux-amd64-v1.19.23.gz","browser_download_url":"https://example.com/mihomo.gz"}]}]`), nil
			}
			if req.URL.String() == "https://example.com/mihomo.gz" {
				var body bytes.Buffer
				zw := gzip.NewWriter(&body)
				if _, err := zw.Write([]byte("new-core")); err != nil {
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
			}
			return nil, fmt.Errorf("unexpected url: %s", req.URL.String())
		}),
	}

	if err := app.CoreUpgradeAlpha(); err != nil {
		t.Fatalf("core upgrade alpha: %v", err)
	}
	if !hasRecordedCall(calls, "systemctl", "restart", "minimalist.service") {
		t.Fatalf("expected restart call, got %#v", calls)
	}
	body, err := os.ReadFile(corePath)
	if err != nil {
		t.Fatalf("read core path: %v", err)
	}
	if string(body) != "new-core" {
		t.Fatalf("expected replaced core, got %q", string(body))
	}
}

func TestCoreUpgradeAlphaSurfacesRestartFailureWithLogs(t *testing.T) {
	app, root := newTestApp(t)
	oldGeteuid := geteuid
	geteuid = func() int { return 0 }
	defer func() { geteuid = oldGeteuid }()

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
			if name == "systemctl" && len(args) >= 2 && args[0] == "restart" {
				return errors.New("restart failed")
			}
			return nil
		},
		outputFn: func(name string, args ...string) (string, string, error) {
			switch {
			case strings.HasSuffix(name, "mihomo-core") && len(args) == 1 && args[0] == "-v":
				return "Mihomo Meta alpha-new", "", nil
			case name == "systemctl" && len(args) >= 2 && args[0] == "is-active":
				return "", "", errors.New("inactive")
			case name == "journalctl":
				return "line1\nline2\n", "", nil
			default:
				return "", "", nil
			}
		},
	}
	app.Client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() == mihomoReleasesAPI {
				return textResponse(http.StatusOK, `[{"tag_name":"Prerelease-Alpha","name":"Prerelease-Alpha","prerelease":true,"assets":[{"name":"mihomo-linux-amd64-v1.19.23.gz","browser_download_url":"https://example.com/mihomo.gz"}]}]`), nil
			}
			var body bytes.Buffer
			zw := gzip.NewWriter(&body)
			if _, err := zw.Write([]byte("new-core")); err != nil {
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

	err = app.CoreUpgradeAlpha()
	if err == nil || !strings.Contains(err.Error(), "restart failed") {
		t.Fatalf("expected restart failure, got %v", err)
	}
	if !strings.Contains(app.Stderr.(*bytes.Buffer).String(), "line1") {
		t.Fatalf("expected journal output in stderr:\n%s", app.Stderr.(*bytes.Buffer).String())
	}
}
```

- [ ] **Step 2: 运行 focused test，确认编排命令尚未实现**

Run:

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/app -run 'TestCoreUpgradeAlpha' -count=1
```

Expected:

```text
FAIL	minimalist/internal/app [build failed]
```

并且报错包含 `app.CoreUpgradeAlpha undefined`。

- [ ] **Step 3: 写最小可用实现**

```go
func (a *App) CoreUpgradeAlpha() error {
	if err := a.requireRoot(); err != nil {
		return err
	}
	cfg, _, err := a.ensureAll()
	if err != nil {
		return err
	}

	currentVersion := ""
	if version, err := a.readBinaryVersion(cfg.Install.CoreBin); err == nil {
		currentVersion = version
	}

	releases, err := a.fetchMihomoReleases()
	if err != nil {
		return err
	}
	release, asset, err := selectLatestAlphaAsset(releases, currentGOOS(), currentGOARCH())
	if err != nil {
		return err
	}

	candidate, err := a.downloadReleaseAsset(asset)
	if err != nil {
		return err
	}
	newVersion, err := a.readBinaryVersion(candidate)
	if err != nil {
		return err
	}

	backupPath, err := replaceFileAtomically(cfg.Install.CoreBin, candidate)
	if err != nil {
		return err
	}
	if err := a.Runner.Run("systemctl", "restart", "minimalist.service"); err != nil {
		logs, _, _ := a.Runner.Output("journalctl", "-u", "minimalist.service", "-n", "20", "--no-pager")
		if strings.TrimSpace(logs) != "" {
			fmt.Fprintln(a.Stderr, logs)
		}
		return err
	}
	if _, _, err := a.Runner.Output("systemctl", "is-active", "minimalist.service"); err != nil {
		return err
	}

	_ = os.Remove(backupPath)
	fmt.Fprintf(a.Stdout, "core path: %s\n", cfg.Install.CoreBin)
	fmt.Fprintf(a.Stdout, "release: %s\n", release.TagName)
	fmt.Fprintf(a.Stdout, "asset: %s\n", asset.Name)
	if currentVersion != "" {
		fmt.Fprintf(a.Stdout, "old version: %s\n", currentVersion)
	}
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

func replaceFileAtomically(targetPath, candidatePath string) (string, error) {
	backupPath := targetPath + ".bak"
	if err := os.Remove(backupPath); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if err := os.Rename(targetPath, backupPath); err != nil {
		return "", err
	}
	if err := os.Rename(candidatePath, targetPath); err != nil {
		return backupPath, err
	}
	return backupPath, nil
}
```

- [ ] **Step 4: 运行 focused test，确认编排链路通过**

Run:

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/app -run 'TestCoreUpgradeAlpha' -count=1
```

Expected:

```text
ok  	minimalist/internal/app	0.xxxs
```

- [ ] **Step 5: 提交升级编排闭环**

```bash
git add internal/app/core_upgrade.go internal/app/core_upgrade_test.go
git commit -m "feat: add alpha core upgrade command"
```

## Task 4: CLI 分发、文档同步与回归验证

**Files:**
- Modify: `internal/cli/cli.go`
- Modify: `internal/cli/cli_test.go`
- Modify: `docs/DECISIONS.md`
- Modify: `docs/STATUS.md`
- Modify: `docs/NEXT_STEP.md`
- Modify: `README.md`

- [ ] **Step 1: 先写 CLI 分发与 usage 的失败测试**

```go
func TestRunWithAppDispatchesCoreUpgradeAlpha(t *testing.T) {
	a, stdout := newCLIApp(t)
	called := false
	a.Runner = noopRunner{}
	a.Client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("network disabled")
	})}

	original := appCoreUpgradeAlpha
	appCoreUpgradeAlpha = func(target *app.App) error {
		called = true
		_, _ = stdout.WriteString("core upgrade called\n")
		return nil
	}
	defer func() { appCoreUpgradeAlpha = original }()

	if err := runWithApp([]string{"core-upgrade-alpha"}, a, false); err != nil {
		t.Fatalf("run core-upgrade-alpha: %v", err)
	}
	if !called {
		t.Fatalf("expected core-upgrade-alpha dispatch")
	}
}

func TestPrintUsageIncludesCoreUpgradeAlpha(t *testing.T) {
	output := captureStdout(t, printUsage)
	if !strings.Contains(output, "minimalist core-upgrade-alpha") {
		t.Fatalf("missing core-upgrade-alpha in usage:\n%s", output)
	}
}
```

- [ ] **Step 2: 运行 CLI focused test，确认分发尚未接线**

Run:

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/cli -run 'TestRunWithAppDispatchesCoreUpgradeAlpha|TestPrintUsageIncludesCoreUpgradeAlpha' -count=1
```

Expected:

```text
FAIL	minimalist/internal/cli [build failed]
```

并且报错包含 `unknown command: core-upgrade-alpha` 或 usage 中缺少新命令。

- [ ] **Step 3: 接好 CLI，并同步文档**

```go
// internal/cli/cli.go
var appCoreUpgradeAlpha = func(a *app.App) error {
	return a.CoreUpgradeAlpha()
}

func runWithApp(args []string, a *app.App, tty bool) error {
	// ...
	switch args[0] {
	case "core-upgrade-alpha":
		return appCoreUpgradeAlpha(a)
	// ...
	}
}

func printUsage() {
	fmt.Println(`minimalist commands:
  minimalist menu
  minimalist install-self
  minimalist setup
  minimalist render-config
  minimalist core-upgrade-alpha
  minimalist start|stop|restart
  minimalist status|show-secret|healthcheck|runtime-audit
  minimalist cutover-preflight
  minimalist cutover-plan
  minimalist import-links
  minimalist router-wizard
  minimalist apply-rules|clear-rules
  minimalist nodes list|rename|enable|disable|remove
  minimalist subscriptions list|add|enable|disable|remove|update
  minimalist rules list|add|remove
  minimalist acl list|add|remove
  minimalist rules-repo summary|entries|find|add|remove|remove-index`)
}
```

```md
<!-- docs/DECISIONS.md -->
- 项目仍不恢复 `alpha/stable` 核心通道切换或 core 回滚。
- 项目允许新增单一用途命令 `core-upgrade-alpha`，仅从官方 `MetaCubeX/mihomo` releases 升级最新 alpha 内核并自动重启 `minimalist.service`。

<!-- README.md -->
- 新增：`sudo minimalist core-upgrade-alpha`
- 行为：读取官方 GitHub releases，选择当前 Linux 架构匹配的最新 alpha 资产，替换 `install.core_bin`，自动重启 `minimalist.service`
- 限制：不提供 stable 通道、指定版本、回滚或自动定时升级
```

- [ ] **Step 4: 跑 CLI focused test 和全量回归**

Run:

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/cli -run 'TestRunWithAppDispatchesCoreUpgradeAlpha|TestPrintUsageIncludesCoreUpgradeAlpha' -count=1
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/app -run 'TestSelectLatestAlphaAsset|TestDownloadReleaseAsset|TestReadBinaryVersion|TestCoreUpgradeAlpha' -count=1
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./...
```

Expected:

```text
ok  	minimalist/internal/cli	0.xxxs
ok  	minimalist/internal/app	0.xxxs
ok  	minimalist/...	...
```

- [ ] **Step 5: 提交 CLI 与文档闭环**

```bash
git add internal/cli/cli.go internal/cli/cli_test.go docs/DECISIONS.md docs/STATUS.md docs/NEXT_STEP.md README.md
git commit -m "docs: document alpha core upgrade"
```

## Self-Review Checklist

- Spec coverage:
  - 官方 `MetaCubeX/mihomo` releases 查询: Task 3
  - 最新 alpha prerelease 与架构资产选择: Task 1
  - 下载、解包与版本校验: Task 2
  - 原子替换与自动重启: Task 3
  - CLI 分发与文档同步: Task 4
- Placeholder scan:
  - 本计划不包含 `TODO`、`TBD`、`implement later`、`类似 Task N`
  - 每个代码步骤都给出实际代码片段和命令
- Type consistency:
  - 统一使用 `githubRelease`、`githubReleaseAsset`、`CoreUpgradeAlpha`、`downloadReleaseAsset`、`readBinaryVersion`

## Notes For Execution

- 若官方 alpha 资产实际不是单纯 `.gz` 裸二进制，而是 `.tar.gz` 容器包，优先在 Task 2 的 focused tests 中先补一个真实结构样本，再最小扩展到 `tar.gz`，不要在实现前预埋多格式分支。
- 不要把该命令塞回 `setup`、`install-self` 或菜单；本计划明确保持显式命令边界。
- 不要顺手加入回滚命令、stable 通道或自动定时升级。
