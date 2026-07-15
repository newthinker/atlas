# Cassandra 危机监控通知模板实施方案

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**设计文档**：`docs/plans/2026-07-14-crisis-notification-templates-design.md`（v1.0，本方案唯一需求来源）
**基础**：`docs/plans/2026-07-13-macro-crisis-monitor-impl.md` 已全部落地（`internal/crisis` 包 + `cmd/atlas/crisis.go` 现存于 master）。本方案是对既有代码的**通知层重写**，不新增采集/规则/状态机能力。

**Goal**：按通知设计 v1.0 重写 crisis 通知层——两级消息家族（五段结构化正文 + 单行速报）、七类消息、emoji 色点、语义句查表、月报 sparkline、日报差异行、周报退出进度，渲染保持纯函数、输入收拢为 `NotifyContext`。

**Architecture**：`internal/crisis` 内拆出 `notify_format.go`（层名/emoji/数值格式/sparkline 等无状态格式化）与 `notify_render.go`（`NotifyContext`/`Trend` 类型 + 七类渲染器），`notify.go` 只留 `Sender`、`Messages` 装配与 `FormatIntradayAlert`；`cmd/atlas/crisis.go` 新增 `buildNotifyContext` 在 `AppendEvaluations` **之前**组装上下文。规则层顺手填充 `PersistDays`/`Wow`/`WowOK` 三个新字段并写入评估行 detail JSON。

**Tech Stack**：Go 1.24.4、testify；无新依赖。全部消息走 `SendText` 纯文本（无 parse_mode）。

## Global Constraints

- 所有 `go build` / `go test` 前缀 `GOTOOLCHAIN=local`（仓库既有约束；sqlite 固定 v1.38.2 不动）。
- 通知文案**禁止出现"必然/一定/即将"**；结构化家族必含 `非交易信号` 页脚，速报家族**不带**页脚（设计 §2/§7）。语义句表与页脚集中为包级常量。
- 阈值一律来自 `configs/crisis-monitor.yaml`；语义句含天数处用 `%d` 占位、渲染时注入 `state_machine` 配置值（设计 §4.1）。`persistLookbackObs=30` 是显示统计窗口上限而非规则阈值，允许留在代码（见补充决策 3）。
- 发送失败仅记 stderr、评估已落库（沿基础方案约定，本方案不改发送循环的错误语义）。
- **修改已有 symbol 前必须先跑 `gitnexus_impact({target, direction:"upstream"})` 并报告影响面**（涉及：Task 1 的 `evalVIX`/`evalSOFREFFR`/`evalUSDJPY`/`buildEvaluations`、Task 9 的 `Messages`/`executeCrisisEvalDaily`/`executeCrisisIntraday`）。HIGH/CRITICAL 风险先停下向用户说明。
- **每个任务提交前**：跑 `gitnexus_detect_changes()` 核对影响面；按用户全局规范运行 code-simplifier sub-agent（Task tool, subagent_type: `code-simplifier:code-simplifier`）。
- 分支：`feature/crisis-notify-templates`（执行时经 superpowers:using-git-worktrees 建隔离工作区）。提交格式 `feat(crisis): ...`。
- 每个任务结束时 `GOTOOLCHAIN=local go build ./...` 必须通过（Task 8 之前旧 `Messages` 签名保持不动，Task 9 一次性切换包内签名与 cmd 调用点，保证任一提交点全仓可编译）。

## 对设计 v1.0 的补充决策（已核实，执行者不需重新决策）

1. **`NotifyContext` 增加 `StaleLastObs map[string]string`**。P2 文案（设计 §5.6）需要"最后观测 07-09（滞后 5 日 > 阈值 4 日）"，设计 §8 的字段清单拿不到最后观测日；cmd 层用 `store.LatestObservation` 组装。阈值取 `freshness.daily_max_lag_days`（nfci 用 weekly），通道名按指标来源写死：move/usdjpy → Yahoo，其余 → FRED。
2. **sparkline 分桶**：设计 §6.4 说"近 21 个观测归一到八阶"但示例均为 7 字符——实现为 7 个连续桶取均值后 min-max 归一；观测 ≤7 时逐点渲染；全平序列全 `▄`；空窗口省略整行。
3. **`persistLookbackObs = 30`**：`PersistDays` 的统计回看窗口。设计 §5.3 示例"持续 9 个交易日"超过 `red_persist_days=5`，说明计数不受规则窗口限制；30 观测（约 6 周）足够覆盖任何值得播报的持续期，是显示口径不是规则阈值。
4. **5y 分位片段仅 vix/move/hy_oas/t10y2y/nfci 显示**：设计 §5 全部示例中 sofr_effr 与 usdjpy 一致省略该片段（usdjpy 用 52 周拥挤分位、sofr_effr 利差水平的 5y 分位无解读价值），写死 `showPct5y`。
5. **`7 指标全绿：` 标题仅当异常区为空且 7 指标全为 GREEN**；存在 ⚪（STALE/NO_DATA/季末抑制）时仍用 `其余指标：`，避免"全绿"失实。
6. **`StateDays` 语义**：状态变更消息 = **前一状态**的持续评估日数（示例"WATCH 已持续 12 个评估日 → BREWING"）；日报/周报/月报 = 当前状态持续日数（含当日）。cmd 在 `AppendEvaluations` 之前计算：变更日取 `PrevState` 的历史连续行数，无变更日取 `State` 的历史连续行数 +1。
7. **usdjpy `周跌` 片段的触发判定**：`WowOK && Wow <= indicators.usdjpy.amber_wow_pct`（渲染层读 cfg），避免 CROWDED-only 的黄灯误带周跌片段。
8. **`ClearStreak` 含当日**：`ClearStreakDays` 只统计历史行；当日的 `any_trigger` 在 cmd 层用 `res.Detail.AnyTrigger` 补上（today 行尚未落库）。

## 文件结构总览

```
internal/crisis/
├── types.go              # [改] IndicatorResult + PersistDays/Wow/WowOK（Task 1）
├── rules.go              # [改] evalVIX/evalSOFREFFR/evalUSDJPY 填充新字段（Task 1）
├── suppress.go           # [改] indDetail JSON 携带新字段（Task 1）
├── eval.go               # [改] buildEvaluations 序列化新字段（Task 1）
├── statemachine.go       # [改] + ClearStreakDays 导出助手（Task 2）
├── notify_format.go      # [新] 层名/emoji/tag/数值格式/sparkline/趋势箭头（Task 3）
├── notify_render.go      # [新] NotifyContext/Trend + 分区 + 七类渲染器（Task 4–7）
├── notify.go             # [重写] Sender + Messages(cfg,nc) + FormatIntradayAlert + 新页脚（Task 9）
├── notify_format_test.go # [新]（Task 3）
├── notify_render_test.go # [新]（Task 4–7）
└── notify_test.go        # [重写] 装配/禁词/页脚全覆盖（Task 9）
cmd/atlas/
├── crisis.go             # [改] buildNotifyContext + 调用切换 + intraday 文案（Task 8–9）
└── crisis_test.go        # [改] 组装单测 + 断言适配（Task 8–9）
```

## 核心接口契约（跨任务公共签名，后续任务以此为准）

```go
// ---- types.go（Task 1 追加字段，其余不动）----
type IndicatorResult struct {
    // ...既有 7 个字段...
    PersistDays int     // sofr_effr：满足当前档位阈值的连续观测数（通知文案用）
    Wow         float64 // usdjpy/vix：周环比；WowOK=false 时无效
    WowOK       bool
}

// ---- statemachine.go（Task 2）----
func ClearStreakDays(hist EvalHistory, max int) (int, error) // any_trigger=false 连续历史日数

// ---- notify_render.go（Task 4，设计 §8 + 补充决策 1/6/8）----
type Trend struct {
    Window []Observation // 近 21 观测（可短）
    Delta  float64       // 当前 − 窗口首
}
type NotifyContext struct {
    Res          *DayResult
    StateDays    int                   // 变更消息=前状态持续日数；否则=当前状态含当日
    SummaryDue   bool                  // 月报/周报到期（cmd 计算）
    NewStale     []string              // 今日新进入 STALE 的指标（P2 去重后）
    StaleLastObs map[string]string     // NewStale 指标的最后观测日（补充决策 1）
    PrevDay      map[string]Evaluation // 前一评估日指标行（较昨日 & NewStale 依据）
    ClearStreak  int                   // any_trigger=false 连续日数，含当日（周报退出进度）
    Trends       map[string]Trend      // 仅月报到期时组装
}

// 渲染器（包内私有，Task 4–7 逐个落地，Task 9 由 Messages 装配）
func indicatorLine(cfg *Config, r IndicatorResult) string
func splitZones(res *DayResult) (abnormal, rest []IndicatorResult)
func bodyZones(cfg *Config, res *DayResult, abnormalTitle string) string
func renderTransition(cfg *Config, nc NotifyContext) string // 消息 1/2
func renderDaily(cfg *Config, nc NotifyContext) string      // 消息 3
func renderMonthly(cfg *Config, nc NotifyContext) string    // 消息 4
func renderWeekly(cfg *Config, nc NotifyContext) string     // 消息 5
func renderOpsAlert(cfg *Config, nc NotifyContext, ind string) string // 消息 6

// ---- notify.go（Task 9 重写；旧 Messages(res, days, due, stale) 同任务删除）----
func Messages(cfg *Config, nc NotifyContext) []string // 消息 1–6
func FormatIntradayAlert(price, base, wow float64, state SystemState, at time.Time) string // 消息 7

// ---- cmd/atlas/crisis.go（Task 8）----
func buildNotifyContext(ctx context.Context, d crisisEvalDeps, res *crisis.DayResult) (crisis.NotifyContext, error)
```

## 任务总览

| Task | 内容 | 产出 |
|---|---|---|
| 1 | IndicatorResult 新字段 + 规则层填充 + detail JSON | types/rules/suppress/eval |
| 2 | `ClearStreakDays` 导出助手 | statemachine.go |
| 3 | notify_format.go：格式化原语 + sparkline | 新文件 |
| 4 | notify_render.go（一）：类型、指标行、分区 | 新文件 |
| 5 | notify_render.go（二）：语义句表 + 状态变更渲染 | §5.1/5.2 |
| 6 | notify_render.go（三）：日报（较昨日）+ 周报（退出进度） | §5.3/5.5 |
| 7 | notify_render.go（四）：月报（趋势区）+ P2 速报 | §5.4/5.6 |
| 8 | cmd 层 `buildNotifyContext`（旧 Messages 暂不动） | cmd/atlas |
| 9 | 切换：notify.go 重写 + cmd 调用切换 + 测试重写 | 全链路 |

---

# 任务分解

### Task 1: IndicatorResult 新字段与规则层填充

**Files:**
- Modify: `internal/crisis/types.go:75-83`（IndicatorResult）、`internal/crisis/rules.go:93-109`（evalVIX）、`internal/crisis/rules.go:124-138`（evalSOFREFFR）、`internal/crisis/rules.go:205-228`（evalUSDJPY）、`internal/crisis/suppress.go:77-80`（indDetail）、`internal/crisis/eval.go:71`（buildEvaluations）
- Test: `internal/crisis/rules_test.go`（追加）、`internal/crisis/eval_test.go`（追加）

**Interfaces:**
- Consumes: 既有 `EvaluateIndicator`/`WowPct`/`allAbove`/`lastN`；测试夹具 `testConfig()`/`baselineSeries()`/`seriesEnding()`（rules_test.go:51-94）
- Produces: `IndicatorResult.PersistDays/Wow/WowOK`（Task 4 渲染消费）；评估行 detail JSON 新增 `persist_days`/`wow`/`wow_ok`（审计与"较昨日"可用）

- [ ] **Step 1: 影响面分析（项目 MUST 规则）**

运行 `gitnexus_impact({target: "evalSOFREFFR", direction: "upstream"})`、同样对 `evalVIX`、`evalUSDJPY`、`buildEvaluations`，报告直接调用方与风险级别。预期调用链均收敛于 `EvaluateIndicator`/`EvalDay`（包内）。本次改动只**新增输出字段**、不改判定结果（evalSOFREFFR 的窗口从 `RedPersistDays` 放宽到 30 条后用 `lastN` 还原原判定窗口，语义等价）。若报告 HIGH/CRITICAL，先停下向用户说明再继续。

- [ ] **Step 2: 写失败测试** — 在 `internal/crisis/rules_test.go` 末尾追加：

