package hestia

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Context Checkpoint: M1c-1 的 TASK-004，done_criteria → test mapping
//
//	functional[0]     "查询 URL 逐参数含全部实测参数 + 中文正确编码"
//	                  → TestBackfillSearchURLCarriesMeasuredParams
//	                  → TestBackfillSearchURLEncodesChineseKeyword
//	functional[1]     "取绝对 URL/标题/发布日期，标题剥 <font> 高亮且不含 '<'"
//	                  → TestParseBackfillSearchPageExtractsFields
//	functional[2]     "只保留 /goutongjiaoliu/113456/113469/ 前缀，同标题两条筛后剩一条"
//	                  → TestFilterBackfillSearchHitsKeepsOnlyGoutongjiaoliu
//	                  → TestFilterBackfillSearchHitsDropsByPrefixNotByIDShape
//	functional[3]     "总页数从 default-result-total-pages 解析"
//	                  → TestParseBackfillSearchPageReadsTotalPages
//	                  → TestParseBackfillSearchPageRejectsMissingTotals
//	boundary[0]       "0 条结果 ⇒ 报错"
//	                  → TestParseBackfillSearchPageRejectsZeroResults
//	                  → TestParseBackfillSearchPageAcceptsPageWithNoKeptColumn（反向：筛后 0 条不报错）
//	boundary[1]       "日期过滤失效 ⇒ 报错"（返工 FIX-1 后判**性质**不判数量）
//	                  → TestFetchBackfillSearchPageRejectsOutOfRangePublished（主判据）
//	                  → TestFetchBackfillSearchPageAcceptsBoundaryDates（反向：闭区间端点放行）
//	                  → TestFetchBackfillSearchPageChecksAllColumnsDates（校验在栏目筛之前）
//	                  → TestParseBackfillSearchPageRejectsAbsurdTotal（粗上界，**不再**指认 advtime）
//	                  → TestParseBackfillSearchPageAcceptsLargestMeasuredTotal（反向：692 与 1136 都放行）
//	返工 FIX-2       "本页条数自洽，静默丢条 ⇒ 报错"
//	                  → TestParseBackfillSearchPageChecksCountAgainstSiteReported（检测层，含末页反向）
//	                  → TestParseBackfillSearchPageRejectsMisalignedItems（只钉错误文案，非独立防线）
//	error_handling[0] "Fetcher 错误 / HTTP 非 200 原样上抛，本层不降级"
//	                  → TestFetchBackfillSearchPagePropagatesFetchError

// 样本 testdata/pboc-search-p1.html 的实测形态（2026-08-14 抓取，HTTP 200）：
//
// ⚠️ 仓库里的这份是 **LF 版**：原始响应 32405 字节含 33 个 CR，被 core.autocrlf=input
// 规范化成 32372 字节（与包内两份 index 快照同）。本文件的正则都用 `(?s)` 与 `\s*`，
// 行尾形态不影响匹配；但若哪天有人照着原始响应写 `\r\n` 的字面量断言，会在仓库版上失配。
//
// qAll=社会融资规模存量统计数据报告、2020-01-01..2026-08-14、pNo=1
// ⇒ total-records=137、total-pages=12、12 条结果 = 6 条 goutongjiaoliu + 6 条
// diaochatongjisi **完美成对**（同一篇报告在两个栏目各一份），12 条标题全部带
// <font color='#FF0000'> 高亮。
const (
	searchSampleItems      = 12
	searchSampleKept       = 6
	searchSampleTotalPages = 12

	// 成对出现的那个标题：样本里出现两次（两个栏目各一份），筛后应只剩一条。
	searchSampleDupTitle = "2025年8月社会融资规模存量统计数据报告"
	// 该标题在 goutongjiaoliu 栏目下的那一份 —— 筛后保留的必须是它。
	searchSampleKeptURL = "https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/5837468/index.html"
	searchSampleKeptID  = "5837468"
	// 同一篇在调查统计司栏目下的那一份 —— 32 位 hex 的 article_id，必须丢弃。
	searchSampleHexURL = "https://www.pbc.gov.cn/diaochatongjisi/116219/116225/35ec0aa27604417888826e7ff128cc4a/index.html"
	// ⚠️ 同为调查统计司栏目、article_id 却是 **19 位纯数字**，与 goutongjiaoliu 侧撞形。
	// 样本里 6 条 diaochatongjisi 只有 2 条是 32 位 hex，另 4 条是这个形态。
	searchSampleDigitDropURL = "https://www.pbc.gov.cn/diaochatongjisi/116219/116225/2025080618505078072/index.html"
)

