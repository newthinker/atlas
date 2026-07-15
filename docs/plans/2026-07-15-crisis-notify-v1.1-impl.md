# Crisis 通知设计 v1.1 实施方案

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**设计文档**：`docs/plans/2026-07-15-crisis-notify-v1.1-design.md`（v1.1，本方案唯一需求来源；R1–R6 编号引用它）
**基础**：sprint-020 已交付的通知层（`internal/crisis/notify*.go`，HEAD ≥ 2393b1e）。

**Goal**：落地设计 v1.1 的 6 项修订——降级溯源警示（R1）、条件符号+CRISIS→WATCH 语义句（R2）、WATCH→BREWING 去预测感（R3）、双非色彩迁移文案（R4）、盘中速报去归因（R5）、P2 术语外化（R6）。

**Architecture**：纯渲染层修订。`notify_render.go`（语义句表 2 键、renderTransition 降级分支、新私有助手 staleDowngradeWarning、diffLine、renderOpsAlert）+ `notify.go`（FormatIntradayAlert 格式串）。**零接口/签名/NotifyContext 字段变更，cmd 层生产代码不动**（仅 cmd 测试断言措辞适配）。

**Tech Stack**：Go 1.24.4、testify；无新依赖（Task 1 引入标准库 `slices`）。

## Global Constraints

- 所有 `go build` / `go test` 前缀 `GOTOOLCHAIN=local`；sqlite 固定 v1.38.2 不动。
- 禁词（必然/一定/即将）不得进入任何新文案；页脚归属规则不变（notifyFooter 只挂结构化家族）。
- **改既有 symbol 前跑 `gitnexus_impact({target, direction:"upstream"})`**（本方案涉及：renderTransition/diffLine/renderOpsAlert/semanticSentence/FormatIntradayAlert——预期全部收敛于 Messages/executeCrisisIntraday，LOW）。HIGH/CRITICAL 先停。
- **每任务提交前**：`gitnexus_detect_changes()` 核对影响面；运行 code-simplifier sub-agent（Task tool, subagent_type: `code-simplifier:code-simplifier`）。
- sprint-020 固化的测试纪律全部适用：每条判据指到断言行；coverprofile 新分支非零；含 >=/<=/恰好 的判据配恰好落界用例 + 变异自检；「A 或 B」逐支用例；cfg 来源值配异值锁；多级判据每级独立区分用例。
- git add 一律明确路径；勿带入 CLAUDE.md/AGENTS.md 的索引头改动。
- 每任务结束 `GOTOOLCHAIN=local go build ./...` 必须通过。

## 文件结构总览

```
internal/crisis/
├── notify_render.go       # [改] semanticSentences 2 键（R2/R3）；renderTransition 降级分支
│                          #      （R2 条件符号 + R1a 警示行）；+staleDowngradeWarning；
│                          #      diffLine 双非色彩分支（R4）；renderOpsAlert（R1b/R6）
├── notify_render_test.go  # [改] 对应断言更新 + 新用例（Task 1–2）
├── notify.go              # [改] FormatIntradayAlert 格式串（R5）（Task 3）
└── notify_test.go         # [改] 盘中速报断言 + 页脚归属改 HasSuffix(notifyFooter)（Task 3）
cmd/atlas/
└── crisis_test.go         # [改] 盘中断言措辞适配（"carry trade" → "成因未核实"）（Task 3）
```

## 任务总览

| Task | 内容 | 设计条目 |
|---|---|---|
| 1 | 语义句 2 键 + 降级条件符号 + 断更降级警示行 | R1a/R2/R3 |
| 2 | diffLine 双非色彩迁移 + P2 速报术语与警示 | R1b/R4/R6 |
| 3 | 盘中速报去归因 + 全家族页脚断言改 HasSuffix | R5（含测试连锁） |

任务间无接口依赖，但 Task 3 的页脚断言重写以 Task 1–2 的文案为最终态，**按序执行**。

---

# 任务分解

### Task 1: 语义句修订、降级条件符号与断更警示行（R1a/R2/R3）

**Files:**
- Modify: `internal/crisis/notify_render.go:162-171`（semanticSentences）、`:192-216`（renderTransition）、import 增 `slices`
- Test: `internal/crisis/notify_render_test.go`（更新 TestSemanticSentenceAllTransitions / TestRenderTransitionUpgrade / TestRenderTransitionDowngrade，新增 TestRenderTransitionStaleWarning / TestRenderTransitionConditionalGlyph）

