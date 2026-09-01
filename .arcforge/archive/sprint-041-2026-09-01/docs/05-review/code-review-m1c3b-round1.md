# M1c-3b · 第一轮 Code Review（常规）

- 审查者：qa-m1c3b
- 时间：2026-09-01
- 对象：`master @ 21815beda194398b1ee9b0777e969f4f102b8ac3`，基线 `32bc1e5f306386ee5c69a54b4bae3e0184aa30f2`
- 范围：`git diff 32bc1e5..21815be` = 23 文件 / **+3156 / −90**
- 心智模型：Reality Checker（默认 NEEDS WORK，每条 PASS 必须附证据）

---

## 0. 我自己跑出来的基础事实（不采信任何转述）

| 项 | 命令 | 结果 |
|---|---|---|
| 构建 | `go build ./...` | rc=0，零输出 |
| hestia 测试 | `go test ./internal/hestia/ -count=1 -cover` | rc=0，**96.2%** |
| cmd 测试 | `go test ./cmd/atlas/ -count=1 -cover` | rc=0，**75.7%** |
| vet | `go vet ./internal/hestia/ ./cmd/atlas/` | rc=0，零输出 |
| gofmt | `gofmt -l .`（剔除 `.worktrees/`） | `internal/hestia/` **零命中**；`cmd/atlas/` 仅 `backtest_test.go` + `crisis_test.go` |
| 生产库未被动 | `shasum -a 256 …/runtime/atlas/data/hestia.db` | `478d40c079c8b0eab7d089bb6f1926725b361a6dc6c850f4c4a651406f3ec28c` —— **与 TASK-010 判据逐字符相等** |

⇒ `gofmt` 判据（「两个既有欠账之外无新增项」）**成立**。覆盖率与 Leader 报的尺 A 一致。

### 真语料端到端复跑（我独立跑的第二次，非引用 TASK-010）

```
/tmp/qa-atlas hestia backfill load --dir <主仓库绝对路径>/data/hestia-backfill-2026-08-14 \
  --db <scratchpad>/qa-hestia.db --allow-incomplete   →  exit=0
```

| 判据 | 期望 | 我实测 |
|---|---|---|
| 恒等式一 | 218 = 199 + 19 | ✓ |
| 恒等式二 | 199 = 161 + 38 | ✓ |
| 恒等式三 | 96 = 54 + 42 | ✓ |
| 恒等式四 | 96 = 75 + 21 | ✓ |
| `merged@v1` 条数 | 42 | ✓ **42**（权威表 28 + pending 14 —— 单查一张表得 28，即 Leader 已知的 DoD 缺陷 #4） |
| `tsf_stock` 非空 | 79 | ✓ **79**（权威表 60 + pending 19） |
| 字段冲突 | 0 | ✓ 0 |
| 部分覆盖节 | 非空 | ✓ 64 条，分布 42/13/3/2/2/2（自洽校验：和 = 64 = 节标题声称值） |

**头号验收数字全部复现。** 本轮的问题不在这些数字上。

---

## 1. CRITICAL

### C-1 · `stock_continuity` 把「跨多期的跳变」按「相邻一期」的阈值判，在本次真跑里造出 4 个假拒绝

**文件**：`internal/hestia/validate.go` 的 `gateStockContinuity`；`internal/hestia/store.go` 的 `Preceding`；
`internal/hestia/thresholds.go` 的 `defaultStockContinuityMax`（本 sprint TASK-005 新写）

**观察（实测，非推理）**

`Preceding` 的 SQL 逐字是：

```sql
SELECT … FROM v_hestia_current WHERE period < ? AND period_type = ? ORDER BY period DESC LIMIT ?
```

**没有任何相邻性约束** —— 它返回的是「最近一个**已被接受**的同类期次」，不是「上一期」。
而 `gateStockContinuity` 里的注释写着：

```go
// prior 按 period 降序，[0] 就是上一期。
```

