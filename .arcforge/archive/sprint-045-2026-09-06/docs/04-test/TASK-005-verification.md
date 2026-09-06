# TASK-005 验证报告 · M2a ingest 接线 + 空 `queue.dir` 守卫 + `contractError` + P2 信号行

- **验证者**：test-m2a-b
- **判定**：**VERIFIED**（附一条高优先级测试残留 M8，见 §3/§4）
- **判定对象**：`verify_baseline.head = 189a9eab5f46d760fd4e4665d2d6e2b0981c5522`（当前 master，含 005 的 merge `f2079584` 与其后 006 的 merge；`git diff --stat f2079584 189a9eab -- internal/hestia` 为 **0 行**，两树在本任务范围内逐字节相同）；dev 提交 `dca854d92fd49236c1a246d085eafd57b5184616`（dev-m2a-a）；discovery sha256 `0953ef67c3cad9b941152b5eeede20e5a347ff4e8920005daa62eaa769f4120e`；承接时 `assignment_epoch=1`
- **验证树**：`../wt-verify-TASK-005-b`（detached 于 `189a9eab…`），全部数字自采于此树
- **上游**：TASK-001～004（均 verified）；开工锚 `d27791c695e8ebd0fd5d54c9161782f52d9b12cb`

## 1. 完成标准覆盖矩阵

