package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/newthinker/atlas/internal/crisis"
)

// Context Checkpoint: done_criteria → test mapping
// functional[0]     "crisis 父命令注册,--crisis-config 默认 crisis-monitor.yaml" → TestCrisisCommandRegistered / TestCrisisConfigFlagDefault
// functional[1]     "backfill --from/--to 调 IngestAll 打印各指标条数"           → (网络路径属 manual non_functional[1]) 错误分支 → TestRunCrisisBackfillErrors
// functional[2]     "backfill --csv/--indicator/--scale 导入 date,value"        → TestRunCrisisBackfillCSV
// boundary[0]       "importCSV 带/不带表头,--scale 生效,source=manual_backfill" → TestImportCSVFrom
// error_handling[0] "配置缺失 / FRED key 不可得 / CSV 坏值坏日期 报错"           → TestImportCSVFrom / TestImportCSVFileErrors / TestRunCrisisBackfillErrors
// non_functional[0] "go build ./cmd/atlas 通过且 go test ./cmd/atlas/ 全绿"       → test 门禁

func newCrisisTestStore(t *testing.T) *crisis.Store {
	t.Helper()
	st, err := crisis.NewStore(filepath.Join(t.TempDir(), "crisis.db"))
	require.NoError(t, err)
	t.Cleanup(func() { st.Close() })
	return st
}

// snapshotCrisisFlags saves and restores the package-level backfill flag vars
// so tests that mutate them don't leak into each other.
func snapshotCrisisFlags(t *testing.T) {
	t.Helper()
	pCfg, pFrom, pTo := crisisCfgPath, backfillFrom, backfillTo
	pCSV, pInd, pScale, pFile := backfillCSV, backfillIndicator, backfillScale, cfgFile
	t.Cleanup(func() {
		crisisCfgPath, backfillFrom, backfillTo = pCfg, pFrom, pTo
		backfillCSV, backfillIndicator, backfillScale, cfgFile = pCSV, pInd, pScale, pFile
	})
}

