package hestia

// Context Checkpoint: done_criteria → test mapping (TASK-005 extract)
// functional[0]  2025 抽满 54 字段且**每个值等于 golden**  → TestExtractFieldsOnV2Sample
// functional[0]  2020 抽出 27 字段且值等于 golden          → TestExtractFieldsOnV1Sample
// functional[0]  反向：抽出而不在 golden 里的字段一律报错  → 上两条内的 reverse 断言
// functional[3]  rate_ibo 与 rate_repo 取值互不相等、各自来自自己的锚定句
//                                                        → TestExtractRateFieldsComeFromOwnAnchors
// boundary[0]    只留本外币句、删掉人民币句 ⇒ 报错而非取到本外币值
//                                                        → TestBalanceRefusesWhenRMBSentenceMissing
// boundary[0]    某期出现两句人民币余额 ⇒ 报错而非最左优先静默选一个
//                                                        → TestBalanceRefusesAmbiguousRMBSentences
// boundary[0]①   期次孪生句：顺序对调后仍取期内合计值      → TestLoanFlowPrefersCumulativeRegardlessOfOrder
// boundary[0]②   币种：外币孪生句不得被当成人民币口径      → TestBalanceIgnoresForeignCurrencyTwin
// boundary[*]    干扰数字：M1 同比取 6.5 非 0.3/2.1        → TestExtractIgnoresAdjacentDistractors
// boundary[*]    v1/v2 措辞差异同一锚点命中               → TestExtractHandlesBothLoanWordings
// error_handling[0] 同字段被赋值两次 → 报错且含字段名      → TestCollectorRejectsDuplicateAssignment
// error_handling[1] 任何模板未命中一律 error，不静默跳过   → TestExtractFailsLoudlyOnMissingSentence、
//                                                          TestExtractFieldsFailsOnMissingSection
// non_functional[1] 每个板块关键词的标题命中数 ≤ 1         → TestSectionKeywordsHitAtMostOneTitle
// functional[*]  贷款作用域把同名子项分派到不同字段        → TestExtractSeparatesLoanScopes

import (
	"math"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectorRejectsDuplicateAssignment(t *testing.T) {
	// 同一字段被两个模板命中，说明作用域切错了。静默取最后一个是最坏的处置——
	// 两个值都合理，事后无从判断哪个对。
	c := newCollector()
	require.NoError(t, c.set(FieldM2, 340.29))
	err := c.set(FieldM2, 213.49)
	require.Error(t, err)
	assert.Contains(t, err.Error(), FieldM2, "错误信息必须指名是哪个字段")
	assert.Contains(t, err.Error(), "already set")
}

func TestExtractSeparatesLoanScopes(t *testing.T) {
	sec := section{
		Title: "五、全年人民币贷款增加16.27万亿元",
		Body: "12月末，本外币贷款余额275.74万亿元，同比增长6.2%。月末人民币贷款余额271.91万亿元，同比增长6.4%。" +
			"全年人民币贷款增加16.27万亿元。分部门看，住户贷款增加4417亿元，" +
			"其中，短期贷款减少8351亿元，中长期贷款增加1.28万亿元；" +
			"企（事）业单位贷款增加15.47万亿元，其中，短期贷款增加4.81万亿元，" +
			"中长期贷款增加8.82万亿元，票据融资增加1.66万亿元；" +
			"非银行业金融机构贷款减少1103亿元。",
	}
	got, err := extractLoanSection(sec)
	require.NoError(t, err)

	// 「短期贷款」「中长期贷款」在两个作用域里各出现一次，必须分派到不同字段
	assert.InDelta(t, -8351.0, got[FieldLoanHHShortYTD], 1e-6)
	assert.InDelta(t, 48100.0, got[FieldLoanCorpShortYTD], 1e-6)
	assert.InDelta(t, 12800.0, got[FieldLoanHHMLTYTD], 1e-6)
	assert.InDelta(t, 88200.0, got[FieldLoanCorpMLTYTD], 1e-6)
	assert.InDelta(t, 16600.0, got[FieldLoanBillYTD], 1e-6)
	assert.InDelta(t, -1103.0, got[FieldLoanNBFIYTD], 1e-6)
	assert.InDelta(t, 154700.0, got[FieldLoanCorpTotalYTD], 1e-6)
	assert.InDelta(t, 271.91, got[FieldLoanBalance], 1e-6)
	assert.InDelta(t, 162700.0, got[FieldLoanFlowYTD], 1e-6)
}

func TestExtractHandlesBothLoanWordings(t *testing.T) {
	// v1 写「住户部门贷款」，v2 写「住户贷款」——同一锚点必须都命中
	v1 := section{Body: "月末人民币贷款余额165.2万亿元，同比增长13.2%。" +
		"上半年人民币贷款增加12.09万亿元。分部门看，住户部门贷款增加3.56万亿元，其中，短期贷款增加7552亿元，" +
		"中长期贷款增加2.8万亿元；企（事）业单位贷款增加8.77万亿元，其中，短期贷款增加2.82万亿元，" +
		"中长期贷款增加4.86万亿元，票据融资增加9697亿元；非银行业金融机构贷款减少2775亿元。"}
	got, err := extractLoanSection(v1)
	require.NoError(t, err)
	assert.InDelta(t, 7552.0, got[FieldLoanHHShortYTD], 1e-6)
	assert.InDelta(t, 28000.0, got[FieldLoanHHMLTYTD], 1e-6)
	assert.InDelta(t, 28200.0, got[FieldLoanCorpShortYTD], 1e-6)
}

func TestExtractIgnoresAdjacentDistractors(t *testing.T) {
	sec := section{
		Title: "一、广义货币增长11.1%",
		Body: "6月末，广义货币(M2)余额213.49万亿元，同比增长11.1%，增速与上月末持平，" +
			"比上年同期高2.6个百分点；狭义货币(M1)余额60.43万亿元，同比增长6.5%，" +
			"增速比上月末低0.3个百分点，比上年同期高2.1个百分点；" +
			"流通中货币(M0)余额7.95万亿元，同比增长9.5%。",
	}
	got, err := extractMoneySection(sec)
	require.NoError(t, err)

	assert.InDelta(t, 11.1, got[FieldM2YoY], 1e-6, "不得取到紧邻的 2.6")
	assert.InDelta(t, 6.5, got[FieldM1YoY], 1e-6, "不得取到紧邻的 0.3 或 2.1")
	assert.InDelta(t, 9.5, got[FieldM0YoY], 1e-6)
}

// —— boundary[0]：本外币 / 外币 / 期次 三类孪生句 ——

// TestBalanceRefusesWhenRMBSentenceMissing 是 reviewer 判定「整份 DoD 里鉴别力
// 最强」的那一条。
//
// 删掉人民币句、只留本外币句：必须**报错**，而不是悄悄取到 336.14——那个数
// 量级相近、格式完全正确，下游任何校验都拦不住。
//
// 本包依赖的机制是「口径限定词是显式捕获组，按值挑并要求唯一」，所以这里
// 命中数为 0 ⇒ 报错。计划书原先依赖的是 `[^外]人民币存款余额` 这个前缀断言，
// 而 reviewer 实测去掉 `[^外]` 依然正确（G8）——那个守卫是「去掉也照样通过」的，
// 本测试对它没有鉴别力，对本包的机制才有。
func TestBalanceRefusesWhenRMBSentenceMissing(t *testing.T) {
	sec := section{Body: "12月末，本外币存款余额336.14万亿元，同比增长9%。" +
		"全年人民币存款增加26.41万亿元。其中，住户存款增加14.64万亿元，" +
		"非金融企业存款增加2.31万亿元，财政性存款增加6579亿元，非银行业金融机构存款增加6.41万亿元。"}

	got, err := extractDepositSection(sec)
	require.Error(t, err, "没有人民币口径的余额句时必须报错，不得退而取本外币值")
	assert.Contains(t, err.Error(), currencyRMB)
	assert.NotContains(t, err.Error(), "336.14", "更不该把本外币的值当成结果带出来")
	assert.Nil(t, got)
}

// TestBalanceRefusesAmbiguousRMBSentences：某期若出现两句人民币余额，最左优先
// 会静默选一个而两个都合理。宁可报错。
func TestBalanceRefusesAmbiguousRMBSentences(t *testing.T) {
	sec := section{Body: "月末人民币存款余额328.64万亿元，同比增长8.7%。" +
		"月末人民币存款余额207.48万亿元，同比增长10.6%。" +
		"全年人民币存款增加26.41万亿元。其中，住户存款增加14.64万亿元，" +
		"非金融企业存款增加2.31万亿元，财政性存款增加6579亿元，非银行业金融机构存款增加6.41万亿元。"}

	_, err := extractDepositSection(sec)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "2", "错误信息应指出命中了几条")
}

// TestBalanceIgnoresForeignCurrencyTwin 覆盖 boundary[0]②。
//
// 两份样本的存/贷板块**末句都是外币孪生句**，且用的是美元单位：
//
//	12月末，外币存款余额1.07万亿美元，同比增长25%。
//
// dev-agent-45 在 T4 实测过：`1.07 万亿美元` 与 `1.07 万亿元` 的 toWanYi()
// **完全相等**，浮点数不带币种信号 ⇒ 只断言最终值抓不住这个失效模式。
// 这里断言的是**选中的那条捕获组**：口径必须是人民币、单位必须是人民币量纲。
func TestBalanceIgnoresForeignCurrencyTwin(t *testing.T) {
	sec := section{Body: "12月末，本外币存款余额336.14万亿元，同比增长9%。" +
		"月末人民币存款余额328.64万亿元，同比增长8.7%。" +
		"全年人民币存款增加26.41万亿元。其中，住户存款增加14.64万亿元，" +
		"非金融企业存款增加2.31万亿元，财政性存款增加6579亿元，非银行业金融机构存款增加6.41万亿元。" +
		"12月末，外币存款余额1.07万亿美元，同比增长25%。全年外币存款增加2135亿美元。"}

	got, err := extractDepositSection(sec)
	require.NoError(t, err)
	assert.InDelta(t, 328.64, got[FieldDepositBalance], 1e-6, "不得取到外币的 1.07 或本外币的 336.14")
	assert.InDelta(t, 264100.0, got[FieldDepositFlowYTD], 1e-6, "不得取到外币增量 2135")

	// 捕获组层面的直接断言：选中的余额句其口径与单位
	m, err := selectRMBBalance(depositBalanceRE, sec.Body, "人民币存款余额")
	require.NoError(t, err)
	assert.Equal(t, currencyRMB, m[1])
	assert.Equal(t, "万亿元", m[3], "人民币口径的余额单位必须是人民币量纲")
}

// TestLoanFlowPrefersCumulativeRegardlessOfOrder 覆盖 boundary[0]①（G3）。
//
// 2020 贷款板块里「上半年人民币贷款增加12.09万亿元」与「6月份，人民币贷款增加
// 1.81万亿元」都能被同一条模板命中。计划书用最左优先**碰巧**选中了对的那句，
// 并给了一个错误的安心结论（说单月句「不含锚点词，自然不进任何作用域」——那对
// 作用域切分成立，对 loanFlowRE 不成立）。
//
// 判据是把两句顺序对调后仍取到期内合计值。
func TestLoanFlowPrefersCumulativeRegardlessOfOrder(t *testing.T) {
	const head = "月末人民币贷款余额165.2万亿元，同比增长13.2%。"
	const tail = "分部门看，住户部门贷款增加3.56万亿元，其中，短期贷款增加7552亿元，" +
		"中长期贷款增加2.8万亿元；企（事）业单位贷款增加8.77万亿元，其中，短期贷款增加2.82万亿元，" +
		"中长期贷款增加4.86万亿元，票据融资增加9697亿元；非银行业金融机构贷款减少2775亿元。"
	const ytd = "上半年人民币贷款增加12.09万亿元，同比多增2.42万亿元。"
	const monthly = "6月份，人民币贷款增加1.81万亿元，同比多增1474亿元。"

	for _, tc := range []struct{ name, body string }{
		{"原文顺序（合计在前）", head + ytd + tail + monthly},
		{"顺序对调（单月在前）", head + monthly + ytd + tail},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := extractLoanSection(section{Body: tc.body})
			require.NoError(t, err)
			assert.InDelta(t, 120900.0, got[FieldLoanFlowYTD], 1e-6,
				"必须取期内合计 12.09 万亿，而不是单月的 1.81 万亿")
		})
	}
}

// —— error_handling[1]：任何模板未命中一律报错 ——

func TestExtractFailsLoudlyOnMissingSentence(t *testing.T) {
	for _, tc := range []struct {
		name string
		fn   func(section) (map[string]float64, error)
		body string
	}{
		{
			"货币板块缺 M1 句",
			extractMoneySection,
			"广义货币(M2)余额340.29万亿元，同比增长8.5%。流通中货币(M0)余额14.13万亿元，同比增长10.2%。",
		},
		{
			"利率板块缺回购句",
			extractRateSection,
			"12月份同业拆借加权平均利率为1.36%，分别比上月和上年同期低0.06个和0.21个百分点。",
		},
		{
			"外汇板块缺汇率句",
			extractFXSection,
			"12月末，国家外汇储备余额为3.36万亿美元。",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.fn(section{Body: tc.body})
			require.Error(t, err, "缺句必须报错——静默跳过会让「解析漏了」被当成「本期本就没有」")
			assert.Nil(t, got)
		})
	}
}

func TestExtractFieldsFailsOnMissingSection(t *testing.T) {
	// v2 期次却找不到社融板块：必须报错，而不是当成 v1 少抽几个字段
	_, err := extractFields([]section{{Title: "三、广义货币增长8.5%", Body: "略"}}, extractorV2)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "社会融资规模")
}

// —— functional[3]：两条利率各自来自自己的锚定句 ——

func TestExtractRateFieldsComeFromOwnAnchors(t *testing.T) {
	// 2025：正文写「质押式回购加权**均**利率」（缺「平」字），且同段还有
	// 「质押式回购日均成交同比增长2.9%」——放松成 `加权.*利率为` 会取到 1.36。
	got, err := extractRateSection(section{Body: "全年银行间人民币市场以拆借、现券和回购方式合计成交2180.31万亿元，" +
		"日均成交8.79万亿元，日均成交同比增长2.1%。其中，同业拆借日均成交同比下降12.1%，" +
		"现券日均成交同比增长2.1%，质押式回购日均成交同比增长2.9%。" +
		"12月份同业拆借加权平均利率为1.36%，分别比上月和上年同期低0.06个和0.21个百分点。" +
		"质押式回购加权均利率为1.4%，分别比上月和上年同期低0.04个和0.25个百分点。"})
	require.NoError(t, err)
	assert.InDelta(t, 1.36, got[FieldRateIBO], 1e-6)
	assert.InDelta(t, 1.4, got[FieldRateRepo], 1e-6)
	assert.NotEqual(t, got[FieldRateIBO], got[FieldRateRepo],
		"两条利率取到同一个值 = 其中一条的锚点被放松到命中了另一句")
}

// —— non_functional[1]：板块关键词的标题命中数 ≤ 1 ——

// TestSectionKeywordsHitAtMostOneTitle 钉住 findSection 真正依赖的那条性质。
//
// T3 唯一存活的变异是「findSection 返回最后一个匹配而非第一个」——它存活不是
// 覆盖缺口，而是**等价变异体**：标题命中唯一时 first/last 行为本就相同。
// 换句话说，「取第一个」当前不可观测，真正让 findSection 正确的是「命中唯一」，
// 而在此之前没有任何断言守护它。
//
// ⚠️ 判据是「≤ 1」而不是「恰为 1」：2020 样本的两个社融关键词命中数是 **0**
// ——那期没有社融板块，这个 0 是正确的「找不到」，T3 的 detectExtractor 正是
// 靠它区分 v1/v2。写成「恰为 1」会用一条断言否掉版本探测机制，而那个红是
// 判据错、不是实现错。不变量是「命中数 ∈ {0,1}」，断言的是**不存在 ≥2**。
func TestSectionKeywordsHitAtMostOneTitle(t *testing.T) {
	for _, sample := range []string{"pboc-2025-12-annual.html", "pboc-2020-06-h1.html"} {
		secs := splitSections(stripHTML(readSample(t, sample)))
		require.NotEmpty(t, secs, "切不出板块，本测试的绿色是假的")

		for _, rule := range sectionRules {
			hits := 0
			for _, s := range secs {
				if strings.Contains(s.Title, rule.keyword) {
					hits++
				}
			}
			assert.LessOrEqualf(t, hits, 1,
				"%s: 关键词 %q 命中了 %d 个标题——findSection 的正确性依赖命中唯一，"+
					"多重命中会让它退回「靠顺序」", sample, rule.keyword, hits)
		}
	}
}

// —— functional[0]：两份真实样本逐字段比对 golden ——

// TestExtractFieldsOnV2Sample 是本任务的主判据。
//
// 原 DoD 只要求「抽出 54 个键」——**一个 54 键全对、值全错的实现能完整通过**。
// 数值比对原本只出现在 T7，而 T7 四条判据全是 manual/review。所以这里同时做
// 正向（每个 golden 项都相等）与反向（抽出的每个键都在 golden 里）两个方向。
func TestExtractFieldsOnV2Sample(t *testing.T) {
	secs := splitSections(stripHTML(readSample(t, "pboc-2025-12-annual.html")))
	got, err := extractFields(secs, extractorV2)
	require.NoError(t, err)

	require.Len(t, got, 54, "rule@v2 期次应抽满 54 个字段")
	assertMatchesGolden(t, got, golden2025)
}

func TestExtractFieldsOnV1Sample(t *testing.T) {
	secs := splitSections(stripHTML(readSample(t, "pboc-2020-06-h1.html")))
	got, err := extractFields(secs, extractorV1)
	require.NoError(t, err)

	require.Len(t, got, 27, "rule@v1 期次只有六板块，恰好 27 个字段")
	assertMatchesGolden(t, got, golden2020)

	// 社融字段必须是「键不存在」，不是 0
	for _, f := range []string{FieldTSFStock, FieldTSFFlowYTD, FieldTSFStockGovtBond} {
		_, ok := got[f]
		assert.Falsef(t, ok, "%s 应当不存在，而不是取零值", f)
	}
}

