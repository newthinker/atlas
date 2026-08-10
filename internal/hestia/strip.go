package hestia

import (
	"html"
	"regexp"
	"strings"
)

// HTML 剥离的三组正则。
//
// script 与 style 分成两条而不是 <(script|style)…</\1>：Go 的 regexp 是 RE2，
// **不支持反向引用**，那种写法会让 MustCompile 直接 panic。
var (
	scriptRE = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script>`)
	styleRE  = regexp.MustCompile(`(?is)<style\b[^>]*>.*?</style>`)

	// 块级标签 → 换行。它们是真实的段落边界，删掉会让不同板块粘成一行，
	// 「^[一-十]、」的板块切分随之失效。
	blockTagRE = regexp.MustCompile(
		`(?i)</?(?:p|div|br|tr|td|th|li|ul|ol|h[1-6]|table|tbody|thead|section|article|hr)\b[^>]*/?>`)

	// 其余标签（span/a/em/strong/font/b/i…）→ 直接删除。
	//
	// 这一条是本文件存在的核心理由：真实样本里「4825」与「亿元」之间隔着
	// 六层 <span>，用换行替换会把数值与单位切开，而 extract 的「单位必须
	// 捕获否则报错」会因此误报——数据完整，只是标记切断了它。
	inlineTagRE = regexp.MustCompile(`<[^>]*>`)

	// 标签删除后会留下空隙；nbsp 也在此一并折叠
	spaceRE = regexp.MustCompile(`[ \t\x{00a0}]+`)
	// 三个及以上连续换行压成两个，便于阅读调试输出
	blankLineRE = regexp.MustCompile(`\n{3,}`)
)

// punctNormalizer 把半角标点归一为全角。
//
// 2020 样本混用（M2 与 M0 两句各一个半角逗号、票据融资一句一个半角分号），
// 2025 全角。归一在这里一次完成，后面每条句式模板就不必同时写两种标点——
// 那种重复迟早会漏掉一处，而漏掉的表现是字段静默失配。
var punctNormalizer = strings.NewReplacer(
	",", "，",
	";", "；",
	"　", " ", // 全角空格 → 普通空格
)

// stripHTML 把原始 HTML 转成可用正则匹配的纯文本。
//
// 处理顺序有讲究：先删 script/style（内含大量数字与标点，会污染板块切分），
// 再把块级标签换成换行、内联标签删掉，最后还原实体与归一标点。
// 实体还原必须在标签处理**之后**——否则内容里的 &lt;p&gt; 会被误当成标签。
func stripHTML(raw []byte) string {
	s := string(raw)
	s = scriptRE.ReplaceAllString(s, "")
	s = styleRE.ReplaceAllString(s, "")
	s = blockTagRE.ReplaceAllString(s, "\n")
	s = inlineTagRE.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = punctNormalizer.Replace(s)
	s = spaceRE.ReplaceAllString(s, " ")
	s = blankLineRE.ReplaceAllString(s, "\n\n")
	return s
}

// metaRE 匹配 <meta name="…" content="…">。
//
// 属性间用 \s+ 而非单个空格：样本里 `<meta  name="createDate"` 与
// `<meta name="ColumnType"  content=…` 都是双空格，站点模板并不规整。
//
// name 走 QuoteMeta：它虽然只来自本包的常量，但拼进正则就该转义，
// 与 bitemporal 对标识符的态度一致。
func metaRE(name string) *regexp.Regexp {
	return regexp.MustCompile(
		`(?is)<meta\s+name="` + regexp.QuoteMeta(name) + `"\s+content="([^"]*)"`)
}

// metaContent 取 <head> 里某个 <meta name="…" content="…"> 的值。
//
// 必须在 stripHTML 之前调用——那个函数会把 <head> 一并抹掉。
//
// 第二返回值区分「不存在」与「存在但为空」：站点确实会输出 content=""
// （SiteDomain、Description 等），调用方需要能分辨是站点没填还是选择器写错了。
func metaContent(raw []byte, name string) (string, bool) {
	m := metaRE(name).FindSubmatch(raw)
	if m == nil {
		return "", false
	}
	return strings.TrimSpace(html.UnescapeString(string(m[1]))), true
}
