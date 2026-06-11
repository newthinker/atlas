// Package price_percentile signals when the current price sits at an extreme
// of its own multi-year distribution. Applies to every asset class.
//
// It deliberately does NOT share a base type with pe_percentile: the two
// strategies have independent threshold semantics that may diverge, and the
// small duplication buys clear boundaries (see plan Task 10).
package price_percentile

import (
	"fmt"

	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/strategy"
	"github.com/newthinker/atlas/internal/valuation"
)

// Strategy emits buy/sell signals from where the current close sits within its
// own multi-year price distribution.
type Strategy struct {
	lookbackYears int
	low, high     float64
	extremeLow    float64
	extremeHigh   float64
}

func New() *Strategy {
	return &Strategy{lookbackYears: 5, low: 25, high: 75, extremeLow: 10, extremeHigh: 90}
}

func (s *Strategy) Name() string        { return "price_percentile" }
func (s *Strategy) Description() string { return "Price position in its own multi-year history" }

func (s *Strategy) RequiredData() strategy.DataRequirements {
	return strategy.DataRequirements{
		PriceHistory: s.lookbackYears * 252,
		AssetTypes: []core.AssetType{
			core.AssetStock, core.AssetIndex, core.AssetETF,
			core.AssetFund, core.AssetCommodity, core.AssetCrypto,
		},
	}
}

func (s *Strategy) Init(cfg strategy.Config) error {
	s.lookbackYears = int(numParam(cfg.Params, "lookback_years", float64(s.lookbackYears)))
	s.low = numParam(cfg.Params, "low", s.low)
	s.high = numParam(cfg.Params, "high", s.high)
	s.extremeLow = numParam(cfg.Params, "extreme_low", s.extremeLow)
	s.extremeHigh = numParam(cfg.Params, "extreme_high", s.extremeHigh)
	if !(s.extremeLow < s.low && s.low < s.high && s.high < s.extremeHigh) {
		return fmt.Errorf("price_percentile: thresholds must satisfy extreme_low < low < high < extreme_high, got %.1f/%.1f/%.1f/%.1f",
			s.extremeLow, s.low, s.high, s.extremeHigh)
	}
	if s.lookbackYears <= 0 {
		return fmt.Errorf("price_percentile: lookback_years must be positive, got %d", s.lookbackYears)
	}
	return nil
}

const minSampleBars = 252 // 不足 1 年样本不出信号（新上市资产防误报）

func (s *Strategy) Analyze(ctx strategy.AnalysisContext) ([]core.Signal, error) {
	if len(ctx.OHLCV) < minSampleBars {
		return nil, nil
	}
	closes := make([]float64, len(ctx.OHLCV))
	for i, b := range ctx.OHLCV {
		closes[i] = b.Close
	}
	cur := closes[len(closes)-1]
	p := valuation.PercentileRank(closes, cur)

	action, conf := s.classify(p)
	if action == "" {
		return nil, nil
	}
	return []core.Signal{{
		Symbol:      ctx.Symbol,
		Action:      action,
		Confidence:  conf,
		Price:       cur,
		Reason:      fmt.Sprintf("price at %.1f%% of %d-year range", p, s.lookbackYears),
		Strategy:    s.Name(),
		GeneratedAt: ctx.Now,
		Metadata: map[string]any{
			"percentile": p, "lookback_years": s.lookbackYears, "sample_size": len(closes),
		},
	}}, nil
}

// classify maps a percentile to (action, confidence); "" means no signal.
// Bands per design §3.1: extreme→0.8+linear(capped 0.95), normal→0.6-0.8 linear.
func (s *Strategy) classify(p float64) (core.Action, float64) {
	switch {
	case p < s.extremeLow:
		return core.ActionStrongBuy, min(0.95, 0.8+0.15*(s.extremeLow-p)/s.extremeLow)
	case p < s.low:
		return core.ActionBuy, 0.6 + 0.2*(s.low-p)/(s.low-s.extremeLow)
	case p > s.extremeHigh:
		return core.ActionStrongSell, min(0.95, 0.8+0.15*(p-s.extremeHigh)/(100-s.extremeHigh))
	case p > s.high:
		return core.ActionSell, 0.6 + 0.2*(p-s.high)/(s.extremeHigh-s.high)
	}
	return "", 0
}

// numParam reads a numeric param tolerating both int and float64, since viper
// may decode YAML numbers either way. ma_crossover's int-only cast is not safe
// to copy here.
func numParam(p map[string]any, key string, def float64) float64 {
	switch v := p[key].(type) {
	case int:
		return float64(v)
	case float64:
		return v
	}
	return def
}
