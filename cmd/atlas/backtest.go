package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

var (
	backtestSymbol string
	backtestFrom   string
	backtestTo     string
)

var backtestCmd = &cobra.Command{
	Use:   "backtest [strategy]",
	Short: "Run backtest on a strategy",
	Long:  "Run a strategy against historical data and show performance statistics",
	Args:  cobra.ExactArgs(1),
	RunE:  runBacktest,
}

func init() {
	backtestCmd.Flags().StringVar(&backtestSymbol, "symbol", "", "Symbol to backtest (required)")
	backtestCmd.Flags().StringVar(&backtestFrom, "from", "", "Start date YYYY-MM-DD (required)")
	backtestCmd.Flags().StringVar(&backtestTo, "to", "", "End date YYYY-MM-DD (required)")

	backtestCmd.MarkFlagRequired("symbol")
	backtestCmd.MarkFlagRequired("from")
	backtestCmd.MarkFlagRequired("to")

	rootCmd.AddCommand(backtestCmd)
}

func runBacktest(cmd *cobra.Command, args []string) error {
	strategy := args[0]

	// Parse from date
	fromDate, err := time.Parse("2006-01-02", backtestFrom)
	if err != nil {
		return fmt.Errorf("invalid from date format (expected YYYY-MM-DD): %w", err)
	}

	// Parse to date
	toDate, err := time.Parse("2006-01-02", backtestTo)
	if err != nil {
		return fmt.Errorf("invalid to date format (expected YYYY-MM-DD): %w", err)
	}

	// Validate date range
	if toDate.Before(fromDate) {
		return fmt.Errorf("end date must be after start date")
	}

	fmt.Println("=== ATLAS Backtest ===")
	fmt.Printf("Strategy: %s\n", strategy)
	fmt.Printf("Symbol:   %s\n", backtestSymbol)
	fmt.Printf("Period:   %s to %s\n", fromDate.Format("2006-01-02"), toDate.Format("2006-01-02"))
	fmt.Println()

	// TODO: Wire up backtest engine
	// - Load historical data for symbol and date range
	// - Initialize strategy by name
	// - Run backtest engine simulation
	// - Calculate and display performance statistics

	fmt.Println("Backtest engine not yet implemented")

	return nil
}
