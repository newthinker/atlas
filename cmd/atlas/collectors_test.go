package main

import (
	"slices"
	"sort"
	"testing"

	"github.com/newthinker/atlas/internal/app"
	"github.com/newthinker/atlas/internal/collector"
	"github.com/newthinker/atlas/internal/collector/eastmoney"
	"github.com/newthinker/atlas/internal/collector/lixinger"
	"github.com/newthinker/atlas/internal/collector/yahoo"
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

// ——— TASK-011：装饰器删除后的正向断言 ———
//
// 被删的 TestMaybeCache_FundamentalNotWrapped 证明的是「maybeCache **不会**包装
// FundamentalCollector」。装饰器删掉后那条命题失去了主语，但它守护的性质仍在：
// **扩展接口的 type assertion 必须在装配之后依然成立**（验收标准 6 / 设计 §1.3）。
// 只删证据不算修复，故用下面两条正向断言承接。

// TestBuildCollectorsRegistersUnwrappedCollectors 断言装配路径不再包装任何 collector。
//
// cache.Enabled 特意设为 true —— 那正是从前触发 maybeCache 包装的开关。若将来有人
// 重新引入某种装饰器，registry 里的具体类型就不再是原始类型，这条会红。
func TestBuildCollectorsRegistersUnwrappedCollectors(t *testing.T) {
	cfg := config.Defaults()
	cfg.Collector.Cache.Enabled = true // 从前这个开关会让 collector 被 CachedCollector 包住
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

	got := application.GetCollectors()
	if len(got) == 0 {
		t.Fatal("装配后 registry 为空，下面的断言会 vacuously true")
	}
	for _, c := range got {
		switch v := c.(type) {
		case *yahoo.Yahoo, *eastmoney.Eastmoney:
			// 原始具体类型，未被任何装饰器包装
		default:
			if c.Name() == "yahoo" || c.Name() == "eastmoney" {
				t.Errorf("%s 注册的是 %T 而非原始具体类型——装饰器会遮蔽扩展接口", c.Name(), v)
			}
		}
	}
}

// lixinger 的两个扩展接口在**编译期**钉住。
//
// ⚠ 这里刻意用 var 断言而非运行期测试。我最初写的是 `var c collector.Collector =
// lixinger.New(...)` 再 type assert 的测试，注入变异时发现它在「代码能编译」的前提下
// **恒真** —— lixinger 直接实现这两个接口，经 collector.Collector 传递后动态类型不变，
// 断言必然成功。**形式与内容不符的测试比没有更糟**：它看起来在守护运行期行为，实际
// 什么都不防，还会让人以为这块已经有覆盖。
//
// 真正的运行期风险是「装配时被装饰器包住导致扩展接口被遮蔽」——那由
// TestBuildCollectorsRegistersUnwrappedCollectors 覆盖（变异 D1 实证：注册一个只嵌入
// collector.Collector 的包装类型即转红）。
//
// 注：DoD 写的 "ValuationCollector" 在仓库中不存在；valuation 能力的消费接口是
// app.ValuationSource，buildCollectors 经 application.SetValuationSources 注入。
var (
	_ collector.FundamentalCollector = (*lixinger.Lixinger)(nil)
	_ app.ValuationSource            = (*lixinger.Lixinger)(nil)
)
