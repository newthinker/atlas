package hestia

import (
	"slices"
	"strings"
)

// tsfSectionFields 返回社融两节（sectionRules 里标 v2Only 的那两条）产出的
// 全部 27 个字段。
//
// 前两组是真派生——遍历模板表，往 tsfStockItems 加一个分项自动纳入。第三组的
// 三个总量字段**只能手写**：profiles.go 的 singletonFields() 把所有单字段规则
// 混在一个扁平列表里（存贷款余额、期内合计、利率、外汇、社融总量），结构上
// 无法按板块区分。这是本包里唯一手写的字段归属，由 golden 键集比对钉住。
//
// 不用 strings.HasPrefix(f, "tsf_") 代替：前缀与板块归属**当前**恰好一致，
// 但那是巧合，模板表才是事实。两个真相源分叉时，先错的一定是前缀。
func tsfSectionFields() []string {
	out := make([]string, 0, 2*len(tsfStockItems)+len(tsfFlowItems)+3)
	for _, it := range tsfStockItems {
		out = append(out, it.balanceField, it.yoyField)
	}
	// 🔴 **两种口径都要列**（M1c-4 的 TASK-006）：TASK-005 之后社融增量节按口径路由，
	// 同一节可能产出 *_ytd 也可能产出 *_mom。本函数回答的是「这两节**会产出哪些列**」，
	// 而不是「这一篇实际产出了哪些」—— 少列 _mom 那 9 个，`without(fieldOrder, 这里)`
	// 就会把它们留在 v1（**根本没有社融节**的报告）的必填集里。
	//
	// 实测（TASK-006 施工中）：只列 _ytd 时 requiredFields(rule@v1) 对一篇标准 v1 样本
	// 报缺 9 个 tsf_flow_*_mom —— 而那一篇连社融节都没有。口径感知救不了它：
	// **两侧都不在场**，缺的是整族，那正是 missingCaliberAware 该报缺的情形。
	// ⚠️ 社融**存量**没有当月口径（tsfStockItems 只有 balance/yoy），故不加。
	for _, it := range tsfFlowItems {
		out = append(out, it.ytdField, it.momField)
	}
	return append(out, FieldTSFStock, FieldTSFStockYoY, FieldTSFFlowYTD, FieldTSFFlowMoM)
}

// tsfStockFields / tsfFlowFields 把上面那 27 个字段按**报告**切成两半：社融存量
// 独立报告只含前者（18），社融增量独立报告只含后者（9）。M1c-3a 的 TASK-003 加，
// 依据 AD-1。
//
// 两个总量归存量、一个归增量，是按各自报告的正文分的——tsf_stock / tsf_stock_yoy
// 出自存量报告，tsf_flow_ytd 出自增量报告。这三个的归属和上面一样是手写的（同一个
// 理由：singletonFields() 无板块归属），由 TestTSFStandaloneFieldsPartitionTSFSection
// 逐个钉住：并集/交集/数量三条断言**都抓不到它们被划反**。
//
// ⚠️ 别顺手把 tsfSectionFields() 改写成 append(tsfStockFields(), tsfFlowFields()...)。
// 看着是消重，实则把「两半合起来正好是那 27 个」从**可证伪的断言**变成恒真——那条
// 断言的全部价值就在于两边是各自独立派生出来的。
func tsfStockFields() []string {
	out := make([]string, 0, 2*len(tsfStockItems)+2)
	for _, it := range tsfStockItems {
		out = append(out, it.balanceField, it.yoyField)
	}
	return append(out, FieldTSFStock, FieldTSFStockYoY)
}

func tsfFlowFields() []string {
	// 两种口径都列，理由同 tsfSectionFields：本函数回答「这一类报告会产出哪些列」。
	// 口径感知（missingCaliberAware）保证单篇只产一侧时不算缺。
	out := make([]string, 0, 2*len(tsfFlowItems)+2)
	for _, it := range tsfFlowItems {
		out = append(out, it.ytdField, it.momField)
	}
	return append(out, FieldTSFFlowYTD, FieldTSFFlowMoM)
}

// without 返回 all 里不在 drop 中的字段，**永远是新切片**——三个分支都靠它交出
// 副本而不是底层数组（fieldOrder 是 DDL、INSERT 列、白名单的共同真相源）。
func without(all, drop []string) []string {
	skip := make(map[string]bool, len(drop))
	for _, f := range drop {
		skip[f] = true
	}
	out := make([]string, 0, len(all))
	for _, f := range all {
		if !skip[f] {
			out = append(out, f)
		}
	}
	return out
}

