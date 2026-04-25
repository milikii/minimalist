# Mihomo Manager 重构路线（Debian NAS 定向）

本文档用于固定当前项目的重构判断，后续会话默认以本文档为执行依据。

## 1. 项目定位

本项目不是通用 Mihomo 客户端，也不追求覆盖 ShellCrash / OpenClash 的全部能力。

本项目的目标固定为：

- 面向 Debian NAS
- 面向宿主机侧部署
- 面向 IPv4 旁路由
- 默认宿主机直连，局域网设备通过 NAS 走旁路由
- 允许控制面按需开放到局域网

本项目当前不追求：

- OpenWrt / firewall4 / nftables 通用兼容
- 真双栈旁路由
- 覆盖 Mihomo 全部代理类型的自研导入解析
- 复刻成熟生态项目的大而全功能面

## 2. 核心判断

### 2.1 协议导入方向

后续原则：

- 不再把“自研解析 URI 并转成 provider YAML”作为主路线。
- 优先使用 Mihomo 官方已经支持的 `proxy-providers` 内容格式。
- 订阅或节点源如果本来就是官方支持的 `yaml / uri / base64` 之一，应尽量直接交给 Mihomo 解析。

这意味着：

- `scripts/statectl.py` 中现有的 `vless/vmess/trojan/ss` 解析逻辑不应继续扩展成更大的协议兼容层。
- 后续即使补协议，也应优先做“过渡兼容”而不是长期主架构。
- 如果需要节点筛选、重命名、前后缀、类型过滤，应优先评估官方 `proxy-providers` 的 `filter / exclude-filter / exclude-type / override / proxy-name` 能否覆盖。

明确结论：

- “让 Mihomo 官方支持的 provider 内容格式成为真相源”是对的。
- “依赖 Mihomo 提供一个官方 CLI 帮我们做 URI 转换”不是当前主路线，因为官方文档没有把这件事定义为推荐工作流。

参考：

- General configuration: https://wiki.metacubex.one/en/config/general/
- proxy-providers configuration: https://wiki.metacubex.one/en/config/proxy-providers/
- proxy-providers contents: https://wiki.metacubex.one/en/config/proxy-providers/content/

### 2.2 官方优先

后续配置能力优先级：

1. 先看 Mihomo 官方已有字段
2. 再决定是否暴露到本项目
3. 最后才考虑自研替代实现

这直接影响以下模块：

- WebUI 下载与更新
- LAN 边界控制
- 控制面认证与跳过认证网段
- UI 路径与下载源
- 运行态读取方式

优先参考的官方字段包括：

- `lan-allowed-ips`
- `lan-disallowed-ips`
- `authentication`
- `skip-auth-prefixes`
- `external-controller-cors`
- `external-controller-tls`
- `external-ui`
- `external-ui-name`
- `external-ui-url`

### 2.3 Debian NAS 特化

这是明确保留的方向，不需要为了“通用性”反向复杂化。

后续允许做的收窄：

- 默认只保证 Debian
- 默认只保证 IPv4 旁路由
- 默认只保证 `iptables + TProxy`
- 不主动补 OpenWrt / firewall4 / nftables 抽象

对应动作：

- `nas-single-lan-dualstack` 应在后续重构中删除，或明确标注为未实现，不允许继续以“真双栈”对外表达。

### 2.4 运行态真相来源

当前项目大量状态来自本地文件，不是来自 Mihomo 运行态。

后续原则：

- 静态意图用本地文件保存
- 运行态状态优先来自 Mihomo REST API

优先改造对象：

- 当前模式
- 策略组选择
- 控制面可达性与 API 状态
- 运行时节点/策略摘要

允许继续保留本地文件真相的部分：

- 安装设置
- 本项目自己的部署参数
- Debian NAS 旁路由向导结果

### 2.5 稳定性优先级

对 NAS 管理脚本，默认值应偏保守。

重构方向：

