# M1c-3a Sprint · 最终 Code Review（两轮）—— **修订版 v2**

> 🔴 **本文取代 v1。v1 的裁决是 PASS，那是错的，已撤回。**
> 成因：三个只读 lens 子代理的正文在我发出 v1 **之后**才送达。我逐条用自己的仪器复验，
> 其中 **6 条成立**，含 2 条静默产出错数据的路径。**v1 的真跑数据与变异结论仍然成立、不撤回**；
> 改变的是裁决与发现清单。第五节逐条记录 v1 里我自己的两处错误。

- 审查者：`qa-m1c3a-1`
- 对象：`master @ f74dc49d4678a7599681ef2dae29f41ee3f2908e`，基线 `4a12794a39ca75f849e1e0cdddb5a21405fce973`
- 语料：`data/hestia-backfill-2026-08-14/articles`，218 篇（金融统计 80 / 社融存量 69 / 社融增量 69）
- 隔离副本：`git worktree --detach f74dc49`，全部探针与变异只作用于副本

## 裁决：**REJECT**

有 high-severity 发现（3 条），且我与 lens 复验后**结论一致、无分歧** —— 按 verdict 规则为 REJECT 而非 CONTESTED。

⚠️ **需要和 Leader 说清楚的分寸**：R1 / R2 / R5 是**可构造但当前语料未触发**的缺陷；
R3 是**现在就成立**的错误分类。我在 v1 用两套独立判据（加总残差）验证过
**现存语料里没有已发生的数据污染**，那个结论复验无误、不撤回。
所以这不是「已经产出了脏数据」，而是「**契约声称已修好的那件事，在另一条路径上没修**，
而那条路径正是 going-forward 的格式」。

---

## 一、复验成立的新发现（全部由我本人重跑，非转述）

### R1 · HIGH · `extract.go:296` `extractTSFFlowSection` 没有作用域切分

TASK-002 的作用域切分只落在 `extractTSFFlowArticle`（独立报告路径）。板块路径仍是
「总量 `mustMatch(tsfFlowTotalRE)` + 分项在整节 `mustMatch`」。`tsfFlowTotalRE` 只认「累计为」
⇒ 累计句被选中，分项在整节唯一命中 ⇒ **取到当月那一套**。

同一段正文（体例逐字取自真实 2023-08 社融增量报告），两条路径结果相反：

```
整篇 extractTSFFlowArticle → err = 社融增量分项 对实体经济发放的人民币贷款 not found   ← 正确
板块 extractTSFFlowSection → err = nil
      tsf_flow_ytd           = 252100   ← 1–8 月累计
      tsf_flow_rmb_loan_ytd  =  13400   ← 8 月单月
      tsf_flow_govt_bond_ytd =  11800   ← 8 月单月
      tsf_flow_equity_ytd    =   1036   ← 8 月单月
```

总量与分项错位 **19×**，每个值单独看都在合法量级内，下游七道闸门无一拦得住。

🔴 **契约层面的问题比代码更重**：CONTRACTS C-1 写着
「⚠️ 这 8 篇现在**响亮失败**而不再静默混装。**失败是修好的表现。**」
—— 这句话对独立报告路径成立，**对板块路径不成立**。而板块路径正是 **v2 月报**走的那条，
v2 月报是 2025-10 起的 going-forward 格式。

**损害条件**：央行把该体例用进 v2 月报的社融增量板块。该体例**不是假想** —— 独立报告里
实际用过 8 篇（2022-07/08/10/11、2023-07/08/10/11）。当前 11 篇 v2 月报均单一口径，故未触发。

### R2 · HIGH · `extract.go:388-391` 锚点缺席即放行，而抽取不依赖锚点

守卫由两个**字面短语**触发（`strings.Index`，`i<0` → `return nil`），
而分部门字段是用 `sectorFlowRE(name)` 在**整节正文**上抽的，与锚点无关。
两者的覆盖面由不同的东西定义，不是同一个集合。

