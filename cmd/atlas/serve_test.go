package main

// Context Checkpoint: done_criteria → test mapping (TASK-007)
// functional[0]   "cache.enabled=true 普通 collector 被 CachedCollector 包装(TTL来自配置)" → TestMaybeCache_EnabledWrapsPlain
// functional[1]   "cache.enabled=false 原样注册不包装"                                 → TestMaybeCache_DisabledNoWrap
// boundary[0]     "FundamentalCollector 等扩展接口的 collector 不被包装破坏断言路径"    → TestMaybeCache_FundamentalNotWrapped
// non_functional[0] "包装后 selector.SelectForSymbol 市场匹配行为不变(Name/Markets透传)" → TestMaybeCache_SelectorRoutingUnchanged

import (
	"context"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/app"
	"github.com/newthinker/atlas/internal/collector"
	"github.com/newthinker/atlas/internal/collector/lixinger"
	"github.com/newthinker/atlas/internal/collector/yahoo"
	"github.com/newthinker/atlas/internal/core"
)

// TASK-012 done_criteria → test mapping
// functional[0]/boundary[0] "估值源注入 typed-nil 防护：nil 指针不得变非 nil 接口"
//   → TestValuationSourceOrNil_NilStaysNil / TestEPSSourceOrNil_NilStaysNil
//   + TestValuationSourceOrNil_RealNonNil / TestEPSSourceOrNil_RealNonNil

// functional[0] + boundary[0]: a nil *lixinger.Lixinger must NOT become a
// non-nil app.ValuationSource, or buildFundamental's `valuationSrc != nil`
// guard would be defeated (typed-nil interface trap) and panic at call time.
func TestValuationSourceOrNil_NilStaysNil(t *testing.T) {
	var nilCollector *lixinger.Lixinger
	got := valuationSourceOrNil(nilCollector)
	if got != nil {
		t.Errorf("valuationSourceOrNil(nil *Lixinger) = %v (typed-nil leak), want untyped nil interface", got)
	}
}

func TestEPSSourceOrNil_NilStaysNil(t *testing.T) {
	var nilCollector *yahoo.Yahoo
	got := epsSourceOrNil(nilCollector)
	if got != nil {
		t.Errorf("epsSourceOrNil(nil *Yahoo) = %v (typed-nil leak), want untyped nil interface", got)
	}
}

// functional[0]: a real collector must be injected as a non-nil interface.
func TestValuationSourceOrNil_RealNonNil(t *testing.T) {
	var _ app.ValuationSource = lixinger.New("dummy-key") // compile-time impl check
	if got := valuationSourceOrNil(lixinger.New("dummy-key")); got == nil {
		t.Error("valuationSourceOrNil(real *Lixinger) = nil, want non-nil source")
	}
}

func TestEPSSourceOrNil_RealNonNil(t *testing.T) {
	var _ app.EPSSource = yahoo.New() // compile-time impl check
	if got := epsSourceOrNil(yahoo.New()); got == nil {
		t.Error("epsSourceOrNil(real *Yahoo) = nil, want non-nil source")
	}
}

// plainCollector implements only collector.Collector.
type plainCollector struct {
	name    string
	markets []core.Market
}

func (p *plainCollector) Name() string                    { return p.name }
func (p *plainCollector) SupportedMarkets() []core.Market { return p.markets }
func (p *plainCollector) Init(cfg collector.Config) error { return nil }
func (p *plainCollector) Start(ctx context.Context) error { return nil }
func (p *plainCollector) Stop() error                     { return nil }
func (p *plainCollector) FetchQuote(symbol string) (*core.Quote, error) {
	return &core.Quote{Symbol: symbol}, nil
}
func (p *plainCollector) FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	return nil, nil
}

// fundamentalCollectorStub implements collector.Collector AND the
// collector.FundamentalCollector extension interface.
type fundamentalCollectorStub struct {
	plainCollector
}

func (f *fundamentalCollectorStub) FetchFundamental(symbol string) (*core.Fundamental, error) {
	return &core.Fundamental{Symbol: symbol}, nil
}
func (f *fundamentalCollectorStub) FetchFundamentalHistory(symbol string, start, end time.Time) ([]core.Fundamental, error) {
	return nil, nil
}

// functional[0]
func TestMaybeCache_EnabledWrapsPlain(t *testing.T) {
	plain := &plainCollector{name: "yahoo", markets: []core.Market{core.MarketUS}}

	got := maybeCache(plain, true, 5*time.Minute)

	if _, ok := got.(*collector.CachedCollector); !ok {
		t.Fatalf("enabled cache must wrap a plain collector in *CachedCollector, got %T", got)
	}
	// Name/SupportedMarkets must pass through the wrapper.
	if got.Name() != "yahoo" {
		t.Errorf("wrapped Name() = %q, want yahoo", got.Name())
	}
	if mk := got.SupportedMarkets(); len(mk) != 1 || mk[0] != core.MarketUS {
		t.Errorf("wrapped SupportedMarkets() = %v, want [US]", mk)
	}
}

// functional[1]
func TestMaybeCache_DisabledNoWrap(t *testing.T) {
	plain := &plainCollector{name: "yahoo", markets: []core.Market{core.MarketUS}}

	got := maybeCache(plain, false, 5*time.Minute)

	if _, ok := got.(*collector.CachedCollector); ok {
		t.Fatal("disabled cache must not wrap the collector")
	}
	if got != collector.Collector(plain) {
		t.Fatal("disabled cache must return the original collector instance")
	}
}

// boundary[0]
func TestMaybeCache_FundamentalNotWrapped(t *testing.T) {
	fund := &fundamentalCollectorStub{plainCollector{name: "lixinger", markets: []core.Market{core.MarketCNA}}}

	got := maybeCache(fund, true, 5*time.Minute)

	if _, ok := got.(*collector.CachedCollector); ok {
		t.Fatal("a FundamentalCollector must not be wrapped (would hide extension methods)")
	}
	if _, ok := got.(collector.FundamentalCollector); !ok {
		t.Fatal("type assertion to FundamentalCollector must still hold after maybeCache")
	}
	if got != collector.Collector(fund) {
		t.Fatal("FundamentalCollector must be returned unwrapped (same instance)")
	}
}

// non_functional[0]
func TestMaybeCache_SelectorRoutingUnchanged(t *testing.T) {
	crypto := &plainCollector{name: "crypto", markets: []core.Market{core.MarketCrypto}}
	wrapped := maybeCache(crypto, true, time.Minute)

	reg := collector.NewRegistry()
	reg.Register(wrapped)

	// Routing is by Name(); the wrapper must keep "crypto" reachable for a
	// crypto symbol.
	got := collector.SelectForSymbol(reg, "BTCUSDT")
	if got == nil {
		t.Fatal("selector returned nil for a crypto symbol")
	}
	if got.Name() != "crypto" {
		t.Fatalf("selector routed to %q, want crypto", got.Name())
	}
	if got != wrapped {
		t.Fatal("selector must return the registered (wrapped) collector instance")
	}
}
