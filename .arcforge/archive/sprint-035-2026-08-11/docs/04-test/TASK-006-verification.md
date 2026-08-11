# TASK-006 验证报告 —— stock_continuity 与三种跳过理由的优先级

- **验证者**：test-agent-24（Reality Checker，默认判定 NEEDS WORK）
- **被验对象**：`master @ 9ec8a6ffcfc47d3a5124e9755672019d3a8205b8`（全 sha）
- **结论**：**VERIFIED（7/7 PASS）**

**消融：8 个变异全部 KILLED，本任务代码零存活。** 另做 3 个探针、1 次选数替换实验、1 次 RED 复现。

---

## 0. 漂移核实与 scope

```
$ git rev-parse HEAD → 9ec8a6ffcfc47d3a5124e9755672019d3a8205b8
$ shasum -a 256 .arcforge/discoveries/TASK-006.json
5c3161ba467b576bb514fa491692a8a9284f248b1f16a014ddcd047cc01b9360  （= verify_baseline 记录值）
$ git show --numstat --oneline 9ec8a6f
52	0	internal/hestia/validate.go
124	3	internal/hestia/validate_test.go
```

零漂移；实改 2 文件 == `writes`。**`wantGateIDs` 是追加**：新增一行 `"stock_continuity"`
（紧跟 `corp_loan_reconcile`，与 gates 表一致），`require.Equal` 原样保留，3 处删除只是
两行注释与「六道→七道」的措辞。**gates 至此七道，与 M0 契约样本的 check ID 数量一致。**

## 1. 门禁（隔离 worktree `/tmp/verify-006`）

```
$ go test ./internal/hestia/ -count=1 -cover  → ok  coverage: 91.9% of statements
$ gofmt -l ; go vet                          → 均无输出
```

本任务 3 条测试 / 9 个子测试全 PASS；`gateStockContinuity` 覆盖率 **100%**。

> 独立佐证：还原后 `validate.go` sha256 `f28aa2d95312f56f93901d512bd79ae9e01120300787a5bc800eb61e3db8d740`，
> 与 dev 在 discovery 记的 `control_group` **逐字相同**（本 Sprint 第二次出现这种独立对上）。

---

## 2. 头等：M7 —— dev 为自己写下的理由设计的证伪实验

### 2.1 复验结果：KILLED，且**因果正确**

```
--- FAIL: TestValidateOnGoldenSamples/2025_全年_rule@v2
        validate_test.go:62   Should be true
--- FAIL: TestValidateOnGoldenSamples/2020_上半年_rule@v1
        validate_test.go:62   Should be true
--- FAIL: TestGatesSkipOnAbsentFields
        validate_test.go:161  Should be true
        Messages: 字段天然缺失不该让整期不过闸
```

**因果核对（Leader 点名要判的那一步）**：致红的是 `:62` 与 `:161` 的
`assert.True(t, rep.Passed)` —— 而**不是** `:59-61` 那个逐个检查
`assert.NotEqual(CheckFailed, c.Status)` 的循环。M7 没有改变任何 Check 的 Status，
只改了聚合逻辑；若红是别的原因引起，状态断言也会跟着响。**它们全都是绿的。**
⇒ **M7 转红确实由「skipped 被当成阻断」引起。**

### 2.2 实验设计本身是否真能证伪那句话 —— **能，而且我把它加强了一格**

dev 的注释是：「『跳过不阻断』已由 `TestValidateOnGoldenSamples` 守住，故此处不必重复断言
`rep.Passed`」。M7 破坏的正是「跳过不阻断」，观察的正是那条测试是否转红。设计对路。

**我另做了一个探针，把结论从「冗余」推进到「那条断言原本就是坏的」**：
实测计划原写法的四种构造下 `rep.Passed` 各是什么 ——

```
本期没有社融板块，但有历史   键数=53  rep.Passed=false  completeness=failed (missing 1: tsf_stock)
v1 期次且无历史            键数=53  rep.Passed=false  completeness=failed (missing 1: tsf_stock)
本期有社融，但库里没有历史   键数=54  rep.Passed=true   completeness=passed
上一期没有社融板块          键数=54  rep.Passed=true   completeness=passed
```

