package crisis

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// 指标行渲染（通知设计 §5 示例逐条对照）。
func TestIndicatorLineRendering(t *testing.T) {
	cfg := testConfig()
	assert.Equal(t, "🔴 信用 hy_oas 612bp · 5y分位 98% · 压力(STRESS)",
		indicatorLine(cfg, IndicatorResult{Indicator: IndHYOAS, Status: StatusRed, Value: 612, Pct5y: 0.98, Tag: TagStress}))
	assert.Equal(t, "🔴 流动性 sofr_effr +28bp · 持续 5 个交易日",
		indicatorLine(cfg, IndicatorResult{Indicator: IndSOFREFFR, Status: StatusRed, Value: 28, Pct5y: 0.99, PersistDays: 5}))
	// wow 触发（-2.1% ≤ amber_wow_pct=-2%）→ 周跌片段
	assert.Equal(t, "🟡 旁证 usdjpy 158.9 · 周跌 2.1%",
		indicatorLine(cfg, IndicatorResult{Indicator: IndUSDJPY, Status: StatusAmber, Value: 158.9, Wow: -0.021, WowOK: true}))
	// CROWDED-only（wow 未触发）→ 无周跌片段（补充决策 7）
	assert.Equal(t, "🟡 旁证 usdjpy 161.7 · 空头拥挤(CROWDED)",
		indicatorLine(cfg, IndicatorResult{Indicator: IndUSDJPY, Status: StatusAmber, Value: 161.7, Wow: -0.005, WowOK: true, Tag: TagCrowded}))
	assert.Equal(t, "🟢 情绪 vix 18.2 · 5y分位 41%",
		indicatorLine(cfg, IndicatorResult{Indicator: IndVIX, Status: StatusGreen, Value: 18.2, Pct5y: 0.41}))
	// Pct5y<0 → 省略分位片段
	assert.Equal(t, "🟢 领先 nfci -0.52",
		indicatorLine(cfg, IndicatorResult{Indicator: IndNFCI, Status: StatusGreen, Value: -0.52, Pct5y: -1}))
	// ⚪ 状态（设计 §6.1）
	assert.Equal(t, "⚪ 情绪 move 88.1 · 数据断更(STALE)",
		indicatorLine(cfg, IndicatorResult{Indicator: IndMOVE, Status: StatusStale, Value: 88.1}))
	assert.Equal(t, "⚪ 领先 nfci 无数据(NO_DATA)",
		indicatorLine(cfg, IndicatorResult{Indicator: IndNFCI, Status: StatusNoData}))
}

// 恰好落界用例：片段的触发是 severity>=AMBER、Wow<=amber_wow_pct 两个含等号的判别，
// 补 severity 恰好=AMBER 与 Wow 恰好==amber_wow_pct 的用例锁方向（防 >=→>、<=→< 变异）。
func TestIndicatorLineBoundaryTriggers(t *testing.T) {
	cfg := testConfig()
	// severity 恰好=AMBER（非 RED）→ sofr 持续片段仍显示
	assert.Equal(t, "🟡 流动性 sofr_effr +15bp · 持续 3 个交易日",
		indicatorLine(cfg, IndicatorResult{Indicator: IndSOFREFFR, Status: StatusAmber, Value: 15, PersistDays: 3}))
	// Wow 恰好==amber_wow_pct(-0.02) → 周跌片段仍触发（<= 边界）
	assert.Equal(t, "🟡 旁证 usdjpy 158.0 · 周跌 2.0%",
		indicatorLine(cfg, IndicatorResult{Indicator: IndUSDJPY, Status: StatusAmber, Value: 158, Wow: -0.02, WowOK: true}))
}

