# TASK-012 验证报告 — /prism/sankey 财报桥页面（含 G4 四环链收口）

- **验证者**: test-agent-9
- **判定对象 HEAD**: `90efe36`（承接时记录，出裁决前复核**未变**）
- **对照基线**: `e415da9`（父提交，TASK-019 终态）
- **结论**: **PASS（verified）**

---

## 0. 本轮口径：从严（leader 已确认）

dev-agent-22 中断前**代码与测试由它同时写出、从未有人见过 RED 阶段**——
**两者互相印证不构成证据**；dev-agent-23 是在既成绿灯上补证据。
故本轮**每条 `verify_by: test` 的 DoD 都要求有对应变异从「绿」翻「KILLED」**，
不接受「测试通过」。这是唯一能补上缺失 RED 的方式。

dev 自报的 7 个变异一律不采信，以下全部为我独立重跑（**我自建 12 个变异体**）。

---

## 1. Harness 与对照组（TASK-019 教训已内化）

- **用 `go test` 直跑，不预编译测试二进制** —— 从构造上消除我上一轮那个 cwd 类 bug
  （预编译二进制在错误 cwd 下会让 9 个相对路径用例恒红，产生假 KILLED）。
- **对照组先跑，且与变异组跑在完全相同的执行环境/包集合下**：**红 0/5**。
- 三重防护：字面替换**命中数须恰为 1**（模板类须恰为 2 = 两份副本各 1）/
  `go vet` + `go build ./...` 编译守卫 / `git checkout` 还原后 `git diff --quiet`。
  **每一轮收尾均 `TREE CLEAN`**（12 轮全部通过）。

---

## 2. 变异矩阵 —— **12/12 全部 KILLED，各红 10/10**

| ID | 变异内容 | 结果 | 杀手用例 |
|---|---|---|---|
| **ME1** | **引擎侧** `graphBuilder.link`（`periods.go:614`）不再记录 note | **KILLED 10/10** | e2e **API 环 + 页面环双双红**；另 `TestBuildSankeyNegativeFlow`、`TestAnalyzePassesThroughGraphNotes` |
| **ME2** | 页面 handler 丢弃 `Graph.Notes` | **KILLED 10/10** | e2e **仅页面环红，API 环全绿**；`TestPrismSankeyNotesRenderedAsFootnote` |
| ME3 | 两条 404 文案改成相同 | KILLED 10/10 | `TestPrismSankeyThreeDistinct404s/no_template` |
| **ME4** | 三条文案**互异但各含他者关键词**（近义句攻击） | **KILLED 10/10** | `TestPrismSankeyThreeDistinct404s`（两两交叉校验段） |
| ME5 | 模板不再渲染 Notes（两份副本同步） | KILLED 10/10 | e2e 页面环 + `TestPrismSankeyNotesRenderedAsFootnote` |
| **ME6** | 模板**无条件**印固定 footnote（**恒真靶**） | **KILLED 10/10** | e2e/`正常数据不产生该 footnote` + `对偶:无 note 时页面不凭空造 footnote` |
| ME7 | 路由退回 `if deps.PrismStore != nil` 块内（catch-all 回归） | KILLED 10/10 | `TestSankeyPageRouteRegistration` 两个子用例 |
| ME8 | 去掉 typed-nil 防护 | KILLED 10/10 | `TestSankeyPageRouteRegistration/service 为 nil` |
| ME9 | 模板去掉 `id="sankey-grid"` | KILLED 10/10 | `TestPrismSankeyPage`、`TestNewHandlerDiskModeRendersSankey` |
| ME10 | 去掉「堆叠柱」视图切换按钮 | KILLED 10/10 | `TestPrismSankeyPageControls` |
| ME11 | `pageNames` 里去掉 `prism_sankey.html`（模板漂移回归） | KILLED 10/10 | 6 个用例同时红，含 e2e 页面环 |
| **ME12** | `symbol` 不透传给 service（硬编码 `"MSFT"`） | **KILLED 10/10** | **仅 e2e 红**（见 §3.3） |

**存活变异：无。** 故 AD-27 第 4 条（存活禁止默认归为语义等价）本轮不适用。

---

## 3. 三个核心判断（我独立确认，非引用 dev）

