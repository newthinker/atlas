// internal/api/handler/web/handler.go
package web

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"path/filepath"
)

//go:embed templates/*
var templateFS embed.FS

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

// Handler provides web UI handlers with template rendering
type Handler struct {
	// pageTemplates holds separate template instances for each page
	// Each instance contains layout.html + the specific page template
	pageTemplates     map[string]*template.Template
	watchlistProvider WatchlistProvider
	strategyProvider  StrategyProvider
}

// NewHandler creates a new web handler with templates loaded from the given directory.
// If templatesDir is empty, it falls back to embedded templates.
func NewHandler(templatesDir string) (*Handler, error) {
	pageTemplates := make(map[string]*template.Template)

	// List of page templates (excluding layout.html)
	pages := []string{"dashboard.html", "signals.html", "watchlist.html", "backtest.html", "settings.html", "symbol_detail.html"}

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
	pages := []string{"dashboard.html", "signals.html", "watchlist.html", "backtest.html", "settings.html", "symbol_detail.html"}

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

// TemplateFS returns the embedded template filesystem for external use.
func TemplateFS() fs.FS {
	subFS, err := fs.Sub(templateFS, "templates")
	if err != nil {
		// This should never happen with valid embed directive
		return templateFS
	}
	return subFS
}
