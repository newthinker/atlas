package app

import (
	"context"
	"fmt"
	"strings"
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

// Market constants
const (
	MarketAShare = "A股"
	MarketHShare = "H股"
	MarketUS     = "美股"
	MarketCrypto = "数字货币"
)

// Type constants
const (
	TypeStock  = "股票"
	TypeFund   = "基金"
	TypeBond   = "债券"
	TypeETF    = "ETF"
	TypeOption = "期权"
	TypeFuture = "期货"
	TypeCrypto = "加密货币"
)

// WatchlistItem represents an item in the watchlist with associated metadata
type WatchlistItem struct {
	Symbol     string
	Name       string
	Market     string // "A股", "H股", "美股", "数字货币"
	Type       string // "股票", "基金", "债券", "ETF", "期权", "期货", "加密货币"
	Strategies []string
}

// App is the main application orchestrator
type App struct {
	cfg        *config.Config
	logger     *zap.Logger
	collectors *collector.Registry
	strategies *strategy.Engine
	notifiers  *notifier.Registry
	router     *router.Router

	watchlistItems []WatchlistItem
	watchlistSet   map[string]struct{}
	interval       time.Duration

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
		cfg:            cfg,
		logger:         logger,
		collectors:     collectors,
		strategies:     strategies,
		notifiers:      notifiers,
		router:         r,
		watchlistItems: []WatchlistItem{},
		watchlistSet:   make(map[string]struct{}),
		interval:       5 * time.Minute,
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
	a.watchlistItems = make([]WatchlistItem, len(symbols))
	a.watchlistSet = make(map[string]struct{}, len(symbols))
	for i, s := range symbols {
		a.watchlistItems[i] = WatchlistItem{Symbol: s, Name: s}
		a.watchlistSet[s] = struct{}{}
	}
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
		zap.Int("watchlist_count", len(a.watchlistItems)),
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
	items := make([]WatchlistItem, len(a.watchlistItems))
	copy(items, a.watchlistItems)
	a.mu.RUnlock()

	if len(items) == 0 {
		a.logger.Debug("no symbols in watchlist")
		return
	}

	a.logger.Debug("starting analysis cycle", zap.Int("symbols", len(items)))

	for _, item := range items {
		if ctx.Err() != nil {
			return
		}

		a.analyzeSymbol(ctx, item.Symbol)
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
		"watchlist":   len(a.watchlistItems),
		"collectors":  len(a.collectors.GetAll()),
		"strategies":  len(a.strategies.GetAll()),
		"notifiers":   len(a.notifiers.GetAll()),
		"router":      a.router.GetStats(),
	}
}

// GetWatchlist returns the current watchlist symbols.
func (a *App) GetWatchlist() []string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	result := make([]string, len(a.watchlistItems))
	for i, item := range a.watchlistItems {
		result[i] = item.Symbol
	}
	return result
}

// GetWatchlistItems returns the full watchlist items with metadata.
func (a *App) GetWatchlistItems() []WatchlistItem {
	a.mu.RLock()
	defer a.mu.RUnlock()
	result := make([]WatchlistItem, len(a.watchlistItems))
	copy(result, a.watchlistItems)
	return result
}

// AddToWatchlist adds a symbol to the watchlist.
func (a *App) AddToWatchlist(symbol string) {
	a.AddToWatchlistWithDetails(symbol, symbol, "", "", nil)
}

// AddToWatchlistWithDetails adds a symbol to the watchlist with name and strategies.
func (a *App) AddToWatchlistWithDetails(symbol, name, market, assetType string, strategies []string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, exists := a.watchlistSet[symbol]; exists {
		return
	}
	a.watchlistSet[symbol] = struct{}{}
	if name == "" {
		name = symbol
	}
	// Auto-detect market and type if not provided
	if market == "" {
		market = DetectMarket(symbol)
	}
	if assetType == "" {
		assetType = DetectType(symbol)
	}
	a.watchlistItems = append(a.watchlistItems, WatchlistItem{
		Symbol:     symbol,
		Name:       name,
		Market:     market,
		Type:       assetType,
		Strategies: strategies,
	})
}

// DetectMarket auto-detects the market based on symbol pattern
func DetectMarket(symbol string) string {
	upperSymbol := strings.ToUpper(symbol)
	switch {
	case strings.HasSuffix(upperSymbol, ".SH") || strings.HasSuffix(upperSymbol, ".SZ"):
		return MarketAShare
	case strings.HasSuffix(upperSymbol, ".HK"):
		return MarketHShare
	case strings.Contains(upperSymbol, "-USD") || strings.Contains(upperSymbol, "-USDT") ||
		strings.HasPrefix(upperSymbol, "BTC") || strings.HasPrefix(upperSymbol, "ETH"):
		return MarketCrypto
	default:
		return MarketUS
	}
}

// DetectType auto-detects the asset type based on symbol pattern
func DetectType(symbol string) string {
	upperSymbol := strings.ToUpper(symbol)
	if strings.Contains(upperSymbol, "-USD") || strings.Contains(upperSymbol, "-USDT") ||
		strings.HasPrefix(upperSymbol, "BTC") || strings.HasPrefix(upperSymbol, "ETH") {
		return TypeCrypto
	}
	return TypeStock
}

// RemoveFromWatchlist removes a symbol from the watchlist.
func (a *App) RemoveFromWatchlist(symbol string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, exists := a.watchlistSet[symbol]; !exists {
		return false
	}
	delete(a.watchlistSet, symbol)
	for i, item := range a.watchlistItems {
		if item.Symbol == symbol {
			a.watchlistItems = append(a.watchlistItems[:i], a.watchlistItems[i+1:]...)
			break
		}
	}
	return true
}

// GetCollectors returns all registered collectors.
func (a *App) GetCollectors() []collector.Collector {
	return a.collectors.GetAll()
}
