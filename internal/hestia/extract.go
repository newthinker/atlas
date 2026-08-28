package hestia

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// 本文件是**代码**：把 profiles.go 的模板跑在板块正文上，累积成字段表。
//
// 三条贯穿全文件的纪律：
//
//  1. 无方向词的句子走 parsePlainAmount，**不给 newAmount 传字面量方向词**。
//     余额/存量句原文没有方向词，硬编码一个「增加」会让方向词白名单在这些
//     调用点上永远不可能触发——约四分之一的数值赋值走这条路。
//  2. 孪生句一律**按捕获组挑，并要求唯一**，不靠最左优先。相邻句子往往量级
//     相近、格式完全正确，选错了下游没有任何校验拦得住。
//  3. 任何模板未命中一律报错。静默跳过会让「解析漏了」被下游当成「本期模板
//     本就没有」——那正是整个设计要禁止的失败方式。

// collector 累积抽取结果，并拒绝重复赋值。
//
// 重复赋值意味着两个模板命中了同一字段——最可能的原因是作用域切错，比如
// 「短期贷款」同时落进住户与企业两个作用域。静默取最后一个是最坏的处置：
// 两个值都合理，事后无从判断哪个对。
type collector struct{ values map[string]float64 }

func newCollector() *collector { return &collector{values: map[string]float64{}} }

func (c *collector) set(field string, v float64) error {
	if old, ok := c.values[field]; ok {
		return fmt.Errorf(
			"hestia: field %s already set to %v, refusing to overwrite with %v: "+
				"two templates matched the same field, most likely a mis-sliced scope",
			field, old, v)
	}
	c.values[field] = v
	return nil
}

func (c *collector) merge(other map[string]float64) error {
	for f, v := range other {
		if err := c.set(f, v); err != nil {
			return err
		}
	}
	return nil
}

// sectionRule 把板块关键词、适用版本与抽取函数绑在一起。
//
// 关键词只用来定位标题（findSection 只匹配 Title）。它的正确性依赖「每个关键词
// 在一份报告里最多命中一个标题」——由 TestSectionKeywordsHitAtMostOneTitle 守护。
type sectionRule struct {
	keyword string
	// v2Only：社融两节只在 **v2 版式**存在（rule@v2 / rule-monthly@v2）。
	v2Only bool
	// noMonthly：月报族不含本节。目前只有外汇节 —— 55 篇月报实测 53 篇正文里
	// 根本没有「国家外汇储备余额」字样（缺口 G1），那不是抓取缺失，是
	// absent-by-design（M1c-3a 的 TASK-006，AD-3）。
	//
	// ⚠️ 刻意**不**拿它去派生 coreSectionKeywords（那边仍按关键词判外汇节）：
	// 「月报族没有本节」与「本节不算核心板块」是两个不同的概念，一个板块完全可能
	// 月报没有却仍属核心。合并成一个标记会让将来给第二个板块打 noMonthly 时，
	// coreSectionKeywords 静默变松 —— 而它变松不会有任何东西转红。
	noMonthly bool
	fn        func(section) (map[string]float64, error)
}

var sectionRules = []sectionRule{
	{keyword: tsfSectionKeyword, v2Only: true, fn: extractTSFStockSection},
	{keyword: "社会融资规模增量", v2Only: true, fn: extractTSFFlowSection},
	{keyword: "广义货币", fn: extractMoneySection},
	{keyword: "人民币存款", fn: extractDepositSection},
	{keyword: "人民币贷款", fn: extractLoanSection},
	{keyword: "加权平均利率", fn: extractRateSection},
	{keyword: fxSectionKeyword, noMonthly: true, fn: extractFXSection},
}

// sectionPathExtractors 是走「切板块 → 逐节抽取」这条路的全部 extractor。
//
// tsf-stock@v1 / tsf-flow@v1 **不在其中**：社融独立报告没有板块结构，整篇当一节
// 由 extractTSFStockArticle / extractTSFFlowArticle 处理（M1c-3a 的 TASK-002）。
// 它们是合法的 extractor 值（validExtractors 收了它们），只是走错了路 —— 所以
// extractFields 收到它们要报错，而不是返回空表。
var sectionPathExtractors = []string{
	extractorV1, extractorV2, extractorMonthlyV1, extractorMonthlyV2,
}

// appliesTo 是「本板块在该 extractor 下适不适用」的**唯一**定义（M1c-3a 的 TASK-006）。
//
// 收敛成一处而不是在 extractFields 的循环体里堆并列 if：两个维度堆两个 if 还能读，
// 第三个维度出现时就不能了，而且那时「板块归属」会散落在循环里，没有任何地方能
// 一眼看全。extract_test.go 的 TestSectionAppliesToIsTheSingleSourceOfScope 拿一张
// 逐字面量的期望表对着它验。
//
// 判据是**声明式跳过**，不是「碰巧 findSection 找不到就放过」——后者是巧合，
// 而巧合会在某期正文里偶然出现社融字样时失效（extractFields 原注释的理由，
// 同样适用于外汇节）。声明式跳过还让「适用板块缺失」仍然响亮失败，
// 两者由 TestExtractFieldsSkipsVsMissesSections 成对钉住。
func (r sectionRule) appliesTo(extractor string) bool {
	if r.v2Only && !isV2Layout(extractor) {
		return false
	}
	if r.noMonthly && isMonthlyFamily(extractor) {
		return false
	}
	return true
}

