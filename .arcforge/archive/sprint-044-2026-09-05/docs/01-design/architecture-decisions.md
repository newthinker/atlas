# 架构/流程决策 · Sprint M1.5（Sprint 044）

设计层决策（spec §1 D1–D6：`hestia_runs` 表形态、`RecordRun` 登记、心跳/最近入库拆开、冷却期可配、`duplicate` 不推进、告警承载是 serve 自己的循环）已由人类在 spec 定稿，此处不重开。以下是 Arcforge 拆分与执行层的裁决，每条带理由与证据（锚 `037d1eb1e4f827c415319519e40f4e2208968920`）。

## AD-1 范围 = 需求 TASK-001 ～ 008；009 结转为归档后人类清单

- **依据**：需求 009 标「🔴 人执行」；前置「首期增量验收已登记 CONTRACTS `## Sprint M1d` §G」实测不存在（M1d 段只到 §F）；`PENDING-ACCEPTANCE.md` 窗口 2026-09-09 ～ 09-15。Global Constraints「工具只产出依据，不做人的判断」。
- **代价**：归档时 M1.5 未投递。final-report 列 009 为「交付后待办」并给 `ANCHOR`。**不跑 `deploy.sh`**。

## AD-2 需求 005 拆为 TASK-005（`internal/alert`）+ TASK-010（`internal/config` 两个新键 + `cmd/atlas` `mapRules` 透传）；需求 006 的 `HestiaConfig` 移入 010

- **依据**：Realistic Scope「每任务 ≤ 1 package、≤ 5 文件」；需求 005 跨 `internal/alert`/`internal/config`/`cmd/atlas` 六文件，需求 006 六文件且与 005 都写 `config.go` ⇒ validator `scope-mutex` 不允许同时在途。
- **形态**：005 = `Rule.Cooldown` + `Evaluate` 按规则取值 + `TestEvaluator_PerRuleCooldown`（3 文件，1 包）；010 = `config.AlertRule.Cooldown` + `config.HestiaConfig{ConfigPath}` + `Config.Hestia` + `mapRules` 透传 + `TestMapRules_FieldMapping` 断言 + 一条 `config_test.go` 解码测试（4 文件，2 包）；006 = `buildHestiaHealth` + `serve.go` 接线 + yaml + md（5 文件）。
- **编号**：保持与需求一一对应，新增的取 **TASK-010**——009 空出，因为需求 009 是人执行的，占用它会让「Arcforge 009」与「需求 009」指向不同的东西。
- **依赖**：010 依赖 005（`TestMapRules_FieldMapping` 要 `alert.Rule.Cooldown`）；006 依赖 004 与 010。

## AD-3 提交信息锚 `<type>(TASK-00N): M1.5 …`（沿 M1d AD-3）

- **依据**：`.claude/hooks/task-completed.sh:133/174` 用 `git log -E --grep="^[a-z]+\(${TASK_ID}\):"` 取本任务已提交改动；需求写的 `feat(M1.5 TASK-001):` **不匹配**，`:175` 的 `NONCONFORMING` 探针会 WARN 且改动对漂移检查不可见。
- Go 注释里仍按需求写 `M1.5 的 TASK-00N`。

## AD-4 涉 `cmd/atlas` 的任务（006 / 007 / 010）带 `coverage_floor: 75`

- **依据**：`go test ./cmd/atlas/ -cover` 实测 **76.3%** < `dev_minimum` 80；门禁对 `packages` 全部包 `-coverpkg` 合并取 total（`task-completed.sh:377-385`），007 与 010 各含两个包，合并值会被 `cmd/atlas` 拉低到 80 附近；`:401` 读任务级 `coverage_floor`。
- **约束不放宽**：DoD 要求交付后各包**不低于基线**（`internal/hestia` 96.5、`internal/config` 83.3、`cmd/atlas` 76.3，均锚 `037d1eb`），floor 只是门禁形态适配。整数截断（`${TOTAL%.*}`）⇒ 写 75。

## AD-5 TASK-003 依赖 TASK-002（需求说「002 与 003 可并行」）

