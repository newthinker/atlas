# TASK-003 验证报告 · `HealthSummary` 只读汇总——`duplicate` 不推进最近入库

- 验证者：test-m15-a · 2026-09-04
- 判定对象：`verify_baseline.head = 321dfb6bba5f412ad7a300a27e7b6c3a6e20efcd`（master，merge of commit `266fe40647aa13f9075b0850bbea80bbb9c70429`，由 dev-m15-b 提交、实现出自 dev-m15-a）；当前 HEAD 与之一致，无漂移
- discovery：`.arcforge/discoveries/TASK-003.json` sha256 `2af75b6ef220ad9e95fd700e8e40412e3d223e50027da121582abc185c411659`（与基线一致）；`assignment_epoch = 2`
- 验证环境：`git worktree add --detach ../wt-verify-TASK-003 321dfb6bba5f412ad7a300a27e7b6c3a6e20efcd`；变异与红阶段复现在 `mktemp -d` + `git archive` 副本；主仓库与验证树三文件 sha256 前后一致（harness 自检 PASS）
- 隔离锚：`e2d1f2be25e6c7f13a6761e6290a654c95dfd529`；覆盖率基线锚 `037d1eb`（hestia 96.5%）

## 结论：**VERIFIED**

7 条 done_criteria 全部有我自跑的输出作证据；范围核对无越界；6 个变异全部 KILLED（含 DoD/Leader 指定的两个）；provenance 如实。

## 范围核对

`git show --numstat 266fe40`：`internal/hestia/health.go` 88/0、`health_test.go` 175/0、`store_test.go` 7/1——恰为声明 `writes` 三文件（显式 pathspec）。`go.mod`/`go.sum`/四冻结文件 diff 为空。

## Provenance 核实（Leader 要求）

| 文件 | `266fe40` 树 | master `321dfb6` | dev-m15-a worktree 未提交副本 | discovery 记的指纹 |
|---|---|---|---|---|
| health.go | `1416ca25…cb56e` | 同 | 同 | 同 |
| health_test.go | `60205921…3e02ea` | 同 | 同 | 同 |
| store_test.go | `bda40768…e7fd3` | 同 | 同 | 同 |

三处逐字节一致、与 discovery `provenance.my_code_changes` 记的三个 sha256 逐一相同 ⇒「代码改动为零、实现出自 dev-m15-a」如实。提交信息正文亦写明出处。判定只看代码与我自己跑的输出，与作者无关。

## Done Criteria 覆盖矩阵

