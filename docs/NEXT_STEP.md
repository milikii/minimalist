# 下一步

## 当前阶段

- Go 版 `minimalist` 主实现已经落地，默认分支保持可构建、可测试。
- 单元与 focused 测试已经覆盖核心配置、状态、provider、rules-repo、runtime 渲染、app 命令编排、CLI 分发与多组失败路径；`internal/app` 经过最新小步硬化已提升到 95.6% 语句覆盖率，`internal/config` 已到 94.3%。
- 这台 Debian NAS 已经是可用实机：`systemd`、`iptables`、`ip rule` 都是真实可达的。
- 现网已经从旧 `mihomo.service` 切换到 Go 版 `minimalist.service`；旧服务当前 `inactive/disabled`，新服务 `active/enabled`。
- 本轮全量 `go test ./...` 和 build 已通过，实机 `healthcheck` / `runtime-audit` / systemd / ip rule / route table smoke 已通过。

## 下一最小闭环

- 当前没有新的功能缺口；下一步优先做切换后观察与最小硬化，不扩协议、不恢复旧运维能力。
- 当前轮次已额外补稳 runtime asset、menu/CLI 节点管理、controller delay 错误输出、空白 rules manifest、空白 secret 持久化、legacy state version 回填、`testNodeDelay` 失败分支、`networkMenu` 关键分发、安装路径阻断、导入读取错误、非法规则目标、`ensureAll` 失败传播、`updateSubscriptions` mixed/disabled 边界、`apply-rules` 空 bypass 输入、菜单 `0` 返回，以及 config/state/rules-repo/provider/CLI 的一组 focused 错误路径。
- 若继续施工，优先选择：
  - 继续观察 `minimalist.service` 24 小时日志；2026-04-28 08:34 CST 已确认 UI/geodata 资源复制后最近启动窗口不再出现启动下载错误，当前 warn/error 计数仍来自切换早期历史窗口。
  - 继续补 `internal/app` 的剩余低覆盖热点，优先 `serviceMenu` / `auditMenu` / `nodesMenu` 这类菜单分发与 `apply-rules`、`updateSubscriptions` 的更细尾部分支；通用错误传播和 `0` 返回分支已完成一轮 focused 收口。
  - 复跑 `runtime-audit`，确认 warn/error 计数只剩历史窗口内记录。
- 旧 `/etc/mihomo`、`mihomo.service`、`/usr/local/bin/mihomo` 与 `/usr/local/lib/mihomo-manager` 已清理；下一步不再围绕旧服务回滚路径推进。
- 保持 README / flows 描述 Go 版 `minimalist` 目标真相；STATUS / NEXT_STEP 记录 live host 已切换完成。

## 本轮不做

- 不做旧状态迁移兼容。
- 不引入 alpha/stable 切换、自同步、回滚 core 等旧运维能力。
- 不扩 `external-controller-tls`。

## 退出条件

- README 与权威文档只描述 Go 版 `minimalist` 当前真相。
- `go test ./...` 覆盖核心命令与系统编排关键路径，且当前轮次的 focused 验证已通过。
- Go 版高风险命令在 legacy live install 存在时已有 guard，人工 cutover 步骤已文档化，且本机 cutover 已完成。
- 当前轮次全量验证与实机 smoke 已完成；后续进入切换后观察与小步质量硬化。
