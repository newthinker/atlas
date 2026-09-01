# TASK-008 验证报告

- **验证者**：test-m1c3b-a
- **被验交付**：commit `5f68f58eb66e784b8e23307cceae9756b4b7eda7`，经 merge `962c3acb29705b58e21aaf4d4d64bcf8c77aca3a` 进 master（`962c3ac` 的第二父即 `5f68f58`）
- **verify_baseline**：head `962c3acb29705b58e21aaf4d4d64bcf8c77aca3a` / discovery_sha256 `32a57cff79e51d7519b71438541860404b170abe5e4ae8fbf6880a6cce2aa818`
- **我测量时的 HEAD**：`962c3acb29705b58e21aaf4d4d64bcf8c77aca3a`（= 基线，**零漂移**；对照测量用 `57a9cad153303a3dbf2c3a6a97c8ac3cfa8a665a`，即本任务 merge 之前的 master）
- **assignment_epoch**：1
- **结论：PASS（verified）**

---

## 0. 漂移、越界、依赖

| 检查 | 结果 |
|---|---|
| `verify_baseline.head` vs 我测量时 HEAD | 均为 `962c3ac`，**零漂移** |
| `discovery_sha256` | `32a57cff…`，与基线逐字相同 |
| 合入的是否逐字节是 dev 那版 | `git diff 5f68f58 HEAD -- <四个文件>` = **0 行** |
| 声明 `writes`（4 项）vs 实际改动 | 4 个文件**逐一对应**，无越界 |
| `go.mod` / `go.sum` | 命中 **0** 条 |
| 注释里任务编号 | 新增行里 **6 处** `TASK-` 引用，**全部**是 `M1c-3b 的 TASK-008` |

---

## 1. done_criteria 覆盖矩阵

| # | 完成标准（摘要） | 证据 | 判定 |
|---|---|---|---|
| functional[0] | R9：为 `stockContinuityRates` 的输入加业务键唯一性断言，注释写明「安全来自发布习惯不来自代码」 | §2 | **PASS** |
| functional[1] | 1a：让 `2022-05` 的标签说真话，**只改标签不补解析器** | §3 | **PASS** |
| functional[2] | 结转项 9：`23` → 真值，写明「随语料变必须重采」，**订正前自己重采** | §4 | **PASS** |
| functional[3] | 矛盾标签统一为「M1c-4 的兜底工作量」 | §5 | **PASS** |
| boundary[0] | 除 R9 新增断言外不改行为；PASS 差值恰等于新增断言数，**背对背** | §6 | **PASS** |
| boundary[1] | 1a 改标签不改分类结果，`2022-05` 仍落同一格 | §3 | **PASS** |
| error_handling[0] | 重采若不是 19 先怀疑判据并报 Leader | §4（实测就是 19，前件未触发） | **PASS** |
| error_handling[1] | `grep -rn 'M1c-3 入库前' internal/hestia/` 应零命中 | §5 | **PASS** |
| non_functional[0] | gofmt / vet / test / 覆盖率 / 无新依赖 / 编号前缀 | §8 | **PASS** |
| non_functional[1] | AD-4 交付流程 | §8 | **PASS** |

---

## 2. functional[0]：R9 断言

新增 `TestStockContinuityRatesAssumeUniqueBusinessKey`（`calibrate_report_test.go`），三段结构：

1. **危害实证**：同 `(annual, 2022-12)` 来两条 ⇒ `N=2`、`Min` 落到 `0` —— 「重复业务键会造出一档环比 0%，而它和一条真正平稳的样本在报告上长得一模一样」。
2. **阴性对照**：去掉重复那条 ⇒ `N=1`、`Min=0.10`。**没有这一半，上面那个 0 也可能来自别处**——这一格是断言不空转的证据。
3. **可复用判据**：`duplicateStockBusinessKeys`，断言它对唯一输入返回空、对重复输入**指名道姓**返回 `["annual/2022-12"]`（不是只报个数）。

注释写明了 DoD 要求的那句理由：「今天重复恰为 0 靠的是央行的发布习惯（2025-10 起社融并进月报、不再单发，两族时间互补），**不是代码**」。

---

## 3. functional[1] / boundary[1]：1a 假标签 —— 成因我独立求值确认

