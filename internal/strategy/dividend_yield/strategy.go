// Package dividend_yield implements a dividend yield strategy
package dividend_yield

import (
	"fmt"
	"time"

	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/strategy"
)

// DividendYield implements a dividend yield strategy
// Generates buy signals when dividend yield is above threshold
type DividendYield struct {
	minYield float64 // Minimum yield percentage for buy signal
}

// New creates a new Dividend Yield strategy
func New(minYield float64) *DividendYield {
	return &DividendYield{
		minYield: minYield,
	}
}

func (d *DividendYield) Name() string { return "dividend_yield" }

func (d *DividendYield) Description() string {
	return fmt.Sprintf("Dividend Yield Strategy (min: %.1f%%)", d.minYield)
}

func (d *DividendYield) RequiredData() strategy.DataRequirements {
	return strategy.DataRequirements{
		PriceHistory: 0,
		Fundamentals: true,
	}
}

func (d *DividendYield) Init(cfg strategy.Config) error {
	if yield, ok := cfg.Params["min_yield"].(float64); ok {
		d.minYield = yield
	}
	return nil
}

func (d *DividendYield) Analyze(ctx strategy.AnalysisContext) ([]core.Signal, error) {
	if ctx.Fundamental == nil {
		return nil, nil
	}

	yield := ctx.Fundamental.DividendYield
	if yield <= 0 {
		return nil, nil // No dividend
	}

	var signals []core.Signal

	if yield >= d.minYield {
		signals = append(signals, core.Signal{
			Symbol:      ctx.Symbol,
			Action:      core.ActionBuy,
			Confidence:  d.calculateConfidence(yield),
			Reason:      fmt.Sprintf("Dividend yield (%.2f%%) above threshold (%.1f%%)", yield, d.minYield),
			Strategy:    d.Name(),
			GeneratedAt: time.Now(),
			Metadata: map[string]any{
				"dividend_yield": yield,
				"min_yield":      d.minYield,
				"type":           "high_dividend",
			},
		})
	}

	return signals, nil
}

func (d *DividendYield) calculateConfidence(yield float64) float64 {
	// Higher yield = higher confidence, capped at 0.9
	diff := (yield - d.minYield) / d.minYield
	confidence := 0.5 + (diff * 0.5)
	if confidence > 0.9 {
		confidence = 0.9
	}
	return confidence
}
