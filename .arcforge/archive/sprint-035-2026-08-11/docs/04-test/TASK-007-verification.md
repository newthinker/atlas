# TASK-007 验证报告 —— 口径豁免应用、Save 接线与 ULP 契约

- **验证者**：test-agent-24（Reality Checker，默认判定 NEEDS WORK）
- **被验对象**：`master @ 4547631dc8fdf03aeb97e84635a4174a8f5cf05c`（全 sha）
- **结论**：**VERIFIED（8/8 PASS）** —— 本 Sprint 最后一个开发任务

**消融：20 个变异 → 16 KILLED / 4 SURVIVED，而 4 个存活体全部是 `CONTRACTS.md` 已明写为
「无守卫」的那四个**（2 个裁定不补 + 2 个留 M1c）。**本轮 harness 已按 dev 建议加装计数自证。**

---

## 0. 漂移核实与 scope

```
$ git rev-parse HEAD → 4547631dc8fdf03aeb97e84635a4174a8f5cf05c
$ shasum -a 256 .arcforge/discoveries/TASK-007.json
3298ac99abdac22d778b73f8f37a530d2a9e0459cef62576c2939073bcafcdf3   （= verify_baseline 记录值）
$ git show --numstat --oneline 4547631
91	0	internal/hestia/CONTRACTS.md
9	0	internal/hestia/thresholds.go
22	0	internal/hestia/thresholds_test.go
34	0	internal/hestia/validate.go
244	5	internal/hestia/validate_test.go
```

零漂移；实改 5 文件 == `writes` 五项。

## 1. 门禁（隔离 worktree `/tmp/verify-007`）

```
$ go test ./internal/hestia/ -count=1 -cover  → ok  coverage: 92.0%
$ go test ./internal/hestia/ -race -count=1   → ok
$ gofmt -l ; go vet ; go build ./...          → 均无输出 / 通过
```

本任务 8 条测试（含 4 个子测试）全 PASS。

---

## 2. 事项① —— 收窄执行的边界：**画对了**

Leader 的原指令是「把 `validate_test.go` 里『精确可表示』一类措辞**统一**改成
『两边舍入到同一个 double』」，并自陈这个推广是错的。我先独立复算六个值：

```
0.02     bits=0x3f947ae147ae147b  double精确值=0.0200000000000000004163336  精确? false
0.03     bits=0x3f9eb851eb851eb8  double精确值=0.0299999999999999988897770  精确? false
0.12     bits=0x3fbeb851eb851eb8  double精确值=0.1199999999999999955591079  精确? false
0.0625   bits=0x3fb0000000000000  double精确值=0.0625000000000000000000000  精确? true
0.03125  bits=0x3fa0000000000000  double精确值=0.0312500000000000000000000  精确? true
50.5     bits=0x4049400000000000  double精确值=50.5000000000000000000000000 精确? true
```

（方法：`big.Float(prec=200)` 解析十进制字面量的**精确值**，与 double 转回的精确值逐位比较 ——
不是看位模式相同，而是判定「这个十进制数能否被 double 无损表示」。三方数据一致。）

### 逐处对账（全包五处相关措辞）

| 位置 | 谈的值 | 该值是否精确 | 原措辞是否成立 | dev 是否改 | 判定 |
|---|---|---|---|---|---|
| `validate_test.go:281/292`（T4 corp 边界） | `0.0625` | **精确** ✓ | 「精确可表示」**成立** | 未改 | ✅ 正确 |
| `validate_test.go:319`（T4 yoy 边界） | `50` / `50.5` | **精确** ✓ | 成立 | 未改 | ✅ 正确 |
| `validate_test.go:557`（T5 deposit 边界） | `0.12` | 不精确 | **原文并未声称它精确** —— 写的是「同一个 float64」「IEEE 除法正确舍入…与 0.12 的字面量解析结果一致」 | 未改 | ✅ 正确 |
| `validate_test.go:592`（T5 drift 边界） | `0.03` | 不精确 | 「`0.03` 不是精确可表示的二进制小数」**是真话** | 未改 | ✅ 正确（不在「措辞错误」范围内） |
| **`validate_test.go:766`（T6 stock 边界）** | **`0.02`** | **不精确** | 原文「此处选**精确可表示**的比例是求稳」**错** | **已改** | ✅ 正确 |

