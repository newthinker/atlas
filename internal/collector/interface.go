package collector

import (
	"context"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

// Config holds collector configuration
type Config struct {
	Enabled  bool
	Markets  []string
	Interval string
	APIKey   string
	Extra    map[string]any
}

// Collector defines the interface for data collectors
type Collector interface {
	// Metadata
	Name() string
	SupportedMarkets() []core.Market

	// Lifecycle
	Init(cfg Config) error
	Start(ctx context.Context) error
	Stop() error

	// Data fetching
	FetchQuote(symbol string) (*core.Quote, error)
	FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error)
}
