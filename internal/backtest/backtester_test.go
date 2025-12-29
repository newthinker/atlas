package backtest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/strategy"
)

// mockProvider implements OHLCVProvider for testing
type mockProvider struct {
	data []core.OHLCV
	err  error
}

func (m *mockProvider) FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.data, nil
}

// mockStrategy implements strategy.Strategy for testing
type mockStrategy struct {
	name     string
	signals  []core.Signal
	required int
}

func (m *mockStrategy) Name() string {
	return m.name
}

func (m *mockStrategy) Description() string {
	return "Mock strategy for testing"
}

func (m *mockStrategy) RequiredData() strategy.DataRequirements {
	return strategy.DataRequirements{
		PriceHistory: m.required,
	}
}

func (m *mockStrategy) Init(cfg strategy.Config) error {
	return nil
}

func (m *mockStrategy) Analyze(ctx strategy.AnalysisContext) ([]core.Signal, error) {
	// Return configured signals based on data length
	if len(ctx.OHLCV) >= m.required && len(m.signals) > 0 {
		// Return one signal at a time based on current position
		idx := len(ctx.OHLCV) - m.required
		if idx < len(m.signals) {
			return []core.Signal{m.signals[idx]}, nil
		}
	}
	return nil, nil
}

func TestBacktester_Run(t *testing.T) {
	now := time.Now()
	baseTime := now.AddDate(0, 0, -10)

	// Create test OHLCV data
	ohlcvData := []core.OHLCV{
		{Symbol: "AAPL", Interval: "1d", Open: 100, High: 105, Low: 99, Close: 102, Volume: 1000, Time: baseTime},
		{Symbol: "AAPL", Interval: "1d", Open: 102, High: 108, Low: 101, Close: 106, Volume: 1200, Time: baseTime.AddDate(0, 0, 1)},
		{Symbol: "AAPL", Interval: "1d", Open: 106, High: 110, Low: 104, Close: 108, Volume: 1100, Time: baseTime.AddDate(0, 0, 2)},
		{Symbol: "AAPL", Interval: "1d", Open: 108, High: 112, Low: 107, Close: 105, Volume: 1300, Time: baseTime.AddDate(0, 0, 3)},
		{Symbol: "AAPL", Interval: "1d", Open: 105, High: 107, Low: 103, Close: 104, Volume: 900, Time: baseTime.AddDate(0, 0, 4)},
	}

	// Create mock signals (Buy -> Sell cycle)
	testSignals := []core.Signal{
		{Symbol: "AAPL", Action: core.ActionBuy, Confidence: 0.8, GeneratedAt: baseTime.AddDate(0, 0, 1)},
		{Symbol: "AAPL", Action: core.ActionHold, Confidence: 0.5, GeneratedAt: baseTime.AddDate(0, 0, 2)},
		{Symbol: "AAPL", Action: core.ActionSell, Confidence: 0.7, GeneratedAt: baseTime.AddDate(0, 0, 3)},
	}

	provider := &mockProvider{data: ohlcvData}
	strat := &mockStrategy{name: "test_strategy", signals: testSignals, required: 2}
	backtester := New(provider)

	result, err := backtester.Run(context.Background(), strat, "AAPL", baseTime, now)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Verify result
	if result.Strategy != "test_strategy" {
		t.Errorf("Strategy = %v, want test_strategy", result.Strategy)
	}

	if result.Symbol != "AAPL" {
		t.Errorf("Symbol = %v, want AAPL", result.Symbol)
	}

	if len(result.Signals) == 0 {
		t.Error("Expected at least one signal")
	}

	// Verify trades were generated
	if len(result.Trades) == 0 {
		t.Error("Expected at least one trade")
	}

	// Verify stats were calculated
	if result.Stats.TotalTrades != len(result.Trades) {
		t.Errorf("TotalTrades = %v, want %v", result.Stats.TotalTrades, len(result.Trades))
	}
}

func TestBacktester_Run_NoData(t *testing.T) {
	provider := &mockProvider{data: []core.OHLCV{}}
	strat := &mockStrategy{name: "test", required: 1}
	backtester := New(provider)

	_, err := backtester.Run(context.Background(), strat, "AAPL", time.Now().AddDate(0, 0, -10), time.Now())
	if err == nil {
		t.Error("Expected error for empty data")
	}
}

func TestBacktester_Run_ProviderError(t *testing.T) {
	provider := &mockProvider{err: errors.New("provider error")}
	strat := &mockStrategy{name: "test", required: 1}
	backtester := New(provider)

	_, err := backtester.Run(context.Background(), strat, "AAPL", time.Now().AddDate(0, 0, -10), time.Now())
	if err == nil {
		t.Error("Expected error from provider")
	}
}

func TestBacktester_Run_ContextCancellation(t *testing.T) {
	ohlcvData := make([]core.OHLCV, 100)
	for i := range ohlcvData {
		ohlcvData[i] = core.OHLCV{Symbol: "AAPL", Close: 100, Time: time.Now().AddDate(0, 0, -100+i)}
	}

	provider := &mockProvider{data: ohlcvData}
	strat := &mockStrategy{name: "test", required: 1}
	backtester := New(provider)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := backtester.Run(ctx, strat, "AAPL", time.Now().AddDate(0, 0, -10), time.Now())
	if err == nil {
		t.Error("Expected context cancellation error")
	}
}

