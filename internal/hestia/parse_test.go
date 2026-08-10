package hestia

// Context Checkpoint: done_criteria → test mapping (TASK-006 parse)
// functional[0]     Parse 串起四层，Values 键全在 allFields 内  → TestParseRealSamples、TestParseValuesKeysAreDeclared
// ——— TASK-008（Parse 值正确性的常驻守护）———
// T008 functional[0..1] Parse 输出的 Values 与 golden **逐键逐值**双向比对，
//                   走 Parse 入口而非 extractFields                → TestParseRealSamples
// T008 boundary[0]  非空转自证 require.NotEmpty(obs.Values)        → TestParseRealSamples
// T008 error_handling[0] 差异能定位到具体字段名（扰动 golden 实测） → assertMatchesGolden 的 InDeltaf/Truef 带字段名
// functional[1]     parseTitle 产出满足 Meta.validate 的两个正则 → TestParseTitle、TestParseTitlePadsMonth
// functional[2]     caliberFor 取值在 validCaliberVersions 内   → TestCaliberForResultIsAlwaysValid
// functional[2]     多条口径注同时适用取**最新**，且不依赖遍历顺序
//                                                             → TestCaliberFor、TestCaliberForIsOrderIndependent
// boundary[0]       两份样本 Meta 七字段与 goldenMeta* 逐字段相等 → TestParseRealSamples
// boundary[1]       period×period_type 组合过 M1b-1 组合校验     → TestParseMetaPassesM1b1Validation
// boundary[2]       monthly 路径零样本 → **显式拒绝**（选项①）   → TestParseRejectsMonthlyUntilSampled
// error_handling[0] 标题形态不认识报错且信息含原标题            → TestParseTitleRejectsUnknownShape
// error_handling[1] Parse 不填 ArticleID                       → TestParseRealSamples、TestParseMetaPassesM1b1Validation
// error_handling[2] 切分不认识就停：detectExtractor 的 error 致命，
//                   且**不产出任何 Values**                     → TestParseStopsAtDetectExtractor
// error_handling[*] 缺 PubDate / ArticleTitle 分别报错          → TestParseRejectsMissingMeta
// non_functional[0] parse.go 不碰库：无 database/sql、无 Save(  → TestParseDoesNotTouchStorage

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTitle(t *testing.T) {
	for _, tc := range []struct {
		title      string
		period     string
		periodType string
	}{
		{"2025年金融统计数据报告", "2025-12", "annual"},
		{"2020年上半年金融统计数据报告", "2020-06", "h1"},
		{"2026年6月金融统计数据报告", "2026-06", "monthly"},
		{"2026年11月金融统计数据报告", "2026-11", "monthly"},
		{"2026年1月金融统计数据报告", "2026-01", "monthly"},
	} {
		t.Run(tc.title, func(t *testing.T) {
			period, pt, err := parseTitle(tc.title)
			require.NoError(t, err)
			assert.Equal(t, tc.period, period)
			assert.Equal(t, tc.periodType, pt)

			// functional[1]：产出必须直接满足 M1b-1 的两个形态契约
			assert.Regexp(t, `\A[0-9]{4}-[0-9]{2}\z`, period, "period 必须形如 YYYY-MM")
			assert.True(t, validPeriodTypes[pt], "period_type 必须在 M1b-1 的白名单内")
		})
	}
}

// TestParseTitlePadsMonth 单独钉住补零。
//
// 「2026年6月」若产出 "2026-6"，M1b-1 的 periodRE 会拒绝它——但更糟的情况是
// 万一那道校验被放宽：bitemporal 用字典序比较业务键，"2026-6" 会成为与
// "2026-06" 不同的键，同一日历月在视图里出现两次，下游环比同比静默算错。
func TestParseTitlePadsMonth(t *testing.T) {
	period, _, err := parseTitle("2026年6月金融统计数据报告")
	require.NoError(t, err)
	assert.Equal(t, "2026-06", period, "月份必须补零")
	assert.Len(t, period, 7)
}

func TestParseTitleRejectsUnknownShape(t *testing.T) {
	for _, bad := range []string{
		"2025年第三季度金融统计数据报告", // 季度报表：不是已知的三种期次
		"2025年社会融资规模统计数据报告", // 另一篇报告，板块结构完全不同
		"金融统计数据报告",
		"2025年13月金融统计数据报告",  // 月份越界
		"2025年0月金融统计数据报告",   // 月份下界
		"2025年金融统计数据报告(修订)", // 后缀不锚定就会被认下来
		"",
	} {
		t.Run(bad, func(t *testing.T) {
			_, _, err := parseTitle(bad)
			require.Error(t, err, "不认识的标题形态必须报错，不能猜")
			if bad != "" {
				assert.Contains(t, err.Error(), bad, "错误信息必须带上原标题，否则排障看不出是哪篇")
			}
		})
	}
}

