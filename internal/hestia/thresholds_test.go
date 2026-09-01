package hestia

import (
	"fmt"
	"maps"
	"math"
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

// 社融存量环比上限必须按 period_type 分成两档（M1c-2 的 TASK-001）。
//
// # 只断「五键齐全且都 > 0」是不够的
//
// 那样的话**把五个值写成同一个数的实现照样绿** —— 而分档正是这次改动的全部内容。
// 所以下面既钉住键集，也钉住两档之间的序关系。
//
// 键集用 periodTypeList() 而不是硬编码：加第六种 period_type 时这条会红，迫使那人
// 给新序列定一个上限。留着不定的话闸门对那条序列记 skipped{no_threshold} 整条消失，
// 而**那是静默的**。
//
// # 为什么是两档而不是五档
//
// Preceding 按 period_type 隔离序列 ⇒ 每年只有一个 h1 期次，h1 的「上一期」是
// **去年的 h1**。q1 / h1 / q1_q3 / annual 四种都是年初起累计口径，相邻两期一律相隔
// 12 个月，只有 monthly 相隔 1 个月。
//
// ⚠️ 不写成「月均上限 × 间隔月数」：那样只需一个数，但它**假设存量线性增长**，
// 而这道闸恰恰是用来抓「不该出现的跳变」的 —— 用线性模型定义「什么叫不线性」是循环的。
func TestDefaultStockContinuityIsTieredByPeriodType(t *testing.T) {
	m := DefaultThresholds().StockContinuityMax

	assert.Equal(t, periodTypeList(), slices.Sorted(maps.Keys(m)),
		"每个 period_type 都要有上限：缺一档不报错，只是那条序列的闸门静默消失")
	for pt, v := range m {
		assert.Positive(t, v, "stock_continuity_max[%s] 必须 > 0", pt)
	}

	assert.Less(t, m["monthly"], m["annual"],
		"monthly 相邻两期相隔 1 个月、annual 相隔 12 个月，前者的上限必须更紧；"+
			"两者相等说明分档只做了形式")
	for _, pt := range []string{"q1", "h1", "q1_q3"} {
		assert.Equal(t, m["annual"], m[pt],
			"%s 与 annual 同为年初起累计、相邻两期同样相隔 12 个月，应当同档", pt)
	}
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
		Version:     "2025-01",
		Period:      "2025-01",
		PeriodTypes: []string{"monthly"},
		SkipChecks:  []string{"deposit_sum"},
		Reason:      "M1 口径纳入个人活期存款，环比与同比均不可比",
	}

	// 先确认 base 本身合法。少了这步，下面每个用例都可能因为别的原因报错，
	// 而测试照样绿——它只断言「有 error」，不关心是不是预期的那个。
	ok := DefaultThresholds()
	ok.CaliberExemptions = []CaliberExemption{base}
	require.NoError(t, ok.validate(), "base 必须合法，否则下面的用例证明不了任何事")

	mutate := func(f func(*CaliberExemption)) Thresholds {
		ex := base
		ex.SkipChecks = slices.Clone(base.SkipChecks)
		ex.PeriodTypes = slices.Clone(base.PeriodTypes)
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
		Version:     "2025-01",
		Period:      "2025-01",
		PeriodTypes: []string{"monthly"},
		SkipChecks:  []string{"deposit_sum", "stock_continuity"},
		Reason:      "口径切换期，加总与环比均不可比",
	}}
	require.NoError(t, cfg.validate())

	// 三处调用一律传 "monthly"，与上面声明的 PeriodTypes 一致：本测试钉的是
	// 期次与检查 ID 两个维度，period_type 维度另由 TestExemptionIsolatesPeriodType
	// 单独钉。让它在这里恒匹配，下面两条 Nil 断言才只可能由各自那个维度造成。
	got := cfg.exemptionFor("2025-01", "monthly", "deposit_sum")
	require.NotNil(t, got)
	assert.Equal(t, "2025-01", got.Version)

	assert.Nil(t, cfg.exemptionFor("2025-01", "monthly", "yoy_sanity"),
		"未列入 SkipChecks 的闸门不该被豁免——豁免按检查 ID 精确指定")
	assert.Nil(t, cfg.exemptionFor("2025-02", "monthly", "deposit_sum"),
		"豁免按期次精确匹配，不该外溢到相邻期")
}

