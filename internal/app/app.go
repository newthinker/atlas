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
	"github.com/newthinker/atlas/internal/meta"
	"github.com/newthinker/atlas/internal/notifier"
	"github.com/newthinker/atlas/internal/router"
	"github.com/newthinker/atlas/internal/storage/signal"
	"github.com/newthinker/atlas/internal/strategy"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
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
	arbitrator signalArbitrator
	executor   SignalExecutor

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

// SetSignalStore wires a signal store into the router so generated signals are
// persisted for the API and web UI.
func (a *App) SetSignalStore(store signal.Store) {
	a.router.SetSignalStore(store)
}

// signalArbitrator resolves conflicting signals into a single decision. It is
// satisfied by *meta.Arbitrator and defined here so the analysis loop can bound
// arbitration latency (see ADR-4) and tests can inject a stub.
type signalArbitrator interface {
	Arbitrate(ctx context.Context, req meta.ArbitrationRequest) (*meta.ArbitrationResult, error)
}

// SetArbitrator enables LLM-based arbitration of conflicting signals. When set,
// symbols producing multiple signals are resolved into a single decision before
// routing.
func (a *App) SetArbitrator(arb *meta.Arbitrator) {
	// Guard against the typed-nil interface pitfall: a nil *meta.Arbitrator must
	// disable arbitration, not become a non-nil interface value.
	if arb == nil {
		a.setArbitratorClient(nil)
		return
	}
	a.setArbitratorClient(arb)
}

// setArbitratorClient stores the arbitrator behind its interface; used by
// SetArbitrator and by tests injecting a stub.
func (a *App) setArbitratorClient(arb signalArbitrator) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.arbitrator = arb
}

// SignalExecutor submits a routed signal for execution (e.g. order placement).
// It is defined here, on the consuming side, so the app orchestration layer does
// not depend on the broker infrastructure layer (see ADR-2).
type SignalExecutor interface {
	SubmitSignal(ctx context.Context, sig core.Signal) error
}

// SetExecutor wires a SignalExecutor so that, after routing, every generated
// signal in an analysis cycle is submitted for execution. When unset, the
// analysis cycle behaves exactly as before.
func (a *App) SetExecutor(e SignalExecutor) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.executor = e
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

	workers := 0
	if a.cfg != nil {
		workers = a.cfg.Analysis.Workers
	}

	a.logger.Debug("starting analysis cycle",
		zap.Int("symbols", len(items)),
		zap.Int("workers", workers),
	)

	// workers <= 1 keeps the original serial path for full backward compatibility.
	if workers <= 1 {
		for _, item := range items {
			if ctx.Err() != nil {
				return
			}
			a.analyzeSymbolSafe(ctx, item)
		}
		return
	}

	// Parallel path: bounded concurrency via errgroup.SetLimit. Each symbol is
	// processed independently; per-symbol failures are isolated in
	// analyzeSymbolSafe so they never cancel siblings or crash the process.
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(workers)
	for _, item := range items {
		if gctx.Err() != nil {
			break // stop dispatching new symbols once cancelled
		}
		g.Go(func() error {
			if gctx.Err() != nil {
				return nil
			}
			a.analyzeSymbolSafe(gctx, item)
			return nil
		})
	}
	_ = g.Wait()
}

// analyzeSymbolSafe runs analyzeSymbol with panic recovery so a single symbol's
// failure cannot crash the process or abort other symbols in the cycle.
func (a *App) analyzeSymbolSafe(ctx context.Context, item WatchlistItem) {
	defer func() {
		if r := recover(); r != nil {
			a.logger.Error("analyzeSymbol panicked, skipping symbol",
				zap.String("symbol", item.Symbol),
				zap.Any("panic", r),
			)
		}
	}()
	a.analyzeSymbol(ctx, item)
}

