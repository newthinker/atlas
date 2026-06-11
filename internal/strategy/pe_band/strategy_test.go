package pe_band

import (
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/strategy"
)

func TestPEBand_ImplementsStrategy(t *testing.T) {
	var _ strategy.Strategy = (*PEBand)(nil)
}

func TestPEBand_Name(t *testing.T) {
	p := New(10, 30)
	if p.Name() != "pe_band" {
		t.Errorf("expected 'pe_band', got %s", p.Name())
	}
}

func TestPEBand_Description(t *testing.T) {
	if got := New(10, 30).Description(); got != "PE Band Strategy (low: 10.0, high: 30.0)" {
		t.Errorf("Description() = %q", got)
	}
}

func TestPEBand_Init(t *testing.T) {
	p := New(10, 30)
	if err := p.Init(strategy.Config{Params: map[string]any{
		"low_threshold":  12.0,
		"high_threshold": 35.0,
	}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.lowThreshold != 12.0 || p.highThreshold != 35.0 {
		t.Errorf("after Init low/high = %.1f/%.1f, want 12.0/35.0", p.lowThreshold, p.highThreshold)
	}
}

func TestPEBand_RequiresFundamentals(t *testing.T) {
	p := New(10, 30)
	req := p.RequiredData()
	if !req.Fundamentals {
		t.Error("PE band should require fundamentals")
	}
}

// Context Checkpoint: done_criteria → test mapping (TASK-009)
// functional[1] "pe_band AssetTypes 恰为 [stock]" → TestPEBand_AssetTypes
func TestPEBand_AssetTypes(t *testing.T) {
	got := New(10, 30).RequiredData().AssetTypes
	if len(got) != 1 || got[0] != core.AssetStock {
		t.Errorf("AssetTypes = %v, want [%q]", got, core.AssetStock)
	}
}

func TestPEBand_BuySignal(t *testing.T) {
	p := New(15, 30)
	ctx := strategy.AnalysisContext{
		Symbol:      "600519",
		Fundamental: &core.Fundamental{Symbol: "600519", PE: 10, Date: time.Now()},
	}
	signals, err := p.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(signals) != 1 || signals[0].Action != core.ActionBuy {
		t.Errorf("expected Buy signal, got %v", signals)
	}
}

func TestPEBand_SellSignal(t *testing.T) {
	p := New(15, 30)
	ctx := strategy.AnalysisContext{
		Symbol:      "600519",
		Fundamental: &core.Fundamental{Symbol: "600519", PE: 40, Date: time.Now()},
	}
	signals, err := p.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(signals) != 1 || signals[0].Action != core.ActionSell {
		t.Errorf("expected Sell signal, got %v", signals)
	}
}

func TestPEBand_NoSignal(t *testing.T) {
	p := New(15, 30)
	ctx := strategy.AnalysisContext{
		Symbol:      "600519",
		Fundamental: &core.Fundamental{Symbol: "600519", PE: 20, Date: time.Now()},
	}
	signals, err := p.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(signals) != 0 {
		t.Errorf("expected no signals, got %d", len(signals))
	}
}

func TestPEBand_NoFundamental(t *testing.T) {
	p := New(15, 30)
	ctx := strategy.AnalysisContext{Symbol: "600519", Fundamental: nil}
	signals, err := p.Analyze(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(signals) != 0 {
		t.Errorf("expected no signals, got %d", len(signals))
	}
}
