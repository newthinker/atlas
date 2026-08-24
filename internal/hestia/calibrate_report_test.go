package hestia

import (
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Context Checkpoint: done_criteria → test mapping（M1c-2 的 TASK-003）
// functional[0] nearest-rank 分位数，1..100 → P5=5/Median=50/P95=95/Min=1/Max=100/N=100
//                                            → TestComputeFieldStatsUsesNearestRank
// functional[0] 附带：不得就地排序调用方的切片        → TestComputeFieldStatsDoesNotMutateInput
// boundary[0]   加性余量 [min-span, max+span]，负值样本 → TestSuggestionIsAdditiveNotMultiplicative
// boundary[0]   上界 K：挡住 ±MaxFloat64 那种「怎么都行」 → 同上（K 常量的理由见其注释）
// boundary[1]   n<3 不给建议，n=2 两值**不同**          → TestSuggestionWithheldBelowMinSamples
//
// functional[1]/[2]/[3] 的渲染部分见本文件后半 / 待 DoD 矛盾裁决（questions[0]）

// shuffled1to100 造 1..100 的乱序切片。
//
// 乱序是**必要**的：顺序输入下「压根没排序、直接取下标」的实现同样能得出
// P5=5 / Median=50 / P95=95，这条用例就平凡通过了。
func shuffled1to100() []float64 {
	out := make([]float64, 0, 100)
	// 固定的、非单调的置换：((i+1)*37)%100，37 与 100 互质 ⇒ 恰好遍历 0..99 各一次。
	// 不用 math/rand：夹具必须逐次可复现，否则失败无法重跑。
	//
	// ⚠️ 为什么是 (i+1) 而不是 i：写成 i 时首元素恰好是 1，也就是最小值 ——
	// 下面那条前提断言当场把它逮住了。**这条前提不是装饰**，它挡的正是
	// 「夹具悄悄退化成有序/半有序，于是不排序的实现也能通过」。
	for i := range 100 {
		out = append(out, float64(((i+1)*37)%100+1))
	}
	return out
}

// 分位数用 nearest-rank（rank = ceil(q*n)，取第 rank 小），**不插值**。
//
// 不插值的理由：这份报告是给人看分布、据以决定「区间该定多宽」的。插值带来的精度
// 对那个决定没有任何影响，却多一处能算错的地方 —— 而算错了不会有任何东西报错，
// 只会让报告上多一个看起来完全正常的数。
func TestComputeFieldStatsUsesNearestRank(t *testing.T) {
	in := shuffled1to100()
	// 前提有两条，缺一不可：
	//  ① 输入确实无序 —— 否则「压根不排序、直接取下标」的实现也能通过；
	//  ② 输入确实是 1..100 的一个置换 —— 否则下面那些期望值本身就是错的。
	require.False(t, slices.IsSorted(in), "用例前提：输入必须无序")
	sortedIn := slices.Clone(in)
	slices.Sort(sortedIn)
	want := make([]float64, 100)
	for i := range want {
		want[i] = float64(i + 1)
	}
	require.Equal(t, want, sortedIn, "用例前提：夹具必须恰好是 1..100 的置换")

	s := computeFieldStats(FieldM2, in)

	assert.Equal(t, FieldM2, s.Field)
	assert.Equal(t, 100, s.N)
	assert.Equal(t, 1.0, s.Min)
	assert.Equal(t, 100.0, s.Max)
	// ceil(0.05*100)=5 ⇒ 第 5 小 = 5；ceil(0.5*100)=50；ceil(0.95*100)=95
	assert.Equal(t, 5.0, s.P5, "nearest-rank：ceil(0.05*100)=5 ⇒ 第 5 小")
	assert.Equal(t, 50.0, s.Median)
	assert.Equal(t, 95.0, s.P95)
}

// **不得就地排序调用方的切片。**
//
// 入参是 CalibrateResult.Samples 的切片本身，而 discovery 明写它的顺序是**期次升序**
// —— 本任务后面要算的「相邻期环比变化率」正依赖那个顺序。就地 sort.Float64s 会把它
// 毁掉，而**本函数自己的全部断言照样绿**：受害的是另一个函数，且没有任何东西会报错。
func TestComputeFieldStatsDoesNotMutateInput(t *testing.T) {
	in := shuffled1to100()
	before := slices.Clone(in)

	computeFieldStats(FieldM2, in)

	assert.Equal(t, before, in, "computeFieldStats 必须排序副本，不得改动调用方的切片")
}

// suggestSpanFactorK 是「建议区间宽度 / 实测跨度」的上界。
//
// **3 是加性规则的精确值**，不是拍脑袋的余量：
//
//	SuggestMax - SuggestMin = (max+span) - (min-span) = (max-min) + 2*span = 3*span
//
// 写死在测试里而不是从实现取，是因为它要挡的正是「实现自己说了算」那一类 ——
// 只断 SuggestMin < Min && SuggestMax > Max 的话，
// `SuggestMin, SuggestMax = -math.MaxFloat64, math.MaxFloat64` **两条全绿**，
// 而那是一个「建议任何值都合法」的实现。
const suggestSpanFactorK = 3.0

// 建议区间用**加性**余量 [min-span, max+span]，不是乘性。
//
// 乘性规则在负值字段上会把已观测到的真实值排除在外：deposit_household_ytd 可以为负
// （住户存款净减少），min=-8200 时 min*0.5 = -4100 > -8200 ⇒ **建议区间把实测最小值
// 排除在外，而它看起来完全像一个正常区间。**
//
// ⚠️ 夹具取值必须让 span 与端点都是**精确可表示的整数**（|x| < 2^53）：
// 下面的上界断言是精确 `<=`，换成「看起来更真实」的小数会让它因 1 ULP 而 flaky，
// 而那种红看起来像真缺陷。这条与 TASK-001 边界用例那段 ULP 注释同源。
func TestSuggestionIsAdditiveNotMultiplicative(t *testing.T) {
	// 形态取自计划夹具：递增四档（q1/h1/q1_q3/annual），且最小值为负。
	in := []float64{-8200, 1200, 78000, 146400}
	s := computeFieldStats(FieldDepositHouseholdYTD, in)

	require.True(t, s.HasSuggestion, "n=4 >= 3，应当给建议")
	assert.Equal(t, -8200.0, s.Min)
	assert.Equal(t, 146400.0, s.Max)

	// 🔴 核心：实测最小值必须落在建议区间**内**。乘性规则在这里会算出 -4100 > -8200。
	assert.LessOrEqual(t, s.SuggestMin, s.Min,
		"建议区间不得把已观测到的最小值排除在外——负值字段上乘性余量会犯这个错")
	assert.GreaterOrEqual(t, s.SuggestMax, s.Max)

	// 上界：挡住「建议任何值都合法」的实现。
	span := s.Max - s.Min
	assert.LessOrEqual(t, s.SuggestMax-s.SuggestMin, suggestSpanFactorK*span,
		"建议区间宽度不得超过实测跨度的 %g 倍——±MaxFloat64 这类实现只有本条挡得住",
		suggestSpanFactorK)
	assert.False(t, math.IsInf(s.SuggestMin, 0) || math.IsInf(s.SuggestMax, 0))

	// 具体取值（span=154600 ⇒ [-162800, 301000]），把加性规则本身钉死。
	assert.Equal(t, -162800.0, s.SuggestMin)
	assert.Equal(t, 301000.0, s.SuggestMax)
}

// n < 3 不给建议。
//
// 理由：span 太小或为 0 时，任何余量规则都会退化成一个几乎没有宽度的区间 ——
// **而过窄的建议比没有建议更危险：它看起来是个结论。**
func TestSuggestionWithheldBelowMinSamples(t *testing.T) {
	tests := []struct {
		name    string
		samples []float64
		want    bool
	}{
		{"n=0", nil, false},
		{"n=1", []float64{100}, false},
		// ⚠️ 两个值必须**不同**：用 [100,100] 的话 span=0，于是把判据错写成
		// `if span > 0` 的实现**照样绿** —— minSamplesForSuggestion 这个常量的
		// 存在理由就此落空，而没有任何东西会红。
		{"n=2（两值不同，span>0）", []float64{100, 250}, false},
		{"n=3（边界值，单独一格）", []float64{100, 250, 400}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := computeFieldStats(FieldM2, tt.samples)
			assert.Equal(t, len(tt.samples), s.N)
			assert.Equal(t, tt.want, s.HasSuggestion)
		})
	}
}

