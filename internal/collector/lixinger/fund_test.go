package lixinger

// Context Checkpoint: done_criteria → test mapping
// functional[0] "FetchFundHistory cn/fund/net-value, netValue→OHLC, RFC3339"
//                                              → TestFetchFundHistory_NetValue
// functional[1] "FetchFundInfoPublic 聚合 e_t_short_name(基金简称)/f_c_name(管理公司)/现任经理/最小回撤*100(percentage)/最新NAV/成立日"
//                                              → TestFetchFundInfoPublic_Aggregates
// functional[2] "FetchFundQuote Price=最新 netValue 且附带 FundInfo"
//                                              → TestFetchFundQuote_LatestNav
// boundary[0]   "profile ok 其余 500 → 仍非 nil 且 profile 字段填充"
//                                              → TestFetchFundInfoPublic_PartialFailureDegrades
// boundary[1]   "空净值 → FetchFundQuote error" → TestFetchFundQuote_EmptyNavIsError
// error_handling "apiKey 为空 → error；profile 失败 → fetchFundInfo nil"
//                → TestFetchFundQuote_EmptyAPIKeyIsError / TestFetchFundHistory_EmptyAPIKeyIsError
//                + TestFetchFundInfoPublic_ProfileFailureIsNil

import (
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fundMux routes the four aggregated fund endpoints to canned real-shaped bodies.
func fundMux() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/cn/fund/net-value", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":1,"message":"success","data":[{"date":"2026-06-10T00:00:00+08:00","netValue":0.5454},{"date":"2026-06-09T00:00:00+08:00","netValue":0.5395}]}`))
	})
	mux.HandleFunc("/cn/fund/profile", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":1,"message":"success","data":[{"c_name":"中国银行股份有限公司","e_t_short_name":"白酒基金LOF","f_c_name":"招商基金管理有限公司","inception_date":"2015-05-27T00:00:00+08:00","op_mode":"契约型开放式"}]}`))
	})
	mux.HandleFunc("/cn/fund/manager", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":1,"message":"success","data":[{"stockCode":"161725","managers":[{"name":"王平","appointmentDate":"2015-05-27T00:00:00+08:00","departureDate":"2016-12-03T00:00:00+08:00"},{"name":"侯昊","appointmentDate":"2017-08-22T00:00:00+08:00"}]}]}`))
	})
	mux.HandleFunc("/cn/fund/drawdown", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":1,"message":"success","data":[{"date":"2026-06-05T00:00:00+08:00","value":-0.3368},{"date":"2026-06-04T00:00:00+08:00","value":-0.3369}]}`))
	})
	return mux
}

