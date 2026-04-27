# 下一步

## 当前阶段

- Go 版 `minimalist` 主实现已经落地，默认分支保持可构建、可测试。
- 单元与 focused 测试已经覆盖核心配置、状态、provider、rules-repo、runtime 渲染、app 命令编排、CLI 分发与多组失败路径。
- 这台 Debian NAS 已经是可用实机：`systemd`、`iptables`、`ip rule` 都是真实可达的。
- 现网仍在跑旧的 `mihomo.service`，而不是 `minimalist.service`，所以当前不是直接对 Go 版 `minimalist` 做收尾验收，而是先把 live install 归属理清。

## 下一最小闭环

- 补一个非破坏性迁移前置闭环，先防止误操作再谈切换：
  - 识别 `mihomo.service` active 且 `minimalist.service` absent 的 legacy live 状态
  - 明确 Go 版 `apply-rules` / `clear-rules` 与旧服务共用 `MIHOMO_*` 链名和 `0x2333` / table `233`
  - 给出 safe cutover 前置检查或文档化步骤，默认不自动停旧服务、不自动清规则
  - 若要改代码，优先做只读 preflight / audit，不直接改变现网规则行为
- 在确认迁移策略前，不对现网 `MIHOMO_*` 规则做清理或重写。
- 若确认要切换到 Go 版，再做最小迁移闭环并重新跑 `setup` / `start` / `restart` / `apply-rules` / `clear-rules` 实机 smoke。
- 保持 README / flows 描述 Go 版 `minimalist` 目标真相；STATUS / NEXT_STEP 只记录 live host 差异，不恢复旧 `mihomo` 作为项目目标。

## 本轮不做

- 不盲目清理现网 `mihomo` 透明代理规则。
- 不直接把当前实机当成已经完成 `minimalist` 部署。
- 不做旧状态迁移兼容。
- 不引入 alpha/stable 切换、自同步、回滚 core 等旧运维能力。
- 不扩 `external-controller-tls`。

## 退出条件

- README 与权威文档只描述 Go 版 `minimalist` 当前真相。
- `go test ./...` 覆盖核心命令与系统编排关键路径。
- 在这台 NAS 上形成一个可验证、可回滚、不会误清现网规则的 Go 版 cutover 前置方案。
