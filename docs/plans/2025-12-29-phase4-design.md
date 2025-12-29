# ATLAS Phase 4 Design: LLM Meta-Strategies & Broker Integration

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add LLM-powered meta-strategies (signal arbitration, strategy synthesis) and broker integration (Futu) to ATLAS.

**Key Features:**
- **Signal Arbitrator** - LLM resolves conflicts when multiple strategies disagree
- **Strategy Synthesizer** - LLM analyzes historical performance to suggest improvements
- **Broker Abstraction** - Read portfolio positions, P&L, trade history (Futu first)
- **Configurable LLM** - Support Claude, OpenAI, and local models (Ollama)

---

## Architecture Overview

```
┌──────────────────────────────────────────────────────────────────────┐
│                         ATLAS Phase 4 Flow                            │
├──────────────────────────────────────────────────────────────────────┤
│                                                                       │
│  ┌─────────────┐     ┌─────────────┐     ┌─────────────────────────┐ │
│  │ Strategies  │────▶│   Router    │────▶│  Signal Arbitrator      │ │
│  │ (existing)  │     │  (filter)   │     │  (LLM resolves conflict)│ │
│  └─────────────┘     └─────────────┘     └───────────┬─────────────┘ │
│                                                      │               │
│                                                      ▼               │
│  ┌─────────────┐     ┌─────────────┐     ┌─────────────────────────┐ │
│  │   Broker    │◀────│  Notifiers  │◀────│   Final Signal          │ │
│  │   (Futu)    │     │ (telegram)  │     │   (arbitrated)          │ │
│  └──────┬──────┘     └─────────────┘     └─────────────────────────┘ │
│         │                                                            │
│         ▼                                                            │
│  ┌─────────────────────────────────────────────────────────────────┐ │
│  │              Strategy Synthesizer (weekly batch)                 │ │
│  │  Inputs: Trade history, Signal history, Broker positions        │ │
│  │  Output: Parameter suggestions, New rules, Combination patterns │ │
│  └─────────────────────────────────────────────────────────────────┘ │
│                                                                       │
└──────────────────────────────────────────────────────────────────────┘
```

---

## Component 1: LLM Provider Abstraction

### Interface

```go
// internal/llm/interface.go
type Provider interface {
    Name() string
    Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
    ChatStream(ctx context.Context, req ChatRequest) (<-chan ChatChunk, error)
}

type ChatRequest struct {
    SystemPrompt string
    Messages     []Message
    MaxTokens    int
    Temperature  float64
    JSONMode     bool  // For structured output
}

type Message struct {
    Role    string  // "user", "assistant"
    Content string
}

type ChatResponse struct {
    Content   string
    Usage     Usage
    FinishReason string
}

type Usage struct {
    InputTokens  int
    OutputTokens int
}
```

### Implementations

| Provider | Package | Notes |
|----------|---------|-------|
| Claude | `llm/claude/` | Anthropic API, Claude 3.5/4 |
| OpenAI | `llm/openai/` | GPT-4o, GPT-4 |
| Ollama | `llm/ollama/` | Local models (Qwen, Llama) |

### Configuration

```yaml
llm:
  provider: claude  # or "openai", "ollama"
  claude:
    api_key: "${ANTHROPIC_API_KEY}"
    model: "claude-sonnet-4-20250514"
  openai:
    api_key: "${OPENAI_API_KEY}"
    model: "gpt-4o"
  ollama:
    endpoint: "http://localhost:11434"
    model: "qwen2.5:32b"
```

---

## Component 2: Signal Arbitrator

Resolves conflicts when multiple strategies generate different signals for the same symbol.

### Interface

```go
// internal/meta/arbitrator.go
type Arbitrator struct {
    llm           llm.Provider
    marketContext MarketContextProvider
    trackRecord   TrackRecordProvider
    newsProvider  NewsProvider
}

type ArbitrationRequest struct {
    Symbol             string
    ConflictingSignals []core.Signal
    MarketContext      MarketContext
    StrategyStats      map[string]StrategyStats
    RecentNews         []NewsItem
}

type ArbitrationResult struct {
    Decision     core.Action
    Confidence   float64
    Reasoning    string
    WeightedFrom []string
}

func (a *Arbitrator) Arbitrate(ctx context.Context, req ArbitrationRequest) (*ArbitrationResult, error)
```

### Context Inputs

The arbitrator considers full context:

1. **Market Context** - Current regime (bull/bear), volatility level, sector trends
2. **Strategy Track Record** - Historical accuracy of each strategy in similar conditions
3. **Recent News** - Last 7 days of relevant news for the symbol