### 3.1 G4 确实贯通到**引擎**，不是「页面能显示传给它的东西」
这是本任务的核心，也是 leader 特别提醒「变异要打在引擎侧」的点。

- **ME1 打在链条起点**：`internal/prism/sankey/periods.go` 的 `graphBuilder.link()`，
  即「`value < 0` 时记一条 note」这段引擎逻辑本身。
- **结果：e2e 的 API 环与页面环双双变红。** ⇒ 页面上那条 footnote 的**因果来源确实是引擎**。
- **e2e 全链我逐行实读，确认无 fake、无手工注入 Notes**：
  真 YAML（写入 `t.TempDir()`）→ 真 `sankey.LoadTemplates` → 真 sqlite `prismstore.Open`
  → 真 `UpsertInstrument`/`UpsertFundamentals`（`NetIncome=350 > OperatingIncome=300`）
  → 真 `sankey.NewService` → 真 `NewServer` → 真 `srv.mux.ServeHTTP`。
  **这正是 TASK-011 里 G4 残留的解药**：彼处 `TestSankeyNotesPassThrough` 是把 note
  字面塞进 fake，因此「引擎真的会在该输入下产生这条 note」从未被验证过。

### 3.2 dev 的 M6 发现属实，且我认为它值得进 AD-27
- **ME2（页面 handler 丢 Notes）：只有页面环红，API 环全绿。**
- 与 ME1（两环双红）对照，精确刻画出：**「端到端用例」实为两条独立的链**
  （引擎→API→JSON、引擎→装配→页面 HTML），**必须两环都断言才算贯通**。
- 推论（我补充）：**只断言其中任一环的「端到端测试」会给出虚假的安全感**——
  它能证明引擎到该环是通的，却对另一环的回归完全无感。这正是 G4 那类残留的一般形态。

### 3.3 e2e 提供了单测**拿不到**的判别力（一个具体证据）
`TestPrismSankeyPage` 有一句 `assert.Equal(t, "MSFT", f.symbol, "symbol 必须透传到 service")`。
但该用例请求的正是 `/prism/sankey/MSFT`——**把 handler 里的 symbol 硬编码成 `"MSFT"`，
这句断言照样绿**（ME12）。
只有 e2e 因为使用了 `BRIDGE` / `NORMAL` **两个不同的 symbol** 才把它杀掉。
⇒ **ME12 是仅由 e2e 捕获的变异**，实证了这条端到端用例不是「锦上添花的重复覆盖」。

### 3.4 对偶断言有牙（否则前三条会被架空）
**ME6（模板无条件印固定 footnote）被两个对偶子用例精确杀死**。
若无对偶，「页面恒印该 footnote」会让 ME1/ME5 的**页面环**断言失去意义——
恒真的断言在变异下也恒绿。对偶是这套证据链的地基，实测有效。

---

## 4. Done Criteria 覆盖矩阵

| # | 完成标准 | verify_by | 对应测试 | 变异证据（我独立重跑） | 判定 |
|---|---|---|---|---|---|
| F0 | 有模板 200，页面含 `id="sankey-grid"` 与 `/static/echarts.min.js` | test | `TestPrismSankeyPage` | **ME9 KILLED 10/10** | **PASS** |
| F1 | 未配置模板的 symbol 返回 404 | test | `TestPrismSankeyThreeDistinct404s/no_template` | **ME3/ME4 KILLED** | **PASS** |
| F2 | 粒度/语言/视图切换、报告期范围、矩阵 table 容器 | test | `TestPrismSankeyPageControls`（16 个锚点） | **ME10 KILLED 10/10** | **PASS** |
| F3 | **AD-22** Notes 渲染为可见 footnote 而非丢弃 | test | `TestPrismSankeyNotesRenderedAsFootnote` + 对偶 | **ME1/ME2/ME5 KILLED；ME6 恒真靶被对偶杀死** | **PASS** |
| F4 | **G4 端到端**贯通四环链，**不得任何一环用 fake 或注入 Notes** | test | `TestSankeyEndToEndEngineNoteReachesPage`（3 子用例） | **ME1 两环双红（打在引擎侧）**；ME2 单环红；**ME12 仅 e2e 捕获** | **PASS** |
| B0 | prism provider 为 nil 时 404 且不 panic | test | `TestSankeyPageRouteRegistration`（2 子用例） | **ME7/ME8 KILLED 10/10** | **PASS** |
| E0 | 三种 404 文案**互不相同**，须两两交叉校验 | test | `TestPrismSankeyThreeDistinct404s` | ME3 KILLED；**ME4 近义句攻击亦 KILLED** | **PASS** |
| NF0 | 两处模板同步且 diff 无差异；两处 pages 列表均加入；既有测试通过 | test | `TestSankeyTemplateDirsInSync`、`TestNewHandlerDiskModeHasPrismTemplates`（实存 `prism_web_test.go:169`） | 两份模板 `diff` **完全相同**；**ME11 KILLED 10/10** | **PASS** |
| NF1 | JS：`resp.data` 解包(AD-8)、IIFE+ES5、Tailwind、id 定位、clamp[0.5,1]+footnote、PNG 导出 | **review** | 人工审查（§5） | N/A | **PASS** |
| NF2 | `go test ./internal/api/... -count=1` 全绿 | test | 全量 | N/A | **PASS** |

