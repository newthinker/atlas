package sankey

// Context Checkpoint: done_criteria → test mapping
// functional[0] BuildPeriods fy 聚合（跨财年、不足 4 季不产出、Segments 求和、非日历财年标签分组） →
//               TestBuildPeriodsFYAggregation, TestBuildPeriodsNonCalendarFiscalYear
// functional[1] DefaultSelection 两分支（季度分支返回本财年已发布季 / 年度分支截取最近 10，AD-15） →
//               TestDefaultSelection
// functional[2] BuildMatrix：季度 yoy 从 allQuarters 反查 / 年度 ≥3 期 cagr / 缺对照期 NaN+none →
//               TestBuildMatrixYoY, TestBuildMatrixAnnualChange
// functional[3] AD-13 桑基残差 + 守恒（other_segment、other_opex、各节点入≈出 ±1、tax_other） →
//               TestBuildSankeyBalance
// boundary[0]   AD-14 除零防护：yoy 对照期为 0/NaN、CAGR 起点 ≤0 或跨期 <2、三比率分母为 0 → NaN 而非 ±Inf →
//               TestBuildMatrixDivisionGuards, TestBuildPeriodsRatioGuards
// boundary[1]   AD-13 负值不画负流（tax_other 为负）+ 残差 |差| < Revenue×0.5% 时省略 →
//               TestBuildSankeyNegativeFlow, TestBuildSankeyResidualOmitted
// boundary[2]   DefaultSelection 边界：皆空 →(nil,"q")；财年首季只 1 期；最新财年 3 季且不伪造 FY →
//               TestDefaultSelection
// non_functional 纯函数无 IO；lang 决定节点名取 NameZH/NameEN；主干硬编码不读模板 →
//               TestBuildSankeyLang, TestBuildMatrixLang

import (
	"math"
	"strings"
	"testing"

	prismstore "github.com/newthinker/atlas/internal/storage/prism"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 单位取真实量级（美元），既贴近生产数据又让 ±1 的守恒容差有意义。
const b = 1e9

func fq(period, end string, rev, gp, oi, ni, rnd, sgna, tax float64) prismstore.FundamentalRow {
	return prismstore.FundamentalRow{
		FiscalPeriod: period, PeriodEnd: end, Source: "edgar",
		Revenue: rev, GrossProfit: gp, OperatingIncome: oi, NetIncome: ni,
		RnD: rnd, SGnA: sgna, IncomeTax: tax,
	}
}

func sr(period, end, key string, rev float64) prismstore.SegmentRow {
	return prismstore.SegmentRow{
		PeriodEnd: end, FiscalPeriod: period, SegmentKey: key,
		Revenue: rev, Source: "edgar_segment",
	}
}

func twoSegTemplate() *Template {
	return &Template{
		Company: "ACME", CIK: "1", SegmentAxis: DefaultSegmentAxis, Version: 1,
		Segments: []Segment{
			{Key: "cloud", NameZH: "云业务", NameEN: "Cloud", Member: "CloudMember"},
			{Key: "devices", NameZH: "硬件设备", NameEN: "Devices", Member: "DevicesMember"},
		},
	}
}

func periodNames(ps []PeriodMetrics) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.Period)
	}
	return out
}

