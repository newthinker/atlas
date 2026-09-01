package hestia

// Context Checkpoint: done_criteria → test mapping (golden)
//
// functional[0]     golden2025 恰好 54 项（全部累计口径字段）/ golden2020 恰好 27 项
//                                                 → TestGoldenTablesAreWellFormed
// functional[0]     goldenMeta* 只填 5 项，ArticleID 与 IngestedAt 留空
//                                                 → TestGoldenMetaIsValid
// functional[1]     每个键都在 allFields 内，且逐键断言（非只比数量）
//                                                 → TestGoldenTablesAreWellFormed
// boundary[0]       2020 恰好是「全部非社融字段」，27 项不是抄漏
//                                                 → TestGoldenTablesAreWellFormed
// error_handling[0] 单位换算量级守护（余额 vs 增量差 10000 倍）
//                                                 → TestGoldenUnitsMatchFieldClass
// non_functional[0] 手工抄录——**无法用测试守住**，见下方「抄录方式」注释；
//                   机械化的那一半是「先于实现 commit 落盘且此后不再修改」。
//                   ⚠️ 不要把 fields_test.go 的 TestFieldNamesAppearOnlyInFieldsGo
//                   记作本文件的守护：它在 fields_test.go:193 明文
//                   `strings.HasSuffix(name, "_test.go") → continue` 豁免了测试文件，
//                   因此对 golden_test.go 一行都不检查。
// non_functional[1] 只依赖 M1b-1 已有的 allFields / Meta / validate，
//                   在 T2–T6 全部未完成时即可编译——本文件能编译本身就是证据。

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// —— 抄录方式（供验证者抽查）——
//
// 两表共 81 个数值逐条抄自 testdata 下的原文 HTML，**不由解析器生成**——那样
// 就是拿实现验证实现，T7 的端到端比对会退化成恒真断言。
//
// 每个条目的行末/行上注释给出原文的**出处段落与原句**，抽查方式：
// 打开 testdata/pboc-2025-12-annual.html 的对应行，核对引号内的字串与数值。
//
//	golden2025 → testdata/pboc-2025-12-annual.html
//	  一、社融存量      L320      二、社融增量  L323     三、货币供应量 L325
//	  四、存款          L327-L328 五、贷款      L331-L332
//	  六、利率          L336      七、外汇      L338
//	golden2020 → testdata/pboc-2020-06-h1.html
//	  一、货币供应量 L320  二、贷款 L322-L323  三、存款 L326-L327
//	  四、利率       L331  五、外汇 L333
//
// 单位按 fields.go 的约定归一：余额/存量→万亿元，增量→亿元，同比/利率→百分数。
// 原文单位与之不同的条目，注释里标出原文值（如 `15.91 万亿`），换算只有 ×10000
// 一种（万亿元→亿元）。

