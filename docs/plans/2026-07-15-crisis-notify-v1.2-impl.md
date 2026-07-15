# Crisis 通知设计 v1.2 实施方案

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**设计文档**：`docs/plans/2026-07-15-crisis-notify-v1.2-design.md`（R7）
**Goal**：「仍异常从句」机制化——从 CRISIS→WATCH 语义句中移除固定从句，改为所有降级在异常区非空时统一追加，消除全绿降级的三重矛盾。

**Architecture**：单文件修订 `internal/crisis/notify_render.go`——语义句表 1 键 + renderTransition 降级分支（从句与 ✅/🔽 共用 splitZones 判定）。零接口变更。

**Tech Stack**：Go 1.24.4、testify；无新依赖。

## Global Constraints

- `GOTOOLCHAIN=local` 前缀所有 go 命令；禁词（必然/一定/即将）零引入；页脚归属不变。
- 改既有 symbol 前跑 gitnexus_impact（renderTransition/semanticSentence——上一 Sprint 已跑均 LOW，索引未变可沿用，提交前 detect_changes 重跑）。
- 提交前 detect_changes + code-simplifier；git add 明确路径。
- 测试纪律：变异自检（从句判定与 🔽 判定双拦）、逐字文案、负向断言（旧从句移除）。

---

### Task 1: R7 仍异常从句机制化

**Files:**
- Modify: `internal/crisis/notify_render.go:168`（semanticSentences CRISIS→WATCH 键）、`:192-225`（renderTransition）
- Test: `internal/crisis/notify_render_test.go`（TestSemanticSentenceAllTransitions 一键期望 + TestRenderTransitionDowngrade/ConditionalGlyph 断言 + 新增子例）

**Interfaces:**
- Consumes: 既有 `splitZones`/`semanticSentence`/`staleDowngradeWarning`
- Produces: 无新符号；修订后的 CRISIS→WATCH 语义句字面值与条件从句「其余层面仍有异常，见下。」

- [ ] **Step 1: 写失败测试**

(a) TestSemanticSentenceAllTransitions 中 CRISIS→WATCH 期望改为：

```go
	{StateCrisis, StateWatch, "情绪层连续 10 个交易日回落至绿。危机状态退出，转入观察期。"},
```

(b) TestRenderTransitionDowngrade：CRISIS→WATCH 全绿用例断言改为：

```go
	msg = renderTransition(cfg, NotifyContext{Res: dayResult(StateCrisis, StateWatch), StateDays: 20})
	assert.True(t, strings.HasPrefix(msg, "[P1] ✅ 状态解除 CRISIS → WATCH"))
	assert.Contains(t, msg, "危机状态退出，转入观察期。")
	assert.NotContains(t, msg, "仍有异常")   // R7：全绿无从句
	assert.NotContains(t, msg, "可能仍异常") // 旧固定从句已移除
```

(c) TestRenderTransitionConditionalGlyph 追加从句断言（含异常与全绿两侧）：

```go
	// R7：从句与 🔽 共用判定——含异常降级带从句，全绿降级不带
	res3 := dayResult(StateCrisis, StateWatch)
	r3 := res3.Results[IndHYOAS]
	r3.Status = StatusAmber
	res3.Results[IndHYOAS] = r3
	msg = renderTransition(cfg, NotifyContext{Res: res3, StateDays: 20})
	assert.True(t, strings.HasPrefix(msg, "[P1] 🔽 状态回落 CRISIS → WATCH"))
	assert.Contains(t, msg, "转入观察期。其余层面仍有异常，见下。")

	// BREWING→WATCH 含异常同样获得从句（机制化，非仅 CRISIS→WATCH）
	res4 := dayResult(StateBrewing, StateWatch)
	r4 := res4.Results[IndHYOAS]
	r4.Status = StatusAmber
	res4.Results[IndHYOAS] = r4
	msg = renderTransition(cfg, NotifyContext{Res: res4, StateDays: 34})
	assert.Contains(t, msg, "回到观察期。其余层面仍有异常，见下。")

	// 全绿 BREWING→WATCH 不带从句
	msg = renderTransition(cfg, NotifyContext{Res: dayResult(StateBrewing, StateWatch), StateDays: 34})
	assert.NotContains(t, msg, "仍有异常")
```

- [ ] **Step 2: 运行确认失败**

Run: `GOTOOLCHAIN=local go test ./internal/crisis/ -run 'TestSemanticSentence|TestRenderTransition' -v`
Expected: FAIL（新期望与 v1.1 文案不符；从句不存在）

- [ ] **Step 3: 实现**

(a) semanticSentences CRISIS→WATCH 键改为：

```go
	"CRISIS→WATCH":   "情绪层连续 %d 个交易日回落至绿。危机状态退出，转入观察期。",
```

(b) renderTransition 降级分支与语义句拼接改为：

```go
	var residualClause string
	// ...
	} else {
		abnormal, _ := splitZones(res)
		glyphAndVerb := "✅ 状态解除" // R2：仅异常区为空时用 ✅（设计 v1.1 原则 2）
		if len(abnormal) > 0 {
			glyphAndVerb = "🔽 状态回落"
			residualClause = "其余层面仍有异常，见下。" // R7：与 🔽 共用同一判定
		}
		first = fmt.Sprintf("[P1] %s %s → %s · %s", glyphAndVerb, res.PrevState, res.State, monthDay(res.Date))
		title = "仍异常："
		tail = fmt.Sprintf("%s 共持续 %d 个评估日 · 下一评估：下一交易日", res.PrevState, nc.StateDays)
		if w := staleDowngradeWarning(nc); w != "" { // R1a：断更溯源警示置于尾注行前
			tail = w + "\n" + tail
		}
	}
	parts := []string{first}
	if s := semanticSentence(cfg, res.PrevState, res.State); s != "" {
		parts = append(parts, s+residualClause)
	} else if residualClause != "" { // 防御：不可达转移无语义句时从句独立成段
		parts = append(parts, residualClause)
	}
```

（`var residualClause string` 声明加在 `var first, title, tail string` 行。）

- [ ] **Step 4: 运行确认通过 + 变异自检**

Run: `GOTOOLCHAIN=local go build ./... && GOTOOLCHAIN=local go test ./internal/crisis/ ./cmd/atlas/ -v`
Expected: PASS（两包全绿，含全家族禁词/页脚回归）

变异自检：`len(abnormal) > 0` → `>= 0` 应同时击穿符号断言（✅ 恒变 🔽）与从句断言（全绿也带从句）——双拦确认后还原。
grep：`grep -rn "可能仍异常" internal/ cmd/ --include=*.go` 应仅测试 NotContains 守卫命中。

- [ ] **Step 5: 提交（先跑 detect_changes 与 code-simplifier）**

```bash
git add internal/crisis/notify_render.go internal/crisis/notify_render_test.go
git commit -m "feat(crisis): conditionalize residual-anomaly clause on downgrades (design v1.2 R7)"
```

## 自查记录

设计 R7 两点（语义句表/条件从句）、渲染示例双形态、测试要点 1–5 全部映射至 Task 1 各 Step；无占位符；`residualClause` 命名全篇一致。
