package app

// Context Checkpoint: done_criteria → test mapping
// functional[0] "全路径 A 股" → TestSnapshotMetrics_FullPathAShare
// functional[1] "无估值降级 / crypto 不记估值 gap"
//               → TestSnapshotMetrics_MissingValuationDegrades / _CryptoNoValuationGap
// functional[2] "symbols 过滤保序 / ≥3 标的并发保序(B4)"
//               → TestSnapshotMetrics_SymbolsFilter / _OrderPreservedConcurrent
// functional[3] "panic 隔离 / 全采集器失败"
//               → TestSnapshotMetrics_PanicIsolated / _AllCollectorsFail
// boundary[1]   "价格百分位极值(B5,严格小于语义)" → TestSnapshotMetrics_PricePercentileExtremes
// non_functional[1] "并发等价 Workers=1 vs 4(B3)" → TestSnapshotMetrics_ConcurrentEquivalence

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/collector"
	"github.com/newthinker/atlas/internal/config"
	"github.com/newthinker/atlas/internal/core"
	"go.uber.org/zap"
)

// snapFake implements collector.Collector for snapshot tests.
type snapFake struct {
	quote      map[string]*core.Quote
	history    map[string][]core.OHLCV
	panicOn    string
	failQuotes bool
}

func (f *snapFake) Name() string { return "snapfake" }
func (f *snapFake) SupportedMarkets() []core.Market {
	return []core.Market{core.MarketUS, core.MarketHK, core.MarketCNA, core.MarketCrypto}
}
func (f *snapFake) Init(collector.Config) error     { return nil }
func (f *snapFake) Start(ctx context.Context) error { return nil }
func (f *snapFake) Stop() error                     { return nil }
func (f *snapFake) FetchQuote(symbol string) (*core.Quote, error) {
	if symbol == f.panicOn {
		panic("boom")
	}
	if f.failQuotes {
		return nil, fmt.Errorf("quote down")
	}
	if q, ok := f.quote[symbol]; ok {
		return q, nil
	}
	return nil, fmt.Errorf("no quote for %s", symbol)
}
func (f *snapFake) FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	if h, ok := f.history[symbol]; ok {
		return h, nil
	}
	return nil, fmt.Errorf("no history for %s", symbol)
}

type snapValuation struct{ pct float64 }

func (v snapValuation) FetchValuationPercentile(symbol string, lookbackYears int) (float64, error) {
	return v.pct, nil
}

type snapFundamental struct{ f *core.Fundamental }

func (s snapFundamental) FetchFundamental(symbol string) (*core.Fundamental, error) {
	if s.f == nil {
		return nil, fmt.Errorf("no fundamental")
	}
	return s.f, nil
}

func bars(closes ...float64) []core.OHLCV {
	out := make([]core.OHLCV, len(closes))
	t := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	for i, c := range closes {
		out[i] = core.OHLCV{Time: t.AddDate(0, 0, i), Open: c, High: c, Low: c, Close: c, Volume: 100}
	}
	return out
}

func newSnapshotApp(t *testing.T, fake *snapFake, items ...WatchlistItem) *App {
	t.Helper()
	a := New(config.Defaults(), zap.NewNop())
	a.RegisterCollector(fake)
	for _, it := range items {
		a.AddToWatchlistWithDetails(it.Symbol, it.Name, it.Market, it.Type, it.Strategies)
	}
	return a
}

func TestSnapshotMetrics_FullPathAShare(t *testing.T) {
	fake := &snapFake{
		quote:   map[string]*core.Quote{"600519.SH": {Price: 1400, ChangePercent: -0.5}},
		history: map[string][]core.OHLCV{"600519.SH": bars(100, 110, 120, 130, 140)},
	}
	a := newSnapshotApp(t, fake, WatchlistItem{Symbol: "600519.SH", Name: "贵州茅台", Market: "A股", Type: "股票"})
	a.SetValuationSources(snapValuation{pct: 12.3}, nil)
	a.SetFundamentalSource(snapFundamental{f: &core.Fundamental{PE: 19.5, PB: 5.96, DividendYield: 4.03}})

	ms := a.SnapshotMetrics(context.Background(), nil)
	if len(ms) != 1 {
		t.Fatalf("want 1 item, got %d", len(ms))
	}
	m := ms[0]
	if m.Price != 1400 || m.ChangePct != -0.5 {
		t.Errorf("quote fields = %v/%v", m.Price, m.ChangePct)
	}
	if m.PE == nil || *m.PE != 19.5 || m.PB == nil || *m.PB != 5.96 || m.DividendYield == nil || *m.DividendYield != 4.03 {
		t.Errorf("valuation fields = %+v", m)
	}
	if m.PEPercentile == nil || *m.PEPercentile != 12.3 {
		t.Errorf("pe percentile = %v", m.PEPercentile)
	}
	// 现价 1400 高于窗口内全部收盘 → 分位 100
	if m.PricePercentile == nil || *m.PricePercentile != 100 {
		t.Errorf("price percentile = %v", m.PricePercentile)
	}
	if len(m.Gaps) != 0 {
		t.Errorf("unexpected gaps: %v", m.Gaps)
	}
}