func TestBackfillSearchURLCarriesMeasuredParams(t *testing.T) {
	from := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)

	raw := backfillSearchURL("社会融资规模存量统计数据报告", from, to, 3)

	u, err := url.Parse(raw)
	require.NoError(t, err)
	assert.Equal(t, "https://wzdig.pbc.gov.cn/search/pcRender", u.Scheme+"://"+u.Host+u.Path)

	q, err := url.ParseQuery(u.RawQuery)
	require.NoError(t, err)

	// 逐参数断言（全部来自 requirements-analysis §4 的实测）。
	// ⚠️ qAll 而不是 q：实测同一关键词 q=1324 条 / qAll=24 条，差 25 倍。
	for _, c := range []struct{ key, want string }{
		{"pageId", "c177a85bd02b4114bebebd210809f691"},
		{"advSearch", "true"},
		{"searchArea", "title"},
		{"sr", "dateTime desc"},
		{"qAll", "社会融资规模存量统计数据报告"},
		{"advtime", "5"},
		{"startTime", "2020-01-01"},
		{"endTime", "2026-08-14"},
		{"pNo", "3"},
	} {
		assert.Equal(t, c.want, q.Get(c.key), "参数 %s", c.key)
	}

	// 分词 OR 的 q 与「等于空查询」的 advepq/adveq 一个都不能出现 ——
	// 前者差 25 倍，后两者实测返回全站 549141 条。
	for _, forbidden := range []string{"q", "advepq", "adveq"} {
		_, present := q[forbidden]
		assert.False(t, present, "不该出现参数 %s", forbidden)
	}
}

func TestBackfillSearchURLEncodesChineseKeyword(t *testing.T) {
	raw := backfillSearchURL("社会融资规模存量统计数据报告", time.Time{}, time.Time{}, 1)

	// 中文必须是 percent-encoding，不能原样出现在 URL 里。
	assert.Contains(t, raw, "qAll=%E7%A4%BE%E4%BC%9A%E8%9E%8D%E8%B5%84%E8%A7%84%E6%A8%A1%E5%AD%98%E9%87%8F%E7%BB%9F%E8%AE%A1%E6%95%B0%E6%8D%AE%E6%8A%A5%E5%91%8A")
	for _, r := range raw {
		require.Less(t, r, rune(0x80), "URL 里出现了未编码的非 ASCII 字符: %q", raw)
	}
}

func TestParseBackfillSearchPageExtractsFields(t *testing.T) {
	hits, _, err := parseBackfillSearchPage(readTestdata(t, "pboc-search-p1.html"), 1)
	require.NoError(t, err)
	require.Len(t, hits, searchSampleItems)

	// 绝对 URL、标题、发布日期三样，逐字断言第一条。
	assert.Equal(t, backfillSearchHit{
		ArticleID: searchSampleKeptID,
		URL:       searchSampleKeptURL,
		Title:     searchSampleDupTitle,
		Published: "2025-09-12",
	}, hits[0])

	// 标题必须剥净 <font color='#FF0000'> 高亮 —— 12 条全部带，一条都不能漏。
	for _, h := range hits {
		assert.NotContains(t, h.Title, "<", "标题未剥净标签: %q", h.Title)
		assert.NotEmpty(t, h.Published)
		assert.True(t, strings.HasPrefix(h.URL, "https://"), "URL 不是绝对的: %q", h.URL)
	}
}

// TestParseBackfillSearchPageStripsHighlightIsNotVacuous 证明上一条的「剥标签」断言
// 不是在空集上平凡为真：样本里 12 条标题的原文**全部**含 <font 高亮。
// 没有这条，把 tagRE 那一步删掉后只要样本恰好不含高亮，测试照样绿。
func TestParseBackfillSearchPageStripsHighlightIsNotVacuous(t *testing.T) {
	raw := string(readTestdata(t, "pboc-search-p1.html"))
	assert.Equal(t, searchSampleItems*5, strings.Count(raw, "<font color='#FF0000'>"),
		"样本里每条标题被拆成 5 段高亮（社会/融资/规模/存量/统计数据报告）")
}

