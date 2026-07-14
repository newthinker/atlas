package crisis

// Context Checkpoint: done_criteria → test mapping (rules)
// functional[0] vix/move 区间+vix 单周涨幅; sofr_effr 持续性 red/amber → TestEvaluateIndicatorStatusPaths
// functional[1] hy_oas 双向黄灯 + 月动量 → TestEvaluateIndicatorStatusPaths / TestEvaluateIndicatorBaseline
// functional[2] t10y2y 倒挂/复陡 STEEPENING; nfci; usdjpy wow 双阈值+52周 CROWDED → TestEvaluateIndicatorStatusPaths
// functional[3] 分位轨升红(Pct5y≥red) → StatusPaths (ramp)；升黄(Pct5y∈[amber,red)) → TestPercentileTrackAmberUpgrade
// functional[4] 基线锚点 AMBER 计数=2 → TestEvaluateIndicatorBaseline
// boundary[0]   Window 空→NO_DATA; staleFor→STALE 直接返回; <60 跳过分位轨但 WindowActualObs 标注 → TestEvaluateIndicatorStatusPaths
// boundary[1]   Pct5y/WindowActualObs 对 track=false 也恒填充 → TestEvaluateIndicatorBaseline (t10y2y/usdjpy WindowActualObs=80)

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// memSeries 实现 SeriesReader（观测按升序存放）。
type memSeries map[string][]Observation

func (m memSeries) Window(indicator, end string, n int) ([]Observation, error) {
	all := m.upTo(indicator, end)
	if len(all) > n {
		all = all[len(all)-n:]
	}
	return all, nil
}

func (m memSeries) WindowSince(indicator, from, end string) ([]Observation, error) {
	var out []Observation
	for _, o := range m[indicator] {
		if o.Date >= from && o.Date <= end {
			out = append(out, o)
		}
	}
	return out, nil
}

func (m memSeries) upTo(indicator, end string) []Observation {
	var out []Observation
	for _, o := range m[indicator] {
		if o.Date <= end {
			out = append(out, o)
		}
	}
	return out
}

// seriesEnding 生成 n 个逐日观测、末日为 end：前 n-1 个取 base，最后一个取 last。
func seriesEnding(end string, n int, base, last float64) []Observation {
	out := make([]Observation, n)
	for i := 0; i < n; i++ {
		v := base
		if i == n-1 {
			v = last
		}
		out[i] = Observation{Date: addDays(end, i-n+1), Value: v}
	}
	return out
}

// baselineSeries 按设计附录基线读数（2026-07-12 核实）构造 7 指标序列。
func baselineSeries(end string) memSeries {
	return memSeries{
		IndVIX:      seriesEnding(end, 80, 15, 15.0),
		IndMOVE:     seriesEnding(end, 80, 70, 69.6),
		IndSOFREFFR: seriesEnding(end, 80, -10, -10),
		IndHYOAS:    seriesEnding(end, 80, 270, 267),
		IndT10Y2Y:   seriesEnding(end, 80, 35, 35),
		IndNFCI:     seriesEnding(end, 80, -0.5, -0.52),
		IndUSDJPY:   seriesEnding(end, 80, 150, 161.7), // 52 周内最高 → CROWDED
	}
}

// testConfig 与 configs/crisis-monitor.yaml 数值一致（引擎测试不读文件）。
func testConfig() *Config {
	return &Config{
		Storage:    StorageCfg{Path: "unused"},
		FRED:       FREDCfg{APIKeyEnv: "FRED_API_KEY"},
		Freshness:  FreshnessCfg{DailyMaxLagDays: 4, WeeklyMaxLagDays: 12},
		Percentile: PercentileCfg{WindowYears: 5, Amber: 0.90, Red: 0.97},
		Indicators: IndicatorsCfg{
			VIX:      VIXCfg{Amber: 25, Red: 30, WeeklySpikePct: 0.50, PercentileTrack: true},
			MOVE:     MOVECfg{Amber: 100, Red: 120, PercentileTrack: true},
			SOFREFFR: SOFREFFRCfg{AmberBp: 10, AmberPersistDays: 3, RedBp: 25, RedPersistDays: 5, PercentileTrack: true, SuppressQuarterEnd: true},
			HYOAS:    HYOASCfg{AmberLowBp: 350, AmberHighBp: 500, RedBp: 600, MomentumBp: 100, MomentumWindowObs: 21, PercentileTrack: true},
			T10Y2Y:   T10Y2YCfg{AmberBp: 25, SteepeningBp: 50, SteepeningLookbackObs: 250},
			NFCI:     NFCICfg{GreenBelow: -0.3, RedAbove: 0, PercentileTrack: true},
			USDJPY:   USDJPYCfg{AmberWowPct: -0.02, RedWowPct: -0.03, Crowded52wPct: 0.98},
		},
		StateMachine: StateMachineCfg{WatchAmberCount: 3, CrisisExitDays: 10, WatchExitDays: 20, BrewingExitDays: 10, DemoteHysteresisDays: 3},
	}
}

