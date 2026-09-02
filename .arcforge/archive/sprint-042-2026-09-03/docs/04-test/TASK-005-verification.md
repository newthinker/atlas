# TASK-005 验证报告 —— 分部门段与合计句按口径路由，不再拒绝整篇

- **验证者**：test-m1c4-a
- **判定对象**：`master @ d50835b533cb57e146ee30be88a68994381b4ab6`（= `verify_baseline.head`）
- **隔离对**（A/B 与变异用）：base `adc0fcafe3cd8cec9cd5eda12f6b4d904c7b7f38` / post `189ca5e743d47ff33ca81f7edd549e82352b78a3`
- **assignment_epoch**：1
- **改动面**：8 文件、1905 增 / 272 删
- **结论：VERIFIED**

---

## 0. 基线核对与两类数字分树

| 项 | verify_baseline | 判定时实测 | 一致 |
|---|---|---|---|
| `head` | `d50835b533cb…` | `git rev-parse HEAD` 同值 | ✅ |
| `discovery_sha256` | `c0f415eaee1a…` | `shasum -a 256` 同值 | ✅ |

- **隔离对父子关系实测**：`189ca5e` 的 parent 确为 `adc0fca`；`d50835b` 的 parents = `adc0fca` + `189ca5e`（merge）。
- **两树间 diff 恰为本任务 8 个文件**，与 `git show --numstat 189ca5e` 逐个相同，合计 **1905 增 / 272 删**，无第三方改动混入。
- 门禁类钉 `d50835b`（交付态），A/B 与变异钉隔离对——**未把 A/B 钉在 `verify_baseline.head` 上**。

---

## 1. done_criteria 覆盖矩阵（8 条）

| # | 维度 | 完成标准（摘要） | 证据 | 判定 |
|---|---|---|---|---|
| F0 | functional | `sectorCaliber` 三值 + `sectorCaliberOf` 取代 `checkSectorCaliber`；`pick()`；🔴 **`cal` 每段求一次、不得在循环内** | §2 | **PASS** |
| F1 | functional | 三处按口径写列；🔴 **合计句也必须路由**；诊断门按 dev 订正保持 `!hasCum` | §3、§6 | **PASS** |
| F2 | functional | 四条测试 + 🔴 **V1/V2/V3 变异矩阵，各自只杀自己那条** | §5 | **PASS** |
| B0 | boundary | TSF 双口径各收一条；作用域切分不动；多于一条仍报错；🔴 **形态②处置** | §4 | **PASS** |
| B1 | boundary | golden 从「一篇」扩到「形态覆盖」，`NotContains` 两条不能省 | §4 | **PASS** |
| E0 | error_handling | A/B **全量 diff**；🔴 **逐篇差集为空**（硬判据）；语料绝对路径 + `--allow-incomplete` | §7 | **PASS** |
| N0 | non_functional | 门禁全过 + 覆盖率 ≥96.1%，**测自 merge 后 master** | §8 | **PASS** |
| N1 | non_functional | 交付流程，**merge 先于 `dev_done`** | merge `02:30:55Z` < `dev_done` `02:33:33Z`（早 2 分 38 秒）；commit 锚定 `feat(TASK-005):` | **PASS** |

---

## 2. F0：`cal` 每段一次（TASK-011 防线的分析基座）

**判据 1、2 机器核实**——`sectorCaliberOf` 在 `extract.go` 的**非声明调用点恰好两个**：

```
extract.go:693  在 extractDepositSection，缩进=1，包裹它的 for 循环：无
extract.go:744  在 extractLoanSection，  缩进=1，包裹它的 for 循环：无
```

我用脚本从每个调用点向上回溯到函数头、收集缩进更浅的 `for` 行——**两处都为空** ⇒ 不在循环内 ✅。

**判据 3**：dev 未改成逐项判，也未走澄清环（不需要）。

