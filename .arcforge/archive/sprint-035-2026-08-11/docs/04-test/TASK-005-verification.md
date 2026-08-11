# TASK-005 验证报告 —— deposit_sum 的绝对残差与漂移两判据合成

- **验证者**：test-agent-24（Reality Checker，默认判定 NEEDS WORK）
- **被验对象**：`master @ 1716dfc69aef79d1ff4b69a80d3231886be411d8`（全 sha）
- **结论**：**VERIFIED（7/7 PASS）**

**消融统计：19 个变异。本任务范围内 11 KILLED + 1 受控存活演示（双态对照）；
跨任务边界排查 1 KILLED + 6 SURVIVED —— 存活的 6 个全部落在 TASK-004 的代码里，
本任务自己的数值比较 100% 有守卫。**

> §4 已在 Leader 提交封闭性论证后重写：由「我自己的枚举」改为「对该论证的复核」。
> 判定不变，但**论证有一处需要纠正**，且该纠正会影响 T6/T7 复用它。

---

## 0. 漂移核实与 scope

```
$ git rev-parse HEAD                     → 1716dfc69aef79d1ff4b69a80d3231886be411d8
$ shasum -a 256 .arcforge/discoveries/TASK-005.json
b9c19e6236d3964cd923edb36c4c8cfde3ad004b4ee7d11bd0bff082d9db4c94   （= verify_baseline 记录值）
$ git show --numstat --oneline 1716dfc
113	0	internal/hestia/validate.go
224	2	internal/hestia/validate_test.go
```

零漂移；实改 2 文件 == `writes` 两项。
**`wantGateIDs` 是追加不是放松**：新增一行 `"deposit_sum"`（位置与 gates 表一致，紧跟
`monetary_hierarchy`），`require.Equal` 原样保留，两处删除只是注释与消息里的「五道→六道」。

## 1. 门禁（隔离 worktree `/tmp/verify-005`）

```
$ go test ./internal/hestia/ -count=1 -cover  → ok  coverage: 91.7% of statements
$ gofmt -l internal/hestia/ ; go vet ./...    → 均无输出
```

本任务 5 条测试 / 12 个子测试全 PASS。`gateDepositSum` 97.1%、`depositResidual` 100%。

## 2. 消融方法学

harness 四道闸：**sha 生效 → `gofmt -l` → `go test -c -o /dev/null` 编译闸 → 还原核实**。
对照组先验全绿。本轮编译闸未触发。

> 独立佐证：还原后 `validate.go` 的 sha256 为 `523a3bec85e2954cbf5c7abfa9ed3052bfb68c0fd63252a973c4f425f68f1eac`，
> 与 dev 在 discovery 的 `control_group` 里记的**逐字相同**。

---

## 3. 头等大事：M2 双态对照

**状态 A**（测试完整 + `drift > DriftMax` → `>=`）：

```
--- FAIL: TestDepositSumBoundaryIsInclusive/漂移恰好等于上限判_passed
    validate_test.go:613  Not equal
      Messages: 实现是 drift > max 判失败，恰好等于必须通过；这里变红说明比较符被改成了 >=
    validate_test.go:615  Should be empty, but was
      drift_exceeded: residual 0.0938 drifted 0.0312 from 3-period mean 0.0625
```

KILLED，**全包只有这一个子测试转红**。

**状态 B**（移除漂移边界那 47 行后重跑同一变异）：先确认删对了地方 —— 移除后未变异实现下
整包仍绿，且 `TestDepositSumBoundaryIsInclusive` 剩下的两个子测试仍 PASS。然后：

```
$ go test ./internal/hestia/ -count=1
ok  	github.com/newthinker/atlas/internal/hestia	0.672s
```

**SURVIVED。⇒ dev 补的漂移边界测试是 M2 的唯一杀手，DoD 的遗漏真实存在且已被堵上。**

---

## 4. 复核 Leader 的边界封闭性论证

Leader 要求「复核论证本身而不是重新枚举」。我按此办：**先跑他给的扫描规则，再逐条验前提。**

### 4.1 结论速览

