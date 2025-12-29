package collector

import (
	"context"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

// FundamentalCollector defines interface for fundamental data collectors
type FundamentalCollector interface {
	// Metadata
	Name() string
	SupportedMarkets() []core.Market

	// Lifecycle
	Init(cfg Config) error
	Start(ctx context.Context) error
	Stop() error

	// Data fetching
	FetchFundamental(symbol string) (*core.Fundamental, error)
	FetchFundamentalHistory(symbol string, start, end time.Time) ([]core.Fundamental, error)
}

// FundamentalRegistry manages fundamental collector instances
type FundamentalRegistry struct {
	collectors map[string]FundamentalCollector
}

// NewFundamentalRegistry creates a new fundamental collector registry
func NewFundamentalRegistry() *FundamentalRegistry {
	return &FundamentalRegistry{
		collectors: make(map[string]FundamentalCollector),
	}
}

func (r *FundamentalRegistry) Register(c FundamentalCollector) {
	r.collectors[c.Name()] = c
}

func (r *FundamentalRegistry) Get(name string) (FundamentalCollector, bool) {
	c, ok := r.collectors[name]
	return c, ok
}

func (r *FundamentalRegistry) GetAll() []FundamentalCollector {
	result := make([]FundamentalCollector, 0, len(r.collectors))
	for _, c := range r.collectors {
		result = append(result, c)
	}
	return result
}