func TestFilterBackfillSearchHitsKeepsOnlyGoutongjiaoliu(t *testing.T) {
	hits, _, err := parseBackfillSearchPage(readTestdata(t, "pboc-search-p1.html"), 1)
	require.NoError(t, err)

	// 筛之前：那个标题出现两次（两个栏目各一份）。
	require.Equal(t, 2, countTitle(hits, searchSampleDupTitle),
		"样本前提变了：该标题不再是成对出现的")

	kept := filterBackfillSearchHits(hits)
	require.Len(t, kept, searchSampleKept)

	// 筛之后：只剩一条，且保留的是 goutongjiaoliu 那条。
	assert.Equal(t, 1, countTitle(kept, searchSampleDupTitle))
	for _, h := range kept {
		if h.Title == searchSampleDupTitle {
			assert.Equal(t, searchSampleKeptURL, h.URL)
			assert.Equal(t, searchSampleKeptID, h.ArticleID)
		}
		assert.Contains(t, h.URL, "/goutongjiaoliu/113456/113469/")
	}
}

// TestFilterBackfillSearchHitsDropsByPrefixNotByIDShape 钉住判据是**栏目前缀**，
// 不是 article_id 的形态。
//
// ⚠️ 立项理由是实测反例：样本里 6 条 diaochatongjisi 只有 **2 条**是 32 位 hex
// （`35ec0aa2…`），另 4 条是 **19 位纯数字**（`2025080618505078072`）——与
// goutongjiaoliu 侧的 id 形态完全撞形。按「32 位 hex」筛会把那 4 条放进来，
// 而它们的 id 在 index 侧根本不存在，TASK-005 的差集会凭空多出 4 条假信号。
func TestFilterBackfillSearchHitsDropsByPrefixNotByIDShape(t *testing.T) {
	hits, _, err := parseBackfillSearchPage(readTestdata(t, "pboc-search-p1.html"), 1)
	require.NoError(t, err)
	require.True(t, containsURL(hits, searchSampleHexURL), "样本前提变了：32 位 hex 那条不在了")
	require.True(t, containsURL(hits, searchSampleDigitDropURL), "样本前提变了：19 位数字的调统司那条不在了")

	kept := filterBackfillSearchHits(hits)
	assert.False(t, containsURL(kept, searchSampleHexURL), "32 位 hex 那条该被丢")
	assert.False(t, containsURL(kept, searchSampleDigitDropURL), "19 位数字的调统司那条同样该被丢")
	for _, h := range kept {
		assert.NotContains(t, h.URL, "/diaochatongjisi/")
	}
}

func TestParseBackfillSearchPageReadsTotalPages(t *testing.T) {
	_, totalPages, err := parseBackfillSearchPage(readTestdata(t, "pboc-search-p1.html"), 1)
	require.NoError(t, err)
	assert.Equal(t, searchSampleTotalPages, totalPages)
}

// TestParseBackfillSearchPageRejectsMissingTotals：解析不到计数字段 ⇒ 报错。
// 与 parsePaging 同则 —— 静默退化成「只看第 1 页」时，调用方永远发现不了还有 11 页。
//
// ⚠️ 断言**钉具体文案**而不是只 require.Error：返工加了页容量守卫之后，「计数字段
// 缺失时静默返回 0」这个变异会让**那条守卫**代为报错（records=0 ⇒ 期望 0 条、实得
// 1 条），只判「有没有错」的话该变异会存活（实测：它确实存活过一轮）。
// 报错的是谁，和有没有报错，是两件事。
func TestParseBackfillSearchPageRejectsMissingTotals(t *testing.T) {
	for _, tc := range []struct{ name, page, want string }{
		{"没有 total-pages", `<input id="default-result-total-records" value="137"/>` + searchItemHTML(searchSampleKeptURL, "标题", "2025年09月12日"), "no total-pages field"},
		{"没有 total-records", `<input id="default-result-total-pages" value="12"/>` + searchItemHTML(searchSampleKeptURL, "标题", "2025年09月12日"), "no total-records field"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := parseBackfillSearchPage([]byte(tc.page), 1)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want, "报错的必须是计数字段守卫本身")
		})
	}
}

// TestParseBackfillSearchPageRejectsZeroResults：一条都解析不到 ⇒ 报错。
// 计数字段俱全（137 条 / 12 页）却匹配不到任何结果，只可能是结果结构改版。
func TestParseBackfillSearchPageRejectsZeroResults(t *testing.T) {
	_, _, err := parseBackfillSearchPage(searchPageHTML(137, 12), 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "0 results")
}