```
锚点 "其中，住户存款" 在正文里: false
checkSectorCaliber        → nil（放行）
extractDepositSection     → err = nil
      deposit_flow_ytd      = 125500   ← 1–4 月累计
      deposit_household_ytd =  -7032   ← 4 月单月
      deposit_corp_ytd      =  -1210   ← 4 月单月
      deposit_fiscal_ytd    =    410   ← 4 月单月
      deposit_nbfi_ytd      =   6716   ← 4 月单月
```

正文取自真实 **2022-04** 报告的存款节，仅补一句真实存在于 2025-04 的累计合计句。

**语料侧证据**（扫全部 80 篇金融统计报告，**含解析失败的**）：
「锚点缺席 ∧ 分部门模板仍命中」的段 = **2**（`2022-04/贷款` 各命中 4 项、`2022-04/存款` 各命中 4 项）。
2022-04 今天不出事，只因它**恰好也没有累计合计句**，在更早一步 `selectRMBCumulativeFlow` 就失败了
—— **安全性来自与守卫无关的巧合**。

⚠️ **这一格有测试，但测不到它**：`extract_test.go:1522`
`TestSectorCaliberStaysSilentWhenNoSectorSegment` 的 fixture 把**锚点和分部门句一起删了**，
于是「紧随其后的 `mustMatch` 会报更具体的那条」在该 fixture 上**恒真**。
它验的是「锚点缺席 ∧ 分部门段真不存在」；可利用的是「锚点缺席 ∧ **分部门句仍在**」。
**而 `checkSectorCaliber` 的注释援引 2022-04 作为该类实例 —— 2022-04 恰属它没覆盖的那一类。**

### R3 · HIGH · `periodAlt` 不认「1-N月」范围前缀 ⇒ 4 篇被贴上假标签并被指示写销

CONTRACTS D 表写：「『1-5月』/『1-8月』这类带范围前缀 → 55 篇月报里**一次都没出现**」。

全语料 `grep -E '(?<![0-9])[0-9]{1,2}-[0-9]{1,2}月'` 命中 **8 篇**。`1-5月` 确实是 0 —— 但
**该断言从一个字面量推广到了整个形态**。4 篇金融统计报告的正文含**累计合计句**：

```
2022-07 金融统计数据报告：1-7月，人民币贷款累计增加14.35万亿元，同比多增5150亿元
2022-08 金融统计数据报告：1-8月，人民币贷款累计增加15.61万亿元
2022-10 金融统计数据报告：1-10月，人民币贷款累计增加18.7万亿元
2022-11 金融统计数据报告：1-11月，人民币贷款累计增加19.91万亿元
```

**下游后果 —— 这一条是现在就成立的错误，不是潜在的**：
`periodAlt` 不认该形态 ⇒ `loanFlowRE`/`depositFlowRE` 看不见这些句子 ⇒
`onlyCurrentMonthFlowSentences` 读成「没有累计句」⇒ 这 4 篇进 `Unsupported`，Err 串写着
「该期报告**只有当月数**、正文无任何期内累计口径的合计句」——**一句关于它们的假话**。

而 CONTRACTS G 明确指示：
「**⚠️「本迭代不解析」那 23 篇不是**（M1c-4 的兜底工作量）—— 那些报告本身没有累计数据，
LLM 也变不出不存在的数。」
⇒ **照办会把 4 期真实可恢复的数据永久写销，而数据就在正文里。**

**同族的第二个覆盖缺口**（同一处 `periodAlt`）：「N月当月」形态。
我用谓词 `当月(人民币|本外币|外币)(存款|贷款)(方向词)` 扫原始 HTML 得 **36 处 / 16 篇**
（lens 用 stripHTML 后的正文 + 数值单位尾巴得 38 处 / 17 篇 —— **口径不同，量级一致**）。
现语料里当月句一律排在分部门段**之后**，故尚未触发；但这意味着
`checkSectorCaliber` 「取最后一个前缀」的正确性**寄生在段落顺序上**，
而 `extract.go:352` 的注释恰恰声称该判据是**结构性**的、与文本顺序无关。