// writeTempCrisisConfig copies the repository crisis config with storage.path
// pointed at a temp db so runCrisisBackfill can open a real store offline.
func writeTempCrisisConfig(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("../../configs/crisis-monitor.yaml")
	require.NoError(t, err)
	dbPath := filepath.Join(t.TempDir(), "crisis.db")
	out := strings.Replace(string(raw), "data/crisis.db", dbPath, 1)
	require.Contains(t, out, dbPath, "storage.path replacement must apply")
	cfgPath := filepath.Join(t.TempDir(), "crisis-monitor.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(out), 0o644))
	return cfgPath
}

func runBackfill(t *testing.T) (stdout, stderr string, err error) {
	t.Helper()
	var out, errb bytes.Buffer
	c := &cobra.Command{}
	c.SetContext(context.Background())
	c.SetOut(&out)
	c.SetErr(&errb)
	err = runCrisisBackfill(c, nil)
	return out.String(), errb.String(), err
}

func TestImportCSVFrom(t *testing.T) {
	st := newCrisisTestStore(t)
	ctx := context.Background()

	// 表头 + 两行数据,scale=100(百分数 → bp)
	csvData := "date,value\n2006-01-03,3.55\n2006-01-04,3.60\n"
	n, err := importCSVFrom(ctx, st, strings.NewReader(csvData), "hy_oas", 100)
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	obs, err := st.Observation(ctx, "hy_oas", "2006-01-03")
	require.NoError(t, err)
	require.NotNil(t, obs)
	assert.InDelta(t, 355.0, obs.Value, 1e-9)
	assert.Equal(t, "manual_backfill", obs.Source)

	// 无表头也可导入
	n, err = importCSVFrom(ctx, st, strings.NewReader("2006-01-05,3.70\n"), "hy_oas", 100)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	// 坏值与坏日期(非首行)报错
	_, err = importCSVFrom(ctx, st, strings.NewReader("2006-01-06,abc\n"), "hy_oas", 1)
	require.Error(t, err)
	_, err = importCSVFrom(ctx, st, strings.NewReader("date,value\nnot-a-date,1\n"), "hy_oas", 1)
	require.Error(t, err)

	// 列数不足报错
	_, err = importCSVFrom(ctx, st, strings.NewReader("2006-01-06\n"), "hy_oas", 1)
	require.Error(t, err)
}

func TestCrisisCommandRegistered(t *testing.T) {
	var crisisUse []string
	for _, c := range rootCmd.Commands() {
		if c.Use == "crisis" {
			for _, sub := range c.Commands() {
				crisisUse = append(crisisUse, sub.Use)
			}
		}
	}
	assert.Contains(t, crisisUse, "backfill")
}

func TestCrisisConfigFlagDefault(t *testing.T) {
	f := crisisCmd.PersistentFlags().Lookup("crisis-config")
	require.NotNil(t, f)
	assert.Equal(t, "configs/crisis-monitor.yaml", f.DefValue)
}

// importCSV 打开真实文件路径:成功导入 + 文件不存在报错。
func TestImportCSVFileErrors(t *testing.T) {
	st := newCrisisTestStore(t)
	ctx := context.Background()

	path := filepath.Join(t.TempDir(), "snap.csv")
	require.NoError(t, os.WriteFile(path, []byte("date,value\n2006-01-03,3.55\n"), 0o644))
	n, err := importCSV(ctx, st, path, "hy_oas", 100)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	_, err = importCSV(ctx, st, filepath.Join(t.TempDir(), "missing.csv"), "hy_oas", 1)
	require.Error(t, err)
}

// backfill --csv 端到端:经 runCrisisBackfill → openCrisisStore → importCSV 入库。
func TestRunCrisisBackfillCSV(t *testing.T) {
	snapshotCrisisFlags(t)
	crisisCfgPath = writeTempCrisisConfig(t)
	csvPath := filepath.Join(t.TempDir(), "hy.csv")
	require.NoError(t, os.WriteFile(csvPath, []byte("date,value\n2006-01-03,3.55\n2006-01-04,3.60\n"), 0o644))
	backfillCSV = csvPath
	backfillIndicator = "hy_oas"
	backfillScale = 100
	backfillFrom, backfillTo = "", ""

	out, _, err := runBackfill(t)
	require.NoError(t, err)
	assert.Contains(t, out, "imported 2 observations for hy_oas")
}

func TestRunCrisisBackfillErrors(t *testing.T) {
	cfgPath := writeTempCrisisConfig(t)

	t.Run("csv without indicator", func(t *testing.T) {
		snapshotCrisisFlags(t)
		crisisCfgPath = cfgPath
		backfillCSV = filepath.Join(t.TempDir(), "x.csv")
		require.NoError(t, os.WriteFile(backfillCSV, []byte("date,value\n"), 0o644))
		backfillIndicator = ""
		_, _, err := runBackfill(t)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "indicator")
	})

	t.Run("no from no csv", func(t *testing.T) {
		snapshotCrisisFlags(t)
		crisisCfgPath = cfgPath
		backfillCSV, backfillIndicator, backfillFrom = "", "", ""
		_, _, err := runBackfill(t)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--from")
	})

	t.Run("fred key missing", func(t *testing.T) {
		snapshotCrisisFlags(t)
		t.Setenv("FRED_API_KEY", "")
		crisisCfgPath = cfgPath
		cfgFile = "" // loadConfigOrDefaults → Defaults(),无 fred key
		backfillCSV, backfillIndicator = "", ""
		backfillFrom = "2006-01-01"
		_, _, err := runBackfill(t)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "FRED key")
	})

	t.Run("crisis config missing", func(t *testing.T) {
		snapshotCrisisFlags(t)
		crisisCfgPath = filepath.Join(t.TempDir(), "nope.yaml")
		backfillCSV, backfillIndicator, backfillFrom = "", "", ""
		_, _, err := runBackfill(t)
		require.Error(t, err)
	})

	t.Run("csv bad value propagates via command", func(t *testing.T) {
		snapshotCrisisFlags(t)
		crisisCfgPath = cfgPath
		backfillCSV = filepath.Join(t.TempDir(), "bad.csv")
		require.NoError(t, os.WriteFile(backfillCSV, []byte("2006-01-03,abc\n"), 0o644))
		backfillIndicator = "hy_oas"
		backfillScale = 1
		_, _, err := runBackfill(t)
		require.Error(t, err)
	})
}

