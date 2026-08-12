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

// pagingRE 匹配分页控件的 JS 调用：
//
//	jumpTo(this,'408','1','/goutongjiaoliu/113456/113469/11040-%1.html')
//	              ↑总页数 ↑当前页 ↑模板，%1 是页码占位
//
// 只捕获总页数与模板，**当前页那一组刻意不捕获**：同一份模板在每一页里都出现，
// 只是当前页那个数不同（p1 是 '1'、p2 是 '2'）。不依赖它，才能对任意一页的快照
// 解析出同一个模板。
var pagingRE = regexp.MustCompile(`jumpTo\(this,'(\d+)','\d+','([^']+)'\)`)

// parsePaging 从 index 页取出分页模板与总页数。
//
// 解析而不是把栏目 ID（11040）写死：ID 变了能自动跟随。更要紧的是解析失败会
// **报错**——页面改版后若退化成「只扫第 1 页」，月报发布 3 周后就掉出那 15 条，
// 管线看起来在跑却再也发现不了任何东西。
func parsePaging(html []byte) (tmpl string, totalPages int, err error) {
	m := pagingRE.FindSubmatch(html)
	if m == nil {
		return "", 0, fmt.Errorf("hestia discover: no paging control found: " +
			"the page layout likely changed; refusing to fall back to page 1 only")
	}
	totalPages, err = strconv.Atoi(string(m[1]))
	if err != nil || totalPages < 1 {
		return "", 0, fmt.Errorf("hestia discover: bad paging total %q", m[1])
	}
	tmpl = string(m[2])
	if !strings.Contains(tmpl, "%1") {
		return "", 0, fmt.Errorf("hestia discover: paging template %q has no %%1 placeholder", tmpl)
	}
	return tmpl, totalPages, nil
}

// pageURL 拼出第 page 页的绝对 URL。
//
// 第 1 页直接返回 indexURL，不套模板。理由**不是**模板生成的 URL 不存在——实测
// （2026-08-12）`11040-1.html` 返回 HTTP 200，38147 字节，与 `index.html` 逐字节
// 相同。真实理由是这两点：
//
//   - **不重复请求**：调用方是先取回 index 页、再从它的字节里解析出模板的，第 1 页
//     的内容此刻已在手上；套模板会为同一份字节再打一次请求。
//   - **不制造别名**：同一页有两个 URL 时，按 URL 去重的调用方会把它数成两页。
//
// ⚠️ 别拿 `index_1.html` 是 404 来论证这件事——那是**另一个 URL**，模板压根不生成它。
func pageURL(indexURL, tmpl string, page int) (string, error) {
	if page <= 1 {
		return indexURL, nil
	}
	return resolveURL(indexURL, strings.Replace(tmpl, "%1", strconv.Itoa(page), 1))
}

// resolveURL 把站内路径 ref 按 base 补成绝对 URL。
//
// 用 ResolveReference 而不是手工拼 scheme+host：它同时处理站内绝对路径
// （/goutongjiaoliu/...）与相对路径两种形态。
//
// 从 pageURL 抽出来给 scanPage 复用——两处要做的是同一件事（把 index 页上的
// 站内路径补全），此前是两份各自的实现。
func resolveURL(base, ref string) (string, error) {
	b, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("hestia discover: bad base url %q: %w", base, err)
	}
	r, err := url.Parse(ref)
	if err != nil {
		return "", fmt.Errorf("hestia discover: bad url %q: %w", ref, err)
	}
	return b.ResolveReference(r).String(), nil
}

