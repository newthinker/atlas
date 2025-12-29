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

func TestPEBand_RequiresFundamentals(t *testing.T) {
	p := New(10, 30)
	req := p.RequiredData()
	if !req.Fundamentals {
		t.Error("PE band should require fundamentals")
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
