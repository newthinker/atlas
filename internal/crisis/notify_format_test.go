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
	assert.Equal(t, "", tagText("")) // 无 tag → 空片段（indicatorLine 常传空 Tag）

	assert.Equal(t, "unknown", layerName("unknown")) // 未知指标兜底原样返回
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

	// 恰好 |Δ|==eps 归属方向（设计"< eps 才横盘"→ 恰好相等即箭头，非 →）
	// 锁 >= / <= 边界，防 >=→> 变异静默通过
	assert.Equal(t, "↗", trendArrow(IndVIX, 0.1))
	assert.Equal(t, "↘", trendArrow(IndVIX, -0.1))
	assert.Equal(t, "↗", trendArrow(IndT10Y2Y, 1))
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

	// 单调降序：首桶最高、末桶最低（exercise min-max 的 v<lo 分支）
	var down []Observation
	for i := 0; i < 21; i++ {
		down = append(down, Observation{Date: addDays("2026-07-10", i-20), Value: float64(20 - i)})
	}
	d := []rune(sparkline(down))
	assert.Len(t, d, 7)
	assert.Equal(t, '█', d[0])
	assert.Equal(t, '▁', d[6])
}
