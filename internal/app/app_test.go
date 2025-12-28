package app

import (
	"context"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/collector"
	"github.com/newthinker/atlas/internal/config"
	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/notifier"
	"github.com/newthinker/atlas/internal/strategy"
)

type mockCollector struct {
	name    string
	history []core.OHLCV
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
