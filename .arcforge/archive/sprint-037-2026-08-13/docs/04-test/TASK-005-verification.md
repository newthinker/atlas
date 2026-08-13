# TASK-005 验证报告 —— T3：Ingest 编排与两处接缝

- **验证者**：test-agent-26
- **代码作者**：dev-agent-54（停机失联）｜ **交付者**：dev-agent-53（改派接手，`assignment_epoch=2`）｜ **提交者**：leader（代提交 `0e2c6fc`）
- **验证基线**：`verify_baseline.head = 0e2c6fc976d90382ecb2122dbeb1ed18eaf8c9c9` = 承接时 HEAD ⇒ **无漂移**
- **结论**：**VERIFIED**（7/7 条 done_criteria 全部 PASS）

---

## 0. 承接核实

| 核项 | 结果 |
|---|---|
| 验证对象漂移 | 无漂移 ✅ |
| `assignment_epoch` | **2**，与 `transitions.jsonl` 记录的收回改派（`in_progress → assigned`，leader，16:22:37）一致 ✅ |
| **DoD 未被改写** | 本任务的 `update` 只动过 `questions` 与 `writes`，**`done_criteria` 从未被 update** ✅ |
| discovery | 文件存在（9141 B，**由新 owner 从零补写**）；任务文件的 `discovery` 字段原本缺失，由我补上 |

### ⚠️ 提交含一个声明外文件 —— 成因已查清，**不是 dev-53 的漂移**

`0e2c6fc` 实际改动：`ingest.go` +140、`ingest_test.go` +496、**`store_test.go` +1/-1**。
而 TASK-005 当前 `writes` 只有 `ingest.go` / `ingest_test.go`。

`transitions.jsonl` 还原出的完整时序：

| 时刻 | 谁 | 动作 |
|---|---|---|
| 15:58:20 | **dev-agent-54** | 把 `store_test.go` **加进** `writes`（合规的越界申报，在 `dev_done` 之前） |
| 16:22:37 | leader | 收回任务（dev-54 失联），`epoch` 1→2 |
| 16:22:56 | **leader** | 把 `store_test.go` 从 `writes` 里**移除** |
| — | leader | 代做那一行改动（导出面守卫登记 `"Ingest"`），记录在 H10 |

⇒ **dev-54 当初做对了**（申报在 `dev_done` 之前）；移除是 leader 为解开 `scope-mutex`（`store_test.go` 同在 TASK-006 的 `writes` 里）以便改派。**这一行不是 dev-53 写的，dev-53 的 scope 干净。**

📌 **值得登记的形态**：`writes` 同时驱动 scope 互斥与漂移判据，**为通过 mutex 而移除声明，代价是漂移判据对这一行失效**——CLAUDE.md 里那句「声明与实际写入不一致 = 范围外的真漂移不会告警」在这里得到实例。本次无害（有 H10 记录 + 提交信息写明 + 我已独立核过合法性），登记是为了让这个代价可见。

**那一行本身合法，我独立核过**：`ingest.go` 全文件唯一写路径是 `:129 d.Store.Save(ctx, obs, rep)`，**裸 `Exec`/`ExecContext` 出现 0 次** ⇒ 经过 Save 而非绕过。

---

## 1. 完成标准覆盖矩阵（7 条）

| # | done_criteria | 对应测试/证据 | 判定 |
|---|---|---|---|
| functional[0] | `IngestDeps{Store,Fetch,Cfg,Out,Force}` + `Ingest(ctx,d) error`；端到端测试用 `syntheticIndex` | 两者均交付；`TestIngestEndToEnd` 走完 发现→抓→解析→校验→入库，用 `syntheticIndex` 合成 index | **PASS** |
| functional[1] | 接缝①：`ArticleID` 由 ingest 补，测试钉住「填进去的就是候选的 ArticleID」 | `TestIngestFillsArticleID`：`assert.Equal(annualID, prior[0].Meta.ArticleID)` —— **钉等值而非非空**，注释写明「填错（比如填成整个 URL）会静默入库」 | **PASS** |
| functional[2] | 接缝②：`Candidate.Period` 与 `obs.Meta.Period` 不一致时**拦下、不入库** | `TestIngestRejectsPeriodMismatch`；实现 `ingest.go:120` 显式比对并返回带两侧期次的错误 | **PASS** |
| boundary[0] | 单期失败不中断整批，最后**汇总返回非零**；要有「一页两条、第一条失败、第二条仍被处理」的用例 | `TestIngestContinuesAfterOneFailure`；实现收集 `errs` 后 `fmt.Errorf(...%d/%d 期失败...)`。**该测试还刻意钉住了顺序**（合成 index 把较新的排前，实现升序处理 ⇒ 先跑的正是会失败的 2020-06），注释说明了不钉顺序时断言会与它证明的东西对不上 | **PASS** |
| error_handling[0] | 五个出口各一用例（Discover/Fetch/Parse/Validate/Save），信息含可定位上下文，`%w` + 自证 | `TestIngestWrapsStageErrors` 五条子测试全绿；错误信息含期次与 article_id。**自证方法见 §2 —— 这是本次验证最要紧的一处** | **PASS** |
| non_functional[0] | 候选按期次**升序**处理；**断言必须是肯定式** | `TestIngestProcessesOldestFirst`：`assert.Equal([]string{"2020-06","2025-12"}, got)` **钉具体序列**（肯定式），前置 `require.Len(got, 2, "否则下面的顺序断言会平凡为真")`，另加 `slices.IsSorted`。实现用 `slices.SortStableFunc` 按 period 升序 | **PASS** |
| non_functional[1] | `gofmt`/`vet` 空、整包全绿、`-race` 绿、覆盖率 ≥93.2%；新增导出物登记 + **正向自证** | 见 §3；正向自证**我自己做了一次** | **PASS** |

