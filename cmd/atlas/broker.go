package main

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/newthinker/atlas/internal/broker"
	"github.com/newthinker/atlas/internal/broker/mock"
	"github.com/newthinker/atlas/internal/config"
	"github.com/newthinker/atlas/internal/logger"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var brokerCmd = &cobra.Command{
	Use:   "broker",
	Short: "Broker operations",
	Long:  `Commands for interacting with the broker (positions, orders, account info).`,
}

var brokerStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check broker connection status",
	RunE:  runBrokerStatus,
}

var brokerPositionsCmd = &cobra.Command{
	Use:   "positions",
	Short: "List current positions",
	RunE:  runBrokerPositions,
}

var brokerOrdersCmd = &cobra.Command{
	Use:   "orders",
	Short: "List recent orders",
	RunE:  runBrokerOrders,
}

var brokerAccountCmd = &cobra.Command{
	Use:   "account",
	Short: "Show account information",
	RunE:  runBrokerAccount,
}

var brokerHistoryCmd = &cobra.Command{
	Use:   "history",
	Short: "Show trade history",
	RunE:  runBrokerHistory,
}

var (
	historyFrom string
	historyTo   string
)

func init() {
	rootCmd.AddCommand(brokerCmd)
	brokerCmd.AddCommand(brokerStatusCmd)
	brokerCmd.AddCommand(brokerPositionsCmd)
	brokerCmd.AddCommand(brokerOrdersCmd)
	brokerCmd.AddCommand(brokerAccountCmd)
	brokerCmd.AddCommand(brokerHistoryCmd)

	brokerHistoryCmd.Flags().StringVar(&historyFrom, "from", "", "Start date (YYYY-MM-DD)")
	brokerHistoryCmd.Flags().StringVar(&historyTo, "to", "", "End date (YYYY-MM-DD)")
}

// withBrokerConnection handles common broker setup and teardown.
func withBrokerConnection(fn func(b broker.LegacyBroker, log *zap.Logger) error) error {
	log := logger.Must(debug)
	defer log.Sync()

	var cfg *config.Config
	if cfgFile != "" {
		var err error
		cfg, err = config.Load(cfgFile)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
	}

	b, err := getBroker(cfg)
	if err != nil {
		return err
	}

	ctx := context.Background()
	if err := b.Connect(ctx); err != nil {
		return fmt.Errorf("connecting to broker: %w", err)
	}
	defer b.Disconnect()

	return fn(b, log)
}

func getBroker(cfg *config.Config) (broker.LegacyBroker, error) {
	if cfg == nil || !cfg.Broker.Enabled {
		// Use mock broker for demo/testing
		return mock.New(), nil
	}

	switch cfg.Broker.Provider {
	case "mock":
		return mock.New(), nil
	case "futu":
		// TODO: Implement Futu broker when OpenD SDK is integrated
		return nil, fmt.Errorf("futu broker not yet implemented, use mock for now")
	default:
		return nil, fmt.Errorf("unknown broker provider: %s", cfg.Broker.Provider)
	}
}

func runBrokerStatus(cmd *cobra.Command, args []string) error {
	return withBrokerConnection(func(b broker.LegacyBroker, log *zap.Logger) error {
		fmt.Printf("Broker: %s\n", b.Name())
		fmt.Printf("Status: CONNECTED\n")
		fmt.Printf("Supported Markets: %v\n", b.SupportedMarkets())
		log.Info("broker status checked", zap.String("broker", b.Name()), zap.Bool("connected", b.IsConnected()))
		return nil
	})
}

