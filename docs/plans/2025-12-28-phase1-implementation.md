# ATLAS Phase 1 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a working trading signal system with Yahoo/Eastmoney data, MA crossover strategy, and Telegram notifications.

**Architecture:** Monolithic Go binary with plugin interfaces. Data flows from collectors → storage → strategy engine → signal router → notifiers. TimescaleDB for hot storage, local filesystem for cold archive.

**Tech Stack:** Go 1.21+, PostgreSQL/TimescaleDB, Cobra CLI, Gin HTTP, Zap logging, pgx driver

---

## Task 1: Project Initialization

**Files:**
- Create: `go.mod`
- Create: `cmd/atlas/main.go`
- Create: `Makefile`

**Step 1: Initialize Go module**

```bash
cd /Users/zuowei/workspace/go/src/github.com/newthinker/atlas/.worktrees/phase1
go mod init github.com/newthinker/atlas
```

**Step 2: Create minimal main.go**

```go
// cmd/atlas/main.go
package main

import "fmt"

func main() {
    fmt.Println("ATLAS - Asset Tracking & Leadership Analysis System")
}
```

**Step 3: Create Makefile**

```makefile
.PHONY: build run test clean

BINARY=atlas
BUILD_DIR=bin

build:
	go build -o $(BUILD_DIR)/$(BINARY) ./cmd/atlas

run: build
	./$(BUILD_DIR)/$(BINARY)

test:
	go test -v ./...

clean:
	rm -rf $(BUILD_DIR)
```

**Step 4: Verify build works**

Run: `make build && make run`
Expected: "ATLAS - Asset Tracking & Leadership Analysis System"

**Step 5: Commit**

```bash
git add go.mod cmd/ Makefile
git commit -m "feat: initialize Go project with basic structure"
```

---

## Task 2: Core Types

**Files:**
- Create: `internal/core/types.go`
- Create: `internal/core/types_test.go`

**Step 1: Write failing test for Quote type**

```go
// internal/core/types_test.go
package core

import (
    "testing"
    "time"
)

func TestQuote_IsValid(t *testing.T) {
    q := Quote{
        Symbol: "600519.SH",
        Market: MarketCNA,
        Price:  1680.50,
        Volume: 1000000,
        Time:   time.Now(),
    }

    if !q.IsValid() {
        t.Error("expected valid quote")
    }

    invalid := Quote{Symbol: "", Price: 0}
    if invalid.IsValid() {
        t.Error("expected invalid quote")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/core/... -v`
Expected: FAIL - package not found

**Step 3: Implement core types**

```go
// internal/core/types.go
package core

import "time"

// Market represents a trading market
type Market string

const (
    MarketUS  Market = "US"
    MarketHK  Market = "HK"
    MarketCNA Market = "CN_A"
    MarketEU  Market = "EU"
)

// AssetType represents the type of financial asset
type AssetType string

const (
    AssetStock     AssetType = "stock"
    AssetIndex     AssetType = "index"
    AssetETF       AssetType = "etf"
    AssetFund      AssetType = "fund"
    AssetCommodity AssetType = "commodity"
)

// Quote represents a real-time price quote
type Quote struct {
    Symbol    string
    Market    Market
    Price     float64
    Volume    int64
    Bid       float64
    Ask       float64
    Time      time.Time
    Source    string
}

// IsValid checks if the quote has required fields
func (q Quote) IsValid() bool {
    return q.Symbol != "" && q.Price > 0
}

// OHLCV represents a candlestick/bar
type OHLCV struct {
    Symbol   string
    Interval string // "1m", "5m", "1d"
    Open     float64
    High     float64
    Low      float64
    Close    float64
    Volume   int64
    Time     time.Time
}

// Action represents a trading signal action
type Action string

const (
    ActionBuy        Action = "buy"
    ActionSell       Action = "sell"
    ActionHold       Action = "hold"
    ActionStrongBuy  Action = "strong_buy"
    ActionStrongSell Action = "strong_sell"
)

// Signal represents a trading signal from a strategy
type Signal struct {
    Symbol      string
    Action      Action
    Confidence  float64
    Reason      string
    Strategy    string
    Metadata    map[string]any
    GeneratedAt time.Time
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/core/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/core/
git commit -m "feat: add core types (Quote, OHLCV, Signal, Market, Action)"
```

---

## Task 3: Configuration System

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`
- Create: `configs/config.example.yaml`

**Step 1: Add viper dependency**

```bash
go get github.com/spf13/viper
```

**Step 2: Write failing test for config loading**

```go
// internal/config/config_test.go
package config

import (
    "os"
    "path/filepath"
    "testing"
)

func TestLoad_FromFile(t *testing.T) {
    // Create temp config file
    content := []byte(`
server:
  host: "127.0.0.1"
  port: 8080

storage:
  hot:
    dsn: "postgres://localhost:5432/atlas"
  cold:
    type: localfs
    path: "/tmp/atlas/archive"
`)

    tmpDir := t.TempDir()
    cfgPath := filepath.Join(tmpDir, "config.yaml")
    if err := os.WriteFile(cfgPath, content, 0644); err != nil {
        t.Fatal(err)
    }

    cfg, err := Load(cfgPath)
    if err != nil {
        t.Fatalf("failed to load config: %v", err)
    }

    if cfg.Server.Port != 8080 {
        t.Errorf("expected port 8080, got %d", cfg.Server.Port)
    }

    if cfg.Storage.Cold.Type != "localfs" {
        t.Errorf("expected localfs, got %s", cfg.Storage.Cold.Type)
    }
}
```

**Step 3: Run test to verify it fails**

Run: `go test ./internal/config/... -v`
Expected: FAIL - package not found

**Step 4: Implement config**

```go
// internal/config/config.go
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
    Router     RouterConfig              `mapstructure:"router"`
    Watchlist  []WatchlistItem           `mapstructure:"watchlist"`
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
    Type string `mapstructure:"type"` // "localfs" or "s3"
    Path string `mapstructure:"path"` // For localfs
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
    Enabled  bool   `mapstructure:"enabled"`
    BotToken string `mapstructure:"bot_token"`
    ChatID   string `mapstructure:"chat_id"`
    URL      string `mapstructure:"url"`
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
```

**Step 5: Run test to verify it passes**

Run: `go test ./internal/config/... -v`
Expected: PASS

**Step 6: Create example config**

```yaml
# configs/config.example.yaml
server:
  host: "0.0.0.0"
  port: 8080
  mode: "release"

storage:
  hot:
    dsn: "postgres://localhost:5432/atlas"
    retention_days: 90
  cold:
    type: localfs
    path: "/mnt/nas/atlas/archive"

collectors:
  yahoo:
    enabled: true
    markets: ["US", "HK"]
    interval: "5m"
  eastmoney:
    enabled: true
    markets: ["CN_A"]
    interval: "30s"

strategies:
  ma_crossover:
    enabled: true
    params:
      fast_period: 50
      slow_period: 200

notifiers:
  telegram:
    enabled: true
    bot_token: "${TELEGRAM_BOT_TOKEN}"
    chat_id: "${TELEGRAM_CHAT_ID}"

router:
  cooldown_hours: 4
  min_confidence: 0.6

watchlist:
  - symbol: "600519.SH"
    name: "贵州茅台"
    strategies: ["ma_crossover"]
  - symbol: "AAPL"
    name: "Apple Inc"
    strategies: ["ma_crossover"]
```

**Step 7: Commit**

```bash
go mod tidy
git add internal/config/ configs/ go.mod go.sum
git commit -m "feat: add configuration system with viper"
```

---

## Task 4: Logging Setup

**Files:**
- Create: `internal/logger/logger.go`
- Create: `internal/logger/logger_test.go`

**Step 1: Add zap dependency**

```bash
go get go.uber.org/zap
```

**Step 2: Write failing test**

```go
// internal/logger/logger_test.go
package logger

import (
    "testing"
)

func TestNew_Development(t *testing.T) {
    log, err := New(true)
    if err != nil {
        t.Fatalf("failed to create logger: %v", err)
    }
    if log == nil {
        t.Fatal("expected non-nil logger")
    }

    // Should not panic
    log.Info("test message")
}

func TestNew_Production(t *testing.T) {
    log, err := New(false)
    if err != nil {
        t.Fatalf("failed to create logger: %v", err)
    }
    if log == nil {
        t.Fatal("expected non-nil logger")
    }
}
```

**Step 3: Run test to verify it fails**

Run: `go test ./internal/logger/... -v`
Expected: FAIL

**Step 4: Implement logger**

```go
// internal/logger/logger.go
package logger

import (
    "go.uber.org/zap"
    "go.uber.org/zap/zapcore"
)

// New creates a new zap logger
func New(development bool) (*zap.Logger, error) {
    var cfg zap.Config

    if development {
        cfg = zap.NewDevelopmentConfig()
        cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
    } else {
        cfg = zap.NewProductionConfig()
    }

    return cfg.Build()
}

