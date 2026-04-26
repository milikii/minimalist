# 当前架构

## 入口与模块

- `mihomo`
  - 负责 CLI 入口、交互菜单、部署编排、systemd/iptables 相关流程
- `lib/common.sh`
  - 负责路径约定、环境读写、基础系统操作、状态文件初始化与通用辅助函数
- `lib/render.sh`
  - 负责 provider/rules 渲染、`config.yaml` 生成、权限与安装侧配套逻辑
- `scripts/statectl.py`
  - 负责节点、规则、订阅状态文件维护，以及 provider/rules 渲染等数据处理
- `scripts/rulepreset.py`
  - 负责规则仓库 manifest 解析、规则校验与内置规则模板渲染
- `tests/smoke.sh`
  - 覆盖配置渲染、状态迁移、规则与字段对齐等无 systemd 依赖的回归
- `tests/service_mock.sh`
  - 用 mock 的 `systemctl` / `journalctl` / `ss` / `iptables` / `curl` 验证服务侧命令和审计输出

## 真相边界

- 静态部署意图：
  - `settings.env`
  - `router.env`
- 本项目状态真相：
  - `state/nodes.json`
  - `state/rules.json`
  - `state/acl.json`
  - `state/subscriptions.json`
- 运行配置产物：
  - `config.yaml`
  - `ruleset/*.rules`
  - `proxy_providers/manual.txt`
  - `proxy_providers/subscriptions/*.txt`

## 当前数据流

1. CLI 命令更新 env 或状态文件
2. `render-config` 读取 env 与状态文件
3. `statectl.py` 生成 provider / rules 渲染产物
4. `lib/render.sh` 组装 `config.yaml`
5. systemd 启停 `mihomo-core`
6. `status` 与 `runtime-audit` 的“当前模式”已优先读取 Mihomo REST API；两者还会读取 `/proxies` 输出最小策略组摘要，并读取 `/version` 输出最小控制面运行态摘要，其余状态仍主要读取本地配置、systemd、端口和日志

## 已收口边界

- 订阅缓存 provider 是运行配置输入，订阅节点不再作为主节点交互真相
- 显式代理认证与免认证网段已经对齐到官方字段
- WebUI 外部名称、下载地址与控制面 CORS 已接入官方字段
- 控制面 TLS 证书管理暂不纳入当前架构边界

## 下一阶段边界

- 阶段 5 进入“代码结构收口”
- 推进粒度已切换为职责块收口，不再默认继续单行 helper 抽取
- 当前已完成四类块级收口：
  - 运行态与审计展示块：`status`、`runtime-audit`、`healthcheck`、`diagnose`、`audit_installation`
  - 安装与同步块：`install_webui`、`install_project`、`install_project_sync`、`disable_project_sync`、`finalize_project_install`
  - manager sync unit 渲染块：通用 render/write、sections、timer static settings、service body
  - `render_config` 渲染块：访问/控制面基础段、DNS/基础配置段、显式代理认证段、provider/group 组装段、rules 尾段
- 当前已完成第五类块级收口：
  - 运行前准备与服务启停块：runtime support files、runtime geo assets、runtime core guard、prepared start/restart/enable-start
- 当前已完成第六类块级收口：
  - 部署与修复编排块：deploy context、repair WebUI flow、setup maintenance、setup service finalize
- 当前已完成第七类块级收口：
  - 订阅刷新编排块：single subscription refresh、refresh success/failure recording、update-subscriptions orchestration
- 当前已完成第八类块级收口：
  - 交互导入编排块：input source selection、URI collection、manual node append、scan result processing
- 当前已完成第九类块级收口：
  - 交互网络向导编排块：current config intro、core input collection、core env writes、detected lan cidrs、bypass env flow
- `render_config` 当前已退回为编排入口，负责状态准备、调用职责块和配置文件权限收尾
- `prepare_runtime_assets` 当前已退回为“根检查 + 节点检查 + 调用职责块 + config test”的编排入口
- `full_setup` 当前已退回为“上下文准备 + 核心保障 + 运行时资产 + WebUI + 定时维护 + 服务状态收尾”的编排入口
- `repair_command` 当前已退回为“上下文准备 + 运行时资产 + WebUI 修复 + 配置测试”的编排入口
- `update_subscriptions_command` 当前已退回为“遍历启用订阅 + 结果统计 + 状态收尾”的编排入口
- `import_links` 当前已退回为“准备输入源 + 收集 URI + scan + 处理结果 + 状态收尾”的编排入口
- `router_wizard` 当前已退回为“展示当前配置 + 收集输入 + snapshot + 写基础 env + 处理派生网段 + 写 bypass + 状态收尾”的编排入口
- 下一闭环优先转向 `mihomo` 的 CLI 分发编排
  - 优先 `main`
- 不在该阶段顺手扩更多控制面能力，也不继续围绕 manager sync unit 做单行 helper 级拆分