// reportTitleRE 匹配《金融统计数据报告》的标题。
//
// 期次段是**可选的** —— 年度报告的标题里既没有「上半年」也没有「N月」
// （testdata 实测：`2025年金融统计数据报告`）。方案报告 4.1 的正则把这一段
// 写成必填，会让每年 1 月的年度数据被静默跳过：不报任何错，只是看起来
// 「今天没有新文章」。
//
// 这条正则里有**两个各自独立的机制**，挡的不是同一类东西（消融实测得出，别记混）：
//
//   - **期次段紧跟「金融统计数据」**：挡住同页上真实存在的干扰项
//     `国新办…介绍2026年上半年货币政策执行和金融统计数据情况` ——「上半年」后面
//     接的是「货币政策执行和」，而不是「金融统计数据」。
//   - **「报告」后缀**：挡住 `2026年7月金融统计数据情况`、`…金融统计数据简报`
//     这一类 —— 期次段确实紧跟「金融统计数据」，只是后缀不对。
//
// ⚠️ 别写成「『金融统计数据报告』六个字紧跟期次段挡住了那条干扰项」：把末尾的
// 「报告」删掉，那条干扰项**照样被拒**（实测），挡它的自始至终是「紧跟」。
// 两个机制各由 TestParsePeriodRejects 里对应的用例守着。
//
// 季度段（TASK-001）写成 `[一二三四五六七八九十]季度` 而不是只列真实存在的
// 「一季度」：多出来的那几个落进 parsePeriod 的**语义层**被显式拒绝，而不是在这里
// 悄悄失配。两种写法可观测结果相同（都不产候选），差别是语义层的拒绝有位置、
// 有注释、有用例钉住 —— 与「13月」「0月」由语义层挡下是同一个安排。
var reportTitleRE = regexp.MustCompile(
	`(\d{4})年(上半年|前三季度|[一二三四五六七八九十]季度|\d{1,2}月)?金融统计数据报告`)

// parsePeriod 从标题解析期次。映射与 types.go 的 periodEndMonth 一致
// （q1→03、h1→06、q1_q3→09、annual→12），不新增第二份期末月约定。
//
// 央行一年只发四份**年初起累计**的《金融统计数据报告》：一季度 / 上半年 /
// 前三季度 / 全年，加上每月的月报。四种累计期次对应四个期末月，月均折算除数
// 分别是 3 / 6 / 9 / 12。
func parsePeriod(title string) (period, periodType string, ok bool) {
	m := reportTitleRE.FindStringSubmatch(title)
	if m == nil {
		return "", "", false
	}
	year, seg := m[1], m[2]
	switch {
	case seg == "":
		return year + "-12", "annual", true
	case seg == "上半年":
		return year + "-06", "h1", true
	case seg == "前三季度":
		// 「前三季度」= 1–9 月累计，不是第 3 季度单季。periodType 因此叫 q1_q3
		// 而不是 q3：两者期末月同为 09，月均折算除数却是 9 与 3，而 types.go 的
		// validPeriodTypes 写着「写错会让信号判定错一个量级」。
		return year + "-09", "q1_q3", true
	case strings.HasSuffix(seg, "季度"):
		// 「一季度」是唯一真实存在的季度形态。二/三/四季度的《金融统计数据报告》
		// 央行**不发**（那三段分别由上半年 / 前三季度 / 全年覆盖），五季度以上更
		// 不成话。真出现了也必须拒：「三季度」字面上无法区分单季与累计，期末月同为
		// 09 而除数一个 3 一个 9 —— 宁可静默漏一期由人补，不可猜一个口径写进权威表。
		if seg != "一季度" {
			return "", "", false
		}
		return year + "-03", "q1", true
	default:
		// 放宽正则（期次段可选）必须配一步语义校验，否则放宽会把非法值带进来：
		// `\d{1,2}月` 匹配得上「13月」，也匹配得上「0月」。
		//
		// ⚠️ 这里的 err 分支**当前不可达**，别照着它写「Atoi 可能失败」这种理由。
		// 实测：Go 的 `\d` 只匹配 ASCII 0-9（全角「１２」、阿拉伯-印度「٣」都不匹配），
		// 加上 `{1,2}` 的长度上限，TrimSuffix 后必是 1-2 位 ASCII 数字 ⇒ Atoi 必成功。
		// 保留它是因为一旦有人放宽长度上限（如改成 `\d+月`），它立刻就可达了——
		// parsePaging 里同形状的那个 err 分支正是因为 `\d+` 无上限而能被溢出触达。
		// 同一个代码形状，两处相反的结论，差别只在正则有没有长度上限。
		n, err := strconv.Atoi(strings.TrimSuffix(seg, "月"))
		if err != nil || n < 1 || n > 12 {
			return "", "", false
		}
		return fmt.Sprintf("%s-%02d", year, n), "monthly", true
	}
}

