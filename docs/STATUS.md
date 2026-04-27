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

当前保留命令：

- 核心主路径：`install-self`、`setup`、`render-config`、`start`、`stop`、`restart`
- 运维查看：`status`、`show-secret`、`healthcheck`、`runtime-audit`
- 交互与资源入口：`menu`、`router-wizard`、`import-links`
- 规则与订阅：`nodes`、`subscriptions`、`rules`、`acl`、`rules-repo`

## 质量状态

- `go build` 已覆盖当前主入口。
- `GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./...` 作为当前全量回归入口。
- 当前测试已经覆盖配置、状态、provider、rules-repo、runtime 渲染、核心 app 命令、CLI 分发、错误透传、路径阻塞、菜单/helper 边界与 system runner。
- 最近一轮补强重点已经从“补单元测试红灯”转为“收口命令链路失败路径和真实 smoke 前置条件”。

## 本机真实验证结论

- 这台 Debian NAS 本身就是实机环境，`systemd` 可达，但当前 `systemd is-system-running` 结果是 `degraded`。
- 当前真正运行的是旧的 `mihomo.service`，不是 `minimalist.service`。它已 `enabled` 且 `active`，`ExecStart=/usr/local/bin/mihomo-core -d /etc/mihomo`，并通过 `ExecStartPre=/usr/local/bin/mihomo apply-rules`、`ExecStopPost=/usr/local/bin/mihomo clear-rules` 管理规则。
- 这台机子上 `/usr/local/bin/minimalist`、`/etc/minimalist`、`/var/lib/minimalist` 目前都不存在，`minimalist.service` 也不存在。
- 现网 `iptables` / `ip rule` 已有真实的 MIHOMO 透明代理状态：`MIHOMO_PRE`、`MIHOMO_PRE_HANDLE`、`MIHOMO_OUT`、`MIHOMO_DNS` 都在，`fwmark 0x2333 lookup 233` 规则和 table `233` 也都在。
- 之前那轮“系统调用不可用”的判断只代表 Codex 沙箱限制，不代表这台 NAS 的真实宿主机状态。
- 只读清点确认 `/usr/local/bin/mihomo` 指向 `/usr/local/lib/mihomo-manager/mihomo`，后者是 shell 版 `mihomo v0.6.0`，不是 Go 版 `minimalist`。
- 旧 `/etc/mihomo` 同时包含 runtime `config.yaml`、`router.env`、`settings.env`、`state/*.json`、provider、ruleset、UI 与 geodata；这不是 Go 版 `/etc/minimalist/config.yaml` + `/var/lib/minimalist/state.json` 的同构目录。
- Go 版目标 unit 当前会使用 `/var/lib/minimalist/mihomo` 作为 `mihomo-core -d` 运行目录；旧 unit 使用 `/etc/mihomo`。
- Go 版与旧 shell 版默认都会操作 `MIHOMO_*` 链名、`0x2333` mark 与 table `233`，所以未切换服务归属前运行 Go 版 `apply-rules` / `clear-rules` 会与现网旧服务争用同一组规则。

## 当前风险与限制

- 这台 NAS 现在跑的是 legacy `mihomo.service`，不能直接当作 `minimalist.service` 的验收环境。
- 在未确认迁移策略前，不要清理或重写现网 `MIHOMO_*` 规则，以免打断正在运行的透明代理。
- 后续如果要验收 Go 版 `minimalist`，需要先把 live install 从 `/etc/mihomo` / `mihomo.service` 迁到 `/etc/minimalist` / `minimalist.service`。
- 旧版本 `settings.env` / `router.env` / `state/*.json` 不兼容，不做迁移。
- 不恢复 `alpha/stable` 核心通道切换、core 回滚、自动同步安装目录、自定义更新/重启定时器和 `external-controller-tls`。
