# Lixinger Collector 修复重写 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 `internal/collector/lixinger` 的 7 个方法按理杏仁真实 API 修正确（统一响应信封、正确端点/参数/字段、退避重试 + 配置开关），并补齐 httptest 测试。

**Architecture:** 新建统一传输层 `client.go`，集中处理请求头、`code:1=成功` 信封语义和退避重试；各业务方法按域拆到 `stock.go`/`fundamental.go`/`fund.go`，`valuation.go` 原地修复；通过函数式选项 `WithRetry` 接入配置开关。

**Tech Stack:** Go, `net/http`, `net/http/httptest`，标准库 `encoding/json`。

设计依据：`docs/superpowers/specs/2026-06-13-lixinger-collector-rewrite-design.md`。

真实 API 关键事实（已实测）：
- 成功响应 `{"code":1,"message":"success","data":[...]}`；失败 `{"code":0,"error":{"message":...}}` 或 `{"code":0,"error":{"name":"ValidationError","messages":[{"message":...}]}}`。
- 日期为 RFC3339（`2026-06-10T00:00:00+08:00`）。
- 估值分位指标以**扁平 dotted key** 返回（如 `"pe_ttm.y5.cvpos": 0.0298`、指数 `"pe_ttm.y5.mcw.cvpos": 0.94`）。
- base URL：`https://open.lixinger.com/api`。

---

## File Structure

| 文件 | 责任 |
|---|---|
| `internal/collector/lixinger/client.go`（新） | 传输层：`request()`、请求头、信封校验、退避重试、`Option`/`WithRetry` |
| `internal/collector/lixinger/lixinger.go`（改） | struct（+retry 字段）、`New(opts...)`/`NewWithBaseURL`/`Init`、`toLixingerSymbol`、生命周期；移除旧 `postJSON`/`lixingerResponse`/`lixingerMetric` |
| `internal/collector/lixinger/stock.go`（新） | `FetchHistory`（candlestick）、`FetchQuote`（candlestick 最新近似） |
| `internal/collector/lixinger/fundamental.go`（新） | `FetchFundamental`、`FetchFundamentalHistory`（non_financial） |
| `internal/collector/lixinger/valuation.go`（改） | `FetchValuationPercentile`：改用 `request()`、扁平 key 解析、指数 `.mcw`；移除 `postJSONRaw`/`digFloat` |
| `internal/collector/lixinger/fund.go`（新） | `FetchFundQuote`、`FetchFundHistory`、`fetchFundInfo`（多接口聚合） |
| `internal/collector/lixinger/client_test.go`（新） | 信封 + 重试测试 |
| `internal/collector/lixinger/stock_test.go`（新） | FetchQuote 测试（FetchHistory 已有 `history_test.go`） |
| `internal/collector/lixinger/fundamental_test.go`（新） | FetchFundamental 测试 |
| `internal/collector/lixinger/valuation_test.go`（**整体重写**） | 个股 + 指数分位测试（code:1 + 扁平 key，去掉 `newTestServer` 依赖） |
| `internal/collector/lixinger/lixinger_httptest_test.go`（**删除**） | 旧文件：反转语义（code:0=成功）+ 已废弃端点 + 定义 `newTestServer`，整体删除 |
| `internal/collector/lixinger/fund_test.go`（新） | 基金净值 + 聚合测试 |
| `cmd/atlas/serve.go`（改:107-112） | 读 `Extra["retry"]` 传入 `WithRetry` |
| `configs/config.yaml`/`config.example.yaml`（改） | lixinger 块加 `retry: true` 注释说明 |

**baseline 状态**：当前 package 测试已是 RED——既有 `history_test.go`（针对 candlestick 端点）对旧
`FetchHistory`（`cn/stock/hq`）失败，且 `lixinger_httptest_test.go` 编码了反转语义。本计划**末尾**
（Task 7）达成全绿；中途各任务保证「本任务新增/重写的测试通过」且 package 可编译。

任务顺序说明：Task 2 先重写 valuation 并删除遗留测试文件（因 `valuation_test.go` 依赖
`newTestServer`，必须与删除同任务完成以保持编译自洽）；`history_test.go` 在 Task 3 实现 candlestick
后转绿，在此之前保持 baseline RED。

---

### Task 1: 传输层 client.go（统一信封 + 退避重试 + 选项）

**Files:**
- Create: `internal/collector/lixinger/client.go`
- Create: `internal/collector/lixinger/client_test.go`
- Modify: `internal/collector/lixinger/lixinger.go`（struct 增字段、构造器）

- [ ] **Step 1: 给 Lixinger struct 增加 retry 字段并改造构造器**

修改 `internal/collector/lixinger/lixinger.go`，把 struct 与构造器替换为：