// TestParseBackfillSearchPageAcceptsPageWithNoKeptColumn 是上一条的**反向**用例：
// 「解析到 0 条」报错，「解析到了但筛完剩 0 条」**不报错**。
//
// 两者必须分开判（与 design-spec §3.2 index 侧那两条同则）：只写前者时，把
// 「筛完 0 条也报错」这个 bug 写进去测试照样绿，而某一页 12 条恰好全是调统司栏目
// 是完全可能的，那时报错会把整条交叉校验打断。
func TestParseBackfillSearchPageAcceptsPageWithNoKeptColumn(t *testing.T) {
	page := searchPageHTML(2, 1,
		searchItemHTML(searchSampleHexURL, "2025年8月社会融资规模存量统计数据报告", "2025年09月12日"),
		searchItemHTML(searchSampleDigitDropURL, "2025年6月社会融资规模存量统计数据报告", "2025年07月14日"),
	)
	hits, _, err := parseBackfillSearchPage(page, 1)
	require.NoError(t, err)
	require.Len(t, hits, 2)
	assert.Empty(t, filterBackfillSearchHits(hits))
}

// TestParseBackfillSearchPageRejectsAbsurdTotal：条数大到不可能 ⇒ 报错。
//
// ⚠️ **返工要点（FIX-1）：这条守卫的错误文案不再指认 `advtime`。**
// 原文案写着「advtime 日期过滤可能已失效」，而 advtime 失效时实测返回的是 240 /
// 1136 条（该关键词的全部历史结果），**远在 5000 以下、这条守卫根本不会触发**。
// 认那个失效模式的是 TestFetchBackfillSearchPageRejectsOutOfRangePublished。
//
// 本条现在只是一道**没有已知触发场景**的粗上界，见实现里的注释（我实测过它名义上
// 的两个场景都由别的守卫拦下）。断言里显式钉住「不出现 advtime」——否则文案改回去
// 也没有任何东西会红。
// ⚠️ 页面刻意造成**满页 12 条**、与计数字段自洽：否则页容量守卫会抢先报错，
// 把上界抬高的变异「替它答掉」（实测：一开始只放 1 条，抬高上界后该变异存活）。
// 断言也钉住 ceiling 这个词 —— 只判「有没有错」认不出报错的是谁。
func TestParseBackfillSearchPageRejectsAbsurdTotal(t *testing.T) {
	page := searchPageHTML(549141, 45762, searchItems(backfillSearchPageSize)...)

	_, _, err := parseBackfillSearchPage(page, 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ceiling", "报错的必须是上界守卫本身")
	assert.Contains(t, err.Error(), "549141", "错误信息必须带上实际条数")
	assert.NotContains(t, err.Error(), "advtime",
		"这条守卫不认 advtime 失效（实测 240/1136 条根本触发不到它），文案不得再这样指认")
}

// TestParseBackfillSearchPageAcceptsLargestMeasuredTotal 是上一条的**反向**用例：
// 实测单关键词全区间最大值（`金融统计数据报告` = 692 条）必须放行。
// 没有这条，把阈值设成 1 也「测试全绿」，而那会让搜索侧一次都跑不起来。
//
// ⚠️ 返工后这个数字有了新下界：`advtime` 失效时实测 1136 条（三关键词最大），
// **上界必须高于它**才不会把 FIX-1 那个失效模式误报成「条数骤增」——两条守卫要
// 各司其职，别让粗上界抢先报出一个指错方向的错误。
func TestParseBackfillSearchPageAcceptsLargestMeasuredTotal(t *testing.T) {
	for _, records := range []int{692, 1136} {
		page := searchPageHTML(records, 58, searchItems(backfillSearchPageSize)...)

		hits, totalPages, err := parseBackfillSearchPage(page, 1)
		require.NoError(t, err, "records=%d 必须放行", records)
		assert.Equal(t, 58, totalPages)
		assert.Len(t, hits, backfillSearchPageSize)
	}
}

// TestParseBackfillSearchPageRejectsMisalignedItems（FIX-2）：条目数与结构标记数
// 不一致 ⇒ 报错。
//
// 立项理由是 test-agent-28 的最小复现：某条目**缺日期**时，非贪婪的结果正则会
// **静默错配并丢条** —— 2 条只匹配出 1 条，且那 1 条是「A 的 URL/标题 + **B 的
// 日期**」，B 整条消失、A 的日期是错的，**全程无错误**。
// 「0 条 ⇒ 报错」看不见这种**部分**丢失，而部分丢失正是「manifest 少几十期」的
// 最可能成因。与 index 侧 TASK-001 的 boundary[2]（匹配数须等于 istitle 计数）同族。
//
// ⚠️ **这条用例钉的是「错误文案指出成因」，不是「有没有报错」** —— 我消融实测过：
// 把结构标记那一步短路掉，本输入**照样报错**（丢了条必然与 total-records 对不上，
// 由 ChecksCountAgainstSiteReported 那条守卫接住）。别据本用例说搜索侧有两道独立的
// 丢条防线；检测层只有一道，这一条升级的是诊断。
func TestParseBackfillSearchPageRejectsMisalignedItems(t *testing.T) {
	page := searchPageHTML(2, 1,
		searchItemHTMLNoDate(searchSampleKeptURL, "2025年8月社会融资规模存量统计数据报告"),
		searchItemHTML(searchSampleDigitDropURL, "2025年6月社会融资规模存量统计数据报告", "2025年07月14日"),
	)

	_, _, err := parseBackfillSearchPage(page, 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "2 result blocks", "错误要指出结构标记数")
	assert.Contains(t, err.Error(), "1", "错误要指出实际匹配数")
}

// TestParseBackfillSearchPageChecksCountAgainstSiteReported（FIX-2）：本页条数必须与
// **站点自报的** total-records 及页容量自洽。
//
// **这一条才是丢条的检测层**（上一条只升级错误文案，见那里的说明）：它锚在站点自己
// 报的数字上，不与 item 正则共享形态假设；若站点把某些条目渲染成别的形态，页内标记数
// 与匹配数会**一起**少掉、看上去「自洽」，而与 total-records 一比就现形。
//
// 页容量 12 与末页算术都是实测（2026-08-14，qAll=社会融资规模存量统计数据报告，
// 137 条 / 12 页）：pNo=2 与 pNo=11 各 12 条，pNo=12 恰好 5 条 = 137 − 11×12。
func TestParseBackfillSearchPageChecksCountAgainstSiteReported(t *testing.T) {
	t.Run("非末页少一条 ⇒ 报错", func(t *testing.T) {
		page := searchPageHTML(137, 12, searchItems(11)...)
		_, _, err := parseBackfillSearchPage(page, 2)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "12")
	})

	t.Run("末页不满 ⇒ 放行", func(t *testing.T) {
		page := searchPageHTML(137, 12, searchItems(5)...)
		hits, _, err := parseBackfillSearchPage(page, 12)
		require.NoError(t, err, "末页 137−11×12=5 条是正常的，不能误报")
		assert.Len(t, hits, 5)
	})

	t.Run("单页结果必须全给出", func(t *testing.T) {
		page := searchPageHTML(5, 1, searchItems(4)...)
		_, _, err := parseBackfillSearchPage(page, 1)
		require.Error(t, err, "只有一页却少给一条，同样是静默丢条")
	})
}

