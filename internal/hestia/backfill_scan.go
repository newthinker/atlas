package hestia

import (
	"context"
	"fmt"
	"regexp"
	"time"
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

// backfillIstitleAnchorRE / backfillInternalHrefRE / backfillInternalIstitleCount
// 一起构成计数守卫的基准：**站内**的 `istitle="true"` 条目数。
//
// ⚠️ 基准是 `istitle="true"` 而**不是** `class="hui12"`：实测 p146 上 `class="hui12"`
// 共 21 处 = 15×span（日期）+ 5×td + 1×table（页面骨架），拿它作基准会让守卫恒假、天天报错。
//
// # 为什么要再排掉站外条目（TASK-007 真跑时撞出来的）
//
// 原基准是 `bytes.Count(html, []byte("istitle=\"true\""))`，在真实语料上**不成立**：
// 央行列表页会混进指向站外的条目（实测 p54 一条 gov.cn 的国务院决定），它带全套
// istitle / title / 日期属性，只有 href 是绝对 URL ⇒ backfillItemRE 的
// `/(\d+)/index\.html` 结构上匹配不到 ⇒ 14 vs 15，整条回填在 p54 硬失败。
// 实测 p1..p155 共 2325 条 istitle 锚，**站外恰好 1 条、不符的页恰好 1 页**，
// 新基准 0 反例。
//
// # 为什么不是「只数 href 形如文章页的锚」
//
// 那个修法更自然，但它让**守卫跟着被守护的东西一起坏**：href 形态哪天变了
// （如 `/index.htm`），基准会同步下降，两个数照样相等，守卫恒绿。
//
//	⇒ **守卫的基准必须独立于它要守护的那个属性。**
//	  基准与被测量共用一个来源时，它测不出那个来源的变化。
//
// 「站内 / 站外」这个分界与 href 的具体形态**正交**，形态变化仍会告警。这与上面
// 「不把 istitle 写进 backfillItemRE」是同一条理由的第二次应用。
//
// # 两条正则各自的 `\s` 都不是装饰
//
// `\sistitle=` 与 `\shref=`：前缀必须是空白，挡的是 `data-istitle=` / `data-href=`
// 这类**后缀同名**属性 —— 与本文件上面 `\stitle="` 挡 `istitle="true"` 完全同构，
// 那次的后果是「条数一条不少、只有标题全错」。`(?:[^>]*\s)?` 让 `istitle` 出现在
// 第一个属性位（`<a istitle="true" …>`）时也数得到，不依赖属性顺序。
var (
	backfillIstitleAnchorRE = regexp.MustCompile(`<a\s(?:[^>]*\s)?istitle="true"[^>]*>`)
	backfillInternalHrefRE  = regexp.MustCompile(`\shref="/`)
)

// backfillInternalIstitleCount 数该页**站内**的 istitle 条目数，即计数守卫的基准。
//
// ⚠️ 判据是「href 以 `/` 开头」这一条，**不是**「href 形如 `/<id>/index.html`」，
// 更不是「backfillItemRE 认得出」（后者把守卫退化成 `n == n`，部分失效从此永不报警）。
// 三者只差一个谓词，各由 TestScanBackfillPageCountBaselineExcludesExternalLinks 的
// 三格分别钉住 —— 尤其第三格（href 形态从 `.html` 变 `.htm`）：**在补它之前，
// 「按形态分」那个变异跑出来是 850 全绿 SURVIVED**，整段论证没有任何东西守着。
func backfillInternalIstitleCount(html []byte) int {
	n := 0
	for _, tag := range backfillIstitleAnchorRE.FindAll(html, -1) {
		if backfillInternalHrefRE.Match(tag) {
			n++
		}
	}
	return n
}

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
	if n := backfillInternalIstitleCount(html); n != len(page.Items) {
		return backfillPage{}, fmt.Errorf(
			"hestia backfill: matched %d list items but the page declares %d site-internal "+
				`istitle="true" entries: the page layout likely changed (partial parse failure)`,
			len(page.Items), n)
	}

	for _, it := range page.Items {
		if backfillTitleRE.MatchString(it.Title) {
			page.Reports = append(page.Reports, it)
		}
	}
	return page, nil
}

