// Package paper provides an in-memory simulated broker that implements
// broker.Broker. Market orders fill immediately and fully at the price
// carried by the order request, while cash, positions, and orders are tracked
// entirely in memory. See design-spec D1.1.
package paper

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/newthinker/atlas/internal/broker"
	"github.com/newthinker/atlas/internal/core"
)

// defaultInitialCash is used when New receives a non-positive initial cash.
const defaultInitialCash = 1_000_000.0

// PaperBroker is an in-memory simulated broker.
type PaperBroker struct {
	mu        sync.RWMutex
	connected bool
	cash      float64
	positions map[string]broker.Position
	orders    map[string]broker.Order
	counter   int64
	handler   broker.OrderUpdateHandler
}

// New creates a PaperBroker with the given initial cash. A non-positive value
// falls back to defaultInitialCash. The instance starts disconnected.
func New(initialCash float64) *PaperBroker {
	if initialCash <= 0 {
		initialCash = defaultInitialCash
	}
	return &PaperBroker{
		cash:      initialCash,
		positions: make(map[string]broker.Position),
		orders:    make(map[string]broker.Order),
	}
}

// Name returns the broker identifier.
func (p *PaperBroker) Name() string { return "paper" }

// SupportedMarkets returns all markets supported by the paper broker.
func (p *PaperBroker) SupportedMarkets() []core.Market {
	return []core.Market{core.MarketUS, core.MarketCNA, core.MarketHK, core.MarketCrypto}
}

// Connect marks the broker as connected.
func (p *PaperBroker) Connect(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.connected {
		return broker.ErrAlreadyConnected
	}
	p.connected = true
	return nil
}

// Disconnect marks the broker as disconnected.
func (p *PaperBroker) Disconnect() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.connected = false
	return nil
}

// IsConnected reports the connection status.
func (p *PaperBroker) IsConnected() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.connected
}

