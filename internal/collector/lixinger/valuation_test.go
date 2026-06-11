package lixinger

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Context Checkpoint: done_criteria → test mapping (TASK-005, plan Task 7)
// functional[0] endpointFor 七用例分派                       → TestEndpointFor
// functional[1] 请求体含 pe_ttm.y5.cvpos; cvpos 0.2345→23.45 → TestFetchValuationPercentile
// functional[2] lookbackYears 映射 y3/y5/y10 粒度            → TestFetchValuationPercentile_Granularity
// boundary      GC=F 商品不发请求返回 error                  → TestEndpointFor(GC=F) + TestFetchValuationPercentile_Unsupported
// error[0]      业务码非0 / data 空 / metric 缺失 → (-1,error)→ _BusinessError / _EmptyData / _MissingMetric
// error(ISSUE-1)合法 JSON + 非200 → error（与畸形/业务码分路径）→ TestFetchValuationPercentile_HTTPError

func TestEndpointFor(t *testing.T) {
	cases := []struct {
		symbol       string
		wantEndpoint string
		wantCode     string
	}{
		{"600519.SH", "cn/company/fundamental/non_financial", "600519"},
		{"000300.SH", "cn/index/fundamental", "000300"},
		{"0700.HK", "hk/company/fundamental", "00700"},
		{"AAPL", "us/company/fundamental", "AAPL"},
		{"^GSPC", "us/index/fundamental", "SPX"},
		{"^HSI", "hk/index/fundamental", "HSI"},
		{"GC=F", "", ""},
	}
	for _, c := range cases {
		ep, code := endpointFor(c.symbol)
		if ep != c.wantEndpoint || code != c.wantCode {
			t.Errorf("endpointFor(%q) = (%q,%q), want (%q,%q)",
				c.symbol, ep, code, c.wantEndpoint, c.wantCode)
		}
	}
}

func TestFetchValuationPercentile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "pe_ttm.y5.cvpos") {
			t.Errorf("missing metric in body: %s", body)
		}
		_, _ = w.Write([]byte(`{"code":0,"data":[{"date":"2026-06-10","stockCode":"600519",
			"pe_ttm":{"y5":{"cvpos":0.2345}}}]}`))
	}))
	defer srv.Close()

	lx := NewWithBaseURL("test-key", srv.URL)
	got, err := lx.FetchValuationPercentile("600519.SH", 5)
	if err != nil {
		t.Fatalf("FetchValuationPercentile: %v", err)
	}
	if got < 23.44 || got > 23.46 { // cvpos 0-1 → 0-100
		t.Errorf("percentile = %v, want 23.45", got)
	}
}

func TestFetchValuationPercentile_Granularity(t *testing.T) {
	cases := []struct {
		lookback int
		wantGran string
	}{
		{1, "y3"}, {3, "y3"}, {4, "y5"}, {5, "y5"}, {6, "y10"}, {10, "y10"},
	}
	for _, c := range cases {
		wantMetric := "pe_ttm." + c.wantGran + ".cvpos"
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), wantMetric) {
				t.Errorf("lookback %d: body missing %q: %s", c.lookback, wantMetric, body)
			}
			_, _ = w.Write([]byte(`{"code":0,"data":[{"pe_ttm":{"` + c.wantGran + `":{"cvpos":0.5}}}]}`))
		}))
		lx := NewWithBaseURL("test-key", srv.URL)
		if _, err := lx.FetchValuationPercentile("600519.SH", c.lookback); err != nil {
			t.Errorf("lookback %d: unexpected error: %v", c.lookback, err)
		}
		srv.Close()
	}
}

func TestFetchValuationPercentile_Unsupported(t *testing.T) {
	// 商品符号 → endpointFor 空，不发请求直接 error（指向无效 baseURL 证明未触网）
	lx := NewWithBaseURL("test-key", "http://127.0.0.1:0")
	if _, err := lx.FetchValuationPercentile("GC=F", 5); err == nil {
		t.Error("expected error for commodity symbol")
	}
}

func TestFetchValuationPercentile_BusinessError(t *testing.T) {
	lx, closeFn := newTestServer(t, http.StatusOK, `{"code":403,"message":"no permission"}`)
	defer closeFn()
	if _, err := lx.FetchValuationPercentile("AAPL", 5); err == nil {
		t.Error("expected error on business code 403")
	}
}

func TestFetchValuationPercentile_HTTPError(t *testing.T) {
	// 合法 JSON body 但 HTTP 500 —— 必须因 StatusCode 守卫报错，
	// 与畸形 JSON / 业务码非0 分路径（wisdom ISSUE-1）。
	lx, closeFn := newTestServer(t, http.StatusInternalServerError,
		`{"code":0,"data":[{"pe_ttm":{"y5":{"cvpos":0.5}}}]}`)
	defer closeFn()
	if _, err := lx.FetchValuationPercentile("600519.SH", 5); err == nil {
		t.Error("expected error on HTTP 500 despite valid JSON body")
	}
}

func TestFetchValuationPercentile_EmptyData(t *testing.T) {
	lx, closeFn := newTestServer(t, http.StatusOK, `{"code":0,"data":[]}`)
	defer closeFn()
	if _, err := lx.FetchValuationPercentile("600519.SH", 5); err == nil {
		t.Error("expected error on empty data")
	}
}

func TestFetchValuationPercentile_MissingMetric(t *testing.T) {
	lx, closeFn := newTestServer(t, http.StatusOK, `{"code":0,"data":[{"stockCode":"600519"}]}`)
	defer closeFn()
	if _, err := lx.FetchValuationPercentile("600519.SH", 5); err == nil {
		t.Error("expected error when pe_ttm.cvpos missing")
	}
}
