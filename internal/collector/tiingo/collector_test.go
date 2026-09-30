package tiingo

// Context Checkpoint: done_criteria → test mapping（TASK-005）
// functional[0]     编译期断言 / 元数据 / Start·Stop / Init 后新 key 生效 → 下方 var _ / TestCollectorMetadataAndInit
// functional[1]     末两根拼装 Quote                                   → TestCollectorFetchQuoteFromLastTwoBars
// functional[2]     FetchQuote 的 startDate = now − 10 天              → TestCollectorFetchQuoteStartDate
// functional[3]     FetchHistory 1d 透传并返回折算后 bars              → TestCollectorFetchHistoryPassthrough
// boundary[0]       单根 / 零根                                        → TestCollectorFetchQuoteSingleBarAndEmpty
// error_handling[0] 白名单拒绝与非 1d 拒绝不发 HTTP；Init 空 key 报错  → TestCollectorRejectsUnsupportedWithoutHTTP / TestCollectorRejectsNonDaily / TestCollectorMetadataAndInit
// 请求记录与计数复用 client_test.go 的 serveBody / reqLog。

import (
	"context"
	"math"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/newthinker/atlas/internal/collector"
	"github.com/newthinker/atlas/internal/core"
)

var _ collector.Collector = (*Collector)(nil)

// testCollector 指向回放 body 的服务器，now 固定为 2026-09-30。
func testCollector(t *testing.T, body string) (*Collector, *reqLog) {
	t.Helper()
	srv, log := serveBody(t, http.StatusOK, body)
	return &Collector{client: NewWithBaseURL(testKey, srv.URL), now: func() time.Time { return day("2026-09-30") }}, log
}

func requests(log *reqLog) int {
	_, _, n := log.last()
	return n
}

func TestCollectorRejectsUnsupportedWithoutHTTP(t *testing.T) {
	c, log := testCollector(t, "[]")
	for _, sym := range []string{"600036.SH", "0700.HK", "^GSPC", "GC=F", "BTC-USD"} {
		_, err := c.FetchHistory(sym, day("2026-09-01"), day("2026-09-30"), "1d")
		require.Error(t, err, sym)
		assert.Contains(t, err.Error(), "unsupported symbol", sym)
		_, err = c.FetchQuote(sym)
		require.Error(t, err, sym)
	}
	assert.Equal(t, 0, requests(log), "不支持的代码不得发出 HTTP、不占配额")
}

func TestCollectorRejectsNonDaily(t *testing.T) {
	c, log := testCollector(t, "[]")
	_, err := c.FetchHistory("AAPL", day("2026-09-01"), day("2026-09-30"), "1h")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "interval")
	assert.Equal(t, 0, requests(log))
}

func TestCollectorFetchHistoryPassthrough(t *testing.T) {
	srv, log := serveFixture(t)
	c := &Collector{client: NewWithBaseURL(testKey, srv.URL), now: time.Now}
	bars, err := c.FetchHistory("NVDA", day("2024-06-05"), day("2024-06-11"), "1d")
	require.NoError(t, err)
	require.Len(t, bars, 5)
	assert.InDelta(t, 122.44, bars[0].Close, 1e-9, "经 client 折算")
	u, _, n := log.last()
	assert.Equal(t, 1, n)
	assert.Equal(t, "/tiingo/daily/NVDA/prices", u.Path)
	assert.Equal(t, "2024-06-05", u.Query().Get("startDate"))
}

func TestCollectorFetchQuoteFromLastTwoBars(t *testing.T) {
	c, _ := testCollector(t, `[
	  {"date": "2026-09-28T00:00:00.000Z", "open": 99, "high": 101, "low": 98, "close": 100, "volume": 10, "splitFactor": 1},
	  {"date": "2026-09-29T00:00:00.000Z", "open": 100, "high": 106, "low": 99, "close": 105, "volume": 20, "splitFactor": 1}
	]`)
	q, err := c.FetchQuote("AAPL")
	require.NoError(t, err)
	assert.Equal(t, "AAPL", q.Symbol)
	assert.Equal(t, core.MarketUS, q.Market)
	assert.Equal(t, 105.0, q.Price)
	assert.Equal(t, 100.0, q.PrevClose)
	assert.Equal(t, 5.0, q.Change)
	assert.InDelta(t, 5.0, q.ChangePercent, 1e-9, "百分数口径，同 yahoo RegularMarketChangePercent")
	assert.Equal(t, 100.0, q.Open)
	assert.Equal(t, 106.0, q.High)
	assert.Equal(t, 99.0, q.Low)
	assert.Equal(t, int64(20), q.Volume)
	assert.Equal(t, day("2026-09-29"), q.Time, "Time 是 K 线日期：让使用方看出不是实时价")
	assert.Equal(t, "tiingo_eod", q.Source)
}

