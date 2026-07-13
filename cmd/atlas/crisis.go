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
	evalDate          string
	evalMode          string
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

var crisisEvalCmd = &cobra.Command{
	Use:   "eval",
	Short: "Fetch latest data and run one evaluation (launchd entrypoint)",
	RunE:  runCrisisEval,
}

var crisisStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Print current system state and latest indicator readings",
	RunE:  runCrisisStatus,
}

func init() {
	crisisCmd.PersistentFlags().StringVar(&crisisCfgPath, "crisis-config",
		"configs/crisis-monitor.yaml", "crisis monitor config path")
	crisisBackfillCmd.Flags().StringVar(&backfillFrom, "from", "", "start date YYYY-MM-DD (FRED/Yahoo backfill)")
	crisisBackfillCmd.Flags().StringVar(&backfillTo, "to", "", "end date YYYY-MM-DD (default today)")
	crisisBackfillCmd.Flags().StringVar(&backfillCSV, "csv", "", "CSV snapshot path (date,value)")
	crisisBackfillCmd.Flags().StringVar(&backfillIndicator, "indicator", "", "indicator for --csv import (e.g. hy_oas)")
	crisisBackfillCmd.Flags().Float64Var(&backfillScale, "scale", 1, "value multiplier for --csv (percent→bp: 100)")
	crisisEvalCmd.Flags().StringVar(&evalDate, "date", "", "override evaluation date YYYY-MM-DD (default: previous trading day)")
	crisisEvalCmd.Flags().StringVar(&evalMode, "mode", "daily", "daily | nfci")
	crisisCmd.AddCommand(crisisBackfillCmd, crisisEvalCmd, crisisStatusCmd)
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
	// missing "fred" key yields a zero CollectorConfig, i.e. empty APIKey
	return cfg.Collectors["fred"].APIKey
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

// crisisEvalDeps 注入依赖使 daily/nfci 流程可单测(模式同 watchlistDeps)。
type crisisEvalDeps struct {
	cfg        *crisis.Config
	store      *crisis.Store
	ingest     func(ctx context.Context, from, to string) (*crisis.IngestReport, error)
	ingestNFCI func(ctx context.Context, from, to string) (int, error)
	now        func() time.Time
	out        io.Writer
	errOut     io.Writer
}

// requiredDaily 是齐备性校验的必要集:FRED 日频序列(设计 §4.3——T+1 未齐则
// 退出等下次唤起);move/usdjpy 缺失走 STALE/NO_DATA 正常评估,nfci 为周频。
var requiredDaily = []string{crisis.IndVIX, crisis.IndHYOAS, crisis.IndT10Y2Y, crisis.IndSOFREFFR}

func runCrisisEval(cmd *cobra.Command, args []string) error {
	ccfg, st, err := openCrisisStore()
	if err != nil {
		return err
	}
	defer st.Close()

	apiKey := resolveFREDKey(ccfg.FRED.APIKeyEnv)
	if apiKey == "" {
		return fmt.Errorf("FRED key missing: set env %s or collectors.fred.api_key in the main config (-c)", ccfg.FRED.APIKeyEnv)
	}
	ig := crisis.NewIngestor(fred.New(apiKey), yahoo.New(), st)

	switch evalMode {
	case "daily":
		deps := crisisEvalDeps{
			cfg: ccfg, store: st, ingest: ig.IngestAll,
			now: time.Now, out: cmd.OutOrStdout(), errOut: cmd.ErrOrStderr(),
		}
		return executeCrisisEvalDaily(cmd.Context(), deps, evalDate)
	case "nfci":
		deps := crisisEvalDeps{
			cfg: ccfg, store: st, ingestNFCI: ig.IngestNFCI,
			now: time.Now, out: cmd.OutOrStdout(), errOut: cmd.ErrOrStderr(),
		}
		return executeCrisisEvalNFCI(cmd.Context(), deps)
	default:
		return fmt.Errorf("unknown --mode %q", evalMode)
	}
}

// executeCrisisEvalNFCI 仅刷新周频 NFCI(now−30d..today),不做评估——NFCI 更新后
// 参与后续 daily 评估(设计 §3.2 条 4)。
func executeCrisisEvalNFCI(ctx context.Context, d crisisEvalDeps) error {
	now := d.now().UTC()
	n, err := d.ingestNFCI(ctx,
		now.AddDate(0, 0, -30).Format("2006-01-02"), now.Format("2006-01-02"))
	if err != nil {
		return err
	}
	fmt.Fprintf(d.out, "nfci refreshed: %d rows\n", n)
	return nil
}

func executeCrisisEvalDaily(ctx context.Context, d crisisEvalDeps, dateOverride string) error {
	target := dateOverride
	if target == "" {
		target = crisis.PrevTradingDay(d.now().UTC()).Format("2006-01-02")
	}

	// 幂等:多时点唤起的第 2+ 次直接空跑(设计 §4.3,幂等由库保证)
	done, err := d.store.HasSystemEvalForDate(ctx, target)
	if err != nil {
		return err
	}
	if done {
		fmt.Fprintf(d.out, "already evaluated %s, nothing to do\n", target)
		return nil
	}

	// 增量采集:45 天回看覆盖 NFCI 周频与假日空洞,upsert 幂等
	from := mustAddDays(target, -45)
	rep, err := d.ingest(ctx, from, d.now().UTC().Format("2006-01-02"))
	if err != nil {
		return err
	}
	for ind, ferr := range rep.YahooErrs {
		fmt.Fprintf(d.errOut, "warning: yahoo %s failed: %v\n", ind, ferr)
	}

	// 数据齐备性:required 序列在 target 日必须有观测(T+1 校验)
	for _, ind := range requiredDaily {
		obs, err := d.store.Observation(ctx, ind, target)
		if err != nil {
			return err
		}
		if obs == nil {
			fmt.Fprintf(d.out, "data not ready for %s (%s missing), waiting for next wakeup\n", target, ind)
			return nil
		}
	}

	res, err := crisis.EvalDay(d.cfg, target, d.store.Reader(ctx), d.store.History(ctx), d.now())
	if err != nil {
		return err
	}
	if err := d.store.AppendEvaluations(ctx, res.Evaluations); err != nil {
		return err
	}
	printDayResult(d.out, res)
	return nil
}

func mustAddDays(date string, n int) string {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return date
	}
	return t.AddDate(0, 0, n).Format("2006-01-02")
}

