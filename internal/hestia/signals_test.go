package hestia

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Context Checkpoint: done_criteria → test mapping（M2a 的 TASK-002）
// functional[0] 三期 golden                          → TestEvaluateGoldenThreePeriods
// functional[0] 月均来源（_mom 优先 / ÷月份 / ÷3,6,9,12） → TestMonthlyAverageSources
// functional[0] 期内月数                             → TestMonthsInPeriod
// functional[0] 缺失 ⇒ unknown、分母减一             → TestEvaluateUnknownShrinksDenominator
// functional[0] 信贷同口径                           → TestEvaluateCreditRefusesMixedCaliber
// functional[0] 剪刀差黄灯 0 分                      → TestEvaluateScissorsYellowScoresZero
// functional[1] AST 守卫登记 Evaluate（27 项）        → store_test.go TestPackageExposesNoWriteFunctions
// boundary[0]   monthsInPeriod 越界月份/长度/未知类型 → TestMonthsInPeriod（子例 boundary）
// boundary[0]   monthlyAverage _ytd 在场但月数 0     → TestMonthlyAverageSources（末段）
// boundary[0]   evalCredit total==0 ⇒ unknown        → TestEvaluateCreditZeroTotalIsUnknown
// boundary[0]   空 Values ⇒ 四个 unknown、0/0        → TestEvaluateEmptyValuesAllUnknown
// boundary[0]   001 变异残留：两线相等被拒           → config_test.go TestLoadConfigRejectsBadQueueAndSignals（两子例）
// （M2a 的 TASK-004 boundary[2]）002 变异残留：五个阈值的相等边 + 混口径反向 → TestEvaluateThresholdEdges（六子例）

// obsAt 与 store_test.go 的 obsWith（固定 validMeta）不同：本文件的用例要指定期次与
// period_type，因为月均要按它们除月数。需求原文把它叫 obsWith，与既有 helper 重名，改名。
func obsAt(period, periodType string, vals map[string]float64) Observation {
	return Observation{Meta: Meta{Period: period, PeriodType: periodType}, Values: vals}
}

// 三期 golden：方案报告 4.7 的表。数值取自回填库 v_hestia_current（2026-09-05）。
//
//	2020H1  剪刀差 6.5−11.1=−4.6 🔴 · 住户中长期 28000/6=4667 🟢 · 住户短期 7552/6 🟢 · 票据 9697/87700=11.1% 🟡 ⇒ 2
//	2025 全年 3.8−8.5 🔴 · 12800/12=1067 🔴 · −8351/12 🔴 · 16600/154700=10.7% 🟡 ⇒ 0
//	2026H1  4.0−8.0 🔴 · 2212/6 🔴 · −5881/6 🔴 · 8143/111300=7.3% 🟢 ⇒ 1
func TestEvaluateGoldenThreePeriods(t *testing.T) {
	cfg := DefaultSignals()
	cases := []struct {
		name string
		obs  Observation
		want Temperature
	}{
		{"2020H1", obsAt("2020-06", "h1", map[string]float64{
			FieldM1YoY: 6.5, FieldM2YoY: 11.1,
			FieldLoanHHMLTYTD: 28000, FieldLoanHHShortYTD: 7552,
			FieldLoanBillYTD: 9697, FieldLoanCorpTotalYTD: 87700,
		}), Temperature{SignalRed, SignalGreen, SignalGreen, SignalYellow, 2, 4}},
		{"2025 全年", obsAt("2025-12", "annual", map[string]float64{
			FieldM1YoY: 3.8, FieldM2YoY: 8.5,
			FieldLoanHHMLTYTD: 12800, FieldLoanHHShortYTD: -8351,
			FieldLoanBillYTD: 16600, FieldLoanCorpTotalYTD: 154700,
		}), Temperature{SignalRed, SignalRed, SignalRed, SignalYellow, 0, 4}},
		{"2026H1", obsAt("2026-06", "h1", map[string]float64{
			FieldM1YoY: 4.0, FieldM2YoY: 8.0,
			FieldLoanHHMLTYTD: 2212, FieldLoanHHShortYTD: -5881,
			FieldLoanBillYTD: 8143, FieldLoanCorpTotalYTD: 111300,
		}), Temperature{SignalRed, SignalRed, SignalRed, SignalGreen, 1, 4}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, Evaluate(c.obs, cfg))
		})
	}
}

