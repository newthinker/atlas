# M1c-3a 架构决策（AD）

> 上游：`.arcforge/docs/01-design/requirements-analysis.md`（实测依据全在那里）

---

## AD-1：月报新增两个专属 extractor 值，不复用 rule@v1

**决策**（人类 2026-08-25 裁决）：

| extractor | 适用 | 篇数 | 必填集 | 由来 |
|---|---|---|---|---|
| `rule@v1` | 季报/年报，含外汇节 | 19 + 2(5节) + 2(3月报) | 27 | 现有 |
| `rule@v2` | 季报/年报 + 社融两节 | 4 | 54 | 现有 |
| **`rule-monthly@v1`** | 4/5 节月报 | **46** | **25** = 27 − `fx_reserve` − `fx_rate` | 新增 |
| **`rule-monthly@v2`** | 7 节月报（含社融） | **7** | **52** = 54 − 同两个 | 新增 |
| **`tsf-stock@v1`** | 社融存量独立报告 | **69** | **18** = 总量 2 + 8 分项 × 2 | 新增 |
| **`tsf-flow@v1`** | 社融增量独立报告 | **69** | **9** = 总量 1 + 8 分项 | 新增 |

**为什么不复用**：`requiredFields` 是 `completeness` 的唯一依据。月报复用 `rule@v1`
会让每篇月报恒报「缺 2 个字段」，社融独立报告复用 `rule@v1` 会恒报「缺 27 个字段」——
而那两个/27 个字段**在那种报告里根本不存在**，不是缺失，是 absent-by-design。
把 by-design 的缺席记成 failed，`completeness` 这个指标就废了。

**为什么不把外汇板块降为「可选」**：那会弱化 `rule@v1` 对 truncated fetch 的防护，
而收紧该防护正是本迭代 TASK-004 的目标之一，两者方向相反。

**字段数自校验**（实测，非推算）：货币 6 + 存款 7 + 贷款 10 + 利率 2 + 外汇 2 = 27；
27 + 社融 27 = 54。月报去掉外汇节的 2 个 ⇒ 25 / 52。

⚠️ **必填集一律从模板表派生，禁止手写字段清单**（沿用 `tsfSectionFields` 的做法）。
`required.go` 头部注释记着理由：模板表才是事实，前缀/手抄清单会先错。

---

## AD-2：`detectExtractor` 判据从「数板块数」换成「板块集合 × period_type」

**现状**：`hasTSF && len==8 → v2`；`!hasTSF && len==6 → v1`；否则 error。
两条都是**数节数**，于是 5 节 v1（2020-09/2022-09，差一个无关节）与全部四种月报布局
一律落进 default。

**新判据**：

```go
core   := hasAllSections(secs, coreSectionKeywords())  // 广义货币/人民币存款/人民币贷款/加权平均利率
hasFX  := findSection(secs, "国家外汇储备")
hasTSF := findSection(secs, tsfSectionKeyword)

switch {
case !core:                        → error（列出缺了哪几个核心板块）
case !hasFX && periodType != "monthly": → error（累计期报告必须有外汇节，见下）
case hasTSF && hasFX:              → rule@v2
case !hasTSF && hasFX:             → rule@v1
case hasTSF && !hasFX:             → rule-monthly@v2
case !hasTSF && !hasFX:            → rule-monthly@v1
}
```

一处改动同时接上四类：5 节 v1（19+2 篇）、4/5 节月报（46）、7 节月报（7）、6 节月报（2）。

### 🔴 新判据引入一条缝，必须同时堵上

「无外汇节」原先是**未知布局**（响亮失败），新判据下变成 `rule-monthly@v1` 的**特征**。
⇒ 一篇 v1 季报若外汇节恰好被截断，会被**静默降级**成 25 字段的月报 extractor，
`completeness` 认为它本就不该有外汇字段，**没有任何闸门会响**。

