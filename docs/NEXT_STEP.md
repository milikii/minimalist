# 下一步

## 当前阶段

- 下一闭环进入阶段 5：代码结构收口。
- 当前已完成第五十刀：`finalize_project_install` 的可执行权限设置已抽到共用 helper。
- 目标不是扩功能，而是在不改变行为的前提下，继续消除 `install_project_sync` / `disable_project_sync` 周边的结构重复逻辑。

## 下一最小闭环

- 提取 `finalize_project_install` 的成功提示 helper
- 保持现有输出文本、顺序与退化行为不变
- 补 focused tests，确保重构不改变行为
- 文档同步切到阶段 5 当前真相

## 本轮不做

- 不新增用户可感知功能
- 不调整运行态真相边界
- 不扩 `external-controller-tls` 实现
- 不做大规模脚本拆分

## 退出条件

- `finalize_project_install` 的成功提示不再与封装函数内联耦合
- 现有 `status` 输出文本、顺序与入口展示行为保持不变
- 相关 smoke / service-mock 回归通过
- 文档同步更新当前阶段结论
