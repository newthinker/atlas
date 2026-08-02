package baostock

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Context Checkpoint: M3.5a Baostock 桥客户端 done_criteria → test mapping
//   functional[0] "GET /daily 的 [{date,close}] 解析为 []core.OHLCV(Time,Close)" → TestFetchDailyParsesResponse
//   functional[1] "symbol 转换 600519.SH→sh.600519、000423.SZ→sz.000423"        → TestFetchDailySymbolConversion
//   functional[2] bridge.py 行为(verify_by: review)                            → 见 scripts/baostock/bridge.py
//   boundary[0]   "桥响应空数组 → 空切片且 err=nil"                              → TestFetchDailyEmpty
//   error_handling[0] "桥返回 500 → error 含状态与正文"                          → TestFetchDailyServerError
//   non_functional[0] setup.sh 幂等(verify_by: review) / [1] live(verify_by: manual)

// bridgeServer 起一个假桥,固定返回 status/body;第二个返回值取最近一次请求的 URL。
func bridgeServer(t *testing.T, status int, body string) (*httptest.Server, func() *url.URL) {
	t.Helper()
	var last *url.URL
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		last = r.URL
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, func() *url.URL { return last }
}

func TestFetchDailyParsesResponse(t *testing.T) {
	srv, lastURL := bridgeServer(t, http.StatusOK,
		`[{"date":"2026-07-31","close":1350.6},{"date":"2026-08-01","close":1360.25}]`)

	bars, err := New(srv.URL).FetchDaily("600519.SH",
		time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)

	got := lastURL()
	require.NotNil(t, got)
	assert.Equal(t, "/daily", got.Path)
	assert.Equal(t, "sh.600519", got.Query().Get("code"))
	assert.Equal(t, "2026-07-01", got.Query().Get("start"))
	assert.Equal(t, "2026-08-01", got.Query().Get("end"))

	require.Len(t, bars, 2)
	assert.Equal(t, "600519.SH", bars[0].Symbol)
	assert.Equal(t, "1d", bars[0].Interval)
	assert.Equal(t, time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC), bars[0].Time)
	assert.InDelta(t, 1350.6, bars[0].Close, 1e-9)
	assert.Equal(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), bars[1].Time)
	assert.InDelta(t, 1360.25, bars[1].Close, 1e-9)
}

func TestFetchDailySymbolConversion(t *testing.T) {
	cases := []struct{ symbol, wantCode string }{
		{"600519.SH", "sh.600519"},
		{"000423.SZ", "sz.000423"},
		{"sh.600519", "sh.600519"}, // 已是桥形态时原样透传
	}
	for _, tc := range cases {
		t.Run(tc.symbol, func(t *testing.T) {
			srv, lastURL := bridgeServer(t, http.StatusOK, `[]`)
			_, err := New(srv.URL).FetchDaily(tc.symbol, time.Now().AddDate(0, 0, -7), time.Now())
			require.NoError(t, err)
			got := lastURL()
			require.NotNil(t, got)
			assert.Equal(t, tc.wantCode, got.Query().Get("code"))
		})
	}
}

func TestFetchDailyEmpty(t *testing.T) {
	srv, _ := bridgeServer(t, http.StatusOK, `[]`)
	bars, err := New(srv.URL).FetchDaily("600519.SH", time.Now().AddDate(0, 0, -7), time.Now())
	require.NoError(t, err)
	assert.NotNil(t, bars)
	assert.Empty(t, bars)
}

func TestFetchDailyServerError(t *testing.T) {
	srv, _ := bridgeServer(t, http.StatusInternalServerError, "baostock login failed")
	_, err := New(srv.URL).FetchDaily("600519.SH", time.Now().AddDate(0, 0, -7), time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
	assert.Contains(t, err.Error(), "baostock login failed")
}

func TestFetchDailyBadJSON(t *testing.T) {
	srv, _ := bridgeServer(t, http.StatusOK, `not json`)
	_, err := New(srv.URL).FetchDaily("600519.SH", time.Now().AddDate(0, 0, -7), time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode")
}

func TestFetchDailyUnreachableBridge(t *testing.T) {
	srv, _ := bridgeServer(t, http.StatusOK, `[]`)
	base := srv.URL
	srv.Close() // 桥未启动时的连接失败路径
	_, err := New(base).FetchDaily("600519.SH", time.Now().AddDate(0, 0, -7), time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "baostock")
}
