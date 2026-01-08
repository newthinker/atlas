package binance

import (
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

// TestBinance_HasRequiredMethods verifies Binance has the required provider methods
func TestBinance_HasRequiredMethods(t *testing.T) {
	b := New()
	// Verify required methods exist by calling them
	_ = b.Name()
	// FetchQuote and FetchHistory signatures are verified by compile-time
}

func TestBinance_Name(t *testing.T) {
	b := New()
	if b.Name() != "binance" {
		t.Errorf("expected 'binance', got '%s'", b.Name())
	}
}

func TestBinance_ToInterval(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"1m", "1m"},
		{"5m", "5m"},
		{"15m", "15m"},
		{"1h", "1h"},
		{"4h", "4h"},
		{"1d", "1d"},
		{"unknown", "1d"},
	}

	b := New()
	for _, tc := range tests {
		got := b.toInterval(tc.input)
		if got != tc.expected {
			t.Errorf("toInterval(%s) = %s, want %s", tc.input, got, tc.expected)
		}
	}
}

// Integration test - skip in CI
func TestBinance_FetchQuote_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	b := New()
	quote, err := b.FetchQuote("BTCUSDT")
	if err != nil {
		t.Fatalf("FetchQuote failed: %v", err)
	}

	if quote.Symbol != "BTCUSDT" {
		t.Errorf("expected symbol BTCUSDT, got %s", quote.Symbol)
	}
	if quote.Price <= 0 {
		t.Errorf("expected positive price, got %f", quote.Price)
	}
	if quote.Market != core.MarketCrypto {
		t.Errorf("expected market CRYPTO, got %s", quote.Market)
	}
}

// Integration test - skip in CI
func TestBinance_FetchHistory_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	b := New()
	end := time.Now()
	start := end.AddDate(0, 0, -7) // Last 7 days

	data, err := b.FetchHistory("BTCUSDT", start, end, "1d")
	if err != nil {
		t.Fatalf("FetchHistory failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("expected at least one OHLCV record")
	}

	for _, ohlcv := range data {
		if ohlcv.Symbol != "BTCUSDT" {
			t.Errorf("expected symbol BTCUSDT, got %s", ohlcv.Symbol)
		}
		if ohlcv.Close <= 0 {
			t.Errorf("expected positive close price, got %f", ohlcv.Close)
		}
	}
}
