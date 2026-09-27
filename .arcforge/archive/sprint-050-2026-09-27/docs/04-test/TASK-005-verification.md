# TASK-005 验证报告（verifier: test-bk-a）

**最终结论：VERIFIED（第 2 轮，返工 r1 @ `6d83a0d8c2f9a5cf8b05f4261f39194d4b8f7e40`，依据任务文件现行的 8 条 DoD）**

## 第 2 轮复验（返工 r1）

- 被验对象：`6d83a0d8c2f9a5cf8b05f4261f39194d4b8f7e40`，等于 verify_baseline.head，也等于判定时主仓库的 HEAD。
- discovery sha256：`0d4203f30c327721d092db86b08f2525531c5e8aa1e5143239f8507c0a6ba191`，与 baseline 一致。
- 在隔离 worktree `../wt-verify-TASK-005` 中验证（detached，钉在上述全 sha）。
- Leader 的裁决（2026-09-27）：块边界吞掉空行属于期望行为。boundary[1] 已改写为「每个非空行按原顺序恰出现一次、每段不以空行开头或结尾、段内空行保留」。F1、F2 并入 functional[3]。

### 改动范围与自证锚点
- `git diff --stat 55b6dda71a36c1249e197845408a8a36975490a1 6d83a0d8c2f9a5cf8b05f4261f39194d4b8f7e40` 只涉及 render.go（numstat 19/5）、render_test.go（113/5）、render_golden.txt（2/2），都在 `writes` 范围内。summary_test.go 未改（blob 744896b2 不变）。
- 4 个文件在 discovery 的测量点 `5b777a501c73a25d97e1352a122084bbafe1954b` 与本次合入点的 blob 逐一相同，其中包含 simplifier 把 TestSplitBlankLines 改写成表驱动的那处改动。下面的数字全部由我重新采集。
- discovery 中关于 Split 的接口契约已订正：删去了错误的 `Join==TrimRight`，改写为新的性质描述，并明确标注【订正】。
- golden 只有两行变化：第 2 行头部加了「；领先 1」，第 13 行变为「CET1（n=2）」。我人工核对：3+1+2+1=7，与覆盖家数一致；CET1 的 N=2 是因为 B 的回退值被排除，与数据一致。其余 25 行未变。

### 重采
- `go vet` 无输出。
- `go test -count=1 -v` rc=0，PASS 117、FAIL 0。
- 计数互验：按缩进数得顶层 58、子测试 59，合计 117，与 `=== RUN` 的数量一致；源码中 `^func Test` 共 58 个，即上一轮的 54 加上本轮新增的 4 个（TestRenderHeaderSumsToTotal、TestRenderStatN、TestSplitBlankLines、TestSplitRenderSampleAllLimits）。两把尺一致。新测试没有 t.Run 子测试，所以子测试数仍是 59。
- 覆盖率 99.7%，Split、fmtStat、Render 均为 100%。
- 用 10 个独立的 seed（`-shuffle=1` 到 `-shuffle=10`）各跑一次，全部 rc=0。

