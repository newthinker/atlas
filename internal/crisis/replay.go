package crisis

import (
	"fmt"
	"time"
)

// ReplayDay 一个回放交易日的完整评估快照。
type ReplayDay struct {
	Date      string
	Res       *DayResult
	StateDays int // 当前状态连续评估日数（含当日；暖机期计入）
}

// ReplayRange 从库内最早 vix 观测日起暖机逐日重放（内存历史、零写入），
// 返回 [from,to] 窗口内的快照。交易日历以 vix 观测日为准（与 Store.EvalDates
// 同口径）。窗口内无交易日返回空切片，措辞由调用方决定。
func ReplayRange(cfg *Config, sr SeriesReader, from, to string) ([]ReplayDay, error) {
	if from > to {
		return nil, fmt.Errorf("from %s is after to %s", from, to)
	}
	cal, err := sr.WindowSince(IndVIX, "", to)
	if err != nil {
		return nil, err
	}
	mem := NewMemHistory()
	evalAt := time.Now()
	var out []ReplayDay
	stateDays := 0
	for _, o := range cal {
		res, err := EvalDay(cfg, o.Date, sr, mem, evalAt)
		if err != nil {
			return nil, fmt.Errorf("evaluating %s: %w", o.Date, err)
		}
		mem.Append(res.Evaluations)
		if res.Transitioned() {
			stateDays = 1
		} else {
			stateDays++
		}
		if o.Date >= from {
			out = append(out, ReplayDay{Date: o.Date, Res: res, StateDays: stateDays})
		}
	}
	return out, nil
}
