# TASK-005 验证报告 —— 模板表与字段抽取

- **验证者**: test-agent-22 / 承接时 `assignment_epoch` = 1
- **验证对象**: `profiles.go` `profiles_test.go` `extract.go` `extract_test.go` @ `8e4811d0c6bc984a0df73e830f9653ae5b8bc648`
- **验证环境**: 隔离 detached worktree，锚 `8e4811d0c6bc984a0df73e830f9653ae5b8bc648`
- **判定**: **PASS → verified**

---

## 一、基线（自证）

| 项 | 我的实测 | dev-45 自报 / Leader 复现 | 一致 |
|---|---|---|---|
| `go test ./internal/hestia/ -v -count=1` | **315 PASS / 0 FAIL / exit 0** | 315 / 0 | ✓ |
| `go test -cover` | **89.1%** | 89.1% | ✓ |
| `go tool cover -func \| tail -1`（门禁口径） | **89.0%** | 89.0% | ✓ |
| `go vet` | exit 0 | exit 0 | ✓ |
| `go test -race` | ok | — | — |
| `git diff --numstat 8ab45db..8e4811d` | **4 文件 +1542/−0** | 4 文件 +1542/−0 | ✓ |

**范围核对**：改动的 4 个文件与声明 `writes` 逐条相同，**无越界申报**。
`verify_baseline.head` = 当前 HEAD = `8e4811d`，声明范围内文件 `git diff` 为空 ⇒ **判定对象未漂移**；
`discovery_sha256` 实测 `ecb18c6a…` 与基线记录**逐字相同**。

变异结束后重跑基线，**315 PASS / 0 FAIL 精确复现**，工作区 `git status` 干净。

> **89.1% 与 89.0% 不是矛盾，是两个量**：`go test` 自报按语句数直算得 89.1%，
> `go tool cover -func` 逐函数汇总得 89.0%（门禁取后者）。同一 commit 上两者各自稳定复现，
> 非测量波动。discovery 里两处已分别标注来源（`key_findings` 89.1 / `gate_result` 89.0）。

---

## 二、完成标准覆盖矩阵（9 条）

| # | 完成标准 | 对应测试 | 变异实证 | 判定 |
|---|---|---|---|---|
| functional[0] | 54/27 字段，键在 `allFields` 内，**每值与 golden 相等**（`InDelta` 1e-6），**双向** | `TestExtractFieldsOnV2Sample` / `V1Sample` + `assertMatchesGolden` | **P12**（余额 `toWanYi`→`toYi`，差 1e4）→ KILLED | **PASS** |
| functional[1] | 7 类句式；**判据 A** 名称禁贪婪；**判据 B** 交替片段无前缀对 | `TestNoGreedyCaptureInTemplates`、`TestProfileAlternationsHaveNoPrefixPairs`、T4 的 `TestNoAlternativeIsPrefixOfAnother` | **P7**（`QuoteMeta`→`(.+)`）→ KILLED（打红 12 条，含 `TestNoGreedyCaptureInTemplates` 与两份 golden 比对） | **PASS** |
| functional[2] | 复用 T4 的 `directionAlt`/`unitAlt`，不另写词表 | `TestTemplatesReuseAmountFragments` | **P13**（profiles 自写词表）→ KILLED | **PASS** |
| functional[3] | `rate_ibo` ≠ `rate_repo` 且各自来自自己的锚定句；容忍 2025 缺「平」 | `TestRateRegexpsToleratePingTypo`、`TestRateRegexpsRejectLoosePatterns` | **P6**（`rateRepoRE` 放松成 `加权.*利率为`）→ KILLED | **PASS**（DoD 机制描述有误，见 F1） |
| boundary[0] | 本外币口径 + 孪生句 ①期内/单月 ②币种捕获组 ③指名机制并单设失败用例 | `TestBalanceRefusesWhenRMBSentenceMissing`、`RefusesAmbiguousRMBSentences`、`IgnoresForeignCurrencyTwin`、`LoanFlowPrefersCumulativeRegardlessOfOrder` | **P1**（谓词恒真）、**P2**（丢口径）、**P3**（丢期次）、**P4**（0 命中退而取隔壁）、**P5**（≥2 命中静默选一）→ **全 KILLED** | **PASS** |
| error_handling[0] | 同字段赋值两次即报错；生产文件 `newAmount("…")` 零命中 | `TestCollectorRejectsDuplicateAssignment` + T4 的 `TestNoLiteralDirectionWordAtProductionCallSites` | **P8**（允许覆盖）→ KILLED；`grep` 实测生产文件**零命中** | **PASS** |
| error_handling[1] | 只有按版本显式跳过的字段允许缺席，任何模板未命中一律 error | `TestExtractFailsLoudlyOnMissingSentence`、`RejectsUnknownExtractor` | **P14**（`mustMatch` 静默跳过）→ KILLED；**P9**（去掉版本校验）→ KILLED | **PASS** |
| non_functional[0] | 数据/代码分离 + 清单自检（覆盖度、无重复、字段名合法） | `TestTemplateTablesCoverAllFields`、`HaveNoDuplicateFields`、`AreDeclaredInFieldsGo` | **P15**（漏登记 `FieldRateRepo`）、**P16**（重复登记同一字段）→ 均 KILLED | **PASS** |
| non_functional[1] | 每个 `t5Keyword` 在每份样本上标题命中 **≤ 1** | `TestSectionKeywordsHitAtMostOneTitle` | 2×2 消融 A/B 独立复现（见五） | **PASS** |