```go
// TestIndicatorResultPersistAndWow 覆盖通知模板需要的持续性/周环比字段
// （通知设计 §8：规则层计算时顺手填充）。
func TestIndicatorResultPersistAndWow(t *testing.T) {
	cfg := testConfig()
	const d = "2026-07-10"

	// sofr_effr：末尾连续 9 个观测 > red_bp(25) → RED 且 PersistDays=9
	// （9 > red_persist_days=5：计数不受规则窗口限制，补充决策 3）
	sr := baselineSeries(d)
	sr[IndSOFREFFR] = append(
		seriesEnding(addDays(d, -9), 40, -10, -10),
		seriesEnding(d, 9, 30, 30)...)
	res, err := EvaluateIndicator(cfg, IndSOFREFFR, d, sr)
	require.NoError(t, err)
	assert.Equal(t, StatusRed, res.RawStatus)
	assert.Equal(t, 9, res.PersistDays)

	// amber 档：末尾 3 个观测在 (10, 25] → AMBER 且 PersistDays=3
	sr[IndSOFREFFR] = append(
		seriesEnding(addDays(d, -3), 40, -10, -10),
		seriesEnding(d, 3, 15, 15)...)
	res, err = EvaluateIndicator(cfg, IndSOFREFFR, d, sr)
	require.NoError(t, err)
	assert.Equal(t, StatusAmber, res.RawStatus)
	assert.Equal(t, 3, res.PersistDays)

	// usdjpy：周环比恰为 red_wow_pct(-3%) → RED，Wow/WowOK 填充
	sr = baselineSeries(d)
	sr[IndUSDJPY] = seriesEnding(d, 80, 160, 155.2) // 155.2/160−1 = −3.0%
	res, err = EvaluateIndicator(cfg, IndUSDJPY, d, sr)
	require.NoError(t, err)
	assert.Equal(t, StatusRed, res.RawStatus)
	assert.True(t, res.WowOK)
	assert.InDelta(t, -0.03, res.Wow, 0.0001)

	// vix：wow 同样填充（基线全平 → wow=0 但 ok=true）
	res, err = EvaluateIndicator(cfg, IndVIX, d, baselineSeries(d))
	require.NoError(t, err)
	assert.True(t, res.WowOK)
	assert.InDelta(t, 0, res.Wow, 0.0001)
}
```

在 `internal/crisis/eval_test.go` 末尾追加（若文件未导入 `time`，补充导入）：

```go
// 评估行 detail JSON 同步携带新字段（审计与"较昨日"对比，通知设计 §8）。
func TestBuildEvaluationsCarriesPersistAndWow(t *testing.T) {
	r := &DayResult{Date: "2026-07-10", Results: map[string]IndicatorResult{}}
	for _, ind := range AllIndicators {
		r.Results[ind] = IndicatorResult{Indicator: ind, Status: StatusGreen, RawStatus: StatusGreen}
	}
	r.Results[IndSOFREFFR] = IndicatorResult{Indicator: IndSOFREFFR,
		Status: StatusRed, RawStatus: StatusRed, PersistDays: 9}
	r.Results[IndUSDJPY] = IndicatorResult{Indicator: IndUSDJPY,
		Status: StatusRed, RawStatus: StatusRed, Wow: -0.031, WowOK: true}

	evals, err := buildEvaluations(r, time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	byInd := map[string]Evaluation{}
	for _, e := range evals {
		byInd[e.Indicator] = e
	}
	assert.Contains(t, byInd[IndSOFREFFR].Detail, `"persist_days":9`)
	assert.Contains(t, byInd[IndUSDJPY].Detail, `"wow":-0.031`)
	assert.Contains(t, byInd[IndUSDJPY].Detail, `"wow_ok":true`)
}
```

- [ ] **Step 3: 运行确认失败**

Run: `GOTOOLCHAIN=local go test ./internal/crisis/ -run 'TestIndicatorResultPersistAndWow|TestBuildEvaluationsCarriesPersistAndWow' -v`
Expected: 编译失败，`unknown field PersistDays`

- [ ] **Step 4: 实现**

`internal/crisis/types.go` — `IndicatorResult` 追加三个字段（放在 `WindowActualObs` 之后）：

```go
type IndicatorResult struct {
	Indicator       string
	Status          Status
	RawStatus       Status
	Tag             Tag
	Value           float64
	Pct5y           float64
	WindowActualObs int
	PersistDays     int     // sofr_effr：满足当前档位阈值的连续观测数（通知文案用）
	Wow             float64 // usdjpy/vix：周环比；WowOK=false 时无效
	WowOK           bool
}
```

`internal/crisis/suppress.go` — `indDetail` 追加字段：

```go
type indDetail struct {
	Raw             Status  `json:"raw"`
	WindowActualObs int     `json:"window_actual_obs"`
	PersistDays     int     `json:"persist_days,omitempty"`
	Wow             float64 `json:"wow,omitempty"`
	WowOK           bool    `json:"wow_ok,omitempty"`
}
```

`internal/crisis/eval.go:71` — marshal 处改为：

```go
		d, err := json.Marshal(indDetail{Raw: ir.RawStatus, WindowActualObs: ir.WindowActualObs,
			PersistDays: ir.PersistDays, Wow: ir.Wow, WowOK: ir.WowOK})
```

`internal/crisis/rules.go` — 三个 eval 函数改造 + 两个新助手：

```go
// persistLookbackObs caps the "持续 N 个交易日" count in notifications. It is
// a display bound, not a rule threshold (thresholds live in YAML); ~6 weeks
// covers any persistence worth reporting.
const persistLookbackObs = 30

func evalVIX(cfg *Config, date string, sr SeriesReader, res *IndicatorResult) error {
	c := cfg.Indicators.VIX
	switch {
	case res.Value > c.Red:
		res.RawStatus = StatusRed
	case res.Value >= c.Amber:
		res.RawStatus = StatusAmber
	}
	win, err := sr.Window(IndVIX, date, 6)
	if err != nil {
		return err
	}
	if wow, ok := WowPct(win); ok {
		res.Wow, res.WowOK = wow, true
		if wow > c.WeeklySpikePct {
			res.RawStatus = maxStatus(res.RawStatus, StatusAmber)
		}
	}
	return nil
}

// evalSOFREFFR: persistence conditions are the core noise filter (design
// §3.1 note 3) — red needs red_persist_days consecutive observations above
// red_bp, amber likewise over its own window. The wider lookback only feeds
// PersistDays for notifications; the rule windows are unchanged via lastN.
func evalSOFREFFR(cfg *Config, date string, sr SeriesReader, res *IndicatorResult) error {
	c := cfg.Indicators.SOFREFFR
	win, err := sr.Window(IndSOFREFFR, date, persistLookbackObs)
	if err != nil {
		return err
	}
	if len(win) >= c.RedPersistDays && allAbove(lastN(win, c.RedPersistDays), c.RedBp) {
		res.RawStatus = StatusRed
		res.PersistDays = consecutiveAbove(win, c.RedBp)
		return nil
	}
	if len(win) >= c.AmberPersistDays && allAbove(lastN(win, c.AmberPersistDays), c.AmberBp) {
		res.RawStatus = StatusAmber
		res.PersistDays = consecutiveAbove(win, c.AmberBp)
	}
	return nil
}

// consecutiveAbove counts trailing observations strictly above threshold.
func consecutiveAbove(obs []Observation, threshold float64) int {
	n := 0
	for i := len(obs) - 1; i >= 0; i-- {
		if obs[i].Value <= threshold {
			break
		}
		n++
	}
	return n
}
```

`evalUSDJPY` 中 wow 分支改为（其余不动）：

```go
	if wow, ok := WowPct(win); ok {
		res.Wow, res.WowOK = wow, true
		switch {
		case wow <= c.RedWowPct:
			res.RawStatus = StatusRed
		case wow <= c.AmberWowPct:
			res.RawStatus = maxStatus(res.RawStatus, StatusAmber)
		}
	}
```

- [ ] **Step 5: 运行确认通过（全包，确认防抖/抑制等既有测试不回归）**

Run: `GOTOOLCHAIN=local go test ./internal/crisis/ -v`
Expected: PASS（新增 2 个测试 + 既有测试全绿）

- [ ] **Step 6: 提交（先跑 gitnexus detect_changes 与 code-simplifier）**

```bash
git add internal/crisis/
git commit -m "feat(crisis): expose persist days and wow on indicator results"
```

### Task 2: `ClearStreakDays` 导出助手

**Files:**
- Modify: `internal/crisis/statemachine.go`（文件末尾追加）
- Test: `internal/crisis/statemachine_test.go`（追加）

**Interfaces:**
- Consumes: `EvalHistory.RecentSystem`、`SysDetail`（statemachine.go:9-15）
- Produces: `ClearStreakDays(hist EvalHistory, max int) (int, error)`（Task 8 cmd 消费，周报退出进度 §6.6）

- [ ] **Step 1: 写失败测试** — 在 `internal/crisis/statemachine_test.go` 末尾追加（若与文件既有系统行构造助手重名，复用既有助手改写为等价断言）：

```go
func clearStreakEval(date string, anyTrigger bool) Evaluation {
	d, _ := json.Marshal(SysDetail{Date: date, AnyTrigger: anyTrigger, Prev: StateWatch})
	return Evaluation{TS: date, Indicator: "", SystemState: StateWatch, Detail: string(d)}
}

// ClearStreakDays：any_trigger=false 的连续历史日数（周报退出进度，设计 §6.6）。
func TestClearStreakDays(t *testing.T) {
	h := NewMemHistory()
	h.Append([]Evaluation{clearStreakEval("2026-07-06", true)})
	h.Append([]Evaluation{clearStreakEval("2026-07-07", false)})
	h.Append([]Evaluation{clearStreakEval("2026-07-08", false)})
	n, err := ClearStreakDays(h, 20)
	require.NoError(t, err)
	assert.Equal(t, 2, n) // 最新两行 false，第三行 true 中断

	// 空历史 → 0
	n, err = ClearStreakDays(NewMemHistory(), 20)
	require.NoError(t, err)
	assert.Equal(t, 0, n)

	// 坏 detail 行 → 中断计数而非上抛（同 systemDetailStreak 的保守约定）
	h2 := NewMemHistory()
	h2.Append([]Evaluation{{TS: "2026-07-07", Detail: "not-json"}})
	h2.Append([]Evaluation{clearStreakEval("2026-07-08", false)})
	n, err = ClearStreakDays(h2, 20)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
}
```

- [ ] **Step 2: 运行确认失败**

Run: `GOTOOLCHAIN=local go test ./internal/crisis/ -run TestClearStreakDays -v`
Expected: 编译失败，`undefined: ClearStreakDays`

- [ ] **Step 3: 实现** — `internal/crisis/statemachine.go` 末尾追加：

```go
// ClearStreakDays counts consecutive historical system rows (newest first,
// looking back at most max rows) whose detail carries any_trigger=false —
// the weekly report's exit-progress numerator (通知设计 §6.6). Today's row is
// not yet persisted when the caller assembles the NotifyContext, so the
// caller adds the current day itself. Unparseable detail breaks the streak
// conservatively instead of erroring (same convention as systemDetailStreak).
func ClearStreakDays(hist EvalHistory, max int) (int, error) {
	prev, err := hist.RecentSystem(max)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range prev {
		var d SysDetail
		if err := json.Unmarshal([]byte(e.Detail), &d); err != nil || d.AnyTrigger {
			break
		}
		n++
	}
	return n, nil
}
```

- [ ] **Step 4: 运行确认通过**

Run: `GOTOOLCHAIN=local go test ./internal/crisis/ -v`
Expected: PASS

- [ ] **Step 5: 提交（先跑 gitnexus detect_changes 与 code-simplifier）**

```bash
git add internal/crisis/statemachine.go internal/crisis/statemachine_test.go
git commit -m "feat(crisis): add ClearStreakDays helper for weekly exit progress"
```

### Task 3: notify_format.go — 格式化原语与 sparkline

**Files:**
- Create: `internal/crisis/notify_format.go`
- Test: `internal/crisis/notify_format_test.go`

**Interfaces:**
- Consumes: `Status`/`Tag`/`Observation`/`severity`（types.go）；测试复用 `seriesEnding`（rules_test.go:51）、`addDays`（dates.go）
- Produces: `layerName`/`icebergRank`/`statusEmoji`/`nonColorNote`/`tagText`/`formatReading`/`formatDelta`/`deltaEpsilon`/`trendArrow`/`showPct5y`/`formatPct5y`/`sparkline`（Task 4–7 渲染消费；全部包内私有纯函数）

- [ ] **Step 1: 写失败测试** — 创建 `internal/crisis/notify_format_test.go`：

