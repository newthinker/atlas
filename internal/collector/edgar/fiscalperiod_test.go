package edgar

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Context Checkpoint: done_criteria → test mapping
//   [TASK-017] FiscalPeriod 治本:让标签成为 period_end 的纯函数
//   functional[0] 标签是 period_end 的纯函数(不随哪份报告胜出而变) → TestFiscalPeriodIsPureFunctionOfPeriodEnd
//   functional[1] 真实 MSFT 的 4 组冲突全部解开且语义正确        → TestFiscalPeriodResolvesRealConflicts
//   functional[2] 非日历财年:6 月结 / 12 月结                    → TestFiscalPeriodJuneFY / TestFiscalPeriodDecemberFY
//   functional[3] 财年结束月由 isAnnual 条目推导;无年度条目降级   → TestFiscalPeriodDegradesWithoutAnnualEntries
//   boundary[0]   含端点:财年末当天属**上一**财年 Q4            → TestFiscalPeriodFiscalYearEndIsQ4(表驱动)
//   boundary[1]   AD-18 golden 只允许第一列变化                  → TestFetchCompanyFactsGolden(golden_test.go)
//
// 真实冲突组经三重验证(真实 DB fundamental_q / 复刻 applyDuration / 年度条目按 fy 分组):
// 恰好 4 组、最大 ×3。需求文档最初列的 7 组里有 3 组是**时长过滤前**的原始 fp=FY 累计期
// (标签实为 2018FY 而非 2018Q4),被 isSingleQuarter/isAnnual 丢弃,永远进不到输出。

// labelsOf 取 period_end → FiscalPeriod 映射,便于逐期锚定断言。
func labelsOf(t *testing.T, fixture string) map[string]string {
	t.Helper()
	srv, _ := factsFileServer(t, fixture, "/api/xbrl/companyfacts/CIK0001045810.json")
	defer srv.Close()
	c := NewWithBaseURL("ua (t@e.com)", srv.URL)
	facts, err := c.FetchCompanyFacts("1045810")
	require.NoError(t, err)
	out := make(map[string]string, len(facts))
	for _, f := range facts {
		out[f.PeriodEnd.Format("2006-01-02")] = f.FiscalPeriod
	}
	return out
}

// assertLabelsUnique 断言不存在两个 period_end 共用同一标签 —— 即本任务要消除的冲突形态。
func assertLabelsUnique(t *testing.T, labels map[string]string) {
	t.Helper()
	seen := map[string]string{}
	for end, label := range labels {
		if prev, dup := seen[label]; dup {
			t.Errorf("标签 %s 同时出现在 %s 与 %s", label, prev, end)
		}
		seen[label] = end
	}
}

// functional[1]:真实 MSFT 的 4 组标签冲突必须全部解开,且解开后的值**语义正确**
// (不是随便给个唯一值)。断言逐期锚定具体标签 —— 只断言"互不重复"挡不住整体错位一格。
func TestFiscalPeriodResolvesRealConflicts(t *testing.T) {
	got := labelsOf(t, "testdata/companyfacts_fy_june.json")

	for _, tc := range []struct{ end, want, why string }{
		// 2025Q4×3:一份 10-K 的三个年度条目全标 fy=2025
		{"2023-06-30", "2023Q4", "FY2023 的年度期末"},
		{"2024-06-30", "2024Q4", "FY2024 的年度期末"},
		{"2025-06-30", "2025Q4", "FY2025 的年度期末"},
		// 2026Q1×2 / 2026Q2×2 / 2026Q3×2:去年同季被较晚 10-Q 重述后继承当期上下文
		{"2024-09-30", "2025Q1", "6 月结财年:9 月末是下一财年的 Q1"},
		{"2025-09-30", "2026Q1", ""},
		{"2024-12-31", "2025Q2", ""},
		{"2025-12-31", "2026Q2", ""},
		{"2025-03-31", "2025Q3", ""},
		{"2026-03-31", "2026Q3", ""},
	} {
		assert.Equal(t, tc.want, got[tc.end], "period_end %s %s", tc.end, tc.why)
	}

	assertLabelsUnique(t, got)
}