**Interfaces:**
- Consumes: 既有 `splitZones`/`colorWord`/`severity`/`NotifyContext.NewStale`/`NotifyContext.PrevDay`（均已存在，零新管道）
- Produces: 包内私有 `staleDowngradeWarning(nc NotifyContext) string`（仅 renderTransition 消费）；修订后的两条语义句字面值（Task 3 全家族测试沿用）

- [ ] **Step 1: 影响面分析（项目 MUST 规则）**

对 `renderTransition`、`semanticSentence` 跑 `gitnexus_impact(upstream)`。预期调用方仅 `Messages`（包内）→ `executeCrisisEvalDaily`，LOW。HIGH/CRITICAL 先停。

- [ ] **Step 2: 写失败测试** — `internal/crisis/notify_render_test.go`：

**(a) 更新 TestSemanticSentenceAllTransitions 中两键的期望值**（表驱动里对应行替换为）：

```go
	{StateWatch, StateBrewing, "信用与流动性双红共振。历史样本中此组合后系统性风险抬升比例显著（样本量小，可能失效）；此为状态描述而非预测，不构成操作依据。"},
	{StateCrisis, StateWatch, "情绪层连续 10 个交易日回落至绿。危机状态退出，转入观察期；信用/流动性等其余层面可能仍异常，见下。"},
```

**(b) 更新 TestRenderTransitionUpgrade**：WATCH→BREWING 断言追加：

```go
	assert.Contains(t, msg, "此为状态描述而非预测，不构成操作依据")
	assert.NotContains(t, msg, "3–12 个月")
```

**(c) 更新 TestRenderTransitionDowngrade**：既有 BREWING→WATCH 用例（hy_oas 置 AMBER，异常区非空）首行断言改为：

```go
	assert.True(t, strings.HasPrefix(msg, "[P1] 🔽 状态回落 BREWING → WATCH · 09-02"))
```

CRISIS→WATCH 用例（dayResult 全绿，异常区空）断言改为：

```go
	msg = renderTransition(cfg, NotifyContext{Res: dayResult(StateCrisis, StateWatch), StateDays: 20})
	assert.True(t, strings.HasPrefix(msg, "[P1] ✅ 状态解除 CRISIS → WATCH"))
	assert.Contains(t, msg, "危机状态退出，转入观察期")
	assert.NotContains(t, msg, "危机状态解除")
```

**(d) 新增两测试**（文件末尾追加）：

