package broker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockBrokerForExecution implements Broker interface for execution tests.
type mockBrokerForExecution struct {
	balance      *Balance
	positions    []Position
	orders       []*Order
	orderSeq     int
	placeErr     error
	balanceErr   error
	positionsErr error
}

func newMockBrokerForExecution() *mockBrokerForExecution {
	return &mockBrokerForExecution{
		balance: &Balance{
			Currency:   "USD",
			Cash:       100000,
			TotalValue: 100000,
		},
		positions: []Position{},
		orders:    []*Order{},
	}
}

func (m *mockBrokerForExecution) Name() string                         { return "mock" }
func (m *mockBrokerForExecution) SupportedMarkets() []core.Market      { return []core.Market{core.MarketUS} }
func (m *mockBrokerForExecution) Connect(ctx context.Context) error    { return nil }
func (m *mockBrokerForExecution) Disconnect() error                    { return nil }
func (m *mockBrokerForExecution) IsConnected() bool                    { return true }
func (m *mockBrokerForExecution) CancelOrder(ctx context.Context, orderID string) error { return nil }
func (m *mockBrokerForExecution) GetOrder(ctx context.Context, orderID string) (*Order, error) {
	return nil, nil
}
func (m *mockBrokerForExecution) GetOpenOrders(ctx context.Context) ([]Order, error) { return nil, nil }
func (m *mockBrokerForExecution) GetPosition(ctx context.Context, symbol string) (*Position, error) {
	return nil, nil
}
func (m *mockBrokerForExecution) Subscribe(handler OrderUpdateHandler) error { return nil }
func (m *mockBrokerForExecution) Unsubscribe() error                          { return nil }

func (m *mockBrokerForExecution) GetBalance(ctx context.Context) (*Balance, error) {
	if m.balanceErr != nil {
		return nil, m.balanceErr
	}
	return m.balance, nil
}

func (m *mockBrokerForExecution) GetPositions(ctx context.Context) ([]Position, error) {
	if m.positionsErr != nil {
		return nil, m.positionsErr
	}
	return m.positions, nil
}

func (m *mockBrokerForExecution) PlaceOrder(ctx context.Context, req OrderRequest) (*Order, error) {
	if m.placeErr != nil {
		return nil, m.placeErr
	}
	m.orderSeq++
	now := time.Now()
	order := &Order{
		OrderID:          fmt.Sprintf("order-%d", m.orderSeq),
		Symbol:           req.Symbol,
		Market:           req.Market,
		Side:             req.Side,
		Type:             req.Type,
		Quantity:         req.Quantity,
		Price:            req.Price,
		Status:           OrderStatusFilled,
		FilledQuantity:   req.Quantity,
		AverageFillPrice: 150.0,
		CreatedAt:        now,
		UpdatedAt:        now,
		FilledAt:         &now,
	}
	m.orders = append(m.orders, order)
	return order, nil
}

func TestExecutionManager_AutoMode_ExecutesImmediately(t *testing.T) {
	broker := newMockBrokerForExecution()
	risk := NewRiskChecker(DefaultRiskConfig(), broker)
	tracker := NewPositionTracker(broker)

	config := ExecutionConfig{
		Mode:           ExecutionAuto,
		DefaultSizePct: 5.0,
	}
	em := NewExecutionManager(config, broker, risk, tracker)

	signal := &core.Signal{
		Symbol:      "AAPL",
		Action:      core.ActionBuy,
		Confidence:  0.8,
		GeneratedAt: time.Now(),
	}

	result, err := em.Execute(context.Background(), signal, 150.0)
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.NotNil(t, result.Order)
	assert.Empty(t, result.PendingID)
	assert.Equal(t, "AAPL", result.Order.Symbol)
	assert.Equal(t, OrderSideBuy, result.Order.Side)
	assert.Len(t, broker.orders, 1)
}

