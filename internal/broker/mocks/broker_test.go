package mocks

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/broker"
	"github.com/newthinker/atlas/internal/core"
)

func TestMockBroker_ConnectDisconnect(t *testing.T) {
	m := New()
	ctx := context.Background()

	// Should not be connected initially
	if m.IsConnected() {
		t.Error("expected broker to be disconnected initially")
	}

	// Connect should succeed
	if err := m.Connect(ctx); err != nil {
		t.Errorf("Connect failed: %v", err)
	}

	// Should be connected after Connect
	if !m.IsConnected() {
		t.Error("expected broker to be connected after Connect")
	}

	// Connect again should fail with ErrAlreadyConnected
	if err := m.Connect(ctx); err != broker.ErrAlreadyConnected {
		t.Errorf("expected ErrAlreadyConnected, got: %v", err)
	}

	// Disconnect should succeed
	if err := m.Disconnect(); err != nil {
		t.Errorf("Disconnect failed: %v", err)
	}

	// Should not be connected after Disconnect
	if m.IsConnected() {
		t.Error("expected broker to be disconnected after Disconnect")
	}

	// Disconnect again should fail with ErrNotConnected
	if err := m.Disconnect(); err != broker.ErrNotConnected {
		t.Errorf("expected ErrNotConnected, got: %v", err)
	}
}

func TestMockBroker_NameAndMarkets(t *testing.T) {
	m := New()

	if name := m.Name(); name != "mock" {
		t.Errorf("expected name 'mock', got: %s", name)
	}

	markets := m.SupportedMarkets()
	if len(markets) != 3 {
		t.Errorf("expected 3 supported markets, got: %d", len(markets))
	}

	expectedMarkets := map[core.Market]bool{
		core.MarketUS:  true,
		core.MarketCNA: true,
		core.MarketHK:  true,
	}
	for _, market := range markets {
		if !expectedMarkets[market] {
			t.Errorf("unexpected market: %s", market)
		}
	}
}

func TestMockBroker_PlaceOrder_CreatesPendingOrder(t *testing.T) {
	m := New()
	ctx := context.Background()

	// Set long fill delay so order stays pending
	m.SetFillDelay(10 * time.Second)

	if err := m.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer m.Disconnect()

	req := broker.OrderRequest{
		Symbol:   "AAPL",
		Market:   core.MarketUS,
		Side:     broker.OrderSideBuy,
		Type:     broker.OrderTypeLimit,
		Quantity: 100,
		Price:    150.00,
	}

	order, err := m.PlaceOrder(ctx, req)
	if err != nil {
		t.Fatalf("PlaceOrder failed: %v", err)
	}

	// Verify order fields
	if order.OrderID == "" {
		t.Error("expected order ID to be set")
	}
	if order.Symbol != "AAPL" {
		t.Errorf("expected symbol AAPL, got: %s", order.Symbol)
	}
	if order.Status != broker.OrderStatusPending {
		t.Errorf("expected status PENDING, got: %s", order.Status)
	}
	if order.Quantity != 100 {
		t.Errorf("expected quantity 100, got: %d", order.Quantity)
	}
	if order.Price != 150.00 {
		t.Errorf("expected price 150.00, got: %f", order.Price)
	}
	if order.FilledQuantity != 0 {
		t.Errorf("expected filled quantity 0, got: %d", order.FilledQuantity)
	}

	// Verify order is retrievable
	retrieved, err := m.GetOrder(ctx, order.OrderID)
	if err != nil {
		t.Fatalf("GetOrder failed: %v", err)
	}
	if retrieved.OrderID != order.OrderID {
		t.Error("retrieved order ID mismatch")
	}
}

func TestMockBroker_PlaceOrder_NotConnected(t *testing.T) {
	m := New()
	ctx := context.Background()

	req := broker.OrderRequest{
		Symbol:   "AAPL",
		Market:   core.MarketUS,
		Side:     broker.OrderSideBuy,
		Type:     broker.OrderTypeMarket,
		Quantity: 100,
	}

	_, err := m.PlaceOrder(ctx, req)
	if err != broker.ErrNotConnected {
		t.Errorf("expected ErrNotConnected, got: %v", err)
	}
}

