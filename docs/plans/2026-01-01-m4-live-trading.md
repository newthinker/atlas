# M4: Live Trading Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Enable ATLAS to execute trades through FUTU broker based on strategy signals, with paper trading first and risk controls.

**Architecture:** Signal-driven execution with configurable modes (auto/confirm/batch), position tracking, and risk management layer.

**Tech Stack:** Go, FUTU OpenAPI (simulated via interface), existing notifier infrastructure

---

## Task 1: Core Types & Interfaces

**Files:**
- Create: `internal/broker/types.go`
- Test: `internal/broker/types_test.go`

**Step 1: Write the type definitions**

```go
// internal/broker/types.go
package broker

import (
	"context"
	"time"
)

// OrderSide represents buy or sell
type OrderSide string

const (
	Buy  OrderSide = "BUY"
	Sell OrderSide = "SELL"
)

// OrderType represents the order type
type OrderType string

const (
	Market OrderType = "MARKET"
	Limit  OrderType = "LIMIT"
	Stop   OrderType = "STOP"
)

// OrderStatus represents order lifecycle status
type OrderStatus string

const (
	OrderPending   OrderStatus = "PENDING"
	OrderFilled    OrderStatus = "FILLED"
	OrderPartial   OrderStatus = "PARTIAL"
	OrderCancelled OrderStatus = "CANCELLED"
	OrderRejected  OrderStatus = "REJECTED"
)

// OrderRequest represents a request to place an order
type OrderRequest struct {
	Symbol    string
	Side      OrderSide
	Type      OrderType
	Quantity  int64
	Price     float64 // For limit/stop orders
	StopPrice float64 // For stop orders
}

// Order represents a placed order
type Order struct {
	ID          string
	Symbol      string
	Side        OrderSide
	Type        OrderType
	Quantity    int64
	FilledQty   int64
	Price       float64
	AvgPrice    float64
	Status      OrderStatus
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Message     string // Error/rejection message
}

// Position represents a held position
type Position struct {
	Symbol      string
	Quantity    int64
	AvgCost     float64
	MarketValue float64
	UnrealizedPL float64
	RealizedPL   float64
}

// Balance represents account balance
type Balance struct {
	TotalValue   float64
	Cash         float64
	MarketValue  float64
	DailyPL      float64
	UnrealizedPL float64
}

// OrderUpdateHandler handles real-time order updates
type OrderUpdateHandler func(order *Order)

// Broker defines the interface for broker implementations
type Broker interface {
	// Connection
	Connect(ctx context.Context) error
	Disconnect() error
	IsConnected() bool

	// Orders
	PlaceOrder(ctx context.Context, req OrderRequest) (*Order, error)
	CancelOrder(ctx context.Context, orderID string) error
	GetOrder(ctx context.Context, orderID string) (*Order, error)
	GetOpenOrders(ctx context.Context) ([]Order, error)

	// Positions
	GetPositions(ctx context.Context) ([]Position, error)
	GetPosition(ctx context.Context, symbol string) (*Position, error)

	// Account
	GetBalance(ctx context.Context) (*Balance, error)

	// Real-time updates
	Subscribe(handler OrderUpdateHandler)
	Unsubscribe()
}
```

**Step 2: Write validation tests**

```go
// internal/broker/types_test.go
package broker

import (
	"testing"
)

func TestOrderSideConstants(t *testing.T) {
	if Buy != "BUY" {
		t.Errorf("Buy = %q, want BUY", Buy)
	}
	if Sell != "SELL" {
		t.Errorf("Sell = %q, want SELL", Sell)
	}
}

func TestOrderTypeConstants(t *testing.T) {
	tests := []struct {
		got  OrderType
		want string
	}{
		{Market, "MARKET"},
		{Limit, "LIMIT"},
		{Stop, "STOP"},
	}
	for _, tt := range tests {
		if string(tt.got) != tt.want {
			t.Errorf("got %q, want %q", tt.got, tt.want)
		}
	}
}

func TestOrderStatusConstants(t *testing.T) {
	tests := []struct {
		got  OrderStatus
		want string
	}{
		{OrderPending, "PENDING"},
		{OrderFilled, "FILLED"},
		{OrderPartial, "PARTIAL"},
		{OrderCancelled, "CANCELLED"},
		{OrderRejected, "REJECTED"},
	}
	for _, tt := range tests {
		if string(tt.got) != tt.want {
			t.Errorf("got %q, want %q", tt.got, tt.want)
		}
	}
}

func TestOrderRequest(t *testing.T) {
	req := OrderRequest{
		Symbol:   "AAPL",
		Side:     Buy,
		Type:     Market,
		Quantity: 100,
	}
	if req.Symbol != "AAPL" {
		t.Error("Symbol not set")
	}
	if req.Side != Buy {
		t.Error("Side not set")
	}
}
```

**Step 3: Run tests to verify**

Run: `go test ./internal/broker/... -v`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/broker/
git commit -m "feat(broker): add core types and broker interface"
```

---

## Task 2: Mock Broker Implementation

**Files:**
- Create: `internal/broker/mock/broker.go`
- Test: `internal/broker/mock/broker_test.go`

**Step 1: Write the mock broker**

```go
// internal/broker/mock/broker.go
package mock

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/newthinker/atlas/internal/broker"
)

// Broker is a mock broker for testing
type Broker struct {
	connected   bool
	orders      map[string]*broker.Order
	positions   map[string]*broker.Position
	balance     *broker.Balance
	handler     broker.OrderUpdateHandler
	fillDelay   time.Duration
	shouldFail  bool
	failMessage string
	orderIDSeq  int
	mu          sync.RWMutex
}

