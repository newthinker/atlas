# TASK-010 验证报告 —— 季报识别②：parse.go 的 titleRE / parseTitle / checkPeriodTypeSupported

- **验证者**：test-agent-26 ｜ **被验交付**：dev-agent-52，提交 `062b460`
- **验证基线**：`verify_baseline.head = 0e2c6fc976d90382ecb2122dbeb1ed18eaf8c9c9` = 承接时 HEAD ⇒ **无漂移**
- **assignment_epoch**：1 ｜ **结论**：**VERIFIED**（6/6 条 done_criteria 全部 PASS）

---

## 0. 承接核实

| 核项 | 结果 |
|---|---|
| 验证对象漂移 | `verify_baseline.head` == 当前 HEAD ✅ |
| **DoD 未被改写** | `transitions.jsonl` 中本任务的 `update` 只动过 `questions`（两次，leader 答复澄清），**`done_criteria` 从未被 update** ✅ |
| 实际改动 vs `writes` | `parse.go` + `parse_test.go`，与声明**逐字一致**，无越界 ✅ |
| discovery | 文件存在（16934 B）；**任务文件的 `discovery` 字段原本缺失，由我补上** |

---

## 1. 完成标准覆盖矩阵（6 条）

| # | done_criteria | 对应测试/证据 | 判定 |
|---|---|---|---|
| functional[0] | `titleRE`/`parseTitle` 认得两种真实季报标题；期次值与 periodType **逐字采用 TASK-001 那套**；用真实正文样本跑 `Parse` 的标题段 | `titleRE` 加 `一季度\|前三季度`；`parseTitle` 返回 `q1→YYYY-03`、`q1_q3→YYYY-09`，**与 TASK-001 的 `periodEndMonth` 逐字一致**；`TestParseTitle` 两条 + `TestParseRejectsQuarterlyUntilExtractorWired` 用 `testdata/pboc-2026-03-q1.html`、`pboc-2025-09-q3.html` 两份**真实正文样本** | **PASS** |
| functional[1] | 既有三种形态（h1/annual/monthly）必须仍绿 | 消融 A1 实测：删季度分支后**既有用例 0 条受影响**（308→306+2，外溢 0）⇒ 「`\A…\z` 全锚定、放宽期次段是唯一改动点」得到实证 | **PASS** |
| boundary[0] | 把新季度类型加进 `checkPeriodTypeSupported` 的**拒绝列表**，理由写进错误信息，由 TASK-004 移除 | `case "q1", "q1_q3":` 返回带理由的错误，**错误串内写明「由 M1b-4b 的 TASK-004 接上后解除本分支」**；`TestParseRejectsQuarterlyUntilExtractorWired` 断言错误含 periodType、原标题、`TASK-004`，且 `assert.Empty(obs.Values)` 不得返回半份结果 | **PASS**（另见 §3 的澄清） |
| error_handling[0] | `parseTitle` 失败错误信息**含原标题**；**若**包了底层错误须用 `%w` + `require.NotNil` 自证，**不要用 `assert.ErrorIs`** | 两条错误路径均 `%q title` 含原标题；实现**不包任何底层错误**（唯一候选 `strconv.Atoi` 的 `convErr` 因正则 `[0-9]{1,2}月` 约束恒为 nil）⇒ **「若」条件不成立，`%w` 部分不适用**；`parse_test.go` 中 `assert.ErrorIs` 出现 **0 次**、`NotErrorIs` **0 次** | **PASS** |
| non_functional[0] | **消融自证**：删 `titleRE` 季度分支，确认新增两条断言转红且**红的是它们**；贴出失败输出的具体那一行 | A1 我已独立复现，红在 `parse_test.go:58` 与 `:352`，见 §2 | **PASS** |
| non_functional[1] | `gofmt`/`vet` 空、整包 `-count=1` 与 `-race` 全绿、覆盖率 ≥93.2% | 见 §2 | **PASS** |

---

## 2. 实跑与消融（隔离 worktree @ `0e2c6fc`）

```
gofmt → 空    vet → 空    build ./... → OK
go test -count=1 -cover → ok  coverage: 93.5%   (门槛 93.2% ✅)
go test -count=1 -race  → ok
顶层 PASS 308 / 全部 PASS 659 / FAIL 0
go tool cover -func → parseTitle 100.0%、checkPeriodTypeSupported 100.0%
```

**三条消融我全部独立重跑，逐条与 dev-52 声称吻合，外溢度全部为 0：**

| # | 变异 | dev 声称 | **我实测** | 外溢 |
|---|---|---|---|---|
| A1 | `titleRE` 删季度分支 + `parseTitle` 删两条 `case` | 恰好 4 条转红全是本任务新增，既有 0 条 | **4 条子测试**：`TestParseTitle/{2026年一季度, 2025年前三季度}` + `TestParseRejectsQuarterlyUntilExtractorWired/{两份样本}`，红在 `parse_test.go:58`、`:352` ✅ | 306+2=308 ✅ |
| A2 | `checkPeriodTypeSupported` 改回 `if periodType != "monthly" { return nil }` 原样 | `EveryPeriodType/{q1,q1_q3}` + `RejectsQuarterly` 转红，红在 `:363` | **完全一致**，红在 `:363/:364/:365/:411`（同测试多条断言） ✅ | 306+2=308 ✅ |
| A3 | `validPeriodTypes` 加第六种取值 `q1_q2`（并给期末月 06 以避开一致性网） | 红在 `parse_test.go:398` `require.Len(decisions, len(validPeriodTypes))` | **只有 `TestEveryPeriodTypeHasAnExplicitSupportDecision` 红，行号 `:398`** ✅ | 307+1=308 ✅ |

