// Package pe_percentile signals when a security's PE sits at an extreme of its
// own multi-year historical distribution. Applies to stocks and indexes.
//
// It deliberately does NOT share a base type with price_percentile: the two
// strategies have independent threshold semantics that may diverge, and the
// ~30 lines of duplication buy clear boundaries (see plan Task 10).
package pe_percentile

import (
	"fmt"
	"strings"

	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/strategy"
)

// Strategy emits buy/sell signals from a precomputed PE percentile carried on
// the analysis context's Fundamental (Fundamental.PEPercentile, 0-100).
type Strategy struct {
	lookbackYears int
	low, high     float64
	extremeLow    float64
	extremeHigh   float64
	// percentileStep is the per-strategy re-alert step carried to the router via
	// Signal.Metadata["percentile_step"]. <= 0 means unconfigured: the router
	// falls back to its global router.percentile_step (design rev4 §2).
	percentileStep float64
}

func New() *Strategy {
	return &Strategy{lookbackYears: 5, low: 20, high: 80, extremeLow: 10, extremeHigh: 90}
}

func (s *Strategy) Name() string        { return "pe_percentile" }
func (s *Strategy) Description() string { return "PE position in its own multi-year history" }

func (s *Strategy) RequiredData() strategy.DataRequirements {
	return strategy.DataRequirements{
		// PriceHistory must be declared: the assembly layer's PE reconstruction
		// reuses the OHLCV fetched for this item, and the window is the max
		// PriceHistory across the item's bound strategies. Omitting it would
		// leave a solo binding with only ~1 year of data, reconstructing a
		// "1-year PE percentile" that contradicts the N-year reason text.
		PriceHistory: s.lookbackYears * 252,
		Fundamentals: true,
		AssetTypes:   []core.AssetType{core.AssetStock, core.AssetIndex},
	}
}

func (s *Strategy) Init(cfg strategy.Config) error {
	s.lookbackYears = int(numParam(cfg.Params, "lookback_years", float64(s.lookbackYears)))
	s.low = numParam(cfg.Params, "low", s.low)
	s.high = numParam(cfg.Params, "high", s.high)
	s.extremeLow = numParam(cfg.Params, "extreme_low", s.extremeLow)
	s.extremeHigh = numParam(cfg.Params, "extreme_high", s.extremeHigh)
	s.percentileStep = numParam(cfg.Params, "percentile_step", 0)
	if !(s.extremeLow < s.low && s.low < s.high && s.high < s.extremeHigh) {
		return fmt.Errorf("pe_percentile: thresholds must satisfy extreme_low < low < high < extreme_high, got %.1f/%.1f/%.1f/%.1f",
			s.extremeLow, s.low, s.high, s.extremeHigh)
	}
	if s.lookbackYears <= 0 {
		return fmt.Errorf("pe_percentile: lookback_years must be positive, got %d", s.lookbackYears)
	}
	return nil
}

func (s *Strategy) Analyze(ctx strategy.AnalysisContext) ([]core.Signal, error) {
	if ctx.Fundamental == nil || ctx.Fundamental.PEPercentile < 0 {
		return nil, nil
	}
	p := ctx.Fundamental.PEPercentile
	action, conf := s.classify(p)
	if action == "" {
		return nil, nil
	}

	// Source encodes how the percentile was obtained: "method" or
	// "method:fallback_reason" (e.g. "lixinger_cvpos:yahoo_eps_insufficient").
	method, fallbackReason, _ := strings.Cut(ctx.Fundamental.Source, ":")
	md := map[string]any{"pe_percentile": p, "method": method, "lookback_years": s.lookbackYears}
	if fallbackReason != "" {
		md["fallback_reason"] = fallbackReason
	}
	// Only carry a positive step; absence signals the router to use its global
	// fallback (design rev4 §2).
	if s.percentileStep > 0 {
		md["percentile_step"] = s.percentileStep
	}

	price := 0.0
	if n := len(ctx.OHLCV); n > 0 {
		price = ctx.OHLCV[n-1].Close
	}
	return []core.Signal{{
		Symbol: ctx.Symbol, Action: action, Confidence: conf, Price: price,
		Reason:   fmt.Sprintf("PE at %.1f%% of its %d-year history (%s)", p, s.lookbackYears, method),
		Strategy: s.Name(), GeneratedAt: ctx.Now, Metadata: md,
	}}, nil
}

// classify maps a PE percentile to (action, confidence); "" means no signal.
// Bands: extreme→0.8+linear(capped 0.95), normal→0.6-0.8 linear.
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
