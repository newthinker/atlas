// internal/api/handler/web/dashboard.go
package web

import (
	"html/template"
	"net/http"
	"path/filepath"
)

type DashboardData struct {
	Title          string
	SignalsToday   int
	BuySignals     int
	SellSignals    int
	WatchlistCount int
}

type Handler struct {
	templates *template.Template
}

func NewHandler(templatesDir string) (*Handler, error) {
	tmpl, err := template.ParseGlob(filepath.Join(templatesDir, "*.html"))
	if err != nil {
		return nil, err
	}
	return &Handler{templates: tmpl}, nil
}

func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	data := DashboardData{
		Title:          "Dashboard",
		SignalsToday:   0, // TODO: Wire to actual data
		BuySignals:     0,
		SellSignals:    0,
		WatchlistCount: 0,
	}

	h.templates.ExecuteTemplate(w, "layout.html", data)
}
