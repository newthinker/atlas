// Package broker provides types and interfaces for broker integrations.
package broker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

// Execution-related errors.
var (
	// ErrPendingOrderNotFound indicates the pending order was not found.
	ErrPendingOrderNotFound = errors.New("execution: pending order not found")
	// ErrInvalidExecutionMode indicates an invalid execution mode.
	ErrInvalidExecutionMode = errors.New("execution: invalid execution mode")
)

// ExecutionMode determines how orders are processed.
type ExecutionMode string

const (
	// ExecutionAuto executes orders immediately.
	ExecutionAuto ExecutionMode = "auto"
	// ExecutionConfirm requires manual confirmation before execution.
	ExecutionConfirm ExecutionMode = "confirm"
	// ExecutionBatch queues orders for batch execution at a specific time.
	ExecutionBatch ExecutionMode = "batch"
)

// PendingState represents the state of a pending order.
type PendingState string

const (
	// PendingStateQueued indicates order is waiting for confirmation.
	PendingStateQueued PendingState = "queued"
	// PendingStateProcessing indicates order is being executed.
	PendingStateProcessing PendingState = "processing"
)

// ExecutionConfig holds configuration for the execution manager.
type ExecutionConfig struct {
	// Mode determines how orders are processed.
	Mode ExecutionMode
	// BatchTime is the time (HH:MM) for batch execution.
	BatchTime string
	// DefaultSizePct is the position size as a percentage of portfolio.
	DefaultSizePct float64
}

// DefaultExecutionConfig returns a sensible default configuration.
func DefaultExecutionConfig() ExecutionConfig {
	return ExecutionConfig{
		Mode:           ExecutionAuto,
		BatchTime:      "09:30",
		DefaultSizePct: 5.0,
	}
}

// PendingOrder represents an order awaiting confirmation.
type PendingOrder struct {
	// ID is the unique identifier for this pending order.
	ID string
	// Request is the order request to be executed.
	Request OrderRequest
	// Price is the price at which the order was created.
	Price float64
	// Signal is the trading signal that generated this order.
	Signal *core.Signal
	// CreatedAt is when the pending order was created.
	CreatedAt time.Time
	// State is the current state of the pending order.
	State PendingState
}

// ExecuteResult represents the outcome of an execution attempt.
type ExecuteResult struct {
	// Success indicates whether the execution was successful.
	Success bool
	// Order is the placed order (nil if pending or failed).
	Order *Order
	// PendingID is the ID for pending orders in confirm mode.
	PendingID string
	// Message provides additional context about the result.
	Message string
}

// ExecutionManager handles order execution based on configured mode.
type ExecutionManager struct {
	config     ExecutionConfig
	broker     Broker
	risk       *RiskChecker
	tracker    *PositionTracker
	pending    map[string]*PendingOrder
	orderIDSeq int
	mu         sync.RWMutex
}

// NewExecutionManager creates a new ExecutionManager with the given dependencies.
func NewExecutionManager(config ExecutionConfig, broker Broker, risk *RiskChecker, tracker *PositionTracker) *ExecutionManager {
	return &ExecutionManager{
		config:     config,
		broker:     broker,
		risk:       risk,
		tracker:    tracker,
		pending:    make(map[string]*PendingOrder),
		orderIDSeq: 0,
	}
}

// Execute processes a trading signal and executes or queues an order.
// Based on the execution mode:
// - Auto: places order immediately
// - Confirm: queues order for confirmation
// - Batch: queues order for batch execution (same as confirm for now)
func (em *ExecutionManager) Execute(ctx context.Context, signal *core.Signal, price float64) (*ExecuteResult, error) {
	if signal == nil {
		return nil, errors.New("execution: signal cannot be nil")
	}

	if price <= 0 {
		return nil, errors.New("execution: price must be positive")
	}

	// Determine order side from signal action
	var side OrderSide
	switch signal.Action {
	case core.ActionBuy, core.ActionStrongBuy:
		side = OrderSideBuy
	case core.ActionSell, core.ActionStrongSell:
		side = OrderSideSell
	default:
		return &ExecuteResult{
			Success: false,
			Message: fmt.Sprintf("signal action %s does not require execution", signal.Action),
		}, nil
	}

	// Get balance to calculate order size
	balance, err := em.broker.GetBalance(ctx)
	if err != nil {
		return nil, fmt.Errorf("execution: failed to get balance: %w", err)
	}

	// Calculate order size: (totalValue * sizePct / 100) / price
	orderValue := balance.TotalValue * (em.config.DefaultSizePct / 100)
	quantity := int64(orderValue / price)

	if quantity <= 0 {
		return &ExecuteResult{
			Success: false,
			Message: "calculated order quantity is zero or negative",
		}, nil
	}

	// Build order request
	request := OrderRequest{
		Symbol:   signal.Symbol,
		Side:     side,
		Type:     OrderTypeMarket,
		Quantity: quantity,
	}

	// Run risk check
	riskResult := em.risk.Check(ctx, request, price)
	if !riskResult.Allowed {
		return &ExecuteResult{
			Success: false,
			Message: fmt.Sprintf("risk check failed: %s", riskResult.Reason),
		}, nil
	}

	// Execute based on mode
	switch em.config.Mode {
	case ExecutionAuto:
		return em.executeImmediate(ctx, request, signal, price)
	case ExecutionConfirm, ExecutionBatch:
		return em.queueForConfirmation(request, signal, price)
	default:
		return nil, ErrInvalidExecutionMode
	}
}