// 验收锚点：设计附录基线 → AMBER 计数恰为 2（hy_oas 自满 + usdjpy 拥挤）。
func TestEvaluateIndicatorBaseline(t *testing.T) {
	const d = "2026-07-10"
	cfg, sr := testConfig(), baselineSeries(d)

	want := map[string]struct {
		status Status
		tag    Tag
	}{
		IndVIX:      {StatusGreen, ""},
		IndMOVE:     {StatusGreen, ""},
		IndSOFREFFR: {StatusGreen, ""},
		IndHYOAS:    {StatusAmber, TagComplacency},
		IndT10Y2Y:   {StatusGreen, ""},
		IndNFCI:     {StatusGreen, ""},
		IndUSDJPY:   {StatusAmber, TagCrowded},
	}
	for ind, w := range want {
		res, err := EvaluateIndicator(cfg, ind, d, sr)
		require.NoError(t, err, ind)
		assert.Equal(t, w.status, res.RawStatus, ind)
		assert.Equal(t, w.tag, res.Tag, ind)
		assert.Equal(t, 80, res.WindowActualObs, ind)
	}
}

func TestEvaluateIndicatorStatusPaths(t *testing.T) {
	cfg := testConfig()
	const d = "2026-07-10"

	// NO_DATA：序列不存在（2018 前 SOFR、早期 MOVE 依赖此路径）
	res, err := EvaluateIndicator(cfg, IndSOFREFFR, d, memSeries{})
	require.NoError(t, err)
	assert.Equal(t, StatusNoData, res.Status)

	// STALE：最新观测滞后 7 天 > daily_max_lag_days=4
	res, err = EvaluateIndicator(cfg, IndVIX, d,
		memSeries{IndVIX: seriesEnding("2026-07-03", 10, 15, 15)})
	require.NoError(t, err)
	assert.Equal(t, StatusStale, res.Status)

	// VIX 单周涨幅 >50% → 至少 AMBER（绝对值 22 仍 < 25）
	res, err = EvaluateIndicator(cfg, IndVIX, d,
		memSeries{IndVIX: seriesEnding(d, 10, 14, 22)}) // 22/14−1 ≈ +57%
	require.NoError(t, err)
	assert.Equal(t, StatusAmber, res.RawStatus)

	// SOFR−EFFR 持续性：5 观测全 >25bp → RED
	res, err = EvaluateIndicator(cfg, IndSOFREFFR, d,
		memSeries{IndSOFREFFR: seriesEnding(d, 5, 30, 30)})
	require.NoError(t, err)
	assert.Equal(t, StatusRed, res.RawStatus)
	// 仅最近 3 观测 >10bp → AMBER
	amber := append(seriesEnding(addDays(d, -3), 2, 5, 5), seriesEnding(d, 3, 15, 15)...)
	res, err = EvaluateIndicator(cfg, IndSOFREFFR, d, memSeries{IndSOFREFFR: amber})
	require.NoError(t, err)
	assert.Equal(t, StatusAmber, res.RawStatus)

	// HY OAS 月走阔 110bp > 100bp → AMBER+STRESS（水平 470 在绿区）
	res, err = EvaluateIndicator(cfg, IndHYOAS, d,
		memSeries{IndHYOAS: seriesEnding(d, 25, 360, 470)})
	require.NoError(t, err)
	assert.Equal(t, StatusAmber, res.RawStatus)
	assert.Equal(t, TagStress, res.Tag)

	// 10Y−2Y 倒挂 → RED
	res, err = EvaluateIndicator(cfg, IndT10Y2Y, d,
		memSeries{IndT10Y2Y: seriesEnding(d, 10, 10, -5)})
	require.NoError(t, err)
	assert.Equal(t, StatusRed, res.RawStatus)
	// 倒挂后复陡 60bp > 50bp → GREEN 但带 STEEPENING 标记（设计 §3.1 说明二）
	steep := append(seriesEnding(addDays(d, -5), 20, -30, -30), seriesEnding(d, 5, 20, 30)...)
	res, err = EvaluateIndicator(cfg, IndT10Y2Y, d, memSeries{IndT10Y2Y: steep})
	require.NoError(t, err)
	assert.Equal(t, StatusGreen, res.RawStatus)
	assert.Equal(t, TagSteepening, res.Tag)

	// NFCI：>0 红、−0.3~0 黄
	res, err = EvaluateIndicator(cfg, IndNFCI, d,
		memSeries{IndNFCI: seriesEnding(d, 10, -0.5, 0.1)})
	require.NoError(t, err)
	assert.Equal(t, StatusRed, res.RawStatus)
	res, err = EvaluateIndicator(cfg, IndNFCI, d,
		memSeries{IndNFCI: seriesEnding(d, 10, -0.5, -0.1)})
	require.NoError(t, err)
	assert.Equal(t, StatusAmber, res.RawStatus)

	// USD/JPY 周跌 3.5% ≤ −3%（日元急升值）→ RED；6 观测 <60 → 不做 CROWDED
	res, err = EvaluateIndicator(cfg, IndUSDJPY, d,
		memSeries{IndUSDJPY: seriesEnding(d, 6, 100, 96.5)})
	require.NoError(t, err)
	assert.Equal(t, StatusRed, res.RawStatus)
	assert.Equal(t, Tag(""), res.Tag)

	// 分位轨：绝对值全绿但当前值处 80 观测的 0.9875 分位 ≥0.97 → RED（双轨任一触发）
	ramp := make([]Observation, 80)
	for i := range ramp {
		ramp[i] = Observation{Date: addDays(d, i-79), Value: 10 + float64(i)*0.1}
	}
	res, err = EvaluateIndicator(cfg, IndVIX, d, memSeries{IndVIX: ramp})
	require.NoError(t, err)
	assert.Equal(t, StatusRed, res.RawStatus)
}

