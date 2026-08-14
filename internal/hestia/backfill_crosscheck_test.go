package hestia

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Context Checkpoint: M1c-1 的 TASK-005，done_criteria → test mapping
//
//	functional[0]     "抓取集 = A ∪ B，交集只出现一次"
//	                  → TestCrossCheckBackfillUnionsBothSides
//	                  → TestCrossCheckBackfillPrefersIndexSideOnOverlap（交集取哪一侧的字段）
//	functional[1]     "only_in_index = A \\ B，断言具体 id 集合"
//	functional[2]     "only_in_search = B \\ A，断言具体 id 集合"
//	                  → 两条同由 TestCrossCheckBackfillUnionsBothSides 逐 id 断言
//	boundary[0]       "两侧完全相同 ⇒ 两差集空、并集 = 任一侧"
//	                  → TestCrossCheckBackfillIdenticalSides
//	boundary[1]       "搜索侧空集（非错误）⇒ only_in_index = 全部 A、并集 = A、reason 为空"
//	                  → TestCrossCheckBackfillEmptySearchIsNotFailure
//	boundary[2]       "补 TASK-004 日期守卫『上界』那半的测试背书"
//	                  → TestCheckBackfillSearchDateRangeRejectsAboveUpperBound
//	                  → TestCheckBackfillSearchDateRangeGuardsBothEnds（成对，钉住两半都在）
//	error_handling[0] "搜索侧失效 ⇒ nil error + 抓取集 = A + reason 非空"
//	                  → TestCrossCheckBackfillFailsOpenOnSearchError
//	                  → TestCrossCheckBackfillSkipLeavesDiffsEmpty（跳过时差集必须空，不得谎报）

// 两侧的候选都用 {id, url, title, published} 四元组，构造出「有交有差」的一组：
//
//	index 侧 A = {a, b, c}
//	搜索侧 B = {b, c, d}
//	∪ = {a, b, c, d}   A\B = {a}   B\A = {d}
//
// 刻意让交集有两条（b、c）而不是一条：只有一条时，「交集去重」与「碰巧只留了第一条」
// 这两种实现分不开。
const (
	xcIDOnlyIndex  = "2025092212550638999"
	xcIDBoth1      = "5837468"
	xcIDBoth2      = "2025092212554883949"
	xcIDOnlySearch = "5868082"
)

func xcIndexItem(id, title, published string) backfillItem {
	return backfillItem{
		ArticleID: id,
		URL:       "https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/" + id + "/index.html",
		Title:     title,
		Published: published,
	}
}

func xcSearchHit(id, title, published string) backfillSearchHit {
	return backfillSearchHit{
		ArticleID: id,
		URL:       "https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/" + id + "/index.html",
		Title:     title,
		Published: published,
	}
}

// xcSides 返回上面那组有交有差的候选。
func xcSides() ([]backfillItem, []backfillSearchHit) {
	index := []backfillItem{
		xcIndexItem(xcIDOnlyIndex, "2020年3月社会融资规模存量统计数据报告", "2020-04-10"),
		xcIndexItem(xcIDBoth1, "2025年8月社会融资规模存量统计数据报告", "2025-09-12"),
		xcIndexItem(xcIDBoth2, "2025年7月社会融资规模存量统计数据报告", "2025-08-13"),
	}
	search := []backfillSearchHit{
		xcSearchHit(xcIDBoth1, "2025年8月社会融资规模存量统计数据报告", "2025-09-12"),
		xcSearchHit(xcIDBoth2, "2025年7月社会融资规模存量统计数据报告", "2025-08-13"),
		xcSearchHit(xcIDOnlySearch, "2025年前三季度金融统计数据报告", "2025-10-15"),
	}
	return index, search
}

func xcIDs(cands []backfillCandidate) []string {
	out := make([]string, 0, len(cands))
	for _, c := range cands {
		out = append(out, c.ArticleID)
	}
	return out
}

