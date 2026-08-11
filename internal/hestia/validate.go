package hestia

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
)

// History 是闸门层对历史数据的全部需求。
//
// 定义在消费方而不是 Store 那边：闸门只要「前 n 期」这一个能力，把它收成
// 单方法接口，单测就能注入假历史，不必为了测一个纯函数去建真库。
//
// 一个方法支撑两道闸（stock_continuity 用 n=1，deposit_sum 的漂移用 n=6），
// 闸门自己算。不为每道闸各开一个方法——那会让接口随闸门数量膨胀。
type History interface {
	// Preceding 返回 period 之前最近 n 期的当前行，按 period 降序。
	//
	// 库里没有历史时返回空切片而不是错误——首期入库是正常路径。
	Preceding(ctx context.Context, period, periodType string, n int) ([]Observation, error)
}

// NoHistory 是恒空的 History，给还没有库的调用方用（例如只想跑无历史闸门的
// dry-run）。
//
// 它存在的意义是让 Validate 能拒绝 nil：nil 一律是接线错误，而「确实没有
// 历史」有一个显式的值来表达。两者混用会让忘记传 Store 这种 bug 悄悄退化成
// 「所有需要历史的闸门都 skipped」。
var NoHistory History = noHistory{}

type noHistory struct{}

func (noHistory) Preceding(context.Context, string, string, int) ([]Observation, error) {
	return nil, nil
}

// historyDepth 是一次取满的历史期数，取两道闸需求的较大者
// （stock_continuity 要 1 期，deposit_sum 的漂移要 6 期）。
//
// 一次取满而不是各取各的：两道闸共用同一份历史，查两次库只是多一次往返，
// 且两次之间库可能已变，闸门会基于不一致的历史下判断。
const historyDepth = 6

// Validate 跑全部闸门，产出 Store.Save 要求的 ValidationReport。
//
// error 只在基础设施故障时返回（配置非法、History 查库失败）。闸门判定失败
// 不是 error——它进 report.Checks，由调用方决定走 pending 还是权威表。
// 两者混用会让调用方分不清「这期数据没过闸」（正常路径）和「数据库连不上」
// （该重试）。
//
// obs.Values 为空是**正常输入**，不在这里特判：五道闸门各自按 absent 跳过，
// 由 completeness 判 failed（spec 第 9 节）。入口加特判会把这条自然性质
// 换成一处需要自己被测试的分支。
func Validate(ctx context.Context, obs Observation, h History, cfg Thresholds) (ValidationReport, error) {
	if h == nil {
		return ValidationReport{}, errors.New(
			"hestia: Validate needs a History (pass NoHistory when there is no store)")
	}
	if err := cfg.validate(); err != nil {
		return ValidationReport{}, err
	}

	prior, err := h.Preceding(ctx, obs.Meta.Period, obs.Meta.PeriodType, historyDepth)
	if err != nil {
		return ValidationReport{}, fmt.Errorf("hestia: validate %s/%s: %w",
			obs.Meta.Period, obs.Meta.PeriodType, err)
	}

	in := gateInput{obs: obs, prior: prior, cfg: cfg}
	checks := make([]Check, 0, len(gates))
	passed := true
	for _, g := range gates {
		c := g.fn(in)
		c.ID = g.id // 闸门函数不写自己的 ID，避免表与实现两处对不上
		if c.Status == CheckFailed {
			passed = false
		}
		checks = append(checks, c)
	}
	return ValidationReport{Passed: passed, Checks: checks}, nil
}

// gateInput 是每道闸门拿到的全部输入。
type gateInput struct {
	obs   Observation
	prior []Observation // 按 period 降序，可能为空
	cfg   Thresholds
}