// executeImmediate places an order immediately.
func (em *ExecutionManager) executeImmediate(ctx context.Context, request OrderRequest, signal *core.Signal, price float64) (*ExecuteResult, error) {
	order, err := em.broker.PlaceOrder(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("execution: failed to place order: %w", err)
	}

	// Update position tracker if order is filled
	if order.IsFilled() {
		em.tracker.UpdateOnFill(order)
	}

	return &ExecuteResult{
		Success: true,
		Order:   order,
		Message: fmt.Sprintf("order placed: %s %d %s @ market", request.Side, request.Quantity, request.Symbol),
	}, nil
}

// queueForConfirmation adds an order to the pending queue.
func (em *ExecutionManager) queueForConfirmation(request OrderRequest, signal *core.Signal, price float64) (*ExecuteResult, error) {
	em.mu.Lock()
	defer em.mu.Unlock()

	em.orderIDSeq++
	pendingID := fmt.Sprintf("pending-%d", em.orderIDSeq)

	pending := &PendingOrder{
		ID:        pendingID,
		Request:   request,
		Price:     price,
		Signal:    signal,
		CreatedAt: time.Now(),
		State:     PendingStateQueued,
	}
	em.pending[pendingID] = pending

	return &ExecuteResult{
		Success:   true,
		PendingID: pendingID,
		Message:   fmt.Sprintf("order queued for confirmation: %s %d %s", request.Side, request.Quantity, request.Symbol),
	}, nil
}

// Confirm executes a pending order.
func (em *ExecutionManager) Confirm(ctx context.Context, pendingID string) (*ExecuteResult, error) {
	em.mu.Lock()
	pending, exists := em.pending[pendingID]
	if !exists {
		em.mu.Unlock()
		return nil, ErrPendingOrderNotFound
	}
	if pending.State == PendingStateProcessing {
		em.mu.Unlock()
		return nil, fmt.Errorf("order already being processed: %s", pendingID)
	}
	// Mark as processing instead of deleting
	pending.State = PendingStateProcessing
	em.pending[pendingID] = pending
	em.mu.Unlock()

	// Place the order
	order, err := em.broker.PlaceOrder(ctx, pending.Request)
	if err != nil {
		// Restore to queued state on failure
		em.mu.Lock()
		pending.State = PendingStateQueued
		em.pending[pendingID] = pending
		em.mu.Unlock()
		return nil, fmt.Errorf("execution: failed to place order: %w", err)
	}

	// Remove from pending on success
	em.mu.Lock()
	delete(em.pending, pendingID)
	em.mu.Unlock()

	// Update position tracker if order is filled
	if order.IsFilled() {
		em.tracker.UpdateOnFill(order)
	}

	return &ExecuteResult{
		Success: true,
		Order:   order,
		Message: fmt.Sprintf("confirmed order executed: %s %d %s", pending.Request.Side, pending.Request.Quantity, pending.Request.Symbol),
	}, nil
}

// Reject removes a pending order without executing it.
func (em *ExecutionManager) Reject(pendingID string) error {
	em.mu.Lock()
	defer em.mu.Unlock()

	if _, exists := em.pending[pendingID]; !exists {
		return ErrPendingOrderNotFound
	}

	delete(em.pending, pendingID)
	return nil
}

// GetPendingOrders returns a copy of all pending orders.
func (em *ExecutionManager) GetPendingOrders() []PendingOrder {
	em.mu.RLock()
	defer em.mu.RUnlock()

	orders := make([]PendingOrder, 0, len(em.pending))
	for _, pending := range em.pending {
		orders = append(orders, *pending)
	}
	return orders
}
