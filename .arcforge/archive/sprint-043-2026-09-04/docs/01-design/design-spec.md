# 设计规格 · Sprint M1d（本 Sprint 只覆盖需求 TASK-001 ～ 008）

设计本体已由人类评审定稿：`/Users/zuowei/workspace/go/src/github.com/newthinker/hestia/docs/superpowers/specs/2026-09-03-hestia-m1d-cutover-design.md`。
本文件**不复述**它，只记 Arcforge 层面的拆分、接口交接与调度形态。需求文档里每个 Task 都带完整代码与测试，dev 以需求原文为准，本文件与 DoD 只写**偏离原文的裁决**与**原文没写的机制约束**。

## 1. 单期数据流（改动后）

```
Discover → [OnlyPeriod 过滤] → ingestOne:
  Fetch → saveSnapshot(快照) → Parse → 期次一致性 → Validate → Save → Out 打印 → send(P0|P2)
                                                                     失败 ⇒ 循环里 send(P1)（notifyError 除外）
```

`Parse` / `Validate` / `Store` 一行不动。

## 2. 任务映射（需求编号 ↔ Arcforge 编号）

| Arcforge | 需求 Task | 标题 | packages | writes | deps | wave |
|---|---|---|---|---|---|---|
| TASK-001 | 001 | 配置 `storage.snapshot_dir` | `./internal/hestia` | `config.go`、`config_test.go`、`configs/hestia.yaml` | — | 1 |
| TASK-002 | 002 | `saveSnapshot` 与幂等规则 | `./internal/hestia` | `snapshot.go`、`snapshot_test.go` | — | 1 |
| TASK-003 | 003 | ingest 接快照（Parse 之前） | `./internal/hestia` | `ingest.go`、`ingest_test.go`、`discover_test.go` | 001, 002 | 2 |
| TASK-004 | 004 | `Sender` 接口与三类消息渲染 | `./internal/hestia` | `notify.go`、`notify_test.go` | — | 1 |
| TASK-005 | 005 | ingest 接通知与错误语义 | `./internal/hestia` | `ingest.go`、`ingest_test.go` | 003, 004 | 3 |
| TASK-006 | 006（**包级部分**） | `IngestDeps.OnlyPeriod` 过滤 | `./internal/hestia` | `ingest.go`、`ingest_test.go` | 005 | 4 |
| TASK-007 | 006（**cmd 部分**）+ 007 | cmd 层：`--only-period` flag、`buildHestiaSender`、plist `--config` | `./cmd/atlas` | `cmd/atlas/hestia.go`、`hestia_test.go`、`deploy/launchd/…hestia-ingest.plist` | 004, 006 | 5 |
| TASK-008 | 008 | 收口：采锚、全量核对、真语料回归、CONTRACTS §A/§B、code-simplifier 终检 | `./internal/hestia/CONTRACTS.md`（docs-only） | `CONTRACTS.md` | 001–007 | 6 |

编号刻意与需求文档**一一对应**（只有 006 被拆、其 cmd 部分并入 007），dev 读需求时不必换算。

## 3. 接口交接（`context_from` 读上游 discovery 时要拿到的东西）

| 上游 | 暴露给下游的接口 | 下游 |
|---|---|---|
| 001 | `Cfg.Storage.SnapshotDir`（非空保证由 `LoadConfig` 给；测试里手工构造 `Config` 时**必须自己填**临时目录） | 003 |
| 002 | `saveSnapshot(dir, articleID string, raw []byte, now time.Time) (snapshotResult, error)`；`snapshotWritten/Unchanged/Diverged` | 003 |
| 003 | `ingestCfg(t *testing.T)`（带 `t.TempDir()`）——**005/006 的新测试全部要用这个签名** | 005, 006 |
| 004 | `type Sender interface{ SendText(string) error }`；`renderP0(obs, rep)`、`renderP1(c, err)`、`renderP2(obs, out)` | 005, 007 |
| 005 | `IngestDeps.Notify Sender`（nil = 不发）；`notifyError`；`fakeSender` 测试桩 | 006, 007 |
| 006 | `IngestDeps.OnlyPeriod string`；`twoEntryFetcher(t)` 测试桩；错误文案 `OnlyPeriod requires Force` / `no candidate for period` | 007 |
| 007 | `buildHestiaSender() hestia.Sender`；`hestiaOnlyPeriod` 变量；plist 带 `--config` | 008、TASK-009（人执行） |

## 4. 调度形态

- `scheduling: dag`：依赖全部 `verified` 即派发。wave 1 三个任务（001 / 002 / 004）并行；之后 003 → 005 → 006 → 007 → 008 **串行**（003/005/006 同写 `ingest.go`，007 依赖 006 的字段，008 依赖全部）。
- **同一 dev 承接 003 → 005 → 006**：连续改同一文件，换人只会多付一次上下文重建的代价。
- 团队：dev × 3（`dev-m1d-a/b/c`）+ test × 1（`test-m1d-a`）；验证积压时再加 `test-m1d-b`。
- 每 dev 独立 worktree `../wt-<TASK>-m1d`，分支 `task/<TASK>-m1d`；**Leader 串行 merge，merge 先于 `dev_done`**（AD-6）。

## 5. 门禁形态差异

| 任务 | 门禁 | 备注 |
|---|---|---|
| 001–006 | `internal/hestia` 覆盖率 ≥ 80（全局）；DoD 另要求 ≥ 96.3% | 门禁量的是 task-scope `-coverpkg=./internal/hestia`，DoD 的 96.3 由验证者核 |
| 007 | `cmd/atlas` 任务级 `coverage_floor: 75` | 该包当前 75.7%（锚 `ae088eb`），低于全局 80；DoD 要求交付后不低于 75.7 |
| 008 | docs-only：全部 DoD 为对象且 `verify_by ∈ {review, manual}`；`packages` 指向 `CONTRACTS.md` | 门禁跳过 Go 段但仍做 scope 漂移校验；本任务**不得改 `.go` 文件** |

## 6. 结转（不在本 Sprint，归档时写进 final-report「交付后待办」）

- 需求 TASK-009 运行时切换（人执行，七步，需要合并后 master 全 sha 作 `ANCHOR`）
- 需求 TASK-010 首期增量验收（2026-09-09 ～ 09-15 时间门控）
- 需求 TASK-011 vault 回写 + 语料副本 + CONTRACTS §C–§F
