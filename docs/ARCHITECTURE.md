# 当前架构

## 入口与模块

- `cmd/minimalist`
  - Go 主二进制入口
- `internal/cli`
  - 命令行参数分发
- `internal/app`
  - 业务命令编排、菜单入口、状态变更与系统命令组合
- `internal/config`
  - 用户配置真相：`/etc/minimalist/config.yaml`
- `internal/state`
  - 程序状态真相：`/var/lib/minimalist/state.json`
- `internal/provider`
  - URI 扫描、节点命名、provider YAML 渲染
- `internal/rulesrepo`
  - 默认规则仓库初始化、搜索、渲染、增删
- `internal/runtime`
  - 运行时 `config.yaml`、rules、provider、systemd、sysctl 文本生成
- `internal/system`
  - 外部命令执行封装

## 真相边界

- 用户配置真相：
  - `/etc/minimalist/config.yaml`
- 程序状态真相：
  - `/var/lib/minimalist/state.json`
- 运行时产物：
  - `/var/lib/minimalist/mihomo/config.yaml`
  - `/var/lib/minimalist/mihomo/proxy_providers/manual.txt`
  - `/var/lib/minimalist/mihomo/proxy_providers/subscriptions/*.txt`
  - `/var/lib/minimalist/mihomo/ruleset/*.rules`

## 当前数据流

1. `minimalist` CLI 或 `menu` 读取配置与状态
2. 资源型命令更新 `state.json` 或 `config.yaml`
3. `render-config` 生成 provider / rules / runtime config
4. `setup` 写 sysctl 与 `minimalist.service`
5. `start` / `restart` 通过 `systemctl` 启停 `mihomo-core`
6. `status` / `runtime-audit` / `healthcheck` 优先读取 Mihomo REST API，不可达时回退本地配置和 systemd 状态

## 当前边界

- 当前仍只承诺 Debian NAS / IPv4 旁路由 / `iptables + TProxy`
- 订阅 provider 缓存仍是运行配置输入真相
- ACL / 自定义规则仍只允许指向手动节点与内置目标
- 控制面 TLS 管理仍不进入当前范围
- 旧 shell / Python 主实现已从主树清理，不再是当前主入口