// PlaceOrder simulates an immediate, full fill at the request price.
func (p *PaperBroker) PlaceOrder(ctx context.Context, request broker.OrderRequest) (*broker.Order, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	// In paper mode every order fills at the price carried by the request.
	if request.Price <= 0 {
		return nil, broker.ErrInvalidPrice
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.connected {
		return nil, broker.ErrNotConnected
	}

	if err := p.applyFill(request); err != nil {
		return nil, err
	}

	order := p.buildFilledOrder(request)
	p.orders[order.OrderID] = order

	if p.handler != nil {
		p.handler(broker.OrderUpdate{
			Order:     order,
			Event:     "filled",
			Timestamp: order.UpdatedAt,
		})
	}
	return &order, nil
}

// applyFill mutates cash and positions for a fill. Caller must hold the lock.
func (p *PaperBroker) applyFill(req broker.OrderRequest) error {
	notional := float64(req.Quantity) * req.Price
	pos := p.positions[req.Symbol]

	switch req.Side {
	case broker.OrderSideBuy:
		if notional > p.cash {
			return broker.ErrInsufficientFunds
		}
		p.cash -= notional
		totalCost := pos.AverageCost*float64(pos.Quantity) + notional
		pos.Quantity += req.Quantity
		pos.AverageCost = totalCost / float64(pos.Quantity)
	case broker.OrderSideSell:
		if req.Quantity > pos.Quantity {
			return fmt.Errorf("%w: cannot sell %d, holding %d", broker.ErrInvalidQuantity, req.Quantity, pos.Quantity)
		}
		p.cash += notional
		pos.Quantity -= req.Quantity
	default:
		return broker.ErrInvalidOrderType
	}

	if pos.Quantity == 0 {
		delete(p.positions, req.Symbol)
		return nil
	}
	pos.Symbol = req.Symbol
	pos.Market = req.Market
	pos.CurrentPrice = req.Price
	pos.CostBasis = pos.AverageCost * float64(pos.Quantity)
	pos.MarketValue = pos.CurrentPrice * float64(pos.Quantity)
	pos.UnrealizedPL = pos.MarketValue - pos.CostBasis
	if pos.CostBasis != 0 {
		pos.UnrealizedPLPercent = pos.UnrealizedPL / pos.CostBasis * 100
	}
	pos.UpdatedAt = time.Now()
	p.positions[req.Symbol] = pos
	return nil
}

// buildFilledOrder constructs a fully filled order. Caller must hold the lock.
func (p *PaperBroker) buildFilledOrder(req broker.OrderRequest) broker.Order {
	p.counter++
	now := time.Now()
	filledAt := now
	return broker.Order{
		OrderID:          fmt.Sprintf("PAPER-%d", p.counter),
		ClientOrderID:    req.ClientOrderID,
		Symbol:           req.Symbol,
		Market:           req.Market,
		Side:             req.Side,
		Type:             req.Type,
		Quantity:         req.Quantity,
		Price:            req.Price,
		Status:           broker.OrderStatusFilled,
		FilledQuantity:   req.Quantity,
		AverageFillPrice: req.Price,
		CreatedAt:        now,
		UpdatedAt:        now,
		FilledAt:         &filledAt,
	}
}

// CancelOrder rejects cancellation: all orders fill immediately, so any lookup
// of an unknown order returns an error and filled orders are not cancellable.
func (p *PaperBroker) CancelOrder(ctx context.Context, orderID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.connected {
		return broker.ErrNotConnected
	}
	o, ok := p.orders[orderID]
	if !ok {
		return broker.ErrOrderNotFound
	}
	if o.IsTerminal() {
		return broker.ErrOrderNotCancellable
	}
	return nil
}

// GetOrder returns a stored order by ID.
func (p *PaperBroker) GetOrder(ctx context.Context, orderID string) (*broker.Order, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if !p.connected {
		return nil, broker.ErrNotConnected
	}
	o, ok := p.orders[orderID]
	if !ok {
		return nil, broker.ErrOrderNotFound
	}
	return &o, nil
}

// GetOpenOrders returns open orders. Paper orders fill immediately, so this is
// always empty.
func (p *PaperBroker) GetOpenOrders(ctx context.Context) ([]broker.Order, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if !p.connected {
		return nil, broker.ErrNotConnected
	}
	var open []broker.Order
	for _, o := range p.orders {
		if o.IsOpen() {
			open = append(open, o)
		}
	}
	return open, nil
}

// GetPositions returns a snapshot of all held positions.
func (p *PaperBroker) GetPositions(ctx context.Context) ([]broker.Position, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if !p.connected {
		return nil, broker.ErrNotConnected
	}
	positions := make([]broker.Position, 0, len(p.positions))
	for _, pos := range p.positions {
		positions = append(positions, pos)
	}
	return positions, nil
}

// GetPosition returns the position for a symbol, or ErrPositionNotFound.
func (p *PaperBroker) GetPosition(ctx context.Context, symbol string) (*broker.Position, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if !p.connected {
		return nil, broker.ErrNotConnected
	}
	pos, ok := p.positions[symbol]
	if !ok {
		return nil, broker.ErrPositionNotFound
	}
	return &pos, nil
}

// GetBalance returns the current account balance.
func (p *PaperBroker) GetBalance(ctx context.Context) (*broker.Balance, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if !p.connected {
		return nil, broker.ErrNotConnected
	}
	total := p.cash
	for _, pos := range p.positions {
		total += pos.MarketValue
	}
	return &broker.Balance{
		Cash:        p.cash,
		BuyingPower: p.cash,
		TotalValue:  total,
		UpdatedAt:   time.Now(),
	}, nil
}

// Subscribe registers an order update handler.
func (p *PaperBroker) Subscribe(handler broker.OrderUpdateHandler) error {
	if handler == nil {
		return broker.ErrSubscriptionFailed
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.handler = handler
	return nil
}

// Unsubscribe removes the order update handler.
func (p *PaperBroker) Unsubscribe() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.handler = nil
	return nil
}
