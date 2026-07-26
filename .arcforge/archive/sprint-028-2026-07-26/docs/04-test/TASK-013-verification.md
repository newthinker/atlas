# TASK-013 验证报告 — /prism/fundamental 财务趋势页 + prism API catch-all 修复

- **验证者**: test-agent-9
- **判定对象 HEAD**: `ba40d70`（承接时记录，出裁决前复核**未变**）
- **对照基线**: `90efe36`（父提交，TASK-012 终态）
- **结论**: **PASS（verified）**

---

## 0. 口径：从严（延续 TASK-012，leader 已确认）

代码由 **dev-agent-22 / 23 / 24 三任接力**完成（前两任中断），
**没有任何单一 agent 完整经历过 RED→GREEN**——测试与实现互相印证不构成证据。
故**每条 `verify_by: test` 的 DoD 都要求有对应变异从绿翻 KILLED**。
dev 自报结果一律不采信，以下为我独立重跑（**我自建 16 个变异体**）。

---

## 1. Harness 与对照组

- 用 `go test` 直跑（不预编译测试二进制）——从构造上消除 TASK-019 那个 cwd 类 bug。
- **对照组先跑，且与变异组跑在完全相同的执行环境与包集合下：红 0/5。**
- 三重防护：字面替换**命中数须恰为 1**（双模板类须恰为 2）/ `go vet` + `go build ./...`
  编译守卫 / `git checkout` 还原后 `git diff --quiet`。**每轮收尾 `TREE CLEAN`。**
- **编译守卫本轮实际触发一次**：我第一版 C10 把 `if math.IsNaN(...)` 改成 `if false`，
  导致 `math` 成为未使用导入 → 判 `COMPILE_FAIL exit 4`、**排除计数**，未误记为 SURVIVED。
  重做为 `return nil → return 0.0`（保持 `math` 被使用）后才计入。

---

## 2. 变异矩阵（16 个）

### 2.1 KILLED —— 12 个，各红 10/10

| ID | 变异 | 杀手用例 |
|---|---|---|
| **A1** | 两个 prism API 退回 `if ... != nil` 块内（不注册，catch-all 缺陷回归） | `TestPrismAPIsAnswerJSON404WhenDisabled`（两子用例）+ `TestSankeyRoutesRegistration/registered even without a service` |
| **A3** | **状态码仍 404**，仅把 JSON 体改成纯文本 `http.Error` | 同上 —— **「只断状态码不够」已被正确防住** |
| **A4** | fixture 去掉 `TemplatesDir`（「绿但盲」回归） | **反盲守卫 `requireDashboardCatchAllPresent` 真的报警** |
| C1 | 页面去掉 `id="fund-chart"` | `TestPrismFundamentalPage` 等 3 个 |
| **C2a** | 模板插入 `\|\| 0` | `TestPrismFundamentalJSHasNoNullToZeroCoercion` |
| C3 | prism_board 去掉 `/prism/fundamental/` 入口 | `TestPrismBoardLinksToFundamental` |
| C4 | 去掉「年度」粒度切换 `data-g="fy"` | `TestPrismFundamentalPageControls` |
| C5 | 两条 404 文案改成同一句 | `TestPrismFundamentalDistinct404s/no_data` |
| **C5b** | 两条文案仍不相等、但**互含对方关键词**（近义句攻击，沿用 TASK-012 的 ME4 手法） | `TestPrismFundamentalDistinct404s`（交叉断言段） |
| **C7** | 掏空 `isNull`（**连 `=== null` 字面一起删**） | `TestPrismFundamentalJSHasNoNullToZeroCoercion` —— **但见 §3，这是偶然** |
| **C10** | Go 侧 `jsonNum` 把 NaN/±Inf 映射成 **0** 而非 null | `TestPrismFundamentalNullsSurviveToPage/±Inf 同样映射为 null` |
| C11 | `pageNames` 去掉 `prism_fundamental.html`（模板漂移） | 5 个用例同时红 |

### 2.2 SURVIVED —— 4 个（**其中 1 个是设计上的归因对照，3 个是已声明的 AD-7 缺口**）

