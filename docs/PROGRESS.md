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
