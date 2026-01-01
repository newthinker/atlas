# M4: Live Trading Design

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Enable ATLAS to execute trades through FUTU broker based on strategy signals, with paper trading first and risk controls.

**Architecture:** Signal-driven execution with configurable modes (auto/confirm/batch), position tracking, and risk management layer.

**Tech Stack:** Go, FUTU OpenAPI, existing notifier infrastructure

---

## Overview

### Scope
- Paper trading first, graduate to real after validation
- FUTU broker integration (HK + US markets)
- Order types: Market, Limit, Stop
- Configurable execution modes
- Minimal risk controls (expandable later)

### Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      ATLAS Server                           │
├─────────────────────────────────────────────────────────────┤
│  Signal Router                                              │
│       │                                                     │
│       ▼                                                     │
│  ┌─────────────────┐                                        │
│  │ExecutionManager │◄── Config (mode: auto/confirm/batch)   │
│  └────────┬────────┘                                        │
│           │                                                 │
│           ▼                                                 │
│  ┌─────────────────┐     ┌─────────────────┐               │
│  │  RiskChecker    │────►│ PositionTracker │               │
│  └────────┬────────┘     └────────┬────────┘               │
│           │                       │                         │
│           ▼                       ▼                         │
│  ┌─────────────────────────────────────────┐               │
│  │            Broker Interface             │               │
│  │  ┌──────────┐  ┌──────────┐            │               │
│  │  │FutuBroker│  │MockBroker│            │               │
│  │  └──────────┘  └──────────┘            │               │
│  └─────────────────────────────────────────┘               │
└─────────────────────────────────────────────────────────────┘
                         │
                         ▼
              ┌─────────────────┐
              │   FUTU OpenD    │
              │  (Paper/Real)   │
              └─────────────────┘
```

---

## Components

### 1. Broker Interface (`internal/broker/broker.go`)

```go
type Broker interface {
    Connect(ctx context.Context) error
    Disconnect() error

    PlaceOrder(ctx context.Context, req OrderRequest) (*Order, error)
    CancelOrder(ctx context.Context, orderID string) error
    GetOrder(ctx context.Context, orderID string) (*Order, error)

    GetPositions(ctx context.Context) ([]Position, error)
    GetAccountBalance(ctx context.Context) (*Balance, error)

    Subscribe(handler OrderUpdateHandler) // Real-time order updates
}
```

### 2. FutuBroker (`internal/broker/futu/`)

Wraps FUTU OpenAPI:
- Connection management with auto-reconnect
- Order placement with market-specific handling (HK vs US)
- Position synchronization
- Real-time order status updates via callback

### 3. ExecutionManager (`internal/broker/execution.go`)

Handles signal-to-order flow:
- **Auto mode**: Execute immediately after risk check
- **Confirm mode**: Notify user, wait for approval via API
- **Batch mode**: Queue orders, execute at configured time

### 4. RiskChecker (`internal/broker/risk.go`)

Pre-trade validation:
- Position size limits (% of portfolio)
- Daily loss limits
- Maximum open positions
- Returns clear rejection reason if blocked

### 5. PositionTracker (`internal/broker/position.go`)

Maintains current state:
- Syncs positions from FUTU on startup
- Updates on order fills
- Calculates unrealized P&L
- Provides data for risk checks

---

## Data Flow

### Signal → Order Flow

```
1. Signal arrives from strategy
   {symbol: "AAPL", action: BUY, confidence: 0.85}
           │
           ▼
2. ExecutionManager receives signal
   - Check execution mode (auto/confirm/batch)
   - If confirm: notify user, wait for approval
   - If batch: queue for later
           │
           ▼
3. RiskChecker validates
   - Get current positions from PositionTracker
   - Check position size limit
   - Check daily loss limit
   - Check max positions
           │
           ▼
4. Calculate order details
   - Position size based on config (e.g., 2% of portfolio)
   - Order type (market/limit based on signal)
           │
           ▼
5. FutuBroker places order
   - Map to FUTU API format
   - Submit to OpenD
   - Return order ID
           │
           ▼
6. Track order status
   - FUTU callback on fill/reject
   - Update PositionTracker
   - Notify via Telegram
