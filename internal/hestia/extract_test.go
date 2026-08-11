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
// sections.go 里那两个字串是**字面量**（T3 的文件，本任务不改它）。若两处哪天
// 分叉，extractFields 会安静地按 v1 去抽一份 v2 报告——少抽 27 个字段，而
// 「缺失」在 M1b-1 的语义里是「本期模板本就没有」，完全无声。
func TestExtractorConstantsMatchDetect(t *testing.T) {
	for _, tc := range []struct{ sample, want string }{
		{"pboc-2025-12-annual.html", extractorV2},
		{"pboc-2020-06-h1.html", extractorV1},
	} {
		t.Run(tc.sample, func(t *testing.T) {
			secs := splitSections(stripHTML(readSample(t, tc.sample)))
			got, err := detectExtractor(secs)
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
