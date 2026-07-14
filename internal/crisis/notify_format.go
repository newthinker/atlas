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
