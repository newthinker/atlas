package hestia

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
)

// checkCompleteness 是 completeness 闸门的 ID，供豁免宽度校验引用。
//
// 它与 gates 表里的那个字面量是**两处**——真相源仍是 gates（knownCheckIDs 从它
// 派生）。改闸门 ID 时这里会静默过期，故由 TestCheckCompletenessIDMatchesGates
// 钉住：该常量必须仍在 knownCheckIDs() 里。
const checkCompleteness = "completeness"

// Thresholds 是七道闸门的全部可调参数。
//
// 它是 Go 结构体而不是 YAML——M1b-3 还没有读配置的调用方（cobra 命令属于
// M1b-4）。现在引入配置文件等于凭空多一层没有消费者的间接。
type Thresholds struct {
	// DepositSumTolerance 是存款加总残差占比的上限（**ytd 族**；mom 族见
	// DepositSumToleranceMoM / …MoMAbs，那一族换了仪器）。
	//
	// 央行报告里的「其中」是**部分列举**而非穷尽——四个部门加起来本就不等于总额，
	// 所以这道闸的容差必然远大于 0。这条理由自 M0 起没变。
	//
	// # 🔴 取值 0.17（M1c-4 的 TASK-010 标定，人类裁决 2026-09-02）
	//
	//	族                n    p50      p90      p95      max
	//	deposit_sum/ytd  52   0.0885   0.1460   0.1518   0.2501
	//	锚 743c50730128be0a648d8d5d2c99cac321b1a9e0，79 期语料 / 213 条记录
	//
	// 取 0.17 而不是覆盖 max 的 0.26：**0.26 会让这道闸对中位期次失效**。
	// 中位分母 147300 下，0.26 允许 38298 亿元的差额，而历史最大绝对差额只有 28000
	// ⇒ 任何已观测幅度的误差都能通过。0.17 允许 25041 < 28000，闸门仍抓得住。
	//
	// 🔴 **已知会 failed 的期次：2020-01（残差 0.2501）。这不是数据错误，不要去查。**
	// 1 月的 ytd 分母就是当月一个月的增量（年初至今 = 当月），28800 是该族最小分母
	// （中位 147300）；它的**绝对**差额 7203 反而**低于**该族中位 13300。
	// ⚠️ 但 1 月不是系统性更高、是**方差更大**：1 月 7 期为 .2501 / .1518 / .0724 /
	// .0501 / .0447 / .0433 / .0234，只有 2 期超过旧值 0.12，最小值是全族偏低的。
	// ⇒ **不能简单地把 1 月排除**，也没有地方可以排除：本字段是单值，不分 period_type
	// （对照：StockContinuityMax 是 map[string]float64，那才是分档的形态）。
	//
	// # 订正：此前这里写的「M0 三期实测 7.65% / 8.57% / 9.06%」已被 79 期语料证伪
	//
	// 那三个数本身没错，错的是**拿它当分布**：0.12 由它拍出，而实测 p95 就有 0.1518、
	// max 0.2501，8 期超过 0.12。下游据此写出的「实测残差**稳定在** 7.6–9.1%」
	// （fields.go A.3）是一句更强的假话 —— 三个点说不出「稳定」。
	// ⚠️ 别再从别处抄回 0.1663：那是 **p95**，也是**非 1 月**期次的 max，不是 max。
	DepositSumTolerance float64 `mapstructure:"deposit_sum_tolerance"`

	// DepositSumDriftMax 是本期残差与前几期残差均值的偏离上限。
	//
	// ±12% 宽到几乎拦不住东西（实测 7.6–9.1%，余量仅 3pct），漂移检测才是
	// 这道闸的实际价值：口径突变会让残差跳档，而绝对值仍在容差内。
	DepositSumDriftMax float64 `mapstructure:"deposit_sum_drift_max"`

	// CorpLoanTolerance 是「短期+中长期+票据 vs 企业合计」的残差占比上限。
	// M0 三期实测 1.16% / 1.42% / 1.58%。
	CorpLoanTolerance float64 `mapstructure:"corp_loan_tolerance"`

	// DepositSumToleranceMoM / CorpLoanToleranceMoM 是**当月口径**那一族的残差上限
	//（M1c-4 的 TASK-007）。
	//
	// 🔴 **M1c-4 的 TASK-010 已标定这两个值**（人类裁决 2026-09-02）。下面保留了标定
	// 之前的说法与它为什么错 —— 删掉旧说法而不留痕，下一个人无从判断这次订正的可信度。
	//
	// # 为什么必须是两个独立的字段，而不是与 ytd 共用一个
	//
	// ytd 的分母是**年初至今累计**、mom 的分母是**单月增量**，量级差一个数量级以上，
	// 残差占比的分布完全不同。共用一个值等于用累计口径的经验去卡单月口径，
	// 要么恒不拦、要么恒拦——两种都让这道闸失去意义。**这条判断经标定证实是对的。**
	//
	// # 标定依据（锚 743c50730128be0a648d8d5d2c99cac321b1a9e0，79 期语料 / 213 条记录）
	//
	//	族                n    p50      p90      p95      max      取值
	//	deposit  ytd     52   0.0885   0.1460   0.1518   0.2501   0.17（见 DepositSumTolerance）
	//	deposit  mom     21   0.2576   0.7796   0.8194   2.9508   **改用 max(K_abs,K_rel×|合计|)**
	//	corpLoan ytd     52   0.0136   0.0247   0.0294   0.0357   0.05（不动）
	//	corpLoan mom     26   0.0176   0.0614   0.0784   0.0788   **0.11 = max × 1.40**
	//
	// CorpLoanToleranceMoM = 0.11：**取值公式是 `实测 max × 1.40`**，其中 1.40 不是
	// 拍的，而是 corp_loan **ytd** 现行值相对其实测 max 的余量倍数（0.05 / 0.0357 = 1.40）
	// —— 那是四族里唯一经真实数据检验（0 期 failed）的值。用同一把尺量两族，
	// 留余量的理由可以逐字复用而不是另找一个。
	//
	// # 🔴 deposit 的 mom 族**换了仪器**，所以它有两个参数
	//
	// 比值判据只在分母远离零时有意义，而 deposit mom 的分母 min=447、**可为负**
	// （2021-07 = −11300）。详见 validate.go 的 caliberBand.absTol 注释与
	// DepositSumToleranceMoMAbs 字段。
	//
	// ⚠️ **一处必须写明的订正**：立项时（提问阶段）把 mom 族残差大**全部**归因于
	// 「分母近零」，那只对了一半。把两族都限制在 |分母| ≥ 10000 的子集里重算：
	//
	//	deposit mom  |分母|≥10000  n=14  p50=0.1672  p95=0.3931
	//	deposit ytd  |分母|≥10000  n=52  p50=0.0885  p95=0.1518
	//
	// ⇒ **排除分母效应之后，mom 族的勾稽余量本身仍是 ytd 的约 2 倍**（「其中」未列举
	// 的部分在单月口径上正负波动大，在累计口径上会相互抵消一部分）。分母近零只解释
	// 尾部为什么从 0.39 冲到 2.95，不解释主体。**K_rel 因此必须取自 mom 自己的分布，
	// 不能沿用 ytd 的 0.17。**
	//
	// # 标定之前这里写的是什么（保留原文，供判断本次订正的可信度）
	//
	// 原注释说这两个值是「未标定的占位数」，取法是**照抄同族 ytd 的值**（都是 0.12 /
	// 0.05），并说明照抄不等于「两族一样」。那个处置在当时是对的 —— 没有分布可依时，
	// 照抄是「不引入新臆断」的最小选择，且它刻意让 mom 族**响亮地 failed**
	// （failed 进 pending 会被人看见，拍宽的容差会让所有 mom 期次静默通过）。
	//
	// 🔴 **但原注释里有一句现在是假的**，且它会让人少标一个容差：
	//
	//	原文：CorpLoanToleranceMoM  🔴 TASK-009 **不产** corp_loan 的残差分布
	//	                            ⇒ TASK-010 **无从标定**，必须…登记为长期未标定占位
	//
	// **TASK-009 的射程后来扩大了，它产出了 corp_loan 六档**（ytd 五档 n=52 + mom 一档
	// n=26，见上表）。这句话写于射程扩大之前，之后没有人回来改它。
	// ⚠️ 危害是具体的：读到它的人会得出「corp_loan 没有依据、只能登记为长期未标定
	// 占位」，**而依据就在同时要读的那份 calibrate 报告里** —— 四个容差会少标一个。
	DepositSumToleranceMoM float64 `mapstructure:"deposit_sum_tolerance_mom"`
	CorpLoanToleranceMoM   float64 `mapstructure:"corp_loan_tolerance_mom"`

	// DepositSumToleranceMoMAbs 是 deposit 当月族的 **K_abs**：绝对容差下限（亿元）。
	//
	// 判据是 `|Σ四部门 − 合计| > max(K_abs, DepositSumToleranceMoM × |合计|)`。
	// K_abs 的唯一作用是**兜住分母趋零的那几期** —— 没有它，比值判据要覆盖
	// 2022-07（分母 447、绝对差额 1319）就得把容差取到 2.95，而那个值在中位分母
	// 12700 下允许 38100 亿元的差额（历史最大仅 8681）⇒ 闸门实质不再判定。
	//
	// # 取值公式（**给不出「从哪个分位来」的取值就是拍的**）
	//
	//	K_abs = max(|Σ分项−合计| : |分母| < 10000 的子集) × 1.40 = 2272 × 1.40 ≈ 3200
	//	K_rel = p95(残差占比 : |分母| ≥ 10000 的子集) × 1.40 = 0.3931 × 1.40 ≈ 0.55
	//
	// 1.40 这个余量倍数取自 corp_loan/ytd 现行值（0.05 / 实测 max 0.0357），四族共用
	// 同一把尺。交叉点 K_abs/K_rel ≈ 5818 亿元：分母小于它的期次走绝对下限。
	//
	// ⚠️ **0 期 failed 是换对仪器的结果，不是调宽的结果**，判别方式是拿同一条
	// 「×1.40」规则套在**旧仪器**上：纯比值需要 2.9508×1.40 = 4.13，那是个 no-op。
	// 新仪器在中位分母下的门限是 6985 亿元，**小于**历史最大绝对差额 8681 ⇒ 仍判得动。
	// 而一次真的量级事故（累计值写进当月列）绝对差额在 10^5 量级，门限 10^4 ⇒ 仍必被抓到。
	//
	// ⚠️ **只有 deposit mom 用它。** ytd 族按人类裁决保持纯比值（0.17）；corp_loan
	// 两族分母远离零，比值判据在那里有效。零值 = 不设下限 = 退化为纯比值。
	DepositSumToleranceMoMAbs float64 `mapstructure:"deposit_sum_tolerance_mom_abs"`

	// StockContinuityMax 是社融存量环比变化率的上限，**按 period_type 分档**
	// （period_type → 上限）。
	//
	// 分档而不是单值：Preceding 按 period_type 隔离序列 ⇒ 每年只有一个 h1 期次，
	// h1 的「上一期」是**去年的 h1**。q1 / h1 / q1_q3 / annual 四种都是年初起累计
	// 口径，相邻两期一律相隔 12 个月，只有 monthly 相隔 1 个月。拿同一个数卡这两
	// 种序列，只能二选一：月度那档形同虚设，或者年度那档每期都被拦下。
	//
	// ⚠️ 不写成「月均上限 × 间隔月数」：那样只需一个数，但它**假设存量线性增长**，
	// 而这道闸恰恰是用来抓「不该出现的跳变」的 —— 用线性模型定义「什么叫不线性」
	// 是循环的。
	//
	// 缺档在两处的处置**刻意相反**，因为是两种处境：
	//   - 运行时（gateStockContinuity）缺档记 skipped{no_threshold:<pt>} 而不是
	//     failed —— 那是「代码里有了新 period_type、配置还没跟上」，判 failed 会让
	//     那条序列的每一期都进 pending，而数据本身没问题。
	//   - 装载时（LoadConfig）**显式写了这张表却漏档**一律拒绝 —— 那是配置疏漏，
	//     而 mapstructure 的 merge 语义会让漏掉的那档静默继承默认值。
	StockContinuityMax map[string]float64 `mapstructure:"stock_continuity_max"`

	// YoYSanityMax 是同比字段绝对值的上限（百分数）。M0 实测最大 25%。
	YoYSanityMax float64 `mapstructure:"yoy_sanity_max"`

	// MagnitudeRanges 是 field → 合理区间。
	//
	// **有意为空**。区间必须用 M1c 回填数据的实际分布标定，不得拍脑袋——
	// DepositSumTolerance 的 ±2% 就是拍脑袋的代价。表为空时 magnitude_sanity
	// 记 skipped{not_calibrated}，填表即生效，无需改代码。
	MagnitudeRanges map[string]Range `mapstructure:"magnitude_ranges"`

	// CaliberExemptions 是口径变更期的定点豁免。
	CaliberExemptions []CaliberExemption `mapstructure:"caliber_exemptions"`
}

