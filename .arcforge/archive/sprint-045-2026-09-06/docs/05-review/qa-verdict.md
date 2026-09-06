# QA Verdict · Sprint M2a（Hestia 契约队列与信号快照）

- **审查者**：qa-m2a（两轮：常规 + 跨视角对抗）
- **审查对象**：`git diff d27791c695e8ebd0fd5d54c9161782f52d9b12cb 4d81143487187faf6fc324f7fdc32ad80a0f6ffd -- internal cmd/atlas configs`（17 文件 +1804/−24）+ `internal/hestia/CONTRACTS.md` `## Sprint M2a` 段
- **审查树**：`../wt-qa-m2a` detached @ `4d81143487187faf6fc324f7fdc32ad80a0f6ffd`（master HEAD，与 Leader 派发消息一致）
- **输入**：TASK-001～007 的 DoD / discovery / 验证报告（含变异存活登记）、AD-1～16、需求计划 `2026-09-05-hestia-m2a-contract-queue.md`（⚠️ `specs/` 目录**没有** M2a 的 design 文档，最新是 m1d-cutover；派发消息说的「同目录 specs 的 design 文档」不存在，本次以计划文档 + AD 为准）
- **结论先行**：**REJECT**（1 CRITICAL · 2 WARNING · 8 SUGGESTION）。CRITICAL 只有一条，修复量约 5 行 + 1 条断言；其余全部可挂账。

## 0. 门禁自采（审查树 `4d81143`，`GOTOOLCHAIN=local`，与 §B 登记逐项相同）

| 项 | 自采 | §B 登记 |
|---|---|---|
| `go test ./internal/hestia/... ./cmd/atlas/... -count=1` | 两包 `ok` | — |
| `internal/hestia` 覆盖率 | **96.6** | 96.6 |
| `cmd/atlas` 覆盖率 | **76.6** | 76.6 |
| `go vet` | 零输出 | 零输出 |
| `gofmt -l` | 恰 `backtest_test.go`、`crisis_test.go` | 同 |
| 四不动文件 + `go.mod`/`go.sum` 自 `d27791c` | 0 行 | 空 |
| `store.go` 删除行 / `Save` 函数体 ± | 0 / 0 | 0 / 0 |
| `find internal/hestia cmd/atlas -type d \( -name pending -o -name queue \)` | 空 | 空 |

## 1. 第一轮：常规 Code Review

### CRITICAL

**C1｜实时路径写出的契约 `extracted_at` 恒为空串**（`internal/hestia/ingest.go:428`，根因 `store.go:769` + `contract.go:113`）

- **机制**：`Store.Save(ctx, obs Observation, …)` 按**值**收 `obs`，在函数体内 `obs.Meta.IngestedAt = s.now()…`（`store.go:769`）给自己那份副本赋值；`Outcome` 只有 `Verdict`/`Table`，时间戳不回传。`ingestOne` 在 `Save` 之后用**调用前的** `obs` 建 `ContractInput{Obs: obs, …}`（`ingest.go:428`），`BuildContract` 取 `in.Obs.Meta.IngestedAt`（`contract.go:113`）⇒ 空串。
- **实证**（临时探针 `_test.go`，跑完已删，`git status` 干净）：真跑 `Ingest` 后读 `pending/2025-12-annual.json`：`"extracted_at": ""`；同一库 `s.Current(...)` 返回 `Meta.IngestedAt = "2026-09-06T03:23:53.015307Z"`。同一观测**实时契约 vs 回放契约**逐行 diff：除预期的 `generated_by`（`/replay` 后缀）与 `checks`（回放为空）之外，**唯一差异就是 L11 `extracted_at`**——`data`/`absent_fields`/`thresholds`/`source_url` 逐字相同。
- **为什么是 CRITICAL**：① 契约是本 Sprint 的交付物，17 个顶层字段之一在 **100% 的生产契约**上为空；② `contract.go:6-8` 写的设计目标「done/ 里的文件能与重放结果 cmp」被它直接打破——回放（`Current` 读库）有值、实时没有；③ CONTRACTS §B 的两份回放样本 `extracted_at` 都有值，**M3 若照样本写消费者，生产上拿到的形状与样本不同**；④ 零测试守卫：`TestBuildContractMapsFields` 的夹具自带 `IngestedAt`，ingest 级用例只断言 `period`/`generated_by`/`is_revision`/`config_version`。
- **成因归属**：需求原文 TASK-005 的接线片段（计划文档 `:1396`）就是 `ContractInput{Obs: obs, …}`，假设 `obs` 带 `IngestedAt`；dev 照抄、验证者按 DoD 逐字核对、007 样本走回放路径——三道都没碰到实时路径的这个字段。**这不是个人失误，是「需求片段的隐含前提没人验」**（与 AD-4/AD-14 同族）。
- **建议修复**（不动 `Save` 函数体，符合本 Sprint 冻结约束）：New/Revision 分支里 `Save` 之后 `cur, ok, err := d.Store.Current(ctx, obs.Meta.Period, obs.Meta.PeriodType)`（New/Revision 时 current 行必是刚存的这行），`in.Obs.Meta.IngestedAt = cur.Meta.IngestedAt`；`!ok`/`err` 走 `fail("contract", contractError{…})`。理由：`Current` 已是回放路径的来源，实时与回放共用同一真相源 ⇒ cmp 目标成立；代价一次索引读。**测试**：`TestIngestWritesContractOnObservation` 加 `assert.NotEmpty(c["extracted_at"])` 且等于 `s.Current(...).Meta.IngestedAt`；变异「不填 `IngestedAt`」须由它独家转红。**替代（本 Sprint 不取）**：`Outcome` 加 `IngestedAt` 字段——要改 `Save` 函数体，冻结。
- **CONTRACTS 落点**：§A 加 **A10**（需求接线片段的隐含前提：`Save` 按值收参不回填）。

