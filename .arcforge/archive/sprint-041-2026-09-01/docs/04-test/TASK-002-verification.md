# TASK-002 验证报告

- **验证者**：test-m1c3b-a
- **被验交付**：commit `87c42333f3378aed8efad63fba67379d1e1e4833`（父 `32bc1e5f306386ee5c69a54b4bae3e0184aa30f2`），经 merge `db19e803a08d63cdd42af8a4bac4dfddc1ff3bf5` 进 master
- **verify_baseline**：head `2433e5577b38f1d0fc8ba77bff4bd2641dee7421` / discovery_sha256 `d0521d5e17e21e908493260fdb3ac0e841fc3c00079dd3a0b702f022eb70fc77`
- **我测量时的 HEAD**：`2433e5577b38f1d0fc8ba77bff4bd2641dee7421`（= 基线，**零漂移**；隔离测量另用 `87c4233` 与 `32bc1e5`，下文每个数字都注明测自哪棵树）
- **assignment_epoch**：1
- **结论：PASS（verified）** —— 含一处 DoD 缺陷发现，订正已落在 TASK-011 的 `functional[4]`，见 §10

---

## 0. 漂移与越界核对

| 检查 | 结果 |
|---|---|
| `verify_baseline.head` vs 我测量时 HEAD | 均为 `2433e5577b38f1d0fc8ba77bff4bd2641dee7421`，**零漂移** |
| `discovery_sha256` | `d0521d5e…` 与基线逐字相同，未漂移 |
| 声明 `writes`（5 项）vs 实际改动 | 实际改动 4 个文件，**全部在声明内**；`types_test.go` 在声明内但未改（理由见 §7）|
| 声明外文件 | 0 个 |
| `go.mod` / `go.sum` | 在 `87c4233` 中**命中 0 条**（Global Constraint 满足）|
| dev 基线 `32bc1e5` → 当前 HEAD，声明范围内 numstat | `7/0 parse_test.go`、`35/1 required.go`、`106/2 required_test.go`、`12/0 types.go` —— **与 `87c4233` 自身 numstat 逐条相同**，说明这四个文件在 merge 后无第三方改动 |

> merge `db19e80` 相对 `87c4233` 在 `internal/hestia` 下另有 175 行差异，逐一核过全部落在 `sections.go`（TASK-009）与 `thresholds{,_test}.go`（TASK-004），**不在 TASK-002 的声明范围内**。

---

## 1. done_criteria 覆盖矩阵

| # | 完成标准（摘要） | verify_by | 证据 | 判定 |
|---|---|---|---|---|
| functional[0] | `extractorMerged` 常量加在 `extractorTSFFlow` 之后 + 进 `validExtractors` + 理由注释 | test | §2 | **PASS** |
| functional[1] | `mergedRequiredFields` 返回并集、按 `fieldOrder` 排序、去重 | test | §3 | **PASS** |
| functional[2] | 三篇齐全 ⇒ `without(fieldOrder, fxSectionFields())`（52）；只有社融两篇 ⇒ 两篇并集且不含 `FieldM2` | test | §3、§4 | **PASS** |
| functional[3] | `requiredFields(extractorMerged)` 返回 nil；`default` 注释区分两者成因 | test | §3、§2 | **PASS** |
| functional[4] | 三处白名单守卫**逐一表态**，不改断言逻辑 | test | §5、§6 | **PASS** |
| functional[5] | 无生产调用点是刻意分工，**不得据此 reject** | review | §7 | **PASS（不判罚）** |
| boundary[0] | 动态取并集的理由**原样落进**函数注释 | review | §8、§10 | **PASS**（条目被满足；理由文本内含 DoD 自身的数字错误，见 §10）|
| boundary[1] | 不得重复；按 `fieldOrder` 升序 | test | §3、§6 | **PASS** |
| error_handling[0] | 守卫转红是它在正常工作，去表态别改断言；新增符号须非导出 | test | §5、§7 | **PASS** |
| non_functional[0] | gofmt / vet / test / 覆盖率 ≥95.9% / 无新依赖 / 编号带 milestone 前缀 | test | §9 | **PASS** |
| non_functional[1] | AD-4 交付流程 | review | §9 | **PASS** |