// 分区排序（通知设计 §6.2，测试要点 2）：异常区严重度降序 + 冰山层序；⚪ 殿后。
func TestSplitZonesOrdering(t *testing.T) {
	res := dayResult(StateWatch, StateWatch)
	set := func(ind string, st Status) {
		r := res.Results[ind]
		r.Status = st
		res.Results[ind] = r
	}
	set(IndVIX, StatusRed)
	set(IndSOFREFFR, StatusRed)
	set(IndHYOAS, StatusAmber)
	set(IndMOVE, StatusStale)

	abnormal, rest := splitZones(res)
	var got []string
	for _, r := range abnormal {
		got = append(got, r.Indicator)
	}
	// 红先于黄；同为红按冰山层序：流动性(sofr) 先于 情绪(vix)
	assert.Equal(t, []string{IndSOFREFFR, IndVIX, IndHYOAS}, got)
	// 其余区固定 AllIndicators 序，⚪ 殿后
	var restInds []string
	for _, r := range rest {
		restInds = append(restInds, r.Indicator)
	}
	assert.Equal(t, []string{IndT10Y2Y, IndNFCI, IndUSDJPY, IndMOVE}, restInds)

	// 第三级：同严重度同冰山层 → AllIndicators 序（vix 先于 move，锁第三级 indicatorIndex 全序比较）
	res2 := dayResult(StateWatch, StateWatch)
	for _, ind := range []string{IndVIX, IndMOVE} {
		r := res2.Results[ind]
		r.Status = StatusAmber
		res2.Results[ind] = r
	}
	ab2, _ := splitZones(res2)
	assert.Equal(t, []string{IndVIX, IndMOVE}, []string{ab2[0].Indicator, ab2[1].Indicator})

	// indicatorIndex 兜底：未知指标排在 AllIndicators 之后
	assert.Equal(t, 0, indicatorIndex(IndVIX))
	assert.Equal(t, len(AllIndicators), indicatorIndex("unknown"))
}

// 区块标题（设计 §4 + 补充决策 5）。
func TestBodyZonesTitles(t *testing.T) {
	cfg := testConfig()
	res := dayResult(StateWatch, StateWatch) // 全绿
	body := bodyZones(cfg, res, "异常指标：")
	assert.True(t, strings.HasPrefix(body, "7 指标全绿：\n🟢 情绪 vix"))
	assert.NotContains(t, body, "异常指标：")

	r := res.Results[IndHYOAS]
	r.Status = StatusAmber
	res.Results[IndHYOAS] = r
	body = bodyZones(cfg, res, "触发共振：")
	assert.Contains(t, body, "触发共振：\n🟡 信用 hy_oas")
	assert.Contains(t, body, "\n\n其余指标：\n🟢 情绪 vix")

	// 全非异常但含 ⚪ → 不用"全绿"标题（补充决策 5）
	r = res.Results[IndHYOAS]
	r.Status = StatusStale
	res.Results[IndHYOAS] = r
	body = bodyZones(cfg, res, "异常指标：")
	assert.True(t, strings.HasPrefix(body, "其余指标：\n"))
	// dayResult 夹具 Value=1，formatReading(IndHYOAS, 1) → "1bp"
	assert.True(t, strings.HasSuffix(body, "⚪ 信用 hy_oas 1bp · 数据断更(STALE)"))
}

func TestMonthDayAndStateRank(t *testing.T) {
	assert.Equal(t, "07-14", monthDay("2026-07-14"))
	// boundary[2]：非 YYYY-MM-DD 长度（≠10）→ 原样返回（防御 fallback）
	assert.Equal(t, "2026-07", monthDay("2026-07"))
	assert.Equal(t, "", monthDay(""))
	assert.True(t, stateRank(StateCrisis) > stateRank(StateBrewing))
	assert.True(t, stateRank(StateBrewing) > stateRank(StateWatch))
	assert.True(t, stateRank(StateWatch) > stateRank(StateNormal))
}

// functional[0]：8 个可达转移每个都有区分断言（多键表——只测部分键剩余键可随便改字）。
// boundary[0]：未知转移返回空串。（设计 §4.1）
func TestSemanticSentenceAllTransitions(t *testing.T) {
	cfg := testConfig() // CrisisExitDays=10, BrewingExitDays=10, WatchExitDays=20
	cases := []struct {
		from, to SystemState
		want     string
	}{
		{StateNormal, StateWatch, "领先层或多指标共振异常。观察期：提高警觉，尚无行动含义。"},
		{StateWatch, StateBrewing, "信用与流动性双红共振。历史样本中此组合后系统性风险抬升比例显著（样本量小，可能失效）；此为状态描述而非预测，不构成操作依据。"},
		{StateNormal, StateCrisis, "情绪层双红：危机进行中。此阶段执行预案而非预测。"},
		{StateWatch, StateCrisis, "情绪层双红：危机进行中。此阶段执行预案而非预测。"},
		{StateBrewing, StateCrisis, "情绪层双红：危机进行中。此阶段执行预案而非预测。"},
		{StateCrisis, StateWatch, "情绪层连续 10 个交易日回落至绿。危机状态退出，转入观察期；信用/流动性等其余层面可能仍异常，见下。"},
		{StateBrewing, StateWatch, "信用/流动性共振解除并稳定 10 个交易日。回到观察期。"},
		{StateWatch, StateNormal, "全部触发条件解除并稳定 20 个交易日。回到常态。"},
	}
	for _, c := range cases {
		assert.Equalf(t, c.want, semanticSentence(cfg, c.from, c.to), "%s→%s", c.from, c.to)
	}
	// boundary[0]：不可达/未知转移 → 空串
	assert.Equal(t, "", semanticSentence(cfg, StateNormal, StateNormal))
	assert.Equal(t, "", semanticSentence(cfg, StateCrisis, StateNormal))
}

