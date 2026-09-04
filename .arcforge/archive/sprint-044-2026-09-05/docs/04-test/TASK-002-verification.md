# TASK-002 验证报告 · `Ingest` 逐候选写 `hestia_runs`，零行时记 `no_new` 心跳

- 验证者：test-m15-a · 2026-09-04
- 判定对象：`verify_baseline.head = e2d1f2be25e6c7f13a6761e6290a654c95dfd529`（master；dev commit `cbac195314b306eddaea944aa39ab87057edb45d` 经 `c1defc00695d22f1b08aeed5e5d78f29c6f4ac61` 合入，其后 TASK-007 的 `e2d1f2b` 合入——`git diff --stat c1defc0 e2d1f2b -- ingest.go ingest_test.go` 为空，范围内无漂移）；当前 HEAD 与 baseline 一致
- discovery：`.arcforge/discoveries/TASK-002.json` sha256 `b88545c29fdb9e655690ec00459dccec314dbfe1db9bd561815412e0d38bf0a1`（与基线一致）
- 验证环境：`git worktree add --detach ../wt-verify-TASK-002 e2d1f2be25e6c7f13a6761e6290a654c95dfd529`；变异与红阶段复现在 `mktemp -d` rsync 副本；主仓库与验证树两文件 sha256 前后一致（harness 自检 PASS）
- 隔离锚：`511ee4264d1df3129b986e4f8857e3284c06d754`（TASK-001 合入后、本任务之前）；覆盖率基线锚 `037d1eb`（hestia 96.5%）

## 结论：**VERIFIED**

8 条 done_criteria 全部有我自跑的输出作证据；范围核对无越界；7 个变异 5 KILLED（含 Leader 指定的两个）、2 SURVIVED（均不是 DoD 缺口，见备注）。

## 范围核对

`git show --numstat cbac195`：`internal/hestia/ingest.go` 125/23、`ingest_test.go` 208/3——恰为声明 `writes` 两文件（显式 pathspec）。`go.mod`/`go.sum`/四冻结文件 diff 为空。

## Done Criteria 覆盖矩阵

