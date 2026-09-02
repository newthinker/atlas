package hestia

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeHistory 让闸门测试不必建库。切片按 period 降序，与 Store.Preceding 的契约一致。
type fakeHistory struct {
	prior []Observation
	err   error

	// priorAll 是「含 pending」的那一份（M1c-4 的 TASK-008）。
	//
	// 🔴 **默认与 prior 不同**：为 nil 时 PrecedingAll 返回 prior，让既有用例语义不变；
	// 显式赋值时两者可以给出**不同的结论**，接线断言就是靠这个分辨
	// 「某道闸读的到底是 prior 还是 priorAll」。
	priorAll []Observation

	// errAll 只让 PrecedingAll 失败（Preceding 仍成功）。
	//
	// 复用 err 做不到这件事：那样 Preceding 会**先**失败并提前返回，
	// PrecedingAll 的错误传播分支一次都跑不到，而测试照样绿 —— 断言会由
	// 另一条路径满足。要的是**这一条**路径。
	errAll error
}

func (f fakeHistory) Preceding(_ context.Context, _, _ string, n int) ([]Observation, error) {
	if f.err != nil {
		return nil, f.err
	}
	if n < len(f.prior) {
		return f.prior[:n], nil
	}
	return f.prior, nil
}

func (f fakeHistory) PrecedingAll(_ context.Context, _, _ string, n int) ([]Observation, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.errAll != nil {
		return nil, f.errAll
	}
	src := f.priorAll
	if src == nil {
		src = f.prior // 未显式给出时与 prior 同 —— 既有用例的语义不变
	}
	if n < len(src) {
		return src[:n], nil
	}
	return src, nil
}

// obsFrom 用 golden 数据造一个观测。复制 Values——测试会改动它，
// 而 golden 是包级变量，改坏了会污染其他测试。
// priorMeta 返回「往前第 i 期」的 Meta（i 从 0 起），期次按 validMeta 的 h1 口径
// 逐期回退一年（M1c-4 的 TASK-008 加了 drift 相邻性约束之后必需）。
//
// ⚠️ 此前各处夹具都直接用 validMeta()，于是「上一期」与「本期」同期、跨度 0。
// 那在没有相邻性约束时无害，加了之后每一格都会被判 non_adjacent_prior ——
// **不是新约束错了，是夹具本来就不该让两者同期，只是此前没有断言看得见。**
func priorMeta(i int) Meta {
	m := validMeta()
	m.Period = fmt.Sprintf("%04d-06", 2025-i) // 2025-06 / 2024-06 / 2023-06 …
	return m
}

func obsFrom(values map[string]float64, extractor string) Observation {
	m := validMeta()
	m.Extractor = extractor
	return Observation{Meta: m, Values: maps.Clone(values)}
}

// 两期真实报告上，没有任何闸门该失败。
//
// 这是最重要的一条：闸门的价值在于「异常时响、正常时静」，而在真实数据上
// 误报是最贵的失败模式——它会让人开始忽略告警。
func TestValidateOnGoldenSamples(t *testing.T) {
	tests := []struct {
		name      string
		values    map[string]float64
		extractor string
	}{
		{"2025 全年 rule@v2", golden2025, extractorV2},
		{"2020 上半年 rule@v1", golden2020, extractorV1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rep, err := Validate(context.Background(),
				obsFrom(tt.values, tt.extractor), NoHistory, DefaultThresholds())
			require.NoError(t, err)

			for _, c := range rep.Checks {
				assert.NotEqual(t, CheckFailed, c.Status,
					"闸门 %s 在真实数据上不该失败：%s", c.ID, c.Reason)
			}
			assert.True(t, rep.Passed)
		})
	}
}

// 每道闸门都要有一个能让它失败的构造样本。
//
// 只测绿色路径证明不了闸门在工作：恒返回 passed 的实现同样能通过。
func TestGatesRejectMalformedData(t *testing.T) {
	tests := []struct {
		name   string
		id     string
		mutate func(map[string]float64)
		reason string
	}{
		{
			"M1 超过 M2", "monetary_hierarchy",
			func(v map[string]float64) { v[FieldM1] = v[FieldM2] + 1 },
			"m2=",
		},
		{
			"企业贷款短期项翻三倍", "corp_loan_reconcile",
			func(v map[string]float64) { v[FieldLoanCorpShortYTD] *= 3 },
			"exceeds",
		},
		{
			"贷款余额同比 88%", "yoy_sanity",
			func(v map[string]float64) { v[FieldLoanBalanceYoY] = 88 },
			"exceeds",
		},
		{
			"删掉一个必填字段", "completeness",
			func(v map[string]float64) { delete(v, FieldRateIBO) },
			"missing",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obs := obsFrom(golden2025, extractorV2)
			tt.mutate(obs.Values)

			rep, err := Validate(context.Background(), obs, NoHistory, DefaultThresholds())
			require.NoError(t, err, "闸门失败不该变成 Go error")
			assert.False(t, rep.Passed)

			c := findCheck(t, rep, tt.id)
			assert.Equal(t, CheckFailed, c.Status)
			assert.Contains(t, c.Reason, tt.reason)
		})
	}
}

func findCheck(t *testing.T, rep ValidationReport, id string) Check {
	t.Helper()
	for _, c := range rep.Checks {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("报告里没有 %s；实际有 %d 道闸门", id, len(rep.Checks))
	return Check{}
}

// 区间表一填就生效，不用改代码。
func TestMagnitudeSanityActivatesWhenCalibrated(t *testing.T) {
	cfg := DefaultThresholds()
	cfg.MagnitudeRanges = map[string]Range{
		FieldFXReserve: {Min: 3, Max: 4, Unit: "万亿美元"},
	}

	obs := obsFrom(golden2025, extractorV2)
	obs.Values[FieldFXReserve] = 42 // 越界两个数量级

	rep, err := Validate(context.Background(), obs, NoHistory, cfg)
	require.NoError(t, err)

	c := findCheck(t, rep, "magnitude_sanity")
	assert.Equal(t, CheckFailed, c.Status)
	assert.Contains(t, c.Reason, "万亿美元", "区间的单位要出现在错误信息里")
}

// 表为空时这道闸沉默——区间必须用 M1c 回填分布标定，不得拍脑袋。
func TestMagnitudeSanitySkipsWhenUncalibrated(t *testing.T) {
	rep, err := Validate(context.Background(),
		obsFrom(golden2025, extractorV2), NoHistory, DefaultThresholds())
	require.NoError(t, err)

	c := findCheck(t, rep, "magnitude_sanity")
	assert.Equal(t, CheckSkipped, c.Status)
	assert.Equal(t, "not_calibrated", c.Reason)
}

// v1 期次天然缺 27 个字段。缺字段的闸门必须 skip 而不是 fail——
// 「没数据」和「数据有问题」是两回事。
func TestGatesSkipOnAbsentFields(t *testing.T) {
	obs := obsFrom(golden2020, extractorV1) // 无社融板块

	rep, err := Validate(context.Background(), obs, NoHistory, DefaultThresholds())
	require.NoError(t, err)
	assert.True(t, rep.Passed, "字段天然缺失不该让整期不过闸")

	// completeness 用的是 v1 的必填集，所以它 passed 而不是 skipped
	assert.Equal(t, CheckPassed, findCheck(t, rep, "completeness").Status)
}

// 每一个 skipped 都必须带 reason——Save 会拒绝没有 reason 的 skip。
func TestEverySkippedCheckHasReason(t *testing.T) {
	for _, values := range []map[string]float64{golden2025, golden2020} {
		ext := extractorV2
		if len(values) == 27 {
			ext = extractorV1
		}
		rep, err := Validate(context.Background(),
			obsFrom(values, ext), NoHistory, DefaultThresholds())
		require.NoError(t, err)

		for _, c := range rep.Checks {
			if c.Status == CheckSkipped {
				assert.NotEmpty(t, c.Reason, "闸门 %s 跳过却没说为什么", c.ID)
			}
		}
	}
}

// 查库失败是基础设施故障，必须返回 error，而不是记成某道闸门失败。
func TestValidateReturnsErrorOnHistoryFailure(t *testing.T) {
	want := errors.New("database is locked")
	_, err := Validate(context.Background(), obsFrom(golden2025, extractorV2),
		fakeHistory{err: want}, DefaultThresholds())

	require.Error(t, err)
	assert.ErrorIs(t, err, want, "底层错误要能被 errors.Is 找到")
	// ErrorIs 单独证不了「包住了」——直接 return err 的实现同样让它为真。
	// 要的是包裹，所以断言 Unwrap 之后确实还有内层。
	require.NotNil(t, errors.Unwrap(err), "必须用 %w 包住，而不是原样返回底层 err")
	assert.ErrorContains(t, err, "validate", "包裹要带上是哪一步失败的")
}

// nil History 一律是接线错误。让它退化成「所有需要历史的闸门都 skipped」
// 会把一个 bug 变成一份看起来正常的报告。
func TestValidateRejectsNilHistory(t *testing.T) {
	_, err := Validate(context.Background(), obsFrom(golden2025, extractorV2),
		nil, DefaultThresholds())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "NoHistory")
}

// 配置非法要立刻响亮失败，而不是让闸门带着错阈值跑完。
func TestValidateRejectsInvalidConfig(t *testing.T) {
	cfg := DefaultThresholds()
	cfg.CaliberExemptions = []CaliberExemption{{
		Version: "2099-01", Period: "2025-01",
		SkipChecks: []string{"deposit_sum"}, Reason: "编造的版本",
	}}

	_, err := Validate(context.Background(), obsFrom(golden2025, extractorV2),
		NoHistory, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown")
}

// —— 以下四条是 DoD 追加的要求，不在上游计划的 Step 1-12 里 ——

// Check.Value 的单位必须符合 spec 第 7 节，各闸门口径不同，逐条钉住。
//
// 计划原文写的是 `c := Check{Value: &r}`（r 是**比例**，golden2025 得 0.0116），
// 与 spec 第 7 节和 M0 契约样本冲突（后两者要亿元绝对量）。Leader 裁定以 spec 为准。
// 这条测试就是那个裁定的守卫——在它之前，量纲错读**不会有任何测试转红**。
//
// 期望值不是抄实现算出来的：-1800 与 -1203 分别是 spec 第 7 节的举例与 M0 契约
// 样本里已经存在的数（`corp_loan_reconcile: -1203`），两者独立于本实现。
func TestCheckValueUnitsFollowSpec(t *testing.T) {
	tests := []struct {
		name         string
		values       map[string]float64
		extractor    string
		wantResidual float64
	}{
		// 152900 - 154700，spec 第 7 节的举例量级
		{"2025 全年", golden2025, extractorV2, -1800},
		// 86497 - 87700，与 M0 契约样本里记的 -1203 逐字一致
		{"2020 上半年", golden2020, extractorV1, -1203},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rep, err := Validate(context.Background(),
				obsFrom(tt.values, tt.extractor), NoHistory, DefaultThresholds())
			require.NoError(t, err)

			corp := findCheck(t, rep, "corp_loan_reconcile")
			require.NotNil(t, corp.Value, "corp_loan_reconcile 必须记 Value")
			assert.Equal(t, tt.wantResidual, *corp.Value,
				"Value 必须是 sum-total 的亿元绝对量并保留符号，不是比例——"+
					"记成比例会让 Grafana 与人工复核把 1.16%% 读成 -1203 亿元")

			// 同一份数据独立算一遍最大同比，不复用 yoyFields()——
			// 用被验对象的尺子量被验对象，口径错了两边会一起错。
			var wantWorst float64
			for f, v := range tt.values {
				if strings.HasSuffix(f, "_yoy") && math.Abs(v) > wantWorst {
					wantWorst = math.Abs(v)
				}
			}
			yoy := findCheck(t, rep, "yoy_sanity")
			require.NotNil(t, yoy.Value, "yoy_sanity 必须记 Value")
			assert.Equal(t, wantWorst, *yoy.Value, "Value 是最大同比绝对值（百分数）")
			assert.Less(t, *yoy.Value, 100.0, "百分数量级：真实数据的同比不该到三位数")

			assert.Nil(t, findCheck(t, rep, "monetary_hierarchy").Value,
				"monetary_hierarchy 判的是序关系，没有单一实测值可记")
		})
	}
}

