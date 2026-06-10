package lixinger

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// --- TASK-010: lixinger 可测性重构 + httptest 测试 ---
// done_criteria → test mapping
// functional[0]     "baseURL 注入后默认行为不变，现有 5 测试不改即过" → 现有 lixinger_test.go 全过 + TestLixinger_NewDefaultsBaseURL
// functional[1]     "httptest 下 FetchQuote/FetchHistory 正确解析"       → TestLixinger_FetchQuote_OK / TestLixinger_FetchHistory_OK
// functional[2]     "httptest 下 FetchFundamental(History) 正确解析"     → TestLixinger_FetchFundamental_OK / TestLixinger_FetchFundamentalHistory_OK
// boundary[0]       "空 data 数组返回空结果或明确错误，不 panic"        → TestLixinger_EmptyData_*
// error_handling[0] "HTTP 非 200 返回 error"                              → TestLixinger_HTTPError_*
// error_handling[1] "畸形 JSON 返回 error"                                → TestLixinger_MalformedJSON_*
// error_handling[2] "200 但业务错误码返回 error，不 panic"               → TestLixinger_BusinessError_*

// newTestServer spins up an httptest.Server that always responds with the given
// status code and raw body, and returns a Lixinger pointed at it.
func newTestServer(t *testing.T, status int, body string) (*Lixinger, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	return NewWithBaseURL("test-key", srv.URL), srv.Close
}

func TestLixinger_NewDefaultsBaseURL(t *testing.T) {
	if got := New("k").baseURL; got != defaultBaseURL {
		t.Errorf("New should default baseURL to %q, got %q", defaultBaseURL, got)
	}
	if got := NewWithBaseURL("k", "").baseURL; got != defaultBaseURL {
		t.Errorf("empty baseURL should fall back to default, got %q", got)
	}
}

func TestLixinger_FetchQuote_OK(t *testing.T) {
	body := `{"code":0,"message":"ok","data":[{"stockCode":"600519","close":1800.5,"open":1790,"high":1820,"low":1785,"volume":12345,"preClose":1795,"change":5.5,"pctChange":0.31}]}`
	l, closeFn := newTestServer(t, http.StatusOK, body)
	defer closeFn()

	q, err := l.FetchQuote("600519.SH")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.Symbol != "600519.SH" || q.Price != 1800.5 || q.Open != 1790 || q.PrevClose != 1795 {
		t.Errorf("unexpected quote: %+v", q)
	}
	if q.Volume != 12345 || q.ChangePercent != 0.31 {
		t.Errorf("unexpected volume/pct: %+v", q)
	}
}

func TestLixinger_FetchHistory_OK(t *testing.T) {
	body := `{"code":0,"data":[{"date":"2026-01-02","open":10,"high":12,"low":9,"close":11,"volume":1000},{"date":"2026-01-03","open":11,"high":13,"low":10,"close":12,"volume":2000}]}`
	l, closeFn := newTestServer(t, http.StatusOK, body)
	defer closeFn()

	bars, err := l.FetchHistory("600519.SH", time.Now().AddDate(0, 0, -10), time.Now(), "1d")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(bars) != 2 {
		t.Fatalf("expected 2 bars, got %d", len(bars))
	}
	if bars[0].Close != 11 || bars[0].Interval != "1d" || bars[1].Volume != 2000 {
		t.Errorf("unexpected bars: %+v", bars)
	}
}

func TestLixinger_FetchFundamental_OK(t *testing.T) {
	body := `{"code":0,"data":[{"stockCode":"600519","pe_ttm":30.5,"pb":9.1,"ps_ttm":12.2,"roe_ttm":28.4,"dividend_yield_ratio":1.5,"market_value":2.2e12}]}`
	l, closeFn := newTestServer(t, http.StatusOK, body)
	defer closeFn()

	f, err := l.FetchFundamental("600519")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.PE != 30.5 || f.PB != 9.1 || f.PS != 12.2 || f.ROE != 28.4 {
		t.Errorf("unexpected fundamental ratios: %+v", f)
	}
	if f.DividendYield != 1.5 || f.MarketCap != 2.2e12 {
		t.Errorf("unexpected yield/cap: %+v", f)
	}
}

