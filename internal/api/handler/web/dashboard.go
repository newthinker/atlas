// internal/api/handler/web/dashboard.go
package web

import (
	"net/http"
)

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
	data := DashboardData{
		Title:          "Dashboard",
		SignalsToday:   0, // TODO: Wire to actual data
		BuySignals:     0,
		SellSignals:    0,
		WatchlistCount: 0,
		RecentSignals:  []SignalView{}, // Empty for now, will be populated when signal store is wired
	}

	h.render(w, "dashboard.html", data)
}
