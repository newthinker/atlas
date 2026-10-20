package pricewatch

// Context Checkpoint: done_criteria → test mapping (TASK-002)
// functional[0] spec §3 示例逐项读入 + 非默认值 fixture         → TestLoadConfig_Valid, TestLoadConfig_NonDefaults
// functional[1] 规则合并（替换/追加/防别名/notify 整条替换）、symbol 大写 → TestLoadConfig_Merge, TestLoadConfig_SymbolNormalized
// functional[2] stock 项不影响 CryptoAssets                      → TestUnsupportedItems
// functional[3] configs/pricewatch.yaml 与 testdata 一致          → TestExampleConfigLoads
// boundary[0]   ai.timeout/fail_alert_after 默认值、heartbeat 空、strategy 大小写 → TestLoadConfig_Defaults
// error_handling[0] 计划 13 行 + sources/quote/资产级 check_interval → TestLoadConfig_Rejects
// error_handling[1] ${ENV} 严格匹配、明文不进错误文本、proxy 原样   → TestLoadConfig_TelegramEnv

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func setTelegramEnv(t *testing.T) {
	t.Helper()
	t.Setenv("PRICEWATCH_TELEGRAM_BOT_TOKEN", "tok123")
	t.Setenv("PRICEWATCH_TELEGRAM_CHAT_ID", "42")
}