// assertMatchesGolden 双向比对：golden 的每一项都要抽到且相等，抽到的每一项
// 都要在 golden 里。少任一方向都会放过一整类错误。
func assertMatchesGolden(t *testing.T, got, want map[string]float64) {
	t.Helper()
	require.NotEmpty(t, want, "golden 表为空，本比对毫无意义")

	for field, w := range want {
		g, ok := got[field]
		if !assert.Truef(t, ok, "字段 %s 没被抽到", field) {
			continue
		}
		assert.InDeltaf(t, w, g, 1e-6, "字段 %s", field)
	}
	for field := range got {
		_, ok := want[field]
		assert.Truef(t, ok, "抽出了 golden 里没有的字段 %s", field)
	}
}

// TestExtractorConstantsMatchDetect 把本包的 extractorV1/V2 常量与 sections.go 的
// detectExtractor 返回值绑在一起。
//
// ⚠️ M1c-3a 的 TASK-004 起，sections.go **直接返回这两个常量**，不再是各写一份的
// 字面量 —— 「两处分叉」那类缺陷因此在结构上不可能发生，本测试不再是它的守卫。
// （原注释：字面量分叉会让 extractFields 安静地按 v1 去抽一份 v2 报告，少抽 27 个
// 字段，而「缺失」在 M1b-1 的语义里是「本期模板本就没有」，完全无声。）
//
// 它现在守的是**探测行为本身**：两份真实样本经 splitSections → detectExtractor
// 必须得到各自的 extractor。这一半从来不是恒真的，消融可杀。
func TestExtractorConstantsMatchDetect(t *testing.T) {
	for _, tc := range []struct{ sample, periodType, want string }{
		{"pboc-2025-12-annual.html", "annual", extractorV2},
		{"pboc-2020-06-h1.html", "h1", extractorV1},
	} {
		t.Run(tc.sample, func(t *testing.T) {
			secs := splitSections(stripHTML(readSample(t, tc.sample)))
			got, err := detectExtractor(secs, tc.periodType)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got, "常量与 detectExtractor 的返回值必须一致")
		})
	}
}

// TestExtractFieldsRejectsUnknownExtractor：认不出的版本标识必须响亮失败，
// 而不是退化成「跳过 v2Only 的两节」抽一份少 27 个字段的结果。
//
// ⚠️ 用**真实的 v2 样本**喂进去，而不是 nil。变异测试逼出来的教训：早先这条
// 测试传的是 nil，于是去掉版本校验后它**依旧通过**——因为报错换成了「找不到
// 广义货币板块」，`require.Error` 照样满足。测试是「因为错误的理由」绿的。
// 给足合法板块之后，少了这道校验就会**成功返回 27 个字段**，那才真正暴露危害。
func TestExtractFieldsRejectsUnknownExtractor(t *testing.T) {
	secs := splitSections(stripHTML(readSample(t, "pboc-2025-12-annual.html")))

	got, err := extractFields(secs, "rule@v3")
	require.Error(t, err, "认不出的版本标识必须报错，而不是按已知模板抽一份没见过的报告")
	assert.Contains(t, err.Error(), "unknown extractor", "错误必须指明是版本认不出，而不是别的原因")
	assert.Contains(t, err.Error(), "rule@v3")
	assert.Nil(t, got)
}

// TestLoanScopesRejectOutOfOrderAnchors 钉住作用域顺序校验。
//
// 作用域从锚点起、到**下一个**锚点止，因此边界正确性依赖锚点在原文里按
// 住户→企业→非银的顺序出现。某期若换了顺序，切出来的段会张冠李戴：企业的
// 短期贷款落进住户字段——值完全合理而字段错，是下游最难发现的一类。
func TestLoanScopesRejectOutOfOrderAnchors(t *testing.T) {
	sec := section{Body: "月末人民币贷款余额271.91万亿元，同比增长6.4%。" +
		"全年人民币贷款增加16.27万亿元。分部门看，企（事）业单位贷款增加15.47万亿元，" +
		"其中，短期贷款增加4.81万亿元，中长期贷款增加8.82万亿元，票据融资增加1.66万亿元；" +
		"住户贷款增加4417亿元，其中，短期贷款减少8351亿元，中长期贷款增加1.28万亿元；" +
		"非银行业金融机构贷款减少1103亿元。"}

	_, err := extractLoanSection(sec)
	require.Error(t, err, "锚点顺序与预期不符时必须报错，不能照旧切段")
	assert.Contains(t, err.Error(), "out of expected order")
}

// TestLoanScopeBoundsSubItemsToItsOwnSector 钉住作用域的**右边界**。
//
// 若作用域不到下一个锚点为止而是一直延伸到正文末尾，住户段就会看见企业段的
// 「短期贷款」。在两份真实样本上这不会显形——两个部门都各有一条短期贷款句，
// 最左优先仍取到自己那条。所以这里构造一个住户段**缺**短期贷款的输入：
// 边界正确时应当报「找不到」，边界失效时会静默借用企业段的 4.81 万亿。
func TestLoanScopeBoundsSubItemsToItsOwnSector(t *testing.T) {
	sec := section{Body: "月末人民币贷款余额271.91万亿元，同比增长6.4%。" +
		"全年人民币贷款增加16.27万亿元。分部门看，住户贷款增加4417亿元，" +
		"其中，中长期贷款增加1.28万亿元；" +
		"企（事）业单位贷款增加15.47万亿元，其中，短期贷款增加4.81万亿元，" +
		"中长期贷款增加8.82万亿元，票据融资增加1.66万亿元；" +
		"非银行业金融机构贷款减少1103亿元。"}

	got, err := extractLoanSection(sec)
	require.Error(t, err, "住户段没有短期贷款句时必须报错，不得借用企业段那条")
	assert.Contains(t, err.Error(), "短期贷款")
	assert.Nil(t, got)
}

// —— TASK-005 返工（QA WARNING-1）：mustMatch 的唯一性 ——

// TestMustMatchRequiresUniqueHit 钉住 mustMatch 的第三种结果。
//
// 修复前它是 FindStringSubmatch + 最左优先：命中两次时**静默取第一个**。
// 本文件开头纪律 2 自述「孪生句一律按捕获组挑并要求唯一」，但那条纪律此前
// 只落实在 selectRMBBalance / selectRMBCumulativeFlow 两族，约 30 条清单模板
// 走的仍是最左优先，且**该选择零测试覆盖**——QA 的变异「mustMatch 取最后一个」
// 因此存活（358 PASS / 0 FAIL，与基线相同）。
//
// 危害不是理论的：构造一个合法的存款板块、把单月分部门句排在累计句之前，
// 修复前 err=nil 且 deposit_household_ytd=21000（应 146400）、
// deposit_nbfi_ytd=+500（应 −64100，**符号也翻了**）。
// 「今天不可触发」是排版事实不是契约——2020 样本已含同体例的板块级单月孪生句。
func TestMustMatchRequiresUniqueHit(t *testing.T) {
	re := sectorFlowRE("住户存款")

	t.Run("命中一次即返回", func(t *testing.T) {
		m, err := mustMatch(re, "其中，住户存款增加14.64万亿元。", "存款分部门 住户存款")
		require.NoError(t, err)
		assert.Equal(t, []string{"增加", "14.64", "万亿元"}, m[1:])
	})

	t.Run("零命中报错且指名模板", func(t *testing.T) {
		_, err := mustMatch(re, "其中，财政性存款增加6579亿元。", "存款分部门 住户存款")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
		assert.Contains(t, err.Error(), "住户存款", "错误必须指名是哪条模板")
	})

	t.Run("多命中必须报错而不是取最左", func(t *testing.T) {
		// 累计句与单月句同体例并存——真实报告里已有这种排版
		const twin = "6月份，住户存款增加2.1万亿元。上半年住户存款增加8.33万亿元。"
		got, err := mustMatch(re, twin, "存款分部门 住户存款")
		require.Error(t, err, "两句同形时必须报错，不得静默取最左那句")
		assert.Nil(t, got)
		assert.Contains(t, err.Error(), "2", "错误信息必须给出命中数")
		assert.Contains(t, err.Error(), "住户存款", "错误必须指名是哪条模板")
		assert.NotContains(t, err.Error(), "2.1", "不该把候选值当结果带出来")
	})
}

// TestListTemplatesHitExactlyOnceOnRealSamples 是常驻守卫：两份真实样本上，
// 每条清单模板在它所属板块内的命中数**恰为 1**。
//
// ⚠️ 判据方向与 T3 的 TestSectionKeywordsHitAtMostOneTitle **不同**：那边是
// `≤ 1`（板块关键词在某些期次正确地找不到，如 2020 无社融）；这边是**恰为 1**
// ——每条清单模板都必须命中，未命中本就该由 mustMatch 报错。
//
// 枚举**从清单表本身派生**，不另写一份模板列表：往 tsfStockItems / tsfFlowItems /
// moneyItems / depositItems / loanScopes 里加一项，本测试自动覆盖它。硬编码的
// 只有「每族属于哪个板块关键词」这 7 个常量，与 extract.go 的 sectionRules 同源。
func TestListTemplatesHitExactlyOnceOnRealSamples(t *testing.T) {
	for _, sample := range []struct {
		file string
		v2   bool
	}{
		{"pboc-2025-12-annual.html", true},
		{"pboc-2020-06-h1.html", false},
	} {
		t.Run(sample.file, func(t *testing.T) {
			secs := splitSections(stripHTML(readSample(t, sample.file)))
			require.NotEmpty(t, secs, "切不出板块，本检查毫无意义")

			bodyOf := func(kw string) string {
				sec, ok := findSection(secs, kw)
				require.Truef(t, ok, "找不到板块 %q", kw)
				return sec.Body
			}

			checked := 0
			once := func(what, text string, re *regexp.Regexp) {
				n := len(re.FindAllStringSubmatch(text, -1))
				assert.Equalf(t, 1, n, "%s: 命中 %d 次，应恰为 1（pattern %s）", what, n, re)
				checked++
			}

			if sample.v2 {
				stock := bodyOf(tsfSectionKeyword)
				once("社融存量总量", stock, tsfStockTotalRE)
				for _, it := range tsfStockItems {
					once("社融存量分项 "+it.name, stock, tsfStockRE(it.name))
				}
				flow := bodyOf("社会融资规模增量")
				// ⚠️ **`tsfFlowTotalRE` 自 M1c-3a 的 TASK-011 起没有生产调用方了**：
				// 板块路径改走整篇路径的核（作用域切分 + 按口径挑总量句），用的是更宽的
				// `tsfFlowArticleTotalRE`，窄模板被它完全覆盖。这一行因此**只是在核对真实
				// 样本的句式**，不再间接测到任何生产路径 —— 记在这里免得后人把它当成
				// 「生产模板有守卫」的证据。profiles.go 解冻时应删掉那条模板（连同本行）。
				once("社融增量总量（模板已无生产调用方，见 extractTSFFlowSection）", flow, tsfFlowTotalRE)
				for _, it := range tsfFlowItems {
					once("社融增量分项 "+it.name, flow, tsfFlowRE(it.name))
				}
			}

			money := bodyOf("广义货币")
			for _, it := range moneyItems {
				once("货币 "+it.code, money, moneyRE(it.name, it.code))
			}

			deposit := bodyOf("人民币存款")
			for _, it := range depositItems {
				once("存款分部门 "+it.name, deposit, sectorFlowRE(it.name))
			}

			// 贷款子项按作用域切段后再数——与 extractLoanSection 的口径一致
			loan := bodyOf("人民币贷款")
			spans, err := loanScopeSpans(loan)
			require.NoError(t, err)
			for i, sp := range spans {
				end := len(loan)
				if i+1 < len(spans) {
					end = spans[i+1].start
				}
				scopeText := loan[sp.start:end]
				if sp.scope.totalField != "" {
					once("作用域合计 "+sp.scope.anchorRE.String(), scopeText, scopeTotalRE(sp.scope))
				}
				for _, it := range sp.scope.items {
					once("作用域子项 "+it.name, scopeText, sectorFlowRE(it.name))
				}
			}

			rate := bodyOf("加权平均利率")
			once("同业拆借利率", rate, rateIBORE)
			once("质押式回购利率", rate, rateRepoRE)

			fx := bodyOf("国家外汇储备")
			once("国家外汇储备", fx, fxReserveRE)
			once("人民币汇率", fx, fxRateRE)

			// 自证：数到 0 条模板时上面全部断言都平凡通过
			want := len(moneyItems) + len(depositItems) + 4
			if sample.v2 {
				want += 1 + len(tsfStockItems) + 1 + len(tsfFlowItems)
			}
			for _, sc := range loanScopes {
				if sc.totalField != "" {
					want++
				}
				want += len(sc.items)
			}
			assert.Equalf(t, want, checked,
				"实际检查了 %d 条模板、清单表里有 %d 条——枚举与表脱节了", checked, want)
		})
	}
}

// —— M1c-3a 的 TASK-002：社融独立报告的整篇抽取 ——
//
// Context Checkpoint: done_criteria → test mapping (M1c-3a 的 TASK-002)
// functional[1]     两个包装函数存在且整篇当一节可跑通
//                                              → TestExtractTSFStockArticleOnSnapshot、
//                                                TestExtractTSFFlowArticleOnSnapshot
// functional[2]     存量 18 字段 / 增量 9 字段，**逐字段数值**双向比对
//                                              → 同上两条（tsfStockArticle2025_08 / tsfFlowArticle2025_08）
// boundary[0]       不抽到「从结构看」段的占比值（钉住已正确的性质）
//                                              → TestTSFStockArticleTakesBalanceNotStructureShare
// boundary[1]       四种方向词 / 同句万亿元与亿元混用 / 负值带符号
//                                              → TestTSFFlowArticleKeepsDirectionSign
// boundary[2]       总量句四类形态（Leader 裁决 A：第 3 类报错）
//                                              → TestTSFFlowArticleTotalSentenceForms
// boundary[2]       口径混装：累计总量 + 当月分项必须响亮失败
//                                              → TestTSFFlowArticleRefusesCaliberMix
// error_handling[0] 缺分项报错且错误信息含分项名
//                                              → TestTSFArticleNamesTheMissingItem
// non_functional[0] 新模板不受 allTemplateRegexps 覆盖，本文件自补贪婪捕获检查
//                                              → TestExtractGoArticleTemplatesHaveNoGreedyCapture

// tsfStockArticle2025_08 逐条抄自 testdata/pboc-2025-08-tsf-stock.html 的总量段原句，
// **不由解析器生成**——那样就是拿实现验证实现。原句：
//
//	初步统计，2025年8月末社会融资规模存量为433.66万亿元，同比增长8.8%。其中，
//	对实体经济发放的人民币贷款余额为265.42万亿元，同比增长6.6%；……
//
// 余额归一到万亿元（原文即万亿元），同比是带符号的百分数（「下降」为负）。
var tsfStockArticle2025_08 = map[string]float64{
	FieldTSFStock:    433.66, // 社会融资规模存量为433.66万亿元
	FieldTSFStockYoY: 8.8,    // 同比增长8.8%

	FieldTSFStockRMBLoan:    265.42, // 对实体经济发放的人民币贷款余额为265.42万亿元
	FieldTSFStockRMBLoanYoY: 6.6,    // 同比增长6.6%
	FieldTSFStockFXLoan:     1.19,   // 对实体经济发放的外币贷款折合人民币余额为1.19万亿元
	FieldTSFStockFXLoanYoY:  -21,    // 同比下降21%
	FieldTSFStockEntrust:    11.15,  // 委托贷款余额为11.15万亿元
	FieldTSFStockEntrustYoY: -0.6,   // 同比下降0.6%
	FieldTSFStockTrust:      4.49,   // 信托贷款余额为4.49万亿元
	FieldTSFStockTrustYoY:   5.5,    // 同比增长5.5%

	FieldTSFStockBankAccept:    2.12,  // 未贴现的银行承兑汇票余额为2.12万亿元
	FieldTSFStockBankAcceptYoY: -4.1,  // 同比下降4.1%
	FieldTSFStockCorpBond:      33.47, // 企业债券余额为33.47万亿元
	FieldTSFStockCorpBondYoY:   3.7,   // 同比增长3.7%
	FieldTSFStockGovtBond:      91.36, // 政府债券余额为91.36万亿元
	FieldTSFStockGovtBondYoY:   21.1,  // 同比增长21.1%
	FieldTSFStockEquity:        11.99, // 非金融企业境内股票余额为11.99万亿元
	FieldTSFStockEquityYoY:     3.4,   // 同比增长3.4%
}

// tsfFlowArticle2025_08 逐条抄自 testdata/pboc-2025-08-tsf-flow.html 的总量段原句。
// 增量字段一律归一到**亿元**，故「12.93万亿元」写作 129300。
//
//	初步统计，2025年前八个月社会融资规模增量累计为26.56万亿元，比上年同期多4.66万亿元。其中，
//	对实体经济发放的人民币贷款增加12.93万亿元，同比少增4851亿元；……
var tsfFlowArticle2025_08 = map[string]float64{
	FieldTSFFlowYTD: 265600, // 社会融资规模增量累计为26.56万亿元

	FieldTSFFlowRMBLoanYTD:    129300, // 对实体经济发放的人民币贷款增加12.93万亿元
	FieldTSFFlowFXLoanYTD:     -816,   // 对实体经济发放的外币贷款折合人民币减少816亿元
	FieldTSFFlowEntrustYTD:    -855,   // 委托贷款减少855亿元
	FieldTSFFlowTrustYTD:      1942,   // 信托贷款增加1942亿元
	FieldTSFFlowBankAcceptYTD: -223,   // 未贴现的银行承兑汇票减少223亿元
	FieldTSFFlowCorpBondYTD:   15600,  // 企业债券净融资1.56万亿元
	FieldTSFFlowGovtBondYTD:   102700, // 政府债券净融资10.27万亿元
	FieldTSFFlowEquityYTD:     2669,   // 非金融企业境内股票融资2669亿元
}

func TestExtractTSFStockArticleOnSnapshot(t *testing.T) {
	got, err := extractTSFStockArticle(stripHTML(readSample(t, "pboc-2025-08-tsf-stock.html")))
	require.NoError(t, err)

	require.Len(t, got, 18, "存量独立报告应抽出总量 2 + 8 分项 ×（余额 + 同比）= 18 个字段")
	assertMatchesGolden(t, got, tsfStockArticle2025_08)
}