```go
// R2（设计 v1.1）：✅ 仅限异常区为空的降级；非空用 🔽 状态回落。恰好落界：
// 恰有一个 🟡 即切换。
func TestRenderTransitionConditionalGlyph(t *testing.T) {
	cfg := testConfig()

	// 全绿降级 → ✅ 状态解除
	msg := renderTransition(cfg, NotifyContext{Res: dayResult(StateWatch, StateNormal), StateDays: 40})
	assert.True(t, strings.HasPrefix(msg, "[P1] ✅ 状态解除 WATCH → NORMAL"))
	assert.NotContains(t, msg, "状态回落")

	// 恰好一个 AMBER → 🔽 状态回落（落界）
	res := dayResult(StateWatch, StateNormal)
	r := res.Results[IndHYOAS]
	r.Status = StatusAmber
	res.Results[IndHYOAS] = r
	msg = renderTransition(cfg, NotifyContext{Res: res, StateDays: 40})
	assert.True(t, strings.HasPrefix(msg, "[P1] 🔽 状态回落 WATCH → NORMAL"))
	assert.NotContains(t, msg, "状态解除")

	// 含 ⚪ 但无 🔴🟡（异常区仍为空）→ ✅（⚪ 不算异常区）
	res2 := dayResult(StateBrewing, StateWatch)
	r2 := res2.Results[IndMOVE]
	r2.Status = StatusStale
	res2.Results[IndMOVE] = r2
	msg = renderTransition(cfg, NotifyContext{Res: res2, StateDays: 34})
	assert.True(t, strings.HasPrefix(msg, "[P1] ✅ 状态解除 BREWING → WATCH"))

	// 升级路径不受影响
	msg = renderTransition(cfg, NotifyContext{Res: dayResult(StateWatch, StateBrewing), StateDays: 12})
	assert.True(t, strings.HasPrefix(msg, "[P0] 🚨 状态升级 WATCH → BREWING"))
}

// R1a（设计 v1.1）：降级当日 NewStale 且断更前为 RED/AMBER → 尾注前插警示行。
// 三条件独立否定 + 多指标 AllIndicators 序 + 断更前恰为 AMBER 落界。
func TestRenderTransitionStaleWarning(t *testing.T) {
	cfg := testConfig()
	downgrade := func(newStale []string, prevDay map[string]Evaluation) string {
		return renderTransition(cfg, NotifyContext{
			Res: dayResult(StateBrewing, StateWatch), StateDays: 34,
			NewStale: newStale, PrevDay: prevDay,
		})
	}

	// 断更前 RED → 警示行出现，且在尾注之前
	msg := downgrade([]string{IndHYOAS},
		map[string]Evaluation{IndHYOAS: {Indicator: IndHYOAS, Status: StatusRed}})
	assert.Contains(t, msg, "⚠ 注意：本次变更当日 hy_oas 数据断更（断更前为红），触发条件可能被动解除而非真实缓解，请人工核实。")
	assert.Less(t, strings.Index(msg, "⚠ 注意"), strings.Index(msg, "共持续"))

	// 断更前恰为 AMBER（落界）→ 出现
	msg = downgrade([]string{IndHYOAS},
		map[string]Evaluation{IndHYOAS: {Indicator: IndHYOAS, Status: StatusAmber}})
	assert.Contains(t, msg, "（断更前为黄）")

	// 否定 1：NewStale 为空 → 无警示
	msg = downgrade(nil, map[string]Evaluation{IndHYOAS: {Indicator: IndHYOAS, Status: StatusRed}})
	assert.NotContains(t, msg, "⚠ 注意")

	// 否定 2：断更前为绿 → 无警示
	msg = downgrade([]string{IndHYOAS},
		map[string]Evaluation{IndHYOAS: {Indicator: IndHYOAS, Status: StatusGreen}})
	assert.NotContains(t, msg, "⚠ 注意")

	// 否定 3：升级路径 → 无警示（即使 NewStale+RED）
	msg = renderTransition(cfg, NotifyContext{
		Res: dayResult(StateWatch, StateBrewing), StateDays: 12,
		NewStale: []string{IndHYOAS},
		PrevDay:  map[string]Evaluation{IndHYOAS: {Indicator: IndHYOAS, Status: StatusRed}},
	})
	assert.NotContains(t, msg, "⚠ 注意")

	// 多指标：AllIndicators 序（vix 先于 hy_oas），颜色同序对应
	msg = downgrade([]string{IndHYOAS, IndVIX}, map[string]Evaluation{
		IndHYOAS: {Indicator: IndHYOAS, Status: StatusAmber},
		IndVIX:   {Indicator: IndVIX, Status: StatusRed},
	})
	assert.Contains(t, msg, "vix、hy_oas 数据断更（断更前为红、黄）")
}
```

- [ ] **Step 3: 运行确认失败**

Run: `GOTOOLCHAIN=local go test ./internal/crisis/ -run 'TestSemanticSentence|TestRenderTransition' -v`
Expected: FAIL（新期望值与 v1.0 文案不符；🔽/警示行不存在）

- [ ] **Step 4: 实现** — `internal/crisis/notify_render.go`：

**(a) import 增加 `"slices"`**（放入既有 import 块字母序）。

**(b) semanticSentences 两键替换**：

```go
	"WATCH→BREWING": "信用与流动性双红共振。历史样本中此组合后系统性风险抬升比例显著（样本量小，可能失效）；此为状态描述而非预测，不构成操作依据。",
	"CRISIS→WATCH":  "情绪层连续 %d 个交易日回落至绿。危机状态退出，转入观察期；信用/流动性等其余层面可能仍异常，见下。",
```

**(c) renderTransition 降级分支替换**（else 块整体改为）：

```go
	} else {
		glyphAndVerb := "✅ 状态解除" // R2：仅异常区为空时用 ✅（设计 v1.1 原则 2）
		if abnormal, _ := splitZones(res); len(abnormal) > 0 {
			glyphAndVerb = "🔽 状态回落"
		}
		first = fmt.Sprintf("[P1] %s %s → %s · %s", glyphAndVerb, res.PrevState, res.State, monthDay(res.Date))
		title = "仍异常："
		tail = fmt.Sprintf("%s 共持续 %d 个评估日 · 下一评估：下一交易日", res.PrevState, nc.StateDays)
		if w := staleDowngradeWarning(nc); w != "" { // R1a：断更溯源警示置于尾注行前
			tail = w + "\n" + tail
		}
	}
```

