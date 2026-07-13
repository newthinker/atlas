package yahoo

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/collector"
	"github.com/newthinker/atlas/internal/core"
)

// newTestServer returns an httptest server replying with the given status and
// body for any path, plus a Yahoo collector wired to it.
func newTestServer(t *testing.T, status int, body string) (*httptest.Server, *Yahoo) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, NewWithBaseURL(srv.URL)
}

func TestYahoo_ImplementsCollector(t *testing.T) {
	var _ collector.Collector = (*Yahoo)(nil)
}

func TestYahoo_Name(t *testing.T) {
	y := New()
	if y.Name() != "yahoo" {
		t.Errorf("expected 'yahoo', got '%s'", y.Name())
	}
}

func TestYahoo_SupportedMarkets(t *testing.T) {
	y := New()
	markets := y.SupportedMarkets()

	if len(markets) == 0 {
		t.Error("expected at least one supported market")
	}
}

func TestYahoo_ToYahooSymbol(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"AAPL", "AAPL"},
		{"0700.HK", "0700.HK"},
		{"600519.SH", "600519.SS"}, // Shanghai -> SS for Yahoo
		{"000001.SZ", "000001.SZ"},
	}

	y := New()
	for _, tc := range tests {
		got := y.toYahooSymbol(tc.input)
		if got != tc.expected {
			t.Errorf("toYahooSymbol(%s) = %s, want %s", tc.input, got, tc.expected)
		}
	}
}

func TestYahoo_DetectMarket(t *testing.T) {
	tests := []struct {
		symbol   string
		expected core.Market
	}{
		{"AAPL", core.MarketUS},
		{"0700.HK", core.MarketHK},
		{"600519.SH", core.MarketCNA},
		{"000001.SZ", core.MarketCNA},
	}

	y := New()
	for _, tc := range tests {
		got := y.detectMarket(tc.symbol)
		if got != tc.expected {
			t.Errorf("detectMarket(%s) = %s, want %s", tc.symbol, got, tc.expected)
		}
	}
}

func TestValidateSymbol(t *testing.T) {
	tests := []struct {
		name    string
		symbol  string
		wantErr bool
	}{
		{"valid US symbol", "AAPL", false},
		{"valid HK symbol", "0700.HK", false},
		{"valid CN symbol", "600519.SH", false},
		{"valid SZ symbol", "000001.SZ", false},
		{"valid lowercase", "aapl", false},
		{"empty symbol", "", true},
		{"too long", "VERYLONGSYMBOLNAME12345", true},
		{"invalid chars", "AAP!L", true},
		{"path injection", "../etc/passwd", true},
		{"url injection", "AAPL?foo=bar", true},
		{"space injection", "AAPL bar", true},
		{"newline injection", "AAPL\nbar", true},
		{"slash injection", "AAPL/bar", true},
		{"ampersand injection", "AAPL&bar=baz", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSymbol(tt.symbol)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSymbol(%q) error = %v, wantErr %v", tt.symbol, err, tt.wantErr)
			}
		})
	}
}

// functional[0]: validateSymbol accepts index(^) and futures(=F) forms,
// rejects empty / bare prefixes / mixed / injection.
func TestValidateSymbol_IndexAndFutures(t *testing.T) {
	cases := []struct {
		symbol string
		wantOK bool
	}{
		{"AAPL", true}, {"600519.SH", true}, {"0700.HK", true},
		{"^GSPC", true}, {"^IXIC", true}, {"^HSI", true},
		{"GC=F", true}, {"CL=F", true}, {"SI=F", true},
		{"", false}, {"^", false}, {"=F", false},
		{"^GSPC.SH", false}, {"GC=X=F", false}, {"AAPL; DROP", false},
	}
	for _, c := range cases {
		err := validateSymbol(c.symbol)
		if (err == nil) != c.wantOK {
			t.Errorf("validateSymbol(%q) ok=%v, want %v", c.symbol, err == nil, c.wantOK)
		}
	}
}