func TestCrossCheckBackfillUnionsBothSides(t *testing.T) {
	index, search := xcSides()

	got := crossCheckBackfill(index, search, nil)

	// 并集：四条，交集那两条各只出现一次。顺序是「index 侧原序 + 搜索侧独有」，
	// 定序是刻意的 —— manifest 要能逐次比对，随机顺序会让每次重跑都产生假 diff。
	assert.Equal(t, []string{xcIDOnlyIndex, xcIDBoth1, xcIDBoth2, xcIDOnlySearch}, xcIDs(got.Fetch))

	// 两个差集断言**具体 id**，不是条数 —— 条数相同而内容错位的实现照样能过条数断言。
	assert.Equal(t, []string{xcIDOnlyIndex}, got.OnlyInIndex, "A \\ B：搜索没索到的")
	assert.Equal(t, []string{xcIDOnlySearch}, got.OnlyInSearch, "B \\ A：index 没翻到的，真风险信号")

	// 交叉校验真做过 ⇒ 没有跳过的理由。
	assert.Empty(t, got.SearchSkippedReason)
}

// TestCrossCheckBackfillPrefersIndexSideOnOverlap：两侧都有的那条，字段取 **index 侧**。
//
// index 侧的标题取自列表项的 `title=` 属性（原文），搜索侧的标题是剥过 <font> 高亮的
// 重建值；URL 也是 index 侧经 resolveURL 归一的。两者通常一致，但**不一致时以主路径为准**
// —— 交叉校验是附加防线，不该反过来改写主路径的数据。
//
// 这条用例刻意让两侧的 Title 不同，否则「取哪一侧」在测试里根本不可观测。
func TestCrossCheckBackfillPrefersIndexSideOnOverlap(t *testing.T) {
	index := []backfillItem{xcIndexItem(xcIDBoth1, "index 侧的标题", "2025-09-12")}
	search := []backfillSearchHit{xcSearchHit(xcIDBoth1, "搜索侧的标题", "2025-09-12")}

	got := crossCheckBackfill(index, search, nil)

	require.Len(t, got.Fetch, 1)
	assert.Equal(t, "index 侧的标题", got.Fetch[0].Title)
	assert.Equal(t, backfillSourceBoth, got.Fetch[0].Source)
}

// TestCrossCheckBackfillTagsSource：每条候选带 index|search|both。
// manifest 的 Article.Source 是这个口径（TASK-003 的契约），而**只有交叉校验这一层
// 知道答案** —— 不在这里给出，TASK-006 就得把交集算法再实现一遍。
func TestCrossCheckBackfillTagsSource(t *testing.T) {
	index, search := xcSides()

	got := crossCheckBackfill(index, search, nil)

	want := map[string]string{
		xcIDOnlyIndex:  backfillSourceIndex,
		xcIDBoth1:      backfillSourceBoth,
		xcIDBoth2:      backfillSourceBoth,
		xcIDOnlySearch: backfillSourceSearch,
	}
	require.Len(t, got.Fetch, len(want))
	for _, c := range got.Fetch {
		assert.Equal(t, want[c.ArticleID], c.Source, "id=%s", c.ArticleID)
	}
}

func TestCrossCheckBackfillIdenticalSides(t *testing.T) {
	index, _ := xcSides()
	search := make([]backfillSearchHit, 0, len(index))
	for _, it := range index {
		search = append(search, xcSearchHit(it.ArticleID, it.Title, it.Published))
	}

	got := crossCheckBackfill(index, search, nil)

	assert.Equal(t, []string{xcIDOnlyIndex, xcIDBoth1, xcIDBoth2}, xcIDs(got.Fetch), "并集应等于任一侧")
	assert.Empty(t, got.OnlyInIndex)
	assert.Empty(t, got.OnlyInSearch)
	assert.Empty(t, got.SearchSkippedReason)
	for _, c := range got.Fetch {
		assert.Equal(t, backfillSourceBoth, c.Source)
	}
}

