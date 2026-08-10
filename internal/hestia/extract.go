package hestia

import (
	"fmt"
	"regexp"
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
	v2Only  bool // 社融两节只在 rule@v2 期次存在
	fn      func(section) (map[string]float64, error)
}

var sectionRules = []sectionRule{
	{tsfSectionKeyword, true, extractTSFStockSection},
	{"社会融资规模增量", true, extractTSFFlowSection},
	{"广义货币", false, extractMoneySection},
	{"人民币存款", false, extractDepositSection},
	{"人民币贷款", false, extractLoanSection},
	{"加权平均利率", false, extractRateSection},
	{"国家外汇储备", false, extractFXSection},
}

// extractFields 按模板版本抽取全部字段。
//
// rule@v1 显式跳过社融两节——那期报告没有它们。显式跳过比「碰巧匹配不到」更
// 清楚：前者是声明，后者是巧合，而巧合会在某期正文里偶然出现社融字样时失效。
func extractFields(secs []section, extractor string) (map[string]float64, error) {
	if extractor != extractorV1 && extractor != extractorV2 {
		return nil, fmt.Errorf(
			"hestia: unknown extractor %q (known: %s, %s): refusing to guess a template set",
			extractor, extractorV1, extractorV2)
	}

	c := newCollector()
	for _, rule := range sectionRules {
		if rule.v2Only && extractor != extractorV2 {
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

// mustMatch 跑一条正则并要求命中，未命中时给出可定位的错误。
func mustMatch(re *regexp.Regexp, body, what string) ([]string, error) {
	m := re.FindStringSubmatch(body)
	if m == nil {
		return nil, fmt.Errorf("hestia: %s not found (pattern %s)", what, re)
	}
	return m, nil
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

func extractTSFFlowSection(sec section) (map[string]float64, error) {
	c := newCollector()

	m, err := mustMatch(tsfFlowTotalRE, sec.Body, "社融增量总量")
	if err != nil {
		return nil, err
	}
	a, err := parsePlainAmount(m[1], m[2]) // 「增量累计为35.6万亿元」无方向词
	if err != nil {
		return nil, err
	}
	if err := c.set(FieldTSFFlowYTD, a.toYi()); err != nil {
		return nil, err
	}

	for _, it := range tsfFlowItems {
		m, err := mustMatch(tsfFlowRE(it.name), sec.Body, "社融增量分项 "+it.name)
		if err != nil {
			return nil, err
		}
		if err := setFlow(c, it.field, m[1], m[2], m[3]); err != nil {
			return nil, err
		}
	}
	return c.values, nil
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