// —— functional[1]: 渲染 ——

// reportRow 取出报告里某个字段那一行，切成列。
//
// # 为什么按列取，而不是 Contains(out, "22")
//
// 报告类断言全在同一份文本上做 Contains，一个期望串极易被**与目标无关的行**冒名满足。
// TASK-002 的 dev 实撞过一次：`Contains(out, "monthly")` 本意是「不解析的 period_type
// 要写出来」，而夹具里那篇的**文件名**恰好含 monthly ⇒ 实现即使把它错记成「读文件失败」
// 该断言仍为真（消融只红 1 条而不是 2 条才暴露）。
//
// 数字更糟：n=4 那个 "4" 会出现在 min=400、p95=1400 里。⇒ 按列取，逐列比对。
//
// 同时断言这样的行**恰好 1 条**：字段名之间存在包含关系（tsf_stock ⊂ tsf_stock_yoy），
// 只断「存在」的话，一个只渲染 tsf_stock_yoy 的实现会让 tsf_stock 那条断言照样绿。
func reportRow(t *testing.T, out, field string) []string {
	t.Helper()
	var hits []string
	for _, ln := range strings.Split(out, "\n") {
		if cols := strings.Fields(ln); len(cols) > 0 && cols[0] == field {
			hits = append(hits, ln)
		}
	}
	require.Len(t, hits, 1, "字段 %q 应当恰好占一行（首列精确等于字段名），实际 %d 行", field, len(hits))
	return strings.Fields(hits[0])
}