```go
// defaultRetryDelays is the SKILL.md-mandated backoff schedule for 429/5xx.
var defaultRetryDelays = []time.Duration{
	1 * time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second,
}

// Lixinger implements FundamentalCollector for the Lixinger open API.
type Lixinger struct {
	apiKey      string
	baseURL     string
	client      *http.Client
	retry       bool            // 429/5xx 退避重试开关
	retryDelays []time.Duration // 退避调度；测试可置零加速
}

// Option configures a Lixinger collector.
type Option func(*Lixinger)

// WithRetry toggles the SKILL.md 429/5xx backoff retry policy.
func WithRetry(enabled bool) Option { return func(l *Lixinger) { l.retry = enabled } }

// New creates a Lixinger collector against the production API. Retry defaults
// to enabled (SKILL.md); pass WithRetry(false) to disable.
func New(apiKey string, opts ...Option) *Lixinger {
	l := newWithBaseURL(apiKey, defaultBaseURL)
	l.retry = true
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// NewWithBaseURL creates a collector pointed at a custom base URL with retry
// disabled. Used by tests to inject an httptest.Server endpoint.
func NewWithBaseURL(apiKey, baseURL string) *Lixinger {
	return newWithBaseURL(apiKey, baseURL)
}

func newWithBaseURL(apiKey, baseURL string) *Lixinger {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &Lixinger{
		apiKey:      apiKey,
		baseURL:     baseURL,
		client:      &http.Client{Timeout: 30 * time.Second},
		retry:       false,
		retryDelays: defaultRetryDelays,
	}
}
```

删除原 `New`/`NewWithBaseURL` 两个函数定义（被上面替换）。保留 `defaultBaseURL` 常量、`Name`/`SupportedMarkets`/`Init`/`Start`/`Stop`/`HasAPIKey`/`toLixingerSymbol` 不动。

- [ ] **Step 2: 写 client.go 的失败测试先（在 client_test.go）**

创建 `internal/collector/lixinger/client_test.go`：

```go
package lixinger

import (
	"net/http"
	"net/http/httptest"
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
```

- [ ] **Step 3: 运行测试确认失败（编译错误：request 未定义）**

Run: `go test ./internal/collector/lixinger/ -run TestRequest -v`
Expected: 编译失败，`lx.request undefined`。

- [ ] **Step 4: 实现 client.go**

创建 `internal/collector/lixinger/client.go`：

```go
package lixinger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// userAgent mirrors a recent Chrome UA as required by the Lixinger skill docs.
const userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) " +
	"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/137.0.0.0 Safari/537.36"

// envelope is the common Lixinger response wrapper. Success is code==1; any
// other code (notably 0) is a business error carrying an `error` object.
type envelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Error   *struct {
		Name     string `json:"name"`
		Message  string `json:"message"`
		Messages []struct {
			Message string `json:"message"`
		} `json:"messages"`
	} `json:"error"`
}

// request POSTs payload as JSON to baseURL/endpoint and returns the raw body
// after validating the Lixinger envelope (code==1). It applies the SKILL.md
// backoff retry policy for 429/5xx when l.retry is enabled; 4xx never retries.
func (l *Lixinger) request(endpoint string, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", l.baseURL, endpoint)

	maxAttempts := 1
	if l.retry {
		maxAttempts = len(l.retryDelays) + 1
	}

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		raw, status, derr := l.doOnce(url, body)
		if derr != nil { // 传输层错误：可重试
			lastErr = derr
		} else if status == http.StatusTooManyRequests || status >= 500 {
			lastErr = fmt.Errorf("lixinger: retryable HTTP status %d", status)
		} else if status != http.StatusOK {
			return nil, fmt.Errorf("lixinger: unexpected HTTP status %d", status) // 4xx 不重试
		} else {
			return parseEnvelope(raw)
		}

		if attempt < maxAttempts-1 {
			time.Sleep(l.retryDelays[attempt])
			continue
		}
	}
	return nil, lastErr
}

func (l *Lixinger) doOnce(url string, body []byte) (raw []byte, status int, err error) {
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := l.client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("lixinger: request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("lixinger: read body: %w", err)
	}
	return raw, resp.StatusCode, nil
}

// parseEnvelope validates the Lixinger envelope and returns the raw body on
// success (code==1) so callers can parse the data array themselves.
func parseEnvelope(raw []byte) ([]byte, error) {
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("lixinger: decode envelope: %w", err)
	}
	if env.Code == 1 {
		return raw, nil
	}
	if env.Error != nil {
		if env.Error.Message != "" {
			return nil, fmt.Errorf("lixinger: API error: %s", env.Error.Message)
		}
		if len(env.Error.Messages) > 0 {
			return nil, fmt.Errorf("lixinger: API error: %s", env.Error.Messages[0].Message)
		}
		if env.Error.Name != "" {
			return nil, fmt.Errorf("lixinger: API error: %s", env.Error.Name)
		}
	}
	return nil, fmt.Errorf("lixinger: API error code %d: %s", env.Code, env.Message)
}
```

