package crisis

// Context Checkpoint: done_criteria → test mapping (eval)
// functional[0] DayResult 8 行可落库 → TestEvalDayBaselineStaysNormal
// functional[1] 指标行 detail {"raw","window_actual_obs"}；系统行 SysDetail/Indicator="" → TestEvalDayBaselineStaysNormal
// functional[2] 空历史冷启动 NORMAL；Transitioned；基线 AmberCount=2 → TestEvalDayBaselineStaysNormal / LeadingRed
// functional[3] sofr_effr raw≥AMBER 且季末窗→SUPPRESSED_SEASONAL；色彩过 ApplyHysteresis → TestEvalDayQuarterEndSuppression / HysteresisHoldsDemotion / HysteresisReleasesDemotion
// boundary[0]   非色彩状态不过滞回直通 → TestEvalDayNoDataLeavesResonance
// error_handling[0] 失败 SeriesReader→返回错误不落半套 → TestEvalDaySeriesReaderError
// 季末抑制“不抑制”半（窗外 AMBER 不被抑制）→ TestEvalDayNoSuppressionOutsideWindow

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var evalAt = time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)

func TestEvalDayBaselineStaysNormal(t *testing.T) {
	const d = "2026-07-10"
	res, err := EvalDay(testConfig(), d, baselineSeries(d), NewMemHistory(), evalAt)
	require.NoError(t, err)

	assert.Equal(t, StateNormal, res.PrevState) // 空历史 = 冷启动 NORMAL
	assert.Equal(t, StateNormal, res.State)
	assert.False(t, res.Transitioned())
	assert.Equal(t, 2, res.Detail.AmberCount) // hy_oas + usdjpy（附录基线）

	require.Len(t, res.Evaluations, 8) // 7 指标行 + 1 系统行
	sys := res.Evaluations[7]
	assert.Equal(t, "", sys.Indicator)
	assert.Equal(t, StateNormal, sys.SystemState)
	assert.Equal(t, d, sys.TS)
	assert.Contains(t, sys.Detail, `"amber_count":2`)
	vix := res.Evaluations[0]
	assert.Equal(t, IndVIX, vix.Indicator)
	assert.Contains(t, vix.Detail, `"raw":"GREEN"`)
	assert.Contains(t, vix.Detail, `"window_actual_obs":80`)
	assert.Equal(t, NowStamp(evalAt), vix.EvalAt) // 可直接落库
}

func TestEvalDayLeadingRedTransitionsToWatch(t *testing.T) {
	const d = "2026-07-10"
	sr := baselineSeries(d)
	sr[IndNFCI] = seriesEnding(d, 80, 0.1, 0.2) // NFCI > 0 → 领先层红

	res, err := EvalDay(testConfig(), d, sr, NewMemHistory(), evalAt)
	require.NoError(t, err)
	assert.Equal(t, StateWatch, res.State)
	assert.True(t, res.Transitioned())
	assert.True(t, res.Detail.AnyTrigger)
}

func TestEvalDayQuarterEndSuppression(t *testing.T) {
	const d = "2026-03-31" // 季末窗口内（周二，已核实）
	sr := baselineSeries(d)
	sr[IndSOFREFFR] = seriesEnding(d, 80, 15, 15) // 持续 >10bp → raw AMBER

	res, err := EvalDay(testConfig(), d, sr, NewMemHistory(), evalAt)
	require.NoError(t, err)
	r := res.Results[IndSOFREFFR]
	assert.Equal(t, StatusSuppressed, r.Status) // 生效状态被季末抑制
	assert.Equal(t, StatusAmber, r.RawStatus)   // raw 保留审计
}

// 季末抑制“不抑制”半：窗外同样 raw AMBER 的 sofr_effr 不被抑制（经滞回直通 AMBER）。
func TestEvalDayNoSuppressionOutsideWindow(t *testing.T) {
	const d = "2026-07-10" // 季中，非季末窗口
	sr := baselineSeries(d)
	sr[IndSOFREFFR] = seriesEnding(d, 80, 15, 15) // 持续 >10bp → raw AMBER

	res, err := EvalDay(testConfig(), d, sr, NewMemHistory(), evalAt)
	require.NoError(t, err)
	r := res.Results[IndSOFREFFR]
	assert.Equal(t, StatusAmber, r.Status) // 未抑制
	assert.Equal(t, StatusAmber, r.RawStatus)
}

func TestEvalDayNoDataLeavesResonance(t *testing.T) {
	const d = "2026-07-10"
	sr := baselineSeries(d)
	delete(sr, IndSOFREFFR) // 模拟 2018 前 SOFR 序列不存在（回测早期段）

	res, err := EvalDay(testConfig(), d, sr, NewMemHistory(), evalAt)
	require.NoError(t, err)
	assert.Equal(t, StatusNoData, res.Results[IndSOFREFFR].Status) // 非色彩直通
	assert.Equal(t, StateNormal, res.State)
	assert.Equal(t, 2, res.Detail.AmberCount) // NO_DATA 不入计数
}