// renderedFieldOrder 按渲染顺序切出字段名序列（首列精确等于某个已知字段名的行）。
func renderedFieldOrder(out string) []string {
	known := map[string]bool{}
	for _, f := range fieldOrder {
		known[f] = true
	}
	var got []string
	for _, ln := range strings.Split(out, "\n") {
		if cols := strings.Fields(ln); len(cols) > 0 && known[cols[0]] {
			got = append(got, cols[0])
		}
	}
	return got
}

// renderFixture 造一份小而全的 CalibrateResult。
//
// ⚠️ 夹具的数**不照抄生产语料**（真语料是 22 / 4，且那两个数历经两次错数）。
// 这里用 6 与 2 是为了同时覆盖「给建议」与「不给建议」两条路径。
func renderFixture() *CalibrateResult {
	// ⚠️ Samples **由 Records 派生**，不手写第二份。
	// 同一事实的两个副本，改一处不会让另一处变红 —— 而夹具里的不一致会产出一条
	// 「看起来在测某件事、实际数据自相矛盾」的用例。生产侧同理（见 collectSamples）。
	recs := []SampleRecord{
		// 期次升序。M2 跨三种 period_type ⇒ 用来验「每行标注样本来自哪几种」。
		{Period: "2021-12", PeriodType: "annual", Values: map[string]float64{FieldM2: 100, FieldTSFStock: 700}},
		{Period: "2022-06", PeriodType: "h1", Values: map[string]float64{FieldM2: 400}},
		{Period: "2022-12", PeriodType: "annual", Values: map[string]float64{FieldM2: 200, FieldTSFStock: 900}},
		{Period: "2023-03", PeriodType: "q1", Values: map[string]float64{FieldM2: 600}},
		{Period: "2023-06", PeriodType: "h1", Values: map[string]float64{FieldM2: 500}},
		{Period: "2023-12", PeriodType: "annual", Values: map[string]float64{FieldM2: 300}},
	}
	return &CalibrateResult{
		Periods:      9,
		Records:      recs,
		Samples:      samplesFromRecords(recs), // M2 n=6（有建议）、tsf_stock n=2（无建议）
		Failures:     []ParseFailure{{Period: "2019-12", Kind: "finance", File: "articles/a.html", Err: "loan scope anchor not found"}},
		Unsupported:  []ParseFailure{{Period: "2024-06", Kind: "tsf_stock", File: "articles/b.html", Err: "本迭代不解析该报告种类"}},
		Unclassified: []string{"某某省2024年上半年金融统计数据报告"},
		FetchFailed:  []Failed{{ID: "9001", URL: "https://x/9001.html", Error: "timeout"}},
		Warnings:     []string{"manifest 无 reconcile 对账摘要"},
	}
}

