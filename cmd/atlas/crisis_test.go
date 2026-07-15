package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
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

	_ "modernc.org/sqlite"

	"github.com/newthinker/atlas/internal/core"
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
	pRFrom, pRTo, pRJSON := replayFrom, replayTo, replayJSON
	t.Cleanup(func() {
		crisisCfgPath, backfillFrom, backfillTo = pCfg, pFrom, pTo
		backfillCSV, backfillIndicator, backfillScale, cfgFile = pCSV, pInd, pScale, pFile
		replayFrom, replayTo, replayJSON = pRFrom, pRTo, pRJSON
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
// functional[3] "nfci 模式仅 IngestNFCI 不评估;status 打印状态/持续/指标"  → TestExecuteCrisisEvalNFCI / TestExecuteCrisisStatus
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

func TestExecuteCrisisEvalNFCI(t *testing.T) {
	st := newCrisisTestStore(t)
	ctx := context.Background()

	var calls int
	var gotFrom, gotTo string
	var buf strings.Builder
	deps := crisisEvalDeps{
		cfg: crisisTestConfig(), store: st,
		ingestNFCI: func(_ context.Context, from, to string) (int, error) {
			calls++
			gotFrom, gotTo = from, to
			return 3, nil
		},
		now: sat711, out: &buf, errOut: io.Discard,
	}
	require.NoError(t, executeCrisisEvalNFCI(ctx, deps))

	assert.Equal(t, 1, calls) // 仅刷新一次
	assert.Equal(t, "2026-06-11", gotFrom)
	assert.Equal(t, "2026-07-11", gotTo) // now−30d..today 窗口
	assert.Contains(t, buf.String(), "nfci refreshed: 3 rows")

	// 不评估:不产生任何系统评估行(设计 §3.2 条 4,NFCI 参与后续 daily)
	sys, err := st.LatestSystemEval(ctx)
	require.NoError(t, err)
	assert.Nil(t, sys)

	// 采集失败透传非零错误
	deps.ingestNFCI = func(context.Context, string, string) (int, error) {
		return 0, errors.New("nfci boom")
	}
	err = executeCrisisEvalNFCI(ctx, deps)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nfci boom")
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

// ---- TASK-013: crisis replay ----
// done_criteria → test mapping
// functional[0] "EvalDates 逐日 EvalDay + MemHistory 累积,不写 crisis_evaluations" → TestExecuteCrisisReplayTransitions(含 LatestSystemEval 仍 nil 的不写库判别断言)
// functional[1] "输出状态转移时间线 + final state + 各态进入次数;--json 机器可读"    → TestExecuteCrisisReplayTransitions / TestExecuteCrisisReplayJSON
// functional[2] "80 日全绿 + 末 3 日 NFCI 转正 → 一次 NORMAL→WATCH 且 final WATCH"  → TestExecuteCrisisReplayTransitions
// boundary[0]   "区间内无 vix 观测 → 返回含 run backfill first 的错误"               → TestExecuteCrisisReplayNoData

// seedReplayWatch 铺 80 日全绿 + 末 3 日 NFCI 转正(领先层红),用于触发一次 NORMAL→WATCH。
func seedReplayWatch(t *testing.T, st *crisis.Store) {
	t.Helper()
	ctx := context.Background()
	seedObservations(t, st, "2026-07-10", 80)
	var red []crisis.Observation
	for _, d := range []string{"2026-07-08", "2026-07-09", "2026-07-10"} {
		red = append(red, crisis.Observation{Date: d, Indicator: crisis.IndNFCI, Value: 0.2,
			Source: "test", FetchedAt: "2026-07-11T00:00:00.000000000Z"})
	}
	require.NoError(t, st.UpsertObservations(ctx, red))
}

func TestExecuteCrisisReplayTransitions(t *testing.T) {
	st := newCrisisTestStore(t)
	ctx := context.Background()
	seedReplayWatch(t, st)

	var buf bytes.Buffer
	require.NoError(t, executeCrisisReplay(ctx, crisisTestConfig(), st, "2026-06-25", "2026-07-10", false, &buf))
	out := buf.String()
	assert.Contains(t, out, "NORMAL → WATCH")
	assert.Contains(t, out, "final state: WATCH")
	assert.Contains(t, out, "entered WATCH")

	// 回放零落库:真相源不被回测污染
	sys, err := st.LatestSystemEval(ctx)
	require.NoError(t, err)
	assert.Nil(t, sys)
}

// --json:每条转移一行 JSON,可解析且含 date/from/to/amber_count。
func TestExecuteCrisisReplayJSON(t *testing.T) {
	st := newCrisisTestStore(t)
	ctx := context.Background()
	seedReplayWatch(t, st)

	var buf bytes.Buffer
	require.NoError(t, executeCrisisReplay(ctx, crisisTestConfig(), st, "2026-06-25", "2026-07-10", true, &buf))

	var parsed int
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if !strings.HasPrefix(line, "{") {
			continue // final-state 汇总行非 JSON
		}
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &m))
		assert.Equal(t, "WATCH", m["to"])
		assert.Contains(t, m, "date")
		assert.Contains(t, m, "amber_count")
		parsed++
	}
	assert.Equal(t, 1, parsed) // 恰一次 NORMAL→WATCH 转移
}