// isV2Layout / isMonthlyFamily 把 extractor 名切成两个**互相正交**的轴：
// 版式（v1 / v2）× 报告族（累计期 / 月报）。四个 sectionPathExtractors 正好
// 是这两个轴的四种组合。
//
// 用穷举比较而不是 strings.HasSuffix("@v2") / HasPrefix("rule-monthly")：
// 字符串形状与语义**当前**恰好一致，但那是命名的巧合，常量表才是事实
// （与 required.go 头部「不用 strings.HasPrefix(f, "tsf_") 代替」同一条理由）。
func isV2Layout(extractor string) bool {
	return extractor == extractorV2 || extractor == extractorMonthlyV2
}

func isMonthlyFamily(extractor string) bool {
	return extractor == extractorMonthlyV1 || extractor == extractorMonthlyV2
}

// extractFields 按模板版本抽取全部字段。
//
// 哪些板块适用由 sectionRule.appliesTo 一处决定（M1c-3a 的 TASK-006，AD-3）：
// rule@v1 跳过社融两节、月报族跳过外汇节。显式跳过比「碰巧匹配不到」更清楚：
// 前者是声明，后者是巧合，而巧合会在某期正文里偶然出现社融字样时失效。
//
// 跳过的板块与 requiredFields 必须归属一致，否则 completeness 会把「本期本就
// 没有」记成「缺失」（或反过来把没人要的字段当成抽到了）。两者是各自独立派生的，
// 由 TestExtractFieldsScopeMatchesRequiredFields 做双向相等比对。
func extractFields(secs []section, extractor string) (map[string]float64, error) {
	// 两种拒绝分开报，因为**排障方向完全不同**：一个是路由错了（值是对的，
	// 该走另一条抽取路径），一个是值本身认不出（模板集无从选起）。合并成一条
	// 会让前者的读者去查取值域、后者的读者去查路由，两边都被指错方向。
	// 这与 Parse 把 PubDate 的三种缺陷分开报是同一条理由。
	//
	// 「合法但不走板块路径」由 validExtractors 减去 sectionPathExtractors **派生**，
	// 不另列一份：llm-fallback@v1 与社融两种都落在这里，将来再加也自动归位。
	switch {
	case slices.Contains(sectionPathExtractors, extractor):
		// 走板块路径，继续
	case slices.Contains(validExtractors, extractor):
		return nil, fmt.Errorf(
			"hestia: extractor %q is valid but does not take the section path "+
				"(section-path extractors: %s): its reports carry no section structure — "+
				"%s and %s go through extractTSFStockArticle / extractTSFFlowArticle. "+
				"Returning an empty field set here would look like a report with no data",
			extractor, strings.Join(sectionPathExtractors, ", "),
			extractorTSFStock, extractorTSFFlow)
	default:
		return nil, fmt.Errorf(
			"hestia: unknown extractor %q (section-path extractors: %s): refusing to guess "+
				"a template set — guessing would read a report this package has never seen "+
				"and report its fields as if they came from a known layout",
			extractor, strings.Join(sectionPathExtractors, ", "))
	}

	c := newCollector()
	for _, rule := range sectionRules {
		if !rule.appliesTo(extractor) {
			continue
		}
		sec, ok := findSection(secs, rule.keyword)
		if !ok {
			return nil, fmt.Errorf(
				"hestia: %s requires a section whose title contains %q, none found",
				extractor, rule.keyword)
		}
		got, err := rule.fn(sec)
		if err != nil {
			return nil, err
		}
		if err := c.merge(got); err != nil {
			return nil, err
		}
	}
	return c.values, nil
}

// mustMatch 跑一条正则并要求它**恰好命中一次**。
//
// 零命中报错是显然的；**多命中同样报错**，这一半是返工补上的（QA WARNING-1）。
// 此前它用 FindStringSubmatch，多命中时**静默取最左那个**——于是本文件开头
// 纪律 2 自述的「孪生句一律按捕获组挑并要求唯一」实际只落实在
// selectRMBBalance / selectRMBCumulativeFlow 两族，约 30 条清单模板仍在最左优先，
// 而那个选择零测试覆盖（变异「取最后一个匹配」因此存活）。
//
// 危害与那两族完全同类，只是位置更深：把单月分部门句排在累计句之前（合法排版，
// 2020 样本已有同体例的板块级孪生句），修复前 err=nil 而 deposit_household_ytd
// 取到单月值、deposit_nbfi_ytd 连**符号都翻了**。两个值都在合理量级内，下游
// 没有任何闸门拦得住。
//
// 与 selectUnique 的分工：那个用于**候选句需要按捕获组筛选**的场合（口径、期次），
// 这个用于「模板本身就该唯一命中」的场合，故不需要谓词。两者的失败语义一致：
// 0 命中与 ≥2 命中都报错，绝不替调用方挑一个。
func mustMatch(re *regexp.Regexp, body, what string) ([]string, error) {
	all := re.FindAllStringSubmatch(body, -1)
	switch len(all) {
	case 1:
		return all[0], nil
	case 0:
		return nil, fmt.Errorf("hestia: %s not found (pattern %s)", what, re)
	default:
		return nil, fmt.Errorf(
			"hestia: %s matched %d sentences (pattern %s): refusing to pick one — "+
				"a template is expected to hit exactly once; more than one means the section "+
				"carries twin sentences (e.g. a month-to-date and a current-month figure) and "+
				"leftmost-first would choose silently, with both values looking plausible",
			what, len(all), re)
	}
}

