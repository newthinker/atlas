package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

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

	log.Info("starting ATLAS server",
		zap.String("host", cfg.Server.Host),
		zap.Int("port", cfg.Server.Port),
	)

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down ATLAS server")
	return nil
}
