package hestia

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/newthinker/atlas/internal/macro/bitemporal"
)

func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	return b
}

// 分页模板藏在 JS 的 onclick 里：
//
//	jumpTo(this,'408','1','/goutongjiaoliu/113456/113469/11040-%1.html')
//	              ↑总页数 ↑当前页 ↑模板，%1 是页码占位
//
// 解析它而不是写死那个 11040：栏目 ID 变了能自动跟随。
func TestParsePaging(t *testing.T) {
	tmpl, total, err := parsePaging(readTestdata(t, "pboc-index-p1.html"))
	require.NoError(t, err)

	assert.Contains(t, tmpl, "%1", "模板里要保留页码占位符")
	assert.Contains(t, tmpl, "/goutongjiaoliu/113456/113469/")
	assert.Greater(t, total, 100, "该栏目实测 408 页；这里只要求是个像样的数")
}

// 「解析而不是写死栏目 ID」这句话，上面那条**证明不了**。
//
// 一个 `return "/goutongjiaoliu/113456/113469/11040-%1.html", 408, nil` 的写死实现
// 能让 TestParsePaging 三条断言全绿——它断的正好都是真实快照里那个栏目的特征值。
// 本条喂一份栏目 ID、路径、总页数全都不同的 HTML，写死实现在这里必然红。
//
// 这是 functional[1]「ID 变了能自动跟随」唯一的实证；删掉它，那句话就退回成声明。
func TestParsePagingFollowsChangedColumnID(t *testing.T) {
	tmpl, total, err := parsePaging([]byte(
		`<a href="###" onclick="jumpTo(this,'12','1','/newpath/999/88888-%1.html')">尾页</a>`))
	require.NoError(t, err)

	assert.Equal(t, "/newpath/999/88888-%1.html", tmpl, "模板必须来自页面，不能写死 11040")
	assert.Equal(t, 12, total, "总页数必须来自页面，不能写死 408")
}

// 同一份模板在每一页里都出现，变的只是「当前页」那个数（p1 是 '1'、p2 是 '2'）。
// 从任意一页的快照都该解析出同一个模板与同一个总页数——否则 Discover 翻到第 2 页
// 之后再解析，会得到与第 1 页不同的模板，翻页链路当场断掉。
//
// 这条也顺带钉住正则**不捕获**当前页那一组：真去用了它，两页的结果就会不同。
func TestParsePagingSamePagingOnEveryPage(t *testing.T) {
	tmpl1, total1, err := parsePaging(readTestdata(t, "pboc-index-p1.html"))
	require.NoError(t, err)
	tmpl2, total2, err := parsePaging(readTestdata(t, "pboc-index-p2.html"))
	require.NoError(t, err)

	assert.Equal(t, tmpl1, tmpl2, "两页解析出的模板必须相同")
	assert.Equal(t, total1, total2, "两页解析出的总页数必须相同")
}

// 解析不到必须报错，不能退化成「只扫第 1 页」。
//
// 静默退化是最坏的结果：页面改版后 discover 每次只看 15 条最新新闻，
// 而月报发布 3 周后就掉出这 15 条 —— 管线看起来在跑，实际再也发现不了任何东西。
func TestParsePagingFailsLoudly(t *testing.T) {
	_, _, err := parsePaging([]byte("<html><body>没有分页控件</body></html>"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "paging")
}

// 分页控件**在**，但里面的数不成话——同样必须报错，理由与上一条完全一样：
// 一个解析成 0 页的实现，翻页循环一次都不会转，静默退化成「什么都发现不了」。
//
// ⚠️ 别把理由写强：负数与非数字**根本走不到 strconv.Atoi**，它们被正则的 (\d+)
// 挡在前面，落进「没有分页控件」那条分支（实测确认，见 wantErr 列）。真正让
// `err != nil` 那半个条件可达的只有**数字溢出**一种输入——没有溢出这一行，
// 实现里的 `err != nil ||` 就是永不为真的死代码，而没人看得出来。
//
// 两层断言，分工写在这里免得后人误删：
//   - 主断言 require.Error + 含 "paging"：守 DoD 本身（无论走哪条分支都成立）。
//   - wantErr 精确串：记录**当前实现下这个输入落进哪条分支**。若哪天只有它变红，
//     先看主断言——主断言仍绿说明 DoD 没破，改这里的 wantErr 即可。
func TestParsePagingRejectsBadTotal(t *testing.T) {
	tests := []struct {
		name    string
		total   string
		wantErr string
	}{
		{"零页", "0", "bad paging total"},
		{"溢出 int64", "99999999999999999999", "bad paging total"},
		{"负数（被正则挡下，到不了 Atoi）", "-5", "no paging control"},
		{"非数字（被正则挡下）", "abc", "no paging control"},
		{"空（被正则挡下）", "", "no paging control"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			html := []byte(`jumpTo(this,'` + tt.total + `','1','/a/b-%1.html')`)

			_, total, err := parsePaging(html)
			require.Error(t, err, "总页数不合法时必须报错，不得退化成只扫第 1 页")
			assert.Contains(t, err.Error(), "paging")
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Zero(t, total, "报错时不得同时返回一个看似可用的页数")
		})
	}
}

// 模板丢了 %1 占位符时，Replace 会是空操作，每一页都拼出同一个 URL——
// 翻页循环会把第 1 页抓 408 遍，且一个新期次都发现不了。同样要当场报错。
func TestParsePagingRejectsTemplateWithoutPlaceholder(t *testing.T) {
	_, _, err := parsePaging([]byte(`jumpTo(this,'408','1','/a/b.html')`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "paging")
	assert.Contains(t, err.Error(), "%1")
}

// 第 1 页用 index.html 本身，第 N 页才套模板；模板是站内绝对路径，
// 要用 index URL 的 scheme+host 补全。
func TestPageURL(t *testing.T) {
	const idx = "https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/index.html"
	const tmpl = "/goutongjiaoliu/113456/113469/11040-%1.html"

	tests := []struct {
		page int
		want string
	}{
		{1, idx},
		{2, "https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/11040-2.html"},
		{408, "https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/11040-408.html"},
	}
	for _, tt := range tests {
		got, err := pageURL(idx, tmpl, tt.page)
		require.NoError(t, err)
		assert.Equal(t, tt.want, got, "第 %d 页", tt.page)
	}
}

// pageURL 的两条错误路径：两个 url.Parse 各一条。
//
// 断言 errors.Unwrap 非 nil，而不是只看错误串里有没有那几个字——`%w` 写成 `%v`
// 时错误串**一模一样**，只有 Unwrap 分得出来。调用方要靠它拿到 *url.Error 判因。
//
// ⚠️ 错误串在 TASK-003 抽出 resolveURL 时变过一次（`bad index url`→`bad base url`、
// `bad paging template`→`bad url`），本条的两处 Contains 随之更新。**这不是放松**：
// 断言的精度没变（仍是各自不同的确定串），Unwrap 那两条一字未动，且当时**没有红**
// ——说明重构没碰包裹性质，红的只有串。两个串仍互不相同，是为了钉住「base 坏」与
// 「ref 坏」在错误信息上必须可分；把它们合并成同一句会让调用方无从判因。
//
// 一处信息变化，记下免得被当成 bug：`bad url` 打印的是**代入页码后的 ref**
// （`/a/b-2\x7f.html`）而不是模板原文（`/a/b-%1\x7f.html`）。实际请求的是前者，
// 排障时前者更有用，但「它来自模板」这个归因需要看上下文才知道。
func TestPageURLRejectsUnparsableInput(t *testing.T) {
	const goodTmpl = "/goutongjiaoliu/113456/113469/11040-%1.html"

	t.Run("index url 不可解析", func(t *testing.T) {
		_, err := pageURL("://nope", goodTmpl, 2)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "bad base url")
		require.NotNil(t, errors.Unwrap(err), "底层 url.Parse 的错误必须被 %%w 包住")
	})

	t.Run("模板不可解析", func(t *testing.T) {
		_, err := pageURL("https://www.pbc.gov.cn/a/index.html", "/a/b-%1\x7f.html", 2)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "bad url")
		// 「bad url」不是「bad base url」的子串，所以上面那条 Contains 目前确实
		// 只认这一支。但那是**串的偶然形状**在替断言把关：谁把 base 那句改成
		// 「bad url (base)」，两条就会同时绿而无人察觉。下面这句让「两支必须可分」
		// 成为被断言的性质，而不是碰巧成立的事实。
		assert.NotContains(t, err.Error(), "bad base url", "ref 出错不该报成 base 出错")
		require.NotNil(t, errors.Unwrap(err), "底层 url.Parse 的错误必须被 %%w 包住")
	})
}

// 年度报告的标题里没有「上半年」也没有「N月」。方案报告 4.1 的正则
// `\d{4}年(上半年|\d{1,2}月)金融统计数据报告` 要求这一段必填，会把每年 1 月的
// 年度数据整个跳过 —— 且不报任何错，只是看起来「今天没有新文章」。
//
// 映射与 types.go 的 periodEndMonth 一致（q1→03、h1→06、q1_q3→09、annual→12），
// 不新增第二份约定。
func TestParsePeriod(t *testing.T) {
	tests := []struct {
		title      string
		period     string
		periodType string
	}{
		{"2026年上半年金融统计数据报告", "2026-06", "h1"},
		{"2026年7月金融统计数据报告", "2026-07", "monthly"},
		{"2026年12月金融统计数据报告", "2026-12", "monthly"},
		{"2025年金融统计数据报告", "2025-12", "annual"},
		// 与上面四条都不同年的一条：年份、月份都必须来自标题本身。
		// 单靠前四条，一个「按标题查表」的实现也能全绿；多一个任意年月不能证伪它，
		// 但能让那张表荒谬到一眼可见。真正排除写死的是下面 Rejects 那组的配对。
		{"2019年3月金融统计数据报告", "2019-03", "monthly"},

		// —— 季报（TASK-001）。两条都是**真实标题**，各自的快照就在 testdata 里 ——
		//
		// 期末月与 types.go 的 periodEndMonth 一致（q1→03、q1_q3→09），仍是一份约定。
		// 两者都是**年初起累计**口径，与 h1/annual 同族：央行一年只发这四份累计报告
		// （一季度 / 上半年 / 前三季度 / 全年），没有「单独第三季度」那一份。
		{"2026年一季度金融统计数据报告", "2026-03", "q1"},     // testdata/pboc-index-p7.html
		{"2025年前三季度金融统计数据报告", "2025-09", "q1_q3"}, // testdata/pboc-index-p18.html
		// 同上一条不同年，理由与 2019 年那条相同：排除按标题查表。
		{"2020年一季度金融统计数据报告", "2020-03", "q1"},
	}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			p, pt, ok := parsePeriod(tt.title)
			require.True(t, ok)
			assert.Equal(t, tt.period, p)
			assert.Equal(t, tt.periodType, pt)
		})
	}
}

