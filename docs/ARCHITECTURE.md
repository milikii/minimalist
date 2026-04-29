# ARCHITECTURE

> 基于 2026-04-28 仓库代码与测试反推生成。本文描述“当前代码已经实现的架构”，不是未来规划。

## 1. 总览

`minimalist` 是一个面向 Debian NAS 场景的本地 CLI 控制面。

它本身不是代理内核，而是围绕 `mihomo-core` 提供以下能力：

- 初始化安装目录、配置文件、状态文件和默认规则仓库
- 把用户配置与节点状态渲染成 Mihomo 运行时文件
- 生成并安装 `systemd` 服务单元与 `sysctl` 配置
- 通过 `iptables + TProxy + ip rule/ip route` 下发旁路由规则
- 管理手动节点、订阅缓存、自定义规则、ACL 和内置规则仓库
- 通过本地 controller API 做状态查看、延迟测试、健康检查和运行审计
- 从 GitHub alpha release 单次升级 `mihomo-core`

运行形态上，`minimalist` 是控制平面，真正被 `systemd` 拉起的长驻进程是 `mihomo-core`。

## 2. 技术栈

- 语言：Go 1.24
- 第三方依赖：`gopkg.in/yaml.v2`
- 进程管理：`systemd`
- 网络编排：`iptables`、`ip rule`、`ip route`
- 外部交互：
  - GitHub Releases API：`core-upgrade-alpha`
  - 订阅 URL：`subscriptions update`
  - Mihomo controller HTTP API：`status`、`healthcheck`、`runtime-audit`、节点测速
  - Docker CLI：解析容器旁路 IP 白名单

## 3. 仓库结构

```text
cmd/minimalist/main.go              CLI 入口

internal/app/                       应用编排层
  app.go                            业务主流程、菜单、systemd/iptables/controller 交互
  core_upgrade.go                   alpha 内核升级

internal/cli/                       命令路由层
  cli.go                            子命令分发与 usage

internal/config/                    用户配置
  config.go                         config.yaml 读写、默认值、secret 兜底

internal/state/                     程序状态
  state.go                          state.json 读写、节点/规则/订阅状态模型

internal/provider/                  节点解析与 provider 渲染
  provider.go                       URI 解析、订阅解码、provider 文件生成

internal/rulesrepo/                 规则仓库
  repo.go                           manifest/规则集读取、校验、查询、增删
  assets/default/...                内置默认规则仓库模板

internal/runtime/                   运行时文件渲染
  runtime.go                        runtime config、service unit、sysctl、目录布局

internal/system/                    命令执行抽象
  system.go                         带超时的 shell command runner

rules-repo/default/...              仓库内镜像规则仓库样本
docs/...                            文档
```

每个核心包都带对应 `*_test.go`，当前测试覆盖面集中在：

- 命令路由
- 文件读写与默认值
- provider 协议解析
- runtime 配置渲染
- 规则仓库操作
- app 层业务流程与错误分支

## 4. 运行时文件与真相边界

### 用户可编辑配置

- `config.yaml`
- 默认路径：`/etc/minimalist/config.yaml`
- 由 `internal/config` 管理

配置模型主要包括：

- `profile`：模板名、模式、规则预设
- `network`：LAN 接口、透明代理入口、DNS 劫持、宿主机流量接管、旁路白名单
- `ports`：mixed/tproxy/dns/controller 端口
- `controller`：绑定地址、secret、CORS
- `access`：显式代理认证、LAN 禁止访问网段
- `install`：`mihomo-core` 二进制路径

### 程序状态

- `state.json`
- 默认路径：`/var/lib/minimalist/state.json`
- 由 `internal/state` 管理

状态模型主要包括：

- `nodes`：手动导入节点与订阅派生节点
- `rules`：自定义规则
- `acl`：ACL 规则
- `subscriptions`：订阅地址、缓存状态、枚举结果

### 运行产物

默认根目录：`/var/lib/minimalist/mihomo/`

- `config.yaml`：最终喂给 `mihomo-core` 的运行配置
- `proxy_providers/manual.txt`：启用的手动节点
- `proxy_providers/subscriptions/<id>.txt`：订阅缓存原文
- `ruleset/builtin.rules`：内置规则仓库渲染结果
- `ruleset/custom.rules`：用户自定义规则
- `ruleset/acl.rules`：ACL 规则
- `Country.mmdb` / `GeoSite.dat` / `ui/`：启动前必须存在的运行资产

### 系统级产物