### 🔴 额外验证：这条守卫本身有没有区分力（V5，超出 DoD）

Leader 指出这是 TASK-011 整条防线的基座，故我补了一个变异：把 `pick()` 改成按条目名奇偶分列（模拟「逐项判」，制造同段两族共存）。

```
--- FAIL: TestSectorCaliberIsComputedOncePerSection/pboc-2023-08-monthly.html
    Messages: 同一分部门段内出现了两族共存（ytd 2/4、mom 2/4）⇒ 口径是逐项判的，
              而 TASK-011 的 |ytd| ≥ |mom| 建立在「一段一个口径」上
```

⇒ **该守卫确实抓得住「换掉分析基座」这件事**，非重言式。

---

## 3. F1：合计句路由 + `2022-07` 实抽（我自己跑，不采信 discovery）

DoD 明确要求「贴出实际抽取结果，不要只说测试通过」。我在**两棵树**上各跑一次 `Parse`，输入是真语料
`data/hestia-backfill-2026-08-14/articles/2025092212552757258.html`（2022-07 金融统计数据报告）：

| 树 | `Parse` 结果 |
|---|---|
| base `adc0fca` | `err = hestia: 人民币存款期内合计 not found among 2 candidate sentence(s) [7月份/人民币 7月份/外币]…` ⇒ **整篇被拒** |
| post `189ca5e` | **`err = nil`**，25 个字段 |

post 侧实际抽取（节选，DoD 点名的两条加粗）：

```
deposit_flow_mom       = 447        ← 🔴 DoD 要求必须实抽出来
deposit_household_mom  = -3380      deposit_corp_mom   = -10400
deposit_fiscal_mom     = 4863       deposit_nbfi_mom   = 8045
loan_flow_ytd          = 143500     ← 🔴 与上面那条共存
loan_hh_short_mom      = -269       loan_hh_mlt_mom    = 1486
loan_corp_total_mom    = 2877       loan_bill_mom      = 3136
deposit_balance = 251.1   loan_balance = 207.03   m2 = 257.81   m1 = 66.18   m0 = 9.65
```

**共存性核实（Leader 特别要求的那条）**：

```
deposit_flow_mom 存在 = true   loan_flow_ytd 存在 = true   两者共存 = true ✅
对侧: deposit_flow_ytd 存在 = false ✅   loan_flow_mom 存在 = false ✅
```

⇒ 存款节只有「7月份…447亿元」走 `_mom`，贷款节有「1-7月…累计」走 `_ytd`，**两侧各判各的**，
且各自的对侧列**一个都没写**。这正是「口径是按族的、不是按整篇的」这条设计前提的实证。
与 discovery 报的 25 字段及各数值逐个相同。

---

## 4. B0/B1：形态②处置与 golden 覆盖

`TestTSFFlowArticleCaliberEdgeCases` **四格全绿**，恰好对应 DoD 要求的判据：

| 子测试 | 对应 DoD 要求 |
|---|---|
| 两族都没有_必须响亮失败 | 「两族都没有才是错误，措辞同样具体」 |
| 当月族两条_仍然硬失败 | 「挑到多于一条仍然报错，不许就近取一条」 |
| **分项抽到一部分_必须报模板缺口而不是静默跳过** | 「抽到一部分又缺另一部分 ⇒ 仍然报错」 |
| **整族缺席_正常放行只写总量** | 「某族作用域内一条分项都没有 ⇒ 只写总量，正常」 |

`TestSectionFailsLoudlyWhenNoFlowSentenceAtAll` 存贷两侧均绿（不得静默产出半份 `Values`）。

**golden 已从「一篇」扩到形态覆盖**（DoD boundary[1] 的硬要求），实测 4 格：

```
pboc-2020-04-monthly.html / 形态①：全篇只有当月句                        PASS
pboc-2023-08-monthly.html / 形态②：合计累计 + 分部门当月                  PASS
pboc-2022-08-monthly.html / 形态②（另一期）：贷款合计累计、存款合计当月    PASS
pboc-2025-08-monthly.html / 阴性：全篇累计，不得被路由误伤                 PASS
```

