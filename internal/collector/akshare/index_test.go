package akshare

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Context Checkpoint (TASK-003):
//   指数走 lg 中文名映射(000300.SH→沪深300)+滚动市盈率中文键+乱序→升序+窗口过滤 → TestFetchIndexSeries
//   PB/PSTTM 恒 NaN                                                             → TestFetchIndexSeries
//   未登记指数 → error 含 no lg index mapping                                   → TestFetchIndexUnknownSymbol

func TestFetchIndexSeries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/public/stock_index_pe_lg", r.URL.Path)
		assert.Equal(t, "沪深300", r.URL.Query().Get("symbol"))
		json.NewEncoder(w).Encode([]map[string]any{
			{"日期": "2026-07-23", "滚动市盈率": 14.6},
			{"日期": "2026-07-22", "滚动市盈率": 14.5}, // 乱序
			{"日期": "2015-01-05", "滚动市盈率": 9.9}, // 窗口外,应被过滤
		})
	}))
	defer srv.Close()

	c := New(srv.URL)
	pts, err := c.FetchIndexValuationSeries("000300.SH", day(2026, 7, 1), day(2026, 7, 23))
	require.NoError(t, err)
	require.Len(t, pts, 2, "窗口外行应被过滤")
	assert.True(t, pts[0].Date.Before(pts[1].Date), "输出升序")
	assert.Equal(t, "2026-07-22", pts[0].Date.Format("2006-01-02"))
	assert.Equal(t, 14.5, pts[0].PETTM)
	assert.Equal(t, 14.6, pts[1].PETTM)
	assert.True(t, math.IsNaN(pts[0].PB), "指数兜底仅 PE,PB NaN")
	assert.True(t, math.IsNaN(pts[1].PSTTM), "指数兜底仅 PE,PS NaN")
}

func TestFetchIndexUnknownSymbol(t *testing.T) {
	c := New("http://127.0.0.1:1")
	_, err := c.FetchIndexValuationSeries("^GSPC", day(2026, 1, 1), day(2026, 7, 23))
	assert.ErrorContains(t, err, "no lg index mapping")
}