func TestExecutionManager_ConfirmMode_QueuesOrder(t *testing.T) {
	broker := newMockBrokerForExecution()
	risk := NewRiskChecker(DefaultRiskConfig(), broker)
	tracker := NewPositionTracker(broker)

	config := ExecutionConfig{
		Mode:           ExecutionConfirm,
		DefaultSizePct: 5.0,
	}
	em := NewExecutionManager(config, broker, risk, tracker)

	signal := &core.Signal{
		Symbol:      "AAPL",
		Action:      core.ActionBuy,
		Confidence:  0.8,
		GeneratedAt: time.Now(),
	}

	result, err := em.Execute(context.Background(), signal, 150.0)
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Nil(t, result.Order)
	assert.NotEmpty(t, result.PendingID)
	assert.Len(t, broker.orders, 0) // No order placed yet

	// Verify pending order exists
	pending := em.GetPendingOrders()
	assert.Len(t, pending, 1)
	assert.Equal(t, result.PendingID, pending[0].ID)
	assert.Equal(t, "AAPL", pending[0].Request.Symbol)
}

func TestExecutionManager_Confirm_ExecutesPendingOrder(t *testing.T) {
	broker := newMockBrokerForExecution()
	risk := NewRiskChecker(DefaultRiskConfig(), broker)
	tracker := NewPositionTracker(broker)

	config := ExecutionConfig{
		Mode:           ExecutionConfirm,
		DefaultSizePct: 5.0,
	}
	em := NewExecutionManager(config, broker, risk, tracker)

	signal := &core.Signal{
		Symbol:      "AAPL",
		Action:      core.ActionBuy,
		Confidence:  0.8,
		GeneratedAt: time.Now(),
	}

	// Queue the order
	result, err := em.Execute(context.Background(), signal, 150.0)
	require.NoError(t, err)
	pendingID := result.PendingID

	// Confirm the order
	confirmResult, err := em.Confirm(context.Background(), pendingID)
	require.NoError(t, err)
	assert.True(t, confirmResult.Success)
	assert.NotNil(t, confirmResult.Order)
	assert.Len(t, broker.orders, 1)

	// Verify pending order is removed
	pending := em.GetPendingOrders()
	assert.Len(t, pending, 0)
}

func TestExecutionManager_Reject_RemovesPendingOrder(t *testing.T) {
	broker := newMockBrokerForExecution()
	risk := NewRiskChecker(DefaultRiskConfig(), broker)
	tracker := NewPositionTracker(broker)

	config := ExecutionConfig{
		Mode:           ExecutionConfirm,
		DefaultSizePct: 5.0,
	}
	em := NewExecutionManager(config, broker, risk, tracker)

	signal := &core.Signal{
		Symbol:      "AAPL",
		Action:      core.ActionBuy,
		Confidence:  0.8,
		GeneratedAt: time.Now(),
	}

	// Queue the order
	result, err := em.Execute(context.Background(), signal, 150.0)
	require.NoError(t, err)
	pendingID := result.PendingID

	// Reject the order
	err = em.Reject(pendingID)
	require.NoError(t, err)

	// Verify pending order is removed
	pending := em.GetPendingOrders()
	assert.Len(t, pending, 0)
	assert.Len(t, broker.orders, 0) // No order was placed
}

func TestExecutionManager_RiskRejection_ReturnsFailure(t *testing.T) {
	broker := newMockBrokerForExecution()
	// Configure high daily loss to trigger risk check failure
	broker.balance.DailyPL = -6000 // 6% loss exceeds default 5% limit

	risk := NewRiskChecker(DefaultRiskConfig(), broker)
	tracker := NewPositionTracker(broker)

	config := ExecutionConfig{
		Mode:           ExecutionAuto,
		DefaultSizePct: 5.0,
	}
	em := NewExecutionManager(config, broker, risk, tracker)

	signal := &core.Signal{
		Symbol:      "AAPL",
		Action:      core.ActionBuy,
		Confidence:  0.8,
		GeneratedAt: time.Now(),
	}

	result, err := em.Execute(context.Background(), signal, 150.0)
	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Message, "risk check failed")
	assert.Contains(t, result.Message, "daily loss limit")
	assert.Len(t, broker.orders, 0) // No order was placed
}

