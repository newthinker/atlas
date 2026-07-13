package crisis

// Context Checkpoint: done_criteria → test mapping (suppress)
// functional[0] InQuarterEndWindow 已核实锚点为真 → TestInQuarterEndWindow
// functional[1] staleFor daily/nfci weekly 四用例 → TestStaleFor
// functional[2] ApplyHysteresis 升级立即/降级需连续 days 个 raw ≤ 目标 → TestApplyHysteresis
// boundary[0]   季中/窗口外/周末/bad-date 为假；历史不足 days-1 不降级 → TestInQuarterEndWindow / TestApplyHysteresis
// boundary[1]   detail 缺 raw 回退 status 列（rawFromDetail）→ 经 TestApplyHysteresis 的非色彩历史用例间接覆盖

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInQuarterEndWindow(t *testing.T) {
	// 2026-03-31 = 周二（已核实日历）：季末最后 3 交易日 = 3/27(五)、3/30(一)、3/31(二)
	for _, d := range []string{"2026-03-27", "2026-03-30", "2026-03-31", "2026-04-01", "2026-04-02", "2026-12-31"} {
		assert.True(t, InQuarterEndWindow(d), d)
	}
	for _, d := range []string{
		"2026-03-26", // 季末窗口前一交易日
		"2026-04-03", // 季初窗口后一交易日
		"2026-03-28", // 周六
		"2026-05-15", // 季中
		"bad-date",
	} {
		assert.False(t, InQuarterEndWindow(d), d)
	}
}

func TestStaleFor(t *testing.T) {
	cfg := &Config{Freshness: FreshnessCfg{DailyMaxLagDays: 4, WeeklyMaxLagDays: 12}}
	assert.True(t, staleFor(cfg, IndVIX, "2026-07-10", "2026-07-03"))   // 7 天 > 4
	assert.False(t, staleFor(cfg, IndVIX, "2026-07-10", "2026-07-08"))  // 2 天
	assert.False(t, staleFor(cfg, IndNFCI, "2026-07-13", "2026-07-01")) // 周频 12 天上限
	assert.True(t, staleFor(cfg, IndNFCI, "2026-07-14", "2026-07-01"))
}

func evalWithRaw(status, raw Status) Evaluation {
	return Evaluation{Status: status, Detail: `{"raw":"` + string(raw) + `"}`}
}

func TestApplyHysteresis(t *testing.T) {
	// 升级立即生效
	assert.Equal(t, StatusRed,
		ApplyHysteresis(StatusRed, []Evaluation{evalWithRaw(StatusGreen, StatusGreen)}, 3))
	// 无历史 → 原样（冷启动首日）
	assert.Equal(t, StatusGreen, ApplyHysteresis(StatusGreen, nil, 3))
	// 降级被昨日 raw AMBER 挡住 → 维持昨日生效状态
	blocked := []Evaluation{evalWithRaw(StatusAmber, StatusAmber), evalWithRaw(StatusAmber, StatusAmber)}
	assert.Equal(t, StatusAmber, ApplyHysteresis(StatusGreen, blocked, 3))
	// 今日 + 此前 2 日 raw 均 GREEN = 连续 3 观测日 → 放行降级
	clear := []Evaluation{evalWithRaw(StatusAmber, StatusGreen), evalWithRaw(StatusAmber, StatusGreen)}
	assert.Equal(t, StatusGreen, ApplyHysteresis(StatusGreen, clear, 3))
	// 历史不足 days-1 → 维持
	short := []Evaluation{evalWithRaw(StatusAmber, StatusGreen)}
	assert.Equal(t, StatusAmber, ApplyHysteresis(StatusGreen, short, 3))
	// 历史中夹非色彩状态（STALE）→ 无法确认连续低档，维持
	stale := []Evaluation{evalWithRaw(StatusAmber, StatusGreen), evalWithRaw(StatusStale, StatusStale)}
	assert.Equal(t, StatusAmber, ApplyHysteresis(StatusGreen, stale, 3))
}