// Context Checkpoint: done_criteria -> test mapping
// functional[0] "validateSymbol(JPY=X) 通过（货币后缀 =X 放行）"        -> case "JPY=X" (wantOK true)
// boundary[0]   "GC=F/^MOVE/普通符号继续通过；JPY=Z 等未知后缀仍拒绝" -> cases GC=F, ^MOVE, JPY=Z
func TestValidateSymbolCurrencyPairs(t *testing.T) {
	cases := []struct {
		symbol string
		wantOK bool
	}{
		{"JPY=X", true},  // Yahoo currency symbol (design §2.1 USD/JPY)
		{"GC=F", true},   // futures suffix unaffected
		{"^MOVE", true},  // index suffix unaffected
		{"JPY=Z", false}, // unknown suffix still rejected
	}
	for _, c := range cases {
		err := validateSymbol(c.symbol)
		if (err == nil) != c.wantOK {
			t.Errorf("validateSymbol(%q) ok=%v, want %v", c.symbol, err == nil, c.wantOK)
		}
	}
}

// functional[1]: ^ must be percent-encoded as %5E in the request path.
func TestFetchQuote_EscapesIndexSymbol(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		w.Write([]byte(`{"chart":{"result":[{"meta":{"symbol":"^GSPC","regularMarketPrice":5000,"chartPreviousClose":4990,"regularMarketTime":1700000000},"timestamp":[1700000000],"indicators":{"quote":[{"open":[4995],"high":[5010],"low":[4980],"close":[5000],"volume":[1000]}]}}],"error":null}}`))
	}))
	defer srv.Close()

	y := NewWithBaseURL(srv.URL)
	if _, err := y.FetchQuote("^GSPC"); err != nil {
		t.Fatalf("FetchQuote(^GSPC) error: %v", err)
	}
	if !strings.Contains(gotPath, "%5EGSPC") {
		t.Errorf("request path %q does not percent-encode ^ as %%5E", gotPath)
	}
}

func TestFetchQuote_ValidatesSymbol(t *testing.T) {
	y := New()
	_, err := y.FetchQuote("../etc/passwd")
	if err == nil {
		t.Error("FetchQuote should reject invalid symbol")
	}
	if !strings.Contains(err.Error(), "invalid symbol format") {
		t.Errorf("expected 'invalid symbol format' error, got: %v", err)
	}
}

func TestFetchHistory_ValidatesSymbol(t *testing.T) {
	y := New()
	_, err := y.FetchHistory("AAPL?foo=bar", time.Now().Add(-24*time.Hour), time.Now(), "1d")
	if err == nil {
		t.Error("FetchHistory should reject invalid symbol")
	}
	if !strings.Contains(err.Error(), "invalid symbol format") {
		t.Errorf("expected 'invalid symbol format' error, got: %v", err)
	}
}

const quoteResponse = `{
  "chart": {
    "result": [{
      "meta": {
        "symbol": "AAPL",
        "regularMarketPrice": 150.5,
        "regularMarketVolume": 1000000,
        "regularMarketTime": 1700000000,
        "regularMarketOpen": 148.0,
        "regularMarketDayHigh": 151.0,
        "regularMarketDayLow": 147.5,
        "chartPreviousClose": 149.0,
        "regularMarketChangePercent": 1.0067
      }
    }],
    "error": null
  }
}`

const historyResponse = `{
  "chart": {
    "result": [{
      "meta": {"symbol": "AAPL"},
      "timestamp": [1700000000, 1700086400],
      "indicators": {
        "quote": [{
          "open":   [100.0, 102.0],
          "high":   [105.0, 106.0],
          "low":    [99.0, 101.0],
          "close":  [104.0, 105.5],
          "volume": [1000, 2000]
        }]
      }
    }],
    "error": null
  }
}`

