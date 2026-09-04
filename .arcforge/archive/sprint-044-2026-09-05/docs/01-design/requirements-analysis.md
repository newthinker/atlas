# 需求分析 · Sprint M1.5（Sprint 044 · Hestia 健康度可观测）

**需求源**：`/Users/zuowei/workspace/go/src/github.com/newthinker/hestia/docs/superpowers/plans/2026-09-04-hestia-m1.5-health.md`（只读，2017 行，9 个 Task + Global Constraints + 9 条交付前清单）
**spec**：同目录 `specs/2026-09-04-hestia-m1.5-health-design.md`（306 行，§0 事实基线 · §1 决策 · §2–§7 设计 · §8 测试 · §9 风险 · §10 不做 · §11 交付判据）
**采样锚**（本文件全部实测于此）：`037d1eb1e4f827c415319519e40f4e2208968920`，分支 `master`，工作树干净
**目标仓库**：本仓库 atlas（模块 `github.com/newthinker/atlas`）；hestia 仓库只是 vault 文档的挂载点，不改它任何文件

## 1. 目标

让「Hestia 管线还活着吗、上一期怎么了」在 `serve` 的 `/metrics` 上可见，并经既有告警循环在停跑或长期无入库时发 Telegram；`hestia status` 能看到最近几次运行。

## 2. 范围裁定：本 Sprint = 需求 TASK-001 ～ TASK-008；TASK-009 结转（AD-1）

- 需求 009「投递与验收」是**人执行**的命令，且前置是「2026-08 月报首期增量验收已登记 CONTRACTS `## Sprint M1d` §G」——实测 CONTRACTS 的 M1d 段只到 §F，`PENDING-ACCEPTANCE.md` 给的窗口是 **2026-09-09 ～ 09-15**。前置在本 Sprint 内不可能成立。
- 归档时 final-report 必须把 009 列为「交付后待办」，并给出它需要的 `ANCHOR`（= 合并后的 master 全 sha）。
- 🔴 **本 Sprint 不跑 `deploy.sh`**（需求 TASK-008 Step 6 明写）。

## 3. 核心功能（本 Sprint）

| # | 功能 | 需求 Task | 包 |
|---|---|---|---|
| F1 | `hestia_runs` 表 + `Run` 类型 + `Store.RecordRun`（第二个写方法，登记而不放宽）/ `RecentRuns` | 001 | `internal/hestia` |
| F2 | `Ingest` 逐候选记一行；零行时 `no_new` 心跳；记录失败不影响已入库 | 002 | `internal/hestia` |
| F3 | `HealthSummary` 只读汇总；`duplicate` 不推进 `LastIngest` | 003 | `internal/hestia` |
| F4 | `metrics.HestiaCollector`：8 事实指标 + `collect_errors`；空表不输出时间戳；出错只计错 | 004 | `internal/metrics` |
| F5 | `alert.Rule.Cooldown` 按规则冷却，未写退回 5 分钟 | 005 | `internal/alert` |
| F6 | 主配置 `AlertRule.Cooldown` + `HestiaConfig{ConfigPath}`；`mapRules` 透传 | 005/006（拆出 **TASK-010**，AD-2） | `internal/config` + `cmd/atlas` |
| F7 | `serve` 按 `hestia.config_path` 注册 collector：未设跳过 / 装不上即启动失败 / 成功注册；样例配置两条规则；部署文档 | 006 | `cmd/atlas` + `configs` + `docs` |
| F8 | `hestia status` 的 `runs` 段（销 M1d 挂账 C2 第二半） | 007 | `internal/hestia` + `cmd/atlas` |
| F9 | 收口：采锚、全量核对、真语料回归、CONTRACTS `## Sprint M1.5` §A/§B | 008 | docs-only |

## 4. 非功能约束（Global Constraints 逐条 → 落点）

