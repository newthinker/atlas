package crisis

import (
	"encoding/json"
	"time"
)

// InQuarterEndWindow reports whether date falls in the quarter-end
// suppression window: the last 3 trading days of a quarter or the first 2 of
// the next (design §3.2 rule 1 — repo-market spikes there are routine).
// Weekday ≈ trading day; weekends and unparseable dates are never in-window.
func InQuarterEndWindow(date string) bool {
	d, err := time.Parse(dateLayout, date)
	if err != nil || isWeekend(d) {
		return false
	}
	cur := lastTradingDayOnOrBefore(quarterEnd(d))
	for i := 0; i < 3; i++ {
		if cur.Equal(d) {
			return true
		}
		cur = PrevTradingDay(cur)
	}
	cur = firstTradingDayOnOrAfter(quarterStart(d))
	for i := 0; i < 2; i++ {
		if cur.Equal(d) {
			return true
		}
		cur = nextTradingDay(cur)
	}
	return false
}

func quarterStart(t time.Time) time.Time {
	m := ((int(t.Month())-1)/3)*3 + 1
	return time.Date(t.Year(), time.Month(m), 1, 0, 0, 0, 0, time.UTC)
}

func quarterEnd(t time.Time) time.Time { return quarterStart(t).AddDate(0, 3, -1) }

func lastTradingDayOnOrBefore(t time.Time) time.Time {
	for isWeekend(t) {
		t = t.AddDate(0, 0, -1)
	}
	return t
}

func firstTradingDayOnOrAfter(t time.Time) time.Time {
	for isWeekend(t) {
		t = t.AddDate(0, 0, 1)
	}
	return t
}

func nextTradingDay(t time.Time) time.Time {
	d := t.AddDate(0, 0, 1)
	for isWeekend(d) {
		d = d.AddDate(0, 0, 1)
	}
	return d
}

// staleFor implements design §3.2 rule 2 (≈48h beyond expected publication,
// widened via config for weekends/holidays). It also covers the MOVE
// "3 consecutive fetch failures" degradation: failed fetches leave the latest
// observation aging past the window. NFCI uses the weekly allowance.
func staleFor(cfg *Config, indicator, evalDate, latestObsDate string) bool {
	maxLag := cfg.Freshness.DailyMaxLagDays
	if indicator == IndNFCI {
		maxLag = cfg.Freshness.WeeklyMaxLagDays
	}
	return daysBetween(latestObsDate, evalDate) > maxLag
}

// indDetail is the JSON persisted in indicator evaluation rows; Raw feeds the
// hysteresis on later days.
type indDetail struct {
	Raw             Status  `json:"raw"`
	WindowActualObs int     `json:"window_actual_obs"`
	PersistDays     int     `json:"persist_days,omitempty"`
	Wow             float64 `json:"wow,omitempty"`
	WowOK           bool    `json:"wow_ok,omitempty"`
}

func rawFromDetail(e Evaluation) Status {
	var d indDetail
	if err := json.Unmarshal([]byte(e.Detail), &d); err == nil && d.Raw != "" {
		return d.Raw
	}
	return e.Status
}

// ApplyHysteresis implements design §3.2 rule 3 (asymmetric debounce):
// upgrades take effect immediately; a downgrade needs `days` consecutive
// observation days — today plus days-1 prior raw statuses — at or below the
// target level, otherwise yesterday's effective status is kept. prev is
// newest-first. Insufficient or non-color history blocks the downgrade.
func ApplyHysteresis(raw Status, prev []Evaluation, days int) Status {
	if len(prev) == 0 || !isColor(raw) {
		return raw
	}
	prevEff := prev[0].Status
	if !isColor(prevEff) || severity(raw) >= severity(prevEff) {
		return raw
	}
	need := days - 1
	if len(prev) < need {
		return prevEff
	}
	for i := 0; i < need; i++ {
		r := rawFromDetail(prev[i])
		if !isColor(r) || severity(r) > severity(raw) {
			return prevEff
		}
	}
	return raw
}