**(d) 文件末尾追加助手**：

```go
// staleDowngradeWarning R1a（设计 v1.1）：状态降级当日有指标新进入 STALE 且
// 断更前为 RED/AMBER 时，生成溯源警示——触发条件可能被动解除而非真实缓解。
// 断更前状态取 PrevDay（昨日行）；指标按 AllIndicators 序，颜色列表同序对应；
// 无符合条件指标返回空串（升级路径由调用方保证不调用本函数）。
func staleDowngradeWarning(nc NotifyContext) string {
	var inds, colors []string
	for _, ind := range AllIndicators {
		if !slices.Contains(nc.NewStale, ind) {
			continue
		}
		prev, ok := nc.PrevDay[ind]
		if !ok || severity(prev.Status) < severity(StatusAmber) {
			continue
		}
		inds = append(inds, ind)
		colors = append(colors, colorWord(prev.Status))
	}
	if len(inds) == 0 {
		return ""
	}
	return fmt.Sprintf("⚠ 注意：本次变更当日 %s 数据断更（断更前为%s），触发条件可能被动解除而非真实缓解，请人工核实。",
		strings.Join(inds, "、"), strings.Join(colors, "、"))
}
```

- [ ] **Step 5: 运行确认通过 + 变异自检**

Run: `GOTOOLCHAIN=local go build ./... && GOTOOLCHAIN=local go test ./internal/crisis/ -v`
Expected: PASS（全包，含既有回归）

变异自检（改后必须 FAIL、还原后 PASS，逐个做）：
1. `len(abnormal) > 0` → `>= 0`（✅ 恒变 🔽）→ TestRenderTransitionConditionalGlyph FAIL
2. `severity(prev.Status) < severity(StatusAmber)` → `<= `（AMBER 不再触发）→ StaleWarning 落界用例 FAIL
3. 删除 `if w := ...` 块 → StaleWarning 主用例 FAIL

coverprofile 核对 staleDowngradeWarning 全分支非零。

- [ ] **Step 6: 提交（先跑 gitnexus detect_changes 与 code-simplifier）**

```bash
git add internal/crisis/notify_render.go internal/crisis/notify_render_test.go
git commit -m "feat(crisis): add downgrade provenance warning and conditional glyph (design v1.1 R1a/R2/R3)"
```

### Task 2: diffLine 双非色彩迁移与 P2 速报修订（R1b/R4/R6）

**Files:**
- Modify: `internal/crisis/notify_render.go:234-259`（diffLine）、`:335-351`（renderOpsAlert）
- Test: `internal/crisis/notify_render_test.go`（更新 TestDiffLineLevels / TestRenderOpsAlert / TestOpsAlertLagInjection 措辞，新增双非色彩与 R1b 用例）

**Interfaces:**
- Consumes: 既有 `isColor`/`nonColorNote`/`colorWord`/`severity`/`NotifyContext.PrevDay`
- Produces: 无新符号；修订后的 P2 文案字面值（Task 3 全家族测试沿用）

- [ ] **Step 1: 影响面分析**

对 `diffLine`、`renderOpsAlert` 跑 `gitnexus_impact(upstream)`。预期调用方 renderDaily/Messages（包内），LOW。

- [ ] **Step 2: 写失败测试**

**(a) TestDiffLineLevels 追加子例**（R4；既有"move 转白（原绿）"混合迁移断言**保持不动**）：

```go
	// R4：双非色彩迁移用具体说明，不再"转白（原白）"
	ncMix := NotifyContext{Res: dayResult(StateBrewing, StateBrewing), PrevDay: map[string]Evaluation{
		IndSOFREFFR: {Indicator: IndSOFREFFR, Status: StatusStale, Value: 28},
		IndNFCI:     {Indicator: IndNFCI, Status: StatusNoData},
	}}
	rs := ncMix.Res.Results[IndSOFREFFR]
	rs.Status = StatusSuppressed
	ncMix.Res.Results[IndSOFREFFR] = rs
	rn := ncMix.Res.Results[IndNFCI]
	rn.Status = StatusStale
	ncMix.Res.Results[IndNFCI] = rn
	line := diffLine(ncMix)
	assert.Contains(t, line, "sofr_effr 转季末抑制（原数据断更(STALE)）")
	assert.Contains(t, line, "nfci 转数据断更(STALE)（原无数据(NO_DATA)）")
	assert.NotContains(t, line, "转白（原白）")
```

