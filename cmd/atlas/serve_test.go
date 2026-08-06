package main

// Context Checkpoint: done_criteria → test mapping (TASK-007)
// 【TASK-011】OHLCV 装饰器(CachedCollector/maybeCache)已删除，缓存改由 policy Gate 承担。
// 原 TestMaybeCache_* 四条随之移除；其守护的「扩展接口不被包装遮蔽」由 collectors_test.go 的
// TestBuildCollectorsRegistersUnwrappedCollectors 与 TestLixingerExtensionInterfacesSurviveAssembly
// 以**正向断言**承接（cache.enabled=false 的语义由 Table.DisableTTL 承接，见 TASK-006）。

import (
	"testing"

	"github.com/newthinker/atlas/internal/app"
	"github.com/newthinker/atlas/internal/collector/lixinger"
	"github.com/newthinker/atlas/internal/collector/yahoo"
	"github.com/newthinker/atlas/internal/config"
	"github.com/newthinker/atlas/internal/notifier/telegram"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// Context Checkpoint: done_criteria → test mapping (TASK-001)
// functional[0]   "telegram 字段齐 → 注册 1 (返回值 + GetStats 双断言)"          → TestRegisterConfiguredNotifiers_TelegramSuccess
// functional[1]   "email host+from+to 齐 → 注册 1"                              → TestRegisterConfiguredNotifiers_EmailSuccess
// functional[2]   "webhook url 齐(headers nil 也行) → 1; telegram+webhook → 2; 每成功一条含名 info" → TestRegisterConfiguredNotifiers_WebhookSuccess / _TelegramAndWebhook
// boundary[0]     "enabled=false(三类)、Notifiers nil/空 map → 0 不 panic"      → TestRegisterConfiguredNotifiers_DisabledSkipped / _NilOrEmpty
// boundary[1]     "必填逐字段缺失表驱动 warn 指明字段; 未知 key → warn unknown"   → TestRegisterConfiguredNotifiers_MissingRequiredFields / _UnknownType
// error_handling  "Register 返回 err(重名) warn+跳过; enabled 配置但注册 0 静默失效 warn" → TestRegisterConfiguredNotifiers_DuplicateRegister / _SilentFailureWarn
// non_functional  "不发起网络(仅构造+注册不调 Send); 既有用例零回归; 覆盖率≥80%"   → 全体用例只构造注册

// observedNotifierLogger returns a logger that captures emitted log entries for
// assertion (info + warn) without any network or stdout side effects.
func observedNotifierLogger() (*zap.Logger, *observer.ObservedLogs) {
	core, logs := observer.New(zapcore.InfoLevel)
	return zap.New(core), logs
}

// newNotifierTestApp builds an app whose notifier registry starts empty so
// registerConfiguredNotifiers' effect is observable via GetStats.
func newNotifierTestApp(notifiers map[string]config.NotifierConfig) (*config.Config, *app.App) {
	cfg := &config.Config{Notifiers: notifiers}
	return cfg, app.New(cfg, zap.NewNop())
}

func statsNotifierCount(t *testing.T, application *app.App) int {
	t.Helper()
	v, ok := application.GetStats()["notifiers"]
	if !ok {
		t.Fatal("GetStats() missing \"notifiers\" key")
	}
	n, ok := v.(int)
	if !ok {
		t.Fatalf("GetStats()[\"notifiers\"] = %T, want int", v)
	}
	return n
}

// functional[0]: telegram with bot_token+chat_id registers exactly one, asserted
// via BOTH the return value and app.GetStats()["notifiers"].
func TestRegisterConfiguredNotifiers_TelegramSuccess(t *testing.T) {
	cfg, application := newNotifierTestApp(map[string]config.NotifierConfig{
		"telegram": {Enabled: true, BotToken: "tok", ChatID: "chat"},
	})
	log, logs := observedNotifierLogger()

	got := registerConfiguredNotifiers(cfg, application, log)

	if len(got) != 1 {
		t.Errorf("return count = %d, want 1", len(got))
	}
	if n := statsNotifierCount(t, application); n != 1 {
		t.Errorf("GetStats notifiers = %d, want 1", n)
	}
	if logs.FilterField(zap.String("notifier", "telegram")).
		FilterMessage("registered notifier").Len() != 1 {
		t.Errorf("expected one info log naming telegram, got entries: %v", logs.All())
	}
}

// functional[1]: email with host+from+to registers exactly one (positive path
// must exist for every supported type, per reviewer note).
func TestRegisterConfiguredNotifiers_EmailSuccess(t *testing.T) {
	cfg, application := newNotifierTestApp(map[string]config.NotifierConfig{
		"email": {Enabled: true, Host: "smtp.example.com", Port: 587, From: "a@b.com", To: []string{"x@y.com"}},
	})
	log, _ := observedNotifierLogger()

	got := registerConfiguredNotifiers(cfg, application, log)

	if len(got) != 1 {
		t.Errorf("return count = %d, want 1", len(got))
	}
	if n := statsNotifierCount(t, application); n != 1 {
		t.Errorf("GetStats notifiers = %d, want 1", n)
	}
}

// functional[2]: webhook with url registers one even when headers is nil.
func TestRegisterConfiguredNotifiers_WebhookSuccess(t *testing.T) {
	cfg, application := newNotifierTestApp(map[string]config.NotifierConfig{
		"webhook": {Enabled: true, URL: "http://localhost/hook", Headers: nil},
	})
	log, _ := observedNotifierLogger()

	got := registerConfiguredNotifiers(cfg, application, log)

	if len(got) != 1 {
		t.Errorf("return count = %d, want 1", len(got))
	}
	if n := statsNotifierCount(t, application); n != 1 {
		t.Errorf("GetStats notifiers = %d, want 1", n)
	}
}

// functional[2]: telegram + webhook both enabled & complete → 2, each emitting a
// named info log.
func TestRegisterConfiguredNotifiers_TelegramAndWebhook(t *testing.T) {
	cfg, application := newNotifierTestApp(map[string]config.NotifierConfig{
		"telegram": {Enabled: true, BotToken: "tok", ChatID: "chat"},
		"webhook":  {Enabled: true, URL: "http://localhost/hook"},
	})
	log, logs := observedNotifierLogger()

	got := registerConfiguredNotifiers(cfg, application, log)

	if len(got) != 2 {
		t.Errorf("return count = %d, want 2", len(got))
	}
	if n := statsNotifierCount(t, application); n != 2 {
		t.Errorf("GetStats notifiers = %d, want 2", n)
	}
	for _, name := range []string{"telegram", "webhook"} {
		if logs.FilterField(zap.String("notifier", name)).
			FilterMessage("registered notifier").Len() != 1 {
			t.Errorf("expected one info log naming %s, got: %v", name, logs.All())
		}
	}
	// total-count info must be emitted.
	if logs.FilterMessage("configured notifiers registered").Len() != 1 {
		t.Errorf("expected one total-count info log, got: %v", logs.All())
	}
}

// boundary[0]: all three types with enabled=false register nothing, no panic.
func TestRegisterConfiguredNotifiers_DisabledSkipped(t *testing.T) {
	cfg, application := newNotifierTestApp(map[string]config.NotifierConfig{
		"telegram": {Enabled: false, BotToken: "tok", ChatID: "chat"},
		"email":    {Enabled: false, Host: "h", From: "a@b.com", To: []string{"x@y.com"}},
		"webhook":  {Enabled: false, URL: "http://localhost/hook"},
	})
	log, logs := observedNotifierLogger()

	got := registerConfiguredNotifiers(cfg, application, log)

	if len(got) != 0 {
		t.Errorf("return count = %d, want 0", len(got))
	}
	if n := statsNotifierCount(t, application); n != 0 {
		t.Errorf("GetStats notifiers = %d, want 0", n)
	}
	// No enabled config → no silent-failure warn.
	if logs.FilterMessageSnippet("signals will not be delivered").Len() != 0 {
		t.Errorf("disabled-only config must not emit silent-failure warn")
	}
}

// boundary[0]: nil and empty notifier maps register nothing, no panic.
func TestRegisterConfiguredNotifiers_NilOrEmpty(t *testing.T) {
	for _, tc := range []struct {
		name      string
		notifiers map[string]config.NotifierConfig
	}{
		{"nil map", nil},
		{"empty map", map[string]config.NotifierConfig{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, application := newNotifierTestApp(tc.notifiers)
			log, logs := observedNotifierLogger()

			got := registerConfiguredNotifiers(cfg, application, log)

			if len(got) != 0 {
				t.Errorf("return count = %d, want 0", len(got))
			}
			if n := statsNotifierCount(t, application); n != 0 {
				t.Errorf("GetStats notifiers = %d, want 0", n)
			}
			if logs.FilterMessageSnippet("signals will not be delivered").Len() != 0 {
				t.Errorf("no enabled config must not emit silent-failure warn")
			}
		})
	}
}

// boundary[1]: per-field missing matrix → 0 registered, warn naming the field.
func TestRegisterConfiguredNotifiers_MissingRequiredFields(t *testing.T) {
	for _, tc := range []struct {
		name      string
		key       string
		cfg       config.NotifierConfig
		wantField string
	}{
		{"telegram missing bot_token", "telegram", config.NotifierConfig{Enabled: true, ChatID: "chat"}, "bot_token"},
		{"telegram missing chat_id", "telegram", config.NotifierConfig{Enabled: true, BotToken: "tok"}, "chat_id"},
		{"email missing host", "email", config.NotifierConfig{Enabled: true, From: "a@b.com", To: []string{"x@y.com"}}, "host"},
		{"email missing from", "email", config.NotifierConfig{Enabled: true, Host: "h", To: []string{"x@y.com"}}, "from"},
		{"email missing to", "email", config.NotifierConfig{Enabled: true, Host: "h", From: "a@b.com"}, "to"},
		{"webhook missing url", "webhook", config.NotifierConfig{Enabled: true}, "url"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, application := newNotifierTestApp(map[string]config.NotifierConfig{tc.key: tc.cfg})
			log, logs := observedNotifierLogger()

			got := registerConfiguredNotifiers(cfg, application, log)

			if len(got) != 0 {
				t.Errorf("return count = %d, want 0", len(got))
			}
			if n := statsNotifierCount(t, application); n != 0 {
				t.Errorf("GetStats notifiers = %d, want 0", n)
			}
			if logs.FilterField(zap.String("field", tc.wantField)).
				FilterLevelExact(zapcore.WarnLevel).Len() != 1 {
				t.Errorf("expected one warn naming missing field %q, got: %v", tc.wantField, logs.All())
			}
		})
	}
}