```go
package crisis

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// 设计 §6.2/§6.1/§6.3：层名映射、冰山层序、emoji、非色彩说明、tag 中英文。
func TestLayerEmojiAndTagText(t *testing.T) {
	assert.Equal(t, "情绪", layerName(IndVIX))
	assert.Equal(t, "情绪", layerName(IndMOVE))
	assert.Equal(t, "流动性", layerName(IndSOFREFFR))
	assert.Equal(t, "信用", layerName(IndHYOAS))
	assert.Equal(t, "领先", layerName(IndT10Y2Y))
	assert.Equal(t, "领先", layerName(IndNFCI))
	assert.Equal(t, "旁证", layerName(IndUSDJPY))

	// 冰山层序：信用→流动性→情绪→领先→旁证（深层异常优先看）
	assert.True(t, icebergRank(IndHYOAS) < icebergRank(IndSOFREFFR))
	assert.True(t, icebergRank(IndSOFREFFR) < icebergRank(IndVIX))
	assert.True(t, icebergRank(IndVIX) < icebergRank(IndT10Y2Y))
	assert.True(t, icebergRank(IndT10Y2Y) < icebergRank(IndUSDJPY))

	assert.Equal(t, "🔴", statusEmoji(StatusRed))
	assert.Equal(t, "🟡", statusEmoji(StatusAmber))
	assert.Equal(t, "🟢", statusEmoji(StatusGreen))
	assert.Equal(t, "⚪", statusEmoji(StatusStale))

	assert.Equal(t, "数据断更(STALE)", nonColorNote(StatusStale))
	assert.Equal(t, "无数据(NO_DATA)", nonColorNote(StatusNoData))
	assert.Equal(t, "季末抑制", nonColorNote(StatusSuppressed))
	assert.Equal(t, "", nonColorNote(StatusGreen))

	assert.Equal(t, "压力(STRESS)", tagText(TagStress))
	assert.Equal(t, "自满(COMPLACENCY)", tagText(TagComplacency))
	assert.Equal(t, "空头拥挤(CROWDED)", tagText(TagCrowded))
	assert.Equal(t, "倒挂后复陡(STEEPENING)", tagText(TagSteepening))
}

// 设计 §6.3：每指标写死一条读数/变化量格式。
func TestFormatReadingAndDelta(t *testing.T) {
	assert.Equal(t, "18.2", formatReading(IndVIX, 18.2))
	assert.Equal(t, "88.1", formatReading(IndMOVE, 88.1))
	assert.Equal(t, "161.7", formatReading(IndUSDJPY, 161.66))
	assert.Equal(t, "612bp", formatReading(IndHYOAS, 612.4))
	assert.Equal(t, "+28bp", formatReading(IndSOFREFFR, 28))
	assert.Equal(t, "-10bp", formatReading(IndSOFREFFR, -10))
	assert.Equal(t, "+35bp", formatReading(IndT10Y2Y, 35))
	assert.Equal(t, "-0.52", formatReading(IndNFCI, -0.52))

	assert.Equal(t, "-2.3", formatDelta(IndVIX, -2.3))
	assert.Equal(t, "+9bp", formatDelta(IndT10Y2Y, 9))
	assert.Equal(t, "-18bp", formatDelta(IndHYOAS, -18))
	assert.Equal(t, "-0.02", formatDelta(IndNFCI, -0.02))

	assert.Equal(t, "98%", formatPct5y(0.98))
	assert.False(t, showPct5y(IndSOFREFFR)) // 补充决策 4
	assert.False(t, showPct5y(IndUSDJPY))
	assert.True(t, showPct5y(IndVIX))
}

// 设计 §6.4：|Δ| 小于该指标显示精度一个单位 → →，否则 ↗/↘。
func TestTrendArrow(t *testing.T) {
	assert.Equal(t, "↘", trendArrow(IndVIX, -2.3))
	assert.Equal(t, "→", trendArrow(IndVIX, 0.05))     // < 0.1
	assert.Equal(t, "→", trendArrow(IndSOFREFFR, 0.9)) // < 1bp
	assert.Equal(t, "↗", trendArrow(IndT10Y2Y, 9))
	assert.Equal(t, "→", trendArrow(IndNFCI, -0.009)) // < 0.01
	assert.Equal(t, "↘", trendArrow(IndNFCI, -0.02))
}

// 设计 §6.4 + 补充决策 2：21 观测 → 7 桶；全平全 ▄；不足 7 逐点；空窗口空串。
func TestSparkline(t *testing.T) {
	assert.Equal(t, "▄▄▄▄▄▄▄", sparkline(seriesEnding("2026-07-10", 21, 5, 5)))

	var win []Observation
	for i := 0; i < 21; i++ {
		win = append(win, Observation{Date: addDays("2026-07-10", i-20), Value: float64(i)})
	}
	s := []rune(sparkline(win))
	assert.Len(t, s, 7)
	assert.Equal(t, '▁', s[0]) // 单调升序：首桶最低
	assert.Equal(t, '█', s[6]) // 末桶最高

	assert.Len(t, []rune(sparkline(win[:3])), 3) // 不足 7 逐点
	assert.Equal(t, "", sparkline(nil))
}
```

- [ ] **Step 2: 运行确认失败**

Run: `GOTOOLCHAIN=local go test ./internal/crisis/ -run 'TestLayerEmojiAndTagText|TestFormatReadingAndDelta|TestTrendArrow|TestSparkline' -v`
Expected: 编译失败，`undefined: layerName`

- [ ] **Step 3: 实现** — 创建 `internal/crisis/notify_format.go`：

```go
package crisis

import (
	"fmt"
	"strings"
)

// layerName 冰山层固定映射（通知设计 §6.2）。
func layerName(ind string) string {
	switch ind {
	case IndVIX, IndMOVE:
		return "情绪"
	case IndSOFREFFR:
		return "流动性"
	case IndHYOAS:
		return "信用"
	case IndT10Y2Y, IndNFCI:
		return "领先"
	case IndUSDJPY:
		return "旁证"
	}
	return ind
}

// icebergRank 异常区同级排序的冰山层序：信用→流动性→情绪→领先→旁证
// （深层异常优先看，通知设计 §6.2）。
func icebergRank(ind string) int {
	switch layerName(ind) {
	case "信用":
		return 0
	case "流动性":
		return 1
	case "情绪":
		return 2
	case "领先":
		return 3
	}
	return 4 // 旁证
}

func statusEmoji(s Status) string {
	switch s {
	case StatusGreen:
		return "🟢"
	case StatusAmber:
		return "🟡"
	case StatusRed:
		return "🔴"
	}
	return "⚪"
}

// nonColorNote ⚪ 状态的说明片段（通知设计 §6.1）。
func nonColorNote(s Status) string {
	switch s {
	case StatusStale:
		return "数据断更(STALE)"
	case StatusNoData:
		return "无数据(NO_DATA)"
	case StatusSuppressed:
		return "季末抑制"
	}
	return ""
}

// tagText 统一 中文(英文)（通知设计 §6.3）。
func tagText(t Tag) string {
	switch t {
	case TagStress:
		return "压力(STRESS)"
	case TagComplacency:
		return "自满(COMPLACENCY)"
	case TagCrowded:
		return "空头拥挤(CROWDED)"
	case TagSteepening:
		return "倒挂后复陡(STEEPENING)"
	}
	return ""
}

// formatReading 每指标读数格式（通知设计 §6.3 写死一条）。
func formatReading(ind string, v float64) string {
	switch ind {
	case IndVIX, IndMOVE, IndUSDJPY:
		return fmt.Sprintf("%.1f", v)
	case IndHYOAS:
		return fmt.Sprintf("%.0fbp", v)
	case IndSOFREFFR, IndT10Y2Y:
		return fmt.Sprintf("%+.0fbp", v)
	}
	return fmt.Sprintf("%+.2f", v) // nfci
}

// formatDelta 变化量格式（月报月变化与日报"较昨日"共用，通知设计 §6.3）。
func formatDelta(ind string, d float64) string {
	switch ind {
	case IndVIX, IndMOVE, IndUSDJPY:
		return fmt.Sprintf("%+.1f", d)
	case IndHYOAS, IndSOFREFFR, IndT10Y2Y:
		return fmt.Sprintf("%+.0fbp", d)
	}
	return fmt.Sprintf("%+.2f", d) // nfci
}

// deltaEpsilon 趋势箭头的"横盘"判定 = 该指标显示精度一个单位（通知设计 §6.4）。
func deltaEpsilon(ind string) float64 {
	switch ind {
	case IndVIX, IndMOVE, IndUSDJPY:
		return 0.1
	case IndNFCI:
		return 0.01
	}
	return 1 // bp 类
}

func trendArrow(ind string, delta float64) string {
	eps := deltaEpsilon(ind)
	switch {
	case delta >= eps:
		return "↗"
	case delta <= -eps:
		return "↘"
	}
	return "→"
}

// showPct5y：sofr_effr（利差水平 5y 分位无解读价值）与 usdjpy（用 52 周拥挤
// 分位）不显示 5y 分位片段（补充决策 4，设计 §5 示例一致省略）。
func showPct5y(ind string) bool {
	return ind != IndSOFREFFR && ind != IndUSDJPY
}

func formatPct5y(p float64) string { return fmt.Sprintf("%.0f%%", p*100) }

var sparkGlyphs = []rune("▁▂▃▄▅▆▇█")

// sparkline 八阶 min-max 归一（通知设计 §6.4 + 补充决策 2）：观测 >7 时分 7 个
// 连续桶取均值（示例均为 7 字符），不足 7 逐点；全平序列全 ▄；空窗口空串。
func sparkline(window []Observation) string {
	if len(window) == 0 {
		return ""
	}
	vals := bucketMeans(window, 7)
	lo, hi := vals[0], vals[0]
	for _, v := range vals {
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}
	if hi == lo {
		return strings.Repeat("▄", len(vals))
	}
	var b strings.Builder
	for _, v := range vals {
		idx := int((v - lo) / (hi - lo) * 8)
		if idx > 7 {
			idx = 7
		}
		b.WriteRune(sparkGlyphs[idx])
	}
	return b.String()
}

func bucketMeans(window []Observation, buckets int) []float64 {
	if len(window) <= buckets {
		out := make([]float64, len(window))
		for i, o := range window {
			out[i] = o.Value
		}
		return out
	}
	out := make([]float64, buckets)
	for i := 0; i < buckets; i++ {
		start, end := i*len(window)/buckets, (i+1)*len(window)/buckets
		var sum float64
		for _, o := range window[start:end] {
			sum += o.Value
		}
		out[i] = sum / float64(end-start)
	}
	return out
}
```

- [ ] **Step 4: 运行确认通过**

Run: `GOTOOLCHAIN=local go test ./internal/crisis/ -v`
Expected: PASS

- [ ] **Step 5: 提交（先跑 gitnexus detect_changes 与 code-simplifier）**

```bash
git add internal/crisis/notify_format.go internal/crisis/notify_format_test.go
git commit -m "feat(crisis): add notification formatting primitives and sparkline"
```

### Task 4: notify_render.go（一）— NotifyContext、指标行与分区

**Files:**
- Create: `internal/crisis/notify_render.go`
- Test: `internal/crisis/notify_render_test.go`

**Interfaces:**
- Consumes: Task 1 的 `PersistDays/Wow/WowOK`、Task 3 全部格式化原语；`severity`/`isColor`（types.go）；测试复用 `dayResult`（notify_test.go:17）、`testConfig`（rules_test.go:77）
- Produces: `Trend`/`NotifyContext`（导出，Task 8 cmd 消费）；`monthDay`/`stateRank`/`indicatorLine`/`splitZones`/`bodyZones`（Task 5–7 渲染消费）

- [ ] **Step 1: 写失败测试** — 创建 `internal/crisis/notify_render_test.go`：