// need 检查闸门所需字段是否都在，缺任一就返回带 absent_field 理由的 skip。
//
// 闸门不该对缺失字段做判定——那等于把「没数据」当成「数据有问题」，
// 而 v1 期次天然缺 27 个字段。
func (in gateInput) need(fields ...string) *Check {
	for _, f := range fields {
		if _, ok := in.obs.Values[f]; !ok {
			return &Check{Status: CheckSkipped, Reason: "absent_field:" + f}
		}
	}
	return nil
}

// v 取值。调用前必须先过 need，否则缺失字段会静默读成 0。
func (in gateInput) v(f string) float64 { return in.obs.Values[f] }

type gate struct {
	id string
	fn func(gateInput) Check
}

// gates 是全部闸门的唯一清单。ID 与 M0 契约样本一致，顺序即报告里 Checks
// 的顺序——让不同期次的报告可以逐行对照。
var gates = []gate{
	{"monetary_hierarchy", gateMonetaryHierarchy},
	{"deposit_sum", gateDepositSum},
	{"corp_loan_reconcile", gateCorpLoanReconcile},
	{"stock_continuity", gateStockContinuity},
	{"yoy_sanity", gateYoYSanity},
	{"completeness", gateCompleteness},
	{"magnitude_sanity", gateMagnitudeSanity},
}

// gateMonetaryHierarchy 查 M2 > M1 > M0。
//
// 最基本的货币层次约束：M2 含 M1 含 M0。它不成立说明抽取把三个数张冠李戴了，
// 比任何容差检查都更直接地指向解析错误。
//
// Value 恒为 nil：这道闸判的是三个数的**序关系**，没有单一实测值可记。
// 硬凑一个（比如 m2-m1）会让下游以为那个数有单位。
func gateMonetaryHierarchy(in gateInput) Check {
	if skip := in.need(FieldM2, FieldM1, FieldM0); skip != nil {
		return *skip
	}
	m2, m1, m0 := in.v(FieldM2), in.v(FieldM1), in.v(FieldM0)
	if m2 > m1 && m1 > m0 {
		return Check{Status: CheckPassed}
	}
	return Check{Status: CheckFailed,
		Reason: fmt.Sprintf("m2=%g m1=%g m0=%g", m2, m1, m0)}
}

// minDriftHistory 是漂移检测所需的最少历史期数。
//
// 1–2 期的均值不稳，噪声会盖过信号：相邻两期残差本身就可能差 1pct，而漂移
// 阈值只有 3pct。3 期是让均值有意义的最低要求。
const minDriftHistory = 3

// depositPartFields 是存款的四个部门分项。
//
// 它们加起来本就不等于总额——央行报告里的「其中」是部分列举而非穷尽，
// M0 三期实测残差 7.65% / 8.57% / 9.06%。
var depositPartFields = []string{
	FieldDepositHouseholdYTD, FieldDepositCorpYTD,
	FieldDepositFiscalYTD, FieldDepositNBFIYTD,
}

