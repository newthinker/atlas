# Cassandra NORMAL 态周报 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 危机监控在 NORMAL 态每周一（评估日）发送周报；当月首个交易日仍发月报，撞日只发月报。

**Architecture:** 方案 A——`crisis` 包新增三值 `SummaryKind` 枚举，cmd 层 `summaryKind` 一处判日（撞日归月报），`Messages` 按枚举路由，`renderWeekly` 尾段按状态分叉（退出进度行仅 WATCH）。渲染层保持纯函数，评估成本不变（NORMAL 周报不拉 Trends 窗口）。

**Tech Stack:** Go（stdlib + testify），无新依赖。

**Spec:** `docs/superpowers/specs/2026-07-23-crisis-normal-weekly-report-design.md`

## Global Constraints

- 「周一」= 评估日 `res.Date` 为周一（与现有 WATCH 周报语义一致），不改 launchd 调度。
- 不变量：结构化消息每日至多 1 条（`Messages` 互斥 switch 结构不变）。
- WATCH 周报输出逐字节不变（现有 `TestRenderWeekly` 断言必须原样通过）。
- `NotifyContext.SummaryDue bool` 替换为 `Summary SummaryKind`（零值 `SummaryNone` 安全静默）。
- 项目门禁：编辑符号前跑 `gitnexus_impact`，提交前跑 `gitnexus_detect_changes`。当前索引落后且 FTS 损坏——降级处理：本计划已通过 grep 枚举全部引用点（见各任务 Files），detect_changes 以「affected_processes 为空/仅涉本计划文件 + git diff 范围」判定（见 memory gitnexus-midsession-reindex）。
- 提交信息规范：`<type>(crisis): <description>`，尾行 `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`。
- 测试日期速查（2026 年）：07-13 周一、07-20 周一、07-21 周二、06-01 周一且为当月首个交易日（撞日）、08-03 周一且为首交易日（8/1 周六）、07-01 周三首交易日。

---

### Task 1: crisis 包 — SummaryKind 枚举与 Messages 路由

**Files:**
- Modify: `internal/crisis/notify.go`（`Messages` switch、新增枚举）
- Modify: `internal/crisis/notify_render.go:21`（`NotifyContext` 字段）
- Test: `internal/crisis/notify_test.go`（5 处 `SummaryDue` 替换 + 1 个新分支用例）
- Test: `internal/crisis/notify_render_test.go:496`（1 处编译修复）

**Interfaces:**
- Produces: `type SummaryKind int`；常量 `SummaryNone`（零值）/ `SummaryWeekly` / `SummaryMonthly`；`NotifyContext.Summary SummaryKind` 字段。Task 3 的 cmd 层依赖这三个导出名。

- [ ] **Step 1: 更新 notify_test.go（RED）**

`internal/crisis/notify_test.go` 中 5 处 `SummaryDue: true` 逐处替换：

- 第 39 行（变更优先否定路径）：`SummaryDue: true, ClearStreak: 8` → `Summary: SummaryWeekly, ClearStreak: 8`
- 第 54 行（NORMAL 月报）：`SummaryDue: true, Trends: testTrends("2026-07-10")` → `Summary: SummaryMonthly, Trends: testTrends("2026-07-10")`
- 第 57 行（WATCH 周报）：`SummaryDue: true, ClearStreak: 8` → `Summary: SummaryWeekly, ClearStreak: 8`
- 第 110 行（禁词全家族·月报）：同第 54 行的替换
- 第 112 行（禁词全家族·周报）：同第 57 行的替换

并在 `TestMessagesDispatch` 的「分支 3/4」块末尾（第 58 行 `assert.Contains(t, msgs[0], "Cassandra 周报")` 之后）新增：

```go
	// —— 分支 4'：NORMAL+周报到期 → 周报（NORMAL 周报设计 2026-07-23）——
	msgs = Messages(cfg, NotifyContext{Res: dayResult(StateNormal, StateNormal), StateDays: 30, Summary: SummaryWeekly})
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0], "Cassandra 周报")
```

注：既有「boundary：NORMAL 非到期 → 零消息」用例不带 Summary 字段，零值即 `SummaryNone`，自动覆盖枚举默认值边界。

- [ ] **Step 2: 运行验证失败**

Run: `go test ./internal/crisis/ -run TestMessages 2>&1 | head -20`
Expected: 编译错误 `undefined: SummaryWeekly`（及 `unknown field Summary`）

- [ ] **Step 3: 实现枚举与路由**

`internal/crisis/notify.go`：在 `Sender` 接口定义之后新增：

```go
// SummaryKind 是摘要类消息的到期类型，cmd 层按评估日与状态判定（撞日归月报，
// NORMAL 周报设计 2026-07-23）。零值 SummaryNone = 不到期。
type SummaryKind int

const (
	SummaryNone SummaryKind = iota
	SummaryWeekly
	SummaryMonthly
)
```

