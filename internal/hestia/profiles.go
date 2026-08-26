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
// 认不出的期次前缀会让整条正则不命中 ⇒ 报错。那是期望行为：形态不认识就响亮
// 失败，而不是套用已知模板去抽一份没见过的报告。
//
// # 季报两个前缀（M1b-4b / TASK-004 接上）
//
// 实测两份真实快照（各 8 板块、七个 sectionRules keyword 全命中 ⇒ detectExtractor
// 判 rule@v2，与年报同构）：一季度正文用 `一季度`、前三季度正文用 `前三季度`。
//
// 🔴 **这里绝不能写 `三季度`**。Go 的 regexp 是 leftmost-first：对 `前三季度人民币贷款…`，
// 若交替里有 `三季度` 而没有 `前三季度`，位置 0 的 `前` 匹不上、位置 1 的 `三` 匹得上
// ⇒ 捕获到 `三季度`，而 cumulativePeriods 里查不到它 ⇒ 报错。届时若为了让它绿而把
// `三季度` 也登记成累计口径，就等于断言「三季度 = 1–9 月累计」——**而那个判断没有
// 任何样本支持**：前三季度是 1–9 月累计，第三季度单季是 7–9 月，两者期末月同为 09
// 而月均除数是 9 与 3。实测佐证：q3 正文里 `三季度` 出现 14 次、`前三季度` 也是 14 次
// ——**每一次 `三季度` 都在 `前三季度` 内部**，不存在独立的「三季度」句。
//
// 写成 `前三季度` 后位置 0 直接匹中整个前缀，与交替内部顺序无关。
// TestProfileAlternationsHaveNoPrefixPairs 把「任何一项都不是另一项的前缀」守住。
//
// ⚠️ 判据是**前缀对**，**没有**扩成「子串对」——这不是遗漏，是实测后的决定：某轮
// reviewer 建议扩成子串对来防「写成 `三季度` 而非 `前三季度`」，实测表明它**对目标
// 缺陷全绿放行**（单个词写错构不成任何词对，交替内部的两两比对看不见它），**同时**
// 把既有安全的 `外币` ⊂ `本外币` 判红 —— 既挡不住真问题又恒响的假红，故不扩。
// 真正守住那个陷阱的是语料锚定的正向断言 TestFlowRECapturesQuarterlyPeriodVerbatim。
// （M1c-3a 的 TASK-005 订正：此处原先写的 `...HaveNoSubstringPairs` 这个测试从不存在，
// 它描述了一次**没有发生的改动**；把否定结论留在原地，免得下一个人再提一次同样的建议。）
//
// # 月报累计前缀（M1c-3a 的 TASK-001 接上）
//
// 月报正文的三种形态（Leader 实读 55 篇月报）：
//
//	前八个月人民币贷款增加…   ← 累计（*_ytd 要的），中文数字
//	8月份，人民币贷款增加…    ← 当月，阿拉伯数字，要排除
//	1月份人民币贷款增加…      ← **累计特例**：1 月的累计就是当月，仅 1 月报
//
// cumulativeMonthAlt **穷举**十项，不写成 `前[一二三四五六七八九十]+个月`：正则能
// 匹配的每一个累计前缀都必须在 cumulativePeriods 里有对应键，否则会出现「正则认得、
// 口径表不认」的组合——那种句子被匹中后当成单月句静默筛掉，表现为整篇命中 0。
// 穷举让这条对应关系可被**机械**核对，见 TestCumulativeMonthAltAndCumulativePeriodsAgree。
//
// 三/六/九在真实语料里不出现（那三个月走 q1/h1/q1_q3 报告），**仍然列上**：多一个
// 无害，漏一个是静默失效。⚠️ parse.go 注释里提到的「1-5月」这类带范围的形态在 55 篇里
// **一次都没出现**；实存的 2 篇 3 月报（2024-03 / 2025-03）用的是「一季度」，已在表内。
//
// 「前十一个月」排在「前十个月」前是零成本的防御，**但不是这里的安全性来源**：本形态下
// 四种排法（枚举×两序、提取公因式 `前(…|十一|十)个月`×两序）实测捕获**逐字相同**——
// Go 的 leftmost-first 是对**整体匹配**而言的，某个分支走到一半失败会退回去试下一个，
// 故「前十个月」这一支在「前十一个月…」上匹到 `前十` 后卡在 `个` vs `一` 即被放弃。
// 顺序真正有语义的前提是两个分支都能走完整条正则（`人民|人民币` 那类前缀对），这十项
// 两两无前缀关系，不构成那个前提。另实测：交替里**缺**某一项时结果是 NO MATCH
// （selectUnique 报 0 candidate sentence、响亮失败），不是静默捕获成相邻的短词。
const (
	cumulativeMonthAlt = `前两个月|前三个月|前四个月|前五个月|前六个月|` +
		`前七个月|前八个月|前九个月|前十一个月|前十个月`

	periodAlt = `全年|上半年|一季度|前三季度|` + cumulativeMonthAlt + `|[0-9]{1,2}月份`
	periodPat = `(` + periodAlt + `)`
)

