// internal/api/handler/web/prism_sankey.go
package web

import (
	"errors"
	"net/http"

	"github.com/newthinker/atlas/internal/prism/sankey"
	prism "github.com/newthinker/atlas/internal/storage/prism"
)

// SankeyAnalyzer is the subset of *sankey.Service the page needs. A narrow
// interface keeps the page testable without a store behind it.
type SankeyAnalyzer interface {
	Analyze(symbol, from, to, granularity, lang string) (*sankey.Analysis, error)
}

// The three 404 reasons are worded apart on purpose. Collapsing them into one
// "no data" line sends the user hunting through the data pipeline when the
// actual problem is a missing template or a server that never loaded any
// (TASK-011 discovery, contract_for_downstream item 5).
const (
	sankeyNotEnabledMsg    = "Prism 财报桥未启用：服务端未加载任何桑基模板（模板目录缺失或加载失败），请检查服务端配置与日志"
	sankeyNoTemplateMsg    = "该标的未配置财报桥模板：请在 configs/prism/templates 下添加该标的的模板 YAML 后重启服务"
	sankeyUnknownSymbolMsg = "标的不在库中：请先同步该标的的财报数据后再打开本页"
)

// SetPrismSankey wires the earnings bridge service. Left unset, the page
// answers sankeyNotEnabledMsg rather than pretending the symbol has no data.
func (h *Handler) SetPrismSankey(a SankeyAnalyzer) {
	h.sankeySvc = a
}

// sankeyNote is one engine note together with the period it belongs to.
type sankeyNote struct{ Period, Text string }

// PrismSankey renders the earnings bridge page at /prism/sankey/{symbol}.
//
// The analysis is fetched server-side even though the page's JS re-fetches it
// on every control change: it is what lets the page tell the three 404s apart
// before rendering anything, and it puts the engine's Notes on the page as
// real footnotes (AD-22) instead of leaving them to a script that may never
// run.
func (h *Handler) PrismSankey(w http.ResponseWriter, r *http.Request, symbol string) {
	if h.sankeySvc == nil {
		http.Error(w, sankeyNotEnabledMsg, http.StatusNotFound)
		return
	}

	q := r.URL.Query()
	granularity := q.Get("granularity")
	if granularity != "q" && granularity != "fy" {
		// An unknown value is treated as unspecified: the page then shows the
		// granularity the engine actually picked (a.Granularity), so the user
		// never sees a granularity other than the one the controls report.
		granularity = ""
	}
	lang := "zh"
	if q.Get("lang") == "en" {
		lang = "en"
	}
	from, to := q.Get("from"), q.Get("to")

	a, err := h.sankeySvc.Analyze(symbol, from, to, granularity, lang)
	switch {
	case errors.Is(err, sankey.ErrNoTemplate):
		http.Error(w, sankeyNoTemplateMsg, http.StatusNotFound)
		return
	case errors.Is(err, prism.ErrNotFound):
		http.Error(w, sankeyUnknownSymbolMsg, http.StatusNotFound)
		return
	case err != nil:
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	periods := make([]string, 0, len(a.Periods))
	var notes []sankeyNote
	for _, p := range a.Periods {
		periods = append(periods, p.Period)
		for _, n := range p.Graph.Notes {
			notes = append(notes, sankeyNote{Period: p.Period, Text: n})
		}
	}

	h.render(w, "prism_sankey.html", map[string]any{
		"Title": symbol + " 财报桥", "Symbol": symbol,
		"Granularity": a.Granularity, "Lang": lang,
		"From": from, "To": to,
		"Periods": periods, "Notes": notes, "Conflicts": a.Conflicts,
		"TemplateVersion": a.Template.Version, "Truncated": a.Truncated,
	})
}