func TestBuildPeriodsQuarterly(t *testing.T) {
	funds := []prismstore.FundamentalRow{
		fq("2026Q1", "2025-09-30", 100*b, 60*b, 30*b, 24*b, 8*b, 7*b, 5*b),
		fq("2026Q2", "2025-12-31", 120*b, 72*b, 36*b, 28*b, 9*b, 8*b, 6*b),
	}
	segs := []prismstore.SegmentRow{
		sr("2026Q1", "2025-09-30", "cloud", 60*b),
		sr("2026Q1", "2025-09-30", "devices", 30*b),
		sr("2026Q2", "2025-12-31", "cloud", 70*b),
		// FiscalPeriod 为空的行无法归期（AD-9 允许为空），必须跳过而不是错记到某一期。
		sr("", "2025-12-31", "cloud", 999*b),
	}

	got := BuildPeriods(funds, segs, "q")
	require.Len(t, got, 2)
	assert.Equal(t, []string{"2026Q1", "2026Q2"}, periodNames(got), "季度应按期升序")

	q1 := got[0]
	assert.Equal(t, 100*b, q1.Revenue)
	assert.Equal(t, 60*b, q1.GrossProfit)
	assert.Equal(t, 30*b, q1.OperatingIncome)
	assert.Equal(t, 24*b, q1.NetIncome)
	assert.Equal(t, 8*b, q1.RnD)
	assert.Equal(t, 7*b, q1.SGnA)
	assert.Equal(t, 5*b, q1.IncomeTax)
	assert.Equal(t, map[string]float64{"cloud": 60 * b, "devices": 30 * b}, q1.Segments)
	assert.InDelta(t, 0.60, q1.GrossMargin, 1e-9)
	assert.InDelta(t, 0.30, q1.OpMargin, 1e-9)
	assert.InDelta(t, 0.24, q1.NetMargin, 1e-9)

	assert.Equal(t, map[string]float64{"cloud": 70 * b}, got[1].Segments,
		"FiscalPeriod 为空的分部行不得被计入任何一期")
}

// 5 个季度跨 2 财年，后一财年只 1 季 → fy 只产出 1 个完整财年。
func TestBuildPeriodsFYAggregation(t *testing.T) {
	funds := []prismstore.FundamentalRow{
		fq("2025Q1", "2024-09-30", 100*b, 60*b, 30*b, 24*b, 8*b, 7*b, 5*b),
		fq("2025Q2", "2024-12-31", 110*b, 66*b, 33*b, 26*b, 8*b, 7*b, 5*b),
		fq("2025Q3", "2025-03-31", 120*b, 72*b, 36*b, 28*b, 9*b, 8*b, 6*b),
		fq("2025Q4", "2025-06-30", 130*b, 78*b, 39*b, 30*b, 9*b, 8*b, 6*b),
		fq("2026Q1", "2025-09-30", 140*b, 84*b, 42*b, 32*b, 10*b, 9*b, 7*b),
	}
	segs := []prismstore.SegmentRow{
		sr("2025Q1", "2024-09-30", "cloud", 60*b),
		sr("2025Q2", "2024-12-31", "cloud", 66*b),
		sr("2025Q3", "2025-03-31", "cloud", 72*b),
		sr("2025Q4", "2025-06-30", "cloud", 78*b),
		sr("2025Q1", "2024-09-30", "devices", 40*b),
		sr("2026Q1", "2025-09-30", "cloud", 84*b),
	}

	got := BuildPeriods(funds, segs, "fy")
	require.Equal(t, []string{"FY2025"}, periodNames(got),
		"只有 4 季齐的财年才产出；FY2026 只有 1 季，不得产出")

	fy := got[0]
	assert.Equal(t, 460*b, fy.Revenue, "流量科目应为 4 季之和")
	assert.Equal(t, 276*b, fy.GrossProfit)
	assert.Equal(t, 138*b, fy.OperatingIncome)
	assert.Equal(t, 108*b, fy.NetIncome)
	assert.Equal(t, 34*b, fy.RnD)
	assert.Equal(t, 30*b, fy.SGnA)
	assert.Equal(t, 22*b, fy.IncomeTax)
	assert.Equal(t, map[string]float64{"cloud": 276 * b, "devices": 40 * b}, fy.Segments,
		"Segments 应按 segment_key 求和，且只计入本财年的季")
	// 比率必须用求和后的值重算，而不是各季比率取平均。
	assert.InDelta(t, 276.0/460.0, fy.GrossMargin, 1e-9)
	assert.InDelta(t, 138.0/460.0, fy.OpMargin, 1e-9)
	assert.InDelta(t, 108.0/460.0, fy.NetMargin, 1e-9)
}