| # | 完成标准（摘要） | 对应测试 / 证据（自采） | 判定 |
|---|---|---|---|
| functional[0] | `Ingest` 入口：`OnlyPeriod` 校验旁、任何 I/O 之前，空 `queue.dir` ⇒ `hestia ingest: queue.dir must not be empty`；`EnsureQueueDirs` 失败 ⇒ `hestia ingest: %w`；`ingestCfg` 三字段；直建 `Config{}` 的用例补 `Queue`；`TestIngestRejectsEmptyQueueDir` 用 `fatalFetcher`；`find` 为空 | diff 核实（守卫位于 `OnlyPeriod` 校验之后、`Force`/Discover 之前）；`ingestCfg` 含 `ConfigVersion:"test"`/`Queue`/`Signals`；`TestIngestRejectsPeriodMismatch` 的直建 `Config` 补 `Queue`；`RejectsEmptyQueueDir`/`RejectsUnusableQueueDir`（两者 `fatalFetcher{t}`）PASS；全量测试跑完后 `find internal/hestia cmd/atlas -type d \( -name pending -o -name queue \)` 命中 **0**。**判据真正生效的证据**：变异 M2（删空串守卫）后同一 `find` 命中 4 条（`internal/hestia/{pending,processing,done,failed}`），守卫在时为 0——AD-5 的预言在本任务实测成立 | PASS |
| functional[1] | 契约段条件 `Table==observations && Verdict ∈ {New, Revision}`（AD-14）；Revision 取 `PriorPublishedAt` 填 `Supersedes`；`WriteContract` 成功打印 `%s contract → %s`；`temp = Evaluate(...)`；三条原文测试 + `TestIngestNoContractOnOutOfOrder`；`encoding/json` import | diff 逐字核实（含 `SourceURL: c.URL`）；`WritesContractOnObservation`（含四子目录）/`NoContractOnPending`/`NoContractOnDuplicate`/`NoContractOnOutOfOrder`（Verdict 行含 `OutOfOrder`、`countRows==2`、`pending` 空、P2 含 `温度 0/0`）/`RevisionContractCarriesSupersedes`（`is_revision=true`、`supersedes_published_at=2026-01-01`）PASS；变异 M1（改回 `!= Duplicate`）由 `OutOfOrder` 用例独家转红，M6/M7 由 Revision 用例转红；import 块含 `encoding/json` | PASS |
| functional[2] | `contractError{err}`（`Error()` 无前缀、`Unwrap`）+ `isContractError`；`PriorPublishedAt`/`WriteContract` 失败 `fail("contract", contractError{…})`；`runRow` 对它 `Outcome` 保持 `RunIngested`、`Error` 首行；循环不改 ⇒ P1 照发；`TestIngestContractWriteFailureKeepsRowAndSkipsP2` 的 AD-4a 夹具与全部断言 | diff 逐条核实（`ingest.go:64-67`/`:117-120`/`:294-304`/`:432`/`:438`）；用例断言逐条对照 DoD：`require.Error` 含 `contract`、`countRows==1`、`texts` 恰 1 条 `HasPrefix "[P1]"` 含 `contract` 不含 `信号 活化`、`Outcome==RunIngested`、`Stage=="contract"`、`Error` 含 `contract`、`Notified==true`——**八条全在**；变异 M3（去掉 contractError 分支 ⇒ 记 failed）、M5（写失败后仍发 P2）、M13（不记 Error 列）均由该用例转红 | PASS |
| functional[3] | `renderP2(obs, out, temp)`；信号行在锚字段行后、`article` 行前；`dot()`；头注释改（AD-11）；既有 7 处补 `Temperature{}`；`TestRenderP2CarriesSignals`；Duplicate 用例加 `温度 0/0`；`ingest.go` 调用改三参 | 格式串与需求原文逐字相同（位置正确）；`dot` 与原文相同；`notify.go` 旧文案「尚未实现」0 处；`renderP2` 调用点 **9** 处（`notify_test.go` 既有 7 + 新增 1 + `ingest.go:448`）**全部三参**；`CarriesSignals`、`DuplicateSaysValuesNotWritten`（含 `温度 0/0`）PASS；变异 M9（unknown 画 🔴）、M15（去 `信号 ` 前缀）由 `CarriesSignals` 转红 | PASS |
| boundary[0] | `TestIngestNotifyFailureIsLoudButNotCascading` 追加：P2 失败时 `pending/2025-12-annual.json` 仍在且可 `Unmarshal` | diff 核实（改用 `cfg` 变量、追加三条断言、既有断言未动）；PASS | PASS |
| error_handling[0] | 红阶段 `tail -8` 含 `renderP2` 参数数编译错误；实现后全包绿、既有 `TestIngestRecordsIngestedRun` 等不受影响 | 验证树把 `ingest.go`/`notify.go` 退回 `057a91c5` 复现：`notify_test.go:155:79: too many arguments in call to renderP2 / have (Observation, Outcome, Temperature) / want (Observation, Outcome)`、`:178:91` 同、`[build failed]`——与 discovery `red_phase` 逐行相同；现行树 `RecordsIngestedRun`/`ContinuesAfterOneFailure`/`ProcessesOldestFirst`/`OnlyPeriodFiltersCandidates` PASS | PASS |
| non_functional[0] | 门禁 | 见 §2 | PASS |
| non_functional[1] | 交付流程（AD-9） | 提交 `dca854d9` 锚 `feat(TASK-005): M2a …`；merge `f2079584`；discovery 记双 sha；code-simplifier「无改动」经 diff 核实（numstat 与 discovery 逐项相同）；`git worktree list` 无 `wt-TASK-005-m2a`/`pre-TASK-005` 残留 | PASS（review） |

**dev 申报核实**：(a) `outPeriods` 改法——正则改为捕获行尾、`Contains(" contract → ")` 跳过，三条既有测试断言未动、现行 PASS，与 discovery `key_findings[0]`/`decisions[0]` 相符；(b) `PriorPublishedAt` 错误分支不可达——`go tool cover` 显示 `ingest.go` 4 个未覆盖块，`git blame` 三个属既有提交（`0ad72699`/`0e2c6fc9`/`cbac1953`），**只有 `:431-433`（`PriorPublishedAt` 错误分支）属 `dca854d9`**，申报属实；(c) `string(bitemporal.OutOfOrder)` 0 处，改用 `.String()`（`internal/macro/bitemporal/classify.go:21`）。

