# 设计规格 · Sprint M1.5（Sprint 044；本 Sprint 只覆盖需求 TASK-001 ～ 008）

设计本体在 spec `2026-09-04-hestia-m1.5-health-design.md` §2–§6，需求文档给了逐步代码；此处只写 Arcforge 拆分层需要的东西：数据流、编号映射、接口交接、调度形态、门禁差异。

## 1. 数据流（改动后）

```
launchd 唤起 ingest ──Discover──▶ 候选[] ──ingestOne(每候选)──▶ runResult ──Store.RecordRun──▶ hestia_runs
                                   │ 0 候选 / 全部跳过 ──────────────▶ recordHeartbeat(no_new) ─┘
serve ──hestia.config_path──▶ LoadConfig → NewStore(同一份 db) ──▶ HestiaCollector.Collect
        每次 /metrics 抓取 ──▶ HealthSummary(ctx, st.DB()) ──▶ 8 事实指标 (+ collect_errors)
        告警循环 ──Registry.Snapshot()──▶ hestia_hours_since_last_run > 30 (cooldown 24h) ──▶ Telegram
hestia status ──Store.RecentRuns(ctx, 5)──▶ RenderStatus(..., runs) 的 `runs:` 段
```

## 2. 任务映射（需求编号 ↔ Arcforge 编号）

| 需求 | Arcforge | packages | writes | deps | wave |
|---|---|---|---|---|---|
| 001 | TASK-001 | `./internal/hestia` | schema.go · types.go · store.go · schema_test.go · store_test.go | — | 1 |
| 002 | TASK-002 | `./internal/hestia` | ingest.go · ingest_test.go | 001 | 2 |
| 003 | TASK-003 | `./internal/hestia` | health.go · health_test.go · store_test.go | 001, **002**（AD-5） | 3 |
| 004 | TASK-004 | `./internal/metrics` | hestia_collector.go · hestia_collector_test.go | 003 | 4 |
| 005（alert 部分） | TASK-005 | `./internal/alert` | rules.go · evaluator.go · evaluator_test.go | — | 1 |
| 005（config+cmd 部分）+ 006 的 `HestiaConfig` | **TASK-010**（AD-2） | `./internal/config` `./cmd/atlas` | config.go · config_test.go · alert_runner.go · alert_runner_test.go | 005 | 2 |
| 006（去掉 `HestiaConfig`） | TASK-006 | `./cmd/atlas` | hestia_health.go · hestia_health_test.go · serve.go · configs/config.example.yaml · docs/deployment.md | 004, 010 | 5 |
| 007 | TASK-007 | `./internal/hestia` `./cmd/atlas` | status.go · status_test.go · cmd/atlas/hestia.go | 001 | 2 |
| 008 | TASK-008（docs-only） | `./internal/hestia/CONTRACTS.md` | CONTRACTS.md | 001–007, 010 | 6 |
| 009 | — | — | — | 结转（AD-1） | — |

`writes` 互斥核对：wave 2 三个任务（002 / 007 / 010）文件两两不交；007 与 010 同在 `cmd/atlas` 包但文件不同，各自 worktree 开发、Leader 串行 merge。`store_test.go` 由 001 与 003 先后写，003 依赖 001 `verified`，不同时在途。

## 3. 接口交接（`context_from` 读上游 discovery 时要拿到的东西）

| 下游 | 从谁 | 要拿到 |
|---|---|---|
| 002 | 001 | `Run` 字段名与语义（空串 ⇒ NULL；`FinishedAt` 零值回落 `RunAt`）；`RecordRun` 的两条拒绝条件；`RecentRuns` 排序 |
| 003 | 001, 002 | 列名（`blocked_check`/`notify_error` 用 `IS NOT NULL` 数）；002 的 `ingested`/`pending` 判定分支（`LastIngest` 只看这两个 outcome） |
| 004 | 003 | `Health` 结构与「零值 = 表里没有」约定；map 非 nil |
| 006 | 004, 010 | `NewHestiaCollector(fetch, now)` 签名；`config.HestiaConfig.ConfigPath` 与 `Config.Hestia` 字段名 |
| 007 | 001 | `RecentRuns(ctx, n)`；`Run` 字段 |
| 010 | 005 | `alert.Rule.Cooldown` 字段名与 0 语义 |
| 008 | 全部 | 各任务 discovery 的自证数字（新增测试计数、覆盖率）与 002 的「未测、靠顺序」登记项 |

## 4. 调度形态

- `scheduling: dag`：依赖全部 `verified` 即派。
- 并行度：wave 1 = {001, 005}；wave 2 = {002, 007, 010}（三个并行，是本 Sprint 并行峰值）；之后 003 → 004 → 006 串行；008 收口。
- 团队：dev × 3（`dev-m15-a/b/c`）+ test × 1（`test-m15-a`，积压加第二个）+ qa × 1（`qa-m15`，全部 verified 后 spawn）。

## 5. 门禁形态差异（本 Sprint 特有）

| 任务 | 差异 | 原因 |
|---|---|---|
| 006 / 007 / 010 | `coverage_floor: 75` | `cmd/atlas` 基线 76.3% < 80；`-coverpkg` 合并后总数会被它拉低（AD-4） |
| 008 | docs-only：`packages`/`writes` 都指向 `CONTRACTS.md`，DoD 全部对象形态 `verify_by: review|manual` | 无代码任务声明（CLAUDE.md） |
| 全部 | 提交锚 `<type>(TASK-00N): M1.5 …` | AD-3 |
| 全部 | gofmt 判据五包、三处既有欠账 | AD-8 |

## 6. 结转（不在本 Sprint，归档时写进 final-report「交付后待办」）

- 需求 009 投递与验收：前置 M1d §G（首期验收，窗口 2026-09-09 ～ 09-15）；需要的 `ANCHOR` = 本 Sprint 合并后的 master 全 sha；含运行时 `config.yaml` 两条规则、三条验收、CONTRACTS §C/§D、M1d §D 挂账 C2 第二半销账、vault 回写。
- 🔴 本 Sprint **不跑 `deploy.sh`**。