// functional[1]：三个 %d 注入转移各配 YAML 调参跟随断言（锁"注入"而非硬编码天数）。
func TestSemanticSentenceConfigInjection(t *testing.T) {
	cfg := testConfig()
	cfg.StateMachine.CrisisExitDays = 7
	cfg.StateMachine.BrewingExitDays = 12
	cfg.StateMachine.WatchExitDays = 25
	assert.Contains(t, semanticSentence(cfg, StateCrisis, StateWatch), "连续 7 个交易日")
	assert.Contains(t, semanticSentence(cfg, StateBrewing, StateWatch), "稳定 12 个交易日")
	assert.Contains(t, semanticSentence(cfg, StateWatch, StateNormal), "稳定 25 个交易日")
}

// 状态升级（§5.1 逐段对照）：首行/语义句/触发共振/尾注/页脚。
func TestRenderTransitionUpgrade(t *testing.T) {
	cfg := testConfig()
	res := dayResult(StateWatch, StateBrewing)
	res.Date = "2026-07-14"
	set := func(ind string, r IndicatorResult) { res.Results[ind] = r }
	set(IndHYOAS, IndicatorResult{Indicator: IndHYOAS, Status: StatusRed, Value: 612, Pct5y: 0.98, Tag: TagStress})
	set(IndSOFREFFR, IndicatorResult{Indicator: IndSOFREFFR, Status: StatusRed, Value: 28, PersistDays: 5})

	msg := renderTransition(cfg, NotifyContext{Res: res, StateDays: 12})
	assert.True(t, strings.HasPrefix(msg, "[P0] 🚨 状态升级 WATCH → BREWING · 07-14\n\n"))
	assert.Contains(t, msg, "信用与流动性双红共振")
	assert.Contains(t, msg, "此为状态描述而非预测，不构成操作依据") // R2（v1.1）
	assert.NotContains(t, msg, "3–12 个月")         // R2：删除具体时窗预测
	assert.Contains(t, msg, "触发共振：\n🔴 信用 hy_oas 612bp · 5y分位 98% · 压力(STRESS)\n🔴 流动性 sofr_effr +28bp · 持续 5 个交易日")
	assert.Contains(t, msg, "\n\n其余指标：\n")
	assert.Contains(t, msg, "WATCH 已持续 12 个评估日 → BREWING · 下一评估：下一交易日")
	assert.True(t, strings.HasSuffix(msg, notifyFooter))

	// 恰好落界：进 WATCH（非 BREWING/CRISIS）→ [P1] ⚠️（锁 State==BREWING||CRISIS 的方向）
	msg = renderTransition(cfg, NotifyContext{Res: dayResult(StateNormal, StateWatch), StateDays: 63})
	assert.True(t, strings.HasPrefix(msg, "[P1] ⚠️ 状态升级 NORMAL → WATCH"))
	assert.Contains(t, msg, "领先层或多指标共振异常")

	// 进 CRISIS → [P0] 且危机语义句
	msg = renderTransition(cfg, NotifyContext{Res: dayResult(StateBrewing, StateCrisis), StateDays: 34})
	assert.True(t, strings.HasPrefix(msg, "[P0] 🚨 状态升级 BREWING → CRISIS"))
	assert.Contains(t, msg, "情绪层双红：危机进行中")
}

