package hestia

// Context Checkpoint: done_criteria → test mapping (types)
// functional[0]      Meta 七字段声明顺序是跨任务契约      → TestMetaFieldOrderIsCrossTaskContract
// functional[1]      Observation/Check/ValidationReport/Outcome 按需求文档定义
//                                                        → TestObservationValuesRepresentAbsence、TestValidationReportAndOutcomeShape
// functional[2]      CheckStatus 及三常量完整             → TestCheckStatusConstants
// functional[1]      三种合法 period_type 均通过          → TestMetaValidateAcceptsValid
// boundary[0]        validate 忽略 IngestedAt             → TestMetaValidateIgnoresIngestedAt
// error_handling[0]  六个必填项逐个单独取空各报错，错误串指名字段
//                                                        → TestMetaValidateRejectsEmptyRequired
// error_handling[1]  period_type 非法值报错，合法集合在代码中显式列出
//                                                        → TestMetaValidateRejectsBadPeriodType
// error_handling[2]  G1：published_at 须匹配 ^\d{4}-\d{2}-\d{2}$、period 须匹配 ^\d{4}-\d{2}$
//                                                        → TestMetaValidateRejectsMalformedPublishedAt、TestMetaValidateRejectsMalformedPeriod
// non_functional[0]  validate 不导出 —— 由本文件全程调用小写 m.validate() 钉住：
//                    若有人把它改成导出的 Validate，整个包的测试立刻编译失败。

import (
	"reflect"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/newthinker/atlas/internal/macro/bitemporal"
)

func validMeta() Meta {
	return Meta{
		Period:         "2026-06",
		PeriodType:     "h1",
		PublishedAt:    "2026-07-15",
		ArticleID:      "2026071512340454869",
		CaliberVersion: "2025-01",
		Extractor:      "rule@v2",
		// IngestedAt 故意留空——它由 Store.Save 填
	}
}

// TestMetaFieldOrderIsCrossTaskContract 把「七个字段的声明顺序」钉成可执行契约。
//
// 这个顺序同时被 Task 3 的 metaColumns（schema.go）与 Task 5 的 insert 取值
// （store.go）复用。三处任一不同序，写入会静默错位——列是 TEXT，把 extractor
// 的值塞进 caliber_version 列不会触发任何数据库错误，只会让下游读到胡话。
// 靠注释约定挡不住这种事，所以这里用 reflect 钉死顺序与字段数：
// 谁增删或重排字段，本条立刻转红，迫使他同步另外两处。
func TestMetaFieldOrderIsCrossTaskContract(t *testing.T) {
	want := []string{
		"Period", "PeriodType", "PublishedAt", "ArticleID",
		"CaliberVersion", "Extractor", "IngestedAt",
	}

	typ := reflect.TypeOf(Meta{})
	got := make([]string, typ.NumField())
	for i := range got {
		got[i] = typ.Field(i).Name
	}

	assert.Equal(t, want, got,
		"Meta 字段顺序是跨任务契约，须与 schema.go 的 metaColumns、store.go 的 insert 取值同序")
}

func TestMetaValidateAcceptsValid(t *testing.T) {
	// period 必须随 period_type 一起给：H10 之后两者不再互相独立。原先固定用
	// validMeta 的 "2026-06" 遍历三种类型，而 2026-06/annual 恰是 H10 要拦的
	// 形态——用 12 去除半年报的期末月。
	for _, tc := range []struct{ periodType, period string }{
		{"monthly", "2026-06"},
		{"h1", "2026-06"},
		{"annual", "2026-12"},
		{"q1", "2026-03"},    // TASK-001：一季度
		{"q1_q3", "2026-09"}, // TASK-001：前三季度（年初起累计，不是第三季度单季）
	} {
		m := validMeta()
		m.PeriodType, m.Period = tc.periodType, tc.period
		require.NoErrorf(t, m.validate(), "period_type=%s period=%s 应合法",
			tc.periodType, tc.period)
	}
}

