# TASK-003 验证报告 —— 板块切分与版本探测

- **验证者**: test-agent-22
- **验证对象**: `internal/hestia/sections.go` + `sections_test.go` @ commit `8ab45dbf631d0038918f2fe2873a5d0678b0ca31`
- **承接时 assignment_epoch**: 1（出裁决时携带 `--expect-epoch 1`）
- **验证环境**: 隔离 detached worktree，锚 `8ab45dbf631d0038918f2fe2873a5d0678b0ca31`
- **判定**: **PASS → verified**

> 说明：本任务于 03:36:40Z 派发，但派发发生在验证者最后一次空闲前扫描（03:16Z）之后 20 分钟，
> `TeammateIdle` 作为一次性拦截点无法再唤醒已停机实例，故实际开工被推迟至 08:27Z 的催办之后。
> 期间**零工作量**（`transitions.jsonl` 中 test-agent-22 仅有 TASK-001 一条写入），非中断续做。

---

## 一、基线计数（自证）

在隔离 worktree @ `8ab45db`：

| 命令 | 结果 |
|---|---|
| `go vet ./internal/hestia/` | exit 0，无输出 |
| `go test ./internal/hestia/ -v -count=1` | **267 PASS / 0 FAIL / exit 0** |
| sections 子集（5 个 Test 前缀） | **45 PASS / 0 FAIL** |
| `go test -cover` | **93.5%** |
| `go test -race` | ok |

dev 自报「45 PASS / 全包 93.5%」——**两项均复现一致**。

---

## 二、完成标准覆盖矩阵

| # | 完成标准 | 对应测试 | 变异实证 | 判定 |
|---|---|---|---|---|
| functional[0] | 2025 切 **8**、2020 切 **6**，Title 与原文小节标题一致 | `TestSplitSectionsOnRealSamples`、`TestSplitSectionsSeparatesTitleAndBody` | N9（`sectionsV2` 8→7）→ KILLED | **PASS** |
| functional[1] | `detectExtractor` 三分支（v2 / v1 / 其余 error） | `TestDetectExtractor`、`TestDetectExtractorRejectsUnknownLayout` | N10、N13（去掉 hasTSF 条件）→ KILLED | **PASS** |
| functional[2] | **7 关键词 × 2 样本**逐个断言；正文提及 ≠ 板块主题 | `TestFindSectionResolvesAllT5Keywords`、`TestFindSectionIgnoresBodyOnlyMentions` | N1（退回 `Title\|\|Body`）→ KILLED，精确打红 `2025/人民币贷款` | **PASS** |
| boundary[0] | 中间态一律 error，**不降级** | `TestDetectExtractorRejectsUnknownLayout`（5 用例） | N6（社融锚点放宽）→ KILLED | **PASS** |
| error_handling[0] | 切分失败返回 error，错误自述实际板块数 | `TestDetectExtractorErrorNamesActualCount`、`TestSplitSectionsReturnsNilWhenNoTitle` | N12（`%d sections`→`%d parts`）→ KILLED；N7（nil→空切片）→ KILLED | **PASS**（附条件，见 P1） |
| non_functional[0] | 纯函数，无 I/O，不依赖包级可变状态 | `TestSplitSectionsIsPure` | 唯一包级 `var` 是不可变编译正则（`regexp` 并发安全）；`-race` 通过 | **PASS** |
| non_functional[1] | 板块数**硬断言**（`require.Len`）非范围断言 | `require.Len(secs, 8)` / `(secs, 6)` | N9 → KILLED | **PASS** |
| non_functional[2] | 尾注段必须显式排除或断言不参与抽取 | `TestFootnoteSectionNeverWinsFindSection` | N14 → KILLED（证明非空转，详见五） | **PASS** |

---

## 三、Leader 重点 ①：Title-only 改法是否引入新的定位失败

**结论：没有引入新的定位失败，而且比 dev 声称的更稳。**

我没有复用 dev 的断言，而是写了独立探针，枚举每个关键词命中的**全部**标题索引与**全部**正文索引
（探针为临时文件，验证后已删除，未进入交付物）。

### 3.1 实测标题（探针输出，与原文 HTML 逐条对照）

**2025（8 板块）**

| idx | Title（实测） | 原文行 |
|---|---|---|
| 0 | 一、社会融资规模存量同比增长8.3% | L319 |
| 1 | 二、全年社会融资规模增量累计为35.6万亿元 | L322 |
| 2 | 三、广义货币增长8.5% | L324 |
| 3 | 四、全年人民币存款增加26.41万亿元 | L326 |
| 4 | 五、全年人民币贷款增加16.27万亿元 | L330 |
| 5 | 六、12月份银行间人民币市场同业拆借月加权平均利率为1.36%，质押式债券回购月加权平均利率为1.4% | L334 |
| 6 | 七、国家外汇储备余额3.36万亿美元 | L337 |
| 7 | 八、全年经常项下跨境人民币结算金额为17.86万亿元，直接投资跨境人民币结算金额为8.46万亿元 | L339 |

