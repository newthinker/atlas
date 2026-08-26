package hestia

import (
	"fmt"
	"regexp"
	"strconv"
)

// 本文件是四层流水线的组装层：strip → detect → scope → extract，外加两处从
// 期次推导元数据的纯函数。它是本子迭代**唯一的导出入口**。

// titleRE 拆解报告标题的五种形态：
//
//	2025年金融统计数据报告        → 年度
//	2026年一季度金融统计数据报告   → 一季度（年初起累计，1–3 月）
//	2020年上半年金融统计数据报告   → 半年度
//	2025年前三季度金融统计数据报告 → 前三季度（年初起累计，1–9 月）
//	2026年6月金融统计数据报告      → 月度
//
// 「金融统计数据报告」是必需的后缀，且两端都锚定：央行同期还发《社会融资规模
// 统计数据报告》，不锚定后缀会把那篇也认下来，而它的板块结构完全不同——认下来
// 会一路错到 extract，且错得像模像样。
//
// # 与 discover.go 的 reportTitleRE 是两份并行的解析，刻意不同（TASK-010）
//
// 那边看列表页的**链接文本**、这边看文章页的 `<meta name="ArticleTitle">`，两者
// 期末月约定必须同源（q1→03、h1→06、q1_q3→09、annual→12），但**季度段的宽窄
// 有意不同**：
//
//   - 那边写宽（`[一二三四五六七八九十]季度`）再由语义层只放行「一季度」——因为
//     在列表页上「不匹配」是**安静**的（一页 15 条里 14 条本就不是报告），把二/三/
//     四季度挡在语义层才留得下位置、注释和用例。
//   - 这边只列 `一季度|前三季度` —— 因为在这里「不匹配」是**响亮**的：parseTitle
//     直接返回 `unrecognized report title %q`，标题原文就在错误信息里。再加一层
//     语义拒绝只是把同一句话换个地方说。
//
// ⚠️ 央行**不发**二/三/四季度的《金融统计数据报告》（那三段分别由上半年 / 前三
// 季度 / 全年覆盖）。真出现「三季度」也必须拒：它字面上无法区分单季（7–9 月）与
// 累计（1–9 月），期末月同为 09 而月均折算除数是 3 与 9 —— 猜错正是 types.go 的
// validPeriodTypes 警告的「错一个量级」。
//
// # 三种报告（M1c-3a 的 TASK-007）
//
// 后缀从「只认金融统计数据报告」扩到三种，并作为 kind 返回给 Parse 做分派。
// 社融两种的正文结构与金融统计报告完全不同（无板块小标题、整篇一段），所以
// **kind 决定走哪条抽取路径**，不是同一条路径上的一个参数。
//
// ⚠️ 三个后缀两两互不为前缀，交替顺序在此无语义；但仍把两个较长的社融后缀写在
// 前面，与本包既有习惯一致（理由见 profiles.go 的 cumulativeMonthAlt 那段：
// 顺序真正有语义的前提是两个分支都能走完整条正则，这里不构成那个前提）。
//
// ⚠️ **与 backfill_reconcile.go 的 backfillPeriodOf 是两份并行的解析**：那边服务对账、
// 看 manifest 里的标题、返回 (period, kind) 不返回 periodType；这边看文章页的
// <meta name="ArticleTitle">、返回三者。**本任务不合并两处**（那会扩大 scope），
// 但**期末月约定必须同源**（q1→03、h1→06、q1_q3→09、annual→12、N月→N）——
// 两边任一处改了期末月而另一处没改，表现是同一期在对账表与观测表里落在不同键上。
var titleRE = regexp.MustCompile(
	`\A([0-9]{4})年(一季度|上半年|前三季度|[0-9]{1,2}月)?` +
		`(社会融资规模存量统计数据报告|社会融资规模增量统计数据报告|金融统计数据报告)\z`)

// 三种报告种类。kindFinance 走板块切分 + detectExtractor；社融两种整篇直抽。
const (
	kindFinance  = "金融统计数据报告"
	kindTSFStock = "社会融资规模存量统计数据报告"
	kindTSFFlow  = "社会融资规模增量统计数据报告"
)

