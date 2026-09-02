package hestia

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Context Checkpoint: done_criteria → test mapping (M1c-3b 的 TASK-006)
// error_handling[0] 恒等式不成立 ⇒ writeLoadReport 返回 error，串含「恒等式」
//                                        → TestWriteLoadReportRejectsBrokenIdentity
// error_handling[1] IncompleteAccepted ⇒ 报告**顶部**警告，含 completed_at 与「缺期」
//                                        → TestLoadReportWarnsOnIncomplete
// error_handling[2] DroppedIDs 逐条列出   → TestLoadReportListsDroppedArticleIDs
// functional[4]     分节顺序固定          → TestLoadReportSectionsAreInFixedOrder
// functional[6]     PartialCoverage 出声   → TestLoadReportListsPartialCoverage

// okResult 是一份四道恒等式都成立的最小结果，供各用例改写单项。
func okResult() *BackfillLoadResult {
	return &BackfillLoadResult{
		Total: 3, Attempted: 3, Unsupported: 0,
		ParsedOK: 3, ParseFailed: 0,
		Merged: 1, SingleArticle: 0, MergedGroups: 1,
		ToObservations: 1, ToPending: 0,
		// 恒等式三现在是**异源**比对（M1c-3b 的 TASK-006，C-4）：一边是分组计数，
		// 一边是库里数出来的 merged@v1 行数。夹具要同时给两边，否则那道闸被
		// MergedRowsCounted==false 跳过，而**跳过时它不会红** —— 那正是它此前的毛病。
		DBMergedRows: 1, MergedRowsCounted: true,
	}
}

// TestWriteLoadReportRejectsBrokenIdentity：恒等式不成立 ⇒ 返回 error（error_handling[0]）。
//
// ⚠️ **本条只断言「返回 error」，不再断言「不输出」**（M1c-3b 的 TASK-006，C-2 改）。
// 此处原注释写的是「报告不能在数字对不上时照样打印一份好看的表格」—— 那句话描述的是
// 被本轮推翻的行为，逐字留着会让下一个人把 C-2 的修复当成回归给改回去。
//
// 🔴 **现在的契约是「照样打印 + 返回 error」**：账对不上时人更需要看见那份报告
// （恒等式一失败的头号成因是 Unclassified 非空，而那批标题原文只在报告里）。
// 「不给看」并不能阻止错误发生，只是让排查的人少一份线索。
// 打印出来的那份表格**带着 error 一起交出**，不会被误当成验收通过。
// 端到端证据见 TestBackfillLoadFailsLoudlyOnUnclassified（走真实路径，非手工凑数）。
func TestWriteLoadReportRejectsBrokenIdentity(t *testing.T) {
	// 四道恒等式各坏一次——只测一道的话，另外三道可以完全没实现而测试全绿。
	for _, tt := range []struct {
		name   string
		break_ func(r *BackfillLoadResult)
	}{
		{"一：Total ≠ Attempted+Unsupported", func(r *BackfillLoadResult) { r.Total = 99 }},
		{"二：Attempted ≠ ParsedOK+ParseFailed", func(r *BackfillLoadResult) { r.ParsedOK = 99 }},
		// 三号现在是异源比对：坏 MergedGroups 而不动 DBMergedRows，两边就对不上。
		// ⚠️ 旧版坏它是**坏不掉**的（Merged/SingleArticle/MergedGroups 三者自洽求和，
		// 单改一个照样满足 `Merged == SingleArticle + MergedGroups`？不 —— 旧版单改
		// MergedGroups 确实会红，但那只证明「和对不上」；真正杀不掉的是**生产代码里
		// 那两个计数器一起写错**的情形，见 TestLoadIdentityThreeIsCrossSourced）。
		{"三：MergedGroups ≠ 库里 merged@v1 行数", func(r *BackfillLoadResult) { r.MergedGroups = 99 }},
		{"四：Merged ≠ ToObservations+ToPending", func(r *BackfillLoadResult) { r.ToPending = 99 }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			res := okResult()
			tt.break_(res)
			var b bytes.Buffer
			err := writeLoadReport(&b, "/x", res)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "恒等式")
		})
	}

	t.Run("四道都成立 ⇒ 不报错", func(t *testing.T) {
		var b bytes.Buffer
		require.NoError(t, writeLoadReport(&b, "/x", okResult()))
		assert.Contains(t, b.String(), "恒等式", "成立时也要把这句结论打出来，否则读者不知道查过")
	})
}

