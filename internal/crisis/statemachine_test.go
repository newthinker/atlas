package crisis

// Context Checkpoint: done_criteria → test mapping (statemachine)
// functional[0] 情绪双红 → CRISIS（任意态）→ TestNextStateTransitions
// functional[1] NORMAL→WATCH：领先层 RED 或 AMBER-及以上计数≥阈（RED 计入）→ TestNextStateTransitions
// functional[2] WATCH→BREWING：hy_oas RED ∧ sofr_effr RED → TestNextStateTransitions
// functional[3] 退出转移连续日重建（CRISIS/WATCH/BREWING 各够天数 vs 差天数）→ TestNextStateExits
// functional[4] MemHistory Append 后新→旧 → TestMemHistoryOrder
// boundary[0]   历史不足→不退出（冷启动）；NO_DATA/SUPPRESSED 退出共振 → TestNextStateExits / TestNextStateTransitions
// error_handling[0] 系统行 detail 不可解析→保守不退出且不报错 → TestNextStateMalformedDetailConservative

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var assertErr = errors.New("history read failed")

// colorResults 构造 7 指标结果：未指定的默认 GREEN。
func colorResults(m map[string]Status) map[string]IndicatorResult {
	out := map[string]IndicatorResult{}
	for _, ind := range AllIndicators {
		s, ok := m[ind]
		if !ok {
			s = StatusGreen
		}
		out[ind] = IndicatorResult{Indicator: ind, Status: s, RawStatus: s}
	}
	return out
}

// histWithSystem 预填 days 条 detail 相同的系统评估行。
func histWithSystem(days int, det SysDetail) *MemHistory {
	h := NewMemHistory()
	b, _ := json.Marshal(det)
	for i := 0; i < days; i++ {
		h.Append([]Evaluation{{Indicator: "", SystemState: det.Prev, Detail: string(b)}})
	}
	return h
}

func TestNextStateTransitions(t *testing.T) {
	cfg := testConfig()
	cases := []struct {
		name string
		prev SystemState
		res  map[string]Status
		want SystemState
	}{
		{"normal stays normal", StateNormal, nil, StateNormal},
		{"leading red → WATCH", StateNormal, map[string]Status{IndNFCI: StatusRed}, StateWatch},
		{"amber-or-worse ≥3 → WATCH", StateNormal,
			map[string]Status{IndVIX: StatusAmber, IndHYOAS: StatusAmber, IndUSDJPY: StatusAmber}, StateWatch},
		{"amber count RED 计入（2 amber+1 red）→ WATCH", StateNormal,
			map[string]Status{IndVIX: StatusAmber, IndMOVE: StatusAmber, IndHYOAS: StatusRed}, StateWatch},
		{"NO_DATA 退出共振（计数只剩 2）", StateNormal,
			map[string]Status{IndVIX: StatusAmber, IndHYOAS: StatusAmber, IndUSDJPY: StatusNoData}, StateNormal},
		{"SUPPRESSED 退出共振", StateNormal,
			map[string]Status{IndVIX: StatusAmber, IndHYOAS: StatusAmber, IndSOFREFFR: StatusSuppressed}, StateNormal},
		{"watch + credit∧liquidity 双红 → BREWING", StateWatch,
			map[string]Status{IndHYOAS: StatusRed, IndSOFREFFR: StatusRed}, StateBrewing},
		{"normal + pair 不直接 BREWING（设计 §3.3 原文只从 WATCH 转入）", StateNormal,
			map[string]Status{IndHYOAS: StatusRed, IndSOFREFFR: StatusRed}, StateNormal},
		{"情绪双红从 NORMAL → CRISIS", StateNormal,
			map[string]Status{IndVIX: StatusRed, IndMOVE: StatusRed}, StateCrisis},
		{"情绪双红从 BREWING → CRISIS", StateBrewing,
			map[string]Status{IndVIX: StatusRed, IndMOVE: StatusRed}, StateCrisis},
		{"MOVE STALE 时单 VIX 红不触发 CRISIS", StateWatch,
			map[string]Status{IndVIX: StatusRed, IndMOVE: StatusStale}, StateWatch},
		{"brewing + pair 仍双红 → 维持 BREWING", StateBrewing,
			map[string]Status{IndHYOAS: StatusRed, IndSOFREFFR: StatusRed}, StateBrewing},
		{"crisis 今日未双绿（非双红）→ 维持 CRISIS", StateCrisis,
			map[string]Status{IndVIX: StatusAmber}, StateCrisis},
		{"watch 今日仍触发（leading red）→ 维持 WATCH", StateWatch,
			map[string]Status{IndNFCI: StatusRed}, StateWatch},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			next, det, err := NextState(cfg, tc.prev, colorResults(tc.res), NewMemHistory())
			require.NoError(t, err)
			assert.Equal(t, tc.want, next)
			assert.Equal(t, tc.prev, det.Prev)
		})
	}
}

