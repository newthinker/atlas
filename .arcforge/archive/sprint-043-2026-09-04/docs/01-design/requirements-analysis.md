# 需求分析 · Sprint M1d（Hestia 运行时切换、通知与增量验收 · M1 收口）

**需求源**（只读输入，不在本仓库）：
`/Users/zuowei/workspace/go/src/github.com/newthinker/hestia/docs/superpowers/plans/2026-09-03-hestia-m1d-cutover.md`（1764 行，11 个 Task + Global Constraints + 交付前清单）
**设计规格**：同目录 `../specs/2026-09-03-hestia-m1d-cutover-design.md`
**采样锚**（本文档全部实测数字所在的树）：`ae088eb253b64b36e10558a02587e3fa657f5f3e`（master）
**分析日期**：2026-09-03

## 1. 目标

让 launchd 真正跑上 M1c 的成果：ingest 抓到的原文落盘、三类 Telegram 消息、`--only-period`，
然后按人执行的清单一次切换运行时目录，用 2026-08 月报做第一期真实增量验收，关闭 M1。

## 2. 范围裁定：本 Sprint（043）= 需求 TASK-001 ～ TASK-008

| 需求 Task | 性质 | 本 Sprint | 理由 |
|---|---|---|---|
| 001–007 | 代码 | ✅ 进 | agent 可 TDD、可门禁、可验证 |
| 008 收口（采锚、全量核对、CONTRACTS §A/§B、code-simplifier） | 代码 + 文档 | ✅ 进 | 需求原文：「Sprint 043 合并 master」「合并与归档走 Arcforge 既有流程」 |
| 009 运行时切换 | **人执行** | ❌ 结转 | Global Constraints 明写「agent 不 `rm` 运行时库、不 `launchctl bootout`」；且它依赖「TASK-008 合并后的 master sha」——那是本 Sprint **归档之后**才存在的锚 |
| 010 首期增量验收 | **时间门控** 2026-09-09 ～ 09-15 | ❌ 结转 | 央行发布日约束，本 Sprint 无法完成 |
| 011 文档回写与语料副本 | 文档 | ❌ 结转 | §C/§D 的结果是它的输入；语料副本落 vault（另一个 git 仓库），不在本仓库 `writes` 射程 |

⚠️ 这三条**不是被砍掉**，是**归档后的人类清单**。本 Sprint 的 `06-acceptance/final-report.md` 必须把它们列为「交付后待办」，并给出 TASK-009 需要的锚（合并后 master 全 sha）。

## 3. 核心功能（本 Sprint）

| # | 功能 | 需求 Task | 落点 |
|---|---|---|---|
| F1 | 配置 `storage.snapshot_dir`：预填默认 `data/hestia-snapshots`、显式空串拒绝、仓库 yaml 显式写出并递增 `config_version` | 001 | `internal/hestia/config.go`、`configs/hestia.yaml` |
| F2 | `saveSnapshot` 三态幂等：不存在写入 / 同字节跳过不改 mtime / 不同字节另存带 UTC 时间戳文件不覆盖；临时文件 + rename | 002 | `internal/hestia/snapshot.go`（新） |
| F3 | `ingestOne` 在 Fetch 之后、Parse 之前落盘快照；写盘失败 ⇒ 该期失败 `wrap("snapshot", err)`；Diverged 打一行 | 003 | `internal/hestia/ingest.go` |
| F4 | `Sender` 窄接口 + `renderP0/P1/P2` 纯函数渲染 | 004 | `internal/hestia/notify.go`（新） |
| F5 | ingest 接通知：pending ⇒ P0，权威表 ⇒ P2（任何 Verdict），失败 ⇒ P1，空跑 0 条；发送失败并进错误链且不级联 | 005 | `internal/hestia/ingest.go` |
| F6 | `OnlyPeriod`：只与 Force 同用，Discover 之后过滤，0 匹配响亮失败 | 006（包级） | `internal/hestia/ingest.go` |
| F7 | cmd 层：`--only-period` flag + 开库前校验；`buildHestiaSender` 照 `buildCrisisSender`；打印通道状态；plist 传 `--config` | 006（cmd 部分）+ 007 | `cmd/atlas/hestia.go`、plist |
| F8 | 收口：采锚 → 全量核对 → 真语料回归数字一个不变 → CONTRACTS 新开 `## Sprint M1d` §A/§B | 008 | `internal/hestia/CONTRACTS.md` |

## 4. 非功能约束（Global Constraints 逐条 → 落点）