// golden2025 是 pboc-2025-12-annual.html 的期望解析结果，54 项全满（累计口径）。
//
// M1c-4 的 TASK-004 之后 fieldOrder 是 76 项，多出的 22 个是当月口径列（*_mom）。
// 这份年报正文只有累计句，那 22 列在它这里是 absent-by-design，本表不该有它们
// —— 由 TestGoldenTablesAreWellFormed 的正反两条子测试钉住。
var golden2025 = map[string]float64{
	// —— A.1 社融总量（3）——
	// L320「初步统计，2025年末社会融资规模存量为442.12万亿元，同比增长8.3%」
	FieldTSFStock:    442.12,
	FieldTSFStockYoY: 8.3,
	// L323「2025年全年社会融资规模增量累计为35.6万亿元」原文 35.6 万亿 → 亿元
	FieldTSFFlowYTD: 356000,

	// —— A.1 社融存量分项（16）—— 全部出自 L320 的「其中」分句，
	// 句式统一为「X余额为N万亿元，同比增长/下降M%」，「下降」取负号。
	FieldTSFStockRMBLoan:       268.4, // 对实体经济发放的人民币贷款
	FieldTSFStockRMBLoanYoY:    6.3,
	FieldTSFStockFXLoan:        1.05, // 对实体经济发放的外币贷款折合人民币
	FieldTSFStockFXLoanYoY:     -18,  // 「同比下降18%」——原文无小数
	FieldTSFStockEntrust:       11.35,
	FieldTSFStockEntrustYoY:    1.3,
	FieldTSFStockTrust:         4.67,
	FieldTSFStockTrustYoY:      8.6,
	FieldTSFStockBankAccept:    2.15, // 未贴现的银行承兑汇票
	FieldTSFStockBankAcceptYoY: -0.3, // 「同比下降0.3%」
	FieldTSFStockCorpBond:      34.24,
	FieldTSFStockCorpBondYoY:   6, // 「同比增长6%」——原文无小数
	FieldTSFStockGovtBond:      94.92,
	FieldTSFStockGovtBondYoY:   17.1,
	FieldTSFStockEquity:        12.2, // 非金融企业境内股票
	FieldTSFStockEquityYoY:     4.1,

	// —— A.1 社融增量分项（8）—— 全部出自 L323 的「其中」分句。
	// 注意这些分句的动词各不相同：增加 / 减少 / 净融资 / 融资，
	// 且每句后半截都跟着一个「同比多增N」的干扰数值，不要抄成它。
	FieldTSFFlowRMBLoanYTD:    159100, // 「增加15.91万亿元」（干扰：同比少增1.13万亿）
	FieldTSFFlowFXLoanYTD:     -2043,  // 「折合人民币减少2043亿元」（干扰：同比少减1873亿）
	FieldTSFFlowEntrustYTD:    1203,   // 「委托贷款增加1203亿元」（干扰：同比多增1780亿）
	FieldTSFFlowTrustYTD:      3682,   // 「信托贷款增加3682亿元」（干扰：同比少增294亿）
	FieldTSFFlowBankAcceptYTD: 112,    // 「增加112亿元」（干扰：同比多增3405亿）
	FieldTSFFlowCorpBondYTD:   23900,  // 「企业债券净融资2.39万亿元」
	FieldTSFFlowGovtBondYTD:   138400, // 「政府债券净融资13.84万亿元」
	FieldTSFFlowEquityYTD:     4763,   // 「非金融企业境内股票融资4763亿元」

	// —— A.2 货币供应量（6）——
	// L325「广义货币(M2)余额340.29万亿元，同比增长8.5%。狭义货币(M1)余额
	//       115.51万亿元，同比增长3.8%。流通中货币(M0)余额14.13万亿元，同比增长10.2%」
	FieldM2:    340.29,
	FieldM2YoY: 8.5,
	FieldM1:    115.51,
	FieldM1YoY: 3.8,
	FieldM0:    14.13,
	FieldM0YoY: 10.2,

	// —— A.3 存款（7）——
	// L327「本外币存款余额336.14万亿元，同比增长9%。月末人民币存款余额328.64万亿元，
	//       同比增长8.7%」——取人民币口径，锚点必须含「人民币」，
	//       否则会抓到紧邻其前的本外币 336.14 / 9%。
	FieldDepositBalance:    328.64,
	FieldDepositBalanceYoY: 8.7,
	// L328「全年人民币存款增加26.41万亿元。其中，住户存款增加14.64万亿元，
	//       非金融企业存款增加2.31万亿元，财政性存款增加6579亿元，
	//       非银行业金融机构存款增加6.41万亿元」
	FieldDepositFlowYTD:      264100, // 26.41 万亿
	FieldDepositHouseholdYTD: 146400, // 14.64 万亿
	FieldDepositCorpYTD:      23100,  // 2.31 万亿
	FieldDepositFiscalYTD:    6579,   // 与前后同句，原文单位却是亿元
	FieldDepositNBFIYTD:      64100,  // 6.41 万亿

	// —— A.4 贷款（10）——
	// L331「本外币贷款余额275.74万亿元，同比增长6.2%。月末人民币贷款余额271.91万亿元，
	//       同比增长6.4%」——同存款，取人民币口径，本外币是干扰项。
	FieldLoanBalance:    271.91,
	FieldLoanBalanceYoY: 6.4,
	// L332「全年人民币贷款增加16.27万亿元。分部门看，住户贷款增加4417亿元，其中，
	//       短期贷款减少8351亿元，中长期贷款增加1.28万亿元；企（事）业单位贷款增加
	//       15.47万亿元，其中，短期贷款增加4.81万亿元，中长期贷款增加8.82万亿元，
	//       票据融资增加1.66万亿元；非银行业金融机构贷款减少1103亿元」
	//
	// 「短期贷款」「中长期贷款」在同一句里出现两次，分属住户与企业两个作用域，
	// 分号是作用域边界——认错作用域会把住户的 -8351 记成企业的。
	// 另注：「住户贷款增加4417亿元」没有对应字段（schema 无 loan_hh_total），不抄。
	FieldLoanFlowYTD:      162700, // 16.27 万亿
	FieldLoanHHShortYTD:   -8351,  // 住户作用域内「短期贷款减少8351亿元」
	FieldLoanHHMLTYTD:     12800,  // 住户作用域内「中长期贷款增加1.28万亿元」
	FieldLoanCorpTotalYTD: 154700, // 15.47 万亿
	FieldLoanCorpShortYTD: 48100,  // 企业作用域内「短期贷款增加4.81万亿元」
	FieldLoanCorpMLTYTD:   88200,  // 企业作用域内「中长期贷款增加8.82万亿元」
	FieldLoanBillYTD:      16600,  // 1.66 万亿
	FieldLoanNBFIYTD:      -1103,  // 「非银行业金融机构贷款减少1103亿元」

	// —— A.5 利率与外部（4）——
	// L336「12月份同业拆借加权平均利率为1.36%，分别比上月和上年同期低0.06个和
	//       0.21个百分点。质押式回购加权均利率为1.4%，分别比上月和上年同期低
	//       0.04个和0.25个百分点」
	//
	// ⚠️ 本篇正文写的是「加权**均**利率」（缺「平」字），而同篇小标题 L334 写的是
	// 「质押式债券回购月加权平均利率」——2020 篇正文 L331 则有「平」。
	// 按「加权平均利率」死锚会在本篇漏掉 rate_repo，T5 需容忍这个错字。
	//
	// 每句紧跟两个「低0.06个和0.21个百分点」的干扰值，要 1.36 / 1.4 而非它们。
	FieldRateIBO:   1.36,
	FieldRateRepo:  1.4,
	FieldFXReserve: 3.36,   // L338「国家外汇储备余额为3.36万亿美元」
	FieldFXRate:    7.0288, // L338「人民币汇率为1美元兑7.0288元人民币」
}

