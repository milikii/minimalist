# TASKS

> 2026-04-28 依据最新需求重排优先级。

## P0：主路径稳定性

- [ ] 统一核心 ready 语义：主路径只围绕“启用的手动节点”
  说明：当前产品不再把订阅当成一等公民，`setup/start/apply-rules` 的成功前提应该统一围绕手动节点主路径。

- [ ] 固定“默认不影响宿主机网络”的契约
  说明：`proxy_host_output: false` 必须成为硬约束，并由测试固定，防止后续模板演化误接管宿主机流量。

- [ ] 固定“不能 DNS 泄露”的契约
  说明：围绕 DNS hijack、fake-ip、DoH、nameserver-policy 和 route 路径建立稳定配置与回归测试。

- [ ] 做一版成熟常规的 Mihomo 配置模板
  说明：不要继续零散拼配置。要把成熟基线模板固定下来，再叠加你的个人规则分流。

- [ ] 前移规则目标校验
  说明：禁止把 disabled manual node 写成规则目标，避免“保存成功，重启才炸”。

- [ ] 前移节点重命名校验
  说明：禁止保留名、禁止重名、禁止制造会污染 runtime proxy 名称的状态。

## P1：运维与可用性

- [ ] 暴露 `minimalist nodes test`
  说明：节点测速已经实现，但非交互 CLI 不完整。

- [ ] 在 help 中明确 `verify-runtime-assets`
  说明：这是 systemd preflight 的关键入口，不能继续隐藏得过深。

- [ ] 写短版 restart / reboot smoke runbook
  说明：把 service、controller、iptables、`ip rule`、table 233、DNS 路径检查压成一套最短日常步骤。

- [ ] 固定规则层顺序
  说明：明确成熟模板层、个人规则层、尾部兜底层的顺序，避免以后改规则时优先级失控。

## P2：增强项

- [ ] 把订阅能力正式降级为增强项
  说明：保留实现，但不再让它驱动主路径设计、测试和文档。

- [ ] `core-upgrade-alpha` 失败自动回滚
  说明：有价值，但属于增强项，不该压过主路径稳定性。

- [ ] `core-upgrade-alpha` 支持 amd64 CPU-level 资产
  说明：也是增强项，放在稳定主线之后。

- [ ] 处理默认规则仓库的双份维护风险
  说明：这是结构性问题，但不会先于主路径稳定性处理。

## 当前顺序

1. 先钉死手动节点主路径
2. 再钉死 DNS 与宿主机网络安全契约
3. 再收口模板与规则层
4. 最后处理 CLI/runbook 和增强项
