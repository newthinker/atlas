package web

import (
	"math"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/newthinker/atlas/internal/prism/sankey"
)

// Context Checkpoint: done_criteria → test mapping (TASK-004 双轴时间错位)
//
//	functional[0] 同一横向位置 = 同一时间（五点断言）
//	              → TestClosesAtPeriodEnds（重采样本身）
//	                TestFundamentalPageAlignsPriceToPeriods（跑到渲染出的 payload）
//	functional[1] 实现路径 = (b) 报告期重采样 + 单条共享 x 轴
//	              → TestFundamentalChartUsesOneSharedXAxis
//	functional[2] dataZoom 拖动后仍对齐：单轴之下 dataZoom 只能同时作用于两条序列，
//	              「拖动后错位」这个失败模式被结构性消除
//	              → TestFundamentalChartUsesOneSharedXAxis（无 xAxisIndex / 无第二条轴）
//	                + 人工验收清单（真实拖动是渲染行为，Go 测不了，AD-7）
//	boundary[0]   股价起点之前的报告期：指标有值、股价为 null，且不得左延
//	              → TestClosesAtPeriodEnds/早于价格序列起点 + TestFundamentalPagePriceMissingBeforeFirstQuote
//	error_hand[0] closes 为空：不报错、指标线不消失
//	              → TestFundamentalPageEmptyPriceSeries
//	non_func[0]   两份模板逐字节一致 → 既有 TestFundamentalTemplateDirsInSync
//	non_func[1]   划分见 discovery：可测的落 Go，纯渲染进人工清单

// alignedFundamental 的价格日期**故意不是**报告期末的整齐子集：
// 2025-09-30 没有报价（最近的是 09-29），2025-03-31 早于整条价格序列。
// 收盘价两两不同 —— 只要重采样错位一格，下面的精确相等就必然红。
func alignedFundamental() *sankey.FundamentalView {
	return &sankey.FundamentalView{
		Symbol:     "MSFT",
		Periods:    []string{"2025Q1", "2025Q2", "2025Q3", "2025Q4", "2026Q1"},
		PeriodEnds: []string{"2025-03-31", "2025-06-30", "2025-09-30", "2025-12-31", "2026-03-31"},
		Metrics: map[string][]float64{
			"revenue": {1000, 1100, 1200, 1300, 1400},
		},
		Dates: []string{
			"2025-05-02", "2025-06-30", "2025-07-01",
			"2025-09-29", "2025-12-31", "2026-01-02", "2026-03-31",
		},
		Closes: []float64{10, 20, 21, 30, 40, 41, 50},
	}
}