func TestMockBroker_PlaceOrder_ValidationErrors(t *testing.T) {
	m := New()
	ctx := context.Background()

	if err := m.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer m.Disconnect()

	tests := []struct {
		name    string
		req     broker.OrderRequest
		wantErr error
	}{
		{
			name: "invalid symbol",
			req: broker.OrderRequest{
				Symbol:   "",
				Side:     broker.OrderSideBuy,
				Type:     broker.OrderTypeMarket,
				Quantity: 100,
			},
			wantErr: broker.ErrInvalidSymbol,
		},
		{
			name: "invalid quantity",
			req: broker.OrderRequest{
				Symbol:   "AAPL",
				Side:     broker.OrderSideBuy,
				Type:     broker.OrderTypeMarket,
				Quantity: 0,
			},
			wantErr: broker.ErrInvalidQuantity,
		},
		{
			name: "limit order without price",
			req: broker.OrderRequest{
				Symbol:   "AAPL",
				Side:     broker.OrderSideBuy,
				Type:     broker.OrderTypeLimit,
				Quantity: 100,
				Price:    0,
			},
			wantErr: broker.ErrInvalidPrice,
		},
		{
			name: "stop order without stop price",
			req: broker.OrderRequest{
				Symbol:    "AAPL",
				Side:      broker.OrderSideSell,
				Type:      broker.OrderTypeStop,
				Quantity:  100,
				StopPrice: 0,
			},
			wantErr: broker.ErrInvalidStopPrice,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := m.PlaceOrder(ctx, tt.req)
			if err != tt.wantErr {
				t.Errorf("expected error %v, got: %v", tt.wantErr, err)
			}
		})
	}
}

func TestMockBroker_OrderFillAfterDelay(t *testing.T) {
	m := New()
	ctx := context.Background()

	// Set short fill delay
	m.SetFillDelay(50 * time.Millisecond)

	if err := m.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer m.Disconnect()

	req := broker.OrderRequest{
		Symbol:   "AAPL",
		Market:   core.MarketUS,
		Side:     broker.OrderSideBuy,
		Type:     broker.OrderTypeLimit,
		Quantity: 100,
		Price:    150.00,
	}

	order, err := m.PlaceOrder(ctx, req)
	if err != nil {
		t.Fatalf("PlaceOrder failed: %v", err)
	}

	// Order should be pending immediately
	if order.Status != broker.OrderStatusPending {
		t.Errorf("expected status PENDING, got: %s", order.Status)
	}

	// Wait for fill
	m.WaitForFills()

	// Order should now be filled
	filled, err := m.GetOrder(ctx, order.OrderID)
	if err != nil {
		t.Fatalf("GetOrder failed: %v", err)
	}

	if filled.Status != broker.OrderStatusFilled {
		t.Errorf("expected status FILLED, got: %s", filled.Status)
	}
	if filled.FilledQuantity != 100 {
		t.Errorf("expected filled quantity 100, got: %d", filled.FilledQuantity)
	}
	if filled.AverageFillPrice != 150.00 {
		t.Errorf("expected average fill price 150.00, got: %f", filled.AverageFillPrice)
	}
	if filled.FilledAt == nil {
		t.Error("expected FilledAt to be set")
	}
}

func TestMockBroker_CancelOrder(t *testing.T) {
	m := New()
	ctx := context.Background()

	// Set long fill delay so we can cancel
	m.SetFillDelay(10 * time.Second)

	if err := m.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer m.Disconnect()

	req := broker.OrderRequest{
		Symbol:   "AAPL",
		Market:   core.MarketUS,
		Side:     broker.OrderSideBuy,
		Type:     broker.OrderTypeLimit,
		Quantity: 100,
		Price:    150.00,
	}

	order, err := m.PlaceOrder(ctx, req)
	if err != nil {
		t.Fatalf("PlaceOrder failed: %v", err)
	}

	// Cancel the order
	if err := m.CancelOrder(ctx, order.OrderID); err != nil {
		t.Fatalf("CancelOrder failed: %v", err)
	}

	// Verify order is cancelled
	cancelled, err := m.GetOrder(ctx, order.OrderID)
	if err != nil {
		t.Fatalf("GetOrder failed: %v", err)
	}
	if cancelled.Status != broker.OrderStatusCancelled {
		t.Errorf("expected status CANCELLED, got: %s", cancelled.Status)
	}

	// Try to cancel again - should fail
	if err := m.CancelOrder(ctx, order.OrderID); err != broker.ErrOrderNotCancellable {
		t.Errorf("expected ErrOrderNotCancellable, got: %v", err)
	}
}