// TestMetaValidateRejectsEmptyRequired 逐个必填字段单独取空——一次性全空只能证明
// 「有校验」，不能证明「每个字段都被校验」。
//
// 断言比对**完整错误串**而非 Contains：`"period"` 是 `"period_type"` 的子串，
// 用 Contains 时一个把两者搞混的实现照样能通过。
func TestMetaValidateRejectsEmptyRequired(t *testing.T) {
	cases := []struct {
		field string
		blank func(*Meta)
	}{
		{"period", func(m *Meta) { m.Period = "" }},
		{"period_type", func(m *Meta) { m.PeriodType = "" }},
		{"published_at", func(m *Meta) { m.PublishedAt = "" }},
		{"article_id", func(m *Meta) { m.ArticleID = "" }},
		{"caliber_version", func(m *Meta) { m.CaliberVersion = "" }},
		{"extractor", func(m *Meta) { m.Extractor = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			m := validMeta()
			tc.blank(&m)
			assert.EqualError(t, m.validate(), "hestia: meta."+tc.field+" must not be empty")
		})
	}
}

func TestMetaValidateRejectsBadPeriodType(t *testing.T) {
	// "quarterly" / "q3" 是 TASK-001 加季度类型后**最可能被误填**的两个：前者是
	// 通用叫法，后者的字面含义（第三季度单季）与真实期次「前三季度累计」不同。
	// 白名单不认它们，必须被拒而不是当成 q1_q3 放行。
	for _, bad := range []string{"quarterly", "H1", "yearly", " h1", "h1 ", "MONTHLY", "q3", "Q1"} {
		m := validMeta()
		m.PeriodType = bad
		require.Error(t, m.validate(), "period_type=%q 应被拒", bad)
	}
}

// 错误信息里的合法取值必须由 validPeriodTypes 派生，不是抄一遍。
//
// 抄一遍的版本在加了第四、第五种取值后会**静默过期**：白名单放行了新值，错误信息
// 却还在说旧的三个 —— 而那条信息正是调用方判断自己该填什么的唯一依据。types.go 的
// checkEnum 注释专门警告过这种写法，而 Meta.validate 的 period_type 分支恰是它自己
// 没用 checkEnum 的两处之一（另一处是 thresholds.go 的 PeriodTypes 校验，由
// thresholds_test.go 的 TestExemptionRejectsBadPeriodTypes/含未知取值 守着）。
//
// 本条是那份派生的绊线：把 Join 改回硬编码串，加了取值就红。
func TestMetaValidateListsEveryValidPeriodTypeInError(t *testing.T) {
	m := validMeta()
	m.PeriodType = "quarterly"
	err := m.validate()
	require.Error(t, err)

	require.NotEmpty(t, validPeriodTypes, "前置锚点：白名单为空时下面的循环平凡通过")
	for pt := range validPeriodTypes {
		assert.Containsf(t, err.Error(), pt,
			"错误信息必须列出全部合法取值，%q 缺席说明文案已与白名单脱节", pt)
	}
}