func TestNextStateExits(t *testing.T) {
	cfg := testConfig()
	greens := colorResults(nil)

	// CRISIS 退出：今日双绿 + 9 日历史双绿 = 持续 10 日 → WATCH
	h := NewMemHistory()
	for i := 0; i < 9; i++ {
		h.Append([]Evaluation{
			{Indicator: IndVIX, Status: StatusGreen},
			{Indicator: IndMOVE, Status: StatusGreen},
		})
	}
	next, _, err := NextState(cfg, StateCrisis, greens, h)
	require.NoError(t, err)
	assert.Equal(t, StateWatch, next)
	// 历史不足 → 维持 CRISIS（冷启动安全）
	next, _, err = NextState(cfg, StateCrisis, greens, NewMemHistory())
	require.NoError(t, err)
	assert.Equal(t, StateCrisis, next)
	// 历史中夹一日 AMBER → 维持
	h.Append([]Evaluation{{Indicator: IndVIX, Status: StatusAmber}, {Indicator: IndMOVE, Status: StatusGreen}})
	next, _, err = NextState(cfg, StateCrisis, greens, h)
	require.NoError(t, err)
	assert.Equal(t, StateCrisis, next)

	// WATCH 退出：今日无触发 + 19 日 any_trigger=false = 持续 20 日 → NORMAL
	next, _, err = NextState(cfg, StateWatch, greens, histWithSystem(19, SysDetail{AnyTrigger: false, Prev: StateWatch}))
	require.NoError(t, err)
	assert.Equal(t, StateNormal, next)
	next, _, err = NextState(cfg, StateWatch, greens, histWithSystem(5, SysDetail{AnyTrigger: false, Prev: StateWatch}))
	require.NoError(t, err)
	assert.Equal(t, StateWatch, next)

	// BREWING 退出：今日非双红 + 9 日 brewing_pair=false = 持续 10 日 → WATCH
	next, _, err = NextState(cfg, StateBrewing, greens, histWithSystem(9, SysDetail{BrewingPair: false, Prev: StateBrewing}))
	require.NoError(t, err)
	assert.Equal(t, StateWatch, next)
	next, _, err = NextState(cfg, StateBrewing, greens, histWithSystem(3, SysDetail{BrewingPair: false, Prev: StateBrewing}))
	require.NoError(t, err)
	assert.Equal(t, StateBrewing, next)

	// WATCH 维持：历史够长但其中一日 any_trigger=true（未全程解除）→ 不退出（pred false 半边）
	mixed := NewMemHistory()
	goodDet, _ := json.Marshal(SysDetail{AnyTrigger: false, Prev: StateWatch})
	trigDet, _ := json.Marshal(SysDetail{AnyTrigger: true, Prev: StateWatch})
	for i := 0; i < 18; i++ {
		mixed.Append([]Evaluation{{Indicator: "", SystemState: StateWatch, Detail: string(goodDet)}})
	}
	mixed.Append([]Evaluation{{Indicator: "", SystemState: StateWatch, Detail: string(trigDet)}})
	next, _, err = NextState(cfg, StateWatch, greens, mixed)
	require.NoError(t, err)
	assert.Equal(t, StateWatch, next)
}