// fxSectionFields 返回外汇板块（sectionRules 里「国家外汇储备」那条，
// extractFXSection）产出的两个字段。
//
// 手写，理由与上面那三个总量字段**完全相同**：singletonFields() 把所有单字段
// 规则混在一个扁平列表里，结构上无法按板块区分。
//
// 手写就必须有人钉住：TestFXSectionFieldsMatchExtractor 拿 extractFXSection 的
// 真实产出与本函数对绑，两个真相源分叉时立刻转红。
func fxSectionFields() []string {
	return []string{FieldFXReserve, FieldFXRate}
}

// caliberFamilies 列出全部「同一指标的两种口径」列对，形如 {x_ytd, x_mom}
// （M1c-4 的 TASK-006）。
//
// 🔴 **从 fieldOrder 按列名派生，不手写第二份名单**：手写的那份迟早与 fieldOrder
// 分叉，而先错的一定是手抄的那份（与 tsfSectionFields / momFields 的注释同一条理由）。
// 配对判据就是列名本身——`x_mom` 的孪生是同词干的 `x_ytd`，两者都必须在 fieldOrder 里。
//
// ⚠️ **只配对真的成对的列**：存量、余额、同比、利率、外汇都没有当月口径的对应物，
// 它们不进这张表 ⇒ missingCaliberAware 对它们原样逐个要求。少了这条约束，
// 「口径感知」会顺手放松掉一大批与本迭代无关的字段，而正向测试全绿。
// 由 TestMissingCaliberAwareLeavesUnpairedFieldsAlone 守。
func caliberFamilies() [][2]string {
	inOrder := make(map[string]bool, len(fieldOrder))
	for _, f := range fieldOrder {
		inOrder[f] = true
	}
	var out [][2]string
	for _, f := range fieldOrder {
		stem, ok := strings.CutSuffix(f, "_mom")
		if !ok {
			continue
		}
		if ytd := stem + "_ytd"; inOrder[ytd] {
			out = append(out, [2]string{ytd, f})
		}
	}
	return out
}

// missingCaliberAware 返回 want 里真正缺失的字段：**同族两列任一在场即不算缺**，
// 结果按 fieldOrder 排序（M1c-4 的 TASK-006）。
//
// 🔴 **本函数存在的理由**：真语料里有 54 篇的分部门段走当月口径，硬要 *_ytd 会让它们
// 恒报「缺一整族字段」—— 那是把 **absent-by-design 记成 failed**，completeness 这个指标
// 就废了（types.go 原话，mergedRequiredFields 因同一理由存在）。
//
// ⚠️ **放松只在同族两列之间**，不是「缺了就去找个像的顶上」：twin 只由 caliberFamilies
// 建，不成对的字段查不到孪生 ⇒ 原样算缺。
//
// ⚠️ **两个方向都要建**（ytd→mom 与 mom→ytd）。只建一个方向的实现能让「want 要 ytd 而
// values 只有 mom」那一格全绿，而反方向在真实路径上会把整族报成缺失 —— 而 TASK-005 落地
// 之后 requiredFields 里**确实含 _mom 列**，那个方向是活的。
// 由 TestMissingCaliberAwareMomToYTDIsReachableOnRealWant 用真实 want 钉住。
func missingCaliberAware(want []string, values map[string]float64) []string {
	twin := make(map[string]string, 2*len(fieldOrder))
	for _, p := range caliberFamilies() {
		twin[p[0]], twin[p[1]] = p[1], p[0]
	}

	var missing []string
	for _, f := range want {
		if _, ok := values[f]; ok {
			continue
		}
		if other, paired := twin[f]; paired {
			if _, ok := values[other]; ok {
				continue
			}
		}
		missing = append(missing, f)
	}

	// 按 fieldOrder 排序：错误信息里的字段顺序要与 DDL/报表一致，否则同一份缺失
	// 在两处读起来像两回事。⚠️ **不是按 want 的顺序**——want 可能是 mergedRequiredFields
	// 合成的，它的顺序取决于由哪几篇合成。
	pos := make(map[string]int, len(fieldOrder))
	for i, f := range fieldOrder {
		pos[f] = i
	}
	slices.SortFunc(missing, func(a, b string) int { return pos[a] - pos[b] })
	return missing
}

