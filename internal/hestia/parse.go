package hestia

import (
	"fmt"
	"regexp"
	"strconv"
)

// 本文件是四层流水线的组装层：strip → detect → scope → extract，外加两处从
// 期次推导元数据的纯函数。它是本子迭代**唯一的导出入口**。

// titleRE 拆解报告标题的三种形态：
//
//	2025年金融统计数据报告        → 年度
//	2020年上半年金融统计数据报告   → 半年度
//	2026年6月金融统计数据报告      → 月度
//
// 「金融统计数据报告」是必需的后缀，且两端都锚定：央行同期还发《社会融资规模
// 统计数据报告》，不锚定后缀会把那篇也认下来，而它的板块结构完全不同——认下来
// 会一路错到 extract，且错得像模像样。
var titleRE = regexp.MustCompile(`\A([0-9]{4})年(上半年|[0-9]{1,2}月)?金融统计数据报告\z`)

// parseTitle 从标题推出 period 与 period_type。
//
// period 是**期末月**：年度报告的数据截至 12 月、上半年截至 6 月。M1b-1 的
// periodEndMonth 组合校验会兜住这里推错的情况——那道校验存在的理由正是
// 「2026-06/annual 会用 12 去除半年报期末月，让月均折算错一个量级」。
func parseTitle(title string) (period, periodType string, err error) {
	m := titleRE.FindStringSubmatch(title)
	if m == nil {
		return "", "", fmt.Errorf(
			"hestia: unrecognized report title %q "+
				"(want 「YYYY年金融统计数据报告」/「YYYY年上半年…」/「YYYY年M月…」)", title)
	}
	year, qualifier := m[1], m[2]

	switch qualifier {
	case "":
		return year + "-12", "annual", nil
	case "上半年":
		return year + "-06", "h1", nil
	default:
		// 「6月」→ 6 → "06"。不补零会产出 "2026-6"：bitemporal 按字典序比较业务键，
		// 那会成为与 "2026-06" 不同的键，同一日历月在视图里出现两次。
		n, convErr := strconv.Atoi(qualifier[:len(qualifier)-len("月")])
		if convErr != nil || n < 1 || n > 12 {
			return "", "", fmt.Errorf("hestia: invalid month in report title %q", title)
		}
		return fmt.Sprintf("%s-%02d", year, n), "monthly", nil
	}
}

// caliberChange 是一次口径变更：since 之后（含当期）适用 version。
type caliberChange struct {
	since   string // 生效期次，形如 YYYY-MM
	version string // 对应 validCaliberVersions 里的取值
}

// caliberChanges 是央行口径变更的生效期，取自报告原文的注释段
// （逐条对应 types.go 的 validCaliberVersions）。
//
// **本表的排列顺序没有语义**——caliberFor 取的是「所有已生效变更里 since 最大的
// 那条」，不是「表里第一条命中的」。这不是洁癖：2025 年报**同时**含注4（2023-01）
// 与注5（2025-01），两条都已生效，「取首个命中」的正确性会寄生在表的排列上。
// 有人按时间正序重排一次表，2025-12 就会静默变成 2023-01——而那是一个**合法**的
// caliber_version，M1b-1 的白名单拦不住它，下游会拿它做跨期对比。
// TestCaliberForIsOrderIndependent 把表逐位旋转后重跑，钉住这一点。
var caliberChanges = []caliberChange{
	{"2023-01", "2023-01"}, // 三类非存款类金融机构纳入统计（2025 年报告注4）
	{"2025-01", "2025-01"}, // M1 口径修订，纳入个人活期存款与非银支付备付金（注5）
}

// caliberBaseVersion 是首次口径变更之前的兜底版本（2020H1 报告注2）。
const caliberBaseVersion = "2015-01"

// caliberFor 按期次推导口径版本：取**所有已生效变更中最新的那条**。
//
// 用字符串比较：period 形如 YYYY-MM、定长补零，字典序即时间序——这是 M1b-1 的
// periodRE 强制形态带来的便利，也是它存在的另一个理由。
func caliberFor(period string) string {
	best := caliberBaseVersion
	bestSince := "" // 空串小于任何 YYYY-MM，故首个已生效项必然胜出

	for _, c := range caliberChanges {
		if c.since <= period && c.since > bestSince {
			best, bestSince = c.version, c.since
		}
	}
	return best
}