（常量名已核实：types.go:13-15 定义 `StatusStale`/`StatusSuppressed`/`StatusNoData`。）

**(b) TestRenderOpsAlert 更新与追加**（R6 措辞 + R1b 条件行）：既有精确相等断言中的
`已标记 STALE 退出共振计数；恢复后自动回归` 全部替换为
`已标记 STALE、不再计入触发判定；数据恢复后自动重新计入`；并追加：

```go
	// R1b：断更前为 RED → 追加警示行；恰为 AMBER（落界）→ 同样追加；绿/缺失 → 不追加
	ncPrev := NotifyContext{Res: res, NewStale: []string{IndMOVE},
		StaleLastObs: map[string]string{IndMOVE: "2026-07-09"},
		PrevDay:      map[string]Evaluation{IndMOVE: {Indicator: IndMOVE, Status: StatusRed}}}
	msg := renderOpsAlert(cfg, ncPrev, IndMOVE)
	assert.Contains(t, msg, "⚠ 断更前为红且计入触发判定，今日若出现状态解除可能为被动解除，请人工核实。")

	ncPrev.PrevDay[IndMOVE] = Evaluation{Indicator: IndMOVE, Status: StatusAmber}
	assert.Contains(t, renderOpsAlert(cfg, ncPrev, IndMOVE), "⚠ 断更前为黄")

	ncPrev.PrevDay[IndMOVE] = Evaluation{Indicator: IndMOVE, Status: StatusGreen}
	assert.NotContains(t, renderOpsAlert(cfg, ncPrev, IndMOVE), "⚠ 断更前")

	delete(ncPrev.PrevDay, IndMOVE) // PrevDay 缺行 → 不追加
	assert.NotContains(t, renderOpsAlert(cfg, ncPrev, IndMOVE), "⚠ 断更前")
```

- [ ] **Step 3: 运行确认失败**

Run: `GOTOOLCHAIN=local go test ./internal/crisis/ -run 'TestDiffLine|TestRenderOpsAlert|TestOpsAlert' -v`
Expected: FAIL

- [ ] **Step 4: 实现**

**(a) diffLine 迁移分支替换**（原 `if prev.Status != cur.Status { ... }` 块改为）：

```go
		if prev.Status != cur.Status {
			if !isColor(prev.Status) && !isColor(cur.Status) { // R4：双非色彩用具体说明
				parts = append(parts, fmt.Sprintf("%s 转%s（原%s）", ind,
					nonColorNote(cur.Status), nonColorNote(prev.Status)))
			} else {
				parts = append(parts, fmt.Sprintf("%s 转%s（原%s）", ind,
					colorWord(cur.Status), colorWord(prev.Status)))
			}
			continue
		}
```

**(b) renderOpsAlert 整函数替换**：

```go
// renderOpsAlert 消息 6：P2 运维速报（通知设计 §5.6，v1.1 R1b/R6）。速报家族无
// 页脚；去重由 cmd 组装 NewStale 时完成。断更前为 RED/AMBER 时追加被动解除警示。
func renderOpsAlert(cfg *Config, nc NotifyContext, ind string) string {
	first := fmt.Sprintf("[P2] 🔧 %s 数据源断更 · %s", ind, monthDay(nc.Res.Date))
	channel := "FRED"
	if ind == IndMOVE || ind == IndUSDJPY {
		channel = "Yahoo"
	}
	var body string
	if lastObs, ok := nc.StaleLastObs[ind]; ok && lastObs != "" {
		maxLag := cfg.Freshness.DailyMaxLagDays
		if ind == IndNFCI {
			maxLag = cfg.Freshness.WeeklyMaxLagDays
		}
		body = fmt.Sprintf("最后观测 %s（滞后 %d 日 > 阈值 %d 日），已标记 STALE、不再计入触发判定；数据恢复后自动重新计入。持续超一周需检查 %s 通道。",
			monthDay(lastObs), daysBetween(lastObs, nc.Res.Date), maxLag, channel)
	} else {
		body = fmt.Sprintf("无历史观测，已标记 STALE、不再计入触发判定；数据恢复后自动重新计入。持续超一周需检查 %s 通道。", channel)
	}
	msg := first + "\n" + body
	if prev, ok := nc.PrevDay[ind]; ok && severity(prev.Status) >= severity(StatusAmber) { // R1b
		msg += fmt.Sprintf("\n⚠ 断更前为%s且计入触发判定，今日若出现状态解除可能为被动解除，请人工核实。", colorWord(prev.Status))
	}
	return msg
}
```

