// Package broker provides types and interfaces for broker integrations.
package broker

import (
	"context"
	"fmt"
)

// RiskConfig defines risk management parameters.
type RiskConfig struct {
	// MaxPositionPct is the maximum percentage of portfolio value allowed in a single position.
	MaxPositionPct float64
	// MaxDailyLossPct is the maximum daily loss percentage before trading is halted.
	MaxDailyLossPct float64
	// MaxOpenPositions is the maximum number of concurrent positions allowed.
	MaxOpenPositions int
}

// DefaultRiskConfig returns a RiskConfig with sensible default values.
func DefaultRiskConfig() RiskConfig {
	return RiskConfig{
		MaxPositionPct:   10.0,
		MaxDailyLossPct:  5.0,
		MaxOpenPositions: 20,
	}
}

// RiskCheckResult represents the outcome of a risk check.
type RiskCheckResult struct {
	// Allowed indicates whether the order is permitted.
	Allowed bool
	// Reason provides explanation when order is rejected.
	Reason string
}

// RiskChecker validates orders against risk management rules.
type RiskChecker struct {
	config RiskConfig
	broker Broker
}

// NewRiskChecker creates a new RiskChecker with the given configuration and broker.
func NewRiskChecker(config RiskConfig, broker Broker) *RiskChecker {
	return &RiskChecker{
		config: config,
		broker: broker,
	}
}

// Check validates an order request against risk management rules.
// It returns a RiskCheckResult indicating whether the order is allowed.
func (r *RiskChecker) Check(ctx context.Context, req OrderRequest, price float64) RiskCheckResult {
	// Get current balance
	balance, err := r.broker.GetBalance(ctx)
	if err != nil {
		return RiskCheckResult{
			Allowed: false,
			Reason:  fmt.Sprintf("failed to get balance: %v", err),
		}
	}

	// Get current positions
	positions, err := r.broker.GetPositions(ctx)
	if err != nil {
		return RiskCheckResult{
			Allowed: false,
			Reason:  fmt.Sprintf("failed to get positions: %v", err),
		}
	}

	// Check daily loss limit
	if balance.TotalValue > 0 {
		// DailyPL is negative when losing money, so we negate it for the percentage calculation
		dailyLossPct := (-balance.DailyPL / balance.TotalValue) * 100
		if dailyLossPct >= r.config.MaxDailyLossPct {
			return RiskCheckResult{
				Allowed: false,
				Reason:  fmt.Sprintf("daily loss limit reached: %.2f%% >= %.2f%%", dailyLossPct, r.config.MaxDailyLossPct),
			}
		}
	}

	// Check max open positions (only for BUY orders)
	if req.Side == OrderSideBuy {
		if len(positions) >= r.config.MaxOpenPositions {
			return RiskCheckResult{
				Allowed: false,
				Reason:  fmt.Sprintf("max open positions reached: %d >= %d", len(positions), r.config.MaxOpenPositions),
			}
		}
	}

	// Check position size limit
	if balance.TotalValue > 0 {
		orderValue := float64(req.Quantity) * price
		positionPct := (orderValue / balance.TotalValue) * 100
		if positionPct > r.config.MaxPositionPct {
			return RiskCheckResult{
				Allowed: false,
				Reason:  fmt.Sprintf("position size too large: %.2f%% > %.2f%%", positionPct, r.config.MaxPositionPct),
			}
		}
	}

	return RiskCheckResult{
		Allowed: true,
		Reason:  "",
	}
}
