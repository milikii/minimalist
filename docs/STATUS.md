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
  - `internal/config` missing secret 回写落盘
  - `internal/provider` scan / render / base64 subscription decode / scannable URI filter / URIBaseKey / imported node dedupe & rename / naming helper / unsupported scheme fallback / `ss` / `vmess` / `vless` / `trojan` 解析 / `xhttp` / `grpc` / `ws` / `h2` / `httpupgrade` provider 渲染分支
  - `internal/rulesrepo` render / search / manifest empty / duplicate entry / invalid entry / missing source / append dedupe / remove-index rewrite / invalid YAML / unsupported manifest type / unsupported manifest target / `copyTree` missing source / `RemoveEntryIndex` 越界
  - `internal/app` import-links / render-config / subscriptions update / setup / start / restart / healthcheck / runtime-audit / menu / router-wizard / clear-rules / apply-rules
  - `internal/app` status 配置回退 / runtime 优先、healthcheck 控制面错误输出、runtime-audit 缺失 runtime 摘要、show-secret、install-self 自定义 bin 父目录、stop、invalid rule target、rename node 联动 target、node/rule/subscription list & remove、rules-repo app 入口、subscription rename guard、subscription update/disable/remove 副作用、subscription HTTP/transport/cache-dir 失败路径、AUTO target guard、rules menu prompt、routing helper、explicit-proxy-only apply-rules、setup preflight 失败透传
  - `internal/cli` top-level `Run(args)` / `help` / `-h` / `show-secret` / rules-repo / nodes / subscriptions / rules / acl helper / usage error / index error / unknown subcommand / 正向分发 / `rules-repo add/remove/remove-index` 成功分发 / `apply-rules` 成功分发 / `render-config` 成功分发 / `runWithApp` 的 start / stop / restart / router-wizard / setup / clear-rules
  - `internal/system` command runner / `Run` delegate / zero timeout default
  - `internal/runtime` paths helper / `EnsureLayout` / `RenderFiles` / `writeRules` / secret fallback / configured secret / external-controller / external-ui / nameserver-policy / DNS 默认静态段落 / profile / fallback-filter / proxy-server-nameserver / nameserver / geox-url / dns.listen / lan-allowed-ips / lan-disallowed-ips / allow-lan / bind-address / log-level / mixed-port / tproxy-port / mode / ipv6 / geo flags / DNS behavior flags / manual & subscription provider / provider health-check / direct-only & AUTO proxy-groups / rules section & order / auth omission / active provider 选择 / `RenderFiles` unsupported rule 失败路径 / `BuildServiceUnit` / `BuildSysctl` / service hardening / install target / core bin / 自定义 bin 父目录
  - `internal/app` install-self / setup / render-config / update-subscriptions / readImportInput / requireRoot 的真实 I/O 失败路径补强
  - `internal/app` `install-self` 非 root 早失败分支补强
  - `internal/app` `start` / `restart` / `stop` / `apply-rules` / `clear-rules` 的 non-root smoke 与 `rules-repo` wrapper 错误透传补强
  - `internal/app` `deleteIPRule` 重试退出、`ApplyRules` `ensureChain` 失败、`Setup` manifest 失败、`install-self` 的 `rules-repo` / `state.json` 路径阻塞、`Setup` `builtin.rules` 路径阻塞补强
  - `internal/app` `Start` / `Restart` 的 manifest 损坏与 runtime 路径阻塞早失败、`Setup` 的 `manual.txt` / `custom.rules` / 最终 `config.yaml` 路径阻塞早失败、`RenderConfig` 的最终 `config.yaml` 路径阻塞补强
  - `internal/cli` `runWithApp` 的 `setup` / `render-config` / `start` / `restart` 失败透传补强
  - `internal/runtime` `RenderFiles` 的 `manual.txt` / `custom.rules` / `builtin.rules` / 最终 `config.yaml` 路径阻塞失败路径补强
  - `subscriptions update -> render-config` 的最小集成断言
  - `render-config` 的规则目标与 provider 组合断言
  - `render-config` 的“无 provider / auth+cors / 仅显式代理 / secret / external-controller / LAN 允许/禁止网段 / external-ui / nameserver-policy / default-nameserver / direct-nameserver / fake-ip-filter / profile / fallback-filter / proxy-server-nameserver / nameserver / geox-url / dns.listen / allow-lan / bind-address / log-level / mixed-port / tproxy-port / mode / ipv6 / geo flags / DNS behavior flags / manual & subscription provider / provider health-check / direct-only & AUTO proxy-groups / rules section & order / auth omission” 边界断言
  - `internal/app` `menu` 主入口分发、`SetNodeEnabled` 手动节点启停与订阅节点只读边界、`rulesMenu` 删除分支
  - `internal/app` `SetSubscriptionEnabled` 启用/越界分支、`rulesMenu` ACL 增删、`Setup` 基于 subscription cache 启服务、`Status` active+manual node 统计
  - `internal/app` `promptList` / `promptBool` / `normalizeRuleInput` / `normalizeRuleKind` helper 扩展映射与显式输入分支
  - `internal/app` `ApplyRules` 无启用手动节点、DNS/OUTPUT 关闭时的跳转省略 smoke
  - `internal/app` `RulesRepoAdd` / `RulesRepoRemove` / `RulesRepoRemoveIndex` 的成功与早失败分支
  - `internal/app` `Setup` runtime layout 阻塞的早失败分支
  - `internal/rulesrepo` `Describe` / `ListEntries` / `DescribeRuleset` / `RemoveEntry` / `ValidateEntry` / `Search` 空关键词 / `Render` 非法条目的成功与早失败分支

## 质量状态

- `go build -o /tmp/gobin/minimalist ./cmd/minimalist`：通过
- `GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./...`：通过
- focused coverage 继续向 `ApplyRules` / `Setup` / `install-self` / `RenderFiles` 的真实失败路径收口

## 当前风险与限制

- 当前 Go 测试已覆盖配置、provider、rules-repo、核心 app 路径、status/healthcheck/runtime-audit 回退与 runtime 优先、top-level CLI 与 `runWithApp` 主要分发、runtime 文本生成、system runner 以及多组 helper 边界；当前剩余缺口进一步收缩到少量 root/真实环境依赖链路和真实主机上的 `iptables` / `ip rule` smoke
- `docs/images/readme-overview.svg` 已移除，后续若需要项目总览图应按 `minimalist` 当前架构重画
- 旧版本 `settings.env` / `router.env` / `state/*.json` 不兼容，不做迁移
