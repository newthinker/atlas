package hestia

import (
	"maps"
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
