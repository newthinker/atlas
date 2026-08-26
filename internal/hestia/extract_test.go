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
				once("社融增量总量", flow, tsfFlowTotalRE)
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
	for f, share := range map[string]float64{
		FieldTSFStockEntrust: 2.6, FieldTSFStockFXLoan: 0.3,
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
		name    string
		body    string
		wantErr string // 非空则要求报错且错误信息含它
		wantYTD float64
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
			// 报告本身不含累计数据 ⇒ 响亮失败，不拿 14200 冒充 ytd
			name: "类3 仅为_非1月", body: tsfFlowBodyNonJanuaryBare,
			wantErr: "2020年10月/单月",
		},
		{
			// 分项有两套，必须取累计段那套（4.21万亿元=42100），不是当月的 7202
			name: "类4 两者都有_累计段在前", body: tsfFlowBodyBothCumulativeFirst,
			wantYTD: 59200, wantRMBLoan: 42100,
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
			require.Len(t, got, 9)
			assert.InDelta(t, tc.wantYTD, got[FieldTSFFlowYTD], 1e-9)
			assert.InDelta(t, tc.wantRMBLoan, got[FieldTSFFlowRMBLoanYTD], 1e-9)
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
func TestTSFFlowArticleRefusesCaliberMix(t *testing.T) {
	got, err := extractTSFFlowArticle(tsfFlowBodyBothMonthlyFirst)
	require.Error(t, err, "累计总量配当月分项必须失败，而不是产出口径混装的 Values")
	assert.Nil(t, got)

	// 钉住**是哪条断言**在守卫：必须是分项在累计作用域内找不到，
	// 而不是别的原因碰巧也让它红。
	assert.Contains(t, err.Error(), "社融增量分项",
		"失败必须来自分项抽取，说明作用域切分生效了")
	assert.Contains(t, err.Error(), "对实体经济发放的人民币贷款")

	// 反向钉住：这一篇的两个值都在原文里，只是分属不同口径。若哪天实现退回
	// 「总量全篇找、分项全篇找」，err 会变 nil 而上面的断言全部失效——所以
	// 这里额外确认那对混装值确实是危险的（量级差 ~30 倍，都在合法区间内）。
	assert.Contains(t, tsfFlowBodyBothMonthlyFirst, "累计为28.7万亿元")
	assert.Contains(t, tsfFlowBodyBothMonthlyFirst, "人民币贷款增加4431亿元")
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
	assert.Contains(t, err.Error(), "matched 2 cumulative sentences",
		"必须说清是「两条都算累计」，而不是笼统的抽取失败")
	assert.Contains(t, err.Error(), "refusing to pick one")

	// 对照：只留一条时同一段文本是能抽通的——否则上面的 Error 可能来自别的原因，
	// 这条断言就不再是在测「拒绝二义」了。
	ok, err := extractTSFFlowArticle(tsfFlowBodyCumulativeOnly)
	require.NoError(t, err)
	require.Len(t, ok, 9)
}
