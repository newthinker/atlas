# TASK-004 验证报告 — /prism/fundamental 双轴时间错位修复

- **验证者**：test-agent-13（Reality Checker，默认判定 NEEDS WORK）
- **验证对象**：master @ `a32fbde`（承接时 HEAD，出裁决前复核未变）
- **任务 epoch**：2 · **裁决**：**VERIFIED**
- **日期**：2026-07-27

> 本报告只记录**我自己实际跑出来的输出**。Leader 的 chrome-devtools 探针与
> dev-agent-30/31 的自测结论在文中出现时一律标注来源，**不作为判定依据**。

---

## 0. 结论

**8 条 done_criteria 全部 PASS**，无阻断项；5 条非阻断记录项见 §6。

判定的核心理由不是「dev 说测过了」，而是：**这次修复把失败模式做成了结构上不可能**——
第二条 x 轴被删除，两条序列共用同一组下标。我用三种互相独立的手段确认了这一点
（Go 测试 + 我自写的 node 桩件跑真实模板 JS + 变异测试），结论一致。

---

## 1. 重点核查项 ① —— `post_simplifier_reverification` 是否充分

**背景**：全局规范强制的 code-simplifier 在 `dev_done` **之后**改了
`closesAtPeriodEnds` 的二分查找（`sort.SearchStrings` + `j == n || dates[j] != end`
→ `slices.BinarySearch` + `!quoted`）。**门禁与 dev-agent-30 的整轮变异测试跑的都是改动前的字节。**

### 1.1 我的处置：不去论证等价性，而是重测当前字节

Leader 与 dev 都给出了逐分支等价性论证。**我没有采信，也不需要采信**——
等价性只在「用旧证据背书新代码」时才是必要前提。我把全部验证
（11 个子用例、覆盖率、10 个变异、node 桩件）**直接跑在 `a32fbde` 的当前字节上**，
其中 4 个变异（R-MUT2 / R-MUT5 / T13-MIN / T13-RIGHT）**直接落在被 simplifier 改过的那个函数上**。
因此「改动前后是否等价」对本次判定是无关变量。

（顺带记录：现文件 import 已无 `sort`、`grep 'sort\.'` 无命中，与声称的改动形态一致。）

### 1.2 「手工复现」是否等价于「重跑门禁」——**结论：等价，且此刻严格更强**

逐条拆 `task-completed.sh` 实际做的事：

| 门禁步骤 | 是否被复现 | 依据 |
|---|---|---|
| (a) `go.mod` 存在 / PKGS 取自 task JSON / DOCS_ONLY 判定 | 不受 2 行重构影响 | `packages` 与 `done_criteria` 未变 |
| (b) **scope 漂移校验**（实际改动的 .go 目录 ⊆ 声明 packages） | 结构上不可能被破坏 | simplifier 只碰 `internal/api/handler/web/prism_fundamental.go`，位于已声明的 `./internal/api/handler/web` 内 |
| (c) `go test $PKGS -coverpkg=<三包> -coverprofile=...` exit 0 | **我独立复现** | 见 §2 |
| (d) coverage total ≥ dev_minimum(80) | **我独立复现 81.3%** | 见 §2 |

**关键观察**：此刻**重跑门禁反而更弱**。门禁的 scope 漂移校验取
`git diff --name-only HEAD` + staged + untracked；`a32fbde` 提交之后这个集合里
**已经没有任何本任务的 .go 文件**，(b) 会**空跑通过**。也就是说，要求「重跑门禁」
只能得到一个比 dev 的手工复现更弱的信号。

⇒ **不要求重跑门禁。** 替代方案成立，且我已亲自复算了其中唯一有信息量的两项。

---

## 2. 覆盖率 —— 用门禁的原命令独立复算

```
go test ./internal/api/handler/web ./internal/api ./internal/prism/sankey \
  -timeout 120s -coverpkg="./internal/api/handler/web,./internal/api,./internal/prism/sankey" \
  -coverprofile=<scratchpad>/gate.out
```

