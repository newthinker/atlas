package hestia

import "regexp"

// 本文件是**数据**：字段清单表与句式模板。抽取流程在 extract.go。
//
// 分工的判据是「新增一个字段要不要改正则」——不需要，才算分开了。八个社融
// 存量分项共用一条 tsfStockRE，加第九项只是往 tsfStockItems 添一行。

// —— 三个共用片段 ——
//
// dirPat / unitPat 由 amount.go 的 directionAlt / unitAlt 拼出，**本文件不另写一份
// 词表**：两份词表迟早分叉，而分叉的表现是某个字段静默取不到值。
//
// numPat 不允许前导正负号——符号一律从方向词读（见 amount.go 的
// parseUnsignedNumber）。
const (
	numPat  = `([0-9]+(?:\.[0-9]+)?)`
	dirPat  = `(` + directionAlt + `)`
	unitPat = `(` + unitAlt + `)`
)

// —— 口径限定词 ——
//
// 存/贷板块里每类余额都有**三**句同形句子，且本外币在前：
//
//	12月末，本外币存款余额336.14万亿元，同比增长9%。
//	月末人民币存款余额328.64万亿元，同比增长8.7%。
//	12月末，外币存款余额1.07万亿美元，同比增长25%。
//
// 我们要的是中间那句。做法**不是**指望 `人民币存款余额` 这个子串恰好不出现在
// `本外币存款余额` 里——那是巧合，把守卫去掉也照样通过（reviewer G8 实证）。
// 做法是把限定词做成显式捕获组：三句都匹配，再**按限定词的值**挑，并要求唯一。
//
// 好处是失效模式全都变响：只剩本外币句时人民币命中数为 0（报错，而不是悄悄
// 取到 336.14）；某期多出一句人民币余额时命中数为 2（报错，而不是最左优先
// 静默选一个）。
//
// 外币句的单位是**美元**（万亿美元 / 亿美元），归一成浮点后与人民币完全无法
// 区分——`1.07 万亿美元` 与 `1.07 万亿元` 的 toWanYi() 完全相等。所以币种只能
// 在捕获组这一层判，不能在数值那一层判。
const (
	currencyAlt = `本外币|外币|人民币`
	currencyPat = `(` + currencyAlt + `)`

	currencyRMB = "人民币"
)

// —— 期次前缀 ——
//
// 2020 贷款板块同一段里有两句都能被「人民币贷款+方向词+数值」命中：
//
//	上半年人民币贷款增加12.09万亿元   ← 期内合计，要它
//	6月份，人民币贷款增加1.81万亿元   ← 单月，不要它
//
// 计划书只用 `人民币贷款(dir)(num)(unit)` + 最左优先，**碰巧**选中了对的那句；
// 两句顺序对调就会取到 1.81。把期次前缀做成显式捕获组后与文本顺序无关。
//
// 认不出的期次前缀（如将来的「前三季度」）会让整条正则不命中 ⇒ 报错。那是
// 期望行为：形态不认识就响亮失败，而不是套用已知模板去抽一份没见过的报告。
const (
	periodAlt = `全年|上半年|[0-9]{1,2}月份`
	periodPat = `(` + periodAlt + `)`
)

// cumulativePeriods 是「期内累计」口径的前缀集合，与单月相对。
//
// 外币孪生句的前缀同样是全年/上半年（「全年外币存款增加2135亿美元」），所以
// 期次与口径**两个维度都要判**，只判其一都会漏。
var cumulativePeriods = map[string]bool{
	"全年":  true,
	"上半年": true,
}

// —— 模板 1：社融存量分项（8 项 × 余额 + 同比 = 16 字段）——
//
// 「对实体经济发放的人民币贷款余额为268.4万亿元，同比增长6.3%」
//
// 「余额为?」：货币板块写「余额340.29」，社融写「余额为268.4」，一个字之差。
//
// 名称一律 QuoteMeta 固定，**不做名称捕获**——贪婪的 `(.+)` 会把
// 「企业债券净融资」切成 name=`企业债券净`，静默漏一个字段（T4 实测）。
type tsfStockItem struct {
	name         string
	balanceField string
	yoyField     string
}