// 非日历财年（MSFT 6 月结）：财年归属只看 fiscal_period 标签前 4 位，
// 与 period_end 落在哪个自然年无关——2026Q1 的 period_end 是 2025-09-30。
func TestBuildPeriodsNonCalendarFiscalYear(t *testing.T) {
	funds := []prismstore.FundamentalRow{
		fq("2025Q3", "2025-03-31", 90*b, 54*b, 27*b, 21*b, 7*b, 6*b, 4*b),
		fq("2025Q4", "2025-06-30", 95*b, 57*b, 28*b, 22*b, 7*b, 6*b, 4*b),
		fq("2026Q1", "2025-09-30", 100*b, 60*b, 30*b, 24*b, 8*b, 7*b, 5*b),
		fq("2026Q2", "2025-12-31", 110*b, 66*b, 33*b, 26*b, 8*b, 7*b, 5*b),
		fq("2026Q3", "2026-03-31", 120*b, 72*b, 36*b, 28*b, 9*b, 8*b, 6*b),
		fq("2026Q4", "2026-06-30", 130*b, 78*b, 39*b, 30*b, 9*b, 8*b, 6*b),
	}

	segs := []prismstore.SegmentRow{
		sr("2026Q1", "2025-09-30", "cloud", 60*b),
		sr("2026Q2", "2025-12-31", "cloud", 66*b),
		sr("2026Q3", "2026-03-31", "cloud", 72*b),
		sr("2026Q4", "2026-06-30", "cloud", 78*b),
	}

	got := BuildPeriods(funds, segs, "fy")
	require.Equal(t, []string{"FY2026"}, periodNames(got),
		"FY2026 的四季（2025-09-30 ~ 2026-06-30）应聚合为一个财年；FY2025 只有 2 季不产出")
	assert.Equal(t, 460*b, got[0].Revenue)
}

func TestBuildPeriodsRatioGuards(t *testing.T) {
	funds := []prismstore.FundamentalRow{
		fq("2026Q1", "2025-09-30", 0, 60*b, 30*b, 24*b, 8*b, 7*b, 5*b),
		fq("2026Q2", "2025-12-31", math.NaN(), 60*b, 30*b, 24*b, 8*b, 7*b, 5*b),
	}
	got := BuildPeriods(funds, nil, "q")
	require.Len(t, got, 2)
	for _, p := range got {
		for name, v := range map[string]float64{
			"GrossMargin": p.GrossMargin, "OpMargin": p.OpMargin, "NetMargin": p.NetMargin,
		} {
			assert.True(t, math.IsNaN(v), "%s/%s 分母为 0 或 NaN 时应为 NaN，实得 %v", p.Period, name, v)
			assert.False(t, math.IsInf(v, 0), "%s/%s 绝不能是 ±Inf：encoding/json 对 Inf 直接报错，整个 API 会 500", p.Period, name)
		}
	}
}

func TestBuildPeriodsEmptyInput(t *testing.T) {
	assert.Empty(t, BuildPeriods(nil, nil, "q"))
	assert.Empty(t, BuildPeriods(nil, nil, "fy"))
}

// periodsNamed builds period columns whose only meaningful field is the label —
// DefaultSelection picks by period name alone.
func periodsNamed(labels ...string) []PeriodMetrics {
	out := make([]PeriodMetrics, 0, len(labels))
	for i, l := range labels {
		out = append(out, PeriodMetrics{Period: l, Revenue: float64(i+1) * b})
	}
	return out
}

