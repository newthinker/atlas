package hestia

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
// 映射与 types.go 的 periodEndMonth 一致（h1→06、annual→12），不新增第二份约定。
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
		assert.Regexp(t, `^\d{14,}$`, c.ArticleID, "article_id 是 URL 里的长数字串")
		assert.True(t, strings.HasPrefix(c.URL, "https://www.pbc.gov.cn/"),
			"站内绝对路径要补成完整 URL，得到 %s", c.URL)
		assert.Regexp(t, `^\d{4}-\d{2}$`, c.Period)
		assert.Contains(t, []string{"monthly", "h1", "annual"}, c.PeriodType)

		// 字段断言覆盖**每一条**而不只是 got[0]：只断言首条时，一个「第一条算对、
		// 其余瞎填」的实现照样全绿，而 Discover 会把整页候选都交给下游。
		for i, c := range got {
			assert.Regexp(t, `^\d{14,}$`, c.ArticleID, "第 %d 条", i)
			assert.True(t, strings.HasPrefix(c.URL, "https://www.pbc.gov.cn/"), "第 %d 条", i)
			assert.Regexp(t, `^\d{4}-\d{2}$`, c.Period, "第 %d 条", i)
			assert.Contains(t, []string{"monthly", "h1", "annual"}, c.PeriodType, "第 %d 条", i)
		}
	})

	t.Run("同页的干扰项不被收进来", func(t *testing.T) {
		got, err := scanPage(readTestdata(t, "pboc-index-p2.html"), base)
		require.NoError(t, err)
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

	for _, page := range []string{"pboc-index-p1.html", "pboc-index-p2.html"} {
		t.Run(page, func(t *testing.T) {
			ms := articleLinkRE.FindAllSubmatch(readTestdata(t, page), -1)
			assert.Len(t, ms, 15, "每页 15 条新闻，提取器应无漏无重")
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
