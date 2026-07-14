package crisis

import (
	"fmt"
	"strings"
)

// Sender is the outbound channel; telegram.Telegram's SendText satisfies it.
// Single channel, no priority routing — urgency rides in the [P0]/[P1]/[P2]
// text prefix (design §4.4).
type Sender interface {
	SendText(text string) error
}

// footer is the fixed boundary disclaimer (design §5). Wording stays
// probabilistic everywhere — deterministic claims are banned by test.
const footer = "\n—\n本通知为风险状态提示（概率语言），非交易信号；指标组合基于有限历史危机样本，存在失效可能；资产操作决策（降杠杆、网格暂停等）不在本模块范围。"

// Messages renders the day's outbound notifications per the design §3.3
// state/notification table: transition alerts ([P0] on entering
// BREWING/CRISIS, else [P1]), daily digests while in BREWING/CRISIS, periodic
// summaries (monthly in NORMAL, weekly in WATCH — summaryDue computed by the
// caller), plus [P2] ops alerts for stale indicators.
func Messages(res *DayResult, stateDays int, summaryDue bool, staleInds []string) []string {
	var msgs []string

	switch {
	case res.Transitioned():
		prefix := "[P1]"
		if res.State == StateBrewing || res.State == StateCrisis {
			prefix = "[P0]"
		}
		msgs = append(msgs, fmt.Sprintf("%s 危机监控状态变更：%s → %s（%s）\n%s%s",
			prefix, res.PrevState, res.State, res.Date, indicatorLines(res), footer))
	case res.State == StateBrewing || res.State == StateCrisis:
		msgs = append(msgs, fmt.Sprintf("[P1] 危机监控日报（%s 第 %d 个评估日，%s）\n%s%s",
			res.State, stateDays, res.Date, indicatorLines(res), footer))
	case summaryDue:
		kind := "月度摘要"
		if res.State == StateWatch {
			kind = "周度摘要"
		}
		msgs = append(msgs, fmt.Sprintf("[P1] 危机监控%s（%s，%s 已持续 %d 个评估日）\n%s%s",
			kind, res.Date, res.State, stateDays, indicatorLines(res), footer))
	}

	for _, ind := range staleInds {
		msgs = append(msgs, fmt.Sprintf(
			"[P2] 运维告警：%s 数据超过新鲜度窗口，标记 STALE，已退出共振计数（%s）", ind, res.Date))
	}
	return msgs
}

// indicatorLines renders one line per indicator: status, reading, 5y
// percentile and tag（设计 §4.4 模板要素），加下一评估提示。
func indicatorLines(res *DayResult) string {
	var b strings.Builder
	for _, ind := range AllIndicators {
		r := res.Results[ind]
		fmt.Fprintf(&b, "%-10s %-20s %10.2f", ind, r.Status, r.Value)
		if r.Pct5y >= 0 {
			fmt.Fprintf(&b, "  5y分位 %2.0f%%", r.Pct5y*100)
		}
		if r.Tag != "" {
			fmt.Fprintf(&b, "  [%s]", r.Tag)
		}
		b.WriteString("\n")
	}
	b.WriteString("下一评估：下一交易日（launchd 多时点唤起）")
	return b.String()
}