## 2. 门禁实测（验证树 `189a9eab5f46d760fd4e4665d2d6e2b0981c5522`）

| 项 | 结果 | 门槛 |
|---|---|---|
| `GOTOOLCHAIN=local go test ./internal/hestia/... ./cmd/atlas/... -count=1` | rc=0，两包 ok | 全绿 |
| 覆盖率 | `internal/hestia` **96.6%**；`cmd/atlas` **76.6%**（含 006，高于 dev 采于 `f2079584` 的 76.4） | ≥96.6 / ≥76.4 |
| `go vet ./internal/hestia/... ./cmd/...` | 零输出，rc=0 | 零输出 |
| `gofmt -l internal/hestia cmd/atlas` | 恰 `backtest_test.go`、`crisis_test.go` | 只允许这两处 |
| 不动文件 + go.mod/go.sum（`d27791c..189a9eab`） | 0 行 | 空 |
| `Save` 函数体 grep | 0 | 0 |
| `hestia.go` import 块 `filepath` | 0 | 不 import |
| 越界申报 `git diff --stat 057a91c5 f2079584` | 恰 4 文件 = `writes`（`ingest.go` 60/2、`ingest_test.go` 200/5、`notify.go` 20/4、`notify_test.go` 16/7，与 discovery numstat 逐项相同） | 无声明外文件 |
| 注释前缀 | 新增行里不带 `M2a 的` 的 `TASK-005` 引用 0 条 | 0 |
| `find … pending/queue`（全量测试后） | 命中 0 | 空 |
| AST / reflect 守卫 | 34 / 14，两守卫 PASS | 不再新增 |
| 目标 + 回归测试 `-v` | 24 条 `--- PASS`（本任务 11 条 + 既有 7 条 `RenderP2*`/`Ingest*` 回归 + 3 守卫 + 3 其它） | — |

## 3. 变异测试（验证树内变异 + `git checkout` 还原；每个先打 diff、目标串不匹配即早退；主仓库四文件 sha256 前后一致；M2 留下的四个目录已清理，收尾 `find` 命中 0）

| # | 变异 | 结果 | 致红测试 |
|---|---|---|---|
| M1 | 契约条件改回 `!= Duplicate` | KILLED | `NoContractOnOutOfOrder` |
| M2 | 删空 `queue.dir` 守卫 | KILLED（并在 `internal/hestia/` 留下 4 个目录） | `RejectsEmptyQueueDir` |
| M3 | `runRow` 去 `contractError` 分支（记 failed） | KILLED | `ContractWriteFailure…` |
| M4 | `contractError.Error()` 加 `contract: ` 前缀 | **SURVIVED** | — |
| M5 | 写失败后不返回、照发 P2 | KILLED | `ContractWriteFailure…` |
| M6 | Revision 不填 `Supersedes` | KILLED | `RevisionContractCarriesSupersedes` |
| M7 | `IsRevision` 恒 false | KILLED | `RevisionContractCarriesSupersedes` |
| M8 | `ingestOne` 不把 `Evaluate` 结果传给 P2（`temp` 保持零值） | **SURVIVED** | — |
| M9 | `dot(unknown)` 画 🔴 | KILLED | `RenderP2CarriesSignals` |
| M10 | `EnsureQueueDirs` 错误不加 `hestia ingest: ` | KILLED | `RejectsUnusableQueueDir` |
| M11 | `SourceURL` 不取 `c.URL` | **SURVIVED** | — |
| M12 | 不打印 `<period> contract → <path>` | **SURVIVED** | — |
| M13 | `contractError` 不记 `Error` 列 | KILLED | `ContractWriteFailure…` |
| M14 | 空串守卫改为只在 `Fetch==nil` 时生效 | KILLED | `RejectsEmptyQueueDir`（`fatalFetcher` 真在守卫「I/O 之前」） |
| M15 | 信号行去 `信号 ` 前缀 | KILLED | `RenderP2CarriesSignals` |

