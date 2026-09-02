# TASK-010 验证报告 —— 人工标定 yaml（22 项区间 + 四个容差）

- **验证者**：test-m1c4-a
- **判定对象**：`master @ bdc8252679b9740af8825b20eb80c485c557a745`（= `verify_baseline.head`）
- **dev commit**：`482cdb9b71b120011ea5eff57b624df745385c03`（parent `743c507…`）
- **assignment_epoch**：1
- **结论：进行中**（本文件随验证推进增量落盘；最终判定见文末）

> ⚠️ **本报告采用增量落盘**：验证者的 session 曾于 09:29 因 API 错误中断一次，
> 中断时手上的复算结果尚未落盘。此后每完成一步即追加，不把结论留在 context 里。

---

## 0. 基线核对（中断前后各一次，两次均一致）

| 项 | verify_baseline | 实测 | 一致 |
|---|---|---|---|
| `head` | `bdc8252679b9…` | 同值 | ✅ |
| `discovery_sha256` | `f52c12bbfe2d…` | 同值 | ✅ |
| `assignment_epoch` | 1 | 1 | ✅ |

`482cdb9` 的 parent 为 `743c507`；`git log 743c507..bdc8252` 只含 `482cdb9` + merge 本身，
**无他人改动混入**。7 个改动文件**全部在 `writes` 内，无越界**；`go.mod`/`go.sum` 零改动。

**改动规模**（`git show --numstat 482cdb9`，与 discovery 逐条一致）：
`configs/hestia.yaml 138/45`、`config.go 67/0`、`config_test.go 96/1`、`fields.go 15/2`、
`thresholds.go 115/41`、`validate.go 72/8`、`validate_test.go 144/10`。

---

## 1. 门禁（我自己在 `bdc8252` 上重跑，未采信派验信里的那次）

| 项 | 实测 |
|---|---|
| `gofmt -l internal/hestia cmd/atlas` | 恰为 `backtest_test.go`、`crisis_test.go` 两个既有欠账 ✅ |
| `go vet ./internal/hestia/... ./cmd/...` | 零输出，退出码 0 ✅ |
| `go test ./internal/hestia/... -count=1` | 退出码 **0** ✅ |
| 覆盖率 | **96.3%** ✅ |
| `go.mod` / `go.sum` | 零改动 ✅ |

---

## 2. 22 项区间的组规则算式（机械复核，DoD 的一整条）

规则（yaml `:117-118` 既有约定）：`[min<0 ? min×4 : −max×0.5, max×4]`，圆整**向外**。

我写脚本对 discovery 里的 22 行**逐行验算**，并**反推圆整粒度**（dev 只说「向外」未说粒度）：

```
2 位有效数字向外圆整：22/22 匹配
3 位有效数字向外圆整：0/22 匹配
圆整方向检查（lo ≤ 算式值 且 hi ≥ 算式值）：全部向外 ✅
区间是否包含实测 [min,max]：全部包含 ✅
```

⇒ **22 项算式全部正确，且圆整规则自洽**（统一为「2 位有效数字、向外」）。

抽样（完整 22 行见 discovery）：

| 字段 | 实测 min..max | 算式值 | yaml 落盘 |
|---|---|---|---|
| `tsf_flow_mom` | 5282..72200 | `[-36100, 288800]` | `[-37000, 290000]` |
| `tsf_flow_equity_mom` | 291..1478 | `[-739, 5912]` | `[-740, 6000]` |
| `deposit_corp_mom` | -24200..12900 | `[-96800, 51600]` | `[-97000, 52000]` |
| `loan_nbfi_mom` | -1474..2170 | `[-5896, 8680]` | `[-5900, 8700]` |

### 落盘核实（表对 ≠ yaml 写对）

| 检查 | 结果 |
|---|---|
| yaml `magnitude_ranges` 解析到的项数 | **76** |
| 22 项在 yaml 中缺失 | 无 ✅ |
| **表 ↔ yaml 逐项比对** | **22/22 一致** ✅ |
| `unit` 缺失 | 无 ✅ |
| 倒置/退化区间（min ≥ max） | 无 ✅ |
| **全覆盖**：`fieldOrder` 有而 yaml 没有 | **无 ✅** |
| yaml 有而 `fieldOrder` 没有 | 无 ✅ |