---

## 三、变异汇总：**16 条（P1–P16）全部 KILLED，0 存活**

harness 内置「**diff 为空即作废本轮**」护栏，每轮打印 diff、首行、`go vet` 退出码与全包 PASS/FAIL。
**该护栏实际生效过一次**：P16 首次施加时 sed 未落地，harness 直接判本轮作废（exit 9）而不是打出
一个假的「SURVIVED」——这正是 T3 验证时我踩过的坑，护栏是那次之后加的。

| # | 变异 | 全包 PASS（基线 315） | 打红的具名测试 | 判定 |
|---|---|---|---|---|
| P1 | `selectRMBBalance` 谓词恒真（不看口径） | 310 | `RefusesWhenRMBSentenceMissing`、`IgnoresForeignCurrencyTwin`、两份 golden 比对 | KILLED |
| P2 | 期内合计丢掉**口径**判据 | 312 | `IgnoresForeignCurrencyTwin`、两份 golden 比对 | KILLED |
| P3 | 期内合计丢掉**期次**判据 | 311 | `LoanFlowPrefersCumulativeRegardlessOfOrder`、**仅** V1Sample | KILLED |
| P4 | `selectUnique` 0 命中时退而取隔壁 | 314 | `RefusesWhenRMBSentenceMissing` | KILLED |
| P5 | `selectUnique` ≥2 命中时静默选第一个 | 314 | `RefusesAmbiguousRMBSentences` | KILLED |
| P6 | `rateRepoRE` 放松成 `加权.*利率为` | 310 | `RejectLoosePatterns`、`FailsLoudlyOnMissingSentence` | KILLED |
| P7 | 名称捕获 `QuoteMeta`→贪婪 `(.+)` | 303 | `NoGreedyCaptureInTemplates` + 7 条 | KILLED |
| P8 | `collector.set` 允许覆盖 | 314 | `CollectorRejectsDuplicateAssignment` | KILLED |
| P9 | `extractFields` 去掉版本校验 | 314 | `RejectsUnknownExtractor` | KILLED |
| P10 | `loanScopeSpans` 去掉顺序校验 | **99** | `LoanScopesRejectOutOfOrderAnchors` | KILLED（**经 panic**，见 F2） |
| P11 | 作用域右边界失效（延伸到正文末） | 314 | `LoanScopeBoundsSubItemsToItsOwnSector` | KILLED |
| P12 | 余额 `toWanYi`→`toYi` | 311 | 两份 golden 比对 + 2 条 | KILLED |
| P13 | profiles 自写词表 | 314 | `TemplatesReuseAmountFragments` | KILLED |
| P14 | `mustMatch` 未命中静默跳过 | 310 | `FailsLoudlyOnMissingSentence`、`LoanScopeBounds…` | KILLED |
| P15 | 清单表漏登记一项 | 314 | `TemplateTablesCoverAllFields` | KILLED |
| P16 | 清单表重复登记同一字段 | 311 | `TemplateTablesHaveNoDuplicateFields` + golden 比对 | KILLED |

**因果性均已逐条核对**：每条变异打红的具名测试都与变异点直接对应。唯一的例外是 P10，
它的击杀机制不是断言失败而是 panic——单列在 F2。

---

## 四、Leader 重点逐条核验

### ① `[^外]` 守卫的偏离设计 —— **偏离是对的，且比原方案强**

我独立跑探针枚举了两份样本上四条模板的**全部**匹配及捕获组（临时文件，验证后已删）：

**2025 存款板块** `balanceRE` → **3 句**
```
["本外币" "336.14" "万亿元" "增长" "9"]
["人民币" "328.64" "万亿元" "增长" "8.7"]     ← 要它
["外币"   "1.07"   "万亿美元" "增长" "25"]
```