| # | 完成标准 | 对应测试 / 证据（均在 e2d1f2b 上自跑） | 判定 |
|---|---|---|---|
| functional[0] | `runResult` 七字段 + `firstLine`；`ingestOne` 返回 `(runResult, error)`；跳过 ⇒ `{processed:false}, nil`；每处 `wrap` 改 `fail`，stage 取原字符串（`has article`/`fetch <URL>`/`snapshot`/`parse`/`validate`/`save`，`mismatch` 单设）；`Save` 后 extractor / pending+第一道 CheckFailed / Duplicate / Ingested；通知失败不走 `fail`（stage 留空）、成功 `notified = d.Notify != nil` | diff 逐行核对全部条目成立；`firstLine` 改名 `firstLineOf`——`status_test.go:161` 既有 `func firstLine(t, s)` 同包冲突属实（我 grep 核实），语义不变；`TestIngestRecordsFailedWithStage`（Stage==`parse`）、`TestIngestRecordsNotifyError`（outcome 仍 ingested、Error 空）PASS；**M4**（`notified = true`）KILLED `:1163`；**M5**（duplicate→ingested）KILLED `:1221`；**M6**（blockedCheck 不填）KILLED `:1193`；**M3**（通知失败改走 `fail`）**SURVIVED**——没有测试断言通知失败情形 `Stage == ""`（DoD 八条测试未列此断言；代码直读 `return res, wrap(stage, err)` 未设 stage，且 AD-14 明示 HealthSummary/collector 不消费 stage） | PASS（备注 M3） |
| functional[1] | `runAt` 在 `d.Out` 判空后；0 候选 ⇒ `recordHeartbeat`；循环形态（`started`、FAILED+P1、P1 送达结果进同一行、`!processed ⇒ continue`、RecordRun 失败 ⇒ `run record FAILED` + `record run:` 前缀）；`processed == 0` ⇒ 心跳（S2）；`runRow` failed 判定；`recordHeartbeat` 形态；**顺序守卫**：RecordRun 在 Save 与 Verdict 打印之后 | `ingest.go:114 if d.Out == nil` → `:117 runAt`；`:178 return d.recordHeartbeat`；循环 `:213-240` 与 DoD 形态逐项一致；`:244 if processed == 0`；`runRow :261`（`!isNotifyError ⇒ RunFailed + firstLineOf`）；`recordHeartbeat :276`（`record heartbeat:` 前缀、`run record FAILED`）；顺序：`RecordRun :237` 在 `Ingest` 循环、`ingestOne` 返回之后，而 `Save :357`/Verdict 打印 `:384` 在 `ingestOne` 内——控制流必然在后（单看行号会误读，discovery 已记）；**M1**（Leader 指定：`Save` 前先 `RecordRun`、失败即 `fail`）⇒ 5 条红含 `KeepsIngestedRow :1306/1307/1322` KILLED；**M2**（Leader 指定：心跳改回 `recorded == 0`）⇒ `:1316 处理过候选就不该再补心跳` KILLED。DoD 字面 `recorded++` 在实现里不存在——S2 之后它没有消费者，省掉是良性 | PASS |
| functional[2] | 八条需求测试通过；既有 `SkipsSeenArticleUnlessForce`/`NotifyFailureIsLoudButNotCascading` 不受影响 | `-v`：IngestedRun / NoNewHeartbeat / PendingWithBlockedCheck / SkippedCandidateFallsBackToHeartbeat / DuplicateOnForce / FailedWithStage / NotifyError / Notified 八个 `--- PASS`，断言内容逐项对 DoD（`2025-12/annual`、Extractor 非空、FinishedAt≥RunAt、共 2 行、`deposit_sum`、`already ingested` 前置、`[no_new, pending]`、`parse` 无换行、`boom`、Notified true）；`internal/hestia` 全包 ok | PASS |
| boundary[0] | `--only-period` 过滤掉的候选不记行；AD-13 写 decisions（含理由与代价） | `TestIngestOnlyPeriodRecordsOnlyKept`：`twoEntryFetcher` 两个可解析候选、`kept 1 of 2` 前置、`countRows==1`、`Period==2025-12` PASS（未用退路）；discovery `decisions[0]` 含 AD-13 理由与「Discover 失败只在 err.log 可见」代价 | PASS |
| boundary[1] | 既有测试零断言改动：删除行 ≤ 3 且只在 `:489/510/530` 的 `d.ingestOne(` 调用形态；`FailedWithStage` 必须真在 `parse` 失败 | `grep -c '^-[^-]'`：以 `511ee42` 与 `037d1eb` 为底均 **3**，三行同为 `requireWrappedStageError(t, d.ingestOne(ctx, annualCandidate()))`，在 511ee42 的行号恰 **489/510/530**；`TestIngestWrapsStageErrors` 五子例 PASS；`FailedWithStage` 断言 `Stage == "parse"` PASS ⇒ 夹具确实在 parse 失败 | PASS |
| error_handling[0] | 红阶段留痕；**必写** `TestIngestRunRecordFailureKeepsIngestedRow`（触发器、`record run`、`HasPeriod` true、`→ hestia_observations` + `run record FAILED`、runs 0 行、无 `no_new` 心跳）+ 零候选变体 `record heartbeat` | discovery `red_phase` 记八条 FAIL 于 `至少要有一行运行记录`（我在副本换回 511ee42 的 `ingest.go` 得到的是签名不匹配的编译错，是签名改动之后的另一时点，不矛盾）；测试断言逐项对 DoD 全部存在且 PASS，另加 `strings.Index` 顺序断言与 `NotContains "no new reports"`；「无 no_new」以错误链 `NotContains "record heartbeat"` 观测（触发器下心跳行同样写不进去，错误链是唯一可观测差别——M2 证明该断言确实在守）；子测试 `零候选时心跳失败同样响亮` PASS | PASS |
| non_functional[0] | 门禁 | `gofmt -l` 五包 = 恰三既有欠账；`go vet` rc=0；五包 `-count=1` 全 ok：**hestia 96.5%**（= 基线，≥ 96.3）/ metrics 98.9% / alert 92.6% / config 83.3% / cmd/atlas 76.3%；无新增依赖；四冻结文件 diff 空；`M1.5 的 TASK-002` 注释 ingest.go ×4、ingest_test.go ×2 | PASS |
| non_functional[1] | AD-6 交付流程 | 提交 `feat(TASK-002): M1.5 …` 匹配门禁 grep；merge `c1defc0` 在 master；`git worktree list` 无 `wt-TASK-002-m15`（已拆）；discovery 同时写 `my_commit_sha`/`merged_master_sha`，自证数字锚 c1defc0、与我在 e2d1f2b 复采一致（范围内文件两锚同内容）；dev 申报的三条 code-simplifier 处置以 diff 核实：`firstLineOf :93`、`isNotifyError :100`（两处消费同判）、`func (d IngestDeps) runRow :261` 保留接收者 | PASS（review） |

