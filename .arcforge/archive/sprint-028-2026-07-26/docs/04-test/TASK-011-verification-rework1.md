# TASK-011 返工轮验证报告（rework1 / epoch=4）

- **验证者**: test-agent-9
- **判定对象 HEAD**: `7059066`（承接时记录，出裁决前 `git rev-parse HEAD` 复核**未变**；
  `git status --porcelain` 对三个相关目录为空，无未提交改动混入判定对象）
- **本轮范围**: `done_criteria.functional` 中带【W…】【G2】前缀的 **6 条**（第 6–11 项）。
  原有 10 条已由 test-agent-8 首轮全部 PASS（`04-test/TASK-011-verification.md`），**本轮不重验**。
- **结论**: **PASS（verified）**

---

## 0. 判别标准（沿用 test-agent-8 在其 checkpoint 中为本轮指定的标准）

> 重跑 P1/P2/P5，确认每个都从 **SURVIVED → KILLED**。
> **绿色测试只能证明没有回归，翻转才证明缺口被堵上。**

我没有采信 dev 的任何自报计数（dev-agent-20 在 commit 后卡死，**未经其自身二次确认**）。
下述全部数据来自我**独立重跑**，并采用**双 worktree 对照**：把「返工前 SURVIVED」也实测出来，
而不是引用前任结论——这样「翻转」才是我自己观测到的事实，而非两份报告的拼接。

- BASE worktree = `3912294`（返工前父提交）
- TARGET worktree = `7059066`（判定对象）
- 两棵树均由 `git worktree add --detach` 建立，起始 `git status --porcelain` 为空。

---

## 1. 核心结论：7 个变异 **全部 SURVIVED → KILLED**

| 变异 | 注入点 | BASE(`3912294`) | TARGET(`7059066`) | 杀手用例（用例层面） |
|---|---|---|---|---|
| **W2** | `service.go` `rows()` `SegmentRows(id)`→`(0)` | **SURVIVED** 红 0/3 | **KILLED 红 10/10** | `TestAnalyzeDefaultSelection` |
| **W4** | `service.go:118` `BuildSankey(p,tmpl,lang)`→`(p,nil,lang)` | **SURVIVED** 红 0/3 | **KILLED 红 10/10** | `TestAnalyzeDefaultSelection` |
| **W5** | `service.go:119` `Metrics: p`→`Metrics: sel[0]` | **SURVIVED** 红 0/3 | **KILLED 红 10/10** | `TestAnalyzeGranularityAndRange/explicit_quarterly` |
| **W10** | `service.go:122` `BuildMatrix(...,lang,...)`→`(...,"",...)` | **SURVIVED** 红 0/3 | **KILLED 红 10/10** | `TestAnalyzeGranularityAndRange/english_labels` |
| **W7**(=前轮 P1) | `service.go` `gross_profit`→`r.OperatingIncome` | **SURVIVED** 红 0/3 | **KILLED 红 10/10** | `TestFundamentalCoversEveryMetricSeries/gross_profit` |
| **W8**(=前轮 P2) | `service.go` `sganda`→`r.RnD` | **SURVIVED** 红 0/3 | **KILLED 红 10/10** | `TestFundamentalCoversEveryMetricSeries/sganda` |
| **G2**(=前轮 P5) | `api/sankey.go:145` 删 `"period_end": m.PeriodEnd` | **SURVIVED** 红 0/3 | **KILLED 红 10/10** | `TestSankeyResponseShape` |

BASE 侧 7/7 在**四包全量**（`./internal/api/... ./cmd/atlas/ ./internal/prism/sankey/`）下
`--- FAIL:` 行数均为 **0**，确认返工前确为真实缺口（含 test-agent-8 未覆盖的 W2/W4/W5/W10 四项，
此前只有 dev-agent-19 的自报，本报告首次给出独立实测）。

**test-agent-8 指定的翻转标准（P1/P2/P5）已达成，且额外四项同样达成。**

---

## 2. 重点核查：9 行表是否**每一行**都有区分力（leader 点名）