// TestLoadReportWarnsOnIncomplete：警告必须在**顶部**（error_handling[1]）。
//
// 它在 load 里的代价比在 calibrate 里重：calibrate 传它的代价是「区间可能偏窄」，
// load 传它的代价是**历史序列直接缺期**，且缺的那些期在库里与「央行本来就没发」
// 完全同形。
func TestLoadReportWarnsOnIncomplete(t *testing.T) {
	res := okResult()
	res.IncompleteAccepted = true
	var b bytes.Buffer
	require.NoError(t, writeLoadReport(&b, "/x", res))

	out := b.String()
	assert.Contains(t, out, "completed_at")
	assert.Contains(t, out, "缺期")

	// 「顶部」是可证伪的：警告的位置必须早于统计数字那一节，否则读者读完才知道前提。
	assert.Less(t, strings.Index(out, "completed_at"), strings.Index(out, "语料总篇数"),
		"放行说明必须排在统计数字之前——它是读下面每一个数的前提")

	t.Run("未放行时不得出现该警告", func(t *testing.T) {
		var b2 bytes.Buffer
		require.NoError(t, writeLoadReport(&b2, "/x", okResult()))
		assert.NotContains(t, b2.String(), "completed_at",
			"没放行却打警告 = 狼来了，会让真警告失去意义")
	})
}

// TestLoadReportListsDroppedArticleIDs：逐条列出，不是只报个数（error_handling[2]）。
func TestLoadReportListsDroppedArticleIDs(t *testing.T) {
	res := okResult()
	res.Groups = []MergedObservation{{
		Obs:        Observation{Meta: Meta{Period: "2020-01", PeriodType: "monthly", ArticleID: "id-a"}},
		Parts:      []string{extractorMonthlyV1, extractorTSFStock, extractorTSFFlow},
		SourceIDs:  []string{"id-a", "id-b", "id-c"},
		DroppedIDs: []string{"id-b", "id-c"},
	}}
	var b bytes.Buffer
	require.NoError(t, writeLoadReport(&b, "/x", res))

	out := b.String()
	assert.Contains(t, out, "id-b")
	assert.Contains(t, out, "id-c", "两条都要列——只列第一条等于报个数")
	assert.Contains(t, out, "id-a", "代表 id 也要出现，否则拼不回 URL")
}

// TestLoadReportListsPartialCoverage：把「部分覆盖的期次」暴露出来（functional[6]）。
//
// 人类 2026-09-01 在 dod-gate 裁决：合并键维持不变，但必须让那 44% 出声 ——
// 它们入了权威表、completeness 也过（tsf-stock@v1 的必填集就是那 18 个），
// 在库里与「央行本来就没发」**完全同形**。
func TestLoadReportListsPartialCoverage(t *testing.T) {
	res := okResult()
	partial := MergedObservation{
		Obs:       Observation{Meta: Meta{Period: "2021-03", PeriodType: "monthly", ArticleID: "only-stock"}},
		Parts:     []string{extractorTSFStock}, // 缺月报族与增量
		SourceIDs: []string{"only-stock"},
	}
	res.Groups = []MergedObservation{partial}
	res.PartialCoverage = []MergedObservation{partial}

	var b bytes.Buffer
	require.NoError(t, writeLoadReport(&b, "/x", res))

	out := b.String()
	assert.Contains(t, out, "部分覆盖")
	assert.Contains(t, out, "2021-03")
	// **缺哪一族**必须说出来，只说「不完整」等于让人再查一遍
	assert.Contains(t, out, "月报", "要指名缺的是哪一族")
	assert.Contains(t, out, "增量")

	t.Run("三族齐全时该节说无", func(t *testing.T) {
		full := okResult()
		full.Groups = []MergedObservation{{
			Obs:   Observation{Meta: Meta{Period: "2020-01", PeriodType: "monthly"}},
			Parts: []string{extractorMonthlyV1, extractorTSFStock, extractorTSFFlow},
		}}
		var b2 bytes.Buffer
		require.NoError(t, writeLoadReport(&b2, "/x", full))

		// ⚠️ 不能对整份输出断 NotContains("2020-01")：该期次照样会出现在「合并组明细」
		// 里，那是对的。判据只能落在**部分覆盖那一节的区间内**。
		out := b2.String()
		from := strings.Index(out, "部分覆盖的期次")
		require.GreaterOrEqual(t, from, 0)
		to := strings.Index(out[from:], "字段冲突")
		require.Greater(t, to, 0)
		section := out[from : from+to]
		assert.Contains(t, section, "（无）", "三族齐全时该节应说无")
		assert.NotContains(t, section, "2020-01", "三族齐全的期次不该出现在这一节")
	})
}