// 🔴 两张表的一致性守卫，**单向**（TASK-001）。
//
// ⚠️ 别把它写成双向：`monthly` **刻意**不在 periodEndMonth 里（任意月份都合法），
// Meta.validate 正是靠「查不到就跳过」实现这一点。给 monthly 编一个期末月会让
// **除该月外每一期月报都被拒**。
//
// 所以两条各自成立、合起来仍能防漏改：
//   - ① periodEndMonth 的每个键都在 validPeriodTypes 里 —— 挡「给一个不存在的
//     period_type 配了期末月」，那条约束永远不会被执行，是死配置。
//   - ② 除 monthly 外，validPeriodTypes 的每个键都有期末月 —— 挡「加了新类型忘了
//     配期末月」，那才是真正危险的一侧：period 与 period_type 的荒谬配对会静默放行，
//     修订链分叉、下游双计，而两条记录各自看起来都正常。
//
// 加第六种取值时若它同样是「任意月份都合法」的形态，改的是下面这个豁免集合，
// 并在 types.go 的注释里写清理由 —— 不要改成双向。
func TestPeriodTypeMapsAreConsistent(t *testing.T) {
	// monthly 是唯一豁免。写成集合而不是 `pt != "monthly"`，是为了让「豁免有几个」
	// 在代码里一眼可数 —— 悄悄多一个豁免就是悄悄丢一条约束。
	exempt := map[string]bool{"monthly": true}

	require.NotEmpty(t, periodEndMonth, "前置锚点：表为空时 ① 平凡通过")
	for pt := range periodEndMonth {
		assert.Truef(t, validPeriodTypes[pt],
			"① periodEndMonth 有 %q 的期末月，但它不是合法 period_type：这条约束永不执行", pt)
	}

	require.NotEmpty(t, validPeriodTypes, "前置锚点：白名单为空时 ② 平凡通过")
	for pt := range validPeriodTypes {
		if exempt[pt] {
			assert.NotContainsf(t, periodEndMonth, pt,
				"%q 是豁免项，给它配期末月会让除该月外每一期都被拒", pt)
			continue
		}
		assert.Containsf(t, periodEndMonth, pt,
			"② %q 是合法 period_type 却没有期末月：period/period_type 的荒谬配对会静默放行", pt)
	}

	// 期末月的形态：Meta.validate 拿它与 Period[5:] 直接比字符串，写成 "3" 而不是
	// "03" 会让每一期该类型都被拒，且错误信息看起来完全合理（"month must be 3"）。
	for pt, mm := range periodEndMonth {
		assert.Regexpf(t, `^(0[1-9]|1[0-2])$`, mm, "%q 的期末月必须是零补齐的两位月份", pt)
	}

	// periodTypeList 必须**定序**：它的两个消费者都是错误信息，而 map 的迭代序是
	// 随机的 —— 不排序的话同一个错误每次跑出来字段顺序都不同，日志无从 diff，
	// 断言也只能退回 Contains。
	got := periodTypeList()
	assert.Len(t, got, len(validPeriodTypes), "白名单里每个取值都要出现，不多不少")
	assert.True(t, slices.IsSorted(got), "顺序必须稳定，得到 %v", got)
}

// TestMetaValidateRejectsMalformedPublishedAt 覆盖 G1 —— M1a 明文指派给写入方的契约。
//
// bitemporal.Lookup 的包注释写死：MAX() 与 Classify 都是字典序，只有 ISO 8601 形态
// 时字典序才等于时间序，喂进 "2026-7-15" 会**静默判错、零告警**，且「明确不在这里加
// 形态校验……由建表方与写入方保证」。Store 就是那个写入方，而 published_at 取自外部
// HTML 的 <meta name="PubDate">，形态不受控恰是最可能发生的事。
//
// 不校验的两种静默后果：①少补零 → 字典序失效 → 当前行视图长期返回旧版本；
// ②混入 RFC3339 → 同一报告被判成 Revision 而非 Duplicate → 多一行假修订，
// 且 refreshArticleID 永不触发。两者都不报错。
func TestMetaValidateRejectsMalformedPublishedAt(t *testing.T) {
	bad := []string{
		"2026-7-15",            // 少补零：字典序 "2026-7-15" > "2026-10-01"，MAX() 判错
		"2026/07/15",           // 分隔符不对
		"2026-07-15T00:00:00Z", // RFC3339：比同键的日期串大，永远判成 Revision
		" 2026-07-15",          // 前导空格
		"2026-07-15 ",          // 尾随空格
		"20260715",             // 无分隔符
		"2026-07-5",            // 日少补零
		"26-07-15",             // 两位年
		"2026-07-15\n",         // 尾随换行（正则须锚到文本末尾而非行末尾）
	}
	for _, v := range bad {
		m := validMeta()
		m.PublishedAt = v
		require.Error(t, m.validate(), "published_at=%q 应被拒", v)
	}
}

// TestMetaValidateRejectsMalformedPeriod 覆盖 G1 的第三种静默后果：period 写成
// "2026-6" 会变成另一个业务键，同一日历月在当前行视图里出现两次，下游环比同比
// 静默算错——同样不报错。
func TestMetaValidateRejectsMalformedPeriod(t *testing.T) {
	bad := []string{
		"2026-6",     // 少补零：与 "2026-06" 是两个不同的业务键
		"2026/06",    // 分隔符不对
		"2026-06-01", // 多了日：period 是期末月，不带日
		" 2026-06",   // 前导空格
		"2026-06 ",   // 尾随空格
		"202606",     // 无分隔符
		"26-06",      // 两位年
		"2026-06\n",  // 尾随换行
	}
	for _, v := range bad {
		m := validMeta()
		m.Period = v
		require.Error(t, m.validate(), "period=%q 应被拒", v)
	}
}