### 2.1 行数与键名
`fundamentalMetrics`（`service.go:161`）实读确认为 **9 条**，dev 的表恰为 9 行且键名逐一对上：
`revenue / gross_profit / operating_income / net_income / rnd / sganda / income_tax / eps_diluted / equity`。
测试内另有 `require.Len(t, want, 9, "…表少一行就是漏测一条序列")` 自锁行数。
**未复现「建 7 行表漏掉 eps_diluted/equity」的缺口。**

### 2.2 fixture 是否真建了，而非断言零值
`fq()`（`periods_test.go:36`）实读确认签名只有 7 个数值参数、**不设 `EPSDiluted`/`Equity`**，
dev「因此自建 fixture」的理由属实。自建 fixture 实测取值
`EPSDiluted: 1.7/1.9`、`Equity: 207e9/217e9`，**均非零**。

### 2.3 逐行探针（我追加，非 dev 提供；均 N=5）
| 探针 | 结果 |
|---|---|
| P-rev `revenue`→`GrossProfit` | **KILLED 5/5** |
| P-oi `operating_income`→`NetIncome` | **KILLED 5/5** |
| P-ni `net_income`→`RnD` | **KILLED 5/5** |
| P-rnd `rnd`→`SGnA` | **KILLED 5/5** |
| P-tax `income_tax`→`NetIncome` | **KILLED 5/5** |
| **P-eps** `eps_diluted`→`NetIncome` | **KILLED 5/5** |
| **P-eps0** `eps_diluted`→`return 0` | **KILLED 5/5** |
| **P-eq** `equity`→`Revenue` | **KILLED 5/5** |
| **P-eq0** `equity`→`return 0` | **KILLED 5/5** |

加上 W7/W8，**9 行逐行皆有牙，无一行恒真**。
**P-eps0 / P-eq0 是针对 leader 所虑「断言了零值」失效模式的直接证伪**：若该模式成立，
把字段改成 `return 0` 应当存活；实测被杀，说明断言锚的是 fixture 里的非零具体值。

### 2.4 键集合断言双向有牙
| 探针 | 结果 |
|---|---|
| P-key 键名 `eps_diluted`→`eps_dil`（序列"消失"） | **KILLED 5/5** |
| P-extra 额外插入一条 `bogus` 序列 | **KILLED 5/5** |

`assert.ElementsMatch` 对**缺失**与**多余**都能捕获，符合 dev「只断值不断集合则整条序列被删可能无人发现」的设计意图。

### 2.5 表是否被静默跳过（表驱动测试的典型失效）
`-v` 实跑：9 个子用例 `=== RUN` 与 `--- PASS` 逐个出现，`--- PASS: .../` 计数 = **9**。
**不存在「一条都没跑却全绿」。**

---

## 3. 追加探针：断言强度是否只够挡 dev 自己那一个写法

| 探针 | 意图 | 结果 |
|---|---|---|
| P-W2b `segs = segs[:1]` | 分部数据只剩 1 行（节点仍在） | **KILLED 5/5** — 断言校的是 `linkValue==230e9` 聚合值，非仅节点存在 |
| P-W4-seg 传入 `Segments` 为空的模板 | 模板非 nil 但无分部 | **KILLED 5/5** — 非仅校验 `tmpl != nil` |
| P-W5b `Metrics: sel[len(sel)-1]` | 错位但不是 `sel[0]` | **KILLED 5/5** — 非只挡 `sel[0]` 一个写法 |
| P-W10-seg 只掐断 `displayName(s, lang)` | 分部行取名路径 | **KILLED 5/5** |
| P-W10-main 只掐断 `pick(lang, r.zh, r.en)` | 主干行取名路径 | **KILLED 5/5** |
| P-G2b `period_end: m.Period` | 键在但值错 | **KILLED 5/5** — 断的是具体日历日 `"2026-06-30"` |

W10 的两条取名路径经 P-W10-seg / P-W10-main **各自独立**被杀，
证实 dev「主干行走 `pick()`、分部行走 `displayName()`，故各钉一条」的说法成立，
而非一条断言顺带覆盖。

---

## 4. AD-27 合规性

### 4.1 我的 harness 三重防护（针对本 sprint 独立发生三次的假阳性）
1. **命中数校验**：字面替换要求命中数**恰为 1**，0 次（锚点写错）或 >1 次（误伤）均 exit 3 判 harness 错误。
   施加后另验 `git diff` 非空。7 个变异在 BASE 与 TARGET 两棵树中**命中数均为 1**（已预先逐一打印核对）。
