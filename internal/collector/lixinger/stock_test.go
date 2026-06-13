package lixinger

// Context Checkpoint: done_criteria → test mapping (TASK-003)
// functional[0] FetchQuote 派生 Price/PrevClose/Change/Source     → TestFetchQuote_DerivesFromCandlestick
// functional[1] FetchHistory candlestick/单数/type/RFC3339/OHLCV  → history_test.go: TestFetchHistory_RequestShapeAndParse
// functional[2] FetchHistory 非200 error、空 data 不报错           → history_test.go: _HTTPErrorIsError / _EmptyData
// boundary[0]   FetchQuote 空 candlestick → error                 → TestFetchQuote_EmptyDataIsError
// boundary[1]   单数 stockCode、无 stockCodes                      → history_test.go (已断言)
// error_handling[0] apiKey 为空 → error (fetchCandlestick 守卫)    → 经 fetchCandlestick；history/quote 共用

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// quoteBody mirrors the real cn/company/candlestick response (RFC3339 dates,
// newest-first). FetchQuote derives a delayed quote from the latest two bars.
const quoteBody = `{"code":1,"message":"success","data":[` +
	`{"date":"2026-06-10T00:00:00+08:00","open":1252.08,"close":1275.88,"high":1282,"low":1250.21,"volume":3924400,"stockCode":"600519"},` +
	`{"date":"2026-06-09T00:00:00+08:00","open":1262.99,"close":1256,"high":1263,"low":1252.55,"volume":2786000,"stockCode":"600519"}` +
	`]}`

func TestFetchQuote_DerivesFromCandlestick(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/cn/company/candlestick") {
			t.Errorf("wrong endpoint: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(quoteBody))
	}))
	defer srv.Close()

	lx := NewWithBaseURL("k", srv.URL)
	q, err := lx.FetchQuote("600519.SH")
	if err != nil {
		t.Fatalf("FetchQuote failed: %v", err)
	}
	if q.Price != 1275.88 {
		t.Errorf("Price = %v, want 1275.88 (latest close)", q.Price)
	}
	if q.PrevClose != 1256 {
		t.Errorf("PrevClose = %v, want 1256 (prior close)", q.PrevClose)
	}
	wantChange := 1275.88 - 1256
	if q.Change < wantChange-0.001 || q.Change > wantChange+0.001 {
		t.Errorf("Change = %v, want ~%v", q.Change, wantChange)
	}
	if q.Source != "lixinger-delayed" {
		t.Errorf("Source = %q, want lixinger-delayed", q.Source)
	}
	if q.Symbol != "600519.SH" {
		t.Errorf("Symbol = %q", q.Symbol)
	}
}

func TestFetchQuote_EmptyDataIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":1,"message":"success","data":[]}`))
	}))
	defer srv.Close()

	lx := NewWithBaseURL("k", srv.URL)
	if _, err := lx.FetchQuote("600519.SH"); err == nil {
		t.Fatal("empty candlestick must yield an error for FetchQuote")
	}
}

// error_handling[0]: the apiKey guard in fetchCandlestick must fail fast (and
// not touch the network) for both FetchQuote and FetchHistory.
func TestStock_EmptyAPIKeyIsError(t *testing.T) {
	lx := NewWithBaseURL("", "http://127.0.0.1:0") // 空 key,且不应触网
	if _, err := lx.FetchQuote("600519.SH"); err == nil || !strings.Contains(err.Error(), "api_key is required") {
		t.Errorf("FetchQuote with empty key: want api_key error, got %v", err)
	}
	if _, err := lx.FetchHistory("600519.SH", time.Now().AddDate(0, 0, -5), time.Now(), "1d"); err == nil || !strings.Contains(err.Error(), "api_key is required") {
		t.Errorf("FetchHistory with empty key: want api_key error, got %v", err)
	}
}
