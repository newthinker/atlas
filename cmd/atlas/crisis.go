package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/newthinker/atlas/internal/collector/fred"
	"github.com/newthinker/atlas/internal/collector/yahoo"
	"github.com/newthinker/atlas/internal/crisis"
)

var (
	crisisCfgPath     string
	backfillFrom      string
	backfillTo        string
	backfillCSV       string
	backfillIndicator string
	backfillScale     float64
)

var crisisCmd = &cobra.Command{
	Use:   "crisis",
	Short: "Macro crisis monitor (Cassandra)",
	Long: `Systemic-risk monitor: seven market-stress indicators, three-color
rules and a NORMAL/WATCH/BREWING/CRISIS state machine. Risk states only —
never trade signals (see docs/plans/atlas-macro-crisis-monitor-design.md).`,
}

var crisisBackfillCmd = &cobra.Command{
	Use:   "backfill",
	Short: "Backfill indicator history from FRED/Yahoo or a CSV snapshot",
	RunE:  runCrisisBackfill,
}

func init() {
	crisisCmd.PersistentFlags().StringVar(&crisisCfgPath, "crisis-config",
		"configs/crisis-monitor.yaml", "crisis monitor config path")
	crisisBackfillCmd.Flags().StringVar(&backfillFrom, "from", "", "start date YYYY-MM-DD (FRED/Yahoo backfill)")
	crisisBackfillCmd.Flags().StringVar(&backfillTo, "to", "", "end date YYYY-MM-DD (default today)")
	crisisBackfillCmd.Flags().StringVar(&backfillCSV, "csv", "", "CSV snapshot path (date,value)")
	crisisBackfillCmd.Flags().StringVar(&backfillIndicator, "indicator", "", "indicator for --csv import (e.g. hy_oas)")
	crisisBackfillCmd.Flags().Float64Var(&backfillScale, "scale", 1, "value multiplier for --csv (percent→bp: 100)")
	crisisCmd.AddCommand(crisisBackfillCmd)
	rootCmd.AddCommand(crisisCmd)
}

func openCrisisStore() (*crisis.Config, *crisis.Store, error) {
	ccfg, err := crisis.LoadConfig(crisisCfgPath)
	if err != nil {
		return nil, nil, err
	}
	st, err := crisis.NewStore(ccfg.Storage.Path)
	if err != nil {
		return nil, nil, err
	}
	return ccfg, st, nil
}

// resolveFREDKey：环境变量优先（launchd/CI 可临时覆盖），否则回退主配置
// collectors.fred.api_key —— configs/config.yaml 在 .gitignore 中，与
// telegram/lixinger 凭据同层，密钥不入库。回退路径依赖根命令的 -c/--config。
func resolveFREDKey(envName string) string {
	if k := os.Getenv(envName); k != "" {
		return k
	}
	cfg, err := loadConfigOrDefaults()
	if err != nil {
		return ""
	}
	if fc, ok := cfg.Collectors["fred"]; ok {
		return fc.APIKey
	}
	return ""
}

func runCrisisBackfill(cmd *cobra.Command, args []string) error {
	ccfg, st, err := openCrisisStore()
	if err != nil {
		return err
	}
	defer st.Close()
	ctx := cmd.Context()

	if backfillCSV != "" {
		if backfillIndicator == "" {
			return fmt.Errorf("--csv requires --indicator")
		}
		n, err := importCSV(ctx, st, backfillCSV, backfillIndicator, backfillScale)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "imported %d observations for %s\n", n, backfillIndicator)
		return nil
	}

	if backfillFrom == "" {
		return fmt.Errorf("--from is required (or use --csv)")
	}
	apiKey := resolveFREDKey(ccfg.FRED.APIKeyEnv)
	if apiKey == "" {
		return fmt.Errorf("FRED key missing: set env %s or collectors.fred.api_key in the main config (-c)", ccfg.FRED.APIKeyEnv)
	}
	to := backfillTo
	if to == "" {
		to = time.Now().UTC().Format("2006-01-02")
	}
	ig := crisis.NewIngestor(fred.New(apiKey), yahoo.New(), st)
	rep, err := ig.IngestAll(ctx, backfillFrom, to)
	if err != nil {
		return err
	}
	for ind, n := range rep.Counts {
		fmt.Fprintf(cmd.OutOrStdout(), "%-10s %6d rows\n", ind, n)
	}
	for ind, ferr := range rep.YahooErrs {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: yahoo %s failed: %v (degrades to STALE)\n", ind, ferr)
	}
	return nil
}

func importCSV(ctx context.Context, st *crisis.Store, path, indicator string, scale float64) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	return importCSVFrom(ctx, st, f, indicator, scale)
}

// importCSVFrom reads date,value rows (optional header), multiplies values by
// scale and upserts them as manual_backfill observations (design §4.3: the
// HY OAS snapshot predating FRED's 3-year truncation comes in this way).
func importCSVFrom(ctx context.Context, st *crisis.Store, r io.Reader, indicator string, scale float64) (int, error) {
	rows, err := csv.NewReader(r).ReadAll()
	if err != nil {
		return 0, err
	}
	stamp := crisis.NowStamp(time.Now())
	obs := make([]crisis.Observation, 0, len(rows))
	for i, rec := range rows {
		if len(rec) < 2 {
			return 0, fmt.Errorf("line %d: want 2 columns date,value", i+1)
		}
		date := strings.TrimSpace(rec[0])
		if _, err := time.Parse("2006-01-02", date); err != nil {
			if i == 0 {
				continue // 表头
			}
			return 0, fmt.Errorf("line %d: bad date %q", i+1, date)
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(rec[1]), 64)
		if err != nil {
			return 0, fmt.Errorf("line %d: bad value %q", i+1, rec[1])
		}
		obs = append(obs, crisis.Observation{
			Date: date, Indicator: indicator, Value: v * scale,
			Source: "manual_backfill", FetchedAt: stamp,
		})
	}
	if err := st.UpsertObservations(ctx, obs); err != nil {
		return 0, err
	}
	return len(obs), nil
}