func printDayResult(w io.Writer, res *crisis.DayResult) {
	if res.Transitioned() {
		fmt.Fprintf(w, "%s: %s → %s\n", res.Date, res.PrevState, res.State)
	} else {
		fmt.Fprintf(w, "%s: %s\n", res.Date, res.State)
	}
	for _, ind := range crisis.AllIndicators {
		r := res.Results[ind]
		fmt.Fprintf(w, "  %-10s %-20s %10.2f  p5y=%.2f  %s\n", ind, r.Status, r.Value, r.Pct5y, r.Tag)
	}
}

func runCrisisStatus(cmd *cobra.Command, args []string) error {
	_, st, err := openCrisisStore()
	if err != nil {
		return err
	}
	defer st.Close()
	return executeCrisisStatus(cmd.Context(), st, cmd.OutOrStdout())
}

func executeCrisisStatus(ctx context.Context, st *crisis.Store, out io.Writer) error {
	sys, err := st.LatestSystemEval(ctx)
	if err != nil {
		return err
	}
	if sys == nil {
		fmt.Fprintln(out, "no evaluations yet — run `atlas crisis eval` after backfill")
		return nil
	}
	days, err := stateStreakDays(ctx, st, sys.SystemState)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "system state: %s (as of %s, %d eval days)\n", sys.SystemState, sys.TS, days)
	for _, ind := range crisis.AllIndicators {
		evals, err := st.RecentIndicatorEvals(ctx, ind, 1)
		if err != nil {
			return err
		}
		if len(evals) == 0 {
			continue
		}
		e := evals[0]
		fmt.Fprintf(out, "  %-10s %-20s %10.2f  p5y=%.2f  %s\n", ind, e.Status, e.Value, e.Pct5y, e.Tag)
	}
	return nil
}

// stateStreakDays 统计与当前状态相同的连续系统评估行数 = 状态持续评估日数。
func stateStreakDays(ctx context.Context, st *crisis.Store, state crisis.SystemState) (int, error) {
	evals, err := st.RecentSystemEvals(ctx, 500)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range evals {
		if e.SystemState != state {
			break
		}
		n++
	}
	return n, nil
}