// TestParseBackfillSearchPageRejectsIDlessKeptURL：要保留的栏目里取不出
// article_id ⇒ 报错。它是交叉校验的键，空 id 会让 TASK-005 把若干条归到同一个 ""
// 键上 —— 差集算错而没有任何错误。别的栏目取不出无所谓（下一步就筛掉了），
// 这条用例把两者都跑一遍。
func TestParseBackfillSearchPageRejectsIDlessKeptURL(t *testing.T) {
	t.Run("保留栏目取不出 id ⇒ 报错", func(t *testing.T) {
		page := searchPageHTML(1, 1, searchItemHTML(
			"https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/5837468/index.shtml",
			"2025年8月社会融资规模存量统计数据报告", "2025年09月12日"))
		_, _, err := parseBackfillSearchPage(page, 1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no article id")
	})

	t.Run("其他栏目取不出 id ⇒ 不报错，随后被筛掉", func(t *testing.T) {
		page := searchPageHTML(2, 1,
			searchItemHTML("https://www.pbc.gov.cn/diaochatongjisi/116219/116225/whatever",
				"2025年8月社会融资规模存量统计数据报告", "2025年09月12日"),
			searchItemHTML(searchSampleKeptURL, searchSampleDupTitle, "2025年09月12日"))
		hits, _, err := parseBackfillSearchPage(page, 1)
		require.NoError(t, err)
		require.Len(t, hits, 2)
		assert.Empty(t, hits[0].ArticleID)
		assert.Len(t, filterBackfillSearchHits(hits), 1)
	})
}

// TestParseBackfillSearchPageRejectsOverflowingCount：计数字段大到 Atoi 溢出 ⇒ 报错。
//
// 正则是 `(\d+)`、**没有长度上限**，所以这条分支是可达的（与 parsePaging 里同形状
// 的那个分支同理）。不判的话 Atoi 返回 0 + err，静默当成「0 条」往下走。
func TestParseBackfillSearchPageRejectsOverflowingCount(t *testing.T) {
	page := []byte(searchItemHTML(searchSampleKeptURL, "标题", "2025年09月12日") + `
		<input type="hidden" id="default-result-total-records" value="999999999999999999999999"/>
		<input type="hidden" id="default-result-total-pages" value="12"/>`)

	_, _, err := parseBackfillSearchPage(page, 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bad total-records")
}

// TestFetchBackfillSearchPagePropagatesParseError：取回成功但解析失败时，
// 错误同样原样上抛 —— 与 Fetcher 出错走的是两条不同的返回路径。
func TestFetchBackfillSearchPagePropagatesParseError(t *testing.T) {
	from := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	kw := "社会融资规模存量统计数据报告"
	u := backfillSearchURL(kw, from, to, 1)
	f := &fakeFetcher{pages: map[string][]byte{u: []byte("<html>改版了</html>")}}

	hits, totalPages, err := fetchBackfillSearchPage(context.Background(), f, kw, from, to, 1)
	require.Error(t, err)
	assert.Nil(t, hits)
	assert.Zero(t, totalPages)
}

// TestFetchBackfillSearchPageRejectsOutOfRangePublished（FIX-1，本次返工的核心）：
// 解析出的**每条** Published 都必须落在 [from, to] 内，越界即报错并指向日期过滤失效。
//
// 为什么判「性质」而不判「数量」——原守卫（总条数上界 5000）**对它自己命名的失效
// 场景实测不会触发**，我逐条复现过（qAll=社会融资规模存量统计数据报告，
// searchArea=title，只动 advtime）：
//
//	现状 advtime=5 + 日期范围 → 137 条，asc 首条 2020-01-16（区间内）
//	advtime 参数被丢弃        → 240 条，asc 首条 **2015-02-10**（区间外）
//	advtime=0                → 240 条，asc 首条 **2015-02-10**（区间外）
//	金融统计数据报告无日期过滤 → 1136 条（三关键词最大）
//
// 240 / 1136 距 5000 差 4.4 倍 ⇒ 量级判据永远不触发；而**区间外的日期立刻出现**，
// 与语料规模无关。
//
// ⚠️ 已知边界，别当它比实际强：`sr=dateTime desc` 下第 1 页是最新的，全在区间内
// ⇒ 本守卫在翻到**含区间外条目的那一页**时才触发（实测那些是靠后的页）。TASK-005
// 会翻完 totalPages，所以一定会触发；但**只取第 1 页的调用方挡不住**。
func TestFetchBackfillSearchPageRejectsOutOfRangePublished(t *testing.T) {
	from := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	kw := "社会融资规模存量统计数据报告"

	// 第 20 页（末页，240−19×12=12 条满页）：advtime 失效后多出来的正是 2020 年
	// 以前的报告，这里放 11 条区间内 + 1 条 2015 年的。
	page := searchPageHTML(240, 20, append(searchItems(11),
		searchItemHTML(searchSampleKeptURL, "2015年1月社会融资规模存量统计数据报告", "2015年02月10日"))...)
	u := backfillSearchURL(kw, from, to, 20)
	f := &fakeFetcher{pages: map[string][]byte{u: page}}

	_, _, err := fetchBackfillSearchPage(context.Background(), f, kw, from, to, 20)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "2015-02-10", "错误要指出越界的那条日期")
	assert.Contains(t, err.Error(), "advtime", "错误要指向日期过滤（advtime）失效")
}

// TestFetchBackfillSearchPageAcceptsBoundaryDates 是上一条的**反向**用例：
// 区间是**闭区间**，恰好等于 from / to 的两条必须放行。
// 没有它，把判据写成开区间（`<=` 写成 `<`）测试照样绿，而那会让每次回填的首末两天
// 变成假报错 —— 假报错走 fail-open 会静默跳过整条交叉校验。
func TestFetchBackfillSearchPageAcceptsBoundaryDates(t *testing.T) {
	from := time.Date(2020, 1, 16, 0, 0, 0, 0, time.UTC)
	to := time.Date(2025, 9, 12, 0, 0, 0, 0, time.UTC)
	kw := "社会融资规模存量统计数据报告"

	page := searchPageHTML(2, 1,
		searchItemHTML(searchSampleKeptURL, "2025年8月社会融资规模存量统计数据报告", "2025年09月12日"),
		searchItemHTML(searchSampleDigitDropURL, "2019年12月社会融资规模存量统计数据报告", "2020年01月16日"),
	)
	u := backfillSearchURL(kw, from, to, 1)
	f := &fakeFetcher{pages: map[string][]byte{u: page}}

	hits, _, err := fetchBackfillSearchPage(context.Background(), f, kw, from, to, 1)
	require.NoError(t, err, "端点日期是闭区间，必须放行")
	assert.Len(t, hits, 1) // 另一条是调统司栏目，被前缀筛掉
}

// TestFetchBackfillSearchPageChecksAllColumnsDates：区间校验发生在**栏目筛之前**。
// 日期过滤失效对两个栏目一视同仁，只查保留的那半会让证据少一半；更要紧的是
// 若某页恰好全是调统司栏目，筛后为空 ⇒ 区间守卫在空集上平凡通过。
func TestFetchBackfillSearchPageChecksAllColumnsDates(t *testing.T) {
	from := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	kw := "社会融资规模存量统计数据报告"

	// 唯一越界的那条在**会被筛掉**的栏目里。
	page := searchPageHTML(1, 1,
		searchItemHTML(searchSampleHexURL, "2015年1月社会融资规模存量统计数据报告", "2015年02月10日"))
	u := backfillSearchURL(kw, from, to, 1)
	f := &fakeFetcher{pages: map[string][]byte{u: page}}

	_, _, err := fetchBackfillSearchPage(context.Background(), f, kw, from, to, 1)
	require.Error(t, err, "越界条目在被筛掉的栏目里，同样要报错")
}

// TestFetchBackfillSearchPagePropagatesFetchError：Fetcher 的错误原样上抛。
//
// 本层**不吞不降级**：跳过交叉校验、打 WARN、主路径照常完成是 TASK-005 调用方的
// 职责（ADR-M1c1-02）。在这一层降级 = 调用方永远不知道校验没做过。
func TestFetchBackfillSearchPagePropagatesFetchError(t *testing.T) {
	f := &fakeFetcher{err: errBoom}

	_, _, err := fetchBackfillSearchPage(context.Background(), f,
		"社会融资规模存量统计数据报告",
		time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC), 1)

	require.Error(t, err)
	assert.ErrorIs(t, err, errBoom)
}

