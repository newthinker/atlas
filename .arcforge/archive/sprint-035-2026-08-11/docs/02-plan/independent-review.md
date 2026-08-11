# 独立 reviewer 反审结论 — Sprint 035

**方法**：spawn 一个只读上游计划 + spec + 既有代码、**禁读 `.arcforge/`**（因而没看过 Leader 的 DoD）的
独立 agent。它在一个由 `git worktree add … 125ad89` 建出的**隔离副本**里，按计划正文**逐字落盘**
`thresholds.go` / `required.go` / `validate.go` / `store.go` 增补 + 三份测试文件，实跑
`GOTOOLCHAIN=local go test ./internal/hestia/ -count=1`。

**Leader 复核**：`git status --porcelain` 确认主仓库 `internal/hestia/` **零改动**；
`git worktree list` 确认其临时 worktree 已清理（残留 4 个是历史 feature 分支，与本次无关）。
下列标注「Leader 已复验」的条目为 Leader 亲自跑过，其余采信 reviewer 的实测记录。

---

## A 类：必定发生、会让 dev 卡住（4 条）

| # | 问题 | 撞点 | 处置 |
|---|---|---|---|
| A1 | `thresholds.go` 的 import 块是**最终态**，Step 3 的代码体一个都没用到 ⇒ 4 个 `imported and not used` | T1 Step 4 | 写入 TASK-001 `non_functional[1]`：**每个 Step 只写该 Step 真正用到的 import** |
| A2 | **新增导出物打红两条导出面守卫** | T1 Step 9（首个「整包绿」） | 写入 TASK-001/003/004 各自的 `non_functional`；并**调整任务图**（见下） |
| A3 | `required.go` 的 `import "slices"` 同一个错（`slices.Clone` 到 Step 7 才用） | T2 Step 4 | 写入 TASK-002 `non_functional[2]` |
| A4 | `TestStockContinuitySkipReasons` 有 **2/4 子测试必红** | T6 Step 5 | 写入 TASK-006 `boundary[0]`（含正确写法二选一） |

### A2 是最贵的一条（Leader 已复验）

`store_test.go:368` 的 `TestPackageExposesNoWriteFunctions` 是**全导出面精确相等**断言：

```go
assert.Equal(t, []string{"NewStore", "Parse", "Store.Close", "Store.DB", "Store.Save"}, got, …)
```

计划新增 `DefaultThresholds`(T1) / `Store.Preceding`(T3) / `Validate`(T4) 三个导出物，
其中 `Store.Preceding` 还同时打红 reflect 版的 `TestStoreExposesNoWriteMethods`（`:349`）。

**为什么贵**：它在 **T1 Step 9** 就把「整包绿」打死，而红的是一条**与 hestia 校验毫无关系的架构守卫**——
dev 大概率会先怀疑自己的新代码。计划的「文件结构」表与 T1/T4 的 Files 列表里**都没有 `store_test.go`**，
给出的排查线索为零。

**处置不是放松断言**：该测试自己的注释（`store_test.go:346-348`）明写「新增任何导出方法都必须在这里
显式登记一次，让『又开了一个写口』成为一个需要动手改测试的决定」。⇒ 登记，并保持精确相等。

### A2 的连带后果：任务图必须改（reviewer 的 §2.12，Leader 采纳）

T1、T3、T4 **都要改 `store_test.go` 里同一处断言**。原计划把 T1/T2/T3 放在同一 wave 并行——
三个 dev 会在同一行上互相覆盖，且 T1 原本的 `writes` 里根本没有 `store_test.go`（会触发 scope 漂移 BLOCKED）。

**调整**（validator 复校通过）：

| 任务 | 原 wave | 新 wave | 依赖变化 | writes 变化 |
|---|---|---|---|---|
| TASK-001 | 1 | 1 | — | **+`store_test.go`** |
| TASK-002 | 1 | 1 | — | — |
| TASK-003 | 1 | **2** | **+TASK-001**（文件级串行化，非逻辑依赖） | — |
| TASK-004 | 2 | **3** | TASK-002,003 | **+`store_test.go`** |
| TASK-005/006/007 | 3/4/5 | **4/5/6** | — | — |

TASK-002 不新增任何导出物（`tsfSectionFields`/`requiredFields` 均非导出），故仍可与 TASK-001 并行。
**并行度从 3 降到 2**，这是共享守卫文件的真实代价。

---

## B 类：必定发生、会产生假绿（4 条，更危险）

判据统一是：**「改坏它有东西会红吗」——答「不会」的就是假绿风险。**