// functional[1]
func TestFetchQuote_ParsesResponse(t *testing.T) {
	_, y := newTestServer(t, http.StatusOK, quoteResponse)

	quote, err := y.FetchQuote("AAPL")
	if err != nil {
		t.Fatalf("FetchQuote failed: %v", err)
	}
	if quote.Symbol != "AAPL" {
		t.Errorf("Symbol = %q, want AAPL", quote.Symbol)
	}
	if quote.Price != 150.5 {
		t.Errorf("Price = %v, want 150.5", quote.Price)
	}
	if quote.Open != 148.0 {
		t.Errorf("Open = %v, want 148.0", quote.Open)
	}
	if quote.High != 151.0 {
		t.Errorf("High = %v, want 151.0", quote.High)
	}
	if quote.Low != 147.5 {
		t.Errorf("Low = %v, want 147.5", quote.Low)
	}
	if quote.PrevClose != 149.0 {
		t.Errorf("PrevClose = %v, want 149.0", quote.PrevClose)
	}
	if quote.Change != 150.5-149.0 {
		t.Errorf("Change = %v, want %v", quote.Change, 150.5-149.0)
	}
	if quote.Volume != 1000000 {
		t.Errorf("Volume = %d, want 1000000", quote.Volume)
	}
	if !quote.Time.Equal(time.Unix(1700000000, 0)) {
		t.Errorf("Time = %v, want %v", quote.Time, time.Unix(1700000000, 0))
	}
	if quote.Source != "yahoo" {
		t.Errorf("Source = %q, want yahoo", quote.Source)
	}
}

// functional[2]
func TestFetchHistory_ParsesResponse(t *testing.T) {
	_, y := newTestServer(t, http.StatusOK, historyResponse)

	bars, err := y.FetchHistory("AAPL", time.Unix(1700000000, 0), time.Unix(1700086400, 0), "1d")
	if err != nil {
		t.Fatalf("FetchHistory failed: %v", err)
	}
	if len(bars) != 2 {
		t.Fatalf("len(bars) = %d, want 2", len(bars))
	}

	first := bars[0]
	if !first.Time.Equal(time.Unix(1700000000, 0)) {
		t.Errorf("bars[0].Time = %v, want %v", first.Time, time.Unix(1700000000, 0))
	}
	if first.Open != 100.0 || first.High != 105.0 || first.Low != 99.0 || first.Close != 104.0 {
		t.Errorf("bars[0] OHLC = %v/%v/%v/%v, want 100/105/99/104",
			first.Open, first.High, first.Low, first.Close)
	}
	if first.Volume != 1000 {
		t.Errorf("bars[0].Volume = %d, want 1000", first.Volume)
	}
	if first.Interval != "1d" {
		t.Errorf("bars[0].Interval = %q, want 1d", first.Interval)
	}

	if bars[1].Close != 105.5 {
		t.Errorf("bars[1].Close = %v, want 105.5", bars[1].Close)
	}
}

// boundary: empty result / empty timestamp must not panic.
func TestFetchHistory_EmptyResult(t *testing.T) {
	t.Run("empty result array", func(t *testing.T) {
		_, y := newTestServer(t, http.StatusOK, `{"chart":{"result":[],"error":null}}`)
		if _, err := y.FetchHistory("AAPL", time.Unix(1, 0), time.Unix(2, 0), "1d"); err == nil {
			t.Error("expected error for empty result, got nil")
		}
	})

	t.Run("empty timestamps", func(t *testing.T) {
		body := `{"chart":{"result":[{"meta":{"symbol":"AAPL"},"timestamp":[],
			"indicators":{"quote":[{"open":[],"high":[],"low":[],"close":[],"volume":[]}]}}],"error":null}}`
		_, y := newTestServer(t, http.StatusOK, body)
		bars, err := y.FetchHistory("AAPL", time.Unix(1, 0), time.Unix(2, 0), "1d")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(bars) != 0 {
			t.Errorf("len(bars) = %d, want 0", len(bars))
		}
	})
}

