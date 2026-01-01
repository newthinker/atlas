// internal/broker/mock/mock.go
package mock

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/newthinker/atlas/internal/broker"
	"github.com/newthinker/atlas/internal/core"
)

// MockBroker implements the LegacyBroker interface for testing.
type MockBroker struct {
	mu         sync.RWMutex
	connected  bool
	positions  []broker.LegacyPosition
	orders     []broker.LegacyOrder
	trades     []broker.Trade
	account    broker.AccountInfo
	orderID    int
	tradeID    int
}

// New creates a new mock broker with sample data.
func New() *MockBroker {
	return &MockBroker{
		positions: []broker.LegacyPosition{
			{
				Symbol:       "AAPL",
				Market:       core.MarketUS,
				Quantity:     100,
				AvgCost:      150.00,
				MarketValue:  17500.00,
				UnrealizedPL: 2500.00,
			},
			{
				Symbol:       "600519.SH",
				Market:       core.MarketCNA,
				Quantity:     50,
				AvgCost:      1800.00,
				MarketValue:  95000.00,
				UnrealizedPL: 5000.00,
			},
		},
		account: broker.AccountInfo{
			TotalAssets:   150000.00,
			Cash:          37500.00,
			BuyingPower:   50000.00,
			MarginUsed:    0,
			DayTradesLeft: 3,
		},
		orderID: 1000,
		tradeID: 2000,
	}
}

// Name returns the broker name.
func (m *MockBroker) Name() string {
	return "mock"
}

// SupportedMarkets returns the supported markets.
func (m *MockBroker) SupportedMarkets() []core.Market {
	return []core.Market{core.MarketUS, core.MarketCNA, core.MarketHK}
}

// Connect establishes connection (no-op for mock).
func (m *MockBroker) Connect(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connected = true
	return nil
}

// Disconnect closes connection.
func (m *MockBroker) Disconnect() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connected = false
	return nil
}

// IsConnected returns connection status.
func (m *MockBroker) IsConnected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.connected
}

// GetPositions returns mock positions.
func (m *MockBroker) GetPositions(ctx context.Context) ([]broker.LegacyPosition, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.connected {
		return nil, fmt.Errorf("not connected")
	}
	return m.positions, nil
}

// GetOrders returns mock orders filtered by criteria.
func (m *MockBroker) GetOrders(ctx context.Context, filter broker.OrderFilter) ([]broker.LegacyOrder, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.connected {
		return nil, fmt.Errorf("not connected")
	}

	var result []broker.LegacyOrder
	for _, o := range m.orders {
		if filter.Symbol != "" && o.Symbol != filter.Symbol {
			continue
		}
		if filter.Status != "" && o.Status != filter.Status {
			continue
		}
		if filter.Side != "" && o.Side != filter.Side {
			continue
		}
		if !filter.Since.IsZero() && o.CreatedAt.Before(filter.Since) {
			continue
		}
		result = append(result, o)
	}
	return result, nil
}

// GetAccountInfo returns mock account info.
func (m *MockBroker) GetAccountInfo(ctx context.Context) (*broker.AccountInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.connected {
		return nil, fmt.Errorf("not connected")
	}
	return &m.account, nil
}

// GetTradeHistory returns mock trades in the time range.
func (m *MockBroker) GetTradeHistory(ctx context.Context, start, end time.Time) ([]broker.Trade, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.connected {
		return nil, fmt.Errorf("not connected")
	}

	var result []broker.Trade
	for _, t := range m.trades {
		if t.Timestamp.Before(start) || t.Timestamp.After(end) {
			continue
		}
		result = append(result, t)
	}
	return result, nil
}

// PlaceOrder simulates placing an order.
func (m *MockBroker) PlaceOrder(ctx context.Context, req broker.LegacyOrderRequest) (*broker.LegacyOrder, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.connected {
		return nil, fmt.Errorf("not connected")
	}

	m.orderID++
	order := broker.LegacyOrder{
		OrderID:   fmt.Sprintf("ORD%d", m.orderID),
		Symbol:    req.Symbol,
		Market:    req.Market,
		Side:      req.Side,
		Type:      req.Type,
		Quantity:  req.Quantity,
		Price:     req.Price,
		Status:    broker.LegacyOrderStatusOpen,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	m.orders = append(m.orders, order)
	return &order, nil
}

// CancelOrder simulates cancelling an order.
func (m *MockBroker) CancelOrder(ctx context.Context, orderID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.connected {
		return fmt.Errorf("not connected")
	}

	for i, o := range m.orders {
		if o.OrderID == orderID {
			if o.Status != broker.LegacyOrderStatusOpen && o.Status != broker.LegacyOrderStatusPending {
				return fmt.Errorf("order cannot be cancelled: %s", o.Status)
			}
			m.orders[i].Status = broker.LegacyOrderStatusCancelled
			m.orders[i].UpdatedAt = time.Now()
			return nil
		}
	}
	return fmt.Errorf("order not found: %s", orderID)
}

// ModifyOrder simulates modifying an order.
func (m *MockBroker) ModifyOrder(ctx context.Context, orderID string, changes broker.OrderChanges) (*broker.LegacyOrder, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.connected {
		return nil, fmt.Errorf("not connected")
	}

	for i, o := range m.orders {
		if o.OrderID == orderID {
			if o.Status != broker.LegacyOrderStatusOpen && o.Status != broker.LegacyOrderStatusPending {
				return nil, fmt.Errorf("order cannot be modified: %s", o.Status)
			}
			if changes.Price != nil {
				m.orders[i].Price = *changes.Price
			}
			if changes.Quantity != nil {
				m.orders[i].Quantity = *changes.Quantity
			}
			m.orders[i].UpdatedAt = time.Now()
			return &m.orders[i], nil
		}
	}
	return nil, fmt.Errorf("order not found: %s", orderID)
}

// AddPosition adds a position for testing.
func (m *MockBroker) AddPosition(pos broker.LegacyPosition) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.positions = append(m.positions, pos)
}

// AddTrade adds a trade for testing.
func (m *MockBroker) AddTrade(trade broker.Trade) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.trades = append(m.trades, trade)
}

// SetAccount sets account info for testing.
func (m *MockBroker) SetAccount(info broker.AccountInfo) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.account = info
}
