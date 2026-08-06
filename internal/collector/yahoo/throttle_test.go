package yahoo

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/collector/policy"
)

// Context Checkpoint: done_criteria → test mapping (设计 §9 M2.1 验收项):
//   functional[0]     "429 → 退避重试成功,恰 2 请求"      → TestDoRetries429ThenSucceeds
//   functional[1]     "500 → 重试成功,恰 2 请求"          → TestDoRetries5xxThenSucceeds
//   functional[2]     "Retry-After: 0 头优先于退避 <1s"    → TestDoHonorsRetryAfterHeader
//   functional[3]     "同域闸门 80ms 相邻间隔 ≥60ms"        → TestDoThrottlesConsecutiveRequests(节流已迁至 policy Gate)
//   error_handling[0] "连续 429 → 恰 4 请求后保留原错误"   → TestDoGivesUpAfterMaxRetries
//
// TASK-002 追加(done_criteria → test mapping):
//   functional     "eps 路径 429 → 重试成功解析 1 点"        → TestFetchEPSHistoryRetries429
//   functional     "retryBudget=1 序列 429/200/429 恰 3 请求" → TestDoRetryBudgetExhausted
//   boundary       "非法 Retry-After → 回退指数退避"          → TestDoRetryAfterInvalidFallsBack
//   error_handling "网络层错误不重试(预算不消耗)"           → TestDoNetworkErrorNoRetry
//
// TASK-001 review_fix(rework 1):
//   boundary       "Retry-After 上界 cap,极大值不阻塞调度"    → TestDoRetryAfterCapped

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
	// 节流已迁到 Gate：经同域闸门注入同样的 80ms，断言与场景不变。
	y.gate = gateWith(policy.Policy{MinInterval: 80 * time.Millisecond})
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

func TestDoRetryAfterCapped(t *testing.T) {
	// 服务端 Retry-After: 1(=1s),但实例 maxRetryAfter 覆盖为 100ms——
	// cap 生效则等待 ~100ms,总耗时 <500ms;若无 cap 会等满 1s。
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n == 0 {
			n++
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(throttleChartBody))
	}))
	t.Cleanup(srv.Close)
	y := NewWithBaseURL(srv.URL)
	y.maxRetryAfter = 100 * time.Millisecond
	start := time.Now()
	if err := fetchOnce(t, y); err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Errorf("Retry-After cap not honored, took %v (waited full 1s?)", elapsed)
	}
}

func TestDoRetryBudgetExhausted(t *testing.T) {
	// 序列:req1=429(消耗唯一预算)→ req2=200(第一次调用成功)
	//       req3=429(预算已空 → 不重试,直接报错)
	y, arrivals := newRetryServer(t, http.StatusTooManyRequests, http.StatusOK, http.StatusTooManyRequests)
	y.retryBudget = 1
	if err := fetchOnce(t, y); err != nil {
		t.Fatalf("first fetch should succeed via retry, got: %v", err)
	}
	err := fetchOnce(t, y)
	if err == nil || !strings.Contains(err.Error(), "unexpected status: 429") {
		t.Fatalf("second fetch should fail fast with budget exhausted, got: %v", err)
	}
	if got := len(arrivals()); got != 3 { // 预算耗尽后绝不发出第 4 个请求
		t.Errorf("expected exactly 3 requests, got %d", got)
	}
}

func TestFetchEPSHistoryRetries429(t *testing.T) {
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n == 0 {
			n++
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		// timeseriesResponse 最小合法体(与 eps_test.go 既有 fixture 同构:
		// 仅解析 timeseries.result[].trailingDilutedEPS[].asOfDate + reportedValue.raw)
		_, _ = w.Write([]byte(`{"timeseries":{"result":[{"trailingDilutedEPS":[{"asOfDate":"2023-11-14","reportedValue":{"raw":6.42}}]}]}}`))
	}))
	t.Cleanup(srv.Close)
	y := NewWithBaseURL(srv.URL)
	y.backoffBase = time.Millisecond
	pts, err := y.FetchEPSHistory("AAPL", time.Unix(1600000000, 0), time.Unix(1700086400, 0))
	if err != nil {
		t.Fatalf("expected eps retry to succeed, got: %v", err)
	}
	if len(pts) != 1 {
		t.Errorf("expected 1 eps point, got %d", len(pts))
	}
}

func TestDoRetryAfterInvalidFallsBack(t *testing.T) {
	// 首响 429 + Retry-After: "garbage"(不可解析)→ 应回退到指数退避
	// (backoffBase<<0),而非当作 0 立即重试。用 backoffBase=200ms 拉开可测间隔:
	// 若忽略非法头走 fallback,两请求间隔 ≈200ms;若误当 0 处理则近乎瞬时。
	var mu sync.Mutex
	var arrivals []time.Time
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		arrivals = append(arrivals, time.Now())
		idx := n
		n++
		mu.Unlock()
		if idx == 0 {
			w.Header().Set("Retry-After", "garbage")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(throttleChartBody))
	}))
	t.Cleanup(srv.Close)
	y := NewWithBaseURL(srv.URL)
	y.backoffBase = 200 * time.Millisecond
	if err := fetchOnce(t, y); err != nil {
		t.Fatalf("expected success after fallback retry, got: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(arrivals) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(arrivals))
	}
	if gap := arrivals[1].Sub(arrivals[0]); gap < 150*time.Millisecond {
		t.Errorf("invalid Retry-After should fall back to backoffBase (~200ms), got gap %v", gap)
	}
}

func TestDoNetworkErrorNoRetry(t *testing.T) {
	// server 提前 Close → 连接失败是网络层错误,do 应直接返回不重试:
	// 以「重试预算未被消耗」锚定「仅 1 次尝试、未走重试路径」。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close() // 立即关闭,后续请求连接失败
	y := NewWithBaseURL(url)
	budgetBefore := y.retryBudget
	if err := fetchOnce(t, y); err == nil {
		t.Fatal("expected network error, got nil")
	}
	if y.retryBudget != budgetBefore {
		t.Errorf("network error must not consume retry budget (no retry): before=%d after=%d", budgetBefore, y.retryBudget)
	}
}