// gateDepositSum 查存款加总，两个判据合成一个状态。
//
// 判据一（绝对残差）：|Σ四部门 − 总额| / 总额 ≤ DepositSumTolerance。
// 判据二（残差漂移）：本期残差与前几期残差均值的偏离 ≤ DepositSumDriftMax。
//
// 为什么合成一道闸而不是拆两道：M0 契约样本确认 check ID 只有七个，
// deposit_sum_tolerance 与 deposit_sum_drift_max 是**阈值名，不是 check ID**。
//
// 为什么需要判据二：±12% 宽到几乎拦不住东西（实测 7.6–9.1%，余量仅 3pct）。
// 口径突变会让残差跳档，而绝对值仍在容差内——漂移才是这道闸的实际价值。
//
// # Value 的单位：比例，与 corp_loan_reconcile 的亿元不同
//
// Value 恒为绝对残差**占比**（M0 契约样本记 0.0857），不因走了哪条判据而变。
// 上面那道闸记的是亿元绝对量，两者刻意不同，见其注释。
func gateDepositSum(in gateInput) Check {
	if skip := in.need(FieldDepositFlowYTD); skip != nil {
		return *skip
	}
	if skip := in.need(depositPartFields...); skip != nil {
		return *skip
	}
	r, ok := depositResidual(in.obs.Values)
	if !ok {
		// 走到这里只可能是零分母：上面两道 need 已经保证字段齐全。
		return Check{Status: CheckSkipped,
			Reason: "zero_denominator:" + FieldDepositFlowYTD}
	}

	c := Check{Value: &r} // Value 恒为绝对残差占比，与 M0 契约样本一致

	// 判据一优先：绝对值都超了，再谈漂移没有意义。
	if r > in.cfg.DepositSumTolerance {
		c.Status = CheckFailed
		c.Reason = fmt.Sprintf("tolerance_exceeded: residual %.4f exceeds %.4f",
			r, in.cfg.DepositSumTolerance)
		return c
	}

	// 判据二。历史不足时绝对值确实查过并通过了，所以记 passed 而不是 skipped——
	// 但漂移没查这件事不能丢，写进 Reason。
	//
	// 两个理由刻意可分：no_prior_period 说「这是首期」，insufficient_history
	// 说「再等几期就好」，对运维含义不同——一个该等，一个该查。
	if len(in.prior) == 0 {
		c.Status = CheckPassed
		c.Reason = "drift_skipped:no_prior_period"
		return c
	}
	hist := make([]float64, 0, len(in.prior))
	for _, p := range in.prior {
		if pr, ok := depositResidual(p.Values); ok {
			hist = append(hist, pr)
		}
	}
	if len(hist) < minDriftHistory {
		c.Status = CheckPassed
		c.Reason = "drift_skipped:insufficient_history"
		return c
	}

	var sum float64
	for _, h := range hist {
		sum += h
	}
	mean := sum / float64(len(hist))
	if drift := math.Abs(r - mean); drift > in.cfg.DepositSumDriftMax {
		c.Status = CheckFailed
		c.Reason = fmt.Sprintf("drift_exceeded: residual %.4f drifted %.4f from %d-period mean %.4f",
			r, drift, len(hist), mean)
		return c
	}
	c.Status = CheckPassed
	return c
}

// depositResidual 算一期的存款加总残差占比。字段不全或总额为零时返回 false。
//
// 历史期次可能缺字段（早期报告的部门划分不同），所以不能假定齐全。算不出来的
// 期次**直接不计入均值**，而不是当成残差 0——那会把均值拉向 0，让正常期次
// 看起来在漂移。
func depositResidual(values map[string]float64) (float64, bool) {
	total, ok := values[FieldDepositFlowYTD]
	if !ok || total == 0 {
		return 0, false
	}
	var sum float64
	for _, f := range depositPartFields {
		v, ok := values[f]
		if !ok {
			return 0, false
		}
		sum += v
	}
	return math.Abs(sum-total) / math.Abs(total), true
}