func runBrokerPositions(cmd *cobra.Command, args []string) error {
	return withBrokerConnection(func(b broker.LegacyBroker, log *zap.Logger) error {
		ctx := context.Background()
		positions, err := b.GetPositions(ctx)
		if err != nil {
			return fmt.Errorf("getting positions: %w", err)
		}

		if len(positions) == 0 {
			fmt.Println("No positions found.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "SYMBOL\tMARKET\tQTY\tAVG COST\tMKT VALUE\tP&L\t")
		fmt.Fprintln(w, "------\t------\t---\t--------\t---------\t---\t")

		for _, p := range positions {
			plSign := ""
			if p.UnrealizedPL >= 0 {
				plSign = "+"
			}
			fmt.Fprintf(w, "%s\t%s\t%d\t%.2f\t%.2f\t%s%.2f\t\n",
				p.Symbol, p.Market, p.Quantity, p.AvgCost, p.MarketValue, plSign, p.UnrealizedPL)
		}
		w.Flush()

		log.Info("positions listed", zap.Int("count", len(positions)))
		return nil
	})
}

func runBrokerOrders(cmd *cobra.Command, args []string) error {
	return withBrokerConnection(func(b broker.LegacyBroker, log *zap.Logger) error {
		ctx := context.Background()
		orders, err := b.GetOrders(ctx, broker.OrderFilter{})
		if err != nil {
			return fmt.Errorf("getting orders: %w", err)
		}

		if len(orders) == 0 {
			fmt.Println("No orders found.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ORDER ID\tSYMBOL\tSIDE\tTYPE\tQTY\tPRICE\tSTATUS\tCREATED\t")
		fmt.Fprintln(w, "--------\t------\t----\t----\t---\t-----\t------\t-------\t")

		for _, o := range orders {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%.2f\t%s\t%s\t\n",
				o.OrderID, o.Symbol, o.Side, o.Type, o.Quantity, o.Price, o.Status,
				o.CreatedAt.Format("2006-01-02 15:04"))
		}
		w.Flush()

		log.Info("orders listed", zap.Int("count", len(orders)))
		return nil
	})
}

func runBrokerAccount(cmd *cobra.Command, args []string) error {
	return withBrokerConnection(func(b broker.LegacyBroker, log *zap.Logger) error {
		ctx := context.Background()
		info, err := b.GetAccountInfo(ctx)
		if err != nil {
			return fmt.Errorf("getting account info: %w", err)
		}

		fmt.Println("Account Summary")
		fmt.Println("---------------")
		fmt.Printf("Total Assets:    $%.2f\n", info.TotalAssets)
		fmt.Printf("Cash:            $%.2f\n", info.Cash)
		fmt.Printf("Buying Power:    $%.2f\n", info.BuyingPower)
		fmt.Printf("Margin Used:     $%.2f\n", info.MarginUsed)
		fmt.Printf("Day Trades Left: %d\n", info.DayTradesLeft)

		log.Info("account info displayed")
		return nil
	})
}

func runBrokerHistory(cmd *cobra.Command, args []string) error {
	return withBrokerConnection(func(b broker.LegacyBroker, log *zap.Logger) error {
		ctx := context.Background()

		// Parse date range
		var start, end time.Time
		var err error
		if historyFrom != "" {
			start, err = time.Parse("2006-01-02", historyFrom)
			if err != nil {
				return fmt.Errorf("invalid from date: %w", err)
			}
		} else {
			start = time.Now().AddDate(0, -1, 0) // Default: 1 month ago
		}

		if historyTo != "" {
			end, err = time.Parse("2006-01-02", historyTo)
			if err != nil {
				return fmt.Errorf("invalid to date: %w", err)
			}
		} else {
			end = time.Now()
		}

		trades, err := b.GetTradeHistory(ctx, start, end)
		if err != nil {
			return fmt.Errorf("getting trade history: %w", err)
		}

		if len(trades) == 0 {
			fmt.Printf("No trades found between %s and %s.\n",
				start.Format("2006-01-02"), end.Format("2006-01-02"))
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "TRADE ID\tSYMBOL\tSIDE\tQTY\tPRICE\tFEE\tTIME\t")
		fmt.Fprintln(w, "--------\t------\t----\t---\t-----\t---\t----\t")

		for _, t := range trades {
			fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%.2f\t%.2f\t%s\t\n",
				t.TradeID, t.Symbol, t.Side, t.Quantity, t.Price, t.Fee,
				t.Timestamp.Format("2006-01-02 15:04"))
		}
		w.Flush()

		log.Info("trade history listed", zap.Int("count", len(trades)))
		return nil
	})
}
