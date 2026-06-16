package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/newthinker/atlas/internal/api"
	"github.com/newthinker/atlas/internal/app"
	"github.com/newthinker/atlas/internal/backtest"
	"github.com/newthinker/atlas/internal/collector"
	"github.com/newthinker/atlas/internal/collector/crypto"
	"github.com/newthinker/atlas/internal/collector/eastmoney"
	"github.com/newthinker/atlas/internal/collector/lixinger"
	"github.com/newthinker/atlas/internal/collector/qlibpit"
	"github.com/newthinker/atlas/internal/collector/yahoo"
	"github.com/newthinker/atlas/internal/config"
	atlasctx "github.com/newthinker/atlas/internal/context"
	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/llm/factory"
	"github.com/newthinker/atlas/internal/logger"
	"github.com/newthinker/atlas/internal/meta"
	"github.com/newthinker/atlas/internal/metrics"
	"github.com/newthinker/atlas/internal/notifier"
	"github.com/newthinker/atlas/internal/notifier/email"
	"github.com/newthinker/atlas/internal/notifier/telegram"
	"github.com/newthinker/atlas/internal/notifier/webhook"
	signalstore "github.com/newthinker/atlas/internal/storage/signal"
	"github.com/newthinker/atlas/internal/strategy"
	"github.com/newthinker/atlas/internal/strategy/ma_crossover"
	"github.com/newthinker/atlas/internal/strategy/pe_percentile"
	"github.com/newthinker/atlas/internal/strategy/price_percentile"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	_ "modernc.org/sqlite"
)

const (
	defaultSignalStoreCapacity = 1000
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the ATLAS server",
	RunE:  runServe,
}

func init() {
	rootCmd.AddCommand(serveCmd)
}