// 月均：_mom 优先（月数 1）；monthly 只有 _ytd 时 ÷ 月份序号；其余四类按 3/6/9/12。
func TestMonthlyAverageSources(t *testing.T) {
	vals := map[string]float64{FieldLoanHHMLTYTD: 7000, FieldLoanHHMLTMoM: 1602}
	v, ok := monthlyAverage(vals, FieldLoanHHMLTYTD, FieldLoanHHMLTMoM, "2023-08", "monthly")
	assert.True(t, ok)
	assert.Equal(t, 1602.0, v, "_mom 在场时用 _mom，不除")

	only := map[string]float64{FieldLoanHHMLTYTD: 7000}
	v, ok = monthlyAverage(only, FieldLoanHHMLTYTD, FieldLoanHHMLTMoM, "2022-07", "monthly")
	assert.True(t, ok)
	assert.Equal(t, 1000.0, v, "2020–2023 月报是 1-N月 累计口径，必须 ÷ 月份序号（7）")

	for pt, div := range map[string]float64{"q1": 3, "h1": 6, "q1_q3": 9, "annual": 12} {
		v, ok = monthlyAverage(only, FieldLoanHHMLTYTD, FieldLoanHHMLTMoM, "2025-12", pt)
		assert.True(t, ok)
		assert.Equal(t, 7000/div, v, pt)
	}

	_, ok = monthlyAverage(map[string]float64{}, FieldLoanHHMLTYTD, FieldLoanHHMLTMoM, "2025-12", "annual")
	assert.False(t, ok)

	// 边界（M2a 的 TASK-002 boundary）：_ytd 在场但期内月数解析为 0 ⇒ 不能除，视为缺失。
	// 否则会除以 0 得 ±Inf，再被 evalThreshold 当成一个「很大」的月均判绿。
	_, ok = monthlyAverage(only, FieldLoanHHMLTYTD, FieldLoanHHMLTMoM, "2022-xx", "monthly")
	assert.False(t, ok, "_ytd 在场但月数 0 ⇒ ok=false")
}

func TestMonthsInPeriod(t *testing.T) {
	assert.Equal(t, 7, monthsInPeriod("2022-07", "monthly"))
	assert.Equal(t, 12, monthsInPeriod("2022-12", "monthly"))
	assert.Equal(t, 3, monthsInPeriod("2026-03", "q1"))
	assert.Equal(t, 9, monthsInPeriod("2025-09", "q1_q3"))
	assert.Equal(t, 0, monthsInPeriod("2022-xx", "monthly"), "解析不出月份 ⇒ 0，调用方视为缺失")

	// 边界（M2a 的 TASK-002 boundary）：需求只覆盖 "2022-xx"，这里把越界月份、长度不对、
	// 未知 periodType 都钉成 0——任何一个漏成非 0 都会让 monthlyAverage 除出一个假月均。
	t.Run("boundary", func(t *testing.T) {
		cases := map[string]struct{ period, pt string }{
			"月份 13":         {"2022-13", "monthly"},
			"月份 00":         {"2022-00", "monthly"},
			"长度 ≠ 7":        {"2022-7", "monthly"},
			"未知 periodType": {"2022-07", "weekly"},
		}
		for name, c := range cases {
			assert.Equalf(t, 0, monthsInPeriod(c.period, c.pt), "%s: monthsInPeriod(%q, %q)", name, c.period, c.pt)
		}
	})
}

// 缺失 ⇒ unknown，Known 减一，Score 只数已知的绿灯。
func TestEvaluateUnknownShrinksDenominator(t *testing.T) {
	obs := obsAt("2026-06", "h1", map[string]float64{
		FieldM1YoY: 4.0, FieldM2YoY: 8.0, // 活化 🔴
		FieldLoanHHMLTYTD: 20000, // 楼市 🟢
		// 住户短期缺失 ⇒ unknown
		FieldLoanBillYTD: 8143, FieldLoanCorpTotalYTD: 111300, // 信贷 🟢
	})
	got := Evaluate(obs, DefaultSignals())
	assert.Equal(t, SignalUnknown, got.Consumption)
	assert.Equal(t, 3, got.Known)
	assert.Equal(t, 2, got.Score)
}

// 信贷成色的票据与合计必须同口径：一个 _ytd 一个 _mom 不算。
func TestEvaluateCreditRefusesMixedCaliber(t *testing.T) {
	obs := obsAt("2023-08", "monthly", map[string]float64{
		FieldLoanBillYTD: 30000, FieldLoanCorpTotalMoM: 9488,
	})
	assert.Equal(t, SignalUnknown, Evaluate(obs, DefaultSignals()).Credit)

	same := obsAt("2023-08", "monthly", map[string]float64{
		FieldLoanBillMoM: 3472, FieldLoanCorpTotalMoM: 9488, // 36.6% ⇒ 🔴
	})
	assert.Equal(t, SignalRed, Evaluate(same, DefaultSignals()).Credit)
}

