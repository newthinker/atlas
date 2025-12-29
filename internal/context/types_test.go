// internal/context/types_test.go
package context

import (
	"testing"
)

func TestMarketRegime_Values(t *testing.T) {
	tests := []struct {
		regime MarketRegime
		want   string
	}{
		{RegimeBull, "bull"},
		{RegimeBear, "bear"},
		{RegimeSideways, "sideways"},
	}

	for _, tt := range tests {
		if string(tt.regime) != tt.want {
			t.Errorf("expected %s, got %s", tt.want, tt.regime)
		}
	}
}

func TestTrend_Values(t *testing.T) {
	tests := []struct {
		trend Trend
		want  string
	}{
		{TrendUp, "up"},
		{TrendDown, "down"},
		{TrendFlat, "flat"},
	}

	for _, tt := range tests {
		if string(tt.trend) != tt.want {
			t.Errorf("expected %s, got %s", tt.want, tt.trend)
		}
	}
}