| 约束 | 实测 / 落点 |
|---|---|
| Go 1.24.4，无新增依赖 | 每个代码任务 GATE 段：`go.mod`/`go.sum` 不得出现在改动里 |
| `Parse`/`Validate`/`Save` 行为不变：四个不动文件 diff 为空；`store.go` 只新增、`Save` 函数体不动 | 001（唯一改 `store.go` 的任务）+ 008 全量核对；基线 sha `037d1eb` |
| 两条写口守卫按「登记而不是放宽」：精确集合 + `RecordRun` 登记理由注释 + `TestRecordRunTouchesOnlyRunsTable` | 001（reflect 守卫 `store_test.go:384`、AST 守卫 `:437`，既有 want 已核对）；003 再登记 `HealthSummary` |
| 业务字段名字面量只许在 `fields.go` 与 `_test.go`；`health.go` 不得引用 `fieldOrder` 里的名字 | 003：`TestFieldNamesAppearOnlyInFieldsGo` 必须仍绿 |
| `cmd/atlas/hestia.go` 不得 import `path/filepath` | 007：`TestHestiaCmdDoesNotResolveDBPath`（只解析 `hestia.go`，新建的 `hestia_health_test.go` import filepath 不受此限） |
| 注释引用任务编号带 milestone 前缀（`M1.5 的 TASK-00N`） | 每任务 |
| 每任务结束 gofmt / vet / 五包测试干净 | 🔴 **口径订正（AD-8）**：`gofmt -l internal cmd/atlas` 实测列出 **28** 个文件，需求「只有 backtest/crisis 两个」为假；收窄为五包，既有欠账 **3** 个：`internal/metrics/snapshot_test.go`、`cmd/atlas/backtest_test.go`、`cmd/atlas/crisis_test.go` |
| `internal/hestia` 覆盖率 ≥ 96.3% | 实测基线 **96.5%**（锚 `037d1eb`）；DoD 写「≥ 96.3 且不低于 96.5 减 0.2」→ 统一为 **≥ 96.3%**，报数带锚 |
| 其余四包覆盖率 | 实测：`internal/metrics` **98.9%**、`internal/alert` **92.3%**、`internal/config` **83.3%**、`cmd/atlas` **76.3%**（< `dev_minimum` 80 ⇒ AD-4 floor） |
| 工具只产出依据，不做人的判断 | 009 结转（AD-1）；样例配置由 006 改，运行时 `config.yaml` 不碰 |
| 提交前 code-simplifier | 每任务自跑（AD-11）；008 终检 |
| 测试文件 import 按需增补 | 每任务 |

## 5. 前提核验（需求文档里的「既有」断言，逐条对照 `037d1eb`）

