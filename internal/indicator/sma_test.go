package indicator

import (
	"math"
	"testing"
)

func TestSMA_Calculate(t *testing.T) {
	prices := []float64{10, 11, 12, 13, 14, 15}

	sma := SMA(prices, 3)

	// SMA(3) for [10,11,12,13,14,15]:
	// [0] = (10+11+12)/3 = 11
	// [1] = (11+12+13)/3 = 12
	// [2] = (12+13+14)/3 = 13
	// [3] = (13+14+15)/3 = 14

	expected := []float64{11, 12, 13, 14}

	if len(sma) != len(expected) {
		t.Fatalf("expected %d values, got %d", len(expected), len(sma))
	}

	for i, v := range expected {
		if sma[i] != v {
			t.Errorf("sma[%d] = %f, want %f", i, sma[i], v)
		}
	}
}

func TestSMA_NotEnoughData(t *testing.T) {
	prices := []float64{10, 11}
	sma := SMA(prices, 5)

	if len(sma) != 0 {
		t.Errorf("expected empty slice, got %d values", len(sma))
	}
}

func TestEMA_Calculate(t *testing.T) {
	prices := []float64{10, 11, 12, 13, 14, 15}
	ema := EMA(prices, 3)

	if len(ema) != 4 {
		t.Fatalf("expected 4 values, got %d", len(ema))
	}

	// First EMA = SMA = 11
	if ema[0] != 11 {
		t.Errorf("first EMA should equal SMA, got %f", ema[0])
	}

	// Subsequent EMAs should trend upward
	for i := 1; i < len(ema); i++ {
		if ema[i] <= ema[i-1] {
			t.Errorf("EMA should be increasing, ema[%d]=%f <= ema[%d]=%f", i, ema[i], i-1, ema[i-1])
		}
	}
}

func TestEMA_NotEnoughData(t *testing.T) {
	prices := []float64{10, 11}
	ema := EMA(prices, 5)

	if len(ema) != 0 {
		t.Errorf("expected empty slice, got %d values", len(ema))
	}
}

func almostEqual(a, b, tolerance float64) bool {
	return math.Abs(a-b) < tolerance
}