> 📌 DoD 写「新增导出物（`IngestDeps`/`Ingest`）登记」，其中 **`IngestDeps` 部分不适用**：它是 struct，AST 版守卫只收 `FuncDecl`、reflect 版只看 `*Store` 方法集，**两条都看不见它**（与 TASK-002 的 `Config` 同理，那次的代码注释已说明这一点）。实际需登记的只有 `Ingest`，已登记。**DoD 措辞不精确，非缺陷。**

---

## 2. 🔴 DoD 明文要求的自证方法，在这一层是**平凡为真** —— 我独立实测证实

DoD `error_handling[0]` 写：「用 `%w` 包住，并用 `require.NotNil(errors.Unwrap(err))` 自证（见 TASK-003 的同条）」。

dev-53 在 discovery 里指出**照搬到这里就是一条新形状的平凡为真**，理由是 `Ingest` 的汇总错误**自身**就用 `%w` 包了 `errors.Join(...)`：

```go
return fmt.Errorf("hestia ingest: %d/%d 期失败 (%s): %w",
    len(errs), len(cands), strings.Join(failedPeriods, ", "), errors.Join(errs...))
```

⇒ `errors.Unwrap(汇总err)` 恒非 nil，**与各 stage 是否用 `%w` 无关**。

**我用探针在隔离树里实测（走汇总路径：Discover 成功、`ingestOne` 的 Fetch 失败）：**

| | 基线（stage 用 `%w`） | 变异（stage 改 `%v`） |
|---|---|---|
| `errors.Unwrap(err) != nil` | true | **true（不变）** |
| `errors.Is(err, errBoom)` | true | **false** |

⇒ **`require.NotNil(errors.Unwrap(err))` 在 `Ingest` 层对 stage 的 `%w` 有无完全无鉴别力，声称被完全证实。**

**dev 的处置是按错误结构分两类判据**（注释写明）：

- **Discover / Fetch**：经 `Ingest` 测，注入哨兵 `errBoom` 后断言 **`errors.Is` 找得到它** —— 这条能真正区分（上表第二行）
- **Parse / Validate / Save**：底层错误产自被调包内部、拿不到哨兵 ⇒ 改为**直测 `ingestOne`**，那一层**没有汇总兜底**，`require.NotNil(Unwrap)` 在那里才真的在守 `%w`
- 「包住」一律不用 `NotErrorIs`（不包裹时 `Unwrap` 返回 nil、`errors.Is(nil, err)` 恒 false ⇒ 平凡为真）

**判定：DoD 的字面要求在此上下文无效，dev 用了更强且正确的判据，`error_handling[0]` 的实质（各出口响亮、`%w` 真的在守、有可证伪的自证）逐条满足 ⇒ PASS。**

⚠️ **这条应当回流成 DoD 模板的修正**：`require.NotNil(errors.Unwrap(err))` 只在「被测函数是错误链的**最外层包裹者**」时有鉴别力；一旦外层还有一次 `%w`（汇总、重试、中间层），它就退化成恒真。判据要跟着错误结构走，不能跨层照搬。

---

## 3. 实跑证据（隔离 worktree @ `0e2c6fc`）

```
gofmt → 空   vet → 空   build ./... → OK
go test -count=1 -cover → ok  coverage: 93.5%   (门槛 93.2% ✅)
go test -count=1 -race  → ok
顶层 PASS 308 / FAIL 0
go tool cover -func → Ingest 90.0% / ingestOne 96.2%
```

