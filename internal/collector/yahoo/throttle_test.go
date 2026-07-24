package yahoo

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// Context Checkpoint: done_criteria → test mapping (设计 §9 M2.1 验收项):
//   functional[0]     "429 → 退避重试成功,恰 2 请求"      → TestDoRetries429ThenSucceeds
//   functional[1]     "500 → 重试成功,恰 2 请求"          → TestDoRetries5xxThenSucceeds
//   functional[2]     "Retry-After: 0 头优先于退避 <1s"    → TestDoHonorsRetryAfterHeader
//   functional[3]     "minInterval=80ms 相邻间隔 ≥60ms"    → TestDoThrottlesConsecutiveRequests
//   error_handling[0] "连续 429 → 恰 4 请求后保留原错误"   → TestDoGivesUpAfterMaxRetries

// throttleChartBody 是 FetchHistory 可解析的最小合法 chart 响应。
const throttleChartBody = `{"chart":{"result":[{"meta":{"symbol":"AAPL","regularMarketPrice":150,"chartPreviousClose":149,"regularMarketTime":1700000000},"timestamp":[1700000000],"indicators":{"quote":[{"open":[149],"high":[151],"low":[148],"close":[150],"volume":[1000]}]}}],"error":null}}`

// newRetryServer 按 statuses 依次响应(超出后一律 200+合法 body),
// 记录每次请求到达时刻,并返回一个退避极短、节流关闭的 Yahoo(测试专用覆盖)。
func newRetryServer(t *testing.T, statuses ...int) (*Yahoo, func() []time.Time) {
	t.Helper()
	var mu sync.Mutex
	var arrivals []time.Time
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		arrivals = append(arrivals, time.Now())
		idx := n
		n++
		mu.Unlock()
		if idx < len(statuses) && statuses[idx] != http.StatusOK {
			w.WriteHeader(statuses[idx])
			return
		}
		_, _ = w.Write([]byte(throttleChartBody))
	}))
	t.Cleanup(srv.Close)
	y := NewWithBaseURL(srv.URL)
	y.minInterval = 0
	y.backoffBase = time.Millisecond
	return y, func() []time.Time { mu.Lock(); defer mu.Unlock(); return append([]time.Time(nil), arrivals...) }
}

func fetchOnce(t *testing.T, y *Yahoo) error {
	t.Helper()
	_, err := y.FetchHistory("AAPL", time.Unix(1600000000, 0), time.Unix(1700086400, 0), "1d")
	return err
}

func TestDoRetries429ThenSucceeds(t *testing.T) {
	y, arrivals := newRetryServer(t, http.StatusTooManyRequests)
	if err := fetchOnce(t, y); err != nil {
		t.Fatalf("expected retry to succeed, got: %v", err)
	}
	if got := len(arrivals()); got != 2 {
		t.Errorf("expected 2 requests (1 fail + 1 retry), got %d", got)
	}
}

func TestDoRetries5xxThenSucceeds(t *testing.T) {
	y, arrivals := newRetryServer(t, http.StatusInternalServerError)
	if err := fetchOnce(t, y); err != nil {
		t.Fatalf("expected retry to succeed, got: %v", err)
	}
	if got := len(arrivals()); got != 2 {
		t.Errorf("expected 2 requests, got %d", got)
	}
}

func TestDoHonorsRetryAfterHeader(t *testing.T) {
	// 服务端首响 429 + Retry-After: 0;客户端 backoffBase 拉到 2s——
	// 若忽略头走指数退避,测试将耗时 ≥2s;尊重头则近乎立即重试。
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n == 0 {
			n++
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(throttleChartBody))
	}))
	t.Cleanup(srv.Close)
	y := NewWithBaseURL(srv.URL)
	y.minInterval = 0
	y.backoffBase = 2 * time.Second
	start := time.Now()
	if err := fetchOnce(t, y); err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("Retry-After: 0 not honored, took %v (fell back to backoffBase?)", elapsed)
	}
}

func TestDoGivesUpAfterMaxRetries(t *testing.T) {
	y, arrivals := newRetryServer(t,
		http.StatusTooManyRequests, http.StatusTooManyRequests,
		http.StatusTooManyRequests, http.StatusTooManyRequests,
		http.StatusTooManyRequests) // 比 1+maxRetries 多备一档,验证不会第 5 次请求
	err := fetchOnce(t, y)
	if err == nil || !strings.Contains(err.Error(), "unexpected status: 429") {
		t.Fatalf("expected 'unexpected status: 429' after retries exhausted, got: %v", err)
	}
	if got := len(arrivals()); got != 4 { // 1 首发 + 3 重试
		t.Errorf("expected 4 requests (1 + maxRetries), got %d", got)
	}
}

func TestDoThrottlesConsecutiveRequests(t *testing.T) {
	y, arrivals := newRetryServer(t) // 全 200
	y.minInterval = 80 * time.Millisecond
	if err := fetchOnce(t, y); err != nil {
		t.Fatalf("first fetch: %v", err)
	}
	if err := fetchOnce(t, y); err != nil {
		t.Fatalf("second fetch: %v", err)
	}
	ts := arrivals()
	if len(ts) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(ts))
	}
	if gap := ts[1].Sub(ts[0]); gap < 60*time.Millisecond { // 留计时容差
		t.Errorf("expected >=60ms gap between requests, got %v", gap)
	}
}
