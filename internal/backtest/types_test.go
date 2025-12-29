package backtest

import (
	"testing"

	"github.com/newthinker/atlas/internal/core"
)

func TestTrade_IsWin(t *testing.T) {
	tests := []struct {
		name  string
		trade Trade
		want  bool
	}{
		{"positive return", Trade{Return: 0.05}, true},
		{"negative return", Trade{Return: -0.02}, false},
		{"zero return", Trade{Return: 0}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.trade.IsWin(); got != tt.want {
				t.Errorf("IsWin() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTrade_IsClosed(t *testing.T) {
	openTrade := Trade{EntrySignal: core.Signal{Symbol: "TEST"}}
	closedTrade := Trade{
		EntrySignal: core.Signal{Symbol: "TEST"},
		ExitSignal:  &core.Signal{Symbol: "TEST"},
	}

	if openTrade.IsClosed() {
		t.Error("open trade should not be closed")
	}
	if !closedTrade.IsClosed() {
		t.Error("closed trade should be closed")
	}
}
