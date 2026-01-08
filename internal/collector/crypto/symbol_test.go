package crypto

import (
	"testing"
)

func TestNormalizeSymbol(t *testing.T) {
	tests := []struct {
		input        string
		defaultQuote string
		expected     string
	}{
		// Basic cases - add default quote
		{"BTC", "USDT", "BTCUSDT"},
		{"btc", "USDT", "BTCUSDT"},
		{"eth", "USDT", "ETHUSDT"},
		{"ETH", "USDT", "ETHUSDT"},

		// With separators
		{"BTC-USDT", "USDT", "BTCUSDT"},
		{"BTC/USDT", "USDT", "BTCUSDT"},
		{"btc-usdt", "USDT", "BTCUSDT"},
		{"btc/usdt", "USDT", "BTCUSDT"},
		{"BTC_USDT", "USDT", "BTCUSDT"},

		// Already normalized
		{"BTCUSDT", "USDT", "BTCUSDT"},
		{"btcusdt", "USDT", "BTCUSDT"},
		{"ETHUSDT", "USDT", "ETHUSDT"},

		// Different quote currencies
		{"BTC-BUSD", "USDT", "BTCBUSD"},
		{"ETH/BTC", "USDT", "ETHBTC"},
		{"BTC", "BUSD", "BTCBUSD"},

		// Edge cases
		{"SOLUSDT", "USDT", "SOLUSDT"},
		{"SOL", "USDT", "SOLUSDT"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := NormalizeSymbol(tc.input, tc.defaultQuote)
			if got != tc.expected {
				t.Errorf("NormalizeSymbol(%q, %q) = %q, want %q",
					tc.input, tc.defaultQuote, got, tc.expected)
			}
		})
	}
}

func TestParseSymbol(t *testing.T) {
	tests := []struct {
		symbol        string
		expectedBase  string
		expectedQuote string
	}{
		{"BTCUSDT", "BTC", "USDT"},
		{"ETHUSDT", "ETH", "USDT"},
		{"ETHBTC", "ETH", "BTC"},
		{"SOLUSDT", "SOL", "USDT"},
		{"BTCBUSD", "BTC", "BUSD"},
		{"DOGEUSDT", "DOGE", "USDT"},
	}

	for _, tc := range tests {
		t.Run(tc.symbol, func(t *testing.T) {
			base, quote := ParseSymbol(tc.symbol)
			if base != tc.expectedBase || quote != tc.expectedQuote {
				t.Errorf("ParseSymbol(%q) = (%q, %q), want (%q, %q)",
					tc.symbol, base, quote, tc.expectedBase, tc.expectedQuote)
			}
		})
	}
}

func TestFormatDisplay(t *testing.T) {
	tests := []struct {
		symbol   string
		expected string
	}{
		{"BTCUSDT", "BTC/USDT"},
		{"ETHUSDT", "ETH/USDT"},
		{"ETHBTC", "ETH/BTC"},
	}

	for _, tc := range tests {
		t.Run(tc.symbol, func(t *testing.T) {
			got := FormatDisplay(tc.symbol)
			if got != tc.expected {
				t.Errorf("FormatDisplay(%q) = %q, want %q", tc.symbol, got, tc.expected)
			}
		})
	}
}

func TestValidateCryptoSymbol(t *testing.T) {
	tests := []struct {
		name    string
		symbol  string
		wantErr bool
	}{
		{"valid symbol", "BTCUSDT", false},
		{"valid lowercase", "btcusdt", false},
		{"empty symbol", "", true},
		{"too long", "VERYLONGSYMBOLNAME12345678901234567890", true},
		{"invalid chars", "BTC!USDT", true},
		{"path injection", "../etc/passwd", true},
		{"url injection", "BTC?foo=bar", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCryptoSymbol(tt.symbol)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCryptoSymbol(%q) error = %v, wantErr %v",
					tt.symbol, err, tt.wantErr)
			}
		})
	}
}
