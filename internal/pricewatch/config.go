package pricewatch

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// AssetCfg 是配置文件中的一个资产条目。
type AssetCfg struct {
	Symbol, Name, Strategy string
	CheckInterval          time.Duration `mapstructure:"check_interval"`
	Rules                  []RuleCfg
}

// Config 是 pricewatch 的配置（spec §3）。Telegram 字段为 ${ENV} 展开后的值。
type Config struct {
	CheckInterval time.Duration `mapstructure:"check_interval"`
	Storage       struct{ Path string }
	Telegram      struct {
		BotToken string `mapstructure:"bot_token"`
		ChatID   string `mapstructure:"chat_id"`
		Proxy    string
	}
	AI struct {
		Enabled bool
		Timeout time.Duration
	}
	NotifyDefaults NotifyPolicy `mapstructure:"notify_defaults"`
	Health         struct {
		FailAlertAfter time.Duration `mapstructure:"fail_alert_after"`
		HeartbeatAt    string        `mapstructure:"heartbeat_at"`
	}
	Strategies map[string][]RuleCfg
	Assets     struct {
		Crypto struct {
			Sources []string
			Quote   string
			Items   []AssetCfg
		}
		Stock, Fund struct{ Items []AssetCfg }
	}

	cryptoAssets []Asset
}

// Asset 是合并、展开后可直接使用的资产。
type Asset struct {
	Symbol   string // 大写，如 "BTC"
	Name     string
	Pair     string // Symbol+Quote，如 "BTCUSDT"（crypto.Provider 的入参）
	Quote    string
	Interval time.Duration // 资产自己的 check_interval，没写则取全局
	Rules    []Rule
}

// LoadConfig 读取并校验 pricewatch 配置。
func LoadConfig(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetDefault("ai.timeout", "60s")
	v.SetDefault("health.fail_alert_after", "30m")
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("reading pricewatch config: %w", err)
	}
	var cfg Config
	// 严格解析：拼错的键（如 heartbeat_al）会静默落空，心跳就被悄悄关掉了
	if err := v.UnmarshalExact(&cfg); err != nil {
		return nil, fmt.Errorf("parsing pricewatch config: %w", err)
	}
	if err := cfg.build(); err != nil {
		return nil, fmt.Errorf("invalid pricewatch config %s: %w", path, err)
	}
	return &cfg, nil
}

// CryptoAssets 返回合并、校验后的加密货币资产。
func (c *Config) CryptoAssets() []Asset { return c.cryptoAssets }

// UnsupportedItems 返回 stock/fund 下非空项的 "stock:XXX" 列表，供调用方打 warn。
func (c *Config) UnsupportedItems() []string {
	var out []string
	for _, it := range c.Assets.Stock.Items {
		out = append(out, "stock:"+it.Symbol)
	}
	for _, it := range c.Assets.Fund.Items {
		out = append(out, "fund:"+it.Symbol)
	}
	return out
}

// build 校验全局字段、展开 Telegram 的 ${ENV}，并生成 cryptoAssets。
func (c *Config) build() error {
	if c.CheckInterval <= 0 {
		return fmt.Errorf("check_interval must be > 0, got %v", c.CheckInterval)
	}
	if c.Health.FailAlertAfter <= 0 {
		return fmt.Errorf("health.fail_alert_after must be > 0, got %v", c.Health.FailAlertAfter)
	}
	if c.AI.Timeout <= 0 {
		return fmt.Errorf("ai.timeout must be > 0, got %v", c.AI.Timeout)
	}
	if h := c.Health.HeartbeatAt; h != "" {
		if _, err := time.Parse("15:04", h); err != nil {
			return fmt.Errorf("health.heartbeat_at %q must be HH:MM", h)
		}
	}
	var err error
	if c.Telegram.BotToken, err = expandEnv("telegram.bot_token", c.Telegram.BotToken); err != nil {
		return err
	}
	if c.Telegram.ChatID, err = expandEnv("telegram.chat_id", c.Telegram.ChatID); err != nil {
		return err
	}

	crypto := c.Assets.Crypto
	if len(crypto.Items) > 0 {
		if len(crypto.Sources) == 0 {
			return fmt.Errorf("assets.crypto.sources must not be empty")
		}
		if crypto.Quote == "" {
			return fmt.Errorf("assets.crypto.quote must not be empty")
		}
	}
	quote := strings.ToUpper(crypto.Quote)
	for _, it := range crypto.Items {
		a, err := c.buildAsset(it, quote)
		if err != nil {
			return err
		}
		c.cryptoAssets = append(c.cryptoAssets, a)
	}
	return nil
}