// functional[0]:同一 period_end 在 fixture 里有两条申报(原始 fy=2025/Q3 与重述 fy=2026/Q3),
// 去重取 filed 更晚者。标签必须与"哪条胜出"无关 —— 这是"纯函数"的可观测判据。
func TestFiscalPeriodIsPureFunctionOfPeriodEnd(t *testing.T) {
	got := labelsOf(t, "testdata/companyfacts_fy_june.json")
	assert.Equal(t, "2025Q3", got["2025-03-31"],
		"胜出条目携带 fy=2026/fp=Q3,若标签仍取自 EDGAR 上下文会得到 2026Q3")
	assert.Equal(t, "2026Q3", got["2026-03-31"],
		"两个 period_end 的上下文标签相同(都是 2026Q3),推导后必须分开")
}

// functional[2] + boundary[0]:6 月结财年逐期锚定。
func TestFiscalPeriodJuneFY(t *testing.T) {
	got := labelsOf(t, "testdata/companyfacts_fy_june.json")
	for end, want := range map[string]string{
		"2022-09-30": "2023Q1", "2022-12-31": "2023Q2", "2023-03-31": "2023Q3", "2023-06-30": "2023Q4",
		"2023-09-30": "2024Q1", "2023-12-31": "2024Q2", "2024-03-31": "2024Q3", "2024-06-30": "2024Q4",
	} {
		assert.Equal(t, want, got[end], "period_end %s", end)
	}
}

// functional[2] + boundary[0]:12 月结(自然年)财年。现有 fixture 清一色 1 月结,
// 12 月是最常见形态且含端点边界在此症状最明显。
func TestFiscalPeriodDecemberFY(t *testing.T) {
	got := labelsOf(t, "testdata/companyfacts_fy_december.json")
	for end, want := range map[string]string{
		"2024-03-31": "2024Q1", "2024-06-30": "2024Q2", "2024-09-30": "2024Q3", "2024-12-31": "2024Q4",
		"2025-03-31": "2025Q1", "2025-06-30": "2025Q2", "2025-09-30": "2025Q3", "2025-12-31": "2025Q4",
	} {
		assert.Equal(t, want, got[end], "period_end %s", end)
	}
}

// boundary[0] 含端点陷阱的专项对偶断言:财年末当天必须归**上一**财年的 Q4,而非新财年 Q1。
// 若比较写成不含端点,所有 Q4 会整体后移一年(2025-06-30 → 2026Q4),**而标签依然互不重复**,
// 只断言"无冲突"的弱断言会全绿放行。故此处锚定具体值,并同时断言"不等于错位值"。
func TestFiscalPeriodFiscalYearEndIsQ4(t *testing.T) {
	for _, tc := range []struct{ fixture, end, want, wrong string }{
		{"testdata/companyfacts_fy_june.json", "2025-06-30", "2025Q4", "2026Q4"},
		{"testdata/companyfacts_fy_june.json", "2024-06-30", "2024Q4", "2025Q4"},
		{"testdata/companyfacts_fy_december.json", "2025-12-31", "2025Q4", "2026Q4"},
		{"testdata/companyfacts_fy_december.json", "2024-12-31", "2024Q4", "2025Q4"},
	} {
		got := labelsOf(t, tc.fixture)
		assert.Equal(t, tc.want, got[tc.end], "%s 财年末当天属上一财年 Q4", tc.end)
		assert.NotEqual(t, tc.wrong, got[tc.end], "%s 被整体错位一格(比较漏了等号)", tc.end)
	}
}

// functional[3] 降级:无年度条目 → 推不出财年结束月 → 回退 EDGAR 原始 fy/fp,
// 并**记录可观测日志**(不得静默产出可能冲突的标签)。
// companyfacts_rev_fallback 是 9 份 golden 里**唯一**无年度条目的,其 golden 基线是降级产物。
func TestFiscalPeriodDegradesWithoutAnnualEntries(t *testing.T) {
	logs := captureLogs(t)

	got := labelsOf(t, "testdata/companyfacts_rev_fallback.json")
	assert.Equal(t, "2026Q1", got["2025-04-27"], "降级时回退 EDGAR 原始 fy/fp")
	assert.Contains(t, strings.ToLower(logs.String()), "annual",
		"降级必须可观测,不能静默")
}