func validYAML(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/valid.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// mutate 把 src 中恰好出现一次的 old 换成 new；出现次数不为 1 时直接失败，防止改坏一处的用例静默变成无改动。
func mutate(t *testing.T, src, old, new string) string {
	t.Helper()
	if n := strings.Count(src, old); n != 1 {
		t.Fatalf("pattern %q occurs %d times, want 1", old, n)
	}
	return strings.Replace(src, old, new, 1)
}

func loadYAML(t *testing.T, content string) (*Config, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pricewatch.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return LoadConfig(path)
}

func mustLoad(t *testing.T, content string) *Config {
	t.Helper()
	c, err := loadYAML(t, content)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	return c
}

func assetBySymbol(t *testing.T, c *Config, sym string) Asset {
	t.Helper()
	for _, a := range c.CryptoAssets() {
		if a.Symbol == sym {
			return a
		}
	}
	t.Fatalf("asset %s not found", sym)
	return Asset{}
}

func ruleByID(t *testing.T, a Asset, id string) Rule {
	t.Helper()
	for _, r := range a.Rules {
		if r.ID == id {
			return r
		}
	}
	t.Fatalf("%s: rule %s not found", a.Symbol, id)
	return Rule{}
}

func ruleIDs(a Asset) []string {
	ids := make([]string, len(a.Rules))
	for i, r := range a.Rules {
		ids[i] = r.ID
	}
	return ids
}

func TestLoadConfig_Valid(t *testing.T) {
	setTelegramEnv(t)
	c := mustLoad(t, validYAML(t))

	btc := assetBySymbol(t, c, "BTC")
	if len(btc.Rules) != 6 {
		t.Errorf("BTC rules = %d, want 6", len(btc.Rules))
	}
	if btc.Interval != 5*time.Minute {
		t.Errorf("BTC Interval = %v, want 5m", btc.Interval)
	}
	if btc.Pair != "BTCUSDT" || btc.Quote != "USDT" || btc.Name != "比特币" {
		t.Errorf("BTC = %+v", btc)
	}
	if got := ruleByID(t, btc, "ath_drawdown").Notify; got != (NotifyPolicy{24 * time.Hour, 2}) {
		t.Errorf("ath_drawdown.Notify = %+v", got)
	}
	if got := ruleByID(t, btc, "week_drop").Notify; got != (NotifyPolicy{time.Hour, 3}) {
		t.Errorf("week_drop.Notify = %+v", got)
	}

	eth := assetBySymbol(t, c, "ETH")
	if got := ruleByID(t, eth, "day_drop").ThresholdPct; got != 3 {
		t.Errorf("ETH day_drop threshold = %v, want 3", got)
	}
	if eth.Interval != 10*time.Minute {
		t.Errorf("ETH Interval = %v, want 10m", eth.Interval)
	}

	if c.Health.HeartbeatAt != "09:00" {
		t.Errorf("HeartbeatAt = %q", c.Health.HeartbeatAt)
	}
	if c.Telegram.BotToken != "tok123" || c.Telegram.ChatID != "42" || c.Telegram.Proxy != "" {
		t.Errorf("Telegram = %+v", c.Telegram)
	}
	if c.CheckInterval != 5*time.Minute || c.Storage.Path != "data/pricewatch.db" || !c.AI.Enabled {
		t.Errorf("Config = %+v", c)
	}
	if c.NotifyDefaults != (NotifyPolicy{time.Hour, 3}) {
		t.Errorf("NotifyDefaults = %+v", c.NotifyDefaults)
	}
	if !reflect.DeepEqual(c.Assets.Crypto.Sources, []string{"okx", "binance"}) {
		t.Errorf("Sources = %v", c.Assets.Crypto.Sources)
	}
}

func TestLoadConfig_NonDefaults(t *testing.T) {
	setTelegramEnv(t)
	src := validYAML(t)
	src = mutate(t, src, "timeout: 60s", "timeout: 20s")
	src = mutate(t, src, "fail_alert_after: 30m", "fail_alert_after: 45m")
	src = mutate(t, src, `heartbeat_at: "09:00"`, `heartbeat_at: "08:30"`)
	c := mustLoad(t, src)
	if c.AI.Timeout != 20*time.Second {
		t.Errorf("AI.Timeout = %v, want 20s", c.AI.Timeout)
	}
	if c.Health.FailAlertAfter != 45*time.Minute {
		t.Errorf("FailAlertAfter = %v, want 45m", c.Health.FailAlertAfter)
	}
	if c.Health.HeartbeatAt != "08:30" {
		t.Errorf("HeartbeatAt = %q, want 08:30", c.Health.HeartbeatAt)
	}
}

func TestLoadConfig_Merge(t *testing.T) {
	setTelegramEnv(t)
	c := mustLoad(t, validYAML(t))
	want := []string{"ath_drawdown", "cycle_rise", "week_drop", "week_rise", "day_drop", "day_rise"}
	eth := assetBySymbol(t, c, "ETH")
	if got := ruleIDs(eth); !reflect.DeepEqual(got, want) {
		t.Errorf("ETH rule ids = %v, want %v", got, want)
	}
	if eth.Rules[4].ThresholdPct != 3 || eth.Rules[4].StepPct != 3 {
		t.Errorf("ETH day_drop not replaced in place: %+v", eth.Rules[4])
	}
	if got := ruleByID(t, assetBySymbol(t, c, "BTC"), "day_drop").ThresholdPct; got != 2 {
		t.Errorf("BTC day_drop threshold = %v, want 2 (strategy slice aliased)", got)
	}

	// BTC 排在 ETH 之前，解析时 ETH 的替换尚未发生；别名只会波及排在 ETH 之后的资产和 Strategies 本身
	c = mustLoad(t, mutate(t, validYAML(t), "  stock: {items: []}",
		"      - {symbol: SOL, name: 索拉纳, strategy: crypto_dual_invest}\n  stock: {items: []}"))
	if got := ruleByID(t, assetBySymbol(t, c, "SOL"), "day_drop").ThresholdPct; got != 2 {
		t.Errorf("SOL day_drop threshold = %v, want 2 (strategy slice aliased)", got)
	}
	if got := c.Strategies["crypto_dual_invest"][4]; got.ID != "day_drop" || got.ThresholdPct != 2 {
		t.Errorf("strategy day_drop mutated: %+v", got)
	}

	// 新 id 追加在末尾；覆盖规则未写 notify 时取 notify_defaults，而不是沿用 strategy 那条的 notify
	src := mutate(t, validYAML(t),
		"          - {id: day_drop, type: drop_from_high, window: 24h, threshold_pct: 3, step_pct: 3}",
		"          - {id: day_drop, type: drop_from_high, window: 24h, threshold_pct: 3, step_pct: 3}\n"+
			"          - {id: ath_drawdown, type: drop_from_high, window: all, threshold_pct: 60, step_pct: 5}\n"+
			"          - {id: month_drop, type: drop_from_high, window: 30d, threshold_pct: 10, step_pct: 5}")
	eth = assetBySymbol(t, mustLoad(t, src), "ETH")
	if got := ruleIDs(eth); !reflect.DeepEqual(got, append(want, "month_drop")) {
		t.Errorf("ETH rule ids = %v", got)
	}
	ath := eth.Rules[0]
	if ath.ThresholdPct != 60 || ath.Notify != (NotifyPolicy{time.Hour, 3}) {
		t.Errorf("ETH ath_drawdown = %+v, want threshold 60 with notify_defaults", ath)
	}
}

func TestLoadConfig_SymbolNormalized(t *testing.T) {
	setTelegramEnv(t)
	src := mutate(t, validYAML(t), "{symbol: BTC,", "{symbol: btc,")
	src = mutate(t, src, "quote: USDT", "quote: usdt")
	btc := assetBySymbol(t, mustLoad(t, src), "BTC")
	if btc.Pair != "BTCUSDT" || btc.Quote != "USDT" {
		t.Errorf("Pair = %q, Quote = %q, want BTCUSDT/USDT", btc.Pair, btc.Quote)
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	setTelegramEnv(t)
	src := validYAML(t)
	src = mutate(t, src, "timeout: 60s", "# timeout unset")
	src = mutate(t, src, "fail_alert_after: 30m", "# fail_alert_after unset")
	src = mutate(t, src, `heartbeat_at: "09:00"`, `heartbeat_at: ""`)
	src = mutate(t, src, "  crypto_dual_invest:", "  Crypto_Dual_Invest:")
	src = mutate(t, src, "{symbol: BTC, name: 比特币, strategy: crypto_dual_invest}", "{symbol: BTC, name: 比特币, strategy: CRYPTO_dual_invest}")
	c := mustLoad(t, src)
	if c.AI.Timeout != 60*time.Second {
		t.Errorf("AI.Timeout = %v, want 60s", c.AI.Timeout)
	}
	if c.Health.FailAlertAfter != 30*time.Minute {
		t.Errorf("FailAlertAfter = %v, want 30m", c.Health.FailAlertAfter)
	}
	if c.Health.HeartbeatAt != "" {
		t.Errorf("HeartbeatAt = %q, want empty", c.Health.HeartbeatAt)
	}
	if n := len(assetBySymbol(t, c, "BTC").Rules); n != 6 {
		t.Errorf("BTC rules = %d, want 6 (strategy case mismatch)", n)
	}
}

func TestLoadConfig_Rejects(t *testing.T) {
	const ethDayDrop = "{id: day_drop, type: drop_from_high, window: 24h, threshold_pct: 3, step_pct: 3}"
	cases := []struct {
		name, old, new, want string
	}{
		{"duplicate rule id", "{id: day_rise,  type: rise_from_low", "{id: day_drop,  type: rise_from_low", "duplicate rule id"},
		{"type foo", "{id: day_rise,  type: rise_from_low", "{id: day_rise,  type: foo", "type"},
		{"window 2d", "{id: week_drop, type: drop_from_high, window: 7d", "{id: week_drop, type: drop_from_high, window: 2d", "window"},
		{"threshold 0", ethDayDrop, strings.Replace(ethDayDrop, "threshold_pct: 3", "threshold_pct: 0", 1), "threshold_pct"},
		{"step -1", ethDayDrop, strings.Replace(ethDayDrop, "step_pct: 3", "step_pct: -1", 1), "step_pct"},
		{"max_per_tier 0", "max_per_tier: 3 ", "max_per_tier: 0 ", "max_per_tier"},
		{"repeat_interval 0s", "repeat_interval: 1h ", "repeat_interval: 0s ", "repeat_interval"},
		{"asset without rules", "      - {symbol: BTC, name: 比特币, strategy: crypto_dual_invest}",
			"      - {symbol: BTC, name: 比特币, strategy: crypto_dual_invest}\n      - {symbol: SOL, name: 索拉纳}", "SOL"},
		{"unknown strategy", "{symbol: BTC, name: 比特币, strategy: crypto_dual_invest}", "{symbol: BTC, name: 比特币, strategy: nope}", "nope"},
		{"plaintext bot_token", `"${PRICEWATCH_TELEGRAM_BOT_TOKEN}"`, `"abc"`, "${ENV}"},
		{"env expands empty", `"${PRICEWATCH_TELEGRAM_BOT_TOKEN}"`, `"${PW_TOKEN}"`, "PW_TOKEN"},
		{"check_interval 0s", "check_interval: 5m ", "check_interval: 0s ", "check_interval"},
		{"heartbeat_at 25:00", `heartbeat_at: "09:00"`, `heartbeat_at: "25:00"`, "heartbeat_at"},
		{"sources empty", "sources: [okx, binance]", "sources: []", "sources"},
		{"quote empty", "quote: USDT", `quote: ""`, "quote"},
		{"asset check_interval negative", "check_interval: 10m", "check_interval: -1m", "check_interval"},
		{"duplicate id in asset rules", "          - " + ethDayDrop, "          - " + ethDayDrop + "\n          - " + ethDayDrop, "duplicate rule id"},
		{"empty symbol", "{symbol: BTC, name: 比特币,", "{symbol: \"\", name: 比特币,", "symbol"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			setTelegramEnv(t)
			t.Setenv("PW_TOKEN", "")
			_, err := loadYAML(t, mutate(t, validYAML(t), c.old, c.new))
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %q should contain %q", err, c.want)
			}
		})
	}
}

