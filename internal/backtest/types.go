package backtest

import (
	"time"

	"github.com/newthinker/atlas/internal/core"
)

// Result holds the complete backtest output
type Result struct {
	Strategy  string
	Symbol    string
	StartDate time.Time
	EndDate   time.Time
	Signals   []core.Signal
	Trades    []Trade
	Stats     Stats
}

// Trade represents a simulated trade from entry to exit
type Trade struct {
	EntrySignal core.Signal
	ExitSignal  *core.Signal // nil if position still open
	EntryPrice  float64
	ExitPrice   float64
	Return      float64 // Percentage return
}

// Stats holds performance statistics
type Stats struct {
	TotalTrades   int
	WinningTrades int
	LosingTrades  int
	WinRate       float64 // Percentage of profitable trades
	TotalReturn   float64 // Net return percentage
	MaxDrawdown   float64 // Largest peak-to-trough decline
	SharpeRatio   float64 // Risk-adjusted return (annualized)
}

// IsWin returns true if the trade was profitable
func (t Trade) IsWin() bool {
	return t.Return > 0
}

// IsClosed returns true if the trade has an exit
func (t Trade) IsClosed() bool {
	return t.ExitSignal != nil
}
