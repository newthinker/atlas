package strategy

import (
	"time"

	"github.com/newthinker/atlas/internal/core"
)

// SinceInceptionBars is the PriceHistory value a percentile strategy declares
// when configured with lookback_years: 0 ("since inception"). It is a large
// trading-day count (~100 years) so historyWindowDays computes a window that
// reaches back past any real listing date; FetchHistory then returns only the
// bars that actually exist, giving each symbol its own full-history window.
// Note: the app-side fetch start is additionally clamped to 1970-01-01 (epochFloor)
// so no Yahoo URL receives a negative Unix timestamp — see internal/app/app.go.
const SinceInceptionBars = 100 * 252

// Config holds strategy configuration
type Config struct {
	Enabled bool
	Params  map[string]any
}

// DataRequirements specifies what data a strategy needs
type DataRequirements struct {
	Markets      []core.Market
	AssetTypes   []core.AssetType
	PriceHistory int  // Days of history needed
	Fundamentals bool // Needs fundamental data
	Indicators   []string
}

// AnalysisContext provides data to strategies
type AnalysisContext struct {
	Symbol       string
	Market       core.Market
	OHLCV        []core.OHLCV
	LatestQuote  *core.Quote
	Fundamental  *core.Fundamental
	Fundamentals map[string]float64
	Indicators   map[string][]float64
	Now          time.Time
}

// Strategy defines the interface for trading strategies
type Strategy interface {
	Name() string
	Description() string
	RequiredData() DataRequirements
	Init(cfg Config) error
	Analyze(ctx AnalysisContext) ([]core.Signal, error)
}
