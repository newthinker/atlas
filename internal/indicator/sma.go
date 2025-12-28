package indicator

// SMA calculates Simple Moving Average
// Returns slice of length: len(prices) - period + 1
func SMA(prices []float64, period int) []float64 {
	if len(prices) < period {
		return []float64{}
	}

	result := make([]float64, 0, len(prices)-period+1)

	// Calculate first SMA
	var sum float64
	for i := 0; i < period; i++ {
		sum += prices[i]
	}
	result = append(result, sum/float64(period))

	// Rolling calculation
	for i := period; i < len(prices); i++ {
		sum = sum - prices[i-period] + prices[i]
		result = append(result, sum/float64(period))
	}

	return result
}

// EMA calculates Exponential Moving Average
func EMA(prices []float64, period int) []float64 {
	if len(prices) < period {
		return []float64{}
	}

	result := make([]float64, 0, len(prices)-period+1)
	multiplier := 2.0 / float64(period+1)

	// Start with SMA as first EMA value
	var sum float64
	for i := 0; i < period; i++ {
		sum += prices[i]
	}
	ema := sum / float64(period)
	result = append(result, ema)

	// Calculate EMA for remaining prices
	for i := period; i < len(prices); i++ {
		ema = (prices[i]-ema)*multiplier + ema
		result = append(result, ema)
	}

	return result
}