用的都是**既有 fixture**，未另行复制（符合「同一形态不要有两份会分叉的样本」）。

---

## 5. F2：变异矩阵（我自己在隔离副本上重跑）

harness 作用在 `../wt-t005-post`；还原**一律用 `cp` 自备份**（dev 报告的坑：隔离树里
`git checkout --` 会还原成 base 版本、把实现一起擦掉）。每格过 `gofmt -e` 语法闸 + 打印变异 diff
逐字核对 + `setup failed|build failed` 有效性闸。

**主工作区指纹前后一致**：`extract.go` = `b3fcc0ce…`、`profiles.go` = `f8f66c4c…`，代码目录改动 **0 行**
（与 discovery 报的两个 sha256 逐字符相同）。

### 对照组：636 PASS / 0 FAIL

### 三个变异全部 KILLED

| 变异 | 顶层 FAIL 数 | 含指定守卫 |
|---|---|---|
| **V1**（无前缀分支 报错 → `caliberCurrent, nil`） | **2** | `TestSectorCaliberOfStillRefusesWhenUnreadable` ✅ + `TestSectorCaliberErrorIsDistinguishable` |
| **V2**（口径判定反向） | **30** | `TestSectorCaliberOfReportsInsteadOfRefusing` ✅ |
| **V3**（`pick()` 的 `caliberCurrent` → `ytdField`） | **12** | `TestMixedCaliberReportRoutesEachSectionToItsOwnColumns` ✅ + `TestNameFieldPickSelectsColumnByCaliber` ✅ |

三格的 FAIL 数与 discovery 报的（2 / 30 / 12）逐个相同。

### 🔴 列唯一性成立（每条守卫只被自己那条杀）

| 守卫 | V1 | V2 | V3 |
|---|---|---|---|
| `TestSectorCaliberOfStillRefusesWhenUnreadable` | **✗** | · | · |
| `TestSectorCaliberOfReportsInsteadOfRefusing` | · | **✗** | · |
| `TestNameFieldPickSelectsColumnByCaliber` | · | · | **✗** |
| `TestMixedCaliberReport…`（golden） | · | **✗** | **✗** |

**dev「行不唯一」的解释我接受且已复核**：golden 被 V2 和 V3 同时杀是**应该的**——V2 把每一篇的口径判反，
golden 的 `NotContains` 正该因此转红。V2/V3 是语义级变异（改的是路由本身），与字符级变异不同，
行不可能只有一个 FAIL。**dev 如实报告了做不到严格双向对角线，没有粉饰**。

### 🔴 V3 杀掉 golden 的**原因**确认（Leader 点名要核的）

不只看「转红了」，我查了失败详情——`extract_test.go:2263`：

```
Error:    map[...] should not contain "loan_hh_short_ytd"
Test:     TestMixedCaliberReportRoutesEachSectionToItsOwnColumns/pboc-2020-04-monthly.html/形态①
Messages: 🔴 住户短期贷款 的值落进了 loan_hh_short_ytd —— 口径错配的两个值都在合法量级内，
          下游没有任何闸门拦得住。这正是本迭代要防的那件事
```

⇒ **正是 `NotContains(FieldLoanHHShortYTD)` 那条断言转红**，不是别的断言顺带红的。
⇒ 本迭代唯一新风险（当月值写进累计列）的现场守卫**确有区分力**。

`TestNameFieldPickSelectsColumnByCaliber` 的失败是单元级的
（`expected: "deposit_household_mom" / actual: "deposit_household_ytd"`），与 golden 形成两级守护。

---

## 6. 🔴 DoD 偏离的独立复核（Leader 只验了理由①，要我补验理由②）

