package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/collector/policy"
	"github.com/newthinker/atlas/internal/config"
)

// Context Checkpoint: done_criteria → test mapping
//
// functional[1]     cache.ttl 经 ApplyTTL 施加到缓存类主题
//                       → TestInitPolicyGateAppliesCacheTTL
// functional[2]     collector.topics 逐主题覆盖生效（含配额）
//                       → TestInitPolicyGateAppliesTopicQuotaOverride
// functional[3]     collector.quota.path 被用作 FileStore 账本路径
//                       → TestInitPolicyGateUsesConfiguredQuotaPath
// boundary[0]       cache.enabled=false 时 TTL 全归零但限流仍生效
//                       → TestInitPolicyGateDisabledCacheStillThrottles
// error_handling[0] nil logger 不 panic / 账本路径不可写时仍完成 SetDefault
//                       → TestInitPolicyGateNilLoggerAndUnwritablePath
// non_functional[0] **【最高风险项 A1】initPolicyGate 必须在 loadConfigOrDefaults 内部**
//                       → TestLoadConfigOrDefaultsInitsPolicyGate
//
// ⚠ A1 的失败模式是「**测试全绿本身就是症状**」：若 initPolicyGate 放在
//   export_ohlcv 的调用点而非 helper 内部，serve 与 export 正常、而 prism refresh
//   拿到懒构造的无账本 Gate，配额彻底失效——上面 4 条 functional 测试全都照常绿，
//   因为它们直接调 initPolicyGate，测不到装配。
//   所以自查的问法不是「变异能否转红」，而是「**哪条测试的绿能证明 refresh 路径
//   真的初始化了 Gate**」。答案必须是 TestLoadConfigOrDefaultsInitsPolicyGate：
//   它走的是 loadConfigOrDefaults 这条 prism refresh 实际调用的入口。

func restoreGate(t *testing.T) {
	t.Helper()
	orig := policy.Default()
	t.Cleanup(func() { policy.SetDefault(orig) })
}

// TestInitPolicyGateAppliesCacheTTL 覆盖 functional[1]：cache.ttl **经 ApplyTTL
// 施加**到缓存类主题。
//
// ⚠ 判据必须能区分「TTL = 配置值」与「TTL = 内置值」。只断言「两次 Fetch 命中缓存」
// 是不够的——内置 TTL 是 5 分钟，删掉 ApplyTTL 后两次 Fetch 照样命中，断言照样绿
// （实测：变异 C2 下该写法存活）。故用一个**短 TTL 并跨过期点再取**：配置值生效时
// 第三次会重新调 fn，内置值生效时不会。
func TestInitPolicyGateAppliesCacheTTL(t *testing.T) {
	restoreGate(t)
	const ttl = 50 * time.Millisecond
	cfg := config.Defaults()
	cfg.Collector.Cache.Enabled = true
	cfg.Collector.Cache.TTL = ttl
	cfg.Collector.Quota.Path = filepath.Join(t.TempDir(), "quota.json")
	// 关掉 yahoo 的 500ms 闸门，否则每次 Fetch 都要等半秒
	iv := time.Duration(0)
	cfg.Collector.Topics = map[string]config.TopicConfig{
		"yahoo.chart": {MinInterval: &iv},
	}

	initPolicyGate(cfg, nil)

	fnCalls := 0
	fn := func() (int, error) { fnCalls++; return 1, nil }
	for i := 0; i < 2; i++ {
		if _, err := policy.Fetch(policy.Default(), "yahoo.chart", "AAPL", fn); err != nil {
			t.Fatal(err)
		}
	}
	if fnCalls != 1 {
		t.Fatalf("TTL 内同 key 应命中缓存: fn 调用 %d 次, want 1", fnCalls)
	}

	time.Sleep(2 * ttl)
	if _, err := policy.Fetch(policy.Default(), "yahoo.chart", "AAPL", fn); err != nil {
		t.Fatal(err)
	}
	if fnCalls != 2 {
		t.Errorf("过了配置的 %v 后应重新调 fn: fn 调用 %d 次, want 2"+
			"（仍为 1 说明用的是内置 5 分钟 TTL，cache.ttl 没有被 ApplyTTL 施加）", ttl, fnCalls)
	}
}

func TestInitPolicyGateDisabledCacheStillThrottles(t *testing.T) {
	restoreGate(t)
	cfg := config.Defaults()
	cfg.Collector.Cache.Enabled = false
	cfg.Collector.Quota.Path = filepath.Join(t.TempDir(), "quota.json")
	// 把 yahoo 闸门调小，避免用例真的等 500ms
	iv := 60 * time.Millisecond
	cfg.Collector.Topics = map[string]config.TopicConfig{
		"yahoo.chart": {MinInterval: &iv},
	}

	initPolicyGate(cfg, nil)

	fnCalls := 0
	fn := func() (int, error) { fnCalls++; return 1, nil }
	start := time.Now()
	for i := 0; i < 2; i++ {
		if _, err := policy.Fetch(policy.Default(), "yahoo.chart", "AAPL", fn); err != nil {
			t.Fatal(err)
		}
	}
	if fnCalls != 2 {
		t.Errorf("cache.enabled=false 时不得缓存: fn 调用 %d 次, want 2", fnCalls)
	}
	if elapsed := time.Since(start); elapsed < 40*time.Millisecond {
		t.Errorf("限流不应随缓存开关关闭, 两次耗时 %v", elapsed)
	}
}

