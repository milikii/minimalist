# mihomo-nas

用于 Debian NAS 的宿主机旁路由 Mihomo 管理脚本。

## 当前运行模型
- 当前项目只承诺 Debian NAS 的 IPv4 旁路由。
- 宿主机默认直连，不默认透明接管本机外连，默认 `PROXY_HOST_OUTPUT=0`。
- 宿主机应用如需代理，显式使用 `http://127.0.0.1:7890`。
- 局域网设备把网关和 DNS 指向 NAS 后，可复用 NAS 上的旁路由能力。
- Docker 默认直连；只有显式设置代理，或后续把对应 bridge 接进透明代理，容器才会走 Mihomo。
- `tailscaled` / `cloudflared` 运行时，脚本拒绝启用宿主机透明接管，避免误伤 SSH / 隧道链路。
- 控制面默认只绑定 `127.0.0.1:${CONTROLLER_PORT:-19090}`；状态页不再默认打印密钥，需显式执行 `mihomo show-secret`。
- `nas-single-lan-dualstack` 当前仅兼容保留，不代表项目已经实现真双栈旁路由。
- 订阅更新会把原始 provider 内容缓存到 `proxy_providers/subscriptions/*.txt`；订阅节点在本地节点列表中仅作只读枚举，启用/关闭以订阅开关为准。
- 状态页中的“手动节点 / 订阅 provider / 订阅缓存”已经分开统计，不再把订阅缓存节点和手动节点混为一谈。
- 重构判断与后续路线见 [docs/refactor-roadmap.md](docs/refactor-roadmap.md)。

## 推荐入口
- `mihomo menu`：默认操作入口；主菜单只保留宿主机旁路由主路径。
- `mihomo status`：查看当前运行定位、入口接口、规则预设、宿主机流量模式和容器直连名单。
- `mihomo show-secret`：显式查看控制面密钥；默认状态页不会直接暴露。
- `mihomo healthcheck`：检查端口监听、WebUI 与 `127.0.0.1:7890` 显式代理是否可用。
- `mihomo runtime-audit`：看 systemd 状态、近 24 小时 warning/error 和健康摘要。
- `./mihomo install-self-sync [minutes]`：把当前 git 工作树安装到本机，并按分钟周期自动同步到 `/usr/local/lib/mihomo-manager`。
- `mihomo apply-default-template`：一键应用项目默认模板，启用项目内置规则仓库，并把宿主机流量恢复为默认直连。
- `mihomo rules-repo`：查看当前项目内置规则仓库摘要和各规则集条目数。
- `mihomo rules-repo-entries [ruleset] [keyword]`：查看某个规则集当前的具体条目，可按关键字过滤。
- `mihomo rules-repo-find [keyword]`：跨规则集搜索规则项。
- `mihomo add-repo-rule [ruleset] [value]`：向项目内置规则仓库追加一条规则。
- `mihomo remove-repo-rule [ruleset] [value]`：从项目内置规则仓库删除一条规则。
- `mihomo remove-repo-rule-index [ruleset] [index]`：按序号删除规则项，适合删除长条目。
- `mihomo rule-presets` / `mihomo set-rule-preset default`：查看或切换项目内置默认规则模板。

## 菜单分层
- 主菜单：部署修复、节点管理、旁路由边界、服务、健康、审计、日志与诊断。
- 高级维护（少用）：规则、模式切换、端口设置、内核更新/回滚、WebUI、定时器和审计。

## 推荐使用顺序
1. `mihomo setup`
2. `mihomo import-links`
3. `mihomo apply-default-template`（保留当前网络形态和控制面绑定，启用项目内置规则仓库，并恢复宿主机默认直连）
4. `mihomo router-wizard`（会按入口接口自动回写 `LAN_CIDRS`）
5. `mihomo healthcheck`
6. 局域网设备把网关和 DNS 指向 NAS