// TestClosesAtPeriodEnds —— functional[0] 的核心：out[i] 必须是 periodEnds[i]
// 当天（或其之前最近一个交易日）的收盘价，而不是 closes[i]。
//
// 「按索引取 closes[i]」正是被修复的缺陷：两条 category 轴各自按索引均匀铺开，
// 于是 2009Q1 的营收与 2016-07 的股价被画在同一横向位置上。
func TestClosesAtPeriodEnds(t *testing.T) {
	dates := []string{"2025-05-02", "2025-06-30", "2025-09-29", "2025-12-31"}
	closes := []float64{10, 20, 30, 40}

	cases := []struct {
		name       string
		periodEnds []string
		dates      []string
		closes     []float64
		want       []float64 // NaN 表示必须为缺失
	}{
		{
			name:       "期末当天有报价：取当天",
			periodEnds: []string{"2025-06-30", "2025-12-31"},
			dates:      dates, closes: closes,
			want: []float64{20, 40},
		},
		{
			name:       "期末当天休市：取之前最近一个交易日",
			periodEnds: []string{"2025-09-30"}, // 09-29 是序列里最近的一天
			dates:      dates, closes: closes,
			want: []float64{30},
		},
		{
			// boundary[0]：不得把最早的收盘价向左延伸——那正是「越往左偏得越多」的另一种写法。
			// 第二个日期只差一天也照样缺失：往后取报价就是用未来的价格解释过去的报表。
			name:       "早于价格序列起点：缺失",
			periodEnds: []string{"2009-03-31", "2025-05-01"},
			dates:      dates, closes: closes,
			want: []float64{math.NaN(), math.NaN()},
		},
		{
			// 12-31 是周六时最后一个交易日是 12-30：这不是数据缺口，是休市。
			name:       "期末落在整条序列末尾之后一天（休市）：仍取最后一个报价",
			periodEnds: []string{"2026-01-01"},
			dates:      dates, closes: closes,
			want: []float64{40},
		},
		{
			// 一周是所有现实休市（含长假、9/11 四个交易日）的上界；超出就不是休市，
			// 而是这个标的的价格没同步上，须画成断点而不是一段假的横线。
			name:       "报价过期恰好一周 / 超过一周：前者仍取、后者缺失",
			periodEnds: []string{"2026-01-07", "2026-01-08"},
			dates:      dates, closes: closes,
			want: []float64{40, math.NaN()},
		},
		{
			// 序列中间的空洞同理：拿 5 月的收盘价去代表 6 月末，图上看不出任何异样。
			name:       "期末落在序列内部的空洞里：缺失",
			periodEnds: []string{"2025-06-01"},
			dates:      dates, closes: closes,
			want: []float64{math.NaN()},
		},
		{
			name:       "期末日期缺失或不合法：缺失（不 panic）",
			periodEnds: []string{"", "2025-13-45"},
			dates:      dates, closes: closes,
			want: []float64{math.NaN(), math.NaN()},
		},
		{
			name:       "整条价格序列为空：全部缺失",
			periodEnds: []string{"2025-06-30", "2025-12-31"},
			dates:      nil, closes: nil,
			want: []float64{math.NaN(), math.NaN()},
		},
		{
			name:       "没有报告期：空结果（不 panic）",
			periodEnds: nil,
			dates:      dates, closes: closes,
			want: []float64{},
		},
		{
			// 收盘价本身可能是 NULL（NaN），照原样传播为缺失，不许拿前一天顶替。
			name:       "当天收盘价为 NaN：仍是缺失",
			periodEnds: []string{"2025-09-29"},
			dates:      dates, closes: []float64{10, 20, math.NaN(), 40},
			want: []float64{math.NaN()},
		},
		{
			// FundamentalProvider 是接口，两条数组由实现方给出，长度不一致不得让页面 panic。
			name:       "dates 与 closes 长度不一致：不 panic",
			periodEnds: []string{"2025-06-30", "2025-12-31"},
			dates:      dates, closes: []float64{10, 20},
			want: []float64{20, math.NaN()},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := closesAtPeriodEnds(c.periodEnds, c.dates, c.closes)
			require.Len(t, got, len(c.periodEnds),
				"重采样结果必须与报告期一一对应——长度不等就没有「同一索引 = 同一时间」可言")
			for i, want := range c.want {
				if math.IsNaN(want) {
					assert.True(t, math.IsNaN(got[i]), "第 %d 期应缺失，实得 %v", i, got[i])
					continue
				}
				assert.Equal(t, want, got[i], "第 %d 期（%s）取错了收盘价", i, c.periodEnds[i])
			}
		})
	}
}

// TestFundamentalPageAlignsPriceToPeriods —— functional[0] 落到真实渲染路径。
//
// 五个报告期逐个断言（DoD 的 0%/25%/50%/75%/100% 五点在 category 轴上就是这五个索引）：
// 图上第 i 个横向位置的指标与股价必须同属第 i 个报告期。缺陷版按索引取 closes，
// 会得到 [10,20,21,30,40] —— 五个位置里四个错。
func TestFundamentalPageAlignsPriceToPeriods(t *testing.T) {
	rec := getFundamentalPage(t, newFundamentalHandler(t, &fakeFundamental{v: alignedFundamental()}), "MSFT")
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()

	assert.Contains(t, body, `"period_closes":[null,20,30,40,50]`,
		"股价必须按报告期末重采样：2025Q1 早于首个报价（null）、2025Q3 期末休市取 09-29 的 30")
	assert.NotContains(t, body, `"period_closes":[10,20,21,30,40]`,
		"这是按索引对齐的缺陷形态：把日频序列的前 5 个点当成 5 个报告期")
	assert.Contains(t, body, `"periods":["2025Q1","2025Q2","2025Q3","2025Q4","2026Q1"]`,
		"指标一侧的刻度不变——两侧同索引才谈得上同时间")
	assert.Contains(t, body, `"revenue":[1000,1100,1200,1300,1400]`,
		"指标序列必须原样保留")
}