// —— TASK-008: 堵住「整期跳过校验」的两条路径（QA round2 的 R2-3）——

// 豁免不得宽到等价于「整期跳过校验」。
//
// 这不是新规矩，是把 validate() 自己的错误文案（「豁免必须按检查 ID 精确指定，
// **不是整期跳过校验**」）变成真的。在此之前该文案是**主动误导**：实现只查
// len(SkipChecks)==0，枚举全部七个 ID 就能绕过，QA 实测 cfg.validate()=nil、
// 0/7 闸门通过、数据进权威表。
//
// 两条路径的成因不同，故判据也不同：
//   - 覆盖全部 ID：字面意义的整期跳过 ⇒ 判据是**集合覆盖关系**
//   - 只豁免 completeness：其余六道遇缺字段一律降级 skipped，completeness 是
//     七道里**唯一**在数据缺失时会 failed 的闸门（这一点由 validate_test.go 的
//     TestValidateHandlesEmptyValuesWithoutSpecialCase 逐条断言：空 Values 下
//     completeness failed、其余六道全 skipped）⇒ 豁免它即等价于整期跳过
//
// ⚠️ 判据**不能是数量阈值**（`len(SkipChecks) > N`）：那会误伤正常的多闸豁免。
func TestThresholdsRejectWholePeriodSkip(t *testing.T) {
	withSkips := func(ids []string) Thresholds {
		c := DefaultThresholds()
		c.CaliberExemptions = []CaliberExemption{{
			Version: "2025-01", Period: "2025-01",
			PeriodTypes: []string{"monthly"},
			SkipChecks:  ids, Reason: "口径切换期",
		}}
		return c
	}

	// ⚠️ 两条断言各自钉的是**能区分两种形态**的那截文案，不是它们共有的「整期跳过」。
	// 消融实测：去掉覆盖全部的校验后，全 ID 输入会落到 completeness 那条规则上、
	// 照样报错，而它的文案里也含「整期跳过」⇒ 若只断言这四个字，这条子测试会
	// **被错误的规则满足**。这正是「有没有一个我想排除的实现能让断言照样绿」的实例。
	t.Run("枚举全部闸门即整期跳过", func(t *testing.T) {
		err := withSkips(knownCheckIDs()).validate()
		require.Error(t, err, "覆盖全部 ID 就是整期跳过校验，必须拒绝")
		assert.Contains(t, err.Error(), "caliber_exemptions[0]", "要指出是第几条豁免")
		assert.Contains(t, err.Error(), "跳过了全部",
			"必须由『覆盖全部』那条规则拒绝——只断言共有的「整期跳过」会被 completeness 规则冒名满足")
	})

	t.Run("只豁免 completeness 也是整期跳过", func(t *testing.T) {
		err := withSkips([]string{"completeness"}).validate()
		require.Error(t, err, "completeness 是唯一会因缺数据 failed 的闸门，豁免它等价于整期跳过")
		assert.Contains(t, err.Error(), "caliber_exemptions[0]")
		assert.Contains(t, err.Error(), "不得豁免 completeness", "要指出是哪个 ID 不能豁免")
	})

	t.Run("两种拒绝理由必须可区分", func(t *testing.T) {
		all := withSkips(knownCheckIDs()).validate()
		comp := withSkips([]string{"completeness"}).validate()
		require.Error(t, all)
		require.Error(t, comp)
		assert.NotEqual(t, all.Error(), comp.Error(),
			"两种形态的错误信息要能分辨，否则配置的人不知道该怎么改")
	})

	// 最容易写坏的一格：堵宽了会误伤正常的多闸豁免。
	t.Run("六道闸门（不含 completeness）仍合法", func(t *testing.T) {
		var six []string
		for _, id := range knownCheckIDs() {
			if id != "completeness" {
				six = append(six, id)
			}
		}
		require.Len(t, six, 6, "七道减去 completeness 应剩六道")
		assert.NoError(t, withSkips(six).validate(),
			"判据是集合覆盖关系而不是数量阈值；这里变红说明堵宽了")
	})
}