**2020（6 板块）**

| idx | Title（实测） | 原文行 |
|---|---|---|
| 0 | 一、广义货币增长11.1%，狭义货币增长6.5% | L319 |
| 1 | 二、上半年人民币贷款增加12.09万亿元，外币贷款增加774亿美元 | L321 |
| 2 | 三、上半年人民币存款增加14.55万亿元，外币存款增加206亿美元 | L325 |
| 3 | 四、6月份银行间人民币市场同业拆借月加权平均利率为1.85%，质押式债券回购月加权平均利率为1.89% | L329 |
| 4 | 五、国家外汇储备余额3.11万亿美元 | L332 |
| 5 | 六、上半年跨境贸易人民币结算业务发生3.08万亿元，直接投资人民币结算业务发生1.72万亿元 | L334 |

**14 个标题全部与原文逐字相符**（原文我在 TASK-001 验证时已逐行读过两份样本）。

### 3.2 关键词歧义矩阵——这是本节的核心证据

| 关键词 | 2025 Title 命中 | 2025 Body 命中 | 2020 Title 命中 | 2020 Body 命中 |
|---|---|---|---|---|
| 社会融资规模存量 | **[0]** | [0, **7**] | [] | [] |
| 社会融资规模增量 | **[1]** | [1, **7**] | [] | [] |
| 广义货币 | **[2]** | [2] | **[0]** | [0] |
| 人民币存款 | **[3]** | [3] | **[2]** | [2] |
| 人民币贷款 | **[4]** | [**0**, 1, 4, **7**] | **[1]** | [1] |
| 加权平均利率 | **[5]** | [5] | **[3]** | [3] |
| 国家外汇储备 | **[6]** | [6] | **[4]** | [4] |

三条独立结论：

1. **每个关键词的标题命中数恰为 1**（2020 的两个社融词恰为 0，是正确的「找不到」）。
   这比「首命中恰好正确」强得多——Title-only 的正确性是**结构性的**，不依赖板块顺序。
   dev 在 discovery 里说的是「Title-only 在 7×2 上全部正确」，实际性质是**唯一**，
   我把它作为独立结论记下。
2. **G1 复现属实**：「人民币贷款」的 Body 命中是 **[0, 1, 4, 7]** ——**四个板块**。
   `Title||Body` 取首命中即 idx 0（社融板块），正是 reviewer 指出的缺陷。
3. **尾注段隐患属实**：idx 7 的 Body 同时含「社会融资规模存量」「社会融资规模增量」
   「人民币贷款」三个完整字面（Body 长 2680 字符，为全篇最长）。

### 3.3 反向确认：Title-only 会不会让某个本该命中的关键词落空

7 个关键词 × 2 样本 = 14 组，除 2020 的两个社融词（本就不该有）外**全部命中且唯一**。
`section.has` 保持 `Title||Body` 未动（`TestSectionHas` 覆盖），用途是 T5 定位到板块**之后**
判断有无可选句式——两者语义不同故未统一，这个区分是正确的。

---

## 四、Leader 重点 ②：首轮 5 个存活变异的补测是否真的杀得死

**全部杀死，且每条只被对应的那一条补测打红——因果精确，无连带伤害。**

| 首轮存活项 | 我的复现变异 | 打红的测试 | 结果 |
|---|---|---|---|
| #7 正文终点 `locs[i+1][0]`→`[1]` | N2 | `TestSectionBodyStopsBeforeNextTitle`（两份样本子测试均红） | **KILLED** |
| #8 Body 去掉 `TrimSpace` | N3 | `TestSplitSectionsTrimsTitleAndBody`（真实样本 + 合成输入） | **KILLED** |
| #9 Title 去掉 `TrimSpace` | N4 | `TestSplitSectionsTrimsTitleAndBody/标题行尾带空白`（**仅**合成输入） | **KILLED** |
| #6 社融判据 `findSection`→`has` | N5 | `TestDetectExtractorJudgesTSFByTitleOnly` | **KILLED** |
| #11 锚点 `社会融资规模存量`→`社会融资规模` | N6 | `TestDetectExtractorRejectsUnknownLayout/八板块但只有社融增量节` | **KILLED** |
| （另）`nil`→空切片 | N7 | `TestSplitSectionsReturnsNilWhenNoTitle` | **KILLED** |

**N4 值得单独说**：它只打红了「标题行尾带空白」这一条合成用例，真实样本子测试仍绿——
**实证了 dev 的说法「Title 的 TrimSpace 在真实样本上是空操作」**。若 dev 只补真实样本断言，
这条变异至今仍会存活。补合成输入是对的。