func TestExecuteCrisisReplayNoData(t *testing.T) {
	st := newCrisisTestStore(t)
	var buf bytes.Buffer
	err := executeCrisisReplay(context.Background(), crisisTestConfig(), st, "2008-01-01", "2008-12-31", false, &buf)
	require.ErrorContains(t, err, "run backfill first")
}

// runCrisisReplay wrapper:必填校验 / 配置错误 / 经 seeded db 委托成功。
func TestRunCrisisReplay(t *testing.T) {
	t.Run("missing flags", func(t *testing.T) {
		snapshotCrisisFlags(t)
		replayFrom, replayTo = "", ""
		require.Error(t, runCrisisReplay(newDiscardCmd(), nil))
	})

	t.Run("config error", func(t *testing.T) {
		snapshotCrisisFlags(t)
		crisisCfgPath = filepath.Join(t.TempDir(), "nope.yaml")
		replayFrom, replayTo = "2026-06-25", "2026-07-10"
		require.Error(t, runCrisisReplay(newDiscardCmd(), nil))
	})

	t.Run("delegates over seeded db", func(t *testing.T) {
		snapshotCrisisFlags(t)
		cfgPath, dbPath := writeTempCrisisConfigDB(t)
		st, err := crisis.NewStore(dbPath)
		require.NoError(t, err)
		seedReplayWatch(t, st)
		require.NoError(t, st.Close())

		crisisCfgPath = cfgPath
		replayFrom, replayTo, replayJSON = "2026-06-25", "2026-07-10", false
		var buf bytes.Buffer
		c := newDiscardCmd()
		c.SetOut(&buf)
		require.NoError(t, runCrisisReplay(c, nil))
		assert.Contains(t, buf.String(), "final state")
	})
}

// stubSender 捕获发送文本并可注入失败，用于 eval 通知接线测试。
type stubSender struct {
	err  error
	sent []string
}

func (s *stubSender) SendText(text string) error {
	s.sent = append(s.sent, text)
	return s.err
}

func TestSummaryDue(t *testing.T) {
	assert.True(t, summaryDue("2026-07-01", crisis.StateNormal))   // 周三 = 当月首交易日 → 月报
	assert.False(t, summaryDue("2026-07-02", crisis.StateNormal))
	assert.True(t, summaryDue("2026-08-03", crisis.StateNormal))   // 8/1 周六 → 首交易日 = 8/3 周一
	assert.True(t, summaryDue("2026-07-13", crisis.StateWatch))    // 周一 → 周报
	assert.False(t, summaryDue("2026-07-14", crisis.StateWatch))
	assert.False(t, summaryDue("2026-07-13", crisis.StateBrewing)) // BREWING 走日报，不走摘要
	assert.False(t, summaryDue("bad-date", crisis.StateNormal))    // 坏日期不发
	assert.False(t, summaryDue("2026-07-04", crisis.StateNormal))  // 周六 → 非交易日，非首交易日
}

// buildCrisisSender 在无 telegram 配置时返回 nil（eval 退化为打印）。
func TestBuildCrisisSenderNoConfig(t *testing.T) {
	assert.Nil(t, buildCrisisSender())
}