### WARNING

**W1｜`Store.Current` 的 `!rows.Next()` 出边返回裸 `rows.Err()`，无 `hestia store current` 前缀**（`store.go:476-477`）

- DoD 003 error_handling[0] 原文：「`Current`/`PriorPublishedAt` 的查询错误带 `hestia store current`… 前缀」。`QueryContext` 失败与 `scanObservation` 失败都包了前缀，唯独迭代错误（如 ctx 在 `Query` 与 `Next` 之间被取消）裸返。`TestStoreCurrentAndPriorErrorsCarryPrefix` 用关库触发的是 `QueryContext` 那条边，不到这里。dev-b 终检子代理已报同一条。
- 影响低（只在 ctx 中断时出现），但与 M1.5 W1（`HealthSummary` 的 `rows.Err()`）同族、且违反 DoD 字面。修：`if err := rows.Err(); err != nil { return …, fmt.Errorf("hestia store current %s/%s: %w", …) }; return Observation{}, false, nil`。

**W2｜`LoadConfig` 不拒空 `config_version`，契约 `thresholds.config_version` 会写 `""`**（`config.go:247` 附近的 `validate()`）——Leader 要我裁的那条

- **裁决：该拒。** 理由：`ConfigVersion` 在生产代码的**唯一**消费点就是契约（`contract.go:120`），字段注释自述用途是「这期用的是哪版配置在契约里一眼可见」——空串使它失去全部意义；`validate()` 已用同一逻辑拒空 `queue.dir`（配置错误在任何 I/O 之前拦下）；仓库 `configs/hestia.yaml` 有值、`ingestCfg` 设 `"test"`，**生产不受影响**，受影响的是 007 那类临时 yaml 与未来第二份配置。
- **成本**（实测）：`config_test.go` 13 个内联 yaml 只有 4 个写了 `config_version`；`cmd/atlas/hestia_test.go` 8 个只有 2 个；`thresholds_test.go` 与 `writeHestiaYAML*` helper 也要补——约 **15 处夹具、横跨两包**。
- **处置建议**：不并进本轮 review_fix（不是需求项、零生产影响、改动面比 C1 大三倍）；记 CONTRACTS §C，作 M2b 前置的独立小任务（`packages` `./internal/hestia` + `./cmd/atlas`，与 001 同形）。Leader 若坚持本轮处理，落 TASK-001（`writes` 已含 `config.go`/`config_test.go`/`hestia_test.go`）。

### SUGGESTION

