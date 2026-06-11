package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/collector"
	"github.com/newthinker/atlas/internal/config"
	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/meta"
	"github.com/newthinker/atlas/internal/notifier"
	"github.com/newthinker/atlas/internal/strategy"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// fakeStrategy is a configurable strategy stub for asset-type binding and
// history-window tests (mockStrategy has fixed RequiredData).
type fakeStrategy struct {
	name         string
	assetTypes   []core.AssetType
	priceHistory int
	fundamentals bool
	signals      []core.Signal

	mu             sync.Mutex
	gotFundamental *core.Fundamental // captured from the last Analyze call
}

func (f *fakeStrategy) Name() string       { return f.name }
func (f *fakeStrategy) Description() string { return "fake" }
func (f *fakeStrategy) RequiredData() strategy.DataRequirements {
	return strategy.DataRequirements{
		AssetTypes:   f.assetTypes,
		PriceHistory: f.priceHistory,
		Fundamentals: f.fundamentals,
	}
}
func (f *fakeStrategy) Init(cfg strategy.Config) error { return nil }
func (f *fakeStrategy) Analyze(ctx strategy.AnalysisContext) ([]core.Signal, error) {
	f.mu.Lock()
	f.gotFundamental = ctx.Fundamental
	f.mu.Unlock()
	out := make([]core.Signal, len(f.signals))
	copy(out, f.signals)
	return out, nil
}

func (f *fakeStrategy) capturedFundamental() *core.Fundamental {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.gotFundamental
}

type mockCollector struct {
	name       string
	history    []core.OHLCV
	fetchCount *int // optional: incremented on each FetchHistory call
}

func (m *mockCollector) Name() string                    { return m.name }
func (m *mockCollector) SupportedMarkets() []core.Market { return []core.Market{core.MarketUS} }
func (m *mockCollector) Init(cfg collector.Config) error { return nil }
func (m *mockCollector) Start(ctx context.Context) error { return nil }
func (m *mockCollector) Stop() error                     { return nil }
func (m *mockCollector) FetchQuote(symbol string) (*core.Quote, error) {
	return &core.Quote{Symbol: symbol, Price: 100}, nil
}
func (m *mockCollector) FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	if m.fetchCount != nil {
		*m.fetchCount++
	}
	return m.history, nil
}

type mockStrategy struct {
	name    string
	signals []core.Signal
}

func (m *mockStrategy) Name() string        { return m.name }
func (m *mockStrategy) Description() string { return "mock" }
func (m *mockStrategy) RequiredData() strategy.DataRequirements {
	return strategy.DataRequirements{PriceHistory: 10}
}
func (m *mockStrategy) Init(cfg strategy.Config) error { return nil }
func (m *mockStrategy) Analyze(ctx strategy.AnalysisContext) ([]core.Signal, error) {
	// Return a fresh copy: the engine stamps Strategy onto each element, so a
	// real strategy hands back freshly computed signals. Sharing one backing
	// array across concurrent analyses would be a mock artifact, not a bug.
	out := make([]core.Signal, len(m.signals))
	copy(out, m.signals)
	return out, nil
}

type mockNotifier struct {
	name string
	mu   sync.Mutex
	recv []core.Signal
}

func (m *mockNotifier) Name() string                   { return m.name }
func (m *mockNotifier) Init(cfg notifier.Config) error { return nil }
func (m *mockNotifier) Send(signal core.Signal) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recv = append(m.recv, signal)
	return nil
}
func (m *mockNotifier) SendBatch(signals []core.Signal) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recv = append(m.recv, signals...)
	return nil
}

// received returns a copy of the signals delivered so far, safe for concurrent
// routing (the parallel analysis path calls Send from multiple goroutines).
func (m *mockNotifier) received() []core.Signal {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]core.Signal, len(m.recv))
	copy(out, m.recv)
	return out
}

func TestApp_New(t *testing.T) {
	cfg := &config.Config{}
	app := New(cfg, nil)

	if app == nil {
		t.Fatal("expected non-nil app")
	}

	stats := app.GetStats()
	if stats["running"].(bool) {
		t.Error("new app should not be running")
	}
}

func TestApp_RegisterComponents(t *testing.T) {
	cfg := &config.Config{}
	app := New(cfg, nil)

	// Register collector
	app.RegisterCollector(&mockCollector{name: "test"})

	// Register strategy
	app.RegisterStrategy(&mockStrategy{name: "test"})

	// Register notifier
	err := app.RegisterNotifier(&mockNotifier{name: "test"})
	if err != nil {
		t.Errorf("failed to register notifier: %v", err)
	}

	stats := app.GetStats()
	if stats["collectors"].(int) != 1 {
		t.Errorf("expected 1 collector, got %d", stats["collectors"].(int))
	}
	if stats["strategies"].(int) != 1 {
		t.Errorf("expected 1 strategy, got %d", stats["strategies"].(int))
	}
	if stats["notifiers"].(int) != 1 {
		t.Errorf("expected 1 notifier, got %d", stats["notifiers"].(int))
	}
}

func TestApp_SetWatchlist(t *testing.T) {
	cfg := &config.Config{}
	app := New(cfg, nil)

	app.SetWatchlist([]string{"AAPL", "GOOG", "TSLA"})

	stats := app.GetStats()
	if stats["watchlist"].(int) != 3 {
		t.Errorf("expected 3 symbols in watchlist, got %d", stats["watchlist"].(int))
	}
}