func runServe(cmd *cobra.Command, args []string) error {
	// Initialize logger
	log := logger.Must(debug)
	defer log.Sync()

	// Load config
	var cfg *config.Config
	var err error

	if cfgFile != "" {
		cfg, err = config.Load(cfgFile)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
	} else {
		cfg = config.Defaults()
		log.Warn("no config file specified, using defaults")
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	log.Info("starting ATLAS server",
		zap.String("host", cfg.Server.Host),
		zap.Int("port", cfg.Server.Port),
	)

	// Create signal store
	sigStore := signalstore.NewMemoryStore(defaultSignalStoreCapacity)

	// Create App
	application := app.New(cfg, log)

	// Persist generated signals so the API and web UI can display them.
	application.SetSignalStore(sigStore)

	// OHLCV cache settings applied when registering collectors below.
	cacheEnabled := cfg.Collector.Cache.Enabled
	cacheTTL := cfg.Collector.Cache.TTL
	if cacheEnabled {
		log.Info("OHLCV collector cache enabled", zap.Duration("ttl", cacheTTL))
	}

	// Register collectors if configured. yahooCollector is declared at function
	// scope so it can later be injected as the app's EPS source for PE-percentile
	// reconstruction (it stays nil when yahoo is not configured).
	var yahooCollector *yahoo.Yahoo
	if collectorCfg, ok := cfg.Collectors["yahoo"]; ok && collectorCfg.Enabled {
		yahooCollector = yahoo.New()
		application.RegisterCollector(maybeCache(yahooCollector, cacheEnabled, cacheTTL))
	}

	// Create Lixinger collector if configured (used as fallback for Eastmoney).
	// retry 开关默认开启（遵循 SKILL.md 退避策略），可在配置中关闭。
	var lixingerCollector *lixinger.Lixinger
	if collectorCfg, ok := cfg.Collectors["lixinger"]; ok && collectorCfg.Enabled && collectorCfg.APIKey != "" {
		retry := true
		if v, ok := collectorCfg.Extra["retry"].(bool); ok {
			retry = v
		}
		lixingerCollector = lixinger.New(collectorCfg.APIKey, lixinger.WithRetry(retry))
		log.Info("lixinger collector initialized as fallback for eastmoney")
	}

	// Register Eastmoney collector for A-shares
	if collectorCfg, ok := cfg.Collectors["eastmoney"]; ok && collectorCfg.Enabled {
		eastmoneyCollector := eastmoney.New()
		// Set Lixinger as fallback if available
		if lixingerCollector != nil {
			eastmoneyCollector.SetLixingerFallback(lixingerCollector)
			log.Info("lixinger fallback configured for eastmoney collector")
		}
		application.RegisterCollector(maybeCache(eastmoneyCollector, cacheEnabled, cacheTTL))
	}

	// Register Crypto collector for digital assets
	if collectorCfg, ok := cfg.Collectors["crypto"]; ok && collectorCfg.Enabled {
		cryptoCollector := crypto.New()
		// Configure from config if available
		if collectorCfg.Extra != nil {
			cryptoCollector.Init(collector.Config{
				Enabled: true,
				Extra:   collectorCfg.Extra,
			})
		}
		application.RegisterCollector(maybeCache(cryptoCollector, cacheEnabled, cacheTTL))
		log.Info("crypto collector registered")
	}

	// Wire qlib warehouse collector after all external collectors are registered
	// so that SelectExternalForSymbol can resolve to them at runtime.
	// The returned db handle is shared with qlibpit below (single open, no double-open).
	qlibWarehouseDB, _ := wireQlibWarehouse(cfg.Qlib, application.CollectorRegistry(), func(p string) (*sql.DB, error) {
		return sql.Open("sqlite", "file:"+p+"?mode=ro")
	}, log)

	// Inject valuation/EPS sources used to assemble PE-percentile fundamentals.
	// Pass through the typed-nil guards so an unconfigured collector stays an
	// untyped-nil interface (see valuationSourceOrNil).
	// When the qlib warehouse is available, wrap the yahoo EPS source with the
	// PIT-correct qlibpit source (yahoo becomes the fallback, no recursive wrapping).
	var epsSrc app.EPSSource = epsSourceOrNil(yahooCollector)
	if qlibWarehouseDB != nil {
		epsSrc = qlibpit.New(qlibWarehouseDB, epsSrc)
		log.Info("qlib PIT EPS source enabled (yahoo fallback)")
	}
	application.SetValuationSources(valuationSourceOrNil(lixingerCollector), epsSrc)

	// Wire configured notifiers (telegram/email/webhook) so routed signals are
	// actually delivered. Done after collectors are registered and before the
	// server starts. Misconfigured entries warn and are skipped, never blocking
	// startup (matches collector/strategy wiring above).
	registerConfiguredNotifiers(cfg, application, log)

	// Create strategy engine and register strategies
	strategies := strategy.NewEngine()
	if strategyCfg, ok := cfg.Strategies["ma_crossover"]; ok && strategyCfg.Enabled {
		// Get params with defaults
		fastPeriod := 50
		slowPeriod := 200
		if fast, ok := strategyCfg.Params["fast_period"].(int); ok {
			fastPeriod = fast
		}
		if slow, ok := strategyCfg.Params["slow_period"].(int); ok {
			slowPeriod = slow
		}
		maStrategy := ma_crossover.New(fastPeriod, slowPeriod)
		if err := maStrategy.Init(strategy.Config{Params: strategyCfg.Params}); err != nil {
			log.Warn("failed to init ma_crossover strategy", zap.Error(err))
		} else {
			strategies.Register(maStrategy)
			application.RegisterStrategy(maStrategy)
		}
	}

	// Percentile strategies: position of price / PE within their own multi-year
	// history. Both read enabled + params from config like ma_crossover above.
	if strategyCfg, ok := cfg.Strategies["price_percentile"]; ok && strategyCfg.Enabled {
		registerConfiguredStrategy(strategies, application, price_percentile.New(), strategy.Config{Params: strategyCfg.Params}, log)
	}
	if strategyCfg, ok := cfg.Strategies["pe_percentile"]; ok && strategyCfg.Enabled {
		registerConfiguredStrategy(strategies, application, pe_percentile.New(), strategy.Config{Params: strategyCfg.Params}, log)
	}

	// Set watchlist from config with full details (name, market, type, strategies)
	for _, item := range cfg.Watchlist {
		application.AddToWatchlistWithDetails(item.Symbol, item.Name, item.Market, item.Type, item.Strategies)
	}

	// Wire the LLM signal arbitrator when enabled.
	if cfg.Meta.Arbitrator.Enabled && cfg.LLM.Provider != "" {
		if arb, err := buildArbitrator(cfg, application.GetCollectors(), log); err != nil {
			log.Warn("failed to enable signal arbitrator", zap.Error(err))
		} else {
			application.SetArbitrator(arb)
			log.Info("LLM signal arbitrator enabled",
				zap.String("provider", cfg.LLM.Provider),
				zap.Int("context_days", cfg.Meta.Arbitrator.ContextDays),
			)
		}
	}

	// Create backtester with first available collector
	var backtester *backtest.Backtester
	collectors := application.GetCollectors()
	if len(collectors) > 0 {
		backtester = backtest.New(collectors[0])
	} else {
		// Create a default yahoo collector for backtesting
		backtester = backtest.New(yahoo.New())
	}

	// Create metrics registry if enabled
	var metricsReg *metrics.Registry
	if cfg.Metrics.Enabled {
		metricsReg = metrics.NewRegistry()
		log.Info("metrics enabled", zap.String("path", cfg.Metrics.Path))
	}

	// Wire the paper-mode execution chain when the broker is enabled. In paper
	// mode this builds PaperBroker → RiskChecker → PositionTracker →
	// ExecutionManager, injects a signal adapter into the app, and exposes the
	// manager through the API dependencies. When disabled or configured for a
	// non-paper mode, execManager is nil and the process starts normally.
	execManager, err := wireExecution(context.Background(), cfg, application, log)
	if err != nil {
		return fmt.Errorf("wiring execution: %w", err)
	}

	// Create server dependencies
	deps := api.Dependencies{
		App:              application,
		SignalStore:      sigStore,
		Backtester:       backtester,
		Strategies:       strategies,
		Metrics:          metricsReg,
		ExecutionManager: execManager,
		Config:           cfg,
	}

	// Create server config
	serverCfg := api.Config{
		Host:         cfg.Server.Host,
		Port:         cfg.Server.Port,
		TemplatesDir: "internal/api/templates",
		APIKey:       cfg.Server.APIKey,
		JobTTLHours:  cfg.Server.JobTTLHours,
		MaxJobs:      cfg.Server.MaxJobs,
	}

	// Create API server
	server, err := api.NewServer(serverCfg, deps, log)
	if err != nil {
		return fmt.Errorf("creating server: %w", err)
	}

	// Start server in goroutine
	go func() {
		if err := server.Start(); err != nil {
			log.Error("server error", zap.Error(err))
		}
	}()

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down ATLAS server")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return server.Shutdown(ctx)
}

// buildArbitrator constructs an LLM-backed signal arbitrator from the configured
// LLM provider and the available collectors (used for market context).
func buildArbitrator(cfg *config.Config, collectors []collector.Collector, log *zap.Logger) (*meta.Arbitrator, error) {
	llmProvider, err := factory.New(cfg.LLM)
	if err != nil {
		return nil, fmt.Errorf("creating llm provider: %w", err)
	}

	// Map collectors by the markets they support for market-context lookups.
	marketCollectors := make(map[core.Market]collector.Collector)
	for _, c := range collectors {
		for _, m := range c.SupportedMarkets() {
			if _, ok := marketCollectors[m]; !ok {
				marketCollectors[m] = c
			}
		}
	}

	marketCtx := atlasctx.NewMarketContextService(marketCollectors)
	trackRecord := atlasctx.NewInMemoryTrackRecord()
	news := atlasctx.NewStaticNewsProvider(nil)

	return meta.NewArbitrator(llmProvider, marketCtx, trackRecord, news, log, meta.ArbitratorConfig{
		ContextDays: cfg.Meta.Arbitrator.ContextDays,
	}), nil
}

// registerConfiguredNotifiers wires the enabled telegram/email/webhook entries
// in cfg.Notifiers into the app's notifier registry and returns the number
// successfully registered. Misconfigured entries (missing required fields,
// unknown type, or a registry rejection such as a duplicate name) are logged at
// warn level and skipped — they never block startup, matching the
// collector/strategy wiring. If any notifier was enabled yet none registered, a
// warn is emitted because routed signals would otherwise be dropped silently.
func registerConfiguredNotifiers(cfg *config.Config, application *app.App, log *zap.Logger) int {
	registered := 0
	enabled := 0

	for key, nc := range cfg.Notifiers {
		if !nc.Enabled {
			continue
		}
		enabled++

		// requireField logs the standardized "missing required field" warning and
		// reports false when a required field is empty, letting each case bail out
		// of the loop iteration uniformly.
		requireField := func(name string, present bool) bool {
			if !present {
				log.Warn("notifier missing required field", zap.String("notifier", key), zap.String("field", name))
			}
			return present
		}

		var n notifier.Notifier
		switch key {
		case "telegram":
			if !requireField("bot_token", nc.BotToken != "") || !requireField("chat_id", nc.ChatID != "") {
				continue
			}
			n = telegram.New(nc.BotToken, nc.ChatID)
		case "email":
			if !requireField("host", nc.Host != "") || !requireField("from", nc.From != "") || !requireField("to", len(nc.To) != 0) {
				continue
			}
			n = email.New(nc.Host, nc.Port, nc.Username, nc.Password, nc.From, nc.To)
		case "webhook":
			if !requireField("url", nc.URL != "") {
				continue
			}
			n = webhook.New(nc.URL, nc.Headers)
		default:
			log.Warn("unknown notifier type", zap.String("notifier", key))
			continue
		}

		if err := application.RegisterNotifier(n); err != nil {
			log.Warn("failed to register notifier", zap.String("notifier", key), zap.Error(err))
			continue
		}
		registered++
		log.Info("registered notifier", zap.String("notifier", key))
	}

	log.Info("configured notifiers registered", zap.Int("count", registered))
	if enabled > 0 && registered == 0 {
		log.Warn("all configured notifiers failed to register; signals will not be delivered")
	}
	return registered
}

// registerConfiguredStrategy initialises s with cfg and, on success, registers
// it with both the signal engine and the app. Init failures are logged and the
// strategy is skipped, matching the ma_crossover wiring.
func registerConfiguredStrategy(engine *strategy.Engine, application *app.App, s strategy.Strategy, cfg strategy.Config, log *zap.Logger) {
	if err := s.Init(cfg); err != nil {
		log.Warn("failed to init strategy", zap.String("strategy", s.Name()), zap.Error(err))
		return
	}
	engine.Register(s)
	application.RegisterStrategy(s)
}

// valuationSourceOrNil returns a non-nil app.ValuationSource only when c is a
// live collector. Returning c directly when it is a nil *lixinger.Lixinger
// would yield a non-nil interface wrapping a nil pointer (the typed-nil trap),
// defeating buildFundamental's `valuationSrc != nil` guard and panicking on use.
func valuationSourceOrNil(c *lixinger.Lixinger) app.ValuationSource {
	if c == nil {
		return nil
	}
	return c
}

// epsSourceOrNil mirrors valuationSourceOrNil for the yahoo EPS source.
func epsSourceOrNil(c *yahoo.Yahoo) app.EPSSource {
	if c == nil {
		return nil
	}
	return c
}

// maybeCache wraps c in an OHLCV TTL CachedCollector when caching is enabled.
//
// Collectors exposing an extension interface consumed via type assertion (e.g.
// collector.FundamentalCollector, implemented by lixinger) are returned
// unwrapped: CachedCollector embeds only collector.Collector and would hide
// those methods, breaking the assertion path. Name and SupportedMarkets pass
// through the wrapper, so collector routing/selection is unaffected.
func maybeCache(c collector.Collector, enabled bool, ttl time.Duration) collector.Collector {
	if !enabled {
		return c
	}
	if _, ok := c.(collector.FundamentalCollector); ok {
		return c
	}
	return collector.NewCached(c, ttl)
}