// TestFetchBackfillSearchPageFetchesConstructedURL：取回的是 backfillSearchURL
// 构造出的那个 URL —— fakeFetcher 只认精确匹配的 key，URL 差一个字节就取不到。
func TestFetchBackfillSearchPageFetchesConstructedURL(t *testing.T) {
	from := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	kw := "社会融资规模存量统计数据报告"
	u := backfillSearchURL(kw, from, to, 1)
	f := &fakeFetcher{pages: map[string][]byte{u: readTestdata(t, "pboc-search-p1.html")}}

	hits, totalPages, err := fetchBackfillSearchPage(context.Background(), f, kw, from, to, 1)
	require.NoError(t, err)

	assert.Equal(t, []string{u}, f.calls)
	assert.Equal(t, searchSampleTotalPages, totalPages)
	// 入口返回的是**筛后**的结果 —— 调用方拿到的直接就是可比对的那一份。
	assert.Len(t, hits, searchSampleKept)
}

// TestFetchBackfillSearchPageDateGuardIsNotVacuousOnRealSample：真实样本页 1 的
// **每一条**日期都必须是有效日期且落在区间内。
//
// 这是 checkBackfillSearchDateRange 的**非空验证**。上面那条用例只断言真实样本
// 「不报错」，而「不报错」有两种成因分不开：日期确实都在区间内（想要的），或者
// 日期解析坏掉返回了空串/无效值而比较恰好不落进越界分支（不想要的）。
//
// ⚠️ 另一半同样重要，写在这里免得被误读：**真实样本页 1 上这条守卫本来就不会
// 触发**。`sr=dateTime desc` ⇒ 越界的老条目排在靠后的页。我实测了 advtime 失效
// 状态（去掉 advtime、240 条）下各页的越界情况：
//
//	pNo=1  2025-09-12..2025-04-13  越界 0 条
//	pNo=11 2020-09-11..2020-03-11  越界 0 条
//	pNo=12 2020-03-11..2019-09-11  越界 **7 条** ← 守卫在此才触发
//	pNo=13 2019-09-11..2019-03-10  越界 12 条
//
// ⇒ 「守卫会报错」这件事只能由**合成页**用例证明
// （TestFetchBackfillSearchPageRejectsOutOfRangePublished），本条证的是另一半：
// 真实语料喂进去时它走的是「逐条比较且都通过」，不是「没有东西可比」。
func TestFetchBackfillSearchPageDateGuardIsNotVacuousOnRealSample(t *testing.T) {
	hits, _, err := parseBackfillSearchPage(readTestdata(t, "pboc-search-p1.html"), 1)
	require.NoError(t, err)
	require.Len(t, hits, searchSampleItems, "非空前提：样本页 1 必须有 12 条可比")

	const lo, hi = "2020-01-01", "2026-08-14"
	for _, h := range hits {
		// 逐条钉「是个合法日期」而不只是「非空」：空串与 "0000-00-00" 都能悄悄
		// 通过字典序比较的下界，而它们不是日期。
		_, perr := time.Parse("2006-01-02", h.Published)
		require.NoError(t, perr, "日期解析坏了: %q（%s）", h.Published, h.Title)
		assert.GreaterOrEqual(t, h.Published, lo, "%s", h.Title)
		assert.LessOrEqual(t, h.Published, hi, "%s", h.Title)
	}
}