// analyzeSymbol fetches data and runs analysis for a single watchlist item.
func (a *App) analyzeSymbol(ctx context.Context, item WatchlistItem) {
	symbol := item.Symbol

	collectors := a.orderedCollectors(symbol)
	if len(collectors) == 0 {
		a.logger.Warn("no collectors available", zap.String("symbol", symbol))
		return
	}

	// Fetch historical data (last 250 trading days for 200-day MA), preferring
	// the collector that matches the symbol's market and falling back to others.
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

	// Honour per-symbol strategy selection when configured; otherwise run all.
	var signals []core.Signal
	var err error
	if len(item.Strategies) > 0 {
		signals, err = a.strategies.AnalyzeWithStrategies(ctx, analysisCtx, item.Strategies)
	} else {
		signals, err = a.strategies.Analyze(ctx, analysisCtx)
	}
	if err != nil {
		a.logger.Error("analysis failed",
			zap.String("symbol", symbol),
			zap.Error(err),
		)
		return
	}

	if len(signals) == 0 {
		return
	}

	// Resolve conflicting signals via the LLM arbitrator when enabled.
	signals = a.arbitrate(ctx, symbol, signals)

	// Snapshot the executor under the lock so SetExecutor can run concurrently.
	a.mu.RLock()
	executor := a.executor
	a.mu.RUnlock()

	// Route signals, then submit each for execution when an executor is wired.
	for _, sig := range signals {
		routed, err := a.router.Route(sig)
		if err != nil {
			a.logger.Error("failed to route signal",
				zap.String("symbol", symbol),
				zap.Error(err),
			)
		}

		// Only submit signals that were actually routed. A signal suppressed by
		// the router (cooldown/confidence/action filters) must not be submitted
		// for execution, otherwise a deduplicated signal still places an order.
		if routed && executor != nil {
			if err := executor.SubmitSignal(ctx, sig); err != nil {
				// Record and continue: one failed submission must not block
				// subsequent signals or subsequent symbols.
				a.logger.Error("failed to submit signal for execution",
					zap.String("symbol", symbol),
					zap.String("action", string(sig.Action)),
					zap.Error(err),
				)
			}
		}
	}

	a.logger.Info("signals generated",
		zap.String("symbol", symbol),
		zap.Int("count", len(signals)),
	)
}

// orderedCollectors returns collectors with the best match for the symbol first,
// so analysis fetches from the right data source while preserving fallbacks.
func (a *App) orderedCollectors(symbol string) []collector.Collector {
	all := a.collectors.GetAll()
	preferred := collector.SelectForSymbol(a.collectors, symbol)
	if preferred == nil {
		return all
	}

	ordered := make([]collector.Collector, 0, len(all))
	ordered = append(ordered, preferred)
	for _, c := range all {
		if c != preferred {
			ordered = append(ordered, c)
		}
	}
	return ordered
}

// arbitrate resolves multiple signals for a symbol into a single decision using
// the LLM arbitrator. It returns the original signals unchanged when no
// arbitrator is configured, there is nothing to resolve, or arbitration fails.
func (a *App) arbitrate(ctx context.Context, symbol string, signals []core.Signal) []core.Signal {
	a.mu.RLock()
	arb := a.arbitrator
	a.mu.RUnlock()

	if arb == nil || len(signals) < 2 {
		return signals
	}

	// Bound a single arbitration call so a slow LLM cannot stall the cycle.
	// On timeout the call returns ctx.Err() and we degrade to the original
	// signals, matching the existing "on error, route originals" semantics.
	timeout := 15 * time.Second
	if a.cfg != nil && a.cfg.Meta.Arbitrator.Timeout > 0 {
		timeout = a.cfg.Meta.Arbitrator.Timeout
	}
	arbCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result, err := arb.Arbitrate(arbCtx, meta.ArbitrationRequest{
		Symbol:             symbol,
		Market:             collector.MarketForSymbol(symbol),
		ConflictingSignals: signals,
	})
	if err != nil {
		a.logger.Warn("arbitration failed, routing original signals",
			zap.String("symbol", symbol),
			zap.Error(err),
		)
		return signals
	}

	return []core.Signal{{
		Symbol:      symbol,
		Action:      result.Decision,
		Confidence:  result.Confidence,
		Reason:      result.Reasoning,
		Strategy:    "meta_arbitrator",
		GeneratedAt: time.Now(),
	}}
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
		"running":    a.running,
		"watchlist":  len(a.watchlistItems),
		"collectors": len(a.collectors.GetAll()),
		"strategies": len(a.strategies.GetAll()),
		"notifiers":  len(a.notifiers.GetAll()),
		"router":     a.router.GetStats(),
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
