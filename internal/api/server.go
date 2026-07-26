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
	"github.com/newthinker/atlas/internal/collector"
	"github.com/newthinker/atlas/internal/config"
	"github.com/newthinker/atlas/internal/metrics"
	"github.com/newthinker/atlas/internal/prism/sankey"
	prismstore "github.com/newthinker/atlas/internal/storage/prism"
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
	Config           *config.Config
	PrismStore       *prismstore.Store // nil = prism 未启用,不注册路由
	PrismLow         float64
	PrismHigh        float64
	// PrismSankey 为 nil = 模板未配置或加载失败,不注册财报桥路由。
	// 注册一个必然报错的端点比不注册更糟: 前端拿到 500 无从判断是配置问题还是故障。
	PrismSankey *sankey.Service
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

// configAdapter adapts config.Config to the web handler's ConfigProvider interface
type configAdapter struct {
	cfg *config.Config
}

func (a *configAdapter) GetNotifiers() map[string]web.NotifierInfo {
	result := make(map[string]web.NotifierInfo)
	if a.cfg == nil {
		return result
	}
	for name, notifier := range a.cfg.Notifiers {
		var details string
		switch name {
		case "telegram":
			if notifier.ChatID != "" {
				details = fmt.Sprintf("Chat ID: %s", notifier.ChatID)
			}
		case "email":
			if notifier.Host != "" {
				details = fmt.Sprintf("SMTP: %s:%d", notifier.Host, notifier.Port)
				if len(notifier.To) > 0 {
					details += fmt.Sprintf(", To: %s", strings.Join(notifier.To, ", "))
				}
			}
		case "webhook":
			if notifier.URL != "" {
				details = fmt.Sprintf("URL: %s", notifier.URL)
			}
		}
		result[name] = web.NotifierInfo{
			Enabled: notifier.Enabled,
			Type:    name,
			Details: details,
		}
	}
	return result
}

func (a *configAdapter) GetRouterConfig() web.RouterInfo {
	if a.cfg == nil {
		return web.RouterInfo{MinConfidence: 0.6, CooldownHours: 4}
	}
	return web.RouterInfo{
		MinConfidence: a.cfg.Router.MinConfidence,
		CooldownHours: a.cfg.Router.CooldownHours,
	}
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

	// Create symbol detail handler with collectors
	var symbolDetailHandler *api.SymbolDetailHandler
	if deps.App != nil {
		reg := collector.NewRegistry()
		for _, c := range deps.App.GetCollectors() {
			reg.Register(c)
		}
		symbolDetailHandler = api.NewSymbolDetailHandler(reg)
	}

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

	// Prism valuation board API (only when a store is injected)
	if deps.PrismStore != nil {
		prismHandler := api.NewPrismHandler(deps.PrismStore, deps.PrismLow, deps.PrismHigh)
		s.mux.Handle("/api/prism/board", wrapHandler(http.HandlerFunc(prismHandler.Board)))
		s.mux.Handle("/api/prism/series", wrapHandler(http.HandlerFunc(prismHandler.Series)))
	}

	// Prism earnings bridge API (only when templates loaded successfully)
	if deps.PrismSankey != nil {
		sankeyHandler := api.NewSankeyHandler(deps.PrismSankey)
		s.mux.Handle("/api/prism/sankey", wrapHandler(http.HandlerFunc(sankeyHandler.Sankey)))
		s.mux.Handle("/api/prism/fundamental", wrapHandler(http.HandlerFunc(sankeyHandler.Fundamental)))
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

	// Symbol detail API routes
	if symbolDetailHandler != nil {
		s.mux.Handle("/api/v1/symbols/", wrapHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := strings.TrimPrefix(r.URL.Path, "/api/v1/symbols/")
			parts := strings.Split(path, "/")
			if len(parts) < 2 {
				http.Error(w, "Invalid path", http.StatusBadRequest)
				return
			}
			symbol := parts[0]
			action := parts[1]

			switch action {
			case "quote":
				symbolDetailHandler.GetQuote(w, r, symbol)
			case "history":
				symbolDetailHandler.GetHistory(w, r, symbol)
			case "indicators":
				symbolDetailHandler.GetIndicators(w, r, symbol)
			default:
				http.Error(w, "Not found", http.StatusNotFound)
			}
		})))
	}

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
		if deps.Config != nil {
			webHandler.SetConfigProvider(&configAdapter{cfg: deps.Config})
		}
		if deps.SignalStore != nil {
			webHandler.SetSignalStore(deps.SignalStore)
		}

		s.mux.HandleFunc("/", webHandler.Dashboard)
		s.mux.HandleFunc("/signals", webHandler.Signals)
		s.mux.HandleFunc("/watchlist", webHandler.Watchlist)
		s.mux.HandleFunc("/backtest", webHandler.Backtest)
		s.mux.HandleFunc("/settings", webHandler.Settings)

		// Symbol detail page
		s.mux.HandleFunc("/symbols/", func(w http.ResponseWriter, r *http.Request) {
			symbol := strings.TrimPrefix(r.URL.Path, "/symbols/")
			if symbol == "" {
				http.Redirect(w, r, "/watchlist", http.StatusFound)
				return
			}
			webHandler.SymbolDetail(w, r, symbol)
		})

		// Prism valuation pages (only when the store is wired)
		if deps.PrismStore != nil {
			webHandler.SetPrismProvider(deps.PrismStore, deps.PrismLow, deps.PrismHigh)
			s.mux.HandleFunc("/prism/board", webHandler.PrismBoard)
			s.mux.HandleFunc("/prism/compare", webHandler.PrismCompare)
			s.mux.HandleFunc("/prism/detail/", func(w http.ResponseWriter, r *http.Request) {
				symbol := strings.TrimPrefix(r.URL.Path, "/prism/detail/")
				webHandler.PrismDetail(w, r, symbol)
			})
			s.mux.HandleFunc("/static/", webHandler.Static)
		}

		// The earnings bridge page is registered whether or not the feature is
		// on, and deliberately outside the block above: an unregistered
		// /prism/... path falls through to the "/" catch-all and answers 200
		// with the dashboard, so a bookmark into a disabled feature shows an
		// unrelated page. Registered, the handler answers a 404 that names the
		// reason (sankeyNotEnabledMsg). The nil check keeps a typed-nil
		// *sankey.Service out of the interface — it would pass the handler's
		// nil test and panic on the first call.
		if deps.PrismSankey != nil {
			webHandler.SetPrismSankey(deps.PrismSankey)
		}
		s.mux.HandleFunc("/prism/sankey/", func(w http.ResponseWriter, r *http.Request) {
			symbol := strings.TrimPrefix(r.URL.Path, "/prism/sankey/")
			webHandler.PrismSankey(w, r, symbol)
		})
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
