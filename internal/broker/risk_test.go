package broker_test

import (
	"context"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/broker"
	"github.com/newthinker/atlas/internal/broker/mocks"
	"github.com/newthinker/atlas/internal/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultRiskConfig(t *testing.T) {
	config := broker.DefaultRiskConfig()

	assert.Equal(t, 10.0, config.MaxPositionPct, "MaxPositionPct should be 10.0")
	assert.Equal(t, 5.0, config.MaxDailyLossPct, "MaxDailyLossPct should be 5.0")
	assert.Equal(t, 20, config.MaxOpenPositions, "MaxOpenPositions should be 20")
}

func TestRiskChecker_Check_OrderAllowed(t *testing.T) {
	// Setup mock broker
	mockBroker := mocks.New()
	err := mockBroker.Connect(context.Background())
	require.NoError(t, err)
	defer mockBroker.Disconnect()

	// Set balance with healthy account
	mockBroker.SetBalance(&broker.Balance{
		Currency:    "USD",
		Cash:        100000.00,
		BuyingPower: 100000.00,
		TotalValue:  100000.00,
		DailyPL:     0,
		UpdatedAt:   time.Now(),
	})

	config := broker.RiskConfig{
		MaxPositionPct:   10.0,
		MaxDailyLossPct:  5.0,
		MaxOpenPositions: 20,
	}

	checker := broker.NewRiskChecker(config, mockBroker)

	// Create order request within limits (5% of portfolio)
	req := broker.OrderRequest{
		Symbol:   "AAPL",
		Market:   core.MarketUS,
		Side:     broker.OrderSideBuy,
		Type:     broker.OrderTypeLimit,
		Quantity: 50,
		Price:    100.0,
	}

	ctx := context.Background()
	result := checker.Check(ctx, req, 100.0) // Order value: 50 * 100 = $5,000 (5% of $100,000)

	assert.True(t, result.Allowed, "Order should be allowed")
	assert.Empty(t, result.Reason, "Reason should be empty when allowed")
}

func TestRiskChecker_Check_PositionSizeTooLarge(t *testing.T) {
	// Setup mock broker
	mockBroker := mocks.New()
	err := mockBroker.Connect(context.Background())
	require.NoError(t, err)
	defer mockBroker.Disconnect()

	// Set balance
	mockBroker.SetBalance(&broker.Balance{
		Currency:    "USD",
		Cash:        100000.00,
		BuyingPower: 100000.00,
		TotalValue:  100000.00,
		DailyPL:     0,
		UpdatedAt:   time.Now(),
	})

	config := broker.RiskConfig{
		MaxPositionPct:   10.0,
		MaxDailyLossPct:  5.0,
		MaxOpenPositions: 20,
	}

	checker := broker.NewRiskChecker(config, mockBroker)

	// Create order request exceeding position size limit (15% of portfolio)
	req := broker.OrderRequest{
		Symbol:   "AAPL",
		Market:   core.MarketUS,
		Side:     broker.OrderSideBuy,
		Type:     broker.OrderTypeLimit,
		Quantity: 150,
		Price:    100.0,
	}

	ctx := context.Background()
	result := checker.Check(ctx, req, 100.0) // Order value: 150 * 100 = $15,000 (15% of $100,000)

	assert.False(t, result.Allowed, "Order should be rejected")
	assert.Contains(t, result.Reason, "position size too large")
	assert.Contains(t, result.Reason, "15.00%")
	assert.Contains(t, result.Reason, "10.00%")
}