// TestLoadReportSectionsAreInFixedOrder：分节顺序固定（functional[4]）。
//
// 稳定才能逐次 diff —— 顺序会飘的报告，每次跑出来的 diff 都是噪声。
func TestLoadReportSectionsAreInFixedOrder(t *testing.T) {
	res := okResult()
	res.Groups = []MergedObservation{{
		Obs:        Observation{Meta: Meta{Period: "2020-01", PeriodType: "monthly", ArticleID: "id-a"}},
		Parts:      []string{extractorMonthlyV1, extractorTSFStock, extractorTSFFlow},
		SourceIDs:  []string{"id-a", "id-b"},
		DroppedIDs: []string{"id-b"},
	}}
	res.PendingReasons = map[string]string{"2020-01/monthly": "completeness failed"}

	var b bytes.Buffer
	require.NoError(t, writeLoadReport(&b, "/the/dir", res))
	out := b.String()

	assert.Contains(t, out, "/the/dir", "标定输入目录要打出来")
	// last 从 -1 起：首节就在索引 0，从 0 起会把它自己判成越界。
	// ⚠️ 锚要用**小节标题**而不是关键词：「落 pending」在上面的统计块里先出现过一次，
	// 拿它当锚会命中那一处，得到一个与分节顺序无关的位置。
	last := -1
	for _, sec := range []string{
		"标定输入", "语料总篇数", "四道恒等式", "合并组明细", "部分覆盖的期次", "字段冲突",
		// M1c-4 的 TASK-011：口径路由与族内量级两节，排在字段冲突之后、落 pending 之前
		"口径路由核对", "族内量级核对", "落 pending 的期次",
	} {
		i := strings.Index(out, sec)
		require.GreaterOrEqualf(t, i, 0, "报告里缺了「%s」这一节", sec)
		assert.Greaterf(t, i, last, "「%s」的位置不对——分节顺序必须固定", sec)
		last = i
	}
}

// TestLoadReportSurfacesUnclassified 钉住一个**设计决策**：标题解析不出期次的篇目
// 既不在 Attempted 也不在 Unsupported，它们无处可去 ⇒ **恒等式一必然不成立**。
//
// 这不是缺陷，是想要的行为：四道恒等式的用途就是把「账对不上」变成响亮失败。
// 而「站点改了期次表述」正是最需要被人看见的一类变化 —— 让它静默通过，那批篇目
// 就从所有表上彻底消失了。错误串必须**点名** Unclassified 是成因，否则人拿到
// 「Total ≠ Attempted + Unsupported」只会去数另外两个数。
func TestLoadReportSurfacesUnclassified(t *testing.T) {
	res := okResult()
	res.Total = 4 // 3 篇进了 Attempted，第 4 篇标题解析不出期次
	res.Unclassified = []string{"央行有关负责人就某事答记者问"}

	var b bytes.Buffer
	err := writeLoadReport(&b, "/x", res)
	require.Error(t, err, "有篇目无处可去时，账就是对不上的")
	assert.Contains(t, err.Error(), "恒等式")
	assert.Contains(t, err.Error(), "标题解析不出期次",
		"必须点名成因，否则人会去数另外两个数")

	// 恒等式补齐后，那批篇目仍要在报告里逐条列出——它们是「站点变了」的唯一线索。
	res.Unsupported = 1
	var b2 bytes.Buffer
	require.NoError(t, writeLoadReport(&b2, "/x", res))
	assert.Contains(t, b2.String(), "央行有关负责人就某事答记者问")
}

// TestLoadReportListsFieldConflicts：冲突非空 ⇒ 逐条列出（含两边的来源 id）。
//
// 冲突非空即表示**字段归属表有错**，不是数据问题：三个 extractor 的字段集设计上
// 不相交（25/18/9），冲突理应恒为 0。所以报的是「哪张表错了」的线索。
func TestLoadReportListsFieldConflicts(t *testing.T) {
	res := okResult()
	res.Conflicts = []MergeConflict{{
		Period: "2020-01", PeriodType: "monthly", Field: FieldTSFStock,
		A: 1.5, B: 2.5, FromA: "id-a", FromB: "id-b",
	}}
	var b bytes.Buffer
	require.NoError(t, writeLoadReport(&b, "/x", res))

	out := b.String()
	assert.Contains(t, out, "字段冲突（预期 0，共 1）")
	assert.Contains(t, out, FieldTSFStock)
	assert.Contains(t, out, "id-a")
	assert.Contains(t, out, "id-b", "两边的来源都要有——只报一边就查不出是哪两篇打架")
}