**这句话只在没有任何一期被拒时才成立。** 该函数的四条 skip 理由（`no_threshold` /
`need(FieldTSFStock)` / `no_prior_period` / `prior_absent_field` / `zero_denominator`）里
**没有一条**是「prior 与本期不相邻」。

我这次真跑的 21 条 pending 里，**4 条是 `stock_continuity`，全部是跨期伪影**：

| 期次 | 报告原文 | 真实相邻期增幅 |
|---|---|---|
| 2025-12/annual | `moved 0.2844 from 344.21 to 442.12, exceeds 0.2000` | **跨 3 年**（2022-12 → 2025-12），逐年实为 **9.8% / 8.0% / 8.3%**，条条远在 0.20 以内 |
| 2026-04/monthly | `moved 0.0620 from 430.22 …, exceeds 0.0500` | 基线是 **2025-06**（430.22），跨 **10 个月** |
| 2026-05/monthly | `moved 0.0665 from 430.22 …` | 同一基线，跨 **11 个月** |
| 2026-07/monthly | `moved 0.0768 from 430.22 …` | 同一基线，跨 **13 个月** |

链路证据（我从库里直接查的）：

```
annual  链：2022-12 OBS 344.21 │ 2023-12 PEND │ 2024-12 PEND │ 2025-12 PEND
monthly 链：2025-06 OBS 430.22 │ 2025-10/11/2026-02 PEND │ 2026-04/05/07 PEND
```

⇒ **`deposit_sum` 拒掉的那几期，把后面每一期的比较窗口拉长了，而阈值仍按一期的长度判。
一次拒绝会自我维持地拒掉后面全部同类期次。**

**为什么这是本 sprint 的问题，而不是「既有缺陷」**

1. `defaultStockContinuityMax` 的整段出处注释是**本 sprint 新写的**（TASK-005），
   它明写「相邻两期相隔 1 个月」「相邻两期一律相隔 12 个月」，
   并用 `stockContinuityRates` 的**相邻对**分布（monthly n=68 max=0.02613 / annual n=6 max=0.13338）标定。
   **标定用的总体与闸门实际比较的总体不是同一个**——这正是归档教训「数字真、却被搬到测不出它的条件下」。
2. `BackfillLoad` 是**本 sprint 新造的**、第一个「一次灌完整段历史、且拿正在建的库当 history」的路径，
   级联只有在这里才成规模出现。
3. 后果不可自愈：DoD 反复写「`--force` 对已入权威表的期次是数据层 no-op ⇒ **拦错了没有出路**」，
   而这里是**拦错了**；文档给的补救（删库重跑）是**确定性的**，重跑逐字复现同样 4 条。
4. 报告把它呈现成数据异常（`tsf_stock moved 0.2844 …`），运维会去查央行数据，
   **而真相在闸门里**。

**建议**（按投入排序，前两条即可闭合）

- `gateStockContinuity` 增一条 skip/降级：`prior[0]` 与本期**不相邻**时，记
  `CheckSkipped{Reason: "prior_not_adjacent:<prior period>→<本期>, gap=<N>"}`，
  或按实际跨度换算上限（`limit × gap` 是最省事的近似，但要写明它是近似）。
  **不要静默按原阈值判** —— 那正是现在的行为。
- 同时订正 `gateStockContinuity` 里「`prior[0]` 就是上一期」那句注释：它是本缺陷的认知源头。
- 报告的 pending 节对这类判因加一句「基线期次」，让「跨了几期」在报告上可见。

---

## 2. WARNING

### W-1 · 「部分覆盖的期次」用**来源族**代替**字段残缺**，已在本次真跑中产出 3 个假阳；对 2025-10 之后的数据将是 100% 假阳

**文件**：`internal/hestia/backfill_load.go` 的 `missingFamilies` / `extractorFamilies`；
`backfill_load_report.go:74-84`

**观察（实测 + 结构双重确认）**

真跑的 64 条「部分覆盖」里，有 **3 条的 54 个业务列全部非空**——即字段**一个都不缺**：

