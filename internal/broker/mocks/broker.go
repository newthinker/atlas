// Package mocks provides mock implementations of broker interfaces for testing.
package mocks

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/newthinker/atlas/internal/broker"
	"github.com/newthinker/atlas/internal/core"
)

// MockBroker implements the broker.Broker interface for testing.
// It simulates order placement, fills, position tracking, and balance updates.
type MockBroker struct {
	mu sync.RWMutex

	// Connection state
	connected bool

	// Order management
	orders      map[string]*broker.Order
	orderID     int64
	fillDelay   time.Duration
	shouldFail  bool
	failMessage string

	// Position management
	positions map[string]*broker.Position

	// Account management
	balance *broker.Balance

	// Subscription management
	handler       broker.OrderUpdateHandler
	stopFillChan  chan struct{}
	fillWaitGroup sync.WaitGroup
}

// New creates a new MockBroker with default settings.
func New() *MockBroker {
	return &MockBroker{
		orders:    make(map[string]*broker.Order),
		positions: make(map[string]*broker.Position),
		balance: &broker.Balance{
			Currency:        "USD",
			Cash:            100000.00,
			BuyingPower:     100000.00,
			TotalValue:      100000.00,
			MarginUsed:      0,
			MarginAvailable: 100000.00,
			UpdatedAt:       time.Now(),
		},
		fillDelay: 100 * time.Millisecond,
	}
}

// Name returns the broker identifier.
func (m *MockBroker) Name() string {
	return "mock"
}

// SupportedMarkets returns the markets supported by this broker.
func (m *MockBroker) SupportedMarkets() []core.Market {
	return []core.Market{core.MarketUS, core.MarketCNA, core.MarketHK}
}

// Connect establishes connection to the mock broker.
func (m *MockBroker) Connect(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.connected {
		return broker.ErrAlreadyConnected
	}

	m.connected = true
	m.stopFillChan = make(chan struct{})
	return nil
}

// Disconnect closes connection to the mock broker.
func (m *MockBroker) Disconnect() error {
	m.mu.Lock()

	if !m.connected {
		m.mu.Unlock()
		return broker.ErrNotConnected
	}

	m.connected = false

	// Signal fill goroutines to stop
	if m.stopFillChan != nil {
		close(m.stopFillChan)
	}
	m.mu.Unlock()

	// Wait for any pending fills to complete
	m.fillWaitGroup.Wait()

	return nil
}

// IsConnected returns the connection status.
func (m *MockBroker) IsConnected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.connected
}