// ============================================================================
// M1c-1 的 TASK-002：翻页与按日期停止
// ============================================================================

// backfillMaxPages 是翻页的兜底上限（design-spec §3.3）。
//
// 它**不是正常出口**：正常出口是日期判停。翻满它意味着日期判定没生效，
// 所以那一支报错而不是返回已收集的部分（见 scanBackfillIndex）。
const backfillMaxPages = 200

// backfillDateLayout 是列表页日期的形态，也是本文件比较日期的唯一形态。
const backfillDateLayout = "2006-01-02"

// backfillIndexCfg 是 index 侧翻页的可调参数。
//
// From 用 time.Time 而不是字符串：与同批的 backfillSearchURL(…, from, to time.Time, …)
// 保持同一个边界类型，TASK-005 的交叉校验要同时喂两侧，类型不一致会在那里变成一处转换。
//
// ⚠️ From 的语义是**发布日期**，不是期次（设计附录 §A1）。index 按发布时间倒序，
// 期次在页面上根本不成序，翻页判停本来就只能看发布日期。推论：`--from 2020-01` 的产出
// **包含 2019 年度的三篇**（实测发布于 2020-01-16）——这是刻意的，不是缺陷。
type backfillIndexCfg struct {
	IndexURL string
	From     time.Time
	MaxPages int // <= 0 时用 backfillMaxPages
}

// backfillIndexPage 是翻页过程中经过的一页。
//
// 带上 HTML 与日期区间，是因为**只有翻页循环知道这两样**：
//
//   - design-spec §7.3 要求 index 页也存盘（它们同样不可再生，存了之后重跑与 M1c-2
//     的核对都能离线做）；
//   - 设计附录 §A3 规定快照落名 `<抓取序号>-<该页最新条目日期>_<该页最老条目日期>.html`
//     ——**页码不能作唯一键**，因为页码随新文章上架而漂移，重跑时同一个文件名会对应
//     不同内容的页。
//
// 不交出去的话，TASK-006 要么把 150 页重抓一遍，要么把这个循环重写一遍。
type backfillIndexPage struct {
	Page   int    // 站点页码，1 起
	URL    string // 实际请求的 URL（第 1 页是 IndexURL 本身）
	HTML   []byte // 原始响应体
	Newest string // 该页最新条目的发布日期 YYYY-MM-DD
	Oldest string // 该页最老条目的发布日期
}

// backfillScanResult 是一次 index 侧扫描的产出。
type backfillScanResult struct {
	// Reports 是去重后的目标报告，按发现顺序（新 → 旧），且**发布日期均 >= From**。
	//
	// 判停是**页级**的、过滤是**条目级**的，两件事：判停那一页横跨 From，页上既有
	// ≥From 的也有 <From 的，前者必须留下（boundary[0]）、后者必须滤掉——否则
	// 「From 比最新一期还新 ⇒ 返回空」（boundary[2]）就不成立，第 1 页的报告会被
	// 原样带回来。两条 DoD 合起来唯一确定了这个语义。
	//
	// 推论：产出**不含**发布早于 From 的报告，所以判停页不会往下游漏进一个残缺期次
	// （那会让 TASK-008 的逐期对账报一条假的「缺篇」）。
	Reports []backfillItem
	// Pages 是实际翻过的页，含判停那一页。
	Pages []backfillIndexPage
}