```go
package crisis

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// 指标行渲染（通知设计 §5 示例逐条对照）。
func TestIndicatorLineRendering(t *testing.T) {
	cfg := testConfig()
	assert.Equal(t, "🔴 信用 hy_oas 612bp · 5y分位 98% · 压力(STRESS)",
		indicatorLine(cfg, IndicatorResult{Indicator: IndHYOAS, Status: StatusRed, Value: 612, Pct5y: 0.98, Tag: TagStress}))
	assert.Equal(t, "🔴 流动性 sofr_effr +28bp · 持续 5 个交易日",
		indicatorLine(cfg, IndicatorResult{Indicator: IndSOFREFFR, Status: StatusRed, Value: 28, Pct5y: 0.99, PersistDays: 5}))
	// wow 触发（-2.1% ≤ amber_wow_pct=-2%）→ 周跌片段
	assert.Equal(t, "🟡 旁证 usdjpy 158.9 · 周跌 2.1%",
		indicatorLine(cfg, IndicatorResult{Indicator: IndUSDJPY, Status: StatusAmber, Value: 158.9, Wow: -0.021, WowOK: true}))
	// CROWDED-only（wow 未触发）→ 无周跌片段（补充决策 7）
	assert.Equal(t, "🟡 旁证 usdjpy 161.7 · 空头拥挤(CROWDED)",
		indicatorLine(cfg, IndicatorResult{Indicator: IndUSDJPY, Status: StatusAmber, Value: 161.7, Wow: -0.005, WowOK: true, Tag: TagCrowded}))
	assert.Equal(t, "🟢 情绪 vix 18.2 · 5y分位 41%",
		indicatorLine(cfg, IndicatorResult{Indicator: IndVIX, Status: StatusGreen, Value: 18.2, Pct5y: 0.41}))
	// Pct5y<0 → 省略分位片段
	assert.Equal(t, "🟢 领先 nfci -0.52",
		indicatorLine(cfg, IndicatorResult{Indicator: IndNFCI, Status: StatusGreen, Value: -0.52, Pct5y: -1}))
	// ⚪ 状态（设计 §6.1）
	assert.Equal(t, "⚪ 情绪 move 88.1 · 数据断更(STALE)",
		indicatorLine(cfg, IndicatorResult{Indicator: IndMOVE, Status: StatusStale, Value: 88.1}))
	assert.Equal(t, "⚪ 领先 nfci 无数据(NO_DATA)",
		indicatorLine(cfg, IndicatorResult{Indicator: IndNFCI, Status: StatusNoData}))
}

// 分区排序（通知设计 §6.2，测试要点 2）：异常区严重度降序 + 冰山层序；⚪ 殿后。
func TestSplitZonesOrdering(t *testing.T) {
	res := dayResult(StateWatch, StateWatch)
	set := func(ind string, st Status) {
		r := res.Results[ind]
		r.Status = st
		res.Results[ind] = r
	}
	set(IndVIX, StatusRed)
	set(IndSOFREFFR, StatusRed)
	set(IndHYOAS, StatusAmber)
	set(IndMOVE, StatusStale)

	abnormal, rest := splitZones(res)
	var got []string
	for _, r := range abnormal {
		got = append(got, r.Indicator)
	}
	// 红先于黄；同为红按冰山层序：流动性(sofr) 先于 情绪(vix)
	assert.Equal(t, []string{IndSOFREFFR, IndVIX, IndHYOAS}, got)
	// 其余区固定 AllIndicators 序，⚪ 殿后
	var restInds []string
	for _, r := range rest {
		restInds = append(restInds, r.Indicator)
	}
	assert.Equal(t, []string{IndT10Y2Y, IndNFCI, IndUSDJPY, IndMOVE}, restInds)
}

// 区块标题（设计 §4 + 补充决策 5）。
func TestBodyZonesTitles(t *testing.T) {
	cfg := testConfig()
	res := dayResult(StateWatch, StateWatch) // 全绿
	body := bodyZones(cfg, res, "异常指标：")
	assert.True(t, strings.HasPrefix(body, "7 指标全绿：\n🟢 情绪 vix"))
	assert.NotContains(t, body, "异常指标：")

	r := res.Results[IndHYOAS]
	r.Status = StatusAmber
	res.Results[IndHYOAS] = r
	body = bodyZones(cfg, res, "触发共振：")
	assert.Contains(t, body, "触发共振：\n🟡 信用 hy_oas")
	assert.Contains(t, body, "\n\n其余指标：\n🟢 情绪 vix")

	// 全非异常但含 ⚪ → 不用"全绿"标题（补充决策 5）
	r = res.Results[IndHYOAS]
	r.Status = StatusStale
	res.Results[IndHYOAS] = r
	body = bodyZones(cfg, res, "异常指标：")
	assert.True(t, strings.HasPrefix(body, "其余指标：\n"))
	// dayResult 夹具 Value=1，formatReading(IndHYOAS, 1) → "1bp"
	assert.True(t, strings.HasSuffix(body, "⚪ 信用 hy_oas 1bp · 数据断更(STALE)"))
}

func TestMonthDayAndStateRank(t *testing.T) {
	assert.Equal(t, "07-14", monthDay("2026-07-14"))
	assert.True(t, stateRank(StateCrisis) > stateRank(StateBrewing))
	assert.True(t, stateRank(StateBrewing) > stateRank(StateWatch))
	assert.True(t, stateRank(StateWatch) > stateRank(StateNormal))
}
```

- [ ] **Step 2: 运行确认失败**

Run: `GOTOOLCHAIN=local go test ./internal/crisis/ -run 'TestIndicatorLine|TestSplitZones|TestBodyZones|TestMonthDay' -v`
Expected: 编译失败，`undefined: indicatorLine`

- [ ] **Step 3: 实现** — 创建 `internal/crisis/notify_render.go`：

```go
package crisis

import (
	"fmt"
	"sort"
	"strings"
)

// Trend is one indicator's monthly-summary window (通知设计 §8).
type Trend struct {
	Window []Observation // 近 21 观测（可短）
	Delta  float64       // 当前 − 窗口首
}

// NotifyContext 收拢渲染输入，cmd 层组装、渲染保持纯函数（通知设计 §8）。
type NotifyContext struct {
	Res          *DayResult
	StateDays    int                   // 变更消息=前状态持续日数；否则=当前状态含当日（补充决策 6）
	SummaryDue   bool                  // 月报/周报到期（cmd 计算）
	NewStale     []string              // 今日新进入 STALE 的指标（P2 去重后）
	StaleLastObs map[string]string     // NewStale 指标的最后观测日（补充决策 1）
	PrevDay      map[string]Evaluation // 前一评估日指标行（较昨日 & NewStale 依据）
	ClearStreak  int                   // any_trigger=false 连续日数，含当日（周报退出进度）
	Trends       map[string]Trend      // 仅月报到期时组装
}

// monthDay renders YYYY-MM-DD as MM-DD（首行规范，通知设计 §3）。
func monthDay(date string) string {
	if len(date) == 10 {
		return date[5:]
	}
	return date
}

// stateRank 严重度序（通知设计 §2）：NORMAL < WATCH < BREWING < CRISIS。
func stateRank(s SystemState) int {
	switch s {
	case StateWatch:
		return 1
	case StateBrewing:
		return 2
	case StateCrisis:
		return 3
	}
	return 0
}

// indicatorLine 渲染一行指标（通知设计 §5 示例格式）：
// 🔴 信用 hy_oas 612bp · 5y分位 98% · 压力(STRESS)
func indicatorLine(cfg *Config, r IndicatorResult) string {
	if note := nonColorNote(r.Status); note != "" {
		head := fmt.Sprintf("⚪ %s %s", layerName(r.Indicator), r.Indicator)
		if r.Status != StatusNoData {
			head += " " + formatReading(r.Indicator, r.Value)
		}
		return head + " · " + note
	}
	head := fmt.Sprintf("%s %s %s %s", statusEmoji(r.Status), layerName(r.Indicator),
		r.Indicator, formatReading(r.Indicator, r.Value))
	var parts []string
	if showPct5y(r.Indicator) && r.Pct5y >= 0 {
		parts = append(parts, "5y分位 "+formatPct5y(r.Pct5y))
	}
	if r.Indicator == IndSOFREFFR && severity(r.Status) >= severity(StatusAmber) && r.PersistDays > 0 {
		parts = append(parts, fmt.Sprintf("持续 %d 个交易日", r.PersistDays))
	}
	if r.Indicator == IndUSDJPY && severity(r.Status) >= severity(StatusAmber) &&
		r.WowOK && r.Wow <= cfg.Indicators.USDJPY.AmberWowPct {
		parts = append(parts, fmt.Sprintf("周跌 %.1f%%", -r.Wow*100))
	}
	if t := tagText(r.Tag); t != "" {
		parts = append(parts, t)
	}
	if len(parts) == 0 {
		return head
	}
	return head + " · " + strings.Join(parts, " · ")
}

// splitZones 通知设计 §6.2：异常区 = 🔴🟡，严重度降序、同级按冰山层序、再按
// AllIndicators 序；其余区 = 🟢 后接 ⚪（已退出共振，视觉最弱、殿后）。
func splitZones(res *DayResult) (abnormal, rest []IndicatorResult) {
	var noncolor []IndicatorResult
	for _, ind := range AllIndicators {
		r := res.Results[ind]
		switch {
		case severity(r.Status) >= severity(StatusAmber):
			abnormal = append(abnormal, r)
		case isColor(r.Status):
			rest = append(rest, r)
		default:
			noncolor = append(noncolor, r)
		}
	}
	sort.SliceStable(abnormal, func(i, j int) bool {
		if severity(abnormal[i].Status) != severity(abnormal[j].Status) {
			return severity(abnormal[i].Status) > severity(abnormal[j].Status)
		}
		return icebergRank(abnormal[i].Indicator) < icebergRank(abnormal[j].Indicator)
	})
	return abnormal, append(rest, noncolor...)
}

// bodyZones 渲染异常区 + 其余区（通知设计 §4 骨架第三段）。
func bodyZones(cfg *Config, res *DayResult, abnormalTitle string) string {
	abnormal, rest := splitZones(res)
	lines := func(rs []IndicatorResult) string {
		out := make([]string, len(rs))
		for i, r := range rs {
			out[i] = indicatorLine(cfg, r)
		}
		return strings.Join(out, "\n")
	}
	restTitle := "其余指标："
	if len(abnormal) == 0 {
		if len(rest) == len(AllIndicators) { // 全为色彩且无异常 = 全绿（补充决策 5）
			restTitle = "7 指标全绿："
		}
		return restTitle + "\n" + lines(rest)
	}
	return abnormalTitle + "\n" + lines(abnormal) + "\n\n" + restTitle + "\n" + lines(rest)
}
```

- [ ] **Step 4: 运行确认通过**

Run: `GOTOOLCHAIN=local go test ./internal/crisis/ -v`
Expected: PASS

- [ ] **Step 5: 提交（先跑 gitnexus detect_changes 与 code-simplifier）**

```bash
git add internal/crisis/notify_render.go internal/crisis/notify_render_test.go
git commit -m "feat(crisis): add NotifyContext, indicator line and zone rendering"
```

### Task 5: notify_render.go（二）— 语义句表与状态变更渲染

**Files:**
- Modify: `internal/crisis/notify_render.go`（追加）
- Test: `internal/crisis/notify_render_test.go`（追加）

**Interfaces:**
- Consumes: Task 4 的 `bodyZones`/`monthDay`/`stateRank`；`Config.StateMachine`（config.go:107）
- Produces: `notifyFooter`（包级常量，新页脚，通知设计 §4.2）、`semanticSentence(cfg, from, to)`、`renderTransition(cfg, nc)`（消息 1/2）

- [ ] **Step 1: 写失败测试** — 在 `internal/crisis/notify_render_test.go` 追加：

