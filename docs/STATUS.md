# 当前状态

## 当前主线

- 当前主线已进入阶段 5，已完成第三刀：`status` / `runtime-audit` 已共享控制面与网络访问静态信息展示 helper。
- 项目权威文档基线已补齐并生效：`STATUS.md`、`NEXT_STEP.md`、`DECISIONS.md`、`ARCHITECTURE.md`。

## 当前真相

- 默认分支：`main`
- 当前定位仍是 Debian NAS / IPv4 旁路由 / `iptables + TProxy`
- 宿主机默认直连，`PROXY_HOST_OUTPUT=0`
- 控制面默认仅绑定 `127.0.0.1:${CONTROLLER_PORT:-19090}`
- 订阅原始 provider 缓存位于 `proxy_providers/subscriptions/*.txt`
- `mihomo nodes` 当前只展示手动节点；订阅节点只保留只读枚举状态

## 阶段结论

- 阶段 1：已完成，README 已与 Debian NAS / IPv4 旁路由定位对齐
- 阶段 2：已完成主线收口，订阅缓存已成为 provider 输入真相源，订阅节点退出主节点交互路径
- 阶段 3：已完成
  - 已接入 `lan-disallowed-ips`
  - 已接入 `authentication`
  - 已接入 `skip-auth-prefixes`
  - 已接入 `external-ui-name`
  - 已接入 `external-ui-url`
  - 已接入 `external-controller-cors.allow-origins`
  - 已接入 `external-controller-cors.allow-private-network`
  - `external-controller-tls` 已明确暂缓，不进入当前阶段实现
- 阶段 4：已完成
  - `mihomo status` 的“当前模式”已优先读取 Mihomo REST API `/configs`
  - `mihomo runtime-audit` 的“当前模式”已优先读取 Mihomo REST API `/configs`
  - `mihomo status` 已优先读取 Mihomo REST API `/proxies`，输出最小策略组运行态摘要
  - `mihomo runtime-audit` 已优先读取 Mihomo REST API `/proxies`，输出最小策略组运行态摘要
  - `mihomo status` 已优先读取 Mihomo REST API `/version`，输出最小控制面运行态摘要
  - `mihomo runtime-audit` 已优先读取 Mihomo REST API `/version`，输出最小控制面运行态摘要
  - 控制面不可达时会回退到本地 `config.yaml`
  - 控制面不可达时，`status` / `runtime-audit` 的策略组摘要会显示“未获取”，不影响其他状态输出
  - 控制面不可达时，`status` / `runtime-audit` 的控制面运行态摘要会显示“未获取”
- 阶段 5：已开始
  - `status` / `runtime-audit` 的运行态摘要拼装逻辑已抽到共用 helper
  - `status` / `runtime-audit` 的控制面静态信息展示已抽到共用 helper
  - `status` / `runtime-audit` 的网络与访问静态信息展示已抽到共用 helper
  - 当前行为与输出文本保持不变
  - 模板/规则预设/IPv6 这组展示字段仍存在重复逻辑，尚未继续收口

## 质量状态

- 默认分支最近闭环均已通过回归并提交
- 当前回归入口：`tests/smoke.sh`、`tests/service_mock.sh`
- 本轮验证结果：
  - `2026-04-25` 执行 `bash tests/smoke.sh`，通过
  - `2026-04-25` 执行 `bash tests/service_mock.sh`，通过

## 当前风险与限制

- `status` / `runtime-audit` 仍存在模板/规则预设/IPv6 展示上的重复逻辑，阶段 5 后续仍需继续收口
- `scripts/statectl.py` 仍保留过渡期协议解析逻辑，尚未退化为更小的状态工具
- `nas-single-lan-dualstack` 仅兼容保留，不代表项目已支持真双栈旁路由
