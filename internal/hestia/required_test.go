package hestia

import (
	"maps"
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
//
// M1c-3a 的 TASK-003 扩到六个 extractor：只查 rule@v2 一个，新分支若返回
// fieldOrder 的切片表达式是看不出来的。改写整条返回值再整体比对 fieldOrder，
// 而不是只看第 0 位——泄漏也可能发生在中段。
func TestRequiredFieldsReturnsCopy(t *testing.T) {
	before := slices.Clone(fieldOrder)
	// 真泄漏时把 fieldOrder 还原，免得一处泄漏连累后面每一个测试，
	// 让「哪条断言红」变成「全都红」。
	t.Cleanup(func() { copy(fieldOrder, before) })

	for _, ex := range []string{
		extractorV1, extractorV2,
		extractorMonthlyV1, extractorMonthlyV2, extractorTSFStock, extractorTSFFlow,
	} {
		t.Run(ex, func(t *testing.T) {
			req := requiredFields(ex)
			require.NotEmpty(t, req)
			for i := range req {
				req[i] = "tampered"
			}
			assert.Equal(t, before, fieldOrder,
				"requiredFields(%s) 泄漏了 fieldOrder 的底层数组", ex)
		})
	}
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

// ── M1c-3a 的 TASK-003：月报与社融独立报告的四个 extractor（AD-1）──────────
//
// Context Checkpoint: done_criteria → test mapping
// functional[0]     "validExtractors 增 4 个值"                  → TestValidExtractorsAcceptsMonthlyAndTSFStandalone
// functional[1]     "requiredFields 增 4 分支，25/52/18/9"       → TestRequiredFieldsMonthlyDropsOnlyFXSection、TestTSFStandaloneFieldsPartitionTSFSection
// functional[2]     "返回副本，不得交出底层数组"                  → TestRequiredFieldsReturnsCopy（扩到六个 extractor）
// boundary[0]       "月报差集恰好 fx_reserve/fx_rate，反向差集空" → TestRequiredFieldsMonthlyDropsOnlyFXSection
// boundary[1]       "stock ∪ flow == tsfSectionFields()，交集空"  → TestTSFStandaloneFieldsPartitionTSFSection
// boundary[2]       "llm-fallback@v1 既有行为一字不变"            → TestRequiredFieldsRejectsAmbiguousExtractor（未改动）
// error_handling[0] "错误信息逐字含全部合法值"                    → TestExtractorEnumErrorListsEveryValidValue
// non_functional[0] "从模板表派生，不手抄清单"                    → TestTSFStandaloneFieldsDeriveFromTemplateTables、TestFXSectionFieldsMatchExtractor

func fieldSet(fs []string) map[string]bool {
	m := make(map[string]bool, len(fs))
	for _, f := range fs {
		m[f] = true
	}
	return m
}

// 四个新值必须被 Meta 的取值域放行。
func TestValidExtractorsAcceptsMonthlyAndTSFStandalone(t *testing.T) {
	for _, ex := range []string{
		extractorMonthlyV1, extractorMonthlyV2, extractorTSFStock, extractorTSFFlow,
	} {
		m := validMeta()
		m.Extractor = ex
		require.NoErrorf(t, m.validate(), "extractor=%s 应合法", ex)
	}

	// 常量的**值**也要钉住：上面那个循环只证明「白名单认得这四个常量」，
	// 常量本身写错一个字符它照样全绿——而这些字串会进数据库、进日志、
	// 被 detectExtractor（TASK-004）逐字返回。
	assert.Equal(t, "rule-monthly@v1", extractorMonthlyV1)
	assert.Equal(t, "rule-monthly@v2", extractorMonthlyV2)
	assert.Equal(t, "tsf-stock@v1", extractorTSFStock)
	assert.Equal(t, "tsf-flow@v1", extractorTSFFlow)
}

// 月报必填集 = 对应季报必填集**精确减去**外汇板块那两个字段。
//
// 断「差集恰好是这两个」而不是「数量少 2」：少 2 个的实现有很多种，其中绝大多数
// 是错的——丢掉两个利率字段同样满足 25 / 52 这个数量。
func TestRequiredFieldsMonthlyDropsOnlyFXSection(t *testing.T) {
	tests := []struct {
		quarterly, monthly string
		want               int
	}{
		{extractorV1, extractorMonthlyV1, 25},
		{extractorV2, extractorMonthlyV2, 52},
	}
	for _, tt := range tests {
		t.Run(tt.monthly, func(t *testing.T) {
			q, m := requiredFields(tt.quarterly), requiredFields(tt.monthly)
			require.Len(t, m, tt.want)

			inM := fieldSet(m)
			dropped := make([]string, 0, 2)
			for _, f := range q {
				if !inM[f] {
					dropped = append(dropped, f)
				}
			}
			assert.ElementsMatch(t, []string{FieldFXReserve, FieldFXRate}, dropped,
				"%s 相对 %s 少掉的不是且只是外汇板块那两个字段", tt.monthly, tt.quarterly)

			// 反向差集为空：月报必填集不得含季报没有的字段。
			inQ := fieldSet(q)
			for _, f := range m {
				assert.Truef(t, inQ[f], "月报必填集里的 %s 在 %s 里没有", f, tt.quarterly)
			}
		})
	}
}

// 社融存量 / 增量两个独立报告的必填集，是社融两节 27 个字段的一个**划分**。
func TestTSFStandaloneFieldsPartitionTSFSection(t *testing.T) {
	stock, flow := requiredFields(extractorTSFStock), requiredFields(extractorTSFFlow)
	require.Len(t, stock, 18)
	require.Len(t, flow, 9)

	// 并集双向包含 tsfSectionFields()。只断 18 + 9 == 27 不够：两个错误的划分
	// 同样满足那个等式。
	inSection := fieldSet(tsfSectionFields())
	union := make(map[string]bool, len(stock)+len(flow))
	for _, f := range slices.Concat(stock, flow) {
		assert.Truef(t, inSection[f], "%s 不在社融两节的 27 个字段里", f)
		union[f] = true
	}
	for f := range inSection {
		assert.Truef(t, union[f], "社融两节的 %s 存量增量两边都没有", f)
	}

	// 交集为空。
	inStock := fieldSet(stock)
	for _, f := range flow {
		assert.Falsef(t, inStock[f], "%s 同时出现在存量与增量必填集里", f)
	}

	// 🔴 上面三条**都抓不到三个总量字段被划反**：把 tsf_flow_ytd 记进存量、
	// tsf_stock_yoy 记进增量，数量仍是 18/9、并集不变、交集仍空，全绿。
	// 故逐个钉死它们的归属。
	assert.Contains(t, stock, FieldTSFStock, "社融存量总量应属于存量报告")
	assert.Contains(t, stock, FieldTSFStockYoY, "社融存量同比应属于存量报告")
	assert.Contains(t, flow, FieldTSFFlowYTD, "社融增量累计应属于增量报告")
}

// 两个划分必须真的**走模板表**，而不是把 27 个字段手抄成两份清单。
//
// 手法同 TestTSFSectionFieldsDerivesFromTemplateTables：往模板表临时插一个前缀
// 不是 tsf_、也不在 fieldOrder 里的哨兵分项——真派生会把它带出来，手抄版不会，
// 且哨兵必须落在**正确的那一边**。
func TestTSFStandaloneFieldsDeriveFromTemplateTables(t *testing.T) {
	origStock, origFlow := tsfStockItems, tsfFlowItems
	t.Cleanup(func() { tsfStockItems, tsfFlowItems = origStock, origFlow })

	tsfStockItems = append(slices.Clone(origStock),
		tsfStockItem{"哨兵分项", "sentinel_balance", "sentinel_balance_yoy"})
	tsfFlowItems = append(slices.Clone(origFlow),
		nameField{"哨兵分项", "sentinel_flow_ytd"})

	stock, flow := requiredFields(extractorTSFStock), requiredFields(extractorTSFFlow)
	assert.Len(t, stock, 20, "往存量模板表加了 2 个字段，存量必填集却没跟着长")
	assert.Len(t, flow, 10, "往增量模板表加了 1 个字段，增量必填集却没跟着长")
	assert.Contains(t, stock, "sentinel_balance")
	assert.Contains(t, stock, "sentinel_balance_yoy")
	assert.Contains(t, flow, "sentinel_flow_ytd")
	assert.NotContains(t, stock, "sentinel_flow_ytd", "增量模板表的字段跑进了存量必填集")
	assert.NotContains(t, flow, "sentinel_balance", "存量模板表的字段跑进了增量必填集")
}

// fxSectionFields() 是本包第二处**手写**的字段归属（第一处是 tsfSectionFields 的
// 三个总量），理由完全相同：singletonFields() 把所有单字段规则混在一个扁平列表里
// （存贷款余额、期内合计、利率、外汇、社融总量），结构上无法按板块区分。
//
// 手写就必须有人钉住。这里拿外汇板块抽取器的**真实产出**做对照——它才是「外汇节
// 产出哪些字段」的事实。两个真相源分叉时（比如往 extractFXSection 加了第三个字段
// 却没加进这里），月报必填集会静默多减/少减一个字段，而 completeness 不会喊。
func TestFXSectionFieldsMatchExtractor(t *testing.T) {
	got, err := extractFXSection(section{
		Body: "国家外汇储备余额为3.34万亿美元。人民币汇率为1美元兑7.1128元人民币。",
	})
	require.NoError(t, err,
		"构造的外汇板块正文没被模板认出来——先看 profiles.go 的 fxReserveRE / fxRateRE 是否改了")

	assert.ElementsMatch(t, fxSectionFields(), slices.Collect(maps.Keys(got)),
		"fxSectionFields() 与 extractFXSection 的实际产出分叉了")
}

// 非法 extractor 的错误信息必须逐字列出**全部**合法值。
//
// 错误信息由白名单本身拼出（checkEnum），不是另抄一份——本测试防的正是有人把它
// 抄成字面量：白名单放行了新值、错误信息却还在说旧的三个，而那条信息正是调用方
// 判断自己该填什么的唯一依据。
//
// 这里逐字写死七个值而不是遍历 validExtractors：遍历版本对「常量值写错」免疫，
// 错的常量会被同样错的错误信息「印证」。
func TestExtractorEnumErrorListsEveryValidValue(t *testing.T) {
	m := validMeta()
	m.Extractor = "rule@v9"
	err := m.validate()
	require.Error(t, err)

	for _, want := range []string{
		"rule@v1", "rule@v2", "llm-fallback@v1",
		"rule-monthly@v1", "rule-monthly@v2", "tsf-stock@v1", "tsf-flow@v1",
	} {
		assert.Containsf(t, err.Error(), want, "错误信息没提到合法值 %s", want)
	}
	assert.Len(t, validExtractors, 7, "白名单增删了取值——请同步本测试的逐字清单")
}
