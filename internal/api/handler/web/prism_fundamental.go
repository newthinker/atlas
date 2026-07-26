// internal/api/handler/web/prism_fundamental.go
package web

import (
	"encoding/json"
	"errors"
	"html/template"
	"math"
	"net/http"

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
	payload, err := json.Marshal(map[string]any{
		"symbol": view.Symbol, "periods": view.Periods, "metrics": metrics,
		"dates": view.Dates, "closes": jsonNums(view.Closes),
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
