# TASK-004 验证报告 —— Validate 骨架与五道无历史闸门

- **验证者**：test-agent-24（Reality Checker，默认判定 NEEDS WORK）
- **被验对象**：`master @ 09183243cc1864a3f2920311e55bdf3bcbbfd06c`（全 sha）
- **结论**：**VERIFIED（8/8 PASS）**，附 **5 项发现**（2 项是消融存活体）

**消融统计：20 个变异 → 18 KILLED（因果均核到断言行号）、2 存活（刻意探查缺口）、
2 次被编译闸判假 KILLED 并作废。**

---

## 0. 漂移核实与 scope

```
$ git rev-parse HEAD                     → 09183243cc1864a3f2920311e55bdf3bcbbfd06c
$ shasum -a 256 .arcforge/discoveries/TASK-004.json
15a3219b4074883c5e17cb92fb8536485e84039e5865a1283bd1733d2d58f34e
$ jq -r .verify_baseline.discovery_sha256 .arcforge/tasks/TASK-004.json
15a3219b4074883c5e17cb92fb8536485e84039e5865a1283bd1733d2d58f34e
$ git show --numstat --oneline 0918324
11	2	internal/hestia/store_test.go
262	1	internal/hestia/validate.go
463	0	internal/hestia/validate_test.go
```

HEAD 与 discovery sha256 均与基线**逐字相同** ⇒ 零漂移。实改 3 文件 == `writes` 三项，无越界。
（`M .arcforge/write-matrix.json` 是 Leader 登记 token，已排除。）

**IDE 诊断 `undefined: Validate` ×10 —— 实跑证伪。** `grep -n "^func Validate"` 得
`validate.go:58`，整包编译通过、测试全绿。以实跑为准。

## 1. 门禁（隔离 worktree `/tmp/verify-004 @0918324…`）

```
$ go test ./internal/hestia/ -count=1 -cover   → ok  coverage: 91.3% of statements
$ gofmt -l internal/hestia/                    → 无输出
$ go vet ./internal/hestia/                    → 无输出
```

本任务 14 条测试（含 24 个子测试）全 PASS。逐函数覆盖率：`Validate` / `need` / `v` /
五道闸门中的四道 / `yoyFields` / `firstN` 均 **100%**，`gateMagnitudeSanity` 84.6%（缺口即发现 1）。

## 2. 消融方法学（Leader 硬要求）

harness `/tmp/mh4.sh` 四道闸：**sha 生效 → `gofmt -l` 语法 → `go test -c -o /dev/null` 编译 → 还原 sha 核实**。
对照组（未变异副本）先验全绿。

> ⚠ **编译闸拦下我自己的 2 次假 KILLED**：
> - **C11 首版**：正则把整段 `cfg.validate()` 删掉而非移位 —— 若不察会把「删除」的结果当成「移位」的结论
> - **C15 首版**：`monetary` 恒 passed 写成丢弃变量 ⇒ `declared and not used: m2/m1`
>
> dev 本轮报告「编译闸一次未触发、无假 KILLED」—— 我以自己的闸为准，**我这边触发了 2 次**。
> 这不是对 dev 的反驳（我们的变异写法不同），只是说明这道闸在本任务上仍然是必要的。

---

## 3. done_criteria 覆盖矩阵

