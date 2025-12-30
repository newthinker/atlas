// internal/api/handler/api/analysis_test.go
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/newthinker/atlas/internal/api/response"
	"github.com/newthinker/atlas/internal/app"
	"github.com/newthinker/atlas/internal/config"
	"go.uber.org/zap"
)

func TestAnalysisHandler_Trigger(t *testing.T) {
	a := app.New(config.Defaults(), zap.NewNop())
	a.SetWatchlist([]string{"AAPL", "GOOG"})

	handler := NewAnalysisHandler(a)

	req := httptest.NewRequest("POST", "/api/v1/analysis/run", nil)
	w := httptest.NewRecorder()

	handler.Trigger(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp response.SuccessResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	data := resp.Data.(map[string]any)
	if data["triggered"] != true {
		t.Error("expected triggered to be true")
	}
	if data["symbols_count"].(float64) != 2 {
		t.Errorf("expected 2 symbols, got %v", data["symbols_count"])
	}
}

func TestAnalysisHandler_Trigger_EmptyWatchlist(t *testing.T) {
	a := app.New(config.Defaults(), zap.NewNop())
	// Don't set any watchlist

	handler := NewAnalysisHandler(a)

	req := httptest.NewRequest("POST", "/api/v1/analysis/run", nil)
	w := httptest.NewRecorder()

	handler.Trigger(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp response.SuccessResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	data := resp.Data.(map[string]any)
	if data["symbols_count"].(float64) != 0 {
		t.Errorf("expected 0 symbols, got %v", data["symbols_count"])
	}
}