// gateCorpLoanReconcile 查企业贷款分项加总：短期 + 中长期 + 票据 vs 企业合计。
//
// M0 三期实测残差 1.16% / 1.42% / 1.58%，比存款那道紧得多——企业贷款的三个
// 分项是穷尽的，不像存款的「其中」是部分列举。
//
// # Value 的单位：亿元绝对量，与 deposit_sum 的比例不同
//
// 判定用**比例**（残差占合计的比重，与容差可比），但 Value 记 sum-total 的
// **亿元绝对量并保留符号**——spec 第 7 节的单位约定如此，M0 契约样本也已经
// 这么记（golden2025 得 -1800、golden2020 得 -1203）。
// 两道闸的 Value 单位不同是**刻意**的：deposit_sum 记比例（0.0906），本闸记
// 亿元（-1800）。下游是 Grafana 面板与 pending 人工复核，把 1.16% 读成
// -1203 亿元是量纲错读，所以此处写明。
// 符号表示方向：负值即分项之和小于合计。
func gateCorpLoanReconcile(in gateInput) Check {
	if skip := in.need(FieldLoanCorpTotalYTD, FieldLoanCorpShortYTD,
		FieldLoanCorpMLTYTD, FieldLoanBillYTD); skip != nil {
		return *skip
	}
	total := in.v(FieldLoanCorpTotalYTD)
	// 零分母会算出 Inf 或 NaN，而 Save 拒绝非有限的 Check.Value——那会让整期
	// 既进不了观测表也进不了 pending（savePending 先 json.Marshal 报告）。
	// 一期企业贷款增量恰好为零是可能的，不该因此让数据整个消失。
	if total == 0 {
		return Check{Status: CheckSkipped,
			Reason: "zero_denominator:" + FieldLoanCorpTotalYTD}
	}
	sum := in.v(FieldLoanCorpShortYTD) + in.v(FieldLoanCorpMLTYTD) + in.v(FieldLoanBillYTD)
	residual := sum - total
	r := math.Abs(residual) / math.Abs(total)

	c := Check{Value: &residual}
	if r <= in.cfg.CorpLoanTolerance {
		c.Status = CheckPassed
		return c
	}
	c.Status = CheckFailed
	c.Reason = fmt.Sprintf("residual %.4f exceeds %.4f", r, in.cfg.CorpLoanTolerance)
	return c
}

// gateStockContinuity 查社融存量的环比变化。
//
// 社融存量是累积量，相邻期变化超过阈值说明要么口径变了、要么抽取错了。
//
// ⚠️ StockContinuityMax 的 2% 未经真实数据验证——M0 的两份样本里只有一份
// 含社融，算不出环比。M1c 回填出连续序列后必须重新标定。
//
// Value 是环比变化率（比例），与 deposit_sum 同为比例、与 corp_loan_reconcile
// 的亿元不同。
func gateStockContinuity(in gateInput) Check {
	// 三种跳过理由按「从根本到表面」排优先级，顺序即下面三个 if 的顺序。
	//
	// 本期没有这个字段是最根本的原因，优先报它：v1 期次两个理由同时成立
	// （没有 tsf_stock，且首期时也没有历史）。报「没有历史」会让人去查回填
	// 进度，而真相是那期报告压根没有社融板块——理由会把排查引向错误方向。
	if skip := in.need(FieldTSFStock); skip != nil {
		return *skip
	}
	if len(in.prior) == 0 {
		return Check{Status: CheckSkipped, Reason: "no_prior_period"}
	}

	// prior 按 period 降序，[0] 就是上一期。
	prev, ok := in.prior[0].Values[FieldTSFStock]
	if !ok {
		return Check{Status: CheckSkipped, Reason: "prior_absent_field:" + FieldTSFStock}
	}
	if prev == 0 {
		// 零分母会算出 Inf，而 Save 拒绝非有限的 Check.Value——那会让整期
		// 既进不了观测表也进不了 pending。
		// 与上一条分开：字段在、只是值为 0，与「上一期压根没这个字段」是
		// 两种不同的处境，理由不该混。
		return Check{Status: CheckSkipped, Reason: "zero_denominator:" + FieldTSFStock}
	}

	cur := in.v(FieldTSFStock)
	// 取绝对值：存量下跌同样是跳变。用 cur-prev 会漏掉整个下跌方向，
	// 而社融存量骤降恰恰是最该报警的情形。
	r := math.Abs(cur-prev) / math.Abs(prev)

	c := Check{Value: &r}
	if r <= in.cfg.StockContinuityMax {
		c.Status = CheckPassed
		return c
	}
	c.Status = CheckFailed
	c.Reason = fmt.Sprintf("%s moved %.4f from %g to %g, exceeds %.4f",
		FieldTSFStock, r, prev, cur, in.cfg.StockContinuityMax)
	return c
}