// golden2020 是 pboc-2020-06-h1.html 的期望解析结果，27 项。
//
// 只有 27 项——那期报告只有六个板块、不含社会融资规模（当时社融单独发布）。
// tsf_* 一个都不该出现：它们是「本期模板本就没有」，不是「解析漏了」。
// 这一区别正是 M1b-1 的 absent 语义（Values 键不存在即缺失）要表达的东西。
var golden2020 = map[string]float64{
	// —— A.2 货币供应量（6）——
	// L320「广义货币(M2)余额213.49万亿元,同比增长11.1%，增速与上月末持平，
	//       比上年同期高2.6个百分点；狭义货币(M1)余额60.43万亿元，同比增长6.5%，
	//       增速比上月末低0.3个百分点，比上年同期高2.1个百分点；
	//       流通中货币(M0)余额7.95万亿元,同比增长9.5%」
	//
	// ⚠️ 原文这一段的「万亿元」后跟的是**半角逗号** ","（M2 与 M0 两处），
	// 与全篇其余的全角「，」不同；按全角逗号分句会把 M2 与其同比切不开。
	// 每个同比后面都拖着 2.6 / 0.3 / 2.1 个百分点的干扰值，要前者。
	FieldM2:    213.49,
	FieldM2YoY: 11.1,
	FieldM1:    60.43, // ⚠️ 旧口径，不含个人活期存款——与 2025 的 M1 不可比
	FieldM1YoY: 6.5,
	FieldM0:    7.95,
	FieldM0YoY: 9.5,

	// —— A.3 存款（7）——
	// L326「本外币存款余额212.99万亿元，同比增长10.5%。月末人民币存款余额207.48
	//       万亿元，同比增长10.6%」——取人民币口径，本外币是干扰项。
	FieldDepositBalance:    207.48,
	FieldDepositBalanceYoY: 10.6,
	// L327「上半年人民币存款增加14.55万亿元，同比多增4.5万亿元。其中，住户存款增加
	//       8.33万亿元，非金融企业存款增加5.28万亿元，财政性存款增加4384亿元，
	//       非银行业金融机构存款减少7446亿元」
	FieldDepositFlowYTD:      145500, // 14.55 万亿（干扰：同比多增4.5万亿）
	FieldDepositHouseholdYTD: 83300,  // 8.33 万亿
	FieldDepositCorpYTD:      52800,  // 5.28 万亿
	FieldDepositFiscalYTD:    4384,
	FieldDepositNBFIYTD:      -7446, // 「非银行业金融机构存款减少7446亿元」

	// —— A.4 贷款（10）——
	// L322「本外币贷款余额171.32万亿元，同比增长13%。月末人民币贷款余额165.2万亿元，
	//       同比增长13.2%」——取人民币口径，本外币是干扰项。
	FieldLoanBalance:    165.2,
	FieldLoanBalanceYoY: 13.2,
	// L323「上半年人民币贷款增加12.09万亿元，同比多增2.42万亿元。分部门看，住户部门
	//       贷款增加3.56万亿元，其中，短期贷款增加7552亿元，中长期贷款增加2.8万亿元；
	//       企（事）业单位贷款增加8.77万亿元，其中，短期贷款增加2.82万亿元，中长期
	//       贷款增加4.86万亿元，票据融资增加9697亿元;非银行业金融机构贷款减少2775亿元」
	//
	// ⚠️ 两处与 2025 篇的措辞差异：作用域词是「住户**部门**贷款」（2025 是「住户贷款」）；
	// 「票据融资增加9697亿元**;**非银行业」用的是**半角分号**，而作用域边界正是分号。
	// 句尾还有「6月份，人民币贷款增加1.81万亿元」——单月数据，本期次是 h1，不抄。
	FieldLoanFlowYTD:      120900, // 12.09 万亿（干扰：同比多增2.42万亿、6月单月1.81万亿）
	FieldLoanHHShortYTD:   7552,   // 住户作用域内「短期贷款增加7552亿元」
	FieldLoanHHMLTYTD:     28000,  // 住户作用域内「中长期贷款增加2.8万亿元」
	FieldLoanCorpTotalYTD: 87700,  // 8.77 万亿
	FieldLoanCorpShortYTD: 28200,  // 企业作用域内「短期贷款增加2.82万亿元」
	FieldLoanCorpMLTYTD:   48600,  // 企业作用域内「中长期贷款增加4.86万亿元」
	FieldLoanBillYTD:      9697,
	FieldLoanNBFIYTD:      -2775, // 「非银行业金融机构贷款减少2775亿元」

	// —— A.5 利率与外部（4）——
	// L331「6月份同业拆借加权平均利率为1.85%，分别比上月和上年同期高0.6个和0.15个
	//       百分点；质押式回购加权平均利率为1.89%，分别比上月和上年同期高0.6个和
	//       0.15个百分点」——本篇「加权平均利率」有「平」字，与 2025 篇不同。
	FieldRateIBO:   1.85,
	FieldRateRepo:  1.89,
	FieldFXReserve: 3.11,   // L333「国家外汇储备余额为3.11万亿美元」
	FieldFXRate:    7.0795, // L333「人民币汇率为1美元兑7.0795元人民币」
}

