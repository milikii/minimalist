<!-- /autoplan restore point: /tmp/main-autoplan-restore-20260428-174048.md -->
# Long-Term Stability Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把当前 Go 版 `minimalist` 从“本机已成功切换并可用”推进到“更接近长期稳定值守”的下一阶段，优先补齐 runtime asset 自检、重启/重启后 smoke 闭环、以及 `core-upgrade-alpha` 的可执行恢复路径；其中 asset 自检必须覆盖手动命令链路和 systemd 开机启动链路，不能只修一半。

**Architecture:** 保持现有边界，不扩产品能力、不恢复 stable 通道、不引入自动 cutover。实现上只沿着现有 `internal/runtime`、`internal/app`、`internal/cli` 与运维文档收口：先提供 runtime asset 缺口判定原语，再让 `setup/start/restart/healthcheck/runtime-audit` 和 systemd `ExecStartPre` 复用同一套 fail-fast 语义，随后把 `core-upgrade-alpha` 的恢复信息和 reboot/restart runbook 文档化。

**Tech Stack:** Go 标准库 `os`、`path/filepath`、`strings`，现有 `internal/runtime`、`internal/app`、`internal/cli`、`internal/system`，以及 `README.md` / `docs/CUTOVER.md` / `docs/STATUS.md` / `docs/NEXT_STEP.md`。

---

## File Map

- Modify: `internal/runtime/runtime.go`
  - 增加 runtime asset 路径与缺口判定 helper
- Modify: `internal/runtime/runtime_test.go`
  - 覆盖 asset 缺失 / 完整两条基础语义
- Modify: `internal/app/app.go`
  - 为 `setup` / `start` / `restart` / `healthcheck` / `runtime-audit` 接入统一 asset 自检
- Modify: `internal/cli/cli.go`
  - 如采用隐藏校验命令，新增 `verify-runtime-assets` 分发，供 systemd `ExecStartPre` 使用
- Modify: `internal/app/app_test.go`
  - 补 asset 缺失 fail-fast、`runtime-audit` fatal-gap、无 systemctl 副作用等 focused tests
- Modify: `internal/cli/cli_test.go`
  - 覆盖隐藏 asset 校验命令的 CLI 分发与输出契约
- Modify: `internal/app/core_upgrade.go`
  - 升级失败时明确输出 backup 路径与恢复建议
- Modify: `internal/app/core_upgrade_test.go`
  - 覆盖恢复提示与 backup 保留语义
- Modify: `README.md`
  - 更新 asset 自检与恢复路径说明
- Modify: `docs/README_FLOWS.md`
  - 更新当前运行态与 fail-fast 真相
- Modify: `docs/CUTOVER.md`
  - 追加 restart/reboot smoke runbook 与升级失败恢复步骤
- Modify: `docs/STATUS.md`
  - 记录稳定性闭环推进结果
- Modify: `docs/NEXT_STEP.md`
  - 主线从 asset 自检推进到 reboot/restart smoke

---

### Task 1: Runtime Asset 缺口判定原语

**Files:**
- Modify: `internal/runtime/runtime.go`
- Test: `internal/runtime/runtime_test.go`

- [ ] **Step 1: 先写 failing tests，固定 asset 缺口语义**

```go
func TestMissingRuntimeAssetsReportsAbsentFilesAndDirectories(t *testing.T) {
	paths := Paths{RuntimeDir: t.TempDir()}
	if err := EnsureLayout(paths); err != nil {
		t.Fatalf("ensure layout: %v", err)
	}
	if err := os.RemoveAll(paths.UIPath()); err != nil {
		t.Fatalf("remove ui dir: %v", err)
	}

	missing := MissingRuntimeAssets(paths)
	want := []string{"Country.mmdb", "GeoSite.dat", "ui/"}
	if !reflect.DeepEqual(missing, want) {
		t.Fatalf("missing assets = %#v, want %#v", missing, want)
	}
}

func TestMissingRuntimeAssetsReturnsNilWhenAssetsPresent(t *testing.T) {
	paths := Paths{RuntimeDir: t.TempDir()}
	if err := EnsureLayout(paths); err != nil {
		t.Fatalf("ensure layout: %v", err)
	}
	if err := os.WriteFile(paths.CountryMMDBPath(), []byte("mmdb"), 0o640); err != nil {
		t.Fatalf("write mmdb: %v", err)
	}
	if err := os.WriteFile(paths.GeoSitePath(), []byte("geosite"), 0o640); err != nil {
		t.Fatalf("write geosite: %v", err)
	}

	missing := MissingRuntimeAssets(paths)
	if len(missing) != 0 {
		t.Fatalf("expected no missing assets, got %#v", missing)
	}
}
```

