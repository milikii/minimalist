# 当前状态

## 当前主线

- 当前主实现已经切到 Go 版 `minimalist`，旧 shell / Python 主入口已从主树清理。
- 当前定位仍是 Debian NAS / IPv4 旁路由 / `iptables + TProxy`，不承诺 OpenWrt、nftables 抽象或双栈模板。
- 当前主命令名：`minimalist`
- 配置真相：`/etc/minimalist/config.yaml`
- 状态真相：`/var/lib/minimalist/state.json`
- 运行产物：`/var/lib/minimalist/mihomo/`

## 已完成能力

- 单二进制 CLI 入口：`cmd/minimalist`
- 配置与状态真相：`internal/config`、`internal/state`
- provider 导入、订阅扫描与渲染：`internal/provider`
- 默认规则仓库初始化、搜索、增删与渲染：`internal/rulesrepo`
- 运行时配置、provider、rules、systemd unit 与 sysctl 文本生成：`internal/runtime`
- 业务命令、菜单与 CLI 分发：`internal/app`、`internal/cli`
- 外部命令封装：`internal/system`
- `menu` 已按运维任务分组：状态总览、部署/修复、节点管理、订阅管理、网络入口与规则仓库、规则与 ACL、服务管理、健康检查与审计

当前保留命令：

- 核心主路径：`install-self`、`setup`、`render-config`、`start`、`stop`、`restart`
- 运维查看：`status`、`show-secret`、`healthcheck`、`runtime-audit`、`cutover-preflight`、`cutover-plan`
- 交互与资源入口：`menu`、`router-wizard`、`import-links`
- 规则与订阅：`nodes`、`subscriptions`、`rules`、`acl`、`rules-repo`

## 质量状态

- `go build` 已覆盖当前主入口。
- `GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./...` 作为当前全量回归入口。
- 当前测试已经覆盖配置、状态、provider、rules-repo、runtime 渲染、核心 app 命令、CLI 分发、错误透传、路径阻塞、菜单/helper 边界与 system runner，并补上了 config/state/provider/system 的关键错误路径、missing-file 分支、订阅输入空值保护、订阅启停分流、订阅删除缓存清理失败、订阅节点 provider-managed 保护、手动节点删除前引用检查、节点重命名空值保护、规则输入 kind / pattern 校验、CLI/app 终端判断、runtime layout 阻塞、runtime rule read error、provider URI fallback、rules-repo 校验边界、controller body read error、`apply-rules` 的关键失败传播、provider 过滤边界、config 随机 secret 回退、state existing-state 复用，以及 config/state/provider 的嵌套父目录创建。
- 最近十个小闭环继续收口 runtime / provider / rules-repo / config / state / app / cli 的 focused coverage：runtime asset 本地预置约束、CLI 节点管理分发、节点菜单校验错误透传、节点测速 controller 错误输出、订阅纯空输入、空白 rules manifest、空白 secret 持久化回写、legacy state 缺失 version 回填，以及此前的订阅空缓存与禁用订阅缓存不生成 provider、runtime 规则文件读错误上浮、CRLF wrapped base64 订阅解码、不支持 URI scan row 回退字段、provider 渲染父目录创建、rules-repo 按序号删除保序、ruleset entries 计数输出、config/state `Ensure` 首次创建父目录，当前都已有 focused tests。
- 2026-04-28 新增一组 focused app tests，继续补稳 `testNodeDelay` 与 `networkMenu` 的关键分发和失败路径；当前 `internal/app` 包覆盖率已提升到 91.8%。

## 本机真实验证结论

- 这台 Debian NAS 本身就是实机环境，`systemd` 可达，但当前 `systemd is-system-running` 结果是 `degraded`。
- 本机已在 2026-04-28 00:14 CST 完成人工 cutover：旧 `mihomo.service` 已 `inactive/disabled`，Go 版 `minimalist.service` 已 `active/enabled`。
- 当前运行入口为 `/usr/local/bin/minimalist`、`/etc/minimalist/config.yaml`、`/var/lib/minimalist/state.json` 与 `/var/lib/minimalist/mihomo/`。
- Go 版服务当前以 `/usr/local/bin/mihomo-core -d /var/lib/minimalist/mihomo` 运行，`ExecStartPre=/usr/local/bin/minimalist apply-rules` 已成功执行。
- 为避免启动阶段依赖外网下载，已把旧 runtime 中的 geodata 与 UI 资源复制到 `/var/lib/minimalist/mihomo/`；不复制旧 env/state 真相，不提交任何节点、secret 或订阅内容。
- 旧状态中 4 个手动节点已导入 Go 版 state 并启用；当前没有订阅。
- `minimalist healthcheck` 已通过，controller 返回 `{"meta":true,"version":"alpha-c59c99a"}`。
- `minimalist status` 已确认当前模式来自 runtime，服务 `active=true enabled=true`，手动节点数为 4。
- `minimalist runtime-audit` 已确认 `providers-ready=true`、`cutover-ready=true`；其中 warn/error 计数包含切换过程中 UI/geodata 缺失导致的历史日志，复制资源并重启后最近 2 分钟 `journalctl -u minimalist.service` 无新增日志。
- 2026-04-28 08:34 CST 复查 `minimalist.service`：服务仍 `active/enabled`，controller 返回 `{"meta":true,"version":"alpha-c59c99a"}`，最近一次 00:21 启动后日志只有正常初始化与 `UI already exists, skip downloading`；24 小时 `warn=5 error=12` 仍只来自 00:07-00:14 切换早期 MMDB/UI 缺失与下载超时历史窗口。
- 当前路由状态已由 Go 版服务接管：`fwmark 0x2333 lookup 233` 存在，table `233` 为 `local default dev lo scope host`，`mangle PREROUTING` 已跳转 `MIHOMO_PRE`，`nat MIHOMO_DNS` 已接入 `bridge1`。
- 旧 `mihomo.service` unit、旧 `/etc/mihomo`、旧 `/usr/local/bin/mihomo` 与旧 `/usr/local/lib/mihomo-manager` 已按人工确认清理；`/usr/local/bin/mihomo-core` 保留为 Go 版底层内核。
- 旧服务快速回滚入口已移除；`cutover-plan` 在旧资产不存在时会输出 `rollback: unavailable; legacy mihomo assets are not present`。
- 本轮最终验证结果：`GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./...`、`GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go build -o /tmp/minimalist-build-check ./cmd/minimalist`、实机 cutover smoke 全部通过；当前轮次新增的 `internal/app` focused tests 也已通过。

## 当前风险与限制

- `runtime-audit` 的 24 小时 warn/error 计数短期内仍会包含本次切换早期 UI/geodata 缺失产生的历史日志；按 2026-04-28 08:34 CST 观察，资源预置后的最近启动窗口未再新增同类错误。
- 当前 guard 只负责阻断误操作；仍不提供自动 cutover、自动回滚或旧配置迁移命令。
- 旧服务资产已清理，后续不再依赖旧 `mihomo.service` 作为回滚路径。
- 旧版本 `settings.env` / `router.env` / `state/*.json` 不兼容，不做迁移。
- 不恢复 `alpha/stable` 核心通道切换、core 回滚、自动同步安装目录、自定义更新/重启定时器和 `external-controller-tls`。
