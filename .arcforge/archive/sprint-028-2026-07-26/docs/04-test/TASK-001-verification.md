# TASK-001 验证报告（EDGAR tag 回退链 + 主干流科目扩展）

- 验证者: test-agent-6
- 交付 commit: c65f9f8（golden 基线，改造**前**）+ d1d86ba（实现）
- 承接时 assignment_epoch: 1
- **判定: VERIFIED（全部 8 条 done_criteria 通过）**

## 1. 实跑证据

```
GOTOOLCHAIN=local go test ./internal/collector/edgar/ -count=1 -cover
ok  github.com/newthinker/atlas/internal/collector/edgar  0.456s  coverage: 93.9% of statements

GOTOOLCHAIN=local go build ./...                                    → exit 0
GOTOOLCHAIN=local go test ./internal/prism/ ./cmd/atlas/ ./internal/storage/prism/ -count=1
  ok internal/prism 0.205s | ok cmd/atlas 1.135s | ok internal/storage/prism 1.453s
gofmt -l internal/collector/edgar/  → 空
GOTOOLCHAIN=local go vet ./internal/collector/edgar/ → exit 0
git status --porcelain internal/collector/edgar cmd/atlas → 空（工作树干净，交付已全部入库）
```

## 2. done_criteria 逐条覆盖矩阵

| # | 完成标准 | 对应测试 | 判定 |
|---|---|---|---|
| F0 | EPS/Equity 首选 tag 缺失时回退命中 | `TestFetchCompanyFactsTagFallback`：`EPSDiluted` 非 NaN 且 == 0.6（回退 tag 原值）、`Equity` 非 NaN == 80000、`DilutedShares` == 24700（回退到 Basic） | **PASS** |
| F1 | EPS 两 tag 皆缺 → NetIncome/DilutedShares 推算（InDelta 1e-9） | 同测试 q2：`assert.InDelta(q2.NetIncome/q2.DilutedShares, q2.EPSDiluted, 1e-9)` + 具体值 0.68。**断言写法与 DoD 要求的 1e-9 精度逐字对应** | **PASS** |
| F2 | 主干流五科目直取与推导 | `TestFetchCompanyFactsMainFlow`：GrossProfit=Revenue−Cost（40000−24000）、SGnA=Selling+GA（5000+3000）、RnD/OperatingIncome/IncomeTax 三项直取逐值断言 | **PASS** |
| B0 | 回退只在首选缺失时生效（负向）；非 NaN 的 EPS 不被推算覆盖；Q4 shares 恒 NaN 故 Q4 EPS 走 FY−ΣQ | `TestFetchCompanyFactsPreferredTagWins`（回退值全设哨兵 **99999**，一旦生效立即被捕获，覆盖 eps/shares/equity/cost 四条链）+ `TagFallback` q1 断言 0.6 未被 15000/24700≈0.607 覆盖 + `TestFetchCompanyFactsQ4EPSNotDerived`（Q4 shares 断言 NaN，Q4 EPS 断言 == 3.0−(0.6+0.7+0.8)） | **PASS** |
| B1 | shares 为 0/NaN 不推算不产生 Inf；SG&A 单侧不求和；毛利负值不钳零；新增 5 字段无法推导保持 NaN | `TestFetchCompanyFactsNoEPSDerivationWithoutShares`（分母 0 与分母 NaN 两例，各自成对断言 `IsNaN` **且** `!IsInf`）+ `MainFlow` q2（`IsNaN(SGnA)` 单侧不求和、`GrossProfit<0` 且值 == 42000−45000 不钳零）+ `Q4EPSNotDerived` 遍历全部季度断言 5 字段皆 NaN | **PASS** |
| E0 | AD-18 golden 回归：9 个 fixture 逐季逐字段一致 | `TestFetchCompanyFactsGolden` 9 子用例 | **PASS**（下文 §3 独立严格证明） |
| N0 (review) | tag 链提为包级变量；basic 近似 diluted 有注释；equity 两 tag 都在时口径不变 | 人工 review：9 个包级 `[]string` 变量（epsTags/sharesTags/equityTags/grossProfitTags/costTags/rndTags/sgnaTags/opIncomeTags/taxTags）照 `revenueTags` 惯例；`sharesTags` 注释明确标注回退项是 **basic** 口径、偏差 <2%；`equityTags` 注释标注含少数股东权益；equity 循环保留「同 end 多条时切片顺序最后一个赢」，回退链只改「取哪个 tag」 | **PASS** |
| N1 (test) | gitnexus impact 无未处置 HIGH/CRITICAL；detect_changes；go test 全绿 | 见 §4，全部**独立复现** | **PASS** |

## 3. AD-18 的独立严格证明（本任务最高风险项）

不采信 dev 的「golden 通过」说法，用 git worktree 做了三重独立证明：

1. **时序真实**：`c65f9f8`（09:26:40，仅含 golden_test.go + 9 份基线，**未动 client.go**）
   早于实现提交 `d1d86ba`。
2. **基线未被篡改**：`git diff --stat c65f9f8 d1d86ba -- internal/collector/edgar/testdata/golden/`
   **输出为空** —— 实现提交一个字节都没碰基线。
3. **基线确实来自改造前的实现**：`git worktree add --detach <tmp> c65f9f8` 后在该工作树跑
   `TestFetchCompanyFactsGolden` → **9 个子用例全 PASS**。即基线与改造前实现自洽，
   排除了「基线由改造后代码生成再回填」的可能。
   而当前 HEAD 用**同一批逐字节未变的基线**同样全 PASS → 改造前后行为在 9 个 fixture 上完全一致。

