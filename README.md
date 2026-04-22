# mihomo-nas

用于 Debian NAS 的宿主机旁路由 Mihomo 管理脚本。

## 当前运行模型
- 宿主机默认直连，不默认透明接管本机外连，默认 `PROXY_HOST_OUTPUT=0`。
- 宿主机应用如需代理，显式使用 `http://127.0.0.1:7890`。
- 局域网设备把网关和 DNS 指向 NAS 后，可复用 NAS 上的旁路由能力。
- Docker 默认直连；只有显式设置代理，或后续把对应 bridge 接进透明代理，容器才会走 Mihomo。
- `tailscaled` / `cloudflared` 运行时，脚本拒绝启用宿主机透明接管，避免误伤 SSH / 隧道链路。

## 关键端口
- `7890`: mixed 显式代理
- `7893`: TProxy
- `1053`: DNS
- `19090`: Controller / WebUI
