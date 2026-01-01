// internal/broker/interface_test.go
package broker

import (
	"testing"
)

func TestLegacyOrderSide_Values(t *testing.T) {
	tests := []struct {
		side LegacyOrderSide
		want string
	}{
		{LegacyOrderSideBuy, "buy"},
		{LegacyOrderSideSell, "sell"},
	}

	for _, tt := range tests {
		if string(tt.side) != tt.want {
			t.Errorf("expected %s, got %s", tt.want, tt.side)
		}
	}
}

func TestLegacyOrderType_Values(t *testing.T) {
	tests := []struct {
		typ  LegacyOrderType
		want string
	}{
		{LegacyOrderTypeMarket, "market"},
		{LegacyOrderTypeLimit, "limit"},
		{LegacyOrderTypeStop, "stop"},
	}

	for _, tt := range tests {
		if string(tt.typ) != tt.want {
			t.Errorf("expected %s, got %s", tt.want, tt.typ)
		}
	}
}

func TestLegacyOrderStatus_Values(t *testing.T) {
	tests := []struct {
		status LegacyOrderStatus
		want   string
	}{
		{LegacyOrderStatusPending, "pending"},
		{LegacyOrderStatusOpen, "open"},
		{LegacyOrderStatusFilled, "filled"},
		{LegacyOrderStatusCancelled, "cancelled"},
		{LegacyOrderStatusRejected, "rejected"},
	}

	for _, tt := range tests {
		if string(tt.status) != tt.want {
			t.Errorf("expected %s, got %s", tt.want, tt.status)
		}
	}
}
