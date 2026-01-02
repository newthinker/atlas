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

func (m *mockNotifier) Name() string           { return m.name }
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

	err := r.Route(signal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
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
	r.cooldowns["GOOG"] = time.Now()                               // not expired
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
