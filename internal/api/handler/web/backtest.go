// internal/api/handler/web/backtest.go
package web

import (
	"net/http"

	"github.com/newthinker/atlas/internal/backtest"
)

type BacktestData struct {
	Title  string
	Result *backtest.Result
}

func (h *Handler) Backtest(w http.ResponseWriter, r *http.Request) {
	data := BacktestData{
		Title:  "Backtest",
		Result: nil,
	}

	h.templates.ExecuteTemplate(w, "layout.html", data)
}
