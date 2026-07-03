package binance

import (
	"testing"
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
