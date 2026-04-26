# minimalist 当前路线

原 `mihomo` shell/Python 收口路线已结束，当前主线固定为 Go 版 `minimalist`。

## 当前固定边界

- Debian NAS
- IPv4 旁路由
- `iptables + TProxy`
- 核心主路径 + 规则/订阅
- 不做旧状态迁移

## 当前主实现

- `cmd/minimalist`
- `internal/cli`
- `internal/app`
- `internal/config`
- `internal/state`
- `internal/provider`
- `internal/rulesrepo`
- `internal/runtime`
- `internal/system`

## 当前下一步

- 继续补 `internal/app` / `internal/system` 测试
- 继续提升 `render-config` / `subscriptions update` 的 golden / integration coverage
- 继续把文档和发布路径稳定到 `minimalist`
