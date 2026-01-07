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
	"github.com/newthinker/atlas/internal/broker"
	"github.com/newthinker/atlas/internal/collector/yahoo"
	"github.com/newthinker/atlas/internal/config"
	"github.com/newthinker/atlas/internal/logger"
	"github.com/newthinker/atlas/internal/metrics"
	signalstore "github.com/newthinker/atlas/internal/storage/signal"
	"github.com/newthinker/atlas/internal/strategy"
	"github.com/newthinker/atlas/internal/strategy/ma_crossover"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
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

	// Register collectors if configured
	if collectorCfg, ok := cfg.Collectors["yahoo"]; ok && collectorCfg.Enabled {
		yahooCollector := yahoo.New()
		application.RegisterCollector(yahooCollector)
	}

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

	// Set watchlist from config with full details (name, market, type, strategies)
	for _, item := range cfg.Watchlist {
		application.AddToWatchlistWithDetails(item.Symbol, item.Name, item.Market, item.Type, item.Strategies)
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

	// Create execution manager if broker is enabled
	var execManager *broker.ExecutionManager
	if cfg.Broker.Enabled {
		// TODO: Create actual broker instance when FUTU integration is ready
		// For now, log that broker is enabled but not yet implemented
		log.Warn("broker enabled but not yet fully implemented",
			zap.String("provider", cfg.Broker.Provider),
			zap.String("mode", cfg.Broker.Mode),
			zap.String("execution_mode", cfg.Broker.Execution.Mode),
		)
		// Once a proper Broker implementation exists:
		// brokerInstance := futu.New(cfg.Broker.Futu)
		// if err := brokerInstance.Connect(context.Background()); err != nil {
		//     return fmt.Errorf("connecting to broker: %w", err)
		// }
		// defer brokerInstance.Disconnect()
		//
		// riskChecker := broker.NewRiskChecker(broker.RiskConfig{...}, brokerInstance)
		// posTracker := broker.NewPositionTracker(brokerInstance)
		// execManager = broker.NewExecutionManager(broker.ExecutionConfig{...}, brokerInstance, riskChecker, posTracker)
	}

	// Create server dependencies
	deps := api.Dependencies{
		App:              application,
		SignalStore:      sigStore,
		Backtester:       backtester,
		Strategies:       strategies,
		Metrics:          metricsReg,
		ExecutionManager: execManager,
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
