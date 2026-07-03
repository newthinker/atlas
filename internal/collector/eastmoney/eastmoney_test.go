package eastmoney

// Context Checkpoint: done_criteria → test mapping
// functional[0] "默认构造行为不变, 现有 5 测试不改即过"        → 现有 TestEastmoney_* (未修改) + TestNewWithBaseURLs_DefaultsUnchanged
// functional[1] "httptest 下 FetchQuote 解析股票行情为 Quote" → TestFetchQuote_Stock
// functional[2] "httptest 下 FetchHistory 解析 K 线为 OHLCV" → TestFetchHistory_Stock
// boundary[0]   "空数据/空列表返回空结果或明确错误不 panic"   → TestFetchHistory_EmptyKlines / TestFetchQuote_NullData
// error[0]      "HTTP 非 200 返回 error"                     → TestFetchQuote_HTTPError / TestFetchHistory_HTTPError
// error[1]      "畸形 JSON 返回 error"                       → TestFetchQuote_MalformedJSON
// error[2]      "200 但业务错误(data null) 返回 error 不 panic" → TestFetchQuote_NullData
// non_func[0]   "包覆盖率 ≥ 80%"                             → 全部 + TestLifecycleNoop / TestFetchQuote_Fund / TestFetchHistory_Fund

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/collector"
	"github.com/newthinker/atlas/internal/core"
)

func TestEastmoney_ImplementsCollector(t *testing.T) {
	var _ collector.Collector = (*Eastmoney)(nil)
}

func TestEastmoney_Name(t *testing.T) {
	e := New()
	if e.Name() != "eastmoney" {
		t.Errorf("expected 'eastmoney', got '%s'", e.Name())
	}
}

func TestEastmoney_SupportedMarkets(t *testing.T) {
	e := New()
	markets := e.SupportedMarkets()

	if len(markets) != 1 || markets[0] != core.MarketCNA {
		t.Error("expected only CN_A market")
	}
}

func TestEastmoney_ParseSymbol(t *testing.T) {
	tests := []struct {
		input      string
		wantCode   string
		wantMarket string
	}{
		{"600519.SH", "600519", "1"}, // Shanghai = 1
		{"000001.SZ", "000001", "0"}, // Shenzhen = 0
	}

	e := New()
	for _, tc := range tests {
		code, market := e.parseSymbol(tc.input)
		if code != tc.wantCode || market != tc.wantMarket {
			t.Errorf("parseSymbol(%s) = (%s, %s), want (%s, %s)",
				tc.input, code, market, tc.wantCode, tc.wantMarket)
		}
	}
}

// Context Checkpoint: done_criteria → test mapping (TASK-004)
// functional[0] "000001.SH(指数)→1 与 000001.SZ(个股)→0 区分；600519.SH 不受影响"
//   → TestParseSymbol_AShareIndexes
// boundary "表外 .SH/.SZ 仍走既有后缀规则" → 表外用例 (000001.SZ / 600519.SH / 399001.SZ)
func TestParseSymbol_AShareIndexes(t *testing.T) {
	e := New()
	cases := []struct {
		symbol, wantCode, wantMarket string
		inTable                      bool // 是否经 AShareIndexSecIDs 权威表命中
	}{
		{"000300.SH", "000300", "1", true},  // 沪深300（表内指数）
		{"000905.SH", "000905", "1", true},  // 中证500（表内指数）
		{"000001.SH", "000001", "1", true},  // 上证指数（表内指数）
		{"399006.SZ", "399006", "0", true},  // 创业板指（表内指数，SZ→0）
		{"000001.SZ", "000001", "0", false}, // 平安银行（表外个股，走后缀规则）
		{"399001.SZ", "399001", "0", false}, // 深证成指（表外，走后缀规则）
		{"600519.SH", "600519", "1", false}, // 个股不受影响
	}
	for _, c := range cases {
		code, market := e.parseSymbol(c.symbol)
		if code != c.wantCode || market != c.wantMarket {
			t.Errorf("parseSymbol(%q) = (%s,%s), want (%s,%s)",
				c.symbol, code, market, c.wantCode, c.wantMarket)
		}
		// 权威性断言：表内符号的 (market,code) 必须等于 AShareIndexSecIDs 的 secid 分解，
		// 证明索引经表（单一真相源）解析而非偶合后缀规则。
		if c.inTable {
			wantSecID := collector.AShareIndexSecIDs[c.symbol]
			if got := market + "." + code; got != wantSecID {
				t.Errorf("parseSymbol(%q) secid = %q, want table value %q", c.symbol, got, wantSecID)
			}
			if !collector.IsAShareIndex(c.symbol) {
				t.Errorf("%q should be a known A-share index", c.symbol)
			}
		}
	}
}