| 待验项 | 结论 |
|---|---|
| 前提 A：全部阈值比较都在闸门函数内 | ✅ **成立**（跨文件扫描确认） |
| 前提 B：`Thresholds` 数值字段 ↔ 比较位置**一一对应** | ✅ **成立**（每个阈值恰好被比较一次） |
| **论证的封闭性** | ⚠️ **对「阈值比较」封闭；对论证正文声称的「数值比较」不封闭** |
| #5/#6 不返工的裁定 | ✅ **不反对**，且我能给它一个更强的依据；但**提醒挂错了地方**，有一条一行的改进 |

### 4.2 前提 A：阈值只在闸门里被比较 —— 成立

跨全部非测试文件扫五个阈值字段与 `Range.Min/Max`：出现点只有
`thresholds.go` 的**字段声明**与 `DefaultThresholds` 的**字面量赋值**（都不是比较），
以及 `validate.go` 的比较与 `fmt.Sprintf`。**没有任何阈值在闸门函数之外被比较。**

### 4.3 前提 B：字段 ↔ 比较位置一一对应 —— 成立（但依赖一个易碎的隐含条件）

| 阈值字段 | 比较位置 | 比较次数 |
|---|---|---|
| `DepositSumTolerance` | `:192` | 1（`:195` 是 Sprintf，非比较） |
| `DepositSumDriftMax` | `:226` | 1 |
| `CorpLoanTolerance` | `:289` | 1（`:294` 是 Sprintf） |
| `YoYSanityMax` | `:322` | 1（`:327` 是 Sprintf） |
| `StockContinuityMax` | —— | 0（TASK-006 将加） |
| `Range.Min` / `Range.Max` | `:398` | 各 1 |

5 个数值阈值 + `Range` 两个字段 = 7，与表的 1-7 行一一对应。**成立。**

⚠️ **但这个论证依赖一个未写出的条件：每个阈值恰好被比较一次。** 当前为真。
若将来某个阈值被用在两处（例如同一容差既做告警级又做失败级），**按字段数计数会静默少算一处**，
而表看上去仍然「对得上」。T6/T7 复用此论证时请连这个条件一起验，不要只数字段。

### 4.4 需要纠正的一处：扫描规则与表不是同一个集合

论证正文写的是：「全文按 `(<=|>=|<|>)` 扫描、排除注释 / `len()` / `for` / `range`，
**剩余数值比较恰好 6 处**」。我照此实跑：

```
1  138:  if m2 > m1 && m1 > m0 {                                   ← 表中无
2  192:  if r > in.cfg.DepositSumTolerance {                       ← 表 #1
3  226:  if drift := math.Abs(r - mean); drift > ...DriftMax {     ← 表 #2
4  289:  if r <= in.cfg.CorpLoanTolerance {                        ← 表 #3
5  313:  if a := math.Abs(v); a > worst {                          ← 表中无
6  322:  if worst <= in.cfg.YoYSanityMax {                         ← 表 #4
7  398:  if v < r.Min || v > r.Max {                               ← 表 #5/#6
```

**7 行、9 个比较**，而表里是 6 个。多出来的是 `:138` 的两处与 `:313` 的一处 ——
它们不含 `len(`、不是 `for`/`range`、不是注释，**按正文写的规则应当入表**。

⇒ **实际执行的过滤器是「与 `Thresholds`/`Range` 字段比」，比正文声称的「数值比较」窄。**
这不是笔误层面的问题：**枚举单位在「声称的规则」与「执行的规则」之间发生了漂移**，
而这正是本论证要修的那个病的同一形状 —— 上一次是「闸门 → 数值比较」，这次是
「数值比较 → 阈值比较」。**正文已经写对了单位，表却是按旧单位建的。**

**这个差别有实际后果，不只是措辞**。我对多出来的三处都做了消融：

| 位置 | 变异 | 结果 | 语义 |
|---|---|---|---|
| `:138` | `m2 > m1` → `>=` | **SURVIVED** | 「M2 恰好等于 M1」会判通过。M2 严格含 M1（M1 + 准货币），相等即可疑数据，`>` 是正确语义 |
| `:138` | `m1 > m0` → `>=` | **SURVIVED** | 同上 |
| `:313` | `a > worst` → `>=` | **SURVIVED** | 并列时报哪个字段名会变，`Value` 不变。近乎无害 |

⇒ **`:138` 的两处是与 #5/#6 完全同形、且语义上更实在的缺口**（magnitude 当前恒 skipped，
monetary 每期都在跑）。它们**不在表里，因而也不在 F17 里**。

