# 当前状态

## 当前主线

- 当前主线已进入阶段 5，已完成第九十二刀：`OnBootSec=` 行已抽到共用 helper。
- 项目权威文档基线已补齐并生效：`STATUS.md`、`NEXT_STEP.md`、`DECISIONS.md`、`ARCHITECTURE.md`。

## 当前真相

- 默认分支：`main`
- 当前定位仍是 Debian NAS / IPv4 旁路由 / `iptables + TProxy`
- 宿主机默认直连，`PROXY_HOST_OUTPUT=0`
- 控制面默认仅绑定 `127.0.0.1:${CONTROLLER_PORT:-19090}`
- 订阅原始 provider 缓存位于 `proxy_providers/subscriptions/*.txt`
- `mihomo nodes` 当前只展示手动节点；订阅节点只保留只读枚举状态

## 阶段结论

- 阶段 1：已完成，README 已与 Debian NAS / IPv4 旁路由定位对齐
- 阶段 2：已完成主线收口，订阅缓存已成为 provider 输入真相源，订阅节点退出主节点交互路径
- 阶段 3：已完成
  - 已接入 `lan-disallowed-ips`
  - 已接入 `authentication`
  - 已接入 `skip-auth-prefixes`
  - 已接入 `external-ui-name`
  - 已接入 `external-ui-url`
  - 已接入 `external-controller-cors.allow-origins`
  - 已接入 `external-controller-cors.allow-private-network`
  - `external-controller-tls` 已明确暂缓，不进入当前阶段实现
- 阶段 4：已完成
  - `mihomo status` 的“当前模式”已优先读取 Mihomo REST API `/configs`
  - `mihomo runtime-audit` 的“当前模式”已优先读取 Mihomo REST API `/configs`
  - `mihomo status` 已优先读取 Mihomo REST API `/proxies`，输出最小策略组运行态摘要
  - `mihomo runtime-audit` 已优先读取 Mihomo REST API `/proxies`，输出最小策略组运行态摘要
  - `mihomo status` 已优先读取 Mihomo REST API `/version`，输出最小控制面运行态摘要
  - `mihomo runtime-audit` 已优先读取 Mihomo REST API `/version`，输出最小控制面运行态摘要
  - 控制面不可达时会回退到本地 `config.yaml`
  - 控制面不可达时，`status` / `runtime-audit` 的策略组摘要会显示“未获取”，不影响其他状态输出
  - 控制面不可达时，`status` / `runtime-audit` 的控制面运行态摘要会显示“未获取”
