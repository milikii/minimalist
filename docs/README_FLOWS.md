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

## 节点与订阅

- `import-links` 导入的是手动节点真相，默认 `disabled`
- `subscriptions update` 拉取的是 provider 缓存真相，订阅节点只保留只读枚举
- ACL / 自定义规则只允许指向手动节点与内置目标

## 运行态观测

- `status` / `runtime-audit` / `healthcheck` 优先读取 Mihomo REST API
- 控制面不可达时回退配置文件、systemd 和本机端口信息