```

### Position Sync Flow

```
Startup:
  FutuBroker.GetPositions() → PositionTracker.Initialize()

On Order Fill:
  FUTU callback → PositionTracker.UpdatePosition()

Periodic (every 5 min):
  FutuBroker.GetPositions() → PositionTracker.Reconcile()
```

---

## Configuration

### Execution Configuration

```yaml
broker:
  enabled: true
  provider: "futu"
  mode: "paper"           # paper, live
  execution:
    mode: "confirm"       # auto, confirm, batch
    batch_time: "09:00"   # For batch mode
    default_size_pct: 2   # 2% of portfolio per position
  futu:
    host: "127.0.0.1"
    port: 11111
    env: "simulate"       # simulate, real
    trade_password: "${FUTU_TRADE_PWD}"
    rsa_key_path: ""
```

### Risk Controls

```yaml
broker:
  risk:
    max_position_pct: 10      # Max 10% in single position
    max_daily_loss_pct: 5     # Stop trading if down 5% today
    max_open_positions: 20    # Max concurrent positions
```

### Validation Rules

- Paper mode: No real money at risk, relaxed limits
- Live mode: All risk checks enforced, requires `env: "real"`
- Mode mismatch (live + simulate) = startup error

---

## Error Handling

| Error | Response |
|-------|----------|
| FUTU connection lost | Retry 3x with backoff, notify, pause execution |
| Order rejected | Log reason, notify, mark signal as failed |
| Risk limit hit | Block order, notify, log which limit |
| Position sync mismatch | Re-sync from FUTU, log discrepancy |

### State Recovery

- On startup: Sync positions from FUTU
- Store pending orders in memory (lost on restart - acceptable for v1)
- Future: Persist order state to database

### Notifications

- Order placed/filled/rejected → Telegram
- Risk limit triggered → Telegram (severity: warning)
- Connection issues → Telegram (severity: critical)

---

## Testing Strategy

### Unit Tests (Mock Broker)

```go
type MockBroker struct {
    orders    []Order
    positions map[string]Position
    fillDelay time.Duration
}

func (m *MockBroker) PlaceOrder(req OrderRequest) (*Order, error)
func (m *MockBroker) GetPositions() ([]Position, error)
func (m *MockBroker) CancelOrder(orderID string) error
```

### Test Cases

| Component | Tests |
|-----------|-------|
| RiskChecker | Position size limits, daily loss limits, max positions |
| ExecutionManager | Auto/confirm/batch modes, order lifecycle |
| PositionTracker | Sync positions, P&L calculation, position updates |
| FutuBroker | Connection handling, order mapping, error cases |

### Integration Tests (Paper Trading)

```go
func TestPaperTradingFlow(t *testing.T) {
    // 1. Connect to FUTU simulate environment
    // 2. Place test order for AAPL
    // 3. Verify order appears in pending
    // 4. Wait for fill (or cancel)
    // 5. Verify position updated
}
```

### Manual Validation Checklist

1. Paper trade single stock (US market)
2. Paper trade HK stock (different settlement)
3. Trigger each risk limit, verify blocking
4. Test connection loss recovery
5. Verify Telegram notifications arrive

### Graduation to Live

- 2 weeks paper trading without issues
- Review all paper trades for correctness
- Start live with single small position
- Gradually increase limits

---

## Implementation Tasks

| Task | Description | Files |
|------|-------------|-------|
| 1 | Core types & interfaces | `internal/broker/types.go` |
| 2 | FUTU broker client | `internal/broker/futu/client.go` |
| 3 | Mock broker for testing | `internal/broker/mock/broker.go` |
| 4 | Risk checker | `internal/broker/risk.go` |
| 5 | Position tracker | `internal/broker/position.go` |
| 6 | Execution manager | `internal/broker/execution.go` |
| 7 | Config & validation | `internal/config/config.go` (extend) |
| 8 | Server integration | `cmd/atlas/serve.go` (wire up) |

### Dependencies

- Task 1 first (types needed by all)
- Tasks 2-5 can parallel after Task 1
- Task 6 needs 2-5 complete
- Tasks 7-8 after Task 6

### Out of Scope (Future)

- Trailing stops
- Bracket orders
- Multi-broker support
- Order persistence/recovery
- Advanced position sizing algorithms
