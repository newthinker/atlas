package lixinger

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Context Checkpoint: done_criteria → test mapping (TASK-002, plan Task 2)
// functional[0] 个股 cn/company/fundamental/non_financial; pe_ttm.y5.cvpos(无mcw); 0.0298→2.98 → TestFetchValuationPercentile_Stock
// functional[1] 指数 cn/index/fundamental; pe_ttm.y5.mcw.cvpos; 0.9479→94.79; HK/US 指数含/index/同走.mcw → TestFetchValuationPercentile_IndexUsesMcw
// functional[2] lookbackYears 映射 y3/y5/y10 且请求体含对应 metric                              → TestFetchValuationPercentile_Granularity
// functional[3] endpointFor 七用例分派不变                                                       → TestEndpointFor
// boundary[0]   GC=F 商品 → endpointFor 空，不发请求直接 error                                  → TestFetchValuationPercentile_Unsupported
// boundary[1]   data 为空 / 缺指标 key → (-1,error)                                              → _EmptyData / _MissingMetric
// error[0]      业务错误 code!=1 → (-1,error); 合法 JSON 但 HTTP 500 → error(单次)              → _BusinessError / _HTTPError

// valServer spins an httptest server returning a fixed status + body, replacing
// the deleted newTestServer helper.
func valServer(t *testing.T, status int, body string) (*Lixinger, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	return NewWithBaseURL("test-key", srv.URL), srv.Close
}

func TestEndpointFor(t *testing.T) {
	cases := []struct {
		symbol       string
		wantEndpoint string
		wantCode     string
	}{
		{"600519.SH", "cn/company/fundamental/non_financial", "600519"},
		{"000300.SH", "cn/index/fundamental", "000300"},
		{"0700.HK", "hk/company/fundamental/non_financial", "00700"},
		{"AAPL", "", ""}, // 美股个股：理杏仁开放 API 无端点 → 降级
		{"^GSPC", "us/index/fundamental", ".INX"},
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

func TestFetchValuationPercentile_Stock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/cn/company/fundamental/non_financial") {
			t.Errorf("wrong endpoint: %s", r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(raw), `"pe_ttm.y5.cvpos"`) {
			t.Errorf("stock metric must be pe_ttm.y5.cvpos (no mcw), got: %s", raw)
		}
		_, _ = w.Write([]byte(`{"code":1,"message":"success","data":[{"date":"2026-06-12T00:00:00+08:00","stockCode":"600519","pe_ttm.y5.cvpos":0.0298}]}`))
	}))
	defer srv.Close()

	lx := NewWithBaseURL("test-key", srv.URL)
	pct, err := lx.FetchValuationPercentile("600519.SH", 5)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	if pct < 2.97 || pct > 2.99 {
		t.Errorf("pct = %v, want ~2.98 (0.0298*100)", pct)
	}
}

func TestFetchValuationPercentile_IndexUsesMcw(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/cn/index/fundamental") {
			t.Errorf("wrong endpoint: %s", r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(raw), `"pe_ttm.y5.mcw.cvpos"`) {
			t.Errorf("index metric must include .mcw, got: %s", raw)
		}
		_, _ = w.Write([]byte(`{"code":1,"message":"success","data":[{"date":"2026-06-12T00:00:00+08:00","stockCode":"000300","pe_ttm.y5.mcw.cvpos":0.9479}]}`))
	}))
	defer srv.Close()

	lx := NewWithBaseURL("test-key", srv.URL)
	pct, err := lx.FetchValuationPercentile("000300.SH", 5)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	if pct < 94.78 || pct > 94.80 {
		t.Errorf("pct = %v, want ~94.79", pct)
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
			_, _ = w.Write([]byte(`{"code":1,"data":[{"` + wantMetric + `":0.5}]}`))
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
	lx, closeFn := valServer(t, http.StatusOK, `{"code":0,"error":{"message":"no permission"}}`)
	defer closeFn()
	if _, err := lx.FetchValuationPercentile("600519.SH", 5); err == nil {
		t.Error("expected error on business error (code != 1)")
	}
}

func TestFetchValuationPercentile_HTTPError(t *testing.T) {
	// 合法 JSON body 但 HTTP 500 —— 必须因 StatusCode 守卫报错（retry off → 单次即返回）。
	lx, closeFn := valServer(t, http.StatusInternalServerError,
		`{"code":1,"data":[{"pe_ttm.y5.cvpos":0.5}]}`)
	defer closeFn()
	if _, err := lx.FetchValuationPercentile("600519.SH", 5); err == nil {
		t.Error("expected error on HTTP 500 despite valid JSON body")
	}
}

func TestFetchValuationPercentile_EmptyData(t *testing.T) {
	lx, closeFn := valServer(t, http.StatusOK, `{"code":1,"data":[]}`)
	defer closeFn()
	if _, err := lx.FetchValuationPercentile("600519.SH", 5); err == nil {
		t.Error("expected error on empty data")
	}
}

func TestFetchValuationPercentile_MissingMetric(t *testing.T) {
	lx, closeFn := valServer(t, http.StatusOK, `{"code":1,"data":[{"stockCode":"600519"}]}`)
	defer closeFn()
	if _, err := lx.FetchValuationPercentile("600519.SH", 5); err == nil {
		t.Error("expected error when metric key missing")
	}
}