| ID | 变异 | 结果 | 性质 |
|---|---|---|---|
| **C2b** | 插 `\|\| 0` **且**停用守卫 | **SURVIVED 0/10** | **归因对照，预期如此**（见 §4） |
| **C7b** | **保留 `=== null` 字面**、只让表达式恒假 | **SURVIVED 0/10** | **AD-7 缺口（真实）** |
| **C8** | `annualSum` 改「跳过缺失季」 | **SURVIVED 0/10** | **AD-7 缺口（真实）** |
| **C9** | `growth` 去掉 `base === 0` 防护 | **SURVIVED 0/10** | **AD-7 缺口（真实）** |

**按 AD-27 第 4 条，存活变异不默认归类为「语义等价」**：上述 3 个经我逐一推理，
**全部是「测试有洞」而非等价变异**（危害推演见 §3.2）。

---

## 3. ★ 核心复核：dev 的 C7b 边界**划得对**（leader 点名）

### 3.1 根因，我独立核实
守卫的**正向**断言是 `assert.Regexp(regexp.MustCompile("===\\s*null|!==\\s*null"), page)`，
属**文本匹配**。我 grep 全模板确认 `=== null` 字面的分布：

| 行 | 内容 | 含 `=== null` 字面？ |
|---|---|---|
| **:59** | `function isNull(v) { return v === null \|\| v === undefined; }` | **是（唯一一处）** |
| :82 / :93 / :110 / :122 / :162 / :212 | 全部是 `isNull(...)` **调用** | 否 |

⇒ **C7 变红是「删掉了那段字面」的偶然结果，不是语义验证。**
⇒ **C7b 保留字面、只让表达式恒假 → 守卫全绿（SURVIVED 0/10）。**

**结论：静态守卫的强度止于文本级。** 它只能拦住
「`|| 0` / `Number(null)` / `+null` 这类把 null 静默变 0 的**写法**出现在模板里」，
**拦不住语义被掏空**。

### 3.2 C7b 的危害是重大的（故不可归为等价变异）
`isNull` 恒 false 后：`divide`/`growth` 不再返回 null，JS 里 `null/5 === 0`、`5/null === Infinity`；
`annualSum` 的 `sum + null` 按 0 处理。⇒ **正是 DoD functional[4] 明令禁止的
「把 null 当 0 画出一条假的归零曲线」**，而静态守卫**全程绿灯**。

### 3.3 判定
**dev 的边界划得对，且 dev 没有拿 C7 的偶然变红去粉饰缺口。**
discovery 的 `KF-5` 与 `honest_gap_statement` 对这件事的描述**准确**。
**(b)(c) 的语义确实没有任何自动化守护——C7b/C8/C9 三个存活是 AD-7 降级的精确代价，不是疏漏。**
> dev 给出的理由是「人工验收清单存在的理由必须准确」——我认同：
> 若让 C7 的偶然变红充当语义证据，人工验收清单就会**少列**三项该由人看的东西，
> 那才是真正的损失。

---

## 4. C2a/C2b 归因对（dev 提醒的第二点，我复现）

| 变异 | 结果 | 说明 |
|---|---|---|
| C2a：插 `\|\| 0` | **KILLED 10/10** | 守卫红了 |
| C2b：插 `\|\| 0` **且**停用守卫 | **SURVIVED 0/10** | **证明红的是守卫本身，不是别的断言** |

**dev 的说法成立。** 单有 C2a 只能证明「插了 `|| 0` 会红」，
不能排除「红的是另一条碰巧也失败的断言」；C2b 把归因钉死。
**这是「守卫有效」的完整证据形态**，与 TASK-011 的对偶断言、TASK-019 的对照组是同一类方法。

---

## 5. Done Criteria 覆盖矩阵