> ⚠️ **我自己的一次失误，记下来**：第一版 yaml 解析器假设是缩进块结构，实际是行内
> flow mapping（`field: {min: X, max: Y, unit: "Z"}`），**解析到 0 项**，于是比对结论
> 输出了「表↔yaml 不符：无 ✅」——那是**空集造成的假绿**。是因为我同时打印了
> 「解析到 N 项」才当场发现。修正后加了 `assert len(got) > 0` 作为前置断言。

---

## 3. 三处 DoD 订正之一：校验落点（**我自己读代码定位，未采信 dev 的说明**）

DoD 原文说那个块「在 `LoadConfig` 的 `if len(t.MagnitudeRanges) > 0 {` 里」。

**事实①**：脚本按 `func` 定义回溯定位——该块在 **`thresholds.go:375`**，
所在函数是 **`func (t Thresholds) validate()`**（起于 278 行），**不是 `LoadConfig`** ⇒ dev 的订正成立。

**事实②**：`Thresholds.validate()` 的**入口**确实是**两个**：

```
LoadConfig → Config.validate()（config.go:57）→ Thresholds.validate()（config.go:222）
Validate() → Thresholds.validate()（validate.go:90）
```

⇒ `validate.go:90` 确实在 `Validate()` 路径上，**照 DoD 原文改会波及每一次 `Validate()`** ⇒ 订正理由成立。

> ⚠️ 我第一遍用「所有 `.validate()` 文本出现」计数得到 **3 个**，差点报成与 dev 不符。
> 查上下文后确认 `config.go:222` 是 **LoadConfig 路径的中间环节**（在 `Config.validate()` 内），
> 不是第三个入口。**是我的仪器口径错了，dev 的「两个调用方」正确。**

**事实③**：落点 `checkMagnitudeRangesComplete`（`config.go:98`）确实**照 `checkStockContinuityComplete`
（`config.go:155`）的形态**；调用点 `config.go:65`，排在 `cfg.validate()`（`:57`）**之后** ✅。

**事实④**：三条方向互补的测试**全部存在且通过**（DoD 要求「只跑一条会以为对了」）：

```
--- PASS: TestLoadConfigRejectsPartialMagnitudeRanges          （拒半张表）
--- PASS: TestLoadConfigAcceptsAbsentMagnitudeRanges           （空表仍合法）
--- PASS: TestEmptyMagnitudeRangesStillValid                   （空表仍合法，另一路径）
--- PASS: TestLoadConfigReportsStructuralErrorBeforeCompleteness（结构性错误优先）
```

---

## 4. Q1 归因更正：两个子集的分位数（**我自己跑生产代码路径复算**）

探针：`BackfillLoad` 建库 → `Store.PrecedingAll` 枚举 97 个合并观测 →
按 `depositPartFields` / `depositPartFieldsMoM` / `corpLoanPartFields(MoM)` 逐族算残差与分母。

### 四族残差汇池（与 discovery 的标定依据表逐个比对）

| 族 | n | p50 | p90 | p95 | max | \|分母\| min | \|分母\| p50 | \|差额\| p95 | \|差额\| max |
|---|---|---|---|---|---|---|---|---|---|
| `deposit/ytd` | 52 | .0885 | .1460 | .1518 | **.2501** | 28800 | 147300 | 27353 | **28000** |
| `deposit/mom` | 21 | .2576 | .7796 | .8194 | **2.9508** | **447** | 12700 | 7879 | **8681** |
| `corp_loan/ytd` | 52 | .0136 | .0247 | .0294 | **.0357** | 25500 | 98000 | 2952 | **3432** |
| `corp_loan/mom` | 26 | .0176 | .0614 | .0784 | **.0788** | 2335 | 7552 | 387 | **635** |

⇒ **与 discovery 报的逐个数字完全吻合**（含 `corp_loan/ytd |差额| max 3432` 这类未在裁决里出现的旁证值）。

### 🔴 归因更正的两个子集（dev 主动更正它给人类的说法，Leader 要我独立复算）

```
deposit/mom  |分母| >= 10000 子集: n=14  p50 0.1672  p95 0.3931
deposit/ytd  |分母| >= 10000 子集: n=52  p50 0.0885  p95 0.1518
deposit/mom  |分母| <  10000 子集: n=7   max|差额| 2272
deposit/ytd  |分母| <  10000 子集: n=0
```