// 阈值边界方向必须被守住：实现是 <=，恰好等于阈值要判 passed。
//
// reviewer 消融实测：把这两道闸的 <= 改成 <，计划的全部测试**无一条转红**。
// 本测试就是补上那道守卫——「恰好等于」是唯一能分辨 <= 与 < 的输入。
//
// 选数纪律：全部用 2 的幂，让比例在 float64 下**精确可表示**，否则测的是
// 浮点舍入而不是判定逻辑。131072=2^17，8192=2^13 ⇒ 8192/131072 = 1/16 = 0.0625；
// 12288=3*2^12 ⇒ 12288/131072 = 3/32 = 0.09375。两者都精确。
func TestGateBoundariesAreInclusive(t *testing.T) {
	const (
		corpTotal   = 131072.0
		exactlyAt   = 8192.0  // 残差 ⇒ 比例恰好 0.0625
		clearlyOver = 12288.0 // 残差 ⇒ 比例 0.09375
	)

	corpCfg := DefaultThresholds()
	corpCfg.CorpLoanTolerance = 0.0625 // = 1/16，精确可表示

	corpWith := func(residual float64) Observation {
		obs := obsFrom(golden2025, extractorV2)
		obs.Values[FieldLoanCorpTotalYTD] = corpTotal
		obs.Values[FieldLoanCorpShortYTD] = corpTotal - residual
		obs.Values[FieldLoanCorpMLTYTD] = 0
		obs.Values[FieldLoanBillYTD] = 0
		return obs
	}

	t.Run("corp_loan 残差恰好等于容差判 passed", func(t *testing.T) {
		rep, err := Validate(context.Background(), corpWith(exactlyAt), NoHistory, corpCfg)
		require.NoError(t, err)
		c := findCheck(t, rep, "corp_loan_reconcile")
		assert.Equal(t, CheckPassed, c.Status,
			"实现是 <=，恰好等于容差必须通过；这里变红说明比较符被改成了 <")
		require.NotNil(t, c.Value)
		assert.Equal(t, -exactlyAt, *c.Value)
	})

	t.Run("corp_loan 残差超出容差判 failed", func(t *testing.T) {
		rep, err := Validate(context.Background(), corpWith(clearlyOver), NoHistory, corpCfg)
		require.NoError(t, err)
		assert.Equal(t, CheckFailed, findCheck(t, rep, "corp_loan_reconcile").Status)
	})

	// YoYSanityMax 默认 50，整数精确可表示；50.5 同样精确（=101/2）。
	yoyWith := func(v float64) Observation {
		obs := obsFrom(golden2025, extractorV2)
		obs.Values[FieldLoanBalanceYoY] = v
		return obs
	}

	t.Run("yoy 恰好等于上限判 passed", func(t *testing.T) {
		rep, err := Validate(context.Background(), yoyWith(50), NoHistory, DefaultThresholds())
		require.NoError(t, err)
		c := findCheck(t, rep, "yoy_sanity")
		assert.Equal(t, CheckPassed, c.Status,
			"实现是 <=，恰好等于上限必须通过；这里变红说明比较符被改成了 <")
		require.NotNil(t, c.Value)
		assert.Equal(t, 50.0, *c.Value, "最大同比就是我们设的那个，否则测的不是边界")
	})

	t.Run("yoy 略超上限判 failed", func(t *testing.T) {
		rep, err := Validate(context.Background(), yoyWith(50.5), NoHistory, DefaultThresholds())
		require.NoError(t, err)
		assert.Equal(t, CheckFailed, findCheck(t, rep, "yoy_sanity").Status)
	})
}

// spec 第 9 节：空 Values 由 completeness 自然拦下，Validate 入口**不加特判**。
//
// 当前行为正确，但计划没有任何测试验证它——一条论证充分的设计断言没有回归防线，
// 将来有人在入口加一句 `if len(obs.Values)==0 { return ... }` 不会有东西转红。
// nil 与空 map 都要测：Go 里读 nil map 合法，但两者在别处常被写成不同分支。
func TestValidateHandlesEmptyValuesWithoutSpecialCase(t *testing.T) {
	tests := []struct {
		name   string
		values map[string]float64
	}{
		{"空 map", map[string]float64{}},
		{"nil map", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := validMeta()
			m.Extractor = extractorV2
			obs := Observation{Meta: m, Values: tt.values}

			rep, err := Validate(context.Background(), obs, NoHistory, DefaultThresholds())
			require.NoError(t, err, "空 Values 是正常输入，不是基础设施故障")
			assert.False(t, rep.Passed, "整期没有数据必须不过闸")

			// 由 completeness 拦下，而不是靠入口特判
			comp := findCheck(t, rep, "completeness")
			assert.Equal(t, CheckFailed, comp.Status, "空 Values 该由 completeness 判 failed")
			assert.Contains(t, comp.Reason, "missing")

			// 其余四道各自按 absent/未标定跳过，且都带 reason
			for _, c := range rep.Checks {
				if c.ID == "completeness" {
					continue
				}
				assert.Equal(t, CheckSkipped, c.Status, "闸门 %s 该跳过而不是失败", c.ID)
				assert.NotEmpty(t, c.Reason, "闸门 %s 跳过却没说为什么", c.ID)
			}
		})
	}
}

// 任何输入下报告必须逐行对应全部闸门——闸门「整个消失」是最难发现的失败模式：
// 报告看起来正常，只是少了一行，Passed 照样可能是 true。
//
// 期望值写成**字面量**而不是 len(gates)/gates[i].id：拿 gates 当期望值是用被验
// 对象的尺子量被验对象——从表里删掉一道闸时两边会一起变小，断言照样绿。
// 消融实测确认过这一点（删 magnitude_sanity，`len(rep.Checks)==len(gates)` 存活）。
// 代价是 T5/T6 每加一道闸都要来改这里一次，那正是想要的：闸门集合的变更应当是
// 一个需要动手的决定，而不是某处 append 的副作用。
func TestReportAlwaysContainsEveryGate(t *testing.T) {
	// 与 M0 契约样本一致的闸门 ID。T5 已加 deposit_sum，T6 已加 stock_continuity，
	// 至此**七道齐全**——M0 契约样本确认 check ID 只有这七个，T7 断言恰好七道。
	wantGateIDs := []string{
		"monetary_hierarchy",
		"deposit_sum",
		"corp_loan_reconcile",
		"stock_continuity",
		"yoy_sanity",
		"completeness",
		"magnitude_sanity",
	}
	require.Equal(t, wantGateIDs, gateIDs(),
		"gates 表本身必须恰好是这七道——少一道时下面的逐行比对会跟着缩水而发现不了")

	broken := obsFrom(golden2025, extractorV2)
	broken.Values[FieldM1] = broken.Values[FieldM2] + 1

	zeroDenom := obsFrom(golden2025, extractorV2)
	zeroDenom.Values[FieldLoanCorpTotalYTD] = 0

	unknownExt := obsFrom(golden2025, "llm-fallback@v1")

	tests := []struct {
		name string
		obs  Observation
	}{
		{"golden2025", obsFrom(golden2025, extractorV2)},
		{"golden2020", obsFrom(golden2020, extractorV1)},
		{"空 Values", Observation{Meta: validMeta(), Values: nil}},
		{"闸门失败", broken},
		{"零分母", zeroDenom},
		{"未知抽取器", unknownExt},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rep, err := Validate(context.Background(), tt.obs, NoHistory, DefaultThresholds())
			require.NoError(t, err)

			var gotIDs []string
			for _, c := range rep.Checks {
				gotIDs = append(gotIDs, c.ID)
			}
			assert.Equal(t, wantGateIDs, gotIDs,
				"报告必须逐行对应全部闸门且顺序一致，一道都不能少")
		})
	}
}

// gateIDs 取 gates 表的 ID 序列，供上面那条断言把表本身钉住。
func gateIDs() []string {
	out := make([]string, 0, len(gates))
	for _, g := range gates {
		out = append(out, g.id)
	}
	return out
}

// 零分母记 skipped 而不是算出 Inf/NaN——Save 拒绝非有限的 Check.Value，
// 那会让整期既进不了观测表也进不了 pending。
func TestCorpLoanSkipsOnZeroDenominator(t *testing.T) {
	obs := obsFrom(golden2025, extractorV2)
	obs.Values[FieldLoanCorpTotalYTD] = 0

	rep, err := Validate(context.Background(), obs, NoHistory, DefaultThresholds())
	require.NoError(t, err)

	c := findCheck(t, rep, "corp_loan_reconcile")
	assert.Equal(t, CheckSkipped, c.Status)
	assert.Contains(t, c.Reason, "zero_denominator")
	if c.Value != nil {
		assert.False(t, math.IsInf(*c.Value, 0) || math.IsNaN(*c.Value),
			"Value 不得是 Inf/NaN——Save 会拒绝，整期数据会消失")
	}
}

// depositWith 造残差可控的存款观测：四部门只有一项非零，总额固定 100，
// 残差 = |household − 100| / 100，所以 residualPct 直接就是百分点。
//
// 包级 helper 而非各测试内的闭包：deposit_sum 的四条测试都要用它。
func depositWith(residualPct float64) map[string]float64 {
	v := maps.Clone(golden2025)
	v[FieldDepositFlowYTD] = 100
	v[FieldDepositHouseholdYTD] = 100 - residualPct
	v[FieldDepositCorpYTD] = 0
	v[FieldDepositFiscalYTD] = 0
	v[FieldDepositNBFIYTD] = 0
	return v
}

// deposit_sum 的两个判据合成一个三态，逐行验证映射表（spec 7.1 + 历史不足一行）。
func TestDepositSumCombinesTwoCriteria(t *testing.T) {
	// ⚠️ **历史期次要真的往前排**（M1c-4 的 TASK-008 加了相邻性约束）：
	// 原来所有 prior 都用 validMeta() 的 2026-06，与本期同期 ⇒ 跨度 0 ≠ 12，
	// 新约束会把每一格都判成 non_adjacent_prior。那**不是**新约束错了 ——
	// 夹具本来就不该让「上一期」与「本期」同期，只是此前没有任何断言看得见。
	// periodType 是 h1 ⇒ expectedPeriodGapMonths 返回 12，故逐期回退一年。
	priorWith := func(pcts ...float64) []Observation {
		out := make([]Observation, 0, len(pcts))
		for i, p := range pcts {
			out = append(out, Observation{Meta: priorMeta(i), Values: depositWith(p)})
		}
		return out
	}

	tests := []struct {
		name       string
		residual   float64   // 本期残差百分点
		prior      []float64 // 历史各期残差百分点
		wantStatus CheckStatus
		wantReason string // 空表示 Reason 必须为空
		wantValue  float64
	}{
		{"无历史，绝对值通过", 10, nil, CheckPassed, "drift_skipped:no_prior_period", 0.10},
		{"无历史，绝对值超标", 20, nil, CheckFailed, "tolerance_exceeded", 0.20},
		// ⚠️ 理由串在 M1c-4 的 TASK-008 收窄了：insufficient_history →
		// insufficient_same_caliber_history，并带上实际 n 与族名。收窄的理由是
		// mom 族的样本不足是**结构性**的（与 ytd 期次在时间轴上交错），
		// 和「新库冷启动」混成一格会让人以为等几期就好。
		{"历史不足三期", 10, []float64{10, 10}, CheckPassed, "drift_skipped:insufficient_same_caliber_history", 0.10},
		{"三期历史，漂移在容许内", 10, []float64{10, 11, 9}, CheckPassed, "", 0.10},
		{"三期历史，漂移超标", 5, []float64{10, 10, 10}, CheckFailed, "drift_exceeded", 0.05},
		{"绝对值超标时不谈漂移", 20, []float64{10, 10, 10}, CheckFailed, "tolerance_exceeded", 0.20},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obs := Observation{Meta: validMeta(), Values: depositWith(tt.residual)}
			rep, err := Validate(context.Background(), obs,
				fakeHistory{prior: priorWith(tt.prior...)}, DefaultThresholds())
			require.NoError(t, err)

			c := findCheck(t, rep, "deposit_sum")
			assert.Equal(t, tt.wantStatus, c.Status)
			if tt.wantReason == "" {
				assert.Empty(t, c.Reason)
			} else {
				assert.Contains(t, c.Reason, tt.wantReason)
			}
			require.NotNil(t, c.Value, "Value 恒为绝对残差占比")
			assert.InDelta(t, tt.wantValue, *c.Value, 1e-9)
		})
	}
}

