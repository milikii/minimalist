# 下一步

## 当前阶段

- 当前主线已经从 shell/Python 收口切到 Go 版 `minimalist` 实现落地。
- 当前第一轮 Go V2 已经完成：
  - Go 模块与 CLI 主入口
  - 配置/状态真相
  - provider / rules 基础渲染
  - rules-repo 默认资产
  - systemd/sysctl 文本生成
  - 基础单元测试
  - `app` / `system` 的最小命令与集成护栏
  - `setup` / `clear-rules` / `apply-rules` 的最小命令编排护栏
- 当前补强已经覆盖：
  - `InstallSelf` / `Setup` / `RenderConfig` / `UpdateSubscriptions` 的真实 I/O 失败路径
  - `readImportInput` 的终端输入截断分支
  - `requireRoot` 的可测错误分支
  - `start` / `restart` / `stop` / `apply-rules` / `clear-rules` 的 non-root smoke
  - `rules-repo` wrapper 的 manifest / ruleset / keyword / invalid entry / index range 错误透传
  - `menu` 主入口分发、`SetNodeEnabled` 手动节点启停与订阅节点只读边界、`SetSubscriptionEnabled` 启用/越界分支
  - `rulesMenu` 主/ACL 增删分支、`promptList` / `promptBool` 显式输入、`normalizeRuleInput` / `normalizeRuleKind` 扩展映射
  - `Setup` 基于 subscription cache 启服务、`Status` active+manual node 统计
  - `rules-repo add/remove/remove-index` 的 `Run` 成功分发
  - `apply-rules` 的 `Run` 成功分发
  - `render-config` 的 `Run` 成功分发
  - `internal/app` focused coverage 已提升到 `88.0%`

## 下一最小闭环

- 继续补 `ApplyRules` 更深的 iptables / ip rule 编排 smoke 断言
- 继续补 `setup` / `status` / `rules-repo` 更贴近真实运行环境的 smoke 断言
- 保持 README / flows / STATUS 只描述 `minimalist` 当前真相，不回退到旧 `mihomo` 叙述

## 本轮不做

- 不恢复旧 `mihomo` 命令入口
- 不做旧状态迁移兼容
- 不引入 alpha/stable 切换、自同步、回滚 core 等旧运维能力
- 不扩 `external-controller-tls`

## 退出条件

- 旧主入口和旧主实现已不再残留在主树中
- `go test ./...` 覆盖核心命令与系统编排关键路径
- README 与权威文档只描述 `minimalist` 当前真相
