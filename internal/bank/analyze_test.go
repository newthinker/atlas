package bank

// Context Checkpoint: done_criteria → test mapping（TASK-003）
// functional[0]  cmb 实测序列最新期/环比/同比           → TestAnalyzeLatestQoQYoY
// functional[1]  600919.SH 按指标回退 + CET1 阈值预警    → TestAnalyzeFallbackPerIndicator600919
// functional[2]  002142.SZ 跨空期环比/同比              → TestAnalyzeAcrossGap002142
// functional[3]  阈值/恶化预警与改善不预警               → TestAlertsLevel, TestAlertsDeterioration
// boundary[0]    严格不等号 + 浮点边界三例               → TestAlertsLevel(等于阈值), TestAlertsDeteriorationBoundaryIsStrict
// boundary[1]    NaN 各形态                              → TestAnalyzeSkipsAllNaNTail, TestAnalyzeIndicatorAllNaN,
//                                                          TestAnalyzeSingleValidValue, TestAnalyzeEmpty, TestAnalyzeYoYPeriodNaN
// non_functional 不修改入参 / vet / 全绿 / 覆盖率         → TestAnalyzeDoesNotMutateInput（输入/期望现场构造）+ go vet / go test -cover

import (
	"fmt"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var nan = math.NaN()

func obs(date string, npl, cov, cet1 float64) Observation {
	return Observation{Period: day(date), Values: [numIndicators]float64{npl, cov, cet1}}
}

// cmb 是 600036.SH 2025-03-31 … 2026-06-30 的实测值（升序）。
var cmb = []Observation{
	obs("2025-03-31", 0.94, 410.03, 14.86),
	obs("2025-06-30", 0.93, 410.93, 14.0),
	obs("2025-09-30", 0.94, 405.93, 13.93),
	obs("2025-12-31", 0.94, 391.79, 14.16),
	obs("2026-03-31", 0.94, 387.76, 14.13),
	obs("2026-06-30", 0.94, 385.1, 14.07),
}

// jsbk 是 600919.SH（江苏银行）2024-06-30 … 2025-09-30 的实测值（升序）。
// 来源：本机 aktools stock_financial_analysis_indicator_em?symbol=600919.SH&indicator=按报告期，
// 字段 NONPERLOAN / BLDKBBL / HXYJBCZL，取数日期 2026-09-27。三季报未披露 CET1（接口为 null）。
var jsbk = []Observation{
	obs("2024-06-30", 0.89, 357.2, 8.99),
	obs("2024-09-30", 0.89, 351.03, nan),
	obs("2024-12-31", 0.89, 350.1, 9.12),
	obs("2025-03-31", 0.86, 343.51, 8.36),
	obs("2025-06-30", 0.84, 331.02, 8.49),
	obs("2025-09-30", 0.84, 322.62, nan),
}

// nbbk 是 002142.SZ（宁波银行）2024-06-30 … 2026-06-30 的实测值（升序）。
// 来源同上（symbol=002142.SZ），取数日期 2026-09-27。一季报/三季报未披露 CET1（接口为 null）。
var nbbk = []Observation{
	obs("2024-06-30", 0.76, 420.55, 9.61),
	obs("2024-09-30", 0.76, 404.8, nan),
	obs("2024-12-31", 0.76, 389.35, 9.84),
	obs("2025-03-31", 0.76, 370.54, 9.32),
	obs("2025-06-30", 0.76, 374.16, 9.65),
	obs("2025-09-30", 0.76, 375.92, nan),
	obs("2025-12-31", 0.76, 373.16, 9.34),
	obs("2026-03-31", 0.76, 369.39, nan),
	obs("2026-06-30", 0.76, 373.35, 9.53),
}

var stdThresholds = ThresholdsCfg{
	NPLMax: 1.5, CoverageMin: 150, CET1Min: 8.5,
	Deterioration: DeteriorationCfg{NPLUp: 0.10, CoverageDown: 20, CET1Down: 0.50},
}

func assertNaNChange(t *testing.T, c Change, msg string) {
	t.Helper()
	assert.True(t, math.IsNaN(c.Value), "%s: Value", msg)
	assert.True(t, math.IsNaN(c.QoQ), "%s: QoQ", msg)
	assert.True(t, math.IsNaN(c.YoY), "%s: YoY", msg)
	assert.True(t, c.Period.IsZero(), "%s: Period", msg)
}

func TestAnalyzeLatestQoQYoY(t *testing.T) {
	latest, ind := Analyze(slices.Clone(cmb))
	p := day("2026-06-30")
	assert.Equal(t, p, latest)
	assert.Equal(t, Change{0.94, 0, 0.01, p}, ind[IndNPL])
	assert.Equal(t, Change{385.1, -2.66, -25.83, p}, ind[IndCoverage], "变动四舍五入到 1e-4，可精确比较")
	assert.Equal(t, Change{14.07, -0.06, 0.07, p}, ind[IndCET1])
}

func TestAnalyzeFallbackPerIndicator600919(t *testing.T) {
	latest, ind := Analyze(slices.Clone(jsbk))
	q3 := day("2025-09-30")
	assert.Equal(t, q3, latest, "NPL/拨备有值即为最新期")
	assert.Equal(t, Change{0.84, 0, -0.05, q3}, ind[IndNPL])
	assert.Equal(t, Change{322.62, -8.4, -28.41, q3}, ind[IndCoverage])
	// CET1 回退到 2025-06-30：环比对 2025-03-31（8.36），同比对 2024-06-30（8.99）。
	assert.Equal(t, Change{8.49, 0.13, -0.5, day("2025-06-30")}, ind[IndCET1])

	alerts := Alerts(BankResult{Name: "江苏银行", Latest: latest, Ind: ind}, stdThresholds)
	assert.Equal(t, []Alert{{"江苏银行", IndCET1, AlertLevel, 8.49, 8.5, day("2025-06-30")}}, alerts,
		"阈值预警对回退值生效，Period 取该指标自己的报告期")
}

func TestAnalyzeAcrossGap002142(t *testing.T) {
	latest, ind := Analyze(slices.Clone(nbbk))
	p := day("2026-06-30")
	assert.Equal(t, p, latest)
	// 2026-03-31 CET1 为 null ⇒ 环比跨空期对 2025-12-31（9.34）；同比对 2025-06-30（9.65）。
	assert.Equal(t, Change{9.53, 0.19, -0.12, p}, ind[IndCET1])
	assert.Equal(t, Change{373.35, 3.96, -0.81, p}, ind[IndCoverage])
}

func TestAnalyzeSkipsAllNaNTail(t *testing.T) {
	latest, ind := Analyze(append(cmb[:6:6], obs("2026-09-30", nan, nan, nan)))
	assert.Equal(t, day("2026-06-30"), latest, "全 NaN 的期不算最新期")
	assert.Equal(t, Change{0.94, 0, 0.01, day("2026-06-30")}, ind[IndNPL])

	// 尾期只有拨备有值：它仍是最新期，而 NPL/CET1 回退到上一期。
	latest, ind = Analyze(append(cmb[:6:6], obs("2026-09-30", nan, 380, nan)))
	assert.Equal(t, day("2026-09-30"), latest, "至少一项非 NaN 即为最新期")
	assert.Equal(t, day("2026-06-30"), ind[IndNPL].Period)
	assert.Equal(t, day("2026-09-30"), ind[IndCoverage].Period)
}

func TestAnalyzeIndicatorAllNaN(t *testing.T) {
	series := []Observation{obs("2025-12-31", 1.0, 200, nan), obs("2026-03-31", 1.1, 190, nan)}
	latest, ind := Analyze(series)
	assert.Equal(t, day("2026-03-31"), latest, "至少一项非 NaN 即为最新期")
	assertNaNChange(t, ind[IndCET1], "CET1 全序列 NaN")
	assert.Equal(t, 0.1, ind[IndNPL].QoQ)

	bad := stdThresholds
	bad.CET1Min = 1e9 // 若 NaN 参与比较为真，这里必然预警
	for _, a := range Alerts(BankResult{Name: "X", Ind: ind}, bad) {
		assert.NotEqual(t, IndCET1, a.Ind, "全 NaN 的指标不预警")
	}
}

func TestAnalyzeSingleValidValue(t *testing.T) {
	series := []Observation{obs("2025-06-30", nan, 200, 10), obs("2026-06-30", 1, 200, 10)}
	_, ind := Analyze(series)
	assert.Equal(t, 1.0, ind[IndNPL].Value)
	assert.Equal(t, day("2026-06-30"), ind[IndNPL].Period)
	assert.True(t, math.IsNaN(ind[IndNPL].QoQ), "只有一个非 NaN 值 ⇒ 无环比")
	assert.True(t, math.IsNaN(ind[IndNPL].YoY), "上年同日该指标为 NaN ⇒ 无同比")

	_, one := Analyze([]Observation{obs("2026-06-30", 1, 200, 10)})
	for _, k := range Indicators {
		assert.True(t, math.IsNaN(one[k].QoQ))
		assert.True(t, math.IsNaN(one[k].YoY))
	}
}

func TestAnalyzeYoYPeriodNaN(t *testing.T) {
	// 上年同日那期存在但 CET1 为 NaN：不得改用更早的 2025-03-31 或其后的 2025-12-31。
	series := []Observation{
		obs("2025-03-31", 1, 200, 9.0),
		obs("2025-06-30", 1, 200, nan),
		obs("2025-12-31", 1, 200, 9.5),
		obs("2026-06-30", 1, 200, 10),
	}
	_, ind := Analyze(series)
	assert.True(t, math.IsNaN(ind[IndCET1].YoY))
	assert.Equal(t, 0.5, ind[IndCET1].QoQ)
	assert.Equal(t, 0.0, ind[IndNPL].YoY, "其他指标同比照常")
}

func TestAnalyzeEmpty(t *testing.T) {
	latest, ind := Analyze(nil)
	assert.True(t, latest.IsZero())
	for _, k := range Indicators {
		assertNaNChange(t, ind[k], k.Label())
	}
}

// 输入与期望都在此现场构造、互不共享底层数组，不依赖任何包级 fixture：
// 若期望取自可能已被别的测试传给 Analyze 的 fixture，幂等的越界写入（写 NaN、原地前向填充）
// 在它被复制之前就已发生，期望与实际同被污染，守卫随执行顺序失效。
func TestAnalyzeDoesNotMutateInput(t *testing.T) {
	fresh := func() []Observation {
		return []Observation{
			obs("2025-06-30", 0.76, 374.16, 9.65),
			obs("2025-09-30", 0.76, nan, nan),
			obs("2025-12-31", nan, 373.16, 9.34),
			obs("2026-03-31", 0.77, 369.39, nan),
			obs("2026-06-30", nan, nan, nan),
		}
	}
	in := fresh()
	Analyze(in)
	assert.Equal(t, fmt.Sprint(fresh()), fmt.Sprint(in), "fmt 把 NaN 打成 \"NaN\"，可逐字比较")
}

func result(npl, cov, cet1 Change) BankResult {
	return BankResult{Name: "X", Latest: day("2026-06-30"), Ind: [numIndicators]Change{npl, cov, cet1}}
}

// 各指标用不同 Period，以证明 Alert.Period 取自该指标的 Change 而非 BankResult.Latest。
var (
	pNPL  = day("2026-06-30")
	pCov  = day("2026-03-31")
	pCET1 = day("2025-12-31")
)

func val(v float64, p time.Time) Change { return Change{v, nan, nan, p} }

func TestAlertsLevel(t *testing.T) {
	cases := map[string]struct {
		r    BankResult
		want []Alert
	}{
		"不良率超限":    {result(val(1.62, pNPL), val(200, pCov), val(10, pCET1)), []Alert{{"X", IndNPL, AlertLevel, 1.62, 1.5, pNPL}}},
		"不良率等于阈值":  {result(val(1.5, pNPL), val(200, pCov), val(10, pCET1)), nil},
		"拨备不足":     {result(val(1, pNPL), val(142, pCov), val(10, pCET1)), []Alert{{"X", IndCoverage, AlertLevel, 142, 150, pCov}}},
		"拨备等于阈值":   {result(val(1, pNPL), val(150, pCov), val(10, pCET1)), nil},
		"CET1不足":   {result(val(1, pNPL), val(200, pCov), val(8.4, pCET1)), []Alert{{"X", IndCET1, AlertLevel, 8.4, 8.5, pCET1}}},
		"CET1等于阈值": {result(val(1, pNPL), val(200, pCov), val(8.5, pCET1)), nil},
		"NaN不预警":   {result(val(nan, time.Time{}), val(nan, time.Time{}), val(nan, time.Time{})), nil},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, Alerts(tc.r, stdThresholds))
		})
	}
}