func TestResolveFREDKey(t *testing.T) {
	t.Run("env takes precedence", func(t *testing.T) {
		t.Setenv("FRED_API_KEY", "env-key")
		assert.Equal(t, "env-key", resolveFREDKey("FRED_API_KEY"))
	})

	t.Run("falls back to main config collectors.fred.api_key", func(t *testing.T) {
		snapshotCrisisFlags(t)
		t.Setenv("FRED_API_KEY", "")
		mainCfg := filepath.Join(t.TempDir(), "config.yaml")
		require.NoError(t, os.WriteFile(mainCfg, []byte("collectors:\n  fred:\n    api_key: cfg-key\n"), 0o644))
		cfgFile = mainCfg
		assert.Equal(t, "cfg-key", resolveFREDKey("FRED_API_KEY"))
	})

	t.Run("missing everywhere returns empty", func(t *testing.T) {
		snapshotCrisisFlags(t)
		t.Setenv("FRED_API_KEY", "")
		cfgFile = ""
		assert.Equal(t, "", resolveFREDKey("FRED_API_KEY"))
	})

	t.Run("unloadable main config returns empty", func(t *testing.T) {
		snapshotCrisisFlags(t)
		t.Setenv("FRED_API_KEY", "")
		cfgFile = filepath.Join(t.TempDir(), "does-not-exist.yaml")
		assert.Equal(t, "", resolveFREDKey("FRED_API_KEY"))
	})
}

// ---- TASK-012: crisis eval / status ----
// done_criteria → test mapping
// functional[0] "目标日=--date 或 PrevTradingDay(now);评估落库打印摘要" → TestExecuteCrisisEvalDaily / TestExecuteCrisisEvalDailyDateOverride
// functional[1] "幂等 HasSystemEvalForDate → already evaluated exit 0"    → TestExecuteCrisisEvalDaily(第二次唤起)
// functional[2] "数据齐备门:四序列缺任一 → not ready exit 0"             → TestExecuteCrisisEvalDailyDataNotReady
// functional[3] "nfci 模式仅 IngestNFCI 不评估;status 打印状态/持续/指标"  → TestRunCrisisEvalNFCI(错误路径) / TestExecuteCrisisStatus
// boundary[0]   "move/usdjpy 缺 target 不阻断(STALE/NO_DATA 正常评估)"    → TestExecuteCrisisEvalDailyMoveMissing
// boundary[1]   "库空 status 打印 no evaluations yet 正常返回"            → TestExecuteCrisisStatus
// error_handling[0] "ingest 失败返回非零错误(区别 not ready 空跑)"        → TestExecuteCrisisEvalDailyIngestError

// crisisTestConfig 与 configs/crisis-monitor.yaml 数值一致(cmd 测试不读文件)。
func crisisTestConfig() *crisis.Config {
	return &crisis.Config{
		Storage:    crisis.StorageCfg{Path: "unused"},
		FRED:       crisis.FREDCfg{APIKeyEnv: "FRED_API_KEY"},
		Freshness:  crisis.FreshnessCfg{DailyMaxLagDays: 4, WeeklyMaxLagDays: 12},
		Percentile: crisis.PercentileCfg{WindowYears: 5, Amber: 0.90, Red: 0.97},
		Indicators: crisis.IndicatorsCfg{
			VIX:      crisis.VIXCfg{Amber: 25, Red: 30, WeeklySpikePct: 0.50, PercentileTrack: true},
			MOVE:     crisis.MOVECfg{Amber: 100, Red: 120, PercentileTrack: true},
			SOFREFFR: crisis.SOFREFFRCfg{AmberBp: 10, AmberPersistDays: 3, RedBp: 25, RedPersistDays: 5, PercentileTrack: true, SuppressQuarterEnd: true},
			HYOAS:    crisis.HYOASCfg{AmberLowBp: 350, AmberHighBp: 500, RedBp: 600, MomentumBp: 100, MomentumWindowObs: 21, PercentileTrack: true},
			T10Y2Y:   crisis.T10Y2YCfg{AmberBp: 25, SteepeningBp: 50, SteepeningLookbackObs: 250},
			NFCI:     crisis.NFCICfg{GreenBelow: -0.3, RedAbove: 0},
			USDJPY:   crisis.USDJPYCfg{AmberWowPct: -0.02, RedWowPct: -0.03, Crowded52wPct: 0.98},
		},
		StateMachine: crisis.StateMachineCfg{WatchAmberCount: 3, CrisisExitDays: 10, WatchExitDays: 20, BrewingExitDays: 10, DemoteHysteresisDays: 3},
	}
}

