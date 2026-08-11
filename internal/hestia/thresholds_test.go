package hestia

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// M0 用两期真实报告加一期对照实测出的极值（见 vault
// Projects/Hestia/M0-契约样本与schema验证.md）。默认阈值必须容得下它们，
// 否则管线第一天就会把正常数据判成异常——DepositSumTolerance 的 ±2%
// 初稿就是这么被否掉的：三期实测残差 7.65% / 8.57% / 9.06%，每一期都会被拦。
const (
	m0MaxDepositResidual  = 0.0906
	m0MaxCorpLoanResidual = 0.0158
	m0MaxYoY              = 25.0
)

func TestDefaultThresholdsAdmitM0Measurements(t *testing.T) {
	c := DefaultThresholds()

	assert.Greater(t, c.DepositSumTolerance, m0MaxDepositResidual,
		"存款加总容差必须容得下 M0 实测的最大残差")
	assert.Greater(t, c.CorpLoanTolerance, m0MaxCorpLoanResidual,
		"企业贷款容差必须容得下 M0 实测的最大残差")
	assert.Greater(t, c.YoYSanityMax, m0MaxYoY,
		"同比上限必须容得下 M0 实测的最大同比")
}

// 区间表有意留空：必须用 M1c 回填分布标定，不得拍脑袋。
//
// 这条测试防的是「有人顺手填了几个看起来合理的数」——那会让一道本该沉默的
// 闸门开始产生未经验证的判定，**而它错了不会响亮失败，只是区间放宽**。
func TestDefaultThresholdsLeaveMagnitudeRangesUncalibrated(t *testing.T) {
	assert.Empty(t, DefaultThresholds().MagnitudeRanges,
		"区间表必须留空到 M1c 用回填分布标定")
}

func TestThresholdsRejectMalformedExemptions(t *testing.T) {
	base := CaliberExemption{
		Version:    "2025-01",
		Period:     "2025-01",
		SkipChecks: []string{"deposit_sum"},
		Reason:     "M1 口径纳入个人活期存款，环比与同比均不可比",
	}

	// 先确认 base 本身合法。少了这步，下面每个用例都可能因为别的原因报错，
	// 而测试照样绿——它只断言「有 error」，不关心是不是预期的那个。
	ok := DefaultThresholds()
	ok.CaliberExemptions = []CaliberExemption{base}
	require.NoError(t, ok.validate(), "base 必须合法，否则下面的用例证明不了任何事")

	mutate := func(f func(*CaliberExemption)) Thresholds {
		ex := base
		ex.SkipChecks = slices.Clone(base.SkipChecks)
		f(&ex)
		c := DefaultThresholds()
		c.CaliberExemptions = []CaliberExemption{ex}
		return c
	}

	tests := []struct {
		name string
		cfg  Thresholds
		want string
	}{
		{"未登记的口径版本", mutate(func(e *CaliberExemption) { e.Version = "2099-01" }), "unknown"},
		{"缺 Period", mutate(func(e *CaliberExemption) { e.Period = "" }), "Period"},
		{"Reason 只有空白", mutate(func(e *CaliberExemption) { e.Reason = "   " }), "Reason"},
		{"SkipChecks 为空", mutate(func(e *CaliberExemption) { e.SkipChecks = nil }), "SkipChecks"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestExemptionForMatchesPeriodAndCheckExactly(t *testing.T) {
	cfg := DefaultThresholds()
	cfg.CaliberExemptions = []CaliberExemption{{
		Version:    "2025-01",
		Period:     "2025-01",
		SkipChecks: []string{"deposit_sum", "stock_continuity"},
		Reason:     "口径切换期，加总与环比均不可比",
	}}
	require.NoError(t, cfg.validate())

	got := cfg.exemptionFor("2025-01", "deposit_sum")
	require.NotNil(t, got)
	assert.Equal(t, "2025-01", got.Version)

	assert.Nil(t, cfg.exemptionFor("2025-01", "yoy_sanity"),
		"未列入 SkipChecks 的闸门不该被豁免——豁免按检查 ID 精确指定")
	assert.Nil(t, cfg.exemptionFor("2025-02", "deposit_sum"),
		"豁免按期次精确匹配，不该外溢到相邻期")
}