- 默认核心通道应改为 `stable`
- `alpha` 保留为显式选择，不作为默认

参考：

- FAQ: https://wiki.metacubex.one/en/startup/faq/

### 2.6 代码质量方向

后续不做“大重写”，做结构性收口。

原则：

- Shell 只做流程编排、systemd、iptables、文件布局
- 协议解析、规则清单处理等复杂数据逻辑尽量下沉，且能删则删
- 优先减少长函数、全局变量耦合、重复读取 env 的模式
- 先做边界清晰，再做抽象

## 3. 具体重构顺序

### 阶段 1：边界收口

目标：

- 删除或降级未真正支持的能力表达
- 把 README 与实际行为对齐

任务：

- 删除 `nas-single-lan-dualstack`，或显式标记未实现
- 明确项目仅针对 Debian NAS / IPv4 旁路由
- 明确 `apply-default-template`、控制面绑定、宿主机流量模式的真实语义

### 阶段 2：导入链路改造

目标：

- 让官方支持的 provider 内容格式成为主路线

当前进展：

- 已把订阅更新落盘为原始 provider 缓存文件：`proxy_providers/subscriptions/*.txt`
- 已让运行配置优先消费这些 provider 缓存，而不是把订阅节点重新渲染进 `manual.txt`
- 订阅节点当前在本地节点列表中仅作为只读枚举缓存，避免本地改名/开关与运行态脱节
- 状态页已拆分“手动节点 / 订阅 provider / 订阅缓存”，开始从“导入节点思维”转向“provider 缓存思维”
- 规则目标已经收紧为只允许手动节点，订阅缓存节点不再被视为稳定的具名目标
- `subscriptions.json` 已开始从扁平字段迁移到两组子对象：
  - `cache`: provider 缓存状态
  - `enumeration`: 本地只读枚举状态
- `mihomo nodes` 已收口为只展示手动节点，订阅缓存节点不再进入节点交互主路径

任务：

- 重新设计订阅落盘格式
- 评估是否可直接存 `uri/base64/yaml` 作为 provider 输入
- 让 `statectl.py` 从“协议解析器”退化为“最小状态工具”
- 停止继续扩展自研协议解析范围

### 阶段 3：安全与官方字段对齐

目标：

- 用官方配置字段替代自研或缺失能力

当前进展：

- 已接入 `lan-disallowed-ips`
- 已接入 `authentication`
- 已接入 `skip-auth-prefixes`
- 已接入 `external-ui-name`
- 已接入 `external-ui-url`
- 已接入 `external-controller-cors`
- 状态页和运行审计已开始展示这些字段
- 这里的 `authentication` / `skip-auth-prefixes` 当前只覆盖显式代理端口，不是控制面认证
- 当前 `external-controller-cors` 只覆盖 `allow-origins` / `allow-private-network`，不包含 TLS 证书管理

任务：

- 暴露 `authentication`
- 暴露 `skip-auth-prefixes`
- 暴露 `lan-disallowed-ips`
- 评估并接入 `external-ui-name` / `external-ui-url`
- 评估 `external-controller-cors` / `external-controller-tls`

### 阶段 4：运行态读取改造

目标：

- 让状态页和审计结果优先基于 Mihomo 运行态

当前进展：

- `mihomo status` 的“当前模式”已优先读取 Mihomo REST API `/configs`
- `mihomo runtime-audit` 的“当前模式”已优先读取 Mihomo REST API `/configs`
- `mihomo status` 已能读取 Mihomo REST API `/proxies`，输出最小策略组运行态摘要
- `mihomo runtime-audit` 已能读取 Mihomo REST API `/proxies`，输出最小策略组运行态摘要
- `mihomo status` 已能读取 Mihomo REST API `/version`，输出最小控制面运行态摘要
- `mihomo runtime-audit` 已能读取 Mihomo REST API `/version`，输出最小控制面运行态摘要
- 控制面不可达时已回退到本地 `config.yaml`，避免状态页因控制面短暂失败而退化不可用

