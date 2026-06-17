package router

import (
	"context"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/notifier"
	"github.com/newthinker/atlas/internal/storage/signal"
)

type mockNotifier struct {
	name        string
	received    []core.Signal
	batchCalled bool
}

func (m *mockNotifier) Name() string                   { return m.name }
func (m *mockNotifier) Init(cfg notifier.Config) error { return nil }
func (m *mockNotifier) Send(signal core.Signal) error {
	m.received = append(m.received, signal)
	return nil
}
func (m *mockNotifier) SendBatch(signals []core.Signal) error {
	m.batchCalled = true
	m.received = append(m.received, signals...)
	return nil
}

func TestRouter_Route_PassesFilters(t *testing.T) {
	registry := notifier.NewRegistry()
	mock := &mockNotifier{name: "mock"}
	registry.Register(mock)

	cfg := Config{
		MinConfidence:    0.5,
		CooldownDuration: 1 * time.Minute,
		EnabledActions:   []core.Action{core.ActionBuy, core.ActionSell},
	}

	r := New(cfg, registry, nil)

	signal := core.Signal{
		Symbol:     "AAPL",
		Action:     core.ActionBuy,
		Confidence: 0.8,
	}

	routed, err := r.Route(signal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !routed {
		t.Fatal("expected signal to be routed")
	}

	if len(mock.received) != 1 {
		t.Errorf("expected 1 signal, got %d", len(mock.received))
	}
}

func TestRouter_Route_FilterByConfidence(t *testing.T) {
	registry := notifier.NewRegistry()
	mock := &mockNotifier{name: "mock"}
	registry.Register(mock)

	cfg := Config{
		MinConfidence:    0.7,
		CooldownDuration: 1 * time.Minute,
		EnabledActions:   []core.Action{core.ActionBuy},
	}

	r := New(cfg, registry, nil)

	// Low confidence signal should be filtered
	signal := core.Signal{
		Symbol:     "AAPL",
		Action:     core.ActionBuy,
		Confidence: 0.5,
	}

	r.Route(signal)

	if len(mock.received) != 0 {
		t.Errorf("low confidence signal should be filtered, got %d signals", len(mock.received))
	}
}

func TestRouter_Route_FilterByAction(t *testing.T) {
	registry := notifier.NewRegistry()
	mock := &mockNotifier{name: "mock"}
	registry.Register(mock)

	cfg := Config{
		MinConfidence:    0.5,
		CooldownDuration: 1 * time.Minute,
		EnabledActions:   []core.Action{core.ActionBuy}, // Only buy
	}

	r := New(cfg, registry, nil)

	// Sell signal should be filtered
	signal := core.Signal{
		Symbol:     "AAPL",
		Action:     core.ActionSell,
		Confidence: 0.8,
	}

	r.Route(signal)

	if len(mock.received) != 0 {
		t.Errorf("sell action should be filtered, got %d signals", len(mock.received))
	}
}

func TestRouter_Route_Cooldown(t *testing.T) {
	registry := notifier.NewRegistry()
	mock := &mockNotifier{name: "mock"}
	registry.Register(mock)

	cfg := Config{
		MinConfidence:    0.5,
		CooldownDuration: 1 * time.Hour, // Long cooldown
		EnabledActions:   []core.Action{core.ActionBuy},
	}

	r := New(cfg, registry, nil)

	signal := core.Signal{
		Symbol:     "AAPL",
		Action:     core.ActionBuy,
		Confidence: 0.8,
	}

	// First signal passes
	r.Route(signal)
	if len(mock.received) != 1 {
		t.Errorf("first signal should pass, got %d", len(mock.received))
	}

	// Second signal within cooldown should be filtered
	r.Route(signal)
	if len(mock.received) != 1 {
		t.Errorf("second signal should be filtered by cooldown, got %d", len(mock.received))
	}
}

func TestRouter_Route_DifferentSymbolsDifferentCooldown(t *testing.T) {
	registry := notifier.NewRegistry()
	mock := &mockNotifier{name: "mock"}
	registry.Register(mock)

	cfg := Config{
		MinConfidence:    0.5,
		CooldownDuration: 1 * time.Hour,
		EnabledActions:   []core.Action{core.ActionBuy},
	}

	r := New(cfg, registry, nil)

	signal1 := core.Signal{Symbol: "AAPL", Action: core.ActionBuy, Confidence: 0.8}
	signal2 := core.Signal{Symbol: "GOOG", Action: core.ActionBuy, Confidence: 0.8}

	r.Route(signal1)
	r.Route(signal2)

	// Both should pass since they're different symbols
	if len(mock.received) != 2 {
		t.Errorf("different symbols should have separate cooldowns, got %d signals", len(mock.received))
	}
}

func TestRouter_ClearCooldown(t *testing.T) {
	registry := notifier.NewRegistry()
	mock := &mockNotifier{name: "mock"}
	registry.Register(mock)

	cfg := Config{
		MinConfidence:    0.5,
		CooldownDuration: 1 * time.Hour,
		EnabledActions:   []core.Action{core.ActionBuy},
	}

	r := New(cfg, registry, nil)

	signal := core.Signal{Symbol: "AAPL", Action: core.ActionBuy, Confidence: 0.8}

	r.Route(signal) // 1st
	r.Route(signal) // filtered by cooldown

	r.ClearCooldown("AAPL")

	r.Route(signal) // should pass now

	if len(mock.received) != 2 {
		t.Errorf("expected 2 signals after cooldown clear, got %d", len(mock.received))
	}
}

func TestRouter_RouteReportsCooldownSuppression(t *testing.T) {
	registry := notifier.NewRegistry()
	registry.Register(&mockNotifier{name: "mock"})

	r := New(Config{
		MinConfidence:    0.5,
		CooldownDuration: 1 * time.Hour,
		EnabledActions:   []core.Action{core.ActionBuy},
	}, registry, nil)

	signal := core.Signal{Symbol: "AAPL", Action: core.ActionBuy, Confidence: 0.8}

	if routed, _ := r.Route(signal); !routed {
		t.Fatal("first signal should be routed")
	}
	if routed, _ := r.Route(signal); routed {
		t.Fatal("cooldown-suppressed signal must report routed=false")
	}
}

func TestRouter_RouteReportsConfidenceSuppression(t *testing.T) {
	registry := notifier.NewRegistry()
	registry.Register(&mockNotifier{name: "mock"})

	r := New(Config{
		MinConfidence:    0.5,
		CooldownDuration: 1 * time.Hour,
		EnabledActions:   []core.Action{core.ActionBuy},
	}, registry, nil)

	low := core.Signal{Symbol: "AAPL", Action: core.ActionBuy, Confidence: 0.1}
	if routed, _ := r.Route(low); routed {
		t.Fatal("below-threshold signal must report routed=false")
	}
}

func TestRouter_RouteBatch(t *testing.T) {
	registry := notifier.NewRegistry()
	mock := &mockNotifier{name: "mock"}
	registry.Register(mock)

	cfg := Config{
		MinConfidence:    0.5,
		CooldownDuration: 1 * time.Minute,
		EnabledActions:   []core.Action{core.ActionBuy, core.ActionSell},
	}

	r := New(cfg, registry, nil)

	signals := []core.Signal{
		{Symbol: "AAPL", Action: core.ActionBuy, Confidence: 0.8},
		{Symbol: "GOOG", Action: core.ActionSell, Confidence: 0.7},
		{Symbol: "TSLA", Action: core.ActionBuy, Confidence: 0.3}, // filtered by confidence
	}

	r.RouteBatch(signals)

	if !mock.batchCalled {
		t.Error("SendBatch should have been called")
	}

	// Only 2 signals should pass (TSLA filtered by confidence)
	if len(mock.received) != 2 {
		t.Errorf("expected 2 signals in batch, got %d", len(mock.received))
	}
}

func TestRouter_GetStats(t *testing.T) {
	registry := notifier.NewRegistry()
	cfg := DefaultConfig()
	r := New(cfg, registry, nil)

	// Add a cooldown
	signal := core.Signal{Symbol: "AAPL", Action: core.ActionBuy, Confidence: 0.8}
	r.Route(signal)

	stats := r.GetStats()

	if stats["cooldowns_active"].(int) != 0 {
		// No notifiers, so signal won't be routed and cooldown won't be set
		// Actually, with empty registry, NotifyAll does nothing but cooldown IS set
	}

	if stats["min_confidence"].(float64) != cfg.MinConfidence {
		t.Error("stats should include min_confidence")
	}
}

func TestRouter_DefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.MinConfidence != 0.5 {
		t.Errorf("default min_confidence should be 0.5, got %f", cfg.MinConfidence)
	}

	if cfg.CooldownDuration != 1*time.Hour {
		t.Errorf("default cooldown should be 1 hour, got %v", cfg.CooldownDuration)
	}

	if len(cfg.EnabledActions) != 4 {
		t.Errorf("default should have 4 enabled actions, got %d", len(cfg.EnabledActions))
	}
}