func TestDefaultSelection(t *testing.T) {
	t.Run("both empty", func(t *testing.T) {
		sel, gran := DefaultSelection(nil, nil)
		assert.Nil(t, sel)
		assert.Equal(t, "q", gran)
	})

	t.Run("latest is mid year quarter", func(t *testing.T) {
		quarters := periodsNamed("2025Q1", "2025Q2", "2025Q3", "2025Q4", "2026Q1", "2026Q2", "2026Q3")
		fys := periodsNamed("FY2025")
		sel, gran := DefaultSelection(quarters, fys)
		assert.Equal(t, "q", gran)
		assert.Equal(t, []string{"2026Q1", "2026Q2", "2026Q3"}, periodNames(sel),
			"应返回最新期所属财年内全部已发布季")
	})

	t.Run("annual keeps latest ten", func(t *testing.T) {
		quarters := periodsNamed("2026Q1", "2026Q2", "2026Q3", "2026Q4")
		fys := periodsNamed("FY2016", "FY2017", "FY2018", "FY2019", "FY2020",
			"FY2021", "FY2022", "FY2023", "FY2024", "FY2025", "FY2026")
		require.Len(t, fys, 11)
		sel, gran := DefaultSelection(quarters, fys)
		assert.Equal(t, "fy", gran)
		require.Len(t, sel, 10, "AD-15：期数上限统一为 10")
		assert.Equal(t, "FY2017", sel[0].Period, "超出上限时取最近 10 个")
		assert.Equal(t, "FY2026", sel[len(sel)-1].Period)
	})

	t.Run("first quarter of a fiscal year", func(t *testing.T) {
		quarters := periodsNamed("2025Q1", "2025Q2", "2025Q3", "2025Q4", "2026Q1")
		fys := periodsNamed("FY2025")
		sel, gran := DefaultSelection(quarters, fys)
		assert.Equal(t, "q", gran)
		assert.Equal(t, []string{"2026Q1"}, periodNames(sel), "该财年只有 1 期就返回这 1 期")
	})

	t.Run("annual report filed but Q4 derivation failed", func(t *testing.T) {
		// FY2026 只有 3 季（Q4 推导失败），fys 里没有 FY2026 —— 不得伪造。
		quarters := periodsNamed("2025Q4", "2026Q1", "2026Q2", "2026Q3")
		fys := periodsNamed("FY2025")
		sel, gran := DefaultSelection(quarters, fys)
		assert.Equal(t, "q", gran)
		assert.Equal(t, []string{"2026Q1", "2026Q2", "2026Q3"}, periodNames(sel))
		for _, p := range sel {
			assert.NotContains(t, p.Period, "FY", "不得伪造未凑齐的财年")
		}
	})

	t.Run("only fiscal years", func(t *testing.T) {
		fys := periodsNamed("FY2025", "FY2026")
		sel, gran := DefaultSelection(nil, fys)
		assert.Equal(t, "fy", gran)
		assert.Equal(t, []string{"FY2025", "FY2026"}, periodNames(sel))
	})
}

func rowByKey(t *testing.T, rows []MatrixRow, key string) MatrixRow {
	t.Helper()
	for _, r := range rows {
		if r.Key == key {
			return r
		}
	}
	keys := make([]string, 0, len(rows))
	for _, r := range rows {
		keys = append(keys, r.Key)
	}
	t.Fatalf("row %s not found in %v", key, keys)
	return MatrixRow{}
}

func qm(period string, rev, gp, oi, ni float64, segs map[string]float64) PeriodMetrics {
	return PeriodMetrics{
		Period: period, Segments: segs,
		Revenue: rev, GrossProfit: gp, OperatingIncome: oi, NetIncome: ni,
		GrossMargin: gp / rev, OpMargin: oi / rev, NetMargin: ni / rev,
	}
}

