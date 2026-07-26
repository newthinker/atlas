// internal/api/handler/web/prism_fundamental.go
package web

import (
	"encoding/json"
	"errors"
	"html/template"
	"math"
	"net/http"
	"slices"
	"time"

	"github.com/newthinker/atlas/internal/prism/sankey"
	prism "github.com/newthinker/atlas/internal/storage/prism"
)

// FundamentalProvider is the subset of *sankey.Service this page needs. A
// narrow interface keeps the page testable without a store behind it.
type FundamentalProvider interface {
	Fundamental(symbol string) (*sankey.FundamentalView, error)
}

// The two 404 reasons are worded apart on purpose: "the feature is off" is an
// operations problem and "this symbol has no filings" is a data problem, and a
// single "no data" line sends the reader after the wrong one.
const (
	fundNotEnabledMsg = "Prism 未启用：服务端未装配财报数据服务，请检查服务端配置与日志"
	fundNoDataMsg     = "该标的暂无财报数据：请先同步该标的的财报后再打开本页"
)

// SetPrismFundamental wires the fundamentals service. Left unset, the page
// answers fundNotEnabledMsg rather than pretending the symbol has no filings.
func (h *Handler) SetPrismFundamental(p FundamentalProvider) {
	h.fundamentalSvc = p
}

// jsonNum maps NaN and ±Inf to null.
//
// The engine uses NaN for "not computable", and roe_ttm is NaN for a symbol's
// first three quarters by design — a TTM figure must not be faked from a
// partial sum. Rendering those as 0 would draw a line that is indistinguishable
// from a genuine zero return (AD-14, and json.Marshal rejects NaN outright).
func jsonNum(v float64) any {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil
	}
	return v
}

func jsonNums(vs []float64) []any {
	out := make([]any, len(vs))
	for i, v := range vs {
		out[i] = jsonNum(v)
	}
	return out
}

// closesAtPeriodEnds resamples a daily close series onto report period ends:
// out[i] is the close on periodEnds[i], or on the last trading day before it.
//
// This is what lets the chart draw price and fundamentals against a single x
// axis. The page previously gave price its own category axis, and a category
// axis spreads its points evenly by index: 71 quarters over 17.5 years and 2513
// trading days over 10 years then line up only at the right edge, so the reader
// saw 2009Q1 revenue next to a 2016 price with nothing on the page to say so.
//
// Only a quote from the period end itself or the week before it counts. Every
// real market closure fits in a week (a long holiday weekend, the four sessions
// after 9/11), so a wider gap is not a closed market but a price series that
// does not cover this period, and the honest answer is NaN — a break in the
// line. The two ways to avoid that break are both worse: a later quote would
// explain a report with a price that did not exist yet, and carrying an older
// close forward would draw a flat stretch that reads as a quiet market when it
// is really a stalled sync.
func closesAtPeriodEnds(periodEnds, dates []string, closes []float64) []float64 {
	const maxStale = 7 * 24 * time.Hour

	out := make([]float64, len(periodEnds))
	// The two series arrive through the FundamentalProvider interface; a length
	// mismatch is a bad implementation, not a reason to panic the page.
	n := min(len(dates), len(closes))
	for i, end := range periodEnds {
		out[i] = math.NaN()
		endDay, err := time.Parse(time.DateOnly, end)
		if err != nil {
			continue
		}
		// Dates are ISO-8601 and ascending, so lexical order is chronological.
		j, quoted := slices.BinarySearch(dates[:n], end)
		if !quoted {
			j-- // market closed that day: fall back to the previous session
		}
		if j < 0 {
			continue // no quote on or before this period end at all
		}
		quoteDay, err := time.Parse(time.DateOnly, dates[j])
		if err != nil || endDay.Sub(quoteDay) > maxStale {
			continue
		}
		out[i] = closes[j]
	}
	return out
}

// PrismFundamental renders the fundamentals trend page at
// /prism/fundamental/{symbol}.
//
// The whole series is fetched server-side and inlined into the page. That is
// what lets the page tell the two 404s apart before rendering anything, and it
// is why switching granularity or metric needs no further request: every
// derived figure (annual aggregation, growth rates, margins) is computed in the
// browser from this one payload.
func (h *Handler) PrismFundamental(w http.ResponseWriter, r *http.Request, symbol string) {
	if h.fundamentalSvc == nil {
		http.Error(w, fundNotEnabledMsg, http.StatusNotFound)
		return
	}

	view, err := h.fundamentalSvc.Fundamental(symbol)
	switch {
	case errors.Is(err, prism.ErrNotFound):
		http.Error(w, fundNoDataMsg, http.StatusNotFound)
		return
	case err != nil:
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	metrics := make(map[string][]any, len(view.Metrics))
	for k, vs := range view.Metrics {
		metrics[k] = jsonNums(vs)
	}
	// Only the resampled price reaches the page: the daily series is what forced
	// the second x axis, and inlining 2500 unplotted points would be dead weight.
	// /api/prism/fundamental still serves the full daily series.
	payload, err := json.Marshal(map[string]any{
		"symbol": view.Symbol, "periods": view.Periods, "metrics": metrics,
		"period_closes": jsonNums(closesAtPeriodEnds(view.PeriodEnds, view.Dates, view.Closes)),
	})
	if err != nil {
		// Unreachable once every float has been through jsonNum, which is the
		// only thing json.Marshal rejects here.
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	h.render(w, "prism_fundamental.html", map[string]any{
		"Title": symbol + " 财务趋势", "Symbol": symbol,
		"Periods": view.Periods,
		// json.Marshal escapes <, > and & by default, so the payload is safe to
		// emit unescaped — and it must be, or the JS would receive a string.
		"PayloadJSON": template.JS(payload),
	})
}
