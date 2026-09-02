package hestia

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
)

// 关于浮点：增量类字段由 万亿×10000 得出，个别值带 ≤1 ULP 的表示误差
// （实测 4.81×10000 = 48099.99999999999）。**闸门一律不得对它们做精确相等
// 比较**。现有七道全是不等式或容差比较，误差被完全盖住——
// 见 TestTrillionConversionCarriesULPError。
//
// ⚠️ 验证这件事时不能在源码里手算：Go 的**无类型常量**算术是精确的，写
// 4.81*10000 得到精确的 48100，会得出「没有误差」的相反结论。必须用运行时变量。
//
// 相关但不同的一条，写给改边界测试的人：阈值边界用例之所以成立，条件是
// **两边舍入到同一个 double**，不是「比例精确可表示」——0.02 本身就不是
// （位模式 0x3f947ae147ae147b，实为 0.020000000000000000416）。
// 而它成立与否取决于**参与运算的量是否精确**，不取决于算路长短：
// 400→408 这类整数输入成立，123→125.46 就低 15 ULP 而失效。

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

	// PrecedingAll 与 Preceding 相同，但**并上未过闸的 pending**：答「近期发生过什么」，
	// 而不是「上一期可信的观测是什么」（M1c-4 的 TASK-008）。
	//
	// 🔴 **只该喂给统计量类判据**（残差漂移这种「本期相对近期常态」的问题）。
	// 逐期比较类判据（如环比）用它 = 拿未通过校验的数据当基准值。
	// 两者的语义差别与选错的后果，见 store.go 的 PrecedingAll 头注释。
	PrecedingAll(ctx context.Context, period, periodType string, n int) ([]Observation, error)
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

func (noHistory) PrecedingAll(context.Context, string, string, int) ([]Observation, error) {
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

	// 🔴 多取一次「含 pending」的历史（M1c-4 的 TASK-008）：只给统计量类判据用。
	// 两次查询而不是一次取全再过滤 —— 「哪些期次可信」这个判断属于 Store，
	// 在这里过滤等于把它复制一份，而两份迟早分叉。
	priorAll, err := h.PrecedingAll(ctx, obs.Meta.Period, obs.Meta.PeriodType, historyDepth)
	if err != nil {
		return ValidationReport{}, err
	}

	in := gateInput{obs: obs, prior: prior, priorAll: priorAll, cfg: cfg}
	checks := make([]Check, 0, len(gates))
	passed := true
	for _, g := range gates {
		c := g.fn(in)
		c.ID = g.id // 闸门函数不写自己的 ID，避免表与实现两处对不上
		if ex := cfg.exemptionFor(obs.Meta.Period, obs.Meta.PeriodType, g.id); ex != nil {
			// 命中豁免记 skipped 而不是 passed：豁免与通过在数据上必须可分，
			// 把「这次没查」记成「查了没问题」等于伪造一次检查记录。
			//
			// 保留 Value——闸门算出的残差仍是有用的观测，只是不据此判定。
			c = Check{ID: g.id, Status: CheckSkipped, Value: c.Value,
				Reason: "caliber_exemption:" + ex.Version}
		}
		if c.Status == CheckFailed {
			passed = false
		}
		checks = append(checks, c)
	}
	return ValidationReport{Passed: passed, Checks: checks}, nil
}

// knownCheckIDs 从 gates 派生，不手写第二份 ID 列表。
//
// 手写的那份会在闸门集合变动时静默过期，两个方向各有一种后果：
//   - **少了**新增闸门的 ID ⇒ 一条合法的豁免被当成拼写错误拒掉（响亮，会被发现）
//   - **留着**已删除闸门的 ID ⇒ 针对一道**不存在的闸**的豁免照旧通过校验，
//     而它永远不会命中任何闸门（`exemptionFor` 比对的是真实的 gates）。
//     配置看起来仍然有效，实际是一条死配置 —— 这个方向是**静默**的。
//
// 注意**拼错的 ID 任何时候都不会被放行**：checkEnum 的语义是「不在 allowed 内
// 一律返回 error」，与列表是否过期无关。（此处原先写反了，Sprint 035 QA R2-14 纠正。）
func knownCheckIDs() []string {
	out := make([]string, len(gates))
	for i, g := range gates {
		out[i] = g.id
	}
	return out
}

