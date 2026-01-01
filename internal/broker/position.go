// Package broker provides types and interfaces for broker integrations.
package broker

import (
	"context"
	"sync"
	"time"
)

// PositionTracker tracks positions and calculates P&L from order fills.
// It maintains an in-memory cache of positions that can be synced from
// the broker and updated locally based on order fills.
type PositionTracker struct {
	broker    Broker
	positions map[string]*Position // symbol -> position
	lastSync  time.Time
	mu        sync.RWMutex
}

// NewPositionTracker creates a new PositionTracker with the given broker.
func NewPositionTracker(broker Broker) *PositionTracker {
	return &PositionTracker{
		broker:    broker,
		positions: make(map[string]*Position),
	}
}

// Sync synchronizes positions from the broker.
// It fetches all positions from the broker and updates the local cache.
func (pt *PositionTracker) Sync(ctx context.Context) error {
	positions, err := pt.broker.GetPositions(ctx)
	if err != nil {
		return err
	}

	pt.mu.Lock()
	defer pt.mu.Unlock()

	// Clear existing positions and replace with broker data
	pt.positions = make(map[string]*Position)
	for i := range positions {
		pos := positions[i]
		pt.positions[pos.Symbol] = &pos
	}
	pt.lastSync = time.Now()

	return nil
}

// GetPosition returns the current position for a symbol.
// If no position exists, returns an empty Position with zero quantity.
func (pt *PositionTracker) GetPosition(symbol string) *Position {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	if pos, exists := pt.positions[symbol]; exists {
		// Return a copy to prevent external modification
		posCopy := *pos
		return &posCopy
	}

	// Return empty position for unknown symbol
	return &Position{
		Symbol:    symbol,
		Quantity:  0,
		UpdatedAt: time.Now(),
	}
}

// GetAllPositions returns all non-zero positions.
func (pt *PositionTracker) GetAllPositions() []Position {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	positions := make([]Position, 0, len(pt.positions))
	for _, pos := range pt.positions {
		if pos.Quantity != 0 {
			positions = append(positions, *pos)
		}
	}
	return positions
}

// UpdateOnFill updates a position based on an order fill.
// For BUY orders: adds to quantity and calculates weighted average cost.
// For SELL orders: reduces quantity and calculates realized P&L.
func (pt *PositionTracker) UpdateOnFill(order *Order) {
	if order == nil || order.FilledQuantity == 0 {
		return
	}

	pt.mu.Lock()
	defer pt.mu.Unlock()

	pos, exists := pt.positions[order.Symbol]
	if !exists {
		pos = &Position{
			Symbol:    order.Symbol,
			Market:    order.Market,
			Quantity:  0,
			UpdatedAt: time.Now(),
		}
		pt.positions[order.Symbol] = pos
	}

	fillPrice := order.AverageFillPrice
	filledQty := order.FilledQuantity

	if order.Side == OrderSideBuy {
		// BUY: add to quantity and calculate weighted average cost
		// new avg cost = (old_cost * old_qty + fill_price * fill_qty) / (old_qty + fill_qty)
		oldQty := pos.Quantity
		oldCost := pos.AverageCost

		totalCost := float64(oldQty)*oldCost + fillPrice*float64(filledQty)
		newQty := oldQty + filledQty

		pos.Quantity = newQty
		if newQty > 0 {
			pos.AverageCost = totalCost / float64(newQty)
		}
	} else if order.Side == OrderSideSell {
		// SELL: reduce quantity and calculate realized P&L
		// realized P&L += (fill_price - avg_cost) * filled_qty
		realizedPL := (fillPrice - pos.AverageCost) * float64(filledQty)
		pos.RealizedPL += realizedPL
		pos.Quantity -= filledQty
	}

	// Update current price and derived values
	pos.CurrentPrice = fillPrice
	pos.MarketValue = float64(pos.Quantity) * pos.CurrentPrice
	pos.CostBasis = float64(pos.Quantity) * pos.AverageCost
	pos.UnrealizedPL = pos.MarketValue - pos.CostBasis
	if pos.CostBasis > 0 {
		pos.UnrealizedPLPercent = (pos.UnrealizedPL / pos.CostBasis) * 100
	} else {
		pos.UnrealizedPLPercent = 0
	}
	pos.UpdatedAt = time.Now()

	// Remove position if quantity is zero
	if pos.Quantity == 0 {
		delete(pt.positions, order.Symbol)
	}
}

// LastSyncTime returns the time of the last successful sync.
func (pt *PositionTracker) LastSyncTime() time.Time {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	return pt.lastSync
}

// TotalUnrealizedPL returns the sum of unrealized P&L across all positions.
func (pt *PositionTracker) TotalUnrealizedPL() float64 {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	var total float64
	for _, pos := range pt.positions {
		total += pos.UnrealizedPL
	}
	return total
}

// TotalRealizedPL returns the sum of realized P&L across all positions.
func (pt *PositionTracker) TotalRealizedPL() float64 {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	var total float64
	for _, pos := range pt.positions {
		total += pos.RealizedPL
	}
	return total
}