// 不该匹配的东西。第一条是同一页上**真实存在**的条目。
//
// ⚠️ 本组与 TestParsePeriod 是**互补的一对，不要删任何一组**：
// 单看本组，一个恒返回 false 的实现全绿；单看那组，一个恒返回 true 的实现
// 在四条正例上也全绿。两组同时存在，两种退化实现才都被排除。
//
// ⚠️ 正则里有**两个各自独立的机制**，别把功劳记混（消融实测，非推断）：
//
//   - **期次段紧跟**：挡住 `国新办…上半年货币政策执行和金融统计数据情况` ——
//     「上半年」后面接的是「货币政策执行和」而不是「金融统计数据」。
//   - **「报告」后缀**：挡住 `2026年7月金融统计数据情况` 这一类 —— 期次段确实紧跟
//     「金融统计数据」，只是后缀不对。
//
// 计划与 DoD 把干扰项被拒的功劳记在「报告」六个字上，**实测是错的**：把正则末尾
// 的「报告」去掉，那条干扰项**照样被拒**（是「紧跟」在挡），而当时没有任何用例会红
// ⇒ 「报告」二字是一段没人守着的代码。下面最后两条就是补给它的守卫。
func TestParsePeriodRejects(t *testing.T) {
	for _, title := range []string{
		"国新办举行新闻发布会 介绍2026年上半年货币政策执行和金融统计数据情况",
		"2026年二季度金融机构贷款投向统计报告",
		"2026年6月金融市场运行情况",
		"中国人民银行公告〔2026〕第20号",
		"2026年13月金融统计数据报告", // 月份上界：正则匹配得上，语义校验挡下
		"2026年0月金融统计数据报告",  // 月份下界：同一处校验的另一侧
		"2026年7月金融统计数据情况",  // 「报告」后缀：期次段紧跟，只是后缀不对
		"2026年上半年金融统计数据简报", // 同上，另一种后缀

		// —— 季度分支的语义边界（TASK-001）——
		//
		// 正则的季度段写成 `[一二三四五六七八九十]季度` 而不是只列「一季度」，
		// 为的是让下面这些落进**语义层**被显式拒绝，而不是在正则那里悄悄失配。
		// 两种写法的可观测结果相同（都不产候选），差别在于：语义层拒绝是一个
		// 有位置、可加注释、可被本组用例钉住的决定。
		//
		// 「二/三/四季度金融统计数据报告」央行**不发**（一年只有一季度/上半年/
		// 前三季度/全年四份累计报告）。真出现了也必须拒：「三季度」字面上无法区分
		// 「第三季度单季」（7-9 月）与「前三季度累计」（1-9 月），期末月同为 09 而
		// 月均折算除数一个是 3 一个是 9 —— 猜错正是 validPeriodTypes 注释警告的
		// 「错一个量级」。宁可静默漏一期由人补，不可猜一个口径写进权威表。
		"2026年二季度金融统计数据报告",
		"2026年三季度金融统计数据报告",
		"2026年四季度金融统计数据报告",
		"2026年五季度金融统计数据报告", // 季度上界：正则匹配得上，语义校验挡下（同 13月 那条）
	} {
		t.Run(title, func(t *testing.T) {
			_, _, ok := parsePeriod(title)
			assert.False(t, ok)
		})
	}
}

// 从真实快照里提取报告条目。第 1 页没有、第 2 页有 —— 这正是要测的常态。
func TestScanPage(t *testing.T) {
	const base = "https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/index.html"

	// ⚠️ 本条与下面「第 2 页提取出报告条目」是**互补的一对，删任何一条都会开一个缺口**：
	// 一个恒返回 nil 的 scanPage 能让本条全绿（空集上平凡为真），是下一条的
	// require.NotEmpty 把它排除掉的。
	//
	// 它守的**不只是** scanPage，还有**快照分布本身**：p1 无报告是 T2 存两份快照的
	// 全部理由（只留有报告的那份就测不出「翻页」）。README 已预告报告再过两三周会掉到
	// 第 3 页；将来更新快照时若把 p1 换成了含报告的页面，本条会红 —— 那正是它该响的
	// 时候，说明「翻页直到找到」这条链路已经失去了它的阴性对照。**别为了让它绿而删它。**
	// （TASK-001 补：放宽 articleLinkRE 后本条的空集**更需要**阴性对照 —— 循环体
	// 现在每页转 59 次而不是 15 次。那份对照在 TestScanPageIgnoresNavigationLinks。）
	t.Run("第 1 页没有报告条目", func(t *testing.T) {
		got, err := scanPage(readTestdata(t, "pboc-index-p1.html"), base)
		require.NoError(t, err)
		assert.Empty(t, got, "首页 15 条覆盖约 20 天，月报发布 3 周后就掉出去了")
	})

	t.Run("第 2 页提取出报告条目", func(t *testing.T) {
		got, err := scanPage(readTestdata(t, "pboc-index-p2.html"), base)
		require.NoError(t, err)
		require.NotEmpty(t, got, "快照抓取时第 2 页应含报告；若已下沉需改用 p3")

		c := got[0]
		assert.Contains(t, c.Title, "金融统计数据报告")
		// ⚠️ 断言只要求「是数字串」，**不设位数下界**。此前这里写的是 `^\d{14,}$`，
		// 与 articleLinkRE 当时的 `\d{14,}` 互为镜像 —— 那不是在守护什么，是把缺陷
		// 钉成了契约：央行 2026-06-26 重建站点后新发文章的 article_id 是 **7 位**
		// （实测 p15 起全是 7 位），位数下界会让它们整批静默失配。
		assert.Regexp(t, `^\d+$`, c.ArticleID, "article_id 是 URL 里的数字串")
		assert.True(t, strings.HasPrefix(c.URL, "https://www.pbc.gov.cn/"),
			"站内绝对路径要补成完整 URL，得到 %s", c.URL)
		assert.Regexp(t, `^\d{4}-\d{2}$`, c.Period)
		assert.Contains(t, periodTypeList(), c.PeriodType)

		// 字段断言覆盖**每一条**而不只是 got[0]：只断言首条时，一个「第一条算对、
		// 其余瞎填」的实现照样全绿，而 Discover 会把整页候选都交给下游。
		for i, c := range got {
			assert.Regexp(t, `^\d+$`, c.ArticleID, "第 %d 条", i)
			assert.True(t, strings.HasPrefix(c.URL, "https://www.pbc.gov.cn/"), "第 %d 条", i)
			assert.Regexp(t, `^\d{4}-\d{2}$`, c.Period, "第 %d 条", i)
			assert.Contains(t, periodTypeList(), c.PeriodType, "第 %d 条", i)
		}
	})

	// 🔴 第 7 页的一季度报（真实快照，抓取于 2026-08-12）。
	//
	// 它的 article_id 是 19 位，旧正则也认得 —— 本条钉的是**标题分支**：
	// 「一季度」此前不在 reportTitleRE 的期次段里，整条链接会被 parsePeriod 拒掉，
	// 与 p18 那条一样静默。两条一起才覆盖两处静默点（链接层 + 标题层）。
	t.Run("第 7 页提取出一季度报", func(t *testing.T) {
		got, err := scanPage(readTestdata(t, "pboc-index-p7.html"), base)
		require.NoError(t, err)
		require.NotEmpty(t, got, "前置锚点：p7 上应当提取到候选，空集会让下面的断言平凡通过")
		require.Len(t, got, 1, "p7 上只有一条报告条目")

		assert.Equal(t, "2026年一季度金融统计数据报告", got[0].Title)
		assert.Equal(t, "2026041311133582598", got[0].ArticleID)
		assert.Equal(t, "2026-03", got[0].Period)
		assert.Equal(t, "q1", got[0].PeriodType)
		assert.Equal(t,
			"https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/2026041311133582598/index.html",
			got[0].URL)
	})

	// 🔴 第 18 页的前三季度报：**本任务的核心用例**，钉住 articleLinkRE 的位数下界。
	//
	// 实测（2026-08-12，见 testdata/README.md）：articleLinkRE 写 `\d{14,}` 时，
	// p18 整页**命中 0 条链接**（不是 0 条候选 —— 是循环体一次都不执行），而 p1/p2/p7
	// 上它命中 15 条 ⇒ 从第 15 页起 `Discover` 翻页照常、返回空，一个字的错都不报。
	//
	// 断言钉的是**字面量 `5868082`**，不是「id 至少 N 位」：位数下界正是那个缺陷的
	// 形状，换一个数字重钉一遍等于把它再钉一次。
	//
	// require.NotEmpty 是前置锚点，且这里**第一次真正兑现**：用旧正则跑本条时
	// scanPage 返回的确实是 nil，没有它下面的 got[0] 会 panic 而不是给出可读的红。
	t.Run("第 18 页提取出前三季度报（7 位 article_id）", func(t *testing.T) {
		got, err := scanPage(readTestdata(t, "pboc-index-p18.html"), base)
		require.NoError(t, err)
		require.NotEmpty(t, got,
			"前置锚点：p18 的 article_id 是 7 位，位数下界会让整页命中 0 条链接")
		require.Len(t, got, 1, "p18 上只有一条报告条目")

		assert.Equal(t, "2025年前三季度金融统计数据报告", got[0].Title)
		assert.Equal(t, "5868082", got[0].ArticleID, "央行 2026-06-26 重建站点后的 id 是 7 位")
		assert.Equal(t, "2025-09", got[0].Period)
		assert.Equal(t, "q1_q3", got[0].PeriodType)
		assert.Equal(t,
			"https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/5868082/index.html",
			got[0].URL)
	})

	t.Run("同页的干扰项不被收进来", func(t *testing.T) {
		got, err := scanPage(readTestdata(t, "pboc-index-p2.html"), base)
		require.NoError(t, err)
		// ⚠️ 这个前置锚点是 TASK-005 补的（TASK-003 遗留）：没有它，`got` 为空时
		// 下面的循环体一次都不执行、断言平凡通过。实测 scanPage 恒返回 nil 时
		// **单跑本子测试是 PASS 的**（整包才被兄弟用例拦住）——而验证者按 DoD
		// 逐条单跑取证时拿到的正是那个假绿，还会写进验收矩阵。
		require.NotEmpty(t, got, "前置锚点：p2 上应当提取到候选，空集会让下面的遍历平凡通过")
		for _, c := range got {
			assert.NotContains(t, c.Title, "国新办",
				"「介绍…金融统计数据情况」不是报告，不该进候选")
		}
	})
}

