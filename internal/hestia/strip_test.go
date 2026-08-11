package hestia

// Context Checkpoint: done_criteria → test mapping (strip)
// functional[0]      块级标签成为段落边界，不粘连          → TestStripKeepsBlockBoundaries
//                    内联标签删除，不在数值中插入空白      → TestStripRejoinsValueSplitByInlineTags
//                    script/style 整段删除                 → TestStripDropsScriptAndStyle
//                    html.UnescapeString 还原实体          → TestStripUnescapesEntities
//                    实体还原须在标签剥离之后（变异 M10 补）→ TestStripUnescapesAfterTagRemoval
// functional[1]      metaContent 取值 + 「不存在」与「存在但为空」区分
//                                                          → TestMetaContent / TestMetaContentMissing
//                                                            / TestMetaContentPresentButEmpty
// boundary[0]        F6 回归钉：4825</span>…<span>亿元 → 4825亿元
//                                                          → TestStripRejoinsValueSplitByInlineTags
// error_handling[0]  半角 , ; 归一为全角（2020 样本 2 逗号 1 分号，逐串断言）
//                                                          → TestStripNormalizesPunctuation
//                                                            / TestStripNormalizesPunctuationInRealSample
// non_functional[0]  纯函数：同一输入连续两次结果相同      → TestStripAndMetaAreDeterministic
// non_functional[1]  真实样本剥离后含锚点句且无残留标签    → TestStripRealSamples
//                    块级边界在真实样本上支撑板块切分（6/8）→ TestStripRealSamplesKeepSectionAnchors

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func readSample(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	require.NoError(t, err)
	return b
}

// TestStripRejoinsValueSplitByInlineTags 是 F6 的回归钉。
//
// raw 逐字抄自 pboc-2025-12-annual.html：「4825」与「亿元」之间隔着六层 span。
// 若把所有标签都替换成换行（最直觉的写法），单位会掉到下一行，而 extract 的
// 「单位必须捕获否则报错」会因此误报——数据其实是完整的，只是标记切断了它。
//
// 断言必须钉在「同比多4825亿元」这个具体串上：只测「标签被去掉了」的写法
// 允许实现在数值与单位之间插入空白/换行，而那正是本条要防的失效。
func TestStripRejoinsValueSplitByInlineTags(t *testing.T) {
	raw := []byte(`<p>企业债券净融资2.39万亿元，同比多4825</span></span></span>` +
		`<span><span><span>亿元</span></span></span><span><span><span>；</span></span></span>` +
		`<span><span><span>政府债券净融资13.84万亿元</span></span></span></p>`)

	got := stripHTML(raw)

	assert.Contains(t, got, "同比多4825亿元", "内联标签不得把数值与单位切开")
	assert.Contains(t, got, "；政府债券净融资13.84万亿元", "内联标签不得把分句切开")
}

// TestStripKeepsBlockBoundaries 与上一条互为反向约束：内联标签不得产生分隔，
// 块级标签则必须产生分隔——两类语义不同，混为一谈会让其中一条失效。
func TestStripKeepsBlockBoundaries(t *testing.T) {
	// 块级标签是真实的段落边界，删掉会把不同板块粘连成一行，
	// 让「^[一-十]、」的板块切分失效
	raw := []byte(`<p>一、广义货币增长8.5%</p><p>二、全年人民币存款增加26.41万亿元</p>`)
	got := stripHTML(raw)

	lines := nonEmptyLines(got)
	require.Len(t, lines, 2, "两个 <p> 应产生两行，而不是粘成一行")
	assert.Equal(t, "一、广义货币增长8.5%", lines[0])
	assert.Equal(t, "二、全年人民币存款增加26.41万亿元", lines[1])
}

