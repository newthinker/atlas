// internal/api/handler/api/watchlist_test.go
package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/newthinker/atlas/internal/api/response"
	"github.com/newthinker/atlas/internal/app"
	"github.com/newthinker/atlas/internal/config"
	"go.uber.org/zap"
)

func TestWatchlistHandler_List(t *testing.T) {
	a := app.New(config.Defaults(), zap.NewNop())
	a.SetWatchlist([]string{"AAPL", "GOOG"})

	handler := NewWatchlistHandler(a)

	req := httptest.NewRequest("GET", "/api/v1/watchlist", nil)
	w := httptest.NewRecorder()

	handler.List(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp response.SuccessResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	data := resp.Data.(map[string]any)
	symbols := data["symbols"].([]any)
	if len(symbols) != 2 {
		t.Errorf("expected 2 symbols, got %d", len(symbols))
	}
}

func TestWatchlistHandler_Add(t *testing.T) {
	a := app.New(config.Defaults(), zap.NewNop())
	handler := NewWatchlistHandler(a)

	body := bytes.NewBufferString(`{"symbol": "AAPL"}`)
	req := httptest.NewRequest("POST", "/api/v1/watchlist", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.Add(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}

	watchlist := a.GetWatchlist()
	if len(watchlist) != 1 || watchlist[0] != "AAPL" {
		t.Errorf("expected AAPL in watchlist, got %v", watchlist)
	}
}

func TestWatchlistHandler_Add_InvalidJSON(t *testing.T) {
	a := app.New(config.Defaults(), zap.NewNop())
	handler := NewWatchlistHandler(a)

	body := bytes.NewBufferString(`{invalid json}`)
	req := httptest.NewRequest("POST", "/api/v1/watchlist", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.Add(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestWatchlistHandler_Add_EmptySymbol(t *testing.T) {
	a := app.New(config.Defaults(), zap.NewNop())
	handler := NewWatchlistHandler(a)

	body := bytes.NewBufferString(`{"symbol": ""}`)
	req := httptest.NewRequest("POST", "/api/v1/watchlist", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.Add(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestWatchlistHandler_Remove(t *testing.T) {
	a := app.New(config.Defaults(), zap.NewNop())
	a.SetWatchlist([]string{"AAPL", "GOOG"})
	handler := NewWatchlistHandler(a)

	req := httptest.NewRequest("DELETE", "/api/v1/watchlist/AAPL", nil)
	w := httptest.NewRecorder()

	handler.Remove(w, req, "AAPL")

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	watchlist := a.GetWatchlist()
	if len(watchlist) != 1 || watchlist[0] != "GOOG" {
		t.Errorf("expected only GOOG in watchlist, got %v", watchlist)
	}
}

func TestWatchlistHandler_Remove_NotFound(t *testing.T) {
	a := app.New(config.Defaults(), zap.NewNop())
	handler := NewWatchlistHandler(a)

	req := httptest.NewRequest("DELETE", "/api/v1/watchlist/AAPL", nil)
	w := httptest.NewRecorder()

	handler.Remove(w, req, "AAPL")

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}
