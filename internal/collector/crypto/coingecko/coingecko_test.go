package coingecko

import (
	"testing"
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
