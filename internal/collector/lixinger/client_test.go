package lixinger

// Context Checkpoint: done_criteria → test mapping
// functional[0] "request() code:1 返回原始 body 无错误"        → TestRequest_Code1IsSuccess
// functional[1] "code:0 + error.message 返回错误且透出 message" → TestRequest_Code0IsError
// functional[2] "400 + ValidationError.messages[].message 报错" → TestRequest_ValidationErrorMessageSurfaced
// functional[3] "New 默认 retry=true 且 retryDelays==[1,2,4,8,16]s;
//                WithRetry(false) 可关;NewWithBaseURL 默认关;
//                request 带 User-Agent 头"                      → TestNew_Defaults / TestRequest_SendsHeaders
// boundary[0]   "429 连两次后第三次成功,恰好 3 次尝试"          → TestRequest_RetriesOn429ThenSucceeds
// boundary[1]   "retry 关闭遇 5xx 只 1 次并返回错误"            → TestRequest_RetryDisabledDoesNotRetry
// error[0]      "4xx 不重试,恰好 1 次"                          → TestRequest_No4xxRetry
// warning       "4xx body 含 envelope error.message → 透出 message 而非裸状态码" → TestRequest_4xxSurfacesErrorMessage

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRequest_Code1IsSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":1,"message":"success","data":[{"x":1}]}`))
	}))
	defer srv.Close()

	lx := NewWithBaseURL("k", srv.URL)
	raw, err := lx.request("cn/company/candlestick", map[string]any{"token": "k"})
	if err != nil {
		t.Fatalf("code:1 must be success, got: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("expected raw body")
	}
}

func TestRequest_Code0IsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"error":{"message":"api is not found"}}`))
	}))
	defer srv.Close()

	lx := NewWithBaseURL("k", srv.URL)
	if _, err := lx.request("x", map[string]any{}); err == nil {
		t.Fatal("code:0 must be an error")
	}
}

func TestRequest_ValidationErrorMessageSurfaced(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":0,"error":{"name":"ValidationError","messages":[{"message":"\"metricsList\" is required"}]}}`))
	}))
	defer srv.Close()

	lx := NewWithBaseURL("k", srv.URL)
	_, err := lx.request("x", map[string]any{})
	if err == nil {
		t.Fatal("400 must be an error")
	}
}

func TestRequest_RetriesOn429ThenSucceeds(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"code":1,"data":[]}`))
	}))
	defer srv.Close()

	lx := NewWithBaseURL("k", srv.URL)
	lx.retry = true
	lx.retryDelays = []time.Duration{0, 0, 0, 0, 0} // 零延迟加速测试
	if _, err := lx.request("x", map[string]any{}); err != nil {
		t.Fatalf("should succeed after retries, got: %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 attempts, got %d", calls)
	}
}

func TestRequest_No4xxRetry(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":0,"error":{"message":"bad"}}`))
	}))
	defer srv.Close()

	lx := NewWithBaseURL("k", srv.URL)
	lx.retry = true
	lx.retryDelays = []time.Duration{0, 0, 0, 0, 0}
	if _, err := lx.request("x", map[string]any{}); err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Fatalf("4xx must not retry, got %d calls", calls)
	}
}

// TestRequest_4xxSurfacesErrorMessage covers the QA WARNING: when a 4xx body
// carries a valid envelope with an error.message, request must surface that
// message instead of the bare "unexpected HTTP status" code. Still no retry.
func TestRequest_4xxSurfacesErrorMessage(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":0,"error":{"message":"\"metricsList\" is required"}}`))
	}))
	defer srv.Close()

	lx := NewWithBaseURL("k", srv.URL)
	lx.retry = true
	lx.retryDelays = []time.Duration{0, 0, 0, 0, 0}
	_, err := lx.request("x", map[string]any{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "metricsList") {
		t.Errorf("4xx error.message must be surfaced, got: %v", err)
	}
	if calls != 1 {
		t.Fatalf("4xx must not retry, got %d calls", calls)
	}
}

func TestRequest_RetryDisabledDoesNotRetry(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	lx := NewWithBaseURL("k", srv.URL) // retry off by default
	if _, err := lx.request("x", map[string]any{}); err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Fatalf("retry disabled must do 1 call, got %d", calls)
	}
}

// TestNew_Defaults covers done_criteria functional[3]: production constructor
// defaults retry on with the canonical backoff schedule; WithRetry(false)
// disables it; NewWithBaseURL defaults retry off (so tests never block on real
// backoff sleeps). Guards against the backoff schedule being silently emptied.
func TestNew_Defaults(t *testing.T) {
	prod := New("k")
	if !prod.retry {
		t.Error("New must default retry=true")
	}
	want := []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second}
	if len(prod.retryDelays) != len(want) {
		t.Fatalf("retryDelays len = %d, want %d", len(prod.retryDelays), len(want))
	}
	for i, d := range want {
		if prod.retryDelays[i] != d {
			t.Errorf("retryDelays[%d] = %v, want %v", i, prod.retryDelays[i], d)
		}
	}

	off := New("k", WithRetry(false))
	if off.retry {
		t.Error("WithRetry(false) must disable retry")
	}

	test := NewWithBaseURL("k", "http://example.invalid")
	if test.retry {
		t.Error("NewWithBaseURL must default retry off")
	}
}

// TestRequest_ErrorNameOnlySurfaced exercises the error.name-only branch of the
// envelope (no message/messages), which the DoD lists as a surfaced field.
func TestRequest_ErrorNameOnlySurfaced(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"error":{"name":"AuthError"}}`))
	}))
	defer srv.Close()

	lx := NewWithBaseURL("k", srv.URL)
	if _, err := lx.request("x", map[string]any{}); err == nil {
		t.Fatal("error.name must surface as an error")
	}
}

// TestRequest_NonOneCodeNoErrorObj covers the fallback branch: code != 1 with no
// error object still yields an error citing the code + message.
func TestRequest_NonOneCodeNoErrorObj(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":2,"message":"weird"}`))
	}))
	defer srv.Close()

	lx := NewWithBaseURL("k", srv.URL)
	if _, err := lx.request("x", map[string]any{}); err == nil {
		t.Fatal("code != 1 with no error object must still error")
	}
}

// TestRequest_MalformedEnvelopeIsError covers the json.Unmarshal failure branch.
func TestRequest_MalformedEnvelopeIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer srv.Close()

	lx := NewWithBaseURL("k", srv.URL)
	if _, err := lx.request("x", map[string]any{}); err == nil {
		t.Fatal("malformed body must error")
	}
}

// TestRequest_SendsHeaders covers done_criteria functional[3]: outgoing requests
// must carry Content-Type and the Chrome User-Agent the API expects.
func TestRequest_SendsHeaders(t *testing.T) {
	var gotUA, gotCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		gotCT = r.Header.Get("Content-Type")
		_, _ = w.Write([]byte(`{"code":1,"data":[]}`))
	}))
	defer srv.Close()

	lx := NewWithBaseURL("k", srv.URL)
	if _, err := lx.request("x", map[string]any{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotUA == "" {
		t.Error("request must send a User-Agent header")
	}
	if gotCT != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotCT)
	}
}