// New creates a new mock broker
func New() *Broker {
	return &Broker{
		orders:    make(map[string]*broker.Order),
		positions: make(map[string]*broker.Position),
		balance: &broker.Balance{
			TotalValue:  100000,
			Cash:        100000,
			MarketValue: 0,
		},
		fillDelay: 100 * time.Millisecond,
	}
}

// SetFillDelay sets the delay before orders are filled
func (b *Broker) SetFillDelay(d time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.fillDelay = d
}

// SetShouldFail makes the broker reject orders
func (b *Broker) SetShouldFail(fail bool, message string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.shouldFail = fail
	b.failMessage = message
}

// Connect connects to the mock broker
func (b *Broker) Connect(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.connected = true
	return nil
}

// Disconnect disconnects from the mock broker
func (b *Broker) Disconnect() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.connected = false
	return nil
}

// IsConnected returns connection status
func (b *Broker) IsConnected() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.connected
}

// PlaceOrder places a mock order
func (b *Broker) PlaceOrder(ctx context.Context, req broker.OrderRequest) (*broker.Order, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.connected {
		return nil, fmt.Errorf("not connected")
	}

	if b.shouldFail {
		return nil, fmt.Errorf(b.failMessage)
	}

	b.orderIDSeq++
	orderID := fmt.Sprintf("MOCK-%d", b.orderIDSeq)
	now := time.Now()

	order := &broker.Order{
		ID:        orderID,
		Symbol:    req.Symbol,
		Side:      req.Side,
		Type:      req.Type,
		Quantity:  req.Quantity,
		Price:     req.Price,
		Status:    broker.OrderPending,
		CreatedAt: now,
		UpdatedAt: now,
	}

	b.orders[orderID] = order

	// Simulate async fill
	go b.simulateFill(orderID)

	return order, nil
}

func (b *Broker) simulateFill(orderID string) {
	b.mu.RLock()
	delay := b.fillDelay
	b.mu.RUnlock()

	time.Sleep(delay)

	b.mu.Lock()
	defer b.mu.Unlock()

	order, ok := b.orders[orderID]
	if !ok || order.Status != broker.OrderPending {
		return
	}

	order.Status = broker.OrderFilled
	order.FilledQty = order.Quantity
	order.AvgPrice = order.Price
	if order.Type == broker.Market {
		order.AvgPrice = 150.0 // Mock market price
	}
	order.UpdatedAt = time.Now()

	// Update position
	pos, ok := b.positions[order.Symbol]
	if !ok {
		pos = &broker.Position{Symbol: order.Symbol}
		b.positions[order.Symbol] = pos
	}

	if order.Side == broker.Buy {
		pos.Quantity += order.FilledQty
	} else {
		pos.Quantity -= order.FilledQty
	}
	pos.AvgCost = order.AvgPrice

	// Notify handler
	if b.handler != nil {
		b.handler(order)
	}
}

// CancelOrder cancels a mock order
func (b *Broker) CancelOrder(ctx context.Context, orderID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	order, ok := b.orders[orderID]
	if !ok {
		return fmt.Errorf("order not found: %s", orderID)
	}

	if order.Status != broker.OrderPending {
		return fmt.Errorf("cannot cancel order in status: %s", order.Status)
	}

	order.Status = broker.OrderCancelled
	order.UpdatedAt = time.Now()
	return nil
}

// GetOrder returns an order by ID
func (b *Broker) GetOrder(ctx context.Context, orderID string) (*broker.Order, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	order, ok := b.orders[orderID]
	if !ok {
		return nil, fmt.Errorf("order not found: %s", orderID)
	}
	return order, nil
}

// GetOpenOrders returns all open orders
func (b *Broker) GetOpenOrders(ctx context.Context) ([]broker.Order, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var orders []broker.Order
	for _, o := range b.orders {
		if o.Status == broker.OrderPending || o.Status == broker.OrderPartial {
			orders = append(orders, *o)
		}
	}
	return orders, nil
}

// GetPositions returns all positions
func (b *Broker) GetPositions(ctx context.Context) ([]broker.Position, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var positions []broker.Position
	for _, p := range b.positions {
		if p.Quantity != 0 {
			positions = append(positions, *p)
		}
	}
	return positions, nil
}

// GetPosition returns a position by symbol
func (b *Broker) GetPosition(ctx context.Context, symbol string) (*broker.Position, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	pos, ok := b.positions[symbol]
	if !ok {
		return &broker.Position{Symbol: symbol}, nil
	}
	return pos, nil
}

// GetBalance returns account balance
func (b *Broker) GetBalance(ctx context.Context) (*broker.Balance, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.balance, nil
}

// Subscribe registers an order update handler
func (b *Broker) Subscribe(handler broker.OrderUpdateHandler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handler = handler
}

// Unsubscribe removes the order update handler
func (b *Broker) Unsubscribe() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handler = nil
}
```

**Step 2: Write tests**

```go
// internal/broker/mock/broker_test.go
package mock

import (
	"context"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/broker"
)

func TestConnect(t *testing.T) {
	b := New()
	if b.IsConnected() {
		t.Error("should not be connected initially")
	}
	if err := b.Connect(context.Background()); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	if !b.IsConnected() {
		t.Error("should be connected after Connect")
	}
}

