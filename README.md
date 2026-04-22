# mihomo-nas

用于 Debian NAS 的宿主机旁路由 Mihomo 管理脚本。

## 当前运行模型
- 宿主机默认直连，不默认透明接管本机外连，默认 `PROXY_HOST_OUTPUT=0`。
- 宿主机应用如需代理，显式使用 `http://127.0.0.1:7890`。
- 局域网设备把网关和 DNS 指向 NAS 后，可复用 NAS 上的旁路由能力。
- Docker 默认直连；只有显式设置代理，或后续把对应 bridge 接进透明代理，容器才会走 Mihomo。
- `tailscaled` / `cloudflared` 运行时，脚本拒绝启用宿主机透明接管，避免误伤 SSH / 隧道链路。
- 控制面默认只绑定 `127.0.0.1:${CONTROLLER_PORT:-19090}`；状态页不再默认打印密钥，需显式执行 `mihomo show-secret`。

## 推荐入口
- `mihomo menu`：默认操作入口；主菜单只保留宿主机旁路由主路径。
- `mihomo status`：查看当前运行定位、入口接口、宿主机流量模式和容器直连名单。
- `mihomo show-secret`：显式查看控制面密钥；默认状态页不会直接暴露。
- `mihomo healthcheck`：检查端口监听、WebUI 与 `127.0.0.1:7890` 显式代理是否可用。
- `mihomo runtime-audit`：看 systemd 状态、近 24 小时 warning/error 和健康摘要。

## 菜单分层
- 主菜单：部署修复、节点管理、旁路由边界、服务、健康、审计、日志与诊断。
- 高级维护（少用）：规则、模式切换、端口设置、内核更新/回滚、WebUI、定时器和规则仓库同步。
- 规则和仓库同步不再挤占主菜单；它们仍可通过命令行直接调用。

## 推荐使用顺序
1. `mihomo setup`
2. `mihomo import-links`
3. `mihomo router-wizard`
4. `mihomo healthcheck`
5. 局域网设备把网关和 DNS 指向 NAS

## 关键端口
- `7890`: mixed 显式代理
- `7893`: TProxy
- `1053`: DNS
- `19090`: Controller / WebUI（默认仅宿主机）
