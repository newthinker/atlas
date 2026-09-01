# M1c-3b · 返工复审（第三轮）

- 审查者：qa-m1c3b
- 时间：2026-09-01
- 对象：`master @ 70eedc4caf3502df9f25f125c7dcc546d06320d3`（上轮审的是 `21815be`）
- 增量：`git diff 21815be..HEAD` = 8 文件 / **+904 / −39**
- **verdict：PASS**（0 CRITICAL / 2 WARNING）

---

## 0. 基础事实（全部我自己跑，不采信转述）

| 项 | 结果 |
|---|---|
| `go build ./...` | rc=0 |
| `go test ./internal/hestia/ -cover` | rc=0，**96.1%** |
| `go test ./cmd/atlas/ -cover` | rc=0，**75.7%** |
| `go vet` | 零输出 |
| `gofmt -l internal/hestia cmd/atlas` | **恰两个既有欠账**，判据成立 |
| 生产库 sha256 | `478d40c0…c28c`，与 sprint 起始逐字符相等 |
| validator | rc=0，12 任务通过 |
| 真跑 | exit=0，四道恒等式成立，**入权威表 79 / 落 pending 17 / stock_continuity 拒绝 0** |
| 头号数未回归 | `merged@v1` 跨两表 = **42**、`tsf_stock` 非空跨两表 = **79**、单篇 54 + 合并组 42 |
| 17 条 pending 判因 | **全部 deposit_sum**（自洽校验 17=17 ✓），无一条 continuity |

---

## 1. 四条 CRITICAL 的闭合核验

方法：隔离 worktree（`git worktree add --detach 70eedc4`）逐条变异，**并检查杀死它的是不是对症的断言**
（五问#3：外溢的 KILLED 不算闭合）。收尾主工作区三个文件指纹 `diff` 逐字节一致，worktree 已 remove+prune。

| 变异 | 结果 | 杀死它的 | 对症？ |
|---|---|---|---|
| **C-1** 去掉相邻性检查 | KILLED（6 条红） | `TestStockContinuitySkipsNonAdjacentPrior`（4 子测试，含「跨 13 个月」「跨 3 年」两个真跑形态）+ `TestStockContinuityDoesNotUseRejectedPeriodAsBaseline` | ✅ **对症** |
| **C-2** 早退分支去掉 `renderLoadReport` | KILLED（1 条红） | `TestBackfillLoadFailsLoudlyOnUnclassified` | ✅ **对症** |
| **C-3** 去掉 `NewStore` 前的输入侧恒等式检查 | KILLED（1 条红） | `TestBackfillLoadDoesNotCreateDBWhenInputIdentitiesFail` | ✅ **对症** |
| **C-4** `len(g.SourceIDs) > 1` → `>= 1`（M17） | KILLED（5 条红） | `TestLoadIdentityThreeIsCrossSourced` + 4 条外溢 | ✅ **对症** |

⚠️ **我第一次的 C-2 变异打错了地方**：我改的是 `writeLoadReport` 内部那两行的顺序，
只红 1 条且是 `TestWriteLoadReportPropagatesWriteError`。查了才发现 **C-2 的头号性质
（Unclassified 标题可见）根本不走 `writeLoadReport`** —— 它走 `BackfillLoad` 的早退分支
直接调 `renderLoadReport`。改打那个分支（C2a）才是对症的，结果见上表。
**这条记下来是因为它差点让我判「C-2 只有外溢守卫」。**

### 端到端复核（用我上轮那份暴露缺陷的同一份语料）

`<scratchpad>/qa-corpus-unclass`（真语料副本，一篇标题改成「央行发布最新金融数据情况通报」）：

| | 上轮（`21815be`） | 本轮（`70eedc4`） |
|---|---|---|
| stdout 字节数 | **0** | **1119** |
| 标题原文可见 | ✗ | ✅ 第 17 行 |
| 失败后是否建库 | **90112 字节 / obs=75 / pend=20** | ✅ **未建库** |
| 错误串是否说明副作用 | 未提 | ✅「**尚未建库、尚未写入任何数据**」 |

**C-2 与 C-3 在生产路径上真闭合了，不是只在单测里闭合。**

### 两条我特别核过的实现细节

- **C-3 的判据是「库文件不存在」而非「返回了 error」**：`assert.True(t, os.IsNotExist(statErr))`，
  且代码里有一行注释点名「上面那个 `require.Error` 在缺陷版本上同样成立」。
  **验证者那条约束真的落进了实现**，不是写在报告里。