```
ok  .../internal/api/handler/web   0.486s  coverage: 21.8% of statements in <三包>
ok  .../internal/api                1.131s  coverage: 46.8% of statements in <三包>
ok  .../internal/prism/sankey       0.664s  coverage: 48.8% of statements in <三包>
total:                                      (statements)   81.3%
```

逐函数：

```
prism_fundamental.go:75   closesAtPeriodEnds   100.0%
prism_fundamental.go:113  PrismFundamental      88.2%
prism_fundamental.go:43   jsonNum              100.0%
prism_fundamental.go:50   jsonNums             100.0%
```

`arcforge.config.json` → `dev_minimum: 80`；`TASK-004.json` **无 `coverage_floor`**（jq 直读为 absent）。
⇒ **81.3% ≥ 80 成立，与 discovery 所报数字逐位一致。**

回归面：`go test ./internal/api/... ./internal/prism/sankey/... -count=1` → **7 个包全 ok，0 FAIL**。

---

## 3. 重点核查项 ② —— 重采样三条边界分支的变异守护

规则：取 ≤ `period_end` 的最近报价，且距期末 ≤ 7 天，否则 NaN（不左延、不右延、不跨空洞）。

| 分支 | 变异 | 结果 |
|---|---|---|
| `endDay.Sub(quoteDay) > maxStale`（7 天窗口） | R-MUT2：`7*24h` → `700*24h` | **KILLED 3/3** @ `^TestClosesAtPeriodEnds$`（红的子用例：恰好一周/超过一周、内部空洞、长度不一致）；SURVIVED 0/3 @ 旧作用域 |
| `j < 0`（期末前无任何报价 ⇒ 不左延） | R-MUT5：`continue` → `j = 0` | **KILLED 3/3** @ `^TestFundamentalPagePriceMissingBeforeFirstQuote$`；**KILLED 3/3** @ `^TestClosesAtPeriodEnds$`；SURVIVED 0/3 @ 旧 |
| `n := min(len(dates), len(closes))` | **T13-MIN（我新增）**：→ `n := len(dates)` | **KILLED 3/3** @ `^TestClosesAtPeriodEnds$`（`index out of range [3] with length 2` panic）；SURVIVED 0/3 @ 旧 |
| 额外：不右延 | **T13-RIGHT（我新增）**：未命中时不再 `j--` | **KILLED 3/3** @ `ClosesAtPeriodEnds` 与 `AlignsPriceToPeriods` |

**三条分支全部有守护。** 但注意：`min` 那条**不在 dev 记录的 8 个变异里**——
测试本身（子用例「dates 与 closes 长度不一致：不 panic」）有判别力，缺的是**变异记录**。见 §6-2。

---

## 4. 重点核查项 ⑤ —— 变异独立重跑（AD-27/28/29 全套护栏）

我自写 harness（`{scratchpad}/mut13/harness.py`），N=3，每条含：
**AD-29 作用域自检**（`go test PKG -list '<regex>'` 计数 > 0）、
**AD-27 对照组**（未变异同环境同作用域实测 0 红）、**施加后可编译校验**、
**sed 命中数校验**（必须恰好 1 处）、**还原后 sha256 逐位比对**。

> ⚠ **我第一版 harness 的作用域自检是坏的**：写成 `go test -run '<regex>' -list '.*'`——
> `-list` 存在时 `-run` 被忽略，于是每个作用域都回报「选中 40 个」。这正是 AD-29
> 记的那类「守卫本身失效却看不出来」。改用 `-list '<regex>'` 后计数变为
> 1/1/1/1/1（五个新用例各 1）、10（旧 TASK-013）、1/5（sankey），读数才有意义。
> **本报告所有作用域数字来自修正后的版本。**

