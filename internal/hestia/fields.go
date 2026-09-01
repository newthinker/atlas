// Package hestia 是央行金融统计数据的采集管线：发现、解析、校验、入库。
//
// 本文件是全部业务字段的唯一真相源。DDL、INSERT 列、白名单都从 fieldOrder
// 派生——手写多份必然不同步，那正是 M0 复盘时列为要避免的设计。
//
// 单位约定（解析器负责归一，库里只存数值）：
//
//	余额 / 存量        万亿元
//	增量 / 新增        亿元
//	同比 / 占比 / 利率  百分数（不带符号）
//
// 单位是 schema 的一部分，改它属于 breaking change，不设 units_version 列。
//
// 唯一真相源的约束是可机械核验的：本包除本文件外的**非 _test.go 文件**不得出现
// 业务字段名的字符串字面量，schema/store 一律遍历 fieldOrder。测试文件豁免——
// golden list 与分组计数必须写字面量才有意义。核验见 TestFieldNamesAppearOnlyInFieldsGo。
//
// # 并发契约
//
// 同一业务键（period + period_type）上的 Save **不保证并发安全，由调用方串行化**。
//
// Save 的 Lookup → Classify → INSERT/UPDATE 不在一个事务里：两个并发的 Save 同键时
// 都可能读到 Exists=false、都判为 New、都执行 INSERT，主键约束让其中一方拿到 UNIQUE
// 错误——那一次的数据随之丢失，且因为错误发生在 insert 阶段，它不会落进 pending 表。
//
// 记成契约而不是加事务，是因为要关掉这个窗口需要 BEGIN IMMEDIATE 级别的写锁，那会让
// 每一次 Save 都持写锁，而不只是真正写入的那些。在出现「并发写同一业务键」的调用方
// 之前，这个代价不值得付。若将来出现，改法是把 Lookup 到写入整段放进 IMMEDIATE 事务，
// 而不是在 Go 侧加锁——Go 侧的锁挡不住另一个进程。
//
// go test -race 发现不了这件事：它只看 Go 层的数据竞争，SQL 层的 TOCTOU 在其视野之外。
package hestia