// 一页上有多条报告时，**每一条都要返回**。
//
// 真实快照测不出这条：p2 恰好只有 1 条报告条目，于是一个「找到第一条就 break」的
// 实现能让上面全部断言照样绿——包括那个「遍历每一条」的字段断言（只有一条可遍历）。
// 这正是「有没有一个我想排除的实现能让断言照样绿」问出来的缺口，只能用合成页面补。
//
// 不是假想的风险：Discover（T5）要靠 scanPage 返回**整页**候选来决定是否继续翻页，
// 漏掉同页的第二条会让那一期静默丢失。央行同月发布年报与 12 月月报正是这种形态。
func TestScanPageReturnsEveryReportOnPage(t *testing.T) {
	const base = "https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/index.html"
	page := []byte(`
<a href="/goutongjiaoliu/113456/113469/2026011512340454111/index.html" target="_blank">2025年金融统计数据报告</a>
<a href="/goutongjiaoliu/113456/113469/2026011518015458222/index.html" target="_blank">国新办举行新闻发布会 介绍2025年货币政策执行和金融统计数据情况</a>
<a href="/goutongjiaoliu/113456/113469/2026011519015458333/index.html" target="_blank">2025年12月金融统计数据报告</a>
`)

	got, err := scanPage(page, base)
	require.NoError(t, err)
	require.Len(t, got, 2, "同页两条报告都要收，中间夹的干扰项不收")

	// 顺序按页面出现顺序，且两条各自解析正确——年报与同月的月报是两个不同的业务键
	// （period_type 是主键的一部分），混为一条会让其中一期永远不入库。
	assert.Equal(t, "2025-12", got[0].Period)
	assert.Equal(t, "annual", got[0].PeriodType)
	assert.Equal(t, "2026011512340454111", got[0].ArticleID)

	assert.Equal(t, "2025-12", got[1].Period)
	assert.Equal(t, "monthly", got[1].PeriodType)
	assert.Equal(t, "2026011519015458333", got[1].ArticleID)
}

// 标题被内联标签包着时要能剥干净。
//
// ⚠️ 这条**必须用合成页面**：实测两份真实快照里，15 条链接的文本都紧跟在 `>` 后面，
// 没有任何内联标签（`>2026年上半年金融统计数据报告</a>`）⇒ tagRE 在真实快照上是
// **空操作**。删掉 tagRE，全部基于快照的测试照样绿 —— 它本来是一段没人守着的代码。
//
// 计划的注释写「列表页的标题常被 <span> 之类包着」，但这两份快照并不支持那句话。
// 保留 tagRE 是**防御性的**（央行改版加个 <span> 就会让标题带标签而解析失败），
// 本条就是那份防御的守卫：没有它，防御在场而无效。
func TestScanPageStripsInlineTags(t *testing.T) {
	const base = "https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/index.html"
	page := []byte(`<a href="/goutongjiaoliu/113456/113469/2026071512340454869/index.html" ` +
		`target="_blank"><span class="t">2026年上半年<em>金融统计数据报告</em></span></a>`)

	got, err := scanPage(page, base)
	require.NoError(t, err)
	require.Len(t, got, 1, "标题被 <span>/<em> 包着时也要认出来")
	assert.Equal(t, "2026年上半年金融统计数据报告", got[0].Title, "内联标签要被剥干净")
	assert.Equal(t, "2026-06", got[0].Period)
	assert.Equal(t, "h1", got[0].PeriodType)
}

// URL 补全失败时必须**报错**，不能把这一条悄悄跳过。
//
// 这是本包反复要防的那个形态：`continue` 掉一条解析失败的候选，管线看起来照常在跑，
// 而那一期的报告再也不会被发现——与 parsePaging 退化成「只扫第 1 页」同源。
// scanPage 里「跳过非报告」是安静的（一页 15 条里 14 条本就不是报告），
// **但「本该是报告却处理不了」必须响**，两者不能混为一谈。
//
// 断言返回 nil 而不只是 err != nil：一个「报错但仍返回已收集条目」的实现会让调用方
// 拿到一份**看起来正常、实则残缺**的候选集，那比直接失败更难发现。
func TestScanPageFailsOnUnresolvableURL(t *testing.T) {
	got, err := scanPage(readTestdata(t, "pboc-index-p2.html"), "://nope")
	require.Error(t, err, "base url 不可解析时必须报错，不得静默丢弃候选")
	assert.Contains(t, err.Error(), "bad base url")
	assert.Nil(t, got, "报错时不得返回部分结果")
	require.NotNil(t, errors.Unwrap(err), "底层 url.Parse 的错误必须被 %%w 包住")
}

// 上面那条**证不出**「不得返回部分结果」，这一条才能。
//
// 消融实测：把 `return nil, err` 改成 `return out, err`，上面那条照样 PASS ——
// 因为 p2 只有 1 条报告，第一条就失败，那一刻 out 本来就是 nil，两种写法**返回值
// 完全相同**。断言写着「报错时不得返回部分结果」，却挡不住它要挡的那件事。
//
// 要区分，必须先成功收集若干条、再让后面某一条失败。这里第二条链接的 href 里塞了
// 一个控制字符让 url.Parse 报错，此时 out 已有 1 条 —— 两种写法这才分得开。
//
// 为什么值得较真：返回「部分结果 + error」的调用方若只看结果不看 err（或只记日志
// 继续跑），拿到的是一份**看起来正常、实则残缺**的候选集，那一期就此静默丢失。
func TestScanPageReturnsNoPartialResultOnError(t *testing.T) {
	const base = "https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/index.html"
	page := []byte("" +
		`<a href="/goutongjiaoliu/113456/113469/2026011512340454111/index.html">2025年金融统计数据报告</a>` +
		"\n" +
		`<a href="/goutongjiaoliu/\x7f/2026011519015458333/index.html">2025年12月金融统计数据报告</a>`)
	// 上面那个 \x7f 是字面四个字符，这里换成真正的控制字符
	page = []byte(strings.Replace(string(page), `\x7f`, "\x7f", 1))

	got, err := scanPage(page, base)
	require.Error(t, err, "URL 不可解析必须报错")
	assert.Contains(t, err.Error(), "bad url")
	assert.Nil(t, got, "已收集到 1 条时报错，仍不得把那 1 条返回出去")
}

// 上面「干扰项不被收进来」单独看是**平凡为真**的：如果 articleLinkRE 压根没提取到
// 那条干扰项，它当然不在结果里，而「过滤器在做功」这个结论就是假的 —— 两种情形在
// scanPage 的返回值上**完全无法区分**。
//
// 本条补上那个区分：直接断言提取器**确实看见了**干扰项。两条合起来才构成
// 「提取到了 → 被 parsePeriod 拒绝」这条因果链；TestParsePeriodRejects 守的是后半段。
//
// reviewer 手工实测过这个事实（两页各恰好命中 15 条 = 每页条目数，干扰项就在 p2 的
// 命中列表里）。把它写成断言而不是停在报告里，是因为**手工实测保不住将来**：
// articleLinkRE 若哪天改得不再匹配干扰项那种形态，上面那条会静默退化成平凡为真。
func TestScanPageFiltersRatherThanMisses(t *testing.T) {
	const jammer = "国新办举行新闻发布会 介绍2026年上半年货币政策执行和金融统计数据情况"

	// 「无漏无重」现在按**新闻栏目下的条目**数，而不是全页命中数。
	//
	// articleLinkRE 放宽到 `\d+` 之后，全页命中里混进了 44 条栏目导航链接
	// （`/rmyh/105145/index.html` 这类，id 是 6 位栏目号）。用全页命中数当判据会把
	// 「15 条新闻一条不漏」这个真正要守的性质，和「导航链接有多少条」这个与本包
	// 无关的页面装修细节绑在一起 —— 央行改一次导航栏就会红，而那不说明任何问题。
	//
	// 16 = 15 条新闻 + 栏目自身那条 `/113469/index.html`（链接文本「新闻发布」）。
	// 后者被 parsePeriod 拒掉，由下面那条「导航链接产出 0 候选」的用例覆盖。
	const newsPath = "/goutongjiaoliu/113456/113469/"
	for _, page := range []string{
		"pboc-index-p1.html", "pboc-index-p2.html",
		"pboc-index-p7.html", "pboc-index-p18.html",
	} {
		t.Run(page, func(t *testing.T) {
			var news int
			for _, m := range articleLinkRE.FindAllSubmatch(readTestdata(t, page), -1) {
				if strings.Contains(string(m[1]), newsPath) {
					news++
				}
			}
			assert.Equal(t, 16, news,
				"每页 15 条新闻 + 1 条栏目自链接，提取器应无漏无重")
		})
	}

	// 干扰项确实在 p2 的命中列表里 —— 即它进了过滤流程，不是没被看见。
	var titles []string
	for _, m := range articleLinkRE.FindAllSubmatch(readTestdata(t, "pboc-index-p2.html"), -1) {
		titles = append(titles, strings.TrimSpace(tagRE.ReplaceAllString(string(m[3]), "")))
	}
	assert.Contains(t, titles, jammer, "干扰项必须被提取器看见，否则「过滤器在做功」无从谈起")

	// 而 parsePeriod 拒绝它 —— 因果链的后半段（与 TestParsePeriodRejects 重叠是有意的：
	// 那条守 parsePeriod 的行为，本条守的是「这条因果链在真实快照上成立」）。
	_, _, ok := parsePeriod(jammer)
	assert.False(t, ok, "干扰项被看见了，必须是被 parsePeriod 拒的")
}

