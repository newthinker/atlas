# Hestia M3 · 架构决策记录（Arcforge 承载层）

业务侧的架构决策在需求文档与其上游 spec 里，本文件只记 **Arcforge 承载层**的三条裁决
（人类 2026-09-08 确认）与它们的理由。

---

## AD-M3-1｜跨仓库任务用「atlas 交付记录文档」承载

**问题**：TASK-003…006 的代码在 loom / nanoclaw。`dev_done` 门禁 `task-completed.sh` 只查 **atlas 仓库**
的 git 变更 ⇒ 声明范围内零变更 ⇒ BLOCKED（`task-completed.sh:441` 的「无代码任务声明范围内没有任何实际变更」）。

**候选**：
- (A) atlas 建交付记录文档作为 `packages`/`writes`，DoD 全 `verify_by: manual` ← **选中**
- (B) 跨仓库任务不进状态机，Leader 直接派子代理做
- (C) loom / nanoclaw 各自 init 一套 Arcforge

**裁决 A**。理由：
- (B) 失去 DoD 逐条验证与单写者保护，而这四个任务恰是**最需要**逐条验证的（跨仓库、无 CI、人执行前置多）。
- (C) 三份真相源、三个 Leader 会话，成本远超收益；且三仓库的改动是**一次交付**，分开归档会丢失关联。
- (A) 让门禁有真实变更可查，同时把「证据在别处」这件事显式化成文档的四节结构。

**已知代价（如实记）**：门禁验的是 atlas 的那份文档，**不是** loom/nanoclaw 的测试。
真正的质量闸在 Test Agent——它必须**按文档给的锚点去目标仓库实跑**。文档写着绿而实际红，
门禁**不会**发现。这是本设计里唯一没有机制兜底的环节，故写进每个跨仓库任务的 DoD：
「验证者去目标仓库实跑并把输出贴进验证报告」。

---

## AD-M3-2｜计划的 TASK-006（集成冒烟）本 sprint 不建任务文件

**问题**：计划的 TASK-006 是「人执行」的集成冒烟，自述前置是「排在 M1.5 + M2a + M3 一次投递之后，
而那次投递又排在 2026-08 月报首期验收（09-09 ~ 09-15）之后」。今天 2026-09-08。

**候选**：
- (A) 不建任务文件，写进 final-report 结转段 ← **选中**
- (B) 建任务文件，走到 `assigned` 后转 `blocked_human`

**裁决 A**。理由：(B) 会让本 sprint 无法全部 `accepted`，`/arcforge-archive` 会被阻止；
而「等一个 7 天后才可能开始的人类动作」不是 `blocked_human` 要表达的东西——那个状态是给
「返工超限 / CONTESTED」用的，用它表达「还没到时候」会稀释它的信号。

**义务的载体**：写进 `06-acceptance/final-report.md` 的结转段 **以及** `internal/hestia/CONTRACTS.md`
的 `## Sprint M3 · §D 结转`（TASK-002 的 DoD 条目）。
⚠️ 只写 final-report 不够——归档后没人会再读它；CONTRACTS.md 是下一个 sprint 一定会读的文件。

---

## AD-M3-3｜nanoclaw 与 loom 都用独立 worktree，不碰本机工作区

**问题**：nanoclaw 本机 checkout 在 `feat/vendor-agent-reach-skill`，工作区有 2 改 2 删；
loom 工作区有 `.claude/hooks/arcforge-write.sh` 的未提交改动。计划要求 nanoclaw 从 `main` 切分支。

**候选**：
- (A) `git worktree add -b <分支> <临时目录> fork/main`，完全不碰当前 checkout ← **选中**
- (B) 按计划原话 `git stash` 后切分支
- (C) 等人类先处理

**裁决 A**。理由：(B) 的 stash 是**共享状态**——dev 中途失联时那个 stash 无人认领，且
`git stash` 不会 stash 掉已删除的已跟踪文件之外的东西，恢复语义对 agent 不直观。
(A) 与 Arcforge 既有的 worktree 隔离协议同构，零风险、可弃、谁建谁拆。

**收尾责任**：TASK-004 建 nanoclaw worktree，**TASK-006 完成后拆**（三任务串行复用同一个）；
TASK-003 的 loom worktree 由 TASK-003 自己建自己拆。dev 失联时由 Leader 在阶段边界
`git worktree list` 扫残留并 `remove --force` + `prune`。

---

## AD-M3-4｜覆盖率下限贴地，错误分支必须显式进 DoD

**不是裁决，是对既有约束的一次显式记录**（免得被当成余量）：
`internal/hestia` 当前实测 **96.6%**，而 DoD 下限也是 **96.6%**。`history.go` 的三条错误返回
（`Preceding` 两次失败 + `WriteHistory` 的 MkdirAll/writeAtomic 失败）若不测，整包覆盖率必跌。
故 TASK-001 的 `error_handling` 维度把它们写成显式条目，而不是留给「跑一下看看」。
