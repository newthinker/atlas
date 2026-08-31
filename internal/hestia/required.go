package hestia

import "slices"

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
		out = append(out, it.field)
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
		out = append(out, it.field)
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

// requiredFields 返回某个 extractor 的期次应当出现的全部字段。
// 未知或语义不足的 extractor 返回 nil，调用方据此记 skipped 而不是 failed。
func requiredFields(extractor string) []string {
	switch extractor {
	case extractorV2:
		// Clone 而不是直接返回：fieldOrder 是 DDL、INSERT 列、白名单的
		// 共同真相源，交出底层数组等于把三者一起交出去。
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
// （2020-01|monthly 只有 2 篇，月报那篇落在解析失败格里）。硬套会让这类组恒报「缺 27
// 个字段」，而那些字段在该组里是 absent-by-design —— 把 by-design 的缺席记成 failed，
// completeness 这个指标就废了。这条是本函数存在的**全部**原因。
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