// 状态降级（§5.2）+ 语义句 %d 注入跟随配置（测试要点 6）。
func TestRenderTransitionDowngrade(t *testing.T) {
	cfg := testConfig()
	res := dayResult(StateBrewing, StateWatch)
	res.Date = "2026-09-02"
	r := res.Results[IndHYOAS]
	r.Status = StatusAmber
	res.Results[IndHYOAS] = r

	msg := renderTransition(cfg, NotifyContext{Res: res, StateDays: 34})
	// hy_oas=AMBER → 异常区非空 → 🔽 状态回落（R2，v1.1）
	assert.True(t, strings.HasPrefix(msg, "[P1] 🔽 状态回落 BREWING → WATCH · 09-02"))
	assert.Contains(t, msg, "稳定 10 个交易日") // brewing_exit_days=10
	assert.Contains(t, msg, "仍异常：\n🟡 信用 hy_oas")
	assert.Contains(t, msg, "BREWING 共持续 34 个评估日 · 下一评估：下一交易日")
	assert.True(t, strings.HasSuffix(msg, notifyFooter))

	cfg.StateMachine.BrewingExitDays = 12 // YAML 调参 → 文案跟随
	assert.Contains(t, renderTransition(cfg, NotifyContext{Res: res, StateDays: 34}), "稳定 12 个交易日")

	// CRISIS→WATCH（全绿 → 异常区空 → ✅ 状态解除）；R3：措辞改「危机状态退出」
	msg = renderTransition(cfg, NotifyContext{Res: dayResult(StateCrisis, StateWatch), StateDays: 20})
	assert.True(t, strings.HasPrefix(msg, "[P1] ✅ 状态解除 CRISIS → WATCH"))
	assert.Contains(t, msg, "情绪层连续 10 个交易日回落至绿")
	assert.Contains(t, msg, "危机状态退出，转入观察期")
	assert.NotContains(t, msg, "危机状态解除")
	// WATCH→NORMAL 注入 watch_exit_days
	msg = renderTransition(cfg, NotifyContext{Res: dayResult(StateWatch, StateNormal), StateDays: 40})
	assert.Contains(t, msg, "稳定 20 个交易日。回到常态")
}

// R2（设计 v1.1）：✅ 仅限异常区为空的降级；非空用 🔽 状态回落。恰好落界：
// 恰有一个 🟡 即切换。
func TestRenderTransitionConditionalGlyph(t *testing.T) {
	cfg := testConfig()

	// 全绿降级 → ✅ 状态解除
	msg := renderTransition(cfg, NotifyContext{Res: dayResult(StateWatch, StateNormal), StateDays: 40})
	assert.True(t, strings.HasPrefix(msg, "[P1] ✅ 状态解除 WATCH → NORMAL"))
	assert.NotContains(t, msg, "状态回落")

	// 恰好一个 AMBER → 🔽 状态回落（落界）
	res := dayResult(StateWatch, StateNormal)
	r := res.Results[IndHYOAS]
	r.Status = StatusAmber
	res.Results[IndHYOAS] = r
	msg = renderTransition(cfg, NotifyContext{Res: res, StateDays: 40})
	assert.True(t, strings.HasPrefix(msg, "[P1] 🔽 状态回落 WATCH → NORMAL"))
	assert.NotContains(t, msg, "状态解除")

	// 含 ⚪ 但无 🔴🟡（异常区仍为空）→ ✅（⚪ 不算异常区）
	res2 := dayResult(StateBrewing, StateWatch)
	r2 := res2.Results[IndMOVE]
	r2.Status = StatusStale
	res2.Results[IndMOVE] = r2
	msg = renderTransition(cfg, NotifyContext{Res: res2, StateDays: 34})
	assert.True(t, strings.HasPrefix(msg, "[P1] ✅ 状态解除 BREWING → WATCH"))

	// 升级路径不受影响
	msg = renderTransition(cfg, NotifyContext{Res: dayResult(StateWatch, StateBrewing), StateDays: 12})
	assert.True(t, strings.HasPrefix(msg, "[P0] 🚨 状态升级 WATCH → BREWING"))
}

