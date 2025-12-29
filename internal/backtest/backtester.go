package backtest

import (
	"context"
	"errors"
	"time"

	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/strategy"
)

// OHLCVProvider defines the interface for fetching historical OHLCV data
type OHLCVProvider interface {
	FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error)
}

// Backtester runs strategy backtests against historical data
type Backtester struct {
	provider OHLCVProvider
}

// New creates a new Backtester with the given OHLCV provider
func New(provider OHLCVProvider) *Backtester {
	return &Backtester{
		provider: provider,
	}
}

// Run executes a backtest for the given strategy and symbol over the specified time range
func (b *Backtester) Run(ctx context.Context, strat strategy.Strategy, symbol string, start, end time.Time) (*Result, error) {
	// Fetch historical data
	ohlcv, err := b.provider.FetchHistory(symbol, start, end, "1d")
	if err != nil {
		return nil, err
	}

	if len(ohlcv) == 0 {
		return nil, errors.New("no historical data available")
	}

	// Get strategy data requirements
	req := strat.RequiredData()
	windowSize := req.PriceHistory
	if windowSize <= 0 {
		windowSize = 1
	}

	var allSignals []core.Signal

	// Run strategy on each bar with rolling window
	for i := 0; i < len(ohlcv); i++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Build rolling window
		windowStart := max(0, i-windowSize+1)
		window := ohlcv[windowStart : i+1]

		// Create analysis context
		analysisCtx := strategy.AnalysisContext{
			Symbol: symbol,
			OHLCV:  window,
			Now:    ohlcv[i].Time,
		}

		// Run strategy analysis
		signals, err := strat.Analyze(analysisCtx)
		if err != nil {
			continue // Skip bars with analysis errors
		}

		// Collect signals with price from OHLCV close
		for _, sig := range signals {
			sig.Price = ohlcv[i].Close
			sig.Strategy = strat.Name()
			allSignals = append(allSignals, sig)
		}
	}

	// Convert signals to trades
	trades := signalsToTrades(allSignals, ohlcv)

	// Calculate statistics
	stats := CalculateStats(trades)

	return &Result{
		Strategy:  strat.Name(),
		Symbol:    symbol,
		StartDate: start,
		EndDate:   end,
		Signals:   allSignals,
		Trades:    trades,
		Stats:     stats,
	}, nil
}

// signalsToTrades converts a series of signals into trades
func signalsToTrades(signals []core.Signal, ohlcv []core.OHLCV) []Trade {
	var trades []Trade
	var openTrade *Trade

	for _, sig := range signals {
		switch sig.Action {
		case core.ActionBuy, core.ActionStrongBuy:
			// Only open a new trade if not already in a position
			if openTrade == nil {
				openTrade = &Trade{
					EntrySignal: sig,
					EntryPrice:  sig.Price,
				}
			}
		case core.ActionSell, core.ActionStrongSell:
			// Close the open trade if we have one
			if openTrade != nil {
				sigCopy := sig
				openTrade.ExitSignal = &sigCopy
				openTrade.ExitPrice = sig.Price
				openTrade.Return = (openTrade.ExitPrice - openTrade.EntryPrice) / openTrade.EntryPrice
				trades = append(trades, *openTrade)
				openTrade = nil
			}
		}
	}

	// Append any open trade at end (position still open)
	if openTrade != nil {
		// Use the last OHLCV close as the current price for open positions
		if len(ohlcv) > 0 {
			openTrade.ExitPrice = ohlcv[len(ohlcv)-1].Close
			openTrade.Return = (openTrade.ExitPrice - openTrade.EntryPrice) / openTrade.EntryPrice
		}
		trades = append(trades, *openTrade)
	}

	return trades
}

// max returns the larger of two integers
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
