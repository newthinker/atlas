package backtest

import (
	"math"
)

// CalculateStats computes performance statistics from trades
func CalculateStats(trades []Trade) Stats {
	if len(trades) == 0 {
		return Stats{}
	}

	var winning, losing int
	var totalReturn float64
	var returns []float64

	for _, t := range trades {
		if !t.IsClosed() {
			continue
		}
		returns = append(returns, t.Return)
		totalReturn += t.Return
		if t.IsWin() {
			winning++
		} else {
			losing++
		}
	}

	closedTrades := winning + losing
	var winRate float64
	if closedTrades > 0 {
		winRate = float64(winning) / float64(closedTrades) * 100
	}

	return Stats{
		TotalTrades:   len(trades),
		WinningTrades: winning,
		LosingTrades:  losing,
		WinRate:       winRate,
		TotalReturn:   totalReturn * 100, // Convert to percentage
		MaxDrawdown:   calculateMaxDrawdown(returns) * 100,
		SharpeRatio:   calculateSharpeRatio(returns),
	}
}

// calculateMaxDrawdown finds the largest peak-to-trough decline
func calculateMaxDrawdown(returns []float64) float64 {
	if len(returns) == 0 {
		return 0
	}

	var maxDD float64
	var peak float64
	cumulative := 1.0

	for _, r := range returns {
		cumulative *= (1 + r)
		if cumulative > peak {
			peak = cumulative
		}
		if peak > 0 {
			dd := (peak - cumulative) / peak
			if dd > maxDD {
				maxDD = dd
			}
		}
	}

	return maxDD
}

// calculateSharpeRatio computes risk-adjusted return
// Assumes risk-free rate of 0 for simplicity
func calculateSharpeRatio(returns []float64) float64 {
	if len(returns) < 2 {
		return 0
	}

	// Calculate mean return
	var sum float64
	for _, r := range returns {
		sum += r
	}
	mean := sum / float64(len(returns))

	// Calculate standard deviation
	var variance float64
	for _, r := range returns {
		variance += (r - mean) * (r - mean)
	}
	stdDev := math.Sqrt(variance / float64(len(returns)-1))

	if stdDev == 0 {
		return 0
	}

	// Annualize (assuming ~252 trading days)
	annualizedReturn := mean * 252
	annualizedStdDev := stdDev * math.Sqrt(252)

	return annualizedReturn / annualizedStdDev
}