func TestEastmoney_ToKlineType(t *testing.T) {
	tests := []struct {
		interval string
		expected string
	}{
		{"1m", "1"},
		{"5m", "5"},
		{"1h", "60"},
		{"1d", "101"},
	}

	e := New()
	for _, tc := range tests {
		got := e.toKlineType(tc.interval)
		if got != tc.expected {
			t.Errorf("toKlineType(%s) = %s, want %s", tc.interval, got, tc.expected)
		}
	}
}

// --- httptest-based tests (TASK-009) ---

// functional[0]: injection constructor keeps production defaults when given empty args.
func TestNewWithBaseURLs_DefaultsUnchanged(t *testing.T) {
	e := NewWithBaseURLs("", "", "", "")
	if e.quoteURL != defaultQuoteURL || e.historyURL != defaultHistoryURL ||
		e.fundURL != defaultFundURL || e.fundHistoryURL != defaultFundHistoryURL {
		t.Error("empty args should preserve default base URLs")
	}
	custom := NewWithBaseURLs("a", "b", "c", "d")
	if custom.quoteURL != "a" || custom.historyURL != "b" || custom.fundURL != "c" || custom.fundHistoryURL != "d" {
		t.Error("non-empty args should override base URLs")
	}
}

// newStockBroker spins up an httptest server returning body for any request and
// wires it as the quote+history base URL of a fresh collector.
func newStockServer(t *testing.T, status int, body string) (*Eastmoney, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return NewWithBaseURLs(srv.URL, srv.URL, srv.URL, srv.URL), srv
}

// functional[1]
func TestFetchQuote_Stock(t *testing.T) {
	body := `{"data":{"f43":1500,"f44":1499,"f45":1501,"f46":1480,"f47":12345,"f48":0,"f51":1520,"f52":1470,"f57":"600519","f58":"X","f60":1485,"f169":15,"f170":1.01}}`
	e, _ := newStockServer(t, http.StatusOK, body)

	q, err := e.FetchQuote("600519.SH")
	if err != nil {
		t.Fatalf("FetchQuote: %v", err)
	}
	if q.Symbol != "600519.SH" || q.Market != core.MarketCNA {
		t.Errorf("symbol/market = %s/%s", q.Symbol, q.Market)
	}
	// stock divisor is 100
	if q.Price != 15.0 || q.Open != 14.8 || q.High != 15.2 || q.Low != 14.7 || q.PrevClose != 14.85 {
		t.Errorf("unexpected prices: %+v", q)
	}
	if q.Volume != 12345 || q.Source != "eastmoney" {
		t.Errorf("volume/source = %d/%s", q.Volume, q.Source)
	}
}

// W1 fix: eastmoney f170 is percent×100, so ChangePercent must be divided by a
// fixed 100 scale — independent of the price divisor (1000 for ETFs). Covers the
// stock path, the ETF path (divisor=1000 must not touch ChangePercent), and the
// zero/negative boundaries.
func TestFetchQuote_ChangePercentScale(t *testing.T) {
	cases := []struct {
		name   string
		symbol string
		f170   string
		want   float64
	}{
		{"stock positive", "600519.SH", "204", 2.04},
		{"etf positive keeps 100 scale", "510300.SH", "204", 2.04},
		{"zero", "600519.SH", "0", 0},
		{"negative", "600519.SH", "-153", -1.53},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := fmt.Sprintf(`{"data":{"f43":1500,"f46":1480,"f51":1520,"f52":1470,"f60":1485,"f47":1,"f169":15,"f170":%s}}`, tc.f170)
			e, _ := newStockServer(t, http.StatusOK, body)
			q, err := e.FetchQuote(tc.symbol)
			if err != nil {
				t.Fatalf("FetchQuote: %v", err)
			}
			if q.ChangePercent != tc.want {
				t.Errorf("ChangePercent = %v, want %v", q.ChangePercent, tc.want)
			}
		})
	}
}