// requiredFields 返回某个 extractor 的期次应当出现的全部字段。
// 未知或语义不足的 extractor 返回 nil，调用方据此记 skipped 而不是 failed。
func requiredFields(extractor string) []string {
	switch extractor {
	case extractorV2:
		// 🔴 M1c-4 的 TASK-006：TASK-004 加的过渡性 `without(fieldOrder, momFields())`
		// 已退场，momFields 一并删除。*_mom 列现在**正常进必填集**，口径感知由
		// gateCompleteness 的 missingCaliberAware 承担 —— 那正是 TASK-006 存在的理由。
		// 留着那个 without 会让 want 恒为单侧，missingCaliberAware 的 mom→ytd 方向
		// 在真实路径上永不走到，而单测（手工构造 want）照样全绿。
		//
		// slices.Clone 而不是直接返回 fieldOrder：交出的必须是副本 —— fieldOrder 是
		// DDL、INSERT 列、白名单的共同真相源，交出底层数组等于把三者一起交出去。
		return slices.Clone(fieldOrder)
	case extractorV1:
		return without(fieldOrder, tsfSectionFields())

	// —— 月报：对应季报的必填集精确减去外汇板块那两个字段（M1c-3a 的 TASK-003）——
	//
	// 55 篇实测里只有 2 篇月报有「国家外汇储备」板块，其余 53 篇正文根本没有
	// fx_reserve / fx_rate 那两句 —— 那不是缺失，是 absent-by-design。
	// 写成「季报 − 外汇节」而不是另列一份 25 / 52 个字段的清单：两份清单会分叉，
	// 而先错的一定是手抄的那份。
	case extractorMonthlyV1:
		return without(requiredFields(extractorV1), fxSectionFields())
	case extractorMonthlyV2:
		return without(requiredFields(extractorV2), fxSectionFields())

	// —— 社融独立报告：整篇只讲社融一节，各占 27 个字段的一半 ——
	//
	// ⚠️ 这两种报告与同期的月报共享 (period, period_type) 业务键，三篇直接 Save
	// 会互相覆盖。合并成一个完整观测是 M1c-3b 的事，本迭代不入库。
	case extractorTSFStock:
		return tsfStockFields()
	case extractorTSFFlow:
		return tsfFlowFields()

	default:
		// llm-fallback@v1 与 merged@v1 都落在这里，见
		// TestRequiredFieldsRejectsAmbiguousExtractor 与 TestRequiredFieldsRejectsBareMerged。
		// 两者的成因**不同**：前者是「还没实现」（M1c-4 补上分支后它就有必填集了），
		// 后者是「这一列结构上说不出必填集」（merged@v1 的必填集取决于由哪几篇合成，
		// 而 extractor 只有一个字符串，见 mergedRequiredFields）——它**永远**不会有分支。
		return nil
	}
}

// mergedRequiredFields 返回由 parts 那几个 extractor 合成的观测应有的字段集
// （M1c-3b 的 TASK-002）。
//
// 取并集而不是硬套 rule-monthly@v2 的 52 字段：实测 42 个合并组里并非每组都齐 3 篇
// （2020-01|monthly 只有 2 篇，月报那篇落在解析失败格里）。硬套会让这类组恒报「缺 25
// 个字段」，而那些字段在该组里是 absent-by-design —— 把 by-design 的缺席记成 failed，
// completeness 这个指标就废了。这条是本函数存在的**全部**原因。
//
// 算术：52 − (18 + 9) = 25，缺的那批恰是 rule-monthly@v1 的 25 个非社融字段。
//
// ⚠️ 这个数曾写成 27（M1c-3b 的 TASK-011 订正）。27 是**相反情形**的数：组里只剩月报
// 那一篇时，硬套 v2 少掉的才是 stock ∪ flow 的 27 个。之所以会滑过去 —— stock ∪ flow
// 恰好也是 27 个，且与 rule@v1 的 27 个完全不相交，于是 27 在本仓库另三处都**正确地**
// 存在着（types.go 讲社融独立报告复用 rule@v1 的后果、validate.go 与 validate_test.go
// 讲 v1 期次天然少掉的那批），搬到这里时它对应的却是**存在**的字段数、不是**缺失**的
// 字段数。这是「正确的局部陈述在转写时丢了限定条件」的又一例。
//
// 那三处的 27 是对的，不要顺手一起改。本文件的自查是「grep 那个错误说法在本文件中的
// 出现次数应为 0」—— 故上面刻意不引用那三处的原文、也不把该 grep 模式写成字面量：
// 任一种引用都会让这条自查在本文件上永久失效（写这段注释时实撞了两次）。
//
// 按 fieldOrder 排序输出，理由与 gateMagnitudeSanity 遍历 fieldOrder 相同：map 迭代
// 顺序随机，同一份数据两次跑报出不同的缺失字段会让排查变成猜谜。
//
// parts 里出现未知/语义不足的 extractor（含 merged@v1 自身）时，requiredFields 对它
// 返回 nil，本函数**静默略过**它 —— 那与「它没贡献任何字段」是同一件事。判「这一组
// 到底该不该算 completeness」是调用方的事，不是本函数的。
func mergedRequiredFields(parts []string) []string {
	want := make(map[string]bool)
	for _, p := range parts {
		for _, f := range requiredFields(p) {
			want[f] = true
		}
	}
	out := make([]string, 0, len(want))
	for _, f := range fieldOrder {
		if want[f] {
			out = append(out, f)
		}
	}
	return out
}