**A3 是 dev-52 自补的，价值最高**：A2 只证明「q1/q1_q3 *现在*被拦住了」，证不了「**下一个**新增的类型也会被拦」。A3 通过引入一个从未存在过的第六种取值，把后者变成了可观察事实。这正是「单点承载不了因果命题，要控制变量的一对」的实例。

---

## 3. 🔴 一处措辞需要澄清（不是缺陷，但会误导归档后的读者）

流转中出现过「dev-52 把 `checkPeriodTypeSupported` 改成了**穷举 switch（默认拒绝）**」的说法。**实现不是这样，我逐行核过：**

```go
func checkPeriodTypeSupported(periodType, title string) error {
	switch periodType {
	case "monthly":   return fmt.Errorf(...)
	case "q1", "q1_q3": return fmt.Errorf(...)
	}
	return nil        // ← 没有 default 分支；末尾仍是「默认放行」
}
```

`grep -c "default:"` = **0**。`h1` 与 `annual` 正是靠这个 `return nil` 放行的（它们是已支持形态），所以这个设计本身没问题。

**真正的保障来自一条测试，而不是生产代码**：`TestEveryPeriodTypeHasAnExplicitSupportDecision` 用
`require.Len(decisions, len(validPeriodTypes))` 强制「白名单里每个取值都必须在表里显式表态」。
新增第六种类型时——**运行时仍会放行，但测试会红**，逼人写下决定与理由。

⚠️ **dev-52 自己的代码注释是准确的**（原文：「而**本条会红**并逼人**明确表态**」），它明说了保障来自这条测试。
偏差出在转述环节把它压缩成了「默认拒绝」。

**为什么要写进报告**：「默认拒绝」暗示**运行时**安全，实际是**测试时**强制表态。两者在下述场景分叉——
若有人改了 `parseTitle` 产出新 periodType **而不动 `validPeriodTypes`**，那条测试不会红（`len` 没变），
`checkPeriodTypeSupported` 会放行它。此时仍有第二道网（`Meta.validate` 的白名单在 `Save` 处拒），
但报错位置远离原因。**这是边缘场景、风险低，不构成缺陷**，登记是为了让归档后的读者知道保障的确切边界。

---

## 4. 我认可 dev-52 的两处判断

**① `%w` 判为「不适用」是对的。** DoD 的措辞是「**若**包了底层错误须用 `%w`」——条件句。实现里唯一的底层错误候选是
`strconv.Atoi`，而 `qualifier` 由正则 `[0-9]{1,2}月` 捕获，去掉「月」后必为 1–2 位数字 ⇒ `convErr` **恒为 nil**，
`n < 1 || n > 12` 那条路径可达但不携带底层错误。**凭空包一个只会让 `require.NotNil(Unwrap)` 变成恒真断言**——
那正是本 Sprint 反复在防的东西。它选择不包并写明理由，是正确的。

**② 「不按 DoD 字面做」这一处也是对的。** DoD 说「把新季度类型加进拒绝列表」，它指出原实现
`if periodType != "monthly" { return nil }` 是**一条默认放行的规则**，只加列表能过 DoD 但把洞留着。
它加的那条表驱动测试把「每个合法 period_type 必须显式表态」变成了绊线。
**这是超出 DoD 而非偏离 DoD**，DoD 的字面要求（拒绝列表 + 理由 + 由 TASK-004 移除）逐条都满足。

---

## 5. DoD 之外的观察

**O1（给 TASK-004 的）** 解除季度拒绝时要动的是 `checkPeriodTypeSupported` 的 `case "q1", "q1_q3"` **和**
`TestEveryPeriodTypeHasAnExplicitSupportDecision` 的 `decisions` 表（把两者的 `supported` 改成 `true` 并换掉 `why`）。
**只改前者会让那条测试红**，而红的理由是「表里说该拒、实际放行了」——一眼看不出是预期变更。

**O2** `parse.go` 的 `titleRE` 与 `discover.go` 的 `reportTitleRE` 是**两份并行解析**，季度段宽窄**有意不同**
（discover 侧写宽 `[一二三四五六七八九十]季度` 再由语义层只放行「一季度」；parse 侧直接窄写 `一季度|前三季度`）。
代码注释已写明理由。**动其中一处时另一处不会红** —— 两者的一致性目前靠注释，没有绊线。当前无害（期末月约定由
`periodEndMonth` 统一），登记备查。

---

## 6. 复现命令（锚已钉全 sha）

```bash
git worktree add --detach ../wt-v-w2 0e2c6fc976d90382ecb2122dbeb1ed18eaf8c9c9
GOTOOLCHAIN=local go test ./internal/hestia/ -count=1 -cover
GOTOOLCHAIN=local go test ./internal/hestia/ -count=1 -run '^TestEveryPeriodTypeHasAnExplicitSupportDecision$' -v
git diff --numstat 062b460^ 062b460 -- internal/hestia/
```
