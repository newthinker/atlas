package api

import (
	"errors"
	"math"
	"net/http"
	"time"

	"github.com/newthinker/atlas/internal/api/response"
	prismstore "github.com/newthinker/atlas/internal/storage/prism"
)

// PrismStore defines the interface needed from *prismstore.Store.
type PrismStore interface {
	Board() ([]prismstore.BoardRow, error)
	Series(symbol, from string) (*prismstore.SeriesData, error)
}

// PrismHandler serves the Prism valuation board JSON API.
type PrismHandler struct {
	store     PrismStore
	low, high float64
}

// NewPrismHandler creates a Prism API handler with low/high percentile thresholds.
func NewPrismHandler(store PrismStore, low, high float64) *PrismHandler {
	return &PrismHandler{store: store, low: low, high: high}
}

// jf marshals NaN as null (encoding/json rejects NaN in float64).
func jf(v float64) any {
	if math.IsNaN(v) {
		return nil
	}
	return v
}

// status picks pctl_10y when available, else pctl_5y, and maps to a label.
func (h *PrismHandler) status(p5, p10 float64) string {
	p := p10
	if math.IsNaN(p) {
		p = p5
	}
	switch {
	case math.IsNaN(p):
		return "na"
	case p < h.low:
		return "low"
	case p > h.high:
		return "high"
	default:
		return "neutral"
	}
}

// Board serves GET /api/prism/board.
func (h *PrismHandler) Board(w http.ResponseWriter, r *http.Request) {
	rows, err := h.store.Board()
	if err != nil {
		response.Error(w, http.StatusInternalServerError, err)
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, b := range rows {
		items = append(items, map[string]any{
			"symbol": b.Symbol, "name": b.Name, "group": b.Group,
			"market": b.Market, "source": b.Source, "as_of": b.AsOf,
			"pe_ttm": jf(b.PETTM), "pb": jf(b.PB),
			"pctl_5y": jf(b.Pctl5Y), "pctl_10y": jf(b.Pctl10Y),
			"status": h.status(b.Pctl5Y, b.Pctl10Y),
		})
	}
	response.JSON(w, http.StatusOK, map[string]any{
		"items": items, "low_pct": h.low, "high_pct": h.high,
	})
}

// windowFrom maps ?window= to a from-date string ("" = max).
func windowFrom(window string, now time.Time) string {
	years := map[string]int{"1y": 1, "3y": 3, "5y": 5}[window]
	if years == 0 {
		return ""
	}
	return now.AddDate(-years, 0, 0).Format("2006-01-02")
}

// Series serves GET /api/prism/series?symbol=&window=.
func (h *PrismHandler) Series(w http.ResponseWriter, r *http.Request) {
	symbol := r.URL.Query().Get("symbol")
	if symbol == "" {
		response.JSON(w, http.StatusBadRequest, map[string]any{"error": "symbol required"})
		return
	}
	sd, err := h.store.Series(symbol, windowFrom(r.URL.Query().Get("window"), time.Now()))
	if err != nil {
		if errors.Is(err, prismstore.ErrNotFound) {
			response.JSON(w, http.StatusNotFound, map[string]any{"error": "unknown symbol"})
			return
		}
		response.Error(w, http.StatusInternalServerError, err)
		return
	}
	njf := func(vs []float64) []any {
		out := make([]any, len(vs))
		for i, v := range vs {
			out[i] = jf(v)
		}
		return out
	}
	response.JSON(w, http.StatusOK, map[string]any{
		"symbol": sd.Symbol, "name": sd.Name, "source": sd.Source,
		"dates": sd.Dates, "pe_ttm": njf(sd.PETTM),
		"pctl_5y": njf(sd.Pctl5Y), "pctl_10y": njf(sd.Pctl10Y),
	})
}
