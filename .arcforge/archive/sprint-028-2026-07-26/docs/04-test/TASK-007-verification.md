# TASK-007 验证报告（多期分析引擎）

- 验证者: test-agent-6
- 承接时 assignment_epoch: 1
- 交付 commit: `f846b47`（periods.go 463 行 + 测试 581 行）、`7bf1d5a`（补 Node.Value 断言）
- **判定: VERIFIED（8 条 done_criteria 全部通过）**

## 1. 实跑证据

```
gofmt -l ./internal/prism/sankey/            → 空（Leader 点名项，确认无输出）
GOTOOLCHAIN=local go vet ./internal/prism/sankey/  → ok
GOTOOLCHAIN=local go test ./internal/prism/sankey/ -count=1 -cover
  ok  github.com/newthinker/atlas/internal/prism/sankey  0.464s  coverage: 95.6%
```

`periods.go` 函数级覆盖：`BuildPeriods` 100% / `DefaultSelection` 100% / `changeOf` 100% /
`safeDiv` 100% / `link` 100% / `build` 100% / `significant` 100% /
`BuildSankey` 96.0% / `aggregateFiscalYears` 96.0% / `buildQuarters` 93.8% / `BuildMatrix` 90.9%。

## 2. done_criteria 逐条覆盖矩阵

| # | 完成标准 | 对应测试 | 判定 |
|---|---|---|---|
| F0 | FY 聚合：5 季跨 2 财年、后一年只 1 季 → 只产出 1 个 FY，流量为 4 季之和，Segments 按 key 求和，不足 4 季不产出；非日历财年标签序列 | `TestBuildPeriodsFYAggregation`（7 个流量科目**精确相等**断言 + `Segments` 整个 map 相等 + 三比率由求和值重算）；`TestBuildPeriodsNonCalendarFiscalYear`（MSFT 6 月结：2026Q1 的 period_end=2025-09-30 仍归 FY2026，FY2025 只 2 季不产出） | **PASS** |
| F1 | DefaultSelection 两分支 + AD-15 上限 10 | `TestDefaultSelection/latest_is_mid_year_quarter`（返回 [2026Q1,2026Q2,2026Q3]，gran=q）、`/annual_keeps_latest_ten`（构造 11 个 FY，断言 `Len(sel,10)` 且 `sel[0]=="FY2017"`） | **PASS** |
| F2 | 季度列 Change=去年同期且 kind=yoy；年度 ≥3 期 → cagr；缺对照 → NaN + none | `TestBuildMatrixYoY`（2026Q2 对比 2025Q2，从 allQuarters 反查）、`TestBuildMatrixAnnualChange/three_or_more_periods_use_CAGR`、`TestBuildMatrixMissingComparison` | **PASS** |
| F3 | AD-13 桑基含两个残差节点、各节点入≈出（±1）、tax_other == OI−NI | `TestBuildSankeyBalance`：11 条链路逐值断言 + 4 个中间节点守恒循环 + 全链路非负断言 + 7 个节点 Value 断言 | **PASS** |
| B0 | AD-14 除零：YoY 基数 0/NaN → NaN；CAGR 起点 ≤0 或跨期 <2 → NaN；三比率分母 0/NaN → NaN | `TestBuildMatrixDivisionGuards` 3 子例（含**全矩阵扫描无 Inf**）、`TestBuildMatrixAnnualChange/CAGR_start_not_positive` + `/CAGR_from_negative_to_negative`、`TestBuildPeriodsRatioGuards`。**所有断言均为 `IsNaN` 与 `!IsInf` 成对** | **PASS** |
| B1 | AD-13 负值不画负流而记账；残差 <Revenue×0.5% 省略 | `TestBuildSankeyNegativeFlow`（tax_other=−3b → 链路值为 0 且 `Notes` **点名「税项及其他」**）、`TestBuildSankeyResidualOmitted`（两个残差均省略，主干仍在） | **PASS** |
| B2 | DefaultSelection 三边界 | `/both_empty` →(nil,"q")；`/first_quarter_of_a_fiscal_year` → 返回该 1 期；`/annual_report_filed_but_Q4_derivation_failed` → 走季度分支且**逐期断言 `NotContains "FY"`**（不伪造） | **PASS** |
| N0 (test) | 四函数纯函数无 IO；lang 决定名称；主干硬编码不读模板；全包测试绿（含 TASK-003 既有用例） | import 仅 fmt/math/sort/strconv/strings/prismstore，全文无 os/io/net/http/time.Now；`TestBuildSankeyLang`+`TestBuildMatrixLang` 双向断言；`trunkRows`/`sankeyLabels` 硬编码；全包 95.6% 绿 | **PASS** |

**设计契约一致性**：四个函数签名与 `design-spec §3.2` **逐字一致**；`Graph.Notes` 与 AD-22 记载一致。

## 3. 抽验 dev 的变异测试自查（Leader 指定重心）

dev 自称构造 20 个变异体、杀死 19 个。我**独立重做**了它声称被杀的 6 个，全部复现：

| 变异 | 结果 |
|---|---|
| M1 other_segment 残差节点不画 | 杀死 → `TestBuildSankeyBalance`, `TestBuildSankeyLang` |
| M2 `MaxPeriods` 10→8 | 杀死 → `TestDefaultSelection` |
| M3 去年同期取同年（`year-1` → `year`） | 杀死 → `TestBuildMatrixYoY`, `MissingComparison`, `DivisionGuards` |
| M4 other_opex 漏减 SGnA | 杀死 → `TestBuildSankeyBalance`, `ResidualOmitted` |
| M5 `Node.Value` 只取入流 | 杀死 → `TestBuildSankeyBalance` |
| M6 `Node.Value` 只取出流 | 杀死 → `TestBuildSankeyBalance` |

