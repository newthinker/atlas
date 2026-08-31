package hestia

import (
	"fmt"
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
	// DepositSumTolerance 是存款加总残差占比的上限。
	//
	// 0.12 而不是方案报告初稿的 0.02：M0 用三期真实数据实测，残差是
	// 7.65% / 8.57% / 9.06%。原因是央行报告里的「其中」是**部分列举**而非
	// 穷尽——四个部门加起来本就不等于总额。±2% 会让每一期都被拦下。
	DepositSumTolerance float64 `mapstructure:"deposit_sum_tolerance"`

	// DepositSumDriftMax 是本期残差与前几期残差均值的偏离上限。
	//
	// ±12% 宽到几乎拦不住东西（实测 7.6–9.1%，余量仅 3pct），漂移检测才是
	// 这道闸的实际价值：口径突变会让残差跳档，而绝对值仍在容差内。
	DepositSumDriftMax float64 `mapstructure:"deposit_sum_drift_max"`

	// CorpLoanTolerance 是「短期+中长期+票据 vs 企业合计」的残差占比上限。
	// M0 三期实测 1.16% / 1.42% / 1.58%。
	CorpLoanTolerance float64 `mapstructure:"corp_loan_tolerance"`

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
		DepositSumTolerance: 0.12,
		DepositSumDriftMax:  0.03,
		CorpLoanTolerance:   0.05,
		StockContinuityMax:  defaultStockContinuityMax(),
		YoYSanityMax:        50,
	}
}

// defaultStockContinuityMax 是社融存量环比上限的默认分档表。
//
// ⚠️ 两个数**都是占位，都未经真实数据验证** —— M0 的两份样本里只有一份含社融，
// 算不出环比。0.02 是 M1b 起就在的占位数；0.15 同为占位，量级只取自「年度序列的
// 相邻两期相隔 12 个月，跳变幅度必然显著大于月度」这一**定性**事实，**不是**
// 0.02 × 12（那是线性外推，见 Thresholds.StockContinuityMax 的注释）。
// M1c 回填出连续序列后由标定结果替换（M1c-3）。
//
// 每次调用返回新 map：调用方（含测试）会就地改它，共用一份会让一处改动波及全局。
func defaultStockContinuityMax() map[string]float64 {
	return map[string]float64{
		"monthly": 0.02, // 相邻两期相隔 1 个月
		// 以下四种都是年初起累计口径，相邻两期一律相隔 12 个月，故同档。
		"q1":     0.15,
		"h1":     0.15,
		"q1_q3":  0.15,
		"annual": 0.15,
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
				continue
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
