package main

// Context Checkpoint: done_criteria → test mapping (TASK-203)
// functional[0]   "Enabled 循环 Snapshot→派生→SetMetrics→EvaluateAll，触发送适配器"
//   → TestAlertRunner_EvaluateOnce_FiresNotifier / TestAlertRunner_Run_TicksAndEvaluates
// functional[1]   "http_error_rate 相邻快照增量；首轮无基线不产出；负增量 clamp 0"
//   → TestDerivedMetrics_HTTPErrorRate_FirstSnapshotNoBaseline / _NormalDelta / _NegativeDeltaClampsZero
// functional[2]   "signals_24h = SignalStore.Count(from=now-24h)"
//   → TestAlertRunner_Signals24h_FromCount
// functional[3]   "config.AlertRule→alert.Rule 映射（含 for 字符串解码）"
//   → TestMapRules_FieldMapping / TestMapRules_DurationStringDecode
// boundary[0]     "Enabled=false 不起 goroutine、不构造 Evaluator"
//   → TestMaybeStartAlertRunner_DisabledReturnsNil
// boundary[1]     "总请求增量 0 不除零"
//   → TestDerivedMetrics_HTTPErrorRate_ZeroTotalDeltaNoDivZero
// error_handling  "ctx 取消 goroutine 有限时间返回；单条 Notify 失败不中断"
//   → TestAlertRunner_Run_StopsOnCtxCancel / TestAlertRunner_EvaluateOnce_NotifierErrorDoesNotStop
//                  + TestAlertRunner_Signals24h_CountErrorSkips

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/alert"
	"github.com/newthinker/atlas/internal/config"
	"github.com/newthinker/atlas/internal/metrics"
	"github.com/newthinker/atlas/internal/notifier"
	"github.com/newthinker/atlas/internal/notifier/telegram"
	signalstore "github.com/newthinker/atlas/internal/storage/signal"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// fakeAlertNotifier structurally satisfies alert.Notifier and records the
// messages it receives so tests can assert on delivery.
type fakeAlertNotifier struct {
	mu   sync.Mutex
	msgs []string
	err  error
}

func (f *fakeAlertNotifier) Name() string { return "fake" }

func (f *fakeAlertNotifier) Notify(msg string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.msgs = append(f.msgs, msg)
	return f.err
}

func (f *fakeAlertNotifier) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.msgs)
}

// fakeSignalCounter implements the signalCounter seam used by the alert loop.
type fakeSignalCounter struct {
	n        int
	err      error
	lastFrom time.Time
}

func (f *fakeSignalCounter) Count(_ context.Context, filter signalstore.ListFilter) (int, error) {
	f.lastFrom = filter.From
	return f.n, f.err
}

// countSince matches the alertRunner.count seam (func(since time.Time)).
func (f *fakeSignalCounter) countSince(since time.Time) (int, error) {
	f.lastFrom = since
	return f.n, f.err
}

// scriptedSnapshot returns snapshots in order (repeating the last), copying each
// so the runner may mutate the returned map without corrupting the fixture —
// matching the production Snapshot() contract of a fresh map per call.
func scriptedSnapshot(snaps ...map[string]float64) func() map[string]float64 {
	i := 0
	return func() map[string]float64 {
		idx := i
		if idx >= len(snaps) {
			idx = len(snaps) - 1
		}
		i++
		src := snaps[idx]
		cp := make(map[string]float64, len(src))
		for k, v := range src {
			cp[k] = v
		}
		return cp
	}
}

func newEvaluatorWith(n alert.Notifier) *alert.Evaluator {
	return alert.NewEvaluator([]alert.Notifier{n})
}

// --- derived metric math (functional[1], boundary[1]) ---

func TestDerivedMetrics_HTTPErrorRate_FirstSnapshotNoBaseline(t *testing.T) {
	var d derivedMetrics
	rate, produced := d.httpErrorRate(map[string]float64{
		"http_requests_total":     100,
		"http_requests_total_5xx": 5,
	})
	if produced {
		t.Errorf("first snapshot must not produce a rate (no baseline), got produced=true rate=%v", rate)
	}
}

