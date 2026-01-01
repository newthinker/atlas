package broker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

// TestOrderSideConstants verifies OrderSide constant values.
func TestOrderSideConstants(t *testing.T) {
	tests := []struct {
		name string
		side OrderSide
		want string
	}{
		{"BUY", OrderSideBuy, "BUY"},
		{"SELL", OrderSideSell, "SELL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.side) != tt.want {
				t.Errorf("OrderSide %s = %q, want %q", tt.name, tt.side, tt.want)
			}
		})
	}
}

// TestOrderTypeConstants verifies OrderType constant values.
func TestOrderTypeConstants(t *testing.T) {
	tests := []struct {
		name     string
		typ      OrderType
		want     string
	}{
		{"MARKET", OrderTypeMarket, "MARKET"},
		{"LIMIT", OrderTypeLimit, "LIMIT"},
		{"STOP", OrderTypeStop, "STOP"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.typ) != tt.want {
				t.Errorf("OrderType %s = %q, want %q", tt.name, tt.typ, tt.want)
			}
		})
	}
}

// TestOrderStatusConstants verifies OrderStatus constant values.
func TestOrderStatusConstants(t *testing.T) {
	tests := []struct {
		name   string
		status OrderStatus
		want   string
	}{
		{"PENDING", OrderStatusPending, "PENDING"},
		{"FILLED", OrderStatusFilled, "FILLED"},
		{"PARTIAL", OrderStatusPartial, "PARTIAL"},
		{"CANCELLED", OrderStatusCancelled, "CANCELLED"},
		{"REJECTED", OrderStatusRejected, "REJECTED"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.status) != tt.want {
				t.Errorf("OrderStatus %s = %q, want %q", tt.name, tt.status, tt.want)
			}
		})
	}
}

// TestOrderRequestInstantiation verifies OrderRequest can be created properly.
func TestOrderRequestInstantiation(t *testing.T) {
	req := OrderRequest{
		Symbol:        "AAPL",
		Market:        core.MarketUS,
		Side:          OrderSideBuy,
		Type:          OrderTypeLimit,
		Quantity:      100,
		Price:         150.50,
		StopPrice:     0,
		TimeInForce:   "DAY",
		ClientOrderID: "client-123",
	}

	if req.Symbol != "AAPL" {
		t.Errorf("Symbol = %q, want %q", req.Symbol, "AAPL")
	}
	if req.Market != core.MarketUS {
		t.Errorf("Market = %q, want %q", req.Market, core.MarketUS)
	}
	if req.Side != OrderSideBuy {
		t.Errorf("Side = %q, want %q", req.Side, OrderSideBuy)
	}
	if req.Type != OrderTypeLimit {
		t.Errorf("Type = %q, want %q", req.Type, OrderTypeLimit)
	}
	if req.Quantity != 100 {
		t.Errorf("Quantity = %d, want %d", req.Quantity, 100)
	}
	if req.Price != 150.50 {
		t.Errorf("Price = %f, want %f", req.Price, 150.50)
	}
}