func TestFetchFundHistory_NetValue(t *testing.T) {
	srv := httptest.NewServer(fundMux())
	defer srv.Close()

	lx := NewWithBaseURL("k", srv.URL)
	rows, err := lx.FetchFundHistory("161725",
		time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	if rows[0].Close != 0.5454 || rows[0].Open != 0.5454 || rows[0].High != 0.5454 || rows[0].Low != 0.5454 {
		t.Errorf("NAV row OHLC must equal netValue, got %+v", rows[0])
	}
	if rows[0].Time.IsZero() {
		t.Error("RFC3339 date must parse into Time")
	}
}

func TestFetchFundInfoPublic_Aggregates(t *testing.T) {
	srv := httptest.NewServer(fundMux())
	defer srv.Close()

	lx := NewWithBaseURL("k", srv.URL)
	info := lx.FetchFundInfoPublic("161725")
	if info == nil {
		t.Fatal("expected non-nil FundInfo")
	}
	if info.Name != "白酒基金LOF" { // e_t_short_name(基金简称)，非 c_name(托管行)
		t.Errorf("Name = %q, want 白酒基金LOF (e_t_short_name)", info.Name)
	}
	if info.ManagementCompany != "招商基金管理有限公司" {
		t.Errorf("ManagementCompany = %q", info.ManagementCompany)
	}
	if info.Manager != "侯昊" { // 唯一无 departureDate 的现任
		t.Errorf("Manager = %q, want 侯昊 (active manager)", info.Manager)
	}
	if math.Abs(info.MaxDrawdown-(-33.69)) > 1e-6 { // 最深回撤 -0.3369 normalized to percentage -33.69
		t.Errorf("MaxDrawdown = %v, want -33.69 (value*100 percentage)", info.MaxDrawdown)
	}
	if info.LatestNAV != 0.5454 {
		t.Errorf("LatestNAV = %v, want 0.5454", info.LatestNAV)
	}
	if info.InceptionDate.IsZero() {
		t.Error("InceptionDate must be parsed")
	}
}

func TestFetchFundQuote_LatestNav(t *testing.T) {
	srv := httptest.NewServer(fundMux())
	defer srv.Close()

	lx := NewWithBaseURL("k", srv.URL)
	q, err := lx.FetchFundQuote("161725")
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	if q.Price != 0.5454 {
		t.Errorf("Price = %v, want latest netValue 0.5454", q.Price)
	}
	if q.FundInfo == nil || q.FundInfo.Name == "" {
		t.Error("FundInfo should be attached")
	}
}

func TestFetchFundInfoPublic_PartialFailureDegrades(t *testing.T) {
	// profile ok, 其余 500：缺字段留空但不致命，仍返回非 nil。
	mux := http.NewServeMux()
	mux.HandleFunc("/cn/fund/profile", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":1,"message":"success","data":[{"c_name":"托管行","e_t_short_name":"X","f_c_name":"Y"}]}`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/cn/fund/profile") {
			w.WriteHeader(http.StatusInternalServerError)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	lx := NewWithBaseURL("k", srv.URL)
	info := lx.FetchFundInfoPublic("161725")
	if info == nil || info.Name != "X" {
		t.Fatalf("profile should still populate, got %+v", info)
	}
}

// TestFetchFundInfoPublic_ProfileFailureIsNil covers: profile call fails →
// fetchFundInfo must return nil (no core profile = no info).
func TestFetchFundInfoPublic_ProfileFailureIsNil(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	lx := NewWithBaseURL("k", srv.URL)
	if info := lx.FetchFundInfoPublic("161725"); info != nil {
		t.Fatalf("profile failure must yield nil FundInfo, got %+v", info)
	}
}

// TestFetchFundQuote_EmptyNavIsError covers boundary[1]: empty net-value series
// must surface an error from FetchFundQuote.
func TestFetchFundQuote_EmptyNavIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":1,"message":"success","data":[]}`))
	}))
	defer srv.Close()

	lx := NewWithBaseURL("k", srv.URL)
	if _, err := lx.FetchFundQuote("161725"); err == nil {
		t.Fatal("empty nav must be an error")
	}
}

// TestFetchFundQuote_EmptyAPIKeyIsError covers error_handling: empty apiKey must
// short-circuit to an error before any network call.
func TestFetchFundQuote_EmptyAPIKeyIsError(t *testing.T) {
	lx := NewWithBaseURL("", "http://127.0.0.1:0") // invalid baseURL proves no network touch
	_, err := lx.FetchFundQuote("161725")
	if err == nil {
		t.Fatal("empty apiKey must yield an error")
	}
	if !strings.Contains(err.Error(), "api_key is required") {
		t.Errorf("error must mention api_key is required, got: %v", err)
	}
}

// TestFetchFundInfoPublic_EmptyAPIKeyIsNil covers error_handling: an empty apiKey
// must short-circuit fetchFundInfo to nil before any network call (invalid
// baseURL proves no network touch).
func TestFetchFundInfoPublic_EmptyAPIKeyIsNil(t *testing.T) {
	lx := NewWithBaseURL("", "http://127.0.0.1:0")
	if info := lx.FetchFundInfoPublic("x"); info != nil {
		t.Fatalf("empty apiKey must yield nil FundInfo, got %+v", info)
	}
}

// TestFetchFundHistory_EmptyAPIKeyIsError mirrors the guard for FetchFundHistory.
func TestFetchFundHistory_EmptyAPIKeyIsError(t *testing.T) {
	lx := NewWithBaseURL("", "http://127.0.0.1:0")
	_, err := lx.FetchFundHistory("161725", time.Time{}, time.Time{})
	if err == nil {
		t.Fatal("empty apiKey must yield an error")
	}
	if !strings.Contains(err.Error(), "api_key is required") {
		t.Errorf("error must mention api_key is required, got: %v", err)
	}
}
