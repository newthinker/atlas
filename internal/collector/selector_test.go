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

// Context Checkpoint: done_criteria → test mapping
// functional[1] "MarketForSymbol index/commodity 全用例" → TestMarketForSymbol_IndexAndCommodity
// functional[2] "SelectForSymbol ^GSPC/^HSI/GC=F→yahoo, 000300.SH→eastmoney" → TestSelectForSymbol_IndexAndCommodityRouteToYahoo
// functional[3] "KnownIndexMarket 表内(market,true)/表外^(_,false)" → TestKnownIndexMarket

func TestMarketForSymbol_IndexAndCommodity(t *testing.T) {
	cases := []struct {
		symbol string
		want   core.Market
	}{
		{"^GSPC", core.MarketUS}, {"^IXIC", core.MarketUS}, {"^DJI", core.MarketUS},
		{"^HSI", core.MarketHK},
		{"^N225", core.MarketUS}, // 表外 ^ 符号默认 US（warning 由 app 层负责）
		{"GC=F", core.MarketUS}, {"CL=F", core.MarketUS},
		{"000300.SH", core.MarketCNA},
		{"AAPL", core.MarketUS}, {"BTC-USDT", core.MarketCrypto},
	}
	for _, c := range cases {
		if got := MarketForSymbol(c.symbol); got != c.want {
			t.Errorf("MarketForSymbol(%q) = %v, want %v", c.symbol, got, c.want)
		}
	}
}

func TestSelectForSymbol_IndexAndCommodityRouteToYahoo(t *testing.T) {
	reg := newRegistryWith("yahoo", "eastmoney", "crypto")
	for _, sym := range []string{"^GSPC", "^HSI", "GC=F"} {
		if c := SelectForSymbol(reg, sym); c == nil || c.Name() != "yahoo" {
			t.Errorf("SelectForSymbol(%q) -> %v, want yahoo", sym, c)
		}
	}
	if c := SelectForSymbol(reg, "000300.SH"); c == nil || c.Name() != "eastmoney" {
		t.Errorf("000300.SH should route to eastmoney")
	}
}

func TestKnownIndexMarket(t *testing.T) {
	// 表内符号返回 (market, true)
	known := []struct {
		symbol string
		want   core.Market
	}{
		{"^GSPC", core.MarketUS},
		{"^IXIC", core.MarketUS},
		{"^DJI", core.MarketUS},
		{"^HSI", core.MarketHK},
		{"^gspc", core.MarketUS}, // 大小写不敏感
	}
	for _, c := range known {
		m, ok := KnownIndexMarket(c.symbol)
		if !ok || m != c.want {
			t.Errorf("KnownIndexMarket(%q) = (%v,%v), want (%v,true)", c.symbol, m, ok, c.want)
		}
	}
	// 表外 ^ 符号返回 (_, false)
	for _, sym := range []string{"^N225", "^FTSE"} {
		if _, ok := KnownIndexMarket(sym); ok {
			t.Errorf("KnownIndexMarket(%q) should be unknown", sym)
		}
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

func TestCSIIndexRouting(t *testing.T) {
	// .CSI 后缀的中证跨市场指数靠 AShareIndexSecIDs 表成员判定路由，
	// 不依赖 .SH/.SZ 后缀（如 930713.CSI 中证人工智能主题）。
	reg := newRegistryWith("yahoo", "eastmoney", "crypto")
	for _, sym := range []string{"930713.CSI", "930604.CSI", "000922.SH"} {
		if c := SelectForSymbol(reg, sym); c == nil || c.Name() != "eastmoney" {
			t.Errorf("SelectForSymbol(%q) -> %v, want eastmoney", sym, c)
		}
		if m := MarketForSymbol(sym); m != core.MarketCNA {
			t.Errorf("MarketForSymbol(%q) = %v, want CN_A", sym, m)
		}
	}
}