func TestMockBroker_CancelOrder_NotFound(t *testing.T) {
	m := New()
	ctx := context.Background()

	if err := m.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer m.Disconnect()

	err := m.CancelOrder(ctx, "nonexistent-order")
	if err != broker.ErrOrderNotFound {
		t.Errorf("expected ErrOrderNotFound, got: %v", err)
	}
}

func TestMockBroker_GetOpenOrders(t *testing.T) {
	m := New()
	ctx := context.Background()

	// Set long fill delay
	m.SetFillDelay(10 * time.Second)

	if err := m.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer m.Disconnect()

	// Place two orders
	req1 := broker.OrderRequest{
		Symbol:   "AAPL",
		Market:   core.MarketUS,
		Side:     broker.OrderSideBuy,
		Type:     broker.OrderTypeLimit,
		Quantity: 100,
		Price:    150.00,
	}
	req2 := broker.OrderRequest{
		Symbol:   "GOOGL",
		Market:   core.MarketUS,
		Side:     broker.OrderSideBuy,
		Type:     broker.OrderTypeLimit,
		Quantity: 50,
		Price:    100.00,
	}

	_, err := m.PlaceOrder(ctx, req1)
	if err != nil {
		t.Fatalf("PlaceOrder 1 failed: %v", err)
	}
	order2, err := m.PlaceOrder(ctx, req2)
	if err != nil {
		t.Fatalf("PlaceOrder 2 failed: %v", err)
	}

	// Cancel one order
	if err := m.CancelOrder(ctx, order2.OrderID); err != nil {
		t.Fatalf("CancelOrder failed: %v", err)
	}

	// Get open orders - should only have one
	openOrders, err := m.GetOpenOrders(ctx)
	if err != nil {
		t.Fatalf("GetOpenOrders failed: %v", err)
	}

	if len(openOrders) != 1 {
		t.Errorf("expected 1 open order, got: %d", len(openOrders))
	}
	if openOrders[0].Symbol != "AAPL" {
		t.Errorf("expected open order for AAPL, got: %s", openOrders[0].Symbol)
	}
}

func TestMockBroker_PositionUpdatedOnFill(t *testing.T) {
	m := New()
	ctx := context.Background()

	m.SetFillDelay(50 * time.Millisecond)

	if err := m.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer m.Disconnect()

	// Place buy order
	req := broker.OrderRequest{
		Symbol:   "AAPL",
		Market:   core.MarketUS,
		Side:     broker.OrderSideBuy,
		Type:     broker.OrderTypeLimit,
		Quantity: 100,
		Price:    150.00,
	}

	_, err := m.PlaceOrder(ctx, req)
	if err != nil {
		t.Fatalf("PlaceOrder failed: %v", err)
	}

	// Wait for fill
	m.WaitForFills()

	// Check position
	pos, err := m.GetPosition(ctx, "AAPL")
	if err != nil {
		t.Fatalf("GetPosition failed: %v", err)
	}

	if pos.Quantity != 100 {
		t.Errorf("expected position quantity 100, got: %d", pos.Quantity)
	}
	if pos.AverageCost != 150.00 {
		t.Errorf("expected average cost 150.00, got: %f", pos.AverageCost)
	}
	if pos.MarketValue != 15000.00 {
		t.Errorf("expected market value 15000.00, got: %f", pos.MarketValue)
	}

	// Place sell order to reduce position
	sellReq := broker.OrderRequest{
		Symbol:   "AAPL",
		Market:   core.MarketUS,
		Side:     broker.OrderSideSell,
		Type:     broker.OrderTypeLimit,
		Quantity: 50,
		Price:    160.00,
	}

	_, err = m.PlaceOrder(ctx, sellReq)
	if err != nil {
		t.Fatalf("PlaceOrder (sell) failed: %v", err)
	}

	m.WaitForFills()

	// Check updated position
	pos, err = m.GetPosition(ctx, "AAPL")
	if err != nil {
		t.Fatalf("GetPosition failed: %v", err)
	}

	if pos.Quantity != 50 {
		t.Errorf("expected position quantity 50, got: %d", pos.Quantity)
	}
}