// boundary[1]: an unknown notifier key (enabled) → 0, warn "unknown notifier type".
func TestRegisterConfiguredNotifiers_UnknownType(t *testing.T) {
	cfg, application := newNotifierTestApp(map[string]config.NotifierConfig{
		"slack": {Enabled: true},
	})
	log, logs := observedNotifierLogger()

	got := registerConfiguredNotifiers(cfg, application, log)

	if len(got) != 0 {
		t.Errorf("return count = %d, want 0", len(got))
	}
	if logs.FilterMessage("unknown notifier type").
		FilterField(zap.String("notifier", "slack")).Len() != 1 {
		t.Errorf("expected one warn for unknown type slack, got: %v", logs.All())
	}
}

// error_handling: RegisterNotifier returns err (duplicate name) → warn + skip,
// not blocking; the pre-registered notifier survives.
func TestRegisterConfiguredNotifiers_DuplicateRegister(t *testing.T) {
	cfg, application := newNotifierTestApp(map[string]config.NotifierConfig{
		"telegram": {Enabled: true, BotToken: "tok", ChatID: "chat"},
	})
	// Pre-register a telegram so the config-driven one collides on Register.
	if err := application.RegisterNotifier(telegram.New("pre", "pre")); err != nil {
		t.Fatalf("pre-register failed: %v", err)
	}
	log, logs := observedNotifierLogger()

	got := registerConfiguredNotifiers(cfg, application, log)

	if len(got) != 0 {
		t.Errorf("return count = %d, want 0 (duplicate skipped)", len(got))
	}
	// The pre-registered notifier must still be the only one present.
	if n := statsNotifierCount(t, application); n != 1 {
		t.Errorf("GetStats notifiers = %d, want 1 (pre-registered survives)", n)
	}
	if logs.FilterMessage("failed to register notifier").
		FilterLevelExact(zapcore.WarnLevel).Len() != 1 {
		t.Errorf("expected one warn on duplicate register, got: %v", logs.All())
	}
}

