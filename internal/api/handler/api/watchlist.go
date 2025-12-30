// internal/api/handler/api/watchlist.go
package api

import (
	"encoding/json"
	"net/http"

	"github.com/newthinker/atlas/internal/api/response"
	"github.com/newthinker/atlas/internal/core"
)

// WatchlistApp defines the interface needed from app.App.
type WatchlistApp interface {
	GetWatchlist() []string
	AddToWatchlist(symbol string)
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest,
			core.WrapError(core.ErrConfigInvalid, err))
		return
	}

	if req.Symbol == "" {
		response.Error(w, http.StatusBadRequest,
			core.WrapError(core.ErrConfigMissing, nil))
		return
	}

	h.app.AddToWatchlist(req.Symbol)

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

	response.JSON(w, http.StatusOK, map[string]any{
		"symbol":  symbol,
		"removed": true,
	})
}