func TestExecutionManager_GetPendingOrders_ListsQueuedOrders(t *testing.T) {
	broker := newMockBrokerForExecution()
	risk := NewRiskChecker(DefaultRiskConfig(), broker)
	tracker := NewPositionTracker(broker)

	config := ExecutionConfig{
		Mode:           ExecutionConfirm,
		DefaultSizePct: 5.0,
	}
	em := NewExecutionManager(config, broker, risk, tracker)

	// Queue multiple orders
	signals := []*core.Signal{
		{Symbol: "AAPL", Action: core.ActionBuy, GeneratedAt: time.Now()},
		{Symbol: "GOOGL", Action: core.ActionSell, GeneratedAt: time.Now()},
		{Symbol: "MSFT", Action: core.ActionBuy, GeneratedAt: time.Now()},
	}

	for _, sig := range signals {
		_, err := em.Execute(context.Background(), sig, 100.0)
		require.NoError(t, err)
	}

	pending := em.GetPendingOrders()
	assert.Len(t, pending, 3)

	// Verify all symbols are present
	symbols := make(map[string]bool)
	for _, p := range pending {
		symbols[p.Request.Symbol] = true
	}
	assert.True(t, symbols["AAPL"])
	assert.True(t, symbols["GOOGL"])
	assert.True(t, symbols["MSFT"])
}

func TestExecutionManager_SellAction_SetsSellSide(t *testing.T) {
	broker := newMockBrokerForExecution()
	risk := NewRiskChecker(DefaultRiskConfig(), broker)
	tracker := NewPositionTracker(broker)

	config := ExecutionConfig{
		Mode:           ExecutionAuto,
		DefaultSizePct: 5.0,
	}
	em := NewExecutionManager(config, broker, risk, tracker)

	signal := &core.Signal{
		Symbol:      "AAPL",
		Action:      core.ActionSell,
		Confidence:  0.8,
		GeneratedAt: time.Now(),
	}

	result, err := em.Execute(context.Background(), signal, 150.0)
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, OrderSideSell, result.Order.Side)
}

func TestExecutionManager_HoldAction_NoExecution(t *testing.T) {
	broker := newMockBrokerForExecution()
	risk := NewRiskChecker(DefaultRiskConfig(), broker)
	tracker := NewPositionTracker(broker)

	config := ExecutionConfig{
		Mode:           ExecutionAuto,
		DefaultSizePct: 5.0,
	}
	em := NewExecutionManager(config, broker, risk, tracker)

	signal := &core.Signal{
		Symbol:      "AAPL",
		Action:      core.ActionHold,
		Confidence:  0.8,
		GeneratedAt: time.Now(),
	}

	result, err := em.Execute(context.Background(), signal, 150.0)
	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Message, "does not require execution")
	assert.Len(t, broker.orders, 0)
}

func TestExecutionManager_Confirm_NotFound(t *testing.T) {
	broker := newMockBrokerForExecution()
	risk := NewRiskChecker(DefaultRiskConfig(), broker)
	tracker := NewPositionTracker(broker)

	config := ExecutionConfig{
		Mode:           ExecutionConfirm,
		DefaultSizePct: 5.0,
	}
	em := NewExecutionManager(config, broker, risk, tracker)

	_, err := em.Confirm(context.Background(), "non-existent-id")
	assert.ErrorIs(t, err, ErrPendingOrderNotFound)
}

func TestExecutionManager_Reject_NotFound(t *testing.T) {
	broker := newMockBrokerForExecution()
	risk := NewRiskChecker(DefaultRiskConfig(), broker)
	tracker := NewPositionTracker(broker)

	config := ExecutionConfig{
		Mode:           ExecutionConfirm,
		DefaultSizePct: 5.0,
	}
	em := NewExecutionManager(config, broker, risk, tracker)

	err := em.Reject("non-existent-id")
	assert.ErrorIs(t, err, ErrPendingOrderNotFound)
}