- [ ] **Step 5: 运行测试确认通过**

Run: `go test ./internal/collector/lixinger/ -run TestRequest -v`
Expected: 6 个用例全 PASS。

- [ ] **Step 6: 提交**

```bash
git add internal/collector/lixinger/client.go internal/collector/lixinger/client_test.go internal/collector/lixinger/lixinger.go
git commit -m "feat(lixinger): unified transport layer with code:1 envelope and backoff retry"
```

---

### Task 2: valuation.go — request()、扁平 key、指数 .mcw（并删除遗留测试文件）

**Files:**
- Delete: `internal/collector/lixinger/lixinger_httptest_test.go`
- Modify: `internal/collector/lixinger/valuation.go`
- Rewrite: `internal/collector/lixinger/valuation_test.go`

> 本任务先删除遗留 httptest 文件（它定义 `newTestServer` 且编码反转语义），并把
> `valuation_test.go` 重写为自包含（不依赖 `newTestServer`），从而保持编译自洽。删除遗留文件
> 会使旧 quote/history/fundamental/fund 方法暂时无测试覆盖——它们将在 Task 3-5 被重写并补测。

- [ ] **Step 1: 删除遗留 httptest 测试文件**

Run: `rm internal/collector/lixinger/lixinger_httptest_test.go`

- [ ] **Step 2: 整体重写 valuation_test.go（自包含、code:1、扁平 key）**

把 `internal/collector/lixinger/valuation_test.go` 整个文件替换为：

```go
package lixinger

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
	if _, err := lx.FetchValuationPercentile("AAPL", 5); err == nil {
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
```

- [ ] **Step 3: 运行测试确认失败**

Run: `go test ./internal/collector/lixinger/ -run TestFetchValuationPercentile -v`
Expected: FAIL（旧实现走 `postJSONRaw` + 嵌套 `digFloat`，且 code 判定反，且指数无 mcw、解析嵌套而非扁平 key）。

- [ ] **Step 4: 重写 valuation.go 的 FetchValuationPercentile，删除 postJSONRaw 与 digFloat**

把 `internal/collector/lixinger/valuation.go` 中的 `FetchValuationPercentile`、`digFloat`、`postJSONRaw` 三个函数整体替换为下面的单个函数（保留 `usHKIndexCodes`、`endpointFor`、`lookbackGranularity` 不变）：

```go
// FetchValuationPercentile returns the PE-TTM historical percentile (0-100) for
// a stock or index via Lixinger's cvpos metric. Index endpoints require the
// market-cap-weighted (.mcw) variant. The metric string doubles as the flat
// response key (e.g. "pe_ttm.y5.cvpos"). Returns (-1, error) for unsupported
// symbols or any failure — callers degrade to "percentile unavailable".
func (l *Lixinger) FetchValuationPercentile(symbol string, lookbackYears int) (float64, error) {
	endpoint, code := endpointFor(symbol)
	if endpoint == "" {
		return -1, fmt.Errorf("lixinger: valuation percentile unsupported for %s", symbol)
	}
	gran := lookbackGranularity(lookbackYears)

	metric := fmt.Sprintf("pe_ttm.%s.cvpos", gran)
	if strings.Contains(endpoint, "/index/") {
		metric = fmt.Sprintf("pe_ttm.%s.mcw.cvpos", gran) // 指数为市值加权
	}

	payload := map[string]any{
		"token":       l.apiKey,
		"date":        "latest",
		"stockCodes":  []string{code},
		"metricsList": []string{metric},
	}
	raw, err := l.request(endpoint, payload)
	if err != nil {
		return -1, err
	}

	var result struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return -1, fmt.Errorf("lixinger: decode valuation response: %w", err)
	}
	if len(result.Data) == 0 {
		return -1, fmt.Errorf("lixinger: no valuation data for %s", symbol)
	}

	v, ok := result.Data[0][metric].(float64) // 扁平 dotted key
	if !ok {
		return -1, fmt.Errorf("lixinger: metric %s missing for %s", metric, symbol)
	}
	return v * 100, nil
}
```

更新 `valuation.go` 的 import 块为（去掉 `bytes`/`io`/`net/http`，新增 `encoding/json`）：

```go
import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/newthinker/atlas/internal/collector"
)
```

- [ ] **Step 5: 运行 valuation 测试确认通过**

Run: `go test ./internal/collector/lixinger/ -run 'TestEndpointFor|TestFetchValuationPercentile' -v`
Expected: 全部 PASS（个股、指数 .mcw、粒度、不支持、业务错误、HTTP 500、空数据、缺指标）。

