package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/newthinker/atlas/internal/collector/fred"
	"github.com/newthinker/atlas/internal/collector/yahoo"
	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/crisis"
	"github.com/newthinker/atlas/internal/notifier/telegram"
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
	replayFrom        string
	replayTo          string
	replayJSON        bool
	replayAsOf        string
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

var crisisReplayCmd = &cobra.Command{
	Use:   "replay",
	Short: "Replay rules and state machine over backfilled history (no writes)",
	Long: `Re-runs the full evaluation pipeline day by day over macro_observations,
keeping evaluations in memory only. Used for the design §6 historical
acceptance (2008-09 / 2020-03 / 2024-08 / 2015-19 false-positive check) and
for threshold tuning: edit configs/crisis-monitor.yaml and re-run.`,
	RunE: runCrisisReplay,
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
	crisisEvalCmd.Flags().StringVar(&evalMode, "mode", "daily", "daily | nfci | intraday")
	crisisReplayCmd.Flags().StringVar(&replayFrom, "from", "", "start date YYYY-MM-DD (required)")
	crisisReplayCmd.Flags().StringVar(&replayTo, "to", "", "end date YYYY-MM-DD (required)")
	crisisReplayCmd.Flags().BoolVar(&replayJSON, "json", false, "emit transitions as JSON lines")
	crisisReplayCmd.Flags().StringVar(&replayAsOf, "as-of", "",
		"replay against what was visible at this instant (RFC3339 or YYYY-MM-DD; empty = current best estimate)")
	crisisCmd.AddCommand(crisisBackfillCmd, crisisEvalCmd, crisisStatusCmd, crisisReplayCmd)
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
	// 必须在构造 collector（下方 yahoo.New()）之前：resolveFREDKey 在 FRED_API_KEY
	// 非空时提前 return、不经 loadConfigOrDefaults，闸门就装不上。
	ensurePolicyGate()
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
	sender     crisis.Sender
}

// requiredDaily 是齐备性校验的必要集:FRED 日频序列(设计 §4.3——T+1 未齐则
// 退出等下次唤起);move/usdjpy 缺失走 STALE/NO_DATA 正常评估,nfci 为周频。
var requiredDaily = []string{crisis.IndVIX, crisis.IndHYOAS, crisis.IndT10Y2Y, crisis.IndSOFREFFR}

func runCrisisEval(cmd *cobra.Command, args []string) error {
	// 同 runCrisisBackfill：本函数里有三处 collector 构造（daily/nfci 的 yahoo.New()
	// 与 intraday 的 yahoo.New().FetchQuote），都必须在闸门装好之后。
	ensurePolicyGate()
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
			sender: buildCrisisSender(),
		}
		return executeCrisisEvalDaily(cmd.Context(), deps, evalDate)
	case "nfci":
		deps := crisisEvalDeps{
			cfg: ccfg, store: st, ingestNFCI: ig.IngestNFCI,
			now: time.Now, out: cmd.OutOrStdout(), errOut: cmd.ErrOrStderr(),
		}
		return executeCrisisEvalNFCI(cmd.Context(), deps)
	case "intraday":
		deps := crisisEvalDeps{
			cfg: ccfg, store: st, now: time.Now,
			out: cmd.OutOrStdout(), errOut: cmd.ErrOrStderr(), sender: buildCrisisSender(),
		}
		return executeCrisisIntraday(cmd.Context(), deps, yahoo.New().FetchQuote)
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
	// NotifyContext 必须在 AppendEvaluations 之前组装：PrevDay/StateDays/
	// ClearStreak 取的是"截至昨日"的历史（通知设计 §8）
	nc, err := buildNotifyContext(ctx, d, res)
	if err != nil {
		return err
	}
	if err := d.store.AppendEvaluations(ctx, res.Evaluations); err != nil {
		return err
	}
	printDayResult(d.out, res)

	for _, msg := range crisis.Messages(d.cfg, nc) {
		if d.sender == nil {
			fmt.Fprintln(d.out, msg) // 未配置 telegram：打印便于本地试运行
			continue
		}
		if err := d.sender.SendText(msg); err != nil {
			// 通知失败不失败退出：评估已落库，状态可由 status 自愈获取（文件真相源）
			fmt.Fprintf(d.errOut, "warning: notify failed: %v\n", err)
		}
	}
	return nil
}

