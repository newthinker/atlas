package crisis

// Context Checkpoint: done_criteria → test mapping (TASK-004 RenderReplayHTML)
// functional[0]  点阵色块数=交易日数 / 期间标题 / 转移明细"→"1 次 / 月度表跨月 / WATCH #eab308 → TestRenderReplayHTMLGoldenFragments
// functional[1]  polyline 条数=有观测指标数 / 点数=观测数(逗号计数,缺口不补点)          → TestRenderReplayHTMLPolylinePoints
// boundary[0]    sofr_effr 全期无数据→省略+注记 / STALE 日 <circle> 打点 title            → TestRenderReplayHTMLNotesAndMarkers
// error_handling days 空→"no replay days to render"                                       → TestRenderReplayHTMLEmpty
//                SeriesReader 错误上抛                                                     → TestRenderReplayHTMLReaderError
// non_functional 自包含(无 http/https) / prefers-color-scheme / overflow-x / amber=25(读 cfg) / 无禁词 / 页脚 → TestRenderReplayHTMLSelfContained

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// htmlFixture 3 个交易日（跨月 06-30/07-01/07-02，触发一次月度分组切换与一次转移），
// vix/hy_oas 有观测序列，sofr_effr 全期无数据；hy_oas 第 2 日观测缺口且 STALE
// （其折线图应出现 x 轴打点，打点只画在有图指标上）。
func htmlFixture(t *testing.T) ([]ReplayDay, memSeries) {
	t.Helper()
	days := []ReplayDay{
		mkReplayDay("2026-06-30", StateNormal, StateNormal, 9, 0, nil,
			map[string]float64{IndVIX: 18}),
		mkReplayDay("2026-07-01", StateNormal, StateWatch, 1, 3,
			map[string]Status{IndHYOAS: StatusStale, IndVIX: StatusRed},
			map[string]float64{IndVIX: 33}),
		mkReplayDay("2026-07-02", StateWatch, StateWatch, 2, 2, nil,
			map[string]float64{IndVIX: 28}),
	}
	sr := memSeries{
		IndVIX: {
			{Date: "2026-06-30", Indicator: IndVIX, Value: 18},
			{Date: "2026-07-01", Indicator: IndVIX, Value: 33},
			{Date: "2026-07-02", Indicator: IndVIX, Value: 28},
		},
		IndHYOAS: {
			{Date: "2026-06-30", Indicator: IndHYOAS, Value: 400},
			{Date: "2026-07-02", Indicator: IndHYOAS, Value: 410},
		},
	}
	return days, sr
}

// functional: 点阵色块数=交易日数；转移明细行数；月度表行数=跨月数。
func TestRenderReplayHTMLGoldenFragments(t *testing.T) {
	days, sr := htmlFixture(t)
	html, err := RenderReplayHTML(testConfig(), days, sr)
	require.NoError(t, err)

	assert.Equal(t, 3, strings.Count(html, `class="day"`), "点阵色块数 = 交易日数")
	assert.Contains(t, html, "2026-06-30 ~ 2026-07-02")
	assert.Contains(t, html, "NORMAL → WATCH")                       // 转移明细
	assert.Equal(t, 1, strings.Count(html, "→"), "仅 1 次转移")
	assert.Contains(t, html, "<td>2026-06</td>")                     // 月度表 2 行
	assert.Contains(t, html, "<td>2026-07</td>")
	assert.Contains(t, html, "#eab308")                              // WATCH 黄色块
}

// functional: 折线 polyline 点数 = 该指标观测数；缺观测日不补点。
func TestRenderReplayHTMLPolylinePoints(t *testing.T) {
	days, sr := htmlFixture(t)
	html, err := RenderReplayHTML(testConfig(), days, sr)
	require.NoError(t, err)

	// 有观测的指标各 1 条 polyline：vix(3 点) + hy_oas(2 点)
	assert.Equal(t, 2, strings.Count(html, "<polyline"))
	vix := extractChart(t, html, IndVIX)
	assert.Equal(t, 3, strings.Count(vix, ","), "vix polyline 3 个坐标点")
	hy := extractChart(t, html, IndHYOAS)
	assert.Equal(t, 2, strings.Count(hy, ","), "hy_oas polyline 2 个坐标点")
}

