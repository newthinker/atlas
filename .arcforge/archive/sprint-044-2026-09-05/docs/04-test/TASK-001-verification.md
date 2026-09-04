# TASK-001 验证报告 · `hestia_runs` 表、`Run`、`Store.RecordRun` / `RecentRuns`、写口守卫登记

- 验证者：test-m15-a · 2026-09-04
- 判定对象：`verify_baseline.head = 511ee4264d1df3129b986e4f8857e3284c06d754`（master，merge of dev commit `5b9859b0042fd41996882fb37cee818b365d1aa1`）；当前 HEAD 与之一致，无漂移
- discovery：`.arcforge/discoveries/TASK-001.json` sha256 `c6dae8c7beccb65727c6620025dc2a23787c9f349f5ad1e141e5303fa16e4967`（与基线一致）
- 验证环境：`git worktree add --detach ../wt-verify-TASK-001 511ee4264d1df3129b986e4f8857e3284c06d754`；变异与红阶段复现在 `mktemp -d` rsync 副本；主仓库与验证树五文件 sha256 前后一致（harness 自检 PASS）
- 基线对照锚：`037d1eb1e4f827c415319519e40f4e2208968920`（hestia 96.5%）

## 结论：**VERIFIED**

8 条 done_criteria 全部有我自跑的输出作证据；范围核对无越界；6 个变异 5 KILLED、1 SURVIVED（非 DoD 缺口，见备注）。

## 范围核对

`git diff --numstat 2db8519 511ee42`：`schema.go` 30/0、`types.go` 28/0、`store.go` 91/1、`schema_test.go` 37/2、`store_test.go` 191/3——恰为声明 `writes` 五文件；dev commit `5b9859b` numstat 同五文件（显式 pathspec）。master 上五文件 sha256 与 `5b9859b` 树逐一相同。`go.mod`/`go.sum`/`parse.go`/`extract.go`/`validate.go`/`fields.go` diff 为空。

## Done Criteria 覆盖矩阵

| # | 完成标准 | 对应测试 / 证据（均在 511ee42 上自跑） | 判定 |
|---|---|---|---|
| functional[0] | `TableRuns` 常量+注释；`runsDDL()` 两条 `IF NOT EXISTS`、13 列手写、无主键；`NewStore`/`openWithSchema`/`TestDDLIsIdempotent` 三处 `append(…, runsDDL()...)` 字面形态；幂等测试 runs 表前后 `tableInfo` 相等；`sqlite_master` 名单加 `TableRuns`；`TestRunsStructureFromLiveDB` 13 列 `ElementsMatch`、不含 `fieldOrder`、五列 `notNull` | 机械计数：列 13 / `IF NOT EXISTS` 2 / `PRIMARY` 0；三处调用点 diff 逐行核对为 DoD 给的字面形态；`TestRunsStructureFromLiveDB`、`TestDDLIsIdempotent`、`TestNewStoreCreatesSchemaIdempotently` 均 `--- PASS`；**M4**（`openWithSchema` 去掉 `runsDDL()`）⇒ `TestDDLIsIdempotent`+`TestRunsStructureFromLiveDB` 同红 KILLED（B1 修法生效） | PASS |
| functional[1] | `RunOutcome` 五常量（值 `no_new/ingested/pending/duplicate/failed`）、`validRunOutcomes`、`Run` 13 字段；`RecordRun` 在 `RecentPending` 之后：`unknown outcome %q`、`RunAt is zero`、`FinishedAt` 回落、UTC RFC3339、8 列 `nullIfEmpty`、`boolToInt`、`Milliseconds()`；三条测试 | `reflect.TypeOf(Run{}).NumField()=13`；`store.go:568 RecentPending → :610 RecordRun → :637 RecentRuns → :705 Save`；`nullIfEmpty` 调用 8 处；`TestRecordRunRoundTrip`（含子例 `FinishedAt_零值回落_RunAt`）/`TestRecordRunEmptyStringsAreNull`（NULL=8、notified=1、`NotifyError==""`）/`TestRecordRunRejectsBadInput`（两种拒绝 + `countRows==0`）均 PASS；**M3**（`nullIfEmpty` 退化为原样存）⇒ `:2829 Not equal` KILLED；**M5**（去掉回落）⇒ 子例红 KILLED | PASS |
| functional[2] | `RecentRuns`：`n<=0 ⇒ nil,nil`；`ORDER BY run_at DESC, rowid DESC LIMIT n`；NULL→空串；`notified != 0`；`duration_ms`→`Duration`；`bad run_at`/`bad finished_at`；`rows.Err()` | 代码逐行核对（COALESCE 在读侧，写侧 `nullIfEmpty` 不变）；`TestRecentRunsNewestFirst`（`[Duplicate, Ingested, NoNew]`、`RecentRuns(ctx,0)` nil）PASS；dev 补的 `TestRecentRunsRejectsCorruptTimestamps`（两例）/`TestRunMethodsPropagateDBErrors` PASS；**M2**（去掉 `notify_error` 的 COALESCE）⇒ `converting NULL to string is unsupported` 三条红 KILLED——COALESCE 改法与「NULL 读回空串」等价且被测试钉住 | PASS |
| functional[3] | 守卫 want 精确集合 12 / 24（字母序）；`TestPackageExposesNoWriteFunctions` 下方追加需求理由注释（含 ADR-0003 射程句）；`TestRecordRunTouchesOnlyRunsTable`；四守卫 `-run` 四个 PASS | 机械计数 want 12 / 24；`RecentRuns` 排在 `RecentPending` 后、`RecordRun` 前（reflect 顺序测试本身即字母序断言）；注释段 8 行与需求原文 `2026-09-04-hestia-m1.5-health.md:247` 起逐字相同（另追加 2 行 `Store.RecentRuns` 配对说明，原文完整保留）；`-run 'ExposesNoWrite|TestDDLIsIdempotent|TestRecordRunTouchesOnlyRunsTable' -v` ⇒ 四个 `--- PASS`；**M1**（Leader 指定：函数体塞 `_ = TableObservations`）⇒ `:2887 should not contain "TableObservations"` KILLED | PASS |
| boundary[0] | `store.go` 只新增：`Save` 在 +/- 行出现 0 次；`-` 行只在 `NewStore` DDL 一处；helper 非导出；按需补 `errors` | DoD 原命令输出 **0**；`-` 行恰一行 `for _, ddl := range []string{…}`；`nullIfEmpty`/`boolToInt` 小写且不在守卫名单；`errors` 已在 `store.go:7` import（无需补，discovery 有记） | PASS |
| error_handling[0] | TDD 红阶段留痕 | 副本把 `schema.go`/`types.go`/`store.go` 换回 `037d1eb` 保留新测试 ⇒ `store_test.go:136:78: undefined: TableRuns` / `:2777 undefined: RunOutcome` / `undefined: Run`；discovery `red_phase` 同形（行号差 4 是 header 注释增量） | PASS |
| non_functional[0] | 门禁 | `gofmt -l` 五包 = 恰三既有欠账；`go vet` rc=0；五包 `-count=1` 全 ok：**hestia 96.5%**（基线 96.5、硬门槛 96.3）/ metrics 98.9% / alert 92.6% / config 83.3% / cmd/atlas 76.3%；无新增依赖；四冻结文件 diff 空；注释含 `M1.5 的 TASK-001` | PASS |
| non_functional[1] | AD-6 交付流程 | 提交 `feat(TASK-001): M1.5 …` 匹配门禁 grep；merge `511ee42` 在 master；`git worktree list` 无 `wt-TASK-001-m15`（已拆）；discovery 同时写 `my_commit_sha`/`merged_master_sha`，各自证数字锚 511ee42 且与我复采一致；code-simplifier 为 dev 自述（保留 COALESCE、回退 `schemaDDL` helper——后者符合「不新增需求之外符号」），review 级不作断言 | PASS（review） |