// 🔴 放宽 articleLinkRE 的**否定式边界**：多进来的栏目导航链接必须产出 0 候选。
//
// 把 `\d{14,}` 放宽到 `\d+` 是本任务的核心改动，代价是每页多命中一批链接——全是
// `/rmyh/105145/index.html` 这类栏目导航页（id 是 6 位栏目号）。放宽本身安全：它们的
// 链接文本是「货币政策」「网站地图」这种栏目名，过不了 parsePeriod。
//
// **但「安全」这件事必须有断言钉住，否则下一个放宽的人没有网**：articleLinkRE 已经
// 没有任何位数约束了，再往下松只能松 href 的形态，而那一步的后果就没人测了。
//
// ⚠️ **断言钉的是「0 条候选」，刻意不钉「多出多少条」** —— 后者是快照的偶然属性。
// 这不是审美判断，是实测：同一个问题「放宽多出几条」有**四个各自都对的答案**，
// 差别只在你去没去重、以及算不算栏目自链接（p1 实测）：
//
//	44 = 全部命中数之差（59 − 15），**含重复**
//	43 = 按 href 去重后多出的条数（网站坐标里「网站地图」那条在页头页尾各一次）
//	43 = 非新闻栏目路径下的命中数，**含重复**（与上一个同值纯属巧合）
//	42 = 非新闻栏目路径下的**去重**条数
//
// 任何一个数写进断言，都会在另一个口径的人重跑时红，而**红的理由与实现无关** ——
// 那种红最容易被误读成「实现错了」然后去改实现。⇒ 前置锚点改用**内容式**
// （NotEmpty + 钉住具体两条导航链接），它不随页头页脚增删而变。
//
// 本条与上面「第 1 页没有报告条目」是**互补的一对**：那条断言 p1 产出空集，本条
// 断言那个空集**不是平凡的** —— 循环体确实转过，转进来的每一条都被 parsePeriod 拒掉。
// 只有那条时，一个恒返回 nil 的 scanPage 与一个正确过滤的 scanPage 无从区分。
func TestScanPageIgnoresNavigationLinks(t *testing.T) {
	const base = "https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/index.html"
	const newsPath = "/goutongjiaoliu/113456/113469/"

	for _, page := range []string{
		"pboc-index-p1.html", "pboc-index-p2.html",
		"pboc-index-p7.html", "pboc-index-p18.html",
	} {
		t.Run(page, func(t *testing.T) {
			var hrefs, titles []string
			for _, m := range articleLinkRE.FindAllSubmatch(readTestdata(t, page), -1) {
				if strings.Contains(string(m[1]), newsPath) {
					continue
				}
				hrefs = append(hrefs, string(m[1]))
				titles = append(titles, strings.TrimSpace(tagRE.ReplaceAllString(string(m[3]), "")))
			}

			// 前置锚点：**内容式**，不钉总数（理由见上面那张四个数的表）。
			// 三条缺一不可 —— 没有它们，下面「全部被拒」的循环会在空集或错的
			// 集合上平凡通过，而那正是本用例要防的事。
			require.NotEmpty(t, hrefs, "放宽位数下界后应当有栏目导航链接被命中")
			assert.Contains(t, hrefs, "/rmyh/105145/index.html", "货币政策栏目那条应在其中")
			assert.Contains(t, titles, "货币政策")
			assert.Contains(t, titles, "网站地图", "页头页尾各一次，也是上面 43/42 之差的来源")

			// 钉 0：这才是本条断言的信息量所在。
			var produced int
			for i, title := range titles {
				if _, _, ok := parsePeriod(title); ok {
					produced++
					t.Errorf("导航链接 %s（%q）不该产出候选", hrefs[i], title)
				}
			}
			assert.Zerof(t, produced, "多命中的 %d 条导航链接必须一条候选都不产", len(hrefs))
		})
	}

	// 整条链路上的同一件事：p1 全页命中的每一条都进了 scanPage 的循环体，
	// 一条候选都不产（同样钉 0，不钉命中总数）。
	got, err := scanPage(readTestdata(t, "pboc-index-p1.html"), base)
	require.NoError(t, err)
	assert.Empty(t, got, "p1 没有报告条目，命中的每一条都被 parsePeriod 拒掉")
}

const testIndexURL = "https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/index.html"

// fakeFetcher 从快照喂页面并记录请求顺序。
//
// calls 是有意记的：「翻了几页」和「翻页顺序」本身就是要断言的行为。只看返回值
// 的话，翻满上限的实现与碰到已知期次就停的实现给出同样的结果。
type fakeFetcher struct {
	pages map[string][]byte
	calls []string
	err   error
}

func (f *fakeFetcher) Get(_ context.Context, url string) ([]byte, error) {
	f.calls = append(f.calls, url)
	if f.err != nil {
		return nil, f.err
	}
	b, ok := f.pages[url]
	if !ok {
		return nil, fmt.Errorf("fake: 没有为 %s 准备页面", url)
	}
	return b, nil
}

// fakeArticleChecker 按 article_id 回答「见过没有」（TASK-011 起判停改用它）。
//
// have 的键是 **article_id**，不是期次 —— 这正是本任务改判据的地方。键写错会让
// 用例静默退化成「库里什么都没有」（恒 false ⇒ 永不提前返回），而那样的测试全绿。
type fakeArticleChecker struct {
	have map[string]bool // key: article_id，语义是「**在权威表里**见过」
	// havePeriod 的键是 period + "/" + periodType。**它不是给 Discover 用的** ——
	// Discover 只调 HasArticle。它存在是为了让「修订版」这个形态可构造：同一期
	// **期次已入库、而这一篇的 article_id 是新的**。没有它，那个场景在 fake 上
	// 根本表达不出来，消融也就无从对照（把实现改回 HasPeriod 时它才被调到）。
	havePeriod map[string]bool
	err        error
}

func (f fakeArticleChecker) HasArticleInObservations(_ context.Context, id string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.have[id], nil
}

func (f fakeArticleChecker) HasPeriod(_ context.Context, p, pt string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.havePeriod[p+"/"+pt], nil
}

// twoPageFetcher 用真实快照拼一个两页的站点，并返回第 2 页的 URL。
func twoPageFetcher(t *testing.T) (*fakeFetcher, string) {
	t.Helper()
	p1 := readTestdata(t, "pboc-index-p1.html")
	p2 := readTestdata(t, "pboc-index-p2.html")

	tmpl, _, err := parsePaging(p1)
	require.NoError(t, err)
	u2, err := pageURL(testIndexURL, tmpl, 2)
	require.NoError(t, err)

	return &fakeFetcher{pages: map[string][]byte{testIndexURL: p1, u2: p2}}, u2
}

// targetOnPage2 问出第 2 页快照上的那一期，供后续用例构造「已入库」状态。
//
// **不硬编码期次**：快照是 TASK-002 当天抓的，上面是哪一期取决于抓取日期。
// 写死 `2026-06/h1` 的测试会在下次更新快照时红，而红的理由与它要守的东西无关。
func targetOnPage2(t *testing.T) Candidate {
	t.Helper()
	items, err := scanPage(readTestdata(t, "pboc-index-p2.html"), testIndexURL)
	require.NoError(t, err)
	require.NotEmpty(t, items, "第 2 页快照里应有报告条目")
	return items[0]
}

// 空库时翻到第 2 页找到报告 —— 这是首跑的常态，也是「只抓第 1 页会漏」的证明。
//
// ⚠️ MaxPages 必须是 **2** 而不是计划写的 3：`twoPageFetcher` 只备两页，而空库下
// `HasPeriod` 恒 false ⇒ Discover 不会提前返回，会一路翻到 MaxPages。实测 MaxPages=3
// 时 fake 报「没有为 .../11040-3.html 准备页面」。
//
// **是测试写错，不是实现写错**：「空库翻满 MaxPages」正是 spec §4.3 定义的首跑行为。
// 若把 Discover 改成「发现候选就停」来让这条变绿，首跑就只能捡到最近一期、历史全漏。
func TestDiscoverFindsReportOnSecondPage(t *testing.T) {
	f, u2 := twoPageFetcher(t)
	want := targetOnPage2(t)

	got, _, err := Discover(context.Background(), f,
		fakeArticleChecker{have: map[string]bool{}},
		DiscoverCfg{IndexURL: testIndexURL, MaxPages: 2})
	require.NoError(t, err)

	require.NotEmpty(t, got)
	assert.Equal(t, want.Period, got[0].Period)
	assert.Equal(t, want.ArticleID, got[0].ArticleID)
	assert.Equal(t, []string{testIndexURL, u2}, f.calls, "应当只请求这两页")
}