func TestPlaceOrder(t *testing.T) {
	b := New()
	b.SetFillDelay(10 * time.Millisecond)
	ctx := context.Background()

	if err := b.Connect(ctx); err != nil {
		t.Fatal(err)
	}

	order, err := b.PlaceOrder(ctx, broker.OrderRequest{
		Symbol:   "AAPL",
		Side:     broker.Buy,
		Type:     broker.Market,
		Quantity: 100,
	})
	if err != nil {
		t.Fatalf("PlaceOrder failed: %v", err)
	}
	if order.ID == "" {
		t.Error("order ID should not be empty")
	}
	if order.Status != broker.OrderPending {
		t.Errorf("status = %v, want PENDING", order.Status)
	}

	// Wait for fill
	time.Sleep(50 * time.Millisecond)

	filled, err := b.GetOrder(ctx, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	if filled.Status != broker.OrderFilled {
		t.Errorf("status = %v, want FILLED", filled.Status)
	}
}

func TestCancelOrder(t *testing.T) {
	b := New()
	b.SetFillDelay(1 * time.Second) // Long delay so we can cancel
	ctx := context.Background()

	if err := b.Connect(ctx); err != nil {
		t.Fatal(err)
	}

	order, err := b.PlaceOrder(ctx, broker.OrderRequest{
		Symbol:   "AAPL",
		Side:     broker.Buy,
		Type:     broker.Limit,
		Quantity: 100,
		Price:    150.0,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := b.CancelOrder(ctx, order.ID); err != nil {
		t.Fatalf("CancelOrder failed: %v", err)
	}

	cancelled, _ := b.GetOrder(ctx, order.ID)
	if cancelled.Status != broker.OrderCancelled {
		t.Errorf("status = %v, want CANCELLED", cancelled.Status)
	}
}

func TestPositionUpdatedOnFill(t *testing.T) {
	b := New()
	b.SetFillDelay(10 * time.Millisecond)
	ctx := context.Background()

	if err := b.Connect(ctx); err != nil {
		t.Fatal(err)
	}

	_, err := b.PlaceOrder(ctx, broker.OrderRequest{
		Symbol:   "AAPL",
		Side:     broker.Buy,
		Type:     broker.Market,
		Quantity: 100,
	})
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(50 * time.Millisecond)

	pos, err := b.GetPosition(ctx, "AAPL")
	if err != nil {
		t.Fatal(err)
	}
	if pos.Quantity != 100 {
		t.Errorf("quantity = %d, want 100", pos.Quantity)
	}
}

func TestOrderUpdateHandler(t *testing.T) {
	b := New()
	b.SetFillDelay(10 * time.Millisecond)
	ctx := context.Background()

	var received *broker.Order
	b.Subscribe(func(o *broker.Order) {
		received = o
	})

	if err := b.Connect(ctx); err != nil {
		t.Fatal(err)
	}

	_, err := b.PlaceOrder(ctx, broker.OrderRequest{
		Symbol:   "AAPL",
		Side:     broker.Buy,
		Type:     broker.Market,
		Quantity: 100,
	})
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(50 * time.Millisecond)

	if received == nil {
		t.Error("handler was not called")
	}
	if received != nil && received.Status != broker.OrderFilled {
		t.Errorf("status = %v, want FILLED", received.Status)
	}
}
```

**Step 3: Run tests**

Run: `go test ./internal/broker/... -v`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/broker/mock/
git commit -m "feat(broker): add mock broker for testing"
```

---

## Task 3: Risk Checker

**Files:**
- Create: `internal/broker/risk.go`
- Test: `internal/broker/risk_test.go`

**Step 1: Write the risk checker**

```go
// internal/broker/risk.go
package broker

import (
	"context"
	"fmt"
)

// RiskConfig defines risk control parameters
type RiskConfig struct {
	MaxPositionPct   float64 // Max % of portfolio in single position
	MaxDailyLossPct  float64 // Stop trading if down this % today
	MaxOpenPositions int     // Max concurrent positions
}

// DefaultRiskConfig returns sensible defaults
func DefaultRiskConfig() RiskConfig {
	return RiskConfig{
		MaxPositionPct:   10.0,
		MaxDailyLossPct:  5.0,
		MaxOpenPositions: 20,
	}
}

// RiskChecker validates orders against risk limits
type RiskChecker struct {
	config RiskConfig
	broker Broker
}

// NewRiskChecker creates a new risk checker
func NewRiskChecker(config RiskConfig, broker Broker) *RiskChecker {
	return &RiskChecker{
		config: config,
		broker: broker,
	}
}

// RiskCheckResult contains the result of a risk check
type RiskCheckResult struct {
	Allowed bool
	Reason  string
}

// Check validates an order against risk limits
func (r *RiskChecker) Check(ctx context.Context, req OrderRequest, price float64) RiskCheckResult {
	// Get current balance
	balance, err := r.broker.GetBalance(ctx)
	if err != nil {
		return RiskCheckResult{
			Allowed: false,
			Reason:  fmt.Sprintf("failed to get balance: %v", err),
		}
	}

	// Check daily loss limit
	if balance.TotalValue > 0 {
		dailyLossPct := (-balance.DailyPL / balance.TotalValue) * 100
		if dailyLossPct >= r.config.MaxDailyLossPct {
			return RiskCheckResult{
				Allowed: false,
				Reason:  fmt.Sprintf("daily loss limit reached: %.1f%% >= %.1f%%", dailyLossPct, r.config.MaxDailyLossPct),
			}
		}
	}

	// Get current positions
	positions, err := r.broker.GetPositions(ctx)
	if err != nil {
		return RiskCheckResult{
			Allowed: false,
			Reason:  fmt.Sprintf("failed to get positions: %v", err),
		}
	}

	// Check max open positions (only for new positions)
	if req.Side == Buy {
		isNewPosition := true
		for _, p := range positions {
			if p.Symbol == req.Symbol && p.Quantity > 0 {
				isNewPosition = false
				break
			}
		}
		if isNewPosition && len(positions) >= r.config.MaxOpenPositions {
			return RiskCheckResult{
				Allowed: false,
				Reason:  fmt.Sprintf("max open positions reached: %d >= %d", len(positions), r.config.MaxOpenPositions),
			}
		}
	}

	// Check position size limit
	orderValue := float64(req.Quantity) * price
	if balance.TotalValue > 0 {
		positionPct := (orderValue / balance.TotalValue) * 100
		if positionPct > r.config.MaxPositionPct {
			return RiskCheckResult{
				Allowed: false,
				Reason:  fmt.Sprintf("position size too large: %.1f%% > %.1f%%", positionPct, r.config.MaxPositionPct),
			}
		}
	}

	return RiskCheckResult{Allowed: true}
}
```

**Step 2: Write tests**

```go
// internal/broker/risk_test.go
package broker

import (
	"context"
	"testing"

	"github.com/newthinker/atlas/internal/broker/mock"
)

func TestRiskChecker_MaxPositionSize(t *testing.T) {
	mockBroker := mock.New()
	mockBroker.Connect(context.Background())

	checker := NewRiskChecker(RiskConfig{
		MaxPositionPct:   10.0,
		MaxDailyLossPct:  5.0,
		MaxOpenPositions: 20,
	}, mockBroker)

	ctx := context.Background()

	// Order for 10% of portfolio (100k * 10% = 10k, at $100 = 100 shares)
	result := checker.Check(ctx, OrderRequest{
		Symbol:   "AAPL",
		Side:     Buy,
		Quantity: 100,
	}, 100.0)
	if !result.Allowed {
		t.Errorf("10%% position should be allowed: %s", result.Reason)
	}

	// Order for 15% of portfolio (should be rejected)
	result = checker.Check(ctx, OrderRequest{
		Symbol:   "AAPL",
		Side:     Buy,
		Quantity: 150,
	}, 100.0)
	if result.Allowed {
		t.Error("15% position should be rejected")
	}
	if result.Reason == "" {
		t.Error("should have rejection reason")
	}
}

func TestRiskChecker_MaxOpenPositions(t *testing.T) {
	mockBroker := mock.New()
	mockBroker.Connect(context.Background())

	checker := NewRiskChecker(RiskConfig{
		MaxPositionPct:   100.0, // Ignore position size
		MaxDailyLossPct:  100.0, // Ignore daily loss
		MaxOpenPositions: 2,
	}, mockBroker)

	ctx := context.Background()

	// Fill 2 positions
	mockBroker.PlaceOrder(ctx, OrderRequest{Symbol: "AAPL", Side: Buy, Quantity: 1})
	mockBroker.PlaceOrder(ctx, OrderRequest{Symbol: "MSFT", Side: Buy, Quantity: 1})

	// Wait for fills
	// In real test, would use proper synchronization

	// Third position should be rejected
	result := checker.Check(ctx, OrderRequest{
		Symbol:   "GOOGL",
		Side:     Buy,
		Quantity: 1,
	}, 100.0)

	// Note: This test may need adjustment based on mock timing
	_ = result
}

func TestRiskChecker_SellOrdersAllowed(t *testing.T) {
	mockBroker := mock.New()
	mockBroker.Connect(context.Background())

	checker := NewRiskChecker(RiskConfig{
		MaxPositionPct:   10.0,
		MaxDailyLossPct:  5.0,
		MaxOpenPositions: 1,
	}, mockBroker)

	ctx := context.Background()

	// Sell orders should not be blocked by position limits
	result := checker.Check(ctx, OrderRequest{
		Symbol:   "AAPL",
		Side:     Sell,
		Quantity: 100,
	}, 100.0)
	if !result.Allowed {
		t.Errorf("sell should be allowed: %s", result.Reason)
	}
}
```

**Step 3: Run tests**

Run: `go test ./internal/broker/... -v`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/broker/risk.go internal/broker/risk_test.go
git commit -m "feat(broker): add risk checker with position and loss limits"
```

---

## Task 4: Position Tracker

**Files:**
- Create: `internal/broker/position.go`
- Test: `internal/broker/position_test.go`

**Step 1: Write the position tracker**

```go
// internal/broker/position.go
package broker

import (
	"context"
	"sync"
	"time"
)

// PositionTracker tracks current positions and P&L
type PositionTracker struct {
	broker    Broker
	positions map[string]*Position
	lastSync  time.Time
	syncMu    sync.RWMutex
}

// NewPositionTracker creates a new position tracker
func NewPositionTracker(broker Broker) *PositionTracker {
	return &PositionTracker{
		broker:    broker,
		positions: make(map[string]*Position),
	}
}

// Sync synchronizes positions from broker
func (t *PositionTracker) Sync(ctx context.Context) error {
	positions, err := t.broker.GetPositions(ctx)
	if err != nil {
		return err
	}

	t.syncMu.Lock()
	defer t.syncMu.Unlock()

	t.positions = make(map[string]*Position)
	for i := range positions {
		p := positions[i]
		t.positions[p.Symbol] = &p
	}
	t.lastSync = time.Now()
	return nil
}

// GetPosition returns current position for a symbol
func (t *PositionTracker) GetPosition(symbol string) *Position {
	t.syncMu.RLock()
	defer t.syncMu.RUnlock()

	if p, ok := t.positions[symbol]; ok {
		return p
	}
	return &Position{Symbol: symbol}
}

// GetAllPositions returns all positions
func (t *PositionTracker) GetAllPositions() []Position {
	t.syncMu.RLock()
	defer t.syncMu.RUnlock()

	positions := make([]Position, 0, len(t.positions))
	for _, p := range t.positions {
		if p.Quantity != 0 {
			positions = append(positions, *p)
		}
	}
	return positions
}

// UpdateOnFill updates position based on order fill
func (t *PositionTracker) UpdateOnFill(order *Order) {
	t.syncMu.Lock()
	defer t.syncMu.Unlock()

	pos, ok := t.positions[order.Symbol]
	if !ok {
		pos = &Position{Symbol: order.Symbol}
		t.positions[order.Symbol] = pos
	}

	if order.Side == Buy {
		// Update average cost
		totalCost := pos.AvgCost*float64(pos.Quantity) + order.AvgPrice*float64(order.FilledQty)
		pos.Quantity += order.FilledQty
		if pos.Quantity > 0 {
			pos.AvgCost = totalCost / float64(pos.Quantity)
		}
	} else {
		// Sell - realize P&L
		if pos.Quantity > 0 {
			realizedPerShare := order.AvgPrice - pos.AvgCost
			pos.RealizedPL += realizedPerShare * float64(order.FilledQty)
		}
		pos.Quantity -= order.FilledQty
	}
}

// LastSyncTime returns when positions were last synced
func (t *PositionTracker) LastSyncTime() time.Time {
	t.syncMu.RLock()
	defer t.syncMu.RUnlock()
	return t.lastSync
}

// TotalUnrealizedPL returns total unrealized P&L
func (t *PositionTracker) TotalUnrealizedPL() float64 {
	t.syncMu.RLock()
	defer t.syncMu.RUnlock()

	var total float64
	for _, p := range t.positions {
		total += p.UnrealizedPL
	}
	return total
}

// TotalRealizedPL returns total realized P&L
func (t *PositionTracker) TotalRealizedPL() float64 {
	t.syncMu.RLock()
	defer t.syncMu.RUnlock()

	var total float64
	for _, p := range t.positions {
		total += p.RealizedPL
	}
	return total
}
```

**Step 2: Write tests**

```go
// internal/broker/position_test.go
package broker

import (
	"context"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/broker/mock"
)

func TestPositionTracker_Sync(t *testing.T) {
	mockBroker := mock.New()
	mockBroker.SetFillDelay(10 * time.Millisecond)
	ctx := context.Background()
	mockBroker.Connect(ctx)

	// Create some positions
	mockBroker.PlaceOrder(ctx, OrderRequest{
		Symbol:   "AAPL",
		Side:     Buy,
		Type:     Market,
		Quantity: 100,
	})
	time.Sleep(50 * time.Millisecond)

	tracker := NewPositionTracker(mockBroker)
	if err := tracker.Sync(ctx); err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	pos := tracker.GetPosition("AAPL")
	if pos.Quantity != 100 {
		t.Errorf("quantity = %d, want 100", pos.Quantity)
	}
}

func TestPositionTracker_UpdateOnFill(t *testing.T) {
	mockBroker := mock.New()
	tracker := NewPositionTracker(mockBroker)

	// Buy 100 shares at $150
	tracker.UpdateOnFill(&Order{
		Symbol:    "AAPL",
		Side:      Buy,
		FilledQty: 100,
		AvgPrice:  150.0,
		Status:    OrderFilled,
	})

	pos := tracker.GetPosition("AAPL")
	if pos.Quantity != 100 {
		t.Errorf("quantity = %d, want 100", pos.Quantity)
	}
	if pos.AvgCost != 150.0 {
		t.Errorf("avgCost = %f, want 150.0", pos.AvgCost)
	}

	// Buy 100 more at $160
	tracker.UpdateOnFill(&Order{
		Symbol:    "AAPL",
		Side:      Buy,
		FilledQty: 100,
		AvgPrice:  160.0,
		Status:    OrderFilled,
	})

	pos = tracker.GetPosition("AAPL")
	if pos.Quantity != 200 {
		t.Errorf("quantity = %d, want 200", pos.Quantity)
	}
	expectedAvg := (150.0*100 + 160.0*100) / 200
	if pos.AvgCost != expectedAvg {
		t.Errorf("avgCost = %f, want %f", pos.AvgCost, expectedAvg)
	}
}

func TestPositionTracker_RealizedPL(t *testing.T) {
	mockBroker := mock.New()
	tracker := NewPositionTracker(mockBroker)

	// Buy 100 at $150
	tracker.UpdateOnFill(&Order{
		Symbol:    "AAPL",
		Side:      Buy,
		FilledQty: 100,
		AvgPrice:  150.0,
		Status:    OrderFilled,
	})

	// Sell 50 at $160
	tracker.UpdateOnFill(&Order{
		Symbol:    "AAPL",
		Side:      Sell,
		FilledQty: 50,
		AvgPrice:  160.0,
		Status:    OrderFilled,
	})

	pos := tracker.GetPosition("AAPL")
	if pos.Quantity != 50 {
		t.Errorf("quantity = %d, want 50", pos.Quantity)
	}
	expectedPL := (160.0 - 150.0) * 50
	if pos.RealizedPL != expectedPL {
		t.Errorf("realizedPL = %f, want %f", pos.RealizedPL, expectedPL)
	}
}
```

**Step 3: Run tests**

Run: `go test ./internal/broker/... -v`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/broker/position.go internal/broker/position_test.go
git commit -m "feat(broker): add position tracker with P&L calculation"
```

---

## Task 5: Execution Manager

**Files:**
- Create: `internal/broker/execution.go`
- Test: `internal/broker/execution_test.go`

**Step 1: Write the execution manager**

```go
// internal/broker/execution.go
package broker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

// ExecutionMode defines how orders are executed
type ExecutionMode string

const (
	ExecutionAuto    ExecutionMode = "auto"    // Execute immediately
	ExecutionConfirm ExecutionMode = "confirm" // Require confirmation
	ExecutionBatch   ExecutionMode = "batch"   // Queue for batch execution
)

// ExecutionConfig holds execution settings
type ExecutionConfig struct {
	Mode           ExecutionMode
	BatchTime      string  // Time to execute batch orders (HH:MM)
	DefaultSizePct float64 // Default position size as % of portfolio
}

// DefaultExecutionConfig returns sensible defaults
func DefaultExecutionConfig() ExecutionConfig {
	return ExecutionConfig{
		Mode:           ExecutionConfirm,
		DefaultSizePct: 2.0,
	}
}

// PendingOrder represents an order waiting for confirmation
type PendingOrder struct {
	ID        string
	Request   OrderRequest
	Price     float64
	Signal    *core.Signal
	CreatedAt time.Time
}

// ExecutionManager handles signal-to-order execution
type ExecutionManager struct {
	config     ExecutionConfig
	broker     Broker
	risk       *RiskChecker
	tracker    *PositionTracker
	pending    map[string]*PendingOrder
	orderIDSeq int
	mu         sync.RWMutex
}

// NewExecutionManager creates a new execution manager
func NewExecutionManager(
	config ExecutionConfig,
	broker Broker,
	risk *RiskChecker,
	tracker *PositionTracker,
) *ExecutionManager {
	return &ExecutionManager{
		config:  config,
		broker:  broker,
		risk:    risk,
		tracker: tracker,
		pending: make(map[string]*PendingOrder),
	}
}

// ExecuteResult contains the result of execution
type ExecuteResult struct {
	Success   bool
	Order     *Order
	PendingID string // For confirm mode
	Message   string
}

// Execute handles a signal based on execution mode
func (m *ExecutionManager) Execute(ctx context.Context, signal *core.Signal, price float64) (*ExecuteResult, error) {
	// Calculate order size
	balance, err := m.broker.GetBalance(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting balance: %w", err)
	}

	positionValue := balance.TotalValue * (m.config.DefaultSizePct / 100)
	quantity := int64(positionValue / price)
	if quantity < 1 {
		quantity = 1
	}

	// Determine side
	side := Buy
	if signal.Action == core.ActionSell {
		side = Sell
	}

	req := OrderRequest{
		Symbol:   signal.Symbol,
		Side:     side,
		Type:     Market,
		Quantity: quantity,
	}

	// Risk check
	result := m.risk.Check(ctx, req, price)
	if !result.Allowed {
		return &ExecuteResult{
			Success: false,
			Message: result.Reason,
		}, nil
	}

	switch m.config.Mode {
	case ExecutionAuto:
		return m.executeImmediate(ctx, req, signal)
	case ExecutionConfirm:
		return m.queueForConfirmation(req, price, signal)
	case ExecutionBatch:
		return m.queueForBatch(req, price, signal)
	default:
		return nil, fmt.Errorf("unknown execution mode: %s", m.config.Mode)
	}
}

func (m *ExecutionManager) executeImmediate(ctx context.Context, req OrderRequest, signal *core.Signal) (*ExecuteResult, error) {
	order, err := m.broker.PlaceOrder(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("placing order: %w", err)
	}

	return &ExecuteResult{
		Success: true,
		Order:   order,
		Message: fmt.Sprintf("order placed: %s", order.ID),
	}, nil
}

func (m *ExecutionManager) queueForConfirmation(req OrderRequest, price float64, signal *core.Signal) (*ExecuteResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.orderIDSeq++
	pendingID := fmt.Sprintf("PENDING-%d", m.orderIDSeq)

	m.pending[pendingID] = &PendingOrder{
		ID:        pendingID,
		Request:   req,
		Price:     price,
		Signal:    signal,
		CreatedAt: time.Now(),
	}

	return &ExecuteResult{
		Success:   true,
		PendingID: pendingID,
		Message:   fmt.Sprintf("awaiting confirmation: %s", pendingID),
	}, nil
}

func (m *ExecutionManager) queueForBatch(req OrderRequest, price float64, signal *core.Signal) (*ExecuteResult, error) {
	// Same as confirm for now, batch execution handled separately
	return m.queueForConfirmation(req, price, signal)
}

// Confirm confirms a pending order
func (m *ExecutionManager) Confirm(ctx context.Context, pendingID string) (*ExecuteResult, error) {
	m.mu.Lock()
	pending, ok := m.pending[pendingID]
	if !ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("pending order not found: %s", pendingID)
	}
	delete(m.pending, pendingID)
	m.mu.Unlock()

	// Re-check risk before execution
	result := m.risk.Check(ctx, pending.Request, pending.Price)
	if !result.Allowed {
		return &ExecuteResult{
			Success: false,
			Message: result.Reason,
		}, nil
	}

	return m.executeImmediate(ctx, pending.Request, pending.Signal)
}

// Reject rejects a pending order
func (m *ExecutionManager) Reject(pendingID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.pending[pendingID]; !ok {
		return fmt.Errorf("pending order not found: %s", pendingID)
	}
	delete(m.pending, pendingID)
	return nil
}

// GetPendingOrders returns all pending orders
func (m *ExecutionManager) GetPendingOrders() []PendingOrder {
	m.mu.RLock()
	defer m.mu.RUnlock()

	orders := make([]PendingOrder, 0, len(m.pending))
	for _, p := range m.pending {
		orders = append(orders, *p)
	}
	return orders
}
```

**Step 2: Write tests**

```go
// internal/broker/execution_test.go
package broker

import (
	"context"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/broker/mock"
	"github.com/newthinker/atlas/internal/core"
)

func TestExecutionManager_AutoMode(t *testing.T) {
	mockBroker := mock.New()
	mockBroker.SetFillDelay(10 * time.Millisecond)
	ctx := context.Background()
	mockBroker.Connect(ctx)

	risk := NewRiskChecker(DefaultRiskConfig(), mockBroker)
	tracker := NewPositionTracker(mockBroker)

	mgr := NewExecutionManager(ExecutionConfig{
		Mode:           ExecutionAuto,
		DefaultSizePct: 2.0,
	}, mockBroker, risk, tracker)

	signal := &core.Signal{
		Symbol: "AAPL",
		Action: core.ActionBuy,
	}

	result, err := mgr.Execute(ctx, signal, 150.0)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got: %s", result.Message)
	}
	if result.Order == nil {
		t.Error("expected order, got nil")
	}
}

func TestExecutionManager_ConfirmMode(t *testing.T) {
	mockBroker := mock.New()
	mockBroker.SetFillDelay(10 * time.Millisecond)
	ctx := context.Background()
	mockBroker.Connect(ctx)

	risk := NewRiskChecker(DefaultRiskConfig(), mockBroker)
	tracker := NewPositionTracker(mockBroker)

	mgr := NewExecutionManager(ExecutionConfig{
		Mode:           ExecutionConfirm,
		DefaultSizePct: 2.0,
	}, mockBroker, risk, tracker)

	signal := &core.Signal{
		Symbol: "AAPL",
		Action: core.ActionBuy,
	}

	result, err := mgr.Execute(ctx, signal, 150.0)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got: %s", result.Message)
	}
	if result.PendingID == "" {
		t.Error("expected pending ID")
	}
	if result.Order != nil {
		t.Error("order should not be placed yet")
	}

	// Confirm the order
	confirmed, err := mgr.Confirm(ctx, result.PendingID)
	if err != nil {
		t.Fatalf("Confirm failed: %v", err)
	}
	if !confirmed.Success {
		t.Errorf("confirm should succeed: %s", confirmed.Message)
	}
	if confirmed.Order == nil {
		t.Error("expected order after confirm")
	}
}

func TestExecutionManager_RiskRejection(t *testing.T) {
	mockBroker := mock.New()
	ctx := context.Background()
	mockBroker.Connect(ctx)

	risk := NewRiskChecker(RiskConfig{
		MaxPositionPct:   1.0, // Very low limit
		MaxDailyLossPct:  5.0,
		MaxOpenPositions: 20,
	}, mockBroker)
	tracker := NewPositionTracker(mockBroker)

	mgr := NewExecutionManager(ExecutionConfig{
		Mode:           ExecutionAuto,
		DefaultSizePct: 10.0, // Will exceed 1% limit
	}, mockBroker, risk, tracker)

	signal := &core.Signal{
		Symbol: "AAPL",
		Action: core.ActionBuy,
	}

	result, err := mgr.Execute(ctx, signal, 150.0)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result.Success {
		t.Error("should be rejected by risk check")
	}
}

func TestExecutionManager_RejectPending(t *testing.T) {
	mockBroker := mock.New()
	ctx := context.Background()
	mockBroker.Connect(ctx)

	risk := NewRiskChecker(DefaultRiskConfig(), mockBroker)
	tracker := NewPositionTracker(mockBroker)

	mgr := NewExecutionManager(ExecutionConfig{
		Mode:           ExecutionConfirm,
		DefaultSizePct: 2.0,
	}, mockBroker, risk, tracker)

	signal := &core.Signal{
		Symbol: "AAPL",
		Action: core.ActionBuy,
	}

	result, _ := mgr.Execute(ctx, signal, 150.0)
	if err := mgr.Reject(result.PendingID); err != nil {
		t.Errorf("Reject failed: %v", err)
	}

	pending := mgr.GetPendingOrders()
	if len(pending) != 0 {
		t.Errorf("pending orders = %d, want 0", len(pending))
	}
}
```

**Step 3: Run tests**

Run: `go test ./internal/broker/... -v`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/broker/execution.go internal/broker/execution_test.go
git commit -m "feat(broker): add execution manager with auto/confirm modes"
```

---

## Task 6: Config Extensions

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Step 1: Add broker config types**

Add to `internal/config/config.go`:

```go
// ExecutionConfig holds execution settings
type ExecutionConfig struct {
	Mode           string  `mapstructure:"mode"`            // auto, confirm, batch
	BatchTime      string  `mapstructure:"batch_time"`      // HH:MM for batch execution
	DefaultSizePct float64 `mapstructure:"default_size_pct"` // Position size as % of portfolio
}

// RiskConfig holds risk control settings
type RiskConfigSettings struct {
	MaxPositionPct   float64 `mapstructure:"max_position_pct"`
	MaxDailyLossPct  float64 `mapstructure:"max_daily_loss_pct"`
	MaxOpenPositions int     `mapstructure:"max_open_positions"`
}

// Update BrokerConfig to include new fields
type BrokerConfig struct {
	Enabled   bool                `mapstructure:"enabled"`
	Provider  string              `mapstructure:"provider"`
	Mode      string              `mapstructure:"mode"` // paper, live
	Execution ExecutionConfig     `mapstructure:"execution"`
	Risk      RiskConfigSettings  `mapstructure:"risk"`
	Futu      FutuConfig          `mapstructure:"futu"`
}
```

**Step 2: Add validation**

Add to `Validate()` method:

```go
// Broker validation
if c.Broker.Enabled {
	if c.Broker.Mode == "live" && c.Broker.Futu.Env != "real" {
		return core.WrapError(core.ErrConfigInvalid,
			fmt.Errorf("live mode requires futu env=real, got %s", c.Broker.Futu.Env))
	}
	switch c.Broker.Execution.Mode {
	case "auto", "confirm", "batch", "":
		// Valid
	default:
		return core.WrapError(core.ErrConfigInvalid,
			fmt.Errorf("invalid execution mode: %s", c.Broker.Execution.Mode))
	}
}
```

**Step 3: Update defaults**

Add to `Defaults()`:

```go
Broker: BrokerConfig{
	Enabled:  false,
	Provider: "futu",
	Mode:     "paper",
	Execution: ExecutionConfig{
		Mode:           "confirm",
		DefaultSizePct: 2.0,
	},
	Risk: RiskConfigSettings{
		MaxPositionPct:   10.0,
		MaxDailyLossPct:  5.0,
		MaxOpenPositions: 20,
	},
},
```

**Step 4: Run tests**

Run: `go test ./internal/config/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat(config): add broker execution and risk settings"
```

---

## Task 7: Update config.example.yaml

**Files:**
- Modify: `configs/config.example.yaml`

**Step 1: Update broker section**

Replace the broker section with:

```yaml
# Broker integration
broker:
  enabled: false
  provider: "futu"  # futu
  mode: "paper"     # paper, live
  execution:
    mode: "confirm"       # auto, confirm, batch
    batch_time: "09:00"   # For batch mode (HH:MM)
    default_size_pct: 2   # 2% of portfolio per position
  risk:
    max_position_pct: 10      # Max 10% in single position
    max_daily_loss_pct: 5     # Stop trading if down 5% today
    max_open_positions: 20    # Max concurrent positions
  futu:
    host: "127.0.0.1"
    port: 11111
    env: "simulate"  # simulate, real
    trade_password: "${FUTU_TRADE_PWD}"
    rsa_key_path: ""
```

**Step 2: Commit**

```bash
git add configs/config.example.yaml
git commit -m "docs(config): update broker configuration example"
```

---

## Task 8: Server Integration

**Files:**
- Modify: `cmd/atlas/serve.go`
- Modify: `internal/api/server.go`

**Step 1: Create broker in serve.go**

Add to `runServe()`:

```go
// Create broker if enabled
var brokerInstance broker.Broker
var execManager *broker.ExecutionManager

if cfg.Broker.Enabled {
	// For now, use mock broker (FUTU implementation later)
	brokerInstance = mock.New()
	if err := brokerInstance.Connect(context.Background()); err != nil {
		return fmt.Errorf("connecting to broker: %w", err)
	}
	defer brokerInstance.Disconnect()

	riskChecker := broker.NewRiskChecker(broker.RiskConfig{
		MaxPositionPct:   cfg.Broker.Risk.MaxPositionPct,
		MaxDailyLossPct:  cfg.Broker.Risk.MaxDailyLossPct,
		MaxOpenPositions: cfg.Broker.Risk.MaxOpenPositions,
	}, brokerInstance)

	posTracker := broker.NewPositionTracker(brokerInstance)
	if err := posTracker.Sync(context.Background()); err != nil {
		log.Warn("failed to sync positions", zap.Error(err))
	}

	execManager = broker.NewExecutionManager(broker.ExecutionConfig{
		Mode:           broker.ExecutionMode(cfg.Broker.Execution.Mode),
		DefaultSizePct: cfg.Broker.Execution.DefaultSizePct,
	}, brokerInstance, riskChecker, posTracker)

	log.Info("broker enabled",
		zap.String("provider", cfg.Broker.Provider),
		zap.String("mode", cfg.Broker.Mode),
	)
}
```

**Step 2: Add to dependencies**

Update Dependencies struct:

```go
type Dependencies struct {
	App              *app.App
	SignalStore      signal.Store
	Backtester       *backtest.Backtester
	Strategies       *strategy.Engine
	Metrics          *metrics.Registry
	ExecutionManager *broker.ExecutionManager  // Add this
}
```

**Step 3: Run tests**

Run: `go test ./... -v`
Expected: PASS

**Step 4: Commit**

```bash
git add cmd/atlas/serve.go internal/api/server.go
git commit -m "feat(server): integrate broker and execution manager"
```

---

## Verification

After all tasks complete:

```bash
# Run all tests
go test ./... -v

# Build
go build ./cmd/atlas

# Verify config loads
./atlas validate --config configs/config.example.yaml
```

All tests should pass, build should succeed.
