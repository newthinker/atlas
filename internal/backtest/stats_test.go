package backtest

import (
	"math"
	"testing"

	"github.com/newthinker/atlas/internal/core"
)

func TestCalculateStats_Empty(t *testing.T) {
	stats := CalculateStats([]Trade{})
	if stats.TotalTrades != 0 {
		t.Error("expected 0 trades for empty input")
	}
}

func TestCalculateStats_WinRate(t *testing.T) {
	exit := &core.Signal{Symbol: "TEST"}
	trades := []Trade{
		{Return: 0.10, ExitSignal: exit}, // win
		{Return: 0.05, ExitSignal: exit}, // win
		{Return: -0.03, ExitSignal: exit}, // loss
		{Return: 0.02, ExitSignal: exit}, // win
	}

	stats := CalculateStats(trades)

	if stats.TotalTrades != 4 {
		t.Errorf("TotalTrades = %d, want 4", stats.TotalTrades)
	}
	if stats.WinningTrades != 3 {
		t.Errorf("WinningTrades = %d, want 3", stats.WinningTrades)
	}
	if stats.WinRate != 75 {
		t.Errorf("WinRate = %f, want 75", stats.WinRate)
	}
}

func TestCalculateStats_TotalReturn(t *testing.T) {
	exit := &core.Signal{Symbol: "TEST"}
	trades := []Trade{
		{Return: 0.10, ExitSignal: exit},
		{Return: -0.05, ExitSignal: exit},
	}

	stats := CalculateStats(trades)

	expected := 5.0 // (0.10 + -0.05) * 100
	if math.Abs(stats.TotalReturn-expected) > 0.001 {
		t.Errorf("TotalReturn = %f, want %f", stats.TotalReturn, expected)
	}
}

func TestCalculateMaxDrawdown(t *testing.T) {
	// Simulate: +10%, +5%, -20%, +10%
	// Peak at 1.155, trough at 0.924, DD = 20%
	returns := []float64{0.10, 0.05, -0.20, 0.10}
	dd := calculateMaxDrawdown(returns)

	if dd < 0.19 || dd > 0.21 {
		t.Errorf("MaxDrawdown = %f, expected ~0.20", dd)
	}
}

func TestCalculateStats_IgnoresOpenTrades(t *testing.T) {
	exit := &core.Signal{Symbol: "TEST"}
	trades := []Trade{
		{Return: 0.10, ExitSignal: exit},  // closed
		{Return: 0.05, ExitSignal: nil},   // open - should be ignored
	}

	stats := CalculateStats(trades)

	if stats.WinningTrades != 1 {
		t.Errorf("should only count closed trades, got %d", stats.WinningTrades)
	}
}
