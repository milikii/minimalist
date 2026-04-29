# 进度日志

## Round 0: 项目初始化

### 完成
- 初始化执行进度日志。

### 测试状态
- 通过: 未运行 / 总计: 未运行

### 遗留 / 下轮继续
- 后续按每轮实际执行追加记录。

### 下轮目标
- 根据 docs/TASKS.md 或线上故障继续推进。

## Round 1 — 2026-04-30 02:34

### 完成
- 排查 7890 HTTP / SOCKS5 代理连不上问题。
- 确认 `mihomo-core` 已监听 `*:7890`，controller 正常，LAN 请求已进入 mihomo。
- 定位根因：有可用节点时 `PROXY` 组默认顺序为 `DIRECT, AUTO`，服务重启后 `MATCH,PROXY` 默认走直连，导致被墙目标超时。
- 将有可用 provider 时的 `PROXY` 组顺序改为 `AUTO, DIRECT`，保留无 provider 时 `DIRECT` 兜底。
- 重新安装 `/usr/local/bin/minimalist`，渲染 `/var/lib/minimalist/mihomo/config.yaml`，并重启 `minimalist.service`。

### 测试状态
- 通过: focused runtime tests、`go test ./...`、`go vet ./...`、`gofmt -l cmd internal`、实机 HTTP/SOCKS5 7890 smoke / 总计: 5 组

### 遗留 / 下轮继续
- 继续观察 `runtime-audit` 的 `fatal-gaps=0` 和 7890 客户端实际使用情况。

### 下轮目标
- 若 7890 再次异常，优先检查 `PROXY.now`、节点 delay 和最近 5 分钟 journal。