- [ ] **Step 2: 跑 focused tests，确认当前缺少实现**

Run:

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/runtime -run TestMissingRuntimeAssets -count=1
```

Expected:

```text
FAIL	minimalist/internal/runtime [build failed]
```

并且报错包含 `undefined: MissingRuntimeAssets` / `undefined: CountryMMDBPath`。

- [ ] **Step 3: 写最小实现，只提供路径和缺口列表，不做自动修复**

```go
func (p Paths) CountryMMDBPath() string { return filepath.Join(p.RuntimeDir, "Country.mmdb") }
func (p Paths) GeoSitePath() string     { return filepath.Join(p.RuntimeDir, "GeoSite.dat") }

func MissingRuntimeAssets(paths Paths) []string {
	missing := make([]string, 0, 3)
	for _, item := range []struct {
		label string
		path  string
		dir   bool
	}{
		{label: "Country.mmdb", path: paths.CountryMMDBPath()},
		{label: "GeoSite.dat", path: paths.GeoSitePath()},
		{label: "ui/", path: paths.UIPath(), dir: true},
	} {
		info, err := os.Stat(item.path)
		if err != nil {
			missing = append(missing, item.label)
			continue
		}
		if item.dir {
			if !info.IsDir() {
				missing = append(missing, item.label)
			}
			continue
		}
		if info.IsDir() || info.Size() == 0 {
			missing = append(missing, item.label)
		}
	}
	return missing
}
```

- [ ] **Step 4: 复跑 focused tests，确认 helper 语义通过**

Run:

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/runtime -run TestMissingRuntimeAssets -count=1
```

Expected:

```text
ok  	minimalist/internal/runtime	0.xxxs
```

- [ ] **Step 5: 提交这一闭环**

```bash
git add internal/runtime/runtime.go internal/runtime/runtime_test.go
git commit -m "fix: detect missing runtime assets"
```

---

### Task 2: `setup/start/restart/healthcheck/runtime-audit` 统一 fail-fast

**Files:**
- Modify: `internal/app/app.go`
- Test: `internal/app/app_test.go`
- Modify: `internal/runtime/runtime.go`
- Modify: `internal/cli/cli.go`
- Test: `internal/cli/cli_test.go`

- [ ] **Step 1: 先写 failing tests，固定高风险链路的 fail-fast 行为**

```go
func TestHealthcheckReportsMissingRuntimeAssets(t *testing.T) {
	app, _ := newTestApp(t)

	err := app.Healthcheck()
	if err == nil || !strings.Contains(err.Error(), "missing runtime assets") {
		t.Fatalf("expected missing runtime assets error, got %v", err)
	}
}

func TestStartFailsWhenRuntimeAssetsMissingWithoutSystemctlCall(t *testing.T) {
	app := newTestAppWithEnabledManualNode(t)
	var calls []commandCall
	app.Runner = fakeRunner{
		runFn: func(name string, args ...string) error {
			calls = append(calls, commandCall{name: name, args: append([]string{}, args...)})
			return nil
		},
	}

	err := app.Start()
	if err == nil || !strings.Contains(err.Error(), "missing runtime assets") {
		t.Fatalf("expected missing runtime assets error, got %v", err)
	}
	if hasRecordedCall(calls, "systemctl", "enable", "--now", "minimalist.service") {
		t.Fatalf("did not expect systemctl call when assets are missing")
	}
}

func TestRuntimeAuditReportsMissingRuntimeAssetsAsFatalGap(t *testing.T) {
	app, _ := newTestApp(t)
	app.Runner = fakeRunner{
		runFn: func(name string, args ...string) error {
			if name == "systemctl" && len(args) >= 2 {
				return nil
			}
			return nil
		},
		outputFn: func(name string, args ...string) (string, string, error) {
			if name == "journalctl" {
				return "", "", nil
			}
			return "", "", errors.New("unavailable")
		},
	}

	if err := app.RuntimeAudit(); err != nil {
		t.Fatalf("runtime audit: %v", err)
	}
	output := app.Stdout.(*bytes.Buffer).String()
	if !strings.Contains(output, "fatal-gap: runtime-assets-missing") {
		t.Fatalf("expected runtime asset fatal gap, output=\n%s", output)
	}
}
```