// threePageFetcher 造一个三页站点：第 3 页复用 p1（无报告条目）。
// 三页是「空库翻满」与「命中即停」这对对照能分开的最小规模——两页时两者
// calls 都是 2，看不出区别。
func threePageFetcher(t *testing.T) *fakeFetcher {
	t.Helper()
	p1 := readTestdata(t, "pboc-index-p1.html")
	p2 := readTestdata(t, "pboc-index-p2.html")

	tmpl, _, err := parsePaging(p1)
	require.NoError(t, err)
	u2, err := pageURL(testIndexURL, tmpl, 2)
	require.NoError(t, err)
	u3, err := pageURL(testIndexURL, tmpl, 3)
	require.NoError(t, err)

	return &fakeFetcher{pages: map[string][]byte{testIndexURL: p1, u2: p2, u3: p1}}
}

// 「空库翻满 MaxPages」与「命中已入库期次即停」是**两个方向相反的行为**，
// 而它们**在返回值上可能完全相同**（都可能是一份候选清单）。本条把两者放进
// 同一个场景 —— 同样的三页站点、同样的 MaxPages=3，**只有 checker 不同** ——
// 并直接断言那个区别本身：calls 长度 3 vs 2。
//
// 这样写才能同时排除两种退化实现：
//   - 「恒翻满」的实现让 B 变成 3 ⇒ 红
//   - 「发现候选就停」的实现让 A 变成 2 ⇒ 红（且那会直接违反 spec §4.3：
//     首跑只捡到最近一期，历史全部漏掉）
//
// 分开写两条用例是不够的：各自单看都只是「calls 等于某个数」，
// 「两者必须不同」这个性质**不属于任何一条**。
func TestDiscoverEmptyStoreExhaustsWhileKnownStopsEarly(t *testing.T) {
	cfg := DiscoverCfg{IndexURL: testIndexURL, MaxPages: 3}
	target := targetOnPage2(t)

	// A：空库 —— 没有任何提前返回的条件，应翻满 MaxPages
	fa := threePageFetcher(t)
	gotA, stopA, err := Discover(context.Background(), fa,
		fakeArticleChecker{have: map[string]bool{}}, cfg)
	require.NoError(t, err)
	assert.Len(t, fa.calls, 3, "空库下 HasArticle 恒 false，应一路翻到 MaxPages（spec §4.3 首跑行为）")
	require.NotEmpty(t, gotA, "第 2 页上的那一期应当被发现")
	assert.Equal(t, StopMaxPages, stopA, "空库翻满上限，停止原因必须是 max_pages")

	// B：那一篇已见过 —— 在第 2 页命中，不该再翻第 3 页
	fb := threePageFetcher(t)
	gotB, stopB, err := Discover(context.Background(), fb,
		fakeArticleChecker{have: map[string]bool{target.ArticleID: true}}, cfg)
	require.NoError(t, err)
	assert.Len(t, fb.calls, 2, "第 2 页命中已见过的文章就该停")
	assert.Empty(t, gotB, "唯一的候选已见过，不该产出任何东西")
	assert.Equal(t, StopSeen, stopB, "命中已见过的文章而停，原因必须是 seen_article")

	// 🔴 TASK-011 的 error_handling[0]：两种停法的**候选清单都可能是空的**
	//（A 若把 MaxPages 设成 1 就是空），必须靠 StopReason 才分得开。
	assert.NotEqual(t, stopA, stopB, "「翻满上限」与「命中已见过」必须可区分")

	// 区别本身 —— 这一行才是本条的核心，前面两条各自都可能被退化实现满足
	assert.Greater(t, len(fa.calls), len(fb.calls),
		"「空库翻满」与「命中即停」必须在请求次数上可区分，只看返回值分不出来")
}

// 碰到已入库的期次立刻停，且**不再请求后续页**。
//
// 只断言返回值验不出这条：翻满上限的实现也会返回同样的空清单。必须看 calls。
// 与上一条的分工：这条把上限设得远高于实际页数（10），验的是「上限没被用满」；
// 上一条验的是「两种行为可区分」。
func TestDiscoverStopsAtKnownPeriod(t *testing.T) {
	f, _ := twoPageFetcher(t)
	target := targetOnPage2(t)

	got, _, err := Discover(context.Background(), f,
		fakeArticleChecker{have: map[string]bool{target.ArticleID: true}},
		DiscoverCfg{IndexURL: testIndexURL, MaxPages: 10})
	require.NoError(t, err)

	assert.Empty(t, got, "唯一的候选已入库，不该产出任何东西")
	assert.Len(t, f.calls, 2, "在第 2 页命中已知期次就该停，不该翻第 3 页")
}

// MaxPages 生效：设成 1 就只看第 1 页，而第 1 页没有报告。
func TestDiscoverRespectsMaxPages(t *testing.T) {
	f, _ := twoPageFetcher(t)

	got, _, err := Discover(context.Background(), f,
		fakeArticleChecker{have: map[string]bool{}},
		DiscoverCfg{IndexURL: testIndexURL, MaxPages: 1})
	require.NoError(t, err)

	assert.Empty(t, got)
	assert.Equal(t, []string{testIndexURL}, f.calls, "MaxPages=1 只该请求第 1 页")
}

// 总页数小于 MaxPages 时不越界请求。
func TestDiscoverDoesNotExceedTotalPages(t *testing.T) {
	// 把 p1 的 jumpTo 总页数改成 2，模拟一个只有两页的栏目
	p1 := readTestdata(t, "pboc-index-p1.html")
	tmpl, total, err := parsePaging(p1)
	require.NoError(t, err)
	shrunk := []byte(strings.Replace(string(p1),
		fmt.Sprintf("jumpTo(this,'%d'", total), "jumpTo(this,'2'", 1))
	require.NotEqual(t, string(p1), string(shrunk), "总页数替换必须真的生效，否则本条测的是原样的 408 页")

	u2, err := pageURL(testIndexURL, tmpl, 2)
	require.NoError(t, err)
	f := &fakeFetcher{pages: map[string][]byte{
		testIndexURL: shrunk,
		u2:           readTestdata(t, "pboc-index-p2.html"),
	}}

	_, _, err = Discover(context.Background(), f,
		fakeArticleChecker{have: map[string]bool{}},
		DiscoverCfg{IndexURL: testIndexURL, MaxPages: 50})
	require.NoError(t, err, "MaxPages=50 但总共只有 2 页，不该请求第 3 页")
	assert.Len(t, f.calls, 2)
}

// 同一期在两页上都出现时只产出一个候选。
//
// 真实成因：翻页期间有新文章上架，边界那条会被挤到下一页重复出现。
//
// ⚠️ `require.NotEmpty` 是**前置锚点，不是装饰**：本条的主断言是「遍历 got，每个期次
// 计数为 1」，而 got 为空时循环体一次都不执行 ⇒ 全部平凡通过。reviewer 消融实证过：
// 把 scanPage 改成恒返回 nil,nil，本条**仍然绿**（整包会被兄弟用例接住，但验证者按
// DoD 逐条单跑取证时会拿到假绿，并写进验收矩阵）。
func TestDiscoverDeduplicatesAcrossPages(t *testing.T) {
	p1 := readTestdata(t, "pboc-index-p1.html")
	p2 := readTestdata(t, "pboc-index-p2.html")
	tmpl, _, err := parsePaging(p1)
	require.NoError(t, err)
	u2, err := pageURL(testIndexURL, tmpl, 2)
	require.NoError(t, err)

	// 两页给同样的内容 —— 同一期出现两次
	f := &fakeFetcher{pages: map[string][]byte{testIndexURL: p2, u2: p2}}

	got, _, err := Discover(context.Background(), f,
		fakeArticleChecker{have: map[string]bool{}},
		DiscoverCfg{IndexURL: testIndexURL, MaxPages: 2})
	require.NoError(t, err)
	require.NotEmpty(t, got, "前置锚点：两页都喂 p2，至少该产出那一期；空集会让下面的计数断言平凡通过")
	assert.Len(t, f.calls, 2, "两页都要真的被请求过，否则「跨页去重」无从谈起")

	seen := map[string]int{}
	for _, c := range got {
		seen[c.Period+"/"+c.PeriodType]++
	}
	for k, n := range seen {
		assert.Equal(t, 1, n, "期次 %s 重复产出了 %d 次", k, n)
	}
}

// 查库失败必须中断，不能当成「未入库」继续翻 —— 那会把整段历史重新抓一遍。
func TestDiscoverFailsOnCheckerError(t *testing.T) {
	f, _ := twoPageFetcher(t)
	want := errors.New("database is locked")

	got, _, err := Discover(context.Background(), f,
		fakeArticleChecker{have: map[string]bool{}, err: want},
		DiscoverCfg{IndexURL: testIndexURL, MaxPages: 2})

	require.Error(t, err)
	assert.ErrorIs(t, err, want)
	assert.Nil(t, got)
	// ⚠️ 上面这句 Nil **证不出**「不返回部分结果」：checker 恒失败 ⇒ 第一次调用就返回
	// 错误，那一刻 out 本来就是空的，`return out, err` 与 `return nil, err` 返回值相同。
	// 真正守住这件事的是 TestDiscoverReturnsNoPartialResultOnCheckerError。
}