// 前两行的 Reason 必须**可分**：no_prior_period 说「这是首期」，
// insufficient_history 说「再等几期就好」——对运维含义不同，一个该等一个该查。
//
// 上面那张表用 assert.Contains 逐行比对，两个理由若合并成同一个字串，
// 「历史不足三期」那行会拿 no_prior_period 去 Contains 而失败；但反过来，
// 若实现把两者都写成 "drift_skipped:no_prior_period_or_insufficient" 之类的
// 超集字串，Contains 会**同时**放过两行。这条把它们钉成互斥。
func TestDepositSumDistinguishesNoHistoryFromShortHistory(t *testing.T) {
	reasonFor := func(prior []Observation) string {
		rep, err := Validate(context.Background(),
			Observation{Meta: validMeta(), Values: depositWith(10)},
			fakeHistory{prior: prior}, DefaultThresholds())
		require.NoError(t, err)
		return findCheck(t, rep, "deposit_sum").Reason
	}

	none := reasonFor(nil)
	short := reasonFor([]Observation{{Meta: validMeta(), Values: depositWith(10)}})

	assert.NotEqual(t, none, short, "首期与历史不足必须给出不同的理由，否则运维无法判断该等还是该查")
	assert.NotContains(t, short, "no_prior_period",
		"历史不足不是首期——理由串不得互相包含，否则 Contains 断言会同时放过两行")
}

// 残差**恰好等于** ±12.0% 容差边界时的判定方向（spec boundary[2]）。
//
// 上游计划遗漏了这一条：它的六行用的是 5%/10%/20%，没有一个落在 12% 上，
// 而独立 reviewer 的消融证实把实现的 `r > Tolerance` 改成 `>=` 时，
// 计划原有的测试**无一转红**。
//
// 选数：残差 12/100 与阈值字面量 0.12 实测是**同一个 float64**
// （都是 8646911284551352p-56），故「恰好等于」是真的相等，不是舍入巧合。
// 这里不必绕道 2 的幂——本构造的中间量（88、100、12）都是远小于 2^53 的整数，
// IEEE 除法正确舍入到最近双精度，与 0.12 的字面量解析结果一致。
func TestDepositSumBoundaryIsInclusive(t *testing.T) {
	cfg := DefaultThresholds()
	require.Equal(t, 0.12, cfg.DepositSumTolerance, "本测试挑的边界值锚在默认容差上")

	t.Run("残差恰好等于容差判 passed", func(t *testing.T) {
		rep, err := Validate(context.Background(),
			Observation{Meta: validMeta(), Values: depositWith(12)}, NoHistory, cfg)
		require.NoError(t, err)

		c := findCheck(t, rep, "deposit_sum")
		assert.Equal(t, CheckPassed, c.Status,
			"实现是 r > tolerance 判失败，恰好等于必须通过；这里变红说明比较符被改成了 >=")
		require.NotNil(t, c.Value)
		assert.Equal(t, cfg.DepositSumTolerance, *c.Value, "残差必须恰好落在阈值上，否则测的不是边界")
	})

	t.Run("残差略超容差判 failed", func(t *testing.T) {
		rep, err := Validate(context.Background(),
			Observation{Meta: validMeta(), Values: depositWith(13)}, NoHistory, cfg)
		require.NoError(t, err)

		c := findCheck(t, rep, "deposit_sum")
		assert.Equal(t, CheckFailed, c.Status)
		assert.Contains(t, c.Reason, "tolerance_exceeded")
	})

	// —— 漂移阈值的边界（**超出 DoD boundary[0] 的范围，本任务自行追加**）——
	//
	// DoD 只点名了 ±12% 这一个边界。但同一形状的洞在漂移判据上也存在：消融实测
	// 把 `drift > DriftMax` 改成 `>=` 时，上面那些用例**无一转红**（M2 存活）。
	// 两个阈值同属一道闸、同一次交付，只堵一个会留下另一个。
	//
	// 选数：3pct 的默认值（0.03）不是精确可表示的二进制小数，故这里用自定义
	// 阈值 1/32 = 0.03125。前三期残差 6.25% ⇒ 均值恰为 1/16，本期 9.375% ⇒ 残差
	// 恰为 3/32，drift 恰为 1/32 —— 四个数实测都是精确值，等号成立与否只取决于
	// 比较符本身，不掺浮点舍入。
	driftCfg := DefaultThresholds()
	driftCfg.DepositSumDriftMax = 0.03125

	priorAt := func(pct float64, n int) []Observation {
		out := make([]Observation, 0, n)
		for k := 0; k < n; k++ {
			out = append(out, Observation{Meta: priorMeta(k), Values: depositWith(pct)})
		}
		return out
	}

	t.Run("漂移恰好等于上限判 passed", func(t *testing.T) {
		rep, err := Validate(context.Background(),
			Observation{Meta: validMeta(), Values: depositWith(9.375)},
			fakeHistory{prior: priorAt(6.25, minDriftHistory)}, driftCfg)
		require.NoError(t, err)

		c := findCheck(t, rep, "deposit_sum")
		assert.Equal(t, CheckPassed, c.Status,
			"实现是 drift > max 判失败，恰好等于必须通过；这里变红说明比较符被改成了 >=")
		assert.Empty(t, c.Reason, "有三期有效历史，漂移是真算过的，不该带 drift_skipped")
	})

	// 对照组的残差必须**仍在绝对容差内**，否则判据一先命中，测到的是
	// tolerance_exceeded 而不是漂移——初稿用 12.5% 就撞了这个（0.125 > 0.12）。
	// 10.9375% ⇒ 残差 7/64，仍 ≤ 12%，而与均值 1/16 的偏离 3/64 已超 1/32。
	t.Run("漂移超出上限判 failed", func(t *testing.T) {
		rep, err := Validate(context.Background(),
			Observation{Meta: validMeta(), Values: depositWith(10.9375)},
			fakeHistory{prior: priorAt(6.25, minDriftHistory)}, driftCfg)
		require.NoError(t, err)

		c := findCheck(t, rep, "deposit_sum")
		assert.Equal(t, CheckFailed, c.Status)
		assert.Contains(t, c.Reason, "drift_exceeded",
			"必须是漂移判据命中，而不是绝对容差——对照值要留在 ±12%% 以内")
	})
}

// 历史里算不出残差的期次不计入均值，而不是当成 0。
//
// 早期报告的部门划分不同，缺字段是常态。把算不出来的期次当成残差 0 会把均值
// 拉向 0——三期有效 10% 加两期「0」，均值变成 6%，本期 10% 就被判成漂移 4pct，
// 一个正常期次凭空变成告警。
func TestDepositSumIgnoresUncomputablePriors(t *testing.T) {
	// ⚠️ 历史逐期回退（M1c-4 的 TASK-008 的相邻性约束）：本期用 validMeta 的 2026-06，
	// prior[0] 必须是 2025-06 才算相邻。本期本身仍用 validMeta()。
	withResidualAt := func(i int, pct float64) Observation {
		return Observation{Meta: priorMeta(i), Values: depositWith(pct)}
	}
	// 缺一个部门分项 ⇒ depositResidualOf 返回 false
	incompleteAt := func(i int) Observation {
		o := withResidualAt(i, 10)
		delete(o.Values, FieldDepositNBFIYTD)
		return o
	}

	prior := []Observation{
		withResidualAt(0, 10), withResidualAt(1, 10), withResidualAt(2, 10),
		incompleteAt(3), incompleteAt(4),
	}

	rep, err := Validate(context.Background(),
		Observation{Meta: validMeta(), Values: depositWith(10)},
		fakeHistory{prior: prior}, DefaultThresholds())
	require.NoError(t, err)

	c := findCheck(t, rep, "deposit_sum")
	assert.Equal(t, CheckPassed, c.Status,
		"三期有效历史的均值就是 10%%，本期 10%% 不该被判成漂移：%s", c.Reason)
	assert.Empty(t, c.Reason, "有足够有效历史时漂移是真算过的，不该带 drift_skipped")
}

// 存款总额为 0 时记 skipped 而不是算出 Inf/NaN（DoD error_handling[0]）。
// 与 TestCorpLoanSkipsOnZeroDenominator 同形——计划实现了这条分支却没测它。
//
// 一期存款增量恰好为零是可能的，而 Save 拒绝非有限的 Check.Value，
// 那会让整期既进不了观测表也进不了 pending。
func TestDepositSumSkipsOnZeroDenominator(t *testing.T) {
	values := depositWith(10)
	values[FieldDepositFlowYTD] = 0

	rep, err := Validate(context.Background(),
		Observation{Meta: validMeta(), Values: values}, NoHistory, DefaultThresholds())
	require.NoError(t, err)

	c := findCheck(t, rep, "deposit_sum")
	assert.Equal(t, CheckSkipped, c.Status)
	assert.Contains(t, c.Reason, "zero_denominator:"+FieldDepositFlowYTD)
	if c.Value != nil {
		assert.False(t, math.IsInf(*c.Value, 0) || math.IsNaN(*c.Value),
			"Value 不得是 Inf/NaN——Save 会拒绝，整期数据会消失")
	}
}

// stockObs 造带指定 tsf_stock 值的观测，供 stock_continuity 的三条测试共用。
func stockObs(v float64) Observation {
	vals := maps.Clone(golden2025)
	vals[FieldTSFStock] = v
	return Observation{Meta: validMeta(), Values: vals}
}

// stockObsOf 是 stockObs 的分档版（M1c-2 的 TASK-001）。
//
// stock_continuity 的上限现在按 period_type 查表，而 stockObs 用的 validMeta() 是
// **h1**（上限 0.15）。凡是判定结果取决于阈值的用例都必须显式写出序列类型，否则
// 边界值与阈值对不上，用例会静默变成平凡真。
func stockObsOf(periodType string, v float64) Observation {
	o := stockObs(v)
	o.Meta.PeriodType = periodType
	return o
}

// stockObsWithout 造缺 tsf_stock 的观测（模拟没有社融板块的 v1 期次）。
func stockObsWithout() Observation {
	o := stockObs(0)
	delete(o.Values, FieldTSFStock)
	return o
}

// asAdjacentPrior 把一个观测改造成「与 validMeta() 那一期（2026-06）**相邻的上一期**」
// （M1c-3b 的 TASK-011 返工，缺陷 C-1）。
//
// 为什么需要它：gateStockContinuity 现在会检查 prior[0] 与本期是否相邻 —— 而本文件
// 原先的夹具让 prior 与本期用**同一个 period**（都来自 validMeta()），那在现实中不可能
// （Preceding 的 SQL 是 `WHERE period < ?`）。夹具不改的话，下面每一格都会变成
// non_adjacent_prior 而 skipped，那些断言就全测不到自己本来要测的东西了。
//
// ⚠️ 期次**写死**，不调用生产代码的 expectedPeriodGapMonths：用被测函数去造夹具，
// 那函数错了夹具跟着错，测试照样绿。这里独立陈述一次「相邻是什么意思」。
func asAdjacentPrior(o Observation) Observation {
	if o.Meta.PeriodType == "monthly" {
		o.Meta.Period = "2026-05" // validMeta() 的 period 是 2026-06
	} else {
		o.Meta.Period = "2025-06" // 其余四档相邻两期相隔 12 个月
	}
	return o
}

// 三种跳过理由必须报对，按「从根本到表面」排优先级。
// v1 期次同时满足前两条，报错了会把排查引向错误方向。
//
// # 这里**不**断言 rep.Passed，是刻意的
//
// 上游计划在每个子测试末尾写了 assert.True(t, rep.Passed, "跳过不阻断")，
// 它想说的是「stock_continuity 跳过不阻断」，写成的却是「整份报告没有任何闸门
// 失败」——观测对象比被测对象大。缺 tsf_stock 的构造只有 53 个键，而
// validMeta() 的 Extractor 是 rule@v2 ⇒ completeness 拿 54 个必填字段一比即
// failed，于是 rep.Passed 恒为 false，那条断言 2/4 必红（reviewer 实跑证实）。
//
// 「跳过不阻断」这条性质本身**已经有守卫**：TestValidateOnGoldenSamples 在两份
// 真实样本上断言 rep.Passed，而那两份样本里都含 skipped 的闸门——聚合逻辑若把
// skipped 当成失败，那条测试会红（本任务消融 M7 实测确认）。所以这里收窄到
// 本闸自己，不重复断言别的闸门的行为。
func TestStockContinuitySkipReasons(t *testing.T) {
	tests := []struct {
		name   string
		obs    Observation
		prior  []Observation
		reason string
	}{
		{
			"本期没有社融板块，但有历史",
			stockObsWithout(), []Observation{asAdjacentPrior(stockObs(400))},
			"absent_field:" + FieldTSFStock,
		},
		{
			"v1 期次且无历史：两个理由同时成立，报最根本的那个",
			stockObsWithout(), nil,
			"absent_field:" + FieldTSFStock,
		},
		{
			"本期有社融，但库里没有历史",
			stockObs(400), nil,
			"no_prior_period",
		},
		{
			"上一期没有社融板块",
			stockObs(400), []Observation{asAdjacentPrior(stockObsWithout())},
			"prior_absent_field:" + FieldTSFStock,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rep, err := Validate(context.Background(), tt.obs,
				fakeHistory{prior: tt.prior}, DefaultThresholds())
			require.NoError(t, err)

			c := findCheck(t, rep, "stock_continuity")
			assert.Equal(t, CheckSkipped, c.Status)
			assert.Equal(t, tt.reason, c.Reason)
		})
	}
}