// R1a（设计 v1.1）：降级当日 NewStale 且断更前为 RED/AMBER → 尾注前插警示行。
// 三条件独立否定 + 多指标 AllIndicators 序 + 断更前恰为 AMBER 落界。
func TestRenderTransitionStaleWarning(t *testing.T) {
	cfg := testConfig()
	downgrade := func(newStale []string, prevDay map[string]Evaluation) string {
		return renderTransition(cfg, NotifyContext{
			Res: dayResult(StateBrewing, StateWatch), StateDays: 34,
			NewStale: newStale, PrevDay: prevDay,
		})
	}

	// 断更前 RED → 警示行出现，且在尾注之前
	msg := downgrade([]string{IndHYOAS},
		map[string]Evaluation{IndHYOAS: {Indicator: IndHYOAS, Status: StatusRed}})
	assert.Contains(t, msg, "⚠ 注意：本次变更当日 hy_oas 数据断更（断更前为红），触发条件可能被动解除而非真实缓解，请人工核实。")
	assert.Less(t, strings.Index(msg, "⚠ 注意"), strings.Index(msg, "共持续"))

	// 断更前恰为 AMBER（落界）→ 出现
	msg = downgrade([]string{IndHYOAS},
		map[string]Evaluation{IndHYOAS: {Indicator: IndHYOAS, Status: StatusAmber}})
	assert.Contains(t, msg, "（断更前为黄）")

	// 否定 1：NewStale 为空 → 无警示
	msg = downgrade(nil, map[string]Evaluation{IndHYOAS: {Indicator: IndHYOAS, Status: StatusRed}})
	assert.NotContains(t, msg, "⚠ 注意")

	// 否定 2：断更前为绿 → 无警示
	msg = downgrade([]string{IndHYOAS},
		map[string]Evaluation{IndHYOAS: {Indicator: IndHYOAS, Status: StatusGreen}})
	assert.NotContains(t, msg, "⚠ 注意")

	// 否定 3：升级路径 → 无警示（即使 NewStale+RED）
	msg = renderTransition(cfg, NotifyContext{
		Res: dayResult(StateWatch, StateBrewing), StateDays: 12,
		NewStale: []string{IndHYOAS},
		PrevDay:  map[string]Evaluation{IndHYOAS: {Indicator: IndHYOAS, Status: StatusRed}},
	})
	assert.NotContains(t, msg, "⚠ 注意")

	// 多指标：AllIndicators 序（vix 先于 hy_oas），颜色同序对应
	msg = downgrade([]string{IndHYOAS, IndVIX}, map[string]Evaluation{
		IndHYOAS: {Indicator: IndHYOAS, Status: StatusAmber},
		IndVIX:   {Indicator: IndVIX, Status: StatusRed},
	})
	assert.Contains(t, msg, "vix、hy_oas 数据断更（断更前为红、黄）")
}

// N1（反审补充）：最坏组合降级消息长度 ≤4096。7 指标全 NewStale 且断更前全 RED
// → 警示行 7 名 + 7 行 ⚪ 正文 + 语义句 + 页脚，是长度上界（全 STALE 时异常区空、
// 走 ✅，但警示行/正文最长）；附一个异常区非空(🔽) + 多 NewStale 变体亦 ≤4096。
func TestRenderTransitionStaleWarningWithinLimit(t *testing.T) {
	cfg := testConfig()
	redPrev := map[string]Evaluation{}
	for _, ind := range AllIndicators {
		redPrev[ind] = Evaluation{Indicator: ind, Status: StatusRed}
	}
	// (a) 7 全 NewStale：最长警示行 + 全 ⚪ 正文
	res := dayResult(StateBrewing, StateWatch)
	for _, ind := range AllIndicators {
		r := res.Results[ind]
		r.Status = StatusStale
		res.Results[ind] = r
	}
	msg := renderTransition(cfg, NotifyContext{Res: res, StateDays: 34, NewStale: AllIndicators, PrevDay: redPrev})
	assert.Contains(t, msg, "⚠ 注意")
	assert.LessOrEqual(t, len(msg), 4096)

	// (b) 异常区非空(🔽)：1 指标今日 RED + 其余 6 NewStale（断更前 RED）
	res2 := dayResult(StateBrewing, StateWatch)
	rr := res2.Results[IndSOFREFFR]
	rr.Status, rr.Value = StatusRed, 30
	res2.Results[IndSOFREFFR] = rr
	var six []string
	for _, ind := range AllIndicators {
		if ind == IndSOFREFFR {
			continue
		}
		r := res2.Results[ind]
		r.Status = StatusStale
		res2.Results[ind] = r
		six = append(six, ind)
	}
	msg = renderTransition(cfg, NotifyContext{Res: res2, StateDays: 34, NewStale: six, PrevDay: redPrev})
	assert.True(t, strings.HasPrefix(msg, "[P1] 🔽 状态回落"))
	assert.LessOrEqual(t, len(msg), 4096)
}

