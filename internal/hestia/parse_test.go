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
		kind       string
	}{
		{"2025年金融统计数据报告", "2025-12", "annual", kindFinance},
		{"2020年上半年金融统计数据报告", "2020-06", "h1", kindFinance},
		{"2026年6月金融统计数据报告", "2026-06", "monthly", kindFinance},
		{"2026年11月金融统计数据报告", "2026-11", "monthly", kindFinance},
		{"2026年1月金融统计数据报告", "2026-01", "monthly", kindFinance},

		// —— 社融两种（M1c-3a 的 TASK-007）——
		//
		// kind 与 (period, periodType) **相互独立**：同一个期次段落在三种报告上都合法，
		// 所以下面四条刻意让期次与 kind 交叉，别只测「社融+annual」一种组合。
		{"2025年8月社会融资规模存量统计数据报告", "2025-08", "monthly", kindTSFStock},
		{"2025年前三季度社会融资规模增量统计数据报告", "2025-09", "q1_q3", kindTSFFlow},
		{"2025年社会融资规模存量统计数据报告", "2025-12", "annual", kindTSFStock},
		{"2020年一季度社会融资规模增量统计数据报告", "2020-03", "q1", kindTSFFlow},

		// —— 季报（TASK-010）。期次值**逐字采用 TASK-001 discovery 里定的那一套** ——
		//
		// 两套会在 Meta.validate 处炸（periodEndMonth 的组合校验），而那是运行时。
		// discover.go 的 parsePeriod 与这里的 parseTitle 是**两份并行的**期次解析，
		// 一个看列表页链接文本、一个看文章页的 <meta name="ArticleTitle">，
		// 期末月约定必须同源：q1→03、h1→06、q1_q3→09、annual→12。
		{"2026年一季度金融统计数据报告", "2026-03", "q1", kindFinance},
		{"2025年前三季度金融统计数据报告", "2025-09", "q1_q3", kindFinance},
	} {
		t.Run(tc.title, func(t *testing.T) {
			period, pt, kind, err := parseTitle(tc.title)
			require.NoError(t, err)
			assert.Equal(t, tc.period, period)
			assert.Equal(t, tc.periodType, pt)
			assert.Equal(t, tc.kind, kind, "kind 决定 Parse 走哪条抽取路径，错了会拿板块切分去处理整篇一段的社融报告")

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
	period, _, _, err := parseTitle("2026年6月金融统计数据报告")
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
		// ⚠️ 「社会融资规模统计数据报告」少了「存量/增量」二字，是**第三篇**报告
		// （M1c-3a 的 TASK-007 之后，存量与增量两种已被 titleRE 认下来，这一种仍不认）。
		// 它是「后缀锚定不能放松成 `社会融资规模.*报告`」的活证据。
		"2025年社会融资规模统计数据报告",
		// DoD error_handling[0] 点名的例子：完全另一类报告
		"2026年二季度金融机构贷款投向统计报告",
		"金融统计数据报告",
		"2025年13月金融统计数据报告",  // 月份越界
		"2025年0月金融统计数据报告",   // 月份下界
		"2025年金融统计数据报告(修订)", // 后缀不锚定就会被认下来
		"",
	} {
		t.Run(bad, func(t *testing.T) {
			_, _, _, err := parseTitle(bad)
			require.Error(t, err, "不认识的标题形态必须报错，不能猜")
			if bad != "" {
				assert.Contains(t, err.Error(), bad, "错误信息必须带上原标题，否则排障看不出是哪篇")
			}
			// M1c-3a 的 TASK-007：认三种报告之后，「形态不认识」这条错误必须把**三种**
			// 都列出来，否则拿到一篇社融报告的人只会看到「想要金融统计数据报告」，方向被指反。
			//
			// ⚠️ **只对这一条错误路径断言**：`2025年13月…` / `2025年0月…` 走的是另一条
			// （`invalid month in report title`）——那两个标题的**形态是认得的**，坏在月份取值。
			// 在那里也列三种形态只会把「你给的月份越界」这个真实原因淹没。
			// 判据用错误前缀区分，而不是靠标题内容反推，免得将来加了新形态时这里悄悄失配。
			if strings.Contains(err.Error(), "unrecognized report title") {
				for _, k := range []string{kindFinance, kindTSFStock, kindTSFFlow} {
					assert.Containsf(t, err.Error(), k, "错误信息必须列出 %q 这种形态", k)
				}
			} else {
				assert.Contains(t, err.Error(), "invalid month",
					"除「形态不认识」外，本组只应出现「月份越界」这一条错误路径；"+
						"冒出第三种说明 parseTitle 多了一条没人知道的失败路径")
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

// TestParseAcceptsMonthlyReports —— 由 TestParseRejectsMonthlyUntilSampled **转正例**而来
// （M1c-3a 的 TASK-007）。
//
// # 来历：它此前断言的行为如今恰好相反，**别把它当过时测试删掉**
//
// 原测试覆盖 boundary[2]，断言「monthly 必须被显式拒绝」，理由是那时**零样本零判据**：
// 月报正文同时含期内累计句与当月句，哪句在前无样本可证，而当时的 cumulativePeriods
// 只认「全年/上半年」。原注释写明「拿到月报样本后删掉这道守卫即可」。
//
// 这与 TASK-004 对季报那次转换是**同一手法**（见下一条测试的注释）：守的东西没变，
// 只是从「中间态说得清」变成「终态跑得通」。删掉它等于把这条路径的端到端证据一起删掉。
//
// ⚠️ **原拒绝理由里有一句是错的，一并记在这里免得后人照它推断**：原注释称
// 「cumulativePeriods 认不出 5 月报的『1-5月』这类前缀」。Leader 全量实读 55 篇月报，
// **「1-5月」这种带范围的形态一次都没出现**——真实形态是「前八个月」这类中文数字前缀
// （累计）与「8月份」（当月，要排除），1 月报的「1月份」是累计特例。那句写于零样本时期，
// 是**推测**而非实测。（同一句话在 parse.go 的 checkPeriodTypeSupported 注释里也订正了。）
//
// 现在四份真实月报快照端到端跑通，解除条件已满足：TASK-001 给了「前N个月」十项与
// 「1月份」特例，TASK-004 让板块切分认 4/5/7 节月报，TASK-006 让抽取侧按 extractor
// 决定板块适用性。
func TestParseAcceptsMonthlyReports(t *testing.T) {
	obs, err := Parse(readTestdata(t, "pboc-2025-08-monthly.html"))
	require.NoError(t, err, "月报解除支持后必须端到端跑通")
	assert.Equal(t, "monthly", obs.Meta.PeriodType)
	assert.NotEmpty(t, obs.Values, "跑通不等于抽到东西——空 Values 同样是失败")

	// checkPeriodTypeSupported 的 monthly 分支已删，但**函数与穷举 switch 保留**：
	// 那道「新增第六种 period_type 逼人明确表态」的防线由
	// TestEveryPeriodTypeHasAnExplicitSupportDecision 使用，函数没了就无处可问。
	require.NoError(t, checkPeriodTypeSupported("monthly", "2026年6月金融统计数据报告"),
		"monthly 分支已删，这里必须放行")

	// parseTitle 这一层始终不受影响：它只做标题→期次/种类的映射
	period, pt, kind, err := parseTitle("2026年6月金融统计数据报告")
	require.NoError(t, err)
	assert.Equal(t, "2026-06", period)
	assert.Equal(t, "monthly", pt)
	assert.Equal(t, kindFinance, kind)
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

			period, pt, _, err := parseTitle(title)
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
		"monthly": {true, "M1c-3a 的 TASK-007 已接线：TASK-001 给了「前N个月」十项与「1月份」特例、TASK-004 让切分认 4/5/7 节月报、TASK-006 让抽取侧按 extractor 定板块适用性；四份真实月报端到端跑通"},
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

// —— M1c-3a 的 TASK-007：三路分派的端到端表 ——
//
// Context Checkpoint: done_criteria → test mapping（M1c-3a 的 TASK-007）
// functional[0]     parseTitle 认三种报告并返回 kind      → TestParseTitle（表里四条社融用例）
// functional[0]     不认识的标题报错且列出三种形态        → TestParseTitleRejectsUnknownShape
// functional[1]     Parse 按 kind 三路分派                → TestParseDispatchesByKindEndToEnd
// functional[2]     删 monthly 分支、保留穷举 switch      → TestParseAcceptsMonthlyReports、
//                                                          TestEveryPeriodTypeHasAnExplicitSupportDecision
// boundary[0]       逐格断言 extractor 与字段数            → TestParseDispatchesByKindEndToEnd
// boundary[1]       *_ytd 取累计句不取当月句               → TestParseTakesPeriodToDateNotCurrentMonth
// boundary[2]       1 月报特例 loan_flow_ytd 非零且等于实值 → TestParseTakesPeriodToDateNotCurrentMonth

// 🔴 端到端逐格断言 `extractor` 与**字段数**，走完整 `Parse`（不是直调抽取函数）。
//
// 期望值**全部按实跑填**：需求文档把两份月报的 extractor 写成 `rule@v2`/54，
// 实测是 `rule-monthly@v1`/25。这一格写错会让测试**绿着而断言的是错的期望**。
//
// 断 extractor **和** 字段数两样：只断 extractor 的话，一个走对了路径却少抽一半字段的
// 实现照样绿；只断字段数的话，两条路径恰好字段数相同时就分不出走的是哪条。
func TestParseDispatchesByKindEndToEnd(t *testing.T) {
	for _, tc := range []struct {
		file       string
		period     string
		periodType string
		extractor  string
		fields     int
	}{
		// —— kindFinance：切分 + detectExtractor 探测版式 ——
		{"pboc-2025-08-monthly.html", "2025-08", "monthly", extractorMonthlyV1, 25},
		{"pboc-2026-07-monthly.html", "2026-07", "monthly", extractorMonthlyV2, 52},
		{"pboc-2025-01-monthly.html", "2025-01", "monthly", extractorMonthlyV1, 25},
		{"pboc-2025-03-monthly.html", "2025-03", "monthly", extractorV1, 27},
		{"pboc-2020-q1q3.html", "2020-09", "q1_q3", extractorV1, 27},

		// —— 社融两种：kind 直接决定 extractor，不经探测 ——
		//
		// 这两行是三路分派的第二、三条路。若实现漏了分派、对它们照样走
		// splitSections + detectExtractor，会命中 missingCoreSections 而报
		// 「新版式/别的报告/抓取被截断」——一句方向完全错的错误信息。
		{"pboc-2025-08-tsf-stock.html", "2025-08", "monthly", extractorTSFStock, 18},
		{"pboc-2025-08-tsf-flow.html", "2025-08", "monthly", extractorTSFFlow, 9},
	} {
		t.Run(tc.file, func(t *testing.T) {
			obs, err := Parse(readTestdata(t, tc.file))
			require.NoError(t, err)
			assert.Equal(t, tc.period, obs.Meta.Period)
			assert.Equal(t, tc.periodType, obs.Meta.PeriodType)
			assert.Equal(t, tc.extractor, obs.Meta.Extractor,
				"extractor 错 = 走错了分派分支，或版式探测判错")
			assert.Len(t, obs.Values, tc.fields,
				"字段数错 = 路径对但抽取不全；只断 extractor 挡不住这种")
		})
	}
}

// 🔴 `*_ytd` 必须取**累计句**的值，不是当月句。
//
// 2025-08 月报正文同时含两句：
//
//	前八个月人民币贷款增加13.46万亿元   ← 要这个（134600）
//	8月份人民币贷款增加…               ← 不是这个
//
// **为什么必须逐值断言**：当月值同样是合法量级、同样在白名单内，只是口径错——
// `magnitude_sanity` 是空表，拦不住。断「有值」或「非零」都放得过去。
func TestParseTakesPeriodToDateNotCurrentMonth(t *testing.T) {
	t.Run("2025-08：累计句 vs 当月句", func(t *testing.T) {
		obs, err := Parse(readTestdata(t, "pboc-2025-08-monthly.html"))
		require.NoError(t, err)
		assert.InDelta(t, 134600.0, obs.Values[FieldLoanFlowYTD], 1.0,
			"取到当月值而非「前八个月」那句的话，量级同样合法、下游无人拦得住")
		assert.InDelta(t, 331.98, obs.Values[FieldM2], 1e-9)
	})

	// 1 月报特例：`1月份` 既是当月也是累计（1 月的累计就是当月），
	// 它是 M1c-3a 的 TASK-001 那条特例在端到端的唯一体现。
	//
	// ⚠️ **不能只断 NotZero**：一个把当月句错当累计句的实现在这里同样非零。
	// 实读原文「1月份人民币贷款增加5.13万亿元」⇒ 51300。
	t.Run("2025-01：1 月报特例", func(t *testing.T) {
		obs, err := Parse(readTestdata(t, "pboc-2025-01-monthly.html"))
		require.NoError(t, err)
		require.NotZero(t, obs.Values[FieldLoanFlowYTD],
			"`1月份` 不在 cumulativePeriods 时这里会是 0（整篇命中 0）")
		assert.InDelta(t, 51300.0, obs.Values[FieldLoanFlowYTD], 1.0,
			"只断非零挡不住「把当月句错当累计句」——1 月这两者恰好同值，"+
				"所以真正区分它们的是别的月份，这里钉住具体值是为了防实现改坏后仍非零")
	})
}

// 🔴 只有当月数、没有累计数的月报 —— `Parse` **必须响亮报错**（M1c-3a 的 TASK-007）。
//
// # 这一格推翻了 DoD boundary[0] 原写的期望
//
// DoD 原写 `pboc-2020-04-monthly.html` →「`rule-monthly@v1` / 25 字段」。实测**报错**，
// 而报错才是正确行为。裁决人 team-lead，依据是它在主仓库独立复核的词频统计。
//
// **机制**（不是结论）：这份报告正文里**没有任何累计句**——`前四个月` / `1-4月` / `累计`
// 各出现 **0** 次，四条小标题全是当月（「三、4月份人民币存款增加1.27万亿元」）。
// 于是 `selectRMBCumulativeFlow` 找到一条候选（`4月份/人民币`）却**拒绝采用**：
// 把当月值装进 `*_ytd` 正是 M1c-3a 的 TASK-009 那道口径守卫要拦的事。
//
// ⚠️ **不是个例**：用 `Parse` 真跑全部 55 篇月报，通过 25 / 失败 30，
// 其中 **22 篇**正是这一条原因。2020-04 是它们的代表，不是异常值。
//
// ⚠️ 「让这类月报也能解析（`*_ytd` 记为缺失而非报错）」是一次**真实的设计变更**，
// 涉及 `extract.go` / `required.go`，不在本任务 `writes` 内 —— 留给 TASK-010 定夺，
// **本任务不为它改自己的交付**。
//
// # 断言为什么要交叉写
//
// 只断「有 error」的话，实现换个理由失败（版式认不出、板块缺失、标题不认）同样绿，
// 而那三种的排障方向完全不同。所以既钉**正向**（错误必须指向「期内合计取不到」，
// 且带上那条被拒的当月候选），也钉**反向**（不得是版式/板块/标题那三类无关错误）。
func TestParseRefusesMonthlyWithoutPeriodToDateSentence(t *testing.T) {
	raw := readTestdata(t, "pboc-2020-04-monthly.html")

	// ① 版式探测这一步是**对的** —— DoD 原期望里仍然成立的那半，一并钉住。
	//    错在更后面：探测出 rule-monthly@v1 之后，抽取层才发现没有累计句。
	secs := splitSections(stripHTML(raw))
	require.Len(t, secs, 4, "这份是 4 节月报")
	ex, err := detectExtractor(secs, "monthly")
	require.NoError(t, err, "版式探测本身不该失败")
	assert.Equal(t, extractorMonthlyV1, ex, "4 节月报仍应被判成 rule-monthly@v1")

	// ② 端到端必须报错，且**不产出半份结果**
	obs, err := Parse(raw)
	require.Error(t, err, "没有累计数可抽时必须响亮失败——静默产出会让 *_ytd 装上当月值")
	assert.Empty(t, obs.Values, "报错时不得同时返回半份 Values")

	// ③ 正向：错误必须指向真实原因
	assert.Contains(t, err.Error(), "期内合计", "错误要指出取不到的是「期内合计」这个口径")
	assert.Contains(t, err.Error(), "4月份",
		"要带上那条被拒的当月候选——排障的人据此才知道「不是没句子，是句子口径不对」")

	// ④ 反向：不得是另外三类无关错误
	for _, unrelated := range []string{
		"unrecognized report title", // 标题层
		"missing core section",      // 板块层
		"unknown extractor",         // 版式层
	} {
		assert.NotContainsf(t, err.Error(), unrelated,
			"换个理由失败也会让 ③ 之外的断言绿——本条钉住失败原因没有漂移到 %q", unrelated)
	}
}

// —— M1c-3a 的 TASK-008：本迭代成果在 CI 里的唯一守卫 ——
//
// Context Checkpoint: done_criteria → test mapping（M1c-3a 的 TASK-008）
// functional[1]  六种 extractor 逐格断言 Extractor 与字段数 → TestParseCoversAllKinds
// functional[1]  至少一格阴性（B 类月报必须报错）          → TestParseCoversAllKinds/阴性
//
// 🔴 **为什么这条测试重要**：真跑验收依赖 `data/hestia-backfill-2026-08-14`（15MB 产物目录），
// 那个目录**不在仓库里**，CI 跑不到。⇒ 本迭代九个任务的成果，在 CI 里**只有这一条测试守着**。
// 它红了就意味着某个 extractor 的端到端路径断了，而真跑验收要等到下一次有人手工执行。
func TestParseCoversAllKinds(t *testing.T) {
	// 期望值**全部按真跑实测填**。
	//
	// ⚠️ 需求文档把两份月报的 extractor 写成 `rule@v2` / 54 字段，**实测是
	// `rule-monthly@v1` / 25** —— 照抄它会让这张表绿着而断言的是错的期望。
	cases := []struct {
		file      string
		period    string
		extractor string
		fields    int
	}{
		// rule@v1：6 节（核心四 + 外汇 + 跨境），27 字段
		{"pboc-2019-annual.html", "2019-12", extractorV1, 27},
		{"pboc-2020-06-h1.html", "2020-06", extractorV1, 27},
		{"pboc-2020-q1q3.html", "2020-09", extractorV1, 27},
		{"pboc-2025-03-monthly.html", "2025-03", extractorV1, 27},

		// rule@v2：8 节（多社融两节），54 字段
		{"pboc-2025-12-annual.html", "2025-12", extractorV2, 54},
		{"pboc-2026-03-q1.html", "2026-03", extractorV2, 54},
		{"pboc-2025-09-q3.html", "2025-09", extractorV2, 54},

		// rule-monthly@v1：4/5 节月报，25 字段
		{"pboc-2025-08-monthly.html", "2025-08", extractorMonthlyV1, 25},
		{"pboc-2025-01-monthly.html", "2025-01", extractorMonthlyV1, 25}, // 1 月特例走通用路径

		// rule-monthly@v2：7 节月报（含社融两节），52 字段
		{"pboc-2026-07-monthly.html", "2026-07", extractorMonthlyV2, 52},

		// 社融两种独立报告：kind 直接决定 extractor，不经 detectExtractor
		{"pboc-2025-08-tsf-stock.html", "2025-08", extractorTSFStock, 18},
		{"pboc-2025-08-tsf-flow.html", "2025-08", extractorTSFFlow, 9},
	}

	covered := map[string]bool{}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			obs, err := Parse(readTestdata(t, tc.file))
			require.NoError(t, err)
			assert.Equal(t, tc.period, obs.Meta.Period)
			assert.Equal(t, tc.extractor, obs.Meta.Extractor,
				"extractor 错 = 分派或版式探测判错了")
			assert.Len(t, obs.Values, tc.fields,
				"字段数错 = 路径对但抽取不全；只断 extractor 挡不住这种")
		})
		covered[tc.extractor] = true
	}

	// 🔴 **白名单逐项表态**：`validExtractors` 里每一个都必须在上表里有真实样本，
	// 除非它在下面这张豁免表里写明了理由。
	//
	// 判据是「**默认要求覆盖**」而不是「列出几个要测的」：新增第八个 extractor 时，
	// 默认放行的写法会让它**没有任何端到端样本**却无人察觉，而本条会红并逼人表态。
	// 与 TestEveryPeriodTypeHasAnExplicitSupportDecision 同族。
	exempt := map[string]string{
		"llm-fallback@v1": "M1c-4 才实现；取值域先行是刻意的（见 types.go 的 validExtractors 注释），" +
			"实现之前不可能有真实样本",

		// M1c-3b 的 TASK-002。注意与上面那条的区别：llm-fallback@v1 是**暂时**没有样本
		// （M1c-4 实现后就该补一格并把这条删掉），merged@v1 是**永远**不会有。
		"merged@v1": "构造上不可能有真实 HTML 样本：它不是从 HTML 解析出来的，是同一 " +
			"(period, period_type, published_at) 的多篇观测在入库前**装配**出来的，" +
			"Parse 永远不会返回它。它的端到端守卫在合并那一层（M1c-3b 的 TASK-011），" +
			"不在本表",
	}
	for _, ex := range validExtractors {
		if why, ok := exempt[ex]; ok {
			assert.NotEmptyf(t, why, "%q 的豁免必须写明理由", ex)
			continue
		}
		assert.Truef(t, covered[ex],
			"extractor %q 没有任何真实样本覆盖 —— 要么补一格，要么写进 exempt 并说明理由。"+
				"CI 里没有 15MB 产物目录，这张表是它唯一的端到端守卫", ex)
	}

	// 🔴 至少一格**阴性**：全是阳性用例的表证明不了守卫在工作。
	//
	// 两格覆盖两种**不同**的拒绝理由——它们的后续动作完全不同，合并成一格就分不出来了。
	t.Run("阴性/B类口径混装必须报错", func(t *testing.T) {
		// 2023-08：合计句是累计（前八个月）而分部门段是当月（8月份）——同一篇之内分段口径不同。
		// 这是 M1c-3a 的 TASK-009 那道口径守卫，端到端仍必须生效。
		obs, err := Parse(readTestdata(t, "pboc-2023-08-monthly.html"))
		require.Error(t, err, "把当月分部门值装进 *_ytd 必须被拒")
		assert.Empty(t, obs.Values, "报错时不得返回半份结果")
		assert.Contains(t, err.Error(), "不是累计口径",
			"失败原因必须是口径守卫本身；换个理由失败说明中间有东西变了")
	})

	t.Run("阴性/C类无累计数据必须报错", func(t *testing.T) {
		// 2020-04：正文里**没有任何累计句**（小标题全是当月），*_ytd 无源可抽。
		// 与 B 类的区别：B 类有累计数据只是分段口径不一致，C 类是数据根本不存在。
		obs, err := Parse(readTestdata(t, "pboc-2020-04-monthly.html"))
		require.Error(t, err, "没有累计数据可抽时必须响亮失败")
		assert.Empty(t, obs.Values)
		assert.Contains(t, err.Error(), "期内合计",
			"C 类的失败点在「取不到期内合计」，与 B 类的口径守卫不是同一条")
		assert.NotContains(t, err.Error(), "不是累计口径",
			"两类的错误必须可区分——合并成同一句会让 calibrate 的分流失去依据")
	})
}
