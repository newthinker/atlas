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
// ⚠️ 本文件**只解析、不决定降级**。HTTP 非 200、日期落在区间外、本页条数不自洽、
// 解析不到计数字段一律原样返回错误；「打 WARN、跳过交叉校验、主路径照常完成」是调用方的职责
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

	// 🔴 **这里曾经有一条 `backfillSearchMaxRecords = 5000` 的总条数上界，已删**
	// （M1c-1 返工，人类 leader 2026-08-14 裁决）。别再加回来 —— 三条理由：
	//
	//  1. **没有任何已知触发场景**。它名义上认「advtime 失效」，而实测（同关键词、同
	//     searchArea=title，只动 advtime）：advtime=5 + 日期范围 → 137 条；advtime 被
	//     丢弃 → 240 条；advtime=0 → 240 条；`金融统计数据报告` 无日期过滤 → 1136 条。
	//     全都远低于 5000。另一个名义场景「空查询返回全站 549141」实测返回的是 HTTP
	//     200 + 16KB 空壳、`default-result-*` 计数字段整个不存在 ⇒ 由 backfillSearchCount
	//     的「计数字段缺失 ⇒ 报错」拦下，同样轮不到它。
	//  2. **它是有害的**：留着就隐含一条「上界必须 > 1136」的约束，而站点内容每月在长；
	//     撞上那天它会**抢在日期守卫之前**报出一个**指错方向**的错误。会抢先报错且方向
	//     错的守卫，比没有守卫糟。
	//  3. **它占着「量级守卫」这个位置**，让后来者以为量级异常已经防住了。
	//
	// ⇒ 认「日期过滤失效」的是 checkBackfillSearchDateRange，判的是**性质**（每条日期
	// 是否落在 [from,to] 内），因此**不依赖任何实测量级、不会随语料增长而失效**。
	// 任何量级阈值都会过期 —— 上面那组 240/1136 本身也只是 2026-08-14 的一次采样。

	// backfillSearchPageSize 是每页条数（实测 2026-08-14，requirements-analysis §4
	// 亦记「每页固定 12 条」）：qAll=社会融资规模存量统计数据报告 / 137 条 / 12 页，
	// pNo=2 与 pNo=11 各 12 条，pNo=12 恰好 5 条 = 137 − 11×12。
	//
	// ⚠️ 站点若改了每页条数，本常量会让**每一页**都判不自洽 ⇒ 调用方按 ADR-M1c1-02
	// fail-open（打 WARN、跳过交叉校验、主路径照常）。这是刻意选的方向：交叉校验停摆
	// 有声，静默丢条无声。
	backfillSearchPageSize = 12
)

// backfillSearchKeywords 是三个 AND 关键词，调用方各查一遍。
//
// 用 `qAll`（AND 语义）而不是 `q`（分词 OR）：**严格单变量对照**实测，qAll 严格窄于 q——
//
//	searchArea=title + 日期窗口：qAll=137 vs q=276  → 2.0×
//	searchArea=title 无日期窗口：qAll=240 vs q=610  → 2.5×
//
// ⚠️ 原注释写的「1324 vs 24 = 25 倍」是**同时动了三个变量**的对照（默认含正文 /
// q vs qAll / 有无日期窗口），不能用来说明 q 与 qAll 的差别（M1c-1 返工 FIX-3）。
// 1324 是「默认含正文 + q + 无日期」那一格的数字。
// `advepq` / `adveq` 单用等于空查询，别用。
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

// 计数字段（隐藏 input）。两个都必须解析得到：总页数供调用方决定翻到第几页，
// 总条数做本页条数自洽校验的**独立锚**（不与 item 正则共享形态假设）。
var (
	backfillSearchTotalPagesRE   = regexp.MustCompile(`id="default-result-total-pages" value="(\d+)"`)
	backfillSearchTotalRecordsRE = regexp.MustCompile(`id="default-result-total-records" value="(\d+)"`)
)

// backfillSearchBlockRE 只匹配「一个结果块的开头」，用来独立数出本页有几条结果。
//
// 它比 backfillSearchItemRE 短得多，**刻意不含日期段** —— 这正是它的用处：某条目
// 缺日期时，非贪婪的 item 正则会跨过它去吃下一条的日期，两条静默变一条，而块计数
// 仍然是 2 ⇒ 不一致暴露出来（M1c-1 返工 FIX-2）。反过来用日期段计数就看不见这件事：
// 那时日期数与匹配数会**一起**变成 1，「自洽」。
var backfillSearchBlockRE = regexp.MustCompile(`(?s)<h3>\s*<a href="`)

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
//   - `advtime=5` + `startTime` / `endTime` 是自定义时间范围；它失效时由
//     checkBackfillSearchDateRange 认出来（判性质，不判量级），见那里的注释
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

