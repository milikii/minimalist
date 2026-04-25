# 当前决策

## 2026-04-25 Debian NAS / IPv4 旁路由保持为唯一承诺范围

- 项目继续只承诺 Debian NAS、宿主机部署、IPv4 旁路由、`iptables + TProxy`
- 不为了通用性补 OpenWrt / firewall4 / nftables 抽象
- `nas-single-lan-dualstack` 仅兼容保留，不作为真双栈能力表达

## 2026-04-25 订阅 provider 缓存是真相源，订阅节点只做只读枚举

- 订阅更新结果落盘到 `proxy_providers/subscriptions/*.txt`
- 运行配置优先消费 provider 缓存，而不是把订阅节点重新渲染进 `manual.txt`
- `mihomo nodes` 只保留手动节点交互，避免订阅缓存节点与运行态脱节
- ACL / 自定义规则的具名目标只允许指向手动节点

## 2026-04-25 优先暴露官方字段，不补自研替代层

- 已接入并保留的字段包括：
  - `lan-disallowed-ips`
  - `authentication`
  - `skip-auth-prefixes`
  - `external-ui-name`
  - `external-ui-url`
  - `external-controller-cors.allow-origins`
  - `external-controller-cors.allow-private-network`
- 这里的 `authentication` / `skip-auth-prefixes` 当前只覆盖显式代理端口，不是控制面认证

## 2026-04-25 暂不接入 external-controller-tls

- 当前项目默认控制面仅绑定本机地址，主线目标不是把控制面长期暴露到局域网或公网
- 一旦接入 `external-controller-tls`，项目就需要同时承担证书来源、私钥落盘、权限、轮换和故障排查边界
- 这会扩大当前阶段的安全与运维真相边界，不符合“小步闭环 + 默认分支持续可用”的原则
- 结论：当前明确不实现控制面 TLS 证书管理；只有当主线要求控制面长期对外开放时，再单独开阶段评估

## 2026-04-25 运行态真相改造按阶段 4 推进

- 静态意图继续保存在 `settings.env`、`router.env` 和本项目状态文件
- 运行态状态下一阶段优先从 Mihomo REST API 读取
- 阶段 4 已完成最小收口：
  - `mihomo status` / `mihomo runtime-audit` 的“当前模式”读取
  - 两者的最小策略组摘要读取
  - 两者的最小控制面运行态摘要读取
- 不一次性扩到全部策略组与摘要

## 2026-04-25 阶段 5 先做展示层结构收口

- 第一刀先抽离 `status` / `runtime-audit` 共用的运行态摘要 helper
- 第二刀再抽离 `status` / `runtime-audit` 共用的控制面静态信息展示 helper
- 第三刀再抽离 `status` / `runtime-audit` 共用的网络与访问静态信息展示 helper
- 第四刀再抽离 `status` / `runtime-audit` 共用的模板/规则预设/IPv6 展示 helper
- 第五刀再抽离 `status` / `runtime-audit` 共用的计数类与节点统计展示 helper
- 第六刀再抽离 `status` 的推荐下一步判断 helper
- 第七刀再抽离 `status` 的警告与收尾展示 helper
- 第八刀再抽离 `status` 的同步与端口展示 helper
- 第九刀再抽离 `status` 的 WebUI / 控制面密钥入口展示 helper
- 第十刀收口 `status` 推荐下一步所需的重复计数解析
- 第十一刀再抽离 `status` 的基础概览展示 helper
- 第十二刀再抽离 `status` 的基础状态采集 helper
- 第十三刀再抽离 `runtime-audit` 的探测与流量摘要展示 helper
- 第十四刀再抽离 `runtime-audit` 的告警与定时器展示 helper
- 第十五刀再抽离 `runtime-audit` 的基础概览展示 helper
- 第十六刀再抽离 `runtime-audit` 的基础状态采集 helper
- 第十七刀再抽离 `runtime-audit` 的探测状态采集 helper
- 第十八刀再抽离 `runtime-audit` 的健康摘要收尾 helper
- 第十九刀再抽离 `runtime-audit` 的告警与定时器状态采集 helper
- 第二十刀再抽离 `healthcheck` 的端口监听检查 helper
- 第二十一刀再抽离 `healthcheck` 的探测检查 helper
- 第二十二刀再抽离 `diagnose` 的配置摘要展示 helper
- 第二十三刀再抽离 `diagnose` 的 systemd / listeners / logs 分段展示 helper
- 目标是降低重复逻辑，不改变用户可见输出
- 后续仍按“更小、更保守、可验证”的顺序继续抽离共用展示块，不直接做大拆分

## 2026-04-25 codex 会话产物不进入版本控制

- `codex.log` 只保留最近三轮会话，不作为仓库真相文档
- `.codex/` 与 `codex.log` 视为本地执行产物，默认不提交到 git
