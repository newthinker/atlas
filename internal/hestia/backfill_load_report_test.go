package hestia

import (
	"bytes"
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
		"标定输入", "语料总篇数", "四道恒等式", "合并组明细", "部分覆盖的期次", "字段冲突", "落 pending 的期次",
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
