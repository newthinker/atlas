package hestia

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Context Checkpoint: done_criteria → test mapping（M1c-1 的 TASK-001）
//
//	functional[0] 三种报告标题逐字     → TestScanBackfillPageFindsThreeReportKinds
//	functional[0] 标题取 title= 属性   → TestScanBackfillPageTakesTitleAttributeNotLinkText
//	functional[1] article_id 无位数下界 → TestScanBackfillPageArticleIDHasNoDigitFloor
//	functional[2] 发布日期 YYYY-MM-DD  → TestScanBackfillPageExtractsPublishedDate
//	functional[3] 13 类干扰项零命中     → TestScanBackfillPageRejectsLookalikes
//	boundary[0]   0 条列表项 ⇒ 报错     → TestScanBackfillPageErrorsOnZeroListItems
//	boundary[1]   0 条报告 ⇒ 空+nil     → TestScanBackfillPageNoReportsIsNotAnError
//	boundary[2]   条数 == istitle 数    → TestScanBackfillPageCountMustMatchIstitle

// backfillTestBase 是解析站内相对路径用的基准 URL。与 discover_test.go 的 base 同形态。
const backfillTestBase = "https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/index.html"

// backfillListItemHTML 造一条与真实 index 页**逐字段同形态**的列表项。
//
// 原文（p146 实测，同一行，`</a></font><span>` 之间无空白）：
//
//	<td height="22" align="left"><font class="newslist_style" style="margin-right:10px;">
//	<a href="/goutongjiaoliu/113456/113469/2025092212550537670/index.html" onclick="void(0)"
//	   target="_blank" title="2020年2月金融统计数据报告" istitle="true">2020年2月金融统计数据报告</a>
//	</font><span class="hui12">2020-03-11</span></td>
//
// text 与 title 分开传：真实语料里 748 条有 124 条（16%）的**链接文本**在约 38 字处被
// 截成 `...`（本仓库 5 份快照里 16/75 条同形态），而 title= 属性永远是全文。
// 这个差异是 functional[0]「取 title= 属性」唯一能被观测到的地方。
func backfillListItemHTML(articleID, title, text, date string) string {
	return fmt.Sprintf(
		`<td height="22" align="left"><font class="newslist_style" style="margin-right:10px;">`+
			`<a href="/goutongjiaoliu/113456/113469/%s/index.html" onclick="void(0)" `+
			`target="_blank" title="%s" istitle="true">%s</a></font>`+
			`<span class="hui12">%s</span></td>`+"\n",
		articleID, title, text, date)
}

// backfillPageHTML 把若干条列表项裹进一份最小的页面骨架。
func backfillPageHTML(items ...string) []byte {
	return []byte(`<html><body><table><tbody><tr>` + strings.Join(items, "") + `</tr></tbody></table></body></html>`)
}

func backfillTitles(items []backfillItem) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.Title)
	}
	return out
}

// 🔴 functional[0]：p146 上**三种**报告各一篇，标题逐字断言。
//
// ⚠️ 三条各一个断言，**不是**「命中 3 条」一条了事：数量对而认错种类时，
// 只断数量的那个写法照样绿（本迭代的全部意义就在于区分「存量」与「增量」）。
//
// ⚠️ 用 p146 不用 p145：上游 spec §9 说「第 145 页三种标题齐全」是**假的**，
// 实测 p145 只有 2 篇，第三篇《2020年一季度金融统计数据报告》在 p144 —— 同日发布
// 被页边界劈开，实测 46% 的页边界是同日边界，是常态不是异常。
func TestScanBackfillPageFindsThreeReportKinds(t *testing.T) {
	page, err := scanBackfillPage(readTestdata(t, "pboc-index-p146-2020.html"), backfillTestBase)
	require.NoError(t, err)
	require.Len(t, page.Reports, 3, "p146 实测三种报告各一篇；空集或多命中都会让下面的断言失去意义")

	got := backfillTitles(page.Reports)
	assert.Contains(t, got, "2020年2月金融统计数据报告")
	assert.Contains(t, got, "2020年2月社会融资规模存量统计数据报告")
	assert.Contains(t, got, "2020年2月社会融资规模增量统计数据报告")

	// 整页 15 条里只有这 3 条是目标 —— 其余 12 条（央行票据互换、支付体系运行情况、
	// 金融市场运行情况…）是同页真实存在的阴性对照，由上面的 Len(3) 一并守住。
	assert.Len(t, page.Items, 15, "p146 实测 15 条列表项。⚠️ 这是**该页**的实测值，不是每页恒 15：实测末页 p408 只有 13 条")
}