func TestAlertsDeterioration(t *testing.T) {
	r := result(Change{1.0, 0.11, nan, pNPL}, Change{200, -20.5, nan, pCov}, Change{10, -0.62, nan, pCET1})
	assert.Equal(t, []Alert{
		{"X", IndNPL, AlertDeterioration, 0.11, 0.10, pNPL},
		{"X", IndCoverage, AlertDeterioration, -20.5, 20, pCov},
		{"X", IndCET1, AlertDeterioration, -0.62, 0.50, pCET1},
	}, Alerts(r, stdThresholds))

	improving := result(Change{1.0, -0.5, nan, pNPL}, Change{200, 30, nan, pCov}, Change{10, 1, nan, pCET1})
	assert.Empty(t, Alerts(improving, stdThresholds), "改善方向不预警")
}

// 三对取值在 float64 下的原始差都越过阈值（Go 与 Python 两把尺实测）：
// 0.67-0.57 = 0.10000000000000009、236.04-256.04 = -20.000000000000028、15.51-16.01 = -0.50000000000000178。
// 十进制小数在二进制下不可精确表示，相减的舍入误差在 1e-14 量级、方向不定；round4 把它拉回到与阈值字面量
// 相同的 float64，严格不等号才按十进制语义判「恰等于阈值不预警」。
func TestAlertsDeteriorationBoundaryIsStrict(t *testing.T) {
	cases := map[string]struct {
		k        Indicator
		from, to float64
		want     float64
	}{
		"NPL":      {IndNPL, 0.57, 0.67, 0.1},
		"Coverage": {IndCoverage, 256.04, 236.04, -20},
		"CET1":     {IndCET1, 16.01, 15.51, -0.5},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			prev, cur := obs("2026-03-31", 1, 200, 10), obs("2026-06-30", 1, 200, 10)
			prev.Values[tc.k], cur.Values[tc.k] = tc.from, tc.to
			_, ind := Analyze([]Observation{prev, cur})
			assert.Equal(t, tc.want, ind[tc.k].QoQ)
			assert.Empty(t, Alerts(BankResult{Name: "X", Ind: ind}, stdThresholds), "恰等于恶化阈值不预警")
		})
	}
}
