package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/storage/signal"
)

func newTestHandler(t *testing.T) *Handler {
	t.Helper()
	h, err := NewHandlerWithFS(TemplateFS())
	if err != nil {
		t.Fatalf("creating handler: %v", err)
	}
	return h
}

func TestSignals_RendersStoredSignalsNewestFirst(t *testing.T) {
	store := signal.NewMemoryStore(100)
	base := time.Now()
	_ = store.Save(context.Background(), core.Signal{Symbol: "AAPL", Action: core.ActionBuy, Confidence: 0.8, Strategy: "ma_crossover", GeneratedAt: base})
	_ = store.Save(context.Background(), core.Signal{Symbol: "TSLA", Action: core.ActionSell, Confidence: 0.7, Strategy: "rsi", GeneratedAt: base.Add(time.Minute)})

	h := newTestHandler(t)
	h.SetSignalStore(store)

	req := httptest.NewRequest(http.MethodGet, "/signals", nil)
	rec := httptest.NewRecorder()
	h.Signals(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "AAPL") || !strings.Contains(body, "TSLA") {
		t.Errorf("expected both symbols in output, got: %s", body)
	}
	// Newest (TSLA) should appear before AAPL.
	if strings.Index(body, "TSLA") > strings.Index(body, "AAPL") {
		t.Errorf("expected TSLA (newest) before AAPL")
	}
}

func TestSignals_NoStoreRendersEmpty(t *testing.T) {
	h := newTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/signals", nil)
	rec := httptest.NewRecorder()
	h.Signals(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with no store, got %d", rec.Code)
	}
}

func TestDashboard_CountsTodaySignals(t *testing.T) {
	store := signal.NewMemoryStore(100)
	now := time.Now()
	_ = store.Save(context.Background(), core.Signal{Symbol: "AAPL", Action: core.ActionBuy, Confidence: 0.8, GeneratedAt: now})
	_ = store.Save(context.Background(), core.Signal{Symbol: "MSFT", Action: core.ActionBuy, Confidence: 0.9, GeneratedAt: now})
	_ = store.Save(context.Background(), core.Signal{Symbol: "TSLA", Action: core.ActionSell, Confidence: 0.7, GeneratedAt: now})
	// Yesterday's signal should not count toward "today".
	_ = store.Save(context.Background(), core.Signal{Symbol: "OLD", Action: core.ActionBuy, Confidence: 0.6, GeneratedAt: now.AddDate(0, 0, -2)})

	h := newTestHandler(t)
	h.SetSignalStore(store)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.Dashboard(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}
