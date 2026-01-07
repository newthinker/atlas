// internal/api/handler/api/watchlist.go
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/newthinker/atlas/internal/api/response"
	"github.com/newthinker/atlas/internal/core"
)

// WatchlistApp defines the interface needed from app.App.
type WatchlistApp interface {
	GetWatchlist() []string
	AddToWatchlist(symbol string)
	AddToWatchlistWithDetails(symbol, name, market, assetType string, strategies []string)
	RemoveFromWatchlist(symbol string) bool
}

// WatchlistHandler handles watchlist API requests.
type WatchlistHandler struct {
	app WatchlistApp
}

// NewWatchlistHandler creates a new watchlist handler.
func NewWatchlistHandler(app WatchlistApp) *WatchlistHandler {
	return &WatchlistHandler{app: app}
}

// AddRequest is the request body for adding a symbol.
type AddRequest struct {
	Symbol string `json:"symbol"`
	Market string `json:"market,omitempty"`
}

// List returns all symbols in the watchlist.
func (h *WatchlistHandler) List(w http.ResponseWriter, r *http.Request) {
	symbols := h.app.GetWatchlist()
	response.JSON(w, http.StatusOK, map[string]any{
		"symbols": symbols,
		"count":   len(symbols),
	})
}

// Add adds a symbol to the watchlist.
func (h *WatchlistHandler) Add(w http.ResponseWriter, r *http.Request) {
	var req AddRequest

	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "application/x-www-form-urlencoded") || strings.HasPrefix(contentType, "multipart/form-data") {
		// Parse form data (from HTMX forms)
		if err := r.ParseForm(); err != nil {
			response.Error(w, http.StatusBadRequest,
				core.WrapError(core.ErrConfigInvalid, err))
			return
		}
		req.Symbol = r.FormValue("symbol")
		req.Market = r.FormValue("market")
	} else {
		// Parse JSON
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			response.Error(w, http.StatusBadRequest,
				core.WrapError(core.ErrConfigInvalid, err))
			return
		}
	}

	if req.Symbol == "" {
		response.Error(w, http.StatusBadRequest,
			core.WrapError(core.ErrConfigMissing, nil))
		return
	}

	// Get name, market, type, and strategies from form
	name := r.FormValue("name")
	if name == "" {
		name = req.Symbol
	}
	market := r.FormValue("market")
	assetType := r.FormValue("type")
	strategies := r.Form["strategies"]

	h.app.AddToWatchlistWithDetails(req.Symbol, name, market, assetType, strategies)

	// Check if this is an HTMX request
	if r.Header.Get("HX-Request") == "true" {
		// Return HTML table row for HTMX
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("HX-Trigger", "closeModal")
		w.WriteHeader(http.StatusCreated)

		// Build strategies HTML
		var strategiesHTML string
		for _, s := range strategies {
			strategiesHTML += fmt.Sprintf(`<span class="inline-block bg-gray-100 rounded px-2 py-1 text-xs mr-1">%s</span>`, s)
		}

		// Build name with market and type inline display
		nameDisplay := name
		if market != "" || assetType != "" {
			nameDisplay = fmt.Sprintf("%s (%s · %s)", name, market, assetType)
		}

		fmt.Fprintf(w, `<tr data-market="%s" data-type="%s">
			<td class="px-6 py-4 whitespace-nowrap text-sm font-medium text-gray-900">%s</td>
			<td class="px-6 py-4 whitespace-nowrap text-sm text-gray-500">%s</td>
			<td class="px-6 py-4 text-sm text-gray-500">%s</td>
			<td class="px-6 py-4 whitespace-nowrap text-sm">
				<button hx-delete="/api/v1/watchlist/%s" hx-target="closest tr" hx-swap="outerHTML"
						class="text-red-600 hover:text-red-800">Remove</button>
			</td>
		</tr>`, market, assetType, req.Symbol, nameDisplay, strategiesHTML, req.Symbol)
		return
	}

	response.JSON(w, http.StatusCreated, map[string]any{
		"symbol": req.Symbol,
		"added":  true,
	})
}

// Remove removes a symbol from the watchlist.
func (h *WatchlistHandler) Remove(w http.ResponseWriter, r *http.Request, symbol string) {
	removed := h.app.RemoveFromWatchlist(symbol)
	if !removed {
		response.Error(w, http.StatusNotFound, core.ErrSymbolNotFound)
		return
	}

	// Check if this is an HTMX request
	if r.Header.Get("HX-Request") == "true" {
		// Return empty content for HTMX (row will be removed)
		w.WriteHeader(http.StatusOK)
		return
	}

	response.JSON(w, http.StatusOK, map[string]any{
		"symbol":  symbol,
		"removed": true,
	})
}