var greenSeedVals = map[string]float64{
	crisis.IndVIX: 15, crisis.IndMOVE: 70, crisis.IndSOFREFFR: -10, crisis.IndHYOAS: 400,
	crisis.IndT10Y2Y: 35, crisis.IndNFCI: -0.5, crisis.IndUSDJPY: 150,
}

// seedIndicators 逐日回推 days 天,为 vals 中每个指标写入全绿观测。
func seedIndicators(t *testing.T, st *crisis.Store, target string, days int, vals map[string]float64) {
	t.Helper()
	tt, err := time.Parse("2006-01-02", target)
	require.NoError(t, err)
	var obs []crisis.Observation
	for i := 0; i < days; i++ {
		d := tt.AddDate(0, 0, -i).Format("2006-01-02")
		for ind, v := range vals {
			obs = append(obs, crisis.Observation{Date: d, Indicator: ind, Value: v,
				Source: "test", FetchedAt: "2026-07-11T00:00:00.000000000Z"})
		}
	}
	require.NoError(t, st.UpsertObservations(context.Background(), obs))
}

func seedObservations(t *testing.T, st *crisis.Store, target string, days int) {
	seedIndicators(t, st, target, days, greenSeedVals)
}

func noopIngest(calls *int) func(context.Context, string, string) (*crisis.IngestReport, error) {
	return func(context.Context, string, string) (*crisis.IngestReport, error) {
		*calls++
		return &crisis.IngestReport{Counts: map[string]int{}, YahooErrs: map[string]error{}}, nil
	}
}

// sat711 是周六上午唤起 → 目标评估日 = 前一交易日 2026-07-10(周五)。
func sat711() time.Time { return time.Date(2026, 7, 11, 8, 0, 0, 0, time.UTC) }

func TestExecuteCrisisEvalDaily(t *testing.T) {
	st := newCrisisTestStore(t)
	ctx := context.Background()
	const target = "2026-07-10"
	seedObservations(t, st, target, 80)

	var calls int
	deps := crisisEvalDeps{
		cfg: crisisTestConfig(), store: st, ingest: noopIngest(&calls),
		now: sat711, out: io.Discard, errOut: io.Discard,
	}

	require.NoError(t, executeCrisisEvalDaily(ctx, deps, ""))
	assert.Equal(t, 1, calls)
	sys, err := st.LatestSystemEval(ctx)
	require.NoError(t, err)
	require.NotNil(t, sys)
	assert.Equal(t, crisis.StateNormal, sys.SystemState)
	assert.Equal(t, target, sys.TS)

	// 幂等:同日第二次唤起既不重复评估也不再采集
	require.NoError(t, executeCrisisEvalDaily(ctx, deps, ""))
	assert.Equal(t, 1, calls)
	evals, err := st.RecentSystemEvals(ctx, 10)
	require.NoError(t, err)
	assert.Len(t, evals, 1)
}

// --date override:显式指定目标日,绕过 PrevTradingDay。
func TestExecuteCrisisEvalDailyDateOverride(t *testing.T) {
	st := newCrisisTestStore(t)
	ctx := context.Background()
	const target = "2026-07-08"
	seedObservations(t, st, target, 80)

	var calls int
	deps := crisisEvalDeps{
		cfg: crisisTestConfig(), store: st, ingest: noopIngest(&calls),
		now: sat711, out: io.Discard, errOut: io.Discard,
	}
	require.NoError(t, executeCrisisEvalDaily(ctx, deps, target))
	sys, err := st.LatestSystemEval(ctx)
	require.NoError(t, err)
	require.NotNil(t, sys)
	assert.Equal(t, target, sys.TS) // 用 override 而非 PrevTradingDay(7/10)
}

