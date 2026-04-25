# 下一步

## 当前阶段

- 当前主线处于阶段 5：代码结构收口。
- 阶段 5 第一轮已完成运行态/审计展示、安装与同步链路、manager sync unit 渲染链的共用逻辑收口。
- 从本轮开始，阶段 5 默认按职责块推进，不再以“第 N 刀”或单行 helper 抽取作为闭环目标。

## 下一最小闭环

- 在 `lib/render.sh` 收口 `render_config` 的块级边界
- 优先拆出访问/控制面基础段、DNS 与基础配置段、provider/rules 装配段
- 保持现有配置文本、输出顺序与退化行为不变
- 补 focused tests，确保块级收口不改变行为
- 文档同步切到阶段 5 当前真相

## 本轮不做

- 不新增用户可感知功能
- 不调整运行态真相边界
- 不扩 `external-controller-tls` 实现
- 不继续新增 manager sync unit 单行 helper
- 不做跨文件大规模拆分

## 退出条件

- `render_config` 的职责块边界更清晰，重复读取/拼装逻辑下降
- `config.yaml` 关键输出、`status` / `runtime-audit` 行为和退化路径保持不变
- 相关 smoke / service-mock 回归通过
- 文档同步更新当前阶段结论
