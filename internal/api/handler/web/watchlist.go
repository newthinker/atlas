// internal/api/handler/web/watchlist.go
package web

import (
	"net/http"
)

type WatchlistItem struct {
	Symbol     string
	Name       string
	Strategies []string
}

type WatchlistData struct {
	Title string
	Items []WatchlistItem
}

func (h *Handler) Watchlist(w http.ResponseWriter, r *http.Request) {
	// TODO: Fetch from config/storage
	data := WatchlistData{
		Title: "Watchlist",
		Items: []WatchlistItem{},
	}

	h.render(w, "watchlist.html", data)
}
