package collector

import (
	"context"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

type fakeCollector struct {
	name    string
	markets []core.Market
}

func (f *fakeCollector) Name() string                    { return f.name }
func (f *fakeCollector) SupportedMarkets() []core.Market { return f.markets }
func (f *fakeCollector) Init(cfg Config) error           { return nil }
func (f *fakeCollector) Start(ctx context.Context) error { return nil }
func (f *fakeCollector) Stop() error                     { return nil }
func (f *fakeCollector) FetchQuote(symbol string) (*core.Quote, error) {
	return &core.Quote{Symbol: symbol}, nil
}
func (f *fakeCollector) FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	return nil, nil
}

func newRegistryWith(names ...string) *Registry {
	reg := NewRegistry()
	for _, n := range names {
		reg.Register(&fakeCollector{name: n})
	}
	return reg
}

func TestSelectForSymbol(t *testing.T) {
	reg := newRegistryWith("yahoo", "eastmoney", "crypto")

	tests := []struct {
		symbol string
		want   string
	}{
		{"AAPL", "yahoo"},
		{"0700.HK", "yahoo"},
		{"600519.SH", "eastmoney"},
		{"000001.SZ", "eastmoney"},
		{"BTC", "crypto"},
		{"BTCUSDT", "crypto"},
		{"ETH-USD", "crypto"},
		{"SOL", "crypto"},
	}

	for _, tt := range tests {
		got := SelectForSymbol(reg, tt.symbol)
		if got == nil {
			t.Fatalf("%s: expected collector %q, got nil", tt.symbol, tt.want)
		}
		if got.Name() != tt.want {
			t.Errorf("%s: expected collector %q, got %q", tt.symbol, tt.want, got.Name())
		}
	}
}

func TestSelectForSymbol_FallbackToYahoo(t *testing.T) {
	// A-share symbol but no eastmoney collector -> fall back to yahoo.
	reg := newRegistryWith("yahoo")
	got := SelectForSymbol(reg, "600519.SH")
	if got == nil || got.Name() != "yahoo" {
		t.Fatalf("expected yahoo fallback, got %v", got)
	}
}

func TestSelectForSymbol_FallbackToAny(t *testing.T) {
	// No preferred collectors registered -> return whatever is available.
	reg := newRegistryWith("custom")
	got := SelectForSymbol(reg, "AAPL")
	if got == nil || got.Name() != "custom" {
		t.Fatalf("expected custom fallback, got %v", got)
	}
}

func TestSelectForSymbol_EmptyRegistry(t *testing.T) {
	if got := SelectForSymbol(NewRegistry(), "AAPL"); got != nil {
		t.Errorf("expected nil for empty registry, got %v", got)
	}
	if got := SelectForSymbol(nil, "AAPL"); got != nil {
		t.Errorf("expected nil for nil registry, got %v", got)
	}
}

func TestMarketForSymbol(t *testing.T) {
	tests := []struct {
		symbol string
		want   core.Market
	}{
		{"AAPL", core.MarketUS},
		{"0700.HK", core.MarketHK},
		{"600519.SH", core.MarketCNA},
		{"000001.SZ", core.MarketCNA},
		{"BTC", core.MarketCrypto},
		{"ETH-USD", core.MarketCrypto},
	}
	for _, tt := range tests {
		if got := MarketForSymbol(tt.symbol); got != tt.want {
			t.Errorf("%s: expected market %q, got %q", tt.symbol, tt.want, got)
		}
	}
}
