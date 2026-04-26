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

## 下一最小闭环

- 为顶层 `Run(args)` 分发、usage error、`--help` 输出补 focused tests
- 为 `render-config` 补更多控制面 bind / secret fallback / LAN 禁止网段断言
- 继续把 README / flows 文档收口到 `minimalist` 当前命令真相

## 本轮不做

- 不恢复旧 `mihomo` 命令入口
- 不做旧状态迁移兼容
- 不引入 alpha/stable 切换、自同步、回滚 core 等旧运维能力
- 不扩 `external-controller-tls`

## 退出条件

- 旧主入口和旧主实现已不再残留在主树中
- `go test ./...` 覆盖核心命令与系统编排关键路径
- README 与权威文档只描述 `minimalist` 当前真相
