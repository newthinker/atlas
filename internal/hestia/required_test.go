package hestia

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Context Checkpoint: done_criteria → test mapping
// functional[0]      "tsfSectionFields() 恰好 27 个且无重复"        → TestTSFSectionFieldsIsExactAndDistinct
// functional[1]      "requiredFields(extractorV2)=54，与 golden2025 双向一致" → TestRequiredFieldsMatchGoldenKeySets/rule@v2
// functional[2]      "requiredFields(extractorV1)=27，与 golden2020 双向一致" → TestRequiredFieldsMatchGoldenKeySets/rule@v1
// boundary[0]        "返回副本，改动不得影响 fieldOrder"             → TestRequiredFieldsReturnsCopy
// error_handling[0]  "llm-fallback@v1 / rule@v3 / \"\" 均返回 nil"   → TestRequiredFieldsRejectsAmbiguousExtractor
// non_functional[0]  "遍历模板表派生，不用 tsf_ 前缀"                → TestTSFSectionFieldsDerivesFromTemplateTables

// 社融两节共 27 个字段：8 个存量分项 × 2（余额 + 同比）+ 8 个增量分项 + 3 个总量。
func TestTSFSectionFieldsIsExactAndDistinct(t *testing.T) {
	got := tsfSectionFields()
	assert.Len(t, got, 27)

	// 长度对不代表内容对：遍历错一张表可能同时多算一个、漏算一个。
	seen := make(map[string]bool, len(got))
	for _, f := range got {
		assert.False(t, seen[f], "字段 %s 重复——派生把同一个字段算了两次", f)
		seen[f] = true
	}
}

// 派生必须真的**走模板表**，而不是 strings.HasPrefix(f, "tsf_") 筛 fieldOrder。
//
// 上面那三条断言（27 个、无重复、与 golden 双向一致）都杀不掉前缀实现：前缀与
// 板块归属**当前**恰好一致，前缀版会给出一模一样的 27 个字段，全绿。所以这里
// 往模板表临时插一行**前缀不是 tsf_、也不在 fieldOrder 里**的分项——真派生会
// 把它带出来，前缀实现不会。
//
// 改的是包级变量，靠 t.Cleanup 还原；本包没有任何 t.Parallel()，测试函数串行跑。
func TestTSFSectionFieldsDerivesFromTemplateTables(t *testing.T) {
	base := len(tsfSectionFields())

	origStock, origFlow := tsfStockItems, tsfFlowItems
	t.Cleanup(func() { tsfStockItems, tsfFlowItems = origStock, origFlow })

	tsfStockItems = append(slices.Clone(origStock),
		tsfStockItem{"哨兵分项", "sentinel_balance", "sentinel_balance_yoy"})
	tsfFlowItems = append(slices.Clone(origFlow),
		nameField{"哨兵分项", "sentinel_flow_ytd"})

	got := tsfSectionFields()
	assert.Len(t, got, base+3, "往模板表加了 3 个字段，派生结果却没跟着长——没在遍历模板表")
	for _, f := range []string{"sentinel_balance", "sentinel_balance_yoy", "sentinel_flow_ytd"} {
		assert.Contains(t, got, f, "模板表新增的 %s 没被派生带出来", f)
	}
}

// 派生结果必须与 golden 表的键集**双向**一致。
//
// golden 是从两份真实报告手工抄出来的，派生是从模板表算出来的——两者独立
// 得出同一个集合，才说明都对。只查一个方向会漏掉「派生多算了字段」这一半。
func TestRequiredFieldsMatchGoldenKeySets(t *testing.T) {
	tests := []struct {
		extractor string
		golden    map[string]float64
		want      int
	}{
		{extractorV2, golden2025, 54},
		{extractorV1, golden2020, 27},
	}
	for _, tt := range tests {
		t.Run(tt.extractor, func(t *testing.T) {
			req := requiredFields(tt.extractor)
			require.Len(t, req, tt.want)

			inReq := make(map[string]bool, len(req))
			for _, f := range req {
				inReq[f] = true
			}
			for _, f := range req {
				assert.Contains(t, tt.golden, f, "必填集里的 %s 在 golden 里没有", f)
			}
			for k := range tt.golden {
				assert.True(t, inReq[k], "golden 里的 %s 不在必填集里", k)
			}
		})
	}
}

// 返回的切片必须是副本。调用方拿到 fieldOrder 本身就能改动全局字段顺序，
// 而 DDL、INSERT 列、白名单全都从它派生——一次误写会同时污染三处。
func TestRequiredFieldsReturnsCopy(t *testing.T) {
	req := requiredFields(extractorV2)
	require.NotEmpty(t, req)
	orig := fieldOrder[0]
	req[0] = "tampered"
	assert.Equal(t, orig, fieldOrder[0], "requiredFields 泄漏了 fieldOrder 的底层数组")
}

// llm-fallback@v1 拿不到必填集：Extractor 同时编码了「抽取方式」和「模板版本」，
// 而这个值只带前者——无从知道它抽的是 v1 还是 v2 期次。
//
// 返回 nil 是**刻意的失败信号**。M1c 启用 LLM 兜底的第一天 completeness 就会
// 全部 skipped，逼实现者面对这个模型缺陷，而不是让它悄悄拿 54 个字段去校验
// 一个只有 27 个字段的 v1 期次。
func TestRequiredFieldsRejectsAmbiguousExtractor(t *testing.T) {
	assert.Nil(t, requiredFields("llm-fallback@v1"))
	assert.Nil(t, requiredFields("rule@v3"))
	assert.Nil(t, requiredFields(""))
}
