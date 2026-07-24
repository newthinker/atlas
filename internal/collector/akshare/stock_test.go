package akshare

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Context Checkpoint (TASK-002, live 校正 akshare 1.18.75):
//   A股/HK 统一走百度双指标 stock_zh/hk_valuation_baidu(lg 已被移除)   → TestFetchAStockBaiduSeries / TestFetchHKStockMerge
//   窗口过滤 + 升序 + PE/PB 双指标按 PE 日期主键合并                     → TestFetchAStockBaiduSeries / TestFetchAStockMissingAndOrder
//   period 策略: <=1y 近一年 / <=3y 近三年 / 更长 近十年+近三年 双拉合并 → TestFetchBaiduPeriodStrategy
//   date 带 T00:00:00.000 后缀(fdate[:10] 兼容)                        → TestFetchAStockBaiduSeries
//   字符串数值经 strconv 解析                                           → TestFetchAStockStringNumerics
//   拉取>0 行全 PETTM NaN(schema 漂移)→ probable schema drift          → TestFetchAStockAllNaNGuard
//   PSTTM 恒 NaN(百度无 PS 能力)                                       → TestFetchAStockBaiduSeries / TestFetchHKStockMerge
//   不支持的 market → error 含 unsupported market                       → TestFetchStockUnsupportedMarket

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

// baiduServer mock 百度接口: 断言 path/symbol/period,按 indicator 分发 PE/PB 行。
// 仅用于单 period(窗口 <=1y → 近一年)的简单用例。
func baiduServer(t *testing.T, wantAPI, wantSymbol, wantPeriod string, pe, pb []map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/public/"+wantAPI, r.URL.Path)
		assert.Equal(t, wantSymbol, r.URL.Query().Get("symbol"))
		assert.Equal(t, wantPeriod, r.URL.Query().Get("period"))
		switch r.URL.Query().Get("indicator") {
		case "市盈率(TTM)":
			json.NewEncoder(w).Encode(pe)
		case "市净率":
			json.NewEncoder(w).Encode(pb)
		default:
			t.Errorf("unexpected indicator %q", r.URL.Query().Get("indicator"))
		}
	}))
}

func TestFetchAStockBaiduSeries(t *testing.T) {
	srv := baiduServer(t, "stock_zh_valuation_baidu", "600519", "近一年",
		[]map[string]any{
			{"date": "2015-01-05T00:00:00.000", "value": 10.0}, // 窗口外,应被过滤
			{"date": "2026-07-22T00:00:00.000", "value": 22.5},
			{"date": "2026-07-23T00:00:00.000", "value": 22.7},
		},
		[]map[string]any{
			{"date": "2026-07-22T00:00:00.000", "value": 8.1},
			{"date": "2026-07-23T00:00:00.000", "value": 8.2},
		})
	defer srv.Close()

	c := New(srv.URL)
	pts, err := c.FetchStockValuationSeries("600519.SH", "CN_A", day(2026, 7, 1), day(2026, 7, 23))
	require.NoError(t, err)
	require.Len(t, pts, 2, "2015 年的行应被窗口过滤")
	assert.Equal(t, "2026-07-22", pts[0].Date.Format("2006-01-02"), "输出升序;T00 后缀经 fdate[:10] 兼容")
	assert.Equal(t, 22.5, pts[0].PETTM)
	assert.Equal(t, 8.1, pts[0].PB)
	assert.Equal(t, 22.7, pts[1].PETTM)
	assert.Equal(t, 8.2, pts[1].PB)
	assert.True(t, math.IsNaN(pts[0].PSTTM), "百度无 PS,PSTTM 恒 NaN")
	assert.True(t, math.IsNaN(pts[1].PSTTM))
}

func TestFetchAStockMissingAndOrder(t *testing.T) {
	srv := baiduServer(t, "stock_zh_valuation_baidu", "600519", "近一年",
		[]map[string]any{
			{"date": "2026-07-23", "value": 22.7}, // 乱序
			{"date": "2026-07-22", "value": 22.5},
		},
		[]map[string]any{
			{"date": "2026-07-22", "value": 8.1}, // 07-23 无 PB → NaN
		})
	defer srv.Close()

	c := New(srv.URL)
	pts, err := c.FetchStockValuationSeries("600519.SH", "CN_A", day(2026, 7, 1), day(2026, 7, 23))
	require.NoError(t, err)
	require.Len(t, pts, 2)
	assert.True(t, pts[0].Date.Before(pts[1].Date), "输出必须升序")
	assert.Equal(t, 8.1, pts[0].PB)
	assert.True(t, math.IsNaN(pts[1].PB), "缺 PB 字段→NaN")
	assert.True(t, math.IsNaN(pts[0].PSTTM))
}

