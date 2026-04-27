# Go 版 cutover runbook

本文件描述从旧 `mihomo.service` 切到 Go 版 `minimalist.service` 的人工步骤。它不是自动迁移脚本。

## 当前前提

- 旧现网服务：`mihomo.service`
- 旧运行目录：`/etc/mihomo`
- Go 版目标服务：`minimalist.service`
- Go 版配置真相：`/etc/minimalist/config.yaml`
- Go 版状态真相：`/var/lib/minimalist/state.json`
- Go 版运行目录：`/var/lib/minimalist/mihomo`

旧 `settings.env` / `router.env` / `state/*.json` 不自动迁移。旧 `/etc/mihomo` 里可能包含订阅、节点、secret 和运行缓存，不能提交到 git，也不要直接复制成 Go 版状态。

## 切换前只读检查

先确认当前处于 legacy live 状态：

```bash
sudo go run ./cmd/minimalist cutover-preflight
sudo go run ./cmd/minimalist cutover-plan
systemctl status mihomo --no-pager
systemctl status minimalist --no-pager
iptables -t mangle -S
iptables -t nat -S
ip rule show
ip route show table 233
```

若 `cutover-preflight` 输出 `cutover-ready=false`，表示旧 `mihomo.service` 仍处于 active/enabled，Go 版高风险命令会被 guard 阻断。`cutover-plan` 只打印下一步建议和回滚入口，不执行切换。

## 准备 Go 版输入

这些步骤不应该停旧服务，也不应该改现网规则：

```bash
sudo go run ./cmd/minimalist install-self
sudo /usr/local/bin/minimalist router-wizard
/usr/local/bin/minimalist import-links
/usr/local/bin/minimalist subscriptions update
/usr/local/bin/minimalist render-config
/usr/local/bin/minimalist healthcheck
```

如果节点或订阅尚未准备好，不进入切换窗口。

## 切换窗口步骤

切换会短暂中断透明代理，应在可回滚的维护窗口执行。

```bash
sudo systemctl disable --now mihomo.service
sudo /usr/local/bin/minimalist cutover-preflight
sudo /usr/local/bin/minimalist setup
sudo /usr/local/bin/minimalist restart
sudo /usr/local/bin/minimalist healthcheck
sudo /usr/local/bin/minimalist status
```

切换后确认：

```bash
systemctl status minimalist --no-pager
iptables -t mangle -S
iptables -t nat -S
ip rule show
ip route show table 233
```

期望结果：

- `minimalist.service` active/enabled
- `mihomo.service` inactive/disabled
- `MIHOMO_*` 规则由 Go 版 `minimalist` 管理
- `fwmark 0x2333 lookup 233` 与 table `233` 存在
- 控制面、DNS、TProxy 端口健康检查通过

## 回滚步骤

如果 Go 版验证失败，先停 Go 版服务，再恢复旧服务：

```bash
sudo systemctl disable --now minimalist.service
sudo systemctl enable --now mihomo.service
systemctl status mihomo --no-pager
iptables -t mangle -S
iptables -t nat -S
ip rule show
ip route show table 233
```

不要在回滚前删除 `/etc/mihomo`、`/usr/local/lib/mihomo-manager` 或 `/usr/local/bin/mihomo`。

## 不做的事

- 不自动迁移旧 env/state。
- 不自动删除旧服务和旧目录。
- 不自动清理或重命名旧 `MIHOMO_*` 规则。
- 不在没有维护窗口和回滚入口时执行 cutover。