// 有历史时这道闸真正生效，含边界值的判定方向。
//
// # ⚠️ 每一格都必须写死 periodType —— 这里踩过一次
//
// M1c-2 把上限改成按 period_type 查表之前，本表用的是 stockObs()，即 validMeta()
// 的 **h1**。分档后 h1 的上限是 0.15，于是四格里两格转红（0.05 的跳变在 h1 下本就
// 该放行），而「恰好在阈值上」那格**静默退化为平凡真**：0.02 <= 0.15 无论边界写成
// `<` 还是 `<=` 都成立，连同它下面那段关于 ULP 的精细论证一起失效。
// **会有人去修红的那两格，绿的那格不会引起任何注意。**
//
// ⇒ 下面 monthly 四格是边界论证，annual 三格钉住「分档真的生效」：
// 第五格与第三格是同一个 6% 跳变，判定相反。
//
// ⚠️ **M1c-3b 的 TASK-005 重标阈值后，本表的常量整体上移过一次**
// （monthly 0.02→0.05、annual 0.15→0.20）。上移的不是「几个数字」而是**边界本身**：
// 两个 atBoundary 格若不跟着改，会静默退化成普通的「阈值内」用例——0.02 <= 0.05
// 无论边界写成 `<` 还是 `<=` 都成立，正是本注释开头记的那次事故的形状。
// 挡住它的是循环体末尾那条 require（它动态取 DefaultThresholds），不是人的记性。
func TestStockContinuityDetectsJump(t *testing.T) {
	tests := []struct {
		name       string
		periodType string
		prev, cur  float64
		wantStatus CheckStatus
		wantRatio  float64
		// atBoundary 标记「这格声称恰好落在阈值上」。见循环体里那条 require ——
		// 它是本表**唯一**能挡住「静默退化」的东西：wantStatus/wantRatio 都挡不住，
		// 把 periodType 从 monthly 改成 h1 后 0.02 <= 0.15 照样 Passed、比例照样
		// 0.02，四条断言全绿，而这格已经不再测边界了。
		atBoundary bool
	}{
		// 8/400 的浮点商与字面量 0.02 **舍入到同一个 double**，所以 r <= max 成立。
		//
		// ⚠️ 注意措辞：0.02 **本身并不精确可表示**（位模式 0x3f947ae147ae147b，
		// 实为 0.020000000000000000416）。这里成立的条件是两边落到同一个 double，
		// 而不是「比例是精确值」。
		//
		// 让它成立的是**参与运算的量精确**（cur−prev 与 prev 都是精确整数），
		// **不是算路短**。M1c-3b 的 TASK-005 在新阈值下重测了同一组性质：
		//   0.05: 400→420 / 300→315 / 200→210 / 500→525  ⇒ r == 0.05 为 true
		//         123→129.15 / 400.1→420.105             ⇒ false
		//   0.20: 400→480 / 300→360 / 200→240 / 500→600  ⇒ r == 0.20 为 true
		//         123→147.6 / 400.1→480.12               ⇒ false
		// ⇒ **改这几行常量时必须保持 cur−prev 与 prev 为精确整数**；换成「看起来
		// 更真实」的小数会让这条边界测试**静默失效**，而不会有任何东西转红。
		{"monthly 恰好在阈值上", "monthly", 400, 420, CheckPassed, 0.05, true},
		{"monthly 阈值内", "monthly", 400, 404, CheckPassed, 0.01, false},
		{"monthly 超过阈值", "monthly", 400, 424, CheckFailed, 0.06, false},
		{"monthly 存量下跌也算跳变", "monthly", 400, 376, CheckFailed, 0.06, false},

		// 同一个 6% 跳变在年度序列上必须**放行** —— annual 的相邻两期相隔 12 个月。
		// 这一格与上面第三格是对照：把五档写成同一个数，两格里必有一格红。
		{"annual 同样的 6% 跳变放行", "annual", 400, 424, CheckPassed, 0.06, false},
		// 80/400 与 84/400 同为精确整数相除，与上面 20/400 同理落在与字面量同一个
		// double 上 —— 改这两行常量时同样必须保持 cur−prev 与 prev 为精确整数。
		{"annual 恰好在阈值上", "annual", 400, 480, CheckPassed, 0.20, true},
		{"annual 超过阈值", "annual", 400, 484, CheckFailed, 0.21, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rep, err := Validate(context.Background(), stockObsOf(tt.periodType, tt.cur),
				fakeHistory{prior: []Observation{asAdjacentPrior(stockObsOf(tt.periodType, tt.prev))}},
				DefaultThresholds())
			require.NoError(t, err)

			c := findCheck(t, rep, "stock_continuity")
			assert.Equal(t, tt.wantStatus, c.Status, "reason=%s", c.Reason)
			require.NotNil(t, c.Value)
			assert.InDelta(t, tt.wantRatio, *c.Value, 1e-9)

			// **精确**相等，不是 InDelta：这格的全部意义就是「r == max 时 r <= max
			// 成立」，容差会把它测没。上面那段 ULP 注释论证的也正是这个等号。
			if tt.atBoundary {
				require.Equal(t, DefaultThresholds().StockContinuityMax[tt.periodType], *c.Value,
					"这格声称恰好落在阈值上，就必须真的落在上面——改 periodType 或改"+
						"prev/cur 常量都会让它静默退化成一个普通的「阈值内」用例")
			}
		})
	}
}

// 上一期存量为 0 时记 skipped 而不是算出 Inf（DoD error_handling[0]）。
// 计划实现了这条分支却没有为它写测试，与 deposit_sum 那条同形。
func TestStockContinuitySkipsOnZeroDenominator(t *testing.T) {
	rep, err := Validate(context.Background(), stockObs(400),
		fakeHistory{prior: []Observation{asAdjacentPrior(stockObs(0))}}, DefaultThresholds())
	require.NoError(t, err)

	c := findCheck(t, rep, "stock_continuity")
	assert.Equal(t, CheckSkipped, c.Status)
	assert.Equal(t, "zero_denominator:"+FieldTSFStock, c.Reason,
		"上一期为 0 必须走零分母分支，而不是 prior_absent_field——字段在，只是值为 0")
	if c.Value != nil {
		assert.False(t, math.IsInf(*c.Value, 0) || math.IsNaN(*c.Value),
			"Value 不得是 Inf/NaN——Save 会拒绝，整期数据会消失")
	}
}

// 运行时缺档记 skipped{no_threshold}，**不是 failed**（M1c-2 的 TASK-001）。
//
// 「装载时必须齐全」与「运行时容忍缺档」看似矛盾，实为两种不同的处境，故落在
// **两处不同的实现**上，各自单独可消融：
//
//   - 装载时缺档 = 配置疏漏 ⇒ LoadConfig 响亮失败
//     （TestLoadConfigRejects 的「显式写了 map 但漏一档」那格）
//   - 运行时缺档 = **代码里有了新 period_type、配置还没跟上** ⇒ 本条
//
// period_type 已经从 3 种扩到 5 种一次（Sprint 037 补 q1 / q1_q3）。下次再扩时若判
// failed，那一批期次会**全部进 pending**，而真实情况只是「还没给这种序列定上限」。
func TestStockContinuitySkipsWhenPeriodTypeHasNoThreshold(t *testing.T) {
	cfg := DefaultThresholds()
	delete(cfg.StockContinuityMax, "monthly")

	t.Run("数据与历史都在，只是没给这条序列定上限", func(t *testing.T) {
		rep, err := Validate(context.Background(), stockObsOf("monthly", 420),
			fakeHistory{prior: []Observation{asAdjacentPrior(stockObsOf("monthly", 400))}}, cfg)
		require.NoError(t, err)

		c := findCheck(t, rep, "stock_continuity")
		assert.Equal(t, CheckSkipped, c.Status,
			"缺档必须 skipped：判 failed 会让那条序列的每一期都进 pending，"+
				"而数据本身没有任何问题")
		assert.Equal(t, "no_threshold:monthly", c.Reason)
	})

	// 缺档与缺字段同时成立时报哪个：报 no_threshold。
	//
	// 沿用本闸原有的「从根本到表面」原则——缺档是**闸门能否判定的前提**，且与本期
	// 数据无关：整条序列的每一期都会命中，改一处配置就全好。报 absent_field 会把
	// 排查引向逐期查数据，而那些期次的数据可能一点问题都没有。
	t.Run("同时还缺 tsf_stock：报最根本的那个", func(t *testing.T) {
		obs := stockObsOf("monthly", 0)
		delete(obs.Values, FieldTSFStock)

		rep, err := Validate(context.Background(), obs,
			fakeHistory{prior: []Observation{asAdjacentPrior(stockObsOf("monthly", 400))}}, cfg)
		require.NoError(t, err)

		c := findCheck(t, rep, "stock_continuity")
		assert.Equal(t, CheckSkipped, c.Status)
		assert.Equal(t, "no_threshold:monthly", c.Reason,
			"缺档优先于 absent_field")
	})
}

// —— TASK-007: 豁免应用、Save 接线、ULP 契约 ——

// gates 恰好七道，ID 与 M0 契约样本一致。
//
// 这不是同义反复：M0 的两份契约样本已按这七个 ID 写好，Grafana 面板与
// pending 的人工复核流程都依赖它们。加一道闸门就要同步改契约文档——
// 这条测试是那个提醒。
func TestGatesMatchContractedCheckIDs(t *testing.T) {
	want := []string{
		"monetary_hierarchy", "deposit_sum", "corp_loan_reconcile",
		"stock_continuity", "yoy_sanity", "completeness", "magnitude_sanity",
	}
	assert.Equal(t, want, knownCheckIDs(), "闸门清单与 M0 契约样本不一致")
}

// 命中豁免记 skipped 而不是 passed——豁免与通过在数据上必须可分。
func TestCaliberExemptionRecordsSkipNotPass(t *testing.T) {
	cfg := DefaultThresholds()
	cfg.CaliberExemptions = []CaliberExemption{{
		Version: "2025-01",
		Period:  validMeta().Period,
		// 与 Period 一样取自 validMeta()，而不是写死 "h1"：两个维度必须都命中
		// 豁免才生效，写死会在 validMeta 改动时静默变成「豁免不命中」，
		// 而那时这条测试红的理由与它要钉的 skipped-not-passed 无关。
		PeriodTypes: []string{validMeta().PeriodType},
		SkipChecks:  []string{"monetary_hierarchy"},
		Reason:      "M1 口径纳入个人活期存款，层次关系在切换期不成立",
	}}

	obs := obsFrom(golden2025, extractorV2)
	obs.Values[FieldM1] = obs.Values[FieldM2] + 1 // 本该失败

	rep, err := Validate(context.Background(), obs, NoHistory, cfg)
	require.NoError(t, err)

	c := findCheck(t, rep, "monetary_hierarchy")
	assert.Equal(t, CheckSkipped, c.Status, "豁免不该记成 passed")
	assert.Equal(t, "caliber_exemption:2025-01", c.Reason)
	assert.True(t, rep.Passed, "被豁免的闸门不阻断入库")
}

