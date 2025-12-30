// internal/api/handler/api/backtest_test.go
package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/api/job"
	"github.com/newthinker/atlas/internal/api/response"
	"github.com/newthinker/atlas/internal/backtest"
	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/strategy"
)

// MockOHLCVProvider for testing
type MockOHLCVProvider struct{}

func (m *MockOHLCVProvider) FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	return []core.OHLCV{
		{Symbol: symbol, Close: 100, Time: start},
		{Symbol: symbol, Close: 105, Time: start.Add(24 * time.Hour)},
		{Symbol: symbol, Close: 110, Time: end},
	}, nil
}

// MockStrategy for testing
type MockStrategy struct{}

func (m *MockStrategy) Name() string        { return "mock" }
func (m *MockStrategy) Description() string { return "mock strategy" }
func (m *MockStrategy) RequiredData() strategy.DataRequirements {
	return strategy.DataRequirements{PriceHistory: 2}
}
func (m *MockStrategy) Init(cfg strategy.Config) error { return nil }
func (m *MockStrategy) Analyze(ctx strategy.AnalysisContext) ([]core.Signal, error) {
	return []core.Signal{{Symbol: ctx.Symbol, Action: core.ActionBuy, Confidence: 0.8}}, nil
}

func TestBacktestHandler_Create(t *testing.T) {
	jobStore := job.NewStore(100, time.Hour)
	backtester := backtest.New(&MockOHLCVProvider{})
	strategies := strategy.NewEngine()
	strategies.Register(&MockStrategy{})

	handler := NewBacktestHandler(jobStore, backtester, strategies)

	body := bytes.NewBufferString(`{
		"symbol": "AAPL",
		"strategy": "mock",
		"start": "2023-01-01",
		"end": "2024-01-01"
	}`)
	req := httptest.NewRequest("POST", "/api/v1/backtest", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.Create(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", w.Code)
	}

	var resp response.SuccessResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	data := resp.Data.(map[string]any)
	if data["job_id"] == nil {
		t.Error("expected job_id in response")
	}
	if data["status"] != "pending" {
		t.Errorf("expected pending status, got %s", data["status"])
	}
}

func TestBacktestHandler_Create_MissingFields(t *testing.T) {
	jobStore := job.NewStore(100, time.Hour)
	backtester := backtest.New(&MockOHLCVProvider{})
	strategies := strategy.NewEngine()

	handler := NewBacktestHandler(jobStore, backtester, strategies)

	body := bytes.NewBufferString(`{"symbol": "AAPL"}`)
	req := httptest.NewRequest("POST", "/api/v1/backtest", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.Create(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestBacktestHandler_Create_InvalidDates(t *testing.T) {
	jobStore := job.NewStore(100, time.Hour)
	backtester := backtest.New(&MockOHLCVProvider{})
	strategies := strategy.NewEngine()
	strategies.Register(&MockStrategy{})

	handler := NewBacktestHandler(jobStore, backtester, strategies)

	body := bytes.NewBufferString(`{
		"symbol": "AAPL",
		"strategy": "mock",
		"start": "invalid-date",
		"end": "2024-01-01"
	}`)
	req := httptest.NewRequest("POST", "/api/v1/backtest", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.Create(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestBacktestHandler_Create_StrategyNotFound(t *testing.T) {
	jobStore := job.NewStore(100, time.Hour)
	backtester := backtest.New(&MockOHLCVProvider{})
	strategies := strategy.NewEngine()
	// Not registering any strategy

	handler := NewBacktestHandler(jobStore, backtester, strategies)

	body := bytes.NewBufferString(`{
		"symbol": "AAPL",
		"strategy": "nonexistent",
		"start": "2023-01-01",
		"end": "2024-01-01"
	}`)
	req := httptest.NewRequest("POST", "/api/v1/backtest", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.Create(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestBacktestHandler_GetStatus(t *testing.T) {
	jobStore := job.NewStore(100, time.Hour)
	backtester := backtest.New(&MockOHLCVProvider{})
	strategies := strategy.NewEngine()

	handler := NewBacktestHandler(jobStore, backtester, strategies)

	// Create a job directly
	j := jobStore.Create("backtest")

	req := httptest.NewRequest("GET", "/api/v1/backtest/"+j.ID, nil)
	w := httptest.NewRecorder()

	handler.GetStatus(w, req, j.ID)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp response.SuccessResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	data := resp.Data.(map[string]any)
	if data["job_id"] != j.ID {
		t.Errorf("expected job_id %s, got %s", j.ID, data["job_id"])
	}
}

func TestBacktestHandler_GetStatus_NotFound(t *testing.T) {
	jobStore := job.NewStore(100, time.Hour)
	backtester := backtest.New(&MockOHLCVProvider{})
	strategies := strategy.NewEngine()

	handler := NewBacktestHandler(jobStore, backtester, strategies)

	req := httptest.NewRequest("GET", "/api/v1/backtest/nonexistent", nil)
	w := httptest.NewRecorder()

	handler.GetStatus(w, req, "nonexistent")

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}
