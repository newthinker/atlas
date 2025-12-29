// internal/api/handler/web/signals.go
package web

import (
	"net/http"
)

type SignalView struct {
	Time       string
	Symbol     string
	Action     string
	Strategy   string
	Confidence int
	Reason     string
}

type SignalsData struct {
	Title   string
	Signals []SignalView
}

func (h *Handler) Signals(w http.ResponseWriter, r *http.Request) {
	// TODO: Fetch actual signals from storage
	data := SignalsData{
		Title:   "Signals",
		Signals: []SignalView{}, // Empty for now
	}

	h.templates.ExecuteTemplate(w, "layout.html", data)
}