修这条需要两处，缺一不可：`periodAlt` 加范围形态，**且** flowRE 要容忍「贷款」与「增加」之间的「累计」二字。

### R4 · MEDIUM-HIGH · 同期三篇的 `period_type` 不一致（17 个期次）

`required.go:106-107` 写「这两种报告与同期的月报**共享** `(period, period_type)` 业务键」。
逐篇 Parse 后按 period 归组比对，**17 个期次不成立**：

```
2020-03 / 2021-03 / 2022-03 / 2023-03  FIN=q1     FLOW=q1     STOCK=monthly
2020-06 … 2025-06（6 个）              FIN=h1     FLOW=h1     STOCK=monthly
2020-09 … 2024-09（5 个）              FIN=q1_q3  FLOW=q1_q3  STOCK=monthly
2024-03 / 2025-03                      FIN=monthly FLOW=q1    STOCK=monthly
```

成因是央行标题措辞本身不一致（存量报告一律题「YYYY年M月…」），`parseTitle` 忠实透传。

**损害条件**：`store.go` 的业务键就是 `(period, period_type)`。M1c-3b 按它合并会把同一期切成两行 ——
`(2024-09, q1_q3)` 拿到 27+9 字段，而 18 个 `tsf_stock_*` 落进一行**凭空多出的 `(2024-09, monthly)` 幽灵月报**，
永远合不进季报观测。这是本 sprint 新暴露的：TSF 两种此前既不被 `Parse` 认（TASK-007），
`calibrate` 也硬过滤（TASK-010 刚删）。

### R5 · MEDIUM · 截断可静默降级 profile（v1 记 LOW，**升级**）

`sections.go` 的截断守卫 `if !hasFX && periodType != periodTypeMonthly` **豁免 monthly**，
而语料里有 2 篇 `period_type=monthly` 却确实带外汇节的报告。把正文在外汇节标题处截断：

```
2024-03  完整: rule@v1         fields=27  fx_reserve=3.25   截断: rule-monthly@v1 fields=25 err=nil required=25 缺失=0
2025-03  完整: rule@v1         fields=27  fx_reserve=3.24   截断: rule-monthly@v1 fields=25 err=nil required=25 缺失=0
```

⇒ 两个 fx 字段被 completeness 读成 **absent-by-design**，没有任何闸门会响，
写进库的 `extractor` 还是一个**合法**值，事后无从追查。

**这正是那道守卫注释里描述的危害，只是发生在它自己豁免掉的那一类上。**
v1 我按「标签/命名问题」记 LOW，**那个定性是错的** —— 它不是记号错。

### R6 · MEDIUM · `requiredFields` 缺分支 ⇒ completeness 对整个 extractor 家族静默停用

`validate.go:459-462`：`req := requiredFields(...)`；`req == nil` → `CheckSkipped{"unknown_extractor:…"}`。
而 completeness 是该文件自己注释写明的「`obs.Values` 为空时**唯一会 failed** 的闸门」。

`types.go` 的注释说对了后果（「两者不同步会让新模板的期次用错必填集，而那是静默的」），
**但没有机制兜底**：`required_test.go:286` 明确说「逐字写死七个值而不是遍历 `validExtractors`」，
`:300` 只断言 `Len == 7`。⇒ 新增 extractor 时漏改 `requiredFields`，**零条红**。

**零成本闭合**（不需新样本）：加一条遍历 `validExtractors`、要求「要么 `requiredFields` 非 nil，
要么在写明理由的豁免表里」的断言。本包已有两处现成范式
（`TestParseCoversAllKinds` 的豁免表、`TestEveryPeriodTypeHasAnExplicitSupportDecision`）。

