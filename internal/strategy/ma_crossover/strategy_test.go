package ma_crossover

import (
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/strategy"
)

func TestMACrossover_ImplementsStrategy(t *testing.T) {
	var _ strategy.Strategy = (*MACrossover)(nil)
}

func TestMACrossover_Name(t *testing.T) {
	s := New(5, 10)
	if s.Name() != "ma_crossover" {
		t.Errorf("expected 'ma_crossover', got '%s'", s.Name())
	}
}

func TestMACrossover_GoldenCross(t *testing.T) {
	s := New(2, 4)

	// Golden cross: fast MA was <= slow MA, now fast > slow
	// With period 2 and 4:
	// prevFast = (p[n-2] + p[n-1]) / 2, prevSlow = (p[n-4] + p[n-3] + p[n-2] + p[n-1]) / 4
	// currFast = (p[n-1] + p[n]) / 2, currSlow = (p[n-3] + p[n-2] + p[n-1] + p[n]) / 4
	//
	// Need: prevFast <= prevSlow AND currFast > currSlow
	// Declining then sharp recovery at the very end
	prices := []float64{
		100, 95, 90, 85, 80, // declining
		120,                  // sharp spike at the end
	}
	// prevFast = (80 + 85) / 2 = 82.5
	// prevSlow = (90 + 85 + 80 + 85) / 4 = 85 -- wait, this is wrong
	// Let me recalculate with indices:
	// n = 5 (6 elements, 0-indexed)
	// prevFast = (p[3] + p[4]) / 2 = (85 + 80) / 2 = 82.5
	// prevSlow = (p[1] + p[2] + p[3] + p[4]) / 4 = (95 + 90 + 85 + 80) / 4 = 87.5
	// currFast = (p[4] + p[5]) / 2 = (80 + 120) / 2 = 100
	// currSlow = (p[2] + p[3] + p[4] + p[5]) / 4 = (90 + 85 + 80 + 120) / 4 = 93.75
	//
	// prevFast (82.5) < prevSlow (87.5) ✓
	// currFast (100) > currSlow (93.75) ✓ Golden cross!

	ohlcv := make([]core.OHLCV, len(prices))
	for i := 0; i < len(prices); i++ {
		ohlcv[i] = core.OHLCV{
			Symbol: "TEST",
			Close:  prices[i],
			Time:   time.Now().Add(time.Duration(-len(prices)+i) * 24 * time.Hour),
		}
	}

	ctx := strategy.AnalysisContext{
		Symbol: "TEST",
		OHLCV:  ohlcv,
		Now:    time.Now(),
	}

	signals, err := s.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(signals) == 0 {
		t.Fatal("expected at least one signal for golden cross")
	}

	if signals[0].Action != core.ActionBuy {
		t.Errorf("expected Buy action for golden cross, got %s", signals[0].Action)
	}
}

func TestMACrossover_NotEnoughData(t *testing.T) {
	s := New(50, 200)

	// Only 100 days of data, need 200 for slow MA
	ohlcv := make([]core.OHLCV, 100)
	for i := 0; i < 100; i++ {
		ohlcv[i] = core.OHLCV{Close: 100}
	}

	ctx := strategy.AnalysisContext{
		Symbol: "TEST",
		OHLCV:  ohlcv,
	}

	signals, err := s.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should return no signals due to insufficient data
	if len(signals) != 0 {
		t.Errorf("expected no signals with insufficient data, got %d", len(signals))
	}
}

func TestMACrossover_DeathCross(t *testing.T) {
	s := New(2, 4)

	// Death cross: fast MA was >= slow MA, now fast < slow
	// Rising then sharp drop at the very end
	prices := []float64{
		80, 85, 90, 95, 100, // rising
		60,                   // sharp drop at the end
	}
	// n = 5
	// prevFast = (p[3] + p[4]) / 2 = (95 + 100) / 2 = 97.5
	// prevSlow = (p[1] + p[2] + p[3] + p[4]) / 4 = (85 + 90 + 95 + 100) / 4 = 92.5
	// currFast = (p[4] + p[5]) / 2 = (100 + 60) / 2 = 80
	// currSlow = (p[2] + p[3] + p[4] + p[5]) / 4 = (90 + 95 + 100 + 60) / 4 = 86.25
	//
	// prevFast (97.5) > prevSlow (92.5) ✓
	// currFast (80) < currSlow (86.25) ✓ Death cross!

	ohlcv := make([]core.OHLCV, len(prices))
	for i := 0; i < len(prices); i++ {
		ohlcv[i] = core.OHLCV{
			Symbol: "TEST",
			Close:  prices[i],
			Time:   time.Now().Add(time.Duration(-len(prices)+i) * 24 * time.Hour),
		}
	}

	ctx := strategy.AnalysisContext{
		Symbol: "TEST",
		OHLCV:  ohlcv,
		Now:    time.Now(),
	}

	signals, err := s.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(signals) == 0 {
		t.Fatal("expected at least one signal for death cross")
	}

	if signals[0].Action != core.ActionSell {
		t.Errorf("expected Sell action for death cross, got %s", signals[0].Action)
	}
}