// parseBackfillSearchPage 解析第 page 页搜索结果，返回**未经栏目筛**的条目与总页数。
//
// 不在这里筛的理由是「0 条 ⇒ 报错」那条守卫的判据：它要判的是**解析失效**
// （检索服务改版），而不是「这一页没有我要的栏目」。某一页 12 条恰好全是调查统计司
// 栏目是完全可能的，那时报错会把整条交叉校验打断。两者由两条用例分开钉住。
//
// 收 page 是为了 backfillSearchCheckCount 的末页算术（M1c-1 返工 FIX-2）：
// 「本页该有几条」只有知道页码才回答得出。
func parseBackfillSearchPage(html []byte, page int) ([]backfillSearchHit, int, error) {
	records, err := backfillSearchCount(html, backfillSearchTotalRecordsRE, "total-records")
	if err != nil {
		return nil, 0, err
	}
	totalPages, err := backfillSearchCount(html, backfillSearchTotalPagesRE, "total-pages")
	if err != nil {
		return nil, 0, err
	}
	ms := backfillSearchItemRE.FindAllSubmatch(html, -1)
	if len(ms) == 0 {
		return nil, 0, fmt.Errorf("hestia backfill search: 0 results parsed while the page claims %d: "+
			"the result layout likely changed", records)
	}
	if err := backfillSearchCheckCount(html, len(ms), records, page); err != nil {
		return nil, 0, err
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

// backfillSearchCheckCount 判本页条数是否自洽（M1c-1 返工 FIX-2）。
//
// 两条检查认的是**同一种失效**（本页少了条），但**依赖路径不同** —— 这才是两条都留着
// 的理由（人类 leader 2026-08-14 裁决）。**别把它们说成「两道独立防线」**：我消融实测
// 过，构造不出「只有第 1 条能发现」的输入。
//
//  1. 与页内结构标记数（`<h3><a href=`）比。认「正则错配丢条」：某条目缺日期时非贪婪
//     的 item 正则会跨过它去吃下一条的日期，2 条静默变 1 条，留下的那条是「A 的
//     URL/标题 + B 的日期」。
//     **它不依赖 backfillSearchPageSize 这个常量** ← 承重点在这里，但成立的方式与
//     直觉不同，我实测过（站点每页 12→15、records=30/pages=2、第 1 页 15 条）：
//
//     A. 页完好：第 2 条误报「15 items but expected 12 … results are being dropped」
//     B. 页里真有一条缺日期：**这一条**先报「parsed 14 items from 15 result blocks:
//     an item likely lacks its date」
//
//     ⇒ 常量过期时**两种情况都会报错**，没有「静默失效」这回事；真正的价值是 B 仍然
//     **可与 A 区分**。删掉这一条，B 会退化成 A 那句关于页容量的话 ⇒ 一个**真实**的
//     丢条会被误判成「站点改了每页条数」这种已知无害的噪音而被打发掉。
//     防的是**误归因**，不是「多一道检测」。
//
//  2. 与**站点自报的** total-records + 页容量比。**日常的检测层**：任何原因导致的少条
//     （错配、整块缺失、响应截断）都会在这里现形，且它锚在站点自己的数字上，不与
//     item 正则共享形态假设。代价是依赖上面那个常量。
//
// 末页算术：前 page-1 页各占满 backfillSearchPageSize 条，本页应有
// records-(page-1)*size 与 size 中的较小者（实测 137 条 / 12 页：pNo=12 恰好 5 条）。
func backfillSearchCheckCount(html []byte, got, records, page int) error {
	if blocks := len(backfillSearchBlockRE.FindAllIndex(html, -1)); blocks != got {
		return fmt.Errorf("hestia backfill search: parsed %d items from %d result blocks: "+
			"an item likely lacks its date and the item regex mis-associated it with the next one",
			got, blocks)
	}

	want := records - (page-1)*backfillSearchPageSize
	if want > backfillSearchPageSize {
		want = backfillSearchPageSize
	}
	if got != want {
		return fmt.Errorf("hestia backfill search: page %d yielded %d items but the site reports "+
			"%d records over pages of %d (expected %d): results are being dropped",
			page, got, records, backfillSearchPageSize, want)
	}
	return nil
}

// checkBackfillSearchDateRange 断言每条 Published 都落在 [from, to] **闭区间**内。
//
// 🔴 这是本层认「日期过滤失效」的**唯一**判据（M1c-1 返工 FIX-1）。原先那条按总条数
// 判量级的守卫对该失效**实测不会触发**：advtime 被丢弃或置 0 时返回 240 条、无日期
// 过滤的 `金融统计数据报告` 返回 1136 条，都远低于 5000 的上界，而返回内容里立刻就有
// 区间外的条目（asc 首条 2015-02-10，实测 2026-08-14）。
//
// 判**性质**而不判**数量**，因此与语料规模无关：站点条目再怎么增长，区间外就是区间外。
//
// ⚠️ 已知边界，别高估它：`sr=dateTime desc` 下第 1 页是最新的、必然在区间内 ⇒ 本守卫
// 在翻到含区间外条目的那一页（实测是靠后的页）才触发。翻完全部页的调用方一定会撞上；
// **只取第 1 页的调用方挡不住**。
//
// 日期用字符串比较：两端都是 `YYYY-MM-DD` 定长格式，字典序即时间序，省掉一次可能
// 失败的解析（而解析失败又要决定怎么处置）。
func checkBackfillSearchDateRange(hits []backfillSearchHit, from, to time.Time) error {
	lo, hi := from.Format("2006-01-02"), to.Format("2006-01-02")
	for _, h := range hits {
		if h.Published < lo || h.Published > hi {
			return fmt.Errorf("hestia backfill search: result published %s falls outside [%s, %s]: "+
				"the advtime date filter likely stopped working (%q)", h.Published, lo, hi, h.Title)
		}
	}
	return nil
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
	hits, totalPages, err := parseBackfillSearchPage(html, page)
	if err != nil {
		return nil, 0, err
	}
	// 区间校验放在**栏目筛之前**：日期过滤失效对两个栏目一视同仁，只查保留的那半会
	// 少一半证据；更要紧的是某页若恰好全是调统司栏目，筛后为空 ⇒ 守卫在空集上平凡通过。
	if err := checkBackfillSearchDateRange(hits, from, to); err != nil {
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