**dump 的字段完备性核实**：`goldenDump` 覆盖 `quarters=N` + 每季
`FiscalPeriod|PeriodEnd|FilingDate|eps|rev|ni|eq|shares`。我用
`git show c65f9f8:...client.go` 取出改造前的 `QuarterlyFact` 定义比对，
其字段恰为这 8 个 —— **改造前的每一个字段都在 dump 中，无遗漏**，「逐季逐字段」名副其实。

**12 位精度这一让步的独立复现**：dev 声称既有实现有 map 迭代序导致的浮点非确定性。
我在改造前的 worktree 写探针，对 9 个 fixture 各跑 40 次全精度（17 位）比对：

```
NONDET companyfacts_mini:        40 次产生 2 种输出（Q4 eps 0.90000000000000036 / 0.89999999999999991）
NONDET companyfacts_shares_noq4: 40 次产生 2 种输出（同一位置）
NONDET companyfacts_split:       40 次产生 2 种输出（Q4 eps 0.15000000000000013 / 0.14999999999999991）
STABLE 其余 6 个 fixture
```

**该主张属实**：抖动相对量级 ~5e-16，全部落在派生 Q4 上，与 dev 的归因（`applyDuration`
的 map 迭代序影响求和次序）一致，且**非本任务引入**。12 位有效数字把两种输出都渲染为
`0.9`，既消除 flaky 又比真实漂移量级（拆股归一化是 2/4/10 倍级）低 4 个数量级以上。
取舍成立，不是掩盖。

## 4. gitnexus 要求的独立复现

- 本地 runner `.gitnexus/run.cjs`（storage 版本 40）读不了索引（版本 42），报
  `LadybugDB unavailable` —— **dev 的说法属实**，非借口。
- 改用 `npx -y gitnexus@latest`：
  `impact FetchCompanyFacts --direction upstream --repo atlas` →
  3 个候选符号，`maxImpactedCount=3`，`maxRisk=**LOW**`，**无 HIGH/CRITICAL**。
- `detect-changes --scope compare --base-ref d7be007` → 29 files / 45 symbols，
  **Affected processes: 0，Risk level: low**。
- **不依赖 gitnexus 的交叉验证**：`grep` 全仓确认 `FetchCompanyFacts` 生产调用点只有
  `internal/prism/refresh.go:389`（经 `EdgarClient` 接口），`QuarterlyFact` 的跨包消费者
  只有 `internal/prism/refresh.go`。且本次改动对 `QuarterlyFact` **只加字段不删不改**，
  属加性变更 —— LOW 风险独立成立。

## 5. 变异测试（验证断言非空洞）

在 `d1d86ba` 的一次性 worktree 上逐个破坏关键守卫，确认测试**真能抓到**（全部命中预期用例）：

| 变异 | 结果 |
|---|---|
| M1 去掉 `q.DilutedShares != 0` 守卫 | `NoEPSDerivationWithoutShares` **FAIL** ✓ |
| M2 去掉「仅 NaN 才推算 EPS」守卫 | `TagFallback` + `Quarterization` + `SplitNormalization` + **`Golden`** 齐 FAIL ✓ |
| M3 SG&A 允许半个和 | `MainFlow` **FAIL** ✓ |
| M4 毛利钳零 `math.Max(0, …)` | `MainFlow` **FAIL** ✓ |
| M5 `epsTags` 倒序 | `PreferredTagWins` **FAIL** ✓ |
| M6 `equityTags` 倒序 | `PreferredTagWins` **FAIL** ✓ |

还原后复跑 `ok`。M2 额外触发 golden 回归，说明 AD-18 基线确实在守护 EPS 口径。
**结论：本任务的测试有真实咬合力，不是形式存在的空洞断言。**

## 6. 静默失效风险点的定向核查

推导回填用 `costs[key]` / `sgnaParts[i][key]` 与 `quarters` 的 map key 关联。
若两侧 key 格式不一致，推导会**静默不生效**（测试若只覆盖同一路径则看不出来）。
核实：`get` 闭包用 `end.Format("2006-01-02")` 建 key，`quarterValues` 用
`e.end.Format("2006-01-02")` 建 key —— **格式一致**，且 M3/M4 变异被捕获反证推导确在生效。

## 7. 覆盖率逐分支核实

`go tool cover -func` → `FetchCompanyFacts` 94.3%，总计 93.9%（dev 声称的基线 92.2% → 93.9% 属实）。
未覆盖语句块经逐行定位，全部是：
- 日期解析失败的防御性 `continue`（client.go:444/453/537 等，需构造畸形 EDGAR 日期串）
- 生产构造器 `New()`（测试一律走 `NewWithBaseURL`）
- 既有的 `PeriodEnd.IsZero() || FilingDate.IsZero()` 跳过分支

**均非本任务新增逻辑**；新增的推导与守卫分支已由 §5 变异测试证明全部有效覆盖。

## 8. 需 Leader 知悉（非缺陷）

dev 在 discovery `key_findings[3]` 提出：需求原文 plan Task 1 还含 `cmd/atlas/prism.go` 的
Degraded 明细日志改动，本任务未实施，担心漏交付。
**我核实后确认该项不会漏**：它正是 **TASK-004**「refresh Degraded/Failed 明细始终打印到 stdout」
的范围，且已有提交 `ef541b7` 承接。dev 的收窄判断正确，无交付缺口。

## 9. 判定

**VERIFIED** —— 8 条 done_criteria 全部通过，AD-18 高风险项经 worktree 三重独立证明，
关键守卫经 6 项变异测试确认有咬合力，gitnexus 风险结论独立复现为 LOW。无需返工。
