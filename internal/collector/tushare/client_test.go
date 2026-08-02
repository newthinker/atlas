package tushare

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Context Checkpoint: done_criteria → test mapping
//   functional[0]     "FetchDailyBasic 发出 POST JSON 并解析 items 为 ValuationPoint" → TestFetchDailyBasic
//   functional[1]     "FetchIndexDaily/FetchHKDaily/FetchDaily → PricePoint"          → TestFetchPriceAPIs
//   functional[2]     "code=40203 → ErrNoPermission(errors.Is),文本含 api 名与 msg"   → TestErrNoPermission
//   boundary[0]       "items 空数组 → 空切片且 err=nil"                                → TestFetchDailyBasicEmpty
//   boundary[1]       "items 日期倒序时输出仍按日期升序"                                → TestFetchDailyBasic
//   error_handling[0] "code!=0 非 40203 → 含 code 与 msg 的 error"                     → TestBusinessError
//   error_handling[0] "HTTP/网络错误包装为含 api 名的 error"                            → TestNetworkError
//   non_functional[0] "连续两次请求最小间隔 200ms(节流)"                                → TestThrottleMinInterval

func tsServer(t *testing.T, resp string, capture *map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(capture))
		w.Write([]byte(resp))
	}))
}

// yesterdayToNow 返回测试通用的日期区间(start=昨天,end=今天)。
func yesterdayToNow() (time.Time, time.Time) {
	return time.Now().AddDate(0, 0, -1), time.Now()
}

func TestFetchDailyBasic(t *testing.T) {
	var got map[string]any
	resp := `{"code":0,"data":{"fields":["ts_code","trade_date","pe_ttm","pb","ps_ttm"],
	 "items":[["600519.SH","20260731",20.41,6.23,9.80],["600519.SH","20260730",20.58,6.28,9.88]]},"msg":""}`
	srv := tsServer(t, resp, &got)
	defer srv.Close()
	c := NewWithBaseURL("tok", srv.URL)
	pts, err := c.FetchDailyBasic("600519.SH",
		time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)

	assert.Equal(t, "daily_basic", got["api_name"])
	assert.Equal(t, "tok", got["token"])
	assert.Equal(t, "ts_code,trade_date,pe_ttm,pb,ps_ttm", got["fields"])
	params := got["params"].(map[string]any)
	assert.Equal(t, "600519.SH", params["ts_code"])
	assert.Equal(t, "20260730", params["start_date"])
	assert.Equal(t, "20260731", params["end_date"])

	require.Len(t, pts, 2)
	assert.True(t, pts[0].Date.Before(pts[1].Date), "升序")
	assert.Equal(t, time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC), pts[0].Date)
	assert.InDelta(t, 20.58, pts[0].PETTM, 1e-9)
	assert.InDelta(t, 6.28, pts[0].PB, 1e-9)
	assert.InDelta(t, 9.88, pts[0].PSTTM, 1e-9)
	assert.InDelta(t, 20.41, pts[1].PETTM, 1e-9)
}

func TestFetchPriceAPIs(t *testing.T) {
	cases := []struct {
		name    string
		symbol  string
		apiName string
		fetch   func(c *Client, symbol string, start, end time.Time) ([]PricePoint, error)
	}{
		{"index_daily", "000300.SH", "index_daily", (*Client).FetchIndexDaily},
		{"hk_daily", "00700.HK", "hk_daily", (*Client).FetchHKDaily},
		{"daily", "600519.SH", "daily", (*Client).FetchDaily},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got map[string]any
			resp := `{"code":0,"data":{"fields":["ts_code","trade_date","close"],
			 "items":[["X","20260731",4012.5],["X","20260730",3990.0]]},"msg":""}`
			srv := tsServer(t, resp, &got)
			defer srv.Close()
			pts, err := tc.fetch(NewWithBaseURL("tok", srv.URL), tc.symbol,
				time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC))
			require.NoError(t, err)

			assert.Equal(t, tc.apiName, got["api_name"])
			assert.Equal(t, "ts_code,trade_date,close", got["fields"])
			assert.Equal(t, tc.symbol, got["params"].(map[string]any)["ts_code"])

			require.Len(t, pts, 2)
			assert.True(t, pts[0].Date.Before(pts[1].Date), "升序")
			assert.InDelta(t, 3990.0, pts[0].Close, 1e-9)
			assert.InDelta(t, 4012.5, pts[1].Close, 1e-9)
		})
	}
}

func TestErrNoPermission(t *testing.T) {
	var got map[string]any
	srv := tsServer(t, `{"code":40203,"data":null,"msg":"抱歉，您没有接口(income)访问权限"}`, &got)
	defer srv.Close()
	start, end := yesterdayToNow()
	_, err := NewWithBaseURL("tok", srv.URL).FetchDailyBasic("600519.SH", start, end)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNoPermission)
	assert.Contains(t, err.Error(), "daily_basic")
	assert.Contains(t, err.Error(), "没有接口(income)访问权限")
}

func TestBusinessError(t *testing.T) {
	var got map[string]any
	srv := tsServer(t, `{"code":2002,"data":null,"msg":"抽取数据积分不足"}`, &got)
	defer srv.Close()
	start, end := yesterdayToNow()
	_, err := NewWithBaseURL("tok", srv.URL).FetchIndexDaily("000300.SH", start, end)
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrNoPermission)
	assert.Contains(t, err.Error(), "index_daily")
	assert.Contains(t, err.Error(), "2002")
	assert.Contains(t, err.Error(), "抽取数据积分不足")
}

func TestNetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close() // 立即关闭:请求必然连接失败
	start, end := yesterdayToNow()
	_, err := NewWithBaseURL("tok", srv.URL).FetchHKDaily("00700.HK", start, end)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hk_daily")
}

func TestFetchDailyBasicEmpty(t *testing.T) {
	var got map[string]any
	srv := tsServer(t, `{"code":0,"data":{"fields":["ts_code","trade_date","pe_ttm","pb","ps_ttm"],"items":[]},"msg":""}`, &got)
	defer srv.Close()
	start, end := yesterdayToNow()
	pts, err := NewWithBaseURL("tok", srv.URL).FetchDailyBasic("600519.SH", start, end)
	require.NoError(t, err)
	assert.Empty(t, pts)
	assert.NotNil(t, pts, "空结果返回空切片而非 nil")
}

func TestThrottleMinInterval(t *testing.T) {
	var got map[string]any
	resp := `{"code":0,"data":{"fields":["ts_code","trade_date","close"],"items":[]},"msg":""}`
	srv := tsServer(t, resp, &got)
	defer srv.Close()
	c := NewWithBaseURL("tok", srv.URL)
	start, end := yesterdayToNow()

	// t0 取在第一次调用之前:节流基准 lastReq 记于请求发出前(≥t0),故第二次
	// 请求必然发出于 t0+minInterval 之后,断言无时钟竞态。
	t0 := time.Now()
	_, err := c.FetchDaily("600519.SH", start, end)
	require.NoError(t, err)
	_, err = c.FetchDaily("600519.SH", start, end)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, time.Since(t0), minInterval, "连续两次请求最小间隔 %v", minInterval)
	assert.Equal(t, 200*time.Millisecond, minInterval)
}