---

## 5. NF1 人工审查（verify_by: review）

| 要求 | 核验方式 | 结果 |
|---|---|---|
| **ES5**（var/function，无 let/const/箭头/class/spread） | `grep -nE "\blet\b\|\bconst\b\|=>\|\bclass\b\|\.\.\."`（排除 `class=` 属性） | **无命中，合规** |
| IIFE 包裹 | 读 `:90 (function () { ... })` | ✓ |
| **AD-8 `resp.data` 解包** | `:155 latest = (resp && resp.data) \|\| {}` | ✓ |
| **裸 404（非 JSON 响应体）防护** | `:142 try { resp = JSON.parse(raw.text); } catch (e) { resp = null; }`，并在 `:147` 兼容 `resp.data.error` 与 `resp.error` 两种错误结构 | ✓ **这正是 API 侧 catch-all 残留的缓解**（见 §7） |
| 统一比例尺 clamp [0.5, 1] | `:250-255 cellHeight()`：`ratio>1→1`、`ratio<0.5→0.5` | ✓ |
| clamp 的 footnote 说明 | `:77` 页面可见文案「各期图高按…缩放并截断在 50%~100%」 | ✓ |
| 单期大图 / 堆叠柱 PNG 导出 | `:300`、`:326` 各一个 `toolbox.feature.saveAsImage` | ✓ |
| id 定位 + Tailwind 原子类 | 全篇 `getElementById`，无 class 选择器依赖 | ✓ |

### 5.1 `renderSingle` 判空修复（code-simplifier 发现的真 bug）—— **无法测试，我读代码确认**
- 缺陷形态：`renderSingle` 在 `periods` 为空时于 `:296 if (!p) { return; }` 提前返回，
  而 `echarts.init` 在 `:297`（**返回之后**）⇒ `singleChart` 保持 `null`；
  切到「单期大图」时对 `null` 调 `.resize()` → TypeError → 整页停摆。
  触发条件真实可达：**有模板但该范围内无报告期**。
- 修复：`:350 renderSingle(d); if (singleChart) { singleChart.resize(); }`，
  `:387` 的 window resize 处亦判空。**逻辑正确。**
- **我另核了一处看似不对称的地方并确认无问题**：`:351 stackChart.resize()` **未判空**。
  这是安全的，因为 `renderStack` 的**第一条语句**就是
  `:314 if (!stackChart) { stackChart = echarts.init(...) }`，在任何 early return 之前，
  故调用后 `stackChart` 必非 null。**两者的不对称由两个函数的结构差异决定，是正确的，不是漏改。**
- 降级行为正确：periods 为空时页面显示空的单期容器，不崩。

---

## 6. 回归与门禁

- **`go test ./... -count=1` 全仓全绿**（无任何 `FAIL` / `--- FAIL`）。
- `go build ./...` 干净；`go vet ./internal/api/...` 无输出。
- **`gofmt`**：`internal/api/handler/web/prism_web_test.go` 不合规，
  但**该文件不在本次提交的 7 个文件内**，且**在 BASE(`e415da9`) 上同样不合规**
  ⇒ **既有问题，非本任务引入，不计入判定**（建议另开清理）。
  **本次实际改动的 5 个 `.go` 文件 `gofmt -l` 全部为空。**
