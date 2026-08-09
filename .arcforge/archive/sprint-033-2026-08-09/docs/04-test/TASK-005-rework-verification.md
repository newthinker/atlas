# TASK-005 返工复验报告（第 1 轮，QA 的 C2）

- **验证者**：test-agent-21 ｜ **epoch**：1 ｜ **rework_count**：1 ｜ **日期**：2026-08-09
- **验证对象（树状态）**：`823ca15fbea53c626e986419d8f91ad200f03859`（= `verify_baseline.head`，亦为当时 HEAD）
- **本任务自身的 delta**：`572f2cebb6a2cb9121f62c24c9ca6498afe05962`（C2）
- **被取代的初版**：`39aa8af0bff0d9c46b8a1a77e633aa1a1eb0c7fb`（原 12 条 DoD 已在 `TASK-005-verification.md` 逐条验过）
- **隔离环境**：`git worktree add --detach ../wt-verify-TASK-005b 823ca15f…`，全程 `GOTOOLCHAIN=local`
- **判定**：**VERIFIED —— fix_items C2 的全部要求满足；两条观察级发现登记在 §5，均不构成阻断**

## 0. 两处需要先澄清的口径

**① 「验证对象是 572f2ce 还是 823ca15」不是矛盾。** 派单消息写 `572f2ce`、任务文件
`verify_baseline.head` 写 `823ca15`——两者指的是不同的东西：baseline 是**派发时刻的树状态**，
`572f2ce` 是**本任务的 delta**。`823ca15` 只是在其上叠了 TASK-006 的 C3/C4（已由 test-agent-20
于 `00:45:01Z` 验过）。我按文件跑树状态 `823ca15`，按 `fix_items` 判归属，两者不冲突。
（我在接手回信里把这一条说成了「分歧」，那个说法过强，此处更正。）

**② fix_items 归属已逐个核过**：C1 → TASK-004，**C2 → TASK-005（本任务，唯一一条）**，
C3/C4 → TASK-006。C1/C3/C4 不在本次判定范围内。

## 1. 漂移核验

| 项 | baseline 记录 | 实测 | |
|---|---|---|---|
| head | `823ca15f…` | `git rev-parse HEAD` 相同 | ✓ |
| discovery sha256 | `4904898452a4…` | `shasum -a 256` 相同 | ✓ |

**无漂移。** 下文全部结论出自我在隔离 worktree 里自己跑的命令。

## 2. 混合提交的计数拆解 —— 我自己重算，未采信 dev 的 64/84

dev 在 discovery 的 `contamination_note` 里申报了 `572f2ce` 含 TASK-004 的 C1。
计数污染必须由我按 commit 边界自己拆，逐 commit 实测：

| commit | 顶层 PASS | 含子测试 | FAIL | 覆盖率 |
|---|---|---|---|---|
| `ed776a0`（干净基线，T7 验过） | 61 | 75 | 0 | 88.7% |
| `572f2ce`（**混合**：C2 + C1） | 64 | 84 | 0 | 89.3% |
| `823ca15`（+ TASK-006 的 C3/C4） | **66** | **86** | 0 | 89.3% |

按测试名做集合差（`comm`），而不是只看数字：

- `572f2ce` 新增 3 条顶层：`TestNewStoreRejectsDriftedCurrentView`（**C1，属 TASK-004**）、
  `TestSaveRejectsUnknownCheckStatus`、`TestSaveRejectsSkippedWithoutReason`（**C2**）
- `TestSaveRejectsUnknownCheckStatus` 带 **6 条子测试**
- ⇒ **C2 净增 = +2 顶层 / +8 含子测试**，与 dev 申报的数字**逐字吻合**
- `823ca15` 新增 2 条顶层 = `TestSaveDuplicateOnlyTouchesTheMatchingRevision`、
  `TestSavePendingColumnsMatchTheirValues`（C3/C4，属 TASK-006）

**`comm -23` 全程为空 ⇒ 无任何测试被删除。** 这条比计数更重要：返工最容易的退化是
「改实现顺手删掉挡路的旧测试」，那在「总数变多了」的表象下完全隐形。

**变异基线取 66 顶层 PASS。**