// selectUnique 从全部候选句里按谓词挑出唯一一条。
//
// 这是本文件对付孪生句的统一手法。相邻句子（本外币/外币、期内合计/单月）量级
// 相近、格式完全正确，最左优先能碰巧选对，但那是文本顺序的副产品——某期换个
// 写法顺序就会静默取错。改成显式按捕获组挑之后：
//
//	命中 0 条 → 报错（而不是退而取隔壁那句）
//	命中 2 条 → 报错（而不是静默选一个，两个值都合理，事后无从分辨）
//
// label 只输出用于分辨的**限定词**，不输出数值——错误信息里带上隔壁句的数字
// 会让人误以为那是结果。
func selectUnique(ms [][]string, what string, keep func([]string) bool, label func([]string) string) ([]string, error) {
	var hits [][]string
	seen := make([]string, 0, len(ms))
	for _, m := range ms {
		seen = append(seen, label(m))
		if keep(m) {
			hits = append(hits, m)
		}
	}

	switch len(hits) {
	case 1:
		return hits[0], nil
	case 0:
		return nil, fmt.Errorf(
			"hestia: %s not found among %d candidate sentence(s) [%s]: refusing to fall back "+
				"to a neighbouring one, it has the right magnitude and format but the wrong meaning",
			what, len(ms), strings.Join(seen, " "))
	default:
		return nil, fmt.Errorf(
			"hestia: %s matched %d sentences [%s]: refusing to pick one, both values look "+
				"plausible and leftmost-first would choose silently",
			what, len(hits), strings.Join(seen, " "))
	}
}

// selectRMBBalance 挑出人民币口径的余额句。捕获组：1=口径 2=数值 3=单位 4=方向词 5=同比。
func selectRMBBalance(re *regexp.Regexp, body, what string) ([]string, error) {
	return selectUnique(re.FindAllStringSubmatch(body, -1), what,
		func(m []string) bool { return m[1] == currencyRMB },
		func(m []string) string { return m[1] })
}

// selectRMBCumulativeFlow 挑出人民币口径、期内累计（非单月）的合计句。
// 捕获组：1=期次 2=口径 3=方向词 4=数值 5=单位。
//
// 两个维度都要判：外币孪生句的期次前缀同样是「全年/上半年」，只判期次会取到
// 「全年外币存款增加2135亿美元」。
func selectRMBCumulativeFlow(re *regexp.Regexp, body, what string) ([]string, error) {
	return selectUnique(re.FindAllStringSubmatch(body, -1), what,
		func(m []string) bool { return cumulativePeriods[m[1]] && m[2] == currencyRMB },
		func(m []string) string { return m[1] + "/" + m[2] })
}

func extractTSFStockSection(sec section) (map[string]float64, error) {
	c := newCollector()

	m, err := mustMatch(tsfStockTotalRE, sec.Body, "社融存量总量")
	if err != nil {
		return nil, err
	}
	if err := setBalanceAndYoY(c, FieldTSFStock, FieldTSFStockYoY, m[1], m[2], m[3], m[4]); err != nil {
		return nil, err
	}

	for _, it := range tsfStockItems {
		m, err := mustMatch(tsfStockRE(it.name), sec.Body, "社融存量分项 "+it.name)
		if err != nil {
			return nil, err
		}
		if err := setBalanceAndYoY(c, it.balanceField, it.yoyField, m[1], m[2], m[3], m[4]); err != nil {
			return nil, err
		}
	}
	return c.values, nil
}

// extractTSFFlowSection 是 v2 板块路径的入口，**逐字走整篇路径的同一段代码**。
//
// 🔴 **它一度是独立的一份实现，而缺陷就藏在那份差异里**（M1c-3a 的 TASK-011，QA R1）：
// 原实现用窄模板 tsfFlowTotalRE 取总量、再拿**整节**去抽分项，没有作用域切分。对真实的
// 2023-08 正文（累计句 → 当月句 → 分项）实测 err=nil 且抽出
// `tsf_flow_ytd=252100`（1–8 月累计）配 `tsf_flow_rmb_loan_ytd=13400`（8 月当月），
// 错位 18.8×，两个值又都在合法量级内 —— 下游没有任何闸门拦得住。
//
// M1c-3a 的 TASK-002 做作用域切分时刻意「板块路径一个字不改」（越界 + 当时无样本），
// 那个判断在当时成立；代价是同一个缺陷在另一条路上原样留了下来。⚠️ **而板块路径正是
// v2 月报走的那条**（央行 2025-10 起把社融并进月报的 going-forward 格式）⇒ 不是历史
// 遗留，是会持续产生错数据的路径。
//
// 🔴 **修法选的是「两条路径共用同一段代码」而不是「给板块路径也补一份切分」**：
// 后者仍是两份实现，「同一段正文得同一结论」只能靠断言维持，而**分叉恰恰是这个缺陷的
// 形状**；共用之后那条性质由构造保证，断言只是把它钉住。
// 由 TestTSFFlowSectionAndArticleAgreeOnSameBody 与
// TestTSFFlowSectionRefusesCaliberMixOnRealArticle 守着。
//
// ⚠️ **留下一笔债**：窄模板 `tsfFlowTotalRE` 自此**没有生产调用方**了（它被
// `tsfFlowArticleTotalRE` 完全覆盖）。它仍登记在 profiles.go 的 `allTemplateRegexps()`
// 里、且 extract_test.go 的模板点名仍在滚它 —— profiles.go 不在本任务 writes 内，
// 删不了。**profiles.go 解冻时应删掉它，并把 `tsfFlowArticleTotalRE` 挪过去**
// （与 M1c-3a 的 TASK-002 记下的那笔债是同一件事）。
func extractTSFFlowSection(sec section) (map[string]float64, error) {
	return extractTSFFlowArticle(sec.Body)
}

