package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/newthinker/atlas/internal/app"
	"github.com/newthinker/atlas/internal/text"
)

var (
	watchlistJSON    bool
	watchlistSymbols string
)

var watchlistCmd = &cobra.Command{
	Use:   "watchlist",
	Short: "Show quote/valuation/percentile metrics for all watchlist symbols",
	Long: `Fetches the latest price, change %, PE/PB/dividend yield and PE/price
percentiles for every watchlist symbol, offline (no running serve needed).
Assembly reuses the analysis loop's exact valuation pipeline.`,
	RunE: runWatchlist,
}

func init() {
	watchlistCmd.Flags().BoolVar(&watchlistJSON, "json", false, "output JSON instead of a table")
	watchlistCmd.Flags().StringVar(&watchlistSymbols, "symbols", "", "comma-separated subset of watchlist symbols")
	rootCmd.AddCommand(watchlistCmd)
}

// watchlistParams are the parsed CLI inputs.
type watchlistParams struct {
	jsonOut bool
	symbols []string // nil = full watchlist
}

// watchlistDeps injects the snapshot function and writers so executeWatchlist
// is unit-testable (mirrors exportDeps).
type watchlistDeps struct {
	snapshot func(ctx context.Context, symbols []string) []app.SymbolMetrics
	known    []string // watchlist symbols, for --symbols validation
	out      io.Writer
	errOut   io.Writer
}

func runWatchlist(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfigOrDefaults()
	if err != nil {
		return err
	}
	log := newStderrLogger()
	application := app.New(cfg, log)
	cleanup, err := buildCollectors(cfg, application, log)
	if err != nil {
		return err
	}
	defer cleanup()
	known := make([]string, 0, len(cfg.Watchlist))
	for _, item := range cfg.Watchlist {
		application.AddToWatchlistWithDetails(item.Symbol, item.Name, item.Market, item.Type, item.Strategies)
		known = append(known, item.Symbol)
	}

	deps := watchlistDeps{snapshot: application.SnapshotMetrics, known: known, out: os.Stdout, errOut: os.Stderr}
	params := watchlistParams{jsonOut: watchlistJSON}
	if s := strings.TrimSpace(watchlistSymbols); s != "" {
		params.symbols = strings.Split(s, ",")
	}
	return executeWatchlist(cmd.Context(), deps, params)
}

// newStderrLogger keeps stdout clean for table/JSON output; warnings from the
// valuation pipeline stay visible on stderr.
func newStderrLogger() *zap.Logger {
	enc := zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig())
	core := zapcore.NewCore(enc, zapcore.Lock(os.Stderr), zapcore.WarnLevel)
	return zap.New(core)
}

func executeWatchlist(ctx context.Context, deps watchlistDeps, params watchlistParams) error {
	filter, err := resolveSymbolFilter(deps, params.symbols)
	if err != nil {
		return err
	}
	ms := deps.snapshot(ctx, filter)
	if len(ms) == 0 {
		fmt.Fprintln(deps.errOut, "watchlist is empty — add symbols in config.yaml or via the web UI")
		return nil
	}
	if allFailed(ms) {
		renderGaps(deps.errOut, ms)
		return fmt.Errorf("all %d symbols failed to fetch any metric", len(ms))
	}
	if params.jsonOut {
		return json.NewEncoder(deps.out).Encode(ms)
	}
	renderTable(deps.out, ms)
	renderGaps(deps.out, ms)
	return nil
}

// resolveSymbolFilter validates --symbols against the watchlist: unknown
// symbols warn and are dropped; nothing left is an error; no filter = nil.
func resolveSymbolFilter(deps watchlistDeps, requested []string) ([]string, error) {
	if len(requested) == 0 {
		return nil, nil
	}
	known := make(map[string]bool, len(deps.known))
	for _, s := range deps.known {
		known[s] = true
	}
	var valid []string
	for _, s := range requested {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if known[s] {
			valid = append(valid, s)
		} else {
			fmt.Fprintf(deps.errOut, "warning: %s is not in the watchlist, skipping\n", s)
		}
	}
	if len(valid) == 0 {
		return nil, fmt.Errorf("none of the requested symbols are in the watchlist")
	}
	return valid, nil
}

// allFailed reports whether every symbol yielded no metric at all.
func allFailed(ms []app.SymbolMetrics) bool {
	for _, m := range ms {
		if m.Price != 0 || m.PricePercentile != nil || m.PEPercentile != nil || m.PE != nil {
			return false
		}
	}
	return true
}

const naCell = "—"

func renderTable(w io.Writer, ms []app.SymbolMetrics) {
	headers := []string{"SYMBOL", "NAME", "MARKET", "PRICE", "CHG%", "PE", "PB", "DYR%", "PE%ILE", "PX%ILE"}
	rows := make([][]string, 0, len(ms))
	for _, m := range ms {
		rows = append(rows, []string{
			m.Symbol, m.Name, m.Market,
			fmt.Sprintf("%.2f", m.Price),
			fmt.Sprintf("%+.1f%%", m.ChangePct),
			fmtPtr(m.PE, "%.1f"),
			fmtPtr(m.PB, "%.2f"),
			fmtPtr(m.DividendYield, "%.2f"),
			fmtPtr(m.PEPercentile, "%.1f"),
			fmtPtr(m.PricePercentile, "%.1f"),
		})
	}
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = text.DisplayWidth(h)
	}
	for _, row := range rows {
		for i, c := range row {
			if cw := text.DisplayWidth(c); cw > widths[i] {
				widths[i] = cw
			}
		}
	}
	writeRow := func(cells []string) {
		var sb strings.Builder
		for i, c := range cells {
			if i == len(cells)-1 {
				sb.WriteString(c) // 末列不补尾空格(对齐 telegram renderTable 约定)
			} else {
				sb.WriteString(text.PadRight(c, widths[i]))
				sb.WriteString("  ")
			}
		}
		fmt.Fprintln(w, sb.String())
	}
	writeRow(headers)
	for _, row := range rows {
		writeRow(row)
	}
}

func fmtPtr(v *float64, format string) string {
	if v == nil {
		return naCell
	}
	return fmt.Sprintf(format, *v)
}

// renderGaps prints the per-symbol data-gap summary after the table.
func renderGaps(w io.Writer, ms []app.SymbolMetrics) {
	for _, m := range ms {
		if len(m.Gaps) > 0 {
			fmt.Fprintf(w, "! %s: %s\n", m.Symbol, strings.Join(m.Gaps, "; "))
		}
	}
}
