// internal/broker/mock/mock_test.go
package mock

import (
	"context"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/broker"
	"github.com/newthinker/atlas/internal/core"
)

func TestMockBroker_ImplementsInterface(t *testing.T) {
	var _ broker.Broker = (*MockBroker)(nil)
}

func TestMockBroker_Connection(t *testing.T) {
	m := New()
	ctx := context.Background()

	if m.IsConnected() {
		t.Error("expected not connected initially")
	}

	if err := m.Connect(ctx); err != nil {
		t.Fatalf("connect error: %v", err)
	}

	if !m.IsConnected() {
		t.Error("expected connected after Connect")
	}

	if err := m.Disconnect(); err != nil {
		t.Fatalf("disconnect error: %v", err)
	}

	if m.IsConnected() {
		t.Error("expected not connected after Disconnect")
	}
}

func TestMockBroker_GetPositions(t *testing.T) {
	m := New()
	ctx := context.Background()

	// Should fail when not connected
	_, err := m.GetPositions(ctx)
	if err == nil {
		t.Error("expected error when not connected")
	}

	m.Connect(ctx)
	positions, err := m.GetPositions(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(positions) != 2 {
		t.Errorf("expected 2 positions, got %d", len(positions))
	}
}

func TestMockBroker_GetAccountInfo(t *testing.T) {
	m := New()
	ctx := context.Background()

	m.Connect(ctx)
	info, err := m.GetAccountInfo(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if info.TotalAssets != 150000.00 {
		t.Errorf("expected 150000.00 total assets, got %f", info.TotalAssets)
	}
}

func TestMockBroker_PlaceOrder(t *testing.T) {
	m := New()
	ctx := context.Background()

	m.Connect(ctx)
	order, err := m.PlaceOrder(ctx, broker.OrderRequest{
		Symbol:   "GOOG",
		Market:   core.MarketUS,
		Side:     broker.OrderSideBuy,
		Type:     broker.OrderTypeLimit,
		Quantity: 10,
		Price:    100.00,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if order.Symbol != "GOOG" {
		t.Errorf("expected GOOG, got %s", order.Symbol)
	}
	if order.Status != broker.OrderStatusOpen {
		t.Errorf("expected open status, got %s", order.Status)
	}
}

func TestMockBroker_CancelOrder(t *testing.T) {
	m := New()
	ctx := context.Background()

	m.Connect(ctx)
	order, _ := m.PlaceOrder(ctx, broker.OrderRequest{
		Symbol:   "GOOG",
		Market:   core.MarketUS,
		Side:     broker.OrderSideBuy,
		Quantity: 10,
	})

	err := m.CancelOrder(ctx, order.OrderID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	orders, _ := m.GetOrders(ctx, broker.OrderFilter{})
	if orders[0].Status != broker.OrderStatusCancelled {
		t.Errorf("expected cancelled status, got %s", orders[0].Status)
	}
}

func TestMockBroker_GetTradeHistory(t *testing.T) {
	m := New()
	ctx := context.Background()

	m.Connect(ctx)

	// Add a trade
	m.AddTrade(broker.Trade{
		TradeID:   "T1",
		Symbol:    "AAPL",
		Side:      broker.OrderSideBuy,
		Quantity:  100,
		Price:     150.00,
		Timestamp: time.Now().Add(-24 * time.Hour),
	})

	trades, err := m.GetTradeHistory(ctx, time.Now().Add(-48*time.Hour), time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(trades) != 1 {
		t.Errorf("expected 1 trade, got %d", len(trades))
	}
}

func TestMockBroker_OrderFilter(t *testing.T) {
	m := New()
	ctx := context.Background()

	m.Connect(ctx)

	// Place multiple orders
	m.PlaceOrder(ctx, broker.OrderRequest{Symbol: "AAPL", Side: broker.OrderSideBuy, Quantity: 10})
	m.PlaceOrder(ctx, broker.OrderRequest{Symbol: "GOOG", Side: broker.OrderSideSell, Quantity: 5})

	// Filter by symbol
	orders, _ := m.GetOrders(ctx, broker.OrderFilter{Symbol: "AAPL"})
	if len(orders) != 1 {
		t.Errorf("expected 1 order for AAPL, got %d", len(orders))
	}

	// Filter by side
	orders, _ = m.GetOrders(ctx, broker.OrderFilter{Side: broker.OrderSideSell})
	if len(orders) != 1 {
		t.Errorf("expected 1 sell order, got %d", len(orders))
	}
}