任务：

- 用 REST API 读取当前模式
- 读取策略组当前选择
- 区分“配置文件状态”和“运行态状态”

### 阶段 5：代码结构收口

目标：

- 降低 shell 脚本复杂度

当前进展：

- `status` / `runtime-audit` 的运行态摘要拼装逻辑已抽到共用 helper
- `status` / `runtime-audit` 的控制面静态信息展示已抽到共用 helper
- `status` / `runtime-audit` 的网络与访问静态信息展示已抽到共用 helper
- `status` / `runtime-audit` 的模板/规则预设/IPv6 展示已抽到共用 helper
- `status` / `runtime-audit` 的计数类与节点统计展示已抽到共用 helper
- `status` 的推荐下一步判断已抽到共用 helper
- `status` 的警告与收尾展示已抽到共用 helper
- `status` 的同步与端口展示已抽到共用 helper
- `status` 的 WebUI / 控制面密钥入口展示已抽到共用 helper
- `status` 推荐下一步所需的计数预处理已抽到共用 helper
- `status` 的基础概览展示已抽到共用 helper
- `status` 的基础状态采集已抽到共用 helper
- `runtime-audit` 的探测与流量摘要展示已抽到共用 helper
- `runtime-audit` 的告警与定时器展示已抽到共用 helper
- `runtime-audit` 的基础概览展示已抽到共用 helper
- `runtime-audit` 的基础状态采集已抽到共用 helper
- `runtime-audit` 的探测状态采集已抽到共用 helper
- `runtime-audit` 的健康摘要收尾已抽到共用 helper
- `runtime-audit` 的告警与定时器状态采集已抽到共用 helper
- `healthcheck` 的端口监听检查已抽到共用 helper
- `healthcheck` 的探测检查已抽到共用 helper
- `diagnose` 的配置摘要展示已抽到共用 helper
- `diagnose` 的 systemd / listeners / logs 分段展示已抽到共用 helper
- `healthcheck` 的基础状态检查已抽到共用 helper
- `audit_installation` 的基础文件存在性检查已抽到共用 helper
- `audit_installation` 的 nodes/rules 渲染漂移检查已抽到共用 helper
- `audit_installation` 的 ACL / 规则预设检查已抽到共用 helper
- `audit_installation` 的 timer / GeoSite 检查已抽到共用 helper
- `audit_installation` 的成功收尾已抽到共用 helper
- `install_geosite_dat` 的成功安装收尾已抽到共用 helper
- `install_webui` 的下载阶段已抽到共用 helper
- `install_webui` 的解压与源码目录识别已抽到共用 helper
- `install_webui` 的部署与持久化收尾已抽到共用 helper
- `install_webui` 的失败收尾已抽到共用 helper
- `install_webui` 的临时工作区清理已抽到共用 helper
- `install_webui` 的参数与目标解析已抽到共用 helper
- `install_webui` 的临时工作区准备已抽到共用 helper
- `install_project_sync` 的入参校验已抽到共用 helper
- `install_project_sync` 的设置持久化已抽到共用 helper
- `disable_project_sync` 的设置重置已抽到共用 helper
- `disable_project_sync` 的运行时清理已抽到共用 helper
- `install_project_sync` 的 systemd 激活收尾已抽到共用 helper
- `install_project_sync` 的成功提示收尾已抽到共用 helper
- `write_manager_sync_units` 的 service unit 写入已抽到共用 helper
- `write_manager_sync_units` 的 timer unit 写入已抽到共用 helper
- `disable_project_sync` 的成功提示收尾已抽到共用 helper
- `install_webui` 的解压失败告警已恢复为可见输出
- 当前仍保持与重构前一致的输出文本与退化行为

任务：

- 拆分 `mihomo` 主脚本中的长函数
- 拆分 `lib/render.sh` 中配置渲染、系统集成、健康审计逻辑
- 明确 `common/render/runtime/install` 等边界

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
