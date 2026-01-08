package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/newthinker/atlas/internal/core"
	"github.com/spf13/viper"
)

type Config struct {
	Server     ServerConfig              `mapstructure:"server"`
	Storage    StorageConfig             `mapstructure:"storage"`
	Collectors map[string]CollectorConfig `mapstructure:"collectors"`
	Strategies map[string]StrategyConfig  `mapstructure:"strategies"`
	Notifiers  map[string]NotifierConfig  `mapstructure:"notifiers"`
	Router     RouterConfig               `mapstructure:"router"`
	Watchlist  []WatchlistItem            `mapstructure:"watchlist"`
	LLM        LLMConfig                  `mapstructure:"llm"`
	Broker     BrokerConfig               `mapstructure:"broker"`
	Meta       MetaConfig                 `mapstructure:"meta"`
	Metrics    MetricsConfig              `mapstructure:"metrics"`
	Alerts     AlertsConfig               `mapstructure:"alerts"`
}

type ServerConfig struct {
	Host        string `mapstructure:"host"`
	Port        int    `mapstructure:"port"`
	Mode        string `mapstructure:"mode"`
	APIKey      string `mapstructure:"api_key"`
	JobTTLHours int    `mapstructure:"job_ttl_hours"`
	MaxJobs     int    `mapstructure:"max_jobs"`
}

type StorageConfig struct {
	Hot  HotStorageConfig  `mapstructure:"hot"`
	Cold ColdStorageConfig `mapstructure:"cold"`
}

type HotStorageConfig struct {
	DSN           string `mapstructure:"dsn"`
	RetentionDays int    `mapstructure:"retention_days"`
}

type ColdStorageConfig struct {
	Type string   `mapstructure:"type"` // "localfs" or "s3"
	Path string   `mapstructure:"path"` // For localfs
	S3   S3Config `mapstructure:"s3"`   // For S3
}

type S3Config struct {
	Bucket    string `mapstructure:"bucket"`
	Endpoint  string `mapstructure:"endpoint"`
	Region    string `mapstructure:"region"`
	AccessKey string `mapstructure:"access_key"`
	SecretKey string `mapstructure:"secret_key"`
	Prefix    string `mapstructure:"prefix"`
}

type CollectorConfig struct {
	Enabled  bool           `mapstructure:"enabled"`
	Markets  []string       `mapstructure:"markets"`
	Interval string         `mapstructure:"interval"`
	APIKey   string         `mapstructure:"api_key"`
	Extra    map[string]any `mapstructure:",remain"` // Flexible options for specific collectors
}

type StrategyConfig struct {
	Enabled bool           `mapstructure:"enabled"`
	Params  map[string]any `mapstructure:"params"`
}

type NotifierConfig struct {
	Enabled  bool              `mapstructure:"enabled"`
	BotToken string            `mapstructure:"bot_token"`
	ChatID   string            `mapstructure:"chat_id"`
	URL      string            `mapstructure:"url"`
	// Email notifier fields
	Host     string            `mapstructure:"host"`
	Port     int               `mapstructure:"port"`
	Username string            `mapstructure:"username"`
	Password string            `mapstructure:"password"`
	From     string            `mapstructure:"from"`
	To       []string          `mapstructure:"to"`
	// Webhook notifier fields
	Headers  map[string]string `mapstructure:"headers"`
}

type RouterConfig struct {
	CooldownHours int     `mapstructure:"cooldown_hours"`
	MinConfidence float64 `mapstructure:"min_confidence"`
}

type WatchlistItem struct {
	Symbol     string   `mapstructure:"symbol"`
	Name       string   `mapstructure:"name"`
	Market     string   `mapstructure:"market"` // "A股", "H股", "美股", "数字货币"
	Type       string   `mapstructure:"type"`   // "股票", "基金", "债券", "ETF", "期权", "期货", "加密货币"
	Strategies []string `mapstructure:"strategies"`
}

type LLMConfig struct {
	Provider string       `mapstructure:"provider"`
	Claude   ClaudeConfig `mapstructure:"claude"`
	OpenAI   OpenAIConfig `mapstructure:"openai"`
	Ollama   OllamaConfig `mapstructure:"ollama"`
}