**建议**：把表的口径明确写成「**阈值比较**」并说明为什么这个更窄的单位够用；
若认为不够用（我认为不够，理由见上），则把 `:138` 的两处补进表与 F17。

**另外两处被 `len()` 过滤器排除的比较**，我也顺手验了，以确认该过滤器无害：
`:215 len(hist) < minDriftHistory`（边界 2 vs 3）**已被守住**（消融 D7 打红 4 处）；
`:373 len(s) <= n`（firstN 截断）**SURVIVED**，后果是恰好 n 项时多打一个「…」，纯文案。
⇒ `len()` 过滤器在本轮没有藏起真问题，但它**能**藏 —— `len(hist) < minDriftHistory` 就是
一个货真价实的边界，只是碰巧有守卫。

### 4.5 #5/#6 不返工的裁定：**不反对**，并给它一个更强的依据

Leader 的理由是「`MagnitudeRanges` 默认为空、该闸恒 `skipped{not_calibrated}` ⇒ 实际影响为零」。
我不反对这个结论，但**「当前为空」这个依据比它能拿到的弱**。更强的依据是：

**空表状态是被机制钉住的，不是碰巧。** `thresholds_test.go:36`
的 `TestDefaultThresholdsLeaveMagnitudeRangesUncalibrated` 用 `assert.Empty` 精确断言，
而我在 TASK-001 验证时做过消融（M4：给 `MagnitudeRanges` 填一条看似合理的 m2 区间）
⇒ **KILLED**（`Should be empty, but was map[m2:{200 400 万亿元}]`）。

⇒ 任何人要填表，**必然先撞红这条测试、必须动手改它**。风险不会静默materialize。
这把裁定从「现在影响小」升级成「影响被一道会响的闸挡在门外」——是个**结构性**理由，
比时点性理由稳。

### 4.6 但提醒挂错了地方 —— 一条一行的改进

F17 写在 `findings-carryover.md` 里，那是一份**需要被记起来**的文档。
而必然会触发的那道闸（上面那条 `assert.Empty` 测试）**当前只字未提边界缺口**，
它的注释只说「防的是有人顺手填了几个看起来合理的数」。

于是 M1c 填表那天的实际路径是：填表 → 该测试转红 → 改它 → **没有任何东西提示还要补边界测试**，
除非那个人恰好想起 F17。

**建议（不属本任务范围，交 T7 或收尾）**：在那条测试的注释里加一行指向 F17，例如
「填表时另需补 `gateMagnitudeSanity` 区间两端的边界测试与遍历顺序守卫，见 F17」。
理由：**把提醒挂在必然会响的绊线上，而不是挂在需要被记起来的文档里** ——
这与本仓库导出面守卫「逼你留一行说明」是同一个手法。

---

## 5. 四条 dev 声称的独立复核

我写了探针打印位模式（跑完即删），**不做推理**。

### 5.1 声称②：`12.0/100` 与 `0.12` 同一个 float64 —— **成立**

```
12.0/100                 = 0.11999999999999999556  bits=0x3fbeb851eb851eb8
0.12 字面量               = 0.11999999999999999556  bits=0x3fbeb851eb851eb8
DefaultThresholds 的容差   = 0.11999999999999999556  bits=0x3fbeb851eb851eb8
实际算路 |88-100|/100     = 0.11999999999999999556  bits=0x3fbeb851eb851eb8
```

四者位模式全同。（dev 记的 `8646911284551352p-56` 与我的 `0x.f5c28f5c28f5cp-3` 是同值不同记法。）

### 5.2 声称①：漂移边界四个值精确 —— **成立**

```
前三期残差 6.25% ⇒ r  = 0.0625    bits=0x3fb0000000000000   == 1/16 ✓
三期均值 mean        = 0.0625    bits=0x3fb0000000000000   == 1/16 ✓
本期残差 9.375% ⇒ r  = 0.09375   bits=0x3fb8000000000000   == 3/32 ✓
drift = |r − mean|   = 0.03125   bits=0x3fa0000000000000   == 1/32 ✓
```

### 5.3 「为什么必须换掉 0.03」的**理由**不是操作性机制（结论仍正确且必要）

dev 写：「`0.03` 不是精确可表示的二进制小数，用它测『恰好等于』测的是舍入而非比较符」。