func TestExtractTSFFlowArticleOnSnapshot(t *testing.T) {
	got, err := extractTSFFlowArticle(stripHTML(readSample(t, "pboc-2025-08-tsf-flow.html")))
	require.NoError(t, err)

	require.Len(t, got, 9, "增量独立报告应抽出总量 1 + 8 分项 = 9 个字段")
	assertMatchesGolden(t, got, tsfFlowArticle2025_08)
}

// TestTSFStockArticleTakesBalanceNotStructureShare 钉住一个**已经正确**的性质，
// 不是在修 bug：报告第二段「从结构看」里分项名逐字相同而数值是占比。
//
//	（总量段）委托贷款余额为11.15万亿元，同比下降0.6%
//	（结构段）委托贷款余额占比2.6%，同比低0.2个百分点
//
// 抽错得到 2.6——量级差四倍，**而 2.6 是个完全合法的余额**，magnitude_sanity
// 现在是空表（skipped{not_calibrated}），拦不住它。
//
// 现有 tsfStockRE 要求「余额」后紧跟**数值 + 单位**，而结构段是「余额占比2.6%」
// （「占」不是数字、缺单位）；tsfStockTotalRE 要求「存量**为**」而结构段写
// 「占同期社会融资规模存量**的**61.2%」。两条各自挡住一半。
//
// # 消融实测：DoD 指定的那个消融**不能证伪本断言**（M1c-3a 的 TASK-002 实测）
//
// DoD boundary[0] 写的验收方式是「把 tsfStockRE 的单位要求去掉，该断言必须转红；
// 若仍绿，说明这条断言测的不是它自称的那个性质」。照做的结果是**仍绿**——但结论
// 不是「断言测错了」，而是**那个消融没有打开它以为打开的那条路**：
//
//	A4  只去掉 unitPat                       → 本断言 SURVIVED（只有 golden 测试红，
//	                                           且红因是捕获组少了一个、不是取到占比值）
//	A4b 占比中缀 + % 入单位 + 低/高 入方向词  → 红（两句同时命中 ⇒ matched 2 sentences）
//	A4d 8 个分项全部改指结构段                → 红，错误是 unknown unit "%"
//
// 「不抽到占比值」由**四道互相独立**的机制守着，去掉任何一道其余三道仍然拦得住：
//
//	① 「占比」/「占同期社会融资规模存量的」中缀 —— 余额为? 后必须紧跟数字
//	② unitPat 白名单 —— 占比句的「单位」是 %，不在表内
//	③ dirPat 白名单 —— 占比句的方向词是「低/高」，不在表内
//	④ amount.go 的 unknown unit 兜底 —— 正则全放开也进不了结果表（A4d 实测）
//
// ⇒ **没有任何单点变异能让抽取器返回 2.6**。故本测试钉的是**结果**而不是某一道闸：
// 它在 A4b/A4d 下确实转红，但转红路径是 require.NoError（抽取整体失败），不是
// 「entrust 取到了 2.6」。把它当成「守卫②的单元测试」会高估它——它守的是那四道
// 合起来的**净效果**。harness 与全部输出见 discovery。
func TestTSFStockArticleTakesBalanceNotStructureShare(t *testing.T) {
	got, err := extractTSFStockArticle(stripHTML(readSample(t, "pboc-2025-08-tsf-stock.html")))
	require.NoError(t, err)
	// 下面的否定式断言在 got 为空表时会**平凡为真**（缺键读出 0，与任何占比值都不等）。
	// 先钉住表是满的，否则那半个测试的绿色不代表任何东西。
	require.Len(t, got, 18)

	// 逐格钉住「取的是总量段的余额」而不是「结构段的占比」。左值是余额、
	// 右注是同一分项在结构段的占比——两者都是合法数值，只有位置能区分。
	assert.InDelta(t, 11.15, got[FieldTSFStockEntrust], 1e-9)   // 结构段占比 2.6
	assert.InDelta(t, 1.19, got[FieldTSFStockFXLoan], 1e-9)     // 结构段占比 0.3
	assert.InDelta(t, 2.12, got[FieldTSFStockBankAccept], 1e-9) // 结构段占比 0.5
	assert.InDelta(t, 33.47, got[FieldTSFStockCorpBond], 1e-9)  // 结构段占比 7.7
	assert.InDelta(t, 91.36, got[FieldTSFStockGovtBond], 1e-9)  // 结构段占比 21.1
	assert.InDelta(t, 11.99, got[FieldTSFStockEquity], 1e-9)    // 结构段占比 2.8
	// 总量：结构段写「占同期社会融资规模存量的61.2%」，取错会得到 61.2
	assert.InDelta(t, 433.66, got[FieldTSFStock], 1e-9)
	assert.InDelta(t, 265.42, got[FieldTSFStockRMBLoan], 1e-9)

	// 否定式与上面的肯定式**互补，不是重复**：肯定式钉住取到的是哪个值，
	// 否定式钉住结构段那 8 个占比值一个都没进结果表。删掉任一半都会放过一类错误。
	//
	// RMBLoan 与 Trust 两条是 M1c-3a 的 TASK-006 补的：test-m1c3a-v1 在 TASK-002
	// 验证中实跑变异 N5（强制「信托贷款=1」）时本测试**保持绿**，只有 golden 测试红
	// ——那个性质仍被覆盖，但不是被本测试覆盖。补齐后 8 个分项 + 总量全在表内。
	for f, share := range map[string]float64{
		FieldTSFStockRMBLoan: 61.2, FieldTSFStockFXLoan: 0.3,
		FieldTSFStockEntrust: 2.6, FieldTSFStockTrust: 1,
		FieldTSFStockBankAccept: 0.5, FieldTSFStockCorpBond: 7.7,
		FieldTSFStockGovtBond: 21.1, FieldTSFStockEquity: 2.8,
		FieldTSFStock: 61.2,
	} {
		assert.Greaterf(t, math.Abs(got[f]-share), 1e-9,
			"%s 取到了「从结构看」段的占比值 %v", f, share)
	}
}

// —— 增量总量句的四类形态（M1c-3a 的 TASK-002 全量实测：69 篇分布 19/6/19/25）——
//
// 下面四段 body 逐字抄自真实语料的总量段，只删掉与本测试无关的尾注。

// tsfFlowBodyCumulativeOnly：仅「累计为」（19 篇，2025 年 8 月体例）
const tsfFlowBodyCumulativeOnly = "初步统计，2025年前八个月社会融资规模增量累计为26.56万亿元，比上年同期多4.66万亿元。" +
	"其中，对实体经济发放的人民币贷款增加12.93万亿元，同比少增4851亿元；" +
	"对实体经济发放的外币贷款折合人民币减少816亿元，同比少减767亿元；委托贷款减少855亿元，同比多减307亿元；" +
	"信托贷款增加1942亿元，同比少增1614亿元；未贴现的银行承兑汇票减少223亿元，同比少减2566亿元；" +
	"企业债券净融资1.56万亿元，同比少2214亿元；政府债券净融资10.27万亿元，同比多4.63万亿元；" +
	"非金融企业境内股票融资2669亿元，同比多1093亿元。"

// tsfFlowBodyJanuaryBare：仅「为」且是 1 月报（6 篇）。1 月的年初至今累计**就是**当月，
// 故这一句虽无「累计」二字，口径仍是累计。抄自 2025 年 1 月报。
const tsfFlowBodyJanuaryBare = "初步统计，2025年1月社会融资规模增量为7.06万亿元，比上年同期多5833亿元。" +
	"其中，对实体经济发放的人民币贷款增加5.22万亿元，同比多增3793亿元；" +
	"对实体经济发放的外币贷款折合人民币减少392亿元，同比多减1381亿元；委托贷款增加449亿元，同比多增808亿元；" +
	"信托贷款增加623亿元，同比少增109亿元；未贴现的银行承兑汇票增加4653亿元，同比少增983亿元；" +
	"企业债券净融资4454亿元，同比多134亿元；政府债券净融资6933亿元，同比多3986亿元；" +
	"非金融企业境内股票融资473亿元，同比多51亿元。"

// tsfFlowBodyNonJanuaryBare：仅「为」且非 1 月（19 篇）。这句是**当月**值，
// 报告本身不含年初至今累计——抄自 2020 年 10 月报。
const tsfFlowBodyNonJanuaryBare = "初步统计，2020年10月社会融资规模增量为1.42万亿元，比上年同期多5493亿元。" +
	"其中，对实体经济发放的人民币贷款增加6663亿元，同比多增1193亿元；" +
	"对实体经济发放的外币贷款折合人民币减少175亿元，同比多减165亿元；委托贷款减少174亿元，同比少减493亿元；" +
	"信托贷款减少875亿元，同比多减251亿元；未贴现的银行承兑汇票减少1089亿元，同比多减36亿元；" +
	"企业债券净融资2522亿元，同比多490亿元；政府债券净融资4931亿元，同比多3060亿元；" +
	"非金融企业境内股票融资927亿元，同比多747亿元。"

// tsfFlowBodyBothCumulativeFirst：两者都有、**累计段在前**（25 篇里的多数体例）。
// 抄自 2020 年 2 月报——两段各带一整套分项，是本包纪律 2 说的孪生句最完整的形态。
const tsfFlowBodyBothCumulativeFirst = "初步统计，2020年前两个月社会融资规模增量累计为5.92万亿元，比上年同期多2717亿元。" +
	"其中，对实体经济发放的人民币贷款增加4.21万亿元，同比少增1183亿元；" +
	"对实体经济发放的外币贷款折合人民币增加765亿元，同比多增527亿元；委托贷款减少382亿元，同比少减826亿元；" +
	"信托贷款减少109亿元，同比多减417亿元；未贴现的银行承兑汇票减少2558亿元，同比多减3241亿元；" +
	"企业债券净融资7747亿元，同比多2043亿元；政府债券净融资9437亿元，同比多3391亿元；" +
	"非金融企业境内股票融资1058亿元，同比多650亿元。\n\n" +
	"2月当月，社会融资规模增量为8554亿元，比上年同期少1111亿元。" +
	"其中，对实体经济发放的人民币贷款增加7202亿元，同比少增439亿元；" +
	"对实体经济发放的外币贷款折合人民币增加252亿元，同比多增357亿元；委托贷款减少356亿元，同比少减152亿元；" +
	"信托贷款减少540亿元，同比多减503亿元；未贴现的银行承兑汇票减少3961亿元，同比多减858亿元；" +
	"企业债券净融资3860亿元，同比多2985亿元；政府债券净融资1824亿元，同比少2523亿元；" +
	"非金融企业境内股票融资449亿元，同比多330亿元。"

// tsfFlowBodyBothMonthlyFirst：两者都有、**当月段在前而累计句孤悬段末**（2022 年 7/8/10/11 共 4 篇）。
// 抄自 2022 年 10 月报。这一篇是本任务发现的静默错误现场，见 TestTSFFlowArticleRefusesCaliberMix。
const tsfFlowBodyBothMonthlyFirst = "初步统计，2022年10月社会融资规模增量为9079亿元，比上年同期少7097亿元。" +
	"其中，对实体经济发放的人民币贷款增加4431亿元，同比少增3321亿元；" +
	"对实体经济发放的外币贷款折合人民币减少724亿元，同比多减691亿元；委托贷款增加470亿元，同比多增643亿元；" +
	"信托贷款减少61亿元，同比少减1000亿元；未贴现的银行承兑汇票减少2157亿元，同比多减1271亿元；" +
	"企业债券净融资2325亿元，同比多64亿元；政府债券净融资2791亿元，同比少3376亿元；" +
	"非金融企业境内股票融资788亿元，同比少58亿元。1-10月，社会融资规模增量累计为28.7万亿元，比上年同期多2.31万亿元。"

// TestTSFFlowArticleTotalSentenceForms 覆盖增量总量句的四类形态。
//
// 🔴 第 3 类（仅「为」且非 1 月，19 篇）**刻意报错**，不是没做完：那句是当月值，
// 而字段名是 tsf_flow_ytd（年初至今累计），与 deposit_flow_ytd / loan_flow_ytd 同族、
// 下游 calibrate 会拿它跨期比。把当月值填进去，量级看起来完全合理而口径是错的——
// 正是本包反复禁止的失败方式。人类在同构问题（月报分部门口径）上已选「只接安全的，
// 其余响亮失败」，这里执行同一裁决。
//
// 🔴 天真放宽成「社会融资规模增量(?:累计)?为」不可行：第 4/5 类那 25 篇会命中两次，
// mustMatch 直接报 matched 2 sentences——原本成功的 25 篇会被打坏。
func TestTSFFlowArticleTotalSentenceForms(t *testing.T) {
	for _, tc := range []struct {
		name     string
		body     string
		wantErr  string // 非空则要求报错且错误信息含它
		wantMoM  bool   // 只有当月族：断言值落 _mom 且 _ytd 一个都没有
		wantBoth bool   // 两族都完整：18 个字段，两套各就各位
		wantYTD  float64
		// wantRMBLoan 钉住**分项**与总量同口径——只断总量的话，累计总量配当月分项
		// （2022 年 10 月体例）会照样绿。
		wantRMBLoan float64
	}{
		{
			name: "类1 仅累计为", body: tsfFlowBodyCumulativeOnly,
			wantYTD: 265600, wantRMBLoan: 129300,
		},
		{
			// 1 月的累计=当月，故无「累计」二字仍是累计口径
			name: "类2 仅为_1月报", body: tsfFlowBodyJanuaryBare,
			wantYTD: 70600, wantRMBLoan: 52200,
		},
		{
			// 🔴 判据换形式不换内容（M1c-4 的 TASK-005）：原来「响亮失败，不拿 14200 冒充 ytd」，
			// 路由化之后 14200 有了自己的列 ⇒ 写进 _mom，而 **_ytd 一个都不许有**。
			// 那句「量级看起来完全合理而口径是错的」现在由列名承担。
			name: "类3 仅为_非1月_走当月列", body: tsfFlowBodyNonJanuaryBare,
			wantMoM: true, wantYTD: 0, wantRMBLoan: 0,
		},
		{
			// 两族都完整 ⇒ 各写各的列，18 个字段。累计段那套（4.21万亿元=42100）进 _ytd，
			// 当月那套（7202）进 _mom —— 原来「必须取累计段那套」现在成了「两套都要，别混」。
			name: "类4 两者都有_累计段在前", body: tsfFlowBodyBothCumulativeFirst,
			wantYTD: 59200, wantRMBLoan: 42100, wantBoth: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := extractTSFFlowArticle(tc.body)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr,
					"错误信息要说清候选句的期次与口径，否则排障只能回去翻原文")
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			switch {
			case tc.wantMoM:
				require.Len(t, got, 9, "只有当月族 ⇒ 总量 + 8 分项，全在 _mom 列")
				assert.NotContains(t, got, FieldTSFFlowYTD,
					"🔴 当月总量落进 _ytd ⇒ 量级完全合理而口径是错的，下游 calibrate 会拿它跨期比")
				assert.NotContains(t, got, FieldTSFFlowRMBLoanYTD)
				assert.Greater(t, got[FieldTSFFlowMoM], 0.0, "当月总量必须落进 _mom")
			case tc.wantBoth:
				require.Len(t, got, 18, "两族都完整 ⇒ 各 9 个字段")
				assert.InDelta(t, tc.wantYTD, got[FieldTSFFlowYTD], 1e-9)
				assert.InDelta(t, tc.wantRMBLoan, got[FieldTSFFlowRMBLoanYTD], 1e-9)
				assert.InDelta(t, 7202.0, got[FieldTSFFlowRMBLoanMoM], 1e-9,
					"当月那套必须落进 _mom，不能覆盖累计那套")
			default:
				require.Len(t, got, 9)
				assert.InDelta(t, tc.wantYTD, got[FieldTSFFlowYTD], 1e-9)
				assert.InDelta(t, tc.wantRMBLoan, got[FieldTSFFlowRMBLoanYTD], 1e-9)
			}
		})
	}
}

// TestTSFFlowArticleRefusesCaliberMix 钉住本任务发现的**静默错误现场**。
//
// 2022 年 7/8/10/11 四篇的体例是「当月句 + 一整套当月分项……累计句」——累计总量
// 孤悬段末，它后面一个分项都没有。加作用域切分之前，extractTSFFlowSection 对
// 2022 年 10 月 **err=nil** 且抽出：
//
//	tsf_flow_ytd           = 287000  ← 段末「1-10月，…累计为28.7万亿元」
//	tsf_flow_rmb_loan_ytd  =   4431  ← 段首「10月…人民币贷款增加4431亿元」= 当月
//
// 总量是累计、分项是当月，量级差约 30 倍，没有任何闸门拦得住（AD-6 里「住户短期
// 贷款跑进企业字段」的同族）。作用域切分把分项限定在被选中的总量句之后、下一条
// 总量句之前，于是这一篇的累计作用域里零分项 ⇒ 响亮失败。
// 🔴 **判据换形式不换内容**（M1c-4 的 TASK-005 路由化）：原来断言「必须失败」，
// 理由是「累计总量配当月分项 = 口径混装，量级差 ~30 倍且都在合法区间内」。本迭代把
// 「失败」换成「分列」之后，同一个理由要求的是：
//
//	累计总量 → tsf_flow_ytd
//	当月总量 → tsf_flow_mom，当月分项 → *_mom
//	🔴 **tsf_flow_rmb_loan_ytd 一个都不许有** ← 这半条才是原断言的直系继承者
//
// 若哪天 pick 在 caliberCurrent 时返回 ytdField，混装立刻复现，而**只有那条 NotContains
// 会红**——正向断言全部照绿（值都对，只是进错了列）。变异 V3 验的就是它。
func TestTSFFlowArticleRoutesCaliberMixToSeparateColumns(t *testing.T) {
	got, err := extractTSFFlowArticle(tsfFlowBodyBothMonthlyFirst)
	require.NoError(t, err, "两族各自完整时应当分列写出，不再整篇失败")

	// 这一篇的两个值都在原文里，只是分属不同口径（形态②：当月段在前、累计句孤悬段末）
	require.Contains(t, tsfFlowBodyBothMonthlyFirst, "累计为28.7万亿元")
	require.Contains(t, tsfFlowBodyBothMonthlyFirst, "人民币贷款增加4431亿元")

	assert.InDelta(t, 287000.0, got[FieldTSFFlowYTD], 1e-9, "段末累计总量 → _ytd")
	assert.InDelta(t, 9079.0, got[FieldTSFFlowMoM], 1e-9, "段首当月总量 → _mom")
	assert.InDelta(t, 4431.0, got[FieldTSFFlowRMBLoanMoM], 1e-9, "当月分项 → _mom")

	// 🔴 原「拒绝」的直系继承者：当月分项绝不许出现在累计列
	assert.NotContains(t, got, FieldTSFFlowRMBLoanYTD,
		"🔴 4431（10 月当月）出现在 tsf_flow_rmb_loan_ytd ⇒ 与 287000（1-10月累计）"+
			"错位约 30 倍，两个值都在合法区间内，下游没有任何闸门拦得住")

	// 累计族的作用域里一个分项都没有（形态②）⇒ 该族只写总量，这是**正常**的，
	// 不是「抽不到就跳过」：由 setTSFFlowItems 的「0 项放行、部分缺失报错」区分。
	for _, f := range []string{
		FieldTSFFlowFXLoanYTD, FieldTSFFlowEntrustYTD, FieldTSFFlowTrustYTD,
		FieldTSFFlowBankAcceptYTD, FieldTSFFlowCorpBondYTD, FieldTSFFlowGovtBondYTD,
		FieldTSFFlowEquityYTD,
	} {
		assert.NotContainsf(t, got, f, "累计作用域内无分项，%s 不该有值", f)
	}
}

