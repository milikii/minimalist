# 当前决策

## 2026-04-26 项目正式切到 `minimalist`

- 新项目名、命令名、systemd unit、配置目录统一使用 `minimalist`
- `mihomo-core` 继续只是底层内核名，不再承担管理器命名
- 旧 `mihomo` 命令入口已删除，不保留 shim

## 2026-04-26 Go V2 作为当前主实现

- 当前主实现已经改为 Go 模块
- shell / Python 旧代码不再作为当前主路线
- 旧实现暂时只保留在仓库中作参考，不再是默认文档真相

## 2026-04-26 能力面收缩到“核心 + 规则/订阅”

- 保留：
  - setup / render-config / start / stop / restart
  - status / show-secret / healthcheck / runtime-audit
  - import-links / router-wizard / menu
  - nodes / subscriptions / rules / acl / rules-repo
- 删除或暂不实现：
  - alpha/stable 核心通道切换
  - core 回滚
  - 自动同步安装目录
  - 自定义更新/重启定时器
  - 双栈模板

## 2026-04-26 配置与状态真相重做

- 用户配置真相：`/etc/minimalist/config.yaml`
- 程序状态真相：`/var/lib/minimalist/state.json`
- 旧 `settings.env` / `router.env` / `state/*.json` 不再兼容，也不迁移

## 2026-04-26 仍保持 Debian NAS / IPv4 旁路由边界

- 继续只承诺 Debian NAS / IPv4 旁路由 / `iptables + TProxy`
- 不补 OpenWrt / firewall4 / nftables 抽象
- 不恢复 `nas-single-lan-dualstack`

## 2026-04-27 实机 legacy install 不做原地覆盖

- 当前 NAS 仍由旧 `mihomo.service` 承载现网透明代理
- Go 版 `minimalist` 不能把旧 `/etc/mihomo` 直接视为自己的配置和状态真相
- 切换到 Go 版前必须先完成非破坏性 preflight / cutover 检查
- `cutover-preflight` 必须保持只读，不创建 `/etc/minimalist`、不启停 systemd unit、不操作 `iptables` / `ip rule`
- `setup` / `start` / `restart` / `apply-rules` / `clear-rules` 在 legacy live 且 `minimalist.service` 尚未 active/enabled 时必须阻断；仅有 Go 版二进制或 unit 文件不足以放行
- 在确认 cutover 方案前，不自动停旧服务、不自动清理现网 `MIHOMO_*` 规则、不自动迁移旧 `settings.env` / `router.env` / `state/*.json`

## 2026-04-27 `clear-rules` 删除失败要上浮

- `ClearRules` 对不存在的 jump 仍然保持幂等忽略
- 如果 `deleteJump` 先确认到规则存在，但随后删除命令失败，必须把错误返回给上层
- 这样 `ApplyRules` 才能在清理阶段失败时直接停止，避免继续写入半新半旧的路由规则

## 2026-04-28 本机现网切到 Go 版 `minimalist`

- 本机 live install 已从旧 `mihomo.service` 切到 Go 版 `minimalist.service`
- 旧状态中 4 个手动节点被导入 Go 版 state 并启用；旧 env/state 文件仍不作为 Go 版真相，也不做通用迁移
- 为避免启动依赖外网下载，本机将旧 runtime 中已有的 geodata 与 UI 资源复制到 `/var/lib/minimalist/mihomo/`
- 项目仍不新增自动 cutover、自动回滚、旧状态迁移、alpha/stable 通道切换或 core 回滚能力；后续只允许单一用途的官方 alpha 内核升级入口

## 2026-04-28 清理旧 `mihomo` 回滚入口

- 经人工确认，已删除旧 `/etc/mihomo`、`/etc/systemd/system/mihomo.service`、`/usr/local/bin/mihomo` 与 `/usr/local/lib/mihomo-manager`
- `/usr/local/bin/mihomo-core` 仍保留，继续作为 Go 版 `minimalist.service` 的底层内核运行
- 旧 `mihomo.service` 快速回滚路径已移除；后续以 Go 版 `minimalist` 为唯一 live 管理入口
- `cutover-plan` 在检测不到旧资产时应明确输出 legacy rollback unavailable，不再提示启用已删除的旧 service

## 2026-04-28 官方 alpha 内核升级入口

- `mihomo-core` 是独立底层内核，可以按官方 `MetaCubeX/mihomo` alpha release 单次升级，不与 `minimalist` 管理器发布节奏绑定
- 新入口固定为 `minimalist core-upgrade-alpha`，不放入交互菜单，避免把一次性高风险维护动作伪装成日常菜单操作
- 成功替换 `/usr/local/bin/mihomo-core` 后自动重启 `minimalist.service` 并检查 active 状态
- `arm64` 直接匹配当前平台资产；`amd64` 不猜测 CPU level，遇到 `compatible` 或 `vN` 资产时要求后续显式策略
- 不提供 stable 通道切换、自动定时更新或 rollback 命令

## 2026-05-16 新增内置 Web 控制面

- 用户明确认为 CLI / menu 已无法承载节点管理、配置管理和核心升级等复杂操作，决定新增 `minimalist webui`
- WebUI 是 `minimalist` 本机控制面，不是重写 Mihomo Dashboard；它复用现有 app 方法和文件真相
- 默认监听 `0.0.0.0:18080`，方便 LAN 内设备直接访问；`--addr 127.0.0.1:18080` 仍可切回本机访问
- LAN 暴露要求强 token：长度至少 16，且不能使用 `minimalist-secret` 兜底值
- WebUI 中的节点变更沿用 CLI 语义：服务 active 时会重新渲染 runtime 并重启 `minimalist.service`
- 首版范围限定为总览、节点、配置、规则、服务、日志和 `core-upgrade-alpha`；不做多用户、权限分级、TLS 终止或独立前端构建链