// cumulativePeriods 是「期内累计」口径的前缀集合，与单月相对。
//
// 外币孪生句的前缀与本币句相同（年报「全年外币存款增加2135亿美元」，一季度
// 「一季度外币存款增加703亿美元」，前三季度「前三季度外币存款增加1658亿美元」
// ——三种期次实测都存在），所以期次与口径**两个维度都要判**，只判其一都会漏。
//
// 🔴 **本表与 periodAlt 是两个独立的硬卡点，不是一处**（reviewer B5 四格实测）：
// 只加 periodAlt 而不加本表 ⇒ 候选句从 1 涨到 3、但口径判定把它们全筛掉，命中 0；
// 只加本表而不加 periodAlt ⇒ 正则根本不命中，候选恒为 1、命中 0，**与现状逐字相同**
// （完全 no-op）。⇒ 消融要做 2×2，且两次单删的失败信息不同，见 profiles_test.go。
var cumulativePeriods = map[string]bool{
	"全年":   true,
	"上半年":  true,
	"一季度":  true,
	"前三季度": true,

	// M1c-3a 的 TASK-001：月报的累计前缀，与 cumulativeMonthAlt **一一对应**
	// （少一个键 ⇒ 那句被当成单月句筛掉；多一个键 ⇒ 正则认不出、永远查不到）。
	"前两个月":  true,
	"前三个月":  true,
	"前四个月":  true,
	"前五个月":  true,
	"前六个月":  true,
	"前七个月":  true,
	"前八个月":  true,
	"前九个月":  true,
	"前十一个月": true,
	"前十个月":  true,

	// 🔴 1 月特例：1 月的累计**就是**当月，报告直接写「1月份人民币贷款增加5.13万亿元」，
	// 与当月句同形。其余月份的当月句一律留在表外（那正是本表存在的理由）。
	//
	// 安全性依赖一条实测前提：**非 1 月报里不出现「1月份+指标」的句子** —— 用全部
	// 218 篇实测得 0 条（需求文档只验了 55 篇，结论一致）。核对语料时必须带词边界
	// `(?<![0-9])`，否则「11月份」里的子串会给出 12 条假阳性；而 Go 侧**不需要**
	// lookbehind：`[0-9]{1,2}月份` 贪婪 + 最左匹配 ⇒ 在「11月份」处捕获完整的
	// 「11月份」、查表不命中，正确排除。见 TestFlowRECapturesMonthlyPeriodVerbatim。
	"1月份": true,
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
// 「余额为」与数字之间可能有**一个空格**（M1c-3a 的 TASK-005）：实测 3 篇
// （2019年 / 2020-01 / 2020-02 社融存量），全部是「对实体经济发放的外币贷款折合人民币」这一项。
//
// 🔴 语料里其实是**两种字符**，但到这一层只剩一种：2020 两篇用 `0x20`，**2019 那篇用
// `0xa0` NO-BREAK SPACE**。Go 的 `\s` = `[\t\n\f\r ]`，**不含 U+00A0** ⇒ 写成 `\s*`
// 会修好 2 篇、静默漏掉 2019 那篇。而 strip.go 的 `spaceRE = [ \t\x{00a0}]+` 把两者
// 都折叠成**单个普通空格**，所以这里 ` ?` 就够，且**不能**用 `\s*` 去「保险」。
// 这个跨文件依赖由 TestTSFStockREToleratesSpaceAfterBalancePrefix 的第一个子测试钉住
// （从 U+00A0 原文出发、经 stripHTML 再匹配），strip.go 改了折叠规则那里会红。
//
// 尾部还要认「，同比持平」（M1c-3a 的 TASK-005，3 处实测：2023-07 / 2025-06 社融存量，
// 与 2026-05 v2 月报社融板块那句**没有「为」字**的）。做法是把 `持平` **单独**接在这条
// 模板的方向词位上，**不经 dirPat**——`持平` 刻意不在 directionAlt 里，理由见 amount.go
// 的 directionFlat：它一旦进全局词表，占比段那 45+ 处「同比持平」全部变成候选。
//
// 数值与 `%` 一起做成可选组：持平句没有它们，捕获组 4 取空串，由 parseRatio 特判成 0。
// 捕获组编号保持 1=数值 2=单位 3=方向词 4=同比值不变（extract.go 按位取 m[1..4]）。
func tsfStockRE(name string) *regexp.Regexp {
	return regexp.MustCompile(
		regexp.QuoteMeta(name) + `余额为? ?` + numPat + unitPat +
			`，同比(` + directionFlat + `|` + directionAlt + `)(?:` + numPat + `%)?`)
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

// 括号**全角与半角都要认**（M1c-3a 的 TASK-005）：实测 218 篇里全角「（M2）」4 篇
// （2026-07 / 2026-04 / 2023-11 / 2023-10）、半角「(M2)」76 篇。7 节月报 2026-07 的
// 探针输出正是 `货币 M2 not found (pattern 广义货币\(M2\)余额…)`。
//
// ⚠️ 不能指望 stripHTML 抹平：它的 punctNormalizer **只**归一逗号、分号与全角空格，
// **不碰括号**（读 strip.go 核实，非推断）⇒ 全角括号原样到达模板层。
// M1 / M0 与 M2 共用本函数，一处放宽三项同时生效。
func moneyRE(name, code string) *regexp.Regexp {
	return regexp.MustCompile(
		regexp.QuoteMeta(name) + `[(（]` + code + `[)）]余额` + numPat + unitPat +
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
		// v1（2019 及更早）写「非金融企业及机关团体贷款」，v2 写「企（事）业单位贷款」
		// ——与住户侧同构，一条正则覆盖两版（M1c-3a 的 TASK-005）。
		//
		// 🔴 **交替必须包在非捕获组 `(?:…)` 里**：scopeTotalRE 做的是**字符串拼接**
		// （`sc.anchorRE.String() + dirPat + numPat + unitPat`），裸交替拼接后结合律
		// 错位成 `A|B(dir)(num)(unit)` ⇒ 在 v2 报告里 `A` 单独命中、三个捕获组全空，
		// 取符号的入口收到三个空串后报「unknown direction word」。独立 reviewer 照原文
		// 实现时实撞过。TestScopeTotalREKeepsCaptureGroupsAfterAnchorAlternation 守着它。
		anchorRE:   regexp.MustCompile(`(?:企（事）业单位贷款|非金融企业及机关团体贷款)`),
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
	// 「余额为?」：**一季度正文写「国家外汇储备余额3.34万亿美元」，没有「为」字**
	// （TASK-004 实测；前三季度/年报/半年报三份都写「余额为」）。与利率那条的
	// 「加权平?均」是同一种一字模板漂移，处置也相同：容忍那一个字，**不放松成
	// `国家外汇储备.*万亿美元`** —— 同段还有「国家外汇储备余额较上月末减少…亿美元」
	// 这类句子，放松后会静默取到它，而 mustMatch 只在命中数 ≠ 1 时才报错。
	//
	// 加 `?` 不会引入重复命中：板块标题（「七、国家外汇储备余额3.34万亿美元」）在
	// sec.Title 里，而抽取只看 sec.Body（extractFXSection）—— 四份样本实测各命中 1 条。
	fxReserveRE = regexp.MustCompile(`国家外汇储备余额为?` + numPat + `万亿美元`)
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