// —— M1c-3a 的 TASK-011 · R1：两条路径必须在同一段正文上给出同一个结论 ——

// 🔴 **作用域切分一度只落在整篇路径上**（QA R1，team-lead 用真实语料独立复现）。
//
// M1c-3a 的 TASK-002 的 discovery 里明写着一条 decision：「extractTSFFlowSection（v2 板块
// 路径）一个字不改……在这里顺手改会越界且无样本支持」。那个理由在当时成立，代价是
// **同一个缺陷原样留在另一条路上**——真实 2023-08 社融增量报告正文实测：
//
//	extractTSFFlowArticle  → err = 社融增量分项 对实体经济发放的人民币贷款 not found  ← 正确
//	extractTSFFlowSection  → err = <nil>
//	     tsf_flow_ytd          = 252100   ← 「2023年前八个月…累计为25.21万亿元」
//	     tsf_flow_rmb_loan_ytd =  13400   ← 「8月份…人民币贷款增加1.34万亿元」= 当月
//	     ⇒ 错位 18.8×，两个值都在合法量级内
//
// ⚠️ **板块路径正是 v2 月报走的那条**（央行 2025-10 起把社融并进月报的 going-forward
// 格式）⇒ 这不是历史遗留，是会持续产生错数据的路径。
//
// 🔴 **本条断言的是关系性属性（两条路径同结论），不是任一条路径的取值。**
// 只断某一条路径的值，另一条悄悄分叉时不会红——而分叉正是这个缺陷的形状
// （与 M1c-3a 的 TASK-006 的 N2、TASK-009 的交叉断言同族：「A 与 B 不能不一致」
// 需要一条会因为它们不一致而红的断言）。
func TestTSFFlowSectionAndArticleAgreeOnSameBody(t *testing.T) {
	v2Flow := func(t *testing.T, sample string) string {
		t.Helper()
		secs := splitSections(stripHTML(readSample(t, sample)))
		sec, ok := findSection(secs, "社会融资规模增量")
		require.Truef(t, ok, "%s 里找不到社融增量板块", sample)
		return sec.Body
	}

	for _, tc := range []struct {
		name string
		body func(*testing.T) string
	}{
		// 真实语料，双口径（累计句 → 当月句 → 分项）—— 缺陷现场本身
		{"真实_2023-08社融增量_双口径", func(t *testing.T) string {
			return stripHTML(readSample(t, "pboc-2023-08-tsf-flow.html"))
		}},
		// 真实语料，单口径 —— boundary 的阴性对照，必须仍然抽得出来
		{"真实_2025-08社融增量_单口径", func(t *testing.T) string {
			return stripHTML(readSample(t, "pboc-2025-08-tsf-flow.html"))
		}},
		// 真实语料，**板块路径的生产输入**：v2 月报里的社融增量节
		{"真实_2026-07月报_社融增量板块", func(t *testing.T) string {
			return v2Flow(t, "pboc-2026-07-monthly.html")
		}},
		{"当月段在前_累计句孤悬段末", func(*testing.T) string { return tsfFlowBodyBothMonthlyFirst }},
		{"累计段在前_当月段在后", func(*testing.T) string { return tsfFlowBodyBothCumulativeFirst }},
		{"仅累计句", func(*testing.T) string { return tsfFlowBodyCumulativeOnly }},
		{"仅为_1月报", func(*testing.T) string { return tsfFlowBodyJanuaryBare }},
		{"仅为_非1月", func(*testing.T) string { return tsfFlowBodyNonJanuaryBare }},
		// M1c-4 的 TASK-005 新增的三种失败/边界体例：**关系性属性对新路径同样成立**。
		// 只在 extractTSFFlowArticle 上测它们，板块路径分叉时本条不会红——而分叉正是
		// R1 那个缺陷的形状。
		{"两族都没有", func(*testing.T) string {
			return "初步统计，2022年10月末社会融资规模存量为341.42万亿元，同比增长9.5%。"
		}},
		{"当月族两条", func(*testing.T) string {
			return "初步统计，2022年10月社会融资规模增量为9079亿元，比上年同期少7097亿元。" +
				"初步统计，2022年9月社会融资规模增量为3.53万亿元，比上年同期多6245亿元。"
		}},
		{"分项抽到一部分", func(*testing.T) string {
			return "初步统计，2022年10月社会融资规模增量为9079亿元，比上年同期少7097亿元。" +
				"其中，对实体经济发放的人民币贷款增加4431亿元；委托贷款增加470亿元。"
		}},
		{"整族缺席_只有总量", func(*testing.T) string {
			return "初步统计，2022年10月社会融资规模增量为9079亿元，比上年同期少7097亿元。"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := tc.body(t)
			gotArticle, errArticle := extractTSFFlowArticle(body)
			gotSection, errSection := extractTSFFlowSection(section{Body: body})

			// ① 成败必须一致。独家杀手：让某一条路径在本体例上单方面成功或失败。
			require.Equalf(t, errArticle == nil, errSection == nil,
				"同一段正文两条路径成败不一致：整篇 err=%v / 板块 err=%v", errArticle, errSection)

			// ② 失败时错误信息必须逐字相同。独家杀手：两条路径各写一份判据
			//    （措辞分叉是实现分叉的第一个可见征兆，而①对它一无所知）。
			if errArticle != nil {
				assert.Equal(t, errArticle.Error(), errSection.Error(),
					"两条路径的失败理由必须是同一条，否则说明判据有两份实现")
			}

			// ③ 成功时取值必须相同。独家杀手：让某一条路径抽出另一套值
			//    （正是缺陷现场的形状：两条都「成功」但值不同）。
			assert.Equal(t, gotArticle, gotSection, "同一段正文抽出的字段必须逐个相同")
		})
	}
}

// TestTSFFlowSectionRoutesCaliberMixOnRealArticle 把缺陷现场钉在**真实正文**上。
//
// ⚠️ 用真实正文而不是构造串是 team-lead 的实测教训：它第一次用自己构造的短语料
// **没能复现**（两条路径都报错，只是错在不同分项上）——**构造语料不完整会掩盖真缺陷**。
//
// 与上面那条互补，别当重复删掉：上面钉的是「两条路径一致」这个**关系**，
// 一个把两条路径都改坏成同样错法的实现会让它绿；这一条钉的是**板块路径本身的结论**。
func TestTSFFlowSectionRoutesCaliberMixOnRealArticle(t *testing.T) {
	body := stripHTML(readSample(t, "pboc-2023-08-tsf-flow.html"))

	// 前提：这一篇确实是双口径体例，两个值都在原文里、分属不同口径。
	// 少了这两条，下面的 require.Error 可能因为**别的**理由绿。
	require.Contains(t, body, "社会融资规模增量累计为25.21万亿元", "累计总量句")
	require.Contains(t, body, "对实体经济发放的人民币贷款增加1.34万亿元", "当月分项句")

	// 🔴 判据换形式不换内容（M1c-4 的 TASK-005）：原来断言「必须失败」，理由是口径混装；
	// 路由化之后由**分列**承担，判据变成「当月分项绝不许落进累计列」。
	got, err := extractTSFFlowSection(section{Body: body})
	require.NoError(t, err, "两族各自完整时应当分列写出，不再整篇失败")

	assert.InDelta(t, 252100.0, got[FieldTSFFlowYTD], 1e-9, "「前八个月…累计为25.21万亿元」→ _ytd")
	assert.InDelta(t, 13400.0, got[FieldTSFFlowRMBLoanMoM], 1e-9, "「8月份…增加1.34万亿元」→ _mom")

	// 🔴 原「拒绝」的直系继承者。缺陷现场的原始形态是
	// tsf_flow_ytd=252100 配 tsf_flow_rmb_loan_ytd=13400，错位 18.8×、两值都在合法量级内。
	assert.NotContains(t, got, FieldTSFFlowRMBLoanYTD,
		"🔴 当月分项 13400 落进累计列 ⇒ 与 252100 错位 18.8×，下游没有任何闸门拦得住")

	// 板块路径与整篇路径必须给出同一结论（R1 的关系性属性，由
	// TestTSFFlowSectionAndArticleAgreeOnSameBody 全面钉住，这里只做同一篇的点对点确认）。
	gotArticle, errArticle := extractTSFFlowArticle(body)
	require.NoError(t, errArticle)
	assert.Equal(t, gotArticle, got, "两条路径在同一段真实正文上必须逐字段相同")
}

// TestTSFFlowArticleKeepsDirectionSign 覆盖 boundary[1]：增量句的方向词比存量丰富
// （增加 / 减少 / 净融资 / 融资），**同一句里万亿元与亿元混用**，且负值必须带符号。
//
// 方向词丢失是静默的——绝对值对、符号错，而两个值都在合法量级内。
func TestTSFFlowArticleKeepsDirectionSign(t *testing.T) {
	got, err := extractTSFFlowArticle(tsfFlowBodyCumulativeOnly)
	require.NoError(t, err)

	for _, tc := range []struct {
		field, dir, unit string
		want             float64
	}{
		// 「增加」+ 万亿元
		{FieldTSFFlowRMBLoanYTD, "增加", "万亿元", 129300},
		// 「减少」+ 亿元 —— 必须是 -816 而不是 816
		{FieldTSFFlowFXLoanYTD, "减少", "亿元", -816},
		{FieldTSFFlowEntrustYTD, "减少", "亿元", -855},
		{FieldTSFFlowBankAcceptYTD, "减少", "亿元", -223},
		// 「增加」+ 亿元
		{FieldTSFFlowTrustYTD, "增加", "亿元", 1942},
		// 「净融资」+ 万亿元 / 亿元混用
		{FieldTSFFlowCorpBondYTD, "净融资", "万亿元", 15600},
		{FieldTSFFlowGovtBondYTD, "净融资", "万亿元", 102700},
		// 「融资」（无「净」）+ 亿元
		{FieldTSFFlowEquityYTD, "融资", "亿元", 2669},
	} {
		t.Run(tc.field, func(t *testing.T) {
			assert.InDelta(t, tc.want, got[tc.field], 1e-9)
			// 符号必须来自方向词，不是来自「负数长这样」的巧合
			if tc.dir == "减少" {
				assert.Negativef(t, got[tc.field], "方向词「减少」必须产出负值")
			} else {
				assert.Positivef(t, got[tc.field], "方向词「%s」必须产出正值", tc.dir)
			}
		})
	}
}

// TestTSFArticleNamesTheMissingItem 覆盖 error_handling[0]：缺任一分项一律报错、
// 不静默补零，且错误信息里含**缺失的分项名**，不是一句笼统的「抽取失败」。
func TestTSFArticleNamesTheMissingItem(t *testing.T) {
	stock := stripHTML(readSample(t, "pboc-2025-08-tsf-stock.html"))
	flow := stripHTML(readSample(t, "pboc-2025-08-tsf-flow.html"))

	for _, tc := range []struct {
		name, from, to, want string
		fn                   func(string) (map[string]float64, error)
		text                 string
	}{
		{
			name: "存量_委托贷款", text: stock, fn: extractTSFStockArticle,
			from: "委托贷款余额为", to: "委托贷款余额约为", want: "委托贷款",
		},
		{
			name: "存量_政府债券", text: stock, fn: extractTSFStockArticle,
			from: "政府债券余额为", to: "政府债券余额约为", want: "政府债券",
		},
		{
			name: "增量_信托贷款", text: flow, fn: extractTSFFlowArticle,
			from: "信托贷款增加", to: "信托贷款约增加", want: "信托贷款",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Contains(t, tc.text, tc.from, "原文里没有这句，本用例改的是空气")
			broken := strings.Replace(tc.text, tc.from, tc.to, 1)
			require.NotEqual(t, tc.text, broken)

			got, err := tc.fn(broken)
			require.Error(t, err, "缺分项必须报错，不能静默补零")
			assert.Nil(t, got)
			assert.Containsf(t, err.Error(), tc.want,
				"错误信息必须指名缺的是哪个分项，否则排障要回去逐条比对原文")
		})
	}
}

// TestExtractGoArticleTemplatesHaveNoGreedyCapture 补一个本任务**新造出来的**缺口。
//
// profiles_test.go 的 TestNoGreedyCaptureInTemplates 只扫 allTemplateRegexps() 与
// profiles.go 的字面量。本任务把 tsfFlowArticleTotalRE 定义在 extract.go（理由见
// 该变量上方注释），于是它落在那条检查的**覆盖范围之外**——检查还在，只是不再
// 覆盖新增的这一条。这里按同一判据补上 extract.go。
func TestExtractGoArticleTemplatesHaveNoGreedyCapture(t *testing.T) {
	greedy := []string{`(.+)`, `(.*)`}

	for _, g := range greedy {
		assert.NotContainsf(t, tsfFlowArticleTotalRE.String(), g,
			"模板 %s 含贪婪捕获 %s", tsfFlowArticleTotalRE, g)
	}

	lits := stringLiteralsOf(t, "extract.go")
	require.NotEmpty(t, lits, "没解析出任何字符串字面量，本测试的绿色是假的")
	for _, lit := range lits {
		for _, g := range greedy {
			assert.NotContainsf(t, lit.text, g, "%s: 字面量 %s 含贪婪捕获 %s", lit.pos, lit.text, g)
		}
	}
}

// TestTSFFlowArticleRefusesTwoCumulativeSentences 钉住选择的**另一半**失败语义：
// 零条累计句报错是显然的，两条同样报错——不替调用方挑一个。
//
// 与 mustMatch / selectUnique 的态度一致：两个值都在合法量级内，最左优先会静默
// 选中其一，事后无从分辨。真实语料里暂无这种体例（69 篇实测累计句恒 ≤ 1 条），
// 但「暂无」不是「不会有」——央行改一次排版就会有，而那时应当响亮失败。
func TestTSFFlowArticleRefusesTwoCumulativeSentences(t *testing.T) {
	// 把 2025 年 8 月的累计句复制一份、只改数值，构造两条累计候选
	twin := tsfFlowBodyCumulativeOnly +
		"\n\n初步统计，2025年前八个月社会融资规模增量累计为26.57万亿元，比上年同期多4.67万亿元。"

	got, err := extractTSFFlowArticle(twin)
	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "（累计口径）matched 2 sentences",
		"必须说清是「两条都算累计」，而不是笼统的抽取失败")
	assert.Contains(t, err.Error(), "refusing to pick one")

	// 对照：只留一条时同一段文本是能抽通的——否则上面的 Error 可能来自别的原因，
	// 这条断言就不再是在测「拒绝二义」了。
	ok, err := extractTSFFlowArticle(tsfFlowBodyCumulativeOnly)
	require.NoError(t, err)
	require.Len(t, ok, 9)
}

// —— M1c-3a 的 TASK-006：extractFields 按 extractor 决定板块适用性（AD-3）——
//
// Context Checkpoint: done_criteria → test mapping (M1c-3a 的 TASK-006)
// functional[0][1]  适用性收敛成 sectionRule.appliesTo 一处
//                                      → TestSectionAppliesToIsTheSingleSourceOfScope
// functional[2]     只接受 4 种走板块路径的 extractor，拒 tsf-stock@v1/tsf-flow@v1 与未知值
//                                      → TestExtractFieldsRejectsNonSectionPathExtractors
// boundary[0]       板块归属与 requiredFields **双向**相等 + 逐字面量锚
//                                      → TestExtractFieldsScopeMatchesRequiredFields
// boundary[1]       rule-monthly@v1 下外汇节是**声明式跳过**（输入**含**外汇节）
//                                      → 同上（wantAbsent 列）+ TestExtractFieldsSkipsVsMissesSections
// boundary[2]       rule-monthly@v2 保留社融两节但仍跳外汇 ⇒ 两个维度独立
//                                      → 同上（rule-monthly@v2 一行）
// error_handling[0] 适用板块缺失仍报错 / 不适用板块缺失才放行（成对）
//                                      → TestExtractFieldsSkipsVsMissesSections

// sectionPathSample 载入 8 节 v2 年报并前置断言它**含全部 7 个板块**。
//
// 这个前置断言是本组测试成立的**前提**，不是装饰：若样本本就缺外汇节，
// 「声明式跳过」与「碰巧 findSection 找不到」两种实现都会绿，
// 整组关于「跳过」的断言零信息量（DoD boundary[1] 点名的假绿）。
func sectionPathSample(t *testing.T) []section {
	t.Helper()
	secs := splitSections(stripHTML(readSample(t, "pboc-2025-12-annual.html")))
	for _, r := range sectionRules {
		_, ok := findSection(secs, r.keyword)
		require.Truef(t, ok,
			"样本缺板块 %q —— 缺了它，本组测试无法区分「声明式跳过」与「碰巧找不到」", r.keyword)
	}
	return secs
}

