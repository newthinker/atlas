# Crypto Collector Design

## Overview

Implementation of a cryptocurrency data collector with multi-source support and automatic fallback.

## Requirements

- Default quote currency: USDT (when not specified)
- Internal symbol format: `BTCUSDT` (normalized from various inputs)
- Multi-data source with automatic fallback
- Configurable provider priority

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    Crypto Collector                      │
│  ┌─────────────────────────────────────────────────┐    │
│  │              Provider Interface                  │    │
│  │  - FetchQuote(symbol) → Quote                   │    │
│  │  - FetchHistory(symbol, start, end) → []OHLCV   │    │
│  └─────────────────────────────────────────────────┘    │
│                          │                               │
│         ┌────────────────┼────────────────┐             │
│         ▼                ▼                ▼             │
│  ┌───────────┐    ┌───────────┐    ┌───────────┐       │
│  │  Binance  │    │ CoinGecko │    │    OKX    │       │
│  │ (default) │    │ (fallback)│    │ (fallback)│       │
│  └───────────┘    └───────────┘    └───────────┘       │
│                          │                               │
│  ┌─────────────────────────────────────────────────┐    │
│  │           Symbol Normalizer                      │    │
│  │  BTC → BTCUSDT | btc-usdt → BTCUSDT             │    │
│  └─────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────┘
```

## Core Components

### 1. Market Constant

Add to `internal/core/types.go`:
```go
const (
    MarketCrypto Market = "CRYPTO"
)
```

### 2. Provider Interface

`internal/collector/crypto/provider.go`:
```go
type Provider interface {
    Name() string
    FetchQuote(symbol string) (*core.Quote, error)
    FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error)
    SupportsSymbol(symbol string) bool
}
```

### 3. Symbol Normalizer

`internal/collector/crypto/symbol.go`:
- `NormalizeSymbol(input, defaultQuote)` - Convert various formats to BTCUSDT
- `ParseSymbol(symbol)` - Extract base and quote (BTC, USDT)
- `FormatDisplay(symbol)` - Format for display as BTC/USDT

### 4. Data Providers

| Provider | API | Rate Limit | Auth |
|----------|-----|------------|------|
| Binance | api.binance.com | 1200/min | None |
| CoinGecko | api.coingecko.com | 10-30/min | Optional |
| OKX | okx.com | 20/2s | None |

### 5. Fallback Logic

```go
func (c *CryptoCollector) FetchQuote(symbol string) (*core.Quote, error) {
    normalized := NormalizeSymbol(symbol, c.defaultQuote)

    var lastErr error
    for _, p := range c.providers {
        quote, err := p.FetchQuote(normalized)
        if err == nil {
            quote.Source = "crypto:" + p.Name()
            return quote, nil
        }
        lastErr = err
    }
    return nil, fmt.Errorf("all providers failed: %w", lastErr)
}
```

## File Structure

```
internal/collector/crypto/
├── crypto.go           # CryptoCollector main entry
├── provider.go         # Provider interface
├── symbol.go           # Symbol normalization
├── binance/
│   └── binance.go      # Binance implementation
├── coingecko/
│   └── coingecko.go    # CoinGecko implementation
└── okx/
    └── okx.go          # OKX implementation
```

## Configuration

```yaml
collectors:
  crypto:
    enabled: true
    default_quote: "USDT"
    providers:
      - binance
      - coingecko
      - okx
    binance:
      base_url: "https://api.binance.com"
    coingecko:
      api_key: ""
    okx:
      base_url: "https://www.okx.com"
```

## Symbol Normalization Examples

| Input | Output |
|-------|--------|
| BTC | BTCUSDT |
| btc | BTCUSDT |
| BTC-USDT | BTCUSDT |
| BTC/USDT | BTCUSDT |
| btcusdt | BTCUSDT |
| ETH | ETHUSDT |

## Error Handling

```go
var (
    ErrSymbolNotFound = errors.New("symbol not found")
    ErrRateLimited    = errors.New("rate limited")
    ErrProviderDown   = errors.New("provider unavailable")
)
```

Only retryable errors trigger fallback; permanent errors return immediately.
