# TASK-002 验证报告 — stripHTML 与 meta 提取

- **验证者**：test-agent-23（Reality Checker，默认 NEEDS WORK）
- **被验对象**：commit `acc5a96f452fa6c8f6a42a8f324600c82a7b73cd`（`internal/hestia/strip.go` 88 行 + `strip_test.go` 250 行，2 files changed / 338 insertions）
- **验证环境**：隔离 worktree `../wt-verify-TASK-002 @ acc5a96`（detached，验毕已移除）
- **assignment_epoch**：1（承接时记录，裁决携带 `--expect-epoch 1`）
- **判定：PASS → `verified`**

---

## 0. 判定依据摘要

| 判据 | 实测值 | 门槛 | 结论 |
|---|---|---|---|
| 任务子集测试 `-run 'TestStrip\|TestMetaContent'` | **23 `--- PASS`**，0 FAIL | 全绿 | PASS |
| 全包 `go test ./internal/hestia/ -count=1` | **146 `--- PASS`**，ok | 无回归 | PASS |
| `go vet ./internal/hestia/` | exit **0** | 绿 | PASS |
| `go test -race -count=2` | ok（3.024s） | 绿 | PASS |
| strip.go 函数覆盖率 | stripHTML / metaRE / metaContent **均 100.0%** | — | PASS |
| 包语句覆盖率 | **91.5%** | dev_minimum 80 | PASS |
| DoD 覆盖 | 6/6 条均有具体、非空洞断言 | 逐条 | PASS |
| 变异测试（dev 的 10 体，我独立复跑） | **10 KILLED / 0 SURVIVED / 0 INVALID** | — | PASS |

**本报告所有数字均由我本人在隔离 worktree 实跑得出，未采信 dev 报告的任何一个数。**

---

## 1. 完成标准覆盖矩阵

| # | 完成标准 | 对应测试 | 断言形态 | 判定 |
|---|---|---|---|---|
| functional[0] | `stripHTML` 剥离标签并归一字符（含 `html.UnescapeString`）；**块级与内联处理必须不同**：块级产生分隔、内联不得在数字中间插入空白 | `TestStripKeepsBlockBoundaries`（块级）<br>`TestStripRejoinsValueSplitByInlineTags`（内联）<br>`TestStripDropsScriptAndStyle`<br>`TestStripUnescapesEntities`<br>`TestStripUnescapesAfterTagRemoval` | 具体串 + 行数 `require.Len(lines,2)` + 逐行 `Equal` | **PASS** |
| functional[1] | `metaContent` 提取 content，第二返回值区分「不存在」与「存在但为空」 | `TestMetaContent`（真实样本 ×2，PubDate/ArticleTitle/createDate）<br>`TestMetaContentMissing`<br>`TestMetaContentPresentButEmpty`（SiteDomain 真实空值 ×2） | 精确 `Equal` 值 + `ok` 真假两侧均断言 | **PASS** |
| boundary[0] | **F6 回归钉**：`4825</span><span>亿元` 必须复原为 `4825亿元`，须用具体串断言 | `TestStripRejoinsValueSplitByInlineTags` | `Contains("同比多4825亿元")`、`Contains("；政府债券净融资13.84万亿元")` | **PASS** |
| error_handling[0] | 半角标点归一；2020 样本 2 逗号 1 分号，对三处具体串断言 | `TestStripNormalizesPunctuation`（合成）<br>`TestStripNormalizesPunctuationInRealSample`（真实 2020） | 三处具体串 `Contains` + `NotContains(",")`/`NotContains(";")` | **PASS** |
| non_functional[0] | 两函数为纯函数；判据「同输入连续两次结果相同」+ 代码审查 | `TestStripAndMetaAreDeterministic` + 代码审查 | `Equal(stripHTML(raw), stripHTML(raw))` ×2 样本 | **PASS** |
| non_functional[1] | 真实样本各跑一次，断言输出含原文锚点句，确保未吃掉正文 | `TestStripRealSamples`（锚点句 + `NotContains("<")`）<br>`TestStripRealSamplesKeepSectionAnchors`（板块 6/8） | 锚点串 + 精确计数 `Len(...,6/8)` | **PASS** |

**无「形式存在但断言空洞」的测试**：全部断言钉在具体字符串或精确计数上，无 `assert true` 类空转。所有 mock 为零——测试直接吃真实 `testdata/` 样本，不存在过度 mock 掩盖问题的风险。

---

## 2. 重点复核一：dev 自报的 harness 静默失效

