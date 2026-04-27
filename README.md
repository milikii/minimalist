# minimalist

面向 Debian NAS 的 Go 版旁路由管理器，使用 `mihomo-core` 作为底层内核。

## 当前定位

- 仅承诺 Debian NAS / IPv4 旁路由 / `iptables + TProxy`
- 默认宿主机直连，`proxy_host_output: false`
- 默认控制面仅绑定 `127.0.0.1:19090`
- 主命令名已经从 `mihomo` 切到 `minimalist`
- 旧 shell / Python 主实现已从主树移除，不再保留兼容入口

## 当前保留能力

- 核心主路径：`install-self`、`setup`、`render-config`、`start` / `stop` / `restart`
- 运维查看：`status`、`show-secret`、`healthcheck`、`runtime-audit`、`cutover-preflight`、`cutover-plan`
- 交互入口：`menu`、`router-wizard`、`import-links`
- 规则与订阅：`nodes`、`subscriptions`、`rules`、`acl`、`rules-repo`

## 开发入口与发布

```bash
go run ./cmd/minimalist --help
go run ./cmd/minimalist menu
go run ./cmd/minimalist setup
```

本地构建：

```bash
go build -o ./minimalist ./cmd/minimalist
```

测试：

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./...
```

发布到具备 systemd 的目标机：

```bash
sudo go run ./cmd/minimalist install-self
sudo /usr/local/bin/minimalist setup
```

## 运行时真相

- 用户配置：`/etc/minimalist/config.yaml`
- 程序状态：`/var/lib/minimalist/state.json`
- 若配置缺失 `controller.secret`，当前会自动补齐默认值并回写配置文件
- 运行产物：`/var/lib/minimalist/mihomo/`
  - `config.yaml`
  - `proxy_providers/manual.txt`：仅包含启用的非订阅节点
  - `proxy_providers/subscriptions/*.txt`
  - `ruleset/*.rules`

当前 provider 输入支持 `vless://`、`trojan://`、`ss://`、`vmess://`。

## 推荐使用顺序

1. `sudo minimalist install-self`
2. `sudo minimalist setup`
3. `minimalist import-links`
4. `minimalist subscriptions update`
5. `minimalist router-wizard`
6. `minimalist healthcheck`
7. `minimalist cutover-preflight`
8. `minimalist cutover-plan`
9. `minimalist status`

补充当前行为：

- `menu` 当前按运维任务分组：状态总览、部署/修复、节点管理、订阅管理、网络入口与规则仓库、规则与 ACL、服务管理、健康检查与审计
- 节点管理包含查看、导入、测试、改名、启用、禁用和删除；节点测试依赖本机 Mihomo controller
- `subscriptions update` 更新的是订阅 provider 缓存；`render-config` 直接读取缓存生成订阅 provider
- 即使当前没有手动节点或订阅 provider，`render-config` 仍会生成仅含 `DIRECT` 的 `PROXY` 组
- provider 导入会按 `URIBaseKey` 去重，并为重名节点自动追加后缀
- 从旧 `mihomo.service` 切到 Go 版前，先按 `docs/CUTOVER.md` 做人工 cutover 检查；当前本机旧服务资产已在切换验证后清理

## 当前限制

- 旧版本 `settings.env` / `router.env` / `state/*.json` 不兼容，不做迁移
- 不保留 `alpha/stable` 核心通道切换、自动同步、自定义更新定时器等旧运维能力
- `nas-single-lan-dualstack` 已不再进入当前产品边界
- `setup` / `start` / `restart` 的真实验证需要 systemd 正常运行
- `apply-rules` / `clear-rules` 的真实验证需要 `CAP_NET_ADMIN` 和可用的 `iptables` / `ip rule`
- `cutover-preflight` 是只读实机检查；若检测到旧 `mihomo.service` 正在承载现网，默认只告警，不停服务、不清规则
- `cutover-plan` 是只读计划输出；只给当前状态、下一步建议和回滚可用性，不执行 cutover
- 在旧 `mihomo.service` active/enabled 且 `minimalist.service` 尚未 active/enabled 时，`setup` / `start` / `restart` / `apply-rules` / `clear-rules` 会返回 `cutover blocked`
- `mihomo-core` 首次启动需要 `Country.mmdb`、`GeoSite.dat` 和 `ui/`；离线或慢网络环境下要先预置到 `/var/lib/minimalist/mihomo/`