---

## 2. 取值域与常量（functional[0] / functional[3] 的注释部分）

`types.go` 新增（+12/−0，全部在 `const` 块与 `validExtractors` 注释）：

- `extractorMerged = "merged@v1"` 位于 `extractorTSFFlow` **之后**（`types.go:155`）；
- 注释含 DoD 点名的那段理由，逐字有「它单独一个字符串**说不出必填集**」「必填集取决于由哪几篇合成」「2020-01|monthly 只有 2 篇」；
- 进 `validExtractors`，并在白名单注释里补了它与前几个的区别（**那几个迟早有 HTML 样本，它是装配出来的、构造上不可能有**）。

我的探针（测量树 `2433e557`）：

```
[V] len(validExtractors) = 8 ; 内容 = [rule@v1 rule@v2 llm-fallback@v1 rule-monthly@v1 rule-monthly@v2 tsf-stock@v1 tsf-flow@v1 merged@v1]
[V] extractorMerged 在其中 = true ; 位置 = 7
```

`required.go` 的 `default` 分支注释已写明两者成因不同：`llm-fallback@v1` 是「还没实现」（M1c-4 补上分支后就有必填集），`merged@v1` 是「这一列结构上说不出必填集」——**永远**不会有分支。

---

## 3. 函数行为：我自己的探针（不采信 discovery 的数字）

测量树 `2433e5577b38f1d0fc8ba77bff4bd2641dee7421`：

```
[N] len(fieldOrder)            = 54      [N] len(fxSectionFields())   = 2
[N] requiredFields(rule@v1)    = 27      [N] requiredFields(rule@v2)  = 54
[N] requiredFields(monthlyV1)  = 25      [N] requiredFields(monthlyV2)= 52
[N] requiredFields(tsfStock)   = 18      [N] requiredFields(tsfFlow)  = 9
[N] without(fieldOrder, fx)    = 52      [N] 25+18+9                  = 52
[U] len(merged(3 parts))       = 52      [U] 与 without(fieldOrder,fx) 同集合 = true
[D] merged([tsfStock,tsfStock]) len=18 ; 与 merged([tsfStock]) 等长 = true
[T] merged([stock,flow]) len=27 ; 含 FieldM2 = false
[B] requiredFields(extractorMerged) == nil = true (len=0)
```

- **functional[2] 的 52 独立复现**：`25+18+9 = 52 = 54−2`，且 `merged(3 parts)` 与 `without(fieldOrder, fx)` 是**同一集合**（我用各自排序后比对，不依赖被测函数自身的顺序）。
- **52 与 54 确实分属两组**：`rule@v1` = 27（季报，**含** fx 两字段），`rule-monthly@v1` = 25（月报无外汇板块）。需求文档 Task 3 的「27/18/9=54」与本任务的「25/18/9=52」**两个都对**，不是同一组 extractor。
- **去重**：同一 extractor 传两次，长度不变（18）。
- **两篇（缺月报）**：27 个字段，**不含 `FieldM2`** —— 25 个非社融字段是 absent-by-design，未进必填集。
- **bare merged 返回 nil** 属实。

---

## 4. 独立复现需求文档 Task 2 Step 1 的矛盾（dev 偏离的正当性）

dev 弃用了需求文档的 `require.Equal(t, tsfStockFields(), got)`。我不采信转述，自己测下标：

```
[C] tsfStockFields() len=18 首=tsf_stock_rmb_loan(fieldOrder下标 3) 末=tsf_stock_yoy(fieldOrder下标 1)
[C] tsfStockFields() 的 fieldOrder 下标序列 = [3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 0 1] ; 是否已升序 = false
[C] mergedRequiredFields([tsf-stock]) 的下标序列 = [0 1 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18] ; 是否升序 = true
[C] require.Equal(tsfStockFields(), merged([tsf-stock])) 会通过吗 = false  ⇒ 矛盾成立 = true
```