- [ ] **Step 6: 确认 package 编译（history_test.go 仍 RED 属预期 baseline）**

Run: `go build ./internal/collector/lixinger/`
Expected: 编译成功。`go test` 此时 `TestFetchHistory_RequestShapeAndParse` 仍失败——这是 baseline，Task 3 修复。

- [ ] **Step 7: 提交**

```bash
git add internal/collector/lixinger/valuation.go internal/collector/lixinger/valuation_test.go
git rm internal/collector/lixinger/lixinger_httptest_test.go
git commit -m "fix(lixinger): valuation percentile via request(), flat dotted key, index .mcw; drop legacy httptest"
```

---

### Task 3: stock.go — FetchHistory + FetchQuote（candlestick）

**Files:**
- Create: `internal/collector/lixinger/stock.go`
- Create: `internal/collector/lixinger/stock_test.go`
- Modify: `internal/collector/lixinger/lixinger.go`（删除旧 `FetchQuote`、`FetchHistory`）

注：`internal/collector/lixinger/history_test.go` 已存在且针对 candlestick 端点，本任务实现需让它通过。

- [ ] **Step 1: 删除 lixinger.go 中的旧 FetchQuote 与 FetchHistory**

在 `internal/collector/lixinger/lixinger.go` 中删除：
- 旧 `FetchQuote`（调用 `cn/stock/real-time` 的方法整段）
- 旧 `FetchHistory`（调用 `cn/stock/hq` 的方法整段）

构建在 Step 5 后修正未使用 import。

- [ ] **Step 2: 写 FetchQuote 失败测试（stock_test.go）**

创建 `internal/collector/lixinger/stock_test.go`：

```go
package lixinger

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
```

- [ ] **Step 3: 运行测试确认失败**

Run: `go test ./internal/collector/lixinger/ -run TestFetchQuote -v`
Expected: 编译失败或 FAIL（旧 FetchQuote 已删/未实现新版）。

- [ ] **Step 4: 实现 stock.go**

创建 `internal/collector/lixinger/stock.go`：

```go
package lixinger

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

// candlestickBar is one row of the cn/company/candlestick response. Dates are
// RFC3339 and rows arrive newest-first.
type candlestickBar struct {
	Date   string  `json:"date"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume float64 `json:"volume"`
}

