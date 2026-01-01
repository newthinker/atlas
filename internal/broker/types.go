// Package broker provides types and interfaces for broker integrations.
package broker

import (
	"context"
	"errors"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

// Broker-specific errors.
var (
	// ErrNotConnected indicates the broker is not connected.
	ErrNotConnected = errors.New("broker: not connected")
	// ErrAlreadyConnected indicates the broker is already connected.
	ErrAlreadyConnected = errors.New("broker: already connected")
	// ErrOrderNotFound indicates the order was not found.
	ErrOrderNotFound = errors.New("broker: order not found")
	// ErrPositionNotFound indicates the position was not found.
	ErrPositionNotFound = errors.New("broker: position not found")
	// ErrInvalidSymbol indicates an invalid or empty symbol.
	ErrInvalidSymbol = errors.New("broker: invalid symbol")
	// ErrInvalidQuantity indicates an invalid quantity.
	ErrInvalidQuantity = errors.New("broker: invalid quantity")
	// ErrInvalidPrice indicates an invalid price for limit orders.
	ErrInvalidPrice = errors.New("broker: invalid price for limit order")
	// ErrInvalidStopPrice indicates an invalid stop price for stop orders.
	ErrInvalidStopPrice = errors.New("broker: invalid stop price for stop order")
	// ErrInvalidOrderType indicates an unsupported order type.
	ErrInvalidOrderType = errors.New("broker: invalid order type")
	// ErrOrderNotCancellable indicates the order cannot be cancelled.
	ErrOrderNotCancellable = errors.New("broker: order cannot be cancelled")
	// ErrInsufficientFunds indicates insufficient funds for the order.
	ErrInsufficientFunds = errors.New("broker: insufficient funds")
	// ErrMarketClosed indicates the market is closed.
	ErrMarketClosed = errors.New("broker: market closed")
	// ErrSubscriptionFailed indicates subscription to updates failed.
	ErrSubscriptionFailed = errors.New("broker: subscription failed")
)

// OrderSide represents the direction of an order.
type OrderSide string

const (
	// OrderSideBuy represents a buy order.
	OrderSideBuy OrderSide = "BUY"
	// OrderSideSell represents a sell order.
	OrderSideSell OrderSide = "SELL"
)

// OrderType represents the type of order execution.
type OrderType string

const (
	// OrderTypeMarket executes at current market price.
	OrderTypeMarket OrderType = "MARKET"
	// OrderTypeLimit executes at specified price or better.
	OrderTypeLimit OrderType = "LIMIT"
	// OrderTypeStop triggers when stop price is reached.
	OrderTypeStop OrderType = "STOP"
)

// OrderStatus represents the lifecycle status of an order.
type OrderStatus string

const (
	// OrderStatusPending indicates order is awaiting submission.
	OrderStatusPending OrderStatus = "PENDING"
	// OrderStatusFilled indicates order has been completely filled.
	OrderStatusFilled OrderStatus = "FILLED"
	// OrderStatusPartial indicates order has been partially filled.
	OrderStatusPartial OrderStatus = "PARTIAL"
	// OrderStatusCancelled indicates order was cancelled.
	OrderStatusCancelled OrderStatus = "CANCELLED"
	// OrderStatusRejected indicates order was rejected by broker.
	OrderStatusRejected OrderStatus = "REJECTED"
)

// OrderRequest represents a request to place a new order.
type OrderRequest struct {
	// Symbol is the ticker symbol (e.g., "AAPL", "600519.SH").
	Symbol string `json:"symbol"`
	// Market identifies which market the symbol trades on.
	Market core.Market `json:"market"`
	// Side indicates buy or sell.
	Side OrderSide `json:"side"`
	// Type specifies the order execution type.
	Type OrderType `json:"type"`
	// Quantity is the number of shares/units to trade.
	Quantity int64 `json:"quantity"`
	// Price is the limit price (required for LIMIT orders).
	Price float64 `json:"price,omitempty"`
	// StopPrice is the trigger price (required for STOP orders).
	StopPrice float64 `json:"stop_price,omitempty"`
	// TimeInForce specifies how long the order remains active.
	TimeInForce string `json:"time_in_force,omitempty"`
	// ClientOrderID is an optional client-specified identifier.
	ClientOrderID string `json:"client_order_id,omitempty"`
}

// Validate checks if the order request has valid required fields.
func (r OrderRequest) Validate() error {
	if r.Symbol == "" {
		return ErrInvalidSymbol
	}
	if r.Quantity <= 0 {
		return ErrInvalidQuantity
	}
	if r.Type == OrderTypeLimit && r.Price <= 0 {
		return ErrInvalidPrice
	}
	if r.Type == OrderTypeStop && r.StopPrice <= 0 {
		return ErrInvalidStopPrice
	}
	return nil
}

// Order represents an order in the broker system.
type Order struct {
	// OrderID is the broker-assigned unique identifier.
	OrderID string `json:"order_id"`
	// ClientOrderID is the client-specified identifier if provided.
	ClientOrderID string `json:"client_order_id,omitempty"`
	// Symbol is the ticker symbol.
	Symbol string `json:"symbol"`
	// Market identifies which market the symbol trades on.
	Market core.Market `json:"market"`
	// Side indicates buy or sell.
	Side OrderSide `json:"side"`
	// Type specifies the order execution type.
	Type OrderType `json:"type"`
	// Quantity is the total order quantity.
	Quantity int64 `json:"quantity"`
	// Price is the limit price for limit orders.
	Price float64 `json:"price,omitempty"`
	// StopPrice is the trigger price for stop orders.
	StopPrice float64 `json:"stop_price,omitempty"`
	// Status is the current order status.
	Status OrderStatus `json:"status"`
	// FilledQuantity is the number of shares filled.
	FilledQuantity int64 `json:"filled_quantity"`
	// AverageFillPrice is the average execution price.
	AverageFillPrice float64 `json:"average_fill_price"`
	// Commission is the total commission charged.
	Commission float64 `json:"commission"`
	// CreatedAt is when the order was created.
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt is when the order was last updated.
	UpdatedAt time.Time `json:"updated_at"`
	// FilledAt is when the order was completely filled (nil if not filled).
	FilledAt *time.Time `json:"filled_at,omitempty"`
	// RejectionReason contains the reason if order was rejected.
	RejectionReason string `json:"rejection_reason,omitempty"`
}

// RemainingQuantity returns the unfilled quantity.
func (o Order) RemainingQuantity() int64 {
	return o.Quantity - o.FilledQuantity
}

// IsFilled returns true if the order is completely filled.
func (o Order) IsFilled() bool {
	return o.Status == OrderStatusFilled
}

// IsOpen returns true if the order is still active.
func (o Order) IsOpen() bool {
	return o.Status == OrderStatusPending || o.Status == OrderStatusPartial
}

// IsTerminal returns true if the order is in a final state.
func (o Order) IsTerminal() bool {
	return o.Status == OrderStatusFilled ||
		o.Status == OrderStatusCancelled ||
		o.Status == OrderStatusRejected
}

// Position represents a holding in a security.
type Position struct {
	// Symbol is the ticker symbol.
	Symbol string `json:"symbol"`
	// Market identifies which market the symbol trades on.
	Market core.Market `json:"market"`
	// Quantity is the number of shares held (negative for short).
	Quantity int64 `json:"quantity"`
	// AverageCost is the average cost basis per share.
	AverageCost float64 `json:"average_cost"`
	// CurrentPrice is the latest market price.
	CurrentPrice float64 `json:"current_price"`
	// MarketValue is the current market value of the position.
	MarketValue float64 `json:"market_value"`
	// UnrealizedPL is the unrealized profit/loss.
	UnrealizedPL float64 `json:"unrealized_pl"`
	// UnrealizedPLPercent is the unrealized P/L as a percentage.
	UnrealizedPLPercent float64 `json:"unrealized_pl_percent"`
	// RealizedPL is the realized profit/loss from closed trades.
	RealizedPL float64 `json:"realized_pl"`
	// CostBasis is the total cost basis.
	CostBasis float64 `json:"cost_basis"`
	// UpdatedAt is when the position was last updated.
	UpdatedAt time.Time `json:"updated_at"`
}

// IsLong returns true if this is a long position.
func (p Position) IsLong() bool {
	return p.Quantity > 0
}

// IsShort returns true if this is a short position.
func (p Position) IsShort() bool {
	return p.Quantity < 0
}

// Balance represents account balance information.
type Balance struct {
	// Currency is the currency code (e.g., "USD", "CNY").
	Currency string `json:"currency"`
	// Cash is the available cash balance.
	Cash float64 `json:"cash"`
	// BuyingPower is the total buying power available.
	BuyingPower float64 `json:"buying_power"`
	// TotalValue is the total account value including positions.
	TotalValue float64 `json:"total_value"`
	// MarginUsed is the amount of margin currently in use.
	MarginUsed float64 `json:"margin_used"`
	// MarginAvailable is the available margin.
	MarginAvailable float64 `json:"margin_available"`
	// DayTradingBuyingPower is the buying power for day trading (US only).
	DayTradingBuyingPower float64 `json:"day_trading_buying_power,omitempty"`
	// UpdatedAt is when the balance was last updated.
	UpdatedAt time.Time `json:"updated_at"`
}

// OrderUpdate represents a real-time order status update.
type OrderUpdate struct {
	// Order is the updated order.
	Order Order `json:"order"`
	// Event describes what changed (e.g., "filled", "partial_fill", "cancelled").
	Event string `json:"event"`
	// Timestamp is when the update occurred.
	Timestamp time.Time `json:"timestamp"`
}

// OrderUpdateHandler is a callback function for order updates.
type OrderUpdateHandler func(update OrderUpdate)

// Broker defines the interface for broker integrations.
type Broker interface {
	// Name returns the broker identifier (e.g., "alpaca", "ibkr", "futu").
	Name() string
	// SupportedMarkets returns the markets supported by this broker.
	SupportedMarkets() []core.Market

	// Connection management
	Connect(ctx context.Context) error
	Disconnect() error
	IsConnected() bool

	// Order operations
	PlaceOrder(ctx context.Context, request OrderRequest) (*Order, error)
	CancelOrder(ctx context.Context, orderID string) error
	GetOrder(ctx context.Context, orderID string) (*Order, error)
	GetOpenOrders(ctx context.Context) ([]Order, error)

	// Position operations
	GetPositions(ctx context.Context) ([]Position, error)
	GetPosition(ctx context.Context, symbol string) (*Position, error)

	// Account operations
	GetBalance(ctx context.Context) (*Balance, error)

	// Real-time updates
	Subscribe(handler OrderUpdateHandler) error
	Unsubscribe() error
}