dev 报的「首元素下标 3、末元素下标 1」**逐字对上**。`tsfStockFields()` 把两个总量字段 `append` 在**末尾**，`fieldOrder` 里它们排在**最前** ⇒ Step 1 的 `Equal` 与 Step 3 的「按 `fieldOrder` 排序」**互斥，任何实现都不可能同时满足**。

改用 DoD `boundary[1]` 陈述的两条**性质**（`ElementsMatch` 比多重集 + `fieldOrder` 升序 + 一格阴性防空转）是**正确处置**，且守得更准：`ElementsMatch` 对重复元素照样红，性质(1) 被完整钉住而不误钉顺序。

---

## 5. 三处白名单守卫：只改声明，未动断言逻辑（functional[4] / error_handling[0]）

| 守卫 | 位置 | 实际改动 | 断言逻辑是否被改 |
|---|---|---|---|
| `TestExtractorEnumErrorListsEveryValidValue` | `required_test.go:288` | 逐字清单加 `"merged@v1"`；`assert.Len(t, validExtractors, 7)` → `8`；注释 7→8 并**新增一句「不要图省事改成 range validExtractors」警告** | **否** —— 仍是 `for _, want := range []string{…字面量…}`，**没有**被改成 `range validExtractors` |
| `TestEveryValidExtractorHasARequiredFieldsDecision` | `required_test.go:320` | `exempt` 表加一条 `"merged@v1"` 及理由（写明与 `llm-fallback@v1` 成因不同：那条将来会被删、这条不会） | **否** |
| `TestParseCoversAllKinds` | `parse_test.go:806` | `exempt` 表加一条 `"merged@v1"` 及理由（「构造上不可能有真实 HTML 样本：它不是从 HTML 解析出来的，是**装配**出来的」） | **否** |

⇒ DoD 特别点名的风险（把 `:288` 改成遍历，会正好放过它唯一挡得住的那类缺陷）**没有发生**，而且 dev 反向加固了它。

**新增符号均非导出**：`extractorMerged`、`mergedRequiredFields` 都是小写开头，`TestPackageExposesNoWriteFunctions` 绿。

---

## 6. 消融：我独立复现三组，含**外溢度**

**方法**：全部跑在我自己的隔离 detached worktree 上，主仓库一个字节不碰；对照组先跑一次；每个变异先验 sha256 确实改变、打印逐行 diff、`go build` 通过后才计数。

| 消融 | 顶层 FAIL 条数 | FAIL 名单 | dev 的自陈 | 判定 |
|---|---|---|---|---|
| **对照组**（未变异） | **0** | — | — | 基准成立 |
| **A4**：把 `extractorMerged` 从 `validExtractors` 里删掉 | **恰好 2** | `TestEveryValidExtractorHasARequiredFieldsDecision`、`TestExtractorEnumErrorListsEveryValidValue` | 「后者是它的反向断言在工作，**不是我引入的耦合**」 | **属实** |
| **A2**：`mergedRequiredFields` 按 `fieldOrder` **倒序**输出 | **恰好 1** | `TestMergedRequiredFieldsIsOrderedAndDeduped` | 「去重与并集断言仍绿，故这条**精确指向排序性质**」 | **属实** |
| **C**：只加 const + 进 `validExtractors`、**不改任何测试** | **恰好 3** | 上述两条 + `TestParseCoversAllKinds` | 「恰好红三条，不多不少」 | **属实** |

**外溢度是这三条的关键**：不是「有测试红了」，而是**恰好红了哪几条**。A2 只红 1 条证明排序断言不是靠别的测试连坐；A4 红 2 条证明那条反向断言（「豁免表的键必须在 `validExtractors` 里」）确实是既有守卫在工作。

**还原核实**（不靠 harness 的自报）：
- 消融 worktree `git diff 2433e557 -- internal/hestia` = **0 行**；
- 主仓库四个文件的 sha256 与 `git show 2433e557:<file>` **逐字节相同**；
- 主仓库 `git status --porcelain internal/hestia` = **空**。