⇒ 在 2/4 构造下，`assert.True(t, rep.Passed)` **在正确实现下也必然失败** ——
它不是「多余的一条断言」，而是**一条不可满足的断言**。

**这让 ㈠ 的性质更清楚**：dev 移除的不是一个有效守卫，而是一个**坏掉的守卫**；
再用 M7 证明它想守的性质另有守卫。两步合起来才构成完整论证，dev 两步都做了。

**如实评价**：这是本 Sprint 我见到的最高级的一次「别把理由写强」——
前几轮的做法是「结论对但理由未经检验」（TASK-003/004 三次）、
然后是「不确定就标不确定」（TASK-005），这次是
**先写下理由、预先声明什么结果算证伪、再去跑**。M7 若存活，那句注释就必须改写；
它没存活，所以注释成立。**做到了。**

一点补充观察：M7 之所以是个好实验，是因为它**变异的是被引用的那条测试所依赖的生产逻辑**，
而不是那条测试本身 —— 后者会退化成 TASK-004 的 M9（变异判据、无效实验）。这条界线 dev 踩对了。

---

## 3. 其余四条

### 3.1 M1 优先级对调 —— 「只打红一条」**成立**

```
--- FAIL: TestStockContinuitySkipReasons/v1_期次且无历史：两个理由同时成立，报最根本的那个
        validate_test.go:753  Not equal
```

全包**唯一**致红子测试，正是 DoD `boundary[1]`（spec `boundary[0]`）点名的那个用例。
另外三个跳过子测试都没红 —— 说明这条断言是**专为优先级设计的**，不是顺带覆盖。

### 3.2 ㈠ vs ㈡ 的理由 —— **成立**

dev 的理由：「㈡也能变绿，但它把『本闸跳过时报什么理由』的正确性挂在 completeness 恰好不失败上」。

我实测了㈡是否真能绿：

```
[方案㈡ 实测] extractorV1 + golden2020 构造:
  v1 无 tsf_stock     键数=27  rep.Passed=true   completeness=passed
  v1 有 tsf_stock     键数=28  rep.Passed=true   completeness=passed
```

**㈡确实能绿**（`requiredFields(v1)` 只查缺失不查多余，28 键照样 passed）。
所以 dev 不是因为㈡不可行才选㈠，而是因为耦合 —— 理由成立：
㈡ 之下，`stock_continuity` 跳过理由的测试会**因为 completeness 的行为变化而红**，
那是与被测对象无关的红。㈠ 的观测对象与被测对象一致。

### 3.3 边界选数的转述 —— **准确**，且我做了替换实验

dev 写的是「此处求稳；本闸算路只有一次减法加一次除法，严格说并非必要；
但先求均值再相减那类链式算路里就是必要的」，并引我 TASK-005 实测的
`0.029999999999999992`（低 2 ULP）作反例。**三处转述我逐条核过，均准确。**

**替换实验**（直接验「并非必要」）：把 `{"恰好在阈值上", 400, 408, ...}` 改成 `300, 306`
（同为比例 1/50，非同一组数）——

- 未变异实现 + 新选数：**PASS**
- `r <= max` → `<` 变异 + 新选数：**仍然 FAIL**（`恰好在阈值上`）

⇒ 「严格说并非必要」成立。

**但我要在这里加一条修正（见 §5 发现 2）**：真正的不变量不是「算路简单」，而是
**「参与运算的量本身是否精确」** —— 本闸在非整数输入下同样会失效。

### 3.4 零分母 vs `prior_absent_field` 的区分 —— **成立**

Leader 要我消融确认它真能区分两者，我做了**两个方向**：

| 变异 | 结果 |
|---|---|
| **M4a** 零分母分支改报 `prior_absent_field:tsf_stock`（值仍是 skipped） | **KILLED** ⇒ `:800 Not equal / 上一期为 0 必须走零分母分支，而不是 prior_absent_field——字段在，只是值为 0` |
| **M4b** 删掉 `prev == 0` 分支，任其算出 Inf | **KILLED** ⇒ `:799`+`:800`+`:803 Value 不得是 Inf/NaN` |