⇒ **dev 的更正属实**：排除分母近零效应后，`deposit/mom` 的 p50（0.1672）仍是 `deposit/ytd`（0.0885）
的 **1.89 倍**、p95（0.3931）是 ytd（0.1518）的 **2.59 倍** ——
**分母近零只解释尾部，不解释主体**，原归因（「mom 残差大是分母近零」）不成立。

⚠️ 这条更正**不改变结论**（换仪器仍对），但**改变取值方法**：按原归因会得出
「K_rel 沿用 ytd 的 0.17 + 加个 K_abs」，而 0.17 远低于大分母子集的 p95 0.3931
⇒ **那 14 期大分母期次里会有相当一部分 failed**。dev 主动更正了自己给人类的依据，
这是本任务里最值得记的一处。

---

## 5. 四个容差的推导核实与「那把尺」

### 5.1 推导逐条复算（用我自己 §4 的实测值，不引用 discovery 的数）

| 量 | 推导式 | 我的复算 | 落盘值 | 判定 |
|---|---|---|---|---|
| **1.40**（余量倍数） | `corp_loan/ytd` 现值 0.05 ÷ 实测 max 0.0357 | **1.4006** | 报 1.40 | ✅ |
| **K_rel** | p95(`deposit/mom`, \|分母\|≥10000 子集) 0.3931 × 1.40 | **0.55034** | `deposit_sum_tolerance_mom: 0.55` | ✅ |
| **K_abs** | max\|差额\|(`deposit/mom`, \|分母\|<10000 子集) 2272 × 1.40 | **3180.8** | `deposit_sum_tolerance_mom_abs: 3200` | ✅（2sig 向外） |
| **corp_loan/mom** | 实测 max 0.0788 × 1.40 | **0.11032** | `corp_loan_tolerance_mom: 0.11` | ✅ |

⇒ **四个值全部可追溯到一个分位数**，没有拍脑袋的数（DoD 的判别式是「给不出『这个数从哪个分位来』的取值就是拍的」）。
`1.40` 本身也核过：它不是 dev 选的，是 `corp_loan/ytd` 现行值相对其实测 max 的余量倍数，
而那是四族里唯一经真实数据检验（0 期 failed）的值。

### 5.2 🔴 dev 自己立的那把尺：中位分母下允许的绝对差额 vs 历史最大绝对差额

DoD 明令禁止「为消掉 failed 而调宽容差」。dev 把这条禁令变成可算的判据。我逐族套用：

| 族 | 门限规则 | 中位分母下允许差额 | 历史最大差额 | 判定 |
|---|---|---|---|---|
| `deposit/ytd` | `0.17 × \|分母\|` | 25041 | 28000 | ✅ 仍抓得住 |
| `deposit/mom` | `max(3200, 0.55×\|分母\|)` | **6985** | **8681** | ✅ 仍抓得住 |
| `corp_loan/ytd` | `0.05 × \|分母\|`（**保持不动**） | 4900 | 3432 | ⚠️ 允许 > 最大 |
| `corp_loan/mom` | `0.11 × \|分母\|` | 831 | 635 | ⚠️ 允许 > 最大 |

**dev 的对照论证成立**（我复算）：同一条 ×1.40 规则套在**旧仪器**（纯比值）上会得到
`2.9508 × 1.40 = 4.1311`，中位分母下允许 **52465** 而历史最大只有 8681
⇒ **那才是 no-op**。新仪器 6985 < 8681 ⇒ **「换对仪器」与「调宽」在这把尺下是可区分的**，
dev 的说法属实。

**量级事故检出力**：`deposit/mom` 新仪器中位门限 6985（10³），而真的量级事故
（累计值写进当月列）绝对差额在 10⁵ ⇒ **仍必被抓到** ✅。

#### ⚠️ 观察项：这把尺对 `corp_loan` 两族给出「允许 > 最大」，而 dev 没有对它们应用

dev 用这把尺论证了 `deposit` 两族，**但没有把同一把尺用在 `corp_loan` 两族上**。我补算后：
两族都是「中位分母下允许的差额 > 该族历史最大差额」——按这把尺的字面含义，
若一个中位分母期次出现该族历史最大幅度的误差（如 `corp_loan/mom` 的 635 / 7552 = 0.0841 < 0.11），
**闸门不会报**。