- `minimalist` 可执行文件：默认 `/usr/local/bin/minimalist`
- `systemd` unit：默认 `/etc/systemd/system/minimalist.service`
- `sysctl`：默认 `/etc/sysctl.d/99-minimalist-router.conf`
- 默认规则仓库：`/etc/minimalist/rules-repo/default/manifest.yaml`

## 5. 模块分工

### `cmd/minimalist`

- 仅做进程入口
- 调 `cli.Run(os.Args[1:])`

### `internal/cli`

- 负责 CLI 语义
- 决定无参时进入菜单还是打印帮助
- 把子命令映射到 `app.App` 方法

当前命令面大致分为：

- 安装部署：`install-self`、`setup`、`render-config`
- 服务生命周期：`start`、`stop`、`restart`
- 运行审计：`status`、`show-secret`、`healthcheck`、`runtime-audit`、`verify-runtime-assets`
- 切换检查：`cutover-preflight`、`cutover-plan`
- 节点/订阅：`import-links`、`router-wizard`、`nodes ...`、`subscriptions ...`
- 规则：`rules ...`、`acl ...`、`rules-repo ...`
- 网络编排：`apply-rules`、`clear-rules`
- 内核升级：`core-upgrade-alpha`

### `internal/app`

这是整个系统的应用编排层，承担几乎全部业务决策与副作用。

主要职责：

- 调用 `config/state/runtime/rulesrepo/provider/system`
- 组织安装、部署、启停、升级、审计、交互菜单流程
- 访问本机 `systemctl`、`iptables`、`ip`、`journalctl`、`docker`
- 访问本地 Mihomo controller 与外部 HTTP 订阅源
- 处理 cutover 风险阻断逻辑

### `internal/config`

- 管理 `config.Config`
- 提供 `Default / Load / Save / Ensure`
- 若 `controller.secret` 缺失，会自动补齐并在必要时回写

### `internal/state`

- 管理 `state.State`
- 保存节点、规则、ACL、订阅及其缓存元信息
- 不做复杂迁移，缺字段时用默认值补齐

### `internal/provider`

- 解析 `vless://`、`trojan://`、`ss://`、`vmess://`
- 支持订阅文本与 Base64 订阅内容解码
- 把节点渲染成 Mihomo `proxy-providers` 格式
- 负责导入去重、命名冲突自动改名

### `internal/rulesrepo`

- 规则仓库 manifest 与条目文件的读写层
- 支持：
  - summary
  - entries
  - find
  - add
  - remove
  - remove-index
- 支持的规则类型：`domain`、`domain_suffix`、`domain_keyword`、`ip_cidr`

### `internal/runtime`

- 管理路径常量与目录布局
- 渲染：
  - runtime `config.yaml`
  - `systemd` unit
  - `sysctl`
  - provider/rules 文件
- 负责判定运行资产是否缺失

### `internal/system`

- 命令执行适配层
- 提供统一超时、stdout/stderr 收集与错误包装
- 便于 app 层测试替身注入

## 6. 核心数据流

### 6.1 首次安装

`install-self`

1. 读取当前可执行文件自身路径
2. 复制到目标 `BinPath`
3. 确保目录布局存在
4. 生成默认 `config.yaml`
5. 生成默认 `state.json`
6. 初始化默认规则仓库

### 6.2 节点导入

`import-links`

1. 从 stdin 读取多行文本
2. 识别是否为直链列表或 Base64 订阅内容
3. 解析支持的 URI
4. 用 `URIBaseKey` 去重
5. 以 `manual` source 写入 `state.json`
6. 默认导入为禁用状态

### 6.3 订阅更新

`subscriptions update`

1. 读取已启用订阅
2. 逐个发起 HTTP GET
3. 将订阅原文缓存到 `proxy_providers/subscriptions/<id>.txt`
4. 扫描支持的 URI 行
5. 清理旧的订阅派生节点
6. 追加新的 `source=subscription` 节点
7. 更新 `LastAttemptAt / LastSuccessAt / LastError / LastCount`

### 6.4 运行配置渲染

`render-config`

1. `ensureAll()` 确保目录、配置、状态、规则仓库存在
2. 校验规则目标是否合法
3. 渲染：
   - 手动 provider
   - 用户规则
   - ACL 规则
   - 内置规则仓库
   - 最终 Mihomo `config.yaml`

运行配置生成逻辑要点：

- 有可用 provider 时，`PROXY` 组由 `AUTO + DIRECT + provider use` 组成；`AUTO` 必须排在第一位，避免服务重启后 `MATCH,PROXY` 默认回落到直连
- 没有 provider 时，`PROXY` 组退化为仅 `DIRECT`
- 订阅 provider 只有在缓存文件存在且包含受支持 URI 时才进入运行配置
- 最终规则顺序为：
  - custom.rules
  - acl.rules
  - builtin.rules
  - `PROCESS-NAME,mihomo,DIRECT`
  - `GEOIP,CN,DIRECT`
  - `MATCH,PROXY`

