# 架构/流程决策 · Sprint M1d

设计层决策（D1–D4）已在 spec §1 由人类定稿，此处不重开。以下是 Arcforge 拆分与执行层的裁决，每条带理由与证据。

## AD-1 范围 = 需求 TASK-001 ～ 008；009 / 010 / 011 结转为归档后人类清单

- **依据**：Global Constraints「工具只产出依据，不做人的判断：切换（009）与验收（010）是人执行的命令；agent 不 `rm` 运行时库、不 `launchctl bootout`」；010 时间门控到 9 月 9–15 日；011 的输入是 009/010 的结果且落点在 vault 仓库。
- **代价**：本 Sprint 归档时 M1 尚未关闭。final-report 必须把三条列为「交付后待办」并给出 009 需要的锚。

## AD-2 需求 TASK-006 拆成「包级 006」+「cmd 层并入 007」

- **依据**：Realistic Scope「每任务 ≤ 1 个 package」；需求 006 与 007 都写 `cmd/atlas/hestia.go` 与 `hestia_test.go`，validator 的 `scope-mutex` 不允许两者同时在途。
- **形态**：006 = `IngestDeps.OnlyPeriod` + 三条包级测试；007 = flag、开库前校验、`buildHestiaSender`、通道状态打印、plist、四条 cmd 测试。
- **副作用**：需求文档的 `TestHestiaFlags` 两条断言与 `TestHestiaOnlyPeriodValidation` 归 007。

## AD-3 提交信息锚改为 `<type>(TASK-00N): M1d …`

- **依据**：`.claude/hooks/task-completed.sh:133` 用 `git log -E --grep="^[a-z]+\(${TASK_ID}\):"` 取「本任务已提交改动」；需求写的 `feat(M1d TASK-001):` **不匹配**该锚 ⇒ 门禁把 `COMMITTED_MINE` 判空。
- **同时满足**：`.claude/CLAUDE.md`「提交：`<type>(TASK-XXX): <description>`」。milestone 前缀放到冒号之后（`feat(TASK-001): M1d storage.snapshot_dir …`）。
- **注释里**仍按需求写 `M1d 的 TASK-001`（那是 Go 注释不是 commit 锚）。

## AD-4 TASK-007 带任务级 `coverage_floor: 75`

- **依据**：`go test ./cmd/atlas/ -cover` 实测 **75.7%**（锚 `ae088eb`）< `dev_minimum` 80 ⇒ 不设则 `dev_done` 必 DENY（`task-completed.sh:399-409` 读 `.coverage_floor`）。
- **约束**：DoD 仍要求交付后覆盖率**不低于 75.7%**（新增代码带测试只会抬高）；floor 是门禁形态适配，不是放宽。
- **不把 floor 设成 75.7**：门禁比较用 `${TOTAL%.*}`（整数截断），75.7 会被读成 75；写 75 与其一致、不制造假精度。

## AD-5 code-simplifier：每任务 dev 提交前自跑；TASK-008 只做终检、不改代码

- **依据**：全局 CLAUDE.md「提交前必须先运行 code-simplifier」是每次提交的义务，不是 sprint 末一次；M1c-4 各次 merge 未跑、留到 QA 才补，本 Sprint 不重复。
- **008 的形态**：docs-only 任务无权改 `.go`。终检若提出改动 ⇒ 写 discovery + `blocked_clarification`，由 Leader 决定是否开一个小代码任务。预期为空（每任务已跑过）。

## AD-6 交付协议沿 M1c-4：每 dev 独立 worktree；**merge 先于 `dev_done`**；Leader 串行 merge

- **依据**（机制，不是偏好）：`task-completed.sh` 的 5 处 `git log --grep` 均不带 `--all` ⇒ 只走 HEAD 祖先链 ⇒ 未合并分支的 commit 对门禁**结构性不可见**，门禁会在没有你代码的树上报绿。
- 分支名 `task/<TASK-ID>-m1d`（当前 `task/*` 分支为 0，后缀是为跨 sprint 不撞名的惯例）。
- 一切 `.arcforge/` 读写 cd 回主仓库（linked worktree 内调写通道会被 DENY）。
- 自证数字在 **merge 后的 master** 上重采，discovery 同时写「我的 commit sha」与「merge 后 master sha」。
- 语料路径用主仓库绝对路径（`data/` 被 `.gitignore`，worktree 里没有）。