// articleLinkRE 匹配一条文章链接：
//
//	<a href="/goutongjiaoliu/113456/113469/2026071512340454869/index.html" ...>标题</a>
//
// 栏目路径不写死（同 parsePaging 的理由：栏目 ID 变了能自动跟随）；article_id 是
// URL 里的数字串，**不设任何位数下界**。
//
// 🔴 此前这里写的是 `\d{14,}`（「实测 19 位」）。那是本包最贵的一次「用样本形状当
// 契约」：央行 2026-06-26 重建过站点，重建后新发文章的 article_id 是 **7 位**
// （2025 年前三季度报是 `5868082`）。实测（2026-08-12）分界在 p14/p15 之间 ——
// p1/p2/p7/p12/p13/p14 每页 15 条全 19 位，p15/p16/p17/p18 每页 15 条全 7 位，
// 页内不混合。于是 `\d{14,}` 在 p18 上**整页命中 0 条链接**：
// 不是 0 条候选，是 `scanPage` 的循环体一次都不执行。而 Discover 照常翻满 MaxPages
// 返回空，一个字的错都不报。
//
// 放宽的代价是每页多命中一批栏目导航链接（`/rmyh/105145/index.html` 这类，id 是
// 6 位栏目号），它们的链接文本是「货币政策」这种栏目名，过不了 parsePeriod ——
// 这句话由 TestScanPageIgnoresNavigationLinks 钉住，不是推断。
//
// ⚠️ 那条用例**刻意不钉「多出多少条」**：同一个问题按去没去重、算不算栏目自链接
// 有四个各自都对的答案（44/43/43/42，见该用例的注释）。钉住的是「0 条候选」。
//
// **别再给它加位数下界**（也别加成 `\d{6,}`）：站点重建过一次就会重建第二次，
// 任何下界都是对下一次 id 形态的猜测，而猜错的形态是静默的。真正的过滤器是
// parsePeriod 对链接文本的判断，那一层看的是标题、不是 id。
//
// `[^>]*>` 要穿过 onclick/target/title/istitle 四个属性才够到 `>`；`(?s)(.*?)</a>`
// 取的是**链接文本**而非 title= 属性值。
//
// ⚠️ 快照是 LF 版：仓库里的两份 index 快照被 core.autocrlf=input 规范化过（各去掉
// 67 个 CR）。`(?s)` 让 `.` 跨行匹配，所以行尾形态不影响本正则；但若哪天有人照着
// 原始 HTTP 响应写 `\r\n` 的字面量断言，会在仓库版上失配。
var articleLinkRE = regexp.MustCompile(`(?s)href="([^"]*?/(\d+)/index\.html)"[^>]*>(.*?)</a>`)

// tagRE 剥掉链接文本里的内联标签。
//
// ⚠️ 别写成「列表页的标题常被 <span> 之类包着」——**这两份快照并不支持那句话**：
// 实测 15×2 条链接的文本都紧跟在 `>` 之后，不含任何内联标签，tagRE 在它们上是
// 空操作。它是**防御性**的（央行改版加一层 <span> 就会让标题带标签而解析失败），
// 而防御性代码在真实语料上没有守卫，所以由 TestScanPageStripsInlineTags 用合成
// 页面钉住——没有那条，删掉 tagRE 全部测试照样绿。
var tagRE = regexp.MustCompile(`<[^>]+>`)

