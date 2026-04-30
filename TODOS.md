# TODOS

> 未来想做、但不在当前主线（长期稳定运行观察）范围内的事项。
> 不进入 `docs/TASKS.md`，避免污染观察期稳定主线。
> 启动这里的事项前，先确认 `docs/NEXT_STEP.md` 已宣布观察期结束。

---

## 菜单 + CLI 交互体验重设计

**状态**：观察期结束（24-72h smoke 持续 `fatal-gaps=0`、`minimalist.service` 在重启 / 重启宿主机后均稳定回到 `active/enabled`）后，再用 `/office-hours` 或 `/design-consultation` 启动产品决策。**不在观察期内动代码**。

### 当前痛点（2026-04-30 自陈 + 代码反推）

#### 视觉层（"各种选项不显眼"）
- `Menu()` / `nodesMenu()` / `subscriptionsMenu()` 等都是纯 `fmt.Fprintln(a.Stdout, "1) xxx")` 一列裸打印；无标题、无 breadcrumb、无颜色、无对齐
- 子菜单外观几乎一样，进入后不知道当前在哪个菜单
- 无状态摘要（当前节点数、订阅数、服务 active/enabled、controller 是否可达）作为上下文
- 高风险动作（删除节点、删除订阅）和无害查看权重一样，没有视觉警示
- `default: 无效选择` 不告诉用户输入了什么、也不说明期望格式

#### 交互层（"莫名其妙的交互逻辑"）
- `nodesMenu` / `subscriptionsMenu` 的 `case "1"` 看完列表后立即 `return` 退回主菜单；再操作要重走一轮 `主菜单 → nodesMenu → 选项`
- "看节点"与"操作节点"完全分开：先选 1 看 ID、记住、退回、选启用/禁用/改名/删除、输入 ID
- 启用 / 禁用 / 改名 / 删除是 4 个独立菜单项，本质是对**同一节点对象**的不同动作；无法在列表里直接对节点 N 发命令
- 输错路径或参数无 undo，只能从主菜单重走

### 候选方向（待启动时展开、不在此预先决策）

1. **最小修补（保留树状菜单）** — 加标题 / breadcrumb / 状态摘要、合并"看 + 操作"为单菜单（看完不退出）、改 default 提示。增量、低风险，仍是数字菜单。
2. **TUI 重写** — `bubbletea` 之类的库，方向键导航、列表内对单条对象直接操作、状态实时刷新。中风险，体验跳跃式提升，引入 TUI 依赖。
3. **CLI all-in** — 删除 `menu`，所有动作做成更好的 `minimalist nodes enable 5` 式子命令；交互式只保留 `router-wizard` / `import-links`。低中风险，心智模型从"逛菜单"变成"敲命令"。
4. **REPL** — `minimalist>` 单一交互入口，输入 `nodes`、`nodes 5 enable`、`status`。中风险，单点入口但学习成本介于菜单和 CLI 之间。

### 参考实现（mihomo / sing-box 社区成熟 shell 项目）

社区已经把这些痛迭代过很多年，下面这些项目的菜单 UX 模式可以直接借鉴，不需要从零设计：

