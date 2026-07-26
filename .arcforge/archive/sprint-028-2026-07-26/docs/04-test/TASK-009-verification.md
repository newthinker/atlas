# TASK-009 验证报告（sankey Service 装配层）

- 验证者: test-agent-6
- 承接时 assignment_epoch: 1
- 交付 commit: `7c4fb15`
- **判定: VERIFIED（8 条 done_criteria 全部通过）**

## 1. 实跑证据

```
gofmt -l ./internal/prism/sankey/ ./internal/storage/prism/   → 空
GOTOOLCHAIN=local go vet（两包）                               → ok
GOTOOLCHAIN=local go test（两包）-count=1 -cover
  ok  internal/prism/sankey    0.861s  coverage: 96.1%   （基线 95.6%）
  ok  internal/storage/prism   1.732s  coverage: 81.7%   （基线 81.3%）
全仓：go build ./... ok；go test ./internal/prism/... ./cmd/atlas/ 全 ok
```

`service.go` 函数级：`Analyze` **100%**、`roeTTM` 100%、`filterRange` 100%、`truncate` 100%、
`langMap` 100%、`NewService` 100%；`Fundamental` 90%、`rows` 80%（未覆盖块全部是
store 方法返回 error 的传播路径——fake 只让 `InstrumentID` 可注入错误，其余三个方法恒返回 nil error）。

**既有用例零删除**：periods_test 16→18（+2 为 M8/M9 新增）、template_test 9→9 未动、
service_test 新增 17。TASK-003/007 的 25 个既有测试**一个未减**。

## 2. done_criteria 逐条覆盖矩阵

| # | 完成标准 | 对应测试 | 变异验证 | 判定 |
|---|---|---|---|---|
| F0 | from/to 皆空走 DefaultSelection；Granularity 一致；Periods 每项含 Period/Graph/Metrics；**AD-22 Notes 透传** | `TestAnalyzeDefaultSelection`、`TestAnalyzePassesThroughGraphNotes`（构造 NetIncome>OperatingIncome 使 tax_other 为负，断言 Notes 非空**且点名「税项及其他」**，并断言所有 link 非负） | **S1 装配处丢弃 Notes → 杀死** | **PASS** |
| F1 | from/to 范围过滤；granularity=fy 为财年聚合 | `TestFilterRange`、`TestAnalyzeGranularityAndRange` 4 子例（含端点包含性与 FY 求和 274b） | **S7 端点改排他 → 杀死** | **PASS** |
| F2 | AD-15 上限 10；Truncated 正负向；**生产引用常量 / 测试写死 10** | `TestTruncateToMaxPeriods`（12→10 且 ≤10 时 Truncated=false 负向）、`TestAnalyzeTruncates` | **S2 常量 10→8 → 杀死**；**S8 Truncated 恒 false → 杀死** | **PASS** |
| F3 | ErrNoTemplate 可被 errors.Is 判定 | `TestAnalyzeNoTemplate`、`TestAnalyzeUnknownSymbol`、`TestAnalyzeTemplatedButNotInStore`（负向：模板存在时**不得**误报 ErrNoTemplate） | **S6 `%w`→`%v` 丢 wrap → 杀死** | **PASS** |
| F4 | Fundamental 返回季度序列 + ROE(TTM) + Dates/Closes；无数据 → prismstore.ErrNotFound | `TestFundamental`、`TestFundamentalNoData`、`TestFundamentalUnknownSymbol` | **S9 无数据改返空视图 → 杀死**；**W12 Periods 反序 → 杀死** | **PASS** |
| B0 | AD-14 ROE 在 Equity 为 0/NaN 时 NaN 不 Inf；Template.Lang 按 lang | `TestROETTM/zero_or_NaN_equity`（**IsNaN 与 !IsInf 成对**）、`TestLangMap`、`/english_labels` | **S5 分母取首季 → 杀死**；**W9 langMap 忽略 lang → 杀死** | **PASS** |
| B1 | 新 5 列全 NULL 时不报错、保持 NaN、无 Inf | `TestAnalyzeWithUnrefreshedColumns`（断言 NoError + IsNaN + 全 link 与全 Matrix 行无 Inf） | — | **PASS** |
| N0 (test) | 全包绿（含 TASK-003/007 既有测试） | 见 §1；既有 25 个用例零删除 | — | **PASS** |

**AD-15 两侧口径**（我上轮更正、Leader 采纳的那条）**双向核实通过**：
- 生产代码 `service.go:206` `return lastN(ps, MaxPeriods), len(ps) > MaxPeriods` —— 引用常量，无字面量；
- 测试 `service_test.go:66/78` 用字面量 `10`，**未**写 `require.Len(got, MaxPeriods)`。
- 实证：S2 把常量改 8 后 `TestDefaultSelection`+`TestTruncateToMaxPeriods`+`TestAnalyzeTruncates`
  三处齐红 —— 证明断言确实独立于被测代码，锚定作用成立。

## 3. 动了 TASK-002 已验收代码的语义等价核实（Leader 点名）

`PriceSeries` 内联的 id 解析改为复用新增的 `InstrumentID`。**三层核实，均通过**：

1. **SQL 逐字相同**：两侧均为 `SELECT id FROM instrument WHERE symbol=?`，无附加条件增减。
2. **错误包装格式串逐字相同**：`fmt.Errorf("%w: %s", ErrNotFound, symbol)` 与
   `fmt.Errorf("prism: query symbol %s: %w", symbol, err)` 两条在新旧实现中完全一致。
3. **运行时实证**（最强证据）：在 `7c4fb15^` 建 worktree，用同一探针分别跑新旧实现，
   覆盖 `NOSUCH` / 空串 / 大小写不匹配 `known` / 命中 `KNOWN` 四种输入，
   比对 `dates`、`closes`、`err.Error()`、`errors.Is(err, ErrNotFound)`：

