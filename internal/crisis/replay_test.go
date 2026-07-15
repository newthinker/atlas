package crisis

// Context Checkpoint: done_criteria → test mapping (replay)
// functional[0] "暖机逐日推进,窗口切片;首日 NORMAL/StateDays=1,07-07 Transitioned CRISIS/StateDays=1,次日=2,前一日 NORMAL StateDays=8" → TestReplayRangeWarmupAndStateDays
// boundary[0]   "窗口切片不影响暖机计数:from=07-08 返回 3 日,首日 CRISIS/PrevState=CRISIS(暖机)/StateDays=2"                    → TestReplayRangeWindowSlice
// boundary[1]   "窗口内无交易日→空切片 err=nil; from>to→error"                                                              → TestReplayRangeEmptyAndBadRange
// functional[1] executeCrisisReplay 逐字节不变 → cmd/atlas TestExecuteCrisisReplay*(回归黄金,不在本文件)

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// replaySeries 铺 n 个连续观测日（末日 end），7 指标常值，最后 redTail 日
// vix/move 置红（情绪双红 → CRISIS）。
func replaySeries(end string, n, redTail int) memSeries {
	base := map[string]float64{
		IndVIX: 15, IndMOVE: 70, IndSOFREFFR: -10, IndHYOAS: 400,
		IndT10Y2Y: 35, IndNFCI: -0.5, IndUSDJPY: 150,
	}
	m := memSeries{}
	for i := 0; i < n; i++ {
		d := addDays(end, i-n+1)
		for ind, v := range base {
			if i >= n-redTail {
				if ind == IndVIX {
					v = 35
				}
				if ind == IndMOVE {
					v = 130
				}
			}
			m[ind] = append(m[ind], Observation{Date: d, Indicator: ind, Value: v})
		}
	}
	return m
}

// functional: 暖机逐日推进，窗口切片正确；转移日 StateDays=1、次日=2。
func TestReplayRangeWarmupAndStateDays(t *testing.T) {
	const end = "2026-07-10" // 12 日 = 2026-06-29..07-10，末 4 日红（07-07 起）
	sr := replaySeries(end, 12, 4)
	days, err := ReplayRange(testConfig(), sr, "2026-06-29", end)
	require.NoError(t, err)
	require.Len(t, days, 12)

	assert.Equal(t, StateNormal, days[0].Res.State)
	assert.Equal(t, 1, days[0].StateDays) // 首日 NORMAL 含当日

	trans := days[8] // 2026-07-07：NORMAL → CRISIS
	assert.Equal(t, "2026-07-07", trans.Date)
	require.True(t, trans.Res.Transitioned())
	assert.Equal(t, StateCrisis, trans.Res.State)
	assert.Equal(t, 1, trans.StateDays) // 转移日 = 1
	assert.Equal(t, 2, days[9].StateDays)
	assert.Equal(t, 8, days[7].StateDays) // 转移前 NORMAL 已持续 8 日
}

// boundary: 窗口切片不影响计数（暖机期计入）；期初态为暖机结果而非冷启动。
func TestReplayRangeWindowSlice(t *testing.T) {
	const end = "2026-07-10"
	sr := replaySeries(end, 12, 4)
	days, err := ReplayRange(testConfig(), sr, "2026-07-08", end)
	require.NoError(t, err)
	require.Len(t, days, 3)
	assert.Equal(t, StateCrisis, days[0].Res.State)
	assert.Equal(t, StateCrisis, days[0].Res.PrevState) // 暖机：07-07 已入 CRISIS
	assert.Equal(t, 2, days[0].StateDays)               // 07-08 = CRISIS 第 2 日
}

// boundary: 窗口无交易日 → 空切片不报错；from > to → 报错。
func TestReplayRangeEmptyAndBadRange(t *testing.T) {
	sr := replaySeries("2026-07-10", 12, 0)
	days, err := ReplayRange(testConfig(), sr, "2027-01-01", "2027-02-01")
	require.NoError(t, err)
	assert.Empty(t, days)

	_, err = ReplayRange(testConfig(), sr, "2026-07-10", "2026-07-01")
	assert.Error(t, err)
}
