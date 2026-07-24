package akshare

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
//   functional[0] A股走 lg 接口+符号映射(600519.SH→600519)+窗口过滤 → TestFetchAStockSeries
//   functional[1] [start,end] 闭区间过滤且升序                       → TestFetchAStockSeries / TestFetchAStockMissingAndOrder
//   functional[2] 字段解析 trade_date/pe_ttm/pb/ps_ttm               → TestFetchAStockSeries
//   boundary[0]   缺字段→NaN、乱序→升序、仅 ps 无 ps_ttm 经兜底键取值 → TestFetchAStockMissingAndOrder / TestFetchAStockPSFallback
//   error[1]      不支持的 market → error 含 unsupported market       → TestFetchStockUnsupportedMarket

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func lgServer(t *testing.T, wantSymbol string, rows []map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/public/stock_a_indicator_lg", r.URL.Path)
		assert.Equal(t, wantSymbol, r.URL.Query().Get("symbol"))
		json.NewEncoder(w).Encode(rows)
	}))
}

func TestFetchAStockSeries(t *testing.T) {
	srv := lgServer(t, "600519", []map[string]any{
		{"trade_date": "2015-01-05", "pe_ttm": 10.0, "pb": 3.0, "ps_ttm": 5.0}, // 窗口外,应被过滤
		{"trade_date": "2026-07-22", "pe_ttm": 22.5, "pb": 8.1, "ps_ttm": 10.2},
		{"trade_date": "2026-07-23", "pe_ttm": 22.7, "pb": 8.2, "ps_ttm": 10.3},
	})
	defer srv.Close()

	c := New(srv.URL)
	pts, err := c.FetchStockValuationSeries("600519.SH", "CN_A", day(2026, 7, 1), day(2026, 7, 23))
	require.NoError(t, err)
	require.Len(t, pts, 2, "2015 年的行应被窗口过滤")
	assert.Equal(t, 22.5, pts[0].PETTM)
	assert.Equal(t, 10.2, pts[0].PSTTM)
	assert.Equal(t, 8.2, pts[1].PB)
	assert.Equal(t, 22.7, pts[1].PETTM)
	assert.Equal(t, "2026-07-22", pts[0].Date.Format("2006-01-02"), "输出升序")
	assert.Equal(t, "2026-07-23", pts[1].Date.Format("2006-01-02"))
}

func TestFetchAStockMissingAndOrder(t *testing.T) {
	srv := lgServer(t, "600519", []map[string]any{
		{"trade_date": "2026-07-23", "pe_ttm": 22.7}, // 乱序 + 缺 pb/ps
		{"trade_date": "2026-07-22", "pe_ttm": 22.5, "pb": 8.1},
	})
	defer srv.Close()

	c := New(srv.URL)
	pts, err := c.FetchStockValuationSeries("600519.SH", "CN_A", day(2026, 7, 1), day(2026, 7, 23))
	require.NoError(t, err)
	require.Len(t, pts, 2)
	assert.True(t, pts[0].Date.Before(pts[1].Date), "输出必须升序")
	assert.True(t, math.IsNaN(pts[1].PB), "缺字段→NaN")
	assert.True(t, math.IsNaN(pts[0].PSTTM))
}

// TestFetchAStockPSFallback 显式覆盖 DoD: 仅有 ps 无 ps_ttm 的行经兜底键取到值。
func TestFetchAStockPSFallback(t *testing.T) {
	srv := lgServer(t, "600519", []map[string]any{
		{"trade_date": "2026-07-22", "pe_ttm": 22.5, "pb": 8.1, "ps": 9.9}, // 仅 ps,无 ps_ttm
	})
	defer srv.Close()

	c := New(srv.URL)
	pts, err := c.FetchStockValuationSeries("600519.SH", "CN_A", day(2026, 7, 1), day(2026, 7, 23))
	require.NoError(t, err)
	require.Len(t, pts, 1)
	assert.Equal(t, 9.9, pts[0].PSTTM, "无 ps_ttm 时须经兜底键 ps 取到值")
}

func TestFetchStockUnsupportedMarket(t *testing.T) {
	c := New("http://127.0.0.1:1")
	_, err := c.FetchStockValuationSeries("XXX", "US", day(2026, 1, 1), day(2026, 7, 23))
	assert.ErrorContains(t, err, "unsupported market")
}

// Context Checkpoint (TASK-003):
//   HK 双指标按日期合并、0700.HK→00700、period=全部、有 PE 缺 PB→PB NaN → TestFetchHKStockMerge
//   仅有 PB 无 PE 的日不产出行(口径明确,无 PE 无法算分位)               → TestFetchHKStockMerge
func TestFetchHKStockMerge(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/public/stock_hk_valuation_baidu", r.URL.Path)
		assert.Equal(t, "00700", r.URL.Query().Get("symbol"))
		assert.Equal(t, "全部", r.URL.Query().Get("period"))
		switch r.URL.Query().Get("indicator") {
		case "市盈率(TTM)":
			json.NewEncoder(w).Encode([]map[string]any{
				{"date": "2026-07-22", "value": 22.1},
				{"date": "2026-07-23", "value": 22.4},
			})
		case "市净率":
			json.NewEncoder(w).Encode([]map[string]any{
				{"date": "2026-07-23", "value": 3.1}, // 07-22 无 PB → NaN
				{"date": "2026-07-20", "value": 2.9}, // 仅有 PB 无 PE → 不产出行
			})
		default:
			t.Errorf("unexpected indicator %q", r.URL.Query().Get("indicator"))
		}
	}))
	defer srv.Close()

	c := New(srv.URL)
	pts, err := c.FetchStockValuationSeries("0700.HK", "HK", day(2026, 7, 1), day(2026, 7, 23))
	require.NoError(t, err)
	require.Len(t, pts, 2, "仅有 PB 无 PE 的 07-20 不产出行(以 PE 日期为主键)")
	assert.Equal(t, "2026-07-22", pts[0].Date.Format("2006-01-02"), "输出升序")
	assert.Equal(t, 22.1, pts[0].PETTM)
	assert.True(t, math.IsNaN(pts[0].PB), "该日仅有 PE → PB NaN")
	assert.Equal(t, 22.4, pts[1].PETTM)
	assert.Equal(t, 3.1, pts[1].PB)
	assert.True(t, math.IsNaN(pts[1].PSTTM), "HK 无 PS,恒 NaN")
	assert.True(t, math.IsNaN(pts[0].PSTTM))
}