func TestDerivedMetrics_HTTPErrorRate_NormalDelta(t *testing.T) {
	var d derivedMetrics
	// Establish baseline.
	d.httpErrorRate(map[string]float64{"http_requests_total": 100, "http_requests_total_5xx": 5})
	// 100 more requests, 20 more 5xx → 0.2.
	rate, produced := d.httpErrorRate(map[string]float64{"http_requests_total": 200, "http_requests_total_5xx": 25})
	if !produced {
		t.Fatal("second snapshot must produce a rate")
	}
	if rate != 0.2 {
		t.Errorf("rate = %v, want 0.2", rate)
	}
}

func TestDerivedMetrics_HTTPErrorRate_NegativeDeltaClampsZero(t *testing.T) {
	var d derivedMetrics
	d.httpErrorRate(map[string]float64{"http_requests_total": 200, "http_requests_total_5xx": 30})
	// Counter reset: 5xx dropped below previous → clamp delta to 0 → rate 0.
	rate, produced := d.httpErrorRate(map[string]float64{"http_requests_total": 300, "http_requests_total_5xx": 5})
	if !produced {
		t.Fatal("expected produced=true")
	}
	if rate != 0 {
		t.Errorf("negative 5xx delta must clamp to rate 0, got %v", rate)
	}
}

func TestDerivedMetrics_HTTPErrorRate_ZeroTotalDeltaNoDivZero(t *testing.T) {
	var d derivedMetrics
	d.httpErrorRate(map[string]float64{"http_requests_total": 100, "http_requests_total_5xx": 5})
	// No new requests → total delta 0 → must not divide by zero, produce 0.
	rate, produced := d.httpErrorRate(map[string]float64{"http_requests_total": 100, "http_requests_total_5xx": 5})
	if !produced {
		t.Fatal("expected produced=true")
	}
	if rate != 0 {
		t.Errorf("zero total delta must yield rate 0, got %v", rate)
	}
}

// --- rule mapping (functional[3]) ---

func TestMapRules_FieldMapping(t *testing.T) {
	in := []config.AlertRule{
		{Name: "r1", Expr: "http_error_rate > 0.1", For: 5 * time.Minute, Severity: "warning", Message: "m1"},
		{Name: "r2", Expr: "signals_24h < 1", For: 0, Severity: "critical", Message: "m2"},
	}
	out := mapRules(in)
	if len(out) != 2 {
		t.Fatalf("mapRules len = %d, want 2", len(out))
	}
	for i := range in {
		if out[i].Name != in[i].Name || out[i].Expr != in[i].Expr || out[i].For != in[i].For ||
			out[i].Severity != in[i].Severity || out[i].Message != in[i].Message {
			t.Errorf("rule %d mismatch: got %+v want %+v", i, out[i], in[i])
		}
	}
}

// functional[3]: a string duration ("5m") in config must decode to a
// time.Duration and survive mapping to the evaluator rule.
func TestMapRules_DurationStringDecode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yaml := "alerts:\n" +
		"  enabled: true\n" +
		"  check_interval: 30s\n" +
		"  rules:\n" +
		"    - name: high_error_rate\n" +
		"      expr: \"http_error_rate > 0.1\"\n" +
		"      for: 5m\n" +
		"      severity: warning\n" +
		"      message: \"API 5xx error rate above 10%\"\n"
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if len(cfg.Alerts.Rules) != 1 {
		t.Fatalf("rules len = %d, want 1", len(cfg.Alerts.Rules))
	}
	if cfg.Alerts.Rules[0].For != 5*time.Minute {
		t.Errorf("config For = %v, want 5m (string duration must decode)", cfg.Alerts.Rules[0].For)
	}
	rules := mapRules(cfg.Alerts.Rules)
	if rules[0].For != 5*time.Minute {
		t.Errorf("mapped For = %v, want 5m", rules[0].For)
	}
	if cfg.Alerts.CheckInterval != 30*time.Second {
		t.Errorf("check_interval = %v, want 30s", cfg.Alerts.CheckInterval)
	}
}

// --- evaluateOnce pipeline (functional[0], functional[2]) ---