## 变异汇总（隔离副本，被测树 e2d1f2b）

| 变异 | 位置 | 结果 |
|---|---|---|
| M1 `Save` 之前先 `RecordRun`，失败即 `fail`（Leader 指定） | ingestOne | KILLED（5 条红） |
| M2 心跳条件改回 `recorded == 0`（Leader 指定） | Ingest 循环后 | KILLED（`:1316`） |
| M3 通知失败改走 `fail`（stage 被设 `send P0/P1`） | ingestOne 末尾 | **SURVIVED** |
| M4 `notified = true`（未配 Notify 也标） | ingestOne 末尾 | KILLED |
| M5 `RunDuplicate` → `RunIngested` | outcome switch | KILLED |
| M6 `blockedCheck` 不填 | outcome switch | KILLED |
| M7 HasArticle 跳过的候选记成 `processed:true, outcome:no_new` | ingestOne | **SURVIVED** |

## 备注（不影响判定，供 TASK-003 / TASK-008 知悉）

- **M3 存活**：`TestIngestRecordsNotifyError` 断言 outcome / Notified / NotifyError / Error，未断言 `Stage == ""`。DoD 八条测试原文就没有这条断言，dev 按 DoD 写；行为本身由代码直读确认正确。若 008 收口时想钉住，加一行 `assert.Empty(t, r.Stage)` 即可。
- **M7 存活**：跳过的候选若被记成一行 `no_new`（带 `Period`）而不补心跳，`TestIngestSkippedCandidateFallsBackToHeartbeat` 的 `[no_new, pending]` 序列不变、行数不变，区分不了。可观测差别是心跳行 `Period` 为空；加一句 `assert.Empty(t, runs[0].Period)` 即可杀。DoD 规定的断言就是序列 + 前置，非 task_defect；对 TASK-003 `LastIngest`（只看 ingested/pending）无影响。
- DoD functional[1] 字面的 `recorded++` 在实现中不存在：S2 把心跳判据改为 `processed` 后它没有消费者，省掉是良性简化。
- 分支 `task/TASK-002-m15` 仍在（已合入）。
- 复现命令（锚全 sha）：`git worktree add --detach <dir> e2d1f2be25e6c7f13a6761e6290a654c95dfd529 && cd <dir> && GOTOOLCHAIN=local go test ./internal/hestia/ -run 'TestIngestRecords|TestIngestSkippedCandidate|TestIngestOnlyPeriodRecordsOnlyKept|TestIngestRunRecordFailureKeepsIngestedRow|TestIngestWrapsStageErrors' -count=1 -v`
