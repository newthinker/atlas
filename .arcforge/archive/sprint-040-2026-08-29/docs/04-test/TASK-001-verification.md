# TASK-001 验证报告 —— 累计前缀：periodAlt 与 cumulativePeriods 两处硬卡点

> **本文件是第 2 轮（返工后复验）报告，覆盖第 1 轮内容。**
> 第 1 轮判定 REJECTED（`task_defect`，`rework_count` 0→1），阻断项已修复。

- **验证者**：test-m1c3a-v1
- **第 1 轮**：REJECTED @ epoch 1 —— `profiles_test.go` 一行注释里的裸 `TASK-007`
- **第 2 轮**：✅ **VERIFIED** @ **epoch 2**，`rework_count` = 1
- **判定对象**：`verify_baseline.head` = `20c05ea46af33ac95ed3a0f7d952cd0feb036dfe`
  = 我采样时 HEAD，`discovery_sha256` = `01fc57d06e…` 亦与基线一致 ⇒ **零漂移**

## 1. 第 1 轮阻断项 —— 已修复

| | |
|---|---|
| 位置 | `internal/hestia/profiles_test.go`（第 1 轮时的 :708） |
| 违反 | `non_functional[0]` 的 Global Constraint「注释里引用任务编号带 milestone 前缀」 |
| 修复 | `f337f88bd140fc42ab8d0f201448604a70955cc1`：`TASK-007` → `M1c-3a 的 TASK-007` |

**我的复查**（口径：`4a12794a…` → `20c05ea…` 限定到本任务 `writes` 两文件的**全部新增行**）：

```
带前缀 8 处，裸编号 0 处   ⇒ 阻断项已修复
```

⚠️ dev 在 discovery 里指出我第 1 轮的计数口径需要说清：**6 处是两个改动文件新增块的合计**
（`profiles.go` 2 处 + `profiles_test.go` 4 处），不是单文件。我复核确认 dev 的说明正确，
第 1 轮的结论（5 带前缀 / 1 裸）不变。第 2 轮因返工新增了注释，总数变为 8 处。

⚠️ **dev 顺带发现、未修、需 Leader 裁断**：同两个文件里另有 **7 处既有裸任务号**
（`TASK-004`×5、`TASK-005`×1、`TASK-006`×1），全部来自更早 sprint 的注释、不在本次新增行内。
按 surgical 原则未动是对的，但**这说明该约束在历史注释上普遍未被执行**——
是否统一整改请 Leader 判断（我倾向单开一个清理任务，不要塞进本 wave）。

## 2. 第 1 轮的非阻断发现 V6 —— dev 采纳并收口，我已复验 KILLED

**第 1 轮**：绕开 `cumulativeMonthAlt`、直接往 `periodAlt` 插一条累计分支
（`…前三季度|前十二个月|` + `cumulativeMonthAlt` + `|[0-9]{1,2}月份`），
本任务 7 个新测试**无一转红**（SURVIVED，且**非等价变异**——它精确制造了「正则认得、
口径表不认 ⇒ 静默筛掉、整篇命中 0」这个本任务全篇要防的失效）。

**修法**（采纳我给的方案，用本包既有惯用法）：

```go
- assert.Containsf(t, periodAlt, cumulativeMonthAlt, …)
+ assert.Equal(t, `全年|上半年|一季度|前三季度|`+cumulativeMonthAlt+`|[0-9]{1,2}月份`, periodAlt, …)
```

**我在新锚 `20c05ea…` 上重跑 V6**：

```
V6 套件 exit=1
   --- FAIL: TestCumulativeMonthAltEnumeratesTenMonths
       Error: Not equal
```

⇒ **KILLED，且只有那一条断言红** —— 因果干净，不是副作用连坐。

### 这一对断言是互补的，不是同源自证

我特意核了一遍「守卫的基准会不会与被守护属性同源」：

| 断言 | 钉的是 | 被哪个变异抓到 |
|---|---|---|
| `assert.Equal(want, strings.Split(cumulativeMonthAlt,"|"))`（**字面 want 列表**） | 常量的**内容** | M2（删词）、V1（协调漂移）、V4（改序） |
| `assert.Equal(四个既有+const+数字月份, periodAlt)`（**引用常量**） | periodAlt 的**组成结构** | V6（绕开常量插分支） |

第二条两边都引用 `cumulativeMonthAlt`，单看确有同源之嫌——**但常量的内容由第一条的
硬编码 want 列表独立钉住**，两条合起来不留缝。dev 还在 `…Agree` 上方加了交叉引用注释
说明二者互补、别当重复删掉。

## 3. done_criteria 覆盖矩阵（8/8 PASS）

