// internal/api/handler/api/signals_test.go
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/api/response"
	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/storage/signal"
)

func TestSignalsHandler_List(t *testing.T) {
	store := signal.NewMemoryStore(100)
	store.Save(context.Background(), core.Signal{
		Symbol:      "AAPL",
		Action:      core.ActionBuy,
		Confidence:  0.85,
		Strategy:    "ma_crossover",
		GeneratedAt: time.Now(),
	})

	handler := NewSignalsHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/signals", nil)
	w := httptest.NewRecorder()

	handler.List(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp response.SuccessResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	data := resp.Data.(map[string]any)
	signals := data["signals"].([]any)
	if len(signals) != 1 {
		t.Errorf("expected 1 signal, got %d", len(signals))
	}
}

func TestSignalsHandler_ListWithFilters(t *testing.T) {
	store := signal.NewMemoryStore(100)
	store.Save(context.Background(), core.Signal{
		Symbol:      "AAPL",
		Strategy:    "ma_crossover",
		GeneratedAt: time.Now(),
	})
	store.Save(context.Background(), core.Signal{
		Symbol:      "GOOG",
		Strategy:    "pe_band",
		GeneratedAt: time.Now(),
	})

	handler := NewSignalsHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/signals?symbol=AAPL", nil)
	w := httptest.NewRecorder()

	handler.List(w, req)

	var resp response.SuccessResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	data := resp.Data.(map[string]any)
	signals := data["signals"].([]any)
	if len(signals) != 1 {
		t.Errorf("expected 1 signal, got %d", len(signals))
	}
}

func TestSignalsHandler_GetByID(t *testing.T) {
	store := signal.NewMemoryStore(100)
	store.Save(context.Background(), core.Signal{
		Symbol:      "AAPL",
		GeneratedAt: time.Now(),
	})

	signals, _ := store.List(context.Background(), signal.ListFilter{})
	signalID := signals[0].ID

	handler := NewSignalsHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/signals/"+signalID, nil)
	w := httptest.NewRecorder()

	handler.GetByID(w, req, signalID)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestSignalsHandler_GetByID_NotFound(t *testing.T) {
	store := signal.NewMemoryStore(100)
	handler := NewSignalsHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/signals/nonexistent", nil)
	w := httptest.NewRecorder()

	handler.GetByID(w, req, "nonexistent")

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}