func TestLoadConfig_TelegramEnv(t *testing.T) {
	const tokenLine = `bot_token: "${PRICEWATCH_TELEGRAM_BOT_TOKEN}"`
	const chatLine = `chat_id: "${PRICEWATCH_TELEGRAM_CHAT_ID}"`
	rejects := []struct{ name, old, new string }{
		{"suffix", tokenLine, `bot_token: "${A}x"`},
		{"prefix", tokenLine, `bot_token: "x${A}"`},
		{"no braces", tokenLine, `bot_token: "$A"`},
		{"chat_id plaintext", chatLine, `chat_id: "12345"`},
		{"chat_id empty", chatLine, `chat_id: "${EMPTY}"`},
		{"token plaintext", tokenLine, `bot_token: "123:ABCdef"`},
	}
	for _, c := range rejects {
		t.Run(c.name, func(t *testing.T) {
			setTelegramEnv(t)
			t.Setenv("A", "val")
			t.Setenv("EMPTY", "")
			_, err := loadYAML(t, mutate(t, validYAML(t), c.old, c.new))
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if strings.Contains(err.Error(), "123:ABCdef") {
				t.Errorf("error leaks plaintext token: %q", err)
			}
		})
	}

	t.Run("proxy kept verbatim", func(t *testing.T) {
		setTelegramEnv(t)
		t.Setenv("X", "http://should-not-expand")
		c := mustLoad(t, mutate(t, validYAML(t), `proxy: ""`, `proxy: "${X}"`))
		if c.Telegram.Proxy != "${X}" {
			t.Errorf("Proxy = %q, want ${X}", c.Telegram.Proxy)
		}
	})
}