func TestRiskChecker_Check_MaxPositionsReached_BuyRejected(t *testing.T) {
	// Setup mock broker
	mockBroker := mocks.New()
	err := mockBroker.Connect(context.Background())
	require.NoError(t, err)
	defer mockBroker.Disconnect()

	// Set balance
	mockBroker.SetBalance(&broker.Balance{
		Currency:    "USD",
		Cash:        100000.00,
		BuyingPower: 100000.00,
		TotalValue:  100000.00,
		DailyPL:     0,
		UpdatedAt:   time.Now(),
	})

	// Add positions up to the limit
	config := broker.RiskConfig{
		MaxPositionPct:   10.0,
		MaxDailyLossPct:  5.0,
		MaxOpenPositions: 3, // Low limit for testing
	}

	// Add 3 existing positions
	mockBroker.AddPosition(broker.Position{
		Symbol:       "AAPL",
		Market:       core.MarketUS,
		Quantity:     10,
		AverageCost:  150.0,
		CurrentPrice: 155.0,
		MarketValue:  1550.0,
		UpdatedAt:    time.Now(),
	})
	mockBroker.AddPosition(broker.Position{
		Symbol:       "GOOGL",
		Market:       core.MarketUS,
		Quantity:     5,
		AverageCost:  2800.0,
		CurrentPrice: 2850.0,
		MarketValue:  14250.0,
		UpdatedAt:    time.Now(),
	})
	mockBroker.AddPosition(broker.Position{
		Symbol:       "MSFT",
		Market:       core.MarketUS,
		Quantity:     20,
		AverageCost:  300.0,
		CurrentPrice: 310.0,
		MarketValue:  6200.0,
		UpdatedAt:    time.Now(),
	})

	checker := broker.NewRiskChecker(config, mockBroker)

	// Try to buy a new position
	req := broker.OrderRequest{
		Symbol:   "NVDA",
		Market:   core.MarketUS,
		Side:     broker.OrderSideBuy,
		Type:     broker.OrderTypeLimit,
		Quantity: 10,
		Price:    500.0,
	}

	ctx := context.Background()
	result := checker.Check(ctx, req, 500.0)

	assert.False(t, result.Allowed, "Buy order should be rejected at max positions")
	assert.Contains(t, result.Reason, "max open positions reached")
	assert.Contains(t, result.Reason, "3 >= 3")
}

func TestRiskChecker_Check_MaxPositionsReached_SellAllowed(t *testing.T) {
	// Setup mock broker
	mockBroker := mocks.New()
	err := mockBroker.Connect(context.Background())
	require.NoError(t, err)
	defer mockBroker.Disconnect()

	// Set balance
	mockBroker.SetBalance(&broker.Balance{
		Currency:    "USD",
		Cash:        100000.00,
		BuyingPower: 100000.00,
		TotalValue:  100000.00,
		DailyPL:     0,
		UpdatedAt:   time.Now(),
	})

	// Add positions up to the limit
	config := broker.RiskConfig{
		MaxPositionPct:   10.0,
		MaxDailyLossPct:  5.0,
		MaxOpenPositions: 3, // Low limit for testing
	}

	// Add 3 existing positions
	mockBroker.AddPosition(broker.Position{
		Symbol:       "AAPL",
		Market:       core.MarketUS,
		Quantity:     10,
		AverageCost:  150.0,
		CurrentPrice: 155.0,
		MarketValue:  1550.0,
		UpdatedAt:    time.Now(),
	})
	mockBroker.AddPosition(broker.Position{
		Symbol:       "GOOGL",
		Market:       core.MarketUS,
		Quantity:     5,
		AverageCost:  2800.0,
		CurrentPrice: 2850.0,
		MarketValue:  14250.0,
		UpdatedAt:    time.Now(),
	})
	mockBroker.AddPosition(broker.Position{
		Symbol:       "MSFT",
		Market:       core.MarketUS,
		Quantity:     20,
		AverageCost:  300.0,
		CurrentPrice: 310.0,
		MarketValue:  6200.0,
		UpdatedAt:    time.Now(),
	})

	checker := broker.NewRiskChecker(config, mockBroker)

	// Try to sell an existing position - should be allowed even at max positions
	req := broker.OrderRequest{
		Symbol:   "AAPL",
		Market:   core.MarketUS,
		Side:     broker.OrderSideSell,
		Type:     broker.OrderTypeLimit,
		Quantity: 5,
		Price:    155.0,
	}

	ctx := context.Background()
	result := checker.Check(ctx, req, 155.0)

	assert.True(t, result.Allowed, "Sell order should be allowed even at max positions")
	assert.Empty(t, result.Reason, "Reason should be empty when allowed")
}