func TestMockBroker_PositionClosedOnFullSell(t *testing.T) {
	m := New()
	ctx := context.Background()

	m.SetFillDelay(50 * time.Millisecond)

	if err := m.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer m.Disconnect()

	// Add initial position
	m.AddPosition(broker.Position{
		Symbol:       "AAPL",
		Market:       core.MarketUS,
		Quantity:     100,
		AverageCost:  150.00,
		CurrentPrice: 150.00,
		MarketValue:  15000.00,
	})

	// Sell entire position
	sellReq := broker.OrderRequest{
		Symbol:   "AAPL",
		Market:   core.MarketUS,
		Side:     broker.OrderSideSell,
		Type:     broker.OrderTypeLimit,
		Quantity: 100,
		Price:    160.00,
	}

	_, err := m.PlaceOrder(ctx, sellReq)
	if err != nil {
		t.Fatalf("PlaceOrder (sell) failed: %v", err)
	}

	m.WaitForFills()

	// Position should not exist
	_, err = m.GetPosition(ctx, "AAPL")
	if err != broker.ErrPositionNotFound {
		t.Errorf("expected ErrPositionNotFound, got: %v", err)
	}

	// GetPositions should return empty
	positions, err := m.GetPositions(ctx)
	if err != nil {
		t.Fatalf("GetPositions failed: %v", err)
	}
	if len(positions) != 0 {
		t.Errorf("expected 0 positions, got: %d", len(positions))
	}
}

func TestMockBroker_BalanceUpdatedOnFill(t *testing.T) {
	m := New()
	ctx := context.Background()

	m.SetFillDelay(50 * time.Millisecond)

	if err := m.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer m.Disconnect()

	// Get initial balance
	initialBalance, err := m.GetBalance(ctx)
	if err != nil {
		t.Fatalf("GetBalance failed: %v", err)
	}
	initialCash := initialBalance.Cash

	// Place buy order
	req := broker.OrderRequest{
		Symbol:   "AAPL",
		Market:   core.MarketUS,
		Side:     broker.OrderSideBuy,
		Type:     broker.OrderTypeLimit,
		Quantity: 100,
		Price:    150.00,
	}

	_, err = m.PlaceOrder(ctx, req)
	if err != nil {
		t.Fatalf("PlaceOrder failed: %v", err)
	}

	m.WaitForFills()

	// Check balance
	balance, err := m.GetBalance(ctx)
	if err != nil {
		t.Fatalf("GetBalance failed: %v", err)
	}

	expectedCash := initialCash - (100 * 150.00)
	if balance.Cash != expectedCash {
		t.Errorf("expected cash %f, got: %f", expectedCash, balance.Cash)
	}
}