`Messages` 的 switch 后两分支改为：

```go
	case nc.Summary == SummaryMonthly && res.State == StateNormal:
		msgs = append(msgs, renderMonthly(cfg, nc))
	case nc.Summary == SummaryWeekly && (res.State == StateNormal || res.State == StateWatch):
		msgs = append(msgs, renderWeekly(cfg, nc))
```

同时把 `Messages` 上方文档注释里的「NORMAL 月报 / WATCH 周报」改为「NORMAL 月报·周报 / WATCH 周报」，`SummaryDue` 字样改为 `Summary`。

`internal/crisis/notify_render.go:21` 字段替换：

```go
	Summary      SummaryKind           // 摘要到期类型（cmd 计算，撞日归月报）
```

`internal/crisis/notify_render_test.go:496`：`SummaryDue: true` → `Summary: SummaryMonthly`。

- [ ] **Step 4: 运行验证通过**

Run: `go test ./internal/crisis/`
Expected: `ok`（全包通过，含 renderWeekly 既有回归）

- [ ] **Step 5: 提交**

先跑 `gitnexus_detect_changes`（scope=all，预期 affected 仅涉 notify 相关符号或空——索引降级判定见 Global Constraints），然后：

```bash
git add internal/crisis/notify.go internal/crisis/notify_render.go internal/crisis/notify_test.go internal/crisis/notify_render_test.go
git commit -m "feat(crisis): route NORMAL weekly summary via SummaryKind

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 2: renderWeekly NORMAL 尾段（去退出进度行）

**Files:**
- Modify: `internal/crisis/notify_render.go:285-293`（`renderWeekly`）
- Test: `internal/crisis/notify_render_test.go`（`TestRenderWeekly` 之后新增用例）

**Interfaces:**
- Consumes: Task 1 的 `NotifyContext.Summary`（本任务不直接用，仅同包共存）。
- Produces: `renderWeekly` 对 `res.State == StateNormal` 输出无「退出进度」行的周报；WATCH 输出不变。

- [ ] **Step 1: 新增失败测试**

`internal/crisis/notify_render_test.go` 在 `TestRenderWeekly`（第 461 行）之后新增：

```go
// NORMAL 周报：复用骨架但无退出进度行（NORMAL 态无退出概念，设计 §3.3）。
func TestRenderWeeklyNormal(t *testing.T) {
	cfg := testConfig()
	res := dayResult(StateNormal, StateNormal)
	res.Date = "2026-07-13"
	msg := renderWeekly(cfg, NotifyContext{Res: res, StateDays: 30})
	assert.True(t, strings.HasPrefix(msg, "[P1] 📅 Cassandra 周报 · 07-13 当周 · NORMAL 已持续 30 个评估日"))
	assert.Contains(t, msg, "7 指标全绿：")
	assert.NotContains(t, msg, "退出进度")
	assert.Contains(t, msg, "下次周报：下周一 · 状态变更即时通知")
	assert.True(t, strings.HasSuffix(msg, notifyFooter))
}
```

- [ ] **Step 2: 运行验证失败**

Run: `go test ./internal/crisis/ -run TestRenderWeeklyNormal -v`
Expected: FAIL（当前输出含「退出进度：触发条件已连续解除 0 日…」）

- [ ] **Step 3: 实现尾段分叉**

`internal/crisis/notify_render.go` 的 `renderWeekly` 整体替换为：

```go
// renderWeekly 消息 5：WATCH/NORMAL 周报（通知设计 §5.5，退出进度见 §6.6）。
// 退出进度行仅 WATCH 态渲染——NORMAL 态无退出概念（NORMAL 周报设计 §3.3）。
func renderWeekly(cfg *Config, nc NotifyContext) string {
	res := nc.Res
	first := fmt.Sprintf("[P1] 📅 Cassandra 周报 · %s 当周 · %s 已持续 %d 个评估日",
		monthDay(res.Date), res.State, nc.StateDays)
	tail := "下次周报：下周一 · 状态变更即时通知"
	if res.State == StateWatch {
		tail = fmt.Sprintf("退出进度：触发条件已连续解除 %d 日（回 NORMAL 需连续 %d 日）\n",
			nc.ClearStreak, cfg.StateMachine.WatchExitDays) + tail
	}
	return strings.Join([]string{first, bodyZones(cfg, res, "异常指标："), tail}, "\n\n") + notifyFooter
}
```

- [ ] **Step 4: 运行验证通过（含 WATCH 回归）**

Run: `go test ./internal/crisis/ -run 'TestRenderWeekly' -v`
Expected: `TestRenderWeekly` 与 `TestRenderWeeklyNormal` 均 PASS（WATCH 输出逐字节不变）
Run: `go test ./internal/crisis/`
Expected: `ok`

- [ ] **Step 5: 提交**

先跑 `gitnexus_detect_changes`（判定标准同前），然后：

```bash
git add internal/crisis/notify_render.go internal/crisis/notify_render_test.go
git commit -m "feat(crisis): NORMAL weekly report drops exit-progress line

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 3: cmd 层 — summaryKind 判日与 buildNotifyContext 组装