| # | 位置 | 描述 | 处置 |
|---|---|---|---|
| S1 | `queue.go:37` | `WriteContract` 的 `MkdirAll(pending)` 失败错误串报 `dir` 而非 `dir/pending`，与 `EnsureQueueDirs` 逐子目录报路径的粒度不一（dev-b 终检已报） | 挂账 §C，顺手修时改 `pending` |
| S2 | `contract_test.go:41` / `notify_test.go:35` | `f` 与 `f64` 同构（`func(float64) *float64`），同包两份（dev-b 终检已报） | 挂账，下次动 `contract_test.go` 时删 `f` 改用 `f64` |
| S3 | `snapshot.go:98`（M2a 新暴露面） | `writeAtomic` 的 tmp 名固定为 `<path>.tmp`。M1d 只有快照一个写者；M2a 起 launchd 的 `ingest` 与运维手工 `contract emit` 可能**同期次并发**写 `pending/<同名>.json`：A 写 tmp → B 截断重写同一 tmp → A rename 拿到 B 的半个文件。概率低（要同秒撞同期），但「原子写」的承诺在这个场景不成立 | 挂账 §C；修法 `os.CreateTemp(pending, name+".*.tmp")` |
| S4 | `notify.go:64` | 注释「temp 零值四个 unknown」不准：`Signal` 零值是 `""` 不是 `"unknown"`，画 ⚪ 靠的是 `dot` 的 default 分支。输出正确，注释会误导后来者拿 `== SignalUnknown` 判零值 | 挂账，改注释或让 `dot` 的 default 注明「含零值」 |
| S5 | `contract.go:93` / `backfill_search.go:43` | 文章 URL 路径前缀 `/goutongjiaoliu/113456/113469/` 两处字面量；央行改栏目 ID 时要改两处 | 挂账；不阻断（M2a 只加了一处，抽公共常量会碰 `backfill_search.go`） |
| S6 | `cmd/atlas/hestia.go:386` | `hestiaPeriodTypeRE` 与 `types.go` 的 `validPeriodTypes` 是同一枚举的第二份；cmd 层拿不到非导出 map，且导出面守卫是精确集合，所以这次复制是合理代价 | 接受；§C 记一句「加第六个 period_type 要改两处」 |
| S7 | `signals_test.go:392` / `contract_test.go:26` / `queue_test.go:207` | 三份 Context Checkpoint 注释分别写守卫「27 项」「32 项」「34 项」——是各任务当时的中间值，终值 34。读者按任一份去数会对不上（Leader 派发消息已列） | 接受为历史记录；建议 001/002/003 那两份改成「当时 N 项，终值见 store_test.go」 |
| S8 | `signals.go` `evalCredit` | `bill/total*100`：回填库 76 行中 `loan_bill_mom` 有两行为负（2020-07：−1021/2645 ⇒ −38.6% ⇒ green），`loan_corp_total_*` 无 ≤0 行。负票据 ⇒ 绿在语义上说得通（票据在缩），但方案报告 4.7 没写负值口径 | 记 §C 供 4.7 补口径；实现不改 |

### 已核实无问题（Reality Checker 要求每条 PASS 附证据）

- **P2 信号接线**（005 review_fix M8）：`TestIngestWritesContractOnObservation` 断言 `温度 0/4`，复验变异 KILLED（005 验证报告 §5）；我在探针里看到同一路径 P2 三行齐全。
- **OutOfOrder 不写契约**（AD-14）：`ingest.go` 条件 `Verdict ∈ {New, Revision}`，`TestIngestNoContractOnOutOfOrder` 独家杀 M1。
- **`source_url` 实时/回放同形**：`discover.go:79` 经 `ResolveReference` 产绝对 URL，与 `pbocArticleURL` 拼法一致，探针 diff 该行无差异——005 存活变异 M11「构造不出区分」的判断成立。
- **`caliber_version`/`extractor` 实时已填**：`parse.go:251` 填 `caliberFor(period)`；探针 `extractor="rule@v2"`。
- **顶层 17 键序**：`TestContractJSONTopLevelKeyOrder` 扫两空格缩进；`MarshalIndent` 对自定义 `MarshalJSON` 输出会 compact 后重新缩进，`data` 段缩进正确（探针输出 L99-L156 可见）。
- **写失败无残留 / P1 照发 / outcome 保持 ingested**：`TestIngestContractWriteFailureKeepsRowAndSkipsP2` 八条断言在；`status.go:68-72` 会打 `stage=contract  <error>`，运维在 `hestia status` 看得到。
- **空 `queue.dir` 守卫在 I/O 之前**：005 M14 用 `fatalFetcher` 证明；M2 删守卫后 `find` 命中 4 条目录（AD-5 预言实测成立）。
- **`PriorPublishedAt` 字典序比较合法**：`types.go:235` `publishedAtRE` 强制 `YYYY-MM-DD`，`MAX()`/`<` 的字典序即时间序。
- **安全**：`WriteContract` 的文件名由 `period`/`period_type` 拼——前者 `Save` 时经 `periodRE`，后者经 `validPeriodTypes`；emit 路径两个 flag 经正则；无路径穿越面。无新增依赖。