2. **编译守卫**：`go build ./...` 显式验编译。AD-27 #2 所述「编译失败输出同样没有 `--- FAIL:` 行，
   与变异存活形态完全相同」——**本轮实际触发两次**：`P-drop`、`P-W10b` 是我自己写坏的探针，
   被守卫判为 `COMPILE_FAIL` 并**排除计数**，未误报为 SURVIVED。守卫确实在工作。
3. **还原校验**：一律 `git checkout -- <file>` 还原（不用文件备份，从根上避免路径不一致），
   还原后 `git diff --quiet` 校验，每轮收尾打印 `FINAL TREE CLEAN`——**三轮 harness（BASE/TARGET/PROBE 系列）全部通过**。

### 4.2 存活变异分类（AD-27 #4）
本轮 TARGET 侧 **零存活**，无需分类。BASE 侧 7 个存活已由本轮翻转证明**全为真实缺口**（非语义等价）——
这是比推理更强的证据：等价变异不可能被补测杀死。

### 4.3 采样次数（AD-27 #5）
TARGET 主变异 N=10、探针 N=5，**红/N 均为满值（p=1.0）**。
dev 在 discovery 中给出的 p=1.0 理由（本批断言不依赖 Go map 遍历序：`hasNode`/`linkValue`/`rowByKey`
遍历 slice，序列断言基于有序 rows，G2 是 JSON map 精确键查找）**我认可**，
且我的独立采样（10/10、5/5，共 7+15 个变异体）与之一致，未观测到任何波动。

### 4.4 对照组（negative control）
未施加任何变异时，4 个杀手用例各跑 10 次：**红 0/10**。
缺这一步则「10/10 全红」也可能是用例本身恒红。dev 亦自建了对照组（20 次红 0 次），方法正确。

### 4.5 一处**非阻断**的字面偏差
DoD 元条目与 AD-27 #2 原文写「施加前先 **`go vet`** 确认可编译」，dev 的 harness 用的是
**`go build ./...`**（我的 harness 同样用 go build）。就「区分编译失败与变异存活」这一目的而言，
`go build` 是更直接的编译检查，**实质等价或更强**，且 AD-27 的两次实际触发均被正确拦下。
另我在 TARGET 基线独立跑过 `go vet ./internal/api/... ./cmd/atlas/ ./internal/prism/sankey/`，**无输出**。
**记为表述偏差，不影响判定。**

---

## 5. 返工轮 Done Criteria 覆盖矩阵

| # | 完成标准（本轮 6 条） | verify_by | 对应测试 | 变异证据（我独立重跑） | 判定 |
|---|---|---|---|---|---|
| F5 | 【W7/W8】`fundamentalMetrics` **全部 9 条**序列表驱动断言 | test | `TestFundamentalCoversEveryMetricSeries`（9 子用例） | W7/W8 各 KILLED 10/10；**逐行探针 9/9 KILLED**（含 P-eps0/P-eq0 证伪零值断言）；P-key/P-extra 证键集合双向有牙 | **PASS** |
| F6 | 【W2/W4】`Analyze` 路径断言图中存在分部链路 | test | `TestAnalyzeDefaultSelection`（`hasNode("云业务")` + `linkValue(云业务→收入)==230e9`） | W2 KILLED 10/10；W4 KILLED 10/10；P-W2b、P-W4-seg 加强探针亦 KILLED | **PASS** |
| F7 | 【W5】多期下 `PeriodView.Metrics` 下标必须被断言 | test | `TestAnalyzeGranularityAndRange/explicit_quarterly`（逐期 `Metrics.Period==Period` + Revenue 对齐） | W5 KILLED 10/10；P-W5b（`sel[len-1]`）亦 KILLED | **PASS** |
| F8 | 【W10】`lang=en` 下矩阵行标签须为英文 | test | `TestAnalyzeGranularityAndRange/english_labels`（`revenue`→"Revenue"、`cloud`→"Cloud"） | W10 KILLED 10/10；P-W10-seg / P-W10-main **两路径各自独立** KILLED | **PASS** |
| F9 | 【G2】`period_end` 断言 | test | `TestSankeyResponseShape`（`assert.Equal("2026-06-30", metrics["period_end"])`） | G2 KILLED 10/10；P-G2b（键在值错）亦 KILLED | **PASS** |
| F10 | **返工验证要求**：五条对应变异须被实际杀死；按 AD-27 记「N 次中红几次」并注明用例层面；施加前验可编译；不得以「测试通过」替代「变异被杀」 | test | 元条目 | 7/7 翻转 SURVIVED→KILLED（§1）；discovery `mutation_log_ad27` 明确注明「N/M 报的是**用例层面**，每次独立 `go test -run <用例> -count=1`，不用 `-count=N`」；含 before_evidence / negative_control / harness_selfcheck 三块 | **PASS**（§4.5 一处字面偏差，非阻断） |

