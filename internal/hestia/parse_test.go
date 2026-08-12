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

		// —— 季报（TASK-010）。期次值**逐字采用 TASK-001 discovery 里定的那一套** ——
		//
		// 两套会在 Meta.validate 处炸（periodEndMonth 的组合校验），而那是运行时。
		// discover.go 的 parsePeriod 与这里的 parseTitle 是**两份并行的**期次解析，
		// 一个看列表页链接文本、一个看文章页的 <meta name="ArticleTitle">，
		// 期末月约定必须同源：q1→03、h1→06、q1_q3→09、annual→12。
		{"2026年一季度金融统计数据报告", "2026-03", "q1"},
		{"2025年前三季度金融统计数据报告", "2025-09", "q1_q3"},
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

// ⚠️ TASK-010 起「季度」不再一律不认 —— `一季度` / `前三季度` 已是合法形态。
// 本组里留下的几条季度标题各有各的拒绝理由，写在行末，别再笼统写成「季度报表不认」。
func TestParseTitleRejectsUnknownShape(t *testing.T) {
	for _, bad := range []string{
		// 「第三季度」带「第」字，且央行发的是「前三季度」（1–9 月累计）而不是
		// 第 3 季度单季（7–9 月）。两者期末月同为 09、月均折算除数却是 9 与 3
		// ——认下来就是 types.go 警告的「错一个量级且完全看不出来」。
		"2025年第三季度金融统计数据报告",
		// 同上，去掉「第」字也不认：央行不发二/三/四季度的《金融统计数据报告》，
		// 那三段分别由上半年 / 前三季度 / 全年覆盖（与 discover.go 的 parsePeriod 同口径）。
		"2025年三季度金融统计数据报告",
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

// 🔴 TASK-010：季度类型同样要被**显式拒绝**，直到 TASK-004 把抽取侧接上。
//
// 用 TASK-001 落在 testdata/ 的**真实季报正文**跑，而不是合成 HTML —— 合成 HTML
// 只能证明「拒绝逻辑会触发」，真实样本才能证明「真拿到这篇报告时它确实触发」。
// 两份样本的 <meta name="ArticleTitle"> 与 PubDate 都是真的（2026-04-13 / 2025-10-15）。
//
// ⚠️ **本条已按原作者的指示完成那次转换**（M1b-4b / TASK-004）：它原名
// TestParseRejectsQuarterlyUntilExtractorWired、断言的是「必须显式拒绝」，
// 原注释写着「TASK-004 接上抽取侧后……**别把它删掉** —— 改成正例，它就变回
// 一条端到端守卫」。现已改成正例并更名。
//
// 记下这段来历是因为**这类测试最容易被后人误删**：它此前断言的行为如今恰好相反，
// 不知情的人会以为它已过时。而它守的东西没变，只是从「中间态说得清」变成
// 「终态跑得通」。
func TestParseAcceptsQuarterlyReports(t *testing.T) {
	for _, tc := range []struct {
		sample, title, period, periodType string
	}{
		{"pboc-2026-03-q1.html", "2026年一季度金融统计数据报告", "2026-03", "q1"},
		{"pboc-2025-09-q3.html", "2025年前三季度金融统计数据报告", "2025-09", "q1_q3"},
	} {
		t.Run(tc.sample, func(t *testing.T) {
			raw := readSample(t, tc.sample)

			// ① 标题层认得它 —— 这是 TASK-010 的正面主张，先钉住。
			title, ok := metaContent(raw, "ArticleTitle")
			require.True(t, ok, "真实样本必须有 ArticleTitle")
			require.Equal(t, tc.title, title, "样本标题与 TASK-001 记录的一致")

			period, pt, err := parseTitle(title)
			require.NoError(t, err, "季报标题必须被 parseTitle 认出")
			assert.Equal(t, tc.period, period)
			assert.Equal(t, tc.periodType, pt)

			// ② 整条 Parse 必须**跑通**（M1b-4b / TASK-004 起）。
			//
			// 本条原是拒绝断言，按原作者留下的指示改成正例而不是删掉 ——
			// 它于是从「中间态的自解释守卫」变回「端到端守卫」，覆盖面不减反增。
			obs, err := Parse(raw)
			require.NoError(t, err, "季报抽取侧已接线（periodAlt + cumulativePeriods），必须跑通")
			assert.Equal(t, tc.period, obs.Meta.Period, "正文解析出的期次要与标题一致")
			assert.Equal(t, tc.periodType, obs.Meta.PeriodType)
			assert.Equal(t, extractorV2, obs.Meta.Extractor, "实测两份季报各 8 板块，与年报同构")
			assert.NotEmpty(t, obs.Values, "跑通意味着真的抽到了值，不是返回一份空壳")
		})
	}
}

// 🔴 每一个合法 period_type 都必须在 checkPeriodTypeSupported 里有**明确的**去向。
//
// 这条守的是那个函数**此前真实存在的缺口**：它写的是 `if periodType != "monthly"
// { return nil }` —— 一条**默认放行**的规则。TASK-001 加了 q1 / q1_q3 之后，
// 两种零样本的期次类型就这样直接穿过去了，而没有任何测试会红。
//
// ⚠️ 判据是**默认放行 vs 默认拒绝**，不是「列表全不全」：新增第六种 period_type 时，
// 默认放行的写法会让它静默进入抽取层（产出看起来正常、实则口径不明的 Values），
// 而本条会红并逼人**明确表态**。表里那句 why 就是逼人写下理由的地方。
//
// 与 types.go 的 TestPeriodTypeMapsAreConsistent 同族：都是「白名单加了新取值，
// 另一处必须跟上」的绊线，只是那条守期末月，这条守抽取侧支持与否。
func TestEveryPeriodTypeHasAnExplicitSupportDecision(t *testing.T) {
	// supported[pt] = 是否允许进入抽取层；why 只写给读代码的人看，不参与断言。
	decisions := map[string]struct {
		supported bool
		why       string
	}{
		"annual":  {true, "有真实样本 pboc-2025-12-annual.html"},
		"h1":      {true, "有真实样本 pboc-2020-06-h1.html"},
		"monthly": {false, "零样本；孪生句问题最严重，哪句在前无样本可证"},
		"q1":      {true, "TASK-004 已接线：periodAlt 加了「一季度」、cumulativePeriods 同步登记；真实样本 pboc-2026-03-q1.html"},
		"q1_q3":   {true, "TASK-004 已接线：periodAlt 加了「前三季度」（**不是**「三季度」）；真实样本 pboc-2025-09-q3.html"},
	}

	// 前置锚点：表必须与白名单**一一对应**。少了会让下面的循环漏掉某个取值，
	// 多了说明表里有个已经不存在的 period_type。
	require.Len(t, decisions, len(validPeriodTypes),
		"新增 period_type 必须在这里明确表态：进抽取层，还是显式拒绝并写明理由")
	for pt := range validPeriodTypes {
		require.Containsf(t, decisions, pt, "period_type %q 没有在本表里表态", pt)
	}

	for pt, d := range decisions {
		t.Run(pt, func(t *testing.T) {
			err := checkPeriodTypeSupported(pt, "2026年X金融统计数据报告")
			if d.supported {
				assert.NoErrorf(t, err, "%s 应当放行（%s）", pt, d.why)
				return
			}
			require.Errorf(t, err, "%s 应当被显式拒绝（%s）", pt, d.why)
			assert.Contains(t, err.Error(), pt, "错误信息要指名是哪种 period_type")
			assert.Contains(t, err.Error(), "2026年X金融统计数据报告", "并带上原标题")
		})
	}
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

// —— TASK-006 返工（QA WARNING-2）：PubDate 的三态与形态校验 ——

// TestParseRejectsBadPubDate 覆盖返工判据。
//
// 修复前 `Parse` 只判 `metaContent` 的第二返回值（`ok`），于是 content=""、
// 「2026-1-15」（月份未补零）、「2026-01-15 09:30:00」（带时分秒）**三种全部
// err=nil 且照常抽出 54 个字段**——错误一路推迟到 `Store.Save` 的
// `publishedAtRE` 才现场，**而那时 raw HTML 早已不在手上**，排障要从一个
// 「格式不对」的报错反推是哪篇文章的哪个 meta。
//
// 这不是设计选择而是漏了一处：对照 `ArticleTitle` 挖空**会**在 Parse 内经
// parseTitle 响亮失败。`published_at` 是全包唯一一个逐字来自外部 HTML、
// 不经任何模板的字段，也是「凡从输入文本读来的东西认不出就报错、绝不猜」
// 这条规则的唯一偏离点。
//
// 用**真实样本改一个字节**而不是构造最小 HTML：这样验的是整条流水线在
// 一份完全正常的报告上因 PubDate 而停下，而不是一个玩具输入。
func TestParseRejectsBadPubDate(t *testing.T) {
	const orig = `<meta name="PubDate" content="2026-01-15">`
	raw := string(readSample(t, "pboc-2025-12-annual.html"))
	require.Contains(t, raw, orig, "样本里的 PubDate 原文变了，本用例的替换不再生效")

	for _, tc := range []struct{ name, replacement string }{
		{"content 为空", `<meta name="PubDate" content="">`},
		{"月份未补零", `<meta name="PubDate" content="2026-1-15">`},
		{"带时分秒", `<meta name="PubDate" content="2026-01-15 09:30:00">`},
		{"整个 meta 缺失", ``},
	} {
		t.Run(tc.name, func(t *testing.T) {
			obs, err := Parse([]byte(strings.Replace(raw, orig, tc.replacement, 1)))
			require.Error(t, err, "PubDate 认不出时必须在 Parse 就停下，不能推迟到 Save")
			assert.Contains(t, err.Error(), "PubDate", "错误必须指名是哪个 meta")
			assert.Empty(t, obs.Values, "不得先抽完 54 个字段再报错")
		})
	}
}

// TestParseDistinguishesPubDateFailureModes 钉住三种失败各有专属措辞。
//
// strip.go 的注释明写 metaContent 的第二返回值「区分『不存在』与『存在但为空』：
// 站点确实会输出 content=""，调用方需要能分辨是站点没填还是选择器写错了」。
//
// ⚠️ 只断言「两条错误不相同」是不够的——变异实测发现的：空串同样过不了
// publishedAtRE，会落到形态分支照样报错，于是 `case pubDate == ""` 那一支
// **不承载任何断言**（变异「删掉空值分支」首轮 SURVIVED）。差异仅来自错误里
// 引用的那个 %q 值，而排障方向并没有被区分出来。故这里额外断言**各自的关键词**，
// 让那一支真正承重。
func TestParseDistinguishesPubDateFailureModes(t *testing.T) {
	const orig = `<meta name="PubDate" content="2026-01-15">`
	raw := string(readSample(t, "pboc-2025-12-annual.html"))
	parseWith := func(replacement string) error {
		_, err := Parse([]byte(strings.Replace(raw, orig, replacement, 1)))
		return err
	}

	missing := parseWith(``)
	empty := parseWith(`<meta name="PubDate" content="">`)
	malformed := parseWith(`<meta name="PubDate" content="2026-1-15">`)
	require.Error(t, missing)
	require.Error(t, empty)
	require.Error(t, malformed)

	assert.NotEqual(t, missing.Error(), empty.Error(), "「不存在」与「存在但为空」不得同措辞")
	assert.NotEqual(t, empty.Error(), malformed.Error(), "「为空」与「形态不合」不得同措辞")
	assert.NotEqual(t, missing.Error(), malformed.Error())

	// 各带专属关键词：只有「不相同」的话，差异可能仅是被引用的值不同，
	// 而三种情形的排障方向（选择器写错 / 站点没填 / 站点填了但格式变了）没被区分。
	assert.Contains(t, missing.Error(), "missing", "缺失应指向「页面结构变了或选择器写错」")
	assert.Contains(t, empty.Error(), "empty", "为空应指向「站点没填」，而不是复用形态不合的措辞")
	assert.Contains(t, malformed.Error(), "YYYY-MM-DD", "形态不合应给出期望形态")
}

// TestParsePubDateShapeMatchesStoreContract 是与 M1b-1 的接缝检查。
//
// Parse 用的形态校验必须与 Store.Save 那道（types.go 的 publishedAtRE）**同一条**，
// 否则会出现「Parse 放行而 Save 拒绝」的缝——那正是返工前的状态。
func TestParsePubDateShapeMatchesStoreContract(t *testing.T) {
	for _, s := range []string{"2026-1-15", "2026-01-15 09:30:00", "", "2026/01/15"} {
		assert.Falsef(t, publishedAtRE.MatchString(s), "%q 不该被 M1b-1 的形态契约接受", s)
	}
	assert.True(t, publishedAtRE.MatchString("2026-01-15"))

	obs, err := Parse(readSample(t, "pboc-2025-12-annual.html"))
	require.NoError(t, err)
	assert.True(t, publishedAtRE.MatchString(obs.Meta.PublishedAt),
		"Parse 产出的 published_at 必须直接满足 Save 的形态契约")
}