### Leader 派发消息里「待我打包/裁定」的输入——逐条处置

| 输入 | 裁定 |
|---|---|
| dev-b 终检 ① `f`/`f64` | S2，挂账 |
| dev-b 终检 ② `Current` 的 `rows.Err()` 无前缀 | **升 W1**（DoD 字面 + M1.5 W1 同族） |
| dev-b 终检 ③ `WriteContract` 报 `dir` | S1，挂账 |
| 三处注释写中间值 27/32/34 | S7，接受为历史记录 |
| 004 存活 Q4（原子写换普通写）/ Q8（权限位） | 接受：单进程测试构造不出「读到半个文件」；Q8 无消费者依赖权限位。§C 已记 |
| 005 存活 M11（`source_url` 回落与显式同形）/ M12（打印行无断言） | 接受：M11 我用生产链路（`ResolveReference`）核过确实同形，不是测试懒；M12 打印行是人看的，`outPeriods` 已显式跳过它 |
| `PriorPublishedAt` 错误分支在 ingest 路径不可达 | 接受：Revision 分支里 `Save` 刚成功就查同一库，唯一可达方式是 ctx 取消——那属于 W1 同一族的「迭代/中断错误」，不单开 |
| `thresholds.config_version` 为 `""` 是否该让 `LoadConfig` 拒 | **W2：该拒**，但建议作独立小任务不并进本轮，理由与成本见 W2 |
| AD-4b / AD-14 人类已拍板 | 未重开；本轮全部发现均在其之外 |

## 2. 第二轮：跨视角对抗

- **codex CLI**：`codex --version` 秒回 `codex-cli 0.139.0`；`codex exec --sandbox read-only` 秒回 **`You've hit your usage limit … try again at Sep 10th, 2026`**，未产出任何审查。⚠️ 与 PENDING #5 记的「30 分钟零输出」是**不同形态**——这次探测「可运行」通过了，卡在配额。建议 PENDING #5 补一句：探测要同时验「有配额」（`codex exec` 一条 `echo ok` 级 prompt 看 rc）。退回纯 Claude 跨视角。
- **三个 lens**：Leader 明令不 spawn 子代理（本 Sprint 两个子代理各挂 2.9h），故由本体分三遍独立过 diff，每遍只带该视角的问题清单，发现如下。

| Lens | 独立发现 | 对 C1 的判定 |
|---|---|---|
| **Skeptic**（逻辑漏洞、边界、隐含假设） | 找到 C1（顺着「`Save` 按值收参，谁给 `obs` 回填时间戳」这条隐含假设追出来）；核了 `source_url`/`caliber_version` 两个同类假设（无问题）；S3 并发 tmp；S8 负票据 | **high**：主路径、100% 命中、零守卫 |
| **Architect**（设计、扩展性、依赖） | C1 的根因是 `Save` 的返回契约（`Outcome`）缺时间戳——`Current` 作为「同一真相源」是本 Sprint 内最不侵入的补法；S5/S6 两处枚举/字面量复制是分层代价，接受；`contractError` 与 `notifyError` 同形但各自命名比泛化更可读，不建议合并 | **high**：违反 `contract.go` 自述的 cmp 设计目标 |
| **Minimalist**（过度设计、可删代码） | 无过度设计：`BuildContract` 收整个 `Config` 是需求签名，不改；S1/S2/S4 三处可删/可简；C1 修法 5 行以内 | **high**：修复代价与影响不成比例，没有理由挂账 |

**三视角对 C1 一致判 high-severity ⇒ 第二轮 verdict：REJECT**（非 CONTESTED）。

## 3. 最终 verdict：**REJECT**

- 阻断项：**C1**（一条）。
- 门禁：全部达标（§0）。
- 契约文档：CONTRACTS §A/§B/§C 与代码相符；§A 需追加 A10（C1 的成因）。

## 4. 建议处置表（供 Leader 执行；review_fix 是 Leader 专属边）