func TestMockBroker_OrderUpdateHandler(t *testing.T) {
	m := New()
	ctx := context.Background()

	m.SetFillDelay(50 * time.Millisecond)

	if err := m.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer m.Disconnect()

	// Set up handler
	var mu sync.Mutex
	var receivedUpdates []broker.OrderUpdate

	handler := func(update broker.OrderUpdate) {
		mu.Lock()
		defer mu.Unlock()
		receivedUpdates = append(receivedUpdates, update)
	}

	if err := m.Subscribe(handler); err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	// Place order
	req := broker.OrderRequest{
		Symbol:   "AAPL",
		Market:   core.MarketUS,
		Side:     broker.OrderSideBuy,
		Type:     broker.OrderTypeLimit,
		Quantity: 100,
		Price:    150.00,
	}

	order, err := m.PlaceOrder(ctx, req)
	if err != nil {
		t.Fatalf("PlaceOrder failed: %v", err)
	}

	// Wait for fill
	m.WaitForFills()

	// Give handler time to receive update
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(receivedUpdates) != 1 {
		t.Fatalf("expected 1 update, got: %d", len(receivedUpdates))
	}

	update := receivedUpdates[0]
	if update.Event != "filled" {
		t.Errorf("expected event 'filled', got: %s", update.Event)
	}
	if update.Order.OrderID != order.OrderID {
		t.Error("update order ID mismatch")
	}
	if update.Order.Status != broker.OrderStatusFilled {
		t.Errorf("expected status FILLED, got: %s", update.Order.Status)
	}
}

func TestMockBroker_CancelOrderHandler(t *testing.T) {
	m := New()
	ctx := context.Background()

	m.SetFillDelay(10 * time.Second) // Long delay to allow cancel

	if err := m.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer m.Disconnect()

	// Set up handler
	var mu sync.Mutex
	var receivedUpdates []broker.OrderUpdate

	handler := func(update broker.OrderUpdate) {
		mu.Lock()
		defer mu.Unlock()
		receivedUpdates = append(receivedUpdates, update)
	}

	if err := m.Subscribe(handler); err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	// Place and cancel order
	req := broker.OrderRequest{
		Symbol:   "AAPL",
		Market:   core.MarketUS,
		Side:     broker.OrderSideBuy,
		Type:     broker.OrderTypeLimit,
		Quantity: 100,
		Price:    150.00,
	}

	order, err := m.PlaceOrder(ctx, req)
	if err != nil {
		t.Fatalf("PlaceOrder failed: %v", err)
	}

	if err := m.CancelOrder(ctx, order.OrderID); err != nil {
		t.Fatalf("CancelOrder failed: %v", err)
	}

	// Give handler time to receive update
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(receivedUpdates) != 1 {
		t.Fatalf("expected 1 update, got: %d", len(receivedUpdates))
	}

	update := receivedUpdates[0]
	if update.Event != "cancelled" {
		t.Errorf("expected event 'cancelled', got: %s", update.Event)
	}
}

func TestMockBroker_Unsubscribe(t *testing.T) {
	m := New()
	ctx := context.Background()

	m.SetFillDelay(50 * time.Millisecond)

	if err := m.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer m.Disconnect()

	// Set up handler
	var mu sync.Mutex
	callCount := 0

	handler := func(update broker.OrderUpdate) {
		mu.Lock()
		defer mu.Unlock()
		callCount++
	}

	if err := m.Subscribe(handler); err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	// Unsubscribe
	if err := m.Unsubscribe(); err != nil {
		t.Fatalf("Unsubscribe failed: %v", err)
	}

	// Place order - handler should not be called
	req := broker.OrderRequest{
		Symbol:   "AAPL",
		Market:   core.MarketUS,
		Side:     broker.OrderSideBuy,
		Type:     broker.OrderTypeLimit,
		Quantity: 100,
		Price:    150.00,
	}

	_, err := m.PlaceOrder(ctx, req)
	if err != nil {
		t.Fatalf("PlaceOrder failed: %v", err)
	}

	m.WaitForFills()
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if callCount != 0 {
		t.Errorf("expected 0 handler calls after unsubscribe, got: %d", callCount)
	}
}

func TestMockBroker_SetShouldFail(t *testing.T) {
	m := New()
	ctx := context.Background()

	if err := m.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer m.Disconnect()

	// Configure failure
	m.SetShouldFail(true, "simulated failure")

	req := broker.OrderRequest{
		Symbol:   "AAPL",
		Market:   core.MarketUS,
		Side:     broker.OrderSideBuy,
		Type:     broker.OrderTypeMarket,
		Quantity: 100,
	}

	_, err := m.PlaceOrder(ctx, req)
	if err == nil {
		t.Error("expected PlaceOrder to fail")
	}
	if err.Error() != "mock broker: simulated failure" {
		t.Errorf("unexpected error message: %v", err)
	}

	// Reset failure mode
	m.SetShouldFail(false, "")

	// Should succeed now
	order, err := m.PlaceOrder(ctx, req)
	if err != nil {
		t.Fatalf("PlaceOrder should succeed after resetting failure: %v", err)
	}
	if order == nil {
		t.Error("expected order to be returned")
	}
}