// 剪刀差介于两线之间是黄灯，黄灯 0 分。
func TestEvaluateScissorsYellowScoresZero(t *testing.T) {
	obs := obsAt("2026-06", "h1", map[string]float64{FieldM1YoY: 7.0, FieldM2YoY: 8.0}) // −1
	got := Evaluate(obs, DefaultSignals())
	assert.Equal(t, SignalYellow, got.Activation)
	assert.Equal(t, 0, got.Score)
	assert.Equal(t, 1, got.Known)
}

// 边界（M2a 的 TASK-002 boundary）：企业贷款合计为 0 时票据占比无定义 ⇒ unknown，
// 不是红也不是绿。_mom 对与 _ytd 对各钉一次——sameCaliberPair 两条分支都要到 total==0 那道闸。
func TestEvaluateCreditZeroTotalIsUnknown(t *testing.T) {
	cases := map[string]map[string]float64{
		"_mom 对 total=0": {FieldLoanBillMoM: 5, FieldLoanCorpTotalMoM: 0},
		"_ytd 对 total=0": {FieldLoanBillYTD: 5, FieldLoanCorpTotalYTD: 0},
	}
	for name, vals := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, SignalUnknown, Evaluate(obsAt("2025-12", "annual", vals), DefaultSignals()).Credit)
		})
	}
}

// 边界（M2a 的 TASK-002 boundary）：空 Values ⇒ 四个 unknown、Score 0、Known 0。
// 温度 0/0 是「没有数据」，与 0/4「四个都红」对读消息的人是两回事。
func TestEvaluateEmptyValuesAllUnknown(t *testing.T) {
	got := Evaluate(obsAt("2025-12", "annual", map[string]float64{}), DefaultSignals())
	assert.Equal(t, Temperature{SignalUnknown, SignalUnknown, SignalUnknown, SignalUnknown, 0, 0}, got)
}

// 002 变异残留（M2a 的 TASK-004 boundary[2]；验证者 test-m2a-b 报告 M1–M5、M13）：五个阈值的
// 相等边与混口径的反向组合此前没有用例，把 `>=` 变异成 `>`、`<` 变异成 `<=` 整包仍绿。
// 相等边逐个钉住：剪刀差 d == active(0) 绿、d == sink(-2) 红；楼市月均 v == warm(2000) 绿；
// 票据比 r == healthy(10%) 黄、r == severe(20%) 红；混口径 (bill_mom, corp_total_ytd) unknown。
// 票据比用 10/100、20/100：bill/total*100 在 float64 下恰等于 10、20（实测），不落在舍入误差里。
func TestEvaluateThresholdEdges(t *testing.T) {
	cfg := DefaultSignals()
	cases := []struct {
		name string
		vals map[string]float64
		pick func(Temperature) Signal
		want Signal
	}{
		{"剪刀差 d == ScissorsActive ⇒ green", map[string]float64{FieldM1YoY: 8.0, FieldM2YoY: 8.0},
			func(x Temperature) Signal { return x.Activation }, SignalGreen},
		{"剪刀差 d == ScissorsSink ⇒ red", map[string]float64{FieldM1YoY: 6.0, FieldM2YoY: 8.0},
			func(x Temperature) Signal { return x.Activation }, SignalRed},
		{"楼市月均 v == HHMltMonthlyWarm（_mom）⇒ green", map[string]float64{FieldLoanHHMLTMoM: 2000},
			func(x Temperature) Signal { return x.Housing }, SignalGreen},
		{"票据比 r == BillRatioHealthy ⇒ yellow", map[string]float64{FieldLoanBillMoM: 10, FieldLoanCorpTotalMoM: 100},
			func(x Temperature) Signal { return x.Credit }, SignalYellow},
		{"票据比 r == BillRatioSevere ⇒ red", map[string]float64{FieldLoanBillMoM: 20, FieldLoanCorpTotalMoM: 100},
			func(x Temperature) Signal { return x.Credit }, SignalRed},
		{"混口径反向 (bill_mom, corp_total_ytd) ⇒ unknown", map[string]float64{FieldLoanBillMoM: 3472, FieldLoanCorpTotalYTD: 30000},
			func(x Temperature) Signal { return x.Credit }, SignalUnknown},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, c.pick(Evaluate(obsAt("2023-08", "monthly", c.vals), cfg)))
		})
	}
}