---

## 二、v1 已报、仍然成立的发现

| # | 级别 | 一句话 | 位置 |
|---|---|---|---|
| R7 | MEDIUM | 「待解析（**金融统计数据报告**，受支持期次）: 195 篇」是假话，实为 57 金融 + 138 社融 | `calibrate.go:368` |
| R8 | MEDIUM | B 表 `rule@v1 \| 6 节` 错，实测 `5节×2 + 6节×21`，与 A 节自相矛盾 | `CONTRACTS.md:1203` |
| R9 | MEDIUM | 「不入库所以安全」覆盖不到 period 敏感的 `stockContinuityRates` | `calibrate_report.go:305` |
| R10 | LOW | 「共 74 篇」不可复现，旧判据逐字复刻实测 **55** | `sections.go:116` |
| R11 | LOW | `checkPeriodTypeSupported` 拒绝分支成死代码（防线本身仍有效） | `parse.go` |

v1 的 F5（月报 extractor 记 `rule@v1`）已并入 **R5** 并升级。

---

## 三、v1 的真跑与变异结论（复验无误，**不撤回**）

- 四格恒等式 195/23/34/0 = 218；六个 extractor 字段数 27/54/25/52/18/9；
  `m2`=50、`tsf_stock`=79、`tsf_flow_ytd`=52；环比 annual=6 / monthly=68；
  34 篇失败的 19+8+4+3 分解 —— 全部独立复现
- **两套独立判据（加总残差）确认现存语料无已发生的数据污染**：
  分部门 50 篇 max 0.215、社融分项 52 期 max 0.089，距混装量级差两个数量级
- `tsfStockRE` 新旧 A/B：6 处 0→1、**0 处 1→2**、0 处非持平空比率
- 变异 20 个 / 20 KILLED（1 个编译失败的无效变异已识别并重做）
- `go vet` 0；`go test ./...` 0

**但见第五节**：这些证据的**射程**比 v1 声称的窄。

---

## 四、未验证 / 我判定不成立的 lens 声称（如实标注，免得被当成已验过）

| lens 声称 | 我的处置 |
|---|---|
| 2023-05 失败根因是 `一、` 前有前导空格 | **未验证**。我在**原始 HTML** 上扫行首缩进序号得 **0 篇**；lens 的口径是 stripHTML 后的文本，我没复跑。且 CONTRACTS E-2 已声明原始错误会被带进 Err 串 ⇒ 价值较低 |
| `tsfFlowScopeEnd` 右边界只认总量句（F1-c） | **未验证**，我没复现。方向与 R1 同族，建议连同 R1 一起看 |
| `isCumulative` 会把往年 1 月对比句判成累计（F7） | **未验证**。lens 自陈语料中未出现 |
| 「持平 + 数值」会静默吞掉数值（F8a） | **未验证**，构造性；lens 自陈现语料未出现 |
| 「1月份」键让 2 月报窃取 1 月值（F3b） | **未验证**，构造性 |
| 「2025-03 值错一个量级」 | lens **自己撤回**了（用相邻期次交叉核对）。我同意撤回：`*_ytd` 一律年初至今，一季度 == 前三个月 |

---

## 五、v1 里我自己的两处错误（记明，不只是改掉）

### 5.1 「守卫覆盖率 100%」有选择偏差，而我把它写成了无条件结论

v1 §0.4 写：「贷款侧锚点缺失：**0 篇**；存款侧锚点缺失：**0 篇**」。

**我的探针在 `Parse` 失败时 `continue` 了**，所以只统计了**解析成功的 50 篇**。
2022-04 解析失败，因此被排除 —— 而它正是唯一的反例。
改扫全部 80 篇后：**锚点缺席但分部门模板仍命中的段 = 2**。