func renderToString(t *testing.T, res *CalibrateResult) string {
	t.Helper()
	var b strings.Builder
	require.NoError(t, renderCalibrateReport(&b, res))
	return b.String()
}

// 字段必须按 fieldOrder **全序**渲染。
//
// ⚠️ 断言的是**整个序列**，不是「A 在 B 前面」这种两两比较：遍历 map（迭代序随机）
// 再补零样本字段的实现，54 个里只查 2 个**有相当概率通过** —— 那是 flaky 绿，
// 而 flaky 绿会被归因成「环境问题」重跑掉。
func TestRenderCalibrateReportFollowsFieldOrder(t *testing.T) {
	got := renderedFieldOrder(renderToString(t, renderFixture()))
	assert.Equal(t, fieldOrder, got, "字段必须按 fieldOrder 全序渲染，且一个不少")
}

// 每行必须带 n（该字段的样本数）。
//
// # 为什么 n 列不可省
//
// 非社融字段与社融字段的样本数**差一个数量级**（社融字段在 rule@v1 期次里位于另外
// 两篇报告，本迭代无解析器）。**这个差异不写在脸上，填表的人会用同样的信心对待
// 两种区间** —— 而 n 很小的那些，区间是靠几个点撑起来的。
//
// ⚠️ 断言不能写成 Contains(out, "6")：表头那行 `字段分布（%d 期样本）` 本身就含数字，
// 一个**完全不输出 n 列**的实现会直接命中。⇒ 按列取（reportRow），比第 2 列。
func TestRenderCalibrateReportShowsPerFieldSampleCount(t *testing.T) {
	out := renderToString(t, renderFixture())

	assert.Equal(t, "6", reportRow(t, out, FieldM2)[1], "n 列必须是该字段自己的样本数")
	assert.Equal(t, "2", reportRow(t, out, FieldTSFStock)[1])
	// 零样本字段同样要有 n，且是 0 —— 不能只渲染有样本的字段。
	assert.Equal(t, "0", reportRow(t, out, FieldM1)[1])
}

// countLines 数出含 want 的行数。用来把「恰好出现一次、且在该出现的那一段」变成断言。
func countLines(out, want string) int {
	n := 0
	for _, ln := range strings.Split(out, "\n") {
		if strings.Contains(ln, want) {
			n++
		}
	}
	return n
}

// 四种去向 + 存疑，每一种都必须被渲染出来，且各占各的一段。
//
// # 这些不是「顺手也渲染一下」，每一条都是 calibrate.go 字段注释里写下的**待验断言**
//
//   - FetchFailed：「不带出来的话，报告会显示『失败：无』，而失败表的用途正是
//     『M1c-3 入库前要清零』」
//   - Unsupported：「**不是失败**，混进 Failures 会在真语料上产生 193 条假失败」
//   - Unclassified：「非 0 意味着站点改了期次表述」——原文照录
//   - Warnings：「说的是语料的性质」，真跑必然出现「无 reconcile 对账摘要」那条
//
// **注释里的每一句『必须/否则』，都应该能指出一条会因它变红的测试。** 这就是那条。
func TestRenderCalibrateReportRendersEveryDisposition(t *testing.T) {
	out := renderToString(t, renderFixture())

	tests := []struct {
		name, want string
	}{
		// 各取一个**互不相同**的判别串：用共有词（如「失败」）的话，一条能命中多段。
		{"③ 解析失败：原样带出 Parse 的错误", "loan scope anchor not found"},
		{"② 本迭代不解析：不是失败", "本迭代不解析该报告种类"},
		{"④ 标题解析不出期次：原文照录", "某某省2024年上半年金融统计数据报告"},
		{"fetch 阶段就没抓到的篇目", "9001"},
		{"存疑项", "manifest 无 reconcile 对账摘要"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, 1, countLines(out, tt.want),
				"该判别串应当恰好出现在一行里；0 = 这一类被吞掉了，>1 = 它同时命中了别的段")
		})
	}
}