| # | 完成标准 | verify_by | 对应测试 | 变异证据 | 判定 |
|---|---|---|---|---|---|
| F0 | 有数据 200，含 `id="fund-chart"` 与 `/static/echarts.min.js` | test | `TestPrismFundamentalPage` | **C1 KILLED** | **PASS** |
| F1 | 无 `fundamental_q` 数据 → 404 | test | `TestPrismFundamentalDistinct404s/no_data` | **C5/C5b KILLED** | **PASS** |
| F2 | 7 指标切换、折线柱状、季/年、dataZoom 容器 | test | `TestPrismFundamentalPageControls`（14 锚点 + dataZoom） | **C4 KILLED** | **PASS** |
| F3 | prism_board 卡片含 `/prism/fundamental/` 入口 | test | `TestPrismBoardLinksToFundamental`（两份模板都查） | **C3 KILLED** | **PASS** |
| F4 | **null 传播 (a)**：含 null 序列不得当 0 画；须覆盖 `roe_ttm` 前 3 季必然 null | test | `TestPrismFundamentalNullsSurviveToPage` + 对偶 | **C10 KILLED**（NaN/±Inf → 0 被抓） | **PASS** |
| B0 | prism 未启用(nil) → 404 且不 panic | test | `TestPrismFundamentalDistinct404s/not_enabled` | **C5 系列 KILLED** | **PASS** |
| B1 | **null 传播 (b)(c)**：按 AD-7 降级为人工验收；**可验证部分 = 静态守卫，且守卫本身须有变异** | **review** | `TestPrismFundamentalJSHasNoNullToZeroCoercion` | **C2a KILLED + C2b 归因对**（守卫有效）；(b)(c) 语义缺口经 **C7b/C8/C9 实证**并如实入清单（§6） | **PASS** |
| E0 | API catch-all：无条件注册 + JSON 404；**负向断言**钉住；与 TASK-011 语义相反须同步更新 | test | `TestPrismAPIsAnswerJSON404WhenDisabled`、`TestSankeyRoutesRegistration`（**已重写为相反语义**） | **A1/A3/A4 全 KILLED** | **PASS** |
| NF0 | 两处模板 diff 无差异；`pageNames` 加入；`TestNewHandlerDiskMode*` 通过 | test | `TestFundamentalTemplateDirsInSync`、`TestNewHandlerDiskModeRendersFundamental` | 两份模板 **逐字节相同**（fundamental 与 board 均 `diff` 空）；**C11 KILLED** | **PASS** |
| NF1 | JS：双轴、增速现算、年度 JS 端聚合不新增请求、风格对齐 | **review** | 人工审查（§7） | N/A | **PASS** |
| NF2 | `go test ./internal/api/... -count=1` 全绿（含 TASK-012 既有测试） | test | 全量 | N/A | **PASS** |

---

## 6. ★ 人工验收清单（**供 TASK-015 直接引用**）

> **以下三项由 test-agent-9 在 TASK-013 验证中以变异测试实证为「无自动化守护」。
> 它们是 AD-7 降级的精确代价，不是疏漏——须由人工在浏览器中验收。**
> 判定对象 `ba40d70`；每项均已实测「存活 0/10」（同执行环境对照组 0/5）。

**M-1 `isNull` 的语义（变异 C7b：保留 `=== null` 字面、表达式恒假 → 存活 0/10）**
- **为何无守护**：静态守卫的正向断言 `assert.Regexp("=== null|!== null")` 是**文本匹配**；
  而 `=== null` 在整份模板中**只出现在 `isNull` 定义那一行**（`prism_fundamental.html:59`）。
  守卫只能拦「把 null 变 0 的写法出现在模板里」，**拦不住 `isNull` 语义被掏空**。
- **人工验收**：打开 `/prism/fundamental/{symbol}`，切到 **ROE(TTM)** 指标（季度粒度）。
  **前 3 个季度必须是曲线断开的缺口，不得是一条落到 0 的线**；
  悬停这 3 个点应显示 `—` 而非 `0`。

**M-2 4 季聚合的「整年作废」策略（变异 C8：改为「跳过缺失季」→ 存活 0/10）**
- **为何无守护**：`annualSum` 是纯 JS，Go 测试无法执行（`go.mod` 无 JS 引擎）。
- **实现语义（我已读码确认）**：`out.push(missing ? null : sum)`（`:115`），即**整年作废**。
- **dev 拒绝「跳过缺失季」的理由（我认同）**：跳过会用 3 个季度加出一个**看起来完全合理**
  的「年营收」——只偏低约 25%，读图人无从察觉，**并伪造出一个假的同比下滑**；
  整年作废则在图上留下**可见缺口**。原则：**宁可显示不出来，也不能显示一个错的。**
- **人工验收**：造一个**缺任一季度**的标的，切到**年度**粒度。
  **该年必须整年缺失（图上断开），不得出现一个偏低但看似正常的柱/点。**

**M-3 增速现算的分母防护（变异 C9：去掉 `base === 0` 判断 → 存活 0/10）**
- **为何无守护**：`growth` 是纯 JS，同上。
- **实现语义（我已读码确认）**：
  `if (isNull(base) || isNull(vals[i]) || base === 0) { out.push(null); }`（`:93`）。
