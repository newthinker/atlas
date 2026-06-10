package app

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/collector"
	"github.com/newthinker/atlas/internal/config"
	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/notifier"
	"github.com/newthinker/atlas/internal/strategy"
)

type mockCollector struct {
	name       string
	history    []core.OHLCV
	fetchCount *int // optional: incremented on each FetchHistory call
}

func (m *mockCollector) Name() string                        { return m.name }
func (m *mockCollector) SupportedMarkets() []core.Market     { return []core.Market{core.MarketUS} }
func (m *mockCollector) Init(cfg collector.Config) error     { return nil }
func (m *mockCollector) Start(ctx context.Context) error     { return nil }
func (m *mockCollector) Stop() error                         { return nil }
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
	return m.signals, nil
}

type mockNotifier struct {
	name     string
	received []core.Signal
}

func (m *mockNotifier) Name() string { return m.name }
func (m *mockNotifier) Init(cfg notifier.Config) error { return nil }
func (m *mockNotifier) Send(signal core.Signal) error {
	m.received = append(m.received, signal)
	return nil
}
func (m *mockNotifier) SendBatch(signals []core.Signal) error {
	m.received = append(m.received, signals...)
	return nil
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
	if len(mockNoti.received) != 1 {
		t.Errorf("expected 1 signal, got %d", len(mockNoti.received))
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

	if len(noti.received) != 1 {
		t.Fatalf("expected 1 signal (only s1), got %d", len(noti.received))
	}
	if noti.received[0].Strategy != "s1" {
		t.Errorf("expected signal from s1, got %q", noti.received[0].Strategy)
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

	if len(mockNoti.received) != 1 {
		t.Errorf("expected 1 routed signal, got %d", len(mockNoti.received))
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

func TestApp_Executor_ErrorDoesNotStop(t *testing.T) {
	app := New(&config.Config{}, nil)
	app.RegisterCollector(&mockCollector{name: "mock", history: executorTestHistory()})
	// Two signals on one symbol; both should be submitted even if executor errors.
	app.RegisterStrategy(&mockStrategy{name: "mock", signals: []core.Signal{
		{Symbol: "TEST", Action: core.ActionBuy, Confidence: 0.8},
		{Symbol: "TEST", Action: core.ActionStrongBuy, Confidence: 0.9},
	}})
	app.SetWatchlist([]string{"TEST", "TEST2"})

	exec := &mockExecutor{err: errSubmitBoom}
	app.SetExecutor(exec)

	app.RunOnce(context.Background())

	// 2 signals per symbol × 2 symbols = 4 submissions despite errors.
	if got := exec.count(); got != 4 {
		t.Fatalf("expected 4 SubmitSignal calls despite errors, got %d", got)
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