func TestExecuteCrisisEvalDailyDataNotReady(t *testing.T) {
	st := newCrisisTestStore(t)
	ctx := context.Background()
	seedObservations(t, st, "2026-07-09", 80) // 数据只到 7/9,目标日 7/10 未齐

	var calls int
	deps := crisisEvalDeps{
		cfg: crisisTestConfig(), store: st, ingest: noopIngest(&calls),
		now: sat711, out: io.Discard, errOut: io.Discard,
	}
	require.NoError(t, executeCrisisEvalDaily(ctx, deps, "")) // 静默 exit 0
	sys, err := st.LatestSystemEval(ctx)
	require.NoError(t, err)
	assert.Nil(t, sys) // 未落任何评估行
}

// boundary:仅四个必备序列齐备,move/usdjpy/nfci 缺 target 观测 → 仍正常评估。
func TestExecuteCrisisEvalDailyMoveMissing(t *testing.T) {
	st := newCrisisTestStore(t)
	ctx := context.Background()
	const target = "2026-07-10"
	seedIndicators(t, st, target, 80, map[string]float64{
		crisis.IndVIX: 15, crisis.IndSOFREFFR: -10, crisis.IndHYOAS: 400, crisis.IndT10Y2Y: 35,
	})

	var calls int
	deps := crisisEvalDeps{
		cfg: crisisTestConfig(), store: st, ingest: noopIngest(&calls),
		now: sat711, out: io.Discard, errOut: io.Discard,
	}
	require.NoError(t, executeCrisisEvalDaily(ctx, deps, ""))
	sys, err := st.LatestSystemEval(ctx)
	require.NoError(t, err)
	require.NotNil(t, sys) // 缺 move/usdjpy 不阻断,系统行照落
	assert.Equal(t, crisis.StateNormal, sys.SystemState)
}