- [ ] **Step 5: 运行确认通过 + 变异自检**

Run: `GOTOOLCHAIN=local go build ./... && GOTOOLCHAIN=local go test ./internal/crisis/ -v`
Expected: PASS

变异自检：1. R4 分支 `&&` → `||` → 双非色彩用例 FAIL；2. R1b `>= severity(StatusAmber)` → `>` → AMBER 落界用例 FAIL。

- [ ] **Step 6: 提交（先跑 detect_changes 与 code-simplifier）**

```bash
git add internal/crisis/notify_render.go internal/crisis/notify_render_test.go
git commit -m "feat(crisis): non-color diff wording and ops alert provenance (design v1.1 R1b/R4/R6)"
```

### Task 3: 盘中速报去归因与页脚断言重构（R5 + 测试连锁）

**Files:**
- Modify: `internal/crisis/notify.go:41-45`（FormatIntradayAlert）
- Test: `internal/crisis/notify_test.go`（TestFormatIntradayAlert、TestMessagesForbiddenWordsAllFamilies）、`cmd/atlas/crisis_test.go`（盘中断言措辞，约 :907-908 附近的 `carry trade` 断言）

**Interfaces:**
- Consumes: `notifyFooter`（包级常量，页脚归属断言的新判定基准）
- Produces: 修订后的盘中速报格式串（cmd 测试断言依赖其中 "成因未核实" 片段）

- [ ] **Step 1: 影响面分析**

对 `FormatIntradayAlert` 跑 `gitnexus_impact(upstream)`。预期调用方仅 `executeCrisisIntraday`，LOW。

- [ ] **Step 2: 写失败测试**

**(a) TestFormatIntradayAlert 断言替换为**：

```go
	at := time.Date(2026, 7, 18, 14, 30, 0, 0, time.Local)
	msg := FormatIntradayAlert(152.1, 157.5, -0.034, StateBrewing, at)
	assert.True(t, strings.HasPrefix(msg, "[P0] 🚨 USD/JPY 盘中急跌 -3.4% · 07-18 14:30"))
	assert.Contains(t, msg, "现价 152.1（5 观测日前 157.5）")
	assert.Contains(t, msg, "系统状态 BREWING")
	assert.Contains(t, msg, "成因未核实，非交易信号")   // R5 内联限定语
	assert.NotContains(t, msg, "carry trade")            // R5 去归因
	assert.Contains(t, msg, "今日此告警不再重复")
	assert.False(t, strings.HasSuffix(msg, notifyFooter)) // 速报仍不挂页脚常量
```

**(b) TestMessagesForbiddenWordsAllFamilies 的家族归属段替换**（禁词与 4096 断言不动）：

```go
	structuredCount, alertCount := 0, 0
	for _, m := range all {
		for _, banned := range []string{"必然", "一定", "即将"} {
			assert.NotContains(t, m, banned)
		}
		// v1.1 R5 连锁：页脚归属改按 notifyFooter 完整常量判定（盘中速报现含
		// "非交易信号"内联限定语，子串判定失效）
		if strings.HasPrefix(m, "[P2]") || strings.Contains(m, "盘中急跌") {
			assert.False(t, strings.HasSuffix(m, notifyFooter), "速报家族不得挂页脚: %s", m[:20])
			alertCount++
		} else {
			assert.True(t, strings.HasSuffix(m, notifyFooter), "结构化家族必须挂页脚: %s", m[:20])
			structuredCount++
		}
		assert.LessOrEqual(t, len(m), 4096)
	}
	assert.Equal(t, 5, structuredCount) // 升级/降级/日报/月报/周报
	assert.Equal(t, 2, alertCount)      // P2 + 盘中
```

