package crisis

import (
	"fmt"
	"strings"
)

// replayFooter 回放专用尾注（设计 §4：不复用 notifyFooter，含"历史回放"限定）。
const replayFooter = "\n—\n历史回放，非实时告警；阈值为当前配置，非事后调参。"

// worstIsMin "最差"方向与各指标红灯方向一致：t10y2y（倒挂向下）、usdjpy
// （急跌向下）取期间最小值，其余取最大值（设计 §4）。
func worstIsMin(ind string) bool { return ind == IndT10Y2Y || ind == IndUSDJPY }

// hasFreshReading 极值只统计有新鲜读数的日：色彩态与季末抑制有当日观测，
// STALE/NO_DATA 无（STALE 的 Value 是旧读数，其原日已计入）。
func hasFreshReading(s Status) bool { return isColor(s) || s == StatusSuppressed }

// RenderReplaySummary 渲染回放窗口标准总结（单条 ≤4096，telegram 直发）。
// days 为空返回空串（调用方保证非空）。
func RenderReplaySummary(cfg *Config, days []ReplayDay) string {
	if len(days) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "【回放总结 %s ~ %s】\n", days[0].Date, days[len(days)-1].Date)

	start := days[0].Res.State
	if days[0].Res.Transitioned() {
		start = days[0].Res.PrevState
	}
	var trans []ReplayDay
	for _, d := range days {
		if d.Res.Transitioned() {
			trans = append(trans, d)
		}
	}
	fmt.Fprintf(&b, "状态：%s 起步 · 期间转移 %d 次\n", start, len(trans))
	if len(trans) == 0 {
		b.WriteString("转移：无\n")
	}
	for _, d := range trans {
		fmt.Fprintf(&b, "%s %s → %s\n", d.Date, d.Res.PrevState, d.Res.State)
	}

	stay := map[SystemState]int{}
	for _, d := range days {
		stay[d.Res.State]++
	}
	var stays []string
	for _, s := range []SystemState{StateCrisis, StateBrewing, StateWatch, StateNormal} {
		if stay[s] > 0 {
			stays = append(stays, fmt.Sprintf("%s %d 日", s, stay[s]))
		}
	}
	b.WriteString("各态停留：" + strings.Join(stays, " · ") + "\n")

	var extremes []string
	for _, ind := range AllIndicators {
		var v float64
		var date string
		for _, d := range days {
			r := d.Res.Results[ind]
			if !hasFreshReading(r.Status) {
				continue
			}
			worse := r.Value > v
			if worstIsMin(ind) {
				worse = r.Value < v
			}
			if date == "" || worse {
				v, date = r.Value, d.Date
			}
		}
		if date != "" {
			extremes = append(extremes, fmt.Sprintf("%s %s（%s）", ind, formatReading(ind, v), date))
		}
	}
	if len(extremes) > 0 {
		b.WriteString("指标极值（期间最差读数）：\n" + strings.Join(extremes, " · ") + "\n")
	}

	peak, peakDate := 0, ""
	for _, d := range days {
		if d.Res.Detail.AmberCount > peak {
			peak, peakDate = d.Res.Detail.AmberCount, d.Date
		}
	}
	if peakDate == "" {
		fmt.Fprintf(&b, "AMBER 峰值：0/%d\n", len(AllIndicators))
	} else {
		fmt.Fprintf(&b, "AMBER 峰值：%d/%d（%s）\n", peak, len(AllIndicators), peakDate)
	}

	var stales []string
	for _, ind := range AllIndicators {
		n := 0
		for _, d := range days {
			if d.Res.Results[ind].Status == StatusStale {
				n++
			}
		}
		if n > 0 {
			stales = append(stales, fmt.Sprintf("%s 缺数 %d 交易日", ind, n))
		}
	}
	if len(stales) > 0 {
		b.WriteString("STALE 统计：" + strings.Join(stales, " · ") + "\n")
	}
	return strings.TrimRight(b.String(), "\n") + replayFooter
}
