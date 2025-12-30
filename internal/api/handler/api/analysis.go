// internal/api/handler/api/analysis.go
package api

import (
	"context"
	"net/http"

	"github.com/newthinker/atlas/internal/api/response"
)

// AnalysisApp defines the interface needed from app.App.
type AnalysisApp interface {
	GetWatchlist() []string
	RunOnce(ctx context.Context)
}

// AnalysisHandler handles analysis trigger API requests.
type AnalysisHandler struct {
	app AnalysisApp
}

// NewAnalysisHandler creates a new analysis handler.
func NewAnalysisHandler(app AnalysisApp) *AnalysisHandler {
	return &AnalysisHandler{app: app}
}

// Trigger runs an analysis cycle.
func (h *AnalysisHandler) Trigger(w http.ResponseWriter, r *http.Request) {
	watchlist := h.app.GetWatchlist()

	// Run analysis in background
	go h.app.RunOnce(context.Background())

	response.JSON(w, http.StatusOK, map[string]any{
		"triggered":     true,
		"symbols_count": len(watchlist),
	})
}
