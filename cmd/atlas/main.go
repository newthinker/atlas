package main

import (
	"os"

	"github.com/spf13/cobra"
)

var (
	cfgFile string
	debug   bool
)

var rootCmd = &cobra.Command{
	Use:   "atlas",
	Short: "ATLAS - Asset Tracking & Leadership Analysis System",
	Long: `ATLAS is a global asset monitoring system with automated trading signals.
It supports multiple markets (US, HK, CN_A) and asset types.`,
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file path")
	rootCmd.PersistentFlags().BoolVarP(&debug, "debug", "d", false, "enable debug mode")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
