<!-- /autoplan restore point: /tmp/main-autoplan-restore-20260501-180033.md -->
# 菜单 + CLI 交互体验重设计 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 minimalist 的菜单 + CLI 交互体验从"无语""不人性化"提升到 233boy/sing-box-yes 那种成熟脚本的水平：短入口双轨、操作后回菜单、看+操作合并、顶部 status header、删除二次确认、host-proxy 一键开关、日志独立命令、顶层导航重排为 5 类。

**Architecture:** Approach A 路线（最小修补 + 短别名双轨）。沿用现有 `internal/app` 业务层（Status/ListNodes/SetNodeEnabled 等不动），只改外层 menu/CLI 呈现 + 加几个新薄壳命令（host-proxy / log）。不引入 TUI 库。

**Tech Stack:** Go 1.24 标准库 (`os`, `os/exec`, `path/filepath`, `bufio`, `strings`)；现有 `internal/app`、`internal/cli`、`internal/config`、`internal/runtime`、`internal/system`；`journalctl`（外部命令，通过 `internal/system.Runner` 调用）。

> **AUTOPLAN STATUS — SUPERSEDED**
>
> 本文档最上方这份 `/autoplan` v2 才是当前可执行计划。
> 下方原始 `Phase 1-4` 草稿保留作历史参考，**不要直接按旧草稿开工**。

---

## AUTOPLAN V2 — Execute This

### Revised Goal

不要再追“像 233boy 一样顺手”这种 shell UX parity。

这一轮真正目标只有 3 个：

1. 把**状态与诊断**做成单独、稳定、始终可达的入口
2. 把 **`host-proxy`** 做成**事务性**操作，失败时不留下脏状态
3. 把 **`log`** 先做成安全可用的 **snapshot CLI**，不碰流式 follow

成功标准不是“菜单更花”，而是：
- 日常 5 个高频动作更短
- 故障时更快定位
- 高风险动作更难误触
- 文档和 CLI 口径一致

### Revised Scope

本轮 **只做**：

- 独立“状态与诊断”面
  - `status`
  - `healthcheck`
  - `runtime-audit`
- 顶部 `statusSnapshot` header
  - 只读本地 config/state/systemctl
  - 不打 controller HTTP
- `host-proxy status|enable|disable`
  - 事务性 service method
  - 失败回滚 config truth
- `log`
  - 默认最近 N 行
  - `--lines`
  - `--errors`
  - `mihomo`
- 文档单一真相
  - 选一页 operator flow 作 canonical

本轮 **不做**：

- `log -f`
- 5 类顶层导航重排
- `configMenu/rulesMenu/logMenu/controlMenu` 全量重构
- TUI 重写
- CLI all-in
- REPL

### Revised Architecture

```text
CLI/menu
  -> thin input/render layer
      -> statusSnapshot()         # cheap local read path
      -> diagnosticsMenu()        # status/healthcheck/runtime-audit
      -> hostProxyService()       # transactional toggle
      -> logSnapshotService()     # bounded journalctl reads

NO:
  - controller HTTP in header
  - Runner.Output() for streaming follow
  - save -> render -> apply direct host-proxy mutation
```

### Revised File Map

Likely new files:
- `internal/app/header.go`
- `internal/app/header_test.go`
- `internal/app/host_proxy.go`
- `internal/app/host_proxy_test.go`
- `internal/app/logs.go`
- `internal/app/logs_test.go`
- `internal/app/prompts.go`

Likely touched files:
- `internal/app/app.go`
- `internal/app/app_test.go`
- `internal/cli/cli.go`
- `internal/cli/cli_test.go`
- `README.md`
- `docs/README_FLOWS.md`
- `docs/STATUS.md`

Must not expand unless separately planned:
- `internal/system/system.go` for streaming
- full menu taxonomy rewrite
- TUI libraries

### Revised Phase 0 — First Lock the Contracts

- [ ] 写清 header 状态枚举
  - service: `running|stopped|unknown`
  - nodes: `none|partial|ready`
  - host-proxy: `off|on|unknown`
- [ ] 写清错误 contract
  - problem
  - likely cause
  - next command
  - doc path
- [ ] 指定 canonical operator doc
  - README 只保留 quickstart
  - 详细 operator flow 只留一页

### Revised Phase 1 — Diagnostics First

#### Task 1.1: `statusSnapshot()` + header

- [ ] 新增纯本地 snapshot helper
- [ ] 禁止 header 走 controller HTTP
- [ ] 新增 focused tests:
  - controller down 时 header 仍快速返回
  - unknown state 不伪装成 stopped
  - 0 个启用手动节点显示 `none`

#### Task 1.2: 独立 diagnostics surface

- [ ] 保留一个明确入口，不让 header 替代诊断
- [ ] 入口至少包含：
  - `status`
  - `healthcheck`
  - `runtime-audit`
- [ ] 菜单文案用“状态与诊断”或“诊断”，不要只叫“日志”

### Revised Phase 2 — Transactional Host Proxy

#### Task 2.1: `SetHostProxy(enabled bool)`

- [ ] 不再用 `save -> render -> apply` 裸串联
- [ ] 先做 preflight
  - cutover ready
  - enabled manual nodes present
- [ ] 失败回滚配置真相
- [ ] 成功后给明确验证输出

#### Task 2.2: CLI surface

- [ ] 用统一动词体系：
  - `host-proxy status`
  - `host-proxy enable`
  - `host-proxy disable`
- [ ] `status` 只读
- [ ] `enable/disable` 默认带确认
- [ ] 文案必须包含回滚提示

#### Task 2.3: Required tests

- [ ] `RenderConfig` 失败回滚
- [ ] `ApplyRules` 失败回滚
- [ ] `ensureCutoverReady` 失败时不改配置
- [ ] 无启用手动节点时不改配置

### Revised Phase 3 — Safe Log Snapshot

#### Task 3.1: Scope reduction

- [ ] 删除首轮 `-f` 设计
- [ ] 不扩 `internal/system` streaming API

#### Task 3.2: CLI contract

- [ ] `log`
- [ ] `log --lines 50`
- [ ] `log --errors`
- [ ] `log mihomo`
- [ ] 可选：`log --since "15 minutes ago"`

#### Task 3.3: Required tests

- [ ] unknown arg
- [ ] missing `journalctl`
- [ ] timeout
- [ ] line count honored
- [ ] `mihomo` target path

### Revised Phase 4 — Menu Loop Safety

- [ ] 抽 `readChoice()`
- [ ] 显式处理 `io.EOF`
- [ ] 子菜单共享同一个 `*bufio.Reader`
- [ ] “看完留在子菜单”只在少数高频菜单先落地
  - `nodes`
  - `diagnostics`
- [ ] 不在本轮重排全部 8 个旧菜单

### Revised Execution Order

1. `statusSnapshot()` + diagnostics 面
2. `host-proxy` 事务性 service
3. `log` snapshot CLI
4. menu loop EOF-safe
5. docs consolidation

### Revised Success Criteria

- 用户能在 2 步内到达 `status/healthcheck/runtime-audit`
- `host-proxy enable` 失败后 config truth 不漂移
- `log --lines N`、`log --errors`、`log mihomo` 可用
- README / operator flow / helptext 不再三套口径
- 菜单首屏即使 controller 不通也不会卡到 30 秒

---

## Pre-Plan State

- 主分支 `main` 与 `origin/main` 同步
- 服务 `minimalist.service` `active/enabled`，`runtime-audit fatal-gaps=0`
- AGENTS.md 阶段已切到"拆解阶段"（commit `04cba7e`）
- TODOS.md 已含完整 spec（9 项参考实现对照表 + 5 类 mental model + 3 项功能缺口）
- `proxy_host_output` 字段已存在于 `internal/config/config.go:39`，默认 `false`，由 `routerWizardWithReader` 在 `internal/app/app.go:454` 通过 promptBool 切换

## File Structure

新增文件：
- `internal/app/prompts.go` — 抽出现有 `promptString/promptIndex/promptBool` + 新增 `promptConfirm` helper（统一放一处便于复用）
- `internal/app/host_proxy.go` — `HostProxyStatus/HostProxyEnable/HostProxyDisable` 三个新方法
- `internal/app/host_proxy_test.go`
- `internal/app/logs.go` — `Logs` 方法，封装 `journalctl`
- `internal/app/logs_test.go`
- `internal/app/header.go` — `renderStatusHeader` 方法
- `internal/app/header_test.go`

修改文件：
- `internal/app/app.go` — `InstallSelf` 加 m symlink；`Menu` 顶层重排为 5 类；移除 case "1": return 退出行为；删除类菜单项接 `promptConfirm`；移除 `Status()` 重复（成为 `renderStatusHeader` 的展开版）；prompt helpers 移到 prompts.go
- `internal/cli/cli.go` — 新增 `host-proxy` / `log` 子命令分发；helptext 更新
- `internal/cli/cli_test.go` — 新子命令的 dispatch 测试

NOT modifying（业务层不动，避免触动 91-100% 覆盖率核心）：
- `internal/config/*` — `ProxyHostOutput` 字段已就位
- `internal/state/*`
- `internal/provider/*`
- `internal/rulesrepo/*`
- `internal/runtime/*` — `Paths` 不动；`BuildServiceUnit` 不动
- `internal/system/*` — `Runner.Run/Output` 直接复用
- `cmd/minimalist/main.go`

---

## Phase 1: 零成本起步 — `m` symlink

**目的**：让 `m menu` / `m status` / `m nodes list` 立即可用。改一处 `InstallSelf`，加一个测试。

### Task 1.1: InstallSelf 创建 `m` symlink

**Files:**
- Modify: `internal/app/app.go:82-111` (`InstallSelf` 函数)
- Test: `internal/app/app_test.go` (追加)

- [ ] **Step 1: Write the failing test**

追加到 `internal/app/app_test.go` 末尾：

```go
func TestInstallSelfCreatesShortAliasSymlink(t *testing.T) {
	app, _ := newTestApp(t)
	if err := app.InstallSelf(); err != nil {
		t.Fatalf("install self: %v", err)
	}
	aliasPath := filepath.Join(filepath.Dir(app.Paths.BinPath), "m")
	info, err := os.Lstat(aliasPath)
	if err != nil {
		t.Fatalf("expected short alias symlink at %s: %v", aliasPath, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected %s to be a symlink, got mode %v", aliasPath, info.Mode())
	}
	target, err := os.Readlink(aliasPath)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if target != app.Paths.BinPath {
		t.Fatalf("symlink target = %q, want %q", target, app.Paths.BinPath)
	}
}

func TestInstallSelfReplacesExistingSymlink(t *testing.T) {
	app, _ := newTestApp(t)
	aliasPath := filepath.Join(filepath.Dir(app.Paths.BinPath), "m")
	if err := os.MkdirAll(filepath.Dir(aliasPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Symlink("/nonexistent/old-target", aliasPath); err != nil {
		t.Fatalf("seed symlink: %v", err)
	}
	if err := app.InstallSelf(); err != nil {
		t.Fatalf("install self: %v", err)
	}
	target, err := os.Readlink(aliasPath)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if target != app.Paths.BinPath {
		t.Fatalf("symlink should be replaced; got target=%q", target)
	}
}

func TestInstallSelfSkipsShortAliasIfNonSymlinkExists(t *testing.T) {
	app, _ := newTestApp(t)
	aliasPath := filepath.Join(filepath.Dir(app.Paths.BinPath), "m")
	if err := os.MkdirAll(filepath.Dir(aliasPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(aliasPath, []byte("real binary"), 0o755); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	if err := app.InstallSelf(); err != nil {
		t.Fatalf("install self should not fail when alias is real file: %v", err)
	}
	data, err := os.ReadFile(aliasPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "real binary" {
		t.Fatalf("alias should not be overwritten; got %q", data)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/app -run 'TestInstallSelfCreates|TestInstallSelfReplaces|TestInstallSelfSkipsShort' -v
```

Expected: FAIL — symlink doesn't exist after `InstallSelf()`.

- [ ] **Step 3: Implement minimal code**

修改 `internal/app/app.go:82-111`，在 `fmt.Fprintf(a.Stdout, "已安装 minimalist...")` 前插入：

```go
	// 创建 m 短别名 symlink（idempotent，幂等）
	aliasPath := filepath.Join(filepath.Dir(a.Paths.BinPath), "m")
	if info, err := os.Lstat(aliasPath); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			// 已存在的 symlink 可以安全替换
			if err := os.Remove(aliasPath); err != nil {
				fmt.Fprintf(a.Stderr, "警告: 移除旧 m symlink 失败: %v\n", err)
			} else if err := os.Symlink(a.Paths.BinPath, aliasPath); err != nil {
				fmt.Fprintf(a.Stderr, "警告: 创建 m symlink 失败: %v\n", err)
			} else {
				fmt.Fprintf(a.Stdout, "已更新短别名 %s -> %s\n", aliasPath, a.Paths.BinPath)
			}
		} else {
			// 非 symlink（真实文件 / 目录） — 不覆盖
			fmt.Fprintf(a.Stderr, "警告: %s 已存在且不是 symlink，跳过短别名创建\n", aliasPath)
		}
	} else {
		// 不存在，直接创建
		if err := os.Symlink(a.Paths.BinPath, aliasPath); err != nil {
			fmt.Fprintf(a.Stderr, "警告: 创建 m symlink 失败: %v\n", err)
		} else {
			fmt.Fprintf(a.Stdout, "已创建短别名 %s -> %s\n", aliasPath, a.Paths.BinPath)
		}
	}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/app -run 'TestInstallSelfCreates|TestInstallSelfReplaces|TestInstallSelfSkipsShort' -v
```