func TestAlertRunner_EvaluateOnce_FiresNotifier(t *testing.T) {
	fake := &fakeAlertNotifier{}
	r := &alertRunner{
		rules:     []alert.Rule{{Name: "err", Expr: "http_error_rate > 0.1", Severity: "warning", Message: "high"}},
		evaluator: newEvaluatorWith(fake),
		snapshot: scriptedSnapshot(
			map[string]float64{"http_requests_total": 100, "http_requests_total_5xx": 5},
			map[string]float64{"http_requests_total": 200, "http_requests_total_5xx": 25}, // rate 0.2 > 0.1
		),
		count: func(time.Time) (int, error) { return 0, nil },
		now:   time.Now,
		log:   zap.NewNop(),
	}
	r.evaluateOnce() // baseline, no rate produced → no fire
	if fake.count() != 0 {
		t.Fatalf("baseline round should not fire, got %d", fake.count())
	}
	r.evaluateOnce() // rate 0.2 → fire
	if fake.count() != 1 {
		t.Errorf("expected 1 notification after threshold breach, got %d", fake.count())
	}
}

func TestAlertRunner_Signals24h_FromCount(t *testing.T) {
	fake := &fakeAlertNotifier{}
	counter := &fakeSignalCounter{n: 7}
	now := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	r := &alertRunner{
		rules:     []alert.Rule{{Name: "sig", Expr: "signals_24h > 5", Severity: "info", Message: "busy"}},
		evaluator: newEvaluatorWith(fake),
		snapshot:  scriptedSnapshot(map[string]float64{}),
		count:     counter.countSince,
		now:       func() time.Time { return now },
		log:       zap.NewNop(),
	}
	r.evaluateOnce()
	if fake.count() != 1 {
		t.Errorf("signals_24h=7 > 5 should fire once, got %d", fake.count())
	}
	if want := now.Add(-24 * time.Hour); !counter.lastFrom.Equal(want) {
		t.Errorf("Count from = %v, want now-24h %v", counter.lastFrom, want)
	}
}

func TestAlertRunner_Signals24h_CountErrorSkips(t *testing.T) {
	fake := &fakeAlertNotifier{}
	counter := &fakeSignalCounter{err: context.DeadlineExceeded}
	core, logs := observer.New(zapcore.WarnLevel)
	r := &alertRunner{
		rules:     []alert.Rule{{Name: "sig", Expr: "signals_24h > 5", Severity: "info", Message: "busy"}},
		evaluator: newEvaluatorWith(fake),
		snapshot:  scriptedSnapshot(map[string]float64{}),
		count:     counter.countSince,
		now:       time.Now,
		log:       zap.New(core),
	}
	r.evaluateOnce() // count errors → signals_24h absent → rule must not fire, no panic
	if fake.count() != 0 {
		t.Errorf("count error must skip signals_24h; rule should not fire, got %d", fake.count())
	}
	if logs.Len() == 0 {
		t.Error("expected a warn log when signals count fails")
	}
}

// error_handling: a notifier returning an error must not abort evaluation of the
// remaining rules.
func TestAlertRunner_EvaluateOnce_NotifierErrorDoesNotStop(t *testing.T) {
	fake := &fakeAlertNotifier{err: context.DeadlineExceeded}
	counter := &fakeSignalCounter{n: 7}
	r := &alertRunner{
		rules: []alert.Rule{
			{Name: "a", Expr: "signals_24h > 5", Severity: "info", Message: "a"},
			{Name: "b", Expr: "signals_24h > 6", Severity: "info", Message: "b"},
		},
		evaluator: newEvaluatorWith(fake),
		snapshot:  scriptedSnapshot(map[string]float64{}),
		count:     counter.countSince,
		now:       time.Now,
		log:       zap.NewNop(),
	}
	r.evaluateOnce()
	if fake.count() != 2 {
		t.Errorf("both firing rules must reach the notifier despite Notify error, got %d", fake.count())
	}
}

// --- run loop lifecycle (functional[0], error_handling) ---