// TestCrossCheckBackfillEmptySearchIsNotFailure：搜索侧**空集**（不是错误）。
//
// ⚠️ 这与「搜索侧失效」是**两种不同情形，处置不同**：
//   - 空集 = 搜索确实一条都没索到（可能是真的，也可能是关键词写错了）⇒ 照常做校验，
//     全部 A 落进 only_in_index，`SearchSkippedReason` **为空**。
//   - 失效 = 这条路没走通 ⇒ 跳过校验，差集留空，`SearchSkippedReason` **非空**。
//
// 合成一种处置的后果很具体：「关键词写错导致 0 条」会被当成「服务挂了」而静默放过。
func TestCrossCheckBackfillEmptySearchIsNotFailure(t *testing.T) {
	index, _ := xcSides()

	got := crossCheckBackfill(index, nil, nil)

	assert.Equal(t, []string{xcIDOnlyIndex, xcIDBoth1, xcIDBoth2}, xcIDs(got.Fetch))
	assert.Equal(t, []string{xcIDOnlyIndex, xcIDBoth1, xcIDBoth2}, got.OnlyInIndex,
		"搜索一条都没索到 ⇒ 全部 A 都是「搜索没索到的」")
	assert.Empty(t, got.OnlyInSearch)
	assert.Empty(t, got.SearchSkippedReason, "空集不是失效，没有跳过理由")
}

// TestCrossCheckBackfillFailsOpenOnSearchError：搜索侧失效 ⇒ fail-open。
//
// ADR-M1c1-02：交叉校验是**附加防线**，不该让一个不可控的第三方检索服务有权否决
// 主路径交付。三个断言缺一不可 —— 只断言 nil error 时，「抓取集被清空」这个 bug
// 照样能过。
func TestCrossCheckBackfillFailsOpenOnSearchError(t *testing.T) {
	index, _ := xcSides()
	searchErr := errors.New("hestia backfill search: HTTP 503")

	got := crossCheckBackfill(index, nil, searchErr)

	assert.Equal(t, []string{xcIDOnlyIndex, xcIDBoth1, xcIDBoth2}, xcIDs(got.Fetch), "抓取集退化为完整的 index 侧")
	require.NotEmpty(t, got.SearchSkippedReason, "有声跳过，不静默")
	assert.Contains(t, got.SearchSkippedReason, "HTTP 503", "跳过理由要带上原始错误")
	for _, c := range got.Fetch {
		assert.Equal(t, backfillSourceIndex, c.Source, "没做校验，不能声称某条也在搜索侧")
	}
}

// TestCrossCheckBackfillSkipLeavesDiffsEmpty：跳过校验时两个差集**必须为空**。
//
// 反面是一个很容易写出、且后果具体的 bug：把「搜索侧为空」的处置直接复用到失效上，
// 于是 `only_in_index` 变成**全部 A**（可能几百条）。那是一句**谎报** —— 它宣称
// 「搜索没索到这几百篇」，而事实是**根本没问过搜索**。读 manifest 的人无从分辨。
func TestCrossCheckBackfillSkipLeavesDiffsEmpty(t *testing.T) {
	index, search := xcSides()

	got := crossCheckBackfill(index, search, errors.New("boom"))

	assert.Empty(t, got.OnlyInIndex, "没问过搜索，就不能说「搜索没索到」")
	assert.Empty(t, got.OnlyInSearch)
	assert.Len(t, got.Fetch, len(index), "搜索侧那条独有的不该混进来 —— 那份数据没被校验过")
}

// TestCrossCheckBackfillDedupsEachSide：两侧各自的重复都只穿透一次。
//
// 上游契约说两侧都已去重（TASK-002 的 `Reports 已按 article_id 去重`、TASK-004 的
// 分页去重），所以这是**防御性**的。防御性代码在真实语料上没有守卫 —— 不钉住的话，
// 把两处 dedup 删掉全部测试照样绿（与 `tagRE` 同则）。
//
// ⚠️ **两侧必须分开钉**：搜索侧那处的条件是 `inIndex[id] || seenSearch[id]`，
// **一条语句、两个半边共用一格覆盖率**。只喂「两侧都有」的重复时，`seenSearch`
// 那半永远走不到，而覆盖率显示 100% —— 这正是本任务 boundary[2] 要补的那个缺口
// 的同一形状（`h.Published < lo || h.Published > hi` 删掉后半无人发现）。
func TestCrossCheckBackfillDedupsEachSide(t *testing.T) {
	t.Run("index 侧重复", func(t *testing.T) {
		dup := xcIndexItem(xcIDBoth1, "标题", "2025-09-12")
		got := crossCheckBackfill([]backfillItem{dup, dup}, nil, nil)

		assert.Equal(t, []string{xcIDBoth1}, xcIDs(got.Fetch))
		// 差集同样只该出现一次 —— 不去重时它会被记两遍，manifest 里凭空多一条。
		assert.Equal(t, []string{xcIDBoth1}, got.OnlyInIndex)
	})

	t.Run("搜索侧重复(且 index 侧没有它)", func(t *testing.T) {
		dup := xcSearchHit(xcIDOnlySearch, "标题", "2025-10-15")
		got := crossCheckBackfill(nil, []backfillSearchHit{dup, dup}, nil)

		assert.Equal(t, []string{xcIDOnlySearch}, xcIDs(got.Fetch))
		assert.Equal(t, []string{xcIDOnlySearch}, got.OnlyInSearch)
	})
}