func TestLixinger_FetchFundamentalHistory_OK(t *testing.T) {
	body := `{"code":0,"data":[{"stockCode":"600519","pe_ttm":30.5,"pb":9.1}]}`
	l, closeFn := newTestServer(t, http.StatusOK, body)
	defer closeFn()

	hist, err := l.FetchFundamentalHistory("600519", time.Now().AddDate(0, 0, -30), time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hist) != 1 || hist[0].PE != 30.5 {
		t.Errorf("unexpected fundamental history: %+v", hist)
	}
}

func TestLixinger_EmptyData_Quote(t *testing.T) {
	l, closeFn := newTestServer(t, http.StatusOK, `{"code":0,"data":[]}`)
	defer closeFn()
	if _, err := l.FetchQuote("600519.SH"); err == nil {
		t.Error("expected error for empty quote data")
	}
}

func TestLixinger_EmptyData_History(t *testing.T) {
	l, closeFn := newTestServer(t, http.StatusOK, `{"code":0,"data":[]}`)
	defer closeFn()
	bars, err := l.FetchHistory("600519.SH", time.Now().AddDate(0, 0, -10), time.Now(), "1d")
	if err != nil {
		t.Fatalf("empty history should return empty slice, not error: %v", err)
	}
	if len(bars) != 0 {
		t.Errorf("expected 0 bars, got %d", len(bars))
	}
}

func TestLixinger_EmptyData_Fundamental(t *testing.T) {
	l, closeFn := newTestServer(t, http.StatusOK, `{"code":0,"data":[]}`)
	defer closeFn()
	if _, err := l.FetchFundamental("600519"); err == nil {
		t.Error("expected error for empty fundamental data")
	}
}

func TestLixinger_HTTPError_Quote(t *testing.T) {
	// Non-200 with a non-JSON body: decode fails downstream → error, no panic.
	l, closeFn := newTestServer(t, http.StatusInternalServerError, "upstream down")
	defer closeFn()
	if _, err := l.FetchQuote("600519.SH"); err == nil {
		t.Error("expected error on HTTP 500")
	}
}

func TestLixinger_HTTPError_Fundamental(t *testing.T) {
	l, closeFn := newTestServer(t, http.StatusBadGateway, "bad gateway")
	defer closeFn()
	if _, err := l.FetchFundamental("600519"); err == nil {
		t.Error("expected error on HTTP 502")
	}
}

func TestLixinger_MalformedJSON_Quote(t *testing.T) {
	l, closeFn := newTestServer(t, http.StatusOK, `{"code":0,"data":[`)
	defer closeFn()
	if _, err := l.FetchQuote("600519.SH"); err == nil {
		t.Error("expected error on malformed JSON")
	}
}

func TestLixinger_MalformedJSON_History(t *testing.T) {
	l, closeFn := newTestServer(t, http.StatusOK, `not json at all`)
	defer closeFn()
	if _, err := l.FetchHistory("600519.SH", time.Now().AddDate(0, 0, -10), time.Now(), "1d"); err == nil {
		t.Error("expected error on malformed JSON")
	}
}

func TestLixinger_BusinessError_Quote(t *testing.T) {
	l, closeFn := newTestServer(t, http.StatusOK, `{"code":1,"message":"token invalid","data":[]}`)
	defer closeFn()
	if _, err := l.FetchQuote("600519.SH"); err == nil {
		t.Error("expected error on non-zero business code")
	}
}

func TestLixinger_BusinessError_Fundamental(t *testing.T) {
	l, closeFn := newTestServer(t, http.StatusOK, `{"code":1,"message":"quota exceeded","data":[]}`)
	defer closeFn()
	if _, err := l.FetchFundamental("600519"); err == nil {
		t.Error("expected error on non-zero business code")
	}
}

