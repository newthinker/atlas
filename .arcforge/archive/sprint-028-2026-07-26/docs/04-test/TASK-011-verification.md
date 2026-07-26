# TASK-011 验证报告 — /api/prism/sankey 与 /api/prism/fundamental

- **验证者**: test-agent-8（接手自 test-agent-7；见下「⛔ 写通道死锁」）
- **判定对象 HEAD**: `3912294`（验证全程复核未变；本任务实际变更仅 `a3864aa` + `3e2630c`，
  `d055490`/`3912294` 属 `internal/collector/edgar/fiscalperiod_test.go`，是他人任务，不在判定对象内）
- **结论**: **NEEDS WORK（建议 rejected）** —— `done_criteria` **逐条 PASS 且证据充分**，
  但任务描述明令「并入本任务」的六项顺带补测（W2/W4/W5/W7/W8/W10）**一件未做**，
  且我用变异实证其中两项确为真实缺口。裁决权归 leader（见 §6 争议点）。

---

## ⛔ 写通道死锁（阻塞裁决落盘，非我可绕过）

`update --field verifier=test-agent-8` 被 DENY：
```
DENY: 该任务 verifier=「test-agent-7」,不是你(test-agent-8)
  → 只能写绑定到你实例的任务
```
读 `.claude/hooks/arcforge-write.sh` + `.arcforge/write-matrix.json` 实证根因：

| 环节 | 事实 | 出处 |
|---|---|---|
| 角色绑定校验对 **transition 与 update 都生效** | `bindings["test-*"]="verifier"`，不匹配即 DENY | arcforge-write.sh L134-140 |
| `verifier` 字段**仅 leader 且仅 transition 模式**可写 | `guard_field_key` | arcforge-write.sh L53-56 |
| 从 `verifying` 出发的边只有 `->verified` / `->rejected`，写者均 `test-*` | leader **无任何合法出边** | write-matrix.json `transitions` |

⇒ **闭环死锁**：TASK-011 只有 `test-agent-7` 能写，而该实例已停止；leader 既不能改绑定
（无可用 transition），也不能自行迁移。**本报告与 checkpoint 可正常落盘**
（`docs/04-test/* <- ["test-*"]`、`checkpoints/{me}* <- self`），**唯独裁决迁移落不了盘**。

可行解（均需 leader/人类决策，我不自行推断、不绕过写通道）：
1. 人类会话外直接编辑 `.arcforge/tasks/TASK-011.json` 的 `verifier` 为 `test-agent-8`；
2. leader 经 `matrix set-token test-agent-7` 重置后由我以 `--as test-agent-7` 写 —— **冒名，不推荐**；
3. 在 `write-matrix.json` 为 leader 补一条 `verifying` 出边（改矩阵，影响面需评估）。

**该死锁本身是框架缺陷**：verifier 中途更换是可预期场景（agent 失联本 sprint 已三次），
矩阵却未留任何合法改绑路径。建议记入 wisdom 并在 `project-template/` 侧修复。

---

## 1. Done Criteria 覆盖矩阵

| # | 完成标准 | verify_by | 对应测试 | 变异证据 | 判定 |
|---|---|---|---|---|---|
| F0 | sankey 200 + `Periods`(每项 Period/Graph/Metrics)/`Matrix`/`Granularity`/`Template` | test | `TestSankeyResponseShape` | — | **PASS** |
| F0a | **AD-22 `Graph.Notes` 透传** + 对偶(正常数据下为空) | test | `TestSankeyNotesPassThrough`（两子用例） | M1 丢弃 Notes → KILLED；**P3 凭空造 Notes → KILLED**（对偶有牙） | **PASS** |
| F0b | **AD-26 `Analysis.Conflicts` 透传**，须断具体值非 `!= nil` | test | `TestSankeyConflictsPassThrough` | M2 丢弃 conflicts → KILLED；P4 detail 置空 → KILLED | **PASS** |
| F1 | `granularity`/`from`/`to`/`lang` 透传到 `Service.Analyze` | test | `TestSankeyQueryParamsReachService` | M4 lang 不传 → KILLED；M5 from/to 不传 → KILLED | **PASS** |
| F2 | 无模板 → 404 且响应体含 `no sankey template` | test | `TestSankeyErrorBranches/no_template` | M6 两种 404 文案混用 → KILLED | **PASS** |
| F3 | fundamental 200 含 `Periods`/`Metrics`/`Dates`/`Closes`；无数据 → 404 | test | `TestFundamentalResponse`（4 子用例） | P6 存储故障降级成 404 → KILLED | **PASS** |
| F4 | **AD-14** jf 同时拦 NaN 与 ±Inf；含 Inf 时 200 非 500 | test | `TestSankeyInfNeverBreaksJSON` | M7 jf 退回只拦 NaN → KILLED；M9 jfSlice 丢 jf → KILLED | **PASS** |
| B0 | 缺 symbol→400；`granularity=x`→400；三种 404 文案互不混淆；`PrismSankey=nil` 不注册路由(负向) | test | `TestSankeyErrorBranches` + `TestSankeyRoutesRegistration` | M3 granularity 不校验 → KILLED；M8 nil 也注册 → KILLED | **PASS** |
| NF0 | serve.go 装配 + **AD-16** 加载失败记日志、降级不注册、启动时一次性加载语义入 discovery | **review** | 人工审查（§3） | N/A | **PASS** |
| NF1 | `GOTOOLCHAIN=local go test ./internal/api/... ./cmd/atlas/ -count=1` 全绿 | test | 全量 | N/A | **PASS** |