// TestNextStateMalformedDetailConservative 覆盖 error_handling[0]：历史系统行
// detail JSON 不可解析时按保守处理——不满足退出条件（维持当前态）且不向上报错。
// 判别性：18 条正常 false 行 + 1 条 malformed = 19 条（=watch_exit_days-1），
// 若坏行被误当作满足退出会转 NORMAL，保守处理则维持 WATCH。
func TestNextStateMalformedDetailConservative(t *testing.T) {
	cfg := testConfig()
	greens := colorResults(nil)
	good, _ := json.Marshal(SysDetail{AnyTrigger: false, Prev: StateWatch})

	h := NewMemHistory()
	for i := 0; i < 18; i++ {
		h.Append([]Evaluation{{Indicator: "", SystemState: StateWatch, Detail: string(good)}})
	}
	h.Append([]Evaluation{{Indicator: "", SystemState: StateWatch, Detail: "{not valid json"}})

	next, _, err := NextState(cfg, StateWatch, greens, h)
	require.NoError(t, err)
	assert.Equal(t, StateWatch, next)
}

// errHistory 是恒报错的 EvalHistory，用于验证接口级 IO 错误被透传（与 malformed
// detail 的保守吞错相对：坏数据行保守不退出，底层读失败则上抛）。
type errHistory struct{ err error }

func (e errHistory) RecentSystem(int) ([]Evaluation, error)            { return nil, e.err }
func (e errHistory) RecentIndicator(string, int) ([]Evaluation, error) { return nil, e.err }

func TestNextStatePropagatesHistoryError(t *testing.T) {
	cfg := testConfig()
	greens := colorResults(nil)
	boom := errHistory{err: assertErr}

	// CRISIS 今日双绿 → sentimentGreenStreak 读 RecentIndicator 失败 → 透传
	_, _, err := NextState(cfg, StateCrisis, greens, boom)
	require.ErrorIs(t, err, assertErr)
	// WATCH 今日无触发 → systemDetailStreak 读 RecentSystem 失败 → 透传
	_, _, err = NextState(cfg, StateWatch, greens, boom)
	require.ErrorIs(t, err, assertErr)
	// BREWING 今日非双红 → systemDetailStreak 读 RecentSystem 失败 → 透传
	_, _, err = NextState(cfg, StateBrewing, greens, boom)
	require.ErrorIs(t, err, assertErr)
}

func TestMemHistoryOrder(t *testing.T) {
	h := NewMemHistory()
	h.Append([]Evaluation{{Indicator: "", TS: "2026-07-01"}, {Indicator: IndVIX, TS: "2026-07-01"}})
	h.Append([]Evaluation{{Indicator: "", TS: "2026-07-02"}, {Indicator: IndVIX, TS: "2026-07-02"}})

	sys, err := h.RecentSystem(5)
	require.NoError(t, err)
	require.Len(t, sys, 2)
	assert.Equal(t, "2026-07-02", sys[0].TS) // 新→旧

	ind, err := h.RecentIndicator(IndVIX, 1)
	require.NoError(t, err)
	require.Len(t, ind, 1)
	assert.Equal(t, "2026-07-02", ind[0].TS)
}

// QA CONTESTED 裁决回归：退出冷却必须在态内累积——危机康复尾段的免触发
// CRISIS 行不得计入 WATCH→NORMAL 的 20 日观察期（BREWING 同理）。
func TestExitStreakRequiresInStateHistory(t *testing.T) {
	cfg := testConfig()
	quiet := colorResults(nil) // 全绿：今日无任何触发

	// 19 条 any_trigger=false 历史行，但其中仅 1 条是 WATCH 态，其余是
	// CRISIS 康复尾段——修复前会被误计满而 T+1 直降 NORMAL。
	mixed := NewMemHistory()
	b, _ := json.Marshal(SysDetail{AnyTrigger: false, Prev: StateCrisis})
	for i := 0; i < 18; i++ {
		mixed.Append([]Evaluation{{Indicator: "", SystemState: StateCrisis, Detail: string(b)}})
	}
	bw, _ := json.Marshal(SysDetail{AnyTrigger: false, Prev: StateWatch})
	mixed.Append([]Evaluation{{Indicator: "", SystemState: StateWatch, Detail: string(bw)}})

	next, _, err := NextState(cfg, StateWatch, quiet, mixed)
	require.NoError(t, err)
	assert.Equal(t, StateWatch, next, "异态历史行不得计入 WATCH 退出冷却")

	// 对照半：同样 19 条全为 WATCH 态 → 正常放行 NORMAL。
	next, _, err = NextState(cfg, StateWatch, quiet, histWithSystem(19, SysDetail{AnyTrigger: false, Prev: StateWatch}))
	require.NoError(t, err)
	assert.Equal(t, StateNormal, next)

	// BREWING 同理：9 条 pair=false 但混入 WATCH 态行 → 维持 BREWING。
	mixedB := NewMemHistory()
	bb, _ := json.Marshal(SysDetail{BrewingPair: false, Prev: StateWatch})
	for i := 0; i < 9; i++ {
		st := StateBrewing
		if i == 4 {
			st = StateWatch
		}
		mixedB.Append([]Evaluation{{Indicator: "", SystemState: st, Detail: string(bb)}})
	}
	next, _, err = NextState(cfg, StateBrewing, quiet, mixedB)
	require.NoError(t, err)
	assert.Equal(t, StateBrewing, next, "异态历史行不得计入 BREWING 退出冷却")
}