func TestUnsupportedItems(t *testing.T) {
	setTelegramEnv(t)
	base := mustLoad(t, validYAML(t))
	if got := base.UnsupportedItems(); len(got) != 0 {
		t.Errorf("base UnsupportedItems = %v, want empty", got)
	}
	c := mustLoad(t, mutate(t, validYAML(t), "stock: {items: []}", `stock: {items: [{symbol: "600519"}]}`))
	if got := c.UnsupportedItems(); !reflect.DeepEqual(got, []string{"stock:600519"}) {
		t.Errorf("UnsupportedItems = %v", got)
	}
	f := mustLoad(t, mutate(t, validYAML(t), "fund:  {items: []}", `fund:  {items: [{symbol: "110011"}]}`))
	if got := f.UnsupportedItems(); !reflect.DeepEqual(got, []string{"fund:110011"}) {
		t.Errorf("UnsupportedItems = %v", got)
	}
	if !reflect.DeepEqual(c.CryptoAssets(), base.CryptoAssets()) {
		t.Error("CryptoAssets changed by stock items")
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	if _, err := LoadConfig(filepath.Join(t.TempDir(), "nope.yaml")); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestExampleConfigLoads(t *testing.T) {
	setTelegramEnv(t)
	example, err := LoadConfig("../../configs/pricewatch.yaml")
	if err != nil {
		t.Fatalf("LoadConfig example: %v", err)
	}
	want := mustLoad(t, validYAML(t))
	for _, sym := range []string{"BTC", "ETH"} {
		if !reflect.DeepEqual(assetBySymbol(t, example, sym), assetBySymbol(t, want, sym)) {
			t.Errorf("%s differs between configs/pricewatch.yaml and testdata/valid.yaml", sym)
		}
	}
}