**F0a/F0b 均满足「构造必然产生该内容的输入 + 断言具体值」的加严要求**：
Conflicts 断言 `period == "FY2025"` 且 `detail` 含 `"收到 5 个季度"`；
Notes 断言含 `"税项及其他"`。两者都配了对偶（正常数据下为空），且对偶经 P3 证明有判别力。

### NF1 实测输出（HEAD=3912294）
```
ok  github.com/newthinker/atlas/internal/api               1.683s
ok  github.com/newthinker/atlas/internal/api/handler/api   1.167s
ok  github.com/newthinker/atlas/internal/api/handler/web   2.605s
ok  github.com/newthinker/atlas/internal/api/job           0.746s
ok  github.com/newthinker/atlas/internal/api/middleware    2.132s
ok  github.com/newthinker/atlas/internal/api/response      0.415s
ok  github.com/newthinker/atlas/cmd/atlas                  3.641s
```
`gofmt -l`（六个改动文件）空；`go vet ./internal/api/... ./cmd/atlas/ ./internal/prism/sankey/` 无输出。

---

## 2. 独立变异测试（AD-27 合规）

**不采信 dev 自报计数**（其第一轮 8 次 KILLED 已自陈为假阳性）。我在隔离 worktree
（`git worktree add ... 3912294`）自建 harness 重跑，三重守卫：
- **(a) 还原校验**：每次 `git checkout --` 后 `git diff --quiet`，不净即 abort（堵 dev 第一轮的变异累积）
- **(b) 编译守卫**：`go build ./...` 失败记 `COMPILE_FAIL` **不计入**（编译失败与「存活」输出形态相同）
- **(c) sed 命中校验**：变异后 `git diff` 为空记 `SED_MISS` 不计入 KILLED
- 收尾 `FINAL TREE CLEAN` 确认（实测通过）

每个变异跑 `./internal/api/handler/api/ ./internal/api/ ./internal/prism/sankey/` 三包全量。

### 2.1 dev 声称的 9 个变异 —— **独立复现，9/9 全部 KILLED，计数属实**

| ID | 变异 | 结果 | 逮住它的用例（与 dev 声称一致？） |
|---|---|---|---|
| M1 | `graphJSON` 丢弃 `notes` | KILLED | `TestSankeyNotesPassThrough/negative_residual...` ✓ |
| M2 | `analysisJSON` 丢弃 `conflicts` | KILLED | `TestSankeyConflictsPassThrough` ✓ |
| M3 | granularity 非法值不再 400 | KILLED | `TestSankeyErrorBranches/invalid_granularity` ✓ |
| M4 | `lang` 不透传（传 `""`） | KILLED | `TestSankeyQueryParamsReachService` ✓ |
| M5 | `from`/`to` 不透传 | KILLED | `TestSankeyQueryParamsReachService` ✓ |
| M6 | 两种 404 文案混用 | KILLED | `TestSankeyErrorBranches/unknown_symbol` ✓ |
| M7 | `jf` 退回只拦 NaN | KILLED | `TestSankeyInfNeverBreaksJSON` + `TestFundamentalResponse/ok` ✓ |
| M8 | `PrismSankey==nil` 也注册路由 | KILLED | `TestSankeyRoutesRegistration/not_registered_without_a_service` ✓ |
| M9 | `jfSlice` 不再过 `jf` | KILLED | `TestPrismSeriesWindowAndErrors` + 上述二者 ✓ |

**两步核对通过**：被点名的用例确在失败集合内，且失败信息指向该规则本身。
M3 的 `+0 lines` 是纯删除型变异所致，`git diff` 非空已确认命中，非 SED_MISS。

### 2.2 我追加的 6 个探针 —— **3 个存活**

