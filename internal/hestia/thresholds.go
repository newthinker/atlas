package hestia

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

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
	DepositSumTolerance float64

	// DepositSumDriftMax 是本期残差与前几期残差均值的偏离上限。
	//
	// ±12% 宽到几乎拦不住东西（实测 7.6–9.1%，余量仅 3pct），漂移检测才是
	// 这道闸的实际价值：口径突变会让残差跳档，而绝对值仍在容差内。
	DepositSumDriftMax float64

	// CorpLoanTolerance 是「短期+中长期+票据 vs 企业合计」的残差占比上限。
	// M0 三期实测 1.16% / 1.42% / 1.58%。
	CorpLoanTolerance float64

	// StockContinuityMax 是社融存量环比变化率的上限。
	//
	// ⚠️ 0.02 未经真实数据验证——M0 的两份样本里只有一份含社融，算不出环比。
	// M1c 回填出连续序列后必须重新标定。
	StockContinuityMax float64

	// YoYSanityMax 是同比字段绝对值的上限（百分数）。M0 实测最大 25%。
	YoYSanityMax float64

	// MagnitudeRanges 是 field → 合理区间。
	//
	// **有意为空**。区间必须用 M1c 回填数据的实际分布标定，不得拍脑袋——
	// DepositSumTolerance 的 ±2% 就是拍脑袋的代价。表为空时 magnitude_sanity
	// 记 skipped{not_calibrated}，填表即生效，无需改代码。
	MagnitudeRanges map[string]Range

	// CaliberExemptions 是口径变更期的定点豁免。
	CaliberExemptions []CaliberExemption
}

// Range 是单个字段的合理区间。
//
// Unit 现在没有消费者。它的作用是让 M1c 填表的人**必须写下单位**——而写下来
// 就会发现包注释声称的三类单位（万亿元 / 亿元 / 百分数）不覆盖 fx_reserve
// （万亿美元）与 fx_rate（元/美元）。
type Range struct {
	Min, Max float64
	Unit     string
}

// CaliberExemption 让某个期次的某几道闸门跳过检查。
//
// 三条约束（方案报告 4.6.3）：按 (期次, 检查 ID) 精确指定、Version 必须已登记、
// 命中记 skipped 而不是 passed。第三条是关键——豁免与通过在数据上必须可分，
// 把「这次没查」记成「查了没问题」等于伪造一次检查记录。
type CaliberExemption struct {
	Version    string   // 口径版本，必须在 validCaliberVersions 内
	Period     string   // 期末月，与 Meta.Period 同格式（"2025-01"）
	SkipChecks []string // 要跳过的检查 ID
	Reason     string   // 必填，写清为什么这期该跳
}

// DefaultThresholds 返回经 M0 真实数据校准的默认阈值。
func DefaultThresholds() Thresholds {
	return Thresholds{
		DepositSumTolerance: 0.12,
		DepositSumDriftMax:  0.03,
		CorpLoanTolerance:   0.05,
		StockContinuityMax:  0.02,
		YoYSanityMax:        50,
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
		if strings.TrimSpace(ex.Reason) == "" {
			return fmt.Errorf("hestia: caliber_exemptions[%d] (%s) 缺 Reason: "+
				"豁免必须写清为什么，否则下一个人无从判断它是否还该留着", i, ex.Period)
		}
		if len(ex.SkipChecks) == 0 {
			return fmt.Errorf("hestia: caliber_exemptions[%d] (%s) 的 SkipChecks 为空: "+
				"豁免必须按检查 ID 精确指定，不是整期跳过校验", i, ex.Period)
		}
	}
	return nil
}

// exemptionFor 返回命中 (period, checkID) 的豁免，没有则返回 nil。
//
// 精确匹配，不做范围或前缀匹配：一次口径变更豁免一个期次，写成范围就会让
// 它悄悄覆盖后续所有期次，那是永久后门而不是定点豁免。
func (t Thresholds) exemptionFor(period, checkID string) *CaliberExemption {
	for i := range t.CaliberExemptions {
		ex := &t.CaliberExemptions[i]
		if ex.Period == period && slices.Contains(ex.SkipChecks, checkID) {
			return ex
		}
	}
	return nil
}