// error_handling[0]: non-200 status.
func TestFetch_NonOKStatus(t *testing.T) {
	_, y := newTestServer(t, http.StatusInternalServerError, `{}`)

	if _, err := y.FetchQuote("AAPL"); err == nil {
		t.Error("FetchQuote: expected error for 500 status, got nil")
	}
	if _, err := y.FetchHistory("AAPL", time.Unix(1, 0), time.Unix(2, 0), "1d"); err == nil {
		t.Error("FetchHistory: expected error for 500 status, got nil")
	}
}

// error_handling[1]: malformed JSON.
func TestFetch_MalformedJSON(t *testing.T) {
	_, y := newTestServer(t, http.StatusOK, `{not valid json`)

	if _, err := y.FetchQuote("AAPL"); err == nil {
		t.Error("FetchQuote: expected error for malformed JSON, got nil")
	}
	if _, err := y.FetchHistory("AAPL", time.Unix(1, 0), time.Unix(2, 0), "1d"); err == nil {
		t.Error("FetchHistory: expected error for malformed JSON, got nil")
	}
}

// Context Checkpoint: done_criteria -> test mapping
// error_handling[nil-fields] "bar with any nil OHLCV field must be skipped, not panic" -> TestFetchHistory_SkipsBarsWithNilFields
// boundary[nil-fields]       "legal bar adjacent to nil bar is returned intact"         -> TestFetchHistory_SkipsBarsWithNilFields

// TestFetchHistory_SkipsBarsWithNilFields verifies that FetchHistory does not
// panic when Yahoo returns a bar whose Open is non-nil but Close (or another
// field) is null, and that such bars are silently skipped while adjacent valid
// bars are returned correctly.
//
// The response has three timestamps:
//   ts[0]: all fields valid     -> bar must appear in output
//   ts[1]: close == null        -> bar must be skipped (was causing panic before fix)
//   ts[2]: volume == null       -> bar must be skipped
func TestFetchHistory_SkipsBarsWithNilFields(t *testing.T) {
	// JSON: open is non-nil for all three entries, but close[1] and volume[2] are null.
	body := `{
  "chart": {
    "result": [{
      "meta": {"symbol": "AAPL"},
      "timestamp": [1700000000, 1700086400, 1700172800],
      "indicators": {
        "quote": [{
          "open":   [100.0, 102.0, 104.0],
          "high":   [105.0, 107.0, 109.0],
          "low":    [99.0,  101.0, 103.0],
          "close":  [104.0, null,  108.0],
          "volume": [1000,  2000,  null]
        }]
      }
    }],
    "error": null
  }
}`
	_, y := newTestServer(t, http.StatusOK, body)

	// A nil-field deref would panic here, which the test runner reports as a failure.
	bars, err := y.FetchHistory("AAPL", time.Unix(1700000000, 0), time.Unix(1700172800, 0), "1d")
	if err != nil {
		t.Fatalf("FetchHistory returned unexpected error: %v", err)
	}

	// Only ts[0] has all fields non-nil; ts[1] and ts[2] must be skipped.
	if len(bars) != 1 {
		t.Fatalf("len(bars) = %d, want 1 (only the fully-populated bar)", len(bars))
	}
	bar := bars[0]
	if !bar.Time.Equal(time.Unix(1700000000, 0)) {
		t.Errorf("bars[0].Time = %v, want %v", bar.Time, time.Unix(1700000000, 0))
	}
	if bar.Open != 100.0 || bar.High != 105.0 || bar.Low != 99.0 || bar.Close != 104.0 {
		t.Errorf("bars[0] OHLC = %v/%v/%v/%v, want 100/105/99/104",
			bar.Open, bar.High, bar.Low, bar.Close)
	}
	if bar.Volume != 1000 {
		t.Errorf("bars[0].Volume = %d, want 1000", bar.Volume)
	}
}