| 约束 | 落点 |
|---|---|
| Go 1.24.4，**无新增依赖** | 每个任务 `non_functional` |
| `Parse`/`Validate`/`Store` 不改：`store.go`/`validate.go`/`parse.go`/`extract.go`/`fields.go` diff 为空 | 每个任务 `non_functional`；TASK-008 全量核对 |
| 业务字段名字面量只许在 `fields.go` 与 `_test.go`（`TestFieldNamesAppearOnlyInFieldsGo`） | TASK-004（`notify.go` 一律用 `Field*` 常量） |
| 导出面精确相等（`TestPackageExposesNoWriteFunctions`），本迭代零新增导出函数 | TASK-002 / 004 / 005 / 006 |
| `cmd/atlas/hestia.go` 不得 import `path/filepath` | TASK-007 |
| `Meta` 七字段不动 | 本迭代不碰 `types.go`，写进 TASK-004 的注意项 |
| 注释里任务编号带 milestone 前缀（`M1d 的 TASK-001`） | 每个任务 |
| 每 task 结束 gofmt / vet / 两包测试干净；`backtest_test.go`、`crisis_test.go` 是既有欠账**不要顺手修** | 每个任务 |
| 覆盖率 `internal/hestia` ≥ **96.3%** | 每个 `internal/hestia` 任务 + TASK-008 |
| 工具只产出依据，不做人的判断 | TASK-009/010 结转为人执行 |
| 提交前跑 `code-simplifier`（全局规范） | 每个任务 dev 自跑；TASK-008 做终检 |

## 5. 前提核验（需求文档里的断言，逐条对照 `ae088eb`）

| 需求断言 | 实测 | 结论 |
|---|---|---|
| `ingestCfg()` 调用点「实测 24 处」 | `ingest_test.go` 21 + `discover_test.go` 3 = **24** | ✓ |
| `hestiaBackfillFromRE`、`buildCrisisSender`、`loadConfigOrDefaults`、`hestiaForce`、`openHestia`、`runHestiaIngest` 存在 | 全部存在（`hestia.go:297` / `crisis.go:425` / `export_signals.go:109` …） | ✓ |
| 五个 `Field*` 常量、`CheckFailed`、`Check.Value *float64`、`Outcome{Verdict,Table}`、`bitemporal.Duplicate.String()` | 全部存在 | ✓ |
| 三个守卫测试存在 | `store_test.go:399` / `fields_test.go:210` / `hestia_test.go:112` | ✓ |
| `errBoom`、`countRows`、`newTestStore`、`Store.HasPeriod`、`outPeriods`、`h1ID/h1File`、`DepositSumTolerance` | 全部存在 | ✓ |
| 覆盖率 96.3% | `go test ./internal/hestia/ -cover` = **96.3%** | ✓ |
| 语料 218 篇 + `manifest.json` | **218**，manifest 存在 | ✓ |
| M1c-4 锚 `4916106` 在 master 祖先链 | 是 | ✓ |
| plist 现有 `ProgramArguments` 无 `--config` | 确认：只有 `--hestia-config` 对 | ✓（改动前提成立） |
| **未核验**：`TestIngestNotifiesP0OnPending` 用 `DepositSumTolerance=1e-9` 就能在 2025 年报夹具上让 `deposit_sum` 判 failed | 未跑 | ⚠️ 写进 TASK-005 DoD：若该闸在夹具上是 skipped，允许换阈值，判据是「恰一条 P0」不是闸名 |

## 6. 需求文档与本仓库机制的冲突 / 缺口（已在 AD 里裁决）

1. 🔴 **提交信息格式**：需求写 `feat(M1d TASK-001): …`，而 `task-completed.sh` 只认 `^[a-z]+\(TASK-001\):` ⇒ 照抄会在 `dev_done` 门禁被 BLOCKED。⇒ 改为 `feat(TASK-001): M1d …`（AD-3）。
2. 🔴 **`cmd/atlas` 包覆盖率 75.7% < `dev_minimum` 80**：cmd 层任务若不带任务级 `coverage_floor` 必被 DENY（AD-4）。
3. **需求 TASK-006 横跨两个包**（`internal/hestia` + `cmd/atlas`），且与 TASK-007 同写 `hestia.go` ⇒ 违反 Realistic Scope，拆成包级 006 + cmd 层 007（AD-2）。
4. **`ingest.go` 被 003/005/006 三个任务串行改写** ⇒ 必须串行（同一 dev 承接），不能并行。
5. **code-simplifier 时机**：需求只在 TASK-008 跑一次，全局规范要求每次提交前跑 ⇒ 每任务 dev 自跑，008 终检（AD-5）。
6. **需求文档在 hestia 仓库**：本仓库任何 agent 不改它（checkbox 不打勾）；dev/verifier 按绝对路径只读。
7. **`data/` 被 `.gitignore`** ⇒ linked worktree 里没有语料目录，TASK-008 真语料回归必须用主仓库绝对路径（沿 M1c-4 AD-3）。

## 7. 复杂度评估

| 任务 | 复杂度 | 依据 |
|---|---|---|
| 001 / 002 / 004 | simple | 需求给了完整代码与测试，无并发依赖 |
| 003 / 005 / 006 | medium | 改共享的 `ingest.go`，要与既有 24 条用例共存 |
| 007 | medium | 两个文件 + plist；cobra 校验顺序是测试要抓的点 |
| 008 | medium | 数字采样纪律重，且是 docs-only（门禁形态不同） |