// buildNotifyContext 组装通知渲染输入（通知设计 §8）。必须在 AppendEvaluations
// 之前调用：PrevDay/StateDays/ClearStreak 都取"截至昨日"的库内历史，当日增量
// （今日行、今日 any_trigger）在此函数内补足。
func buildNotifyContext(ctx context.Context, d crisisEvalDeps, res *crisis.DayResult) (crisis.NotifyContext, error) {
	nc := crisis.NotifyContext{Res: res, Summary: summaryKind(res.Date, res.State)}

	nc.PrevDay = map[string]crisis.Evaluation{}
	for _, ind := range crisis.AllIndicators {
		evals, err := d.store.RecentIndicatorEvals(ctx, ind, 1)
		if err != nil {
			return nc, err
		}
		if len(evals) > 0 {
			nc.PrevDay[ind] = evals[0]
		}
	}

	// 变更消息展示"前状态已持续 N 日"；无变更消息含当日（补充决策 6）
	if res.Transitioned() {
		days, err := stateStreakDays(ctx, d.store, res.PrevState)
		if err != nil {
			return nc, err
		}
		nc.StateDays = days
	} else {
		days, err := stateStreakDays(ctx, d.store, res.State)
		if err != nil {
			return nc, err
		}
		nc.StateDays = days + 1
	}

	// P2 去重：仅"昨日非 STALE、今日 STALE"的指标发一次（通知设计 §2）
	nc.StaleLastObs = map[string]string{}
	for _, ind := range crisis.AllIndicators {
		if res.Results[ind].Status != crisis.StatusStale {
			continue
		}
		if prev, ok := nc.PrevDay[ind]; ok && prev.Status == crisis.StatusStale {
			continue
		}
		nc.NewStale = append(nc.NewStale, ind)
		o, err := d.store.LatestObservation(ctx, ind)
		if err != nil {
			return nc, err
		}
		if o != nil {
			nc.StaleLastObs[ind] = o.Date
		}
	}

	// 周报退出进度：历史 any_trigger=false 连续日数 + 今日（补充决策 8）
	if res.State == crisis.StateWatch && nc.Summary == crisis.SummaryWeekly && !res.Detail.AnyTrigger {
		base, err := crisis.ClearStreakDays(d.store.History(ctx), crisis.StateWatch, d.cfg.StateMachine.WatchExitDays)
		if err != nil {
			return nc, err
		}
		nc.ClearStreak = base + 1
	}

	// 月报趋势：仅 SummaryMonthly ∧ NORMAL 时组装（通知设计 §8）
	if nc.Summary == crisis.SummaryMonthly && res.State == crisis.StateNormal {
		nc.Trends = map[string]crisis.Trend{}
		for _, ind := range crisis.AllIndicators {
			win, err := d.store.SeriesWindow(ctx, ind, res.Date, 21)
			if err != nil {
				return nc, err
			}
			if len(win) == 0 {
				continue
			}
			nc.Trends[ind] = crisis.Trend{Window: win, Delta: win[len(win)-1].Value - win[0].Value}
		}
	}
	return nc, nil
}

// buildCrisisSender 复用主配置 notifiers.telegram 凭据（serve.go:330 同款构造，
// notifier 零改动）。未配置或缺凭据 → nil（eval 退化为打印）。
func buildCrisisSender() crisis.Sender {
	cfg, err := loadConfigOrDefaults()
	if err != nil {
		return nil
	}
	nc, ok := cfg.Notifiers["telegram"]
	if !ok || !nc.Enabled || nc.BotToken == "" || nc.ChatID == "" {
		return nil
	}
	return telegram.New(nc.BotToken, nc.ChatID, telegram.WithProxy(nc.Proxy))
}