**(c) cmd/atlas/crisis_test.go**：盘中测试中 `assert.Contains(..., "carry trade")` 类断言改为 `assert.Contains(..., "成因未核实")`（只改字符串，不放宽强度；具体行以 grep `carry trade` 为准）。

- [ ] **Step 3: 运行确认失败**

Run: `GOTOOLCHAIN=local go test ./internal/crisis/ -run 'TestFormatIntraday|TestMessagesForbidden' -v && GOTOOLCHAIN=local go test ./cmd/atlas/ -run TestCrisis -v`
Expected: FAIL（新文案不存在）

- [ ] **Step 4: 实现** — `internal/crisis/notify.go` FormatIntradayAlert 格式串替换：

```go
// FormatIntradayAlert 消息 7：盘中 JPY 速报（通知设计 §5.7，v1.1 R5：去因果
// 归因，报事实 + 内联限定语；该限定语非页脚常量，速报家族无页脚规则不变）。
// at 为本地时区时刻；每日一次去重由 executeCrisisIntraday 的评估行保证。
func FormatIntradayAlert(price, base, wow float64, state SystemState, at time.Time) string {
	return fmt.Sprintf(
		"[P0] 🚨 USD/JPY 盘中急跌 %.1f%% · %s\n现价 %.1f（5 观测日前 %.1f）· 系统状态 %s · 成因未核实，非交易信号。今日此告警不再重复。",
		wow*100, at.Format("01-02 15:04"), price, base, state)
}
```

- [ ] **Step 5: 全量验证 + 变异自检**

Run: `GOTOOLCHAIN=local go build ./... && GOTOOLCHAIN=local go test ./internal/crisis/ ./cmd/atlas/ -v`
Expected: PASS（两包全绿）

变异自检：给盘中速报误挂 notifyFooter（临时在返回值 + notifyFooter）→ TestFormatIntradayAlert 与全家族测试均 FAIL；还原。

grep 终检：`grep -rn "carry trade\|退出共振计数\|危机状态解除\|3–12 个月" internal/ cmd/` 应零命中（生产与测试）。

- [ ] **Step 6: 提交（先跑 detect_changes 与 code-simplifier）**

```bash
git add internal/crisis/notify.go internal/crisis/notify_test.go cmd/atlas/crisis_test.go
git commit -m "feat(crisis): factual intraday alert wording and footer-constant assertions (design v1.1 R5)"
```

---

## 自查记录（设计 v1.1 ↔ 任务覆盖）

| 设计条目 | 覆盖任务 |
|---|---|
| R1a 降级警示行（三条件+多指标序+落界） | Task 1（staleDowngradeWarning + TestRenderTransitionStaleWarning） |
| R1b P2 条件警示（RED/AMBER 落界/绿否定/缺行否定） | Task 2（renderOpsAlert + 追加用例） |
| R2 条件符号 ✅/🔽 + CRISIS→WATCH 语义句 | Task 1（TestRenderTransitionConditionalGlyph + 语义句表） |
| R3 WATCH→BREWING 去预测感（删时间窗+非预测从句） | Task 1 |
| R4 双非色彩迁移（混合迁移维持 v1.0） | Task 2（diffLine 分支 + 双用例） |
| R5 盘中去归因 + 内联限定语 + 页脚断言 HasSuffix 连锁 | Task 3 |
| R6 术语外化（两处措辞） | Task 2 |
| 新增原则 1–4（§1） | 措辞由 R2/R3 落地；原则 2/3 由条件符号/警示行落地；原则 4 由 R6 落地 |
| §4 测试要点 1–7 | 1→T1、2→T2、3→T1、4→T1（语义句逐字）、5→T2、6→T3、7→T3（禁词回归+4096） |

**类型一致性**：`staleDowngradeWarning(nc NotifyContext) string` 仅 Task 1 定义与消费；Task 2/3 无新符号；`notifyFooter` 为既有常量。三任务均不改任何导出签名。

## 执行说明

- 推荐执行方式：规模小（3 任务、单包为主），**executing-plans 在本 session 顺序执行**即可；如需隔离并行可走 subagent-driven-development，但 Task 1/2 同文件必须串行。
- 在 `feature/crisis-notify-templates` 分支上继续（v1.0 实现所在分支，尚未合并）。
- 完成后可选：在设计 v1.0 文档头部加一行指针注明 §4.1/§5.2/§5.6/§5.7/§6.5 已由 v1.1 修订。