func TestFetchAStockStringNumerics(t *testing.T) {
	srv := baiduServer(t, "stock_zh_valuation_baidu", "600519", "近一年",
		[]map[string]any{{"date": "2026-07-22", "value": "22.5"}},
		[]map[string]any{{"date": "2026-07-22", "value": "8.1"}})
	defer srv.Close()

	c := New(srv.URL)
	pts, err := c.FetchStockValuationSeries("600519.SH", "CN_A", day(2026, 7, 1), day(2026, 7, 23))
	require.NoError(t, err)
	require.Len(t, pts, 1)
	assert.Equal(t, 22.5, pts[0].PETTM, "字符串数值须解析为 float")
	assert.Equal(t, 8.1, pts[0].PB)
}

func TestFetchAStockAllNaNGuard(t *testing.T) {
	// value 键漂移(value→val),PE 解析后全部 NaN,守卫应报错而非静默产出空 PE 序列。
	srv := baiduServer(t, "stock_zh_valuation_baidu", "600519", "近一年",
		[]map[string]any{
			{"date": "2026-07-22", "val": 22.5},
			{"date": "2026-07-23", "val": 22.7},
		},
		[]map[string]any{{"date": "2026-07-22", "value": 8.1}})
	defer srv.Close()

	c := New(srv.URL)
	_, err := c.FetchStockValuationSeries("600519.SH", "CN_A", day(2026, 7, 1), day(2026, 7, 23))
	require.Error(t, err)
	assert.ErrorContains(t, err, "probable schema drift")
}

// TestFetchBaiduPeriodStrategy 断言 period 选择: <=1y→近一年;<=3y→近三年;
// 更长→近十年+近三年双拉,近三年日频值覆盖近十年同日稀疏值。
func TestFetchBaiduPeriodStrategy(t *testing.T) {
	end := day(2026, 7, 23)
	cases := []struct {
		name        string
		start       time.Time
		wantPeriods []string
	}{
		{"span<=1y→近一年", day(2026, 1, 1), []string{"近一年"}},
		{"span<=3y→近三年", day(2024, 1, 1), []string{"近三年"}},
		{"span>3y→近十年+近三年", day(2016, 1, 1), []string{"近十年", "近三年"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			seen := map[string]bool{}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				period := r.URL.Query().Get("period")
				mu.Lock()
				seen[period] = true
				mu.Unlock()
				// 每 period 回一行 PE/PB,近三年覆盖近十年同日值(此处仅验策略,值不冲突)
				var rows []map[string]any
				if r.URL.Query().Get("indicator") == "市盈率(TTM)" {
					rows = []map[string]any{{"date": "2026-07-23", "value": 22.7}}
				} else {
					rows = []map[string]any{{"date": "2026-07-23", "value": 8.2}}
				}
				json.NewEncoder(w).Encode(rows)
			}))
			defer srv.Close()

			c := New(srv.URL)
			_, err := c.FetchStockValuationSeries("600519.SH", "CN_A", tc.start, end)
			require.NoError(t, err)
			for _, p := range tc.wantPeriods {
				assert.True(t, seen[p], "应请求 period=%s", p)
			}
			assert.Len(t, seen, len(tc.wantPeriods), "不得请求策略外的 period")
		})
	}
}

func TestFetchStockUnsupportedMarket(t *testing.T) {
	c := New("http://127.0.0.1:1")
	_, err := c.FetchStockValuationSeries("XXX", "US", day(2026, 1, 1), day(2026, 7, 23))
	assert.ErrorContains(t, err, "unsupported market")
}

// TestFetchHKStockMerge: HK 走同一百度路径,0700.HK→00700,PE 日期为主键合并,
// 有 PE 缺 PB→PB NaN;仅有 PB 无 PE 的日不产出行;PSTTM 恒 NaN。
func TestFetchHKStockMerge(t *testing.T) {
	srv := baiduServer(t, "stock_hk_valuation_baidu", "00700", "近一年",
		[]map[string]any{
			{"date": "2026-07-22", "value": 22.1},
			{"date": "2026-07-23", "value": 22.4},
		},
		[]map[string]any{
			{"date": "2026-07-23", "value": 3.1}, // 07-22 无 PB → NaN
			{"date": "2026-07-20", "value": 2.9}, // 仅有 PB 无 PE → 不产出行
		})
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