**但这不构成缺陷，理由三条**（我核过）：

1. `corp_loan/ytd` 的 `0.05` 是**保持不动**的现值，人类在 Q3 明确说不动，本任务一个字没改；
2. `corp_loan/mom` 的 `0.11` 是**人类在 Q3 裁决的** `= max × 1.40`，dev 忠实执行（我复算 0.11032）；
3. **`×1.40` 规则本身就蕴含「允许 > max」** —— 任何按 `max×1.40` 定的比值容差，
   在产生该 max 的分母上都允许 1.4 倍差额。⇒ 这把尺与 `×1.40` 规则是**两把不同的尺**，
   对 `corp_loan` 两族必然给出相反结论，而人类采纳的是后者。

⇒ 记为观察项：**判据应用不一致**（同一把尺只用在两族上）。不影响判定，但若 CONTRACTS 要
引用「那把尺」作为通用判据，应写明它与 `×1.40` 规则的关系。

### 5.3 `corp_loan_tolerance_mom` 从 0.05 调到 0.11 是否触犯「不得为消 failed 而调宽」

**不触犯。** yaml 自己写明了：

```
# ⚠️ 现值 0.05 是照抄 ytd 的占位，在 79 期上有 5/26 failed；标定后归零。
```

⇒ `0.05` 从未针对 mom 族标定，是**照抄 ytd 的占位值**；而 mom 族实测 max 0.0788 **本就高于该占位**。
调到 0.11 是**首次标定**，不是把一个已标定的值放宽。这与 22 项区间「组规则纠正偏紧占位」同构。

---

## 6. 标定前后对照（独立复算，30 → 1）

⚠️ **先对齐口径**：discovery 该节标题写「79 期语料」，正文写「全 213 条记录」，两者不一致；
但它给出的分母（52 / 21 / 52 / 26）**正是我 §4 探针算出的四族 n**，即**97 个合并观测**口径。
我按这个口径复算（对每族用标定前/后两套容差各判一次）：

| 族 | n | passed | FAILED | 与 dev 报的 |
|---|---|---|---|---|
| `deposit_sum/ytd` | 52 | 44 → **51** | **8 → 1** | ✅ 逐个一致 |
| `deposit_sum/mom` | 21 | 4 → **21** | **17 → 0** | ✅ |
| `corp_loan/ytd` | 52 | 52 → 52 | **0 → 0** | ✅ |
| `corp_loan/mom` | 26 | 21 → **26** | **5 → 0** | ✅ |
| **合计** | | | **30 → 1** | ✅ |

**剩下的那 1 条实测确实是 `2020-01`** ✅ —— 与裁决「留作一个可见的 failed」一致。

标定前 FAILED 的期次（我的复算，供 TASK-012 对照）：
- `deposit/ytd` 8 期：2020-01 / 2022-01 / 2023-09 / 2024-04 / 2024-05 / 2024-06 / 2024-07 / 2024-08
- `deposit/mom` 17 期：2020-05/07/08/10、2021-02/04/05/08/10/11、2022-02/07/08/10、2023-02/04/05
- `corp_loan/mom` 5 期：2020-10 / 2021-04 / 2021-05 / 2022-07 / 2023-07

⚠️ **「FAILED 归零」在本任务里不是好消息的同义词**，但 §5.2 那把尺已证明
`deposit/mom` 归零是换对仪器的结果（新门限 6985 仍小于历史最大 8681），不是调宽。

### 6.1 真 load 上的交叉验证（另一把尺）

我另在 pre(`743c507`) / post(`bdc8252`) 两棵树上各跑两轮 `backfill load`（交替、确定性自检通过，
pre/post 各两轮逐字节一致）：`tolerance_exceeded` 总数 **5 → 1**，其中
`deposit_sum[mom]` **4 → 0**、`deposit_sum[ytd]` **1 → 1**。

⚠️ 这把尺的绝对值与 §6 不同（load 报告只列进 pending 的记录，且含 drift 等其它判据），
**但方向一致**，且 post 侧剩的唯一一条同样是 ytd 族。两把尺互不矛盾。

---

