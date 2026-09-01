# M1c-3b · 第二轮 Adversarial Review（跨视角对抗）

- 审查者：qa-m1c3b
- 时间：2026-09-01（**修订版**：并入三个 lens 的正文，并订正我上一版的两处）
- 对象：`master @ 21815beda194398b1ee9b0777e969f4f102b8ac3`
- 变更规模：+3156 / −90 ⇒ **Large** ⇒ 三个 lens（Skeptic / Architect / Minimalist）

---

## 0. 关于 lens 子代理 —— **订正我上一版的说法**

上一版我写「本轮对抗性发现全部出自我本体，不是三方合议」。**那句话现在是错的，收回。**

三个 lens 的最终回复当时被 idle hook 循环截成了 stub，我两轮索要未果，据此下了那个限定。
Leader 随后从 transcript 里把正文提取出来给了我：

- `<scratchpad>/leader-m1c3b/lens-architect.md`（66146 字符）
- `<scratchpad>/leader-m1c3b/lens-minimalist.md`（45889 字符）
- Skeptic 正文未提取到，我只拿到 notification 摘要里的三条 CRITICAL（含变异编号 M17/M19）

**读完的判断：lens 的发现确实独立于我本体，且有我完全没覆盖到的。**
Architect 的两条 CRITICAL（下面 C-2 / C-3）我一条都没找到。
⇒ 那句限定必须改成：**本轮是三方 + 我本体的合议，Skeptic 一侧只有部分内容。**

**这件事本身也是一条发现**（建议进机制缺口清单）：
idle hook 给只读子代理的解锁条件是**落盘**，而只读子代理按角色定义**做不到** ⇒
产出卡在它自己手里，父代理只看到一句 stub。
⚠️ 提取时还有一个坑：三份 notification 都报 `tool_uses: 0`，实际是 34/39/49 次 Bash
——**那个字段不可信，不能据它判断子代理有没有干活**。

---

## 1. 🔴 我要更正 Leader 的一处判断：字段集相交，但不在会出事的地方

Leader 据 Skeptic 第 3 条独立算了字段集求交，得 `rule-monthly@v2 ∩ tsf-stock@v1 = 18`、
`rule-monthly@v2 ∩ rule@v2 = 52`、`tsf-stock@v1 ∩ rule@v2 = 18`，
结论是「**「三 extractor 字段集设计上不相交」为假**」，并认为够 CRITICAL、
足以动摇人类在 dod-gate 的那个裁决。

**那三个交集数我复现了，全对。但结论不成立。** 我自己算了**全部六个 extractor 的两两交集**：

```
|rule@v1|=27  |rule@v2|=54  |rule-monthly@v1|=25  |rule-monthly@v2|=52
|tsf-stock@v1|=18  |tsf-flow@v1|=9   |fieldOrder|=54

rule@v1         ∩ tsf-stock@v1 =  0      rule@v1         ∩ tsf-flow@v1 = 0
rule-monthly@v1 ∩ tsf-stock@v1 =  0      rule-monthly@v1 ∩ tsf-flow@v1 = 0
tsf-stock@v1    ∩ tsf-flow@v1  =  0
rule@v2         ∩ tsf-stock@v1 = 18      rule@v2         ∩ tsf-flow@v1 = 9
rule-monthly@v2 ∩ tsf-stock@v1 = 18      rule-monthly@v2 ∩ tsf-flow@v1 = 9
```

**代码注释点名的是哪三个？** 原文是「三个 extractor 的字段集设计上不相交**（27 / 18 / 9 = 54）**」
——27/18/9 唯一对应 `rule@v1` / `tsf-stock@v1` / `tsf-flow@v1`。
**这三个的两两交集全是 0，且 27+18+9 = 54 = `fieldOrder` 全集，是一个完美划分。**
⇒ **注释对它点名的那三个是真的。**

**而且我做了实测**，不止算集合。在真语料上探针跑到 `mergeByBusinessKey`，
统计每个合并组的 `Parts` 组合与**组内两两字段集交集**：

```
 42  tsf-stock@v1                                    17  rule-monthly@v1 + tsf-flow@v1 + tsf-stock@v1
 15  rule@v1 + tsf-flow@v1                            6  rule-monthly@v2
  6  rule@v1 + tsf-flow@v1 + tsf-stock@v1             4  rule@v2
  2  rule@v1 + tsf-stock@v1                           2  tsf-flow@v1
  2  tsf-flow@v1 + tsf-stock@v1
组数合计 = 96（自洽校验 ✓）      组内字段集相交的对数 = 0
```

