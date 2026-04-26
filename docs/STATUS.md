# 当前状态

## 当前主线

- 项目已经开始从 `mihomo` shell/Python 版本切换到 Go 版 **`minimalist`**。
- 当前仓库里已经落下新的 Go 模块、主 CLI、配置/状态真相、rules-repo 资产、provider 渲染和基础测试。
- 旧 shell / Python 主实现已从主树中清理，当前仓库只保留 Go 版 `minimalist` 主线。

## 当前真相

- 当前主命令名：`minimalist`
- 当前定位：Debian NAS / IPv4 旁路由 / `iptables + TProxy`
- 主配置文件：`/etc/minimalist/config.yaml`
- 主状态文件：`/var/lib/minimalist/state.json`
- 运行产物目录：`/var/lib/minimalist/mihomo`
- 当前保留能力：
  - `install-self`
  - `setup`
  - `render-config`
  - `start` / `stop` / `restart`
  - `status`
  - `show-secret`
  - `healthcheck`
  - `runtime-audit`
  - `import-links`
  - `router-wizard`
  - `menu`
  - `nodes`
  - `subscriptions`
  - `rules`
  - `acl`
  - `rules-repo`

## 当前实现结论

- Go 版已实现单二进制 CLI 入口：`cmd/minimalist`
- Go 版已实现用户配置与程序状态真相：
  - `internal/config`
  - `internal/state`
- Go 版已实现订阅解码、URI 扫描和 provider 渲染：
  - `internal/provider`
- Go 版已实现默认规则仓库初始化与规则操作：
  - `internal/rulesrepo`
- Go 版已实现运行时产物渲染、systemd unit 与 sysctl 文本生成：
  - `internal/runtime`
- Go 版已实现命令编排与菜单入口：
  - `internal/app`
  - `internal/cli`
- Go 版已补最小测试护栏：
  - `internal/config` round-trip
  - `internal/provider` scan / render
  - `internal/rulesrepo` render / search
  - `internal/app` import-links / render-config / subscriptions update / setup / start / restart / healthcheck / runtime-audit / menu / router-wizard / clear-rules / apply-rules
  - `internal/cli` top-level `Run(args)` / `help` / `-h` / `show-secret` / rules-repo / nodes / subscriptions / rules / acl helper / usage error / index error / unknown subcommand / 正向分发
  - `internal/system` command runner
  - `internal/runtime` paths helper / `EnsureLayout` / `RenderFiles` / `writeRules` / secret fallback / configured secret / external-controller / external-ui / nameserver-policy / DNS 默认静态段落 / profile / fallback-filter / proxy-server-nameserver / nameserver / geox-url / dns.listen / lan-allowed-ips / lan-disallowed-ips / allow-lan / bind-address / log-level / mixed-port / tproxy-port / mode / ipv6 / geo flags / DNS behavior flags / manual & subscription provider / provider health-check / direct-only & AUTO proxy-groups / rules section & order / auth omission / BuildServiceUnit / BuildSysctl / service hardening / install target / core bin
  - `subscriptions update -> render-config` 的最小集成断言
  - `render-config` 的规则目标与 provider 组合断言
  - `render-config` 的“无 provider / auth+cors / 仅显式代理 / secret / external-controller / LAN 允许/禁止网段 / external-ui / nameserver-policy / default-nameserver / direct-nameserver / fake-ip-filter / profile / fallback-filter / proxy-server-nameserver / nameserver / geox-url / dns.listen / allow-lan / bind-address / log-level / mixed-port / tproxy-port / mode / ipv6 / geo flags / DNS behavior flags / manual & subscription provider / provider health-check / direct-only & AUTO proxy-groups / rules section & order / auth omission” 边界断言

## 质量状态

- `go build -o /tmp/gobin/minimalist ./cmd/minimalist`：通过
- `GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./...`：通过

## 当前风险与限制

- 当前 Go 测试已覆盖配置、provider、rules-repo、核心 app 路径、top-level CLI、主要 CLI helper、runtime 文本生成和 system runner，但 `runtime` / `cli` 测试中仍有少量可继续合并的重复场景和命名不统一问题
- `docs/images/readme-overview.svg` 已移除，后续若需要项目总览图应按 `minimalist` 当前架构重画
- 旧版本 `settings.env` / `router.env` / `state/*.json` 不兼容，不做迁移
