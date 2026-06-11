package yahoo

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Context Checkpoint: done_criteria → test mapping
// functional[2] "FetchEPSHistory 解析 trailingDilutedEPS 升序 + type 参数" → TestFetchEPSHistory
// functional[3] "NewWithBaseURLs 双端点；NewWithBaseURL 兼容"           → TestFetchEPSHistory (NewWithBaseURLs) + 既有测试
// boundary[0]   "空/缺字段返回空 slice + nil；raw<=0 保留"              → TestFetchEPSHistory_EmptyAndIndexSymbol / TestFetchEPSHistory_KeepsNonPositive
// error_handling[0] "指数符号不发请求直接 error"                        → TestFetchEPSHistory_EmptyAndIndexSymbol
// ISSUE-1       "HTTP 非 200 与畸形 JSON 分路径报错"                    → TestFetchEPSHistory_NonOKStatus / TestFetchEPSHistory_MalformedJSON

// functional[2] + functional[3]
func TestFetchEPSHistory(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "type=trailingDilutedEPS") {
			t.Errorf("missing type param: %s", r.URL.RawQuery)
		}
		w.Write([]byte(`{"timeseries":{"result":[{"meta":{"type":["trailingDilutedEPS"]},
			"timestamp":[1672444800,1680307200],
			"trailingDilutedEPS":[
				{"asOfDate":"2022-12-31","reportedValue":{"raw":6.11}},
				{"asOfDate":"2023-03-31","reportedValue":{"raw":5.89}}
			]}]}}`))
	}))
	defer srv.Close()

	y := NewWithBaseURLs(srv.URL, srv.URL)
	pts, err := y.FetchEPSHistory("AAPL",
		time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("FetchEPSHistory: %v", err)
	}
	if len(pts) != 2 || pts[0].EPS != 6.11 || pts[1].Date.Format("2006-01-02") != "2023-03-31" {
		t.Errorf("unexpected points: %+v", pts)
	}
	if pts[0].Date.Format("2006-01-02") != "2022-12-31" {
		t.Errorf("pts[0].Date = %v, want 2022-12-31", pts[0].Date)
	}
}

// boundary[0] + error_handling[0]
func TestFetchEPSHistory_EmptyAndIndexSymbol(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"timeseries":{"result":[{"meta":{}}]}}`))
	}))
	defer srv.Close()
	y := NewWithBaseURLs(srv.URL, srv.URL)
	pts, err := y.FetchEPSHistory("SOMESTOCK", time.Now().AddDate(-5, 0, 0), time.Now())
	if err != nil {
		t.Errorf("empty result should not error, got %v", err)
	}
	if len(pts) != 0 {
		t.Errorf("expected empty result, got %+v", pts)
	}
	// 指数符号不应发起请求，直接报错（设计 §2.5：仅个股）
	if _, err := y.FetchEPSHistory("^GSPC", time.Now().AddDate(-5, 0, 0), time.Now()); err == nil {
		t.Error("expected error for index symbol")
	}
}

// boundary[0]: reportedValue.raw <= 0 的点保留（剔除语义归 valuation 层）
func TestFetchEPSHistory_KeepsNonPositive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"timeseries":{"result":[{"meta":{"type":["trailingDilutedEPS"]},
			"trailingDilutedEPS":[
				{"asOfDate":"2022-12-31","reportedValue":{"raw":-1.5}},
				{"asOfDate":"2023-03-31","reportedValue":{"raw":0}}
			]}]}}`))
	}))
	defer srv.Close()
	y := NewWithBaseURLs(srv.URL, srv.URL)
	pts, err := y.FetchEPSHistory("AAPL", time.Now().AddDate(-2, 0, 0), time.Now())
	if err != nil {
		t.Fatalf("FetchEPSHistory: %v", err)
	}
	if len(pts) != 2 {
		t.Fatalf("non-positive points should be kept, got %d points: %+v", len(pts), pts)
	}
	if pts[0].EPS != -1.5 || pts[1].EPS != 0 {
		t.Errorf("unexpected EPS values: %+v", pts)
	}
}

// ISSUE-1: HTTP 非 200 走 StatusCode 守卫（合法 JSON body + 非 200），
// 与畸形 JSON 测试不同路径。
func TestFetchEPSHistory_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		// 合法 JSON body，确保触发 error 的是 StatusCode 守卫而非 decode 失败
		w.Write([]byte(`{"timeseries":{"result":[{"meta":{"type":["trailingDilutedEPS"]},
			"trailingDilutedEPS":[{"asOfDate":"2022-12-31","reportedValue":{"raw":6.11}}]}]}}`))
	}))
	defer srv.Close()
	y := NewWithBaseURLs(srv.URL, srv.URL)
	if _, err := y.FetchEPSHistory("AAPL", time.Now().AddDate(-2, 0, 0), time.Now()); err == nil {
		t.Error("expected error for 503 status, got nil")
	}
}

// ISSUE-1: 畸形 JSON 走 decode 失败路径（200 + 非法 body），与 StatusCode 守卫区分。
func TestFetchEPSHistory_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{not valid json`))
	}))
	defer srv.Close()
	y := NewWithBaseURLs(srv.URL, srv.URL)
	if _, err := y.FetchEPSHistory("AAPL", time.Now().AddDate(-2, 0, 0), time.Now()); err == nil {
		t.Error("expected error for malformed JSON, got nil")
	}
}
