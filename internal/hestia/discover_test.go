package hestia

import (
	"errors"
	"os"
	"path/filepath"
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
func TestPageURLRejectsUnparsableInput(t *testing.T) {
	const goodTmpl = "/goutongjiaoliu/113456/113469/11040-%1.html"

	t.Run("index url 不可解析", func(t *testing.T) {
		_, err := pageURL("://nope", goodTmpl, 2)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "bad index url")
		require.NotNil(t, errors.Unwrap(err), "底层 url.Parse 的错误必须被 %%w 包住")
	})

	t.Run("模板不可解析", func(t *testing.T) {
		_, err := pageURL("https://www.pbc.gov.cn/a/index.html", "/a/b-%1\x7f.html", 2)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "bad paging template")
		require.NotNil(t, errors.Unwrap(err), "底层 url.Parse 的错误必须被 %%w 包住")
	})
}