// summaryKind：NORMAL → 当月首个交易日发月报（设计 §4.3：不加第 4 个 plist，
// 在 daily eval 内判断），其余周一发周报（撞日归月报，NORMAL 周报设计 §3.1）；
// WATCH → 周一发周报；BREWING/CRISIS → 无摘要（日报已覆盖）。
func summaryKind(date string, state crisis.SystemState) crisis.SummaryKind {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return crisis.SummaryNone
	}
	switch state {
	case crisis.StateNormal:
		if isFirstTradingDayOfMonth(t) {
			return crisis.SummaryMonthly
		}
		if t.Weekday() == time.Monday {
			return crisis.SummaryWeekly
		}
	case crisis.StateWatch:
		if t.Weekday() == time.Monday {
			return crisis.SummaryWeekly
		}
	}
	return crisis.SummaryNone
}

func isFirstTradingDayOfMonth(t time.Time) bool {
	if t.Weekday() == time.Saturday || t.Weekday() == time.Sunday {
		return false
	}
	first := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	for first.Weekday() == time.Saturday || first.Weekday() == time.Sunday {
		first = first.AddDate(0, 0, 1)
	}
	return t.Equal(first)
}

// intradayIndicator 是盘中告警的去重行标识（不属于 7 个正式指标）。
const intradayIndicator = "usdjpy_intraday"

