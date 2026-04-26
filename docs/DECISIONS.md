# 当前决策

## 2026-04-26 项目正式切到 `minimalist`

- 新项目名、命令名、systemd unit、配置目录统一使用 `minimalist`
- `mihomo-core` 继续只是底层内核名，不再承担管理器命名
- 旧 `mihomo` 命令入口已删除，不保留 shim

## 2026-04-26 Go V2 作为当前主实现

- 当前主实现已经改为 Go 模块
- shell / Python 旧代码不再作为当前主路线
- 旧实现暂时只保留在仓库中作参考，不再是默认文档真相

## 2026-04-26 能力面收缩到“核心 + 规则/订阅”

- 保留：
  - setup / render-config / start / stop / restart
  - status / show-secret / healthcheck / runtime-audit
  - import-links / router-wizard / menu
  - nodes / subscriptions / rules / acl / rules-repo
- 删除或暂不实现：
  - alpha/stable 核心通道切换
  - core 回滚
  - 自动同步安装目录
  - 自定义更新/重启定时器
  - 双栈模板

## 2026-04-26 配置与状态真相重做

- 用户配置真相：`/etc/minimalist/config.yaml`
- 程序状态真相：`/var/lib/minimalist/state.json`
- 旧 `settings.env` / `router.env` / `state/*.json` 不再兼容，也不迁移

## 2026-04-26 仍保持 Debian NAS / IPv4 旁路由边界

- 继续只承诺 Debian NAS / IPv4 旁路由 / `iptables + TProxy`
- 不补 OpenWrt / firewall4 / nftables 抽象
- 不恢复 `nas-single-lan-dualstack`