// 豁免只对指定期次生效，不外溢。写成范围或前缀匹配会让一次性豁免变成永久后门。
func TestCaliberExemptionDoesNotLeakToOtherPeriods(t *testing.T) {
	cfg := DefaultThresholds()
	cfg.CaliberExemptions = []CaliberExemption{{
		Version: "2025-01", Period: "2025-01",
		// period_type 取观测的那个，让 Period 成为**唯一**不匹配的维度：
		// 若这里写成别的取值，下面的 failed 断言会同时有两个成因，
		// 而它要钉的只是「豁免不外溢到别的期次」。
		PeriodTypes: []string{validMeta().PeriodType},
		SkipChecks:  []string{"monetary_hierarchy"},
		Reason:      "口径切换期",
	}}

	obs := obsFrom(golden2025, extractorV2) // Period 是 validMeta() 的 2026-06
	obs.Values[FieldM1] = obs.Values[FieldM2] + 1

	rep, err := Validate(context.Background(), obs, NoHistory, cfg)
	require.NoError(t, err)

	assert.Equal(t, CheckFailed, findCheck(t, rep, "monetary_hierarchy").Status,
		"别的期次的豁免不该在这期生效")
	assert.False(t, rep.Passed)
}

// SkipChecks 里的 ID 必须真实存在。
//
// 打错一个字（deposit_summ）在没有校验时会静默失效——豁免看起来配上了，
// 实际那道闸门照跑，而配置的人以为已经跳过。
func TestExemptionRejectsUnknownCheckID(t *testing.T) {
	cfg := DefaultThresholds()
	cfg.CaliberExemptions = []CaliberExemption{{
		Version: "2025-01", Period: "2025-01",
		PeriodTypes: []string{"monthly"},
		SkipChecks:  []string{"deposit_summ"}, Reason: "拼错的 ID",
	}}

	_, err := Validate(context.Background(),
		obsFrom(golden2025, extractorV2), NoHistory, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "deposit_summ")
}

// 增量类字段由 万亿×10000 得出，个别值存在 ≤1 ULP 的表示误差。
//
// 这条测试钉住的是**误差存在这个事实本身**。闸门一律不得对这些值做精确相等
// 比较——现有七道全是不等式或容差比较，所以它现在不咬人，但下一个加闸门的人
// 不会知道，除非有东西写着。
//
// ⚠️ 验证时不能在源码里手算：Go 的无类型常量算术是精确的，写 4.81*10000
// 会得到精确的 48100，从而得出「没有误差」的相反结论。必须用运行时值。
func TestTrillionConversionCarriesULPError(t *testing.T) {
	trillion := 4.81 // 变量，不是编译期常量表达式
	got := trillion * 10000

	assert.NotEqual(t, 48100.0, got,
		"若这个等式成立说明换算方式变了——去掉闸门里『不得精确相等比较』的契约")
	assert.InDelta(t, 48100.0, got, 1e-9,
		"误差必须小到闸门的容差完全盖住；不成立说明换算出了真问题")
}

// Validate 的产出必须能被 Save 接受。
//
// 两边各自的测试都绿，接起来仍可能不通：Save 对报告另有要求（Checks 非空、
// skipped 必带 reason、Value 必须有限），而 Validate 是唯一的报告生产者。
// 这条测试同时验证 Store 能当 History 用。
func TestValidateOutputIsAcceptedBySave(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name       string
		values     map[string]float64
		extractor  string
		period     string
		periodType string
	}{
		{"v2 全字段", golden2025, extractorV2, "2025-12", "annual"},
		{"v1 无社融", golden2020, extractorV1, "2020-06", "h1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestStore(t)
			obs := Observation{
				Meta: Meta{
					Period: tt.period, PeriodType: tt.periodType,
					PublishedAt: tt.period + "-15", ArticleID: "art-" + tt.period,
					CaliberVersion: "2025-01", Extractor: tt.extractor,
				},
				Values: maps.Clone(tt.values),
			}

			rep, err := Validate(ctx, obs, s, DefaultThresholds())
			require.NoError(t, err)

			out, err := s.Save(ctx, obs, rep)
			require.NoError(t, err, "Validate 的产出必须能被 Save 接受")
			assert.Equal(t, TableObservations, out.Table)
		})
	}
}

// 没过闸的数据进 pending，报告本身也要能被序列化。
func TestFailedValidationLandsInPending(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	obs := Observation{
		Meta: Meta{
			Period: "2025-12", PeriodType: "annual",
			PublishedAt: "2026-01-15", ArticleID: "art-bad",
			CaliberVersion: "2025-01", Extractor: extractorV2,
		},
		Values: maps.Clone(golden2025),
	}
	obs.Values[FieldM1] = obs.Values[FieldM2] + 1 // 层次倒置

	rep, err := Validate(ctx, obs, s, DefaultThresholds())
	require.NoError(t, err)
	require.False(t, rep.Passed)

	out, err := s.Save(ctx, obs, rep)
	require.NoError(t, err)
	assert.Equal(t, TablePending, out.Table)
}

// —— 以下两条是 Leader 追加的要求（DoD non_functional[2]），不在上游计划里 ——

// monetary_hierarchy 的两处比较都必须是**严格**大于。
//
// 验证者实测：把 `m2 > m1 && m1 > m0` 改成 `>=`，两处**均无测试转红**。
// 即「M2 恰好等于 M1」会被判 passed —— 而 M2 严格含 M1（M2 = M1 + 准货币），
// 相等意味着准货币为零，那是可疑数据而不是正常数据。
//
// 这与 magnitude 的边界缺口同形，但语义上更实在：magnitude 恒 skipped 至 M1c，
// 而 monetary_hierarchy **每期都在跑**，是七道闸里当下就有信号的三道之一。
//
// 选数：直接令两数相等，不涉及任何除法或换算，等号成立与否只取决于比较符。
func TestMonetaryHierarchyRejectsEquality(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]float64)
	}{
		{"M2 恰好等于 M1", func(v map[string]float64) { v[FieldM2] = v[FieldM1] }},
		{"M1 恰好等于 M0", func(v map[string]float64) { v[FieldM1] = v[FieldM0] }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obs := obsFrom(golden2025, extractorV2)
			tt.mutate(obs.Values)

			rep, err := Validate(context.Background(), obs, NoHistory, DefaultThresholds())
			require.NoError(t, err)

			c := findCheck(t, rep, "monetary_hierarchy")
			assert.Equal(t, CheckFailed, c.Status,
				"层次必须严格递减；这里变红说明比较符被放宽成了 >=")
			assert.False(t, rep.Passed)
		})
	}
}

// 豁免生效时报告仍须逐行齐全。
//
// 与 TestReportAlwaysContainsEveryGate 互补而非重复：那条跑的是**无豁免**配置，
// 覆盖不到本任务新加的豁免分支。豁免分支整个替换了 Check 值，若写成 `continue`
// 就会让被豁免的闸门**从报告里消失** —— 而 rep.Passed 照样是 true，
// 看起来正是「豁免生效了」的样子。删掉任一条都会重新开一个缺口。
func TestReportKeepsEveryGateUnderExemption(t *testing.T) {
	cfg := DefaultThresholds()
	cfg.CaliberExemptions = []CaliberExemption{{
		Version:     "2025-01",
		Period:      validMeta().Period,
		PeriodTypes: []string{validMeta().PeriodType},
		SkipChecks:  []string{"monetary_hierarchy", "yoy_sanity"},
		Reason:      "口径切换期，层次与同比均不可比",
	}}

	obs := obsFrom(golden2025, extractorV2)
	obs.Values[FieldM1] = obs.Values[FieldM2] + 1 // 本该 failed 的输入

	rep, err := Validate(context.Background(), obs, NoHistory, cfg)
	require.NoError(t, err)

	var gotIDs []string
	for _, c := range rep.Checks {
		gotIDs = append(gotIDs, c.ID)
	}
	assert.Equal(t, knownCheckIDs(), gotIDs,
		"豁免只改判定，不得让闸门从报告里消失")

	// 被豁免的两道都记 skipped 且带 reason（Save 会拒绝无 reason 的 skip）
	for _, id := range []string{"monetary_hierarchy", "yoy_sanity"} {
		c := findCheck(t, rep, id)
		assert.Equal(t, CheckSkipped, c.Status, "%s 应记 skipped", id)
		assert.Equal(t, "caliber_exemption:2025-01", c.Reason)
	}
	// yoy_sanity 本来会算出 Value，豁免后必须保留——残差仍是有用的观测
	assert.NotNil(t, findCheck(t, rep, "yoy_sanity").Value,
		"豁免保留 Value：闸门算出的观测仍有用，只是不据此判定")
}

// ── M1c-3b 的 TASK-011：merged@v1 的 completeness 接线（阻断级缺口 A-1）──────
//
// Context Checkpoint: done_criteria → test mapping (M1c-3b 的 TASK-011)
// functional[2]  merged 观测缺字段 ⇒ completeness 必须 CheckFailed 而非 CheckSkipped
//                                              → TestMergedCompletenessIsEvaluated
// functional[3]  Parts 不入库，Save→Preceding 往返后恒为 nil
//                                              → TestMergedPartsDoNotRoundTrip
// boundary[0]    Parts 为空/全无效 ⇒ CheckSkipped，**绝不能是 CheckPassed**
//                                              → TestMergedCompletenessSkipsWhenPartsYieldNoFields

// mergedObs 造一个 merged@v1 观测：Parts 指定由哪几篇合成，Values 覆盖 fields 里的每一个。
//
// 值本身无意义（completeness 只看键在不在），但必须是有限数——checkValues 会拒 NaN/Inf。
func mergedObs(parts, fields []string) Observation {
	m := validMeta()
	m.Extractor = extractorMerged
	vals := make(map[string]float64, len(fields))
	for i, f := range fields {
		vals[f] = float64(i + 1)
	}
	return Observation{Meta: m, Values: vals, Parts: parts}
}

// TestMergedCompletenessIsEvaluated 是本任务的**真正验收点**。
//
// 缺口 A-1：gateCompleteness 拿的是 requiredFields(Meta.Extractor)，而 merged@v1 落
// default 返回 nil ⇒ 整道闸 skipped{unknown_extractor:merged@v1}；而 validate.go 的
// passed 只在 CheckFailed 时翻转 ⇒ 42 个合并观测的 completeness **谁都不查**，
// 带着「零告警」进权威表。
//
// ⚠️ 四道恒等式抓不住这个缺陷——它们全由同一批计数器派生，内部自洽 ≠ 闸门真的执行了。
// 只有本条断言能：它要求那道闸对 merged@v1 **真的算出了缺失字段**。
func TestMergedCompletenessIsEvaluated(t *testing.T) {
	// 社融两篇合成：必填集 = stock ∪ flow，故意少给一个字段。
	full := append(tsfStockFields(), tsfFlowFields()...)
	require.Greater(t, len(full), 1, "样本太小，删一个字段后断言不成立")
	dropped := full[0]
	obs := mergedObs([]string{extractorTSFStock, extractorTSFFlow}, full[1:])

	rep, err := Validate(context.Background(), obs, NoHistory, DefaultThresholds())
	require.NoError(t, err, "闸门失败不该变成 Go error")

	c := findCheck(t, rep, "completeness")
	assert.Equal(t, CheckFailed, c.Status,
		"merged@v1 的 completeness 必须真的被求值。若这里是 skipped，说明 gateCompleteness "+
			"仍在拿 requiredFields(Meta.Extractor)——那正是缺口 A-1：42 个合并观测的 "+
			"completeness 谁都不查，还带着零告警进权威表")
	assert.NotContains(t, c.Reason, "unknown_extractor",
		"reason 仍是 unknown_extractor ⇒ 分支根本没走到 mergedRequiredFields")
	assert.Contains(t, c.Reason, "missing 1",
		"少给一个字段就该报缺 1 个；数目不对说明必填集取错了组")
	assert.Contains(t, c.Reason, dropped, "报出的缺失字段必须就是被删掉的那个")

	// 反向一格：字段齐全时必须 passed —— 只有失败用例的话，一个「恒 failed」的
	// 实现也能让上面全绿。
	whole := mergedObs([]string{extractorTSFStock, extractorTSFFlow}, full)
	repOK, err := Validate(context.Background(), whole, NoHistory, DefaultThresholds())
	require.NoError(t, err)
	assert.Equal(t, CheckPassed, findCheck(t, repOK, "completeness").Status,
		"stock ∪ flow 全给齐时 completeness 该 passed")
}

