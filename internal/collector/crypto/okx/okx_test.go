package okx

import (
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

// TestOKX_HasRequiredMethods verifies OKX has the required provider methods
func TestOKX_HasRequiredMethods(t *testing.T) {
	o := New()
	// Verify required methods exist by calling them
	_ = o.Name()
	// FetchQuote and FetchHistory signatures are verified by compile-time
}

func TestOKX_Name(t *testing.T) {
	o := New()
	if o.Name() != "okx" {
		t.Errorf("expected 'okx', got '%s'", o.Name())
	}
}

func TestOKX_ToInstID(t *testing.T) {
	tests := []struct {
		symbol   string
		expected string
	}{
		{"BTCUSDT", "BTC-USDT"},
		{"ETHUSDT", "ETH-USDT"},
		{"SOLUSDT", "SOL-USDT"},
		{"ETHBTC", "ETH-BTC"},
	}

	o := New()
	for _, tc := range tests {
		got := o.toInstID(tc.symbol)
		if got != tc.expected {
			t.Errorf("toInstID(%s) = %s, want %s", tc.symbol, got, tc.expected)
		}
	}
}

func TestOKX_ToInterval(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"1m", "1m"},
		{"5m", "5m"},
		{"1h", "1H"},
		{"4h", "4H"},
		{"1d", "1D"},
	}

	o := New()
	for _, tc := range tests {
		got := o.toInterval(tc.input)
		if got != tc.expected {
			t.Errorf("toInterval(%s) = %s, want %s", tc.input, got, tc.expected)
		}
	}
}

// Integration test - skip in CI
func TestOKX_FetchQuote_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	o := New()
	quote, err := o.FetchQuote("BTCUSDT")
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
func TestOKX_FetchHistory_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	o := New()
	end := time.Now()
	start := end.AddDate(0, 0, -7) // Last 7 days

	data, err := o.FetchHistory("BTCUSDT", start, end, "1d")
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
