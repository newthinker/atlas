// internal/api/handler/web/handler.go
package web

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"math"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/newthinker/atlas/internal/core"
	prism "github.com/newthinker/atlas/internal/storage/prism"
	"github.com/newthinker/atlas/internal/storage/signal"
)

//go:embed templates/*
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

// WatchlistItemData represents a watchlist item with metadata
type WatchlistItemData struct {
	Symbol     string
	Name       string
	Market     string
	Type       string
	Strategies []string
}

// WatchlistProvider provides watchlist data
type WatchlistProvider interface {
	GetWatchlist() []string
	GetWatchlistItems() []WatchlistItemData
}

// StrategyProvider provides strategy information
type StrategyProvider interface {
	GetStrategyNames() []string
}

// ConfigProvider provides configuration data for the settings page
type ConfigProvider interface {
	GetNotifiers() map[string]NotifierInfo
	GetRouterConfig() RouterInfo
}

// NotifierInfo holds notifier display info
type NotifierInfo struct {
	Enabled bool
	Type    string // telegram, email, webhook
	Details string // Additional details for display
}

// RouterInfo holds router configuration
type RouterInfo struct {
	MinConfidence float64
	CooldownHours int
}

// Handler provides web UI handlers with template rendering
type Handler struct {
	// pageTemplates holds separate template instances for each page
	// Each instance contains layout.html + the specific page template
	pageTemplates     map[string]*template.Template
	watchlistProvider WatchlistProvider
	strategyProvider  StrategyProvider
	configProvider    ConfigProvider
	signalStore       signal.Store
	prismProvider     PrismProvider
	prismLow          float64
	prismHigh         float64
}

// NewHandler creates a new web handler with templates loaded from the given directory.
// If templatesDir is empty, it falls back to embedded templates.
func NewHandler(templatesDir string) (*Handler, error) {
	pageTemplates := make(map[string]*template.Template)

	// List of page templates (excluding layout.html)
	pages := []string{"dashboard.html", "signals.html", "watchlist.html", "backtest.html", "settings.html", "symbol_detail.html", "prism_board.html", "prism_detail.html"}

	for _, page := range pages {
		var tmpl *template.Template
		var err error

		if templatesDir != "" {
			// Parse layout first, then the page template
			layoutPath := filepath.Join(templatesDir, "layout.html")
			pagePath := filepath.Join(templatesDir, page)
			tmpl, err = template.ParseFiles(layoutPath, pagePath)
			if err != nil {
				return nil, fmt.Errorf("parsing template %s: %w", page, err)
			}
		} else {
			// Use embedded templates
			subFS, err := fs.Sub(templateFS, "templates")
			if err != nil {
				return nil, fmt.Errorf("accessing embedded templates: %w", err)
			}

			// Parse layout first, then the page template
			tmpl, err = template.ParseFS(subFS, "layout.html", page)
			if err != nil {
				return nil, fmt.Errorf("parsing embedded template %s: %w", page, err)
			}
		}

		pageTemplates[page] = tmpl
	}

	return &Handler{pageTemplates: pageTemplates}, nil
}

// NewHandlerWithFS creates a new web handler using a custom filesystem.
// This is useful for testing or custom template sources.
func NewHandlerWithFS(fsys fs.FS) (*Handler, error) {
	pageTemplates := make(map[string]*template.Template)
	pages := []string{"dashboard.html", "signals.html", "watchlist.html", "backtest.html", "settings.html", "symbol_detail.html", "prism_board.html", "prism_detail.html"}

	for _, page := range pages {
		tmpl, err := template.ParseFS(fsys, "layout.html", page)
		if err != nil {
			return nil, fmt.Errorf("parsing template %s from fs: %w", page, err)
		}
		pageTemplates[page] = tmpl
	}

	return &Handler{pageTemplates: pageTemplates}, nil
}

