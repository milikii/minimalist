# minimalist 关键流程

本页只描述 Go 版 `minimalist` 当前真相。

## 主路径

1. `minimalist install-self`
2. `minimalist setup`
3. `minimalist import-links` 或 `minimalist subscriptions update`
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
   - `/var/lib/minimalist/mihomo/proxy_providers/subscriptions/*.txt`
   - `/var/lib/minimalist/mihomo/ruleset/*.rules`
4. `minimalist.service` 通过 `mihomo-core` 读取运行目录启动
5. `setup` 现在会显式返回 `sysctl -p`、`systemctl daemon-reload` 和 `systemctl enable --now` 的失败，不再假装部署成功

补充当前真相：

- `render-config` 是运行产物唯一生成入口；即使没有 provider，也会生成仅含 `DIRECT` 的 `PROXY` 组。
- DNS、controller、profile、proxy-groups、rules、provider health-check、service unit 与 sysctl 输出已经由 focused tests 固定。
- 顶层 `minimalist rules|acl|subscriptions|rules-repo ...` 当前都直接分发到同一组底层 CLI helper。
- `setup` / `start` / `restart` 的真实验证依赖 systemd；`apply-rules` / `clear-rules` 的真实验证依赖 `CAP_NET_ADMIN` 与可用 `iptables` / `ip rule`。

## 节点与订阅

- `import-links` 导入的是手动节点真相，默认 `disabled`
- `subscriptions update` 拉取的是 provider 缓存真相，订阅节点只保留只读枚举
- `render-config` 会把订阅缓存和手动节点分别渲染到不同 provider 文件；`manual.txt` 不包含订阅节点
- provider 导入当前会按 `URIBaseKey` 去重，并为重名节点自动加后缀
- provider 命名当前优先使用 URI fragment 或 `vmess.ps`，协议不支持时会落到保守回退命名
- ACL / 自定义规则只允许指向手动节点与内置目标

## 辅助入口

- `minimalist menu` 是当前交互入口，内部仍分发到同一组 CLI 命令
- 顶层 `minimalist --help` / `help` / 非 TTY 空参数当前都回落到同一份 usage 输出
- 顶层 `minimalist setup` / `render-config` / `start` / `stop` / `restart` / `clear-rules` 等命令当前仍直接分发到同一组 `internal/app` 实现
- `minimalist router-wizard` 直接回写 `/etc/minimalist/config.yaml`
- `minimalist rules-repo summary|entries|find` 用于查看当前内置规则仓库真相
- `minimalist cutover-preflight` 只读检查 legacy `mihomo.service`、旧安装路径和 Go 版目标路径，不写配置、不停服务、不改规则
- `minimalist cutover-plan` 只读输出当前 cutover 状态、下一步建议和回滚入口，不执行切换

## 运行态观测

- `status` / `runtime-audit` / `healthcheck` 优先读取 Mihomo REST API
- 控制面不可达时回退配置文件、systemd 和本机端口信息
- `status` 当前会优先展示 runtime mode；控制面不可达时回退到 `config.yaml` 中的 mode
- `runtime-audit` 在控制面不可达时不会伪造 runtime 摘要，只保留本地状态和日志告警计数
- `cutover-preflight` 当前实机输出 `cutover-ready=false` 时，表示仍处于旧 `mihomo.service` live install 状态；高风险命令会返回 `cutover blocked`，不会自动停旧服务或清规则
- `cutover-plan` 当前只服务人工 runbook，不替代维护窗口决策