// 状态变更日经 Sender 发送通知（functional[2]）。
func TestExecuteCrisisEvalDailySendsNotification(t *testing.T) {
	st := newCrisisTestStore(t)
	ctx := context.Background()
	const target = "2026-07-10"
	vals := map[string]float64{
		crisis.IndVIX: 15, crisis.IndMOVE: 70, crisis.IndSOFREFFR: -10, crisis.IndHYOAS: 400,
		crisis.IndT10Y2Y: 35, crisis.IndNFCI: 0.1, crisis.IndUSDJPY: 150, // nfci>0 领先红 → NORMAL→WATCH
	}
	seedIndicators(t, st, target, 80, vals)

	sender := &stubSender{}
	var calls int
	deps := crisisEvalDeps{
		cfg: crisisTestConfig(), store: st, ingest: noopIngest(&calls),
		now: sat711, out: io.Discard, errOut: io.Discard, sender: sender,
	}
	require.NoError(t, executeCrisisEvalDaily(ctx, deps, ""))
	require.Len(t, sender.sent, 1)
	assert.True(t, strings.HasPrefix(sender.sent[0], "[P1]"))
	assert.Contains(t, sender.sent[0], "NORMAL → WATCH")
}

// 发送失败仅记 stderr 不失败退出（error_handling[0]）。
func TestExecuteCrisisEvalDailyNotifyFailureDoesNotAbort(t *testing.T) {
	st := newCrisisTestStore(t)
	ctx := context.Background()
	const target = "2026-07-10"
	vals := map[string]float64{
		crisis.IndVIX: 15, crisis.IndMOVE: 70, crisis.IndSOFREFFR: -10, crisis.IndHYOAS: 400,
		crisis.IndT10Y2Y: 35, crisis.IndNFCI: 0.1, crisis.IndUSDJPY: 150,
	}
	seedIndicators(t, st, target, 80, vals)

	var errBuf bytes.Buffer
	var calls int
	deps := crisisEvalDeps{
		cfg: crisisTestConfig(), store: st, ingest: noopIngest(&calls),
		now: sat711, out: io.Discard, errOut: &errBuf,
		sender: &stubSender{err: errors.New("telegram down")},
	}
	require.NoError(t, executeCrisisEvalDaily(ctx, deps, "")) // 不失败退出
	assert.Contains(t, errBuf.String(), "notify failed")
	// 评估已落库（文件真相源，通知丢失可自愈）
	sys, err := st.LatestSystemEval(ctx)
	require.NoError(t, err)
	require.NotNil(t, sys)
	assert.Equal(t, crisis.StateWatch, sys.SystemState)
}

// sender 未配置（nil）时通知打印到 out，便于本地试运行。
func TestExecuteCrisisEvalDailyNilSenderPrints(t *testing.T) {
	st := newCrisisTestStore(t)
	ctx := context.Background()
	const target = "2026-07-10"
	vals := map[string]float64{
		crisis.IndVIX: 15, crisis.IndMOVE: 70, crisis.IndSOFREFFR: -10, crisis.IndHYOAS: 400,
		crisis.IndT10Y2Y: 35, crisis.IndNFCI: 0.1, crisis.IndUSDJPY: 150,
	}
	seedIndicators(t, st, target, 80, vals)

	var out bytes.Buffer
	var calls int
	deps := crisisEvalDeps{
		cfg: crisisTestConfig(), store: st, ingest: noopIngest(&calls),
		now: sat711, out: &out, errOut: io.Discard, // sender 缺省 nil
	}
	require.NoError(t, executeCrisisEvalDaily(ctx, deps, ""))
	assert.Contains(t, out.String(), "[P1]")
}

func TestStaleIndicators(t *testing.T) {
	res := &crisis.DayResult{Results: map[string]crisis.IndicatorResult{}}
	for _, ind := range crisis.AllIndicators {
		res.Results[ind] = crisis.IndicatorResult{Indicator: ind, Status: crisis.StatusGreen}
	}
	mv := res.Results[crisis.IndMOVE]
	mv.Status = crisis.StatusStale
	res.Results[crisis.IndMOVE] = mv

	assert.Equal(t, []string{crisis.IndMOVE}, staleIndicators(res))
}

// seedBrewing 预置一条 BREWING 系统行，使 intraday 进入告警评估路径。
func seedBrewing(t *testing.T, st *crisis.Store, date string) {
	t.Helper()
	require.NoError(t, st.AppendEvaluations(context.Background(), []crisis.Evaluation{{
		TS: date, EvalAt: "2026-07-10T00:00:00.000000000Z", Indicator: "",
		SystemState: crisis.StateBrewing, Detail: "{}",
	}}))
}

func intradayDeps(st *crisis.Store) crisisEvalDeps {
	return crisisEvalDeps{
		cfg: crisisTestConfig(), store: st,
		now:    func() time.Time { return time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC) },
		out:    io.Discard, errOut: io.Discard,
	}
}