**Files:**
- Modify: `cmd/atlas/crisis.go:344,392,400-401,431-445`（`buildNotifyContext`、`summaryDue`→`summaryKind`）
- Test: `cmd/atlas/crisis_test.go:690-698,1034,1119,1129,1139` + 新增 NORMAL 周一组装用例

**Interfaces:**
- Consumes: Task 1 的 `crisis.SummaryKind` / `crisis.SummaryNone` / `crisis.SummaryWeekly` / `crisis.SummaryMonthly`、`NotifyContext.Summary`。
- Produces: `func summaryKind(date string, state crisis.SystemState) crisis.SummaryKind`（包内私有，替换原 `summaryDue`）。

- [ ] **Step 1: 重写触发测试（RED）**

`cmd/atlas/crisis_test.go` 第 690–698 行的 `TestSummaryDue` 整体替换为：

```go
func TestSummaryKind(t *testing.T) {
	assert.Equal(t, crisis.SummaryMonthly, summaryKind("2026-07-01", crisis.StateNormal)) // 周三 = 当月首交易日 → 月报
	assert.Equal(t, crisis.SummaryNone, summaryKind("2026-07-02", crisis.StateNormal))
	assert.Equal(t, crisis.SummaryMonthly, summaryKind("2026-08-03", crisis.StateNormal)) // 8/1 周六 → 首交易日 = 8/3 周一，撞日归月报
	assert.Equal(t, crisis.SummaryMonthly, summaryKind("2026-06-01", crisis.StateNormal)) // 6/1 周一 ∧ 首交易日 → 撞日只发月报
	assert.Equal(t, crisis.SummaryWeekly, summaryKind("2026-07-13", crisis.StateNormal))  // NORMAL 普通周一 → 周报（本需求核心）
	assert.Equal(t, crisis.SummaryNone, summaryKind("2026-07-14", crisis.StateNormal))    // NORMAL 周二 → 静默
	assert.Equal(t, crisis.SummaryWeekly, summaryKind("2026-07-13", crisis.StateWatch))   // WATCH 周一 → 周报（回归）
	assert.Equal(t, crisis.SummaryNone, summaryKind("2026-07-14", crisis.StateWatch))
	assert.Equal(t, crisis.SummaryNone, summaryKind("2026-07-13", crisis.StateBrewing)) // BREWING 走日报，不走摘要
	assert.Equal(t, crisis.SummaryNone, summaryKind("bad-date", crisis.StateNormal))    // 坏日期不发
	assert.Equal(t, crisis.SummaryNone, summaryKind("2026-07-04", crisis.StateNormal))  // 周六 → 非交易日
}
```

同文件 4 处组装断言替换：

- 第 1034 行：`assert.True(t, nc.SummaryDue)` → `assert.Equal(t, crisis.SummaryWeekly, nc.Summary)`
- 第 1119 行：`assert.True(t, ncN.SummaryDue)` → `assert.Equal(t, crisis.SummaryMonthly, ncN.Summary)`
- 第 1129 行：`assert.False(t, ncD.SummaryDue)` → `assert.Equal(t, crisis.SummaryNone, ncD.Summary)`
- 第 1139 行：`assert.True(t, ncT.SummaryDue)` → `assert.Equal(t, crisis.SummaryWeekly, ncT.Summary)`

（第 1112、1122 行注释中的 `SummaryDue` 相应改为 `Summary`/`SummaryWeekly`，第 1102 行注释「WATCH ∧ SummaryDue ∧ !AnyTrigger」改为「WATCH ∧ SummaryWeekly ∧ !AnyTrigger」。）

在 `TestBuildNotifyContextClearStreakConditions`（第 1141 行）之后新增：

```go
// NORMAL 周一：Summary=weekly，但不组装 Trends（仅月报）也不计 ClearStreak（仅 WATCH）。
func TestBuildNotifyContextNormalWeekly(t *testing.T) {
	d := newNotifyTestDeps(t)
	res := notifyDayResult("2026-07-13", crisis.StateNormal, crisis.StateNormal) // 周一，非月初
	res.Detail = crisis.SysDetail{AnyTrigger: false}
	nc, err := buildNotifyContext(context.Background(), d, res)
	require.NoError(t, err)
	assert.Equal(t, crisis.SummaryWeekly, nc.Summary)
	assert.Nil(t, nc.Trends)
	assert.Equal(t, 0, nc.ClearStreak)
}
```

- [ ] **Step 2: 运行验证失败**