// TestLoadReportSurfacesAllSignals：W-6 / W-7 / W-8 三条新增的「出声」路径都要真的印出来
// （M1c-3b 的 TASK-006 返工）。
//
// 🔴 三条都是**只在异常时才走**的分支。加了却不测，等于把「会出声」写成了声称 ——
// 而它们存在的全部理由就是异常发生那一次能被看见。
func TestLoadReportSurfacesAllSignals(t *testing.T) {
	res := okResult()
	res.SHAUnverified = 3
	res.FetchFailed = []Failed{{ID: "9001"}, {ID: "9002"}}
	res.PartOverlaps = []string{FieldTSFStock, FieldTSFStockYoY}
	res.PartialCoverage = []MergedObservation{{
		Obs:   Observation{Meta: Meta{Period: "2025-08", PeriodType: "monthly", PublishedAt: "2025-09-12"}},
		Parts: []string{extractorTSFStock},
	}}
	// 故意给 8 个缺失字段：超过 6 个时报告要截断并打「…等」，那条分支也要走到。
	res.MissingFields = map[string][]string{
		groupKey(res.PartialCoverage[0]): fieldOrder[:8],
	}

	var b bytes.Buffer
	require.NoError(t, writeLoadReport(&b, "/x", res))
	out := b.String()

	assert.Contains(t, out, "3 篇的 manifest 没有 sha256", "W-7：未做完整性校验的篇数要出声")
	assert.Contains(t, out, "fetch 阶段未抓到（2", "W-8：manifest.failed 要入账")
	assert.Contains(t, out, "不计入上面四道恒等式",
		"W-8：必须写明它不参与恒等式——混进去会造出假失败")
	assert.Contains(t, out, "9001", "fetch 失败的 id 要逐条列出，只给个数没法查")
	assert.Contains(t, out, "前提已破", "W-6：必填集相交要出声")
	assert.Contains(t, out, FieldTSFStock)
	assert.Contains(t, out, "缺  8 个字段", "W-1：报缺了多少个字段")
	assert.Contains(t, out, "…等", "超过 6 个时截断，否则这一节会被几十个字段名淹掉")
	assert.Contains(t, out, "（缺族: ", "族作为补充说明仍要打印")
}

// TestWriteLoadReportPropagatesWriteError：写不出去时要原样报出，**不要**被恒等式错误盖掉
// （M1c-3b 的 TASK-006，C-2 的边界）。
//
// C-2 把顺序改成「先写再校验」之后，w 不可用是一条新的失败路径：此时若继续查恒等式，
// 返回的会是一个恒等式错误，而真正的成因（写不出去）就消失了。
func TestWriteLoadReportPropagatesWriteError(t *testing.T) {
	res := okResult()
	res.Total = 99 // 顺便让恒等式也不成立：证明返回的是写错误而不是恒等式错误

	err := writeLoadReport(errWriter{}, "/x", res)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "写坏了")
	assert.NotContains(t, err.Error(), "恒等式",
		"写不出去时不得再查恒等式——那会把真正的成因盖掉")
}

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) { return 0, errors.New("写坏了") }

// —— 口径路由核对（M1c-4 的 TASK-011）——
//
// Context Checkpoint: done_criteria → test mapping (M1c-4 的 TASK-011)
// functional[0]  checkCaliberRouting 抓「当月值写进累计列」 → TestCaliberRoutingCatchesMonthlyValueInYTDColumn
//                正确路由不报                                → TestCaliberRoutingAcceptsCorrectRouting
// functional[1]  报告印四个数 + 异号跳过字样                → TestCaliberRoutingSkipCountIsNotSilent
//                单侧计数并出声                              → TestCaliberRoutingCountsSingleSided
//                四类计数的自洽校验                          → TestCaliberRoutingCountsArePartition
//                逐对清单与每对观测数                        → TestLoadReportListsComparedPairs
// functional[2]  跨语料族内中位数不等式                      → TestCaliberFamilyMediansCatchWholeFamilyShift
//                样本不足要出声、不静默跳过                  → TestCaliberFamilyMediansReportsInsufficient
// boundary[0]    异号不判、计数为 1                          → TestCaliberRoutingSkipsOppositeSigns
// boundary[1]    ytd == mom 合法（1 月形态）                 → TestCaliberRoutingAcceptsEqualValues

// mkMerged 造一条最小 MergedObservation。
//
// ⚠️ 需求文档在多处引用 mkMerged / loadReportWithRouting，但**包里都不存在**
// （grep 实测）。loadReportWithRouting 无法照抄——renderLoadReport 的真实签名是
// 三参数写 io.Writer、返回 error，不是文档写的单参数返回 string。
func mkMerged(period, periodType string, vals map[string]float64) MergedObservation {
	return MergedObservation{
		Obs: Observation{
			Meta:   Meta{Period: period, PeriodType: periodType, ArticleID: period + "-" + periodType},
			Values: vals,
		},
		SourceIDs: []string{period},
	}
}

// TestCaliberRoutingCatchesMonthlyValueInYTDColumn：当月值写进累计列必须被抓到。
//
// 判据是精确恒等式 ytd_n == ytd_{n-1} + mom_n。两列写反时，累计列装的是当月量级的
// 小数，恒等式差出一个数量级 ⇒ 必红。
//
// 🔴 这是本迭代头号风险的可执行形式：TASK-005 把「拒绝整篇」改成「按口径路由」之后，
// 路由错的产物**量级完全合理**，下游没有任何闸门拦得住 —— 这条断言就是那个闸门。
func TestCaliberRoutingCatchesMonthlyValueInYTDColumn(t *testing.T) {
	obs := []MergedObservation{
		mkMerged("2023-06", "monthly", map[string]float64{FieldLoanFlowYTD: 157341}),
		mkMerged("2023-07", "monthly", map[string]float64{
			FieldLoanFlowYTD: 3459,   // ← 错：这是当月值
			FieldLoanFlowMoM: 160800, // ← 错：这是累计值
		}),
	}

	v, st := checkCaliberRouting(obs)

	require.Len(t, v, 1, "ytd 与 mom 写反了，累计恒等式必然不成立")
	require.Equal(t, FieldLoanFlowYTD, v[0].YTDField)
	require.Equal(t, "2023-07", v[0].Period)
	require.InDelta(t, 318141, v[0].Expected, 0.001, "期望值 = 上期 157341 + 本月 160800")
	require.Equal(t, 1, st.Comparable, "6 月那条无上期，只有 7 月这条算得了")
}

