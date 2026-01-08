// internal/api/handler/web/settings.go
package web

import (
	"fmt"
	"net/http"
)

// NotifierView represents a notifier for display
type NotifierView struct {
	Name    string
	Type    string
	Details string // Additional details like chat_id, email recipients, webhook url
	Enabled bool
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
		MinConfidence:    0.6,
		CooldownDuration: "4h",
		EnabledActions:   []string{"Buy", "Sell", "StrongBuy", "StrongSell"},
		Notifiers:        []NotifierView{},
	}

	// Get config from provider if available
	if h.configProvider != nil {
		routerCfg := h.configProvider.GetRouterConfig()
		data.MinConfidence = routerCfg.MinConfidence
		data.CooldownDuration = formatDuration(routerCfg.CooldownHours)

		// Get notifiers
		notifiers := h.configProvider.GetNotifiers()
		for name, info := range notifiers {
			data.Notifiers = append(data.Notifiers, NotifierView{
				Name:    name,
				Type:    info.Type,
				Details: info.Details,
				Enabled: info.Enabled,
			})
		}
	}

	h.render(w, "settings.html", data)
}

// formatDuration converts hours to a human-readable duration string
func formatDuration(hours int) string {
	if hours == 0 {
		return "0h"
	}
	if hours >= 24 && hours%24 == 0 {
		return fmt.Sprintf("%dd", hours/24)
	}
	return fmt.Sprintf("%dh", hours)
}