// TestCheckBackfillSearchDateRangeRejectsAboveUpperBound 补 M1c-1 TASK-004 遗留的缺口
// （本任务 boundary[2]）。
//
// 🔴 立项依据：test-agent-28 复验 TASK-004 时的一个 **SURVIVED** 变异 ——
//
//   - if h.Published < lo || h.Published > hi {
//   - if h.Published < lo {
//
// 删掉上界那半，**本任务用例 / 整包 / go test ./... 三级全部 exit 0**。
// 成因：TASK-004 的越界用例日期全在 `from` **之下**（2015-02-10），而
// AcceptsBoundaryDates 用的是恰好等于 `to` 的日期，只能证明比较不是 `>=`，
// **证不了上界存在**。
//
// ⚠️ `checkBackfillSearchDateRange` 的语句覆盖率是 **100%**，对这个缺口毫无信号 ——
// 整个 `if` 是一条语句，两个半边共用一格。**覆盖率 ≠ 守卫有效**的一个干净实例。
//
// 为什么补在这里：该分支在本 sprint 不可达（CLI 只有 `--from`，`to` 恒为今天），
// 但 `to` 是**参数不是常量**；M1c-2 一旦回填某个历史区间，上界就变成 `desc` 排序下
// **第 1 页就触发**的那一半 —— 即最早的检测点。
func TestCheckBackfillSearchDateRangeRejectsAboveUpperBound(t *testing.T) {
	from := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2022, 12, 31, 0, 0, 0, 0, time.UTC) // 历史区间，上界不是「今天」

	hits := []backfillSearchHit{
		xcSearchHit(xcIDBoth1, "2021年8月社会融资规模存量统计数据报告", "2021-09-10"), // 区间内
		xcSearchHit(xcIDBoth2, "2025年7月社会融资规模存量统计数据报告", "2025-08-13"), // **超出上界**
	}

	err := checkBackfillSearchDateRange(hits, from, to)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "2025-08-13", "要指出越界的那条日期")
	assert.Contains(t, err.Error(), "2022-12-31", "要指出被越过的上界")
}

// TestCheckBackfillSearchDateRangeGuardsBothEnds 把两半**成对**钉住。
//
// 单写上一条时，「只判上界、不判下界」的实现照样能过 —— 与它本来要修的缺口同形，
// 只是方向相反。表驱动让两半在同一处可见，删掉任一半都会有一格转红。
func TestCheckBackfillSearchDateRangeGuardsBothEnds(t *testing.T) {
	from := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2022, 12, 31, 0, 0, 0, 0, time.UTC)

	for _, tc := range []struct {
		name      string
		published string
		wantErr   bool
	}{
		{"低于下界", "2019-12-31", true},
		{"恰好等于下界", "2020-01-01", false},
		{"区间内", "2021-06-15", false},
		{"恰好等于上界", "2022-12-31", false},
		{"高于上界", "2023-01-01", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := checkBackfillSearchDateRange(
				[]backfillSearchHit{xcSearchHit(xcIDBoth1, "标题", tc.published)}, from, to)
			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.published)
				return
			}
			require.NoError(t, err)
		})
	}
}