dev 自报：第一版用 `git diff` 判定变异是否生效，而 `strip.go` 在其 worktree 里是**未跟踪文件**、`git diff` 恒为空 ⇒ 10 个变异体全被误报「未生效」；只看「无 SURVIVED」就会误判满分。

**复核结论：它最终采用的判定方式确实有效，那份「10 杀 0 存活」可信。**

我**没有复用它的脚本**，而是自写 harness（`mutate_verify.py`），判定生效与否用 Python `difflib` **直接比对源文件前后内容**，完全不经 git——从根上避开它踩的那个坑。独立跑出的结果与它逐体吻合：

| 变异 | 我的判定 | diff | `go vet` exit | PASS/基线 | 杀手测试 |
|---|---|---|---|---|---|
| M1 span 列入块级 | KILLED | 2 行 | **0** | 22/23 | TestStripRejoinsValueSplitByInlineTags |
| M2 内联标签换成换行 | KILLED | 2 行 | **0** | 22/23 | TestStripRejoinsValueSplitByInlineTags |
| M3 取消半角逗号归一 | KILLED | 1 行 | **0** | 21/23 | TestStripNormalizesPunctuation + …InRealSample |
| M4 取消半角分号归一 | KILLED | 1 行 | **0** | 21/23 | 同上两条 |
| M5 块级标签删除而非换行 | KILLED | 2 行 | **0** | 19/23 | TestStripKeepsBlockBoundaries + …KeepSectionAnchors |
| M6 meta 空值当作不存在 | KILLED | 2 行 | **0** | 20/23 | TestMetaContentPresentButEmpty |
| M7 不删 script | KILLED | 1 行 | **0** | 22/23 | TestStripDropsScriptAndStyle |
| M8 不做实体还原 | KILLED | 2 行 | **0** | 21/23 | TestStripUnescapesEntities + …AfterTagRemoval |
| M9 meta 属性间只容单空格 | KILLED | 2 行 | **0** | 20/23 | TestMetaContent（含两样本子测试） |
| M10 实体还原提前到标签处理之前 | KILLED | 2 行 | **0** | 22/23 | **TestStripUnescapesAfterTagRemoval** |

**合计 KILLED=10 / SURVIVED=0 / INVALID=0。**

### 四条自证逐条核实

1. **diff 非空**：10 体每体 1–2 行变化，全部实测非零（用 difflib，不用 git）。
2. **`go vet` 红绿都查 → 10 体全部 exit=0**。这条是本次复核的重点，**独立确认成立**：没有任何一个变异是靠编译错误「杀死」的，不存在本 Sprint 反复出现的假 KILLED 形态。
3. **`--- PASS` 严格低于基线 23**（判据是 `pc < BASE && rc != 0`，不是「0 失败」）：实测落在 19–22，无一为 0，也无一等于 23。
4. **首行完整**：每体记录测试输出首行，全部为 `=== RUN   TestStripRejoinsValueSplitByInlineTags`，证明测试真跑起来了而非未编译/未匹配。

与 dev 报告的唯一差异：M5 我测 19/23（它报 18）、M6 我测 20/23（它报 19），差 1 个子测试计数——源于两边变异实现的细微不同。**不影响判定**：两边都是 PASS 严格低于基线的 KILLED。

---

## 3. 重点复核二：M10 首轮存活与补测的有效性

问题：补的 `TestStripUnescapesAfterTagRemoval` 是真钉住了**顺序**，还是只钉住了「能还原实体」？

**结论：真钉住了顺序。**

判据是 M10 的杀手清单——该变异（把 `html.UnescapeString` 提前到标签剥离之前，其余一字不改）下 **22/23，唯一失败的就是 `TestStripUnescapesAfterTagRemoval` 本身**。若该测试只钉「能还原实体」，M10 保留了还原能力，就该存活。它没存活，说明断言确实区分了顺序：

```go
raw := []byte(`<p>正文里出现 a &lt;p&gt; b 这种转义写法</p>`)
assert.Contains(t, got, "a <p> b", "还原出的 <p> 不应再被当成标签剥掉")
```

顺序反过来时 `&lt;p&gt;` 先还原成真 `<p>`，随即被 `blockTagRE` 换成 `\n`，`"a <p> b"` 消失 ⇒ 断言失败。**与 `TestStripUnescapesEntities` 不重复**：后者在 M10 下仍然通过（还原能力没丢），只有前者能分辨顺序。

**关于「真实数据永远暴露不出该路径」**：我独立复现并确认——两份样本的 `&nbsp;` / `&amp;` / `&lt;` / `&gt;` / `&quot;` 计数**全部为 0**。dev 的判断成立，此路径只能用合成样本验证，这个补测是**唯一**的守护。