func TestAlertRunner_Run_TicksAndEvaluates(t *testing.T) {
	var ticks int32
	r := &alertRunner{
		interval:  5 * time.Millisecond,
		rules:     nil,
		evaluator: newEvaluatorWith(&fakeAlertNotifier{}),
		snapshot: func() map[string]float64 {
			atomic.AddInt32(&ticks, 1)
			return map[string]float64{}
		},
		count: func(time.Time) (int, error) { return 0, nil },
		now:   time.Now,
		log:   zap.NewNop(),
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { r.run(ctx); close(done) }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done
	if atomic.LoadInt32(&ticks) < 1 {
		t.Errorf("run should have evaluated at least once, got %d ticks", ticks)
	}
}

func TestAlertRunner_Run_StopsOnCtxCancel(t *testing.T) {
	r := &alertRunner{
		interval:  time.Hour, // large so only ctx cancel can end the loop
		evaluator: newEvaluatorWith(&fakeAlertNotifier{}),
		snapshot:  func() map[string]float64 { return map[string]float64{} },
		count:     func(time.Time) (int, error) { return 0, nil },
		now:       time.Now,
		log:       zap.NewNop(),
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { r.run(ctx); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("run did not return within 2s after ctx cancel (goroutine leak)")
	}
}

// --- start gate (boundary[0]) ---

func TestMaybeStartAlertRunner_DisabledReturnsNil(t *testing.T) {
	cfg := &config.Config{Alerts: config.AlertsConfig{Enabled: false}}
	r := maybeStartAlertRunner(context.Background(), cfg, nil, nil, &fakeSignalCounter{}, zap.NewNop())
	if r != nil {
		t.Error("alerts disabled must not start a runner (nil expected)")
	}
}

// Enabled path with real notifiers + registry: exercises adapter wrapping, the
// registry-backed snapshot closure, the count seam, and the default interval
// fallback (check_interval unset).
func TestMaybeStartAlertRunner_WrapsNotifiersAndRegistrySnapshot(t *testing.T) {
	reg := metrics.NewRegistry()
	cfg := &config.Config{Alerts: config.AlertsConfig{Enabled: true, CheckInterval: 0}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r := maybeStartAlertRunner(ctx, cfg,
		[]notifier.Notifier{telegram.New("tok", "chat")}, reg, &fakeSignalCounter{n: 3}, zap.NewNop())
	if r == nil {
		t.Fatal("expected a runner")
	}
	if r.interval != defaultAlertInterval {
		t.Errorf("interval = %v, want default %v", r.interval, defaultAlertInterval)
	}
	if snap := r.snapshot(); snap == nil {
		t.Error("registry-backed snapshot must return a non-nil map")
	}
	if n, err := r.count(time.Now()); err != nil || n != 3 {
		t.Errorf("count seam = (%d, %v), want (3, nil)", n, err)
	}
}

func TestAlertRunner_EvaluateOnce_NilSnapshotNoPanic(t *testing.T) {
	r := &alertRunner{
		rules:     nil,
		evaluator: newEvaluatorWith(&fakeAlertNotifier{}),
		snapshot:  func() map[string]float64 { return nil },
		count:     func(time.Time) (int, error) { return 0, nil },
		now:       time.Now,
		log:       zap.NewNop(),
	}
	r.evaluateOnce() // nil snapshot must be handled without panic
}

func TestMaybeStartAlertRunner_EnabledStarts(t *testing.T) {
	cfg := &config.Config{Alerts: config.AlertsConfig{
		Enabled:       true,
		CheckInterval: 10 * time.Millisecond,
		Rules:         []config.AlertRule{{Name: "r", Expr: "http_error_rate > 0.1", Severity: "warning", Message: "m"}},
	}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := maybeStartAlertRunner(ctx, cfg, nil, nil, &fakeSignalCounter{}, zap.NewNop())
	if r == nil {
		t.Fatal("alerts enabled must start a runner (non-nil expected)")
	}
	if r.interval != 10*time.Millisecond {
		t.Errorf("runner interval = %v, want 10ms", r.interval)
	}
	if len(r.rules) != 1 {
		t.Errorf("runner rules = %d, want 1", len(r.rules))
	}
}