```
旧: sym="NOSUCH" err="prism: symbol not found: NOSUCH" isNotFound=true
新: sym="NOSUCH" err="prism: symbol not found: NOSUCH" isNotFound=true
（四种输入逐行 diff 为空）
```

**结论：语义完全等价，TASK-002 已验收行为零回归。** 无需还原成两份独立实现——
消除重复是正当简化，且 dev 主动披露而非静默混入。

## 4. 我在 TASK-007 提的三项补测：验收通过

**M11 按 Leader 强调的标准验收**——不是「改了 fixture」而是「改后断言真有区分力」。
fixture 已把 2025Q3 由 `gp=72b, oi=36b` 改为 `70b / 34b`，各季比率不再等比。
**直接重跑我原来那两个变异体**：

| 变异 | TASK-007 时 | 现在 |
|---|---|---|
| 仅 `GrossMargin` 改为各季平均 | ⚠ 存活 | **杀死 → TestBuildPeriodsFYAggregation** |
| 仅 `OpMargin` 改为各季平均 | ⚠ 存活 | **杀死 → TestBuildPeriodsFYAggregation** |

**M8/M9 同样验收通过**：`TestBuildPeriodsSkipsUnlabeledFundamentalRow` 与
`TestBuildSankeySegmentMissingThisPeriod` 分别杀死对应变异体。

## 5. 变异测试总计 25 项：19 杀 6 存活

抽验 dev 声称的接线缺陷修复（`BuildMatrix` 第四参传 `quarters` 而非 `sel`）：
**S3 改回传 `sel` → 被 `TestAnalyzeMatrixComparesAgainstQuartersOutsideRange` 杀死**，修复有效。

### ⚠ 6 个存活变异体——均非 DoD 违反，但都是真实测试缺口

Leader 提示「dev 连续两轮在『fixture 数据分布决定断言区分力』上出错，多戳几下」。
我按接线视角设计了 12 个变异体，**发现 6 个存活，且集中在 dev 自己指认的主缺陷形态（接线错误）上**：

| # | 变异 | 后果 | 性质 |
|---|---|---|---|
| **W2** | `SegmentRows` 传 0 而非 id | 分部数据全空 → 桑基退化为「全部收入都是 other_segment 残差」 | **真缺口** |
| **W4** | `BuildSankey` 传 `nil` 模板 | 同上：图上不再有任何分部节点 | **真缺口**（与 W2 同根） |
| **W5** | `PeriodView.Metrics` 恒取 `sel[0]` | 多期时每期都带第一期的指标 | **真缺口** |
| **W7** | `gross_profit` 映射到 `OperatingIncome` | 前端毛利曲线显示的是营业利润 | **真缺口** |
| **W8** | `rnd` 映射到 `SGnA` | 同类字段错配 | **真缺口** |
| **W10** | `lang` 未传给 `BuildMatrix` | lang=en 时矩阵行标签仍是中文 | **真缺口** |
| （W3） | `PriceSeries` 的 `from` 传非空 | — | fake 忽略 `from`，**非生产缺陷**，是 fake 的局限 |

**根因分析（三处，各不相同）**：

- **W2/W4 同根**：`Analyze` 路径**没有任何断言检查图里存在分部链路**。
  `TestAnalyzeDefaultSelection` 只断言 `NotEmpty(Graph.Links)`——主干链路恒在，故恒真；
  `/english_labels` 断言的 `hasNode(Graph, "Revenue")` 也是主干节点。
  BuildSankey 的分部处理在 TASK-007 的 `TestBuildSankeyBalance` 有单元级覆盖，
  但**「分部确实流过了装配层」这件事本身无人断言**。
  → 补一行 `assert.True(hasNode(got.Periods[0].Graph, "云业务"))` 即可同时杀掉 W2 与 W4。

- **W5**：多期用例（`explicit quarterly` 4 期）只断言 `viewNames`（标签），
  单期用例才断言 `Metrics.Revenue` 且下标恰为 0 —— 于是「每期带自己的 Metrics」这件事
  在多期情形下从未被验证。
  → 在 4 期用例里加一条逐期 `Metrics.Revenue` 断言即可。

- **W7/W8**：`TestFundamental` 只断言了 `revenue` 与 `roe_ttm` 两条序列，
  `fundamentalMetrics` 里其余 **7 条**（gross_profit / operating_income / net_income /
  rnd / sganda / income_tax / eps_diluted）全部无断言。字段错配会静默把错数据送到前端。
  → 表驱动逐 key 断言一遍即可。

**这与 M11 是同一类问题的另一种形态**：M11 是「fixture 数值巧合让断言失去区分力」，
这 6 个是「断言压根没覆盖到该字段/该路径」。共同点是**光看测试代码都像已经测了**——
`TestFundamental` 看起来在测 Fundamental、`TestAnalyzeDefaultSelection` 看起来在测 Analyze，
只有逐字段/逐路径做变异才暴露出覆盖的边界。

## 6. 判定

**VERIFIED** —— 8 条 done_criteria 全部通过；AD-22 Notes 透传经变异证实真打通；
AD-15 两侧口径双向核实；动到 TASK-002 的重构经运行时实证语义等价、零回归；
我在 TASK-007 提的三项补测按「区分力」标准验收通过。

§5 的 6 个存活变异体**均不在 DoD 范围内**，故不阻断放行，作为加固建议移交 Leader 判断归属。
按价值排序推荐：**W7/W8（一个表驱动断言，覆盖 7 条无断言序列，性价比最高）
→ W2/W4（一行，同时杀两个）→ W5（一行）→ W10（一行）**。