**v2 系两个 extractor 只以单篇出现，从不进合并组。** ⇒ 冲突为 0 **不是运气**。

⇒ **人类那个 dod-gate 裁决的前提没有被推翻，不需要重新评估。**

**真正成立的、较弱的那条发现**（我记 WARNING，见 W-6）：
`mergeByBusinessKey` 只按 `(period, period_type, published_at)` 归组，
**没有任何东西限制哪些 extractor 可以进同一组**。一旦 v2 系报告与 tsf 系同日同期出现，
就有 18 或 9 个字段重叠 ⇒ 可能真冲突 ⇒ 而按 W-4，冲突**不阻断入库、不影响退出码**。
**前提由语料形状守着，而非由代码守着；前提破的时候失效是静默的（exit 0）。**

---

## 2. Leader 的问题：把判别式跑一遍 DoD（0 处也是结论）

Leader 已收回「找第 8 处」这个存在性提法，改成：拿判别式
**「X 和 P 分别在哪个函数、哪个阶段产生？答案相同或答不上，那条 DoD 可疑」**
把所有「用数 X 证明性质 P」的 DoD 逐条过一遍。**我过了，结果如下（不外推、不凑数）。**

| # | DoD 条目 | X 的产地 | P 的产地 | 判定 |
|---|---|---|---|---|
| 1 | TASK-010「`merged@v1` = 42」 | `Save`/DB 的 extractor 列 | `mergeGroup` 写的就是那个 extractor | 🔴 **同源** —— 已确认倒下（已知 #4，单表得 28） |
| 2 | TASK-010「`unknown_extractor:merged@v1` 条数 = 0」 | 报告文本 | `gateCompleteness` 分支 | 🔴 **恒真** —— 已确认（已知 #7） |
| 3 | TASK-010 / 006「恒等式三 `96 = 54 + 42`」 | `SingleArticle`/`MergedGroups` 计数器 | `Merged = len(groups)`，二者同一循环 | 🔴 **恒真（新）** —— 见 C-4，我用变异 M17 端到端证实 |
| 4 | TASK-005「monthly 阈值 > 0.02613」 | `stockContinuityRates`（calibrate 阶段，**相邻对**） | 闸门在 load 阶段比的是**最近已接受期** | 🔴 **异源但两源分叉** —— 即 C-1 |
| 5 | TASK-011「`TestMergedPartsDoNotRoundTrip`」 | `scanObservation`（读路径） | 「不入库」是写路径的性质 | 🔴 **答非所问** —— 见 D-10 |
| 6 | TASK-008「PASS 差值 = 新增断言数」 | `--- PASS` 行数（测试函数数） | 断言数 | 🔴 **仪器错** —— 见 D-11 |
| 7 | TASK-010「`tsf_stock` = 79」 | DB 两表非空计数（Save 阶段） | 「合并不影响标定」（calibrate 阶段） | ✅ **真异源**，判据成立，我已复现 79 |
| 8 | TASK-001「背对背 diff 逐字节一致」 | 改动前后两份二进制的 stdout | 「重构不改行为」 | ✅ **强判据**，无可挑剔 |
| 9 | TASK-002「并集 = 52 = `without(fieldOrder, fxSectionFields())`」 | `requiredFields` 求并 | 右式独立从 `fieldOrder` 推 | ✅ **两条独立推导**，好判据 |
| 10 | TASK-005「`TestShippedConfigLoadsAndIsCalibrated` 读真配置」 | 仓库真 yaml | 配置可被接受 | ✅ 强判据，且填补了「CI 从不调真配置」的洞 |
| 11 | TASK-012「F12 顺序：6 字段 × 10 轮」 | 报告的 Reason 前缀 | 遍历 `fieldOrder` | ✅ 强判据（存活概率 (1/6)^10），且自守前提 |
| 12 | TASK-009「注释改动前后 PASS 数相等」 | PASS 计数 | 「不改行为」 | 🟡 **弱但无害**：对纯注释改动近乎必然成立，不误导 |
| 13 | TASK-006「`PartialCoverage` 列出缺哪一族」 | 来源族 | 理由说的是**缺多少字段** | 🔴 **判据与理由是两个量** —— 见 D-9 |

**结果：13 条里 7 条可疑（2 条是 Leader 已知的），6 条经得住。** 这是过完的结果，不是找到的目标数。

---

## 3. 新增 CRITICAL（来自 lens，由我实测坐实）