## 变异汇总（隔离副本，被测树 511ee42）

| 变异 | 位置 | 结果 |
|---|---|---|
| M1 `RecordRun` 体内加 `_ = TableObservations` | store.go | KILLED（TouchesOnlyRunsTable） |
| M2 去 `COALESCE(notify_error,'')` | store.go | KILLED（EmptyStringsAreNull + CorruptTimestamps） |
| M3 `nullIfEmpty` 恒返回原串 | store.go | KILLED（NULL 计数 8 ≠） |
| M4 `openWithSchema` 去 `runsDDL()` | schema_test.go | KILLED（DDLIsIdempotent + RunsStructure） |
| M5 `FinishedAt` 不回落 | store.go | KILLED（RoundTrip 子例） |
| M6 `ORDER BY` 去 `rowid DESC` | store.go | **SURVIVED** |

## 备注（不影响判定）

- **M6 存活**：SQLite 走 `hestia_runs_run_at` 索引反向扫描时，同 `run_at` 的行碰巧按 rowid 逆序返回，`TestRecentRunsNewestFirst` 区分不了「显式平局键」与「引擎偶然顺序」。DoD 只要求 SQL 字面含 `rowid DESC` 且测试顺序为 `[Duplicate, Ingested, NoNew]`，两者都满足；这是测试强度的已知边界，供 TASK-007（消费 `RecentRuns` 顺序）知悉。
- 理由注释段在需求原文之外追加了两行 `Store.RecentRuns` 配对说明，原文 8 行逐字保留，不违反「原文照抄」。
- 分支 `task/TASK-001-m15` 仍在（已合入）。
- 复现命令（锚全 sha）：`git worktree add --detach <dir> 511ee4264d1df3129b986e4f8857e3284c06d754 && cd <dir> && GOTOOLCHAIN=local go test ./internal/hestia/ -run 'TestRunsStructure|TestRecordRun|TestRecentRuns|TestRunMethods|ExposesNoWrite|TestDDLIsIdempotent' -count=1 -v`
