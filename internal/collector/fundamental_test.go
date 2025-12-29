package collector

import (
	"context"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

type mockFundamentalCollector struct {
	name string
}

func (m *mockFundamentalCollector) Name() string                    { return m.name }
func (m *mockFundamentalCollector) SupportedMarkets() []core.Market { return []core.Market{core.MarketCNA} }
func (m *mockFundamentalCollector) Init(cfg Config) error           { return nil }
func (m *mockFundamentalCollector) Start(ctx context.Context) error { return nil }
func (m *mockFundamentalCollector) Stop() error                     { return nil }
func (m *mockFundamentalCollector) FetchFundamental(symbol string) (*core.Fundamental, error) {
	return &core.Fundamental{Symbol: symbol, PE: 15.5}, nil
}
func (m *mockFundamentalCollector) FetchFundamentalHistory(symbol string, start, end time.Time) ([]core.Fundamental, error) {
	return []core.Fundamental{{Symbol: symbol}}, nil
}

func TestFundamentalRegistry(t *testing.T) {
	r := NewFundamentalRegistry()
	mock := &mockFundamentalCollector{name: "test"}

	r.Register(mock)

	c, ok := r.Get("test")
	if !ok {
		t.Fatal("expected to find collector")
	}
	if c.Name() != "test" {
		t.Errorf("expected name 'test', got %s", c.Name())
	}

	all := r.GetAll()
	if len(all) != 1 {
		t.Errorf("expected 1 collector, got %d", len(all))
	}
}
