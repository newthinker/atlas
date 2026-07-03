//go:build integration

package coingecko

import (
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

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