// goldenMeta2025 只填 Parse 应产出的 5 项，见 TestGoldenMetaIsValid 的注释。
var goldenMeta2025 = Meta{
	Period:         "2025-12",
	PeriodType:     "annual",
	PublishedAt:    "2026-01-15", // L18 <meta name="PubDate" content="2026-01-15">
	CaliberVersion: "2025-01",    // period ≥ 2025-01：原文注 5 的 M1 口径修订
	Extractor:      "rule@v2",    // 八板块含社融
}

var goldenMeta2020 = Meta{
	Period:         "2020-06",
	PeriodType:     "h1",
	PublishedAt:    "2020-07-10", // L18 <meta name="PubDate" content="2020-07-10">
	CaliberVersion: "2015-01",    // period < 2023-01
	Extractor:      "rule@v1",    // 六板块无社融
}

// ytdCaliberFields 是 fieldOrder 中**累计口径**的字段（M1c-4 的 TASK-004）。
//
// 两份 golden 都抄自累计口径的报告（2025 年报 / 2020 上半年报），当月口径的
// *_mom 列在它们的正文里根本不存在 —— 与 golden2020 不含社融字段完全同理，
// 是「本期模板本就没有」，不是抄漏。
//
// 同样用**派生**而不是写死：*_mom 那一族是 TASK-004 一次加进来的，将来若有当月
// 口径的 golden 表，它用的是本函数的补集，两边不会分叉。
func ytdCaliberFields() []string {
	var out []string
	for _, f := range fieldOrder {
		if !strings.HasSuffix(f, "_mom") {
			out = append(out, f)
		}
	}
	return out
}

