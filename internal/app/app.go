package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
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
	"github.com/newthinker/atlas/internal/valuation"
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
	TypeIndex  = "指数"
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

	valuationSrc ValuationSource
	epsSrc       EPSSource

	watchlistItems []WatchlistItem
	watchlistSet   map[string]struct{}
	interval       time.Duration

	mu      sync.RWMutex
	running bool
	cancel  context.CancelFunc

	// warned dedupes per-key warnings (e.g. asset-type binding mismatches and
	// out-of-list index symbols) so the parallel analysis loop logs each once.
	warned sync.Map
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

// ValuationSource provides a PE-TTM cvpos percentile (0-100) for a symbol. It is
// satisfied by *lixinger.Lixinger; declared as a narrow interface so the app
// layer does not depend on the concrete collector package.
type ValuationSource interface {
	FetchValuationPercentile(symbol string, lookbackYears int) (float64, error)
}

// EPSSource provides a trailing diluted EPS history for a symbol. It is
// satisfied by *yahoo.Yahoo.
type EPSSource interface {
	FetchEPSHistory(symbol string, start, end time.Time) ([]core.EPSPoint, error)
}

// SetValuationSources injects the valuation (lixinger) and EPS (yahoo) data
// sources used to assemble PE-percentile fundamentals. Either may be nil, in
// which case the corresponding path is treated as unavailable.
//
// Invariant (QA S1): this MUST be called during assembly, before Start. The
// fields are read lock-free by buildFundamental on the parallel analysis
// workers; set-once-before-Start is what keeps that race-free (mirrors the
// executor wiring contract). Do not call it once the analysis loop is running.
func (a *App) SetValuationSources(vs ValuationSource, es EPSSource) {
	a.valuationSrc = vs
	a.epsSrc = es
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

	// Resolve which bound strategies actually apply to this asset type. A
	// non-empty binding that filters down to nothing means the item is
	// intentionally excluded from analysis (design §3.3), so skip the fetch.
	var effective []string
	if len(item.Strategies) > 0 {
		effective = a.effectiveStrategies(item)
		if len(effective) == 0 {
			return
		}
	}

	collectors := a.orderedCollectors(symbol)
	if len(collectors) == 0 {
		a.logger.Warn("no collectors available", zap.String("symbol", symbol))
		return
	}

	// Fetch enough history for the most demanding bound strategy (trading days
	// converted to calendar days), preferring the collector that matches the
	// symbol's market and falling back to others.
	end := time.Now()
	start := end.AddDate(0, 0, -a.historyWindowDays(item))

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
		Market: collector.MarketForSymbol(symbol),
		OHLCV:  ohlcv,
		Now:    time.Now(),
	}
	// Assemble the PE-percentile fundamental only when a bound strategy needs it.
	if a.needsFundamentals(effective) {
		analysisCtx.Fundamental = a.buildFundamental(symbol, item.Type, ohlcv)
	}

	// Honour per-symbol strategy selection when configured; otherwise run all.
	// effective is the asset-type-filtered binding (non-empty here, since an
	// all-filtered binding returned early above).
	var signals []core.Signal
	var err error
	if len(effective) > 0 {
		signals, err = a.strategies.AnalyzeWithStrategies(ctx, analysisCtx, effective)
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

	// Carry a reference price onto the synthesized decision. Without it the
	// downstream executor receives Price=0, which ExecutionManager rejects
	// ("price must be positive") so the arbitrated decision would never trade
	// (QA W1 / CARRYOVER I3; mirrors the 784ed71 ma_crossover fix).
	return []core.Signal{{
		Symbol:      symbol,
		Action:      result.Decision,
		Confidence:  result.Confidence,
		Price:       referencePrice(signals),
		Reason:      result.Reasoning,
		Strategy:    "meta_arbitrator",
		GeneratedAt: time.Now(),
	}}
}

