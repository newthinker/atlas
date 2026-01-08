package coingecko

import (
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

// TestCoinGecko_HasRequiredMethods verifies CoinGecko has the required provider methods
func TestCoinGecko_HasRequiredMethods(t *testing.T) {
	c := New("")
	// Verify required methods exist by calling them
	_ = c.Name()
	// FetchQuote and FetchHistory signatures are verified by compile-time
}

func TestCoinGecko_Name(t *testing.T) {
	c := New("")
	if c.Name() != "coingecko" {
		t.Errorf("expected 'coingecko', got '%s'", c.Name())
	}
}

func TestCoinGecko_SymbolToID(t *testing.T) {
	tests := []struct {
		symbol   string
		expected string
	}{
		{"BTCUSDT", "bitcoin"},
		{"ETHUSDT", "ethereum"},
		{"BNBUSDT", "binancecoin"},
		{"SOLUSDT", "solana"},
		{"XRPUSDT", "ripple"},
		{"DOGEUSDT", "dogecoin"},
		{"ADAUSDT", "cardano"},
		{"UNKNOWNUSDT", "unknown"}, // Unknown symbol returns lowercase base
	}

	c := New("")
	for _, tc := range tests {
		got := c.symbolToID(tc.symbol)
		if got != tc.expected {
			t.Errorf("symbolToID(%s) = %s, want %s", tc.symbol, got, tc.expected)
		}
	}
}

func TestCoinGecko_SymbolToVsCurrency(t *testing.T) {
	tests := []struct {
		symbol   string
		expected string
	}{
		{"BTCUSDT", "usd"},
		{"ETHBTC", "btc"},
		{"SOLETH", "eth"},
		{"BTCBUSD", "usd"},
	}

	c := New("")
	for _, tc := range tests {
		got := c.symbolToVsCurrency(tc.symbol)
		if got != tc.expected {
			t.Errorf("symbolToVsCurrency(%s) = %s, want %s", tc.symbol, got, tc.expected)
		}
	}
}

// Integration test - skip in CI
func TestCoinGecko_FetchQuote_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	c := New("")
	quote, err := c.FetchQuote("BTCUSDT")
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
	if quote.Source != "coingecko" {
		t.Errorf("expected source coingecko, got %s", quote.Source)
	}
}

// Integration test - skip in CI
func TestCoinGecko_FetchHistory_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	c := New("")
	end := time.Now()
	start := end.AddDate(0, 0, -7) // Last 7 days

	data, err := c.FetchHistory("BTCUSDT", start, end, "1d")
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