// backfillDateRange 取一页里最新/最老的发布日期。
//
// 取整页的 max/min，**不取首末条**：真实语料确实是倒序的，但那是观测值不是契约，
// 而相邻页连续性守卫的正确性直接依赖这两个数的定义。页内顺序哪天变了，
// 取首末条会让守卫比较到错误的两个数、且不会有任何东西报错。
//
// 比较用**字典序**：Published 已被 backfillItemRE 钉成 `\d{4}-\d{2}-\d{2}`，
// ISO-8601 定长形态下字典序与时序等价，且避开时区与解析失败两种新的失败模式。
// 调用方保证 items 非空（scanBackfillPage 的 0 条守卫已经拦在前面）。
func backfillDateRange(items []backfillItem) (newest, oldest string) {
	newest, oldest = items[0].Published, items[0].Published
	for _, it := range items[1:] {
		newest = max(newest, it.Published)
		oldest = min(oldest, it.Published)
	}
	return newest, oldest
}

// scanBackfillIndex 逐页扫描 index，收集目标报告，按**发布日期**停止翻页。
//
// # 为什么按日期停而不按页码
//
// 页码随新文章上架而漂移——今天的第 150 页明天就不是（design-spec §3.3）。
//
// # 判停那一页必须先完整扫描、再停
//
// 实测：以 From=2020-01-01 为例，p151 的 15 条跨 **2019-12-25..2020-01-14**，其中
// **7 条 ≥ From**（DoD 与本注释上一版写的上界 2020-01-09 是笔误，实测到 01-14）。把停止判断放在扫描之前会把它们
// 连同整页一起丢掉。⚠️ 实测 p151 上恰好 0 条目标报告 ⇒ **这个 bug 在真实语料上
// 一个测试都不会红**，守着它的是 TestScanBackfillIndexScansStopPageBeforeBreaking
// 那份合成页面。
//
// # 三条出口，两种性质
//
//   - **日期判停**（正常出口）：某页最老一条早于 From ⇒ 扫完该页后返回。
//   - **翻完站点自称的 totalPages**（正常出口）：没有更多页可翻了。
//   - **翻满 MaxPages**（失败出口）：报错。那说明日期判定没生效，静默返回已收集的
//     部分会让「回填看起来跑完了」而窗口外的期次一个都没抓。
//
// ⚠️ 后两者**必须分开**：合并成报错的话，totalPages < MaxPages 的小栏目（或 From 早于
// 站点最早一篇）会在**已经看完全部页面**的情况下失败；合并成放行的话，MaxPages 那条
// 兜底就没人守着了。这与 discover.go 的 StopExhausted / StopMaxPages 是同一个区分。
//
// # index 页抓取失败：立刻中止，原样返回
//
// **不**当成「这页没有」继续翻，也**不**记 failed 继续 —— 这与「单篇文章抓取失败记
// failed 并继续」（TASK-006）处置相反，是刻意的：少一页 index = 静默少 15 条候选，
// 与「整页 0 条」等价危害。
func scanBackfillIndex(ctx context.Context, f Fetcher, cfg backfillIndexCfg) (backfillScanResult, error) {
	first, err := f.Get(ctx, cfg.IndexURL)
	if err != nil {
		return backfillScanResult{}, err
	}
	tmpl, totalPages, err := parsePaging(first)
	if err != nil {
		return backfillScanResult{}, err
	}

	limit := cfg.MaxPages
	if limit <= 0 {
		limit = backfillMaxPages
	}
	exhausted := false
	if limit > totalPages {
		limit = totalPages // 别请求不存在的页
		exhausted = true   // 翻完就是翻完了，不是「上限拦住了」
	}

	from := cfg.From.Format(backfillDateLayout)
	var res backfillScanResult
	seen := make(map[string]bool) // 按 article_id 去重：边界那条会在翻页时重复出现
	prevOldest := ""

	for page := 1; page <= limit; page++ {
		html, url := first, cfg.IndexURL // 第 1 页已经取过了，不重复请求
		if page > 1 {
			if url, err = pageURL(cfg.IndexURL, tmpl, page); err != nil {
				return backfillScanResult{}, err
			}
			if html, err = f.Get(ctx, url); err != nil {
				return backfillScanResult{}, err
			}
		}

		scanned, err := scanBackfillPage(html, cfg.IndexURL)
		if err != nil {
			return backfillScanResult{}, err
		}
		newest, oldest := backfillDateRange(scanned.Items)

		// 相邻页连续性：p(N).oldest >= p(N+1).newest。
		//
		// # 它检测的是「列表前移导致的**重复**」，**不是**跳页
		//
		// 🔴 别把它读成「抓跳页」——**方向正好相反**，这是 TASK-002 返工时由 test-agent-27
		// 构造观察出来的（非推理）：
		//
		//   - **新文上架** ⇒ 后续条目整体**下移** ⇒ p(N+1) 的开头重新出现 p(N) 已见过的条目
		//     ⇒ `newest` 变新 ⇒ **本守卫触发**。这一支没有数据丢失，重复由下面的 seen 去重吃掉。
		//   - **条目下架/撤回** ⇒ 后续条目整体**前移** ⇒ p(N) 与 p(N+1) 之间那段被整个略过
		//     ⇒ `newest` 变**老** ⇒ 判据更满足 ⇒ **本守卫永不触发**。
		//
		// ⇒ **下架导致的整页跳过，本守卫抓不到**，而那一支才是有数据丢失的一支。
		// 由 TestScanBackfillIndexSkipIsInvisibleToContinuityGuard 钉住这个盲区。
		//
		// **跳页检测归 M1c-1 的 TASK-008（完整性对账）**：它按 (年, 期次表述) 归组、
		// 用结构规则（v1 期次应 3 篇 / v2 期次应 1 篇）列缺篇——那是个**全局正向**判据，
		// 不需要知道翻页时发生了什么。本层只报「序乱了」，不报「少了哪几期」。
		//
		// ⚠️ 必须是 `>=` 不是 `>`：验证者三批独立快照实测 22 对，**取等号约占一半**
		// （同日发布被页边界劈开，是常态不是异常）。写成 `>` 会把近一半的正常翻页判成错误。
		//
		// ⚠️ 上一版注释这里写的是「实测 41 对相邻页 41/41 成立」——**那个数没有任何人验过**，
		// 传播链是 reviewer → DoD → 本注释。现在这句换成了独立三批实测（0 反例，delta ∈ [0,5]）。
		if prevOldest != "" && newest > prevOldest {
			return backfillScanResult{}, fmt.Errorf(
				"hestia backfill: page %d starts at %s which is newer than page %d's oldest entry %s: "+
					"the listing shifted while paging (entries may repeat across pages)",
				page, newest, page-1, prevOldest)
		}

		res.Pages = append(res.Pages, backfillIndexPage{
			Page: page, URL: url, HTML: html, Newest: newest, Oldest: oldest,
		})
		for _, r := range scanned.Reports {
			// 条目级过滤与页级判停是**两件事**：判停那一页横跨 From，页上 ≥From 的
			// 必须留（boundary[0]），<From 的必须滤（否则 boundary[2]「From 比最新
			// 一期还新 ⇒ 返回空」不成立）。少了这一句，第 1 页的报告会被原样带回。
			if r.Published < from {
				continue
			}
			if seen[r.ArticleID] {
				continue
			}
			seen[r.ArticleID] = true
			res.Reports = append(res.Reports, r)
		}

		// 停止判断放在**收集之后**：见上面「判停那一页必须先完整扫描」。
		if oldest < from {
			return res, nil
		}
		prevOldest = oldest
	}

	if exhausted {
		return res, nil
	}
	return backfillScanResult{}, fmt.Errorf(
		"hestia backfill: reached max_pages=%d without any page falling before from=%s: "+
			"the date cutoff never took effect; refusing to return a partial result silently",
		limit, from)
}