// TestExtractFieldsScopeMatchesRequiredFields 是 boundary[0] 的机械保证：
// **同一份输入**喂给四种走板块路径的 extractor，抽出的字段集必须与
// requiredFields(该 extractor) **双向**相等。
//
// 两侧是**独立派生**的，这是本断言有意义的前提：
//
//	左侧 = 真跑 extractFields 的实际产出（sectionRules × appliesTo，抽取侧）
//	右侧 = requiredFields 的清单（fieldOrder − 各板块字段，completeness 侧）
//
// 若改成「左侧也用 fieldOrder 减去被跳过板块的字段」，两侧就共用了同一套减法，
// 断言退化成恒真——那正是本包反复警告的「守卫的基准与被守护属性同源」。
//
// 少了这条，「跳过了却仍在必填集里」（completeness 把抽不到的字段记成缺失）
// 与「留着却不在必填集里」（抽到了却没人要）都不会有任何东西转红。
//
// ⚠️ **逐字面量锚不是重复**：集合恒等式抓不到「划反」——M1c-3a 的 TASK-003 实测，
// 三个社融总量字段划反后「18+9==27 且交集为空」全部保持绿。wantPresent /
// wantAbsent 两列钉的是**具体哪个字段在哪一侧**，只有它们抓得到。
//
// ⚠️ tsf-stock@v1 / tsf-flow@v1 **刻意不在本表内**：它们整篇当一节，走
// extractTSFStockArticle / extractTSFFlowArticle，根本不经 extractFields
// （M1c-3a 的 TASK-002 交付）。喂给 extractFields 应当报错，那一格由
// TestExtractFieldsRejectsNonSectionPathExtractors 单独钉。
// 写在这里是因为「沉默地不测」与「想过并决定不测」在代码里长得一模一样。
func TestExtractFieldsScopeMatchesRequiredFields(t *testing.T) {
	secs := sectionPathSample(t)

	for _, tc := range []struct {
		extractor   string
		wantN       int
		wantPresent []string
		wantAbsent  []string
	}{
		{
			extractor: extractorV2, wantN: 54,
			wantPresent: []string{FieldTSFStock, FieldTSFFlowYTD, FieldFXReserve, FieldFXRate, FieldM2},
		},
		{
			// 社融两节不适用；外汇节适用
			extractor: extractorV1, wantN: 27,
			wantPresent: []string{FieldFXReserve, FieldFXRate, FieldM2, FieldLoanBalance},
			wantAbsent:  []string{FieldTSFStock, FieldTSFStockYoY, FieldTSFFlowYTD, FieldTSFStockGovtBond},
		},
		{
			// 🔴 两个维度独立：社融两节**保留**、外汇节**跳过**
			extractor: extractorMonthlyV2, wantN: 52,
			wantPresent: []string{FieldTSFStock, FieldTSFStockYoY, FieldTSFFlowYTD, FieldTSFStockGovtBond, FieldM2},
			wantAbsent:  []string{FieldFXReserve, FieldFXRate},
		},
		{
			// 两个维度同时生效
			extractor: extractorMonthlyV1, wantN: 25,
			wantPresent: []string{FieldM2, FieldLoanBalance, FieldDepositBalance, FieldRateIBO},
			wantAbsent:  []string{FieldTSFStock, FieldTSFFlowYTD, FieldFXReserve, FieldFXRate},
		},
	} {
		t.Run(tc.extractor, func(t *testing.T) {
			got, err := extractFields(secs, tc.extractor)
			require.NoError(t, err)

			keys := make([]string, 0, len(got))
			for f := range got {
				keys = append(keys, f)
			}

			// ⚠️ **顺序与 assert/require 的选择都是刻意的**：逐字面量锚放在最前，
			// 且全组一律用 assert（不用 require）。require 失败会**中止本子测试**，
			// 它后面的断言对任何触发它的消融都不可见——那样消融只能得到「红了」，
			// 得不到「红在哪一条」，而后者才是归因需要的信息。
			for _, f := range tc.wantPresent {
				assert.Containsf(t, got, f, "%s 下 %s 应当被抽到", tc.extractor, f)
			}
			for _, f := range tc.wantAbsent {
				assert.NotContainsf(t, got, f,
					"%s 下 %s 必须**不存在**（键不存在，不是取零值）", tc.extractor, f)
			}

			// 双向相等：ElementsMatch 是互相包含，不是只比数量。
			// 只比数量的话，「抽到 A 缺 B」与「抽到 B 缺 A」无法区分。
			assert.ElementsMatch(t, requiredFields(tc.extractor), keys,
				"抽取侧的板块归属与 completeness 侧的必填集分叉了")

			// 数量单独断一次：它是给人读的锚（25/27/52/54 是 TASK-003 discovery
			// 里写死的四个数），也让 ElementsMatch 的失败更好定位。
			assert.Len(t, got, tc.wantN)
		})
	}
}

// TestExtractFieldsSkipsVsMissesSections 把「跳过」与「缺失」放成一对，覆盖
// error_handling[0]。
//
// 单独测「外汇节缺失时放行」是不够的：把 extractFields 写成「任何板块找不到
// 都放行」同样会绿。必须同时钉住「**适用**板块缺失仍然报错」，两格合起来才
// 说明放行是有条件的。
func TestExtractFieldsSkipsVsMissesSections(t *testing.T) {
	full := sectionPathSample(t)

	drop := func(keyword string) []section {
		out := make([]section, 0, len(full))
		for _, s := range full {
			if strings.Contains(s.Title, keyword) {
				continue
			}
			out = append(out, s)
		}
		require.Lenf(t, out, len(full)-1, "应当恰好删掉一节 %q", keyword)
		return out
	}

	t.Run("不适用板块缺失_放行", func(t *testing.T) {
		// 月报族本就没有外汇节 ⇒ 缺了也应当抽满 25 个字段
		got, err := extractFields(drop(fxSectionKeyword), extractorMonthlyV1)
		require.NoError(t, err)
		assert.Len(t, got, 25)
	})

	t.Run("适用板块缺失_报错", func(t *testing.T) {
		// 同一个 extractor、同样是缺一节，只因这节适用 ⇒ 必须报错
		got, err := extractFields(drop("人民币贷款"), extractorMonthlyV1)
		require.Error(t, err, "适用板块缺失必须报错，「跳过」不能退化成「所有缺失都放行」")
		assert.Contains(t, err.Error(), "人民币贷款", "错误信息要指名缺的是哪个板块")
		assert.Contains(t, err.Error(), extractorMonthlyV1)
		assert.Nil(t, got)
	})

	t.Run("外汇节缺失_在累计期仍报错", func(t *testing.T) {
		// 同一份缺外汇的输入，换成 rule@v1（外汇节适用）就必须报错。
		// 这一格证明放行是**按 extractor** 决定的，不是「外汇节永远可选」。
		got, err := extractFields(drop(fxSectionKeyword), extractorV1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), fxSectionKeyword)
		assert.Nil(t, got)
	})
}

// TestSectionAppliesToIsTheSingleSourceOfScope 钉住 functional[0]：适用性有且只有
// 一处定义，且它对四种走板块路径的 extractor 都给得出答案。
//
// 逐字面量列出期望，而不是把 appliesTo 的实现再写一遍——后者是拿被验对象的
// 尺子量它自己。
func TestSectionAppliesToIsTheSingleSourceOfScope(t *testing.T) {
	// 期望表：板块关键词 → 该板块适用的 extractor 集合（逐字面量，照 description 的表抄）
	want := map[string][]string{
		tsfSectionKeyword: {extractorV2, extractorMonthlyV2},
		"社会融资规模增量":        {extractorV2, extractorMonthlyV2},
		fxSectionKeyword:  {extractorV1, extractorV2},
		"广义货币":            {extractorV1, extractorV2, extractorMonthlyV1, extractorMonthlyV2},
		"人民币存款":           {extractorV1, extractorV2, extractorMonthlyV1, extractorMonthlyV2},
		"人民币贷款":           {extractorV1, extractorV2, extractorMonthlyV1, extractorMonthlyV2},
		"加权平均利率":          {extractorV1, extractorV2, extractorMonthlyV1, extractorMonthlyV2},
	}
	require.Len(t, want, len(sectionRules),
		"期望表与 sectionRules 条数不符——新增板块必须在这里表态，不能默认全适用")

	for _, r := range sectionRules {
		t.Run(r.keyword, func(t *testing.T) {
			applicable, ok := want[r.keyword]
			require.Truef(t, ok, "sectionRules 里的 %q 没在期望表里表态", r.keyword)

			var got []string
			for _, e := range sectionPathExtractors {
				if r.appliesTo(e) {
					got = append(got, e)
				}
			}
			assert.ElementsMatch(t, applicable, got)
		})
	}
}

// TestExtractFieldsRejectsNonSectionPathExtractors 覆盖 functional[2]。
//
// 三类都要拒，且理由不同：
//
//	tsf-stock@v1 / tsf-flow@v1 —— 合法 extractor，但**走错了路**（整篇当一节）
//	rule@v3 / ""               —— 根本不认识
//
// 早先那条 TestExtractFieldsRejectsUnknownExtractor 的教训在这里同样适用：
// 必须喂**真实的合法板块**，否则去掉校验后报错换成「找不到广义货币板块」，
// require.Error 照样满足——测试会因为错误的理由绿。
func TestExtractFieldsRejectsNonSectionPathExtractors(t *testing.T) {
	secs := sectionPathSample(t)

	for _, tc := range []struct{ name, extractor string }{
		{"社融存量独立报告走的是整篇路径", extractorTSFStock},
		{"社融增量独立报告走的是整篇路径", extractorTSFFlow},
		{"根本不认识的版本标识", "rule@v3"},
		{"空串", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := extractFields(secs, tc.extractor)
			require.Error(t, err)
			assert.Nil(t, got)
			assert.Contains(t, err.Error(), tc.extractor, "错误信息要回显收到的是什么")

			// 白名单由**同一份**常量拼出，测试也照那份比对——两处各抄一份必然分叉
			for _, e := range sectionPathExtractors {
				assert.Containsf(t, err.Error(), e,
					"错误信息要列全走板块路径的 extractor，缺 %s", e)
			}
		})
	}

	// 反向对照：白名单里的四个都不该被这道校验挡下。
	// 少了它，把校验写成「一律拒绝」也能让上面四格全绿。
	for _, e := range sectionPathExtractors {
		_, err := extractFields(secs, e)
		assert.NoErrorf(t, err, "%s 走板块路径，不应被 extractor 白名单挡下", e)
	}
}

// —— M1c-3a 的 TASK-009：分部门口径守卫 ——
//
// Context Checkpoint: done_criteria → test mapping (M1c-3a 的 TASK-009)
// functional[0]   分部门段之前最近的期次前缀查 cumulativePeriods，不在表里 ⇒ 报错
//                 两侧（贷款「分部门看」/ 存款「其中，住户存款」）都守
//                                          → TestSectorCaliberGuardsBothSides
// functional[1]   判据是结构性的：同一份正文只改期次前缀就翻转结论，数值一字未动
//                                          → TestSectorCaliberIsStructuralNotNumeric
// functional[2]   既有季报/年报一字不变（回归底线）
//                                          → TestSectorCaliberKeepsCumulativeSamplesIntact
//                                            + 既有 TestExtractFieldsOnV1/V2Sample 保持绿
// boundary[0]     A 类通过 / B 类被拒，两侧都有用例
//                                          → TestSectorCaliberGuardsBothSides
// boundary[2]     C' 类（2022-08）不顺手去救；C 类（2020-04）同样被拒
//                                          → TestSectorCaliberRejectsNonCumulativeSamples
// error_handling  错误信息含实际前缀，且与「板块缺失」「锚点找不到」可区分
//                                          → TestSectorCaliberErrorIsDistinguishable
// non_functional  🟡 N2：两种 extractor 拒绝必须可区分（TASK-006 的 decision 升级成契约）
//                                          → TestExtractFieldsDistinguishesTwoRejectionKinds

// TestSectorCaliberGuardsBothSides 是本任务的核心格：A 类通过、B 类被拒，**两侧各测一次**。
//
// ⚠️ **两侧必须分别直调 extractLoanSection / extractDepositSection**，不能只跑 extractFields：
// sectionRules 里存款节排在贷款节**之前**，所以端到端喂一份 B 类月报时**存款守卫先触发**，
// 贷款侧那道守卫**一次都没执行**。只跑端到端的话，把贷款侧守卫整个删掉测试照样绿。
func TestSectorCaliberGuardsBothSides(t *testing.T) {
	// 🔴 判据换形式不换内容（M1c-4 的 TASK-005）：B 类原来断言「必须报错，不能把当月值
	// 装进 *_ytd」。路由化之后同一个理由要求「当月值必须落进 _mom，且 _ytd 一个都没有」
	// ——那句「不能装进 *_ytd」逐字保留，只是从「靠拒绝达成」变成「靠选列达成」。
	for _, tc := range []struct {
		sample string
		class  string
		mom    bool // true = B 类，值应落 _mom
		prefix string
	}{
		{sample: "pboc-2025-08-monthly.html", class: "A 类：分部门跟在累计句后", mom: false, prefix: "前八个月"},
		{sample: "pboc-2023-08-monthly.html", class: "B 类：分部门跟在当月句后", mom: true, prefix: "8月份"},
	} {
		secs := splitSections(stripHTML(readSample(t, tc.sample)))

		for _, side := range []struct {
			name string
			kw   string
			fn   func(section) (map[string]float64, error)
			// 该侧分部门字段的一对代表，用来钉「进了哪一列」
			ytdField, momField string
		}{
			{"贷款侧", "人民币贷款", extractLoanSection, FieldLoanHHShortYTD, FieldLoanHHShortMoM},
			{"存款侧", "人民币存款", extractDepositSection, FieldDepositHouseholdYTD, FieldDepositHouseholdMoM},
		} {
			t.Run(tc.sample+"/"+side.name, func(t *testing.T) {
				sec, ok := findSection(secs, side.kw)
				require.Truef(t, ok, "样本缺 %q 节，本用例测不到东西", side.kw)

				got, err := side.fn(sec)
				require.NoErrorf(t, err, "%s：%s 应当正常抽取（路由化之后不再整篇拒绝）", tc.class, side.name)

				want, notWant := side.ytdField, side.momField
				if tc.mom {
					want, notWant = side.momField, side.ytdField
				}
				assert.Containsf(t, got, want, "%s：%s 的值应当落进 %s", tc.class, side.name, want)
				assert.NotContainsf(t, got, notWant,
					"🔴 %s：%s 的值落进了 %s —— 前缀是 %q，装错列的两个值都在合法量级内，"+
						"下游没有任何闸门拦得住", tc.class, side.name, notWant, tc.prefix)
			})
		}
	}
}

// TestSectorCaliberIsStructuralNotNumeric 钉住 functional[1]：判据是**结构性**的。
//
// 同一段正文、**数值一字不改**，只把分部门段之前的期次前缀在「累计」与「当月」之间切换，
// 结论必须跟着翻转。若实现写成了数值启发式（分部门合计 vs 累计句偏差比），这两格会
// 得到同一个结论——因为两组输入的数值完全相同。
//
// 这是「守卫判的是性质还是数量」的直接检验：数量在两格里是常量，只有性质变了。
func TestSectorCaliberIsStructuralNotNumeric(t *testing.T) {
	const tail = "，住户贷款增加3922亿元，其中，短期贷款增加2320亿元，中长期贷款增加1602亿元；" +
		"企（事）业单位贷款增加9488亿元，其中，短期贷款减少401亿元，中长期贷款增加6444亿元，" +
		"票据融资增加3472亿元；非银行业金融机构贷款减少358亿元。"
	const head = "8月末，本外币贷款余额237.23万亿元，同比增长10.5%。" +
		"人民币贷款余额232.28万亿元，同比增长11.1%。前八个月人民币贷款增加17.44万亿元。"

	// ⚠️ bridge 用「新增贷款」而不是「人民币贷款」：flowRE 要求币种限定词紧贴「贷款」，
	// 所以这一句**不会**被期内合计模板命中——head 里那句才是唯一的合计句。
	// 若 bridge 也写成「人民币贷款增加…」，累计那格会因两条合计句命中而报
	// 「matched 2 sentences」，两格就不再是最小对了（实撞）。
	for _, tc := range []struct {
		name    string
		bridge  string // 分部门段之前的最后一句，**两格只有期次前缀这一个 token 不同**
		wantMoM bool   // true = 该前缀是当月口径，值应落 _mom
	}{
		{"当月前缀_走当月列", "8月份新增贷款1.36万亿元。分部门看", true},
		{"累计前缀_走累计列", "前八个月新增贷款1.36万亿元。分部门看", false},
		// 1 月报：当月**就是**年初至今累计，`1月份` 已在 cumulativePeriods 里
		// （M1c-3a 的 TASK-001 加），查表自然命中 ⇒ **不需要任何特例分支**。
		// 这一格与上面「当月前缀_拒绝」只差前缀里的月份数字，却结论相反——
		// 若实现写成了「N月份一律拒」这类形状匹配而不是查表，这一格会红。
		//
		// ⚠️ DoD boundary[0] 本来要用真实快照 pboc-2025-01-monthly.html 测这一格，
		// 但那份快照属于 TASK-007 的 writes，交付时尚未合入 master（我不去抓别人
		// writes 里的文件）。故改用合成输入钉同一条性质，并在 discovery 里申报。
		{"1月报当月即累计_走累计列", "1月份新增贷款1.36万亿元。分部门看", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := extractLoanSection(section{Body: head + tc.bridge + tail})
			require.NoError(t, err)
			// 🔴 三格的数值逐字相同（2320），**翻转的只有前缀里那一个 token**，
			// 而结论是「进哪一列」。判据仍是结构性的：查表，不是形状匹配、不是数值启发式。
			if tc.wantMoM {
				assert.InDelta(t, 2320.0, got[FieldLoanHHShortMoM], 1e-9)
				assert.NotContains(t, got, FieldLoanHHShortYTD,
					"🔴 当月值落进 _ytd —— 那正是本判据存在的全部理由")
				return
			}
			assert.InDelta(t, 2320.0, got[FieldLoanHHShortYTD], 1e-9)
			assert.NotContains(t, got, FieldLoanHHShortMoM)
		})
	}
}

