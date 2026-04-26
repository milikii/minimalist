# 当前状态

## 当前主线

- 当前主线已进入阶段 5，推进粒度已从单行 helper 抽取切换到职责块收口。
- 阶段 5 第一轮共用逻辑收口已完成：运行态/审计展示、安装与同步链路、manager sync unit 渲染链均已完成当前最小收口。
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
  - 运行态与审计展示块已完成一轮共用逻辑收口，覆盖 `status`、`runtime-audit`、`healthcheck`、`diagnose`、`audit_installation`
  - 安装与同步块已完成一轮共用逻辑收口，覆盖 `install_webui`、`install_project`、`install_project_sync`、`disable_project_sync`、`finalize_project_install`
  - manager sync unit 渲染链已完成当前最小收口，通用 render/write、sections、timer static settings、service body 已完成抽离
  - `lib/render.sh` 的 `render_config` 已完成当前块级收口：
    - 访问/控制面基础段已独立
    - DNS 与基础配置段已独立
    - 显式代理认证段已独立
    - provider / proxy-group 组装段已独立
    - rules 尾段已独立
  - `render_config` 当前已收敛为“准备上下文 + 调用职责块 + 权限收尾”的编排函数
  - `render_config` 的关键输出顺序已补 focused tests，覆盖 access / dns / auth / provider / rules 五段相对位置
  - `mihomo` 的运行前准备与服务启停编排已完成当前最小收口：
    - 运行配置/服务/sysctl 写入段已独立
    - geodata 自动修复段已独立
    - 缺核心时自动安装段已独立
    - `start` / `restart` / `enable-start` 已共用同一条 prepared systemctl 编排链
  - 服务侧 focused tests 已补到 `start` / `restart` / `enable-start` 的前置资产顺序，以及缺核心时自动安装路径
  - `mihomo` 的部署与修复编排已完成当前最小收口：
    - `setup` / `repair` 的公共上下文准备段已独立
    - `repair` 的 WebUI 修复退化分支已独立
    - `setup` 的定时维护配置段已独立
    - `setup` 的最终服务状态决策段已独立
  - 服务侧 focused tests 已补到 `repair` 的 WebUI 跳过重装/失败退化，以及 `setup` 的定时器落盘、有节点启动、无节点停用分支
  - `mihomo` 的订阅刷新编排已完成当前最小收口：
    - 单订阅 provider 缓存刷新段已独立
    - 成功/失败记账与提示输出段已独立
    - `update-subscriptions` 已退回为“遍历启用订阅 + 结果统计 + 状态收尾”的编排入口
  - 服务侧 focused tests 已补到 `update-subscriptions` 的成功刷新、无启用订阅和 curl 失败记账分支
  - `mihomo` 的交互导入编排已完成当前最小收口：
    - 输入源 fd 选择与 stdin fallback 已独立并修正
    - URI 收集段已独立
    - 支持节点提示/追加段已独立
    - 不支持协议告警与 scan 结果循环已独立
    - `import-links` 已退回为“准备输入源 + 收集 URI + scan + 处理结果 + 状态收尾”的编排入口
  - `smoke` 已补到 `import-links` 的 stdin 成功导入、协议跳过和无有效节点失败分支
  - `install_webui` 的解压失败告警输出已恢复，与重构前真相一致
  - 当前行为与输出文本保持与重构前真相一致

## 质量状态

- 默认分支最近闭环均已通过回归并提交
- 当前回归入口：`tests/smoke.sh`、`tests/service_mock.sh`
- 本轮验证结果：
  - `2026-04-26` 执行 `bash tests/smoke.sh`，通过
  - `2026-04-26` 执行 `bash tests/service_mock.sh`，通过

## 当前风险与限制

- `mihomo` 中的交互式长编排仍有剩余热点，当前下一优先级已转向 `router_wizard`
- manager sync unit 周边已出现低收益单行 helper 粒度，后续默认不再沿该方向继续细拆
- `scripts/statectl.py` 仍保留过渡期协议解析逻辑，尚未退化为更小的状态工具
- `nas-single-lan-dualstack` 仅兼容保留，不代表项目已支持真双栈旁路由
