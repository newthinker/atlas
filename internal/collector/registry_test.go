package collector

import (
	"context"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

// mockCollector for testing
type mockCollector struct {
	name string
}

func (m *mockCollector) Name() string                    { return m.name }
func (m *mockCollector) SupportedMarkets() []core.Market { return []core.Market{core.MarketUS} }
func (m *mockCollector) Init(cfg Config) error           { return nil }
func (m *mockCollector) Start(ctx context.Context) error { return nil }
func (m *mockCollector) Stop() error                     { return nil }
func (m *mockCollector) FetchQuote(symbol string) (*core.Quote, error) {
	return &core.Quote{Symbol: symbol, Price: 100}, nil
}
func (m *mockCollector) FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	return nil, nil
}

func TestRegistry_Register(t *testing.T) {
	r := NewRegistry()

	mock := &mockCollector{name: "mock"}
	r.Register(mock)

	c, ok := r.Get("mock")
	if !ok {
		t.Fatal("expected to find registered collector")
	}

	if c.Name() != "mock" {
		t.Errorf("expected name 'mock', got '%s'", c.Name())
	}
}

func TestRegistry_GetAll(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockCollector{name: "a"})
	r.Register(&mockCollector{name: "b"})

	all := r.GetAll()
	if len(all) != 2 {
		t.Errorf("expected 2 collectors, got %d", len(all))
	}
}
