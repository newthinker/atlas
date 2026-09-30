package tiingo

// Context Checkpoint: done_criteria → test mapping（TASK-009）
// functional[0]     NVDA 样本 [06-05, 06-11] 折算含 Open、Volume ×10、升序  → TestNormalizeNVDASplitAndClip
// functional[1]     先折算后截取（end 早于拆股）                         → TestNormalizeEndBeforeLaterSplitStillAdjusted
// boundary[0]       非整数因子：Volume 四舍五入（3×1.5 → 5）             → TestNormalizeNonIntegerFactorRoundsVolume
// boundary[1]       乱序输入仍升序、按日期累乘                           → TestNormalizeUnorderedInput
// boundary[2]       坏日期 / 缺价行跳过、volume null 记 0、缺价行因子参与 → TestNormalizeSkipsBadRows
// boundary[3]       按日闭区间、时分秒截断、0 根返回空切片               → TestNormalizeClipInclusiveByDay
// error_handling[0] splitFactor 0 / 负数整段失败                          → TestNormalizeRejectsInvalidSplitFactor

import (
	"encoding/json"
	"math"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// day 解析 YYYY-MM-DD 为 UTC 零点（本包测试共用）。
func day(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func fp(v float64) *float64 { return &v }

// row 构造一行全字段非 null 的价格（O/H/L/C 同值）。
func row(date string, price, volume, splitFactor float64) priceRow {
	return priceRow{
		Date: date + "T00:00:00.000Z",
		Open: fp(price), High: fp(price), Low: fp(price), Close: fp(price),
		Volume: fp(volume), SplitFactor: fp(splitFactor),
	}
}

func loadSample(t *testing.T) []priceRow {
	t.Helper()
	raw, err := os.ReadFile("testdata/nvda_split_sample.json")
	require.NoError(t, err)
	var rows []priceRow
	require.NoError(t, json.Unmarshal(raw, &rows))
	return rows
}

func TestNormalizeNVDASplitAndClip(t *testing.T) {
	bars, err := normalize("NVDA", loadSample(t), day("2024-06-05"), day("2024-06-11"))
	require.NoError(t, err)
	require.Len(t, bars, 5, "闭区间 [06-05, 06-11] 共 5 个交易日")

	assert.Equal(t, day("2024-06-05"), bars[0].Time)
	assert.InDelta(t, 122.44, bars[0].Open, 1e-9, "Open 同样 ÷10")
	assert.InDelta(t, 123.44, bars[0].High, 1e-9)
	assert.InDelta(t, 121.44, bars[0].Low, 1e-9)
	assert.InDelta(t, 122.44, bars[0].Close, 1e-9, "拆股前 ÷10，与 yahoo close 同口径")
	assert.Equal(t, int64(10000), bars[0].Volume, "成交量 ×10")

	assert.Equal(t, day("2024-06-10"), bars[3].Time)
	assert.InDelta(t, 121.79, bars[3].Open, 1e-9, "拆股当日及之后不折算")
	assert.InDelta(t, 122.79, bars[3].High, 1e-9)
	assert.InDelta(t, 120.79, bars[3].Low, 1e-9)
	assert.InDelta(t, 121.79, bars[3].Close, 1e-9)
	assert.Equal(t, int64(10000), bars[3].Volume)

	for i, b := range bars {
		assert.Equal(t, "NVDA", b.Symbol)
		assert.Equal(t, "1d", b.Interval)
		if i > 0 {
			assert.True(t, bars[i-1].Time.Before(b.Time), "升序")
		}
	}
}

func TestNormalizeEndBeforeLaterSplitStillAdjusted(t *testing.T) {
	bars, err := normalize("NVDA", loadSample(t), day("2024-06-03"), day("2024-06-07"))
	require.NoError(t, err)
	require.Len(t, bars, 5)
	assert.InDelta(t, 115.0, bars[0].Close, 1e-9, "end 早于拆股日，仍须按其后的拆股折算")
	assert.InDelta(t, 120.888, bars[4].Close, 1e-9)
}

func TestNormalizeNonIntegerFactorRoundsVolume(t *testing.T) {
	rows := []priceRow{
		row("2024-06-03", 3, 3, 1),
		row("2024-06-04", 2, 7, 1.5),
	}
	bars, err := normalize("AAPL", rows, day("2024-06-01"), day("2024-06-30"))
	require.NoError(t, err)
	require.Len(t, bars, 2)
	assert.Equal(t, int64(5), bars[0].Volume, "3×1.5=4.5 四舍五入为 5（截断会得 4）")
	assert.InDelta(t, 2.0, bars[0].Open, 1e-9, "价格 ÷1.5")
	assert.InDelta(t, 2.0, bars[0].High, 1e-9)
	assert.InDelta(t, 2.0, bars[0].Low, 1e-9)
	assert.InDelta(t, 2.0, bars[0].Close, 1e-9)
	assert.Equal(t, int64(7), bars[1].Volume, "拆股当日不折算")
	assert.InDelta(t, 2.0, bars[1].Close, 1e-9)
}

func TestNormalizeUnorderedInput(t *testing.T) {
	sample := loadSample(t)
	want, err := normalize("NVDA", sample, day("2024-06-03"), day("2024-06-14"))
	require.NoError(t, err)
	require.Len(t, want, len(sample))

	reversed := make([]priceRow, len(sample))
	for i, r := range sample {
		reversed[len(sample)-1-i] = r
	}
	shuffled := make([]priceRow, 0, len(sample))
	for _, i := range []int{6, 2, 9, 0, 5, 3, 8, 1, 7, 4} {
		shuffled = append(shuffled, sample[i])
	}

	for name, rows := range map[string][]priceRow{"倒序": reversed, "打乱": shuffled} {
		t.Run(name, func(t *testing.T) {
			got, err := normalize("NVDA", rows, day("2024-06-03"), day("2024-06-14"))
			require.NoError(t, err)
			require.Len(t, got, len(want))
			for i := range want {
				assert.Equal(t, want[i].Time, got[i].Time, "第 %d 根", i)
				assert.InDelta(t, want[i].Open, got[i].Open, 1e-9, "第 %d 根", i)
				assert.InDelta(t, want[i].High, got[i].High, 1e-9, "第 %d 根", i)
				assert.InDelta(t, want[i].Low, got[i].Low, 1e-9, "第 %d 根", i)
				assert.InDelta(t, want[i].Close, got[i].Close, 1e-9, "第 %d 根", i)
				assert.Equal(t, want[i].Volume, got[i].Volume, "第 %d 根", i)
			}
		})
	}
}

func TestNormalizeSkipsBadRows(t *testing.T) {
	nullOf := func(date string, clear func(*priceRow)) priceRow {
		r := row(date, 1, 1, 1)
		clear(&r)
		return r
	}
	good := row("2024-06-05", 2, 5, 1)
	good.Volume = nil

	rows := []priceRow{
		{Date: "bad", Open: fp(1), High: fp(1), Low: fp(1), Close: fp(1), Volume: fp(1), SplitFactor: fp(1)},
		{Date: "2024-6", Open: fp(1), High: fp(1), Low: fp(1), Close: fp(1), Volume: fp(1), SplitFactor: fp(1)},
		nullOf("2024-06-03", func(r *priceRow) { r.Close = nil }),
		nullOf("2024-06-04", func(r *priceRow) { r.Open = nil }),
		nullOf("2024-06-06", func(r *priceRow) { r.High = nil }),
		nullOf("2024-06-07", func(r *priceRow) { r.Low = nil }),
		good,
	}
	bars, err := normalize("AAPL", rows, day("2024-06-01"), day("2024-06-30"))
	require.NoError(t, err)
	require.Len(t, bars, 1, "坏日期 / close 缺失 / O·H·L 任一缺失的行跳过")
	assert.Equal(t, day("2024-06-05"), bars[0].Time)
	assert.Equal(t, int64(0), bars[0].Volume, "volume 缺失记 0")
	assert.InDelta(t, 2.0, bars[0].Close, 1e-9)

	t.Run("splitFactor null 视为 1", func(t *testing.T) {
		r := row("2024-06-04", 5, 1, 1)
		r.SplitFactor = nil
		bars, err := normalize("AAPL", []priceRow{row("2024-06-03", 10, 1, 1), r}, day("2024-06-01"), day("2024-06-30"))
		require.NoError(t, err)
		require.Len(t, bars, 2)
		assert.InDelta(t, 10.0, bars[0].Close, 1e-9)
	})

	t.Run("拆股当日价格全 null 时因子仍参与折算", func(t *testing.T) {
		split := priceRow{Date: "2024-06-04T00:00:00.000Z", SplitFactor: fp(2)}
		bars, err := normalize("AAPL", []priceRow{row("2024-06-03", 100, 1, 1), split}, day("2024-06-01"), day("2024-06-30"))
		require.NoError(t, err)
		require.Len(t, bars, 1)
		assert.InDelta(t, 50.0, bars[0].Close, 1e-9, "缺价行的 splitFactor 仍参与折算")
	})
}

func TestNormalizeClipInclusiveByDay(t *testing.T) {
	sample := loadSample(t)
	at := func(s string, h, m int) time.Time {
		return day(s).Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute)
	}

	cases := []struct {
		name        string
		start, end  time.Time
		first, last time.Time
		n           int
	}{
		{"零点端点", day("2024-06-04"), day("2024-06-10"), day("2024-06-04"), day("2024-06-10"), 5},
		{"带时分秒的端点按日截断", at("2024-06-04", 23, 30), at("2024-06-10", 0, 1), day("2024-06-04"), day("2024-06-10"), 5},
		{"单日区间（start 时刻晚于 end 时刻）", at("2024-06-11", 15, 0), at("2024-06-11", 9, 0), day("2024-06-11"), day("2024-06-11"), 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bars, err := normalize("NVDA", sample, tc.start, tc.end)
			require.NoError(t, err)
			require.Len(t, bars, tc.n)
			assert.Equal(t, tc.first, bars[0].Time)
			assert.Equal(t, tc.last, bars[len(bars)-1].Time)
		})
	}

	t.Run("截取后 0 根返回空切片", func(t *testing.T) {
		for _, rows := range [][]priceRow{sample, nil} {
			bars, err := normalize("NVDA", rows, day("2025-01-01"), day("2025-01-31"))
			require.NoError(t, err)
			assert.NotNil(t, bars, "空切片而非 nil")
			assert.Empty(t, bars)
		}
	})
}

func TestNormalizeRejectsInvalidSplitFactor(t *testing.T) {
	for _, sf := range []float64{0, -2, math.NaN(), math.Inf(1)} {
		// 坏因子行在截取窗口之外同样整段失败：折算依赖全量行。
		rows := []priceRow{row("2024-06-03", 10, 1, 1), row("2024-06-20", 10, 1, sf)}
		bars, err := normalize("AAPL", rows, day("2024-06-01"), day("2024-06-05"))
		require.Error(t, err, "splitFactor=%v", sf)
		assert.Nil(t, bars)
		assert.Contains(t, err.Error(), "invalid splitFactor")
		assert.Contains(t, err.Error(), "2024-06-20")
	}
}