| # | 完成标准 | 对应测试 / 证据（均在 321dfb6 上自跑） | 判定 |
|---|---|---|---|
| functional[0] | `Health{LastRun, LastIngest; RunsByOutcome; BlockedByCheck; PendingReview, NotifyFailures}`（零值 = 表里没有、不接收 now）；`HealthSummary(ctx, Querier)`：两 map 非 nil；`LastRun = MAX(run_at)`、`LastIngest = MAX(CASE WHEN outcome IN (ingested, pending) THEN run_at END)` 一条 SQL 参数化；`GROUP BY outcome`；`BlockedByCheck` 只数 `outcome = pending AND blocked_check IS NOT NULL`；`NotifyFailures` 数 `notify_error IS NOT NULL`；`PendingReview = count(*) FROM hestia_pending`；每条错误带 `hestia health: <步骤>:`；`rows.Scan` 出错先 `rows.Close()` | `health.go` 全文逐条核对全部成立（`:25` 两 map 预建；`:28-30` 一条 SQL 两参数；`:44` GROUP BY；`:56-58` pending 过滤；`:73-74` notify；`:77` pending 表；`:50/:66` Scan 出错 `rows.Close()` 后返回）；`TestHealthSummaryEmpty`/`Timestamps`/`Counts`/`PendingReview` PASS；**M1**（`CASE WHEN … OR 1=1`）⇒ `Timestamps :59` + `RejectsCorruptRunAt/LastIngest_坏` KILLED；**M3**（`BlockedByCheck` 留 nil）⇒ `Empty :42` + `Counts` KILLED；**M4**（notify 去过滤）⇒ `Counts :87` KILLED；**M6**（前缀写错）⇒ `PropagatesQueryErrors/query_blocked_by_check :164` KILLED | PASS |
| functional[1] | 四条测试通过；S6：`p1 := …; p1.BlockedCheck = …` 先 gofmt | 四条 `--- PASS`（Timestamps 断言 LastRun=16 日心跳、LastIngest=12 日 pending；Counts 断言 `{pending:3, failed:1, ingested:1, no_new:2}`、`{deposit_sum:2, stock_continuity:1}`、NotifyFailures 2；PendingReview 经真实 `Ingest` 得 1/1）；`health_test.go:66-67` 已拆成两行；`gofmt -l` 五包只列三既有欠账 | PASS |
| functional[2] | AST 守卫 want 在 `"DefaultThresholds"` 与 `"Discover"` 之间插 `"HealthSummary"`，精确 **25** 项；reflect 守卫不改；`TestFieldNamesAppearOnlyInFieldsGo` 仍绿 | 机械计数：AST want **25**、reflect want **12**（不变）；diff 显示插入位置恰在 `DefaultThresholds` 与 `Discover` 之间，并追加「为什么名单里多了 HealthSummary」注释段；`TestPackageExposesNoWriteFunctions`、`TestStoreExposesNoWriteMethods`、`TestFieldNamesAppearOnlyInFieldsGo` 均 `--- PASS`（`health.go` 只出现 `run_at`/`outcome`/`blocked_check`/`notify_error` 列名） | PASS |
| boundary[0] | `parseNullTime`：NULL ⇒ 零值 nil 错；非法串 ⇒ `hestia health: bad run_at:`；变异 M1（CASE WHEN 放宽 ⇒ Timestamps 红）、M2（去 `outcome = ?` ⇒ Counts 红，靠 failed 夹具 `BlockedCheck = "x"`） | `TestHealthSummaryRejectsCorruptRunAt/{LastRun 坏, LastIngest 坏}` PASS；**M5**（`parseNullTime` 吞错返回零值）⇒ 两子例同红 KILLED；**M1** KILLED（见上）；**M2**（`WHERE outcome = ?` → `WHERE (? IS NOT NULL)`）⇒ `Counts :86` KILLED——`health_test.go:76` 的 `f.BlockedCheck = "x"` 那行在（带注释说明），走的是 DoD 推荐路径 | PASS |
| error_handling[0] | 红阶段 `undefined: HealthSummary` | 副本移走 `health.go` ⇒ `health_test.go:37:12: undefined: HealthSummary`（`:56/:83/:98` 同）；discovery `red_phase` 同文案 | PASS |
| non_functional[0] | 门禁 | `gofmt -l` 五包 = 恰三既有欠账；`go vet` rc=0；五包 `-count=1` 全 ok：**hestia 96.6%**（基线 96.5、硬门槛 96.3；Leader 预演 96.6 一致）/ metrics 98.9% / alert 92.6% / config 83.3% / cmd/atlas 76.3%；无新增依赖；四冻结文件 diff 空；`M1.5 的 TASK-003` 注释三文件均有 | PASS |
| non_functional[1] | AD-6 交付流程 | 提交 `feat(TASK-003): M1.5 …` 匹配门禁 grep，分支 `task/TASK-003-m15-r2`（不动 dev-a 的分支）；merge `321dfb6` 在 master；`git worktree list` 无 `wt-TASK-003-m15-r2`（已拆）；discovery 同时写 `my_commit`/`master_after_merge` 与 `provenance`，自证数字锚 321dfb6 与我复采一致；code-simplifier 为 dev 自述（三文件 shasum 比对，review 级） | PASS（review） |

## 变异汇总（隔离副本，被测树 321dfb6）

| 变异 | 位置 | 结果 |
|---|---|---|
| M1 `LastIngest` 的 `CASE WHEN` 放宽到任意 outcome（DoD/Leader） | health.go | KILLED |
| M2 `BlockedByCheck` 去掉 `outcome = ?` 过滤（DoD/Leader） | health.go | KILLED（靠 `BlockedCheck = "x"`） |
| M3 `BlockedByCheck` map 不预建 | health.go | KILLED |
| M4 `NotifyFailures` 去掉 `notify_error IS NOT NULL` | health.go | KILLED |
| M5 `parseNullTime` 吞掉解析错误 | health.go | KILLED |
| M6 `blocked by check` 错误前缀写错 | health.go | KILLED |

## 备注（不影响判定）