| # | 完成标准 | 对应测试 | 消融证据（致红行号） | 判定 |
|---|---|---|---|---|
| functional[0] | 两期 golden 无闸门 failed 且 `Passed==true` | `TestValidateOnGoldenSamples` | 该测试遍历 `rep.Checks`，**闸门全没了会平凡为真** —— 由 `TestReportAlwaysContainsEveryGate` 兜住（C5/C12 证） | PASS |
| functional[1] | 每道闸各有失败样本、Reason 含关键字 | `TestGatesRejectMalformedData` | 四道闸**各自**恒 passed 均被杀：C15'(monetary)、C18(corp)、C17(yoy)、C16(completeness)，全部红在 `:105/:108/:109` 对应子测试 | PASS |
| functional[2] | `Check.Value` 单位符合 spec 第 7 节且各有断言 | `TestCheckValueUnitsFollowSpec` | **C1** `Value: &residual`→`&r`（退回计划原文）⇒ `:253` 两个子测试全红；**C6** monetary 凭空造 Value ⇒ `:270 Expected nil, but got: (*float64)` | PASS |
| boundary[0] | corp/yoy 各「恰好等于阈值判 passed」+ 略超 failed | `TestGateBoundariesAreInclusive` | **C2** `r <= tol`→`<` ⇒ `:307`；**C3** `worst <= max`→`<` ⇒ `:330`。两条都带「这里变红说明比较符被改成了 `<`」的 message | PASS |
| boundary[1] | v1 缺字段 skipped 非 failed；completeness passed；magnitude 空表 skipped、填表即生效、单位入错误信息；**遍历走 fieldOrder** | `TestValidateHandlesEmptyValues…` / `TestMagnitudeSanity×2` | **C9** `need` 返 passed ⇒ `:376`；**C19** magnitude 空表返 passed ⇒ `:150`+`:376`；**C20** 错误信息丢单位 ⇒ `:140`。⚠ **「遍历走 fieldOrder」子句 C10 存活**（见发现 1） | PASS（附发现 1） |
| error_handling[0] | ①History 失败 `errors.Is` 可找 ②nil History 含 `NoHistory` ③配置非法**在跑闸门之前**含 `unknown` ④空 `Values` 不 panic 不 error | 四条测试 | **C7** `%w`→`%v` ⇒ `:193 Target error should be in err chain` + `:196 Expected value not to be nil`；**C4** 入口加空 Values 特判 ⇒ `:364`+`:432`。⚠ **③的「在跑闸门之前」子句 C11' 存活**（见发现 2） | PASS（附发现 2） |
| non_functional[0] | 每个 skipped 带非空 Reason；任何输入下报告行数恒等于闸门数 | `TestEverySkippedCheckHasReason` / `TestReportAlwaysContainsEveryGate` | **C8** `need` 的 skip 去掉 Reason ⇒ `:377`（**注意：不是 `TestEverySkipped…` 杀的**，见发现 3）；**C5/C12/C13** 删闸/删另一道闸/调换顺序 ⇒ 均 `:401` | PASS（附发现 3） |
| non_functional[1] | RED 因预期原因；`Validate` 登记进导出面守卫且保持精确相等；零分母 skipped；gofmt/vet/整包绿 | — | RED 独立复现（§5）；**C14** 伪造导出 `InsertRow` ⇒ `store_test.go:406`（**只红 AST 版**，reflect 版仍绿，与 dev 判断一致）；`TestCorpLoanSkipsOnZeroDenominator` PASS | PASS |

---

## 4. Leader 点名的五件事

### 4.1 事项① —— `-1800` / `-1203` 不是从实现反推

**我能证成的（三条独立观察）：**

1. **手算复核**（不依赖任何被验代码）：
   - golden2025：`48100 + 88200 + 16600 − 154700 = −1800` ✓
   - golden2020：`28200 + 48600 + 9697 − 87700 = −1203` ✓
2. **两个数出现在实现之前写下的文档里**：`independent-review.md` 第 79/81 行写着
   「**亿元（-1800）**」与「M0 契约样本已经这么记（`corp_loan_reconcile: -1203` 是亿元）」。
   时序：该文件 mtime `2026-08-11 16:51:07`，commit `0918324` 的时间是 `17:51:34 +0800`
   —— **文档早于实现整整一小时**。
3. **golden 数据本 sprint 未被动过**：
   `git log 125ad89..0918324 -- internal/hestia/golden_test.go` **输出为空**，
   且 TASK-004 的 `writes` 不含 `golden_test.go`。⇒ 期望值只能来自既有真实数据。

