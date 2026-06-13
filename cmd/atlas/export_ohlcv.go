package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/newthinker/atlas/internal/backtest"
	"github.com/newthinker/atlas/internal/collector"
	"github.com/newthinker/atlas/internal/collector/crypto"
	"github.com/newthinker/atlas/internal/collector/eastmoney"
	"github.com/newthinker/atlas/internal/collector/lixinger"
	"github.com/newthinker/atlas/internal/collector/yahoo"
	"github.com/newthinker/atlas/internal/config"
	"github.com/newthinker/atlas/internal/core"
	"github.com/spf13/cobra"
)

// benchmarkSymbol is the CSI 300 index (SH000300 in qlib form). Strategy
// evaluation is meaningless without it, so a failed benchmark fetch is fatal.
const benchmarkSymbol = "000300.SH"

// ohlcvCSVHeader is the eight-column qlib dump_bin contract: factor is always 1
// because prices are already 前复权 (fqt=1) at the source and the evaluator
// never multiplies $factor (spec §已钉死口径).
var ohlcvCSVHeader = []string{"symbol", "date", "open", "high", "low", "close", "volume", "factor"}

// toQlibInstrument mirrors scripts/qlib_eval/qlib_eval/symbols.py
// to_qlib_instrument — keep the two in sync (the contract test shares samples).
// 600519.SH -> SH600519, 399001.SZ -> SZ399001; every non-A-share symbol is
// rejected (Phase 1 is A-share only, design §1.1).
func toQlibInstrument(symbol string) (string, error) {
	switch {
	case strings.HasSuffix(symbol, ".SH"):
		return "SH" + strings.TrimSuffix(symbol, ".SH"), nil
	case strings.HasSuffix(symbol, ".SZ"):
		return "SZ" + strings.TrimSuffix(symbol, ".SZ"), nil
	}
	return "", fmt.Errorf("not an A-share symbol: %s", symbol)
}

type exportOHLCVParams struct {
	Symbols  []string
	From, To string
	OutDir   string
}

// ohlcvDeps holds the injectable dependencies so the core can run offline and
// deterministically: tests inject a per-symbol provider and a no-op sleep.
type ohlcvDeps struct {
	provider backtest.OHLCVProvider // 测试注入；CLI 路径逐 symbol SelectForSymbol
	errOut   io.Writer              // per-symbol 降级摘要
	sleep    func()                 // 300ms 礼貌延迟，测试注入空函数
}

// executeExportOHLCV writes one qlib-convention CSV per A-share symbol into
// p.OutDir (filename = lowercase instrument, e.g. sh600519.csv). Per-symbol
// failures (non-A-share, fetch error, empty bars) degrade with an errOut summary
// and a non-zero exit, but already-written CSVs are kept and the remaining
// symbols keep exporting. A failed or empty benchmark is fatal.
func executeExportOHLCV(deps ohlcvDeps, p exportOHLCVParams) error {
	from, err := parseBacktestDate("from", p.From)
	if err != nil {
		return err
	}
	to := time.Now()
	if p.To != "" {
		if to, err = parseBacktestDate("to", p.To); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(p.OutDir, 0o755); err != nil {
		return fmt.Errorf("creating output dir %s: %w", p.OutDir, err)
	}

	var failures []string
	for _, symbol := range p.Symbols {
		instrument, err := toQlibInstrument(symbol)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", symbol, err))
			continue
		}
		bars, err := deps.provider.FetchHistory(symbol, from, to, "1d")
		if err == nil && len(bars) == 0 {
			err = fmt.Errorf("no data")
		}
		if err != nil {
			if symbol == benchmarkSymbol {
				return fmt.Errorf("benchmark %s: %w", symbol, err)
			}
			failures = append(failures, fmt.Sprintf("%s: %v", symbol, err))
			continue
		}
		if err := writeOHLCVCSV(p.OutDir, instrument, bars); err != nil {
			return err
		}
		deps.sleep()
	}

	if len(failures) > 0 {
		fmt.Fprintf(deps.errOut, "warning: %d symbol(s) failed to export:\n", len(failures))
		for _, f := range failures {
			fmt.Fprintf(deps.errOut, "  %s\n", f)
		}
		return fmt.Errorf("%d symbol(s) failed to export", len(failures))
	}
	return nil
}