func TestSnapshotMetrics_MissingValuationDegrades(t *testing.T) {
	fake := &snapFake{
		quote:   map[string]*core.Quote{"AAPL": {Price: 210, ChangePercent: 1.2}},
		history: map[string][]core.OHLCV{"AAPL": bars(200, 205, 215)},
	}
	a := newSnapshotApp(t, fake, WatchlistItem{Symbol: "AAPL", Name: "Apple", Market: "美股", Type: "股票"})
	// 不注入任何估值源

	m := a.SnapshotMetrics(context.Background(), nil)[0]
	if m.PE != nil || m.PB != nil || m.DividendYield != nil || m.PEPercentile != nil {
		t.Errorf("valuation should be nil: %+v", m)
	}
	if m.PricePercentile == nil {
		t.Error("price percentile should still compute")
	}
	if len(m.Gaps) == 0 {
		t.Error("expected gaps recorded")
	}
}

func TestSnapshotMetrics_CryptoNoValuationGap(t *testing.T) {
	fake := &snapFake{
		quote:   map[string]*core.Quote{"BTC-USD": {Price: 65000, ChangePercent: 2.0}},
		history: map[string][]core.OHLCV{"BTC-USD": bars(60000, 62000, 64000)},
	}
	a := newSnapshotApp(t, fake, WatchlistItem{Symbol: "BTC-USD", Name: "Bitcoin", Market: "数字货币", Type: "加密货币"})

	m := a.SnapshotMetrics(context.Background(), nil)[0]
	if m.PE != nil || m.PEPercentile != nil {
		t.Errorf("crypto valuation should be nil: %+v", m)
	}
	for _, g := range m.Gaps {
		if strings.Contains(g, "valuation") || strings.Contains(g, "fundamental") {
			t.Errorf("crypto should not report valuation gap, got %q", g)
		}
	}
	if m.PricePercentile == nil {
		t.Error("price percentile should compute for crypto")
	}
}

func TestSnapshotMetrics_SymbolsFilter(t *testing.T) {
	fake := &snapFake{
		quote:   map[string]*core.Quote{"AAPL": {Price: 210}, "MSFT": {Price: 400}},
		history: map[string][]core.OHLCV{"AAPL": bars(200), "MSFT": bars(390)},
	}
	a := newSnapshotApp(t, fake,
		WatchlistItem{Symbol: "AAPL", Type: "股票"},
		WatchlistItem{Symbol: "MSFT", Type: "股票"},
	)
	ms := a.SnapshotMetrics(context.Background(), []string{"MSFT"})
	if len(ms) != 1 || ms[0].Symbol != "MSFT" {
		t.Fatalf("filter failed: %+v", ms)
	}
}

func TestSnapshotMetrics_PanicIsolated(t *testing.T) {
	fake := &snapFake{
		panicOn: "BAD",
		quote:   map[string]*core.Quote{"GOOD": {Price: 10}},
		history: map[string][]core.OHLCV{"GOOD": bars(9, 10)},
	}
	a := newSnapshotApp(t, fake,
		WatchlistItem{Symbol: "BAD", Type: "股票"},
		WatchlistItem{Symbol: "GOOD", Type: "股票"},
	)
	ms := a.SnapshotMetrics(context.Background(), nil)
	if len(ms) != 2 {
		t.Fatalf("want 2 items, got %d", len(ms))
	}
	var bad, good SymbolMetrics
	for _, m := range ms {
		if m.Symbol == "BAD" {
			bad = m
		} else {
			good = m
		}
	}
	if len(bad.Gaps) == 0 {
		t.Error("panicked symbol should record a gap")
	}
	if good.Price != 10 {
		t.Errorf("healthy symbol should be unaffected: %+v", good)
	}
}

func TestSnapshotMetrics_AllCollectorsFail(t *testing.T) {
	fake := &snapFake{failQuotes: true}
	a := newSnapshotApp(t, fake, WatchlistItem{Symbol: "AAPL", Type: "股票"})
	m := a.SnapshotMetrics(context.Background(), nil)[0]
	if m.Price != 0 || m.PricePercentile != nil || len(m.Gaps) == 0 {
		t.Errorf("expected empty metrics with gaps: %+v", m)
	}
}