func extractMoneySection(sec section) (map[string]float64, error) {
	c := newCollector()
	for _, it := range moneyItems {
		m, err := mustMatch(moneyRE(it.name, it.code), sec.Body, "货币 "+it.code)
		if err != nil {
			return nil, err
		}
		if err := setBalanceAndYoY(c, it.balanceField, it.yoyField, m[1], m[2], m[3], m[4]); err != nil {
			return nil, err
		}
	}
	return c.values, nil
}

// —— M1c-3a 的 TASK-009：分部门口径守卫 ——
//
// 月报的分部门数字有两种口径，而**两者都是合法量级**。2023-08 实读：
//
//	前八个月人民币贷款增加17.44万亿元。          ← 累计句 → loan_flow_ytd = 174400
//	8月份人民币贷款增加1.36万亿元。分部门看，住户贷款增加3922亿元，
//	  其中，短期贷款增加2320亿元…                ← 分部门跟在**当月句**后面
//
// 不加守卫时 loan_hh_short_ytd 会装 2320（8 月单月），而同一份 Observation 里
// loan_flow_ytd 是 174400（累计）——**同一份观测内部口径混杂**，而七道闸门一道
// 也拦不住：corp_loan_reconcile 查的是分部门**内部**自洽（错位后仍成立）、
// gateDepositSum 在 validate 阶段而 calibrate 不跑闸门、所有值都在合法量级内
// 而 magnitude_sanity 是空表。3a 的 calibrate 报告又是 M1c-3b 填 magnitude_ranges
// 的唯一输入 ⇒ 混杂值会污染下游标定。
//
// 判据是**结构性**的：「分部门段之前最近的那个期次前缀，决定分部门的口径」。
// Leader 对 54 篇有分部门段的月报逐篇比对该判据与数值分类，54/54 一致、0 例外。
//
// ⚠️ **刻意不用数值启发式**（「分部门合计与累计句偏差 > X%」）：那是拿一个测量值
// 当判据，会随语料漂移而坏，且阈值本身无人守护。⚠️ **也不用期次黑名单**：新增期次
// 时静默失效，而失效方向是**放行错数据**。
//
// ⚠️ 1 月报不需要特例分支：`1月份` 已在 cumulativePeriods 里（M1c-3a 的 TASK-001 加），
// 查表自然命中。若发现需要为它写分支，说明判据实现偏了。
const (
	// 贷款侧与存款侧的分部门段锚点。两侧是同一个切换点、同一个结论（Leader 实测）。
	loanSectorAnchor    = "分部门看"
	depositSectorAnchor = "其中，住户存款"
)

// sectorPeriodRE 用来在分部门段之前找**最近的**期次前缀。
//
// 它直接由 profiles.go 的 periodPat 编译，**不另写一份期次词表**——两份词表迟早
// 分叉，而分叉的表现是某类报告静默走错口径分支（与本文件开头纪律同源）。
//
// 按本包分工这条编译结果属于 profiles.go；放这里是**范围约束**：profiles.go 不在
// M1c-3a 的 TASK-009 的 writes 里。词表本身仍只有一份，所以分叉风险没有被引入。
var sectorPeriodRE = regexp.MustCompile(periodPat)

// —— 分部门抽取的覆盖面（M1c-3a 的 TASK-011，QA R2）——
//
// 这两个函数列出**抽取真正会去命中的全部模板**，供 checkSectorCaliber 定位被守护的那一段。
// 它们与 extractDepositSection / extractLoanSection 从**同一批数据**（depositItems /
// loanScopes）派生，不另写一份名单 —— 判据的定义域与被守护动作的定义域必须同源，
// 而「两份名单迟早分叉」正是本文件开头那条纪律。
func depositSectorCoverage() []*regexp.Regexp {
	out := make([]*regexp.Regexp, 0, len(depositItems))
	for _, it := range depositItems {
		out = append(out, sectorFlowRE(it.name))
	}
	return out
}

// loanSectorCoverage 比存款侧多收作用域锚点：extractLoanSection 先用 loanScopeSpans
// 按锚点切段、再在段内抽子项，**锚点本身也是抽取会碰到的定位点**。
func loanSectorCoverage() []*regexp.Regexp {
	out := make([]*regexp.Regexp, 0, 2*len(loanScopes))
	for _, sc := range loanScopes {
		out = append(out, sc.anchorRE)
		for _, it := range sc.items {
			out = append(out, sectorFlowRE(it.name))
		}
	}
	return out
}