// Must creates a logger or panics
func Must(development bool) *zap.Logger {
    log, err := New(development)
    if err != nil {
        panic(err)
    }
    return log
}
```

**Step 5: Run test to verify it passes**

Run: `go test ./internal/logger/... -v`
Expected: PASS

**Step 6: Commit**

```bash
go mod tidy
git add internal/logger/ go.mod go.sum
git commit -m "feat: add structured logging with zap"
```

---

## Task 5: CLI Framework with Cobra

**Files:**
- Modify: `cmd/atlas/main.go`
- Create: `cmd/atlas/serve.go`
- Create: `cmd/atlas/version.go`

**Step 1: Add cobra dependency**

```bash
go get github.com/spf13/cobra
```

**Step 2: Rewrite main.go with cobra**

```go
// cmd/atlas/main.go
package main

import (
    "os"

    "github.com/spf13/cobra"
)

var (
    cfgFile string
    debug   bool
)

var rootCmd = &cobra.Command{
    Use:   "atlas",
    Short: "ATLAS - Asset Tracking & Leadership Analysis System",
    Long: `ATLAS is a global asset monitoring system with automated trading signals.
It supports multiple markets (US, HK, CN_A) and asset types.`,
}

func init() {
    rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file path")
    rootCmd.PersistentFlags().BoolVarP(&debug, "debug", "d", false, "enable debug mode")
}

func main() {
    if err := rootCmd.Execute(); err != nil {
        os.Exit(1)
    }
}
```

**Step 3: Create version command**

```go
// cmd/atlas/version.go
package main

import (
    "fmt"

    "github.com/spf13/cobra"
)

var (
    Version   = "dev"
    GitCommit = "unknown"
    BuildTime = "unknown"
)

var versionCmd = &cobra.Command{
    Use:   "version",
    Short: "Print version information",
    Run: func(cmd *cobra.Command, args []string) {
        fmt.Printf("ATLAS %s\n", Version)
        fmt.Printf("  Git commit: %s\n", GitCommit)
        fmt.Printf("  Build time: %s\n", BuildTime)
    },
}

func init() {
    rootCmd.AddCommand(versionCmd)
}
```

**Step 4: Create serve command placeholder**

```go
// cmd/atlas/serve.go
package main

import (
    "fmt"
    "os"
    "os/signal"
    "syscall"

    "github.com/newthinker/atlas/internal/config"
    "github.com/newthinker/atlas/internal/logger"
    "github.com/spf13/cobra"
    "go.uber.org/zap"
)

var serveCmd = &cobra.Command{
    Use:   "serve",
    Short: "Start the ATLAS server",
    RunE:  runServe,
}

func init() {
    rootCmd.AddCommand(serveCmd)
}

func runServe(cmd *cobra.Command, args []string) error {
    // Initialize logger
    log := logger.Must(debug)
    defer log.Sync()

    // Load config
    var cfg *config.Config
    var err error

    if cfgFile != "" {
        cfg, err = config.Load(cfgFile)
        if err != nil {
            return fmt.Errorf("loading config: %w", err)
        }
    } else {
        cfg = config.Defaults()
    }

    log.Info("starting ATLAS server",
        zap.String("host", cfg.Server.Host),
        zap.Int("port", cfg.Server.Port),
    )

    // Wait for shutdown signal
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit

    log.Info("shutting down ATLAS server")
    return nil
}
```

**Step 5: Verify CLI works**

Run: `make build && ./bin/atlas version`
Expected: ATLAS dev

Run: `./bin/atlas --help`
Expected: Shows help with serve and version commands

**Step 6: Commit**

```bash
go mod tidy
git add cmd/atlas/ go.mod go.sum
git commit -m "feat: add CLI framework with cobra (serve, version commands)"
```

---

## Task 6: Collector Interface

**Files:**
- Create: `internal/collector/interface.go`
- Create: `internal/collector/registry.go`
- Create: `internal/collector/registry_test.go`

**Step 1: Write failing test for registry**

```go
// internal/collector/registry_test.go
package collector

import (
    "context"
    "testing"
    "time"

    "github.com/newthinker/atlas/internal/core"
)

// mockCollector for testing
type mockCollector struct {
    name string
}

func (m *mockCollector) Name() string                    { return m.name }
func (m *mockCollector) SupportedMarkets() []core.Market { return []core.Market{core.MarketUS} }
func (m *mockCollector) Init(cfg Config) error          { return nil }
func (m *mockCollector) Start(ctx context.Context) error { return nil }
func (m *mockCollector) Stop() error                     { return nil }
func (m *mockCollector) FetchQuote(symbol string) (*core.Quote, error) {
    return &core.Quote{Symbol: symbol, Price: 100}, nil
}
func (m *mockCollector) FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
    return nil, nil
}

func TestRegistry_Register(t *testing.T) {
    r := NewRegistry()

    mock := &mockCollector{name: "mock"}
    r.Register(mock)

    c, ok := r.Get("mock")
    if !ok {
        t.Fatal("expected to find registered collector")
    }

    if c.Name() != "mock" {
        t.Errorf("expected name 'mock', got '%s'", c.Name())
    }
}