// —— AD-8 增补用例 ——

// TestSnapshotMetrics_OrderPreservedConcurrent (B4): ≥3 标的在并发(Workers=4)下
// 结果仍逐位对齐 watchlist 顺序。
func TestSnapshotMetrics_OrderPreservedConcurrent(t *testing.T) {
	order := []string{"AAA", "BBB", "CCC", "DDD", "EEE"}
	fake := &snapFake{quote: map[string]*core.Quote{}, history: map[string][]core.OHLCV{}}
	items := make([]WatchlistItem, len(order))
	for i, s := range order {
		fake.quote[s] = &core.Quote{Price: float64(i + 1)}
		fake.history[s] = bars(float64(i + 1))
		items[i] = WatchlistItem{Symbol: s, Type: "股票"}
	}
	a := newSnapshotApp(t, fake, items...)
	if a.cfg.Analysis.Workers < 2 {
		t.Fatalf("test needs concurrent workers, got %d", a.cfg.Analysis.Workers)
	}
	ms := a.SnapshotMetrics(context.Background(), nil)
	if len(ms) != len(order) {
		t.Fatalf("want %d items, got %d", len(order), len(ms))
	}
	for i, want := range order {
		if ms[i].Symbol != want {
			t.Errorf("position %d: got %q, want %q", i, ms[i].Symbol, want)
		}
	}
}

// TestSnapshotMetrics_ConcurrentEquivalence (B3): Workers=1 与 Workers=4 结果切片
// 逐元素相等。配合 `go test -race` 验证无 data race。
func TestSnapshotMetrics_ConcurrentEquivalence(t *testing.T) {
	build := func(workers int) []SymbolMetrics {
		fake := &snapFake{quote: map[string]*core.Quote{}, history: map[string][]core.OHLCV{}}
		var items []WatchlistItem
		for i := 0; i < 6; i++ {
			s := fmt.Sprintf("S%d", i)
			fake.quote[s] = &core.Quote{Price: float64(10 * (i + 1)), ChangePercent: float64(i)}
			fake.history[s] = bars(float64(i+1), float64(i+2), float64(i+3))
			items = append(items, WatchlistItem{Symbol: s, Name: s, Type: "股票"})
		}
		cfg := config.Defaults()
		cfg.Analysis.Workers = workers
		a := New(cfg, zap.NewNop())
		a.RegisterCollector(fake)
		for _, it := range items {
			a.AddToWatchlistWithDetails(it.Symbol, it.Name, it.Market, it.Type, it.Strategies)
		}
		return a.SnapshotMetrics(context.Background(), nil)
	}
	serial := build(1)
	parallel := build(4)
	if len(serial) != len(parallel) {
		t.Fatalf("length mismatch: %d vs %d", len(serial), len(parallel))
	}
	for i := range serial {
		if serial[i].Symbol != parallel[i].Symbol ||
			serial[i].Price != parallel[i].Price ||
			serial[i].ChangePct != parallel[i].ChangePct ||
			!eqF64Ptr(serial[i].PricePercentile, parallel[i].PricePercentile) ||
			!eqF64Ptr(serial[i].PEPercentile, parallel[i].PEPercentile) {
			t.Errorf("position %d differs: serial=%+v parallel=%+v", i, serial[i], parallel[i])
		}
	}
}

func eqF64Ptr(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// TestSnapshotMetrics_PricePercentileExtremes (B5): PercentileRank 严格小于语义。
// current 大于全部收盘→100；current==最小收盘→0；current 落序列内部→中间值。
func TestSnapshotMetrics_PricePercentileExtremes(t *testing.T) {
	cases := []struct {
		name    string
		current float64
		closes  []float64
		want    float64
	}{
		{"above all", 200, []float64{100, 110, 120, 130}, 100},
		{"equals min", 100, []float64{100, 110, 120, 130}, 0},   // 严格小于:无一 <100
		{"internal", 115, []float64{100, 110, 120, 130}, 50},    // 100,110 < 115 → 2/4
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &snapFake{
				quote:   map[string]*core.Quote{"X": {Price: tc.current}},
				history: map[string][]core.OHLCV{"X": bars(tc.closes...)},
			}
			a := newSnapshotApp(t, fake, WatchlistItem{Symbol: "X", Type: "股票"})
			m := a.SnapshotMetrics(context.Background(), nil)[0]
			if m.PricePercentile == nil {
				t.Fatalf("price percentile nil for %s", tc.name)
			}
			if *m.PricePercentile != tc.want {
				t.Errorf("%s: price percentile = %v, want %v", tc.name, *m.PricePercentile, tc.want)
			}
		})
	}
}