dev 未按 DoD 原文把诊断门下移，保持 `if !hasCum`（`extract.go:387` 实测确认）。

### 理由①（Leader 已验）：留在原位不会误报

我复跑确认：`TestCumulativeFlowDistinguishesUnknownPrefixFromNoCumulativeSentence` 的 A 格
（真语料 2020-04 纯当月）`require.Empty(unrecognisedPeriodPrefixes(...))` 通过。

### 🔴 理由②（Leader 要我独立复核）：下移会制造静默数据丢失 —— **确证**

我做了 **V4 变异**：把诊断门改成 DoD 原文要求的 `!hasCum && !hasCur`（即「下移」），
然后用同一份输入跑探针，**对照原实现**：

输入（当月合计句 + 一条前缀不被认识的累计句）：
```
4月末，月末人民币贷款余额201.66万亿元，同比增长10.9%。
4月份人民币贷款增加6454亿元，同比少增8231亿元。
今年以来，人民币贷款累计增加18.7万亿元。
```

| 实现 | 结果 |
|---|---|
| **下移后**（DoD 原文） | **未报错**，走当月路径 `cumulative=false`、命中前缀 `"4月份"`、值 `6454亿元` ⇒ 🔴 **那句 18.7 万亿被静默丢弃，无任何诊断** |
| **原实现**（dev 保留的） | 响亮报错：`失败的成因是**期次前缀不被识别**，不是本节没有合计句: …前缀 今年以来 不在 periodAlt 里…` |

⇒ **理由②成立**：DoD 原文要求的「下移」确实会制造静默数据丢失，而那正是 R3 的失效方式、
也正是这段诊断存在的全部理由。**Leader 的裁决（认可 dev 偏离）是对的，DoD 原文是错的。**

**且该守卫是精确的**：V4 下 B 格转红，而 A、A2、C 三格**保持绿**——它只钉住这一个决定，不误伤。

⚠️ 附带发现：**`boundary[1]`** 的正文里仍留着 DoD 原文那段「诊断门**必须下移到两族都空之后**」，
与 `functional[1]` 里 dev 的订正**直接矛盾**。本次按 Leader 裁决判，但**该矛盾仍留在任务文件里**，
后续若有人只读 `boundary[1]` 会得到相反结论。

> 🔴 **订正（防回流）**：本报告初版把这个条目号写成了 `boundary[0]`，**错的**。经对任务文件逐条求值：
> `boundary[0]` 含「下移」**0 次**，`boundary[1]` 含 1 次（`functional[1]` 另有 7 次，是 dev 订正段在引述它）。
> 由 Leader 指出并复核。**若你从别处读到「boundary[0]」的引用，那是本错误的回流，以此处为准。**

---

## 7. E0：A/B 背对背与逐篇差集（硬判据，两把独立的尺）

```
base = adc0fcafe3cd8cec9cd5eda12f6b4d904c7b7f38
post = 189ca5e743d47ff33ca81f7edd549e82352b78a3
交替：base(r1) → post(r1) → base(r2) → post(r2)
语料：主仓库绝对路径 + --allow-incomplete，四次退出码均 0
```

- **确定性自检**：base 两轮逐字节一致 ✅；post 两轮逐字节一致 ✅。
- 输出 206 行 → 112 行。

### 🔴 硬判据：逐篇差集（尺 A —— 期次集合）

| 项 | 结果 |
|---|---|
| base 未成功期次数（解析失败 ∪ 本迭代不解析，去重） | **29** |
| post 未成功期次数 | **5**（`2020-02` / `2021-05` / `2022-04` / `2022-05` / `2023-05`） |
| 🔴 **post 未成功 ∖ base 未成功**（= 改动前成功、改动后失败） | **空** ✅ |
| 反向（救回，不必为空） | **24 期** |

救回的 24 期：`2020-04/05/07/08/10/11`、`2021-02/04/07/08/10/11`、`2022-02/07/08/10/11`、
`2023-02/04/07/08/10/11`、`2026-01`。

