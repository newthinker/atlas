package yahoo

import (
	"testing"

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