func TestExecuteCrisisIntraday(t *testing.T) {
	st := newCrisisTestStore(t)
	ctx := context.Background()
	deps := intradayDeps(st)
	var quoteCalls int
	quote := func(string) (*core.Quote, error) {
		quoteCalls++
		return &core.Quote{Price: 145}, nil
	}

	// 非 BREWING/CRISIS → 空跑退出，连行情都不取（设计 §4.3：空跑成本近零）
	require.NoError(t, executeCrisisIntraday(ctx, deps, quote))
	assert.Equal(t, 0, quoteCalls)

	// BREWING 态 + 5 观测前收盘 150、现价 145 → wow=−3.3% ≤ −3% → 告警一次
	seedIndicators(t, st, "2026-07-09", 10, map[string]float64{crisis.IndUSDJPY: 150})
	seedBrewing(t, st, "2026-07-09")
	require.NoError(t, executeCrisisIntraday(ctx, deps, quote))
	assert.Equal(t, 1, quoteCalls)
	sent, err := st.HasIndicatorEvalForDate(ctx, "usdjpy_intraday", "2026-07-10")
	require.NoError(t, err)
	assert.True(t, sent)

	// 同日第二次唤起 → 每日一次去重，不再取行情
	require.NoError(t, executeCrisisIntraday(ctx, deps, quote))
	assert.Equal(t, 1, quoteCalls)
}

// error_handling：FetchQuote 失败 → 返回非 nil error 且不写去重行（下次可重试）。
func TestExecuteCrisisIntradayQuoteError(t *testing.T) {
	st := newCrisisTestStore(t)
	ctx := context.Background()
	deps := intradayDeps(st)
	seedIndicators(t, st, "2026-07-09", 10, map[string]float64{crisis.IndUSDJPY: 150})
	seedBrewing(t, st, "2026-07-09")

	quote := func(string) (*core.Quote, error) { return nil, errors.New("yahoo down") }
	require.Error(t, executeCrisisIntraday(ctx, deps, quote))

	sent, err := st.HasIndicatorEvalForDate(ctx, "usdjpy_intraday", "2026-07-10")
	require.NoError(t, err)
	assert.False(t, sent) // 未写去重行 → 下次触发可重试
}

// boundary：不足 5 观测 或 5 观测前收盘为 0 → 静默跳过不告警、不写去重行。
func TestExecuteCrisisIntradaySilentSkips(t *testing.T) {
	quote := func(string) (*core.Quote, error) { return &core.Quote{Price: 145}, nil }

	t.Run("insufficient history", func(t *testing.T) {
		st := newCrisisTestStore(t)
		ctx := context.Background()
		seedIndicators(t, st, "2026-07-09", 3, map[string]float64{crisis.IndUSDJPY: 150}) // 仅 3 观测
		seedBrewing(t, st, "2026-07-09")
		require.NoError(t, executeCrisisIntraday(ctx, intradayDeps(st), quote))
		sent, err := st.HasIndicatorEvalForDate(ctx, "usdjpy_intraday", "2026-07-10")
		require.NoError(t, err)
		assert.False(t, sent)
	})

	t.Run("zero base close", func(t *testing.T) {
		st := newCrisisTestStore(t)
		ctx := context.Background()
		// 5 观测但最早一条(=win[0]，5 观测前收盘)为 0
		require.NoError(t, st.UpsertObservations(ctx, []crisis.Observation{
			{Date: "2026-07-06", Indicator: crisis.IndUSDJPY, Value: 0, Source: "test", FetchedAt: "x"},
			{Date: "2026-07-07", Indicator: crisis.IndUSDJPY, Value: 150, Source: "test", FetchedAt: "x"},
			{Date: "2026-07-08", Indicator: crisis.IndUSDJPY, Value: 150, Source: "test", FetchedAt: "x"},
			{Date: "2026-07-09", Indicator: crisis.IndUSDJPY, Value: 150, Source: "test", FetchedAt: "x"},
			{Date: "2026-07-10", Indicator: crisis.IndUSDJPY, Value: 150, Source: "test", FetchedAt: "x"},
		}))
		seedBrewing(t, st, "2026-07-09")
		require.NoError(t, executeCrisisIntraday(ctx, intradayDeps(st), quote))
		sent, err := st.HasIndicatorEvalForDate(ctx, "usdjpy_intraday", "2026-07-10")
		require.NoError(t, err)
		assert.False(t, sent)
	})
}