// 🔴 functional[0]：标题必须取 `title=` 属性，不取链接文本。
//
// 这条在**真实语料上没有守卫**，必须用合成用例钉住：实测 748 条列表项里 124 条（16%）
// 的链接文本在约 38 字处被截断，而三种目标报告的标题最长 23 字、**当前恰好都没被截断**
// ⇒ 把实现改成取链接文本，全部真实样本测试照样绿。
// （与 discover.go 给 tagRE 写的是同一种理由：防御性代码在真实语料上没有守卫。）
//
// 这里刻意把链接文本截在「…数据报」处 —— 截断后 backfillTitleRE 认不出它，
// 取错来源的实现会在 require.Len(Reports, 1) 当场红，而不是悄悄给出个短标题。
func TestScanBackfillPageTakesTitleAttributeNotLinkText(t *testing.T) {
	const full = "2020年2月社会融资规模存量统计数据报告"
	html := backfillPageHTML(backfillListItemHTML(
		"2025092212550538131", full, "2020年2月社会融资规模存量统计数据报...", "2020-03-11"))

	page, err := scanBackfillPage(html, backfillTestBase)
	require.NoError(t, err)
	require.Len(t, page.Reports, 1, "取链接文本的实现在这里认不出报告（截断后没有『报告』二字）")

	assert.Equal(t, full, page.Reports[0].Title, "标题必须是 title= 属性的全文，不是被截断的链接文本")
	assert.Equal(t, full, page.Items[0].Title)
}

// 🔴 functional[1]：article_id **不设任何位数下界**，两个方向各一条真实样本。
//
// 🔴 Sprint 037 教训：此前 discover.go 的 articleLinkRE 写着 `\d{14,}`（「实测 19 位」），
// 央行 2026-06-26 重建站点后新发文章的 id 变成 7 位 ⇒ 整页命中 0 条、循环体一次都不执行、
// 一个字的错都不报。位数下界是对下一次 id 形态的猜测，而猜错的形态是静默的。
//
// ⚠️ 也别写任何依赖「页内位数一致」的逻辑：discover.go 里「p15/p16/p17/p18 每页 15 条
// 全 7 位，页内不混合」那句注释**现已为假**（实测 p15 = 1×19 位 + 14×7 位，p19 亦混合）。
// 该注释的更正由 M1c-1 的 TASK-007 登记进 CONTRACTS。
func TestScanBackfillPageArticleIDHasNoDigitFloor(t *testing.T) {
	t.Run("p146 是 19 位 id", func(t *testing.T) {
		page, err := scanBackfillPage(readTestdata(t, "pboc-index-p146-2020.html"), backfillTestBase)
		require.NoError(t, err)

		var got backfillItem
		for _, r := range page.Reports {
			if r.Title == "2020年2月金融统计数据报告" {
				got = r
			}
		}
		require.NotEmpty(t, got.ArticleID, "前置锚点：没取到那条时下面断的就是零值")
		assert.Equal(t, "2025092212550537670", got.ArticleID)
		assert.Len(t, got.ArticleID, 19)
		assert.Equal(t,
			"https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/2025092212550537670/index.html",
			got.URL, "站内绝对路径要补成完整 URL")
	})

	t.Run("p18 是 7 位 id", func(t *testing.T) {
		page, err := scanBackfillPage(readTestdata(t, "pboc-index-p18.html"), backfillTestBase)
		require.NoError(t, err)
		require.Len(t, page.Reports, 1, "p18 实测只有《2025年前三季度金融统计数据报告》一篇")

		assert.Equal(t, "2025年前三季度金融统计数据报告", page.Reports[0].Title)
		assert.Equal(t, "5868082", page.Reports[0].ArticleID)
		assert.Len(t, page.Reports[0].ArticleID, 7, "7 位 —— 任何 `\\d{14,}` 形态的下界都会在这里红")
	})
}