func TestCaliberFor(t *testing.T) {
	for _, tc := range []struct{ period, want string }{
		{"2019-12", "2015-01"},
		{"2020-06", "2015-01"},
		{"2022-12", "2015-01"}, // 2023-01 生效前的最后一期
		{"2023-01", "2023-01"}, // 边界：当期即生效
		{"2024-12", "2023-01"},
		{"2025-01", "2025-01"}, // 边界：M1 口径修订当期
		{"2025-12", "2025-01"}, // 2025 年报：注4 与注5 **同时适用**，取最新
		{"2026-06", "2025-01"},
	} {
		t.Run(tc.period, func(t *testing.T) {
			assert.Equal(t, tc.want, caliberFor(tc.period))
		})
	}
}

// TestCaliberForIsOrderIndependent 钉住 functional[2] 那句「**在实现中显式表达
// 「取最新」，而非依赖遍历顺序**」。
//
// 2025 年报同时含注4（2023-01）与注5（2025-01），两条口径注都已生效。若实现成
// 「按表顺序取首个命中」，正确性就寄生在表的排列上——有人按时间正序重排一次表，
// 2025-12 会静默变成 2023-01，而那是一个**合法**的 caliber_version，
// M1b-1 的白名单拦不住它，下游会拿它做跨期对比。
//
// 本测试把表打乱后重跑：结果必须一字不差。
func TestCaliberForIsOrderIndependent(t *testing.T) {
	original := caliberChanges
	t.Cleanup(func() { caliberChanges = original })

	probes := []string{"2019-12", "2022-12", "2023-01", "2024-12", "2025-01", "2025-12", "2030-06"}
	want := make([]string, len(probes))
	for i, p := range probes {
		want[i] = caliberFor(p)
	}

	// 逐个旋转，覆盖全部循环排列（含正序、倒序之外的中间排列）
	for shift := 1; shift < len(original); shift++ {
		rotated := make([]caliberChange, 0, len(original))
		rotated = append(rotated, original[shift:]...)
		rotated = append(rotated, original[:shift]...)
		caliberChanges = rotated

		for i, p := range probes {
			assert.Equalf(t, want[i], caliberFor(p),
				"表旋转 %d 位后 caliberFor(%q) 变了——说明实现依赖遍历顺序而非「取最新」", shift, p)
		}
	}
}

// TestCaliberForResultIsAlwaysValid 保证推导结果一定过得了 M1b-1 的白名单。
//
// caliberFor 是 caliber_version 的唯一产地，而 M1b-1 用枚举校验它——两处若脱节，
// Parse 会产出一个 Save 必拒的 Observation，而那要到 M1b-4 接线时才暴露。
func TestCaliberForResultIsAlwaysValid(t *testing.T) {
	for _, p := range []string{"0001-01", "2015-01", "2019-12", "2023-01", "2025-01", "2030-06", "9999-12"} {
		got := caliberFor(p)
		assert.Containsf(t, validCaliberVersions, got,
			"caliberFor(%q)=%q 不在 M1b-1 的白名单里", p, got)
	}
}

// TestCaliberChangesAreDeclaredVersions：口径变更表里的每个 version 也必须在白名单内。
//
// 上一条只抽查了若干 period；这条从表的另一端封口，新增一条口径注时立刻生效。
func TestCaliberChangesAreDeclaredVersions(t *testing.T) {
	require.NotEmpty(t, caliberChanges, "口径变更表为空，本检查毫无意义")
	for _, c := range caliberChanges {
		assert.Containsf(t, validCaliberVersions, c.version,
			"口径变更表里的 %q 不在 M1b-1 的 validCaliberVersions 内", c.version)
	}
}