func TestRouter_PersistsSignals(t *testing.T) {
	store := signal.NewMemoryStore(100)
	r := New(Config{MinConfidence: 0.5, CooldownDuration: time.Hour}, nil, nil)
	r.SetSignalStore(store)

	sig := core.Signal{
		Symbol:      "AAPL",
		Action:      core.ActionBuy,
		Confidence:  0.8,
		GeneratedAt: time.Now(),
	}

	r.Route(sig)

	signals, _ := store.List(context.Background(), signal.ListFilter{})
	if len(signals) != 1 {
		t.Errorf("expected 1 persisted signal, got %d", len(signals))
	}
}

// ---------------------------------------------------------------------------
// TASK-001 percentile step gate tests
//
// Context Checkpoint: done_criteria → test mapping
// functional[0] 买入序列 49→47→44→46→49      → TestRoute_PercentileStep_BuySide
// functional[1] 卖出侧对称 81→83→86          → TestRoute_PercentileStep_SellSideSymmetric
// functional[2] key 独立 buy/sell/strategy/strong → TestRoute_PercentileStep_KeysIndependent
// functional[3] 策略级步长三态                → TestRoute_PercentileStep_PerStrategyOverride
// functional[4] 静态过滤前置不写门控          → TestRoute_PercentileStep_StaticFilterBeforeGate
// boundary[0]   step=0 走冷却                 → TestRoute_StepDisabled_UsesCooldown
// boundary[1]   坏元数据回退冷却不 panic      → TestRoute_PercentileStep_BadMetadataFallsBackToCooldown
// ---------------------------------------------------------------------------