// colorWord 四分支（绿/黄/红 + 非色彩统一"白"）。
func TestColorWord(t *testing.T) {
	assert.Equal(t, "绿", colorWord(StatusGreen))
	assert.Equal(t, "黄", colorWord(StatusAmber))
	assert.Equal(t, "红", colorWord(StatusRed))
	assert.Equal(t, "白", colorWord(StatusStale))
	assert.Equal(t, "白", colorWord(StatusNoData))
	assert.Equal(t, "白", colorWord(StatusSuppressed))
}

// diffLine 多级判据（通知设计 §6.5）——每级单独锁：
// 1) 状态迁移优先于读数变化（既变色又变值 → 只出迁移句，互斥）
// 2) 读数变化仅列当日异常区指标（非异常区变值 → 不出现）
// 3) 顺序按 AllIndicators；4) 缺 PrevDay 行不参与；5) ⚪ 迁移 → "转白"
func TestDiffLineLevels(t *testing.T) {
	res := dayResult(StateBrewing, StateBrewing) // 全绿 Value=1
	set := func(ind string, r IndicatorResult) { res.Results[ind] = r }
	set(IndHYOAS, IndicatorResult{Indicator: IndHYOAS, Status: StatusRed, Value: 618})      // 异常，且变色+变值
	set(IndSOFREFFR, IndicatorResult{Indicator: IndSOFREFFR, Status: StatusRed, Value: 30}) // 异常，但缺 PrevDay
	set(IndMOVE, IndicatorResult{Indicator: IndMOVE, Status: StatusStale, Value: 1})        // ⚪ 迁移

	nc := NotifyContext{Res: res, PrevDay: map[string]Evaluation{
		IndHYOAS: {Indicator: IndHYOAS, Status: StatusAmber, Value: 500}, // 变色(黄→红)+变值(+118)
		IndVIX:   {Indicator: IndVIX, Status: StatusGreen, Value: 5},     // 非异常，仅变值(-4) → 不出现
		IndMOVE:  {Indicator: IndMOVE, Status: StatusGreen, Value: 1},    // 绿→STALE → 转白
	}}
	line := diffLine(nc)
	// 顺序按 AllIndicators：move(idx1) 先于 hy_oas(idx3)；迁移句互斥（hy_oas 无 +118bp）
	assert.Equal(t, "较昨日：move 转白（原绿） · hy_oas 转红（原黄）", line)
	assert.NotContains(t, line, "+118bp") // 迁移优先：读数变化被抑制（互斥）
	assert.NotContains(t, line, "vix")    // 非异常区变值不出现
	assert.NotContains(t, line, "sofr")   // 缺 PrevDay 不参与
}

// 日报（§5.3）：首行第 N 日、异常指标区、较昨日差异行、盘中提示尾注。
func TestRenderDaily(t *testing.T) {
	cfg := testConfig()
	res := dayResult(StateBrewing, StateBrewing)
	res.Date = "2026-07-18"
	set := func(ind string, r IndicatorResult) { res.Results[ind] = r }
	set(IndHYOAS, IndicatorResult{Indicator: IndHYOAS, Status: StatusRed, Value: 618, Pct5y: 0.98, Tag: TagStress})
	set(IndUSDJPY, IndicatorResult{Indicator: IndUSDJPY, Status: StatusAmber, Value: 158.9, Wow: -0.021, WowOK: true})

	nc := NotifyContext{Res: res, StateDays: 5, PrevDay: map[string]Evaluation{
		IndHYOAS:  {Indicator: IndHYOAS, Status: StatusRed, Value: 612},
		IndUSDJPY: {Indicator: IndUSDJPY, Status: StatusGreen, Value: 160.1},
		IndVIX:    {Indicator: IndVIX, Status: StatusGreen, Value: 1}, // 读数不变且非异常 → 不出现
	}}
	msg := renderDaily(cfg, nc)
	assert.True(t, strings.HasPrefix(msg, "[P1] 📍 BREWING 日报 第 5 日 · 07-18\n\n异常指标：\n🔴 信用 hy_oas 618bp"))
	// 状态迁移优先 + 读数变化仅列异常区指标（§6.5）；顺序按 AllIndicators
	assert.Contains(t, msg, "较昨日：hy_oas +6bp · usdjpy 转黄（原绿）")
	assert.NotContains(t, msg, "usdjpy -1.2") // usdjpy 既变色又变值 → 只出迁移句（互斥）
	assert.Contains(t, msg, "盘中 JPY 监测运行中（每 30 分钟）· 下一评估：下一交易日")
	assert.True(t, strings.HasSuffix(msg, notifyFooter))

	// 完全无变化 → "较昨日：无变化"（boundary[0]）
	res2 := dayResult(StateCrisis, StateCrisis)
	nc2 := NotifyContext{Res: res2, StateDays: 2, PrevDay: map[string]Evaluation{
		IndVIX: {Indicator: IndVIX, Status: StatusGreen, Value: 1},
	}}
	msg = renderDaily(cfg, nc2)
	assert.True(t, strings.HasPrefix(msg, "[P1] 📍 CRISIS 日报 第 2 日"))
	assert.Contains(t, msg, "较昨日：无变化")
}