| 需求断言 | 实测 | 结论 |
|---|---|---|
| `tableInfo`/`openWithSchema`（schema_test）、`newTestStore`/`newTestStoreAt`/`countRows`（store_test）、`sqliteDSN`（store.go） | 均存在 | ✓ |
| `annualFetcher`/`fakeSender`/`errBoom`/`ingestCfg`/`syntheticIndex`/`indexEntry`/`articleURL`/`annualID`/`annualTitle`（ingest_test）、`fakeFetcher`（discover_test）、`testIndexURL` | 均存在（`fakeFetcher` 在 `discover_test.go`，同包可用） | ✓ |
| `type Querier`（store.go）、`pendingDDL`/`currentViewDDL`（schema.go）、`notifyError`/`d.send`（ingest.go）、`renderP1`（notify.go） | 均存在；`ingestOne` 里的 `wrap` 是**函数内闭包**，`fail` 须在同一作用域定义 | ✓ |
| `bitemporal` 是否已在 ingest.go 导入 | **未导入**（需求「若尚未导入」的分支成立） | ✓ 002 须补 import |
| `RenderStatus` 既有调用点「四处」 | 实为 `status_test.go` **5 处**（:30 :64 :86 :110 :129）+ `cmd/atlas/hestia.go:348` | ⚠️ DoD 写实数 |
| `mockNotifier`/`advanceTime`/`NewEvaluator`/`e.cooldown`/`e.lastFired[rule.Name]` | 均存在（`advanceTime` 是 evaluator.go 里的非导出方法） | ✓ |
| `TestMapRules_FieldMapping` 两条规则输入 | 存在（`alert_runner_test.go:172`），r1/r2 形态与需求一致 | ✓ |
| `serve.go` `metricsReg` 段 | `serve.go:180-184`，紧接其后是 `maybeStartAlertRunner` | ✓ |
| `config.go` `Prism` 字段、`AlertRule` 五字段 | `:30`、`:273-279` | ✓ |
| `metrics.Registry` 有 `Gather`/`MustRegister`/`Snapshot` | 嵌入 `*prometheus.Registry`；`Snapshot` 对 gauge/counter **按名字跨标签求和**（snapshot.go） | ✓（`hestia_runs_total` 13 = 10+2+1 成立） |
| `hestia.LoadConfig` 最小 yaml（db_path、snapshot_dir、index_url、max_pages、timeout） | `validate()` 恰要这五个非空/正值，thresholds 由 `DefaultThresholds()` 预填 | ✓（若 `t.validate()` 对豁免另有要求，006 DoD 给了出路） |
| `docs/deployment.md` 有「服务清单」表、serve 一行 | `:252` 表头、`:256` `| serve | 常驻 | Web/API |` | ✓ |
| `configs/config.example.yaml` 有 `alerts.rules`（`:235`）、`prism:`（`:243`） | 存在；rules 目前只有 `high_error_rate` 一条 | ✓ |
| CONTRACTS M1d §D 挂账 C2 | `:3313` 一行 | ✓（007 只销第二半，落点在 009 结转） |
| 提交信息 `feat(M1.5 TASK-001):` | 门禁锚 `^[a-z]+\(TASK-001\):`（task-completed.sh:133/174）**不匹配** | ❌ AD-3 |
| 「`git status --short` 必须干净」 | sprint 进行中主仓库恒有 `?? .arcforge/tasks/` 等 | ❌ AD-7（沿 M1d） |
| 「002 与 003 可并行」 | 003 的 `TestHealthSummaryPendingReview` 断言 `RunsByOutcome[RunPending]==1`，依赖 002 已让 `Ingest` 写 runs | ❌ AD-5：003 依赖 002 |

## 6. 需求文档与本仓库机制的冲突 / 缺口（已在 AD 里裁决）

1. 提交锚格式（AD-3）。
2. 需求 005 跨三个包六个文件、006 跨两包六文件 ⇒ 超 Realistic Scope，且两者都写 `config.go`（AD-2 拆出 TASK-010）。
3. `cmd/atlas` 76.3% < 80（AD-4）。
4. gofmt 判据（AD-8）。
5. 采锚口径（AD-7）。
6. 心跳射程：Discover 失败 / `--only-period` 无候选是**循环之前**的失败，不记行——`hestia_stalled` 文案「launchd 没跑，或跑了没落库」正是为它写的（AD-13）。
7. spec §8「`RecordRun` 失败不影响已入库行」需求自己判为**不可构造**，改由代码顺序保证并在 CONTRACTS §B 登记「未测、靠顺序」（002 DoD 照此写，008 §B 落表）。

## 7. 复杂度评估

| 任务 | 复杂度 | 依据 |
|---|---|---|
| 001 | medium | 5 文件；两条守卫精确集合 + 源码级守卫；`nullIfEmpty` 语义（空串 ⇒ NULL）是 003 的前提 |
| 002 | **complex** | 改 `ingestOne` 签名与主循环、8 条测试、错误链形态（`notifyError` 穿透）、既有测试不得受影响 |
| 003 | simple | 新文件 + 4 条测试；SQL 三条 |
| 004 | medium | prometheus Collector 接口、空表省略、`Snapshot` 可见性 |
| 005 | simple | 一个字段 + 三行 |
| 010 | simple | 两个字段 + 一行透传 + 测试断言 |
| 006 | medium | 三种启动语义、`serve.go` 接线、yaml/md |
| 007 | simple | 签名加参 + 6 处调用点 + 渲染 |
| 008 | medium | docs-only，但真语料回归与全量核对 |