**我不能证成的（如实划界）**：spec 第 7 节与 M0 契约样本**本体在本仓库之外**
（`.arcforge/docs/01-design/design-spec.md` 是指针文件，指向 hestia 仓库与 Obsidian vault）。
我核到的是**仓库内的中间证据 + 算术**，没有直接读到那两份原始文档。

**断言强度**：`assert.Equal(t, tt.wantResidual, *corp.Value)` 是精确相等。
换成比例（0.0116）、换成符号相反（+1800）都会红 —— C1 实测确认。

### 4.2 事项② —— 两处边界守卫：**均 KILLED**，但选数理由不必要

C2（corp `<=`→`<`）红在 `:307`，C3（yoy `<=`→`<`）红在 `:330`，各自被对应的
「恰好等于阈值判 passed」子测试杀死。reviewer 实测的两个存活变异**确已被补上**。

**选数理由（2 的幂）我判定为「结论对、理由偏强」。** 先用探针实测位级关系：

```
dev 的选数 2^17/2^13, tol=1/16        r=0.0625              ULP差=0  r==tol:true
十进制 100000/5000, tol=0.05          r=0.050000000000000003 ULP差=0  r==tol:true
十进制 30000/900,  tol=0.03           r=0.029999999999999999 ULP差=0  r==tol:true
十进制 70000/4900, tol=0.07           r=0.070000000000000007 ULP差=0  r==tol:true
```

再做**理由的直接消融**：把边界选数整组换成 `corpTotal=100000 / exactlyAt=5000 /
clearlyOver=7000`、`tol=0.05`（全非 2 的幂），结果 ——

- 未变异实现 + 十进制选数：**PASS**
- C2 变异（`<=`→`<`）+ 十进制选数：**仍然 FAIL**（`:307`，同一条 message）

⇒ **2 的幂不是必要条件。** 机制上的原因是：本测试定义域内所有中间量都是远小于 2^53 的整数
（float64 精确表示），且 IEEE-754 除法是**正确舍入**的 —— 当精确商恰等于某个十进制字面量的
精确值时，两边必然舍入到同一个 double。

**这不改变判定**：该纪律是无害且合理的默认做法，且真正的风险场景确实存在
（比例写不成精确字面量，如 1/30；或算术链离开 2^53 整数域）。只是在**这条测试里**它不吃重。

### 4.3 事项③ —— 空 `Values` 回归防线：**KILLED**

C4（入口加 `if len(obs.Values)==0 { return Passed:true }`）同时打红两处：
`validate_test.go:364 Should be false / 整期没有数据必须不过闸`（空 map 与 nil map 两个子测试）
与 `:432`（报告行数）。spec 第 9 节的设计断言确有防线。

### 4.4 事项④ —— 删闸变异 **KILLED**，且自指尺子已清干净

- **C5**（删 `magnitude_sanity`）⇒ `:401 Not equal / gates 表本身必须恰好是这五道`，
  即字面量 `require.Equal(wantGateIDs, gateIDs())` 那条。**修正确实生效。**
- **C12**（改删 `monetary_hierarchy`）⇒ 同样 `:401` ⇒ 杀伤**不是 magnitude 专属**
  （magnitude 另有两条专属测试，可能造成「其实是被别的测试杀的」错觉，故补此对照）。
- **C13**（只调换两道闸的顺序，一道不删）⇒ 同样 `:401` ⇒ 顺序也被钉住。

**自指尺子排查（`grep` 全部测试文件）：**

| 位置 | 形态 | 判定 |
|---|---|---|
| `validate_test.go:121` | `t.Fatalf(..., len(rep.Checks))` | 只是错误消息，不是期望值 —— **无害** |
| `validate_test.go:440` | `make([]string, 0, len(gates))` | 容量提示 —— **无害** |
| `validate_test.go:257` | 注释「不复用 `yoyFields()`」，测试确实自己重算最大同比 | **主动避开，正确** |
| `fields_test.go:155` | `assert.Equal(len(fieldOrder), len(allFields))` | **不在本任务 writes 内**，且两侧是不同对象，未评判 |
| `profiles_test.go:72` | `assert.Equal(len(fieldOrder), len(covered))` | 同上 |

