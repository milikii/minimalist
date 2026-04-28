# PLAN

> 日期：2026-04-28
> 新锚点：稳定、长期运行、无 DNS 泄露、默认不影响宿主机网络。

## 结论

项目要收回到一个很窄但很重要的目标：

> Debian NAS 宿主机上跑一个稳定的 `mihomo-core` 旁路由，核心是手动节点和少量自定义规则，面板直接用现成 Mihomo 面板。

这意味着：

- 订阅不是主线
- 内核升级不是主线
- 规则仓库编辑器不是主线
- runtime audit 再花哨也不是主线

主线只有四件事：

1. 旁路由稳定
2. DNS 不泄露
3. 宿主机默认不受影响
4. 模板成熟且规则分流好维护

## Step 0 — Scope Challenge

### 当前状态判断

项目现在属于 **stabilizing / repaying debt**，不是 feature expansion。

### 最大 blast radius

- DNS 泄露
- 宿主机网络被误接管
- 规则主路径不一致，导致服务看起来启动了但实际不可用
- 模板越改越散，长期维护失控

### What Already Exists

- [internal/config/config.go](/home/projects/minimalist/internal/config/config.go:82)
  默认就是 `proxy_host_output: false`
- [internal/app/app.go](/home/projects/minimalist/internal/app/app.go:1008)
  当前只有在显式开启时才挂 `OUTPUT` 跳转
- [internal/runtime/runtime.go](/home/projects/minimalist/internal/runtime/runtime.go:219)
  已有 fake-ip、DoH、nameserver-policy、direct-nameserver 这套基础 DNS 模板
- [internal/runtime/runtime.go](/home/projects/minimalist/internal/runtime/runtime.go:248)
  已有 `proxy-groups` 生成逻辑
- [internal/app/app.go](/home/projects/minimalist/internal/app/app.go:631)
  已有规则 target 校验入口，但还不够早

### 当前真正的问题

- 核心主路径和增强项没有分层
- “不 DNS 泄露”还不是一条被测试固定的契约
- “宿主机默认直连”还不是一条被测试固定的契约
- 模板虽然能用，但还没有正式收敛成“成熟常规模板 + 个人规则叠加”

### 这轮不做什么

- 不扩协议
- 不自研面板
- 不把订阅补成一等公民
- 不优先做 `core-upgrade-alpha`

## 架构结论

### 目标结构

```text
手动节点
    +
成熟常规模板
    +
你的个人规则分流
    |
    v
runtime config
    |
    +--> LAN DNS 走 Mihomo 路径
    +--> 宿主机默认直连
    +--> TProxy 只服务旁路由主路径
```

### 核心原则

- ready 判定围绕手动节点，不围绕订阅
- `proxy_host_output: false` 是默认安全姿态
- DNS 劫持只针对 LAN
- 配置模板先求成熟、再求灵活

## Code Quality Review

### Issue 1

`setup/start/apply-rules` 的核心成功条件要统一回“启用的手动节点”。

理由：

- 这才符合真实需求
- 可以把订阅从主路径里拿掉
- 会让代码、文档、测试重新说同一种语言

### Issue 2

DNS 配置现在是“已有基础”，不是“已被明确设计完成”。

理由：

- 现在有 fake-ip、DoH、policy，不等于已经形成稳定模板契约
- 需要围绕“不 DNS 泄露”重新检查和固定顺序

### Issue 3

宿主机默认直连现在更多是“默认值”，还不是“正式产品承诺”。

理由：

- 一旦后续有人动模板或 `apply-rules`，这条最容易悄悄被破坏
- 必须变成测试和文档都锁死的行为

## Test Review

```text
CODE PATHS
[+] internal/app/app.go
  ├── main-path readiness
  │   ├── [GAP] enabled manual node -> ready
  │   ├── [GAP] subscription-only -> not core-ready
  │   └── [GAP] no manual node -> reject
  ├── AddRule()
  │   ├── [GAP] disabled manual node target -> reject
  │   └── [GAP] enabled manual node target -> allow
  └── RenameNode()
      ├── [GAP] reserved name -> reject
      ├── [GAP] duplicate name -> reject
      └── [GAP] valid rename -> preserve behavior

[+] internal/runtime/runtime.go
  ├── buildRuntimeConfig()
  │   ├── [GAP] mature baseline template ordering fixed
  │   ├── [GAP] DNS path contract fixed
  │   ├── [GAP] personal rule layer ordering fixed
  │   └── [GAP] host-safe defaults fixed

USER FLOWS
[+] manual-node deployment
  ├── [GAP] install -> import manual nodes -> setup -> start
  ├── [GAP] LAN DNS follows Mihomo path
  └── [GAP] host traffic remains direct by default
```

