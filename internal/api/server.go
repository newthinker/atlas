// internal/api/server.go
package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/newthinker/atlas/internal/api/handler/api"
	"github.com/newthinker/atlas/internal/api/handler/web"
	"github.com/newthinker/atlas/internal/api/job"
	"github.com/newthinker/atlas/internal/api/middleware"
	"github.com/newthinker/atlas/internal/app"
	"github.com/newthinker/atlas/internal/backtest"
	"github.com/newthinker/atlas/internal/broker"
	"github.com/newthinker/atlas/internal/metrics"
	"github.com/newthinker/atlas/internal/storage/signal"
	"github.com/newthinker/atlas/internal/strategy"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

// Server represents the HTTP server for ATLAS
type Server struct {
	httpServer *http.Server
	logger     *zap.Logger
	mux        *http.ServeMux
}

// Config holds server configuration
type Config struct {
	Host         string
	Port         int
	TemplatesDir string
	APIKey       string
	JobTTLHours  int
	MaxJobs      int
}

// Dependencies holds all server dependencies
type Dependencies struct {
	App              *app.App
	SignalStore      signal.Store
	Backtester       *backtest.Backtester
	Strategies       *strategy.Engine
	Metrics          *metrics.Registry
	ExecutionManager *broker.ExecutionManager
}

// watchlistAdapter adapts app.App to the web handler's WatchlistProvider interface
type watchlistAdapter struct {
	app *app.App
}

func (a *watchlistAdapter) GetWatchlist() []string {
	return a.app.GetWatchlist()
}

func (a *watchlistAdapter) GetWatchlistItems() []web.WatchlistItemData {
	appItems := a.app.GetWatchlistItems()
	result := make([]web.WatchlistItemData, len(appItems))
	for i, item := range appItems {
		result[i] = web.WatchlistItemData{
			Symbol:     item.Symbol,
			Name:       item.Name,
			Market:     item.Market,
			Type:       item.Type,
			Strategies: item.Strategies,
		}
	}
	return result
}

// NewServer creates a new HTTP server
func NewServer(cfg Config, deps Dependencies, logger *zap.Logger) (*Server, error) {
	mux := http.NewServeMux()

	s := &Server{
		httpServer: &http.Server{
			Addr:         fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
			Handler:      mux,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
		logger: logger,
		mux:    mux,
	}

	// Set up routes
	if err := s.setupRoutes(cfg, deps); err != nil {
		return nil, fmt.Errorf("setting up routes: %w", err)
	}

	return s, nil
}

// setupRoutes configures all HTTP routes
func (s *Server) setupRoutes(cfg Config, deps Dependencies) error {
	// Create job store
	ttl := time.Duration(cfg.JobTTLHours) * time.Hour
	if ttl == 0 {
		ttl = time.Hour
	}
	maxJobs := cfg.MaxJobs
	if maxJobs == 0 {
		maxJobs = 100
	}
	jobStore := job.NewStore(maxJobs, ttl)

	// Create API handlers
	signalsHandler := api.NewSignalsHandler(deps.SignalStore)
	watchlistHandler := api.NewWatchlistHandler(deps.App)
	backtestHandler := api.NewBacktestHandler(jobStore, deps.Backtester, deps.Strategies)
	analysisHandler := api.NewAnalysisHandler(deps.App)
	symbolsHandler := api.NewSymbolsHandler()

	// Auth middleware for API routes
	authMiddleware := middleware.APIKeyAuth(cfg.APIKey)

	// Metrics and logging middleware
	var metricsMiddleware func(http.Handler) http.Handler
	var loggingMiddleware func(http.Handler) http.Handler

	if deps.Metrics != nil {
		metricsMiddleware = metrics.HTTPMiddleware(deps.Metrics)
		loggingMiddleware = metrics.LoggingMiddleware(s.logger)

		// Add metrics endpoint
		s.mux.Handle("/metrics", promhttp.HandlerFor(deps.Metrics, promhttp.HandlerOpts{}))
	}

	// Helper to wrap handlers with all middleware (logging -> metrics -> auth)
	wrapHandler := func(handler http.Handler) http.Handler {
		h := authMiddleware(handler)
		if metricsMiddleware != nil {
			h = metricsMiddleware(h)
		}
		if loggingMiddleware != nil {
			h = loggingMiddleware(h)
		}
		return h
	}

	// API v1 routes (with auth, metrics, logging)
	s.mux.Handle("/api/v1/signals", wrapHandler(http.HandlerFunc(signalsHandler.List)))
	s.mux.Handle("/api/v1/signals/", wrapHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/v1/signals/")
		signalsHandler.GetByID(w, r, id)
	})))
	s.mux.Handle("/api/v1/watchlist", wrapHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			watchlistHandler.List(w, r)
		case http.MethodPost:
			watchlistHandler.Add(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})))
	s.mux.Handle("/api/v1/watchlist/", wrapHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		symbol := strings.TrimPrefix(r.URL.Path, "/api/v1/watchlist/")
		if r.Method == http.MethodDelete {
			watchlistHandler.Remove(w, r, symbol)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})))
	s.mux.Handle("/api/v1/symbols/search", wrapHandler(http.HandlerFunc(symbolsHandler.Search)))
	s.mux.Handle("/api/v1/backtest", wrapHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			backtestHandler.Create(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})))
	s.mux.Handle("/api/v1/backtest/", wrapHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jobID := strings.TrimPrefix(r.URL.Path, "/api/v1/backtest/")
		backtestHandler.GetStatus(w, r, jobID)
	})))
	s.mux.Handle("/api/v1/analysis/run", wrapHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			analysisHandler.Trigger(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})))

	// Health endpoint (no auth)
	s.mux.HandleFunc("/api/health", s.handleHealth)

	// Web UI routes (no auth - same origin)
	if cfg.TemplatesDir != "" {
		webHandler, err := web.NewHandler(cfg.TemplatesDir)
		if err != nil {
			return fmt.Errorf("creating web handler: %w", err)
		}

		// Wire up data providers
		if deps.App != nil {
			webHandler.SetWatchlistProvider(&watchlistAdapter{app: deps.App})
		}
		if deps.Strategies != nil {
			webHandler.SetStrategyProvider(deps.Strategies)
		}

		s.mux.HandleFunc("/", webHandler.Dashboard)
		s.mux.HandleFunc("/signals", webHandler.Signals)
		s.mux.HandleFunc("/watchlist", webHandler.Watchlist)
		s.mux.HandleFunc("/backtest", webHandler.Backtest)
		s.mux.HandleFunc("/settings", webHandler.Settings)
	}

	return nil
}

// Start starts the HTTP server
func (s *Server) Start() error {
	s.logger.Info("starting HTTP server", zap.String("addr", s.httpServer.Addr))
	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}
	return nil
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("shutting down HTTP server")
	return s.httpServer.Shutdown(ctx)
}

// Health handler
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}