// checkCompleteness 是 gates 表里那个 ID 的第二处副本，必须不过期。
//
// 它一旦与 gates 脱节，上面那条「不得豁免 completeness」的校验会**静默失效**：
// slices.Contains 找不到匹配 ⇒ 永不触发 ⇒ QA 实测的那条整期跳过路径重新打开，
// 而没有任何测试会因此转红（新 ID 照样通过 checkEnum，因为它是从 gates 派生的）。
func TestCheckCompletenessIDMatchesGates(t *testing.T) {
	assert.Contains(t, knownCheckIDs(), checkCompleteness,
		"checkCompleteness 已与 gates 表脱节——豁免宽度校验会静默失效")
}

// —— TASK-006: 豁免键补上 PeriodTypes ——
//
// Context Checkpoint: done_criteria → test mapping
// functional[0]     只声明 annual 时 monthly/h1 都不命中     → TestExemptionIsolatesPeriodType
// functional[1]     一条豁免可覆盖三种 period_type          → TestExemptionCanCoverMultiplePeriodTypes
// boundary[0]       空切片报错、不默认全部                   → TestExemptionRejectsBadPeriodTypes/为空
// boundary[1]       PeriodTypes 不是新的宽度绕过路径         → TestAllPeriodTypesIsNotAWidthBypass
// error_handling[0] 未知取值报错，含非法值与合法取值列表      → TestExemptionRejectsBadPeriodTypes/含未知取值

// 豁免只对列出的 period_type 生效。
//
// 这是加 PeriodTypes 的**全部理由**：在此之前同月的 annual 与 monthly 会被同一条
// 豁免同时命中 —— 而它们是两条独立序列，一次只影响其中一条的口径变更会连带放行
// 另一条，且那次放行在报告里长得和一次深思熟虑的豁免一模一样。
func TestExemptionIsolatesPeriodType(t *testing.T) {
	cfg := DefaultThresholds()
	cfg.CaliberExemptions = []CaliberExemption{{
		Version:     "2025-01",
		Period:      "2025-01",
		PeriodTypes: []string{"annual"},
		SkipChecks:  []string{"deposit_sum"},
		Reason:      "年度口径切换",
	}}
	require.NoError(t, cfg.validate())

	assert.NotNil(t, cfg.exemptionFor("2025-01", "annual", "deposit_sum"))
	assert.Nil(t, cfg.exemptionFor("2025-01", "monthly", "deposit_sum"),
		"只声明了 annual，monthly 不该被命中")
	assert.Nil(t, cfg.exemptionFor("2025-01", "h1", "deposit_sum"))
}

// 一条豁免可以覆盖多种 period_type —— 一次口径变更常常三条序列都受影响。
//
// 与上一条互补而非重复：上一条防的是「一条豁免外溢到没声明的序列」，这一条防的是
// 「把匹配写成只取第一个 / 只允许单值」那类堵过头的实现。缺了它，一个恒取
// PeriodTypes[0] 的实现能让上一条照样绿。
func TestExemptionCanCoverMultiplePeriodTypes(t *testing.T) {
	cfg := DefaultThresholds()
	cfg.CaliberExemptions = []CaliberExemption{{
		Version:     "2025-01",
		Period:      "2025-01",
		PeriodTypes: []string{"monthly", "h1", "annual"},
		SkipChecks:  []string{"deposit_sum"},
		Reason:      "M1 口径纳入个人活期存款，三条序列同时受影响",
	}}
	require.NoError(t, cfg.validate())

	for _, pt := range []string{"monthly", "h1", "annual"} {
		assert.NotNil(t, cfg.exemptionFor("2025-01", pt, "deposit_sum"), "period_type=%s", pt)
	}
}

func TestExemptionRejectsBadPeriodTypes(t *testing.T) {
	mk := func(pts []string) Thresholds {
		c := DefaultThresholds()
		c.CaliberExemptions = []CaliberExemption{{
			Version: "2025-01", Period: "2025-01",
			PeriodTypes: pts,
			SkipChecks:  []string{"deposit_sum"},
			Reason:      "测试用",
		}}
		return c
	}

	// 两条断言各自钉的是**能区分两种形态**的那截文案：两条规则的错误信息都含
	// "PeriodTypes"，只断言它会让任一条被另一条冒名满足。
	t.Run("为空", func(t *testing.T) {
		err := mk(nil).validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "PeriodTypes")
		assert.Contains(t, err.Error(), "留空不等于",
			"必须由『为空』那条规则拒绝，而不是被『含未知取值』冒名满足")
	})
	t.Run("含未知取值", func(t *testing.T) {
		err := mk([]string{"monthly", "quarterly"}).validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "quarterly", "要指出是哪个取值非法")

		// 合法取值列表在错误信息里是硬编码的第三份副本（types.go:143 是第二份）。
		// 这个循环遍历 validPeriodTypes 本身，是那份副本的绊线：白名单加了第四种
		// 取值而错误文案没跟上，这里就红。
		for pt := range validPeriodTypes {
			assert.Contains(t, err.Error(), pt,
				"错误信息必须列出全部合法取值，%q 缺席说明文案已与白名单脱节", pt)
		}
	})
}

