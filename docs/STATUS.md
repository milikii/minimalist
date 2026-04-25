# 当前状态

## 当前主线

- 当前主线已完成阶段 3 的字段对齐收口，下一闭环进入阶段 4 的运行态读取改造。
- 本轮已补齐项目要求的权威文档基线：`STATUS.md`、`NEXT_STEP.md`、`DECISIONS.md`、`ARCHITECTURE.md`。

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
- 阶段 4：未开始，`show_status` / `runtime-audit` 仍主要依赖本地配置与 systemd 信息，不以 Mihomo REST API 为主真相

## 质量状态

- 最近稳定提交：`0be19f6 feat: expose controller cors fields`
- 当前回归入口：`tests/smoke.sh`、`tests/service_mock.sh`
- 本轮验证结果：
  - `2026-04-25` 执行 `bash tests/smoke.sh`，通过
  - `2026-04-25` 执行 `bash tests/service_mock.sh`，通过

## 当前风险与限制

- 运行态模式、策略组选择和控制面摘要还没有从 Mihomo REST API 读取
- `scripts/statectl.py` 仍保留过渡期协议解析逻辑，尚未退化为更小的状态工具
- `nas-single-lan-dualstack` 仅兼容保留，不代表项目已支持真双栈旁路由