`TestIngest*` 全部 `--- PASS:`（含 `WrapsStageErrors` 五条、`ReportsVerdictAndTable` 两条、`SkipsSeenArticleUnlessForce` 两条子测试）。

**导出面登记的正向自证（我自己做的）**：从 AST 版期望列表删掉 `"Ingest"` ⇒
`TestPackageExposesNoWriteFunctions` 红在 `store_test.go:420`，顶层 `PASS=307 FAIL=1`（基线 308，**外溢 0**）
⇒ 登记确实在按精确集合相等工作，不是被放宽成包含关系。

---

## 4. 交付者与作者分离 —— discovery 写清楚了

`discovery.by` = `dev-agent-53（接手 dev-agent-54 的交付，epoch=2）`，另有 `provenance` 段。**符合要求。**

**`provenance` 与 `transitions.jsonl` 逐条对照（leader 要求核，此处给证据而非结论）：**

| `provenance` 声称 | `transitions.jsonl` 实录 | 核对 |
|---|---|---|
| leader 于 **16:22:37Z** 收回改派（`in_progress → assigned`，epoch 1→2） | `{"from":"in_progress","to":"assigned","by":"leader","at":"2026-08-12T16:22:37Z"}` | ✅ **逐秒一致** |
| leader 于 **16:22:56Z** 把 `store_test.go` 从 `writes` 移除 | `{"op":"update","by":"leader","at":"2026-08-12T16:22:56Z"}` | ✅ **逐秒一致** |
| 代码由 dev-agent-54 完成 | dev-54 于 15:41:51 认领，15:46:16 / 15:58:20 两次 `update` | ✅ |
| `assignment_epoch` 1→2 | 任务文件 `assignment_epoch = 2` | ✅ |
| 「我的贡献是独立验证 + 落盘，**没有改一行代码**」 | `files_modified: []` | ✅ 自洽 |

⇒ **provenance 与审计流水完全对得上。** 这是本任务归属的唯一记录 ——
`transitions.jsonl` 的 `by` 字段只记「谁转的状态」，**不记「谁写的代码」**，
而本框架下所有 agent 共用同一 git 身份（`author=zuowei`），提交作者字段同样不携带身份信息。

`files_modified: []` **是正确的**，不是漏填：交付已完整满足全部 DoD，为「显得有产出」去重写别人已验证的工作才是错的。

更值得记的是它没有只做"搬运"：它对 dev-54 留下的关键判断做了**独立复核**，并在 discovery 里写明「**dev-54 在错误链判据上做了一个精细且正确的区分，我实测确认它的『理由』也成立（不只是结论成立）**」——
即上面 §2 那件事。**接手者复核前任的理由而不只是接受其结论，这正是「结论对但理由错」那一族的正确处置方式。**

---

## 5. DoD 之外的观察

**O1（与我上一轮报的 Discover 问题是同一件事的两面）** `TestIngestReportsVerdictAndTable` 的注释指出：经 `Ingest` 的候选都已过 `Discover` 的 `HasPeriod` 过滤 ⇒ `Save` 的 `Verdict` **结构上恒为 `New`，`Duplicate`/`Revision` 当前不可达**。

**它刻意不把这条钉成断言**，理由是「把当前的局限焊死，将来有人放开这条路时红的理由是反的」——**这个判断我认同**。

📌 补充我上一轮实测的成因：`discover.go` 是 `if has { return out, nil }`，**命中已入库期次就停止翻页**。所以不只是 `Verdict` 恒 `New`——**已入库期次的修订版一旦插队排在前面，其后所有未入库期次都会被静默漏掉**（我已用真实快照实证：返回 0 条候选、只翻 1 页、`err == nil`）。两者同源，`Verdict` 恒 `New` 是它温和的那一面。

**O2** `Ingest` 覆盖率 90.0%，未覆盖的是汇总分支的部分路径；`ingestOne` 96.2%。均高于包门槛，登记备查。

---

## 6. 复现命令（锚已钉全 sha）

```bash
git worktree add --detach ../wt-v-t005 0e2c6fc976d90382ecb2122dbeb1ed18eaf8c9c9
GOTOOLCHAIN=local go test ./internal/hestia/ -count=1 -cover   # 93.5%
GOTOOLCHAIN=local go test ./internal/hestia/ -count=1 -race
# §2 的探针：走汇总路径（urlFailFetcher + syntheticIndex），比对 stage 的 %w→%v 前后
#   errors.Unwrap(err)!=nil 恒 true；errors.Is(err, errBoom) 由 true 变 false
```

---

## 7. ⚠️ 验证后补记：一条我在首轮验证中**漏掉**的过期注释

