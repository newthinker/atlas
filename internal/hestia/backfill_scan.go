package hestia

import (
	"bytes"
	"fmt"
	"regexp"
)

// backfillTitleRE 匹配本迭代要回填的三种报告标题。
//
// 期次段与既有 reportTitleRE 逐字相同（`上半年|前三季度|[一二三四五六七八九十]季度|\d{1,2}月`），
// 扩的只是**报告种类**：加社融存量 / 增量两种。期次段仍是**可选的** —— 年度报告的标题里
// 既没有「上半年」也没有「N月」（实测 `2025年金融统计数据报告`），而且实测（设计附录 §E）
// 年度期次下**三种报告都不写期次段**，社融存量并不写成「12月」。
//
// ⚠️ 期次段必须**紧跟**在报告种类之前 —— 挡住的是这几类真实存在的干扰项：
//
//   - `2019年三季度小额贷款公司统计数据报告` —— 期次段 +「统计数据报告」，只差中间那段
//   - `2020年11月厦门市金融统计数据报告` —— 省市分行 30+ 家同名报告
//   - `2020年一季度地区社会融资规模增量统计表` —— 只差「地区」二字与后缀
//
// **别放宽成「标题里含『金融统计数据报告』」**：那一条 `.*统计数据报告` 能让「三种都识别」
// 的验收满分通过，同时把小额贷款公司报告和 30+ 家省市分行的报告一起收进来。
//
// 与 reportTitleRE 一样，这里还有**第二个独立机制**：三种种类各自都以「报告」结尾，
// 挡的是 `2026年7月金融统计数据情况`、`2020年2月社会融资规模存量统计数据`（漏了「报告」）
// 这一类 —— 期次段确实紧跟，只是种类不完整。两个机制各由 TestScanBackfillPageRejectsLookalikes
// 里分组标注的用例守着，别把功劳记混。
//
// **不解析期次**（design-spec §7）：manifest 只记标题原文。parsePeriod 本迭代不调用 ——
// 「存量说 3月、增量说 一季度」那个坑留给 M1c-2 面对数据，而不是在这里猜。
var backfillTitleRE = regexp.MustCompile(
	`(\d{4})年(上半年|前三季度|[一二三四五六七八九十]季度|\d{1,2}月)?(金融统计数据报告|社会融资规模存量统计数据报告|社会融资规模增量统计数据报告)`)

// backfillItemRE 一条列表项：href / article_id / title 属性 / 发布日期。
//
// 实测结构（p146，同一行，`</a></font><span>` 之间无空白）：
//
//	<a href="/goutongjiaoliu/113456/113469/2025092212550537670/index.html" onclick="void(0)"
//	   target="_blank" title="2020年2月金融统计数据报告" istitle="true">2020年2月金融统计数据报告</a>
//	</font><span class="hui12">2020-03-11</span>
//
// # 🔴 `\stitle="` 里那个 `\s` 不是装饰
//
// design-spec §3.1 给的原式是 `href="([^"]*?/(\d+)/index\.html)"[^>]*title="([^"]*)"`。
// `[^>]*` 会一路吞到**最后一个** `title="`，而那个是 `istitle="true"` 的尾巴
// ⇒ 捕获组拿到的是字面量 `true`，p146 上 15 条**全部**如此。这不是「少匹配几条」那种
// 会被计数守卫接住的失效：条数一条不少、日期一天不差，**只有标题全错**，
// 于是 backfillTitleRE 一条都认不出、整页静默产出 0 篇报告。
//
// `\s` 让 `title=` 必须以空白开头，`istitle=` 里 `title` 前面是 `s` ⇒ 结构上落不进去。
// 配套把 `*` 改成 `*?`：贪婪版即使有 `\s` 边界也会先试最右边那个。
//
// # article_id 不设任何位数下界
//
// 🔴 Sprint 037 教训：discover.go 的 articleLinkRE 曾写 `\d{14,}`（「实测 19 位」），
// 央行 2026-06-26 重建站点后新发文章的 id 变成 7 位 ⇒ 整页命中 0 条、循环体一次都不执行、
// 一个字的错都不报。实测 p146 = 19 位、p18 = 7 位，且**页内会混合**
// （实测 p15 = 1×19 位 + 14×7 位，p19 亦混合 —— discover.go 里「页内不混合」那句注释已为假，
// 更正由 M1c-1 的 TASK-007 登记进 CONTRACTS）。任何下界都是对下一次 id 形态的猜测，
// 而猜错的形态是静默的。
//
// # 为什么不把 istitle="true" 也写进这条正则
//
// 写进去两个数就恒相等，backfillPage 的计数守卫会退化成 `n == n` —— 用被验对象自己的
// 尺子量自己。两者必须来自**互相独立**的判据：正则看的是「href + title + 日期三件套齐不齐」，
// 计数基准看的是「页面自称有几个条目」，站点只改其中一侧时才有东西会红。
//
// `(?s)` 让 `.` 跨行匹配（仓库里的快照被 core.autocrlf=input 规范化过，行尾形态不影响本正则）。
var backfillItemRE = regexp.MustCompile(
	`(?s)href="([^"]*?/(\d+)/index\.html)"[^>]*?\stitle="([^"]*)"[^>]*>.*?</a>\s*</font>\s*<span class="hui12">\s*(\d{4}-\d{2}-\d{2})\s*</span>`)

