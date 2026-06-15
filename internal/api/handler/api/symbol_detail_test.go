// internal/api/handler/api/symbol_detail_test.go
package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/api/response"
	"github.com/newthinker/atlas/internal/collector"
	"github.com/newthinker/atlas/internal/core"
)

// Context Checkpoint: done_criteria → test mapping (TASK-013)
// functional[0] "qlib FetchHistory 报错时 GetHistory/GetIndicators 回落 external 返回外部数据(非500)"
//   → TestGetHistory_QlibErrorFallsBackToExternal / TestGetIndicators_QlibErrorFallsBackToExternal
// functional[1] "qlib 成功仍直接返回仓库数据"            → TestGetHistory_QlibSuccessNoFallback
// boundary[0]   "qlib 报错且外部不可用时返回 500"         → TestGetHistory_QlibErrorNoExternalReturns500
// boundary[1]   "非 qlib collector 报错不触发 fallback"   → TestGetHistory_NonQlibErrorNoFallback

// fakeDetailCollector is a full collector.Collector stub for handler tests.
type fakeDetailCollector struct {
	name    string
	covers  map[string]bool
	history []core.OHLCV
	histErr error
	// calls counts FetchHistory invocations (to assert fallback not double-routed).
	calls int
}

func (f *fakeDetailCollector) Name() string                    { return f.name }
func (f *fakeDetailCollector) SupportedMarkets() []core.Market { return nil }
func (f *fakeDetailCollector) Init(cfg collector.Config) error { return nil }
func (f *fakeDetailCollector) Start(ctx context.Context) error { return nil }
func (f *fakeDetailCollector) Stop() error                     { return nil }
func (f *fakeDetailCollector) FetchQuote(symbol string) (*core.Quote, error) {
	return &core.Quote{Symbol: symbol}, nil
}
func (f *fakeDetailCollector) FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	f.calls++
	if f.histErr != nil {
		return nil, f.histErr
	}
	return f.history, nil
}

// Covers makes the fake satisfy the warehouseCoverer interface used by
// collector.SelectForSymbol to prefer the qlib collector.
func (f *fakeDetailCollector) Covers(symbol string) bool { return f.covers[symbol] }

// extBars is a deterministic set of OHLCV bars returned by the external source.
func extBars() []core.OHLCV {
	return []core.OHLCV{
		{Symbol: "AAPL", Interval: "1d", Close: 1.0, Time: mustDate("2024-01-02")},
		{Symbol: "AAPL", Interval: "1d", Close: 2.0, Time: mustDate("2024-01-03")},
	}
}

func mustDate(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}

// newQlibPlusExternalRegistry registers a qlib collector that covers AAPL but
// fails FetchHistory, plus a working external "yahoo" collector.
func newQlibPlusExternalRegistry(qlibErr error, ext *fakeDetailCollector) *collector.Registry {
	reg := collector.NewRegistry()
	reg.Register(&fakeDetailCollector{
		name:    "qlib",
		covers:  map[string]bool{"AAPL": true},
		histErr: qlibErr,
	})
	if ext != nil {
		reg.Register(ext)
	}
	return reg
}

// TestGetHistory_QlibErrorFallsBackToExternal: qlib covers AAPL but FetchHistory
// errors → handler must fall back to the external collector and return its data,
// not a 500.
func TestGetHistory_QlibErrorFallsBackToExternal(t *testing.T) {
	ext := &fakeDetailCollector{name: "yahoo", history: extBars()}
	reg := newQlibPlusExternalRegistry(errors.New("warehouse read failed"), ext)
	h := NewSymbolDetailHandler(reg)

	req := httptest.NewRequest("GET", "/api/v1/symbols/AAPL/history?range=3M&interval=1d", nil)
	w := httptest.NewRecorder()
	h.GetHistory(w, req, "AAPL")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 after qlib→external fallback, got %d body=%s", w.Code, w.Body.String())
	}
	if ext.calls != 1 {
		t.Errorf("expected external FetchHistory called once, got %d", ext.calls)
	}

	var resp response.SuccessResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	data := resp.Data.(map[string]any)
	bars := data["data"].([]any)
	if len(bars) != 2 {
		t.Fatalf("expected 2 external bars, got %d", len(bars))
	}
}

