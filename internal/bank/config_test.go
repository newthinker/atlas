package bank

// Context Checkpoint: done_criteria → test mapping
// functional[0] 合法配置加载/缺省值/DataSymbol/DisplayName/阈值解析/rank_by 映射 → TestLoadConfigValid, TestLoadConfigRankBy
// functional[1] 仓库自带 configs/bank-monitor.yaml 可加载（条目与阈值数值）     → TestShippedConfigLoads
// functional[2] Indicators 顺序/Label/HigherIsWorse；helpers_test.go 提供 day() → TestIndicators, TestDayHelper
// boundary[0]   拒绝表逐条断言错误文案（含 3968.HK 与 03968.HK 判重）          → TestLoadConfigRejects
// boundary[1]   六个阈值字段 × {0, 负数, .nan} 逐个被拒且错误含字段名          → TestLoadConfigRejectsBadThresholds
// error_handling[0] 文件不存在/YAML 语法错/校验失败错误含路径                  → TestLoadConfigMissingFile, TestLoadConfigBadYAML, TestLoadConfigInvalidIncludesPath
// non_functional[0] go vet 无输出、go test 全绿、覆盖率 ≥ 80%                 → 命令行核验

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validThresholds = `
thresholds:
  npl_max: 1.5
  coverage_min: 150
  cet1_min: 8.5
  deterioration: {npl_up: 0.10, coverage_down: 20, cet1_down: 0.50}
`

func writeCfg(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "bank.yaml")
	require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	return p
}

func TestLoadConfigValid(t *testing.T) {
	p := writeCfg(t, `
banks:
  - {market: CN_A, symbol: 600036.SH, name: 招商银行}
  - {market: HK, symbol: 03968.HK, name: 招商银行H, a_share_ref: 600036.SH}
  - {market: CN_A, symbol: 000001.SZ}
`+validThresholds)

	cfg, err := LoadConfig(p)
	require.NoError(t, err)
	assert.Equal(t, "http://127.0.0.1:8180", cfg.Source.AktoolsURL, "缺省 aktools 地址")
	assert.Equal(t, IndNPL, cfg.RankIndicator(), "rank_by 缺省为 npl")
	require.Len(t, cfg.Banks, 3)
	assert.Equal(t, "600036.SH", cfg.Banks[0].DataSymbol())
	assert.Equal(t, "600036.SH", cfg.Banks[1].DataSymbol(), "港股取 a_share_ref")
	assert.Equal(t, "000001.SZ", cfg.Banks[2].DataSymbol(), "CN_A 取 symbol")
	assert.Equal(t, "招商银行", cfg.Banks[0].DisplayName())
	assert.Equal(t, "招商银行H", cfg.Banks[1].DisplayName())
	assert.Equal(t, "000001.SZ", cfg.Banks[2].DisplayName(), "无 name 时回退 symbol")

	th := cfg.Thresholds
	assert.Equal(t, 1.5, th.NPLMax)
	assert.Equal(t, 150.0, th.CoverageMin)
	assert.Equal(t, 8.5, th.CET1Min)
	assert.Equal(t, 0.10, th.Deterioration.NPLUp)
	assert.Equal(t, 20.0, th.Deterioration.CoverageDown)
	assert.Equal(t, 0.5, th.Deterioration.CET1Down)
}

func TestLoadConfigExplicitAktoolsURL(t *testing.T) {
	p := writeCfg(t, "source: {aktools_url: 'http://10.0.0.2:9000'}\nbanks:\n  - {market: CN_A, symbol: 600036.SH}\n"+validThresholds)
	cfg, err := LoadConfig(p)
	require.NoError(t, err)
	assert.Equal(t, "http://10.0.0.2:9000", cfg.Source.AktoolsURL)
}

func TestLoadConfigRankBy(t *testing.T) {
	for key, want := range map[string]Indicator{"npl": IndNPL, "coverage": IndCoverage, "cet1": IndCET1} {
		t.Run(key, func(t *testing.T) {
			p := writeCfg(t, "rank_by: "+key+"\nbanks:\n  - {market: CN_A, symbol: 600036.SH}\n"+validThresholds)
			cfg, err := LoadConfig(p)
			require.NoError(t, err)
			assert.Equal(t, want, cfg.RankIndicator())
		})
	}
}