// error_handling: an enabled config that ends up registering nothing emits the
// silent-failure warn (the exact bug this task fixes).
func TestRegisterConfiguredNotifiers_SilentFailureWarn(t *testing.T) {
	cfg, application := newNotifierTestApp(map[string]config.NotifierConfig{
		"telegram": {Enabled: true, BotToken: "tok"}, // missing chat_id → 0 registered
	})
	log, logs := observedNotifierLogger()

	got := registerConfiguredNotifiers(cfg, application, log)

	if len(got) != 0 {
		t.Errorf("return count = %d, want 0", len(got))
	}
	if logs.FilterMessageSnippet("signals will not be delivered").
		FilterLevelExact(zapcore.WarnLevel).Len() != 1 {
		t.Errorf("expected one silent-failure warn, got: %v", logs.All())
	}
}

// TASK-012 done_criteria → test mapping
// functional[0]/boundary[0] "估值源注入 typed-nil 防护：nil 指针不得变非 nil 接口"
//   → TestValuationSourceOrNil_NilStaysNil / TestEPSSourceOrNil_NilStaysNil
//   + TestValuationSourceOrNil_RealNonNil / TestEPSSourceOrNil_RealNonNil

// functional[0] + boundary[0]: a nil *lixinger.Lixinger must NOT become a
// non-nil app.ValuationSource, or buildFundamental's `valuationSrc != nil`
// guard would be defeated (typed-nil interface trap) and panic at call time.
func TestValuationSourceOrNil_NilStaysNil(t *testing.T) {
	var nilCollector *lixinger.Lixinger
	got := valuationSourceOrNil(nilCollector)
	if got != nil {
		t.Errorf("valuationSourceOrNil(nil *Lixinger) = %v (typed-nil leak), want untyped nil interface", got)
	}
}

func TestEPSSourceOrNil_NilStaysNil(t *testing.T) {
	var nilCollector *yahoo.Yahoo
	got := epsSourceOrNil(nilCollector)
	if got != nil {
		t.Errorf("epsSourceOrNil(nil *Yahoo) = %v (typed-nil leak), want untyped nil interface", got)
	}
}

// functional[0]: a real collector must be injected as a non-nil interface.
func TestValuationSourceOrNil_RealNonNil(t *testing.T) {
	var _ app.ValuationSource = lixinger.New("dummy-key") // compile-time impl check
	if got := valuationSourceOrNil(lixinger.New("dummy-key")); got == nil {
		t.Error("valuationSourceOrNil(real *Lixinger) = nil, want non-nil source")
	}
}

func TestEPSSourceOrNil_RealNonNil(t *testing.T) {
	var _ app.EPSSource = yahoo.New() // compile-time impl check
	if got := epsSourceOrNil(yahoo.New()); got == nil {
		t.Error("epsSourceOrNil(real *Yahoo) = nil, want non-nil source")
	}
}
