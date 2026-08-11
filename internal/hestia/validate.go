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
	{"corp_loan_reconcile", gateCorpLoanReconcile},
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