**你点名的 #7 危害我也确认了**：2025 第五板块（贷款）后面紧跟的第六板块标题里确实带着
`1.36%` 与 `1.4%` 两个利率数字（见 3.1 表 idx 5）。正文多吃一行标题后，T5 在贷款作用域
找句式确有抽到利率数的通路——这是静默错值而非报错，补测钉住它是必要的。

---

## 五、变异汇总：13 条，**12 KILLED / 1 SURVIVED**

每条附四条自证：diff 非空 / 首行完整（`package hestia`）/ `go vet` 红绿都查 / `--- PASS` 计数与基线比较。
脚本内置「diff 为空即作废本轮」的护栏。

| # | 变异 | sections 子集 PASS（基线 45） | 全包 PASS（基线 267） | 判定 |
|---|---|---|---|---|
| N1 | `findSection` 退回 `Title\|\|Body` | 40 | 262 | KILLED |
| N2 | 正文终点 `[i+1][0]`→`[i+1][1]` | 42 | 264 | KILLED |
| N3 | Body 去 `TrimSpace` | 42 | 264 | KILLED |
| N4 | Title 去 `TrimSpace` | 43 | 265 | KILLED |
| N5 | 社融判据 `findSection`→`has` | 44 | 266 | KILLED |
| N6 | 锚点放宽为「社会融资规模」 | 43 | 265 | KILLED |
| N7 | `return nil`→`[]section{}` | 44 | 266 | KILLED |
| N8 | 正则去掉 `(?m)^` 行首锚定 | 44 | 266 | KILLED |
| N9 | `sectionsV2` 8→7 | 43 | 265 | KILLED |
| N10 | v2 分支去掉 `hasTSF` 条件 | 42 | 264 | KILLED |
| N11 | `findSection` 返回**最后一个**匹配 | **45（=基线）** | **267（=基线）** | **SURVIVED** |
| N12 | 错误措辞 `%d sections`→`%d parts` | 42 | 264 | KILLED |
| N13 | v1 分支去掉 `!hasTSF` 条件 | 43 | 265 | KILLED |
| N14 | `Title\|\|Body` **且**取最后一个匹配 | 37 | 259 | KILLED |

### 5.1 N1 的因果性

N1 打红 5 条，其中 `TestFindSectionResolvesAllT5Keywords` 下**精确定位到 `2025/人民币贷款`
这一个子测试**——与 3.2 歧义矩阵预测的唯一歧义点完全一致。非连带伤害。

### 5.2 N14 的用途：验证尾注守护不是空转

我注意到 **N1 并未打红 `TestFootnoteSectionNeverWinsFindSection`**，于是专门构造 N14 追查。

原因是结构性的：尾注挂在**最后**一个板块，而 `findSection` 取**第一个**命中——
即使退回 `Title||Body`，那 3 个关键词在更靠前的板块也能命中，尾注永远轮不到。
这正是 dev 在该测试注释里写的「至今没被选中只是**顺序上的巧合**」。

N14（同时破坏两个前提：看正文 + 取最后一个）**成功打红该测试**，
证明 non_functional[2] 的守护**有真实鉴别力，不是空转**。dev 的描述准确。

### 5.3 N11 SURVIVED 的定性

四条自证齐全（diff 非空、首行完整、`go vet` exit 0、PASS 计数**等于**基线 45/267 而非低于）。

**它是等价变异体，不是覆盖缺口**：由 3.2 已证每个关键词标题命中唯一，
first-match 与 last-match 在两份样本上行为完全相同。但由此引出 P2（见下）。

---

## 六、Leader 重点 ③：`error_handling[0]` 的 DoD 解读 —— **我的判定：接受，附一个条件**

### 6.1 DoD 原文与实现的对照

DoD：「切分失败（如无任何板块标题）返回 error，错误信息能指出实际切出几个板块，
便于定位是样本变了还是切分规则错了。」

实现：`splitSections` 返回 `nil` 无 error；`detectExtractor` 返回
`hestia: unrecognized layout: 0 sections, tsf_section=false (known: 6 without tsf = rule@v1, 8 with tsf = rule@v2)…`

### 6.2 接受的理由

1. **DoD 文本没有指名哪个函数返回 error**，只要求「切分失败 → 返回 error 且自述实际板块数」。
   在包的对外行为层面（T6 `Parse`）这一条被满足。
2. **DoD 陈述的目的被完整满足，且更好**：`detectExtractor` 的错误除板块数外还带
   `tsf_section` 与两个已知形态，对「是样本变了还是切分规则错了」的判别信息严格更多。
3. **两条错误路径确实更差**：`detectExtractor` 无论如何都得处理 `len==0`，
   让 `splitSections` 也返回 error 会为同一条件造出两个报错点。
