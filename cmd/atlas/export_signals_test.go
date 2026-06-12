package main

// Context Checkpoint: done_criteria → test mapping (TASK-003)
// functional[0]   "golden CSV 逐字节：7列/%.2f/metadata JSON 转义/from 过滤 warm-up" → TestExportSignals_GoldenCSV
// functional[1]   "Fundamentals 策略动态拒绝(requires fundamentals + 可用清单)"      → TestExportSignals_RejectsFundamentalStrategies
// functional[2]   "未知策略名报错 + 可用清单"                                         → TestExportSignals_UnknownStrategy
// functional[3]   "engine 注册全部 5 策略(默认构造器)"                                → TestNewExportEngine_RegistersAllFive
// functional[4]   "真实 CLI engine 路径 pe_band → requires fundamentals(非 unknown)" → TestExportSignals_PEBandViaCLIEngineRejected
// boundary[0a]    "SkippedBars>0 → errOut 一行摘要"                                   → TestExportSignals_SkippedBarsSummary
// boundary[0b]    "metadata 为 nil → 该列空串"                                        → TestExportSignals_NilMetadataEmptyColumn
// error_handling[0] "from/to 解析失败、from>to 明确错误"                              → TestExportSignals_DateErrors
// non_functional[0] "export-signals --help 输出全部 flags"                           → TestExportCommand_UsageListsAllFlags

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/strategy"
)

// makeBars builds n consecutive trading-day bars (weekends skipped) starting at
// startDate, with Close incrementing from startClose. Used to craft golden data.
func makeBars(t *testing.T, startDate string, n int, startClose float64) []core.OHLCV {
	t.Helper()
	day, err := time.Parse(dateLayout, startDate)
	if err != nil {
		t.Fatalf("makeBars start date: %v", err)
	}
	bars := make([]core.OHLCV, 0, n)
	close := startClose
	for len(bars) < n {
		if wd := day.Weekday(); wd != time.Saturday && wd != time.Sunday {
			bars = append(bars, core.OHLCV{Symbol: "600519.SH", Interval: "1d", Close: close, Time: day})
			close++
		}
		day = day.AddDate(0, 0, 1)
	}
	return bars
}

// engineWithStrategies registers the given strategy instances into a fresh Engine.
func engineWithStrategies(strats ...strategy.Strategy) *strategy.Engine {
	e := strategy.NewEngine()
	for _, s := range strats {
		e.Register(s)
	}
	return e
}

// flatStub emits a BUY on every bar with a deliberately wrong GeneratedAt (the
// engine must overwrite it) and a fixed metadata map.
type flatStub struct{}

func (flatStub) Name() string        { return "flat_stub" }
func (flatStub) Description() string { return "" }
func (flatStub) RequiredData() strategy.DataRequirements {
	return strategy.DataRequirements{PriceHistory: 1}
}
func (flatStub) Init(strategy.Config) error { return nil }
func (flatStub) Analyze(ctx strategy.AnalysisContext) ([]core.Signal, error) {
	return []core.Signal{{
		Symbol: ctx.Symbol, Action: core.ActionBuy, Confidence: 0.7,
		Metadata:    map[string]any{"k": 1},
		GeneratedAt: time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC),
	}}, nil
}

// fundamentalsStub requires fundamental data and must be rejected by the whitelist.
type fundamentalsStub struct{}

func (fundamentalsStub) Name() string        { return "funda_stub" }
func (fundamentalsStub) Description() string { return "" }
func (fundamentalsStub) RequiredData() strategy.DataRequirements {
	return strategy.DataRequirements{Fundamentals: true}
}
func (fundamentalsStub) Init(strategy.Config) error { return nil }
func (fundamentalsStub) Analyze(strategy.AnalysisContext) ([]core.Signal, error) {
	return nil, nil
}

// nilMetaStub emits a BUY on every bar with nil Metadata.
type nilMetaStub struct{}

func (nilMetaStub) Name() string        { return "nilmeta_stub" }
func (nilMetaStub) Description() string { return "" }
func (nilMetaStub) RequiredData() strategy.DataRequirements {
	return strategy.DataRequirements{PriceHistory: 1}
}
func (nilMetaStub) Init(strategy.Config) error { return nil }
func (nilMetaStub) Analyze(ctx strategy.AnalysisContext) ([]core.Signal, error) {
	return []core.Signal{{Symbol: ctx.Symbol, Action: core.ActionBuy, Confidence: 0.5}}, nil
}

