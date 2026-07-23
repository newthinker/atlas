package valuation

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/newthinker/atlas/internal/core"
)

// Context Checkpoint: done_criteria → test mapping
//   functional[0] 阶梯对齐(生效前 100/2=50、生效后 100/4=25)+升序等长 → TestReconstructPESeriesStepAlignment
//   functional[1] RollingPercentile 窗口内 0-100 分位                  → TestRollingPercentile
//   boundary[0]   窗口样本 < minPoints → NaN                          → TestRollingPercentile
//   boundary[1]   对齐 EPS<=0 或 close<=0 的交易日被跳过               → TestReconstructPESeriesSkipsNonPositive
//   error[0]      正 EPS 点 < MinEPSPoints → ErrInsufficientEPS       → TestReconstructPESeriesInsufficient

func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func makeEPS(n int, start time.Time, eps float64) []core.EPSPoint {
	pts := make([]core.EPSPoint, n)
	for i := range pts {
		pts[i] = core.EPSPoint{Date: start.AddDate(0, 3*i, 0), EPS: eps}
	}
	return pts
}

func TestReconstructPESeriesStepAlignment(t *testing.T) {
	eps := makeEPS(8, day(2024, 1, 1), 2.0) // 8 个季度点,EPS(TTM)=2
	eps[7].EPS = 4.0                        // 最后一点翻倍 → PE 阶梯下移
	last := eps[7].Date                     // 2025-10-01
	closes := []core.OHLCV{
		{Time: last.AddDate(0, 0, -1), Close: 100}, // 生效前:PE=100/2=50
		{Time: last.AddDate(0, 0, 1), Close: 100},  // 生效后:PE=100/4=25
	}
	dates, pe, err := ReconstructPESeries(closes, eps)
	require.NoError(t, err)
	require.Len(t, pe, 2)
	assert.True(t, dates[0].Before(dates[1]))
	assert.InDelta(t, 50.0, pe[0], 1e-9)
	assert.InDelta(t, 25.0, pe[1], 1e-9)
}

func TestReconstructPESeriesInsufficient(t *testing.T) {
	_, _, err := ReconstructPESeries(
		[]core.OHLCV{{Time: day(2026, 1, 1), Close: 10}},
		makeEPS(MinEPSPoints-1, day(2024, 1, 1), 1.0))
	assert.ErrorIs(t, err, ErrInsufficientEPS)
}

func TestReconstructPESeriesSkipsNonPositive(t *testing.T) {
	// boundary[1]: 对齐 EPS<=0 或 close<=0 的交易日不进输出序列
	// 9 个季度点(前 8 正满足 MinEPSPoints 门槛)+ 末尾一个亏损季度
	eps := makeEPS(9, day(2024, 1, 1), 2.0)
	eps[8].EPS = -1.0 // 最后季度亏损 → 生效后交易日被跳过
	last := eps[8].Date
	closes := []core.OHLCV{
		{Time: last.AddDate(0, 0, -1), Close: 100}, // 对齐正 EPS,保留
		{Time: last.AddDate(0, 0, 1), Close: 100},  // 对齐 EPS=-1 → 跳过
		{Time: last.AddDate(0, 0, -2), Close: 0},   // close<=0 → 跳过
	}
	dates, pe, err := ReconstructPESeries(closes, eps)
	require.NoError(t, err)
	require.Len(t, pe, 1)
	require.Len(t, dates, 1)
	assert.InDelta(t, 50.0, pe[0], 1e-9)
}

func TestRollingPercentile(t *testing.T) {
	// 4 个点,窗口 1 年,minPoints=2:
	// i=0/i=1 窗口内样本不足 → NaN;i>=2 窗口含前值且当前最大 → 100 分位
	dates := []time.Time{day(2025, 1, 1), day(2025, 4, 1), day(2025, 7, 1), day(2025, 10, 1)}
	values := []float64{10, 20, 30, 40}
	got := RollingPercentile(dates, values, 1, 2)
	require.Len(t, got, 4)
	assert.True(t, math.IsNaN(got[0]))
	assert.True(t, math.IsNaN(got[1]))
	assert.InDelta(t, 100.0, got[2], 1e-9) // 窗口 {10,20},当前 30 → 100
	assert.InDelta(t, 100.0, got[3], 1e-9)
}