// failOnNthChecker 前 n 次正常返回「未入库」，第 n+1 次开始返回错误。
//
// 指针接收者是必须的：要跨调用计数。（fakeArticleChecker 是值接收者，复制一份就丢了计数。）
type failOnNthChecker struct {
	n    int
	seen int
	err  error
}

func (c *failOnNthChecker) HasArticleInObservations(_ context.Context, _ string) (bool, error) {
	c.seen++
	if c.seen > c.n {
		return false, c.err
	}
	return false, nil
}

// 查库失败发生在**已经收集到若干候选之后**时，仍然不得把那部分返回出去。
//
// ⚠️ 这条是消融逼出来的，而且是**同一个形态在本 Sprint 的第二次**：TASK-003 的
// `TestScanPageFailsOnUnresolvableURL` 也写着 `assert.Nil(got, "不得返回部分结果")`
// 却杀不掉 `return out, err` —— 因为它的场景里第一条就失败，out 本来就是空的。
// 我在 T3 修了那个实例，写本条上游的 TestDiscoverFailsOnCheckerError 时**又犯了一遍**。
// ⇒ 「错误路径断言 Nil」只有在**失败点之前已经收集过东西**时才有鉴别力。
//
// 为什么较真：返回「部分候选 + error」的调用方若只看结果不看 err，拿到的是一份
// 看起来正常、实则残缺的清单，缺掉的那些期次就此静默丢失 —— 而查库失败恰恰是
// 最可能被当成「偶发、重试就好」而忽略 err 的一类。
func TestDiscoverReturnsNoPartialResultOnCheckerError(t *testing.T) {
	// 合成一页两条报告 + 一个只有 1 页的分页控件：limit=1，一页内产出两个候选，
	// 让 checker 在第 2 个候选上失败 —— 此时 out 已有 1 条，两种写法才分得开。
	page := []byte(`
<a href="/goutongjiaoliu/113456/113469/2026011512340454111/index.html">2025年金融统计数据报告</a>
<a href="/goutongjiaoliu/113456/113469/2026011519015458333/index.html">2025年12月金融统计数据报告</a>
<a href="###" onclick="jumpTo(this,'1','1','/goutongjiaoliu/113456/113469/11040-%1.html')">尾页</a>`)

	// 前置自证：这一页确实产出两个候选，否则「第 2 个上失败」根本不成立
	items, err := scanPage(page, testIndexURL)
	require.NoError(t, err)
	require.Len(t, items, 2, "本条依赖同页两个候选，合成页面必须真的产出两条")

	want := errors.New("database is locked")
	f := &fakeFetcher{pages: map[string][]byte{testIndexURL: page}}

	got, _, err := Discover(context.Background(), f,
		&failOnNthChecker{n: 1, err: want},
		DiscoverCfg{IndexURL: testIndexURL, MaxPages: 5})

	require.Error(t, err)
	assert.ErrorIs(t, err, want)
	assert.Nil(t, got, "第 1 条已收进 out、第 2 条查库失败——那 1 条也不得返回出去")
}

// 抓页失败必须中断。
//
// ⚠️ `fakeFetcher.err` 这个字段计划里定义了却**从未被任何用例用过**（reviewer 指出），
// 即「抓页失败」这条路径原本零覆盖。两个子测试分别覆盖首页与翻页中途 ——
// 后者更要紧：翻页中途失败若被吞掉，Discover 会拿着**残缺的**候选清单正常返回，
// 而那一期就此静默丢失。
func TestDiscoverFailsOnFetchError(t *testing.T) {
	t.Run("第 1 页抓取失败", func(t *testing.T) {
		f, _ := twoPageFetcher(t)
		want := errors.New("connection refused")
		f.err = want

		got, _, err := Discover(context.Background(), f,
			fakeArticleChecker{have: map[string]bool{}},
			DiscoverCfg{IndexURL: testIndexURL, MaxPages: 2})

		require.Error(t, err)
		assert.ErrorIs(t, err, want, "底层抓取错误必须能被调用方识别出来")
		assert.Nil(t, got)
	})

	t.Run("翻页中途抓取失败", func(t *testing.T) {
		p1 := readTestdata(t, "pboc-index-p1.html")
		tmpl, _, err := parsePaging(p1)
		require.NoError(t, err)
		u2, err := pageURL(testIndexURL, tmpl, 2)
		require.NoError(t, err)

		// 只备第 1 页：翻到第 2 页时 fake 报错，且错误里带着那一页的 URL
		f := &fakeFetcher{pages: map[string][]byte{testIndexURL: p1}}

		got, _, err := Discover(context.Background(), f,
			fakeArticleChecker{have: map[string]bool{}},
			DiscoverCfg{IndexURL: testIndexURL, MaxPages: 2})

		require.Error(t, err, "翻页中途失败必须中断，不能拿着残缺清单正常返回")
		assert.Contains(t, err.Error(), u2,
			"错误要带出是哪一页挂了——Discover 会连续请求多页，不带 URL 就分不清")
		assert.Nil(t, got)
		assert.Len(t, f.calls, 2, "失败发生在第 2 页，说明确实翻到了那里")
	})
}

// 解析页面失败也必须中断。
//
// 这条路径在别的用例里都不可达（真实快照 + 正常 base 时 scanPage 从不出错），
// 所以专门构造：把 IndexURL 设成不可解析的串，页面本身照常喂 —— 抓取成功、
// 分页解析成功，卡在 scanPage 补全条目 URL 那一步。
//
// 不测它的话，Discover 里那句 `if err != nil { return nil, err }` 就是没人走过的代码，
// 改成 `continue` 也不会有任何东西变红 —— 而那意味着整页候选被静默丢弃。
func TestDiscoverFailsOnScanError(t *testing.T) {
	const badBase = "://nope"
	f := &fakeFetcher{pages: map[string][]byte{
		badBase: readTestdata(t, "pboc-index-p2.html"),
	}}

	got, _, err := Discover(context.Background(), f,
		fakeArticleChecker{have: map[string]bool{}},
		DiscoverCfg{IndexURL: badBase, MaxPages: 1})

	require.Error(t, err, "条目 URL 补全失败必须中断，不得静默丢弃整页候选")
	assert.Contains(t, err.Error(), "bad base url")
	assert.Nil(t, got)
	require.NotNil(t, errors.Unwrap(err), "底层 url.Parse 的错误必须被 %%w 包住")
}

// 分页控件解析不出来时，Discover 必须把 parsePaging 的错误透传出去。
//
// T2 让 parsePaging 在解析不到分页控件时报错，理由是「不得静默退化成只扫第 1 页」。
// 那条纪律要在 Discover 这一层才真正兑现：若这里把错误吞掉、拿第 1 页的条目继续，
// 退化就照样发生了 —— 只是发生在调用方看不见的地方。
func TestDiscoverFailsOnPagingParseError(t *testing.T) {
	f := &fakeFetcher{pages: map[string][]byte{
		testIndexURL: []byte("<html><body>改版了，没有分页控件</body></html>"),
	}}

	got, _, err := Discover(context.Background(), f,
		fakeArticleChecker{have: map[string]bool{}},
		DiscoverCfg{IndexURL: testIndexURL, MaxPages: 5})

	require.Error(t, err, "解析不到分页控件必须中断，不得退化成只扫第 1 页")
	assert.Contains(t, err.Error(), "paging")
	assert.Nil(t, got)
	assert.Len(t, f.calls, 1, "第 1 页就解析失败，不该再请求任何页")
}

// 拼下一页 URL 失败也必须中断。
//
// 与上一条（scanPage 失败）是 Discover 里两条不同的错误出口，成因也不同：这条坏的是
// **页面里解析出来的模板**，而不是调用方给的 base。构造法：把快照 jumpTo 里的模板塞
// 一个控制字符 —— 第 1 页照常解析（p1 本来就没有报告），翻第 2 页时 pageURL 才炸。
//
// 若这里改成「break 掉、把已有的返回」，Discover 会拿着**只扫了第 1 页**的结果正常返回，
// 而第 1 页恰恰是常态没有报告的那一页 —— 管线看起来在跑，实际什么都发现不了。
func TestDiscoverFailsOnPageURLError(t *testing.T) {
	p1 := readTestdata(t, "pboc-index-p1.html")
	tmpl, _, err := parsePaging(p1)
	require.NoError(t, err)

	// ⚠️ 用 pagingRE 自己定位再改，别拿 tmpl 直接 Replace：实测该模板串在 p1 里
	// **出现两次**，靠前的那处是 `jumpToPage(event,this,…)` 的 onkeydown（pagingRE
	// 并不匹配它），`Replace(…, 1)` 会改中它，而 jumpTo 那处原封不动 ——
	// 结果是「变异没生效但测试照样红」，红在 fake 缺页上，与本条要守的东西无关。
	loc := pagingRE.FindIndex(p1)
	require.NotNil(t, loc, "p1 里应当有 jumpTo 分页控件")
	seg := strings.Replace(string(p1[loc[0]:loc[1]]), tmpl, tmpl+"\x7f", 1)
	broken := append(append(append([]byte{}, p1[:loc[0]]...), seg...), p1[loc[1]:]...)

	// 自证变异确实作用到了**被解析的那一处**，而不是页面上别处的同名串
	brokenTmpl, _, err := parsePaging(broken)
	require.NoError(t, err)
	require.NotEqual(t, tmpl, brokenTmpl, "解析出的模板必须已被改坏，否则本条测的是好模板")

	f := &fakeFetcher{pages: map[string][]byte{testIndexURL: broken}}

	got, _, err := Discover(context.Background(), f,
		fakeArticleChecker{have: map[string]bool{}},
		DiscoverCfg{IndexURL: testIndexURL, MaxPages: 2})

	require.Error(t, err, "拼不出下一页 URL 必须中断，不得只扫第 1 页就正常返回")
	assert.Contains(t, err.Error(), "bad url")
	assert.Nil(t, got)
	assert.Len(t, f.calls, 1, "第 2 页的 URL 都没拼出来，不该发出第二次请求")
}

