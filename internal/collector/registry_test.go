package collector

// Context Checkpoint: done_criteria → test mapping (TASK-001)
// functional[0]     七个 collector 注册顺序 × 100 次 GetAll       → TestRegistry_GetAllKeepsRegistrationOrder
// functional[1]     同名重注册覆盖且保持原位置，Get 返回新实例    → TestRegistry_ReRegisterKeepsPosition
// boundary[0]       空 Registry 的 GetAll 长度 0                  → TestRegistry_GetAllEmpty
// non_functional[0] 并发 Register（不同名+同名）与 GetAll 无竞争  → TestRegistry_ConcurrentRegisterGetAll（-race）

import (
	"context"
	"fmt"
	"reflect"
	"sync"
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

func TestRegistry_GetAllKeepsRegistrationOrder(t *testing.T) {
	r := NewRegistry()
	names := []string{"yahoo", "eastmoney", "crypto", "tushare", "baostock", "tiingo", "qlib"}
	for _, n := range names {
		r.Register(&mockCollector{name: n})
	}
	for i := 0; i < 100; i++ { // map 迭代顺序随机：多次调用才能暴露
		all := r.GetAll()
		got := make([]string, len(all))
		for j, c := range all {
			got[j] = c.Name()
		}
		if !reflect.DeepEqual(got, names) {
			t.Fatalf("GetAll 第 %d 次 = %v, want %v", i, got, names)
		}
	}
}

func TestRegistry_ReRegisterKeepsPosition(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockCollector{name: "a"})
	r.Register(&mockCollector{name: "b"})
	replacement := &mockCollector{name: "a"}
	r.Register(replacement)

	all := r.GetAll()
	if len(all) != 2 || all[0] != replacement || all[1].Name() != "b" {
		t.Fatalf("同名重注册应覆盖且保持原位置, got %v", all)
	}
	if c, ok := r.Get("a"); !ok || c != replacement {
		t.Fatalf("Get(\"a\") = %v, %v; want replacement", c, ok)
	}
}

func TestRegistry_GetAllEmpty(t *testing.T) {
	all := NewRegistry().GetAll()
	if all == nil || len(all) != 0 {
		t.Fatalf("空 Registry 的 GetAll 应返回长度 0 的切片, got %#v", all)
	}
}

func TestRegistry_ConcurrentRegisterGetAll(t *testing.T) {
	r := NewRegistry()
	const distinct = 10
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(2)
		go func() { // 每个 goroutine 注册同一组名字：不同名与同名重注册混合
			defer wg.Done()
			for i := 0; i < distinct; i++ {
				r.Register(&mockCollector{name: fmt.Sprintf("c%d", i)})
			}
		}()
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				_ = r.GetAll()
			}
		}()
	}
	wg.Wait()

	all := r.GetAll()
	if len(all) != distinct {
		t.Fatalf("GetAll 长度 = %d, want %d", len(all), distinct)
	}
	seen := make(map[string]bool, len(all))
	for _, c := range all {
		if seen[c.Name()] {
			t.Fatalf("GetAll 出现重复名 %q", c.Name())
		}
		seen[c.Name()] = true
	}
}
