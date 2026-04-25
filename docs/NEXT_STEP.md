# 下一步

## 当前阶段

- 下一闭环进入阶段 5：代码结构收口。
- 当前已完成第九十一刀：`ExecStart=` 行已抽到共用 helper。
- 目标不是扩功能，而是在不改变行为的前提下，继续消除 `render_manager_sync_service_unit` / `render_manager_sync_timer_unit` 周边的结构重复逻辑。

## 下一最小闭环

- 提取 `OnBootSec=` 行 helper
- 保持现有输出文本、顺序与退化行为不变
- 补 focused tests，确保重构不改变行为
- 文档同步切到阶段 5 当前真相

## 本轮不做

- 不新增用户可感知功能
- 不调整运行态真相边界
- 不扩 `external-controller-tls` 实现
- 不做大规模脚本拆分

## 退出条件

- `OnBootSec=` 行不再内联在 timer 静态设置 helper 中
- 现有 `status` 输出文本、顺序与入口展示行为保持不变
- 相关 smoke / service-mock 回归通过
- 文档同步更新当前阶段结论