Run: `go test ./cmd/atlas/ -run 'TestSummaryKind|TestBuildNotifyContext' 2>&1 | head -20`
Expected: 编译错误 `undefined: summaryKind`（及 `nc.SummaryDue undefined`）

- [ ] **Step 3: 实现 summaryKind 与组装改造**

`cmd/atlas/crisis.go` 第 431–445 行的 `summaryDue` 整体替换为：

```go
// summaryKind：NORMAL → 当月首个交易日发月报（设计 §4.3：不加第 4 个 plist，
// 在 daily eval 内判断），其余周一发周报（撞日归月报，NORMAL 周报设计 §3.1）；
// WATCH → 周一发周报；BREWING/CRISIS → 无摘要（日报已覆盖）。
func summaryKind(date string, state crisis.SystemState) crisis.SummaryKind {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return crisis.SummaryNone
	}
	switch state {
	case crisis.StateNormal:
		if isFirstTradingDayOfMonth(t) {
			return crisis.SummaryMonthly
		}
		if t.Weekday() == time.Monday {
			return crisis.SummaryWeekly
		}
	case crisis.StateWatch:
		if t.Weekday() == time.Monday {
			return crisis.SummaryWeekly
		}
	}
	return crisis.SummaryNone
}
```

`buildNotifyContext` 三处：

- 第 344 行：`nc := crisis.NotifyContext{Res: res, SummaryDue: summaryDue(res.Date, res.State)}` → `nc := crisis.NotifyContext{Res: res, Summary: summaryKind(res.Date, res.State)}`
- 第 392 行：`if res.State == crisis.StateWatch && nc.SummaryDue && !res.Detail.AnyTrigger {` → `if res.State == crisis.StateWatch && nc.Summary == crisis.SummaryWeekly && !res.Detail.AnyTrigger {`
- 第 400–401 行：注释「仅 SummaryDue ∧ NORMAL」改「仅 SummaryMonthly ∧ NORMAL」；`if nc.SummaryDue && res.State == crisis.StateNormal {` → `if nc.Summary == crisis.SummaryMonthly && res.State == crisis.StateNormal {`

- [ ] **Step 4: 运行验证通过**

Run: `go test ./cmd/atlas/ ./internal/crisis/`
Expected: 两包均 `ok`
Run: `go build ./...`
Expected: 无输出（编译通过）

- [ ] **Step 5: 提交**

先跑 `gitnexus_detect_changes`（判定标准同前），然后：

```bash
git add cmd/atlas/crisis.go cmd/atlas/crisis_test.go
git commit -m "feat(crisis): NORMAL sends weekly summary on Mondays, monthly wins on collision

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 4: 文档更新与终验

**Files:**
- Modify: `docs/ops/crisis-monitor-notifications.md`（§2 频率表 + §5 排障速查）

**Interfaces:**
- Consumes: Task 1–3 已落地的行为（文档描述与实现一致）。
- Produces: 运维手册与新行为对齐。

- [ ] **Step 1: 更新运维手册**

`docs/ops/crisis-monitor-notifications.md` §2 表格中 NORMAL 行：

```markdown
| NORMAL | `[P1] 📅` 周报（无退出进度行） | 每周一 1 条 |
| NORMAL | `[P1] 📅` 月报（含 21 观测日趋势 sparkline） | 每月首个交易日 1 条；与周一撞日只发月报 |
```

（替换原单行 `| NORMAL | [P1] 📅 月报（含 21 观测日趋势 sparkline） | 每月首个交易日 1 条 |`。）

§5 排障速查第一行「NORMAL 态非月初 / WATCH 态非周一属正常静默」改为：

```markdown
| 当日没收到任何消息 | NORMAL/WATCH 态非周一（且 NORMAL 非月初）属正常静默；再查 `logs/crisis-daily.out.log` 是否 `data not ready` 或 `already evaluated` |
```

- [ ] **Step 2: 全量验证**

Run: `go test ./... 2>&1 | tail -20`
Expected: 全部 `ok`（或既有跳过项不变）

- [ ] **Step 3: code-simplifier 门禁（全局规范）**

用 Task tool 调用 `subagent_type: "code-simplifier:code-simplifier"`，prompt：
「请检查并简化最近修改的代码文件：internal/crisis/notify.go、internal/crisis/notify_render.go、cmd/atlas/crisis.go」。
若有简化建议则应用并重跑 `go test ./internal/crisis/ ./cmd/atlas/`，确认仍全绿。

- [ ] **Step 4: 提交**

先跑 `gitnexus_detect_changes`（判定标准同前），然后：

```bash
git add docs/ops/crisis-monitor-notifications.md
git commit -m "docs(crisis): update notification runbook for NORMAL weekly report

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

（若 Step 3 产生代码简化，随同一提交或单独 `refactor(crisis)` 提交。）