// nonTSFFieldCount 是 fieldOrder 中不属于社融板块的字段数。
//
// 它就是 golden2020 应有的项数：2020H1 报告只有六个板块、不含社会融资规模
// （当时社融单独发布），所以「27 项」是**本期模板本就没有**，不是抄漏。
// 用「fieldOrder 减去 tsf_*」算出来而不是写死 27，是为了让 schema 增删字段时
// 这个期望值自动跟随——写死的 27 会在下次加字段时变成一条说谎的断言。
func nonTSFFields() []string {
	var out []string
	for _, f := range ytdCaliberFields() {
		if !strings.HasPrefix(f, "tsf_") {
			out = append(out, f)
		}
	}
	return out
}

// TestGoldenTablesAreWellFormed 是对期望值本身的检查。
//
// golden 表是后面每个任务的验收基准，它自己错了会让所有测试一起说谎——
// 所以先钉住它的形状：项数、键的合法性、v1 不含社融。
func TestGoldenTablesAreWellFormed(t *testing.T) {
	require.Len(t, golden2025, 54, "rule@v2 期次应填满全部 54 个累计口径字段")
	require.Len(t, golden2020, 27, "rule@v1 期次只有六板块，恰好 27 个字段")

	// 派生表达式与上面两个字面量互为对照：只写派生，两边一起错时全绿；只写
	// 字面量，加一个 *_ytd 字段却忘了抄进 golden 时它照样绿（M1c-4 的 TASK-004）。
	require.Len(t, ytdCaliberFields(), 54,
		"累计口径字段数变了 ⇒ golden2025 要么该跟着抄，要么新字段该是当月口径")

	for _, tc := range []struct {
		name  string
		table map[string]float64
	}{
		{"2025", golden2025},
		{"2020", golden2020},
	} {
		t.Run(tc.name+"/键都在白名单内", func(t *testing.T) {
			for k := range tc.table {
				assert.Truef(t, allFields[k], "键 %q 不在 fieldOrder 中", k)
			}
		})
	}

	// 逐键断言而非只比数量：数量对而键写错（抄成另一个合法字段名）在上面的
	// Len 与白名单检查里都不会红，要到 T5/T7 才暴露，且那时会被误读成抽取错误。
	t.Run("2025/54 个累计口径字段一个不缺", func(t *testing.T) {
		for _, f := range ytdCaliberFields() {
			assert.Containsf(t, golden2025, f, "字段 %q 未抄录", f)
		}
		// 反向：当月口径列一个都不该在里面。2025 年报正文只有累计句，
		// 抄进来的必然是把累计值装进了 *_mom —— 那正是本迭代最想防的那件事。
		for k := range golden2025 {
			assert.Falsef(t, strings.HasSuffix(k, "_mom"),
				"2025 年报是累计口径，不该有当月口径字段，但出现了 %q", k)
		}
	})

	// golden2020 必须**恰好**是全部非社融字段：这同时钉住了两件事——
	// 27 项不是抄漏（该有的一个不少），且没有社融字段混进来（不该有的一个不多）。
	t.Run("2020/恰好是全部非社融字段", func(t *testing.T) {
		for _, f := range nonTSFFields() {
			assert.Containsf(t, golden2020, f, "非社融字段 %q 未抄录，27 项对不上", f)
		}
		for k := range golden2020 {
			assert.Falsef(t, strings.HasPrefix(k, "tsf_"),
				"2020 期次不该有社融字段，但出现了 %q", k)
		}
	})
}