// wow 未触红（现价高于基期）→ 不告警、不写去重行。
func TestExecuteCrisisIntradayNoTrigger(t *testing.T) {
	st := newCrisisTestStore(t)
	ctx := context.Background()
	seedIndicators(t, st, "2026-07-09", 10, map[string]float64{crisis.IndUSDJPY: 150})
	seedBrewing(t, st, "2026-07-09")

	quote := func(string) (*core.Quote, error) { return &core.Quote{Price: 149}, nil } // wow≈−0.7% > −3%
	require.NoError(t, executeCrisisIntraday(ctx, intradayDeps(st), quote))
	sent, err := st.HasIndicatorEvalForDate(ctx, "usdjpy_intraday", "2026-07-10")
	require.NoError(t, err)
	assert.False(t, sent)
}

// 配置了 Sender 时触发告警经 SendText 发送（成功路径）。
func TestExecuteCrisisIntradaySendsViaSender(t *testing.T) {
	st := newCrisisTestStore(t)
	ctx := context.Background()
	seedIndicators(t, st, "2026-07-09", 10, map[string]float64{crisis.IndUSDJPY: 150})
	seedBrewing(t, st, "2026-07-09")

	sender := &stubSender{}
	deps := intradayDeps(st)
	deps.sender = sender
	quote := func(string) (*core.Quote, error) { return &core.Quote{Price: 145}, nil } // wow=−3.3%

	require.NoError(t, executeCrisisIntraday(ctx, deps, quote))
	require.Len(t, sender.sent, 1)
	assert.True(t, strings.HasPrefix(sender.sent[0], "[P0]"))
	assert.Contains(t, sender.sent[0], "carry trade")
}

// Sender 发送失败仅记 stderr 不失败退出，且去重行已落库（不重复告警）。
func TestExecuteCrisisIntradaySendFailureDoesNotAbort(t *testing.T) {
	st := newCrisisTestStore(t)
	ctx := context.Background()
	seedIndicators(t, st, "2026-07-09", 10, map[string]float64{crisis.IndUSDJPY: 150})
	seedBrewing(t, st, "2026-07-09")

	var errBuf bytes.Buffer
	deps := intradayDeps(st)
	deps.sender = &stubSender{err: errors.New("telegram down")}
	deps.errOut = &errBuf
	quote := func(string) (*core.Quote, error) { return &core.Quote{Price: 145}, nil }

	require.NoError(t, executeCrisisIntraday(ctx, deps, quote))
	assert.Contains(t, errBuf.String(), "notify failed")
	sent, err := st.HasIndicatorEvalForDate(ctx, "usdjpy_intraday", "2026-07-10")
	require.NoError(t, err)
	assert.True(t, sent) // 去重行先落库，避免下次重复告警
}

// Context Checkpoint: done_criteria → test mapping (TASK-008 buildNotifyContext)
// functional[0] PrevDay 取昨日行而非当日            → TestBuildNotifyContext (PrevDay[vix]==GREEN)
// functional[1] StateDays 非变更=streak+1 / 变更=前状态streak
//                                                   → TestBuildNotifyContext(=2) / TestBuildNotifyContextTransitionAndTrends(=2)
// functional[2] NewStale 去重 + StaleLastObs 缺省/命中
//                                                   → TestBuildNotifyContext(nil obs) / TestBuildNotifyContextStaleLastObs(命中)
// functional[3] ClearStreak WATCH∧SummaryDue∧!AnyTrigger 三条件
//                                                   → TestBuildNotifyContext(=2) + TestBuildNotifyContextClearStreakConditions(逐条否定=0)
// functional[4] Trends 仅 SummaryDue∧NORMAL 升序 Delta=末-首、无观测不进 map
//                                                   → TestBuildNotifyContextTransitionAndTrends
// error_handling[0] RecentIndicatorEvals/LatestObservation/SeriesWindow 错误原样上抛
//                                                   → TestBuildNotifyContextStoreErrors（stateStreakDays/History 二查询被
//                                                     首个 RecentIndicatorEvals 结构性遮蔽，见 discovery，无法在具体 store 下独立触发）

func newNotifyTestDeps(t *testing.T) crisisEvalDeps {
	t.Helper()
	return newNotifyDepsAt(t, filepath.Join(t.TempDir(), "crisis.db"))
}