> ⚠️ 我的 harness 有一处比较 bug 要记下：`shasum -a 256 < file` 的输出带尾随 `  -`，与 `cut -d' ' -f1` 出的纯 sha 比较**恒不等**，于是中途打了两次「还原后 sha 复位 = NO」的**假告警**。我没有采信那两行 echo，而是用上面三条独立判据复核。这正是「硬编码/格式错配的 echo 永远不会错也永远不告诉你任何事」的又一例 —— **判定只锚定数据，不锚定结论行**。

---

## 7. 无生产调用点与 `types_test.go` 未改（functional[5] / DoD 点名的两处「像遗漏」）

- **`mergedRequiredFields` 只有测试调用点**：`validate.go` 未被触碰，生产接线在 TASK-011。DoD `functional[5]` 明示这是刻意分工，**本报告不以此判罚**。
- **`types_test.go` 在 `writes` 内但未改**：`32bc1e5 → 87c4233` 的 diff **0 行**。dev 给的理由我独立核过并**成立**：`TestMetaValidateAcceptsKnownEnums` 遍历的是硬编码三值字面量 `{"rule@v1", "rule@v2", "llm-fallback@v1"}`，**不是** `validExtractors`，因此新增第八个取值构造上不会触及它。`writes` 是可写路径的上界、不是必写清单。

**下游契约核实**（TASK-011 会依赖，故我单独验）：

```
[X] merged([未知, merged@v1, tsfFlow]) len=9 ; 等于 tsfFlow 集合 = true   ← 脏 parts 静默略过，属实
[X] merged(nil) == nil = false ; len = 0                                ← 空切片、不是 nil，与 discovery 给下游的承诺逐字一致
```

后一条是**实质性的**：`merged(nil)` 返回非 nil 的空切片，意味着下游若写 `if req == nil { skipped }` 会漏判。discovery 的 `notes_for_downstream` 已明确写出这一点。

---

## 8. 动态取并集的理由已原样落盘（boundary[0]）

DoD 要求那段理由「原样落进 `mergedRequiredFields` 的注释」。核对结果：**落了**，`required.go:120-133` 逐字含

- 「实测 42 个合并组里并非每组都齐 3 篇（2020-01|monthly 只有 2 篇，月报那篇落在解析失败格里）」
- 「把 by-design 的缺席记成 failed，completeness 这个指标就废了」
- 「这条是本函数存在的**全部**原因」

排序的理由也落了（与 `gateMagnitudeSanity` 遍历 `fieldOrder` 同一条：map 迭代顺序随机会让排查变成猜谜）。

**⇒ 本条目被满足。但这段被要求原样落盘的文本里，DoD 自身的一个数字是错的 —— 见下一节。**

---

## 9. 门禁与交付流程

| 项 | 结果 | 测量树 |
|---|---|---|
| `go test ./internal/hestia/... -count=1` | 全绿 | `87c4233`、`2433e557` |
| 三个新测试 + 三个守卫 + `TestPackageExposesNoWriteFunctions` + `TestRequiredFieldsRejectsAmbiguousExtractor` | **8/8 PASS**（逐个 `-run` 跑过） | `87c4233` |
| 覆盖率（`go test -cover`）**背对背交错 DEL/BASE/BASE/DEL** | `87c4233` = **95.9%**、`32bc1e5` = **95.9%**、再跑 `32bc1e5` = 95.9%、再跑 `87c4233` = 95.9% | 两树 |
| 覆盖率（当前 master） | **95.9%** | `2433e5577b38f1d0fc8ba77bff4bd2641dee7421` |
| 不低于基线 95.9% | 满足（等于） | — |
| `gofmt -l internal/hestia cmd/atlas` | 仅 `cmd/atlas/backtest_test.go`、`cmd/atlas/crisis_test.go`（**2 项，零新增**） | `87c4233` |
| `go vet ./internal/hestia/... ./cmd/...` | **0 行输出** | `87c4233` |
| 无新增依赖 | `go.mod`/`go.sum` 命中 **0** | — |
| 注释里任务编号带 milestone 前缀 | 新增行里 **9 处** `TASK-` 引用**全部**是 `M1c-3b 的 TASK-00X`，**无裸编号** | — |
| AD-4：merge 早于 `dev_done` | merge `db19e80` @ `2026-08-31T23:56:02Z` < `dev_done` @ `23:56:48Z`（**早 46 秒**） | — |
| AD-4：commit message 锚定 | `feat(TASK-002): merged@v1 取值域 + 必填集按参与合并的 extractor 动态取并集` ✔ | — |
| AD-4：自拆 worktree | `git worktree list` 中**无** `wt-TASK-002-m1c3b` | — |