func TestCollectorFetchQuoteStartDate(t *testing.T) {
	c, log := testCollector(t, `[{"date": "2026-09-29T00:00:00.000Z", "open": 1, "high": 1, "low": 1, "close": 1, "volume": 1, "splitFactor": 1}]`)
	_, err := c.FetchQuote("AAPL")
	require.NoError(t, err)
	u, _, _ := log.last()
	assert.Equal(t, "2026-09-20", u.Query().Get("startDate"), "now(09-30) − 10 天")
}

func TestCollectorFetchQuoteSingleBarAndEmpty(t *testing.T) {
	c, _ := testCollector(t, `[{"date": "2026-09-29T00:00:00.000Z", "open": 1, "high": 1, "low": 1, "close": 105, "volume": 1, "splitFactor": 1}]`)
	q, err := c.FetchQuote("AAPL")
	require.NoError(t, err)
	assert.Equal(t, 105.0, q.Price)
	assert.Equal(t, 0.0, q.PrevClose)
	assert.Equal(t, 0.0, q.Change)
	assert.Equal(t, 0.0, q.ChangePercent)

	c, _ = testCollector(t, `[]`)
	_, err = c.FetchQuote("AAPL")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no recent bars")
}

func TestCollectorMetadataAndInit(t *testing.T) {
	c := NewCollector("k")
	assert.Equal(t, "tiingo", c.Name())
	assert.Equal(t, []core.Market{core.MarketUS}, c.SupportedMarkets())
	assert.NoError(t, c.Start(context.Background()))
	assert.NoError(t, c.Stop())
	assert.Error(t, c.Init(collector.Config{}), "空 key 报错")

	// Init 换 key 后，请求头必须真的带新 key（只断言 Init 返回 nil 测不出「没替换」）。
	srv, log := serveBody(t, http.StatusOK, "[]")
	c.client = NewWithBaseURL("k", srv.URL)
	require.NoError(t, c.Init(collector.Config{APIKey: "k2"}))
	_, err := c.FetchHistory("AAPL", day("2026-09-01"), day("2026-09-30"), "1d")
	require.NoError(t, err)
	_, auth, n := log.last()
	assert.Equal(t, 1, n)
	assert.Equal(t, "Token k2", auth)
}

// QA L11：前一根收盘为 0 时不得算出 ±Inf（会进入 snapshot 的 JSON）。
func TestCollectorFetchQuoteZeroPrevClose(t *testing.T) {
	c, _ := testCollector(t, `[
	  {"date": "2026-09-28T00:00:00.000Z", "open": 0, "high": 0, "low": 0, "close": 0, "volume": 1, "splitFactor": 1},
	  {"date": "2026-09-29T00:00:00.000Z", "open": 1, "high": 1, "low": 1, "close": 105, "volume": 1, "splitFactor": 1}
	]`)
	q, err := c.FetchQuote("AAPL")
	require.NoError(t, err)
	assert.Equal(t, 0.0, q.ChangePercent)
	assert.False(t, math.IsInf(q.ChangePercent, 0) || math.IsNaN(q.ChangePercent))
}

// QA L12：行情的 Symbol 是 atlas 形态（BRK.B），不是请求 Tiingo 用的 BRK-B。
func TestCollectorFetchQuoteKeepsAtlasSymbol(t *testing.T) {
	c, _ := testCollector(t, `[{"date": "2026-09-29T00:00:00.000Z", "open": 1, "high": 1, "low": 1, "close": 105, "volume": 1, "splitFactor": 1}]`)
	q, err := c.FetchQuote("BRK.B")
	require.NoError(t, err)
	assert.Equal(t, "BRK.B", q.Symbol)
}