// fetchCandlestick is the shared call behind FetchHistory and FetchQuote.
func (l *Lixinger) fetchCandlestick(symbol string, start, end time.Time) ([]candlestickBar, error) {
	if l.apiKey == "" {
		return nil, fmt.Errorf("lixinger: api_key is required")
	}
	payload := map[string]any{
		"token":     l.apiKey,
		"stockCode": l.toLixingerSymbol(symbol), // 单数，复数会 404
		"type":      "fc_rights",                // 标准前复权，与 eastmoney 一致
		"startDate": start.Format("2006-01-02"),
		"endDate":   end.Format("2006-01-02"),
	}
	raw, err := l.request("cn/company/candlestick", payload)
	if err != nil {
		return nil, err
	}
	var result struct {
		Data []candlestickBar `json:"data"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("lixinger: decode candlestick: %w", err)
	}
	return result.Data, nil
}

// FetchHistory fetches forward-adjusted daily OHLCV from cn/company/candlestick.
func (l *Lixinger) FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	bars, err := l.fetchCandlestick(symbol, start, end)
	if err != nil {
		return nil, err
	}
	data := make([]core.OHLCV, 0, len(bars))
	for _, b := range bars {
		t, err := time.Parse(time.RFC3339, b.Date)
		if err != nil {
			continue
		}
		data = append(data, core.OHLCV{
			Symbol:   symbol,
			Interval: interval,
			Open:     b.Open,
			High:     b.High,
			Low:      b.Low,
			Close:    b.Close,
			Volume:   int64(b.Volume),
			Time:     t,
		})
	}
	return data, nil
}

// FetchQuote approximates a quote from the latest candlestick bar. Lixinger has
// no real-time quote API, so this is delayed data (Source "lixinger-delayed").
func (l *Lixinger) FetchQuote(symbol string) (*core.Quote, error) {
	end := time.Now()
	start := end.AddDate(0, 0, -10) // 取最近约 10 天，容纳停牌/周末
	bars, err := l.fetchCandlestick(symbol, start, end)
	if err != nil {
		return nil, err
	}
	if len(bars) == 0 {
		return nil, fmt.Errorf("lixinger: no candlestick data for %s", symbol)
	}
	latest := bars[0] // newest-first
	q := &core.Quote{
		Symbol: symbol,
		Market: core.MarketCNA,
		Price:  latest.Close,
		Open:   latest.Open,
		High:   latest.High,
		Low:    latest.Low,
		Volume: int64(latest.Volume),
		Time:   time.Now(),
		Source: "lixinger-delayed",
	}
	if len(bars) > 1 {
		prev := bars[1].Close
		q.PrevClose = prev
		q.Change = latest.Close - prev
		if prev != 0 {
			q.ChangePercent = (latest.Close - prev) / prev * 100
		}
	}
	return q, nil
}
```

- [ ] **Step 5: 运行测试确认通过（含已有的 history_test.go）+ 编译**

Run: `go build ./internal/collector/lixinger/ && go test ./internal/collector/lixinger/ -run 'TestFetchQuote|TestFetchHistory' -v`
Expected: 编译成功；`TestFetchQuote_*` 与 `TestFetchHistory_*`（已有 3 个）全 PASS。若 `lixinger.go` 残留 `bytes`/`strings` 等未用 import，删除之。

- [ ] **Step 6: 提交**

```bash
git add internal/collector/lixinger/stock.go internal/collector/lixinger/stock_test.go internal/collector/lixinger/lixinger.go
git commit -m "fix(lixinger): FetchHistory/FetchQuote use cn/company/candlestick"
```

---

### Task 4: fundamental.go — FetchFundamental（non_financial, metricsList）

**Files:**
- Create: `internal/collector/lixinger/fundamental.go`
- Create: `internal/collector/lixinger/fundamental_test.go`
- Modify: `internal/collector/lixinger/lixinger.go`（删旧 `FetchFundamental`/`FetchFundamentalHistory`/`postJSON`/`lixingerResponse`/`lixingerMetric`）

- [ ] **Step 1: 删除 lixinger.go 中旧的 FetchFundamental/FetchFundamentalHistory/postJSON 与相关类型**

在 `internal/collector/lixinger/lixinger.go` 删除：旧 `FetchFundamental`、`FetchFundamentalHistory`、`postJSON`、`lixingerResponse`、`lixingerMetric`。删除后 `bytes`/`net/http` 等若不再被 lixinger.go 使用则一并清理 import。

- [ ] **Step 2: 写 FetchFundamental 失败测试（fundamental_test.go）**

创建 `internal/collector/lixinger/fundamental_test.go`：

```go
package lixinger

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
	if f.DividendYield != 0.0403 {
		t.Errorf("DividendYield = %v, want 0.0403 (dyr)", f.DividendYield)
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
```

- [ ] **Step 3: 运行测试确认失败**

Run: `go test ./internal/collector/lixinger/ -run TestFetchFundamental -v`
Expected: 编译失败或 FAIL（FetchFundamental 已被删除/未实现）。

- [ ] **Step 4: 实现 fundamental.go**

创建 `internal/collector/lixinger/fundamental.go`：

```go
package lixinger

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

// FetchFundamental fetches latest valuation metrics for an A-share stock from
// cn/company/fundamental/non_financial. Note: ROE is not a valid price metric
// on this endpoint, so core.Fundamental.ROE is left zero.
func (l *Lixinger) FetchFundamental(symbol string) (*core.Fundamental, error) {
	if l.apiKey == "" {
		return nil, fmt.Errorf("lixinger: api_key is required")
	}
	payload := map[string]any{
		"token":       l.apiKey,
		"date":        "latest",
		"stockCodes":  []string{l.toLixingerSymbol(symbol)},
		"metricsList": []string{"pe_ttm", "pb", "ps_ttm", "dyr", "mc"},
	}
	raw, err := l.request("cn/company/fundamental/non_financial", payload)
	if err != nil {
		return nil, fmt.Errorf("lixinger: fetch fundamental: %w", err)
	}
	var result struct {
		Data []struct {
			PETTM float64 `json:"pe_ttm"`
			PB    float64 `json:"pb"`
			PSTTM float64 `json:"ps_ttm"`
			DYR   float64 `json:"dyr"`
			MC    float64 `json:"mc"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("lixinger: decode fundamental: %w", err)
	}
	if len(result.Data) == 0 {
		return nil, fmt.Errorf("lixinger: no data for symbol %s", symbol)
	}
	d := result.Data[0]
	return &core.Fundamental{
		Symbol:        symbol,
		Market:        core.MarketCNA,
		Date:          time.Now(),
		PE:            d.PETTM,
		PB:            d.PB,
		PS:            d.PSTTM,
		DividendYield: d.DYR,
		MarketCap:     d.MC,
		Source:        "lixinger",
	}, nil
}

