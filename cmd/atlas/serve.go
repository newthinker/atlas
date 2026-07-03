package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/newthinker/atlas/internal/api"
	"github.com/newthinker/atlas/internal/app"
	"github.com/newthinker/atlas/internal/backtest"
	"github.com/newthinker/atlas/internal/collector"
	"github.com/newthinker/atlas/internal/collector/lixinger"
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

	// Create signal store per config (sqlite by default, persistent). A sqlite
	// open failure exits rather than silently degrading to memory.
	sigStore, closeSignalStore, err := buildSignalStore(cfg, log)
	if err != nil {
		return fmt.Errorf("creating signal store: %w", err)
	}
	defer closeSignalStore()

	// Create App
	application := app.New(cfg, log)

	// Persist generated signals so the API and web UI can display them.
	application.SetSignalStore(sigStore)

	// Register collectors + valuation/EPS/fundamental sources (shared with
	// `atlas watchlist`).
	cleanupCollectors, err := buildCollectors(cfg, application, log)
	if err != nil {
		return fmt.Errorf("wiring collectors: %w", err)
	}
	defer cleanupCollectors()

	// Wire configured notifiers (telegram/email/webhook) so routed signals are
	// actually delivered. Done after collectors are registered and before the
	// server starts. Misconfigured entries warn and are skipped, never blocking
	// startup (matches collector/strategy wiring above). The returned notifiers
	// are reused as alert sinks when the alert loop is enabled below.
	notifiers := registerConfiguredNotifiers(cfg, application, log)

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

	// Start the alert evaluation loop when enabled. It periodically snapshots
	// metrics, derives http_error_rate / signals_24h, and evaluates the
	// configured rules, delivering to the same notifiers via alert adapters.
	// The loop stops when appCtx is cancelled during shutdown. When alerts are
	// disabled this is a no-op (no goroutine, no evaluator).
	appCtx, appCancel := context.WithCancel(context.Background())
	defer appCancel()
	maybeStartAlertRunner(appCtx, cfg, notifiers, metricsReg, sigStore, log)

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

// buildSignalStore constructs the signal store from config. backend=sqlite
// (the default) opens a persistent SQLiteStore at the configured path and
// returns a cleanup that closes it; backend=memory returns an in-memory store
// with a no-op cleanup. A sqlite open failure is returned as an error so serve
// exits rather than silently falling back to memory — dropping signals the
// operator explicitly asked to persist would be worse than failing fast.
func buildSignalStore(cfg *config.Config, log *zap.Logger) (signalstore.Store, func(), error) {
	switch cfg.Storage.Signals.Backend {
	case "memory":
		log.Info("signal store: in-memory (non-persistent)")
		return signalstore.NewMemoryStore(defaultSignalStoreCapacity), func() {}, nil
	case "sqlite", "":
		path := cfg.Storage.Signals.Path
		if path == "" {
			path = "data/signals.db"
		}
		store, err := signalstore.NewSQLiteStore(path)
		if err != nil {
			return nil, nil, fmt.Errorf("opening sqlite signal store at %s: %w", path, err)
		}
		log.Info("signal store: sqlite (persistent)", zap.String("path", path))
		return store, func() { _ = store.Close() }, nil
	default:
		return nil, nil, fmt.Errorf("invalid storage.signals.backend: %s (want memory or sqlite)", cfg.Storage.Signals.Backend)
	}
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
// in cfg.Notifiers into the app's notifier registry and returns the notifiers
// successfully registered (so the alert loop can wrap the same instances as
// alert sinks). Misconfigured entries (missing required fields,
// unknown type, or a registry rejection such as a duplicate name) are logged at
// warn level and skipped — they never block startup, matching the
// collector/strategy wiring. If any notifier was enabled yet none registered, a
// warn is emitted because routed signals would otherwise be dropped silently.
func registerConfiguredNotifiers(cfg *config.Config, application *app.App, log *zap.Logger) []notifier.Notifier {
	registered := make([]notifier.Notifier, 0)
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
			n = telegram.New(nc.BotToken, nc.ChatID, telegram.WithProxy(nc.Proxy))
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
		registered = append(registered, n)
		log.Info("registered notifier", zap.String("notifier", key))
	}

	log.Info("configured notifiers registered", zap.Int("count", len(registered)))
	if enabled > 0 && len(registered) == 0 {
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