// Range 是单个字段的合理区间。
//
// Unit 现在没有消费者。它的作用是让 M1c 填表的人**必须写下单位**——而写下来
// 就会发现包注释声称的三类单位（万亿元 / 亿元 / 百分数）不覆盖 fx_reserve
// （万亿美元）与 fx_rate（元/美元）。
type Range struct {
	Min  float64 `mapstructure:"min"`
	Max  float64 `mapstructure:"max"`
	Unit string  `mapstructure:"unit"`
}

// CaliberExemption 让某个期次的某几道闸门跳过检查。
//
// 三条约束（方案报告 4.6.3）：按 (期次, 检查 ID) 精确指定、Version 必须已登记、
// 命中记 skipped 而不是 passed。第三条是关键——豁免与通过在数据上必须可分，
// 把「这次没查」记成「查了没问题」等于伪造一次检查记录。
//
// M1b-4a 在「精确指定」上补了第三个维度 PeriodTypes：期次只到月份，而同一个月
// 可能同时有 monthly 与 annual 两条独立序列。
type CaliberExemption struct {
	Version     string   `mapstructure:"version"`      // 口径版本，必须在 validCaliberVersions 内
	Period      string   `mapstructure:"period"`       // 期末月，与 Meta.Period 同格式（"2025-01"）
	PeriodTypes []string `mapstructure:"period_types"` // 适用的 period_type，必填非空
	SkipChecks  []string `mapstructure:"skip_checks"`  // 要跳过的检查 ID
	Reason      string   `mapstructure:"reason"`       // 必填，写清为什么这期该跳
}