func TestEvalDayHysteresisHoldsDemotion(t *testing.T) {
	const d = "2026-07-10"
	sr := baselineSeries(d) // 今日 raw 全绿基线（move 绿）
	hist := NewMemHistory()
	hist.Append([]Evaluation{{Indicator: IndMOVE, Status: StatusAmber, Detail: `{"raw":"AMBER"}`}})

	res, err := EvalDay(testConfig(), d, sr, hist, evalAt)
	require.NoError(t, err)
	r := res.Results[IndMOVE]
	assert.Equal(t, StatusGreen, r.RawStatus)
	assert.Equal(t, StatusAmber, r.Status) // 降级被防抖挡住（昨日 raw AMBER）
}

// 滞回“放行降级”半：今日 raw 绿 + 此前 demote_hysteresis_days-1 日 raw 全绿 → 放行到 GREEN。
func TestEvalDayHysteresisReleasesDemotion(t *testing.T) {
	const d = "2026-07-10"
	sr := baselineSeries(d) // 今日 move raw 绿
	hist := NewMemHistory()
	// 2 日历史（=demote_hysteresis_days-1）：生效 AMBER 但 raw GREEN → 连续低档
	hist.Append([]Evaluation{{Indicator: IndMOVE, Status: StatusAmber, Detail: `{"raw":"GREEN"}`}})
	hist.Append([]Evaluation{{Indicator: IndMOVE, Status: StatusAmber, Detail: `{"raw":"GREEN"}`}})

	res, err := EvalDay(testConfig(), d, sr, hist, evalAt)
	require.NoError(t, err)
	r := res.Results[IndMOVE]
	assert.Equal(t, StatusGreen, r.RawStatus)
	assert.Equal(t, StatusGreen, r.Status) // 连续低档放行降级
}

// errReader 恒报错，用于验证 EvalDay 在指标评估出错时返回错误、不落半套结果。
type errReader struct{ err error }

func (e errReader) Window(string, string, int) ([]Observation, error)          { return nil, e.err }
func (e errReader) WindowSince(string, string, string) ([]Observation, error) { return nil, e.err }

func TestEvalDaySeriesReaderError(t *testing.T) {
	const d = "2026-07-10"
	res, err := EvalDay(testConfig(), d, errReader{err: assertErr}, NewMemHistory(), evalAt)
	require.ErrorIs(t, err, assertErr)
	assert.Nil(t, res) // 不落半套结果
}

// EvalHistory 读失败也透传（滞回阶段 hist.RecentIndicator 报错）：sr 正常、hist 恒报错。
func TestEvalDayHistoryError(t *testing.T) {
	const d = "2026-07-10"
	res, err := EvalDay(testConfig(), d, baselineSeries(d), errHistory{err: assertErr}, evalAt)
	require.ErrorIs(t, err, assertErr)
	assert.Nil(t, res)
}

// TestEvalDayResumesPreviousState 覆盖 prevState 从历史系统行恢复的半边（eval.go:49-50，
// 冷启动半之外）：非空历史 → PrevState 取历史系统行 SystemState 而非 NORMAL。replay
// 的逐日状态 carry 依赖此分支——坏掉则每日从 NORMAL 重启、多日转移链断裂。
func TestEvalDayResumesPreviousState(t *testing.T) {
	const d = "2026-07-10"
	hist := histWithSystem(1, SysDetail{AnyTrigger: false, Prev: StateWatch}) // 一条 WATCH 系统行

	res, err := EvalDay(testConfig(), d, baselineSeries(d), hist, evalAt)
	require.NoError(t, err)
	assert.Equal(t, StateWatch, res.PrevState) // 从历史恢复（非冷启动 NORMAL）
	assert.Equal(t, StateWatch, res.State)     // 历史仅 1 条 < 19 → 不退出，维持 WATCH
	assert.False(t, res.Transitioned())
}

// 评估行 detail JSON 同步携带新字段（审计与"较昨日"对比，通知设计 §8）。
func TestBuildEvaluationsCarriesPersistAndWow(t *testing.T) {
	r := &DayResult{Date: "2026-07-10", Results: map[string]IndicatorResult{}}
	for _, ind := range AllIndicators {
		r.Results[ind] = IndicatorResult{Indicator: ind, Status: StatusGreen, RawStatus: StatusGreen}
	}
	r.Results[IndSOFREFFR] = IndicatorResult{Indicator: IndSOFREFFR,
		Status: StatusRed, RawStatus: StatusRed, PersistDays: 9}
	r.Results[IndUSDJPY] = IndicatorResult{Indicator: IndUSDJPY,
		Status: StatusRed, RawStatus: StatusRed, Wow: -0.031, WowOK: true}

	evals, err := buildEvaluations(r, time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	byInd := map[string]Evaluation{}
	for _, e := range evals {
		byInd[e.Indicator] = e
	}
	assert.Contains(t, byInd[IndSOFREFFR].Detail, `"persist_days":9`)
	assert.Contains(t, byInd[IndUSDJPY].Detail, `"wow":-0.031`)
	assert.Contains(t, byInd[IndUSDJPY].Detail, `"wow_ok":true`)
}