func TestMockBroker_SetBalance(t *testing.T) {
	m := New()
	ctx := context.Background()

	if err := m.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer m.Disconnect()

	// Set custom balance
	customBalance := &broker.Balance{
		Currency:        "CNY",
		Cash:            500000.00,
		BuyingPower:     500000.00,
		TotalValue:      500000.00,
		MarginUsed:      0,
		MarginAvailable: 500000.00,
		UpdatedAt:       time.Now(),
	}
	m.SetBalance(customBalance)

	// Retrieve and verify
	balance, err := m.GetBalance(ctx)
	if err != nil {
		t.Fatalf("GetBalance failed: %v", err)
	}

	if balance.Currency != "CNY" {
		t.Errorf("expected currency CNY, got: %s", balance.Currency)
	}
	if balance.Cash != 500000.00 {
		t.Errorf("expected cash 500000.00, got: %f", balance.Cash)
	}
}

func TestMockBroker_AddPosition(t *testing.T) {
	m := New()
	ctx := context.Background()

	if err := m.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer m.Disconnect()

	// Add custom position
	m.AddPosition(broker.Position{
		Symbol:       "GOOGL",
		Market:       core.MarketUS,
		Quantity:     50,
		AverageCost:  2500.00,
		CurrentPrice: 2600.00,
		MarketValue:  130000.00,
	})

	// Retrieve and verify
	pos, err := m.GetPosition(ctx, "GOOGL")
	if err != nil {
		t.Fatalf("GetPosition failed: %v", err)
	}

	if pos.Symbol != "GOOGL" {
		t.Errorf("expected symbol GOOGL, got: %s", pos.Symbol)
	}
	if pos.Quantity != 50 {
		t.Errorf("expected quantity 50, got: %d", pos.Quantity)
	}

	// Check positions list
	positions, err := m.GetPositions(ctx)
	if err != nil {
		t.Fatalf("GetPositions failed: %v", err)
	}
	if len(positions) != 1 {
		t.Errorf("expected 1 position, got: %d", len(positions))
	}
}

func TestMockBroker_MarketOrderFill(t *testing.T) {
	m := New()
	ctx := context.Background()

	m.SetFillDelay(50 * time.Millisecond)

	if err := m.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer m.Disconnect()

	// Place market order (no price specified)
	req := broker.OrderRequest{
		Symbol:   "AAPL",
		Market:   core.MarketUS,
		Side:     broker.OrderSideBuy,
		Type:     broker.OrderTypeMarket,
		Quantity: 100,
	}

	order, err := m.PlaceOrder(ctx, req)
	if err != nil {
		t.Fatalf("PlaceOrder failed: %v", err)
	}

	m.WaitForFills()

	// Check filled order
	filled, err := m.GetOrder(ctx, order.OrderID)
	if err != nil {
		t.Fatalf("GetOrder failed: %v", err)
	}

	if filled.Status != broker.OrderStatusFilled {
		t.Errorf("expected status FILLED, got: %s", filled.Status)
	}
	// Market orders use default price of 100.0
	if filled.AverageFillPrice != 100.0 {
		t.Errorf("expected average fill price 100.0, got: %f", filled.AverageFillPrice)
	}
}