// TestSectorCaliberRoutesNonCumulativeSamplesToMoM 是原
// TestSectorCaliberRejectsNonCumulativeSamples 的**路由化版本**（M1c-4 的 TASK-005）。
//
// 🔴 **判据换了形式，守的东西没变。** 原来断言「C / C' 类被拒」，理由是「当月的分部门值
// 装进 *_ytd 会让内部口径不一致，两者都在合法量级内、下游没有闸门拦得住」。本迭代把拒绝
// 换成路由之后，那个担忧由「`_mom` 与 `_ytd` 是两个不同的列」承担 ⇒ 判据相应变成：
//
//	sectorCaliberOf 判它是 caliberCurrent（**报出口径**，不再报错）
//	分部门值落在 _mom 列，且 **_ytd 列一个都没有**（← 这半条才是原拒绝理由的继承者）
//
// ⚠️ **不要把这条测试删掉换成新写的**：它盯的两篇是当初逼出这条守卫的真实样本，
// 换掉样本等于换掉证据。
//
// C' 类（2022-08）现在是**期望中的混合体**：合计句「1-8月…累计增加」是累计口径 ⇒
// loan_flow_ytd；而分部门段跟在当月句之后 ⇒ 分项走 _mom。旧注释说「救了那句会把这一篇
// 从安全失败变成口径混杂」——那句话在**单列**的世界里成立，在**双列**的世界里正是设计目标。
func TestSectorCaliberRoutesNonCumulativeSamplesToMoM(t *testing.T) {
	for _, tc := range []struct{ sample, class, prefix string }{
		{"pboc-2022-08-monthly.html", "C' 类：有 1-N月 累计合计句但分部门是当月", "8月份"},
		{"pboc-2020-04-monthly.html", "C 类：全篇只有当月句", "4月份"},
	} {
		secs := splitSections(stripHTML(readSample(t, tc.sample)))
		for _, side := range []struct {
			name, kw, anchor   string
			fn                 func(section) (map[string]float64, error)
			coverage           func() []*regexp.Regexp
			momField, ytdField string
		}{
			{"贷款侧", "人民币贷款", loanSectorAnchor, extractLoanSection, loanSectorCoverage,
				FieldLoanHHShortMoM, FieldLoanHHShortYTD},
			{"存款侧", "人民币存款", depositSectorAnchor, extractDepositSection, depositSectorCoverage,
				FieldDepositHouseholdMoM, FieldDepositHouseholdYTD},
		} {
			t.Run(tc.sample+"/"+side.name, func(t *testing.T) {
				sec, ok := findSection(secs, side.kw)
				require.True(t, ok)

				// ① 口径判定：报出 caliberCurrent，不再报错
				cal, cerr := sectorCaliberOf(sec.Body, side.anchor, side.coverage())
				require.NoError(t, cerr, "%s：路由化之后不该再拒绝", tc.class)
				assert.Equalf(t, caliberCurrent, cal,
					"%s：分部门段前最近的前缀是 %q，应判当月口径", tc.class, tc.prefix)

				// ② 🔴 原拒绝理由的继承者：值落在 _mom，**_ytd 一个都没有**
				got, err := side.fn(sec)
				require.NoErrorf(t, err, "%s：能读出口径就该分列写出来，不该整篇失败", tc.class)
				assert.Containsf(t, got, side.momField, "当月口径的分部门值必须落进 _mom 列")
				assert.NotContainsf(t, got, side.ytdField,
					"🔴 当月值出现在 %s 列 —— 这正是本迭代要防的那件事："+
						"两个值都在合法量级内，下游没有任何闸门拦得住", side.ytdField)
			})
		}
	}
}

// TestSectorCaliberKeepsCumulativeSamplesIntact 是 functional[2] 的回归底线：
// 既有季报/年报的分部门段前缀是 全年 / 一季度 / 前三季度 / 上半年，都在表内，
// 本守卫对它们必须完全透明。
//
// 逐样本 × 逐侧断言「无错 + 分部门字段仍在」，比「整包测试没红」精确：后者会被
// 别的测试掩盖，也说不清是哪一篇哪一侧。
func TestSectorCaliberKeepsCumulativeSamplesIntact(t *testing.T) {
	for _, sample := range []string{
		"pboc-2025-12-annual.html", "pboc-2020-06-h1.html",
		"pboc-2025-09-q3.html", "pboc-2026-03-q1.html",
	} {
		secs := splitSections(stripHTML(readSample(t, sample)))
		for _, side := range []struct {
			name, kw string
			fn       func(section) (map[string]float64, error)
			field    string
		}{
			{"贷款侧", "人民币贷款", extractLoanSection, FieldLoanHHShortYTD},
			{"存款侧", "人民币存款", extractDepositSection, FieldDepositHouseholdYTD},
		} {
			t.Run(sample+"/"+side.name, func(t *testing.T) {
				sec, ok := findSection(secs, side.kw)
				require.True(t, ok)
				got, err := side.fn(sec)
				require.NoError(t, err, "累计期报告不得被本守卫误伤")
				assert.Contains(t, got, side.field)
			})
		}
	}
}

// TestSectorCaliberErrorIsDistinguishable 覆盖 error_handling：三种失败的排障方向
// 完全不同，措辞必须可区分。
//
// 判据不是「各自的信息里有没有自己的关键词」——那样把三条文案改成同一句也能全绿。
// 而是**交叉**：每条必须含自己的标志串**且不含**另外两条的。
func TestSectorCaliberErrorIsDistinguishable(t *testing.T) {
	const (
		markCaliber = "读不出它是累计还是当月口径"      // M1c-4 的 TASK-005：路由化后唯一保留的口径类拒绝
		markSection = "requires a section" // 板块整节缺失
		markAnchor  = "scope anchor"       // 贷款作用域锚点找不到
	)
	for _, tc := range []struct {
		name    string
		run     func() error
		want    string
		notWant []string
	}{
		{
			// 🔴 M1c-4 的 TASK-005：「口径不对」不再是一种失败——能读出前缀就分列写。
			// 路由化之后唯一保留的口径类拒绝是「**读不出**口径」，故这一格换成它：
			// 有分部门段、但它之前一个期次前缀都没有。
			name: "读不出口径",
			run: func() error {
				// 🔴 分部门句排在**全文最前**：sectorSegmentStart 取「抽取会碰到的最早
				// 那一句」的位置，于是它之前的文本为空 ⇒ 一个期次前缀都读不到。
				// 余额句与合计句排在其后（它们的匹配与位置无关），本节其余部分完好，
				// 所以这一格失败的**唯一**理由就是读不出口径。
				_, err := extractDepositSection(section{Body: "住户存款增加14.64万亿元，" +
					"非金融企业存款增加2.31万亿元，财政性存款增加6579亿元，非银行业金融机构存款减少7446亿元。" +
					"8月末，人民币存款余额301.6万亿元，同比增长10.5%。" +
					"前八个月人民币存款增加15.6万亿元。"})
				return err
			},
			want: markCaliber, notWant: []string{markSection, markAnchor},
		},
		{
			name: "板块整节缺失",
			run: func() error {
				// 喂一份**不含**任何必需板块的输入：含「广义货币」标题的话会进
				// extractMoneySection 报「货币 M2 not found」，那是另一种失败（实撞）。
				_, err := extractFields([]section{{Title: "九、与本包无关的一节", Body: "略"}}, extractorMonthlyV1)
				return err
			},
			want: markSection, notWant: []string{markCaliber, markAnchor},
		},
		{
			name: "作用域锚点找不到",
			run: func() error {
				// 累计前缀（守卫放行）+ 有「分部门看」但没有任何部门锚点
				_, err := extractLoanSection(section{Body: "月末人民币贷款余额232.28万亿元，同比增长11.1%。" +
					"前八个月人民币贷款增加17.44万亿元。分部门看，没有任何部门。"})
				return err
			},
			want: markAnchor, notWant: []string{markCaliber},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run()
			require.Error(t, err)
			assert.Containsf(t, err.Error(), tc.want, "缺自己的标志串")
			for _, nw := range tc.notWant {
				assert.NotContainsf(t, err.Error(), nw,
					"含了另一种失败的标志串 %q ⇒ 两者不可区分，排障会被指错方向", nw)
			}
		})
	}
}

// TestExtractFieldsDistinguishesTwoRejectionKinds 是 non_functional 里那条 🟡：
// 把 M1c-3a 的 TASK-006 只写在 decision 里的设计意图升级成契约。
//
// 那条 decision 说「两种拒绝的排障方向完全不同，合并会把两边都指错方向」，但当时
// 没有任何断言守着它——test-m1c3a-v2 的变异 N2 把「合法但走错路」那条 case 改成恒 false，
// 三个值（含**已知合法的 llm-fallback@v1**）全落进 default 报成 unknown extractor，
// **全套零条红**。原因是既有断言只查「回显 extractor 名」+「含全部白名单值」，
// 而**两条分支都满足**——查的是共有属性，而要守的是「两者可区分」这个关系性属性。
//
// 交叉断言：各自含**只有自己才有**的标志串，且**不含**对方的。
func TestExtractFieldsDistinguishesTwoRejectionKinds(t *testing.T) {
	const (
		markWrongPath = "does not take the section path"
		markUnknown   = "unknown extractor"
	)
	secs := splitSections(stripHTML(readSample(t, "pboc-2025-12-annual.html")))

	for _, tc := range []struct{ name, extractor, want, notWant string }{
		{"合法但走错路_存量", extractorTSFStock, markWrongPath, markUnknown},
		{"合法但走错路_增量", extractorTSFFlow, markWrongPath, markUnknown},
		{"合法但走错路_llm兜底", "llm-fallback@v1", markWrongPath, markUnknown},
		{"值本身认不出", "rule@v3", markUnknown, markWrongPath},
		{"空串", "", markUnknown, markWrongPath},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := extractFields(secs, tc.extractor)
			require.Error(t, err)
			assert.Containsf(t, err.Error(), tc.want, "缺自己的标志串")
			assert.NotContainsf(t, err.Error(), tc.notWant,
				"含了另一种拒绝的标志串 ⇒ 两者不可区分，而它们的排障方向相反")
		})
	}
}

// —— M1c-3a 的 TASK-011 · error_handling[0]（从 M1c-3a 的 TASK-012 转入）——

// 🔴 **两种成因完全不同的失败，一度收敛成同一条错误信息**。
//
// `flowRE` = `periodPat` + 「，?」+ 句尾模板。期次前缀不在 `periodAlt` 里时**整条正则
// 不命中** ⇒ 那句话根本没进候选集 ⇒ `selectUnique` 报的是「没找到」，与「本节确实
// 没有累计句」**逐字同形**：
//
//	2020-04（正文真的没有累计句）      → not found among 1 candidate(s) [4月份/人民币]
//	2022-10（有「1-10月…累计增加」，只是前缀不认识）
//	                                  → not found among 2 candidate(s) [10月份/人民币 10月份/外币]
//
// 结构完全相同，唯一差别是候选数，**而那个数不携带「为什么」**。
//
// 🔴 **两者的后续动作相反**：前者是解析器缺口（该往 `periodAlt` / `cumulativePeriods` 加，
// M1c-4 要修），后者是报告本身没数据（修不了，正确的是标注）。R3 的原始损害正是
// 这一条——「该期报告只有当月数」被写进了验收报告与 CONTRACTS，**而数据就在正文里**。
//
// ⚠️ **交叉断言**：两条错误串**互不出现**在对方那一格里。「两条分支可区分」是**关系性**
// 属性，两边都满足「有报错」时它照样可以被破坏（M1c-3a 的 TASK-006 的 N2 同族）。
func TestCumulativeFlowDistinguishesUnknownPrefixFromNoCumulativeSentence(t *testing.T) {
	const (
		markerNoSentence = "not found among" // A 类：候选集里没有累计口径的那一条
		markerUnknown    = "期次前缀不被识别"        // B 类：有那句话，只是前缀不认识
	)

	t.Run("A_本节确实没有累计句_真实语料2020-04", func(t *testing.T) {
		secs := splitSections(stripHTML(readSample(t, "pboc-2020-04-monthly.html")))
		sec, ok := findSection(secs, "人民币贷款")
		require.True(t, ok)

		// 🔴 **本格同时是「诊断门没有下移」的正当性证据**（M1c-4 的 TASK-005）。
		//
		// DoD 要求把「前缀不认识」诊断从「累计族为空」下移到「两族都空」，理由是
		// 「一篇只有当月合计句的报告会被误报」。**实测该前提为假**：诊断器自己的谓词
		// 已经足够精确——2020-04 是纯当月报告，它返回 **[]**，门留在原位也不会响。
		require.Empty(t, unrecognisedPeriodPrefixes(sec.Body, loanFlowRE),
			"用例前提兼 DoD 前提证伪：纯当月报告不会让诊断器有话说，故诊断门无需下移")

		// 路由化之后它不再报错，而是走当月列——那正是本迭代的目标。
		m, cum, err := selectRMBFlowByCaliber(loanFlowRE, sec.Body, currencyRMB+"贷款期内合计")
		require.NoError(t, err, "只有当月合计句的报告不该被整篇拒绝")
		assert.False(t, cum, "口径必须报当月：报累计会把 4 月当月值写进 loan_flow_ytd")
		assert.Equal(t, "4月份", m[1])
	})

	// 两族都没有时才是错误，且**不得**报成「前缀不认识」——A 类原本守的就是这条交叉。
	t.Run("A2_两族都没有任何合计句_合成", func(t *testing.T) {
		const body = "月末人民币贷款余额201.66万亿元，同比增长10.9%。前十一个月外币贷款增加123亿美元。"
		require.Empty(t, unrecognisedPeriodPrefixes(body, loanFlowRE), "用例前提：纯 A 类")

		_, _, err := selectRMBFlowByCaliber(loanFlowRE, body, currencyRMB+"贷款期内合计")
		require.Error(t, err, "一条人民币合计句都没有 ⇒ 必须响亮失败，不能静默产出空 Values")
		assert.Contains(t, err.Error(), markerNoSentence)
		assert.NotContains(t, err.Error(), markerUnknown,
			"交叉：A 类不得报成「前缀不认识」——那会把一篇没数据的报告送进 M1c-4 的兜底清单")
	})

	// 🔴 **我先写下「TASK-012 之后全语料已无 B 类」，随后实测把它证伪了**——留着这句
	// 是因为下一个人很可能做同样的假设。真值（本任务在 `05b50be` 上扫 218 篇、160 个板块）：
	//
	//	2022-05 存款侧 / 贷款侧   前缀「今年前5个月」   ← **活的 B 类**，本诊断当场抓到
	//	2020-02 / 2020-03 两侧    前缀「N月当月」        ← 惰性：那一节另有被认识的累计句
	//	                                                （`hits>0`，诊断根本不会被问到）
	//
	// 「今年前5个月，人民币贷款累计增加10.87万亿元」就在正文里，而 TASK-012 补的是
	// `1-N月` 一族、没有覆盖它 ⇒ **R3 的失效方式此刻仍然活着，只是换了个前缀写法**。
	//
	// ⚠️ 本格仍用**合成**正文，是刻意的：真实的 2022-05 一旦有人把「今年前N个月」补进
	// `periodAlt`（那是正确的修复），这一格就会红——**守卫会被一次正确的修复打坏**。
	// 合成前缀「今年以来」不会有人去加，所以它钉住的是**性质**而不是某一期语料的现状。
	// 合成的只有「前缀」这一个变量，句尾与真实 2022 年月报逐字同形。
	t.Run("B_有累计句但前缀不被认识_合成", func(t *testing.T) {
		const body = "4月末，月末人民币贷款余额201.66万亿元，同比增长10.9%。" +
			"4月份人民币贷款增加6454亿元，同比少增8231亿元。" +
			"今年以来，人民币贷款累计增加18.7万亿元。"

		// 前提①：那句话确实在正文里，且句尾与真实体例同形
		require.Contains(t, body, "人民币贷款累计增加18.7万亿元")
		// 前提②：`flowRE` 确实认不出它 —— 否则这一格根本不成立
		require.NotContains(t, loanFlowRE.String(), "今年以来")
		require.Len(t, unrecognisedPeriodPrefixes(body, loanFlowRE), 1,
			"用例前提：恰好一句「形状正确但前缀不认识」")

		// 🔴 **本格是「诊断门必须留在累计族为空」的承重证据**（M1c-4 的 TASK-005）：
		// 这段正文**同时**有当月合计句（4月份…增加6454亿元）与一条前缀不认识的累计句。
		// 若按 DoD 原文把门下移到「两族都空」，当月族有命中 ⇒ 诊断不触发 ⇒ 静默走当月
		// 路径、把可恢复的 18.7 万亿累计数据丢掉且**不告诉任何人**（实测确认过这个行为）。
		// 那正是 R3 的失效方式，也正是本诊断存在的全部理由。
		_, _, err := selectRMBFlowByCaliber(loanFlowRE, body, currencyRMB+"贷款期内合计")
		require.Error(t, err, "累计族为空且存在不被认识的前缀 ⇒ 必须响亮报解析器缺口")
		assert.Contains(t, err.Error(), markerUnknown)
		assert.Contains(t, err.Error(), "今年以来",
			"要报出**实际读到的**那个前缀，否则下一个人还得自己去正文里找")
		assert.NotContains(t, err.Error(), markerNoSentence,
			"交叉：B 类不得报成「没找到」——那正是 R3 那句假话的来源")
	})

	// 🔴 **这一格是消融逼出来的**：变异「把 `!slices.ContainsFunc(ms, keep)` 这道门放宽成
	// 『合口径命中 <= 1』」在补它之前**全套零条红、SURVIVED** —— 而那道门是承重的：真语料 2020-02 / 2020-03
	// 两侧正是这个形态（另有被认识的累计句 `前两个月` / `一季度`，同时存在前缀不被认识的
	// 「N月当月」句）。门一旦松掉，这两期会从**抽取成功**变成报一句「前缀不认识」的假话。
	//
	// 与 A / B 构成最小三元组：三格的正文只差「有没有被认识的累计句」这一个变量。
	t.Run("C_既有被认识的累计句也有不被认识的前缀_必须照常抽出", func(t *testing.T) {
		const body = "前两个月人民币贷款累计增加4.24万亿元，同比多增1.06万亿元。" +
			"2月当月人民币贷款增加9057亿元，同比多增199亿元。"

		// 前提：诊断器**确实有话可说** —— 否则这一格测不到那道门
		require.NotEmpty(t, unrecognisedPeriodPrefixes(body, loanFlowRE),
			"用例前提：必须存在前缀不被认识的句子，否则门开不开都一样")

		m, cum, err := selectRMBFlowByCaliber(loanFlowRE, body, currencyRMB+"贷款期内合计")
		require.NoError(t, err, "有被认识的累计句时必须照常抽出，不得被诊断器抢先报错")
		require.True(t, cum, "取的是累计句，口径必须报累计")
		assert.Equal(t, "前两个月", m[1], "取的必须是那条累计句，不是隔壁的当月句")
	})
}