// TestEvaluateIndicatorAbsoluteBands 逐条覆盖 done_criteria functional 要求的
// 各指标绝对阈值区间（红/黄档），避免只测部分分支。所有序列 <60 观测以隔离
// 绝对轨（分位轨因 WindowActualObs<minPercentileObs 不触发）。
func TestEvaluateIndicatorAbsoluteBands(t *testing.T) {
	cfg := testConfig()
	const d = "2026-07-10"

	tests := []struct {
		name       string
		indicator  string
		series     []Observation
		wantStatus Status
		wantTag    Tag
	}{
		// vix 绝对区间（区别于 weekly spike 路径）
		{"vix red", IndVIX, seriesEnding(d, 10, 15, 32), StatusRed, ""},   // >30
		{"vix amber", IndVIX, seriesEnding(d, 10, 15, 27), StatusAmber, ""}, // [25,30]
		// move 区间
		{"move red", IndMOVE, seriesEnding(d, 10, 70, 125), StatusRed, ""},   // >120
		{"move amber", IndMOVE, seriesEnding(d, 10, 70, 105), StatusAmber, ""}, // [100,120]
		// hy_oas 红 + 上侧黄（STRESS）
		{"hyoas red", IndHYOAS, seriesEnding(d, 10, 400, 650), StatusRed, ""},        // >600
		{"hyoas amber stress", IndHYOAS, seriesEnding(d, 10, 480, 550), StatusAmber, TagStress}, // [500,600]
		// t10y2y 黄区间 [0,25]
		{"t10y2y amber", IndT10Y2Y, seriesEnding(d, 10, 40, 10), StatusAmber, ""},
		// usdjpy 单黄 wow（−2.5% ≤ −2% 但 > −3%），6 观测 <60 不做 CROWDED
		{"usdjpy amber wow", IndUSDJPY, seriesEnding(d, 6, 100, 97.5), StatusAmber, ""},
		// sofr_effr amber 且总观测正好 3（走 lastN 的 len<=n 分支）
		{"sofr amber short window", IndSOFREFFR, seriesEnding(d, 3, 15, 15), StatusAmber, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := EvaluateIndicator(cfg, tt.indicator, d, memSeries{tt.indicator: tt.series})
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, res.RawStatus)
			assert.Equal(t, tt.wantTag, res.Tag)
		})
	}
}

// TestPercentileTrackAmberUpgrade 覆盖 functional[3] 的 AMBER 半边（rules.go
// `case res.Pct5y >= cfg.Percentile.Amber`）：VIX 绝对轨全绿，但当前值处 80 观测
// 的 [0.90,0.97) 分位 → 分位轨升黄。判别性：amber 分支坏则退回 GREEN 使用例失败。
func TestPercentileTrackAmberUpgrade(t *testing.T) {
	cfg := testConfig()
	const d = "2026-07-10"

	// 80 观测：73 个低值(10) + 6 个中值(24.5) + 当前值(24)。严格小于 24 的恰 73 个
	// → 分位 73/80 = 0.9125 ∈ [0.90,0.97)。全部 <25 → 绝对轨绿；末 6 观测 WowPct<0
	// → 不触发周涨。
	vals := make([]Observation, 80)
	for i := range vals {
		v := 10.0
		switch {
		case i == 79:
			v = 24.0
		case i >= 73:
			v = 24.5
		}
		vals[i] = Observation{Date: addDays(d, i-79), Value: v}
	}
	res, err := EvaluateIndicator(cfg, IndVIX, d, memSeries{IndVIX: vals})
	require.NoError(t, err)
	assert.Equal(t, StatusAmber, res.RawStatus)
	assert.InDelta(t, 0.9125, res.Pct5y, 1e-9)
	assert.Equal(t, 80, res.WindowActualObs)
}