### 复验清单（出自我的 checkpoint）
| 项 | 结果 |
|---|---|
| 我预判的「超长单行后接空行」会产出段首空行 | 已修复。`Split("短\n"+长×25+"\n\n尾",10)` 的最后一段是 `"尾"`。我写了一个判别力探针：在隔离树里去掉修复（`n==0 && ln==0` 时 continue）后，同一输入的最后一段变回 `"\n尾"`，探针转红。这个形态也已写进 TestSplitBlankLines 的第 4 例 |
| 空行反例 | TestSplitBlankLines：`Split("a\n\nb",3)=["a","b"]`（边界处空行丢弃），`Split("a\n\nb",4)=["a\n\nb"]`（段内空行保留） |
| renderSample 全 limit 遍历 | TestSplitRenderSampleAllLimits 遍历 limit=1…字符数+1，每个 limit 都调用 assertSplitProps |
| 性质断言本身是否足够强（逐条读 assertSplitProps） | 每段非空；不以换行开头或结尾；超过 limit 的段必须是单行；每段都是原文从 pos 往后的**逐字节子串**（因此段内空行不可能被压缩或改动）；段与段之间的间隙只能是换行；段必须对齐行首和行尾；末段之后只能剩下换行。合起来就等价于「非空行按序恰好出现一次，段内空行保留」 |
| 独立于 dev 断言的复核 | 我另写了一个探针，不复用 assertSplitProps，而是按行模型直接比较非空行序列、首尾空行、是否为连续片段、超限只能单行。覆盖：renderSample 的全部 limit，加上 20000 例随机文本（字母表中空行权重 2/7，含中文、emoji 和超长行，limit 取 1–15，seed 20260927）。全部通过 |
| F1：头部「领先 N」与分项和 | TestRenderHeaderSumsToTotal 用正则解析头部，对 golden 样本和另一个含 Ahead 的样本分别断言各分项之和等于覆盖家数；无 Ahead 时头部字符串与旧格式逐字相同 |
| F2：统计行「（n=K）」 | TestRenderStatN 断言：N 等于当期主体数时不标注；CET1 标注为「（n=2）」。N=0 时仍显示「CET1 无数据」且不带 n，由 TestRenderNaNShowsNA 守住，U10 可证 |
| T1–T7 | 除等价变异 T4 外全部 KILLED。T1 与 T3 现在还会让 BlankLines 和 AllLimits 两个用例同时转红 |
| Q27 | T8 KILLED，只红 TestSummarizeAheadFallbackAtPeriodNotInStats |

### 变异回归（脚本 scratchpad/tbka/mut_tbka_task005r.py：上一轮的 T1–T30 加上本轮新增的 U1–U11）
- T1–T30：29 个 KILLED，T4 仍是等价变异。原因是 n==0 时 flush 的是空缓冲，输出不变。
- 本轮新增的 11 个变异全部 KILLED：

| ID | 变异 | 转红 |
|---|---|---|
| U1 | 去掉段首空行跳过 | BlankLines、AllLimits |
| U2 | 所有空行都跳过（段内空行也丢） | BlankLines、AllLimits |
| U3 | 遇到空行就切段 | BlankLines（只有 `("a\n\nb",4)` 这个精确用例能抓到，靠性质断言抓不到，因为这样切出的每段仍是合法片段） |
| U4 | 领先数为 0 也显示 | HeaderSumsToTotal |
| U5 | 从不显示领先 | Sections、Golden、HeaderSums |
| U6 | 领先数误用 Stale 的数量 | 同上 |
| U7 | 统计行总是标 n | Sections、Golden、StatN |
| U8 | 统计行从不标 n | 同上 |
| U9 | 标注值用当期主体数 | 同上 |
| U10 | N=0 时也标 n | NaNShowsNA |
| U11 | 覆盖数与分项和不一致 | Sections、Golden、HeaderSums |

- 合计 41 个变异，40 个 KILLED，1 个等价。收尾时 worktree 干净，主仓库指纹一致。

### 第 2 轮覆盖矩阵（现行 8 条）
| # | 判定 | 依据 |
|---|---|---|
| functional[0] 段落顺序 | PASS | TestRenderSections 按序断言（22 行加上 CET1 行）。第 1 轮的 T21–T26、T30 在本轮重跑后仍然 KILLED |
| functional[1] golden | PASS | 逐字节比较；只变了两行，已人工复核 |
| functional[2] 预警行 | PASS | 与第 1 轮相同，T9–T15、T29 KILLED |
| functional[3] F1/F2 | PASS | 见复验清单；U4–U11 全部 KILLED |
| boundary[0] 无预警/无当期/NaN | PASS | T13、T16–T20 KILLED |
| boundary[1] Split（改写后） | PASS | 精确边界由 T1、T2 证明；超长行由 T5 证明；空行性质由 U1–U3、我的独立探针与随机样本证明；空串得 0 段由 TestSplitShortAndEmpty 证明 |
| boundary[2] Q27 | PASS | T8 KILLED |
| non_functional | PASS | vet、测试 117/0、覆盖率 99.7%、10 个 seed 全绿 |

---