// TestMetaValidateIgnoresIngestedAt —— IngestedAt 由 Save 填写并覆盖调用方传值，
// 校验不看它。判据是「零值与任意值结果相同」，而不只是「两者都不报错」：
// 后者在实现只认某一种取值时也会通过。
func TestMetaValidateIgnoresIngestedAt(t *testing.T) {
	for _, v := range []string{"", "随便什么值", "2026-07-15T00:00:00Z", "   "} {
		base := validMeta()
		base.IngestedAt = v
		require.NoError(t, base.validate(), "IngestedAt=%q 不该影响校验", v)

		// 同一个非法 Meta，IngestedAt 取任意值时错误必须完全一致
		bad := validMeta()
		bad.Extractor = ""
		bad.IngestedAt = v
		require.EqualError(t, bad.validate(), "hestia: meta.extractor must not be empty",
			"IngestedAt=%q 不该改变错误结果", v)
	}
}

func TestCheckStatusConstants(t *testing.T) {
	assert.Equal(t, CheckStatus("passed"), CheckPassed)
	assert.Equal(t, CheckStatus("failed"), CheckFailed)
	assert.Equal(t, CheckStatus("skipped"), CheckSkipped)
}

// TestObservationValuesRepresentAbsence 钉住 Values 用 map 表示缺失的设计：
// 键不存在即该字段缺失，与「值为 0」是两回事——2020 年的报告只有六个板块，
// 用零值表示缺失会让「本就没有」与「解析漏了」无法区分。
func TestObservationValuesRepresentAbsence(t *testing.T) {
	obs := Observation{
		Meta:   validMeta(),
		Values: map[string]float64{"present_zero": 0},
	}
	require.NoError(t, obs.Meta.validate())

	v, ok := obs.Values["present_zero"]
	assert.True(t, ok, "键存在即字段存在，哪怕值为 0")
	assert.Equal(t, 0.0, v)

	_, ok = obs.Values["absent"]
	assert.False(t, ok, "键不存在即字段缺失")
}

func TestValidationReportAndOutcomeShape(t *testing.T) {
	value := 1.5
	rep := ValidationReport{
		Passed: false,
		Checks: []Check{
			{ID: "deposit_sum", Status: CheckFailed, Value: &value},
			{ID: "mom_jump", Status: CheckSkipped, Reason: "no_prior_period"},
		},
	}
	require.Len(t, rep.Checks, 2)
	require.NotNil(t, rep.Checks[0].Value)
	assert.Equal(t, 1.5, *rep.Checks[0].Value)
	assert.Nil(t, rep.Checks[1].Value, "无意义时为 nil，而不是 0")
	assert.Equal(t, "no_prior_period", rep.Checks[1].Reason)

	out := Outcome{Verdict: bitemporal.Revision, Table: "hestia_observations"}
	assert.Equal(t, bitemporal.Revision, out.Verdict)
	assert.Equal(t, "hestia_observations", out.Table)
}

// ── M1b-1.5 · C-2：caliber_version 与 extractor 的取值域 ────────────────────
// error_handling[C-2] caliber_version 非枚举值 → 报错 → TestMetaValidateRejectsUnknownCaliber
// error_handling[C-2] extractor 非枚举值       → 报错 → TestMetaValidateRejectsUnknownExtractor
// functional[C-2]     三个口径版本与三个抽取器均放行 → TestMetaValidateAcceptsKnownEnums

func TestMetaValidateAcceptsKnownEnums(t *testing.T) {
	for _, cv := range []string{"2015-01", "2023-01", "2025-01"} {
		m := validMeta()
		m.CaliberVersion = cv
		require.NoErrorf(t, m.validate(), "caliber_version=%s 应合法", cv)
	}
	for _, ex := range []string{"rule@v1", "rule@v2", "llm-fallback@v1"} {
		m := validMeta()
		m.Extractor = ex
		require.NoErrorf(t, m.validate(), "extractor=%s 应合法", ex)
	}
}