func TestApp_RunOnce(t *testing.T) {
	cfg := &config.Config{}
	app := New(cfg, nil)

	// Create mock data with enough for MA crossover
	history := make([]core.OHLCV, 10)
	for i := 0; i < 10; i++ {
		history[i] = core.OHLCV{
			Symbol: "TEST",
			Close:  float64(100 + i),
			Time:   time.Now().Add(time.Duration(-10+i) * 24 * time.Hour),
		}
	}

	mockColl := &mockCollector{name: "mock", history: history}
	mockStrat := &mockStrategy{
		name: "mock",
		signals: []core.Signal{
			{Symbol: "TEST", Action: core.ActionBuy, Confidence: 0.8},
		},
	}
	mockNoti := &mockNotifier{name: "mock"}

	app.RegisterCollector(mockColl)
	app.RegisterStrategy(mockStrat)
	app.RegisterNotifier(mockNoti)
	app.SetWatchlist([]string{"TEST"})

	ctx := context.Background()
	app.RunOnce(ctx)

	// Signal should have been routed to notifier
	if got := mockNoti.received(); len(got) != 1 {
		t.Errorf("expected 1 signal, got %d", len(got))
	}
}

func TestApp_PerSymbolStrategies(t *testing.T) {
	cfg := &config.Config{}
	app := New(cfg, nil)

	history := make([]core.OHLCV, 10)
	for i := range history {
		history[i] = core.OHLCV{Symbol: "TEST", Close: float64(100 + i), Time: time.Now()}
	}

	app.RegisterCollector(&mockCollector{name: "mock", history: history})
	app.RegisterStrategy(&mockStrategy{name: "s1", signals: []core.Signal{{Symbol: "TEST", Action: core.ActionBuy, Confidence: 0.8}}})
	app.RegisterStrategy(&mockStrategy{name: "s2", signals: []core.Signal{{Symbol: "TEST", Action: core.ActionBuy, Confidence: 0.8}}})

	noti := &mockNotifier{name: "mock"}
	app.RegisterNotifier(noti)

	// Only s1 is selected for this symbol.
	app.AddToWatchlistWithDetails("TEST", "Test", "", "", []string{"s1"})

	app.RunOnce(context.Background())

	got := noti.received()
	if len(got) != 1 {
		t.Fatalf("expected 1 signal (only s1), got %d", len(got))
	}
	if got[0].Strategy != "s1" {
		t.Errorf("expected signal from s1, got %q", got[0].Strategy)
	}
}

func TestApp_PreferredCollectorTriedFirst(t *testing.T) {
	cfg := &config.Config{}
	app := New(cfg, nil)

	history := make([]core.OHLCV, 10)
	for i := range history {
		history[i] = core.OHLCV{Symbol: "600519.SH", Close: float64(100 + i), Time: time.Now()}
	}

	var emCount, yhCount int
	// eastmoney has data; yahoo would be the fallback and should not be called.
	app.RegisterCollector(&mockCollector{name: "eastmoney", history: history, fetchCount: &emCount})
	app.RegisterCollector(&mockCollector{name: "yahoo", history: nil, fetchCount: &yhCount})
	app.RegisterStrategy(&mockStrategy{name: "s1", signals: []core.Signal{{Symbol: "600519.SH", Action: core.ActionBuy, Confidence: 0.8}}})
	app.RegisterNotifier(&mockNotifier{name: "mock"})

	app.AddToWatchlistWithDetails("600519.SH", "Moutai", "", "", nil)

	app.RunOnce(context.Background())

	if emCount != 1 {
		t.Errorf("expected eastmoney to be fetched once, got %d", emCount)
	}
	if yhCount != 0 {
		t.Errorf("expected yahoo not to be fetched, got %d", yhCount)
	}
}

func TestApp_StartStop(t *testing.T) {
	cfg := &config.Config{}
	app := New(cfg, nil)
	app.SetInterval(100 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan error)
	go func() {
		done <- app.Start(ctx)
	}()

	// Wait for timeout
	err := <-done
	if err != context.DeadlineExceeded {
		t.Errorf("expected deadline exceeded, got %v", err)
	}

	stats := app.GetStats()
	if stats["running"].(bool) {
		t.Error("app should not be running after stop")
	}
}

func TestApp_CannotStartTwice(t *testing.T) {
	cfg := &config.Config{}
	app := New(cfg, nil)
	app.SetInterval(1 * time.Second)

	ctx, cancel := context.WithCancel(context.Background())

	// Start in background
	go app.Start(ctx)

	// Give it time to start
	time.Sleep(50 * time.Millisecond)

	// Try to start again
	err := app.Start(context.Background())
	if err == nil {
		t.Error("expected error when starting twice")
	}

	cancel()
	time.Sleep(50 * time.Millisecond)
}

func TestApp_NoCollectorsNoError(t *testing.T) {
	cfg := &config.Config{}
	app := New(cfg, nil)
	app.SetWatchlist([]string{"TEST"})

	// Should not panic even without collectors
	ctx := context.Background()
	app.RunOnce(ctx)
}

func TestApp_EmptyWatchlistNoError(t *testing.T) {
	cfg := &config.Config{}
	app := New(cfg, nil)

	// Should not panic with empty watchlist
	ctx := context.Background()
	app.RunOnce(ctx)
}

// --- TASK-002: SignalExecutor 接线点测试 ---
// Context Checkpoint: done_criteria → test mapping
// functional[0]     "设 executor 后每个被路由信号触发一次 SubmitSignal" → TestApp_Executor_SubmitsRoutedSignals
// functional[1]     "未调 SetExecutor 时行为不变"                       → 现有 app 测试全过 + TestApp_Executor_NilByDefault
// boundary[0]       "本周期无信号时不调 SubmitSignal"                   → TestApp_Executor_NoSignalsNoSubmit
// error_handling[0] "SubmitSignal 返错记日志不中断后续"                 → TestApp_Executor_ErrorDoesNotStop
// non_functional[0] "SetExecutor 与分析循环并发 -race 无竞争"           → TestApp_Executor_ConcurrentSetAndRun

type mockExecutor struct {
	mu       sync.Mutex
	received []core.Signal
	err      error
}

func (m *mockExecutor) SubmitSignal(ctx context.Context, sig core.Signal) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.received = append(m.received, sig)
	return m.err
}

func (m *mockExecutor) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.received)
}