// TestFlowTailIsDerivedFromFlowRENotRewritten 钉住诊断器**不另写一份模板**。
//
// `unrecognisedPeriodPrefixes` 要问「句尾对了、只是前缀没对」，就需要一条「去掉期次
// 前缀的 flowRE」。它是从 `flowRE` 自身的 pattern **切出来**的，不是照着抄一份 ——
// 抄一份就会分叉，而分叉的表现是诊断器悄悄失效（M1c-3a 的 TASK-011 的 R1 同族：
// 两份实现之间的一致性只能靠断言维持，而分叉正是缺陷的形状）。
//
// ⚠️ 这一条同时挡住**静默失效**：切不出前缀时诊断器返回 nil（不适用），
// 若哪天 `flowRE` 换了形状而没人发现，本条会红，而不是让诊断器无声地永远返回空。
func TestFlowTailIsDerivedFromFlowRENotRewritten(t *testing.T) {
	for _, re := range []*regexp.Regexp{loanFlowRE, depositFlowRE, flowRE("贷款")} {
		tail := cumulativeFlowTail(re)
		require.NotNilf(t, tail, "%s 切不出句尾 ⇒ 诊断器对它恒返回空，等于没有", re)
		assert.Truef(t, strings.HasSuffix(re.String(), tail.String()),
			"句尾必须是 flowRE 的真后缀，否则两者已经分叉")
		assert.Equalf(t, periodPat+"，?"+tail.String(), re.String(),
			"flowRE 必须恰好是「期次前缀 + 句尾」，多一段少一段都说明形状变了")
	}
}

// —— M1c-3a 的 TASK-011 · R2：判据必须与「抽取的覆盖面」同源 ——

// 🔴 **锚点缺席即放行，而抽取根本不看锚点**（QA R2）。
//
// `checkSectorCaliber` 原先只问「正文里有没有那个锚点短语」，缺席就 return nil；而
// `extractDepositSection` 用 `sectorFlowRE` 扫**整节**、`extractLoanSection` 按
// `loanScopeSpans` 的锚点切段，**两者都不依赖那个文本锚点** ⇒ **守卫不表态而抽取照做**。
//
// 真实反例就是本用例的 2022-04（QA 扫全部 80 篇金融统计报告得「锚点缺席但分部门模板
// 仍命中」的段恰好 2 个，贷款侧、存款侧各 1；dev-m1c3a-a 在 `05b50be` 上独立复算，
// 两侧判定由 `nil(不表态)` 变成 `ERR(非累计:4月份)`，**全语料 160 个段里也只有这 2 个改变判定**）。
//
// ⚠️ **它今天不出事，靠的是与本守卫无关的巧合**：2022-04 也没有累计合计句，在更早一道
// 闸（`人民币贷款期内合计 not found`）就被拒了。安全性来自巧合而不是守卫，正是
// `checkSectorCaliber` 自己注释里记着的那条线索——当初 D4 变异（锚点缺席 ⇒ 报错）SURVIVED。
//
// 🔴 **修法不是换一个更宽的锚点短语**：现状的失效模式是**判据的定义域与被守护动作的
// 定义域不一致**（守卫看锚点、抽取看模板），换个短语仍然是两个不同的定义域。
// 故判据改成「抽取会碰到的最靠前那个定位点」，名单由 depositSectorCoverage /
// loanSectorCoverage 从抽取用的同一批数据派生。
func TestSectorCaliberJudgesByExtractionCoverageNotAnchor(t *testing.T) {
	secs := splitSections(stripHTML(readSample(t, "pboc-2022-04-monthly.html")))

	for _, side := range []struct {
		name, keyword, anchor, what string
		coverage                    func() []*regexp.Regexp
		extract                     func(section) (map[string]float64, error)
		momField, ytdField          string
	}{
		{"存款侧", "人民币存款", depositSectorAnchor, currencyRMB + "存款分部门段", depositSectorCoverage,
			extractDepositSection, FieldDepositHouseholdMoM, FieldDepositHouseholdYTD},
		{"贷款侧", "人民币贷款", loanSectorAnchor, currencyRMB + "贷款分部门段", loanSectorCoverage,
			extractLoanSection, FieldLoanHHShortMoM, FieldLoanHHShortYTD},
	} {
		t.Run(side.name, func(t *testing.T) {
			sec, ok := findSection(secs, side.keyword)
			require.Truef(t, ok, "2022-04 里找不到 %q 板块", side.keyword)

			// —— 三条前提，缺一条这一格就在测别的东西 ——
			// ① 锚点确实缺席：在场的话旧判据也能覆盖，本格证明不了新判据的价值
			require.NotContainsf(t, sec.Body, side.anchor,
				"用例前提：锚点 %q 必须缺席", side.anchor)
			// ② 抽取确实会命中分部门句：一个都不命中时「不表态」才是对的
			hits := 0
			for _, re := range side.coverage() {
				if re.MatchString(sec.Body) {
					hits++
				}
			}
			require.GreaterOrEqual(t, hits, 1,
				"用例前提：本节必须有分部门模板命中，否则守卫本就该不表态")
			// ③ 这一期确实是当月口径：否则守卫表态之后的结论应当是放行
			require.Contains(t, sec.Body, "4月份", "用例前提：期次前缀是当月")

			// 路由化之后（M1c-4 的 TASK-005）判据从「必须报错」变成「必须表态为当月口径」。
			// 守的东西没变：锚点缺席但抽取会照做时，本判定**不能沉默**——沉默的后果由
			// 「报错」换成了「选错列」，而选错列同样是量级合理的错值。
			cal, err := sectorCaliberOf(sec.Body, side.anchor, side.coverage())
			require.NoError(t, err, "能读出前缀就不该报错，路由化只保留「读不出」那一条拒绝")
			assert.Equal(t, caliberCurrent, cal,
				"锚点缺席但抽取会照做 ⇒ 必须按覆盖面模板求起点并判出当月口径，不能沉默放行")

			// 端到端：值必须落进 _mom 列，_ytd 列一个都没有
			got, ferr := side.extract(sec)
			if ferr != nil {
				// ⚠️ 2022-04 贷款侧另有**与本格无关**的模板缺口（作用域子项「短期贷款」
				// 在该期正文里确实不存在）。本格的主体是口径判定，端到端只做一条弱断言：
				// 失败理由不得是口径——否则说明路由把一篇能读出口径的报告又拒了。
				assert.NotContains(t, ferr.Error(), "读不出它是累计还是当月口径",
					"能读出前缀就不该以口径为由失败")
				return
			}
			assert.Contains(t, got, side.momField)
			assert.NotContainsf(t, got, side.ytdField,
				"🔴 当月值出现在 %s 列 —— 本迭代要防的正是这件事", side.ytdField)
		})
	}
}

// TestSectorCaliberPrefixIsReadFromTheEarliestSectorLocus 一并钉住收紧判据的**阴性对照**
// 与「起点取最靠前」这个选择。
//
// ⚠️ **阴性对照不可省**。本 sprint 有过一次教训：照抄 reviewer 的建议收紧判据，实测
// **对目标缺陷全绿放行、同时把既有安全的用例判红**——既挡不住真问题又恒响的假红。
// 收紧之前必须先跑一次「它会把现有什么判红」。全语料实测（`05b50be`，80 篇金融统计
// × 两侧 = 160 个段）判定发生变化的**恰好 2 个**，都是 2022-04、都是
// `nil(不表态) → ERR(非累计:4月份)` ⇒ **没有任何一个原本放行的段被判红**。
//
// 🔴 **第三格是消融逼出来的**：变异「起点取最靠后而不是最靠前」在补它之前**全套零条红、
// SURVIVED** —— 真实语料上碰巧两种取法结论相同（2022-04 里最后一个当月前缀恰好又排在
// 累计前缀之后）。那正是 M1c-3a 的 TASK-009 的 D4 与 TASK-006 的 N2 同一形态：
// **写在注释里的设计选择本身没有守卫**。三格里前两格换成「取最靠后」照样绿，只有第三格红。
//
// 前两格与 TestSectorCaliberKeepsCumulativeSamplesIntact **不重复**：那条是端到端
// （抽取成功且有字段），这条**直调守卫**——本次改的正是守卫自己的判据。
func TestSectorCaliberPrefixIsReadFromTheEarliestSectorLocus(t *testing.T) {
	anchoredCumulative := func(t *testing.T) string {
		t.Helper()
		secs := splitSections(stripHTML(readSample(t, "pboc-2025-08-monthly.html")))
		sec, ok := findSection(secs, "人民币存款")
		require.True(t, ok)
		require.Contains(t, sec.Body, depositSectorAnchor, "本格的前提就是锚点在场")
		return sec.Body
	}

	// 两条合成正文只差**期次前缀的位置**，数值与句子一个字不动 —— 最小对。
	// 真实语料里没有「无锚点 + 累计前缀」这一类（2022-04 是唯一无锚点的段，而它是当月），
	// 故这一格只能合成；合成的是**前缀位置**这一个变量，不是整篇体例。
	const sectorSentences = "住户存款增加1000亿元。" +
		"非金融企业存款增加500亿元。财政性存款增加200亿元。非银行业金融机构存款增加300亿元。"
	const cumulativeFirst = "前八个月人民币存款增加20.24万亿元。" + sectorSentences
	// 当月前缀在首条分部门句之前、累计前缀在它**之后** —— 取最靠前 ⇒ 读到「8月份」判拒；
	// 取最靠后 ⇒ 读到「前八个月」误放行。两种取法在这一格结论相反。
	const monthlyFirstCumulativeLater = "8月份人民币存款增加909亿元。住户存款增加1000亿元。" +
		"前八个月住户存款和非金融企业存款分别增加7.12万亿元和1.27万亿元。" +
		"非金融企业存款增加500亿元。财政性存款增加200亿元。非银行业金融机构存款增加300亿元。"

	for _, tc := range []struct {
		name    string
		body    func(*testing.T) string
		wantCal sectorCaliber
	}{
		{"锚点在场_累计", anchoredCumulative, caliberCumulative},
		{"无锚点_首条分部门句之前是累计前缀", func(*testing.T) string { return cumulativeFirst }, caliberCumulative},
		// 🔴 判据换形式不换内容（M1c-4 的 TASK-005）：原来断言「拒绝」，理由是首条分部门句
		// 之前是当月前缀。路由化之后同一个理由要求它判**当月口径**——判成累计就等于把当月
		// 值送进 _ytd，正是原来那条拒绝要防的。取更靠后的累计前缀会得到 caliberCumulative，
		// 本格即红。
		{"无锚点_首条之前是当月而更靠后才有累计前缀_必须判当月",
			func(*testing.T) string { return monthlyFirstCumulativeLater }, caliberCurrent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cal, err := sectorCaliberOf(tc.body(t), depositSectorAnchor, depositSectorCoverage())
			require.NoError(t, err, "三格的正文都有期次前缀，不该触发「读不出口径」那条拒绝")
			assert.Equal(t, tc.wantCal, cal,
				"口径判定必须由**首条分部门句之前**那个前缀决定；取成更靠后的前缀会选错列")
		})
	}
}

// TestSectorCaliberStaysSilentWhenNoSectorSegment 钉住 checkSectorCaliber 的第三种
// 返回：**本节没有分部门段时不表态**。
//
// ⚠️ **订正（M1c-3a 的 TASK-011）**：本条原写「锚点不存在时不表态（实测 55 篇月报中
// 2022-04 是这一类）」，**那个例子是错的** —— 2022-04 锚点确实缺席，但它两侧各有 4 条
// 分部门句会被抽取命中，属于 QA R2 那个缺陷的现场，不属于本格。判据随之从「锚点在不在」
// 改成「抽取会不会碰到分部门句」，本格的前提也跟着补了一条（见下面的 coverage 循环）。
//
// 🔴 **这条是消融逼出来的**：M1c-3a 的 TASK-009 变异 D4 把「锚点不存在 ⇒ return nil」
// 改成「⇒ 报错」，**全套零条红、SURVIVED**。那个决定当时只写在 checkSectorCaliber 的
// 注释里，没有任何断言守着它——与 test-m1c3a-v2 在 TASK-006 用 N2 发现的形态同族：
// **被写进 rationale 的设计价值，本身没有守卫。**
//
// 决定本身的理由：没有分部门段时「口径」这个问题不适用，而分部门字段是必需的，
// 紧随其后的 mustMatch / loanScopeSpans 会报**更具体**的那条（「缺的是哪个分部门」
// 或「哪个作用域锚点找不到」）。让更具体的先说话。
//
// 故断言分两层：① 本守卫直调返回 nil；② 端到端的错误必须是**那条更具体的**，
// 而不是任何一条来自本守卫的。只写 ② 不够——「不含本守卫的标志串」在 D4 那种
// 换个措辞的变异下照样成立。
func TestSectorCaliberStaysSilentWhenNoSectorSegment(t *testing.T) {
	for _, tc := range []struct {
		name, body, anchor, wantMark string
		fn                           func(section) (map[string]float64, error)
		coverage                     func() []*regexp.Regexp
	}{
		{
			name: "贷款侧",
			body: "月末人民币贷款余额232.28万亿元，同比增长11.1%。" +
				"前八个月人民币贷款增加17.44万亿元。",
			anchor: loanSectorAnchor, wantMark: "scope anchor", fn: extractLoanSection,
			coverage: loanSectorCoverage,
		},
		{
			name: "存款侧",
			body: "月末人民币存款余额278.76万亿元，同比增长10.5%。" +
				"前八个月人民币存款增加20.24万亿元。",
			anchor: depositSectorAnchor, wantMark: "存款分部门", fn: extractDepositSection,
			coverage: depositSectorCoverage,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NotContainsf(t, tc.body, tc.anchor,
				"用例前提：正文里不能有锚点 %q，否则测的不是这一格", tc.anchor)
			// 前提②（M1c-3a 的 TASK-011 补）：**一条分部门句都不能命中**。
			// 判据从「锚点在不在」改成「抽取会不会碰到东西」之后，只声明锚点缺席已经
			// 不足以把这一格钉在「无分部门段」上了——2022-04 正是「锚点缺席但分部门句
			// 仍在」的真实反例，它属于隔壁那一格（见
			// TestSectorCaliberJudgesByExtractionCoverageNotAnchor）。
			for _, re := range tc.coverage() {
				require.Falsef(t, re.MatchString(tc.body),
					"用例前提：本节不能命中任何分部门模板（%s），否则测的不是这一格", re)
			}

			// ① 本判定对「无分部门段」不表态：返回 caliberAbsent 且不报错
			cal, cerr := sectorCaliberOf(tc.body, tc.anchor, tc.coverage())
			assert.NoError(t, cerr, "没有分部门段时必须放行，让更具体的那条错误先说话")
			assert.Equal(t, caliberAbsent, cal,
				"必须报 absent 而不是随便选一族——pick 对 absent 返回空串，"+
					"任何一族都会让下游写进一个它没资格判断的列")

			// ② 端到端仍然失败，但报的是**更具体**的那条
			got, err := tc.fn(section{Body: tc.body})
			require.Error(t, err, "分部门字段是必需的，缺了整段仍须失败")
			assert.Contains(t, err.Error(), tc.wantMark,
				"错误必须来自更具体的那条（缺哪个分部门 / 哪个作用域锚点），"+
					"而不是一句笼统的「没有分部门段」")
			assert.NotContains(t, err.Error(), "累计口径",
				"无分部门段不是口径问题，不该报成口径错误")
			assert.Empty(t, got)
		})
	}
}

// —— M1c-4 的 TASK-005：sectorCaliberOf 报口径而不再拒绝 ——
//
// Context Checkpoint: done_criteria → test mapping（M1c-4 的 TASK-005）
// functional[0]  sectorCaliber 三值 + sectorCaliberOf 取代 checkSectorCaliber
//                          → TestSectorCaliberOfReportsInsteadOfRefusing
// functional[0]  cal 每段求一次（不在循环内） → TestSectorCaliberIsComputedOncePerSection
// functional[2]  路由化后唯一保留的拒绝       → TestSectorCaliberOfStillRefusesWhenUnreadable（V1）
// functional[2]  pick 按口径选列              → TestNameFieldPickSelectsColumnByCaliber（profiles_test.go，V3 的一半）
// boundary[1]    真实体例 golden，两条 NotContains
//                          → TestMixedCaliberReportRoutesEachSectionToItsOwnColumns（V3 的现场）

