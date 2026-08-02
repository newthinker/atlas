package tushare

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/newthinker/atlas/internal/collector"
	"github.com/newthinker/atlas/internal/core"
)

// Context Checkpoint: TASK-005 done_criteria → test mapping
//   functional[4] "tushare(FetchDaily)登记进 collector registry 作 A 股行情二跳"
//     → TestClientImplementsCollector / TestCollectorFetchHistoryRoutesBySymbol
//     / TestCollectorFetchQuote / TestCollectorInitRequiresToken
//
// 登记进 registry 的前提是实现 collector.Collector 全接口(Register 只收该接口),
// 故本文件覆盖的是「能被登记且路由正确」,而非 Prism 编排(编排不消费行情,ADR#6)。

func TestClientImplementsCollector(t *testing.T) {
	var _ collector.Collector = (*Client)(nil) // 编译期约束:接口漂移立刻变红
	c := New("tok")
	assert.Equal(t, "tushare", c.Name())
	assert.ElementsMatch(t, []core.Market{core.MarketCNA, core.MarketHK}, c.SupportedMarkets())
	assert.NoError(t, c.Start(nil))
	assert.NoError(t, c.Stop())
}

func TestCollectorInitRequiresToken(t *testing.T) {
	c := NewWithBaseURL("", "http://x")
	require.Error(t, c.Init(collector.Config{}), "无 token 必须报错,否则会登记一个必然 40203 的采集器")

	require.NoError(t, c.Init(collector.Config{APIKey: "tok"}))
	assert.Equal(t, "tok", c.token, "Init 须接受配置注入的 key")
}

// A 股指数与个股在 tushare 分属不同 api:个股用 daily,指数用 index_daily,港股用
// hk_daily。走错 api 不会报错,只会返回空 items(实测),故按 symbol 形态路由必须锁死。
func TestCollectorFetchHistoryRoutesBySymbol(t *testing.T) {
	cases := []struct {
		symbol  string
		wantAPI string
	}{
		{"600519.SH", "daily"},
		{"000423.SZ", "daily"},
		{"000300.SH", "index_daily"}, // A 股指数(collector.AShareIndexSecIDs 成员)
		{"0700.HK", "hk_daily"},
	}
	for _, tc := range cases {
		t.Run(tc.symbol, func(t *testing.T) {
			var got map[string]any
			resp := `{"code":0,"data":{"fields":["ts_code","trade_date","close"],
			 "items":[["X","20260731",10.5],["X","20260730",10.0]]},"msg":""}`
			srv := tsServer(t, resp, &got)
			defer srv.Close()

			bars, err := NewWithBaseURL("tok", srv.URL).FetchHistory(tc.symbol,
				time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC),
				time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC), "1d")
			require.NoError(t, err)
			assert.Equal(t, tc.wantAPI, got["api_name"])

			require.Len(t, bars, 2)
			assert.Equal(t, tc.symbol, bars[0].Symbol, "Symbol 回填入参形态")
			assert.Equal(t, "1d", bars[0].Interval)
			assert.InDelta(t, 10.0, bars[0].Close, 1e-9, "升序:早的一天在前")
			assert.InDelta(t, 10.5, bars[1].Close, 1e-9)
		})
	}
}

// 非日线粒度直接报错:tushare 这几个 api 只有日线,静默当日线返回会让调用方拿到
// 与请求粒度不符的数据。
func TestCollectorFetchHistoryRejectsNonDaily(t *testing.T) {
	_, err := New("tok").FetchHistory("600519.SH", time.Now().AddDate(0, 0, -1), time.Now(), "5m")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "5m")
}

func TestCollectorFetchQuote(t *testing.T) {
	var got map[string]any
	resp := `{"code":0,"data":{"fields":["ts_code","trade_date","close"],
	 "items":[["X","20260731",21.0],["X","20260730",20.0]]},"msg":""}`
	srv := tsServer(t, resp, &got)
	defer srv.Close()

	q, err := NewWithBaseURL("tok", srv.URL).FetchQuote("600519.SH")
	require.NoError(t, err)
	require.NotNil(t, q)
	assert.Equal(t, "600519.SH", q.Symbol)
	assert.InDelta(t, 21.0, q.Price, 1e-9, "最新一根收盘价")
	assert.InDelta(t, 20.0, q.PrevClose, 1e-9)
	assert.InDelta(t, 1.0, q.Change, 1e-9)
	assert.InDelta(t, 5.0, q.ChangePercent, 1e-9)
}

func TestCollectorFetchQuoteEmpty(t *testing.T) {
	var got map[string]any
	srv := tsServer(t, `{"code":0,"data":{"fields":["ts_code","trade_date","close"],"items":[]},"msg":""}`, &got)
	defer srv.Close()

	_, err := NewWithBaseURL("tok", srv.URL).FetchQuote("600519.SH")
	require.Error(t, err, "无数据须报错,不得返回零值报价")
}

func TestCollectorFetchHistoryPropagatesError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()
	_, err := NewWithBaseURL("tok", srv.URL).FetchHistory("600519.SH",
		time.Now().AddDate(0, 0, -1), time.Now(), "1d")
	require.Error(t, err)
}