```go
// 状态升级（§5.1 逐段对照）：首行/语义句/触发共振/尾注/页脚。
func TestRenderTransitionUpgrade(t *testing.T) {
	cfg := testConfig()
	res := dayResult(StateWatch, StateBrewing)
	res.Date = "2026-07-14"
	set := func(ind string, r IndicatorResult) { res.Results[ind] = r }
	set(IndHYOAS, IndicatorResult{Indicator: IndHYOAS, Status: StatusRed, Value: 612, Pct5y: 0.98, Tag: TagStress})
	set(IndSOFREFFR, IndicatorResult{Indicator: IndSOFREFFR, Status: StatusRed, Value: 28, PersistDays: 5})

	msg := renderTransition(cfg, NotifyContext{Res: res, StateDays: 12})
	assert.True(t, strings.HasPrefix(msg, "[P0] 🚨 状态升级 WATCH → BREWING · 07-14\n\n"))
	assert.Contains(t, msg, "信用与流动性双红共振")
	assert.Contains(t, msg, "触发共振：\n🔴 信用 hy_oas 612bp · 5y分位 98% · 压力(STRESS)\n🔴 流动性 sofr_effr +28bp · 持续 5 个交易日")
	assert.Contains(t, msg, "\n\n其余指标：\n")
	assert.Contains(t, msg, "WATCH 已持续 12 个评估日 → BREWING · 下一评估：下一交易日")
	assert.True(t, strings.HasSuffix(msg, notifyFooter))

	// 进 WATCH（非 BREWING/CRISIS）→ [P1] ⚠️
	msg = renderTransition(cfg, NotifyContext{Res: dayResult(StateNormal, StateWatch), StateDays: 63})
	assert.True(t, strings.HasPrefix(msg, "[P1] ⚠️ 状态升级 NORMAL → WATCH"))
	assert.Contains(t, msg, "领先层或多指标共振异常")

	// 进 CRISIS → [P0] 且危机语义句
	msg = renderTransition(cfg, NotifyContext{Res: dayResult(StateBrewing, StateCrisis), StateDays: 34})
	assert.True(t, strings.HasPrefix(msg, "[P0] 🚨 状态升级 BREWING → CRISIS"))
	assert.Contains(t, msg, "情绪层双红：危机进行中")
}

// 状态降级（§5.2）+ 语义句 %d 注入跟随配置（测试要点 6）。
func TestRenderTransitionDowngrade(t *testing.T) {
	cfg := testConfig()
	res := dayResult(StateBrewing, StateWatch)
	res.Date = "2026-09-02"
	r := res.Results[IndHYOAS]
	r.Status = StatusAmber
	res.Results[IndHYOAS] = r

	msg := renderTransition(cfg, NotifyContext{Res: res, StateDays: 34})
	assert.True(t, strings.HasPrefix(msg, "[P1] ✅ 状态解除 BREWING → WATCH · 09-02"))
	assert.Contains(t, msg, "稳定 10 个交易日") // brewing_exit_days=10
	assert.Contains(t, msg, "仍异常：\n🟡 信用 hy_oas")
	assert.Contains(t, msg, "BREWING 共持续 34 个评估日 · 下一评估：下一交易日")

	cfg.StateMachine.BrewingExitDays = 12 // YAML 调参 → 文案跟随
	assert.Contains(t, renderTransition(cfg, NotifyContext{Res: res, StateDays: 34}), "稳定 12 个交易日")

	// CRISIS→WATCH 与 WATCH→NORMAL 分别注入 crisis_exit_days / watch_exit_days
	msg = renderTransition(cfg, NotifyContext{Res: dayResult(StateCrisis, StateWatch), StateDays: 20})
	assert.Contains(t, msg, "情绪层连续 10 个交易日回落至绿")
	msg = renderTransition(cfg, NotifyContext{Res: dayResult(StateWatch, StateNormal), StateDays: 40})
	assert.Contains(t, msg, "稳定 20 个交易日。回到常态")
}
```

- [ ] **Step 2: 运行确认失败**

Run: `GOTOOLCHAIN=local go test ./internal/crisis/ -run TestRenderTransition -v`
Expected: 编译失败，`undefined: renderTransition`

- [ ] **Step 3: 实现** — 在 `internal/crisis/notify_render.go` 追加：

```go
// notifyFooter 页脚（通知设计 §4.2）：只挂结构化家族；速报是事实陈述不带页脚。
// 措辞集中为常量，禁词与"非交易信号"由单测全家族兜底（通知设计 §7）。
const notifyFooter = "\n—\n风险状态提示（概率语言），非交易信号；指标基于有限历史样本，可能失效；操作决策不在本模块范围。"

const crisisSentence = "情绪层双红：危机进行中。此阶段执行预案而非预测。"

// semanticSentences 语义句查表（通知设计 §4.1），键 = "FROM→TO"（状态机可达的
// 全部转移）。含天数处用 %d 占位，semanticSentence 注入 state_machine 配置值。
var semanticSentences = map[string]string{
	"NORMAL→WATCH":   "领先层或多指标共振异常。观察期：提高警觉，尚无行动含义。",
	"WATCH→BREWING":  "信用与流动性双红共振。历史样本中，此组合出现后 3–12 个月内系统性风险显著抬升（样本量小，存在失效可能）。",
	"NORMAL→CRISIS":  crisisSentence,
	"WATCH→CRISIS":   crisisSentence,
	"BREWING→CRISIS": crisisSentence,
	"CRISIS→WATCH":   "情绪层连续 %d 个交易日回落至绿。危机状态解除，转入观察期。",
	"BREWING→WATCH":  "信用/流动性共振解除并稳定 %d 个交易日。回到观察期。",
	"WATCH→NORMAL":   "全部触发条件解除并稳定 %d 个交易日。回到常态。",
}

// semanticSentence 查表并注入 %d（避免 YAML 调参后文案失真，通知设计 §4.1）。
// 未知转移返回空串（渲染时省略语义句段）。
func semanticSentence(cfg *Config, from, to SystemState) string {
	s, ok := semanticSentences[string(from)+"→"+string(to)]
	if !ok {
		return ""
	}
	sm := cfg.StateMachine
	switch {
	case from == StateCrisis && to == StateWatch:
		return fmt.Sprintf(s, sm.CrisisExitDays)
	case from == StateBrewing && to == StateWatch:
		return fmt.Sprintf(s, sm.BrewingExitDays)
	case from == StateWatch && to == StateNormal:
		return fmt.Sprintf(s, sm.WatchExitDays)
	}
	return s
}

// renderTransition 消息 1/2：状态升级/降级（通知设计 §5.1/§5.2）。
func renderTransition(cfg *Config, nc NotifyContext) string {
	res := nc.Res
	var first, title, tail string
	if stateRank(res.State) > stateRank(res.PrevState) {
		prefix := "[P1] ⚠️"
		if res.State == StateBrewing || res.State == StateCrisis {
			prefix = "[P0] 🚨"
		}
		first = fmt.Sprintf("%s 状态升级 %s → %s · %s", prefix, res.PrevState, res.State, monthDay(res.Date))
		title = "触发共振："
		tail = fmt.Sprintf("%s 已持续 %d 个评估日 → %s · 下一评估：下一交易日",
			res.PrevState, nc.StateDays, res.State)
	} else {
		first = fmt.Sprintf("[P1] ✅ 状态解除 %s → %s · %s", res.PrevState, res.State, monthDay(res.Date))
		title = "仍异常："
		tail = fmt.Sprintf("%s 共持续 %d 个评估日 · 下一评估：下一交易日", res.PrevState, nc.StateDays)
	}
	parts := []string{first}
	if s := semanticSentence(cfg, res.PrevState, res.State); s != "" {
		parts = append(parts, s)
	}
	parts = append(parts, bodyZones(cfg, res, title), tail)
	return strings.Join(parts, "\n\n") + notifyFooter
}
```

- [ ] **Step 4: 运行确认通过**

Run: `GOTOOLCHAIN=local go test ./internal/crisis/ -v`
Expected: PASS

- [ ] **Step 5: 提交（先跑 gitnexus detect_changes 与 code-simplifier）**

```bash
git add internal/crisis/notify_render.go internal/crisis/notify_render_test.go
git commit -m "feat(crisis): render state transition messages with semantic sentences"
```

### Task 6: notify_render.go（三）— 日报（较昨日）与周报（退出进度）

**Files:**
- Modify: `internal/crisis/notify_render.go`（追加）
- Test: `internal/crisis/notify_render_test.go`（追加）

**Interfaces:**
- Consumes: Task 4/5 全部；`NotifyContext.PrevDay`/`ClearStreak`
- Produces: `colorWord`/`diffLine(nc)`（§6.5）、`renderDaily(cfg, nc)`（消息 3）、`renderWeekly(cfg, nc)`（消息 5）

- [ ] **Step 1: 写失败测试** — 在 `internal/crisis/notify_render_test.go` 追加：

```go
// 日报（§5.3）：首行第 N 日、异常指标区、较昨日差异行、盘中提示尾注。
func TestRenderDaily(t *testing.T) {
	cfg := testConfig()
	res := dayResult(StateBrewing, StateBrewing)
	res.Date = "2026-07-18"
	set := func(ind string, r IndicatorResult) { res.Results[ind] = r }
	set(IndHYOAS, IndicatorResult{Indicator: IndHYOAS, Status: StatusRed, Value: 618, Pct5y: 0.98, Tag: TagStress})
	set(IndUSDJPY, IndicatorResult{Indicator: IndUSDJPY, Status: StatusAmber, Value: 158.9, Wow: -0.021, WowOK: true})

	nc := NotifyContext{Res: res, StateDays: 5, PrevDay: map[string]Evaluation{
		IndHYOAS:  {Indicator: IndHYOAS, Status: StatusRed, Value: 612},
		IndUSDJPY: {Indicator: IndUSDJPY, Status: StatusGreen, Value: 160.1},
		IndVIX:    {Indicator: IndVIX, Status: StatusGreen, Value: 1}, // 读数不变且非异常 → 不出现
	}}
	msg := renderDaily(cfg, nc)
	assert.True(t, strings.HasPrefix(msg, "[P1] 📍 BREWING 日报 第 5 日 · 07-18\n\n异常指标：\n🔴 信用 hy_oas 618bp"))
	// 状态迁移优先 + 读数变化仅列异常区指标（§6.5）；顺序按 AllIndicators
	assert.Contains(t, msg, "较昨日：hy_oas +6bp · usdjpy 转黄（原绿）")
	assert.Contains(t, msg, "盘中 JPY 监测运行中（每 30 分钟）· 下一评估：下一交易日")
	assert.True(t, strings.HasSuffix(msg, notifyFooter))

	// 完全无变化 → "较昨日：无变化"（测试要点 4）
	res2 := dayResult(StateCrisis, StateCrisis)
	nc2 := NotifyContext{Res: res2, StateDays: 2, PrevDay: map[string]Evaluation{
		IndVIX: {Indicator: IndVIX, Status: StatusGreen, Value: 1},
	}}
	msg = renderDaily(cfg, nc2)
	assert.True(t, strings.HasPrefix(msg, "[P1] 📍 CRISIS 日报 第 2 日"))
	assert.Contains(t, msg, "较昨日：无变化")
}

// 周报（§5.5）：首行当周、退出进度（§6.6）、下次周报尾注。
func TestRenderWeekly(t *testing.T) {
	cfg := testConfig()
	res := dayResult(StateWatch, StateWatch)
	res.Date = "2026-07-20"
	msg := renderWeekly(cfg, NotifyContext{Res: res, StateDays: 18, ClearStreak: 8})
	assert.True(t, strings.HasPrefix(msg, "[P1] 📅 Cassandra 周报 · 07-20 当周 · WATCH 已持续 18 个评估日"))
	assert.Contains(t, msg, "7 指标全绿：")
	assert.Contains(t, msg, "退出进度：触发条件已连续解除 8 日（回 NORMAL 需连续 20 日）")
	assert.Contains(t, msg, "下次周报：下周一 · 状态变更即时通知")
	assert.True(t, strings.HasSuffix(msg, notifyFooter))
}
```

- [ ] **Step 2: 运行确认失败**

Run: `GOTOOLCHAIN=local go test ./internal/crisis/ -run 'TestRenderDaily|TestRenderWeekly' -v`
Expected: 编译失败，`undefined: renderDaily`

- [ ] **Step 3: 实现** — 在 `internal/crisis/notify_render.go` 追加：

```go
func colorWord(s Status) string {
	switch s {
	case StatusGreen:
		return "绿"
	case StatusAmber:
		return "黄"
	case StatusRed:
		return "红"
	}
	return "白" // STALE / NO_DATA / SUPPRESSED_SEASONAL
}

// diffLine "较昨日"差异行（通知设计 §6.5）：状态迁移优先（usdjpy 转黄（原绿）），
// 读数变化仅列当日异常区指标（hy_oas +6bp）；完全无变化 → 无变化。
func diffLine(nc NotifyContext) string {
	abnormal, _ := splitZones(nc.Res)
	inAbnormal := map[string]bool{}
	for _, r := range abnormal {
		inAbnormal[r.Indicator] = true
	}
	var parts []string
	for _, ind := range AllIndicators {
		prev, ok := nc.PrevDay[ind]
		if !ok {
			continue
		}
		cur := nc.Res.Results[ind]
		if prev.Status != cur.Status {
			parts = append(parts, fmt.Sprintf("%s 转%s（原%s）", ind, colorWord(cur.Status), colorWord(prev.Status)))
			continue
		}
		if d := cur.Value - prev.Value; inAbnormal[ind] && d != 0 {
			parts = append(parts, ind+" "+formatDelta(ind, d))
		}
	}
	if len(parts) == 0 {
		return "较昨日：无变化"
	}
	return "较昨日：" + strings.Join(parts, " · ")
}

// renderDaily 消息 3：BREWING/CRISIS 无变更日报（通知设计 §5.3）。
func renderDaily(cfg *Config, nc NotifyContext) string {
	res := nc.Res
	first := fmt.Sprintf("[P1] 📍 %s 日报 第 %d 日 · %s", res.State, nc.StateDays, monthDay(res.Date))
	tail := diffLine(nc) + "\n盘中 JPY 监测运行中（每 30 分钟）· 下一评估：下一交易日"
	return strings.Join([]string{first, bodyZones(cfg, res, "异常指标："), tail}, "\n\n") + notifyFooter
}

// renderWeekly 消息 5：WATCH 周报（通知设计 §5.5，退出进度见 §6.6）。
func renderWeekly(cfg *Config, nc NotifyContext) string {
	res := nc.Res
	first := fmt.Sprintf("[P1] 📅 Cassandra 周报 · %s 当周 · %s 已持续 %d 个评估日",
		monthDay(res.Date), res.State, nc.StateDays)
	tail := fmt.Sprintf("退出进度：触发条件已连续解除 %d 日（回 NORMAL 需连续 %d 日）\n下次周报：下周一 · 状态变更即时通知",
		nc.ClearStreak, cfg.StateMachine.WatchExitDays)
	return strings.Join([]string{first, bodyZones(cfg, res, "异常指标："), tail}, "\n\n") + notifyFooter
}
```

