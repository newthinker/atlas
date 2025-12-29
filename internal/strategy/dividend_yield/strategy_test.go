package dividend_yield

import (
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/strategy"
)

func TestDividendYield_ImplementsStrategy(t *testing.T) {
	var _ strategy.Strategy = (*DividendYield)(nil)
}

func TestDividendYield_Name(t *testing.T) {
	d := New(3.0)
	if d.Name() != "dividend_yield" {
		t.Errorf("expected 'dividend_yield', got %s", d.Name())
	}
}

func TestDividendYield_BuySignal(t *testing.T) {
	d := New(3.0) // Buy when yield >= 3%

	ctx := strategy.AnalysisContext{
		Symbol: "600519",
		Fundamental: &core.Fundamental{
			Symbol:        "600519",
			DividendYield: 4.5, // 4.5% > 3%, should trigger buy
			Date:          time.Now(),
		},
	}

	signals, err := d.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(signals) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(signals))
	}

	if signals[0].Action != core.ActionBuy {
		t.Errorf("expected Buy action, got %s", signals[0].Action)
	}
}

func TestDividendYield_NoSignal_LowYield(t *testing.T) {
	d := New(3.0)

	ctx := strategy.AnalysisContext{
		Symbol: "600519",
		Fundamental: &core.Fundamental{
			Symbol:        "600519",
			DividendYield: 2.0, // 2% < 3%, no signal
			Date:          time.Now(),
		},
	}

	signals, err := d.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(signals) != 0 {
		t.Errorf("expected no signals for low yield, got %d", len(signals))
	}
}

func TestDividendYield_NoSignal_ZeroYield(t *testing.T) {
	d := New(3.0)

	ctx := strategy.AnalysisContext{
		Symbol: "600519",
		Fundamental: &core.Fundamental{
			Symbol:        "600519",
			DividendYield: 0,
			Date:          time.Now(),
		},
	}

	signals, err := d.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(signals) != 0 {
		t.Errorf("expected no signals for zero yield, got %d", len(signals))
	}
}

func TestDividendYield_NoFundamental(t *testing.T) {
	d := New(3.0)

	ctx := strategy.AnalysisContext{
		Symbol:      "600519",
		Fundamental: nil,
	}

	signals, err := d.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(signals) != 0 {
		t.Errorf("expected no signals without fundamentals, got %d", len(signals))
	}
}

func TestDividendYield_RequiresFundamentals(t *testing.T) {
	d := New(3.0)
	req := d.RequiredData()
	if !req.Fundamentals {
		t.Error("dividend_yield should require fundamentals")
	}
}

func TestDividendYield_Init(t *testing.T) {
	d := New(3.0)
	err := d.Init(strategy.Config{
		Params: map[string]any{
			"min_yield": 5.0,
		},
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if d.minYield != 5.0 {
		t.Errorf("expected minYield 5.0, got %f", d.minYield)
	}
}