- [ ] **Step 2: 跑 focused tests，确认当前行为不满足要求**

Run:

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/app -run 'TestHealthcheckReportsMissingRuntimeAssets|TestStartFailsWhenRuntimeAssetsMissingWithoutSystemctlCall|TestRuntimeAuditReportsMissingRuntimeAssetsAsFatalGap' -count=1
```

Expected:

```text
--- FAIL: TestHealthcheckReportsMissingRuntimeAssets
--- FAIL: TestStartFailsWhenRuntimeAssetsMissingWithoutSystemctlCall
--- FAIL: TestRuntimeAuditReportsMissingRuntimeAssetsAsFatalGap
```

- [ ] **Step 3: 写最小实现，复用同一个 asset readiness helper**

```go
func (a *App) ensureRuntimeAssetsReady() error {
	missing := runtime.MissingRuntimeAssets(a.Paths)
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf(
		"missing runtime assets: %s; preseed them under %s",
		strings.Join(missing, ", "),
		a.Paths.RuntimeDir,
	)
}
```

同时新增一个仅供 systemd 使用的校验入口，避免宿主机 reboot 后绕过 CLI 手动路径：

```go
func (a *App) VerifyRuntimeAssets() error {
	return a.ensureRuntimeAssetsReady()
}
```

CLI 分发：

```go
case "verify-runtime-assets":
	return app.VerifyRuntimeAssets()
```

systemd unit 改成：

```ini
ExecStartPre=+%s verify-runtime-assets
ExecStartPre=+%s apply-rules
```

在以下链路里接入：

```go
func (a *App) Setup() error {
	// render files ...
	if err := a.ensureRuntimeAssetsReady(); err != nil {
		return err
	}
	// systemctl enable --now ...
}

func (a *App) Start() error {
	if err := a.RenderConfig(); err != nil {
		return err
	}
	if err := a.ensureRuntimeAssetsReady(); err != nil {
		return err
	}
	return a.Runner.Run("systemctl", "enable", "--now", "minimalist.service")
}

func (a *App) Restart() error {
	if err := a.RenderConfig(); err != nil {
		return err
	}
	if err := a.ensureRuntimeAssetsReady(); err != nil {
		return err
	}
	return a.Runner.Run("systemctl", "restart", "minimalist.service")
}

func (a *App) Healthcheck() error {
	if err := a.ensureRuntimeAssetsReady(); err != nil {
		return err
	}
	// existing controller checks ...
}
```

并把 `runtime-audit` 的 fatal gap 扩成：

```go
if err := a.ensureRuntimeAssetsReady(); err != nil {
	fatalGaps = append(fatalGaps, "runtime-assets-missing")
	fmt.Fprintf(a.Stdout, "runtime-assets: %v\n", err)
}
```

- [ ] **Step 4: 跑 focused tests 与相关回归**

Run:

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/app ./internal/cli ./internal/runtime -run 'TestHealthcheckReportsMissingRuntimeAssets|TestStartFailsWhenRuntimeAssetsMissingWithoutSystemctlCall|TestRuntimeAuditReportsMissingRuntimeAssetsAsFatalGap|TestSetupWithProvidersEnablesService|TestRestartRendersConfigAndRestartsService|TestRunDispatchesVerifyRuntimeAssetsThroughRun|TestBuildServiceUnitUsesVerifyRuntimeAssetsPreflight' -count=1
```

Expected:

```text
ok  	minimalist/internal/app	0.xxxs
```

- [ ] **Step 5: 提交这一闭环**

```bash
git add internal/app/app.go internal/app/app_test.go
git commit -m "fix: fail fast when runtime assets are missing"
```

---

### Task 3: `core-upgrade-alpha` 恢复路径可执行化