## 第 1 轮（55b6dda，REJECTED）——原文保留

- 被验对象：`55b6dda71a36c1249e197845408a8a36975490a1`（= verify_baseline.head = 判定时的主仓库 HEAD）
- discovery sha256：`107862795677ff229dbf7c38808711ba5b910dfc66995fb378e92830f4b20c7b`（= baseline）
- 验证环境：隔离 worktree `../wt-verify-TASK-005`（detached，钉在上述全 sha），go1.24.4 darwin/arm64
- 结论：**REJECTED（task_defect）**。boundary[1] 要求 Split 满足「按 `\n` 拼回无损」，这一条被反例证伪；其余 6 条通过。

## 范围与自证锚点
- `git diff --stat d4d1d051a376b0e8c36a834461c62949ea1b18c9 55b6dda71a36c1249e197845408a8a36975490a1` 只涉及 4 个文件：render.go +168、render_test.go +188、summary_test.go +21、testdata/render_golden.txt +27，与 `writes` 一致。其中 summary_test.go 的 numstat 是 `21 0`，纯追加。
- 4 个文件在 discovery 的测量点 `5dcf6fd341e617a5d2b2c36ed27f39187f6560aa` 与合入点 55b6dda 的 blob 逐一相同，说明 dev 的自证数字测的就是合入的这版。下面的数字全部是我在 55b6dda 上重采的。

## 非功能重采
| 检查 | 结果 |
|---|---|
| `go vet ./internal/bank/` | 无输出，rc=0 |
| `go test -count=1 -v -coverprofile` | rc=0，PASS 113 / FAIL 0 |
| 计数互验 | 尺 1（按输出缩进）：顶层 54 + 子测试 59 = 113，与 `=== RUN` 的 113 相等。尺 2（按源码）：`^func Test` 共 54 个 = 43 + render 10 + Q27 1；新增测试没有子测试，所以子测试数仍是 59。两把尺一致 |
| 覆盖率 | 包整体 99.7%，render.go 所有函数 100% |
| 执行顺序 | `-shuffle=1` 到 `-shuffle=10` 共 10 个独立 seed 全部 rc=0 |

## 重点核查
- **① golden 逐字节比对，且样本覆盖六种形态**：golden 共 1394 字节 27 行，以换行结尾，不含 `NaN`/`Inf`。在隔离树里把 golden 改一个字符（150.00→150.01）或在末尾多加一个换行，TestRenderGolden 都会转红（rc=1），所以确实是逐字节比较。六种形态都能在 golden 里找到：Stale 带字段缺失（第 22、26 行，D银行）；Failed 带 H 股别名（第 24–25 行，E银行H）；Ahead（第 27 行，G银行）；CET1 截至（第 18 行，B银行「8.49%（截至 2026-03-31）」）；期次≠统计期的预警（第 8 行，行末「（2026-03-31）」）；149.96 边缘值（第 6、12、19 行）。
- **② 预警行字面上看得出越界**：golden 第 6 行 `149.96% < 150.00%`，第 7 行 `环比 -20.04pp（超 20.00pp）`。T9（精度改成 1 位）和 T29（恶化阈值改成 1 位）都 KILLED。
- **③ Split 精确边界**：TestSplitExactBoundary 用 7 个字符的文本，limit=7 时不切、limit=6 时切。T1（`> limit+1`）、T2（`>= limit`）、T3（行长算上换行）、T7（按字节计数）都 KILLED，而且只有这个测试转红。
- **④ Q27**：T8 把统计输入改成 Current∪Ahead 后 KILLED，只有 TestSummarizeAheadFallbackAtPeriodNotInStats 转红。用例的形态符合我在 TASK-004 提的要求：H 是 Ahead，它的 CET1 回退值 Period 恰好等于统计期，值为 5，一旦进入统计就会变成最差。断言覆盖了 N、Mean、Worst、WorstVal 以及 Current 名单。