func executorTestHistory() []core.OHLCV {
	history := make([]core.OHLCV, 10)
	for i := 0; i < 10; i++ {
		history[i] = core.OHLCV{
			Symbol: "TEST",
			Close:  float64(100 + i),
			Time:   time.Now().Add(time.Duration(-10+i) * 24 * time.Hour),
		}
	}
	return history
}

func TestApp_Executor_SubmitsRoutedSignals(t *testing.T) {
	app := New(&config.Config{}, nil)
	app.RegisterCollector(&mockCollector{name: "mock", history: executorTestHistory()})
	app.RegisterStrategy(&mockStrategy{name: "mock", signals: []core.Signal{
		{Symbol: "TEST", Action: core.ActionBuy, Confidence: 0.8},
	}})
	app.SetWatchlist([]string{"TEST"})

	exec := &mockExecutor{}
	app.SetExecutor(exec)

	app.RunOnce(context.Background())

	if got := exec.count(); got != 1 {
		t.Fatalf("expected 1 SubmitSignal call, got %d", got)
	}
	if exec.received[0].Symbol != "TEST" || exec.received[0].Action != core.ActionBuy {
		t.Errorf("unexpected submitted signal: %+v", exec.received[0])
	}
}

func TestApp_Executor_NilByDefault(t *testing.T) {
	// Without SetExecutor the cycle must behave exactly as before (no panic).
	app := New(&config.Config{}, nil)
	app.RegisterCollector(&mockCollector{name: "mock", history: executorTestHistory()})
	app.RegisterStrategy(&mockStrategy{name: "mock", signals: []core.Signal{
		{Symbol: "TEST", Action: core.ActionBuy, Confidence: 0.8},
	}})
	mockNoti := &mockNotifier{name: "mock"}
	app.RegisterNotifier(mockNoti)
	app.SetWatchlist([]string{"TEST"})

	app.RunOnce(context.Background())

	if got := mockNoti.received(); len(got) != 1 {
		t.Errorf("expected 1 routed signal, got %d", len(got))
	}
}

func TestApp_Executor_NoSignalsNoSubmit(t *testing.T) {
	app := New(&config.Config{}, nil)
	app.RegisterCollector(&mockCollector{name: "mock", history: executorTestHistory()})
	app.RegisterStrategy(&mockStrategy{name: "mock", signals: nil}) // no signals
	app.SetWatchlist([]string{"TEST"})

	exec := &mockExecutor{}
	app.SetExecutor(exec)

	app.RunOnce(context.Background())

	if got := exec.count(); got != 0 {
		t.Fatalf("expected 0 SubmitSignal calls when no signals, got %d", got)
	}
}

var errSubmitBoom = fmt.Errorf("submit boom")

// perSymbolStrategy emits exactly one routable signal for whichever symbol is
// being analyzed, so distinct symbols produce distinct (non-cooldown-colliding)
// routed signals.
type perSymbolStrategy struct{}

func (perSymbolStrategy) Name() string        { return "per-symbol" }
func (perSymbolStrategy) Description() string { return "per-symbol" }
func (perSymbolStrategy) RequiredData() strategy.DataRequirements {
	return strategy.DataRequirements{PriceHistory: 10}
}
func (perSymbolStrategy) Init(cfg strategy.Config) error { return nil }
func (perSymbolStrategy) Analyze(ctx strategy.AnalysisContext) ([]core.Signal, error) {
	return []core.Signal{{Symbol: ctx.Symbol, Action: core.ActionBuy, Confidence: 0.8}}, nil
}

func TestApp_Executor_ErrorDoesNotStop(t *testing.T) {
	app := New(&config.Config{}, nil)
	app.RegisterCollector(&mockCollector{name: "mock", history: executorTestHistory()})
	// Distinct symbols each route their own signal; an erroring executor must
	// not stop subsequent symbols from being submitted.
	app.RegisterStrategy(perSymbolStrategy{})
	app.SetWatchlist([]string{"AAA", "BBB", "CCC"})

	exec := &mockExecutor{err: errSubmitBoom}
	app.SetExecutor(exec)

	app.RunOnce(context.Background())

	if got := exec.count(); got != 3 {
		t.Fatalf("expected 3 SubmitSignal calls despite errors, got %d", got)
	}
}

// W2 regression: a signal suppressed by the router (cooldown) must NOT be
// submitted for execution, otherwise a deduplicated signal still places an order.
func TestApp_Executor_CooldownSuppressedNotSubmitted(t *testing.T) {
	app := New(&config.Config{}, nil)
	app.RegisterCollector(&mockCollector{name: "mock", history: executorTestHistory()})
	// Two signals on the SAME symbol: the first routes (and sets cooldown), the
	// second is suppressed by cooldown and must not reach the executor.
	app.RegisterStrategy(&mockStrategy{name: "mock", signals: []core.Signal{
		{Symbol: "TEST", Action: core.ActionBuy, Confidence: 0.8},
		{Symbol: "TEST", Action: core.ActionStrongBuy, Confidence: 0.9},
	}})
	noti := &mockNotifier{name: "mock"}
	app.RegisterNotifier(noti)
	app.SetWatchlist([]string{"TEST"})

	exec := &mockExecutor{}
	app.SetExecutor(exec)

	app.RunOnce(context.Background())

	if got := exec.count(); got != 1 {
		t.Fatalf("cooldown-suppressed signal must not be submitted: want 1 SubmitSignal, got %d", got)
	}
	// Sanity: routing itself also suppressed the second signal.
	if got := len(noti.received()); got != 1 {
		t.Fatalf("expected 1 routed signal, got %d", got)
	}
}

func TestApp_Executor_ConcurrentSetAndRun(t *testing.T) {
	app := New(&config.Config{}, nil)
	app.RegisterCollector(&mockCollector{name: "mock", history: executorTestHistory()})
	app.RegisterStrategy(&mockStrategy{name: "mock", signals: []core.Signal{
		{Symbol: "TEST", Action: core.ActionBuy, Confidence: 0.8},
	}})
	app.SetWatchlist([]string{"TEST"})

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			app.SetExecutor(&mockExecutor{})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			app.RunOnce(context.Background())
		}
	}()
	wg.Wait()
}