**Files:**
- Modify: `internal/app/core_upgrade.go`
- Test: `internal/app/core_upgrade_test.go`
- Modify: `README.md`
- Modify: `docs/CUTOVER.md`

- [ ] **Step 1: 先写 failing test，要求升级失败时明确告知 backup 与恢复方式**

```go
func TestCoreUpgradeAlphaRestartFailureMentionsBackupPathAndRestoreCommand(t *testing.T) {
	app, root := newTestApp(t)
	coreBin := filepath.Join(root, "bin", "mihomo-core")
	// prepare config, old core, release API, download payload...

	err := app.CoreUpgradeAlpha()
	if err == nil {
		t.Fatalf("expected restart failure")
	}
	if !strings.Contains(err.Error(), coreBin+".bak") {
		t.Fatalf("expected backup path in error, got %v", err)
	}
	if !strings.Contains(err.Error(), "mv "+coreBin+".bak "+coreBin) {
		t.Fatalf("expected restore command in error, got %v", err)
	}
}
```

- [ ] **Step 2: 跑 focused test，确认当前错误信息不够可执行**

Run:

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/app -run TestCoreUpgradeAlphaRestartFailureMentionsBackupPathAndRestoreCommand -count=1
```

Expected:

```text
--- FAIL: TestCoreUpgradeAlphaRestartFailureMentionsBackupPathAndRestoreCommand
```

- [ ] **Step 3: 最小实现，只增强错误上下文，不新增 rollback 子命令**

```go
backupPath, err := replaceCoreBinaryAtomically(cfg.Install.CoreBin, candidate)
if err != nil {
	return err
}
if err := a.restartMinimalistServiceAfterCoreUpgrade(); err != nil {
	return fmt.Errorf(
		"%w; backup preserved at %s; restore with: mv %s %s && systemctl restart minimalist.service",
		err,
		backupPath,
		backupPath,
		cfg.Install.CoreBin,
	)
}
```

- [ ] **Step 4: 更新 runbook 与 README**

在 `README.md` 增加一句：

```markdown
- `core-upgrade-alpha` 若替换成功但服务重启失败，会保留 `<core_bin>.bak` 并输出恢复命令
```

在 `docs/CUTOVER.md` 增加恢复段：

```bash
sudo mv /usr/local/bin/mihomo-core.bak /usr/local/bin/mihomo-core
sudo systemctl restart minimalist.service
sudo /usr/local/bin/minimalist healthcheck
```

- [ ] **Step 5: 跑 focused tests**

Run:

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/app -run 'TestCoreUpgradeAlphaRestartFailureMentionsBackupPathAndRestoreCommand|TestCoreUpgradeAlphaSurfacesRestartFailureWithLogs' -count=1
```

Expected:

```text
ok  	minimalist/internal/app	0.xxxs
```

- [ ] **Step 6: 提交这一闭环**

```bash
git add internal/app/core_upgrade.go internal/app/core_upgrade_test.go README.md docs/CUTOVER.md
git commit -m "fix: document recoverable core upgrade failures"
```

---

### Task 4: Restart / Reboot Smoke Runbook 与主线同步

**Files:**
- Modify: `docs/CUTOVER.md`
- Modify: `docs/README_FLOWS.md`
- Modify: `docs/STATUS.md`
- Modify: `docs/NEXT_STEP.md`

- [ ] **Step 1: 写出 restart / reboot smoke runbook**

在 `docs/CUTOVER.md` 增加一节：

```markdown
## Post-cutover restart / reboot smoke

### service restart smoke

```bash
sudo systemctl restart minimalist.service
systemctl is-active minimalist.service
systemctl is-enabled minimalist.service
/usr/local/bin/minimalist healthcheck
/usr/local/bin/minimalist runtime-audit
ip rule show
ip route show table 233
iptables -t mangle -S | grep MIHOMO_PRE
iptables -t nat -S | grep MIHOMO_DNS
```

### host reboot smoke

```bash
sudo reboot
# reconnect after boot
systemctl is-active minimalist.service
systemctl is-enabled minimalist.service
/usr/local/bin/minimalist healthcheck
/usr/local/bin/minimalist runtime-audit
ip rule show
ip route show table 233
```
```

- [ ] **Step 2: 同步主线文档**

更新：