## 7. 两件裁决点名要落盘的（逐一核在不在）

### ① 2020-01 显式登记为已知 failed 期次 —— ✅ **两处都在**

```
configs/hestia.yaml:59          🔴 已知会 failed 的期次：2020-01（残差 0.2501）。这不是数据错误，不要去查。
internal/hestia/thresholds.go:39  （同一句，逐字一致）
```

裁决要求的三要素齐全：**成因**（1 月的 ytd 分母就是当月一个月＝该族最小分母 28800，
而其绝对差额 7203 反低于该族中位）、**「不是数据错误」**、**「不要去查」**。

### ② yaml `:196` 那句「ytd 的区间对 mom 一定偏宽」已订正 —— ✅ **在，且带两处反例**

订正落在 `configs/hestia.yaml:268-274`（行号因本次新增 138 行而后移）：

```
⚠️ **订正**：占位时这里写「…ytd 的区间对 mom **一定偏宽**」——**那句不普遍成立**（实测证否）。
   两处反例：`deposit_fiscal_mom` 的组规则下界 −34000 **宽于**占位 −13000；
             `tsf_flow_bankaccept_mom` 的组规则上界 26000 **宽于**占位 24000。
   ⇒ 照抄 ytd 在这两项上给出的是**偏紧**的区间。今天还没误拦（逐项核过 76 字段 × 213 期，
   实测出界 0 处），但 deposit_fiscal_mom 的下界余量只剩 1.53 倍，而组规则要求 4 倍。
```

两处反例的数值我在 §2 的 22 行复核里都验过：
`deposit_fiscal_mom → [-34000, 55000]`、`tsf_flow_bankaccept_mom → [-17000, 26000]`，
与订正文字**逐字对得上**。

---

## 8. 变异（我自己跑，隔离 worktree `../wt-t010b` @ `bdc8252`）

主工作区 `validate.go` / `thresholds.go` 的 sha256 前后一致，代码目录改动 **0 行**。
每格闸：`gofmt -e` 语法闸 + **变异 diff 必须非空** + `build/setup failed` + `panic` +
**`=== RUN` 计数须等于对照组**（这一条是我在 TASK-008 被崩溃型变异骗过之后加的）。

**对照组：691 PASS / 0 FAIL、`=== RUN` 1469。**

### 8.1 M3：把漂移循环写死成 ytd 族（DoD 说「M3 早已闭合」，dev 用变异证的）

```
--- FAIL: TestDepositSumSkipsWhenPriorsAreOtherCaliber     ← 且只有它一条（690 PASS）
=== RUN 1469（= 对照组）⇒ 套件跑全
```

⇒ **dev 的结论成立**：杀它的正是 TASK-008 加的那条测试（而那段 DoD 写于 TASK-008 合入之前），
「M3 早已闭合、不补重复测试」属实。这条我验过 TASK-008，正在射程内。

### 8.2 校验落点：照 DoD 原文把全覆盖校验放进 `Thresholds.validate()`

```
顶层 FAIL=12  顶层 PASS=679   === RUN 1469（= 对照组）⇒ 套件跑全
```

转红的 12 条及其归属：

| 测试 | 文件 | 在 `writes` 内 |
|---|---|---|
| `TestBackfillLoadRecordsPendingReasons` | `backfill_load_test.go` | **否** |
| `TestMagnitudeRangesAcceptValidTable` | `thresholds_test.go` | **否** |
| `TestMagnitudeRangesRejectBlankUnit` | `thresholds_test.go` | **否** |
| `TestMagnitudeRangesRejectDegenerateRange` | `thresholds_test.go` | **否** |
| `TestMagnitudeRangesRejectInvertedRange` | `thresholds_test.go` | **否** |
| `TestMagnitudeRangesRejectNaNFromYAML` | `thresholds_test.go` | **否** |
| `TestMagnitudeRangesRejectNaNOrInf` | `thresholds_test.go` | **否** |
| `TestMagnitudeRangesRequireUnit` | `thresholds_test.go` | **否** |
| `TestLoadConfigRejectsPartialMagnitudeRanges` | `config_test.go` | 是 |
| `TestMagnitudeSanityActivatesWhenCalibrated` | `validate_test.go` | 是 |
| `TestMagnitudeSanityBoundariesAreInclusive` | `validate_test.go` | 是 |
| `TestMagnitudeSanityReportsEarliestFieldInFieldOrder` | `validate_test.go` | 是 |

