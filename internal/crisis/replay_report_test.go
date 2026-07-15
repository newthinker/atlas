package crisis

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Context Checkpoint: done_criteria → test mapping (TASK-002 ReplayReport)
// functional[0]     daily 首行回放态+StateDays / PrevDay 链差异行 / 页脚 → TestReplayReportDaily
// functional[1]     monthly 21 日 Trends / 空窗口指标省略           → TestReplayReportMonthly
// boundary[0]       窗口首日 prev=nil → "较昨日：无变化"             → TestReplayReportDailyFirstDay
// boundary[1]       状态门控忽略 NORMAL 日也渲染                     → TestReplayReportDailyIgnoresGate
// error_handling[0] 未知 form 报错且含 form 值                       → TestReplayReportUnknownForm

// mkReplayDay 手工构造回放日：7 指标默认 GREEN，statuses/vals 覆盖指定指标。
func mkReplayDay(date string, prev, state SystemState, stateDays, amber int,
	statuses map[string]Status, vals map[string]float64) ReplayDay {
	res := &DayResult{
		Date: date, Results: map[string]IndicatorResult{},
		PrevState: prev, State: state,
		Detail: SysDetail{Date: date, AmberCount: amber, Prev: prev},
	}
	for _, ind := range AllIndicators {
		r := IndicatorResult{Indicator: ind, Status: StatusGreen, Value: 10, Pct5y: -1}
		if s, ok := statuses[ind]; ok {
			r.Status = s
		}
		if v, ok := vals[ind]; ok {
			r.Value = v
		}
		res.Results[ind] = r
	}
	return ReplayDay{Date: date, Res: res, StateDays: stateDays}
}

// functional: daily 首行含回放态与 StateDays；PrevDay 链使差异行真实可用。
func TestReplayReportDaily(t *testing.T) {
	prev := mkReplayDay("2026-07-09", StateCrisis, StateCrisis, 1, 2, nil, nil)
	day := mkReplayDay("2026-07-10", StateCrisis, StateCrisis, 2, 2,
		map[string]Status{IndVIX: StatusRed}, map[string]float64{IndVIX: 42})

	out, err := ReplayReport(testConfig(), "daily", day, &prev, memSeries{})
	require.NoError(t, err)
	assert.Contains(t, out, "CRISIS 日报 第 2 日")
	assert.Contains(t, out, "较昨日：")
	assert.Contains(t, out, "vix 转红（原绿）") // PrevDay 链生效
	assert.Contains(t, out, "非交易信号")       // 消息家族页脚保留
}

// boundary: 窗口首日 prev=nil → PrevDay 空 map → 差异行"无变化"。
func TestReplayReportDailyFirstDay(t *testing.T) {
	day := mkReplayDay("2026-07-10", StateBrewing, StateBrewing, 3, 1, nil, nil)
	out, err := ReplayReport(testConfig(), "daily", day, nil, memSeries{})
	require.NoError(t, err)
	assert.Contains(t, out, "较昨日：无变化")
}

// boundary: 状态门控忽略——NORMAL 日也渲染 daily。
func TestReplayReportDailyIgnoresGate(t *testing.T) {
	day := mkReplayDay("2026-07-10", StateNormal, StateNormal, 5, 0, nil, nil)
	out, err := ReplayReport(testConfig(), "daily", day, nil, memSeries{})
	require.NoError(t, err)
	assert.Contains(t, out, "NORMAL 日报 第 5 日")
}

// functional: monthly 从 sr 装 21 日 Trends；空窗口指标省略趋势行。
func TestReplayReportMonthly(t *testing.T) {
	sr := memSeries{IndVIX: seriesEnding("2026-07-01", 21, 15, 18)}
	day := mkReplayDay("2026-07-01", StateNormal, StateNormal, 40, 0, nil,
		map[string]float64{IndVIX: 18})
	out, err := ReplayReport(testConfig(), "monthly", day, nil, sr)
	require.NoError(t, err)
	assert.Contains(t, out, "Cassandra 月报 · 2026-07")
	assert.Contains(t, out, "近 21 个交易日趋势")
	require.Equal(t, 1, strings.Count(out, "vix"), "仅 vix 有趋势行")
	assert.NotContains(t, out, "hy_oas") // 空窗口省略
}

// error_handling: 未知 form 报错。
func TestReplayReportUnknownForm(t *testing.T) {
	day := mkReplayDay("2026-07-10", StateNormal, StateNormal, 1, 0, nil, nil)
	_, err := ReplayReport(testConfig(), "weekly", day, nil, memSeries{})
	assert.ErrorContains(t, err, "weekly")
}