| 项 | 建议动作 | 任务 | `reason_class` | `fix_items` 摘要 |
|---|---|---|---|---|
| C1 | **开 review_fix** | TASK-005（`writes` 已含 `ingest.go`/`ingest_test.go`） | `task_defect`（交付物主路径缺陷；DoD 未自相矛盾，故不是 `dod_defect`——且 005 已有一轮 `dod_defect`，再记会触发「累计第 2 次转 blocked_human」的误熔断） | ① `ingestOne` New/Revision 分支 `Save` 后经 `d.Store.Current` 取 `IngestedAt` 填进 `in.Obs.Meta`，失败走 `contractError`；② `TestIngestWritesContractOnObservation` 加 `extracted_at` 非空且等于 `Current` 值的断言，变异「不填」独家转红；③ 不改 `Save`/四不动文件；门禁数字 merge 后重采 |
| W1 | 并进同一 review_fix（dev 先 `update --json-field writes` 追加 `store.go`/`store_test.go`，003 已 verified 无 scope 冲突）；或 Leader 选挂账 §C | TASK-005 | 同上 | `Current` 的 `rows.Err()` 包前缀 + 一条测试（ctx 已取消再 `Current`） |
| W2 | **不并进本轮**；CONTRACTS §C 记「`LoadConfig` 应拒空 `config_version`（QA 裁决），M2b 前独立小任务，约 15 处夹具」 | 新任务 | — | — |
| S1–S8 | CONTRACTS §C 挂账（M2 首批打包处理，沿 M1.5 C4 形态）；S7 可在 C1 的 review_fix 里顺手改注释（同文件不新增 writes 的只有 `ingest_test.go`，其余三份不在 005 writes 内，不建议顺手） | — | — | — |
| A10 | 随 C1 修复进 CONTRACTS §A（由 Leader 或 007 记录员补） | TASK-007 docs | — | — |
| PENDING #5 | 补「codex 配额」形态 | — | — | — |

## 5. 复审计划

review_fix 合入 master 后，我复核三件事：① 探针重跑（实时 `extracted_at` 非空且 == `Current`）；② 变异「不填 `IngestedAt`」KILLED；③ 门禁 §0 八项不变。三项过 ⇒ 改 verdict 为 PASS。

---

## 6. 复审（review_fix 2 · 2026-09-06）

- **复审对象**：master `589835aa2f7373c5c650072411228ddb02d012b6`（fix commit `b9d351b1`，merge 后；`git diff --stat 4d81143 589835aa` 恰 `ingest.go` +12 / `ingest_test.go` +27 / `CONTRACTS.md` +4，无删除行）；复审树 `../wt-qa-m2a-r2` detached @ 同 sha；test-m2a-b 复验 VERIFIED 在先。
- **修复形态核对**：`ingestOne` New/Revision 分支在 `Save` 后经 `d.Store.Current` 回读，`obs.Meta.IngestedAt = cur.Meta.IngestedAt`（`ingest.go:439`），取不到走 `contractError`；不动 `Save`；`TestIngestWritesContractOnObservation` 追加「`extracted_at == Current.IngestedAt`」与「实时 vs 回放契约除 `generated_by`/`validation.checks` 外逐键相等」两组断言；CONTRACTS §A 加 A10、§B 加一行。与 §4 建议 ①②③ 逐项相符。

| §5 项 | 结果 | 证据 |
|---|---|---|
| ① 探针重跑 | **PASS** | 我自己的临时探针（跑完已删，`git status` 干净）：`extracted_at="2026-09-06T04:43:03.673081Z"`，`Current.IngestedAt` 同值，`equal=true` |
| ② 变异 KILLED | **PASS** | 把 `ingest.go:439` 改成 `_ = cur`（语法合法、`git diff --stat` 1 文件 ±1）⇒ `--- FAIL: TestIngestWritesContractOnObservation`，整包 FAIL；`git checkout` 还原后 status 0 行、该用例 `ok` |
| ③ 门禁八项 | **PASS** | 两包 `-count=1` ok；覆盖率 `internal/hestia` **96.6** / `cmd/atlas` **76.6**；vet 零输出；gofmt 恰两处既有；四不动文件 + `go.mod`/`go.sum` 0 行；`store.go` 删除行 0；`find pending/queue` 空；`hestia.go` 无 `path/filepath` import |

- **备注（不阻断）**：精确覆盖率 96.5505%，`go test -cover` 显示 96.6（Leader 按 AD-17 以 DoD 仪器为准）；新增 `!ok` 分支（「just saved but not current」）在 New/Revision 下构造上不可达，与 `PriorPublishedAt` 错误分支同族，验证者已如实登记。`cmp.Or` 需 Go ≥ 1.22，`go.mod` 为 1.24.4。
- **W1 / W2 / S1–S8**：按 Leader 处置挂账（§C / final-report），本次不复核。

## 7. 最终 verdict（复审后）：**PASS**

C1 已修复并由独立探针 + 变异双重证实；无 CRITICAL/WARNING 未处置项（W1/W2 经 Leader 裁决挂账，不在本轮修复范围）；门禁不变。
