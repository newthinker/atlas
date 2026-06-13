package lixinger

// Context Checkpoint: done_criteria → test mapping
// functional[0] "命中 cn/company/fundamental/non_financial，metricsList 且不含 roe_ttm/market_value"
//                                                  → TestFetchFundamental_RequestShapeAndParse
// functional[1] "PE/PB/PS、DividendYield=dyr*100(percentage)、MarketCap=mc、Source=lixinger" → 同上
// functional[2] "ROE 保持 0（non_financial 不提供）"                          → 同上
// boundary[0]   "空 data → error"                  → TestFetchFundamental_EmptyDataIsError
// error_handling[0] "apiKey 为空 → error"          → TestFetchFundamental_EmptyAPIKeyIsError
//                                                  + TestFetchFundamentalHistory_PropagatesError
// (FetchFundamentalHistory 单元素切片契约)         → TestFetchFundamentalHistory_SingleElement

import (
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFetchFundamental_RequestShapeAndParse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/cn/company/fundamental/non_financial") {
			t.Errorf("wrong endpoint: %s", r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		body := string(raw)
		if !strings.Contains(body, "metricsList") {
			t.Errorf("must use metricsList, got: %s", body)
		}
		if strings.Contains(body, "roe_ttm") || strings.Contains(body, "market_value") {
			t.Errorf("must not request invalid price metrics, got: %s", body)
		}
		_, _ = w.Write([]byte(`{"code":1,"message":"success","data":[{"date":"2026-06-12T00:00:00+08:00","dyr":0.0403,"mc":1614992921147.91,"pb":5.9617,"pe_ttm":19.5248,"ps_ttm":9.212,"stockCode":"600519"}]}`))
	}))
	defer srv.Close()

	lx := NewWithBaseURL("k", srv.URL)
	f, err := lx.FetchFundamental("600519")
	if err != nil {
		t.Fatalf("FetchFundamental failed: %v", err)
	}
	if f.PE != 19.5248 || f.PB != 5.9617 || f.PS != 9.212 {
		t.Errorf("PE/PB/PS mismatch: %+v", f)
	}
	if math.Abs(f.DividendYield-4.03) > 1e-6 { // dyr 0.0403 normalized to percentage 4.03
		t.Errorf("DividendYield = %v, want 4.03 (dyr*100 percentage)", f.DividendYield)
	}
	if f.MarketCap != 1614992921147.91 {
		t.Errorf("MarketCap = %v, want mc value", f.MarketCap)
	}
	if f.ROE != 0 {
		t.Errorf("ROE must be 0 (non_financial does not provide it), got %v", f.ROE)
	}
	if f.Source != "lixinger" {
		t.Errorf("Source = %q", f.Source)
	}
}

func TestFetchFundamental_EmptyDataIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":1,"message":"success","data":[]}`))
	}))
	defer srv.Close()

	lx := NewWithBaseURL("k", srv.URL)
	if _, err := lx.FetchFundamental("600519"); err == nil {
		t.Fatal("empty data must be an error")
	}
}

// TestFetchFundamentalHistory_SingleElement verifies the history wrapper
// returns the latest fundamental as a one-element slice.
func TestFetchFundamentalHistory_SingleElement(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":1,"message":"success","data":[{"date":"2026-06-12T00:00:00+08:00","dyr":0.0403,"mc":1614992921147.91,"pb":5.9617,"pe_ttm":19.5248,"ps_ttm":9.212,"stockCode":"600519"}]}`))
	}))
	defer srv.Close()

	lx := NewWithBaseURL("k", srv.URL)
	rows, err := lx.FetchFundamentalHistory("600519", time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("FetchFundamentalHistory failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected single-element slice, got %d", len(rows))
	}
	if rows[0].PE != 19.5248 || rows[0].Source != "lixinger" {
		t.Errorf("unexpected row: %+v", rows[0])
	}
}

// TestFetchFundamentalHistory_PropagatesError ensures fetch failures bubble up.
func TestFetchFundamentalHistory_PropagatesError(t *testing.T) {
	lx := NewWithBaseURL("", "http://127.0.0.1:0")
	if _, err := lx.FetchFundamentalHistory("600519", time.Time{}, time.Time{}); err == nil {
		t.Fatal("expected error from underlying FetchFundamental")
	}
}

// TestFetchFundamental_EmptyAPIKeyIsError covers error_handling[0]: an empty
// apiKey must short-circuit to an error before any network call.
func TestFetchFundamental_EmptyAPIKeyIsError(t *testing.T) {
	lx := NewWithBaseURL("", "http://127.0.0.1:0") // invalid baseURL proves no network touch
	_, err := lx.FetchFundamental("600519")
	if err == nil {
		t.Fatal("empty apiKey must yield an error")
	}
	if !strings.Contains(err.Error(), "api_key is required") {
		t.Errorf("error must mention api_key is required, got: %v", err)
	}
}
