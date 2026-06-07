// internal/api/handler/web/signals.go
package web

import (
	"net/http"

	"github.com/newthinker/atlas/internal/storage/signal"
)

// signalsPageLimit caps how many signals the signals page renders.
const signalsPageLimit = 200

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
	signals := []SignalView{}
	if h.signalStore != nil {
		stored, err := h.signalStore.List(r.Context(), signal.ListFilter{})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		signals = recentSignalViews(stored, signalsPageLimit)
	}

	data := SignalsData{
		Title:   "Signals",
		Signals: signals,
	}

	h.render(w, "signals.html", data)
}
