# Go V2 路线说明

原 `mihomo` shell/Python 收口路线已经被 Go V2 `minimalist` 实现取代。

当前路线固定为：

- 项目名与命令名：`minimalist`
- 主实现语言：Go
- 产品边界：Debian NAS / IPv4 旁路由 / `iptables + TProxy`
- 保留能力：核心主路径 + 规则/订阅
- 不做旧状态迁移

当前文档真相请以以下文件为准：

- `docs/STATUS.md`
- `docs/NEXT_STEP.md`
- `docs/DECISIONS.md`
- `docs/ARCHITECTURE.md`

任务：

- 用 REST API 读取当前模式
- 读取策略组当前选择
- 区分“配置文件状态”和“运行态状态”

### 阶段 5：代码结构收口

目标：

- 降低 shell 脚本复杂度
- 优先减少长函数与跨职责编排
- 以职责块收口为主，不再默认按单行 helper 推进

当前进展：

- 已完成一轮运行态与审计展示块收口，覆盖 `status`、`runtime-audit`、`healthcheck`、`diagnose`、`audit_installation`
- 已完成一轮安装与同步块收口，覆盖 `install_webui`、`install_project`、`install_project_sync`、`disable_project_sync`、`finalize_project_install`
- 已完成 manager sync unit 渲染链的当前最小收口，通用 render/write、sections、timer static settings、service body 已抽离
- 已完成 `lib/render.sh` 的 `render_config` 当前块级收口，访问/控制面、DNS 基础配置、显式代理认证、provider/group、rules 尾段均已独立
- 已完成 `mihomo` 运行前准备与服务启停编排的当前最小收口，runtime support files、runtime geo assets、runtime core guard、prepared start/restart/enable-start 已独立
- 已完成 `mihomo` 部署与修复编排的当前最小收口，setup/repair context、repair WebUI、setup maintenance、setup service finalize 已独立
- 已完成 `mihomo` 订阅刷新编排的当前最小收口，single subscription refresh、success/failure recording、update-subscriptions orchestration 已独立
- 已完成 `mihomo` 交互导入编排的当前最小收口，input source selection、URI collection、manual node append、scan result processing 已独立
- 已完成 `mihomo` 交互网络向导编排的当前最小收口，current config intro、core input collection、core env writes、detected lan cidrs、bypass env flow 已独立
- 已完成 `mihomo` CLI 入口分发的当前最小收口，default entry fallback、shared core update dispatch、workflow command dispatch、maintenance command dispatch 已独立
- `install_webui` 的解压失败告警已恢复为可见输出
- 当前仍保持与重构前一致的输出文本与退化行为

下一优先级：

1. 先收口 `mihomo` 主脚本中更易验证的交互长编排
   - 优先 `interactive_menu`
2. 再收口 `mihomo` 其他长编排函数
   - 优先 menus / diagnostics side dispatch
3. 最后为 `scripts/statectl.py` 退化做准备
   - 限制新增协议解析逻辑
   - 优先把状态迁移、provider 渲染、CLI 入口边界写清

阶段约束：

- 不继续以 manager sync unit 的单行 helper 抽取作为默认推进目标
- 不在阶段 5 顺手扩控制面能力、协议范围或做大规模跨文件拆分
- 每轮必须有 focused tests 或现有 `smoke` / `service_mock` 覆盖

任务：

- 收口 `mihomo` 的长编排函数边界
- 为 `statectl.py` 退化成更小状态工具准备边界

### 阶段 6：测试升级

目标：

- 增加真实集成验证，而不是只靠 mock

任务：

- 增加基于真实 `mihomo-core -t` 的回归
- 增加最小 systemd 单元写入校验
- 增加最小 iptables/TProxy 规则联动验证

## 4. 当前明确不做

- 不把项目扩展成 OpenWrt 通用方案
- 不补全所有 Mihomo 代理类型的自研 URI 解析器
- 不为了“看起来高级”引入大规模抽象层
- 不把单机 Debian NAS 项目改造成前后端分离系统

## 5. 后续会话执行规则

新会话默认按以下顺序推进：

1. 先确认当前阶段
2. 只做该阶段最小必要修改
3. 修改代码同时更新 README 或本路线文档
4. 每个阶段完成后再进入下一个阶段

如果后续判断与本文档冲突，以最新 commit 中的本文档为准。