## Performance Review

这一轮性能不是主风险。

真正要防的是：

- 错误配置导致反复 restart
- DNS 路径错误导致难排查的网络问题
- 模板分层不清导致长期维护成本飙升

## 执行计划

### PR-1：Main Path Narrowing

目标：

- 把主路径正式收回“手动节点 + 自定义规则”

Files:

- [internal/app/app.go](/home/projects/minimalist/internal/app/app.go)
- [internal/app/app_test.go](/home/projects/minimalist/internal/app/app_test.go)
- [docs/PRD.md](/home/projects/minimalist/docs/PRD.md)
- [docs/TASKS.md](/home/projects/minimalist/docs/TASKS.md)

Tasks:

- [ ] 统一 main-path ready 判定
- [ ] subscription-only 不再被当成核心成功路径
- [ ] 前移 rule target 校验
- [ ] 前移 node rename 校验

### PR-2：DNS + Host Safety Contract

目标：

- 把“不 DNS 泄露、默认不影响宿主机网络”固定成模板和测试契约

Files:

- [internal/runtime/runtime.go](/home/projects/minimalist/internal/runtime/runtime.go)
- [internal/runtime/runtime_test.go](/home/projects/minimalist/internal/runtime/runtime_test.go)
- [internal/app/app.go](/home/projects/minimalist/internal/app/app.go)
- [internal/app/app_test.go](/home/projects/minimalist/internal/app/app_test.go)

Tasks:

- [ ] 审视并固定成熟常规模板的 DNS 结构
- [ ] 固定 LAN DNS hijack 语义
- [ ] 固定 `proxy_host_output: false` 的默认安全姿态
- [ ] 为 DNS 路径和 host-safe 行为补 focused tests

### PR-3：Template + Rules Layering

目标：

- 做一版成熟常规模板，并明确你的个人规则叠加层

Files:

- [internal/runtime/runtime.go](/home/projects/minimalist/internal/runtime/runtime.go)
- [internal/runtime/runtime_test.go](/home/projects/minimalist/internal/runtime/runtime_test.go)
- [README.md](/home/projects/minimalist/README.md)
- [docs/README_FLOWS.md](/home/projects/minimalist/docs/README_FLOWS.md)

Tasks:

- [ ] 固定成熟基线模板
- [ ] 固定个人规则层的插入位置
- [ ] 固定规则 tail 顺序
- [ ] 把模板层次写进文档

### PR-4：Operator Surface Alignment

目标：

- 让 CLI/help/runbook 和真实主路径一致

Files:

- [internal/cli/cli.go](/home/projects/minimalist/internal/cli/cli.go)
- [internal/cli/cli_test.go](/home/projects/minimalist/internal/cli/cli_test.go)
- [docs/CUTOVER.md](/home/projects/minimalist/docs/CUTOVER.md)
- [docs/STATUS.md](/home/projects/minimalist/docs/STATUS.md)
- [docs/NEXT_STEP.md](/home/projects/minimalist/docs/NEXT_STEP.md)

Tasks:

- [ ] 暴露 `minimalist nodes test`
- [ ] 在 help 里说明 `verify-runtime-assets`
- [ ] 写短版 restart / reboot smoke runbook
- [ ] 同步状态文档

### PR-5：Enhancements Later

后置增强项：

- `core-upgrade-alpha`
- 订阅体验
- 规则仓库增强

## Failure Modes

| Codepath | Failure | 应对 |
|---|---|---|
| manual-node setup | 没有手动节点却进入主路径 | 早失败 |
| DNS path | LAN DNS 泄露 | 模板 + tests 固定 |
| host output | 宿主机流量被误接管 | 默认关闭 + tests 固定 |
| rule mutation | 写入非法 target | mutation 阶段直接拒绝 |
| reboot | 规则或 controller 没恢复 | runbook 验证 |

## NOT in Scope

- 把订阅重新做成核心主路径
- 优先做 `core-upgrade-alpha`
- 自研面板
- 更大的网络边界扩展

## Parallelization

顺序执行更合适。

- PR-1 和 PR-2 都会碰 `internal/app`
- PR-2 和 PR-3 都会碰 `internal/runtime`
- 这是同一条稳定主线，不值得并行拆脏

结论：

```text
PR-1 -> PR-2 -> PR-3 -> PR-4
PR-5 放后面
```

## 完成定义

这一轮完成，不看“多了几个命令”，只看：

- 主路径明确是手动节点 + 自定义规则
- DNS 不泄露的契约被固定
- 宿主机默认不受影响的契约被固定
- 模板成熟且规则层次清晰
- 日常运维入口和文档与真实行为一致