| 变异 | DoD | 新作用域 | 旧作用域 |
|---|---|---|---|
| R-MUT1 页面重新直接吃日频 `view.Closes` | functional[0] | KILLED 3/3 @ `^TestFundamentalPageAlignsPriceToPeriods$` (1) | SURVIVED 0/3 (10) |
| R-MUT2 `maxStale` 7d→700d | functional[0] | KILLED 3/3 @ `^TestClosesAtPeriodEnds$` (1) | SURVIVED 0/3 (10) |
| R-MUT4b 两份模板同时重开第二条 x 轴 | functional[1][2] | KILLED 3/3 @ `^TestFundamentalChartUsesOneSharedXAxis$` (1) | SURVIVED 0/3 (10) |
| R-MUT5 `j<0` 改 `j=0`（左延） | boundary[0] | KILLED 3/3 @ `PriceMissingBeforeFirstQuote` (1) + `ClosesAtPeriodEnds` (1) | SURVIVED 0/3 (10) |
| R-MUT6 `out` 长度跟着价格序列走 | error_handling[0] | KILLED 3/3 @ `^TestFundamentalPageEmptyPriceSeries$` (1) | **KILLED 3/3 (10)** ← 例外 |
| R-MUT7 service 不再带出 `PeriodEnds` | functional[0] | KILLED 3/3 @ sankey `^TestFundamentalPeriodEnds$` (1) | SURVIVED 0/3 (1) |
| R-MUT8 无价格当成 500 | error_handling[0] | KILLED 3/3 @ `^TestFundamentalPageEmptyPriceSeries$` (1) | SURVIVED 0/3 (10) |
| **T13-MIN**（我新增） | error_handling[0] | KILLED 3/3 @ `^TestClosesAtPeriodEnds$` (1) | SURVIVED 0/3 (10) |
| **T13-RIGHT**（我新增） | 重采样/不右延 | KILLED 3/3 @ 两个作用域 | SURVIVED 0/3 (10) |
| **T13-EQ**（我新增，等价候选） | — | **SURVIVED 0/3** | — |

括号内为该作用域实际选中的测试数。所有对照组实测 0 红；所有变异施加后均确认可编译
（否则「红」是编译错误而非判别力）；每条还原后 sha256 与原值逐位一致。
**工作区无残留**：`git status` 与开工时完全一致，无 `.bak` 文件，
`prism_fundamental.go`=`08d000c5…`、两份模板=`0af22766…`、`sankey/service.go`=`14525b79…`。

### 4.1 MUT-6 例外的解释是否成立 —— **成立，且我复现了它的机理**

dev 报「MUT-6 在旧作用域也 3/3 红」并如实记为例外。我复现后看到旧侧的失败是
`--- FAIL: TestPrismFundamentalPage` 伴随 `panic: index out of range [2] with length 2`：
`sampleFundamental()` 有 4 个报告期但只有 2 个收盘价，`out` 长度一旦跟着 `closes` 走就会
在第 3 期越界。**dev 的措辞「旧用例本来就会因为 period_closes 长度塌陷而失败」与实际机理吻合。**

更重要的是这不影响 error_handling[0] 的判别力结论：**R-MUT8 是干净对照**
（新作用域 KILLED 3/3、旧作用域 SURVIVED 0/3），说明
`TestFundamentalPageEmptyPriceSeries` 确实独立守住了「无价格不得当成错误」这条标准。

### 4.2 存活变异 T13-EQ —— 按 AD-27 **不默认归类为等价，而是证明**

变异：删掉报价日期解析失败分支，`if err != nil || endDay.Sub(quoteDay) > maxStale`
→ `if endDay.Sub(quoteDay) > maxStale`。结果 SURVIVED 0/3（两个作用域）。

我写了独立小程序实测而非推断：

```
endDay.Sub(zeroTime) = 2562047h47m16.854775807s   > 7d? true   (= maxDuration)
```