- `docs/README_FLOWS.md`：说明 restart/reboot smoke 是当前长期稳定主线的一部分
- `docs/STATUS.md`：记录 runbook 已落地，但 reboot smoke 是否真实执行要与结果分开写
- `docs/NEXT_STEP.md`：把“下一闭环”更新成“执行并验证 restart/reboot smoke”

- [ ] **Step 3: review 文档 diff**

Run:

```bash
git diff -- docs/CUTOVER.md docs/README_FLOWS.md docs/STATUS.md docs/NEXT_STEP.md
```

Expected:

```text
只包含 restart/reboot smoke runbook 与主线状态同步，没有无关历史回抄
```

- [ ] **Step 4: 提交这一闭环**

```bash
git add docs/CUTOVER.md docs/README_FLOWS.md docs/STATUS.md docs/NEXT_STEP.md
git commit -m "docs: add restart and reboot smoke runbook"
```

---

### Task 5: 最终回归与收尾

**Files:**
- Verify only: `internal/runtime/runtime.go`, `internal/runtime/runtime_test.go`, `internal/app/app.go`, `internal/app/app_test.go`, `internal/app/core_upgrade.go`, `internal/app/core_upgrade_test.go`, `internal/cli/cli_test.go`, `README.md`, `docs/CUTOVER.md`, `docs/README_FLOWS.md`, `docs/STATUS.md`, `docs/NEXT_STEP.md`

- [ ] **Step 1: 跑全量回归**

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./...
```

Expected:

```text
ok  	minimalist/internal/app
ok  	minimalist/internal/runtime
```

并且没有失败包。

- [ ] **Step 2: 跑 build**

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go build -o /tmp/minimalist-build-check ./cmd/minimalist
```

Expected:

```text
exit 0
```

- [ ] **Step 3: 如本机环境允许，执行最小实机 smoke**

```bash
/usr/local/bin/minimalist healthcheck
/usr/local/bin/minimalist runtime-audit
systemctl is-active minimalist.service
systemctl is-enabled minimalist.service
ip rule show
ip route show table 233
```

Expected:

```text
healthcheck 成功
runtime-audit 输出 alerts-24h / alerts-recent / fatal-gaps
minimalist.service active
table 233 与 fwmark 规则存在
```

- [ ] **Step 4: 提交最终文档同步**

```bash
git add README.md docs/CUTOVER.md docs/README_FLOWS.md docs/STATUS.md docs/NEXT_STEP.md
git commit -m "docs: sync long-term stability hardening status"
```

---

## AUTOPLAN REVIEW REPORT

### Phase 1 — CEO Review

**Plan summary**

这份计划的方向是对的：已经从“继续补 coverage”切到“长期稳定值守”。真正的问题是它原始版本只覆盖了**人工命令链路**，没有完全覆盖**宿主机 reboot 后的 systemd 启动链路**。如果只在 `setup/start/restart/healthcheck` 里做 asset fail-fast，那么人在场时更安全，机器自己重启时还是会绕过校验。

**Auto-decisions**

- 已自动接受一个范围内扩展：把 runtime asset 校验延伸到 systemd `ExecStartPre`
- 未接受更大范围扩展：不恢复 stable 通道、不新增自动下载、不新增自动 rollback 命令

**Premise challenge**

- 前提 1：长期稳定的主要缺口是 asset 缺失、重启后恢复、升级后恢复。这个前提成立。
- 前提 2：只靠 runbook 就能覆盖 reboot 场景。这个前提不成立，必须有代码级 preflight。
- 前提 3：现在不需要恢复 stable 通道也能推进长期稳定。当前阶段成立，因为最急的是 fail-fast 和恢复路径，不是产品边界扩张。

**What already exists**

- `runtime.Paths` 已有 `UIPath()`、`RuntimeDir` 等路径原语，可直接承载 asset 缺口判定。
- `BuildServiceUnit()` 已集中生成 systemd unit，是接入 `verify-runtime-assets` 的正确位置。
- `Setup/Start/Restart/Healthcheck/RuntimeAudit` 已集中在 `internal/app/app.go`，当前 blast radius 小且边界清楚。
- `core-upgrade-alpha` 已有 backup 保留语义，可在现有错误路径上补恢复提示，无需重做升级流程。

