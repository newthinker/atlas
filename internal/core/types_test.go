package core

import (
	"testing"
	"time"
)

func TestQuote_IsValid(t *testing.T) {
	q := Quote{
		Symbol: "600519.SH",
		Market: MarketCNA,
		Price:  1680.50,
		Volume: 1000000,
		Time:   time.Now(),
	}

	if !q.IsValid() {
		t.Error("expected valid quote")
	}

	invalid := Quote{Symbol: "", Price: 0}
	if invalid.IsValid() {
		t.Error("expected invalid quote")
	}
}

func TestMarket_Constants(t *testing.T) {
	markets := []Market{MarketUS, MarketHK, MarketCNA, MarketEU}
	expected := []string{"US", "HK", "CN_A", "EU"}

	for i, m := range markets {
		if string(m) != expected[i] {
			t.Errorf("expected %s, got %s", expected[i], m)
		}
	}
}

func TestAction_Constants(t *testing.T) {
	actions := []Action{ActionBuy, ActionSell, ActionHold, ActionStrongBuy, ActionStrongSell}
	expected := []string{"buy", "sell", "hold", "strong_buy", "strong_sell"}

	for i, a := range actions {
		if string(a) != expected[i] {
			t.Errorf("expected %s, got %s", expected[i], a)
		}
	}
}

func TestFundamental_IsValid(t *testing.T) {
	tests := []struct {
		name string
		f    Fundamental
		want bool
	}{
		{"valid", Fundamental{Symbol: "600519", Date: time.Now()}, true},
		{"empty symbol", Fundamental{Date: time.Now()}, false},
		{"zero date", Fundamental{Symbol: "600519"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.f.IsValid(); got != tt.want {
				t.Errorf("IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}
