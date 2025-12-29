// internal/broker/interface.go
package broker

import (
	"context"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

// Broker defines the interface for broker integrations.
type Broker interface {
	// Metadata
	Name() string
	SupportedMarkets() []core.Market

	// Connection
	Connect(ctx context.Context) error
	Disconnect() error
	IsConnected() bool

	// Read operations (Phase 4 scope)
	GetPositions(ctx context.Context) ([]Position, error)
	GetOrders(ctx context.Context, filter OrderFilter) ([]Order, error)
	GetAccountInfo(ctx context.Context) (*AccountInfo, error)
	GetTradeHistory(ctx context.Context, start, end time.Time) ([]Trade, error)

	// Write operations (defined but not implemented in Phase 4)
	PlaceOrder(ctx context.Context, order OrderRequest) (*Order, error)
	CancelOrder(ctx context.Context, orderID string) error
	ModifyOrder(ctx context.Context, orderID string, changes OrderChanges) (*Order, error)
}

// Position represents a portfolio position.
type Position struct {
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

// OrderSide represents buy or sell.
type OrderSide string

const (
	OrderSideBuy  OrderSide = "buy"
	OrderSideSell OrderSide = "sell"
)

// OrderType represents the order type.
type OrderType string

const (
	OrderTypeMarket OrderType = "market"
	OrderTypeLimit  OrderType = "limit"
	OrderTypeStop   OrderType = "stop"
)

// OrderStatus represents the order status.
type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "pending"
	OrderStatusOpen      OrderStatus = "open"
	OrderStatusFilled    OrderStatus = "filled"
	OrderStatusCancelled OrderStatus = "cancelled"
	OrderStatusRejected  OrderStatus = "rejected"
)

// Order represents an order.
type Order struct {
	OrderID      string      `json:"order_id"`
	Symbol       string      `json:"symbol"`
	Market       core.Market `json:"market"`
	Side         OrderSide   `json:"side"`
	Type         OrderType   `json:"type"`
	Quantity     int64       `json:"quantity"`
	Price        float64     `json:"price,omitempty"`
	Status       OrderStatus `json:"status"`
	FilledQty    int64       `json:"filled_qty"`
	AvgFillPrice float64     `json:"avg_fill_price"`
	CreatedAt    time.Time   `json:"created_at"`
	UpdatedAt    time.Time   `json:"updated_at"`
}

// OrderFilter filters orders for queries.
type OrderFilter struct {
	Symbol string
	Status OrderStatus
	Side   OrderSide
	Since  time.Time
}

// OrderRequest represents a new order request.
type OrderRequest struct {
	Symbol   string
	Market   core.Market
	Side     OrderSide
	Type     OrderType
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
	TradeID   string      `json:"trade_id"`
	OrderID   string      `json:"order_id"`
	Symbol    string      `json:"symbol"`
	Market    core.Market `json:"market"`
	Side      OrderSide   `json:"side"`
	Quantity  int64       `json:"quantity"`
	Price     float64     `json:"price"`
	Fee       float64     `json:"fee"`
	Timestamp time.Time   `json:"timestamp"`
}