// Parse 把一篇央行《金融统计数据报告》的 HTML 解析成 Observation。
//
// 返回的 Observation 是**未完成的**：ArticleID 留空——那是 URL 的属性，而 Parse
// 只拿到字节。M1b-4 的 discover 知道 URL，由它填；IngestedAt 由 Store.Save 填
// （只有它知道入库时刻）。因此 Parse 的结果直接喂给 Save 会被 Meta.validate
// 拒绝——这不是缺陷，是边界：让 Parse 编造一个 ArticleID 才是缺陷。
//
// 元数据在剥离标签**之前**取：stripHTML 会把整个 <head> 一并抹掉。
func Parse(raw []byte) (Observation, error) {
	// PubDate 三态 + 形态，四种情形各自报错且措辞不同（返工补，QA WARNING-2）。
	//
	// published_at 是全包**唯一**一个逐字来自外部 HTML、不经任何模板的字段，因此
	// 也是「凡从输入文本读来的东西认不出就报错、绝不猜」这条规则此前的唯一偏离点：
	// 修复前只判 `ok`，于是 content=""、「2026-1-15」、「2026-01-15 09:30:00」
	// 三种全部放行，错误一路推迟到 Store.Save 的 publishedAtRE 才现场——**而那时
	// raw HTML 早已不在手上**，只剩一条「格式不对」要反推是哪篇文章的哪个 meta。
	// 对照 ArticleTitle：把它挖空**会**在下面的 parseTitle 处响亮失败。
	//
	// 「不存在」与「存在但为空」分开报，是因为 metaContent 的第二返回值正是为此
	// 设计的（strip.go 的注释：「调用方需要能分辨是站点没填还是选择器写错了」）——
	// 两者的排障方向完全不同，合并成一条等于把那个刻意提供的区分丢掉。
	//
	// 形态校验复用 types.go 的 publishedAtRE，与 Store.Save 用的是**同一条**：
	// 各写一份迟早分叉，而分叉的表现是「Parse 放行、Save 拒绝」的缝。
	pubDate, ok := metaContent(raw, "PubDate")
	switch {
	case !ok:
		return Observation{}, fmt.Errorf(
			"hestia: missing <meta name=\"PubDate\">: it is the only trustworthy source " +
				"of published_at — the article id in the URL is a site-migration timestamp " +
				"and createDate is a CMS record time. A missing tag usually means the page " +
				"structure changed or the selector is wrong, not that the site left it blank")
	case pubDate == "":
		return Observation{}, fmt.Errorf(
			"hestia: <meta name=\"PubDate\"> is present but its content is empty: the site " +
				"published the article without a date. Refusing to substitute today, the " +
				"article id or createDate — published_at drives the bitemporal revision chain")
	case !publishedAtRE.MatchString(pubDate):
		return Observation{}, fmt.Errorf(
			"hestia: <meta name=\"PubDate\"> content %q does not match YYYY-MM-DD: refusing "+
				"to normalise it here — Store.Save enforces the same shape, and a value that "+
				"only fails there arrives without the raw HTML needed to diagnose it",
			pubDate)
	}
	title, ok := metaContent(raw, "ArticleTitle")
	if !ok {
		return Observation{}, fmt.Errorf(
			"hestia: missing <meta name=\"ArticleTitle\">: period and period_type are derived from it")
	}

	period, periodType, err := parseTitle(title)
	if err != nil {
		return Observation{}, err
	}
	if err := checkPeriodTypeSupported(periodType, title); err != nil {
		return Observation{}, err
	}

	// 切分之后**立即**探测版本，并把 error 当致命：不认识的形态就停在这里，
	// 不进入抽取层。splitSections 的签名没有 error，它能这样设计的前提正是
	// 「紧邻的 detectExtractor 会报错，且错误信息严格更多」——那个前提由这两行
	// 兑现，不是别处已有的事实。删掉或挪后这一段，T3 的保证就随之消失。
	secs := splitSections(stripHTML(raw))
	extractor, err := detectExtractor(secs)
	if err != nil {
		return Observation{}, err
	}

	values, err := extractFields(secs, extractor)
	if err != nil {
		return Observation{}, err
	}

	return Observation{
		Meta: Meta{
			Period:         period,
			PeriodType:     periodType,
			PublishedAt:    pubDate,
			CaliberVersion: caliberFor(period),
			Extractor:      extractor,
			// ArticleID 由 M1b-4 填，IngestedAt 由 Store.Save 填
		},
		Values: values,
	}, nil
}

// checkPeriodTypeSupported 挡住零样本验证的期次形态。
//
// 目前只有 monthly 落在这里：两份样本一份 annual、一份 h1，月报**一份都没有**。
// 而月报恰恰是孪生句问题最严重的形态——正文同时含期内累计句与当月句，且哪句在前
// 无样本可证；extract 侧的 cumulativePeriods 只认「全年/上半年」，对 5 月报的
// 「1-5月」这类前缀会整条不命中。
//
// 悄悄放行会让一份月报产出**看起来完全正常**的 Values：键数对、量级对、全在白名单
// 内，而 YTD 字段里装的可能是当月数——下游没有任何闸门拦得住。故显式拒绝。
//
// 拿到月报样本后删掉这个函数与它的调用即可，parseTitle 及其测试一行都不用改。
func checkPeriodTypeSupported(periodType, title string) error {
	if periodType != "monthly" {
		return nil
	}
	return fmt.Errorf(
		"hestia: period_type monthly is not supported yet (title %q): no monthly sample exists "+
			"to validate against, and monthly reports carry both a period-to-date and a "+
			"current-month sentence for the same field — refusing to guess which one the "+
			"*_ytd fields should hold", title)
}