// DefaultThresholds 返回经 M0 真实数据校准的默认阈值。
func DefaultThresholds() Thresholds {
	return Thresholds{
		DepositSumTolerance: 0.17, // M1c-4 的 TASK-010 标定，见字段注释
		DepositSumDriftMax:  0.03,
		CorpLoanTolerance:   0.05,

		// 🔴 **已标定**（M1c-4 的 TASK-010，人类裁决）：取值公式与分位见字段注释。
		// 订正：原注释写「corp_loan 那个 TASK-009 不产分布，需 Leader 裁决」——
		// **那句已成假话**，TASK-009 射程扩大后产出了 corp_loan 六档（ytd n=52 / mom n=26）。
		// deposit 的 mom 族换用 max(K_abs, K_rel×|合计|)，故它有两个参数。
		DepositSumToleranceMoM:    0.55,
		DepositSumToleranceMoMAbs: 3200,
		CorpLoanToleranceMoM:      0.11,
		StockContinuityMax:        defaultStockContinuityMax(),
		YoYSanityMax:              50,
	}
}

// defaultStockContinuityMax 是社融存量环比上限的默认分档表。
//
// 这两个数由 M1c-3b 的 TASK-005 用回填语料的实测分布标定，**取代了此前的占位值**
// （monthly 0.02 / 其余 0.15，M1b 起就在，原注释自陈「都未经真实数据验证」）。
//
// # 出处（tsf_stock 相邻期环比变化率，按 period_type 分档）
//
//	档     n    p95        max        取值
//	monthly 68  0.02291    0.02613    0.05
//	annual   6  0.13338    0.13338    0.20
//
// 标定语料 data/hestia-backfill-2026-08-14（218 篇，尝试解析 199 期）。
// 复现：go run ./cmd/atlas hestia backfill calibrate --dir <语料绝对路径> --allow-incomplete
// ⚠️ 语料未被 git 跟踪（.gitignore 的 data/），所以这几个数**无法从仓库自证**——
// 换语料重标时必须重跑上面那条命令，不要沿用这里的数字。
//
// # monthly 0.02 → 0.05 不是「为了让数据通过而放宽阈值」
//
// 0.02 是 M0/M1b 时代的**占位值**，当时算不出环比（两份样本里只有一份含社融）。
// 实测出来后它站不住：**p95 = 0.02291 已经越过 0.02** ⇒ 按旧值回填，至少 5% 的
// monthly 期次会被判进 pending。而 `--force` 对已入权威表的期次是数据层 no-op，
// **拦错了没有出路**。0.05 是这道闸门第一个有依据的值，不是把闸门开大。
//
// 且 0.05 仍拦得住它该拦的：社融存量单月环比 5%，按当前存量量级（约 400 万亿元）
// 意味着一个月增加约 20 万亿元——那是解析错误或口径断裂，不是真实经济波动。
//
// # ⚠️ q1 / h1 / q1_q3 三档 n=0，是**继承 annual，不是标定过的**
//
// 语料里这三种期次的 tsf_stock 相邻期样本数为零（见上表：只有 monthly 与 annual
// 有 n）。它们取 0.20 的依据是「四档同为年初起累计口径、相邻两期一律相隔 12 个月」
// 这一**同族**论证——比原来的 0.15 拍脑袋值强，但**弱于它们自己的数据**。
// 将来这三种期次攒出样本后应当各自重标，届时它们可能不再与 annual 同值。
//
// 每次调用返回新 map：调用方（含测试）会就地改它，共用一份会让一处改动波及全局。
func defaultStockContinuityMax() map[string]float64 {
	return map[string]float64{
		"monthly": 0.05, // 相邻两期相隔 1 个月；实测 n=68, max=0.02613
		// 以下四种都是年初起累计口径，相邻两期一律相隔 12 个月，故同档。
		// ⚠️ 只有 annual 有实测（n=6, max=0.13338），其余三档 n=0，是继承来的。
		"q1":     0.20,
		"h1":     0.20,
		"q1_q3":  0.20,
		"annual": 0.20,
	}
}