// TestCaliberRoutingAcceptsCorrectRouting：正确路由不报。
func TestCaliberRoutingAcceptsCorrectRouting(t *testing.T) {
	obs := []MergedObservation{
		mkMerged("2023-06", "monthly", map[string]float64{FieldLoanFlowYTD: 157341}),
		mkMerged("2023-07", "monthly", map[string]float64{
			FieldLoanFlowYTD: 160800, // 157341 + 3459
			FieldLoanFlowMoM: 3459,
		}),
	}

	v, st := checkCaliberRouting(obs)

	require.Empty(t, v)
	require.Equal(t, 1, st.Comparable)
}

// TestCaliberRoutingAcceptsSignCrossingCumulative 是**真语料假阳的回归钉**。
//
// 🔴 这一条记录的是 DoD 原判据 |ytd| >= |mom| 被证否的那个实例。真语料 2020-02：
//
//	信托:       432 + (-540)  = -108  vs 报告值 -109（取整差 1）
//	未贴现票据: 1403 + (-3961) = -2558 vs 报告值 -2558（**逐位相等**）
//
// 两条在原判据下都被报成「违反」，而它们**都是合法数据**：社融分项年内正负交替，
// 累计穿越零点后 |累计| 合法地小于 |某月|。⚠️ 异号跳过**挡不住**它 —— 这两例 ytd 与
// mom 同号（都是负），是被跳过条件放行之后才判的。
//
// 恒等式对它们逐位成立（信托差 1 落在 ±1 取整容差内）。本测试用真值，改回不等式判据
// 会让它立刻转红。
func TestCaliberRoutingAcceptsSignCrossingCumulative(t *testing.T) {
	obs := []MergedObservation{
		mkMerged("2020-01", "monthly", map[string]float64{
			FieldTSFFlowTrustYTD: 432, FieldTSFFlowTrustMoM: 432,
			FieldTSFFlowBankAcceptYTD: 1403, FieldTSFFlowBankAcceptMoM: 1403,
		}),
		mkMerged("2020-02", "monthly", map[string]float64{
			FieldTSFFlowTrustYTD: -109, FieldTSFFlowTrustMoM: -540,
			FieldTSFFlowBankAcceptYTD: -2558, FieldTSFFlowBankAcceptMoM: -3961,
		}),
	}

	v, st := checkCaliberRouting(obs)

	require.Empty(t, v, "累计穿越零点是合法形态，不得报违反")
	require.Equal(t, 4, st.Comparable, "1 月两对退化式 + 2 月两对恒等式，四对都判过")
}

// TestCaliberRoutingChecksOppositeSignPairs：ytd 与 mom 异号时**照样精确验证**。
//
// ⚠️ DoD 原本要求「异号跳过并计数」，那是**不等式判据**的需要：|累计| 与 |某月| 在
// 异号时比大小没有语义。换成恒等式之后符号不再相关，异号的对能被精确验证 ——
// 保留跳过只会让本可以判的对静默漏判，那正是本任务要消除的失明。
//
// 真语料 2023-07：存款当月 −1.12 万亿、累计 +18.98 万亿，正是这个形态。
func TestCaliberRoutingChecksOppositeSignPairs(t *testing.T) {
	obs := []MergedObservation{
		mkMerged("2023-06", "monthly", map[string]float64{FieldDepositFlowYTD: 201000}),
		mkMerged("2023-07", "monthly", map[string]float64{
			FieldDepositFlowYTD: 189800, // 201000 + (-11200)
			FieldDepositFlowMoM: -11200,
		}),
	}

	v, st := checkCaliberRouting(obs)

	require.Empty(t, v, "异号但恒等式成立，不是违反")
	require.Equal(t, 1, st.Comparable, "异号的对现在**能判**，不再被跳过")
}