func TestBuildMatrixYoY(t *testing.T) {
	all := []PeriodMetrics{
		qm("2025Q1", 100*b, 60*b, 30*b, 24*b, map[string]float64{"cloud": 60 * b, "devices": 40 * b}),
		qm("2025Q2", 110*b, 66*b, 33*b, 26*b, map[string]float64{"cloud": 66 * b, "devices": 44 * b}),
		qm("2025Q3", 120*b, 72*b, 36*b, 28*b, map[string]float64{"cloud": 72 * b, "devices": 48 * b}),
		qm("2026Q1", 150*b, 90*b, 45*b, 36*b, map[string]float64{"cloud": 90 * b, "devices": 60 * b}),
		qm("2026Q2", 165*b, 99*b, 50*b, 40*b, map[string]float64{"cloud": 99 * b}),
	}
	sel := all[3:]

	rows := BuildMatrix(sel, twoSegTemplate(), "zh", all)
	require.NotEmpty(t, rows)

	rev := rowByKey(t, rows, "revenue")
	assert.Equal(t, []float64{150 * b, 165 * b}, rev.Values, "列 = 各选中期")
	assert.Equal(t, "yoy", rev.ChangeKind)
	assert.InDelta(t, 165.0/110.0-1, rev.Change, 1e-9, "季度末列应对比去年同期（2026Q2 vs 2025Q2）")

	cloud := rowByKey(t, rows, "cloud")
	assert.Equal(t, "云业务", cloud.Label)
	assert.Equal(t, []float64{90 * b, 99 * b}, cloud.Values)
	assert.InDelta(t, 99.0/66.0-1, cloud.Change, 1e-9)

	// 2026Q2 缺 devices 分部值 → NaN（而不是 0：0 会谎称该分部收入归零）。
	devices := rowByKey(t, rows, "devices")
	require.Len(t, devices.Values, 2)
	assert.Equal(t, 60*b, devices.Values[0])
	assert.True(t, math.IsNaN(devices.Values[1]), "缺失的分部值应为 NaN，实得 %v", devices.Values[1])

	// 三个比率行也在矩阵内。
	assert.InDelta(t, 0.6, rowByKey(t, rows, "gross_margin").Values[0], 1e-9)
	assert.InDelta(t, 0.3, rowByKey(t, rows, "op_margin").Values[0], 1e-9)
	assert.InDelta(t, 0.24, rowByKey(t, rows, "net_margin").Values[0], 1e-9)
}

func TestBuildMatrixMissingComparison(t *testing.T) {
	// allQuarters 里没有 2025Q1，去年同期缺失。
	sel := []PeriodMetrics{qm("2026Q1", 150*b, 90*b, 45*b, 36*b, nil)}
	rows := BuildMatrix(sel, twoSegTemplate(), "zh", sel)

	rev := rowByKey(t, rows, "revenue")
	assert.Equal(t, "none", rev.ChangeKind, "缺对照期 → ChangeKind none")
	assert.True(t, math.IsNaN(rev.Change), "缺对照期 → Change NaN，实得 %v", rev.Change)
}

// AD-14：对照期存在但不可用（0 或 NaN）时必须返回 NaN，绝不能是 ±Inf。
func TestBuildMatrixDivisionGuards(t *testing.T) {
	t.Run("year ago is zero", func(t *testing.T) {
		all := []PeriodMetrics{
			qm("2025Q1", 0, 0, 0, 0, nil),
			qm("2026Q1", 150*b, 90*b, 45*b, 36*b, nil),
		}
		rows := BuildMatrix(all[1:], twoSegTemplate(), "zh", all)
		rev := rowByKey(t, rows, "revenue")
		assert.True(t, math.IsNaN(rev.Change), "去年同期为 0 → NaN，实得 %v", rev.Change)
		assert.False(t, math.IsInf(rev.Change, 0), "±Inf 会让 encoding/json 报错并使 API 返回 500")
	})

	t.Run("year ago is NaN", func(t *testing.T) {
		all := []PeriodMetrics{
			qm("2025Q1", math.NaN(), math.NaN(), math.NaN(), math.NaN(), nil),
			qm("2026Q1", 150*b, 90*b, 45*b, 36*b, nil),
		}
		rows := BuildMatrix(all[1:], twoSegTemplate(), "zh", all)
		for _, key := range []string{"revenue", "gross_profit", "operating_income", "net_income"} {
			r := rowByKey(t, rows, key)
			assert.True(t, math.IsNaN(r.Change), "%s：去年同期为 NaN → NaN，实得 %v", key, r.Change)
			assert.False(t, math.IsInf(r.Change, 0), "%s 绝不能是 ±Inf", key)
		}
	})

	t.Run("no Inf anywhere in the matrix", func(t *testing.T) {
		all := []PeriodMetrics{
			qm("2025Q1", 0, 0, 0, 0, map[string]float64{"cloud": 0}),
			qm("2026Q1", 150*b, 90*b, 45*b, 36*b, map[string]float64{"cloud": 90 * b}),
		}
		rows := BuildMatrix(all[1:], twoSegTemplate(), "zh", all)
		require.NotEmpty(t, rows)
		for _, r := range rows {
			assert.False(t, math.IsInf(r.Change, 0), "row %s 的 Change 是 Inf", r.Key)
			for i, v := range r.Values {
				assert.False(t, math.IsInf(v, 0), "row %s 第 %d 列是 Inf", r.Key, i)
			}
		}
	})
}