- [ ] **Step 4: 运行确认通过**

Run: `GOTOOLCHAIN=local go test ./internal/crisis/ -v`
Expected: PASS

- [ ] **Step 5: 提交（先跑 gitnexus detect_changes 与 code-simplifier）**

```bash
git add internal/crisis/notify_render.go internal/crisis/notify_render_test.go
git commit -m "feat(crisis): render daily digest and weekly summary messages"
```

### Task 7: notify_render.go（四）— 月报（趋势区）与 P2 运维速报

**Files:**
- Modify: `internal/crisis/notify_render.go`（追加）
- Test: `internal/crisis/notify_render_test.go`（追加）

**Interfaces:**
- Consumes: Task 3 的 `sparkline`/`trendArrow`/`formatDelta`；Task 4 的类型与格式；`Config.Freshness`（config.go:30）、`SysDetail.AmberCount`（statemachine.go:13）、`daysBetween`（dates.go）
- Produces: `trendLine`/`nextMonthlyDue`/`renderMonthly(cfg, nc)`（消息 4）、`renderOpsAlert(cfg, nc, ind)`（消息 6）

- [ ] **Step 1: 写失败测试** — 在 `internal/crisis/notify_render_test.go` 追加：

```go
// testTrends 为 dayResult 的 7 指标各造一段 21 观测趋势窗口。
func testTrends(end string) map[string]Trend {
	out := map[string]Trend{}
	for _, ind := range AllIndicators {
		win := seriesEnding(end, 21, 10, 12)
		out[ind] = Trend{Window: win, Delta: win[len(win)-1].Value - win[0].Value}
	}
	return out
}

// 月报（§5.4）：单一趋势区（无异常/正常分区）、sparkline+月变化并列、
// AMBER 计数尾注、下次月报。
func TestRenderMonthly(t *testing.T) {
	cfg := testConfig()
	res := dayResult(StateNormal, StateNormal)
	res.Date = "2026-08-03"
	res.Detail = SysDetail{AmberCount: 2}
	r := res.Results[IndHYOAS]
	r.Status, r.Tag, r.Value, r.Pct5y = StatusAmber, TagComplacency, 267, 0.03
	res.Results[IndHYOAS] = r

	nc := NotifyContext{Res: res, StateDays: 63, SummaryDue: true, Trends: testTrends(res.Date)}
	msg := renderMonthly(cfg, nc)
	assert.True(t, strings.HasPrefix(msg, "[P1] 📅 Cassandra 月报 · 2026-08 · NORMAL 已持续 63 个评估日\n\n近 21 个交易日趋势（走势 · 月变化 · 5y分位）：\n"))
	// 趋势行：读数（取 IndicatorResult.Value，dayResult 夹具=1.0）
	// sparkline 箭头+Δ · 分位 [· tag]；窗口 10→12 → Δ=+2 → vix "↗+2.0"
	assert.Contains(t, msg, "🟢 情绪 vix 1.0 ")
	assert.Contains(t, msg, "↗+2.0 · 50%")
	assert.Contains(t, msg, "🟡 信用 hy_oas 267bp ")
	assert.Contains(t, msg, "↗+2bp · 3% · 自满(COMPLACENCY)")
	assert.NotContains(t, msg, "异常指标：") // 月报特例：不分区（设计 §4）
	assert.NotContains(t, msg, "其余指标：")
	assert.Contains(t, msg, "AMBER 计数 2（触发 WATCH 需 ≥3）· 下次月报：9 月首个交易日")
	assert.True(t, strings.HasSuffix(msg, notifyFooter))

	// 空窗口 → 省略该行（测试要点 3）
	delete(nc.Trends, IndMOVE)
	assert.NotContains(t, renderMonthly(cfg, nc), "move")
}

// P2 运维速报（§5.6）：两行、无页脚、滞后与通道名。
func TestRenderOpsAlert(t *testing.T) {
	cfg := testConfig()
	res := dayResult(StateNormal, StateNormal)
	res.Date = "2026-07-14"
	nc := NotifyContext{Res: res, NewStale: []string{IndMOVE},
		StaleLastObs: map[string]string{IndMOVE: "2026-07-09"}}

	msg := renderOpsAlert(cfg, nc, IndMOVE)
	assert.Equal(t, "[P2] 🔧 move 数据源断更 · 07-14\n最后观测 07-09（滞后 5 日 > 阈值 4 日），已标记 STALE 退出共振计数；恢复后自动回归。持续超一周需检查 Yahoo 通道。", msg)
	assert.NotContains(t, msg, "非交易信号") // 速报无页脚（设计 §2）

	// nfci 用周频阈值 + FRED 通道
	nc.StaleLastObs[IndNFCI] = "2026-06-30"
	msg = renderOpsAlert(cfg, nc, IndNFCI)
	assert.Contains(t, msg, "滞后 14 日 > 阈值 12 日")
	assert.Contains(t, msg, "FRED 通道")

	// 最后观测日缺失 → 降级文案
	msg = renderOpsAlert(cfg, nc, IndVIX)
	assert.Contains(t, msg, "无历史观测")
}
```

- [ ] **Step 2: 运行确认失败**

Run: `GOTOOLCHAIN=local go test ./internal/crisis/ -run 'TestRenderMonthly|TestRenderOpsAlert' -v`
Expected: 编译失败，`undefined: renderMonthly`

- [ ] **Step 3: 实现** — 在 `internal/crisis/notify_render.go` 追加（文件 import 增加 `"time"`）：

```go
// trendLine 月报趋势行（通知设计 §5.4/§6.4）：
// 🟡 信用 hy_oas 267bp ▃▂▂▁▁▁▁ ↘-18bp · 3% · 自满(COMPLACENCY)
func trendLine(r IndicatorResult, tr Trend) string {
	head := fmt.Sprintf("%s %s %s %s %s %s%s",
		statusEmoji(r.Status), layerName(r.Indicator), r.Indicator,
		formatReading(r.Indicator, r.Value), sparkline(tr.Window),
		trendArrow(r.Indicator, tr.Delta), formatDelta(r.Indicator, tr.Delta))
	var parts []string
	if showPct5y(r.Indicator) && r.Pct5y >= 0 {
		parts = append(parts, formatPct5y(r.Pct5y))
	}
	if t := tagText(r.Tag); t != "" {
		parts = append(parts, t)
	}
	if note := nonColorNote(r.Status); note != "" {
		parts = append(parts, note)
	}
	if len(parts) == 0 {
		return head
	}
	return head + " · " + strings.Join(parts, " · ")
}

func nextMonthlyDue(date string) string {
	t, err := time.Parse(dateLayout, date)
	if err != nil {
		return "下月首个交易日"
	}
	return fmt.Sprintf("%d 月首个交易日", int(t.AddDate(0, 1, 0).Month()))
}

// renderMonthly 消息 4：NORMAL 月报（通知设计 §5.4）。月报特例：不做异常/
// 正常分区，单一趋势区按 AllIndicators 顺序；空趋势窗口省略该行。
func renderMonthly(cfg *Config, nc NotifyContext) string {
	res := nc.Res
	month := res.Date
	if len(month) >= 7 {
		month = month[:7]
	}
	first := fmt.Sprintf("[P1] 📅 Cassandra 月报 · %s · %s 已持续 %d 个评估日",
		month, res.State, nc.StateDays)
	lines := []string{"近 21 个交易日趋势（走势 · 月变化 · 5y分位）："}
	for _, ind := range AllIndicators {
		tr, ok := nc.Trends[ind]
		if !ok || len(tr.Window) == 0 {
			continue
		}
		lines = append(lines, trendLine(res.Results[ind], tr))
	}
	tail := fmt.Sprintf("AMBER 计数 %d（触发 WATCH 需 ≥%d）· 下次月报：%s",
		res.Detail.AmberCount, cfg.StateMachine.WatchAmberCount, nextMonthlyDue(res.Date))
	return strings.Join([]string{first, strings.Join(lines, "\n"), tail}, "\n\n") + notifyFooter
}

// renderOpsAlert 消息 6：P2 运维速报（通知设计 §5.6）。速报家族：单事实陈述、
// 无页脚。去重（仅新进入 STALE 当日发）由 cmd 组装 NewStale 时完成。
func renderOpsAlert(cfg *Config, nc NotifyContext, ind string) string {
	first := fmt.Sprintf("[P2] 🔧 %s 数据源断更 · %s", ind, monthDay(nc.Res.Date))
	channel := "FRED"
	if ind == IndMOVE || ind == IndUSDJPY {
		channel = "Yahoo"
	}
	lastObs, ok := nc.StaleLastObs[ind]
	if !ok || lastObs == "" {
		return first + fmt.Sprintf("\n无历史观测，已标记 STALE 退出共振计数；恢复后自动回归。持续超一周需检查 %s 通道。", channel)
	}
	maxLag := cfg.Freshness.DailyMaxLagDays
	if ind == IndNFCI {
		maxLag = cfg.Freshness.WeeklyMaxLagDays
	}
	return first + fmt.Sprintf("\n最后观测 %s（滞后 %d 日 > 阈值 %d 日），已标记 STALE 退出共振计数；恢复后自动回归。持续超一周需检查 %s 通道。",
		monthDay(lastObs), daysBetween(lastObs, nc.Res.Date), maxLag, channel)
}
```

- [ ] **Step 4: 运行确认通过**

Run: `GOTOOLCHAIN=local go test ./internal/crisis/ -v`
Expected: PASS

- [ ] **Step 5: 提交（先跑 gitnexus detect_changes 与 code-simplifier）**

```bash
git add internal/crisis/notify_render.go internal/crisis/notify_render_test.go
git commit -m "feat(crisis): render monthly trend summary and ops stale alerts"
```

### Task 8: cmd 层 `buildNotifyContext`（旧 Messages 暂不切换）

**Files:**
- Modify: `cmd/atlas/crisis.go`（追加函数；`executeCrisisEvalDaily` 本任务**不动**）
- Test: `cmd/atlas/crisis_test.go`（追加）

**Interfaces:**
- Consumes: `crisis.NotifyContext`/`Trend`（Task 4）、`crisis.ClearStreakDays`（Task 2）、既有 `stateStreakDays`（crisis.go:507）、`summaryDue`（crisis.go:354）、`Store.RecentIndicatorEvals/LatestObservation/SeriesWindow`
- Produces: `buildNotifyContext(ctx, d, res) (crisis.NotifyContext, error)`（Task 9 接线消费）

**关键约束（通知设计 §8 + 补充决策 6/8）**：本函数必须在 `AppendEvaluations` **之前**调用——`PrevDay`/`StateDays`/`ClearStreak` 取的都是"截至昨日"的库内历史，当日增量在函数内补。

- [ ] **Step 1: 写失败测试** — 在 `cmd/atlas/crisis_test.go` 追加（复用文件既有的 store/deps 构造惯例；若无独立 store 助手则按下方 `newNotifyTestDeps` 自建）：