| # | 问题 | 为什么没人会发现 | 处置 |
|---|---|---|---|
| B1 | **`corp_loan_reconcile` 的 `Value` 是比例，spec 与 M0 契约样本要的是亿元** | 计划里**没有任何测试断言 Value 单位** ⇒ 全套绿 | TASK-004 `functional[2]`：**以 spec 为准**（见下） |
| B2 | `deposit_sum` 的 **±12% 边界无守卫** | 消融实验：`>` 改 `>=` **无一测试转红** | TASK-005 `boundary[0]`（Leader 独立追溯亦发现此条） |
| B3 | `corp_loan_reconcile` / `yoy_sanity` 的边界**同样无守卫** | 同上消融，两组均存活 | TASK-004 `boundary[0]` |
| B4 | spec 第 9 节「空 `Values`」**无回归防线** | 当前行为正确，但没东西阻止将来有人加特判改坏它 | TASK-004 `error_handling[0]` ④ |

### B1 的裁定（Leader 已复验 spec 与字段值）

spec 第 7 节「`Check.Value` 的单位约定」表逐字写着：

| 闸门 | Value | 单位 |
|---|---|---|
| `deposit_sum` | 残差占比 | 比例（0.0906） |
| **`corp_loan_reconcile`** | **残差绝对值** | **亿元（-1800）** |

并附「M0 契约样本已经这么记（`corp_loan_reconcile: -1203` 是亿元），此处沿用。
**差异写进各闸门的文档注释**，避免下游误读」。

而计划的实现写 `c := Check{Value: &r}`，`r` 是比例（reviewer 实测 golden2025 得 `0.0116`）。

**裁定：以 spec 为准。** 理由：①spec 是需求真相源；②**M0 契约样本是已经存在的数据**，
新数据改用比例会与它不一致；③下游是 Grafana 面板与 pending 人工复核（spec 明说），
把 `1.16%` 读成 `-1203 亿元`是量纲错读。

⇒ 实现改为：**判定仍用比例**，`Value` 记 `sum - total`（**保留符号**——spec 的例子 `-1800` 与
M0 的 `-1203` 都是负的，符号表示分项和小于合计这个方向）。

### B2/B3 的消融实验（reviewer 实施）

每次只翻转一个比较符，跑计划的全部测试，统计**新增**的红：

| 消融 | 结果 |
|---|---|
| `deposit_sum` 的 `r > Tolerance` → `>=` | **存活** |
| `corp_loan_reconcile` 的 `r <= Tolerance` → `<` | **存活** |
| `yoy_sanity` 的 `worst <= Max` → `<` | **存活** |
| `stock_continuity` 的 `r <= Max` → `<`（对照组） | **被杀** |

⇒ 七道闸门里四个数值阈值，**只有 `stock_continuity` 一个的边界方向有守卫**，
而 spec `boundary[2]` 点名要的 `deposit_sum` 恰好没有。

对照组被杀是这组实验的关键——它证明「无新增红」不是因为实验方法失灵。

---

## C 类：会误导但不致命

| # | 问题 | 处置 |
|---|---|---|
| C1 | 计划正文「`yoy_sanity` … 用测试钉住数量 = **11**」**实为 14**（Leader 已复验：9 社融 + 3 货币 + `deposit_balance_yoy` + `loan_balance_yoy`） | 计划里没有任何 Step 真写了这条测试，属**既错误又不存在的承诺**。`make(…, 0, 11)` 仅容量提示，无害。写入派单 prompt 提醒 |
| C2 | 计划自审 L2508「**无遗漏。**」不成立（B1/B2/B4 三处） | 已在 `requirement-dod-matrix.md` 与本文件登记 |
| C3 | 计划两张文件表互相矛盾；`sectionRules` 在 `extract.go` 而非 `sections.go` | 派单 prompt 提醒以任务 `writes` 为准 |

---

## D 类：reviewer 实测通过、**不要在返工时被一起改掉**

- T3 的 `Preceding` 实现与六条测试（6/6 PASS），含 `scanObservation` 的 `NullFloat64` absent 语义
- T5 的 `deposit_sum` 六行映射表（含「算不出的历史期次不计入均值」）
- T6 的 `TestStockContinuityDetectsJump` 四行（含 8/400 精确可表示的边界选值）
- T7 的豁免四条 + ULP + Save 接线两条
- `checkEnum` 的签名改动（**不改变任何既有错误信息**；既有测试只用 `assert.Contains`，无逐字断言）
- T2 的「27 字段双向一致」核心断言（reviewer 独立复算 + 实跑，**成立**）

## 字段常量核对：**零个对不上**

reviewer 对计划全文抽取的全部 `FieldXxx` 逐个 grep `fields.go`，
`FieldM0/M1/M2`、五个 `FieldDeposit*`、五个 `FieldLoan*`、三个 `FieldTSF*`、`FieldRateIBO`、`FieldFXReserve`
**全部存在且拼写一致**，三份独立证据（逐条 grep + `go build` 无输出 + `go vet` 无输出）。
这一部分是干净的。