func TestExecutionManager_NilSignal_ReturnsError(t *testing.T) {
	broker := newMockBrokerForExecution()
	risk := NewRiskChecker(DefaultRiskConfig(), broker)
	tracker := NewPositionTracker(broker)

	config := ExecutionConfig{
		Mode:           ExecutionAuto,
		DefaultSizePct: 5.0,
	}
	em := NewExecutionManager(config, broker, risk, tracker)

	_, err := em.Execute(context.Background(), nil, 150.0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "signal cannot be nil")
}

func TestExecutionManager_InvalidPrice_ReturnsError(t *testing.T) {
	broker := newMockBrokerForExecution()
	risk := NewRiskChecker(DefaultRiskConfig(), broker)
	tracker := NewPositionTracker(broker)

	config := ExecutionConfig{
		Mode:           ExecutionAuto,
		DefaultSizePct: 5.0,
	}
	em := NewExecutionManager(config, broker, risk, tracker)

	signal := &core.Signal{
		Symbol:      "AAPL",
		Action:      core.ActionBuy,
		Confidence:  0.8,
		GeneratedAt: time.Now(),
	}

	_, err := em.Execute(context.Background(), signal, 0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "price must be positive")

	_, err = em.Execute(context.Background(), signal, -10)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "price must be positive")
}

func TestExecutionManager_BatchMode_QueuesLikeConfirm(t *testing.T) {
	broker := newMockBrokerForExecution()
	risk := NewRiskChecker(DefaultRiskConfig(), broker)
	tracker := NewPositionTracker(broker)

	config := ExecutionConfig{
		Mode:           ExecutionBatch,
		BatchTime:      "09:30",
		DefaultSizePct: 5.0,
	}
	em := NewExecutionManager(config, broker, risk, tracker)

	signal := &core.Signal{
		Symbol:      "AAPL",
		Action:      core.ActionBuy,
		Confidence:  0.8,
		GeneratedAt: time.Now(),
	}

	result, err := em.Execute(context.Background(), signal, 150.0)
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Nil(t, result.Order)
	assert.NotEmpty(t, result.PendingID)
	assert.Len(t, broker.orders, 0) // No order placed yet

	pending := em.GetPendingOrders()
	assert.Len(t, pending, 1)
}

func TestExecutionManager_StrongBuyAction_SetsBuySide(t *testing.T) {
	broker := newMockBrokerForExecution()
	risk := NewRiskChecker(DefaultRiskConfig(), broker)
	tracker := NewPositionTracker(broker)

	config := ExecutionConfig{
		Mode:           ExecutionAuto,
		DefaultSizePct: 5.0,
	}
	em := NewExecutionManager(config, broker, risk, tracker)

	signal := &core.Signal{
		Symbol:      "AAPL",
		Action:      core.ActionStrongBuy,
		Confidence:  0.9,
		GeneratedAt: time.Now(),
	}

	result, err := em.Execute(context.Background(), signal, 150.0)
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, OrderSideBuy, result.Order.Side)
}

func TestExecutionManager_StrongSellAction_SetsSellSide(t *testing.T) {
	broker := newMockBrokerForExecution()
	risk := NewRiskChecker(DefaultRiskConfig(), broker)
	tracker := NewPositionTracker(broker)

	config := ExecutionConfig{
		Mode:           ExecutionAuto,
		DefaultSizePct: 5.0,
	}
	em := NewExecutionManager(config, broker, risk, tracker)

	signal := &core.Signal{
		Symbol:      "AAPL",
		Action:      core.ActionStrongSell,
		Confidence:  0.9,
		GeneratedAt: time.Now(),
	}

	result, err := em.Execute(context.Background(), signal, 150.0)
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, OrderSideSell, result.Order.Side)
}