- **C-4 的替代判据是真异源**：`assert.Equal(t, res.MergedGroups, res.DBMergedRows)`（跨两表），
  并同时钉了具体值（`MergedGroups==0` / `SingleArticle==2` / `Merged==2`）。
  我上轮建议的两条都实现了。

### W-1 也复核了，是真闭合

上轮那 **3 个 54/54 字段却被列进「部分覆盖」的假阳，现在一个都没有**；
未被列出的 18 行字段数恰为 {52, 54}。报告改成逐条列出**缺哪些字段**，缺族降为括号里的补充说明。

⚠️ 我核这条时先得到一个「无 ✓」的结论，**那是假的**：正则没跟上新版式（多了 `@published_at`），
`partial` 集合为空 ⇒ 判断恒不触发 ⇒ 打印「无」。加了 `assert hdr == len(partial)` 才炸出来
（61 vs 解析出 59，差的两条是右对齐的 `缺  9 个字段` 两个空格）。**修好后结论仍成立，但那第一次是零观测。**

另：报告说「缺 25」而库里实缺 27，差 2 —— 我查了，**是 `fx_reserve`/`fx_rate`**，
月报无外汇板块，属 absent-by-design。**报告的口径是对的，我那把「对 54 列数」的尺是错的。**

---

## 2. WARNING（2 条，均不阻断）

### W-11 · 末尾路径的「先渲染后校验」没有对症守卫

`writeLoadReport` 现在是「先 `renderLoadReport` 后 `checkLoadIdentities`」。
把这两行换回来（M-C2b），**只红 1 条：`TestWriteLoadReportPropagatesWriteError`**
—— 那条测试守的是**写失败要向上传播**，不是「账不平也要出报告」。它红是因为
identity 先失败就返回了 identity error、盖住了 write error，属**形态一「为错误的理由变红」**。

grep 确认：**没有任何测试断言「恒等式三/四不成立时 `out` 仍非空」**。

⇒ 行为是对的，守卫是蹭来的。若将来有人动 `TestWriteLoadReportPropagatesWriteError`
或写失败语义，这个顺序就完全无守卫了。
**建议（一条断言）**：造一个恒等式四不成立的 `res`，断言 `out.String()` 非空且含「合并组明细」。

### W-12 · 那道验收闸的措辞没有按 Leader 自己的订正更新，而它现在写在 `done_criteria` 里

`TASK-006.done_criteria.non_functional[2]` 当前原文仍是：

> 每一条**新补或修改的判据**，验证者必须先证明它能红……转不红即说明补的**仍是**空判据

Leader 已经识别出它的射程缺陷（**预设了 dev 补了判据**），并给出了正确表述
（「每一条 fix_item 修复的**行为**，都必须有一条能红的守卫；没有守卫的修复不算完成」），
**但订正只停在消息里，没有进载体。**

这与 Leader 自己认领的第 9 处（`fix_items[1]` 要求了行为、没要求守卫）是**同一形状**，
而且这次那句有缺陷的措辞**正躺在下一个验证者要照着做的地方**。

⚠️ **机制提醒**：`TASK-006` 现在是 `verified`，owner_table 里该状态的合法写者是 `test-*`
——**Leader 和我都改不了它，只有 `test-m1c3b-b` 能。** 若决定订正，请走它；
若判断本 sprint 不必改，那么**在结转进 M1c-4 时必须用订正后的措辞**，别把这句原样抄过去。

---

## 3. 回答 Leader 的四个问题

### Q1 四条 CRITICAL 是否真闭合 —— **是**，见 §1，变异 + 端到端双证，四条杀手全部对症。

### Q2 返工有没有引入新缺陷 —— **没有引入缺陷，但有一处守卫是蹭来的**（W-11）

- 全量测试绿、vet 零输出、gofmt 判据成立、覆盖率 96.2%→96.1%（+904 行下掉 0.1pp，正常）。
- 三个头号数（42 / 79 / 54+42）无回归。
- 关于「`renderLoadReport` 的拆分是否让『先渲染』成为结构性质」：
  **验证者说的「部分是」我复现并同意，而且可以说得更准** ——
  拆分让**早退路径**的先渲染成为结构性质（C2a 对症红）；
  **末尾路径不是**，那两行顺序可换回，且换回后只红一条不对症的测试。
  ⇒ 你写进 merge commit 的那句乐观表述，**在末尾路径上确实不成立**，你的更正是对的。

### Q3 D-8 总纲（守恒判据对错投免疫）有没有被真正落实 —— **不够，但已有一次正确的实例化**

- **已落实的**：恒等式三换成与库里 `merged@v1` 行数的**真异源**比对。
  这正是总纲要的形状 —— 守恒判据（三）配了一条**不同产地**的交叉校验。**这是模板，不是特例。**