- dev-m15-a 的孤儿 worktree `../wt-TASK-003-m15`（分支 `task/TASK-003-m15`，三文件未提交、与合入内容逐字节相同）仍在，dev-a 已静默，属 Leader 阶段边界清理项（`git worktree remove --force` + `prune`）。分支 `task/TASK-003-m15-r2` 也仍在（已合入）。
- dev-a 在需求四条之外加的 `TestHealthSummaryRejectsCorruptRunAt`/`TestHealthSummaryPropagatesQueryErrors` 正对 boundary[0] 的 `parseNullTime` 与错误前缀要求，M5/M6 由它们独家杀死；hestia 覆盖率因此 96.6%。
- 复现命令（锚全 sha）：`git worktree add --detach <dir> 321dfb6bba5f412ad7a300a27e7b6c3a6e20efcd && cd <dir> && GOTOOLCHAIN=local go test ./internal/hestia/ -run 'TestHealthSummary|ExposesNoWrite|TestFieldNamesAppearOnlyInFieldsGo' -count=1 -v`

---

## 返工复验（QA W1 `review_fix`）· test-m15-b · 2026-09-04

复验由 test-m15-b 承接，原 verifier test-m15-a 自 15:00Z 派验后无产物（checkpoint 停在 13:42Z、报告无追加），经 `stale-dispatch` 第 2 步于 15:57:25Z 走逃生边 `verifying → verifying` 改派。`verify_baseline` 未刷新。

- 判定对象：`verify_baseline.head = 5dafbf0cf947204255470d659b563829d30f2c3d`（master，merge of `53f1412e6a207dbf7dc82c5c57ec3112a16a6cba` `fix(TASK-003): M1.5 …`，dev-m15-b 自己实现）；判定时 HEAD 与之一致，无漂移；discovery sha256 `8a40c4dd96a84d7473d7876140c47de957550430cc8715682f7c68ef2c94e468` 与基线一致；`assignment_epoch = 2`
- 验证环境：`git worktree add --detach ../wt-verify-TASK-003-b 5dafbf0cf947204255470d659b563829d30f2c3d`；变异在 `mktemp -d` + `git archive 5dafbf0…` 副本上；主仓库与验证树 `health.go` sha256 `bb7bb299…` 每个变异窗口内与收尾均一致
- 返工前基线锚：`a03293d8c74a78e013368aee92a5b8ed7cd177c5`

### 结论：**VERIFIED**

3 条 `fix_items` 与 7 条 `done_criteria` 全部有我自跑的输出作证据；范围核对无越界；5 个变异全部 KILLED。

### 范围核对

`git diff --numstat a03293d 5dafbf0` ⇒ `internal/hestia/health.go` 32/22、`health_test.go` 55/0（与 discovery `verification.rework_w1.numstat_master` 一致），两文件均在声明 `writes` 内；`go.mod`/`go.sum`/四冻结文件 `diff 037d1eb..5dafbf0` 为空；`git show --stat 53f1412` 同两文件。

### fix_items 覆盖矩阵（均在 5dafbf0 上自跑）

| # | fix_item | 证据 | 判定 |
|---|---|---|---|
| 1 | 两段 GROUP BY 循环补 `rows.Err()`，带 `hestia health: <步骤>:` 前缀；推荐抽 `groupCount` helper | `health.go:71-92` 新增非导出 `groupCount(ctx, q, step, query, args...)`：`defer rows.Close()`、Scan 错误与 `rows.Err()` 均包 `hestia health: %s:` 前缀；`RunsByOutcome`（`:41`）与 `BlockedByCheck`（`:49`）两段改走它，五条 SQL 语义不变 | PASS |
| 2 | 补测试钉住：`rows.Err()` 非 nil 时报错且含步骤前缀；变异「删掉 `rows.Err()` 检查」必红 | `TestHealthSummaryReportsRowsErr/{runs_by_outcome, blocked_by_check}` PASS（`cancelingQuerier` 在第 N 次 `QueryContext` 后 cancel 并轮询到 `rows.Err()` 非 nil 再交回，断言 `require.Error` + `Contains` 前缀 + `ErrorIs(context.Canceled)`）；变异 **W1a** 删掉整个 `rows.Err()` 检查块 ⇒ 两子例红 KILLED；**W1b** 改成 `_ = rows.Err()` ⇒ KILLED；**W1c** 前缀丢 `step` ⇒ KILLED | PASS |
| 3 | 约束不变：无 `fieldOrder` 字面量；不新增导出符号；AST 守卫 25 项；hestia ≥ 96.3 且不低于 96.6；提交锚 `fix(TASK-003): M1.5 …`；merge 后重采、discovery 记 W1；`dev_done --expect-epoch 2` | `TestFieldNamesAppearOnlyInFieldsGo` PASS；`groupCount` 非导出（`func groupCount` 小写）；`store_test.go:453` want 机械计数 **25**、`"HealthSummary"` 位置不变；`go test ./internal/hestia/... -cover` **96.6%**；提交信息 `fix(TASK-003): M1.5 HealthSummary 的 GROUP BY 循环查 rows.Err()…`；discovery `decisions` 记 W1 修法与 `cancelingQuerier` 理由、`verification.rework_w1` 全部数字锚 5dafbf0 与我复采一致；transitions.jsonl `in_progress → dev_done by dev-m15-b 14:59:37Z`（`rework_count=1`） | PASS |