**错在哪**：不是探针写错了，是**我在报告里丢掉了测量条件**。
「50 篇成功报告里 0 篇」是真的；「守卫覆盖率 100%」是我加上去的。
而**被排除的那批（解析失败的）恰恰是最可能暴露问题的**。

### 5.2 「变异 20/20 KILLED」的射程被我说宽了

20 个变异全部打在**已实现的那条路径上**。没有一个问「**另一条路径有没有同样的守卫**」——
R1（板块路径无作用域切分）与 R2（锚点缺席）都在变异集的射程之外。
v1 §0.7 我还特意写了「其中 5 个是**生成集之外**的」，
**但那 5 个换的是值和判据，没有一个换入口** —— 仍在我自己的因果模型内。

`checkSectorCaliber` 的注释自己记着：D4（锚点缺席 ⇒ 报错）当初 **SURVIVED**。
那条记录就在代码里，我读过那个文件，**没有把它当成线索去追**。

### 5.3 措辞过宽一处

v1 给 Leader 的消息里写「`"1月份"` 全局键：非 1 月报里出现 **0 次**（218 篇）」。
报告正文 §0.6 的两条谓词是精确的（`loanFlowRE`/`depositFlowRE` 捕获、分部门锚点前最近前缀），
**但消息里的概括版丢了谓词**。裸 token 在非 1 月报里实际出现 19 处
（注5「自统计2025年1月份数据起」×17、社融注3 ×2），全部落在最后一节，
因此那两条精确谓词的结论仍然成立 —— 但「出现 0 次」这个说法是错的。

---

## 六、给 Leader 的 fix_items 与归属建议

`reason_class` 建议 **`task_defect`**。

| # | fix_item | 建议归属 |
|---|---|---|
| 1 | `extractTSFFlowSection` 补作用域切分，与 `extractTSFFlowArticle` 同构；或显式声明该路径不支持双口径体例并加断言 | extract.go（TASK-002/006 面） |
| 2 | `checkSectorCaliber`：锚点缺席时不得静默放行 —— 判据应与**抽取的覆盖面**同源（如「本节命中了任何分部门模板」而非「有没有那个短语」） | extract.go（TASK-009 面） |
| 3 | `periodAlt` 加「N-N月」范围形态 + flowRE 容忍「累计」二字；并重跑分流，把那 4 篇移出 `Unsupported` | profiles.go + calibrate.go |
| 4 | 订正 CONTRACTS：D 表那条、C-1「失败是修好的表现」的射程、B 表 `rule@v1` 节数、G 节加「23 篇里有 4 篇实为可恢复」与 `period_type` 不一致 | CONTRACTS.md |
| 5 | `sections.go` 截断守卫的 monthly 豁免要能区分「本就没有外汇节」与「被截断」 | sections.go |
| 6 | `required_test.go` 加遍历 `validExtractors` 的非 nil 断言（零成本，本包有现成范式） | required.go 面 |
| 7 | `calibrate.go:368` 标签去掉 kind 限定词或按 kind 分列，并补一条断言 | calibrate.go |

⚠️ **归属上的一个提醒**：R3 横跨三个任务的产物
（`profiles.go` 面 / `parse.go` 面 / `parse_test.go` 与 `CONTRACTS.md` 面）。
**打回任何单一任务都修不全**，且会把跨任务的文档缺陷记成某个 dev 的返工。
建议 CONTRACTS 订正（fix_item 4）由 Leader 在 `accepted` 前统一处理、**不计入任何 dev 的 `rework_count`**；
只把 1/2/3/5/6/7 这些代码面的项走 `review_fix`。

---

## 七、给复核者的工具警告

lens 报了一条我认为值得转述的环境坑（我全程用 `python3`，未受影响，**未独立复现**）：
本机 macOS 的 `awk -F'\t'` 与 `cut -f` 在含 CJK 的 TSV 上可能给出错误的字段切分
—— 命令跑通、退出码 0、数字看着合理。复核本报告的任何计数请用 `python3`。