- **没落实的**：「每设一条守恒判据必须配一条分流正确性判据」作为 M1c-4 的**标准要求**，
  目前只在 final-report 素材里，是散文。
- **我的判断：本 sprint 不必为此再开一轮。** 理由是它要约束的是**下一个 sprint 的拆分动作**，
  在本 sprint 里没有可执行的落点。但**结转时它必须是一行可照抄的 DoD 模板句，不是一段道理**：

  > 【分流正确性】凡本任务产出「N = A + B」形式的守恒判据，必须同时给出一条判据，
  > 说明**落在 B 这一侧的每一项都不是由本工具自身造成的**（逐条给出判因 + 排除工具成因）。

  ⚠️ 这一条能不能成立，不是现在可验证的 —— 我只能说措辞已可执行，**能否被执行要到 M1c-4 才知道**。

### Q4 有没有第 10 处 DoD 缺陷 —— **有 1 处，就是 W-12**；其余新判据我逐条过了判别式，**没有第 11 处**

按上轮做法把判别式（X 与 P 的产地）跑一遍**本轮新增/修改**的判据：

| 新判据 | X 的产地 | P 的产地 | 判定 |
|---|---|---|---|
| `TestLoadIdentityThreeIsCrossSourced` | `MergedGroups` 计数器 | DB 里 `merged@v1` 行数 | ✅ **真异源**（M17 只动计数器不动 extractor 改写，实测能抓） |
| `TestBackfillLoadDoesNotCreateDBWhenInputIdentitiesFail` | `os.Stat` 的库文件 | 「没产生不可逆副作用」 | ✅ 对症，且刻意避开了 `require.Error` 这条空判据 |
| `TestStockContinuitySkipsNonAdjacentPrior` | 闸门 Reason | 相邻性 | ✅ 对症，4 个子测试含两个真跑形态 |
| `TestBackfillLoadFailsLoudlyOnUnclassified` | `out.String()` 含标题原文 | 「运维看得见标题」 | ✅ 从「只看 err」升级成「看 out」，Architect 的原指控已闭合 |
| 验收闸（`done_criteria`） | —— | —— | 🔴 **射程不足 ⇒ W-12** |
| 「末尾路径先渲染」 | 无判据 | —— | 🟡 无守卫 ⇒ W-11 |

**0 处是结论，1 处也是结论 —— 这次是 1 处，不外推。**

---

## 4. 一条量化（不是新缺陷，是给 W-5 补上数字）

C-1 修好后我插桩实测（打点在 `non_adjacent_prior` 分支**内部**）：
**该分支命中 20 次**（18 monthly + 2 annual），与 dev 报的数一致。其中：

```
落在权威表（skipped 明细无处可查）: 11
落在 pending（report JSON 有存）  :  9      自洽 ✓
```

且 `non_adjacent_prior` 在核对报告里出现 **0 次**。

⇒ **11 个期次带着「连续性闸从未被评估」进了权威表，而任何地方都没说这件事。**

**这不是返工引入的，也不比修复前更糟** —— 修复前那 16 条是**拿一个无意义的基线评估后判了通过**，
比「没评估」更坏。C-1 是严格改进。

但它把我上轮的 **W-5**（过闸观测的 `CheckSkipped` 端到端不可观测）从「理论缺口」变成了
**11 行的实测面**，且 C-1 的修复**增加了 W-5 的分量**：它把一类「静默判错」转成了「静默未判」。

**建议（零 schema 成本，仍是我上轮给的那条）**：报告加一节
「**过闸但有 skipped 的期次及其理由**」，从 `res.Groups` 就能算。
这一节会把这 11 条和将来所有同类一次性变可见。建议**进 M1c-4，与 W-5 合并**。

---

## 5. verdict

**PASS** —— 0 CRITICAL。

四条 CRITICAL 全部闭合，且**杀手全部对症**（不是外溢 KILLED）；W-1 一并真闭合；
无回归；三个头号验收数未变。剩下 2 条 WARNING（W-11 守卫蹭来的、W-12 闸门措辞射程不足）
**都不阻断**，建议按上面各自的一行处方处理，或明确结转 M1c-4。

**不建议再开一轮 `review_fix`。** TASK-006 的 `rework_count` 已是 2、上限 3，
而剩下两条的量级（各一条断言 / 一句措辞）与再走一轮返工的代价不成比例。

⚠️ 但 **W-12 必须有明确去向**：它现在躺在下一个验证者要照着做的地方，
且只有 `test-m1c3b-b` 改得了。"结转 M1c-4 时用订正后的措辞" 是可接受的处置，
**"知道了" 不是** —— 那句话会被原样抄过去。
