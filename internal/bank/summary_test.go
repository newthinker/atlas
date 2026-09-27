package bank

// Context Checkpoint: done_criteria → test mapping（TASK-004，summary 部分）
// functional[1]  分组与排名方向、四组互斥且并集为全部   → TestSummarizeGroupsAndRanks, TestSummarizeRankDirection
// functional[2]  统计期取众数（D15），并列取晚，失败不计 → TestSummarizeModePeriod
// functional[3]  Stat：N/均值/中位数奇偶/最优最差方向    → TestSummarizeGroupsAndRanks, TestSummarizeMedianEven
// boundary[0]    NaN 与回退值不进统计、排最后；无当期主体 → TestSummarizeExcludesNaNAndFallback, TestSummarizeNoCurrent
// boundary[1]    并列值排名稳定                           → TestSummarizeTiesKeepConfigOrder
// non_functional vet / 全绿 / 覆盖率                      → go vet / go test -cover

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// br 构造一个三项都在 latest 当期披露的主体；NaN 指标的 Period 为零值（与 Analyze 一致）。
func br(name, latest string, npl, cov, cet1 float64) BankResult {
	r := BankResult{Name: name}
	if latest != "" {
		r.Latest = day(latest)
	}
	for k, v := range []float64{npl, cov, cet1} {
		p := r.Latest
		if math.IsNaN(v) {
			p = time.Time{}
		}
		r.Ind[k] = val(v, p)
	}
	return r
}

// withFallback 把 r 的指标 k 改成回退到更早报告期 p 的值 v。
func withFallback(r BankResult, k Indicator, v float64, p string) BankResult {
	r.Ind[k] = val(v, day(p))
	return r
}

func names(rs []BankResult) []string {
	out := []string{}
	for _, r := range rs {
		out = append(out, r.Name)
	}
	return out
}

func failedAt(name, latest string) BankResult {
	r := br(name, latest, nan, nan, nan)
	r.Err = errors.New("timeout")
	return r
}

func sample() []BankResult {
	return []BankResult{
		br("C银行", "2026-06-30", 1.62, 142, 8.9),
		br("A银行", "2026-06-30", 0.94, 385, 14.07),
		br("D银行", "2026-03-31", 1.1, 180, 9.5),
		br("G银行", "2026-09-30", 0.5, 500, 20),
		br("B银行", "2026-06-30", 1.0, 214.93, 10.04),
		{Name: "E银行", Err: errors.New("timeout")},
		br("F银行", "", nan, nan, nan),
	}
}

func TestSummarizeGroupsAndRanks(t *testing.T) {
	in := sample()
	s := Summarize(in, IndNPL)
	assert.Equal(t, day("2026-06-30"), s.Period, "统计期取众数，而非最晚的 G银行 2026-09-30")
	assert.Equal(t, IndNPL, s.RankBy)
	assert.Equal(t, []string{"A银行", "B银行", "C银行"}, names(s.Current), "不良率升序")
	assert.Equal(t, []string{"D银行", "F银行"}, names(s.Stale), "早于统计期 + 无任何数据")
	assert.Equal(t, []string{"G银行"}, names(s.Ahead), "晚于统计期单列，不进统计")
	assert.Equal(t, []string{"E银行"}, names(s.Failed))

	all := slices.Concat(names(s.Current), names(s.Stale), names(s.Ahead), names(s.Failed))
	want := names(in)
	sort.Strings(all)
	sort.Strings(want)
	assert.Equal(t, want, all, "四组互斥且并集为全部输入")

	npl := s.Stats[IndNPL]
	assert.Equal(t, 3, npl.N)
	assert.InDelta(t, (0.94+1.0+1.62)/3, npl.Mean, 1e-12)
	assert.Equal(t, 1.0, npl.Median, "奇数个取中位")
	assert.Equal(t, Stat{N: 3, Mean: npl.Mean, Median: 1.0, Best: "A银行", Worst: "C银行", BestVal: 0.94, WorstVal: 1.62}, npl)

	cov := s.Stats[IndCoverage]
	assert.Equal(t, "A银行", cov.Best, "拨备覆盖率越高越好")
	assert.Equal(t, 385.0, cov.BestVal)
	assert.Equal(t, "C银行", cov.Worst)
	assert.Equal(t, 142.0, cov.WorstVal)
	assert.Equal(t, 214.93, cov.Median)
}