**实测 `3.0/100 == 0.03` 为 `true`**（同为 `0x3f9eb851eb851eb8`）⇒ **字面量不精确本身不构成理由**。

真正的机制是：`drift` 的算路是**「均值（÷3）→ 相减」的链**，而不是判据一那样的单次除法。
逐组实测（前三期同一残差、本期高 3pct、阈值 0.03）：

| 选数 | drift | `== 0.03` |
|---|---|---|
| **5% → 8%** | `0.029999999999999991951`（`…eb6`，**低 2 ULP**） | **false** |
| 6% → 9% / 7% → 10% / 8.5% → 11.5% | `…eb8` | true |
| **6.25% → 9.375%（dev 采用，阈值 1/32）** | `0.03125` | **true** |

⇒ 四个十进制选数里**有一个会静默落在阈值下方**，那条「恰好等于」的测试就测不到边界、变异存活，
而它看上去和其他三个一模一样。**风险真实、不可凭直觉预测，dev 的选择是必要的。**

**与 TASK-004 的对照**：那次我实测出「必须用 2 的幂」**不必要**（判据一是单次除法，十进制照样精确）；
这次同一形状的纪律**是必要的**（漂移是减法链）。
⇒ **这条纪律的必要性取决于算路，不取决于阈值字面量。** 一次除法可以宽容，一条减法链不行。

### 5.4 声称③：对照组从 12.5% 改 10.9375% —— **修正正确**

```
初稿 12.5%:    r = 0.125            r > 0.12 ? true   ⇒ 确实先被判据一(tolerance_exceeded)命中
改后 10.9375%: r = 0.109375 (=7/64)  r <= 0.12 ? true  ⇒ 不被判据一拦截
               drift = 0.046875 (=3/64)  drift > 1/32 ? true ⇒ 确实走到漂移分支
```

且测试本身有 `assert.Contains(c.Reason, "drift_exceeded", "必须是漂移判据命中，而不是绝对容差")`
把这件事钉住，不是靠注释自觉。

### 5.5 声称④：`0.0857` 的不确定标注 —— **恰当，且我能把它再收紧一格**

独立复算（不经被验实现，直接用 golden 里的五个数）：

```
golden2025  total=264100  sum=240179  残差=0.090576   ⇒ 与计划 9.06% / spec 0.0906 一致 ✓
golden2020  total=145500  sum=133038  残差=0.085649   ⇒ 与 M0 引用的 0.0857 不等
```

**(a) 该不确定不影响任何断言。** 全仓 `grep 0.0857` 只命中 `store_test.go:1111/1137/1441`
与 `validate.go:173`。前三处是 **Store 层手工构造的 `ValidationReport` fixture**
（TASK-003 期既有，与 golden2020 无派生关系），第四处是**注释**。本任务全部断言用的是
`depositWith(pct)` 构造出的残差。

**(b) golden2025 = 0.090576 确认。**

**我能补的一格**：`0.085649` 四舍五入到四位是 **`0.0856`，不是 `0.0857`**
⇒ **「同一计算的舍入差异」这个解释被排除**，dev「倾向另一期」的方向因此得到加强；
但仍**不能证实**（M0 可能对同一期用了略有不同的源数）。所以「倾向但未证实」是**准确的校准**。

**如实评价**：这是本 Sprint 我见到的最好的一次不确定处理 —— 不确定的位置标对了
（不确定的是「0.0857 属于哪一期」，而不是「残差算得对不对」；后者它算了并给出可核对的数字）。

### 5.6 超集字串的对抗性验证

| 变异 | 理由串 | 表驱动 | 可分性测试 |
|---|---|---|---|
| E2 | 两处都写 `drift_skipped:no_prior_period` | **红**（`:517`） | 红 |
| E1 | 两处都写 `drift_skipped:no_prior_period\|insufficient_history` | **红**（不含完整的 `drift_skipped:insufficient_history`） | 红 |
| **E3** | 两处都写 `drift_skipped:no_prior_period drift_skipped:insufficient_history`（**同时含两个完整期望子串**） | **绿** | **红**（`:544` NotEqual、`:545` NotContains） |

⇒ **E3 才是 dev 说的那个场景，而它确实只被可分性测试杀死。声称成立。**
（我头两次构造的串都被表驱动抓到了，记此以说明结论不是随手一试得来的。）

---

## 6. done_criteria 覆盖矩阵

