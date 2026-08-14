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
//	boundary[1]       "总条数骤增到全站量级 ⇒ 报错，指向 advtime 失效"
//	                  → TestParseBackfillSearchPageRejectsSiteWideTotal
//	                  → TestParseBackfillSearchPageAcceptsLargestMeasuredTotal（反向：实测最大值不报错）
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
	hits, _, err := parseBackfillSearchPage(readTestdata(t, "pboc-search-p1.html"))
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
	hits, _, err := parseBackfillSearchPage(readTestdata(t, "pboc-search-p1.html"))
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
	hits, _, err := parseBackfillSearchPage(readTestdata(t, "pboc-search-p1.html"))
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
	_, totalPages, err := parseBackfillSearchPage(readTestdata(t, "pboc-search-p1.html"))
	require.NoError(t, err)
	assert.Equal(t, searchSampleTotalPages, totalPages)
}

// TestParseBackfillSearchPageRejectsMissingTotals：解析不到计数字段 ⇒ 报错。
// 与 parsePaging 同则 —— 静默退化成「只看第 1 页」时，调用方永远发现不了还有 11 页。
func TestParseBackfillSearchPageRejectsMissingTotals(t *testing.T) {
	for _, tc := range []struct{ name, page string }{
		{"没有 total-pages", `<input id="default-result-total-records" value="137"/>` + searchItemHTML(searchSampleKeptURL, "标题", "2025年09月12日")},
		{"没有 total-records", `<input id="default-result-total-pages" value="12"/>` + searchItemHTML(searchSampleKeptURL, "标题", "2025年09月12日")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := parseBackfillSearchPage([]byte(tc.page))
			require.Error(t, err)
		})
	}
}

// TestParseBackfillSearchPageRejectsZeroResults：一条都解析不到 ⇒ 报错。
// 计数字段俱全（137 条 / 12 页）却匹配不到任何结果，只可能是结果结构改版。
func TestParseBackfillSearchPageRejectsZeroResults(t *testing.T) {
	_, _, err := parseBackfillSearchPage(searchPageHTML(137, 12))
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
	page := searchPageHTML(137, 12,
		searchItemHTML(searchSampleHexURL, "2025年8月社会融资规模存量统计数据报告", "2025年09月12日"),
		searchItemHTML(searchSampleDigitDropURL, "2025年6月社会融资规模存量统计数据报告", "2025年07月14日"),
	)
	hits, _, err := parseBackfillSearchPage(page)
	require.NoError(t, err)
	require.Len(t, hits, 2)
	assert.Empty(t, filterBackfillSearchHits(hits))
}

// TestParseBackfillSearchPageRejectsSiteWideTotal：条数骤增到全站量级 ⇒ 报错。
//
// 549141 是实测的空查询返回值（advepq/adveq 单用）。`advtime=5` 是前端已注释掉、
// 后端仍接受的未公开参数，它哪天失效时返回的正是这个量级，而**那种结果看起来完全
// 正常，只是多得多**。
func TestParseBackfillSearchPageRejectsSiteWideTotal(t *testing.T) {
	page := searchPageHTML(549141, 45762,
		searchItemHTML(searchSampleKeptURL, "随便什么标题", "2025年09月12日"))

	_, _, err := parseBackfillSearchPage(page)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "advtime", "错误信息必须指向 advtime 过滤失效")
	assert.Contains(t, err.Error(), "549141", "错误信息必须带上实际条数")
}

// TestParseBackfillSearchPageAcceptsLargestMeasuredTotal 是上一条的**反向**用例：
// 实测单关键词全区间最大值（`金融统计数据报告` = 692 条）必须放行。
// 没有它，把阈值设成 1 也「测试全绿」，而那会让搜索侧一次都跑不起来。
func TestParseBackfillSearchPageAcceptsLargestMeasuredTotal(t *testing.T) {
	page := searchPageHTML(692, 58,
		searchItemHTML(searchSampleKeptURL, "2025年8月金融统计数据报告", "2025年09月12日"))

	hits, totalPages, err := parseBackfillSearchPage(page)
	require.NoError(t, err)
	assert.Equal(t, 58, totalPages)
	assert.Len(t, hits, 1)
}

// TestParseBackfillSearchPageRejectsIDlessKeptURL：要保留的栏目里取不出
// article_id ⇒ 报错。它是交叉校验的键，空 id 会让 TASK-005 把若干条归到同一个 ""
// 键上 —— 差集算错而没有任何错误。别的栏目取不出无所谓（下一步就筛掉了），
// 这条用例把两者都跑一遍。
func TestParseBackfillSearchPageRejectsIDlessKeptURL(t *testing.T) {
	t.Run("保留栏目取不出 id ⇒ 报错", func(t *testing.T) {
		page := searchPageHTML(137, 12, searchItemHTML(
			"https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/5837468/index.shtml",
			"2025年8月社会融资规模存量统计数据报告", "2025年09月12日"))
		_, _, err := parseBackfillSearchPage(page)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no article id")
	})

	t.Run("其他栏目取不出 id ⇒ 不报错，随后被筛掉", func(t *testing.T) {
		page := searchPageHTML(137, 12,
			searchItemHTML("https://www.pbc.gov.cn/diaochatongjisi/116219/116225/whatever",
				"2025年8月社会融资规模存量统计数据报告", "2025年09月12日"),
			searchItemHTML(searchSampleKeptURL, searchSampleDupTitle, "2025年09月12日"))
		hits, _, err := parseBackfillSearchPage(page)
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

	_, _, err := parseBackfillSearchPage(page)
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

// searchPageHTML 合成一页搜索结果：计数字段 + 若干条目（可为零条）。
func searchPageHTML(records, pages int, items ...string) []byte {
	return []byte(strings.Join(items, "\n") + fmt.Sprintf(`
	<div class="default-result-list-paging">
		<input type="hidden" id="default-result-total-records" value="%d"/>
		<input type="hidden" id="default-result-total-pages" value="%d"/>
		<input type="hidden" id="default-result-total-curPageNo" value="1"/>
	</div>`, records, pages))
}
