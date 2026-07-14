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
		if r.Status == StatusNoData { // 无读数：直接空格接说明（boundary[0]）
			return head + " " + note
		}
		return head + " " + formatReading(r.Indicator, r.Value) + " · " + note
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
	sort.Slice(abnormal, func(i, j int) bool {
		a, b := abnormal[i], abnormal[j]
		if severity(a.Status) != severity(b.Status) {
			return severity(a.Status) > severity(b.Status) // 一级：严重度降序
		}
		if icebergRank(a.Indicator) != icebergRank(b.Indicator) {
			return icebergRank(a.Indicator) < icebergRank(b.Indicator) // 二级：冰山层序
		}
		return indicatorIndex(a.Indicator) < indicatorIndex(b.Indicator) // 三级：AllIndicators 序（显式，不靠排序稳定性）
	})
	return abnormal, append(rest, noncolor...)
}

// indicatorIndex is the position of ind in AllIndicators — the third-level
// abnormal-zone tiebreak (通知设计 §6.2). Made explicit so the ordering is a
// total comparator rather than an artifact of sort stability (which n≤7 slices
// can't distinguish stable-vs-unstable and so can't lock via mutation).
func indicatorIndex(ind string) int {
	for i, x := range AllIndicators {
		if x == ind {
			return i
		}
	}
	return len(AllIndicators)
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
		if allGreen(rest) { // 无异常且 7 指标全 GREEN 才叫全绿；含 ⚪ 仍用"其余指标："（补充决策 5）
			restTitle = "7 指标全绿："
		}
		return restTitle + "\n" + lines(rest)
	}
	return abnormalTitle + "\n" + lines(abnormal) + "\n\n" + restTitle + "\n" + lines(rest)
}

// allGreen reports whether every result is GREEN — the "7 指标全绿" gate. rest
// from splitZones merges greens and ⚪ (STALE/NO_DATA/季末抑制); a single ⚪
// disqualifies the all-green title (补充决策 5).
func allGreen(rs []IndicatorResult) bool {
	for _, r := range rs {
		if r.Status != StatusGreen {
			return false
		}
	}
	return true
}
