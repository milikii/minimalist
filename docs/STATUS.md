# 当前状态

## 当前主线

- 项目已经开始从 `mihomo` shell/Python 版本切换到 Go 版 **`minimalist`**。
- 当前仓库里已经落下新的 Go 模块、主 CLI、配置/状态真相、rules-repo 资产、provider 渲染和基础测试。
- 旧 shell / Python 代码当前仍保留在仓库中作参考，但旧 `mihomo` 命令入口已删除，不再作为当前主实现。

## 当前真相

- 当前主命令名：`minimalist`
- 当前定位：Debian NAS / IPv4 旁路由 / `iptables + TProxy`
- 主配置文件：`/etc/minimalist/config.yaml`
- 主状态文件：`/var/lib/minimalist/state.json`
- 运行产物目录：`/var/lib/minimalist/mihomo`
- 当前保留能力：
  - `install-self`
  - `setup`
  - `render-config`
  - `start` / `stop` / `restart`
  - `status`
  - `show-secret`
  - `healthcheck`
  - `runtime-audit`
  - `import-links`
  - `router-wizard`
  - `menu`
  - `nodes`
  - `subscriptions`
  - `rules`
  - `acl`
  - `rules-repo`

## 当前实现结论

- Go 版已实现单二进制 CLI 入口：`cmd/minimalist`
- Go 版已实现用户配置与程序状态真相：
  - `internal/config`
  - `internal/state`
- Go 版已实现订阅解码、URI 扫描和 provider 渲染：
  - `internal/provider`
- Go 版已实现默认规则仓库初始化与规则操作：
  - `internal/rulesrepo`
- Go 版已实现运行时产物渲染、systemd unit 与 sysctl 文本生成：
  - `internal/runtime`
- Go 版已实现命令编排与菜单入口：
  - `internal/app`
  - `internal/cli`

## 质量状态

- `go build -o /tmp/gobin/minimalist ./cmd/minimalist`：通过
- `GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./...`：通过

## 当前风险与限制

- 旧 shell / Python 参考代码仍在仓库中，尚未做第二轮物理清理
- 当前 Go 测试主要覆盖配置 round-trip、rules-repo 渲染和 provider 基础渲染，系统命令 mock 还不完整
- 旧版本 `settings.env` / `router.env` / `state/*.json` 不兼容，不做迁移