```
2025-09/q1_q3  rule@v2  54/54 字段非空   报告写「缺: 社融存量、社融增量」
2026-03/q1     rule@v2  54/54 字段非空   报告写「缺: 社融存量、社融增量」
2026-06/h1     rule@v2  54/54 字段非空   报告写「缺: 社融存量、社融增量」
```

结构上也必然如此：`required.go` 里 `case extractorV2: return slices.Clone(fieldOrder)`
——`rule@v2` 的必填集**就是全部 54 个字段**，它们能进权威表本身就意味着
`gateCompleteness` 刚刚验过这 54 个字段齐全。**报告说缺的那两族数据，闸门上一秒才确认它在。**

**为什么会更糟**：本 sprint 的全部前提是「2025-10 之前央行分三篇发，之后自己并篇」。
并篇后的报告就是 `rule@v2` / `rule-monthly@v2` 这种自带社融板块的单篇 ⇒
`Parts` 恒为 `[月报族]` ⇒ **今后每一个完整期次都会被列进「部分覆盖」**。
这一节是人类在 dod-gate 裁决 A-3 时点名要求的「让 44% 残缺出声」，
**一个对完整数据也报警的告警等于没有告警**。

**建议**：判据换成**字段**而不是**来源**——用「本组非空字段集 ⊊ 该期应有的完整集」
（或直接复用 `mergedRequiredFields` 的并集与全集比），并把**缺哪些字段**打出来
（那才是运维要的）。来源族可以留作补充说明，但不能当判据。

### W-2 · `TestMergedPartsDoNotRoundTrip` 不守它 DoD 声称守的性质（第 8 处 DoD 缺陷）

**文件**：`internal/hestia/validate_test.go` 的 `TestMergedPartsDoNotRoundTrip`；
`store.go` 的 `scanObservation`

DoD（TASK-011 functional[3]）原话：「把「不入库」从**注释**变成**断言**」。
实际这条断言只观测**读**路径。

**观察（隔离 worktree 变异实测，主工作区零改动）**：
给 `observationsDDL()` 加一列 `parts TEXT`、给 `insertSQL` 加
`cols=append(cols,"parts"); args=append(args, strings.Join(obs.Parts,","))`
⇒ `Parts` **真的写进了库**，而

```
go test ./internal/hestia/ -run TestMergedPartsDoNotRoundTrip -count=1  →  rc=0  PASS
```

静态佐证：`grep -c Parts internal/hestia/store.go` = **0**，`schema.go` = **0**
⇒ 该断言的取值只由 `scanObservation` 决定，而它一个字都没提 `Parts`。

真正杀死这个变异的是**另外四条既有测试**：`TestObservationsColumnsDeriveFromFieldOrder` /
`TestCurrentViewStructureFromLiveDB` / `TestInsertSQLColumnOrderIsDeterministic` /
`TestInsertSQLOmitsAbsentFields` —— **DoD 一条都没点名**。
即本 sprint 自己归纳的「全绿假信号形态一：为错误的理由变绿」。

**结论仍然成立**（`Parts` 确实不入库，且**确实**被机制守着），错的是**归属**。
**建议**：补一条真正打在写路径上的断言，例如
`sql, _ := insertSQL(obs); assert.NotContains(t, sql, "parts")`，并在注释里点名
上面那四条才是列清单的守卫。

### W-3 · `thresholds.go` 的注释贴在它自己反对的那个循环上（与需求文档已知缺陷④同形，在交付代码里复发）

**文件**：`internal/hestia/thresholds.go`，`for name := range t.MagnitudeRanges` 上方

```go
// 遍历 fieldOrder 而不是 map：map 迭代顺序随机，同一份坏配置两次跑
// 报出不同的那一项，会让排查变成猜谜（与 gateMagnitudeSanity 同理）。
for name := range t.MagnitudeRanges {     // ←←← 这一行就在 range map
```