// ArticleChecker 回答「这篇文章是否见过」。Discover 用它决定翻页何时停。
//
// 接口定义在**消费方**而不是 Store 侧：Discover 用它，Store 实现它。这样
// discover 的测试可以喂一个 fake，不必开真库。
//
// # 为什么从 HasPeriod 换成 HasArticle（TASK-011，推翻此处原有的论证）
//
// 原注释写着「判停用**期次**而不是 article_id」，理由是站点迁移会让全部 id 变新
// ⇒ 每次唤起都翻满上限。**那个担心是真的，但它换来的代价更大，且是静默的**：
//
// 按期次判停的前提是「index 按时间倒序 ⇒ 再往后只会更旧」，而那句话把**发布时间
// 倒序**与**期次倒序**当成了同一件事。央行重发某期时两者分叉：修订版发布时间最新
// ⇒ 排最前，期次却是旧的、已入库 ⇒ **第一条就命中、当场停**，其后所有未入库期次
// 一个都发现不了。返回空 + nil error，与「今天没有新报告」在调用方看来完全一样。
//
// 按 article_id 判停时，修订版有**新 id** ⇒ 不命中 ⇒ 继续翻、被收进候选。
//
// **原论证担心的代价现在有出口，且我实测确认过**（见 discovery TASK-011）：
// hestia 的业务键是 {period, period_type}（store.go 的 NewSpec），**article_id 不在
// 业务键里** ⇒ 迁移后 Lookup 照样查得到那一行、published_at 不变 ⇒ Classify 判
// Duplicate ⇒ Save 走 refreshArticleID 把 id 刷成新的 ⇒ **下一轮就命中、恢复正常**。
// ⇒ 代价从「每次唤起都翻满」降级为「迁移后一轮全量重抓，之后自愈」，
// 而 MaxPages 把那一轮限制在有限范围内。TestDiscoverSelfHealsAfterSiteMigration 钉住它。
//
// # 为什么是 HasArticleInObservations 而不是 HasArticle（人类 2026-08-13 裁决）
//
// `HasArticle` 查两表（含 pending），拿它判停会让**落进 pending 的期次再也不会被
// 自动重试** —— Discover 见到它的 id 就停，修了校验 bug 或调了阈值之后它不会自己
// 回来，只能靠 --force。Sprint 035 已登记「落 pending 的期次对依赖历史的闸门永久
// 不可见」是结构性问题，那样等于把「更难自愈」变成「不可能自愈」。
//
// 只查权威表则三种情形都对：修订版（新 id、不在权威表）⇒ 不停、继续翻；正常已处理
// （id 在权威表）⇒ 停；pending 期次（id 只在 pending）⇒ **仍交出候选**，由 ingest 那层
// 的 `HasArticle`（查两表）挡住并打印 already ingested。
//
// ⇒ TASK-003/005 定案的「两层判据方向相反而都对」原样保住，本任务只把 Discover
// 这一层的判据从**期次**换成 **article_id**，没有把两层合并。
type ArticleChecker interface {
	HasArticleInObservations(ctx context.Context, articleID string) (bool, error)
}

// StopReason 说明 Discover 为何停止翻页。
//
// 存在的理由是**当前最危险的属性是「静默」**：返回空 + nil error 时，「命中已见过的
// 文章、正常停」与「翻满上限、可能还有更多」在调用方看来完全一样，而后者意味着窗口
// 外的期次永远发现不了。判据换成 article_id 并不会让这件事自己消失 —— 它是独立的一条。
type StopReason string

const (
	// StopSeen：命中已见过的 article_id，正常停。
	StopSeen StopReason = "seen_article"
	// StopMaxPages：翻满 cfg.MaxPages 仍未命中已见过的文章 —— **窗口外可能还有**。
	StopMaxPages StopReason = "max_pages"
	// StopExhausted：把站点的 totalPages 翻完了，没有更多页可翻。
	StopExhausted StopReason = "exhausted"
)

// Candidate 是 index 页上一条待抓的报告。
//
// Discover 只产出它，**不抓文章页** —— 取正文是 M1b-4b 的 ingest 串起来做的。
// 这条边界让 discover 能用快照完整测试，不碰网络。
type Candidate struct {
	ArticleID, URL, Title, Period, PeriodType string
}

// scanPage 从一页 HTML 里提取报告条目，非报告的链接一律跳过。
//
// 「跳过」是安静的，而这里安静是对的：一页 15 条里通常只有 0～1 条是报告，其余
// 都是正常的其他公告。这与 parsePaging 解析不到分页控件必须报错**不是同一回事**——
// 那里安静意味着整条发现链路失效，这里安静只是「这条不是我要的」。
func scanPage(html []byte, base string) ([]Candidate, error) {
	var out []Candidate
	for _, m := range articleLinkRE.FindAllSubmatch(html, -1) {
		title := strings.TrimSpace(tagRE.ReplaceAllString(string(m[3]), ""))
		period, periodType, ok := parsePeriod(title)
		if !ok {
			continue
		}
		abs, err := resolveURL(base, string(m[1]))
		if err != nil {
			return nil, err
		}
		out = append(out, Candidate{
			ArticleID:  string(m[2]),
			URL:        abs,
			Title:      title,
			Period:     period,
			PeriodType: periodType,
		})
	}
	return out, nil
}