// PlaceOrder places a new order.
func (m *MockBroker) PlaceOrder(ctx context.Context, req broker.OrderRequest) (*broker.Order, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.connected {
		return nil, broker.ErrNotConnected
	}

	// Validate the request
	if err := req.Validate(); err != nil {
		return nil, err
	}

	// Check for configured failure
	if m.shouldFail {
		return nil, fmt.Errorf("mock broker: %s", m.failMessage)
	}

	// Generate order ID
	m.orderID++
	orderID := fmt.Sprintf("MOCK-%d", m.orderID)

	now := time.Now()
	order := &broker.Order{
		OrderID:       orderID,
		ClientOrderID: req.ClientOrderID,
		Symbol:        req.Symbol,
		Market:        req.Market,
		Side:          req.Side,
		Type:          req.Type,
		Quantity:      req.Quantity,
		Price:         req.Price,
		StopPrice:     req.StopPrice,
		Status:        broker.OrderStatusPending,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	m.orders[orderID] = order

	// Start async fill simulation
	m.fillWaitGroup.Add(1)
	go m.simulateFill(orderID)

	// Return a copy
	orderCopy := *order
	return &orderCopy, nil
}

// simulateFill simulates an order fill after the configured delay.
func (m *MockBroker) simulateFill(orderID string) {
	defer m.fillWaitGroup.Done()

	m.mu.RLock()
	delay := m.fillDelay
	stopChan := m.stopFillChan
	m.mu.RUnlock()

	// Wait for fill delay or stop signal
	select {
	case <-time.After(delay):
		// Continue to fill
	case <-stopChan:
		// Broker disconnected, don't fill
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	order, exists := m.orders[orderID]
	if !exists || order.Status != broker.OrderStatusPending {
		return
	}

	// Simulate fill
	now := time.Now()
	order.Status = broker.OrderStatusFilled
	order.FilledQuantity = order.Quantity
	order.FilledAt = &now
	order.UpdatedAt = now

	// Calculate fill price (use order price for limit, simulate market price)
	fillPrice := order.Price
	if order.Type == broker.OrderTypeMarket {
		// Simulate a market price (use 100.0 as default)
		fillPrice = 100.0
	}
	order.AverageFillPrice = fillPrice

	// Update position
	m.updatePosition(order, fillPrice)

	// Update balance
	m.updateBalance(order, fillPrice)

	// Send update via handler
	if m.handler != nil {
		update := broker.OrderUpdate{
			Order:     *order,
			Event:     "filled",
			Timestamp: now,
		}
		// Call handler outside lock to prevent deadlock
		handler := m.handler
		go handler(update)
	}
}

// updatePosition updates positions based on a filled order.
func (m *MockBroker) updatePosition(order *broker.Order, fillPrice float64) {
	posKey := order.Symbol
	pos, exists := m.positions[posKey]

	if !exists {
		pos = &broker.Position{
			Symbol:       order.Symbol,
			Market:       order.Market,
			Quantity:     0,
			AverageCost:  0,
			CurrentPrice: fillPrice,
		}
		m.positions[posKey] = pos
	}

	if order.Side == broker.OrderSideBuy {
		// Calculate new average cost
		totalCost := float64(pos.Quantity)*pos.AverageCost + float64(order.FilledQuantity)*fillPrice
		pos.Quantity += order.FilledQuantity
		if pos.Quantity > 0 {
			pos.AverageCost = totalCost / float64(pos.Quantity)
		}
	} else {
		// Sell order reduces position
		pos.Quantity -= order.FilledQuantity
	}

	pos.CurrentPrice = fillPrice
	pos.MarketValue = float64(pos.Quantity) * fillPrice
	pos.CostBasis = float64(pos.Quantity) * pos.AverageCost
	pos.UnrealizedPL = pos.MarketValue - pos.CostBasis
	if pos.CostBasis > 0 {
		pos.UnrealizedPLPercent = (pos.UnrealizedPL / pos.CostBasis) * 100
	}
	pos.UpdatedAt = time.Now()

	// Remove position if quantity is zero
	if pos.Quantity == 0 {
		delete(m.positions, posKey)
	}
}

// updateBalance updates balance based on a filled order.
func (m *MockBroker) updateBalance(order *broker.Order, fillPrice float64) {
	tradeValue := float64(order.FilledQuantity) * fillPrice

	if order.Side == broker.OrderSideBuy {
		m.balance.Cash -= tradeValue
		m.balance.BuyingPower -= tradeValue
	} else {
		m.balance.Cash += tradeValue
		m.balance.BuyingPower += tradeValue
	}

	m.balance.UpdatedAt = time.Now()
	m.recalculateTotalValue()
}

// recalculateTotalValue recalculates total account value.
func (m *MockBroker) recalculateTotalValue() {
	positionValue := 0.0
	for _, pos := range m.positions {
		positionValue += pos.MarketValue
	}
	m.balance.TotalValue = m.balance.Cash + positionValue
}

// CancelOrder cancels an order by ID.
func (m *MockBroker) CancelOrder(ctx context.Context, orderID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.connected {
		return broker.ErrNotConnected
	}

	order, exists := m.orders[orderID]
	if !exists {
		return broker.ErrOrderNotFound
	}

	if !order.IsOpen() {
		return broker.ErrOrderNotCancellable
	}

	now := time.Now()
	order.Status = broker.OrderStatusCancelled
	order.UpdatedAt = now

	// Send update via handler
	if m.handler != nil {
		update := broker.OrderUpdate{
			Order:     *order,
			Event:     "cancelled",
			Timestamp: now,
		}
		handler := m.handler
		go handler(update)
	}

	return nil
}

// GetOrder retrieves an order by ID.
func (m *MockBroker) GetOrder(ctx context.Context, orderID string) (*broker.Order, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.connected {
		return nil, broker.ErrNotConnected
	}

	order, exists := m.orders[orderID]
	if !exists {
		return nil, broker.ErrOrderNotFound
	}

	// Return a copy
	orderCopy := *order
	return &orderCopy, nil
}

// GetOpenOrders retrieves all open orders.
func (m *MockBroker) GetOpenOrders(ctx context.Context) ([]broker.Order, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.connected {
		return nil, broker.ErrNotConnected
	}

	var openOrders []broker.Order
	for _, order := range m.orders {
		if order.IsOpen() {
			openOrders = append(openOrders, *order)
		}
	}
	return openOrders, nil
}

// GetPositions retrieves all positions.
func (m *MockBroker) GetPositions(ctx context.Context) ([]broker.Position, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.connected {
		return nil, broker.ErrNotConnected
	}

	positions := make([]broker.Position, 0, len(m.positions))
	for _, pos := range m.positions {
		positions = append(positions, *pos)
	}
	return positions, nil
}

// GetPosition retrieves a position by symbol.
func (m *MockBroker) GetPosition(ctx context.Context, symbol string) (*broker.Position, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.connected {
		return nil, broker.ErrNotConnected
	}

	if symbol == "" {
		return nil, broker.ErrInvalidSymbol
	}

	pos, exists := m.positions[symbol]
	if !exists {
		return nil, broker.ErrPositionNotFound
	}

	// Return a copy
	posCopy := *pos
	return &posCopy, nil
}

// GetBalance retrieves the account balance.
func (m *MockBroker) GetBalance(ctx context.Context) (*broker.Balance, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.connected {
		return nil, broker.ErrNotConnected
	}

	// Return a copy
	balanceCopy := *m.balance
	return &balanceCopy, nil
}

// Subscribe registers a handler for order updates.
func (m *MockBroker) Subscribe(handler broker.OrderUpdateHandler) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.connected {
		return broker.ErrNotConnected
	}

	if handler == nil {
		return broker.ErrSubscriptionFailed
	}

	m.handler = handler
	return nil
}

// Unsubscribe removes the order update handler.
func (m *MockBroker) Unsubscribe() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.connected {
		return broker.ErrNotConnected
	}

	m.handler = nil
	return nil
}

// Helper methods for testing

// SetFillDelay sets the delay before orders are filled.
func (m *MockBroker) SetFillDelay(delay time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.fillDelay = delay
}

// SetShouldFail configures the mock to fail order placement.
func (m *MockBroker) SetShouldFail(shouldFail bool, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shouldFail = shouldFail
	m.failMessage = message
}

// SetBalance sets the account balance for testing.
func (m *MockBroker) SetBalance(balance *broker.Balance) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.balance = balance
}

// AddPosition adds a position for testing.
func (m *MockBroker) AddPosition(pos broker.Position) {
	m.mu.Lock()
	defer m.mu.Unlock()
	posCopy := pos
	m.positions[pos.Symbol] = &posCopy
	m.recalculateTotalValue()
}

// GetOrderCount returns the number of orders for testing.
func (m *MockBroker) GetOrderCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.orders)
}

// WaitForFills waits for all pending fills to complete.
func (m *MockBroker) WaitForFills() {
	m.fillWaitGroup.Wait()
}

// Ensure MockBroker implements broker.Broker interface.
var _ broker.Broker = (*MockBroker)(nil)
