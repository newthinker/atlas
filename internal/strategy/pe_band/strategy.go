package pe_band

import (
	"fmt"
	"time"

	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/strategy"
)

// PEBand implements a PE band strategy
type PEBand struct {
	lowThreshold  float64
	highThreshold float64
}

func New(lowThreshold, highThreshold float64) *PEBand {
	return &PEBand{lowThreshold: lowThreshold, highThreshold: highThreshold}
}

func (p *PEBand) Name() string { return "pe_band" }

func (p *PEBand) Description() string {
	return fmt.Sprintf("PE Band Strategy (low: %.1f, high: %.1f)", p.lowThreshold, p.highThreshold)
}

func (p *PEBand) RequiredData() strategy.DataRequirements {
	return strategy.DataRequirements{PriceHistory: 0, Fundamentals: true}
}

func (p *PEBand) Init(cfg strategy.Config) error {
	if low, ok := cfg.Params["low_threshold"].(float64); ok {
		p.lowThreshold = low
	}
	if high, ok := cfg.Params["high_threshold"].(float64); ok {
		p.highThreshold = high
	}
	return nil
}

func (p *PEBand) Analyze(ctx strategy.AnalysisContext) ([]core.Signal, error) {
	if ctx.Fundamental == nil {
		return nil, nil
	}

	pe := ctx.Fundamental.PE
	if pe <= 0 {
		return nil, nil
	}

	var signals []core.Signal

	if pe < p.lowThreshold {
		signals = append(signals, core.Signal{
			Symbol:      ctx.Symbol,
			Action:      core.ActionBuy,
			Confidence:  p.calculateConfidence(pe, p.lowThreshold, true),
			Reason:      fmt.Sprintf("PE (%.2f) below threshold (%.1f)", pe, p.lowThreshold),
			GeneratedAt: time.Now(),
			Metadata:    map[string]any{"pe": pe, "type": "pe_undervalued"},
		})
	}

	if pe > p.highThreshold {
		signals = append(signals, core.Signal{
			Symbol:      ctx.Symbol,
			Action:      core.ActionSell,
			Confidence:  p.calculateConfidence(pe, p.highThreshold, false),
			Reason:      fmt.Sprintf("PE (%.2f) above threshold (%.1f)", pe, p.highThreshold),
			GeneratedAt: time.Now(),
			Metadata:    map[string]any{"pe": pe, "type": "pe_overvalued"},
		})
	}

	return signals, nil
}

func (p *PEBand) calculateConfidence(pe, threshold float64, isBuy bool) float64 {
	var diff float64
	if isBuy {
		diff = (threshold - pe) / threshold
	} else {
		diff = (pe - threshold) / threshold
	}
	confidence := 0.5 + (diff * 2)
	if confidence > 0.9 {
		confidence = 0.9
	}
	if confidence < 0.5 {
		confidence = 0.5
	}
	return confidence
}