⇒ **改的那一处确实该改，没改的四处确实不该改。收窄的边界画得准确。**
若照原指令全量替换，会把 `:281`/`:319` 两处**正确的**「精确可表示」改成错的。

### ULP 数复核

```
400→408 / 300→306 / 250→255 / 350→357  ULP差 = 0    ==0.02? true
123→125.46                             ULP差 = -15  ==0.02? false
400.1→408.102                          ULP差 = -35  ==0.02? false
```

**两个数（−15 与 −35）与四组 0 全部复核一致。** dev 补的 −35 属实。

---

## 3. 事项② —— harness 静默失效：建议成立，**我自查发现自己也有同洞的变体**

### 3.1 对建议的评估：**成立，且应与编译闸并列**

失败模式的分类是准确的：

| | 输出表现 | 被发现的概率 |
|---|---|---|
| **假 KILLED** | 打出一行**假话** | 较高 —— 假话会被追查（本 Sprint 已被闸拦下 4 次） |
| **静默失效** | **少了一行**，其余 ✓ 正常 | **低** —— 没有任何一行是错的，只是缺席 |

四道闸都是「对**已经发生的**这次变异」做判断（生效/语法/编译/还原），
**结构上无法回答「这次变异有没有发生」**。计数自证（发起数 == 结论数）补的正是这一层，
且它是**唯一**不依赖人逐块阅读的机制。

### 3.2 我自己的 harness：实测有同洞的变体

不读代码下结论，直接跑：

```
--- 情形 a：$W 不存在（如 worktree 已被拆）---
/tmp/old_harness_test.sh: line 2: cd: /tmp/DOES-NOT-EXIST: No such file or directory
[本应出现的 ########## 块：无]

--- 情形 b：正则匹配不上（变异没落盘）---
########## 自查-b 故意用匹配不上的正则 (validate.go) ##########
!!! 生效闸：sha 未变 —— 作废
```

⇒ **情形 b 我有闸能捕获（sha 生效闸），情形 a 没有** ——
`cd "$W" || return 1` 会**不产出任何块**，只剩 bash 自己的一行 stderr。
若 stderr 被重定向或淹没在长输出里，那条变异就从计数里悄悄消失了。

**诚实交代**：我历次报的 KILLED 之所以可信，靠的是**我逐块读了 `--- diff ---` 与
`Error Trace:`** —— 一条没跑的变异既没有 diff 也没有 Error Trace，我会看出来。
但那是**人工核对，不是机制**。dev 的建议严格更强。

### 3.3 本轮已实装

本任务的 harness 加了 `MUT_COUNT`/`RESULT_COUNT` 与 `tally`，每批收尾打印：

```
===== 计数自证：发起 3 条，产出结论 3 条 =====  ✓ 一一对应
===== 计数自证：发起 4 条，产出结论 4 条 =====  ✓ 一一对应
===== 计数自证：发起 6 条，产出结论 6 条 =====  ✓ 一一对应
```

另加了自标签的 `[结论: KILLED]` / `[结论: **SURVIVED**]` 行，使每块自述结论而非靠读者推断。
并把 `cd` 失败、文件不存在、`perl` 失败三条路径都改成**打印一行再 return**。

---

## 4. 事项③ —— 「主动不做」是否正当：**正当，而且是量出来的**

dev 称计划 Step 1 的 `TestValidateAlwaysReportsEveryGate`「与 TASK-004 那条重复且**更弱**」。
我没有停在读代码，而是把计划那条**原形态写进一次性副本**实测：

```
(a) 未变异实现：计划那条 PASS ✓（说明我复现的形态正确）
(b) 从 gates 删掉一道闸后：
    --- PASS: TestPlanValidateAlwaysReportsEveryGate   ← 计划那条（len(gates) 自指）
    --- FAIL: TestReportAlwaysContainsEveryGate        ← TASK-004 那条（字面量）
```