**2020 贷款板块** `flowRE` → **4 句**
```
["上半年" "人民币" "增加" "12.09" "万亿元"]   ← 要它
["6月份"  "人民币" "增加" "1.81"  "万亿元"]
["上半年" "外币"   "增加" "774"   "亿美元"]
["6月份"  "外币"   "增加" "154"   "亿美元"]
```

三点判断：

1. **把限定词做成显式捕获组，比 `[^外]` 严格更强**。`[^外]` 是「靠子串不重叠的巧合」，
   而捕获组把三句都变成**可见的候选**，再按值挑并要求唯一。失效模式全部变响：
   0 命中报错（不退而取本外币）、2 命中报错（不最左优先静默选一）。P1/P4/P5 三条变异证实。
2. **dev-45 指出的 `[^外]` 假阴性成立，但它是「潜在缺陷」而非已发生的 bug**：该守卫要求
   「人民币」前必须存在一个字符，而 T3 的 `section.Body` 两端已 `TrimSpace`，某期正文若以该句
   开头就会匹配失败。**在两份现有样本上从未触发**——余额句前面都有「12月末，」「6月末，」，
   紧邻「人民币」的字符是「末」。判该设计偏离的分量时按「潜在」算，不按「实际错误」算。
3. **「要求唯一」这一步的价值超出 DoD 预期**：2020 的 4 个候选意味着**任一单维度谓词都会得到
   2 命中 ⇒ 报错**，而不是静默取错。P2/P3 实测正是如此——降级为单维度不会产出错值，会产出错误。

### ② 「因为错误的理由而绿」 —— 修法有效，**我另找了同类形态**

P9（去掉版本校验）现在被 `TestExtractFieldsRejectsUnknownExtractor` 干净打红，
错误信息断言（`unknown extractor` + `rule@v3`）确实是杀死它的那一条。修法有效。

**判据是「这个断言能不能被另一条错误路径满足」，不是「有没有断言文案」**（dev-45 提出、
Leader 采纳的边界，我照此收窄）。要求所有 `require.Error` 都加 `Contains` 会把测试绑死在
措辞上，改一句提示就假红——那是「恒响的检查」的对偶。**没有竞争路径的 `require.Error` 是干净的。**

按此判据排查其余四条，**未再发现可被竞争路径顶替的**。**证据是变异隔离而非 `Contains` 的存在**：
P4 / P5 / P10 / P11 四条变异分别只打红 `RefusesWhenRMBSentenceMissing` /
`RefusesAmbiguousRMBSentences` / `LoanScopesRejectOutOfOrderAnchors` /
`LoanScopeBoundsSubItemsToItsOwnSector` 中的一条——各自咬住一条互不相同的错误路径，
这正是「无竞争路径」的操作化形式。

### ③ M8 / M9 两条构造性测试 —— **确实钉住了它声称的东西**

- **M9 右边界**（`TestLoanScopeBoundsSubItemsToItsOwnSector`）：构造「住户段缺短期贷款」的输入。
  **P11** 实测——边界失效后住户段一路延伸到正文末，借用企业段的 4.81 万亿；该测试单独打红。✓
- **M8 顺序**（`TestLoanScopesRejectOutOfOrderAnchors`）：构造企业段在住户段之前的输入。
  **P10** 实测单独打红。✓ 但击杀机制是 panic 而非断言，见 F2。

两条测试各自被**且仅被**对应变异打红，互不重叠，符合「构造性测试钉住真实数据观测不到的失效」的设计意图。

### ④ 两条需求文档没写的实测事实 —— **全部确认属实**

1. **外币孪生句用美元单位** ✓ 探针输出中外币句单位全部是 `万亿美元` / `亿美元`。
   dev-45 的补充说明也对：`currencyAlt` **包含**「外币」是刻意的——若不含，
   `外币存款余额1.07万亿美元` 里既无 `本外币存款余额` 也无 `人民币存款余额` 子串，
   那句会**根本不可见**而非被排除，候选计数便看不到它。含进来才使「要求唯一」的计数有意义。
2. **2020 存款板块也有月度孪生句** ✓ 探针实测 `["6月份" "人民币" "增加" "2.9" "万亿元"]`。
   **DoD 的 G3 只点了贷款板块**，存款板块同形这一点是 dev-45 补的。属实且重要。

### ⑤ 第 9 条「≤ 1」—— 落地正确，2×2 消融独立复现

实测数据与我在 T3 的歧义矩阵对得上：2025 七个关键词命中数全为 1，2020 五个为 1、**两个为 0**。
判据写「≤ 1」而非「恰为 1」是必须的。

---