// TestMergedCompletenessSkipsWhenPartsYieldNoFields 钉住本任务**最容易引入的新缺陷**。
//
// mergedRequiredFields 的 out := make([]string, 0, len(want)) **永远返回非 nil 切片**
// （M1c-3b 的 TASK-002 在 interfaces_exposed 里写明了这条边界）⇒ 判据若写成
// `req == nil`，对 merged@v1 **恒不命中**，会一路落到 len(missing)==0 返回 CheckPassed：
// 一个 Parts 为空的合并观测被判「completeness 通过」，而它一个字段都没查。
//
// 🔴 这比缺口 A-1 原本的 skipped **更糟**：skipped 在报告里可见、可被 M1c-3b 的
// TASK-010 的判据数出来；passed 是**完全静默**的。修 A-1 时引入一个同类的静默失效，
// 是本任务唯一不可接受的结果。故判据必须是 len(req) == 0。
func TestMergedCompletenessSkipsWhenPartsYieldNoFields(t *testing.T) {
	for _, tt := range []struct {
		name  string
		parts []string
	}{
		{"Parts 为 nil", nil},
		{"Parts 为空切片", []string{}},
		{"Parts 全是拿不到必填集的值", []string{"bogus@v9", "llm-fallback@v1"}},
		{"Parts 只含 merged 自身（自指，同样说不出必填集）", []string{extractorMerged}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// 给足 Values：若判据写错成 req == nil，这里会 passed 而不是 skipped，
			// 而 Values 非空正是让那个错误显形的条件。
			obs := mergedObs(tt.parts, tsfStockFields())

			rep, err := Validate(context.Background(), obs, NoHistory, DefaultThresholds())
			require.NoError(t, err)

			c := findCheck(t, rep, "completeness")
			assert.NotEqual(t, CheckPassed, c.Status,
				"必填集为空时判 passed = 一个字段都没查却说「通过」，这是静默失效，"+
					"比 skipped 更糟；成因通常是判据写成了 req == nil（mergedRequiredFields "+
					"永远返回非 nil 切片，那个判据恒不命中）")
			assert.Equal(t, CheckSkipped, c.Status)
			assert.Contains(t, c.Reason, "unknown_extractor:"+extractorMerged,
				"skipped 必须说明原因，否则报告里看不出这一格为什么没查")
		})
	}
}

// TestMergedPartsDoNotRoundTrip 把「Parts 不入库」从**注释**变成**断言**。
//
// 一个不持久化的导出字段最容易被后人当成能往返的：读回来恒为 nil 而代码看不出来，
// 于是「历史观测的 Parts」会被静默当成「没有 parts」而不是「这个问题问不了」。
//
// ⚠️ 放在 validate_test.go 而不是 store_test.go：后者在 M1c-3b 的 TASK-003/006 的
// writes 里，写那里会造成 scope 互斥。
func TestMergedPartsDoNotRoundTrip(t *testing.T) {
	s := newTestStore(t)

	parts := []string{extractorTSFStock, extractorTSFFlow}
	obs := mergedObs(parts, tsfStockFields())
	obs.Meta.Period = "2025-12"
	obs.Meta.PeriodType = "monthly"
	obs.Meta.PublishedAt = "2025-12-15"
	obs.Meta.ArticleID = "art-2025-12"

	_, err := s.Save(context.Background(), obs, passing())
	require.NoError(t, err)

	got, err := s.Preceding(context.Background(), "2026-01", "monthly", 1)
	require.NoError(t, err)
	require.Len(t, got, 1, "刚存的那期该读得回来")

	assert.Nil(t, got[0].Parts,
		"Parts 是进程内的合并取证，metaColumns 是七列、insertSQL 显式列举，它不在其中 ⇒ "+
			"读回来必须是 nil。若这里非 nil，说明有人把它加进了持久化路径")
	assert.Equal(t, extractorMerged, got[0].Meta.Extractor,
		"extractor 该照常往返——用它证明上面那条 nil 不是因为整条记录没读到")

	// 调用方手里的那份不受影响：Save 改的是值参数的副本。
	assert.Equal(t, parts, obs.Parts, "Save 不得清空调用方的 Parts")
}

// —— gateMagnitudeSanity 消费端的两个守卫缺口（M1c-3b 的 TASK-012，F12 / F17#5#6）——
//
// 由 M1c-3b 的 TASK-005 的 dev 在填表前查实并上报。两处**实现都是对的**，缺的是守卫：
// 在 TASK-005 填完 54 项之前，magnitude_sanity 恒 skipped{not_calibrated}，所以这两个
// 缺口影响为零；**填完的那一刻这道闸开始真正判定，缺口同时变成真的**。
//
// ⚠️ 下面两条的 RED 判据与常规 TDD 不同：它们在**未做任何实现改动时就是 PASS 的**，
// 有效性只能靠变异证明（实测结果见各自注释与 discovery 的 mutation 段）。
// **实现本身一律不要改**——本节补的是守卫，不是修 bug。

// TestMagnitudeSanityReportsEarliestFieldInFieldOrder（F12）：多个字段同时越界时，
// 报出的必须是 fieldOrder 里**最靠前**的那个，且反复跑结果相同。
//
// 守的是 gateMagnitudeSanity 里的 `for _, f := range fieldOrder`（该文件有两处同形
// 循环，这里指的是 gateMagnitudeSanity 函数内的那个）。改成 `range in.cfg.MagnitudeRanges`
// 后，报出的字段随 map 迭代序随机 —— 同一份数据两次跑报出不同的越界字段，排查变成猜谜。
//
// ⚠️ 用**六个**越界字段而不是两个：单次比较在变异下仍有 1/N 概率蒙对，
// 六个字段 × 十轮把存活概率压到 (1/6)^10 ≈ 1.7e-8，变异必然被杀。
func TestMagnitudeSanityReportsEarliestFieldInFieldOrder(t *testing.T) {
	// 六个字段都在 golden2025 里，且横跨 fieldOrder 的前中后段。
	// tsf_stock 是 fieldOrder[0]，故它必须是被报出的那一个。
	outOfRange := []string{
		FieldTSFStock, FieldTSFStockRMBLoan, FieldTSFStockEntrust,
		FieldM2, FieldM0, FieldFXReserve,
	}
	require.Equal(t, FieldTSFStock, fieldOrder[0],
		"本用例依赖 tsf_stock 是 fieldOrder 的第一项；fieldOrder 改了这条要跟着改")

	cfg := DefaultThresholds()
	cfg.MagnitudeRanges = make(map[string]Range, len(outOfRange))
	for _, f := range outOfRange {
		// [0, 0.001] 对这六个字段的实测值（3.36 ~ 356000）全部越界
		cfg.MagnitudeRanges[f] = Range{Min: 0, Max: 0.001, Unit: "测试用"}
	}

	// 跑十轮：map 迭代序每次随机，稳定性只有反复跑才证得出。
	for i := range 10 {
		obs := obsFrom(golden2025, extractorV2)
		rep, err := Validate(context.Background(), obs, NoHistory, cfg)
		require.NoError(t, err)
		c := findCheck(t, rep, "magnitude_sanity")
		require.Equal(t, CheckFailed, c.Status)
		assert.Truef(t, strings.HasPrefix(c.Reason, FieldTSFStock+"="),
			"第 %d 轮报出的是 %q；越界字段必须按 fieldOrder 报最靠前的那个（tsf_stock），"+
				"否则同一份数据两次跑会指向不同字段", i, c.Reason)
	}
}

// TestMagnitudeSanityBoundariesAreInclusive（F17 #5/#6）：区间是**闭区间**——
// 恰好落在 min / max 上算通过，越过一档才算失败。
//
// 守的是 gateMagnitudeSanity 里的 `if v < r.Min || v > r.Max`（唯一一处）。
// 现有的 TestMagnitudeSanityActivatesWhenCalibrated 用 42 对区间 [3,4]，
// **离边界两个数量级** ⇒ 把 < 改成 <= 或 > 改成 >= 都不会有任何测试转红。
// 本条用恰好落在边界上的值把这两个比较符各自钉死。
func TestMagnitudeSanityBoundariesAreInclusive(t *testing.T) {
	const lo, hi = 3.0, 4.0
	tests := []struct {
		name string
		v    float64
		want CheckStatus
	}{
		{"恰好等于 min：闭区间，通过", lo, CheckPassed},
		{"恰好等于 max：闭区间，通过", hi, CheckPassed},
		{"区间正中：通过", (lo + hi) / 2, CheckPassed},
		{"刚低于 min 一档：失败", math.Nextafter(lo, math.Inf(-1)), CheckFailed},
		{"刚高于 max 一档：失败", math.Nextafter(hi, math.Inf(1)), CheckFailed},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultThresholds()
			cfg.MagnitudeRanges = map[string]Range{
				FieldFXReserve: {Min: lo, Max: hi, Unit: "万亿美元"},
			}
			obs := obsFrom(golden2025, extractorV2)
			obs.Values[FieldFXReserve] = tc.v

			rep, err := Validate(context.Background(), obs, NoHistory, cfg)
			require.NoError(t, err)
			assert.Equalf(t, tc.want, findCheck(t, rep, "magnitude_sanity").Status,
				"fx_reserve=%v 对区间 [%v,%v]", tc.v, lo, hi)
		})
	}
}

// ── M1c-3b 的 TASK-011 返工（C-1）：prior[0] 不等于「上一期」 ──────────────
//
// Context Checkpoint: fix_items → test mapping
// C-1 修法①（非相邻不得按原阈值判） → TestStockContinuitySkipsNonAdjacentPrior
// C-1 修法④（跨接缝：中间期被拒）   → TestStockContinuityDoesNotUseRejectedPeriodAsBaseline

// TestStockContinuitySkipsNonAdjacentPrior：prior[0] 与本期不相邻时必须 skipped。
//
// 🔴 `Preceding` 的 SQL 是 `WHERE period < ? AND period_type = ? ORDER BY period DESC`
// —— 它返回**最近 N 个已被接受的期次**，**没有相邻性约束**。而本闸原先的注释断言
// 「prior[0] 就是上一期」，那句话**只在从未拒过任何一期时成立**，而这道闸的存在前提
// 就是会拒。真跑撞出 4 条拒绝，基线跨 10/11/13 个月、跨 3 年。
func TestStockContinuitySkipsNonAdjacentPrior(t *testing.T) {
	for _, tt := range []struct {
		name       string
		periodType string
		prev, cur  string // period
		adjacent   bool
	}{
		{"monthly 相邻", "monthly", "2025-07", "2025-08", true},
		{"monthly 跨一期", "monthly", "2025-06", "2025-08", false},
		{"monthly 跨 13 个月（真跑撞到的形态）", "monthly", "2024-07", "2025-08", false},
		{"monthly 跨年相邻", "monthly", "2024-12", "2025-01", true},
		{"annual 相邻（相隔 12 个月）", "annual", "2024-12", "2025-12", true},
		{"annual 跨 3 年（真跑撞到的形态）", "annual", "2022-12", "2025-12", false},
		{"h1 相邻", "h1", "2024-06", "2025-06", true},
		{"q1 跨两期", "q1", "2023-03", "2025-03", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// 值取一个**必定超限**的跳变：若相邻性判据失效，它会 failed；
			// 判据生效时非相邻那几格该 skipped。两种结果截然不同，不会互相掩盖。
			cur := stockObsOf(tt.periodType, 100)
			cur.Meta.Period = tt.cur
			prev := stockObsOf(tt.periodType, 10)
			prev.Meta.Period = tt.prev

			rep, err := Validate(context.Background(), cur,
				fakeHistory{prior: []Observation{prev}}, DefaultThresholds())
			require.NoError(t, err)
			c := findCheck(t, rep, "stock_continuity")

			if tt.adjacent {
				require.Equal(t, CheckFailed, c.Status,
					"相邻且跳变超限 ⇒ 必须照常判 failed；这一格是防「一律 skip」的空实现")
				return
			}
			assert.Equal(t, CheckSkipped, c.Status,
				"prior[0] 与本期不相邻，拿它当基线算出来的环比是伪影，不得据此判 failed")
			assert.Contains(t, c.Reason, "non_adjacent_prior",
				"跳过理由必须能与 no_prior_period / prior_absent_field 区分——"+
					"三者的后续动作完全不同")
			assert.Contains(t, c.Reason, tt.prev, "理由里要带上那个不相邻的基线期次，否则查不出跨了多久")
		})
	}
}

