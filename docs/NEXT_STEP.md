# 下一步

## 当前阶段

- 当前主线处于阶段 5：代码结构收口。
- 阶段 5 第一轮已完成运行态/审计展示、安装与同步链路、manager sync unit 渲染链的共用逻辑收口。
- `lib/render.sh` 的 `render_config` 已完成当前块级收口，输出顺序 focused tests 已补齐。
- `mihomo` 的运行前准备与服务启停编排已完成当前最小收口。
- `mihomo` 的部署与修复编排已完成当前最小收口。
- `mihomo` 的订阅刷新编排已完成当前最小收口。
- `mihomo` 的交互导入编排已完成当前最小收口。
- `mihomo` 的交互网络向导编排已完成当前最小收口。
- `mihomo` 的 CLI 入口分发已完成当前最小收口。
- `mihomo` 的交互菜单编排已完成当前最小收口。
- `scripts/statectl.py` 的协议解析链已完成当前最小收口。
- `scripts/statectl.py` 的 provider 渲染链已完成当前最小收口。
- 阶段 5 下一闭环转向 `scripts/statectl.py` 的 provider 传输层选项热点。

## 下一最小闭环

- 在 `scripts/statectl.py` 收口 provider 组装尾段
- 优先围绕 `build_vless_provider_item` 与 `render_vless_xhttp_opts` 的剩余耦合
- 保持现有 provider YAML 字段、协议支持范围和渲染输出不变
- 优先复用现有 `smoke` 的 `test_protocol_renderers` / `render-config` 回归，必要时只补最小 focused tests
- 文档同步切到阶段 5 当前真相

## 本轮不做

- 不新增用户可感知功能
- 不调整运行态真相边界
- 不扩 `external-controller-tls` 实现
- 不继续新增 manager sync unit 单行 helper
- 不回退已完成的 `render_config` 职责块收口
- 不回退已完成的运行前准备与服务启停编排收口
- 不回退已完成的部署与修复编排收口
- 不回退已完成的订阅刷新编排收口
- 不回退已完成的交互导入编排收口
- 不回退已完成的交互网络向导编排收口
- 不回退已完成的 CLI 入口分发收口
- 不回退已完成的交互菜单编排收口
- 不回退已完成的协议解析链收口
- 不回退已完成的 provider 渲染链收口
- 不做跨文件大规模拆分

## 退出条件

- provider 传输层选项 helper 的职责块边界更清晰，TLS/network/xhttp 组合复杂度下降
- provider YAML 字段、协议支持范围和渲染输出保持不变
- 相关 smoke / service-mock 回归通过
- 文档同步更新当前阶段结论
