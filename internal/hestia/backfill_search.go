package hestia

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// 搜索侧扫描（M1c-1 的 TASK-004，design-spec §4）。
//
// 这是 index 翻页之外的**第二来源**，只做交叉校验、不做主路径（ADR-M1c1-01）：
// 站内检索确实更精确（三个关键词全区间共 82 页，对比 index 的约 150 页），但实测
// 它自己有一处不自洽 —— 2020 全年搜「社会融资规模存量统计数据报告」= 24 条 = 12 期
// × 2 栏目、完美齐全，而全区间同一关键词只有 137 条，理论值 79 个月 × 2 栏目 = 158
// 条，**差 21 条**。搜索漏了搜索自己不会说，所以它做校验、不做唯一依据。
//
// ⚠️ 本文件**只解析、不决定降级**。HTTP 非 200、条数骤增、解析不到计数字段一律
// 原样返回错误；「打 WARN、跳过交叉校验、主路径照常完成」是调用方的职责
// （ADR-M1c1-02 的 fail-open）。在这一层降级 = 调用方永远不知道校验没做过。
const (
	backfillSearchBase   = "https://wzdig.pbc.gov.cn/search/pcRender"
	backfillSearchPageID = "c177a85bd02b4114bebebd210809f691"

	// backfillSearchKeepPrefix 是唯一保留的栏目前缀。
	//
	// 实测同一篇报告在两个栏目各有一份，样本 12 条 = 6 对：
	//
	//	/goutongjiaoliu/113456/113469/5837468/index.html                           ← 保留
	//	/diaochatongjisi/116219/116225/35ec0aa27604417888826e7ff128cc4a/index.html ← 丢弃
	//
	// 丢弃的理由是**口径**：交叉校验按 article_id 比对两侧（design-spec §5），而调查
	// 统计司那份的 id 在 index 侧（沟通交流栏目）根本不存在 ⇒ 不筛掉的话，差集里会
	// 凭空多出一半的假信号。
	//
	// ⚠️ 判据必须是**栏目前缀**，不能是「article_id 长得像 32 位 hex」。样本里 6 条
	// diaochatongjisi 只有 2 条是 32 位 hex，另 4 条是 19 位纯数字
	// （`2025080618505078072`）—— 与 goutongjiaoliu 侧的 id 形态完全撞形。
	// 这句话由 TestFilterBackfillSearchHitsDropsByPrefixNotByIDShape 钉住。
	backfillSearchKeepPrefix = "/goutongjiaoliu/113456/113469/"

	// backfillSearchMaxRecords 是总条数的上界，超过即判 `advtime` 过滤失效。
	//
	// 为什么需要上界：`advtime=5`（自定义时间范围）**是前端 radio 里已被注释掉、
	// 后端仍接受的未公开参数**（2026-08-14 实测有效）。未公开行为会在无预告的情况下
	// 变，而它失效时返回的是**全站量级**（实测空查询 549141 条）—— 那种结果看起来
	// 完全正常，只是多得多，不设上界就只会静默地把几万条无关结果当成候选。
	//
	// 取值 5000 的依据（两侧都留够余量）：
	//   - 下侧：实测单关键词全区间最大 692 条（`金融统计数据报告`）。三个关键词每月
	//     各新增 2～3 条，再跑十年也不到 1100 条 ⇒ 5000 不会误伤正常增长。
	//   - 上侧：失效时的量级是 549141，与 5000 差 100 倍 ⇒ 不会漏判。
	// 两端各有一条用例钉住（Rejects·SiteWideTotal / Accepts·LargestMeasuredTotal）。
	backfillSearchMaxRecords = 5000
)

// backfillSearchKeywords 是三个 AND 关键词，调用方各查一遍。
//
// 用 `qAll`（AND 语义）而不是 `q`（分词 OR）：实测同一关键词、同为
// `searchArea=title`，`q`=1324 条 / `qAll`=24 条，**差 25 倍**。
// `advepq` / `adveq` 单用等于空查询（返回全站 549141 条），别用。
var backfillSearchKeywords = []string{
	"金融统计数据报告",
	"社会融资规模存量统计数据报告",
	"社会融资规模增量统计数据报告",
}

