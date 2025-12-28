package eastmoney

import (
	"testing"

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