- **依据**：需求 003 的 `TestHealthSummaryPendingReview` 末行 `assert.Equal(t, 1, h.RunsByOutcome[RunPending])` 要 `Ingest` 已经写 `hestia_runs`——那是 002 的交付。并行时该断言必红，dev 只能删断言或走澄清环。
- **代价**：关键路径多一段（001 → 002 → 003 → 004 → 006）；wave 2 仍有 007/010 并行填充。
- **已知替代、刻意不取**（reviewer S3）：用 `Save(obs, failing())` + 手工 `RecordRun(pending)` 造夹具可让 003 脱离 002 进 wave 2。不取的理由：要保留「经真实 `Ingest` 落 pending ⇒ `PendingReview` 与 `RunsByOutcome[pending]` 同为 1」这层集成证据；且 wave 2 已有 002/007/010 三个并行任务，再加一个超出 dev × 3 的吞吐，串行不增加总时长。

## AD-6 交付协议沿 M1d AD-6：每 dev 独立 worktree；merge 先于 `dev_done`；Leader 串行 merge

- **依据**（机制）：`task-completed.sh` 的 `git log --grep` 均不带 `--all` ⇒ 未合并分支对门禁结构性不可见，它会在没有你代码的树上报绿。
- 分支 `task/<TASK-ID>-m15`（当前 `task/*` 分支 0 条）；主分支 **master**。
- 自证数字在 **merge 后的 master** 上重采；discovery 同时写「我的 commit sha」与「merge 后 master sha」。
- 语料 `data/hestia-backfill-2026-08-14` 用主仓库绝对路径（`data/` 被 `.gitignore`，worktree 里没有）。
- Leader merge 纪律（M1d 事故 5）：预演与正式 merge 放同一个 Bash，同条命令打 `rc=$?` 与 `git rev-parse HEAD`。

## AD-7 采锚口径收窄（沿 M1d AD-7）

- `git status --short -- internal cmd/atlas configs docs go.mod go.sum` 为空（需求原文「`git status --short` 必须干净」在 sprint 进行中恒不成立：`?? .arcforge/tasks/` 等）；`ANCHOR=$(git rev-parse HEAD)`；写 CONTRACTS 前 `git diff --numstat $ANCHOR HEAD -- internal cmd/atlas configs docs` 为空。

## AD-8 gofmt 判据收窄为五包；既有欠账三处

- **依据**：`gofmt -l internal cmd/atlas` 实测列出 **28** 个文件，需求「只允许 backtest_test.go / crisis_test.go」为假——那是对 `cmd/atlas` 说的，`internal` 下另有 26 处历史欠账。
- **判据**：`gofmt -l internal/hestia internal/metrics internal/alert internal/config cmd/atlas` 只允许 `internal/metrics/snapshot_test.go`、`cmd/atlas/backtest_test.go`、`cmd/atlas/crisis_test.go`（**不要顺手修**）。

## AD-9 需求文档只读；本仓库 agent 不改 hestia 仓库任何文件；`- [ ]` 不打勾

## AD-10 未核验前提写进 DoD 时必须带「若为假怎么办」

- 本 Sprint 已核验前提见 `requirements-analysis.md` §5。(a) 006 的最小 hestia.yaml 能过 `LoadConfig`——**reviewer 已核验为真**（`cmd/atlas/hestia_test.go:224-232` 同形配置通过），DoD 里保留「补缺的键、不放宽 `validate`」作备用出路；仍未跑过的：(a′) 006 `config.Load` 装载整份 `config.example.yaml`——出路：因 `hestia:`/`alerts:` 之外的段失败 ⇒ 澄清环，不改其他段；(b) 002 的 `TestIngestRecordsFailedWithStage` 里 `Parse` 错误串是否含「parse」——`wrap("parse", err)` 文案含它，成立；(c) `hestia_runs` 空表下 `hestia status` 的既有 `TestStatusOnEmptyStore` 是否仍绿——`RecentRuns` 对空表返回 nil、`runs: 0`，成立。

## AD-11 code-simplifier：每任务 dev 提交前自跑；TASK-008 只终检、不改代码（沿 M1d AD-5）

## AD-12 团队 dev × 3 + test × 1

- wave 2 有三个可并行任务（002 / 007 / 010），之后串行。1 个验证者串行验 9 个任务，积压时加第二个。

## AD-13 心跳射程：只覆盖「Discover 成功之后」

