# TASK-007 验证报告 · `hestia status` 显示最近 5 次运行（`runs` 段）

- 验证者：test-m15-a · 2026-09-04（11:03Z 判定；11:10Z 按 Leader 补充追加 §补充 与 M3b，判定不变）
- 判定对象：`verify_baseline.head = e2d1f2be25e6c7f13a6761e6290a654c95dfd529`（master，merge of dev commit `f9410c026e28300523091ccc007607f214f0c422`）；当前 HEAD 与之一致，无漂移
- discovery：`.arcforge/discoveries/TASK-007.json` sha256 `0aa410bfebf582525a4ece99e496f7caa0101bc024f302374c8a7ca72168332f`（与基线一致）
- 验证环境：`git worktree add --detach ../wt-verify-TASK-007 e2d1f2be25e6c7f13a6761e6290a654c95dfd529`；变异与红阶段复现在 `mktemp -d` rsync / `git archive` 副本；主仓库与验证树四文件 sha256 前后一致（harness 自检 PASS）
- 隔离锚：`511ee4264d1df3129b986e4f8857e3284c06d754`；覆盖率基线锚 `037d1eb`（hestia 96.5% / cmd/atlas 76.3%）

## 结论：**VERIFIED**

7 条 done_criteria 全部有我自跑的输出作证据；范围核对无越界；7 个有效变异 6 KILLED、1 SURVIVED（预期内，见备注）。

## 范围核对

`git show --numstat f9410c0`：`cmd/atlas/hestia.go` 11/1、`hestia_test.go` 78/0、`internal/hestia/status.go` 26/1、`status_test.go` 47/5——恰为声明 `writes` 四文件（显式 pathspec；B3 补进的 `hestia_test.go` 在内）。`go.mod`/`go.sum`/四冻结文件 diff 为空。

## Done Criteria 覆盖矩阵

| # | 完成标准 | 对应测试 / 证据（均在 e2d1f2b 上自跑） | 判定 |
|---|---|---|---|
| functional[0] | `RenderStatus(w, dbPath, obs, pending, runs []Run) error`；`return nil` 前加 `runs` 段（注释带「销 M1d 挂账 C2 的第二半」）：`\nruns: %d\n`，每行 `  <RFC3339 UTC>  %-9s<outcome>` + 条件追加 `period/period_type`、`stage=`、`<error>`、`notify_error=`，`Fprintln` 收尾；import `time` | diff 逐行核对：签名、注释文案、五个 Fprintf 形态、`time` import 均与 DoD 一致；**M4**（去 `stage=` 前缀）⇒ `status_test.go:183` KILLED；**M6**（`runs:` 计数恒 0）⇒ `:182` + `hestia_test.go:286` KILLED | PASS |
| functional[1] | `TestRenderStatusRuns`（三行、`runs: 3`、`failed     2026-08/monthly  stage=parse  boom`、`ingested   … notify_error=telegram down`、`no_new`）与 `TestRenderStatusRunsEmpty`（`runs: 0`）通过；既有 5 处调用点补 `nil`、断言不动；`ReportsWriteError` 仍绿 | 两条测试 `--- PASS`（断言字面与 DoD 逐字相同）；`status_test.go` 删除行恰 **5** 且全是 `RenderStatus(` 调用的尾行（`:50/:75/:97/:121/:141`），既有断言零改动；`TestRenderStatusReportsWriteError` PASS | PASS |
| functional[2] | `runHestiaStatus`：`pending` 之后 `runs, err := st.RecentRuns(ctx, runsLimit)`，`const runsLimit = 5` 在 `statusLimit` 旁、注释指 spec §6 并说明为何与 `statusLimit` 不同，错误传播，第五参数传 `runs`；**B3**：`TestHestiaStatusOnEmptyStore` 补 `runs: 0`；新增 `TestHestiaStatusShowsRecentFiveRuns`（6 行 ⇒ `runs: 5` 且 `2026-01/monthly` 不出现）；`TestHestiaCmdDoesNotResolveDBPath` 仍绿；AST 守卫 want 不变 | `hestia.go:16 statusLimit` / `:22 runsLimit = 5`（注释含 spec §6 + 「一月一行 vs 一天三次」理由）/ `:354 RecentRuns(ctx, runsLimit)` + `if err != nil { return err }` / `:358 RenderStatus(…, runs)`；三条 cmd 测试 `--- PASS`（ShowsRecentFiveRuns 断言 `runs: 5`、`2026-06` 在、`2026-01` 不在）；`hestia.go` 含 `"path/filepath"` = 0；AST want 计数 24 不变、`TestPackageExposesNoWriteFunctions` PASS；**M3**（`runsLimit = 10`）与 **M3b**（Leader 指定形态 `runsLimit = 6`）⇒ 均 `hestia_test.go:286/288` KILLED；**M5**（吞掉 RecentRuns 错误）⇒ `:318` KILLED | PASS |
| boundary[0] | `runs` nil 与空切片同形；`%-9s` 对 `duplicate`（9 字符）恰不截断，dev 不得调宽度凑断言；`RunAt` 先 `UTC()` | `TestRenderStatusRunsEmpty/{nil,empty}` 两子例 PASS；`printf duplicate | wc -c` = 9；**M2b**（`%-9s`→`%-8s`，锚定 Fprintf 行）⇒ `:183/:184` 两条断言同红 KILLED——证明断言里的空格数确是宽度的结果；**M1**（去 `UTC()`）**SURVIVED**：测试输入本就是 `time.UTC`，且 `RecentRuns` 解析 `Z` 后缀得到的 Location 也是 UTC，该调用是防御性的、无可观测差别——按 Leader 预设记为测试强度边界，不阻断 | PASS |
| error_handling[0] | 红阶段 `too many arguments in call to RenderStatus` | 副本把 `status.go`/`hestia.go` 换回 `511ee42` 保留新测试 ⇒ `status_test.go:50:7: too many arguments in call to RenderStatus … want (io.Writer, string, []StatusRow, []PendingRow)`；discovery `red_phase` 同文案 | PASS |
| non_functional[0] | 门禁 | `gofmt -l` 五包 = 恰三既有欠账；`go vet` rc=0；五包 `-count=1` 全 ok：**hestia 96.5%**（= 基线，≥ 96.3）/ **cmd/atlas 76.3%**（= 基线；dev 补 `TestHestiaStatusPropagatesRunsError` 才守住，见备注）/ metrics 98.9% / alert 92.6% / config 83.3%；无新增依赖；四冻结文件 diff 空；`M1.5 的 TASK-007` 注释四文件均有 | PASS |
| non_functional[1] | AD-6 交付流程 | 提交 `feat(TASK-007): M1.5 …` 匹配门禁 grep；merge `e2d1f2b` 在 master；`git worktree list` 无 `wt-TASK-007-m15`（已拆）；discovery 同时写 `my_commit`/`master_after_merge`，自证数字锚 e2d1f2b 与我复采一致；code-simplifier 一项 dev 自述为假，见 §补充（归属错标，不影响交付物） | PASS（review） |

