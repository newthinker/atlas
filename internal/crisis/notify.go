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

// SummaryKind 是摘要类消息的到期类型，cmd 层按评估日与状态判定（撞日归月报，
// NORMAL 周报设计 2026-07-23）。零值 SummaryNone = 不到期。
type SummaryKind int

const (
	SummaryNone SummaryKind = iota
	SummaryWeekly
	SummaryMonthly
)

// Messages renders the day's outbound notifications per 通知设计 §2 的消息类型
// 矩阵：结构化家族（状态变更 / BREWING·CRISIS 日报 / NORMAL 月报·周报 / WATCH 周报）
// 至多一条，加每个新进入 STALE 的指标一条 [P2] 速报。Summary、NewStale 等
// 输入由 cmd 层在落库前组装（buildNotifyContext）。
func Messages(cfg *Config, nc NotifyContext) []string {
	var msgs []string
	res := nc.Res
	switch {
	case res.Transitioned():
		msgs = append(msgs, renderTransition(cfg, nc))
	case res.State == StateBrewing || res.State == StateCrisis:
		msgs = append(msgs, renderDaily(cfg, nc))
	case nc.Summary == SummaryMonthly && res.State == StateNormal:
		msgs = append(msgs, renderMonthly(cfg, nc))
	case nc.Summary == SummaryWeekly && (res.State == StateNormal || res.State == StateWatch):
		msgs = append(msgs, renderWeekly(cfg, nc))
	}
	for _, ind := range nc.NewStale {
		msgs = append(msgs, renderOpsAlert(cfg, nc, ind))
	}
	return msgs
}

// FormatIntradayAlert 消息 7：盘中 JPY 速报（通知设计 §5.7，v1.1 R5：去因果
// 归因，报事实 + 内联限定语；该限定语非页脚常量，速报家族无页脚规则不变）。
// at 为本地时区时刻；每日一次去重由 executeCrisisIntraday 的评估行保证。
func FormatIntradayAlert(price, base, wow float64, state SystemState, at time.Time) string {
	return fmt.Sprintf(
		"[P0] 🚨 USD/JPY 盘中急跌 %.1f%% · %s\n现价 %.1f（5 观测日前 %.1f）· 系统状态 %s · 成因未核实，非交易信号。今日此告警不再重复。",
		wow*100, at.Format("01-02 15:04"), price, base, state)
}