func (c *Config) buildAsset(it AssetCfg, quote string) (Asset, error) {
	sym := strings.ToUpper(it.Symbol)
	if sym == "" {
		return Asset{}, fmt.Errorf("asset symbol must not be empty")
	}
	if it.CheckInterval < 0 {
		return Asset{}, fmt.Errorf("asset %s: check_interval must not be negative, got %v", sym, it.CheckInterval)
	}
	var base []RuleCfg
	if it.Strategy != "" {
		// viper 把 map 键小写化，引用时同样小写
		s, ok := c.Strategies[strings.ToLower(it.Strategy)]
		if !ok {
			return Asset{}, fmt.Errorf("asset %s: unknown strategy %q", sym, it.Strategy)
		}
		base = s
	}
	cfgs, err := mergeRules(base, it.Rules)
	if err != nil {
		return Asset{}, fmt.Errorf("asset %s: %w", sym, err)
	}
	if len(cfgs) == 0 {
		return Asset{}, fmt.Errorf("asset %s: no rules", sym)
	}
	a := Asset{Symbol: sym, Name: it.Name, Pair: sym + quote, Quote: quote, Interval: it.CheckInterval}
	if a.Interval == 0 {
		a.Interval = c.CheckInterval
	}
	for _, rc := range cfgs {
		r, err := ParseRule(rc, c.NotifyDefaults)
		if err != nil {
			return Asset{}, fmt.Errorf("asset %s: %w", sym, err)
		}
		a.Rules = append(a.Rules, r)
	}
	return a, nil
}

// mergeRules 先取 strategy 的规则，overrides 中同 id 的整条替换，新 id 追加。
// 结果是新切片，不与 base 共享底层数组。
func mergeRules(base, overrides []RuleCfg) ([]RuleCfg, error) {
	out := append([]RuleCfg(nil), base...)
	pos := make(map[string]int, len(out))
	for i, r := range out {
		if _, dup := pos[r.ID]; dup {
			return nil, fmt.Errorf("duplicate rule id %q", r.ID)
		}
		pos[r.ID] = i
	}
	seen := make(map[string]bool, len(overrides))
	for _, r := range overrides {
		if seen[r.ID] {
			return nil, fmt.Errorf("duplicate rule id %q", r.ID)
		}
		seen[r.ID] = true
		if i, ok := pos[r.ID]; ok {
			out[i] = r
		} else {
			pos[r.ID] = len(out)
			out = append(out, r)
		}
	}
	return out, nil
}

// placeholderPrefix 是仓库里 launchd plist 给 Telegram 变量放的占位值前缀（QA-14）。
const placeholderPrefix = "REPLACE_WITH_"

var envRefRE = regexp.MustCompile(`^\$\{([A-Za-z0-9_]+)\}$`)

// expandEnv 只接受整值为 ${NAME} 的写法；错误文本不回显原值，避免把明文密钥写进日志。
func expandEnv(field, raw string) (string, error) {
	m := envRefRE.FindStringSubmatch(raw)
	if m == nil {
		return "", fmt.Errorf("%s must be written as ${ENV}", field)
	}
	val := os.Getenv(m[1])
	if val == "" {
		return "", fmt.Errorf("%s: environment variable %s is empty", field, m[1])
	}
	if strings.HasPrefix(val, placeholderPrefix) {
		return "", fmt.Errorf("%s: environment variable %s still holds the %s… placeholder from the repo plist; "+
			"put the real value into the copy installed under ~/Library/LaunchAgents/ (see the plist comment)", field, m[1], placeholderPrefix)
	}
	return val, nil
}