func TestMetaValidateRejectsUnknownCaliber(t *testing.T) {
	// "2024-07" 形态完全合法，却不对应任何真实的口径变更——形态正则拦不住它，
	// 而放行一个虚构的边界等于让下游按它做跨期对比。这正是不用 ^\d{4}-\d{2}$
	// 而用枚举的理由。
	for _, bad := range []string{"garbage", "2099-99", "2024-07", "2025-1", " 2025-01", "2025-01 "} {
		m := validMeta()
		m.CaliberVersion = bad
		err := m.validate()
		require.Errorf(t, err, "caliber_version=%q 应被拒", bad)
		assert.Contains(t, err.Error(), "caliber_version")
	}
}

func TestMetaValidateRejectsUnknownExtractor(t *testing.T) {
	// rule@v3 被拒是有意的：新增抽取器版本时必须同步更新白名单，而那一步正是
	// 提醒去补 completeness 的 profile（M1b-3）——两者不同步会让新模板的期次
	// 用错必填集，且那是静默的。
	for _, bad := range []string{"garbage", "rule@v3", "rule", "RULE@V2", "rule@v2 "} {
		m := validMeta()
		m.Extractor = bad
		err := m.validate()
		require.Errorf(t, err, "extractor=%q 应被拒", bad)
		assert.Contains(t, err.Error(), "extractor")
	}
}

// ── M1b-1.5 · H10：period 与 period_type 的组合 ─────────────────────────────
// error_handling[H10] 组合非法（两值单独合法、配对荒谬）→ 报错
//                                          → TestMetaValidateRejectsBadPeriodCombination
// functional[H10]     合法组合与任意月份的 monthly 均放行
//                                          → TestMetaValidateAcceptsValidCombinations

func TestMetaValidateRejectsBadPeriodCombination(t *testing.T) {
	cases := []struct{ period, periodType string }{
		{"2026-06", "annual"}, // 会用 12 去除半年报期末月，月均折算错一个量级
		{"2026-03", "h1"},     // 上半年的期末月只能是 06
		{"2026-01", "annual"},
		{"2026-12", "h1"},
		// TASK-001 的两种季度类型。前两条是**最容易真的写错**的那两个：q1_q3 与
		// h1 的期末月差 3 个月、除数差 3，而 2026-06/q1_q3 两值单独看都合法。
		{"2026-06", "q1_q3"},
		{"2026-09", "q1"},
		{"2026-12", "q1_q3"},
		{"2026-09", "annual"},
	}
	for _, tc := range cases {
		t.Run(tc.period+"/"+tc.periodType, func(t *testing.T) {
			m := validMeta()
			m.Period, m.PeriodType = tc.period, tc.periodType
			err := m.validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.periodType)
			// 必须是**组合**那道检查拒的，不能是别的。没有这句时，一个还没进白名单的
			// period_type（TASK-001 加 q1/q1_q3 之前正是如此）会被枚举检查先拒掉，
			// 用例照样全绿 —— 而它声称守的组合检查根本没被执行到。
			assert.Contains(t, err.Error(), "month must be",
				"要由期末月检查拒绝，不是被 period_type 枚举检查冒名满足")
		})
	}
}

func TestMetaValidateAcceptsValidCombinations(t *testing.T) {
	// 2026-12/monthly 放行：央行每年 1 月的年报同时含全年与 12 月单月数据，
	// 若解析器把两者都抽出来，YYYY-12/monthly 就是真实期次；且它与
	// YYYY-12/annual 的业务键本就不同（period_type 是主键的一部分）。
	ok := []struct{ period, periodType string }{
		{"2026-12", "annual"},
		{"2026-06", "h1"},
		{"2026-01", "monthly"},
		{"2026-06", "monthly"}, // 6 月的月度数据与 h1 并存，业务键不同
		{"2026-12", "monthly"},
		{"2026-03", "q1"},
		{"2026-09", "q1_q3"},
		// 与 h1/annual 同理：同一期末月的季报与月报是两条独立序列，都要放行。
		{"2026-03", "monthly"},
		{"2026-09", "monthly"},
	}
	for _, tc := range ok {
		t.Run(tc.period+"/"+tc.periodType, func(t *testing.T) {
			m := validMeta()
			m.Period, m.PeriodType = tc.period, tc.periodType
			require.NoError(t, m.validate())
		})
	}
}