### 3.1 旧标签确实是假话

旧串断言的是**报告的性质**：「该期报告只有当月数、正文无任何期内累计口径的合计句」。

我对那篇真文章直接求值（测量树 `962c3ac`，语料 `data/hestia-backfill-2026-08-14`）：

```
[T] cumulativePeriods["今年前5个月"] = false      （cumulativePeriods 共 26 个键）
[T] 2022-05 金融统计数据报告：onlyCurrentMonthFlowSentences = true   ← 判据说「没有累计句」
[T] 正文里「今年前5个月」出现 2 次 ⇒ 累计句其实存在
[T]   原句: 今年前5个月，人民币贷款累计增加10.87万亿元，同比多增2326亿元…
[T]   原句: 今年前5个月，人民币存款累计增加13.99万亿元，同比多增3.8万亿元…
```

⇒ **正文里明明有两句累计口径的合计句**，而判据返回 `true`。旧标签是假话，**dev 的诊断完全属实**。

新标签只说到「**按现有期次前缀表**在正文里找不到任何期内累计口径的合计句」，并显式声明不区分两种成因、指向原始解析错误作分辨 —— 这是判据**能够支持**的最强断言。

### 3.2 dev 顺手订正了成因注释（DoD 没要求）

判据上方原写「『有没有累计句』是**报告本身的性质**」——该句不成立：`onlyCurrentMonthFlowSentences` 复用 `cumulativePeriods` 查表，答的是「**表认得的**累计句有没有」，是「报告 × 词表」的性质。dev 一并订正了。

**我认可这个处置且认为它是本任务最有价值的一处**：它是假标签的**成因**。只改结论不改理由，下一个人会照那句注释重新推出同样的假标签——而**结论正确的时候没有人会去复查理由**。该注释在 `calibrate.go`，在 `writes` 内，**不构成越界**。

### 3.3 boundary[1]：分类结果未变（按段落归属判，不按「行里含 2022-05」）

⚠️ 报告里「2022-05」出现两次、是**两篇不同的文章**，用「行里含该串」判归属是坏仪器。我按段落解析：

| 文章 | 改动前 | 改动后 |
|---|---|---|
| `2025092212552655029.html`（金融统计数据报告） | **本迭代不解析**（Unsupported） | **本迭代不解析** |
| `2025092212552660938.html`（社会融资规模增量统计数据报告） | **解析失败**（Failures） | **解析失败** |

⇒ **未换格，只有理由串变了。未碰解析器。**

---

## 4. functional[2] / error_handling[0]：19 是我自己重采的

我在验证 worktree 里编译 `962c3ac` 的二进制，自己跑了一次：

```
atlas hestia backfill calibrate --dir <corpus> --allow-incomplete
  待解析（受支持期次）: 199 篇
  本迭代不解析: 19 篇          ← 我的重采值
  解析失败（M1c-4 的兜底工作量）: 38 篇
  标题解析不出期次: 0 条
```

**得数 19，与结转项声称的 19 一致**，`error_handling[0]` 的前件（重采不是 19）未触发。

`calibrate.go` 的注释已由「真语料 **23** 篇」改为「本轮真语料上是 **19** 篇」，并补了 DoD 要求的那句：「⚠️ **这个数随语料变，改语料时必须重采**，不要沿用这里写下的值。重采方式是跑 calibrate 看『本迭代不解析』那一栏」。

> dev 用了三把尺（摘要行 / 明细区表头 / 摘要分组求和），并说明第三把走的是另一条渲染路径。我这一把是**第四把、来源独立**（自己编译、自己跑、读摘要行），与它们一致。

---

## 5. functional[3] / error_handling[1]：矛盾标签 —— 实测 9 处，DoD 列了 6 处

**我独立数了一遍**（按措辞 grep，不按行号）：

```
git grep -c 'M1c-3 入库前' 5f68f58^ -- internal/hestia/
  calibrate.go            : 3
  calibrate_report.go     : 2
  calibrate_report_test.go: 2
  calibrate_test.go       : 2
  ───────────────────────────
  合计                      9 行
```