// sectorSegmentStart 求「被守护那一段」的起点，判据与**抽取的覆盖面同源**。
//
// 取锚点与全部覆盖面模板里**最靠前**的那个命中位置：抽取会碰到的最早那一句，其口径由
// 它之前的期次前缀决定。全都不命中 → -1，表示本节没有分部门段。
//
// ⚠️ **锚点这一项实测是惰性的，仍然保留**——这话是量出来的，不是推的：把它整项去掉后
// 全语料 **160 个分部门段判定变化 0 个**（`05b50be`），变异 M8 在测试套件上同样 SURVIVED
// （**那一格是预先声明「应当绿」的**，不是事后解释）。保留的理由只有一条：它只会让被
// 检查的前缀窗口**变宽**，即失效方向是「多报一次口径不对」而不是「静默放行」；去掉它则
// 判据完全依赖模板位置，某天出现「锚点在前、模板命中在后且中间夹着累计前缀」的新体例时
// 会往放行的方向错。⇒ 一行、失效方向安全、已标明惰性。**别把它当成「有守卫的东西」。**
func sectorSegmentStart(body, anchor string, coverage []*regexp.Regexp) int {
	start := -1
	take := func(i int) {
		if i >= 0 && (start < 0 || i < start) {
			start = i
		}
	}
	take(strings.Index(body, anchor))
	for _, re := range coverage {
		if loc := re.FindStringIndex(body); loc != nil {
			take(loc[0])
		}
	}
	return start
}

// checkSectorCaliber 校验分部门段的口径：取**被守护那段的起点**之前最近的期次前缀，
// 查 cumulativePeriods。不在表里就报错，一个分部门字段都不产出。
//
// 三种返回：
//
//	本节没有分部门段    → nil，本守卫**不表态**（见下）
//	起点前无期次前缀    → 报错（读不出口径，不猜）
//	前缀不在累计表里    → 报错（当月口径，拒绝装进 *_ytd）
//
// **没有分部门段时刻意放行**，不是漏判：那时「口径」这个问题不适用，而分部门字段是
// 必需的，紧随其后的 mustMatch / loanScopeSpans 会以「分部门 X not found」或
// 「scope anchor not found」报错——那条信息比「读不出口径」更具体。让更具体的先说话。
// 由 TestSectorCaliberStaysSilentWhenNoSectorSegment 钉住（那是 D4 消融逼出来的）。
//
// 🔴 **「有没有分部门段」一度只问锚点短语在不在，那是错的**（M1c-3a 的 TASK-011，QA R2）：
// 抽取侧 `extractDepositSection` 拿 `sectorFlowRE` 扫整节、`extractLoanSection` 按
// `loanScopeSpans` 的锚点切段，**两者都不依赖那个文本锚点** ⇒ 锚点缺席时守卫沉默放行
// 而抽取照做。真实语料 2022-04 两侧都是这一类（全语料 160 个段里也只有这 2 个）。
// 它今天不出事只因那一期恰好也没有累计合计句、在更早一道闸就被拒了 ——
// **安全性来自与本守卫无关的巧合**。
//
// ⚠️ **修法不是换一个更宽的锚点短语**：失效模式是**判据的定义域与被守护动作的定义域
// 不一致**，换个短语仍然是两个不同的定义域。故起点改由 sectorSegmentStart 从**抽取
// 用的同一批模板**求出（depositSectorCoverage / loanSectorCoverage）。
func checkSectorCaliber(body, anchor, what string, coverage []*regexp.Regexp) error {
	i := sectorSegmentStart(body, anchor, coverage)
	if i < 0 {
		return nil
	}
	prefixes := sectorPeriodRE.FindAllString(body[:i], -1)
	if len(prefixes) == 0 {
		return fmt.Errorf(
			"hestia: %s（起点在第 %d 字节）之前没有任何期次前缀，读不出它是累计还是当月口径: "+
				"refusing to guess — 猜错会把当月值装进 *_ytd 字段，量级完全合理而口径是错的",
			what, i)
	}

	// 取**最近**的那个：累计句与当月句可能同时出现（2023-08 两句都有），
	// 决定分部门口径的是紧挨着它的那一句，不是本节里的第一句。
	last := prefixes[len(prefixes)-1]
	if !cumulativePeriods[last] {
		return fmt.Errorf(
			"hestia: %s的期次前缀 %q 不是累计口径，拒绝把当月分部门值装进 *_ytd 字段: "+
				"同一份观测里合计字段取的是累计值，混进当月的分部门值会让内部口径不一致，"+
				"而两者都在合法量级内、下游没有任何闸门拦得住",
			what, last)
	}
	return nil
}

func extractDepositSection(sec section) (map[string]float64, error) {
	c := newCollector()

	m, err := selectRMBBalance(depositBalanceRE, sec.Body, currencyRMB+"存款余额")
	if err != nil {
		return nil, err
	}
	if err := setBalanceAndYoY(c, FieldDepositBalance, FieldDepositBalanceYoY, m[2], m[3], m[4], m[5]); err != nil {
		return nil, err
	}

	m, err = selectRMBCumulativeFlow(depositFlowRE, sec.Body, currencyRMB+"存款期内合计")
	if err != nil {
		return nil, err
	}
	if err := setFlow(c, FieldDepositFlowYTD, m[3], m[4], m[5]); err != nil {
		return nil, err
	}

	// 分部门段的口径守卫（M1c-3a 的 TASK-009）：必须在抽任何分部门字段**之前**。
	// 放在循环里逐项判等于把同一个结论算四遍，且第一项抽完再报错时 collector 已脏。
	if err := checkSectorCaliber(sec.Body, depositSectorAnchor, currencyRMB+"存款分部门段", depositSectorCoverage()); err != nil {
		return nil, err
	}

	// 四个部门名互不重叠，不需要作用域
	for _, it := range depositItems {
		m, err := mustMatch(sectorFlowRE(it.name), sec.Body, "存款分部门 "+it.name)
		if err != nil {
			return nil, err
		}
		if err := setFlow(c, it.field, m[1], m[2], m[3]); err != nil {
			return nil, err
		}
	}
	return c.values, nil
}