// 列出全部三种 period_type **不是**新的绕过路径。
//
// Sprint 035 收尾堵了两条「整期跳过校验」：SkipChecks 覆盖全部闸门、含 completeness。
// 加 PeriodTypes 后要确认这两条判据**仍只看 SkipChecks** —— 一条覆盖三种序列的豁免，
// 跳的仍是指定的那几道闸，是合法的。把这个组合钉住，免得后人以为「宽度校验也该看
// PeriodTypes」而在别处放松。
func TestAllPeriodTypesIsNotAWidthBypass(t *testing.T) {
	all := []string{"monthly", "h1", "annual"}

	t.Run("三种序列 + 一道闸：合法", func(t *testing.T) {
		c := DefaultThresholds()
		c.CaliberExemptions = []CaliberExemption{{
			Version: "2025-01", Period: "2025-01", PeriodTypes: all,
			SkipChecks: []string{"deposit_sum"}, Reason: "口径变更影响三条序列",
		}}
		assert.NoError(t, c.validate())
	})

	// 下面两条沿用 TestThresholdsRejectWholePeriodSkip 的钉法：断言的是能区分两种
	// 形态的那截文案，不是它们共有的「整期跳过」——只断言共有部分会被另一条规则
	// 冒名满足。
	t.Run("三种序列 + 全部闸门：仍然被拒", func(t *testing.T) {
		c := DefaultThresholds()
		c.CaliberExemptions = []CaliberExemption{{
			Version: "2025-01", Period: "2025-01", PeriodTypes: all,
			SkipChecks: knownCheckIDs(), Reason: "试图整期跳过",
		}}
		err := c.validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "跳过了全部")
	})

	t.Run("三种序列 + completeness：仍然被拒", func(t *testing.T) {
		c := DefaultThresholds()
		c.CaliberExemptions = []CaliberExemption{{
			Version: "2025-01", Period: "2025-01", PeriodTypes: all,
			SkipChecks: []string{checkCompleteness}, Reason: "试图走便宜的整期跳过",
		}}
		err := c.validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "不得豁免 "+checkCompleteness)
	})
}

// —— magnitude_ranges 的校验（M1c-3b 的 TASK-004）——
//
// Context Checkpoint: done_criteria → test mapping
// functional[1] 「未知字段名 ⇒ error，错误串含该键名」  → TestMagnitudeRangesRejectUnknownField
// functional[2] 「Min >= Max ⇒ error，错误串含字段名」  → TestMagnitudeRangesRejectInvertedRange
// functional[3] 「Unit 为空 ⇒ error，错误串含 unit」    → TestMagnitudeRangesRequireUnit
// boundary[0]   「空表仍然合法」                        → TestEmptyMagnitudeRangesStillValid

// TestMagnitudeRangesRejectUnknownField：表里出现 fieldOrder 之外的键 ⇒ 拒。
//
// 不拒的代价是静默：gateMagnitudeSanity 对未知键 continue，
// 配置的人以为那个字段被守着了，实际那道闸对它完全不设防。
// 与 caliber_exemptions 的 deposit_summ 同类，那个已被 checkEnum 堵住。
func TestMagnitudeRangesRejectUnknownField(t *testing.T) {
	th := DefaultThresholds()
	th.MagnitudeRanges = map[string]Range{
		"m2_typoed": {Min: 0, Max: 1000, Unit: "万亿元"},
	}
	err := th.validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "m2_typoed")
}

// TestMagnitudeRangesRejectInvertedRange：Min >= Max ⇒ 拒。
//
// 倒置的区间会让该字段**每一期都失败**，而失败理由串
// 「x=… outside [大,小]」读起来像数据错，不像配置错。
func TestMagnitudeRangesRejectInvertedRange(t *testing.T) {
	th := DefaultThresholds()
	th.MagnitudeRanges = map[string]Range{
		FieldM2: {Min: 1000, Max: 0, Unit: "万亿元"},
	}
	err := th.validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), FieldM2)
}