// executeCrisisIntraday（设计 §4.3 intraday_jpy 行）：先读库中系统状态，非
// BREWING/CRISIS 立即退出；否则用 JPY=X 实时价对库中 5 观测前收盘算周环比，
// 触红即发 [P0]（捕捉 carry trade 急平仓），以评估行做每日一次去重。
func executeCrisisIntraday(ctx context.Context, d crisisEvalDeps, quote func(string) (*core.Quote, error)) error {
	sys, err := d.store.LatestSystemEval(ctx)
	if err != nil {
		return err
	}
	if sys == nil || (sys.SystemState != crisis.StateBrewing && sys.SystemState != crisis.StateCrisis) {
		return nil
	}

	today := d.now().UTC().Format("2006-01-02")
	sent, err := d.store.HasIndicatorEvalForDate(ctx, intradayIndicator, today)
	if err != nil {
		return err
	}
	if sent {
		return nil
	}

	q, err := quote("JPY=X")
	if err != nil {
		return err
	}
	win, err := d.store.SeriesWindow(ctx, crisis.IndUSDJPY, today, 5)
	if err != nil {
		return err
	}
	if len(win) < 5 || win[0].Value == 0 {
		return nil // 历史不足，无法算周环比
	}
	wow := q.Price/win[0].Value - 1
	if wow > d.cfg.Indicators.USDJPY.RedWowPct {
		return nil
	}

	// 先落去重行再发送（文件真相源先行，通知丢失不重复告警）
	if err := d.store.AppendEvaluations(ctx, []crisis.Evaluation{{
		TS: today, EvalAt: crisis.NowStamp(d.now()), Indicator: intradayIndicator,
		Status: crisis.StatusRed, Value: q.Price,
		Detail: fmt.Sprintf(`{"wow":%.4f}`, wow),
	}}); err != nil {
		return err
	}
	msg := crisis.FormatIntradayAlert(q.Price, win[0].Value, wow, sys.SystemState, d.now())
	if d.sender == nil {
		fmt.Fprintln(d.out, msg)
		return nil
	}
	if err := d.sender.SendText(msg); err != nil {
		fmt.Fprintf(d.errOut, "warning: notify failed: %v\n", err)
	}
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

// parseAsOf 校验并规范化 --as-of 的入参。
//
// 两种形态都收（Leader 裁定 O5）：RFC3339（含纳秒，生产 fetched_at 就是那个形态）
// 与纯日期 YYYY-MM-DD（语义 = 当日 00:00:00Z 起点）。理由是两份文档各写了一种
// —— spec §5 判据四写 `--as-of 2026-07-13`，plan 的 flag 帮助文案写 RFC3339
// （`Step 2: 实现`，plan:769，那是 plan 里 RFC3339 的唯一一处）；只收其一会让照
// 另一份敲命令的人撞「格式非法」，而那是纯粹的摩擦，不是它该学的东西。
//
// 🔴 **只收 UTC（Z 结尾）**：这个值最终进 AsOfQuery 的 `fetched_at <= ?`，而
// fetched_at 是 TEXT ⇒ SQLite 做的是**字典序比较，不是时刻比较**。带偏移量的
// 入参与库里的 …Z 形态不同构，会静默错位。实测反例：
//
//	库内修订  2026-07-12T20:00:00.000000000Z
//	as-of     2026-07-13T00:00:00+08:00        （UTC = 2026-07-12T16:00:00Z）
//	字典序    db <= asof 为 true  ⇒ 该行被包含
//	时刻序    它在 as-of 之后 4 小时 ⇒ 本应排除
//
// 一个发生在 as-of **之后**的修订被静默包含，输出形状完全正常、不报错 —— 正是
// AsOfQuery 注释里担心的「静默偏移一个修订」，换了个成因。
//
// ⚠️ 为什么拒绝而不是 `t.UTC().Format(...)` 转换。先撤回一句旧说法：这里曾写着
// 「拒绝是唯一不产生新错位的选项」——假的，**本函数采用的 A 自己就有错位**。
//
// 三个方案，**同一库内行 row = …08.150777000Z，按入参逐格对照**（表头那一行就是入参，
// 三列是三种入参；跨列比较无意义 —— 上一版正是跨列比出了「B 更准」这个反向结论）：
//
//	入参 →                    …08Z（整秒）        …08.150777000Z   …08.000000000Z
//	A 原样透传（本函数采用）   row<=asof true      true（精确命中）  **false（正确排除）**
//	B t.UTC().Format 截尾      true（同 A）        true（≤1µs）      **true（过包含 ≤1s）**
//	C 定宽 .000000000 layout   解析失败            true（精确命中）  false（正确排除，同 A）
//
// 🔴 **A 与 B 各有一个对方没有的失效模式，谁也不「从不劣于」谁**（这里曾写着「A 在每一列
// 都弱优于 B，B 从不优于 A」——**那句是假的，已撤回**）：
//
//	方案   失效模式                            触发条件                     实测
//	A      **欠包含** —— 与 as-of 同一时刻的    入参小数位 **> 9**          小数位 10..15 全部命中
//	       那一行被排除                                                    （≤9 一侧零欠包含）
//	B      **过包含 ≤1s**                       入参给满 9 位且尾随零多      `.000000000Z` 被 Format
//	                                                                       削成 `…08Z`
//
// ⚠️ A 那一格里 **B 是对的**：`Format` 把超长小数规范化掉，正好消掉该失效
// （`row <= …08.1507770000Z` 为 false ✗，而 `row <= Format(…)=…08.150777Z` 为 true ✓）。
// ⇒ **存在 B 严格优于 A 的格子**，所以不能说 A 「从不劣于」B。
//
// ⚠️ 两者的**最坏**上界都是 1s（`…08.999999999Z <= …08Z` 为 true），但触发条件不同：
// A 的 1s 出现在入参为整秒形态时（第一列）—— 那不止来自「调用方自己敲整秒」，
// **纯日期分支自己就产出整秒形态**（见下）；B 的 1s 在调用方已给满 9 位时照样出现。
//
// 🔴 **那么为什么仍然选 A —— 这个理由此处未论证。** 已知的候选（都未验）：A 的失效触发
// 条件（入参写超过 9 位）是调用方能避免的，而 B 的触发条件（给满 9 位）是正常用法；
// 以及对 as-of 语义而言「过包含」可能比「欠包含」更糟（它让查询看见 as-of 之后写入的
// 修订）。**这两条都只是推论，没有生产数据支撑，所以不写成理由。** ⇒ 当前选 A 是既有
// 实现的延续，不是本注释论证出来的结论；若要改成 B，需要先把上面那个权衡真正论证一遍。
//
// ℹ️ `Format` 削的是**尾随零，削几位取决于数据**（实测 `.150777000Z`→削 3 位、
// `.100000000Z`→削 8 位、`.000000000Z`→连小数点一起削 10 位变 `…08Z`）。
//
// C 在第二、三列与 A 同样精确，**但它不排除偏移形态**：`Z07:00` 这个尾部同时接受 Z 与
// 偏移量，实测 `2026-07-13T00:00:00.000000000+08:00` **解析成功**（UTC=2026-07-12T16:00:00Z，
// 日期退到前一天）。
//
// 🔴 **「不排除偏移」包括零偏移形态 `±00:00`** —— 即本函数第一版那个缺陷（`off != 0` 放行
// `+00:00`）在 C 方案下会原样再现。特地写出来是因为：第一版漏掉它，正是因为「偏移」这个词
// 在读者的默认理解里已经把 `±00:00` 排除在外了。要排除偏移得把 layout 末尾**写死 `Z`**
// （实测：写死 `Z` 的版本对 `+08:00`/`-05:00`/`+00:00`/小写 `z`/无时区全部拒绝，只收 `Z`）；
// 仓库现存的 `timeLayout`（dates.go）正是 `Z07:00`，它接受偏移。
//
// C 真正被否决的理由是另一条：**它要求入参写满 9 位小数**，纯日期 `2026-07-13` 与整秒
// `…08Z` 都会被拒，与下面「纯日期补成 00:00:00Z」的契约直接冲突。
//
// 所以真实理由不是「错位最小」，是**错位的上界由谁决定**：
//
//	带偏移量的入参   上界由入参偏移量决定；Go 的 RFC3339 解析器判的是「小时字段 ≤24
//	                 且分钟字段 ≤60」，实测连 +24:60（等效 25h）都收，+25:00 与 +24:99
//	                 才失败 ——调用方写得再精确也消不掉（本函数整类拒绝它，见下）
//	…Z 形态的入参    上界 = 调用方自己给的精度。给满 9 位就是 0，给整秒就是 1s
//
// ⚠️ 后一条依赖一个不变量：**库内每一行 fetched_at 都是定宽 9 位**。**生产写路径**
// （`ingest.go:101/139/163` 与本文件 `:204`，四处全部经 `NowStamp` =
// `t.UTC().Format(timeLayout)`）保证了它，故在本库形态下成立 —— ⚠️ 主语是「生产写路径」
// 而非「全部写入点」：测试夹具会直接塞非定宽值（如 `FetchedAt: "x"`）。
// 另注：`eval.go` 的 `NowStamp` 写的是 `crisis_evaluations.eval_at`，与 `fetched_at` 无关。
// 而**写路径没有校验**（`UpsertObservations` 原样写入，`migrate.go` 的
// 搬运是 `SELECT fetched_at FROM …_v1`，老库里是什么就搬什么）⇒ 从外部老库迁入时不保证。
//
// 🔴 **不变量破例时上面那个「A 每一列弱优于 B」会反转**：`row=…08Z`（无小数）对
// `asof=…08.000000000Z`（满 9 位）⇒ A 为 **false**（同一时刻被错误排除），而 B 把 asof
// 压成 `…08Z` ⇒ **true**（正确包含）。⇒ 那一格 **B 严格优于 A**。A 的优势是**有条件的**，
// 条件就是这个不变量。
//
// ⇒ 拒绝偏移形态，是把**调用方无法控制**的错位换成**调用方能控制**的错位，不是把错位
// 消掉。B 之所以仍被否决，看最后一列：调用方已经给满 9 位了，`Format` 仍把它压成整秒，
// 于是那一格从「能控制」变回「不能控制」。
//
// 与 `store_test.go` 的 `TestAsOfNanoPrecisionBoundary` 不矛盾：它用的 as-of 正是第一列
// 那个整秒形态，钉住的是**包含方向**（`'.'(0x2E) < 'Z'(0x5A)` ⇒ 整秒 as-of 会包含同秒带
// 小数的行），推导见 `store_test.go:828-832`。⚠️ 那里**没有**给出 ≤1s 这个上界 —— 上界是
// 本注释第一列补的，别按「1s」去那边找。
//
// RFC3339 原样返回（保留用户给的精度，生产 fetched_at 就是纳秒形态），纯日期补成
// 00:00:00Z，两条分支的输出都是 Z 结尾的 UTC RFC3339，读起来不用猜它被当成了几点。
//
// ⚠️ 但**两条分支的输出宽度不同**，别把「都是 Z 结尾」读成「与库内同形」：
//
//	库内 fetched_at      2026-07-14T05:42:08.150777000Z   len=30（定宽 9 位）
//	RFC3339 分支输出     原样，用户给几位就是几位          len 随入参
//	纯日期分支输出       2026-07-14T00:00:00Z             len=20（整秒）
//
// 🔴 **纯日期分支落在上面那张表的「整秒」那一列** —— 也就是说，**本函数自己会产出
// 整秒形态**，过包含上界 1s 不止在「调用方自己敲整秒」时出现。而纯日期恰是 spec §5
// 判据四的原文形态（`--as-of 2026-07-13`），是最常被敲的那一种。实测：库内行
// `…T00:00:00.999999999Z` 对纯日期输出 `…T00:00:00Z` ⇒ 字典序 true、时刻序 false
// ⇒ 过包含 999.999999ms。
//
// 空值返回空，由调用方走当前形态；那是 --as-of 缺省时唯一合理的含义，不是错误。
func parseAsOf(v string) (string, error) {
	if v == "" {
		return "", nil
	}
	if _, err := time.Parse(time.RFC3339Nano, v); err == nil {
		// 🔴 判据是「**原文**以 Z 结尾」，不是「解析后的偏移量为零」。
		//
		// 前一版写的是 `if _, off := t.Zone(); off != 0`，它漏掉 `+00:00` 与
		// `-00:00` —— 那两种形态解析后 off 确实是 0，于是被放行、原样透传进
		// `fetched_at <= ?`。而那里做的是**文本**比较：
		//
		//	row   = 2026-07-13T00:00:00.000000000Z      （与 as-of 同一时刻）
		//	row <= 2026-07-13T00:00:00Z        → true   包含
		//	row <= 2026-07-13T00:00:00+00:00   → false  **排除**
		//
		// 同一时刻的两种合法 RFC3339 写法给出相反结果，无错误、输出形状正常。
		// 危害是欠包含，漏掉的恰是「as-of 那一瞬间的那个修订」—— 与 AsOfQuery
		// 里 `<=` 写成 `<` 是同一类错误，只是换了一条路径进来。
		//
		// ⇒ `off != 0` 问的是**语义**，而被比较的是**文本**；判据必须与被比较的
		// 东西同口径。这也是为什么前一版的错误文案（"need a trailing Z … an
		// offset form silently misaligns"）**没有覆盖它自己描述的集合**：
		// `+00:00` 既是偏移形式、又没有 trailing Z，却过了那道闸。
		if !strings.HasSuffix(v, "Z") {
			return "", fmt.Errorf(
				"--as-of %q: need UTC with a trailing Z — fetched_at is compared as text, "+
					"so any offset form (including +00:00) silently misaligns; "+
					"write e.g. 2026-07-13T00:00:00Z", v)
		}
		return v, nil
	}
	if t, err := time.Parse("2006-01-02", v); err == nil {
		return t.UTC().Format(time.RFC3339), nil
	}
	return "", fmt.Errorf(
		"--as-of %q: want RFC3339 (e.g. 2026-07-13T00:00:00Z) or a plain date (2026-07-13)", v)
}

func runCrisisReplay(cmd *cobra.Command, args []string) error {
	if replayFrom == "" || replayTo == "" {
		return fmt.Errorf("--from and --to are required")
	}
	// 校验在开库**之前**：参数写错的人要在第一秒知道，不等开库
	// （同 runCrisisReplay 既有的 --from/--to 校验，也同 hestia.go 的 --only-period）。
	asOf, err := parseAsOf(replayAsOf)
	if err != nil {
		return err
	}
	ccfg, st, err := openCrisisStore()
	if err != nil {
		return err
	}
	defer st.Close()

	// 分支而不是无条件 st.AsOf(asOf)：让「不带 --as-of ⇒ 与改动前**逐字同一条
	// 路径**」一眼可见。functional[1] 是【守住】类断言，而守住的正是这条路径没变；
	// 写成无条件调用虽等价（AsOf("") 返回当前形态），却要多读一层才能确认。
	//
	// AsOf 返回的副本共用同一个 *sql.DB（AD-8），所以上面那个 defer st.Close()
	// 已经覆盖它 —— 不要在这里再 Close 一次。
	reader := st
	if asOf != "" {
		reader = st.AsOf(asOf)
	}
	return executeCrisisReplay(cmd.Context(), ccfg, reader, replayFrom, replayTo, asOf, replayJSON, cmd.OutOrStdout())
}

// executeCrisisReplay 逐日重放:观测来自 sqlite,评估历史只进 MemHistory,
// 不写 crisis_evaluations(审计表只属于 live eval)。v1.1 起统一暖机语义:
// 引擎从库内最早观测日推进,窗口期初态为暖机结果。
func executeCrisisReplay(ctx context.Context, cfg *crisis.Config, st *crisis.Store, from, to, asOf string, jsonOut bool, out io.Writer) error {
	days, err := crisis.ReplayRange(cfg, st.Reader(ctx), from, to)
	if err != nil {
		return err
	}
	if len(days) == 0 {
		// 带 --as-of 时「空」是**正确答案**而不是错误：那个时点上本来就还没有
		// 这段区间的观测（判据四）。不带 --as-of 时空仍然报错 —— 那多半是忘了
		// backfill，提示他比静默返回 0 行有用。同一个空结果，两种语境两种含义。
		if asOf != "" {
			return nil
		}
		return fmt.Errorf("no observations between %s and %s — run backfill first", from, to)
	}

	entered := map[crisis.SystemState]int{}
	for _, day := range days {
		res := day.Res
		if !res.Transitioned() {
			continue
		}
		entered[res.State]++
		if jsonOut {
			b, _ := json.Marshal(map[string]any{
				"date": day.Date, "from": res.PrevState, "to": res.State, "amber_count": res.Detail.AmberCount,
			})
			fmt.Fprintln(out, string(b))
		} else {
			fmt.Fprintf(out, "%s  %s → %s (amber=%d)\n", day.Date, res.PrevState, res.State, res.Detail.AmberCount)
		}
	}

	fmt.Fprintf(out, "\nfinal state: %s over %d eval days\n", days[len(days)-1].Res.State, len(days))
	for _, s := range []crisis.SystemState{crisis.StateWatch, crisis.StateBrewing, crisis.StateCrisis} {
		fmt.Fprintf(out, "entered %-8s %d times\n", s, entered[s])
	}
	return nil
}