- Discover 失败、`--only-period` 无候选（`ingest.go:129-131`、`:175-179`）都在主循环之前 `return err`，**不记行**——它们已经让退出码非零、err.log 留痕；`hestia_stalled` 的文案「launchd 没跑，或跑了没落库」正是为「跑了但没到记行那一步」写的。写进 002 boundary，不扩 spec。

## AD-14 `stage` 列对 fetch 阶段取 `"fetch <URL>"`（沿既有 `wrap` 文案）

- spec §2.1 把 stage 枚举写成 `fetch | snapshot | parse | mismatch | validate | save | notify`；既有 `wrap("fetch "+c.URL, err)` 带 URL，且枚举外还有 `has article`（`ingest.go:222`）。**理由订正（reviewer S1）**：初版写「既有测试可能断言 URL」——实测 `ingest_test.go` 无任何引用 fetch 阶段 URL 文本的断言，该理由不成立；站得住的理由是需求原文就是 `fail("fetch "+c.URL)`，且 `TestIngestWrapsStageErrors`（`:455`）守着既有错误串形态，改裸 `fetch` 等于改需求。`HealthSummary`/collector 不消费 `stage`。取「stage = 传给 `fail` 的原字符串」；通知失败不走 `fail`、stage 留空只记 `notify_error`。**落点**：TASK-002 discovery `decisions` + TASK-008 §A **A7**（条目列全：`has article` / `fetch <URL>` / `snapshot` / `parse` / `mismatch` / `validate` / `save`；旁注 AD-13）。

## AD-15 reviewer 反审的处置（2026-09-04，只读子代理，先读需求与 spec 再比对）

判定 NEEDS WORK（3 阻断 + 8 建议），Leader 逐条打开文件:行号核实后**全部采纳**；完整报告见 `02-plan/dod-review.md`。

| # | 条目 | 严重度 | 处置 | 落点 |
|---|---|---|---|---|
| B1 | `openWithSchema`/`TestDDLIsIdempotent` 只建三段 DDL，runs 结构测试必红、幂等测试恒真 | **阻断** | 两处 DDL 列表加 `runsDDL()`；幂等测试加 runs 前后相等；`sqlite_master` 名单加 `TableRuns` | TASK-001 functional[0] |
| B2 | 「`RecordRun` 失败不影响已入库行无法构造」为假——同文件表级触发器手法可直接复用 | **阻断** | 必写 `TestIngestRunRecordFailureKeepsIngestedRow` + 零候选变体；008 §B 改「已测」 | TASK-002 error_handling[0]、TASK-008 functional[3] |
| B3 | 007 `writes` 漏 `cmd/atlas/hestia_test.go`，cmd 接线零测试 | **阻断** | `writes`/`estimated_files` 补；`runs: 0` / 6 行 ⇒ `runs: 5` 两条断言 | TASK-007 |
| S1 | AD-14 理由不成立；A7 漏 `has article`；AD-14 与 008 DoD 矛盾 | 建议 | AD-14 订正；A7 列全 | AD-14、TASK-008 functional[3] |
| S2 | 心跳用 `recorded == 0` 会在 `RecordRun` 失败时补假心跳 | 建议 | 改 `processed == 0` | TASK-002 functional[1] |
| S3 | AD-5 有零成本替代 | 建议 | 保留 AD-5，记明替代 | AD-5 |
| S4 | 示例 yaml 整份装载未核验；`NewNop` 断不到日志 | 建议 | 出路 + `zaptest/observer` | TASK-006 functional[1]、boundary[0] |
| S5 | `FinishedAt` 回落无测试；结构测试不钉 NOT NULL | 建议 | 子例 + 五列 `notNull` | TASK-001 functional[0]/[1] |
| S6 | 需求测试代码不过 gofmt | 建议 | DoD 提醒 | TASK-003 functional[1] |
| S7 | 出错分支应断言 `runs_total` 缺席 | 建议 | 加一行 | TASK-004 functional[1] |
| S8 | 回归标 `manual` | 建议 | 改 `review` | TASK-008 functional[2] |
| — | AD-10(a) 已核验为真 | 备注 | 撤「未跑过」 | AD-10 |

**未采纳**：无。reviewer 每条都给了文件:行号，Leader 逐一打开核实，无一为假；其中 B2 证伪的是**需求文档自己的断言**（「无法在不破坏 `Save` 的前提下构造」），我拆分时照抄了它——与 M1d 的 AD-7 同形：需求里的「不可能」也是待验前提。
