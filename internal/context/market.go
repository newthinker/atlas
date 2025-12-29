// internal/context/market.go
package context

import (
	"context"
	"math"
	"time"

	"github.com/newthinker/atlas/internal/collector"
	"github.com/newthinker/atlas/internal/core"
)

// MarketContextService implements MarketContextProvider.
type MarketContextService struct {
	collectors map[core.Market]collector.Collector
}

// NewMarketContextService creates a new market context service.
func NewMarketContextService(collectors map[core.Market]collector.Collector) *MarketContextService {
	return &MarketContextService{
		collectors: collectors,
	}
}

// GetContext returns market context for the given market.
func (s *MarketContextService) GetContext(ctx context.Context, market core.Market) (*MarketContext, error) {
	// Get a representative index for the market
	indexSymbol := getMarketIndex(market)

	col, ok := s.collectors[market]
	if !ok {
		// Return basic context if no collector available
		return &MarketContext{
			Market:     market,
			Regime:     RegimeSideways,
			Volatility: 0.15, // Default moderate volatility
			UpdatedAt:  time.Now(),
		}, nil
	}

	// Fetch historical data for the index (60 days)
	end := time.Now()
	start := end.AddDate(0, 0, -60)
	ohlcv, err := col.FetchHistory(indexSymbol, start, end, "1d")
	if err != nil {
		return &MarketContext{
			Market:     market,
			Regime:     RegimeSideways,
			Volatility: 0.15,
			UpdatedAt:  time.Now(),
		}, nil
	}

	// Calculate regime and volatility
	regime := calculateRegime(ohlcv)
	volatility := calculateVolatility(ohlcv)

	return &MarketContext{
		Market:     market,
		Regime:     regime,
		Volatility: volatility,
		UpdatedAt:  time.Now(),
	}, nil
}

// getMarketIndex returns a representative index symbol for the market.
func getMarketIndex(market core.Market) string {
	switch market {
	case core.MarketCNA:
		return "000001.SH" // Shanghai Composite
	case core.MarketHK:
		return "HSI" // Hang Seng Index
	case core.MarketUS:
		return "SPY" // S&P 500 ETF
	default:
		return ""
	}
}

// calculateRegime determines the market regime based on price action.
func calculateRegime(data []core.OHLCV) MarketRegime {
	if len(data) < 20 {
		return RegimeSideways
	}

	// Compare recent average to earlier average
	recent := data[len(data)-10:]
	earlier := data[len(data)-30 : len(data)-10]

	recentAvg := avgClose(recent)
	earlierAvg := avgClose(earlier)

	change := (recentAvg - earlierAvg) / earlierAvg

	if change > 0.05 {
		return RegimeBull
	} else if change < -0.05 {
		return RegimeBear
	}
	return RegimeSideways
}

// calculateVolatility calculates annualized volatility from daily returns.
func calculateVolatility(data []core.OHLCV) float64 {
	if len(data) < 2 {
		return 0.15
	}

	// Calculate daily returns
	returns := make([]float64, len(data)-1)
	for i := 1; i < len(data); i++ {
		if data[i-1].Close > 0 {
			returns[i-1] = (data[i].Close - data[i-1].Close) / data[i-1].Close
		}
	}

	// Calculate standard deviation of returns
	mean := 0.0
	for _, r := range returns {
		mean += r
	}
	mean /= float64(len(returns))

	variance := 0.0
	for _, r := range returns {
		variance += (r - mean) * (r - mean)
	}
	variance /= float64(len(returns))

	// Annualize (assuming ~252 trading days)
	dailyVol := math.Sqrt(variance)
	annualizedVol := dailyVol * math.Sqrt(252)

	return annualizedVol
}

func avgClose(data []core.OHLCV) float64 {
	if len(data) == 0 {
		return 0
	}
	sum := 0.0
	for _, d := range data {
		sum += d.Close
	}
	return sum / float64(len(data))
}