func TestSignalsToTrades(t *testing.T) {
	now := time.Now()

	ohlcv := []core.OHLCV{
		{Close: 100, Time: now},
		{Close: 105, Time: now.AddDate(0, 0, 1)},
		{Close: 110, Time: now.AddDate(0, 0, 2)},
		{Close: 108, Time: now.AddDate(0, 0, 3)},
		{Close: 115, Time: now.AddDate(0, 0, 4)},
		{Close: 120, Time: now.AddDate(0, 0, 5)},
	}

	signals := []core.Signal{
		{Symbol: "AAPL", Action: core.ActionBuy, Price: 100, GeneratedAt: now},
		{Symbol: "AAPL", Action: core.ActionSell, Price: 110, GeneratedAt: now.AddDate(0, 0, 2)},
		{Symbol: "AAPL", Action: core.ActionStrongBuy, Price: 108, GeneratedAt: now.AddDate(0, 0, 3)},
		{Symbol: "AAPL", Action: core.ActionStrongSell, Price: 120, GeneratedAt: now.AddDate(0, 0, 5)},
	}

	trades := signalsToTrades(signals, ohlcv)

	// Expect 2 closed trades
	if len(trades) != 2 {
		t.Fatalf("Expected 2 trades, got %d", len(trades))
	}

	// First trade: Buy at 100, Sell at 110 = 10% return
	if trades[0].EntryPrice != 100 {
		t.Errorf("Trade 1 EntryPrice = %v, want 100", trades[0].EntryPrice)
	}
	if trades[0].ExitPrice != 110 {
		t.Errorf("Trade 1 ExitPrice = %v, want 110", trades[0].ExitPrice)
	}
	expectedReturn1 := (110.0 - 100.0) / 100.0
	if trades[0].Return != expectedReturn1 {
		t.Errorf("Trade 1 Return = %v, want %v", trades[0].Return, expectedReturn1)
	}
	if !trades[0].IsClosed() {
		t.Error("Trade 1 should be closed")
	}

	// Second trade: StrongBuy at 108, StrongSell at 120
	if trades[1].EntryPrice != 108 {
		t.Errorf("Trade 2 EntryPrice = %v, want 108", trades[1].EntryPrice)
	}
	if trades[1].ExitPrice != 120 {
		t.Errorf("Trade 2 ExitPrice = %v, want 120", trades[1].ExitPrice)
	}
	expectedReturn2 := (120.0 - 108.0) / 108.0
	if trades[1].Return != expectedReturn2 {
		t.Errorf("Trade 2 Return = %v, want %v", trades[1].Return, expectedReturn2)
	}
	if !trades[1].IsClosed() {
		t.Error("Trade 2 should be closed")
	}
}

func TestSignalsToTrades_OpenPosition(t *testing.T) {
	now := time.Now()

	ohlcv := []core.OHLCV{
		{Close: 100, Time: now},
		{Close: 105, Time: now.AddDate(0, 0, 1)},
		{Close: 110, Time: now.AddDate(0, 0, 2)},
	}

	signals := []core.Signal{
		{Symbol: "AAPL", Action: core.ActionBuy, Price: 100, GeneratedAt: now},
	}

	trades := signalsToTrades(signals, ohlcv)

	// Expect 1 open trade
	if len(trades) != 1 {
		t.Fatalf("Expected 1 trade, got %d", len(trades))
	}

	if trades[0].EntryPrice != 100 {
		t.Errorf("EntryPrice = %v, want 100", trades[0].EntryPrice)
	}
	// Open trade should use last OHLCV close
	if trades[0].ExitPrice != 110 {
		t.Errorf("ExitPrice = %v, want 110 (last close)", trades[0].ExitPrice)
	}
	if trades[0].IsClosed() {
		t.Error("Trade should be open (no ExitSignal)")
	}
}

func TestSignalsToTrades_NoSignals(t *testing.T) {
	ohlcv := []core.OHLCV{{Close: 100}}
	trades := signalsToTrades(nil, ohlcv)
	if len(trades) != 0 {
		t.Errorf("Expected 0 trades, got %d", len(trades))
	}
}

func TestSignalsToTrades_SellWithoutBuy(t *testing.T) {
	now := time.Now()
	ohlcv := []core.OHLCV{{Close: 100, Time: now}}
	signals := []core.Signal{
		{Symbol: "AAPL", Action: core.ActionSell, Price: 100, GeneratedAt: now},
	}

	trades := signalsToTrades(signals, ohlcv)
	// Sell without open position should result in no trades
	if len(trades) != 0 {
		t.Errorf("Expected 0 trades for sell without buy, got %d", len(trades))
	}
}

func TestMax(t *testing.T) {
	tests := []struct {
		a, b, want int
	}{
		{1, 2, 2},
		{5, 3, 5},
		{0, 0, 0},
		{-1, 1, 1},
		{-5, -3, -3},
	}

	for _, tt := range tests {
		got := max(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("max(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}