// extractLoanSection 是唯一需要作用域的板块。
//
// 「短期贷款」「中长期贷款」在住户与企业两处各出现一次，指向不同字段。作用域
// 从锚点起、到下一个锚点止。
func extractLoanSection(sec section) (map[string]float64, error) {
	c := newCollector()

	m, err := selectRMBBalance(loanBalanceRE, sec.Body, currencyRMB+"贷款余额")
	if err != nil {
		return nil, err
	}
	if err := setBalanceAndYoY(c, FieldLoanBalance, FieldLoanBalanceYoY, m[2], m[3], m[4], m[5]); err != nil {
		return nil, err
	}

	m, err = selectRMBCumulativeFlow(loanFlowRE, sec.Body, currencyRMB+"贷款期内合计")
	if err != nil {
		return nil, err
	}
	if err := setFlow(c, FieldLoanFlowYTD, m[3], m[4], m[5]); err != nil {
		return nil, err
	}

	// 分部门段的口径守卫（M1c-3a 的 TASK-009），先于作用域切分：
	// 口径不对时整段分部门都不该抽，没必要先算作用域边界。
	if err := checkSectorCaliber(sec.Body, loanSectorAnchor, currencyRMB+"贷款分部门段", loanSectorCoverage()); err != nil {
		return nil, err
	}

	spans, err := loanScopeSpans(sec.Body)
	if err != nil {
		return nil, err
	}
	for i, sp := range spans {
		end := len(sec.Body)
		if i+1 < len(spans) {
			end = spans[i+1].start
		}
		scopeText := sec.Body[sp.start:end]

		if sp.scope.totalField != "" {
			m, err := mustMatch(scopeTotalRE(sp.scope), scopeText, "作用域合计 "+sp.scope.anchorRE.String())
			if err != nil {
				return nil, err
			}
			if err := setFlow(c, sp.scope.totalField, m[1], m[2], m[3]); err != nil {
				return nil, err
			}
		}

		for _, it := range sp.scope.items {
			m, err := mustMatch(sectorFlowRE(it.name), scopeText, "作用域子项 "+it.name)
			if err != nil {
				return nil, err
			}
			if err := setFlow(c, it.field, m[1], m[2], m[3]); err != nil {
				return nil, err
			}
		}
	}
	return c.values, nil
}

type loanSpan struct {
	scope loanScope
	start int
}

// loanScopeSpans 定位每个作用域的起点，并校验它们的先后顺序。
//
// 锚点在原文里本就按住户→企业→非银的顺序出现。这里不排序而是校验——若某期换了
// 顺序，作用域边界会算错，那种错会让企业的短期贷款落进住户字段：值合理而字段错，
// 是本文件最难在下游被发现的一类。
//
// ⚠️ 这道校验**同时是切片越界的唯一防线**，而不只是语义守卫。extractLoanSection
// 用 `sec.Body[sp.start:end]` 切段，其中 `end = spans[i+1].start`——**升序保证一旦
// 失效，那句切片就会 panic**（变异实测：去掉本校验后是 `panic: slice bounds out of
// range [287:123]` 中断整个测试二进制，不是干净的断言失败）。生产路径上不会发生
// （真实报告的锚点有序，无序时本函数已先返回 error），故不是缺陷；但删改本段的人
// 需要知道自己动的是内存安全而不仅是字段归属。若将来改成「排序后再切」，
// 记得排序本身也要保证严格递增——相等的起点同样会切出负长度。
func loanScopeSpans(body string) ([]loanSpan, error) {
	spans := make([]loanSpan, 0, len(loanScopes))
	for _, sc := range loanScopes {
		loc := sc.anchorRE.FindStringIndex(body)
		if loc == nil {
			return nil, fmt.Errorf("hestia: loan scope anchor %s not found", sc.anchorRE)
		}
		spans = append(spans, loanSpan{scope: sc, start: loc[0]})
	}
	for i := 1; i < len(spans); i++ {
		if spans[i].start <= spans[i-1].start {
			return nil, fmt.Errorf(
				"hestia: loan scope anchors are out of expected order (%s at %d, %s at %d): "+
					"scope slicing would assign sub-items to the wrong sector",
				spans[i-1].scope.anchorRE, spans[i-1].start,
				spans[i].scope.anchorRE, spans[i].start)
		}
	}
	return spans, nil
}

// plainNumberItem 是「一条正则 → 一个字段」的读数句：既无方向词也无单位，
// 数值直接就是结果。利率与外汇两节都是这个形态。
type plainNumberItem struct {
	re    *regexp.Regexp
	field string
	what  string
}

func extractPlainNumbers(body string, items []plainNumberItem) (map[string]float64, error) {
	c := newCollector()
	for _, it := range items {
		m, err := mustMatch(it.re, body, it.what)
		if err != nil {
			return nil, err
		}
		v, err := parsePlainNumber(m[1])
		if err != nil {
			return nil, err
		}
		if err := c.set(it.field, v); err != nil {
			return nil, err
		}
	}
	return c.values, nil
}

// 利率无方向词也无单位。
func extractRateSection(sec section) (map[string]float64, error) {
	return extractPlainNumbers(sec.Body, []plainNumberItem{
		{rateIBORE, FieldRateIBO, "同业拆借利率"},
		{rateRepoRE, FieldRateRepo, "质押式回购利率"},
	})
}

