# 需求分析 · Sprint M2a（契约队列与信号快照）

**需求源**：`/Users/zuowei/workspace/go/src/github.com/newthinker/hestia/docs/superpowers/plans/2026-09-05-hestia-m2a-contract-queue.md`（只读，1772 行，7 个 Task + Global Constraints + 8 条交付前清单）
**spec**：同目录 `specs/2026-09-05-hestia-m2a-contract-queue-design.md`（14.8KB；§2.2 契约键序）
**采样锚**（本文件全部实测于此）：`d27791c695e8ebd0fd5d54c9161782f52d9b12cb`，分支 `master`，工作树干净
**目标仓库**：本仓库 atlas（模块 `github.com/newthinker/atlas`，Go 1.24.4）；hestia 仓库只是文档挂载点，不改它任何文件

## 1. 目标

每一期入权威表的观测都在 `<queue.dir>/pending/` 留下一份固定键序的契约 JSON（M3 的 warp-hestia 消费）；P2 通知带四信号与综合温度；契约只快照阈值不带信号结果；`hestia contract emit` 能为已入库期次回放契约。

## 2. 范围裁定：需求 TASK-001 ～ 007 全部进本 Sprint（AD-1）

- 007 是收口任务：需求里的「合并 master / Sprint 归档」是 Leader 动作；「code-simplifier」每个代码任务提交前各跑一次（全局规范），007 只做终检审查；CONTRACTS `## Sprint M2a` §A/§B/§C 是 007 的唯一交付物 ⇒ **docs-only**。
- 🔴 **不跑 `deploy.sh`**：投递与 M1.5 一起，等 `PENDING-ACCEPTANCE.md` 的首期验收登记。

## 3. 核心功能

| # | 功能 | 需求 Task | 包 |
|---|---|---|---|
| F1 | `Config` 加 `Queue{Dir}` 与 `Signals`（8 键）；预填、空串拒绝、两条倒置校验、`temp_scale` 钉 `"0-4"`；`configs/hestia.yaml` 两段 + `config_version` 递增 | 001 | `internal/hestia`（+ `cmd/atlas` 测试 helper，AD-5） |
| F2 | `Evaluate(obs, Signals) Temperature`：活化（剪刀差三态）/ 楼市 / 消费（月均二态）/ 信贷（票据占比三态，同口径）；缺失 `unknown` 且分母减一；`monthlyAverage` `_mom` 优先 → monthly ÷ MM → 3/6/9/12 | 002 | `internal/hestia` |
| F3 | `Contract` 结构（固定键序，`data`/`absent_fields` 按 `fieldOrder`）、`BuildContract` 纯函数、`JSON()` 逐字节确定、`FileName()` 带 `period_type`；`Store.Current` / `Store.PriorPublishedAt` 两个只读方法 | 003 | `internal/hestia` |
| F4 | `EnsureQueueDirs`（四目录幂等）、`WriteContract`（只写 `pending/`、同名覆盖、`writeAtomic`） | 004 | `internal/hestia` |
| F5 | `Ingest` 入口建齐队列；`ingestOne` 在 `Save` 入权威表且非 Duplicate 后写契约再发 P2；写失败 `stage=contract` 且 P2 不发；P2 加信号行 | 005 | `internal/hestia` |
| F6 | `atlas hestia contract emit --period --period-type --stdout`：回放契约（`Replay`、`checks: []`、`is_revision` 按库里有无更早 `published_at`） | 006 | `cmd/atlas` |
| F7 | CONTRACTS `## Sprint M2a` §A（六条契约更正）/ §B（实测登记，锚前置）/ §C（结转） | 007 | `internal/hestia/CONTRACTS.md` |

## 4. 非功能与全局约束（每个代码任务的 GATE 段逐字承载）