## 五、2×2 消融：我独立复现，并补了第三格

| 消融（隔离 worktree） | `TestSectionKeywordsHitAtMostOneTitle`（≤1） | T3 的 `TestFindSectionResolvesAllT5Keywords` |
|---|---|---|
| **A** `findSection` 恒 `false` | **PASS**（察觉不到） | **FAIL**（接住） |
| **B** `splitSections` 末尾追加同标题板块 | **FAIL**（接住） | **PASS**（察觉不到） |
| **C** `secs` 非空但 `Title` 全为空 | **PASS**（平凡为真） | **FAIL**（接住） |

A/B 与 dev-45 报的完全一致：**每条恰好接住对方看不见的那一格**。

**C 是我加的，用来核 Leader 点名要我查的那件事**——dev-45 自称
「实现里的 `require.NotEmpty(t, secs, …)` 只挡住『`splitSections` 返空』这一种空转，
挡不住『secs 非空却无任何标题命中』」。

**该自我限定准确**：消融 C 下 `secs` 有 8 个元素（`NotEmpty` 通过）、所有 Title 为空
（每个关键词命中数为 0）⇒「≤1」平凡为真，只有配对的肯定式测试接住。
**它对自己自证覆盖范围的判断是对的，没有夸大。**

判读全程只锚定这两条具名测试的红绿，不看红的条数。
（消融 A 因插入提前 `return` 导致 `go vet` 报 unreachable code，`exit=1`；
`unreachable` 不在 `go test` 默认的 vet 子集内，测试二进制正常编译运行，结论不受影响。）

---

## 六、独立核实项

- **T3 文件一行未动** ✓ `sections_test.go` 的 blob sha 在 `8ab45db` 与 `8e4811d` 均为
  `5e2b02bf31bc09e1a1b18ced3aec685c6fc1176c`；`sections.go` 同样无 diff。
  `TestFindSectionResolvesAllT5Keywords` 确在 `sections_test.go:325`。
- **生产文件字面量方向词零命中** ✓ `grep -rnE 'newAmount\("|parseRatio\("' --include='*.go'`
  在全部非 `_test.go` 文件上零命中；`extract.go` 内四处调用全部传变量。
- **golden 比对的依赖方向正确** ✓ `assertMatchesGolden` 比对的是 TASK-001 手工抄录、
  经我全量 81/81 独立核对过的 golden 表——**不是拿实现校验实现**。

---

## 七、发现的问题

### F1（中）DoD `functional[3]` 与 `profiles_test.go:370` 注释的**机制描述是错的**

> ⚠️ **本条的定性已被更正，见本文末「附录：F1 定性更正」**（2026-08-10 验 TASK-006 时补测）：
> 下文「机制描述是错的」这个定性**偏重了**——准确说法是「在本样本上不成立，但危害真实、
> 依赖句序才显形」。**原文以下保留不改**，更正在附录。

两处都声称「放松成 `质押式回购加权.*利率为` 会静默取到 **1.36**」。在真实 2025 利率板块正文上实测：

| 形态 | 实际捕获 |
|---|---|
| `质押式回购加权平?均利率为`（现行） | **1.4** ✓ |
| `质押式回购加权.*利率为`（注释原话） | **1.4** |
| `质押式回购加权.*?利率为` | **1.4** |
| `加权.*利率为`（DoD 原话） | **1.4** |
| `加权.*?利率为` | **1.36** ← 只有这一种真会取错 |

机制：保留 `质押式回购` 前缀会把匹配**起点**锚在回购句上，而 1.36 出现在它**之前**，
结构上取不到。只有「丢掉前缀 **且** 改非贪婪」才会取到 1.36。

**结论与守护都是对的，错的只是理由**：`TestRateRegexpsRejectLoosePatterns` 用
**孤立句逐条喂**的手法，与全文顺序无关，上述四种放松形态它**全都能抓**（P6 实证）。
这条测试比催生它的那个理由更强。

**为什么值得记**：这是**同一份 DoD 里第二次**出现「貌似合理但未经实测的正则机制断言」——
第一次是「RE2 取首个可匹配 ⇒ `净融资` 须排在 `融资` 前」，已被 dev-45 与 test-agent-23
各自实测推翻（交替顺序整个写反，输出逐字节相同）。两次的形状相同：**把一条正确的引擎机制
套到了一个不适用的语境上**。建议此类断言一律先跑一次再写进 DoD。

**处置建议**：改注释与 DoD 措辞即可（把「会取到 1.36」改成「`加权.*?利率为` 会取到 1.36；
带前缀的放松形态在本样本上碰巧仍取 1.4，但这是文本顺序的副产品，不构成安全依据」），
**不改实现、不改测试**。