func TestSummarizeRankDirection(t *testing.T) {
	rs := []BankResult{
		br("A银行", "2026-06-30", 0.94, 385, 14.07),
		br("B银行", "2026-06-30", 1.0, 214.93, 15),
		br("C银行", "2026-06-30", 1.62, 142, 8.9),
	}
	assert.Equal(t, []string{"B银行", "A银行", "C银行"}, names(Summarize(rs, IndCET1).Current), "CET1 降序")
	assert.Equal(t, []string{"A银行", "B银行", "C银行"}, names(Summarize(rs, IndCoverage).Current), "拨备降序")
	assert.Equal(t, []string{"A银行", "B银行", "C银行"}, names(Summarize(rs, IndNPL).Current), "不良率升序")
	assert.Equal(t, "B银行", Summarize(rs, IndNPL).Stats[IndCET1].Best, "Stats 方向与 rank_by 无关")
}

func TestSummarizeModePeriod(t *testing.T) {
	t.Run("提前披露的一家不拉动统计期", func(t *testing.T) {
		var rs []BankResult
		for i := range 10 {
			rs = append(rs, br(string(rune('A'+i))+"银行", "2026-06-30", 1, 200, 10))
		}
		rs = append(rs, br("早报银行", "2026-12-31", 1, 200, 10))
		s := Summarize(rs, IndNPL)
		assert.Equal(t, day("2026-06-30"), s.Period)
		assert.Len(t, s.Current, 10)
		assert.Equal(t, []string{"早报银行"}, names(s.Ahead))
		assert.Empty(t, s.Stale)
		assert.Equal(t, 10, s.Stats[IndNPL].N)
	})
	for name, order := range map[string][]string{
		"早期在前": {"2026-06-30", "2026-06-30", "2026-09-30", "2026-09-30"},
		"晚期在前": {"2026-09-30", "2026-09-30", "2026-06-30", "2026-06-30"},
		"交错":   {"2026-06-30", "2026-09-30", "2026-06-30", "2026-09-30"},
	} {
		t.Run("并列取较晚/"+name, func(t *testing.T) {
			var rs []BankResult
			for i, p := range order {
				rs = append(rs, br(string(rune('A'+i))+"银行", p, 1, 200, 10))
			}
			s := Summarize(rs, IndNPL)
			assert.Equal(t, day("2026-09-30"), s.Period)
			assert.Len(t, s.Current, 2)
			assert.Len(t, s.Stale, 2)
		})
	}
	t.Run("失败与无数据主体不参与计数", func(t *testing.T) {
		rs := []BankResult{
			failedAt("E1银行", "2026-03-31"), failedAt("E2银行", "2026-03-31"), failedAt("E3银行", "2026-03-31"),
			br("F1银行", "", nan, nan, nan), br("F2银行", "", nan, nan, nan), br("F3银行", "", nan, nan, nan),
			br("A银行", "2026-06-30", 1, 200, 10),
		}
		s := Summarize(rs, IndNPL)
		assert.Equal(t, day("2026-06-30"), s.Period)
		assert.Equal(t, []string{"A银行"}, names(s.Current))
		assert.Equal(t, []string{"F1银行", "F2银行", "F3银行"}, names(s.Stale))
		assert.Len(t, s.Failed, 3)
	})
}

func TestSummarizeMedianEven(t *testing.T) {
	rs := []BankResult{
		br("A银行", "2026-06-30", 0.94, 385, 14.07),
		br("B银行", "2026-06-30", 1.0, 214.93, 10.04),
		br("C银行", "2026-06-30", 1.62, 142, 8.9),
		br("D银行", "2026-06-30", 1.2, 400, 11),
	}
	s := Summarize(rs, IndNPL)
	assert.InDelta(t, (1.0+1.2)/2, s.Stats[IndNPL].Median, 1e-12, "偶数个取中间两数均值")
	// 按 NPL 排名后拨备序列为 385/214.93/400/142（无序），中位数须先排序再取。
	assert.InDelta(t, (214.93+385)/2, s.Stats[IndCoverage].Median, 1e-12)
}