| # | 完成标准 | 对应测试 | 消融（致红行号） | 判定 |
|---|---|---|---|---|
| functional[0] | 映射表六种情形 + 前两行 Reason 可分 | `TestDepositSumCombinesTwoCriteria`（6 子测试）+ `TestDepositSumDistinguishes…` | **F2** 无历史→skipped ⇒ `:513`+`:569`；**D7** `len(hist)` 边界 ⇒ 4 处；**E3** 超集串 ⇒ **仅** `:544`/`:545` | PASS |
| functional[1] | `Check.Value` 恒为绝对残差占比 | 同上 + 边界测试 | **F4** Value 改 r/2 ⇒ 表驱动 6 行全红 + `:572` | PASS |
| functional[2] | `deposit_sum` 在 `monetary_hierarchy` 之后，gates 六行 | `TestReportAlwaysContainsEveryGate` | **G1** 挪到表末尾 ⇒ `:402` | PASS |
| boundary[0] | 残差恰好 = ±12% 判 passed + 略超 failed | `TestDepositSumBoundaryIsInclusive` 前两子测试 | **D6** `r >`→`>=` ⇒ `:569`；选数精确性见 §5.1 | PASS |
| boundary[1] | 算不出残差的历史期不计入均值 | `TestDepositSumIgnoresUncomputablePriors` | **F1** 当成 0 计入 ⇒ `:660`（均值被拉到 6%）+ `:662` | PASS |
| error_handling[0] | 总额为 0 记 `skipped{zero_denominator}`，无 Inf/NaN | `TestDepositSumSkipsOnZeroDenominator` | **F3** 不短路 ⇒ `:679`/`:680`（`residual +Inf exceeds 0.1200`）/`:682` | PASS |
| non_functional[0] | 新闸门不在真实数据上误报 | `TestValidateOnGoldenSamples` / `TestEverySkippedCheckHasReason` | 均 PASS；独立复算 golden2025 残差 0.090576 ≤ 0.12 | PASS |
| non_functional[1] | RED 因预期原因；gofmt/vet/整包绿 | — | RED 独立复现见 §7 | PASS |

> **超出 DoD 的追加守卫（dev 主动补，我复验有效）**：漂移阈值边界（§3）。

---

## 7. RED 因果的独立复现

自己把 `deposit_sum` 从 gates 表摘掉后跑：

```
--- FAIL: TestDepositSumCombinesTwoCriteria/无历史，绝对值通过
    validate_test.go:512: 报告里没有 deposit_sum；实际有 5 道闸门
    （六个子测试同一原因，无编译错误、无 imported and not used）
```

与 discovery 记录的原文**逐字同形**。

## 8. 主工作区完整性

```
523a3bec85e2954cbf5c7abfa9ed3052bfb68c0fd63252a973c4f425f68f1eac  validate.go
e8cafc0b7e0fb820d32d0562d57e6d5a4e98d1c7bdc79742a8fc6f26bc17ca7a  validate_test.go
$ git status --porcelain  → 仅 .arcforge/ 条目
```

与开工时逐字节一致。全部变异只在 `/tmp/mut-005` 内进行。

## 9. 未据以判不通过的项

- validator 的 2 条 `scope-writes-outside-packages`（AD-035-4 已知假阳），整体 `exit=0`。

---

## 结论：**VERIFIED**

7 条 done_criteria 全部有对应测试、全部有消融证明断言在守卫、全部核对致红归因。
M2 双态对照坐实 dev 补的漂移边界测试是唯一杀手。四条声称全部核实为真，
其中「换用 1/32」**结论必要而理由需修正**（§5.3），「0.0857 不确定」**标注恰当且被再收紧一格**（§5.5）。

**对 Leader 封闭性论证的复核（§4）**：两个前提均成立，一一对应成立；
但**扫描规则与表不是同一个集合**（规则产出 9 个比较，表收 6 个），
执行的实际单位是「阈值比较」而非正文声称的「数值比较」——
`:138` 的两处（monetary，每期都在跑）是同形且语义更实在的缺口，不在表也不在 F17。
#5/#6 不返工的裁定**不反对**，并已给出更强的依据（空表被 `assert.Empty` + TASK-001 消融机制钉住），
附一条改进建议：**把 F17 的提醒挂到那条必然会响的测试的注释上**。