func TestLoadConfigRejects(t *testing.T) {
	one := "banks:\n  - {market: CN_A, symbol: 600036.SH}\n"
	cases := map[string]struct{ body, want string }{
		"空列表":     {"banks: []\n" + validThresholds, "banks 不能为空"},
		"市场非法":    {"banks:\n  - {market: US, symbol: JPM}\n" + validThresholds, "仅支持 CN_A / HK"},
		"A股格式":    {"banks:\n  - {market: CN_A, symbol: '600036'}\n" + validThresholds, "格式应为 600036.SH"},
		"港股格式":    {"banks:\n  - {market: HK, symbol: 3968, a_share_ref: 600036.SH}\n" + validThresholds, "格式应为 3968.HK"},
		"纯港股":     {"banks:\n  - {market: HK, symbol: 2388.HK}\n" + validThresholds, "须填 a_share_ref"},
		"映射格式":    {"banks:\n  - {market: HK, symbol: 3968.HK, a_share_ref: '600036'}\n" + validThresholds, "须填 a_share_ref"},
		"重复":      {one + "  - {market: CN_A, symbol: 600036.SH}\n" + validThresholds, "重复"},
		"港股前导零重复": {"banks:\n  - {market: HK, symbol: 3968.HK, a_share_ref: 600036.SH}\n  - {market: HK, symbol: 03968.HK, a_share_ref: 600036.SH}\n" + validThresholds, "重复"},
		"rank_by": {"rank_by: roe\n" + one + validThresholds, "rank_by"},
		"缺阈值":     {one, "npl_max 须 > 0"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := LoadConfig(writeCfg(t, tc.body))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

// thresholdsWith 生成合法阈值段，但把 field 的值替换为 val。
func thresholdsWith(field, val string) string {
	vals := map[string]string{
		"npl_max": "1.5", "coverage_min": "150", "cet1_min": "8.5",
		"npl_up": "0.10", "coverage_down": "20", "cet1_down": "0.50",
	}
	vals[field] = val
	return "thresholds:\n" +
		"  npl_max: " + vals["npl_max"] + "\n" +
		"  coverage_min: " + vals["coverage_min"] + "\n" +
		"  cet1_min: " + vals["cet1_min"] + "\n" +
		"  deterioration:\n" +
		"    npl_up: " + vals["npl_up"] + "\n" +
		"    coverage_down: " + vals["coverage_down"] + "\n" +
		"    cet1_down: " + vals["cet1_down"] + "\n"
}

func TestLoadConfigRejectsBadThresholds(t *testing.T) {
	one := "banks:\n  - {market: CN_A, symbol: 600036.SH}\n"
	// 对照组：未替换任何字段时合法，证明被拒只由被替换的那个值引起。
	_, err := LoadConfig(writeCfg(t, one+thresholdsWith("", "")))
	require.NoError(t, err)

	fields := []struct{ key, name string }{
		{"npl_max", "npl_max"},
		{"coverage_min", "coverage_min"},
		{"cet1_min", "cet1_min"},
		{"npl_up", "deterioration.npl_up"},
		{"coverage_down", "deterioration.coverage_down"},
		{"cet1_down", "deterioration.cet1_down"},
	}
	for _, f := range fields {
		for _, bad := range []string{"0", "-1", ".nan"} {
			t.Run(f.key+"="+bad, func(t *testing.T) {
				_, err := LoadConfig(writeCfg(t, one+thresholdsWith(f.key, bad)))
				require.Error(t, err)
				assert.Contains(t, err.Error(), "thresholds."+f.name+" 须 > 0")
			})
		}
	}
}

func TestLoadConfigMissingFile(t *testing.T) {
	_, err := LoadConfig(filepath.Join(t.TempDir(), "nope.yaml"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading bank config")
}

func TestLoadConfigBadYAML(t *testing.T) {
	_, err := LoadConfig(writeCfg(t, "banks: [\n  - {market: CN_A\n"))
	require.Error(t, err)
}

func TestLoadConfigInvalidIncludesPath(t *testing.T) {
	p := writeCfg(t, "banks: []\n"+validThresholds)
	_, err := LoadConfig(p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid bank config "+p)
}

func TestShippedConfigLoads(t *testing.T) {
	cfg, err := LoadConfig("../../configs/bank-monitor.yaml")
	require.NoError(t, err)
	require.Len(t, cfg.Banks, 21)

	cnA := func(symbol, name string) BankCfg { return BankCfg{Market: "CN_A", Symbol: symbol, Name: name} }
	want := []BankCfg{
		cnA("601398.SH", "工商银行"), cnA("601288.SH", "农业银行"), cnA("601988.SH", "中国银行"),
		cnA("601939.SH", "建设银行"), cnA("601328.SH", "交通银行"), cnA("601658.SH", "邮储银行"),
		cnA("600036.SH", "招商银行"), cnA("601166.SH", "兴业银行"), cnA("601998.SH", "中信银行"),
		cnA("600000.SH", "浦发银行"), cnA("600016.SH", "民生银行"), cnA("601818.SH", "光大银行"),
		cnA("000001.SZ", "平安银行"), cnA("600015.SH", "华夏银行"), cnA("601916.SH", "浙商银行"),
		cnA("002142.SZ", "宁波银行"), cnA("600919.SH", "江苏银行"), cnA("601169.SH", "北京银行"),
		cnA("601009.SH", "南京银行"), cnA("601229.SH", "上海银行"),
		{Market: "HK", Symbol: "3968.HK", Name: "招商银行H", AShareRef: "600036.SH"},
	}
	assert.Equal(t, want, cfg.Banks)
	assert.Equal(t, "600036.SH", cfg.Banks[20].DataSymbol())

	assert.Equal(t, ThresholdsCfg{
		NPLMax: 1.5, CoverageMin: 150, CET1Min: 8.5,
		Deterioration: DeteriorationCfg{NPLUp: 0.10, CoverageDown: 20, CET1Down: 0.50},
	}, cfg.Thresholds)
}

func TestIndicators(t *testing.T) {
	assert.Equal(t, []Indicator{IndNPL, IndCoverage, IndCET1}, Indicators)
	labels := make([]string, 0, len(Indicators))
	for _, ind := range Indicators {
		labels = append(labels, ind.Label())
	}
	assert.Equal(t, "不良率/拨备覆盖率/CET1", strings.Join(labels, "/"))
	assert.True(t, IndNPL.HigherIsWorse())
	assert.False(t, IndCoverage.HigherIsWorse())
	assert.False(t, IndCET1.HigherIsWorse())
}

func TestDayHelper(t *testing.T) {
	assert.Equal(t, time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC), day("2026-06-30"))
	assert.Panics(t, func() { day("2026/06/30") })
}