// TestParseRealSamples 同时覆盖 Meta 七字段与 Values 的逐键逐值比对。
//
// Values 那一半（TASK-008 补）**不是 T5 的重复**，两者喂的输入不同：
//
//	T5 的 TestExtractFieldsOn*Sample → 喂 extractFields，输入是它自己构造的固定 sections
//	本条                             → 喂 Parse，输入是原始 HTML，走完 strip→detect→scope→extract
//
// ⇒ **本条能抓住 T5 抓不到的：上游任一层的变化导致值错。** 在补上它之前，
// parse_test.go 对 Values 只有 require.Len 的键数断言，于是 stripHTML 之类上游微变
// 可以让 Parse 产出「键数仍是 54/27、值已经错」的结果而**常驻套件不红**。
// 两条断言必须并存，不得以「重复」为由删掉任何一条。
func TestParseRealSamples(t *testing.T) {
	for _, tc := range []struct {
		sample string
		want   Meta
		values int
		golden map[string]float64
	}{
		{"pboc-2025-12-annual.html", goldenMeta2025, 54, golden2025},
		{"pboc-2020-06-h1.html", goldenMeta2020, 27, golden2020},
	} {
		t.Run(tc.sample, func(t *testing.T) {
			obs, err := Parse(readSample(t, tc.sample))
			require.NoError(t, err)

			assert.Equal(t, tc.want.Period, obs.Meta.Period)
			assert.Equal(t, tc.want.PeriodType, obs.Meta.PeriodType)
			assert.Equal(t, tc.want.PublishedAt, obs.Meta.PublishedAt)
			assert.Equal(t, tc.want.CaliberVersion, obs.Meta.CaliberVersion)
			assert.Equal(t, tc.want.Extractor, obs.Meta.Extractor)
			assert.Empty(t, obs.Meta.ArticleID, "ArticleID 是 URL 的属性，Parse 看不到 URL")
			assert.Empty(t, obs.Meta.IngestedAt, "IngestedAt 由 Store.Save 填")

			require.Len(t, obs.Values, tc.values)

			// 非空转自证：放在比对之前，让「一个字段都没抽到」以这条的措辞失败，
			// 而不是以下面 54 条「字段 X 没被抽到」的形式刷屏。
			require.NotEmpty(t, obs.Values, "抽出 0 个字段，本比对毫无意义")

			// 双向比对：golden 每项都要抽到且相等（InDelta 1e-6），
			// 抽到的每项也都要在 golden 内。helper 复用 extract_test.go 的那一份，
			// 不另写——两份比对逻辑迟早分叉。
			assertMatchesGolden(t, obs.Values, tc.golden)
		})
	}

	t.Run("六板块期次不该有社融字段", func(t *testing.T) {
		obs, err := Parse(readSample(t, "pboc-2020-06-h1.html"))
		require.NoError(t, err)
		_, ok := obs.Values[FieldTSFStock]
		assert.False(t, ok, "应当是键不存在，而不是零值")
	})
}

// TestParseValuesKeysAreDeclared 覆盖 functional[0] 的后半句。
func TestParseValuesKeysAreDeclared(t *testing.T) {
	for _, sample := range []string{"pboc-2025-12-annual.html", "pboc-2020-06-h1.html"} {
		obs, err := Parse(readSample(t, sample))
		require.NoError(t, err)
		require.NotEmpty(t, obs.Values, "抽出 0 个字段，本检查毫无意义")
		for k := range obs.Values {
			assert.Truef(t, allFields[k], "%s: Values 含 allFields 之外的键 %q", sample, k)
		}
	}
}

func TestParseRejectsMissingMeta(t *testing.T) {
	for _, tc := range []struct{ name, raw, want string }{
		{"缺 PubDate",
			`<html><head><meta name="ArticleTitle" content="2025年金融统计数据报告"></head><body></body></html>`,
			"PubDate"},
		{"缺 ArticleTitle",
			`<html><head><meta name="PubDate" content="2026-01-15"></head><body></body></html>`,
			"ArticleTitle"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			obs, err := Parse([]byte(tc.raw))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
			assert.Empty(t, obs.Values)
		})
	}
}

// TestParseStopsAtDetectExtractor 覆盖 error_handling[2]。
//
// 这条落地的是 T3 留下的一个**悬空承诺**：T3 接受 `splitSections` 无 error 的
// 签名，依据是「切分失败由紧邻的 detectExtractor 报错」。但 grep 实测两个函数
// 的生产调用点当时都是 0——那个「紧邻」是对本任务的预期，不是既有事实。
// T3 的 error_handling[0] 所要求的保证，在本函数写出来之前不存在于代码库任何位置。
//
// 判据有两半，缺一不可：
//  1. 报错且信息含**实际**板块数（不是期望值——报「期望 6 或 8」而不说实际几个，
//     排障时等于没说）
//  2. **不产出任何 Values**——是「切分不认识就停」，不是「先抽取再报错」
func TestParseStopsAtDetectExtractor(t *testing.T) {
	const head = `<html><head><meta name="PubDate" content="2026-01-15">` +
		`<meta name="ArticleTitle" content="2025年金融统计数据报告"></head><body>`

	for _, tc := range []struct {
		name, body, wantCount string
	}{
		{"0 板块", `<p>正文里没有任何板块标题。</p>`, "0"},
		{"3 板块", `<p>一、甲</p><p>二、乙</p><p>三、丙</p>`, "3"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			obs, err := Parse([]byte(head + tc.body + `</body></html>`))
			require.Error(t, err, "板块数不认识时必须停下，不能按已知模板去抽一份没见过的报告")
			assert.Contains(t, err.Error(), tc.wantCount, "错误信息必须给出实际板块数")
			assert.Empty(t, obs.Values, "不得先抽取再报错——此时不该有任何 Values")
		})
	}
}