// newNotifyDepsAt 暴露 db 路径，供错误路径用例从旁路连接删表隔离故障。
func newNotifyDepsAt(t *testing.T, dbPath string) crisisEvalDeps {
	t.Helper()
	st, err := crisis.NewStore(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { st.Close() })
	cfg := &crisis.Config{
		Freshness:    crisis.FreshnessCfg{DailyMaxLagDays: 4, WeeklyMaxLagDays: 12},
		StateMachine: crisis.StateMachineCfg{WatchAmberCount: 3, CrisisExitDays: 10, WatchExitDays: 20, BrewingExitDays: 10, DemoteHysteresisDays: 3},
	}
	return crisisEvalDeps{cfg: cfg, store: st, now: time.Now, out: io.Discard, errOut: io.Discard}
}

func notifyDayResult(date string, prev, cur crisis.SystemState) *crisis.DayResult {
	results := map[string]crisis.IndicatorResult{}
	for _, ind := range crisis.AllIndicators {
		results[ind] = crisis.IndicatorResult{Indicator: ind, Status: crisis.StatusGreen, RawStatus: crisis.StatusGreen, Value: 1}
	}
	return &crisis.DayResult{Date: date, Results: results, PrevState: prev, State: cur}
}

// dropMacroObs 从旁路连接删 macro_observations 表：crisis_evaluations 保持可查，
// 使 LatestObservation/SeriesWindow 的错误路径可在不触发前置查询失败的前提下独立触发。
func dropMacroObs(t *testing.T, dbPath string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=busy_timeout(5000)")
	require.NoError(t, err)
	defer db.Close()
	_, err = db.Exec("DROP TABLE macro_observations")
	require.NoError(t, err)
}

// buildNotifyContext：PrevDay 取前一评估日行、NewStale 去重、StateDays 语义、
// ClearStreak 与 Trends 的按需组装（通知设计 §8，测试要点 5）。
func TestBuildNotifyContext(t *testing.T) {
	d := newNotifyTestDeps(t)
	ctx := context.Background()

	// 昨日（07-17）：全绿 WATCH 系统行 + vix/move 指标行；move 昨日已 STALE
	sysDetail := `{"date":"2026-07-17","any_trigger":false,"brewing_pair":false,"amber_count":0,"prev":"WATCH"}`
	require.NoError(t, d.store.AppendEvaluations(ctx, []crisis.Evaluation{
		{TS: "2026-07-17", EvalAt: "2026-07-18T01:00:00.000000000Z", Indicator: crisis.IndVIX, Status: crisis.StatusGreen, Value: 15},
		{TS: "2026-07-17", EvalAt: "2026-07-18T01:00:00.000000000Z", Indicator: crisis.IndMOVE, Status: crisis.StatusStale, Value: 88},
		{TS: "2026-07-17", EvalAt: "2026-07-18T01:00:00.000000000Z", Indicator: "", SystemState: crisis.StateWatch, Detail: sysDetail},
	}))
	require.NoError(t, d.store.UpsertObservations(ctx, []crisis.Observation{
		{Date: "2026-07-13", Indicator: crisis.IndMOVE, Value: 88, Source: "yahoo", FetchedAt: "2026-07-13T00:00:00.000000000Z"},
	}))

	// 今日（07-20，周一 → WATCH 周报到期）：move 持续 STALE，vix 新进入 STALE
	res := notifyDayResult("2026-07-20", crisis.StateWatch, crisis.StateWatch)
	setStale := func(ind string) {
		r := res.Results[ind]
		r.Status = crisis.StatusStale
		res.Results[ind] = r
	}
	setStale(crisis.IndMOVE)
	setStale(crisis.IndVIX)
	res.Detail = crisis.SysDetail{AnyTrigger: false}

	nc, err := buildNotifyContext(ctx, d, res)
	require.NoError(t, err)
	assert.Equal(t, crisis.StatusGreen, nc.PrevDay[crisis.IndVIX].Status) // 昨日行而非当日
	assert.Equal(t, []string{crisis.IndVIX}, nc.NewStale)                 // move 昨日已 STALE → 不重复（要点 5）
	assert.Equal(t, 2, nc.StateDays)                                      // 昨日 1 行 WATCH + 今日
	assert.True(t, nc.SummaryDue)                                         // 周一 + WATCH
	assert.Equal(t, 2, nc.ClearStreak)                                    // 昨日 any_trigger=false + 今日
	assert.Nil(t, nc.Trends)                                              // 非 NORMAL 月报 → 不组装

	// vix 无观测 → StaleLastObs 缺省；move 持续 STALE 不进 NewStale 故也不查
	_, ok := nc.StaleLastObs[crisis.IndVIX]
	assert.False(t, ok)
}