// TestOrderRequestValidate verifies OrderRequest validation.
func TestOrderRequestValidate(t *testing.T) {
	tests := []struct {
		name    string
		req     OrderRequest
		wantErr error
	}{
		{
			name: "valid market order",
			req: OrderRequest{
				Symbol:   "AAPL",
				Market:   core.MarketUS,
				Side:     OrderSideBuy,
				Type:     OrderTypeMarket,
				Quantity: 100,
			},
			wantErr: nil,
		},
		{
			name: "valid limit order",
			req: OrderRequest{
				Symbol:   "AAPL",
				Market:   core.MarketUS,
				Side:     OrderSideBuy,
				Type:     OrderTypeLimit,
				Quantity: 100,
				Price:    150.00,
			},
			wantErr: nil,
		},
		{
			name: "valid stop order",
			req: OrderRequest{
				Symbol:    "AAPL",
				Market:    core.MarketUS,
				Side:      OrderSideSell,
				Type:      OrderTypeStop,
				Quantity:  100,
				StopPrice: 145.00,
			},
			wantErr: nil,
		},
		{
			name: "empty symbol",
			req: OrderRequest{
				Symbol:   "",
				Market:   core.MarketUS,
				Side:     OrderSideBuy,
				Type:     OrderTypeMarket,
				Quantity: 100,
			},
			wantErr: ErrInvalidSymbol,
		},
		{
			name: "zero quantity",
			req: OrderRequest{
				Symbol:   "AAPL",
				Market:   core.MarketUS,
				Side:     OrderSideBuy,
				Type:     OrderTypeMarket,
				Quantity: 0,
			},
			wantErr: ErrInvalidQuantity,
		},
		{
			name: "negative quantity",
			req: OrderRequest{
				Symbol:   "AAPL",
				Market:   core.MarketUS,
				Side:     OrderSideBuy,
				Type:     OrderTypeMarket,
				Quantity: -10,
			},
			wantErr: ErrInvalidQuantity,
		},
		{
			name: "limit order without price",
			req: OrderRequest{
				Symbol:   "AAPL",
				Market:   core.MarketUS,
				Side:     OrderSideBuy,
				Type:     OrderTypeLimit,
				Quantity: 100,
				Price:    0,
			},
			wantErr: ErrInvalidPrice,
		},
		{
			name: "stop order without stop price",
			req: OrderRequest{
				Symbol:    "AAPL",
				Market:    core.MarketUS,
				Side:      OrderSideSell,
				Type:      OrderTypeStop,
				Quantity:  100,
				StopPrice: 0,
			},
			wantErr: ErrInvalidStopPrice,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Validate() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

// TestOrderInstantiation verifies Order can be created and methods work.
func TestOrderInstantiation(t *testing.T) {
	now := time.Now()
	filledAt := now.Add(time.Minute)

	order := Order{
		OrderID:          "ORD-12345",
		ClientOrderID:    "client-123",
		Symbol:           "AAPL",
		Market:           core.MarketUS,
		Side:             OrderSideBuy,
		Type:             OrderTypeLimit,
		Quantity:         100,
		Price:            150.50,
		Status:           OrderStatusFilled,
		FilledQuantity:   100,
		AverageFillPrice: 150.25,
		Commission:       1.00,
		CreatedAt:        now,
		UpdatedAt:        filledAt,
		FilledAt:         &filledAt,
	}

	if order.OrderID != "ORD-12345" {
		t.Errorf("OrderID = %q, want %q", order.OrderID, "ORD-12345")
	}
	if order.Symbol != "AAPL" {
		t.Errorf("Symbol = %q, want %q", order.Symbol, "AAPL")
	}
	if order.Quantity != 100 {
		t.Errorf("Quantity = %d, want %d", order.Quantity, 100)
	}
	if order.FilledQuantity != 100 {
		t.Errorf("FilledQuantity = %d, want %d", order.FilledQuantity, 100)
	}
}

// TestOrderRemainingQuantity verifies RemainingQuantity calculation.
func TestOrderRemainingQuantity(t *testing.T) {
	tests := []struct {
		name     string
		quantity int64
		filled   int64
		want     int64
	}{
		{"unfilled", 100, 0, 100},
		{"partial", 100, 50, 50},
		{"filled", 100, 100, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			order := Order{
				Quantity:       tt.quantity,
				FilledQuantity: tt.filled,
			}
			if got := order.RemainingQuantity(); got != tt.want {
				t.Errorf("RemainingQuantity() = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestOrderIsFilled verifies IsFilled method.
func TestOrderIsFilled(t *testing.T) {
	tests := []struct {
		name   string
		status OrderStatus
		want   bool
	}{
		{"pending", OrderStatusPending, false},
		{"partial", OrderStatusPartial, false},
		{"filled", OrderStatusFilled, true},
		{"cancelled", OrderStatusCancelled, false},
		{"rejected", OrderStatusRejected, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			order := Order{Status: tt.status}
			if got := order.IsFilled(); got != tt.want {
				t.Errorf("IsFilled() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestOrderIsOpen verifies IsOpen method.
func TestOrderIsOpen(t *testing.T) {
	tests := []struct {
		name   string
		status OrderStatus
		want   bool
	}{
		{"pending", OrderStatusPending, true},
		{"partial", OrderStatusPartial, true},
		{"filled", OrderStatusFilled, false},
		{"cancelled", OrderStatusCancelled, false},
		{"rejected", OrderStatusRejected, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			order := Order{Status: tt.status}
			if got := order.IsOpen(); got != tt.want {
				t.Errorf("IsOpen() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestOrderIsTerminal verifies IsTerminal method.
func TestOrderIsTerminal(t *testing.T) {
	tests := []struct {
		name   string
		status OrderStatus
		want   bool
	}{
		{"pending", OrderStatusPending, false},
		{"partial", OrderStatusPartial, false},
		{"filled", OrderStatusFilled, true},
		{"cancelled", OrderStatusCancelled, true},
		{"rejected", OrderStatusRejected, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			order := Order{Status: tt.status}
			if got := order.IsTerminal(); got != tt.want {
				t.Errorf("IsTerminal() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestPositionInstantiation verifies Position can be created properly.
func TestPositionInstantiation(t *testing.T) {
	now := time.Now()

	pos := Position{
		Symbol:              "AAPL",
		Market:              core.MarketUS,
		Quantity:            100,
		AverageCost:         150.00,
		CurrentPrice:        175.00,
		MarketValue:         17500.00,
		UnrealizedPL:        2500.00,
		UnrealizedPLPercent: 16.67,
		RealizedPL:          500.00,
		CostBasis:           15000.00,
		UpdatedAt:           now,
	}

	if pos.Symbol != "AAPL" {
		t.Errorf("Symbol = %q, want %q", pos.Symbol, "AAPL")
	}
	if pos.Quantity != 100 {
		t.Errorf("Quantity = %d, want %d", pos.Quantity, 100)
	}
	if pos.MarketValue != 17500.00 {
		t.Errorf("MarketValue = %f, want %f", pos.MarketValue, 17500.00)
	}
	if pos.UnrealizedPL != 2500.00 {
		t.Errorf("UnrealizedPL = %f, want %f", pos.UnrealizedPL, 2500.00)
	}
}

// TestPositionIsLongIsShort verifies IsLong and IsShort methods.
func TestPositionIsLongIsShort(t *testing.T) {
	tests := []struct {
		name      string
		quantity  int64
		wantLong  bool
		wantShort bool
	}{
		{"long position", 100, true, false},
		{"short position", -50, false, true},
		{"zero position", 0, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pos := Position{Quantity: tt.quantity}
			if got := pos.IsLong(); got != tt.wantLong {
				t.Errorf("IsLong() = %v, want %v", got, tt.wantLong)
			}
			if got := pos.IsShort(); got != tt.wantShort {
				t.Errorf("IsShort() = %v, want %v", got, tt.wantShort)
			}
		})
	}
}

// TestBalanceInstantiation verifies Balance can be created properly.
func TestBalanceInstantiation(t *testing.T) {
	now := time.Now()

	balance := Balance{
		Currency:              "USD",
		Cash:                  50000.00,
		BuyingPower:           100000.00,
		TotalValue:            150000.00,
		MarginUsed:            25000.00,
		MarginAvailable:       75000.00,
		DayTradingBuyingPower: 200000.00,
		UpdatedAt:             now,
	}

	if balance.Currency != "USD" {
		t.Errorf("Currency = %q, want %q", balance.Currency, "USD")
	}
	if balance.Cash != 50000.00 {
		t.Errorf("Cash = %f, want %f", balance.Cash, 50000.00)
	}
	if balance.BuyingPower != 100000.00 {
		t.Errorf("BuyingPower = %f, want %f", balance.BuyingPower, 100000.00)
	}
	if balance.TotalValue != 150000.00 {
		t.Errorf("TotalValue = %f, want %f", balance.TotalValue, 150000.00)
	}
	if balance.MarginUsed != 25000.00 {
		t.Errorf("MarginUsed = %f, want %f", balance.MarginUsed, 25000.00)
	}
	if balance.MarginAvailable != 75000.00 {
		t.Errorf("MarginAvailable = %f, want %f", balance.MarginAvailable, 75000.00)
	}
}

// TestOrderUpdateInstantiation verifies OrderUpdate can be created properly.
func TestOrderUpdateInstantiation(t *testing.T) {
	now := time.Now()

	update := OrderUpdate{
		Order: Order{
			OrderID:  "ORD-12345",
			Symbol:   "AAPL",
			Status:   OrderStatusFilled,
			Quantity: 100,
		},
		Event:     "filled",
		Timestamp: now,
	}

	if update.Order.OrderID != "ORD-12345" {
		t.Errorf("Order.OrderID = %q, want %q", update.Order.OrderID, "ORD-12345")
	}
	if update.Event != "filled" {
		t.Errorf("Event = %q, want %q", update.Event, "filled")
	}
	if update.Timestamp != now {
		t.Errorf("Timestamp = %v, want %v", update.Timestamp, now)
	}
}

// TestOrderUpdateHandler verifies OrderUpdateHandler can be used.
func TestOrderUpdateHandler(t *testing.T) {
	var received *OrderUpdate

	handler := OrderUpdateHandler(func(update OrderUpdate) {
		received = &update
	})

	update := OrderUpdate{
		Order: Order{
			OrderID: "ORD-12345",
			Status:  OrderStatusFilled,
		},
		Event:     "filled",
		Timestamp: time.Now(),
	}

	handler(update)

	if received == nil {
		t.Fatal("handler was not called")
	}
	if received.Order.OrderID != "ORD-12345" {
		t.Errorf("received OrderID = %q, want %q", received.Order.OrderID, "ORD-12345")
	}
}

// TestBrokerErrors verifies error values are distinct and have proper messages.
func TestBrokerErrors(t *testing.T) {
	errs := []error{
		ErrNotConnected,
		ErrAlreadyConnected,
		ErrOrderNotFound,
		ErrPositionNotFound,
		ErrInvalidSymbol,
		ErrInvalidQuantity,
		ErrInvalidPrice,
		ErrInvalidStopPrice,
		ErrInvalidOrderType,
		ErrOrderNotCancellable,
		ErrInsufficientFunds,
		ErrMarketClosed,
		ErrSubscriptionFailed,
	}

	// Verify all errors have non-empty messages
	for _, err := range errs {
		if err.Error() == "" {
			t.Errorf("error has empty message: %v", err)
		}
	}

	// Verify all errors are distinct
	seen := make(map[string]bool)
	for _, err := range errs {
		msg := err.Error()
		if seen[msg] {
			t.Errorf("duplicate error message: %s", msg)
		}
		seen[msg] = true
	}
}

// TestBrokerInterfaceCompleteness verifies the Broker interface is well-defined.
// This is a compile-time check that the interface has all expected methods.
func TestBrokerInterfaceCompleteness(t *testing.T) {
	// This test verifies at compile time that a mock type
	// could implement the Broker interface with all required methods.
	var _ Broker = (*mockBrokerForTest)(nil)
}

// mockBrokerForTest is a minimal mock that demonstrates the interface is complete.
type mockBrokerForTest struct{}

func (m *mockBrokerForTest) Name() string                                        { return "" }
func (m *mockBrokerForTest) SupportedMarkets() []core.Market                     { return nil }
func (m *mockBrokerForTest) Connect(_ context.Context) error                     { return nil }
func (m *mockBrokerForTest) Disconnect() error                                   { return nil }
func (m *mockBrokerForTest) IsConnected() bool                                   { return false }
func (m *mockBrokerForTest) PlaceOrder(_ context.Context, _ OrderRequest) (*Order, error) {
	return nil, nil
}
func (m *mockBrokerForTest) CancelOrder(_ context.Context, _ string) error       { return nil }
func (m *mockBrokerForTest) GetOrder(_ context.Context, _ string) (*Order, error) {
	return nil, nil
}
func (m *mockBrokerForTest) GetOpenOrders(_ context.Context) ([]Order, error)    { return nil, nil }
func (m *mockBrokerForTest) GetPositions(_ context.Context) ([]Position, error)  { return nil, nil }
func (m *mockBrokerForTest) GetPosition(_ context.Context, _ string) (*Position, error) {
	return nil, nil
}
func (m *mockBrokerForTest) GetBalance(_ context.Context) (*Balance, error)      { return nil, nil }
func (m *mockBrokerForTest) Subscribe(_ OrderUpdateHandler) error                { return nil }
func (m *mockBrokerForTest) Unsubscribe() error                                  { return nil }
