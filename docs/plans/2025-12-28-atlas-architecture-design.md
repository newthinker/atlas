# ATLAS Architecture Design

## Overview

ATLAS (Asset Tracking & Leadership Analysis System) is a global asset monitoring system with automated trading signal generation. This document describes the system architecture.

**Key Requirements:**
- Primary use case: Automated trading signals (buy/sell recommendations based on strategies)
- Deployment: Self-hosted cloud, scalable components (local-first for initial phase)
- Data sources: Mixed (free APIs + Lixinger for fundamental data)
- Strategies: Technical, fundamental, and ML (starting with technical)
- Actions: Notifications first, designed for future automated execution
- Storage: Tiered (hot for recent, cold on local NAS for historical)

## High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         ATLAS Core                               │
├─────────────────────────────────────────────────────────────────┤
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐              │
│  │   Data      │  │  Strategy   │  │   Signal    │              │
│  │ Collectors  │──▶│   Engine    │──▶│   Router    │              │
│  │  (plugins)  │  │  (plugins)  │  │             │              │
│  └─────────────┘  └─────────────┘  └──────┬──────┘              │
│         │                                  │                     │
│         ▼                                  ▼                     │
│  ┌─────────────┐                   ┌─────────────┐              │
│  │   Market    │                   │  Notifiers  │              │
│  │  Data Store │                   │  (plugins)  │              │
│  └─────────────┘                   └─────────────┘              │
├─────────────────────────────────────────────────────────────────┤
│                      Config & API Layer                          │
│         REST API  •  WebSocket  •  CLI  •  Web UI               │
└─────────────────────────────────────────────────────────────────┘
```

**Core Components:**

- **Data Collectors** - Plugins that fetch market data from various sources. Each collector implements a common interface and runs on configurable schedules.
- **Market Data Store** - Central time-series database. Hot data in TimescaleDB, cold data archived to local NAS.
- **Strategy Engine** - Loads strategy plugins, feeds them market data, collects signals. Strategies are isolated.
- **Signal Router** - Receives signals from strategies, deduplicates, applies filters, and routes to notifiers.
- **Notifiers** - Plugins for Telegram, email, webhook, etc. Future: broker API integration.
- **API Layer** - REST for configuration, WebSocket for real-time signal streaming, CLI for operations.

## Data Collectors

### Plugin Interface

```go
type DataCollector interface {
    // Metadata
    Name() string
    SupportedMarkets() []Market        // e.g., US, HK, CN_A
    SupportedAssetTypes() []AssetType  // e.g., Stock, Index, ETF, Commodity

    // Lifecycle
    Init(config CollectorConfig) error
    Start(ctx context.Context) error
    Stop() error

    // Data fetching
    FetchQuote(symbol string) (*Quote, error)
    FetchHistory(symbol string, start, end time.Time, interval Interval) ([]OHLCV, error)
    Subscribe(symbols []string, handler func(*Quote)) error  // For real-time sources
}
```

### Free Collectors

| Collector | Markets | Data Type | Rate Limit |
|-----------|---------|-----------|------------|
| `yahoo` | US, HK, EU | Delayed quotes, history | Free, 2000/hr |
| `eastmoney` | CN_A, CN_Fund | Real-time quotes, history | Scraping, careful |
| `sina` | CN_A, HK | Real-time quotes | Free, moderate |

### Paid Collector

| Collector | Markets | Data Type | Notes |
|-----------|---------|-----------|-------|
| `lixinger` | CN_A | Fundamental data (PE, PB, ROE, revenue, etc.) | API key required |

**Lixinger Integration Notes:**
- Primary use: Fundamental data for CN stocks (financial/non-financial companies)
- Endpoints: `/cn/company/fundamental/non_financial`, `/cn/company/fs/*` for financial statements
- Complements free collectors - they provide price/quotes, Lixinger provides fundamentals
- Rate limits per their API tier - cached aggressively since fundamentals update quarterly

**Scheduling:** Each collector declares its preferred update interval. A central scheduler coordinates to respect rate limits and prioritize real-time subscriptions over polling.

**Data Normalization:** All collectors output a common `Quote` and `OHLCV` struct. Currency, timezone, and symbol formats are normalized before storage.

## Strategy Engine

### Strategy Plugin Interface

```go
type Strategy interface {
    // Metadata
    Name() string
    Description() string
    RequiredData() DataRequirements  // What data this strategy needs

    // Lifecycle
    Init(config StrategyConfig) error

    // Core logic
    Analyze(ctx AnalysisContext) ([]Signal, error)
}

type DataRequirements struct {
    Markets      []Market
    AssetTypes   []AssetType
    PriceHistory Duration      // e.g., 200 days for MA200
    Fundamentals bool          // Needs Lixinger data?
    Indicators   []string      // Pre-computed: "SMA_20", "RSI_14", etc.
}

type Signal struct {
    Symbol      string
    Action      Action         // Buy, Sell, Hold, StrongBuy, StrongSell
    Confidence  float64        // 0.0 - 1.0
    Reason      string         // Human-readable explanation
    Metadata    map[string]any // Strategy-specific data
    GeneratedAt time.Time
}
```

### Built-in Strategies (Phase 1)

| Strategy | Type | Description |
|----------|------|-------------|
| `ma_crossover` | Technical | Golden/death cross (MA50/MA200) |
| `rsi_extreme` | Technical | RSI oversold (<30) / overbought (>70) |
| `pe_band` | Fundamental | PE below historical 20th percentile |
| `dividend_yield` | Fundamental | Yield above threshold + stable payout |

**Execution Model:** Strategies run on a schedule (e.g., every minute for technical, daily for fundamental). The engine manages concurrency - strategies run in parallel but each strategy processes assets sequentially to avoid race conditions.

**Indicator Pre-computation:** Common indicators (SMA, EMA, RSI, MACD, Bollinger) are pre-computed and cached. Strategies declare what they need; the engine ensures data is ready before calling `Analyze()`.

## Signal Router & Notifiers

### Signal Router

```go
type SignalRouter struct {
    filters    []SignalFilter
    notifiers  []Notifier
    history    SignalHistory    // Track sent signals for deduplication
}

type SignalFilter interface {
    Name() string
    ShouldPass(signal Signal, history SignalHistory) bool
}
```

### Built-in Filters

| Filter | Purpose |
|--------|---------|
| `cooldown` | Don't repeat same signal for same asset within N hours |
| `confidence_threshold` | Only pass signals above minimum confidence |
| `market_hours` | Optionally suppress signals outside trading hours |
| `aggregator` | Combine multiple weak signals into one strong signal |

### Signal Flow

```
Strategy Engine
      │
      ▼
┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│  Cooldown   │───▶│ Confidence  │───▶│ Aggregator  │
│   Filter    │    │  Threshold  │    │   Filter    │
└─────────────┘    └─────────────┘    └─────────────┘
                                             │
                         ┌───────────────────┼───────────────────┐
                         ▼                   ▼                   ▼
                   ┌──────────┐        ┌──────────┐        ┌──────────┐
                   │ Telegram │        │  Email   │        │ Webhook  │
                   └──────────┘        └──────────┘        └──────────┘
```

### Notifier Interface

```go
type Notifier interface {
    Name() string
    Init(config NotifierConfig) error
    Send(signal Signal) error
    SendBatch(signals []Signal) error  // For digest mode
}
```

### Built-in Notifiers

| Notifier | Features |
|----------|----------|
| `telegram` | Real-time alerts, inline buttons for quick actions |
| `email` | Daily/weekly digest mode, HTML formatting |
| `webhook` | POST JSON to any endpoint (future broker integration) |
| `log` | Write to file/stdout for debugging |

### Message Format (Telegram example)

```
🟢 BUY Signal: 600519.SH (贵州茅台)
Strategy: pe_band
Confidence: 85%
Reason: PE (22.3) below 5-year 20th percentile (24.1)
Price: ¥1,680.50
───────────────
[View Chart] [Dismiss] [Mute 24h]
```

## Data Storage

### Storage Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                      Hot Storage                                 │
│                    (TimescaleDB)                                 │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐             │
│  │   quotes    │  │   ohlcv     │  │ fundamentals│             │
│  │  (real-time)│  │  (1m/5m/1d) │  │ (quarterly) │             │
│  └─────────────┘  └─────────────┘  └─────────────┘             │
│                                                                  │
│  Retention: 90 days    Auto-compression    Continuous aggs      │
└─────────────────────────────────────────────────────────────────┘
                            │
                   (nightly archive job)
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│                      Cold Storage                                │
│              (Pluggable Storage Backend)                         │
│                                                                  │
│  ┌──────────────────┐    ┌──────────────────┐                  │
│  │   LocalFS/NAS    │    │  S3-Compatible   │                  │
│  │   (Phase 1)      │    │  (Phase 2)       │                  │
│  │                  │    │                  │                  │
│  │  /mnt/nas/atlas/ │    │  MinIO / AWS S3  │                  │
│  │  └── archive/    │    │                  │                  │
│  └──────────────────┘    └──────────────────┘                  │
│                                                                  │
│  Format: Parquet (columnar, compressed)                         │
│  Partitioned by: market/year/month                              │
└─────────────────────────────────────────────────────────────────┘
```

### Cold Storage Interface

```go
type ArchiveStorage interface {
    // Write archived data
    Write(ctx context.Context, path string, data []byte) error

    // Read for backtesting
    Read(ctx context.Context, path string) ([]byte, error)
    List(ctx context.Context, prefix string) ([]string, error)

    // Lifecycle
    Delete(ctx context.Context, path string) error
}

// Implementations
type LocalFSStorage struct {
    basePath string  // e.g., "/mnt/nas/atlas/archive"
}

type S3Storage struct {
    bucket   string
    endpoint string  // For MinIO or other S3-compatible
    client   *s3.Client
}
```

### Configuration

```yaml
storage:
  hot:
    type: timescaledb
    dsn: "postgres://localhost:5432/atlas"
    retention_days: 90

  cold:
    type: localfs              # or "s3" for cloud
    path: "/mnt/nas/atlas/archive"

    # Future S3 config (commented out for Phase 1)
    # type: s3
    # endpoint: "https://s3.amazonaws.com"
    # bucket: "atlas-archive"
```

### Archive File Structure (NAS)

```
/mnt/nas/atlas/archive/
├── ohlcv/
│   ├── CN_A/
│   │   ├── 2024/
│   │   │   ├── 01/
│   │   │   │   ├── 600519.parquet
│   │   │   │   └── 000001.parquet
│   │   │   └── 02/
│   │   └── 2023/
│   └── US/
│       └── 2024/
├── fundamentals/
│   └── CN_A/
│       └── 2024/
└── signals/
    └── 2024/
```

### Core Tables (TimescaleDB)

```sql
-- Real-time quotes (hypertable, partitioned by time)
CREATE TABLE quotes (
    time        TIMESTAMPTZ NOT NULL,
    symbol      TEXT NOT NULL,
    market      TEXT NOT NULL,
    price       DECIMAL(18,4),
    volume      BIGINT,
    bid         DECIMAL(18,4),
    ask         DECIMAL(18,4)
);

-- OHLCV candles (hypertable)
CREATE TABLE ohlcv (
    time        TIMESTAMPTZ NOT NULL,
    symbol      TEXT NOT NULL,
    interval    TEXT NOT NULL,  -- '1m', '5m', '1d'
    open        DECIMAL(18,4),
    high        DECIMAL(18,4),
    low         DECIMAL(18,4),
    close       DECIMAL(18,4),
    volume      BIGINT
);

-- Fundamental data from Lixinger
CREATE TABLE fundamentals (
    time        TIMESTAMPTZ NOT NULL,
    symbol      TEXT NOT NULL,
    metric      TEXT NOT NULL,  -- 'pe_ttm', 'pb', 'roe', etc.
    value       DECIMAL(18,6)
);

-- Generated signals (for history/audit)
CREATE TABLE signals (
    time        TIMESTAMPTZ NOT NULL,
    symbol      TEXT NOT NULL,
    strategy    TEXT NOT NULL,
    action      TEXT NOT NULL,
    confidence  DECIMAL(5,4),
    reason      TEXT,
    notified    BOOLEAN DEFAULT FALSE
);
```

**TimescaleDB Features Used:**
- **Hypertables** - Auto-partition by time for fast range queries
- **Compression** - 90%+ compression on data older than 7 days
- **Continuous Aggregates** - Pre-compute 1h/1d candles from 1m data
- **Retention Policies** - Auto-drop data older than 90 days (after archiving)

## Configuration & API Layer

### Configuration Example

```yaml
# config.yaml
server:
  host: "0.0.0.0"
  port: 8080
  mode: "release"  # or "debug"

collectors:
  yahoo:
    enabled: true
    markets: ["US", "HK"]
    interval: "5m"
  eastmoney:
    enabled: true
    markets: ["CN_A"]
    interval: "30s"
  lixinger:
    enabled: true
    api_key: "${LIXINGER_API_KEY}"  # From env
    interval: "24h"

strategies:
  ma_crossover:
    enabled: true
    params:
      fast_period: 50
      slow_period: 200
  pe_band:
    enabled: true
    params:
      lookback_years: 5
      threshold_percentile: 20

watchlist:
  - symbol: "600519.SH"
    name: "贵州茅台"
    strategies: ["ma_crossover", "pe_band"]
  - symbol: "AAPL"
    name: "Apple Inc"
    strategies: ["ma_crossover"]

notifiers:
  telegram:
    enabled: true
    bot_token: "${TELEGRAM_BOT_TOKEN}"
    chat_id: "${TELEGRAM_CHAT_ID}"
  webhook:
    enabled: false
    url: "https://example.com/hook"

router:
  cooldown_hours: 4
  min_confidence: 0.6
```

### API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/v1/health` | Health check |
| GET | `/api/v1/quotes/:symbol` | Latest quote |
| GET | `/api/v1/ohlcv/:symbol` | Historical candles |
| GET | `/api/v1/signals` | Recent signals (paginated) |
| GET | `/api/v1/watchlist` | List watched assets |
| POST | `/api/v1/watchlist` | Add asset to watchlist |
| DELETE | `/api/v1/watchlist/:symbol` | Remove from watchlist |
| GET | `/api/v1/strategies` | List available strategies |
| POST | `/api/v1/strategies/:name/backtest` | Run backtest |
| WS | `/api/v1/ws/signals` | Real-time signal stream |

### CLI Commands

```bash
# Run the server
atlas serve

# Manage watchlist
atlas watchlist add 600519.SH --strategies ma_crossover,pe_band
atlas watchlist list
atlas watchlist remove AAPL

# Manual operations
atlas fetch 600519.SH              # Force fetch latest data
atlas analyze 600519.SH            # Run all strategies now
atlas backtest pe_band --from 2020-01-01 --to 2024-01-01

# Database management
atlas db migrate                   # Run migrations
atlas db archive                   # Force archive to cold storage
```

### Authentication

**Phase 1 - Simple:**
- Single API key in config for local use
- Optional: disable auth entirely for localhost-only deployment

**Phase 2 - If needed:**
- JWT tokens with expiry
- Role-based access (admin, viewer)

## Project Structure

```
atlas/
├── cmd/
│   └── atlas/
│       └── main.go              # CLI entrypoint
│
├── internal/
│   ├── config/
│   │   ├── config.go            # Config structs & loading
│   │   └── validate.go          # Config validation
│   │
│   ├── core/
│   │   ├── types.go             # Quote, OHLCV, Signal, etc.
│   │   ├── market.go            # Market enums (US, HK, CN_A)
│   │   └── asset.go             # AssetType enums
│   │
│   ├── collector/
│   │   ├── interface.go         # DataCollector interface
│   │   ├── registry.go          # Plugin registration
│   │   ├── scheduler.go         # Collection scheduling
│   │   ├── yahoo/
│   │   │   └── yahoo.go
│   │   ├── eastmoney/
│   │   │   └── eastmoney.go
│   │   ├── sina/
│   │   │   └── sina.go
│   │   └── lixinger/
│   │       └── lixinger.go
│   │
│   ├── storage/
│   │   ├── interface.go         # Storage interfaces
│   │   ├── timescale/
│   │   │   ├── client.go
│   │   │   ├── quotes.go
│   │   │   ├── ohlcv.go
│   │   │   └── fundamentals.go
│   │   └── archive/
│   │       ├── interface.go     # ArchiveStorage interface
│   │       ├── localfs.go       # Local/NAS implementation
│   │       └── s3.go            # S3 implementation (Phase 2)
│   │
│   ├── indicator/
│   │   ├── interface.go         # Indicator interface
│   │   ├── sma.go
│   │   ├── ema.go
│   │   ├── rsi.go
│   │   ├── macd.go
│   │   └── bollinger.go
│   │
│   ├── strategy/
│   │   ├── interface.go         # Strategy interface
│   │   ├── engine.go            # Strategy execution engine
│   │   ├── registry.go          # Plugin registration
│   │   ├── ma_crossover/
│   │   │   └── strategy.go
│   │   ├── rsi_extreme/
│   │   │   └── strategy.go
│   │   ├── pe_band/
│   │   │   └── strategy.go
│   │   └── dividend_yield/
│   │       └── strategy.go
│   │
│   ├── router/
│   │   ├── router.go            # Signal router
│   │   └── filter/
│   │       ├── interface.go
│   │       ├── cooldown.go
│   │       ├── confidence.go
│   │       └── aggregator.go
│   │
│   ├── notifier/
│   │   ├── interface.go         # Notifier interface
│   │   ├── registry.go
│   │   ├── telegram/
│   │   │   └── telegram.go
│   │   ├── email/
│   │   │   └── email.go
│   │   └── webhook/
│   │       └── webhook.go
│   │
│   └── api/
│       ├── server.go            # HTTP server setup
│       ├── handler/
│       │   ├── health.go
│       │   ├── quotes.go
│       │   ├── signals.go
│       │   └── watchlist.go
│       └── ws/
│           └── signals.go       # WebSocket handler
│
├── migrations/
│   ├── 001_initial.up.sql
│   ├── 001_initial.down.sql
│   └── ...
│
├── configs/
│   ├── config.example.yaml
│   └── config.local.yaml        # Git-ignored
│
├── scripts/
│   ├── setup_db.sh
│   └── archive_cron.sh
│
├── docs/
│   └── plans/
│       └── 2025-12-28-atlas-architecture-design.md
│
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

### Key Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/spf13/cobra` | CLI framework |
| `github.com/spf13/viper` | Config management |
| `github.com/gin-gonic/gin` | HTTP router |
| `github.com/gorilla/websocket` | WebSocket support |
| `github.com/jackc/pgx/v5` | PostgreSQL driver |
| `github.com/xitongsys/parquet-go` | Parquet file handling |
| `go.uber.org/zap` | Structured logging |

## Error Handling & Resilience

### Error Categories

```go
type ErrorCategory int

const (
    ErrTransient   ErrorCategory = iota  // Retry: network timeout, rate limit
    ErrPermanent                         // Don't retry: invalid symbol, auth failed
    ErrDataQuality                       // Log & skip: bad data from source
)

type AtlasError struct {
    Category  ErrorCategory
    Component string        // "collector:yahoo", "strategy:pe_band"
    Message   string
    Cause     error
    Retryable bool
}
```

### Collector Resilience

| Scenario | Behavior |
|----------|----------|
| Rate limited | Exponential backoff (1s → 2s → 4s → max 5min) |
| Network timeout | Retry 3x, then mark source degraded |
| Source down | Fall back to other sources if available |
| Bad data | Log warning, skip record, continue |

### Strategy Resilience

| Scenario | Behavior |
|----------|----------|
| Missing data | Skip asset, log warning, don't crash |
| Strategy panic | Recover, log error, disable strategy temporarily |
| Slow execution | Timeout after configurable duration |

### Circuit Breaker (per collector/notifier)

```go
type CircuitBreaker struct {
    failureThreshold int           // e.g., 5 failures
    resetTimeout     time.Duration // e.g., 5 minutes
    state            State         // Closed, Open, HalfOpen
}
```

- **Closed**: Normal operation
- **Open**: Too many failures, skip calls, return cached/error
- **Half-Open**: After timeout, allow one test request

### Graceful Shutdown

```go
func (a *Atlas) Shutdown(ctx context.Context) error {
    // 1. Stop accepting new work
    a.scheduler.Stop()

    // 2. Wait for in-flight operations (with timeout)
    a.strategies.DrainWithTimeout(30 * time.Second)

    // 3. Flush pending notifications
    a.router.Flush()

    // 4. Close database connections
    a.storage.Close()

    return nil
}
```

### Observability

```go
// Structured logging throughout
log.Info("signal generated",
    zap.String("symbol", signal.Symbol),
    zap.String("strategy", signal.Strategy),
    zap.String("action", signal.Action),
    zap.Float64("confidence", signal.Confidence),
)

// Metrics (optional, for Phase 2)
// - collector_requests_total{source, status}
// - strategy_signals_total{strategy, action}
// - notifier_sent_total{channel, status}
```

## Implementation Phases

| Phase | Scope |
|-------|-------|
| **Phase 1** | Core engine, Yahoo + Eastmoney collectors, MA crossover strategy, Telegram notifier, local storage |
| **Phase 2** | Lixinger integration, fundamental strategies, more notifiers |
| **Phase 3** | Backtesting framework, Web UI, S3 cold storage option |
| **Phase 4** | ML strategies, broker API integration |
