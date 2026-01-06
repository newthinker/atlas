# Watchlist Market & Type Labels Design

## Overview

Add Market and Type classification labels to watchlist items, with autocomplete symbol search functionality.

## Requirements

1. Add Market and Type fields to watchlist items
2. Auto-detect market/type based on symbol pattern
3. Display inline with name: "Apple Inc (美股 · 股票)"
4. Autocomplete symbol search as user types
5. Support config.yaml for market/type specification

## Data Model

### Markets (市场)
- A股
- H股
- 美股
- 数字货币

### Types (类型)
- 股票
- 基金
- 债券
- ETF
- 期权
- 期货
- 加密货币

### WatchlistItem Struct

```go
type WatchlistItem struct {
    Symbol     string
    Name       string
    Market     string   // "A股", "H股", "美股", "数字货币"
    Type       string   // "股票", "基金", "债券", "ETF", "期权", "期货", "加密货币"
    Strategies []string
}
```

### Config Schema

```yaml
watchlist:
  - symbol: "600036.SH"
    name: "招商银行"
    market: "A股"
    type: "股票"
    strategies: ["ma_crossover"]
```

## Auto-Detection Logic

Based on symbol pattern:

| Pattern | Market | Type |
|---------|--------|------|
| `.SH` or `.SZ` suffix | A股 | 股票 |
| `.HK` suffix | H股 | 股票 |
| `-USD`, `-USDT`, `BTC`, `ETH` | 数字货币 | 加密货币 |
| No suffix / US pattern | 美股 | 股票 |

## Symbol Search API

### Endpoint

```
GET /api/v1/symbols/search?q=<query>
```

### Logic

- Query starts with digits → Search Eastmoney (A股)
- Query starts with letters → Search Yahoo (US/HK)
- Returns up to 10 results

### Response

```json
{
  "results": [
    {
      "symbol": "600036.SH",
      "name": "招商银行",
      "market": "A股",
      "type": "股票"
    }
  ]
}
```

## UI Design

### Add Symbol Modal

```
Symbol:     [600036.SH          ] <- autocomplete dropdown
Name:       [招商银行            ] <- auto-filled, editable
Market:     [A股 ▼]              <- dropdown, auto-filled
Type:       [股票 ▼]              <- dropdown, auto-filled
Strategies: ☑ ma_crossover

[Cancel] [Add]
```

### Autocomplete Flow

1. User types 2+ characters in symbol field
2. Debounced API call (300ms delay)
3. Show dropdown with search results
4. User selects → auto-fill Symbol, Name, Market, Type
5. User can edit any field before submitting

### Watchlist Table Display

Name column shows: `{Name} ({Market} · {Type})`

Example:
| SYMBOL | NAME | STRATEGIES |
|--------|------|------------|
| AAPL | Apple Inc (美股 · 股票) | ma_crossover |
| 600519.SH | Kweichow Moutai (A股 · 股票) | ma_crossover |
| BTC-USD | Bitcoin (数字货币 · 加密货币) | ma_crossover |

## Files to Modify

### Backend

| File | Changes |
|------|---------|
| `internal/app/app.go` | Add Market, Type fields to WatchlistItem |
| `internal/config/config.go` | Add Market, Type to config struct |
| `internal/api/handler/api/watchlist.go` | Handle new fields in Add/List |
| `internal/api/handler/api/symbols.go` | **New** - Symbol search handler |
| `internal/api/handler/web/handler.go` | Update WatchlistItemData struct |
| `internal/api/handler/web/watchlist.go` | Pass new fields to template |
| `internal/api/server.go` | Register `/api/v1/symbols/search` route |
| `cmd/atlas/serve.go` | Load Market, Type from config |

### Frontend

| File | Changes |
|------|---------|
| `internal/api/handler/web/templates/watchlist.html` | Add dropdowns, autocomplete JS |
| `internal/api/templates/watchlist.html` | Same (external copy) |

## Implementation Order

1. Update data model (app.go, config.go)
2. Update config loading (serve.go)
3. Add symbol search handler (symbols.go)
4. Update API watchlist handler
5. Update web handler and adapter
6. Update templates with dropdowns and autocomplete
7. Test complete flow