**NOT in scope**

- stable 通道、自动定时升级、rollback 子命令：当前主线不做，原因是会扩大产品边界。
- 自动下载 runtime assets：当前主线不做，原因是长期稳定优先于联网自愈的不确定性。
- 旧状态迁移与旧服务回滚恢复：当前主线不做，原因是旧资产已清理且不再是 live 真相。

**Dream state delta**

```text
CURRENT
  人工切换已成功，当前主机可用
      |
      v
THIS PLAN
  补齐 asset 校验、重启/重启后 smoke、升级失败恢复提示
      |
      v
12-MONTH IDEAL
  Debian NAS 旁路由在 reboot / service restart / core maintenance 后都能可预期恢复，
  所有失败都能 fail-fast、可观测、可执行恢复
```

### Phase 2 — Design Review

Skipped. No UI scope detected in this plan. 我检查了计划内容，涉及的是 CLI、systemd、runtime assets、runbook 和运维输出契约，没有新的界面、屏幕、交互流或设计系统变更。

### Phase 3 — Eng Review

**Architecture review**

系统边界清楚，但原始计划少了一条关键启动路径。完整依赖图应是：

```text
config/state
    |
    v
runtime.Paths ----> MissingRuntimeAssets()
    |                      |
    |                      v
    |                VerifyRuntimeAssets()
    |                      |
    v                      v
BuildServiceUnit() --> ExecStartPre verify-runtime-assets
    |                      |
    v                      v
Setup/Start/Restart/Healthcheck/RuntimeAudit
    |
    v
systemctl / mihomo-core / route state
```

如果不把 `VerifyRuntimeAssets()` 放到 systemd preflight，reboot 路径就会绕过人工命令链路，这是本次最高优先级架构缺口。已自动并入计划。

**Error & Rescue Registry**

| Method / Codepath | Failure mode | Rescued? | Action | User sees |
|---|---|---:|---|---|
| `MissingRuntimeAssets` | mmdb/geosite/ui 缺失 | Y | 返回缺口列表 | 缺少哪些 assets |
| `VerifyRuntimeAssets` | 缺少 runtime assets | Y | fail-fast | 明确缺失项和预置目录 |
| `Setup` | assets 缺失但准备启服务 | Y | 停在 systemctl 前 | 不会误启服务 |
| `Start` / `Restart` | assets 缺失 | Y | 停在 systemctl 前 | 不会误启服务 |
| `Healthcheck` | assets 缺失 | Y | 直接返回错误 | 明确不是 controller 问题 |
| `RuntimeAudit` | assets 缺失 | Y | 记为 `fatal-gap` | 可区分历史噪音与致命缺口 |
| `CoreUpgradeAlpha` | 替换成功但重启失败 | Y | 保留 backup，输出恢复命令 | 可执行恢复路径 |

**Failure Modes Registry**

| Codepath | Failure mode | Rescued? | Test? | User sees? | Logged? |
|---|---|---:|---:|---:|---:|
| systemd boot | assets 缺失 | Y | planned | yes | via systemd stderr |
| runtime-audit | controller 不可达 | Y | yes | yes | partial |
| runtime-audit | assets 缺失 | Y | planned | yes | yes |
| core-upgrade-alpha | restart failed after replace | Y | planned | yes | yes |
| reboot smoke | route rules missing after boot | N | doc-only | operator-runbook | manual |

唯一还没变成代码级保障的是“reboot 后路由状态缺失”的自动检测，它仍在 runbook 层，不在当前闭环里再扩范围。

**Test diagram**

```text
NEW CODEPATHS
- MissingRuntimeAssets(): assets 完整 / 缺失 / 空文件 / 目录误占位
- VerifyRuntimeAssets(): pass / fail-fast
- BuildServiceUnit(): verify-runtime-assets preflight 注入
- Setup(): assets ready vs missing
- Start/Restart(): assets ready vs missing
- Healthcheck(): controller reachable vs assets missing
- RuntimeAudit(): history / recent / fatal-gap(runtime-assets-missing)
- CoreUpgradeAlpha(): restart failure with backup restore guidance

NEW OPERATOR FLOWS
- first boot after install
- service restart after config render
- host reboot after cutover
- failed core upgrade recovery
```

**Performance review**