func TestExecutionManager_CalculatesCorrectQuantity(t *testing.T) {
	broker := newMockBrokerForExecution()
	broker.balance.TotalValue = 100000

	risk := NewRiskChecker(DefaultRiskConfig(), broker)
	tracker := NewPositionTracker(broker)

	config := ExecutionConfig{
		Mode:           ExecutionConfirm, // Use confirm to inspect pending order
		DefaultSizePct: 5.0,              // 5% of 100000 = 5000
	}
	em := NewExecutionManager(config, broker, risk, tracker)

	signal := &core.Signal{
		Symbol:      "AAPL",
		Action:      core.ActionBuy,
		Confidence:  0.8,
		GeneratedAt: time.Now(),
	}

	// At price 100, quantity should be 5000 / 100 = 50
	result, err := em.Execute(context.Background(), signal, 100.0)
	require.NoError(t, err)

	pending := em.GetPendingOrders()
	require.Len(t, pending, 1)
	assert.Equal(t, int64(50), pending[0].Request.Quantity)

	// Clean up and test with different price
	err = em.Reject(result.PendingID)
	require.NoError(t, err)

	// At price 250, quantity should be 5000 / 250 = 20
	_, err = em.Execute(context.Background(), signal, 250.0)
	require.NoError(t, err)

	pending = em.GetPendingOrders()
	require.Len(t, pending, 1)
	assert.Equal(t, int64(20), pending[0].Request.Quantity)
}

func TestDefaultExecutionConfig(t *testing.T) {
	config := DefaultExecutionConfig()
	assert.Equal(t, ExecutionAuto, config.Mode)
	assert.Equal(t, "09:30", config.BatchTime)
	assert.Equal(t, 5.0, config.DefaultSizePct)
}

func TestExecutionManager_PendingOrder_HasQueuedState(t *testing.T) {
	broker := newMockBrokerForExecution()
	risk := NewRiskChecker(DefaultRiskConfig(), broker)
	tracker := NewPositionTracker(broker)

	config := ExecutionConfig{
		Mode:           ExecutionConfirm,
		DefaultSizePct: 5.0,
	}
	em := NewExecutionManager(config, broker, risk, tracker)

	signal := &core.Signal{
		Symbol:      "AAPL",
		Action:      core.ActionBuy,
		Confidence:  0.8,
		GeneratedAt: time.Now(),
	}

	_, err := em.Execute(context.Background(), signal, 150.0)
	require.NoError(t, err)

	pending := em.GetPendingOrders()
	require.Len(t, pending, 1)
	assert.Equal(t, PendingStateQueued, pending[0].State)
}

// slowMockBroker is a mock broker that delays PlaceOrder to test race conditions.
type slowMockBroker struct {
	*mockBrokerForExecution
	delay     time.Duration
	callCount atomic.Int32
}

func newSlowMockBroker(delay time.Duration) *slowMockBroker {
	return &slowMockBroker{
		mockBrokerForExecution: newMockBrokerForExecution(),
		delay:                  delay,
	}
}

func (m *slowMockBroker) PlaceOrder(ctx context.Context, req OrderRequest) (*Order, error) {
	m.callCount.Add(1)
	time.Sleep(m.delay)
	return m.mockBrokerForExecution.PlaceOrder(ctx, req)
}

