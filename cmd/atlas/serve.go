package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/newthinker/atlas/internal/api"
	"github.com/newthinker/atlas/internal/config"
	"github.com/newthinker/atlas/internal/logger"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
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

	// Create API server
	server, err := api.NewServer(api.Config{
		Host:         cfg.Server.Host,
		Port:         cfg.Server.Port,
		TemplatesDir: "internal/api/templates",
	}, log)
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
