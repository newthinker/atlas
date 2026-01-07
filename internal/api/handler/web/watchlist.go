// internal/api/handler/web/watchlist.go
package web

import (
	"net/http"
)

// Market and Type options
var (
	Markets = []string{"A股", "H股", "美股", "数字货币"}
	Types   = []string{"股票", "基金", "债券", "ETF", "期权", "期货", "加密货币"}
)

// WatchlistItem is local type for template rendering
type WatchlistItem struct {
	Symbol     string
	Name       string
	Market     string
	Type       string
	Strategies []string
}

// WatchlistData holds data for the watchlist page template
type WatchlistData struct {
	Title      string
	Items      []WatchlistItem
	Strategies []string
	Markets    []string
	Types      []string
}

func (h *Handler) Watchlist(w http.ResponseWriter, r *http.Request) {
	var items []WatchlistItem

	// Fetch watchlist items from provider if available
	if h.watchlistProvider != nil {
		providerItems := h.watchlistProvider.GetWatchlistItems()
		for _, item := range providerItems {
			items = append(items, WatchlistItem{
				Symbol:     item.Symbol,
				Name:       item.Name,
				Market:     item.Market,
				Type:       item.Type,
				Strategies: item.Strategies,
			})
		}
	}

	// Get available strategies
	var strategies []string
	if h.strategyProvider != nil {
		strategies = h.strategyProvider.GetStrategyNames()
	}
	if len(strategies) == 0 {
		// Default strategies if provider not available
		strategies = []string{"MA Crossover", "PE Band", "Dividend Yield"}
	}

	data := WatchlistData{
		Title:      "Watchlist",
		Items:      items,
		Strategies: strategies,
		Markets:    Markets,
		Types:      Types,
	}

	h.render(w, "watchlist.html", data)
}