func TestRiskChecker_Check_DailyLossLimitReached(t *testing.T) {
	// Setup mock broker
	mockBroker := mocks.New()
	err := mockBroker.Connect(context.Background())
	require.NoError(t, err)
	defer mockBroker.Disconnect()

	// Set balance with daily loss exceeding limit
	// DailyPL = -5000 means 5% loss on $100,000 portfolio
	mockBroker.SetBalance(&broker.Balance{
		Currency:    "USD",
		Cash:        95000.00,
		BuyingPower: 95000.00,
		TotalValue:  100000.00,
		DailyPL:     -5000.0, // -5% daily loss
		UpdatedAt:   time.Now(),
	})

	config := broker.RiskConfig{
		MaxPositionPct:   10.0,
		MaxDailyLossPct:  5.0,
		MaxOpenPositions: 20,
	}

	checker := broker.NewRiskChecker(config, mockBroker)

	// Try to place any order
	req := broker.OrderRequest{
		Symbol:   "AAPL",
		Market:   core.MarketUS,
		Side:     broker.OrderSideBuy,
		Type:     broker.OrderTypeLimit,
		Quantity: 10,
		Price:    100.0,
	}

	ctx := context.Background()
	result := checker.Check(ctx, req, 100.0)

	assert.False(t, result.Allowed, "Order should be rejected when daily loss limit reached")
	assert.Contains(t, result.Reason, "daily loss limit reached")
	assert.Contains(t, result.Reason, "5.00%")
}

func TestRiskChecker_Check_DailyLossExceedsLimit(t *testing.T) {
	// Setup mock broker
	mockBroker := mocks.New()
	err := mockBroker.Connect(context.Background())
	require.NoError(t, err)
	defer mockBroker.Disconnect()

	// Set balance with daily loss exceeding limit (7% loss)
	mockBroker.SetBalance(&broker.Balance{
		Currency:    "USD",
		Cash:        93000.00,
		BuyingPower: 93000.00,
		TotalValue:  100000.00,
		DailyPL:     -7000.0, // -7% daily loss
		UpdatedAt:   time.Now(),
	})

	config := broker.RiskConfig{
		MaxPositionPct:   10.0,
		MaxDailyLossPct:  5.0,
		MaxOpenPositions: 20,
	}

	checker := broker.NewRiskChecker(config, mockBroker)

	// Try to place any order
	req := broker.OrderRequest{
		Symbol:   "AAPL",
		Market:   core.MarketUS,
		Side:     broker.OrderSideBuy,
		Type:     broker.OrderTypeLimit,
		Quantity: 10,
		Price:    100.0,
	}

	ctx := context.Background()
	result := checker.Check(ctx, req, 100.0)

	assert.False(t, result.Allowed, "Order should be rejected when daily loss exceeds limit")
	assert.Contains(t, result.Reason, "daily loss limit reached")
	assert.Contains(t, result.Reason, "7.00%")
}

func TestRiskChecker_Check_PositiveDailyPL_Allowed(t *testing.T) {
	// Setup mock broker
	mockBroker := mocks.New()
	err := mockBroker.Connect(context.Background())
	require.NoError(t, err)
	defer mockBroker.Disconnect()

	// Set balance with positive daily P/L (profit)
	mockBroker.SetBalance(&broker.Balance{
		Currency:    "USD",
		Cash:        105000.00,
		BuyingPower: 105000.00,
		TotalValue:  105000.00,
		DailyPL:     5000.0, // +5% daily profit
		UpdatedAt:   time.Now(),
	})

	config := broker.RiskConfig{
		MaxPositionPct:   10.0,
		MaxDailyLossPct:  5.0,
		MaxOpenPositions: 20,
	}

	checker := broker.NewRiskChecker(config, mockBroker)

	// Create order request within limits
	req := broker.OrderRequest{
		Symbol:   "AAPL",
		Market:   core.MarketUS,
		Side:     broker.OrderSideBuy,
		Type:     broker.OrderTypeLimit,
		Quantity: 50,
		Price:    100.0,
	}

	ctx := context.Background()
	result := checker.Check(ctx, req, 100.0)

	assert.True(t, result.Allowed, "Order should be allowed with positive daily P/L")
	assert.Empty(t, result.Reason, "Reason should be empty when allowed")
}