func TestRegistry_GetAll(t *testing.T) {
    r := NewRegistry()
    r.Register(&mockCollector{name: "a"})
    r.Register(&mockCollector{name: "b"})

    all := r.GetAll()
    if len(all) != 2 {
        t.Errorf("expected 2 collectors, got %d", len(all))
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/collector/... -v`
Expected: FAIL

**Step 3: Implement interface and registry**

```go
// internal/collector/interface.go
package collector

import (
    "context"
    "time"

    "github.com/newthinker/atlas/internal/core"
)

// Config holds collector configuration
type Config struct {
    Enabled  bool
    Markets  []string
    Interval string
    APIKey   string
    Extra    map[string]any
}

// Collector defines the interface for data collectors
type Collector interface {
    // Metadata
    Name() string
    SupportedMarkets() []core.Market

    // Lifecycle
    Init(cfg Config) error
    Start(ctx context.Context) error
    Stop() error

    // Data fetching
    FetchQuote(symbol string) (*core.Quote, error)
    FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error)
}
```

```go
// internal/collector/registry.go
package collector

import "sync"

// Registry manages collector plugins
type Registry struct {
    mu         sync.RWMutex
    collectors map[string]Collector
}

// NewRegistry creates a new collector registry
func NewRegistry() *Registry {
    return &Registry{
        collectors: make(map[string]Collector),
    }
}

// Register adds a collector to the registry
func (r *Registry) Register(c Collector) {
    r.mu.Lock()
    defer r.mu.Unlock()
    r.collectors[c.Name()] = c
}

// Get retrieves a collector by name
func (r *Registry) Get(name string) (Collector, bool) {
    r.mu.RLock()
    defer r.mu.RUnlock()
    c, ok := r.collectors[name]
    return c, ok
}

// GetAll returns all registered collectors
func (r *Registry) GetAll() []Collector {
    r.mu.RLock()
    defer r.mu.RUnlock()

    result := make([]Collector, 0, len(r.collectors))
    for _, c := range r.collectors {
        result = append(result, c)
    }
    return result
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/collector/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/collector/
git commit -m "feat: add collector interface and registry"
```

---

## Task 7: Yahoo Finance Collector

**Files:**
- Create: `internal/collector/yahoo/yahoo.go`
- Create: `internal/collector/yahoo/yahoo_test.go`

**Step 1: Write failing test**

```go
// internal/collector/yahoo/yahoo_test.go
package yahoo

import (
    "testing"

    "github.com/newthinker/atlas/internal/collector"
    "github.com/newthinker/atlas/internal/core"
)

func TestYahoo_ImplementsCollector(t *testing.T) {
    var _ collector.Collector = (*Yahoo)(nil)
}

func TestYahoo_Name(t *testing.T) {
    y := New()
    if y.Name() != "yahoo" {
        t.Errorf("expected 'yahoo', got '%s'", y.Name())
    }
}

func TestYahoo_SupportedMarkets(t *testing.T) {
    y := New()
    markets := y.SupportedMarkets()

    if len(markets) == 0 {
        t.Error("expected at least one supported market")
    }
}

func TestYahoo_ParseSymbol(t *testing.T) {
    tests := []struct {
        input    string
        expected string
    }{
        {"AAPL", "AAPL"},
        {"0700.HK", "0700.HK"},
        {"600519.SH", "600519.SS"}, // Shanghai -> SS for Yahoo
        {"000001.SZ", "000001.SZ"},
    }

    y := New()
    for _, tc := range tests {
        got := y.toYahooSymbol(tc.input)
        if got != tc.expected {
            t.Errorf("toYahooSymbol(%s) = %s, want %s", tc.input, got, tc.expected)
        }
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/collector/yahoo/... -v`
Expected: FAIL

**Step 3: Implement Yahoo collector**

```go
// internal/collector/yahoo/yahoo.go
package yahoo

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "strings"
    "time"

    "github.com/newthinker/atlas/internal/collector"
    "github.com/newthinker/atlas/internal/core"
)

const (
    baseURL = "https://query1.finance.yahoo.com/v8/finance/chart"
)

// Yahoo implements the Yahoo Finance collector
type Yahoo struct {
    client  *http.Client
    config  collector.Config
}

// New creates a new Yahoo collector
func New() *Yahoo {
    return &Yahoo{
        client: &http.Client{
            Timeout: 10 * time.Second,
        },
    }
}

func (y *Yahoo) Name() string {
    return "yahoo"
}

func (y *Yahoo) SupportedMarkets() []core.Market {
    return []core.Market{core.MarketUS, core.MarketHK, core.MarketEU}
}

func (y *Yahoo) Init(cfg collector.Config) error {
    y.config = cfg
    return nil
}

func (y *Yahoo) Start(ctx context.Context) error {
    return nil
}

func (y *Yahoo) Stop() error {
    return nil
}

// toYahooSymbol converts internal symbol format to Yahoo format
func (y *Yahoo) toYahooSymbol(symbol string) string {
    // Shanghai stocks: 600519.SH -> 600519.SS
    if strings.HasSuffix(symbol, ".SH") {
        return strings.TrimSuffix(symbol, ".SH") + ".SS"
    }
    return symbol
}

// FetchQuote fetches real-time quote
func (y *Yahoo) FetchQuote(symbol string) (*core.Quote, error) {
    yahooSymbol := y.toYahooSymbol(symbol)
    url := fmt.Sprintf("%s/%s?interval=1d&range=1d", baseURL, yahooSymbol)

    resp, err := y.client.Get(url)
    if err != nil {
        return nil, fmt.Errorf("fetching quote: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
    }

    var result chartResponse
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, fmt.Errorf("decoding response: %w", err)
    }

    if result.Chart.Error != nil {
        return nil, fmt.Errorf("yahoo error: %s", result.Chart.Error.Description)
    }

    if len(result.Chart.Result) == 0 {
        return nil, fmt.Errorf("no data for symbol: %s", symbol)
    }

    r := result.Chart.Result[0]
    meta := r.Meta

    return &core.Quote{
        Symbol: symbol,
        Market: y.detectMarket(symbol),
        Price:  meta.RegularMarketPrice,
        Volume: int64(meta.RegularMarketVolume),
        Time:   time.Unix(int64(meta.RegularMarketTime), 0),
        Source: "yahoo",
    }, nil
}

// FetchHistory fetches historical OHLCV data
func (y *Yahoo) FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
    yahooSymbol := y.toYahooSymbol(symbol)
    yahooInterval := y.toYahooInterval(interval)

    url := fmt.Sprintf("%s/%s?interval=%s&period1=%d&period2=%d",
        baseURL, yahooSymbol, yahooInterval, start.Unix(), end.Unix())

    resp, err := y.client.Get(url)
    if err != nil {
        return nil, fmt.Errorf("fetching history: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
    }

    var result chartResponse
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, fmt.Errorf("decoding response: %w", err)
    }

    if result.Chart.Error != nil {
        return nil, fmt.Errorf("yahoo error: %s", result.Chart.Error.Description)
    }

    if len(result.Chart.Result) == 0 {
        return nil, fmt.Errorf("no data for symbol: %s", symbol)
    }

    r := result.Chart.Result[0]
    timestamps := r.Timestamp
    quotes := r.Indicators.Quote[0]

    data := make([]core.OHLCV, 0, len(timestamps))
    for i, ts := range timestamps {
        if quotes.Open[i] == nil {
            continue // Skip missing data
        }
        data = append(data, core.OHLCV{
            Symbol:   symbol,
            Interval: interval,
            Open:     *quotes.Open[i],
            High:     *quotes.High[i],
            Low:      *quotes.Low[i],
            Close:    *quotes.Close[i],
            Volume:   int64(*quotes.Volume[i]),
            Time:     time.Unix(int64(ts), 0),
        })
    }

    return data, nil
}

func (y *Yahoo) toYahooInterval(interval string) string {
    switch interval {
    case "1m":
        return "1m"
    case "5m":
        return "5m"
    case "1h":
        return "1h"
    case "1d":
        return "1d"
    default:
        return "1d"
    }
}

func (y *Yahoo) detectMarket(symbol string) core.Market {
    if strings.HasSuffix(symbol, ".HK") {
        return core.MarketHK
    }
    if strings.HasSuffix(symbol, ".SH") || strings.HasSuffix(symbol, ".SZ") {
        return core.MarketCNA
    }
    return core.MarketUS
}

// Yahoo API response types
type chartResponse struct {
    Chart struct {
        Result []chartResult `json:"result"`
        Error  *struct {
            Code        string `json:"code"`
            Description string `json:"description"`
        } `json:"error"`
    } `json:"chart"`
}

type chartResult struct {
    Meta       chartMeta   `json:"meta"`
    Timestamp  []int       `json:"timestamp"`
    Indicators indicators  `json:"indicators"`
}

type chartMeta struct {
    Symbol              string  `json:"symbol"`
    RegularMarketPrice  float64 `json:"regularMarketPrice"`
    RegularMarketVolume int     `json:"regularMarketVolume"`
    RegularMarketTime   int     `json:"regularMarketTime"`
}

type indicators struct {
    Quote []quoteIndicator `json:"quote"`
}

type quoteIndicator struct {
    Open   []*float64 `json:"open"`
    High   []*float64 `json:"high"`
    Low    []*float64 `json:"low"`
    Close  []*float64 `json:"close"`
    Volume []*int     `json:"volume"`
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/collector/yahoo/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/collector/yahoo/
git commit -m "feat: add Yahoo Finance collector"
```

---

## Task 8: Eastmoney Collector

**Files:**
- Create: `internal/collector/eastmoney/eastmoney.go`
- Create: `internal/collector/eastmoney/eastmoney_test.go`

**Step 1: Write failing test**

```go
// internal/collector/eastmoney/eastmoney_test.go
package eastmoney

import (
    "testing"

    "github.com/newthinker/atlas/internal/collector"
    "github.com/newthinker/atlas/internal/core"
)

func TestEastmoney_ImplementsCollector(t *testing.T) {
    var _ collector.Collector = (*Eastmoney)(nil)
}

func TestEastmoney_Name(t *testing.T) {
    e := New()
    if e.Name() != "eastmoney" {
        t.Errorf("expected 'eastmoney', got '%s'", e.Name())
    }
}

func TestEastmoney_ParseSymbol(t *testing.T) {
    tests := []struct {
        input      string
        wantCode   string
        wantMarket string
    }{
        {"600519.SH", "600519", "1"},  // Shanghai = 1
        {"000001.SZ", "000001", "0"},  // Shenzhen = 0
    }

    e := New()
    for _, tc := range tests {
        code, market := e.parseSymbol(tc.input)
        if code != tc.wantCode || market != tc.wantMarket {
            t.Errorf("parseSymbol(%s) = (%s, %s), want (%s, %s)",
                tc.input, code, market, tc.wantCode, tc.wantMarket)
        }
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/collector/eastmoney/... -v`
Expected: FAIL

**Step 3: Implement Eastmoney collector**

```go
// internal/collector/eastmoney/eastmoney.go
package eastmoney

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "regexp"
    "strconv"
    "strings"
    "time"

    "github.com/newthinker/atlas/internal/collector"
    "github.com/newthinker/atlas/internal/core"
)

const (
    quoteURL   = "https://push2.eastmoney.com/api/qt/stock/get"
    historyURL = "https://push2his.eastmoney.com/api/qt/stock/kline/get"
)

// Eastmoney implements the Eastmoney collector for A-shares
type Eastmoney struct {
    client *http.Client
    config collector.Config
}

// New creates a new Eastmoney collector
func New() *Eastmoney {
    return &Eastmoney{
        client: &http.Client{
            Timeout: 10 * time.Second,
        },
    }
}

func (e *Eastmoney) Name() string {
    return "eastmoney"
}

func (e *Eastmoney) SupportedMarkets() []core.Market {
    return []core.Market{core.MarketCNA}
}

func (e *Eastmoney) Init(cfg collector.Config) error {
    e.config = cfg
    return nil
}

func (e *Eastmoney) Start(ctx context.Context) error {
    return nil
}

func (e *Eastmoney) Stop() error {
    return nil
}

// parseSymbol converts 600519.SH to (600519, 1) for Eastmoney API
// Shanghai = 1, Shenzhen = 0
func (e *Eastmoney) parseSymbol(symbol string) (code, market string) {
    parts := strings.Split(symbol, ".")
    if len(parts) != 2 {
        return symbol, "1"
    }

    code = parts[0]
    switch parts[1] {
    case "SH":
        market = "1"
    case "SZ":
        market = "0"
    default:
        market = "1"
    }
    return
}

// FetchQuote fetches real-time quote from Eastmoney
func (e *Eastmoney) FetchQuote(symbol string) (*core.Quote, error) {
    code, market := e.parseSymbol(symbol)
    secid := fmt.Sprintf("%s.%s", market, code)

    url := fmt.Sprintf("%s?secid=%s&fields=f43,f44,f45,f46,f47,f48,f50,f57,f58,f60",
        quoteURL, secid)

    resp, err := e.client.Get(url)
    if err != nil {
        return nil, fmt.Errorf("fetching quote: %w", err)
    }
    defer resp.Body.Close()

    var result quoteResponse
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, fmt.Errorf("decoding response: %w", err)
    }

    if result.Data == nil {
        return nil, fmt.Errorf("no data for symbol: %s", symbol)
    }

    d := result.Data
    return &core.Quote{
        Symbol: symbol,
        Market: core.MarketCNA,
        Price:  float64(d.F43) / 100, // Price in cents
        Volume: int64(d.F47),
        Bid:    float64(d.F44) / 100,
        Ask:    float64(d.F45) / 100,
        Time:   time.Now(),
        Source: "eastmoney",
    }, nil
}

// FetchHistory fetches historical OHLCV data
func (e *Eastmoney) FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
    code, market := e.parseSymbol(symbol)
    secid := fmt.Sprintf("%s.%s", market, code)
    klt := e.toKlineType(interval)

    url := fmt.Sprintf("%s?secid=%s&klt=%s&fqt=1&beg=%s&end=%s&fields1=f1,f2,f3,f4,f5,f6&fields2=f51,f52,f53,f54,f55,f56",
        historyURL, secid, klt,
        start.Format("20060102"),
        end.Format("20060102"))

    resp, err := e.client.Get(url)
    if err != nil {
        return nil, fmt.Errorf("fetching history: %w", err)
    }
    defer resp.Body.Close()

    var result historyResponse
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, fmt.Errorf("decoding response: %w", err)
    }

    if result.Data == nil || len(result.Data.Klines) == 0 {
        return nil, fmt.Errorf("no history for symbol: %s", symbol)
    }

    data := make([]core.OHLCV, 0, len(result.Data.Klines))
    re := regexp.MustCompile(`(\d{4}-\d{2}-\d{2}),([^,]+),([^,]+),([^,]+),([^,]+),([^,]+)`)

    for _, line := range result.Data.Klines {
        matches := re.FindStringSubmatch(line)
        if len(matches) < 7 {
            continue
        }

        t, _ := time.Parse("2006-01-02", matches[1])
        open, _ := strconv.ParseFloat(matches[2], 64)
        close, _ := strconv.ParseFloat(matches[3], 64)
        high, _ := strconv.ParseFloat(matches[4], 64)
        low, _ := strconv.ParseFloat(matches[5], 64)
        volume, _ := strconv.ParseInt(matches[6], 10, 64)

        data = append(data, core.OHLCV{
            Symbol:   symbol,
            Interval: interval,
            Open:     open,
            High:     high,
            Low:      low,
            Close:    close,
            Volume:   volume,
            Time:     t,
        })
    }

    return data, nil
}

func (e *Eastmoney) toKlineType(interval string) string {
    switch interval {
    case "1m":
        return "1"
    case "5m":
        return "5"
    case "15m":
        return "15"
    case "30m":
        return "30"
    case "1h":
        return "60"
    case "1d":
        return "101"
    default:
        return "101"
    }
}

// Response types
type quoteResponse struct {
    Data *quoteData `json:"data"`
}

type quoteData struct {
    F43 int    `json:"f43"` // Current price (cents)
    F44 int    `json:"f44"` // Bid
    F45 int    `json:"f45"` // Ask
    F46 int    `json:"f46"` // Open
    F47 int64  `json:"f47"` // Volume
    F48 int64  `json:"f48"` // Amount
    F57 string `json:"f57"` // Code
    F58 string `json:"f58"` // Name
}

type historyResponse struct {
    Data *historyData `json:"data"`
}

type historyData struct {
    Code   string   `json:"code"`
    Name   string   `json:"name"`
    Klines []string `json:"klines"`
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/collector/eastmoney/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/collector/eastmoney/
git commit -m "feat: add Eastmoney collector for A-shares"
```

---

## Task 9: Strategy Interface

**Files:**
- Create: `internal/strategy/interface.go`
- Create: `internal/strategy/engine.go`
- Create: `internal/strategy/engine_test.go`

**Step 1: Write failing test**

```go
// internal/strategy/engine_test.go
package strategy

import (
    "context"
    "testing"
    "time"

    "github.com/newthinker/atlas/internal/core"
)

type mockStrategy struct {
    name    string
    signals []core.Signal
}

func (m *mockStrategy) Name() string        { return m.name }
func (m *mockStrategy) Description() string { return "mock strategy" }
func (m *mockStrategy) RequiredData() DataRequirements {
    return DataRequirements{PriceHistory: 200}
}
func (m *mockStrategy) Init(cfg Config) error { return nil }
func (m *mockStrategy) Analyze(ctx AnalysisContext) ([]core.Signal, error) {
    return m.signals, nil
}

func TestEngine_RegisterAndRun(t *testing.T) {
    engine := NewEngine()

    mockSig := core.Signal{
        Symbol:     "AAPL",
        Action:     core.ActionBuy,
        Confidence: 0.8,
        Strategy:   "mock",
    }

    engine.Register(&mockStrategy{
        name:    "mock",
        signals: []core.Signal{mockSig},
    })

    ctx := AnalysisContext{
        Symbol: "AAPL",
        OHLCV:  []core.OHLCV{},
    }

    signals, err := engine.Analyze(context.Background(), ctx)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    if len(signals) != 1 {
        t.Fatalf("expected 1 signal, got %d", len(signals))
    }

    if signals[0].Action != core.ActionBuy {
        t.Errorf("expected Buy action, got %s", signals[0].Action)
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/strategy/... -v`
Expected: FAIL

**Step 3: Implement interface and engine**

```go
// internal/strategy/interface.go
package strategy

import (
    "time"

    "github.com/newthinker/atlas/internal/core"
)

// Config holds strategy configuration
type Config struct {
    Enabled bool
    Params  map[string]any
}

// DataRequirements specifies what data a strategy needs
type DataRequirements struct {
    Markets      []core.Market
    AssetTypes   []core.AssetType
    PriceHistory int  // Days of history needed
    Fundamentals bool // Needs fundamental data
    Indicators   []string
}

// AnalysisContext provides data to strategies
type AnalysisContext struct {
    Symbol       string
    Market       core.Market
    OHLCV        []core.OHLCV
    LatestQuote  *core.Quote
    Fundamentals map[string]float64
    Indicators   map[string][]float64
    Now          time.Time
}

// Strategy defines the interface for trading strategies
type Strategy interface {
    Name() string
    Description() string
    RequiredData() DataRequirements
    Init(cfg Config) error
    Analyze(ctx AnalysisContext) ([]core.Signal, error)
}
```

```go
// internal/strategy/engine.go
package strategy

import (
    "context"
    "sync"

    "github.com/newthinker/atlas/internal/core"
)

// Engine manages and runs strategies
type Engine struct {
    mu         sync.RWMutex
    strategies map[string]Strategy
}

// NewEngine creates a new strategy engine
func NewEngine() *Engine {
    return &Engine{
        strategies: make(map[string]Strategy),
    }
}

// Register adds a strategy to the engine
func (e *Engine) Register(s Strategy) {
    e.mu.Lock()
    defer e.mu.Unlock()
    e.strategies[s.Name()] = s
}

// Get retrieves a strategy by name
func (e *Engine) Get(name string) (Strategy, bool) {
    e.mu.RLock()
    defer e.mu.RUnlock()
    s, ok := e.strategies[name]
    return s, ok
}

// GetAll returns all registered strategies
func (e *Engine) GetAll() []Strategy {
    e.mu.RLock()
    defer e.mu.RUnlock()

    result := make([]Strategy, 0, len(e.strategies))
    for _, s := range e.strategies {
        result = append(result, s)
    }
    return result
}

// Analyze runs all strategies on the given context
func (e *Engine) Analyze(ctx context.Context, analysisCtx AnalysisContext) ([]core.Signal, error) {
    e.mu.RLock()
    strategies := make([]Strategy, 0, len(e.strategies))
    for _, s := range e.strategies {
        strategies = append(strategies, s)
    }
    e.mu.RUnlock()

    var allSignals []core.Signal

    for _, s := range strategies {
        select {
        case <-ctx.Done():
            return allSignals, ctx.Err()
        default:
        }

        signals, err := s.Analyze(analysisCtx)
        if err != nil {
            // Log error but continue with other strategies
            continue
        }

        // Add strategy name to signals
        for i := range signals {
            signals[i].Strategy = s.Name()
        }

        allSignals = append(allSignals, signals...)
    }

    return allSignals, nil
}

// AnalyzeWithStrategies runs specific strategies
func (e *Engine) AnalyzeWithStrategies(ctx context.Context, analysisCtx AnalysisContext, strategyNames []string) ([]core.Signal, error) {
    var allSignals []core.Signal

    for _, name := range strategyNames {
        select {
        case <-ctx.Done():
            return allSignals, ctx.Err()
        default:
        }

        s, ok := e.Get(name)
        if !ok {
            continue
        }

        signals, err := s.Analyze(analysisCtx)
        if err != nil {
            continue
        }

        for i := range signals {
            signals[i].Strategy = s.Name()
        }

        allSignals = append(allSignals, signals...)
    }

    return allSignals, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/strategy/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/strategy/
git commit -m "feat: add strategy interface and engine"
```

---

## Task 10: MA Crossover Strategy

**Files:**
- Create: `internal/strategy/ma_crossover/strategy.go`
- Create: `internal/strategy/ma_crossover/strategy_test.go`
- Create: `internal/indicator/sma.go`
- Create: `internal/indicator/sma_test.go`

**Step 1: Write failing test for SMA indicator**

```go
// internal/indicator/sma_test.go
package indicator

import (
    "testing"
)

func TestSMA_Calculate(t *testing.T) {
    prices := []float64{10, 11, 12, 13, 14, 15}

    sma := SMA(prices, 3)

    // SMA(3) for [10,11,12,13,14,15]:
    // [0] = (10+11+12)/3 = 11
    // [1] = (11+12+13)/3 = 12
    // [2] = (12+13+14)/3 = 13
    // [3] = (13+14+15)/3 = 14

    expected := []float64{11, 12, 13, 14}

    if len(sma) != len(expected) {
        t.Fatalf("expected %d values, got %d", len(expected), len(sma))
    }

    for i, v := range expected {
        if sma[i] != v {
            t.Errorf("sma[%d] = %f, want %f", i, sma[i], v)
        }
    }
}

func TestSMA_NotEnoughData(t *testing.T) {
    prices := []float64{10, 11}
    sma := SMA(prices, 5)

    if len(sma) != 0 {
        t.Errorf("expected empty slice, got %d values", len(sma))
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/indicator/... -v`
Expected: FAIL

**Step 3: Implement SMA**

```go
// internal/indicator/sma.go
package indicator

// SMA calculates Simple Moving Average
// Returns slice of length: len(prices) - period + 1
func SMA(prices []float64, period int) []float64 {
    if len(prices) < period {
        return []float64{}
    }

    result := make([]float64, 0, len(prices)-period+1)

    // Calculate first SMA
    var sum float64
    for i := 0; i < period; i++ {
        sum += prices[i]
    }
    result = append(result, sum/float64(period))

    // Rolling calculation
    for i := period; i < len(prices); i++ {
        sum = sum - prices[i-period] + prices[i]
        result = append(result, sum/float64(period))
    }

    return result
}

// EMA calculates Exponential Moving Average
func EMA(prices []float64, period int) []float64 {
    if len(prices) < period {
        return []float64{}
    }

    result := make([]float64, 0, len(prices)-period+1)
    multiplier := 2.0 / float64(period+1)

    // Start with SMA as first EMA value
    var sum float64
    for i := 0; i < period; i++ {
        sum += prices[i]
    }
    ema := sum / float64(period)
    result = append(result, ema)

    // Calculate EMA for remaining prices
    for i := period; i < len(prices); i++ {
        ema = (prices[i]-ema)*multiplier + ema
        result = append(result, ema)
    }

    return result
}
```

**Step 4: Run indicator test**

Run: `go test ./internal/indicator/... -v`
Expected: PASS

**Step 5: Write failing test for MA Crossover strategy**

```go
// internal/strategy/ma_crossover/strategy_test.go
package ma_crossover

import (
    "testing"
    "time"

    "github.com/newthinker/atlas/internal/core"
    "github.com/newthinker/atlas/internal/strategy"
)

func TestMACrossover_ImplementsStrategy(t *testing.T) {
    var _ strategy.Strategy = (*MACrossover)(nil)
}

func TestMACrossover_GoldenCross(t *testing.T) {
    s := New(5, 10)

    // Create data where fast MA crosses above slow MA (golden cross)
    // Fast MA will be higher at the end
    ohlcv := make([]core.OHLCV, 15)
    prices := []float64{100, 101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 115, 120, 125, 130}

    for i := 0; i < 15; i++ {
        ohlcv[i] = core.OHLCV{
            Symbol: "TEST",
            Close:  prices[i],
            Time:   time.Now().Add(time.Duration(-15+i) * 24 * time.Hour),
        }
    }

    ctx := strategy.AnalysisContext{
        Symbol: "TEST",
        OHLCV:  ohlcv,
        Now:    time.Now(),
    }

    signals, err := s.Analyze(ctx)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    // Should generate a buy signal for golden cross
    if len(signals) == 0 {
        t.Fatal("expected at least one signal")
    }

    if signals[0].Action != core.ActionBuy {
        t.Errorf("expected Buy action for golden cross, got %s", signals[0].Action)
    }
}

func TestMACrossover_NotEnoughData(t *testing.T) {
    s := New(50, 200)

    // Only 100 days of data, need 200 for slow MA
    ohlcv := make([]core.OHLCV, 100)
    for i := 0; i < 100; i++ {
        ohlcv[i] = core.OHLCV{Close: 100}
    }

    ctx := strategy.AnalysisContext{
        Symbol: "TEST",
        OHLCV:  ohlcv,
    }

    signals, err := s.Analyze(ctx)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    // Should return no signals due to insufficient data
    if len(signals) != 0 {
        t.Errorf("expected no signals with insufficient data, got %d", len(signals))
    }
}
```

**Step 6: Run test to verify it fails**

Run: `go test ./internal/strategy/ma_crossover/... -v`
Expected: FAIL

**Step 7: Implement MA Crossover strategy**

```go
// internal/strategy/ma_crossover/strategy.go
package ma_crossover

import (
    "fmt"
    "time"

    "github.com/newthinker/atlas/internal/core"
    "github.com/newthinker/atlas/internal/indicator"
    "github.com/newthinker/atlas/internal/strategy"
)

// MACrossover implements a moving average crossover strategy
type MACrossover struct {
    fastPeriod int
    slowPeriod int
}

// New creates a new MA Crossover strategy
func New(fastPeriod, slowPeriod int) *MACrossover {
    return &MACrossover{
        fastPeriod: fastPeriod,
        slowPeriod: slowPeriod,
    }
}

func (m *MACrossover) Name() string {
    return "ma_crossover"
}

func (m *MACrossover) Description() string {
    return fmt.Sprintf("MA Crossover (%d/%d)", m.fastPeriod, m.slowPeriod)
}

func (m *MACrossover) RequiredData() strategy.DataRequirements {
    return strategy.DataRequirements{
        PriceHistory: m.slowPeriod + 10, // Extra buffer
        Indicators:   []string{"SMA"},
    }
}

func (m *MACrossover) Init(cfg strategy.Config) error {
    if fast, ok := cfg.Params["fast_period"].(int); ok {
        m.fastPeriod = fast
    }
    if slow, ok := cfg.Params["slow_period"].(int); ok {
        m.slowPeriod = slow
    }
    return nil
}

func (m *MACrossover) Analyze(ctx strategy.AnalysisContext) ([]core.Signal, error) {
    if len(ctx.OHLCV) < m.slowPeriod {
        return nil, nil // Not enough data
    }

    // Extract closing prices
    prices := make([]float64, len(ctx.OHLCV))
    for i, bar := range ctx.OHLCV {
        prices[i] = bar.Close
    }

    // Calculate moving averages
    fastMA := indicator.SMA(prices, m.fastPeriod)
    slowMA := indicator.SMA(prices, m.slowPeriod)

    if len(fastMA) < 2 || len(slowMA) < 2 {
        return nil, nil
    }

    // Align the MAs (fast MA has more values since it uses shorter period)
    offset := len(fastMA) - len(slowMA)
    if offset < 0 {
        return nil, nil
    }

    // Get current and previous values
    currFast := fastMA[len(fastMA)-1]
    prevFast := fastMA[len(fastMA)-2]
    currSlow := slowMA[len(slowMA)-1]
    prevSlow := slowMA[len(slowMA)-2]

    var signals []core.Signal

    // Golden Cross: fast crosses above slow
    if prevFast <= prevSlow && currFast > currSlow {
        signals = append(signals, core.Signal{
            Symbol:      ctx.Symbol,
            Action:      core.ActionBuy,
            Confidence:  m.calculateConfidence(currFast, currSlow),
            Reason:      fmt.Sprintf("Golden Cross: MA%d (%.2f) crossed above MA%d (%.2f)", m.fastPeriod, currFast, m.slowPeriod, currSlow),
            GeneratedAt: time.Now(),
            Metadata: map[string]any{
                "fast_ma": currFast,
                "slow_ma": currSlow,
                "type":    "golden_cross",
            },
        })
    }

    // Death Cross: fast crosses below slow
    if prevFast >= prevSlow && currFast < currSlow {
        signals = append(signals, core.Signal{
            Symbol:      ctx.Symbol,
            Action:      core.ActionSell,
            Confidence:  m.calculateConfidence(currFast, currSlow),
            Reason:      fmt.Sprintf("Death Cross: MA%d (%.2f) crossed below MA%d (%.2f)", m.fastPeriod, currFast, m.slowPeriod, currSlow),
            GeneratedAt: time.Now(),
            Metadata: map[string]any{
                "fast_ma": currFast,
                "slow_ma": currSlow,
                "type":    "death_cross",
            },
        })
    }

    return signals, nil
}

// calculateConfidence returns higher confidence for larger divergence
func (m *MACrossover) calculateConfidence(fast, slow float64) float64 {
    diff := (fast - slow) / slow
    if diff < 0 {
        diff = -diff
    }

    // Scale to 0.5-0.9 range based on divergence
    confidence := 0.5 + (diff * 10)
    if confidence > 0.9 {
        confidence = 0.9
    }
    return confidence
}
```

**Step 8: Run test to verify it passes**

Run: `go test ./internal/strategy/ma_crossover/... -v`
Expected: PASS

**Step 9: Commit**

```bash
git add internal/indicator/ internal/strategy/ma_crossover/
git commit -m "feat: add SMA indicator and MA Crossover strategy"
```

---

## Task 11: Notifier Interface

**Files:**
- Create: `internal/notifier/interface.go`
- Create: `internal/notifier/registry.go`
- Create: `internal/notifier/registry_test.go`

**Step 1: Write failing test**

```go
// internal/notifier/registry_test.go
package notifier

import (
    "testing"

    "github.com/newthinker/atlas/internal/core"
)

type mockNotifier struct {
    name     string
    sent     []core.Signal
}

func (m *mockNotifier) Name() string                  { return m.name }
func (m *mockNotifier) Init(cfg Config) error         { return nil }
func (m *mockNotifier) Send(s core.Signal) error      { m.sent = append(m.sent, s); return nil }
func (m *mockNotifier) SendBatch(s []core.Signal) error { m.sent = append(m.sent, s...); return nil }

func TestRegistry_Register(t *testing.T) {
    r := NewRegistry()
    mock := &mockNotifier{name: "mock"}

    r.Register(mock)

    n, ok := r.Get("mock")
    if !ok {
        t.Fatal("expected to find mock notifier")
    }

    if n.Name() != "mock" {
        t.Errorf("expected name 'mock', got '%s'", n.Name())
    }
}

func TestRegistry_Broadcast(t *testing.T) {
    r := NewRegistry()
    m1 := &mockNotifier{name: "m1"}
    m2 := &mockNotifier{name: "m2"}

    r.Register(m1)
    r.Register(m2)

    signal := core.Signal{Symbol: "TEST", Action: core.ActionBuy}

    if err := r.Broadcast(signal); err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    if len(m1.sent) != 1 || len(m2.sent) != 1 {
        t.Error("expected signal sent to both notifiers")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/notifier/... -v`
Expected: FAIL

**Step 3: Implement interface and registry**

```go
// internal/notifier/interface.go
package notifier

import "github.com/newthinker/atlas/internal/core"

// Config holds notifier configuration
type Config struct {
    Enabled  bool
    BotToken string
    ChatID   string
    URL      string
    Extra    map[string]any
}

// Notifier defines the interface for notification channels
type Notifier interface {
    Name() string
    Init(cfg Config) error
    Send(signal core.Signal) error
    SendBatch(signals []core.Signal) error
}
```

```go
// internal/notifier/registry.go
package notifier

import (
    "sync"

    "github.com/newthinker/atlas/internal/core"
)

// Registry manages notifier plugins
type Registry struct {
    mu        sync.RWMutex
    notifiers map[string]Notifier
}

// NewRegistry creates a new notifier registry
func NewRegistry() *Registry {
    return &Registry{
        notifiers: make(map[string]Notifier),
    }
}

// Register adds a notifier to the registry
func (r *Registry) Register(n Notifier) {
    r.mu.Lock()
    defer r.mu.Unlock()
    r.notifiers[n.Name()] = n
}

// Get retrieves a notifier by name
func (r *Registry) Get(name string) (Notifier, bool) {
    r.mu.RLock()
    defer r.mu.RUnlock()
    n, ok := r.notifiers[name]
    return n, ok
}

// GetAll returns all registered notifiers
func (r *Registry) GetAll() []Notifier {
    r.mu.RLock()
    defer r.mu.RUnlock()

    result := make([]Notifier, 0, len(r.notifiers))
    for _, n := range r.notifiers {
        result = append(result, n)
    }
    return result
}

// Broadcast sends a signal to all notifiers
func (r *Registry) Broadcast(signal core.Signal) error {
    r.mu.RLock()
    notifiers := make([]Notifier, 0, len(r.notifiers))
    for _, n := range r.notifiers {
        notifiers = append(notifiers, n)
    }
    r.mu.RUnlock()

    var lastErr error
    for _, n := range notifiers {
        if err := n.Send(signal); err != nil {
            lastErr = err
        }
    }
    return lastErr
}

// BroadcastBatch sends multiple signals to all notifiers
func (r *Registry) BroadcastBatch(signals []core.Signal) error {
    r.mu.RLock()
    notifiers := make([]Notifier, 0, len(r.notifiers))
    for _, n := range r.notifiers {
        notifiers = append(notifiers, n)
    }
    r.mu.RUnlock()

    var lastErr error
    for _, n := range notifiers {
        if err := n.SendBatch(signals); err != nil {
            lastErr = err
        }
    }
    return lastErr
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/notifier/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/notifier/
git commit -m "feat: add notifier interface and registry"
```

---

## Task 12: Telegram Notifier

**Files:**
- Create: `internal/notifier/telegram/telegram.go`
- Create: `internal/notifier/telegram/telegram_test.go`

**Step 1: Write failing test**

```go
// internal/notifier/telegram/telegram_test.go
package telegram

import (
    "testing"

    "github.com/newthinker/atlas/internal/notifier"
)

func TestTelegram_ImplementsNotifier(t *testing.T) {
    var _ notifier.Notifier = (*Telegram)(nil)
}

func TestTelegram_Name(t *testing.T) {
    tg := New("token", "chatid")
    if tg.Name() != "telegram" {
        t.Errorf("expected 'telegram', got '%s'", tg.Name())
    }
}

func TestTelegram_FormatSignal(t *testing.T) {
    tg := New("token", "chatid")

    signal := testSignal()
    msg := tg.formatMessage(signal)

    if msg == "" {
        t.Error("expected non-empty message")
    }

    // Check for key components
    if !containsAll(msg, "BUY", "AAPL", "ma_crossover") {
        t.Error("message missing expected components")
    }
}

func containsAll(s string, substrs ...string) bool {
    for _, sub := range substrs {
        if !contains(s, sub) {
            return false
        }
    }
    return true
}

func contains(s, substr string) bool {
    return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
    for i := 0; i <= len(s)-len(substr); i++ {
        if s[i:i+len(substr)] == substr {
            return true
        }
    }
    return false
}

func testSignal() core.Signal {
    return core.Signal{
        Symbol:     "AAPL",
        Action:     core.ActionBuy,
        Confidence: 0.85,
        Reason:     "Golden Cross detected",
        Strategy:   "ma_crossover",
    }
}
```

**Step 2: Add import and run test to verify it fails**

Run: `go test ./internal/notifier/telegram/... -v`
Expected: FAIL

**Step 3: Implement Telegram notifier**

```go
// internal/notifier/telegram/telegram.go
package telegram

import (
    "bytes"
    "encoding/json"
    "fmt"
    "net/http"
    "strings"
    "time"

    "github.com/newthinker/atlas/internal/core"
    "github.com/newthinker/atlas/internal/notifier"
)

const apiURL = "https://api.telegram.org/bot%s/sendMessage"

// Telegram implements Telegram notifications
type Telegram struct {
    botToken string
    chatID   string
    client   *http.Client
}

// New creates a new Telegram notifier
func New(botToken, chatID string) *Telegram {
    return &Telegram{
        botToken: botToken,
        chatID:   chatID,
        client: &http.Client{
            Timeout: 10 * time.Second,
        },
    }
}

func (t *Telegram) Name() string {
    return "telegram"
}

func (t *Telegram) Init(cfg notifier.Config) error {
    if cfg.BotToken != "" {
        t.botToken = cfg.BotToken
    }
    if cfg.ChatID != "" {
        t.chatID = cfg.ChatID
    }
    return nil
}

func (t *Telegram) Send(signal core.Signal) error {
    msg := t.formatMessage(signal)
    return t.sendMessage(msg)
}

func (t *Telegram) SendBatch(signals []core.Signal) error {
    if len(signals) == 0 {
        return nil
    }

    var sb strings.Builder
    sb.WriteString("📊 *ATLAS Signal Batch*\n")
    sb.WriteString(fmt.Sprintf("_%d signals_\n\n", len(signals)))

    for _, sig := range signals {
        sb.WriteString(t.formatSignalCompact(sig))
        sb.WriteString("\n")
    }

    return t.sendMessage(sb.String())
}

func (t *Telegram) formatMessage(s core.Signal) string {
    emoji := t.actionEmoji(s.Action)
    action := strings.ToUpper(string(s.Action))

    var sb strings.Builder
    sb.WriteString(fmt.Sprintf("%s *%s Signal*: `%s`\n", emoji, action, s.Symbol))
    sb.WriteString(fmt.Sprintf("Strategy: %s\n", s.Strategy))
    sb.WriteString(fmt.Sprintf("Confidence: %.0f%%\n", s.Confidence*100))
    sb.WriteString(fmt.Sprintf("Reason: %s\n", s.Reason))
    sb.WriteString("───────────────")

    return sb.String()
}

func (t *Telegram) formatSignalCompact(s core.Signal) string {
    emoji := t.actionEmoji(s.Action)
    return fmt.Sprintf("%s `%s` - %s (%.0f%%)",
        emoji, s.Symbol, s.Strategy, s.Confidence*100)
}

func (t *Telegram) actionEmoji(action core.Action) string {
    switch action {
    case core.ActionBuy, core.ActionStrongBuy:
        return "🟢"
    case core.ActionSell, core.ActionStrongSell:
        return "🔴"
    default:
        return "⚪"
    }
}

func (t *Telegram) sendMessage(text string) error {
    url := fmt.Sprintf(apiURL, t.botToken)

    payload := map[string]any{
        "chat_id":    t.chatID,
        "text":       text,
        "parse_mode": "Markdown",
    }

    body, err := json.Marshal(payload)
    if err != nil {
        return fmt.Errorf("marshaling payload: %w", err)
    }

    resp, err := t.client.Post(url, "application/json", bytes.NewReader(body))
    if err != nil {
        return fmt.Errorf("sending message: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("telegram API returned status: %d", resp.StatusCode)
    }

    return nil
}
```

**Step 4: Fix test imports and run**

```go
// internal/notifier/telegram/telegram_test.go
package telegram

import (
    "testing"

    "github.com/newthinker/atlas/internal/core"
    "github.com/newthinker/atlas/internal/notifier"
)

func TestTelegram_ImplementsNotifier(t *testing.T) {
    var _ notifier.Notifier = (*Telegram)(nil)
}

func TestTelegram_Name(t *testing.T) {
    tg := New("token", "chatid")
    if tg.Name() != "telegram" {
        t.Errorf("expected 'telegram', got '%s'", tg.Name())
    }
}

func TestTelegram_FormatSignal(t *testing.T) {
    tg := New("token", "chatid")

    signal := testSignal()
    msg := tg.formatMessage(signal)

    if msg == "" {
        t.Error("expected non-empty message")
    }

    // Check for key components
    if !containsAll(msg, "BUY", "AAPL", "ma_crossover") {
        t.Error("message missing expected components")
    }
}

func containsAll(s string, substrs ...string) bool {
    for _, sub := range substrs {
        if !contains(s, sub) {
            return false
        }
    }
    return true
}

func contains(s, substr string) bool {
    return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
    for i := 0; i <= len(s)-len(substr); i++ {
        if s[i:i+len(substr)] == substr {
            return true
        }
    }
    return false
}

func testSignal() core.Signal {
    return core.Signal{
        Symbol:     "AAPL",
        Action:     core.ActionBuy,
        Confidence: 0.85,
        Reason:     "Golden Cross detected",
        Strategy:   "ma_crossover",
    }
}
```

Run: `go test ./internal/notifier/telegram/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/notifier/telegram/
git commit -m "feat: add Telegram notifier"
```

---

## Task 13: Signal Router

**Files:**
- Create: `internal/router/router.go`
- Create: `internal/router/router_test.go`
- Create: `internal/router/filter/cooldown.go`
- Create: `internal/router/filter/confidence.go`

**Step 1: Write failing test**

```go
// internal/router/router_test.go
package router

import (
    "testing"
    "time"

    "github.com/newthinker/atlas/internal/core"
)

func TestRouter_FiltersBelowConfidence(t *testing.T) {
    r := New(RouterConfig{
        MinConfidence: 0.6,
        CooldownHours: 4,
    })

    signals := []core.Signal{
        {Symbol: "AAPL", Confidence: 0.5}, // Below threshold
        {Symbol: "GOOG", Confidence: 0.8}, // Above threshold
    }

    filtered := r.Filter(signals)

    if len(filtered) != 1 {
        t.Fatalf("expected 1 signal, got %d", len(filtered))
    }

    if filtered[0].Symbol != "GOOG" {
        t.Errorf("expected GOOG, got %s", filtered[0].Symbol)
    }
}

func TestRouter_CooldownFilter(t *testing.T) {
    r := New(RouterConfig{
        MinConfidence: 0.5,
        CooldownHours: 1,
    })

    signal := core.Signal{
        Symbol:      "AAPL",
        Action:      core.ActionBuy,
        Confidence:  0.8,
        Strategy:    "test",
        GeneratedAt: time.Now(),
    }

    // First signal should pass
    filtered := r.Filter([]core.Signal{signal})
    if len(filtered) != 1 {
        t.Fatal("first signal should pass")
    }

    // Mark as sent
    r.MarkSent(signal)

    // Same signal should be filtered (cooldown)
    filtered = r.Filter([]core.Signal{signal})
    if len(filtered) != 0 {
        t.Error("duplicate signal should be filtered by cooldown")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/router/... -v`
Expected: FAIL

**Step 3: Implement router**

```go
// internal/router/router.go
package router

import (
    "sync"
    "time"

    "github.com/newthinker/atlas/internal/core"
)

// RouterConfig holds router configuration
type RouterConfig struct {
    MinConfidence float64
    CooldownHours int
}

// signalKey uniquely identifies a signal for deduplication
type signalKey struct {
    Symbol   string
    Strategy string
    Action   core.Action
}

// Router filters and routes signals
type Router struct {
    config  RouterConfig
    history map[signalKey]time.Time
    mu      sync.RWMutex
}

// New creates a new signal router
func New(cfg RouterConfig) *Router {
    return &Router{
        config:  cfg,
        history: make(map[signalKey]time.Time),
    }
}

// Filter applies all filters to signals
func (r *Router) Filter(signals []core.Signal) []core.Signal {
    var result []core.Signal

    for _, sig := range signals {
        if r.shouldPass(sig) {
            result = append(result, sig)
        }
    }

    return result
}

func (r *Router) shouldPass(sig core.Signal) bool {
    // Confidence filter
    if sig.Confidence < r.config.MinConfidence {
        return false
    }

    // Cooldown filter
    if r.isInCooldown(sig) {
        return false
    }

    return true
}

func (r *Router) isInCooldown(sig core.Signal) bool {
    key := signalKey{
        Symbol:   sig.Symbol,
        Strategy: sig.Strategy,
        Action:   sig.Action,
    }

    r.mu.RLock()
    lastSent, exists := r.history[key]
    r.mu.RUnlock()

    if !exists {
        return false
    }

    cooldown := time.Duration(r.config.CooldownHours) * time.Hour
    return time.Since(lastSent) < cooldown
}

// MarkSent records that a signal was sent
func (r *Router) MarkSent(sig core.Signal) {
    key := signalKey{
        Symbol:   sig.Symbol,
        Strategy: sig.Strategy,
        Action:   sig.Action,
    }

    r.mu.Lock()
    r.history[key] = time.Now()
    r.mu.Unlock()
}

// CleanupHistory removes old entries from history
func (r *Router) CleanupHistory() {
    r.mu.Lock()
    defer r.mu.Unlock()

    cutoff := time.Now().Add(-24 * time.Hour)
    for key, t := range r.history {
        if t.Before(cutoff) {
            delete(r.history, key)
        }
    }
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/router/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/router/
git commit -m "feat: add signal router with cooldown and confidence filters"
```

---

## Task 14: Integration - Wire Everything Together

**Files:**
- Modify: `cmd/atlas/serve.go`
- Create: `internal/app/app.go`

**Step 1: Create app orchestrator**

```go
// internal/app/app.go
package app

import (
    "context"
    "fmt"
    "sync"
    "time"

    "github.com/newthinker/atlas/internal/collector"
    "github.com/newthinker/atlas/internal/collector/eastmoney"
    "github.com/newthinker/atlas/internal/collector/yahoo"
    "github.com/newthinker/atlas/internal/config"
    "github.com/newthinker/atlas/internal/core"
    "github.com/newthinker/atlas/internal/notifier"
    "github.com/newthinker/atlas/internal/notifier/telegram"
    "github.com/newthinker/atlas/internal/router"
    "github.com/newthinker/atlas/internal/strategy"
    "github.com/newthinker/atlas/internal/strategy/ma_crossover"
    "go.uber.org/zap"
)

// App is the main application orchestrator
type App struct {
    config     *config.Config
    log        *zap.Logger
    collectors *collector.Registry
    strategies *strategy.Engine
    notifiers  *notifier.Registry
    router     *router.Router

    stopCh     chan struct{}
    wg         sync.WaitGroup
}

// New creates a new App instance
func New(cfg *config.Config, log *zap.Logger) (*App, error) {
    app := &App{
        config:     cfg,
        log:        log,
        collectors: collector.NewRegistry(),
        strategies: strategy.NewEngine(),
        notifiers:  notifier.NewRegistry(),
        router: router.New(router.RouterConfig{
            MinConfidence: cfg.Router.MinConfidence,
            CooldownHours: cfg.Router.CooldownHours,
        }),
        stopCh: make(chan struct{}),
    }

    if err := app.initCollectors(); err != nil {
        return nil, fmt.Errorf("initializing collectors: %w", err)
    }

    if err := app.initStrategies(); err != nil {
        return nil, fmt.Errorf("initializing strategies: %w", err)
    }

    if err := app.initNotifiers(); err != nil {
        return nil, fmt.Errorf("initializing notifiers: %w", err)
    }

    return app, nil
}

func (a *App) initCollectors() error {
    // Yahoo collector
    if cfg, ok := a.config.Collectors["yahoo"]; ok && cfg.Enabled {
        y := yahoo.New()
        if err := y.Init(collector.Config{
            Enabled:  cfg.Enabled,
            Markets:  cfg.Markets,
            Interval: cfg.Interval,
        }); err != nil {
            return err
        }
        a.collectors.Register(y)
        a.log.Info("registered collector", zap.String("name", "yahoo"))
    }

    // Eastmoney collector
    if cfg, ok := a.config.Collectors["eastmoney"]; ok && cfg.Enabled {
        e := eastmoney.New()
        if err := e.Init(collector.Config{
            Enabled:  cfg.Enabled,
            Markets:  cfg.Markets,
            Interval: cfg.Interval,
        }); err != nil {
            return err
        }
        a.collectors.Register(e)
        a.log.Info("registered collector", zap.String("name", "eastmoney"))
    }

    return nil
}

func (a *App) initStrategies() error {
    // MA Crossover strategy
    if cfg, ok := a.config.Strategies["ma_crossover"]; ok && cfg.Enabled {
        fastPeriod := 50
        slowPeriod := 200

        if v, ok := cfg.Params["fast_period"].(int); ok {
            fastPeriod = v
        }
        if v, ok := cfg.Params["slow_period"].(int); ok {
            slowPeriod = v
        }

        s := ma_crossover.New(fastPeriod, slowPeriod)
        a.strategies.Register(s)
        a.log.Info("registered strategy", zap.String("name", "ma_crossover"))
    }

    return nil
}

func (a *App) initNotifiers() error {
    // Telegram notifier
    if cfg, ok := a.config.Notifiers["telegram"]; ok && cfg.Enabled {
        tg := telegram.New(cfg.BotToken, cfg.ChatID)
        a.notifiers.Register(tg)
        a.log.Info("registered notifier", zap.String("name", "telegram"))
    }

    return nil
}

// Start starts the application
func (a *App) Start(ctx context.Context) error {
    a.log.Info("starting ATLAS")

    // Start analysis loop for each watchlist item
    for _, item := range a.config.Watchlist {
        a.wg.Add(1)
        go a.analyzeLoop(ctx, item)
    }

    return nil
}

func (a *App) analyzeLoop(ctx context.Context, item config.WatchlistItem) {
    defer a.wg.Done()

    ticker := time.NewTicker(1 * time.Minute)
    defer ticker.Stop()

    a.log.Info("starting analysis loop", zap.String("symbol", item.Symbol))

    for {
        select {
        case <-ctx.Done():
            return
        case <-a.stopCh:
            return
        case <-ticker.C:
            if err := a.analyzeSymbol(ctx, item); err != nil {
                a.log.Error("analysis failed",
                    zap.String("symbol", item.Symbol),
                    zap.Error(err))
            }
        }
    }
}

func (a *App) analyzeSymbol(ctx context.Context, item config.WatchlistItem) error {
    // Fetch historical data
    var ohlcv []core.OHLCV
    var quote *core.Quote

    for _, c := range a.collectors.GetAll() {
        // Try to get data from this collector
        history, err := c.FetchHistory(item.Symbol,
            time.Now().AddDate(0, 0, -250), // ~1 year
            time.Now(),
            "1d")
        if err == nil && len(history) > 0 {
            ohlcv = history
        }

        q, err := c.FetchQuote(item.Symbol)
        if err == nil {
            quote = q
        }
    }

    if len(ohlcv) == 0 {
        return fmt.Errorf("no data available for %s", item.Symbol)
    }

    // Run analysis
    analysisCtx := strategy.AnalysisContext{
        Symbol:      item.Symbol,
        OHLCV:       ohlcv,
        LatestQuote: quote,
        Now:         time.Now(),
    }

    signals, err := a.strategies.AnalyzeWithStrategies(ctx, analysisCtx, item.Strategies)
    if err != nil {
        return err
    }

    if len(signals) == 0 {
        return nil
    }

    // Filter signals
    filtered := a.router.Filter(signals)
    if len(filtered) == 0 {
        return nil
    }

    // Send notifications
    for _, sig := range filtered {
        a.log.Info("signal generated",
            zap.String("symbol", sig.Symbol),
            zap.String("action", string(sig.Action)),
            zap.String("strategy", sig.Strategy),
            zap.Float64("confidence", sig.Confidence))

        if err := a.notifiers.Broadcast(sig); err != nil {
            a.log.Error("notification failed", zap.Error(err))
        }

        a.router.MarkSent(sig)
    }

    return nil
}

// Stop gracefully stops the application
func (a *App) Stop() error {
    a.log.Info("stopping ATLAS")
    close(a.stopCh)
    a.wg.Wait()
    return nil
}
```

**Step 2: Update serve command**

```go
// cmd/atlas/serve.go
package main

import (
    "context"
    "fmt"
    "os"
    "os/signal"
    "syscall"

    "github.com/newthinker/atlas/internal/app"
    "github.com/newthinker/atlas/internal/config"
    "github.com/newthinker/atlas/internal/logger"
    "github.com/spf13/cobra"
    "go.uber.org/zap"
)

var serveCmd = &cobra.Command{
    Use:   "serve",
    Short: "Start the ATLAS server",
    RunE:  runServe,
}

func init() {
    rootCmd.AddCommand(serveCmd)
}

func runServe(cmd *cobra.Command, args []string) error {
    // Initialize logger
    log := logger.Must(debug)
    defer log.Sync()

    // Load config
    var cfg *config.Config
    var err error

    if cfgFile != "" {
        cfg, err = config.Load(cfgFile)
        if err != nil {
            return fmt.Errorf("loading config: %w", err)
        }
    } else {
        cfg = config.Defaults()
        log.Warn("no config file specified, using defaults")
    }

    log.Info("starting ATLAS server",
        zap.String("host", cfg.Server.Host),
        zap.Int("port", cfg.Server.Port),
    )

    // Create and start app
    atlas, err := app.New(cfg, log)
    if err != nil {
        return fmt.Errorf("creating app: %w", err)
    }

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    if err := atlas.Start(ctx); err != nil {
        return fmt.Errorf("starting app: %w", err)
    }

    // Wait for shutdown signal
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit

    log.Info("shutting down ATLAS server")
    return atlas.Stop()
}
```

**Step 3: Run build to verify it compiles**

Run: `make build`
Expected: Successful build

**Step 4: Run all tests**

Run: `make test`
Expected: All tests pass

**Step 5: Commit**

```bash
git add cmd/atlas/serve.go internal/app/
git commit -m "feat: integrate all components in app orchestrator"
```

---

## Task 15: Final Integration Test

**Files:**
- Create: `test/integration/app_test.go`

**Step 1: Create integration test**

```go
// test/integration/app_test.go
package integration

import (
    "context"
    "testing"
    "time"

    "github.com/newthinker/atlas/internal/app"
    "github.com/newthinker/atlas/internal/config"
    "github.com/newthinker/atlas/internal/logger"
)

func TestApp_StartStop(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test in short mode")
    }

    log := logger.Must(true)
    defer log.Sync()

    cfg := &config.Config{
        Server: config.ServerConfig{
            Host: "localhost",
            Port: 8080,
        },
        Storage: config.StorageConfig{
            Hot: config.HotStorageConfig{
                RetentionDays: 90,
            },
            Cold: config.ColdStorageConfig{
                Type: "localfs",
                Path: "/tmp/atlas-test",
            },
        },
        Collectors: map[string]config.CollectorConfig{
            "yahoo": {
                Enabled:  true,
                Markets:  []string{"US"},
                Interval: "5m",
            },
        },
        Strategies: map[string]config.StrategyConfig{
            "ma_crossover": {
                Enabled: true,
                Params: map[string]any{
                    "fast_period": 5,
                    "slow_period": 10,
                },
            },
        },
        Router: config.RouterConfig{
            CooldownHours: 1,
            MinConfidence: 0.5,
        },
        Watchlist: []config.WatchlistItem{
            {
                Symbol:     "AAPL",
                Name:       "Apple Inc",
                Strategies: []string{"ma_crossover"},
            },
        },
    }

    atlas, err := app.New(cfg, log)
    if err != nil {
        t.Fatalf("failed to create app: %v", err)
    }

    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    if err := atlas.Start(ctx); err != nil {
        t.Fatalf("failed to start app: %v", err)
    }

    // Let it run briefly
    time.Sleep(100 * time.Millisecond)

    if err := atlas.Stop(); err != nil {
        t.Fatalf("failed to stop app: %v", err)
    }
}
```

**Step 2: Run integration test**

Run: `go test ./test/integration/... -v`
Expected: PASS

**Step 3: Commit**

```bash
git add test/
git commit -m "test: add integration test for app lifecycle"
```

---

## Task 16: Final Cleanup and Documentation

**Step 1: Run all tests**

```bash
make test
```

**Step 2: Update README**

Update README.md with quick start instructions.

**Step 3: Final commit**

```bash
git add -A
git commit -m "docs: update README with Phase 1 completion"
```

---

## Summary

Phase 1 delivers:
- Core types (Quote, OHLCV, Signal)
- Configuration system (YAML + env vars)
- CLI framework (serve, version commands)
- Yahoo Finance collector
- Eastmoney collector for A-shares
- SMA indicator
- MA Crossover strategy
- Telegram notifier
- Signal router with cooldown/confidence filters
- App orchestrator wiring everything together

Total: 16 tasks, ~50 bite-sized steps