// TestParseRejectsMonthlyUntilSampled 覆盖 boundary[2]，本任务选择**选项①**。
//
// monthly 路径零样本、零判据：两份样本一份 annual 一份 h1。而 T5 已实测，
// 月报正是孪生句问题最严重的形态——正文同时含期内累计句与当月句，
// 且哪句在前**无样本可证**；T5 的 cumulativePeriods 只认「全年/上半年」，
// 对 5 月报的「1-5月」这类前缀会整条不命中。
//
// 两条路里选显式拒绝，理由与全包一致：拿不准时不要猜。悄悄放行会让一份月报
// 产出**看起来完全正常**的 Values——键数对、量级对、全在白名单内，而 YTD 字段
// 装的可能是当月数。那种错下游没有任何闸门拦得住。
//
// parseTitle 仍然解析 monthly（它只做标题→期次的映射，本身可测且正确），
// 拒绝只发生在 Parse 这一层——拿到月报样本后删掉这道守卫即可，
// 那时 parseTitle 与它的测试一行都不用改。
func TestParseRejectsMonthlyUntilSampled(t *testing.T) {
	raw := []byte(`<html><head><meta name="PubDate" content="2026-07-10">` +
		`<meta name="ArticleTitle" content="2026年6月金融统计数据报告"></head><body>` +
		`<p>一、甲</p><p>二、乙</p></body></html>`)

	obs, err := Parse(raw)
	require.Error(t, err, "月报无样本验证，必须显式拒绝而不是沉默放行")
	assert.Contains(t, err.Error(), "monthly")
	assert.Empty(t, obs.Values)

	// parseTitle 这一层不受影响：它对月报标题仍然正确
	period, pt, err := parseTitle("2026年6月金融统计数据报告")
	require.NoError(t, err)
	assert.Equal(t, "2026-06", period)
	assert.Equal(t, "monthly", pt)
}

// TestParseMetaPassesM1b1Validation 是与 M1b-1 的接缝检查，同时覆盖 boundary[1]。
//
// Parse 按设计不填 ArticleID，所以要补一个才能过 validate——这一步同时验证了
// 「其余五项都填对了」与「那个空是有意留的」。period×period_type 的组合校验
// （h1→06、annual→12）也在 validate 里，一并被这条盖住。
func TestParseMetaPassesM1b1Validation(t *testing.T) {
	for _, f := range []string{"pboc-2020-06-h1.html", "pboc-2025-12-annual.html"} {
		t.Run(f, func(t *testing.T) {
			obs, err := Parse(readSample(t, f))
			require.NoError(t, err)

			m := obs.Meta
			require.Error(t, m.validate(), "缺 ArticleID 时应当不通过")

			m.ArticleID = "filled-by-m1b4"
			require.NoError(t, m.validate(),
				"补上 ArticleID 后其余五项都应合法——含 period×period_type 组合与两个枚举")
		})
	}
}

// TestParseDoesNotTouchStorage 覆盖 non_functional[0]。
//
// 走 go/parser 而不是整文件子串匹配：注释里说明「本层不落库」时会写到
// Save 这个词，整文件扫描会把说明文字判成违规。恒响的检查会被训练成忽略。
func TestParseDoesNotTouchStorage(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "parse.go", nil, 0)
	require.NoError(t, err)

	require.NotEmpty(t, f.Imports, "解析出 0 个 import，本检查的绿色是假的")
	for _, im := range f.Imports {
		assert.NotEqual(t, `"database/sql"`, im.Path.Value, "parse.go 不得直接碰数据库")
		// import 路径里也不该出现任何 sql 驱动
		assert.NotContains(t, strings.ToLower(im.Path.Value), "sqlite")
	}

	calls := 0
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		calls++
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			assert.NotEqual(t, "Save", sel.Sel.Name, "parse.go 不得调用 Save —— 落库是 M1b-4 的事")
		}
		return true
	})
	require.NotZero(t, calls, "没扫到任何函数调用，本检查的绿色是假的")
}
