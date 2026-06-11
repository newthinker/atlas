package valuation

import (
	"errors"
	"sort"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

var (
	// ErrInsufficientEPS: 有效（EPS>0）季度点 < MinEPSPoints，数据缺失，调用方可走理杏仁兜底。
	ErrInsufficientEPS = errors.New("valuation: insufficient positive EPS points")
	// ErrNonPositiveEPS: 当前 EPS(TTM) ≤ 0，真实亏损，PE 分位无意义，调用方直接跳过（不兜底）。
	ErrNonPositiveEPS = errors.New("valuation: current EPS is non-positive")
)

// MinEPSPoints is the minimum number of positive quarterly EPS points required
// to reconstruct a meaningful PE series.
const MinEPSPoints = 8

// ReconstructPEPercentile rebuilds the historical PE series by aligning each
// daily close with the latest EPS(TTM) point at or before it (step function),
// drops days whose aligned EPS <= 0, and returns the percentile of the current
// PE within that series.
func ReconstructPEPercentile(closes []core.OHLCV, eps []core.EPSPoint) (float64, error) {
	// 1. Sort a copy of the EPS points ascending by date.
	pts := make([]core.EPSPoint, len(eps))
	copy(pts, eps)
	sort.Slice(pts, func(i, j int) bool { return pts[i].Date.Before(pts[j].Date) })

	// 2. Need at least MinEPSPoints positive quarterly points to be meaningful.
	positive := 0
	for _, p := range pts {
		if p.EPS > 0 {
			positive++
		}
	}
	if positive < MinEPSPoints {
		return -1, ErrInsufficientEPS
	}

	// 3. Current EPS(TTM) is the latest point; non-positive means a real loss,
	//    so the PE percentile is meaningless and the caller must skip (no fallback).
	currentEPS := pts[len(pts)-1].EPS
	if currentEPS <= 0 {
		return -1, ErrNonPositiveEPS
	}

	// 4. Step-align each close with the latest EPS at or before its time; drop
	//    days whose aligned EPS <= 0 (loss quarters).
	peSeries := make([]float64, 0, len(closes))
	for _, bar := range closes {
		e, ok := latestEPSAtOrBefore(pts, bar.Time)
		if !ok || e <= 0 {
			continue
		}
		peSeries = append(peSeries, bar.Close/e)
	}
	// load-bearing: an empty PE series is a data-availability failure, not a
	// success — never let PercentileRank's -1 ride out with a nil error.
	if len(peSeries) == 0 || len(closes) == 0 {
		return -1, ErrInsufficientEPS
	}

	currentPE := closes[len(closes)-1].Close / currentEPS
	return PercentileRank(peSeries, currentPE), nil
}

// latestEPSAtOrBefore returns the EPS of the latest point whose date is at or
// before t. pts must be sorted ascending by date. ok is false when every point
// is strictly after t.
func latestEPSAtOrBefore(pts []core.EPSPoint, t time.Time) (eps float64, ok bool) {
	i := sort.Search(len(pts), func(i int) bool { return pts[i].Date.After(t) })
	if i == 0 {
		return 0, false
	}
	return pts[i-1].EPS, true
}