## 3. fix_items C2 的逐条判定

| C2 的要求（原文摘要） | 实证手段 | 判定 |
|---|---|---|
| 改为白名单：`Passed==true` 时每个 `Status` ∈ {`CheckPassed`, `CheckSkipped`} | M1 / M2 / M6 / M9 全部 KILLED | **PASS** |
| 未知状态一律拒绝（六种错拼） | M1 精确红掉那 6 条子测试 | **PASS** |
| 在错误里回显原串 | M3 红 4/6 —— 两条重言式，见 §5 O-1 | **PASS**（有观察） |
| `verify`：六种错拼全部被拒**且零行落盘** | M7 红 11 条，全部是「报错须零行」用例 | **PASS** |
| 顺带项：让 `types.go:116`「Skipped 必填 Reason」注释成立 | M5 KILLED | **PASS** |

原 12 条 DoD 在本轮的处置：实现改动只落在 `checkReportConsistency`（C2）与 `NewStore`+`verifyCurrentView`（C1，非本任务），
**无测试被删**，整包 66/86 全绿 ⇒ 原判定继续成立，不重复取证。

## 4. 变异实证（9 条有效：8 KILLED / 1 SURVIVED）

harness `scratchpad/mut5b.sh`，四条自证：① diff 非空 ② 首行仍是 `package hestia`
③ `go vet` **红绿都查** ④ 存活时顶层 PASS 必须 == 基线 66。

| # | 变异 | 结果 | 红的测试 |
|---|---|---|---|
| M1 | `default` 分支变空操作（**即返工前的黑名单行为**） | KILLED | `TestSaveRejectsUnknownCheckStatus` 的 **6 条子测试全红** |
| M2 | 零值 `""` 混进白名单 | KILLED | **只红** `status=` 一条 |
| M3 | 移除原串回显（只报 `c.ID`） | KILLED | 红 4：`FAILED` / `PASSED` / `pending` / `passed ` |
| **M4'** | `unknown` 与 `failed` 的报错顺序对调 | **SURVIVED** | §5 O-2 |
| M5 | 取消「Skipped 必填 Reason」 | KILLED | `TestSaveRejectsSkippedWithoutReason` |
| M6 | `CheckSkipped` 落入 `default`（过度拦截） | KILLED | 3 条 |
| M7 | `checkReportConsistency` 挪到 `insert` **之后**（先写再报错） | KILLED | 11 条 |
| M8 | 去掉 `if !rep.Passed { return nil }` 早退 | KILLED | 9 条 |
| M9 | 白名单漏掉 `CheckPassed` | KILLED | 18 条 |

### KILLED 的因果核查（不只看条数）

按我在 T3 定下的判据——**连带伤害的标志是「红的测试与变异点无因果关系」，不是「红的条数多」**：

- **M1（6 红）**：红的恰好是六种错拼的六条子测试，一一对应。
- **M6（3 红）**：`TestSaveRejectsUnknownCheckStatus` 的对照组、`TestSaveRejectsSkippedWithoutReason`
  的第二半、`TestSaveRejectsSelfContradictoryReport` 的 `s2` ——**三处都直接构造 `CheckSkipped`**。
- **M7（11 红）**：全部是「错误路径必须零行」的用例（含 `assertNoRowsAnywhere` 的三组）。
- **M8（9 红）**：全部是 `Passed=false` → pending 的用例，直接依赖那条早退。
- **M9（18 红）**：条数最多，但**因果单一**——`passing()` 返回 `Status: CheckPassed`，
  把它移出白名单等于让每一个用 `passing()` 的测试都拒绝入库。**18 条红全部同因，不是连带伤害。**

### harness 自证与一次「先换写法再判无效」

- **SELF-A**：故意造未使用局部变量 → 编译失败 → harness 判 **INVALID**，未记 KILLED。
- **SELF-B**：故意把首行改坏 → harness 判 **INVALID**。
- **M4 第一次写法**误命中 `checkValues` 导致 `undefined: failed`。按 test-agent-20 在 T4 的 N2 更正
  （**「变异编译不过」常是变异写法的产物，不是判据的性质**），我换了精确锚重做为 M4'，
  **没有记成 INVALID 就收工**——若当时收工，M4' 这条存活变异就会被漏掉。