// 变更日 StateDays = 前状态持续日数（补充决策 6）；NORMAL 月报日组装 Trends。
func TestBuildNotifyContextTransitionAndTrends(t *testing.T) {
	d := newNotifyTestDeps(t)
	ctx := context.Background()

	// 历史：两行 WATCH 系统行
	for _, ts := range []string{"2026-07-16", "2026-07-17"} {
		require.NoError(t, d.store.AppendEvaluations(ctx, []crisis.Evaluation{
			{TS: ts, EvalAt: ts + "T23:00:00.000000000Z", Indicator: "", SystemState: crisis.StateWatch,
				Detail: `{"any_trigger":true,"prev":"WATCH"}`},
		}))
	}
	res := notifyDayResult("2026-07-20", crisis.StateWatch, crisis.StateBrewing) // 升级日
	nc, err := buildNotifyContext(ctx, d, res)
	require.NoError(t, err)
	assert.Equal(t, 2, nc.StateDays) // 前状态 WATCH 的历史持续 = 2

	// NORMAL + 当月首个交易日（2026-08-03 周一）→ Trends 组装、窗口 ≤21 升序
	d2 := newNotifyTestDeps(t)
	var obs []crisis.Observation
	for i := 0; i < 25; i++ {
		obs = append(obs, crisis.Observation{Date: mustAddDays("2026-08-03", -i), Indicator: crisis.IndVIX,
			Value: float64(20 - i), Source: "fred", FetchedAt: "2026-08-03T00:00:00.000000000Z"})
	}
	require.NoError(t, d2.store.UpsertObservations(ctx, obs))
	res2 := notifyDayResult("2026-08-03", crisis.StateNormal, crisis.StateNormal)
	nc2, err := buildNotifyContext(ctx, d2, res2)
	require.NoError(t, err)
	require.NotNil(t, nc2.Trends)
	vix := nc2.Trends[crisis.IndVIX]
	assert.Len(t, vix.Window, 21)
	assert.InDelta(t, vix.Window[len(vix.Window)-1].Value-vix.Window[0].Value, vix.Delta, 1e-9)
	_, hasMove := nc2.Trends[crisis.IndMOVE] // 无观测指标不进 map（月报省略该行）
	assert.False(t, hasMove)
}

// StaleLastObs 命中分支：今日新进 STALE 且有观测 → 记最后观测日（补充决策 1）。
func TestBuildNotifyContextStaleLastObs(t *testing.T) {
	d := newNotifyTestDeps(t)
	ctx := context.Background()
	require.NoError(t, d.store.AppendEvaluations(ctx, []crisis.Evaluation{
		{TS: "2026-07-17", EvalAt: "2026-07-18T01:00:00.000000000Z", Indicator: crisis.IndVIX, Status: crisis.StatusGreen, Value: 15},
	}))
	require.NoError(t, d.store.UpsertObservations(ctx, []crisis.Observation{
		{Date: "2026-07-11", Indicator: crisis.IndVIX, Value: 15, Source: "fred", FetchedAt: "2026-07-11T00:00:00.000000000Z"},
		{Date: "2026-07-14", Indicator: crisis.IndVIX, Value: 16, Source: "fred", FetchedAt: "2026-07-14T00:00:00.000000000Z"},
	}))
	res := notifyDayResult("2026-07-20", crisis.StateWatch, crisis.StateWatch)
	r := res.Results[crisis.IndVIX]
	r.Status = crisis.StatusStale
	res.Results[crisis.IndVIX] = r
	res.Detail = crisis.SysDetail{AnyTrigger: false}

	nc, err := buildNotifyContext(ctx, d, res)
	require.NoError(t, err)
	assert.Equal(t, []string{crisis.IndVIX}, nc.NewStale)
	assert.Equal(t, "2026-07-14", nc.StaleLastObs[crisis.IndVIX]) // 最后观测日（LatestObservation ts DESC）
}

