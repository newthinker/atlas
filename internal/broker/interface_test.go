// internal/broker/interface_test.go
package broker

import (
	"testing"
)

func TestOrderSide_Values(t *testing.T) {
	tests := []struct {
		side OrderSide
		want string
	}{
		{OrderSideBuy, "buy"},
		{OrderSideSell, "sell"},
	}

	for _, tt := range tests {
		if string(tt.side) != tt.want {
			t.Errorf("expected %s, got %s", tt.want, tt.side)
		}
	}
}

func TestOrderType_Values(t *testing.T) {
	tests := []struct {
		typ  OrderType
		want string
	}{
		{OrderTypeMarket, "market"},
		{OrderTypeLimit, "limit"},
		{OrderTypeStop, "stop"},
	}

	for _, tt := range tests {
		if string(tt.typ) != tt.want {
			t.Errorf("expected %s, got %s", tt.want, tt.typ)
		}
	}
}

func TestOrderStatus_Values(t *testing.T) {
	tests := []struct {
		status OrderStatus
		want   string
	}{
		{OrderStatusPending, "pending"},
		{OrderStatusOpen, "open"},
		{OrderStatusFilled, "filled"},
		{OrderStatusCancelled, "cancelled"},
		{OrderStatusRejected, "rejected"},
	}

	for _, tt := range tests {
		if string(tt.status) != tt.want {
			t.Errorf("expected %s, got %s", tt.want, tt.status)
		}
	}
}
