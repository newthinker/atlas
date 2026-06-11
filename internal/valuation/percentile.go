// Package valuation provides pure functions for historical-percentile
// computations used by the price_percentile and pe_percentile strategies.
package valuation

// PercentileRank returns the percentage (0-100) of series values strictly
// less than current. Returns -1 for an empty series.
func PercentileRank(series []float64, current float64) float64 {
	if len(series) == 0 {
		return -1
	}
	less := 0
	for _, v := range series {
		if v < current {
			less++
		}
	}
	return float64(less) / float64(len(series)) * 100
}
