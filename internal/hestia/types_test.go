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
	for _, bad := range []string{"quarterly", "H1", "yearly", " h1", "h1 ", "MONTHLY"} {
		m := validMeta()
		m.PeriodType = bad
		require.Error(t, m.validate(), "period_type=%q 应被拒", bad)
	}
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
	}
	for _, tc := range cases {
		t.Run(tc.period+"/"+tc.periodType, func(t *testing.T) {
			m := validMeta()
			m.Period, m.PeriodType = tc.period, tc.periodType
			err := m.validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.periodType)
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
	}
	for _, tc := range ok {
		t.Run(tc.period+"/"+tc.periodType, func(t *testing.T) {
			m := validMeta()
			m.Period, m.PeriodType = tc.period, tc.periodType
			require.NoError(t, m.validate())
		})
	}
}
