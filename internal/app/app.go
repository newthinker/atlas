package app

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/newthinker/atlas/internal/collector"
	"github.com/newthinker/atlas/internal/config"
	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/notifier"
	"github.com/newthinker/atlas/internal/router"
	"github.com/newthinker/atlas/internal/strategy"
	"go.uber.org/zap"
)

// App is the main application orchestrator
type App struct {
	cfg        *config.Config
	logger     *zap.Logger
	collectors *collector.Registry
	strategies *strategy.Engine
	notifiers  *notifier.Registry
	router     *router.Router

	watchlist []string
	interval  time.Duration

	mu      sync.RWMutex
	running bool
	cancel  context.CancelFunc
}

// New creates a new App instance
func New(cfg *config.Config, logger *zap.Logger) *App {
	if logger == nil {
		logger = zap.NewNop()
	}

	collectors := collector.NewRegistry()
	strategies := strategy.NewEngine()
	notifiers := notifier.NewRegistry()

	routerCfg := router.Config{
		MinConfidence:    0.5,
		CooldownDuration: 1 * time.Hour,
		EnabledActions:   []core.Action{core.ActionBuy, core.ActionSell, core.ActionStrongBuy, core.ActionStrongSell},
	}
	r := router.New(routerCfg, notifiers, logger)

	return &App{
		cfg:        cfg,
		logger:     logger,
		collectors: collectors,
		strategies: strategies,
		notifiers:  notifiers,
		router:     r,
		watchlist:  []string{},
		interval:   5 * time.Minute,
	}
}

// RegisterCollector adds a collector to the app
func (a *App) RegisterCollector(c collector.Collector) {
	a.collectors.Register(c)
}

// RegisterStrategy adds a strategy to the app
func (a *App) RegisterStrategy(s strategy.Strategy) {
	a.strategies.Register(s)
}

// RegisterNotifier adds a notifier to the app
func (a *App) RegisterNotifier(n notifier.Notifier) error {
	return a.notifiers.Register(n)
}

// SetWatchlist sets the symbols to monitor
func (a *App) SetWatchlist(symbols []string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.watchlist = symbols
}

// SetInterval sets the analysis interval
func (a *App) SetInterval(d time.Duration) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.interval = d
}

// Start begins the monitoring loop
func (a *App) Start(ctx context.Context) error {
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return fmt.Errorf("app already running")
	}
	a.running = true

	ctx, cancel := context.WithCancel(ctx)
	a.cancel = cancel
	a.mu.Unlock()

	a.logger.Info("ATLAS starting",
		zap.Strings("watchlist", a.watchlist),
		zap.Duration("interval", a.interval),
	)

	// Initial run
	a.runAnalysisCycle(ctx)

	// Start ticker for periodic analysis
	ticker := time.NewTicker(a.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			a.logger.Info("ATLAS shutting down")
			a.mu.Lock()
			a.running = false
			a.mu.Unlock()
			return ctx.Err()
		case <-ticker.C:
			a.runAnalysisCycle(ctx)
		}
	}
}

// Stop stops the monitoring loop
func (a *App) Stop() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cancel != nil {
		a.cancel()
	}
}

// runAnalysisCycle fetches data and runs strategies for all symbols
func (a *App) runAnalysisCycle(ctx context.Context) {
	a.mu.RLock()
	symbols := make([]string, len(a.watchlist))
	copy(symbols, a.watchlist)
	a.mu.RUnlock()

	if len(symbols) == 0 {
		a.logger.Debug("no symbols in watchlist")
		return
	}

	a.logger.Debug("starting analysis cycle", zap.Int("symbols", len(symbols)))

	for _, symbol := range symbols {
		if ctx.Err() != nil {
			return
		}

		a.analyzeSymbol(ctx, symbol)
	}
}

// analyzeSymbol fetches data and runs analysis for a single symbol
func (a *App) analyzeSymbol(ctx context.Context, symbol string) {
	// Get collector for this symbol (simplified: just use first available)
	collectors := a.collectors.GetAll()
	if len(collectors) == 0 {
		a.logger.Warn("no collectors available", zap.String("symbol", symbol))
		return
	}

	// Fetch historical data (last 250 trading days for 200-day MA)
	end := time.Now()
	start := end.AddDate(0, 0, -365)

	var ohlcv []core.OHLCV
	var fetchErr error

	for _, c := range collectors {
		ohlcv, fetchErr = c.FetchHistory(symbol, start, end, "1d")
		if fetchErr == nil && len(ohlcv) > 0 {
			break
		}
	}

	if fetchErr != nil || len(ohlcv) == 0 {
		a.logger.Debug("failed to fetch data",
			zap.String("symbol", symbol),
			zap.Error(fetchErr),
		)
		return
	}

	// Run analysis
	analysisCtx := strategy.AnalysisContext{
		Symbol: symbol,
		OHLCV:  ohlcv,
		Now:    time.Now(),
	}

	signals, err := a.strategies.Analyze(ctx, analysisCtx)
	if err != nil {
		a.logger.Error("analysis failed",
			zap.String("symbol", symbol),
			zap.Error(err),
		)
		return
	}

	// Route signals
	for _, signal := range signals {
		if err := a.router.Route(signal); err != nil {
			a.logger.Error("failed to route signal",
				zap.String("symbol", symbol),
				zap.Error(err),
			)
		}
	}

	if len(signals) > 0 {
		a.logger.Info("signals generated",
			zap.String("symbol", symbol),
			zap.Int("count", len(signals)),
		)
	}
}

// RunOnce performs a single analysis cycle (useful for testing)
func (a *App) RunOnce(ctx context.Context) {
	a.runAnalysisCycle(ctx)
}

// GetStats returns application statistics
func (a *App) GetStats() map[string]any {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return map[string]any{
		"running":     a.running,
		"watchlist":   len(a.watchlist),
		"collectors":  len(a.collectors.GetAll()),
		"strategies":  len(a.strategies.GetAll()),
		"notifiers":   len(a.notifiers.GetAll()),
		"router":      a.router.GetStats(),
	}
}

// GetWatchlist returns the current watchlist.
func (a *App) GetWatchlist() []string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	result := make([]string, len(a.watchlist))
	copy(result, a.watchlist)
	return result
}

// AddToWatchlist adds a symbol to the watchlist.
func (a *App) AddToWatchlist(symbol string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	// Check if already exists
	for _, s := range a.watchlist {
		if s == symbol {
			return
		}
	}
	a.watchlist = append(a.watchlist, symbol)
}

// RemoveFromWatchlist removes a symbol from the watchlist.
func (a *App) RemoveFromWatchlist(symbol string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i, s := range a.watchlist {
		if s == symbol {
			a.watchlist = append(a.watchlist[:i], a.watchlist[i+1:]...)
			return true
		}
	}
	return false
}

// GetCollectors returns all registered collectors.
func (a *App) GetCollectors() []collector.Collector {
	return a.collectors.GetAll()
}
