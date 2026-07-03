package main

import (
	"sort"
	"testing"

	"github.com/newthinker/atlas/internal/app"
	"github.com/newthinker/atlas/internal/collector/lixinger"
	"github.com/newthinker/atlas/internal/config"
	"go.uber.org/zap"
)

// buildCollectors with an empty config must succeed, register nothing that
// requires network, and return a nil-safe cleanup.
func TestBuildCollectors_EmptyConfig(t *testing.T) {
	cfg := config.Defaults()
	cfg.Collectors = nil // 无任何采集器配置
	application := app.New(cfg, zap.NewNop())

	cleanup, err := buildCollectors(cfg, application, zap.NewNop())
	if err != nil {
		t.Fatalf("buildCollectors: %v", err)
	}
	cleanup() // 必须 nil-safe,不 panic
	if n := len(application.GetCollectors()); n != 0 {
		t.Errorf("empty config should register no collectors, got %d", n)
	}
}

// With yahoo/eastmoney/crypto enabled, the exact set of registered collector
// names must match the pre-refactor expectation — a machine-checkable
// zero-change anchor for the serve.go migration (AD-8/B10).
func TestBuildCollectors_Defaults(t *testing.T) {
	cfg := config.Defaults()
	cfg.Collectors = map[string]config.CollectorConfig{
		"yahoo":     {Enabled: true},
		"eastmoney": {Enabled: true},
		"crypto":    {Enabled: true},
	}
	application := app.New(cfg, zap.NewNop())

	cleanup, err := buildCollectors(cfg, application, zap.NewNop())
	if err != nil {
		t.Fatalf("buildCollectors: %v", err)
	}
	defer cleanup()

	var got []string
	for _, c := range application.GetCollectors() {
		got = append(got, c.Name())
	}
	sort.Strings(got)
	want := []string{"crypto", "eastmoney", "yahoo"}
	if len(got) != len(want) {
		t.Fatalf("collector set = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("collector set = %v, want %v", got, want)
		}
	}
}

// fundamentalSourceOrNil must return an untyped-nil interface for a nil
// collector (no typed-nil trap, mirroring valuationSourceOrNil) and a live
// source otherwise — this is what gates buildFundamental's PE/PB/dividend path.
func TestFundamentalSourceOrNil(t *testing.T) {
	if fs := fundamentalSourceOrNil(nil); fs != nil {
		t.Errorf("nil collector must yield an untyped-nil interface, got %v", fs)
	}
	if fs := fundamentalSourceOrNil(lixinger.New("dummy-key")); fs == nil {
		t.Error("live collector must yield a non-nil FundamentalSource")
	}
}
