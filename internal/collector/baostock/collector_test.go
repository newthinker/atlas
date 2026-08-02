package baostock

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/newthinker/atlas/internal/collector"
	"github.com/newthinker/atlas/internal/core"
)

// Context Checkpoint: TASK-005 done_criteria → test mapping
//   functional[4] "baostock 客户端登记进 collector registry 作 A 股行情三跳"
//     → TestClientImplementsCollector / TestCollectorFetchHistory / TestCollectorFetchQuote
//
// Register 只收 collector.Collector,故登记的前提是实现全接口;本文件覆盖这一层。

func TestClientImplementsCollector(t *testing.T) {
	var _ collector.Collector = (*Client)(nil) // 编译期约束:接口漂移立刻变红
	c := New("http://127.0.0.1:8181")
	assert.Equal(t, "baostock", c.Name())
	assert.Equal(t, []core.Market{core.MarketCNA}, c.SupportedMarkets())
	assert.NoError(t, c.Init(collector.Config{}), "桥无需鉴权,Init 不应因缺 key 失败")
	assert.NoError(t, c.Start(nil))
	assert.NoError(t, c.Stop())
}

func TestCollectorFetchHistory(t *testing.T) {
	srv, _ := bridgeServer(t, http.StatusOK, `[{"date":"2026-07-30","close":1340.0},{"date":"2026-07-31","close":1350.6}]`)

	bars, err := New(srv.URL).FetchHistory("600519.SH",
		time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC), "1d")
	require.NoError(t, err)
	require.Len(t, bars, 2)
	assert.Equal(t, "600519.SH", bars[0].Symbol)
	assert.Equal(t, "1d", bars[0].Interval)
	assert.InDelta(t, 1340.0, bars[0].Close, 1e-9)
	assert.InDelta(t, 1350.6, bars[1].Close, 1e-9)
}

// 桥只有日线;非日线粒度静默按日线返回会让调用方拿到与请求不符的数据。
func TestCollectorFetchHistoryRejectsNonDaily(t *testing.T) {
	_, err := New("http://127.0.0.1:8181").FetchHistory("600519.SH",
		time.Now().AddDate(0, 0, -1), time.Now(), "5m")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "5m")
}

func TestCollectorFetchQuote(t *testing.T) {
	srv, _ := bridgeServer(t, http.StatusOK, `[{"date":"2026-07-30","close":100.0},{"date":"2026-07-31","close":105.0}]`)

	q, err := New(srv.URL).FetchQuote("600519.SH")
	require.NoError(t, err)
	require.NotNil(t, q)
	assert.Equal(t, "600519.SH", q.Symbol)
	assert.InDelta(t, 105.0, q.Price, 1e-9)
	assert.InDelta(t, 100.0, q.PrevClose, 1e-9)
	assert.InDelta(t, 5.0, q.Change, 1e-9)
	assert.InDelta(t, 5.0, q.ChangePercent, 1e-9)
}

func TestCollectorFetchQuoteEmpty(t *testing.T) {
	srv, _ := bridgeServer(t, http.StatusOK, `[]`)

	_, err := New(srv.URL).FetchQuote("600519.SH")
	require.Error(t, err, "无数据须报错,不得返回零值报价")
}

// 桥不可达(TASK-003 实测:上游 10030 不通时桥转 500)时错误须透传,
// 让上层能据此判定该跳失败。
func TestCollectorFetchHistoryPropagatesBridgeError(t *testing.T) {
	srv, _ := bridgeServer(t, http.StatusInternalServerError, "baostock login failed")

	_, err := New(srv.URL).FetchHistory("600519.SH",
		time.Now().AddDate(0, 0, -1), time.Now(), "1d")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}