`time.Parse` 失败返回零值 Time（公元 1 年），差值超出 `int64` 纳秒上限被 **饱和** 到
`maxDuration ≈ 292 年`，必然 `> 7d` ⇒ 两种写法对**任何真实报告期**都走同一分支。
**唯一可区分的输入**：`period_end == "0001-01-01"`（该串能被合法解析，差值为 0s）
且同一期的报价日期不可解析——两列同出一张 SQLite 表，现实不可达。
⇒ 判定为**等价变异（已证明，非默认归类）**，不构成测试缺口。

---

## 5. 重点核查项 ③④ —— 划分诚实性与路径 (b) 理由

### 5.1 ③ 「自动化 vs 人工」的划分 —— **诚实，声明属实**

dev 的自我限制原话：「Go 测试断言的是模板**源码文本**里没有 xAxisIndex/xAxis.push——
这与『ECharts 真的只建了一条轴』之间隔着一整个 JS 执行」。

我逐字核对 `TestFundamentalChartUsesOneSharedXAxis`：`os.ReadFile` 两份模板 +
`assert.NotContains/Contains` 字符串匹配，**确实只到文本级**，没有任何 JS 求值。
`automated_regression` 那 5 条我逐条回到测试源码确认存在且断言非空洞。
**没有发现把测不了的说成测了的地方。**

### 5.2 我把这条限制往前推了一步（独立于 dev-agent-30 的桩件）

为了检验「文本级断言之外究竟有没有漏洞」，我自写 node 桩件（`{scratchpad}/mut13/jscheck.js`），
从**磁盘模板**抽出 chart 脚本、桩掉 `echarts`/`document`、注入自造 payload
（8 期，**前 2 期在价格序列之前、第 5 期是序列内部空洞**），检查真正构造出来的 option 对象：

| 模式 | xAxis 条数 | 指标 series | 股价 series |
|---|---|---|---|
| 季度 + 叠加 | **1** | xAxisIndex 未设(→0)、len 8 | xAxisIndex 未设(→0)、yAxisIndex 1、len 8 |
| **年度** + 叠加 | **1** | len 2 | len 2 |
| **柱状** + 叠加 | **1** | len 8 | len 8 |
| 仅指标 | **1** | len 8 | — |
| 季度 + 叠加 + **空价格序列** | **1** | len 8，照常渲染 | len 0，**无 JS 报错** |

逐刻度配对（季度）：
`2024Q1=null 2024Q2=null 2024Q3=100 2024Q4=110 2025Q1=null 2025Q2=130 2025Q3=140 2025Q4=150`
—— 起点前两期为 null（不左延）、**内部空洞 2025Q1 保持 null 且 `connectNulls:false`**。
年度视图 `2024Q4=110 / 2025Q4=150`，与 `annualLabels()` 取 `periods[i+3]` 同下标，**未重新引入错位**。

⇒ 这项证据**属于 dev 归类的「一次性工具核验」而非自动化回归**（我的桩件不在 CI 里、
且用的是桩 echarts 不是真 ECharts）。**dev 把它放在这一档是对的。**
它同时说明：人工清单里的「序列**内部**空洞断点」在**数据层已确认为 null + connectNulls:false**，
剩下待人工确认的只是**视觉呈现**，可以据此收窄该条。

### 5.3 ④ 路径 (b) 的三条理由（dev-agent-31 重建）是否与代码事实相符 —— **三条全部相符**

| 重建的理由 | 代码事实 | 判定 |
|---|---|---|
| 年度粒度下聚合值不属于任何一天 | `state.gran` 有 `'fy'`（`data-g="fy"` 按钮实在）；`annualSum()` 每 4 季合 1 点；`annualLabels()` push `periods[i+3]`（该年最后一季的 fiscal label，**不是日期**） | 相符 |
| 柱状图在不等宽间隔上会重叠 | `state.kind` 有 `'bar'`（`data-k="bar"` 按钮实在）；series `type: state.kind` 直接吃它 | 相符 |
| 日频点会让 dataZoom 缩放时财报点全部离开视野 | `dataZoom: [{type:'slider'},{type:'inside'}]` 实在；71 个报告期 vs 2513 个日频点的密度差是任务描述里已实测的事实 | 相符（反事实论证，与实现无矛盾） |