func TestSummarizeExcludesNaNAndFallback(t *testing.T) {
	rs := []BankResult{
		// A 的 CET1 回退到上一期，且回退值若参与统计会成为最优。
		withFallback(br("A银行", "2026-06-30", 0.94, 385, nan), IndCET1, 20, "2026-03-31"),
		br("B银行", "2026-06-30", 1.0, 214.93, 10.04),
		br("N银行", "2026-06-30", 1.1, 200, nan),
		br("C银行", "2026-06-30", 1.62, 142, 14.07),
	}
	s := Summarize(rs, IndCET1)
	assert.Equal(t, []string{"C银行", "B银行", "A银行", "N银行"}, names(s.Current), "回退值与 NaN 排最后，彼此保持配置顺序")
	cet1 := s.Stats[IndCET1]
	assert.Equal(t, 2, cet1.N, "N 只计当期值")
	assert.InDelta(t, (10.04+14.07)/2, cet1.Mean, 1e-12)
	assert.InDelta(t, (10.04+14.07)/2, cet1.Median, 1e-12)
	assert.Equal(t, "C银行", cet1.Best, "回退值 20 不得成为最优")
	assert.Equal(t, 14.07, cet1.BestVal)
	assert.Equal(t, "B银行", cet1.Worst)
	assert.Equal(t, 4, s.Stats[IndNPL].N, "其他指标不受影响")

	// 回退值恰为最差时同样不得入选。
	rs[0] = withFallback(br("A银行", "2026-06-30", 0.94, 385, nan), IndCET1, 5, "2026-03-31")
	cet1 = Summarize(rs, IndCET1).Stats[IndCET1]
	assert.Equal(t, "B银行", cet1.Worst)
	assert.Equal(t, 10.04, cet1.WorstVal)
}

func TestSummarizeTiesKeepConfigOrder(t *testing.T) {
	rs := []BankResult{
		br("B银行", "2026-06-30", 1.0, 200, 10),
		br("A银行", "2026-06-30", 0.9, 200, 10),
		br("D银行", "2026-06-30", 1.0, 200, 10),
		br("C银行", "2026-06-30", 1.0, 200, 10),
	}
	assert.Equal(t, []string{"A银行", "B银行", "D银行", "C银行"}, names(Summarize(rs, IndNPL).Current))
	assert.Equal(t, []string{"B银行", "A银行", "D银行", "C银行"}, names(Summarize(rs, IndCoverage).Current))

	// 超过 12 个元素：sort 包对小切片用插入排序（本身稳定），只有这里能区分 SliceStable 与 Slice。
	var many []BankResult
	var want []string
	for i := range 40 {
		name := fmt.Sprintf("银行%02d", i)
		many = append(many, br(name, "2026-06-30", float64(i%2), 200, 10))
		if i%2 == 0 {
			want = append(want, name)
		}
	}
	for i := 1; i < 40; i += 2 {
		want = append(want, fmt.Sprintf("银行%02d", i))
	}
	assert.Equal(t, want, names(Summarize(many, IndNPL).Current))
}

func TestSummarizeNoCurrent(t *testing.T) {
	s := Summarize([]BankResult{{Name: "E银行", Err: errors.New("x")}, br("F银行", "", nan, nan, nan)}, IndNPL)
	require.True(t, s.Period.IsZero())
	assert.Empty(t, s.Current)
	assert.Empty(t, s.Ahead)
	assert.Equal(t, []string{"F银行"}, names(s.Stale))
	for _, k := range Indicators {
		st := s.Stats[k]
		assert.Equal(t, 0, st.N, k.Label())
		for _, v := range []float64{st.Mean, st.Median, st.BestVal, st.WorstVal} {
			assert.True(t, math.IsNaN(v), k.Label())
		}
		assert.Empty(t, st.Best, k.Label())
		assert.Empty(t, st.Worst, k.Label())
	}
}

// Q27（TASK-005 boundary[2]）：Ahead 主体的 CET1 回退值恰好落在统计期（600919 在 Q3 的真实形态），
// 只靠 Period 过滤挡不住它；必须因为它不在 Current 中而不进统计与排名。
func TestSummarizeAheadFallbackAtPeriodNotInStats(t *testing.T) {
	rs := []BankResult{
		br("A银行", "2026-06-30", 0.94, 385, 14.07),
		br("B银行", "2026-06-30", 1.0, 214.93, 10.04),
		br("C银行", "2026-06-30", 1.62, 142, 9.0),
		// H 的 NPL/拨备已披露 2026-09-30，CET1 仍停在 2026-06-30，且回退值若进统计会成为最差。
		withFallback(br("H银行", "2026-09-30", 0.84, 322.62, nan), IndCET1, 5, "2026-06-30"),
	}
	s := Summarize(rs, IndCET1)
	require.Equal(t, day("2026-06-30"), s.Period)
	assert.Equal(t, []string{"H银行"}, names(s.Ahead))
	assert.Equal(t, []string{"A银行", "B银行", "C银行"}, names(s.Current), "Ahead 主体不在排名中")
	cet1 := s.Stats[IndCET1]
	assert.Equal(t, 3, cet1.N)
	assert.InDelta(t, (14.07+10.04+9.0)/3, cet1.Mean, 1e-12)
	assert.Equal(t, "C银行", cet1.Worst)
	assert.Equal(t, 9.0, cet1.WorstVal)
}