// extractChart 取 <polyline ... points="..."> 的 points 属性值（按指标节顺序）。
func extractChart(t *testing.T, html, ind string) string {
	t.Helper()
	i := strings.Index(html, "<h3>"+ind+"</h3>")
	require.GreaterOrEqual(t, i, 0, ind)
	rest := html[i:]
	j := strings.Index(rest, `points="`)
	require.GreaterOrEqual(t, j, 0, ind)
	rest = rest[j+len(`points="`):]
	return rest[:strings.Index(rest, `"`)]
}

// boundary: sofr_effr 全期无数据 → 省略该幅 + 专用注记；STALE 日打点存在。
func TestRenderReplayHTMLNotesAndMarkers(t *testing.T) {
	days, sr := htmlFixture(t)
	html, err := RenderReplayHTML(testConfig(), days, sr)
	require.NoError(t, err)

	assert.Contains(t, html, "该指标自 2018-04 起才有数据")
	assert.Contains(t, html, `<circle`, "STALE/缺数日 x 轴打点")
	assert.Contains(t, html, "2026-07-01 STALE")
}

// error_handling: days 为空 → 报错 "no replay days to render"（触发半）。
func TestRenderReplayHTMLEmpty(t *testing.T) {
	_, err := RenderReplayHTML(testConfig(), nil, memSeries{})
	require.Error(t, err)
	assert.ErrorContains(t, err, "no replay days to render")
}

// error_handling: SeriesReader 错误 → 上抛（触发半，复用包内 failReader stub）。
func TestRenderReplayHTMLReaderError(t *testing.T) {
	days, _ := htmlFixture(t)
	sr := failReader{windowSinceErr: errors.New("db down")}
	_, err := RenderReplayHTML(testConfig(), days, sr)
	require.Error(t, err)
	assert.ErrorContains(t, err, "db down")
}

// stateColor 四态色板逐一覆盖（fixture 只出现 NORMAL/WATCH，此处补 BREWING/CRISIS）。
func TestStateColorPalette(t *testing.T) {
	assert.Equal(t, "#16a34a", stateColor(StateNormal))
	assert.Equal(t, "#eab308", stateColor(StateWatch))
	assert.Equal(t, "#f97316", stateColor(StateBrewing))
	assert.Equal(t, "#dc2626", stateColor(StateCrisis))
}

// thresholdLines 逐指标横线规则：5 指标各自阈值线 + nfci/t10y2y 单线；
// usdjpy 周环比规则无水平阈值线 → 返回 nil（原 nf review 项转直接断言）。
func TestThresholdLines(t *testing.T) {
	cfg := testConfig()
	assert.Len(t, thresholdLines(cfg, IndVIX), 2)
	assert.Len(t, thresholdLines(cfg, IndMOVE), 2)
	assert.Len(t, thresholdLines(cfg, IndSOFREFFR), 2)
	assert.Len(t, thresholdLines(cfg, IndHYOAS), 2)
	assert.Len(t, thresholdLines(cfg, IndNFCI), 1)    // 只画 red_above
	assert.Len(t, thresholdLines(cfg, IndT10Y2Y), 1)  // 只画 amber_bp
	assert.Nil(t, thresholdLines(cfg, IndUSDJPY))     // 周环比规则无水平阈值线

	// 值来自 cfg（判别性：vix amber/red = testConfig 的 25/30）
	vix := thresholdLines(cfg, IndVIX)
	assert.Equal(t, 25.0, vix[0].v)
	assert.Equal(t, 30.0, vix[1].v)
}

// non_functional: 自包含（无外链）、亮暗兼容、禁词、阈值来自 cfg。
func TestRenderReplayHTMLSelfContained(t *testing.T) {
	days, sr := htmlFixture(t)
	html, err := RenderReplayHTML(testConfig(), days, sr)
	require.NoError(t, err)

	assert.NotContains(t, html, "http://")
	assert.NotContains(t, html, "https://")
	assert.Contains(t, html, "prefers-color-scheme")
	assert.Contains(t, html, "overflow-x")
	assert.Contains(t, html, "amber=25") // testConfig 的 vix amber，证明读 cfg
	for _, banned := range []string{"必然", "一定", "即将"} {
		assert.NotContains(t, html, banned)
	}
	assert.Contains(t, html, "历史回放，非实时告警")
}
