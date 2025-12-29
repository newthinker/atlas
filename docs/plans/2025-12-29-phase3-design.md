# ATLAS Phase 3 Design

## Overview

Phase 3 adds three components to ATLAS:
1. **Backtesting Framework** - Validate strategies on historical data
2. **Web UI** - Dashboard for monitoring and management
3. **S3 Cold Storage** - Cloud-based archival option

## Component 1: Backtesting Framework

**Purpose:** Run strategies against historical OHLCV data to validate performance before deployment.

### Interface

```go
// internal/backtest/backtest.go

type Backtester struct {
    storage  storage.OHLCVReader
    strategy strategy.Strategy
}

type BacktestResult struct {
    Strategy     string
    Symbol       string
    StartDate    time.Time
    EndDate      time.Time
    Signals      []core.Signal    // All generated signals
    Trades       []Trade          // Simulated trades from signals
    Stats        BacktestStats    // Summary statistics
}

type BacktestStats struct {
    TotalTrades   int
    WinRate       float64  // Percentage of profitable trades
    TotalReturn   float64  // Net return percentage
    MaxDrawdown   float64  // Largest peak-to-trough decline
    SharpeRatio   float64  // Risk-adjusted return
}

type Trade struct {
    EntrySignal   core.Signal
    ExitSignal    *core.Signal  // nil if still open
    EntryPrice    float64
    ExitPrice     float64
    Return        float64
}

func (b *Backtester) Run(ctx context.Context, symbol string, start, end time.Time) (*BacktestResult, error)
```

### Usage

**CLI:**
```bash
atlas backtest ma_crossover --symbol AAPL --from 2023-01-01 --to 2024-01-01
```

**API:**
```
POST /api/v1/strategies/ma_crossover/backtest
{
  "symbol": "AAPL",
  "start": "2023-01-01",
  "end": "2024-01-01"
}
```

### Data Flow

1. Load historical OHLCV from storage
2. Run strategy's `Analyze()` on each data point
3. Collect generated signals
4. Simulate trades (buy signal → sell signal = trade)
5. Calculate summary statistics

---

## Component 2: Web UI

**Stack:** Go `embed` + `html/template` + HTMX + Tailwind CSS (CDN)

### Routes

| Route | Purpose |
|-------|---------|
| `/` | Dashboard - recent signals, system status |
| `/signals` | Signal history with filters (symbol, strategy, date) |
| `/watchlist` | Manage watched symbols and their strategies |
| `/backtest` | Run backtests, view results |
| `/settings` | Configure notifiers, router thresholds |

### Features

- **Live updates** - WebSocket pushes new signals to dashboard
- **HTMX interactions** - Add/remove watchlist items, run backtest inline
- **Minimal JS** - Only HTMX + Chart.js for visualizations
- **Single binary** - Templates embedded via `go:embed`

### Structure

```
internal/api/
├── server.go          # Add template rendering
├── handler/
│   ├── api/           # JSON API handlers (existing)
│   └── web/           # HTML handlers (new)
│       ├── dashboard.go
│       ├── signals.go
│       ├── watchlist.go
│       ├── backtest.go
│       └── settings.go
└── templates/
    ├── layout.html    # Base layout with nav
    ├── partials/      # HTMX partial templates
    ├── dashboard.html
    ├── signals.html
    ├── watchlist.html
    ├── backtest.html
    └── settings.html

static/
├── css/               # Custom styles (if needed)
└── js/                # Minimal custom JS
```

### Styling

Tailwind CSS via CDN - no build step required:
```html
<script src="https://cdn.tailwindcss.com"></script>
```

---

## Component 3: S3 Cold Storage

**Purpose:** Archive historical data to any S3-compatible storage.

### Interface

Implements existing `ArchiveStorage` interface:

```go
// internal/storage/archive/s3.go

type S3Storage struct {
    client   *s3.Client
    bucket   string
    prefix   string
}

func New(cfg S3Config) (*S3Storage, error)

func (s *S3Storage) Write(ctx context.Context, path string, data []byte) error
func (s *S3Storage) Read(ctx context.Context, path string) ([]byte, error)
func (s *S3Storage) List(ctx context.Context, prefix string) ([]string, error)
func (s *S3Storage) Delete(ctx context.Context, path string) error
```

### Configuration

```yaml
storage:
  cold:
    type: s3
    bucket: "atlas-archive"
    endpoint: "https://s3.amazonaws.com"     # or MinIO/R2/B2 URL
    region: "us-east-1"
    access_key: "${AWS_ACCESS_KEY_ID}"
    secret_key: "${AWS_SECRET_ACCESS_KEY}"
    prefix: "archive/"
```

### Compatibility

Works with any S3-compatible service:
- AWS S3
- MinIO (self-hosted)
- Cloudflare R2
- Backblaze B2

### File Structure

```
s3://bucket/prefix/
├── ohlcv/
│   ├── CN_A/
│   │   └── 2024/
│   │       └── 01/
│   │           └── 600519.parquet
│   └── US/
│       └── 2024/
├── fundamentals/
│   └── CN_A/
└── signals/
    └── 2024/
```

**Format:** Parquet (columnar, compressed, efficient for time-series)

---

## Implementation Order

| Order | Component | Rationale |
|-------|-----------|-----------|
| 1 | S3 Storage | Small, self-contained, extends existing interface |
| 2 | Backtesting | Needs storage access, foundational for validation |
| 3 | Web UI | Largest scope, integrates with backtest results |

---

## Dependencies

```go
// New go.mod additions
github.com/aws/aws-sdk-go-v2
github.com/aws/aws-sdk-go-v2/config
github.com/aws/aws-sdk-go-v2/service/s3
github.com/xitongsys/parquet-go       // Parquet read/write
```

Frontend (CDN, no Go deps):
- HTMX
- Tailwind CSS
- Chart.js (optional, for visualizations)

---

## Testing Strategy

| Component | Approach |
|-----------|----------|
| S3 Storage | Mock S3 client; integration test with MinIO container |
| Backtesting | Unit test stats; synthetic OHLCV data |
| Web UI | `httptest` handlers; template rendering tests |

---

## Success Criteria

- [ ] S3 storage passes read/write/list/delete tests
- [ ] Backtest CLI produces valid stats for ma_crossover
- [ ] Web dashboard displays live signals via WebSocket
- [ ] All tests pass, `go vet` clean