func TestApp_WatchlistManagement(t *testing.T) {
	app := New(&config.Config{}, nil)

	app.AddToWatchlist("AAPL")
	app.AddToWatchlist("AAPL") // duplicate ignored
	app.AddToWatchlistWithDetails("BTC-USD", "Bitcoin", "", "", []string{"ma"})

	if got := app.GetWatchlist(); len(got) != 2 {
		t.Fatalf("expected 2 symbols, got %d (%v)", len(got), got)
	}

	items := app.GetWatchlistItems()
	var btc *WatchlistItem
	for i := range items {
		if items[i].Symbol == "BTC-USD" {
			btc = &items[i]
		}
	}
	if btc == nil {
		t.Fatal("BTC-USD not found in watchlist items")
	}
	if btc.Market != MarketCrypto || btc.Type != TypeCrypto {
		t.Errorf("expected auto-detected crypto market/type, got %q/%q", btc.Market, btc.Type)
	}

	if !app.RemoveFromWatchlist("AAPL") {
		t.Error("expected RemoveFromWatchlist to return true for existing symbol")
	}
	if app.RemoveFromWatchlist("MISSING") {
		t.Error("expected RemoveFromWatchlist to return false for missing symbol")
	}
	if len(app.GetWatchlist()) != 1 {
		t.Errorf("expected 1 symbol after removal, got %d", len(app.GetWatchlist()))
	}
}

func TestApp_DetectMarketAndType(t *testing.T) {
	cases := []struct {
		symbol     string
		wantMarket string
		wantType   string
	}{
		{"600000.SH", MarketAShare, TypeStock},
		{"000001.SZ", MarketAShare, TypeStock},
		{"00700.HK", MarketHShare, TypeStock},
		{"BTC-USD", MarketCrypto, TypeCrypto},
		{"ETH-USDT", MarketCrypto, TypeCrypto},
		{"AAPL", MarketUS, TypeStock},
	}
	for _, c := range cases {
		if got := DetectMarket(c.symbol); got != c.wantMarket {
			t.Errorf("DetectMarket(%q)=%q, want %q", c.symbol, got, c.wantMarket)
		}
		if got := DetectType(c.symbol); got != c.wantType {
			t.Errorf("DetectType(%q)=%q, want %q", c.symbol, got, c.wantType)
		}
	}
}

func TestApp_SettersAndAccessors(t *testing.T) {
	app := New(&config.Config{}, nil)

	// These setters must not panic with nil/zero values.
	app.SetSignalStore(nil)
	app.SetArbitrator(nil)
	app.RegisterCollector(&mockCollector{name: "c1"})

	if got := app.GetCollectors(); len(got) != 1 {
		t.Errorf("expected 1 collector, got %d", len(got))
	}

	// Stop before Start must be a no-op (cancel is nil).
	app.Stop()
}

// --- TASK-005: worker pool 并行化 + 仲裁超时测试 ---
// Context Checkpoint: done_criteria → test mapping
// functional[0]     "workers>1 全标的处理且确实并行"   → TestApp_ParallelWorkers_ProcessesAllConcurrently
// functional[1]     "workers<=1 走串行路径"            → TestApp_SerialWhenWorkersLE1 + 现有测试
// functional[2]     "arbitrate WithTimeout 超时返回原信号" → TestApp_ArbitrateTimeout_ReturnsOriginal
// boundary[0]       "空 watchlist 不 panic; ctx 取消尽快返回" → TestApp_Parallel_EmptyWatchlist / TestApp_Parallel_CtxCancelled
// error_handling[0] "单标的 panic 不影响其他标的、不退进程"  → TestApp_Parallel_PanicIsolated
// error_handling[1] "仲裁超时记 warning 原信号继续路由"      → TestApp_ArbitrateTimeout_ReturnsOriginal

// concurrencyCollector tracks how many FetchHistory calls run in parallel.
type concurrencyCollector struct {
	delay     time.Duration
	active    int32
	maxActive int32
	calls     int32
}

func (c *concurrencyCollector) Name() string                    { return "concurrency" }
func (c *concurrencyCollector) SupportedMarkets() []core.Market { return []core.Market{core.MarketUS} }
func (c *concurrencyCollector) Init(cfg collector.Config) error { return nil }
func (c *concurrencyCollector) Start(ctx context.Context) error { return nil }
func (c *concurrencyCollector) Stop() error                     { return nil }
func (c *concurrencyCollector) FetchQuote(symbol string) (*core.Quote, error) {
	return &core.Quote{Symbol: symbol, Price: 100}, nil
}
func (c *concurrencyCollector) FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	atomic.AddInt32(&c.calls, 1)
	cur := atomic.AddInt32(&c.active, 1)
	for {
		max := atomic.LoadInt32(&c.maxActive)
		if cur <= max || atomic.CompareAndSwapInt32(&c.maxActive, max, cur) {
			break
		}
	}
	time.Sleep(c.delay)
	atomic.AddInt32(&c.active, -1)
	return executorTestHistory(), nil
}

func symbolsN(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("SYM%d", i)
	}
	return out
}

func newParallelApp(workers int, coll collector.Collector) *App {
	cfg := &config.Config{}
	cfg.Analysis.Workers = workers
	app := New(cfg, nil)
	app.RegisterCollector(coll)
	app.RegisterStrategy(&mockStrategy{name: "s", signals: []core.Signal{
		{Symbol: "X", Action: core.ActionBuy, Confidence: 0.8},
	}})
	return app
}

func TestApp_ParallelWorkers_ProcessesAllConcurrently(t *testing.T) {
	coll := &concurrencyCollector{delay: 40 * time.Millisecond}
	app := newParallelApp(4, coll)
	app.SetWatchlist(symbolsN(8))

	app.RunOnce(context.Background())

	if got := atomic.LoadInt32(&coll.calls); got != 8 {
		t.Errorf("expected all 8 symbols processed, got %d", got)
	}
	if got := atomic.LoadInt32(&coll.maxActive); got < 2 {
		t.Errorf("expected concurrent execution (maxActive>=2), got %d", got)
	}
}