⇒ **本任务范围内无残留自指尺子。**

### 4.5 事项⑤ —— M9「无效实验」的判定：**成立**

**我的独立结论：成立。** 理由比 dev 给的更收紧一点：

变异测试的契约是「**被试 = 生产代码，判据 = 测试套件**」。变异判据本身是在问另一个问题
（「判据自己有没有被保护」），而对任何**自足**的套件，这个问题的答案先验就是「没有」——
测试不会因为自己被放松而失败。若把它计入存活，那么**每一条断言都是一个存活变异**，
存活率这个指标随即失去意义。所以不计入是对的。

**但 dev 措辞里的「不可能由同一套测试自证」偏强。** 本仓库**存在元守卫先例**：
`fields_test.go:185 TestFieldNamesAppearOnlyInFieldsGo` 用 `parser.ParseFile` 读源码 AST，
`amount_test.go:331 TestNoLiteralDirectionWordAtProductionCallSites` 用 `os.ReadFile` 读源码文本。
⇒ 技术上**可以**写一条「断言形态守卫」。准确的说法是
**「可以做，但刻意没做，且这一缺席是有文档记录、被接受的性质」**，而不是「不可能」。
**操作性判定（无效实验、不计入存活）不受影响。**

---

## 5. RED 因果的独立复现

不采信 discovery 原文，自己删掉 `Validate` 函数体后跑整包：

```
internal/hestia/validate_test.go:54:16: undefined: Validate
internal/hestia/validate_test.go:103:16: undefined: Validate
... （共 10 处调用点，形态一致）
FAIL	github.com/newthinker/atlas/internal/hestia [build failed]
```

与计划预期 `undefined: Validate` 一致，**无 `imported and not used` 干扰**。

---

## 6. 五项发现

### 发现 1（中）：C10 存活 —— 「遍历走 `fieldOrder` 而非 map」零守卫

把 `gateMagnitudeSanity` 的 `for _, f := range fieldOrder { r, ok := ...MagnitudeRanges[f] ... }`
换成 `for f, r := range in.cfg.MagnitudeRanges`，**整包全绿**（`ok`，编译闸已通过）。

成因：唯一填表的测试 `TestMagnitudeSanityActivatesWhenCalibrated` 只放**一条**区间，
map 只有一个元素时迭代顺序无从暴露。

**实现本身做对了**（走 fieldOrder），DoD 的实现约束满足，故不判失败。
但这条 DoD 明写的性质**没有任何东西守着**。

**已实证可补救 —— 是「没测」不是「测不了」**：我在一次性副本里写了一条守卫
（两条区间 + 两个字段同时越界，断言必然报 `fieldOrder` 里靠前的 `m2`，跑 20 轮）：

- 正确实现下：**PASS**
- C10 变异下：**FAIL** —— `"fx_reserve=3.36 outside [0,1] 万亿美元" does not contain "m2"`

（该守卫是我的验证探针，跑完即删，未进交付物。）

### 发现 2（中）：C11' 存活 —— 「配置校验在跑任何闸门之前」零守卫

把 `cfg.validate()` 从入口移到闸门循环**之后**（仍返回同样的 error），**整包全绿**。
现有 `TestValidateRejectsInvalidConfig` 只断言「返回 error 且含 `unknown`」，
不区分它发生在闸门之前还是之后。

实现做对了；该半句无守卫。可补：临时往 `gates` 表 append 一个记录「我被调用过」的哨兵闸门
（本包已有操作包级变量的先例，见 `required_test.go` 的哨兵分项），断言配置非法时它未被调用。

### 发现 3（低）：DoD 点名的 `TestEverySkippedCheckHasReason` 近乎平凡为真