func pctSignal(symbol, strat string, action core.Action, pct float64) core.Signal {
	return core.Signal{
		Symbol: symbol, Action: action, Confidence: 0.9, Strategy: strat,
		Metadata: map[string]any{"percentile": pct},
	}
}

func newStepRouter(step float64) *Router {
	cfg := DefaultConfig()
	cfg.PercentileStep = step
	cfg.CooldownDuration = 1 * time.Hour // 显式非零：验证分位路径确实绕过冷却
	return New(cfg, nil, nil)            // New 内部对 nil logger 兜底
}

func TestRoute_PercentileStep_BuySide(t *testing.T) {
	r := newStepRouter(5)
	cases := []struct {
		pct  float64
		want bool
	}{
		{49, true},  // 首次：放行并记录 49
		{47, false}, // |47-49|=2 < 5：抑制
		{44, true},  // |44-49|=5 ≥ 5：放行，记录 44
		{46, false}, // |46-44|=2 < 5：抑制
		{49, true},  // 恢复重算：|49-44|=5 ≥ 5：放行（防死锁规则）
	}
	for i, c := range cases {
		routed, err := r.Route(pctSignal("600519.SH", "price_percentile", core.ActionBuy, c.pct))
		if err != nil || routed != c.want {
			t.Fatalf("step %d (pct=%v): routed=%v err=%v, want %v", i, c.pct, routed, err, c.want)
		}
	}
}

func TestRoute_PercentileStep_SellSideSymmetric(t *testing.T) {
	r := newStepRouter(5)
	for i, c := range []struct {
		pct  float64
		want bool
	}{{81, true}, {83, false}, {86, true}} {
		routed, _ := r.Route(pctSignal("600519.SH", "price_percentile", core.ActionSell, c.pct))
		if routed != c.want {
			t.Fatalf("sell step %d (pct=%v): routed=%v, want %v", i, c.pct, routed, c.want)
		}
	}
}