⇒ **`writes` 之外 8 条 —— 与 dev 报的「8 条在 writes 外」逐字吻合**。
且它们测的确实是 **NaN / 倒置区间 / 单位缺失 / 退化区间**这类**与全覆盖无关**的性质
（夹具用「只含待测字段的小表」是正当用法）⇒ **dev 的订正理由完全成立**。

⚠️ 总数我测 **12** 而 dev 报 **9**（关键的「8 条在 writes 外」一致）。差在 `writes` **内**那 4 条
（`config_test.go` 1 + `validate_test.go` 3）——推测 dev 计数时那几条尚未加入，或它的变异形态略异。
**不影响订正理由的成立**，但记下来。

---

## 9. 观察项（不影响判定）

1. **那把尺没有用在 `corp_loan` 两族上**（§5.2）——若应用，两族都显示「中位分母允许 > 历史最大」。
   成因是它与人类采纳的 `×1.40` 规则是两把不同的尺，必然给出相反结论。
   若 CONTRACTS 要引用「那把尺」作通用判据，应写明这层关系。
2. **discovery 该节的口径自述不一致**：标题写「79 期语料」、正文写「全 213 条记录」，
   而它给出的分母（52/21/52/26）实为 **97 个合并观测**。三个数字互不相同。
   结论无误（我按 97 观测口径复算得到完全相同的 30→1），但**口径字样应更正**，
   否则 TASK-012 复核时会拿错分母。
3. 落点变异总数 12 vs dev 报 9（§8.2）。

## 10. 我自己在本次验证中的三次失误（如实记录）

1. **yaml 解析器返回 0 项造成空集假绿**：第一版按缩进块解析，实际是行内 flow mapping，
   比对结论输出「表↔yaml 不符：无 ✅」——那是**空集**。靠同时打印「解析到 N 项」发现，
   修正后加了 `assert len(got) > 0`。
2. **`.validate()` 调用方计数口径错**：我数「所有文本出现」得 3，dev 说 2。查上下文后确认
   `config.go:222` 是 LoadConfig 路径的**中间环节**，入口确实是两个——**是我的仪器错了**。
3. **落点变异第一次锚定失败**（tab/空格不符），`diff` 为空而测试全绿，若不看 diff 会记成
   「照原文改也没事 ⇒ dev 的订正理由不成立」这个**方向完全相反的错误结论**。
   靠「变异 diff 必须非空」这道闸接住，改用行号定位后重做。

⇒ 三次都是**假绿**（空集 / 口径 / 未生效），且都被「打印中间量」拦住。
本报告所有结论均为闸通过后的结果。

---

## 11. 结论

**DoD 各条均已逐项核实**，证据全部由验证者独立产生：

| 项 | 结论 |
|---|---|
| 门禁（自己在 `bdc8252` 重跑） | 全过，覆盖率 96.3% ✅ |
| 22 项组规则算式 | **22/22 吻合**，圆整规则自洽（2sig 向外），yaml **逐项一致 + 全覆盖** ✅ |
| 三处 DoD 订正 | **全部成立**：落点两个事实我自己读代码定位；M3 变异 KILLED 且只杀那一条；Q1 归因更正的两个子集分位数我自己算出 ✅ |
| 四个容差推导 | 四个值**全部可追溯到一个分位数**，`1.40` 本身也核过（1.4006）✅ |
| 「不得为消 failed 而调宽」 | `deposit/mom` 新门限 6985 < 历史最大 8681，**仍抓得住**；旧仪器套同规则会得 52465（no-op）⇒ **换仪器与调宽可区分** ✅ |
| 标定前后对照 | **30 → 1** 四族逐个吻合，剩的 1 条确是 `2020-01` ✅ |
| 两件裁决落盘物 | 2020-01 登记（yaml + thresholds.go 两处）、yaml `:196` 订正（含两处反例）**都在** ✅ |
| 忠实执行裁决 | 四个容差、22 项区间、两件落盘物**逐项对得上 `questions[].answer`**；Q5 的分法也按 Leader 裁决执行（只产依据不产建议值）✅ |

**判定：VERIFIED**