## 5. 两条观察级发现（均不阻断）

### O-1：六条「回显原串」断言里有 **2 条是重言式**

M3（移除回显）只红了 4 条。用临时探针取出 M3 下的实际错误串：

```
hestia: validation report has unknown check status: deposit_sum (want one of "passed", "failed", "skipped")
```

| `bad` | `Contains(err, string(bad))` | 为什么 |
|---|---|---|
| `"FAILED"` | false → 红 ✓ | 大写不在串里 |
| `"fail"` | **true → 绿 ✗** | 被 want-list 里的字面量 `"failed"` 满足 |
| `""` | **true → 绿 ✗** | `Contains(s, "")` **恒真** |
| `"PASSED"` / `"pending"` / `"passed "` | false → 红 ✓ | |

⇒ 恰恰是**最难诊断的两个输入**（漏填零值、词形错拼），「回显原串」这条要求处于无保护状态。

**不判 FAIL 的理由**：这两条子测试的**主判据**（`require.Error` + `Contains(err,"deposit_sum")` +
`assertNoRowsAnywhere`）依然有效——M1 证明六条全都能挡住回归。失效的只是回显这一条子断言，
且实现对六种输入是同一行代码（`fmt.Sprintf("%s=%q", …)`），不存在真实缺陷。

**修法（已按上面那条错误串逐一核过，六条都会转红）**：把断言从裸原串换成整个键值片段——
`assert.Contains(err.Error(), fmt.Sprintf("%s=%q", "deposit_sum", string(bad)))`。

**这是本包第 3 次同形**：T2 的 `error_handling[0]`、T3 的 O-1（`period` 被 `period_type` 吞掉）、本条。
⇒ 建议 final-report 立一条包级规约：**`assert.Contains` 的针若来自被测输入，必须先确认它不会被
错误信息里的固定部分（want-list、字段名、模板文案）顺带满足**。三次都出在「固定文案里恰好含有针」这一点上。

### O-2：M4' 存活 —— 「未知先于 failed 报」是一条**有整段论证、零测试**的决定

dev 在 commit message 与代码注释里都写了「未知状态排在最前：状态无法解释时，『这份报告是否自洽』
本身就无从回答」。把两个 `if` 块对调后**整包 66/86 全绿**。

用探针证明它**不是等价变异**——混合报告 `[{typo_check,"fail"}, {real_fail,CheckFailed}]`：

| | 错误串 |
|---|---|
| 原实现 | `unknown check status: typo_check="fail" (want one of …)` |
| M4' 变异体 | `contradicts itself: Passed is true but check(s) real_fail failed` |

**两者都拒绝、都零行落盘** ⇒ 危害封顶在**诊断质量**：M1b-3 会被指向 `real_fail` 这个真失败，
而真正的根因是 `typo_check` 的拼写——正是 dev 那段论证要避免的情形。

**不判 FAIL 的理由**：该顺序不在 `fix_items` C2 的 `requirement` 或 `verify` 内，是 dev 自加的设计决定。
建议补一条混合报告用例（一个未知 + 一个失败，断言错误串指向未知），成本一条 `t.Run`。

## 6. 收尾回归（@ `823ca15f…`）

| 项 | 实测 |
|---|---|
| `go build ./...` | rc=0 |
| `go vet ./internal/hestia/...` | rc=0 |
| `gofmt -l internal/hestia/` | 空 |
| `go test ./internal/hestia/... -count=1` | **66 顶层 / 86 含子 / 0 FAIL / 0 SKIP**，覆盖率 89.3% |
| `go test ./...` | rc=0，**64 包全 ok、0 FAIL、过滤后残留 0 行** |
| `git status --porcelain` | 空（探针文件已删、全部变异已还原） |

## 7. 结论

`fix_items` C2 的全部要求均满足并有变异实证支撑；混合提交的计数已由我独立拆解、
与 dev 申报一致；返工过程**未删除任何既有测试**。两条观察级发现（O-1 断言重言式、
O-2 报错顺序无测试）都不产生假绿、也不对应任何未满足的判据，登记待 QA/后续子迭代处理。

**判定：VERIFIED。**
