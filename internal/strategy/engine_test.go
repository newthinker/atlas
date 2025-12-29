package strategy

import (
	"context"
	"testing"

	"github.com/newthinker/atlas/internal/core"
)

type mockStrategy struct {
	name    string
	signals []core.Signal
}

func (m *mockStrategy) Name() string        { return m.name }
func (m *mockStrategy) Description() string { return "mock strategy" }
func (m *mockStrategy) RequiredData() DataRequirements {
	return DataRequirements{PriceHistory: 200}
}
func (m *mockStrategy) Init(cfg Config) error { return nil }
func (m *mockStrategy) Analyze(ctx AnalysisContext) ([]core.Signal, error) {
	return m.signals, nil
}

func TestEngine_RegisterAndRun(t *testing.T) {
	engine := NewEngine()

	mockSig := core.Signal{
		Symbol:     "AAPL",
		Action:     core.ActionBuy,
		Confidence: 0.8,
		Strategy:   "mock",
	}

	engine.Register(&mockStrategy{
		name:    "mock",
		signals: []core.Signal{mockSig},
	})

	ctx := AnalysisContext{
		Symbol: "AAPL",
		OHLCV:  []core.OHLCV{},
	}

	signals, err := engine.Analyze(context.Background(), ctx)
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

func TestEngine_GetAll(t *testing.T) {
	engine := NewEngine()
	engine.Register(&mockStrategy{name: "a"})
	engine.Register(&mockStrategy{name: "b"})

	all := engine.GetAll()
	if len(all) != 2 {
		t.Errorf("expected 2 strategies, got %d", len(all))
	}
}

func TestEngine_AnalyzeWithStrategies(t *testing.T) {
	engine := NewEngine()

	engine.Register(&mockStrategy{
		name:    "s1",
		signals: []core.Signal{{Symbol: "A", Action: core.ActionBuy}},
	})
	engine.Register(&mockStrategy{
		name:    "s2",
		signals: []core.Signal{{Symbol: "B", Action: core.ActionSell}},
	})

	ctx := AnalysisContext{Symbol: "TEST"}

	// Only run s1
	signals, err := engine.AnalyzeWithStrategies(context.Background(), ctx, []string{"s1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(signals) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(signals))
	}

	if signals[0].Strategy != "s1" {
		t.Errorf("expected strategy s1, got %s", signals[0].Strategy)
	}
}