**dev 的自查可信**。M5/M6 双向被杀，证明 `7bf1d5a` 补的节点值断言确实钉死了 `max(入流,出流)`
——断言表里同时含只出不入的源节点（云业务 40b / 硬件设备 23b / 其他分部 2.585b）与
只入不出的汇节点（净利润 24.7b / 税项及其他 5.3b），少任一类都杀不掉其中一个变异体。

## 4. 「等价变异体」的独立判定（Leader 点名项）——确认是等价，非缺陷

dev 称「去掉空 `FiscalPeriod` 分部行的 `continue`」是等价变异体。我**没有采信结构推理，做了实证**：

- **结构层**：`byPeriod` 是 `buildQuarters` 的局部 map，唯一读点是
  `Segments: byPeriod[r.FiscalPeriod]`，而该处位于 `for _, r := range f` 内、
  `r.FiscalPeriod == ""` 已被 `continue` 跳过 → `byPeriod[""]` 不可能被读。
- **实证层**：写探针喂入**含空标签分部行**的数据（含一个仅存在于空标签行的独有 key `ghost`，
  用来放大差异），分别在原始实现与变异实现上 dump `BuildPeriods` 的 q/fy 两种粒度输出
  **以及**下游 `BuildSankey` 的全部链路值与 Notes，**逐字节 diff 为空**。

**结论：确系等价变异体，`byPeriod[""]` 桶真的无人读取，不是未覆盖的缺陷。**
探针顺带印证了 dev 的另一主张——被跳过的收入未凭空消失，而是落进残差：
`其他分部→收入 = 40`（Revenue 100 − cloud 60），空标签的 777/888 未被计入任何期。

## 5. 我补充的变异体（dev 的 20 个之外）——发现 3 个存活

| 变异 | 结果 |
|---|---|
| M10 FY 聚合去掉「不足 4 季」守卫 | 杀死 ✓ |
| M12 残差阈值 0.5%→5% | 杀死 ✓ |
| M13 tax_other 改用 `IncomeTax` 而非 `OI−NI` | 杀死 ✓ |
| **M8 去掉空 `FiscalPeriod` 的「主表行」`continue`** | **⚠ 存活** |
| **M9 模板分部在本期无数据时不跳过（当 0 处理）** | **⚠ 存活** |
| **M11a/M11b 仅 `GrossMargin` 或仅 `OpMargin` 改为「各季比率平均」** | **⚠ 存活** |

三个存活项**均不构成 DoD 违反**（DoD 未要求这三处），但都是真实的测试薄弱点：

**M8 —— AD-9 场景未被测试。** 分部行的空标签跳过有等价性论证，但**主表行**（`FundamentalRow`）
的同款跳过是**有可观测行为的**：去掉后会产出一个 `Period == ""` 的期列并排在最前，
下游 TASK-009 会渲染出一个无名列。当前无任何测试覆盖该守卫。

**M9 —— 桑基侧的「模板分部在本期无数据」未被测试。** 矩阵侧有覆盖
（`TestBuildMatrixYoY` 断言缺失分部为 NaN 而非 0），但桑基侧的 `if !ok { continue }`
无测试。这是真实场景（分部中途新增/裁撤，或某季缺某分部）。当前行为正确
（跳过 → 收入落进 other_segment 残差），只是无回归保护。

**M11a/M11b —— 一处 dev 自述不准确，需要更正。**
dev 在 discovery 里称「FY 三个比率必须用求和后的分子分母重算……变异测试确认该行为被断言锁住」。
**该属性确实被锁住了，但只由 `NetMargin` 一行锁住，且属数据巧合而非断言设计**：

```
测试 fixture 各季比率：
GrossMargin  各季=[0.6, 0.6, 0.6, 0.6]                 平均=求和后=0.6        差=0
OpMargin     各季=[0.3, 0.3, 0.3, 0.3]                 平均=求和后=0.3        差=0
NetMargin    各季=[0.24, 0.2364, 0.2333, 0.2308]       平均=0.23512 求和后=0.23478  差=3.3e-4
```

`GrossMargin`/`OpMargin` 的各季比率**恒定**，因此「加权」与「取平均」两种算法结果**完全相同**，
这两行断言对该属性是**无区分力的**；只有 `NetMargin` 各季不均匀（差 3.3e-4 > 容差 1e-9）才杀掉变异。
后果：若将来有人只改 `GrossMargin` 的算法，无任何测试会红。
**修法极小：把 fixture 里某一季的 gp/oi 改成非等比即可（如 2025Q3 的 gp 由 72b 改 70b）。**

## 6. 覆盖率逐分支核实

未覆盖的 9 个语句块，逐一定位后分类：
- **M8/M9 对应的两条**（`periods.go:65` 空标签主表行跳过、`:389` 模板分部无数据跳过）——见 §5，真实薄弱点。
- 其余 7 条均为防御性守卫，构造成本高且非死代码：`fiscalYearOf` 的 `len<4`、
  `previousYearLabel` 的 `len<5` 与 `Atoi` 失败、`findPeriod` 的空名、`BuildMatrix` 的空选择、
  `cagr` 的 `IsInf` 兜底、`aggregateFiscalYears` 的空财年跳过。**接受不覆盖。**

## 7. 判定

**VERIFIED** —— 8 条 done_criteria 全部通过；dev 的变异测试自查经 6 项抽验**可信**；
其声称的等价变异体经结构 + 实证双重核实**确系等价**；`gofmt` 无输出。
§5 的三个存活变异体均非 DoD 违反，作为加固建议移交 Leader 判断归属（推荐至少做 M11 的 fixture 微调，
一行改动即可让三个比率行都具备区分力）。