const (
	// —— A.1 社会融资规模 · 总量（3） ——
	FieldTSFStock    = "tsf_stock"
	FieldTSFStockYoY = "tsf_stock_yoy"
	FieldTSFFlowYTD  = "tsf_flow_ytd"

	// —— A.1 社会融资规模 · 存量分项（8 项 × 余额/同比 = 16） ——
	// 这 16 个是 M0 发现 6 补充的：原 schema 只有社融总量，写不出
	// 「政府债券占比同比 +1.3pct」这类判断，而那是解读的核心。
	FieldTSFStockRMBLoan       = "tsf_stock_rmb_loan"
	FieldTSFStockRMBLoanYoY    = "tsf_stock_rmb_loan_yoy"
	FieldTSFStockFXLoan        = "tsf_stock_fx_loan"
	FieldTSFStockFXLoanYoY     = "tsf_stock_fx_loan_yoy"
	FieldTSFStockEntrust       = "tsf_stock_entrust"
	FieldTSFStockEntrustYoY    = "tsf_stock_entrust_yoy"
	FieldTSFStockTrust         = "tsf_stock_trust"
	FieldTSFStockTrustYoY      = "tsf_stock_trust_yoy"
	FieldTSFStockBankAccept    = "tsf_stock_bankaccept"
	FieldTSFStockBankAcceptYoY = "tsf_stock_bankaccept_yoy"
	FieldTSFStockCorpBond      = "tsf_stock_corp_bond"
	FieldTSFStockCorpBondYoY   = "tsf_stock_corp_bond_yoy"
	FieldTSFStockGovtBond      = "tsf_stock_govt_bond"
	FieldTSFStockGovtBondYoY   = "tsf_stock_govt_bond_yoy"
	FieldTSFStockEquity        = "tsf_stock_equity"
	FieldTSFStockEquityYoY     = "tsf_stock_equity_yoy"

	// —— A.1 社会融资规模 · 增量分项（8） ——
	FieldTSFFlowRMBLoanYTD    = "tsf_flow_rmb_loan_ytd"
	FieldTSFFlowGovtBondYTD   = "tsf_flow_govt_bond_ytd"
	FieldTSFFlowCorpBondYTD   = "tsf_flow_corp_bond_ytd"
	FieldTSFFlowFXLoanYTD     = "tsf_flow_fx_loan_ytd"
	FieldTSFFlowEntrustYTD    = "tsf_flow_entrust_ytd"
	FieldTSFFlowTrustYTD      = "tsf_flow_trust_ytd"
	FieldTSFFlowBankAcceptYTD = "tsf_flow_bankaccept_ytd"
	FieldTSFFlowEquityYTD     = "tsf_flow_equity_ytd"

	// —— A.1 社会融资规模 · 增量当月（9，M1c-4 的 TASK-004） ——
	//
	// 🔴 **为什么需要这一族**：央行在 2020–2023 的相当多期次里，社融增量报告
	// **只发当月数**（实测 2020-04 全文只有「2020年4月社会融资规模增量为3.09万亿元」，
	// 没有任何累计句）；另一些期次则是「累计合计句孤悬段末 + 当月句带整套分项」
	// （实测 2022-07/08/10/11、2023-07/08/10/11）。
	//
	// 此前解析器对这两种形态都**拒绝整篇**，而拒绝是对的 —— 把当月值装进 *_ytd
	// 会得到量级完全合理的错值，下游没有任何闸门拦得住。缺的不是抽取能力，
	// 是这一族列（spec §1）。
	//
	// ⚠️ 本任务（M1c-4 的 TASK-004）只加列与声明，**抽取侧按口径选列是 M1c-4 的
	// TASK-005**。在那之前这 22 列恒为空，不进任何字段分布表。
	FieldTSFFlowMoM           = "tsf_flow_mom"
	FieldTSFFlowRMBLoanMoM    = "tsf_flow_rmb_loan_mom"
	FieldTSFFlowFXLoanMoM     = "tsf_flow_fx_loan_mom"
	FieldTSFFlowEntrustMoM    = "tsf_flow_entrust_mom"
	FieldTSFFlowTrustMoM      = "tsf_flow_trust_mom"
	FieldTSFFlowBankAcceptMoM = "tsf_flow_bankaccept_mom"
	FieldTSFFlowCorpBondMoM   = "tsf_flow_corp_bond_mom"
	FieldTSFFlowGovtBondMoM   = "tsf_flow_govt_bond_mom"
	FieldTSFFlowEquityMoM     = "tsf_flow_equity_mom"

	// —— A.2 货币供应量（6） ——
	// M1 口径在 2025-01 修订过（纳入个人活期存款），跨该点的同比无效。
	// 这件事校验闸门拦不住，只能靠 caliber_version 标注。
	FieldM2    = "m2"
	FieldM2YoY = "m2_yoy"
	FieldM1    = "m1"
	FieldM1YoY = "m1_yoy"
	FieldM0    = "m0"
	FieldM0YoY = "m0_yoy"

	// —— A.3 存款（7） ——
	// 四个分部门加总不等于总额：报告里的「其中」是部分列举而非穷举，
	// 实测残差稳定在 7.6–9.1%。deposit_sum 闸门的容差因此是 ±12% 而非 ±2%。
	FieldDepositBalance      = "deposit_balance"
	FieldDepositBalanceYoY   = "deposit_balance_yoy"
	FieldDepositFlowYTD      = "deposit_flow_ytd"
	FieldDepositHouseholdYTD = "deposit_household_ytd"
	FieldDepositCorpYTD      = "deposit_corp_ytd"
	FieldDepositFiscalYTD    = "deposit_fiscal_ytd"
	FieldDepositNBFIYTD      = "deposit_nbfi_ytd"

	// —— A.3 存款 · 当月（5，M1c-4 的 TASK-004） ——
	//
	// 实测 2022-07 金融统计：存款节只有「7月份人民币存款增加447亿元」，
	// 而同一篇的贷款节有「1-7月，人民币贷款累计增加14.35万亿元」
	// ⇒ **同一份观测里不同字段族的口径不同**，这是本族存在的直接理由。
	FieldDepositFlowMoM      = "deposit_flow_mom"
	FieldDepositHouseholdMoM = "deposit_household_mom"
	FieldDepositCorpMoM      = "deposit_corp_mom"
	FieldDepositFiscalMoM    = "deposit_fiscal_mom"
	FieldDepositNBFIMoM      = "deposit_nbfi_mom"

	// —— A.4 贷款（10） ——
	FieldLoanBalance      = "loan_balance"
	FieldLoanBalanceYoY   = "loan_balance_yoy"
	FieldLoanFlowYTD      = "loan_flow_ytd"
	FieldLoanHHShortYTD   = "loan_hh_short_ytd"
	FieldLoanHHMLTYTD     = "loan_hh_mlt_ytd"
	FieldLoanCorpTotalYTD = "loan_corp_total_ytd"
	FieldLoanCorpShortYTD = "loan_corp_short_ytd"
	FieldLoanCorpMLTYTD   = "loan_corp_mlt_ytd"
	FieldLoanBillYTD      = "loan_bill_ytd"
	FieldLoanNBFIYTD      = "loan_nbfi_ytd"

	// —— A.4 贷款 · 当月（8，M1c-4 的 TASK-004） ——
	//
	// ⚠️ **贷款族不是可选项。** extract.go 的分部门口径守卫也守着贷款分部门段，
	// 只是存款那道先返回。实测 2023-07：「前七个月人民币贷款增加16.08万亿元……
	// 7月份人民币贷款增加3459亿元。分部门看，住户贷款减少2007亿元……」
	// ⇒ 合计有两种口径、分部门全是当月。不加这一族，那 4 篇修好存款后
	// 会在贷款守卫上换个地方失败。
	FieldLoanFlowMoM      = "loan_flow_mom"
	FieldLoanHHShortMoM   = "loan_hh_short_mom"
	FieldLoanHHMLTMoM     = "loan_hh_mlt_mom"
	FieldLoanCorpTotalMoM = "loan_corp_total_mom"
	FieldLoanCorpShortMoM = "loan_corp_short_mom"
	FieldLoanCorpMLTMoM   = "loan_corp_mlt_mom"
	FieldLoanBillMoM      = "loan_bill_mom"
	FieldLoanNBFIMoM      = "loan_nbfi_mom"

	// —— A.5 利率与外部（4） ——
	FieldRateIBO   = "rate_ibo"
	FieldRateRepo  = "rate_repo"
	FieldFXReserve = "fx_reserve"
	FieldFXRate    = "fx_rate"
)