// boundary[0] 月界漂移:52/53 周财报日历的期末会漂到次月头几天。q4guard fixture(1 月结)里
// 2022-05-01 覆盖 2~4 月(应为 Q1)、2022-07-31 覆盖 5~7 月(应为 Q2) —— 若按 end.Month()
// 直接算季序,两者都会得到 Q2 而**共用同一标签**,正是本任务要消除的那类冲突。
func TestFiscalPeriodMonthBoundaryDrift(t *testing.T) {
	got := labelsOf(t, "testdata/companyfacts_q4guard.json")
	for end, want := range map[string]string{
		"2022-05-01": "2023Q1", // 月初 → 归上一个月(4 月)
		"2022-07-31": "2023Q2",
		"2022-10-30": "2023Q3",
	} {
		assert.Equal(t, want, got[end], "period_end %s", end)
	}
	assertLabelsUnique(t, got)
}

// 负向锚点(Leader 建议):年报里披露的**季度长度**数据带 fp=FY(90 天),
// isSingleQuarter 要求 fp∈{Q1,Q2,Q3}、isAnnual 要求 350~380 天,两道过滤都不收,
// 故不得产出任何季度。此前这条过滤无测试锚定 —— 正是需求文档最初把 7 组冲突算错的根源
// (统计时漏了 fp 检查,把这些被过滤掉的条目当成了输出)。若有人放宽 fp 检查
// (「duration 够了就行」),本用例会立刻变红。
func TestFiscalPeriodRejectsAnnualTaggedQuarterlyEntry(t *testing.T) {
	got := labelsOf(t, "testdata/companyfacts_fy_june.json")
	_, exists := got["2026-06-30"]
	assert.False(t, exists,
		"fp=FY 且 90 天的条目必须被两道时长/fp 过滤挡下,不得产出季度(实际得到标签 %q)", got["2026-06-30"])
}

// ---------------------------------------------------------------------------
// [TASK-018] 两个推导 helper 的直接锚点。
//
// 它们此前**只被上层用例间接执行**:覆盖率把两处算作「已覆盖」,但被改坏时没有任何
// 断言会红 —— 覆盖率度量的是「执行到」,不是「被约束住」。本组用例补的是后者。
//   functional[0] anchorYearMonth 三档阈值(14/15/16)   → TestAnchorYearMonthDayThreshold
//   functional[1] fiscalYearEndMonth 取众数            → TestFiscalYearEndMonthPicksMode
//   boundary[0]   anchorYearMonth 跨年回绕             → TestAnchorYearMonthYearWraparound
//   boundary[1]   平票取较小月份且稳定(连跑 ≥20 次)     → TestFiscalYearEndMonthTieBreakIsStable
//   error[0]      无年度事实 → ok=false 且 month 为零值 → TestFiscalYearEndMonthWithoutAnnualFacts
// ---------------------------------------------------------------------------

// annualFact 造一条能通过 isAnnual(fp=FY 且 350~380 天)的年度事实,期末为 end。
func annualFact(t *testing.T, end string) rawFact {
	t.Helper()
	e, err := time.Parse(dateLayout, end)
	require.NoError(t, err)
	f := rawFact{Start: e.AddDate(-1, 0, 1).Format(dateLayout), End: end, FP: "FY", Filed: end}
	require.True(t, isAnnual(f), "fixture 前提:%s 必须被 isAnnual 接受", end)
	return f
}

// gaapWithAnnualEnds 把各 end 打包成 fiscalYearEndMonth 的入参。它对所有 tag×unit 一视同仁地
// 投票,与事实挂在哪个 tag/unit 下无关,故全部塞进单个 tag×unit 即可覆盖。
func gaapWithAnnualEnds(t *testing.T, ends ...string) map[string]tagFacts {
	t.Helper()
	facts := make([]rawFact, 0, len(ends))
	for _, e := range ends {
		facts = append(facts, annualFact(t, e))
	}
	return map[string]tagFacts{"Revenues": {Units: map[string][]rawFact{"USD": facts}}}
}