故 `detectExtractor` **必须接收 `periodType`**（签名变更，调用点仅 `Parse` 一处）：
只有 `monthly` 才允许无外汇节。这是本 AD 里唯一的破坏性改动。

⚠️ 这条与需求文档 T3 的自我要求同源——它写着「换判据必须配一条『真截断仍被拒』的
测试，否则新判据会连真截断一起放行」。本 AD 把那条要求**扩展到外汇维度**。

---

## AD-3：`extractFields` 按 extractor 决定板块适用性

现状只有一个维度：`rule.v2Only && extractor != extractorV2 → continue`。
新增月报族后需要第二个维度：外汇板块在 `rule-monthly@*` 下不适用。

**做法**：把「板块是否适用于该 extractor」收敛成**一个函数**，不要在 `extractFields`
里堆 if。板块归属与 `requiredFields` 必须同源，否则会出现「跳过了却仍在必填集里」
或反之——两者分叉时先错的一定是手抄的那份。

⚠️ 判据是**声明式跳过**，不是「碰巧 `findSection` 找不到就放过」。`extractFields`
现有注释写得很清楚：「显式跳过比『碰巧匹配不到』更清楚：前者是声明，后者是巧合，
而巧合会在某期正文里偶然出现社融字样时失效」。同一条理由适用于外汇节。

---

## AD-4：`parseTitle` 增加 kind 返回值，`Parse` 按 kind 分派

```go
const (
    kindFinance  = "金融统计数据报告"
    kindTSFStock = "社会融资规模存量统计数据报告"
    kindTSFFlow  = "社会融资规模增量统计数据报告"
)
func parseTitle(title string) (period, periodType, kind string, err error)
```

`Parse` 三条路：`kindFinance` 走 `splitSections → detectExtractor → extractFields`；
另两种**没有板块结构**，整篇当一节直接调 `extractTSFStockArticle` / `extractTSFFlowArticle`。

**社融整篇复用现有抽取函数的依据**（需求文档实读，Leader 复核代码确认）：
`section` 只有 `Title` / `Body` 两个字段，而 `extractTSFStockSection` **只读 `Body`**
——它本就不知道自己是被板块切分喂的还是被整篇喂的。⇒ 不需要新模板。

签名变更是编译错误，调用点由编译器逐个点出。

---

## AD-5：`checkPeriodTypeSupported` 删 monthly 分支，不删函数

`parse.go:219-224` 的原话：穷举 switch 的价值是「新增第六种 period_type 会由
`TestEveryPeriodTypeHasAnExplicitSupportDecision` 逼人明确表态」。删函数这道防线就没了。

同时更正 `parse.go:229` 那条把形态写成「1-5月」的注释——55 篇实测已推翻它。

---

## AD-6：模板措辞变体两处（互不相干，合并只因同改 profiles.go）

**(a) 企业贷款作用域锚点**：`企（事）业单位贷款|非金融企业及机关团体贷款`。
⚠️ **住户侧不动**——现有 `住户(?:部门)?贷款` 已覆盖两版（Leader 实测 C5）。

🔴 锚点错位的后果不是「抽不到」，是**住户的短期贷款跑进企业字段**——两个值都是
合法量级，而 `corp_loan_reconcile` 是**加总**校验，错位后总和不变、拦不住。
故必须配**逐字段**的数值断言，不能只断言「无 error」。

**(b) `moneyRE` 的全角括号**：`广义货币（M2）` 4 篇（2026-07/2026-04/2023-11/2023-10）。
现为硬编码 `\(`。M1 / M0 同句同形，一并受影响。

### 🔴 更正：「交替顺序不能反」这条 rationale 是**假的**（dev-m1c3a-a 实测证伪，Leader 独立复验）

本 AD 原写着「正则交替**长的写在前**（Go 交替是最左优先），`十一|十` 顺序不可反」。
**这条从需求文档抄来，三级传播到本 AD 与三个任务的 DoD，而它不成立。**