```go
func newNotifyTestDeps(t *testing.T) crisisEvalDeps {
	t.Helper()
	st, err := crisis.NewStore(filepath.Join(t.TempDir(), "crisis.db"))
	require.NoError(t, err)
	t.Cleanup(func() { st.Close() })
	cfg := &crisis.Config{
		Freshness:    crisis.FreshnessCfg{DailyMaxLagDays: 4, WeeklyMaxLagDays: 12},
		StateMachine: crisis.StateMachineCfg{WatchAmberCount: 3, CrisisExitDays: 10, WatchExitDays: 20, BrewingExitDays: 10, DemoteHysteresisDays: 3},
	}
	return crisisEvalDeps{cfg: cfg, store: st, now: time.Now, out: io.Discard, errOut: io.Discard}
}

func notifyDayResult(date string, prev, cur crisis.SystemState) *crisis.DayResult {
	results := map[string]crisis.IndicatorResult{}
	for _, ind := range crisis.AllIndicators {
		results[ind] = crisis.IndicatorResult{Indicator: ind, Status: crisis.StatusGreen, RawStatus: crisis.StatusGreen, Value: 1}
	}
	return &crisis.DayResult{Date: date, Results: results, PrevState: prev, State: cur}
}

// buildNotifyContext：PrevDay 取前一评估日行、NewStale 去重、StateDays 语义、
// ClearStreak 与 Trends 的按需组装（通知设计 §8，测试要点 5）。
func TestBuildNotifyContext(t *testing.T) {
	d := newNotifyTestDeps(t)
	ctx := context.Background()

	// 昨日（07-17）：全绿 WATCH 系统行 + vix/move 指标行；move 昨日已 STALE
	sysDetail := `{"date":"2026-07-17","any_trigger":false,"brewing_pair":false,"amber_count":0,"prev":"WATCH"}`
	require.NoError(t, d.store.AppendEvaluations(ctx, []crisis.Evaluation{
		{TS: "2026-07-17", EvalAt: "2026-07-18T01:00:00.000000000Z", Indicator: crisis.IndVIX, Status: crisis.StatusGreen, Value: 15},
		{TS: "2026-07-17", EvalAt: "2026-07-18T01:00:00.000000000Z", Indicator: crisis.IndMOVE, Status: crisis.StatusStale, Value: 88},
		{TS: "2026-07-17", EvalAt: "2026-07-18T01:00:00.000000000Z", Indicator: "", SystemState: crisis.StateWatch, Detail: sysDetail},
	}))
	require.NoError(t, d.store.UpsertObservations(ctx, []crisis.Observation{
		{Date: "2026-07-13", Indicator: crisis.IndMOVE, Value: 88, Source: "yahoo", FetchedAt: "2026-07-13T00:00:00.000000000Z"},
	}))

	// 今日（07-20，周一 → WATCH 周报到期）：move 持续 STALE，vix 新进入 STALE
	res := notifyDayResult("2026-07-20", crisis.StateWatch, crisis.StateWatch)
	setStale := func(ind string) {
		r := res.Results[ind]
		r.Status = crisis.StatusStale
		res.Results[ind] = r
	}
	setStale(crisis.IndMOVE)
	setStale(crisis.IndVIX)
	res.Detail = crisis.SysDetail{AnyTrigger: false}

	nc, err := buildNotifyContext(ctx, d, res)
	require.NoError(t, err)
	assert.Equal(t, crisis.StatusGreen, nc.PrevDay[crisis.IndVIX].Status) // 昨日行而非当日
	assert.Equal(t, []string{crisis.IndVIX}, nc.NewStale)                 // move 昨日已 STALE → 不重复（要点 5）
	assert.Equal(t, 2, nc.StateDays)                                      // 昨日 1 行 WATCH + 今日
	assert.True(t, nc.SummaryDue)                                         // 周一 + WATCH
	assert.Equal(t, 2, nc.ClearStreak)                                    // 昨日 any_trigger=false + 今日
	assert.Nil(t, nc.Trends)                                              // 非 NORMAL 月报 → 不组装

	// vix 无观测 → StaleLastObs 缺省；move 持续 STALE 不进 NewStale 故也不查
	_, ok := nc.StaleLastObs[crisis.IndVIX]
	assert.False(t, ok)
}

// 变更日 StateDays = 前状态持续日数（补充决策 6）；NORMAL 月报日组装 Trends。
func TestBuildNotifyContextTransitionAndTrends(t *testing.T) {
	d := newNotifyTestDeps(t)
	ctx := context.Background()

	// 历史：两行 WATCH 系统行
	for _, ts := range []string{"2026-07-16", "2026-07-17"} {
		require.NoError(t, d.store.AppendEvaluations(ctx, []crisis.Evaluation{
			{TS: ts, EvalAt: ts + "T23:00:00.000000000Z", Indicator: "", SystemState: crisis.StateWatch,
				Detail: `{"any_trigger":true,"prev":"WATCH"}`},
		}))
	}
	res := notifyDayResult("2026-07-20", crisis.StateWatch, crisis.StateBrewing) // 升级日
	nc, err := buildNotifyContext(ctx, d, res)
	require.NoError(t, err)
	assert.Equal(t, 2, nc.StateDays) // 前状态 WATCH 的历史持续 = 2

	// NORMAL + 当月首个交易日（2026-08-03 周一）→ Trends 组装、窗口 ≤21 升序
	d2 := newNotifyTestDeps(t)
	var obs []crisis.Observation
	for i := 0; i < 25; i++ {
		obs = append(obs, crisis.Observation{Date: mustAddDays("2026-08-03", -i), Indicator: crisis.IndVIX,
			Value: float64(20 - i), Source: "fred", FetchedAt: "2026-08-03T00:00:00.000000000Z"})
	}
	require.NoError(t, d2.store.UpsertObservations(ctx, obs))
	res2 := notifyDayResult("2026-08-03", crisis.StateNormal, crisis.StateNormal)
	nc2, err := buildNotifyContext(ctx, d2, res2)
	require.NoError(t, err)
	require.NotNil(t, nc2.Trends)
	vix := nc2.Trends[crisis.IndVIX]
	assert.Len(t, vix.Window, 21)
	assert.InDelta(t, vix.Window[len(vix.Window)-1].Value-vix.Window[0].Value, vix.Delta, 1e-9)
	_, hasMove := nc2.Trends[crisis.IndMOVE] // 无观测指标不进 map（月报省略该行）
	assert.False(t, hasMove)
}
```

（若 crisis_test.go 缺少 `path/filepath`/`context`/`io`/`time` 导入则补充。）

- [ ] **Step 2: 运行确认失败**

Run: `GOTOOLCHAIN=local go test ./cmd/atlas/ -run TestBuildNotifyContext -v`
Expected: 编译失败，`undefined: buildNotifyContext`

- [ ] **Step 3: 实现** — 在 `cmd/atlas/crisis.go` 的 `executeCrisisEvalDaily` 之后追加：

```go
// buildNotifyContext 组装通知渲染输入（通知设计 §8）。必须在 AppendEvaluations
// 之前调用：PrevDay/StateDays/ClearStreak 都取"截至昨日"的库内历史，当日增量
// （今日行、今日 any_trigger）在此函数内补足。
func buildNotifyContext(ctx context.Context, d crisisEvalDeps, res *crisis.DayResult) (crisis.NotifyContext, error) {
	nc := crisis.NotifyContext{Res: res, SummaryDue: summaryDue(res.Date, res.State)}

	nc.PrevDay = map[string]crisis.Evaluation{}
	for _, ind := range crisis.AllIndicators {
		evals, err := d.store.RecentIndicatorEvals(ctx, ind, 1)
		if err != nil {
			return nc, err
		}
		if len(evals) > 0 {
			nc.PrevDay[ind] = evals[0]
		}
	}

	// 变更消息展示"前状态已持续 N 日"；无变更消息含当日（补充决策 6）
	if res.Transitioned() {
		days, err := stateStreakDays(ctx, d.store, res.PrevState)
		if err != nil {
			return nc, err
		}
		nc.StateDays = days
	} else {
		days, err := stateStreakDays(ctx, d.store, res.State)
		if err != nil {
			return nc, err
		}
		nc.StateDays = days + 1
	}

	// P2 去重：仅"昨日非 STALE、今日 STALE"的指标发一次（通知设计 §2）
	nc.StaleLastObs = map[string]string{}
	for _, ind := range crisis.AllIndicators {
		if res.Results[ind].Status != crisis.StatusStale {
			continue
		}
		if prev, ok := nc.PrevDay[ind]; ok && prev.Status == crisis.StatusStale {
			continue
		}
		nc.NewStale = append(nc.NewStale, ind)
		if o, err := d.store.LatestObservation(ctx, ind); err != nil {
			return nc, err
		} else if o != nil {
			nc.StaleLastObs[ind] = o.Date
		}
	}

	// 周报退出进度：历史 any_trigger=false 连续日数 + 今日（补充决策 8）
	if res.State == crisis.StateWatch && nc.SummaryDue && !res.Detail.AnyTrigger {
		base, err := crisis.ClearStreakDays(d.store.History(ctx), d.cfg.StateMachine.WatchExitDays)
		if err != nil {
			return nc, err
		}
		nc.ClearStreak = base + 1
	}

	// 月报趋势：仅 SummaryDue ∧ NORMAL 时组装（通知设计 §8）
	if nc.SummaryDue && res.State == crisis.StateNormal {
		nc.Trends = map[string]crisis.Trend{}
		for _, ind := range crisis.AllIndicators {
			win, err := d.store.SeriesWindow(ctx, ind, res.Date, 21)
			if err != nil {
				return nc, err
			}
			if len(win) == 0 {
				continue
			}
			nc.Trends[ind] = crisis.Trend{Window: win, Delta: win[len(win)-1].Value - win[0].Value}
		}
	}
	return nc, nil
}
```

- [ ] **Step 4: 运行确认通过（全仓编译 + cmd 测试）**

Run: `GOTOOLCHAIN=local go build ./... && GOTOOLCHAIN=local go test ./cmd/atlas/ -v`
Expected: PASS（旧 `Messages` 仍在用，通知输出不变）

- [ ] **Step 5: 提交（先跑 gitnexus detect_changes 与 code-simplifier）**

```bash
git add cmd/atlas/crisis.go cmd/atlas/crisis_test.go
git commit -m "feat(crisis): assemble NotifyContext ahead of notification switch"
```

### Task 9: 切换 — notify.go 重写、cmd 接线与全家族测试

**Files:**
- Modify: `internal/crisis/notify.go`（整体重写）、`cmd/atlas/crisis.go:312-335`（executeCrisisEvalDaily 通知段）、`cmd/atlas/crisis.go:437-438`（executeCrisisIntraday 文案）
- Delete（随重写移除）: 旧 `footer` 常量、旧 `Messages(res, days, due, stale)`、`indicatorLines`（notify.go:17-71）、cmd 的 `staleIndicators`（crisis.go:379-387，本次改动后无调用方）
- Test: `internal/crisis/notify_test.go`（整体重写）、`cmd/atlas/crisis_test.go`（断言核对）

**Interfaces:**
- Consumes: Task 5–7 的五个渲染器、Task 8 的 `buildNotifyContext`
- Produces: `Messages(cfg *Config, nc NotifyContext) []string`（消息 1–6 装配）、`FormatIntradayAlert(price, base, wow, state, at)`（消息 7）——通知设计 §8 的最终公开接口

- [ ] **Step 1: 影响面分析（项目 MUST 规则）**

运行 `gitnexus_impact({target: "Messages", direction: "upstream"})`、同样对 `executeCrisisEvalDaily`、`executeCrisisIntraday`。预期 `Messages` 的调用方仅 `executeCrisisEvalDaily`（本任务同步改）；两个 execute 函数的调用方仅 `runCrisisEval`。HIGH/CRITICAL 先停下说明。

- [ ] **Step 2: 重写失败测试** — `internal/crisis/notify_test.go` 整体替换为（保留既有 `dayResult` 助手，头部映射注释同步更新）：