探针实测两份 golden 上每道闸的实际 Status：

```
golden2025  monetary_hierarchy   passed    corp_loan_reconcile passed
            yoy_sanity           passed    completeness        passed
            magnitude_sanity     skipped   reason="not_calibrated"
golden2020  （同上，唯一 skip 仍是 magnitude_sanity）
```

⇒ 该测试在两份数据上**只遍历到一个 skip**，且那个 Reason 是硬编码常量字符串；
`absent_field` 这条真正会漏 Reason 的路径**它根本没走到**。
C8（把 `need` 的 skip 去掉 Reason）实际是被 **DoD 追加的**
`TestValidateHandlesEmptyValuesWithoutSpecialCase:377` 杀死的。

**要求本身被守住了，只是守在别处。** 记此以免下游以为那条测试在守 absent 路径。

### 发现 4（低）：`TestGatesSkipOnAbsentFields` 名不副实

同一探针显示：golden2020（v1，缺 27 个字段）上四道非 magnitude 闸门**全部 passed**，
没有发生任何 `absent_field` 跳过 —— 因为 v1 缺的都是 `tsf_` 字段，而这四道闸都不需要它们。
该测试实际只断言了 `rep.Passed == true` 与 `completeness == passed`。
「缺字段的闸门记 skipped」由 C9 证明确实守在空 `Values` 那条测试里。

### 发现 5（低）：dev 的三处「理由」偏强 —— 同一形状本 Sprint 第三次

| # | 声称 | 实测 |
|---|---|---|
| a | 「边界值必须选 2 的幂，否则测的是浮点舍入」 | 换成 100000/5000、tol=0.05 后**照样 PASS、照样杀死 `<`**（§4.2） |
| b | M9「不可能由同一套测试自证」 | 本仓库存在两处元守卫先例；准确说法是「可以做但刻意没做」（§4.5） |
| c | （TASK-003 已记）`NotErrorIs(Unwrap(err), err)` | **本任务已改用正确写法** `require.NotNil(t, errors.Unwrap(err))`（`validate_test.go:196`），C7 实测由它致红 —— **上轮发现已被采纳** |

**三条的结论都正确**，被高估的是理由。之所以值得记：理由是下游复现时的唯一入口，
而结论正确会让理由永远不被复查。（c 已闭合，说明这个反馈回路是通的。）

---

## 7. 未据以判不通过的项

- validator 的 3 条 `scope-writes-outside-packages`（AD-035-4 已知形状级假阳），
  validator 整体 `exit=0`、`✓ 任务图校验通过（7 个任务）`。
- IDE 诊断的 `undefined: Validate` ×10 —— 实跑证伪，属 RED 阶段滞留快照。

## 8. 主工作区完整性

```
$ shasum -a 256 internal/hestia/validate.go internal/hestia/validate_test.go internal/hestia/store_test.go
63f89b59d6d85e252f13a25c56268311df7bd3fb725ee67ba6fd870920ae47e0  validate.go
a7206129b2b6c66acfa12878306a29b9e6ffad2bc78642d2a916dd61b7b68ea6  validate_test.go
1469b2141671301f52695c87016b1d7861d24044bfbd61660bff2825dcaa9580  store_test.go
$ git status --porcelain  → 仅 .arcforge/ 条目
```

三文件 sha256 与开工时逐字节一致，`internal/` 一个字节未动。全部变异只在 `/tmp/mut-004` 内进行。

---

## 结论：**VERIFIED**

8 条 done_criteria 全部有对应测试、全部有消融证明断言在守卫、全部核对了致红归因。
Leader 点名的五件事逐条回复：①②③④⑤ 的**操作性结论全部成立**，其中②⑤的**理由**被我实测收窄。
两个存活变异（C10 / C11'）对应 DoD 中两条**实现约束型子句**——实现均正确，但无回归防线；
两者都已被我证明**可补救**，建议交由 T5/T6 或收尾任务处理。