⇒ **计划那条在删闸场景下完全抓不到**（`len(rep.Checks) == len(gates)` 与逐位
`gates[i].id` 两边一起缩水），TASK-004 的字面量那条抓得到。**「更弱」成立，不是省事的说辞。**

而 dev 把精力放到了真正新增的表面（豁免分支），由 A1 证明有效。**判定：正当。**

---

## 5. 事项④ —— 「互补而非重复」的证伪判据：**成立，且变异的是生产逻辑**

预先声明的判据：若 A1（豁免分支改 `continue`）下两条**同时**转红，则该句为错。

A1 实测失败列表：

```
--- FAIL: TestCaliberExemptionRecordsSkipNotPass
--- FAIL: TestReportKeepsEveryGateUnderExemption   （:1033 豁免只改判定，不得让闸门从报告里消失）
```

**`TestReportAlwaysContainsEveryGate` 未出现在失败列表 ⇒ 保持绿。** 判据未被触发，该句成立。

**是否退化成 M9 型无效实验**：A1 改的是 `validate.go` 的**豁免分支**（生产逻辑），
不是任何断言。⇒ **不是**无效实验，与 TASK-004 M9 的界线踩对了。

---

## 6. 事项⑤ —— `CONTRACTS.md` 的 8+4 收口表：**准确，我在当前提交上逐条复核过**

### 6.1 总数与归属

按 F19 方法论（**最宽正则生成候选集，过滤只在标注阶段**）扫描当前提交的 `validate.go`：
**10 行 / 12 个比较** —— 与表的总数一致，归属逐条对得上。

### 6.2 **不沿用旧结论**，12 条全部在当前提交重跑

表是**留存的断言**，而我在 T4/T5/T6 得到的结论是在**别的提交**上取的；
TASK-007 动过 `validate.go`（加了豁免分支与注释块），旧结论可能过期。故全部重测：

| # | 表达式 | 表里标注 | **当前提交实测** |
|---|---|---|---|
| 1–2 | `m2 > m1 && m1 > m0` | ✅ 有守卫（本 Sprint 收尾补） | **KILLED**（A7，且**只有** `TestMonetaryHierarchyRejectsEquality` 转红 ⇒ 它是唯一守卫） |
| 3 | `r > DepositSumTolerance` | ✅ | **KILLED**（`:570`） |
| 4 | `len(hist) < minDriftHistory` | ✅ | **KILLED**（4 处） |
| 5 | `drift > DepositSumDriftMax` | ✅ | **KILLED**（`:614`/`:616`） |
| 6 | `r <= CorpLoanTolerance` | ✅ | **KILLED**（`:307`） |
| 7 | `r <= StockContinuityMax` | ✅ | **KILLED**（`:792`） |
| 8 | `worst <= YoYSanityMax` | ✅ | **KILLED**（`:330`） |
| 9 | `a > worst` | ❌ 裁定不补 | **SURVIVED** ✓ 与标注一致 |
| 10 | `len(s) <= n` | ❌ 裁定不补 | **SURVIVED** ✓ |
| 11–12 | `v < r.Min \|\| v > r.Max` | ❌ 留 M1c | **SURVIVED ×2** ✓ |

⇒ **8 有守卫 / 4 无守卫，逐条属实；空缺都明写了理由**（F19 方法论第 5 条满足）。

### 6.3 不写行号的决定：**赞同**

理由与「验证命令不得钉 `HEAD`」同构 —— 行号是必然过期的锚。本任务自己就是证明：
派单给的 `:139/:193/…/:450` 在加了注释块与豁免分支后**全部漂移**
（我实测的当前行号是 `:173/:227/…/:484`）。按归属与表达式排的表则不受影响。

---

## 7. done_criteria 覆盖矩阵