func TestBuildMatrixAnnualChange(t *testing.T) {
	fy := func(name string, rev float64) PeriodMetrics {
		return qm(name, rev, rev*0.6, rev*0.3, rev*0.24, map[string]float64{"cloud": rev * 0.6})
	}

	t.Run("three or more periods use CAGR", func(t *testing.T) {
		sel := []PeriodMetrics{fy("FY2024", 100*b), fy("FY2025", 121*b), fy("FY2026", 144*b)}
		rows := BuildMatrix(sel, twoSegTemplate(), "zh", nil)
		rev := rowByKey(t, rows, "revenue")
		assert.Equal(t, "cagr", rev.ChangeKind)
		assert.InDelta(t, math.Pow(144.0/100.0, 1.0/2.0)-1, rev.Change, 1e-9, "区间 CAGR 跨 n-1 期")
	})

	t.Run("two periods fall back to yoy", func(t *testing.T) {
		sel := []PeriodMetrics{fy("FY2025", 100*b), fy("FY2026", 130*b)}
		rows := BuildMatrix(sel, twoSegTemplate(), "zh", nil)
		rev := rowByKey(t, rows, "revenue")
		assert.Equal(t, "yoy", rev.ChangeKind, "年度 2 期 → 对比上年")
		assert.InDelta(t, 0.30, rev.Change, 1e-9)
	})

	t.Run("single period has no comparison", func(t *testing.T) {
		rows := BuildMatrix([]PeriodMetrics{fy("FY2026", 100*b)}, twoSegTemplate(), "zh", nil)
		rev := rowByKey(t, rows, "revenue")
		assert.Equal(t, "none", rev.ChangeKind)
		assert.True(t, math.IsNaN(rev.Change))
	})

	t.Run("CAGR start not positive", func(t *testing.T) {
		sel := []PeriodMetrics{fy("FY2024", 0), fy("FY2025", 10*b), fy("FY2026", 20*b)}
		rows := BuildMatrix(sel, twoSegTemplate(), "zh", nil)
		rev := rowByKey(t, rows, "revenue")
		assert.True(t, math.IsNaN(rev.Change), "起点 ≤0 时 CAGR 无意义 → NaN，实得 %v", rev.Change)
		assert.False(t, math.IsInf(rev.Change, 0))
	})

	// 起点与终点同为负数时两负相除得正比值，会算出一个「看着合理」的正增长率
	// （-10b → -20b 得 +41%），比 Inf 更危险，因为它不会被任何 Inf 防护拦住。
	t.Run("CAGR from negative to negative", func(t *testing.T) {
		sel := []PeriodMetrics{fy("FY2024", -10*b), fy("FY2025", -15*b), fy("FY2026", -20*b)}
		rows := BuildMatrix(sel, twoSegTemplate(), "zh", nil)
		rev := rowByKey(t, rows, "revenue")
		assert.True(t, math.IsNaN(rev.Change), "起点为负时不得产出正增长率，实得 %v", rev.Change)
	})
}

func TestBuildMatrixLang(t *testing.T) {
	sel := []PeriodMetrics{qm("FY2026", 100*b, 60*b, 30*b, 24*b, map[string]float64{"cloud": 60 * b})}
	zh := BuildMatrix(sel, twoSegTemplate(), "zh", nil)
	en := BuildMatrix(sel, twoSegTemplate(), "en", nil)
	assert.Equal(t, "云业务", rowByKey(t, zh, "cloud").Label)
	assert.Equal(t, "Cloud", rowByKey(t, en, "cloud").Label)
	assert.NotEqual(t, rowByKey(t, zh, "revenue").Label, rowByKey(t, en, "revenue").Label,
		"主干科目的标签也应随 lang 切换")
}