// render executes the specified page template with the given data
func (h *Handler) render(w http.ResponseWriter, page string, data any) {
	tmpl, ok := h.pageTemplates[page]
	if !ok {
		http.Error(w, "template not found: "+page, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "layout.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// SetWatchlistProvider sets the watchlist data provider
func (h *Handler) SetWatchlistProvider(p WatchlistProvider) {
	h.watchlistProvider = p
}

// SetStrategyProvider sets the strategy data provider
func (h *Handler) SetStrategyProvider(p StrategyProvider) {
	h.strategyProvider = p
}

// SetConfigProvider sets the configuration data provider
func (h *Handler) SetConfigProvider(p ConfigProvider) {
	h.configProvider = p
}

// SetSignalStore sets the signal persistence store used by the dashboard and
// signals pages.
func (h *Handler) SetSignalStore(s signal.Store) {
	h.signalStore = s
}

// PrismProvider supplies Prism board/series data to web pages.
type PrismProvider interface {
	Board() ([]prism.BoardRow, error)
	Series(symbol, from string) (*prism.SeriesData, error)
}

// SetPrismProvider wires the Prism data provider and estimate thresholds.
func (h *Handler) SetPrismProvider(p PrismProvider, low, high float64) {
	h.prismProvider = p
	h.prismLow, h.prismHigh = low, high
}

// prismCard is one rendered card on the board page.
type prismCard struct {
	Symbol, Name, Group, Source, AsOf, Status, StatusZH string
	PETTM, PB, Pctl                                     string // 已格式化,"—" 表缺失
	PctlVal                                             float64
}

// fmtVal formats a metric to prec decimals; NaN renders as "—".
func fmtVal(v float64, prec int) string {
	if math.IsNaN(v) {
		return "—"
	}
	return strconv.FormatFloat(v, 'f', prec, 64)
}

// PrismBoard renders the server-side card matrix at /prism/board with ?group= filter.
func (h *Handler) PrismBoard(w http.ResponseWriter, r *http.Request) {
	if h.prismProvider == nil {
		http.Error(w, "prism not enabled", http.StatusNotFound)
		return
	}
	rows, err := h.prismProvider.Board()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	group := r.URL.Query().Get("group")
	groups := map[string]bool{}
	var cards []prismCard
	for _, b := range rows {
		groups[b.Group] = true
		if group != "" && b.Group != group {
			continue
		}
		p := b.Pctl10Y
		if math.IsNaN(p) {
			p = b.Pctl5Y
		}
		status, statusZH := "neutral", "中性"
		switch {
		case math.IsNaN(p):
			status, statusZH = "na", "—"
		case p < h.prismLow:
			status, statusZH = "low", "低估"
		case p > h.prismHigh:
			status, statusZH = "high", "高估"
		}
		cards = append(cards, prismCard{
			Symbol: b.Symbol, Name: b.Name, Group: b.Group, Source: b.Source,
			AsOf: b.AsOf, Status: status, StatusZH: statusZH,
			PETTM: fmtVal(b.PETTM, 1), PB: fmtVal(b.PB, 2), Pctl: fmtVal(p, 0), PctlVal: p,
		})
	}
	var groupList []string
	for g := range groups {
		groupList = append(groupList, g)
	}
	sort.Strings(groupList)
	h.render(w, "prism_board.html", map[string]any{
		"Title": "Prism 估值面板", "Cards": cards,
		"Groups": groupList, "ActiveGroup": group,
	})
}

// PrismDetail renders the ECharts detail page at /prism/detail/{symbol}.
func (h *Handler) PrismDetail(w http.ResponseWriter, r *http.Request, symbol string) {
	if h.prismProvider == nil {
		http.Error(w, "prism not enabled", http.StatusNotFound)
		return
	}
	sd, err := h.prismProvider.Series(symbol, "")
	if err != nil {
		http.Error(w, "unknown symbol", http.StatusNotFound)
		return
	}
	h.render(w, "prism_detail.html", map[string]any{
		"Title": sd.Name + " 估值时序", "Symbol": sd.Symbol,
		"Name": sd.Name, "Source": sd.Source,
	})
}

// Static serves vendored assets (echarts.min.js) from the embedded FS.
func (h *Handler) Static(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/static/")
	if name != "echarts.min.js" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeFileFS(w, r, staticFS, "static/"+name)
}

// toSignalView converts a domain signal into its display representation.
func toSignalView(s core.Signal) SignalView {
	return SignalView{
		Time:       s.GeneratedAt.Format("2006-01-02 15:04"),
		Symbol:     s.Symbol,
		Action:     string(s.Action),
		Strategy:   s.Strategy,
		Confidence: int(s.Confidence * 100),
		Reason:     s.Reason,
	}
}

// recentSignalViews converts signals to views, newest first, capped at limit.
// The store returns signals oldest-first, so we iterate in reverse. A limit <= 0
// returns all signals.
func recentSignalViews(signals []core.Signal, limit int) []SignalView {
	views := make([]SignalView, 0, len(signals))
	for i := len(signals) - 1; i >= 0; i-- {
		if limit > 0 && len(views) >= limit {
			break
		}
		views = append(views, toSignalView(signals[i]))
	}
	return views
}

// TemplateFS returns the embedded template filesystem for external use.
func TemplateFS() fs.FS {
	subFS, err := fs.Sub(templateFS, "templates")
	if err != nil {
		// This should never happen with valid embed directive
		return templateFS
	}
	return subFS
}
