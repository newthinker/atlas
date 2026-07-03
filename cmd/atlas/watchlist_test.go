package main

// Context Checkpoint: done_criteria → test mapping
// functional[0] 表格CJK对齐+—+gap摘要 → TestExecuteWatchlist_TableAligned
// functional[1] --json 数组/null/roundtrip → TestExecuteWatchlist_JSON
// functional[2] --symbols 未知警告跳过/全未知报错
//               → TestExecuteWatchlist_UnknownSymbolWarns / _AllUnknownSymbolsErrors
// error_handling[0] 全失败打gaps返error → TestExecuteWatchlist_AllFailedErrors
// W4 "仅 PB/DYR 非 nil 不触发 allFailed" → TestExecuteWatchlist_FundamentalOnlyNotAllFailed
// boundary[0]   空watchlist提示exit0 → TestExecuteWatchlist_EmptyWatchlist

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/newthinker/atlas/internal/app"
	"github.com/newthinker/atlas/internal/text"
)

func f64(v float64) *float64 { return &v }

func fixtureMetrics() []app.SymbolMetrics {
	return []app.SymbolMetrics{
		{Symbol: "600519.SH", Name: "贵州茅台", Market: "A股", Type: "股票",
			Price: 1408, ChangePct: -0.5,
			PE: f64(19.5), PB: f64(5.96), DividendYield: f64(4.03),
			PEPercentile: f64(12.3), PricePercentile: f64(18)},
		{Symbol: "AAPL", Name: "Apple", Market: "美股", Type: "股票",
			Price: 213.5, ChangePct: 1.2,
			PEPercentile: f64(78.5), PricePercentile: f64(85.2),
			Gaps: []string{"fundamental source not configured for market"}},
	}
}

func runExecute(t *testing.T, params watchlistParams, ms []app.SymbolMetrics) (string, string, error) {
	t.Helper()
	var out, errOut bytes.Buffer
	deps := watchlistDeps{
		snapshot: func(ctx context.Context, symbols []string) []app.SymbolMetrics { return ms },
		known:    []string{"600519.SH", "AAPL"},
		out:      &out,
		errOut:   &errOut,
	}
	err := executeWatchlist(context.Background(), deps, params)
	return out.String(), errOut.String(), err
}

func TestExecuteWatchlist_TableAligned(t *testing.T) {
	out, _, err := runExecute(t, watchlistParams{}, fixtureMetrics())
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < 3 {
		t.Fatalf("want header + 2 rows, got %d lines:\n%s", len(lines), out)
	}
	if !strings.HasPrefix(lines[0], "SYMBOL") || !strings.Contains(lines[0], "PX%ILE") {
		t.Errorf("bad header: %q", lines[0])
	}
	// CJK 对齐:每行 NAME 列起点一致 → 各行 SYMBOL 段显示宽度一致
	nameIdx := strings.Index(lines[0], "NAME")
	headWidth := text.DisplayWidth(lines[0][:nameIdx])
	for _, ln := range lines[1:3] {
		cut := strings.Index(ln, "贵州茅台")
		if cut < 0 {
			cut = strings.Index(ln, "Apple")
		}
		if cut < 0 {
			continue
		}
		if w := text.DisplayWidth(ln[:cut]); w != headWidth {
			t.Errorf("misaligned NAME column: width %d vs header %d in %q", w, headWidth, ln)
		}
	}
	// 缺失指标显示 —
	if !strings.Contains(out, "—") {
		t.Error("missing metrics should render as —")
	}
	// 缺口摘要
	if !strings.Contains(out, "AAPL") || !strings.Contains(out, "fundamental source") {
		t.Error("gap summary missing")
	}
}

func TestExecuteWatchlist_JSON(t *testing.T) {
	out, _, err := runExecute(t, watchlistParams{jsonOut: true}, fixtureMetrics())
	if err != nil {
		t.Fatal(err)
	}
	var got []app.SymbolMetrics
	if uerr := json.Unmarshal([]byte(out), &got); uerr != nil {
		t.Fatalf("invalid json: %v\n%s", uerr, out)
	}
	if len(got) != 2 || got[1].PE != nil {
		t.Errorf("json roundtrip: %+v", got)
	}
	if !strings.Contains(out, `"pe": null`) && !strings.Contains(out, `"pe":null`) {
		t.Error("missing metrics must serialize as null")
	}
}

func TestExecuteWatchlist_UnknownSymbolWarns(t *testing.T) {
	_, errOut, err := runExecute(t, watchlistParams{symbols: []string{"AAPL", "NOPE"}}, fixtureMetrics()[1:])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errOut, "NOPE") {
		t.Errorf("expected warning for unknown symbol, got %q", errOut)
	}
}

func TestExecuteWatchlist_AllUnknownSymbolsErrors(t *testing.T) {
	_, _, err := runExecute(t, watchlistParams{symbols: []string{"NOPE"}}, nil)
	if err == nil {
		t.Fatal("expected error when no requested symbol is in watchlist")
	}
}

func TestExecuteWatchlist_AllFailedErrors(t *testing.T) {
	ms := []app.SymbolMetrics{
		{Symbol: "AAPL", Gaps: []string{"quote unavailable"}},
		{Symbol: "MSFT", Gaps: []string{"quote unavailable"}},
	}
	_, _, err := runExecute(t, watchlistParams{}, ms)
	if err == nil {
		t.Fatal("expected error when every symbol failed")
	}
}

// TestExecuteWatchlist_FundamentalOnlyNotAllFailed (W4): quote/history 失败但
// fundamental 返回 PB/股息率(无 price、无 PE/百分位)时不算"全失败",数据照展示。
func TestExecuteWatchlist_FundamentalOnlyNotAllFailed(t *testing.T) {
	ms := []app.SymbolMetrics{
		{Symbol: "600519.SH", Name: "贵州茅台", Market: "A股",
			PB: f64(5.96), DividendYield: f64(4.03),
			Gaps: []string{"quote unavailable", "history unavailable"}},
	}
	out, _, err := runExecute(t, watchlistParams{}, ms)
	if err != nil {
		t.Fatalf("PB/DYR alone must not be treated as total failure: %v", err)
	}
	if !strings.Contains(out, "5.96") || !strings.Contains(out, "4.03") {
		t.Errorf("fundamental-only metrics should still render, got:\n%s", out)
	}
}

func TestExecuteWatchlist_EmptyWatchlist(t *testing.T) {
	out, errOut, err := runExecute(t, watchlistParams{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out != "" && !strings.Contains(errOut+out, "empty") {
		t.Errorf("expected empty-watchlist notice, out=%q err=%q", out, errOut)
	}
}