// Discover 全程不碰文章页。取正文是 4b 的事。
func TestDiscoverNeverFetchesArticlePages(t *testing.T) {
	f, _ := twoPageFetcher(t)
	got, _, err := Discover(context.Background(), f,
		fakeArticleChecker{have: map[string]bool{}},
		DiscoverCfg{IndexURL: testIndexURL, MaxPages: 2})
	require.NoError(t, err)
	require.NotEmpty(t, got, "前置条件：应当发现了候选（空集会让下面的遍历平凡通过）")

	for _, c := range got {
		assert.NotContains(t, f.calls, c.URL,
			"候选的文章 URL 不该被请求：Discover 只发现，不取正文")
	}
}

// 🔴 TASK-011 的存在理由：**已入库期次排在前面，会让它之后的所有未入库期次静默消失。**
//
// 用两份真实快照拼一个站点：p7 上是 2026-03/q1（**假设已入库**），p2 上是 2026-06/h1
// （未入库）。真实央行页面正是这个形态——index 按**发布时间**倒序，而修订版重发时
// 发布时间最新、期次却是旧的，于是「已入库的旧期次」会排到「未入库的新期次」前面。
//
// 判停注释写的理由是「index 按时间倒序，再往后只会更旧」——它把**发布时间倒序**
// 和**期次倒序**当成了同一件事。这两者在重发/修订时分叉。
func TestDiscoverDoesNotStopAtKnownPeriodAheadOfUnknownOne(t *testing.T) {
	p7 := readTestdata(t, "pboc-index-p7.html")
	p2 := readTestdata(t, "pboc-index-p2.html")

	tmpl, _, err := parsePaging(p7)
	require.NoError(t, err)
	u2, err := pageURL(testIndexURL, tmpl, 2)
	require.NoError(t, err)
	f := &fakeFetcher{pages: map[string][]byte{testIndexURL: p7, u2: p2}}

	// 前置锚点：确认两份快照确实是「第 1 页一期、第 2 页另一期」的形态，
	// 否则下面断言的空集可能来自快照本身没有条目，而不是判停规则。
	first, err := scanPage(p7, testIndexURL)
	require.NoError(t, err)
	require.Len(t, first, 1, "p7 上应恰有一期")
	second, err := scanPage(p2, testIndexURL)
	require.NoError(t, err)
	require.Len(t, second, 1, "p2 上应恰有一期")
	require.NotEqual(t, first[0].Period, second[0].Period, "两页必须是不同期次")

	got, _, err := Discover(context.Background(), f,
		fakeArticleChecker{
			// 🔴 这正是「修订版」的形态，也是本用例的全部要害：
			//   - 期次 2026-03/q1 **已入库**（havePeriod 命中）
			//   - 但这一篇的 article_id 是**新的**（have 不命中）——重发产生新 URL
			// 按期次判停 ⇒ 第一条就停 ⇒ 第 2 页那期静默消失。
			// 按 article_id 判停 ⇒ 不停 ⇒ 继续翻，第 2 页那期被发现。
			have:       map[string]bool{},
			havePeriod: map[string]bool{first[0].Period + "/" + first[0].PeriodType: true},
		},
		DiscoverCfg{IndexURL: testIndexURL, MaxPages: 2})

	require.NoError(t, err)

	// 🔴 这几条是本任务的全部理由。
	require.NotEmpty(t, got, "第 1 页那期已入库，不该让第 2 页那期跟着消失")
	assert.Len(t, f.calls, 2, "必须翻到第 2 页，而不是停在第 1 页")

	var periods []string
	for _, c := range got {
		periods = append(periods, c.Period)
	}
	// 受害者：它在第 1 页那条之后，旧实现里整个消失。
	assert.Containsf(t, periods, second[0].Period,
		"第 2 页那期未入库的必须被发现，得到 %v", periods)
	// 修订版自己也该被收进来 —— 它有新的 article_id，就是没见过的东西。
	// ⚠️ 断言的是 Contains 而不是 got[0]：**顺序不是本用例要守的性质**，
	// 写成 got[0] 会让它同时绑死「谁排第一」，那是另一条测试的事。
	assert.Containsf(t, periods, first[0].Period,
		"修订版有新 article_id，本身也是没见过的，应当一并收进候选，得到 %v", periods)
}

// 🔴 TASK-011 boundary[0]：**站点迁移后一轮自愈**，用真库实证而不是推理。
//
// 换成 article_id 判停时，`discover.go` 原有一段带 M0 实测支撑的反论证挡在前面：
//
//	按 article_id 判停，一次迁移后全部 id 变新，**每次唤起都会翻满上限**，
//	且每期都被当成新文章。
//
// 那段论证的前半是对的（迁移后第一轮确实全 miss、确实翻满），**错在「每次」**。
// 本用例证明代价只发生一轮：
//
//	第 1 轮：库里存**旧** id ⇒ HasArticle(新 id) miss ⇒ 不停 ⇒ 翻满 ⇒ 候选被交出来
//	  ↓ 走一次 Save（模拟 ingest）
//	Save 的 Lookup 按业务键 {period, period_type} 查 —— **article_id 不在业务键里**
//	（store.go 的 NewSpec），迁移不改 published_at ⇒ Classify 判 Duplicate
//	  ↓ Duplicate 分支调 refreshArticleID，把那一行的 article_id 刷成新的
//	第 2 轮：HasArticle(新 id) 命中 ⇒ **当场停**，恢复正常
//
// ⚠️ **链条里最脆的一环是「Lookup 会不会把新 id 当成另一个业务键」** —— 若 article_id
// 在业务键里，第 1 轮就会判 New、id 永不刷新，「一轮自愈」退化成「每轮全量重抓」，
// 也就是那段反论证担心的东西换了个位置发生。本用例的第 1 轮断言 Verdict 就是钉它。
func TestDiscoverSelfHealsAfterSiteMigration(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	f, _ := twoPageFetcher(t)
	target := targetOnPage2(t) // 第 2 页快照上那一期，article_id 取自快照（= 迁移后的「新」id）
	cfg := DiscoverCfg{IndexURL: testIndexURL, MaxPages: 2}

	const oldID = "2025092212550713215" // 迁移前的旧 id（M0 实测的那种时间戳形态）
	require.NotEqual(t, oldID, target.ArticleID, "前置锚点：新旧 id 必须不同，否则本用例什么都没测")

	// 库里那一行存的是**旧** id —— 这就是「站点迁移之后」的状态。
	seed := Observation{
		Meta: Meta{
			Period: target.Period, PeriodType: target.PeriodType,
			PublishedAt: "2026-07-15", ArticleID: oldID,
			CaliberVersion: "2025-01", Extractor: "rule@v2",
		},
		Values: map[string]float64{FieldM2: 300},
	}
	_, err := s.Save(ctx, seed, passing())
	require.NoError(t, err)

	// —— 第 1 轮：miss ⇒ 不停 ⇒ 翻满 ——
	got1, stop1, err := Discover(ctx, f, s, cfg)
	require.NoError(t, err)
	require.NotEmpty(t, got1, "新 id 没见过，这一期必须被交出来")
	assert.Equal(t, StopMaxPages, stop1, "迁移后第一轮没有任何 id 命中，应翻满上限")
	assert.Len(t, f.calls, 2, "翻满 MaxPages=2")

	var found *Candidate
	for i := range got1 {
		if got1[i].Period == target.Period && got1[i].PeriodType == target.PeriodType {
			found = &got1[i]
		}
	}
	require.NotNil(t, found, "那一期必须在候选里")
	require.Equal(t, target.ArticleID, found.ArticleID, "候选带的是 index 上的新 id")

	// —— 模拟 ingest 走一次 Save：这是自愈真正发生的地方 ——
	migrated := seed
	migrated.Meta.ArticleID = found.ArticleID // 只有 id 变了，期次与发布日都没变
	out, err := s.Save(ctx, migrated, passing())
	require.NoError(t, err)
	// 🔴 最脆的一环：必须是 Duplicate。判 New 意味着 article_id 进了业务键，
	// 那样 id 永不刷新，「一轮自愈」就退化成「每轮全量重抓」。
	require.Equal(t, bitemporal.Duplicate, out.Verdict,
		"同期次 + 新 article_id 必须判 Duplicate —— 这一环不成立的话整条自愈链都不成立")
	assert.Equal(t, TableObservations, out.Table, "Duplicate 只刷 id，不写 pending")

	// id 确实被刷新了（不看 Save 的返回值，直接问库）
	has, err := s.HasArticle(ctx, found.ArticleID)
	require.NoError(t, err)
	require.True(t, has, "refreshArticleID 应当已把那一行的 id 刷成新的")

	// —— 第 2 轮：命中 ⇒ 当场停 ——
	f2, _ := twoPageFetcher(t)
	got2, stop2, err := Discover(ctx, f2, s, cfg)
	require.NoError(t, err)
	assert.Equal(t, StopSeen, stop2, "第 2 轮应当命中已见过的文章而停")
	assert.Empty(t, got2, "没有新东西了")
	assert.Len(t, f2.calls, 2,
		"p1 无报告条目故仍会翻到 p2，在 p2 命中即停 —— 关键是 stop2 从 max_pages 变成了 seen_article")
}