// referencePrice returns the price to stamp on a synthesized arbitration signal:
// the first positive price among the conflicting inputs (all priced at the same
// cycle's latest close). Returns 0 only when no input carries a price, in which
// case the executor's positive-price guard still suppresses an unpriced order.
func referencePrice(signals []core.Signal) float64 {
	for _, s := range signals {
		if s.Price > 0 {
			return s.Price
		}
	}
	return 0
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
	case upperSymbol == "^HSI":
		// Hang Seng index: report H股 so the UI market label matches the HK
		// routing in collector.MarketForSymbol (avoids 美股/H股 inconsistency).
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
	switch {
	case strings.HasPrefix(upperSymbol, "^"), collector.IsAShareIndex(symbol):
		return TypeIndex
	case strings.HasSuffix(upperSymbol, "=F"):
		return TypeFuture
	case strings.Contains(upperSymbol, "-USD") || strings.Contains(upperSymbol, "-USDT") ||
		strings.HasPrefix(upperSymbol, "BTC") || strings.HasPrefix(upperSymbol, "ETH"):
		return TypeCrypto
	default:
		return TypeStock
	}
}

// assetTypeOf maps the Chinese UI type label to the core asset type used by
// strategy AssetTypes declarations. Empty means unsupported in phase 1.
func assetTypeOf(appType string) core.AssetType {
	switch appType {
	case TypeStock:
		return core.AssetStock
	case TypeIndex:
		return core.AssetIndex
	case TypeETF:
		return core.AssetETF
	case TypeFund:
		return core.AssetFund
	case TypeFuture:
		return core.AssetCommodity
	case TypeCrypto:
		return core.AssetCrypto
	default:
		return ""
	}
}

// warnOnce logs a warning the first time it sees a given key, deduping repeats
// across the concurrent analysis loop.
func (a *App) warnOnce(key, msg string, fields ...zap.Field) {
	if _, loaded := a.warned.LoadOrStore(key, struct{}{}); loaded {
		return
	}
	a.logger.Warn(msg, fields...)
}

// effectiveStrategies returns the item's bound strategies minus those whose
// AssetTypes declaration excludes the item's asset type. Mismatches are logged
// once per (symbol,strategy) pair (design §3.3). Unregistered names are passed
// through so the engine surfaces its own error.
func (a *App) effectiveStrategies(item WatchlistItem) []string {
	if strings.HasPrefix(item.Symbol, "^") {
		if _, known := collector.KnownIndexMarket(item.Symbol); !known {
			a.warnOnce("unknown-index:"+item.Symbol,
				"index symbol outside phase-1 list, market defaults to US",
				zap.String("symbol", item.Symbol))
		}
	}

	at := assetTypeOf(item.Type)
	out := make([]string, 0, len(item.Strategies))
	for _, name := range item.Strategies {
		s, ok := a.strategies.Get(name)
		if !ok {
			out = append(out, name) // 未注册的留给 engine 报错路径
			continue
		}
		decl := s.RequiredData().AssetTypes
		if len(decl) == 0 || (at != "" && slices.Contains(decl, at)) {
			out = append(out, name)
			continue
		}
		a.warnOnce("binding:"+item.Symbol+":"+name,
			"strategy skipped: asset type not supported",
			zap.String("symbol", item.Symbol), zap.String("strategy", name),
			zap.String("asset_type", item.Type))
	}
	return out
}

// historyWindowDays converts the max PriceHistory (trading days) demanded by the
// item's strategies into calendar days, with the legacy 365-day floor.
func (a *App) historyWindowDays(item WatchlistItem) int {
	maxBars := 0
	for _, name := range item.Strategies {
		if s, ok := a.strategies.Get(name); ok {
			if ph := s.RequiredData().PriceHistory; ph > maxBars {
				maxBars = ph
			}
		}
	}
	if maxBars <= 250 {
		return 365
	}
	// 交易日→自然日：真实折算系数 365/252 ≈ 1.448（×7/5 只折周末、漏节假日，
	// 对 5×252=1260 bars 会算出 1794 < 1825，不满足 5 年覆盖）。
	return maxBars*365/252 + 30
}

// valuationLookbackYears is the phase-1 fixed PE-percentile lookback window,
// matching the strategy default; later phases may push it down to a parameter.
const valuationLookbackYears = 5