// sectorCaliberOf 的三种返回，各用一份真实语料钉住。
//
// ⚠️ **判据是「等于哪一个枚举值」而不是「没报错」**：一个恒返回 caliberCumulative 的实现
// 能让「没报错」全绿，而那正是把当月值送进 _ytd 的那条路（变异 V2 就是这么改的）。
func TestSectorCaliberOfReportsInsteadOfRefusing(t *testing.T) {
	for _, tc := range []struct {
		sample, keyword string
		anchor          string
		coverage        func() []*regexp.Regexp
		want            sectorCaliber
		why             string
	}{
		{"pboc-2025-08-monthly.html", "人民币贷款", loanSectorAnchor, loanSectorCoverage,
			caliberCumulative, "分部门段跟在「前八个月」之后"},
		{"pboc-2023-08-monthly.html", "人民币贷款", loanSectorAnchor, loanSectorCoverage,
			caliberCurrent, "分部门段跟在「8月份」之后 —— 同一篇里合计句却是累计，正是双口径体例"},
		{"pboc-2023-08-monthly.html", "人民币存款", depositSectorAnchor, depositSectorCoverage,
			caliberCurrent, "存款侧同构"},
	} {
		t.Run(tc.sample+"/"+tc.keyword+"/"+tc.why, func(t *testing.T) {
			secs := splitSections(stripHTML(readSample(t, tc.sample)))
			sec, ok := findSection(secs, tc.keyword)
			require.True(t, ok)

			cal, err := sectorCaliberOf(sec.Body, tc.anchor, tc.coverage())
			require.NoError(t, err, "能读出期次前缀就不该报错——路由化只保留「读不出」那一条")
			assert.Equal(t, tc.want, cal, "%s", tc.why)
		})
	}

	// 第三种返回：本节没有分部门段 ⇒ 不表态。用一段只有余额与合计句的正文。
	cal, err := sectorCaliberOf("前八个月人民币存款增加15.6万亿元。", depositSectorAnchor, depositSectorCoverage())
	require.NoError(t, err)
	assert.Equal(t, caliberAbsent, cal,
		"没有任何分部门句 ⇒ absent。判成某一族会让下游写进一个它没资格判断的列")
}

// 🔴 **路由化之后唯一还保留的拒绝**，是原设计「refusing to guess」的落点。
//
// 删掉它，路由就变成了掷硬币：口径未知时把值随便写进某一列，而两列都在合法量级内、
// 下游没有任何闸门拦得住——本迭代整个是围绕这件事做的。
//
// # 变异判据（交付前实测，DoD functional[2] 的 V1）
//
// 把 sectorCaliberOf 里「段前无期次前缀 ⇒ 报错」那个分支改成 `return caliberCurrent, nil`
// （读不出就当当月），**本测试必须转红**。实测：M1 变异下本测试 FAIL，另两条 PASS。
//
// ⚠️ **构造输入时先问：被测的那个分支会被执行到吗？** sectorCaliberOf 是多段判定
// （sectorSegmentStart → FindAllString → 查表），任何一步都可能让后面的步骤根本不被求值。
// 这里把分部门句排在**全文最前**，于是 `body[:i]` 为空串 ⇒ 确实走到「没有任何期次前缀」
// 那一支。若把它放在合计句之后，前面就有前缀了，本测试会恒绿（TASK-003 的教训）。
func TestSectorCaliberOfStillRefusesWhenUnreadable(t *testing.T) {
	const body = "住户存款增加14.64万亿元，非金融企业存款增加2.31万亿元，" +
		"财政性存款增加6579亿元，非银行业金融机构存款减少7446亿元。" +
		"8月末，人民币存款余额301.6万亿元，同比增长10.5%。前八个月人民币存款增加15.6万亿元。"

	// 前提①：分部门段的起点确实在全文最前 ⇒ 它之前没有任何文本可供找前缀
	require.Equal(t, 0, sectorSegmentStart(body, depositSectorAnchor, depositSectorCoverage()),
		"用例前提：分部门句必须排在最前，否则测不到「读不出口径」那一支")
	// 前提②：正文里**确实有**期次前缀，只是不在分部门段之前 ——
	// 少了这一条，本格可能因为「全文根本没有前缀」而绿，那测的是另一回事
	require.NotEmpty(t, sectorPeriodRE.FindAllString(body, -1),
		"用例前提：正文里有前缀，只是位置在分部门段之后")

	cal, err := sectorCaliberOf(body, depositSectorAnchor, depositSectorCoverage())
	require.Error(t, err, "🔴 读不出口径必须拒绝——猜一个就是掷硬币")
	assert.Equal(t, caliberAbsent, cal, "报错时不得同时返回一个可用的口径")
	assert.Contains(t, err.Error(), "读不出它是累计还是当月口径",
		"错误必须点名是**口径读不出**，与「没有分部门段」「模板不命中」都不同")
	assert.Contains(t, err.Error(), "refusing to guess")

	// 端到端：整节必须失败，不产出任何分部门字段（两列都不许有）
	got, ferr := extractDepositSection(section{Body: body})
	require.Error(t, ferr)
	assert.Contains(t, ferr.Error(), "存款分部门段", "调用方要把是哪一侧补进错误里")
	assert.NotContains(t, got, FieldDepositHouseholdYTD)
	assert.NotContains(t, got, FieldDepositHouseholdMoM)

	// 贷款侧同构：**两侧各有一处 wrap，只测一侧的话另一侧漏了 what 也不会红**
	const loanBody = "住户贷款增加3922亿元，其中，短期贷款增加2320亿元，中长期贷款增加1602亿元；" +
		"企（事）业单位贷款增加9488亿元，其中，短期贷款减少401亿元，中长期贷款增加6444亿元，" +
		"票据融资增加3472亿元；非银行业金融机构贷款减少358亿元。" +
		"8月末，人民币贷款余额232.28万亿元，同比增长11.1%。前八个月人民币贷款增加17.44万亿元。"
	require.Equal(t, 0, sectorSegmentStart(loanBody, loanSectorAnchor, loanSectorCoverage()),
		"用例前提：贷款分部门句同样排在最前")

	lcal, lerr := sectorCaliberOf(loanBody, loanSectorAnchor, loanSectorCoverage())
	require.Error(t, lerr)
	assert.Equal(t, caliberAbsent, lcal)

	lgot, lferr := extractLoanSection(section{Body: loanBody})
	require.Error(t, lferr)
	assert.Contains(t, lferr.Error(), "贷款分部门段")
	assert.Contains(t, lferr.Error(), "读不出它是累计还是当月口径")
	assert.NotContains(t, lgot, FieldLoanHHShortYTD)
	assert.NotContains(t, lgot, FieldLoanHHShortMoM)
}

// 🔴 `cal` 必须**每段求一次**，不得在分部门循环内部逐项判（DoD functional[0]）。
//
// 这不是「算四遍浪费」的问题。sectorCaliberOf 对一个段返回**一个**标量 ⇒ pick 对该段
// 全部条目返回同一族 ⇒ 另一族一个都不写 ⇒ **成对列在同一观测里必然单侧**。
// TASK-011 的整条防线（`|ytd| ≥ |mom|`）建立在这个前提上；改成逐项判不是「实现得更细」，
// 是**换掉下游防线的分析基座**，而表现上看不出来。
//
// ⚠️ 判据是**性质**（同一段内所有分部门字段同族），不是「调用了几次」——后者要么靠计数
// 桩、要么靠读源码，都会在重构时假红。性质判据只在真的分叉时红。
func TestSectorCaliberIsComputedOncePerSection(t *testing.T) {
	for _, tc := range []struct {
		sample, keyword string
		fn              func(section) (map[string]float64, error)
		ytd, mom        []string
	}{
		{"pboc-2023-08-monthly.html", "人民币存款", extractDepositSection,
			[]string{FieldDepositHouseholdYTD, FieldDepositCorpYTD, FieldDepositFiscalYTD, FieldDepositNBFIYTD},
			[]string{FieldDepositHouseholdMoM, FieldDepositCorpMoM, FieldDepositFiscalMoM, FieldDepositNBFIMoM}},
		{"pboc-2025-08-monthly.html", "人民币存款", extractDepositSection,
			[]string{FieldDepositHouseholdYTD, FieldDepositCorpYTD, FieldDepositFiscalYTD, FieldDepositNBFIYTD},
			[]string{FieldDepositHouseholdMoM, FieldDepositCorpMoM, FieldDepositFiscalMoM, FieldDepositNBFIMoM}},
	} {
		t.Run(tc.sample, func(t *testing.T) {
			secs := splitSections(stripHTML(readSample(t, tc.sample)))
			sec, ok := findSection(secs, tc.keyword)
			require.True(t, ok)
			got, err := tc.fn(sec)
			require.NoError(t, err)

			nYTD, nMoM := 0, 0
			for _, f := range tc.ytd {
				if _, ok := got[f]; ok {
					nYTD++
				}
			}
			for _, f := range tc.mom {
				if _, ok := got[f]; ok {
					nMoM++
				}
			}
			// 一段一个口径 ⇒ 必然是「一族全有、另一族全无」
			assert.Truef(t, (nYTD == len(tc.ytd) && nMoM == 0) || (nMoM == len(tc.mom) && nYTD == 0),
				"同一分部门段内出现了两族共存（ytd %d/%d、mom %d/%d）⇒ 口径是逐项判的，"+
					"而 TASK-011 的 |ytd| ≥ |mom| 建立在「一段一个口径」上",
				nYTD, len(tc.ytd), nMoM, len(tc.mom))
		})
	}
}

// 🔴🔴 **本迭代头号风险的现场守卫**（DoD boundary[1]）。
//
// 把「拒绝」改成「路由」时，原拒绝理由背后的担忧必须换形式保留：
//
//	旧：合计取累计值、分部门混进当月值 ⇒ 内部口径不一致，两者都在合法量级内、
//	    下游没有任何闸门拦得住 ⇒ **拒绝整篇**
//	新：当月值写 `_mom`、累计值写 `_ytd` ⇒ **两个不同的列**
//
// ⇒ **那两条 NotContains 才是原拒绝的直系继承者，一条都不能省。** 正向断言（值对不对）
// 对「当月值进了 _ytd」完全不敏感——值本身是对的，只是进错了列。
//
// # 变异判据（交付前实测，DoD functional[2] 的 V3）
//
// 把 `nameField.pick` 的 `caliberCurrent` 分支改成返回 `nf.ytdField`（当月值写进累计列，
// 正是本迭代要防的那件事），**本测试的 NotContains 必须转红**。
// 🔴 若 V3 杀不动，说明头号风险的现场守卫没有区分力，**不要交付**。
//
// # 形态覆盖，不是单篇快照
//
// 只钉一篇是**正例快照**而不是不变式。三种形态各一条，用的都是 testdata 里的现成 fixture
// （不另外 cp：同一形态有两份样本迟早分叉）：
//
//	形态①  全篇只有当月句           pboc-2020-04-monthly.html
//	形态②  合计累计 + 分部门当月     pboc-2023-08-monthly.html / pboc-2022-08-monthly.html
//	阴性    全篇累计                 pboc-2025-08-monthly.html —— 不得被路由误伤
func TestMixedCaliberReportRoutesEachSectionToItsOwnColumns(t *testing.T) {
	for _, tc := range []struct {
		sample, form string
		// 合计句的口径**贷存两侧各判各的**：累计 ⇒ _ytd 有值且 _mom 无；当月 ⇒ 反之。
		// ⚠️ 两侧不必相同——2022-08 的贷款节有「1-8月…累计增加15.61万亿元」而存款节
		// 一句累计合计句都没有。最初我把两侧写成同一个 flag，被这一篇当场证伪。
		loanFlowCumulative    bool
		depositFlowCumulative bool
		// 分部门段的口径（贷存两侧实测一致，见 sectorCaliberOf 的判据说明）
		sectorCumulative bool
	}{
		{"pboc-2020-04-monthly.html", "形态①：全篇只有当月句", false, false, false},
		{"pboc-2023-08-monthly.html", "形态②：合计累计 + 分部门当月", true, true, false},
		{"pboc-2022-08-monthly.html", "形态②（另一期）：贷款合计累计、存款合计当月、分部门皆当月", true, false, false},
		{"pboc-2025-08-monthly.html", "阴性：全篇累计，不得被路由误伤", true, true, true},
	} {
		t.Run(tc.sample+"/"+tc.form, func(t *testing.T) {
			obs, err := Parse(readTestdata(t, tc.sample))
			require.NoError(t, err, "能读出口径的报告都应当产出，不再整篇拒绝")

			check := func(what string, cumulative bool, ytdField, momField string) {
				t.Helper()
				want, notWant := momField, ytdField
				if cumulative {
					want, notWant = ytdField, momField
				}
				assert.Containsf(t, obs.Values, want, "%s 的值应当落进 %s", what, want)
				// 🔴 原「拒绝」的直系继承者
				assert.NotContainsf(t, obs.Values, notWant,
					"🔴 %s 的值落进了 %s —— 口径错配的两个值都在合法量级内，"+
						"下游没有任何闸门拦得住。这正是本迭代要防的那件事", what, notWant)
			}

			// 合计句（走 selectRMBFlowByCaliber 这条路由）
			check("贷款期内合计", tc.loanFlowCumulative, FieldLoanFlowYTD, FieldLoanFlowMoM)
			check("存款期内合计", tc.depositFlowCumulative, FieldDepositFlowYTD, FieldDepositFlowMoM)

			// 分部门（走 sectorCaliberOf + pick 这条路由）—— 与合计句**各判各的**，
			// 形态②里两者恰好相反，正是「同一篇之内分段口径不同」这个体例本身。
			check("住户短期贷款", tc.sectorCumulative, FieldLoanHHShortYTD, FieldLoanHHShortMoM)
			check("住户存款", tc.sectorCumulative, FieldDepositHouseholdYTD, FieldDepositHouseholdMoM)
		})
	}
}

// 社融侧双口径的三条边界（M1c-4 的 TASK-005）。
//
// 这三格分别钉住 extractTSFFlowArticle / setTSFFlowItems 里三条**没有别的测试会走到**的
// 分支，而每一条的失效方式都是静默的：
//
//	两族都没有        → 不报错就会静默产出一个空 Values
//	当月族多于一条    → 就近取一条 = 孪生句里挑错那个（累计族的同名纪律，两族都要有）
//	分项抽到一部分    → 静默跳过会把「模板缺口」这类真缺陷一起吞掉
func TestTSFFlowArticleCaliberEdgeCases(t *testing.T) {
	t.Run("两族都没有_必须响亮失败", func(t *testing.T) {
		// 有形似总量的句子，但一条都不被 tsfFlowArticleTotalRE 认成总量句
		const body = "初步统计，2022年10月末社会融资规模存量为341.42万亿元，同比增长9.5%。"
		got, err := extractTSFFlowArticle(body)
		require.Error(t, err, "一条总量句都没有 ⇒ 必须失败，不能静默产出空 Values")
		assert.Nil(t, got)
		assert.Contains(t, err.Error(), "累计与当月两族都没有",
			"措辞要说清是「两族都没有」，与「只有当月」「只有累计」都不同")
		assert.Contains(t, err.Error(), "candidate sentence(s)",
			"要点名候选句数，否则排障看不出是「没句子」还是「句子口径不对」")
	})

	t.Run("当月族两条_仍然硬失败", func(t *testing.T) {
		// 两条当月总量句（10 月与 9 月），一条累计句都没有 ⇒ 落进当月族且有两条
		const body = "初步统计，2022年10月社会融资规模增量为9079亿元，比上年同期少7097亿元。" +
			"初步统计，2022年9月社会融资规模增量为3.53万亿元，比上年同期多6245亿元。"
		got, err := extractTSFFlowArticle(body)
		require.Error(t, err, "孪生句不许就近取一条——这条纪律对两族都成立，不是累计族专有")
		assert.Nil(t, got)
		assert.Contains(t, err.Error(), "（当月口径）matched 2 sentences")
		assert.Contains(t, err.Error(), "refusing to pick one")
	})

	t.Run("分项抽到一部分_必须报模板缺口而不是静默跳过", func(t *testing.T) {
		// 当月总量句 + **只有两个**分项（其余六个缺席）
		const body = "初步统计，2022年10月社会融资规模增量为9079亿元，比上年同期少7097亿元。" +
			"其中，对实体经济发放的人民币贷款增加4431亿元；委托贷款增加470亿元。"
		got, err := extractTSFFlowArticle(body)
		require.Error(t, err,
			"🔴 抽到一部分又缺另一部分是**模板缺口**，静默跳过会让字段无声消失")
		assert.Nil(t, got)
		assert.Contains(t, err.Error(), "只抽到 2/8 项")
		assert.Contains(t, err.Error(), "整族缺席是正常的",
			"错误要说清「整族缺席」与「部分缺失」是两回事，否则下一个人会把判据放松成静默跳过")
	})

	t.Run("整族缺席_正常放行只写总量", func(t *testing.T) {
		// 与上一格的**最小对**：分项一个都没有 ⇒ 该族只有总量，正常
		const body = "初步统计，2022年10月社会融资规模增量为9079亿元，比上年同期少7097亿元。"
		got, err := extractTSFFlowArticle(body)
		require.NoError(t, err, "整族缺席是正常体例（形态②的累计句就是这样孤悬段末）")
		assert.InDelta(t, 9079.0, got[FieldTSFFlowMoM], 1e-9)
		assert.Len(t, got, 1, "只有总量，一个分项都不该有")
	})
}

// 🔴 **两族都没有时，整节必须失败，不得静默产出一份半空的 Values**（DoD functional[1]）。
//
// 路由化最容易滑向的形态是「取不到就跳过」：合计句两族都没有时如果只是不写那一列，
// 一份只有余额、没有任何流量字段的 Values 会被当成成功结果送进下游标定——而
// 「字段缺失」在 map 里与「这一期没有这个数」完全同形。
//
// ⚠️ 端到端测（而不是只测 selectRMBFlowByCaliber）：错误在函数里产生是一回事，
// **调用方有没有把它传上去**是另一回事。少了这一条，一个把 `if err != nil` 写成
// `_ = err` 的实现照样让单元测试全绿。
func TestSectionFailsLoudlyWhenNoFlowSentenceAtAll(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		fn         func(section) (map[string]float64, error)
		balance    string
	}{
		{"存款侧", "8月末，人民币存款余额301.6万亿元，同比增长10.5%。",
			extractDepositSection, FieldDepositBalance},
		{"贷款侧", "8月末，人民币贷款余额232.28万亿元，同比增长11.1%。",
			extractLoanSection, FieldLoanBalance},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// 前提：余额句本身是好的 —— 否则失败会来自更早一步，测不到本条
			require.Contains(t, tc.body, "余额")

			got, err := tc.fn(section{Body: tc.body})
			require.Error(t, err, "一条合计句都没有 ⇒ 必须响亮失败")
			assert.Contains(t, err.Error(), "累计与当月两族都没有",
				"措辞要说清是「两族都没有」，与「只有当月」「前缀不认识」都不同")
			assert.NotContains(t, got, tc.balance,
				"报错时不得返回半份 Values —— 只有余额没有流量的观测会被下游当成完整结果")
		})
	}
}