// TestCaliberRoutingAcceptsEqualValues：1 月的 ytd == mom 是**合法**的。
//
// 🔴 为什么合法：1 月报的「年初至今累计」**就等于**当月——年初到 1 月底只包含
// 1 月这一个月。恒等式在 1 月退化成 ytd_1 == mom_1，不需要上一期。
//
// ⚠️ 本测试存在的理由：下一个人很可能觉得「累计至少要比当月大吧」而给 1 月也去找
// 上一期（找不到 ⇒ 全部 1 月落进「无上期」而**永不被验证**），或把退化式写成
// 不等式。两种改法都不会看到红，除非有这一条。
func TestCaliberRoutingAcceptsEqualValues(t *testing.T) {
	obs := []MergedObservation{mkMerged("2024-01", "monthly", map[string]float64{
		FieldLoanFlowYTD: 49200, // 1 月：年初至今 == 当月
		FieldLoanFlowMoM: 49200,
	})}

	v, st := checkCaliberRouting(obs)

	require.Empty(t, v, "1 月报的累计等于当月，是合法形态")
	require.Equal(t, 1, st.Comparable, "1 月退化式**照样算过**，不是「无上期」")
	require.Zero(t, st.NoPrior, "1 月不需要上一期")
}

// TestCaliberRoutingCountsNoPrior：取不到上一期时必须**计数**，不得静默跳过。
//
// 🔴 「没查」与「查过没问题」在报告上必须可区分 —— 这正是本任务要修的那类毛病。
func TestCaliberRoutingCountsNoPrior(t *testing.T) {
	obs := []MergedObservation{mkMerged("2023-07", "monthly", map[string]float64{
		FieldLoanFlowYTD: 160800, FieldLoanFlowMoM: 3459, // 批内没有 2023-06
	})}

	v, st := checkCaliberRouting(obs)

	require.Empty(t, v)
	require.Zero(t, st.Comparable, "取不到上期就是没查过")
	require.Equal(t, 1, st.NoPrior, "无上期必须计数并出声")
}

// TestCaliberRoutingCountsSingleSided：只有一侧有值时**必须计数**，不得静默跳过。
//
// 🔴 两位独立反审的共同结论：路由错误最典型的产物**恰恰是单侧**——一整族被误判成
// 同一个口径就会整族写进 *_ytd（或 *_mom），另一侧一个都没有 ⇒ 每一对都单侧
// ⇒ 若单侧不计数，报告输出「共 0 对，违反 0」，**与「一切正常」逐字相同**。
//
// ⚠️ 真语料 2020-04 全文只有当月句 ⇒ 只有 _mom 列。单侧是**常态**，不是异常。
func TestCaliberRoutingCountsSingleSided(t *testing.T) {
	obs := []MergedObservation{mkMerged("2020-04", "monthly", map[string]float64{
		FieldLoanFlowMoM:    3459,
		FieldDepositFlowMoM: -1120,
	})}

	v, st := checkCaliberRouting(obs)

	require.Empty(t, v)
	require.Zero(t, st.Comparable)
	require.Equal(t, 2, st.SingleSided, "两对各只有一侧在场，必须计入单侧")
}

// TestCaliberRoutingCountsArePartition 是这四个数字的**自洽校验**。
//
// 每一个 (观测, 成对列) 组合必然恰好落进四类之一：可判 / 单侧 / 无上期 / 两侧皆空。
// ⇒ Comparable + SingleSided + NoPrior + Absent == len(caliberFamilies()) × len(obs)。
//
// 🔴 少了这一条，「共 N 对」这个数字**自己无法自证**：N 变小既可能是语料形态变了，
// 也可能是判定逻辑漏掉了一整类，而两者在报告上无法区分。
//
// ⚠️ 判据必须含 Absent。DoD 原文写的是「N + S + M 等于成对列理论总数 × 观测数」，
// **实测 3 vs 44** —— 它漏了「两侧皆空」，而绝大多数 (观测, 对) 组合正属于该类。
// 订正已写进 done_criteria。
func TestCaliberRoutingCountsArePartition(t *testing.T) {
	obs := []MergedObservation{
		mkMerged("2023-06", "monthly", map[string]float64{FieldLoanFlowYTD: 157341}),
		mkMerged("2023-07", "monthly", map[string]float64{
			FieldLoanFlowYTD: 160800, FieldLoanFlowMoM: 3459, // 可判
			FieldDepositFlowYTD: 189800, FieldDepositFlowMoM: -11200, // 无上期
		}),
		mkMerged("2020-04", "monthly", map[string]float64{
			FieldLoanFlowMoM: 3459, // 单侧
		}),
	}

	_, st := checkCaliberRouting(obs)

	total := len(caliberFamilies()) * len(obs)
	require.Equal(t, total, st.Comparable+st.SingleSided+st.NoPrior+st.Absent,
		"四类计数必须构成一个划分，否则「共 N 对」自己无法自证")
	require.NotZero(t, st.Absent, "两侧皆空是常态，DoD 原文漏掉的正是这一类")
}