func linkValue(t *testing.T, g Graph, source, target string) float64 {
	t.Helper()
	for _, l := range g.Links {
		if l.Source == source && l.Target == target {
			return l.Value
		}
	}
	t.Fatalf("link %s→%s not found in %v", source, target, linkNames(g))
	return 0
}

func hasLink(g Graph, source, target string) bool {
	for _, l := range g.Links {
		if l.Source == source && l.Target == target {
			return true
		}
	}
	return false
}

func linkNames(g Graph) []string {
	out := make([]string, 0, len(g.Links))
	for _, l := range g.Links {
		out = append(out, l.Source+"→"+l.Target)
	}
	return out
}

func hasNode(g Graph, name string) bool {
	for _, n := range g.Nodes {
		if n.Name == name {
			return true
		}
	}
	return false
}

// AD-13：真实数据里 Σsegments ≠ Revenue、RnD+SGnA ≠ opex 都是常态，
// 测试数据**故意让两处残差非零**，否则守恒断言根本没测到残差节点。
func balancedPeriod() PeriodMetrics {
	return PeriodMetrics{
		Period:          "FY2026",
		Segments:        map[string]float64{"cloud": 40 * b, "devices": 23 * b}, // Σ=63b，比 Revenue 少 2.585b
		Revenue:         65.585 * b,
		GrossProfit:     45 * b, // cogs = 20.585b
		OperatingIncome: 30 * b, // opex = 15b
		RnD:             7.5 * b,
		SGnA:            6 * b, // other_opex = 1.5b
		NetIncome:       24.7 * b,
		IncomeTax:       5 * b, // tax_other = 30b - 24.7b = 5.3b
	}
}

func TestBuildSankeyBalance(t *testing.T) {
	p := balancedPeriod()
	g := BuildSankey(p, twoSegTemplate(), "zh")

	// 两个残差节点必须存在且非零，否则本用例的守恒断言是空洞的。
	otherSeg := linkValue(t, g, "其他分部", "收入")
	otherOpex := linkValue(t, g, "营业费用", "其他营业费用")
	assert.InDelta(t, 2.585*b, otherSeg, 1, "other_segment = Revenue − Σsegments")
	assert.InDelta(t, 1.5*b, otherOpex, 1, "other_opex = (GrossProfit − OperatingIncome) − RnD − SGnA")
	require.NotZero(t, otherSeg, "残差为 0 的测试数据测不到残差节点")
	require.NotZero(t, otherOpex)

	assert.InDelta(t, 40*b, linkValue(t, g, "云业务", "收入"), 1)
	assert.InDelta(t, 23*b, linkValue(t, g, "硬件设备", "收入"), 1)
	assert.InDelta(t, 20.585*b, linkValue(t, g, "收入", "营业成本"), 1)
	assert.InDelta(t, 45*b, linkValue(t, g, "收入", "毛利"), 1)
	assert.InDelta(t, 15*b, linkValue(t, g, "毛利", "营业费用"), 1)
	assert.InDelta(t, 30*b, linkValue(t, g, "毛利", "营业利润"), 1)
	assert.InDelta(t, 7.5*b, linkValue(t, g, "营业费用", "研发费用"), 1)
	assert.InDelta(t, 6*b, linkValue(t, g, "营业费用", "销售及管理费用"), 1)
	assert.InDelta(t, 24.7*b, linkValue(t, g, "营业利润", "净利润"), 1)
	assert.InDelta(t, 5.3*b, linkValue(t, g, "营业利润", "税项及其他"), 1,
		"tax_other = OperatingIncome − NetIncome")

	// 中间节点入流 ≈ 出流（±1）。
	for _, name := range []string{"收入", "毛利", "营业费用", "营业利润"} {
		var in, out float64
		for _, l := range g.Links {
			if l.Target == name {
				in += l.Value
			}
			if l.Source == name {
				out += l.Value
			}
		}
		assert.InDelta(t, in, out, 1, "节点 %s 入流 %v 与出流 %v 不守恒", name, in, out)
	}

	for _, l := range g.Links {
		assert.GreaterOrEqual(t, l.Value, 0.0, "link %s→%s 不得为负", l.Source, l.Target)
	}
}

