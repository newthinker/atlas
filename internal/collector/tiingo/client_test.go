package tiingo

// Context Checkpoint: done_criteria → test mapping（TASK-004，client_test.go 部分）
// functional[0]     请求形状 / BRK.B 映射                          → TestFetchHistoryRequestShape / TestFetchHistoryTickerMapping
// functional[1]     端到端折算、200 [] 空切片、非 UTC 时区口径      → TestFetchHistorySplitNormalizedAndClipped / TestFetchHistoryEmptyBody / TestFetchHistoryNonUTCDates
// error_handling[0] 状态码分类 + 前缀 + 脱敏                      → TestFetchHistoryStatusErrorsRedacted
//                   截断前先脱敏（key 跨第 200 字节不泄露前缀）   → TestFetchHistoryStatusErrorKeyAcrossTruncation
// error_handling[1] 传输 / decode / HTTP 错误断链且脱敏            → TestFetchHistoryErrorsRedactedAndUnchained
// non_functional[0] Proxy nil + Timeout 30s                         → TestClientTransportIgnoresProxy
// 其余（Gate 接线、缓存、配额、超时）见 gate_test.go。day() 定义在 normalize_test.go（TASK-009）。

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testKey = "secret-key-123"

// reqLog 记录 handler 看到的最后一次请求。handler 跑在服务端 goroutine，
// 经网络的同步对 race detector 不可见，故必须加锁（否则 -race 误报）。
type reqLog struct {
	mu   sync.Mutex
	url  url.URL
	auth string
	n    int
}

func (l *reqLog) last() (url.URL, string, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.url, l.auth, l.n
}

