package valuation

import (
	"math"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

// ReconstructPESeries rebuilds the daily PE(TTM) series by step-aligning
// closes with the EPS(TTM) points (yahoo 路径:EPSPoint.Date 为报告期;M2 接入
// EDGAR 后升级为 filing date 生效,见设计文档 §5.1).
func ReconstructPESeries(closes []core.OHLCV, eps []core.EPSPoint) ([]time.Time, []float64, error) {
	pts, err := sortedEPSWithGate(eps)
	if err != nil {
		return nil, nil, err
	}
	dates, pe := alignPE(closes, pts)
	return dates, pe, nil
}

// RollingPercentile computes, for each i, the percentile (0-100) of values[i]
// within the lookback window (dates[i]-years, dates[i]). Windows with fewer
// than minPoints samples yield NaN. dates must be ascending.
func RollingPercentile(dates []time.Time, values []float64, years, minPoints int) []float64 {
	out := make([]float64, len(values))
	lo := 0
	for i := range values {
		cutoff := dates[i].AddDate(-years, 0, 0)
		for lo < i && !dates[lo].After(cutoff) {
			lo++
		}
		window := values[lo:i]
		if len(window) < minPoints {
			out[i] = math.NaN()
			continue
		}
		out[i] = PercentileRank(window, values[i])
	}
	return out
}