// FetchFundamentalHistory returns the latest fundamental as a single-element
// slice (the endpoint exposes point-in-time valuation, not a true series).
func (l *Lixinger) FetchFundamentalHistory(symbol string, start, end time.Time) ([]core.Fundamental, error) {
	f, err := l.FetchFundamental(symbol)
	if err != nil {
		return nil, err
	}
	return []core.Fundamental{*f}, nil
}
```

- [ ] **Step 5: 运行测试确认通过 + 编译**

Run: `go build ./internal/collector/lixinger/ && go test ./internal/collector/lixinger/ -run TestFetchFundamental -v`
Expected: 编译成功；2 个用例 PASS。若 `lixinger.go` 残留未用 import 报错，删除之。

- [ ] **Step 6: 提交**

```bash
git add internal/collector/lixinger/fundamental.go internal/collector/lixinger/fundamental_test.go internal/collector/lixinger/lixinger.go
git commit -m "fix(lixinger): FetchFundamental uses metricsList with dyr/mc; drop invalid ROE metric"
```

---

### Task 5: fund.go — 基金净值 + 多接口聚合

**Files:**
- Create: `internal/collector/lixinger/fund.go`
- Create: `internal/collector/lixinger/fund_test.go`
- Modify: `internal/collector/lixinger/lixinger.go`（删旧 `FetchFundQuote`/`fetchFundInfo`/`FetchFundInfoPublic`/`FetchFundHistory`）

- [ ] **Step 1: 删除 lixinger.go 中旧的基金方法**

删除：旧 `FetchFundQuote`（调 `cn/fund/nav`）、`FetchFundInfoPublic`、`fetchFundInfo`（调 `cn/fund/fundamental`）、`FetchFundHistory`（调 `cn/fund/nav/history`）。删除后清理 lixinger.go 不再使用的 import。此时 `lixinger.go` 应仅剩常量、struct/构造器（Task 1）、`Name`/`SupportedMarkets`/`Init`/`Start`/`Stop`/`HasAPIKey`/`toLixingerSymbol`。

- [ ] **Step 2: 写基金测试（fund_test.go）**

创建 `internal/collector/lixinger/fund_test.go`：

```go
package lixinger

