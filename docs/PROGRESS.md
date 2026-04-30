# 进度日志

> 2026-04-30 重新开始 Round 0 循环。本轮之前的工作历程概要见下方"项目历程概要"。
> 详细每轮记录已不再回填；如需精确历史请查 `git log` 与 `docs/DECISIONS.md`。

## 项目历程概要（Round 0 之前）

### 2026-04-22 ~ 2026-04-26：从旧 shell/Python 切到 Go 版主实现
- 旧 `mihomo-manager`（shell + Python `statectl.py` / `rulepreset.py`）从主树移除。
- Go 版 `cmd/minimalist` + `internal/{app,cli,config,state,provider,rulesrepo,runtime,system}` 目录结构落地。
- 主命令名从 `mihomo` 切到 `minimalist`。

### 2026-04-27：核心契约前移
- `clear-rules` 删除失败必须上浮（详见 DECISIONS.md）。
- `setup/start/restart/apply-rules/clear-rules` 在 legacy live + Go 版未 active 时硬阻断。

### 2026-04-28：现网切换 + 内核升级入口 + autoplan 评审
- 本机从旧 `mihomo.service` 切到 Go 版 `minimalist.service`：4 个手动节点导入、geodata/UI 资源就地复制、旧资产清理。
- `core-upgrade-alpha` 入口落地：仅显式单次官方 alpha 升级，自动重启服务，不做 stable/rollback/定时。
- `runtime-audit` 输出拆成 `alerts-24h` / `alerts-recent` / `fatal-gaps`，区分历史噪音与当前异常。
- 连续多轮 `internal/app` focused 测试硬化，覆盖率推到 97%+。
- 用 `/autoplan` 评审了 `docs/superpowers/plans/2026-04-28-long-term-stability-hardening.md`，关键结论是 runtime asset 校验必须接到 systemd `ExecStartPre`。
- `ensureRuntimeAssetsReady` / `VerifyRuntimeAssets` / 隐藏 CLI `verify-runtime-assets` 落地，systemd unit `ExecStartPre` 接上预检。

### 2026-04-29：CLI 表面收口 + 实机 smoke 完成
- `minimalist nodes test` 在非交互 CLI 暴露；`verify-runtime-assets` 在 `--help` 显式列出。
- 完成 service restart smoke 与 host reboot smoke：`minimalist.service` 在两种重启路径下都能恢复 `active/enabled`，controller、`MIHOMO_PRE`/`MIHOMO_DNS` 链、`fwmark 0x2333 lookup 233` 与 table 233 全部稳定。
- 主线从"代码面收口"进入"长时间观察"阶段。

---

## Round 0: 项目初始化

### 完成
- 初始化执行进度日志。

### 测试状态
- 通过: 未运行 / 总计: 未运行

### 遗留 / 下轮继续
- 后续按每轮实际执行追加记录。

### 下轮目标
- 根据 docs/TASKS.md 或线上故障继续推进。

## Round 1 — 2026-04-30 02:34

### 完成
- 排查 7890 HTTP / SOCKS5 代理连不上问题。
- 确认 `mihomo-core` 已监听 `*:7890`，controller 正常，LAN 请求已进入 mihomo。
- 定位根因：有可用节点时 `PROXY` 组默认顺序为 `DIRECT, AUTO`，服务重启后 `MATCH,PROXY` 默认走直连，导致被墙目标超时。
- 将有可用 provider 时的 `PROXY` 组顺序改为 `AUTO, DIRECT`，保留无 provider 时 `DIRECT` 兜底。
- 重新安装 `/usr/local/bin/minimalist`，渲染 `/var/lib/minimalist/mihomo/config.yaml`，并重启 `minimalist.service`。

### 测试状态
- 通过: focused runtime tests、`go test ./...`、`go vet ./...`、`gofmt -l cmd internal`、实机 HTTP/SOCKS5 7890 smoke / 总计: 5 组

### 遗留 / 下轮继续
- 继续观察 `runtime-audit` 的 `fatal-gaps=0` 和 7890 客户端实际使用情况。

### 下轮目标
- 若 7890 再次异常，优先检查 `PROXY.now`、节点 delay 和最近 5 分钟 journal。

## Round 2 — 2026-05-01 01:24

### 完成
- 排查 Windows 客户端在 Tailscale / ZeroTier 常驻时访问 `192.168.2.220:7890` 代理握手被关闭的问题。
- 确认服务端 `minimalist.service`、controller、`PROXY.now` 与 7890 监听均正常，失败形态符合 `lan-allowed-ips` 来源白名单拒绝。
- 新增 `access.lan_allowed_cidrs`，让显式代理端口白名单可独立放行远程可信网段，不污染旁路由真实 `network.lan_cidrs`。
- 实机 `/etc/minimalist/config.yaml` 已加入 `100.64.0.0/10` 与 `10.156.67.0/24`，并重新安装、渲染、重启服务。

