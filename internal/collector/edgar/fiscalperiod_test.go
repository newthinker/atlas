package edgar

import (
	"strings"
	"testing"

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