---

## 10. 🔴 单列发现：DoD `boundary[0]` 强制原样落盘的「缺 27 个字段」实测应为 25

### 10.1 事实

DoD `boundary[0]` 要求原样落盘的句子是：

> 实测 `2020-01|monthly` 那组**只有 2 篇**（月报那篇落在解析失败格里）。硬套 `rule-monthly@v2` 的 52 字段会让这类组恒报「**缺 27 个字段**」…

dev 照办，该句逐字进了 `internal/hestia/required.go:127`。**那个 27 是错的，应为 25。**

我在交付树 `87c4233` 上的探针：

```
[P] |rule-monthly@v2| = 52 ; |tsfStock| = 18 ; |tsfFlow| = 9 ; |stock∪flow| = 27 ; |rule-monthly@v1| = 25

[Q1] 组里只有社融两篇（月报那篇解析失败）→ 硬套 v2 会报缺 25 个字段   ← 注释描述的正是这个情形
[Q2] 组里只有月报一篇                    → 硬套 v2 会报缺 27 个字段   ← 27 是"相反情形"的数

[R] Q1 缺的那批恰是 rule-monthly@v1 的 25 个 = true
[R] Q2 缺的那批恰是 stock∪flow 的 27 个     = true
```

算术：`52 − (18 + 9) = 25`。**注释描述的是 Q1，用的却是 Q2 的数。**

**同一次交付里 dev 自己的测试断言用的就是 25** —— `required_test.go` 的
`"缺月报那篇时，25 个非社融字段是 absent-by-design，不得进必填集"`。
⇒ **仓库当前内部自相矛盾：注释错、测试对。**

### 10.2 溯源：三个正确的 27，被搬到了第四个情形

我把仓库里所有的 27 逐个求值，结论是**只有本任务新增这一处是错的**：

| 位置 | 情形 | 实测 | 判定 |
|---|---|---|---|
| `types.go:141`（既有） | 社融独立报告复用 `rule@v1` | 缺 **27** | ✅ 对 |
| `validate.go:137`（既有） | v1 期次相对 v2 | 缺 **27**（54 − 27） | ✅ 对 |
| **`required.go:127`（本任务新增）** | 组里只剩社融两篇，硬套 `rule-monthly@v2` | 缺 **25** | ❌ 写成了 27 |

**为什么它能滑过去**：

```
[O1] 社融存量独立报告复用 rule@v1 → 缺 27 ；社融增量独立报告复用 rule@v1 → 缺 27
[O2] v1 期次相对 v2              → 缺 27（|v2|-|v1| = 27）
[O3] 组里只剩社融两篇，硬套 rule-monthly@v2 → 缺 25
[O4] rule@v1 的 27 个 = rule-monthly@v1 的 25 个 + fx 的 2 个 = true
[O5] rule@v1 的 27 个与 stock∪flow 的 27 个不相交 = true（交集 0）
```

`stock ∪ flow` **恰好也是 27 个字段**（18+9），且与 `rule@v1` 的 27 个**完全不相交**。于是 27 在仓库里以三种不同含义**同时正确**地存在着；搬到第四个情形时，它对应的是**存在**的字段数，而不是**缺失**的字段数。

⇒ 这是「**数字是真的，却被搬到了测不出它的条件下 / 正确的局部陈述在转写时丢了限定条件**」的**第三例**（前两例是需求文档 Task 2 的「54 应为 52」与 Task 3 的 `require.Equal` 矛盾，两例都由 dev 在本任务中发现并申报）。

