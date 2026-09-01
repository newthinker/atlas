# TASK-003 验证报告

- **验证者**：test-m1c3b-a
- **被验交付**：commit `d74d930e105e6843d8674d7ff148eda0411d751d`（父 `2433e5577b38f1d0fc8ba77bff4bd2641dee7421`），经 merge `4a65aa3` 进 master
- **verify_baseline**：head `962c3acb29705b58e21aaf4d4d64bcf8c77aca3a` / discovery_sha256 `59238f855d57a06bba9b1c4b8a494c5187ead89a1f22591ef6ed3e92f0e461ad`
- **我测量时的 HEAD**：`962c3acb29705b58e21aaf4d4d64bcf8c77aca3a`（= 基线，**零漂移**；对照测量另用 `2433e5577b38f1d0fc8ba77bff4bd2641dee7421`）
- **assignment_epoch**：1
- **结论：PASS（verified）**

---

## 0. 漂移、越界、依赖

| 检查 | 结果 |
|---|---|
| `verify_baseline.head` vs 我测量时 HEAD | 均为 `962c3ac`，**零漂移** |
| `discovery_sha256` | `59238f85…`，与基线逐字相同 |
| 声明 `writes`（3 项）vs 实际改动 | 实际只新建 2 个文件，**全部在声明内**；`store_test.go` 是预留、未改动（理由见 §5） |
| 从 dev 基线 `2433e55` 到当前 HEAD，声明范围内 numstat | `144/0 backfill_load.go`、`154/0 backfill_load_test.go` —— **与 `d74d930` 自身 numstat 逐条相同**，说明这两个文件在 merge 后无第三方改动 |
| `go.mod` / `go.sum` | 命中 **0** 条 |
| 注释里任务编号 | 新增行里 **2 处** `TASK-` 引用，**全部**是 `M1c-3b 的 TASK-003` |

---

## 1. done_criteria 覆盖矩阵

| # | 完成标准（摘要） | 证据 | 判定 |
|---|---|---|---|
| functional[0] | 新建 `backfill_load.go`，两个导出类型 + 三个非导出函数，**整段照抄需求文档 Step 3 含全部注释** | §2 | **PASS** |
| functional[1] | 三篇同键 ⇒ 合成一条，字段并集，`SourceIDs` 三个都在，`Conflicts` 空 | §3、§4 | **PASS** |
| functional[2] | `article_id` 优先月报；无月报取字典序最小；被丢弃的记进 `DroppedIDs` | §3、§4 | **PASS** |
| functional[3] | 输出按 `period` 升序、同期按 `period_type` 升序 | §3、§4 | **PASS** |
| boundary[0] | 单篇不得改写成 `merged@v1`，且 `DroppedIDs` 为空 | §3、§4 | **PASS** |
| boundary[1] | 组内按 `article_id` 升序；遍历 `fieldOrder` 而非 `Values` | §4 | **PASS** |
| error_handling[0] | 冲突记进 `Conflicts`，**不做静默取值** | §3、§4 | **PASS** |
| error_handling[1] | 同字段同值不算冲突 | §3、§4 | **PASS** |
| error_handling[2] | `TestPackageExposesNoWriteFunctions` 不因本任务转红；导出类型不改成非导出 | §5 | **PASS** |
| non_functional[0] | gofmt / vet / test / 覆盖率 ≥95.9% / 无新依赖 / 编号带 milestone 前缀 | §7 | **PASS** |
| non_functional[1] | AD-4 交付流程 | §7 | **PASS** |

---

## 2. functional[0]：「逐字节照抄」我独立复现了

这一条的判据依赖需求文档，而**需求文档不在仓库里**（`git grep` 全仓库、`.arcforge/docs/01-design/` 三份都只有 101–141 行，远不到「第 535-678 行」）。我把它找了出来：

```
/private/tmp/.../scratchpad 同级的 tasks/bhs7nbz6m.output
  1408 行，mtime 2026-08-31 23:31:31
  首行自述：「# Hestia M1c-3b 实施计划 · 历史回填批量入库」
```

该 mtime **早于 dev 开工时刻**（`assigned → in_progress` @ `2026-09-01T00:59:20Z`），且 `678 − 535 + 1 = 144` 与交付文件行数吻合。文档第 534 行是 ` ```go `、第 679 行是 ` ``` `，故 535-678 恰是该代码块正文。

**逐字节比对结果**：

| | 行数 | sha256 |
|---|---|---|
| 文档 `bhs7nbz6m.output` 第 535-678 行 | 144 | `e8f47b381118f00621bdf676e981940f35fa696896c6cb5858e757cf1b144489` |
| 交付的 `internal/hestia/backfill_load.go` @ `962c3ac` | 144 | `e8f47b381118f00621bdf676e981940f35fa696896c6cb5858e757cf1b144489` |
| `diff` 行数 | — | **0** |

