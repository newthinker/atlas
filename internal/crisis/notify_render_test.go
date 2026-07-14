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