## 变异汇总（隔离副本，被测树 e2d1f2b）

| 变异 | 位置 | 结果 |
|---|---|---|
| M1 去掉 `RunAt.UTC()` | status.go | **SURVIVED**（预期：无可观测差别） |
| M2b `%-9s` → `%-8s`（Fprintf 行） | status.go | KILLED |
| M3 `runsLimit = 5` → `10` | hestia.go | KILLED |
| M3b `runsLimit = 5` → `6`（Leader 指定形态） | hestia.go | KILLED |
| M4 去掉 `stage=` 前缀 | status.go | KILLED |
| M5 吞掉 `RecentRuns` 错误 | hestia.go | KILLED |
| M6 `runs:` 计数恒 0 | status.go | KILLED |

（首版 M2 的 perl 表达式命中的是注释里的 `%-9s` 而非代码，gofmt/vet 闸不拦注释改动，靠打印 diff 才发现——按语义闸纪律作废并重跑为 M2b。）

## 补充：discovery 里「code-simplifier 无改动」为假（dev-m15-c 自报，Leader 转来；落点本报告，**不改 discovery**）

- **事实**：discovery `decisions[3]` 与 `verification.code_simplifier` 写「跑过、无改动（numstat 前后一致核实）」。实际 code-simplifier 改了 `cmd/atlas/hestia_test.go` 的 `TestHestiaStatusShowsRecentFiveRuns` 两处（去掉单次使用的 `dir`；循环体提 `runAt := at.AddDate(0, i, 0)`），一减一增净 0 行。
- **核实**：`git show f9410c0:cmd/atlas/hestia_test.go | grep -n 'AddDate'` ⇒ `273: runAt := at.AddDate(0, i, 0)`（存在，且 master `e2d1f2b` 同文件同行号 273）；该测试函数体内无 `dir :=`（`:259` 直接 `dbPath := filepath.Join(t.TempDir(), "hestia.db")`）。两处改动**已在 `f9410c0` 里**，自证数字采自含改动的树，交付物无误，只是归属写错。
- **仪器边界**：`git diff --numstat` 对「净零行」改动无鉴别力（一减一增相抵），dev 据它得出「无改动」是假阴——判「改没改」要用内容判据（`git diff` 本身 / 文件 sha256），不用行数聚合。**不构成 reject 理由**。
- **处置**：discovery 保持原样——`verified` 后时机守卫无条件 DENY 覆盖，且错标签本身有诊断价值（它记录了「numstat 核实」这个会假阴的动作）。

## 备注（不影响判定）

- dev 在 DoD 之外补了 `TestHestiaStatusPropagatesRunsError`（database/sql 直连塞坏 `run_at` ⇒ status 报 `bad run_at`）以守住 `cmd/atlas` 76.3% 基线，文件在 `writes` 内、discovery 已申报；diff 核实它未改任何既有断言（`hestia_test.go` 删除行 = 0）；它测的是 functional[2]「错误传播」这条行为，M5 由它独家杀死。
- `TestHestiaStatusShowsRecentFiveRuns` 刻意只依赖 `run_at` 次序、不依赖 rowid（引用了我在 TASK-001 报告里记的 M6 存活）。
- 分支 `task/TASK-007-m15` 仍在（已合入）。
- 复现命令（锚全 sha）：`git worktree add --detach <dir> e2d1f2be25e6c7f13a6761e6290a654c95dfd529 && cd <dir> && GOTOOLCHAIN=local go test ./internal/hestia/ -run TestRenderStatus -count=1 -v && GOTOOLCHAIN=local go test ./cmd/atlas/ -run TestHestiaStatus -count=1 -v`