---

## 4. 两条硬判据的独立复核

### ① F6 钉：raw 是否逐字抄自 2025 样本的六层 span

**成立。** 我在 `pboc-2025-12-annual.html` 偏移 18816 处定位到原文，测试 raw 的实质部分逐字命中：

```
企业债券净融资2.39万亿元，同比多4825</span></span></span><span><span><span>亿元</span></span></span><span><span><span>；</span></span></span><span><span><span>政府债券净融资13.84万亿元
```

`in sample == True`。样本中该片段之后紧接 `，同比多2.54万亿元；非金融企业境内股票融资4763亿元，同比多1863亿元。`——测试在句中截断并补上闭合 `</span>×3` 与外层 `<p>`，**六层 span 形态原样保留，非简化版**。断言钉在 `同比多4825亿元` 与 `；政府债券净融资13.84万亿元` 两个具体串上，符合 DoD「不能只测标签被去掉了」的要求；M1/M2 两个方向的变异各自被它杀死，守护有效。

### ② 半角标点：2020 恰好 2 逗号 1 分号、2025 为 0

**独立复现成立。** 我按 strip.go 的前四步（删 script/style、块级换行、内联删除）复原「未归一正文」后计数：

| 样本 | 原始文件 | 未归一正文 | 归一后输出 |
|---|---|---|---|
| pboc-2020-06-h1 | 26 逗号 / 139 分号 | **2 逗号 / 1 分号** | 0 / 0 |
| pboc-2025-12-annual | 28 逗号 / 470 分号 | **0 / 0** | 0 / 0 |

三处位置与 DoD 逐字一致：

- `广义货币(M2)余额213.49万亿元,同比增长11.1%`
- `流通中货币(M0)余额7.95万亿元,同比增长9.5%`
- `票据融资增加9697亿元;非银行业金融机构贷款减少2775亿元`

原始文件里的 26/139 与 28/470 绝大多数落在 `<script>` / `<style>` / `href="javascript:…"` / `onclick` 内，被剥离吃掉——这**实测印证了「必须先删 script/style」的理由**，不是防御性冗余。

**分号那处确为作用域边界**：该行完整读作「…企（事）业单位贷款增加8.77万亿元，其中，短期贷款…票据融资增加9697亿元;**非银行业金融机构贷款减少2775亿元**。」——分号左侧属企业单位口径，右侧是并列的非银行业金融机构。**T5 可依赖此归一**；不归一则该边界在全角模板下失配。

---

## 5. 三个实测事实的抽验

| dev 报告 | 我的独立复现 | 结论 |
|---|---|---|
| 「社会融资规模」在 2020 样本出现 **0 次** | 2020：raw **0** 次、stripped **0** 次；2025：raw 10 次、stripped 8 次 | **成立**。据此按样本分别断言是**正确处置**——一刀切会造出永远失败的测试 |
| meta 属性有**双空格**，M9 证明单空格正则会让 PubDate/ArticleTitle 全取不到 | 两份样本各命中 `<meta  name` ×1、`"  content` ×1；M9 独立复跑 **KILLED，PASS 20/23**，杀手正是 `TestMetaContent` 及其两个样本子测试 | **成立**。`\s+` 是必需，非冗余 |
| `(?m)^[一二三四五六七八九十]、` 实测 2020=**6**、2025=**8** | 2020 得 `[一、二、三、四、五、六、]` 共 **6**；2025 得 `[一、…八、]` 共 **8** | **成立**。**T3 板块切分地基确认可用** |

附带核实：两份样本剥离后残留 `<` 均为 **0**。

---

## 6. 下游约定：`nonEmptyLines` 可复用性

**确认可复用。** `strip_test.go:242` 的 `func nonEmptyLines(s string) []string` 是**文件作用域的包级函数**，不是某个测试内部的闭包/私有逻辑；且 `internal/hestia` 下全部 6 个 `_test.go` 均声明 `package hestia`（内部测试包，非 `hestia_test`），因此 T3 的 `sections_test.go` 只要同样用 `package hestia` 即可直接调用。

**⚠ 补充一条 dev 未提的冲突预警**：strip_test.go 同时占用了 **`readSample`**（`strip_test.go:31`）这个包级名。T3/T4 若各自定义 `readSample` 读 testdata，**同样会同包编译冲突**。已确认目前与 dev-agent-45 的 `amount_test.go`（仅 `reverseAlternation`）无冲突。

---

## 7. 声明范围核对（越界申报检查）

