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
	set(IndHYOAS, IndicatorResult{Indicator: IndHYOAS, Status: StatusRed, Value: 618})   // 异常，且变色+变值
	set(IndSOFREFFR, IndicatorResult{Indicator: IndSOFREFFR, Status: StatusRed, Value: 30}) // 异常，但缺 PrevDay
	set(IndMOVE, IndicatorResult{Indicator: IndMOVE, Status: StatusStale, Value: 1})       // ⚪ 迁移

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