// TestMagnitudeRangesRequireUnit：Unit 为空 ⇒ 拒。
//
// Unit 只出现在失败理由串里，空着不影响判定 —— 正因如此它会被漏填，
// 而漏填之后失败信息少了唯一能判断「是不是单位搞错了」的那一项。
func TestMagnitudeRangesRequireUnit(t *testing.T) {
	th := DefaultThresholds()
	th.MagnitudeRanges = map[string]Range{
		FieldM2: {Min: 0, Max: 1000},
	}
	err := th.validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "unit")
}

// TestEmptyMagnitudeRangesStillValid：空表仍然合法。
//
// 空表 ⇒ magnitude_sanity 记 skipped{not_calibrated}，那是刻意设计的状态
// （M1b-3），不能因为加了校验就把它判非法。
func TestEmptyMagnitudeRangesStillValid(t *testing.T) {
	th := DefaultThresholds()
	require.NoError(t, th.validate())
}

// TestMagnitudeRangesAcceptValidTable：合法的非空表必须通过。
//
// ⚠️ 这条**不在 TASK-004 的 done_criteria 里**，是补的回归护栏，理由：上面三条
// 全部在校验中途 return，空表那条整块跳过 —— 「非空且合法 ⇒ 走完两轮循环、
// 落到 return nil」这条路径于是一条测试都没有。而它正是 M1c-3b 的 TASK-005
// 要走的那条：那个任务要手填 54 项，校验一旦退化成「非空即拒」，54 项会全被
// 挡在 LoadConfig 外面，且症状是配置的人怎么改都不对。
//
// 用三个不同板块的字段而不是一个：单字段过不了「循环只跑了一轮也算过」这关。
func TestMagnitudeRangesAcceptValidTable(t *testing.T) {
	th := DefaultThresholds()
	th.MagnitudeRanges = map[string]Range{
		FieldM2:        {Min: 200, Max: 400, Unit: "万亿元"},
		FieldM2YoY:     {Min: -5, Max: 25, Unit: "百分数"},
		FieldFXReserve: {Min: 3, Max: 4, Unit: "万亿美元"},
	}
	require.NoError(t, th.validate())
}

// Context Checkpoint: done_criteria → test mapping（M1c-3b 的 TASK-005）
// functional[0] 五档改为 monthly 0.05 / 其余 0.20 → TestStockContinuityMaxIsCalibrated
// functional[1] 钉住具体数字而非「大于 0」        → TestStockContinuityMaxIsCalibrated
// functional[2] 阈值须越过实测极值                → TestMonthlyThresholdCoversObservedMaximum

// 钉住的是**这四个数本身**，不是「大于 0」——它们各自有出处（见
// defaultStockContinuityMax 的注释），随手改一个应当在这里转红，
// 迫使改的人一并去更新那段出处。
//
// 与 TestDefaultStockContinuityIsTieredByPeriodType 不重复：那条钉的是**结构**
// （键集齐全、monthly 严格小于 annual、四种累计口径同档），改数字不会让它红；
// 这条钉的是**取值**。两条都在，结构与取值各有守卫。
func TestStockContinuityMaxIsCalibrated(t *testing.T) {
	assert.Equal(t, map[string]float64{
		"monthly": 0.05,
		"q1":      0.20,
		"h1":      0.20,
		"q1_q3":   0.20,
		"annual":  0.20,
	}, DefaultThresholds().StockContinuityMax,
		"这四个数来自 M1c-3b 的实测分布；改动前先读 defaultStockContinuityMax 的注释，"+
			"那段写明了每个数的出处与 n")
}

// 阈值必须**严格越过**实测极值，否则那个真实出现过的期次会被自己的闸门拦下。
//
// 这条是那次修正的墓碑：旧的 monthly=0.02 **低于实测 p95（0.02291）** ⇒ 至少 5%
// 的 monthly 期次会被判进 pending，而 `--force` 对已入权威表的期次是数据层
// no-op ⇒ **拦错了没有出路**。
//
// ⚠️ 断言写成「阈值 > 实测极值」而不是「阈值 == 0.05」：后者与上面那条重复，
// 且它钉的是取值、不是**性质**。这条要守的性质是「容得下已经观测到的东西」，
// 换一批语料重标之后它应当仍然成立。
func TestMonthlyThresholdCoversObservedMaximum(t *testing.T) {
	m := DefaultThresholds().StockContinuityMax

	assert.Greater(t, m["monthly"], 0.02613,
		"monthly 实测极值 0.026130169926258526（n=68，M1c-3b 标定）："+
			"阈值不越过它，那一期就会被自己的闸门拦下")
	assert.Greater(t, m["annual"], 0.13338,
		"annual 实测极值 0.13338108312442792（n=6，M1c-3b 标定）")
}