实测四种排法在 `前十一个月人民币贷款增加…` 上的捕获：

```
前十一个月|前十个月      → "前十一个月"
前十个月|前十一个月      → "前十一个月"     ← 顺序反了，结果一样
前(?:十一|十)个月        → "前十一个月"
前(?:十|十一)个月        → "前十一个月"
```

**四种逐字相同。** 错因：我以为 leftmost-first 意味着「第一个分支匹上就定了」，
实际 Go 的 `regexp` **会回溯**——`十` 匹配后卡在 `个` vs `一`，退回来试 `十一` 并成功。
leftmost-first 是对**整个正则能否匹配成功**而言，半路失败的分支会被放弃。

⇒ **「长的写在前」保留为零成本防御，但不要写那个 rationale**（写了会被后人引用，
而它是错的）。真正的安全性来源是 `cumulativeMonthAlt` 与 `cumulativePeriods` 的
**一一对应**，由测试机械守住。

⚠️ 对 `企（事）业单位贷款|非金融企业及机关团体贷款` 而言顺序更是无关——两者**互不为前缀**，
连回溯都用不上。

---

## AD-7：累计前缀两处硬卡点，缺一即静默命中 0

`profiles.go` 现有注释（reviewer B5 四格实测）已记明：`periodAlt` 与
`cumulativePeriods` 是**两个独立的硬卡点**。只加前者 ⇒ 候选句涨了但口径判定全筛掉；
只加后者 ⇒ 正则不命中，**与现状逐字相同（完全 no-op）**。

⇒ 必须有一条测试机械地守住两者的**一一对应**，且必须做**反向消融**证明两处都真接上了。

**1 月报特例**：`1月份` 就是累计句（1 月的累计=当月），与当月句同形。
安全性依赖实测前提：**非 1 月报里不出现「1月份+指标」**——Leader 用全部 218 篇
验证得 **0 条**（需求文档只验了 55 篇，结论一致）。

⚠️ 验证该前提**必须带词边界** `(?<![0-9])`：`11月份` 含子串 `1月份`。不带边界会得出
12 条假阳性。而 **Go 侧不需要 lookbehind**：`[0-9]{1,2}月份` 的 `{1,2}` 贪婪 + 最左
匹配 ⇒ 在「11月份」处捕获完整的 `11月份`，查表不命中、正确排除。这条要有测试钉住。

---

## AD-8：wave 划分由 scope 互斥倒推，不是由主题倒推

八个任务全部落在 `internal/hestia` **一个 package**，文件级互斥是唯一的并行约束。
`writes` 精确到**单个文件**（testdata 精确到单份快照），wave 内两两不交。

关键路径：`TASK-003 → TASK-004 → TASK-007 → TASK-008`（4 层）。

⚠️ 每份 testdata 快照**只属于一个任务**，其余任务只读。这是 wave 1/2 能三路并行的
前提，也是「谁建谁负责」的边界。

---

## AD-9：验收只认真跑，且 pre/post 必须同源同时刻重采

`calibrate` 真跑是本迭代**唯一的端到端验收**。

- **pre 基线已由 Leader 在动手前采**（`01-design/calibrate-baseline-BEFORE.txt`，
  HEAD `4a12794`）——改动开始后就再也采不到了。
- **post 必须在最后一次改动之后统一重采**。分批采的数字即使每个在当时都对，合到
  一份报告里也不构成对最终产物的自证：验证者无法从数字本身看出它测的是哪棵树。
- 报告里每个自证数字（n、失败数、覆盖率、字段数）都要与它的**测量条件同行落地**，
  并写明采样时的 HEAD 全 sha。

⚠️ 目标值「非社融 n 约 80」是**推算，只有告警权**。真跑值才是要固化进 CONTRACTS 的
那一个（沿用契约 `backfillExpectedPeriods` 的先例）。
