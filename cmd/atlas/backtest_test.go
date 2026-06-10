package main

// Context Checkpoint: done_criteria → test mapping (TASK-008)
// functional[0]   "实跑回测引擎并输出信号数/交易数/Stats"        → TestExecuteBacktest_RunsEngineAndOutputs
// functional[1]   "策略不存在列出可用策略+非0退出"               → TestExecuteBacktest_UnknownStrategy
// boundary[0]     "--from 晚于 --to 参数错误(非0退出)"           → TestExecuteBacktest_FromAfterTo
// boundary[1]     "空 OHLCV 友好提示不 panic"                    → TestExecuteBacktest_EmptyData
// error_handling[0] "拉取历史失败错误+非0退出"                    → TestExecuteBacktest_FetchError
// non_functional[0] "离线确定性(注入 provider, 不打真实 API)"     → 全部用 stubProvider

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/strategy"
)

// stubProvider is an in-memory OHLCVProvider for offline, deterministic tests.
type stubProvider struct {
	data  []core.OHLCV
	err   error
	calls int
}

func (s *stubProvider) FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	s.calls++
	return s.data, s.err
}

// mockBtStrategy emits BUY on cheap bars and SELL on expensive bars so a
// buy→sell cycle yields exactly one trade over the crafted data.
type mockBtStrategy struct{ name string }

func (m *mockBtStrategy) Name() string                  { return m.name }
func (m *mockBtStrategy) Description() string            { return "mock backtest strategy" }
func (m *mockBtStrategy) Init(cfg strategy.Config) error { return nil }
func (m *mockBtStrategy) RequiredData() strategy.DataRequirements {
	return strategy.DataRequirements{PriceHistory: 1}
}
func (m *mockBtStrategy) Analyze(ctx strategy.AnalysisContext) ([]core.Signal, error) {
	if len(ctx.OHLCV) == 0 {
		return nil, nil
	}
	last := ctx.OHLCV[len(ctx.OHLCV)-1]
	switch {
	case last.Close <= 102:
		return []core.Signal{{Symbol: ctx.Symbol, Action: core.ActionBuy}}, nil
	case last.Close >= 106:
		return []core.Signal{{Symbol: ctx.Symbol, Action: core.ActionSell}}, nil
	}
	return nil, nil
}

func sampleOHLCV() []core.OHLCV {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	closes := []float64{102, 106, 108, 105, 104}
	out := make([]core.OHLCV, len(closes))
	for i, c := range closes {
		out[i] = core.OHLCV{Symbol: "AAPL", Interval: "1d", Close: c, Open: c, High: c + 2, Low: c - 2, Time: base.AddDate(0, 0, i)}
	}
	return out
}

func engineWith(names ...string) *strategy.Engine {
	e := strategy.NewEngine()
	for _, n := range names {
		e.Register(&mockBtStrategy{name: n})
	}
	return e
}

// functional[0]
func TestExecuteBacktest_RunsEngineAndOutputs(t *testing.T) {
	prov := &stubProvider{data: sampleOHLCV()}
	var buf bytes.Buffer
	deps := backtestDeps{provider: prov, strategies: engineWith("mock"), out: &buf}

	err := executeBacktest(deps, "mock", "AAPL", "2026-01-01", "2026-01-10")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prov.calls == 0 {
		t.Fatal("provider was not invoked — engine not actually run")
	}
	out := buf.String()
	for _, want := range []string{"Signals", "Trades", "Win Rate", "Total Return"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q section.\n--- output ---\n%s", want, out)
		}
	}
}

// functional[1]
func TestExecuteBacktest_UnknownStrategy(t *testing.T) {
	prov := &stubProvider{data: sampleOHLCV()}
	var buf bytes.Buffer
	deps := backtestDeps{provider: prov, strategies: engineWith("ma_crossover"), out: &buf}

	err := executeBacktest(deps, "ghost", "AAPL", "2026-01-01", "2026-01-10")
	if err == nil {
		t.Fatal("expected non-nil error (non-zero exit) for unknown strategy")
	}
	out := buf.String()
	if !strings.Contains(out, "ma_crossover") {
		t.Errorf("expected available strategies listed, got:\n%s", out)
	}
	if prov.calls != 0 {
		t.Error("provider should not be called when strategy is unknown")
	}
}

// boundary[0]
func TestExecuteBacktest_FromAfterTo(t *testing.T) {
	prov := &stubProvider{data: sampleOHLCV()}
	deps := backtestDeps{provider: prov, strategies: engineWith("mock"), out: &bytes.Buffer{}}

	err := executeBacktest(deps, "mock", "AAPL", "2026-01-10", "2026-01-01")
	if err == nil {
		t.Fatal("expected error when --from is after --to")
	}
	if prov.calls != 0 {
		t.Error("provider should not be called when date range is invalid")
	}
}

// boundary[1]
func TestExecuteBacktest_EmptyData(t *testing.T) {
	prov := &stubProvider{data: nil}
	var buf bytes.Buffer
	deps := backtestDeps{provider: prov, strategies: engineWith("mock"), out: &buf}

	err := executeBacktest(deps, "mock", "AAPL", "2026-01-01", "2026-01-10")
	if err != nil {
		t.Fatalf("empty data should be a friendly no-op, not an error: %v", err)
	}
	if !strings.Contains(strings.ToLower(buf.String()), "no historical data") {
		t.Errorf("expected friendly empty-data message, got:\n%s", buf.String())
	}
}

// error_handling[0]
func TestExecuteBacktest_FetchError(t *testing.T) {
	prov := &stubProvider{err: errors.New("api unreachable")}
	deps := backtestDeps{provider: prov, strategies: engineWith("mock"), out: &bytes.Buffer{}}

	err := executeBacktest(deps, "mock", "AAPL", "2026-01-01", "2026-01-10")
	if err == nil {
		t.Fatal("expected error (non-zero exit) when history fetch fails")
	}
}

// boundary: invalid date format
func TestExecuteBacktest_InvalidDate(t *testing.T) {
	deps := backtestDeps{provider: &stubProvider{}, strategies: engineWith("mock"), out: &bytes.Buffer{}}

	if err := executeBacktest(deps, "mock", "AAPL", "not-a-date", "2026-01-10"); err == nil {
		t.Fatal("expected error for invalid --from date")
	}
}