## done_criteria 覆盖矩阵
| # | 条目 | 对应测试 | 断言是否测到 | 变异证据 | 判定 |
|---|---|---|---|---|---|
| functional[0] | 各段落与行按规定顺序出现 | TestRenderSections（22 行按顺序 Index）、TestRenderAliasesAndMissingFieldsOutsideCurrent | 逐行 Index 递增，信息区各类别的顺序有断言 | T21 T22 T23 T25 T26 T30 KILLED | PASS |
| functional[1] | golden 逐字节比对，样本含六种形态 | TestRenderGolden | 字符串整体相等；改一个字符、多一个尾换行都会转红 | T9–T30 中涉及渲染的都会让 golden 转红 | PASS |
| functional[2] | 预警行方向与格式、149.96 与 −20.04 的写法、期次标注 | TestRenderSections、TestRenderDeteriorationAlert | 字面完整行断言，同期的行末没有期次、非同期的有 | T10 T11 T14 T15 T29 KILLED | PASS |
| boundary[0] | 无预警、无当期、NaN 的展示，全文不含 NaN/Inf | TestRenderNoAlertsNoCurrent、TestRenderNaNShowsNA、TestRenderGolden | 字面断言 | T13 T16 T17 T18 T19 T20 KILLED | PASS |
| **boundary[1]** | Split：每段 ≤ limit；恰好等于不切、多 1 必切；超长行独占一段；**按 `\n` 拼回无损**；空串得 0 段 | TestSplit*（4 个） | 前四项都测到。**「无损」只在不含空行的文本上断言过** | T1–T3、T5–T7 KILLED；T4 等价（见变异表）。**我构造的反例证伪了「无损」** | **FAIL** |
| boundary[2] | Q27 守卫 | TestSummarizeAheadFallbackAtPeriodNotInStats | 见重点核查 ④ | T8 KILLED | PASS |
| non_functional[0] | vet、测试全绿、覆盖率 ≥80% | 命令行重采 | 见上 | — | PASS |

## 缺陷：Split 在块边界处吞掉空行，「按 `\n` 拼回无损」不成立
- 位置：render.go:151 `strings.TrimRight(cur.String(), "\n")`。flush 时会把块尾部的**所有**换行都去掉。当块的最后一行是空行时，这个空行就在两块之间消失了。
- 最小反例：`Split("a\n\nb", 3)` 得到 `["a", "b"]`，拼回是 `"a\nb"`，原文 `TrimRight` 后是 `"a\n\nb"`。`"a\n\n\nb"` 也会被切成 `["a","b"]`，一次丢两个空行。
- 用本任务自己的输出验证：我写了一个只在隔离树里跑、跑完即删的探针，对 `renderSample()`（828 字符，含 4 个段间空行）遍历 limit=1…829。其中 826 个 limit 会切成多段，**166 个拼回有损**，最大的有损 limit 是 679。例如 limit=679 时切成 2 段，第 2 段从「ℹ️ 未更新/失败」开始，拼回后空行从 4 个变成 3 个。Render 的每个段落标题前都有一个空行，所以只要块边界恰好落在段落交界处，就会触发。
- 为什么测试没发现：TestSplitRespectsLimitAndLines 是唯一断言 `Join == TrimRight(text)` 的用例，而它的输入是 300 行不含空行的文本。其余 Split 用例也都没有空行。测试名宣称的「只在行边界切分，拼回无损」比它实际的输入覆盖面更大。discovery 的 interfaces_exposed 也把 `strings.Join(chunks, "\n") == strings.TrimRight(text, "\n")` 写成了无条件成立的契约，而这个契约是假的。
- 定级：实际影响很轻，Telegram 里只是两条消息之间少一个空行，不丢任何内容行。但 DoD 把「无损」写成了可测的性质，实现却对本包 Render 的真实输出违反了它，所以按 task_defect 处理。
- 建议修复方向：
  1. flush 时只去掉块末尾那**一个**换行，例如 `strings.TrimSuffix(s, "\n")`，不要用 TrimRight 去掉全部换行。去掉后为空的块是否丢弃，要按「拼回无损」重新推一遍。另外，由空行开头的块（前一块以超长行结尾时会出现）目前拼回是无损的，改完要确认没有被破坏。
  2. 补用例：`Split("a\n\nb", 3)` 拼回相等；并对 `renderSample()` 遍历全部 limit 断言 `Join == TrimRight`。这正是我的探针，可以直接照抄。
  3. 自检：T1、T2、T3 仍要被抓到，这个空行反例和遍历用例要转绿。