// 🔴 functional[2]：发布日期取紧随 `</a>` 的 `<span class="hui12">`，产出 YYYY-MM-DD。
//
// 期望值是**从 p146 样本自己核出来的**：三种报告同日发布于 2020-03-11
// （别照抄文档里的示例日期，那些是从 p145 抄的）。
//
// ⚠️ 判据必须是 `<span class="hui12">` 而不是 `class="hui12"`：p146 实测
// `class="hui12"` 共 21 处 = 15×span（日期）+ 5×td + 1×table（页面骨架）。
// 选错基准会让 boundary[2] 的计数守卫恒假、天天报错。
func TestScanBackfillPageExtractsPublishedDate(t *testing.T) {
	page, err := scanBackfillPage(readTestdata(t, "pboc-index-p146-2020.html"), backfillTestBase)
	require.NoError(t, err)
	require.Len(t, page.Reports, 3)

	for _, r := range page.Reports {
		assert.Equal(t, "2020-03-11", r.Published, "%s 的发布日期", r.Title)
	}

	// 逐条覆盖整页，而不只是那 3 条报告：TASK-002 的按日期判停与相邻页连续性守卫
	// 用的是**整页**条目的日期，某一条为空会让那两条守卫在这里看不见的地方失效。
	for i, it := range page.Items {
		assert.Regexp(t, `^\d{4}-\d{2}-\d{2}$`, it.Published, "第 %d 条 %q", i, it.Title)
	}
	assert.Equal(t, "2020-03-25", page.Items[0].Published, "p146 首条")
	assert.Equal(t, "2020-03-11", page.Items[len(page.Items)-1].Published, "p146 末条")
}

// 🔴 functional[3]：13 类干扰项**零命中**，与三条正向断言**成对**。
//
// ⚠️ 阴性断言在空集上平凡为真 —— 一个恒返回 nil 的实现能让 13 条 NotContains 全绿。
// 所以正向对照与阴性对照放在**同一份页面、同一次断言**里：Reports 必须**恰好**是
// 那几条正例，多一条少一条都红。
//
// ⚠️ 只要求「三种都识别」时，一条 `.*统计数据报告` 就能满分通过，同时把小额贷款公司
// 报告一起抓回来。真正挡住它们的是**期次段紧跟报告种类**——别放宽它。
func TestScanBackfillPageRejectsLookalikes(t *testing.T) {
	// 正例：三种报告 + 两种期次形态（前三季度 / 年度无期次段 / 存量退化成月）。
	// 后三条是真实标题：p18 快照、discover_test.go 已钉住的年度形态、
	// 设计附录 §E 实测的「累计期次里只有存量退化成月」。
	accepted := []string{
		"2020年2月金融统计数据报告",
		"2020年2月社会融资规模存量统计数据报告",
		"2020年2月社会融资规模增量统计数据报告",
		"2025年前三季度金融统计数据报告",
		"2025年金融统计数据报告",
		"2020年3月社会融资规模存量统计数据报告",
	}

	// 13 类干扰项，按**被拒的机制**分组。provenance 逐条标注：
	// 「实测」= Leader 52 页 / 748 条语料实测或本仓库快照可复核；「构造」= 我按同族造的近似例。
	rejected := []struct{ title, why string }{
		// —— A. 期次段与报告种类之间插了别的词（「紧跟」在挡）——
		{"2019年三季度小额贷款公司统计数据报告", "实测：期次段 +「统计数据报告」，只差中间那段"},
		{"2022年三季度小额贷款公司统计数据报告", "实测：同上，另一年"},
		{"2020年11月厦门市金融统计数据报告", "实测：省市分行 30+ 家，只差「厦门市」三字"},
		{"2019年前三季度地区社会融资规模统计表", "实测：期次段 +「社会融资规模」，差「地区」与后缀"},
		{"2020年一季度地区社会融资规模增量统计表", "实测：只差「地区」二字与后缀是「统计表」"},
		{"2026年二季度金融机构贷款投向统计报告", "实测（p1 快照可复核）：期次段后接「金融机构贷款投向」"},
		{"国新办举行新闻发布会 介绍2026年上半年货币政策执行和金融统计数据情况", "实测（p2 快照可复核）：「上半年」后接的是「货币政策执行和」"},

		// —— B. 期次段形态不在表内 ——
		{"2019年第三季度金融统计数据新闻发布会文字实录", "实测：「第三季度」——正则的 [一二三四五六七八九十]季度 不含「第」"},
		{"2020年11月份吉林省金融统计数据报告", "实测：「月份」而非「月」"},
		{"2019年第四季度支付体系运行总体情况", "实测（p146 同页）：「第四季度」，且后缀是「情况」"},

		// —— C. 整条标题里没有 \d{4}年 ——
		{"人民银行调查统计司有关负责人就4月份金融统计数据情况答《金融时报》记者问", "实测：只有「4月份」，没有四位年份"},

		// —— D. 期次段紧跟成立，只是报告种类不完整（「报告」后缀在挡）——
		{"2026年7月金融统计数据情况", "构造（discover.go 已登记同类）：期次紧跟，后缀是「情况」"},
		{"2020年2月社会融资规模存量统计数据", "构造：期次紧跟，漏了末尾「报告」二字"},
	}

	var items []string
	for i, title := range accepted {
		items = append(items, backfillListItemHTML(fmt.Sprintf("900000000000000%04d", i), title, title, "2020-03-11"))
	}
	for i, d := range rejected {
		items = append(items, backfillListItemHTML(fmt.Sprintf("800000000000000%04d", i), d.title, d.title, "2020-03-11"))
	}

	page, err := scanBackfillPage(backfillPageHTML(items...), backfillTestBase)
	require.NoError(t, err)
	require.Len(t, page.Items, len(accepted)+len(rejected), "前置锚点：整页 19 条都要被扫到，否则下面的阴性断言是平凡的")

	got := backfillTitles(page.Reports)
	assert.ElementsMatch(t, accepted, got, "Reports 必须恰好是那 6 条正例：多一条 = 干扰项漏网，少一条 = 正例被误拒")
	for _, d := range rejected {
		assert.NotContains(t, got, d.title, d.why)
	}
}