func clearStreakEval(date string, anyTrigger bool) Evaluation {
	d, _ := json.Marshal(SysDetail{Date: date, AnyTrigger: anyTrigger, Prev: StateWatch})
	return Evaluation{TS: date, Indicator: "", SystemState: StateWatch, Detail: string(d)}
}

// ClearStreakDays：state 态内、any_trigger=false 的连续历史日数（周报退出进度，
// 设计 §6.6）。clearStreakEval 造的都是 WATCH 态行，故既有子例传 StateWatch。
func TestClearStreakDays(t *testing.T) {
	h := NewMemHistory()
	h.Append([]Evaluation{clearStreakEval("2026-07-06", true)})
	h.Append([]Evaluation{clearStreakEval("2026-07-07", false)})
	h.Append([]Evaluation{clearStreakEval("2026-07-08", false)})
	n, err := ClearStreakDays(h, StateWatch, 20)
	require.NoError(t, err)
	assert.Equal(t, 2, n) // 最新两行 false，第三行 true 中断

	// 空历史 → 0
	n, err = ClearStreakDays(NewMemHistory(), StateWatch, 20)
	require.NoError(t, err)
	assert.Equal(t, 0, n)

	// 坏 detail 行 → 中断计数而非上抛（同 systemDetailStreak 的保守约定）
	h2 := NewMemHistory()
	h2.Append([]Evaluation{{TS: "2026-07-07", Indicator: "", SystemState: StateWatch, Detail: "not-json"}})
	h2.Append([]Evaluation{clearStreakEval("2026-07-08", false)})
	n, err = ClearStreakDays(h2, StateWatch, 20)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	// RecentSystem 返回错误 → 原样上抛（error_handling[1]）
	_, err = ClearStreakDays(errHistory{err: assertErr}, StateWatch, 20)
	assert.ErrorIs(t, err, assertErr)

	// max 封顶回看深度：3 行全 false，max=2 只数 2（boundary[1]）
	h3 := NewMemHistory()
	h3.Append([]Evaluation{clearStreakEval("2026-07-06", false)})
	h3.Append([]Evaluation{clearStreakEval("2026-07-07", false)})
	h3.Append([]Evaluation{clearStreakEval("2026-07-08", false)})
	n, err = ClearStreakDays(h3, StateWatch, 2)
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	// 态内计数（QA CRITICAL 修复）：CRISIS 康复尾段 18 行 false + 今日进 WATCH 的 1 行
	// false，喂 StateWatch 只应数 WATCH 那 1 行——异态历史行不得污染 WATCH 退出进度。
	// （RecentSystem 新→旧：WATCH 在最前，遇 CRISIS 态即 break）
	mixed := NewMemHistory()
	cb, _ := json.Marshal(SysDetail{AnyTrigger: false, Prev: StateCrisis})
	for i := 0; i < 18; i++ {
		mixed.Append([]Evaluation{{Indicator: "", SystemState: StateCrisis, Detail: string(cb)}})
	}
	mixed.Append([]Evaluation{clearStreakEval("2026-07-09", false)}) // WATCH 态，最新
	n, err = ClearStreakDays(mixed, StateWatch, 20)
	require.NoError(t, err)
	assert.Equal(t, 1, n) // 修复前会数满 19
}