leader 在我出具判定后指出 `ingest_test.go:68-72` 有一条过期注释，**我首轮没有发现它**。已实测复核，如实补记于此。

该注释声称：

> 「…… `parse.go` 有自己独立的 `titleRE`（`parse.go:21`），**没跟着放宽** —— 实测 `Parse` 对这两份直接报 `unrecognized report title`。」

**我在隔离树（`0e2c6fc`）上用探针实跑，实际报的是：**

```
hestia: period_type q1 is not supported yet (title "2026年一季度金融统计数据报告"):
  季报抽取侧尚未接线——profiles.go 的 periodAlt 只认「全年|上半年|N月份」……
  由 M1b-4b 的 TASK-004 接上后解除本分支
```

（`pboc-2025-09-q3.html` 同形，报 `q1_q3`。）

⇒ **注释的结论仍成立**（这两份季报正文确实仍进不了 ingest 链路，用它们当失败源仍然正确），
**但理由整个换了**：不再是「`titleRE` 没跟着放宽」（TASK-010 已经放宽了），而是 TASK-010 新加的**显式拒绝分支**。

**判定不变（仍 VERIFIED）**：它只是注释，不参与任何断言，8 条 `TestIngest*` 全绿。
leader 已把「更正这句注释」写进 TASK-004 的 DoD 并把 `ingest_test.go` 加进其 `writes` —— 那条错误信息自己就写着
「由 M1b-4b 的 TASK-004 接上后解除本分支」，代码与注释同处一次改动，不会漏。

**dev-53 没有自己改它是对的，理由是机制而非客气**：本任务当时是 `verifying`，`ingest_test.go` 正是判定对象，
它改就是制造一次 `verify_baseline` 漂移。

### 这条漏检值得记下来的地方

它是**跨任务的注释过期**：注释写于 TASK-005 开发时（当时为真），被 TASK-010 的改动**从外部**变成假的，
而**两个任务都不会因此变红** —— TASK-010 不碰 `ingest_test.go`，TASK-005 的断言不依赖那句话。

⇒ 我首轮的检查覆盖了「断言是否有效」「实现是否符合 DoD」，**但没有覆盖「注释是否仍与当前代码一致」**。
后者在并行 wave 里尤其容易失效，而它恰恰是下一个接手者的入口。
**这与我在 §5 记的 `Preceding` 注释写反是同一族：不参与断言的文字，没有任何机制守护。**

---

## 8. ⚠️ 验证后补记（2026-08-13）：**本报告 §5 的 O1 已被 TASK-011 从外部变假**

**O1 转述的这句现在是错的**：

> 「经 `Ingest` 的候选都已过 `Discover` 的 `HasPeriod` 过滤 ⇒ `Save` 的 `Verdict` **结构上恒为 `New`，
> `Duplicate`/`Revision` 当前不可达**」

**它在写下时（判定对象 `0e2c6fc`）是对的**，随后被两处改动从外部变假：

1. **TASK-011** 把 `Discover` 的判停从 `HasPeriod(期次)` 换成 `HasArticleInObservations(article_id)`
   ⇒ 修订版（新 id）**不再被判停挡住**；
2. **TASK-011** 另加 `ingest.go:58` 的 `if d.Force { known = neverSeen{} }`
   ⇒ **`--force` 穿透判停**，可重跑已在观测表的期次。

**QA 给出的证据比任何测试都硬**：**生产实跑 `--force` 打出 `2026-06 Duplicate`**
⇒ 「Verdict 恒为 New」**在真实央行数据上就是假的**。

### 我为什么没有自己发现它

⚠️ **我在验 TASK-007 时确认过它的反面**：那份报告 §1 的 `boundary[0]②` 逐条核过
`TestForceOnObservedPeriodIsDuplicate`——**「`--force` 重跑已观测期次 ⇒ 走到 `Save` 且判 `Duplicate`」**，
我还独立跑了消融。

⇒ **也就是说：我在 TASK-007 报告里确认了 `Duplicate` 可达，同时没有回头更正 TASK-005 报告里那句「不可达」。**

⇒ 这与本 Sprint QA 报出的 WARNING-2（三份过期副本）**是同一形态，而我的报告是第四份**：
**写下时为真、被上游任务从外部变假、而两个任务都不会因此变红**——
唯一能发现它的动作是**回头检查自己旧报告里被本次改动影响的陈述**，而我没做。

📌 **登记这条比更正它更有价值**：验证报告是归档物，**它和代码注释一样会过期，且同样没有任何机制守护**。
⇒ 可执行的补法：**每次验证发现「上游改动推翻了某个既有陈述」时，`grep` 一次自己已交付的报告。**
本例里 `grep -l "不可达" docs/04-test/*.md` 一条命令就能命中。