M4a 是关键的那个：它**只改理由字串、不改状态、不产生 Inf**，只有那条带解释的
`assert.Equal` 能抓到。⇒ 两种处境确实被区分开了，不是靠「反正都是 skipped」蒙混。

**M5**（`prior_absent_field` 退化成 `no_prior_period`）同样 KILLED（`:753`）。

---

## 4. done_criteria 覆盖矩阵

| # | 完成标准 | 对应测试 | 消融（致红行号） | 判定 |
|---|---|---|---|---|
| functional[0] | 环比判定生效含边界方向；`Value` 为变化率 | `TestStockContinuityDetectsJump`（4 子测试） | **M2** `r <=`→`<` ⇒ `:784`（`恰好在阈值上`） | PASS |
| functional[1] | 存量下跌同样判跳变 | 同上「存量下跌也算跳变」 | **M3** 去掉 `math.Abs` ⇒ `:784`+`:786`（`0.05 vs -0.05`） | PASS |
| functional[2] | 位于 `corp_loan_reconcile` 之后，gates 恰好七道 | `TestReportAlwaysContainsEveryGate` | **G1** 挪到表末尾 ⇒ `:403` | PASS |
| boundary[0] | 三种跳过理由优先级；**断言收窄到本闸**（不断言 `rep.Passed`） | `TestStockContinuitySkipReasons`（4 子测试） | **M5** ⇒ `:753`；四子测试全 PASS（计划原写法会 2 红 2 绿，见 §2.2） | PASS |
| boundary[1] | v1 期次同时满足①②时必须报① | 同上第 2 个子测试 | **M1** ⇒ **只** `:753` 一条 | PASS |
| error_handling[0] | 上一期为 0 记 `zero_denominator`，不得 Inf | `TestStockContinuitySkipsOnZeroDenominator` | **M4a**/**M4b** ⇒ `:799`/`:800`/`:803` | PASS |
| non_functional[0] | 两份 golden 仍绿，且理由如 DoD 预言 | `TestValidateOnGoldenSamples` | 我的探针实测（见下） | PASS |
| non_functional[1] | RED 因预期原因；gofmt/vet/整包绿 | — | RED 独立复现见 §6 | PASS |

**non_functional[0] 的独立实测**（我自己的探针，不采信 dev 的转述）：

```
golden2025/rule@v2   闸门数=7 rep.Passed=true  stock_continuity: skipped "no_prior_period"        value=<nil>
golden2020/rule@v1   闸门数=7 rep.Passed=true  stock_continuity: skipped "absent_field:tsf_stock" value=<nil>
```

与 DoD 的预言**逐字一致**。

---

## 5. 三项发现（均不构成 DoD 失败）

### 发现 1（低，措辞级）：「精确可表示的比例」这个说法本身不准

DoD `functional[0]` 与测试注释都用了「边界测试必须选**精确可表示**的比例」。实测：

```
0.02 字面量  bits=0x3f947ae147ae147b  = 0.020000000000000000416
```

**`0.02` 不是精确可表示的二进制小数。** 成立的条件是
`fl((cur−prev)/prev) == fl(0.02)` —— 即两边**舍入到同一个 double**，不是「精确」。
（这与我在 TASK-004/005 反复量到的是同一件事：IEEE 除法正确舍入，
当精确商恰等于该十进制字面量的精确值时两边必然相等。）

### 发现 2（低但可操作）：真正的不变量是「输入 × 算路」，不是「算路简单」

dev 的注释说「本闸算路只有一次减法加一次除法，严格说并非必要」。这句在**当前用例**下对，
但它给出的理由会误导后人。实测同一道闸在**非整数输入**下同样失效：

| 选数 | r | `== 0.02` |
|---|---|---|
| 400 → 408 / 300 → 306 / 250 → 255 / 350 → 357 | `0.020000000000000000416` | **true** |
| **123 → 125.46** | `0.019999999999999948375`（低 **15 ULP**） | **false** |
| **400.1 → 408.102** | `0.019999999999999878986` | **false** |

⇒ 让这组用例成立的**不是算路简单，而是 `cur−prev` 与 `prev` 都是精确的整数**。
若后人把边界用例改成「看起来更真实」的小数（比如社融存量的真实量级带小数），
**这条边界测试会静默失效**，而注释会让人以为「本闸算路简单，随便改没关系」。

**建议（交 T7 或收尾，一行注释）**：把那句改成
「用**整数**构造使 `cur−prev` 与 `prev` 精确 —— 必要性取决于**参与运算的量是否精确**，
不取决于算路长短」。这与我上一轮建议的「把提醒挂在必然会响的绊线上」同向：
注释就挂在编辑者必然会读到的那几行常量上。

### 发现 3（低）：`len()` 过滤器的危害是潜伏的

按 F19 的修正方法论（**最宽正则生成候选集，过滤只在标注阶段**）实跑 `validate.go`：
**10 行 / 12 个比较**，比按原规则（含 `len()` 排除）得到的 8 行多出两处：

- `:216 len(hist) < minDriftHistory` —— **货真价实的边界**（2 期 vs 3 期），且**有守卫**
  （TASK-005 我的 D7 打红 4 处）
- `:425 len(s) <= n`（firstN 截断）—— 无守卫，Leader 已裁定不补（文案级）

⇒ `len()` 过滤器这次**没有藏起真问题**，但 `:216` 说明它**能**藏一个真边界。
既然 F19 已改成「过滤只在标注阶段」，这条自然消解；记此作为该修正确实必要的一个实例。

### 附：当前 12 个比较的守卫状态（供 T7 收口）

| 有守卫（6） | 无守卫（6） |
|---|---|
| `:193` (T5)、`:216` (T5)、`:227` (T5 追加)、`:290` (T4)、**`:340`（本任务）**、`:374` (T4) | `:139`×2（monetary，已入 T7 `non_functional[2]`）、`:365`（yoy 并列，裁定不补）、`:425`（firstN 文案，裁定不补）、`:450`×2（magnitude 区间两端，F17 留 M1c） |

**本任务新增的唯一比较 `:340` 已有守卫（M2 KILLED）。**

---

## 6. RED 因果的独立复现

自己把 `stock_continuity` 从 gates 表摘掉后跑：

```
--- FAIL: TestStockContinuitySkipReasons/本期没有社融板块，但有历史
    validate_test.go:751: 报告里没有 stock_continuity；实际有 6 道闸门
    （九个子测试同一原因，无编译错误、无 imported and not used）
```

与 discovery 记录的原文**逐字同形**（含「实际有 6 道闸门」这个数）。

## 7. 主工作区完整性

```
f28aa2d95312f56f93901d512bd79ae9e01120300787a5bc800eb61e3db8d740  validate.go
9383afbca20911a403a5a9122373935ba02dde80bed8288b44457304fb8ef3ff  validate_test.go
$ git status --porcelain  → 仅 .arcforge/ 条目
```

与开工时逐字节一致；全部变异只在 `/tmp/mut-006` 内进行。

## 8. 未据以判不通过的项

- validator 的 2 条 `scope-writes-outside-packages`（AD-035-4 已知假阳），整体 `exit=0`。

---

## 结论：**VERIFIED**

7 条 done_criteria 全部有对应测试、全部有消融证明断言在守卫、全部核对致红归因；
**8 个变异全 KILLED，本任务代码零存活**。

Leader 点名的五件事：M7 的证伪实验**设计成立且因果正确**，我并把它加强了一格
（那条被省掉的断言在 2/4 构造下**原本就不可满足**，㈠ 移除的是坏守卫而非有效守卫）；
M1 的「只打红一条」成立；㈠ vs ㈡ 的理由成立（㈡确实能绿，选㈠是为了解耦）；
边界选数的三处转述准确，替换实验证实「并非必要」；零分母与 `prior_absent_field`
的区分由**只改理由字串**的 M4a 单独证明。

三项发现均为措辞/方法论级，不影响判定，其中**发现 2 建议交 T7 改一行注释**。