**⇒ 逐字节一致，含全部注释。** （该 sha256 亦与 dev 在 discovery 里报的 `e8f47b38…` 相同。）

**测试文件（Step 1 块）的差异是合理的**：文档的测试代码块 122 行，交付文件 154 行，`diff` 显示交付**多出 32 行前缀**——`package hestia`、`import`、以及一段「done_criteria → 测试映射」注释。**其余 122 行逐字相同**（122 + 32 = 154）。而 `functional[0]` 要求逐字节照抄的是 **Step 3（实现）**，不是 Step 1；文档的测试块本身不含 package/import，不加就无法编译。**不构成偏离。**

---

## 3. 七个 `TestMerge*` 全通过

测量树 `962c3ac`：

```
--- PASS: TestMergeByBusinessKeyUnionsFields
--- PASS: TestMergeSingleArticleKeepsOriginalExtractor
--- PASS: TestMergePrefersMonthlyArticleID
--- PASS: TestMergeWithoutMonthlyFallsBackToSmallestID
--- PASS: TestMergeRecordsFieldConflict
--- PASS: TestMergeSameValueIsNotAConflict
--- PASS: TestMergeIsStableAndSorted
```

---

## 4. 我自己的探针（不采信测试的绿色，直接对函数求值）

测量树 `962c3ac`：

```
[S1] 正序输入 -> [2020-04-10 2020-04-20]
[S1] 逆序输入 -> [2020-04-20 2020-04-10]
[S1] 两次输出相同 = false  ⇒ dev 声明的「不保证」成立 = true
[S2] 乱序输入不同 period -> [2020-03 2020-05]      （DoD 保证的升序）
[S3] 同 period 不同 period_type -> [annual quarterly]（DoD 保证的第二项）
[K] 冲突条数=1 ; 记录={Field:m2 A:100 B:200 FromA:id-1 FromB:id-2} ; 取的值=100（保留先见的 A，且已出声）
[K] 同值冲突条数=0
[O] 输入顺序 Z,A -> SourceIDs=[id-A id-Z] Parts=[tsf-flow@v1 tsf-stock@v1]
[O] 输入顺序 A,Z -> SourceIDs=[id-A id-Z] Parts=[tsf-flow@v1 tsf-stock@v1]
[O] 两次一致 = true
[O] 无月报时 ArticleID=id-A（字典序最小）; DroppedIDs=[id-Z]
```

- **boundary[1] 得到实测支持**：交换输入顺序，`SourceIDs` 与 `Parts` **逐项不变** ⇒ 组内按 `article_id` 升序生效；代码层面 `mergeGroup` 第 78-80 行 `SortStableFunc` 比 `ArticleID`，第 99 行 `for _, f := range fieldOrder` 而非遍历 `Values`，两条都对上。
- **error_handling[0]**：冲突被记录且**带 FromA/FromB 指名道姓**；值保留先见的 A 但**已出声**，不是静默取值。
- **functional[2]**：无月报时取字典序最小、被丢弃的进 `DroppedIDs`。

### 4.1 排序保证的确切射程——dev 的「不保证」声明**属实**

`mergeByBusinessKey` 的分组键是三元组 `(period, periodType, publishedAt)`，而 `SortStableFunc` 的比较函数只比**前两项**。两组 `(period, periodType)` 相同而 `publishedAt` 不同时比较相等 ⇒ 保持插入序 = manifest 序。

我用构造用例实测（`[S1]`）：同 `(2020-03, monthly)`、`published_at` 分别为 `2020-04-10` / `2020-04-20` 的两组，**正序输入与逆序输入得到相反的输出顺序**。

⇒ dev 在 `interfaces_exposed` 里写给 TASK-006 的射程说明（「保证……不保证同期同类型而 `published_at` 不同的两组之间的相对顺序」）**经实测成立**，不是纸面推断。实现符合 `functional[3]` 的字面要求，缺口在**注释的理由比保证宽**（「稳定才能逐次 diff」需要「manifest 同期内顺序不变」这个额外前提）。dev 未自行修，因为那会破坏 `functional[0]` 要求的逐字节照抄——**这个处置我认可**，Leader 已批准。

### 4.2 ⚠️ 我自己的一处探针伪影，一并记下

我的探针里有一行：

```
[C] 组内三篇 CaliberVersion: 2015-01 / 2015-01 / 2015-01 ; 合并后 = 2023-01
```

看起来像「组内不一致」的缺陷。**不是。** `mkParsed`（测试辅助）把 `CaliberVersion` **硬编码成 `"2023-01"`**、与 `period` 无关，而我左边打的是**生产路径**的 `caliberFor("2019-12")`。两个数来自两条不同的路径，**我的探针在比不可比的东西**。