// TestGetIndicators_QlibErrorFallsBackToExternal: same fallback path via GetIndicators.
func TestGetIndicators_QlibErrorFallsBackToExternal(t *testing.T) {
	ext := &fakeDetailCollector{name: "yahoo", history: extBars()}
	reg := newQlibPlusExternalRegistry(errors.New("warehouse read failed"), ext)
	h := NewSymbolDetailHandler(reg)

	req := httptest.NewRequest("GET", "/api/v1/symbols/AAPL/indicators?range=3M", nil)
	w := httptest.NewRecorder()
	h.GetIndicators(w, req, "AAPL")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 after qlib→external fallback, got %d body=%s", w.Code, w.Body.String())
	}
	if ext.calls != 1 {
		t.Errorf("expected external FetchHistory called once, got %d", ext.calls)
	}
}

// TestGetHistory_QlibSuccessNoFallback: qlib succeeds → its data returned, external untouched.
func TestGetHistory_QlibSuccessNoFallback(t *testing.T) {
	reg := collector.NewRegistry()
	reg.Register(&fakeDetailCollector{
		name:    "qlib",
		covers:  map[string]bool{"AAPL": true},
		history: []core.OHLCV{{Symbol: "AAPL", Interval: "1d", Close: 9.0, Time: mustDate("2024-01-02")}},
	})
	ext := &fakeDetailCollector{name: "yahoo", history: extBars()}
	reg.Register(ext)
	h := NewSymbolDetailHandler(reg)

	req := httptest.NewRequest("GET", "/api/v1/symbols/AAPL/history?range=3M&interval=1d", nil)
	w := httptest.NewRecorder()
	h.GetHistory(w, req, "AAPL")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ext.calls != 0 {
		t.Errorf("external must not be called when qlib succeeds, got %d calls", ext.calls)
	}
	var resp response.SuccessResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	bars := resp.Data.(map[string]any)["data"].([]any)
	if len(bars) != 1 {
		t.Fatalf("expected 1 warehouse bar, got %d", len(bars))
	}
}

// TestGetHistory_QlibErrorNoExternalReturns500: qlib errors and no external
// collector available → original 500 error semantics preserved.
func TestGetHistory_QlibErrorNoExternalReturns500(t *testing.T) {
	reg := newQlibPlusExternalRegistry(errors.New("warehouse read failed"), nil)
	h := NewSymbolDetailHandler(reg)

	req := httptest.NewRequest("GET", "/api/v1/symbols/AAPL/history?range=3M&interval=1d", nil)
	w := httptest.NewRecorder()
	h.GetHistory(w, req, "AAPL")

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when qlib errors and no external, got %d", w.Code)
	}
}

// TestGetHistory_NonQlibErrorNoFallback: a non-qlib selected collector that
// errors must NOT trigger fallback (zero behaviour change for existing path).
func TestGetHistory_NonQlibErrorNoFallback(t *testing.T) {
	reg := collector.NewRegistry()
	// Only yahoo registered; it is selected for AAPL and errors.
	yahoo := &fakeDetailCollector{name: "yahoo", histErr: errors.New("api down")}
	reg.Register(yahoo)
	h := NewSymbolDetailHandler(reg)

	req := httptest.NewRequest("GET", "/api/v1/symbols/AAPL/history?range=3M&interval=1d", nil)
	w := httptest.NewRecorder()
	h.GetHistory(w, req, "AAPL")

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("non-qlib error must stay 500 (no fallback), got %d", w.Code)
	}
	if yahoo.calls != 1 {
		t.Errorf("yahoo should be called exactly once (no re-route), got %d", yahoo.calls)
	}
}