// TestStockContinuityDoesNotUseRejectedPeriodAsBaseline 是**跨接缝**的那条
// （fix_items[1] 第 4 步）：它同时经过 `Preceding`（产出 prior）与 gate（消费 prior）。
//
// 🔴 为什么必须跨接缝：两端各自都对 —— `Preceding` 忠实返回「最近 N 个已接受的期次」，
// gate 忠实按 prior[0] 算环比。缺陷在**两者之间的那个假设**上（「已接受的最近一期
// 就是相邻上一期」），而它不属于任何一端，两端的测试都抓不到。
// 与本 sprint 的 A-1（`MergedObservation.Parts` vs `Observation.Parts`）**同形**。
//
// 场景：A 入库 → B 被拒（落 pending，不在 viewCurrent 里）→ C 来了，
// `Preceding(C)` 于是返回 A，而 A 与 C 相隔两个月。
func TestStockContinuityDoesNotUseRejectedPeriodAsBaseline(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	save := func(t *testing.T, period string, v float64, rep ValidationReport) Outcome {
		t.Helper()
		o := stockObsOf("monthly", v)
		o.Meta.Period = period
		o.Meta.PublishedAt = period + "-15"
		o.Meta.ArticleID = "art-" + period
		out, err := s.Save(ctx, o, rep)
		require.NoError(t, err)
		return out
	}

	// A：正常入权威表
	outA := save(t, "2025-01", 100, passing())
	require.Equal(t, TableObservations, outA.Table)

	// B：未过闸 ⇒ 落 pending ⇒ **不会出现在 viewCurrent 里**
	outB := save(t, "2025-02", 101, ValidationReport{
		Passed: false,
		Checks: []Check{{ID: "completeness", Status: CheckFailed, Reason: "missing 3: a,b,c"}},
	})
	require.Equal(t, TablePending, outB.Table, "未过闸的期次必须落 pending")

	// 接缝在这里：Preceding 看不见 B，于是把 A 当成 C 的「上一期」。
	prior, err := s.Preceding(ctx, "2025-03", "monthly", 6)
	require.NoError(t, err)
	require.Len(t, prior, 1, "B 落了 pending，viewCurrent 里只剩 A")
	require.Equal(t, "2025-01", prior[0].Meta.Period,
		"这就是缺陷的根：Preceding 返回的「最近一期」与 C 相隔两个月")

	// C：拿 A 当基线会算出 100→180 的伪跳变（80%，远超 monthly 上限）
	c3 := stockObsOf("monthly", 180)
	c3.Meta.Period = "2025-03"
	rep, err := Validate(ctx, c3, s, DefaultThresholds())
	require.NoError(t, err)

	got := findCheck(t, rep, "stock_continuity")
	assert.NotEqual(t, CheckFailed, got.Status,
		"拿跨期基线算出的环比是伪影，不得据此拒绝本期。"+
			"真跑里这个缺陷造成 4 条误拒，且构成正反馈：拒绝 → 基线跨度变长 → 更多拒绝，"+
			"而 --force 对已入权威表的期次是 no-op ⇒ 拦错了没有出路")
	assert.Equal(t, CheckSkipped, got.Status)
	assert.Contains(t, got.Reason, "non_adjacent_prior")
	assert.Contains(t, got.Reason, "2025-01", "要指出被跳过的那个基线是哪一期")
}

// —— M1c-4 的 TASK-007：deposit_sum 与 corp_loan_reconcile 按口径族分别校验 ——
//
// Context Checkpoint: done_criteria → test mapping（M1c-4 的 TASK-007）
// functional[0]  depositPartFieldsMoM 与 depositPartFields 逐项对应
//                                          → TestDepositPartFieldsAgreeAcrossCalibers
// functional[0]  corpLoanPartFieldsMoM 同上 → TestCorpLoanPartFieldsAgreeAcrossCalibers
// functional[1]  当月族齐全 ⇒ 不得 skipped{absent}
//                                          → TestDepositSumChecksMoMFamily
//                                            TestCorpLoanReconcileChecksMoMFamily
// functional[1]  两族都齐时取 ytd（裁决，不靠迭代顺序）
//                                          → TestDepositSumPrefersYTDWhenBothPresent
// functional[1]  两族都不齐 ⇒ skipped，且 Reason 说清哪一族缺
//                                          → TestDepositSumSkipsWhenNeitherFamilyComplete
// boundary[0]    两个 MoM 容差是未标定占位，且已标注
//                                          → TestMoMTolerancesAreDeclaredAndNonZero
// ⚠️ 既有的 TestCorpLoanSkipsOnZeroDenominator 不得被破坏（零分母 ⇒ skipped 而非 Inf/NaN）。

// momOnly 把一份 ytd 口径的 Values 整族翻成 mom 口径：凡是 `x_ytd` 且 fieldOrder 里
// 有同词干的 `x_mom`，就换成 mom 列。
//
// ⚠️ 用**翻转真实 golden**而不是手搓几个字段：手搓的 map 只含闸门要看的那几列，
// 一个「凡是缺 ytd 就当成 mom」的实现照样绿；翻转过来的观测其余字段也都是 mom 口径，
// 更接近真实的整族位移。
func momOnly(values map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(values))
	for k, v := range values {
		if stem, ok := strings.CutSuffix(k, "_ytd"); ok && slices.Contains(fieldOrder, stem+"_mom") {
			out[stem+"_mom"] = v
			continue
		}
		out[k] = v
	}
	return out
}

// 两族清单必须**逐项对应**：同一个部门在两族里位置相同。
//
// 判据是逐项比词干，不是「两边都有 4 个」——长度相等而顺序错位的两份清单会让
// depositResidualOf 算出一个「加总正确但配对错误」的残差，而它看起来完全正常。
func TestDepositPartFieldsAgreeAcrossCalibers(t *testing.T) {
	require.Len(t, depositPartFieldsMoM, len(depositPartFields))
	for i := range depositPartFields {
		assert.Equal(t,
			strings.TrimSuffix(depositPartFields[i], "_ytd"),
			strings.TrimSuffix(depositPartFieldsMoM[i], "_mom"),
			"第 %d 项两族不对应：%s vs %s", i, depositPartFields[i], depositPartFieldsMoM[i])
		assert.Truef(t, strings.HasSuffix(depositPartFields[i], "_ytd"), "%s 不是 ytd 列", depositPartFields[i])
		assert.Truef(t, strings.HasSuffix(depositPartFieldsMoM[i], "_mom"), "%s 不是 mom 列", depositPartFieldsMoM[i])
	}
}

func TestCorpLoanPartFieldsAgreeAcrossCalibers(t *testing.T) {
	require.Len(t, corpLoanPartFieldsMoM, len(corpLoanPartFields))
	for i := range corpLoanPartFields {
		assert.Equal(t,
			strings.TrimSuffix(corpLoanPartFields[i], "_ytd"),
			strings.TrimSuffix(corpLoanPartFieldsMoM[i], "_mom"),
			"第 %d 项两族不对应：%s vs %s", i, corpLoanPartFields[i], corpLoanPartFieldsMoM[i])
	}
}

// 🔴 当月族齐全时**必须真的判**，不得 skipped{absent}。
//
// 那正是本任务存在的理由：口径路由之后观测可能整族走 _mom，闸门只认 _ytd 会让
// 那些期次完全不被校验 —— 而报告上看不出「这一期没查过」，比拦错更糟。
func TestDepositSumChecksMoMFamily(t *testing.T) {
	obs := obsFrom(momOnly(golden2025), extractorV2)

	// 前提：这份观测确实一个 ytd 分部门列都没有，否则测的是隔壁那一族
	require.NotContains(t, obs.Values, FieldDepositFlowYTD, "用例前提：整族已翻成 mom")
	require.Contains(t, obs.Values, FieldDepositFlowMoM)

	rep, err := Validate(context.Background(), obs, NoHistory, DefaultThresholds())
	require.NoError(t, err)

	c := findCheck(t, rep, "deposit_sum")
	assert.NotEqualf(t, CheckSkipped, c.Status,
		"🔴 当月族齐全却 skipped —— 这一期完全没被校验，而报告上看不出来。Reason=%q", c.Reason)
	assert.NotContains(t, c.Reason, "absent_field")
	require.NotNil(t, c.Value, "判了就必须有残差值")

	// 残差与同一份数据走 ytd 时逐字相同：整族平移不改变加总关系
	ytdR, ok := depositResidualOf(golden2025, FieldDepositFlowYTD, depositPartFields)
	require.True(t, ok)
	assert.InDelta(t, ytdR, *c.Value, 1e-12,
		"整族翻成 mom 之后残差应当逐字不变——变了说明选族选到了别的列")
}

func TestCorpLoanReconcileChecksMoMFamily(t *testing.T) {
	obs := obsFrom(momOnly(golden2025), extractorV2)
	require.NotContains(t, obs.Values, FieldLoanCorpTotalYTD, "用例前提：整族已翻成 mom")
	require.Contains(t, obs.Values, FieldLoanCorpTotalMoM)

	rep, err := Validate(context.Background(), obs, NoHistory, DefaultThresholds())
	require.NoError(t, err)

	c := findCheck(t, rep, "corp_loan_reconcile")
	assert.NotEqualf(t, CheckSkipped, c.Status,
		"🔴 当月族齐全却 skipped —— 这一期完全没被校验。Reason=%q", c.Reason)
	assert.NotContains(t, c.Reason, "absent_field")
	require.NotNil(t, c.Value)

	// Value 仍是**亿元绝对量并保留符号**（与 deposit_sum 的比例刻意不同，见 gate 注释）
	sum := golden2025[FieldLoanCorpShortYTD] + golden2025[FieldLoanCorpMLTYTD] + golden2025[FieldLoanBillYTD]
	assert.InDelta(t, sum-golden2025[FieldLoanCorpTotalYTD], *c.Value, 1e-9,
		"整族翻成 mom 之后 Value 应当逐字不变，且仍是亿元绝对量")
}

// 🔴 两族都齐时**取 ytd**。这是裁决，不是实现细节。
//
// 靠 map 迭代顺序碰运气的实现会在两族都齐时随机选一族，而两族的容差不同、残差也不同
// ⇒ 同一份数据每次跑出的结论可能不一样。
//
// 判据是「取到的残差等于 ytd 那一族的」而不是「Reason 里写着 ytd」：后者一个把族名
// 写死成 "ytd" 的实现照样绿。
func TestDepositSumPrefersYTDWhenBothPresent(t *testing.T) {
	values := maps.Clone(golden2025)
	// 造一份两族都齐、但 mom 族残差**明显不同**的观测：mom 的分项全部减半，
	// 于是两族的残差必然不等，选错族一定看得出来。
	for i, f := range depositPartFields {
		values[depositPartFieldsMoM[i]] = values[f] / 2
	}
	values[FieldDepositFlowMoM] = values[FieldDepositFlowYTD]

	ytdR, ok := depositResidualOf(values, FieldDepositFlowYTD, depositPartFields)
	require.True(t, ok)
	momR, ok := depositResidualOf(values, FieldDepositFlowMoM, depositPartFieldsMoM)
	require.True(t, ok)
	require.Greater(t, math.Abs(ytdR-momR), 1e-6,
		"用例前提：两族残差必须不同，否则这一格分辨不出选了谁")

	rep, err := Validate(context.Background(), obsFrom(values, extractorV2), NoHistory, DefaultThresholds())
	require.NoError(t, err)
	c := findCheck(t, rep, "deposit_sum")
	require.NotNil(t, c.Value)
	assert.InDelta(t, ytdR, *c.Value, 1e-12,
		"两族都齐时必须取 ytd（主口径，容差的标定样本以它为主）")
}

// 两族都不齐 ⇒ skipped，且 Reason 要**说清是哪一族缺什么**。
//
// 只报一族的话运维会去查错的列 —— 而两族的列名长得很像，查错了不容易察觉。
func TestDepositSumSkipsWhenNeitherFamilyComplete(t *testing.T) {
	values := maps.Clone(golden2025)
	delete(values, FieldDepositHouseholdYTD) // ytd 族缺一项；mom 族本来就整族缺席

	rep, err := Validate(context.Background(), obsFrom(values, extractorV2), NoHistory, DefaultThresholds())
	require.NoError(t, err)

	c := findCheck(t, rep, "deposit_sum")
	require.Equal(t, CheckSkipped, c.Status)
	assert.Contains(t, c.Reason, "ytd:absent_field:"+FieldDepositHouseholdYTD, "要说清 ytd 族缺哪一项")
	assert.Contains(t, c.Reason, "mom:absent_field:", "两族的诊断都要带上")
}

// 两个 MoM 容差必须**写下来且非零**，否则走 mom 族的期次会因残差恒超而全部 failed。
//
// ⚠️ 本条**不**断言取值是多少 —— 它们是未标定占位，钉死取值等于把占位固化成契约。
// 「它是占位」这件事由 thresholds.go 的注释承担（那半是给人读的，没有断言能替代）。
func TestMoMTolerancesAreDeclaredAndNonZero(t *testing.T) {
	cfg := DefaultThresholds()
	assert.Greater(t, cfg.DepositSumToleranceMoM, 0.0,
		"为 0 会让每一期走 mom 族的观测都因残差超限而 failed")
	assert.Greater(t, cfg.CorpLoanToleranceMoM, 0.0)
}

