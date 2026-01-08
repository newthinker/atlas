// internal/api/handler/web/symbol_detail.go
package web

import (
	"net/http"
)

// SymbolDetailData holds data for the symbol detail page template
type SymbolDetailData struct {
	Title      string
	Symbol     string
	Name       string
	Market     string
	Type       string
	Strategies []string
}

// SymbolDetail renders the symbol detail page
func (h *Handler) SymbolDetail(w http.ResponseWriter, r *http.Request, symbol string) {
	// Get symbol metadata from watchlist if available
	var name, market, assetType string
	if h.watchlistProvider != nil {
		for _, item := range h.watchlistProvider.GetWatchlistItems() {
			if item.Symbol == symbol {
				name = item.Name
				market = item.Market
				assetType = item.Type
				break
			}
		}
	}

	// Default name to symbol if not found
	if name == "" {
		name = symbol
	}

	// Get available strategies
	var strategies []string
	if h.strategyProvider != nil {
		strategies = h.strategyProvider.GetStrategyNames()
	}
	if len(strategies) == 0 {
		strategies = []string{"ma_crossover", "pe_band", "dividend_yield"}
	}

	data := SymbolDetailData{
		Title:      symbol + " - Detail",
		Symbol:     symbol,
		Name:       name,
		Market:     market,
		Type:       assetType,
		Strategies: strategies,
	}

	h.render(w, "symbol_detail.html", data)
}