| # | 完成标准 | 对应测试 | 消融（致红行号） | 判定 |
|---|---|---|---|---|
| functional[0] | `knownCheckIDs()` 从 `gates` 派生、七个 ID 且顺序正确 | `TestGatesMatchContractedCheckIDs` | **A4** 顺序反转 ⇒ `:828`+`:1033`；**N2** 改成手写清单 + 删一道闸 ⇒ `:403`+`:1033`（**注**：此时 `TestGatesMatchContractedCheckIDs` 自己是绿的 —— 「派生」这条由与报告的交叉比对守住，见 §9.2） | PASS |
| functional[1] | 每道闸门都出现在报告里、逐位对应（v1 大量跳过的输入上测） | TASK-004 `TestReportAlwaysContainsEveryGate` + 本任务 `TestReportKeepsEveryGateUnderExemption` | **N1** 删一道闸 ⇒ `:403`/`:828`/`:1027`；**A1** ⇒ `:1033`。未加计划那条更弱的重复测试，理由经实测（§4） | PASS |
| functional[2] | 命中豁免记 `skipped{caliber_exemption:<v>}` 非 passed、保留 Value、`rep.Passed` 仍 true | `TestCaliberExemptionRecordsSkipNotPass` + `TestReportKeepsEveryGateUnderExemption` | **A2** 记 passed ⇒ `:848`+`:1039`；**A3** 丢 Value ⇒ `:1043` | PASS |
| boundary[0] | 豁免不外溢（接入 Validate 后端到端） | `TestCaliberExemptionDoesNotLeakToOtherPeriods` | **A6** `==`→`<=` ⇒ `:868`+`:870`（同时打红 TASK-001 的 `thresholds_test.go:121`，两层互补） | PASS |
| error_handling[0] | `SkipChecks` 未知 ID 返回 error 且含原文；ID 从 `gates` 派生；未破坏 TASK-001 的豁免校验 | `TestExemptionRejectsUnknownCheckID` | **A5** 删掉校验循环 ⇒ `:886`；TASK-001 的 `TestThresholdsRejectMalformedExemptions` 4 子测试**仍全绿**（我实跑确认） | PASS |
| non_functional[0] | `Validate` 产出能被 `Save` 接受（真库 v2/v1）；未过闸落 pending；`Store` 能当 `History` | `TestValidateOutputIsAcceptedBySave`（2 子测试）+ `TestFailedValidationLandsInPending` | **A9** `need()` 的 skip 去掉 Reason ⇒ `:942 Received unexpected error / Validate 的产出必须能被 Save 接受` ⇒ 该测试确实在查 `Save` 的额外要求，不是走过场 | PASS |
| non_functional[1] | ULP 契约用**变量**钉住误差存在 + 包内注释写下契约 | `TestTrillionConversionCarriesULPError` + `validate.go` 顶部注释块 | 我把 `trillion := 4.81` 改成 `const trillion = 4.81` ⇒ **该测试立即 FAIL**（`Should not be: 48100`）⇒「必须用运行时变量」这条前提是**自我强制**的，不是靠注释自觉。另实测：常量表达式 `4.81*10000 == 48100` 为 **true**，变量为 **false**（−1 ULP） | PASS |
| non_functional[2] | `CONTRACTS.md` 三节 + ㈠ monetary 两个边界 + ㈡ 提醒挂到绊线；gofmt/vet/`-race`/`go build` | `CONTRACTS.md` Sprint 035 一节 + `TestMonetaryHierarchyRejectsEquality` + `thresholds_test.go:36-58` 的绊线注释 | **A7** ⇒ 两个子测试均红（`:1001`/`:1003`），且**全量套件只有它转红** ⇒ 既复现我在 T5 的 SURVIVED 结论，也证明它是该边界的唯一守卫。绊线注释我逐行读过：指向 F12/F17/F19，**逐条列出了要补的三件事**，并写明「挂在绊线上而不是挂在需要被记起来的文档里」 | PASS |

---

## 8. RED 因果的独立复现

删掉 `knownCheckIDs` 后跑整包：

```
internal/hestia/thresholds.go:109:35: undefined: knownCheckIDs
internal/hestia/validate_test.go:828:24: undefined: knownCheckIDs
internal/hestia/validate_test.go:1033:18: undefined: knownCheckIDs
FAIL	github.com/newthinker/atlas/internal/hestia [build failed]
```