// —— M1c-4 的 TASK-008：漂移基线改用 priorAll（含 pending），并加相邻性约束 ——
//
// Context Checkpoint: done_criteria → test mapping（M1c-4 的 TASK-008）
// functional[0] "基线含未过闸期次"     → TestDepositSumDriftBaselineIncludesPending
// functional[1] "基线不相邻时 skip"     → TestDepositSumSkipsOnNonAdjacentBaseline
// boundary[0]   "同族样本不足要说清"     → TestDepositSumSkipsWhenPriorsAreOtherCaliber
// boundary[1]   "stock_continuity 不跟着改" → TestStockContinuityStillReadsAcceptedPriorsOnly

// 🔴 正反馈链：闸门自己制造它要检测的异常。
//
// 真跑实测的链条（2024-04…08 被 tolerance 拒 ⇒ 落 pending ⇒ 基线看不见它们 ⇒
// 2024-10/11 因偏离一个**虚高的旧均值**被 drift 拒）在这里按同一形状复现。
//
// 正当性见 gateDepositSum 注释：基线用的是「残差」这个统计量，若常态只由已通过
// 这道闸的期次组成，drift 的定义本身就是循环的。
func TestDepositSumDriftBaselineIncludesPending(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	cfg := DefaultThresholds()

	depositObsAt := func(period string, pct float64) Observation {
		return Observation{
			Meta: Meta{
				Period: period, PeriodType: "monthly", PublishedAt: period + "-15",
				ArticleID: "art-" + period, CaliberVersion: "2025-01", Extractor: extractorV2,
			},
			Values: depositWith(pct),
		}
	}
	save := func(period string, pct float64, rep ValidationReport, wantTable string) {
		t.Helper()
		out, err := s.Save(ctx, depositObsAt(period, pct), rep)
		require.NoError(t, err)
		require.Equalf(t, wantTable, out.Table, "前置条件：%s 该落 %s", period, wantTable)
	}

	// 三期正常历史（残差 7.84%）进权威表
	for _, p := range []string{"2024-01", "2024-02", "2024-03"} {
		save(p, 7.84, passing(), TableObservations)
	}
	// 随后五期残差 14.6% 超 tolerance(0.12) ⇒ 落 pending ⇒ 权威表从此停在 2024-03
	for _, p := range []string{"2024-04", "2024-05", "2024-06", "2024-07", "2024-08"} {
		save(p, 14.6, failing(), TablePending)
	}

	// —— 前置事实：旧实现（只看权威表）此刻必判 drift_exceeded ——
	//
	// 不去模拟旧实现，而是把「旧基线是什么、它离本期有多远」当场算出来：
	// 模拟需要绕过新加的相邻性约束，绕的过程本身会变成一个没人复核的假设。
	acc, err := s.Preceding(ctx, "2024-09", "monthly", historyDepth)
	require.NoError(t, err)
	require.Len(t, acc, 3, "前提：2024-04…08 全落 pending，权威表里只剩最早那三期")
	var accSum float64
	for _, o := range acc {
		r, ok := depositResidualOf(o.Values, FieldDepositFlowYTD, depositPartFields)
		require.True(t, ok)
		accSum += r
	}
	staleMean := accSum / float64(len(acc))
	require.Greater(t, math.Abs(0.119-staleMean), cfg.DepositSumDriftMax,
		"前提：本期 11.9%% 相对那个**冻结的**旧均值 %.4f 已超漂移上限 —— "+
			"这一格红了说明造型没复现出正反馈，下面的绿就没有意义", staleMean)

	// —— 本期：残差 11.9% 未超 tolerance，且与「近五期真实发生过的 14.6%」很接近 ——
	rep, err := Validate(ctx, depositObsAt("2024-09", 11.9), s, cfg)
	require.NoError(t, err)
	c := findCheck(t, rep, "deposit_sum")
	assert.Equalf(t, CheckPassed, c.Status,
		"基线必须看得见落 pending 的那五期，否则闸门在拿自己拒绝的结果当常态。Reason=%q", c.Reason)
	assert.Empty(t, c.Reason,
		"2024-08 与本期相邻、同族历史足够 ⇒ 漂移是**真算过**的，不该带 drift_skipped")
}

// 基线不相邻时 **skip 而不是按跨度放宽**。
//
// 放宽需要一个放宽系数，而语料里没有数据能回答「跨 24 个月该放宽多少」；
// 往闸里塞一个没测过的数正是本任务在修的那类缺陷本身。
func TestDepositSumSkipsOnNonAdjacentBaseline(t *testing.T) {
	prior := []Observation{
		{Meta: priorMeta(1), Values: depositWith(10)}, // 2024-06，距本期 2026-06 隔 24 个月
		{Meta: priorMeta(2), Values: depositWith(10)},
		{Meta: priorMeta(3), Values: depositWith(10)},
	}
	rep, err := Validate(context.Background(),
		Observation{Meta: validMeta(), Values: depositWith(10)},
		fakeHistory{prior: prior}, DefaultThresholds())
	require.NoError(t, err)

	c := findCheck(t, rep, "deposit_sum")
	assert.Equal(t, CheckPassed, c.Status, "绝对值确实查过并通过了，记 passed 而不是 skipped")
	assert.Contains(t, c.Reason, "drift_skipped:non_adjacent_prior")
	assert.Contains(t, c.Reason, "2024-06", "要指出被跳过的基线是哪一期")
	assert.Contains(t, c.Reason, "2026-06", "以及它离本期有多远")
}

// 🔴 mom 族取不到同族基线是**设计的必然后果**，不是 bug —— 但必须在 Reason 里可数、可查。
//
// mom 与 ytd 期次在时间轴上交错，一个 mom 期次的前几期往往整批是 ytd，
// depositResidualOf 对它们逐个返回 false。⇒ 那批新救回的观测，drift 闸可能恒 skip
// 而**完全没有保护**。笼统记成 insufficient_history 会与「新库冷启动」混成一格，
// 而两者成因完全不同：冷启动等几期就好，本族样本不足是结构性的。
func TestDepositSumSkipsWhenPriorsAreOtherCaliber(t *testing.T) {
	// 历史四期全是 ytd 族，本期整族翻成 mom
	prior := []Observation{
		{Meta: priorMeta(0), Values: depositWith(10)},
		{Meta: priorMeta(1), Values: depositWith(10)},
		{Meta: priorMeta(2), Values: depositWith(10)},
		{Meta: priorMeta(3), Values: depositWith(10)},
	}
	obs := obsFrom(momOnly(golden2025), extractorV2)
	obs.Values[FieldDepositFlowMoM] = 100
	obs.Values[FieldDepositHouseholdMoM] = 90
	obs.Values[FieldDepositCorpMoM] = 0
	obs.Values[FieldDepositFiscalMoM] = 0
	obs.Values[FieldDepositNBFIMoM] = 0
	require.NotContains(t, obs.Values, FieldDepositFlowYTD, "用例前提：本期整族是 mom")

	rep, err := Validate(context.Background(), obs, fakeHistory{prior: prior}, DefaultThresholds())
	require.NoError(t, err)

	c := findCheck(t, rep, "deposit_sum")
	assert.Equal(t, CheckPassed, c.Status)
	assert.Contains(t, c.Reason, "drift_skipped:insufficient_same_caliber_history",
		"不得与「新库冷启动」的 no_prior_period 混成一格")
	assert.Contains(t, c.Reason, "mom", "要说清是哪一族的样本不足")
	assert.Contains(t, c.Reason, "n=0", "要带上**本族**实际拿到几期")
	assert.Contains(t, c.Reason, "prior=4",
		"以及一共有几期历史 —— 两个数一起才看得出「有历史但都不是这一族」")
}

// 接线断言：stock_continuity 读的是 in.prior（只含已过闸），**不是** in.priorAll。
//
// 两个方向都断言，缺一不可：只测前者会放过「两个都读」的实现，
// 只测后者会放过「压根没接线」的实现。
//
// 🔴 为什么这条必须钉住：两道闸对「未过闸的期次」要的东西相反。
// drift 问「近期常态是什么」，pending 里的期次是常态的一部分；
// stock_continuity 问「上一期的存量是多少」，拿一个**没通过校验的存量值**当基准，
// 等于用可疑数据去判可疑数据。M1c-3b 的 TASK-011 刚把这道闸的基线收严过一次。
func TestStockContinuityStillReadsAcceptedPriorsOnly(t *testing.T) {
	cfg := DefaultThresholds()
	obs := stockObsOf("monthly", 101)
	obs.Meta.Period = "2025-02"
	usable := stockObsOf("monthly", 100)
	usable.Meta.Period = "2025-01"

	// 方向一：只有 priorAll 有货 ⇒ 必须 skip（读了 priorAll 就会算出结果）
	only := gateStockContinuity(gateInput{obs: obs, priorAll: []Observation{usable}, cfg: cfg})
	assert.Equal(t, CheckSkipped, only.Status,
		"🔴 stock_continuity 读到了 priorAll —— 那是含未过闸期次的读口，"+
			"拿没通过校验的存量当基准，等于用可疑数据判可疑数据")
	assert.Equal(t, "no_prior_period", only.Reason)

	// 方向二：只有 prior 有货 ⇒ 必须真的算（否则上面那格是「压根没接线」也会绿）
	wired := gateStockContinuity(gateInput{obs: obs, prior: []Observation{usable}, cfg: cfg})
	assert.NotEqual(t, CheckSkipped, wired.Status,
		"prior 有货时必须真算，否则上一格分辨不出「没读 priorAll」还是「什么都没读」")
	require.NotNil(t, wired.Value)
}

// PrecedingAll 查库失败必须让整个 Validate 失败，**不能当成「没有历史」静默降级**。
//
// 降级的后果不是「少查一次漂移」而是「报告上写着 passed 而那道闸压根没查」——
// 与 drift_skipped 不同，静默降级在报告里不留任何痕迹。
func TestValidatePropagatesPrecedingAllError(t *testing.T) {
	boom := errors.New("boom: pending 表读不了")
	_, err := Validate(context.Background(),
		Observation{Meta: validMeta(), Values: depositWith(10)},
		fakeHistory{prior: []Observation{{Meta: priorMeta(0), Values: depositWith(10)}}, errAll: boom},
		DefaultThresholds())
	require.Error(t, err, "PrecedingAll 出错时 Validate 必须失败")
	assert.ErrorIs(t, err, boom)
}

// 🔴 权威表**一期都没有**、而 pending 里有历史时，仍然必须真算漂移。
//
// 这一格是变异测试逼出来的（M6：把 len(in.priorAll)==0 写成 len(in.prior)==0，
// 13 个变异体里它是唯一存活的真缺口）。上面那条 IncludesPending 分辨不出它——
// 那里权威表有三期，prior 非空，两种写法都往下走。
//
// 而这恰恰是正反馈链**最坏的形态**：近期每一期都被拒 ⇒ 权威表在这一段完全是空的
// ⇒ 误写成 in.prior 会报 no_prior_period，报告上看起来像「新库冷启动」，
// 而真相是「这段时间每一期都被这道闸拒了」。两者的运维含义完全相反。
func TestDepositSumDriftWorksWhenAllPriorsArePending(t *testing.T) {
	pendingOnly := []Observation{
		{Meta: priorMeta(0), Values: depositWith(10)}, // 2025-06，与本期 2026-06 相隔 12 个月（h1 相邻）
		{Meta: priorMeta(1), Values: depositWith(10)},
		{Meta: priorMeta(2), Values: depositWith(10)},
	}
	rep, err := Validate(context.Background(),
		Observation{Meta: validMeta(), Values: depositWith(10)},
		// prior 显式为空：权威表这一段一期都没有
		fakeHistory{prior: nil, priorAll: pendingOnly}, DefaultThresholds())
	require.NoError(t, err)

	c := findCheck(t, rep, "deposit_sum")
	assert.Equal(t, CheckPassed, c.Status)
	assert.Empty(t, c.Reason,
		"🔴 priorAll 里有三期同族历史，漂移必须**真算**。"+
			"报 no_prior_period 会把「这段每期都被拒」伪装成「新库冷启动」，运维含义相反")
}