## 本机源码自动同步
- 默认 `mihomo install-self` 仍然是一次性拷贝安装。
- 如果这台机器直接以当前仓库工作树为真相源维护，可在仓库目录执行 `sudo ./mihomo install-self-sync`。
- 该命令会先执行一次安装，再写入 `mihomo-manager-sync.service` 和 `mihomo-manager-sync.timer`，后续按分钟周期把工作树同步到安装目录。
- 关闭自动同步：`sudo mihomo disable-self-sync`
- 这只保证“本机安装目录跟随本机仓库工作树”，不会自动从 GitHub 拉取更新。

## 项目内置规则仓库
- 网络模板仍只负责旁路由入口形态，例如单 LAN / 双栈 / 多 bridge；规则策略由项目内置默认模板提供。
- 当前项目已经自带本地规则仓库：`rules-repo/default/`
- 当前内置规则集名称：
  - `pt`
  - `fcm-site`
  - `fcm-ip`
- 默认模板当前内置三组规则：
  - `PT` 相关域名走 `DIRECT`
  - `FCM` 相关域名走 `PROXY`
  - `FCM` 相关 IP 走 `PROXY`
- 如需直接维护项目内置规则仓库，可使用：
  - `mihomo rules-repo-entries pt`
  - `mihomo rules-repo-entries pt hd`
  - `mihomo rules-repo-find google`
  - `mihomo add-repo-rule pt example.com`
  - `mihomo remove-repo-rule pt example.com`
  - `mihomo remove-repo-rule-index pt 12`
- 在交互式添加/删除时，命令会先显示目标规则集摘要，再提示输入，减少误改。
- `add-repo-rule` 会按规则集类型做最小校验：
  - `pt` / `fcm-site` 这类域名规则会拒绝明显非法字符
  - `fcm-ip` 这类 `ip_cidr` 规则会校验 CIDR 格式
- `mihomo apply-default-template` 会在不改你当前 LAN / bridge 入口形态的前提下，统一设置：
  - `CONFIG_MODE=rule`
  - `RULESET_PRESET=default`
  - `PROXY_HOST_OUTPUT=0`
- 控制面绑定 `CONTROLLER_BIND_ADDRESS` 会保留当前值，不再被该命令强制改回 `127.0.0.1`。
- 手工自定义规则和 ACL 仍优先于项目默认模板，便于你在机器侧做覆盖。

## 关键端口
- `7890`: mixed 显式代理
- `7893`: TProxy
- `1053`: DNS
- `19090`: Controller / WebUI（默认仅宿主机）

## DNS 设计
- `default-nameserver` 只负责解析上游 DNS 自己的域名，减少 bootstrap 继续走宿主机系统 DNS。
- `fallback` 走 `cloudflare-dns.com` / `dns.google`，并显式加 `#RULES`，让海外加密 DNS 的链路更贴近旁路由规则。
- `direct-nameserver` 固定为国内 DoH，确保 DIRECT 直连域名解析更稳定。
- `fake-ip-filter` 保留 NAS/局域网常见兼容项，避免 captive portal、局域网域名和 STUN 被 fake-ip 破坏。
- 现在 `GeoSite.dat` 已通过探针验证，默认模板已重新启用 `nameserver-policy` 与 `fallback-filter.geosite: gfw` 这类更强 DNS 策略。

## geosite 现状
- `GeoSite.dat` 资产治理路径已经落地，可通过 `mihomo install-geosite` 下载并做最小探针验证。
- 当前默认模板已经重新启用 geosite 型 DNS 增强规则；如需确认资产状态，可执行 `mihomo audit-installation`。

## GeoSite 资产治理
- 可显式执行 `mihomo install-geosite` 下载并验证 `GeoSite.dat`。
- 下载会按顺序尝试 GitHub release、JSDelivr、JSDelivr-CF，避免单一源抽风时整条治理链失效。
- 脚本会先下载到临时文件，再用最小 geosite 配置做探针；只有验证通过才会覆盖运行目录。
- `mihomo audit-installation` 会继续报告当前 `GeoSite.dat` 是否真的 ready；若不可用，审计直接失败。