// 周报（§5.5）：首行当周、退出进度（§6.6）、下次周报尾注。
func TestRenderWeekly(t *testing.T) {
	cfg := testConfig()
	res := dayResult(StateWatch, StateWatch)
	res.Date = "2026-07-20"
	msg := renderWeekly(cfg, NotifyContext{Res: res, StateDays: 18, ClearStreak: 8})
	assert.True(t, strings.HasPrefix(msg, "[P1] 📅 Cassandra 周报 · 07-20 当周 · WATCH 已持续 18 个评估日"))
	assert.Contains(t, msg, "7 指标全绿：")
	assert.Contains(t, msg, "退出进度：触发条件已连续解除 8 日（回 NORMAL 需连续 20 日）")
	assert.Contains(t, msg, "下次周报：下周一 · 状态变更即时通知")
	assert.True(t, strings.HasSuffix(msg, notifyFooter))

	// WatchExitDays 注入锁（testConfig 恰为 20，须异值断言防硬编码；同 T5 注入锁法）
	cfg.StateMachine.WatchExitDays = 25
	assert.Contains(t, renderWeekly(cfg, NotifyContext{Res: res, StateDays: 18, ClearStreak: 8}), "回 NORMAL 需连续 25 日")
}

// testTrends 为 dayResult 的 7 指标各造一段 21 观测趋势窗口。
func testTrends(end string) map[string]Trend {
	out := map[string]Trend{}
	for _, ind := range AllIndicators {
		win := seriesEnding(end, 21, 10, 12)
		out[ind] = Trend{Window: win, Delta: win[len(win)-1].Value - win[0].Value}
	}
	return out
}

// nextMonthlyDue：正常解析 → "{下月} 月首个交易日"；不可解析 → 降级 "下月首个交易日"。
func TestNextMonthlyDue(t *testing.T) {
	assert.Equal(t, "9 月首个交易日", nextMonthlyDue("2026-08-03"))
	assert.Equal(t, "1 月首个交易日", nextMonthlyDue("2026-12-15")) // 跨年：12 月 +1 → 1 月
	assert.Equal(t, "下月首个交易日", nextMonthlyDue("bad-date"))    // boundary[1]：不可解析降级
	assert.Equal(t, "下月首个交易日", nextMonthlyDue(""))
}