func TestExecutionManager_Confirm_ConcurrentCallsOnlyOneSucceeds(t *testing.T) {
	// Use a slow broker to ensure both goroutines try to confirm at the same time
	broker := newSlowMockBroker(50 * time.Millisecond)
	risk := NewRiskChecker(DefaultRiskConfig(), broker)
	tracker := NewPositionTracker(broker)

	config := ExecutionConfig{
		Mode:           ExecutionConfirm,
		DefaultSizePct: 5.0,
	}
	em := NewExecutionManager(config, broker, risk, tracker)

	signal := &core.Signal{
		Symbol:      "AAPL",
		Action:      core.ActionBuy,
		Confidence:  0.8,
		GeneratedAt: time.Now(),
	}

	// Queue the order
	result, err := em.Execute(context.Background(), signal, 150.0)
	require.NoError(t, err)
	pendingID := result.PendingID

	// Try to confirm from multiple goroutines simultaneously
	var wg sync.WaitGroup
	successCount := atomic.Int32{}
	alreadyProcessingCount := atomic.Int32{}

	numGoroutines := 10
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			_, err := em.Confirm(context.Background(), pendingID)
			if err == nil {
				successCount.Add(1)
			} else if err.Error() == fmt.Sprintf("order already being processed: %s", pendingID) {
				alreadyProcessingCount.Add(1)
			}
		}()
	}

	wg.Wait()

	// Only one confirm should succeed
	assert.Equal(t, int32(1), successCount.Load(), "expected exactly one successful confirm")

	// All others should get "already being processed" error
	assert.Equal(t, int32(numGoroutines-1), alreadyProcessingCount.Load(),
		"expected all other confirms to get 'already being processed' error")

	// PlaceOrder should only be called once
	assert.Equal(t, int32(1), broker.callCount.Load(), "PlaceOrder should only be called once")

	// Pending order should be removed
	pending := em.GetPendingOrders()
	assert.Len(t, pending, 0)
}

func TestExecutionManager_Confirm_RestoresStateOnFailure(t *testing.T) {
	broker := newMockBrokerForExecution()
	broker.placeErr = errors.New("broker unavailable")

	risk := NewRiskChecker(DefaultRiskConfig(), broker)
	tracker := NewPositionTracker(broker)

	config := ExecutionConfig{
		Mode:           ExecutionConfirm,
		DefaultSizePct: 5.0,
	}
	em := NewExecutionManager(config, broker, risk, tracker)

	signal := &core.Signal{
		Symbol:      "AAPL",
		Action:      core.ActionBuy,
		Confidence:  0.8,
		GeneratedAt: time.Now(),
	}

	// Queue the order
	result, err := em.Execute(context.Background(), signal, 150.0)
	require.NoError(t, err)
	pendingID := result.PendingID

	// Confirm should fail
	_, err = em.Confirm(context.Background(), pendingID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "broker unavailable")

	// Pending order should still exist with queued state
	pending := em.GetPendingOrders()
	require.Len(t, pending, 1)
	assert.Equal(t, PendingStateQueued, pending[0].State)
	assert.Equal(t, pendingID, pending[0].ID)

	// Now fix the broker and try again
	broker.placeErr = nil
	confirmResult, err := em.Confirm(context.Background(), pendingID)
	require.NoError(t, err)
	assert.True(t, confirmResult.Success)
	assert.NotNil(t, confirmResult.Order)

	// Pending order should now be removed
	pending = em.GetPendingOrders()
	assert.Len(t, pending, 0)
}

func TestExecutionManager_Confirm_AlreadyProcessingError(t *testing.T) {
	broker := newSlowMockBroker(100 * time.Millisecond)
	risk := NewRiskChecker(DefaultRiskConfig(), broker)
	tracker := NewPositionTracker(broker)

	config := ExecutionConfig{
		Mode:           ExecutionConfirm,
		DefaultSizePct: 5.0,
	}
	em := NewExecutionManager(config, broker, risk, tracker)

	signal := &core.Signal{
		Symbol:      "AAPL",
		Action:      core.ActionBuy,
		Confidence:  0.8,
		GeneratedAt: time.Now(),
	}

	// Queue the order
	result, err := em.Execute(context.Background(), signal, 150.0)
	require.NoError(t, err)
	pendingID := result.PendingID

	// Start first confirmation in background
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = em.Confirm(context.Background(), pendingID)
	}()

	// Wait a bit for the first goroutine to start processing
	time.Sleep(10 * time.Millisecond)

	// Second confirm should get "already being processed" error
	_, err = em.Confirm(context.Background(), pendingID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "order already being processed")

	wg.Wait()
}