// ClearStreak 三条件（WATCH ∧ SummaryDue ∧ !AnyTrigger）逐条独立否定 → 计数留 0。
func TestBuildNotifyContextClearStreakConditions(t *testing.T) {
	ctx := context.Background()
	seedClearHistory := func(d crisisEvalDeps) {
		require.NoError(t, d.store.AppendEvaluations(ctx, []crisis.Evaluation{
			{TS: "2026-07-17", EvalAt: "2026-07-18T01:00:00.000000000Z", Indicator: "",
				SystemState: crisis.StateWatch, Detail: `{"any_trigger":false,"prev":"WATCH"}`},
		}))
	}

	// 否定 WATCH：NORMAL 月报日（SummaryDue 真）∧ !AnyTrigger，但非 WATCH → 块跳过
	dNotWatch := newNotifyTestDeps(t)
	seedClearHistory(dNotWatch)
	resN := notifyDayResult("2026-08-03", crisis.StateNormal, crisis.StateNormal)
	resN.Detail = crisis.SysDetail{AnyTrigger: false}
	ncN, err := buildNotifyContext(ctx, dNotWatch, resN)
	require.NoError(t, err)
	assert.True(t, ncN.SummaryDue)
	assert.Equal(t, 0, ncN.ClearStreak)

	// 否定 SummaryDue：WATCH ∧ 周二（非周一）∧ !AnyTrigger → 块跳过
	dNotDue := newNotifyTestDeps(t)
	seedClearHistory(dNotDue)
	resD := notifyDayResult("2026-07-21", crisis.StateWatch, crisis.StateWatch) // 周二
	resD.Detail = crisis.SysDetail{AnyTrigger: false}
	ncD, err := buildNotifyContext(ctx, dNotDue, resD)
	require.NoError(t, err)
	assert.False(t, ncD.SummaryDue)
	assert.Equal(t, 0, ncD.ClearStreak)

	// 否定 !AnyTrigger：WATCH ∧ 周一 ∧ AnyTrigger=true → 块跳过
	dTrig := newNotifyTestDeps(t)
	seedClearHistory(dTrig)
	resT := notifyDayResult("2026-07-20", crisis.StateWatch, crisis.StateWatch) // 周一
	resT.Detail = crisis.SysDetail{AnyTrigger: true}
	ncT, err := buildNotifyContext(ctx, dTrig, resT)
	require.NoError(t, err)
	assert.True(t, ncT.SummaryDue)
	assert.Equal(t, 0, ncT.ClearStreak)
}

// store 查询错误原样上抛：关库触发 RecentIndicatorEvals；删 macro_observations
// 表（保留 crisis_evaluations）分别触发 LatestObservation 与 SeriesWindow。
func TestBuildNotifyContextStoreErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("RecentIndicatorEvals", func(t *testing.T) {
		d := newNotifyTestDeps(t)
		require.NoError(t, d.store.Close()) // 关库 → 首个 PrevDay 查询即失败
		_, err := buildNotifyContext(ctx, d, notifyDayResult("2026-07-20", crisis.StateWatch, crisis.StateWatch))
		require.Error(t, err)
	})

	t.Run("LatestObservation", func(t *testing.T) {
		dbPath := filepath.Join(t.TempDir(), "crisis.db")
		d := newNotifyDepsAt(t, dbPath)
		require.NoError(t, d.store.AppendEvaluations(ctx, []crisis.Evaluation{
			{TS: "2026-07-17", EvalAt: "2026-07-18T01:00:00.000000000Z", Indicator: crisis.IndVIX, Status: crisis.StatusGreen, Value: 15},
		}))
		dropMacroObs(t, dbPath) // crisis_evaluations 仍可查，NewStale 组装时 LatestObservation 失败
		res := notifyDayResult("2026-07-20", crisis.StateWatch, crisis.StateWatch)
		r := res.Results[crisis.IndVIX]
		r.Status = crisis.StatusStale
		res.Results[crisis.IndVIX] = r
		res.Detail = crisis.SysDetail{AnyTrigger: false}
		_, err := buildNotifyContext(ctx, d, res)
		require.Error(t, err)
	})

	t.Run("SeriesWindow", func(t *testing.T) {
		dbPath := filepath.Join(t.TempDir(), "crisis.db")
		d := newNotifyDepsAt(t, dbPath)
		dropMacroObs(t, dbPath) // 无 STALE 指标 → 直达 Trends 组装的 SeriesWindow 失败
		res := notifyDayResult("2026-08-03", crisis.StateNormal, crisis.StateNormal) // NORMAL 月报日
		_, err := buildNotifyContext(ctx, d, res)
		require.Error(t, err)
	})
}