### 测试状态
- 通过: focused runtime tests、`go test ./...`、`go vet ./...`、`gofmt -l cmd internal`、Tailscale / ZeroTier 7890 HTTP smoke / 总计: 5 组

### 遗留 / 下轮继续
- Windows 客户端应优先使用 `100.118.67.82:7890` 或 `10.156.67.142:7890` 再复测。

### 下轮目标
- 若 Windows 仍失败，抓取 Windows `ipconfig` 与两条 `curl -v` 输出，对照 `lan-allowed-ips` 来源网段继续排查。

## Round 3 — 2026-05-01 02:10

### 完成
- 将订阅能力在用户可见入口中正式降级为增强项：状态输出改为 `订阅(增强项)`，帮助信息把 `subscriptions` 移到 enhancement commands，菜单标注 `订阅管理（增强项）`。
- 保留订阅已有实现与 runtime 渲染能力，但文档明确 `setup` / `start` 的核心成功路径只看启用的手动节点，订阅缓存不能替代手动节点。
- 更新 `README.md`、`docs/README_FLOWS.md`、`docs/ARCHITECTURE.md`、`docs/STATUS.md` 与 `docs/TASKS.md`，关闭 P2 订阅降级任务。

### 测试状态
- 通过: focused app/cli tests、`go test ./...`、`go vet ./...`、`gofmt -l cmd internal` / 总计: 5 组

### 遗留 / 下轮继续
- P2 剩余任务仍是 `core-upgrade-alpha` 失败自动回滚、amd64 CPU-level 资产策略，以及默认规则仓库双份维护风险。

### 下轮目标
- 继续处理下一个 P2 增强项，优先选择不会影响当前 live 稳定性的文档或纯测试闭环。

## Round 4 — 2026-05-01 02:28

### 完成
- 为 `core-upgrade-alpha` 增加替换后重启失败的自动回滚：恢复 `.bak` 到 `core_bin`，并再次重启 `minimalist.service`。
- 补充回滚保护测试：重启失败会恢复旧 core 并消耗 `.bak`；恢复失败时保留 `.bak` 并在错误中输出备份路径。
- 更新 README、ARCHITECTURE、README_FLOWS、STATUS 与 TASKS，关闭 P2 `core-upgrade-alpha` 失败自动回滚任务。

### 测试状态
- 通过: focused core-upgrade tests、`go test ./...`、`go vet ./...`、`gofmt -l cmd internal` / 总计: 4 组

### 遗留 / 下轮继续
- P2 剩余任务：`core-upgrade-alpha` 支持 amd64 CPU-level 资产、默认规则仓库双份维护风险。

### 下轮目标
- 继续 P2 `core-upgrade-alpha` 支持 amd64 CPU-level 资产。

## Round 5 — 2026-05-01 02:45

### 完成
- 新增 `install.core_amd64_cpu_level` 配置字段，用于显式选择 `core-upgrade-alpha` 的 amd64 CPU-level 资产。
- `core-upgrade-alpha` 在 `amd64` 上支持 `compatible` / `v1` / `v2` / `v3` 等显式资产选择；未配置时继续拒绝猜测，保持安全默认。
- 补充选择逻辑、完整升级流程和配置 roundtrip 测试；更新 README、ARCHITECTURE、README_FLOWS、STATUS 与 TASKS。

### 测试状态
- 通过: focused app/config tests、`go test ./...`、`go vet ./...`、`gofmt -l cmd internal` / 总计: 4 组

### 遗留 / 下轮继续
- P2 剩余任务：默认规则仓库双份维护风险。

### 下轮目标
- 处理默认规则仓库双份维护风险。

## Round 6 — 2026-05-01 02:58

### 完成
- 删除仓库根部 `rules-repo/default` 镜像样本，避免它继续与 `internal/rulesrepo/assets/default` 漂移。
- 明确 `internal/rulesrepo/assets/default` 是内置默认规则仓库唯一源；运行时仍由 `InitDefaultRepo` 复制到 `/etc/minimalist/rules-repo/default/`。
- 更新 ARCHITECTURE、STATUS、TASKS 与 PROGRESS，关闭 P2 默认规则仓库双份维护风险。

### 测试状态
- 通过: `go test ./internal/rulesrepo`、`go test ./...`、`go vet ./...`、`gofmt -l cmd internal` / 总计: 4 组

### 遗留 / 下轮继续
- 当前 TASKS.md 已无未完成任务。

### 下轮目标
- 进入收尾复查或按新指令继续。