TASK-004 的 DoD boundary[1] **明确写了**：「「未知键」那一轮**只能** range map……
那一轮的顺序不确定性是可接受的，因为它一旦命中就是配置错误、必须修」。
**实现是对的，DoD 是对的，注释把那条限定条件丢了并说了反话。**
现状：表里有两个错字段名时，报哪一个是随机的，而注释向读者保证它是确定的。
既有测试 `TestMagnitudeRangesRejectUnknownField` 只放**一个**未知键 ⇒ 这条不确定性无守卫、也无人会发现。

**建议**：把注释改成 DoD 里那句原话（「这一轮只能 range map，其不确定性可接受，因为……」）。
若希望确定，`sort.Strings` 一下未知键再报第一个，两行的事。

### W-4 · 字段冲突只打印一行，不影响退出码、不阻断入库，而注释自陈「必须响亮失败」

**文件**：`internal/hestia/backfill_load.go:112-124`；`cmd/atlas/hestia.go` 的 `runHestiaBackfillLoad`

代码注释：「一旦出现就说明字段归属表错了——**那是必须响亮失败的事**，而「取第一个」会让一张错的归属表永远不被发现」。
实际行为：记进 `Conflicts`、**`vals[f]` 保留第一个值**、照常 `Validate` + `Save`、
报告里出一行、`runHestiaBackfillLoad` 丢弃 `res` 只返回 `err` ⇒ **exit 0**。
即：既做了它反对的「取第一个」，又没有「响亮失败」。运维按退出码自动化时冲突完全不可见。

本次真跑冲突为 0，所以**没有实际损害**——但这条路径至今**从未在真语料上执行过**，
它的正确性完全依赖那句「三个 extractor 字段集设计上不相交」的前提。

**建议**：二选一并把注释改到与行为一致——
(a) 冲突非空 ⇒ `BackfillLoad` 返回非 nil error（与四道恒等式同级，因为成因同样是「表错了」）；或
(b) 保留现状但把注释改成「记录并继续，退出码不受影响」，并在报告该节加一句显眼的后果说明。
我推荐 (a)：注释给的理由（错的归属表永远不被发现）只有 (a) 能兑现。

### W-5 · `CheckSkipped` 在**过闸的**观测上端到端不可观测——第 7 处 DoD 缺陷的底层成因，本 sprint 未修

**文件**：`internal/hestia/schema.go` 的 `metaColumns`；`store.go` 的 `insertSQL` vs `savePending`

`hestia_observations` 只有七个 meta 列 + 54 个业务列，**不存 `ValidationReport`**；
完整报告只在 `savePending` 里落成 JSON。⇒ **一条观测只要过了闸，它的 skipped 明细就永久消失**。

这正是 Leader 已发现的第 7 处 DoD 缺陷（「真跑时 `unknown_extractor:merged@v1` 条数为 0」恒真）的**机制根源**：
那 42 条合并观测全部过闸进权威表，它们的 skipped 理由**结构上没有任何落点**，
所以无论闸门有没有真的执行，那个字符串的条数都是 0。

我复跑确认：整份报告里 `unknown_extractor` 出现 **0** 次——**改与不改都是 0**。

**建议**（超出本 sprint 范围，建议写进 CONTRACTS 的 M1c-4 清单）：
给权威表加一个 `skipped_checks TEXT` 列，或至少让 `writeLoadReport` 汇总一节
「过闸但有 skipped 的期次及其理由」——那一节从 `res.Groups` 就能算，不需要改 schema。
**后者半天就能做，且直接把那条恒真判据变成可求值的。**

---

## 3. SUGGESTION

### S-1 · 报告里「合并组」一词在相隔 6 行处指两个不同的集合

```
  合并后观测:      96   （单篇 54 + 合并组 42）      ← 合并组 = 42
  合并组明细（96 组）                                 ← 合并组 = 96
```
`res.Groups` 是全部 96 组（含单篇），节标题却沿用「合并组」。
在一份**专供人工核账**的报告里，同一个词相邻两处取两个值是要命的。
建议节标题改「全部观测明细（96 组，其中合并组 42）」。

### S-2 · 恒等式失败 ⇒ 整份报告不输出，而报告是 `DroppedIDs` 的**唯一载体**

