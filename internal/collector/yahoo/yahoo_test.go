package yahoo

import (
	"strings"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/collector"
	"github.com/newthinker/atlas/internal/core"
)

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
