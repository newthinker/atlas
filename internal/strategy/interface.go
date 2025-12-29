package strategy

import (
	"time"

	"github.com/newthinker/atlas/internal/core"
)

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