// buildFundamental assembles Fundamental.PEPercentile for one watchlist item
// when any bound strategy needs fundamentals. Returns nil when the item's asset
// class has no valuation path (commodity/crypto/fund). Path table (design §3.2):
//
//	CN stock/index -> lixinger cvpos (lixinger IS the source, no fallback chain)
//	US/HK index    -> lixinger cvpos (sole path)
//	US/HK stock    -> yahoo EPS reconstruction; on missing-data errors fall back
//	                  to lixinger; on ErrNonPositiveEPS skip entirely (a real
//	                  loss — a fallback percentile would mislead).
func (a *App) buildFundamental(symbol, appType string, ohlcv []core.OHLCV) *core.Fundamental {
	at := assetTypeOf(appType)
	if at != core.AssetStock && at != core.AssetIndex {
		return nil
	}
	market := collector.MarketForSymbol(symbol)
	f := &core.Fundamental{Symbol: symbol, Market: market, Date: time.Now(), PEPercentile: -1}

	switch {
	case market == core.MarketCNA, at == core.AssetIndex:
		// A 股（股票+指数）与美/港指数：理杏仁唯一路径。
		if a.valuationSrc == nil {
			a.warnOnce("lixinger:"+symbol, "valuation percentile unavailable: lixinger not configured",
				zap.String("symbol", symbol))
			return f
		}
		pct, err := a.valuationSrc.FetchValuationPercentile(symbol, valuationLookbackYears)
		if err != nil {
			a.warnOnce("lixinger:"+symbol, "valuation percentile fetch failed",
				zap.String("symbol", symbol), zap.Error(err))
			return f
		}
		f.PEPercentile, f.Source = pct, "lixinger_cvpos"
		return f

	default:
		// 美/港个股：Yahoo EPS 重建主路径。epsSrc 未配置（yahoo 未启用）也算
		// "主路径不可用·数据缺失"，直接进理杏仁兜底（设计 §5）。
		if a.epsSrc == nil {
			if a.valuationSrc != nil {
				if pct, ferr := a.valuationSrc.FetchValuationPercentile(symbol, valuationLookbackYears); ferr == nil {
					f.PEPercentile, f.Source = pct, "lixinger_cvpos:yahoo_not_configured"
					return f
				}
			}
			a.warnOnce("pepct:"+symbol, "pe percentile unavailable: no eps source and no fallback",
				zap.String("symbol", symbol))
			return f
		}
		end := time.Now()
		eps, err := a.epsSrc.FetchEPSHistory(symbol, end.AddDate(-valuationLookbackYears, 0, -90), end)
		if err == nil {
			pct, rerr := valuation.ReconstructPEPercentile(ohlcv, eps)
			switch {
			case rerr == nil:
				f.PEPercentile, f.Source = pct, "reconstructed"
				return f
			case errors.Is(rerr, valuation.ErrNonPositiveEPS):
				return f // 真实亏损：不可用，绝不兜底（设计 §3.2/§5）
			}
			err = rerr // ErrInsufficientEPS（或其它数据缺失）→ 落入兜底
		}
		if a.valuationSrc != nil {
			if pct, ferr := a.valuationSrc.FetchValuationPercentile(symbol, valuationLookbackYears); ferr == nil {
				f.PEPercentile = pct
				f.Source = "lixinger_cvpos:" + fallbackReason(err)
				return f
			}
		}
		a.warnOnce("pepct:"+symbol, "pe percentile unavailable (primary and fallback failed)",
			zap.String("symbol", symbol), zap.Error(err))
		return f
	}
}

// fallbackReason classifies why the yahoo reconstruction path yielded to the
// lixinger fallback, encoded into Fundamental.Source for observability.
func fallbackReason(err error) string {
	if errors.Is(err, valuation.ErrInsufficientEPS) {
		return "yahoo_eps_insufficient"
	}
	return "yahoo_eps_error"
}

// needsFundamentals reports whether any of the named (registered) strategies
// declares a fundamental-data requirement.
func (a *App) needsFundamentals(names []string) bool {
	for _, n := range names {
		if s, ok := a.strategies.Get(n); ok && s.RequiredData().Fundamentals {
			return true
		}
	}
	return false
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