Expected: PASS, all three.

- [ ] **Step 5: Run full suite + vet + fmt**

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./...
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go vet ./...
gofmt -l cmd internal
```

Expected: all PASS, fmt 输出空。

- [ ] **Step 6: Commit**

```bash
git add internal/app/app.go internal/app/app_test.go
git commit -m "$(cat <<'EOF'
feat(install): create m short-alias symlink in InstallSelf

InstallSelf 现在会在 BinPath 旁边创建 m -> minimalist 的 symlink，让
"敲 minimalist menu"（11 字符）变成 "敲 m menu"（6 字符）。

幂等性：
- 已存在的 symlink 替换为新 target（防止旧位置 stale）
- 已存在的非 symlink（真实文件/目录）跳过并 warn，不覆盖
- 不存在直接创建

3 条 focused tests 覆盖三种状态。
EOF
)"
```

### Task 1.2: Phase 1 实机验证 + push

- [ ] **Step 1: 重新 install-self（root）**

```bash
sudo go run ./cmd/minimalist install-self
```

Expected stdout: 包含 `已创建短别名 /usr/local/bin/m -> /usr/local/bin/minimalist`（首次）或 `已更新短别名 ...`（重装）。

- [ ] **Step 2: smoke**

```bash
ls -la /usr/local/bin/m /usr/local/bin/minimalist
m --help | head -3
m status
```

Expected: `m` 是 symlink 指向 `minimalist`；`m --help` 输出与 `minimalist --help` 一致；`m status` 跑通。

- [ ] **Step 3: 服务无影响验证**

```bash
systemctl is-active minimalist.service
systemctl is-enabled minimalist.service
```

Expected: `active` / `enabled`（symlink 创建不影响服务）。

- [ ] **Step 4: push**

```bash
git push origin main
```

---

## Phase 2: 高 ROI 三件套

**目的**：顶部 status header + 操作后回菜单 + promptConfirm helper（删除类二次确认）。这三件加一起是 233boy 模式的核心，覆盖 80% 的 UX 痛点。

### Task 2.1: 抽出 prompt helpers 到 `internal/app/prompts.go`

**Files:**
- Create: `internal/app/prompts.go`
- Modify: `internal/app/app.go:1745-1778` (移除 promptString/promptIndex/promptBool 的实现)
- Test: 现有调用应该继续通过

- [ ] **Step 1: 创建 `internal/app/prompts.go`**

```go
package app

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// promptString prompts for a string with a default value displayed in brackets.
// Empty input returns the current value.
func promptString(reader *bufio.Reader, out io.Writer, label, current string) string {
	fmt.Fprintf(out, "%s [%s]: ", label, current)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return current
	}
	return line
}

// promptIndex prompts for a positive integer index. Returns error on parse failure or non-positive.
func promptIndex(reader *bufio.Reader, out io.Writer, label string) (int, error) {
	fmt.Fprintf(out, "%s: ", label)
	line, _ := reader.ReadString('\n')
	value := strings.TrimSpace(line)
	index, err := strconv.Atoi(value)
	if err != nil || index <= 0 {
		return 0, fmt.Errorf("invalid %s: %q", label, value)
	}
	return index, nil
}

// promptBool prompts for a 0/1 toggle with current value shown.
func promptBool(reader *bufio.Reader, out io.Writer, label string, current bool) bool {
	currentValue := "0"
	if current {
		currentValue = "1"
	}
	fmt.Fprintf(out, "%s [0/1][%s]: ", label, currentValue)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return current
	}
	return line == "1"
}
```

- [ ] **Step 2: 从 `app.go` 移除三个 prompt 函数**

删除 `internal/app/app.go:1745-1778` 全部 18 行。

- [ ] **Step 3: 验证编译 + 现有测试通过**

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/app/... -count=1
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go vet ./...
gofmt -l cmd internal
```

Expected: PASS（纯文件移动，不改语义）。

- [ ] **Step 4: Commit**

```bash
git add internal/app/prompts.go internal/app/app.go
git commit -m "refactor(app): extract prompt helpers to prompts.go"
```

### Task 2.2: 新增 `promptConfirm` helper

**Files:**
- Modify: `internal/app/prompts.go`
- Create: `internal/app/prompts_test.go`

- [ ] **Step 1: Write failing test (`internal/app/prompts_test.go`)**

```go
package app

import (
	"bufio"
	"strings"
	"testing"
)

func TestPromptConfirmDefaultsToFalseOnEmpty(t *testing.T) {
	var out strings.Builder
	reader := bufio.NewReader(strings.NewReader("\n"))
	if promptConfirm(reader, &out, "确认删除", false) {
		t.Fatalf("expected default false on empty input")
	}
	if !strings.Contains(out.String(), "[y/N]") {
		t.Fatalf("expected [y/N] suffix when default is false; got: %q", out.String())
	}
}

func TestPromptConfirmDefaultsToTrueOnEmpty(t *testing.T) {
	var out strings.Builder
	reader := bufio.NewReader(strings.NewReader("\n"))
	if !promptConfirm(reader, &out, "确认", true) {
		t.Fatalf("expected default true on empty input")
	}
	if !strings.Contains(out.String(), "[Y/n]") {
		t.Fatalf("expected [Y/n] suffix when default is true; got: %q", out.String())
	}
}

func TestPromptConfirmAcceptsYesVariants(t *testing.T) {
	for _, in := range []string{"y", "Y", "yes", "YES", "Yes"} {
		var out strings.Builder
		reader := bufio.NewReader(strings.NewReader(in + "\n"))
		if !promptConfirm(reader, &out, "确认", false) {
			t.Errorf("input %q should return true", in)
		}
	}
}

func TestPromptConfirmRejectsNonYes(t *testing.T) {
	for _, in := range []string{"n", "no", "N", "0", "x", " ", "delete"} {
		var out strings.Builder
		reader := bufio.NewReader(strings.NewReader(in + "\n"))
		if promptConfirm(reader, &out, "确认", false) {
			t.Errorf("input %q should return false", in)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/app -run TestPromptConfirm -v
```

Expected: FAIL — `promptConfirm` undefined.

- [ ] **Step 3: Implement promptConfirm in `internal/app/prompts.go`**

追加到 `prompts.go`：

```go
// promptConfirm asks for y/n confirmation.
// Empty input returns defaultYes.
// Accepts y/yes (case-insensitive) as true; everything else is false.
// Suffix [y/N] when defaultYes is false; [Y/n] when defaultYes is true.
func promptConfirm(reader *bufio.Reader, out io.Writer, label string, defaultYes bool) bool {
	suffix := "[y/N]"
	if defaultYes {
		suffix = "[Y/n]"
	}
	fmt.Fprintf(out, "%s %s: ", label, suffix)
	line, _ := reader.ReadString('\n')
	answer := strings.ToLower(strings.TrimSpace(line))
	if answer == "" {
		return defaultYes
	}
	return answer == "y" || answer == "yes"
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/app -run TestPromptConfirm -v
```

Expected: 4/4 PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/app/prompts.go internal/app/prompts_test.go
git commit -m "feat(app): add promptConfirm helper for y/n confirmation"
```

### Task 2.3: 实现 `renderStatusHeader`

**Files:**
- Create: `internal/app/header.go`
- Create: `internal/app/header_test.go`

- [ ] **Step 1: Write failing test (`internal/app/header_test.go`)**

```go
package app

import (
	"strings"
	"testing"
)

func TestRenderStatusHeaderIncludesServiceLabel(t *testing.T) {
	app, _ := newTestApp(t)
	header := app.renderStatusHeader()
	if !strings.Contains(header, "服务:") {
		t.Errorf("header missing 服务: prefix; got: %q", header)
	}
	if !strings.Contains(header, "节点:") {
		t.Errorf("header missing 节点: prefix; got: %q", header)
	}
	if !strings.Contains(header, "宿主机:") {
		t.Errorf("header missing 宿主机: prefix; got: %q", header)
	}
}

