# AGENTS.md — minimalist

> 继承全局 AGENTS.md 所有规则。本文件仅补充项目特定约束。
> 本文件与全局 AGENTS.md 冲突时，本文件优先。

---

## 一、当前阶段 ← 每次推进前手动更新这一行

**当前阶段：长期稳定运行观察 / 工具：实机 smoke + runtime-audit**

```
阶段选项（手动切换）：
- 决策阶段        → Gstack /plan-ceo-review /plan-eng-review
- 拆解阶段        → Superpowers /writing-plans
- 执行阶段        → Superpowers /executing-plans   ← 编码时锁在这里
- 排查阶段        → Superpowers /investigate
- 收尾阶段        → Gstack /qa → Superpowers /ship
- 长期稳定运行观察 → 实机 smoke + runtime-audit + journalctl   ← 当前在这里
```

---

## 二、项目基本信息

```
项目名：minimalist
技术栈：Go 1.24、systemd、mihomo-core、iptables + TProxy、YAML/JSON
部署目标：Debian NAS 单机旁路由宿主机（IPv4）
仓库：git@github-mihomo-nas:milikii/minimalist.git
```

---

## 三、必读文件清单（每轮执行前按序读取）

```
docs/PRD.md            ← 需求源头（只读，执行中不得修改）
docs/ARCHITECTURE.md   ← 架构决策（只读，执行中不得修改）
docs/TASKS.md          ← 当前任务列表（执行时标记完成状态）
docs/STATUS.md         ← 当前主线状态与实机验证结论
docs/NEXT_STEP.md      ← 当前推荐闭环与退出条件
docs/PROGRESS.md       ← 进度日志（每轮追加，不得覆盖历史）
docs/BLOCKERS.md       ← 阻断问题（遇到时写入，停止并报告）
docs/DECISIONS.md      ← 执行中遇到的新决策点记录
```

启动检查：若 `docs/PRD.md`、`docs/ARCHITECTURE.md`、`docs/TASKS.md`、`docs/STATUS.md`、`docs/NEXT_STEP.md`、`docs/DECISIONS.md` 缺失，停止执行并报告缺失文件，不得猜测内容继续推进。
`docs/PROGRESS.md` 不存在时，创建并写入 "Round 0: 项目初始化" 后继续。
`docs/BLOCKERS.md` 不存在时，创建空文件后继续。

---

## 四、执行循环协议（单轮标准流程）

每轮严格按以下顺序执行，不得跳步：

```
STEP 1 [读取]
  → 读 docs/PROGRESS.md 确认上一轮终止点
  → 读 docs/TASKS.md 找到第一个未完成任务
  → 读 docs/PRD.md 对齐当前目标

STEP 2 [规划]
  → 列出本轮要完成的具体任务（最多 3 项）
  → 确认每项任务与 PRD 有明确对应

STEP 3 [执行]
  → 实现代码
  → 所有新功能必须有对应测试
  → 不允许出现 TODO / FIXME / placeholder 注释

STEP 4 [验证]
  → 运行测试：`env GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./...`
  → 运行静态检查：`env GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go vet ./...`
  → 运行格式检查：`gofmt -l cmd internal`（输出必须为空）
  → 测试或 lint 失败时，本轮内修复，不得带着失败进入下一轮

STEP 5 [提交]
  → git add -A
  → git commit -m "feat: [本轮完成内容的简洁描述]"
  → 更新 docs/TASKS.md：已完成项打勾
  → 追加 docs/PROGRESS.md（格式见下方）

STEP 6 [继续]
  → TASKS.md 还有未完成项 → 立即开始下一轮
  → 全部完成 → 输出完成摘要，等待用户指令
  → 不得询问用户，不得等待确认（blocker 除外）
```

---

## 五、PROGRESS.md 写入格式

每轮追加如下格式，不得覆盖历史：

```markdown
## Round [N] — [YYYY-MM-DD HH:MM]

### 完成
- [具体完成项]

### 测试状态
- 通过: X / 总计: Y

### 遗留 / 下轮继续
- [如有]

### 下轮目标
- [明确的下一步]
```

---

## 六、代码质量硬性要求

```
- 无任何硬编码密钥 / 密码（使用环境变量，写入 .env.example）
- 无孤立未调用的函数 / 方法 / 变量
- 无注释掉的废弃代码块
- 每个导出函数 / 类型有符合 Go 习惯的 doc comment
- 错误必须显式返回或处理，不允许静默吞错（明确忽略且有理由的除外）
- 文件结构严格遵循 docs/ARCHITECTURE.md 中定义的目录规范
- 不引入 ARCHITECTURE.md 未列出的新依赖
```

---

## 七、禁止行为

```
✗ 不得在未完成当前任务时切换到其他任务
✗ 不得推翻 ARCHITECTURE.md 中已锁定的架构决策
✗ 不得修改已通过测试的代码（除非 PRD 明确要求）
✗ 不得创建超出 PRD 范围的功能（即使你认为有用）
✗ 不得在测试失败时执行 git commit
✗ 不得省略 docs/PROGRESS.md 的更新
✗ 不得静默跳过 blocker，必须记录并报告
```

---

## 八、一键启动命令

### 执行阶段（最常用）
```
codex "读取 AGENTS.md 和 docs/TASKS.md，从第一个未完成任务开始，按 AGENTS.md 执行循环协议连续推进，每轮完成后更新 docs/PROGRESS.md 和 docs/TASKS.md，全部任务完成后执行 git push origin main，中途不要停，不要询问确认"
```

### 接续上次进度
```
codex "读取 AGENTS.md 和 docs/PROGRESS.md，找到上次中断点，从 docs/TASKS.md 中第一个未完成项继续，执行到全部完成后 push"
```

### 遇到 blocker 排查
```
codex "读取 AGENTS.md 和 docs/BLOCKERS.md，针对记录的问题用 /investigate 模式排查，给出根因分析和修复方案，修复后验证通过再 commit"
```

### 收尾 QA 与发布
```
codex "读取 AGENTS.md，当前进入收尾阶段，运行完整测试套件，整理分支，执行 /ship 流程，最终 git push 并输出发布摘要"
```
