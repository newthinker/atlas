package main

import (
	"slices"
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

// collectorNames 返回已登记采集器名的升序列表。
func collectorNames(application *app.App) []string {
	var got []string
	for _, c := range application.GetCollectors() {
		got = append(got, c.Name())
	}
	sort.Strings(got)
	return got
}

// TASK-005 f5:tushare/baostock 作为 A 股行情二跳/三跳登记进 registry(spec §2,ADR#10)。
//
// baostock 的地址来自 PrismConfig 默认值,但 buildCollectors 在 serve.go 里跑在
// prismCfg.ApplyDefaults() **之前**,且那次 ApplyDefaults 作用在一份副本上;runtime
// configs/config.yaml 也没写 baostock_base_url。若这里直接读 cfg.Prism.BaostockBaseURL,
// 登记条件永远为假 —— 三跳静默缺席且无任何一层会报错。故本用例刻意不设该字段。
func TestBuildCollectors_RegistersAShareBackupHops(t *testing.T) {
	cfg := config.Defaults()
	cfg.Collectors = map[string]config.CollectorConfig{
		"yahoo":     {Enabled: true},
		"eastmoney": {Enabled: true},
		"tushare":   {Enabled: true, APIKey: "tok"},
	}
	cfg.Prism.Enabled = true // 不设 BaostockBaseURL:默认值必须自己套上
	application := app.New(cfg, zap.NewNop())

	cleanup, err := buildCollectors(cfg, application, zap.NewNop())
	if err != nil {
		t.Fatalf("buildCollectors: %v", err)
	}
	defer cleanup()

	got := collectorNames(application)
	want := []string{"baostock", "eastmoney", "tushare", "yahoo"}
	if !slices.Equal(got, want) {
		t.Fatalf("collector set = %v, want %v", got, want)
	}
}

// 缺 key 的 tushare 不得登记:它的每次调用都必然 40203,登记只会让降级链多一跳
// 无效等待。baostock 随 Prism 一起启停 —— Prism 关掉时那座桥通常没在跑。
func TestBuildCollectors_SkipsBackupHopsWhenUnconfigured(t *testing.T) {
	cfg := config.Defaults()
	cfg.Collectors = map[string]config.CollectorConfig{
		"yahoo":   {Enabled: true},
		"tushare": {Enabled: true}, // 无 APIKey
	}
	cfg.Prism.Enabled = false
	application := app.New(cfg, zap.NewNop())

	cleanup, err := buildCollectors(cfg, application, zap.NewNop())
	if err != nil {
		t.Fatalf("buildCollectors: %v", err)
	}
	defer cleanup()

	got := collectorNames(application)
	if slices.Contains(got, "tushare") {
		t.Errorf("缺 key 的 tushare 不得登记, got %v", got)
	}
	if slices.Contains(got, "baostock") {
		t.Errorf("Prism 未启用时不得登记 baostock, got %v", got)
	}
}