**11/15 KILLED**。存活 4 个，实现均与 DoD/需求原文一致（我逐行核对），是测试缺用例：

- 🔴 **M8（高优先级）**：`temp = Evaluate(obs, d.Cfg.Signals)` 这条接线没有任何 ingest 级测试守卫。`TestRenderP2CarriesSignals` 只测渲染函数本身；ingest 路径上唯一断言温度的是 OutOfOrder 用例的 `温度 0/0`——那恰是零值。**该突变在生产上会让每条 P2 都打 `温度 0/0`，套件全绿。** 这是 M2a 需求 F5「P2 带四信号与综合温度」的核心接线。我用临时测试（已删）跑真实 `Ingest` 抓到 P2 第三行是 `信号 活化🔴 楼市🔴 消费🔴 信贷🟡 · 温度 0/4`（与 002 的 2025 年报 golden 一致；注意是 **0/4** 不是 0/0）。**补救只需一条断言**：给 `TestIngestWritesContractOnObservation` 加 `fakeSender` 并 `assert.Contains(sender.texts[0], "信号 活化🔴 楼市🔴 消费🔴 信贷🟡 · 温度 0/4")`。
- M4：`contract: contract:` 双前缀无守卫（DoD functional[2] 明写不要打成这样，用例只 `Contains "contract"`）。补一条 `NotContains(err.Error(), "contract: contract:")` 即可。
- M11：夹具里 `c.URL == pbocArticleURL(ArticleID)`，回落值与显式值相同，构造不出区分（需要一条 URL 形状不同的候选）。低严重度。
- M12：`<period> contract → <path>` 打印行无断言（`outPeriods` 只是跳过它）。低严重度。

**裁决理由**：DoD 列出的测试（三条原文 + OutOfOrder + `CarriesSignals` + Duplicate `温度 0/0` + boundary 追加）dev 全部落地，实现与 DoD/原文逐字一致，门禁达标 ⇒ 不构成 `task_defect`，判 VERIFIED。但 M8 的严重度高于此前任务的残留（那些是边界值，这条是主路径接线），**建议 Leader 不要等到 QA**：`verified → review_fix` 是你的合法边，可立即以 `reason_class=task_defect` 之外的口径（建议 `fix_items` 只含上面那一条断言 + M4 一条）退回 dev-m2a-a 补，或并进 QA 轮次；007 是 docs-only，不能承接。

## 4. 结论

- **VERIFIED**：8 条 DoD 全部有对应测试或证据；AD-4a 夹具与 AD-4b 八条断言逐条在；AD-14 OutOfOrder 用例真在守卫（M1 独家转红）；空 `queue.dir` 守卫真在「I/O 之前」（M14）且 `find` 判据在本任务实测生效（M2 对照）；`renderP2` 9 处调用全三参；红阶段逐行复现；dev 三条申报（`outPeriods`、`PriorPublishedAt` 不可达分支、`.String()`）全部经我独立核实属实。
- 残留：M8（高，建议立即补一条断言）、M4（低）、M11/M12（低）。

---

## 5. 复验（review_fix 第 1 轮，2026-09-06）

- **判定**：**VERIFIED**
- **判定对象**：`verify_baseline.head = fe6a80993930da8d830fcfa45185b586d43f2983`（含 fix commit `f743d2f04a400095a695833b43ecca3804e93fca`，merge 后 master）；discovery sha256 `6ade86609848a92f07a358a6763d954248b2ec92e3bb004a5566a1dee71fe820`（原内容保留 + 新增 `review_fix` 节，`commit_sha`/`merged_master_sha` 原值未动）；承接时 `assignment_epoch=1`、`rework_count=1`
- **验证树**：`../wt-verify-TASK-005-b2`（detached 于 `fe6a8099…`）
- **改动范围**：`git diff --stat 189a9eab fe6a8099` 恰 `internal/hestia/ingest_test.go` +10/-1，与 discovery `review_fix.files_modified`/numstat 一致；`implementation_changed=false` 属实（`ingest.go` 无 diff）