// TestFundamentalChartUsesOneSharedXAxis —— functional[1] + functional[2]。
//
// 两条 category 轴（点数与跨度都不同）是错位的**根因**：轴按索引均匀铺开、
// dataZoom 又按百分比作用于索引，两条线只在最右端偶然对齐。修复方式是让股价与
// 指标共用同一条 x 轴，于是「未缩放」与「拖动后」的对齐都由结构保证，而不靠巧合。
//
// 两份模板都查：只改 embed 那份时单测全绿，而 atlas serve 读磁盘那份（AD-4）。
func TestFundamentalChartUsesOneSharedXAxis(t *testing.T) {
	for _, p := range []string{"templates/prism_fundamental.html", "../../templates/prism_fundamental.html"} {
		src, err := os.ReadFile(p)
		require.NoError(t, err, "%s 必须存在", p)
		page := string(src)

		assert.NotContains(t, page, "xAxisIndex", "%s: 出现 xAxisIndex 即意味着又有了第二条 x 轴——"+
			"两条轴按各自索引铺开，未缩放时就已错位（TASK-004）", p)
		assert.NotContains(t, page, "xAxis.push", "%s: 不得再往 xAxis 里追加第二条轴", p)
		assert.Contains(t, page, "period_closes", "%s: 股价必须取服务端按报告期重采样的 period_closes", p)
		assert.NotContains(t, page, "data.dates", "%s: 日频 dates/closes 已不再进页面 payload；"+
			"若要恢复日频股价，必须同时换成 time 轴，否则错位回归", p)
	}
}

// TestFundamentalPagePriceMissingBeforeFirstQuote —— boundary[0]。
//
// COST 的真实形态：营收从 2009Q1 起，股价从 2016-07-26 起。重合之前的那一段，
// 指标必须照画、股价必须断开——既不能把首个收盘价向左延伸，也不能让指标线消失。
func TestFundamentalPagePriceMissingBeforeFirstQuote(t *testing.T) {
	v := &sankey.FundamentalView{
		Symbol:     "COST",
		Periods:    []string{"2009Q1", "2009Q2", "2016Q4"},
		PeriodEnds: []string{"2009-03-31", "2009-06-30", "2016-12-31"},
		Metrics:    map[string][]float64{"revenue": {100, 200, 300}},
		Dates:      []string{"2016-07-26", "2016-12-30"},
		Closes:     []float64{150, 160},
	}
	rec := getFundamentalPage(t, newFundamentalHandler(t, &fakeFundamental{v: v}), "COST")
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()

	assert.Contains(t, body, `"period_closes":[null,null,160]`,
		"股价起点之前必须是断点；写成 150 就是把首个收盘价向左延伸了 7 年")
	assert.Contains(t, body, `"revenue":[100,200,300]`,
		"股价缺失不得连累指标序列——两条线各自缺各自的")
}

// TestFundamentalPageEmptyPriceSeries —— error_handling[0]。
// V 在 TASK-002 之前就是「有财报、无价格」的状态，是真实可达的场景。
func TestFundamentalPageEmptyPriceSeries(t *testing.T) {
	v := &sankey.FundamentalView{
		Symbol:     "V",
		Periods:    []string{"2025Q1", "2025Q2"},
		PeriodEnds: []string{"2025-03-31", "2025-06-30"},
		Metrics:    map[string][]float64{"revenue": {900, 950}},
	}
	rec := getFundamentalPage(t, newFundamentalHandler(t, &fakeFundamental{v: v}), "V")
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()

	assert.Contains(t, body, `"period_closes":[null,null]`,
		"无价格数据时每个报告期都是缺失——长度仍与 periods 一致，索引对应关系不塌")
	assert.Contains(t, body, `"revenue":[900,950]`, "指标线必须照常渲染")
	assert.Contains(t, body, `id="price-toggle"`, "叠加开关仍在（勾选后只是画不出股价，不报错）")
}