// validate 检查配置自身是否自洽。Validate 在最前面调用它——配置错了应当
// 立刻响亮失败，而不是让闸门带着错阈值跑完，产出一份看起来正常的报告。
func (t Thresholds) validate() error {
	for i, ex := range t.CaliberExemptions {
		field := "caliber_exemptions[" + strconv.Itoa(i) + "].Version"
		if err := checkEnum(field, ex.Version, validCaliberVersions); err != nil {
			return err
		}
		if ex.Period == "" {
			return fmt.Errorf("hestia: caliber_exemptions[%d] 缺 Period", i)
		}
		// 空切片报错，**不默认全部**。
		//
		// Go 零值让「忘了写 period_types」与「显式写全部三种」不可分，而两者后果
		// 差得很远：前者是配置疏漏，后者是深思熟虑的决定。要求非空就把它们分开了
		// —— 疏漏会响亮失败，而不是静默放行三条序列。
		if len(ex.PeriodTypes) == 0 {
			return fmt.Errorf("hestia: caliber_exemptions[%d] (%s) 的 PeriodTypes 为空: "+
				"必须显式列出适用的 period_type；留空不等于「全部」，"+
				"同月的 annual 与 monthly 是两条独立序列", i, ex.Period)
		}
		// 直接查 validPeriodTypes 而不是 checkEnum：那个函数收 []string，而白名单
		// 这边是 map[string]bool。合法取值因此在错误文案里是硬编码的第三份副本
		// （types.go 的 Meta.validate 是第二份）；thresholds_test.go 的
		// TestExemptionRejectsBadPeriodTypes/含未知取值 遍历 validPeriodTypes 本身，
		// 是这份副本的绊线——加了第四种取值而文案没跟上就会红。
		//
		// TASK-001 兑现了那份预防，并把副本消掉：文案里的取值列表现在由
		// periodTypeList() 从 validPeriodTypes 派生（types.go 的 Meta.validate 同此）。
		// 改之前这里与 types.go 各硬编码着一句 "monthly|h1|annual"，加完 q1/q1_q3
		// 两句都会**静默过期** —— 白名单放行了新值，而这条信息正是配置的人判断自己
		// 该填什么的唯一依据。上面那条绊线现在守的是「别改回硬编码」。
		for _, pt := range ex.PeriodTypes {
			if !validPeriodTypes[pt] {
				return fmt.Errorf("hestia: caliber_exemptions[%d] (%s) 的 PeriodTypes 含未知取值 %q "+
					"(want %s)", i, ex.Period, pt, strings.Join(periodTypeList(), "|"))
			}
		}
		if strings.TrimSpace(ex.Reason) == "" {
			return fmt.Errorf("hestia: caliber_exemptions[%d] (%s) 缺 Reason: "+
				"豁免必须写清为什么，否则下一个人无从判断它是否还该留着", i, ex.Period)
		}
		if len(ex.SkipChecks) == 0 {
			return fmt.Errorf("hestia: caliber_exemptions[%d] (%s) 的 SkipChecks 为空: "+
				"豁免必须按检查 ID 精确指定，不是整期跳过校验", i, ex.Period)
		}
		// ID 必须真实存在，且列表从 gates 派生——打错一个字（deposit_summ）在没有
		// 这道校验时会**静默失效**：豁免看起来配上了，实际那道闸门照跑，而配置的
		// 人以为已经跳过。M1b-3 的 T1 写这个结构时 gates 表尚不存在，
		// 故留到 **M1b-3 的 TASK-007**（已兑现，就是紧接着这段的 checkEnum）。
		//
		// ⚠️ 编号带 milestone 前缀不是啰嗦：任务 ID 每个 Sprint 从 001 重开，
		// 光写「T7」会让下一个 Sprint 的 TASK-007 以为这是自己的待办，
		// 进而去动一段已经正确、且有 TestExemptionRejectsUnknownCheckID 守着的代码。
		for _, id := range ex.SkipChecks {
			field := "caliber_exemptions[" + strconv.Itoa(i) + "].SkipChecks"
			if err := checkEnum(field, id, knownCheckIDs()); err != nil {
				return err
			}
		}
		// 以下两条堵的是同一件事——豁免宽到等价于「整期跳过校验」，而上面那条
		// len(SkipChecks)==0 的文案早就声称这不可能。两条路径成因不同，故判据不同。
		//
		// ⚠️ 判据一律**不能是数量阈值**（len(SkipChecks) > N）：那会误伤正常的
		// 多闸豁免——一次口径变更同时豁免六道是合法的。
		known := knownCheckIDs()
		coversAll := true
		for _, id := range known {
			if !slices.Contains(ex.SkipChecks, id) {
				coversAll = false
				break
			}
		}
		if coversAll {
			return fmt.Errorf("hestia: caliber_exemptions[%d] (%s) 跳过了全部 %d 道闸门: "+
				"这就是整期跳过校验，而豁免必须按检查 ID 精确指定（spec 4.6.3 约束 1）；"+
				"要针对某几道闸放行就逐个列出，不要枚举全部", i, ex.Period, len(known))
		}
		// completeness 是七道里**唯一**会因数据缺失而 failed 的闸门：其余六道遇缺
		// 字段一律降级 skipped（由 validate_test.go 的
		// TestValidateHandlesEmptyValuesWithoutSpecialCase 逐条断言）。所以单独
		// 豁免它就足以让一个几乎空白的期次整期过闸进权威表——比枚举七个 ID 便宜得多，
		// 而字面上完全符合「按检查 ID 精确指定」。
		if slices.Contains(ex.SkipChecks, checkCompleteness) {
			return fmt.Errorf("hestia: caliber_exemptions[%d] (%s) 不得豁免 %s: "+
				"其余六道遇缺字段一律降级 skipped，它是唯一会因数据缺失而 failed 的一道，"+
				"豁免它等价于整期跳过校验（一个只有若干字段的残缺期次会直接进权威表）",
				i, ex.Period, checkCompleteness)
		}
	}
	// —— magnitude_ranges 的校验（M1c-3b 的 TASK-004）——
	//
	// 空表合法：那是 magnitude_sanity 记 skipped{not_calibrated} 的正常状态（M1b-3）。
	// 非空则逐项校验，因为 gateMagnitudeSanity 对未知键是 continue ——
	// 打错一个字段名会**静默失效**，表看起来配上了而那道闸对该字段照样不设防。
	// 与 caliber_exemptions 的 deposit_summ 同类，那个已被 checkEnum 堵住，这里此前没有。
	//
	// 遍历 fieldOrder 建集合而不是 slices.Contains 逐个扫：54 个字段 × 54 项配置
	// 是 2916 次比较，虽然都不慢，但集合表达的是「fieldOrder 是唯一真相源」这件事。
	if len(t.MagnitudeRanges) > 0 {
		known := make(map[string]bool, len(fieldOrder))
		for _, f := range fieldOrder {
			known[f] = true
		}
		// 遍历 fieldOrder 而不是 map：map 迭代顺序随机，同一份坏配置两次跑
		// 报出不同的那一项，会让排查变成猜谜（与 gateMagnitudeSanity 同理）。
		for name := range t.MagnitudeRanges {
			if !known[name] {
				return fmt.Errorf("hestia: magnitude_ranges 含未知字段 %q: "+
					"gateMagnitudeSanity 对未知键是跳过，配置会**静默失效**——"+
					"字段名的唯一真相源是 fields.go 的 fieldOrder", name)
			}
		}
		for _, f := range fieldOrder {
			r, ok := t.MagnitudeRanges[f]
			if !ok {
				// 缺项在这里**刻意不拦**，全覆盖校验放在 config.go 的
				// checkMagnitudeRangesComplete（M1c-4 的 TASK-010）。
				//
				// 🔴 为什么不放在这里：本方法**不只被 LoadConfig 调用**——
				// validate.go:90 的 Validate() 每次校验都会走它。在这里拦，会让
				// 「程序化构造一张只含待测字段的小表」这种正当用法整个失效
				// （实测：改这里会让 9 条既有测试变红，它们测的是 NaN/倒置区间/
				// 单位缺失等**与全覆盖无关**的性质）。
				//
				// 而 DoD 描述的缺口是**运维换一份自己的 yaml**——那是装载路径，
				// 不是校验路径。放在 LoadConfig 侧既堵住了那个缺口，又不波及
				// 程序化调用方。形态照 checkStockContinuityComplete，不另发明。
				continue
			}
			// NaN / Inf 必须先于下面的 min >= max 检查（M1c-3b 的 TASK-012）。
			//
			// IEEE 754 规定涉 NaN 的比较除 != 外**恒假**，所以 r.Min >= r.Max
			// 拦不住它；而下游 gateMagnitudeSanity 的判据 v < r.Min || v > r.Max
			// **两侧同样恒假** ⇒ 该字段的幅度闸完全不设防，且报 passed。
			// 这与「打错字段名」是同一失效模式的第二条入口：表看起来配上了，
			// 而那道闸对该字段照样不设防。
			//
			// 实测（M1c-3b 的 TASK-004 交付后自查）：{NaN,NaN} / {NaN,1000} /
			// {0,NaN} / {-Inf,+Inf} 四种此前全部通过 validate；配 fx_reserve=42
			// （越界两个数量级）⇒ magnitude_sanity = passed。
			//
			// ±Inf 一并拒绝：[-Inf,+Inf] 在数据上与「忘了填」不可分，而一个永远
			// 不会失败的区间与没配这个字段等价 —— 差别只在配置的人以为配了。
			if math.IsNaN(r.Min) || math.IsNaN(r.Max) ||
				math.IsInf(r.Min, 0) || math.IsInf(r.Max, 0) {
				return fmt.Errorf("hestia: magnitude_ranges[%s] 的 min/max 必须是有限实数, "+
					"实得 min=%v max=%v: NaN 参与的比较恒假、Inf 区间永不越界，"+
					"两者都会让这道闸对该字段完全不设防且报 passed——"+
					"YAML 里写 .nan / .inf 也会走到这里", f, r.Min, r.Max)
			}
			if r.Min >= r.Max {
				return fmt.Errorf("hestia: magnitude_ranges[%s] 的 min(%g) >= max(%g): "+
					"倒置的区间会让该字段每一期都失败，而理由串读起来像数据错、不像配置错",
					f, r.Min, r.Max)
			}
			if strings.TrimSpace(r.Unit) == "" {
				return fmt.Errorf("hestia: magnitude_ranges[%s] 缺 unit: "+
					"它只出现在失败理由串里、不影响判定，正因如此会被漏填，"+
					"而漏填之后失败信息少了唯一能判断「是不是单位搞错了」的那一项", f)
			}
		}
	}
	return nil
}

// exemptionFor 返回命中 (period, periodType, checkID) 的豁免，没有则返回 nil。
//
// 精确匹配，不做范围或前缀匹配：一次口径变更豁免一个期次，写成范围就会让
// 它悄悄覆盖后续所有期次，那是永久后门而不是定点豁免。
//
// periodType 是 M1b-4a 补上的第三个维度：在此之前同月的 annual 与 monthly 会被
// 同一条豁免同时命中 —— 而它们是两条独立序列，一次只影响其中一条的口径变更
// 会连带放行另一条。
func (t Thresholds) exemptionFor(period, periodType, checkID string) *CaliberExemption {
	for i := range t.CaliberExemptions {
		ex := &t.CaliberExemptions[i]
		if ex.Period == period &&
			slices.Contains(ex.PeriodTypes, periodType) &&
			slices.Contains(ex.SkipChecks, checkID) {
			return ex
		}
	}
	return nil
}