| fix_item | 断言（diff 核实） | 复验变异 | 结果 |
|---|---|---|---|
| M8（高） | `TestIngestWritesContractOnObservation` 注入 `fakeSender`，`require.Len(texts,1)` + `Contains "信号 活化🔴 楼市🔴 消费🔴 信贷🟡 · 温度 0/4"` | `temp = Evaluate(...)` → `_ = Evaluate(...)` | **KILLED**，独家由该用例转红 |
| M4（低） | `TestIngestContractWriteFailureKeepsRowAndSkipsP2` 加 `NotContains(err.Error(), "contract: contract:")` | `contractError.Error()` 加 `contract: ` 前缀 | **KILLED**，独家由该用例转红 |

顺带复核首轮的 M1（OutOfOrder 条件改回 `!= Duplicate`）与 M2（删空 `queue.dir` 守卫）在新树上仍 KILLED；M2 留下的四个目录已清理。

**门禁复核（验证树 `fe6a8099`）**：两包 `-count=1` rc=0；覆盖率 `internal/hestia` **96.6** / `cmd/atlas` **76.6**（不跌）；vet 零输出；gofmt 恰两处既有欠账；四个不动文件 + go.mod/go.sum 自 `d27791c` 0 行；`find … pending/queue` 命中 0；新增行无不带前缀的 `TASK-005` 引用；两条目标用例 `--- PASS`。主仓库 `ingest.go`/`ingest_test.go` sha256 变异前后一致。

**残留**：M11（`source_url` 回落值与显式值同形）/ M12（`contract →` 打印行无断言）按 Leader 裁决记录不转移。

---

## 6. 复验（review_fix 第 2 轮 · QA C1「实时契约 `extracted_at` 恒为空串」，2026-09-06）

- **判定**：**VERIFIED**
- **判定对象**：`verify_baseline.head = 589835aa2f7373c5c650072411228ddb02d012b6`（含 fix commit `b9d351b198a40775369dae42170010a1b817af04`，merge 后 master）；discovery sha256 `ffd295ca38af9be4a122cb77604f75b2f0e052d17d14abe1c7ae7e3019f00cea`（新增 `review_fix_round2` 节）；承接时 `assignment_epoch=1`、`rework_count=2`
- **验证树**：`../wt-verify-TASK-005-b3`（detached 于 `589835aa…`）
- **改动范围**：`git diff --stat 4d81143 589835aa` 恰 `ingest.go` +12 / `ingest_test.go` +27 / `CONTRACTS.md` +4（全在 `writes` 内，`writes` 已含 `CONTRACTS.md`）；`store.go` 自 `4d81143` 无 diff、`Save` grep 0 ⇒ 「不动 `Save`/`Store`」属实

