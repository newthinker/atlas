package crisis

import (
	"encoding/json"
	"time"
)

// DayResult is one day's full evaluation: per-indicator results, the state
// transition and the ready-to-persist audit rows (7 indicator rows + 1
// system row, in AllIndicators order then system last).
type DayResult struct {
	Date        string
	Results     map[string]IndicatorResult
	PrevState   SystemState
	State       SystemState
	Detail      SysDetail
	Evaluations []Evaluation
}

func (r *DayResult) Transitioned() bool { return r.State != r.PrevState }

// EvalDay runs the full pipeline for one observation date: rules → seasonal
// suppression → hysteresis → state machine → audit rows. It is pure over
// (SeriesReader, EvalHistory), so live eval and replay share it.
func EvalDay(cfg *Config, date string, sr SeriesReader, hist EvalHistory, evalAt time.Time) (*DayResult, error) {
	results := make(map[string]IndicatorResult, len(AllIndicators))
	for _, ind := range AllIndicators {
		r, err := EvaluateIndicator(cfg, ind, date, sr)
		if err != nil {
			return nil, err
		}
		switch {
		case ind == IndSOFREFFR && cfg.Indicators.SOFREFFR.SuppressQuarterEnd &&
			severity(r.RawStatus) >= severity(StatusAmber) && InQuarterEndWindow(date):
			r.Status = StatusSuppressed // 设计 §3.2 条 1：仅记录不告警、退出共振
		case isColor(r.RawStatus):
			prev, err := hist.RecentIndicator(ind, cfg.StateMachine.DemoteHysteresisDays-1)
			if err != nil {
				return nil, err
			}
			r.Status = ApplyHysteresis(r.RawStatus, prev, cfg.StateMachine.DemoteHysteresisDays)
		}
		results[ind] = r
	}

	prevState := StateNormal // 冷启动基线（设计 §4.3：backfill 后直接初始化 NORMAL）
	if sys, err := hist.RecentSystem(1); err != nil {
		return nil, err
	} else if len(sys) > 0 && sys[0].SystemState != "" {
		prevState = sys[0].SystemState
	}

	next, det, err := NextState(cfg, prevState, results, hist)
	if err != nil {
		return nil, err
	}
	det.Date = date

	res := &DayResult{Date: date, Results: results, PrevState: prevState, State: next, Detail: det}
	if res.Evaluations, err = buildEvaluations(res, evalAt); err != nil {
		return nil, err
	}
	return res, nil
}

func buildEvaluations(r *DayResult, evalAt time.Time) ([]Evaluation, error) {
	stamp := NowStamp(evalAt)
	out := make([]Evaluation, 0, len(AllIndicators)+1)
	for _, ind := range AllIndicators {
		ir := r.Results[ind]
		d, err := json.Marshal(indDetail{Raw: ir.RawStatus, WindowActualObs: ir.WindowActualObs,
			PersistDays: ir.PersistDays, Wow: ir.Wow, WowOK: ir.WowOK})
		if err != nil {
			return nil, err
		}
		out = append(out, Evaluation{
			TS: r.Date, EvalAt: stamp, Indicator: ind,
			Status: ir.Status, Tag: ir.Tag, Value: ir.Value, Pct5y: ir.Pct5y,
			Detail: string(d),
		})
	}
	d, err := json.Marshal(r.Detail)
	if err != nil {
		return nil, err
	}
	out = append(out, Evaluation{
		TS: r.Date, EvalAt: stamp, Indicator: "",
		SystemState: r.State, Detail: string(d),
	})
	return out, nil
}