type ClaudeConfig struct {
	APIKey string `mapstructure:"api_key"`
	Model  string `mapstructure:"model"`
}

type OpenAIConfig struct {
	APIKey string `mapstructure:"api_key"`
	Model  string `mapstructure:"model"`
}

type OllamaConfig struct {
	Endpoint string `mapstructure:"endpoint"`
	Model    string `mapstructure:"model"`
}

// BrokerConfig holds broker integration settings.
type BrokerConfig struct {
	Enabled   bool                  `mapstructure:"enabled"`
	Provider  string                `mapstructure:"provider"`
	Mode      string                `mapstructure:"mode"` // paper, live
	Execution ExecutionConfigOpts   `mapstructure:"execution"`
	Risk      RiskConfigOpts        `mapstructure:"risk"`
	Futu      FutuConfig            `mapstructure:"futu"`
}

// ExecutionConfigOpts holds execution settings for the broker.
type ExecutionConfigOpts struct {
	Mode           string  `mapstructure:"mode"`             // auto, confirm, batch
	BatchTime      string  `mapstructure:"batch_time"`       // HH:MM for batch execution
	DefaultSizePct float64 `mapstructure:"default_size_pct"` // Position size as % of portfolio
}

// RiskConfigOpts holds risk control settings.
type RiskConfigOpts struct {
	MaxPositionPct   float64 `mapstructure:"max_position_pct"`
	MaxDailyLossPct  float64 `mapstructure:"max_daily_loss_pct"`
	MaxOpenPositions int     `mapstructure:"max_open_positions"`
}

// FutuConfig holds Futu broker settings.
type FutuConfig struct {
	Host          string `mapstructure:"host"`
	Port          int    `mapstructure:"port"`
	Env           string `mapstructure:"env"` // "simulate" or "real"
	TradePassword string `mapstructure:"trade_password"`
	RSAKeyPath    string `mapstructure:"rsa_key_path"`
}

// MetaConfig holds LLM meta-strategy settings.
type MetaConfig struct {
	Arbitrator  ArbitratorConfig  `mapstructure:"arbitrator"`
	Synthesizer SynthesizerConfig `mapstructure:"synthesizer"`
}

// ArbitratorConfig holds signal arbitrator settings.
type ArbitratorConfig struct {
	Enabled      bool `mapstructure:"enabled"`
	ContextDays  int  `mapstructure:"context_days"`
}

// SynthesizerConfig holds strategy synthesizer settings.
type SynthesizerConfig struct {
	Enabled   bool   `mapstructure:"enabled"`
	Schedule  string `mapstructure:"schedule"`
	MinTrades int    `mapstructure:"min_trades"`
}

// MetricsConfig holds metrics configuration.
type MetricsConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Path    string `mapstructure:"path"`
}

// AlertsConfig holds alerts configuration.
type AlertsConfig struct {
	Enabled       bool          `mapstructure:"enabled"`
	CheckInterval time.Duration `mapstructure:"check_interval"`
	Rules         []AlertRule   `mapstructure:"rules"`
}

// AlertRule defines a single alert rule.
type AlertRule struct {
	Name     string        `mapstructure:"name"`
	Expr     string        `mapstructure:"expr"`
	For      time.Duration `mapstructure:"for"`
	Severity string        `mapstructure:"severity"`
	Message  string        `mapstructure:"message"`
}