**原有 10 条（F0–F4/B0/NF0/NF1）**：按 leader 指示不重验，援引 test-agent-8 首轮报告。
我另实测确认**未回归**：八包全量绿（§6），生产代码 `git diff` 全程未改（本次提交仅两个 `_test.go`，+122/-0）。

---

## 6. 回归与门禁（均我独立实测）

- **八包全量**（`./internal/api/... ./cmd/atlas/ ./internal/prism/sankey/ -count=1`）：
  TARGET **全绿**；BASE 亦全绿 → 无回归。
- `go build ./...`：干净。`go vet`（三包）：无输出。
- `gofmt -l`（两个改动文件）：**空**。
- **覆盖率（不作判定依据，仅确认未倒退）**：四包连采 3 次读数**完全稳定**
  `52.4% / 34.9% / 74.2% / 97.5%`（`internal/api` / `cmd/atlas` 口径见下），
  与 BASE 采样**逐位相同** → **零倒退**。
  dev 自报的聚合 71.6% 我**未独立复算**（该数字由 `dev_done` 门禁产出，且 `coverage_floor=70`
  是人类决策、明确不作判定依据）；但 dev「与返工前**持平**、本次纯加断言只增判别力不增覆盖行」
  的说法与我的逐包实测**一致**，属**如实标注而非虚报**。

---

## 7. Discovery 核查

`.arcforge/discoveries/TASK-011.json`：
- 新增 `rework_epoch4_by_dev_agent_20` ✓（含 scope / production_code_untouched / key_correction /
  fixture_note / mutation_log_ad27 / why_each_assertion_has_teeth / files_modified_rework / verification_rework）
- `ad14_two_layer_evidence` 保留 ✓
- `contract_for_downstream` 保留 ✓ —— **更正一处核查方法**：它**不是顶层键**，
  嵌在 `interfaces_exposed.contract_for_downstream` 下。用 `jq 'has("contract_for_downstream")'`
  查会得到 `false` 而误判为「被删」。我用路径遍历确认其实存。**下游（TASK-012）勿被顶层 `has()` 误导。**

---

## 8. 遗留（不阻断本任务，供 leader 与下游知悉）

1. **G4 残留延续**（test-agent-8 已记、leader 已收下）：四环链 007→009→011→012
   **仍无任何单一端到端用例**，各环只在自己边界受测。本轮补测全部落在 service 层与 handler 层各自边界内，
   **未改变**这一残留。TASK-012 验证时应留意。
2. **AD-27 #2 文本建议**：AD-27 与本任务 DoD 写的是「先 `go vet`」，而实践中（dev 与我）都用
   `go build ./...`，后者对「是否可编译」更直接。建议在 `project-template/` 侧把 AD-27 措辞
   放宽为「`go build`（或 `go vet`）显式验编译」，避免后续 agent 在字面合规上纠结。**非本任务范围。**
3. **本轮无任何阻断级或中级问题。**

---

## 9. 裁决

**verified**。

理由：本轮 6 条 DoD **逐条 PASS 且证据为「翻转」而非「变绿」**——
7 个变异在返工前后由我在双 worktree 中分别实测为 SURVIVED 与 KILLED（各 10/10），
harness 三重防护全程有效（且编译守卫**实际拦下两次**，证明它不是摆设）。
leader 点名的两处风险（表建成 7 行 / `eps_diluted`·`equity` 断言零值）经 15 个追加探针**逐一证伪**：
表确为 9 行，9 行逐行有牙，`return 0` 变异同样被杀。
无回归、无覆盖率倒退、生产代码一行未改。