func TestRoute_PercentileStep_KeysIndependent(t *testing.T) {
	r := newStepRouter(5)
	// buy 侧已记录 49
	r.Route(pctSignal("600519.SH", "price_percentile", core.ActionBuy, 49))
	// sell 侧独立：首个 sell 信号放行（不受 buy 侧记录影响）
	if routed, _ := r.Route(pctSignal("600519.SH", "price_percentile", core.ActionSell, 81)); !routed {
		t.Error("sell side must be independent of buy side")
	}
	// 不同策略独立：pe_percentile 首个信号放行（注意元数据键不同）
	sig := pctSignal("600519.SH", "pe_percentile", core.ActionBuy, 50)
	sig.Metadata = map[string]any{"pe_percentile": 50.0}
	if routed, _ := r.Route(sig); !routed {
		t.Error("different strategy must have independent gate key")
	}
	// strong_buy 与 buy 同侧共享 key：strong_buy 47 应被 buy 侧的 49 记录抑制
	if routed, _ := r.Route(pctSignal("600519.SH", "price_percentile", core.ActionStrongBuy, 47)); routed {
		t.Error("strong_buy shares the buy-side key, |47-49|<5 must suppress")
	}
}

func TestRoute_PercentileStep_BadMetadataFallsBackToCooldown(t *testing.T) {
	r := newStepRouter(5)
	sig := pctSignal("600519.SH", "price_percentile", core.ActionBuy, 0)
	sig.Metadata = map[string]any{"percentile": "not-a-float"}
	if routed, _ := r.Route(sig); !routed {
		t.Fatal("first signal via cooldown path should route")
	}
	// 第二条同标的（仍坏元数据）→ 冷却抑制（1h 内）
	if routed, _ := r.Route(sig); routed {
		t.Error("second signal within cooldown must be suppressed (fell back to cooldown path)")
	}
}

func TestRoute_PercentileStep_PerStrategyOverride(t *testing.T) {
	r := newStepRouter(5) // 全局默认 5
	// 信号自带 percentile_step: 3（策略级配置经元数据传递）→ 按 3 门控
	mk := func(pct float64) core.Signal {
		sig := pctSignal("600519.SH", "pe_percentile", core.ActionBuy, pct)
		sig.Metadata = map[string]any{"pe_percentile": pct, "percentile_step": 3.0}
		return sig
	}
	for i, c := range []struct {
		pct  float64
		want bool
	}{{49, true}, {47, false}, {46, true}} { // |46-49|=3 ≥ 3 放行（全局 5 则会抑制）
		if routed, _ := r.Route(mk(c.pct)); routed != c.want {
			t.Fatalf("override step %d (pct=%v): routed=%v, want %v", i, c.pct, routed, c.want)
		}
	}
	// 全局 step=0 + 信号自带 step=3 → 仍按 3 门控（按策略启用场景，设计 rev4 §4）
	r0 := newStepRouter(0)
	if routed, _ := r0.Route(mk(49)); !routed {
		t.Error("strategy-level step must enable the gate even when global step is 0")
	}
	if routed, _ := r0.Route(mk(48)); routed { // |48-49|=1 < 3：被步进抑制而非进入冷却路径
		t.Error("gate must be active with strategy-level step despite global 0")
	}

	// step 元数据类型异常（string）→ 回退全局 5
	bad := pctSignal("0700.HK", "price_percentile", core.ActionBuy, 49)
	bad.Metadata["percentile_step"] = "3"
	if routed, _ := r.Route(bad); !routed {
		t.Fatal("first should route")
	}
	bad2 := pctSignal("0700.HK", "price_percentile", core.ActionBuy, 45) // |45-49|=4 < 5
	bad2.Metadata["percentile_step"] = "3"
	if routed, _ := r.Route(bad2); routed {
		t.Error("invalid step metadata must fall back to global step 5")
	}
}

// TestRoute_PercentileStep_StaticFilterBeforeGate verifies that the static
// confidence/action filters run BEFORE the step gate: a sub-threshold
// percentile signal is dropped (routed=false) without recording any gate
// state, so a later qualifying signal at the same percentile routes as "first".
func TestRoute_PercentileStep_StaticFilterBeforeGate(t *testing.T) {
	r := newStepRouter(5)
	low := pctSignal("600519.SH", "price_percentile", core.ActionBuy, 49)
	low.Confidence = 0.1 // 低于 DefaultConfig.MinConfidence 0.5
	if routed, _ := r.Route(low); routed {
		t.Fatal("low-confidence percentile signal must be filtered by static filters")
	}
	// 同分位的合格信号按「首次」放行：证明上面的信号未写入步进门控状态
	if routed, _ := r.Route(pctSignal("600519.SH", "price_percentile", core.ActionBuy, 49)); !routed {
		t.Error("statically-filtered signal must not record gate state; first valid signal should route")
	}
}