### 6.5 部署与启动

`setup` / `start` / `restart`

1. root 权限检查
2. cutover 风险检查
3. 渲染 runtime 文件
4. 检查运行资产是否齐全
5. 写入 `systemd` unit 与 `sysctl`
6. `sysctl -p`
7. `systemctl daemon-reload`
8. 视 provider 就绪情况决定是否启用服务

`minimalist.service` 的关键行为：

- `ExecStartPre=+minimalist verify-runtime-assets`
- `ExecStartPre=+minimalist apply-rules`
- `ExecStart=<mihomo-core> -d <runtime-dir>`
- `ExecReload=+minimalist apply-rules`
- `ExecStopPost=+minimalist clear-rules`

### 6.6 透明代理规则下发

`apply-rules`

1. root 与 cutover 检查
2. 读取配置与状态
3. 若当前是“仅显式代理”模式，则先清理规则再退出
4. 否则创建/刷新 `MIHOMO_*` iptables chains
5. 写入入口接口、保留网段、旁路名单、Docker 容器白名单、DNS 劫持规则
6. 写入 `TPROXY` 与宿主机 `OUTPUT` 标记规则
7. 写入 `ip route table 233` 与 `ip rule priority 100`

当前透明代理实现边界：

- 仅 IPv4
- 仅 `iptables`
- 仅 `TPROXY`
- 旁路标记使用 `0x2333 / 9011 / table 233`

### 6.7 运行观测

`status` / `healthcheck` / `runtime-audit`

- `status`：输出配置摘要、当前模式、服务状态、手动节点数、订阅就绪数
- `healthcheck`：输出关键端口与 controller `/version`
- `runtime-audit`：
  - 复用 `status`
  - 统计最近 24h / 15min journal warn/error
  - 输出 provider 是否就绪
  - 输出 cutover 状态
  - 报告 fatal gaps

### 6.8 Core 升级

`core-upgrade-alpha`

1. root 检查
2. 读取配置拿到 `core_bin`
3. 从 GitHub Releases 拉取 release 列表
4. 选择最新 Linux alpha 资产
5. 下载并解压 `.gz`
6. 备份旧二进制并原子替换
7. 重启 `minimalist.service`
8. 输出新旧版本与资产信息

## 7. Cutover 保护机制

代码中显式保留了从旧 `mihomo.service` 切到 Go 版 `minimalist` 的防护层。

判断输入包括：

- `mihomo.service` 是否 active/enabled
- 旧 `/usr/local/bin/mihomo` 是否存在
- 旧 `/etc/mihomo` 是否存在
- `minimalist.service` 是否 active/enabled

高风险命令在 cutover 未就绪时会直接阻断：

- `setup`
- `start`
- `restart`
- `apply-rules`
- `clear-rules`

`cutover-preflight` 与 `cutover-plan` 都是只读命令，不自动执行切换。

## 8. 内置规则仓库

内置默认规则仓库通过 `embed` 打包在二进制内，并在运行时复制到配置目录。

当前内置规则集只有三组：

- `pt`：PT 域名直连
- `fcm-site`：FCM 相关域名走代理
- `fcm-ip`：FCM 相关 IP 走代理

仓库内还保留了一份 `rules-repo/default/` 镜像样本，内容与 `internal/rulesrepo/assets/default/` 对应。

## 9. 设计约束与已知边界

- 目标环境被硬编码为 Debian + systemd + iptables + IPv4 旁路由
- 不做 legacy shell/Python 状态迁移
- 不做运行资产自动下载，必须预置 `Country.mmdb`、`GeoSite.dat`、`ui/`
- 透明代理规则依赖 root、`CAP_NET_ADMIN` 和可用的 `iptables/ip`
- 节点协议仅覆盖 `vless/trojan/ss/vmess`
- 订阅节点是 provider-managed，不允许直接重命名/启停/删除单条订阅派生节点

## 10. 当前架构评价

这个仓库的架构非常明确：小而集中的本地控制面。

优点：

- 依赖少
- 模块边界清晰
- 文件真相边界稳定
- 副作用集中在 `internal/app`
- 测试覆盖广

当前主要风险点：

- 控制面能力和实际 service 启动前置条件仍有少量不一致
- 升级路径的校验与回滚能力不足
- CLI、菜单、README 之间存在可见性不完全一致的问题
- 内置规则仓库与仓库镜像双份维护，存在未来漂移风险
