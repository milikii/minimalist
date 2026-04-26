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

## 2026-04-26 render_config 收口完成后转向服务编排链

- `lib/render.sh` 的 `render_config` 当前已按职责块拆出访问/控制面、DNS 基础配置、显式代理认证、provider/group 组装、rules 尾段
- 该函数保留为编排入口，但当前主复杂度已从大段配置拼装下降为“上下文准备 + 块调用 + 权限收尾”
- 阶段 5 的下一优先级转向 `mihomo` 主脚本中的运行前准备与服务启停编排，优先 `prepare_runtime_assets`、`start_service_command`、`restart_service_command`、`enable_and_start_service_command`
- 原因不是功能扩展，而是这些命令共享同一套高风险前置准备链，且已有 `service_mock` 回归基础，适合继续按职责块收口

## 2026-04-26 服务启停编排收口后转向 setup/repair

- `prepare_runtime_assets` 当前已拆出运行配置/服务/sysctl 写入、geodata 自动修复、缺核心时自动安装三段职责块
- `start_service_command`、`restart_service_command`、`enable_and_start_service_command` 已共用同一条 prepared systemctl 编排链
- 阶段 5 的下一优先级转向 `full_setup` 与 `repair_command`
- 原因是它们仍然保留安装、修复、WebUI 退化、节点为空时启动决策等分支，是当前部署侧剩余的主要编排热点

## 2026-04-26 setup/repair 收口后优先转向可验证的订阅刷新链

- `full_setup` 与 `repair_command` 当前已收口为“公共上下文准备 + 运行时资产 + WebUI 分支 + 定时维护 + 服务状态收尾”的职责块编排
- 下一优先级不直接转向交互式流程，而是先处理已有 `service_mock` 覆盖的 `update_subscriptions_command`
- 原因是 `router_wizard`、`import_links` 主要依赖交互输入，当前更保守、可验证的下一步是先收口非交互的 provider 缓存刷新链
- 待订阅刷新链收口后，再评估 `router_wizard`、`import_links` 与 `main` 的后续收口顺序

## 2026-04-26 订阅刷新收口后优先转向 import_links

- `update_subscriptions_command` 当前已收口为“遍历启用订阅 + 单订阅刷新 + 结果统计 + 状态收尾”的编排入口
- `service_mock` 已补到订阅刷新成功、无启用订阅和 curl 失败记账分支，当前可验证非交互链已暂时收口
- 下一优先级确定为 `import_links`，暂不先做 `router_wizard`
- 原因是 `import_links` 已具备 `/dev/stdin` fallback，并且复用 `scan-uris` 与 `append-node` 既有能力，更容易先补 focused tests 再做职责块收口
- `router_wizard` 继续排在其后，待 `import_links` 收口后再处理网络参数采集与 env 写入编排

## 2026-04-26 import_links 收口后优先转向 router_wizard

- `import_links` 当前已收口为“准备输入源 + 收集 URI + scan + 处理结果 + 状态收尾”的编排入口
- 本轮顺手修正了非 TTY 下的 stdin fallback 真相，避免 `import-links` 在管道输入场景错误读取 `/dev/tty` 或误消费 scan 结果文件
- `smoke` 已补到 `import-links` 的 stdin 成功导入、协议跳过和无有效节点失败分支，当前交互导入链已有最小回归护栏
- 下一优先级确定为 `router_wizard`
- 原因是它仍保留当前配置展示、多个输入字段校验和集中 env 写盘，是当前交互链里剩余最大的长编排块

## 2026-04-26 router_wizard 收口后优先转向 main