- **[233boy/sing-box](https://github.com/233boy/sing-box)** — 最被采纳的 sing-box 一键 + 管理脚本。短入口 `sb` + 短动词（`a/add`、`c/change`、`d/del`、`i/info`、`v/version`、`s/status`、`log`、`qr`、`url`）。同一脚本 = menu + 直接子命令双轨。
- **[FranzKafkaYu/sing-box-yes](https://github.com/FranzKafkaYu/sing-box-yes)**（英文 fork：[MiSaturo/sing-box-yes-english](https://github.com/MiSaturo/sing-box-yes-english)） — 社区公认最干净的菜单脚本架构参考。
- **[TheyCallMeSecond/sing-box-manager](https://github.com/TheyCallMeSecond/sing-box-manager)** — `/usr/local/bin/singbox` symlink 安装模式 + 多协议共存。
- **[chise0713/sing-box-install](https://github.com/chise0713/sing-box-install)** — 纯 action/option 风格（无菜单）的反例，可作为 Approach C "CLI all-in" 的参考。
- **[官方 sing-box.app/deb-install.sh](https://sing-box.sagernet.org/installation/package-manager/)** — 最简的 package manager 路径，分发参考。

#### 直接戳中本项目痛点的模式

下面表格把社区做法和 minimalist 当前实现对照（启动 `/design-consultation` 时直接拿来反推改造清单）：

| 模式 | minimalist 当前 | 社区做法（233boy / sing-box-yes） | 借鉴成本 |
|------|------|------|------|
| 短入口别名 | `minimalist menu`（11 字符） | `sb`（2 字符），install-self 时多建一个 symlink | 极低（几行 Go） |
| 双轨：menu + 短动词 | menu 和 CLI 是两套 | 同一脚本：`sb` 进菜单 / `sb a` 直接 add | 已有半套 CLI，加短别名映射即可 |
| 操作后回菜单 | `case "1": return a.ListNodes()` 看完即退 | `action && show_menu` 链式回到菜单 | 改 nodesMenu/subscriptionsMenu 的 case 分支 |
| 看 + 操作合并 | "看节点" / "启用节点" 是 4 个独立菜单项 | `i/info` 显示完后菜单仍在，可接着 `c/change` | 重构 nodesMenu 数据流 |
| 顶部 status header | "状态总览" 是菜单选项 1 | 进 menu 第一行就是 service status / 节点数 / controller 状态 | 在 Menu() 循环顶部加一段 ensureAll + 渲染 |
| 高风险动作二次确认 | 删除节点 / 删除订阅没有 confirm | 删除类强制 `confirm()` y/n（默认 n） | 加一个 promptConfirm helper |
| 统一日志层级 | 散落的 `fmt.Fprintln` | `LOGD/LOGI/LOGE/LOGW` 分级 + 颜色 | 包一层 helper，逐步替换 |
| 状态常量 | 散落字符串 | `readonly STATUS_RUNNING / STATUS_NOT_RUNNING / STATUS_NOT_INSTALL` | Go 这边用 const block 即可 |
| 备份 + 滚动恢复 | `core-upgrade-alpha` 已有 | 升级前 backup config，失败回滚 | 已对齐，无需借鉴 |
| GitHub API 取版本 | `core-upgrade-alpha` 已有 | tag_name 解析 + 架构匹配 | 已对齐，无需借鉴 |

#### 关键洞察

- **绝大多数痛可以用"短别名 + 操作后回菜单 + 顶部 status header"三件套消除**——这是 Approach A（最小修补）的核心组合，比 TUI 重写性价比高得多。
- **真正需要 Approach B（TUI 重写）的只有"看节点列表里直接对节点 N 发命令"**——bubbletea 才有的列表内操作。如果接受"看完后菜单仍在 + 短命令"折中，A + 短别名可能就够了。
- **Approach C（CLI all-in）社区里也有人在做**（chise0713），但主流仍是双轨。建议 **A + 短别名（双轨）** 作为下一轮 design-consultation 的默认推荐起点。

### 重要功能优先级（用户 mental model — 2026-04-30 自陈）

下一轮重设计的**顶层导航分组**应按这 5 类，不再是当前 7 个组：

| # | 分组 | 当前实现现状 | 重设计动作 |
|---|------|------|------|
| 1 | **节点管理** | `nodesMenu` + `nodes list/test/rename/enable/disable/remove` | UX 重做（参考前面 9 项对照表），功能完整 |
| 2 | **配置管理** | `router-wizard`（名字不直观）+ 散落的 `ruleset/custom.rules` `acl.rules` | 重命名为 "配置管理"（或类似），统一入口；散落的配置文件指针化（菜单不直接编辑文本，但显式指向路径） |
| 3 | **规则管理** | `rulesAndACLMenu` + `rules-repo` + `acl` + `rules` — 当前散在 "网络入口与规则仓库" 和 "规则与 ACL" 两组 | 合并成单一 "规则管理" 入口：自定义 / ACL / 仓库规则 / 默认 tail 都在这里看 |
| 4 | **日志** | **完全缺失** — 不在 7 菜单组，不在 25+ CLI 命令 | **新增**：`minimalist log` 命令 + 菜单 "日志" 入口，封装 `journalctl -u minimalist.service` 与 `mihomo-core` 日志的常用过滤（最近 N 行、follow、warn/error 过滤、特定时段） |
| 5 | **控制启停** | `serviceMenu` + `start/stop/restart/status` | 名字 "服务管理" → "控制启停" 更直观；功能完整 |

**当前 7 组中需要降权 / 重排的**：
- "状态总览"（菜单选项 1）→ 升级为顶部 status header（参考 233boy 模式），不再是菜单项
- "部署/修复"（`install-self/setup/render-config`）→ 合并到 "控制启停"，作为高级二级菜单
- "订阅管理" → 降级为 "配置管理" 下的子项（与 `docs/TASKS.md` P2 "把订阅能力正式降级为增强项" 一致）
- "网络入口与规则仓库" → 拆开：网络入口归 "配置管理"，规则仓库归 "规则管理"
- "健康检查与审计" → 部分归 "日志"（`runtime-audit` / `healthcheck`），部分归 "控制启停"（`cutover-preflight` / `cutover-plan`）

### 待补的功能缺口（重设计时一并补）

#### 1. 日志查看入口（完全新增）

当前要看日志只能 `journalctl -u minimalist.service` / `journalctl -u mihomo-core` 手敲，不在菜单也不在 CLI。

**最小可行设计**：
- `minimalist log` — 默认显示 minimalist.service 最近 50 行
- `minimalist log -f` — follow
- `minimalist log mihomo` — 切换到 mihomo-core 日志
- `minimalist log --errors` — 只看 warn/error
- 菜单 "日志" 入口：默认渲染最近 50 行 minimalist.service + `runtime-audit` 的 `alerts-recent`，提供 "查看 mihomo 日志" / "follow 模式" / "只看 error" 三个子动作

**实现成本**：低（封 `os/exec` 起 `journalctl`，CLI 子命令分发）

#### 2. 接管宿主机流量一键开关（功能已存在，UX 缺失）

`proxy_host_output` 字段功能完整，但目前**只能通过走完整个 `router-wizard` 多步流程切换**。需要独立成显式 toggle：

**最小可行设计**：
- `minimalist host-proxy status` — 当前是 on 还是 off
- `minimalist host-proxy on` / `minimalist host-proxy off` — 切换并 apply-rules
- 菜单 "配置管理" 顶部一个显眼 toggle：`[x] 宿主机接管: 关闭`（对应 233boy 的 status header 模式）
- 切换时强制 `confirm()` 二次确认（高风险动作）+ 显示当前 iptables `OUTPUT` 链状态对比

**实现成本**：低（字段已经存在，只需要新 CLI 子命令 + apply-rules 复用）

**重要约束**：
- **默认必须是 off**（README 明确：`proxy_host_output: false`，主线不默认接管宿主机流量）
- 切换为 on 后必须有显著视觉提示（status header 红字 / "宿主机已接管"）
- 重启 / reboot 后保持上次的状态（已经写入 config.yaml，自动满足）

#### 3. 短入口别名（参考实现段已讨论）

`m` symlink + 短动词映射，install-self 时多建一个 symlink 即可。零工程量。


### 观察期内的 self-observation 任务（24-72h，可填）

每次自己用 `minimalist menu` 或 CLI 时，记一条到下方"高频路径日志"：
- 想做的事（"启用一个新节点"、"看 7890 是不是通"）
- 实际走了哪几步
- 哪一步最让你无语（一句原话）

目标 5-10 条真实路径，作为重设计时的高频路径输入。比"凭印象设计"靠谱得多。

#### 高频路径日志

<!-- 模板：YYYY-MM-DD HH:MM — 想做 X → 实际走了 N 步 → 第 K 步让我"……"（原话） -->

(待填)
