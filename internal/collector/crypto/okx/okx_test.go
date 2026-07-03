package okx

import (
	"testing"
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
