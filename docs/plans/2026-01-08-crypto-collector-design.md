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

> **未实现，实施时裁剪**：实际 `internal/collector/crypto/provider.go` 的 `Provider`
> 接口只有 `Name/FetchQuote/FetchHistory` 三个方法，**没有 `SupportsSymbol`**——
> fallback 靠遍历所有 provider 试错，不预筛支持的 symbol。若无明确需求勿补此方法。

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

> **未实现，实施时裁剪**：crypto 包**未定义**上述 sentinel 错误。现状 provider
> 直接返回原始 `error`，collector 用 `fmt.Errorf("all providers failed: %w", lastErr)`
> 包裹最后一个错误（`internal/collector/crypto/crypto.go`）。core 包虽有通用
> `core.ErrSymbolNotFound`，但它是 `*core.Error` 而非此处的 `errors.New`，二者不通用。

Only retryable errors trigger fallback; permanent errors return immediately.

> **未实现，实施时裁剪**：实际 `FetchQuote`/`FetchHistory` **无差别遍历所有 provider**，
> 任何错误都继续 fallback 到下一个源，不区分「可重试 vs 永久」——没有错误分类逻辑。
> 若无监控数据佐证需要短路永久错误，勿引入此复杂度。