// parseTitle 从标题推出 period 与 period_type。
//
// period 是**期末月**：年度报告的数据截至 12 月、上半年截至 6 月。M1b-1 的
// periodEndMonth 组合校验会兜住这里推错的情况——那道校验存在的理由正是
// 「2026-06/annual 会用 12 去除半年报期末月，让月均折算错一个量级」。
func parseTitle(title string) (period, periodType, kind string, err error) {
	m := titleRE.FindStringSubmatch(title)
	if m == nil {
		return "", "", "", fmt.Errorf(
			"hestia: unrecognized report title %q "+
				"(want 「YYYY年[一季度|上半年|前三季度|M月]」+ one of "+
				"「%s」/「%s」/「%s」)", title, kindFinance, kindTSFStock, kindTSFFlow)
	}
	year, qualifier, kind := m[1], m[2], m[3]

	switch qualifier {
	case "":
		return year + "-12", "annual", kind, nil
	case "一季度":
		return year + "-03", "q1", kind, nil
	case "上半年":
		return year + "-06", "h1", kind, nil
	case "前三季度":
		// 「前三季度」= 1–9 月累计，不是第 3 季度单季。periodType 叫 q1_q3 而不是
		// q3，理由与期末月一起定在 TASK-001 的 discovery 里：两者期末月同为 09，
		// 月均折算除数却是 9 与 3。
		return year + "-09", "q1_q3", kind, nil
	default:
		// 「6月」→ 6 → "06"。不补零会产出 "2026-6"：bitemporal 按字典序比较业务键，
		// 那会成为与 "2026-06" 不同的键，同一日历月在视图里出现两次。
		n, convErr := strconv.Atoi(qualifier[:len(qualifier)-len("月")])
		if convErr != nil || n < 1 || n > 12 {
			return "", "", "", fmt.Errorf("hestia: invalid month in report title %q", title)
		}
		return fmt.Sprintf("%s-%02d", year, n), "monthly", kind, nil
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

	period, periodType, kind, err := parseTitle(title)
	if err != nil {
		return Observation{}, err
	}
	if err := checkPeriodTypeSupported(periodType, title); err != nil {
		return Observation{}, err
	}

	// —— 按 kind 三路分派（M1c-3a 的 TASK-007）——
	//
	// 三种报告的正文结构不同，**不是同一条路径上的一个参数**：金融统计数据报告有
	// 板块小标题、需要切分后探测版式；社融两种整篇一段，没有可切的板块，因此
	// extractor 直接由 kind 决定，不经探测。
	//
	// ⚠️ `detectExtractor` 只对 kindFinance 有意义。对社融两种调用它会命中
	// missingCoreSections 而报「这是新版式/别的报告/抓取被截断」——一句**方向完全
	// 错**的错误信息：它们本来就没有那四个核心板块。
	var (
		extractor string
		values    map[string]float64
	)
	switch kind {
	case kindFinance:
		// 切分之后**立即**探测版本，并把 error 当致命：不认识的形态就停在这里，
		// 不进入抽取层。splitSections 的签名没有 error，它能这样设计的前提正是
		// 「紧邻的 detectExtractor 会报错，且错误信息严格更多」——那个前提由这两行
		// 兑现，不是别处已有的事实。删掉或挪后这一段，T3 的保证就随之消失。
		secs := splitSections(stripHTML(raw))
		extractor, err = detectExtractor(secs, periodType)
		if err != nil {
			return Observation{}, err
		}
		values, err = extractFields(secs, extractor)
	case kindTSFStock:
		extractor = extractorTSFStock
		values, err = extractTSFStockArticle(stripHTML(raw))
	case kindTSFFlow:
		extractor = extractorTSFFlow
		values, err = extractTSFFlowArticle(stripHTML(raw))
	default:
		// 不可达：kind 来自 titleRE 的捕获组，取值受正则约束。留着是为了让「将来
		// 往 titleRE 加第四种后缀却忘了在这里加分支」响亮失败，而不是静默走到
		// 下面用零值 extractor 造出一份 Observation。
		return Observation{}, fmt.Errorf(
			"hestia: unhandled report kind %q from title %q: titleRE recognises it but "+
				"Parse has no dispatch branch for it", kind, title)
	}
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

// checkPeriodTypeSupported 挡住抽取侧还接不了的期次形态。
//
// 悄悄放行会让这些期次产出**看起来完全正常**的 Values：键数对、量级对、全在白名单
// 内，而里面装的是错口径的数——下游没有任何闸门拦得住。故显式拒绝。
//
// # 判据是「默认拒绝」，不是「列出几个坏的」（TASK-010 改）
//
// 原来写的是 `if periodType != "monthly" { return nil }` —— 一条**默认放行**的规则。
// TASK-001 往 validPeriodTypes 加了 q1 / q1_q3 之后，这两种抽取侧根本没接的期次
// 就这样直接穿过去了，而**没有任何测试会红**。改成穷举 switch 之后，新增第六种
// period_type 会由 TestEveryPeriodTypeHasAnExplicitSupportDecision 逼人明确表态。
//
// 解除的方式是**逐个删分支**，不是删整个函数：
//   - q1 / q1_q3：**已由 M1b-4b 的 TASK-004 删除** —— periodAlt 加了「一季度|前三季度」、
//     cumulativePeriods 加了同两个键，两份真实快照端到端跑通（各 8 板块、rule@v2）。
//   - monthly：**已由 M1c-3a 的 TASK-007 删除** —— periodAlt 加了「前N个月」十项与
//     「1月份」特例（TASK-001）、板块切分认 4/5/7 节月报（TASK-004）、抽取侧按
//     extractor 决定板块适用性（TASK-006），四份真实月报快照端到端跑通。
//
// ⚠️ **原分支里那句「extract 侧认不出 5 月报的『1-5月』这类前缀」是错的**，一并记在这里
// 免得后人照它推断：Leader 全量实读 55 篇月报，「1-5月」这种带范围的形态**一次都没出现**；
// 真实形态是「前八个月」这类中文数字前缀（累计）与「8月份」（当月，要排除），
// 1 月报的「1月份」则是累计特例。那句注释写于零样本时期，是**推测**而非实测。
//
// 三处解除都不用动 parseTitle 及其测试。
func checkPeriodTypeSupported(periodType, title string) error {
	switch periodType {
	// 目前五种 period_type 全部受支持，故本 switch 暂时没有分支。
	//
	// 🔴 **不要因此删掉这个函数**（M1c-3a 的 TASK-007 删 monthly 分支时的决定）：
	// 空 switch 看起来像死代码，但它承载的是**「新增第六种 period_type 必须明确表态」**
	// 这条防线 —— TestEveryPeriodTypeHasAnExplicitSupportDecision 遍历 validPeriodTypes
	// 逐个来问，函数没了那道遍历就无处可问。删掉它的代价不是少一层校验，是让
	// 「加了新期次却没人想过抽取侧接不接」重新变成静默的。
	}
	return nil
}