// 🔴 `Failures` 为空**不等于**没有失败：fetch 阶段没抓到的那些也是。
//
// 这条单独立一格，因为它是 FetchFailed 那段注释点名的失效形态 ——
// 一个只看 Failures 的实现会在这份输入上宣布「解析失败：无」，而**同一份产物里
// 确实有一篇没抓到**。M1c-3 入库前要清零的是两者之和。
func TestRenderCalibrateReportCountsFetchFailuresWhenParseFailuresAreEmpty(t *testing.T) {
	res := renderFixture()
	res.Failures = nil // 只剩 fetch 阶段的失败

	out := renderToString(t, res)

	assert.Equal(t, 1, countLines(out, "9001"), "fetch 阶段没抓到的篇目必须出现")
	assert.Equal(t, 1, countLines(out, "timeout"), "原因要带出来，否则无从判断该不该重抓")
	// 报告不得在这份输入上宣布「一件失败都没有」。
	assert.NotContains(t, out, noFailureClaim,
		"Failures 为空但 FetchFailed 非空时，不得宣布无失败——两者之和才是要清零的东西")
}

// 有样本 / 无样本两种行的形状。
func TestRenderCalibrateReportMarksFieldsWithoutSamples(t *testing.T) {
	out := renderToString(t, renderFixture())

	// ⚠️ 本条**不断言 n 列** —— n 由 TestRenderCalibrateReportShowsPerFieldSampleCount
	// 独占。两处都断的话，「删掉 n 列」这个消融会同时红两条测试，
	// 而 DoD 要的验收是「红且**只红那一条**」：一个消融红几条，本身就是它精确度的度量。
	m1 := reportRow(t, out, FieldM1) // 零样本
	assert.Contains(t, strings.Join(m1, " "), noValueMark,
		"零样本字段的分位数必须打 %q，不得打 0——0 是一个合法的实测值", noValueMark)

	m2 := reportRow(t, out, FieldM2) // n=6，有建议
	assert.NotContains(t, strings.Join(m2, " "), noValueMark)

	// n=2 ⇒ 不给建议，但分位数照常给：**「没有建议」不等于「没有数据」**。
	tsf := reportRow(t, out, FieldTSFStock)
	assert.NotContains(t, strings.Join(tsf[2:len(tsf)-1], " "), noValueMark,
		"n=2 仍有分位数，不该打 —")
	assert.Equal(t, noSuggestionMark, tsf[len(tsf)-1],
		"n<3 时建议列必须显式标注「不给建议」，留空会被读成「区间就是这么宽」")
}

// —— functional[2]: 每行必须标注样本来自哪几种 period_type ——