`writeLoadReport` 先校验后打印（理由充分）。但 `Out != nil` 那条契约的全部论证正是
「报告还是 `DroppedIDs` 的唯一载体」。恒等式一旦不成立，同一个东西照样丢失。
另：恒等式四（`Merged = ToObservations + ToPending`）在**任一期 Validate/Save 出错**时必然不成立，
而它的错误串**不点名这个最常见成因**（恒等式一点名了 `Unclassified`，四没有）。
建议：四的错误串补一句「若本次有单期校验/入库失败，差额通常就是它们」；
并考虑把「丢弃明细」在恒等式失败时也照常打出来（它与账对不对无关）。

### S-3 · TASK-008 boundary[0] 的判据用错了仪器

DoD 写「`grep -c '^--- PASS'` 的差值**恰等于 R9 新增的断言数**」。
`--- PASS` 数的是测试函数/子测试，**不是断言**。验证者在报告里静默换成了
「与 R9 新增的**顶层测试函数数**相等」才使它成立（`TASK-008-verification.md:158`）。
若 R9 当初写成往既有测试里加断言，差值恒 0 ⇒ 判据不可满足。
这是归档教训「拿高度相关的可观测量代替性质本身」的又一次，且**这次出在 DoD 里、不在执行里**。
建议：这类判据一律写成「顶层测试函数数」或「`go test -v` 的 RUN 条数」，不要写「断言数」。

### S-4 · `store.Close()` 的错误被 `defer` 丢弃

`BackfillLoad` 里 `defer store.Close()`。sqlite 驱动关闭时的错误（如未 flush）会被静默吞掉。
本包其它地方对 `rows.Close()` 用 `_ =` 显式表态，这里连表态都没有。
考虑到本函数的产出是「一个新建的、要长期当权威库用的文件」，建议至少
`defer func(){ if err := store.Close(); err != nil { errs = append(errs, err) } }()`
（需把 `errs` 提到 defer 可见处）。

---

## 4. 我查了但**没有**发现问题的（避免第二轮重复劳动）

- **恒等式二并非恒真**：我核了 `cal.Periods` 的两处增减（`calibrate.go` 循环首 `++`、
  Unsupported 分流处 `--`）。它确实与 `ParsedOK+ParseFailed` 走不同的记账路径，
  去掉那个 `--` 会让恒等式一和二**同时**报错。代码注释的那段论证成立。
- **`TestMagnitudeSanityReportsEarliestFieldInFieldOrder`（F12 守卫）质量很高**：
  用六个越界字段 × 十轮把变异存活概率压到 (1/6)^10，并用
  `require.Equal(t, FieldTSFStock, fieldOrder[0])` 自守前提。无可挑剔。
- **SQL 注入**：`insertSQL` 的列名全部来自常量 `metaColumns` / `fieldOrder`，值走 `?` 占位符；
  表名是常量。`Preceding` 同理。无注入面。
- **cmd 分层**：`cmd/atlas/hestia.go` 未 import `path/filepath`，`--db` 存在性检查确实在
  `BackfillLoad` 里且**先于** `NewStore`。守卫遵守。
- **`--db` 无默认值**、`MarkFlagRequired("dir","db")`：与 DoD 一致，且理由（默认值会写进生产库）成立。
- **生产库未被改动**：sha256 与 TASK-010 判据逐字符相等（见 §0）。

---

## 5. 第一轮结论

**NEEDS WORK** —— 1 条 CRITICAL（C-1）、5 条 WARNING。

需要强调的是：本 sprint 的**工程质量本身是高的**——四道恒等式、42/107/96、
`tsf_stock`=79、冲突=0、覆盖率、gofmt、生产库指纹，我逐条独立复跑，**全部成立**。
C-1 与 W-1 都不是「代码写错了」，而是**两个判据各自用一个高度相关的可观测量
代替了它真正要判的性质**（来源族 ≈ 字段残缺；最近接受期 ≈ 上一期），
近似失效时不报错、只给一个像样的数——与本 sprint 自己反复归纳的失效模式完全同族。