// backfillSearchItemRE 一条搜索结果：绝对 URL / 标题（含 <font> 高亮）/ 发布日期。
//
// 日期那一段写死成不带属性的 `<span>`：同一个 `<p class="dates">` 里还有一个
// `<span class="date_">`（内容是 URL 本身），带属性 ⇒ 匹配不上，正好把它排除。
// `.*?` 全部非贪婪，所以每条结果只吃到**紧随其后的**那个日期。
var backfillSearchItemRE = regexp.MustCompile(
	`(?s)<h3>\s*<a href="([^"]+)"[^>]*>(.*?)</a>.*?<span>(\d{4})年(\d{2})月(\d{2})日</span>`)

// 计数字段（隐藏 input）。两个都必须解析得到：
// 总页数供调用方决定翻到第几页，总条数供上面那条 advtime 守卫。
var (
	backfillSearchTotalPagesRE   = regexp.MustCompile(`id="default-result-total-pages" value="(\d+)"`)
	backfillSearchTotalRecordsRE = regexp.MustCompile(`id="default-result-total-records" value="(\d+)"`)
)

// backfillSearchIDRE 从结果 URL 末段取 article_id。
// 与 articleLinkRE 同则**不设位数下界、不限字符集**：站点重建过一次
// （19 位 → 7 位），任何形态假设都是对下一次重建的猜测。
var backfillSearchIDRE = regexp.MustCompile(`/([^/]+)/index\.html$`)

// backfillSearchHit 是搜索结果里的一条报告。
//
// 三样字段直接就是 manifest 需要的（URL / 标题 / 发布日期），不必再解析文章页；
// ArticleID 是交叉校验的键 —— 筛掉调查统计司那份之后，两侧的 article_id 同口径。
type backfillSearchHit struct {
	ArticleID string
	URL       string
	Title     string
	Published string // YYYY-MM-DD
}

// backfillSearchURL 构造第 page 页的查询 URL。
//
// 参数全部来自实测（requirements-analysis §4），别凭 API 直觉改：
//   - `searchArea=title` 只搜标题（默认搜正文，同一关键词 1324 条 vs 610 条）
//   - `sr=dateTime desc` 按发布时间倒序 —— 值非法时服务端返回 **302**（实测
//     `sr=garbage` 即 302），不是忽略该参数
//   - `advtime=5` + `startTime` / `endTime` 是自定义时间范围，见上面的守卫注释
//
// ⚠️ url.Values.Encode 把 `sr` 的空格编码成 `+`（而非 `%20`），**实测两者等价**：
// 同一查询下 `dateTime+desc` 与 `dateTime%20desc` 的总条数（137）与首条日期
// （2025-09-12）逐字相同；换成 `asc` 时两者又同步变成 2020-01-16 ⇒ 参数确实生效，
// 不是被忽略。别为了「看起来像实测那条 URL」把它改回手工拼接。
func backfillSearchURL(keyword string, from, to time.Time, page int) string {
	q := url.Values{}
	q.Set("pageId", backfillSearchPageID)
	q.Set("advSearch", "true")
	q.Set("searchArea", "title")
	q.Set("sr", "dateTime desc")
	q.Set("qAll", keyword)
	q.Set("advtime", "5")
	q.Set("startTime", from.Format("2006-01-02"))
	q.Set("endTime", to.Format("2006-01-02"))
	q.Set("pNo", strconv.Itoa(page))
	return backfillSearchBase + "?" + q.Encode()
}

