package ma_crossover

import (
	"fmt"
	"time"

	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/indicator"
	"github.com/newthinker/atlas/internal/strategy"
)

// MACrossover implements a moving average crossover strategy
type MACrossover struct {
	fastPeriod int
	slowPeriod int
}

// New creates a new MA Crossover strategy
func New(fastPeriod, slowPeriod int) *MACrossover {
	return &MACrossover{
		fastPeriod: fastPeriod,
		slowPeriod: slowPeriod,
	}
}

func (m *MACrossover) Name() string {
	return "ma_crossover"
}

func (m *MACrossover) Description() string {
	return fmt.Sprintf("MA Crossover (%d/%d)", m.fastPeriod, m.slowPeriod)
}

func (m *MACrossover) RequiredData() strategy.DataRequirements {
	return strategy.DataRequirements{
		PriceHistory: m.slowPeriod + 10, // Extra buffer
		Indicators:   []string{"SMA"},
	}
}

func (m *MACrossover) Init(cfg strategy.Config) error {
	if fast, ok := cfg.Params["fast_period"].(int); ok {
		m.fastPeriod = fast
	}
	if slow, ok := cfg.Params["slow_period"].(int); ok {
		m.slowPeriod = slow
	}
	return nil
}

func (m *MACrossover) Analyze(ctx strategy.AnalysisContext) ([]core.Signal, error) {
	if len(ctx.OHLCV) < m.slowPeriod {
		return nil, nil // Not enough data
	}

	// Extract closing prices
	prices := make([]float64, len(ctx.OHLCV))
	for i, bar := range ctx.OHLCV {
		prices[i] = bar.Close
	}

	// Calculate moving averages
	fastMA := indicator.SMA(prices, m.fastPeriod)
	slowMA := indicator.SMA(prices, m.slowPeriod)

	if len(fastMA) < 2 || len(slowMA) < 2 {
		return nil, nil
	}

	// Align the MAs (fast MA has more values since it uses shorter period)
	offset := len(fastMA) - len(slowMA)
	if offset < 0 {
		return nil, nil
	}

	// Get current and previous values
	currFast := fastMA[len(fastMA)-1]
	prevFast := fastMA[len(fastMA)-2]
	currSlow := slowMA[len(slowMA)-1]
	prevSlow := slowMA[len(slowMA)-2]

	var signals []core.Signal

	// Golden Cross: fast crosses above slow
	if prevFast <= prevSlow && currFast > currSlow {
		signals = append(signals, core.Signal{
			Symbol:      ctx.Symbol,
			Action:      core.ActionBuy,
			Confidence:  m.calculateConfidence(currFast, currSlow),
			Reason:      fmt.Sprintf("Golden Cross: MA%d (%.2f) crossed above MA%d (%.2f)", m.fastPeriod, currFast, m.slowPeriod, currSlow),
			GeneratedAt: time.Now(),
			Metadata: map[string]any{
				"fast_ma": currFast,
				"slow_ma": currSlow,
				"type":    "golden_cross",
			},
		})
	}

	// Death Cross: fast crosses below slow
	if prevFast >= prevSlow && currFast < currSlow {
		signals = append(signals, core.Signal{
			Symbol:      ctx.Symbol,
			Action:      core.ActionSell,
			Confidence:  m.calculateConfidence(currFast, currSlow),
			Reason:      fmt.Sprintf("Death Cross: MA%d (%.2f) crossed below MA%d (%.2f)", m.fastPeriod, currFast, m.slowPeriod, currSlow),
			GeneratedAt: time.Now(),
			Metadata: map[string]any{
				"fast_ma": currFast,
				"slow_ma": currSlow,
				"type":    "death_cross",
			},
		})
	}

	return signals, nil
}

// calculateConfidence returns higher confidence for larger divergence
func (m *MACrossover) calculateConfidence(fast, slow float64) float64 {
	diff := (fast - slow) / slow
	if diff < 0 {
		diff = -diff
	}

	// Scale to 0.5-0.9 range based on divergence
	confidence := 0.5 + (diff * 10)
	if confidence > 0.9 {
		confidence = 0.9
	}
	return confidence
}