func TestStripNormalizesPunctuation(t *testing.T) {
	// 2020 样本混用半角 , 与 ; 。归一在 strip 里一次完成，
	// 后面的模板就不必每条都写两种标点。
	raw := []byte(`<p>余额213.49万亿元,同比增长11.1%;流通中货币</p>`)
	got := stripHTML(raw)

	assert.Contains(t, got, "万亿元，同比")
	assert.Contains(t, got, "11.1%；流通中货币")
	assert.NotContains(t, got, ",", "半角逗号应已归一")
	assert.NotContains(t, got, ";", "半角分号应已归一")
}

// TestStripNormalizesPunctuationInRealSample 把归一钉在 2020 样本的三处真实
// 混用上（2 个半角逗号、1 个半角分号，位置逐一实测确认）。
//
// 不做归一，M2/M0 两组四个字段会因模板只写全角标点而直接失配——失配处离
// 根因很远，所以判据放在这里，且必须是具体串。
func TestStripNormalizesPunctuationInRealSample(t *testing.T) {
	got := stripHTML(readSample(t, "pboc-2020-06-h1.html"))

	assert.Contains(t, got, "广义货币(M2)余额213.49万亿元，同比增长11.1%")
	assert.Contains(t, got, "流通中货币(M0)余额7.95万亿元，同比增长9.5%")
	assert.Contains(t, got, "票据融资增加9697亿元；非银行业金融机构贷款减少2775亿元")

	assert.NotContains(t, got, ",", "正文不应残留半角逗号")
	assert.NotContains(t, got, ";", "正文不应残留半角分号")
}

func TestStripDropsScriptAndStyle(t *testing.T) {
	// script/style 内含大量数字与标点，不删会污染板块切分
	raw := []byte(`<p>正文1.5万亿元</p><script>var x = 999;</script>` +
		`<style>.c{width:888px}</style><p>正文二</p>`)
	got := stripHTML(raw)

	assert.Contains(t, got, "正文1.5万亿元")
	assert.Contains(t, got, "正文二")
	assert.NotContains(t, got, "999")
	assert.NotContains(t, got, "888")
}

func TestStripUnescapesEntities(t *testing.T) {
	raw := []byte(`<p>企（事）业&nbsp;单位贷款&amp;票据</p>`)
	got := stripHTML(raw)
	assert.Contains(t, got, "单位贷款&票据") // &amp; 已还原且未插入空白
	assert.NotContains(t, got, "&nbsp;")
	assert.NotContains(t, got, "&amp;")
}

// TestStripUnescapesAfterTagRemoval 钉住实体还原与标签剥离的**先后顺序**。
//
// 顺序反过来（先还原再剥标签），正文里被转义的 &lt;p&gt; 会先变成真的 <p>，
// 再被当成块级标签吃掉——原文一段字符静默消失，且两份样本都没有实体，
// 真实数据永远暴露不出来。这条是变异测试 M10 存活后补的。
func TestStripUnescapesAfterTagRemoval(t *testing.T) {
	raw := []byte(`<p>正文里出现 a &lt;p&gt; b 这种转义写法</p>`)
	got := stripHTML(raw)

	assert.Contains(t, got, "a <p> b", "还原出的 <p> 不应再被当成标签剥掉")
}

func TestMetaContent(t *testing.T) {
	for _, tc := range []struct {
		file, pubDate, title string
	}{
		{"pboc-2020-06-h1.html", "2020-07-10", "2020年上半年金融统计数据报告"},
		{"pboc-2025-12-annual.html", "2026-01-15", "2025年金融统计数据报告"},
	} {
		t.Run(tc.file, func(t *testing.T) {
			raw := readSample(t, tc.file)

			pub, ok := metaContent(raw, "PubDate")
			require.True(t, ok, "PubDate 是 published_at 的唯一可信来源")
			assert.Equal(t, tc.pubDate, pub)

			title, ok := metaContent(raw, "ArticleTitle")
			require.True(t, ok)
			assert.Equal(t, tc.title, title)

			// createDate 在两份样本里都写成 `<meta  name="createDate"`（name 前两个
			// 空格），顺带钉住属性间空白不规则时仍能取到值。
			created, ok := metaContent(raw, "createDate")
			require.True(t, ok)
			require.GreaterOrEqual(t, len(created), 10)
			assert.NotEqual(t, pub, created[:10],
				"createDate 是 CMS 记录时间，不得用作 published_at")
		})
	}
}