// parseBackfillSearchPage 解析一页搜索结果，返回**未经栏目筛**的条目与总页数。
//
// 不在这里筛的理由是「0 条 ⇒ 报错」那条守卫的判据：它要判的是**解析失效**
// （检索服务改版），而不是「这一页没有我要的栏目」。某一页 12 条恰好全是调查统计司
// 栏目是完全可能的，那时报错会把整条交叉校验打断。两者由两条用例分开钉住。
func parseBackfillSearchPage(html []byte) ([]backfillSearchHit, int, error) {
	records, err := backfillSearchCount(html, backfillSearchTotalRecordsRE, "total-records")
	if err != nil {
		return nil, 0, err
	}
	totalPages, err := backfillSearchCount(html, backfillSearchTotalPagesRE, "total-pages")
	if err != nil {
		return nil, 0, err
	}
	if records > backfillSearchMaxRecords {
		return nil, 0, fmt.Errorf("hestia backfill search: %d results exceed the %d ceiling: "+
			"the advtime date filter likely stopped working (an empty query returns the whole site)",
			records, backfillSearchMaxRecords)
	}

	ms := backfillSearchItemRE.FindAllSubmatch(html, -1)
	if len(ms) == 0 {
		return nil, 0, fmt.Errorf("hestia backfill search: 0 results parsed while the page claims %d: "+
			"the result layout likely changed", records)
	}

	hits := make([]backfillSearchHit, 0, len(ms))
	for _, m := range ms {
		u := string(m[1])
		id := backfillSearchArticleID(u)
		// 要保留的那个栏目里，article_id 取不出来 ⇒ 报错。
		// 它是交叉校验的**键**（design-spec §5）：留着空 id 往下走，TASK-005 会把
		// 若干条都归到 "" 这一个键上，差集凭空多出/少掉几条而没有任何错误。
		// 别的栏目取不出无所谓 —— 那些条目下一步就被筛掉了。
		if id == "" && strings.Contains(u, backfillSearchKeepPrefix) {
			return nil, 0, fmt.Errorf("hestia backfill search: no article id in %q: "+
				"the result url layout likely changed", u)
		}
		hits = append(hits, backfillSearchHit{
			ArticleID: id,
			URL:       u,
			// 标题里嵌 <font color='#FF0000'> 高亮（样本 12 条各被拆成 5 段），
			// 用包内既有的 tagRE 剥 —— 不留标签给下游按字符串比对标题。
			Title:     strings.TrimSpace(tagRE.ReplaceAllString(string(m[2]), "")),
			Published: fmt.Sprintf("%s-%s-%s", m[3], m[4], m[5]),
		})
	}
	return hits, totalPages, nil
}

// filterBackfillSearchHits 只保留沟通交流栏目那一份，见 backfillSearchKeepPrefix。
func filterBackfillSearchHits(hits []backfillSearchHit) []backfillSearchHit {
	var out []backfillSearchHit
	for _, h := range hits {
		if strings.Contains(h.URL, backfillSearchKeepPrefix) {
			out = append(out, h)
		}
	}
	return out
}

// fetchBackfillSearchPage 取回并解析第 page 页，返回**筛后**的条目与总页数。
//
// Fetcher 的错误（含 HTTP 非 200）原样上抛 —— 本层不吞不降级，见文件头注释。
// 翻页由调用方按 totalPages 决定：限速与逐页落盘都在抓取层（design-spec §7）。
func fetchBackfillSearchPage(ctx context.Context, f Fetcher, keyword string, from, to time.Time, page int) ([]backfillSearchHit, int, error) {
	html, err := f.Get(ctx, backfillSearchURL(keyword, from, to, page))
	if err != nil {
		return nil, 0, err
	}
	hits, totalPages, err := parseBackfillSearchPage(html)
	if err != nil {
		return nil, 0, err
	}
	return filterBackfillSearchHits(hits), totalPages, nil
}

// backfillSearchCount 读一个隐藏 input 里的计数字段。
//
// 解析不到 ⇒ 报错，与 parsePaging 同则：静默退化成「只看第 1 页」时，调用方永远
// 发现不了还有 11 页，而搜索侧的全部价值就在于它索到了 index 没翻到的那些。
func backfillSearchCount(html []byte, re *regexp.Regexp, field string) (int, error) {
	m := re.FindSubmatch(html)
	if m == nil {
		return 0, fmt.Errorf("hestia backfill search: no %s field found: "+
			"the search result layout likely changed", field)
	}
	n, err := strconv.Atoi(string(m[1]))
	if err != nil || n < 0 {
		return 0, fmt.Errorf("hestia backfill search: bad %s %q", field, m[1])
	}
	return n, nil
}

// backfillSearchArticleID 取 URL 末段的 article_id；形态不符时返回空串
// （由栏目前缀筛决定这条要不要留，不在这里判）。
func backfillSearchArticleID(u string) string {
	if m := backfillSearchIDRE.FindStringSubmatch(u); m != nil {
		return m[1]
	}
	return ""
}