### 🔴 C-2 · 「标题解析不出期次」的标题原文在生产路径上**永远打不出来**（Architect 发现，我实测坐实）

Architect 读码推出、标注「未实测」。**我把它做成了测量。**

`writeLoadReport` 第一件事是 `checkLoadIdentities`，不过就 `return`，**一个字节都不写 `w`**。
而 `len(Unclassified) > 0` ⟺ 恒等式一必不成立（代码注释自己写明）。
⇒ 打印标题原文的那段（`backfill_load_report.go:49-54`）**在生产路径上不可达**。

**我的实测**（拷贝真语料到临时目录，把一篇的标题改成解析不出期次的串，跑真二进制）：

```
exit=1
stdout 字节数 = 0                         ← 报告一个字都没有
stderr: 核对报告的恒等式不成立……
        一：Total(218) ≠ Attempted(198) + Unsupported(19)（有 1 条标题解析不出期次…）
```

⇒ 运维在「**站点改了期次表述**」——按代码自己的说法是「最需要被人看见的变化」——这个场景里，
**只拿到一个数字「1 条」，看不到任何一个标题**。

两端的测试都在、都绿，没有一条问「运维看得见标题吗」：
`TestLoadReportSurfacesUnclassified` 为了跑到打印分支，手工设 `res.Unsupported = 1` 去凑平恒等式
——**它构造的是生产路径上不可能存在的状态**；`TestBackfillLoadFailsLoudlyOnUnclassified` 只断 `err`
与内存里的 `res.Unclassified`，**从不看 `out`**。

**建议（可照抄的动作，不是「以后要注意」）**：
`checkLoadIdentities` 从 `writeLoadReport` 里提出来，由 `BackfillLoad` 调用；
恒等式失败时**先渲染报告 + 顶部印失败说明**再返回 error。
同包 `collectSamples` 就是「先渲染再判错」，**该原则在 calibrate 侧遵守了、在 load 侧反过来了**。
补一条端到端断言：`out` 含标题原文。

### 🔴 C-3 · 恒等式在**库已写完之后**才校验，失败留下的库会挡住重跑（Architect 发现，我实测坐实）

顺序是：`loadParsedArticles` → `NewStore`（建库）→ 96 次 `Validate+Save` → **最后**才校验恒等式。
而恒等式一、二的全部加数在 `loadParsedArticles` 里就已确定，**与库无关**。

**我的实测**（同一次跑）：

```
失败后 DB 仍在：90112 字节，observations=75、pending=20   ← 灌完了 95 行才宣布账对不上
原样重跑 → exit=1：「…已存在：回填是一次性动作…重跑请先删掉该文件再来」
```

⇒ 「一开始就知道要失败」的情形，现在的行为是**建库、灌 95 行、然后宣布账对不上、不出报告**，
而**第一条错误串里没有一个字提到它刚建了一个库**。运维要撞第二次才知道。

**建议（零成本）**：把 `checkLoadIdentities` 拆成 `checkInputIdentities`（一、二）与其余两道，
前者在 `NewStore` **之前**调。这把「不可逆副作用先于账目检查」这个顺序错误彻底消除。

### 🔴 C-4 · 恒等式三**恒真**——它正是恒等式二那段注释明令避免的形状（Minimalist 读码 + Skeptic 变异 M17，我端到端复现）

```go
res.Merged = len(groups)
for _, g := range groups {
    if len(g.SourceIDs) > 1 { res.MergedGroups++ } else { res.SingleArticle++ }
}
// 恒等式三：res.Merged != res.SingleArticle + res.MergedGroups
```

每组必然且只增一个计数器 ⇒ **两者之和恒等于 `len(groups)` 恒等于 `Merged`**。
**这就是「一个由两个加数派生出来的和，再拿去和这两个加数比」——作者在恒等式二的注释里
用一整段论证明令避免的那件事，恒等式三原样犯了。**

**我的实测（M17，隔离 worktree）**：把判据 `len(g.SourceIDs) > 1` 改成 `>= 1`：

```
go test ./internal/hestia/ ./cmd/atlas/ -count=1   →  rc=0，FAIL 数 = 0     ← 变异存活
真语料重跑 → exit=0，报告唯一差异是这一行：
  −  合并后观测: 96  （单篇 54 + 合并组 42）
  +  合并后观测: 96  （单篇 0 + 合并组 96）
  且「四道恒等式: 全部成立 ✓」照常打印
```

