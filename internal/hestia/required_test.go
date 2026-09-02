package hestia

import (
	"maps"
	"slices"
	"strings"
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

// 社融两节共 36 个字段：8 个存量分项 × 2（余额 + 同比）+ 8 个增量分项 × 2（ytd + mom）
// + 4 个总量（tsf_stock / tsf_stock_yoy / tsf_flow_ytd / tsf_flow_mom）。
//
// ⚠️ **27 → 36 是 M1c-4 的 TASK-006 的改动**：TASK-005 之后社融增量节按口径路由，
// 同一节可能产出 *_ytd 也可能产出 *_mom ⇒ 本函数（回答「这两节会产出哪些列」）必须两侧都列。
// 少列 _mom 那 9 个，`without(fieldOrder, 这里)` 会把它们留在 v1（根本没有社融节的报告）
// 的必填集里 —— 实测那样 requiredFields(rule@v1) 会对标准 v1 样本报缺 9 个 tsf_flow_*_mom。
// 社融**存量**没有当月口径，故存量那半不变。
func TestTSFSectionFieldsIsExactAndDistinct(t *testing.T) {
	got := tsfSectionFields()
	assert.Len(t, got, 36)

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
		nameField{"哨兵分项", "sentinel_flow_ytd", "sentinel_flow_mom"})

	got := tsfSectionFields()
	// +4 而不是 +3（M1c-4 的 TASK-006）：增量哨兵是一个 nameField，它现在带出
	// **两列**（ytd + mom），存量哨兵仍是 2 列 ⇒ 2 + 2 = 4。
	assert.Len(t, got, base+4, "往模板表加了 4 个字段，派生结果却没跟着长——没在遍历模板表")
	for _, f := range []string{"sentinel_balance", "sentinel_balance_yoy", "sentinel_flow_ytd", "sentinel_flow_mom"} {
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
		// M1c-4 的 TASK-006：*_mom 列进必填集（momFields 的过渡性 without 已退场），
		// 口径感知由 gateCompleteness 的 missingCaliberAware 承担 ⇒ 54→76、27→40。
		{extractorV2, golden2025, 76},
		{extractorV1, golden2020, 40},
	}
	for _, tt := range tests {
		t.Run(tt.extractor, func(t *testing.T) {
			req := requiredFields(tt.extractor)
			require.Len(t, req, tt.want)

			inReq := make(map[string]bool, len(req))
			for _, f := range req {
				inReq[f] = true
			}
			// 🔴 **判据换形式不换内容**（M1c-4 的 TASK-006）：原来断言「必填集 ≡ golden 键集」，
			// 理由是「必填集不能要求一份完整观测拿不出的字段」。*_mom 列进必填集之后
			// 那个等式**结构上不成立**——golden 是一篇累计口径的报告，它只产 *_ytd，
			// 而必填集两侧都列。⇒ 同一个理由现在的形式是：
			//
			//	口径感知之下 golden 一个字段都不缺（每个必填项要么自己在场、要么孪生在场）
			//
			// ⚠️ 这比原断言**更严**：它同时要求 caliberFamilies 把该配的都配上了——
			// 漏配一对，那一对的 _mom 就会出现在 missing 里。
			assert.Emptyf(t, missingCaliberAware(req, tt.golden),
				"口径感知之下 %s 的 golden 不该缺任何必填字段", tt.extractor)

			// 反向仍是严格包含：golden 里的每个键都必须在必填集里（多抽出一个字段
			// 而必填集不知道它，同样是分叉）。
			for k := range tt.golden {
				assert.True(t, inReq[k], "golden 里的 %s 不在必填集里", k)
			}

			// 🔴 交叉：把 golden 的 *_ytd 全部换成 *_mom，口径感知同样不该报缺。
			// 少了这一条，一个只建了 ytd→mom 单向 twin 的实现能让上面那条全绿。
			flipped := make(map[string]float64, len(tt.golden))
			for k, v := range tt.golden {
				if stem, ok := strings.CutSuffix(k, "_ytd"); ok {
					if slices.Contains(fieldOrder, stem+"_mom") {
						flipped[stem+"_mom"] = v
						continue
					}
				}
				flipped[k] = v
			}
			assert.Emptyf(t, missingCaliberAware(req, flipped),
				"把 golden 的 _ytd 全换成 _mom 之后，%s 同样不该缺任何必填字段", tt.extractor)
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
		// M1c-4 的 TASK-006：*_mom 进必填集 ⇒ 25→38、52→74（各 +13：
		// 存款 5 + 贷款 8；社融那 9 个 _mom 已随 tsfSectionFields 一并划归社融节）。
		{extractorV1, extractorMonthlyV1, 38},
		{extractorV2, extractorMonthlyV2, 74},
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

// 社融存量 / 增量两个独立报告的必填集，是社融两节 36 个字段的一个**划分**。
func TestTSFStandaloneFieldsPartitionTSFSection(t *testing.T) {
	stock, flow := requiredFields(extractorTSFStock), requiredFields(extractorTSFFlow)
	require.Len(t, stock, 18)
	// 9 → 18（M1c-4 的 TASK-006）：增量侧两种口径都列，理由同 tsfSectionFields。
	require.Len(t, flow, 18)

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
		nameField{"哨兵分项", "sentinel_flow_ytd", "sentinel_flow_mom"})

	stock, flow := requiredFields(extractorTSFStock), requiredFields(extractorTSFFlow)
	assert.Len(t, stock, 20, "往存量模板表加了 2 个字段，存量必填集却没跟着长")
	assert.Len(t, flow, 20, "往增量模板表加了 2 个字段（ytd+mom），增量必填集却没跟着长")
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
// 这里逐字写死八个值而不是遍历 validExtractors：遍历版本对「常量值写错」免疫，
// 错的常量会被同样错的错误信息「印证」。
//
// ⚠️ 增删取值时**照着改这份清单**，不要图省事改成 range validExtractors —— 那正好
// 把本测试唯一挡得住的那类缺陷放过去。
func TestExtractorEnumErrorListsEveryValidValue(t *testing.T) {
	m := validMeta()
	m.Extractor = "rule@v9"
	err := m.validate()
	require.Error(t, err)

	for _, want := range []string{
		"rule@v1", "rule@v2", "llm-fallback@v1",
		"rule-monthly@v1", "rule-monthly@v2", "tsf-stock@v1", "tsf-flow@v1",
		"merged@v1", // M1c-3b 的 TASK-002
	} {
		assert.Containsf(t, err.Error(), want, "错误信息没提到合法值 %s", want)
	}
	assert.Len(t, validExtractors, 8, "白名单增删了取值——请同步本测试的逐字清单")
}

// ── M1c-3a 的 TASK-012（QA fix 6）：validExtractors 的逐项表态 ──────────────
//
// Context Checkpoint: done_criteria → test mapping (M1c-3a 的 TASK-012)
// functional[3]  遍历 validExtractors，新增第八个而不登记 requiredFields 必须红
//                                          → TestEveryValidExtractorHasARequiredFieldsDecision

// TestEveryValidExtractorHasARequiredFieldsDecision 要求 validExtractors 里的**每一个**值
// 都对 requiredFields 有明确归属：要么返回非空必填集，要么登记在下面的豁免表里并写明理由。
//
// 遍历取值域本身而不是手写七个值：将来加第八个 extractor 时它会**自动**进入本测试并被
// 要求表态——不登记就红。这与本包既有的两处范式同一用意（sections_test.go 的
// TestMonthlyIsTheOnlyPeriodTypeExemptFromFX 遍历 validPeriodTypes、types.go 的
// TestEveryPeriodTypeHasAnExplicitSupportDecision）。
//
// 🔴 **少了它的失效方式是静默的**：新 extractor 落进 requiredFields 的 default 分支返回 nil，
// completeness 记 skipped 而不是 failed —— 那一期看起来「本就没有这些字段」，没有任何闸门会响。
// 这正是 QA fix 6 指出的缺口。
func TestEveryValidExtractorHasARequiredFieldsDecision(t *testing.T) {
	// 豁免表：键是 extractor，值是**为什么**它拿不到必填集。
	// 写理由而不是只写 true —— 「想过并决定豁免」与「忘了登记」在代码里长得一样。
	exempt := map[string]string{
		"llm-fallback@v1": "Extractor 同时编码「抽取方式」与「模板版本」，这个值只带前者，" +
			"无从知道它抽的是 v1 还是 v2 期次。返回 nil 是刻意的失败信号，见 " +
			"TestRequiredFieldsRejectsAmbiguousExtractor。",

		// M1c-3b 的 TASK-002。与上面那条的成因**不同**，别把两条读成同一类：
		// llm-fallback@v1 是「还没实现」——M1c-4 补上分支后它就该从本表里删掉；
		// merged@v1 是「这一列结构上说不出必填集」——它**永远**不会有分支。
		"merged@v1": "这一列结构上说不出必填集：必填集取决于由哪几篇报告合成，而 Extractor " +
			"只有一个字符串（实测 42 组里并非每组都齐 3 篇）。真正的必填集走 " +
			"mergedRequiredFields(parts)，见 TestRequiredFieldsRejectsBareMerged 与 " +
			"TestMergedRequiredFieldsIsUnionOfParts。与 llm-fallback@v1 的「还没实现」成因不同：" +
			"那条将来会被删掉，这条不会。",
	}

	require.NotEmpty(t, validExtractors, "取值域为空，本测试的绿色是假的")
	for _, ex := range validExtractors {
		t.Run(ex, func(t *testing.T) {
			got := requiredFields(ex)
			if reason, ok := exempt[ex]; ok {
				assert.NotEmpty(t, reason, "豁免必须写明理由")
				assert.Nilf(t, got, "%s 登记为豁免，requiredFields 就该返回 nil；"+
					"若它现在有必填集了，请把它从豁免表里删掉", ex)
				return
			}
			assert.NotEmptyf(t, got, "%s 在 validExtractors 里却拿不到必填集："+
				"它会落进 requiredFields 的 default 分支返回 nil，completeness 记 skipped 而非 "+
				"failed —— 那一期看起来「本就没有这些字段」，没有任何闸门会响。"+
				"新增 extractor 时必须在 requiredFields 里加分支，或登记进本测试的豁免表并写明理由", ex)
		})
	}

	// 反向：豁免表里不得有 validExtractors 之外的键——那种键永远不会被求值，
	// 是一条看起来在豁免、实际零效力的记录。
	for ex := range exempt {
		assert.Containsf(t, validExtractors, ex,
			"豁免表里的 %q 不在 validExtractors 里：这条豁免永远不会被求值", ex)
	}
}

// ── M1c-3b 的 TASK-002：merged@v1 的取值域与必填集 ──────────────────────────
//
// Context Checkpoint: done_criteria → test mapping (M1c-3b 的 TASK-002)
// functional[2]  三篇齐全 ⇒ without(fieldOrder, fxSectionFields())；只有社融两篇 ⇒ 只是这两篇的并集
//                                                    → TestMergedRequiredFieldsIsUnionOfParts
// functional[3]  requiredFields(extractorMerged) 返回 nil → TestRequiredFieldsRejectsBareMerged
// boundary[1]    同一 extractor 传两次不得重复；按 fieldOrder 升序
//                                                    → TestMergedRequiredFieldsIsOrderedAndDeduped

// TestMergedRequiredFieldsIsUnionOfParts：合并观测的必填集 = 各部分必填集的并集。
func TestMergedRequiredFieldsIsUnionOfParts(t *testing.T) {
	// 三篇齐全 ⇒ fieldOrder 减去外汇两字段，即 54 − 2 = 52 个。
	//
	// ⚠️ 这 52 = rule-monthly@v1(25) + tsf-stock@v1(18) + tsf-flow@v1(9)，**不是** 54。
	// 需求文档 Task 3 注释里的「27/18/9=54」用的是 rule@v1（季报，27 个，**含**
	// fx 两字段），与这里的 rule-monthly@v1（月报无外汇板块，25 个）不是同一组
	// extractor —— 两个数都对，别混起来。
	three := mergedRequiredFields([]string{
		extractorMonthlyV1, extractorTSFStock, extractorTSFFlow})
	// M1c-4 的 TASK-006：TASK-004 加的那层 `without(..., momFields())` 已随
	// momFields 一起退场 —— *_mom 列现在正常进必填集，口径感知由 gateCompleteness
	// 的 missingCaliberAware 承担。
	want := without(fieldOrder, fxSectionFields())
	require.ElementsMatch(t, want, three,
		"月报v1 + 社融存量 + 社融增量 应当覆盖除外汇两字段外的全部 52 个字段")

	// 写成 without(fieldOrder, ...) 这个**派生**表达式而不是字面量 52：两份清单
	// 会分叉，而先错的一定是手抄的那份（与 requiredFields 里「季报 − 外汇节」同一
	// 理由）。数量单独钉一条，防的是派生表达式两边一起错。
	require.Len(t, three, 74,
		"38(月报v1) + 18(社融存量) + 18(社融增量) = 74 = 76 − 2；对不上说明三者不再互斥或字段表变了。"+
			"⚠️ M1c-4 的 TASK-006 起三个数都变了：*_mom 进必填集、社融增量两侧都列")

	// 只有两篇（实测 2020-01|monthly 就是这样）⇒ 必填集只能是这两篇的并集。
	two := mergedRequiredFields([]string{extractorTSFStock, extractorTSFFlow})
	require.ElementsMatch(t, append(tsfStockFields(), tsfFlowFields()...), two,
		"缺月报那篇时，25 个非社融字段是 absent-by-design，不得进必填集")
	require.NotContains(t, two, FieldM2,
		"硬套 rule-monthly@v2 的 52 字段会让这类组恒报缺字段，completeness 随之失效")
}

// TestMergedRequiredFieldsIsOrderedAndDeduped：并集必须按 fieldOrder 排序且无重复。
//
// 排序的理由与 gateMagnitudeSanity 遍历 fieldOrder 相同：map 迭代顺序随机，
// 同一份数据两次跑报出不同的缺失字段，会让排查变成猜谜。
func TestMergedRequiredFieldsIsOrderedAndDeduped(t *testing.T) {
	idx := map[string]int{}
	for i, f := range fieldOrder {
		idx[f] = i
	}
	requireFieldOrderAscending := func(t *testing.T, got []string) {
		t.Helper()
		for i := 1; i < len(got); i++ {
			require.Lessf(t, idx[got[i-1]], idx[got[i]],
				"必须按 fieldOrder 升序：%s 排在了 %s 之前", got[i-1], got[i])
		}
	}

	// 故意传入有重叠的两份（同一个 extractor 传两次）。
	//
	// ⚠️ 这里断的是 ElementsMatch 而不是 Equal：ElementsMatch 比的是**多重集**，
	// 重复元素照样会让它红，所以「不得出现两次」被完整钉住；而 Equal 会连**顺序**
	// 一起钉死到 tsfStockFields() 上，那是错的 —— tsfStockFields() 把两个总量字段
	// append 在**末尾**，fieldOrder 里它们却排在**最前**。需求文档 Task 2 Step 1 给的
	// require.Equal(tsfStockFields(), got) 与它自己 Step 3 的「按 fieldOrder 排序」
	// 相互矛盾，任一实现都不可能同时满足；本任务 done_criteria 的 boundary[1] 说的是
	// 「不得重复 + 按 fieldOrder 升序」两条**性质**，故按性质断言。
	dup := mergedRequiredFields([]string{extractorTSFStock, extractorTSFStock})
	require.ElementsMatch(t, tsfStockFields(), dup,
		"重复的 extractor 不得让字段出现两次（ElementsMatch 比多重集，重复会红）")
	requireFieldOrderAscending(t, dup)

	all := mergedRequiredFields([]string{extractorMonthlyV1, extractorTSFStock})
	requireFieldOrderAscending(t, all)

	// 阴性：升序断言本身得能红。把并集倒过来喂给同一个判据必须失败，否则上面两条
	// 绿色证明不了任何事（长度 <= 1 时循环体一次都不进，那种「绿」是假的）。
	require.Greater(t, len(all), 1, "样本太短，升序断言的循环体不会执行")
	reversed := slices.Clone(all)
	slices.Reverse(reversed)
	require.False(t, slices.IsSortedFunc(reversed, func(a, b string) int {
		return idx[a] - idx[b]
	}), "倒序的并集竟然仍算升序——说明 idx 建错了，上面的升序断言是空转")
}

// TestRequiredFieldsRejectsBareMerged：merged@v1 单独问必填集是问不出来的。
//
// 它的必填集取决于**由哪几篇合成**，而 extractor 这一列只有一个字符串。
// 返回 nil ⇒ 调用方记 skipped 而不是 failed，与 llm-fallback@v1 同一条出路，
// 但成因不同：那个是「还没实现」，这个是「这一列结构上说不出必填集」。
func TestRequiredFieldsRejectsBareMerged(t *testing.T) {
	require.Nil(t, requiredFields(extractorMerged),
		"merged@v1 的必填集只能由 mergedRequiredFields(parts) 给出")
}

// —— M1c-4 的 TASK-006：completeness 的必填集改口径感知 ——
//
// Context Checkpoint: done_criteria → test mapping（M1c-4 的 TASK-006）
// functional[0]  caliberFamilies 从 fieldOrder 派生     → TestCaliberFamiliesPairsEveryMomWithItsYTD
// functional[1]  两个方向都不算缺                        → TestMissingCaliberAwareAcceptsEitherSide
// functional[1]  两侧都空才算缺，按 fieldOrder 排序      → TestMissingCaliberAwareStillReportsRealGaps
// functional[1]  🔴 放松的**边界**：不成对的字段原样要求 → TestMissingCaliberAwareLeavesUnpairedFieldsAlone
// boundary[0]    🔴 mom→ytd 方向在**真实路径**上真的会走到
//                                                        → TestMissingCaliberAwareMomToYTDIsReachableOnRealWant
// functional[2]  gateCompleteness 换成一次调用           → validate_test.go 的既有 completeness 测试

// caliberFamilies 必须**从 fieldOrder 派生**，不手写第二份名单。
//
// 判据是「22 对」而不是「非空」：一个只返回一对的实现能让「非空」全绿，而
// 口径感知会对其余 21 对完全失效——那 21 族在真语料里恰恰是最常缺的。
func TestCaliberFamiliesPairsEveryMomWithItsYTD(t *testing.T) {
	fams := caliberFamilies()
	require.Len(t, fams, 22, "TASK-004 加了 22 个 _mom 列，每个都该有 _ytd 孪生")

	inOrder := make(map[string]bool, len(fieldOrder))
	for _, f := range fieldOrder {
		inOrder[f] = true
	}
	seenMoM := map[string]bool{}
	for _, p := range fams {
		ytd, mom := p[0], p[1]
		assert.Truef(t, strings.HasSuffix(ytd, "_ytd"), "第一项必须是 _ytd 列，得到 %q", ytd)
		assert.Truef(t, strings.HasSuffix(mom, "_mom"), "第二项必须是 _mom 列，得到 %q", mom)
		assert.Equalf(t, strings.TrimSuffix(mom, "_mom"), strings.TrimSuffix(ytd, "_ytd"),
			"配对必须由**列名**派生：%q 与 %q 的词干不同", ytd, mom)
		assert.Truef(t, inOrder[ytd] && inOrder[mom], "%q/%q 必须都在 fieldOrder 里", ytd, mom)
		assert.Falsef(t, seenMoM[mom], "%q 出现了两次", mom)
		seenMoM[mom] = true
	}

	// 反向闭合：fieldOrder 里每个 _mom 列都必须在配对里出现一次，一个都不许漏。
	// 只断「22 对」挡不住「配了 22 对但漏掉某一列、多算了另一列」。
	for _, f := range fieldOrder {
		if strings.HasSuffix(f, "_mom") {
			assert.Truef(t, seenMoM[f], "fieldOrder 里的 %q 没有出现在 caliberFamilies 里", f)
		}
	}
}

// 两个方向都不算缺：want 要 _ytd 而 values 只有 _mom，以及反过来。
//
// ⚠️ **两个方向都要测**：只测一个方向的话，一个 twin map 只填了单向的实现照样全绿，
// 而另一半在真实路径上会把整族报成缺失。
func TestMissingCaliberAwareAcceptsEitherSide(t *testing.T) {
	t.Run("want要ytd_values只有mom", func(t *testing.T) {
		got := missingCaliberAware(
			[]string{FieldDepositHouseholdYTD, FieldLoanHHShortYTD},
			map[string]float64{FieldDepositHouseholdMoM: 1, FieldLoanHHShortMoM: 2})
		assert.Empty(t, got, "同族另一侧在场 ⇒ 不算缺（那 54 篇是 absent-by-design）")
	})
	t.Run("want要mom_values只有ytd", func(t *testing.T) {
		got := missingCaliberAware(
			[]string{FieldDepositHouseholdMoM, FieldLoanHHShortMoM},
			map[string]float64{FieldDepositHouseholdYTD: 1, FieldLoanHHShortYTD: 2})
		assert.Empty(t, got, "🔴 mom→ytd 方向——单向 twin map 会让这一格红")
	})
}

// 两侧都空才算真缺，且结果**按 fieldOrder 排序**。
//
// 🔴 期望数组的顺序是 {FieldM2, FieldDepositHouseholdYTD} —— **需求文档写反了**。
// 实测 fieldOrder 里 FieldM2 在 FieldDepositHouseholdYTD **之前**（A.2 货币供应量在
// A.3 存款之前）。⚠️ **照文档抄会红，而红的位置指向排序逻辑**——照着那个位置去「修排序」
// 会把一个正确的实现改坏。遇到这条红先核对 fieldOrder 的实际顺序再动手。
func TestMissingCaliberAwareStillReportsRealGaps(t *testing.T) {
	// 前提：把顺序断言的依据摆出来，而不是让读者相信我
	iM2, iDep := slices.Index(fieldOrder, FieldM2), slices.Index(fieldOrder, FieldDepositHouseholdYTD)
	require.NotEqual(t, -1, iM2)
	require.NotEqual(t, -1, iDep)
	require.Lessf(t, iM2, iDep, "用例前提：fieldOrder 里 %s 必须在 %s 之前", FieldM2, FieldDepositHouseholdYTD)

	got := missingCaliberAware(
		// 刻意逆序传入：不排序的实现会原样吐出来
		[]string{FieldDepositHouseholdYTD, FieldM2, FieldLoanHHShortYTD},
		map[string]float64{FieldLoanHHShortMoM: 1})

	assert.Equal(t, []string{FieldM2, FieldDepositHouseholdYTD}, got,
		"两侧都空的才算缺，且必须按 fieldOrder 排序；loan_hh_short 有 _mom 在场故不算缺")
}

// 🔴 放松的**边界**，本任务最不能省的一条。
//
// 存量、余额、同比、利率、外汇都没有当月口径的对应物 ⇒ 必须原样逐个要求。
// 少了这一条，一个「凡缺失就去掉 _ytd 后缀找孪生」的实现会顺手放松掉一大批与本迭代
// 无关的字段，而正向测试全绿——那正是「口径感知」失控的形态。
func TestMissingCaliberAwareLeavesUnpairedFieldsAlone(t *testing.T) {
	unpaired := []string{
		FieldTSFStock, FieldTSFStockYoY, // 存量：没有当月口径
		FieldDepositBalance, FieldLoanBalanceYoY, // 余额与同比
		FieldRateIBO, FieldM2, FieldM2YoY, // 利率、货币供应量
		FieldFXReserve, FieldFXRate, // 外汇
	}
	for _, f := range unpaired {
		require.Falsef(t, strings.HasSuffix(f, "_ytd") || strings.HasSuffix(f, "_mom"),
			"用例前提：%q 不该是成对列", f)
	}

	got := missingCaliberAware(unpaired, map[string]float64{})
	// 集合相等：一个都不许被放松。⚠️ 用 ElementsMatch 而不是 Equal —— 返回值按
	// fieldOrder 排序，而 unpaired 是按「哪一类」列的，两者顺序本来就不同。
	assert.ElementsMatch(t, unpaired, got,
		"不成对的字段一个都不许被放松——它们本来就没有另一侧可以顶替")
	// 顺序单独钉：结果必须按 fieldOrder 升序
	idx := make([]int, len(got))
	for i, f := range got {
		idx[i] = slices.Index(fieldOrder, f)
	}
	assert.True(t, slices.IsSorted(idx), "结果必须按 fieldOrder 排序，实得 %v", got)

	// 交叉：给一个「同名但后缀不同」的干扰值，仍不得顶替
	got = missingCaliberAware([]string{FieldTSFStock},
		map[string]float64{FieldTSFStockYoY: 1, FieldTSFFlowMoM: 2})
	assert.Equal(t, []string{FieldTSFStock}, got,
		"别的字段在场不构成顶替——顶替关系只在同族两列之间")
}

// 🔴 **mom→ytd 方向在真实路径上真的会走到**（DoD boundary[0] 第 3 点）。
//
// 上面 TestMissingCaliberAwareAcceptsEitherSide 用的是**手工构造**的 want，
// 那条即使在真实 want 恒为单侧（例如 momFields() 的 without 还留着）时也全绿。
// ⇒ 必须用 `requiredFields(...)` 的**真实返回值**再钉一次：
// 它若不含任何 _mom 列，mom→ytd 那半边逻辑就是死代码，而没有任何测试会告诉你。
func TestMissingCaliberAwareMomToYTDIsReachableOnRealWant(t *testing.T) {
	for _, ex := range []string{extractorV2, extractorMonthlyV2} {
		t.Run(ex, func(t *testing.T) {
			req := requiredFields(ex)
			require.NotEmpty(t, req)

			var moms []string
			for _, f := range req {
				if strings.HasSuffix(f, "_mom") {
					moms = append(moms, f)
				}
			}
			require.NotEmptyf(t, moms,
				"🔴 requiredFields(%s) 一个 _mom 列都没有 ⇒ missingCaliberAware 的 "+
					"mom→ytd 分支在真实路径上永不走到，那半边逻辑的死活无人知道。"+
					"（若 momFields() 的 without 还留着，就会是这个结果）", ex)

			// 真实 want + 只有 ytd 的 values ⇒ 那些 _mom 列不得算缺
			values := map[string]float64{}
			for _, m := range moms {
				values[strings.TrimSuffix(m, "_mom")+"_ytd"] = 1
			}
			got := missingCaliberAware(moms, values)
			assert.Emptyf(t, got, "真实 want 里的 _mom 列有 _ytd 顶替时不得算缺，实得缺 %v", got)
		})
	}
}
