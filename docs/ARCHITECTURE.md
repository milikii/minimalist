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
6. `status` / `runtime-audit` 读取本地配置、systemd、端口和日志，当前还不是 REST 运行态优先

## 已收口边界

- 订阅缓存 provider 是运行配置输入，订阅节点不再作为主节点交互真相
- 显式代理认证与免认证网段已经对齐到官方字段
- WebUI 外部名称、下载地址与控制面 CORS 已接入官方字段
- 控制面 TLS 证书管理暂不纳入当前架构边界

## 下一阶段边界

- 阶段 4 只新增“运行态读取优先”能力
- 第一刀仅覆盖 `mihomo status` 的当前模式读取与控制面回退
- 不在该阶段顺手重构脚本结构或扩展更多控制面能力