- `router_wizard` 当前已收口为“展示当前配置 + 收集输入 + snapshot + 写基础 env + 处理派生网段 + 写 bypass + 状态收尾”的编排入口
- `smoke` 已补到 `router-wizard` 的 stdin 成功更新、入口网段识别失败保留现值和非法宿主机接管值失败分支
- 下一优先级确定为 `main`
- 原因是 `main` 仍保留 CLI 参数分发、interactive fallback 和 alpha/stable update 的内联 snapshot 分支，是 `mihomo` 主脚本剩余最集中的入口编排热点
- `router_wizard` 收口后，继续优先处理单文件内剩余热点，比提前切到 `scripts/statectl.py` 更保守、更易验证

## 2026-04-26 main 收口后优先转向 interactive_menu

- `main` 当前已收口为“处理默认入口 + 分发到 workflow/maintenance helper + 未知命令兜底”的编排入口
- `smoke` / `service_mock` 已补到 `main` 的无参数非 TTY usage、未知命令、`update-alpha --quiet` 快照与 `update-stable` 输出分支
- 下一优先级确定为 `interactive_menu`
- 原因是 `interactive_menu` 仍保留一级菜单、二级子菜单、`press_enter` 收尾和失败回到菜单的集中编排，是 `mihomo` 主脚本剩余最显眼的交互热点
- 当前已有 `service_mock` 的菜单回归基础，先继续收口菜单编排比提前转向 `scripts/statectl.py` 更保守

## 2026-04-26 interactive_menu 收口后转向 scripts/statectl.py 协议解析链

- `interactive_menu` 当前已收口为“展示菜单 + 读取 action + 调用分发 helper + 处理退出”的编排入口
- `service_mock` 已补到菜单健康检查失败后回到菜单、顶层无效选择和部署子菜单无效选择分支
- 下一优先级确定为 `scripts/statectl.py` 的协议解析链，优先 `parse_uri_info`、`uri_info` 与 `scan_uri_rows`
- 原因不是扩协议，而是该文件仍保留协议解析、scan 结果归一化和 provider 渲染之间的过渡期耦合
- 当前已有 `smoke` 的 `scan-uris` / `import-links` / `render-config` 回归基础，先收口协议解析链比直接碰 provider 渲染更保守

## 2026-04-26 协议解析链收口后转向 provider_item_from_node

- `parse_uri_info` 当前已收口为“准备 scheme + 选择 parser + 调用 parser”的编排入口
- `uri_info` 当前已收口为“取 scheme + parse + 成功/失败归一化”的编排入口
- `scan_uri_rows` 当前已收口为“遍历可扫描 URI + 组装 scan row”的编排入口
- 下一优先级确定为 `provider_item_from_node`，优先 vless/trojan/ss/vmess 渲染分支与 TLS/network 选项组合
- 原因不是调整 provider 真相，而是该函数仍集中承担多协议 YAML 渲染和 xhttp/download-settings 收尾，是 `scripts/statectl.py` 剩余最显眼的协议热点
- 当前已有 `smoke` 的协议渲染与 `render-config` 回归基础，先收口 provider 渲染链比继续下钻命名/去重 helper 更保守

## 2026-04-26 provider 传输层选项 helper 收口后转向 provider 组装尾段

- `provider_item_from_node` 当前已收口为“parse URI + 选择 renderer + 调用 renderer”的编排入口
- vless/trojan/ss/vmess 协议渲染段以及 xhttp download-settings 收尾段已独立
- `apply_common_tls_fields`、`apply_network_opts` 与 `xhttp_download_settings_from_mapping` 已完成当前最小收口
- 下一优先级确定为 `build_vless_provider_item` 与 `render_vless_xhttp_opts`
- 原因不是扩协议，而是 provider 输出链当前剩余的复杂度已主要集中在 VLESS 组装尾段
- 当前已有 `smoke` 的 reality/ws/plugin/xhttp download-settings 渲染护栏，先收口这些尾段比继续拆 `render_provider` 更保守

## 2026-04-26 codex 会话产物不进入版本控制

- 会话落盘统一为 `codex.md`，只保留最近三轮会话，不作为仓库真相文档
- `.codex/`、`codex.md` 与遗留 `codex.log` 视为本地执行产物，默认不提交到 git
