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
//
// # 读到这里的人：填表时改这条断言还不够，另有三件事必须一并补
//
// magnitude_sanity 现在恒 skipped{not_calibrated}，所以它的守卫缺口至今**影响为零**。
// 填表的那一刻这道闸开始真正判定，三个缺口同时变成真的（详见
// .arcforge/docs/02-plan/findings-carryover.md 的 F12 / F17 / F19）：
//
//  1. **遍历顺序守卫**（F12）：实现须遍历 fieldOrder 而非 map ——
//     map 迭代顺序随机，同一份数据两次跑会报出不同的越界字段，排查变成猜谜。
//     当前该缺口是**存活变异**（改成遍历 map 后整套测试全绿）。补救方法已实证。
//  2. **Min / Max 两个边界方向**（F17 #5/#6）：`v < r.Min` 与 `v > r.Max` 各自的
//     比较符现在都没有守卫。现有用例用 42 对区间 [3,4]，**离边界两个数量级**，
//     把 `<` 改成 `<=` 不会有任何测试转红。
//  3. **Range.Unit 的单位**：Unit 至今没有消费者。填表时写下单位就会发现，
//     包注释声称的三类单位（万亿元/亿元/百分数）不覆盖 fx_reserve（万亿美元）
//     与 fx_rate（元/美元）。
//
// 为什么把这段挂在这里而不是只写在那份文档里：**这条断言是必然会响的绊线** ——
// 任何人填表都必须先撞红它、必须动手改它，于是必然读到这段。而 findings-carryover.md
// 是一份**需要被记起来**的文档，M1c 那天的真实路径是「填表 → 红 → 改它 → 没有
// 任何东西提示还要补边界测试」，除非那人恰好想起 F17。
// 同 store_test.go 的导出面守卫：**把提醒挂在绊线上，而不是挂在需要被记起来的文档里。**
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