⇒ **报告可以印出一对完全错误的数（0/96），恒等式照样宣布全部成立，全套测试照样绿，退出码 0。**
而 `54 + 42` 里那个 **42** 正是 TASK-010 DoD 的头号验收值之一。

**建议**：`SingleArticle` / `MergedGroups` 至少要有一条钉住具体值的断言
（fixture 上断 `SingleArticle == N` 且 `MergedGroups == M`，而不是断它们的和）；
或把恒等式三换成一条**真异源**的交叉校验：
`MergedGroups` 应等于 DB 里 `extractor = 'merged@v1'` 的行数（**跨两张表**）。
后者一举把已知 #4 和本条一起闭合。

---

## 4. 新增 WARNING（lens 提出，我采信）

- **W-6 · 合并组不限制 extractor 组合**（见 §1）。前提由语料守着，不由代码守着；破了是静默的。
  **建议**：`mergeGroup` 里若组内任意两 part 的必填集相交，直接记一条 `MergeConflict` 级别的告警
  ——这条判据可求值，且不依赖「有没有真的撞上值」。
- **W-7 · `eachParsedArticle` 的返回值（sha256 未校验篇数）在 load 路径被静默丢弃**
  （`backfill_load.go:339`，Architect + Minimalist 独立命中）。
  calibrate 侧把它汇总成 warning，load 侧直接扔。sha256 未校验意味着文件可能被改过/截断，
  **而截断的 HTML 仍可能 Parse 成功、只是少抽字段**——这正是 load 最不该忽略的一类。
- **W-8 · load 路径完全不消费 `manifest.failed` 与 `manifestWarnings`**（Architect）。
  「fetch 就没抓到」的篇目在核对报告里**彻底不存在**，`Total = len(m.Articles)` 不含它们
  ⇒ 四道恒等式对这批**结构上不可见**。
- **W-9 · 「月报族」四常量清单在同一文件里抄了两遍**（`backfill_load.go:160` 的 `pickArticleID`
  与 `:401` 的 `extractorFamilies`，Architect + Minimalist 独立命中）。
  **同一事实的两个副本，改一处不会让另一处变红**——本仓库明令反对的模式。
  新增 extractor 要同步 6 处，只有 1 处有守卫（`validExtractors` 的逐项表态测试）。
- **W-10 · `PendingReasons` 的键是 `period + "/" + period_type`，少了 `published_at`**（Architect），
  与合并键不同构。同期不同 `published_at` 的两条会互相覆盖判因。本语料未撞上。

---

## 5. Architect 的方案层发现 = Leader 认领的第 8 处 DoD 缺陷（我复核了数字）

TASK-011 的 DoD 用「改 `Validate` 签名要动 **39 个调用点、4 个文件**」否掉了
「不要 `Observation.Parts`」这个方案，转而选了加字段打补丁。

**我自己核的数**：

```
.Parts 在 internal/hestia 之外          → 零命中
hestia.Validate 的跨包调用点            → 零命中
hestia.Validate 的包内非测试调用点      → 2 个（ingest.go:196、backfill_load.go:274）
```

（Architect 报「3 个非测试调用点」，真值是 **2**；不影响结论。）

⇒ Architect 的推荐（改包内 `validateWith(..., req []string)`，导出签名一个都不用动）
**代价确实远低于 39**。Leader 已认领：**「不是算错一个数，是用一个没核实的数否掉了正确的方案」**。

**这条与前 7 处的性质不同，我同意 Leader 的定级**：前 7 处影响**验收判定**，
这一处影响**技术方案选型**——而那个被否掉的方案，正是本可以让 A-1 那个接缝缺陷**不存在**的方案
（没有 `Observation.Parts` 这个冗余，就没有「包装上有、内层没有」这条接缝）。
**我们为一个不必存在的冗余配了一道守卫，而那个冗余已经造成过一次真缺陷。**

---

## 6. 按 Leader 要求：归属划分（本 sprint 引入 vs 既有前提）

| 发现 | 归属 | 处置方向 |
|---|---|---|
| C-1（跨期当相邻期判） | **既有闸门 + 本 sprint 的新标定与新批量路径共同致害** | 闸门要改；**分流结果变动属人类裁决** |
| C-2（Unclassified 标题不可达） | **本 sprint 引入**（`writeLoadReport` 是新的） | 该修，零争议 |
| C-3（建库先于账目检查） | **本 sprint 引入** | 该修，零成本 |
| C-4（恒等式三恒真） | **本 sprint 引入** | 该修 |
| W-1（PartialCoverage 用来源代字段） | **本 sprint 引入** | 该修 |
| W-6（合并组不限 extractor 组合） | **既有前提**，本 sprint 依赖了它 | **记明并限定适用范围**即可，配一条可求值告警 |
| W-2/D-10、W-3、W-4、W-9、W-10 | 本 sprint 引入 | 该修 |
| W-5（skipped 在过闸观测上不可观测） | **既有 schema** | 移交 M1c-4，或加报告小节（半天） |

