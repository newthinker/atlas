package crisis

// Context Checkpoint: done_criteria → test mapping (TASK-003 RenderReplaySummary)
// functional[0]  期初态/转移列表/各态停留(严重度降序)/AMBER 峰值 → TestRenderReplaySummaryTransitions
// functional[1]  极值方向 vix 最大 / t10y2y 最小 / usdjpy 最小(reviewer 补强) / STALE 日不计入极值 / STALE 统计仅非零 → TestRenderReplaySummaryExtremesAndStale
// boundary[0]    无转移→"转移：无" / 全零 STALE 省略 / AMBER 0/7 → TestRenderReplaySummaryNoTransition
// boundary[1]    窗口首日即转移日→期初态取 PrevState(reviewer 补强) → TestRenderReplaySummaryFirstDayTransition
// error_handling days 空→空串不 panic → TestRenderReplaySummaryEmpty
// non_functional 千日+10 转移 ≤4096 rune / 无禁词 / "指标极值" 恰 1 次 → TestRenderReplaySummaryUnder4096
//                回放专用 replayFooter / 不含 notifyFooter 决策句 → TestRenderReplaySummaryTransitions

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// functional: 转移列表、期初态、各态停留、AMBER 峰值。
func TestRenderReplaySummaryTransitions(t *testing.T) {
	days := []ReplayDay{
		mkReplayDay("2026-07-06", StateNormal, StateNormal, 3, 0, nil, nil),
		mkReplayDay("2026-07-07", StateNormal, StateWatch, 1, 3, nil, nil), // 转移
		mkReplayDay("2026-07-08", StateWatch, StateWatch, 2, 2, nil, nil),
	}
	out := RenderReplaySummary(testConfig(), days)
	assert.Contains(t, out, "【回放总结 2026-07-06 ~ 2026-07-08】")
	assert.Contains(t, out, "状态：NORMAL 起步 · 期间转移 1 次")
	assert.Contains(t, out, "2026-07-07 NORMAL → WATCH")
	assert.NotContains(t, out, "转移：无")
	assert.Contains(t, out, "WATCH 2 日 · NORMAL 1 日") // 严重度降序、仅出现过的态
	assert.Contains(t, out, "AMBER 峰值：3/7（2026-07-07）")
	assert.Contains(t, out, "历史回放，非实时告警；阈值为当前配置，非事后调参。")
	assert.NotContains(t, out, "操作决策不在本模块范围") // 不复用 notifyFooter
}

// boundary: 无转移 → 「转移：无」；全零 STALE → 省略整行。
func TestRenderReplaySummaryNoTransition(t *testing.T) {
	days := []ReplayDay{
		mkReplayDay("2026-07-09", StateNormal, StateNormal, 1, 0, nil, nil),
		mkReplayDay("2026-07-10", StateNormal, StateNormal, 2, 0, nil, nil),
	}
	out := RenderReplaySummary(testConfig(), days)
	assert.Contains(t, out, "期间转移 0 次")
	assert.Contains(t, out, "转移：无")
	assert.NotContains(t, out, "STALE 统计")
	assert.Contains(t, out, "AMBER 峰值：0/7")
}

// functional: 极值方向逐指标——vix 取最大、t10y2y 取最小、usdjpy 取最小（reviewer
// 补强，落界判别）；STALE 日读数不计入极值；STALE 统计仅列非零。
func TestRenderReplaySummaryExtremesAndStale(t *testing.T) {
	days := []ReplayDay{
		mkReplayDay("2026-07-08", StateNormal, StateNormal, 1, 0, nil,
			map[string]float64{IndVIX: 30, IndT10Y2Y: -20, IndUSDJPY: 150}),
		mkReplayDay("2026-07-09", StateNormal, StateNormal, 2, 0,
			map[string]Status{IndMOVE: StatusStale},
			map[string]float64{IndVIX: 80.9, IndT10Y2Y: -55, IndMOVE: 999, IndUSDJPY: 145}),
		mkReplayDay("2026-07-10", StateNormal, StateNormal, 3, 0, nil,
			map[string]float64{IndVIX: 25, IndT10Y2Y: 10, IndMOVE: 120, IndUSDJPY: 155}),
	}
	out := RenderReplaySummary(testConfig(), days)
	assert.Contains(t, out, "vix 80.9（2026-07-09）")     // 最大值方向
	assert.Contains(t, out, "t10y2y -55bp（2026-07-09）") // 最小值方向
	assert.Contains(t, out, "usdjpy 145.0（2026-07-09）") // 最小值方向（若取最大会是 155@07-10）
	assert.Contains(t, out, "move 120.0（2026-07-10）")   // STALE 日 999 不计入
	assert.Contains(t, out, "move 缺数 1 交易日")
	assert.NotContains(t, out, "vix 缺数")
}

// boundary(reviewer 补强): 窗口首日即为转移日 → 期初态取 PrevState 而非 State。
func TestRenderReplaySummaryFirstDayTransition(t *testing.T) {
	days := []ReplayDay{
		mkReplayDay("2026-07-07", StateNormal, StateWatch, 1, 3, nil, nil), // 首日即转移
		mkReplayDay("2026-07-08", StateWatch, StateWatch, 2, 2, nil, nil),
	}
	out := RenderReplaySummary(testConfig(), days)
	assert.Contains(t, out, "状态：NORMAL 起步") // 取 PrevState；若取 State 会是 "WATCH 起步"
	assert.Contains(t, out, "期间转移 1 次")
	assert.Contains(t, out, "2026-07-07 NORMAL → WATCH")
}

// error_handling: days 为空返回空串，不 panic。
func TestRenderReplaySummaryEmpty(t *testing.T) {
	assert.Equal(t, "", RenderReplaySummary(testConfig(), nil))
	assert.NotPanics(t, func() { RenderReplaySummary(testConfig(), []ReplayDay{}) })
}

// non_functional: 千日量级 + 多次转移仍 ≤4096（2006–2009 全期近似）。
func TestRenderReplaySummaryUnder4096(t *testing.T) {
	var days []ReplayDay
	state := StateNormal
	for i := 0; i < 1000; i++ {
		d := addDays("2006-01-02", i)
		prev := state
		if i%100 == 99 { // 10 次转移
			if state == StateNormal {
				state = StateCrisis
			} else {
				state = StateNormal
			}
		}
		sd := i%100 + 1
		days = append(days, mkReplayDay(d, prev, state, sd, i%8, nil, nil))
	}
	out := RenderReplaySummary(testConfig(), days)
	require.LessOrEqual(t, len([]rune(out)), 4096)
	for _, banned := range []string{"必然", "一定", "即将"} { // 禁词沿用
		assert.NotContains(t, out, banned)
	}
	assert.Equal(t, 1, strings.Count(out, "指标极值"))
}