func countTitle(hits []backfillSearchHit, title string) int {
	n := 0
	for _, h := range hits {
		if h.Title == title {
			n++
		}
	}
	return n
}

func containsURL(hits []backfillSearchHit, u string) bool {
	for _, h := range hits {
		if h.URL == u {
			return true
		}
	}
	return false
}

// searchItemHTML 合成一条搜索结果，结构照抄样本（含 <p class="dates"> 里那个
// 带 class 的 <span class="date_">，它**不该**被日期正则误吃）。
func searchItemHTML(url, title, date string) string {
	return fmt.Sprintf(`<h3>
		<a href="%s" target="_blank" key="k" appId="a">%s</a>
	</h3>
	<div class="content clearfix">
		<p class="txtCon hasImg">摘要</p>
		<p class="dates">
			<a href="%s"><span class="date_"> %s</span></a>
			<span>%s</span>
			<i></i>
		</p>
	</div>`, url, title, url, url, date)
}

// searchItemHTMLNoDate 合成一条**缺日期**的结果 —— test-agent-28 复现的那个形态。
// 非贪婪的结果正则会跨过它去吃下一条的日期，两条静默变一条。
func searchItemHTMLNoDate(url, title string) string {
	return fmt.Sprintf(`<h3>
		<a href="%s" target="_blank" key="k" appId="a">%s</a>
	</h3>
	<div class="content clearfix">
		<p class="txtCon hasImg">摘要</p>
		<p class="dates">
			<a href="%s"><span class="date_"> %s</span></a>
			<i></i>
		</p>
	</div>`, url, title, url, url)
}

// searchItems 造 n 条互不相同、日期都在区间内的正常结果。
func searchItems(n int) []string {
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, searchItemHTML(
			fmt.Sprintf("https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/%d/index.html", 5837468+i),
			fmt.Sprintf("2025年%d月社会融资规模存量统计数据报告", i%12+1),
			"2025年09月12日"))
	}
	return out
}

// searchPageHTML 合成一页搜索结果：计数字段 + 若干条目（可为零条）。
func searchPageHTML(records, pages int, items ...string) []byte {
	return []byte(strings.Join(items, "\n") + fmt.Sprintf(`
	<div class="default-result-list-paging">
		<input type="hidden" id="default-result-total-records" value="%d"/>
		<input type="hidden" id="default-result-total-pages" value="%d"/>
		<input type="hidden" id="default-result-total-curPageNo" value="1"/>
	</div>`, records, pages))
}