- 阶段 5：已开始
  - `status` / `runtime-audit` 的运行态摘要拼装逻辑已抽到共用 helper
  - `status` / `runtime-audit` 的控制面静态信息展示已抽到共用 helper
  - `status` / `runtime-audit` 的网络与访问静态信息展示已抽到共用 helper
  - `status` / `runtime-audit` 的模板/规则预设/IPv6 展示已抽到共用 helper
  - `status` / `runtime-audit` 的计数类与节点统计展示已抽到共用 helper
  - `status` 的推荐下一步判断已抽到共用 helper
  - `status` 的警告与收尾展示已抽到共用 helper
  - `status` 的同步与端口展示已抽到共用 helper
  - `status` 的 WebUI / 控制面密钥入口展示已抽到共用 helper
  - `status` 推荐下一步所需的计数预处理已抽到共用 helper
  - `status` 的基础概览展示已抽到共用 helper
  - `status` 的基础状态采集已抽到共用 helper
  - `runtime-audit` 的探测与流量摘要展示已抽到共用 helper
  - `runtime-audit` 的告警与定时器展示已抽到共用 helper
  - `runtime-audit` 的基础概览展示已抽到共用 helper
  - `runtime-audit` 的基础状态采集已抽到共用 helper
  - `runtime-audit` 的探测状态采集已抽到共用 helper
  - `runtime-audit` 的健康摘要收尾已抽到共用 helper
  - `runtime-audit` 的告警与定时器状态采集已抽到共用 helper
  - `healthcheck` 的端口监听检查已抽到共用 helper
  - `healthcheck` 的探测检查已抽到共用 helper
  - `diagnose` 的配置摘要展示已抽到共用 helper
  - `diagnose` 的 systemd / listeners / logs 分段展示已抽到共用 helper
  - `healthcheck` 的基础状态检查已抽到共用 helper
  - `audit_installation` 的基础文件存在性检查已抽到共用 helper
  - `audit_installation` 的 nodes/rules 渲染漂移检查已抽到共用 helper
  - `audit_installation` 的 ACL / 规则预设检查已抽到共用 helper
  - `audit_installation` 的 timer / GeoSite 检查已抽到共用 helper
  - `audit_installation` 的成功收尾已抽到共用 helper
  - `install_geosite_dat` 的成功安装收尾已抽到共用 helper
  - `install_webui` 的下载阶段已抽到共用 helper
  - `install_webui` 的解压与源码目录识别已抽到共用 helper
  - `install_webui` 的部署与持久化收尾已抽到共用 helper
  - `install_webui` 的失败收尾已抽到共用 helper
  - `install_webui` 的临时工作区清理已抽到共用 helper
  - `install_webui` 的参数与目标解析已抽到共用 helper
  - `install_webui` 的临时工作区准备已抽到共用 helper
  - `install_project_sync` 的入参校验已抽到共用 helper
  - `install_project_sync` 的设置持久化已抽到共用 helper
  - `disable_project_sync` 的设置重置已抽到共用 helper
  - `disable_project_sync` 的运行时清理已抽到共用 helper
  - `install_project_sync` 的 systemd 激活收尾已抽到共用 helper
  - `install_project_sync` 的成功提示收尾已抽到共用 helper
  - `write_manager_sync_units` 的 service unit 写入已抽到共用 helper
  - `write_manager_sync_units` 的 timer unit 写入已抽到共用 helper
  - `disable_project_sync` 的成功提示收尾已抽到共用 helper
  - `install_project` 的安装树复制与元数据清理已抽到共用 helper
  - `install_project` 的命令链接与成功提示收尾已抽到共用 helper
  - `finalize_project_install` 的命令链接写入已抽到共用 helper
  - `finalize_project_install` 的可执行权限设置已抽到共用 helper
  - `finalize_project_install` 的成功提示已抽到共用 helper
  - `prepare_project_install_tree` 的目标目录重建与源码复制已抽到共用 helper
  - `prepare_project_install_tree` 的元数据清理已抽到共用 helper
  - `cleanup_project_install_tree_metadata` 的 VCS 元数据清理已抽到共用 helper
  - `cleanup_project_install_tree_metadata` 的 Python 缓存清理已抽到共用 helper
  - `cleanup_project_install_tree_metadata` 的备份垃圾清理已抽到共用 helper
  - `cleanup_project_sync_runtime` 的 unit 文件删除已抽到共用 helper
  - `activate_project_sync_runtime` / `cleanup_project_sync_runtime` 的 systemd reload 已抽到共用 helper
  - `cleanup_project_sync_runtime` 的 timer 停用已抽到共用 helper
  - `activate_project_sync_runtime` 的 timer 启用已抽到共用 helper
  - `persist_project_sync_settings` 的 MANAGER_SYNC 三连写已抽到共用 helper
  - `reset_project_sync_settings` 已复用 `write_manager_sync_settings`
  - `validate_project_sync_inputs` 的同步间隔校验已抽到共用 helper
  - `validate_project_sync_inputs` 的 git 工作树校验已抽到共用 helper
  - `validate_project_sync_inputs` 的源码入口校验已抽到共用 helper
  - `validate_project_sync_inputs` 的源码树校验已抽到共用 helper
  - `install_project_sync` 的安装与设置前置步骤已抽到共用 helper
  - `install_project_sync` 的激活与成功提示收尾已抽到共用 helper
  - `disable_project_sync` 的重置前置已抽到共用 helper
  - `disable_project_sync` 的清理与成功提示收尾已抽到共用 helper
  - `disable_project_sync` 的总编排已抽到共用 helper
  - `write_manager_sync_service_unit` 的 unit 内容已抽到共用 helper
  - `write_manager_sync_timer_unit` 的 unit 内容已抽到共用 helper
  - `write_manager_sync_units` 的 service/timer 写入编排已抽到共用 helper
  - `render_manager_sync_service_unit` 的 ConditionPathExists 内容已抽到共用 helper
  - `render_manager_sync_service_unit` 的 Service 段内容已抽到共用 helper
  - `render_manager_sync_timer_unit` 的 Timer 段内容已抽到共用 helper
  - `render_manager_sync_timer_unit` 的 Install 段内容已抽到共用 helper
  - manager sync unit 的通用文件写入已抽到共用 helper
  - manager sync unit 的通用 Unit 头部已抽到共用 helper
  - `render_manager_sync_service_unit` 的 sections 已抽到共用 helper
  - `render_manager_sync_timer_unit` 的 sections 已抽到共用 helper
  - manager sync unit 的通用 render 包装已抽到共用 helper
  - manager sync unit 的通用 render+write 编排已抽到共用 helper
  - manager sync unit 的通用 sections 已先接入 service
  - manager sync unit 的通用 sections 已接入 timer
  - `ConditionPathExists=` 的通用输出已抽到共用 helper
  - timer 静态设置已抽到共用 helper
  - timer 动态间隔行已抽到共用 helper
  - `WorkingDirectory=` 行已抽到共用 helper
  - `ExecStart=` 行已抽到共用 helper
  - `OnBootSec=` 行已抽到共用 helper
  - `install_webui` 的解压失败告警输出已恢复，与重构前真相一致
  - 当前行为与输出文本保持与重构前真相一致

## 质量状态

- 默认分支最近闭环均已通过回归并提交
- 当前回归入口：`tests/smoke.sh`、`tests/service_mock.sh`
- 本轮验证结果：
  - `2026-04-25` 执行 `bash tests/smoke.sh`，通过
  - `2026-04-25` 执行 `bash tests/service_mock.sh`，通过

## 当前风险与限制

- timer 静态设置里的 `Unit=mihomo-manager-sync.service` 行仍内联在 helper 中，阶段 5 后续可继续推进
- `scripts/statectl.py` 仍保留过渡期协议解析逻辑，尚未退化为更小的状态工具
- `nas-single-lan-dualstack` 仅兼容保留，不代表项目已支持真双栈旁路由