var tsfStockItems = []tsfStockItem{
	{"对实体经济发放的人民币贷款", FieldTSFStockRMBLoan, FieldTSFStockRMBLoanYoY},
	{"对实体经济发放的外币贷款折合人民币", FieldTSFStockFXLoan, FieldTSFStockFXLoanYoY},
	{"委托贷款", FieldTSFStockEntrust, FieldTSFStockEntrustYoY},
	{"信托贷款", FieldTSFStockTrust, FieldTSFStockTrustYoY},
	{"未贴现的银行承兑汇票", FieldTSFStockBankAccept, FieldTSFStockBankAcceptYoY},
	{"企业债券", FieldTSFStockCorpBond, FieldTSFStockCorpBondYoY},
	{"政府债券", FieldTSFStockGovtBond, FieldTSFStockGovtBondYoY},
	{"非金融企业境内股票", FieldTSFStockEquity, FieldTSFStockEquityYoY},
}

// tsfStockRE 要求「余额」后紧跟数字与单位，因此不会命中同板块第二段的占比句
// 「委托贷款余额占比2.6%，同比低0.1个百分点」——那句「余额」后是「占」。
func tsfStockRE(name string) *regexp.Regexp {
	return regexp.MustCompile(
		regexp.QuoteMeta(name) + `余额为?` + numPat + unitPat + `，同比` + dirPat + numPat + `%`)
}

// —— 模板 2：社融增量分项（8 字段）——
//
// 「对实体经济发放的人民币贷款增加15.91万亿元」
// 「企业债券净融资2.39万亿元」   ← 方向词是「净融资」
// 「非金融企业境内股票融资4763亿元」← 方向词是「融资」
//
// 名称与存量分项不完全相同：存量叫「非金融企业境内股票」，增量句里那个「融资」
// 既像名称尾巴也是方向词——这里把它交给 dirPat 匹配。
//
// 每个分项后面都跟着一个同比干扰值（「同比多增1780亿元」「同比少减1873亿元」
// 「同比多4825亿元」）。模板到单位就收尾，且「多增/少增/少减」**不在**方向词
// 白名单内（见 amount.go），两道都挡着它。
// nameField 是「名称 → 字段」的清单条目，社融增量、存款分部门与贷款作用域子项
// 三张表同形，共用一个类型。
type nameField struct{ name, field string }

var tsfFlowItems = []nameField{
	{"对实体经济发放的人民币贷款", FieldTSFFlowRMBLoanYTD},
	{"对实体经济发放的外币贷款折合人民币", FieldTSFFlowFXLoanYTD},
	{"委托贷款", FieldTSFFlowEntrustYTD},
	{"信托贷款", FieldTSFFlowTrustYTD},
	{"未贴现的银行承兑汇票", FieldTSFFlowBankAcceptYTD},
	{"企业债券", FieldTSFFlowCorpBondYTD},
	{"政府债券", FieldTSFFlowGovtBondYTD},
	{"非金融企业境内股票", FieldTSFFlowEquityYTD},
}

func tsfFlowRE(name string) *regexp.Regexp {
	return regexp.MustCompile(regexp.QuoteMeta(name) + dirPat + numPat + unitPat)
}

// —— 模板 3：货币供应量（3 项 × 余额 + 同比 = 6 字段）——
//
// 「广义货币(M2)余额340.29万亿元，同比增长8.5%」
//
// 2020 那份后面还跟着「增速与上月末持平，比上年同期高2.6个百分点」——模板到
// `%` 就收尾，那些干扰数字以「个百分点」结尾，不会被吃进来。
var moneyItems = []struct {
	name, code   string
	balanceField string
	yoyField     string
}{
	{"广义货币", "M2", FieldM2, FieldM2YoY},
	{"狭义货币", "M1", FieldM1, FieldM1YoY},
	{"流通中货币", "M0", FieldM0, FieldM0YoY},
}

func moneyRE(name, code string) *regexp.Regexp {
	return regexp.MustCompile(
		regexp.QuoteMeta(name) + `\(` + code + `\)余额` + numPat + unitPat +
			`，同比` + dirPat + numPat + `%`)
}