⇒ **9 处**，与 dev 的实测一致；**DoD 只列了 6 处**（漏了 `calibrate_report.go:138` 与 `calibrate_report_test.go` 两处），dev 自己补齐并申报了。三处漏项**都在本任务的 `writes` 内**，不构成越界。

**DoD `error_handling[1]` 的可求值判据**：

```
grep -rn 'M1c-3 入库前' internal/hestia/  ⇒  命中 0 条
```

统一后的措辞「M1c-4 的兜底工作量」现有 **7 处** = 9 − 2，缺的正是下面那两处**刻意未照抄**的。

### 5.1 dev 的第 1 处自主判断：`FetchFailed` 两处不照抄归属 —— 我认可

`calibrate.go` 的 `FetchFailed` 字段注释与它在 `calibrate_test.go:690` 的镜像，讲的是 **fetch 阶段没抓到**的篇目。dev 没有机械替换成「M1c-4 的兜底工作量」，而是改成「让该修的东西可见」并补了一句「⚠️ 这一格**不归 M1c-4 兜底**：没抓到的文章连正文都没有，LLM 补不了，要补的是 fetch」。

**这是对的**：裁决要统一的是**同一批 38 篇解析失败**的归属，`FetchFailed` 是另一格、另一种补救路径。机械替换会造出一句**新的假话**，而本任务修的正是这类毛病。

⚠️ 特别值得记的是 dev 自己指出了这一点：**`grep` 判据在两种改法下都是 0 命中，它分辨不出这个区别**。这是对**判据射程**的主动披露——判据满足不等于做对了，而做错的那一版同样满足判据。

---

## 6. boundary[0]：行为不变 —— 背对背复现

隔离到两棵独立 worktree：BASE = `57a9cad`（本任务 merge 之前的 master，已含 TASK-005/011）、POST = `962c3ac`。交错 **POST / BASE / BASE / POST**：

| 轮次 | 顶层 `--- PASS` | 顶层 `--- FAIL` |
|---|---|---|
| POST r1 | **579** | 0 |
| BASE r1 | **578** | 0 |
| BASE r2 | **578** | 0 |
| POST r2 | **579** | 0 |

**差值恰为 1**，与 R9 新增的顶层测试函数数相等。

**不止比计数，还比集合**（剥掉耗时只比名字，`LC_ALL=C sort` 后 `comm`）：

```
只在 POST 出现: TestStockContinuityRatesAssumeUniqueBusinessKey   （1 条）
只在 BASE 出现: （0 条）
```

⇒ 新增的**恰是** R9 那一个，没有任何用例消失或改名。**boundary[0] 满足。**

> ⚠️ dev 记录的坑我复现时也会遇到：`--- PASS` 行**含耗时**，`(0.01s)` vs `(0.00s)` 的抖动会让无关用例被列进 diff（dev 第一次跑出 14 个假差异）。我从一开始就 `sed 's/ (.*//'` 剥掉耗时。这与我在 TASK-009 撞到的「默认 locale 对 CJK 排序不稳定」同族：**都是工具的性质被误当成被测对象的性质**。

### 6.1 报告输出确实变了 62 行 —— 四类归尽、零杂项

本任务与 TASK-001「逐字节不变」**方向相反**：统一标签与订正假标签本来就是在改印给人看的字。所以该验的是**计数不变 + 变化行可归类**。

**先证仪器确定性**（否则 62 这个数不可信）：

```
base 跑两次逐字节一致 = true
post 跑两次逐字节一致 = true
```

**再比对**：

```
报告行数 base=186  post=186        （无行增删）
diff 变化行数 = 62 = 31 旧 + 31 新
```

| 方向 | 分类 | 条数 |
|---|---|---|
| 旧（`<`） | 含「该期报告只有当月数」（旧 Unsupported 标签） | **30** |
| 旧（`<`） | 含「M1c-3 入库前要清零」（旧明细区标题） | **1** |
| 旧（`<`） | **其余（杂项）** | **0** |
| 新（`>`） | 含「按现有期次前缀表」（新 Unsupported 标签） | **30** |
| 新（`>`） | 含「M1c-4 的兜底工作量）（38 篇」（新明细区标题） | **1** |
| 新（`>`） | **其余（杂项）** | **0** |