import (
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
		_, _ = w.Write([]byte(`{"code":1,"message":"success","data":[{"c_name":"招商中证白酒指数","f_c_name":"招商基金管理有限公司","inception_date":"2015-05-27T00:00:00+08:00","op_mode":"契约型开放式"}]}`))
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
	if rows[0].Close != 0.5454 || rows[0].Open != 0.5454 {
		t.Errorf("NAV row OHLC must equal netValue, got %+v", rows[0])
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
	if info.Name != "招商中证白酒指数" {
		t.Errorf("Name = %q", info.Name)
	}
	if info.ManagementCompany != "招商基金管理有限公司" {
		t.Errorf("ManagementCompany = %q", info.ManagementCompany)
	}
	if info.Manager != "侯昊" { // 唯一无 departureDate 的现任
		t.Errorf("Manager = %q, want 侯昊 (active manager)", info.Manager)
	}
	if info.MaxDrawdown != -0.3369 { // 最深回撤
		t.Errorf("MaxDrawdown = %v, want -0.3369", info.MaxDrawdown)
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
		_, _ = w.Write([]byte(`{"code":1,"message":"success","data":[{"c_name":"X","f_c_name":"Y"}]}`))
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
```

- [ ] **Step 3: 运行测试确认失败**

Run: `go test ./internal/collector/lixinger/ -run TestFetchFund -v`
Expected: 编译失败/FAIL（基金方法已删除/未实现）。

- [ ] **Step 4: 实现 fund.go**

创建 `internal/collector/lixinger/fund.go`：

```go
package lixinger

import (
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

type fundNetValue struct {
	Date     string  `json:"date"`
	NetValue float64 `json:"netValue"`
}

// fetchNetValues pulls the unit-NAV series (newest-first) from cn/fund/net-value.
func (l *Lixinger) fetchNetValues(code string, start, end time.Time) ([]fundNetValue, error) {
	payload := map[string]any{
		"token":     l.apiKey,
		"stockCode": code, // 单数
		"startDate": start.Format("2006-01-02"),
		"endDate":   end.Format("2006-01-02"),
	}
	raw, err := l.request("cn/fund/net-value", payload)
	if err != nil {
		return nil, err
	}
	var result struct {
		Data []fundNetValue `json:"data"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("lixinger: decode net-value: %w", err)
	}
	return result.Data, nil
}

// FetchFundHistory fetches historical unit NAV. OHLC are all set to the NAV.
func (l *Lixinger) FetchFundHistory(symbol string, start, end time.Time) ([]core.OHLCV, error) {
	if l.apiKey == "" {
		return nil, fmt.Errorf("lixinger: api_key is required")
	}
	rows, err := l.fetchNetValues(l.toLixingerSymbol(symbol), start, end)
	if err != nil {
		return nil, err
	}
	data := make([]core.OHLCV, 0, len(rows))
	for _, r := range rows {
		t, err := time.Parse(time.RFC3339, r.Date)
		if err != nil {
			continue
		}
		data = append(data, core.OHLCV{
			Symbol:   symbol,
			Interval: "1d",
			Open:     r.NetValue,
			High:     r.NetValue,
			Low:      r.NetValue,
			Close:    r.NetValue,
			Volume:   0,
			Time:     t,
		})
	}
	return data, nil
}

// FetchFundQuote fetches the latest fund NAV plus aggregated metadata.
func (l *Lixinger) FetchFundQuote(symbol string) (*core.Quote, error) {
	if l.apiKey == "" {
		return nil, fmt.Errorf("lixinger: api_key is required")
	}
	code := l.toLixingerSymbol(symbol)
	rows, err := l.fetchNetValues(code, time.Now().AddDate(0, 0, -10), time.Now())
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("lixinger: no fund nav for %s", symbol)
	}
	latest := rows[0]
	q := &core.Quote{
		Symbol:   symbol,
		Market:   core.MarketCNA,
		Price:    latest.NetValue,
		Time:     time.Now(),
		Source:   "lixinger",
		FundInfo: l.fetchFundInfo(code),
	}
	if len(rows) > 1 {
		prev := rows[1].NetValue
		q.PrevClose = prev
		q.Change = latest.NetValue - prev
		if prev != 0 {
			q.ChangePercent = (latest.NetValue - prev) / prev * 100
		}
	}
	return q, nil
}

// FetchFundInfoPublic exposes fetchFundInfo for the eastmoney fallback.
func (l *Lixinger) FetchFundInfoPublic(code string) *core.FundInfo {
	return l.fetchFundInfo(code)
}

// fetchFundInfo aggregates profile + manager + drawdown + latest NAV. Each
// sub-fetch is best-effort: a failure leaves its fields empty, never fatal.
// Returns nil only if the core profile call fails.
func (l *Lixinger) fetchFundInfo(code string) *core.FundInfo {
	info := &core.FundInfo{}

	// profile：名称、公司、成立日、运作方式
	if raw, err := l.request("cn/fund/profile", map[string]any{
		"token": l.apiKey, "stockCodes": []string{code},
	}); err == nil {
		var res struct {
			Data []struct {
				CName         string `json:"c_name"`
				FCName        string `json:"f_c_name"`
				InceptionDate string `json:"inception_date"`
				OpMode        string `json:"op_mode"`
			} `json:"data"`
		}
		if json.Unmarshal(raw, &res) == nil && len(res.Data) > 0 {
			d := res.Data[0]
			info.Name = d.CName
			info.ManagementCompany = d.FCName
			info.FundType = d.OpMode
			if t, err := time.Parse(time.RFC3339, d.InceptionDate); err == nil {
				info.InceptionDate = t
			}
		}
	} else {
		return nil // 核心概况都拿不到则视为无信息
	}

	// manager：现任（无 departureDate）经理
	if raw, err := l.request("cn/fund/manager", map[string]any{
		"token": l.apiKey, "stockCodes": []string{code},
	}); err == nil {
		var res struct {
			Data []struct {
				Managers []struct {
					Name          string `json:"name"`
					DepartureDate string `json:"departureDate"`
				} `json:"managers"`
			} `json:"data"`
		}
		if json.Unmarshal(raw, &res) == nil && len(res.Data) > 0 {
			for _, m := range res.Data[0].Managers {
				if m.DepartureDate == "" { // 在任
					info.Manager = m.Name
					break
				}
			}
		}
	}

	// drawdown：最深回撤（最小 value）
	if raw, err := l.request("cn/fund/drawdown", map[string]any{
		"token": l.apiKey, "stockCode": code, "granularity": "y1",
		"startDate": time.Now().AddDate(-1, 0, 0).Format("2006-01-02"),
		"endDate":   time.Now().Format("2006-01-02"),
	}); err == nil {
		var res struct {
			Data []struct {
				Value float64 `json:"value"`
			} `json:"data"`
		}
		if json.Unmarshal(raw, &res) == nil && len(res.Data) > 0 {
			min := math.Inf(1)
			for _, d := range res.Data {
				if d.Value < min {
					min = d.Value
				}
			}
			if !math.IsInf(min, 1) {
				info.MaxDrawdown = min
			}
		}
	}

	// latest NAV
	if rows, err := l.fetchNetValues(code, time.Now().AddDate(0, 0, -10), time.Now()); err == nil && len(rows) > 0 {
		info.LatestNAV = rows[0].NetValue
		info.NAVDate = rows[0].Date
	}

	return info
}
```

- [ ] **Step 5: 运行测试确认通过**

Run: `go test ./internal/collector/lixinger/ -run TestFetchFund -v`
Expected: 4 个用例 PASS。

- [ ] **Step 6: 确认整个 package 测试全绿**

Run: `go test ./internal/collector/lixinger/ -v`
Expected: 全 PASS（client / valuation / stock / fundamental / fund / history）。

- [ ] **Step 7: 提交**

```bash
git add internal/collector/lixinger/fund.go internal/collector/lixinger/fund_test.go internal/collector/lixinger/lixinger.go
git commit -m "fix(lixinger): fund NAV via net-value + aggregated profile/manager/drawdown info"
```

---

### Task 6: serve.go 接线 + 配置开关

**Files:**
- Modify: `cmd/atlas/serve.go:107-112`
- Modify: `configs/config.yaml`、`configs/config.example.yaml`

- [ ] **Step 1: 修改 serve.go 读取 retry 开关**

把 `cmd/atlas/serve.go` 第 107-112 行（lixinger 构造块）替换为：

```go
	// Create Lixinger collector if configured (used as fallback for Eastmoney).
	// retry 开关默认开启（遵循 SKILL.md 退避策略），可在配置中关闭。
	var lixingerCollector *lixinger.Lixinger
	if collectorCfg, ok := cfg.Collectors["lixinger"]; ok && collectorCfg.Enabled && collectorCfg.APIKey != "" {
		retry := true
		if v, ok := collectorCfg.Extra["retry"].(bool); ok {
			retry = v
		}
		lixingerCollector = lixinger.New(collectorCfg.APIKey, lixinger.WithRetry(retry))
		log.Info("lixinger collector initialized as fallback for eastmoney")
	}
```

- [ ] **Step 2: 在配置文件 lixinger 块加 retry 说明**

修改 `configs/config.yaml` 的 lixinger 块，在 `interval` 后加一行：

```yaml
  lixinger:
    enabled: true
    api_key: "0b0487dc-e619-4ccf-a558-2180eda1450a"
    markets: ["CN_A"]
    interval: "24h"
    retry: true   # 429/5xx 退避重试（1/2/4/8/16s，最多5次）；设 false 关闭
```

同样在 `configs/config.example.yaml` 的 lixinger 块加 `retry: true` 注释行（api_key 保持 `"${LIXINGER_API_KEY}"`）。

- [ ] **Step 3: 确认整个仓库编译 + serve 包测试**

Run: `go build ./... && go test ./cmd/atlas/ ./internal/collector/... 2>&1 | tail -20`
Expected: 编译通过；相关测试 PASS。

- [ ] **Step 4: 提交**

```bash
git add cmd/atlas/serve.go configs/config.yaml configs/config.example.yaml
git commit -m "feat(lixinger): wire retry config switch (default on)"
```

---

### Task 7: 清理 + 全量验证

**Files:**
- Delete: `_probe/`（临时探测工具）

- [ ] **Step 1: 删除临时探测目录**

Run: `rm -rf _probe`

- [ ] **Step 2: 全量构建 + 测试 + vet**

Run: `go build ./... && go vet ./internal/collector/lixinger/ && go test ./... 2>&1 | tail -30`
Expected: 构建通过、vet 无告警、`./internal/collector/lixinger/` 测试全 PASS（其余包仅限与本次无关的既有失败）。

- [ ] **Step 3: 用真实 API 端到端冒烟（可选，需联网）**

> 该步骤需网络与真实 token，CI 可跳过。若手动验证：临时写一个 `main` 调用
> `lixinger.New(key, lixinger.WithRetry(true))` 的 `FetchHistory("600519.SH", start, end, "1d")`、
> `FetchValuationPercentile("600519.SH", 5)`、`FetchValuationPercentile("000300.SH", 5)`、
> `FetchFundQuote("161725")`，确认各返回有效数据后删除该临时文件。

- [ ] **Step 4: 提交清理**

```bash
git add -A
git commit -m "chore(lixinger): remove temporary API probe"
```

---

## 自查覆盖

- 信封 code:1 → Task 1 `parseEnvelope`
- 退避重试 + 4xx 不重试 + 开关 → Task 1 测试 + Task 6 接线
- 删除遗留反转语义测试文件 + valuation 自包含重写 → Task 2
- FetchValuationPercentile 扁平 key/个股/指数 .mcw → Task 2
- FetchHistory candlestick/单数/type/RFC3339 → Task 3 + 已有 `history_test.go`
- FetchQuote candlestick 近似/delayed → Task 3
- FetchFundamental metricsList/dyr/mc/ROE 留零 → Task 4
- 基金 net-value/profile/manager/drawdown 聚合/部分失败降级 → Task 5
- 配置开关接线 → Task 6
- 旧 `postJSON`/`postJSONRaw`/`digFloat`/旧端点全部移除 → Task 2/3/4/5
- `core` 类型不改动（ROE 仅置零） → Task 4
```