### 尺 B（独立仪器）：字段样本数单调性

尺 A 有一个盲区：若某期从「受支持」变成「不受支持」，它既不在 post 的未成功明细里、也不成功，
尺 A 抓不到。故补一把不同种类的尺——**若有期次从成功变失败/不再尝试，它贡献的字段样本会消失**：

```
base 字段数=76  post 字段数=76
n 下降的字段：无 ✅        post 里消失的字段：无 ✅
n 上升=66  持平=10
```

⇒ 两把尺互相印证，**没有任何期次从成功变失败**。

### 聚合数

| | base | post |
|---|---|---|
| 待解析（受支持期次） | 200 | **217** |
| 本迭代不解析 | 18 | **1** |
| 解析失败 | **38**（见 §9 观察项） | **4** |

---

## 8. N0：门禁类（测自 merge 后 master `d50835b`）

| 项 | 实测 |
|---|---|
| `gofmt -l internal/hestia cmd/atlas` | 恰为 `backtest_test.go`、`crisis_test.go` 两个既有欠账 ✅ |
| `go vet ./internal/hestia/... ./cmd/...` | 零输出，退出码 0 ✅ |
| `go test ./internal/hestia/... ./cmd/... -count=1` | 退出码 **0**，2 包 ok、0 FAIL ✅ |
| 覆盖率（`internal/hestia`） | **96.1%**，未跌破（仍零余量）✅ |
| `go.mod` / `go.sum` | `git show --numstat 189ca5e` 命中 **0** ✅ |
| 注释任务编号 | `M1c-4 的 TASK-005` 出现 **31** 处 ✅ |

### 越界申报与既有测试完整性

- **越界申报**：`writes` 由 5 扩到 7，`calibrate_test.go` 与 `parse_test.go` 均已在 `writes` 里 ✅，
  且是在 `dev_done` **之前**经 `update` 补入的（符合纪律）。
- **两个新 fixture 是派生而非复制**（Leader 要我确认「一字未动」）：

  | 派生 | 行数 | diff 变更行 | 实际改动 |
  |---|---|---|---|
  | `pboc-2020-04-monthly-broken-ordinals.html` | 440 = 440 | **2**（1 改 1） | 仅第 320 行 `一、`→`五、` |
  | `pboc-2022-08-monthly-broken-ordinals.html` | 437 = 437 | **2**（1 改 1） | 仅第 319 行 `一、`→`五、` |

  ⇒ **合计句一字未动**，确认属实 ✅。

- **既有测试一条未删**（结构闸四项）：

  | 闸 | base | post |
  |---|---|---|
  | `go test -list` 顶层测试数 | 629 | 637 |
  | `func Test` 源码计数 | 629 | 637 |
  | `t.Run` 计数 | 304 | 313 |
  | 断言计数（`assert.`+`require.`） | 2869 | 2949 |

  消失的 4 个顶层测试**全部有对应改名版本**，与 discovery 的迁移清单逐个吻合：

  ```
  TestSectorCaliberRejectsNonCumulativeSamples      → TestSectorCaliberRoutesNonCumulativeSamplesToMoM
  TestSelectRMBCumulativeFlowPicksMonthlyCumulative → TestSelectRMBFlowByCaliberRoutesMonthlySentences
  TestTSFFlowArticleRefusesCaliberMix               → TestTSFFlowArticleRoutesCaliberMixToSeparateColumns
  TestTSFFlowSectionRefusesCaliberMixOnRealArticle  → TestTSFFlowSectionRoutesCaliberMixOnRealArticle
  ```

  新增 12 = 8 条新测试 + 4 条改名 ⇒ **一条未删**成立 ✅。

---

## 9. 🔴 观察项：一个自证数字与它声明的锚不同源（不构成 reject）