### 10.3 为什么仍判 PASS，以及订正落在哪里

**判 PASS 的理由**：`boundary[0]` 这一格要求的是「这条理由**原样落进**函数注释」，dev 原样落了 —— **DoD 的内容错，不等于 DoD 的条目未被满足**。dev 一行没做错。

我把这一点上报 Leader 并给出 A（`verified` + 另行订正）/ B（`rejected` + `dod_defect`）两个选项，Leader 独立复算得到与我逐字相同的 Q1/Q2 两组数后**裁决 A**，理由是：

1. `boundary[0]` 确实被满足；
2. `dod_defect` **不免除 `rework_count`**（只有 `env_infra` 免计数），走 B 会给一个一行没做错的 dev 记一次返工，而 `rework_count` 是熔断判据（`max_rework=3`），用它记「不是他的锅」会污染那个判据；
3. 订正有比「另开任务」更强的载体，而且**已经落好了**。

**订正的落点（我已直读核实，不是采信转述）**：

- 写进 **`.arcforge/tasks/TASK-011.json` 的 `done_criteria.functional[4]`** —— 不是 discovery、不是 final-report。TASK-011 正是接线 `mergedRequiredFields` 的任务，它的 dev **必然会读那段注释**。
- TASK-011 的 `writes` 已扩到 `./internal/hestia/required.go`（现为 4 项：`types.go` / `validate.go` / `validate_test.go` / `required.go`）。
- 该条 DoD 里写明了可机械求值的验收判据：改完 `grep -c '缺 27' internal/hestia/required.go` 应为 **0**，且只改这一个数字与配套说明、保留「这条是本函数存在的全部原因」那句；并把「为什么会滑过去」（三个 27 的巧合）一并写进订正后的注释。

**这个决定有一个明确的代价，Leader 已承认并接受**：从现在到 TASK-011 交付之间，master 上确实带着一句自相矛盾的注释。它有**明确的关闭点**（TASK-011 的一条 DoD）而不是「以后再说」，另在 final-report 的移交项里留了第二道保险。

---

## 11. 其余核对（不构成缺陷）

- **code-simplifier**：我 grep 过 discovery 全文，**完全没有出现这个词** —— 既未声称「跑过了」也未声称「审过没问题」，不存在把后者写成前者的过度声称。Leader 已批准这条有据偏离（理由：`discovery` 子命令无 `--expect-status`，写它与派验之间不原子；而结论是「没问题」信息量约等于零），**不判罚**。
- **discovery 的自证锚点**：`resample_anchor` 声明全部数字统一重采于 `87c4233` 且当时 `git status --porcelain` 为 0 条 —— 与我在同一 commit 上独立测得的数字逐条一致，锚点可信。
- **`merged(nil)` 这条边界的双路径印证**：我从「验证下游契约」这条路验到它返回非 nil 空切片；Leader 从 TASK-011 的实现形状那条路撞到同一件事（原写 `if req == nil { skipped }` 会让空 `Parts` 一路落到 `CheckPassed`，比 skipped 更糟因为完全静默），已把 DoD 改成 `len(req) == 0`。**两条独立路径验到同一条边界**，该契约现已被下游正确消费。

---

## 12. 结论

十一条 done_criteria **逐条 PASS**，无失败用例、无未覆盖项、无覆盖率不足、无越界、无新增依赖。

关键性质由**独立实现的探针**（基数、并集、去重、升序、nil、脏 parts、空切片）与**三组带外溢度的消融**（恰好红 2 / 恰好红 1 / 恰好红 3）共同证实，而非只看「测试全绿」。dev 的三条自陈（A4 的反向断言联动、A2 精确指向排序、只加 const 恰好红三条）与那处需求文档矛盾，**全部经我独立复现属实**。

发现一处 DoD 自身的数字错误（§10），dev 无过错，订正已落进 TASK-011 `functional[4]` 这一强载体并附可机械求值的判据。

**判定：VERIFIED。**