```go
package crisis

// Context Checkpoint: done_criteria → test mapping (notify v2，通知设计 v1.0)
// §2 消息类型矩阵/装配唯一性 → TestMessagesDispatch
// §7 禁词 + 页脚（结构化含"非交易信号"、速报不含页脚）→ TestMessagesForbiddenWordsAllFamilies
// §5.7 盘中速报 → TestFormatIntradayAlert

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func dayResult(prev, cur SystemState) *DayResult {
	results := map[string]IndicatorResult{}
	for _, ind := range AllIndicators {
		results[ind] = IndicatorResult{Indicator: ind, Status: StatusGreen, RawStatus: StatusGreen, Value: 1, Pct5y: 0.5}
	}
	return &DayResult{Date: "2026-07-10", Results: results, PrevState: prev, State: cur}
}

// 装配矩阵（通知设计 §2）：结构化家族至多一条 + NewStale 各一条 P2。
func TestMessagesDispatch(t *testing.T) {
	cfg := testConfig()

	// 状态变更优先（即使同时是 BREWING 日）
	msgs := Messages(cfg, NotifyContext{Res: dayResult(StateWatch, StateBrewing), StateDays: 12})
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0], "状态升级 WATCH → BREWING")

	// 降级也通知，仅 P1（设计 §2）
	msgs = Messages(cfg, NotifyContext{Res: dayResult(StateBrewing, StateWatch), StateDays: 34})
	require.Len(t, msgs, 1)
	assert.True(t, strings.HasPrefix(msgs[0], "[P1] ✅ 状态解除"))

	// BREWING/CRISIS 无变更 → 日报
	msgs = Messages(cfg, NotifyContext{Res: dayResult(StateBrewing, StateBrewing), StateDays: 5})
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0], "BREWING 日报 第 5 日")

	// NORMAL 非到期 → 零消息（boundary）
	assert.Empty(t, Messages(cfg, NotifyContext{Res: dayResult(StateNormal, StateNormal), StateDays: 30}))

	// NORMAL + 到期 → 月报；WATCH + 到期 → 周报
	msgs = Messages(cfg, NotifyContext{Res: dayResult(StateNormal, StateNormal), StateDays: 63,
		SummaryDue: true, Trends: testTrends("2026-07-10")})
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0], "Cassandra 月报")
	msgs = Messages(cfg, NotifyContext{Res: dayResult(StateWatch, StateWatch), StateDays: 18, SummaryDue: true, ClearStreak: 8})
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0], "Cassandra 周报")

	// NewStale 追加 P2；与结构化消息可并发
	msgs = Messages(cfg, NotifyContext{Res: dayResult(StateBrewing, StateBrewing), StateDays: 5,
		NewStale: []string{IndMOVE}, StaleLastObs: map[string]string{IndMOVE: "2026-07-05"}})
	require.Len(t, msgs, 2)
	assert.True(t, strings.HasPrefix(msgs[1], "[P2] 🔧 move 数据源断更"))
}

// 盘中速报（§5.7）：wow 为负渲染急跌百分比、本地时分、无页脚。
func TestFormatIntradayAlert(t *testing.T) {
	at := time.Date(2026, 7, 18, 14, 30, 0, 0, time.Local)
	msg := FormatIntradayAlert(152.1, 157.5, -0.034, StateBrewing, at)
	assert.True(t, strings.HasPrefix(msg, "[P0] 🚨 USD/JPY 盘中急跌 -3.4% · 07-18 14:30"))
	assert.Contains(t, msg, "现价 152.1（5 观测日前 157.5）")
	assert.Contains(t, msg, "系统状态 BREWING")
	assert.Contains(t, msg, "今日此告警不再重复")
	assert.NotContains(t, msg, "非交易信号") // 速报家族无页脚
}

// Global Constraints（通知设计 §7，测试要点 1）：7 类消息全覆盖禁词与页脚归属。
func TestMessagesForbiddenWordsAllFamilies(t *testing.T) {
	cfg := testConfig()
	staleCtx := func(res *DayResult) NotifyContext {
		return NotifyContext{Res: res, StateDays: 5, NewStale: []string{IndMOVE},
			StaleLastObs: map[string]string{IndMOVE: "2026-07-05"}}
	}
	var all []string
	all = append(all, Messages(cfg, NotifyContext{Res: dayResult(StateWatch, StateBrewing), StateDays: 12})...) // 1 升级
	all = append(all, Messages(cfg, NotifyContext{Res: dayResult(StateBrewing, StateWatch), StateDays: 34})...) // 2 降级
	all = append(all, Messages(cfg, staleCtx(dayResult(StateCrisis, StateCrisis)))...)                          // 3 日报 + 6 P2
	all = append(all, Messages(cfg, NotifyContext{Res: dayResult(StateNormal, StateNormal), StateDays: 63,
		SummaryDue: true, Trends: testTrends("2026-07-10")})...) // 4 月报
	all = append(all, Messages(cfg, NotifyContext{Res: dayResult(StateWatch, StateWatch), StateDays: 18,
		SummaryDue: true, ClearStreak: 8})...) // 5 周报
	all = append(all, FormatIntradayAlert(152.1, 157.5, -0.034, StateBrewing,
		time.Date(2026, 7, 18, 14, 30, 0, 0, time.UTC))) // 7 盘中
	require.Len(t, all, 7)

	for _, m := range all {
		for _, banned := range []string{"必然", "一定", "即将"} {
			assert.NotContains(t, m, banned)
		}
		structured := strings.HasPrefix(m, "[P0] 🚨 状态升级") || strings.HasPrefix(m, "[P1]")
		if structured {
			assert.Contains(t, m, "非交易信号") // 页脚只挂结构化家族
		} else {
			assert.NotContains(t, m, "非交易信号")
		}
		assert.LessOrEqual(t, len(m), 4096) // telegram 上限（设计 §7）
	}
}
```

- [ ] **Step 3: 运行确认失败**

Run: `GOTOOLCHAIN=local go test ./internal/crisis/ -run 'TestMessages|TestFormatIntraday' -v`
Expected: 编译失败（新 `Messages` 签名不存在 / 旧测试已删）

- [ ] **Step 4: 重写 notify.go** — 整文件替换为：

```go
package crisis

import (
	"fmt"
	"time"
)

// Sender is the outbound channel; telegram.Telegram's SendText satisfies it.
// Single channel, no priority routing — urgency rides in the [P0]/[P1]/[P2]
// text prefix. All messages are plain text (no parse_mode): emoji and
// sparkline glyphs are ordinary characters (通知设计 §7).
type Sender interface {
	SendText(text string) error
}

// Messages renders the day's outbound notifications per 通知设计 §2 的消息类型
// 矩阵：结构化家族（状态变更 / BREWING·CRISIS 日报 / NORMAL 月报 / WATCH 周报）
// 至多一条，加每个新进入 STALE 的指标一条 [P2] 速报。SummaryDue、NewStale 等
// 输入由 cmd 层在落库前组装（buildNotifyContext）。
func Messages(cfg *Config, nc NotifyContext) []string {
	var msgs []string
	res := nc.Res
	switch {
	case res.Transitioned():
		msgs = append(msgs, renderTransition(cfg, nc))
	case res.State == StateBrewing || res.State == StateCrisis:
		msgs = append(msgs, renderDaily(cfg, nc))
	case nc.SummaryDue && res.State == StateNormal:
		msgs = append(msgs, renderMonthly(cfg, nc))
	case nc.SummaryDue && res.State == StateWatch:
		msgs = append(msgs, renderWeekly(cfg, nc))
	}
	for _, ind := range nc.NewStale {
		msgs = append(msgs, renderOpsAlert(cfg, nc, ind))
	}
	return msgs
}

// FormatIntradayAlert 消息 7：盘中 JPY 速报（通知设计 §5.7）。at 为本地时区
// 时刻；速报家族无页脚，每日一次去重由 executeCrisisIntraday 的评估行保证。
func FormatIntradayAlert(price, base, wow float64, state SystemState, at time.Time) string {
	return fmt.Sprintf(
		"[P0] 🚨 USD/JPY 盘中急跌 %.1f%% · %s\n现价 %.1f（5 观测日前 %.1f）· 系统状态 %s · 疑似 carry trade 快速平仓。今日此告警不再重复。",
		wow*100, at.Format("01-02 15:04"), price, base, state)
}
```

- [ ] **Step 5: cmd 接线** — `cmd/atlas/crisis.go` 两处：

`executeCrisisEvalDaily` 中 `EvalDay` 之后的段落改为（原 `days, err := stateStreakDays(...)` 调用删除）：

```go
	res, err := crisis.EvalDay(d.cfg, target, d.store.Reader(ctx), d.store.History(ctx), d.now())
	if err != nil {
		return err
	}
	// NotifyContext 必须在 AppendEvaluations 之前组装：PrevDay/StateDays/
	// ClearStreak 取的是"截至昨日"的历史（通知设计 §8）
	nc, err := buildNotifyContext(ctx, d, res)
	if err != nil {
		return err
	}
	if err := d.store.AppendEvaluations(ctx, res.Evaluations); err != nil {
		return err
	}
	printDayResult(d.out, res)

	for _, msg := range crisis.Messages(d.cfg, nc) {
		if d.sender == nil {
			fmt.Fprintln(d.out, msg) // 未配置 telegram：打印便于本地试运行
			continue
		}
		if err := d.sender.SendText(msg); err != nil {
			// 通知失败不失败退出：评估已落库，状态可由 status 自愈获取（文件真相源）
			fmt.Fprintf(d.errOut, "warning: notify failed: %v\n", err)
		}
	}
	return nil
```

`executeCrisisIntraday` 中 `msg := fmt.Sprintf("[P0] 盘中告警：...")` 一行改为：

```go
	msg := crisis.FormatIntradayAlert(q.Price, win[0].Value, wow, sys.SystemState, d.now())
```

同时删除已无调用方的 `staleIndicators`（crisis.go:379-387）。

- [ ] **Step 6: 核对 cmd 断言**

Run: `GOTOOLCHAIN=local go build ./... && GOTOOLCHAIN=local go test ./cmd/atlas/ -run TestCrisis -v`

既有断言预期兼容（crisis_test.go:720-721 的 `[P1]`/`NORMAL → WATCH` 在新首行 `[P1] ⚠️ 状态升级 NORMAL → WATCH · MM-DD` 中仍命中；crisis_test.go:907-908 的 `[P0]`/`carry trade` 在新盘中文案中仍命中）。若个别断言失败，按新模板措辞更新断言文本（只改字符串，不放宽断言强度）。

- [ ] **Step 7: 全量测试**

Run: `GOTOOLCHAIN=local go test ./internal/crisis/ ./cmd/atlas/ -v`
Expected: PASS

- [ ] **Step 8: 提交（先跑 gitnexus detect_changes 与 code-simplifier）**

```bash
git add internal/crisis/ cmd/atlas/
git commit -m "feat(crisis): switch notifications to structured template families"
```

---

## 自查记录（设计 v1.0 ↔ 任务覆盖）

| 设计条目 | 覆盖任务 |
|---|---|
| §2 消息类型矩阵（7 类、前缀、页脚归属） | Task 9（Messages 装配 + 全家族测试） |
| §3 首行规范 | Task 5/6/7 各渲染器首行 |
| §4 五段骨架、语义句查表、页脚常量 | Task 5（notifyFooter/semanticSentences/renderTransition） |
| §5.1–5.7 七类示例 | Task 5（5.1/5.2）、Task 6（5.3/5.5）、Task 7（5.4/5.6）、Task 9（5.7） |
| §6.1 状态 emoji / 非色彩说明 | Task 3（statusEmoji/nonColorNote） |
| §6.2 层名与排序 | Task 3（layerName/icebergRank）+ Task 4（splitZones） |
| §6.3 数值格式 / tag / 持续 / 周跌 | Task 3（formatReading 等）+ Task 4（indicatorLine）+ Task 1（PersistDays/Wow） |
| §6.4 sparkline 与箭头 | Task 3（sparkline/trendArrow）+ Task 7（trendLine） |
| §6.5 较昨日差异行 | Task 6（diffLine） |
| §6.6 退出进度 | Task 2（ClearStreakDays）+ Task 6（renderWeekly）+ Task 8（含当日补足） |
| §7 禁词 / 长度 / 纯文本 / 发送失败 | Task 9 测试（禁词 + 4096）；发送循环与 SendText 路径不动 |
| §8 NotifyContext / 新字段 / cmd 组装职责 | Task 1/2/4/8/9（PrevDay 前置、Trends 按需、intraday 切换） |
| §9 测试要点 1–6 | 1→Task 9；2→Task 4；3→Task 3/7；4→Task 6；5→Task 8；6→Task 5 |

**类型一致性**：`Messages(cfg *Config, nc NotifyContext)`、`FormatIntradayAlert(price, base, wow float64, state SystemState, at time.Time)`、`ClearStreakDays(hist EvalHistory, max int)` 与设计 §8 签名一致（NotifyContext 多出 `StaleLastObs`，见补充决策 1）。渲染器统一 `render*(cfg *Config, nc NotifyContext) string`，`renderOpsAlert` 多一个 `ind string` 参数。

## 执行说明

- 推荐执行方式：superpowers:subagent-driven-development（每任务派发独立 subagent，任务间 review）；或 superpowers:executing-plans 在本 session 批量执行。
- 执行前先经 superpowers:using-git-worktrees 在隔离工作区建分支 `feature/crisis-notify-templates`。
- Task 1–7 全部收在 `internal/crisis` 包内、旧 `Messages` 不动，任意提交点全仓可编译可测试；Task 8 只增不改；Task 9 是唯一的签名切换点（包 + cmd + 测试一次提交）。
- 完成后按通知设计 §1 的"下游"约定，可在 `docs/plans/2026-07-13-macro-crisis-monitor-impl.md` 头部加一行指针注明 Task 12/14/15 的通知实现已由本方案取代（可选，不阻塞交付）。
