package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/newthinker/atlas/internal/backtest"
	"github.com/newthinker/atlas/internal/collector"
	"github.com/newthinker/atlas/internal/collector/crypto"
	"github.com/newthinker/atlas/internal/collector/eastmoney"
	"github.com/newthinker/atlas/internal/collector/yahoo"
	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/strategy"
	"github.com/newthinker/atlas/internal/strategy/dividend_yield"
	"github.com/newthinker/atlas/internal/strategy/ma_crossover"
	"github.com/newthinker/atlas/internal/strategy/pe_band"
	"github.com/newthinker/atlas/internal/strategy/pe_percentile"
	"github.com/newthinker/atlas/internal/strategy/price_percentile"
	"github.com/spf13/cobra"
)

var (
	exportStrategies []string
	exportSymbols    []string
	exportFrom       string
	exportTo         string
	exportOut        string
)

var exportCmd = &cobra.Command{
	Use:   "export-signals",
	Short: "Replay strategies over history and export raw signals as CSV",
	Long: "Replay the given strategies over historical data and export the raw, " +
		"pre-router signals as CSV for offline evaluation (e.g. qlib).",
	RunE: runExportSignals,
}

func init() {
	exportCmd.Flags().StringSliceVar(&exportStrategies, "strategies", nil, "Comma-separated strategy names (required)")
	exportCmd.Flags().StringSliceVar(&exportSymbols, "symbols", nil, "Comma-separated symbols (required)")
	exportCmd.Flags().StringVar(&exportFrom, "from", "", "Start date YYYY-MM-DD (required)")
	exportCmd.Flags().StringVar(&exportTo, "to", "", "End date YYYY-MM-DD (required)")
	exportCmd.Flags().StringVar(&exportOut, "out", "signals.csv", `Output file ("-" for stdout)`)

	exportCmd.MarkFlagRequired("strategies")
	exportCmd.MarkFlagRequired("symbols")
	exportCmd.MarkFlagRequired("from")
	exportCmd.MarkFlagRequired("to")

	rootCmd.AddCommand(exportCmd)
}

// exportParams holds the parsed CLI inputs for an export run.
type exportParams struct {
	Strategies []string
	Symbols    []string
	From, To   string
}

// exportDeps holds the injectable dependencies of the export command so the core
// logic can be tested offline and deterministically.
type exportDeps struct {
	provider   backtest.OHLCVProvider // test injects bars; CLI path selects a collector per symbol
	strategies *strategy.Engine
	out        io.Writer // CLI informational output
	errOut     io.Writer // skipped-bar summary
}

// csvHeader is the seven-column contract consumed by the qlib evaluation pipeline.
var csvHeader = []string{"symbol", "date", "strategy", "action", "confidence", "price", "metadata"}

// newExportEngine builds the engine for the CLI path with ALL strategies
// registered (including fundamentals-dependent ones). Registering them is
// required so that `--strategies pe_band` reaches the explicit "requires
// fundamentals" rejection in executeExport instead of the unknown-strategy
// branch (design §2.1/§5). Registration is not execution: fundamentals
// strategies are rejected by the whitelist before any replay.
func newExportEngine() *strategy.Engine {
	e := strategy.NewEngine()
	e.Register(ma_crossover.New(50, 200))
	e.Register(price_percentile.New())
	// Fundamentals strategies — registered only so the whitelist can reject them
	// explicitly; the constructor thresholds are never exercised offline.
	e.Register(pe_band.New(15, 30))
	e.Register(dividend_yield.New(3.0))
	e.Register(pe_percentile.New())
	return e
}

// registryProvider adapts a collector registry to OHLCVProvider, selecting the
// right collector per symbol on each fetch.
type registryProvider struct {
	reg *collector.Registry
}

func (r registryProvider) FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	p := collector.SelectForSymbol(r.reg, symbol)
	if p == nil {
		return nil, fmt.Errorf("no collector available for symbol %s", symbol)
	}
	return p.FetchHistory(symbol, start, end, interval)
}