// Load reads configuration from file
func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)

	// Support environment variable overrides
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	// Expand environment variables in string values
	for _, key := range v.AllKeys() {
		val := v.GetString(key)
		if strings.HasPrefix(val, "${") && strings.HasSuffix(val, "}") {
			envKey := strings.TrimSuffix(strings.TrimPrefix(val, "${"), "}")
			v.Set(key, os.Getenv(envKey))
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	return &cfg, nil
}

// Defaults returns a config with sensible defaults
func Defaults() *Config {
	return &Config{
		Server: ServerConfig{
			Host:        "0.0.0.0",
			Port:        8080,
			Mode:        "release",
			JobTTLHours: 1,
			MaxJobs:     100,
		},
		Storage: StorageConfig{
			Hot: HotStorageConfig{
				RetentionDays: 90,
			},
			Cold: ColdStorageConfig{
				Type: "localfs",
			},
		},
		Router: RouterConfig{
			CooldownHours: 4,
			MinConfidence: 0.6,
		},
		Metrics: MetricsConfig{
			Enabled: true,
			Path:    "/metrics",
		},
		Alerts: AlertsConfig{
			Enabled:       false,
			CheckInterval: 60 * time.Second,
		},
		Broker: BrokerConfig{
			Enabled:  false,
			Provider: "futu",
			Mode:     "paper",
			Execution: ExecutionConfigOpts{
				Mode:           "confirm",
				DefaultSizePct: 2.0,
			},
			Risk: RiskConfigOpts{
				MaxPositionPct:   10.0,
				MaxDailyLossPct:  5.0,
				MaxOpenPositions: 20,
			},
		},
	}
}

// Validate checks the configuration for errors.
func (c *Config) Validate() error {
	// Server validation
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return core.WrapError(core.ErrConfigInvalid,
			fmt.Errorf("port must be between 1 and 65535, got %d", c.Server.Port))
	}

	// Router validation
	if c.Router.MinConfidence < 0 || c.Router.MinConfidence > 1 {
		return core.WrapError(core.ErrConfigInvalid,
			fmt.Errorf("min_confidence must be between 0 and 1, got %f", c.Router.MinConfidence))
	}
	if c.Router.CooldownHours < 0 {
		return core.WrapError(core.ErrConfigInvalid,
			fmt.Errorf("cooldown_hours cannot be negative, got %d", c.Router.CooldownHours))
	}

	// LLM validation - if provider set, check config exists
	if c.LLM.Provider != "" {
		switch c.LLM.Provider {
		case "claude":
			if c.LLM.Claude.APIKey == "" {
				return core.WrapError(core.ErrConfigMissing,
					fmt.Errorf("claude api_key required when provider is claude"))
			}
		case "openai":
			if c.LLM.OpenAI.APIKey == "" {
				return core.WrapError(core.ErrConfigMissing,
					fmt.Errorf("openai api_key required when provider is openai"))
			}
		case "ollama":
			if c.LLM.Ollama.Endpoint == "" {
				return core.WrapError(core.ErrConfigMissing,
					fmt.Errorf("ollama endpoint required when provider is ollama"))
			}
		}
	}

	// Broker validation
	if c.Broker.Enabled {
		if c.Broker.Mode == "live" && c.Broker.Futu.Env != "real" {
			return core.WrapError(core.ErrConfigInvalid,
				fmt.Errorf("live mode requires futu env=real, got %s", c.Broker.Futu.Env))
		}
		switch c.Broker.Execution.Mode {
		case "auto", "confirm", "batch", "":
			// Valid
		default:
			return core.WrapError(core.ErrConfigInvalid,
				fmt.Errorf("invalid execution mode: %s", c.Broker.Execution.Mode))
		}
	}

	return nil
}

// WarnHardcodedSecrets logs warnings for secrets that appear to be hardcoded
// instead of using environment variable syntax (${ENV_VAR}).
func (c *Config) WarnHardcodedSecrets(logger func(string)) {
	secretFields := []struct {
		name  string
		value string
	}{
		{"server.api_key", c.Server.APIKey},
		{"storage.cold.s3.access_key", c.Storage.Cold.S3.AccessKey},
		{"storage.cold.s3.secret_key", c.Storage.Cold.S3.SecretKey},
		{"broker.futu.trade_password", c.Broker.Futu.TradePassword},
		{"llm.claude.api_key", c.LLM.Claude.APIKey},
		{"llm.openai.api_key", c.LLM.OpenAI.APIKey},
	}

	for _, f := range secretFields {
		if f.value != "" && !strings.HasPrefix(f.value, "${") {
			logger(fmt.Sprintf("WARNING: %s appears to be hardcoded (use ${ENV_VAR} syntax)", f.name))
		}
	}
}