| ID | 探针 | 结果 | 分类（AD-27 #4，**不默认归为等价**） |
|---|---|---|---|
| P1 | `fundamentalMetrics` 把 `gross_profit` 映射到 `r.OperatingIncome` | **SURVIVED** | **(真实缺口)** 前端毛利曲线会画成营业利润，全仓无一断言 |
| P2 | `fundamentalMetrics` 把 `sganda` 映射到 `r.RnD` | **SURVIVED** | **(真实缺口)** 同上 |
| P3 | `graphJSON` 凭空返回固定 notes | KILLED | 对偶断言有判别力 |
| P4 | conflicts 的 `detail` 置空 | KILLED | 具体值断言有判别力 |
| P5 | `metricsJSON` 丢弃 `period_end` | **SURVIVED** | **(真实缺口)** 见 §4-G2 |
| P6 | Fundamental 把存储故障也报 404 | KILLED | `3e2630c` 补的用例有效 |

---

## 3. NF0 人工审查（AD-16 降级可观测）

`cmd/atlas/serve.go` 在 `prismCfg.Enabled` 分支内：
```go
templates, err := sankey.LoadTemplates(sankeyTemplateDir)
switch {
case err != nil:
    log.Error("prism sankey disabled: loading templates failed", zap.String("dir", ...), zap.Error(err))
case len(templates) == 0:
    log.Warn("prism sankey disabled: no templates configured", ...)
default:
    prismSankey = sankey.NewService(prismStore, templates)
    log.Info("prism sankey enabled", zap.Int("templates", len(templates)))
}
```
- 加载 error **不吞**：`log.Error` 带 dir 与 error ✓；serve 仍启动、`prismSankey` 保持 nil → 两条路由不注册 ✓
- 「注册一个必然报错的端点比不注册更糟」的理由已写进 `server.go` 注释 ✓
- `len==0` 单独走 Warn 是 DoD 外的合理加强 ✓
- 「启动时一次性加载，变更需重启」：`sankeyTemplateDir` 常量注释 + `serve.go` 内联注释均声明，
  且已记入 discovery（`decisions[].decision`）✓ —— DoD 明确要求「记入 discovery」，已满足
- `configs/prism/templates/` 实存 5 个模板（aapl/amzn/googl/msft/nvda），生产路径可用 ✓

**NF0 PASS。**

---

## 4. 发现的问题

### 🔴 G1（建议阻断）任务描述明令「并入本任务」的六项顺带补测 —— 一件未做

**事实**（非推测）：
```
$ git log --oneline a8c8354..HEAD -- internal/prism/sankey/
(空)
```
该包一行未改；dev 亦自陈「一行未改（全程 git status 确认）」。

任务描述原文明确：「**sankey 包后续无其他任务触碰,故并入本任务；packages 已扩含
`./internal/prism/sankey`**」，随后按性价比逐项开列 W7/W8、W2/W4、W5、W10 并给出修法。

**变异实证（描述预言的缺陷逐字命中）**：
- 描述称「把 `gross_profit` 映射到 `OperatingIncome`…都不会红（前端毛利曲线会画成营业利润）」
  → 我的 **P1 SURVIVED**，逐字复现。
- 描述称「`fundamentalMetrics` 其余 7 条序列全部无断言」
  → 实证 `internal/prism/sankey/service_test.go:382-385` 至今**只断言 `revenue` 与 `roe_ttm`**；
  `fundamentalMetrics`(service.go:161) 9 个 key 中其余 8 个零断言 → **P2 亦 SURVIVED**。

**为何本任务的 handler 侧补测不能替代**：`TestFundamentalResponse` 用的是 `fakeSankeyService`，
**完全不经过 `fundamentalMetrics`**，故 handler 层再密的断言也覆盖不到这条映射。

**为何这次漏掉**：dev 曾就该包发问，但只问了「是否要改 / 能否从 packages 摘掉」（纯覆盖率视角）；
leader 答复亦只处理覆盖率口径与 packages 取舍。**双方均未触及补测义务**，属意外漏做而非有意 descope。
因描述声明「sankey 包后续无其他任务触碰」，**此为最后一次机会**，放行即永久丢失。

**建议修复**（描述已给出，成本极低）：
1. W7/W8：`TestFundamental` 加一个表驱动断言，9 条序列各喂互不相同的值，逐条断言 `Metrics[key][i]` 精确相等
2. W2/W4：`TestAnalyzeDefaultSelection` 加 `assert.True(hasNode(..., "云业务"))` —— 一行杀两个存活体
3. W5：多期用例断言 `Periods[i].Metrics.Revenue` 对应第 i 期（现仅单期且下标恰为 0）
4. W10：`lang=en` 时断言 Matrix 行标签为英文

### 🟠 G2（中，建议随手补）`period_end` 全仓零断言
**P5 SURVIVED**：`metricsJSON` 丢弃 `"period_end"` 后三包无一变红。
代码实现是对的（`"period_end": m.PeriodEnd`），但没有任何测试锚定它。
- 该字段**不在** DoD 文字内，也**不在** TASK-009 `contract_for_downstream` 的四条里
  （那四条是 Notes / MaxPeriods / jf-Inf / 错误码映射），故**不判为 DoD 违反**。
