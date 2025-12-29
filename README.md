# ATLAS

**Asset Tracking & Leadership Analysis System**

A global asset monitoring system with automated trading signal generation. ATLAS supports multiple markets (US, HK, China A-shares), various asset types, and provides intelligent trading signals through technical and fundamental analysis strategies.

## Features

- **Multi-Market Coverage** - US, Hong Kong, China A-shares markets
- **Multiple Strategies** - MA Crossover, PE Band, Dividend Yield, RSI (extensible)
- **LLM Meta-Strategies** - AI-powered signal arbitration and strategy synthesis
- **Broker Integration** - Portfolio positions, orders, trade history (Futu planned)
- **Backtesting** - Test strategies against historical data
- **Multiple Notifiers** - Telegram, Email, Webhook
- **Web Dashboard** - HTMX-powered real-time UI
- **Extensible Architecture** - Plugin-based collectors, strategies, and notifiers

## Quick Start

### Prerequisites

- Go 1.21+
- (Optional) TimescaleDB for production storage

### Installation

```bash
# Clone the repository
git clone https://github.com/newthinker/atlas.git
cd atlas

# Build
go build -o bin/atlas ./cmd/atlas

# Verify installation
./bin/atlas version
```

### Configuration

Copy the example configuration:

```bash
cp config.example.yaml config.yaml
```

Edit `config.yaml` with your settings:

```yaml
server:
  host: "0.0.0.0"
  port: 8080

collectors:
  yahoo:
    enabled: true
    markets: ["US", "HK"]
  eastmoney:
    enabled: true
    markets: ["CN_A"]

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

watchlist:
  - symbol: "AAPL"
    name: "Apple Inc"
    strategies: ["ma_crossover"]
  - symbol: "600519.SH"
    name: "Kweichow Moutai"
    strategies: ["ma_crossover", "pe_band"]
```

### Running

```bash
# Start the server
./bin/atlas serve -c config.yaml

# Or with debug logging
./bin/atlas serve -c config.yaml --debug
```

Access the web dashboard at http://localhost:8080

## CLI Commands

```bash
# Server
atlas serve                    # Start the ATLAS server

# Backtesting
atlas backtest ma_crossover \
  --symbol AAPL \
  --from 2024-01-01 \
  --to 2024-12-01

# Broker operations (uses mock broker by default)
atlas broker status            # Check broker connection
atlas broker positions         # List current positions
atlas broker orders            # List recent orders
atlas broker account           # Show account summary
atlas broker history           # Show trade history
```

## Project Structure

```
atlas/
├── cmd/atlas/           # CLI entry point
├── internal/
│   ├── api/             # HTTP server and web UI
│   ├── backtest/        # Backtesting framework
│   ├── broker/          # Broker abstraction
│   ├── collector/       # Data collectors (Yahoo, Eastmoney, Lixinger)
│   ├── config/          # Configuration management
│   ├── context/         # Market context providers
│   ├── core/            # Core types (Quote, OHLCV, Signal)
│   ├── indicator/       # Technical indicators (SMA, EMA)
│   ├── llm/             # LLM providers (Claude, OpenAI, Ollama)
│   ├── meta/            # LLM meta-strategies (Arbitrator, Synthesizer)
│   ├── notifier/        # Notification channels
│   ├── router/          # Signal routing and filtering
│   ├── storage/         # Archive storage (LocalFS, S3)
│   └── strategy/        # Trading strategies
├── docs/
│   ├── deployment.md    # Deployment guide
│   ├── user-manual.md   # User manual
│   ├── api-reference.md # API documentation
│   └── plans/           # Design documents
└── config.example.yaml  # Example configuration
```

## Documentation

- [Deployment Guide](docs/deployment.md) - Production deployment, Docker, environment setup
- [User Manual](docs/user-manual.md) - Detailed usage guide for strategies, signals, and broker
- [API Reference](docs/api-reference.md) - REST API, WebSocket, and configuration schema

## Supported Markets

| Market | Code | Data Source |
|--------|------|-------------|
| US Stocks | `US` | Yahoo Finance |
| Hong Kong | `HK` | Yahoo Finance |
| China A-Shares | `CN_A` | Eastmoney |

## Built-in Strategies

| Strategy | Type | Description |
|----------|------|-------------|
| `ma_crossover` | Technical | Golden/Death cross (MA50/MA200) |
| `pe_band` | Fundamental | PE below historical percentile |
| `dividend_yield` | Fundamental | High yield + stable payout |

## LLM Integration

ATLAS supports LLM-powered meta-strategies:

- **Signal Arbitrator** - Resolves conflicting signals from multiple strategies using market context, news, and strategy track records
- **Strategy Synthesizer** - Analyzes historical performance to suggest parameter tuning and new trading rules

Supported LLM providers:
- Claude (Anthropic)
- OpenAI (GPT-4)
- Ollama (local models)

## License

MIT

## Contributing

Contributions are welcome! Please read the architecture design in `docs/plans/` before submitting PRs.