// TestCaliberFamilyMediansCatchWholeFamilyShift：整族被写错列时，跨语料中位数能抓到。
//
// 🔴 本条守的是逐观测判据的**盲区**：这里每一条观测都只有一侧有值（整族位移的典型
// 形态），checkCaliberRouting 一对都判不了、只会计成单侧。而按 period_type 分档比
// 中位数不要求两列共存 —— 它是唯一能抓住这种形态的自动判据。
func TestCaliberFamilyMediansCatchWholeFamilyShift(t *testing.T) {
	// 累计列装的是当月量级的小数，当月列装的是累计量级的大数 ⇒ 整族写反了
	var obs []MergedObservation
	for i, ytd := range []float64{3000, 3200, 3400, 3600, 3800} {
		obs = append(obs, mkMerged(fmt.Sprintf("2023-%02d", i+1), "monthly",
			map[string]float64{FieldLoanFlowYTD: ytd}))
	}
	for i, mom := range []float64{150000, 160000, 170000, 180000, 190000} {
		obs = append(obs, mkMerged(fmt.Sprintf("2024-%02d", i+1), "monthly",
			map[string]float64{FieldLoanFlowMoM: mom}))
	}

	// 先证明盲区真的存在：逐观测判据在这批数据上一条都判不了
	v, st := checkCaliberRouting(obs)
	require.Empty(t, v, "前提：逐观测判据对单侧数据判不出任何东西")
	require.Zero(t, st.Comparable, "前提：一对都没得比")

	viol, insuf := checkCaliberFamilyMedians(obs, 3)

	require.Empty(t, insuf, "两侧各 5 个样本，够判")
	require.Len(t, viol, 1, "累计列的中位数显著小于当月列 ⇒ 整族写错了列")
	require.Equal(t, FieldLoanFlowYTD, viol[0].YTDField)
	require.Equal(t, "monthly", viol[0].PeriodType)
}

// TestCaliberFamilyMediansReportsInsufficient：样本不足要**报出来**，不得静默跳过。
//
// ⚠️ 静默跳过正是本任务要修的那个毛病：「没查」与「查过没问题」在报告上不可区分。
func TestCaliberFamilyMediansReportsInsufficient(t *testing.T) {
	obs := []MergedObservation{mkMerged("2023-07", "monthly", map[string]float64{
		FieldLoanFlowYTD: 160800, FieldLoanFlowMoM: 3459,
	})}

	viol, insuf := checkCaliberFamilyMedians(obs, 3)

	require.Empty(t, viol)
	require.Len(t, insuf, 1, "只有 1 个样本、门槛 3 ⇒ 必须报「样本不足」而不是静默跳过")
	require.Equal(t, 1, insuf[0].YTDCount)
}

// TestCaliberFamilyMediansAcceptEqualMedians：两侧中位数相等**不算违反**。
//
// 理由与 TestCaliberRoutingAcceptsEqualValues 同源：1 月的累计就等于当月，某个分档
// 若恰好全由 1 月构成，两侧中位数会相等。DoD 原文的严格 `>` 会把它判成违反 ——
// 那是假阳，已订正进 done_criteria。
func TestCaliberFamilyMediansAcceptEqualMedians(t *testing.T) {
	var obs []MergedObservation
	for i := 0; i < 4; i++ {
		obs = append(obs, mkMerged(fmt.Sprintf("202%d-01", i+1), "monthly",
			map[string]float64{FieldLoanFlowYTD: 49200, FieldLoanFlowMoM: 49200}))
	}

	viol, insuf := checkCaliberFamilyMedians(obs, 3)

	require.Empty(t, viol, "1 月形态两侧相等，是合法的")
	require.Empty(t, insuf)
}

// TestCaliberRoutingSkipCountIsNotSilent：跳过数必须印在报告正文里。
//
// 🔴 这条守的是**断言本身的有效性**，不是断言的结论：若取号逻辑写反，每一对都会被
// 判成异号、断言恒 skip 而报告一片绿。跳过数是这条断言「自己还活着吗」的唯一信号。
//
// ⚠️ 单侧数同理，且更要紧 —— 异号写反是实现缺陷，单侧却是**路由错误本身的典型产物**
// （整族被误判成同一口径 ⇒ 每对都单侧）。两个数都不印，报告在最需要它的时候是绿的。
func TestCaliberRoutingSkipCountIsNotSilent(t *testing.T) {
	res := okResult()
	res.Groups = []MergedObservation{
		mkMerged("2023-07", "monthly", map[string]float64{
			FieldDepositFlowYTD: 189800, FieldDepositFlowMoM: -11200, // 无上期
		}),
		mkMerged("2020-04", "monthly", map[string]float64{
			FieldLoanFlowMoM: 3459, // 单侧
		}),
	}

	var b bytes.Buffer
	require.NoError(t, renderLoadReport(&b, "/x", res))
	out := b.String()

	assert.Contains(t, out, "口径路由核对")
	assert.Contains(t, out, "无上期 1", "无上期数必须出现在报告正文里——「没查」与「查过没问题」必须可区分")
	assert.Contains(t, out, "单侧跳过 1", "单侧跳过数同样必须出声")
	assert.Contains(t, out, "预期违反 0")
}