// writeOHLCVCSV renders bars as the eight-column qlib CSV at
// {dir}/{lowercase instrument}.csv. Prices use %.2f, volume is an integer and
// factor is the literal "1".
func writeOHLCVCSV(dir, instrument string, bars []core.OHLCV) error {
	path := filepath.Join(dir, strings.ToLower(instrument)+".csv")
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating %s: %w", path, err)
	}
	defer f.Close()

	cw := csv.NewWriter(f)
	if err := cw.Write(ohlcvCSVHeader); err != nil {
		return err
	}
	for _, b := range bars {
		row := []string{
			instrument,
			b.Time.Format(dateLayout),
			fmt.Sprintf("%.2f", b.Open),
			fmt.Sprintf("%.2f", b.High),
			fmt.Sprintf("%.2f", b.Low),
			fmt.Sprintf("%.2f", b.Close),
			fmt.Sprintf("%d", b.Volume),
			"1",
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// resolveOHLCVSymbols picks the symbol set to export. An explicit --symbols flag
// wins as-is (the CLI layer validates benchmark presence). Otherwise the set is
// derived from the watchlist's A-share (.SH/.SZ) symbols plus the benchmark,
// order-preserved and de-duplicated. An empty derived set (no flag and no
// A-share in the watchlist) is an error — it must never degrade to exporting the
// benchmark alone (plan C1-1).
func resolveOHLCVSymbols(flag []string, watchlist []config.WatchlistItem) ([]string, error) {
	if len(flag) > 0 {
		return flag, nil
	}
	var shares []string
	for _, item := range watchlist {
		if strings.HasSuffix(item.Symbol, ".SH") || strings.HasSuffix(item.Symbol, ".SZ") {
			shares = append(shares, item.Symbol)
		}
	}
	if len(shares) == 0 {
		return nil, fmt.Errorf("no A-share symbols in watchlist and no --symbols provided")
	}

	result := make([]string, 0, len(shares)+1)
	seen := make(map[string]bool)
	for _, s := range append(shares, benchmarkSymbol) {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result, nil
}

// --- CLI wiring ---

var (
	exportOHLCVSymbols []string
	exportOHLCVFrom    string
	exportOHLCVTo      string
	exportOHLCVOutDir  string
)

var exportOHLCVCmd = &cobra.Command{
	Use:   "export-ohlcv",
	Short: "Export per-instrument OHLCV CSVs in qlib dump_bin convention",
	Long: "Export one qlib-convention OHLCV CSV per A-share instrument for the " +
		"offline qlib data bundle. With no --symbols the set defaults to the " +
		"config watchlist's A-shares plus the benchmark; the benchmark must always " +
		"be present for the evaluation to be meaningful.",
	RunE: runExportOHLCV,
}

func init() {
	exportOHLCVCmd.Flags().StringSliceVar(&exportOHLCVSymbols, "symbols", nil,
		"Comma-separated symbols (default: watchlist A-shares + benchmark)")
	exportOHLCVCmd.Flags().StringVar(&exportOHLCVFrom, "from", "", "Start date YYYY-MM-DD (required)")
	exportOHLCVCmd.Flags().StringVar(&exportOHLCVTo, "to", "", "End date YYYY-MM-DD (default: today)")
	exportOHLCVCmd.Flags().StringVar(&exportOHLCVOutDir, "out-dir", "qlib_csv", "Output directory for per-instrument CSVs")

	exportOHLCVCmd.MarkFlagRequired("from")

	rootCmd.AddCommand(exportOHLCVCmd)
}

// requireBenchmark enforces, at the CLI layer, that the resolved symbol set
// includes the benchmark — without it strategy evaluation is meaningless. This
// continues the layering decision from the export-ohlcv core (TASK-001): the
// core stays pure, the "list must contain the benchmark" check lives here.
func requireBenchmark(symbols []string) error {
	if slices.Contains(symbols, benchmarkSymbol) {
		return nil
	}
	return fmt.Errorf("symbol set must include benchmark %s for evaluation to be meaningful", benchmarkSymbol)
}

// loadConfigOrDefaults mirrors serve.go's cfgFile + config.Load pattern: an
// explicit --config is loaded, otherwise the built-in defaults are used.
// (export-signals carries no config loading, so it cannot serve as a reference —
// plan C1-5.)
func loadConfigOrDefaults() (*config.Config, error) {
	if cfgFile != "" {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return nil, fmt.Errorf("loading config: %w", err)
		}
		return cfg, nil
	}
	return config.Defaults(), nil
}

// newCollectorRegistry builds the CLI collector registry: yahoo, eastmoney and
// crypto. When the config carries an enabled Lixinger key, Lixinger is wired as
// eastmoney's A-share fallback (mirrors serve.go) so that history degrades to
// the Lixinger candlestick API when the eastmoney kline endpoint is unreachable
// (index → cn/index/candlestick, stock → company). Without --config the API key
// is absent and the fallback stays off.
func newCollectorRegistry(cfg *config.Config) *collector.Registry {
	reg := collector.NewRegistry()
	reg.Register(yahoo.New())

	em := eastmoney.New()
	if lc, ok := cfg.Collectors["lixinger"]; ok && lc.Enabled && lc.APIKey != "" {
		retry := true
		if v, ok := lc.Extra["retry"].(bool); ok {
			retry = v
		}
		em.SetLixingerFallback(lixinger.New(lc.APIKey, lixinger.WithRetry(retry)))
	}
	reg.Register(em)
	reg.Register(crypto.New())
	return reg
}

func runExportOHLCV(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfigOrDefaults()
	if err != nil {
		return err
	}
	symbols, err := resolveOHLCVSymbols(exportOHLCVSymbols, cfg.Watchlist)
	if err != nil {
		return err
	}
	if err := requireBenchmark(symbols); err != nil {
		return err
	}

	reg := newCollectorRegistry(cfg)

	deps := ohlcvDeps{
		provider: registryProvider{reg: reg},
		errOut:   os.Stderr,
		sleep:    func() { time.Sleep(300 * time.Millisecond) },
	}
	return executeExportOHLCV(deps, exportOHLCVParams{
		Symbols: symbols,
		From:    exportOHLCVFrom,
		To:      exportOHLCVTo,
		OutDir:  exportOHLCVOutDir,
	})
}
