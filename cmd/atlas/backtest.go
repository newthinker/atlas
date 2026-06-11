package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"text/tabwriter"
	"time"

	"github.com/newthinker/atlas/internal/backtest"
	"github.com/newthinker/atlas/internal/collector"
	"github.com/newthinker/atlas/internal/collector/crypto"
	"github.com/newthinker/atlas/internal/collector/eastmoney"
	"github.com/newthinker/atlas/internal/collector/yahoo"
	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/strategy"
	"github.com/newthinker/atlas/internal/strategy/ma_crossover"
	"github.com/newthinker/atlas/internal/strategy/price_percentile"
	"github.com/spf13/cobra"
)

const dateLayout = "2006-01-02"

var (
	backtestSymbol string
	backtestFrom   string
	backtestTo     string
)

var backtestCmd = &cobra.Command{
	Use:   "backtest [strategy]",
	Short: "Run backtest on a strategy",
	Long:  "Run a strategy against historical data and show performance statistics",
	Args:  cobra.ExactArgs(1),
	RunE:  runBacktest,
}

func init() {
	backtestCmd.Flags().StringVar(&backtestSymbol, "symbol", "", "Symbol to backtest (required)")
	backtestCmd.Flags().StringVar(&backtestFrom, "from", "", "Start date YYYY-MM-DD (required)")
	backtestCmd.Flags().StringVar(&backtestTo, "to", "", "End date YYYY-MM-DD (required)")

	backtestCmd.MarkFlagRequired("symbol")
	backtestCmd.MarkFlagRequired("from")
	backtestCmd.MarkFlagRequired("to")

	rootCmd.AddCommand(backtestCmd)
}

// backtestDeps holds the injectable dependencies of the backtest command so the
// core logic can be tested offline and deterministically.
type backtestDeps struct {
	provider   backtest.OHLCVProvider
	strategies *strategy.Engine
	out        io.Writer
}

// staticOHLCVProvider serves pre-fetched bars to the engine, so the history is
// fetched from the real collector only once (for validation) and reused.
type staticOHLCVProvider struct {
	data []core.OHLCV
}

func (s staticOHLCVProvider) FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	return s.data, nil
}

func runBacktest(cmd *cobra.Command, args []string) error {
	reg := collector.NewRegistry()
	reg.Register(yahoo.New())
	reg.Register(eastmoney.New())
	reg.Register(crypto.New())

	provider := collector.SelectForSymbol(reg, backtestSymbol)
	if provider == nil {
		return fmt.Errorf("no collector available for symbol %s", backtestSymbol)
	}

	engine := strategy.NewEngine()
	engine.Register(ma_crossover.New(50, 200))
	// price_percentile works on OHLCV alone, so it backtests offline. pe_percentile
	// is intentionally not registered: it reads a precomputed PE percentile from an
	// online valuation source the backtest engine does not provide.
	engine.Register(price_percentile.New())

	deps := backtestDeps{provider: provider, strategies: engine, out: os.Stdout}
	return executeBacktest(deps, args[0], backtestSymbol, backtestFrom, backtestTo)
}

// parseBacktestDate parses a YYYY-MM-DD date, wrapping failures with which flag
// (e.g. "from"/"to") produced them.
func parseBacktestDate(flag, value string) (time.Time, error) {
	t, err := time.Parse(dateLayout, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid %s date format (expected YYYY-MM-DD): %w", flag, err)
	}
	return t, nil
}

// executeBacktest parses inputs, runs the backtest engine over historical data
// and renders the result. It returns a non-nil error (non-zero exit) on invalid
// input, an unknown strategy or a data-fetch failure; empty data is reported as
// a friendly no-op.
func executeBacktest(deps backtestDeps, strategyName, symbol, fromStr, toStr string) error {
	from, err := parseBacktestDate("from", fromStr)
	if err != nil {
		return err
	}
	to, err := parseBacktestDate("to", toStr)
	if err != nil {
		return err
	}
	if to.Before(from) {
		return fmt.Errorf("end date must be after start date")
	}

	strat, ok := deps.strategies.Get(strategyName)
	if !ok {
		fmt.Fprintf(deps.out, "unknown strategy %q\nAvailable strategies: %v\n",
			strategyName, deps.strategies.GetStrategyNames())
		return fmt.Errorf("unknown strategy: %s", strategyName)
	}

	data, err := deps.provider.FetchHistory(symbol, from, to, "1d")
	if err != nil {
		return fmt.Errorf("fetching history for %s: %w", symbol, err)
	}
	if len(data) == 0 {
		fmt.Fprintf(deps.out, "No historical data for %s between %s and %s.\n",
			symbol, from.Format(dateLayout), to.Format(dateLayout))
		return nil
	}

	result, err := backtest.New(staticOHLCVProvider{data: data}).Run(context.Background(), strat, symbol, from, to)
	if err != nil {
		return fmt.Errorf("running backtest: %w", err)
	}

	printBacktestResult(deps.out, result, from, to)
	return nil
}

// printBacktestResult renders the backtest result as a simple table.
func printBacktestResult(out io.Writer, r *backtest.Result, from, to time.Time) {
	fmt.Fprintln(out, "=== ATLAS Backtest ===")
	fmt.Fprintf(out, "Strategy: %s\n", r.Strategy)
	fmt.Fprintf(out, "Symbol:   %s\n", r.Symbol)
	fmt.Fprintf(out, "Period:   %s to %s\n\n", from.Format(dateLayout), to.Format(dateLayout))

	s := r.Stats
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "Signals:\t%d\n", len(r.Signals))
	fmt.Fprintf(w, "Trades:\t%d\n", len(r.Trades))
	fmt.Fprintf(w, "Winning Trades:\t%d\n", s.WinningTrades)
	fmt.Fprintf(w, "Losing Trades:\t%d\n", s.LosingTrades)
	fmt.Fprintf(w, "Win Rate:\t%.2f%%\n", s.WinRate)
	fmt.Fprintf(w, "Total Return:\t%.2f%%\n", s.TotalReturn)
	fmt.Fprintf(w, "Max Drawdown:\t%.2f%%\n", s.MaxDrawdown)
	fmt.Fprintf(w, "Sharpe Ratio:\t%.2f\n", s.SharpeRatio)
	w.Flush()
}