// TestLoadReportListsComparedPairs：逐对可判观测数必须印出来。
//
// 🔴 理由：「异号跳过数 ≠ 总对数」这条守卫只发现得了「取号逻辑写反」，
// **发现不了「某一对从未被比较过」** —— 而后者正是整族位移的表现。
// 22 对逐条列出，恒为 0 的那一对才看得见。
func TestLoadReportListsComparedPairs(t *testing.T) {
	res := okResult()
	res.Groups = []MergedObservation{
		mkMerged("2023-06", "monthly", map[string]float64{FieldLoanFlowYTD: 157341}),
		mkMerged("2023-07", "monthly", map[string]float64{
			FieldLoanFlowYTD: 160800, FieldLoanFlowMoM: 3459,
		}),
	}

	var b bytes.Buffer
	require.NoError(t, renderLoadReport(&b, "/x", res))
	out := b.String()

	from := strings.Index(out, "逐对可判观测数")
	require.GreaterOrEqual(t, from, 0, "报告里缺了逐对清单")
	to := strings.Index(out[from:], "族内量级核对")
	require.Greater(t, to, 0)
	section := out[from : from+to]

	// 22 对**每一对**都要出现，不能只印非零的——恒为 0 的那些才是要看的
	for _, p := range caliberFamilies() {
		assert.Containsf(t, section, strings.TrimSuffix(p[0], "_ytd"),
			"逐对清单漏了 %s", p[0])
	}
	assert.Contains(t, section, "loan_flow                    1", "被比较过的那一对应记 1")
	assert.Contains(t, section, "loan_bill                    0", "没比较过的对应显式记 0")
}

// TestLoadReportSurfacesFamilyMedianSection：族内量级核对一节必须出现，且样本不足要出声。
func TestLoadReportSurfacesFamilyMedianSection(t *testing.T) {
	res := okResult()
	res.Groups = []MergedObservation{mkMerged("2023-07", "monthly", map[string]float64{
		FieldLoanFlowYTD: 160800, FieldLoanFlowMoM: 3459,
	})}

	var b bytes.Buffer
	require.NoError(t, renderLoadReport(&b, "/x", res))
	out := b.String()

	assert.Contains(t, out, "族内量级核对")
	assert.Contains(t, out, "样本不足未判 1", "样本不足的对必须报出来，不得静默跳过")
}

// TestLoadReportRendersRoutingViolation：违反**被渲染出来**的那条路径必须有测试走过。
//
// 🔴 单测证明 checkCaliberRouting 找得到违反，但闸门触发时人读到的是**报告**。
// 渲染路径没被走过 ⇒ 真出问题那天才第一次执行它。差额的绝对值与百分比尤其要钉：
// 它们是把「路由错（~99%）」与「发布取整 + 央行数据修订（千分之几）」分开的唯一依据。
func TestLoadReportRendersRoutingViolation(t *testing.T) {
	res := okResult()
	res.Groups = []MergedObservation{
		mkMerged("2023-06", "monthly", map[string]float64{FieldLoanFlowYTD: 157341}),
		mkMerged("2023-07", "monthly", map[string]float64{
			FieldLoanFlowYTD: 3459,   // 写反了
			FieldLoanFlowMoM: 160800, //
		}),
	}

	var b bytes.Buffer
	require.NoError(t, renderLoadReport(&b, "/x", res))
	out := b.String()

	assert.Contains(t, out, "共 1 违反")
	assert.Contains(t, out, "2023-07/monthly")
	assert.Contains(t, out, "loan_flow_ytd=3459")
	assert.Contains(t, out, "上期 157341")
	// 期望值 157341 + 160800 = 318141，差 314682，相对 98.913%
	assert.Contains(t, out, "318141", "期望值要印出来，否则人得自己算")
	assert.Contains(t, out, "差 314682", "绝对差要印")
	assert.Contains(t, out, "98.913%", "相对差要印——它才是与发布噪声（千分之几）分开的依据")
}

// TestLoadReportRendersFamilyMedianViolation：族内量级违反同样要走渲染路径。
func TestLoadReportRendersFamilyMedianViolation(t *testing.T) {
	res := okResult()
	var g []MergedObservation
	for i, ytd := range []float64{3000, 3200, 3400} {
		g = append(g, mkMerged(fmt.Sprintf("2023-%02d", i+1), "monthly",
			map[string]float64{FieldLoanFlowYTD: ytd}))
	}
	for i, mom := range []float64{150000, 160000, 170000} {
		g = append(g, mkMerged(fmt.Sprintf("2024-%02d", i+1), "monthly",
			map[string]float64{FieldLoanFlowMoM: mom}))
	}
	res.Groups = g

	var b bytes.Buffer
	require.NoError(t, renderLoadReport(&b, "/x", res))
	out := b.String()

	assert.Contains(t, out, "族内量级核对（按 period_type，预期违反 0，共 1 违反")
	assert.Contains(t, out, "整族可能写错列")
	assert.Contains(t, out, "median|ytd|=3200", "两侧中位数都要印，才看得出差多少")
	assert.Contains(t, out, "median|mom|=160000")
}
