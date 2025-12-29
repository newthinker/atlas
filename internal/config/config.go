package config

import (
	"fmt"
	"os"
	"strings"

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
}

type ServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
	Mode string `mapstructure:"mode"`
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
	Enabled  bool     `mapstructure:"enabled"`
	Markets  []string `mapstructure:"markets"`
	Interval string   `mapstructure:"interval"`
	APIKey   string   `mapstructure:"api_key"`
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
			Host: "0.0.0.0",
			Port: 8080,
			Mode: "release",
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
	}
}