func TestRenderStatusHeaderHighlightsHostProxyOn(t *testing.T) {
	app, _ := newTestApp(t)
	cfg, err := config.Ensure(app.Paths.ConfigPath())
	if err != nil {
		t.Fatalf("ensure config: %v", err)
	}
	cfg.Network.ProxyHostOutput = true
	if err := config.Save(app.Paths.ConfigPath(), cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	header := app.renderStatusHeader()
	if !strings.Contains(header, "★已接管★") {
		t.Errorf("header should highlight host-proxy when ON; got: %q", header)
	}
}

func TestRenderStatusHeaderShowsHostProxyOff(t *testing.T) {
	app, _ := newTestApp(t)
	header := app.renderStatusHeader()
	if !strings.Contains(header, "宿主机: 关闭") {
		t.Errorf("header should show 宿主机: 关闭 by default; got: %q", header)
	}
}
```

注意：测试需要 `import "minimalist/internal/config"`。如果 `config.Save` 不存在，先用 `cfg.Network.ProxyHostOutput = true` 然后 `config.Ensure` 再读出来不会工作——需要写回。检查 `internal/config` 是否有 `Save` 函数：

```bash
grep -n "^func.*Save\|^func Save" internal/config/*.go
```

如果没有 `Save`，第二个测试改用直接构造 `App` 但用预填的 config 文件方式（写 yaml）。

- [ ] **Step 2: Run test to verify it fails**

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/app -run TestRenderStatusHeader -v
```

Expected: FAIL — `renderStatusHeader` undefined.

- [ ] **Step 3: Implement `internal/app/header.go`**

```go
package app

import (
	"fmt"
)

// renderStatusHeader returns a compact one-line summary for menu top.
// Format: === minimalist | 服务: 运行中 | 节点: 4 启用 / 4 总计 | 宿主机: 关闭 ===
func (a *App) renderStatusHeader() string {
	cfg, st, err := a.ensureAll()
	if err != nil {
		return fmt.Sprintf("=== minimalist (config error: %v) ===\n", err)
	}
	serviceActive := commandOK(a.Runner.Run("systemctl", "is-active", "--quiet", "minimalist.service"))
	serviceLabel := "已停止"
	if serviceActive {
		serviceLabel = "运行中"
	}
	totalNodes := a.manualNodeCount(st)
	enabledNodes := 0
	for _, n := range st.Nodes {
		if n.Enabled {
			enabledNodes++
		}
	}
	hostProxyLabel := "关闭"
	if cfg.Network.ProxyHostOutput {
		hostProxyLabel = "★已接管★"
	}
	return fmt.Sprintf("=== minimalist | 服务: %s | 节点: %d 启用 / %d 总计 | 宿主机: %s ===\n",
		serviceLabel, enabledNodes, totalNodes, hostProxyLabel)
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/app -run TestRenderStatusHeader -v
```

Expected: 3/3 PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/app/header.go internal/app/header_test.go
git commit -m "feat(app): add renderStatusHeader for menu top"
```

### Task 2.4: 把 status header 接入 `Menu()` 并移除"状态总览"作为菜单项

**Files:**
- Modify: `internal/app/app.go:472-514` (`Menu` 函数)
- Modify: `internal/app/app_test.go` (现有 menu 测试可能需要调整)

- [ ] **Step 1: Write failing test**

追加到 `internal/app/app_test.go`：

```go
func TestMenuRendersStatusHeaderEachLoop(t *testing.T) {
	app, _ := newTestApp(t)
	app.Stdin = strings.NewReader("0\n") // 立即退出
	if err := app.Menu(); err != nil {
		t.Fatalf("menu: %v", err)
	}
	out := app.Stdout.(*bytes.Buffer).String()
	if !strings.Contains(out, "=== minimalist") {
		t.Errorf("menu should render status header at top; got: %q", out)
	}
	// 旧的 "1) 状态总览" 不再出现
	if strings.Contains(out, "1) 状态总览") {
		t.Errorf("status overview should be replaced by header, not menu item")
	}
}
```

注意：现有 `newTestApp` 把 Stdout 设成什么？检查：

```bash
grep -A3 "Stdout:" internal/app/app_test.go | head -5
```

如果不是 `bytes.Buffer`，调整断言读取方式。

- [ ] **Step 2: Run to verify fail**

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/app -run TestMenuRendersStatusHeader -v
```

Expected: FAIL.

- [ ] **Step 3: Modify `Menu()` in `internal/app/app.go:472-514`**

替换为：

```go
func (a *App) Menu() error {
	reader := bufio.NewReader(a.Stdin)
	report := func(err error) {
		if err != nil {
			fmt.Fprintln(a.Stderr, err)
		}
	}
	for {
		fmt.Fprint(a.Stdout, a.renderStatusHeader())
		fmt.Fprintln(a.Stdout, "1) 部署/修复")
		fmt.Fprintln(a.Stdout, "2) 节点管理")
		fmt.Fprintln(a.Stdout, "3) 订阅管理")
		fmt.Fprintln(a.Stdout, "4) 网络入口与规则仓库")
		fmt.Fprintln(a.Stdout, "5) 规则与 ACL")
		fmt.Fprintln(a.Stdout, "6) 服务管理")
		fmt.Fprintln(a.Stdout, "7) 健康检查与审计")
		fmt.Fprintln(a.Stdout, "0) 退出")
		fmt.Fprint(a.Stdout, "> ")
		line, _ := reader.ReadString('\n')
		switch strings.TrimSpace(line) {
		case "1":
			report(a.deployMenu(reader))
		case "2":
			report(a.nodesMenu(reader))
		case "3":
			report(a.subscriptionsMenu(reader))
		case "4":
			report(a.networkMenu(reader))
		case "5":
			report(a.rulesAndACLMenu(reader))
		case "6":
			report(a.serviceMenu(reader))
		case "7":
			report(a.auditMenu(reader))
		case "0":
			return nil
		default:
			fmt.Fprintf(a.Stdout, "无效选择: %q（请输入 0-7）\n", strings.TrimSpace(line))
		}
	}
}
```

注意：菜单选项编号下移一位（原来 "1) 状态总览" 取消，新 1=部署/修复...）。Phase 4 时整体重排，Phase 2 这里只是中间状态。

- [ ] **Step 4: Run test to verify pass**

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/app -run TestMenu -v
```

Expected: 新测试 PASS。**警告**：现有的 `TestMenuDispatches*` 类测试可能会失败（因为编号变了）。逐个检查并更新现有测试中的菜单输入 `"1\n"` → `"1\n"` 还指向"部署/修复"等。

- [ ] **Step 5: 修复现有测试中的编号映射**

使用 grep 找到所有用菜单输入数字的测试：

```bash
grep -n 'Stdin = strings.NewReader("1\\n\|app.Menu\|TestMenu' internal/app/app_test.go | head -30
```

逐个比对原来 1→状态总览 / 2→部署/修复 / 3→节点 / 4→订阅 / 5→网络 / 6→规则 / 7→服务 / 8→审计，新映射为 0→退出 / 1→部署 / 2→节点 / 3→订阅 / 4→网络 / 5→规则 / 6→服务 / 7→审计。**所有数字 -1，原来的 "1" 测试要么删除（状态总览不再存在）要么改成调用 `Status()` 而不是从 menu 进入。**

- [ ] **Step 6: Run full app tests + vet + fmt**

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/app/... -count=1
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go vet ./...
gofmt -l cmd internal
```

Expected: all PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/app/app.go internal/app/app_test.go
git commit -m "$(cat <<'EOF'
feat(menu): render status header at top, remove "状态总览" menu item

- 状态总览升级为顶部 status header（每次菜单循环渲染）
- 菜单顶层从 8 项降为 7 项，编号下移
- default 提示改为 "无效选择: %q（请输入 0-7）" 显示用户输入
- 现有测试调整菜单编号映射（-1）

Phase 2 part of TODOS.md menu redesign.
EOF
)"
```

### Task 2.5: 移除 "看完即退" 行为（操作后回菜单）

**Files:**
- Modify: `internal/app/app.go:1416-1465` (`nodesMenu`)
- Modify: `internal/app/app.go:1468-1512` (`subscriptionsMenu`)
- Modify: `internal/app/app.go:1559-1604` (`rulesAndACLMenu`)

- [ ] **Step 1: Write failing test**

追加到 `app_test.go`：

```go
func TestNodesMenuStaysOpenAfterListing(t *testing.T) {
	app := newTestAppWithEnabledManualNode(t)
	// 输入 "1\n0\n" — 看节点列表，然后退回主菜单
	reader := bufio.NewReader(strings.NewReader("1\n0\n"))
	if err := app.nodesMenu(reader); err != nil {
		t.Fatalf("nodes menu: %v", err)
	}
	out := app.Stdout.(*bytes.Buffer).String()
	// 看完节点列表后菜单应该再次显示
	listMenuCount := strings.Count(out, "1) 查看节点")
	if listMenuCount < 2 {
		t.Errorf("nodes menu should re-render after list, found %d times in: %q", listMenuCount, out)
	}
}

func TestSubscriptionsMenuStaysOpenAfterListing(t *testing.T) {
	app, _ := newTestApp(t)
	reader := bufio.NewReader(strings.NewReader("1\n0\n"))
	if err := app.subscriptionsMenu(reader); err != nil {
		t.Fatalf("subscriptions menu: %v", err)
	}
	out := app.Stdout.(*bytes.Buffer).String()
	listMenuCount := strings.Count(out, "1) 查看订阅")
	if listMenuCount < 2 {
		t.Errorf("subscriptions menu should re-render after list, found %d times in: %q", listMenuCount, out)
	}
}
```

- [ ] **Step 2: Run to verify fail**

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/app -run 'TestNodesMenuStaysOpen|TestSubscriptionsMenuStaysOpen' -v
```

Expected: FAIL — listMenuCount=1（看完就退了）。

- [ ] **Step 3: 改 `nodesMenu` 的 case "1"**

`internal/app/app.go:1429-1430` 当前：

```go
		case "1":
			return a.ListNodes()
```

改为：

```go
		case "1":
			if err := a.ListNodes(); err != nil {
				fmt.Fprintln(a.Stderr, err)
			}
			continue
```

同样改 `case "3"`（TestNodes）：

```go
		case "3":
			if err := a.TestNodes(); err != nil {
				fmt.Fprintln(a.Stderr, err)
			}
			continue
```

剩余 `case "2/4/5/6/7"` 也都把 `return a.X(...)` 改成 `if err := ...; err != nil { ... }; continue`。

- [ ] **Step 4: 同样改 `subscriptionsMenu` (line 1468-1512)**

`case "1"` ListSubscriptions、`case "6"` UpdateSubscriptions、剩余 add/enable/disable/remove 同样改。

- [ ] **Step 5: 同样改 `rulesAndACLMenu` (line 1559-1604)**

每个 case 的 `return a.X(...)` 改为 `if err := ...; err != nil { ... }; continue`。

- [ ] **Step 6: 同样改 `networkMenu` / `serviceMenu` / `auditMenu` / `deployMenu`**

行号分别 1393 / 1606 / 1632 / 1514。同样模式。

- [ ] **Step 7: Run all menu tests**

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/app -run 'TestNodesMenu|TestSubscriptionsMenu|TestRulesAndACLMenu|TestNetworkMenu|TestServiceMenu|TestAuditMenu|TestDeployMenu' -v
```

Expected: 新加的 stay-open 测试 PASS。现有 dispatch 测试可能需要调整 stdin（多加一个 "0\n" 退出）。

- [ ] **Step 8: 修复现有测试**

如果现有测试假设"看完就退出"，需要在它们的 stdin 末尾追加 "0\n"。逐个修复直到全绿。

- [ ] **Step 9: Run full suite + vet + fmt**

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./...
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go vet ./...
gofmt -l cmd internal
```

Expected: all PASS.

- [ ] **Step 10: Commit**

```bash
git add internal/app/app.go internal/app/app_test.go
git commit -m "$(cat <<'EOF'
feat(menu): operations stay in submenu (no more "看完即退")

之前 nodesMenu / subscriptionsMenu / rulesAndACLMenu 等子菜单的每个 case
都用 'return a.X(...)' 退回主菜单。改成 'if err := ...; continue'，
看完节点列表后菜单仍在，可以继续操作。

匹配 233boy 'action && show_menu' 模式。覆盖：
- nodesMenu (cases 1-7)
- subscriptionsMenu (cases 1-6)
- rulesAndACLMenu, networkMenu, serviceMenu, auditMenu, deployMenu

Phase 2 of TODOS.md menu redesign.
EOF
)"
```

### Task 2.6: 删除类菜单项接 `promptConfirm`

**Files:**
- Modify: `internal/app/app.go` (nodesMenu case "7", subscriptionsMenu case "5", rulesAndACLMenu remove)

- [ ] **Step 1: Write failing test**

```go
func TestNodesMenuRemovePromptsForConfirm(t *testing.T) {
	app := newTestAppWithEnabledManualNode(t)
	// 输入 "7\n1\nn\n0\n" — 选删除、节点 ID 1、拒绝确认、退出
	reader := bufio.NewReader(strings.NewReader("7\n1\nn\n0\n"))
	if err := app.nodesMenu(reader); err != nil {
		t.Fatalf("nodes menu: %v", err)
	}
	out := app.Stdout.(*bytes.Buffer).String()
	if !strings.Contains(out, "[y/N]") {
		t.Errorf("delete should prompt for confirm with [y/N]; got: %q", out)
	}
	// 节点应仍存在（未确认）
	st, err := state.Load(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if len(st.Nodes) != 1 {
		t.Errorf("node should not be deleted on n; got %d nodes", len(st.Nodes))
	}
}

func TestNodesMenuRemoveProceedsOnYesConfirm(t *testing.T) {
	app := newTestAppWithEnabledManualNode(t)
	reader := bufio.NewReader(strings.NewReader("7\n1\ny\n0\n"))
	if err := app.nodesMenu(reader); err != nil {
		t.Fatalf("nodes menu: %v", err)
	}
	st, err := state.Load(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if len(st.Nodes) != 0 {
		t.Errorf("node should be deleted on y; got %d nodes", len(st.Nodes))
	}
}
```

- [ ] **Step 2: Run to verify fail**

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/app -run 'TestNodesMenuRemovePrompts|TestNodesMenuRemoveProceeds' -v
```

Expected: FAIL.

- [ ] **Step 3: Modify nodesMenu case "7"**

当前（已经在 Task 2.5 改成 continue）：

```go
		case "7":
			index, err := promptIndex(reader, a.Stdout, "节点 ID")
			if err != nil {
				fmt.Fprintln(a.Stderr, err)
				continue
			}
			if err := a.RemoveNode(index); err != nil {
				fmt.Fprintln(a.Stderr, err)
			}
			continue
```

改为：

```go
		case "7":
			index, err := promptIndex(reader, a.Stdout, "节点 ID")
			if err != nil {
				fmt.Fprintln(a.Stderr, err)
				continue
			}
			if !promptConfirm(reader, a.Stdout, fmt.Sprintf("确认删除节点 #%d", index), false) {
				fmt.Fprintln(a.Stdout, "已取消")
				continue
			}
			if err := a.RemoveNode(index); err != nil {
				fmt.Fprintln(a.Stderr, err)
			}
			continue
```

- [ ] **Step 4: 同样改 subscriptionsMenu case "5"**

```go
		case "5":
			index, err := promptIndex(reader, a.Stdout, "订阅 ID")
			if err != nil {
				fmt.Fprintln(a.Stderr, err)
				continue
			}
			if !promptConfirm(reader, a.Stdout, fmt.Sprintf("确认删除订阅 #%d（同时清理本地缓存）", index), false) {
				fmt.Fprintln(a.Stdout, "已取消")
				continue
			}
			if err := a.RemoveSubscription(index); err != nil {
				fmt.Fprintln(a.Stderr, err)
			}
			continue
```

- [ ] **Step 5: 同样改 rulesAndACLMenu 删除规则的 case**

行号 ~1583 (rulesAndACL remove) — 相同模式。

- [ ] **Step 6: Run tests + add subscription remove test**

```go
func TestSubscriptionsMenuRemovePromptsForConfirm(t *testing.T) {
	// 类似 TestNodesMenuRemovePromptsForConfirm
	...
}
```

- [ ] **Step 7: Run + verify pass**

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/app -run 'TestNodesMenuRemove|TestSubscriptionsMenuRemove|TestRulesAndACLMenuRemove' -v
```

Expected: PASS.

- [ ] **Step 8: Run full suite**

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./...
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go vet ./...
gofmt -l cmd internal
```

- [ ] **Step 9: Commit**

```bash
git add internal/app/app.go internal/app/app_test.go
git commit -m "feat(menu): require y/n confirm for delete operations"
```

### Task 2.7: Phase 2 实机验证 + push

- [ ] **Step 1: 安装 + 重启服务**

```bash
sudo go run ./cmd/minimalist install-self
sudo systemctl restart minimalist.service
sleep 2
```

- [ ] **Step 2: smoke**

```bash
m status
m healthcheck
m runtime-audit | grep fatal-gaps
echo "---"
# 跑 menu 看顶部 header
echo "0" | m menu
```

Expected:
- `runtime-audit` 显示 `fatal-gaps=0`
- `m menu` 头部输出包含 `=== minimalist | 服务: 运行中 | 节点: ...` 而不是 `1) 状态总览`
- `0` 退出菜单

- [ ] **Step 3: 服务还稳**

```bash
systemctl is-active minimalist.service
systemctl is-enabled minimalist.service
journalctl -u minimalist.service --since "5 minutes ago" | tail -10
```

Expected: `active/enabled`，最近 5 分钟无 error。

- [ ] **Step 4: push**

```bash
git push origin main
```

---

## Phase 3: 功能补全 — host-proxy + log

**目的**：把 `proxy_host_output` 字段（已存在）暴露成显式 `host-proxy on/off/status` CLI；新增 `log` 子命令封装 `journalctl`。

### Task 3.1: 实现 `HostProxyStatus`

**Files:**
- Create: `internal/app/host_proxy.go`
- Create: `internal/app/host_proxy_test.go`

- [ ] **Step 1: Write failing test (`internal/app/host_proxy_test.go`)**

```go
package app

import (
	"bytes"
	"strings"
	"testing"
)

func TestHostProxyStatusReportsOff(t *testing.T) {
	app, _ := newTestApp(t)
	if err := app.HostProxyStatus(); err != nil {
		t.Fatalf("host-proxy status: %v", err)
	}
	out := app.Stdout.(*bytes.Buffer).String()
	if !strings.Contains(out, "off") && !strings.Contains(out, "关闭") {
		t.Errorf("expected status off/关闭 by default; got: %q", out)
	}
}

func TestHostProxyStatusReportsOnWhenConfigured(t *testing.T) {
	app, _ := newTestApp(t)
	// 直接通过 HostProxyEnable 设上去（也是后面要测的方法）
	app.Stdin = strings.NewReader("y\n")
	if err := app.HostProxyEnable(); err != nil {
		t.Fatalf("host-proxy enable: %v", err)
	}
	app.Stdout.(*bytes.Buffer).Reset()
	if err := app.HostProxyStatus(); err != nil {
		t.Fatalf("host-proxy status: %v", err)
	}
	out := app.Stdout.(*bytes.Buffer).String()
	if !strings.Contains(out, "on") && !strings.Contains(out, "已接管") {
		t.Errorf("expected status on/已接管 after enable; got: %q", out)
	}
}
```

- [ ] **Step 2: Run to verify fail**

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/app -run TestHostProxy -v
```

Expected: FAIL — undefined.

- [ ] **Step 3: Create `internal/app/host_proxy.go`**

```go
package app

import (
	"fmt"

	"minimalist/internal/config"
)

// HostProxyStatus prints whether host OUTPUT proxy is enabled.
func (a *App) HostProxyStatus() error {
	cfg, _, err := a.ensureAll()
	if err != nil {
		return err
	}
	if cfg.Network.ProxyHostOutput {
		fmt.Fprintln(a.Stdout, "宿主机流量接管: on (★已接管★)")
	} else {
		fmt.Fprintln(a.Stdout, "宿主机流量接管: off (关闭)")
	}
	return nil
}

// HostProxyEnable turns on host OUTPUT proxy. Requires y/N confirm (high risk).
// Saves config + re-renders runtime + applies rules.
func (a *App) HostProxyEnable() error {
	if err := a.requireRoot(); err != nil {
		return err
	}
	cfg, _, err := a.ensureAll()
	if err != nil {
		return err
	}
	if cfg.Network.ProxyHostOutput {
		fmt.Fprintln(a.Stdout, "宿主机流量接管已是 on，无需变更")
		return nil
	}
	reader := bufioReader(a.Stdin)
	fmt.Fprintln(a.Stdout, "⚠️  接管宿主机流量后，host OUTPUT 链将被劫持到 mihomo。")
	fmt.Fprintln(a.Stdout, "   出错时可能导致宿主机自身网络异常，请确保有 console / IPMI 兜底。")
	if !promptConfirm(reader, a.Stdout, "确认开启宿主机流量接管", false) {
		fmt.Fprintln(a.Stdout, "已取消")
		return nil
	}
	cfg.Network.ProxyHostOutput = true
	if err := config.Save(a.Paths.ConfigPath(), cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	if err := a.RenderConfig(); err != nil {
		return fmt.Errorf("render config: %w", err)
	}
	if err := a.ApplyRules(); err != nil {
		return fmt.Errorf("apply rules: %w", err)
	}
	fmt.Fprintln(a.Stdout, "已开启宿主机流量接管")
	return nil
}

// HostProxyDisable turns off host OUTPUT proxy. Idempotent.
func (a *App) HostProxyDisable() error {
	if err := a.requireRoot(); err != nil {
		return err
	}
	cfg, _, err := a.ensureAll()
	if err != nil {
		return err
	}
	if !cfg.Network.ProxyHostOutput {
		fmt.Fprintln(a.Stdout, "宿主机流量接管已是 off，无需变更")
		return nil
	}
	cfg.Network.ProxyHostOutput = false
	if err := config.Save(a.Paths.ConfigPath(), cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	if err := a.RenderConfig(); err != nil {
		return fmt.Errorf("render config: %w", err)
	}
	if err := a.ApplyRules(); err != nil {
		return fmt.Errorf("apply rules: %w", err)
	}
	fmt.Fprintln(a.Stdout, "已关闭宿主机流量接管")
	return nil
}

// bufioReader wraps a.Stdin in a *bufio.Reader (small helper to avoid imports clutter).
func bufioReader(r interface{}) *bufio.Reader {
	if br, ok := r.(*bufio.Reader); ok {
		return br
	}
	return bufio.NewReader(r.(io.Reader))
}
```

注意：
- 需要 import `bufio` 和 `io`
- `config.Save` 必须存在；如果不存在，需要先实现（看 `internal/config/config.go` 是否已有 yaml.Marshal+WriteFile 的导出函数；若无则在该文件中加 `func Save(path string, cfg Config) error`）
- `requireRoot()` 已存在于 `internal/app/app.go`

如果 `config.Save` 不存在：

```go
// 加到 internal/config/config.go 末尾
func Save(path string, cfg Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
```

并补一个 `TestSaveRoundTrip` 测试。

- [ ] **Step 4: Run tests**

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/app -run TestHostProxy -v
```

Expected: 2/2 PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/app/host_proxy.go internal/app/host_proxy_test.go internal/config/config.go internal/config/config_test.go
git commit -m "feat(app): add HostProxyStatus/Enable/Disable methods"
```

### Task 3.2: CLI 分发 `host-proxy`

**Files:**
- Modify: `internal/cli/cli.go`
- Modify: `internal/cli/cli_test.go`

- [ ] **Step 1: Write failing test**

```go
func TestCLIHostProxyDispatch(t *testing.T) {
	cases := []struct {
		args   []string
		method string
	}{
		{[]string{"host-proxy", "status"}, "HostProxyStatus"},
		{[]string{"host-proxy", "on"}, "HostProxyEnable"},
		{[]string{"host-proxy", "off"}, "HostProxyDisable"},
	}
	for _, tc := range cases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			app := mockApp{calls: []string{}}
			err := dispatch(&app, tc.args)
			if err != nil {
				t.Fatalf("dispatch %v: %v", tc.args, err)
			}
			if len(app.calls) != 1 || app.calls[0] != tc.method {
				t.Errorf("expected %s; got %v", tc.method, app.calls)
			}
		})
	}
}
```

注意：`mockApp` / `dispatch` 函数需要看现有 `cli_test.go` 的测试 pattern。如果用的是 `appAdapter` interface 等，按现有 pattern 复用。

- [ ] **Step 2: Run to verify fail**

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/cli -run TestCLIHostProxy -v
```

Expected: FAIL — `host-proxy` not dispatched.

- [ ] **Step 3: Add dispatch in `internal/cli/cli.go`**

在主 `switch` 块（`case "rules-repo":` 之后、`case "-h", "--help":` 之前）：

```go
	case "host-proxy":
		return dispatchHostProxy(a, args)
```

并在文件末尾加：

```go
func dispatchHostProxy(a App, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: minimalist host-proxy status|on|off")
	}
	switch args[0] {
	case "status":
		return a.HostProxyStatus()
	case "on":
		return a.HostProxyEnable()
	case "off":
		return a.HostProxyDisable()
	default:
		return fmt.Errorf("unknown host-proxy action: %q", args[0])
	}
}
```

注意：需要看 `App` interface 是否在 `cli.go` 里以接口形式定义（目前看 line 26 用 `a.InstallSelf()` 直接调，可能是结构体或 interface）。

```bash
grep -n "type App\|^type.*interface" internal/cli/cli.go
```

如果是 interface，需要把 `HostProxyStatus/Enable/Disable` 加到接口定义。

- [ ] **Step 4: 更新 helptext**

`internal/cli/cli.go:74` 的 `--help` 分支末尾追加（在 `minimalist rules-repo summary|...` 那行之后）：

```go
		fmt.Fprintln(out, "  minimalist host-proxy status|on|off")
```

- [ ] **Step 5: Run tests**

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/cli -run TestCLIHostProxy -v
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./...
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/cli/cli.go internal/cli/cli_test.go
git commit -m "feat(cli): dispatch host-proxy status|on|off subcommand"
```

### Task 3.3: 实现 `Logs` 方法

**Files:**
- Create: `internal/app/logs.go`
- Create: `internal/app/logs_test.go`

- [ ] **Step 1: Write failing test**

```go
package app

import (
	"strings"
	"testing"
)

func TestLogsCallsJournalctlMinimalist(t *testing.T) {
	app, _ := newTestApp(t)
	called := []string{}
	app.Runner = fakeRunner{
		outputFn: func(name string, args ...string) (string, string, error) {
			called = append(called, name+" "+strings.Join(args, " "))
			return "fake log line\n", "", nil
		},
	}
	if err := app.Logs(LogOptions{Lines: 50}); err != nil {
		t.Fatalf("logs: %v", err)
	}
	if len(called) != 1 {
		t.Fatalf("expected 1 journalctl call; got %d (%v)", len(called), called)
	}
	if !strings.Contains(called[0], "journalctl") {
		t.Errorf("expected journalctl, got: %s", called[0])
	}
	if !strings.Contains(called[0], "-u minimalist.service") && !strings.Contains(called[0], "-u minimalist") {
		t.Errorf("expected -u minimalist.service unit, got: %s", called[0])
	}
	if !strings.Contains(called[0], "-n 50") {
		t.Errorf("expected -n 50 line limit, got: %s", called[0])
	}
}

func TestLogsTargetsMihomoWhenSpecified(t *testing.T) {
	app, _ := newTestApp(t)
	called := []string{}
	app.Runner = fakeRunner{
		outputFn: func(name string, args ...string) (string, string, error) {
			called = append(called, name+" "+strings.Join(args, " "))
			return "", "", nil
		},
	}
	if err := app.Logs(LogOptions{Target: "mihomo", Lines: 50}); err != nil {
		t.Fatalf("logs: %v", err)
	}
	if !strings.Contains(called[0], "mihomo-core.service") && !strings.Contains(called[0], "mihomo") {
		t.Errorf("expected mihomo target, got: %s", called[0])
	}
}

func TestLogsErrorsFlagAddsPriorityFilter(t *testing.T) {
	app, _ := newTestApp(t)
	called := []string{}
	app.Runner = fakeRunner{
		outputFn: func(name string, args ...string) (string, string, error) {
			called = append(called, name+" "+strings.Join(args, " "))
			return "", "", nil
		},
	}
	if err := app.Logs(LogOptions{Errors: true, Lines: 50}); err != nil {
		t.Fatalf("logs: %v", err)
	}
	if !strings.Contains(called[0], "-p err") && !strings.Contains(called[0], "--priority=err") {
		t.Errorf("expected -p err filter, got: %s", called[0])
	}
}
```

注意：`fakeRunner` 现有结构需要看；如果它不支持 `outputFn`，需要扩展。

- [ ] **Step 2: Run to verify fail**

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/app -run TestLogs -v
```

Expected: FAIL.

- [ ] **Step 3: Create `internal/app/logs.go`**

```go
package app

import (
	"fmt"
)

// LogOptions controls Logs() behavior.
type LogOptions struct {
	Target string // "minimalist" (default) or "mihomo"
	Lines  int    // -n value, default 50
	Follow bool   // -f
	Errors bool   // --priority=err
}

// Logs runs `journalctl -u <unit> -n <lines> [...]` and prints output.
// Default target = minimalist.service.
func (a *App) Logs(opts LogOptions) error {
	unit := "minimalist.service"
	if opts.Target == "mihomo" {
		unit = "mihomo-core.service"
	}
	if opts.Lines <= 0 {
		opts.Lines = 50
	}
	args := []string{"-u", unit, "-n", fmt.Sprintf("%d", opts.Lines)}
	if opts.Follow {
		args = append(args, "-f")
	}
	if opts.Errors {
		args = append(args, "-p", "err")
	}
	out, errOut, err := a.Runner.Output("journalctl", args...)
	if err != nil {
		return fmt.Errorf("journalctl: %w (stderr: %s)", err, errOut)
	}
	fmt.Fprint(a.Stdout, out)
	return nil
}
```

- [ ] **Step 4: 如果 fakeRunner 没有 outputFn 字段，扩展它**

看 `internal/app/app_test.go` 里 `fakeRunner` 当前结构：

```bash
grep -A10 "type fakeRunner struct" internal/app/app_test.go
```

如果只有 `runFn`，加 `outputFn func(name string, args ...string) (string, string, error)`，并实现 `Output` 方法。

- [ ] **Step 5: Run tests + verify pass**

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/app -run TestLogs -v
```

Expected: 3/3 PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/app/logs.go internal/app/logs_test.go internal/app/app_test.go
git commit -m "feat(app): add Logs() wrapping journalctl with target/lines/follow/errors options"
```

### Task 3.4: CLI 分发 `log`

**Files:**
- Modify: `internal/cli/cli.go`
- Modify: `internal/cli/cli_test.go`

- [ ] **Step 1: Write failing test**

```go
func TestCLILogDefaults(t *testing.T) {
	app := mockApp{}
	if err := dispatch(&app, []string{"log"}); err != nil {
		t.Fatalf("dispatch log: %v", err)
	}
	if app.lastLogOptions.Target != "" || app.lastLogOptions.Follow || app.lastLogOptions.Errors {
		t.Errorf("default log: expected zero options, got %+v", app.lastLogOptions)
	}
}

func TestCLILogParsesFlags(t *testing.T) {
	cases := []struct {
		args     []string
		expect   LogOptions
	}{
		{[]string{"log", "-f"}, LogOptions{Follow: true}},
		{[]string{"log", "--errors"}, LogOptions{Errors: true}},
		{[]string{"log", "mihomo"}, LogOptions{Target: "mihomo"}},
		{[]string{"log", "-f", "mihomo", "--errors"}, LogOptions{Target: "mihomo", Follow: true, Errors: true}},
	}
	for _, tc := range cases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			app := mockApp{}
			if err := dispatch(&app, tc.args); err != nil {
				t.Fatalf("dispatch: %v", err)
			}
			if app.lastLogOptions.Target != tc.expect.Target ||
				app.lastLogOptions.Follow != tc.expect.Follow ||
				app.lastLogOptions.Errors != tc.expect.Errors {
				t.Errorf("expected %+v; got %+v", tc.expect, app.lastLogOptions)
			}
		})
	}
}
```

- [ ] **Step 2: Run to verify fail**

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/cli -run TestCLILog -v
```

Expected: FAIL.

- [ ] **Step 3: Add `case "log":` dispatch in `internal/cli/cli.go`**

在主 switch 末尾加：

```go
	case "log":
		return dispatchLog(a, args)
```

并实现 `dispatchLog`：

```go
func dispatchLog(a App, args []string) error {
	opts := app.LogOptions{Lines: 50}
	for _, arg := range args {
		switch arg {
		case "-f", "--follow":
			opts.Follow = true
		case "--errors":
			opts.Errors = true
		case "minimalist":
			opts.Target = "minimalist"
		case "mihomo":
			opts.Target = "mihomo"
		default:
			return fmt.Errorf("unknown log argument: %q", arg)
		}
	}
	return a.Logs(opts)
}
```

注意：如果 `cli` 包不能 import `app`（循环依赖），需要把 `LogOptions` 移到一个公共位置（比如 `internal/options` 或直接在 cli 包重新定义并由 app 接收 interface 形式参数）。检查现有项目的 import 方向：

```bash
grep "import" internal/app/app.go | head -10
grep "import" internal/cli/cli.go | head -10
```

如果 cli 已经 import app（例如通过 interface），用 `app.LogOptions`。否则把 `LogOptions` 也搬到 cli 包并通过 interface 传入。

- [ ] **Step 4: 更新 helptext**

```go
		fmt.Fprintln(out, "  minimalist log [mihomo] [-f] [--errors]")
```

- [ ] **Step 5: Run tests**

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/cli -run TestCLILog -v
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./...
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/cli/cli.go internal/cli/cli_test.go
git commit -m "feat(cli): dispatch log subcommand with -f / --errors / mihomo flags"
```

### Task 3.5: Phase 3 实机验证 + push

- [ ] **Step 1: Install + restart**

```bash
sudo go run ./cmd/minimalist install-self
sudo systemctl restart minimalist.service
sleep 2
```

- [ ] **Step 2: smoke 新命令**

```bash
m host-proxy status     # 应显示 off
m log -n 5 || m log     # 应显示最近日志（包含 mihomo-core 启动）
m log mihomo            # 应显示 mihomo-core 日志
m log --errors          # 只显示错误（可能空）
```

- [ ] **Step 3: 验证 host-proxy off → on → off 完整路径（小心：on 可能影响宿主机网络）**

```bash
# 确认当前是 off
m host-proxy status

# 不实际 toggle on，仅验证 off 路径：
echo "n" | sudo m host-proxy on    # 拒绝二次确认
m host-proxy status                # 仍然 off

# 完整 toggle 验证留给手动操作（高风险）
```

- [ ] **Step 4: 服务还稳**

```bash
systemctl is-active minimalist.service
m runtime-audit | grep fatal-gaps
```

Expected: `active`，`fatal-gaps=0`。

- [ ] **Step 5: push**

```bash
git push origin main
```

---

## Phase 4: 顶层导航重排为 5 类

**目的**：把当前 7 个菜单组（Phase 2 之后）合并/重命名为 5 类（节点 / 配置 / 规则 / 日志 / 启停），匹配用户 mental model。

### Task 4.1: 新建 `configMenu`（合并 router-wizard + subscriptions + host-proxy toggle）

**Files:**
- Modify: `internal/app/app.go` (新增 configMenu，行号 in flux)

- [ ] **Step 1: Write failing test**

```go
func TestConfigMenuDispatchesRouterWizard(t *testing.T) {
	app, _ := newTestApp(t)
	app.Stdin = strings.NewReader("...") // 多行输入触达 routerWizardWithReader 默认值
	reader := bufio.NewReader(strings.NewReader("1\n" + strings.Repeat("\n", 14) + "0\n"))
	if err := app.configMenu(reader); err != nil {
		t.Fatalf("config menu: %v", err)
	}
	out := app.Stdout.(*bytes.Buffer).String()
	if !strings.Contains(out, "1) 网络/路由配置") {
		t.Errorf("config menu should list 网络/路由配置 as item 1; got: %q", out)
	}
	if !strings.Contains(out, "宿主机接管") {
		t.Errorf("config menu should expose host-proxy toggle; got: %q", out)
	}
}
```

- [ ] **Step 2: Run to verify fail**

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/app -run TestConfigMenu -v
```

Expected: FAIL — `configMenu` undefined.

- [ ] **Step 3: 实现 `configMenu`**

加到 `internal/app/app.go`（删除原 `networkMenu` 之前或之后，看清晰度）：

```go
// configMenu groups all "configuration" actions: router-wizard, subscriptions,
// host-proxy toggle. Replaces the original networkMenu + subscriptionsMenu split.
func (a *App) configMenu(reader *bufio.Reader) error {
	for {
		fmt.Fprint(a.Stdout, a.renderStatusHeader())
		fmt.Fprintln(a.Stdout, "-- 配置管理 --")
		fmt.Fprintln(a.Stdout, "1) 网络/路由配置（router-wizard）")
		fmt.Fprintln(a.Stdout, "2) 订阅管理（导入 / 启停 / 更新 / 删除）")
		fmt.Fprintln(a.Stdout, "3) 宿主机接管开关")
		fmt.Fprintln(a.Stdout, "0) 返回")
		fmt.Fprint(a.Stdout, "> ")
		line, _ := reader.ReadString('\n')
		switch strings.TrimSpace(line) {
		case "1":
			if err := a.routerWizardWithReader(reader); err != nil {
				fmt.Fprintln(a.Stderr, err)
			}
			continue
		case "2":
			if err := a.subscriptionsMenu(reader); err != nil {
				fmt.Fprintln(a.Stderr, err)
			}
			continue
		case "3":
			if err := a.hostProxyToggleMenu(reader); err != nil {
				fmt.Fprintln(a.Stderr, err)
			}
			continue
		case "0":
			return nil
		default:
			fmt.Fprintf(a.Stdout, "无效选择: %q（请输入 0-3）\n", strings.TrimSpace(line))
		}
	}
}

// hostProxyToggleMenu shows current state and offers toggle.
func (a *App) hostProxyToggleMenu(reader *bufio.Reader) error {
	cfg, _, err := a.ensureAll()
	if err != nil {
		return err
	}
	fmt.Fprintf(a.Stdout, "当前状态: 宿主机流量接管 = %v\n", cfg.Network.ProxyHostOutput)
	if cfg.Network.ProxyHostOutput {
		if !promptConfirm(reader, a.Stdout, "确认关闭", true) {
			return nil
		}
		return a.HostProxyDisable()
	}
	return a.HostProxyEnable()
}
```

- [ ] **Step 4: Run tests**

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/app -run TestConfigMenu -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/app/app.go internal/app/app_test.go
git commit -m "feat(menu): add configMenu merging router-wizard + subscriptions + host-proxy toggle"
```

### Task 4.2: 新建 `rulesMenu`（合并 rulesAndACL + rules-repo）

**Files:**
- Modify: `internal/app/app.go`

- [ ] **Step 1: Write failing test**

```go
func TestRulesMenuOffersAllSources(t *testing.T) {
	app, _ := newTestApp(t)
	reader := bufio.NewReader(strings.NewReader("0\n"))
	if err := app.rulesMenu(reader); err != nil {
		t.Fatalf("rules menu: %v", err)
	}
	out := app.Stdout.(*bytes.Buffer).String()
	for _, want := range []string{"自定义规则", "ACL", "仓库规则"} {
		if !strings.Contains(out, want) {
			t.Errorf("rules menu missing %q; got: %q", want, out)
		}
	}
}
```

- [ ] **Step 2: Run to verify fail**

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/app -run TestRulesMenu -v
```

Expected: FAIL.

- [ ] **Step 3: 实现 `rulesMenu`**

```go
// rulesMenu groups custom rules + ACL + repo rules into one entry.
// Replaces rulesAndACLMenu split + the rules-repo CLI being hidden.
func (a *App) rulesMenu(reader *bufio.Reader) error {
	for {
		fmt.Fprint(a.Stdout, a.renderStatusHeader())
		fmt.Fprintln(a.Stdout, "-- 规则管理 --")
		fmt.Fprintln(a.Stdout, "1) 自定义规则（custom.rules）")
		fmt.Fprintln(a.Stdout, "2) ACL 规则（acl.rules）")
		fmt.Fprintln(a.Stdout, "3) 仓库规则（builtin.rules）")
		fmt.Fprintln(a.Stdout, "0) 返回")
		fmt.Fprint(a.Stdout, "> ")
		line, _ := reader.ReadString('\n')
		switch strings.TrimSpace(line) {
		case "1":
			if err := a.customRulesSubmenu(reader); err != nil {
				fmt.Fprintln(a.Stderr, err)
			}
			continue
		case "2":
			if err := a.aclRulesSubmenu(reader); err != nil {
				fmt.Fprintln(a.Stderr, err)
			}
			continue
		case "3":
			if err := a.repoRulesSubmenu(reader); err != nil {
				fmt.Fprintln(a.Stderr, err)
			}
			continue
		case "0":
			return nil
		default:
			fmt.Fprintf(a.Stdout, "无效选择: %q（请输入 0-3）\n", strings.TrimSpace(line))
		}
	}
}
```

`customRulesSubmenu` / `aclRulesSubmenu` / `repoRulesSubmenu` 是把现有 `rulesAndACLMenu` 的 case 拆三块独立子菜单。具体内容从 `internal/app/app.go:1559-1604` 抽取。

- [ ] **Step 4: 实现三个子菜单**

每个子菜单用现有 `RuleAdd / RuleRemove / RuleList / ACL* / RulesRepo*` 业务方法（不动业务层）。模式跟 nodesMenu 类似：list / add / remove + 0 返回。

- [ ] **Step 5: Run tests**

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/app -run TestRulesMenu -v
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/app -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/app/app.go internal/app/app_test.go
git commit -m "feat(menu): add rulesMenu merging rulesAndACL + rules-repo"
```

### Task 4.3: 新建 `logMenu`（封装 Logs + runtime-audit）

**Files:**
- Modify: `internal/app/app.go`

- [ ] **Step 1: Write failing test**

```go
func TestLogMenuOffersFollowAndErrorsAndRuntimeAudit(t *testing.T) {
	app, _ := newTestApp(t)
	app.Runner = fakeRunner{
		outputFn: func(name string, args ...string) (string, string, error) {
			return "", "", nil
		},
	}
	reader := bufio.NewReader(strings.NewReader("0\n"))
	if err := app.logMenu(reader); err != nil {
		t.Fatalf("log menu: %v", err)
	}
	out := app.Stdout.(*bytes.Buffer).String()
	for _, want := range []string{"最近 50 行", "follow", "只看错误", "runtime-audit"} {
		if !strings.Contains(out, want) {
			t.Errorf("log menu missing %q; got: %q", want, out)
		}
	}
}
```

- [ ] **Step 2: Run to verify fail + Step 3: Implement**

```go
func (a *App) logMenu(reader *bufio.Reader) error {
	for {
		fmt.Fprint(a.Stdout, a.renderStatusHeader())
		fmt.Fprintln(a.Stdout, "-- 日志 --")
		fmt.Fprintln(a.Stdout, "1) 最近 50 行 minimalist 日志")
		fmt.Fprintln(a.Stdout, "2) 最近 50 行 mihomo-core 日志")
		fmt.Fprintln(a.Stdout, "3) follow 模式（Ctrl-C 退出）")
		fmt.Fprintln(a.Stdout, "4) 只看错误（warn/error）")
		fmt.Fprintln(a.Stdout, "5) runtime-audit 摘要")
		fmt.Fprintln(a.Stdout, "0) 返回")
		fmt.Fprint(a.Stdout, "> ")
		line, _ := reader.ReadString('\n')
		switch strings.TrimSpace(line) {
		case "1":
			if err := a.Logs(LogOptions{Lines: 50}); err != nil {
				fmt.Fprintln(a.Stderr, err)
			}
			continue
		case "2":
			if err := a.Logs(LogOptions{Target: "mihomo", Lines: 50}); err != nil {
				fmt.Fprintln(a.Stderr, err)
			}
			continue
		case "3":
			if err := a.Logs(LogOptions{Follow: true, Lines: 50}); err != nil {
				fmt.Fprintln(a.Stderr, err)
			}
			continue
		case "4":
			if err := a.Logs(LogOptions{Errors: true, Lines: 100}); err != nil {
				fmt.Fprintln(a.Stderr, err)
			}
			continue
		case "5":
			if err := a.RuntimeAudit(); err != nil {
				fmt.Fprintln(a.Stderr, err)
			}
			continue
		case "0":
			return nil
		default:
			fmt.Fprintf(a.Stdout, "无效选择: %q（请输入 0-5）\n", strings.TrimSpace(line))
		}
	}
}
```

- [ ] **Step 4-5: Test + Commit**

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/app -run TestLogMenu -v
git add internal/app/app.go internal/app/app_test.go
git commit -m "feat(menu): add logMenu wrapping Logs + runtime-audit"
```

### Task 4.4: 合并 `deployMenu` 进 `serviceMenu`，重命名为 `controlMenu`

**Files:**
- Modify: `internal/app/app.go`

- [ ] **Step 1: Write failing test**

```go
func TestControlMenuIncludesDeployActions(t *testing.T) {
	app, _ := newTestApp(t)
	reader := bufio.NewReader(strings.NewReader("0\n"))
	if err := app.controlMenu(reader); err != nil {
		t.Fatalf("control menu: %v", err)
	}
	out := app.Stdout.(*bytes.Buffer).String()
	for _, want := range []string{"start", "stop", "restart", "install-self", "setup", "render-config", "cutover"} {
		if !strings.Contains(out, want) {
			t.Errorf("control menu missing %q; got: %q", want, out)
		}
	}
}
```

- [ ] **Step 2: Implement**

```go
// controlMenu replaces deployMenu + serviceMenu. Groups all "控制启停" actions:
// install-self / setup / render-config / start/stop/restart / cutover-preflight / cutover-plan.
func (a *App) controlMenu(reader *bufio.Reader) error {
	for {
		fmt.Fprint(a.Stdout, a.renderStatusHeader())
		fmt.Fprintln(a.Stdout, "-- 控制启停 --")
		fmt.Fprintln(a.Stdout, "1) start — 启动服务")
		fmt.Fprintln(a.Stdout, "2) stop — 停止服务")
		fmt.Fprintln(a.Stdout, "3) restart — 重启服务")
		fmt.Fprintln(a.Stdout, "4) setup — 重新部署 + 应用规则")
		fmt.Fprintln(a.Stdout, "5) render-config — 仅渲染配置（不重启）")
		fmt.Fprintln(a.Stdout, "6) install-self — 重装二进制")
		fmt.Fprintln(a.Stdout, "7) cutover-preflight — 切换检查（只读）")
		fmt.Fprintln(a.Stdout, "8) cutover-plan — 切换计划（只读）")
		fmt.Fprintln(a.Stdout, "0) 返回")
		fmt.Fprint(a.Stdout, "> ")
		line, _ := reader.ReadString('\n')
		switch strings.TrimSpace(line) {
		case "1":
			if err := a.Start(); err != nil {
				fmt.Fprintln(a.Stderr, err)
			}
			continue
		case "2":
			if err := a.Stop(); err != nil {
				fmt.Fprintln(a.Stderr, err)
			}
			continue
		case "3":
			if err := a.Restart(); err != nil {
				fmt.Fprintln(a.Stderr, err)
			}
			continue
		case "4":
			if err := a.Setup(); err != nil {
				fmt.Fprintln(a.Stderr, err)
			}
			continue
		case "5":
			if err := a.RenderConfig(); err != nil {
				fmt.Fprintln(a.Stderr, err)
			}
			continue
		case "6":
			if err := a.InstallSelf(); err != nil {
				fmt.Fprintln(a.Stderr, err)
			}
			continue
		case "7":
			if err := a.CutoverPreflight(); err != nil {
				fmt.Fprintln(a.Stderr, err)
			}
			continue
		case "8":
			if err := a.CutoverPlan(); err != nil {
				fmt.Fprintln(a.Stderr, err)
			}
			continue
		case "0":
			return nil
		default:
			fmt.Fprintf(a.Stdout, "无效选择: %q（请输入 0-8）\n", strings.TrimSpace(line))
		}
	}
}
```

- [ ] **Step 3-5: Test + Commit**

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/app -run TestControlMenu -v
git add internal/app/app.go internal/app/app_test.go
git commit -m "feat(menu): merge deployMenu + serviceMenu into controlMenu"
```

### Task 4.5: 重排顶层 `Menu()` 为 5 类

**Files:**
- Modify: `internal/app/app.go:472` (`Menu` 函数 — 在 Task 2.4 已经动过)

- [ ] **Step 1: Update failing test**

更新 `TestMenuRendersStatusHeaderEachLoop` 以及任何依赖菜单选项的测试：

```go
func TestMenuTopLevelHasFiveCategories(t *testing.T) {
	app, _ := newTestApp(t)
	app.Stdin = strings.NewReader("0\n")
	if err := app.Menu(); err != nil {
		t.Fatalf("menu: %v", err)
	}
	out := app.Stdout.(*bytes.Buffer).String()
	for _, want := range []string{
		"1) 节点管理",
		"2) 配置管理",
		"3) 规则管理",
		"4) 日志",
		"5) 控制启停",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("menu missing %q; got: %q", want, out)
		}
	}
	for _, gone := range []string{
		"状态总览", "部署/修复", "网络入口与规则仓库", "规则与 ACL",
		"服务管理", "健康检查与审计", "订阅管理",
	} {
		if strings.Contains(out, gone) {
			t.Errorf("old menu label %q should be gone; got: %q", gone, out)
		}
	}
}
```

- [ ] **Step 2: Update `Menu()` (`internal/app/app.go:472`)**

```go
func (a *App) Menu() error {
	reader := bufio.NewReader(a.Stdin)
	report := func(err error) {
		if err != nil {
			fmt.Fprintln(a.Stderr, err)
		}
	}
	for {
		fmt.Fprint(a.Stdout, a.renderStatusHeader())
		fmt.Fprintln(a.Stdout, "1) 节点管理")
		fmt.Fprintln(a.Stdout, "2) 配置管理")
		fmt.Fprintln(a.Stdout, "3) 规则管理")
		fmt.Fprintln(a.Stdout, "4) 日志")
		fmt.Fprintln(a.Stdout, "5) 控制启停")
		fmt.Fprintln(a.Stdout, "0) 退出")
		fmt.Fprint(a.Stdout, "> ")
		line, _ := reader.ReadString('\n')
		switch strings.TrimSpace(line) {
		case "1":
			report(a.nodesMenu(reader))
		case "2":
			report(a.configMenu(reader))
		case "3":
			report(a.rulesMenu(reader))
		case "4":
			report(a.logMenu(reader))
		case "5":
			report(a.controlMenu(reader))
		case "0":
			return nil
		default:
			fmt.Fprintf(a.Stdout, "无效选择: %q（请输入 0-5）\n", strings.TrimSpace(line))
		}
	}
}
```

- [ ] **Step 3: 删除已废弃的 menu 函数**

逐一删除（已被 configMenu / rulesMenu / logMenu / controlMenu 取代）：
- `deployMenu` (1393-1414)
- `subscriptionsMenu`：保留作为 configMenu 子菜单调用，**不删**
- `networkMenu` (1514-1557) — 内容已合并到 routerWizard / configMenu，删
- `rulesAndACLMenu` (1559-1604) — 已被 rulesMenu 替代，删
- `serviceMenu` (1606-1630) — 已合并到 controlMenu，删
- `auditMenu` (1632-XXX) — 内容（`runtime-audit` / `healthcheck`）拆到 logMenu / controlMenu，删

注意：删除函数前确认所有调用点已迁移。`grep -n "networkMenu\|rulesAndACLMenu\|serviceMenu\|auditMenu\|deployMenu" internal/app/app.go` 应只返回函数定义本身。

- [ ] **Step 4: 调整大量现有测试**

`TestNetworkMenu*` / `TestServiceMenu*` / `TestAuditMenu*` / `TestDeployMenu*` / `TestRulesAndACLMenu*` 都要么删除（菜单不存在）要么改名映射到新菜单。预计 30+ 测试需要调整。

逐个改：
1. 测试名改：`TestNetworkMenuDispatchesRouterWizard` → `TestConfigMenuDispatchesRouterWizard`
2. 测试中调用的函数：`app.networkMenu(reader)` → `app.configMenu(reader)`
3. 输入序列：根据新菜单结构调整（菜单项可能换序号了）

- [ ] **Step 5: Run full app suite**

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/app/... -count=1 -v 2>&1 | tail -50
```

Expected: all PASS.

- [ ] **Step 6: vet + fmt + 全项目 test**

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go vet ./...
gofmt -l cmd internal
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./...
```

Expected: clean.

- [ ] **Step 7: Commit**

```bash
git add internal/app/app.go internal/app/app_test.go
git commit -m "$(cat <<'EOF'
feat(menu): rearrange top-level menu to 5 categories

按用户 mental model 把当前 7 组重排为 5 类：
- 节点管理 (nodesMenu)
- 配置管理 (configMenu = router-wizard + subscriptions + host-proxy toggle)
- 规则管理 (rulesMenu = rulesAndACL + rules-repo 合并)
- 日志 (logMenu = Logs + runtime-audit)
- 控制启停 (controlMenu = deployMenu + serviceMenu 合并)

废弃 / 删除:
- deployMenu, networkMenu, rulesAndACLMenu, serviceMenu, auditMenu

依赖测试调整 30+ 处。

完成 Phase 4 of TODOS.md menu redesign.
EOF
)"
```

### Task 4.6: 更新 CLI helptext + README

**Files:**
- Modify: `internal/cli/cli.go` (helptext)
- Modify: `README.md` (使用顺序段落)

- [ ] **Step 1: 更新 helptext (`internal/cli/cli.go`)**

把现有命令列表分组：

```go
		fmt.Fprintln(out, "minimalist commands:")
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "  入口:")
		fmt.Fprintln(out, "    minimalist menu                       (短别名: m menu)")
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "  节点管理:")
		fmt.Fprintln(out, "    minimalist nodes list|test|rename|enable|disable|remove")
		fmt.Fprintln(out, "    minimalist import-links")
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "  配置管理:")
		fmt.Fprintln(out, "    minimalist router-wizard")
		fmt.Fprintln(out, "    minimalist subscriptions list|add|enable|disable|remove|update")
		fmt.Fprintln(out, "    minimalist host-proxy status|on|off")
		fmt.Fprintln(out, "    minimalist render-config")
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "  规则管理:")
		fmt.Fprintln(out, "    minimalist rules list|add|remove")
		fmt.Fprintln(out, "    minimalist acl list|add|remove")
		fmt.Fprintln(out, "    minimalist rules-repo summary|entries|find|add|remove|remove-index")
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "  日志:")
		fmt.Fprintln(out, "    minimalist log [mihomo] [-f] [--errors]")
		fmt.Fprintln(out, "    minimalist runtime-audit")
		fmt.Fprintln(out, "    minimalist healthcheck")
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "  控制启停:")
		fmt.Fprintln(out, "    minimalist start|stop|restart")
		fmt.Fprintln(out, "    minimalist setup")
		fmt.Fprintln(out, "    minimalist install-self")
		fmt.Fprintln(out, "    minimalist core-upgrade-alpha")
		fmt.Fprintln(out, "    minimalist cutover-preflight")
		fmt.Fprintln(out, "    minimalist cutover-plan")
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "  内部 / 高级:")
		fmt.Fprintln(out, "    minimalist apply-rules|clear-rules")
		fmt.Fprintln(out, "    minimalist verify-runtime-assets")
		fmt.Fprintln(out, "    minimalist status|show-secret")
```

- [ ] **Step 2: 更新 README "推荐使用顺序" 段**

`README.md` 修改：
- 把 `minimalist` 命令前缀都改成 `m`（保持 minimalist 也可以但默认演示 `m`）
- 在"推荐使用顺序"末尾追加：
  ```
  日常运维：
  - m status        # 看当前状态
  - m log -f        # follow 服务日志
  - m log --errors  # 只看错误
  - m host-proxy status  # 查看宿主机接管开关
  ```

- [ ] **Step 3: 运行 CLI helptext 测试**

如果 `cli_test.go` 里有 `TestCLIHelpListsAllCommands` 类的测试，更新它。如果没有，加一个：

```go
func TestCLIHelpListsAllSubcommands(t *testing.T) {
	out := captureHelp(t)
	for _, cmd := range []string{
		"menu", "nodes", "subscriptions", "host-proxy", "log",
		"rules", "acl", "rules-repo", "start", "stop", "restart",
		"setup", "install-self", "core-upgrade-alpha", "router-wizard",
		"cutover-preflight", "cutover-plan", "runtime-audit",
		"healthcheck", "verify-runtime-assets", "import-links",
		"render-config", "apply-rules", "clear-rules", "status",
		"show-secret",
	} {
		if !strings.Contains(out, cmd) {
			t.Errorf("help should list %q; got: %q", cmd, out)
		}
	}
}
```

- [ ] **Step 4: Run + commit**

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/cli -count=1
gofmt -l cmd internal
git add internal/cli/cli.go internal/cli/cli_test.go README.md
git commit -m "docs(cli): regroup helptext + README to 5-class mental model"
```

### Task 4.7: Phase 4 实机验证 + push

- [ ] **Step 1: 完整安装 + smoke**

```bash
sudo go run ./cmd/minimalist install-self
sudo systemctl restart minimalist.service
sleep 3
```

- [ ] **Step 2: 顶层菜单看清楚**

```bash
echo "0" | m menu | head -20
```

Expected 输出形如：

```
=== minimalist | 服务: 运行中 | 节点: 4 启用 / 4 总计 | 宿主机: 关闭 ===
1) 节点管理
2) 配置管理
3) 规则管理
4) 日志
5) 控制启停
0) 退出
> 
```

- [ ] **Step 3: 各子菜单走一圈**

```bash
# 节点管理 - 看节点（操作完留在菜单）
printf "1\n1\n0\n0\n" | m menu | tail -30

# 配置管理 - host-proxy toggle 入口
printf "2\n3\n0\n0\n" | m menu | tail -10

# 日志 - runtime-audit
printf "4\n5\n0\n0\n" | m menu | tail -10
```

每个都该返回干净，不报 panic。

- [ ] **Step 4: 服务还稳**

```bash
systemctl is-active minimalist.service
m runtime-audit | grep fatal-gaps
m healthcheck
```

Expected: 全部稳定。

- [ ] **Step 5: 测试覆盖率快照**

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/app -coverprofile=/tmp/minimalist-app.cover -count=1
go tool cover -func=/tmp/minimalist-app.cover | tail -5
```

Expected: `internal/app` 覆盖率仍 >= 90%（目标：保持 91-100% 区间不掉）。

- [ ] **Step 6: push**

```bash
git push origin main
```

---

## Closure: 更新 docs/STATUS.md + docs/NEXT_STEP.md

最后一步把项目状态文档同步到新真相。

- [ ] **Step 1: 更新 docs/STATUS.md**

- 在"当前主线"段追加："2026-04-30 完成菜单 + CLI 重设计 Phase 1-4：m 短别名、status header、操作后回菜单、删除二次确认、host-proxy 一键开关、log 命令、5 类顶层导航"
- "已完成能力"段补：`host-proxy`、`log`、`m` 短别名

- [ ] **Step 2: 更新 docs/NEXT_STEP.md**

- "当前阶段"段：从"长期稳定运行观察"改回"代码主线收口后再次进入观察期"
- "下一最小闭环"段：补"开始新一轮 24-72h 观察，重点看 5 类菜单 UX 是否符合 mental model；高频路径日志补到 TODOS.md"
- "本轮不做"段：把"TUI 重写"和"CLI all-in"列为本轮不做（仍在 Phase 5 可选事项）

- [ ] **Step 3: 更新 docs/PROGRESS.md**

追加一段：

```markdown
## Round 2 — 2026-04-30 [HH:MM]

### 完成
- 菜单 + CLI 交互体验重设计 Phase 1-4 全部落地
- 新增 m 短别名、status header、promptConfirm、host-proxy CLI、log CLI
- 顶层导航重排为 5 类（节点 / 配置 / 规则 / 日志 / 启停）
- 6 处删除类动作接二次确认，12+ 处 case "1": return 改为操作后回菜单

### 测试状态
- 通过: focused tests for prompts/header/host_proxy/logs + 全量 go test ./... + go vet + gofmt + 实机 smoke
- 覆盖率: internal/app 维持 91%+

### 遗留 / 下轮继续
- Phase 5（TUI 重写 / CLI all-in 激进改造）作为可选未来事项，不在本轮范围
- 进入新一轮 24-72h 观察期，看 5 类菜单 UX 是否符合 mental model
- 高频路径日志补 TODOS.md（self-observation）

### 下轮目标
- 24-72h 观察通过后，根据使用反馈决定是否启动 Phase 5
```

- [ ] **Step 4: 更新 AGENTS.md 阶段**

切到"长期稳定运行观察"（再来一轮）：

```markdown
**当前阶段：长期稳定运行观察 / 工具：实机 smoke + runtime-audit + 高频路径自我观察**

> 2026-04-30 晚 完成菜单 + CLI 重设计 Phase 1-4 后再次进入观察期。
```

- [ ] **Step 5: Commit + push**

```bash
git add docs/STATUS.md docs/NEXT_STEP.md docs/PROGRESS.md AGENTS.md
git commit -m "$(cat <<'EOF'
docs: sync status to post-redesign reality

菜单 + CLI 重设计 Phase 1-4 完成后的状态同步：
- STATUS: 已完成能力补 m / host-proxy / log / 5 类顶层导航
- NEXT_STEP: 阶段切回观察期；下一闭环看 5 类 mental model 是否对齐
- PROGRESS: Round 2 记录
- AGENTS: 阶段切回观察期
EOF
)"
git push origin main
```

---

## Failure Modes Registry

| # | 失败模式 | 触发条件 | 影响 | 缓解 |
|---|---|---|---|---|
| 1 | m symlink 创建失败（权限） | 非 root 跑 install-self；目标目录只读 | 用户得敲 `minimalist menu`（11 字符） | install-self 已有 requireRoot；symlink 失败仅 warn 不 abort |
| 2 | host-proxy on 后宿主机失联 | iptables OUTPUT 链劫持失败 + 没有 console 兜底 | 宿主机 SSH 不通；要 IPMI 介入 | 二次确认 + warning 文案；off 路径快速可用；每次都重写 config 后 apply-rules |
| 3 | journalctl 不存在或 systemd 不可用 | 容器化部署 / 非 systemd 环境 | `m log` 失败 | Logs() 直接返回 journalctl 错误；用户清楚问题在哪 |
| 4 | 操作后回菜单导致死循环 | 业务方法持续返回 error 但 continue | 用户陷在子菜单 | 0) 返回 始终可用；error 通过 fmt.Fprintln 显示后继续 |
| 5 | 顶部 status header 调 systemctl 阻塞 | systemctl is-active 慢 | 菜单刷新慢 | renderStatusHeader 使用现有 commandOK helper（已有 timeout） |
| 6 | 测试覆盖率掉到 91% 以下 | Phase 4 大量测试调整后漏测分支 | 后续维护风险增加 | 每个 phase 末尾 coverprofile 快照核对；不达标补测试 |
| 7 | 删除二次确认默认 "y" 导致误删 | 拼写或思维定势按 y | 节点 / 订阅误删 | promptConfirm 默认 false（[y/N]）；空 enter = 取消 |
| 8 | configMenu 嵌套调用 routerWizard 时 stdin 缓冲冲突 | 共用 reader 但 wizard 内部消耗多行 | 后续菜单卡死 | 全部 sub-menu 都接受 reader 参数（不新建 NewReader）；现有 nodesMenu 已有此模式 |
| 9 | yaml.Marshal 把 secret 持久化时空白处理 | config.Save 实现细节 | 配置写丢字段 | 复用现有 config.Ensure 的 yaml round-trip 验证；新增 TestSaveRoundTrip |
| 10 | Phase 4 删除老 menu 函数后某 CLI 入口仍引用 | grep 漏检 | 编译失败 | 删除前 grep -rn 全仓；编译时立即暴露 |

---

## NOT in scope (this plan)

- TUI 重写（bubbletea / tview） — Phase 5 可选未来事项
- CLI all-in（删除 menu 命令） — Phase 5 可选未来事项
- REPL 模式 — Phase 5 之外
- 颜色 / 加粗输出 — 不引入 ANSI 处理库；视觉强调用 `★` 等纯文本符号
- 日志统一分级 helper（LOGD/LOGI/LOGE） — 留给后续如果需要再做；当前 fmt.Fprintln 够用
- DESIGN.md（视觉设计系统）— 不需要，CLI/TUI 工具不走 web 设计路径
- 国际化（i18n） — 主线就是中文 NAS 用户

---

## What already exists (don't re-implement)

| Need | Existing |
|---|---|
| 状态查询 | `Status()` (app.go:222), `Healthcheck()` (255), `RuntimeAudit()` |
| 节点 CRUD | `ListNodes/TestNodes/RenameNode/SetNodeEnabled/RemoveNode` |
| 订阅 CRUD | `ListSubscriptions/AddSubscription/SetSubscriptionEnabled/RemoveSubscription/UpdateSubscriptions` |
| 规则 CRUD | `Rule*Add/Remove/List`, `ACL*`, `RulesRepo*` |
| 配置持久化 | `config.Ensure`（已读取 + 创建）；`config.Save` 可能需要新增（见 Task 3.1 Step 3） |
| 实机命令封装 | `internal/system.Runner.Run / Output` |
| `proxy_host_output` 字段 | `internal/config/config.go:39`（功能已存在，仅缺 UX） |
| systemctl 服务管理 | `Start/Stop/Restart` 已封装 systemctl |
| iptables apply | `ApplyRules / ClearRules` |
| confirm helper | **新增**（`promptConfirm`）—— 现有 `promptBool` 是 0/1 不是 y/n |

---

## Self-Review Checklist

### Spec coverage
- [x] Phase 1: m symlink — Task 1.1
- [x] Phase 2 顶部 status header — Task 2.3, 2.4
- [x] Phase 2 操作后回菜单 — Task 2.5
- [x] Phase 2 promptConfirm + 删除二次确认 — Task 2.2, 2.6
- [x] Phase 3 host-proxy CLI — Task 3.1, 3.2
- [x] Phase 3 log CLI — Task 3.3, 3.4
- [x] Phase 4 5 类顶层重排 — Task 4.1-4.5
- [x] Phase 4 helptext 重组 — Task 4.6
- [x] 每阶段实机 smoke + push — Task 1.2, 2.7, 3.5, 4.7

### Placeholder scan
- 无 "TBD"/"TODO"/"implement later"
- 所有代码块都是完整实现，不是伪代码
- 测试代码都是可运行的 Go

### Type consistency
- `LogOptions` 定义在 Task 3.3，使用在 Task 3.4 — 一致
- `HostProxyStatus/Enable/Disable` 命名一致
- `configMenu/rulesMenu/logMenu/controlMenu` 命名一致
- `promptConfirm(reader, out, label, defaultYes)` 签名一致

### 已知风险点
- Task 2.4 - 2.5 现有测试调整量大（菜单编号 + stay-open 模式），实施时可能暴露更多需要修的旧测试。Buffer 充足时间。
- Task 4.5 删除 5 个老菜单函数后大约有 30+ 个测试需要更新。预算 1-2 小时调整测试。
- `config.Save` 可能不存在；如不存在，Task 3.1 Step 3 多一个 sub-task。

---

## GSTACK REVIEW REPORT

Review mode: `/autoplan`
Date: 2026-05-01
Base branch: `main`
Plan status: `DONE_WITH_CONCERNS`
Restore point: `/tmp/main-autoplan-restore-20260501-180033.md`

### Intake Summary

- 目标计划文件是这份菜单 + CLI 重设计方案，不是稳定性主线计划。
- 当前真实实现仍是旧 8 项裸数字菜单，首屏没有 header、没有 5 类重排、没有 `host-proxy`、没有 `log` 子命令。
- 本计划尾部却写了“Phase 1-4 完成后的状态同步”和 `[x]` 自检项，和代码现状直接冲突。

### Phase 1 — CEO Review

Premise gate:
- 已由用户本轮输入“菜单还是很烂，简陋的没法看”隐式确认。问题真实存在，不需要再验证是否值得做。

Premise challenge:
- “做得像 233boy/sing-box-yes”不是产品目标，只是参考物。
- 真正目标应是：把 5 个最高频运维动作的完成时间、输入长度、误操作风险降到最低。
- 当前计划默认 `Approach A`，但没有先用真实路径数据证明 menu-first 比 CLI-first 更适合当前用户。

What actually exists today:

| Need | Current reality |
|---|---|
| 顶层菜单 | `Menu()` 仍是 8 项裸打印，见 `internal/app/app.go:472` |
| 节点操作 | `nodesMenu()` 仍是“看节点 / 启用 / 禁用 / 删除”拆开的旧模型，见 `internal/app/app.go:1416` |
| 订阅操作 | `subscriptionsMenu()` 仍是旧模型，见 `internal/app/app.go:1468` |
| CLI 入口 | 仍无 `host-proxy`、`log` 分发，见 `internal/cli/cli.go:14-205` |
| 配置保存 | `config.Save` 已存在，计划尾部“可能不存在”是过期信息 |

Dream state delta:

```text
CURRENT
  裸数字菜单
  + 诊断入口分散
  + 高风险动作混层
  + 文档与计划状态失真

THIS PLAN
  试图一次性补 m/header/stay-open/log/host-proxy/5类导航
  但还没锁定状态模型、事务性、文档单一真相

12-MONTH IDEAL
  CLI-first or dual-track operator surface
  + 快速诊断
  + 危险动作分层
  + 文档一处为真
  + 故障时比现在更快恢复，不只是更顺手
```

Implementation alternatives review:

| Approach | Completeness | Verdict |
|---|---:|---|
| A. 继续按当前 Phase 1-4 一次性推进 | 4/10 | 拒绝，范围太大且状态定义没锁 |
| B. 缩成“诊断 + host-proxy + CLI 日常层”先落地 | 8/10 | 推荐，最贴近真实运维闭环 |
| C. 先做 menu-first / CLI-first / REPL-first 三方案对比，再实现 | 7/10 | 也合理，但会延后直接改造 |

CEO DUAL VOICES — CONSENSUS TABLE:

| Dimension | Codex | Subagent | Consensus |
|---|---|---|---|
| Premises valid? | No | No | CONFIRMED concern |
| Right problem to solve? | No | No | CONFIRMED concern |
| Scope calibration correct? | No | No | CONFIRMED concern |
| Alternatives sufficiently explored? | No | No | CONFIRMED concern |
| Competitive / moat framing covered? | No | No | CONFIRMED concern |
| 6-month trajectory sound? | No | No | CONFIRMED concern |

CEO completion summary:
- Score: `4/10`
- Verdict: 不应直接开工。
- Main call: 先把“更像 233boy”改写成“更快完成高频运维动作”，并把第一阶段收窄到真实闭环。

### Phase 2 — Design Review

Design scorecard:

| Dimension | Score | Main gap |
|---|---:|---|
| 信息层级 | 4/10 | 诊断入口被 header 吞掉 |
| 风险分层 | 3/10 | `controlMenu` 混合高危与日常动作 |
| 状态定义 | 2/10 | 没有 `unknown/error/partial` 枚举 |
| 空态 / 成功态 / 错误态 | 2/10 | 只写了“继续留在菜单里” |
| 文本 wireframe 具体度 | 5/10 | 有工程步骤，没有统一屏幕骨架 |
| CLI 语法冻结度 | 3/10 | `log -n 5` 与 parser 设计冲突 |
| 终端适配 | 4/10 | 没定义长输出、窄终端、分页策略 |

DESIGN DUAL VOICES — CONSENSUS TABLE:

| Dimension | Codex | Subagent | Consensus |
|---|---|---|---|
| Hierarchy sound? | No | No | CONFIRMED concern |
| Diagnostics preserved? | No | No | CONFIRMED concern |
| High-risk actions separated? | No | No | CONFIRMED concern |
| States fully specified? | No | No | CONFIRMED concern |
| UX spec concrete enough? | No | No | CONFIRMED concern |
| CLI syntax locked? | No | No | CONFIRMED concern |

Design completion summary:
- Score: `3/10`
- Verdict: 还不是可施工的 UX spec。
- Required fixes:
  - 恢复单独的“状态与诊断”面
  - 给 header、成功、失败、空态、部分成功补状态表
  - 给 5 个顶层菜单补文本 wireframe

### Phase 3 — Engineering Review

Architecture reality:

```text
current
  CLI/menu
    -> internal/app/app.go
        -> config/state/runtime/rulesrepo/system
        -> systemctl / iptables / ip / journalctl / controller HTTP

planned
  CLI/menu
    -> header/log/host-proxy/config/rules/control submenus
    -> still through internal/app/app.go
    -> same root-side effects

missing before implementation
  statusSnapshot()     # cheap local header read path
  hostProxyService()   # transactional toggle
  log snapshot API     # non-streaming, explicit limits
  readChoice()         # EOF-safe loop helper
```

Core engineering findings:
- `log -f` 与现有 `Runner.Output()` 抽象不兼容。当前 runner 只支持“进程退出后一次性返回”，不支持流式 follow。
- `host-proxy on/off` 不是事务性的。按计划执行会出现“配置已写、规则半成功”的脏状态。
- 顶部 header 不能复用完整 `Status()` 逻辑，否则 controller 不通时菜单会卡到超时。
- “操作后回菜单”会放大当前 `ReadString('\n')` 忽略 EOF 的问题，非交互或输入耗尽下可能死循环。
- 计划把 blast radius 说轻了。实际会同时改 `internal/app`、`internal/cli`、测试、README、flows、status 文档和 live smoke。
- 计划尾部的完成态是假的，会直接误导实施顺序和测试判断。

ENG DUAL VOICES — CONSENSUS TABLE:

| Dimension | Codex | Subagent | Consensus |
|---|---|---|---|
| Architecture sound? | No | No | CONFIRMED concern |
| Test coverage sufficient? | No | No | CONFIRMED concern |
| Performance / latency risks addressed? | No | No | CONFIRMED concern |
| Security / high-risk ops covered? | No | No | CONFIRMED concern |
| Error paths handled? | No | No | CONFIRMED concern |
| Deployment risk manageable? | No | No | CONFIRMED concern |

Test diagram:

```text
operator flows
  menu header render
    -> service state
    -> node readiness
    -> host proxy state
    -> controller unavailable fallback

  menu loops
    -> list action stays in submenu
    -> invalid choice
    -> EOF / stdin exhausted

  host-proxy
    -> enable success
    -> enable render failure rollback
    -> enable apply failure rollback
    -> disable success
    -> cutover blocked

  logs
    -> snapshot success
    -> unknown arg
    -> missing journalctl
    -> timeout
    -> follow unsupported or re-architected
```

Test plan artifact:
- [2026-05-01-menu-cli-autoplan-test-plan.md](/home/projects/minimalist/docs/reviews/2026-05-01-menu-cli-autoplan-test-plan.md:1)

Engineering completion summary:
- Score: `3/10`
- Verdict: 不能直接施工。
- Blockers:
  - 删掉或重做 `log -f`
  - 先设计 `host-proxy` 事务边界
  - 先抽 `statusSnapshot()` 和 `readChoice()`

### Phase 3.5 — DX Review

Developer journey map:

| Stage | Current plan state |
|---|---|
| Find entrypoint | Better for熟手，但仍有 `menu` / CLI / README / flows 多入口 |
| Understand first action | 未定义 hello world |
| Try common command | `m` 缩短输入，但不是 canonical safe path |
| Recover from error | 仍缺 problem/cause/fix/docs contract |
| Find diagnostics | 计划把诊断入口压扁了 |
| Override defaults | `host-proxy` 缺 dry-run / staged apply |

DX scorecard:

| Dimension | Score |
|---|---:|
| TTHW clarity | 2/10 |
| CLI ergonomics | 5/10 |
| Error messages | 2/10 |
| Docs single source of truth | 3/10 |
| Escape hatches | 3/10 |
| Upgrade / migration safety | 5/10 |
| Daily operator speed | 6/10 |
| Consistency | 4/10 |

DX DUAL VOICES — CONSENSUS TABLE:

| Dimension | Codex | Subagent | Consensus |
|---|---|---|---|
| Getting started under 5 min? | No | No | CONFIRMED concern |
| API/CLI naming guessable? | Partial | No | CONFIRMED concern |
| Error messages actionable? | No | No | CONFIRMED concern |
| Docs findable and complete? | No | No | CONFIRMED concern |
| Escape hatches safe enough? | No | No | CONFIRMED concern |
| Daily operator layer well scoped? | Partial | Partial | CONFIRMED concern |

DX completion summary:
- Score: `4/10`
- Verdict: 这是熟手优化，不是完整 DX 方案。
- Required fixes:
  - 定义 hello world 和 TTHW
  - 确立一页 operator source of truth
  - 把 `m` 从 canonical 改成 optional shortcut
  - 为新增命令统一错误 contract

### Cross-Phase Themes

1. Plan state drift
   - 计划尾部把大量未做功能写成已完成，CEO、Eng、DX 都认为这会污染后续执行。

2. Diagnostics got squeezed out
   - 设计层、工程层、DX 层都指出：header 不能代替 `status + healthcheck + runtime-audit`。

3. High-risk operations are under-modeled
   - `host-proxy` 的事务性、风险分层、dry-run、回滚在设计、工程、DX 三层都被独立打回。

4. The plan is over-specific in mechanics and under-specific in states
   - 有很多代码块，但没有把状态模型、失败路径、文档真相锁死。

## User Challenges

### Challenge 1 — 不要按当前 Phase 1-4 一次性开工

You said:
- 想把菜单和 CLI 做到成熟脚本那种顺手水平。

Both models recommend:
- 先不要按当前完整计划施工，先收窄成“诊断层 + 日常控制层 + host-proxy 安全边界”。

Why:
- 当前计划范围过大，状态模型不完整，还把大量未做功能写成已完成。

What context we might be missing:
- 你可能已经非常确定自己就是想保留 menu-first，而不是 CLI-first。

If we're wrong, the cost is:
- 会延后你拿到一个更好看菜单的时间。

### Challenge 2 — 保留独立诊断面，不要让 header 替代诊断

You said:
- 顶部 status header + 5 类导航。

Both models recommend:
- 必须保留显式“状态与诊断”面，至少容纳 `status`、`healthcheck`、`runtime-audit`。

Why:
- header 只能是摘要，不能承担完整故障诊断。

What context we might be missing:
- 你也许愿意把诊断全部转成 CLI，不想保留菜单项。

If we're wrong, the cost is:
- 会多保留一个菜单面，导航不如 5 类方案简洁。

### Challenge 3 — `log -f` 不应按现抽象直接进入首轮

You said:
- 新增 `log` 独立命令。

Both models recommend:
- 先只做 snapshot log，`-f` 要么推迟，要么先扩 streaming runner。

Why:
- 现有 `Runner.Output()` 是一次性返回，不支持长期 follow。

What context we might be missing:
- 你可能接受为了 `-f` 一次性改 `internal/system` 抽象。

If we're wrong, the cost is:
- 首轮日志命令能力会弱一点。

## Taste Decisions

1. `menu-first` 还是 `CLI-first + 精简 menu`
   - Recommendation: 后者。当前用户画像本来就能接受 CLI，menu 更适合作为入口而不是全部主战场。

2. `m` 是否作为文档 canonical
   - Recommendation: 不要。保留 `minimalist` 为 canonical，`m` 只是 shortcut。

3. 顶层导航是否坚持 5 类
   - Recommendation: 可以保持 5 类，但其中一类必须是“状态与诊断”，不能只有“日志”。

## Decision Audit Trail

| # | Phase | Decision | Classification | Principle | Rationale | Rejected |
|---|---|---|---|---|---|---|
| 1 | CEO | 不按当前 Phase 1-4 一次性推进 | User Challenge | P1 + P2 | 范围大、状态模型不完整、计划状态失真 | “文档已足够，可直接开工” |
| 2 | CEO | 把目标从“像 233boy”改成“缩短高频运维路径” | Mechanical | P3 + P5 | 更贴近 PRD 和真实用户闭环 | Shell parity 目标 |
| 3 | Design | 恢复单独诊断入口 | User Challenge | P1 | header 不能代替诊断 | 只保留 header + log |
| 4 | Design | 给 header/成功/失败/空态补状态表 | Mechanical | P1 | 当前状态设计缺失 | 边做边补 |
| 5 | Eng | 首轮移除或重做 `log -f` | User Challenge | P5 | 现 runner 不支持流式 follow | 继续基于 `Output()` 硬做 |
| 6 | Eng | `host-proxy` 必须事务化 | Mechanical | P1 + P5 | 配置和 live 规则不能分裂 | save -> render -> apply 直推 |
| 7 | Eng | 先抽 `statusSnapshot()` 和 `readChoice()` | Mechanical | P5 | 先减耦合，再重排菜单 | 直接往 `app.go` 继续加分支 |
| 8 | DX | README 不再把 `m` 当 canonical | Taste | P3 | alias 是 best-effort，不该成为主路径 | 全文切 `m` |
| 9 | DX | 定义单一 operator source of truth | Mechanical | P4 | 避免 README / FLOWS / runbook 三套口径 | 多页并行维护 |