---

## 7. 第二轮 verdict

**REJECT** —— **4 条 CRITICAL**（C-1 ~ C-4）、**10 条 WARNING**、若干 SUGGESTION、
**6 处 DoD 层缺陷**（D-8 总纲 + D-9 ~ D-12 + Leader 认领的方案选型那处）。

**不是 CONTESTED**：全部 CRITICAL 都有可复现的实测证据（我逐条跑过），无 reviewer 分歧。
唯一的分歧我已在 §1 处理——**Leader 的字段集结论我更正了，方向是降级不是升级**。

### fix_items（供 Leader 编排 `review_fix`）

| # | 修什么 | 级别 | 归属 |
|---|---|---|---|
| 1 | C-1：`prior[0]` 与本期不相邻时不得按原阈值判；订正「`[0]` 就是上一期」注释；`Preceding` 文档写明只返回已入权威表的期次；补跨接缝测试 | CRITICAL | 人类裁决门 |
| 2 | C-2：`checkLoadIdentities` 提出 `writeLoadReport`；恒等式失败时**先渲染再报错**；补「`out` 含标题原文」的端到端断言 | CRITICAL | 直接派工 |
| 3 | C-3：拆出 `checkInputIdentities`（恒等式一、二），在 `NewStore` **之前**调 | CRITICAL | 直接派工 |
| 4 | C-4：`SingleArticle`/`MergedGroups` 各自钉具体值；恒等式三换成与 DB `merged@v1` 行数（跨两表）的异源交叉校验 | CRITICAL | 直接派工 |
| 5 | W-1/D-9：`PartialCoverage` 判据从「缺哪一族」换成「缺哪些字段」 | WARNING | 直接派工 |
| 6 | W-2/D-10：补 `assert.NotContains(insertSQL(obs) 的列清单, "parts")` | WARNING | 直接派工 |
| 7 | W-3：`thresholds.go` 注释改回 DoD 原话 | WARNING | 直接派工 |
| 8 | W-4 + W-6：冲突非空 ⇒ 返回 error；并在 `mergeGroup` 里对「组内两 part 必填集相交」出告警 | WARNING | 直接派工 |
| 9 | W-7/W-8/W-9/W-10 + S-1：sha256 未校验数汇总、`manifest.failed` 入账、月报族清单去重、`PendingReasons` 键补 `published_at`、节标题去歧义 | WARNING | 可合成一个任务 |
| 10 | 方案层：评估 Architect 的 `validateWith(..., req []string)`，删掉 `Observation.Parts` | WARNING | **建议单独立项**，它消除的是 A-1 那条接缝本身 |

**`reason_class` 建议：`dod_defect`**（C-1、C-4、W-1、D-10 的 dev 与验证者都忠实做到了
DoD 要求的事，是 DoD 要求的那件事不对）。C-2/C-3 更接近 `task_defect`，
若 Leader 要拆两批派，可按此分。

---

## 8. 把 Leader 那把尺用在我自己的产出上

> 如果一条经验的载体是「某人当时注意到了」，它写进文档也不会被下一个人执行；
> 只有落成**手段**（命令、判别式、可照抄的步骤）才真的转移。

我照这把尺过了一遍自己的报告，**改掉了三处「以后要注意」式的处方**：

| 原措辞（不会被执行） | 改成（可执行） |
|---|---|
| 「守恒判据要配分流正确性判据」 | **写成 M1c-4 每任务 DoD 的强制条目**：「落 pending 的每一期逐条给出判因，并说明它不可能由本工具自身造成」 |
| 「注意合并组的 extractor 组合」 | **写成代码里的一条告警**：`mergeGroup` 里组内两 part 必填集相交即出声（判据可求值，不依赖是否真撞上值） |
| 「恒等式三没有守卫」 | **写成一条具体断言**：`MergedGroups` == DB 里 `merged@v1` 的跨表行数（真异源） |

⇒ 上面 fix_items 的 10 条**全部是动作**，没有一条是「以后注意」。
