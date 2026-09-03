# TASK-006 验证报告 · IngestDeps.OnlyPeriod：只与 Force 同用，Discover 之后过滤，0 匹配响亮失败（包级）

- 验证者：test-m1d-a　承接 epoch：1　判定：**VERIFIED**
- verify_baseline.head：`3c567609e7135035cabf375bdf207d1861e18d46`（master）；discovery sha256 与基线一致 `65069bba…4341`
- dev commit：`a9cf331655b07cc659f73c08233c1d06821945d2`（父 `1b6ef242a1bda4ad889540f6b360c07aef7c8c9e`；merge `3c56760`）；改动 `ingest.go` 22+/0-、`ingest_test.go` 72+/0-，`cmd/atlas` 未动（`git diff --stat 1b6ef24 3c56760 -- cmd` 0 行）
- 验证树：`git worktree add --detach ../wt-verify-TASK-006-m1d 3c567609e7135035cabf375bdf207d1861e18d46`；变异在独立树 `…-mut`（同 sha）上做，每次后 `git checkout --` 还原并核实 porcelain 为 0；主仓库 `ingest.go` sha256 前缀 `3fa8661d71ad4567` 与 `3c56760` 一致；A/B 覆盖率钉在 `a9cf331` 与 `1b6ef24`

## 1. 结论

字段、校验位置、过滤位置、三处文案与需求原文 Step 1–3 逐字一致；cmd 层 Step 4 按 AD-2 归 TASK-007，本任务未碰 `cmd/atlas`。三条新测试全绿，在原文基础上多了 R-006a 的 `assert.Empty(f.calls)`。6 组变异（含 Leader 点名四组与「被过滤那期不发消息」「配置错误不碰库」两处）全部 KILLED。R-006b 备注属实。门禁全绿、覆盖率 96.4%、无越界。本任务是本 sprint 第一个 dev 自跑变异后交付的，验证结果与其自报一致。

## 2. Done Criteria 覆盖矩阵

| # | 完成标准 | 对应测试 / 证据 | 判定 |
|---|---|---|---|
| functional[0] | `Force+OnlyPeriod` 两条候选只处理目标期：`outPeriods==["2025-12"]`、`kept 1 of 2`、observations 1、h1 未入库、pending 0、`texts` 恰 1 含 `2025-12/annual` | `TestIngestOnlyPeriodFiltersCandidates` PASS，七条断言逐项对应；变异 M2（谓词恒 false）⇒ FAIL；M3（过滤不作用于循环，`cands` 不变）⇒ FAIL（h1 入库、texts 2 ⇒ 被过滤那期发了消息即红） | PASS |
| functional[1] | 过滤在 Discover 之后、`ingestOne` 之前；Discover 不动 | `ingest.go:169-180` 位于 `SortStableFunc` 之后、`for _, c := range cands` 之前；`discover.go` 不在 diff 中；`kept 1 of 2` 证明 Discover 看到两条；M3 ⇒ FAIL | PASS |
| boundary[0] | 非空且 `!Force` ⇒ 错含 `OnlyPeriod requires Force`、不碰库；位置在 `d.Out == nil` 之后、Discover 之前 | `TestIngestOnlyPeriodRequiresForce` PASS（observations 0）；`ingest.go:89-92` 紧跟 `d.Out` 归一化；变异 M6（删校验）⇒ FAIL | PASS |
| boundary[0] R-006a | `len(f.calls) == 0` 钉「Discover 之前拦下」 | `assert.Empty(t, f.calls)` 在用例内；变异 M1（校验挪到 Discover 之后）⇒ FAIL | PASS |
| error_handling[0] | 0 匹配 ⇒ 错含 `1999-01` 与 `no candidate`，不走 `no new reports` | `TestIngestOnlyPeriodNoMatchFailsLoudly` PASS；变异 M4（改走 `no new reports` 返回 nil）⇒ FAIL；M9（文案丢期次）⇒ FAIL | PASS |
| error_handling[0] R-006b 备注 | 0 匹配时 `kept 0 of N` 仍先打出 | 探针实测 Out = `…\ndiscover stopped: exhausted (2 candidate(s))\nonly-period 1999-01: kept 0 of 2 candidate(s)\n`，err = `hestia ingest: no candidate for period 1999-01 within max_pages 3`；核对时未把该行当成功迹象 | N/A（备注） |
| non_functional[0] | 既有测试全绿不改断言；TDD 红 `unknown field OnlyPeriod`；不新增导出函数；`twoEntryFetcher` 形态 | diff 中既有用例零改动（`ingest_test.go` 纯追加 72 行）；discovery `tdd_red` 附 3 处行号；`grep '^+(func|type|var|const) [A-Z]'` 无；`twoEntryFetcher` 与原文逐字；`TestPackageExposesNoWriteFunctions`、`TestFieldNamesAppearOnlyInFieldsGo` PASS | PASS |
| non_functional[1] 门禁 | gofmt / vet / 两包 / 覆盖率 ≥ 96.3% / 五个不动文件 / 无新依赖 / 注释前缀 | 树 `3c56760`：`gofmt -l` 仅 `backtest_test.go`、`crisis_test.go`；`go vet` rc=0；两包 `-count=1` ok；`-cover` **96.4%**（`Ingest` 97.8%）；A/B 背对背 `a9cf331` 96.4% / `1b6ef24` 96.4%；`git diff --stat 4916106 3c56760 -- {store,validate,parse,extract,fields}.go` 0 行；`go.mod/go.sum/types.go` 相对 `ae088eb` 0 行；`ingest.go:42` 注释 `M1d 的 TASK-006` | PASS |
| non_functional[2] 交付流程 | 提交锚 / code-simplifier / merge 先于 dev_done / 重采 / discovery | `feat(TASK-006): M1d …` 匹配 `^[a-z]+\(TASK-006\):`；merge `3c56760` 早于 `dev_done`（10:34:18Z）；discovery 含 my_commit / master_after_merge / master_gates_after_merge / mutations_self_check；无 `wt-TASK-006-m1d` 残留 | PASS |