// href 补不成绝对 URL 时**报错**，不是把那一条悄悄跳过。
//
// 跳过会同时踩两条守卫的空子：条目数少一条 ⇒ 计数守卫报的是「部分失效」这个**错误的成因**，
// 而真实成因（一条 href 不合法）再也看不见。与 discover.go 的 scanPage 同处置。
//
// `%zz` 不是合法的百分号转义，url.Parse 直接拒 —— 这是这条分支唯一够得着的触发方式
// （href 已经被正则约束成 `.../\d+/index.html` 形态了）。
func TestScanBackfillPageFailsOnUnparsableHref(t *testing.T) {
	html := []byte(`<td><font><a href="/goutongjiaoliu/%zz/113469/2025092212550537670/index.html" ` +
		`onclick="void(0)" target="_blank" title="2020年2月金融统计数据报告" istitle="true">` +
		`2020年2月金融统计数据报告</a></font><span class="hui12">2020-03-11</span></td>`)

	_, err := scanBackfillPage(html, backfillTestBase)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bad url")
}

// 🔴 boundary[0]：某页匹配到 **0 条列表项 ⇒ 报错**，不是返回空切片。
//
// ⚠️ 这条**只能用合成用例**：实测 `11040-409.html` 与 `11040-500.html` 返回的是
// **HTTP 404 / 146 字节**，不是 200 + 0 条 ⇒ 真实站点**给不出** 0 条这个输入。
// 只用真实语料的话，这条 DoD 会以「没遇到过」的形式恒绿。
//
// ⚠️ 与 HTTP 404 **分开判**：scanBackfillPage 只在 HTTP 200 的响应体上调用，
// 404 由抓取层（M1c-1 的 TASK-002 / TASK-005）在进到这里之前处置。
// 本函数看到 0 条，含义只能是「200 了但版式变了」——软 404 与维护页也落在这一支。
func TestScanBackfillPageErrorsOnZeroListItems(t *testing.T) {
	t.Run("维护页：200 但一条列表项都没有", func(t *testing.T) {
		_, err := scanBackfillPage([]byte(
			`<html><body><div class="notice">系统维护中，请稍后访问</div></body></html>`), backfillTestBase)
		require.Error(t, err, "静默返回空 ⇒ 回填看起来跑完了而 manifest 少了几十期")
		assert.Contains(t, err.Error(), "layout", "错误信息要指向「页面结构可能变了」")
	})

	t.Run("软 404 的 146 字节短响应", func(t *testing.T) {
		_, err := scanBackfillPage([]byte(`<html><head><title>404</title></head><body>Not Found</body></html>`), backfillTestBase)
		require.Error(t, err)
	})
}

// 🔴 boundary[1]：某页**有**列表项但其中 0 条是报告 ⇒ 正常返回空 + nil error。
//
// ⚠️ 这条与 boundary[0] **互补，不是重复**：只写 boundary[0] 时，把「0 条报告也报错」
// 这个 bug 写进实现，测试照样绿。大多数页都是这个形态（实测 p1、p146 之外的绝大多数页）。
// 别因为看起来像重复就删掉任何一条，也别把两条合成一个测试函数。
func TestScanBackfillPageNoReportsIsNotAnError(t *testing.T) {
	t.Run("合成页：3 条列表项、0 条报告", func(t *testing.T) {
		html := backfillPageHTML(
			backfillListItemHTML("2025092212550518686", "中国人民银行于3月25日开展央行票据互换（CBS）操作", "中国人民银行于3月25日开展央行票据互换（CBS）操作", "2020-03-25"),
			backfillListItemHTML("2025092212554577612", "2020年2月份金融市场运行情况", "2020年2月份金融市场运行情况", "2020-03-20"),
			backfillListItemHTML("2025092212550552611", "2019年第四季度支付体系运行总体情况", "2019年第四季度支付体系运行总体情况", "2020-03-17"),
		)

		page, err := scanBackfillPage(html, backfillTestBase)
		require.NoError(t, err, "「这页没有报告」是正常的，不是解析失效")
		assert.Empty(t, page.Reports)
		assert.Len(t, page.Items, 3, "列表项本身必须照常交出来 —— TASK-002 要靠它算该页的日期区间")
	})

	// 真实语料上的同一形态：p1 是首页，15 条里一条报告都没有（月报发布约 3 周后才掉出去）。
	t.Run("真实快照 p1：15 条列表项、0 条报告", func(t *testing.T) {
		page, err := scanBackfillPage(readTestdata(t, "pboc-index-p1.html"), backfillTestBase)
		require.NoError(t, err)
		assert.Empty(t, page.Reports)
		assert.NotEmpty(t, page.Items)
	})
}

