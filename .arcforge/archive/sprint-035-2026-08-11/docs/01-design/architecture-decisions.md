# 架构决策 — Sprint 035（Hestia M1b-3 validate）

本 Sprint 的技术设计由上游计划给定（见 `design-spec.md`）。本文件只记录 **Arcforge 流程层**
必须回答、而计划未覆盖的决策。

---

## AD-035-1：worktree 隔离与 dev_done 门禁的结构冲突 —— **方案 C 已获人类批准（2026-08-11）**

### 事实

`CLAUDE.md` 规定「每个 dev 在自己的 git worktree 里干活」（人类决策 2026-07-27，
立项依据是一个 Sprint 内四起同根因并发事故）。

而项目记忆记录：**worktree 隔离与 dev_done 门禁是结构冲突，已跨 Sprint 复发两次**
（sprint-031 dev 自建、sprint-034 Leader 派单要求，两次成因不同但后果相同）。

Leader 本轮直读 `.claude/hooks/task-completed.sh`（414 行）确认了机制，不依赖记忆：

- 第 3 段：`go test $PKGS -coverpkg=... -coverprofile=...` —— `$PKGS` 来自任务声明的 `packages`，
  **在调用写通道的那个工作区执行**。而写通道**禁止在 linked worktree 中调用**（主模式入口即 DENY），
  ⇒ 门禁必然在主工作区跑。
- ⇒ 若 dev 的代码只在 worktree 里，门禁测的是主工作区那份**不含新代码的旧树**。
  旧树本来就全绿、覆盖率本来就 89.4% > 80 ⇒ **门禁通过，且一行新代码都没被看过**。
- 最危险之处：它不失败，它成功。`verify_baseline.head` 随后记下一个不含交付物的 sha，
  验证者照那个 sha 去看 —— 三层全部显示「正常」。

### 裁定：方案 C（并行时隔离，串行时直连）

人类于 2026-08-11 在 DoD 确认门前批准。让每一段各自付它真正需要的成本：

| | wave 1（TASK-001/002/003，三 dev 并行） | wave 2-5（TASK-004..007，单 dev 串行） |
|---|---|---|
| 工作区 | **各自 worktree**（`git worktree add -b task/TASK-00X ../wt-TASK-00X master`） | **主工作区直接干活** |
| 收尾时序 | dev 在 worktree 提交 -> 回主仓库告知 Leader -> **Leader `git merge --ff-only`** -> dev 才 `transition dev_done` | **先 `transition dev_done`、后 `git commit`** |
| commit 锚点 | body 需加一行 `feat(TASK-00X): <描述>`（见 AD-035-2） | 不需要（未提交改动天然可见） |
| 清理 | dev 自己 `git worktree remove ../wt-TASK-00X`（交付动作的一部分） | 无 |

**wave1 为什么要隔离**：三个任务 `writes` 零重叠、`git commit` 用显式 pathspec 也不会互相扫走暂存区，
但计划要求每个 Step 都跑 `go test ./internal/hestia/` **整包** —— 三人同时在同一个包目录里新建文件时，
任何一人写到一半的文件都会让另外两人的整包测试**编译失败**，产生与自己改动无关的假红。

**wave2-5 为什么不隔离**：T4->T5->T6->T7 严格串行，同一时刻只有一个 dev 在写 `validate.go`。
隔离价值为零，却要全额付出门禁冲突的成本。

**时序不可颠倒**：wave1 的「Leader merge 在前、dev transition 在后」是这个方案唯一的安全依据。
颠倒过来门禁就在旧树上空转，而它会**成功**。

---

## AD-035-2：commit 与 transition 的先后决定漂移判定是否可见

`task-completed.sh` 的 scope 漂移集 `ACTUAL_FILES` = 未提交改动（工作树/暂存区/untracked）
∪ `COMMITTED_BOUNDED`。而 `COMMITTED_BOUNDED` 只收录**同时满足**两条的提交：

1. 提交信息中存在一行匹配 `^[a-z]+\(TASK-XXX\):`
2. 提交时间不早于 `last_transition.at`（本轮转入 `in_progress` 的时刻）