### Prompt Structure

Prompts are stored in `internal/meta/prompts/` and are configurable:

1. System: Trading signal arbitrator role
2. Context: Market regime, strategy accuracy, relevant news
3. Task: Analyze conflicting signals, decide best action
4. Output: Structured JSON with decision, confidence, reasoning

---

## Component 3: Strategy Synthesizer

Analyzes historical performance to suggest strategy improvements.

### Interface

```go
// internal/meta/synthesizer.go
type Synthesizer struct {
    llm         llm.Provider
    signalStore SignalHistoryProvider
    tradeStore  TradeHistoryProvider
}

type SynthesisRequest struct {
    TimeRange     TimeRange
    Strategies    []string
    Trades        []backtest.Trade
    Signals       []core.Signal
    MarketPeriods []MarketPeriod
}

type SynthesisResult struct {
    ParameterSuggestions []ParameterSuggestion
    NewRules             []RuleProposal
    CombinationRules     []CombinationRule
    Explanation          string
}

type ParameterSuggestion struct {
    Strategy     string
    Parameter    string
    CurrentVal   any
    SuggestedVal any
    Rationale    string
    BacktestDiff float64
}

type CombinationRule struct {
    Conditions []SignalCondition  // e.g., "MA_Crossover=BUY AND RSI<40"
    Action     core.Action
    Confidence float64
    Evidence   string
}
```

### Synthesis Capabilities

| Capability | Description |
|------------|-------------|
| Parameter Tuning | Suggest adjustments to existing strategy parameters |
| Rule Discovery | Propose new trading rules based on historical patterns |
| Combination Rules | Discover when multiple weak signals indicate strong opportunity |

### Execution

- Runs on schedule (weekly) or on-demand via CLI
- Outputs suggestions for human review before applying

---

## Component 4: Broker Abstraction

### Interface

```go
// internal/broker/interface.go
type Broker interface {
    // Metadata
    Name() string
    SupportedMarkets() []core.Market

    // Connection
    Connect(ctx context.Context) error
    Disconnect() error
    IsConnected() bool

    // Read operations (Phase 4)
    GetPositions(ctx context.Context) ([]Position, error)
    GetOrders(ctx context.Context, filter OrderFilter) ([]Order, error)
    GetAccountInfo(ctx context.Context) (*AccountInfo, error)
    GetTradeHistory(ctx context.Context, start, end time.Time) ([]Trade, error)

    // Write operations (Future - not implemented in Phase 4)
    PlaceOrder(ctx context.Context, order OrderRequest) (*Order, error)
    CancelOrder(ctx context.Context, orderID string) error
    ModifyOrder(ctx context.Context, orderID string, changes OrderChanges) (*Order, error)
}

type Position struct {
    Symbol       string
    Market       core.Market
    Quantity     int64
    AvgCost      float64
    MarketValue  float64
    UnrealizedPL float64
    RealizedPL   float64
}

type AccountInfo struct {
    TotalAssets   float64
    Cash          float64
    BuyingPower   float64
    MarginUsed    float64
    DayTradesLeft int
}

type Order struct {
    OrderID     string
    Symbol      string
    Side        OrderSide  // Buy, Sell
    Type        OrderType  // Market, Limit, Stop
    Quantity    int64
    Price       float64
    Status      OrderStatus
    FilledQty   int64
    AvgFillPrice float64
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

### Phase 4 Scope

- **Read operations only** - Portfolio positions, P&L, order history
- **Write operations defined** - Interface ready for future automation
- **Futu implementation first** - Other brokers can be added later

---

## Component 5: Futu Broker Implementation

### Architecture

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   ATLAS     │────▶│  Futu OpenD │────▶│ Futu Server │
│  (broker)   │     │  (gateway)  │     │   (cloud)   │
└─────────────┘     └─────────────┘     └─────────────┘
     Go client       Local daemon        Brokerage API
```

**Note:** Futu requires running their OpenD gateway locally. ATLAS connects to OpenD via protobuf.

### Implementation

```go
// internal/broker/futu/futu.go
type FutuBroker struct {
    client    *futu.Client
    config    Config
    connected bool
    mu        sync.RWMutex
}

type Config struct {
    Host         string
    Port         int
    SecurityFirm string
    RSAKeyPath   string
    TradePwd     string
    Env          TrdEnv  // REAL or SIMULATE
}
```

### Configuration

```yaml
broker:
  enabled: true
  provider: futu
  futu:
    host: "127.0.0.1"
    port: 11111
    env: simulate  # or "real"
    trade_password: "${FUTU_TRADE_PWD}"
    rsa_key_path: "/path/to/futu_rsa.key"
```

