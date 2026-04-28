# 下一步

## 当前阶段

- Go 版 `minimalist` 主实现已经落地，默认分支保持可构建、可测试。
- 当前代码面已经完成三件关键收口：
  - 手动节点是唯一旁路由核心成功路径
  - DNS / host-safe 默认姿态已被 tests 固定
  - help / runbook 已补齐到当前运维真相，模板层次文档已在前一轮对齐
- 单元与 focused 测试已经覆盖核心配置、状态、provider、rules-repo、runtime 渲染、app 命令编排、CLI 分发与多组失败路径；历史收口过程中曾多次拉升 `internal/app` / `internal/config` 覆盖率，但当前判断主线不再以覆盖率数字为目标。
- 这台 Debian NAS 已经是可用实机：`systemd`、`iptables`、`ip rule` 都是真实可达的。
- 现网已经从旧 `mihomo.service` 切换到 Go 版 `minimalist.service`；旧服务当前 `inactive/disabled`，新服务 `active/enabled`。
- 本轮全量 `go test ./...` 和 build 已通过；新增 `core-upgrade-alpha` focused app/CLI tests 也已通过。此前实机 `healthcheck` / `runtime-audit` / systemd / ip rule / route table smoke 已通过；十轮连续硬化后复验仍保持同样结果。

## 下一最小闭环

- 当前没有新的主路径功能缺口；下一步优先做实机 restart / reboot smoke，不扩协议、不恢复旧运维能力。
- 当前主线从“补剩余低覆盖率热点”切到“长期稳定运行达标口径”。判断标准不再是多几个 focused tests，而是：
  - `minimalist.service` 经过冷启动 / 重启 / 宿主机 reboot 后都能稳定回到 `active/enabled`
  - `/var/lib/minimalist/mihomo/` 的 `Country.mmdb`、`GeoSite.dat`、`ui/` 缺失时有明确 fail-fast 或修复指引，不靠人工记忆补文件
  - `runtime-audit` 能区分历史噪音与当前故障，`warn/error` 计数不再因为切换早期历史窗口持续误导判断
  - `core-upgrade-alpha` 保持“显式、高风险、人工执行”定位；升级失败或升级后服务异常时有可执行恢复路径
  - 连续观察窗口内没有新的未知重启、规则丢失、controller 不可达或 provider false-ready 误判
- 本次十个最小闭环已补上 `menu -> ImportLinks` / `networkMenu -> RouterWizard` 共享 reader、索引输入空回车保护、`controllerRequest` path/空白归一化、unsupported subscription cache readiness 误判，以及 `core-upgrade-alpha` 的空 payload、stderr 版本回退和 release API 错误详情；这些都已经通过 `go test ./...`、`go test ./internal/app -coverprofile=/tmp/minimalist-app.cover` 与 `go build` 复验。
- 本轮十个最小闭环已补上 `controllerRuntimeSummary` / `controllerConfigMode` 的 HTTP 失败判定、`testNodeDelay` 的 missing/negative delay 保护、`restartMinimalistServiceAfterCoreUpgrade` 的 active 状态校验、`releaseIsNewer` 的自然排序，以及 nodes / subscriptions / network / rules 菜单索引校验；这些都已经通过 `go test ./...` 和 `go build` 复验。
- 当前轮次已额外补稳 runtime asset、menu/CLI 节点管理、controller delay 错误输出、空白 rules manifest、空白 secret 持久化、legacy state version 回填、`testNodeDelay` 失败分支、`networkMenu` 关键分发、安装路径阻断、导入读取错误、非法规则目标、`ensureAll` 失败传播、`updateSubscriptions` mixed/disabled 边界、`apply-rules` 空 bypass 输入、菜单 `0` 返回，以及 config/state/rules-repo/provider/CLI 的一组 focused 错误路径。
- 当前轮次又补稳了 `Setup` root guard、`Status` ensureAll 失败传播、`ImportLinks` / `RouterWizard` / `UpdateSubscriptions` 写回失败，以及 `Menu -> nodes/network/rules/service/audit` 主分发；`internal/app` 覆盖率已从 96.3% 提升到 97.1%。
- 本次再连续十轮补稳了 `nodesMenu -> TestNodes`、`subscriptions/network/service/audit` 的 invalid-choice retry、`rulesAndACLMenu -> List ACL`、`hasReadyProviders` 边界、controller `mode` 缺失键、订阅更新非法 URL 与缓存写入失败记录，以及 `ensureAll` 的 rules-repo 初始化失败传播；该轮覆盖率快照曾提升到 97.8%。
- 本次新增 `core-upgrade-alpha` 最小闭环：显式从官方 `MetaCubeX/mihomo` alpha release 升级 `/usr/local/bin/mihomo-core`，成功替换后自动重启 `minimalist.service`；不新增菜单入口、不做 stable 通道、不提供 rollback 命令、不做定时自动更新。
- 若继续施工，优先选择：
  - 继续观察 `minimalist.service` 24 小时日志；2026-04-28 08:34 CST 已确认 UI/geodata 资源复制后最近启动窗口不再出现启动下载错误，当前 warn/error 计数仍来自切换早期历史窗口。
  - `runtime-audit` 收口已完成：当前已把 24 小时粗粒度 `warn/error` 计数拆成可区分“历史窗口 / 当前窗口 / 致命缺口”的信号。
  - 按新的 runbook 实际执行一轮 service restart smoke，并把结果回写 `docs/STATUS.md`。
  - 再按同一 runbook 执行一轮 host reboot smoke，并确认 `minimalist.service`、controller、iptables 规则、`ip rule` 与 route table 都自动恢复。
  - 最后才回头补 `internal/app` / `core-upgrade-alpha` 的剩余低覆盖尾分支；稳定性闭环优先级高于继续追 coverage。
- 旧 `/etc/mihomo`、`mihomo.service`、`/usr/local/bin/mihomo` 与 `/usr/local/lib/mihomo-manager` 已清理；下一步不再围绕旧服务回滚路径推进。
- 保持 README / flows 描述 Go 版 `minimalist` 目标真相；STATUS / NEXT_STEP 记录 live host 已切换完成。

## 本轮不做

- 不做旧状态迁移兼容。
- 不引入 alpha/stable 切换、自同步、回滚 core 等旧运维能力；只保留显式 `core-upgrade-alpha` 单次官方 alpha 升级。
- 不扩 `external-controller-tls`。

## 退出条件

- README 与权威文档只描述 Go 版 `minimalist` 当前真相。
- `go test ./...` 覆盖核心命令与系统编排关键路径，且当前轮次的 focused 验证已通过。
- Go 版高风险命令在 legacy live install 存在时已有 guard，人工 cutover 步骤已文档化，且本机 cutover 已完成。
- 当前轮次全量验证与实机 smoke 已完成；后续进入切换后观察与小步质量硬化。
- “长期稳定运行”退出条件不是今天能跑，而是上述稳定性口径全部过线后，主线才能视为收口。