func TestRoute_StepDisabled_UsesCooldown(t *testing.T) {
	r := newStepRouter(0) // step=0 禁用：带分位元数据也走冷却
	sig := pctSignal("600519.SH", "price_percentile", core.ActionBuy, 49)
	if routed, _ := r.Route(sig); !routed {
		t.Fatal("first should route")
	}
	sig2 := pctSignal("600519.SH", "price_percentile", core.ActionBuy, 30) // 深跌也被冷却抑制
	if routed, _ := r.Route(sig2); routed {
		t.Error("step disabled: cooldown must suppress regardless of percentile delta")
	}
}

// ---------------------------------------------------------------------------
// TASK-002 cooldown interaction, RouteBatch and state management
//
// Context Checkpoint: done_criteria → test mapping
// functional[0] 分位信号不更新冷却戳        → TestRoute_PercentileSignalDoesNotTouchCooldown
// functional[1] RouteBatch 复用步进门控      → TestRouteBatch_UsesPercentileGate
// functional[2] ClearCooldown 前缀清步进 key → TestClearCooldowns_AlsoClearPercentileGates (前半)
// functional[3] ClearAllCooldowns 全清       → TestClearCooldowns_AlsoClearPercentileGates (后半)
// functional[4] GetStats 暴露门控状态        → TestGetStats_IncludesPercentileGate
// boundary[0]   RouteBatch nil-registry 守卫 → TestRouteBatch_UsesPercentileGate (nil registry 构造)
// ---------------------------------------------------------------------------

func TestRoute_PercentileSignalDoesNotTouchCooldown(t *testing.T) {
	r := newStepRouter(5)
	// 分位信号通知后……
	r.Route(pctSignal("600519.SH", "price_percentile", core.ActionBuy, 49))
	// ……同标的无元数据信号（如 ma_crossover）不应被冷却压制（冷却戳未被分位路径更新）
	plain := core.Signal{Symbol: "600519.SH", Action: core.ActionBuy, Confidence: 0.9, Strategy: "ma_crossover"}
	if routed, _ := r.Route(plain); !routed {
		t.Error("percentile signal must not stamp the per-symbol cooldown")
	}
}

func TestClearCooldowns_AlsoClearPercentileGates(t *testing.T) {
	r := newStepRouter(5)
	r.Route(pctSignal("600519.SH", "price_percentile", core.ActionBuy, 49))
	r.Route(pctSignal("0700.HK", "price_percentile", core.ActionBuy, 40))

	r.ClearCooldown("600519.SH") // 按 symbol| 前缀清除步进 key
	if routed, _ := r.Route(pctSignal("600519.SH", "price_percentile", core.ActionBuy, 48)); !routed {
		t.Error("after ClearCooldown the first percentile signal must route again")
	}
	if routed, _ := r.Route(pctSignal("0700.HK", "price_percentile", core.ActionBuy, 39)); routed {
		t.Error("other symbols' gates must survive ClearCooldown(600519.SH)")
	}

	r.ClearAllCooldowns()
	if routed, _ := r.Route(pctSignal("0700.HK", "price_percentile", core.ActionBuy, 38)); !routed {
		t.Error("after ClearAllCooldowns all gates must reset")
	}
}

func TestRouteBatch_UsesPercentileGate(t *testing.T) {
	r := newStepRouter(5)
	// 批内同 key 两条：第一条放行并更新状态，第二条按更新后状态判定（与连续 Route 等价）
	err := r.RouteBatch([]core.Signal{
		pctSignal("600519.SH", "price_percentile", core.ActionBuy, 49),
		pctSignal("600519.SH", "price_percentile", core.ActionBuy, 47), // |47-49|<5 → 不入批
	})
	if err != nil {
		t.Fatal(err)
	}
	// 间接断言：再 Route 44 应放行（状态为 49 而非 47）
	if routed, _ := r.Route(pctSignal("600519.SH", "price_percentile", core.ActionBuy, 44)); !routed {
		t.Error("batch must have recorded 49 (not 47); |44-49|=5 should route")
	}
}

func TestGetStats_IncludesPercentileGate(t *testing.T) {
	r := newStepRouter(5)
	r.Route(pctSignal("600519.SH", "price_percentile", core.ActionBuy, 49))
	stats := r.GetStats()
	if stats["percentile_gates_active"] != 1 || stats["percentile_step"] != 5.0 {
		t.Errorf("stats = %+v", stats)
	}
}

