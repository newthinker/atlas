package hestia

import "strconv"

// Signal 是一个冷热信号的判定（方案报告 4.7）。四态而非三态：输入缺失时是 unknown，
// 不是红——「没有数据」和「数据不好」对读消息的人是两回事。
type Signal string

const (
	SignalGreen   Signal = "green"
	SignalYellow  Signal = "yellow"
	SignalRed     Signal = "red"
	SignalUnknown Signal = "unknown"
)

// Temperature 是四信号与综合温度。Score 是已知信号里的绿灯数，Known 是非 unknown 的
// 信号数——温度打成 Score/Known，有 unknown 时分母减一，**不假装是 4**。
type Temperature struct {
	Activation, Housing, Consumption, Credit Signal
	Score, Known                             int
}

// Evaluate 是纯函数（M2a 的 TASK-002）：只看本期观测与阈值，不读历史、不做 I/O。
// 契约里不带它的结果（附录 A.6）；Loom 从契约快照的阈值算同一份。
func Evaluate(obs Observation, cfg Signals) Temperature {
	var t Temperature
	t.Activation = evalActivation(obs.Values, cfg)
	t.Housing = evalThreshold(obs, FieldLoanHHMLTYTD, FieldLoanHHMLTMoM, cfg.HHMltMonthlyWarm)
	t.Consumption = evalThreshold(obs, FieldLoanHHShortYTD, FieldLoanHHShortMoM, cfg.HHShortMonthlyWarm)
	t.Credit = evalCredit(obs.Values, cfg)
	for _, s := range []Signal{t.Activation, t.Housing, t.Consumption, t.Credit} {
		if s == SignalUnknown {
			continue
		}
		t.Known++
		if s == SignalGreen {
			t.Score++
		}
	}
	return t
}

// evalActivation：M1−M2 剪刀差。≥ active 绿；≤ sink 红；介于之间黄。
func evalActivation(vals map[string]float64, cfg Signals) Signal {
	m1, ok1 := vals[FieldM1YoY]
	m2, ok2 := vals[FieldM2YoY]
	if !ok1 || !ok2 {
		return SignalUnknown
	}
	switch d := m1 - m2; {
	case d >= cfg.ScissorsActive:
		return SignalGreen
	case d <= cfg.ScissorsSink:
		return SignalRed
	default:
		return SignalYellow
	}
}

// evalThreshold：月均 ≥ warm 绿，否则红（楼市、消费两个信号没有黄）。
func evalThreshold(obs Observation, ytd, mom string, warm float64) Signal {
	v, ok := monthlyAverage(obs.Values, ytd, mom, obs.Meta.Period, obs.Meta.PeriodType)
	if !ok {
		return SignalUnknown
	}
	if v >= warm {
		return SignalGreen
	}
	return SignalRed
}

// evalCredit：票据 ÷ 企业贷款合计（百分数）。两者必须**同口径**——都 _mom 或都 _ytd；
// 一个当月一个累计的比值没有意义。比值不需要除以月数，同口径下分子分母同除。
// 合计为 0 时比值无定义 ⇒ unknown（不是红也不是绿）。
func evalCredit(vals map[string]float64, cfg Signals) Signal {
	bill, total, ok := sameCaliberPair(vals, FieldLoanBillYTD, FieldLoanBillMoM, FieldLoanCorpTotalYTD, FieldLoanCorpTotalMoM)
	if !ok || total == 0 {
		return SignalUnknown
	}
	switch r := bill / total * 100; {
	case r < cfg.BillRatioHealthy:
		return SignalGreen
	case r >= cfg.BillRatioSevere:
		return SignalRed
	default:
		return SignalYellow
	}
}

// sameCaliberPair 在 (_mom, _mom) 与 (_ytd, _ytd) 里取第一对都非空的；_mom 优先。
func sameCaliberPair(vals map[string]float64, aYTD, aMoM, bYTD, bMoM string) (float64, float64, bool) {
	if a, okA := vals[aMoM]; okA {
		if b, okB := vals[bMoM]; okB {
			return a, b, true
		}
	}
	if a, okA := vals[aYTD]; okA {
		if b, okB := vals[bYTD]; okB {
			return a, b, true
		}
	}
	return 0, 0, false
}

// monthlyAverage 取一个流量字段的月均（方案报告 4.7，M2a 订正）：
//   - _mom 非空 ⇒ 就是它（月数 1）
//   - monthly 只有 _ytd ⇒ ÷ 月份序号（2020–2023 的月报是「1-N月」累计口径）
//   - q1 / h1 / q1_q3 / annual ⇒ ÷ 3 / 6 / 9 / 12
//
// 月数解析为 0 时视为缺失（ok=false），不除——否则 ÷0 得 ±Inf，会被 evalThreshold 当成
// 一个「很大」的月均判绿。
func monthlyAverage(vals map[string]float64, ytd, mom, period, periodType string) (float64, bool) {
	if v, ok := vals[mom]; ok {
		return v, true
	}
	v, ok := vals[ytd]
	if !ok {
		return 0, false
	}
	n := monthsInPeriod(period, periodType)
	if n == 0 {
		return 0, false
	}
	return v / float64(n), true
}

// monthsInPeriod：期内月数。monthly 取 period 的 MM；解析不出、越界（00/13）、长度不是 7
// 或未知 periodType ⇒ 0（调用方视为缺失）。
//
// "q1" 等 period_type 字面量不是业务字段名，不受字段名守卫约束；validPeriodTypes 在
// types.go 已有同样的字面量。
func monthsInPeriod(period, periodType string) int {
	switch periodType {
	case "q1":
		return 3
	case "h1":
		return 6
	case "q1_q3":
		return 9
	case "annual":
		return 12
	case "monthly":
		if len(period) != 7 {
			return 0
		}
		m, err := strconv.Atoi(period[5:])
		if err != nil || m < 1 || m > 12 {
			return 0
		}
		return m
	}
	return 0
}