// functional[0]:阈值三档必须同时存在 —— 只有 15/16 无法区分 `> 15` 与 `>= 15`;
// 只有 14/16 无法定位阈值落在哪一天。三档一起才把 `end.Day() > 15` 钉死。
func TestAnchorYearMonthDayThreshold(t *testing.T) {
	for _, tc := range []struct {
		day, wantY, wantM int
		why               string
	}{
		{14, 2025, 5, "day<=15 归上一个月"},
		{15, 2025, 5, "15 仍算月初漂移(阈值是 > 15,不是 >= 15)"},
		{16, 2025, 6, "16 起才算本月"},
	} {
		y, m := anchorYearMonth(time.Date(2025, time.June, tc.day, 0, 0, 0, 0, time.UTC))
		assert.Equal(t, tc.wantY, y, "2025-06-%02d 年份:%s", tc.day, tc.why)
		assert.Equal(t, tc.wantM, m, "2025-06-%02d 月份:%s", tc.day, tc.why)
	}
}

// boundary[0]:1 月且 day<=15 时回绕到上一年 12 月。断言年份**确实减 1**、月份为 12
// (而非 0 或 -1 这种朴素 m-1 的产物)。
func TestAnchorYearMonthYearWraparound(t *testing.T) {
	y, m := anchorYearMonth(time.Date(2025, time.January, 5, 0, 0, 0, 0, time.UTC))
	assert.Equal(t, 2024, y, "1 月初归上一年")
	assert.Equal(t, 12, m, "月份回绕为 12,不能是 0 或 -1")

	// 对照:同为 1 月但 day>15 时不回绕。
	y2, m2 := anchorYearMonth(time.Date(2025, time.January, 25, 0, 0, 0, 0, time.UTC))
	assert.Equal(t, 2025, y2)
	assert.Equal(t, 1, m2)
}

// functional[1]:少数派月份(1 票,月份值 **3**)与多数派(5 票,月份值 **6**)共存。
// 少数派月份值刻意**小于**多数派 —— 否则「取首个」或「取较小」的错误实现会与「取众数」
// 给出同一答案,用例就是空洞的。
func TestFiscalYearEndMonthPicksMode(t *testing.T) {
	gaap := gaapWithAnnualEnds(t,
		"2021-03-31",                                                         // 少数派:3 月,1 票
		"2020-06-30", "2021-06-30", "2022-06-30", "2023-06-30", "2024-06-30", // 多数派:6 月,5 票
	)
	m, ok := fiscalYearEndMonth(gaap)
	require.True(t, ok)
	assert.Equal(t, 6, m, "取票数最多的月份,而非最小的(3)或首个遇到的")
}

// boundary[1]:两个月份票数完全相同时恒取较小者。**必须连跑多次** —— Go 的 map 迭代序
// 每次 range 都随机,单次调用对「稳定性」零证明力:丢掉平票规则的实现有约一半概率碰巧给对。
func TestFiscalYearEndMonthTieBreakIsStable(t *testing.T) {
	gaap := gaapWithAnnualEnds(t,
		"2021-03-31", "2022-03-31", // 3 月,2 票
		"2021-09-30", "2022-09-30", // 9 月,2 票
	)
	const runs = 50
	for i := 0; i < runs; i++ {
		m, ok := fiscalYearEndMonth(gaap)
		require.True(t, ok)
		require.Equal(t, 3, m, "第 %d/%d 次调用:平票必须恒取较小月份,不得随 map 迭代序抖动", i+1, runs)
	}
}

// error[0]:全部事实都被 isAnnual 过滤掉时返回 ok=false,且 month 为零值 ——
// 调用方(FetchCompanyFacts)据此走降级分支,绝不能把 0 当成有效月份用。
func TestFiscalYearEndMonthWithoutAnnualFacts(t *testing.T) {
	quarterly := rawFact{Start: "2025-01-01", End: "2025-03-31", FP: "Q1", Filed: "2025-04-25"}
	require.False(t, isAnnual(quarterly), "fixture 前提:该条必须被 isAnnual 拒绝")

	for name, gaap := range map[string]map[string]tagFacts{
		"只有季度事实":  {"Revenues": {Units: map[string][]rawFact{"USD": {quarterly}}}},
		"空 units": {"Revenues": {Units: map[string][]rawFact{}}},
		"空 gaap":  {},
	} {
		m, ok := fiscalYearEndMonth(gaap)
		assert.False(t, ok, "%s:应报告无法推导", name)
		assert.Zero(t, m, "%s:month 必须是零值,调用方不得当作有效月份", name)
	}
}