func TestMetaContentMissing(t *testing.T) {
	_, ok := metaContent([]byte(`<html><head></head></html>`), "PubDate")
	assert.False(t, ok)
}

// TestMetaContentPresentButEmpty 钉住第二返回值的全部信息量：
// 「不存在」与「存在但为空」必须可区分，否则调用方只能把空串一律当缺失，
// 分不清是站点没填还是选择器写错了。
//
// SiteDomain 在两份样本里都是 content=""，是真实的空值样本。
func TestMetaContentPresentButEmpty(t *testing.T) {
	for _, f := range []string{"pboc-2020-06-h1.html", "pboc-2025-12-annual.html"} {
		t.Run(f, func(t *testing.T) {
			v, ok := metaContent(readSample(t, f), "SiteDomain")
			assert.True(t, ok, "SiteDomain 存在，只是内容为空")
			assert.Equal(t, "", v)
		})
	}
}

func TestStripRealSamples(t *testing.T) {
	for _, tc := range []struct {
		file    string
		anchors []string
	}{
		// 2020 上半年报告没有社融板块（CONTRACTS H8），锚点句按样本各取所有
		{"pboc-2020-06-h1.html", []string{"广义货币", "人民币贷款", "人民币存款余额"}},
		{"pboc-2025-12-annual.html", []string{"广义货币", "人民币贷款", "人民币存款余额", "社会融资规模"}},
	} {
		t.Run(tc.file, func(t *testing.T) {
			got := stripHTML(readSample(t, tc.file))

			assert.NotContains(t, got, "<", "不应残留标签")
			for _, a := range tc.anchors {
				assert.Contains(t, got, a, "剥离不应把正文一起吃掉")
			}
		})
	}
}

// TestStripRealSamplesKeepSectionAnchors 是「块级标签必须产生分隔」在真实样本
// 上的判据：板块标题写成 `<p><strong>一、…`，只有 <p> 换行、<strong> 删除，
// 下游 splitSections 的 `(?m)^[一-十]、` 才锚得住。
//
// 计数逐一核对过原文：2020 上半年 6 个板块，2025 年报 8 个。
func TestStripRealSamplesKeepSectionAnchors(t *testing.T) {
	sectionRE := regexp.MustCompile(`(?m)^[一二三四五六七八九十]、`)

	for _, tc := range []struct {
		file string
		want int
	}{
		{"pboc-2020-06-h1.html", 6},
		{"pboc-2025-12-annual.html", 8},
	} {
		t.Run(tc.file, func(t *testing.T) {
			got := stripHTML(readSample(t, tc.file))
			assert.Len(t, sectionRE.FindAllString(got, -1), tc.want,
				"块级标签未产生行首边界，板块切分会失效")
		})
	}
}

// TestStripAndMetaAreDeterministic 覆盖「纯函数」一条中可测的部分：
// 无包级可变状态则同一输入连续两次结果必然相同。不读文件、不访问网络
// 由代码审查保证（strip.go 只 import html/regexp/strings）。
func TestStripAndMetaAreDeterministic(t *testing.T) {
	for _, f := range []string{"pboc-2020-06-h1.html", "pboc-2025-12-annual.html"} {
		t.Run(f, func(t *testing.T) {
			raw := readSample(t, f)

			assert.Equal(t, stripHTML(raw), stripHTML(raw))

			v1, ok1 := metaContent(raw, "PubDate")
			v2, ok2 := metaContent(raw, "PubDate")
			assert.Equal(t, v1, v2)
			assert.Equal(t, ok1, ok2)
		})
	}
}

// nonEmptyLines 按行切分并去掉空行与首尾空白。
func nonEmptyLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}
