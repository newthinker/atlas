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

// requiredFields 返回某个 extractor 的期次应当出现的全部字段。
// 未知或语义不足的 extractor 返回 nil，调用方据此记 skipped 而不是 failed。
func requiredFields(extractor string) []string {
	switch extractor {
	case extractorV2:
		// Clone 而不是直接返回：fieldOrder 是 DDL、INSERT 列、白名单的
		// 共同真相源，交出底层数组等于把三者一起交出去。
		return slices.Clone(fieldOrder)
	case extractorV1:
		section := tsfSectionFields()
		tsf := make(map[string]bool, len(section))
		for _, f := range section {
			tsf[f] = true
		}
		out := make([]string, 0, len(fieldOrder)-len(tsf))
		for _, f := range fieldOrder {
			if !tsf[f] {
				out = append(out, f)
			}
		}
		return out
	default:
		// llm-fallback@v1 落在这里，见 TestRequiredFieldsRejectsAmbiguousExtractor。
		return nil
	}
}