- 如果 Leader 认为块边界的空行本来就不该保留，那就是 DoD 措辞的问题，需要改 DoD。但即便如此，discovery 里那条契约也得改写成带条件的形式。

## 变异表
脚本：scratchpad/tbka/mut_tbka_task005.py，变异由我独立设计。每个变异的锚点唯一，先 vet 通过再跑测试；每轮之后 `git checkout` 还原，并比对主仓库指纹。收尾时 worktree 干净，主仓库指纹一致。

| ID | 变异 | 结果 |
|---|---|---|
| T1 | Split 判断改为 `> limit+1` | KILLED |
| T2 | Split 判断改为 `>= limit` | KILLED |
| T3 | 行长计入换行 | KILLED |
| T4 | 去掉 `n > 0` | SURVIVED，**等价**：n==0 时 flush 的是空缓冲，被 `c != ""` 丢弃，输出不变 |
| T5 | 超长行在行内截断 | KILLED |
| T6 | 保留空段 | KILLED |
| T7 | n 按字节计数 | KILLED |
| T8 | Q27：统计输入改为 Current∪Ahead | KILLED |
| T9 | 精度改为 1 位 | KILLED |
| T10 | 预警行从不标期次 | KILLED |
| T11 | 预警行总是标期次 | KILLED |
| T12 | 不标「截至」 | KILLED |
| T13 | NaN 也标「截至」 | KILLED |
| T14 | 阈值比较符号反向 | KILLED |
| T15 | 变动值不带正号 | KILLED |
| T16 | 取值为 NaN 时原样输出 | KILLED |
| T17 | 变动值为 NaN 时原样输出 | KILLED |
| T18 | 空预警不写「无」 | KILLED |
| T19 | 无当期主体时也输出统计段 | KILLED |
| T20 | N=0 时不写「无数据」 | KILLED |
| T21 | 排名段不写别名 | KILLED |
| T22 | 信息区不写别名 | KILLED |
| T23 | 不列 Ahead 行 | KILLED |
| T24 | 字段缺失漏掉 Ahead | KILLED |
| T25 | 字段缺失漏掉 Stale | KILLED |
| T26 | 失败行排在未更新之前 | KILLED |
| T27 | 统计标题的 n 改用 Stat.N | KILLED |
| T28 | 覆盖行计数漏掉 Ahead | KILLED |
| T29 | 恶化阈值精度改为 1 位 | KILLED |
| T30 | 标题日期改用统计期 | KILLED |

共 30 个，29 个 KILLED，1 个等价。

## 观察项：Leader 已登记的 F1/F2 确实存在（不据此拒绝）
- **TASK-005-F1 确认**：golden 第 2 行「覆盖 7 家（统计期 2026-06-30：3 家；未更新 2；失败 1）」，3+2+1=6≠7，差的 1 家是 Ahead 的 G银行（第 27 行）。另外，T28（覆盖计数去掉 Ahead）会被抓到，说明「7」这个数本身有断言守着；F1 修复时如果在头部插入「领先 1」，只需要重生成 golden。
- **TASK-005-F2 确认**：golden 第 10 行标题写 `n=3`，但 CET1 那一行（第 13 行）实际只统计了 A银行 14.07 和 C银行 8.90 两个值，B银行 8.49 是回退值被排除，所以 CET1 的 N=2。均值 (14.07+8.90)/2 = 11.485，显示为 `11.48%`，这是 float64 存的 11.48499… 按 `%.2f` 输出的结果，与 discovery 的说明一致。

## 返工范围
只需改 render.go 的 Split flush 逻辑，并补 render_test.go 里的用例。golden 不受影响：TestRenderGolden 比对的是 Render 的输出，不经过 Split。复验时我会重跑 T1–T7 和空行反例，确认 golden 与 Render 未变，其余条目的结论可以直接沿用。