// 外储原文就是万亿美元、汇率是元/美元，两者都不属于 fields.go 的三类单位约定，
// 也都没有方向词——直接取读数。
func extractFXSection(sec section) (map[string]float64, error) {
	return extractPlainNumbers(sec.Body, []plainNumberItem{
		{fxReserveRE, FieldFXReserve, "国家外汇储备"},
		{fxRateRE, FieldFXRate, "人民币汇率"},
	})
}

// setBalanceAndYoY 写一对「余额 + 同比」。
//
// 余额句原文**没有方向词**（「余额为442.12万亿元」「余额340.29万亿元」），所以走
// parsePlainAmount 这个恒正入口；同比有方向词，符号从它读。
func setBalanceAndYoY(c *collector, balanceField, yoyField, num, unit, dir, ratio string) error {
	a, err := parsePlainAmount(num, unit)
	if err != nil {
		return err
	}
	if err := c.set(balanceField, a.toWanYi()); err != nil {
		return err
	}
	yoy, err := parseRatio(dir, ratio)
	if err != nil {
		return err
	}
	return c.set(yoyField, yoy)
}

// setFlow 写一个增量字段：有方向词，归一到亿元。
func setFlow(c *collector, field, dir, num, unit string) error {
	a, err := newAmount(dir, num, unit)
	if err != nil {
		return err
	}
	return c.set(field, a.toYi())
}

// —— M1c-3a 的 TASK-002：社融存量/增量**独立报告**的整篇抽取 ——
//
// 央行同期发三篇：《金融统计数据报告》（有板块结构，走 splitSections →
// detectExtractor → extractFields）与社融存量/增量两篇独立报告。后两篇**没有板块
// 结构**——整篇就是一段正文，没有「一、二、三」标题可切。
//
// 整篇当一节即可复用既有抽取函数：section 只有 Title / Body 两个字段，而
// extractTSF*Section 只读 Body——它本就不知道自己是被板块切分喂的还是被整篇喂的。
//
// 这两个函数在 M1c-3a 的 TASK-007（Parse 按 kind 分派）接上之前**没有生产调用方**，
// 这是刻意的分层，不是漏接。

// extractTSFStockArticle 抽取一篇《社会融资规模存量统计数据报告》的全文。
//
// 存量侧真的只是一行包装：全量实测 69 篇，整篇喂进去与板块喂进去的结果没有差别。
// 报告第二段「从结构看」里分项名逐字相同而数值是占比，既有模板已经挡住了它
// （tsfStockRE 要求「余额」后紧跟数值 + 单位，tsfStockTotalRE 要求「存量**为**」），
// 故这里不需要增量侧那样的作用域切分。TestTSFStockArticleTakesBalanceNotStructureShare
// 钉住这个已经正确的性质。
func extractTSFStockArticle(text string) (map[string]float64, error) {
	return extractTSFStockSection(section{Body: text})
}

// tsfFlowArticleTotalRE 匹配独立增量报告里的**任意一条**总量句，两种措辞都收。
//
// 捕获组：1=年 2=月（仅「YYYY年M月」这种前缀才有）3=「累计」标记 4=数值 5=单位。
//
// # 为什么它不在 profiles.go
//
// 按本包分工它属于那边（「数据：字段清单表与句式模板」）。放这里是**范围约束**：
// profiles.go 本 wave 归 M1c-3a 的 TASK-001 与 TASK-005，不在本任务的 writes 里。
// 代价是它落在 profiles_test.go 的 TestNoGreedyCaptureInTemplates 覆盖之外——
// extract_test.go 的 TestExtractGoArticleTemplatesHaveNoGreedyCapture 按同一判据补上。
// profiles.go 空出来之后应当把这条挪过去，并登记进 allTemplateRegexps()。
//
// # 为什么不是把 tsfFlowTotalRE 放宽成「增量(?:累计)?为」
//
// 全量实测 69 篇，总量句是**四类**而不是两类：
//
//	仅「累计为」                19 篇   ← 现状已成功
//	仅「为」且 1 月报            6 篇   ← 1 月的累计=当月，安全
//	仅「为」且非 1 月           19 篇   ← 那句是**当月**值，报告不含累计数据
//	两者都有                    25 篇   ← 现状已成功
//
// 天真放宽会让最后那 25 篇同时命中两句，mustMatch 报 matched 2 sentences ——
// **原本成功的 25 篇会被打坏**。所以这里不放宽模板，而是把两种措辞都收下来，
// 再按口径挑唯一一条，与 selectRMBCumulativeFlow 对付孪生句的手法同构。
var tsfFlowArticleTotalRE = regexp.MustCompile(
	`(?:([0-9]{4})年([0-9]{1,2})月)?社会融资规模增量(累计)?为` + numPat + unitPat)

// tsfFlowTotal 是一条总量句：捕获组 + 它在原文里的起点。
//
// 起点是必需的，不是顺手记的：分项句紧跟总量句，口径由**所在段**决定，
// 故分项只能在被选中的那条总量句的作用域内抽。
type tsfFlowTotal struct {
	groups []string
	start  int
}

