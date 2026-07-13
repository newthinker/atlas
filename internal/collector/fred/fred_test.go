package fred

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Context Checkpoint: done_criteria → test mapping
// functional[0]     "query 参数齐全 + 解析 date/value 为 []Observation" → TestFetchSeriesParsesAndSkipsMissing
// functional[1]     "New 使用默认 baseURL；NewWithBaseURL 注入"          → TestNewUsesDefaultBaseURL (+ 其余用例经 NewWithBaseURL)
// boundary[0]       "value 为 \".\" 的观测被过滤"                        → TestFetchSeriesParsesAndSkipsMissing
// boundary[1]       "start/end 为空串时不携带对应 query 参数"            → TestFetchSeriesOmitsEmptyDateParams
// error_handling[0] "5xx/网络错误指数退避重试至多 3 次，耗尽包装错误"    → TestFetchSeriesRetriesOn5xx / TestFetchSeriesExhaustsRetries
// error_handling[1] "4xx 不重试立即返回；value 不可解析返回错误"          → TestFetchSeriesNoRetryOn4xx / TestFetchSeriesInvalidValue

func TestFetchSeriesParsesAndSkipsMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/series/observations", r.URL.Path)
		q := r.URL.Query()
		assert.Equal(t, "VIXCLS", q.Get("series_id"))
		assert.Equal(t, "test-key", q.Get("api_key"))
		assert.Equal(t, "json", q.Get("file_type"))
		assert.Equal(t, "2026-07-01", q.Get("observation_start"))
		assert.Equal(t, "2026-07-03", q.Get("observation_end"))
		fmt.Fprint(w, `{"observations":[
			{"date":"2026-07-01","value":"15.0"},
			{"date":"2026-07-02","value":"."},
			{"date":"2026-07-03","value":"17.5"}]}`)
	}))
	defer srv.Close()

	c := NewWithBaseURL("test-key", srv.URL)
	obs, err := c.FetchSeries(context.Background(), "VIXCLS", "2026-07-01", "2026-07-03")
	require.NoError(t, err)
	require.Len(t, obs, 2) // "." 缺失值被过滤（FRED 约定）
	assert.Equal(t, Observation{Date: "2026-07-01", Value: 15.0}, obs[0])
	assert.Equal(t, Observation{Date: "2026-07-03", Value: 17.5}, obs[1])
}

func TestFetchSeriesRetriesOn5xx(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		fmt.Fprint(w, `{"observations":[{"date":"2026-07-01","value":"1"}]}`)
	}))
	defer srv.Close()

	c := NewWithBaseURL("k", srv.URL)
	c.backoff = time.Millisecond
	obs, err := c.FetchSeries(context.Background(), "SOFR", "", "")
	require.NoError(t, err)
	assert.Len(t, obs, 1)
	assert.Equal(t, 2, calls)
}

func TestFetchSeriesNoRetryOn4xx(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	c := NewWithBaseURL("k", srv.URL)
	c.backoff = time.Millisecond
	_, err := c.FetchSeries(context.Background(), "SOFR", "", "")
	require.Error(t, err)
	assert.Equal(t, 1, calls) // 4xx 不重试
}

// functional[1]: New 走默认 baseURL（指向真实 FRED 域名，不发请求，仅断言字段）。
func TestNewUsesDefaultBaseURL(t *testing.T) {
	c := New("k")
	assert.Equal(t, defaultBaseURL, c.baseURL)
	assert.Equal(t, "k", c.apiKey)
}

// boundary[1]: start/end 为空串时不携带 observation_start / observation_end。
func TestFetchSeriesOmitsEmptyDateParams(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		assert.False(t, q.Has("observation_start"))
		assert.False(t, q.Has("observation_end"))
		fmt.Fprint(w, `{"observations":[]}`)
	}))
	defer srv.Close()

	c := NewWithBaseURL("k", srv.URL)
	_, err := c.FetchSeries(context.Background(), "SOFR", "", "")
	require.NoError(t, err)
}

// error_handling[0]: 5xx 持续 → 重试耗尽返回包装错误。
func TestFetchSeriesExhaustsRetries(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	c := NewWithBaseURL("k", srv.URL)
	c.backoff = time.Millisecond
	_, err := c.FetchSeries(context.Background(), "SOFR", "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "after retries")
	assert.Equal(t, 3, calls) // 至多 3 次尝试
}

// error_handling[1]: value 不可解析（非 "." 的非法值）→ 返回错误、不重试。
func TestFetchSeriesInvalidValue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"observations":[{"date":"2026-07-01","value":"N/A"}]}`)
	}))
	defer srv.Close()

	c := NewWithBaseURL("k", srv.URL)
	_, err := c.FetchSeries(context.Background(), "SOFR", "", "")
	require.Error(t, err)
}
