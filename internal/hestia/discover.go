package hestia

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
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
	base, err := url.Parse(indexURL)
	if err != nil {
		return "", fmt.Errorf("hestia discover: bad index url %q: %w", indexURL, err)
	}
	ref, err := url.Parse(strings.Replace(tmpl, "%1", strconv.Itoa(page), 1))
	if err != nil {
		return "", fmt.Errorf("hestia discover: bad paging template %q: %w", tmpl, err)
	}
	// ResolveReference 处理站内绝对路径与相对路径两种形态，比手工拼 scheme+host 稳。
	return base.ResolveReference(ref).String(), nil
}