func TestLixinger_FetchFundQuote_OK(t *testing.T) {
	// fund/nav returns NAV; fund info call hits the same server (returns same body,
	// which has no fund fields → fundInfo nil), exercising FetchFundQuote parsing.
	body := `{"code":0,"data":[{"fundCode":"110011","nav":3.5,"accNav":4.2,"navChange":0.05,"navPct":1.45,"date":"2026-01-02"}]}`
	l, closeFn := newTestServer(t, http.StatusOK, body)
	defer closeFn()

	q, err := l.FetchFundQuote("110011")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.Price != 3.5 || q.Change != 0.05 || q.ChangePercent != 1.45 {
		t.Errorf("unexpected fund quote: %+v", q)
	}
}

func TestLixinger_FetchFundHistory_OK(t *testing.T) {
	body := `{"code":0,"data":[{"date":"2026-01-02","nav":3.5},{"date":"2026-01-03","nav":3.6}]}`
	l, closeFn := newTestServer(t, http.StatusOK, body)
	defer closeFn()

	bars, err := l.FetchFundHistory("110011", time.Now().AddDate(0, 0, -10), time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(bars) != 2 || bars[0].Close != 3.5 || bars[1].Close != 3.6 {
		t.Errorf("unexpected fund history: %+v", bars)
	}
	// NAV maps to all four OHLC fields.
	if bars[0].Open != bars[0].Close || bars[0].High != bars[0].Low {
		t.Errorf("expected flat OHLC for NAV bar: %+v", bars[0])
	}
}

func TestLixinger_NoAPIKey(t *testing.T) {
	l := NewWithBaseURL("", "http://example.invalid")
	if _, err := l.FetchQuote("600519.SH"); err == nil {
		t.Error("expected error when api key missing")
	}
	if _, err := l.FetchHistory("600519.SH", time.Now(), time.Now(), "1d"); err == nil {
		t.Error("expected error when api key missing")
	}
	if _, err := l.FetchFundQuote("110011"); err == nil {
		t.Error("expected error when api key missing")
	}
	if _, err := l.FetchFundHistory("110011", time.Now(), time.Now()); err == nil {
		t.Error("expected error when api key missing")
	}
}

func TestLixinger_Lifecycle(t *testing.T) {
	l := New("test-key")
	if !l.HasAPIKey() {
		t.Error("HasAPIKey should be true with a key set")
	}
	if New("").HasAPIKey() {
		t.Error("HasAPIKey should be false without a key")
	}
	if err := l.Start(context.Background()); err != nil {
		t.Errorf("Start returned error: %v", err)
	}
	if err := l.Stop(); err != nil {
		t.Errorf("Stop returned error: %v", err)
	}
}

func TestLixinger_FetchFundInfoPublic(t *testing.T) {
	body := `{"code":0,"data":[{"fundCode":"110011","name":"易方达","manager":"张三","management_company":"易方达基金","fund_size":1.2e9,"fund_type":"混合型","annualized_return":12.3,"max_drawdown":-8.1}]}`
	l, closeFn := newTestServer(t, http.StatusOK, body)
	defer closeFn()

	info := l.FetchFundInfoPublic("110011")
	if info == nil {
		t.Fatal("expected non-nil fund info")
	}
	if info.Name != "易方达" || info.Manager != "张三" || info.FundType != "混合型" {
		t.Errorf("unexpected fund info: %+v", info)
	}
}

func TestLixinger_FetchFundInfoPublic_BusinessError(t *testing.T) {
	// Non-zero code → fetchFundInfo returns nil without panicking.
	l, closeFn := newTestServer(t, http.StatusOK, `{"code":1,"message":"err","data":[]}`)
	defer closeFn()
	if info := l.FetchFundInfoPublic("110011"); info != nil {
		t.Errorf("expected nil fund info on business error, got %+v", info)
	}
}
