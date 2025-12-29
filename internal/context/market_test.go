// internal/context/market_test.go
package context

import (
	"context"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

func TestMarketContextService_GetContext_NoCollector(t *testing.T) {
	service := NewMarketContextService(nil)

	ctx := context.Background()
	mc, err := service.GetContext(ctx, core.MarketCNA)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mc.Market != core.MarketCNA {
		t.Errorf("expected market CNA, got %s", mc.Market)
	}
	if mc.Regime != RegimeSideways {
		t.Errorf("expected sideways regime, got %s", mc.Regime)
	}
}

func TestCalculateRegime_Bull(t *testing.T) {
	// Create data with upward trend
	data := make([]core.OHLCV, 40)
	for i := 0; i < 40; i++ {
		data[i] = core.OHLCV{
			Time:  time.Now().AddDate(0, 0, -40+i),
			Close: 100 + float64(i)*0.5, // Gradual increase
		}
	}

	regime := calculateRegime(data)
	if regime != RegimeBull {
		t.Errorf("expected bull regime, got %s", regime)
	}
}

func TestCalculateRegime_Bear(t *testing.T) {
	// Create data with downward trend
	data := make([]core.OHLCV, 40)
	for i := 0; i < 40; i++ {
		data[i] = core.OHLCV{
			Time:  time.Now().AddDate(0, 0, -40+i),
			Close: 100 - float64(i)*0.5, // Gradual decrease
		}
	}

	regime := calculateRegime(data)
	if regime != RegimeBear {
		t.Errorf("expected bear regime, got %s", regime)
	}
}

func TestCalculateRegime_Sideways(t *testing.T) {
	// Create flat data
	data := make([]core.OHLCV, 40)
	for i := 0; i < 40; i++ {
		data[i] = core.OHLCV{
			Time:  time.Now().AddDate(0, 0, -40+i),
			Close: 100, // No change
		}
	}

	regime := calculateRegime(data)
	if regime != RegimeSideways {
		t.Errorf("expected sideways regime, got %s", regime)
	}
}

func TestCalculateVolatility(t *testing.T) {
	// Create data with known volatility pattern
	data := []core.OHLCV{
		{Close: 100},
		{Close: 101}, // 1% return
		{Close: 100}, // -0.99% return
		{Close: 102}, // 2% return
		{Close: 101}, // -0.98% return
	}

	vol := calculateVolatility(data)
	if vol < 0.1 || vol > 0.5 {
		t.Errorf("volatility %f seems unreasonable", vol)
	}
}

func TestGetMarketIndex(t *testing.T) {
	tests := []struct {
		market core.Market
		want   string
	}{
		{core.MarketCNA, "000001.SH"},
		{core.MarketHK, "HSI"},
		{core.MarketUS, "SPY"},
	}

	for _, tt := range tests {
		got := getMarketIndex(tt.market)
		if got != tt.want {
			t.Errorf("getMarketIndex(%s) = %s, want %s", tt.market, got, tt.want)
		}
	}
}