func runExportSignals(cmd *cobra.Command, args []string) error {
	reg := collector.NewRegistry()
	reg.Register(yahoo.New())
	reg.Register(eastmoney.New())
	reg.Register(crypto.New())

	out := os.Stdout
	var w io.Writer = out
	if exportOut != "-" {
		f, err := os.Create(exportOut)
		if err != nil {
			return fmt.Errorf("creating output file %s: %w", exportOut, err)
		}
		defer f.Close()
		w = f
	}

	deps := exportDeps{
		provider:   registryProvider{reg: reg},
		strategies: newExportEngine(),
		out:        out,
		errOut:     os.Stderr,
	}
	if err := executeExport(deps, w, exportParams{
		Strategies: exportStrategies,
		Symbols:    exportSymbols,
		From:       exportFrom,
		To:         exportTo,
	}); err != nil {
		return err
	}
	if exportOut != "-" {
		fmt.Fprintf(out, "Wrote signals to %s\n", exportOut)
	}
	return nil
}

// executeExport replays each (symbol, strategy) pair through the backtest engine
// and writes raw signals (pre-router, per design §2.2) as CSV to w. The three
// writers are distinct: w receives the CSV data, deps.out CLI information and
// deps.errOut the skipped-bar summary.
//
// To give warm-up-dependent strategies enough history, data is fetched from a
// start shifted back by maxBars*365/252+30 calendar days (the app.historyWindowDays
// convention), and only signals with GeneratedAt >= from are emitted.
func executeExport(deps exportDeps, w io.Writer, p exportParams) error {
	from, err := parseBacktestDate("from", p.From)
	if err != nil {
		return err
	}
	to, err := parseBacktestDate("to", p.To)
	if err != nil {
		return err
	}
	if to.Before(from) {
		return fmt.Errorf("end date must be after start date")
	}

	// Validate the whole whitelist before fetching any data.
	strats := make([]strategy.Strategy, 0, len(p.Strategies))
	maxBars := 0
	for _, name := range p.Strategies {
		strat, ok := deps.strategies.Get(name)
		if !ok {
			return fmt.Errorf("unknown strategy %q (available: %s)", name, strings.Join(offlineNames(deps.strategies), ", "))
		}
		req := strat.RequiredData()
		if req.Fundamentals {
			return fmt.Errorf("strategy %q requires fundamentals and cannot be replayed offline (available: %s)",
				name, strings.Join(offlineNames(deps.strategies), ", "))
		}
		if req.PriceHistory > maxBars {
			maxBars = req.PriceHistory
		}
		strats = append(strats, strat)
	}

	warmupStart := from.AddDate(0, 0, -(maxBars*365/252 + 30))

	cw := csv.NewWriter(w)
	if err := cw.Write(csvHeader); err != nil {
		return err
	}

	totalSkipped := 0
	for _, symbol := range p.Symbols {
		for _, strat := range strats {
			result, err := backtest.New(deps.provider).Run(context.Background(), strat, symbol, warmupStart, to)
			if err != nil {
				return fmt.Errorf("replaying %s on %s: %w", strat.Name(), symbol, err)
			}
			totalSkipped += result.SkippedBars
			for _, sig := range result.Signals {
				if sig.GeneratedAt.Before(from) {
					continue // warm-up signal before the requested window
				}
				row, err := signalRow(symbol, sig)
				if err != nil {
					return err
				}
				if err := cw.Write(row); err != nil {
					return err
				}
			}
		}
	}

	cw.Flush()
	if err := cw.Error(); err != nil {
		return err
	}
	if totalSkipped > 0 {
		fmt.Fprintf(deps.errOut, "warning: skipped %d bar(s) with analysis errors during replay\n", totalSkipped)
	}
	return nil
}

// signalRow renders one signal as the seven CSV columns. metadata is JSON-encoded
// (nil → empty string).
func signalRow(symbol string, sig core.Signal) ([]string, error) {
	meta := ""
	if sig.Metadata != nil {
		b, err := json.Marshal(sig.Metadata)
		if err != nil {
			return nil, fmt.Errorf("marshaling metadata for %s: %w", symbol, err)
		}
		meta = string(b)
	}
	return []string{
		symbol,
		sig.GeneratedAt.Format(dateLayout),
		sig.Strategy,
		string(sig.Action),
		fmt.Sprintf("%.2f", sig.Confidence),
		fmt.Sprintf("%.2f", sig.Price),
		meta,
	}, nil
}

// offlineNames lists the registered strategies that can be replayed offline
// (those not requiring fundamentals), sorted for stable error messages.
func offlineNames(e *strategy.Engine) []string {
	var names []string
	for _, name := range e.GetStrategyNames() {
		if s, ok := e.Get(name); ok && !s.RequiredData().Fundamentals {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}
