# API Reference

This document covers the ATLAS REST API, WebSocket endpoints, and configuration schema.

## Table of Contents

- [REST API](#rest-api)
- [WebSocket](#websocket)
- [Configuration Schema](#configuration-schema)
- [Data Types](#data-types)

---

## REST API

Base URL: `http://localhost:8080/api`

### Health Check

```
GET /api/health
```

**Response:**

```json
{
  "status": "ok"
}
```

---

### Signals

#### List Signals

```
GET /api/signals
```

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `symbol` | string | Filter by symbol (e.g., "AAPL") |
| `strategy` | string | Filter by strategy (e.g., "ma_crossover") |
| `action` | string | Filter by action: "BUY", "SELL", "HOLD" |
| `from` | string | Start date (YYYY-MM-DD) |
| `to` | string | End date (YYYY-MM-DD) |
| `limit` | int | Max results (default: 50, max: 500) |
| `offset` | int | Pagination offset |

**Response:**

```json
{
  "signals": [
    {
      "id": "sig_abc123",
      "symbol": "AAPL",
      "name": "Apple Inc",
      "market": "US",
      "action": "BUY",
      "confidence": 0.85,
      "strategy": "ma_crossover",
      "reason": "Golden Cross - MA50 crossed above MA200",
      "price": 178.50,
      "timestamp": "2024-12-30T10:30:00Z"
    }
  ],
  "total": 1,
  "limit": 50,
  "offset": 0
}
```

#### Get Signal by ID

```
GET /api/signals/{id}
```

**Response:**

```json
{
  "id": "sig_abc123",
  "symbol": "AAPL",
  "name": "Apple Inc",
  "market": "US",
  "action": "BUY",
  "confidence": 0.85,
  "strategy": "ma_crossover",
  "reason": "Golden Cross - MA50 crossed above MA200",
  "price": 178.50,
  "timestamp": "2024-12-30T10:30:00Z",
  "metadata": {
    "fast_ma": 182.30,
    "slow_ma": 175.20,
    "crossover_days": 2
  }
}
```

---

### Watchlist

#### List Watchlist

```
GET /api/watchlist
```

**Response:**

```json
{
  "assets": [
    {
      "symbol": "AAPL",
      "name": "Apple Inc",
      "market": "US",
      "strategies": ["ma_crossover"],
      "last_price": 178.50,
      "updated_at": "2024-12-30T10:00:00Z"
    },
    {
      "symbol": "600519.SH",
      "name": "Kweichow Moutai",
      "market": "CN_A",
      "strategies": ["ma_crossover", "pe_band"],
      "last_price": 1850.00,
      "updated_at": "2024-12-30T10:00:00Z"
    }
  ]
}
```

#### Add to Watchlist

```
POST /api/watchlist
```

**Request Body:**

```json
{
  "symbol": "MSFT",
  "name": "Microsoft Corp",
  "strategies": ["ma_crossover"]
}
```

**Response:**

```json
{
  "symbol": "MSFT",
  "name": "Microsoft Corp",
  "market": "US",
  "strategies": ["ma_crossover"],
  "created_at": "2024-12-30T11:00:00Z"
}
```

#### Remove from Watchlist

```
DELETE /api/watchlist/{symbol}
```

**Response:**

```json
{
  "deleted": true,
  "symbol": "MSFT"
}
```

---

### Backtest

#### Run Backtest

```
POST /api/backtest
```

**Request Body:**

```json
{
  "strategy": "ma_crossover",
  "symbol": "AAPL",
  "from": "2023-01-01",
  "to": "2024-01-01",
  "params": {
    "fast_period": 50,
    "slow_period": 200
  }
}
```

**Response:**

```json
{
  "id": "bt_xyz789",
  "strategy": "ma_crossover",
  "symbol": "AAPL",
  "period": {
    "from": "2023-01-01",
    "to": "2024-01-01"
  },
  "stats": {
    "total_trades": 12,
    "winning_trades": 8,
    "losing_trades": 4,
    "win_rate": 0.667,
    "total_return": 0.245,
    "max_drawdown": -0.082,
    "sharpe_ratio": 1.45
  },
  "trades": [
    {
      "entry_date": "2023-02-15",
      "entry_price": 152.30,
      "exit_date": "2023-03-20",
      "exit_price": 165.40,
      "return": 0.086,
      "action": "BUY"
    }
  ]
}
```

#### Get Backtest Result

```
GET /api/backtest/{id}
```

Returns the same structure as the POST response.

---

### Broker

#### Get Account Info

```
GET /api/broker/account
```

**Response:**

```json
{
  "total_assets": 150000.00,
  "cash": 37500.00,
  "buying_power": 50000.00,
  "margin_used": 0.00,
  "day_trades_left": 3
}
```

#### Get Positions

```
GET /api/broker/positions
```

**Response:**

```json
{
  "positions": [
    {
      "symbol": "AAPL",
      "market": "US",
      "quantity": 100,
      "avg_cost": 150.00,
      "market_value": 17500.00,
      "unrealized_pl": 2500.00
    }
  ]
}
```

#### Get Orders

```
GET /api/broker/orders
```

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `symbol` | string | Filter by symbol |
| `status` | string | Filter by status: "open", "filled", "cancelled" |
| `side` | string | Filter by side: "buy", "sell" |

**Response:**

```json
{
  "orders": [
    {
      "order_id": "ord_123",
      "symbol": "AAPL",
      "market": "US",
      "side": "buy",
      "type": "limit",
      "quantity": 10,
      "price": 175.00,
      "status": "open",
      "created_at": "2024-12-30T09:30:00Z"
    }
  ]
}
```

#### Get Trade History

```
GET /api/broker/history
```

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `from` | string | Start date (YYYY-MM-DD) |
| `to` | string | End date (YYYY-MM-DD) |

**Response:**

```json
{
  "trades": [
    {
      "trade_id": "trd_456",
      "symbol": "AAPL",
      "side": "buy",
      "quantity": 100,
      "price": 150.00,
      "fee": 0.50,
      "timestamp": "2024-11-15T14:30:00Z"
    }
  ]
}
```

---

### LLM Meta-Strategies

#### Arbitrate Signals

```
POST /api/meta/arbitrate
```

**Request Body:**

```json
{
  "symbol": "600519.SH",
  "signals": [
    {
      "strategy": "ma_crossover",
      "action": "SELL",
      "confidence": 0.75,
      "reason": "Death Cross"
    },
    {
      "strategy": "pe_band",
      "action": "BUY",
      "confidence": 0.82,
      "reason": "PE below 20th percentile"
    }
  ]
}
```

**Response:**

```json
{
  "decision": "HOLD",
  "confidence": 0.70,
  "reasoning": "PE band suggests undervaluation, but technical weakness indicates poor entry timing. Wait for MA confirmation before buying.",
  "provider": "claude"
}
```

#### Synthesize Strategy

```
POST /api/meta/synthesize
```

**Request Body:**

```json
{
  "strategy": "ma_crossover",
  "from": "2024-01-01",
  "to": "2024-12-01"
}
```

**Response:**

```json
{
  "recommendations": [
    {
      "type": "parameter",
      "description": "Increase fast MA period from 50 to 60",
      "expected_improvement": "Reduce false signals by ~15%",
      "confidence": 0.72
    },
    {
      "type": "rule",
      "description": "Add volume confirmation - require 1.5x average volume on crossover day",
      "expected_improvement": "Improve win rate by ~8%",
      "confidence": 0.68
    }
  ],
  "analysis": {
    "total_signals": 156,
    "profitable": 98,
    "unprofitable": 58,
    "patterns_found": 3
  },
  "provider": "claude"
}
```

---

## WebSocket

WebSocket endpoint: `ws://localhost:8080/ws`

### Connection

```javascript
const ws = new WebSocket('ws://localhost:8080/ws');

ws.onopen = () => {
  // Subscribe to signals
  ws.send(JSON.stringify({
    type: 'subscribe',
    channels: ['signals', 'quotes']
  }));
};

ws.onmessage = (event) => {
  const message = JSON.parse(event.data);
  console.log(message);
};
```

### Message Types

#### Subscribe

```json
{
  "type": "subscribe",
  "channels": ["signals", "quotes"]
}
```

#### Unsubscribe

```json
{
  "type": "unsubscribe",
  "channels": ["quotes"]
}
```

#### Signal Event

```json
{
  "type": "signal",
  "data": {
    "symbol": "AAPL",
    "action": "BUY",
    "confidence": 0.85,
    "strategy": "ma_crossover",
    "reason": "Golden Cross",
    "timestamp": "2024-12-30T10:30:00Z"
  }
}
```

#### Quote Event

```json
{
  "type": "quote",
  "data": {
    "symbol": "AAPL",
    "price": 178.50,
    "change": 2.30,
    "change_pct": 1.3,
    "volume": 45678900,
    "timestamp": "2024-12-30T10:30:00Z"
  }
}
```

#### Error Event

```json
{
  "type": "error",
  "error": "Invalid channel: foo"
}
```

---

## Configuration Schema

Complete configuration file schema with all options:

```yaml
# Server configuration
server:
  host: "0.0.0.0"         # Bind address
  port: 8080              # HTTP port
  read_timeout: 30s       # Request read timeout
  write_timeout: 30s      # Response write timeout

# Data collectors
collectors:
  yahoo:
    enabled: true
    markets: ["US", "HK"]
    rate_limit: 5         # Requests per second
    timeout: 10s

  eastmoney:
    enabled: true
    markets: ["CN_A"]
    rate_limit: 10
    timeout: 10s

  lixinger:
    enabled: false
    api_key: "${LIXINGER_API_KEY}"
    timeout: 30s

# Trading strategies
strategies:
  ma_crossover:
    enabled: true
    params:
      fast_period: 50     # Short-term MA period
      slow_period: 200    # Long-term MA period
      ma_type: "sma"      # "sma" or "ema"

  pe_band:
    enabled: false
    params:
      lookback_years: 5
      threshold_percentile: 20

  dividend_yield:
    enabled: false
    params:
      min_yield: 3.0
      min_payout_years: 5
      max_payout_ratio: 80

# Signal routing
router:
  cooldown_hours: 4       # Don't repeat same signal within N hours
  min_confidence: 0.6     # Only pass signals >= this confidence

# Notification channels
notifiers:
  telegram:
    enabled: true
    bot_token: "${TELEGRAM_BOT_TOKEN}"
    chat_id: "${TELEGRAM_CHAT_ID}"
    parse_mode: "HTML"    # "HTML" or "Markdown"

  email:
    enabled: false
    host: "smtp.gmail.com"
    port: 587
    username: "your@gmail.com"
    password: "${EMAIL_PASSWORD}"
    from: "ATLAS <atlas@example.com>"
    to:
      - "you@example.com"
    tls: true

  webhook:
    enabled: false
    url: "https://your-server.com/atlas-webhook"
    headers:
      Authorization: "Bearer ${WEBHOOK_TOKEN}"
    timeout: 10s
    retry_count: 3

# LLM configuration
llm:
  provider: "claude"      # "claude", "openai", or "ollama"

  claude:
    api_key: "${ANTHROPIC_API_KEY}"
    model: "claude-sonnet-4-20250514"
    max_tokens: 4096

  openai:
    api_key: "${OPENAI_API_KEY}"
    model: "gpt-4o"
    max_tokens: 4096

  ollama:
    endpoint: "http://localhost:11434"
    model: "qwen2.5:32b"
    timeout: 60s

# LLM meta-strategies
meta:
  arbitrator:
    enabled: true
    context_days: 7       # Days of context for arbitration

  synthesizer:
    enabled: false
    schedule: "0 0 * * 0" # Cron: weekly on Sunday
    min_trades: 50        # Minimum trades for analysis

# Broker configuration
broker:
  enabled: false
  provider: "mock"        # "mock" or "futu"

  futu:
    host: "127.0.0.1"
    port: 11111
    env: "simulate"       # "simulate" or "real"
    trade_password: "${FUTU_TRADE_PWD}"

# Storage configuration
storage:
  hot:
    dsn: ""               # Empty = in-memory, or PostgreSQL DSN
    retention_days: 90

  cold:
    type: "localfs"       # "localfs" or "s3"

    localfs:
      path: "./data/archive"

    s3:
      bucket: "atlas-archive"
      endpoint: "https://s3.amazonaws.com"
      region: "us-east-1"
      access_key: "${AWS_ACCESS_KEY_ID}"
      secret_key: "${AWS_SECRET_ACCESS_KEY}"
      prefix: "atlas/"

# Asset watchlist
watchlist:
  - symbol: "AAPL"
    name: "Apple Inc"
    strategies: ["ma_crossover"]

  - symbol: "600519.SH"
    name: "Kweichow Moutai"
    strategies: ["ma_crossover", "pe_band"]

  - symbol: "0700.HK"
    name: "Tencent Holdings"
    strategies: ["ma_crossover"]
```

---

## Data Types

### Market

```go
type Market string

const (
    MarketUS   Market = "US"    // US stocks
    MarketHK   Market = "HK"    // Hong Kong stocks
    MarketCNA  Market = "CN_A"  // China A-shares
)
```

### Action

```go
type Action string

const (
    ActionBuy  Action = "BUY"
    ActionSell Action = "SELL"
    ActionHold Action = "HOLD"
)
```

### Signal

```go
type Signal struct {
    Symbol     string    `json:"symbol"`
    Name       string    `json:"name"`
    Market     Market    `json:"market"`
    Action     Action    `json:"action"`
    Confidence float64   `json:"confidence"`
    Strategy   string    `json:"strategy"`
    Reason     string    `json:"reason"`
    Price      float64   `json:"price"`
    Timestamp  time.Time `json:"timestamp"`
    Metadata   map[string]interface{} `json:"metadata,omitempty"`
}
```

### Quote

```go
type Quote struct {
    Symbol    string    `json:"symbol"`
    Price     float64   `json:"price"`
    Open      float64   `json:"open"`
    High      float64   `json:"high"`
    Low       float64   `json:"low"`
    Close     float64   `json:"close"`
    Volume    int64     `json:"volume"`
    Timestamp time.Time `json:"timestamp"`
}
```

### OHLCV

```go
type OHLCV struct {
    Date   time.Time `json:"date"`
    Open   float64   `json:"open"`
    High   float64   `json:"high"`
    Low    float64   `json:"low"`
    Close  float64   `json:"close"`
    Volume int64     `json:"volume"`
}
```

### Position

```go
type Position struct {
    Symbol       string  `json:"symbol"`
    Market       Market  `json:"market"`
    Quantity     int     `json:"quantity"`
    AvgCost      float64 `json:"avg_cost"`
    MarketValue  float64 `json:"market_value"`
    UnrealizedPL float64 `json:"unrealized_pl"`
}
```

### Order

```go
type Order struct {
    OrderID   string    `json:"order_id"`
    Symbol    string    `json:"symbol"`
    Market    Market    `json:"market"`
    Side      string    `json:"side"`      // "buy" or "sell"
    Type      string    `json:"type"`      // "market" or "limit"
    Quantity  int       `json:"quantity"`
    Price     float64   `json:"price"`
    Status    string    `json:"status"`    // "open", "filled", "cancelled"
    CreatedAt time.Time `json:"created_at"`
}
```

### Trade

```go
type Trade struct {
    TradeID   string    `json:"trade_id"`
    Symbol    string    `json:"symbol"`
    Side      string    `json:"side"`
    Quantity  int       `json:"quantity"`
    Price     float64   `json:"price"`
    Fee       float64   `json:"fee"`
    Timestamp time.Time `json:"timestamp"`
}
```

---

## Error Responses

All API errors follow this format:

```json
{
  "error": {
    "code": "INVALID_SYMBOL",
    "message": "Symbol 'XYZ' not found",
    "details": {
      "symbol": "XYZ"
    }
  }
}
```

### Error Codes

| Code | HTTP Status | Description |
|------|-------------|-------------|
| `INVALID_REQUEST` | 400 | Malformed request body |
| `INVALID_SYMBOL` | 400 | Unknown symbol |
| `INVALID_STRATEGY` | 400 | Unknown strategy |
| `INVALID_DATE_RANGE` | 400 | Invalid date range |
| `NOT_FOUND` | 404 | Resource not found |
| `RATE_LIMITED` | 429 | Too many requests |
| `INTERNAL_ERROR` | 500 | Server error |
| `BROKER_ERROR` | 503 | Broker connection error |
| `LLM_ERROR` | 503 | LLM provider error |

---

## Rate Limits

| Endpoint | Limit |
|----------|-------|
| `/api/signals` | 100 req/min |
| `/api/backtest` | 10 req/min |
| `/api/meta/*` | 20 req/min |
| `/api/broker/*` | 60 req/min |
| WebSocket | 1 connection/IP |

Exceeded limits return:

```json
{
  "error": {
    "code": "RATE_LIMITED",
    "message": "Rate limit exceeded",
    "details": {
      "retry_after": 30
    }
  }
}
```