**四格计数前后完全一致**：待解析 **199** / 本迭代不解析 **19** / 解析失败 **38** / 标题解析不出期次 **0**。

⇒ 62 行**全部**是这两处措辞的替换，**零杂项**。dev 的归类逐条复现。

---

## 7. dev 的第 3 处自主判断：`duplicateStockBusinessKeys` 只看带 `tsf_stock` 的记录

dev 声称对全体 `Records` 求重复会得 42 组、拿它当判据会让断言从第一天起就红。我实测（测量树 `962c3ac`，真语料）：

```
[D] Records 总条数 = 161
[D] 对【全体 Records】求重复业务键 = 42 组      （dev 声称 42 ✓）
[D] 带 tsf_stock 的记录数 = 79                  （dev 声称 79 ✓）
[D] duplicateStockBusinessKeys(全体 Records) = []  ⇒ 加了筛子后唯一性成立
```

⇒ **两个数逐一对上**。不加筛子的话断言确实会从第一天起就红，然后被人调松或删掉。`stockContinuityRates` 的分组前置条件本来就是这个筛子，**限制作用域是正确的**，不是把断言写弱。

---

## 8. 门禁与交付流程

| 项 | 结果 | 测量树 |
|---|---|---|
| `go test ./internal/hestia/... -count=1` | `ok`，FAIL 0 | `962c3ac` |
| `go test ./cmd/... -count=1` | `ok`（报告输出变了，特意确认无 cmd 测试断言旧串） | `962c3ac` |
| `gofmt -l internal/hestia cmd/atlas` | 仅 `cmd/atlas/backtest_test.go`、`cmd/atlas/crisis_test.go`（**零新增项**） | `962c3ac` |
| `go vet ./internal/hestia/... ./cmd/...` | **0 行输出** | `962c3ac` |
| 覆盖率 | **96.0%** @ `962c3acb29705b58e21aaf4d4d64bcf8c77aca3a`（背对背对照 `2433e55` = 95.9%） | 两树 |
| 不低于基线 95.9% | 满足（**高于**）。⚠️ 高出的部分**不来自本任务**——同批合入的 TASK-005/011 改变了整棵树的分母，dev 已把 sha 与数字同行写进 discovery | — |
| AD-4：merge 早于 `dev_done` | merge `962c3ac` @ `2026-09-01T01:36:54Z` < `dev_done` @ `01:40:16Z`（**早 3 分 22 秒**） | — |
| AD-4：commit message 锚定 | `refactor(TASK-008): calibrate 族结转项四条……` ✔ | — |
| AD-4：自拆 worktree | `git worktree list` 中无 `wt-TASK-008-m1c3b` | — |

---

## 9. INFO（不构成缺陷）

**INFO-1｜commit message 与 discovery 的 PASS 数不同，是树不同、不是矛盾。**
commit message 写「顶层 PASS 565→566」（写于 `5f68f58`，其父是 TASK-005/011 合入之前的树）；discovery 写「578→579」（采于合并后的主线）。**两处差值都是 1**，我复现的是后者。**不是数字打架，是两棵树。**

**INFO-2｜R9 断言的性质：它钉住的是前提，不是保证。**
dev 在 `interfaces_exposed` 明写「`stockContinuityRates` 的输入唯一性**仍然只是断言，不是代码保证**」，并点出过渡月双发、或 M1c-4 修好 2026-01 那类失败都会让它变成非 0。这个披露是准确的，下游填 `magnitude_ranges` 的人需要知道。

---

## 10. 结论

十条 done_criteria **逐条 PASS**，无失败用例、无越界、无新增依赖。

关键数字**全部由我独立重采**而非采信：19 是我自己编译二进制跑 calibrate 得到的（第四把独立的尺）；9 处矛盾标签是我自己 `git grep` 数的（证实 dev 对、DoD 漏三处）；62 行变化经**先证仪器确定性**后归类为四类零杂项；578→579 是背对背交错四轮 + 名字集合 `comm` 比对。dev 的三处自主判断（`FetchFailed` 不照抄、订正成因注释、`duplicateStockBusinessKeys` 限作用域）我逐条求值确认成立，**其中第一处 dev 自己指出「grep 判据分辨不出这个区别」，是对判据射程的主动披露**。

**判定：VERIFIED。**