| 核点（Leader 派验清单） | 核实 | 结果 |
|---|---|---|
| (1) `ingestOne` 在 `Save` 后经 `Store.Current` 回读 `IngestedAt`；err/!ok 合并一个 `contractError` 分支 | diff 逐行核实：`cur, ok, err := d.Store.Current(ctx, period, periodType)`；`if err != nil \|\| !ok { return fail("contract", contractError{err: cmp.Or(err, errors.New("just saved but not current"))}) }`；`obs.Meta.IngestedAt = cur.Meta.IngestedAt`；位置在 New/Revision 分支内、`BuildContract` 之前；`cmp` import（Go 1.24.4）；注释带 `M2a 的 TASK-005 返工 2` | ✓ |
| (2) `extracted_at == Current().Meta.IngestedAt`；实时 vs 回放除 `generated_by`/`validation.checks` 外逐键相同 | `ingest_test.go` +27 核实：`require.NotEmpty(cur.Meta.IngestedAt)` 前置 + `assert.Equal(cur.Meta.IngestedAt, c["extracted_at"])`；回放契约用 `Current + PriorPublishedAt + BuildContract{Replay:true, Report{Passed:true}}`，两侧删 `generated_by` 与 `validation.checks` 后 `assert.Equal(replay, c)`；PASS | ✓ |
| 变异「不填 `IngestedAt`」由前者独家转红 | MA（`_ = cur`）⇒ **KILLED**，唯一转红 `TestIngestWritesContractOnObservation`；MC（填 `PublishedAt`）、MD（填常量）同样独家转红 | ✓ |
| (3) CONTRACTS A10 + §B 一行 | A10 含成因（`store.go:739/:769`、`Outcome` 不回传、`ingest.go:428`、`contract.go:113`）、修法（不动 `Save`，`Current` 回读）、守卫、「三道都没核」与零守卫说明；§B 「实时 vs 回放契约」行记 diff 仅 `generated_by`/`checks`、覆盖率 96.6 不变。两处 `TASK-005 返工 2` 不带 `M2a 的` 前缀是 CONTRACTS 正文而非 Go 注释，前缀规则不涉 | ✓ |
| (4) 门禁 | 两包 `-count=1` rc=0；`GOTOOLCHAIN=local go test ./internal/hestia/ -cover`（DoD 指定仪器）打 **96.6**；`cmd/atlas` 76.6；vet 零输出；gofmt 恰两处；四个不动文件 + go.mod/go.sum 自 `d27791c` 0 行；`Save` grep 0；AST 34 / reflect 14；`find … pending/queue` 0 | ✓ |
| 覆盖率（AD-17，`plan.md:70`） | 我从 coverprofile 算精确值 **2743/2841 = 96.5505%**，与裁决数字逐位相同；DoD 仪器打 96.6 ⇒ 门槛成立；`go tool cover -func` 总计 89.8% 是两包合并口径。新增未覆盖块 `ingest.go:436-438` 即 `Current` 回读失败分支（`IngestDeps.Store` 是 `*Store` 不可注入，`Save` 成功后无法让同一 Store 的下一条 `Current` 失败），与 `:443-445` `PriorPublishedAt` 错误分支同类，如实登记、不补测试抬数字 | ✓ |
| 红阶段 | 验证树把 `ingest.go` 退回 `4d81143`（修复前实现）+ 现行测试：`ingest_test.go:1402` `expected: "2026-09-06T04:39:49.826261Z" actual: ""`，`:1420` 逐键比对同样 FAIL 于 `extracted_at`——与 discovery `red_phase` 同形（时间戳为各自运行时刻） | ✓ |

**变异（验证树内变异 + `git checkout` 还原，主仓库两文件 sha256 前后一致）**

| # | 变异 | 结果 |
|---|---|---|
| MA | `obs.Meta.IngestedAt = cur.Meta.IngestedAt` → `_ = cur` | KILLED（独家 `WritesContractOnObservation`） |
| MB | 删掉 `err/!ok` 失败分支 | **SURVIVED**（不可达分支，dev 与 AD-17 已申报） |
| MC | 填 `cur.Meta.PublishedAt` | KILLED（独家） |
| MD | 填常量 `"2026-01-01T00:00:00Z"`（第一版因 `cur` 未使用编译失败、判为无效变异体后重做） | KILLED（独家） |
| ME | 失败分支恒成立 | KILLED（29 条转红） |
| MF | `Current` 用错 `period_type` | KILLED（29 条转红） |
| MG | `SourceURL` 不取 `c.URL` | **SURVIVED**（首轮 M11 同形：夹具回落值与显式值相同） |

5/7 KILLED；两个存活均为已知且已申报的不可观测项。

- 结论：QA C1 的修复实现正确、守卫真在（三个「填错/不填」变异各被独家杀死）、门禁达标、CONTRACTS 记录完整。**VERIFIED**。