func TestRiskChecker_Check_ExactlyAtPositionLimit(t *testing.T) {
	// Setup mock broker
	mockBroker := mocks.New()
	err := mockBroker.Connect(context.Background())
	require.NoError(t, err)
	defer mockBroker.Disconnect()

	// Set balance
	mockBroker.SetBalance(&broker.Balance{
		Currency:    "USD",
		Cash:        100000.00,
		BuyingPower: 100000.00,
		TotalValue:  100000.00,
		DailyPL:     0,
		UpdatedAt:   time.Now(),
	})

	config := broker.RiskConfig{
		MaxPositionPct:   10.0,
		MaxDailyLossPct:  5.0,
		MaxOpenPositions: 20,
	}

	checker := broker.NewRiskChecker(config, mockBroker)

	// Create order request exactly at the 10% limit
	req := broker.OrderRequest{
		Symbol:   "AAPL",
		Market:   core.MarketUS,
		Side:     broker.OrderSideBuy,
		Type:     broker.OrderTypeLimit,
		Quantity: 100,
		Price:    100.0,
	}

	ctx := context.Background()
	result := checker.Check(ctx, req, 100.0) // Order value: 100 * 100 = $10,000 (exactly 10%)

	assert.True(t, result.Allowed, "Order should be allowed when exactly at position limit")
	assert.Empty(t, result.Reason, "Reason should be empty when allowed")
}

func TestRiskChecker_Check_SlightlyOverPositionLimit(t *testing.T) {
	// Setup mock broker
	mockBroker := mocks.New()
	err := mockBroker.Connect(context.Background())
	require.NoError(t, err)
	defer mockBroker.Disconnect()

	// Set balance
	mockBroker.SetBalance(&broker.Balance{
		Currency:    "USD",
		Cash:        100000.00,
		BuyingPower: 100000.00,
		TotalValue:  100000.00,
		DailyPL:     0,
		UpdatedAt:   time.Now(),
	})

	config := broker.RiskConfig{
		MaxPositionPct:   10.0,
		MaxDailyLossPct:  5.0,
		MaxOpenPositions: 20,
	}

	checker := broker.NewRiskChecker(config, mockBroker)

	// Create order request slightly over the 10% limit (10.01%)
	req := broker.OrderRequest{
		Symbol:   "AAPL",
		Market:   core.MarketUS,
		Side:     broker.OrderSideBuy,
		Type:     broker.OrderTypeLimit,
		Quantity: 101,
		Price:    99.11,
	}

	ctx := context.Background()
	result := checker.Check(ctx, req, 99.11) // Order value: 101 * 99.11 = $10,010.11 (10.01%)

	assert.False(t, result.Allowed, "Order should be rejected when slightly over position limit")
	assert.Contains(t, result.Reason, "position size too large")
}

func TestRiskChecker_Check_MarketOrder(t *testing.T) {
	// Setup mock broker
	mockBroker := mocks.New()
	err := mockBroker.Connect(context.Background())
	require.NoError(t, err)
	defer mockBroker.Disconnect()

	// Set balance
	mockBroker.SetBalance(&broker.Balance{
		Currency:    "USD",
		Cash:        100000.00,
		BuyingPower: 100000.00,
		TotalValue:  100000.00,
		DailyPL:     0,
		UpdatedAt:   time.Now(),
	})

	config := broker.RiskConfig{
		MaxPositionPct:   10.0,
		MaxDailyLossPct:  5.0,
		MaxOpenPositions: 20,
	}

	checker := broker.NewRiskChecker(config, mockBroker)

	// Create market order - price is passed separately
	req := broker.OrderRequest{
		Symbol:   "AAPL",
		Market:   core.MarketUS,
		Side:     broker.OrderSideBuy,
		Type:     broker.OrderTypeMarket,
		Quantity: 50,
	}

	ctx := context.Background()
	result := checker.Check(ctx, req, 150.0) // Using market price of $150

	assert.True(t, result.Allowed, "Market order should be allowed within limits")
	assert.Empty(t, result.Reason, "Reason should be empty when allowed")
}