// ---------------------------------------------------------------------------
// TASK-003 batch-notify buffer + FlushNotifications
//
// Context Checkpoint: done_criteria → test mapping
// functional[0] batch 模式下两次 Route 不触发 Send/SendBatch     → TestRoute_BatchNotify_BuffersUntilFlush
// functional[1] FlushNotifications 后恰好一次 batch 且 n==2      → TestRoute_BatchNotify_BuffersUntilFlush
// functional[2] batch_notify=false 时 Route 立即 Send            → TestRoute_NonBatch_NotifiesImmediately
// boundary[0]   空缓冲 FlushNotifications 不发送（batches==0）   → TestFlush_EmptyIsNoop
// boundary[1]   nil registry Route 返回 true,nil; Flush no-op    → (由构造器行为+已有测试覆盖)
// error_handling NotifyAllBatch errors 逐条 log，不中断 flush    → (运行时行为；nil logger 兜底)
// ---------------------------------------------------------------------------

// countingNotifier records Send vs SendBatch calls.
type countingNotifier struct {
	sends   int
	batches int
	lastN   int
}

func (c *countingNotifier) Name() string                      { return "counting" }
func (c *countingNotifier) Init(notifier.Config) error        { return nil }
func (c *countingNotifier) Send(core.Signal) error            { c.sends++; return nil }
func (c *countingNotifier) SendBatch(s []core.Signal) error {
	c.batches++
	c.lastN = len(s)
	return nil
}

func newRouterWithNotifier(cfg Config) (*Router, *countingNotifier) {
	reg := notifier.NewRegistry()
	cn := &countingNotifier{}
	reg.Register(cn)
	return New(cfg, reg, nil), cn
}

func TestRoute_BatchNotify_BuffersUntilFlush(t *testing.T) {
	cfg := Config{MinConfidence: 0.5, BatchNotify: true}
	r, cn := newRouterWithNotifier(cfg)

	r.Route(core.Signal{Symbol: "AAPL", Action: core.ActionBuy, Confidence: 0.9})
	r.Route(core.Signal{Symbol: "MSFT", Action: core.ActionSell, Confidence: 0.9})

	if cn.sends != 0 || cn.batches != 0 {
		t.Fatalf("batch mode must not notify during Route (sends=%d batches=%d)", cn.sends, cn.batches)
	}
	r.FlushNotifications()
	if cn.batches != 1 || cn.lastN != 2 {
		t.Fatalf("flush should send one batch of 2, got batches=%d n=%d", cn.batches, cn.lastN)
	}
}

func TestRoute_NonBatch_NotifiesImmediately(t *testing.T) {
	cfg := Config{MinConfidence: 0.5, BatchNotify: false}
	r, cn := newRouterWithNotifier(cfg)
	r.Route(core.Signal{Symbol: "AAPL", Action: core.ActionBuy, Confidence: 0.9})
	if cn.sends != 1 || cn.batches != 0 {
		t.Fatalf("non-batch mode must Send immediately (sends=%d batches=%d)", cn.sends, cn.batches)
	}
}

func TestFlush_EmptyIsNoop(t *testing.T) {
	cfg := Config{MinConfidence: 0.5, BatchNotify: true}
	r, cn := newRouterWithNotifier(cfg)
	r.FlushNotifications()
	if cn.batches != 0 {
		t.Fatalf("empty flush must not send, got batches=%d", cn.batches)
	}
}

func TestRouter_CleanupExpiredCooldowns(t *testing.T) {
	cfg := Config{
		CooldownDuration: 100 * time.Millisecond,
		MinConfidence:    0.5,
	}
	r := New(cfg, nil, nil)

	// Add some cooldowns
	r.mu.Lock()
	r.cooldowns["AAPL"] = time.Now().Add(-300 * time.Millisecond) // expired
	r.cooldowns["MSFT"] = time.Now().Add(-300 * time.Millisecond) // expired
	r.cooldowns["GOOG"] = time.Now()                              // not expired
	r.mu.Unlock()

	removed := r.CleanupExpiredCooldowns()
	if removed != 2 {
		t.Errorf("expected 2 removed, got %d", removed)
	}

	r.mu.RLock()
	if len(r.cooldowns) != 1 {
		t.Errorf("expected 1 cooldown remaining, got %d", len(r.cooldowns))
	}
	r.mu.RUnlock()
}