func TestMockBroker_NotConnectedErrors(t *testing.T) {
	m := New()
	ctx := context.Background()

	// All methods should fail when not connected
	_, err := m.GetOrder(ctx, "test")
	if err != broker.ErrNotConnected {
		t.Errorf("GetOrder: expected ErrNotConnected, got: %v", err)
	}

	_, err = m.GetOpenOrders(ctx)
	if err != broker.ErrNotConnected {
		t.Errorf("GetOpenOrders: expected ErrNotConnected, got: %v", err)
	}

	_, err = m.GetPositions(ctx)
	if err != broker.ErrNotConnected {
		t.Errorf("GetPositions: expected ErrNotConnected, got: %v", err)
	}

	_, err = m.GetPosition(ctx, "AAPL")
	if err != broker.ErrNotConnected {
		t.Errorf("GetPosition: expected ErrNotConnected, got: %v", err)
	}

	_, err = m.GetBalance(ctx)
	if err != broker.ErrNotConnected {
		t.Errorf("GetBalance: expected ErrNotConnected, got: %v", err)
	}

	err = m.CancelOrder(ctx, "test")
	if err != broker.ErrNotConnected {
		t.Errorf("CancelOrder: expected ErrNotConnected, got: %v", err)
	}

	err = m.Subscribe(func(update broker.OrderUpdate) {})
	if err != broker.ErrNotConnected {
		t.Errorf("Subscribe: expected ErrNotConnected, got: %v", err)
	}

	err = m.Unsubscribe()
	if err != broker.ErrNotConnected {
		t.Errorf("Unsubscribe: expected ErrNotConnected, got: %v", err)
	}
}

func TestMockBroker_GetPositionInvalidSymbol(t *testing.T) {
	m := New()
	ctx := context.Background()

	if err := m.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer m.Disconnect()

	_, err := m.GetPosition(ctx, "")
	if err != broker.ErrInvalidSymbol {
		t.Errorf("expected ErrInvalidSymbol, got: %v", err)
	}
}

func TestMockBroker_SubscribeNilHandler(t *testing.T) {
	m := New()
	ctx := context.Background()

	if err := m.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer m.Disconnect()

	err := m.Subscribe(nil)
	if err != broker.ErrSubscriptionFailed {
		t.Errorf("expected ErrSubscriptionFailed, got: %v", err)
	}
}

func TestMockBroker_DisconnectStopsFills(t *testing.T) {
	m := New()
	ctx := context.Background()

	// Set longer fill delay
	m.SetFillDelay(500 * time.Millisecond)

	if err := m.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	// Place order
	req := broker.OrderRequest{
		Symbol:   "AAPL",
		Market:   core.MarketUS,
		Side:     broker.OrderSideBuy,
		Type:     broker.OrderTypeLimit,
		Quantity: 100,
		Price:    150.00,
	}

	order, err := m.PlaceOrder(ctx, req)
	if err != nil {
		t.Fatalf("PlaceOrder failed: %v", err)
	}

	// Disconnect immediately (before fill)
	if err := m.Disconnect(); err != nil {
		t.Fatalf("Disconnect failed: %v", err)
	}

	// Reconnect to check order status
	if err := m.Connect(ctx); err != nil {
		t.Fatalf("Reconnect failed: %v", err)
	}
	defer m.Disconnect()

	// Order should still be pending (fill was aborted)
	retrieved, err := m.GetOrder(ctx, order.OrderID)
	if err != nil {
		t.Fatalf("GetOrder failed: %v", err)
	}

	if retrieved.Status != broker.OrderStatusPending {
		t.Errorf("expected status PENDING after disconnect, got: %s", retrieved.Status)
	}
}

func TestMockBroker_GetOrderCount(t *testing.T) {
	m := New()
	ctx := context.Background()

	m.SetFillDelay(10 * time.Second)

	if err := m.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer m.Disconnect()

	if count := m.GetOrderCount(); count != 0 {
		t.Errorf("expected 0 orders initially, got: %d", count)
	}

	req := broker.OrderRequest{
		Symbol:   "AAPL",
		Market:   core.MarketUS,
		Side:     broker.OrderSideBuy,
		Type:     broker.OrderTypeMarket,
		Quantity: 100,
	}

	_, _ = m.PlaceOrder(ctx, req)
	_, _ = m.PlaceOrder(ctx, req)

	if count := m.GetOrderCount(); count != 2 {
		t.Errorf("expected 2 orders, got: %d", count)
	}
}

// Ensure MockBroker implements broker.Broker interface at compile time
var _ broker.Broker = (*MockBroker)(nil)
