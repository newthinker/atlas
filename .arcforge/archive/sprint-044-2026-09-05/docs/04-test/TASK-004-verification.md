# TASK-004 验证报告 · `metrics.HestiaCollector`——九个指标，空表不输出时间戳，出错只计 `collect_errors`

- 验证者：test-m15-a · 2026-09-04
- 判定对象：`verify_baseline.head = 4c14d7903cbd01258ddeab9e5f3aab5641871c47`（master，merge of dev commit `3ba1e9fd159a6d51c03f4200dfc56af05ce21486`，dev-m15-c）；当前 HEAD 与之一致，无漂移
- discovery：`.arcforge/discoveries/TASK-004.json` sha256 `e306d816cc8c6df1442d386b86cdcbb950702227f0d433de4ad6bbe94d16484a`（与基线一致）
- 验证环境：`git worktree add --detach ../wt-verify-TASK-004 4c14d7903cbd01258ddeab9e5f3aab5641871c47`；变异与红阶段复现在 `mktemp -d` + `git archive` 副本；主仓库与验证树两文件 sha256 前后一致（harness 自检 PASS）
- 隔离锚：`321dfb6bba5f412ad7a300a27e7b6c3a6e20efcd`；覆盖率基线锚 `037d1eb`（metrics 98.9%）

## 结论：**VERIFIED**

7 条 done_criteria 全部有我自跑的输出作证据；范围核对无越界；8 个有效变异全部 KILLED（含 S7 与 `now` 注入两个指定项）。

## 范围核对

`git show --numstat 3ba1e9f`：`internal/metrics/hestia_collector.go` 108/0、`hestia_collector_test.go` 208/0——恰为声明 `writes` 两文件（显式 pathspec，均为新文件）；`internal/metrics` 其他文件 diff 为空。`go.mod`/`go.sum`/四冻结文件 diff 为空。

## 导出面与指标表（Leader ④）

- `go doc -all ./internal/metrics` 中 Hestia 相关导出恰为：`type HealthFunc`、`type HestiaCollector`、`func NewHestiaCollector(fetch HealthFunc, now func() time.Time) *HestiaCollector`、`(*HestiaCollector).Collect`、`(*HestiaCollector).Describe`——无其他新增导出符号；`hestiaScrapeTimeout`（5s）与 `allOutcomes` 非导出。
- 九个指标名各出现 1 处；Help 九条逐条 `grep -F` 在需求原文命中；类型：`last_run_timestamp`/`last_ingest_timestamp`/`hours_since_last_run`/`hours_since_last_ingest`/`pending_review` 为 `GaugeValue`，`runs_total{outcome}`/`validation_blocked_total{check_id}`/`notify_failures_total` 为 `CounterValue`，`collect_errors_total` 为实例内 `prometheus.NewCounter`。
- code-simplifier 三处申报以代码核实：`hestiaScrapeTimeout` 包级常量（`:38`，`Collect :80` 用它包 `context.WithTimeout`）、`Describe` 单循环列九个 Desc、测试 `fetchFullHealth` 闭包与 `if mf, ok := fam[x]; ok` 缺席断言——均不改行为与导出面。

## Done Criteria 覆盖矩阵