// fieldOrder 是全部 76 个业务字段，按附录 A 的分组顺序。
//
// 顺序有意义：DDL 的列顺序、INSERT 的列顺序都跟随它。遍历字段时一律用
// 这个切片而不是 map——map 迭代顺序随机，会让每次生成的 SQL 都不同，
// 日志无法比对。
//
// M1c-4 的 TASK-004 起，三个流量族各自带一份当月口径列（*_mom），**紧跟在
// 对应的 *_ytd 项之后**：报告按本切片的顺序打印，同族聚在一起读起来才成话。
var fieldOrder = []string{
	// A.1 社融 · 总量
	FieldTSFStock, FieldTSFStockYoY, FieldTSFFlowYTD,
	// A.1 社融 · 存量分项
	FieldTSFStockRMBLoan, FieldTSFStockRMBLoanYoY,
	FieldTSFStockFXLoan, FieldTSFStockFXLoanYoY,
	FieldTSFStockEntrust, FieldTSFStockEntrustYoY,
	FieldTSFStockTrust, FieldTSFStockTrustYoY,
	FieldTSFStockBankAccept, FieldTSFStockBankAcceptYoY,
	FieldTSFStockCorpBond, FieldTSFStockCorpBondYoY,
	FieldTSFStockGovtBond, FieldTSFStockGovtBondYoY,
	FieldTSFStockEquity, FieldTSFStockEquityYoY,
	// A.1 社融 · 增量分项
	FieldTSFFlowRMBLoanYTD, FieldTSFFlowGovtBondYTD, FieldTSFFlowCorpBondYTD,
	FieldTSFFlowFXLoanYTD, FieldTSFFlowEntrustYTD, FieldTSFFlowTrustYTD,
	FieldTSFFlowBankAcceptYTD, FieldTSFFlowEquityYTD,
	// A.1 社融 · 增量当月（M1c-4 的 TASK-004）
	//
	// FieldTSFFlowMoM 是总量，放在增量**分项之后**而不是跟着 FieldTSFFlowYTD
	// （A.1 总量那一行）：同族相邻的判据是「报告里读起来成话」，而当月这一族
	// 整体是一段，总量领着它自己的分项。
	FieldTSFFlowMoM,
	FieldTSFFlowRMBLoanMoM, FieldTSFFlowGovtBondMoM, FieldTSFFlowCorpBondMoM,
	FieldTSFFlowFXLoanMoM, FieldTSFFlowEntrustMoM, FieldTSFFlowTrustMoM,
	FieldTSFFlowBankAcceptMoM, FieldTSFFlowEquityMoM,
	// A.2 货币供应量
	FieldM2, FieldM2YoY, FieldM1, FieldM1YoY, FieldM0, FieldM0YoY,
	// A.3 存款
	FieldDepositBalance, FieldDepositBalanceYoY, FieldDepositFlowYTD,
	FieldDepositHouseholdYTD, FieldDepositCorpYTD, FieldDepositFiscalYTD,
	FieldDepositNBFIYTD,
	// A.3 存款 · 当月（M1c-4 的 TASK-004）
	FieldDepositFlowMoM,
	FieldDepositHouseholdMoM, FieldDepositCorpMoM, FieldDepositFiscalMoM,
	FieldDepositNBFIMoM,
	// A.4 贷款
	FieldLoanBalance, FieldLoanBalanceYoY, FieldLoanFlowYTD,
	FieldLoanHHShortYTD, FieldLoanHHMLTYTD,
	FieldLoanCorpTotalYTD, FieldLoanCorpShortYTD, FieldLoanCorpMLTYTD,
	FieldLoanBillYTD, FieldLoanNBFIYTD,
	// A.4 贷款 · 当月（M1c-4 的 TASK-004）
	FieldLoanFlowMoM,
	FieldLoanHHShortMoM, FieldLoanHHMLTMoM,
	FieldLoanCorpTotalMoM, FieldLoanCorpShortMoM, FieldLoanCorpMLTMoM,
	FieldLoanBillMoM, FieldLoanNBFIMoM,
	// A.5 利率与外部
	FieldRateIBO, FieldRateRepo, FieldFXReserve, FieldFXRate,
}

// allFields 是字段白名单。Observation.Values 的键会拼进 INSERT 的列名，
// 解析器写错一个键名，没有它就是一条 SQL 错误或更糟的静默行为。
var allFields = func() map[string]bool {
	m := make(map[string]bool, len(fieldOrder))
	for _, f := range fieldOrder {
		m[f] = true
	}
	return m
}()