// functional[2]
func TestFetchHistory_Stock(t *testing.T) {
	body := `{"data":{"code":"600519","name":"X","klines":["2024-01-02,10.0,11.0,12.0,9.0,1000","2024-01-03,11.0,12.5,13.0,10.5,2000"]}}`
	e, _ := newStockServer(t, http.StatusOK, body)

	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC)
	bars, err := e.FetchHistory("600519.SH", start, end, "1d")
	if err != nil {
		t.Fatalf("FetchHistory: %v", err)
	}
	if len(bars) != 2 {
		t.Fatalf("bars = %d, want 2", len(bars))
	}
	// kline format: date,open,close,high,low,volume
	b0 := bars[0]
	if b0.Open != 10.0 || b0.Close != 11.0 || b0.High != 12.0 || b0.Low != 9.0 || b0.Volume != 1000 {
		t.Errorf("bar0 unexpected: %+v", b0)
	}
	if b0.Symbol != "600519.SH" || b0.Interval != "1d" {
		t.Errorf("bar0 symbol/interval = %s/%s", b0.Symbol, b0.Interval)
	}
}

// boundary[0] + error[2]: HTTP 200 with null data returns error, no panic.
func TestFetchQuote_NullData(t *testing.T) {
	e, _ := newStockServer(t, http.StatusOK, `{"data":null}`)
	if _, err := e.FetchQuote("600519.SH"); err == nil {
		t.Fatal("expected error for null data")
	}
}

// boundary[0]: empty klines list returns error, no panic.
func TestFetchHistory_EmptyKlines(t *testing.T) {
	e, _ := newStockServer(t, http.StatusOK, `{"data":{"code":"600519","name":"X","klines":[]}}`)
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC)
	if _, err := e.FetchHistory("600519.SH", start, end, "1d"); err == nil {
		t.Fatal("expected error for empty klines")
	}
}

// error[0]: status code itself must drive the error. Body is VALID JSON that
// would parse successfully on 200, so a passing test proves a real StatusCode
// guard (not a decode-failure path shared with the malformed-JSON test).
func TestFetchQuote_HTTPError(t *testing.T) {
	validBody := `{"data":{"f43":1500,"f46":1480,"f51":1520,"f52":1470,"f60":1485,"f47":1,"f169":15,"f170":1.0}}`
	e, _ := newStockServer(t, http.StatusInternalServerError, validBody)
	if _, err := e.FetchQuote("600519.SH"); err == nil {
		t.Fatal("expected error for HTTP 500 even with valid JSON body")
	}
}

// error[0]: same as above for the history endpoint.
func TestFetchHistory_HTTPError(t *testing.T) {
	validBody := `{"data":{"code":"600519","name":"X","klines":["2024-01-02,10.0,11.0,12.0,9.0,1000"]}}`
	e, _ := newStockServer(t, http.StatusInternalServerError, validBody)
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC)
	if _, err := e.FetchHistory("600519.SH", start, end, "1d"); err == nil {
		t.Fatal("expected error for HTTP 500 even with valid JSON body")
	}
}

// error[0]: fund quote endpoint must also honor the status code (valid JSONP body).
func TestFetchQuote_Fund_HTTPError(t *testing.T) {
	validJSONP := `jsonpgz({"fundcode":"110011","dwjz":"1.50","gsz":"1.60","gszzl":"6.67"});`
	e, _ := newStockServer(t, http.StatusInternalServerError, validJSONP)
	if _, err := e.FetchQuote("110011.OF"); err == nil {
		t.Fatal("expected error for fund quote HTTP 500 even with valid JSONP body")
	}
}

// error[0]: fund history endpoint must also honor the status code (valid JSON body).
func TestFetchHistory_Fund_HTTPError(t *testing.T) {
	validBody := `{"Data":{"LSJZList":[{"FSRQ":"2024-01-02","DWJZ":"1.50"}]}}`
	e, _ := newStockServer(t, http.StatusInternalServerError, validBody)
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC)
	if _, err := e.FetchHistory("110011.OF", start, end, "1d"); err == nil {
		t.Fatal("expected error for fund history HTTP 500 even with valid JSON body")
	}
}