discovery 与 Leader 的派验信都写「解析失败 **39** → 4」。**我在 dev 自己声明的 base 锚
`adc0fca` 上实测是 38**，三把尺一致（汇总行 38、明细节标题 38、明细区实际条数 38）。

查清了 39 的来源：

```
d54df6d（TASK-002 的 post，我上一轮验证时实测）        解析失败 = 39
  ↓ 中间合入 TASK-003（fcd7109 + 8f91015 + 98cabf6 + adc0fca：moneyRE 容忍短从句，2020-01 的 M1 可抽）
adc0fca（dev 声明的 base 锚，我本轮实测）              解析失败 = 38
```

⇒ **那个 39 对应的是 TASK-003 合入之前的树**，不是它声明的 `adc0fca`。

**为什么不据此判红**：

1. 它**不影响硬判据**——「逐篇差集为空」是我用两把独立的尺**从头复算**的，与 dev 报的数字无关。
2. DoD `error_handling[0]` 明确写「**不要在这里写死期望数字**——TASK-012 才是采数的地方」，
   聚合数在本任务里不是判据。
3. post 侧的 4 与全部其它自证数字（29/5/24、25 字段、V1/V2/V3 的 2/30/12、两个 sha256）
   我逐项复核**全部吻合**，这是唯一一处不符。

**但必须记下**，因为它属于本 sprint 反复强调的那一类：**自证数字采样于比它声明的锚更早的树，
而两个数字在报告里长得一模一样**。Leader 的派验信也复述了 39，若进 CONTRACTS 或 TASK-012
的记账会被继续传下去。

### 另一条观察项：任务文件内部仍有一处矛盾

**`boundary[1]`** 结尾保留着 DoD 原文「诊断门**必须下移到两族都空之后**」，而 `functional[1]` 里是
dev 的订正「**未下移**，判据是 `!hasCum`」。Leader 已裁决按订正判，本次据此判定；
但**矛盾文本仍在任务文件里**，只读 `boundary[1]` 会得到相反结论。
（条目号订正见 §6 末的防回流说明——初版误作 `boundary[0]`。）

---

## 10. 测试质量评审

- **非空洞**：五个变异（V1/V2/V3 + 我补的 V4/V5）全部 KILLED，每条守卫都能被对应的实现改动杀掉。
- **精确**：V4 只杀 B 格（A/A2/C 保持绿）、V1 只杀 2 条、列唯一性成立 ⇒ 守卫互不遮蔽。
- **真语料为主**：golden 用 4 份既有 fixture；`2022-07` 端到端走真文章；
  合成输入只用在「性质不随语料漂移」的场合（B 格用合成前缀「今年以来」，
  并在注释里写明理由——真实的 2022-05 一旦被正确修复，用真语料的守卫会被**一次正确的修复打坏**）。
- **迁移而非删除**：断言「必须报错」的改成断言「必须落进哪一列」，判据换形式不换内容，结构闸四项已核。

---

## 11. 结论

**8 条 done_criteria 全部 PASS**。核心判据均由验证者独立产生，未采信 discovery 的结论：

- 硬判据「逐篇差集为空」用**两把不同种类的尺**从头复算（期次集合 + 字段样本数单调性）；
- `2022-07` 的 `deposit_flow_mom=447` 与 `loan_flow_ytd=143500` **共存**是我在两棵树上各跑一次
  `Parse` 得到的，且对侧两列确认为空；
- V1/V2/V3 全部 KILLED、**列唯一性成立**，V3 杀 golden 的**原因**核到了具体断言行；
- Leader 要我补验的 **DoD 偏离理由② 确证**（下移后 18.7 万亿静默丢失，原实现响亮报错）；
- 额外补了 V5，确认「每段一个口径」这条 TASK-011 分析基座**确有守卫**。

发现一处自证数字与其声明锚不同源（§9），不影响任何判定结论，已记录待 Leader 处置。

**判定：VERIFIED**