// serveBody 返回一个对每个请求都回 status + body 的服务器及其请求记录。
func serveBody(t *testing.T, status int, body string) (*httptest.Server, *reqLog) {
	t.Helper()
	log := &reqLog{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.mu.Lock()
		log.url, log.auth, log.n = *r.URL, r.Header.Get("Authorization"), log.n+1
		log.mu.Unlock()
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, log
}

// serveFixture 返回一个回放 NVDA 样本的服务器及其请求记录。
func serveFixture(t *testing.T) (*httptest.Server, *reqLog) {
	t.Helper()
	body, err := os.ReadFile("testdata/nvda_split_sample.json")
	require.NoError(t, err)
	return serveBody(t, http.StatusOK, string(body))
}

func TestFetchHistoryRequestShape(t *testing.T) {
	srv, log := serveFixture(t)
	_, err := NewWithBaseURL(testKey, srv.URL).FetchHistory("NVDA", day("2024-06-03"), day("2024-06-14"))
	require.NoError(t, err)

	u, auth, _ := log.last()
	assert.Equal(t, "/tiingo/daily/NVDA/prices", u.Path)
	assert.Equal(t, url.Values{"startDate": {"2024-06-03"}}, u.Query(), "query 仅 startDate：不带 endDate，end 之后的拆股必须可见")
	assert.NotContains(t, u.RawQuery, testKey, "token 不进 URL")
	assert.Equal(t, "Token "+testKey, auth)
}

func TestFetchHistoryTickerMapping(t *testing.T) {
	srv, log := serveFixture(t)
	_, err := NewWithBaseURL(testKey, srv.URL).FetchHistory("BRK.B", day("2024-06-03"), day("2024-06-14"))
	require.NoError(t, err)
	u, _, _ := log.last()
	assert.Equal(t, "/tiingo/daily/BRK-B/prices", u.Path)
}

func TestFetchHistorySplitNormalizedAndClipped(t *testing.T) {
	srv, _ := serveFixture(t)
	bars, err := NewWithBaseURL(testKey, srv.URL).FetchHistory("NVDA", day("2024-06-05"), day("2024-06-11"))
	require.NoError(t, err)
	require.Len(t, bars, 5, "闭区间 [06-05, 06-11] 共 5 个交易日")
	assert.Equal(t, day("2024-06-05"), bars[0].Time)
	assert.InDelta(t, 122.44, bars[0].Close, 1e-9, "拆股前 ÷10，与 yahoo close 同口径")
	assert.Equal(t, "NVDA", bars[0].Symbol)
}

func TestFetchHistoryEmptyBody(t *testing.T) {
	srv, _ := serveBody(t, http.StatusOK, `[]`)
	bars, err := NewWithBaseURL(testKey, srv.URL).FetchHistory("NVDA", day("2024-06-05"), day("2024-06-11"))
	require.NoError(t, err)
	assert.NotNil(t, bars, "空切片而非 nil")
	assert.Empty(t, bars)
}

// 非 UTC 时区的 start/end：startDate 与截取下界都取该时间**自身时区**的日历日。
// Asia/Shanghai 06-05 07:00 在 UTC 是 06-04 23:00——任一侧改用 UTC 日期就会与另一侧错开一天。
func TestFetchHistoryNonUTCDates(t *testing.T) {
	sh, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	start := time.Date(2024, 6, 5, 7, 0, 0, 0, sh)
	end := time.Date(2024, 6, 11, 7, 0, 0, 0, sh)

	srv, log := serveFixture(t)
	bars, err := NewWithBaseURL(testKey, srv.URL).FetchHistory("NVDA", start, end)
	require.NoError(t, err)

	u, _, _ := log.last()
	assert.Equal(t, "2024-06-05", u.Query().Get("startDate"))
	require.Len(t, bars, 5)
	assert.Equal(t, day("2024-06-05"), bars[0].Time, "截取下界与 startDate 同为 06-05")
	assert.Equal(t, day("2024-06-11"), bars[4].Time)
}

func TestFetchHistoryStatusErrorsRedacted(t *testing.T) {
	cases := []struct {
		status int
		body   string
		want   string
	}{
		{404, `{"detail": "Error: Ticker 'ZZZZ' not found"}`, "not found: Error: Ticker 'ZZZZ' not found"},
		{400, `{"detail": "bad request"}`, "permission/config error (not retryable)"},
		{401, `{"detail": "unauthorized"}`, "permission/config error (not retryable)"},
		{403, `{"detail": "token ` + testKey + ` invalid"}`, "permission/config error (not retryable)"},
		{429, `{"detail": "slow down"}`, "rate limited (retryable)"},
		{500, `oops ` + testKey, "HTTP 500"},
	}
	for _, tc := range cases {
		srv, _ := serveBody(t, tc.status, tc.body)
		_, err := NewWithBaseURL(testKey, srv.URL).FetchHistory("ZZZZ", day("2024-06-01"), day("2024-06-30"))
		require.Error(t, err, "status %d", tc.status)
		assert.Contains(t, err.Error(), tc.want, "status %d", tc.status)
		assert.True(t, strings.HasPrefix(err.Error(), "tiingo: "), "status %d", tc.status)
		assert.NotContains(t, err.Error(), testKey, "status %d：错误文本不得含 token", tc.status)
	}
}

// 返工 R1（test-tg-b）：body 截断到 200 字节若先于脱敏，key 跨第 200 字节时只剩前缀，
// ReplaceAll 匹配不到，前缀原样进入错误文本（会经 prism 报告外发）。故断言任何 ≥4 字符前缀都不出现。
func TestFetchHistoryStatusErrorKeyAcrossTruncation(t *testing.T) {
	srv, _ := serveBody(t, http.StatusInternalServerError, strings.Repeat("x", 190)+testKey)
	_, err := NewWithBaseURL(testKey, srv.URL).FetchHistory("AAPL", day("2024-06-01"), day("2024-06-30"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 500")
	for n := 4; n <= len(testKey); n++ {
		assert.NotContains(t, err.Error(), testKey[:n], "错误文本含 key 的 %d 字符前缀", n)
	}
}

// 空 key 不得做替换：strings.ReplaceAll 的 old 为空时会在每个字符之间插入替换串。
func TestFetchHistoryEmptyKeyNotRedacted(t *testing.T) {
	srv, _ := serveBody(t, http.StatusInternalServerError, "oops")
	_, err := NewWithBaseURL("", srv.URL).FetchHistory("AAPL", day("2024-06-01"), day("2024-06-30"))
	require.Error(t, err)
	assert.Equal(t, "tiingo: AAPL: HTTP 500: oops", err.Error())
}

// 所有出口都必须断链：留链则 errors.Unwrap 可取回未脱敏原文（*url.Error 携带完整 URL）。
func TestFetchHistoryErrorsRedactedAndUnchained(t *testing.T) {
	notJSON, _ := serveBody(t, http.StatusOK, `<html>`+testKey+`</html>`)
	httpErr, _ := serveBody(t, http.StatusBadGateway, `bad gateway `+testKey)
	cases := map[string]string{
		"transport": "http://127.0.0.1:1/" + testKey,
		"decode":    notJSON.URL,
		"http":      httpErr.URL,
	}
	for name, base := range cases {
		_, err := NewWithBaseURL(testKey, base).FetchHistory("AAPL", day("2024-06-01"), day("2024-06-30"))
		require.Error(t, err, name)
		assert.True(t, strings.HasPrefix(err.Error(), "tiingo: "), name)
		assert.NotContains(t, err.Error(), testKey, name)
		assert.Nil(t, errors.Unwrap(err), "%s：必须用 %%v 断链", name)
	}
}

// 进程级 https_proxy 下必须直连（经 7897 代理 TLS 必然失败，设计 §2.1）。
// ⚠ 不能用「设代理环境变量后请求仍到达 httptest」来测：Go 的 ProxyFromEnvironment
// 对 localhost/127.0.0.1 本就不走代理，那种写法有无 Proxy:nil 都会通过。故做结构断言。
func TestClientTransportIgnoresProxy(t *testing.T) {
	c := New(testKey)
	tr, ok := c.hc.Transport.(*http.Transport)
	require.True(t, ok, "Transport 须为 *http.Transport")
	assert.Nil(t, tr.Proxy, "Proxy 必须为 nil：不继承 HTTP(S)_PROXY")
	assert.Equal(t, 30*time.Second, c.hc.Timeout)
}

// QA L4：脱敏须不区分大小写——上游若回显大写形式的 token，区分大小写的替换会漏掉。
func TestFetchHistoryRedactsCaseInsensitively(t *testing.T) {
	srv, _ := serveBody(t, http.StatusInternalServerError, "oops "+strings.ToUpper(testKey))
	_, err := NewWithBaseURL(testKey, srv.URL).FetchHistory("AAPL", day("2024-06-01"), day("2024-06-30"))
	require.Error(t, err)
	assert.NotContains(t, strings.ToLower(err.Error()), testKey, "任何大小写形式的 token 都不得出现在错误里")
	assert.Contains(t, err.Error(), "<redacted>")
}

// QA L5：不跟随重定向。Go 对同主机（哪怕换端口）的重定向会带上 Authorization，
// Tiingo API 本身不需要重定向，故一律不跟随、按 HTTP 3xx 报错。
func TestFetchHistoryDoesNotFollowRedirect(t *testing.T) {
	target, targetLog := serveBody(t, http.StatusOK, "[]")
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+r.URL.RequestURI(), http.StatusFound)
	}))
	t.Cleanup(redirector.Close)

	_, err := NewWithBaseURL(testKey, redirector.URL).FetchHistory("AAPL", day("2024-06-01"), day("2024-06-30"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 302")
	_, _, n := targetLog.last()
	assert.Equal(t, 0, n, "重定向目标不得收到请求（否则 token 随之送出）")
}

// Skeptic S1：200 但响应体是错误对象时，原因不能被 decode 错误吞掉。
func TestFetchHistoryDecodeErrorKeepsDetail(t *testing.T) {
	srv, _ := serveBody(t, http.StatusOK, `{"detail": "Error: startDate is invalid"}`)
	_, err := NewWithBaseURL(testKey, srv.URL).FetchHistory("AAPL", day("2024-06-01"), day("2024-06-30"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Error: startDate is invalid")
}