// —— 模板 4：存款分部门（4 字段）——
//
// 「住户存款增加14.64万亿元，非金融企业存款增加2.31万亿元，财政性存款增加6579亿元，
//
//	非银行业金融机构存款减少7446亿元」
//
// 四个部门名互不重叠，不需要作用域。
var depositItems = []nameField{
	{"住户存款", FieldDepositHouseholdYTD},
	{"非金融企业存款", FieldDepositCorpYTD},
	{"财政性存款", FieldDepositFiscalYTD},
	{"非银行业金融机构存款", FieldDepositNBFIYTD},
}

func sectorFlowRE(name string) *regexp.Regexp {
	return regexp.MustCompile(regexp.QuoteMeta(name) + dirPat + numPat + unitPat)
}

// —— extractor 版本标识 ——
//
// sections.go 的 detectExtractor 返回的是同样这两个字串的字面量。这里给它们
// 命名而不是在 extract.go 里再抄一遍，并由 TestExtractorConstantsMatchDetect
// 把两处绑起来——否则某天改了一处就会静默走到「按 v1 抽一份 v2 报告」。
const (
	extractorV1 = "rule@v1"
	extractorV2 = "rule@v2"
)

// —— 模板 5：贷款作用域内的子项 ——
//
// 「住户贷款增加4417亿元，其中，短期贷款减少8351亿元，中长期贷款增加1.28万亿元；
//
//	企（事）业单位贷款增加15.47万亿元，其中，短期贷款增加4.81万亿元，…」
//
// 「短期贷款」在两个作用域里各出现一次，指向不同字段——这是全篇唯一需要作用域
// 的地方，也是本任务存在的主要理由。
type loanScope struct {
	anchorRE   *regexp.Regexp // 作用域起点
	totalField string         // 该部门的合计字段（"" 表示合计不在 54 字段内）
	items      []nameField
}

var loanScopes = []loanScope{
	{
		// v1 写「住户部门贷款」，v2 写「住户贷款」——一条正则覆盖两版
		anchorRE:   regexp.MustCompile(`住户(?:部门)?贷款`),
		totalField: "", // 住户贷款合计不在 54 字段内（A.6 列为派生指标）
		items: []nameField{
			{"短期贷款", FieldLoanHHShortYTD},
			{"中长期贷款", FieldLoanHHMLTYTD},
		},
	},
	{
		anchorRE:   regexp.MustCompile(`企（事）业单位贷款`),
		totalField: FieldLoanCorpTotalYTD,
		items: []nameField{
			{"短期贷款", FieldLoanCorpShortYTD},
			{"中长期贷款", FieldLoanCorpMLTYTD},
			{"票据融资", FieldLoanBillYTD},
		},
	},
	{
		anchorRE:   regexp.MustCompile(`非银行业金融机构贷款`),
		totalField: FieldLoanNBFIYTD,
		items:      nil, // 无子项
	},
}

// —— 模板 6：带口径限定词的余额句 ——
//
// 三句同形句子全部匹配，由 extract.go 按限定词挑人民币那句并要求唯一。
// 捕获组：1=口径 2=数值 3=单位 4=方向词 5=同比。
var (
	depositBalanceRE = balanceRE("存款")
	loanBalanceRE    = balanceRE("贷款")
)

func balanceRE(kind string) *regexp.Regexp {
	return regexp.MustCompile(
		currencyPat + kind + `余额` + numPat + unitPat + `，同比` + dirPat + numPat + `%`)
}

// —— 模板 7：带期次前缀的期内合计句 ——
//
// 捕获组：1=期次 2=口径 3=方向词 4=数值 5=单位。
// 「上半年人民币贷款增加…」无逗号，「6月份，人民币贷款增加…」有——故 `，?`。
var (
	depositFlowRE = flowRE("存款")
	loanFlowRE    = flowRE("贷款")
)

func flowRE(kind string) *regexp.Regexp {
	return regexp.MustCompile(
		periodPat + `，?` + currencyPat + kind + dirPat + numPat + unitPat)
}