4. **该行为有测试守护且已变异证实**：`TestDetectExtractorErrorNamesActualCount`
   钉住 `"0 sections"` 与 `"3 sections"`，N12 证明它杀得死措辞漂移。
5. **dev 主动上报请求复核**，而非静默偏离 DoD。

### 6.3 但 dev 的理由里有一处**事实不成立**，条件由此而来

dev 写「全仓唯一生产调用点是 T6 Parse 里 `secs := splitSections(...)` 紧跟 `detectExtractor(secs)`」。

实测（`grep -rn` 全仓）：**`splitSections` 的生产调用点数量是 0**，
`detectExtractor` 同样是 0——两者目前**只被测试调用**，T6 尚未编写。

也就是说，那个「紧邻」不是**既有事实**，而是**对 T6 的预期**。
后果具体：**`error_handling[0]` 要求的保证目前不存在于代码库的任何位置**，
它完全依赖 T6 被按承诺的方式写出来，而当前没有任何机制强制这一点。

缓解因素：`splitSections` 是包私有（小写），误用半径限于 `package hestia`；
且其文档注释把该契约写清楚了。所以这是**协作约定风险**，不是设计缺陷。

### 6.4 我的判定

**接受当前签名，不退回。** 同时提出条件（记为 P1）：
**T6 的 DoD 应显式要求 `Parse` 在 `splitSections` 之后立即调用 `detectExtractor`
并将其 error 视为致命**。这把一条隐式跨任务耦合转成写明的契约，
否则 `error_handling[0]` 承诺的保证将始终悬空。

若 Leader 判定必须由 `splitSections` 返回 error，dev 已给出成本：2 行改动 + T6 加一次错误检查。
我不建议这么做，理由见 6.2。

---

## 七、发现的问题

### P1（中）`error_handling[0]` 的保证目前悬空，依赖尚未编写的 T6

见 6.3。**处置建议：在 T6 的 DoD 里写明 `Parse` 必须紧接着调用 `detectExtractor` 并把 error 当致命。**
附带纠正：dev discovery 里「全仓唯一生产调用点是 T6 Parse」应为「**计划中的**唯一调用点；当前生产调用点为 0」。

### P2（低）保证 `findSection` 正确的真正不变量是「标题命中唯一」，而它无人守护

`findSection` 文档写「返回…**第一个**板块」，但该语义当前**不可观测**（N11 存活）。
真正让它正确的是 3.2 证明的**结构性唯一**。风险在于：

> 若未来某期报告在**预期板块之后**新增一个同样命中该关键词的标题，
> `TestFindSectionResolvesAllT5Keywords` 仍然全绿（首命中依旧返回期望索引），
> 而歧义已经存在、正确性重新退化为「靠顺序」——**正是 G1 那一类 bug**。

**建议（非阻塞）**：给 `t5Keywords` 补一条「每个关键词在每份样本上标题命中数恰为 1」的断言。
成本极低，且把当前的「幸运唯一」升级为被守护的不变量。

### P3（信息，无需动作）尾注守护的触发条件

`TestFootnoteSectionNeverWinsFindSection` 在 N1 下不打红（尾注是最后一节，`findSection` 取首个），
在 N14 下打红。**非空转**，且与 dev 自己的注释完全一致。记录以免后续验证者误判其覆盖范围。

### P4（极低）`section.has` 生产代码中暂无调用者

只被测试调用，留给 T5。DoD `functional[2]` 明确要求它存在，**不是缺陷**，记录以免被当成死代码清理。

---

## 八、范围与漂移核查

- `git show --stat 8ab45db` = **2 files changed, 590 insertions**（`sections.go` 145 + `sections_test.go` 445）
- 声明 `writes` = `["./internal/hestia/sections.go", "./internal/hestia/sections_test.go"]`
- **恰好一致，无越界申报，无他人产物混入**
- `verify_baseline.head` = `8ab45db` = 当前 HEAD，声明范围内文件 `git diff` 为空，**判定对象未漂移**

---

## 九、结论

八条完成标准逐条有证据支撑，其中 7 条有变异实证的机器守护，1 条（`non_functional[0]` 纯函数）
由代码结构 + `-race` 佐证。13 条变异 12 杀 1 存活，唯一存活项经证明为等价变异体。

dev 对 G1 采取的是**改实现而非改期望**的修法，且改法经独立探针证明是结构性正确（标题命中唯一）
而非侥幸；首轮 5 个存活变异的补测经逐条复现全部有效；对 DoD 的一处偏离主动上报而非静默处理。
自报数据（45 PASS / 93.5%）全部复现一致。

发现的 4 个问题中无一构成退回理由：P1 是需要写进 T6 DoD 的跨任务条件，P2 是可选的加固建议，
P3/P4 是记录性说明。

**判定：PASS → `verified`**
