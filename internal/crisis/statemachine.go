package crisis

import "encoding/json"

// SysDetail is the JSON payload of system-level evaluation rows. The
// consecutive-day exit counters are rebuilt from these historical flags, so
// the process holds no in-memory counters (design §4.3: stateless process,
// sqlite as the single source of truth).
type SysDetail struct {
	Date        string      `json:"date"`
	AnyTrigger  bool        `json:"any_trigger"`
	BrewingPair bool        `json:"brewing_pair"`
	AmberCount  int         `json:"amber_count"`
	Prev        SystemState `json:"prev"`
}

// coloredStatus returns the resonance-eligible status of ind; non-color
// statuses (NO_DATA / STALE / SUPPRESSED_SEASONAL) leave the count entirely
// (design §3.2 rules 2 and 5 share this exit).
func coloredStatus(res map[string]IndicatorResult, ind string) Status {
	if s := res[ind].Status; isColor(s) {
		return s
	}
	return ""
}

// amberOrWorseCount counts indicators at AMBER or above — "全系统 AMBER ≥ 3"
// read as at-least-amber so reds are not perversely excluded (plan's
// state-machine semantics note).
func amberOrWorseCount(res map[string]IndicatorResult) int {
	n := 0
	for _, ind := range AllIndicators {
		if severity(coloredStatus(res, ind)) >= severity(StatusAmber) {
			n++
		}
	}
	return n
}

// NextState implements design §3.3. Precedence: the sentiment double-red
// CRISIS trigger fires from any state; all exits require the full
// consecutive-day streak and default to staying put when history is short.
func NextState(cfg *Config, prev SystemState, res map[string]IndicatorResult, hist EvalHistory) (SystemState, SysDetail, error) {
	sm := cfg.StateMachine
	sentimentDoubleRed := coloredStatus(res, IndVIX) == StatusRed && coloredStatus(res, IndMOVE) == StatusRed
	leadingRed := coloredStatus(res, IndT10Y2Y) == StatusRed || coloredStatus(res, IndNFCI) == StatusRed
	brewingPair := coloredStatus(res, IndHYOAS) == StatusRed && coloredStatus(res, IndSOFREFFR) == StatusRed
	amberCount := amberOrWorseCount(res)

	det := SysDetail{
		AnyTrigger:  leadingRed || amberCount >= sm.WatchAmberCount || brewingPair || sentimentDoubleRed,
		BrewingPair: brewingPair,
		AmberCount:  amberCount,
		Prev:        prev,
	}

	if sentimentDoubleRed {
		return StateCrisis, det, nil
	}

	switch prev {
	case StateCrisis:
		ok, err := sentimentGreenStreak(res, hist, sm.CrisisExitDays)
		if err != nil || !ok {
			return StateCrisis, det, err
		}
		return StateWatch, det, nil

	case StateBrewing:
		if brewingPair {
			return StateBrewing, det, nil
		}
		ok, err := systemDetailStreak(hist, sm.BrewingExitDays, StateBrewing, func(d SysDetail) bool { return !d.BrewingPair })
		if err != nil || !ok {
			return StateBrewing, det, err
		}
		return StateWatch, det, nil

	case StateWatch:
		if brewingPair {
			return StateBrewing, det, nil
		}
		if det.AnyTrigger {
			return StateWatch, det, nil
		}
		ok, err := systemDetailStreak(hist, sm.WatchExitDays, StateWatch, func(d SysDetail) bool { return !d.AnyTrigger })
		if err != nil || !ok {
			return StateWatch, det, err
		}
		return StateNormal, det, nil

	default: // NORMAL（含冷启动，设计 §4.3）
		if leadingRed || amberCount >= sm.WatchAmberCount {
			return StateWatch, det, nil
		}
		return StateNormal, det, nil
	}
}

// sentimentGreenStreak: 今日 vix/move 双绿，且此前 days-1 个评估日的指标行均为
// GREEN（历史不足 = 不允许退出）。
func sentimentGreenStreak(res map[string]IndicatorResult, hist EvalHistory, days int) (bool, error) {
	if coloredStatus(res, IndVIX) != StatusGreen || coloredStatus(res, IndMOVE) != StatusGreen {
		return false, nil
	}
	for _, ind := range []string{IndVIX, IndMOVE} {
		prev, err := hist.RecentIndicator(ind, days-1)
		if err != nil {
			return false, err
		}
		if len(prev) < days-1 {
			return false, nil
		}
		for _, e := range prev {
			if e.Status != StatusGreen {
				return false, nil
			}
		}
	}
	return true, nil
}

// systemDetailStreak: 此前 days-1 个系统评估行均处于 state 态且 detail 满足
// pred（今日条件由调用方先判）。异态历史行意味着进入 state 尚不足 days-1 日，
// 冷却期必须在态内重新累积——否则危机康复尾段的免触发日会把 WATCH/BREWING 的
// 观察期压缩掉（QA 裁决：设计 §3.3 的"持续 N 交易日"限定态内计数）。detail JSON
// 不可解析时按保守处理——该行视为不满足退出条件（不退出）而非上抛错误，
// 以免一条坏历史行使整日评估失败（DoD error_handling）。
func systemDetailStreak(hist EvalHistory, days int, state SystemState, pred func(SysDetail) bool) (bool, error) {
	prev, err := hist.RecentSystem(days - 1)
	if err != nil {
		return false, err
	}
	if len(prev) < days-1 {
		return false, nil
	}
	for _, e := range prev {
		if e.SystemState != state {
			return false, nil
		}
		var d SysDetail
		if err := json.Unmarshal([]byte(e.Detail), &d); err != nil {
			return false, nil
		}
		if !pred(d) {
			return false, nil
		}
	}
	return true, nil
}
