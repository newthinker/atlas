// internal/api/handler/web/settings.go
package web

import (
	"net/http"
)

// NotifierView represents a notifier for display
type NotifierView struct {
	Name string
	Type string
}

// SettingsData holds data for the settings template
type SettingsData struct {
	Title            string
	MinConfidence    float64
	CooldownDuration string
	EnabledActions   []string
	Notifiers        []NotifierView
}

// Settings renders the settings page
func (h *Handler) Settings(w http.ResponseWriter, r *http.Request) {
	data := SettingsData{
		Title:            "Settings",
		MinConfidence:    0.5,              // TODO: Wire to actual config
		CooldownDuration: "1h",             // TODO: Wire to actual config
		EnabledActions:   []string{"Buy", "Sell", "StrongBuy", "StrongSell"},
		Notifiers:        []NotifierView{}, // TODO: Wire to actual notifier registry
	}

	h.render(w, "settings.html", data)
}