// skipStub fails analysis on the bar whose Close == failClose, exercising the
// SkippedBars summary path; otherwise emits a BUY.
type skipStub struct{ failClose float64 }

func (skipStub) Name() string        { return "skip_stub" }
func (skipStub) Description() string { return "" }
func (skipStub) RequiredData() strategy.DataRequirements {
	return strategy.DataRequirements{PriceHistory: 1}
}
func (skipStub) Init(strategy.Config) error { return nil }
func (s skipStub) Analyze(ctx strategy.AnalysisContext) ([]core.Signal, error) {
	if len(ctx.OHLCV) > 0 && ctx.OHLCV[len(ctx.OHLCV)-1].Close == s.failClose {
		return nil, io.ErrUnexpectedEOF
	}
	return []core.Signal{{Symbol: ctx.Symbol, Action: core.ActionBuy, Confidence: 0.5}}, nil
}

func TestExportSignals_GoldenCSV(t *testing.T) {
	bars := makeBars(t, "2024-01-02", 5, 100) // 5 trading days, close 100..104
	deps := exportDeps{
		provider:   staticOHLCVProvider{data: bars},
		strategies: engineWithStrategies(flatStub{}),
		out:        &bytes.Buffer{},
		errOut:     &bytes.Buffer{},
	}
	var buf bytes.Buffer
	err := executeExport(deps, &buf, exportParams{
		Strategies: []string{"flat_stub"}, Symbols: []string{"600519.SH"},
		From: "2024-01-03", To: "2024-01-08", // from later than first bar → warm-up filtered
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `symbol,date,strategy,action,confidence,price,metadata
600519.SH,2024-01-03,flat_stub,buy,0.70,101.00,"{""k"":1}"
600519.SH,2024-01-04,flat_stub,buy,0.70,102.00,"{""k"":1}"
600519.SH,2024-01-05,flat_stub,buy,0.70,103.00,"{""k"":1}"
600519.SH,2024-01-08,flat_stub,buy,0.70,104.00,"{""k"":1}"
`
	if got := buf.String(); got != want {
		t.Errorf("golden mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestExportSignals_RejectsFundamentalStrategies(t *testing.T) {
	deps := exportDeps{
		provider:   staticOHLCVProvider{data: makeBars(t, "2024-01-02", 3, 100)},
		strategies: engineWithStrategies(fundamentalsStub{}, flatStub{}),
		out:        &bytes.Buffer{}, errOut: &bytes.Buffer{},
	}
	err := executeExport(deps, io.Discard, exportParams{
		Strategies: []string{"funda_stub"}, Symbols: []string{"600519.SH"},
		From: "2024-01-02", To: "2024-01-05",
	})
	if err == nil || !strings.Contains(err.Error(), "requires fundamentals") {
		t.Errorf("want explicit fundamentals rejection, got %v", err)
	}
	// available list must be dynamic (mention the offline strategy), not hard-coded.
	if err != nil && !strings.Contains(err.Error(), "flat_stub") {
		t.Errorf("rejection should list available offline strategies, got %v", err)
	}
}

func TestExportSignals_UnknownStrategy(t *testing.T) {
	deps := exportDeps{
		provider:   staticOHLCVProvider{data: makeBars(t, "2024-01-02", 3, 100)},
		strategies: engineWithStrategies(flatStub{}),
		out:        &bytes.Buffer{}, errOut: &bytes.Buffer{},
	}
	err := executeExport(deps, io.Discard, exportParams{
		Strategies: []string{"does_not_exist"}, Symbols: []string{"600519.SH"},
		From: "2024-01-02", To: "2024-01-05",
	})
	if err == nil || !strings.Contains(err.Error(), "unknown strategy") {
		t.Errorf("want unknown strategy error, got %v", err)
	}
	if err != nil && !strings.Contains(err.Error(), "flat_stub") {
		t.Errorf("unknown strategy error should list available strategies, got %v", err)
	}
}

func TestNewExportEngine_RegistersAllFive(t *testing.T) {
	eng := newExportEngine()
	want := []string{"ma_crossover", "price_percentile", "pe_band", "dividend_yield", "pe_percentile"}
	for _, name := range want {
		if _, ok := eng.Get(name); !ok {
			t.Errorf("newExportEngine missing strategy %q (registered: %v)", name, eng.GetStrategyNames())
		}
	}
}

func TestExportSignals_PEBandViaCLIEngineRejected(t *testing.T) {
	// Drive the REAL CLI engine: pe_band must reach the fundamentals rejection,
	// NOT the unknown-strategy branch (regression guard for plan T4 warning).
	deps := exportDeps{
		provider:   staticOHLCVProvider{data: makeBars(t, "2024-01-02", 3, 100)},
		strategies: newExportEngine(),
		out:        &bytes.Buffer{}, errOut: &bytes.Buffer{},
	}
	err := executeExport(deps, io.Discard, exportParams{
		Strategies: []string{"pe_band"}, Symbols: []string{"600519.SH"},
		From: "2024-01-02", To: "2024-01-05",
	})
	if err == nil || !strings.Contains(err.Error(), "requires fundamentals") {
		t.Fatalf("pe_band via CLI engine must hit fundamentals rejection, got %v", err)
	}
	if strings.Contains(err.Error(), "unknown strategy") {
		t.Errorf("pe_band fell into unknown-strategy branch — not all strategies registered: %v", err)
	}
}

func TestExportSignals_SkippedBarsSummary(t *testing.T) {
	bars := makeBars(t, "2024-01-02", 5, 100) // close 100..104
	errBuf := &bytes.Buffer{}
	deps := exportDeps{
		provider:   staticOHLCVProvider{data: bars},
		strategies: engineWithStrategies(skipStub{failClose: 102}), // one bar skipped
		out:        &bytes.Buffer{}, errOut: errBuf,
	}
	err := executeExport(deps, &bytes.Buffer{}, exportParams{
		Strategies: []string{"skip_stub"}, Symbols: []string{"600519.SH"},
		From: "2024-01-02", To: "2024-01-08",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ToLower(errBuf.String()), "skip") {
		t.Errorf("want skipped-bars summary on errOut, got %q", errBuf.String())
	}
}

func TestExportSignals_NilMetadataEmptyColumn(t *testing.T) {
	bars := makeBars(t, "2024-01-02", 2, 100)
	var buf bytes.Buffer
	deps := exportDeps{
		provider:   staticOHLCVProvider{data: bars},
		strategies: engineWithStrategies(nilMetaStub{}),
		out:        &bytes.Buffer{}, errOut: &bytes.Buffer{},
	}
	err := executeExport(deps, &buf, exportParams{
		Strategies: []string{"nilmeta_stub"}, Symbols: []string{"600519.SH"},
		From: "2024-01-02", To: "2024-01-05",
	})
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected header + data rows, got %q", buf.String())
	}
	// last column (metadata) must be empty → row ends with a trailing comma.
	for _, row := range lines[1:] {
		if !strings.HasSuffix(row, ",") {
			t.Errorf("nil metadata should yield empty trailing column, got row %q", row)
		}
	}
}

func TestExportSignals_DateErrors(t *testing.T) {
	deps := exportDeps{
		provider:   staticOHLCVProvider{data: makeBars(t, "2024-01-02", 3, 100)},
		strategies: engineWithStrategies(flatStub{}),
		out:        &bytes.Buffer{}, errOut: &bytes.Buffer{},
	}
	cases := []struct {
		name, from, to string
	}{
		{"bad from", "nope", "2024-01-05"},
		{"bad to", "2024-01-02", "nope"},
		{"from after to", "2024-01-10", "2024-01-02"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := executeExport(deps, io.Discard, exportParams{
				Strategies: []string{"flat_stub"}, Symbols: []string{"600519.SH"},
				From: c.from, To: c.to,
			})
			if err == nil {
				t.Errorf("%s: want error, got nil", c.name)
			}
		})
	}
}

func TestExportCommand_UsageListsAllFlags(t *testing.T) {
	usage := exportCmd.UsageString()
	for _, flag := range []string{"--strategies", "--symbols", "--from", "--to", "--out"} {
		if !strings.Contains(usage, flag) {
			t.Errorf("export-signals usage missing flag %s:\n%s", flag, usage)
		}
	}
}