// backfillIstitleMark 是计数守卫的基准。
//
// ⚠️ 基准是 `istitle="true"` 而**不是** `class="hui12"`：实测 p146 上 `class="hui12"`
// 共 21 处 = 15×span（日期）+ 5×td + 1×table（页面骨架），拿它作基准会让守卫恒假、天天报错。
var backfillIstitleMark = []byte(`istitle="true"`)

// backfillItem 是 index 页上的一条列表项 —— **不论它是不是目标报告**。
//
// 整页条目都交出来（而不是只交报告），因为 M1c-1 的 TASK-002 有两处判据要用**整页**的日期：
// 按发布日期判停（`oldestOnPage.Before(from)`）与相邻页日期连续性
// （`p(N).oldest >= p(N+1).newest`）。只交报告的话，一页 15 条里通常 0～1 条是报告，
// 那两条守卫会建立在一个几乎恒空的集合上。
type backfillItem struct {
	ArticleID string // URL 里的数字串，位数不定
	URL       string // 绝对 URL（站内路径经 resolveURL 补全）
	Title     string // 取自 title= 属性，不是链接文本
	Published string // YYYY-MM-DD
}

// backfillPage 是一页 index 的扫描结果。
//
// Reports 是 Items 的子集，按页面顺序。分两个字段而不是给 Items 加一个 IsReport 标记：
// 调用方对两者的用法完全不同 —— Items 只被拿去算日期区间，Reports 才进 manifest。
type backfillPage struct {
	Items   []backfillItem
	Reports []backfillItem
}

// scanBackfillPage 扫描一页 index，交出整页列表项与其中的目标报告。
//
// 只在 **HTTP 200 的响应体**上调用：站点的越界页（实测 `11040-409.html` / `11040-500.html`）
// 返回的是 **HTTP 404 / 146 字节**，那一支由抓取层在进到这里之前处置。本函数看到 0 条，
// 含义只能是「200 了但版式变了」—— 软 404 与维护页也落在这一支。
//
// 三条守卫，各挡一种失效，**不能合并**：
//
//   - **0 条列表项 ⇒ 报错**：整页解析失效。回填翻 150 页跨越了一次真实站点重建
//     （2026-06-26），静默返回空的后果是回填看起来跑完了而 manifest 少了几十期。
//   - **条数 ≠ istitle 数 ⇒ 报错**：**部分**失效。上一条只挡得住整页归零 —— 正则少匹配
//     3 条而不是全部时它一个字都不报，而站点改版往往只改**某一类**列表项的属性顺序。
//     「部分失效」正是「三个月后发现 manifest 少了几十期」最可能的成因。
//   - **有条目但 0 条是报告 ⇒ 正常返回空 + nil**：大多数页都是这个形态，不是失效。
func scanBackfillPage(html []byte, base string) (backfillPage, error) {
	var page backfillPage
	for _, m := range backfillItemRE.FindAllSubmatch(html, -1) {
		abs, err := resolveURL(base, string(m[1]))
		if err != nil {
			return backfillPage{}, err
		}
		page.Items = append(page.Items, backfillItem{
			ArticleID: string(m[2]),
			URL:       abs,
			Title:     string(m[3]),
			Published: string(m[4]),
		})
	}

	if len(page.Items) == 0 {
		return backfillPage{}, fmt.Errorf("hestia backfill: no list items matched on this page: " +
			"the page layout likely changed; refusing to report it as an empty page")
	}
	if n := bytes.Count(html, backfillIstitleMark); n != len(page.Items) {
		return backfillPage{}, fmt.Errorf(
			"hestia backfill: matched %d list items but the page declares %d %s entries: "+
				"the page layout likely changed (partial parse failure)",
			len(page.Items), n, backfillIstitleMark)
	}

	for _, it := range page.Items {
		if backfillTitleRE.MatchString(it.Title) {
			page.Reports = append(page.Reports, it)
		}
	}
	return page, nil
}