| # | 完成标准（摘要） | 对应测试 | 判定 |
|---|---|---|---|
| functional[0] | `cumulativeMonthAlt` 穷举十项；`periodAlt` 接上它 | `TestCumulativeMonthAltEnumeratesTenMonths` | ✅ |
| functional[1] | `cumulativePeriods` 增 11 键（10 + `1月份`） | `TestCumulativePeriodsKeySet`（精确键集 = 15） | ✅ |
| functional[2] | 机械守住一一对应 | `TestCumulativeMonthAltAndCumulativePeriodsAgree`（双向） | ✅ **缺口 V6 已收口** |
| boundary[0] | 当月句前缀仍被排除 | `TestSingleMonthPrefixesAreMatchedButNotCumulative`（1..12） | ✅ |
| boundary[1] | 「11月份」不退化成「1月份」 | `TestFlowRECapturesMonthlyPeriodVerbatim` | ✅ |
| boundary[2] | 既有四键保留、既有用例全绿不变 | 键集测试 + 全量套件 | ✅ |
| error_handling[0] | 两次单删各自转红且**失败信息不同** | M1 / M2 消融 | ✅（见 §4） |
| non_functional[0] | gofmt/vet/全绿/覆盖率/无新依赖/**milestone 前缀** | — | ✅ **全部满足** |

## 4. 消融复跑（锚 `20c05ea…`，7 个全部 KILLED）

| ID | 变异 | 结果 | 区分性断言 |
|---|---|---|---|
| **M1**（DoD 指定） | `cumulativePeriods["前八个月"]` → `false` | KILLED | `Agree/正向：正则认得的必须在口径表里`（`Should be true`）+ `KeySet` |
| **M2**（DoD 指定） | `cumulativeMonthAlt` 去掉「前八个月」 | KILLED | `Agree/反向：口径表里的必须被正则整段认出`（`NotNil`）+ `EnumeratesTenMonths` |
| V1 | 协调漂移（两侧同删「前六个月」） | KILLED | 仅两条穷举断言；`Agree` 双向保持绿 |
| V2 | 口径表加正则认不出的键 | KILLED | `KeySet` + `Agree/反向` |
| V3 | `[0-9]{1,2}` → `[0-9]{2}` | KILLED | `SingleMonth/1..9月份` 等 |
| V4 | 互换「前十一个月」/「前十个月」 | KILLED（**零信息量**，等价变异） | 仅穷举断言的顺序 |
| **V6** | 绕开常量往 `periodAlt` 插分支 | **KILLED（本轮新增）** | 仅 `EnumeratesTenMonths` 的**等值**断言 |

**`error_handling[0]` 复核**：M1 与 M2 的区分性断言仍分别落在 `Agree/正向` 与 `Agree/反向`，
**两次失败信息不同**，且共同转红的两个子测试里断言本身也不同
（M1 `Not equal` 口径判定 vs M2 `NotNil` 正则匹不上）⇒ 仍然满足。

**隔离**：`git archive <全 sha>` → `mktemp -d`；对照组先 exit=0；四道闸；
主工作区两文件 sha256 + `git status --porcelain` 指纹在开始、7 个窗口内、收尾共 **9 次**校验，
**逐次相同，全程一个字节未碰**。harness 锚点经 `ABLATE_ANCHOR` 覆写为 `20c05ea…`（全 sha）。

## 5. 返工后的回归数字（我自采，锚 `20c05ea…`）

| 指标 | 值 |
|---|---|
| 套件（两轮） | exit=0 / exit=0 |
| `gofmt -l internal/hestia/` | 0 行 |
| `go vet ./internal/hestia/` | 0 行，exit=0 |
| RUN | **1013 = 504 顶层 + 497(第1层) + 12(第2层)** |
| 与返工前 `cea6b3c` 比 | **新增 0 条、消失 0 条**（本次是断言内改动，不产生新 RUN 项） |
| 覆盖率（两轮取值全同） | **1678/1756 = 95.5581%** ≥ 95.5% ✅ |

分母属整棵树（现含 TASK-002 / TASK-003），故与第 1 轮的 `1618/1694` 不同——**不是回归**。

## 6. 🔴 对第 1 轮报告的方法学更正（我自己的错）

第 1 轮报告里我给的 RUN **层级分解**用的是「按名字中 `/` 个数做直方图」。
**该方法在本包上是错的**——名字自带斜杠（`"N/A"`、`137_条_/_12_页`、`<a>/</a>`）
会被误记成更深层级。改用「祖先是否存在于 RUN 名集合」重算：

| 锚 | 第 1 轮写的 ❌ | 正确值 |
|---|---|---|
| `4a12794a…` | 483 + 420 + 37 + 4 = 944 | **483 + 449(第1层) + 12(第2层) = 944** |
| `4f469e96…` | 489 + 445 + 37 + 4 = 975 | **489 + 474(第1层) + 12(第2层) = 975** |

**载重数字全部不受影响**：总数 944 / 975、顶层 483 / 489、子测试合计 461 / 486、
`+31 = 6 顶层 + 25 子测试`、`0 条消失` —— 第 1 轮的结论一个字不变。

⚠️ **dev 的 discovery 里 `verification.suite` 仍写着 `420 / 37 / 4`**（同一错误，第 1 轮我还
「确认逐字相符」）。我**没有改它**——`verifying` 期间改交付物/自陈就是判定对象漂移。
建议 Leader 在 `accepted` 时更正，或记进 CONTRACTS。

⚠️ 教训：**求和自洽校验挡不住这类错**。`483+420+37+4` 与 `483+449+12` 都等于 944，
因为错的是同一总数在各档之间的**再分配**——自洽校验只发现漏项，不发现错分。
（这个错是我在验 TASK-002 时、因与 dev-b 的报数不符去追查才发现的；
我当时差点反过来判 dev-b 错。）

## 7. 结论

✅ **VERIFIED**（第 2 轮，epoch 2，`rework_count` = 1）。

- 第 1 轮的唯一阻断项已修复，我按同一口径复查：**裸编号 0 处**。
- 第 1 轮的非阻断缺口 V6 被 dev 主动采纳并收口，我在新锚上复验：**KILLED 且因果干净**。
- 7 个消融全部 KILLED，`error_handling[0]` 的「失败信息不同」复核仍成立。
- 回归零变化（RUN 名 0 增 0 减），覆盖率 95.5581% ≥ 95.5%，gofmt / vet 空。

**留给 Leader 的两件事**（均非阻断）：
1. 同两文件里 **7 处既有裸任务号**（更早 sprint 的注释）是否统一整改。
2. dev discovery 的 `verification.suite` 层级分解需在 `accepted` 时更正（见 §6）。
