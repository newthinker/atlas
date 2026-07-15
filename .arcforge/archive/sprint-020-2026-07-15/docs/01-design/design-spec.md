# 设计规格 — crisis 通知层重写

> 完整设计见 `docs/plans/2026-07-14-crisis-notification-templates-impl.md`（含逐任务代码与测试）。
> 本文件仅摘录跨任务契约，Dev/Test 以实施方案为准。

## 文件结构

```
internal/crisis/
├── types.go              # [改] IndicatorResult + PersistDays/Wow/WowOK（T1）
├── rules.go              # [改] evalVIX/evalSOFREFFR/evalUSDJPY 填充新字段（T1）
├── suppress.go           # [改] indDetail JSON 携带新字段（T1）
├── eval.go               # [改] buildEvaluations 序列化新字段（T1）
├── statemachine.go       # [改] + ClearStreakDays（T2）
├── notify_format.go      # [新] 格式化原语 + sparkline（T3）
├── notify_render.go      # [新] NotifyContext/Trend + 七类渲染器（T4–7）
├── notify.go             # [重写] Sender + Messages(cfg,nc) + FormatIntradayAlert（T9）
cmd/atlas/
├── crisis.go             # [改] buildNotifyContext + 调用切换 + intraday 文案（T8–9）
```

## 公共签名（后续任务以此为准）

```go
// T1 types.go
type IndicatorResult struct { /* 既有 7 字段 */ PersistDays int; Wow float64; WowOK bool }

// T2 statemachine.go
func ClearStreakDays(hist EvalHistory, max int) (int, error)

// T4 notify_render.go（导出）
type Trend struct { Window []Observation; Delta float64 }
type NotifyContext struct {
    Res *DayResult; StateDays int; SummaryDue bool
    NewStale []string; StaleLastObs map[string]string
    PrevDay map[string]Evaluation; ClearStreak int; Trends map[string]Trend
}

// T4–7 渲染器（包内私有）
func indicatorLine(cfg *Config, r IndicatorResult) string
func splitZones(res *DayResult) (abnormal, rest []IndicatorResult)
func bodyZones(cfg *Config, res *DayResult, abnormalTitle string) string
func renderTransition/renderDaily/renderMonthly/renderWeekly(cfg *Config, nc NotifyContext) string
func renderOpsAlert(cfg *Config, nc NotifyContext, ind string) string

// T9 notify.go（旧 Messages(res,days,due,stale) 同任务删除）
func Messages(cfg *Config, nc NotifyContext) []string
func FormatIntradayAlert(price, base, wow float64, state SystemState, at time.Time) string

// T8 cmd/atlas/crisis.go
func buildNotifyContext(ctx context.Context, d crisisEvalDeps, res *crisis.DayResult) (crisis.NotifyContext, error)
```

## 关键时序约束

`buildNotifyContext` 必须在 `AppendEvaluations` **之前**调用——PrevDay/StateDays/ClearStreak
取"截至昨日"的库内历史，当日增量在函数内补足（补充决策 6/8）。

## 兼容性策略

T1–7 全部收在 `internal/crisis` 包内、旧 `Messages` 不动；T8 只增不改；
T9 是唯一签名切换点（包 + cmd + 测试一次提交），保证任一提交点全仓可编译。
