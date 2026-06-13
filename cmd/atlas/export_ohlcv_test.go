package main

// Context Checkpoint: done_criteria → test mapping (TASK-001)
// functional[0]   "toQlibInstrument 契约: 000300.SH→SH000300(+sh000300.csv 派生)/600519.SH/399001.SZ/930713.CSI; HK: 0700.HK→HK00700/2800.HK→HK02800/^HSI→HSI/^HSCE→HSCEI; AAPL/^GSPC/GC=F/BTC-USDT/0700.HK.X/^HSTECH 拒绝" → TestToQlibInstrument_Contract
// functional[1]   "golden CSV 逐字节: 8列 header + 三行互异 OHLCV + factor 恒1 + sh600519.csv"                    → TestExportOHLCV_GoldenCSV
// functional[2]   "resolveOHLCVSymbols: watchlist .SH/.SZ 过滤 + 基准去重 → {600519.SH,000300.SH}"             → TestResolveOHLCVSymbols_Default
// boundary[0]     "非 A 股符号在清单 → 不落盘 + errOut 摘要 + 返回 error, 已成功 CSV 保留"                       → TestExportOHLCV_NonAShareRejectedIntoSummary
// boundary[1]     "非基准 A 股拉取失败(errs 600000.SH) → 降级不中断 + 摘要含该符号 + 非0退出, 已成功 CSV 保留"   → TestExportOHLCV_NonBenchmarkFailureDegrades
// boundary[2]     "watchlist 空且无 --symbols → resolver 返回 error (绝不退化为只导基准)"                       → TestResolveOHLCVSymbols_EmptyWatchlistIsError
// error_handling[0] "基准 000300.SH 拉取失败/空 bars → 立即返回 error 且消息含 benchmark"                       → TestExportOHLCV_BenchmarkFailureIsFatal

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/config"
	"github.com/newthinker/atlas/internal/core"
)

// makeOHLCVBars builds n consecutive trading-day bars (weekends skipped) starting
// at start, with O/H/L/C/V deliberately distinct per bar so the golden CSV can
// catch column-misordering (the existing makeBars only fills Close/Time, so its
// all-zero O/H/L/V cannot). bar i: Open=100+i, High=101+i, Low=99+i,
// Close=100.5+i, Volume=1000+i. This generation rule is INTERLOCKED with the
// golden want string below — do not change one side alone.
func makeOHLCVBars(start string, n int) []core.OHLCV {
	day, err := time.Parse(dateLayout, start)
	if err != nil {
		panic("makeOHLCVBars start date: " + err.Error())
	}
	bars := make([]core.OHLCV, 0, n)
	for len(bars) < n {
		if wd := day.Weekday(); wd != time.Saturday && wd != time.Sunday {
			i := len(bars) // bar index; reused across columns so they stay in lockstep
			k := float64(i)
			bars = append(bars, core.OHLCV{
				Interval: "1d",
				Open:     100 + k,
				High:     101 + k,
				Low:      99 + k,
				Close:    100.5 + k,
				Volume:   1000 + int64(i),
				Time:     day,
			})
		}
		day = day.AddDate(0, 0, 1)
	}
	return bars
}

// fakeOHLCVProvider serves per-symbol bars and per-symbol errors, so failure
// semantics (one symbol fails, others succeed) can be expressed — which the
// existing staticOHLCVProvider (ignores symbol, never errors) cannot.
type fakeOHLCVProvider struct {
	data map[string][]core.OHLCV
	errs map[string]error
}

func (f fakeOHLCVProvider) FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	if err := f.errs[symbol]; err != nil {
		return nil, err
	}
	return f.data[symbol], nil
}

func TestToQlibInstrument_Contract(t *testing.T) {
	cases := []struct{ in, want string }{
		{"000300.SH", "SH000300"}, // 与 symbols.py tests 同样本
		{"600519.SH", "SH600519"},
		{"399001.SZ", "SZ399001"},
		{"930713.CSI", "CSI930713"}, // 中证跨市场指数（.CSI）
		{"0700.HK", "HK00700"},      // 港股股票：HK + 5 位补零
		{"2800.HK", "HK02800"},      // 港股 ETF
		{"^HSI", "HSI"},             // 恒生指数
		{"^HSCE", "HSCEI"},          // 国企指数（HSCEI）
	}
	for _, c := range cases {
		got, err := toQlibInstrument(c.in)
		if err != nil || got != c.want {
			t.Errorf("toQlibInstrument(%q) = (%q,%v), want %q", c.in, got, err, c.want)
		}
	}
	for _, bad := range []string{"AAPL", "^GSPC", "GC=F", "BTC-USDT", "0700.HK.X", "^HSTECH"} {
		if _, err := toQlibInstrument(bad); err == nil {
			t.Errorf("toQlibInstrument(%q) should reject non-A-share", bad)
		}
	}
	// spec 钉死：文件名层派生断言
	if ins, _ := toQlibInstrument("000300.SH"); strings.ToLower(ins)+".csv" != "sh000300.csv" {
		t.Errorf("filename derivation broken: %s", ins)
	}
}

