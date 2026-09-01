package hestia

import "strings"

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
	for _, it := range tsfFlowItems {
		out = append(out, it.ytdField)
	}
	return append(out, FieldTSFStock, FieldTSFStockYoY, FieldTSFFlowYTD)
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
	out := make([]string, 0, len(tsfFlowItems)+1)
	for _, it := range tsfFlowItems {
		out = append(out, it.ytdField)
	}
	return append(out, FieldTSFFlowYTD)
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

// momFields 是 fieldOrder 里全部当月口径列（M1c-4 的 TASK-004 加的那 22 个）。
//
// 🔴 **这是一段有明确退场条件的过渡代码，退场点是 M1c-4 的 TASK-006。**
//
// TASK-004 只加列与声明，抽取侧按口径选列是 TASK-005 的事 ⇒ 在 TASK-005 落地之前
// 这 22 列**恒为空**。让它们直接进必填集，每一期的 completeness 都会立刻报「缺 22
// 个字段」—— 那是把 absent-by-design 记成 failed，正是 types.go 与
// mergedRequiredFields 都在防的同一件事。
//
// TASK-006 把 gateCompleteness 的缺失判定换成口径感知的 missingCaliberAware
// （*_ytd 与 *_mom 任一在场即不算缺）。**那一步落地后本函数与下面两处 without
// 调用应当一并删除**，requiredFields 的两个分支回到「整个 fieldOrder」与
// 「fieldOrder − 社融节」。
//
// 从 fieldOrder 按后缀派生而不是手写第二份名单：手写的那份迟早与 fieldOrder 分叉，
// 而先错的一定是手抄的那份（与 tsfSectionFields 的注释同一条理由）。
func momFields() []string {
	var out []string
	for _, f := range fieldOrder {
		if strings.HasSuffix(f, "_mom") {
			out = append(out, f)
		}
	}
	return out
}

// requiredFields 返回某个 extractor 的期次应当出现的全部字段。
// 未知或语义不足的 extractor 返回 nil，调用方据此记 skipped 而不是 failed。
func requiredFields(extractor string) []string {
	switch extractor {
	case extractorV2:
		// without 而不是 slices.Clone(fieldOrder)：*_mom 暂不进必填集，见 momFields。
		// 它同时保住了「交出的是副本」——fieldOrder 是 DDL、INSERT 列、白名单的
		// 共同真相源，交出底层数组等于把三者一起交出去。
		return without(fieldOrder, momFields())
	case extractorV1:
		return without(without(fieldOrder, tsfSectionFields()), momFields())

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
