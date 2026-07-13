package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
