# minimalist 关键流程

本页只描述 Go 版 `minimalist` 当前真相。

## 主路径

1. `minimalist install-self`
2. `minimalist setup`
3. `minimalist import-links`
4. `minimalist router-wizard`
5. `minimalist render-config`
6. `minimalist start`
7. `minimalist healthcheck`
8. `minimalist cutover-preflight`
9. `minimalist cutover-plan`
10. `minimalist status`

## 配置与状态流

1. `minimalist` 读取 `/etc/minimalist/config.yaml`
   - 若配置文件缺少 `controller.secret`，当前会在 `Ensure` 阶段补齐并回写
2. 资源型命令更新 `/var/lib/minimalist/state.json`
3. `render-config` 生成：
   - `/var/lib/minimalist/mihomo/config.yaml`
   - `/var/lib/minimalist/mihomo/proxy_providers/manual.txt`
   - `/var/lib/minimalist/mihomo/proxy_providers/subscriptions/*.txt`（订阅增强项缓存）
   - `/var/lib/minimalist/mihomo/ruleset/*.rules`
   - 规则层顺序固定为 `custom.rules` -> `acl.rules` -> `builtin.rules` -> 尾部默认规则
   - baseline 模板固定，DNS / fake-ip / DoH / profile / controller 作为稳定骨架
4. `minimalist.service` 通过 `mihomo-core` 读取运行目录启动
5. `setup` 现在会显式返回 `sysctl -p`、`systemctl daemon-reload` 和 `systemctl enable --now` 的失败，不再假装部署成功

补充当前真相：

- `render-config` 是运行产物唯一生成入口；即使没有 provider，也会生成仅含 `DIRECT` 的 `PROXY` 组。
- `render-config` 的规则层按固定顺序拼装：个人规则层（custom/ACL） -> 仓库规则层 -> 尾部默认兜底规则
- DNS、controller、profile、proxy-groups、rules、provider health-check、service unit 与 sysctl 输出已经由 focused tests 固定。
- 顶层 `minimalist rules|acl|rules-repo ...` 当前都直接分发到同一组底层 CLI helper；`minimalist subscriptions ...` 保留为增强项 helper。
- `setup` / `start` / `restart` 的真实验证依赖 systemd；`apply-rules` / `clear-rules` 的真实验证依赖 `CAP_NET_ADMIN` 与可用 `iptables` / `ip rule`。

## 节点与订阅

- `import-links` 导入的是手动节点真相，默认 `disabled`
- `subscriptions update` 拉取的是增强项 provider 缓存真相，订阅节点只保留只读枚举
- `setup` / `start` 的核心成功路径只看启用的手动节点，订阅缓存就绪不能替代手动节点
- `render-config` 会把订阅缓存和手动节点分别渲染到不同 provider 文件；`manual.txt` 不包含订阅节点
- provider 导入当前会按 `URIBaseKey` 去重，并为重名节点自动加后缀
- provider 命名当前优先使用 URI fragment 或 `vmess.ps`，协议不支持时会落到保守回退命名
- ACL / 自定义规则只允许指向手动节点与内置目标
- `ruleset/custom.rules` 和 `ruleset/acl.rules` 是个人分流层，`ruleset/builtin.rules` 是仓库规则层，尾部兜底规则固定不变

## 辅助入口

- `minimalist menu` 是当前交互入口，顶层按节点管理、配置管理、规则管理、日志与诊断、控制启停 5 类高频任务分组
- 节点管理会先显示节点列表；输入节点 ID 进入单节点操作面板，可直接启用/禁用、改名、测试或确认删除；`a` 导入节点，`t` 测试全部启用节点
- 配置管理包含 `router-wizard`、宿主机接管开关、订阅管理（增强项）、重新渲染配置和关键路径提示
- 规则管理合并自定义规则、ACL 与规则仓库入口；日志与诊断合并 `status`、`healthcheck`、`runtime-audit` 与 snapshot `log`
- 当前菜单只暴露 Go 版日常保留能力；`core-upgrade-alpha` 是显式 CLI 维护入口，不放回菜单，也不恢复旧版通道切换、core 回滚或自动更新
- 顶层 `minimalist --help` / `help` / 非 TTY 空参数当前都回落到同一份 usage 输出
- 顶层 `minimalist setup` / `render-config` / `core-upgrade-alpha` / `start` / `stop` / `restart` / `clear-rules` 等命令当前仍直接分发到同一组 `internal/app` 实现
- `minimalist router-wizard` 直接回写 `/etc/minimalist/config.yaml`
- `minimalist rules-repo summary|entries|find` 用于查看当前内置规则仓库真相
- `minimalist cutover-preflight` 只读检查 legacy `mihomo.service`、旧安装路径和 Go 版目标路径，不写配置、不停服务、不改规则
- `minimalist cutover-plan` 只读输出当前 cutover 状态、下一步建议和回滚可用性，不执行切换

## 运行态观测

- `status` / `runtime-audit` / `healthcheck` 优先读取 Mihomo REST API
- 控制面不可达时回退配置文件、systemd 和本机端口信息
- `status` 当前会优先展示 runtime mode；控制面不可达时回退到 `config.yaml` 中的 mode
- `runtime-audit` 在控制面不可达时不会伪造 runtime 摘要，并会把日志信号拆成 `alerts-24h`、`alerts-recent` 与 `fatal-gaps`
- `cutover-preflight` 输出 `cutover-ready=false` 时，表示仍处于旧 `mihomo.service` live install 状态；高风险命令会返回 `cutover blocked`，不会自动停旧服务或清规则
- 当前实机旧 `mihomo` 资产已清理，`cutover-plan` 应显示 legacy rollback unavailable
- `mihomo-core` 首次启动依赖 `Country.mmdb`、`GeoSite.dat` 和 `ui/`；离线或慢网络环境要先预置到 `/var/lib/minimalist/mihomo/`
- `minimalist core-upgrade-alpha` 只升级官方 alpha `mihomo-core` 并重启 `minimalist.service`；若重启失败会自动恢复旧 core 并再次重启服务，不修改 `minimalist` 自身二进制、不切 stable 通道、不创建定时器
- `amd64` 主机如需使用 CPU-level 资产，先在 `/etc/minimalist/config.yaml` 的 `install.core_amd64_cpu_level` 显式写入 `compatible` / `v1` / `v2` / `v3` 等值；为空时不会猜测选择
- `cutover-plan` 当前只服务人工 runbook，不替代维护窗口决策
