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
		{StateWatch, StateBrewing, "信用与流动性双红共振。历史样本中，此组合出现后 3–12 个月内系统性风险显著抬升（样本量小，存在失效可能）。"},
		{StateNormal, StateCrisis, "情绪层双红：危机进行中。此阶段执行预案而非预测。"},
		{StateWatch, StateCrisis, "情绪层双红：危机进行中。此阶段执行预案而非预测。"},
		{StateBrewing, StateCrisis, "情绪层双红：危机进行中。此阶段执行预案而非预测。"},
		{StateCrisis, StateWatch, "情绪层连续 10 个交易日回落至绿。危机状态解除，转入观察期。"},
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
	assert.True(t, strings.HasPrefix(msg, "[P1] ✅ 状态解除 BREWING → WATCH · 09-02"))
	assert.Contains(t, msg, "稳定 10 个交易日") // brewing_exit_days=10
	assert.Contains(t, msg, "仍异常：\n🟡 信用 hy_oas")
	assert.Contains(t, msg, "BREWING 共持续 34 个评估日 · 下一评估：下一交易日")
	assert.True(t, strings.HasSuffix(msg, notifyFooter))

	cfg.StateMachine.BrewingExitDays = 12 // YAML 调参 → 文案跟随
	assert.Contains(t, renderTransition(cfg, NotifyContext{Res: res, StateDays: 34}), "稳定 12 个交易日")

	// CRISIS→WATCH 与 WATCH→NORMAL 分别注入 crisis_exit_days / watch_exit_days
	msg = renderTransition(cfg, NotifyContext{Res: dayResult(StateCrisis, StateWatch), StateDays: 20})
	assert.Contains(t, msg, "情绪层连续 10 个交易日回落至绿")
	msg = renderTransition(cfg, NotifyContext{Res: dayResult(StateWatch, StateNormal), StateDays: 40})
	assert.Contains(t, msg, "稳定 20 个交易日。回到常态")
}