- `writes` 声明：`./internal/hestia/strip.go`、`./internal/hestia/strip_test.go`
- `git show --stat acc5a96` 实际：**恰为这两个文件**，338 insertions，无第三个文件
- **无越界申报，无需事后补声明。**

**验证基线漂移核对**：`verify_baseline.head = acc5a96`，`discovery_sha256 = 48046a24…5440fa` 与当前 discovery 文件 sha256 **完全一致**。主仓库 HEAD 验证期间前移至 `65a4db0`（dev-agent-45 的 `amount.go`/`amount_test.go`，属 TASK 无关任务），但 `git diff acc5a96..65a4db0 -- internal/hestia/strip.go internal/hestia/strip_test.go` **为空**——**声明范围未漂移**，判定对象与交付物同一。

---

## 8. 咨询项（**不构成退回理由**，供 T3/T4 与 QA 参考）

我在 dev 的 10 体之外自设 7 个变异，专门探它在 `interfaces_exposed` 里**向下游承诺、但无任何 DoD 条款要求**的性质。结果 **1 杀 6 存**：

| 自设变异 | 结果 | 说明 |
|---|---|---|
| X6 不删 style（只删 script） | **KILLED**（22/23） | 守护到位 |
| X1 取消空白折叠 `[ \t ]+`→单空格 | SURVIVED | `interfaces_exposed` 承诺了，无测试守护 |
| X2 取消连续空行压缩 `\n{3,}`→`\n\n` | SURVIVED | 自述「便于阅读调试」，纯装饰，无守护属合理 |
| X3 `metaContent` 不做 `TrimSpace` | SURVIVED | 承诺「值已 TrimSpace」，无守护（样本 meta 值无首尾空白） |
| X4 `metaContent` 不做 `UnescapeString` | SURVIVED | 承诺「值已 UnescapeString」，无守护（**样本实体数为 0，真实数据永远暴露不出**，与 M10 同构） |
| X5 取消全角空格归一 | SURVIVED | 未在 DoD 与承诺中 |
| X7 块级标签表砍到只剩 p/div/br | SURVIVED | DoD 只要求「块级产生分隔」，`TestStripKeepsBlockBoundaries` 仅测 `<p>`；`table/tr/td/li/h1-6` 等无判据 |

**这 6 个存活体全部落在 DoD 之外**，`done_criteria` 无一条要求它们，因此**不作为退回依据**——为每个标签逐一写测试属过度规格。但它们是**真实的下游风险**：T3/T4 若依赖「空白已折叠」「meta 值已 TrimSpace/Unescape」，这些性质当前**没有测试守护**，未来重构可能静默破坏。建议记入 wisdom，若下游确实依赖则由依赖方补钉。

X4 与 X7 值得单独留意：X4 与 M10 是同一失效模式（真实样本实体数为 0 使该路径不可暴露）；X7 意味着若 PBOC 页面改用 `<table>` 承载数据，块级表的正确性无判据。

---

## 9. 对 DoD 本身的意见

**`non_functional[1]` 措辞有缺陷（轻微，建议 Leader 修正措辞，不影响本任务判定）**：该条字面点名锚点句「社会融资规模」，而 2020 样本中该词出现 **0 次**（我已独立复现 raw/stripped 均为 0）。若照字面一刀切实现，会造出一个**永远失败**的测试。dev 改为按样本分别给锚点集是正确处置，并已在 discovery 中说明理由（与 CONTRACTS H8 一致）。建议把该条改为「按样本分别指定锚点句」以免后续任务再踩。

其余 5 条 DoD 具体、可测试、判据明确，尤其 `boundary[0]`（F6）与 `error_handling[0]`（半角标点）都直接给出了必须断言的具体串——这是本次能做出高置信判定的主因。`error_handling[0]` 删掉「空输入不 panic」的决定亦正确。

---

## 10. 结论

**PASS → `verified`。**

判定依据的压倒性体现在四个独立维度同时成立：DoD **6/6** 条有具体断言覆盖、任务子集 **23/23 PASS** 且全包 146 PASS 无回归、strip.go 三函数**覆盖率 100%**、以及我**独立复跑的 10 体变异 10 杀 0 存活且 vet 全部 exit=0**（无假 KILLED）。dev 报告中的每一项事实主张（F6 逐字性、2/1/0 标点计数、社融 0 次、meta 双空格、板块 6/8、实体数 0）均经我独立复现吻合，**未发现任何夸大或未经验证的主张**；它主动披露的 harness 静默失效已被其四条自证抓住，我用不经 git 的独立路径复核得到同一结论。

存在的 6 个变异存活体全部落在 DoD 之外，记为咨询项转告下游，不构成退回理由。