// error_handling:注入 ingest 失败 → 返回非零错误(区别于 not ready 空跑)。
func TestExecuteCrisisEvalDailyIngestError(t *testing.T) {
	st := newCrisisTestStore(t)
	ctx := context.Background()
	deps := crisisEvalDeps{
		cfg: crisisTestConfig(), store: st,
		ingest: func(context.Context, string, string) (*crisis.IngestReport, error) {
			return nil, errors.New("fred boom")
		},
		now: sat711, out: io.Discard, errOut: io.Discard,
	}
	err := executeCrisisEvalDaily(ctx, deps, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}

func TestExecuteCrisisStatus(t *testing.T) {
	st := newCrisisTestStore(t)
	ctx := context.Background()

	var buf strings.Builder
	require.NoError(t, executeCrisisStatus(ctx, st, &buf))
	assert.Contains(t, buf.String(), "no evaluations yet")

	seedObservations(t, st, "2026-07-10", 80)
	deps := crisisEvalDeps{
		cfg: crisisTestConfig(), store: st, ingest: noopIngest(new(int)),
		now: sat711, out: io.Discard, errOut: io.Discard,
	}
	require.NoError(t, executeCrisisEvalDaily(ctx, deps, ""))

	buf.Reset()
	require.NoError(t, executeCrisisStatus(ctx, st, &buf))
	out := buf.String()
	assert.Contains(t, out, "NORMAL")
	assert.Contains(t, out, "vix")
}

// runCrisisEval / runCrisisStatus 的 openCrisisStore 错误路径(配置缺失)。
func TestRunCrisisEvalStatusConfigError(t *testing.T) {
	snapshotCrisisFlags(t)
	crisisCfgPath = filepath.Join(t.TempDir(), "nope.yaml")

	c := newDiscardCmd()
	require.Error(t, runCrisisStatus(c, nil))
	require.Error(t, runCrisisEval(c, nil))
}

// writeTempCrisisConfigDB 同 writeTempCrisisConfig,但同时回传替换后的 db 路径,
// 供需要预置库内容的 runCrisisEval 用例复用。
func writeTempCrisisConfigDB(t *testing.T) (cfgPath, dbPath string) {
	t.Helper()
	raw, err := os.ReadFile("../../configs/crisis-monitor.yaml")
	require.NoError(t, err)
	dbPath = filepath.Join(t.TempDir(), "crisis.db")
	out := strings.Replace(string(raw), "data/crisis.db", dbPath, 1)
	cfgPath = filepath.Join(t.TempDir(), "crisis-monitor.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(out), 0o644))
	return
}

func newDiscardCmd() *cobra.Command {
	c := &cobra.Command{}
	c.SetContext(context.Background())
	c.SetOut(io.Discard)
	c.SetErr(io.Discard)
	return c
}

// runCrisisEval 的三条离线可测分支:key 缺失 / unknown mode / daily 幂等短路(不触网)。
func TestRunCrisisEvalModes(t *testing.T) {
	t.Run("fred key missing", func(t *testing.T) {
		snapshotCrisisFlags(t)
		cfgPath, _ := writeTempCrisisConfigDB(t)
		crisisCfgPath, cfgFile = cfgPath, ""
		t.Setenv("FRED_API_KEY", "")
		evalMode, evalDate = "daily", ""
		err := runCrisisEval(newDiscardCmd(), nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "FRED key")
	})

	t.Run("unknown mode", func(t *testing.T) {
		snapshotCrisisFlags(t)
		cfgPath, _ := writeTempCrisisConfigDB(t)
		crisisCfgPath = cfgPath
		t.Setenv("FRED_API_KEY", "k")
		evalMode = "bogus"
		err := runCrisisEval(newDiscardCmd(), nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown")
	})

	t.Run("daily idempotent skips network", func(t *testing.T) {
		snapshotCrisisFlags(t)
		cfgPath, dbPath := writeTempCrisisConfigDB(t)

		// 预置同一 db 的 target 评估行 → runCrisisEval daily 幂等短路,不调 ingest。
		st, err := crisis.NewStore(dbPath)
		require.NoError(t, err)
		seedObservations(t, st, "2026-07-10", 80)
		deps := crisisEvalDeps{cfg: crisisTestConfig(), store: st, ingest: noopIngest(new(int)),
			now: sat711, out: io.Discard, errOut: io.Discard}
		require.NoError(t, executeCrisisEvalDaily(context.Background(), deps, ""))
		require.NoError(t, st.Close())

		crisisCfgPath = cfgPath
		t.Setenv("FRED_API_KEY", "k")
		evalMode, evalDate = "daily", "2026-07-10"
		require.NoError(t, runCrisisEval(newDiscardCmd(), nil)) // already evaluated,无网络
	})
}

func TestMustAddDays(t *testing.T) {
	assert.Equal(t, "2026-06-25", mustAddDays("2026-07-10", -15))
	assert.Equal(t, "not-a-date", mustAddDays("not-a-date", -45)) // 坏日期原样返回
}

// printDayResult 的状态迁移分支(PrevState != State 打印 "→")。
func TestPrintDayResultTransition(t *testing.T) {
	var buf bytes.Buffer
	printDayResult(&buf, &crisis.DayResult{
		Date: "2026-07-10", PrevState: crisis.StateNormal, State: crisis.StateWatch,
		Results: map[string]crisis.IndicatorResult{},
	})
	assert.Contains(t, buf.String(), "→")
	assert.Contains(t, buf.String(), "WATCH")
}

// stateStreakDays 在遇到不同系统态时中断计数。
func TestStateStreakDaysBreak(t *testing.T) {
	st := newCrisisTestStore(t)
	ctx := context.Background()
	require.NoError(t, st.AppendEvaluations(ctx, []crisis.Evaluation{
		{TS: "2026-07-10", EvalAt: "2026-07-10T00:00:00.000000000Z", Indicator: "", SystemState: crisis.StateNormal},
		{TS: "2026-07-09", EvalAt: "2026-07-09T00:00:00.000000000Z", Indicator: "", SystemState: crisis.StateWatch},
	}))
	n, err := stateStreakDays(ctx, st, crisis.StateNormal)
	require.NoError(t, err)
	assert.Equal(t, 1, n) // 7/10 NORMAL 计入,遇 7/09 WATCH 中断
}