func TestExportOHLCV_GoldenCSV(t *testing.T) {
	bars := makeOHLCVBars("2024-01-02", 3)
	dir := t.TempDir()
	deps := ohlcvDeps{
		provider: fakeOHLCVProvider{data: map[string][]core.OHLCV{"600519.SH": bars}},
		errOut:   io.Discard,
		sleep:    func() {},
	}
	err := executeExportOHLCV(deps, exportOHLCVParams{
		Symbols: []string{"600519.SH"}, From: "2024-01-01", To: "2024-01-10", OutDir: dir,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "sh600519.csv")) // 文件名 = instrument 小写
	want := `symbol,date,open,high,low,close,volume,factor
SH600519,2024-01-02,100.00,101.00,99.00,100.50,1000,1
SH600519,2024-01-03,101.00,102.00,100.00,101.50,1001,1
SH600519,2024-01-04,102.00,103.00,101.00,102.50,1002,1
`
	if string(got) != want {
		t.Errorf("golden mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestExportOHLCV_NonAShareRejectedIntoSummary(t *testing.T) {
	bars := makeOHLCVBars("2024-01-02", 3)
	dir := t.TempDir()
	var errOut bytes.Buffer
	deps := ohlcvDeps{
		provider: fakeOHLCVProvider{data: map[string][]core.OHLCV{"600519.SH": bars}},
		errOut:   &errOut,
		sleep:    func() {},
	}
	err := executeExportOHLCV(deps, exportOHLCVParams{
		Symbols: []string{"AAPL", "600519.SH"}, From: "2024-01-01", To: "2024-01-10", OutDir: dir,
	})
	if err == nil {
		t.Fatal("expected non-zero exit (error) when a non-A-share symbol is in the list")
	}
	if !strings.Contains(errOut.String(), "AAPL") {
		t.Errorf("errOut summary should mention AAPL, got: %q", errOut.String())
	}
	// 已成功 CSV 保留
	if _, statErr := os.Stat(filepath.Join(dir, "sh600519.csv")); statErr != nil {
		t.Errorf("successful CSV sh600519.csv should be kept: %v", statErr)
	}
	// 非 A 股不落盘（不应有 aapl.csv）
	if _, statErr := os.Stat(filepath.Join(dir, "aapl.csv")); statErr == nil {
		t.Errorf("non-A-share AAPL must not be written")
	}
}

func TestExportOHLCV_NonBenchmarkFailureDegrades(t *testing.T) {
	bars := makeOHLCVBars("2024-01-02", 3)
	dir := t.TempDir()
	var errOut bytes.Buffer
	deps := ohlcvDeps{
		provider: fakeOHLCVProvider{
			data: map[string][]core.OHLCV{"600519.SH": bars},
			errs: map[string]error{"600000.SH": errors.New("boom")},
		},
		errOut: &errOut,
		sleep:  func() {},
	}
	err := executeExportOHLCV(deps, exportOHLCVParams{
		Symbols: []string{"600000.SH", "600519.SH"}, From: "2024-01-01", To: "2024-01-10", OutDir: dir,
	})
	if err == nil {
		t.Fatal("expected non-zero exit when a non-benchmark symbol fails")
	}
	if !strings.Contains(errOut.String(), "600000.SH") {
		t.Errorf("errOut summary should mention failed symbol 600000.SH, got: %q", errOut.String())
	}
	// 其余符号继续导出，已成功 CSV 保留
	if _, statErr := os.Stat(filepath.Join(dir, "sh600519.csv")); statErr != nil {
		t.Errorf("other symbols must keep exporting after a non-benchmark failure: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "sh600000.csv")); statErr == nil {
		t.Errorf("failed symbol 600000.SH must not produce a CSV")
	}
}

func TestExportOHLCV_BenchmarkFailureIsFatal(t *testing.T) {
	dir := t.TempDir()
	deps := ohlcvDeps{
		provider: fakeOHLCVProvider{
			errs: map[string]error{"000300.SH": errors.New("boom")},
		},
		errOut: io.Discard,
		sleep:  func() {},
	}
	err := executeExportOHLCV(deps, exportOHLCVParams{
		Symbols: []string{"000300.SH"}, From: "2024-01-01", To: "2024-01-10", OutDir: dir,
	})
	if err == nil {
		t.Fatal("benchmark fetch failure must be fatal")
	}
	if !strings.Contains(err.Error(), "benchmark") {
		t.Errorf("fatal error message must mention benchmark, got: %v", err)
	}
}

func TestResolveOHLCVSymbols_Default(t *testing.T) {
	watchlist := []config.WatchlistItem{
		{Symbol: "600519.SH"},
		{Symbol: "BTC-USDT"},
		{Symbol: "^GSPC"},
		{Symbol: "000300.SH"},
	}
	got, err := resolveOHLCVSymbols(nil, watchlist, "cn")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"600519.SH", "000300.SH"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("resolveOHLCVSymbols = %v, want %v", got, want)
	}
}

func TestResolveOHLCVSymbols_EmptyWatchlistIsError(t *testing.T) {
	// C1-1 防线：watchlist 为空且未显式 --symbols → 报错（绝不退化为只导基准）
	if _, err := resolveOHLCVSymbols(nil, nil, "cn"); err == nil {
		t.Fatal("empty watchlist with no --symbols must be an error, not degrade to benchmark-only")
	}
	// 仅含非 A 股的 watchlist 同样视为空集（过滤后无 A 股）
	if _, err := resolveOHLCVSymbols(nil, []config.WatchlistItem{{Symbol: "AAPL"}, {Symbol: "BTC-USDT"}}, "cn"); err == nil {
		t.Fatal("watchlist without any A-share must be an error")
	}
}

// --- TASK-002 CLI wiring ---
// functional[0] "export-ohlcv usage 列出 --symbols/--from/--to/--out-dir" → TestExportOHLCVCommand_UsageListsAllFlags
// functional[1] "CLI 层基准校验: --symbols 不含 000300.SH → 报错含 benchmark" → TestRunExportOHLCV_BenchmarkMissingIsFatal

func TestExportOHLCVCommand_UsageListsAllFlags(t *testing.T) {
	usage := exportOHLCVCmd.UsageString()
	for _, flag := range []string{"--symbols", "--from", "--to", "--out-dir", "--market"} {
		if !strings.Contains(usage, flag) {
			t.Errorf("export-ohlcv usage missing flag %s:\n%s", flag, usage)
		}
	}
}

func TestRunExportOHLCV_BenchmarkMissingIsFatal(t *testing.T) {
	// CLI 层校验（承接 TASK-001 分层语义）：显式 --symbols 不含基准 000300.SH →
	// 立即报错含 benchmark，且早于任何网络/解析（resolver 透传 flag → requireBenchmark 拦截）。
	saved := exportOHLCVSymbols
	t.Cleanup(func() { exportOHLCVSymbols = saved })

	exportOHLCVSymbols = []string{"600519.SH"} // 缺基准
	cfgFile = ""                               // → config.Defaults()，watchlist 空但 flag 非空走透传

	err := runExportOHLCV(exportOHLCVCmd, nil)
	if err == nil {
		t.Fatal("symbol set without benchmark must be fatal at the CLI layer")
	}
	if !strings.Contains(err.Error(), "benchmark") {
		t.Errorf("error must mention benchmark, got: %v", err)
	}
}

// --- TASK-003 market 参数化 ---
// functional[0] "benchmarkForMarket: cn→000300.SH、hk→^HSI"                                  → TestBenchmarkForMarket
// functional[1] "resolveOHLCVSymbols(nil,wl,\"hk\") 选 .HK+^HSI/^HSCE、排除 A 股、含基准、去重" → TestResolveOHLCVSymbols_HKMarket
// error_handling[1] "hk 基准 ^HSI 取数失败 → executeExportOHLCV 返回 error 且消息含 benchmark"  → TestExportOHLCV_HKBenchmarkFailureIsFatal

func TestResolveOHLCVSymbols_HKMarket(t *testing.T) {
	wl := []config.WatchlistItem{
		{Symbol: "0700.HK"}, {Symbol: "2800.HK"}, {Symbol: "^HSI"},
		{Symbol: "^HSCE"}, {Symbol: "600519.SH"}, // A股应被 hk market 排除
	}
	got, err := resolveOHLCVSymbols(nil, wl, "hk")
	if err != nil {
		t.Fatalf("hk resolve: %v", err)
	}
	want := map[string]bool{"0700.HK": true, "2800.HK": true, "^HSI": true, "^HSCE": true}
	for _, s := range got {
		if s == "600519.SH" {
			t.Error("A-share must be excluded from hk market set")
		}
		delete(want, s)
	}
	if len(want) != 0 {
		t.Errorf("hk set missing symbols: %v (got %v)", want, got)
	}
	// 基准 ^HSI 必须在内
	if !slices.Contains(got, "^HSI") {
		t.Errorf("hk set must include benchmark ^HSI, got %v", got)
	}
}

func TestBenchmarkForMarket(t *testing.T) {
	if benchmarkForMarket("cn") != "000300.SH" {
		t.Errorf("cn benchmark = %q, want 000300.SH", benchmarkForMarket("cn"))
	}
	if benchmarkForMarket("hk") != "^HSI" {
		t.Errorf("hk benchmark = %q, want ^HSI", benchmarkForMarket("hk"))
	}
}

// TestExportOHLCV_HKBenchmarkFailureIsFatal is the reviewer-mandated direct
// assertion (DoD error_handling[1]): with the HK benchmark ^HSI injected via
// p.Benchmark, a failed ^HSI fetch must make executeExportOHLCV fatal. This
// proves the market benchmark reaches the core's fatal branch — the core used
// to hardcode benchmarkSymbol (000300.SH), which would have silently degraded
// ^HSI to a non-fatal per-symbol failure.
func TestExportOHLCV_HKBenchmarkFailureIsFatal(t *testing.T) {
	dir := t.TempDir()
	deps := ohlcvDeps{
		provider: fakeOHLCVProvider{
			errs: map[string]error{"^HSI": errors.New("boom")},
		},
		errOut: io.Discard,
		sleep:  func() {},
	}
	err := executeExportOHLCV(deps, exportOHLCVParams{
		Symbols:   []string{"^HSI"},
		From:      "2024-01-01",
		To:        "2024-01-10",
		OutDir:    dir,
		Benchmark: "^HSI",
	})
	if err == nil {
		t.Fatal("hk benchmark ^HSI fetch failure must be fatal")
	}
	if !strings.Contains(err.Error(), "benchmark") {
		t.Errorf("fatal error message must mention benchmark, got: %v", err)
	}
}

// TestRequireBenchmark_Market covers both the present (nil) and missing (error
// mentioning the market benchmark) branches for cn and hk — requireBenchmark now
// resolves the benchmark per market via benchmarkForMarket.
func TestRequireBenchmark_Market(t *testing.T) {
	cases := []struct {
		name    string
		symbols []string
		market  string
		wantErr bool
		wantStr string
	}{
		{"cn present", []string{"600519.SH", "000300.SH"}, "cn", false, ""},
		{"cn missing", []string{"600519.SH"}, "cn", true, "000300.SH"},
		{"hk present", []string{"0700.HK", "^HSI"}, "hk", false, ""},
		{"hk missing", []string{"0700.HK"}, "hk", true, "^HSI"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := requireBenchmark(c.symbols, c.market)
			if (err != nil) != c.wantErr {
				t.Fatalf("requireBenchmark(%v,%q) err=%v, wantErr=%v", c.symbols, c.market, err, c.wantErr)
			}
			if c.wantErr && !strings.Contains(err.Error(), c.wantStr) {
				t.Errorf("error must mention benchmark %q, got: %v", c.wantStr, err)
			}
		})
	}
}

// QA fix F3: --market 只接受 {cn,hk}；非法取值不得静默按 cn 跑，必须在
// runExportOHLCV 入口报错（消息含 unknown market），且早于任何 resolver/网络。
func TestRunExportOHLCV_RejectsUnknownMarket(t *testing.T) {
	savedSymbols := exportOHLCVSymbols
	savedMarket := exportOHLCVMarket
	savedCfg := cfgFile
	t.Cleanup(func() {
		exportOHLCVSymbols = savedSymbols
		exportOHLCVMarket = savedMarket
		cfgFile = savedCfg
	})

	exportOHLCVSymbols = []string{"0700.HK", "^HSI"} // 合法集合，证明拦截不是别的分支
	cfgFile = ""

	for _, m := range []string{"us", "HK", "cn ", "", "crypto"} {
		exportOHLCVMarket = m
		err := runExportOHLCV(exportOHLCVCmd, nil)
		if err == nil {
			t.Fatalf("--market %q must be rejected, got nil error", m)
		}
		if !strings.Contains(err.Error(), "unknown market") {
			t.Errorf("--market %q error must mention unknown market, got: %v", m, err)
		}
	}
}