- 无新增依赖（`go.mod`/`go.sum` 无 diff）；`Parse`/`Validate`/`Save` 不动（四个不动文件 diff 为空；`store.go` 只新增只读方法）
- 写口守卫「登记而不是放宽」：reflect want 12 → **14**（`Current`、`PriorPublishedAt`）；AST want 25 → **31**（`BuildContract`、`EnsureQueueDirs`、`Evaluate`、`WriteContract`、`Store.Current`、`Store.PriorPublishedAt`）——需求头写「+5」是笔误，它列了 6 个名字（AD-10）
- 业务字段名字面量只在 `fields.go` 与 `_test.go`（`TestFieldNamesAppearOnlyInFieldsGo`）
- `cmd/atlas/hestia.go` 不 import `path/filepath`（既有守卫）
- gofmt 只允许 `cmd/atlas/backtest_test.go`、`cmd/atlas/crisis_test.go`（锚 `d27791c` 实测恰这两处）；vet 零输出
- 覆盖率：`internal/hestia` **≥ 96.6%**（需求硬门槛 = 基线，背对背实测 96.6）；`cmd/atlas` 不低于基线 76.4
- 注释引用任务编号写 `M2a 的 TASK-00N`；提交锚 `<type>(TASK-00N): M2a …`（AD-2）

## 5. 需求文档与仓库现状的冲突（拆分前查实，均有 AD）

| # | 需求怎么写 | 现状 | 处置 |
|---|---|---|---|
| C1 | 「002 与 003 可并行」 | 002/003/004 都要改 `store_test.go` 的守卫 want（AST 守卫是精确集合，新导出不登记就红）⇒ validator `scope-mutex` 不允许同时在途 | 串行：001→002→003→004→{005,006}→007（AD-3） |
| C2 | 005 写失败用例把 `Queue.Dir` 指向一个文件 | `Ingest` 入口先 `EnsureQueueDirs` ⇒ 在任何候选处理前就返回错误 ⇒ `countRows==1`、`newestRun` 断言必红 | 夹具改「预建 `pending/2025-12-annual.json` 为目录」让 `os.Rename` EISDIR（AD-4a） |
| C3 | 005 写失败 ⇒ `hestia_runs` 记 `ingested`、`sender.texts` 为空 | 既有循环对非 `notifyError` 一律发 P1；`runRow` 对非 `notifyError` 一律记 `failed` | 引入 `contractError`（同 `notifyError` 形态）：outcome 保持 `ingested`、`Error` 列记首行、`stage=contract`；**P1 照发**（它是失败通知，不是「说入库了」的 P2）；断言改「恰 1 条且以 `[P1]` 开头、不含信号行」（AD-4b，🔴 gate 上请人拍板） |
| C4 | 006 用例 cleanup 硬编码 `"configs/hestia.yaml"`；`RejectsBadPeriod` 不还原 `hestiaCfgPath` | 既有用例统一 `old := hestiaCfgPath; t.Cleanup(restore)` | 沿既有模式（AD-8） |
| C5 | 未提 `Queue.Dir` 为空时 `Ingest` 的行为 | `EnsureQueueDirs("")` = 在**进程 cwd**建四个目录：直建 `Config` 的 `ingest_test.go:199` 会在 `internal/hestia/` 里建 `pending/`，`cmd/atlas` 10 处 `runHestiaIngest` 用例（`writeHestiaYAMLWithDB` 无 `queue`）会建 `cmd/atlas/queue/hestia/` | `Ingest` 入口空串报错（AD-5）；`writeHestiaYAMLWithDB` 补 `queue.dir`（放 001） |
| C6 | 003 才给 `Signals` 加 json tag | tag 属于 001 的结构定义 | 移到 001（AD-6） |

## 6. 复杂度与依赖

| TASK | 复杂度 | 依赖 | 备注 |
|---|---|---|---|
| 001 | low | — | 2 包（含 cmd 测试 helper），`coverage_floor: 75` |
| 002 | medium | 001 | 纯函数；三期 golden 数值来自回填库 |
| 003 | medium | 002 | 固定键序；两个只读方法；守卫登记 |
| 004 | low | 003 | 复用 `writeAtomic` |
| 005 | high | 002, 003, 004 | 接线 + 错误分类 + P2；C2/C3/C5 三处订正 |
| 006 | medium | 003, 004 | `cmd/atlas`，`coverage_floor: 75` |
| 007 | low | 001–006 | docs-only |

**团队**：dev × 2、test × 1（链几乎全串行，只有 005 ∥ 006；第二个 dev 主要吃 006 与接手）。
