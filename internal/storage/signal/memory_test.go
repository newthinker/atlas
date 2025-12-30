// internal/storage/signal/memory_test.go
package signal

import (
	"context"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

func TestMemoryStore_SaveAndList(t *testing.T) {
	store := NewMemoryStore(100)
	ctx := context.Background()

	sig := core.Signal{
		Symbol:      "AAPL",
		Action:      core.ActionBuy,
		Confidence:  0.85,
		Strategy:    "ma_crossover",
		GeneratedAt: time.Now(),
	}

	err := store.Save(ctx, sig)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	signals, err := store.List(ctx, ListFilter{Symbol: "AAPL"})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(signals) != 1 {
		t.Errorf("expected 1 signal, got %d", len(signals))
	}
}

func TestMemoryStore_ListByStrategy(t *testing.T) {
	store := NewMemoryStore(100)
	ctx := context.Background()

	store.Save(ctx, core.Signal{Symbol: "AAPL", Strategy: "ma_crossover", GeneratedAt: time.Now()})
	store.Save(ctx, core.Signal{Symbol: "GOOG", Strategy: "pe_band", GeneratedAt: time.Now()})

	signals, _ := store.List(ctx, ListFilter{Strategy: "ma_crossover"})
	if len(signals) != 1 {
		t.Errorf("expected 1, got %d", len(signals))
	}
}

func TestMemoryStore_ListByTimeRange(t *testing.T) {
	store := NewMemoryStore(100)
	ctx := context.Background()

	now := time.Now()
	store.Save(ctx, core.Signal{Symbol: "AAPL", GeneratedAt: now.Add(-2 * time.Hour)})
	store.Save(ctx, core.Signal{Symbol: "GOOG", GeneratedAt: now})

	signals, _ := store.List(ctx, ListFilter{From: now.Add(-1 * time.Hour)})
	if len(signals) != 1 {
		t.Errorf("expected 1, got %d", len(signals))
	}
}

func TestMemoryStore_MaxSize(t *testing.T) {
	store := NewMemoryStore(2)
	ctx := context.Background()

	store.Save(ctx, core.Signal{Symbol: "A", GeneratedAt: time.Now()})
	store.Save(ctx, core.Signal{Symbol: "B", GeneratedAt: time.Now()})
	store.Save(ctx, core.Signal{Symbol: "C", GeneratedAt: time.Now()})

	signals, _ := store.List(ctx, ListFilter{})
	if len(signals) != 2 {
		t.Errorf("expected 2 (max size), got %d", len(signals))
	}
}

func TestMemoryStore_GetByID(t *testing.T) {
	store := NewMemoryStore(100)
	ctx := context.Background()

	sig := core.Signal{Symbol: "AAPL", GeneratedAt: time.Now()}
	store.Save(ctx, sig)

	signals, _ := store.List(ctx, ListFilter{})
	if len(signals) == 0 {
		t.Fatal("no signals saved")
	}

	retrieved, err := store.GetByID(ctx, signals[0].ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if retrieved.Symbol != "AAPL" {
		t.Errorf("wrong symbol: %s", retrieved.Symbol)
	}
}
