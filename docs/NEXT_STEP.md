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
  - `ApplyRules` 无启用手动节点、DNS/OUTPUT 关闭时的跳转省略 smoke、`deleteIPRule` 重试退出、`ensureChain` 失败透传
  - `RulesRepoAdd` / `RulesRepoRemove` / `RulesRepoRemoveIndex` 的成功与早失败分支
  - `Setup` runtime layout 阻塞、manifest 损坏、`builtin.rules` 路径阻塞的早失败分支
  - `install-self` 的 `rules-repo` / `state.json` 路径阻塞失败分支
  - `runtime.RenderFiles` 的 `manual.txt` / `custom.rules` / `builtin.rules` / 最终 `config.yaml` 路径阻塞失败分支
  - `Start` / `Restart` 的 manifest 损坏与 runtime 路径阻塞早失败、`Setup` 的 `manual.txt` / `custom.rules` / 最终 `config.yaml` 路径阻塞早失败、`RenderConfig` 的最终 `config.yaml` 路径阻塞补强
  - `cli` `runWithApp` 的 `setup` / `render-config` / `start` / `restart` 失败透传补强
  - `rulesrepo` `Search` 空关键词 / `Render` 非法条目早失败分支
  - `rules-repo add/remove/remove-index` 的 `Run` 成功分发
  - `apply-rules` 的 `Run` 成功分发
  - `render-config` 的 `Run` 成功分发

## 下一最小闭环

- 在真实主机上补 `ApplyRules` / `ClearRules` 的 `iptables` / `ip rule` smoke，验证当前命令编排与测试假设一致
- 若本机具备 systemd / root 环境，继续补 `setup` / `start` / `restart` 的真实 smoke，重点验证当前测试覆盖之外的系统命令实际回执
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