// DiscoverCfg 是发现阶段的可调参数。mapstructure tag 供 config.go 装载。
type DiscoverCfg struct {
	IndexURL string        `mapstructure:"index_url"`
	MaxPages int           `mapstructure:"max_pages"`
	Timeout  time.Duration `mapstructure:"timeout"`
}

// Discover 扫描央行发布页，返回尚未入库的报告期次，最近的排在前面。
//
// 为什么要翻页：index 是混杂的新闻列表（实测 6115 条 / 408 页 / 每页 15 条），
// 每页只覆盖约 20 天，而月报每月 10–15 日发布 —— 发布约 3 周后必然掉出第 1 页。
// 只抓第 1 页在正常运行的日子也会漏。
//
// 为什么碰到**已见过的文章**就停：index 按发布时间倒序，再往前翻只会遇到更早发布
// 的文章，而它们在上一轮（或更早）已经被扫过了。
//
// ⚠️ **判据是 article_id 而不是期次**（TASK-011）。别改回期次：修订版重发时发布时间
// 最新、期次却是旧的、已入库 ⇒ 第一条就命中、当场停，**其后所有未入库期次静默消失**。
// 理由与自愈路径写在 ArticleChecker 上，那不是随手换的判据。
//
// ⚠️ **空库时会一直翻到 MaxPages**，这是 spec §4.3 定义的首跑行为，不是缺陷：
// 提前返回的条件只有「命中已见过的文章」，而空库下 HasArticle 恒 false。
// 别改成「发现候选就停」——那样首跑只能捡到最近一期，历史全部漏掉。
//
// 第二个返回值 StopReason 说明为何停下。**「翻满上限而停」与「命中已见过而停」
// 必须可分**：前者意味着窗口外可能还有未发现的期次，而两者的候选清单都可能是空的。
//
// 不抓文章页 —— 只产出候选清单，取正文是 4b 的 ingest 做的。
func Discover(ctx context.Context, f Fetcher, known ArticleChecker, cfg DiscoverCfg) ([]Candidate, StopReason, error) {
	first, err := f.Get(ctx, cfg.IndexURL)
	if err != nil {
		return nil, "", err
	}
	tmpl, totalPages, err := parsePaging(first)
	if err != nil {
		return nil, "", err
	}

	limit := cfg.MaxPages
	exhausted := false
	if limit > totalPages {
		limit = totalPages // 别请求不存在的页
		exhausted = true   // 翻完就是翻完了，不是「上限拦住了」
	}

	var out []Candidate
	// 按 article_id 去重，与判停同一把标识。用期次去重会让「同期次的另一篇」
	// （原文与修订版同时在榜）在**查库之前**就被 continue 掉，判停因此漏判。
	seen := make(map[string]bool) // 同一篇在分页边界上可能出现两次

	for page := 1; page <= limit; page++ {
		html := first // 第 1 页已经取过了，不重复请求
		if page > 1 {
			u, err := pageURL(cfg.IndexURL, tmpl, page)
			if err != nil {
				return nil, "", err
			}
			if html, err = f.Get(ctx, u); err != nil {
				return nil, "", err
			}
		}

		items, err := scanPage(html, cfg.IndexURL)
		if err != nil {
			return nil, "", err
		}
		for _, it := range items {
			if seen[it.ArticleID] {
				continue // 翻页时新文章上架会把边界那条挤到下一页重复出现
			}
			has, err := known.HasArticleInObservations(ctx, it.ArticleID)
			if err != nil {
				// 查库失败不能当成「没见过」继续翻 —— 那会把整段历史重新抓一遍。
				return nil, "", err
			}
			if has {
				return out, StopSeen, nil
			}
			seen[it.ArticleID] = true
			out = append(out, it)
		}
	}
	if exhausted {
		return out, StopExhausted, nil
	}
	return out, StopMaxPages, nil
}