真正该核的是 dev 的论证，我按代码核了：`parse.go:251` 写 `CaliberVersion: caliberFor(period)`，而 `caliberFor` 的函数体只遍历 `caliberChanges` 拿 `c.since` 与 **`period`** 比——**除 `period` 外不吃任何输入**。`period` 是业务键成员 ⇒ 组内必然一致。**「由构造保证、不需要额外守卫」成立。**

（记在这里是因为它正是「拿高度相关的可观测量代替性质本身」的形状；差一步我会把测试辅助的硬编码值报成生产缺陷。）

---

## 5. `store_test.go` 未改动 —— dev 的理由我独立核了

`error_handling[2]` 说导出类型不会触发导出面守卫。dev 声称自己读了判定逻辑而非照搬 Leader 的结论。我去看了源码：

```go
// internal/hestia/store_test.go:414
fn, ok := d.(*ast.FuncDecl)
if !ok || !fn.Name.IsExported() {
    continue
}
```

只收 `*ast.FuncDecl`；类型声明是 `*ast.GenDecl`，`ok` 为 false ⇒ `continue`，**根本不进那个循环**。该文件自己的注释（`:522`、`:587`）也写着同一件事。

⇒ `MergeConflict` / `MergedObservation` 是 struct 类型，不触发；三个新函数 `mergeByBusinessKey` / `mergeGroup` / `pickArticleID` **全是非导出**。实测 `TestPackageExposesNoWriteFunctions` **PASS**。

`store_test.go` 在 `writes` 内但未改动 —— DoD 明写「若最终没动它，如实说明即可」，dev 如实说明了。**不构成缺陷。**

---

## 6. 接缝缺口（Leader 已知，**不在本任务 DoD 内**，不判罚）

我实测确认该缺口存在：

```
[G] MergedObservation.Parts     = [rule-monthly@v1 tsf-stock@v1 tsf-flow@v1]
[G] MergedObservation.Obs.Parts = [] (len=0)
```

代码层面：`backfill_load.go` 里 `Obs.Parts` 的赋值**命中 0 处**；`mergeGroup` 只写了 `m.Obs.Values` / `.Meta.Extractor` / `.Meta.ArticleID`。而 `gateCompleteness` 读的是 `Obs.Parts`。

**但 TASK-003 的 DoD 要求的是 `MergedObservation.Parts`**（`functional[1]` 与 `interfaces_exposed` 都是这个口径），dev 做到了。缺口在 TASK-003 产出与 TASK-011 消费之间的**接缝**，两侧 DoD 各自只覆盖自己那一半。修复已并进 TASK-006 的 `functional[1]`。**本报告按 TASK-003 自己的 DoD 判，不以此判红。**

---

## 7. 门禁与交付流程

| 项 | 结果 | 测量树 |
|---|---|---|
| `go test ./internal/hestia/... -count=1` | `ok` | `962c3ac` |
| `go test ./cmd/... -count=1` | `ok` | `962c3ac` |
| `gofmt -l internal/hestia cmd/atlas` | 仅 `cmd/atlas/backtest_test.go`、`cmd/atlas/crisis_test.go`（**零新增项**） | `962c3ac` |
| `go vet ./internal/hestia/... ./cmd/...` | **0 行输出** | `962c3ac` |
| 覆盖率**背对背交错 CUR/BASE/BASE/CUR** | `962c3ac` = **96.0%** / `2433e55` = 95.9% / `2433e55` = 95.9% / `962c3ac` = **96.0%** | 两树 |
| 不低于基线 95.9% | 满足（**高于**） | — |
| AD-4：merge 早于 `dev_done` | merge `4a65aa3` @ `2026-09-01T01:36:07Z` < `dev_done` @ `01:37:56Z`（**早 1 分 49 秒**） | — |
| AD-4：commit message 锚定 | `feat(TASK-003): 按 (period,period_type,published_at) 合并观测，字段取并集、冲突不静默` ✔ | — |
| AD-4：自拆 worktree | `git worktree list` 中无 `wt-TASK-003-m1c3b` | — |

> ⚠️ 覆盖率高于基线的部分不全归本任务——同批合入的 TASK-005/008/011 也改变了整棵树的分母。判据是「不低于 95.9%」，满足。

---

## 8. 结论

十一条 done_criteria **逐条 PASS**。

最吃紧的 `functional[0]`（逐字节照抄）**不是采信 dev 的自陈**：我定位到需求文档本体（mtime 早于 dev 开工），把第 535-678 行与交付文件做 sha256 比对，**逐字节相同**。行为性质由**七个交付测试 + 我自己的一组构造探针**双重覆盖，其中排序射程与冲突取证是我自己构造用例实测的。两处 dev 的自主判断（`store_test.go` 的 AST 守卫、`CaliberVersion` 的构造保证）我都回到源码求值确认成立。

**判定：VERIFIED。**