// gateInput 是每道闸门拿到的全部输入。
type gateInput struct {
	obs Observation
	// prior 按 period 降序，可能为空。
	//
	// ⚠️ **prior[0] 不是「上一期」**，是「最近一个**已被接受**的期次」——
	// Preceding 既无相邻性约束、也看不见落 pending 的期次（见 store.go 的 Preceding
	// 头注释）。凡是「相邻两期」才有意义的判定（如 gateStockContinuity 的环比），
	// **必须自己核对期次跨度**，别沿用「[0] 就是上一期」那个假设。
	prior []Observation

	// priorAll 与 prior 相同但**并上 pending**，按 period 降序，可能为空
	//（M1c-4 的 TASK-008）。
	//
	// 🔴 **只有统计量类判据可以读它。** 逐期比较类判据（gateStockContinuity 的环比）
	// 必须继续用 prior —— 用 priorAll 等于拿未通过校验的数据当基准值。
	// 由 validate_test.go 的接线断言守着；那条测试是这条纪律**唯一的机制**，别删。
	priorAll []Observation

	cfg Thresholds
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

// depositPartFieldsMoM 是同四个部门的**当月口径**列，与 depositPartFields **逐项对应**
// （M1c-4 的 TASK-007）。
//
// 顺序必须一致：两族清单的对应关系由 TestDepositPartFieldsAgreeAcrossCalibers 逐项钉住。
// 手写两份而不是从 depositPartFields 派生，是因为字段名不是机械后缀替换就能得到的
// （FieldDepositHouseholdYTD → FieldDepositHouseholdMoM 恰好是，但那是巧合不是约定）——
// 手写就必须有人钉住，那条测试就是钉子。
var depositPartFieldsMoM = []string{
	FieldDepositHouseholdMoM, FieldDepositCorpMoM,
	FieldDepositFiscalMoM, FieldDepositNBFIMoM,
}

// corpLoanPartFields / corpLoanPartFieldsMoM 是企业贷款勾稽的三个分项，两族逐项对应。
//
// ⚠️ 此前这三项是**内联**在 gateCorpLoanReconcile 里的字面量。提成变量是为了让
// TestCorpLoanPartFieldsAgreeAcrossCalibers 有东西可比 —— 两族的对应关系若只存在于
// 两段并列的代码里，分叉时没有任何断言会红。
var (
	corpLoanPartFields = []string{
		FieldLoanCorpShortYTD, FieldLoanCorpMLTYTD, FieldLoanBillYTD,
	}
	corpLoanPartFieldsMoM = []string{
		FieldLoanCorpShortMoM, FieldLoanCorpMLTMoM, FieldLoanBillMoM,
	}
)

// caliberBand 是一道勾稽闸在某个口径下要看的合计列、分项列与容差
// （M1c-4 的 TASK-007）。
//
// 🔴 **两族的容差不能共用一个值**：ytd 的分母是年初至今累计、mom 是单月增量，
// 量级差一个数量级以上，残差占比的分布完全不同。故 tol 是 band 的一部分，
// 而不是在闸里写死 in.cfg.XxxTolerance。
type caliberBand struct {
	name  string // "ytd" / "mom"，进 Reason 用
	total string
	parts []string
	tol   float64 // K_rel：相对容差（残差占比上限）

	// absTol 是 K_abs：**绝对**容差下限（亿元）。0 表示不设下限 ⇒ 判据退化为纯比值
	// `|Σ分项−合计| > tol×|合计|`，与本字段引入之前逐位等价。
	//
	// # 为什么需要它（M1c-4 的 TASK-010，人类裁决）
	//
	// 比值判据 `|Σ分项−合计| / |合计|` 只在**分母远离零**时有意义。实测（79 期语料，
	// 锚 743c507）deposit 的当月族分母 min=447、**且可为负**（2021-07 = −11300），
	// 而 ytd 族分母 min=28800。残差最大的 2022-07 分母仅 447、绝对差额 1319 ——
	// 那个 1319 比该族绝对差额的中位数 2025 **还小**：它不是异常期次，是分母最小的
	// 那一期。纯比值容差要覆盖它得取到 2.95，而那个值在中位分母下允许 38100 亿元的
	// 差额（历史最大仅 8681）⇒ 闸门实质失效。
	//
	// ⚠️ **本 sprint 的 TASK-011 已对另一道闸做过同一件事**：backfill_load_report.go
	// 的 caliberIdentityLimit = max(K_abs, K_rel×|expected|)，其注释写的
	// 「兜 |expected| 极小时相对容差失效」与此处逐字同因。
	//
	// 🔴 **只有 deposit 的 mom 族用它**（人类在 Q1/Q2 分别裁决：mom 换仪器、ytd 保持
	// 纯比值取 0.17）。corp_loan 两族的分母远离零（ytd min=25500 / mom min=2335 且
	// 恒为正）⇒ 比值判据在那一族是有效的，absTol 保持 0。
	absTol float64
}

// bandLimitRatio 是某一族在本期的**实际判定门限，换算成残差占比**。
//
// 提成函数而不是在判定处内联：Reason 要印出门限（读者据此判断「差这么多算不算多」），
// 而**印出来的门限与判定用的门限必须是同一个** —— 两处各算一遍迟早分叉，而分叉的
// 表现是报告说「残差 0.39、门限 0.28」却没有判 failed。这条理由照抄
// backfill_load_report.go 的 caliberIdentityLimit，不另发明。
//
// # 🔴 为什么在**比值域**而不是绝对域比较
//
// 判据的语义是 `|Σ分项−合计| > max(K_abs, K_rel×|合计|)`，两边同除 |合计| 得
// `r > max(K_abs/|合计|, K_rel)`。两种写法数学上等价，**浮点上不等价**：
// 绝对域要算 `r×|合计|`，那一次乘法会引入舍入，使 absTol==0 的族在**恰好等于容差**
// 时的判定可能翻转 —— 而 TestDepositSumBoundaryIsInclusive 正是钉这个边界的
// （它挑的残差与容差字面量实测是同一个 float64，等号成立与否只取决于比较符本身）。
//
// 用比值域写，absTol == 0 时 max(0/|合计|, tol) **恒等于** tol，与引入 absTol 之前
// 的 `r > b.tol` **逐位等价**，不是「大致等价」。
//
// ⚠️ 调用方必须保证 |total| != 0（bandDiagnosis 已经把零分母挡在 skipped 那条路上）。
func bandLimitRatio(b caliberBand, total float64) float64 {
	return math.Max(b.tol, b.absTol/math.Abs(total))
}

// bandDiagnosis 返回空串表示这一族**算得出来**；否则返回一条说清「为什么算不出」
// 的理由，格式与既有的 absent_field: / zero_denominator: 逐字一致。
//
// ⚠️ 沿用既有格式而不是另起一套：既有测试与下游报告都按那两个前缀读
// （validate_test.go 的 "zero_denominator:"+FieldDepositFlowYTD 就是逐字比对）。
func bandDiagnosis(values map[string]float64, b caliberBand) string {
	for _, f := range append([]string{b.total}, b.parts...) {
		if _, ok := values[f]; !ok {
			return "absent_field:" + f
		}
	}
	if values[b.total] == 0 {
		return "zero_denominator:" + b.total
	}
	return ""
}

// pickCaliberBand 在两族里选一族。
//
// 🔴 **两族都齐时取 ytd**（主口径，且容差的标定样本以它为主）；ytd 算不出就试 mom；
// 两族都不齐才 skipped。**顺序是裁决，不是实现细节** —— 靠 map 迭代顺序碰运气的实现
// 会在两族都齐时随机选一族，而两族的容差不同、残差也不同，那意味着同一份数据
// 每次跑出的结论可能不一样。由 TestDepositSumPrefersYTDWhenBothPresent 钉住。
//
// 都算不出时 Reason **同时带上两族各自的诊断** —— 只报一族的话，运维会去查错的列。
func pickCaliberBand(values map[string]float64, bands []caliberBand) (caliberBand, *Check) {
	var why []string
	for _, b := range bands {
		d := bandDiagnosis(values, b)
		if d == "" {
			return b, nil
		}
		why = append(why, b.name+":"+d)
	}
	return caliberBand{}, &Check{Status: CheckSkipped, Reason: strings.Join(why, " ")}
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
	// 🔴 先选族（M1c-4 的 TASK-007）：口径路由之后观测可能整族走 _mom，
	// 闸门只认 _ytd 会让那些期次 skipped{absent} 而**完全不被校验** ——
	// 那比拦错更糟：报告上看不出「这一期没查过」。
	// 🔴 具名字段而不是位置初始化（M1c-4 的 TASK-010 加 absTol 时改）：位置初始化在
	// 结构体加字段时**编译不过**是好事，但改对之后没有任何东西保证「哪个数进了哪个字段」
	// —— tol 与 absTol 都是 float64，写反了照样编译、照样跑，只是判据整个错位。
	b, skip := pickCaliberBand(in.obs.Values, []caliberBand{
		{name: "ytd", total: FieldDepositFlowYTD, parts: depositPartFields,
			tol: in.cfg.DepositSumTolerance},
		{name: "mom", total: FieldDepositFlowMoM, parts: depositPartFieldsMoM,
			tol: in.cfg.DepositSumToleranceMoM, absTol: in.cfg.DepositSumToleranceMoMAbs},
	})
	if skip != nil {
		return *skip
	}
	r, ok := depositResidualOf(in.obs.Values, b.total, b.parts)
	if !ok {
		// 走不到：pickCaliberBand 已经保证这一族字段齐全且分母非零。
		return Check{Status: CheckSkipped, Reason: "zero_denominator:" + b.total}
	}

	c := Check{Value: &r} // Value 恒为绝对残差占比，与 M0 契约样本一致

	// 判据一优先：绝对值都超了，再谈漂移没有意义。
	// ⚠️ 比的是**选出的那一族的**容差，不是写死的 in.cfg.DepositSumTolerance；
	// Reason 带上 total 列名 —— 两族容差不同，不写清是哪一族没法复核。
	// 🔴 判据的**语义**是 `|Σ分项−合计| > max(K_abs, K_rel×|合计|)`（M1c-4 的 TASK-010），
	// 但**实际在比值域比较**：两边同除 |合计| 得 `r > max(K_abs/|合计|, K_rel)`。
	// absTol==0 的族（ytd、以及 corp_loan 两族）逐位等价于原来的 `r > b.tol`。
	// Reason 印出**实际用的那个门限**，理由见 bandLimitRatio 的注释。
	//
	// ⚠️ 本注释原文是「🔴 在**绝对域**比较……理由见 bandLimit 的注释」，**两处都已失真**
	// （M1c-4 的 TASK-014 订正）：
	//   - 「绝对域」描述的是 TASK-010 **已实测证否并废弃**的第一版实现 —— 绝对域要算
	//     `r×|合计|`，那次乘法的舍入会让 absTol==0 的族在**恰好等于容差**时判定翻转，
	//     `TestDepositSumBoundaryIsInclusive` 当场变红。换成比值域正是为了消掉它。
	//   - `bandLimit` 这个标识符**从未存在**，函数叫 `bandLimitRatio`（名字里的 Ratio
	//     恰恰就是「在比值域」的意思）。
	// ⇒ 留着它比没有更糟：它读起来像一份有据可查的设计说明，而它描述的那个设计已被推翻。
	tot := math.Abs(in.v(b.total))
	limit := bandLimitRatio(b, tot)
	if r > limit {
		c.Status = CheckFailed
		// `exceeds %.4f` 印的就是**判定用的那个门限**（比值域），与 residual 同量纲。
		// 括号里再给一次绝对量与算式，让「为什么这一期过/不过」不必自己换算：
		// K_abs=0 的族（ytd、corp_loan 两族）括号里的 max 第一项恒为 0，读起来即
		// 「门限就是 K_rel」。
		c.Reason = fmt.Sprintf(
			"tolerance_exceeded[%s]: residual %.4f exceeds %.4f "+
				"(=max(K_abs %.0f/|%.0f|, K_rel %.4f)，绝对门限 %.0f 亿元, total=%s)",
			b.name, r, limit, b.absTol, tot, b.tol, limit*tot, b.total)
		return c
	}

	// 判据二。历史不足时绝对值确实查过并通过了，所以记 passed 而不是 skipped——
	// 但漂移没查这件事不能丢，写进 Reason。
	//
	// 两个理由刻意可分：no_prior_period 说「这是首期」，insufficient_history
	// 说「再等几期就好」，对运维含义不同——一个该等，一个该查。
	// 🔴 基线改用 priorAll（**含未过闸的 pending**），M1c-4 的 TASK-008。
	//
	// 真跑实测的正反馈链：2024-04…08 五期被 tolerance 拒（残差 0.1460/0.1490/0.1505/
	// 0.1663/0.1211）⇒ 不进权威表 ⇒ 基线看不见它们 ⇒ 2024-10/11 因「偏离一个虚高的
	// 3 期均值 0.0784」被 drift 拒。⇒ **闸门自己制造了它要检测的异常。**
	//
	// 正当性：基线用的是**残差这个统计量**，不是那些期次的字段值。drift 问的是
	// 「本期残差相对近期常态是否突变」，若常态只由已通过这道闸的期次组成，
	// **定义本身就是循环的**。与 M1c-3b 的 TASK-011 修的 stock_continuity 同族缺陷。
	if len(in.priorAll) == 0 {
		c.Status = CheckPassed
		c.Reason = "drift_skipped:no_prior_period"
		return c
	}

	// 🔴 相邻性约束（M1c-4 的 TASK-008）：priorAll[0] 是「最近发生过的一期」，
	// **不保证是上一期**。跨度不对时 skip，**不按跨度放宽** —— 放宽需要一个放宽系数，
	// 而语料里没有数据能回答「跨 13 个月该放宽多少」；往闸里塞一个没测过的数，
	// 正是本任务在修的那类缺陷本身（沿 M1c-3b 的 TASK-011 对 stock_continuity 的裁决）。
	//
	// ⚠️ 两种错的代价不对称：误 skip 只损失一次检查**且 reason 可见**，误 fail 不可自愈。
	if gap, ok := periodGapMonths(in.priorAll[0].Meta.Period, in.obs.Meta.Period); !ok ||
		gap != expectedPeriodGapMonths(in.obs.Meta.PeriodType) {
		c.Status = CheckPassed
		c.Reason = fmt.Sprintf("drift_skipped:non_adjacent_prior (%s → %s)",
			in.priorAll[0].Meta.Period, in.obs.Meta.Period)
		return c
	}

	hist := make([]float64, 0, len(in.priorAll))
	for _, p := range in.priorAll {
		// 🔴 历史残差必须用**同一族**算：拿 ytd 的残差去和 mom 的均值比，
		// 两个分母量级差一个数量级，漂移判定完全失去意义。
		if pr, ok := depositResidualOf(p.Values, b.total, b.parts); ok {
			hist = append(hist, pr)
		}
	}
	if len(hist) < minDriftHistory {
		c.Status = CheckPassed
		// 🔴 **说清是「本族」样本不足并带上实际 n**（M1c-4 的 TASK-008）。
		//
		// 笼统记成 insufficient_history 会与「新库冷启动」混成一格，而两者成因完全不同：
		// 冷启动等几期就好；**本族样本不足是结构性的** —— mom 与 ytd 期次在时间轴上交错，
		// 一个 mom 期次的前几期往往全是 ytd，depositResidualOf 对它们逐个返回 false。
		// ⇒ 那批新救回的观测，drift 闸可能**恒 skip 而完全没有保护**，报告上只有这一行。
		// 带上 n 与族名，是为了让「没保护」这件事在报告里可数、可查。
		c.Reason = fmt.Sprintf("drift_skipped:insufficient_same_caliber_history (%s n=%d<%d, prior=%d)",
			b.name, len(hist), minDriftHistory, len(in.priorAll))
		return c
	}

	var sum float64
	for _, h := range hist {
		sum += h
	}
	mean := sum / float64(len(hist))
	if drift := math.Abs(r - mean); drift > in.cfg.DepositSumDriftMax {
		c.Status = CheckFailed
		c.Reason = fmt.Sprintf("drift_exceeded[%s]: residual %.4f drifted %.4f from %d-period mean %.4f (total=%s)",
			b.name, r, drift, len(hist), mean, b.total)
		return c
	}
	c.Status = CheckPassed
	return c
}

// depositResidualOf 算一期的加总残差占比（M1c-4 的 TASK-007 由 depositResidual 泛化
// 成按口径族取列）。字段不全或总额为零时返回 false。
//
// 历史期次可能缺字段（早期报告的部门划分不同），所以不能假定齐全。算不出来的
// 期次**直接不计入均值**，而不是当成残差 0——那会把均值拉向 0，让正常期次
// 看起来在漂移。
func depositResidualOf(values map[string]float64, total string, parts []string) (float64, bool) {
	tot, ok := values[total]
	if !ok || tot == 0 {
		return 0, false
	}
	var sum float64
	for _, f := range parts {
		v, ok := values[f]
		if !ok {
			return 0, false
		}
		sum += v
	}
	return math.Abs(sum-tot) / math.Abs(tot), true
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
	// 先选族，理由同 gateDepositSum（M1c-4 的 TASK-007）。
	//
	// ⚠️ **零分母仍然走 skipped 而不是 failed**：Inf/NaN 会被 Save 拒绝，那会让整期
	// 既进不了观测表也进不了 pending（savePending 先 json.Marshal 报告）。一期企业贷款
	// 增量恰好为零是可能的，不该因此让数据整个消失。这条纪律没有变，只是判定它的地方
	// 挪进了 bandDiagnosis —— 两族都零分母时 Reason 会同时带上两族的 zero_denominator:。
	// absTol 刻意不设（保持 0 ⇒ 纯比值）：本闸两族的分母都远离零（ytd min=25500 /
	// mom min=2335 且恒为正，实测 79 期语料）⇒ 比值判据在这里是有效的仪器。
	b, skip := pickCaliberBand(in.obs.Values, []caliberBand{
		{name: "ytd", total: FieldLoanCorpTotalYTD, parts: corpLoanPartFields,
			tol: in.cfg.CorpLoanTolerance},
		{name: "mom", total: FieldLoanCorpTotalMoM, parts: corpLoanPartFieldsMoM,
			tol: in.cfg.CorpLoanToleranceMoM},
	})
	if skip != nil {
		return *skip
	}
	total := in.v(b.total)
	var sum float64
	for _, f := range b.parts {
		sum += in.v(f)
	}
	residual := sum - total
	r := math.Abs(residual) / math.Abs(total)

	c := Check{Value: &residual}
	// 🔴 经 bandLimitRatio 而不是直接比 b.tol（M1c-4 的 TASK-014）。
	//
	// ⚠️ **今天这是陷阱不是活缺陷**：本闸两族的 absTol 都是 0（见上面「absTol 刻意不设」
	// 那段的实测理由），而 `math.Max(tol, 0/|total|)` 对有限非零 total 恒等于 tol
	// （0.0/|total| == +0.0，tol > 0）⇒ 改前改后的判定**逐位相同**，真语料上一个
	// 字节都不会变。pickCaliberBand 已保证 total != 0。
	//
	// 改它是为了消除一个**只在将来发作**的静默失效面：改前谁给这两族填一个 absTol，
	// 会被直接忽略 —— 编译通过、测试全绿、报告照出，没有任何东西会告诉他。
	// 由 TestCorpLoanReconcileGoesThroughBandLimitRatio（接线）与
	// TestCorpLoanBandRespectsAbsTol（band 层）合起来守着。
	if r <= bandLimitRatio(b, math.Abs(total)) {
		c.Status = CheckPassed
		return c
	}
	c.Status = CheckFailed
	// Reason 带上族名与 total 列名：两族容差不同，不写清是哪一族没法复核。
	c.Reason = fmt.Sprintf("residual %.4f exceeds %.4f [%s, total=%s]", r, b.tol, b.name, b.total)
	return c
}

// gateStockContinuity 查社融存量的环比变化。
//
// 社融存量是累积量，相邻期变化超过阈值说明要么口径变了、要么抽取错了。
//
// ⚠️ StockContinuityMax 的两档（monthly 0.02 / 其余 0.15）**都仍是占位数**——
// M0 的两份样本里只有一份含社融，算不出环比。M1c-2 的 TASK-001 只把它从单值改成
// 按 period_type 分档（相邻两期相隔 1 个月 vs 12 个月），**没有**用真实数据标定
// 取值；标定由 M1c-3 拿回填序列做。
//
// Value 是环比变化率（比例），与 deposit_sum 同为比例、与 corp_loan_reconcile
// 的亿元不同。
func gateStockContinuity(in gateInput) Check {
	// 四种跳过理由按「从根本到表面」排优先级，顺序即下面四个 if 的顺序。
	//
	// 缺档排第一：它是这道闸**能否判定的前提**，且与本期数据无关——整条序列的每
	// 一期都会命中，改一处配置就全好。先报数据侧理由会把排查引向逐期查数据，而
	// 那些期次的数据可能一点问题都没有。
	//
	// ⚠️ 记 skipped 而不是 failed：period_type 已经从 3 种扩到 5 种一次
	// （Sprint 037 补 q1 / q1_q3）。下次再扩时若判 failed，那一批期次会**全部进
	// pending**，而真实情况只是「还没给这种序列定上限」。装载时的缺档是另一回事
	// ——那是配置疏漏，由 LoadConfig 响亮拒绝。
	limit, ok := in.cfg.StockContinuityMax[in.obs.Meta.PeriodType]
	if !ok {
		return Check{Status: CheckSkipped,
			Reason: "no_threshold:" + in.obs.Meta.PeriodType}
	}

	// 本期没有这个字段是最根本的数据侧原因，优先报它：v1 期次两个理由同时成立
	// （没有 tsf_stock，且首期时也没有历史）。报「没有历史」会让人去查回填
	// 进度，而真相是那期报告压根没有社融板块——理由会把排查引向错误方向。
	if skip := in.need(FieldTSFStock); skip != nil {
		return *skip
	}
	if len(in.prior) == 0 {
		return Check{Status: CheckSkipped, Reason: "no_prior_period"}
	}

	// 🔴 **prior[0] 是「最近一个【已被接受】的期次」，不是「上一期」**
	// （M1c-3b 的 TASK-011 返工，缺陷 C-1）。
	//
	// Preceding 的 SQL 只有 `WHERE period < ? AND period_type = ?`，**没有相邻性约束**，
	// 且它查的是 viewCurrent —— 落 pending 的期次它看不见。此处原先写着「prior[0] 就是
	// 上一期」，那句话**只在从未拒过任何一期时成立，而这道闸的存在前提就是会拒**。
	//
	// 真跑实测：4 条 stock_continuity 拒绝，基线跨 10 / 11 / 13 个月、跨 3 年；且构成
	// **正反馈**——拒绝 → 基线跨度变长 → 环比更大 → 更多拒绝。而 --force 对已入权威表的
	// 期次是数据层 no-op ⇒ **拦错了没有出路**。报告还会把它呈现成数据异常，运维会去查央行。
	//
	// ⚠️ 排在字段/零分母检查**之前**：这条问的是「这个基线**够不够格**当基线」，比
	// 「它有没有 tsf_stock」更根本。顺序反了会对一个 13 个月前的期次报
	// prior_absent_field，把排查引向那一期的数据——与本函数开头那段「理由会把排查引向
	// 错误方向」是同一个错误。
	if gap, ok := periodGapMonths(in.prior[0].Meta.Period, in.obs.Meta.Period); !ok ||
		gap != expectedPeriodGapMonths(in.obs.Meta.PeriodType) {
		return Check{Status: CheckSkipped, Reason: fmt.Sprintf(
			"non_adjacent_prior:%s(gap=%d,want=%d)", in.prior[0].Meta.Period,
			gap, expectedPeriodGapMonths(in.obs.Meta.PeriodType))}
	}

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
	if r <= limit {
		c.Status = CheckPassed
		return c
	}
	c.Status = CheckFailed
	c.Reason = fmt.Sprintf("%s moved %.4f from %g to %g, exceeds %.4f",
		FieldTSFStock, r, prev, cur, limit)
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
	// merged@v1 的必填集取决于由哪几篇合成，requiredFields 只拿得到一个字符串，
	// 结构上说不出来（见 TestRequiredFieldsRejectsBareMerged）。改问 Parts。
	// 不改 Validate 的签名：gateInput 已持有 obs，改签名要动 39 个调用点、4 个文件。
	if in.obs.Meta.Extractor == extractorMerged {
		req = mergedRequiredFields(in.obs.Parts)
	}
	// 🔴 判据是 len(req) == 0，**不能写成 req == nil**（M1c-3b 的 TASK-011）。
	// mergedRequiredFields 的 out := make([]string, 0, len(want)) **永远返回非 nil 切片**，
	// 故 req == nil 对 merged@v1 恒不命中 ⇒ 一路落到 len(missing)==0 返回 CheckPassed：
	// 一个 Parts 为空的合并观测被判「completeness 通过」而它一个字段都没查。
	// 那比本任务要修的 skipped 更糟——skipped 在报告里可见，passed 完全静默。
	// 见 TestMergedCompletenessSkipsWhenPartsYieldNoFields。
	//
	// 对非 merged 路径**逐字等价**：requiredFields 对未知 extractor 返回 nil，
	// 而 len(nil) == 0 同样成立。
	if len(req) == 0 {
		return Check{Status: CheckSkipped,
			Reason: "unknown_extractor:" + in.obs.Meta.Extractor}
	}
	// 🔴 口径感知（M1c-4 的 TASK-006）：*_ytd 与 *_mom 任一在场即不算缺。
	//
	// 真语料里 54 篇的分部门段走当月口径，硬要 *_ytd 会让它们恒报「缺一整族字段」——
	// 那是把 **absent-by-design 记成 failed**，completeness 这个指标就废了
	// （types.go 原话，mergedRequiredFields 因同一理由存在）。
	//
	// ⚠️ 变的**只有这一行**：req 的来源（requiredFields / mergedRequiredFields 的选择、
	// len(req)==0 ⇒ skipped{unknown_extractor}）、Value: &n、firstN 截断都一行不动。
	missing := missingCaliberAware(req, in.obs.Values)
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

// expectedPeriodGapMonths 返回某种 period_type 的相邻两期应当相隔几个月。
//
// 复用 thresholds.go 里 StockContinuityMax 分档时已经写下的事实：「四档同为年初起
// 累计口径、相邻两期一律相隔 12 个月」，只有 monthly 是 1 个月。**不另立一张表**——
// 两份清单会分叉，而先错的一定是后抄的那份。
func expectedPeriodGapMonths(periodType string) int {
	if periodType == periodTypeMonthly {
		return 1
	}
	return 12
}

// periodGapMonths 返回 from 到 to 相隔几个月；period 不是 "YYYY-MM" 时返回 false。
//
// 解析失败按**不相邻**处理（调用点的 `!ok ||`）：拿一个解析不出来的期次当基线，
// 与拿一个跨了三年的当基线一样没有依据。Meta.validate 已经挡住非法 period，
// 这条是纵深防御，不是主要防线。
func periodGapMonths(from, to string) (int, bool) {
	a, err := time.Parse("2006-01", from)
	if err != nil {
		return 0, false
	}
	b, err := time.Parse("2006-01", to)
	if err != nil {
		return 0, false
	}
	return (b.Year()-a.Year())*12 + int(b.Month()) - int(a.Month()), true
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
