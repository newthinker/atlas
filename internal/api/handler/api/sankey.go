package api

import (
	"errors"
	"net/http"

	"github.com/newthinker/atlas/internal/api/response"
	"github.com/newthinker/atlas/internal/prism/sankey"
	prismstore "github.com/newthinker/atlas/internal/storage/prism"
)

// SankeyService is the subset of *sankey.Service the handlers need.
type SankeyService interface {
	Analyze(symbol, from, to, granularity, lang string) (*sankey.Analysis, error)
	Fundamental(symbol string) (*sankey.FundamentalView, error)
}

// SankeyHandler serves the earnings bridge and fundamentals JSON APIs.
type SankeyHandler struct {
	svc SankeyService
}

func NewSankeyHandler(svc SankeyService) *SankeyHandler {
	return &SankeyHandler{svc: svc}
}

// Sankey serves GET /api/prism/sankey.
func (h *SankeyHandler) Sankey(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	symbol := q.Get("symbol")
	if symbol == "" {
		response.JSON(w, http.StatusBadRequest, map[string]any{"error": "symbol required"})
		return
	}
	granularity := q.Get("granularity")
	switch granularity {
	case "", "q", "fy":
	default:
		// Falling back to quarterly would show the user a different granularity
		// than the one they asked for, with nothing to say so.
		response.JSON(w, http.StatusBadRequest,
			map[string]any{"error": "granularity must be q or fy"})
		return
	}

	analysis, err := h.svc.Analyze(symbol, q.Get("from"), q.Get("to"), granularity, q.Get("lang"))
	if err != nil {
		switch {
		case errors.Is(err, sankey.ErrNoTemplate):
			response.JSON(w, http.StatusNotFound, map[string]any{"error": "no sankey template"})
		case errors.Is(err, prismstore.ErrNotFound):
			response.JSON(w, http.StatusNotFound, map[string]any{"error": "unknown symbol"})
		default:
			response.Error(w, http.StatusInternalServerError, err)
		}
		return
	}
	response.JSON(w, http.StatusOK, analysisJSON(analysis))
}

// Fundamental serves GET /api/prism/fundamental.
func (h *SankeyHandler) Fundamental(w http.ResponseWriter, r *http.Request) {
	symbol := r.URL.Query().Get("symbol")
	if symbol == "" {
		response.JSON(w, http.StatusBadRequest, map[string]any{"error": "symbol required"})
		return
	}
	view, err := h.svc.Fundamental(symbol)
	if err != nil {
		if errors.Is(err, prismstore.ErrNotFound) {
			response.JSON(w, http.StatusNotFound, map[string]any{"error": "unknown symbol"})
			return
		}
		response.Error(w, http.StatusInternalServerError, err)
		return
	}

	metrics := make(map[string]any, len(view.Metrics))
	for k, vs := range view.Metrics {
		metrics[k] = jfSlice(vs)
	}
	response.JSON(w, http.StatusOK, map[string]any{
		"symbol": view.Symbol, "periods": view.Periods, "metrics": metrics,
		"dates": view.Dates, "closes": jfSlice(view.Closes),
	})
}

// analysisJSON renders the analysis with every float passed through jf: the
// engine promises no ±Inf, and this is the second line of that defence (AD-14).
func analysisJSON(a *sankey.Analysis) map[string]any {
	periods := make([]any, 0, len(a.Periods))
	for _, p := range a.Periods {
		periods = append(periods, map[string]any{
			"period":  p.Period,
			"graph":   graphJSON(p.Graph),
			"metrics": metricsJSON(p.Metrics),
		})
	}

	matrix := make([]any, 0, len(a.Matrix))
	for _, row := range a.Matrix {
		matrix = append(matrix, map[string]any{
			"key": row.Key, "label": row.Label, "values": jfSlice(row.Values),
			"change": jf(row.Change), "change_kind": row.ChangeKind,
		})
	}

	// Conflicts carries the periods the engine refused to aggregate; dropping it
	// would leave the page unable to explain why a fiscal year is missing (AD-26).
	conflicts := make([]any, 0, len(a.Conflicts))
	for _, c := range a.Conflicts {
		conflicts = append(conflicts, map[string]any{"period": c.Period, "detail": c.Detail})
	}

	return map[string]any{
		"symbol": a.Symbol, "granularity": a.Granularity, "truncated": a.Truncated,
		"periods": periods, "matrix": matrix, "conflicts": conflicts,
		"template": map[string]any{"version": a.Template.Version, "lang": a.Template.Lang},
	}
}

// graphJSON keeps Notes attached to the graph: it is the only record of a flow
// drawn at zero width because it was negative (AD-22). Without it the diagram
// simply comes up short with nothing to explain the gap.
func graphJSON(g sankey.Graph) map[string]any {
	nodes := make([]any, 0, len(g.Nodes))
	for _, n := range g.Nodes {
		nodes = append(nodes, map[string]any{"name": n.Name, "value": jf(n.Value)})
	}
	links := make([]any, 0, len(g.Links))
	for _, l := range g.Links {
		links = append(links, map[string]any{
			"source": l.Source, "target": l.Target, "value": jf(l.Value),
		})
	}
	return map[string]any{"nodes": nodes, "links": links, "notes": g.Notes}
}

func metricsJSON(m sankey.PeriodMetrics) map[string]any {
	segments := make(map[string]any, len(m.Segments))
	for k, v := range m.Segments {
		segments[k] = jf(v)
	}
	return map[string]any{
		"period": m.Period, "period_end": m.PeriodEnd, "segments": segments,
		"revenue": jf(m.Revenue), "gross_profit": jf(m.GrossProfit),
		"operating_income": jf(m.OperatingIncome), "net_income": jf(m.NetIncome),
		"rnd": jf(m.RnD), "sganda": jf(m.SGnA), "income_tax": jf(m.IncomeTax),
		"gross_margin": jf(m.GrossMargin), "op_margin": jf(m.OpMargin),
		"net_margin": jf(m.NetMargin),
	}
}