// —— 模板 8：社融总量、利率与外汇 ——
var (
	// 社融增量总量：「2025年全年社会融资规模增量累计为35.6万亿元」
	tsfFlowTotalRE = regexp.MustCompile(`社会融资规模增量累计为` + numPat + unitPat)
	// 社融存量总量：「社会融资规模存量为442.12万亿元，同比增长8.3%」
	// 要求「存量」后紧跟「为」，故不会命中第二段的「占同期社会融资规模存量的60.7%」。
	tsfStockTotalRE = regexp.MustCompile(
		`社会融资规模存量为` + numPat + unitPat + `，同比` + dirPat + numPat + `%`)

	// 利率：2025 正文写「质押式回购加权**均**利率」（缺「平」字），2020 写
	// 「加权平均利率」。用 `平?` 容忍这一个字，**不能**放松成 `加权.*利率为`
	// ——同板块还有「同业拆借加权平均利率为1.36%」，与 1.4 同量级、同格式、
	// 同小数位，放松后会静默取到它，七道闸门一道都拦不住。
	//
	// 到 `%` 收尾，后面的「分别比上月和上年同期低0.06个和0.21个百分点」不会被吃进来。
	rateIBORE  = regexp.MustCompile(`同业拆借加权平?均利率为` + numPat + `%`)
	rateRepoRE = regexp.MustCompile(`质押式回购加权平?均利率为` + numPat + `%`)

	// 外汇：两句句式各不相同，且都没有方向词
	fxReserveRE = regexp.MustCompile(`国家外汇储备余额为` + numPat + `万亿美元`)
	fxRateRE    = regexp.MustCompile(`人民币汇率为1美元兑` + numPat + `元人民币`)
)

// scopeTotalRE 是作用域合计句的模板：锚点本身 + 方向词 + 数值 + 单位。
//
// 放在本文件而不是在 extract.go 里现拼，是为了让它和别的模板一样受
// TestNoGreedyCaptureInTemplates 约束——拼在别处的正则不在那道检查的视野内。
func scopeTotalRE(sc loanScope) *regexp.Regexp {
	return regexp.MustCompile(sc.anchorRE.String() + dirPat + numPat + unitPat)
}

// singletonFields 是不属于任何清单表、由单条模板各管一个的字段。
//
// 单列出来是为了让 profiles_test.go 的覆盖度检查能把它们算进去——否则
// 「加了字段没加规则」的守卫会在这十三个字段上失效。
func singletonFields() []string {
	return []string{
		FieldDepositBalance, FieldDepositBalanceYoY,
		FieldLoanBalance, FieldLoanBalanceYoY,
		FieldDepositFlowYTD, FieldLoanFlowYTD,
		FieldTSFStock, FieldTSFStockYoY, FieldTSFFlowYTD,
		FieldRateIBO, FieldRateRepo, FieldFXReserve, FieldFXRate,
	}
}

// allTemplateRegexps 枚举本文件生成的全部正则，供测试做统一的形态检查
// （当前是「不得含贪婪捕获」）。新增模板时请一并登记，否则它不受检查。
func allTemplateRegexps() []*regexp.Regexp {
	out := []*regexp.Regexp{
		depositBalanceRE, loanBalanceRE, depositFlowRE, loanFlowRE,
		tsfFlowTotalRE, tsfStockTotalRE, rateIBORE, rateRepoRE,
		fxReserveRE, fxRateRE,
	}
	for _, it := range tsfStockItems {
		out = append(out, tsfStockRE(it.name))
	}
	for _, it := range tsfFlowItems {
		out = append(out, tsfFlowRE(it.name))
	}
	for _, it := range moneyItems {
		out = append(out, moneyRE(it.name, it.code))
	}
	for _, it := range depositItems {
		out = append(out, sectorFlowRE(it.name))
	}
	for _, sc := range loanScopes {
		out = append(out, sc.anchorRE, scopeTotalRE(sc))
		for _, it := range sc.items {
			out = append(out, sectorFlowRE(it.name))
		}
	}
	return out
}