// gateYoYSanity 查同比字段的绝对值，Value 记最大者（单位：百分数）。
//
// 用 _yoy 后缀筛而不是走模板表：_yoy 是 fields.go 里明确的命名规则，这个后缀
// 就是约定本身。区别于 completeness——那里的 tsf_ 前缀只是碰巧与板块归属一致，
// 真相源是模板表。
func gateYoYSanity(in gateInput) Check {
	var worst float64
	var worstField string
	seen := false
	for _, f := range yoyFields() {
		v, ok := in.obs.Values[f]
		if !ok {
			continue
		}
		seen = true
		if a := math.Abs(v); a > worst {
			worst, worstField = a, f
		}
	}
	if !seen {
		return Check{Status: CheckSkipped, Reason: "absent_field:*_yoy"}
	}

	c := Check{Value: &worst}
	if worst <= in.cfg.YoYSanityMax {
		c.Status = CheckPassed
		return c
	}
	c.Status = CheckFailed
	c.Reason = fmt.Sprintf("%s=%.2f exceeds %.2f", worstField, worst, in.cfg.YoYSanityMax)
	return c
}

// yoyFields 是全部同比字段，从 fieldOrder 筛后缀而不是手写第五份清单。
func yoyFields() []string {
	out := make([]string, 0, 16)
	for _, f := range fieldOrder {
		if strings.HasSuffix(f, "_yoy") {
			out = append(out, f)
		}
	}
	return out
}

// gateCompleteness 查必填字段是否齐全。
//
// 在当前设计下它恒真：extract 的纪律是「任何模板未命中一律报错」，Parse 的
// 输出键数只可能是 54 或 27。保留它是**派生的纵深防御**——把这条隐含假设
// 变成显式断言。M1c 加 LLM 兜底后抽取会变成部分成功，那一刻它立刻有意义。
//
// 它也是 obs.Values 为空时唯一会 failed 的闸门（spec 第 9 节）：其余四道各自
// 按 absent 跳过，这道把「整期没有数据」判成不过闸。
func gateCompleteness(in gateInput) Check {
	req := requiredFields(in.obs.Meta.Extractor)
	if req == nil {
		return Check{Status: CheckSkipped,
			Reason: "unknown_extractor:" + in.obs.Meta.Extractor}
	}
	var missing []string
	for _, f := range req {
		if _, ok := in.obs.Values[f]; !ok {
			missing = append(missing, f)
		}
	}
	if len(missing) == 0 {
		return Check{Status: CheckPassed}
	}
	n := float64(len(missing))
	return Check{Status: CheckFailed, Value: &n,
		Reason: "missing " + strconv.Itoa(len(missing)) + ": " +
			strings.Join(firstN(missing, 3), ",")}
}

// firstN 截断切片，免得错误信息列出五十个字段名。
func firstN(s []string, n int) []string {
	if len(s) <= n {
		return s
	}
	return append(slices.Clone(s[:n]), "…")
}

// gateMagnitudeSanity 查各字段是否落在合理区间。
//
// 区间表有意为空到 M1c——必须用回填数据的实际分布标定，不得拍脑袋。
// 表为空时整道闸 skipped{not_calibrated}，填表即生效，无需改代码。
func gateMagnitudeSanity(in gateInput) Check {
	if len(in.cfg.MagnitudeRanges) == 0 {
		return Check{Status: CheckSkipped, Reason: "not_calibrated"}
	}
	// 遍历 fieldOrder 而不是 map：map 迭代顺序随机，同一份数据两次跑会报出
	// 不同的越界字段，让排查变成猜谜。
	for _, f := range fieldOrder {
		r, ok := in.cfg.MagnitudeRanges[f]
		if !ok {
			continue
		}
		v, ok := in.obs.Values[f]
		if !ok {
			continue
		}
		if v < r.Min || v > r.Max {
			out := v
			return Check{Status: CheckFailed, Value: &out,
				Reason: fmt.Sprintf("%s=%g outside [%g,%g] %s", f, v, r.Min, r.Max, r.Unit)}
		}
	}
	return Check{Status: CheckPassed}
}