func TestInitPolicyGateAppliesTopicQuotaOverride(t *testing.T) {
	restoreGate(t)
	cfg := config.Defaults()
	cfg.Collector.Quota.Path = filepath.Join(t.TempDir(), "quota.json")
	limit := 1
	iv := time.Duration(0)
	cfg.Collector.Topics = map[string]config.TopicConfig{
		"tushare.daily_basic": {QuotaLimit: &limit, MinInterval: &iv},
	}

	initPolicyGate(cfg, nil)

	fn := func() (int, error) { return 1, nil }
	if _, err := policy.Fetch(policy.Default(), "tushare.daily_basic", "k", fn); err != nil {
		t.Fatalf("首次应放行: %v", err)
	}
	if _, err := policy.Fetch(policy.Default(), "tushare.daily_basic", "k2", fn); err == nil {
		t.Error("配额上限被覆盖为 1，第二次应被拒")
	}
}

func TestInitPolicyGateUsesConfiguredQuotaPath(t *testing.T) {
	restoreGate(t)
	dir := t.TempDir()
	cfg := config.Defaults()
	cfg.Collector.Quota.Path = filepath.Join(dir, "nested", "quota.json")
	iv := time.Duration(0)
	cfg.Collector.Topics = map[string]config.TopicConfig{
		"tushare.daily_basic": {MinInterval: &iv},
	}

	initPolicyGate(cfg, nil)
	if _, err := policy.Fetch(policy.Default(), "tushare.daily_basic", "k", func() (int, error) { return 1, nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "nested", "quota.json")); err != nil {
		t.Errorf("账本未写到配置路径: %v", err)
	}
}

// TestInitPolicyGateNilLoggerAndUnwritablePath 覆盖 error_handling[0] 的两句：
// nil logger 不 panic（离线 CLI 路径就是这么调的），且账本路径不可写时**仍完成
// SetDefault** —— 由 FileStore 在 Take 时 fail-open，而不是让装配阶段就崩掉。
func TestInitPolicyGateNilLoggerAndUnwritablePath(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("initPolicyGate 不得 panic: %v", r)
		}
	}()

	restoreGate(t)
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.Defaults()
	// 父路径是文件 → 账本目录建不出来
	cfg.Collector.Quota.Path = filepath.Join(blocker, "sub", "quota.json")
	iv := time.Duration(0)
	cfg.Collector.Topics = map[string]config.TopicConfig{
		"tushare.daily_basic": {MinInterval: &iv},
	}

	initPolicyGate(cfg, nil) // nil logger

	g := policy.Default()
	if g == nil {
		t.Fatal("账本不可写时仍应完成 SetDefault")
	}
	// FileStore 在 Take 时 fail-open：请求照常放行，不阻断降级链
	fnCalls := 0
	if _, err := policy.Fetch(g, "tushare.daily_basic", "k", func() (int, error) {
		fnCalls++
		return 1, nil
	}); err != nil {
		t.Errorf("账本不可写时应 fail-open 放行, got %v", err)
	}
	if fnCalls != 1 {
		t.Errorf("fn 调用 %d 次, want 1", fnCalls)
	}
}

// TestLoadConfigOrDefaultsInitsPolicyGate 覆盖 non_functional[0]（最高风险项 A1）。
//
// **它是唯一能证明 prism refresh 路径真的装上了 Gate 的测试。** 其余四条 functional
// 测试都直接调 initPolicyGate，把它挪到别处（比如只放在 export_ohlcv 的调用点）
// 那四条照样全绿，而 prism refresh 会拿到懒构造的无账本 Gate、配额彻底失效。
//
// 判据选「账本文件是否出现在配置路径」而非「Gate 非 nil」：Default() 永不返回 nil
// （懒构造），非 nil 证明不了任何事。账本落盘才能区分「装配过的 Gate」与「懒构造的 Gate」。
func TestLoadConfigOrDefaultsInitsPolicyGate(t *testing.T) {
	dir := t.TempDir()
	quotaPath := filepath.Join(dir, "from-config-quota.json")
	cfgPath := filepath.Join(dir, "config.yaml")
	yaml := "collector:\n" +
		"  quota:\n" +
		"    path: " + quotaPath + "\n" +
		"  topics:\n" +
		"    tushare.daily_basic:\n" +
		"      min_interval: 0\n"
	if err := os.WriteFile(cfgPath, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	origFile := cfgFile
	origGate := policy.Default()
	t.Cleanup(func() { cfgFile = origFile; policy.SetDefault(origGate) })
	cfgFile = cfgPath

	// 清空单例：确保下面观测到的账本路径只可能来自本次 loadConfigOrDefaults。
	policy.SetDefault(nil)

	cfg, err := loadConfigOrDefaults()
	if err != nil {
		t.Fatalf("loadConfigOrDefaults: %v", err)
	}
	if cfg.Collector.Quota.Path != quotaPath {
		t.Fatalf("配置未读到 quota.path: got %q, want %q", cfg.Collector.Quota.Path, quotaPath)
	}

	if _, err := policy.Fetch(policy.Default(), "tushare.daily_basic", "k", func() (int, error) {
		return 1, nil
	}); err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if _, err := os.Stat(quotaPath); err != nil {
		t.Errorf("loadConfigOrDefaults 没有在内部初始化 Gate —— "+
			"prism refresh 走的正是这条入口，它拿到的会是懒构造的无账本 Gate，配额彻底失效: %v", err)
	}
}
