// internal/api/server_test.go
package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/newthinker/atlas/internal/app"
	"github.com/newthinker/atlas/internal/backtest"
	"github.com/newthinker/atlas/internal/config"
	"github.com/newthinker/atlas/internal/storage/signal"
	"github.com/newthinker/atlas/internal/strategy"
	"go.uber.org/zap"
)

func TestServer_Health(t *testing.T) {
	deps := Dependencies{
		App:         app.New(config.Defaults(), zap.NewNop()),
		SignalStore: signal.NewMemoryStore(100),
		Backtester:  nil, // Not needed for health check
		Strategies:  strategy.NewEngine(),
	}

	srv, err := NewServer(Config{
		Host: "localhost",
		Port: 0,
	}, deps, zap.NewNop())
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestServer_APIAuth_Required(t *testing.T) {
	deps := Dependencies{
		App:         app.New(config.Defaults(), zap.NewNop()),
		SignalStore: signal.NewMemoryStore(100),
		Backtester:  nil,
		Strategies:  strategy.NewEngine(),
	}

	srv, _ := NewServer(Config{
		Host:   "localhost",
		Port:   0,
		APIKey: "test-key",
	}, deps, zap.NewNop())

	// Without API key
	req := httptest.NewRequest("GET", "/api/v1/signals", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without key, got %d", w.Code)
	}
}

func TestServer_APIAuth_ValidKey(t *testing.T) {
	deps := Dependencies{
		App:         app.New(config.Defaults(), zap.NewNop()),
		SignalStore: signal.NewMemoryStore(100),
		Backtester:  nil,
		Strategies:  strategy.NewEngine(),
	}

	srv, _ := NewServer(Config{
		Host:   "localhost",
		Port:   0,
		APIKey: "test-key",
	}, deps, zap.NewNop())

	// With API key
	req := httptest.NewRequest("GET", "/api/v1/signals", nil)
	req.Header.Set("X-API-Key", "test-key")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with key, got %d", w.Code)
	}
}

func TestServer_APIAuth_Disabled(t *testing.T) {
	deps := Dependencies{
		App:         app.New(config.Defaults(), zap.NewNop()),
		SignalStore: signal.NewMemoryStore(100),
		Backtester:  nil,
		Strategies:  strategy.NewEngine(),
	}

	// Empty APIKey = disabled auth
	srv, _ := NewServer(Config{
		Host:   "localhost",
		Port:   0,
		APIKey: "",
	}, deps, zap.NewNop())

	req := httptest.NewRequest("GET", "/api/v1/signals", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with disabled auth, got %d", w.Code)
	}
}

func TestServer_Watchlist(t *testing.T) {
	a := app.New(config.Defaults(), zap.NewNop())
	a.SetWatchlist([]string{"AAPL"})

	deps := Dependencies{
		App:         a,
		SignalStore: signal.NewMemoryStore(100),
		Backtester:  nil,
		Strategies:  strategy.NewEngine(),
	}

	srv, _ := NewServer(Config{
		Host: "localhost",
		Port: 0,
	}, deps, zap.NewNop())

	req := httptest.NewRequest("GET", "/api/v1/watchlist", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestServer_WithBacktester(t *testing.T) {
	a := app.New(config.Defaults(), zap.NewNop())

	deps := Dependencies{
		App:         a,
		SignalStore: signal.NewMemoryStore(100),
		Backtester:  backtest.New(nil), // nil provider is ok for route setup
		Strategies:  strategy.NewEngine(),
	}

	srv, err := NewServer(Config{
		Host: "localhost",
		Port: 0,
	}, deps, zap.NewNop())

	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Just verify we can create the server with backtester
	if srv == nil {
		t.Error("expected server to be created")
	}
}