func TestApp_SerialWhenWorkersLE1(t *testing.T) {
	coll := &concurrencyCollector{delay: 10 * time.Millisecond}
	app := newParallelApp(1, coll)
	app.SetWatchlist(symbolsN(5))

	app.RunOnce(context.Background())

	if got := atomic.LoadInt32(&coll.calls); got != 5 {
		t.Errorf("expected 5 symbols processed, got %d", got)
	}
	if got := atomic.LoadInt32(&coll.maxActive); got != 1 {
		t.Errorf("serial path must never run concurrently, maxActive=%d", got)
	}
}

func TestApp_Parallel_EmptyWatchlist(t *testing.T) {
	coll := &concurrencyCollector{delay: time.Millisecond}
	app := newParallelApp(4, coll)
	// no watchlist
	app.RunOnce(context.Background()) // must not panic
	if got := atomic.LoadInt32(&coll.calls); got != 0 {
		t.Errorf("expected 0 calls for empty watchlist, got %d", got)
	}
}

func TestApp_Parallel_CtxCancelled(t *testing.T) {
	coll := &concurrencyCollector{delay: 50 * time.Millisecond}
	app := newParallelApp(4, coll)
	app.SetWatchlist(symbolsN(20))

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	start := time.Now()
	app.RunOnce(ctx)
	elapsed := time.Since(start)

	if elapsed > 500*time.Millisecond {
		t.Errorf("cancelled cycle should return promptly, took %v", elapsed)
	}
	if got := atomic.LoadInt32(&coll.calls); got == 20 {
		t.Errorf("cancelled cycle should not dispatch all symbols, got %d", got)
	}
}

// panicStrategy panics for one symbol to verify worker-level isolation.
type panicStrategy struct{ panicSymbol string }

func (p *panicStrategy) Name() string        { return "panic" }
func (p *panicStrategy) Description() string { return "panic" }
func (p *panicStrategy) RequiredData() strategy.DataRequirements {
	return strategy.DataRequirements{PriceHistory: 10}
}
func (p *panicStrategy) Init(cfg strategy.Config) error { return nil }
func (p *panicStrategy) Analyze(ctx strategy.AnalysisContext) ([]core.Signal, error) {
	if ctx.Symbol == p.panicSymbol {
		panic("boom in " + ctx.Symbol)
	}
	return []core.Signal{{Symbol: ctx.Symbol, Action: core.ActionBuy, Confidence: 0.8}}, nil
}

func TestApp_Parallel_PanicIsolated(t *testing.T) {
	cfg := &config.Config{}
	cfg.Analysis.Workers = 4
	app := New(cfg, nil)
	app.RegisterCollector(&mockCollector{name: "mock", history: executorTestHistory()})
	app.RegisterStrategy(&panicStrategy{panicSymbol: "BOOM"})
	noti := &mockNotifier{name: "mock"}
	app.RegisterNotifier(noti)
	app.SetWatchlist([]string{"BOOM", "OK1", "OK2"})

	app.RunOnce(context.Background()) // must not crash the process

	got := noti.received()
	if len(got) != 2 {
		t.Fatalf("expected 2 signals from non-panicking symbols, got %d", len(got))
	}
	for _, s := range got {
		if s.Symbol == "BOOM" {
			t.Errorf("did not expect a signal from panicking symbol")
		}
	}
}

// slowArbitrator blocks until its context is cancelled, simulating a slow LLM.
type slowArbitrator struct{ called int32 }

