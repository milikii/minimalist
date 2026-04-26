# minimalist 关键流程

本页只描述 Go 版 `minimalist` 当前真相。

## 主路径

1. `minimalist install-self`
2. `minimalist setup`
3. `minimalist import-links` 或 `minimalist subscriptions update`
4. `minimalist router-wizard`
5. `minimalist render-config`
6. `minimalist start`
7. `minimalist healthcheck`
8. `minimalist status`

## 配置与状态流

1. `minimalist` 读取 `/etc/minimalist/config.yaml`
2. 资源型命令更新 `/var/lib/minimalist/state.json`
3. `render-config` 生成：
   - `/var/lib/minimalist/mihomo/config.yaml`
   - `/var/lib/minimalist/mihomo/proxy_providers/manual.txt`
   - `/var/lib/minimalist/mihomo/proxy_providers/subscriptions/*.txt`
   - `/var/lib/minimalist/mihomo/ruleset/*.rules`
4. `minimalist.service` 通过 `mihomo-core` 读取运行目录启动

补充当前真相：

- 即使当前没有手动节点或订阅 provider，`render-config` 仍会生成仅含 `DIRECT` 的 `PROXY` 组
- 开启显式代理认证或控制面 CORS 时，对应段落直接写入运行时 `config.yaml`
- DNS 相关默认静态段落当前固定由 `render-config` 生成，包括 `default-nameserver`、`direct-nameserver`、`fake-ip-filter` 与 `nameserver-policy`
- `profile`、`fallback-filter`、`proxy-server-nameserver` 当前也由 `render-config` 直接生成固定默认段落
- `nameserver`、`geox-url`、`dns.listen` 当前同样由 `render-config` 直接生成固定默认段落
- `allow-lan`、`bind-address`、`log-level`、`ipv6`、geo 与 DNS 行为标志当前也由 `render-config` 直接生成固定默认段落
- `secret`、`external-controller`、`lan-allowed-ips`、`lan-disallowed-ips` 当前同样由 `render-config` 直接生成确定性段落
- 顶层 `minimalist rules|acl|subscriptions|rules-repo ...` 当前都已直接分发到同一组底层 CLI helper

## 节点与订阅

- `import-links` 导入的是手动节点真相，默认 `disabled`
- `subscriptions update` 拉取的是 provider 缓存真相，订阅节点只保留只读枚举
- ACL / 自定义规则只允许指向手动节点与内置目标

## 辅助入口

- `minimalist menu` 是当前交互入口，内部仍分发到同一组 CLI 命令
- 顶层 `minimalist --help` / `help` / 非 TTY 空参数当前都回落到同一份 usage 输出
- 顶层 `minimalist setup` / `start` / `clear-rules` 等 root-only 命令当前仍直接分发到同一组 `internal/app` 实现
- `minimalist router-wizard` 直接回写 `/etc/minimalist/config.yaml`
- `minimalist rules-repo summary|entries|find` 用于查看当前内置规则仓库真相

## 运行态观测

- `status` / `runtime-audit` / `healthcheck` 优先读取 Mihomo REST API
- 控制面不可达时回退配置文件、systemd 和本机端口信息