// —— magnitude_ranges 的守卫补完（M1c-3b 的 TASK-012）——
//
// Context Checkpoint: done_criteria → test mapping
// functional[0] NaN/Inf 穿透        → TestMagnitudeRangesRejectNaNOrInf
// functional[1] YAML 可达性          → TestMagnitudeRangesRejectNaNFromYAML
// functional[2] 存活变异 Min==Max    → TestMagnitudeRangesRejectDegenerateRange
// functional[2] 存活变异 Unit 空白   → TestMagnitudeRangesRejectBlankUnit
//
// ⚠️ 这四条的 RED 判据**不同**，别用同一把尺：
//   - 前两条在加 NaN 校验前**失败**（validate 返回 nil）⇒ 正常 TDD 的 RED
//   - 后两条在当前实现上**直接 PASS**（实现本来就拒绝它们）⇒ 补的是**守卫缺口**
//     而非行为缺口，其有效性只能靠**变异**证明（见各自注释里记录的实测结果）

// TestMagnitudeRangesRejectNaNOrInf：Min/Max 为 NaN 或 ±Inf ⇒ 拒。
//
// IEEE 754 规定涉 NaN 的比较除 != 外**恒假**，所以上面那条 r.Min >= r.Max
// 拦不住它；而下游 gateMagnitudeSanity 的判据 v < r.Min || v > r.Max
// **两侧同样恒假** ⇒ 该字段的幅度闸完全不设防，且报 passed。
// 这与「打错字段名」是同一失效模式的第二条入口。
//
// 实测（M1c-3b 的 TASK-004 交付后自查，隔离 worktree @ e0205607）：下面四种形态
// validate() **全部返回 nil**；配 fx_reserve=42（越界两个数量级）⇒ magnitude_sanity = passed。
func TestMagnitudeRangesRejectNaNOrInf(t *testing.T) {
	tests := []struct {
		name     string
		min, max float64
	}{
		{"min 与 max 都是 NaN", math.NaN(), math.NaN()},
		{"只有 min 是 NaN", math.NaN(), 1000},
		{"只有 max 是 NaN", 0, math.NaN()},
		{"负无穷到正无穷", math.Inf(-1), math.Inf(1)},
		{"只有 min 是 -Inf", math.Inf(-1), 1000},
		{"只有 max 是 +Inf", 0, math.Inf(1)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			th := DefaultThresholds()
			th.MagnitudeRanges = map[string]Range{
				FieldM2: {Min: tc.min, Max: tc.max, Unit: "万亿元"},
			}
			err := th.validate()
			require.Error(t, err, "NaN/Inf 区间会让该字段的幅度闸完全不设防且报 passed")
			// 断言字段名与成因都在，而不只是「有 error」：只断有 error 的话，
			// 任何一条别的校验顺手报错都能让这条测试变绿。
			require.Contains(t, err.Error(), FieldM2)
			require.Contains(t, err.Error(), "有限")
		})
	}
}

