# minimalist

面向 Debian NAS 的 Go 版旁路由管理器，使用 `mihomo-core` 作为底层内核。

## 当前定位

- 仅承诺 Debian NAS / IPv4 旁路由 / `iptables + TProxy`
- 默认宿主机直连，`proxy_host_output: false`
- 默认控制面仅绑定 `127.0.0.1:19090`
- 主命令名已经从 `mihomo` 切到 `minimalist`
- 旧 shell / Python 代码当前仅作仓库内参考，不再作为主入口

## 当前保留能力

- 核心主路径：`install-self`、`setup`、`render-config`、`start` / `stop` / `restart`
- 运维查看：`status`、`show-secret`、`healthcheck`、`runtime-audit`
- 交互入口：`menu`、`router-wizard`、`import-links`
- 规则与订阅：`nodes`、`subscriptions`、`rules`、`acl`、`rules-repo`

## 开发运行

```bash
go run ./cmd/minimalist --help
go run ./cmd/minimalist menu
go run ./cmd/minimalist setup
```

构建本地二进制：

```bash
go build -o ./minimalist ./cmd/minimalist
```

安装到系统：

```bash
sudo go run ./cmd/minimalist install-self
sudo /usr/local/bin/minimalist setup
```

## 运行时真相

- 用户配置：`/etc/minimalist/config.yaml`
- 程序状态：`/var/lib/minimalist/state.json`
- 运行产物：`/var/lib/minimalist/mihomo/`
  - `config.yaml`
  - `proxy_providers/manual.txt`
  - `proxy_providers/subscriptions/*.txt`
  - `ruleset/*.rules`

## 推荐使用顺序

1. `sudo minimalist install-self`
2. `sudo minimalist setup`
3. `minimalist import-links`
4. `minimalist subscriptions update`
5. `minimalist router-wizard`
6. `minimalist healthcheck`
7. `minimalist status`

## 当前限制

- 旧版本 `settings.env` / `router.env` / `state/*.json` 不兼容，不做迁移
- 不保留 `alpha/stable` 核心通道切换、自动同步、自定义更新定时器等旧运维能力
- `nas-single-lan-dualstack` 已不再进入当前产品边界

## 测试

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./...
```