- 但 design-spec:331 记其为「AD-26 补充（TASK-016 返工轮）：财年取**最后一季**的 period_end」，
  TASK-012 渲染很可能依赖。fixture 已有 `PeriodEnd: "2026-06-30"`，
  在 `TestSankeyResponseShape` 加一行 `assert.Equal(t, "2026-06-30", metrics["period_end"])` 即可。

### 🟡 G3（低，仅表述瑕疵，结论不受影响）AD-14 layer-1 的「唯一一处裸除法」
**我独立复核了 dev 的论证，结论成立**，但有一处措辞不准：
```
$ 扫描 internal/prism/sankey 全部非 _test .go（periods.go / service.go / template.go）
periods.go:300  return num / den
periods.go:455  days := c.Sub(p).Hours() / 24
periods.go:470  r := math.Pow(last/first, 1/float64(spans)) - 1
```
- 除法确实**只有三处**（dev 称三处，属实）✓
- 但 **470 行含两个除法算符**（`last/first` 与 `1/float64(spans)`），dev 称「唯一一处裸除法」（单数）不确。
  我另行核实无害：`cagr` 开头 `if spans < 1 ... return NaN` 使 `1/float64(spans)` 不可能除零；
  `first <= 0` 亦提前返回；`last` 为 +Inf 时 `math.Pow` 得 +Inf，被末尾 `if math.IsInf(r,0) { return NaN }` 兜住 ✓
- `safeDiv`（300 行）实读确认拦 `den==0 / IsNaN(den) / IsInf(den) / IsInf(num)` 四情形，
  `num` 为 NaN 时结果仍 NaN，**不可能返回 ±Inf** ✓
- 455 行除数为常量 24、返回 bool，不进数值输出 ✓
- 声称的五个 layer-1 引擎测试**全部实存**（`periods_test.go:519` "no Inf anywhere in the matrix"、
  `:789 TestBuildSankeyNaNStaysNaN`、`:185 TestBuildPeriodsRatioGuards`、`:494 TestBuildMatrixDivisionGuards`、
  `service_test.go:346 TestAnalyzeWithUnrefreshedColumns`）✓

**判定：AD-14 两层论证充分，我独立认可。**

### ⚪ G4（信息，非缺陷）Notes 构造方式弱于前任预案
`TestSankeyNotesPassThrough` 是把 `Graph.Notes` **字面塞进 fake**，而非用
`NetIncome > OperatingIncome` 驱动真引擎产出。低于 test-agent-7 预案的「构造必然产生该内容的输入」。
**判为可接受**：本任务被测单元是 handler，其上游是窄接口 `SankeyService`；
「负残差 → 产出 Notes」归 TASK-007 已验证范围，「装配」归 TASK-009；
类型契约由 `Dependencies.PrismSankey *sankey.Service` → `NewSankeyHandler` **编译期强制**。
且 P3 证明对偶断言有判别力。**记为残留**：四环链（007→009→011→012）**无任何单一端到端用例**，
各环只在自己边界上被测；TASK-012 验证时应留意。

---

## 5. 未作为判定依据的项

- **覆盖率**：`coverage_floor=70`（人类决策）。本任务新增/改动的 13 个函数**实测全部 100.0%**
  （`NewSankeyHandler/Sankey/Fundamental/analysisJSON/graphJSON/metricsJSON` + `jf/jfSlice` 等），
  连采 3 次读数稳定（本包无 map 遍历序依赖）。拉低整包数字的是仓库本就无单测的 `runServe`(0%)。
  **不构成质量问题，不作判定依据。**
- **dev 第一轮变异作废**：已按要求独立重跑，见 §2.1，dev 第二轮计数属实。

## 6. 争议点（请 leader 裁定）

G1 落在 **`description` 与 `done_criteria` 的边界**上：
- 支持 PASS：`done_criteria` 十条**逐条 PASS 且证据压倒性**；W 项不在 `done_criteria` 任何一条内；
  「DoD 是一切测试的唯一依据」。
- 支持 REJECT：`description` 是任务契约的一部分，措辞是祈使且逐项开列（含修法与性价比排序）；
  `packages` 为此专门扩含该包；描述自称「最后一次机会」；我已用变异**实证**其中两项是真实缺口
  且与描述的预言逐字吻合；修复成本约 1 个表驱动 + 3 行断言。

**我的建议：rejected**，`reject_reason` 只列 G1（+ 顺带 G2），其余全部 PASS 并在报告中明确记载，
返工面极小。若 leader 认定 W 项属 DoD 外（例如另开 TASK 收口），则本任务可直接 **verified**——
但**必须落一个后继任务**，否则该包再无任务触碰，六个存活变异体永久留存。
