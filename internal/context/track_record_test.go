// internal/context/track_record_test.go
package context

import (
	"context"
	"testing"
)

func TestInMemoryTrackRecord_GetStats_New(t *testing.T) {
	tr := NewInMemoryTrackRecord()
	ctx := context.Background()

	stats, err := tr.GetStats(ctx, "strategy1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stats.Strategy != "strategy1" {
		t.Errorf("expected strategy1, got %s", stats.Strategy)
	}
	if stats.TotalSignals != 0 {
		t.Errorf("expected 0 signals, got %d", stats.TotalSignals)
	}
}

func TestInMemoryTrackRecord_UpdateStats(t *testing.T) {
	tr := NewInMemoryTrackRecord()
	ctx := context.Background()

	tr.UpdateStats("strategy1", &StrategyStats{
		Strategy:     "strategy1",
		TotalSignals: 100,
		WinRate:      0.6,
		AvgReturn:    0.05,
	})

	stats, err := tr.GetStats(ctx, "strategy1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stats.TotalSignals != 100 {
		t.Errorf("expected 100 signals, got %d", stats.TotalSignals)
	}
	if stats.WinRate != 0.6 {
		t.Errorf("expected 0.6 win rate, got %f", stats.WinRate)
	}
}

func TestInMemoryTrackRecord_RecordOutcome(t *testing.T) {
	tr := NewInMemoryTrackRecord()

	// Record a win
	tr.RecordOutcome("strategy1", true, 0.1)

	ctx := context.Background()
	stats, err := tr.GetStats(ctx, "strategy1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stats.TotalSignals != 1 {
		t.Errorf("expected 1 signal, got %d", stats.TotalSignals)
	}
	if stats.WinRate != 1.0 {
		t.Errorf("expected 1.0 win rate, got %f", stats.WinRate)
	}
	if stats.AvgReturn != 0.1 {
		t.Errorf("expected 0.1 avg return, got %f", stats.AvgReturn)
	}

	// Record a loss
	tr.RecordOutcome("strategy1", false, -0.05)

	stats, _ = tr.GetStats(ctx, "strategy1")
	if stats.TotalSignals != 2 {
		t.Errorf("expected 2 signals, got %d", stats.TotalSignals)
	}
	if stats.WinRate != 0.5 {
		t.Errorf("expected 0.5 win rate, got %f", stats.WinRate)
	}
}

func TestInMemoryTrackRecord_GetAllStats(t *testing.T) {
	tr := NewInMemoryTrackRecord()

	tr.UpdateStats("strategy1", &StrategyStats{Strategy: "strategy1", TotalSignals: 10})
	tr.UpdateStats("strategy2", &StrategyStats{Strategy: "strategy2", TotalSignals: 20})

	ctx := context.Background()
	all, err := tr.GetAllStats(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(all) != 2 {
		t.Errorf("expected 2 strategies, got %d", len(all))
	}
	if all["strategy1"].TotalSignals != 10 {
		t.Errorf("expected 10 signals for strategy1, got %d", all["strategy1"].TotalSignals)
	}
	if all["strategy2"].TotalSignals != 20 {
		t.Errorf("expected 20 signals for strategy2, got %d", all["strategy2"].TotalSignals)
	}
}
