package bank

import (
	"math"
	"sort"
	"time"
)

// Stat 是某指标在统计期内的分布；N==0 时数值为 NaN、名称为空。
type Stat struct {
	N                 int
	Mean, Median      float64
	Best, Worst       string
	BestVal, WorstVal float64
}

// Summary 按统计期把主体分为四组。统计期 = 以之为 Latest 的未失败主体数最多的报告期
// （并列取较晚），避免个别银行提前披露把统计期拉到只有一家的那一期。
type Summary struct {
	Period  time.Time
	RankBy  Indicator
	Current []BankResult // Latest == Period，按 RankBy 由优到劣
	Stale   []BankResult // Latest 早于 Period，或无任何数据
	Ahead   []BankResult // Latest 晚于 Period，不进同期统计与排名
	Failed  []BankResult // 拉取失败
	Stats   [numIndicators]Stat
}

func Summarize(results []BankResult, rankBy Indicator) Summary {
	s := Summary{RankBy: rankBy, Period: modePeriod(results)}
	for _, r := range results {
		switch {
		case r.Err != nil:
			s.Failed = append(s.Failed, r)
		case r.Latest.IsZero() || r.Latest.Before(s.Period):
			s.Stale = append(s.Stale, r)
		case r.Latest.After(s.Period):
			s.Ahead = append(s.Ahead, r)
		default:
			s.Current = append(s.Current, r)
		}
	}
	sort.SliceStable(s.Current, func(i, j int) bool {
		return better(rankBy, s.value(s.Current[i], rankBy), s.value(s.Current[j], rankBy))
	})
	for _, k := range Indicators {
		s.Stats[k] = s.stat(k)
	}
	return s
}

// modePeriod 返回未失败且有数据的主体中出现最多的 Latest，并列取较晚；无则零值。
func modePeriod(results []BankResult) time.Time {
	count := map[time.Time]int{}
	var best time.Time
	for _, r := range results {
		if r.Err != nil || r.Latest.IsZero() {
			continue
		}
		count[r.Latest]++
		if c, bc := count[r.Latest], count[best]; c > bc || (c == bc && r.Latest.After(best)) {
			best = r.Latest
		}
	}
	return best
}

// value 返回 r 的指标 k 在统计期的值；回退到更早报告期的值视同 NaN。
func (s Summary) value(r BankResult, k Indicator) float64 {
	if !r.Ind[k].Period.Equal(s.Period) {
		return math.NaN()
	}
	return r.Ind[k].Value
}

// better 报告 a 是否严格优于 b；NaN 视为最差。
func better(k Indicator, a, b float64) bool {
	switch {
	case math.IsNaN(a):
		return false
	case math.IsNaN(b):
		return true
	case k.HigherIsWorse():
		return a < b
	default:
		return a > b
	}
}

func (s Summary) stat(k Indicator) Stat {
	st := Stat{Mean: math.NaN(), Median: math.NaN(), BestVal: math.NaN(), WorstVal: math.NaN()}
	var vals []float64
	for _, r := range s.Current {
		v := s.value(r, k)
		if math.IsNaN(v) {
			continue
		}
		vals = append(vals, v)
		if st.Best == "" || better(k, v, st.BestVal) {
			st.Best, st.BestVal = r.Name, v
		}
		if st.Worst == "" || better(k, st.WorstVal, v) {
			st.Worst, st.WorstVal = r.Name, v
		}
	}
	st.N = len(vals)
	if st.N == 0 {
		return st
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	st.Mean = sum / float64(st.N)
	sort.Float64s(vals)
	m := st.N / 2
	if st.N%2 == 1 {
		st.Median = vals[m]
	} else {
		st.Median = (vals[m-1] + vals[m]) / 2
	}
	return st
}
