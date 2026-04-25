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
- 当前已抽离 `status` / `runtime-audit` 共用的运行态摘要 helper
- 当前已抽离 `status` / `runtime-audit` 共用的控制面静态信息展示 helper
- 当前已抽离 `status` / `runtime-audit` 共用的网络与访问静态信息展示 helper
- 当前已抽离 `status` / `runtime-audit` 共用的模板/规则预设/IPv6 展示 helper
- 当前已抽离 `status` / `runtime-audit` 共用的计数类与节点统计展示 helper
- 当前已抽离 `status` 的推荐下一步判断 helper
- 当前已抽离 `status` 的警告与收尾展示 helper
- 当前已抽离 `status` 的同步与端口展示 helper
- 当前已抽离 `status` 的 WebUI / 控制面密钥入口展示 helper
- 当前已抽离 `status` 推荐下一步所需的计数预处理 helper
- 当前已抽离 `status` 的基础概览展示 helper
- 当前已抽离 `status` 的基础状态采集 helper
- 当前已抽离 `runtime-audit` 的探测与流量摘要展示 helper
- 当前已抽离 `runtime-audit` 的告警与定时器展示 helper
- 当前已抽离 `runtime-audit` 的基础概览展示 helper
- 当前已抽离 `runtime-audit` 的基础状态采集 helper
- 当前已抽离 `runtime-audit` 的探测状态采集 helper
- 当前已抽离 `runtime-audit` 的健康摘要收尾 helper
- 当前已抽离 `runtime-audit` 的告警与定时器状态采集 helper
- 当前已抽离 `healthcheck` 的端口监听检查 helper
- 当前已抽离 `healthcheck` 的探测检查 helper
- 当前已抽离 `diagnose` 的配置摘要展示 helper
- 当前已抽离 `diagnose` 的 systemd / listeners / logs 分段展示 helper
- 当前已抽离 `healthcheck` 的基础状态检查 helper
- 当前已抽离 `audit_installation` 的基础文件存在性检查 helper
- 当前已抽离 `audit_installation` 的 nodes/rules 渲染漂移检查 helper
- 当前已抽离 `audit_installation` 的 ACL / 规则预设检查 helper
- 当前已抽离 `audit_installation` 的 timer / GeoSite 检查 helper
- 当前已抽离 `audit_installation` 的成功收尾 helper
- 当前已抽离 `install_geosite_dat` 的成功安装收尾 helper
- 当前已抽离 `install_webui` 的下载阶段 helper
- 当前已抽离 `install_webui` 的解压与源码目录识别 helper
- 当前已抽离 `install_webui` 的部署与持久化收尾 helper
- 当前已抽离 `install_webui` 的失败收尾 helper
- 当前已抽离 `install_webui` 的临时工作区清理 helper
- 当前已抽离 `install_webui` 的参数与目标解析 helper
- 当前已抽离 `install_webui` 的临时工作区准备 helper
- 当前已抽离 `install_project_sync` 的入参校验 helper
- 当前已抽离 `install_project_sync` 的设置持久化 helper
- 当前已抽离 `disable_project_sync` 的设置重置 helper
- 当前已抽离 `disable_project_sync` 的运行时清理 helper
- 当前已抽离 `install_project_sync` 的 systemd 激活收尾 helper
- 下一刀先抽离 `install_project_sync` 的成功提示收尾 helper
- 不在该阶段顺手重构脚本结构或扩展更多控制面能力
