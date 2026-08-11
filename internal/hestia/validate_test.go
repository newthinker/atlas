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
	// 与 M0 契约样本一致的闸门 ID。T5 加 deposit_sum、T6 加 stock_continuity，
	// T7 断言恰好七道。
	wantGateIDs := []string{
		"monetary_hierarchy",
		"corp_loan_reconcile",
		"yoy_sanity",
		"completeness",
		"magnitude_sanity",
	}
	require.Equal(t, wantGateIDs, gateIDs(),
		"gates 表本身必须恰好是这五道——少一道时下面的逐行比对会跟着缩水而发现不了")

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
