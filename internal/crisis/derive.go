package crisis

import "github.com/newthinker/atlas/internal/valuation"

// SpreadBp converts a SOFR/EFFR pair (both in percent) to a spread in bp
// (design §2.2: sofr_effr_spread_bp).
func SpreadBp(sofr, effr float64) float64 { return (sofr - effr) * 100 }

// WowPct returns close_t/close_{t-5 obs} − 1 over an ascending window
// (design §2.2: usdjpy_wow_pct; also VIX weekly spike). ok=false when fewer
// than 6 observations are available or the base is zero.
func WowPct(window []Observation) (float64, bool) {
	if len(window) < 6 {
		return 0, false
	}
	cur := window[len(window)-1].Value
	base := window[len(window)-6].Value
	if base == 0 {
		return 0, false
	}
	return cur/base - 1, true
}

// MomChange returns window[last] − window[last−n] in the series' own unit
// (design §2.2: hy_oas_mom_bp with n=21). ok=false when n<=0 or fewer than
// n+1 observations are available.
func MomChange(window []Observation, n int) (float64, bool) {
	if n <= 0 || len(window) < n+1 {
		return 0, false
	}
	return window[len(window)-1].Value - window[len(window)-1-n].Value, true
}

// Percentile returns current's rank within window as 0–1 plus the actual
// observation count (design §2.2: short windows are used as-is and the
// actual size annotated). Empty window returns (-1, 0).
func Percentile(window []Observation, current float64) (float64, int) {
	if len(window) == 0 {
		return -1, 0
	}
	vals := make([]float64, len(window))
	for i, o := range window {
		vals[i] = o.Value
	}
	return valuation.PercentileRank(vals, current) / 100, len(vals)
}