| # | 完成标准 | 对应测试 / 证据（均在 4c14d79 上自跑） | 判定 |
|---|---|---|---|
| functional[0] | `HealthFunc`；`NewHestiaCollector(fetch, now)` 实现 `prometheus.Collector`；5s 超时包住 `fetch`；九个指标名/类型/标签/Help 固定，`runs_total` 五 outcome 恒输出；`TestHestiaCollector_FullOutput` | 见上节；`TestHestiaCollector_FullOutput` `--- PASS`（断言逐项对 DoD：九族齐全、`Unix()`、2 / 48、3、1、`{no_new:10, ingested:2, pending:1, duplicate:0, failed:0}`、`deposit_sum` 1、`collect_errors` 0）；**M3**（只输出出现过的 outcome）⇒ `:130 got map[ingested:2 no_new:10 pending:1]` + `:167` KILLED；**M6b**（blocked 值 +PendingReview）⇒ `:137 value:4` KILLED | PASS |
| functional[1] | 空表四个时间戳类不输出、`pending_review` 仍 0、`runs_total` 仍五序列（`EmptyOmitsTimestamps`）；`fetch` 出错不输出任何事实指标、只 `collect_errors.Inc()`，测试**还要断言 `fam["hestia_runs_total"]` 为 nil**（S7），第二次 = 2（`ErrorEmitsOnlyCollectErrors`） | 两条 `--- PASS`；`hestia_collector_test.go:185-187` 有 `fam["hestia_runs_total"]` 缺席断言；**M4**（`LastRun` 零值也输出）⇒ `:157` ×2 KILLED；**M5**（出错不 `Inc`）⇒ `:189/:193` KILLED；**S7 三形态**：**M1**（只去 `return`）⇒ 重复 `Collect` 让 `Gather` 报 `collected before`、在 `:181 gather:` Fatal（KILLED 但不是目标断言）；**M1b**（去 `return` 且不重复 Collect ⇒ 零值 Health 继续输出）⇒ `:183` + `:186` 双红；**M1c**（S7 原话「出错仍输出五个恒 0 序列」）⇒ **只**红在 `:186 must not emit runs_total when HealthSummary fails`——证明该断言独立承重，只断 `pending_review` 缺席时 M1c 会存活 | PASS |
| functional[2] | `reg.Snapshot()["hestia_hours_since_last_run"] == 2`、`["hestia_runs_total"] == 13` | `TestHestiaCollector_VisibleInSnapshot` `--- PASS`；M2 时 `:203 snapshot … = -380.96, want 2` 连带红，证明该断言在守 | PASS |
| boundary[0] | 空 `BlockedByCheck` ⇒ `validation_blocked_total` 族无序列且测试不 panic；`range` 顺序随机只按标签取；`Collect` 无 `time.Now()`，变异换 `time.Now` ⇒ `hours_since` 必红；不新增导出符号 | `EmptyOmitsTimestamps :170` 用 `if mf, ok := fam[…]; ok` 判族缺席（不解引用）；`firstMetric` helper 对空族 `Fatalf` 而非 `[0]` panic；`labelValue` 按标签取；`time.Now()` 在非注释行出现 **0** 次（唯一命中在 `:24` 注释）；**M2**（`now := time.Now()`）⇒ `:114 -380.96 want 2`、`:117 -334.96 want 48`、`:203` KILLED；导出面见上节 | PASS |
| error_handling[0] | 红阶段 `undefined: NewHestiaCollector`；`MustNewConstMetric` 不写 recover | 副本移走 `hestia_collector.go` ⇒ `:31:38 undefined: HestiaCollector`、`:95/:147/:177 undefined: NewHestiaCollector`；discovery `red_phase` 同形；源码无 `recover` | PASS |
| non_functional[0] | 门禁 | `gofmt -l` 五包 = 恰三既有欠账；`go vet` rc=0；五包 `-count=1` 全 ok：**metrics 99.2%**（基线 98.9，与 Leader 预演一致）/ hestia 96.6% / alert 92.6% / config 83.3% / cmd/atlas 76.3%；无新增依赖；四冻结文件 diff 空；`M1.5 的 TASK-004` 注释两文件均有 | PASS |
| non_functional[1] | AD-6 交付流程 | 提交 `feat(TASK-004): M1.5 …` 匹配门禁 grep；merge `4c14d79` 在 master；`git worktree list` 无 `wt-TASK-004-m15`（已拆）；discovery 同时写 `my_commit`/`master_after_merge`，自证数字锚 4c14d79 与我复采一致；code-simplifier 三处改动已申报并核实（见上节）；`provenance` 写明原派 dev-m15-a 未开工即收回、代码全由 dev-m15-c 写（`git worktree list` 无 dev-a 残留，与之相符） | PASS（review） |

## 变异汇总（隔离副本，被测树 4c14d79）

| 变异 | 位置 | 结果 |
|---|---|---|
| M1 出错分支只去 `return`（Leader ①） | Collect | KILLED（重复 Collect ⇒ `:181 gather:` Fatal） |
| M1b 去 `return` 且不重复 Collect | Collect | KILLED（`:183` + `:186`） |
| M1c 出错时仍输出五个恒 0 `runs_total`（S7 原话） | Collect | KILLED（**仅** `:186`） |
| M2 `now := c.now()` → `time.Now()`（Leader ②） | Collect | KILLED（`:114/:117/:203`） |
| M3 `runs_total` 只输出出现过的 outcome | Collect | KILLED |
| M4 `LastRun` 零值也输出时间戳 | Collect | KILLED |
| M5 出错不 `Inc` | Collect | KILLED |
| M6b `blocked` 值错 | Collect | KILLED |

（M6 首版让 `n` 未使用，被 harness 的 vet 有效性闸拦下作废，重做为 M6b。）

## 备注（不影响判定）

- 分支 `task/TASK-004-m15` 仍在（已合入）。
- 供 TASK-006 接线：空库上 `hestia_pending_review` 与 `hestia_runs_total` 可见、`hestia_last_run_timestamp` 不可见（discovery `interfaces_exposed[2]` 已写，与 `EmptyOmitsTimestamps` 一致）。
- 复现命令（锚全 sha）：`git worktree add --detach <dir> 4c14d7903cbd01258ddeab9e5f3aab5641871c47 && cd <dir> && GOTOOLCHAIN=local go test ./internal/metrics/ -run TestHestiaCollector -count=1 -v`
