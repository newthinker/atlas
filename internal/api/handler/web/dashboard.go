// internal/api/handler/web/dashboard.go
package web

import (
	"net/http"
	"time"

	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/storage/signal"
)

const recentSignalsLimit = 10

// DashboardData holds data for the dashboard template
type DashboardData struct {
	Title          string
	SignalsToday   int
	BuySignals     int
	SellSignals    int
	WatchlistCount int
	RecentSignals  []SignalView
}

// Dashboard renders the dashboard page
func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	watchlistCount := 0
	if h.watchlistProvider != nil {
		watchlistCount = len(h.watchlistProvider.GetWatchlist())
	}

	data := DashboardData{
		Title:          "Dashboard",
		WatchlistCount: watchlistCount,
		RecentSignals:  []SignalView{},
	}

	if h.signalStore != nil {
		ctx := r.Context()

		todayStart := time.Now().Truncate(24 * time.Hour)
		data.SignalsToday, _ = h.signalStore.Count(ctx, signal.ListFilter{From: todayStart})
		data.BuySignals, _ = h.signalStore.Count(ctx, signal.ListFilter{From: todayStart, Action: core.ActionBuy})
		data.SellSignals, _ = h.signalStore.Count(ctx, signal.ListFilter{From: todayStart, Action: core.ActionSell})

		recent, err := h.signalStore.List(ctx, signal.ListFilter{})
		if err == nil {
			data.RecentSignals = recentSignalViews(recent, recentSignalsLimit)
		}
	}

	h.render(w, "dashboard.html", data)
}