// 去重键必须与判停键**是同一把标识**（都用 article_id），否则判停会漏判。
//
// 场景：同一期的**两篇**同时在榜 —— 修订版（新 id，没见过）排在前，原文（旧 id，
// 已见过）紧随其后。这在重发那几天是真实形态。
//
//   - 按 article_id 去重：原文那条进得了 has 检查 ⇒ 命中 ⇒ **停**（StopSeen）
//   - 按期次去重：修订版已把 `期次` 这个键占上 ⇒ 原文在**查库之前**就被 continue
//     掉 ⇒ 那次命中根本没发生 ⇒ 一路翻到上限（StopMaxPages）
//
// ⚠️ 本条是补上去的：消融实测发现把去重键改回期次时**没有任何断言变红** ——
// 我在 discover.go 那句「用期次去重会让判停漏判」当时只是一句没人守的声明。
// 现在它由 StopReason 这个可观测差异钉住。
//
// 后果不是数据错，而是**每次唤起都白翻满上限**（判停失效 ⇒ 永远走不到提前返回）。
func TestDiscoverDedupKeyMatchesStopKey(t *testing.T) {
	const seenID, freshID = "5868082", "5999999"
	page := []byte(`
<a href="/goutongjiaoliu/113456/113469/` + freshID + `/index.html">2025年前三季度金融统计数据报告</a>
<a href="/goutongjiaoliu/113456/113469/` + seenID + `/index.html">2025年前三季度金融统计数据报告</a>
<a href="###" onclick="jumpTo(this,'9','1','/goutongjiaoliu/113456/113469/11040-%1.html')">尾页</a>
`)
	// 前置锚点：两条必须同期次、不同 id —— 场景本身要成立。
	items, err := scanPage(page, testIndexURL)
	require.NoError(t, err)
	require.Len(t, items, 2, "合成页上应有两条同期次的报告")
	require.Equal(t, items[0].Period, items[1].Period, "两条必须是同一期")
	require.NotEqual(t, items[0].ArticleID, items[1].ArticleID, "两条必须是不同的文章")

	f := &fakeFetcher{pages: map[string][]byte{testIndexURL: page}}
	got, stop, err := Discover(context.Background(), f,
		fakeArticleChecker{have: map[string]bool{seenID: true}},
		DiscoverCfg{IndexURL: testIndexURL, MaxPages: 1})
	require.NoError(t, err)

	assert.Equal(t, StopSeen, stop,
		"原文那条已见过，必须走到 has 检查并触发停止；按期次去重会让它被跳过 ⇒ 变成 max_pages")
	require.Len(t, got, 1, "只有修订版那条是没见过的")
	assert.Equal(t, freshID, got[0].ArticleID)
}

// 🔴 人类 2026-08-13 裁决 (B) 的**行为保证**：pending 里的期次必须仍被交出来。
//
// 判停若用 `HasArticle`（查两表），pending 那篇会让 Discover 当场停 ⇒ 那一期
// **再也不会被自动重试**：修了校验 bug 或调了阈值之后它不会自己回来，只能靠 --force。
// 而 Sprint 035 已登记「落 pending 的期次对依赖历史的闸门永久不可见」是结构性问题。
//
// 用 `HasArticleInObservations`（只查权威表）则：pending 那篇不命中 ⇒ 不停 ⇒ 候选
// 照常交出 ⇒ 由 ingest 层的 `HasArticle` 挡住并打印 already ingested。**两层各司其职。**
//
// ⚠️ 这条与 store_test.go 的 TestHasArticleInObservationsIgnoresPending 是**互补的一对**：
// 那条钉「两个方法对同一行答案相反」，本条钉「Discover 用的是不认 pending 的那个」。
// 只有那条时，把 Discover 改回 HasArticle 不会有任何东西红。
func TestDiscoverStillYieldsPendingPeriods(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	f, _ := twoPageFetcher(t)
	target := targetOnPage2(t)

	// 那一期落 pending，article_id 与 index 上的**完全一致**（即「上一轮抓过、没过闸」）
	_, err := s.Save(ctx, Observation{
		Meta: Meta{
			Period: target.Period, PeriodType: target.PeriodType, PublishedAt: "2026-07-15",
			ArticleID: target.ArticleID, CaliberVersion: "2025-01", Extractor: "rule@v2",
		},
		Values: map[string]float64{FieldM2: 300},
	}, failing())
	require.NoError(t, err)

	// 前置锚点：确认它确实只在 pending 里 —— 否则本用例测的是另一件事。
	inBoth, err := s.HasArticle(ctx, target.ArticleID)
	require.NoError(t, err)
	require.True(t, inBoth, "前置：这篇应当在 pending 里")
	inObs, err := s.HasArticleInObservations(ctx, target.ArticleID)
	require.NoError(t, err)
	require.False(t, inObs, "前置：它不该在权威表里")

	got, stop, err := Discover(ctx, f, s, DiscoverCfg{IndexURL: testIndexURL, MaxPages: 2})
	require.NoError(t, err)

	require.NotEmpty(t, got, "pending 里的期次必须仍被交出来，否则它永远等不到重试")
	assert.Equal(t, target.ArticleID, got[0].ArticleID)
	assert.Equal(t, StopMaxPages, stop, "没有任何 id 在权威表里，应翻满上限而不是提前停")
}

// 🔴 `StopExhausted` 此前**没有任何测试断言过**（QA WARNING-1 实测：`grep StopExhausted`
// 全仓仅 3 处，全在 discover.go 的注释/定义/返回）。
//
// 它与 `StopMaxPages` 的区别是运维要的：**exhausted = 站点翻完了，窗口外没有东西了；
// max_pages = 上限拦住了，窗口外可能还有**。两者都返回「没更多候选」，只有 StopReason 分得开。
func TestDiscoverReportsExhaustedWhenSiteIsShorterThanMaxPages(t *testing.T) {
	// 合成站点只有 1 页（分页控件写 totalPages=1），而 MaxPages 要 3 ⇒ 翻完为止。
	page := syntheticIndex(t, indexEntry{annualID, annualTitle})
	f := &fakeFetcher{pages: map[string][]byte{testIndexURL: page}}

	got, stop, err := Discover(context.Background(), f,
		fakeArticleChecker{have: map[string]bool{}},
		DiscoverCfg{IndexURL: testIndexURL, MaxPages: 3})
	require.NoError(t, err)

	require.NotEmpty(t, got, "前置锚点：这一页上有一条报告，空集会让下面的断言失去意义")
	assert.Equal(t, StopExhausted, stop,
		"站点只有 1 页而 MaxPages=3 ⇒ 是「翻完了」不是「被上限拦住」")
	assert.Len(t, f.calls, 1, "只有 1 页可翻")
}

// 🔴 `Ingest` 必须把停止原因**说出来**，而且在**有候选时**也说（QA WARNING-1）。
//
// 原先它只在 `len(cands) == 0` 时打印 ⇒ 而 **`max_pages` 且有候选恰恰是主要形态**
// （空库首跑必然如此）。那一轮之后 MaxPages 以外的历史**永久不可达**，
// 而这条信息**在唯一会发生它的那一轮被静默吞掉**、退出码 0。
//
// ⚠️ 本条放在 discover_test.go 而不是 ingest_test.go：`ingest_test.go` 属 TASK-007 的
// writes（两个返工任务同时在途，同一文件会撞 scope-mutex），而这条断言的对象是
// `StopReason` 这个 discover 侧的契约**是否到达了运维**，放在它旁边也说得通。
func TestIngestReportsStopReasonEvenWithCandidates(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	f := &fakeFetcher{pages: map[string][]byte{
		testIndexURL:         syntheticIndex(t, indexEntry{annualID, annualTitle}),
		articleURL(annualID): readTestdata(t, annualFile),
	}}
	var out bytes.Buffer
	require.NoError(t, Ingest(ctx, IngestDeps{Store: s, Fetch: f, Out: &out, Cfg: ingestCfg(t)}))

	// 前置锚点：这一轮确实处理了候选（否则走的是 len(cands)==0 那条旧分支）。
	require.NotEmpty(t, outPeriods(out.String()), "本用例要的是「有候选」那条路径")
	assert.Contains(t, out.String(), "discover stopped:",
		"有候选时也必须说明为何停 —— 否则 max_pages 这个「窗口外可能还有」的信号被吞掉")
	assert.Contains(t, out.String(), string(StopExhausted),
		"合成站点只有 1 页，应当报 exhausted")
}

// 🔴 **日常最常见的那一轮**：什么都没有新的，Discover 命中已见过的文章就停。
//
// ⚠️ 这条路径**在 TASK-011 之后一度没有任何测试**：判停键从期次换成 article_id 之后，
// 原先唯一走到它的 `TestIngestSkipsSeenArticleUnlessForce/默认跳过` 改走了「候选交出来、
// 由 ingestOne 挡住」那条（pending 里的文章不再让 Discover 停）⇒ `len(cands)==0`
// 那个分支变成**未覆盖**，而它恰恰是**一个月里 28 天都会走的那条**。
//
// 覆盖率把这件事显出来了：加 cwd 守卫后 ingest.go 的 Ingest 掉到 89.7%，
// 逐块看才发现少的不是新代码，是这条老路径。
func TestIngestReportsNothingNewOnSecondRun(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	// 第 1 轮：正常入库
	var first bytes.Buffer
	require.NoError(t, Ingest(ctx, IngestDeps{
		Store: s, Fetch: annualFetcher(t), Out: &first, Cfg: ingestCfg(t),
	}))
	require.NotEmpty(t, outPeriods(first.String()), "前置锚点：第 1 轮应当真的处理了一期")

	// 第 2 轮：同一篇已在权威表 ⇒ Discover 命中即停 ⇒ 零候选
	var out bytes.Buffer
	require.NoError(t, Ingest(ctx, IngestDeps{
		Store: s, Fetch: annualFetcher(t), Out: &out, Cfg: ingestCfg(t),
	}))

	assert.Empty(t, outPeriods(out.String()), "没有新东西可处理")
	assert.Contains(t, out.String(), "no new reports")
	assert.Contains(t, out.String(), string(StopSeen),
		"必须说明是「命中已见过的文章」而停，不是「翻满上限」—— 两者在这一行上同形")
}