**计划要求的 commit 主题是 `feat(hestia):` / `test(hestia):`，不匹配第 1 条。**
项目记忆已实测过这一后果：整个 Sprint 的提交无一匹配，漂移判定对已提交改动完全失明。

⇒ 两条应对，按 AD-035-1 的分段选用：

- **先 transition、后 commit**（wave2-5 采用）：改动未提交 ⇒ `CHANGED_FILES` 完整可见，
  漂移判定天然有效，**不需要任何 commit 技巧**，commit 主题原样遵循计划的 `feat(hestia):` 约定。
- **先 commit、后 transition**（wave1 的 worktree 流程必然如此）：必须给门禁一个锚点。
  `git log --grep` 是**逐行**匹配整条 message（脚本注释明确说明，Leader 已复核），
  ⇒ 保持主题为 `feat(hestia): <描述>`，在 commit body 中**另起一行**写 `feat(TASK-00X): <同样描述>`。
  这样 git 历史仍符合仓库约定，门禁也能认出这是本任务的提交。

---

## AD-035-3：RED 阶段必须真的跑出预期失败，不得跳过

计划的每个 Task 都把「跑测试确认失败」写成独立 Step，并给出**预期的具体失败信息**
（如 `undefined: DefaultThresholds`、`t.validate undefined`、`findCheck` 的 `t.Fatalf`）。

`arcforge.config.json` 的 `tdd.require_failing_test_first = true`。

⇒ 每个任务的 DoD 都写入「RED 阶段实际观察到的失败信息与计划预期一致」，
且要求 dev 把**实际输出原文**（而非「符合预期」四个字）写进 discovery 的 `verification` 字段。

理由：计划自己在 T1 Step 6 就写道「少了这步，下面每个用例都可能因为别的原因报错，而测试照样绿」。
RED 若因**别的原因**失败（例如 `imported and not used`，计划的 Global Constraints 第 27 条专门警告过），
那条 RED 就没有证明任何事。**「失败了」不等于「因为预期的原因失败了」。**

---

## AD-035-4：`scope-writes-outside-packages` 告警是本仓库已知假阳，不得照它改

本 Sprint 全部 7 个任务都是标准形状：`packages: ["./internal/hestia"]`（Go 包路径，喂给 `go test -coverpkg`）
+ `writes: ["./internal/hestia/xxx.go", ...]`（具体文件路径，用于 scope 互斥与漂移）。

validator 会对在途任务报告 `scope-writes-outside-packages`。**这是形状级假阳**
（sprint-034 TASK-003 实测）：文件确实就在那个包目录下，只是没被前缀匹配上。

**按该告警的提示做会更糟**：把 `.go` 文件路径塞进 `packages`，会让非包路径被喂给
`go test -coverpkg`，产生 `no Go files ... [setup failed]` 而弄坏覆盖率门禁。

⇒ 处置：**原样放着，不动任务声明**。该规则是告警级（不影响退出码）。
本条写入 dev/test/QA 的派单 prompt，避免任何人「顺手修正」。

同理，TASK-007 的 `CONTRACTS.md` 出现在 `writes` 而**不在** `packages` —— 这是刻意的，
非 Go 路径进 `-coverpkg` 会弄坏覆盖率门禁。这正是 `writes`/`packages` 两个口径分离的正当用例。

---

## AD-035-5：不重写上游设计，但也不盲从

上游计划已含完整代码与自审。Leader **不**重新设计，也**不**允许 dev 偏离计划的实现方案自由发挥。

但这**不等于**「照抄即可、无需判断」——Leader 独立追溯已经查出计划自审的两个问题
（见 `02-plan/requirement-dod-matrix.md` 的发现 1、发现 2）：

- dev 若发现计划的代码有错（编译不过、断言与实现矛盾、行号漂移、字段常量拼写对不上），
  **必须走 `blocked_clarification` 把问题写进 `questions[]`**，不得静默「修正」计划。
- 理由：静默修复会掩盖计划自身的缺陷（项目记忆有专条）——同一缺陷被无声补三次，
  「反复发生的机制问题」就会伪装成「偶发的个人失误」而永远拿不到频次证据。
