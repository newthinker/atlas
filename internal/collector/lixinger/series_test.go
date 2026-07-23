package lixinger

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Context Checkpoint: done_criteria → test mapping
// functional[0]     "公司 symbol 请求打到 non_financial + payload 区间/指标齐全" → TestFetchValuationSeriesCompany
// functional[1]     "指数 symbol 打到 cn/index/fundamental + .mcw 五指标"        → TestFetchValuationSeriesIndexUsesMCW
// functional[2]     "cvpos ×100 为百分位;date 接受 ISO8601 与 YYYY-MM-DD"       → TestFetchValuationSeriesCompany / TestFetchValuationSeriesIndexUsesMCW / TestFetchValuationSeriesMissingAndOrder
// boundary[0]       "缺某指标 → NaN;响应乱序 → 输出按 Date 升序"                 → TestFetchValuationSeriesMissingAndOrder
// error_handling[0] "不支持的 symbol 返回 error;date 解析失败返回含 symbol 的包装 error" → TestFetchValuationSeriesUnsupportedSymbol / TestFetchValuationSeriesBadDate

func seriesServer(t *testing.T, wantPath string, capture *map[string]any, data []map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, wantPath, r.URL.Path)
		require.NoError(t, json.NewDecoder(r.Body).Decode(capture))
		json.NewEncoder(w).Encode(map[string]any{"code": 1, "data": data})
	}))
}

func TestFetchValuationSeriesCompany(t *testing.T) {
	var got map[string]any
	srv := seriesServer(t, "/cn/company/fundamental/non_financial", &got, []map[string]any{
		{"date": "2026-07-22T00:00:00+08:00", "pe_ttm": 25.5, "pb": 8.0, "ps_ttm": 10.1,
			"pe_ttm.y5.cvpos": 0.42, "pe_ttm.y10.cvpos": 0.30},
	})
	defer srv.Close()

	l := NewWithBaseURL("test-key", srv.URL)
	pts, err := l.FetchValuationSeries("600519.SH",
		time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 22, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)

	assert.Equal(t, "2026-07-01", got["startDate"])
	assert.Equal(t, "2026-07-22", got["endDate"])
	assert.ElementsMatch(t, []any{"600519"}, got["stockCodes"])
	assert.ElementsMatch(t,
		[]any{"pe_ttm", "pb", "ps_ttm", "pe_ttm.y5.cvpos", "pe_ttm.y10.cvpos"},
		got["metricsList"])

	require.Len(t, pts, 1)
	assert.Equal(t, 25.5, pts[0].PETTM)
	assert.InDelta(t, 42.0, pts[0].Pctl5Y, 1e-9) // cvpos ×100
	// 用日历日断言(而非 Truncate)避免依赖运行机器时区:ISO8601 带 +08:00 在其自身时区下即 7-22。
	assert.Equal(t, "2026-07-22", pts[0].Date.Format("2006-01-02"))
}

func TestFetchValuationSeriesIndexUsesMCW(t *testing.T) {
	var got map[string]any
	srv := seriesServer(t, "/cn/index/fundamental", &got, []map[string]any{
		{"date": "2026-07-22", "pe_ttm.mcw": 12.2, "pb.mcw": 1.3, "ps_ttm.mcw": 1.1,
			"pe_ttm.y5.mcw.cvpos": 0.41, "pe_ttm.y10.mcw.cvpos": 0.36},
	})
	defer srv.Close()

	l := NewWithBaseURL("test-key", srv.URL)
	pts, err := l.FetchValuationSeries("000300.SH",
		time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 22, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)

	assert.ElementsMatch(t,
		[]any{"pe_ttm.mcw", "pb.mcw", "ps_ttm.mcw", "pe_ttm.y5.mcw.cvpos", "pe_ttm.y10.mcw.cvpos"},
		got["metricsList"])
	require.Len(t, pts, 1)
	assert.Equal(t, 12.2, pts[0].PETTM)
	assert.InDelta(t, 36.0, pts[0].Pctl10Y, 1e-9)
}

func TestFetchValuationSeriesMissingAndOrder(t *testing.T) {
	var got map[string]any
	srv := seriesServer(t, "/cn/company/fundamental/non_financial", &got, []map[string]any{
		{"date": "2026-07-22", "pe_ttm": 26.0}, // 乱序 + 缺 cvpos/pb
		{"date": "2026-07-21", "pe_ttm": 25.0, "pe_ttm.y5.cvpos": 0.40},
	})
	defer srv.Close()

	l := NewWithBaseURL("test-key", srv.URL)
	pts, err := l.FetchValuationSeries("600519.SH",
		time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 22, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Len(t, pts, 2)
	assert.True(t, pts[0].Date.Before(pts[1].Date), "结果必须升序")
	assert.True(t, math.IsNaN(pts[1].Pctl5Y))
	assert.True(t, math.IsNaN(pts[0].PB))
}

func TestFetchValuationSeriesUnsupportedSymbol(t *testing.T) {
	l := NewWithBaseURL("test-key", "http://unused.invalid")
	_, err := l.FetchValuationSeries("AAPL",
		time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 22, 0, 0, 0, 0, time.UTC))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AAPL")
}

func TestFetchValuationSeriesBadDate(t *testing.T) {
	var got map[string]any
	srv := seriesServer(t, "/cn/company/fundamental/non_financial", &got, []map[string]any{
		{"date": "22/07/2026", "pe_ttm": 25.5},
	})
	defer srv.Close()

	l := NewWithBaseURL("test-key", srv.URL)
	_, err := l.FetchValuationSeries("600519.SH",
		time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 22, 0, 0, 0, 0, time.UTC))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "600519.SH")
	assert.Contains(t, err.Error(), "22/07/2026")
}
