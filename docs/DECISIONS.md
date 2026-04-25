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
- 第二十四刀再抽离 `healthcheck` 的基础状态检查 helper
- 第二十五刀再抽离 `audit_installation` 的基础文件存在性检查 helper
- 第二十六刀再抽离 `audit_installation` 的 nodes/rules 渲染漂移检查 helper
- 第二十七刀再抽离 `audit_installation` 的 ACL / 规则预设检查 helper
- 第二十八刀再抽离 `audit_installation` 的 timer / GeoSite 检查 helper
- 第二十九刀再抽离 `audit_installation` 的成功收尾 helper
- 第三十刀再抽离 `install_geosite_dat` 的成功安装收尾 helper
- 第三十一刀再抽离 `install_webui` 的下载阶段 helper
- 第三十二刀再抽离 `install_webui` 的解压与源码目录识别 helper
- 第三十三刀再抽离 `install_webui` 的部署与持久化收尾 helper
- 第三十四刀再抽离 `install_webui` 的失败收尾 helper
- 第三十五刀再抽离 `install_webui` 的临时工作区清理 helper
- 第三十六刀再抽离 `install_webui` 的参数与目标解析 helper
- 第三十七刀再抽离 `install_webui` 的临时工作区准备 helper
- 第三十八刀再抽离 `install_project_sync` 的入参校验 helper
- 第三十九刀再抽离 `install_project_sync` 的设置持久化 helper
- 第四十刀再抽离 `disable_project_sync` 的设置重置 helper
- 第四十一刀再抽离 `disable_project_sync` 的运行时清理 helper
- 第四十二刀再抽离 `install_project_sync` 的 systemd 激活收尾 helper
- 第四十三刀再抽离 `install_project_sync` 的成功提示收尾 helper
- 第四十四刀再抽离 `write_manager_sync_units` 的 service unit 写入 helper
- 第四十五刀再抽离 `write_manager_sync_units` 的 timer unit 写入 helper
- 第四十六刀再抽离 `disable_project_sync` 的成功提示收尾 helper
- 第四十七刀再抽离 `install_project` 的安装树复制与元数据清理 helper
- 第四十八刀再抽离 `install_project` 的命令链接与成功提示收尾 helper
- 第四十九刀再抽离 `finalize_project_install` 的命令链接写入 helper
- 第五十刀再抽离 `finalize_project_install` 的可执行权限设置 helper
- 第五十一刀再抽离 `finalize_project_install` 的成功提示 helper
- 第五十二刀再抽离 `prepare_project_install_tree` 的目标目录重建与源码复制 helper
- 第五十三刀再抽离 `prepare_project_install_tree` 的元数据清理 helper
- 第五十四刀再抽离 `cleanup_project_install_tree_metadata` 的 VCS 元数据清理 helper
- 第五十五刀再抽离 `cleanup_project_install_tree_metadata` 的 Python 缓存清理 helper
- 第五十六刀再抽离 `cleanup_project_install_tree_metadata` 的备份垃圾清理 helper
- 第五十七刀再抽离 `cleanup_project_sync_runtime` 的 unit 文件删除 helper
- 第五十八刀再抽离 `activate_project_sync_runtime` / `cleanup_project_sync_runtime` 的 systemd reload helper
- 第五十九刀再抽离 `cleanup_project_sync_runtime` 的 timer 停用 helper
- 第六十刀再抽离 `activate_project_sync_runtime` 的 timer 启用 helper
- 第六十一刀再抽离 `persist_project_sync_settings` 的 MANAGER_SYNC 三连写 helper
- 第六十二刀再让 `reset_project_sync_settings` 复用 `write_manager_sync_settings`
- 第六十三刀再抽离 `validate_project_sync_inputs` 的同步间隔校验 helper
- 第六十四刀再抽离 `validate_project_sync_inputs` 的 git 工作树校验 helper
- 第六十五刀先抽离 `validate_project_sync_inputs` 的源码入口校验 helper
- 目标是降低重复逻辑，不改变用户可见输出
- 后续仍按“更小、更保守、可验证”的顺序继续抽离共用展示块，不直接做大拆分

## 2026-04-25 codex 会话产物不进入版本控制

- `codex.log` 只保留最近三轮会话，不作为仓库真相文档
- `.codex/` 与 `codex.log` 视为本地执行产物，默认不提交到 git