三条都是**拒绝 (a) 的论据**，本质上是反事实的，无法被「验证为真」；
我能做也只需做的是确认它们**不与代码矛盾**——确认无矛盾。
DoD `functional[1]` 只要求「选 (b) 须说明为何可接受」，理由已给出、
且已显式标注为「重建、如与前任本意有出入以代码事实为准」。**符合要求。**

---

## 6. 非阻断记录项（不影响裁决，建议 Leader 归档时带走）

1. **子用例计数不准**：discovery 与 commit message 均称 `TestClosesAtPeriodEnds`
   「12 个子用例」，`go test -v` 实测为 **11 个**（discovery 自己列举的清单也正好 11 项）。
   全部存在且通过，属文档计数误差。
2. **`min()` 分支不在 dev 的变异记录里**：8 条变异未覆盖 `n := min(len(dates), len(closes))`。
   我补的 T13-MIN 证明该分支**已被现有子用例守住**（KILLED 3/3），
   ⇒ 缺的是记录完整性，不是测试缺口。
3. **作用域计数无法逐条复现**：discovery 记 `1/14/12/1/1/1/1/11`。我能复现 `1`（各新用例）、
   `10`（旧 TASK-013 全部）、`14`（web 包 `.*Fundamental.*`），但 `12`/`11` 对不上——
   **旧作用域的正则未逐字记录**，只写了「旧 TASK-013 全部用例」。
   AD-28 要求作用域是必需字段，建议今后把**正则原文**一并落盘，而不只是描述。
4. **MUT-4 的变异形态未写明改一份还是两份模板**：我先只改磁盘那份，旧作用域即因
   `TestFundamentalTemplateDirsInSync` 也变红（与 dev 记录的 SURVIVED 不符）；
   两份同改（R-MUT4b，保持 parity）才复现出 dev 的读数。dev 的记录**是对的**，
   但形态描述不足以让人盲复现。
5. **T13-EQ 等价变异**（§4.2）已证明，记录在案以免将来有人把它当缺口重开。

---

## 7. 人工验收清单（非我职责，但确认划分合理）

dev 列的 5 项 —— **划分合理**，其中 1 项可据 §5.2 收窄：

| # | 人工项 | 我的意见 |
|---|---|---|
| 1 | dataZoom 真实拖动后仍对齐 | 保留。结构上已不可能错位（单轴 + 等长序列，我在 option 层确认），但字面要求需真实浏览器 |
| 2 | 年度粒度 + 股价 | 保留。我的 node 桩件已过（2/2 对齐），真实 ECharts 未验 |
| 3 | tooltip 同期 | 保留。`trigger:'axis'` 在单轴下天然同刻度，但 formatter 的显示效果需人眼 |
| 4 | 序列**内部**空洞画成断点 | **可收窄**为「仅视觉确认」——数据层我已确认为 null + `connectNulls:false`（§5.2） |
| 5 | 取舍声明可见性 | 保留。文案在模板第 40–41 行实在，是否醒目需人眼 |

## 8. 两项非阻断、未作判定依据

- `detect_changes()` / `impact` 未跑：GitNexus MCP 不在子代理工具集，dev 未擅自
  `npx gitnexus analyze`（属正确克制）。记录在案。
- `.arcforge/{tasks,discoveries}/TASK-004.json` 未提交：本仓惯例由 Leader 在归档提交带走。

## 9. 附：本次验证的可复现产物

- harness：`{scratchpad}/mut13/harness.py`（AD-27/28/29 护栏齐备）
- 变异定义：`{scratchpad}/mut13/muts.py` / `muts2.py` / `muts3.py`
- node 桩件：`{scratchpad}/mut13/jscheck.js`（+ `jscheck_empty.js`）
- 等价性实测：`{scratchpad}/mut13/eqproof/main.go`
- 覆盖率 profile：`{scratchpad}/mut13/gate.out`