// TestMagnitudeRangesRejectNaNFromYAML：.nan / .inf 能从 YAML 字面量一路进到
// MagnitudeRanges，本条钉住这条路径已被堵上。
//
// 可达性不是理论构造：viper 走 yaml.v3，`min: .nan` 会被解析成真正的 NaN 而不报错。
// 实测（M1c-3b 的 TASK-004 交付后自查，加校验前）：往真配置追加 min: .nan 后
// LoadConfig err = nil，NaN 原样落进表里 —— 所以这条必须从 **YAML 出发**测，
// 只测结构体覆盖不到「YAML 能不能表达出这个值」这一段。
//
// ⚠️ 每个用例都刻意**避开** r.Min >= r.Max 那条既有校验，否则会因它而被拒、
// 从而证明不了 NaN/Inf 守卫的存在（实测踩过：min=.inf & max=1000 时
// +Inf >= 1000 为真 ⇒ 被倒置区间那条拦下，用例**为错误的理由变绿**）。
func TestMagnitudeRangesRejectNaNFromYAML(t *testing.T) {
	tests := []struct {
		name, min, max string
	}{
		// NaN 参与的比较恒假 ⇒ Min >= Max 为 false ⇒ 逃过既有校验
		{"min 是 .nan", ".nan", "1000"},
		{"max 是 .nan", "0", ".nan"},
		{"min 与 max 都是 .nan", ".nan", ".nan"},
		// -Inf < 1000、0 < +Inf ⇒ Min >= Max 均为 false ⇒ 同样逃过
		{"min 是 -.inf", "-.inf", "1000"},
		{"max 是 .inf", "0", ".inf"},
		{"负无穷到正无穷", "-.inf", ".inf"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := writeConfig(t, `
config_version: "2026-08-12"
storage:
  db_path: data/hestia.db
discover:
  index_url: https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/index.html
  max_pages: 3
  timeout: 30s
thresholds:
  magnitude_ranges:
    m2:
      min: `+tc.min+`
      max: `+tc.max+`
      unit: "万亿元"
`)
			_, err := LoadConfig(p)
			require.Error(t, err, "YAML 字面量 min=%s max=%s 必须在装载时被拒，"+
				"而不是静默变成一道不设防的闸", tc.min, tc.max)
			require.Contains(t, err.Error(), FieldM2)
			// 断「有限」而不只是断字段名：这几个用例都刻意避开了倒置区间那条校验，
			// 但若将来有人放宽了它，只断字段名的用例会被那条错误冒名满足。
			require.Contains(t, err.Error(), "有限")
		})
	}
}

// TestMagnitudeRangesRejectDegenerateRange：Min == Max ⇒ 拒。
//
// 🔴 这条补的是**守卫缺口**，不是行为缺口：当前实现（r.Min >= r.Max）本来就拒绝它，
// 所以本测试在**未做任何实现改动时就是 PASS 的**。它的有效性只能靠变异证明 ——
// 把 >= 改成 >，本条必须转红（M1c-3b 的 TASK-012 已实测，见 discovery 的 mutation 段）。
//
// 为什么值得补：M1c-3b 的 TASK-004 的 discovery 在 interfaces_exposed 里向 TASK-005
// **广告了「min == max 也会被拒」这条契约**，而在本任务之前没有任何测试钉住它。
// 谁把 >= 顺手写成 >，这条已被下游依赖的契约就静默失效。
func TestMagnitudeRangesRejectDegenerateRange(t *testing.T) {
	th := DefaultThresholds()
	th.MagnitudeRanges = map[string]Range{
		FieldM2: {Min: 100, Max: 100, Unit: "万亿元"},
	}
	err := th.validate()
	require.Error(t, err, "min == max 的区间宽度为零，该字段每一期都会失败")
	// 断 min(100) 而不只是断字段名：缺 unit 那条错误串里同样含 FieldM2，
	// 只断字段名的话，判据与用例数据耦合，换个数据就分不清是哪条分支在响。
	require.Contains(t, err.Error(), "min(100)")
}

// TestMagnitudeRangesRejectBlankUnit：Unit 为纯空白 ⇒ 拒。
//
// 🔴 同上，补的是**守卫缺口**：当前实现用 strings.TrimSpace 本来就拒绝它，本测试
// 未改实现即 PASS。有效性靠变异证明 —— 把 TrimSpace(r.Unit) == "" 改成
// r.Unit == ""，本条必须转红（已实测）。
//
// 单独测 TrimSpace **有本文件的先例**：TestThresholdsRejectMalformedExemptions
// 的表里就有 Reason = "   " 那一档。照既有惯例写。
func TestMagnitudeRangesRejectBlankUnit(t *testing.T) {
	for _, unit := range []string{"   ", "\t", " \n "} {
		t.Run(fmt.Sprintf("%q", unit), func(t *testing.T) {
			th := DefaultThresholds()
			th.MagnitudeRanges = map[string]Range{
				FieldM2: {Min: 0, Max: 1000, Unit: unit},
			}
			err := th.validate()
			require.Error(t, err, "纯空白的 unit 与漏填等价，失败理由串里会是一片空白")
			require.Contains(t, err.Error(), "unit")
		})
	}
}