// 🔴 混池的危害是**具体**的：fieldOrder 里相当比例是 `*_ytd` **累计量**，
// q1 与 annual 的量纲根本不同（前者 3 个月、后者 12 个月）。混在一起算 min/max，
// 跨度会横跨整个范围，再加上余量 ⇒ **得到一个宽到拦不住任何东西的区间**。
//
// 而 MagnitudeRanges 是 map[string]Range（**只有 field 一维**，gateMagnitudeSanity
// 也只按 field 查表）⇒ 工具**不替他解决**，但必须**让他看见**。
func TestRenderCalibrateReportAnnotatesPeriodTypeMix(t *testing.T) {
	out := renderToString(t, renderFixture())

	// M2 的 6 个样本来自 annual×3 / h1×2 / q1×1 —— 三种都要出现，且带各自的条数。
	m2 := reportRow(t, out, FieldM2)
	mix := m2[2]
	for _, want := range []string{"annual×3", "h1×2", "q1×1"} {
		assert.Contains(t, mix, want,
			"每行必须标注样本来自哪几种 period_type 及各自条数；实际列内容=%q", mix)
	}

	// 单一来源的字段只标那一种 —— 否则「混了」与「没混」在报告上不可分。
	tsf := reportRow(t, out, FieldTSFStock)
	assert.Equal(t, "annual×2", tsf[2], "只来自一种 period_type 时不得虚报其它种类")
}

// —— functional[3]: tsf_stock 逐 period_type 的相邻期环比变化率 ——

// rateFixture 造一份专供环比用的记录集。**故意留洞**，见下面那条测试。
func rateFixture() []SampleRecord {
	return []SampleRecord{
		// annual：2019 → 2021，**中间缺 2020**（真语料里 2024 就没有一季度，
		// 且 3 篇 Parse 失败另挖了三个洞）
		{Period: "2019-12", PeriodType: "annual", Values: map[string]float64{FieldTSFStock: 200}},
		{Period: "2021-12", PeriodType: "annual", Values: map[string]float64{FieldTSFStock: 220}},
		{Period: "2022-12", PeriodType: "annual", Values: map[string]float64{FieldTSFStock: 231}},
		// h1：两期 ⇒ 1 个环比
		{Period: "2022-06", PeriodType: "h1", Values: map[string]float64{FieldTSFStock: 400}},
		{Period: "2023-06", PeriodType: "h1", Values: map[string]float64{FieldTSFStock: 300}}, // 下跌也算跳变
		// 只有一期 ⇒ 0 个环比（不是 1 个 0）
		{Period: "2023-03", PeriodType: "q1", Values: map[string]float64{FieldTSFStock: 500}},
	}
}

// 环比必须**在同一 period_type 内**算，绝不跨类型。
//
// 跨类型算是没有意义的：Preceding 按 period_type 隔离序列，annual 的「上一期」是
// **去年的 annual**。把 2022-06/h1 当成 2021-12/annual 的下一期，算出的是两条
// 不同序列之间的差，而它看起来和一个真实的环比一模一样。
func TestStockContinuityRatesGroupedByPeriodType(t *testing.T) {
	got := stockContinuityRates(rateFixture())

	// annual: 200→220 (0.10)、220→231 (0.05)；h1: 400→300 (0.25)；q1: 单期无环比
	require.Contains(t, got, "annual")
	require.Contains(t, got, "h1")
	assert.Equal(t, 2, got["annual"].N, "annual 三期 ⇒ 两个相邻对")
	assert.Equal(t, 1, got["h1"].N)
	assert.NotContains(t, got, "q1", "只有一期时没有相邻对，不得凭空产生一个 0")

	assert.InDelta(t, 0.05, got["annual"].Min, 1e-12)
	assert.InDelta(t, 0.10, got["annual"].Max, 1e-12)
	// 下跌同样是跳变：取绝对值，否则整个下跌方向都漏掉。
	assert.InDelta(t, 0.25, got["h1"].Min, 1e-12)
}