// 残差小于 Revenue×0.5% 时省略该节点（避免图上出现无意义的毛刺）。
func TestBuildSankeyResidualOmitted(t *testing.T) {
	p := balancedPeriod()
	p.Segments = map[string]float64{"cloud": 40 * b, "devices": 25.5 * b} // Σ=65.5b，差 0.085b < 0.328b
	p.RnD = 9 * b
	p.SGnA = 5.9 * b // other_opex = 0.1b < 0.328b

	g := BuildSankey(p, twoSegTemplate(), "zh")
	assert.False(t, hasNode(g, "其他分部"), "残差 |差| < Revenue×0.5% 时应省略 other_segment")
	assert.False(t, hasNode(g, "其他营业费用"), "同理应省略 other_opex")
	assert.False(t, hasLink(g, "其他分部", "收入"))
	assert.False(t, hasLink(g, "营业费用", "其他营业费用"))

	// 省略残差后主干仍在。
	assert.InDelta(t, 45*b, linkValue(t, g, "收入", "毛利"), 1)
}

// 利息收入大的公司会出现 NetIncome > OperatingIncome，使 tax_other 为负：
// 不画负流，改为 0 宽 + footnote 记账（守恒断言只对非负流成立）。
func TestBuildSankeyNegativeFlow(t *testing.T) {
	p := balancedPeriod()
	p.NetIncome = 33 * b // > OperatingIncome 30b → tax_other = -3b

	g := BuildSankey(p, twoSegTemplate(), "zh")
	for _, l := range g.Links {
		assert.GreaterOrEqual(t, l.Value, 0.0, "link %s→%s 画了负流", l.Source, l.Target)
		assert.False(t, math.IsInf(l.Value, 0))
	}
	assert.Zero(t, linkValue(t, g, "营业利润", "税项及其他"), "负值应以 0 宽表示")
	require.NotEmpty(t, g.Notes, "负残差必须被显式记录，否则图上凭空少一块而无人知情")
	assert.Contains(t, strings.Join(g.Notes, "\n"), "税项及其他", "footnote 应点明是哪个节点为负")
}

func TestBuildSankeyLang(t *testing.T) {
	p := balancedPeriod()
	en := BuildSankey(p, twoSegTemplate(), "en")
	assert.True(t, hasNode(en, "Cloud"), "分部节点名应取 NameEN，实得 %v", nodeNames(en))
	assert.True(t, hasNode(en, "Revenue"), "主干节点名也应随 lang 切换")
	assert.True(t, hasNode(en, "Other Segments"))
	assert.False(t, hasNode(en, "云业务"))

	zh := BuildSankey(p, twoSegTemplate(), "zh")
	assert.True(t, hasNode(zh, "云业务"))
	assert.True(t, hasNode(zh, "收入"))
}

func nodeNames(g Graph) []string {
	out := make([]string, 0, len(g.Nodes))
	for _, n := range g.Nodes {
		out = append(out, n.Name)
	}
	return out
}

// NaN 保持 NaN 交前端渲染 "—"，但绝不能变成 Inf。
func TestBuildSankeyNaNStaysNaN(t *testing.T) {
	p := balancedPeriod()
	p.GrossProfit = math.NaN()

	g := BuildSankey(p, twoSegTemplate(), "zh")
	for _, l := range g.Links {
		assert.False(t, math.IsInf(l.Value, 0), "link %s→%s 是 Inf", l.Source, l.Target)
	}
	for _, n := range g.Nodes {
		assert.False(t, math.IsInf(n.Value, 0), "node %s 是 Inf", n.Name)
	}
	assert.True(t, math.IsNaN(linkValue(t, g, "收入", "毛利")), "缺值应保持 NaN")
}