// isCumulative 判定一条总量句是不是「年初至今累计」口径。
//
// 两条判据，缺一不可：
//
//	带「累计」二字                          → 累计（19 + 25 篇）
//	「YYYY年1月」前缀且不带「累计」          → 累计（6 篇：1 月的年初至今就是当月）
//
// 其余一律当单月。**「YYYY年10月…增量为」必须判成单月**：那 19 篇报告确实
// 只有当月数，把它填进名为 _ytd 的字段会得到一个量级完全合理而口径错误的值，
// 下游 calibrate 会拿它跨期比——本包反复禁止的失败方式。
//
// ⚠️ 判据落在「1 月」而不是「一位数月份」：`([0-9]{1,2})` 对「10月」捕获的是
// 「10」，与「1」不等，靠的是捕获组而不是字符串前缀。
func (h tsfFlowTotal) isCumulative() bool {
	if h.groups[3] == "累计" {
		return true
	}
	return h.groups[1] != "" && h.groups[2] == "1"
}

// label 只输出用于分辨的限定词，不输出数值——错误信息里带上隔壁句的数字
// 会让人误以为那是结果（与 selectUnique 的 label 同一约定）。
func (h tsfFlowTotal) label() string {
	period := "无期次前缀"
	if h.groups[1] != "" {
		period = h.groups[1] + "年" + h.groups[2] + "月"
	}
	caliber := "单月"
	if h.isCumulative() {
		caliber = "累计"
	}
	return period + "/" + caliber
}

// extractTSFFlowArticle 抽取一篇《社会融资规模增量统计数据报告》的全文。
//
// 与存量侧不同，这里**不能**只做一行包装：增量报告常同时载有累计段与当月段，
// 两段各带一整套同形的分项句。实测 2022 年 7/8/10/11 四篇的体例是「当月句 +
// 一整套当月分项……累计句孤悬段末」，整篇直接喂给 extractTSFFlowSection 会
// err=nil 而抽出**累计总量 + 当月分项**：
//
//	tsf_flow_ytd          = 287000   ← 段末「1-10月，…累计为28.7万亿元」
//	tsf_flow_rmb_loan_ytd =   4431   ← 段首「10月…人民币贷款增加4431亿元」
//
// 量级差约 30 倍，两个值又都在合法区间内，下游没有任何闸门拦得住。故这里按
// 作用域切分：总量句按口径挑唯一一条，分项只在**该句之后、下一条总量句之前**
// 的文本里抽。切法与 extractLoanSection 的 loanScopeSpans 同构，只是边界由
// 总量句而不是部门锚点划定。
func extractTSFFlowArticle(text string) (map[string]float64, error) {
	totals := findTSFFlowTotals(text)

	labels := make([]string, 0, len(totals))
	cumulative := make([]tsfFlowTotal, 0, 1)
	for _, h := range totals {
		labels = append(labels, h.label())
		if h.isCumulative() {
			cumulative = append(cumulative, h)
		}
	}

	switch len(cumulative) {
	case 1:
	case 0:
		return nil, fmt.Errorf(
			"hestia: 社融增量总量（年初至今累计口径）not found among %d candidate sentence(s) [%s]: "+
				"refusing to fall back to a current-month sentence — it has the right magnitude "+
				"and format but the wrong caliber, and the field is named *_ytd",
			len(totals), strings.Join(labels, " "))
	default:
		return nil, fmt.Errorf(
			"hestia: 社融增量总量 matched %d cumulative sentences [%s]: refusing to pick one, "+
				"both values look plausible and leftmost-first would choose silently",
			len(cumulative), strings.Join(labels, " "))
	}
	total := cumulative[0]

	c := newCollector()
	a, err := parsePlainAmount(total.groups[4], total.groups[5]) // 总量句无方向词
	if err != nil {
		return nil, err
	}
	if err := c.set(FieldTSFFlowYTD, a.toYi()); err != nil {
		return nil, err
	}

	scope := text[total.start:tsfFlowScopeEnd(totals, total.start, len(text))]
	for _, it := range tsfFlowItems {
		m, err := mustMatch(tsfFlowRE(it.name), scope, "社融增量分项 "+it.name)
		if err != nil {
			return nil, err
		}
		if err := setFlow(c, it.field, m[1], m[2], m[3]); err != nil {
			return nil, err
		}
	}
	return c.values, nil
}

// findTSFFlowTotals 找出全文所有总量句。用 FindAllStringSubmatchIndex 而不是
// FindAllStringSubmatch，因为作用域切分要的正是位置。
func findTSFFlowTotals(text string) []tsfFlowTotal {
	locs := tsfFlowArticleTotalRE.FindAllStringSubmatchIndex(text, -1)
	out := make([]tsfFlowTotal, 0, len(locs))
	for _, loc := range locs {
		groups := make([]string, len(loc)/2)
		for i := range groups {
			// 未参与匹配的可选组，起止都是 -1 —— 留空串
			if loc[2*i] >= 0 {
				groups[i] = text[loc[2*i]:loc[2*i+1]]
			}
		}
		out = append(out, tsfFlowTotal{groups: groups, start: loc[0]})
	}
	return out
}

// tsfFlowScopeEnd 求作用域右边界：位置上**紧随其后**的那条总量句的起点，
// 没有则到文末。
//
// 边界按位置取、不按口径取：2022 年 10 月那一篇被选中的累计句就是最后一条，
// 于是作用域里一个分项都没有 ⇒ mustMatch 零命中 ⇒ 响亮失败。那正是期望行为，
// 该报告确实不含累计口径的分项。
func tsfFlowScopeEnd(totals []tsfFlowTotal, start, end int) int {
	for _, h := range totals {
		if h.start > start && h.start < end {
			end = h.start
		}
	}
	return end
}