- 提交内容干净：7 个源文件，**无 `.arcforge/` 或 `.claude/` 混入**（`git show --name-only` 核实）。
- **覆盖率（不作判定依据，仅确认不倒退）**：5 次采样**完全稳定，零波动**
  | 包 | BASE | TARGET | 变化 |
  |---|---|---|---|
  | `internal/api/handler/web` | 63.3% | **67.8%** | **+4.5pp** |
  | `internal/api` | 34.9% | **52.2%** | **+17.3pp** |
  两包均**上升**，无倒退。`coverage_floor=60` 是人类决策的验收边界，不作我的判定依据。

---

## 7. 已知残留（**不计入本任务判定**，按 DoD scope 单列）

1. **API 侧 catch-all（比 web 侧更严重，已在 TASK-013 DoD）**：
   我核实 `internal/api/server.go` **无 `HandleFunc("/api/")` 自有 catch-all**，
   故 `deps.PrismSankey == nil` 时 `/api/prism/sankey` 与 `/api/prism/fundamental`
   同样落到 `"/"` → 200 + dashboard HTML，前端 `r.json()` 会拿到 HTML 而抛异常。
   **discovery `known_gaps[2]` 对此的描述准确**（dev 提交前主动补正了这条，
   理由是「verifier 会读这份 discovery，留着会让它按一个低估的 blast radius 去验收」——
   **该判断正确，这条确实影响我的验收范围认定**）。
   **本任务只修 web 侧，符合 scope，不因此退回。**
   页面侧 JS 的 `JSON.parse` try/catch（§5）**部分缓解**了该问题的前端表现。
2. **web 侧既有三页**：`/prism/board`、`/prism/compare`、`/prism/detail/` 在 prism 关闭时
   仍返回 200 dashboard（本任务实测坐实 2896 字节）。**不在 scope**，建议另开任务。
3. **页面 JS 无自动化测试**：Go 无 JS 引擎，浏览器里 JS 再 fetch 并重绘的那一步无法驱动。
   **dev 的应对是正确的**：把 Notes **服务端渲染**一份，使该链在无 JS 时也成立
   （AD-22 的信息因此不依赖脚本执行）；模板 4 处最易回归点靠静态文本守卫
   `TestPrismSankeyJSUnwrapsDataAndGuardsBare404` 兜底。**已在 `known_gaps` 如实声明。**
4. **`renderSingle` 判空修复无自动化覆盖**（§5.1），只有代码注释防回归。**已如实声明。**

---

## 8. 合规缺口（非阻断，不作判定依据）

CLAUDE.md 要求提交前跑 `detect_changes()`，但 **dev 的工具集内无 GitNexus MCP**，故未执行；
它**没有擅自跑 `npx gitnexus analyze`**（仓库级重操作）——**在我看来这是正确的克制**：
超出授权范围的重操作比漏一个只读检查风险更大。**属环境限制而非失职，记录在案。**

---

## 9. 裁决

**verified**。

理由（按本轮从严口径）：
- **10 条 DoD 逐条 PASS**，其中 **8 条 `verify_by: test` 全部有变异从绿翻 KILLED**，
  1 条 `review` 已人工逐项审查，1 条为全量绿。
- **12 个变异体全部 KILLED（各红 10/10），对照组 0/5，存活为零**，
  harness 三重防护每轮 `TREE CLEAN`。
- **G4 收口达标且证据是最强形态**：ME1 打在**引擎侧**，e2e **两环双红**，
  链路的因果方向被钉死；e2e 全链实读确认无 fake、无注入。
- **dev 的 M6 发现经我复现属实**（ME2 只红页面环、API 环全绿），
  且我另发现 **ME12 仅由 e2e 捕获**，两者共同证明这条端到端用例有单测拿不到的判别力。
- 对偶断言经 ME6 恒真靶验证有牙，整套证据链无「恒真」地基问题。
- 全仓回归绿、覆盖率两包均升、提交干净。

残留（§7）均**在 scope 之外且已在 discovery 如实声明**，其中 API 侧已由 leader 写入 TASK-013 DoD。