- **人工验收**：造一个**基期为 0**（或基期缺失）的序列，切到 **营收 YoY / 净利 YoY**。
  **页面不得出现 `Infinity`、`-Infinity` 或 `NaN` 字样**，应显示 `—`。

> **补充说明（给 TASK-015 起草人）**：变异 **C7 会 10/10 变红，但不可用作 (b)(c) 有守护的证据**——
> 它红是因为删掉了 `=== null` 字面，属文本级偶然捕获。**判别证据是 C7b（保留字面、恒假）存活。**

---

## 7. NF1 人工审查（verify_by: review）

| 要求 | 核验 | 结果 |
|---|---|---|
| 双轴（左轴指标 / 右轴价格） | `id="price-toggle"` + 双 yAxis 配置 | ✓ |
| 增速由 JS 从季度序列**现算** | `growth(vals, lag)`（`:88-98`），非服务端预算 | ✓ |
| 年度切换为 JS 端 4 季聚合、**不新增请求** | `annualSum`/`annualLast`，全页**无 `fetch(`**（并由断言钉死） | ✓ |
| **AD-8 不适用的前提被钉住** | 整份序列服务端内联；`assert.NotContains(page, "fetch(")` —— 一旦有人加 fetch 该断言变红，提醒补 `resp.data` 解包 | ✓ **这是正确的处理方式**：把「前提」而非「结论」写成断言 |
| 派生计算遇缺失一律产出 null | `divide`/`growth`/`annualSum`/`annualLast` 四处均显式判 `isNull` | ✓（**语义无自动化守护，见 §6**） |
| 风格对齐既有 prism 模板 | IIFE + ES5 + Tailwind 原子类 + id 定位 | ✓ |

---

## 8. 回归与门禁

- **`go test ./... -count=1` 全仓无任何 `FAIL`**（含 TASK-012 既有测试）。
- `go build ./...` 干净；`go vet ./internal/api/...` 无输出。
- **两份模板逐字节一致**：`prism_fundamental.html`、`prism_board.html` 的 `diff` 均为空。
- **`gofmt`**：本次提交改动的 **6 个 `.go` 文件全部 clean**。
  `internal/api/handler/web/prism_web_test.go` 不合规，但**未被本次提交触碰、且在 BASE
  (`90efe36`) 上已不合规** ⇒ **既有问题，不计入判定**（leader 已另开清理）。
- 提交干净：10 个文件，**无 `.arcforge/` 或 `.claude/` 混入**。
- **覆盖率（不作判定依据，仅确认不倒退）**：5 次采样**完全稳定，零波动**
  | 包 | BASE | TARGET | 变化 |
  |---|---|---|---|
  | `internal/api/handler/web` | 67.8% | **70.5%** | +2.7pp |
  | `internal/api` | 52.2% | **53.7%** | +1.5pp |
  | `internal/api/handler/api` | 52.4% | 52.4% | 持平 |
  **无倒退。** `coverage_floor=58` 是人类决策的验收边界，不作我的判定依据。

---

## 9. 裁决

**verified**。

理由：
- **11 条 DoD 逐条 PASS**；其中 9 条 `verify_by: test` **全部有变异从绿翻 KILLED**，
  2 条 `review` 已人工逐项审查。
- **16 个变异体**：12 KILLED（各红 10/10）、1 归因对照按设计存活（C2b）、
  **3 个存活是已声明的 AD-7 缺口**、1 个我自己的坏变异被编译守卫排除。对照组 0/5。
- **E0（API catch-all）修复到位且证据完整**：A1（退回不注册）、
  **A3（状态码仍 404 仅改文本体）**、**A4（fixture 退化）** 全部 10/10 红——
  其中 A3 正是我上一轮预告的「只断状态码不够」，A4 证明**反盲守卫真的会报警**。
- **★ leader 点名复核的 C7b 边界：dev 划得对。** 我独立实测 C7 KILLED / **C7b SURVIVED**，
  并 grep 核实根因（`=== null` 仅存在于 `isNull` 定义行）——
  **静态守卫止于文本级，(b)(c) 语义无自动化守护，三个存活是 AD-7 的精确代价而非疏漏。**
  dev 未用 C7 的偶然变红粉饰缺口，该判断诚实且正确。
- §6 人工验收清单已按可直接引用的形式写好，**TASK-015 须原样收录 M-1/M-2/M-3 三项**。
