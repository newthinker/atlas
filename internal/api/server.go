// internal/api/server.go
package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/newthinker/atlas/internal/api/handler/web"
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
}

// NewServer creates a new HTTP server
func NewServer(cfg Config, logger *zap.Logger) (*Server, error) {
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
	if err := s.setupRoutes(cfg.TemplatesDir); err != nil {
		return nil, fmt.Errorf("setting up routes: %w", err)
	}

	return s, nil
}

// setupRoutes configures all HTTP routes
func (s *Server) setupRoutes(templatesDir string) error {
	// Web UI routes
	webHandler, err := web.NewHandler(templatesDir)
	if err != nil {
		return fmt.Errorf("creating web handler: %w", err)
	}

	s.mux.HandleFunc("/", webHandler.Dashboard)
	s.mux.HandleFunc("/signals", webHandler.Signals)
	s.mux.HandleFunc("/watchlist", webHandler.Watchlist)
	s.mux.HandleFunc("/backtest", webHandler.Backtest)

	// API routes (placeholder for future)
	s.mux.HandleFunc("/api/signals/recent", s.handleRecentSignals)
	s.mux.HandleFunc("/api/backtest", s.handleBacktest)
	s.mux.HandleFunc("/api/watchlist", s.handleWatchlist)
	s.mux.HandleFunc("/api/health", s.handleHealth)

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

// API handlers (placeholders)
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

func (s *Server) handleRecentSignals(w http.ResponseWriter, r *http.Request) {
	// TODO: Return actual signals
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(`<div class="p-6 text-gray-500">No recent signals</div>`))
}

func (s *Server) handleBacktest(w http.ResponseWriter, r *http.Request) {
	// TODO: Run actual backtest
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(`<div class="p-6 text-gray-500">Backtest feature coming soon</div>`))
}

func (s *Server) handleWatchlist(w http.ResponseWriter, r *http.Request) {
	// TODO: Handle watchlist CRUD
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
}