func (s *slowArbitrator) Arbitrate(ctx context.Context, req meta.ArbitrationRequest) (*meta.ArbitrationResult, error) {
	atomic.AddInt32(&s.called, 1)
	select {
	case <-time.After(5 * time.Second):
		return &meta.ArbitrationResult{Decision: core.ActionSell, Confidence: 0.99}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestApp_ArbitrateTimeout_ReturnsOriginal(t *testing.T) {
	cfg := &config.Config{}
	cfg.Meta.Arbitrator.Timeout = 50 * time.Millisecond
	app := New(cfg, nil)
	app.RegisterCollector(&mockCollector{name: "mock", history: executorTestHistory()})
	// Two conflicting signals trigger arbitration (len >= 2).
	app.RegisterStrategy(&mockStrategy{name: "s", signals: []core.Signal{
		{Symbol: "TEST", Action: core.ActionBuy, Confidence: 0.8, Strategy: "a"},
		{Symbol: "TEST", Action: core.ActionSell, Confidence: 0.7, Strategy: "b"},
	}})
	noti := &mockNotifier{name: "mock"}
	app.RegisterNotifier(noti)
	app.SetWatchlist([]string{"TEST"})

	arb := &slowArbitrator{}
	app.setArbitratorClient(arb)

	start := time.Now()
	app.RunOnce(context.Background())
	elapsed := time.Since(start)

	if elapsed > 2*time.Second {
		t.Errorf("arbitration must time out quickly, took %v", elapsed)
	}
	if atomic.LoadInt32(&arb.called) != 1 {
		t.Errorf("expected arbitrator to be invoked once, got %d", arb.called)
	}
	// On timeout we degrade to the original signals. The router's per-symbol
	// cooldown lets only the first through, but the key assertion is that it is
	// an ORIGINAL signal, never the arbitrated "meta_arbitrator" decision.
	got := noti.received()
	if len(got) == 0 {
		t.Fatal("expected at least one original signal routed on timeout, got 0")
	}
	for _, s := range got {
		if s.Strategy == "meta_arbitrator" {
			t.Errorf("timed-out arbitration must not produce an arbitrated signal: %+v", s)
		}
	}
}

// okArbitrator resolves immediately to a fixed decision (no price of its own).
type okArbitrator struct{ decision core.Action }

func (o *okArbitrator) Arbitrate(ctx context.Context, req meta.ArbitrationRequest) (*meta.ArbitrationResult, error) {
	return &meta.ArbitrationResult{Decision: o.decision, Confidence: 0.9, Reasoning: "test"}, nil
}

// TestApp_ArbitrateSignalIsPriced guards QA W1 / CARRYOVER I3: the synthesized
// meta_arbitrator decision must carry a reference price (from the conflicting
// inputs) so a real executor does not reject it as an unpriced (Price=0) order.
func TestApp_ArbitrateSignalIsPriced(t *testing.T) {
	app := New(&config.Config{}, zap.NewNop())
	app.setArbitratorClient(&okArbitrator{decision: core.ActionSell})

	conflicting := []core.Signal{
		{Symbol: "TEST", Action: core.ActionBuy, Confidence: 0.8, Price: 123.45, Strategy: "a"},
		{Symbol: "TEST", Action: core.ActionSell, Confidence: 0.7, Price: 123.45, Strategy: "b"},
	}
	out := app.arbitrate(context.Background(), "TEST", conflicting)

	if len(out) != 1 || out[0].Strategy != "meta_arbitrator" {
		t.Fatalf("expected one arbitrated signal, got %+v", out)
	}
	if out[0].Price <= 0 {
		t.Errorf("arbitrated signal must carry a positive reference price, got %v", out[0].Price)
	}
	if out[0].Price != 123.45 {
		t.Errorf("arbitrated price = %v, want reference 123.45 from conflicting signals", out[0].Price)
	}
}

func TestReferencePrice(t *testing.T) {
	if got := referencePrice([]core.Signal{{Price: 0}, {Price: 50}, {Price: 99}}); got != 50 {
		t.Errorf("referencePrice = %v, want first positive 50", got)
	}
	if got := referencePrice([]core.Signal{{Price: 0}, {Price: 0}}); got != 0 {
		t.Errorf("referencePrice with no priced signal = %v, want 0", got)
	}
}

// --- TASK-010: asset-type detection, binding validation, dynamic window ---
//
// Context Checkpoint: done_criteria → test mapping
// functional[0] "DetectType index/commodity 全用例" → TestDetectType_IndexAndCommodity
// functional[1] "assetTypeOf 七映射 + DetectMarket(^HSI)→H股" → TestAssetTypeOf, TestDetectMarket_HSI
// functional[2] "effectiveStrategies 过滤+空不限+二次仅1 warning" → TestEffectiveStrategies_FiltersByAssetType
// functional[3] "historyWindowDays 5*252→≥1825, 无策略→365" → TestHistoryWindowDays
// boundary[0]   "Strategies非空但effective空→直接返回; 表外^绑定 warnOnce" →
//               TestAnalyzeSymbol_AllFilteredReturnsEarly, TestEffectiveStrategies_UnknownIndexWarnsOnce
// error_handling[0] "未注册策略名透传" → TestEffectiveStrategies_UnregisteredPassThrough

func TestDetectType_IndexAndCommodity(t *testing.T) {
	cases := []struct{ symbol, want string }{
		{"^GSPC", TypeIndex}, {"^HSI", TypeIndex},
		{"000300.SH", TypeIndex}, {"000001.SH", TypeIndex},
		{"000001.SZ", TypeStock}, {"600519.SH", TypeStock},
		{"GC=F", TypeFuture},
		{"BTC-USDT", TypeCrypto}, {"AAPL", TypeStock},
	}
	for _, c := range cases {
		if got := DetectType(c.symbol); got != c.want {
			t.Errorf("DetectType(%q) = %q, want %q", c.symbol, got, c.want)
		}
	}
}

func TestAssetTypeOf(t *testing.T) {
	cases := []struct {
		appType string
		want    core.AssetType
	}{
		{TypeStock, core.AssetStock}, {TypeIndex, core.AssetIndex},
		{TypeETF, core.AssetETF}, {TypeFund, core.AssetFund},
		{TypeFuture, core.AssetCommodity}, {TypeCrypto, core.AssetCrypto},
		{TypeBond, ""}, // 一期不支持 → 空值，装配层按"全跳过+warning"处理
	}
	for _, c := range cases {
		if got := assetTypeOf(c.appType); got != c.want {
			t.Errorf("assetTypeOf(%q) = %q, want %q", c.appType, got, c.want)
		}
	}
}

func TestDetectMarket_HSI(t *testing.T) {
	// ^HSI must report H股 so the UI market label stays consistent with the
	// collector.MarketForSymbol HK routing (plan Task 6).
	if got := DetectMarket("^HSI"); got != MarketHShare {
		t.Errorf("DetectMarket(^HSI) = %q, want %q", got, MarketHShare)
	}
}

func TestEffectiveStrategies_FiltersByAssetType(t *testing.T) {
	obsCore, logs := observer.New(zap.WarnLevel)
	app := New(&config.Config{}, zap.New(obsCore))
	app.RegisterStrategy(&fakeStrategy{name: "stock_only", assetTypes: []core.AssetType{core.AssetStock}})
	app.RegisterStrategy(&fakeStrategy{name: "all_assets"}) // AssetTypes 空 = 不限

	item := WatchlistItem{Symbol: "GC=F", Type: TypeFuture, Strategies: []string{"stock_only", "all_assets"}}
	got := app.effectiveStrategies(item)
	if len(got) != 1 || got[0] != "all_assets" {
		t.Fatalf("effectiveStrategies = %v, want [all_assets]", got)
	}

	// 二次调用不重复告警（warnOnce 去重）
	_ = app.effectiveStrategies(item)
	if n := logs.FilterMessage("strategy skipped: asset type not supported").Len(); n != 1 {
		t.Errorf("skip warning count = %d, want 1 (deduped)", n)
	}
}

func TestEffectiveStrategies_UnregisteredPassThrough(t *testing.T) {
	app := New(&config.Config{}, nil)
	item := WatchlistItem{Symbol: "AAPL", Type: TypeStock, Strategies: []string{"ghost"}}
	got := app.effectiveStrategies(item)
	if len(got) != 1 || got[0] != "ghost" {
		t.Errorf("effectiveStrategies = %v, want [ghost] (unregistered passed to engine error path)", got)
	}
}

func TestEffectiveStrategies_UnknownIndexWarnsOnce(t *testing.T) {
	obsCore, logs := observer.New(zap.WarnLevel)
	app := New(&config.Config{}, zap.New(obsCore))
	item := WatchlistItem{Symbol: "^N225", Type: TypeIndex, Strategies: []string{"ghost"}}
	app.effectiveStrategies(item)
	app.effectiveStrategies(item)
	if n := logs.FilterMessage("index symbol outside phase-1 list, market defaults to US").Len(); n != 1 {
		t.Errorf("unknown-index warning count = %d, want 1", n)
	}
}

func TestHistoryWindowDays(t *testing.T) {
	app := New(&config.Config{}, nil)
	app.RegisterStrategy(&fakeStrategy{name: "pp", priceHistory: 5 * 252})
	item := WatchlistItem{Symbol: "AAPL", Type: TypeStock, Strategies: []string{"pp"}}
	if d := app.historyWindowDays(item); d < 1825 { // 5y 交易日 → ≥5y 自然日
		t.Errorf("historyWindowDays = %d, want >= 1825", d)
	}
	// 无策略声明时回退 365（现状兼容）
	if d := app.historyWindowDays(WatchlistItem{Symbol: "X"}); d != 365 {
		t.Errorf("default window = %d, want 365", d)
	}
}

func TestAnalyzeSymbol_AllFilteredReturnsEarly(t *testing.T) {
	app := New(&config.Config{}, nil)
	notif := &mockNotifier{name: "n"}
	if err := app.RegisterNotifier(notif); err != nil {
		t.Fatalf("RegisterNotifier: %v", err)
	}
	app.RegisterCollector(&mockCollector{
		name:    "yahoo",
		history: []core.OHLCV{{Close: 100}, {Close: 101}},
	})
	// Bound only to a stock-only strategy that WOULD emit a buy signal; binding
	// it to a futures symbol must filter it out and skip analysis entirely.
	app.RegisterStrategy(&fakeStrategy{
		name:       "stock_only",
		assetTypes: []core.AssetType{core.AssetStock},
		signals:    []core.Signal{{Symbol: "GC=F", Action: core.ActionBuy, Confidence: 0.9}},
	})
	item := WatchlistItem{Symbol: "GC=F", Type: TypeFuture, Strategies: []string{"stock_only"}}
	app.analyzeSymbol(context.Background(), item)
	if n := len(notif.received()); n != 0 {
		t.Errorf("expected no signals when all bound strategies filtered, got %d", n)
	}
}

// --- TASK-011: PE percentile orchestration (buildFundamental fallback chain) ---
//
// Context Checkpoint: done_criteria → test mapping
// functional[0] "六路径表" + functional[1] "亏损 stubVal.calls==0" → TestBuildPEPercentile_Paths
// functional[2] "epsSrc 未配置→yahoo_not_configured"        → TestBuildFundamental_EPSNotConfigured
// boundary[0]   "商品/加密/基金→nil; 双 nil→-1+warnOnce 不 panic" → TestBuildFundamental_NilSourcesAndUnsupported
// error_handling[0] "理杏仁 fetch 失败→warnOnce+(-1)"        → TestBuildFundamental_LixingerFetchError

// stubVal/stubEPS are call-counting valuation/eps source stubs. The loss case
// asserts stubVal.calls == 0 (the no-fallback invariant).
type stubVal struct {
	pct   float64
	err   error
	calls int
}

func (s *stubVal) FetchValuationPercentile(string, int) (float64, error) {
	s.calls++
	return s.pct, s.err
}

type stubEPS struct {
	pts []core.EPSPoint
	err error
}

func (s *stubEPS) FetchEPSHistory(string, time.Time, time.Time) ([]core.EPSPoint, error) {
	return s.pts, s.err
}

// epsBase is the anchor for fixture dates. EPS points MUST predate every close
// bar, otherwise the step alignment finds no point, the PE series is empty and
// ReconstructPEPercentile returns ErrInsufficientEPS — making the "primary
// reconstruction" case fail in a baffling way (plan Task 13 load-bearing note).
var epsBase = time.Now().AddDate(-3, 0, 0)

// validEPS8 returns 8 positive quarterly EPS(TTM) points from epsBase, enough
// for MinEPSPoints with a positive current EPS.
func validEPS8() []core.EPSPoint {
	pts := make([]core.EPSPoint, 8)
	for i := range pts {
		pts[i] = core.EPSPoint{Date: epsBase.AddDate(0, 3*i, 0), EPS: 4 + 0.1*float64(i)}
	}
	return pts
}

// lossEPS returns 8 positive quarterly points plus a final negative one, so the
// current EPS(TTM) is non-positive → ErrNonPositiveEPS (real loss, no fallback).
func lossEPS() []core.EPSPoint {
	return append(validEPS8(), core.EPSPoint{Date: epsBase.AddDate(0, 24, 0), EPS: -1})
}

// sampleCloses returns n daily bars starting one month after epsBase (later than
// the first EPS point so every bar aligns to a point).
func sampleCloses(n int) []core.OHLCV {
	start := epsBase.AddDate(0, 1, 0)
	out := make([]core.OHLCV, n)
	for i := range out {
		out[i] = core.OHLCV{Close: 100 + float64(i%50), Time: start.AddDate(0, 0, i)}
	}
	return out
}

func TestBuildPEPercentile_Paths(t *testing.T) {
	cases := []struct {
		name       string
		symbol     string
		appType    string
		eps        []core.EPSPoint
		epsErr     error
		valPct     float64
		valErr     error
		wantPct    bool   // expect PEPercentile >= 0
		wantSource string // Source prefix
		wantNoVal  bool   // valuation source must NOT be consulted
	}{
		{"A股走理杏仁", "600519.SH", TypeStock, nil, nil, 23.4, nil, true, "lixinger_cvpos", false},
		{"美股主路径重建", "AAPL", TypeStock, validEPS8(), nil, 0, errors.New("unused"), true, "reconstructed", true},
		{"美股EPS不足→兜底成功", "AAPL", TypeStock, nil, nil, 41.2, nil, true, "lixinger_cvpos:", false},
		{"美股EPS不足→兜底也失败", "AAPL", TypeStock, nil, nil, -1, errors.New("no permission"), false, "", false},
		{"美股真实亏损→直接跳过不兜底", "LOSS", TypeStock, lossEPS(), nil, 99, nil, false, "", true},
		{"美/港指数走理杏仁", "^GSPC", TypeIndex, nil, nil, 88.0, nil, true, "lixinger_cvpos", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := New(&config.Config{}, zap.NewNop())
			sv := &stubVal{pct: c.valPct, err: c.valErr}
			a.SetValuationSources(sv, &stubEPS{pts: c.eps, err: c.epsErr})

			f := a.buildFundamental(c.symbol, c.appType, sampleCloses(700))

			gotAvail := f != nil && f.PEPercentile >= 0
			if gotAvail != c.wantPct {
				t.Fatalf("availability mismatch: got %v, want %v (f=%+v)", gotAvail, c.wantPct, f)
			}
			if c.wantPct && !strings.HasPrefix(f.Source, c.wantSource) {
				t.Errorf("Source = %q, want prefix %q", f.Source, c.wantSource)
			}
			if c.wantNoVal && sv.calls != 0 {
				t.Errorf("valuation source must not be consulted, but calls = %d", sv.calls)
			}
		})
	}
}

func TestBuildFundamental_EPSNotConfigured(t *testing.T) {
	// US stock with no EPS source but a working valuation source: fall straight
	// to lixinger with the yahoo_not_configured reason.
	a := New(&config.Config{}, zap.NewNop())
	a.SetValuationSources(&stubVal{pct: 55.0}, nil)
	f := a.buildFundamental("AAPL", TypeStock, sampleCloses(300))
	if f == nil || f.PEPercentile < 0 {
		t.Fatalf("expected available fundamental, got %+v", f)
	}
	if f.Source != "lixinger_cvpos:yahoo_not_configured" {
		t.Errorf("Source = %q, want lixinger_cvpos:yahoo_not_configured", f.Source)
	}
}

func TestBuildFundamental_NilSourcesAndUnsupported(t *testing.T) {
	a := New(&config.Config{}, zap.NewNop()) // no sources configured

	// Unsupported asset classes return nil regardless of sources.
	for _, tc := range []struct{ symbol, appType string }{
		{"GC=F", TypeFuture}, {"BTC-USDT", TypeCrypto}, {"510300.SH", TypeFund},
	} {
		if f := a.buildFundamental(tc.symbol, tc.appType, sampleCloses(10)); f != nil {
			t.Errorf("buildFundamental(%q,%q) = %+v, want nil", tc.symbol, tc.appType, f)
		}
	}

	// CN stock with both sources nil: unavailable fundamental, no panic.
	f := a.buildFundamental("600519.SH", TypeStock, sampleCloses(10))
	if f == nil || f.PEPercentile != -1 {
		t.Fatalf("expected PEPercentile=-1 fundamental, got %+v", f)
	}
	// US stock with both sources nil: unavailable, no panic.
	f = a.buildFundamental("AAPL", TypeStock, sampleCloses(10))
	if f == nil || f.PEPercentile != -1 {
		t.Fatalf("expected PEPercentile=-1 fundamental, got %+v", f)
	}
}

func TestBuildFundamental_LixingerFetchError(t *testing.T) {
	obsCore, logs := observer.New(zap.WarnLevel)
	a := New(&config.Config{}, zap.New(obsCore))
	a.SetValuationSources(&stubVal{pct: -1, err: errors.New("rate limited")}, nil)
	f := a.buildFundamental("600519.SH", TypeStock, sampleCloses(10))
	if f == nil || f.PEPercentile != -1 {
		t.Fatalf("expected PEPercentile=-1 on fetch error, got %+v", f)
	}
	if logs.Len() == 0 {
		t.Errorf("expected a warnOnce log on lixinger fetch failure")
	}
}

func TestAnalyzeSymbol_AssemblesFundamentalWhenNeeded(t *testing.T) {
	a := New(&config.Config{}, zap.NewNop())
	a.SetValuationSources(&stubVal{pct: 23.4}, nil)
	a.RegisterCollector(&mockCollector{name: "eastmoney", history: sampleCloses(300)})
	strat := &fakeStrategy{name: "val", fundamentals: true, priceHistory: 10}
	a.RegisterStrategy(strat)

	item := WatchlistItem{Symbol: "600519.SH", Type: TypeStock, Strategies: []string{"val"}}
	a.analyzeSymbol(context.Background(), item)

	got := strat.capturedFundamental()
	if got == nil {
		t.Fatal("expected strategy to receive an assembled Fundamental, got nil")
	}
	if got.Source != "lixinger_cvpos" || got.PEPercentile != 23.4 {
		t.Errorf("Fundamental = %+v, want lixinger_cvpos / 23.4", got)
	}
}

func TestAnalyzeSymbol_SkipsFundamentalWhenNotNeeded(t *testing.T) {
	a := New(&config.Config{}, zap.NewNop())
	a.SetValuationSources(&stubVal{pct: 50}, nil)
	a.RegisterCollector(&mockCollector{name: "eastmoney", history: sampleCloses(300)})
	strat := &fakeStrategy{name: "plain", priceHistory: 10} // fundamentals=false
	a.RegisterStrategy(strat)

	item := WatchlistItem{Symbol: "600519.SH", Type: TypeStock, Strategies: []string{"plain"}}
	a.analyzeSymbol(context.Background(), item)

	if got := strat.capturedFundamental(); got != nil {
		t.Errorf("expected no Fundamental for non-fundamental strategy, got %+v", got)
	}
}