### F2（低）`loanScopeSpans` 的顺序校验同时是 **panic 安全的唯一防线**，而注释只写了语义理由

P10 去掉顺序校验后，击杀机制不是断言失败而是
`panic: runtime error: slice bounds out of range [287:123]`，整个测试二进制中断
（PASS 315→99）。原因：`extractLoanSection` 用 `end = spans[i+1].start` 去切
`sec.Body[sp.start:end]`，**这一步的内存安全完全依赖 `loanScopeSpans` 保证升序**。

现行代码有该校验，**生产路径不存在这个 panic**，故不是缺陷。但注释只说了
「scope slicing would assign sub-items to the wrong sector」（语义后果），
读者可能以为删掉它只是降低正确性。**建议补一句「同时保证下游切片的上下界有序」。**

### F3（信息）**2025 样本不检验期次维度**

P3（丢掉期次判据）只打红 `LoanFlowPrefersCumulativeRegardlessOfOrder` 与 **V1Sample**，
**V2Sample 仍绿**。探针证实成因：2025 是年报，`flowRE` 只有 2 个候选（全年人民币 / 全年外币），
无月度孪生句；2020 有 4 个（2 期次 × 2 币种）。

⇒ **期次维度的真实样本覆盖只由 2020 一份样本承担**，另加那条构造性测试。
记录供 T7 参考：若将来样本集调整，这一维度的覆盖会随 2020 样本一起消失。

---

## 八、结论

9 条完成标准逐条有证据支撑，**每一条都有至少一条变异实证**。16 条独立变异全部被杀、0 存活，
因果逐条核对；3 条消融验证了新旧断言的配对关系并核准了 dev-45 对自身自证边界的判断。
自报数字（315 PASS / 89.1% / +1542−0）三方（dev-45 / Leader / 我）完全一致，范围无越界，
判定对象未漂移。

dev-45 对计划书的最大偏离（弃用 `[^外]`，改显式捕获组 + 唯一性）**经实测强于原方案**，
且它把偏离连同理由主动写进了 discovery。它对 DoD 中一处错误机制（交替顺序）做了实测推翻，
本次我又查出同族的第二处（F1）——但那两处**都不影响实现与测试的正确性**。

三个发现均为文档/注释级，**无一构成退回理由**。

**判定：PASS → `verified`**

---

## ⚠️ 附录：F1 定性更正（test-agent-22 于 2026-08-10 验证 TASK-006 时补测）

**保留上文 F1 原文不改，此处追加更正**——判定依据只追加、不原地改写。

上文 F1 写「DoD 与注释的机制描述**是错的**」，并给出机制解释「保留 `质押式回购` 前缀会把匹配
**起点**锚在回购句上，1.36 在其之前取不到」。

**那个解释只覆盖了带前缀的形态，不适用于 `加权.*利率为`**（它的起点确实落在拆借句）。
dev-45 在 TASK-006 里补的机制才完整：**贪婪的 `.*` 会一路吃到本段最后一个「利率为」，
而那正是回购句。**

更要紧的是它新加的一句「两份样本里回购句恰好都排在最后」——我把两句**顺序对调**后实测：

| 形态 | 原文顺序（拆借在前） | **顺序对调**（回购在前） |
|---|---|---|
| `质押式回购加权平?均利率为`（现行实现） | 1.4 ✓ | 1.4 ✓ |
| `质押式回购加权.*利率为` | 1.4 | **1.36 取错** |
| `加权.*利率为` | 1.4 | **1.36 取错** |
| `加权.*?利率为` | **1.36 取错** | 1.4 |

**⇒ 贪婪与非贪婪随句序互换谁出错，两种放松形态都不安全**；现行 `平?` 形态在两种顺序下
都正确——它的正确性**不依赖句序**，而任何放松形态都依赖。

**更正后的准确表述**：DoD 那句话**在本样本上不成立**，但它警示的危害**是真实的**，
只是依赖句序才显形。上文「机制描述是错的」这个定性**偏重了**。

**不受影响的部分**（原判定与守护均维持）：
- `TestRateRegexpsRejectLoosePatterns` 用**孤立句逐条喂**，与全文顺序无关，四种放松形态全都抓得住；
- 「不得放松锚点」这个结论**更强了**——它现在有两个独立理由（贪婪与非贪婪各有一种句序下取错）；
- TASK-005 的判定 `verified` 不变。

追加来源：TASK-006 验证（`4948f0f317f2bcc67beb689dc397673b1f2f58b4`），
完整对照见 `.arcforge/docs/04-test/TASK-006-verification.md` 第五节。