### done_criteria 回归（首轮 7 条在 5dafbf0 上重跑）

| # | 证据 | 判定 |
|---|---|---|
| functional[0..2] | `TestHealthSummary{Empty,Timestamps,Counts,PendingReview,RejectsCorruptRunAt×2,PropagatesQueryErrors×7}`、`TestStoreExposesNoWriteMethods`、`TestPackageExposesNoWriteFunctions`、`TestFieldNamesAppearOnlyInFieldsGo` 共 18 行 `--- PASS`、0 FAIL | PASS |
| boundary[0] | **M1**（`CASE WHEN … OR 1=1`）⇒ `TestHealthSummaryTimestamps` 红 KILLED；**M2**（`WHERE outcome = ?` → `WHERE (? IS NOT NULL)`）⇒ `TestHealthSummaryCounts` 红 KILLED | PASS |
| error_handling[0] | 首轮已核（`undefined: HealthSummary`），本轮未动实现文件名，不重复 | PASS（沿用首轮） |
| non_functional[0] | `gofmt -l` 五包恰列 `internal/metrics/snapshot_test.go`、`cmd/atlas/backtest_test.go`、`cmd/atlas/crisis_test.go`；`go vet` 五包 rc=0；`GOTOOLCHAIN=local go test` 五包 `-count=1` 全 ok；hestia 96.6%；无新增依赖；冻结文件 diff 空 | PASS |
| non_functional[1] | AD-6：分支 `task/TASK-003-m15-fix` → 提交 53f1412 → Leader merge 5dafbf0 → master 重采 → discovery `rework_w1`；`git worktree list` 无 `wt-TASK-003-m15-fix` | PASS（review） |

### 变异汇总（隔离副本，被测树 5dafbf0，harness `scratchpad/test-m15-b-mutate-TASK-003-w1.sh`，锚可覆写默认全 sha）

| 变异 | 位置 | 结果 |
|---|---|---|
| W1a 删掉 `groupCount` 的 `rows.Err()` 检查块（fix_items[1] 指定） | health.go:87-89 | KILLED（`ReportsRowsErr` 两子例） |
| W1b `_ = rows.Err()` 吞掉 | health.go:87-89 | KILLED |
| W1c `rows.Err()` 错误前缀丢 `step` | health.go:88 | KILLED |
| M1 `LastIngest` 的 `CASE WHEN` 放宽（回归） | health.go:29 | KILLED |
| M2 `BlockedByCheck` 去掉 `outcome = ?`（回归） | health.go:52 | KILLED |

对照组原状 ok；每个变异体先 gofmt/vet 有效性闸、打印与原文 diff 核对。harness 首跑时 W1a 的自检断言把 helper 注释里的 `rows.Err()` 字样也算进去 ⇒ 变异未落盘被报「未生效」，是 harness 缺陷不是变异存活；修正断言后整套重跑，结果如上。

### 备注（不影响判定）

- test-m15-a 的 `../wt-verify-TASK-003-w1`（detach @ 5dafbf0，工作树干净）仍在，由 Leader 收；`../wt-verify-TASK-003-b` 由我拆。
- 复现命令（锚全 sha）：`git worktree add --detach <dir> 5dafbf0cf947204255470d659b563829d30f2c3d && cd <dir> && GOTOOLCHAIN=local go test ./internal/hestia/ -run 'TestHealthSummary|ExposesNoWrite|TestFieldNamesAppearOnlyInFieldsGo' -count=1 -v`