// 🔴 「相邻期」= **排序后相邻的两个样本**，不是「相差一个季度/一年」。
//
// 实测序列**有洞**：2024 年只有年报/上半年/前三季度，**没有一季度**；另有 3 篇
// Parse 失败（2019-12 / 2020-09 / 2022-09）各挖一个洞。
// 若实现按「期次必须相差固定间隔」配对，**跨洞的那一对会被整个丢掉** ——
// 而丢掉不会有任何东西报错，只会让 n 悄悄变小、分布悄悄变窄。
func TestStockContinuityRatesPairAdjacentSamplesAcrossGaps(t *testing.T) {
	got := stockContinuityRates(rateFixture())

	// 夹具里 annual 是 2019 → 2021 → 2022（**2020 缺席**）。
	// 按「相邻样本」配对 ⇒ 2 对；按「相差一年」配对 ⇒ 只剩 2021→2022 这 1 对。
	require.Equal(t, 2, got["annual"].N,
		"跨洞的 2019→2021 也是一对相邻样本；按固定间隔配对会把它丢掉，而丢掉是静默的")
	assert.InDelta(t, 0.10, got["annual"].Max, 1e-12, "2019→2021 那对（20/200）必须在里面")
}

// 上一期为 0 时跳过该对，不产生 Inf。
//
// 与 gateStockContinuity 的 zero_denominator 分支同源：Inf 会污染整段分位数，
// 而报告上的 +Inf 会被读成「这个字段疯了」，实际只是分母恰好为 0。
func TestStockContinuityRatesSkipZeroDenominator(t *testing.T) {
	recs := []SampleRecord{
		{Period: "2021-12", PeriodType: "annual", Values: map[string]float64{FieldTSFStock: 0}},
		{Period: "2022-12", PeriodType: "annual", Values: map[string]float64{FieldTSFStock: 220}},
		{Period: "2023-12", PeriodType: "annual", Values: map[string]float64{FieldTSFStock: 231}},
	}
	got := stockContinuityRates(recs)

	assert.Equal(t, 1, got["annual"].N, "分母为 0 的那一对必须跳过，只剩 220→231")
	assert.False(t, math.IsInf(got["annual"].Max, 0), "不得产生 Inf")
	assert.InDelta(t, 0.05, got["annual"].Max, 1e-12)
}

// 这一节必须真的出现在报告里，且**沿用 n<3 的规则，不为它破例**。
//
// 预期样本很少（Parse 拒绝 monthly ⇒ monthly 档零样本；年度间隔档约个位数），
// 正因为少，才更不能给一个靠两三个点撑起来的「建议」。
func TestRenderCalibrateReportIncludesStockContinuitySection(t *testing.T) {
	res := renderFixture()
	res.Records = rateFixture()
	res.Samples = samplesFromRecords(res.Records)

	out := renderToString(t, res)

	require.Contains(t, out, stockRateSectionTitle, "环比变化率一节必须出现在报告里")
	// 该节按 period_type 一行一档，行首是 period_type。
	annual := reportRow(t, out, "annual")

	// 🔴 中间五列此前**无人守**（TASK-003 验证者三组消融全部 SURVIVED、完全静默）：
	// 原来这里只读 annual[1]（n）、最后一列（建议区间）、和「有没有 —」
	// ⇒ 环比一节丢掉 p5 列、或把 p5·median·p95·max 全打成 min，**无一变红**。
	// ⚠️ TASK-003 的 DoD **字面列举了** n/min/p5/median/p95/max 六个 —— 六个里四个
	// 可以被替换成 min 而没有任何东西出声。**「列在 DoD 里」不蕴含「有断言守着」。**
	//
	// 修法（比整行列数 + 两端取值）：列数挡住「少一列」，min/max 取值挡住「全打成同一个数」。
	require.Len(t, annual, 8,
		"环比表一行 8 列：period_type n min p5 median p95 max 建议区间；"+
			"少一列说明有列被删掉了，而删列此前完全静默")
	assert.Equal(t, "2", annual[1], "annual 两个相邻对")
	// annual 的两个环比是 0.05 与 0.1（200→220→231）⇒ min≠max，
	// 「把中间几列全打成 min」的实现会让 max 这一列变成 0.05 而红。
	assert.Equal(t, "0.05", annual[2], "min 列")
	assert.Equal(t, "0.1", annual[6], "max 列——与 min 不同，故「全打成 min」会红")
	assert.Equal(t, noSuggestionMark, annual[len(annual)-1],
		"n<3 的规则对这一节同样适用，不得为「样本本来就少」破例")
}