// TestGoldenUnitsMatchFieldClass 守护单位换算。
//
// 余额记万亿元、增量记亿元，两者差 10000 倍。抄录时照搬原文单位（把增量
// 「2.39万亿元」记成 2.39 而非 23900）不会被上面任何一条形状检查发现，
// 要到 T7 端到端才红，而那时根因在这里、现场却在解析器——正是 DoD
// error_handling[0] 点名要避免的误诊。
//
// 只能拦量级错误，拦不住抄错数字（264100 抄成 264200 照样通过）。
// 后者没有机械化手段，只能靠上面的逐条原文出处供人抽查。
func TestGoldenUnitsMatchFieldClass(t *testing.T) {
	for _, tc := range []struct {
		name  string
		table map[string]float64
	}{
		{"2025", golden2025},
		{"2020", golden2020},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.table {
				switch {
				case strings.HasSuffix(k, "_yoy"), strings.HasPrefix(k, "rate_"):
					// 百分数：央行报告里的同比与利率都在两位数以内
					assert.LessOrEqualf(t, abs(v), 100.0,
						"%q=%v 像是没归一成百分数", k, v)
				case strings.HasSuffix(k, "_ytd"):
					// 增量记亿元。全样本最小的是 2025 的未贴现银行承兑汇票
					// 112 亿元，所以 <100 只可能是漏乘 10000。
					assert.GreaterOrEqualf(t, abs(v), 100.0,
						"%q=%v 疑似仍是万亿元，增量应记亿元（×10000）", k, v)
				case k == FieldFXRate:
					// 汇率既非余额也非增量，1 美元兑人民币在个位数
					assert.Lessf(t, v, 100.0, "%q=%v 不像汇率", k, v)
				default:
					// 余额/存量记万亿元。最大的是社融存量 442.12，
					// 四位数只可能是照搬了亿元。
					assert.Lessf(t, abs(v), 1000.0,
						"%q=%v 疑似仍是亿元，余额应记万亿元（÷10000）", k, v)
				}
			}
		})
	}
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

// TestGoldenMetaIsValid 确认两份期望 Meta 能通过 M1b-1 的校验。
//
// 只填 5 项：ArticleID 是 URL 的属性由 M1b-4 填，IngestedAt 由 Store.Save
// 覆写。这两项若在 golden 里填了值，golden Meta 就永远不可能与 Parse 的输出
// 相等——留空是 T7 能比对的前提，不是遗漏。
//
// 补上 ArticleID 才调 validate：parse 按设计不填它，而 validate 要求它非空，
// 这一步同时验证了那条边界。
func TestGoldenMetaIsValid(t *testing.T) {
	for _, tc := range []struct {
		name string
		meta Meta
	}{
		{"2025", goldenMeta2025},
		{"2020", goldenMeta2020},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Empty(t, tc.meta.ArticleID, "ArticleID 是 URL 属性，由 M1b-4 填")
			require.Empty(t, tc.meta.IngestedAt, "IngestedAt 由 Store.Save 覆写")

			m := tc.meta
			m.ArticleID = "placeholder-filled-by-m1b4"
			require.NoError(t, m.validate(), "其余 5 项应已填齐且合法")
		})
	}
}