**越界申报核对**：`git show --stat a9cf331` ⇒ 恰 `ingest.go`、`ingest_test.go`，与 `writes` 一致；两文件在 `a9cf331` 与 `3c56760` 逐字节一致。

## 3. 变异汇总（独立变异树，6/6 KILLED）

| 变异 | 期望 | 实测 |
|---|---|---|
| M1 校验挪到 Discover 之后 | RequiresForce 红（calls 非空） | KILLED |
| M2 过滤谓词恒 false | FiltersCandidates + NoMatch 红 | KILLED（2 条红） |
| M3 过滤只算计数、不作用于循环 | FiltersCandidates 红 | KILLED |
| M4 0 匹配改走 `no new reports` 返回 nil | NoMatch 红 | KILLED |
| M6 删掉 requires-Force 校验 | RequiresForce 红 | KILLED |
| M9 错误文案丢掉期次 | NoMatch 红 | KILLED |

## 4. 交接给 TASK-007（cmd 层）的核对点

- discovery `interfaces_exposed[0]`：cmd 层应在 `openHestia()` 之前先做 `--only-period requires --force` 与格式（`hestiaBackfillFromRE`）校验，包级校验是第二道防线——需求原文 Step 4 的 `TestHestiaOnlyPeriodValidation` 正是抓「校验放到开库之后」这个顺序错误。
- 过滤只比 `Candidate.Period`（YYYY-MM）不看 `PeriodType`，同月 annual 与 monthly 都保留——discovery 已声明，cmd 层 help 文案不要暗示按类型过滤。

## 5. 复现命令（锚全 sha）

```bash
git worktree add --detach ../wt-verify-TASK-006-m1d 3c567609e7135035cabf375bdf207d1861e18d46
cd ../wt-verify-TASK-006-m1d
GOTOOLCHAIN=local go test ./internal/hestia/ -run 'TestIngestOnlyPeriod' -v -count=1
GOTOOLCHAIN=local go test ./internal/hestia/ -cover -count=1     # 96.4%
```