// 🔴 boundary[2]：匹配到的列表项数必须等于该页 `istitle="true"` 的出现次数，不等 ⇒ 报错。
//
// 理由：「0 条 ⇒ 报错」只挡得住**整页失效**，挡不住**部分失效** —— 正则少匹配 3 条而不是
// 全部时，它一个字都不报。而站点改版往往只改**某一类**列表项的属性顺序，不是把整页推倒；
// 「部分失效」正是「三个月后发现 manifest 少了几十期」最可能的成因。
//
// ⚠️ 计数基准用 `istitle="true"`（实测 p145/p146/p120 各 15 个），**不能用 `class="hui12"`**
// （各 21 个，含 6 处页面骨架 —— 5×td + 1×table）。
// ⚠️ 不得断言「每页恒 15 条」：实测末页 p408 只有 13 条，硬编码 15 会在 `--from 2005-01` 上炸。
func TestScanBackfillPageCountMustMatchIstitle(t *testing.T) {
	t.Run("真实快照上两个数相等", func(t *testing.T) {
		for _, name := range []string{"pboc-index-p146-2020.html", "pboc-index-p18.html", "pboc-index-p1.html"} {
			page, err := scanBackfillPage(readTestdata(t, name), backfillTestBase)
			require.NoError(t, err, "%s", name)
			assert.Len(t, page.Items, 15, "%s 实测 15 条（**该页**的实测值，不是每页恒 15）", name)
		}
	})

	t.Run("部分失效：3 个 istitle 只解析出 2 条 ⇒ 报错", func(t *testing.T) {
		// 中间那条缺了 `<span class="hui12">` 日期块 —— 模拟「站点只改了某一类列表项」。
		broken := `<td height="22" align="left"><font class="newslist_style">` +
			`<a href="/goutongjiaoliu/113456/113469/2025092212550536990/index.html" onclick="void(0)" ` +
			`target="_blank" title="中国人民银行公告〔2020〕第5号" istitle="true">中国人民银行公告〔2020〕第5号</a></font></td>` + "\n"

		html := backfillPageHTML(
			backfillListItemHTML("2025092212550537670", "2020年2月金融统计数据报告", "2020年2月金融统计数据报告", "2020-03-11"),
			broken,
			backfillListItemHTML("2025092212550538131", "2020年2月社会融资规模存量统计数据报告", "2020年2月社会融资规模存量统计数据报告", "2020-03-11"),
		)

		_, err := scanBackfillPage(html, backfillTestBase)
		require.Error(t, err, "少认一条也必须报错 —— 静默少认正是 manifest 缺期的成因")
		assert.Contains(t, err.Error(), "istitle", "错误信息要说清是拿哪个基准对不上的")
		assert.Contains(t, err.Error(), "layout")
	})

	// 反方向的失效：站点把 istitle 属性整个换掉（条目还在、还解析得出来）。
	// 计数基准归零而条目非零 ⇒ 同样报错。**不能**因为「反正条目都拿到了」就放行 ——
	// 基准失效之后这条守卫对后续所有页面都成了摆设，而它恰恰是唯一能发现部分失效的东西。
	t.Run("istitle 属性整体被换掉 ⇒ 报错", func(t *testing.T) {
		html := []byte(strings.ReplaceAll(
			string(backfillPageHTML(backfillListItemHTML(
				"2025092212550537670", "2020年2月金融统计数据报告", "2020年2月金融统计数据报告", "2020-03-11"))),
			`istitle="true"`, `data-title="true"`))

		_, err := scanBackfillPage(html, backfillTestBase)
		require.Error(t, err, "计数基准消失 = 守卫失效，必须出声")
		assert.Contains(t, err.Error(), "istitle")
	})
}