这份计划性能风险很低。新增逻辑主要是几个 `os.Stat` / `os.ReadFile` 级别的本地文件检查，调用频率低，远低于 controller 请求和 route 编排开销。最大的工程收益是“用极小代价换掉 reboot 路径的不确定性”。

### Phase 3.5 — DX Review

DX scope detected. 这份计划虽然不是给外部开发者的 SDK，但它明确改动 CLI 契约、错误信息、README 和运维 runbook，因此需要做 DX 审视。

**Developer / operator journey map**

| Stage | Current | Planned improvement |
|---|---|---|
| discover | 需要翻 README 和 CUTOVER | 在 README 明确 asset 与恢复路径 |
| install | 可执行 | 不变 |
| first start | 依赖人工记住预置 assets | fail-fast 提示缺失项 |
| health check | 可能把 asset 问题看成 controller 问题 | 明确区分 |
| runtime audit | 已区分 history/recent/fatal | 继续保留 |
| restart | 当前缺少 asset 统一前置校验 | 加入 preflight |
| reboot | 依赖人工 smoke | 补固定 runbook |
| upgrade | 有 backup 但恢复路径不够可执行 | 输出 restore 命令 |
| incident recovery | 需要靠记忆拼命令 | 文档化固定步骤 |

**DX scorecard**

| Dimension | Score / 10 | Notes |
|---|---:|---|
| Getting started | 6 | 有 README，但重启与 asset 依赖仍偏隐式 |
| Error messages | 8 | 当前主线已明显改善，计划会继续补强 |
| CLI ergonomics | 7 | 需要一个隐藏 preflight 命令服务 systemd |
| Docs clarity | 7 | reboot / recovery 还需再明确 |
| Recovery confidence | 5 | 是本次计划重点提升项 |
| Upgrade safety | 6 | 有 backup，缺恢复命令与验证口径 |
| Operational discoverability | 7 | `runtime-audit` 已改善 |
| Long-term maintainability | 8 | blast radius 小，边界清楚 |

**DX implementation checklist**

- [ ] README 写明 runtime assets 缺失的 fail-fast 语义
- [ ] runbook 写明 restart / reboot smoke 步骤
- [ ] upgrade 失败时输出可复制恢复命令
- [ ] CLI 隐藏 preflight 命令只做一件事，名字直白

### Cross-Phase Themes

- **同一个高置信主题出现两次：不能只修人工命令链路，必须覆盖 systemd 自启动链路。**
- CEO 视角把它看成“长期稳定定义不完整”，Eng 视角把它看成“reboot 路径绕过 preflight”。
- 这是本次自动评审里最重要的修改点。

### Artifacts

- Test plan artifact: `/tmp/minimalist-long-term-stability-test-plan-20260428.md`
- Restore point: `/tmp/main-autoplan-restore-20260428-174048.md`
- Outside voices: skipped in this run; no writable `~/.gstack/projects` artifact root, no authorized subagent dispatch in current session

## Decision Audit Trail

| # | Phase | Decision | Classification | Principle | Rationale | Rejected |
|---|---|---|---|---|---|---|
| 1 | CEO | 接受“长期稳定主线”前提 | mechanical | P6 | 当前文档与实机状态一致，主线方向正确 | 继续补 coverage |
| 2 | CEO | 将 runtime asset 校验扩到 systemd `ExecStartPre` | mechanical | P1+P2 | 这是 reboot 稳定性的必需项，且在 blast radius 内 | 只修手动命令链路 |
| 3 | CEO | 不恢复 stable 通道 / rollback 子命令 | mechanical | P3+P4 | 当前阶段先收口恢复路径，不扩大产品边界 | 扩能力 |
| 4 | ENG | 接受隐藏 `verify-runtime-assets` 命令 | mechanical | P5 | 显式、直白、可被 systemd 复用 | shell 片段式校验 |
| 5 | ENG | 保持 reboot smoke 先以 runbook 落地 | taste | P3 | 先拿到可重复验证步骤，比立即扩成更大自动化更稳 | 继续扩成自动化守护 |
| 6 | DX | 强制 upgrade 失败输出恢复命令 | mechanical | P1 | 对 operator 最直接，降低事故恢复认知负担 | 只写日志，不给命令 |