// 月报（§5.4）：单一趋势区（无异常/正常分区）、sparkline+月变化并列、
// AMBER 计数尾注、下次月报。
func TestRenderMonthly(t *testing.T) {
	cfg := testConfig()
	res := dayResult(StateNormal, StateNormal)
	res.Date = "2026-08-03"
	res.Detail = SysDetail{AmberCount: 2}
	r := res.Results[IndHYOAS]
	r.Status, r.Tag, r.Value, r.Pct5y = StatusAmber, TagComplacency, 267, 0.03
	res.Results[IndHYOAS] = r
	// ⚪ 指标进趋势区 → 趋势行带非色彩说明（trendLine 的 nonColorNote 分支）
	mv := res.Results[IndMOVE]
	mv.Status, mv.Value = StatusStale, 88.1
	res.Results[IndMOVE] = mv

	nc := NotifyContext{Res: res, StateDays: 63, SummaryDue: true, Trends: testTrends(res.Date)}
	msg := renderMonthly(cfg, nc)
	assert.Contains(t, msg, "⚪ 情绪 move 88.1 ") // ⚪ 趋势行
	assert.Contains(t, msg, "· 数据断更(STALE)")   // nonColorNote 分支
	assert.True(t, strings.HasPrefix(msg, "[P1] 📅 Cassandra 月报 · 2026-08 · NORMAL 已持续 63 个评估日\n\n近 21 个交易日趋势（走势 · 月变化 · 5y分位）：\n"))
	assert.Contains(t, msg, "🟢 情绪 vix 1.0 ")
	assert.Contains(t, msg, "↗+2.0 · 50%")
	assert.Contains(t, msg, "🟡 信用 hy_oas 267bp ")
	assert.Contains(t, msg, "↗+2bp · 3% · 自满(COMPLACENCY)")
	assert.NotContains(t, msg, "异常指标：") // 月报特例：不分区（设计 §4）
	assert.NotContains(t, msg, "其余指标：")
	assert.Contains(t, msg, "AMBER 计数 2（触发 WATCH 需 ≥3）· 下次月报：9 月首个交易日")
	assert.True(t, strings.HasSuffix(msg, notifyFooter))

	// watch_amber_count 注入锁（testConfig=3，异值断言防硬编码）
	cfg.StateMachine.WatchAmberCount = 5
	assert.Contains(t, renderMonthly(cfg, nc), "触发 WATCH 需 ≥5）")
	cfg.StateMachine.WatchAmberCount = 3 // 复原

	// boundary[0]「趋势窗口缺失或为空」是两分支，各一用例：
	// (1) 缺失（!ok）：Trends 无该键
	delete(nc.Trends, IndMOVE)
	assert.NotContains(t, renderMonthly(cfg, nc), "move")
	// (2) 为空（len(Window)==0）：键在但窗口空
	nc.Trends[IndT10Y2Y] = Trend{Window: nil}
	assert.NotContains(t, renderMonthly(cfg, nc), "t10y2y")

	// 月报日期不可解析 → 尾注降级 "下月首个交易日"（boundary[1] 后半）
	res.Date = "bad-date"
	assert.Contains(t, renderMonthly(cfg, NotifyContext{Res: res, StateDays: 1, Trends: testTrends("2026-08-03")}), "下次月报：下月首个交易日")
}

// P2 运维速报（§5.6）：两行、无页脚、滞后与通道名。
func TestRenderOpsAlert(t *testing.T) {
	cfg := testConfig()
	res := dayResult(StateNormal, StateNormal)
	res.Date = "2026-07-14"
	nc := NotifyContext{Res: res, NewStale: []string{IndMOVE},
		StaleLastObs: map[string]string{IndMOVE: "2026-07-09"}}

	msg := renderOpsAlert(cfg, nc, IndMOVE)
	assert.Equal(t, "[P2] 🔧 move 数据源断更 · 07-14\n最后观测 07-09（滞后 5 日 > 阈值 4 日），已标记 STALE 退出共振计数；恢复后自动回归。持续超一周需检查 Yahoo 通道。", msg)
	assert.NotContains(t, msg, "非交易信号") // 速报无页脚（设计 §2）

	// nfci 用周频阈值 + FRED 通道（特例分支）
	nc.StaleLastObs[IndNFCI] = "2026-06-30"
	msg = renderOpsAlert(cfg, nc, IndNFCI)
	assert.Contains(t, msg, "滞后 14 日 > 阈值 12 日")
	assert.Contains(t, msg, "FRED 通道")

	// 通道映射非示例指标：usdjpy→Yahoo、vix→FRED（补充决策 1）
	nc.StaleLastObs[IndUSDJPY] = "2026-07-08"
	assert.Contains(t, renderOpsAlert(cfg, nc, IndUSDJPY), "Yahoo 通道")
	// 最后观测日缺失 → 降级文案（vix 不在 StaleLastObs），且 vix→FRED
	msg = renderOpsAlert(cfg, nc, IndVIX)
	assert.Contains(t, msg, "无历史观测")
	assert.Contains(t, msg, "FRED 通道")
}

// P2 阈值注入锁：daily/weekly max_lag_days 均须异值断言（testConfig 4/12）。
func TestOpsAlertLagInjection(t *testing.T) {
	cfg := testConfig()
	cfg.Freshness.DailyMaxLagDays = 3
	cfg.Freshness.WeeklyMaxLagDays = 10
	res := dayResult(StateNormal, StateNormal)
	res.Date = "2026-07-14"
	nc := NotifyContext{Res: res, StaleLastObs: map[string]string{
		IndMOVE: "2026-07-09", IndNFCI: "2026-06-30"}}
	assert.Contains(t, renderOpsAlert(cfg, nc, IndMOVE), "阈值 3 日")  // daily 注入
	assert.Contains(t, renderOpsAlert(cfg, nc, IndNFCI), "阈值 10 日") // weekly 注入
}