### Dependencies

- `github.com/futuopen/ftapi4go` - Official Futu Go SDK

---

## Component 6: Context Providers

### Market Context

```go
// internal/context/market.go
type MarketContextProvider interface {
    GetContext(ctx context.Context, market core.Market) (*MarketContext, error)
}

type MarketContext struct {
    Regime       MarketRegime  // Bull, Bear, Sideways
    Volatility   float64
    SectorTrends map[string]Trend
    InterestRate float64
    UpdatedAt    time.Time
}
```

### News Provider

```go
// internal/context/news.go
type NewsProvider interface {
    GetNews(ctx context.Context, symbol string, days int) ([]NewsItem, error)
    GetMarketNews(ctx context.Context, market core.Market, days int) ([]NewsItem, error)
}

type NewsItem struct {
    Title       string
    Summary     string
    Source      string
    URL         string
    Symbols     []string
    Sentiment   float64
    PublishedAt time.Time
}
```

### News Sources

| Source | Markets | Notes |
|--------|---------|-------|
| Eastmoney | CN_A | Extend existing collector |
| Futu | HK, US, CN_A | Broker provides news feed |
| RSS | Any | Configurable feeds |

### Configuration

```yaml
context:
  news:
    providers: [eastmoney, futu]
    cache_hours: 6
  market:
    volatility_source: yahoo
    update_interval: 1h
```

---

## Project Structure

```
internal/
├── llm/
│   ├── interface.go
│   ├── claude/
│   │   └── claude.go
│   ├── openai/
│   │   └── openai.go
│   └── ollama/
│       └── ollama.go
│
├── meta/
│   ├── arbitrator.go
│   ├── arbitrator_test.go
│   ├── synthesizer.go
│   ├── synthesizer_test.go
│   └── prompts/
│       ├── arbitrator.txt
│       └── synthesizer.txt
│
├── broker/
│   ├── interface.go
│   ├── futu/
│   │   ├── futu.go
│   │   ├── futu_test.go
│   │   └── convert.go
│   └── mock/
│       └── mock.go
│
└── context/
    ├── market.go
    ├── news.go
    └── track_record.go
```

---

## CLI Commands

```bash
# LLM meta-strategies
atlas arbitrate --symbol 600519.SH    # Force arbitration for a symbol
atlas synthesize --from 2024-01-01    # Run synthesis on historical data

# Broker operations
atlas broker status                   # Check broker connection
atlas broker positions                # List current positions
atlas broker orders                   # List recent orders
atlas broker history --from 2024-01-01  # Trade history
```

---

## Configuration Summary

```yaml
# LLM providers
llm:
  provider: claude
  claude:
    api_key: "${ANTHROPIC_API_KEY}"
    model: "claude-sonnet-4-20250514"
  openai:
    api_key: "${OPENAI_API_KEY}"
    model: "gpt-4o"
  ollama:
    endpoint: "http://localhost:11434"
    model: "qwen2.5:32b"

# Meta-strategies
meta:
  arbitrator:
    enabled: true
    min_conflict_confidence: 0.5
    context_days: 7
  synthesizer:
    enabled: true
    schedule: "0 0 * * 0"  # Weekly
    min_trades: 50

# Broker
broker:
  enabled: true
  provider: futu
  futu:
    host: "127.0.0.1"
    port: 11111
    env: simulate
    trade_password: "${FUTU_TRADE_PWD}"

# Context providers
context:
  news:
    providers: [eastmoney, futu]
  market:
    volatility_source: yahoo
```

---

## Risk Controls (Future Automation)

When write operations are enabled:

| Control | Description |
|---------|-------------|
| Position Limits | Max % of portfolio per symbol |
| Daily Loss Limit | Stop trading if daily loss exceeds threshold |
| Human Approval | Require approval above certain order sizes |
| Circuit Breaker | Pause on consecutive losses |

---

## Implementation Tasks

| Task | Description |
|------|-------------|
| 1 | LLM interface + Claude provider |
| 2 | OpenAI provider |
| 3 | Ollama provider |
| 4 | Market context provider |
| 5 | News provider (extend Eastmoney) |
| 6 | Track record provider |
| 7 | Signal Arbitrator core |
| 8 | Arbitrator integration with Router |
| 9 | Strategy Synthesizer core |
| 10 | Synthesizer CLI command |
| 11 | Broker interface |
| 12 | Futu broker implementation |
| 13 | Broker CLI commands |
| 14 | Web UI additions (positions, synthesis) |
| 15 | Final integration test |
