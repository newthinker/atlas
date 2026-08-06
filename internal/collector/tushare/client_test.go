package tushare

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/newthinker/atlas/internal/collector/policy"
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
// [review_fix 轮次 1,QA 提出]
//   M5 "默认端点必须 https(token 走 POST body)"                                        → TestDefaultBaseURLIsHTTPS
//   S1 "40203 重载码拆分:msg 含「频率超限」→ 临时性 ErrRateLimited"                     → TestErrRateLimited
//   S1 "非限频的 40203 仍为永久性 ErrNoPermission(functional[2] 被收窄而非推翻)"        → TestErrNoPermissionStillPermanentWhenNotRateLimited

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

// 节流已迁到 policy 策略表;这里断言 tushare 确实**经过**了闸门
// (200ms 的生产取值由 policy 包的 TestLookupBuiltinTopics 守住)。
func TestThrottleViaGate(t *testing.T) {
	var got map[string]any
	resp := `{"code":0,"data":{"fields":["ts_code","trade_date","close"],"items":[]},"msg":""}`
	srv := tsServer(t, resp, &got)
	defer srv.Close()

	tbl := zeroTable()
	tbl.Set("tushare.daily", policy.Policy{Domain: "tushare", MinInterval: 80 * time.Millisecond})
	c := NewWithBaseURL("tok", srv.URL)
	c.gate = policy.New(tbl, nil)
	start, end := yesterdayToNow()

	t0 := time.Now()
	_, err := c.FetchDaily("600519.SH", start, end)
	require.NoError(t, err)
	_, err = c.FetchDaily("000001.SZ", start, end) // 换标的,避免命中缓存键
	require.NoError(t, err)
	assert.GreaterOrEqual(t, time.Since(t0), 60*time.Millisecond, "连续两次请求须被闸门拉开间隔")
}

// M5(QA MAJOR):默认端点必须是 https —— token 走 POST body,明文 http 会让凭证
// 对路径上任意中间节点可见。常量化断言使「顺手改回 http」立刻变红。
func TestDefaultBaseURLIsHTTPS(t *testing.T) {
	assert.True(t, strings.HasPrefix(defaultBaseURL, "https://"),
		"默认端点须为 https,否则 token 明文过网")
	assert.Equal(t, "https://api.tushare.pro", defaultBaseURL)
}

// S1(QA 专项1):40203 是**重载码** —— 限频与无权限同码,只能靠 msg 区分。
// 限频是临时错误(等窗口即可恢复),若沿用永久性的 ErrNoPermission,降级链会把它
// 写成「权限不足,配置性问题」,运维会去查积分档而实际只需等窗口
// (TASK-006 断源演练实锤:600036/000423 撞限频却被报成权限问题)。
func TestErrRateLimited(t *testing.T) {
	var got map[string]any
	srv := tsServer(t, `{"code":40203,"data":null,"msg":"抱歉，您访问接口(daily_basic)频率超限(1次/分钟)，具体频次详情：https://tushare.pro/document/1?doc_id=108。"}`, &got)
	defer srv.Close()
	start, end := yesterdayToNow()
	_, err := NewWithBaseURL("tok", srv.URL).FetchDailyBasic("600519.SH", start, end)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrRateLimited)
	assert.NotErrorIs(t, err, ErrNoPermission, "限频是临时错误,不得判为永久性权限问题")
	assert.Contains(t, err.Error(), "daily_basic")
	assert.Contains(t, err.Error(), "频率超限", "保留原始 msg 便于运维定位")
}

// 限频分支优先级更高,但不得吞掉真正的无权限:两者共存时按 msg 精确分流。
func TestErrNoPermissionStillPermanentWhenNotRateLimited(t *testing.T) {
	var got map[string]any
	srv := tsServer(t, `{"code":40203,"data":null,"msg":"抱歉，您没有接口(index_dailybasic)访问权限"}`, &got)
	defer srv.Close()
	start, end := yesterdayToNow()
	_, err := NewWithBaseURL("tok", srv.URL).FetchIndexDaily("000300.SH", start, end)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNoPermission)
	assert.NotErrorIs(t, err, ErrRateLimited)
}

// S2(review_fix 轮次 2):守住「判据与窗口口径无关」这条性质本身。
//
// 为什么单独立一条:TestErrRateLimited 的 msg 恰好是「1次/分钟」,所以把 rateLimitMarker
// 写死成完整串「频率超限(1次/分钟)」时整套用例仍全绿(test-agent-14 复验的唯一存活突变)。
// 那样一来 S1 的核心性质零守护,而失效方式极隐蔽——要等上游改窗口文案才暴露,届时限频
// 又会被误判成永久错误,退回原问题。
//
// 表里刻意放**不同窗口口径**:实测同一 token 先报「1次/分钟」、约 75 秒后报「1次/小时」,
// 窗口是会变的量。任何把窗口写进判据的改动都会让这里至少一行转红。
func TestErrRateLimitedIsWindowAgnostic(t *testing.T) {
	windows := []string{
		"抱歉，您访问接口(daily_basic)频率超限(1次/分钟)，具体频次详情：https://tushare.pro/document/1?doc_id=108。",
		"抱歉，您访问接口(hk_daily)频率超限(1次/小时)，具体频次详情：https://tushare.pro/document/1?doc_id=108。",
		"抱歉，您访问接口(daily)频率超限(200次/天)。", // 上游若再改窗口口径,判据仍须成立
	}
	for _, msg := range windows {
		t.Run(msg[:min(len(msg), 40)], func(t *testing.T) {
			var got map[string]any
			srv := tsServer(t, `{"code":40203,"data":null,"msg":`+strconv.Quote(msg)+`}`, &got)
			defer srv.Close()
			start, end := yesterdayToNow()
			_, err := NewWithBaseURL("tok", srv.URL).FetchDailyBasic("600519.SH", start, end)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrRateLimited, "判据须只认「频率超限」,与窗口口径无关")
			assert.NotErrorIs(t, err, ErrNoPermission)
		})
	}
}
