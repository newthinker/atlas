# User Manual

This guide explains how to use ATLAS for trading signal generation and portfolio monitoring.

## Table of Contents

- [Overview](#overview)
- [Watchlist Management](#watchlist-management)
- [Trading Strategies](#trading-strategies)
- [Signal Routing](#signal-routing)
- [Notifications](#notifications)
- [Backtesting](#backtesting)
- [LLM Meta-Strategies](#llm-meta-strategies)
- [Broker Integration](#broker-integration)
- [Web Dashboard](#web-dashboard)

---

## Overview

ATLAS monitors assets on your watchlist, runs trading strategies against them, and sends notifications when signals are generated. The workflow is:

```
Watchlist → Data Collection → Strategy Analysis → Signal Router → Notifications
```

---

## Watchlist Management

### Adding Assets

Define assets to monitor in `config.yaml`:

```yaml
watchlist:
  - symbol: "AAPL"
    name: "Apple Inc"
    strategies: ["ma_crossover"]

  - symbol: "600519.SH"
    name: "Kweichow Moutai"
    strategies: ["ma_crossover", "pe_band", "dividend_yield"]

  - symbol: "0700.HK"
    name: "Tencent Holdings"
    strategies: ["ma_crossover"]
```

### Symbol Formats

| Market | Format | Example |
|--------|--------|---------|
| US | Ticker | `AAPL`, `MSFT`, `GOOGL` |
| Hong Kong | Code.HK | `0700.HK`, `9988.HK` |
| China A-Shares | Code.SH/SZ | `600519.SH`, `000001.SZ` |

### Strategy Assignment

Each asset can have multiple strategies assigned. Only assigned strategies will generate signals for that asset.

---

## Trading Strategies

### MA Crossover (Technical)

Generates signals based on moving average crossovers.

**Configuration:**

```yaml
strategies:
  ma_crossover:
    enabled: true
    params:
      fast_period: 50    # Short-term MA period
      slow_period: 200   # Long-term MA period
      ma_type: "sma"     # "sma" or "ema"
```

**Signals:**

| Condition | Signal | Confidence |
|-----------|--------|------------|
| Fast MA crosses above Slow MA | BUY (Golden Cross) | 0.7-0.9 |
| Fast MA crosses below Slow MA | SELL (Death Cross) | 0.7-0.9 |

### PE Band (Fundamental)

Generates signals when P/E ratio falls below historical percentiles.

**Configuration:**

```yaml
strategies:
  pe_band:
    enabled: true
    params:
      lookback_years: 5       # Historical data period
      threshold_percentile: 20  # Buy when PE below this percentile
```

**Signals:**

| Condition | Signal | Confidence |
|-----------|--------|------------|
| PE below 20th percentile | BUY | 0.6-0.8 |
| PE above 80th percentile | SELL | 0.6-0.8 |

**Requirements:**
- Lixinger API key (for China A-shares fundamentals)

### Dividend Yield (Fundamental)

Generates signals for high dividend yield stocks.

**Configuration:**

```yaml
strategies:
  dividend_yield:
    enabled: true
    params:
      min_yield: 3.0          # Minimum dividend yield %
      min_payout_years: 5     # Minimum consecutive dividend years
      max_payout_ratio: 80    # Maximum payout ratio %
```

**Signals:**

| Condition | Signal | Confidence |
|-----------|--------|------------|
| High yield + stable payout | BUY | 0.6-0.8 |

---

## Signal Routing

The signal router filters and deduplicates signals before sending to notifiers.

### Filters

**Cooldown Filter:**
Prevents repeated signals for the same asset within a time window.

```yaml
router:
  cooldown_hours: 4  # Don't repeat same signal within 4 hours
```

**Confidence Threshold:**
Only passes signals above a minimum confidence level.

```yaml
router:
  min_confidence: 0.6  # Only pass signals with confidence >= 60%
```

### Signal Flow

```
Strategy Signal
      ↓
  Cooldown Filter (deduplicate)
      ↓
  Confidence Filter (quality gate)
      ↓
  Notifiers (Telegram, Email, Webhook)
```

---

## Notifications

### Telegram

Receive real-time signals via Telegram bot.

**Setup:**

1. Create a bot with [@BotFather](https://t.me/botfather)
2. Get your chat ID (send `/start` to [@userinfobot](https://t.me/userinfobot))
3. Configure:

```yaml
notifiers:
  telegram:
    enabled: true
    bot_token: "${TELEGRAM_BOT_TOKEN}"
    chat_id: "${TELEGRAM_CHAT_ID}"
```

**Message Format:**

```
🟢 BUY Signal: AAPL (Apple Inc)
Strategy: ma_crossover
Confidence: 85%
Reason: Golden Cross - MA50 crossed above MA200
Price: $178.50
```

### Email

Receive daily/weekly digest emails.

**Configuration:**

```yaml
notifiers:
  email:
    enabled: true
    host: "smtp.gmail.com"
    port: 587
    username: "your@gmail.com"
    password: "${EMAIL_PASSWORD}"
    from: "ATLAS <atlas@example.com>"
    to:
      - "you@example.com"
```

### Webhook

Send signals to any HTTP endpoint.

**Configuration:**

```yaml
notifiers:
  webhook:
    enabled: true
    url: "https://your-server.com/atlas-webhook"
    headers:
      Authorization: "Bearer ${WEBHOOK_TOKEN}"
```

**Payload:**

```json
{
  "symbol": "AAPL",
  "action": "BUY",
  "confidence": 0.85,
  "strategy": "ma_crossover",
  "reason": "Golden Cross - MA50 crossed above MA200",
  "timestamp": "2024-12-30T10:30:00Z"
}
```

---

## Backtesting

Test strategies against historical data before using them live.

### Running a Backtest

```bash
atlas backtest ma_crossover \
  --symbol AAPL \
  --from 2023-01-01 \
  --to 2024-01-01
```

### Output

```
Backtest Results: ma_crossover on AAPL
Period: 2023-01-01 to 2024-01-01
─────────────────────────────────
Total Trades:    12
Winning Trades:  8
Losing Trades:   4
Win Rate:        66.7%
Total Return:    24.5%
Max Drawdown:    -8.2%
Sharpe Ratio:    1.45
```

### Web UI Backtesting

1. Navigate to http://localhost:8080/backtest
2. Select strategy, symbol, and date range
3. Click "Run Backtest"
4. View results and trade history

---

## LLM Meta-Strategies

ATLAS uses LLMs for advanced signal analysis.

### Signal Arbitrator

When multiple strategies generate conflicting signals for the same asset, the Arbitrator uses an LLM to resolve the conflict.

**What it considers:**
- Market regime (bull/bear/sideways)
- Historical accuracy of each strategy
- Recent news and sentiment
- Signal confidence levels

**Configuration:**

```yaml
llm:
  provider: claude  # or "openai", "ollama"
  claude:
    api_key: "${ANTHROPIC_API_KEY}"
    model: "claude-sonnet-4-20250514"

meta:
  arbitrator:
    enabled: true
    context_days: 7  # Days of news/context to consider
```

**Example:**
```
Conflicting Signals for 600519.SH:
- ma_crossover: SELL (confidence: 0.75)
- pe_band: BUY (confidence: 0.82)

Arbitrator Decision: HOLD
Reasoning: PE band suggests undervaluation, but technical weakness
indicates poor entry timing. Wait for MA confirmation.
```

### Strategy Synthesizer

Analyzes historical trading performance to suggest improvements.

**What it suggests:**
- Parameter adjustments (e.g., "increase MA period from 50 to 60")
- New trading rules based on patterns
- Signal combination rules

**Configuration:**

```yaml
meta:
  synthesizer:
    enabled: true
    schedule: "0 0 * * 0"  # Run weekly
    min_trades: 50          # Minimum trades for analysis
```

**Run manually:**

```bash
atlas synthesize --from 2024-01-01
```

### LLM Providers

**Claude (Recommended):**
```yaml
llm:
  provider: claude
  claude:
    api_key: "${ANTHROPIC_API_KEY}"
    model: "claude-sonnet-4-20250514"
```

**OpenAI:**
```yaml
llm:
  provider: openai
  openai:
    api_key: "${OPENAI_API_KEY}"
    model: "gpt-4o"
```

**Ollama (Local):**
```yaml
llm:
  provider: ollama
  ollama:
    endpoint: "http://localhost:11434"
    model: "qwen2.5:32b"
```

---

## Broker Integration

ATLAS can connect to brokers for portfolio monitoring.

### Viewing Positions

```bash
atlas broker positions
```

Output:
```
SYMBOL     MARKET  QTY  AVG COST  MKT VALUE  P&L
------     ------  ---  --------  ---------  ---
AAPL       US      100  150.00    17500.00   +2500.00
600519.SH  CN_A    50   1800.00   95000.00   +5000.00
```

### Account Summary

```bash
atlas broker account
```

Output:
```
Account Summary
---------------
Total Assets:    $150,000.00
Cash:            $37,500.00
Buying Power:    $50,000.00
Margin Used:     $0.00
Day Trades Left: 3
```

### Trade History

```bash
atlas broker history --from 2024-01-01 --to 2024-12-31
```

### Configuration

```yaml
broker:
  enabled: true
  provider: mock  # or "futu" when implemented
  futu:
    host: "127.0.0.1"
    port: 11111
    env: simulate  # or "real"
```

**Note:** Currently only the mock broker is implemented. Futu integration is planned for future releases.

---

## Web Dashboard

Access the web dashboard at http://localhost:8080

### Dashboard

Overview of recent signals, positions, and system status.

### Signals Page

View and filter trading signals:
- Filter by symbol, strategy, action
- View signal history
- Signal details with reasoning

### Watchlist Page

Manage your monitored assets:
- Add/remove symbols
- Assign strategies
- View current prices

### Backtest Page

Run backtests through the web interface:
- Select strategy and parameters
- Choose date range
- View results with charts

---

## Tips and Best Practices

### Strategy Combination

Use multiple strategies for better signal quality:

```yaml
watchlist:
  - symbol: "600519.SH"
    strategies: ["ma_crossover", "pe_band"]  # Technical + Fundamental
```

### Confidence Thresholds

Start with higher thresholds to reduce noise:

```yaml
router:
  min_confidence: 0.7  # Only high-confidence signals
```

Lower gradually as you gain trust in the system.

### Backtesting Before Live

Always backtest strategies on your specific assets before enabling notifications:

```bash
# Test each strategy
atlas backtest ma_crossover --symbol AAPL --from 2020-01-01 --to 2024-01-01
atlas backtest pe_band --symbol 600519.SH --from 2020-01-01 --to 2024-01-01
```

### LLM Costs

LLM arbitration uses API calls. To control costs:
- Set higher confidence thresholds to reduce conflicts
- Use Ollama for local, free inference
- Disable arbitration if not needed

```yaml
meta:
  arbitrator:
    enabled: false  # Disable to save costs
```

---

## Troubleshooting

### No Signals Generated

1. Check strategies are enabled in config
2. Verify symbols are in watchlist with strategies assigned
3. Lower confidence threshold temporarily
4. Check logs for errors: `./bin/atlas serve --debug`

### Missing Fundamental Data

For PE Band and Dividend Yield strategies on China A-shares:
1. Ensure Lixinger is configured with valid API key
2. Check API quota hasn't been exceeded

### Telegram Notifications Not Received

1. Verify bot token is correct
2. Ensure bot is added to chat
3. Check chat ID is correct (negative for groups)
4. Test: `curl "https://api.telegram.org/bot${TOKEN}/sendMessage?chat_id=${CHAT_ID}&text=test"`
