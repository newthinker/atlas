// internal/broker/interface.go
// This file contains legacy types for backward compatibility.
// New code should use types from types.go.
package broker

import (
	"context"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

// LegacyBroker defines the legacy interface for broker integrations.
// Deprecated: Use Broker interface from types.go instead.
type LegacyBroker interface {
	// Metadata
	Name() string
	SupportedMarkets() []core.Market

	// Connection
	Connect(ctx context.Context) error
	Disconnect() error
	IsConnected() bool

	// Read operations (Phase 4 scope)
	GetPositions(ctx context.Context) ([]LegacyPosition, error)
	GetOrders(ctx context.Context, filter OrderFilter) ([]LegacyOrder, error)
	GetAccountInfo(ctx context.Context) (*AccountInfo, error)
	GetTradeHistory(ctx context.Context, start, end time.Time) ([]Trade, error)

	// Write operations (defined but not implemented in Phase 4)
	PlaceOrder(ctx context.Context, order LegacyOrderRequest) (*LegacyOrder, error)
	CancelOrder(ctx context.Context, orderID string) error
	ModifyOrder(ctx context.Context, orderID string, changes OrderChanges) (*LegacyOrder, error)
}

// LegacyPosition represents a portfolio position (legacy format).
// Deprecated: Use Position from types.go instead.
type LegacyPosition struct {
	Symbol       string      `json:"symbol"`
	Market       core.Market `json:"market"`
	Quantity     int64       `json:"quantity"`
	AvgCost      float64     `json:"avg_cost"`
	MarketValue  float64     `json:"market_value"`
	UnrealizedPL float64     `json:"unrealized_pl"`
	RealizedPL   float64     `json:"realized_pl"`
}

// AccountInfo represents account summary.
type AccountInfo struct {
	TotalAssets   float64 `json:"total_assets"`
	Cash          float64 `json:"cash"`
	BuyingPower   float64 `json:"buying_power"`
	MarginUsed    float64 `json:"margin_used"`
	DayTradesLeft int     `json:"day_trades_left"`
}

// LegacyOrderSide represents buy or sell (legacy format).
// Deprecated: Use OrderSide from types.go instead.
type LegacyOrderSide string

const (
	LegacyOrderSideBuy  LegacyOrderSide = "buy"
	LegacyOrderSideSell LegacyOrderSide = "sell"
)

// LegacyOrderType represents the order type (legacy format).
// Deprecated: Use OrderType from types.go instead.
type LegacyOrderType string

const (
	LegacyOrderTypeMarket LegacyOrderType = "market"
	LegacyOrderTypeLimit  LegacyOrderType = "limit"
	LegacyOrderTypeStop   LegacyOrderType = "stop"
)

// LegacyOrderStatus represents the order status (legacy format).
// Deprecated: Use OrderStatus from types.go instead.
type LegacyOrderStatus string

const (
	LegacyOrderStatusPending   LegacyOrderStatus = "pending"
	LegacyOrderStatusOpen      LegacyOrderStatus = "open"
	LegacyOrderStatusFilled    LegacyOrderStatus = "filled"
	LegacyOrderStatusCancelled LegacyOrderStatus = "cancelled"
	LegacyOrderStatusRejected  LegacyOrderStatus = "rejected"
)

// LegacyOrder represents an order (legacy format).
// Deprecated: Use Order from types.go instead.
type LegacyOrder struct {
	OrderID      string            `json:"order_id"`
	Symbol       string            `json:"symbol"`
	Market       core.Market       `json:"market"`
	Side         LegacyOrderSide   `json:"side"`
	Type         LegacyOrderType   `json:"type"`
	Quantity     int64             `json:"quantity"`
	Price        float64           `json:"price,omitempty"`
	Status       LegacyOrderStatus `json:"status"`
	FilledQty    int64             `json:"filled_qty"`
	AvgFillPrice float64           `json:"avg_fill_price"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

// OrderFilter filters orders for queries.
type OrderFilter struct {
	Symbol string
	Status LegacyOrderStatus
	Side   LegacyOrderSide
	Since  time.Time
}

// LegacyOrderRequest represents a new order request (legacy format).
// Deprecated: Use OrderRequest from types.go instead.
type LegacyOrderRequest struct {
	Symbol   string
	Market   core.Market
	Side     LegacyOrderSide
	Type     LegacyOrderType
	Quantity int64
	Price    float64 // For limit orders
}

// OrderChanges represents modifications to an existing order.
type OrderChanges struct {
	Price    *float64
	Quantity *int64
}

// Trade represents a completed trade.
type Trade struct {
	TradeID   string          `json:"trade_id"`
	OrderID   string          `json:"order_id"`
	Symbol    string          `json:"symbol"`
	Market    core.Market     `json:"market"`
	Side      LegacyOrderSide `json:"side"`
	Quantity  int64           `json:"quantity"`
	Price     float64         `json:"price"`
	Fee       float64         `json:"fee"`
	Timestamp time.Time       `json:"timestamp"`
}