## AD-7 自证数字采样锚前置（沿 M1c-4，写进 TASK-008 的第一条 DoD）——**判据口径已订正**

- 先 `git status --short -- internal/hestia cmd/atlas configs deploy go.mod go.sum` 为空、`ANCHOR=$(git rev-parse HEAD)`，之后每个数字标锚；写 CONTRACTS 前再核 `git diff --numstat $ANCHOR HEAD -- internal/hestia cmd/atlas configs deploy` 为空。
- 🔴 **订正记录**：初版照抄需求「`git status --short` 必须干净」，reviewer 判为**阻断**、Leader 核实成立——主仓库整个 sprint 都有 `?? .arcforge/tasks/`、`?? .arcforge/docs/` 与两处会话外的运行时同步改动，该判据在本仓库**恒不成立**，dev 只能违反 DoD 或把收口卡死。收窄为代码范围后，要保证的性质「代码范围内无未提交改动」不变。这是需求文档的上游笔误（它把「工作区干净」当成了单人仓库的常态）。

## AD-8 团队 dev × 3 + test × 1

- wave 1 有三个可并行任务，之后全部串行。3 个 dev 在 wave 1 之后只剩 1–2 个在忙，idle 实例成本低；1 个验证者串行验 8 个任务，积压时加第二个。

## AD-9 需求文档只读；本仓库 agent 不改 hestia 仓库任何文件

- 需求文档是 superpowers 格式（带 `- [ ]` checkbox），本流程**不打勾**。进度真相源是 `.arcforge/tasks/*.json`。

## AD-10 未核验前提写进 DoD 时必须带「若为假怎么办」

- `TestIngestNotifiesP0OnPending` 依赖「`DepositSumTolerance=1e-9` 在 2025 年报夹具上能让 `deposit_sum` 判 failed」——Leader 拆分时未跑；**reviewer 随后核实该前提已被既有测试证实**（`ingest_test.go:318-330` 用同样阈值 + `annualFetcher` 断言落 `TablePending`，全绿），DoD 已改写、备用出路保留。原则不变：DoD 写明：若该闸在夹具上 skipped，可换任何能造出一道 failed 闸的阈值，判据是「恰一条 P0 且 pending=1」，换法记 discovery。
- 理由：M1c-4 有 12 条 DoD 断言被实测证否、9 条由 dev 先发现；把「若为假」的出路写进去，dev 不必走澄清环。

## AD-11 reviewer 反审的处置（2026-09-03，独立 reviewer `dod-reviewer-m1d`，只读需求与 spec 后比对）

判定 NEEDS WORK（1 阻断 + 若干建议），全部核实后处置如下；完整报告与逐条处置见 `02-plan/dod-review.md`。

| # | 条目 | 严重度 | 处置 | 落点 |
|---|---|---|---|---|
| R-008 | TASK-008「`git status --short` 必须干净」在本仓库恒不成立 | **阻断** | 收窄为代码范围口径 | TASK-008 functional[0]、AD-7 |
| R-003 | 第 25 处手写 `Config{}`（`ingest_test.go:178`）接线后必红 | 建议（会被红暴露，但 DoD 数字误导） | 点名修法、不改断言；补 diverged 正向测试 | TASK-003 functional[0]/[2] |
| R-005a | Duplicate 路径 ingest 层无「会调 send」的断言 | 建议 | 扩既有 `TestForceOnObservedPeriodIsDuplicate` | TASK-005 functional[0] |
| R-005b | P1 自身发送失败分支无测试 | 建议 | 新增用例 | TASK-005 error_handling[0] |
| R-007a/b | `buildHestiaSender` 无测试；「不低于 75.7」与 floor 75 会扯皮 | 建议 | 两条测试；判据改「≥ floor 且新增代码有测试」 | TASK-007 functional[1]、non_functional[0] |
| R-001 / R-002 / R-004 / R-006 | yaml 行无守卫 / 写盘失败分支无测试 / ytd+mom 同在 / 用 `calls` 钉「未发请求」 | 建议 | 全部采纳 | 各任务 |
| — | AD-10「未核验」可撤 | 备注 | 已撤，前提由既有测试证实 | TASK-005、AD-10 |
| — | TDD 红阶段输出只能靠 dev 自述 | 备注 | 保留为弱证据，不阻断 | — |

**未采纳**：无。reviewer 每条都给了文件:行号，Leader 逐条打开核实，无一为假。