与 discovery 记录的形态一致，无 `imported and not used` 干扰（本任务未新增任何 import）。

## 9. 两点观察（**不是缺陷**，但先说清楚以免 QA 误判）

### 9.1 同包内留有两处「过强的因果」措辞，其中一处与本任务新写的契约注释相左

- `validate_test.go:281`：「全部用 2 的幂…**否则**测的是浮点舍入」——「否则」蕴含**必要性**，
  而我在 TASK-004 实测证否（换成 `100000/5000, tol=0.05` 照样杀得掉变异）。
- 本任务新加的 `validate.go:22` 则写：「**不取决于算路长短**，取决于参与运算的量是否精确」。
- `validate_test.go:592`：「`0.03` 不精确**故**改用自定义阈值」——事实真、因果错
  （真机制是**减法链**，我在 TASK-005 实测 `3.0/100 == 0.03` 为 true）。

**这两处不在 Leader 指令的覆盖范围内**（它们对自己谈的值说的都是真话），
**dev 不改是对的**。但两种说法现在并存于同一个包，QA 可以决定是否统一。

### 9.2 `TestReportKeepsEveryGateUnderExemption` 的 `assert.Equal(knownCheckIDs(), gotIDs)` 是自指形态

一眼看去像 TASK-004 M5 修掉的那个毛病（两边同源、一起缩水）。**实测表明它不是**：

- **N1**（删一道闸、`knownCheckIDs` 仍派生）：两边同缩，该断言不吃重 —— 但删闸由
  `TestGatesMatchContractedCheckIDs`（字面量 7 个 ID）与 TASK-004 那条（字面量）双重兜住。
- **N2**（`knownCheckIDs` 写死 + 删一道闸，即 DoD functional[0] 真正关心的失效模式）：
  两边**不再同源**，该断言**立刻致红**（`:1033`）。

⇒ 它在**该判别的那个维度上**是判别的。记此以免 QA 按形态误判。

## 10. 主工作区完整性

```
84f502268a7056c1cec9cc0a3e566f55a802f38ea338c74c2c4541f3a60b10a4  validate.go
1699bd7acad841c19165b1a163a50c5b79687449af8698fb6123b94ea0598c84  thresholds.go
7c73ef28959c75040bfc623214332ba65ede68bfb0980d53e786354c33e3c0ec  validate_test.go
1fdf0aa3c3d022ce83a08eb6bcf54fe4459568da6fd4e70e8c4f294f3e196ca6  thresholds_test.go
3b5b3d7c69d33e4fc5a39d4a28f6a3468fe49b4a13854c08fb6ba0d04c452397  CONTRACTS.md
$ git status --porcelain  → 仅 .arcforge/ 条目
```

与开工时逐字节一致；全部变异只在 `/tmp/mut-007` 内进行。

## 11. 未据以判不通过的项

- validator 的 5 条 `scope-writes-outside-packages`（AD-035-4 已知假阳），整体 `exit=0`。
- dev 申报未运行 `npx gitnexus analyze`（计划 Step 11）——属仓库级操作，已申报留给 Leader，
  不属 done_criteria。

---

## 结论：**VERIFIED**

8 条 done_criteria 全部有对应测试、全部有消融证明断言在守卫、全部核对致红归因。
20 个变异 16 KILLED、4 SURVIVED，而**存活的四个恰是 `CONTRACTS.md` 已明写为无守卫的那四个**
——即交付物对自己的缺口的描述与实测**完全一致**，这本身是本任务最有分量的一条证据。

Leader 点名的五件事逐条成立：①收窄边界画对（逐处对账 + 六值复算 + 两个 ULP 数复核）；
②建议成立，**我自查发现自己也有同洞的变体并已实装计数自证**；③「不做」正当且经实测；
④证伪判据成立且变异的是生产逻辑；⑤8+4 表准确，我在**当前提交**上逐条重跑而非沿用旧结论。
