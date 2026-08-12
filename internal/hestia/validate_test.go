package hestia

import (
	"context"
	"errors"
	"maps"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeHistory 让闸门测试不必建库。切片按 period 降序，与 Store.Preceding 的契约一致。
type fakeHistory struct {
	prior []Observation
	err   error
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

// obsFrom 用 golden 数据造一个观测。复制 Values——测试会改动它，
// 而 golden 是包级变量，改坏了会污染其他测试。
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
	priorWith := func(pcts ...float64) []Observation {
		out := make([]Observation, 0, len(pcts))
		for _, p := range pcts {
			out = append(out, Observation{Meta: validMeta(), Values: depositWith(p)})
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
		{"历史不足三期", 10, []float64{10, 10}, CheckPassed, "drift_skipped:insufficient_history", 0.10},
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
		for range n {
			out = append(out, Observation{Meta: validMeta(), Values: depositWith(pct)})
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
	withResidual := func(pct float64) Observation {
		return Observation{Meta: validMeta(), Values: depositWith(pct)}
	}
	// 缺一个部门分项 ⇒ depositResidual 返回 false
	incomplete := func() Observation {
		o := withResidual(10)
		delete(o.Values, FieldDepositNBFIYTD)
		return o
	}

	prior := []Observation{
		withResidual(10), withResidual(10), withResidual(10),
		incomplete(), incomplete(),
	}

	rep, err := Validate(context.Background(), withResidual(10),
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

// stockObsWithout 造缺 tsf_stock 的观测（模拟没有社融板块的 v1 期次）。
func stockObsWithout() Observation {
	o := stockObs(0)
	delete(o.Values, FieldTSFStock)
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
			stockObsWithout(), []Observation{stockObs(400)},
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
			stockObs(400), []Observation{stockObsWithout()},
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
func TestStockContinuityDetectsJump(t *testing.T) {
	tests := []struct {
		name       string
		prev, cur  float64
		wantStatus CheckStatus
		wantRatio  float64
	}{
		// 8/400 的浮点商与字面量 0.02 **舍入到同一个 double**，所以 r <= max 成立。
		//
		// ⚠️ 注意措辞：0.02 **本身并不精确可表示**（位模式 0x3f947ae147ae147b，
		// 实为 0.020000000000000000416）。这里成立的条件是两边落到同一个 double，
		// 而不是「比例是精确值」。
		//
		// 让它成立的是**参与运算的量精确**（cur−prev 与 prev 都是精确整数），
		// **不是算路短**。实测同一道闸在非整数输入下同样失效：
		//   400→408 / 300→306 / 250→255 / 350→357  ⇒ r == 0.02 为 true
		//   123→125.46                             ⇒ false（低 15 ULP）
		//   400.1→408.102                          ⇒ false
		// ⇒ **改这几行常量时必须保持 cur−prev 与 prev 为精确整数**；换成「看起来
		// 更真实」的小数会让这条边界测试**静默失效**，而不会有任何东西转红。
		{"恰好在阈值上", 400, 408, CheckPassed, 0.02},
		{"阈值内", 400, 404, CheckPassed, 0.01},
		{"超过阈值", 400, 420, CheckFailed, 0.05},
		{"存量下跌也算跳变", 400, 380, CheckFailed, 0.05},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rep, err := Validate(context.Background(), stockObs(tt.cur),
				fakeHistory{prior: []Observation{stockObs(tt.prev)}},
				DefaultThresholds())
			require.NoError(t, err)

			c := findCheck(t, rep, "stock_continuity")
			assert.Equal(t, tt.wantStatus, c.Status, "reason=%s", c.Reason)
			require.NotNil(t, c.Value)
			assert.InDelta(t, tt.wantRatio, *c.Value, 1e-9)
		})
	}
}

// 上一期存量为 0 时记 skipped 而不是算出 Inf（DoD error_handling[0]）。
// 计划实现了这条分支却没有为它写测试，与 deposit_sum 那条同形。
func TestStockContinuitySkipsOnZeroDenominator(t *testing.T) {
	rep, err := Validate(context.Background(), stockObs(400),
		fakeHistory{prior: []Observation{stockObs(0)}}, DefaultThresholds())
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
