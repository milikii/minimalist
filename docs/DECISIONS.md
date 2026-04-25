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

## 2026-04-26 阶段 5 改为按职责块收口

- 阶段 5 继续遵守“不改行为、先降复杂度”的原则，但默认推进粒度从单行 helper 抽取改为职责块收口
- 当前已完成第一轮共用逻辑收口：
  - 运行态与审计展示块：`status`、`runtime-audit`、`healthcheck`、`diagnose`、`audit_installation`
  - 安装与同步块：`install_webui`、`install_project`、`install_project_sync`、`disable_project_sync`、`finalize_project_install`
  - manager sync unit 渲染块：通用 render/write、sections、timer static settings、service body
- 自本决策生效后，阶段 5 默认不再按“第 N 刀”或 manager sync unit 单行 helper 抽取推进
- 后续优先级固定为：
  - 先收口 `lib/render.sh` 的 `render_config` 块级边界
  - 再收口 `mihomo` 主脚本中的长编排函数
  - 最后为 `scripts/statectl.py` 退化成更小状态工具做准备
- 只有当某个 helper 抽取能直接消除更大块重复逻辑时，才允许继续新增 helper
- 目标是降低真实复杂度，而不是继续堆积形式上的更细拆分

## 2026-04-26 codex 会话产物不进入版本控制

- 会话落盘统一为 `codex.md`，只保留最近三轮会话，不作为仓库真相文档
- `.codex/`、`codex.md` 与遗留 `codex.log` 视为本地执行产物，默认不提交到 git