// error[1]
func TestFetchQuote_MalformedJSON(t *testing.T) {
	e, _ := newStockServer(t, http.StatusOK, `{not json`)
	if _, err := e.FetchQuote("600519.SH"); err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

// non_func[0]: fund quote path (JSONP) via injected fund base URL.
func TestFetchQuote_Fund(t *testing.T) {
	body := `jsonpgz({"fundcode":"110011","name":"F","jzrq":"2024-01-01","dwjz":"1.5000","gsz":"1.6000","gszzl":"6.67","gztime":"2024-01-02 15:00"});`
	e, _ := newStockServer(t, http.StatusOK, body)

	q, err := e.FetchQuote("110011.OF")
	if err != nil {
		t.Fatalf("FetchQuote fund: %v", err)
	}
	if q.Source != "eastmoney-fund" {
		t.Errorf("source = %s, want eastmoney-fund", q.Source)
	}
	if q.Price != 1.6 || q.PrevClose != 1.5 {
		t.Errorf("fund price/prevclose = %v/%v", q.Price, q.PrevClose)
	}
}

// non_func[0]: fund history path via injected fund-history base URL.
func TestFetchHistory_Fund(t *testing.T) {
	body := `{"Data":{"LSJZList":[{"FSRQ":"2024-01-03","DWJZ":"1.60","LJJZ":"2.0","JZZZL":"1.0"},{"FSRQ":"2024-01-02","DWJZ":"1.50","LJJZ":"1.9","JZZZL":"0.5"}]}}`
	e, _ := newStockServer(t, http.StatusOK, body)

	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC)
	bars, err := e.FetchHistory("110011.OF", start, end, "1d")
	if err != nil {
		t.Fatalf("FetchHistory fund: %v", err)
	}
	if len(bars) != 2 {
		t.Fatalf("bars = %d, want 2", len(bars))
	}
	// reversed to chronological order: oldest first
	if bars[0].Time.After(bars[1].Time) {
		t.Error("fund history not in chronological order")
	}
	if bars[0].Close != 1.50 {
		t.Errorf("bars[0].Close = %v, want 1.50", bars[0].Close)
	}
}

// non_func[0]: lifecycle no-ops and config setters do not panic.
func TestLifecycleNoop(t *testing.T) {
	e := New()
	if err := e.Init(collector.Config{}); err != nil {
		t.Errorf("Init: %v", err)
	}
	if err := e.Start(context.TODO()); err != nil {
		t.Errorf("Start: %v", err)
	}
	if err := e.Stop(); err != nil {
		t.Errorf("Stop: %v", err)
	}
	e.SetLixingerFallback(nil)
}

// non_func[0]: exhaustively cover pure classification/parse helpers.
func TestPureHelpers(t *testing.T) {
	e := New()

	etfCases := map[string]bool{
		"159915.SZ": true, "510300.SH": true, "511990.SH": true,
		"512000.SH": true, "513050.SH": true, "515000.SH": true,
		"516000.SH": true, "518880.SH": true,
		"600519.SH": false, "12345": false, // wrong length
	}
	for sym, want := range etfCases {
		if got := e.isETF(sym); got != want {
			t.Errorf("isETF(%s) = %v, want %v", sym, got, want)
		}
	}

	fundCases := map[string]bool{
		"110011.OF": true, "000001.OF": true, "200001.OF": true,
		"300001.OF": true, "500001.OF": true,
		"159915.SZ": false, // ETF is not an open-end fund
		"600519.SH": false, // starts with 6
		"12345":     false, // wrong length
		// A-share indexes share leading digits with fund codes but are
		// not funds — they must take the index secid path, not fund NAV
		// (regression: FetchHistory("000300.SH") failed as "no history for fund")
		"000300.SH": false, "000001.SH": false, "399001.SZ": false,
		// Shenzhen main-board equities (000xxx.SZ) share the leading '0' with
		// open-end fund codes but carry an exchange suffix → exchange-listed
		// stocks, never open-end funds (regression: 五粮液/东阿阿胶 routed to
		// fund NAV path and lost OHLCV).
		"000858.SZ": false, "000423.SZ": false, "000001.SZ": false,
	}
	for sym, want := range fundCases {
		if got := e.isFund(sym); got != want {
			t.Errorf("isFund(%s) = %v, want %v", sym, got, want)
		}
	}

	klineCases := map[string]string{
		"1m": "1", "5m": "5", "15m": "15", "30m": "30",
		"1h": "60", "1d": "101", "weird": "101",
	}
	for in, want := range klineCases {
		if got := e.toKlineType(in); got != want {
			t.Errorf("toKlineType(%s) = %s, want %s", in, got, want)
		}
	}

	// parseSymbol: no-dot input falls back to (symbol, "1").
	if code, mk := e.parseSymbol("600519"); code != "600519" || mk != "1" {
		t.Errorf("parseSymbol(no dot) = (%s,%s)", code, mk)
	}
}
